// Entry for pages with no page-specific logic (the 404 page): shell only.
// Every page bundles the shared runtime plus its own thin entry -- see
// partials/ui/page-js.html.
import { initShell } from "../runtime/shell.ts";

initShell();
