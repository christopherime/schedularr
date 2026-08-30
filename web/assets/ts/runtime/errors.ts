// Shared error rendering -- the one place a thrown value becomes operator-
// facing text. Every consumer binds the result via x-text (textContent),
// never innerHTML: problem+json detail strings come from the API and are
// never trusted as markup.
import { ApiError } from "./api.ts";

/**
 * The structured shape the `ui/problem` partial renders: a detail line
 * plus the muted `REF <request_id>` correlation line when the API sent
 * one. Section-level error state on every page holds one of these (or
 * null) instead of a bare string.
 */
export interface ProblemView {
  /** The API's own problem title ("tunarr unreachable", ...). */
  title: string;
  detail: string | null;
  /** request_id from the problem+json body, for server-log correlation. */
  requestId: string | null;
}

/** Structures any thrown value for the problem partial. */
export function toProblemView(err: unknown): ProblemView {
  if (err instanceof ApiError) {
    return { title: err.title, detail: err.detail ?? null, requestId: err.request_id ?? null };
  }
  return {
    title: "Request failed",
    detail: err instanceof Error ? err.message : String(err),
    requestId: null,
  };
}

/**
 * One-line rendering for row-scoped/inline errors ("<title>: <detail>",
 * or just the title): the compact form used where a full problem panel
 * would drown a table row.
 */
export function describeError(err: unknown): string {
  const p = toProblemView(err);
  return p.detail ? `${p.title}: ${p.detail}` : p.title;
}

/** The problem partial's own one-liner ("<title>: <detail>") for a
 * ProblemView that is already structured. */
export function problemLine(p: ProblemView): string {
  return p.detail ? `${p.title}: ${p.detail}` : p.title;
}
