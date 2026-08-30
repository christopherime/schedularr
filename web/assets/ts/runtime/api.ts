// Typed fetch wrapper for /api/v1 -- the ONLY module allowed to call
// fetch() against the Schedularr API. Page modules go through
// apiGet/apiSend so token injection, problem+json parsing, 401 handling,
// timeouts, and the mutation entry guard stay in one place (PRODUCT.md,
// "Contract-first" and "No silent failures").
//
// Since the v0.5.0 runtime refactor every page bundle compiles this module
// in as part of ONE bundle per page (see partials/ui/page-js.html), so the
// ApiError class identity is the same everywhere `instanceof` checks it --
// the old `window.schedularr` cross-bundle identity hack is gone.
import type { operations, paths } from "../gen/types";
import { getToken } from "./token.ts";

/** RFC 7807 problem+json fields, matching components["schemas"]["Problem"]. */
export interface ProblemFields {
  type: string;
  title: string;
  status: number;
  detail?: string;
  request_id?: string;
}

/**
 * Thrown by apiGet/apiSend for any non-2xx response. Carries the parsed
 * problem+json fields (falling back to sane defaults when the server
 * didn't send a problem+json body at all, e.g. a proxy-generated error).
 * A timed-out request throws this too, with status 0 (no HTTP response).
 */
export class ApiError extends Error {
  readonly type: string;
  readonly title: string;
  readonly status: number;
  readonly detail?: string;
  readonly request_id?: string;

  constructor(problem: ProblemFields) {
    super(problem.detail ?? problem.title);
    this.name = "ApiError";
    this.type = problem.type;
    this.title = problem.title;
    this.status = problem.status;
    this.detail = problem.detail;
    this.request_id = problem.request_id;
  }
}

// ---- typed path building -------------------------------------------------
//
// The contract's paths (gen/types.d.ts, generated from api/openapi.yaml)
// are the only strings apiPath accepts -- a typo'd path is a compile
// error, not a runtime 404. Path parameters are substituted with
// encodeURIComponent (show titles routinely contain spaces/punctuation);
// query values are appended only when defined, matching the wire
// convention of omitting rather than sending blanks.

const API_PREFIX = "/api/v1";

/** Builds a concrete request path from a contract path template. */
export function apiPath<P extends keyof paths & string>(
  path: P,
  params?: Record<string, string | number>,
  query?: Record<string, string | number | undefined>,
): string {
  const concrete = path.replace(/\{([^}]+)\}/g, (_, name: string) => {
    const value = params?.[name];
    if (value === undefined) {
      throw new Error(`apiPath: missing path parameter "${name}" for ${path}`);
    }
    return encodeURIComponent(String(value));
  });
  let qs = "";
  if (query) {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined) search.set(key, String(value));
    }
    const s = search.toString();
    if (s !== "") qs = `?${s}`;
  }
  return API_PREFIX + concrete + qs;
}

// ---- 401 + re-auth wiring ------------------------------------------------

type Callback = () => void;
const unauthorizedCallbacks: Callback[] = [];
const reauthCallbacks: Callback[] = [];

/**
 * Registers a callback fired every time a request comes back 401. The
 * shell (runtime/shell.ts) registers one that opens the token panel;
 * callers never retry automatically -- a 401 does not auto-retry the
 * failed action (the re-auth broadcast below is what re-fires loads,
 * and only after a new token has actually probed successfully).
 */
export function onUnauthorized(cb: Callback): void {
  unauthorizedCallbacks.push(cb);
}

export function notifyUnauthorized(): void {
  for (const cb of unauthorizedCallbacks) cb();
}

/**
 * Registers a callback fired when the token panel's Save has probed GET
 * /status successfully with a new token ("arming"). Page components
 * register one in init() that re-fires whichever of their loads failed --
 * this replaces the per-section-Retry-after-401 grind.
 */
export function onReauth(cb: Callback): void {
  reauthCallbacks.push(cb);
}

export function broadcastReauth(): void {
  for (const cb of reauthCallbacks) cb();
}

// ---- request plumbing ----------------------------------------------------

async function parseProblem(res: Response): Promise<ApiError> {
  const contentType = res.headers.get("Content-Type") ?? "";
  if (contentType.includes("json")) {
    try {
      const body = (await res.json()) as Partial<ProblemFields>;
      return new ApiError({
        type: body.type ?? "about:blank",
        title: body.title ?? res.statusText ?? "request failed",
        status: body.status ?? res.status,
        detail: body.detail,
        request_id: body.request_id,
      });
    } catch {
      // Body wasn't valid JSON despite the content type -- fall through
      // to the generic problem below rather than throwing a parse error
      // out of an error path.
    }
  }
  return new ApiError({
    type: "about:blank",
    title: res.statusText || "request failed",
    status: res.status,
  });
}

async function parseBody<T>(res: Response): Promise<T> {
  if (res.status === 204) {
    return undefined as T;
  }
  const contentType = res.headers.get("Content-Type") ?? "";
  if (contentType.includes("json")) {
    return (await res.json()) as T;
  }
  // application/yaml (blocks export) and anything else text-shaped.
  return (await res.text()) as unknown as T;
}

// Reads never take this long against a same-LAN instance; writes get more
// headroom because /generate and /apply do real planning work against
// Tunarr before they answer.
const GET_TIMEOUT_MS = 15_000;
const SEND_TIMEOUT_MS = 60_000;

async function request<T>(method: string, path: string, body: unknown, timeoutMs: number): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  let requestBody: BodyInit | undefined;
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    requestBody = JSON.stringify(body);
  }

  // AbortController timeout: a hung request surfaces as an inline problem
  // instead of a section that silently loads forever.
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  let res: Response;
  try {
    res = await fetch(path, { method, headers, body: requestBody, signal: controller.signal });
  } catch (err) {
    if (controller.signal.aborted) {
      throw new ApiError({
        type: "about:blank",
        title: "request timed out",
        status: 0,
        detail: `${method} ${path} took longer than ${Math.round(timeoutMs / 1000)}s`,
      });
    }
    throw err;
  } finally {
    clearTimeout(timer);
  }

  if (res.status === 401) {
    notifyUnauthorized();
  }

  if (!res.ok) {
    throw await parseProblem(res);
  }

  return parseBody<T>(res);
}

/** GET path and decode the JSON (or text) body as T. */
export function apiGet<T>(path: string): Promise<T> {
  return request<T>("GET", path, undefined, GET_TIMEOUT_MS);
}

// Entry guard on every mutating call: an identical mutation (same method,
// path, and body) fired while the first is still in flight shares the
// first request's promise instead of hitting the wire twice. This is the
// client-level backstop under the per-component pending/submitting flags
// -- a double-click can no longer double-POST even if a view-layer guard
// slips.
const inflightMutations = new Map<string, Promise<unknown>>();

/**
 * Send a non-GET request (POST/PUT/PATCH/DELETE). body is JSON-encoded
 * when provided; the decoded response body is returned as T (undefined
 * for a 204).
 */
export function apiSend<T>(method: string, path: string, body?: unknown): Promise<T> {
  const key = `${method} ${path} ${body === undefined ? "" : JSON.stringify(body)}`;
  const existing = inflightMutations.get(key);
  if (existing) {
    return existing as Promise<T>;
  }
  const p = request<T>(method, path, body, SEND_TIMEOUT_MS).finally(() => {
    inflightMutations.delete(key);
  });
  inflightMutations.set(key, p);
  return p;
}

// ---- typed helper aliases over gen/types.d.ts --------------------------
//
// Page modules look these up by operationId (matches api/openapi.yaml)
// instead of threading the full paths[...]["get"]["responses"]["200"]...
// chain through every call site.
//
//   apiGet<ApiResponse<"listBlocks", 200>>(apiPath("/blocks"))
//   apiSend<ApiResponse<"createBlock", 201>>("POST", apiPath("/blocks"), body)

/** The application/json response body operation Op returns for Status. */
export type ApiResponse<
  Op extends keyof operations,
  Status extends keyof operations[Op]["responses"],
> = operations[Op]["responses"][Status] extends { content: { "application/json": infer Body } }
  ? Body
  : never;

/** The application/json request body operation Op expects, if any. */
export type ApiRequestJSON<Op extends keyof operations> = operations[Op] extends {
  requestBody?: { content: { "application/json": infer Body } };
}
  ? Body
  : never;
