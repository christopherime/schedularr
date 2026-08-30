// Unit tests for the encrypted token vault (runtime/token.ts). Node >= 20
// ships WebCrypto as globalThis.crypto, so the AES-GCM round trip runs for
// REAL here. IndexedDB does not exist under Node, so the key store -- the
// one seam token.ts abstracts -- is an in-memory fake honoring the same
// contract (the CryptoKey object persists as-is, like structured clone),
// and localStorage is a Map-backed fake injected the same way.
import assert from "node:assert/strict";
import test from "node:test";

import type { TokenKeyStore } from "../assets/ts/runtime/token.ts";

const { createTokenVault } = await import("../assets/ts/runtime/token.ts");

const LEGACY_KEY = "schedularr_api_token";
const V2_KEY = "schedularr_api_token_v2";
const BASE64 = /^[A-Za-z0-9+/]+=*$/;

function memoryKeyStore() {
  const store = {
    held: null as CryptoKey | null,
    loads: 0,
    async load(): Promise<CryptoKey | null> {
      store.loads += 1;
      return store.held;
    },
    async save(key: CryptoKey): Promise<void> {
      store.held = key;
    },
  };
  return store;
}

const brokenKeyStore: TokenKeyStore = {
  load: () => Promise.reject(new Error("indexedDB unavailable")),
  save: () => Promise.reject(new Error("indexedDB unavailable")),
};

function memoryStorage() {
  const items = new Map<string, string>();
  return {
    items,
    getItem: (name: string) => items.get(name) ?? null,
    setItem: (name: string, value: string) => {
      items.set(name, value);
    },
    removeItem: (name: string) => {
      items.delete(name);
    },
  };
}

test("setToken writes {iv, ciphertext} base64, never the plaintext", async () => {
  const keys = memoryKeyStore();
  const storage = memoryStorage();
  const vault = createTokenVault(keys, () => storage);

  await vault.setToken("tunarr-bearer-9000");
  assert.equal(vault.getToken(), "tunarr-bearer-9000");

  const stored = storage.items.get(V2_KEY);
  assert.ok(stored, "v2 entry written");
  assert.ok(!stored.includes("tunarr-bearer-9000"), "plaintext must not appear at rest");
  const entry = JSON.parse(stored) as { iv: string; ciphertext: string };
  assert.match(entry.iv, BASE64);
  assert.match(entry.ciphertext, BASE64);
  assert.equal(storage.items.has(LEGACY_KEY), false);
});

test("the encryption key is generated non-extractable, encrypt/decrypt only", async () => {
  const keys = memoryKeyStore();
  const vault = createTokenVault(keys, () => memoryStorage());
  await vault.loadToken();
  assert.ok(keys.held, "key generated and saved on first hydration");
  assert.equal(keys.held.extractable, false);
  assert.deepEqual([...keys.held.usages].sort(), ["decrypt", "encrypt"]);
});

test("round trip: a fresh vault over the same stores decrypts the token", async () => {
  const keys = memoryKeyStore();
  const storage = memoryStorage();
  await createTokenVault(keys, () => storage).setToken("survives-reload");

  const reload = createTokenVault(keys, () => storage);
  assert.equal(reload.getToken(), null, "cache empty before hydration");
  assert.equal(await reload.loadToken(), "survives-reload");
  assert.equal(reload.getToken(), "survives-reload", "sync reads work after hydration");
});

test("every encryption uses a fresh IV", async () => {
  const storage = memoryStorage();
  const vault = createTokenVault(memoryKeyStore(), () => storage);
  await vault.setToken("same-token");
  const first = JSON.parse(storage.items.get(V2_KEY) ?? "") as { iv: string };
  await vault.setToken("same-token");
  const second = JSON.parse(storage.items.get(V2_KEY) ?? "") as { iv: string };
  assert.notEqual(first.iv, second.iv);
});

test("migration: legacy plaintext is adopted, re-encrypted, and removed", async () => {
  const keys = memoryKeyStore();
  const storage = memoryStorage();
  storage.items.set(LEGACY_KEY, "legacy-plain-token");

  const vault = createTokenVault(keys, () => storage);
  assert.equal(await vault.loadToken(), "legacy-plain-token");
  assert.equal(storage.items.has(LEGACY_KEY), false, "plaintext entry removed");
  const stored = storage.items.get(V2_KEY);
  assert.ok(stored, "encrypted v2 entry written");
  assert.ok(!stored.includes("legacy-plain-token"));

  // The next session reads the migrated ciphertext back.
  assert.equal(await createTokenVault(keys, () => storage).loadToken(), "legacy-plain-token");
});

test("loadToken is memoized single-flight", async () => {
  const keys = memoryKeyStore();
  const vault = createTokenVault(keys, () => memoryStorage());
  const [a, b] = await Promise.all([vault.loadToken(), vault.loadToken()]);
  await vault.loadToken();
  assert.equal(a, b);
  assert.equal(keys.loads, 1, "one hydration pass for all callers");
});

test("unavailable crypto degrades to memory-only, never plaintext at rest", async () => {
  const storage = memoryStorage();
  storage.items.set(LEGACY_KEY, "legacy-plain-token");

  const vault = createTokenVault(brokenKeyStore, () => storage);
  assert.equal(await vault.loadToken(), "legacy-plain-token", "session keeps the token in memory");
  assert.equal(storage.items.has(LEGACY_KEY), false, "plaintext removed even without crypto");
  assert.equal(storage.items.has(V2_KEY), false, "no unencrypted fallback entry");

  await vault.setToken("fresh-token");
  assert.equal(vault.getToken(), "fresh-token");
  assert.equal(storage.items.size, 0, "memory-only: nothing persisted");

  await vault.clearToken();
  assert.equal(vault.getToken(), null);
});

test("clearToken removes the entry and cache but keeps the CryptoKey", async () => {
  const keys = memoryKeyStore();
  const storage = memoryStorage();
  const vault = createTokenVault(keys, () => storage);
  await vault.setToken("to-be-cleared");

  await vault.clearToken();
  assert.equal(vault.getToken(), null);
  assert.equal(storage.items.has(V2_KEY), false);
  assert.ok(keys.held, "encryption key survives a clear for the next token");
  assert.equal(await createTokenVault(keys, () => storage).loadToken(), null);
});

test("undecryptable ciphertext is dropped and treated as no token", async () => {
  const keys = memoryKeyStore();
  const storage = memoryStorage();
  // A valid-looking entry encrypted under a key we no longer hold.
  storage.items.set(V2_KEY, JSON.stringify({ iv: "AAAAAAAAAAAAAAAA", ciphertext: "AAAAAAAAAAAAAAAAAAAAAAAA" }));
  const vault = createTokenVault(keys, () => storage);
  assert.equal(await vault.loadToken(), null);
  assert.equal(storage.items.has(V2_KEY), false, "dead ciphertext removed");

  // Outright garbage (not even JSON) is equally non-fatal.
  storage.items.set(V2_KEY, "not-json");
  assert.equal(await createTokenVault(keys, () => storage).loadToken(), null);
});
