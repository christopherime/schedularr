// Cron readback over the vendored cronstrue UMD global -- shared by the
// blocks editor's schedule picker and the guide inspector's cron line.
// Pages that import this must load cronstrue first (ui/page-js's
// `cronstrue: true` arg carries the ordering constraint).

declare const cronstrue: {
  toString(expr: string, options?: { throwExceptionOnParseError?: boolean }): string;
};

const CRONSTRUE_ERROR_PREFIX = "An error occurred";

/** Plain-language readback for a cron string, or null for blank/
 * unparseable input (never thrown out of a template expression -- Alpine
 * evaluates x-text/x-show inline, so a thrown error here would break the
 * whole panel's render, not just this one field). */
export function cronReadback(raw: string): string | null {
  const trimmed = raw.trim();
  if (trimmed === "") return null;
  let text: string;
  try {
    text = cronstrue.toString(trimmed, { throwExceptionOnParseError: false });
  } catch {
    return null;
  }
  return text.startsWith(CRONSTRUE_ERROR_PREFIX) ? null : text;
}
