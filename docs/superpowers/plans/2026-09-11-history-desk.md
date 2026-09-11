# History Desk Implementation Plan (v0.5.12)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **UI tasks (7–10) additionally REQUIRE the `impeccable` skill** — CLAUDE.md rule 4. Prose written for docs and the CHANGELOG gets cleaned with `stop-slop`.

**Goal:** Put the deletion foundations v0.5.11 built in front of the operator: remove one show's progression from the desk, clean up a date range, and see what is actually stored — each behind a dry run, a confirm that names real counts, and a guard that refuses to cut off what is on air.

**Architecture:** Two endpoints over one existing primitive and one new one. `DELETE /state/series/{show_title}` calls `store.RemoveShow` and translates its refusal into a 409 that names the blocks to edit first. Range cleanup gets a new `store.DeleteHistoryRange`, built the same way `RemoveShow` was — one transaction, an empty-ProgramID sentinel for occurrences it empties, and the instant normalisation the exact-match lookups already received. A storage readout counts rows per table so the strip reports what is there rather than what policy says should be. On the front end the TRACKED pane grows a row action, the page grows a STORAGE strip, and both deletion flows share the existing confirm dialog and the two-step framing the refusal demands.

**Tech Stack:** Go 1.2x (stdlib `testing`, `-race`), `sqlx` + `mattn/go-sqlite3`, `golang-migrate`, oapi-codegen v2 (chi-server), Hugo + hand-written CSS + Alpine.js (vendored) + TypeScript (`node --test`).

**Spec:**

- `docs/roadmap.md` — the "Then — History desk: pending" entry is this slice's scope statement.
- `docs/superpowers/specs/2026-08-30-v1-station-terminology-media-history-design.md` — §3.3 (the five invariants a deletion must respect, all now enforced inside `RemoveShow`), §3.4 (the endpoint shapes, the STORAGE strip, the unbackfillable-confirm rule), §5 Q5/Q9 (scrub the key, refuse on air).
- `TODO.md` — "Deferred (history desk foundations)". That list is this slice's brief; two of its entries are work items here, not context.
- `internal/store/removal.go` — `RemoveShow`'s doc comment is the authority on why a removal refuses and what it touches.

## Global Constraints

- **Deletion is unbackfillable, and every surface says so.** Both confirms state plainly that the data does not come back. The spec's §3.4 wording is the standard: the same honesty the empty states already carry.
- **Nothing deletes without a dry run first.** Both endpoints take `dry_run`, and the UI always runs it before offering the confirm, so the dialog names a real count rather than a guess.
- **The on-air guard applies to range cleanup.** `scheduler.OnAirOccurrences` plus `Handlers.refuseIfOnAir` already guard block deletes and dark-windows; a range whose span intersects a currently-airing occurrence is refused with the same 409 naming when it becomes safe (Q9, answered 2026-09-08).
- **Range predicates get the instant normalisation.** `TODO.md`'s deferred list: exact-match lookups were normalised in v0.5.11, ranges were not, "fix it there, with the test that is already in place". A cutoff the operator picks over data spanning a `log.timezone` change is exactly where the bytewise-prefix assumption stops holding.
- **Lean and clean (CLAUDE.md rule 1).** No deprecation aliases, no shims. Anything superseded goes in the same commit.
- **Docs in the same commit (rule 3).** README, CLAUDE.md, AGENTS.md, GEMINI.md, `docs/`, `mkdocs.yml` nav, `web/DESIGN.md`, CHANGELOG, `docs/roadmap.md`, `TODO.md`.
- **`make test` green before every commit** (rule 2); `golangci-lint run` clean before the release commit. `gosec` is broken against this toolchain and fails on a clean tree — do not treat its failure as this slice's regression and do not try to fix it here.
- **Lint limits:** cyclomatic ≤ 15, cognitive ≤ 20, nesting ≤ 5, results ≤ 3, **arguments ≤ 5**.
- **Error wrapping:** `fmt.Errorf("failed to X: %w", err)`. **Logging:** `slog`, snake_case keys.
- **Blocked packages:** `github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`. **No new dependency in this slice.**
- **Contract-first:** `api/openapi.yaml` before any handler; `make generate` and `make web-types` regenerate. Generated files are never hand-edited.
- **CSP:** `style-src 'self'` — geometry through `element.style.setProperty`, never an inline `style` attribute.

## Scope decision: this slice is the deletion desk

The roadmap's History-desk entry lists four things: bulk cursor operations, YAML import/export, the removal and range-cleanup flows, and the STORAGE strip. This plan implements the last two and leaves the first two to the slice after it.

They are different subsystems that happen to share a route. Deletion is destructive, needs a transaction, an on-air guard, a dry run and a two-step refusal flow; bulk cursor editing is a multi-select over an existing PATCH. Shipping them together would make one plan nobody can review as a unit, and it would gate the work whose foundations are already built and warm on work that has no dependency on it. The roadmap entry is amended at Task 11 to say so rather than quietly dropping half of it.

**Explicitly out of scope, and why:** bulk cursor operations and series-state YAML import/export (next slice); backfilling `show_title` for pre-`000012` rows (unrecoverable without the live Tunarr catalogue — see the deferred list); making `Engine.Commit` transactional (open since v0.3.0, and `RemoveShow` does not need it); removing an apply run or a block from the desk.

---

## File Structure

### Created

| File                                 | Responsibility                                                                 |
|--------------------------------------|--------------------------------------------------------------------------------|
| `internal/store/rangedelete.go`      | `DeleteHistoryRange` + its dry-run count, one transaction, sentinel-preserving |
| `internal/store/rangedelete_test.go` | range semantics, sentinel, normalisation, dry run                              |
| `internal/api/removal.go`            | `DELETE /state/series/{show_title}`, `DELETE /history`, storage readout        |
| `internal/api/removal_test.go`       | handler tests including the 409 shapes                                         |

### Modified

| File                              | Change                                                            |
|-----------------------------------|-------------------------------------------------------------------|
| `api/openapi.yaml`                | the two DELETEs, `GET /storage`, `RemovalReport`, `StorageReport` |
| `internal/store/sqlite.go`        | range predicates normalised; row counts per table                 |
| `internal/api/blocks.go`          | `refuseIfOnAir` generalised so a range can use it                 |
| `web/layouts/history/list.html`   | STORAGE strip, TRACKED row action, range-cleanup form             |
| `web/assets/ts/pages/history.ts`  | both flows, dry-run-then-confirm, the two-step refusal            |
| `web/assets/css/main.css`         | strip and destructive-action treatment                            |
| `web/layouts/kit/list.html`       | fixtures for the strip and both confirms                          |
| docs + CHANGELOG + roadmap + TODO | Task 11                                                           |

---

# Phase A — The server half

## Task 1: Normalise the range predicates

**Files:**

- Modify: `internal/store/sqlite.go` (the range `WHERE` clauses)
- Test: `internal/store/sqlite_test.go`

**Interfaces:** no new symbols. Every `scheduled_at`/`occurrence_start` range comparison compares normalised instants.

**Why first:** range cleanup is about to hand the operator an arbitrary cutoff over these columns. v0.5.11 fixed the exact-match sites and deliberately left the ranges, recording why in `TODO.md`: the date prefix dominates the bytewise comparison for same-zone data, and a test pins that retention prunes by instant. That assumption breaks precisely when a database spans a `log.timezone` change, which a hand-picked cutoff will find.

**The existing test to build on:** `internal/store/sqlite_test.go` already carries the instant-independence test v0.5.11 added for ranges. Read it first — it passes today for the reason above, so it is a starting point, not a guard.

- [ ] **Step 1: Find every range predicate**

```bash
grep -n "scheduled_at [<>]\|occurrence_start [<>]\|scheduled_at >=\|occurrence_start >=" internal/store/*.go
```

Expected: the nine range sites the v0.5.11 audit counted. Compare against the three exact-match sites already carrying `datetime()` so the two treatments are told apart.

- [ ] **Step 2: Write the failing test**

Extend the existing range test so its rows are written with **different offsets** for the same instants, which is what a `log.timezone` change produces:

```go
func TestScheduleHistory_RangeIsIndependentOfTheWritersOffset(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()
	ctx := context.Background()

	// The same two instants, written by processes in different zones --
	// which is what a log.timezone change leaves behind in one database.
	base := time.Date(2026, 3, 14, 23, 30, 0, 0, time.UTC)
	plus14 := time.FixedZone("UTC+14", 14*3600)
	minus11 := time.FixedZone("UTC-11", -11*3600)

	require.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		{ProgramID: "east", ChannelID: "ch", BlockName: "b", Title: "East", ShowTitle: "East",
			ScheduledAt: base.In(plus14), OccurrenceStart: base.In(plus14)},
		{ProgramID: "west", ChannelID: "ch", BlockName: "b", Title: "West", ShowTitle: "West",
			ScheduledAt: base.Add(time.Hour).In(minus11), OccurrenceStart: base.Add(time.Hour).In(minus11)},
	}))

	// A cutoff between the two instants must split them by INSTANT, not by
	// the text each writer happened to store.
	cutoff := base.Add(30 * time.Minute)
	entries, err := s.ListScheduleHistory(ctx, cutoff)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the cutoff splits by instant, not by stored text")
	assert.Equal(t, "west", entries[0].ProgramID)
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test -race ./internal/store -run RangeIsIndependentOfTheWritersOffset -v`
Expected: FAIL — both rows returned, or the wrong one, because `'2026-03-15T13:30:00+14:00' >= '2026-03-14T23:30:00Z'` compares as text.

- [ ] **Step 4: Normalise**

Apply the same treatment the exact-match sites carry — `datetime(column) >= datetime(?)` — to every range predicate found in Step 1, with a comment at the first one explaining the pair and pointing at the exact-match sites:

```go
	// datetime() on BOTH sides, for the same reason the exact-match
	// lookups carry it: a stored instant keeps its writer's offset, and a
	// bytewise comparison of two differently-offset renderings of the same
	// moment is wrong. Ranges were left alone in v0.5.11 because the date
	// prefix dominates for same-zone data; a hand-picked range-cleanup
	// cutoff over a database spanning a log.timezone change is where that
	// stops holding.
```

**Do not rewrite the stored values.** `scheduled_at` and `occurrence_start` are in primary keys, and SQLite's `datetime()` drops sub-second precision — the v0.5.11 audit rejected a data rewrite for exactly this reason. Normalise in the predicate only.

- [ ] **Step 5: Verify nothing else moved**

Run: `go test -race ./internal/store ./internal/scheduler ./internal/service`
Expected: PASS, including the retention-prunes-by-instant test the deferred note names.

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "fix(store): compare history ranges by instant, not by the writer's offset"
```

---

## Task 2: `DeleteHistoryRange`

**Files:**

- Create: `internal/store/rangedelete.go`, `internal/store/rangedelete_test.go`

**Interfaces:**

- Consumes: the normalised predicates (Task 1); `RemovalReport` (exists).
- Produces:
  - `store.HistoryRange{From, To time.Time; ChannelID, BlockName, ShowTitle string}`
  - `(*Store).CountHistoryRange(ctx, HistoryRange) (RemovalReport, error)` — the dry run
  - `(*Store).DeleteHistoryRange(ctx, HistoryRange) (RemovalReport, error)`

**The invariants it inherits from `RemoveShow`.** Read `internal/store/removal.go`'s doc comment before writing a line; this primitive respects the same rules for the same reasons:

- **One transaction.** Airings and snapshot scrubbing land together or not at all.
- **An emptied occurrence keeps its sentinel.** Deleting the last airing of an occurrence would make `GetCommittedOccurrence` answer "never planned", and the next apply would re-plan a slot that already went out — rewriting history rather than removing from it. Reuse `occurrencesLosingEveryAiring` and the empty-`ProgramID` sentinel insert; do not write a second implementation.
- **Snapshots lose keys, not rows** — but only for a title-scoped range. An untargeted range deletes airings and leaves snapshots to retention: a snapshot is per-occurrence and a date range is not a statement about any particular title in it.
- **A NULL `post_state_json` stays NULL.**

**Window forms.** `From` zero means "everything before `To`"; `To` zero means "everything after `From`"; both zero is an error, not "delete everything" — a destructive default with no operator intent behind it.

- [ ] **Step 1: Write the failing tests**

```go
func TestDeleteHistoryRange_DeletesOnlyInsideTheWindow(t *testing.T)
func TestDeleteHistoryRange_NarrowsByChannelBlockAndTitle(t *testing.T)
func TestDeleteHistoryRange_EmptiedOccurrenceKeepsItsSentinel(t *testing.T)
func TestDeleteHistoryRange_RefusesAnUnboundedWindow(t *testing.T)
func TestCountHistoryRange_CountsWithoutDeleting(t *testing.T)
func TestDeleteHistoryRange_IsInstantIndependent(t *testing.T)
```

Each asserts on a `RemovalReport` and then re-reads the store. The sentinel test is the important one: delete every airing of one occurrence, then assert `GetCommittedOccurrence` still reports the occurrence as committed with no programs.

- [ ] **Step 2: Run to verify they fail**

Run: `go test -race ./internal/store -run HistoryRange -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

`CountHistoryRange` and `DeleteHistoryRange` share one predicate builder so the dry run cannot drift from the delete:

```go
// rangeWhere builds the shared predicate. The dry run and the delete MUST
// use the same one: a preview that counts different rows than the delete
// removes is worse than no preview, because the operator confirms against
// it.
func rangeWhere(r HistoryRange) (string, []any, error)
```

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/store`
Expected: PASS.

- [ ] **Step 5: Commit**

---

## Task 3: The storage readout

**Files:**

- Modify: `internal/store/sqlite.go` (or a small `storage.go`), `api/openapi.yaml`, `internal/api/removal.go`

**Interfaces:**

- Produces: `store.StorageCounts{SeriesStates, Airings, Snapshots, ApplyRuns, Warnings int64; OldestAiring, NewestAiring *time.Time}`; `(*Store).StorageCounts(ctx)`; `GET /api/v1/storage`.

**Why it reports rows, not policy.** The strip exists so the operator can see what is actually stored. Retention says what *should* age out; a database that had retention widened, or that has been running since before a knob existed, holds whatever it holds. Reporting the configured window here would be reporting an intention as a measurement, which is the same failure the guide's draft mode refuses.

Include the oldest and newest airing instants: they are what makes the range-cleanup form pickable rather than a guess, and they cost one `MIN`/`MAX` in the same query.

- [ ] Failing test, implement, wire the endpoint, verify, commit.

---

## Task 4: `DELETE /state/series/{show_title}`

**Files:**

- Modify: `api/openapi.yaml`
- Create: `internal/api/removal.go`, `internal/api/removal_test.go`

**Interfaces:**

- Consumes: `store.RemoveShow`, `store.ShowStillScheduledError`.
- Produces: `DELETE /api/v1/state/series/{show_title}?dry_run=bool` → `200 RemovalReport`; `409` naming the blocks; `404` for an untracked title.

**The 409 is the feature, not the error path.** `RemoveShow` refuses while any block still lists the show, and its error carries `BlockNames`. That list must reach the wire as a field, not only inside a prose `detail` — the desk builds its first step out of it, and a UI that has to parse an English sentence to find the block names is a UI that breaks when the sentence is reworded.

```yaml
    RemovalRefusal:
      type: object
      required: [show_title, blocks]
      description: >-
        The 409 body when a removal is refused because blocks still list
        the show. `blocks` is what the operator has to edit first.
      properties:
        show_title: { type: string }
        blocks:
          type: array
          items: { type: string }
```

Carry it as a problem+json extension member rather than a separate body, so the existing client error path keeps working: `type`, `title`, `status`, `detail`, `request_id`, plus `blocks`.

- [ ] **Step 1: Write the failing tests**

```go
func TestRemoveShow_DryRunCountsWithoutDeleting(t *testing.T)
func TestRemoveShow_RemovesAndReportsPerTable(t *testing.T)
func TestRemoveShow_409NamesTheBlocksToEditFirst(t *testing.T)  // asserts on the `blocks` array, not the prose
func TestRemoveShow_UnknownTitleIs404(t *testing.T)
func TestRemoveShow_RefusesWhileTheShowIsOnAir(t *testing.T)
```

- [ ] Steps 2–6: run, implement, verify, commit — following Tasks 1–2's shape.

---

## Task 5: `DELETE /history`

**Files:** `api/openapi.yaml`, `internal/api/removal.go`, `internal/api/removal_test.go`, `internal/api/blocks.go`

**Interfaces:**

- Consumes: `store.DeleteHistoryRange`/`CountHistoryRange` (Task 2), the generalised on-air guard.
- Produces:

```text
DELETE /history?before=<RFC3339>[&channel_id=&block_name=&show_title=][&dry_run=true]
DELETE /history?from=<RFC3339>&to=<RFC3339>[&…][&dry_run=true]
  200 → RemovalReport
  400 → both or neither window form, or from > to
  409 → the range intersects a currently-on-air occurrence
```

**Generalising the guard.** `Handlers.refuseIfOnAir` takes a `store.BlockRecord` today. A range is not one block, so extract the predicate half — "which of these blocks is on air, and when is it safe" — and let both callers use it. Do not duplicate the envelope logic; `scheduler.OnAirOccurrences`'s doc comment is explicit that the wider of the two envelopes is deliberate, and a second copy would drift.

For a range, the blocks to check are those with an occurrence inside the window; refuse naming the block and the instant it becomes safe, exactly as the block-delete guard does.

- [ ] Failing tests including both window forms, the mutual exclusion, `from > to`, and the on-air refusal; implement; verify; commit.

---

## Task 6: Phase A gate

- [ ] `golangci-lint run` clean; `make test` green; `make validate` clean.
- [ ] Drive both endpoints against the real binary with a seeded database: a dry run reports counts and changes nothing; the delete reports the same counts; a second delete reports zeros.
- [ ] Prove the refusal: create a block listing a show, try to remove it, confirm the 409 carries the block name in `blocks`.
- [ ] Prove the sentinel: delete every airing of one occurrence by range, then confirm a re-apply does not re-plan that slot.

---

# Phase B — The desk

> Every task invokes the `impeccable` skill first. The design contract is `web/DESIGN.md`; reuse the shared partials (`confirm`, `problem`, `empty`, `skeleton`, `tape`, `plate`) and the runtime modules. Do not re-implement them.

## Task 7: The STORAGE strip

**Files:** `web/layouts/history/list.html`, `web/assets/ts/pages/history.ts`, `web/assets/css/main.css`, `web/layouts/kit/list.html`

Row counts per table, the oldest and newest airing, and the effective retention window beside them — measurement first, policy second, visibly distinguished. It sits above the band selector because it describes the whole record rather than one pane.

- [ ] Contrast computed for any new pairing in both palettes and recorded in `web/DESIGN.md`'s evidence table. `/kit/` fixtures for loaded, loading, error and empty-database states.

## Task 8: Remove from TRACKED, including the first step

**Files:** `web/assets/ts/pages/history.ts`, `web/layouts/history/list.html`

A row action that runs the dry run, then opens the shared confirm naming the exact counts and saying plainly that the data does not come back.

**The two-step flow is the point.** `TODO.md`'s deferred list: "The desk UI should surface that first step rather than letting them hit the 409 cold." Before offering removal, the page already knows which blocks list the show — it can ask `GET /blocks` — so a row whose show is still scheduled shows the removal as blocked, names the blocks, and links to each one's editor. The 409 stays as the server's own guard, but the operator should not meet it by surprise.

- [ ] Failing tests for the pure predicate (given blocks and a show, which block names list it), implement, verify, commit.

## Task 9: Range cleanup

**Files:** `web/layouts/history/list.html`, `web/assets/ts/pages/history.ts`

A form in the AS-RUN pane's toolbar region: the two window forms as one control pair defaulted from the storage readout's oldest and newest, the optional narrowing fields prefilled from whatever filters the pane already has, a dry run, and the shared confirm. A 409 renders the on-air refusal with the instant it becomes safe.

- [ ] Failing tests for the window-form validation (exactly one form, `from <= to`), implement, verify, commit.

## Task 10: Phase B gate

- [ ] `make web` clean; `npm test --prefix web` green.
- [ ] Drive the real binary in a browser: dry run, confirm, tape line, the strip updating afterwards, the blocked-removal row, the on-air refusal.
- [ ] Batched screenshot round — desktop and mobile, light and dark.
- [ ] Run the mechanical detector once over the changed UI files.

---

# Phase C

## Task 11: Documentation and release

- [ ] `docs/api-reference.md`: both DELETEs, `GET /storage`, the 409 shapes including the `blocks` extension member.
- [ ] `docs/web-ui-guide.md`: the strip, both flows, the two-step removal, what a refusal means.
- [ ] `docs/scheduling-concepts.md`: what a removal touches and what it deliberately leaves — the sentinel above all, since "the slot stays committed and empty" is surprising until it is explained.
- [ ] `docs/deployment.md`: deletion is unbackfillable; retention still does the routine work.
- [ ] `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`: the new files in the architecture tree.
- [ ] `docs/roadmap.md`: convert the History-desk entry into a shipped one **and amend it** to say bulk cursor operations and YAML import/export moved to the following slice, with the reason from the Scope decision above.
- [ ] `TODO.md`: close the two deferred entries this slice resolves (range normalisation, no endpoint calls `RemoveShow`); open a "Deferred (history desk)" section.
- [ ] `CHANGELOG.md`: new version heading plus the compare link at the foot.
- [ ] `stop-slop` over every paragraph; `markdownlint-cli2` shows no new findings against the baseline.
- [ ] Hand the operator paste text for the cluster wiki page and the Obsidian note.

---

## Self-Review

**Scope coverage.**

| Requirement                                             | Task                                                |
|---------------------------------------------------------|-----------------------------------------------------|
| Remove-from-history flow over `RemoveShow`              | 4, 8                                                |
| Range cleanup                                           | 2, 5, 9                                             |
| STORAGE strip                                           | 3, 7                                                |
| Range predicates normalised (deferred item)             | 1                                                   |
| An endpoint calls `RemoveShow` (deferred item)          | 4                                                   |
| Two-step refusal surfaced, not met cold (deferred item) | 8                                                   |
| On-air refusal on range delete (Q9)                     | 5                                                   |
| Dry run before every destructive confirm                | 2, 4, 5, 8, 9                                       |
| Unbackfillable stated on every confirm                  | 8, 9, 11                                            |
| Bulk cursor operations, YAML import/export              | **deferred to the next slice — see Scope decision** |

**Placeholders:** Tasks 3, 4, 5 and 7–9 carry their interfaces, invariants and test names but compress the step list, because each follows the failing-test → implement → verify → commit shape spelled out in full in Tasks 1 and 2. What they do not repeat is boilerplate; what they do carry is the reasoning specific to them. Expand a task in place if it proves too thin during execution rather than improvising past it.

**Type consistency:** `RemovalReport` is the one report shape — returned by `RemoveShow` (exists), `CountHistoryRange`, `DeleteHistoryRange`, and both endpoints. `HistoryRange` is built once in Task 2 and parsed from the query string in Task 5. The on-air predicate extracted in Task 5 is the same one `refuseIfOnAir` already uses, not a second copy.

**Risk to watch:** Task 1 touches nine predicates across queries that other slices depend on. Its own test proves the fix; the surrounding suites prove nothing else moved. Run them, do not assume.
