// Typed fetch wrapper for /api/v1. This is the ONLY module that is allowed
// to call fetch() against the Schedularr API -- page modules (Tasks 4-7)
// go through apiGet/apiSend so token injection, problem+json parsing, and
// 401 handling stay in one place (see PRODUCT.md, "Contract-first" and
// "No silent failures").
import type { operations } from "./gen/types";
import { getToken } from "./token";

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

type UnauthorizedCallback = () => void;
const unauthorizedCallbacks: UnauthorizedCallback[] = [];

/**
 * Registers a callback fired every time a request comes back 401. The
 * shell (web/assets/ts/main.ts) registers one that opens the token panel;
 * callers never retry automatically -- the brief is explicit that a 401
 * does not auto-retry the failed action.
 */
export function onUnauthorized(cb: UnauthorizedCallback): void {
  unauthorizedCallbacks.push(cb);
}

function notifyUnauthorized(): void {
  for (const cb of unauthorizedCallbacks) cb();
}

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

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
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

  const res = await fetch(path, { method, headers, body: requestBody });

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
  return request<T>("GET", path);
}

/**
 * Send a non-GET request (POST/PUT/PATCH/DELETE). body is JSON-encoded
 * when provided; the decoded response body is returned as T (undefined
 * for a 204).
 */
export function apiSend<T>(method: string, path: string, body?: unknown): Promise<T> {
  return request<T>(method, path, body);
}

// ---- typed helper aliases over gen/types.d.ts --------------------------
//
// Page modules look these up by operationId (matches api/openapi.yaml)
// instead of threading the full paths[...]["get"]["responses"]["200"]...
// chain through every call site.
//
//   apiGet<ApiResponse<"listBlocks", 200>>("/api/v1/blocks")
//   apiSend<ApiResponse<"createBlock", 201>>("POST", "/api/v1/blocks", body)

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
