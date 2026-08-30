// Storage for the Schedularr API bearer token -- encrypted at rest since
// v0.5.5 (GitHub code-scanning alert #2, CodeQL
// js/clear-text-storage-of-sensitive-data: the token used to sit in
// localStorage as plaintext). There is still no server session, no cookie,
// no CSRF surface (PRODUCT.md, "Token-once, same-origin") -- only the
// resting shape changed.
//
// How it works: an AES-GCM-256 CryptoKey is generated once with
// `extractable: false` -- scripts can USE it to encrypt/decrypt but can
// never read the key material back out -- and persisted in IndexedDB,
// which structured-clones the CryptoKey object itself. The token is
// encrypted under a fresh random IV on every write and stored in
// localStorage as {iv, ciphertext} (base64) under TOKEN_KEY.
//
// Threat model, in plain language: this defends the AT-REST copy --
// browser-profile disk files and their backups, extensions or tooling that
// scrape localStorage wholesale, exactly the CodeQL class above. It does
// NOT defend against a script running in this page: an XSS payload could
// call WebCrypto with the same key and decrypt the token the same way this
// module does. The strict same-origin CSP (docs/web-ui-guide.md) is the
// defense on that front, not this encryption.
//
// Key naming: the legacy v1 key ("schedularr_api_token") held plaintext
// and is retired -- hydration transparently migrates a surviving v1 value
// into the encrypted v2 entry and deletes the plaintext, so the migration
// IS the sanctioned rename path for the old "do not rename" contract. The
// operator-facing contract now lives on the v2 key: do not rename either
// key again outside a migration like this one.
const LEGACY_TOKEN_KEY = "schedularr_api_token";
const TOKEN_KEY = "schedularr_api_token_v2";

// ---- key store -----------------------------------------------------------
//
// The one seam this module abstracts: where the CryptoKey lives. The real
// implementation is IndexedDB (the only browser store that persists a
// non-extractable CryptoKey); the unit tests inject an in-memory fake with
// the same contract, because Node has WebCrypto but no IndexedDB.

/** Persists the (non-extractable) token-encryption CryptoKey. */
export interface TokenKeyStore {
  load(): Promise<CryptoKey | null>;
  save(key: CryptoKey): Promise<void>;
}

const IDB_NAME = "schedularr";
const IDB_STORE = "crypto-keys";
const IDB_ENTRY = "token-key";

function openKeyDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const open = indexedDB.open(IDB_NAME, 1);
    open.onupgradeneeded = () => {
      open.result.createObjectStore(IDB_STORE);
    };
    open.onsuccess = () => resolve(open.result);
    open.onerror = () => reject(open.error ?? new Error("indexedDB open failed"));
  });
}

const idbKeyStore: TokenKeyStore = {
  async load(): Promise<CryptoKey | null> {
    const db = await openKeyDb();
    try {
      const store = db.transaction(IDB_STORE, "readonly").objectStore(IDB_STORE);
      const got = await new Promise<unknown>((resolve, reject) => {
        const req = store.get(IDB_ENTRY);
        req.onsuccess = () => resolve(req.result as unknown);
        req.onerror = () => reject(req.error ?? new Error("indexedDB read failed"));
      });
      return got instanceof CryptoKey ? got : null;
    } finally {
      db.close();
    }
  },
  async save(key: CryptoKey): Promise<void> {
    const db = await openKeyDb();
    try {
      const tx = db.transaction(IDB_STORE, "readwrite");
      tx.objectStore(IDB_STORE).put(key, IDB_ENTRY);
      await new Promise<void>((resolve, reject) => {
        tx.oncomplete = () => resolve();
        tx.onabort = () => reject(tx.error ?? new Error("indexedDB write aborted"));
        tx.onerror = () => reject(tx.error ?? new Error("indexedDB write failed"));
      });
    } finally {
      db.close();
    }
  },
};

// ---- crypto --------------------------------------------------------------

interface EncryptedEntry {
  iv: string;
  ciphertext: string;
}

function toBase64(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

function fromBase64(text: string): Uint8Array<ArrayBuffer> {
  const bin = atob(text);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i += 1) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

async function encryptToken(key: CryptoKey, value: string): Promise<string> {
  // Fresh random IV on EVERY encryption -- AES-GCM's guarantees collapse
  // when an IV is reused under the same key.
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, new TextEncoder().encode(value));
  const entry: EncryptedEntry = { iv: toBase64(iv), ciphertext: toBase64(new Uint8Array(ciphertext)) };
  return JSON.stringify(entry);
}

async function decryptToken(key: CryptoKey, stored: string): Promise<string> {
  const entry = JSON.parse(stored) as Partial<EncryptedEntry>;
  if (typeof entry.iv !== "string" || typeof entry.ciphertext !== "string") {
    throw new Error("malformed encrypted token entry");
  }
  const plaintext = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: fromBase64(entry.iv) },
    key,
    fromBase64(entry.ciphertext),
  );
  return new TextDecoder().decode(plaintext);
}

// ---- vault ---------------------------------------------------------------

type TokenStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;

/** The token module's surface; createTokenVault is exported for the unit
 * tests, which build vaults over in-memory fakes. */
export interface TokenVault {
  loadToken(): Promise<string | null>;
  getToken(): string | null;
  setToken(value: string): Promise<void>;
  clearToken(): Promise<void>;
}

export function createTokenVault(
  keys: TokenKeyStore,
  storage: () => TokenStorage = () => window.localStorage,
): TokenVault {
  // In-memory cache -- the source of truth between hydration and the next
  // write. getToken() reads it synchronously, so the sync call sites keep
  // working once loadToken() has resolved.
  let cached: string | null = null;
  // The encryption key, or null in memory-only mode (see hydrate).
  let key: CryptoKey | null = null;
  let hydration: Promise<string | null> | null = null;

  function read(name: string): string | null {
    try {
      return storage().getItem(name);
    } catch {
      return null;
    }
  }

  function removeQuiet(name: string): void {
    try {
      storage().removeItem(name);
    } catch {
      // Unavailable storage holds nothing worth removing.
    }
  }

  async function obtainKey(): Promise<CryptoKey> {
    const existing = await keys.load();
    if (existing) return existing;
    const fresh = await crypto.subtle.generateKey({ name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
    await keys.save(fresh);
    return fresh;
  }

  async function hydrate(): Promise<string | null> {
    try {
      key = await obtainKey();
    } catch {
      // WebCrypto or IndexedDB unavailable (rare: private-mode lockdowns,
      // storage globally disabled). Honest degradation: the token lives in
      // memory only -- it works until reload and the token panel simply
      // re-opens next session. NEVER fall back to persisting plaintext;
      // plaintext at rest is the exact defect this module removes.
      key = null;
    }

    const legacy = read(LEGACY_TOKEN_KEY);
    if (legacy !== null) {
      // One-time transparent migration from the plaintext v1 entry: adopt
      // the value, persist it encrypted when we can, and remove the
      // plaintext UNCONDITIONALLY -- even when encryption is unavailable,
      // plaintext must not survive this read.
      cached = legacy;
      if (key !== null) {
        try {
          storage().setItem(TOKEN_KEY, await encryptToken(key, legacy));
        } catch {
          // Persisting failed; the in-memory copy still carries the session.
        }
      }
      removeQuiet(LEGACY_TOKEN_KEY);
      return cached;
    }

    if (key === null) return cached;
    const stored = read(TOKEN_KEY);
    if (stored === null) return cached;
    try {
      cached = await decryptToken(key, stored);
    } catch {
      // Undecryptable ciphertext (the IndexedDB key was cleared while
      // localStorage survived, or the entry is corrupt) can never be read
      // again -- drop it and treat it as "no token".
      removeQuiet(TOKEN_KEY);
      cached = null;
    }
    return cached;
  }

  function loadToken(): Promise<string | null> {
    // Memoized single-flight: every caller shares ONE hydration pass, and
    // later calls resolve immediately from the same settled promise.
    hydration ??= hydrate();
    return hydration;
  }

  function getToken(): string | null {
    return cached;
  }

  async function setToken(value: string): Promise<void> {
    await loadToken();
    if (key !== null) {
      // Persist first: a failed write throws to the caller (the token
      // panel surfaces it) and leaves the previous state intact.
      storage().setItem(TOKEN_KEY, await encryptToken(key, value));
    }
    // Memory-only mode persists nothing (see hydrate) -- the cache alone
    // carries the token for this session.
    cached = value;
  }

  async function clearToken(): Promise<void> {
    await loadToken();
    // Cache first, so the in-memory token is gone even when the storage
    // remove throws (which the caller surfaces, matching setToken). The
    // IndexedDB CryptoKey deliberately stays: it holds no secret once the
    // ciphertext is gone, and the next token reuses it.
    cached = null;
    if (key === null) {
      removeQuiet(TOKEN_KEY);
      return;
    }
    storage().removeItem(TOKEN_KEY);
  }

  return { loadToken, getToken, setToken, clearToken };
}

const vault = createTokenVault(idbKeyStore);

/**
 * Hydrates the token cache (open the IndexedDB key, decrypt the stored
 * entry, run the one-time plaintext migration) and resolves to the token,
 * or null when none is stored or storage is unavailable. Memoized
 * single-flight -- await it anywhere ordering matters (the API client does
 * before attaching the Authorization header); after the first resolution
 * it is effectively synchronous. Never rejects.
 */
export const loadToken = vault.loadToken;

/**
 * Returns the cached token, or null when none is stored OR hydration has
 * not resolved yet -- sync call sites are only trustworthy after an
 * awaited loadToken() (shell init awaits it before the auto-open
 * decision).
 */
export const getToken = vault.getToken;

/**
 * Encrypts and persists the API token, then updates the cache. Rejects if
 * the write fails (quota, disabled storage); callers surface that to the
 * user instead of pretending the save succeeded. In memory-only
 * degradation the token is cached without persisting -- valid until
 * reload.
 */
export const setToken = vault.setToken;

/** Removes the stored API token and the cache (the encryption key stays).
 * Rejects under the same conditions as setToken. */
export const clearToken = vault.clearToken;
