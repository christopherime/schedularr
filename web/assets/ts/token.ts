// Token storage for the Schedularr API bearer token. localStorage only --
// there is no server session, no cookie, no CSRF surface (see PRODUCT.md,
// "Token-once, same-origin"). The key is part of the binding contract for
// Tasks 4-7: do not rename it.
const TOKEN_KEY = "schedularr_api_token";

/**
 * Returns the stored API token, or null when none is stored or
 * localStorage is unavailable (private-browsing lockdown, disabled
 * storage, etc.). A read failure is treated the same as "no token" --
 * every caller already has to handle that case.
 */
export function getToken(): string | null {
  try {
    return window.localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

/**
 * Persists the API token. Throws if localStorage is unavailable or the
 * write fails (quota, disabled storage); callers surface that to the
 * user instead of pretending the save succeeded.
 */
export function setToken(value: string): void {
  window.localStorage.setItem(TOKEN_KEY, value);
}

/** Removes the stored API token. Throws under the same conditions as setToken. */
export function clearToken(): void {
  window.localStorage.removeItem(TOKEN_KEY);
}
