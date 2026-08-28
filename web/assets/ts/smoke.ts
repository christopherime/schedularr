// Smoke test proving generated types load under strict TS. Task 3 replaces
// this file with the real API client (web/assets/ts/api.ts).
import type { components } from "./gen/types";

export type BlockRecord = components["schemas"]["BlockRecord"];
