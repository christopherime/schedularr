# Changelog

All notable changes to Schedularr will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.3] - 2026-08-30

One live production bug, found through real use of v0.2.2's deployed
reorder flow -- not a review pass.

### Fixed

- **Reordering a block through `PUT /blocks/{id}` made the pending
  (not-yet-aired) occurrence SKIP episodes.** Observed on the cluster:
  tonight's occurrence was committed with each show's E2 (cursors
  legitimately advanced to E3 at plan time); the PUT then invalidated
  the block's occurrence snapshots -- a round-2-era step aimed at
  operator cursor edits -- deleting the occurrence's SEED, so the
  seedless re-derive fell back to the LIVE cursor (E3) and re-planned
  tonight with the E3s: the committed E2s would never have aired.
  Engine-level reorder tests kept passing because they mutate the spec
  directly with snapshots intact; the handler wiring defeated the
  seed-preserving semantics the snapshot design exists to provide. Under
  the final architecture the invalidation was redundant AND harmful:
  spec edits already take effect through re-derive-from-seed + CURRENT
  spec ("same episodes, new arrangement"), and invalidation is only
  correct where the seed ITSELF must be overridden -- operator cursor
  writes (`PATCH /state/series`, CLI `state set`/`reset`/`import`) --
  plus `DeleteBlock`'s orphan cleanup, both unchanged. `UpdateBlock`
  (`internal/api/blocks.go`) no longer touches snapshots at all. Edge
  cases ruled through rather than papered over: a spec edit REMOVING a
  series leaves a harmless unused seed entry (the re-derive simply
  doesn't plan it, and the baseline-agreement guard keeps its eventual
  post-state replay from touching live state); a `channel_id` change
  leaves the seed valid (occurrence identity is block + start); a spec
  edit ADDING a series re-derives that show from the seed path's
  deterministic S01E01 default. Caveat: this differs from an unseeded
  occurrence (which plans a new show from its persisted cursor), so a
  show with prior progress added to a block with a pending seeded
  occurrence re-airs S01E01 once in that occurrence; live state is
  protected by the missing-baseline guard. Second caveat: the
  seed-preserving guarantee covers series list/order and
  episodes_per_block/duration edits -- NOT cron. A cron change MOVES
  occurrences to new start keys that have no seed, so an occurrence
  already committed for the old start re-plans from the live cursor
  and its committed round is skipped; follow an unavoidable cron
  change with a compensating `PATCH /state/series` rewind. No narrow
  re-seed path was added for either case. New
  regression tests at the previously-untested handler+engine layer:
  `TestUpdateBlock_Reorder_PendingOccurrenceKeepsSameEpisodes` (real PUT
  handler, real store, real engine: commit [alpha-e2, beta-e2], PUT the
  reorder, re-apply -> the SAME occurrence re-plans [beta-e2, alpha-e2]
  with cursors unchanged; probed red against the reverted handler,
  which reproduces the exact live failure [beta-e3, alpha-e3]) and
  `TestPatchSeriesState_OverridesSeed_PendingOccurrenceRederivesFromNewCursor`
  (the companion pin: cursor edits still override seeds);
  `TestUpdateBlock_PreservesOccurrenceSnapshots` replaces the three
  retired invalidation-era Update tests. `docs/scheduling-concepts.md`
  now states the distinction outright: spec edits are seed-preserving
  (same episodes), cursor edits are seed-overriding.

## [0.2.2] - 2026-08-30

A critical bug plus two smaller ones found during live multi-block
testing against a real Tunarr 1.3.13 instance.

### Fixed

- **Apply is now idempotent per occurrence, instead of re-advancing
  series cursors on every re-apply that still covered it.** Live
  evidence: a Saturday-night block was planned with E01 on its first
  apply, E04 after two more; a show's `series_state` drifted to S2E3
  after a day of test applies with nothing having actually aired that
  far. Root cause: the 24h apply window and the default 6h cron interval
  mean the same future occurrence falls inside several consecutive
  applies' windows, and every one of them re-ran `PlanBlock` from
  scratch, each time advancing the series cursor and writing a fresh
  `schedule_history` row for content that was never going to air
  differently, since the next apply would just overwrite it anyway.
  Conflict-dropped occurrences had the same problem one level worse:
  `PlanBlock` ran (and advanced state) for every planned occurrence
  *before* conflict resolution ever discarded the losing ones.
  Fixed with per-`(block_name, occurrence_start)` idempotence, split by
  block type:
  - `internal/scheduler/engine.go`'s `GenerateForTimeRange` now runs in
    three phases -- build occurrence "shells" (time only, no content),
    resolve conflicts on the shells, *then* plan content only for
    occurrences that survived -- so a conflict-dropped occurrence never
    reaches `PlanBlock` and can't advance anything.
  - Filter blocks: an already-committed occurrence (aired or not) is
    replayed verbatim from `schedule_history` -- there's no
    spec-derived cursor to re-derive for a random pick, so freezing the
    choice once made is the only idempotent option.
  - Series blocks needed more: users can (and do) edit a block's
    `series` order, add/remove a series, or change
    `episodes_per_block`/`duration` before an occurrence airs, and
    expect that to take effect -- freezing the final program list like
    a filter block would silently ignore such an edit. So a series
    occurrence's per-show cursor *snapshot* (season/episode/completed/
    disabled/run_count, captured immutably the first time it's ever
    planned, in the new `series_occurrence_snapshots` table) is stored
    separately from its assignment: a still-future occurrence is
    re-derived every apply from that fixed snapshot plus the *current*
    block spec (same snapshot + unchanged spec = idempotent; a spec
    edit changes the result); once an occurrence's start time has
    passed, it's replayed verbatim like a filter block, forever.
  - `schedule_history` gained `occurrence_start`, `sequence`,
    `duration_ms`, `title`, and `type` columns (migration `000003`) so
    an occurrence's assignment can be looked up and fully reconstructed
    by `(block_name, occurrence_start)` instead of only supporting the
    existing recency-dedup query shape; `series_occurrence_snapshots`
    (migration `000004`) is new.
  - Also fixed a related `RunCount` inflation bug surfaced by testing
    this: `markSeriesCompleted` used to re-run its bookkeeping (and
    increment `run_count` again) every time a completed series was
    re-examined within one wide apply window, not just once per actual
    completion.
  - Regression tests: two consecutive `Runner.Run(Apply: true)` calls
    over the same window now produce byte-identical plans and leave
    series state unchanged; a conflict-dropped series occurrence is
    proven to never even create a `series_state` row; and the exact
    reorder-before-airing scenario (commit `[A, B, C]`, reorder the spec
    to `[C, A, B]`, re-apply -- same occurrence, new order, same episode
    per show, unchanged series state) is covered directly.
- **Conflict resolution is no longer invisible to API callers.**
  Overlapping occurrences resolving by priority (or, tied, first-come)
  used to be visible only in a server-side INFO log line -- `POST
  /generate` and `POST /apply` responses carried nothing. `PlanResult`
  gained an optional `warnings` array (`api/openapi.yaml`, both
  codegens regenerated) listing each dropped occurrence's block name,
  occurrence time, and the block it lost to; omitted entirely (not an
  empty array) when there's nothing to report. The
  [Web UI's Schedule page](docs/web-ui-guide.md#schedule-schedule) now
  renders a warning panel above the channel list after every preview or
  apply when the response carries any.
- **A live Tunarr endpoint returning a fractional-millisecond channel
  duration (observed: `691200000.9999`) no longer triggers a resty WARN
  retry loop.** `tunarr.Channel.Duration` was `int64`, which failed to
  decode that response at all; `internal/httpclient.Client`'s retry
  condition fires on any non-nil error regardless of HTTP status, so
  every such decode failure cost up to `MaxRetries` (3) extra live
  requests against Tunarr for a response shape that could never decode
  differently. Widened to `float64`, matching `tunarr.Program.Duration`'s
  existing "float from Tunarr API" convention -- the field is otherwise
  unused by any caller in this repo, so this has no other effect.
  Regression test asserts both the decode and that the server is hit
  exactly once (no retry).
- **Eight release-review findings against the idempotent-apply work
  above, all engine-side.** All in `internal/scheduler/engine.go` unless
  noted:
  - **A not-yet-aired occurrence's re-derive could silently re-pin a
    series back to `start_episode`/`start_season` on every single
    apply, not just the first.** A reconstructed `SeriesStateSnapshot`
    always left `LastAired` nil, which `initializeSeriesState` reads as
    "never initialized" -- so a block configured with `start_episode: 5`
    that had legitimately progressed to E6 got silently re-derived back
    to E5 on the next apply. Fixed with a `Seeded bool` field on
    `SeriesStateSnapshot` (`internal/scheduler/state.go`) recording
    whether the cursor was already initialized at capture time;
    `statesFromSnapshot` now reconstructs a non-nil marker `LastAired`
    exactly when `Seeded` is true.
  - **Reordering (or otherwise editing) a series block's spec before
    two-or-more not-yet-aired occurrences aired could air the same
    episode twice and silently skip another show entirely.** Each
    occurrence's re-derive only ever read its OWN stored snapshot, so
    when occurrence 1 was re-derived, nothing updated occurrence 2's
    snapshot to match occurrence 1's actual new end state -- occurrence
    2 kept re-deriving against a stale baseline. New
    `planSeriesOccurrences` plans a whole block's surviving occurrences
    in one chained pass (a running `map[string]*SeriesState` threaded
    across all of them, only ever the first occurrence in the batch
    touching real persisted state), rewriting later occurrences'
    snapshots as it goes; `GenerateForTimeRange`'s phase 3 now calls it
    once per series block instead of looping `PlanBlock` per occurrence
    (which only chained a single occurrence at a time).
  - **`PATCH /state/series/{show_title}` (an operator's manual cursor
    reset) was silently shadowed for up to the schedule-generation
    window.** Once an occurrence has a snapshot, its planning never
    re-reads `series_state` again. `PatchSeriesState`
    (`internal/api/state.go`), and `UpdateBlock`/`DeleteBlock`
    (`internal/api/blocks.go`), now delete every not-yet-aired
    occurrence snapshot for the affected block(s) via a new
    `StateStore.DeleteFutureOccurrenceSnapshots`, so the next apply
    re-derives them against the just-changed state/spec instead of
    keeping a stale snapshot until it ages out on its own.
  - **`series_occurrence_snapshots` had no retention/GC, and was keyed
    by the renameable `block_name`.** Migration `000005` re-keys the
    table by `block_id` (`scheduler.Block` gained a `Block.ID` field,
    populated by `service.ActiveBlocks` from the store record) and adds
    a `recorded_at` column; new `StateStore.CleanupOccurrenceSnapshots`
    (called from `Engine.Commit`, mirroring
    `CleanupScheduleHistory`'s window) prunes by `recorded_at` -- a real
    wall-clock write timestamp, like `schedule_history.scheduled_at` --
    not `occurrence_start`, which would otherwise prune a legitimately
    fresh snapshot whenever the schedule-generation window itself
    doesn't start at "now". `SaveOccurrenceSnapshot` is now an upsert
    (needed by the chain fix above, which rewrites existing snapshots).
    Also removed the dead "shouldn't happen" fallback that used to
    re-derive an aired occurrence from the current spec when its
    committed history had been pruned -- an aired occurrence's content
    is historical fact a later apply must never invent; it now correctly
    returns nothing to replay instead.
  - **Re-deriving a not-yet-aired series occurrence with filler/fallback
    content enabled reshuffled it differently on every apply**, via the
    global, unseeded `math/rand` source -- so a filler-enabled series
    block wasn't apply-idempotent even though its episode selection was.
    New `occurrenceRand(blockID, occurrenceStart)` deterministically
    seeds a `*rand.Rand` (FNV-1a hash of both) threaded through
    `applySeriesFallback`/`applyBlockFiller`/`getFiller`, so re-deriving
    the same occurrence reproduces the exact same filler selection and
    order.
  - **`resolveConflicts` could leave two occurrences overlapping in its
    own output.** It stopped scanning a winning slot against the rest of
    `resolved` the moment it evicted the FIRST lower-priority slot it
    overlapped, so a slot overlapping three-or-more already-resolved
    slots only ever evicted one of them (silently dropped from the
    `warnings` returned too) -- violating `buildAnchoredLineup`'s
    sorted-non-overlap precondition and risking a negative gap /
    wall-clock drift. Now restarts the same index after an eviction
    instead of breaking out, so a winner is checked against every
    overlapping resolved slot, not just the first.
  - **Every apply anchored a channel's pushed lineup at "now," cutting
    off whatever occurrence was currently on air mid-episode.**
    `GenerateForTimeRange`'s phase 1 now also injects an "on-air" shell
    per block, if one exists (`onAirOccurrenceStart`: an occurrence
    whose `[start, start+duration)` window contains `start` itself,
    even though its own start is before the generation window) -- never
    re-planned once it has a snapshot, content always a verbatim replay.
    `service.anchorForChannel` (`internal/service/schedule.go`) shifts a
    channel's own lineup anchor to that occurrence's original
    `StartTime` instead of the Run's global `start` whenever one exists,
    which is what lets Tunarr's wall-clock playback formula resolve to
    a position partway through the episode instead of restarting it.
  - Regression tests: the reorder scenario extended to two occurrences
    (catches the chain bug directly -- a duplicate episode airing
    twice); a two-apply byte-identical check with series filler fallback
    enabled; a three-way mutual-overlap conflict test asserting no
    residual overlap survives; an on-air-apply test asserting the
    running occurrence stays in the lineup at its original `StartTime`
    across a later re-apply; a `start_episode` re-pin regression; and
    new `internal/store` SQL-level coverage for
    `GetOccurrenceSnapshot`/`SaveOccurrenceSnapshot`'s upsert and
    `block_id` keying, `DeleteFutureOccurrenceSnapshots`, and
    `CleanupOccurrenceSnapshots`' `recorded_at`-based pruning (none of
    which had a dedicated SQL-level test before this round).
- **A second release-review pass on the fixes directly above found 8
  more issues, introduced by that fix architecture itself.** All in
  `internal/scheduler/engine.go` unless noted:
  - **Chain-mode planning (the scratch `snapshotSeriesContext` path)
    never wrote `e.pendingStates`, so `series_state` froze at whatever
    the very first real plan produced, no matter how many further
    occurrences actually aired afterward** (probe: cursor stuck at
    S1E2 while e1..e4 had genuinely aired). Compounded with a block
    edit's snapshot invalidation, this could re-air an already-aired
    episode once its stale cursor was next used for a real plan. Fixed:
    `planSeriesOccurrences` now syncs `e.pendingStates` (cloned, never
    aliased) from the chain every time it processes an aired/on-air
    occurrence, since that occurrence's content is settled historical
    fact by then. Caught (via CI, wall-clock-dependent) and fixed a
    second issue in the same mechanism during this same round: the
    chain-advance that runs for an aired occurrence always stamps
    `LastAired` with a fresh `time.Now()`, so a naive sync bumped it on
    every idempotent re-apply of an already-settled occurrence, not just
    the first time. `syncPendingStatesFromChain` now preserves the
    already-persisted `LastAired` instead, only adopting the chain's
    value the very first time a show is synced at all.
  - **`establishSeriesChain`'s "no snapshot" branch assumed that meant
    "never planned before," which `DeleteFutureOccurrenceSnapshots`
    breaks: it deletes only the snapshot, never the paired
    `schedule_history` rows.** Re-deriving after such a wipe appended a
    second, overlapping set of history rows on top of the first instead
    of replacing them (probe: `GetCommittedOccurrence` returned two
    generations mixed together for the same occurrence), and -- for an
    on-air occurrence specifically -- silently replaced already-aired
    content with something freshly (and differently) picked, reintroducing
    the exact mid-episode-cutoff bug fixed earlier in this same release.
    Fixed by unifying both cases into one rule: an occurrence with
    existing committed history is never real-planned again, aired or
    not -- an aired one always replays verbatim via
    `airedSeriesOccurrenceContent`; a not-yet-aired one re-derives from
    the (now kept-accurate, see above) live `series_state` and replaces
    its stored assignment via `ReplaceOccurrenceHistory`, never appends.
  - **`resolveConflicts` could leave an earlier eviction committed even
    after its evictor itself lost to a still-higher-priority slot**
    (probe: A/C don't overlap, both overlap B; B evicts A then loses to
    C -- A stayed dropped despite never conflicting with the real
    survivor C, and its `Warning` blamed B, which was never even
    scheduled). Rewritten entirely: slots are now processed
    highest-priority-first and only ever greedily kept if they overlap
    nothing already kept -- once kept, a slot can never be beaten by
    anything considered afterward, so the "evict then lose" case can't
    happen at all.
  - **`api/blocks.go`'s `UpdateBlock`/`DeleteBlock` (and, for
    consistency, `api/state.go`'s `PatchSeriesState`) turned a failed
    post-mutation snapshot invalidation into a `500` for a write that
    had already committed successfully.** Now logged and NOT surfaced
    as a response error -- the primary mutation is authoritative by
    then, and there is no compensating client action anyway (the call
    is naturally idempotent, so a retry repeats it for free).
  - **An on-air occurrence whose committed history has been pruned (or
    predates the `occurrence_start` column) resolves to zero programs,
    but was still used to anchor the channel** -- pointless (there is no
    real content whose mid-episode position needs to keep making sense)
    and produced two directly-adjacent `"flex"` lineup entries with no
    content between them. `service.anchorForChannel`
    (`internal/service/schedule.go`) now skips a zero-program slot when
    choosing the anchor; `buildAnchoredLineup`'s new `appendFlex` helper
    merges directly-adjacent flex entries into one wherever they occur.
  - Regression tests: a three-apply cursor-advances-as-occurrences-age
    check; a snapshot-wiped-but-history-remains no-double-history check;
    a post-migration on-air-with-no-snapshot verbatim-replay check; a
    three-slot evict-then-lose priority conflict check; handler-level
    coverage for `PATCH /state/series`, `PUT /blocks/{id}`, and
    `DELETE /blocks/{id}` actually invalidating future occurrence
    snapshots (previously only the SQL primitive had a test); and a
    zero-program on-air anchor/flex-merging check.
- **A third release-review pass found the fix above (chain-mode
  planning syncing `e.pendingStates`) itself broken three ways -- all
  new regressions in that exact mechanism.** Root cause, shared across
  all three: the aired-occurrence chain advance was a fresh re-plan
  against the *current* spec, whose result then got persisted. Fixed by
  replacing the re-plan with a pure derivation from what actually aired:
  - **Re-applying during a single on-air occurrence, repeatedly, walked
    the cursor forward on every apply (E2 -> E5 across three applies)
    and churned the NEXT occurrence's content each time.** Case-2 chain
    seeding (`establishSeriesChain`, no snapshot but committed history
    survives -- migration `000005`'s `DROP TABLE` on deploy day, or any
    later apply of the same on-air occurrence, since the aired branch
    never re-snapshots itself) already reflects that occurrence's own
    prior consumption; re-deriving it via a scratch re-plan and
    persisting the result advanced it a second (and third...) time on
    top of that. New `scheduler.advanceStateFromCommittedContent`
    derives each show's cursor purely from the occurrence's own frozen
    committed content and its `occurrenceStart` (never a re-plan), and
    is *advance-only*: a candidate is applied only if it's genuinely
    ahead of the current cursor, so re-deriving the same frozen content
    twice is a no-op and the cursor (and every chained-forward
    occurrence) stabilizes after the first apply.
  - **An operator's `PATCH /state/series` (cursor jump + `disabled`)
    issued while a show's occurrence was on air got silently reverted by
    the very next apply.** Root cause was two-fold: (1) syncing copied
    the WHOLE chain-derived `SeriesState`, including `Completed`/
    `Disabled` -- fields an aired occurrence's content can never
    legitimately imply anything about; (2)
    `DeleteFutureOccurrenceSnapshots`' plain `occurrence_start > now`
    cutoff never reaches an occurrence that's *currently* on air (its
    `occurrence_start` is already in the past), so its stale pre-PATCH
    snapshot survived and kept re-deriving from the old baseline. Fixed:
    `syncPendingStatesFromChain` now writes only `CurrentSeason`/
    `CurrentEpisode`/`LastAired`, and only for shows
    `advanceStateFromCommittedContent` actually advanced -- never
    `Completed`/`Disabled`/`RunCount`; new `store.InvalidationCutoff`
    widens the cutoff by the block's own duration (`now - duration`,
    i.e. "not yet *finished*," not just "not yet started"), used by
    `PatchSeriesState`, `UpdateBlock`/`DeleteBlock`, and the CLI's
    `state reset`/`state set` (via new `store.InvalidateSeriesOccurrenceSnapshots`,
    now the single shared implementation all four call). Combined with
    the advance-only rule above, a patched cursor that's ahead of what
    an on-air occurrence's frozen content implies is never clobbered
    back down.
  - **Editing a block's `episodes_per_block` (e.g. 1 -> 4) after an
    occurrence had already aired retroactively changed how far its
    cursor advanced** (E2 -> E5, even though only one episode had
    actually aired), because the chain advance re-planned against the
    *current* (edited) spec instead of the occurrence's own frozen
    content. Fixed by the same content-only derivation above: a spec
    edit can no longer affect an already-aired occurrence's contribution
    to the cursor at all.
  - **The `Seeded`-snapshot marker (round 2 finding 1's fix, a fixed
    unix-epoch sentinel meaning "already initialized") could leak into
    persisted `series_state.last_aired` as `1970-01-01`.** Structurally
    impossible now: `syncPendingStatesFromChain` only ever reads
    `LastAired` for shows `advanceStateFromCommittedContent` just
    advanced, and that function always stamps a fresh, real value
    (`occurrenceStart`) in the same step -- the sentinel is never in the
    value it reads.
  - **`last_aired` stayed frozen in steady state** (cursor kept
    advancing E2 -> E5 while `last_aired` never moved, going stale in
    `GET /state/series`) -- now stamped from the occurrence's own
    `occurrenceStart` every time a show is genuinely advanced:
    deterministic (same occurrence always stamps the same value) and
    keeps moving forward as later occurrences age into aired.
  - `TestRunner_Run_Apply_IsIdempotentPerOccurrence`
    (`internal/service/schedule_test.go`) now pins `Runner.now` (new
    field, defaults to `time.Now`) to a fixed instant inside the test
    block's on-air window, so it deterministically exercises the
    on-air code path on every run instead of only when the real wall
    clock happened to land there (~half the time before this).
  - Regression tests: direct unit tests for
    `advanceStateFromCommittedContent`'s determinism, advance-only rule,
    and Seeded-marker non-leak; end-to-end triple-reapply-during-on-air
    (stable cursor + stable next-occurrence content), PATCH-during-on-air
    (cursor and `disabled` both survive), and spec-edit-after-aired
    checks; handler-level coverage for the widened on-air invalidation
    cutoff and for the log-and-continue fix actually firing (a new
    `corruptOccurrenceSnapshotsTable` test helper drops just that one
    table so only the invalidation step fails, not the primary
    mutation); and store-level coverage for `InvalidationCutoff`/
    `InvalidateSeriesOccurrenceSnapshots` directly.
- **A fourth review pass found the content-derived cursor advance above
  structurally unsound, and it has been replaced wholesale: a series
  occurrence's effect on the persisted cursor is now decided exactly
  once, at plan time, stored, and replayed -- never re-derived from
  committed content.** `advanceStateFromCommittedContent` and
  `syncPendingStatesFromChain` (both introduced by the previous pass)
  are deleted; the mechanism descriptions in that pass's entry are
  superseded by this one. Deriving from content's `max(season, episode)`
  was a broken foundation: `schedule_history` rows carry no
  season/episode metadata at all (only ID/title/duration/type), and
  content fundamentally cannot represent a plan whose cursor moved
  backward. Migration `000006` adds `post_state_json` to
  `series_occurrence_snapshots` (the per-show cursor as of the END of
  the occurrence's plan, captured alongside the existing pre-plan seed)
  and `operator_updated_at` to `series_state` (stamped by
  `PATCH /state/series` and the CLI's `state set`/`state reset`; never
  by engine writes). The aired branch (`internal/scheduler/engine.go`)
  now replays each aired occurrence's stored post-state -- into the
  planning chain unconditionally, and into persisted `series_state`
  through two guards in `Engine.syncPostStates`: *operator wins* (a show
  whose operator stamp is newer than the occurrence's commit
  (`recorded_at`) is skipped outright -- the backstop that makes an
  operator's BACKWARD cursor jump stick even when snapshot invalidation
  failed or raced an in-flight apply) and *monotonic* (the live cursor
  is only ever moved strictly forward, so a slower block's stale on-air
  replay can never drag back a cursor another block already advanced).
  Specific bugs this closes, each now a permanent regression test:
  - **`on_complete: restart` completing mid-occurrence** (content
    `[E1..E3]`, correct post-state S01E01) used to persist
    `max(content)+1` = S01E04, leaving the next occurrence permanently
    empty once committed -- the stored post-state is the only
    representation that can carry "the cursor wrapped."
  - **Two blocks scheduling the same show**: re-applying the slower
    block's on-air occurrence dragged the shared live cursor backward
    (E7 -> E3, re-airing E3-E6); the advance-only comparison ran against
    the block's own stale chain baseline, not the live value.
  - **An operator's backward jump** (`state reset`, or PATCH E10 -> E2)
    was re-advanced by the next apply's aired-branch sync; it now sticks
    (both via the invalidation path -- an aired occurrence with no
    snapshot contributes no advance and the chain re-seeds from the
    freshly patched live state, landing the change on the next
    not-yet-aired occurrence -- and via the operator-wins guard when the
    stale snapshot survived).
  - **A committed program that vanished from the Tunarr catalog** is
    reconstructed from history with no season/episode metadata, which
    made the old derivation skip the show entirely and re-air the same
    episodes; post-state replay never reads content at all.
  - **Block edits computed the snapshot-invalidation cutoff from the
    POST-edit spec and ignored `max_duration_overflow_minutes`**
    (`internal/api/blocks.go`, `store.InvalidationCutoff`): shortening a
    block's duration left the occurrence still airing under the OLD,
    longer envelope with a live stale snapshot. `InvalidationCutoff` now
    takes the block spec and widens by duration + overflow, and
    `UpdateBlock` captures the cutoff from the PRE-edit spec before
    overwriting it.
  - Legacy `series_occurrence_snapshots` rows written before migration
    `000006` (no post-state) degrade gracefully: aired occurrences still
    replay committed content verbatim and simply contribute no cursor
    advance of their own (the old code's plan already advanced live
    state itself); future occurrences self-heal on their next
    re-derivation. Covered at both the store and engine level.
  - Regression tests beyond the above: direct unit tests for
    `syncPostStates` (cursor-only writes, `LastAired` stamped from the
    occurrence's own airtime, Seeded-sentinel non-leak, both guards);
    `docs/scheduling-concepts.md`'s idempotent-apply section rewritten
    to describe the real post-state-replay/operator-wins semantics.
- **The final-gate review corrected the replay guard semantics above:
  the anti-drag-back guard is now PROVENANCE-scoped (plan order), not
  value-scoped, and post-state replay carries the completion fields
  too.** This supersedes the previous entry's *monotonic* guard
  description and lifts its `Completed`/`Disabled`/`RunCount` exclusion:
  - **A value-scoped "only move the cursor forward" guard dropped a
    legitimate `on_complete: restart` wrap (post-state S01E01 after
    S01E05) as "backward"** -- the persisted cursor froze at the
    pre-wrap high-water mark, and the next snapshot invalidation
    (operator write, block edit/delete, or retention GC) re-derived
    from the frozen cursor, regressing onto ALREADY-AIRED episodes
    permanently. Migration `000007` adds `plan_seq` to
    `series_occurrence_snapshots` (the engine-allocated, strictly
    monotonic sequence -- `Engine.nextPlanSeq` -- of the plan
    generation that wrote the row) and `cursor_plan_seq` to
    `series_state` (the provenance of the current cursor value: the
    plan whose post-state last wrote it, also stamped by a case-3
    real-plan's direct live write). A replay now wins exactly when its
    `plan_seq` is newer than the live provenance -- in EITHER
    direction, so wraps land -- while a stale, older-plan replay (a
    slower block sharing the show) is still rejected. The operator-wins
    stamp guard is unchanged and still checked first. Pre-000007 rows
    default to `plan_seq` 0 and simply contribute no advance, the same
    graceful degradation as pre-000006 rows.
  - **`Completed`/`Disabled`/`RunCount` were read off the post-state
    and discarded, so `on_complete: disable` never disabled and
    `max_runs` never tripped in persisted state** (the chain re-decided
    -- and re-logged -- the disable on every apply while `GET
    /state/series` showed `runs: 0` and an active show forever). The
    blanket exclusion existed to protect operator PATCHes, which the
    `operator_updated_at` stamp now does properly -- so
    `Engine.syncPostStates` writes all three from the post-state under
    the same two guards, `RunCount` via max() (run counts only
    accumulate, never regress or double-count).
  - **`state import` (restoring a backup) stamped no
    `operator_updated_at` -- it wrote the FILE's stale/NULL stamp --
    and invalidated no snapshots**, so a just-restored cursor was
    silently re-advanced by the next apply. `Store.ImportSeriesStates`
    now stamps every imported row with a fresh operator write time (a
    backup's own stamp records some PAST write, not this one), and
    `stateImportCmd` (`cmd/state.go`) invalidates every imported show's
    not-yet-finished occurrence snapshots, warn-and-continue, exactly
    like `state reset`/`state set`.
  - The prior round's restart regression test was found vacuous (its
    wrap target coincided with the fresh-default cursor and its first
    apply real-planned the wrap directly into live state) and was
    rewritten to discriminate: live cursor first established at the
    pre-wrap high-water mark, the wrap arriving ONLY via the
    aired-branch replay of a chain-planned occurrence, held stable
    across three applies, and surviving a full snapshot invalidation
    without regressing onto just-aired episodes -- verified to fail
    with the guard reverted AND with the sync stubbed. New regression
    tests: provenance-rejects-stale-plan and
    provenance-allows-backward-wrap unit tests; on_complete:disable
    persists after airing; max_runs trips exactly once (run_count
    stable at 2); import stamps a fresh operator write (store-level)
    and an imported older cursor sticks across applies (service-level,
    real store + fake Tunarr). `docs/scheduling-concepts.md` updated so
    "decided exactly once at plan time, replayed into the persisted
    cursor" is true again, including for wraps and completion state.
- **The closing review pass refined the provenance guard's backward-move
  rule and hardened the sequence allocator** (amending the previous
  entry's "newer plan wins in EITHER direction"):
  - **Provenance alone reopened the shared-show rewind from the other
    side**: a not-yet-aired occurrence is re-derived every apply, so it
    always carries the freshest `plan_seq` -- while its stored PRE-plan
    baseline stays frozen. When two blocks share a show, the slower
    block's occurrence eventually airs holding "newest plan + ancient
    baseline" and rewound live E7 -> E3, re-airing e3-e6 on the next
    new occurrence. A BACKWARD move now additionally requires baseline
    agreement -- the plan's own `PreStates` cursor must equal the live
    cursor (compare-and-swap). A restart wrap passes (its pre-state IS
    the live high-water mark it planned from); a stale-baseline slow
    block fails. Forward moves keep pure plan-order provenance. Sharing
    a `show_title` across multiple series blocks is thereby supported
    (one cooperatively-advanced cursor, never rewound past the furthest
    point reached) and is now documented honestly in
    `docs/scheduling-concepts.md`, including that it interleaves ONE
    continuous run, not per-block parallel runs.
  - **A case-3 real plan stamped `cursor_plan_seq` on every
    `block.Series` show holding a pendingStates entry -- including
    entries written by an EARLIER block in the same apply.** A second
    block that never reached a shared show (disabled, completed, or
    duration exhausted before its turn) bumped the live provenance
    without changing the value, outranking -- and silently dropping --
    the first block's legitimate later replay. Only shows the plan
    actually changed (pre/post capture diff) are stamped now, via max()
    rather than assignment so provenance can never move backward.
  - **The sequence allocator's floor was the wall clock alone**: a
    backward clock step, or an import carrying a clock-ahead
    `cursor_plan_seq`, wedged the guard (every replay <= live
    provenance, silently dropped) until the clock caught up. New
    `StateStore.MaxPlanSeq` (max across snapshot `plan_seq` and live
    `cursor_plan_seq`) seeds `Engine.lastPlanSeq` at construction, so
    fresh sequences always outrank every stored one; a seed failure
    logs and falls back to the wall clock rather than failing
    construction.
  - Regression tests, each probed red against the reverted behavior:
    the reviewer's two-block rewind probe end to end (B plans
    far-future early, A advances to E7 and airs, B airs with newest
    seq -> live stays E7, next new occurrence continues e7-e8);
    backward-move CAS unit tests (wrap-with-agreeing-baseline lands,
    stale baseline and missing baseline rejected);
    second-block-never-reaches-shared-show provenance no-bump;
    plan-seq floor seeding (clock-ahead stored provenance) and
    store-level `MaxPlanSeq` spanning both tables.

## [0.2.1] - 2026-08-29

Bugs found during the first live `--apply` against a real Tunarr 1.3.13
instance and this deployment's configured (non-UTC) timezone -- including
one raised in review after the first round of fixes: the applied lineup
carried no wall-clock anchoring at all, so scheduled content aired
back-to-back from wherever the channel's playback happened to be, not at
its block's actual cron time.

### Fixed

- **`UpdateSchedule` now sends the live manual-lineup contract, not a
  dead PUT route.** Live-verified this session: a real Tunarr 1.3.13
  instance has no PUT route for `/api/channels/{id}/programming` at all
  (`404 {"message":"Route PUT:... not found"}`); `tunarr.Client` sent
  PUT anyway. Verified the actual contract against the Tunarr source at
  tag `v1.3.13` (`github.com/chrisbenincasa/tunarr`) rather than the
  stale vendored `docs/tunarr/openapi.json` (v1.0.16), which also
  documents this route incorrectly (it lists a required `programs`
  field the live "manual" branch never reads). `Client.UpdateSchedule`
  now POSTs `{"type": "manual", "lineup": [...], "append": false}`
  (`types/src/api/index.ts`'s `ManualProgramLineupSchema`), matching
  what `server/src/db/channel/LineupRepository.ts`'s `updateLineup`
  actually consumes. `internal/service/schedule_test.go`'s fake Tunarr
  server now mirrors live semantics (404 on PUT, decodes and validates
  the POST body) so a regression back to PUT fails a test instead of
  silently passing.
- **The applied lineup now carries real wall-clock anchoring instead of
  being pushed as bare back-to-back content.** Found in review of the
  fix above: `flattenSlots` concatenated every slot's programs in order
  and dropped every `ScheduledSlot.StartTime`/`EndTime`, and nothing
  ever built a "flex" (dead-air) entry -- so a `30 20 * * 6` block aired
  whenever the channel's playback cursor happened to reach it, not at
  20:30. Root cause, source-verified against tag `v1.3.13`: Tunarr
  computes playback position purely from elapsed wall-clock time since
  `channel.startTime`, modulo the pushed lineup's own total duration
  (`server/src/stream/StreamProgramCalculator.ts`'s
  `calculateStreamDuration`) -- there is no way to attach an absolute
  timestamp to an individual lineup item. Fixed with two changes,
  applied together on every `UpdateSchedule` call: (1)
  `service.buildAnchoredLineup` (`internal/service/schedule.go`)
  converts a channel's scheduled slots into a lineup that pads every
  gap with a `"flex"` entry -- before the first slot, between slots, for
  whatever's left of a slot's own nominal duration once its content runs
  out, and a trailing pad out to the apply window's end -- so cumulative
  item durations from offset 0 equal each item's real wall-clock offset,
  and the pushed lineup always covers at least the entire apply window
  (never less, preventing Tunarr from looping it back to the start
  before the next cron re-apply replaces it); (2)
  `tunarr.Client.UpdateSchedule` now anchors `channel.startTime` to that
  same window-start instant before pushing the lineup, via a
  read-modify-write round trip against `PUT /api/channels/{id}`
  (`SaveableChannelSchema`) -- the only live-reachable way to change it
  (`ChannelDB.updateChannelStartTime` and
  `LineupRepository.setChannelPrograms`'s optional `startTime` parameter
  both exist server-side but are unreachable from any HTTP route in this
  version). `tunarr.LineupItem` (`internal/external/tunarr/models.go`)
  now models both the `"content"` and `"flex"` wire variants; `internal/service`'s
  fake Tunarr server mirrors both the channel-anchor round trip and the
  lineup POST so a regression back to unanchored, flex-less pushes fails
  a test. Documented as an explicit "channel ownership" contract --
  every apply fully replaces the target channel's whole timeline,
  off-hours included -- in `docs/scheduling-concepts.md` and on
  `Client.UpdateSchedule`'s doc comment.
- **Cron occurrence planning now honors the configured timezone instead
  of always evaluating in UTC.** `log.timezone` was loaded correctly and
  threaded all the way to `scheduler.Engine.location`, but
  `GenerateForTimeRange` never actually applied it: it passed `start`
  (a bare `time.Now()`, carrying the process's own zone -- UTC in a
  container without `TZ` set) straight into `robfig/cron`'s
  `SpecSchedule.Next()`, which -- for a cron string with no `CRON_TZ=`
  prefix, which none of this repo's blocks carry -- matches calendar
  fields against whatever `Location` its input already has, not against
  a schedule-level setting. A block like `30 20 * * 6` was therefore
  planned against 20:30 UTC, not 20:30 in the deployment's configured
  zone, e.g. still planning tonight's occurrence after 20:30 in the real
  zone had already passed. Fixed by converting `start` to
  `e.location` at the top of `GenerateForTimeRange`
  (`internal/scheduler/engine.go`); `Next()` converts its result back
  to the same location it was given, so the rest of the occurrence loop
  follows automatically. Regression test uses a synthetic
  `time.FixedZone` (not a real IANA zone) so it doesn't depend on the
  test environment's tzdata.

## [0.2.0] - 2026-08-29

Series-based scheduling now works end-to-end against a live Tunarr
instance: two pre-existing bugs are fixed (the show/season ID join, and a
program-type validator that discarded an entire fetched page instead of
skipping the one entry it didn't recognize). Also in this release: a
Simple-mode schedule picker, library-aware show/genre/rating autocomplete
backed by two new read-only API endpoints, a full UI/copy polish pass, an
MkDocs documentation site, and a `kin-openapi` dependency bump that closes
both open Dependabot alerts.

### Added - 2026-08-28

#### HTTP API Server

- **`schedularr serve`**: new command running the HTTP API and the cron
  scheduling loop in one long-lived process, replacing the old daemon
  loop (see "Removed" below). Endpoints: blocks CRUD, block
  import/export, schedule generate/apply, schedule history, series
  state, channels, status, plus unauthenticated `/healthz`, `/readyz`,
  `/metrics`, and `/openapi.json`.
- **Contract-first API**: `api/openapi.yaml` (OpenAPI 3.0.3) is the
  source of truth; `internal/api/gen/server.gen.go` is generated from it
  via `make generate` (oapi-codegen) and must not be hand-edited.
- **Bearer-token auth**: `/api/v1/*` requires `Authorization: Bearer
  <token>`, constant-time compared. Token comes from
  `SCHEDULARR_API_TOKEN` (wins when set) or the `api.token` config key;
  `serve` refuses to start with a token under 32 characters unless
  `--insecure-no-auth`/`api.insecure_no_auth` is set.
- **`internal/service`**: extracted the generate/apply workflow out of
  the CLI so `cmd/generate.go` and the API's schedule endpoints share one
  implementation (`service.Runner`).
- **`internal/store`**: SQLite-backed persistence for blocks, series
  state, and schedule history (see "Changed" below).

#### Web UI

- **A Hugo-built web UI**, embedded into the `schedularr` binary via
  `go:embed` (`web/embed.go`) and mounted by `schedularr serve` at the
  same origin and port as the API, behind everything else the router
  already handles (system routes and `/api/v1/*` still win first). Four
  pages, each backed entirely by the existing `/api/v1` contract, no new
  server-side endpoints:
  - **Dashboard** (`/`) -- Tunarr reachability, server version, block
    count, and the last 7 days of schedule history.
  - **Blocks** (`/blocks/`) -- list, create, edit, delete, and
    enable/disable scheduling blocks, including the full filter- and
    series-block field sets and a plain-language cron-pattern hint.
  - **Schedule** (`/schedule/`) -- dry-run preview of the next schedule
    cycle per channel, then an explicit-confirmation apply.
  - **Series** (`/series/`) -- every tracked show's season/episode
    cursor, with inline editing and completed/disabled toggles.
  - A single bearer token (the same one `schedularr serve` was started
    with) unlocks the UI; it lives only in the browser's `localStorage`
    and is never embedded in the served HTML/JS. A `401` from any API
    call reopens the token panel automatically.
- **`web/DESIGN.md`**: the shipped design system -- token inventory,
  component-class catalog, the light/dark mechanism (OS/browser
  preference only, no manual toggle), WCAG contrast evidence, and the
  Alpine.js conventions the UI code follows.
- **Content-Security-Policy** on every UI response, alongside the
  existing `X-Content-Type-Options`/`Referrer-Policy` headers: `default-src
  'self'; script-src 'self' 'unsafe-eval'; style-src 'self'; img-src
  'self' data:; connect-src 'self'; frame-ancestors 'none'`. Every
  directive is same-origin/none (no third-party origins anywhere in the
  UI); `'unsafe-eval'` is required by Alpine.js 3's expression evaluation.
  See `web/DESIGN.md`'s Content-Security-Policy section for the full
  rationale.
- **Blocks list delete-confirm** moves focus to the Confirm button once
  the row's Delete button is replaced by Confirm/Cancel, instead of
  leaving a keyboard/screen-reader user's focus stranded on the removed
  element.
- See the README's [Web UI](README.md#-web-ui) section for the full page
  tour, build instructions (`make web`), and prerequisites (Hugo ≥
  0.120, Node/npm).

### Changed - 2026-08-28

#### Module rename

- Module path changed from `github.com/geekxflood/schedularr` to
  `github.com/christopherime/schedularr` (repository transferred to a
  new owner). Every import path was rewritten in the same change;
  `schema.json`'s `$id` was missed at the time and corrected later in
  this same sub-project.

#### Blocks moved into the SQLite store

- Scheduling blocks now live in `internal/store`, not in a config file.
  `scheduler.yaml` is a **first-run import format only**: on an empty
  store, `internal/blockio.Bootstrap` imports its blocks once; the file
  is never read again afterward, and editing it post-bootstrap has no
  effect. Manage blocks going forward through `/api/v1/blocks`.
- `schedularr scheduler init` still authors a `scheduler.yaml` import
  file; `config.yaml`'s inline `scheduler:` block (legacy support) is no
  longer consulted by any code path -- config.cue documents the field but
  nothing reads it (flagged for cleanup, not yet removed).

#### `serve` replaces `run`

- The `run` daemon command (interval-based generate-and-apply loop,
  SIGHUP config reload) is gone; `serve` runs the same generate-and-apply
  cycle on a cron timer alongside the HTTP API, sharing one store
  connection and one graceful-shutdown path. SIGHUP reload and `--once`
  were not carried over -- `serve` has no config-reload story and is
  always long-running.
- Cadence is controlled by the `cron_interval` config key (default `6h`,
  a top-level key since it governs the scheduler, not the HTTP server) or
  `serve --interval`/`-i`, which overrides it when passed explicitly.

### Removed - 2026-08-28

#### Interactive TUI

- Deleted entirely: `internal/tui/`, `cmd/tui.go`, and the
  `charmbracelet/bubbletea`/`lipgloss`/`huh` dependencies. No deprecated
  alias was kept.
- `generate --apply` now requires an explicit `--yes` flag -- the
  `charmbracelet/huh` confirmation prompt it used to show is gone, and
  there is no other interactive confirmation. `--apply` without `--yes`
  fails fast with an error instead of running.

#### Jellyfin, Sonarr, and Radarr integrations

- Removed `internal/external/jellyfin/`, `internal/external/sonarr/`,
  `internal/external/radarr/`, `cmd/content_sources.go`, and their config
  sections in `cmd/schema/config.cue`. Tunarr is now the sole runtime
  integration; content availability filtering and the Jellyfin
  guide-refresh hook were removed along with the clients, not ported.

#### `run` command

- The interval-based daemon command is gone; see "`serve` replaces
  `run`" above.

### Added - 2026-08-29 (media discovery API)

#### Media discovery endpoints

- **`GET /api/v1/media/shows`** and **`GET /api/v1/media/meta`**: the first
  deliberate post-v1 contract change to `api/openapi.yaml`. `listMediaShows`
  returns every distinct show title Runner's Tunarr fetch has seen, grouped
  from `Type == "episode"` programs, with each show's episode count
  (`MediaShow{title, episode_count}`); `getMediaMeta` returns the distinct
  `genres`/`ratings` observed across every fetched program
  (`MediaMeta{genres, ratings}`), both sorted ascending. Both reuse
  `Runner.fetchPrograms` -- the same fetch-then-cache path `generate` uses
  to build its scheduling candidate pool -- so neither issues an extra
  Tunarr request beyond warming or reading the existing 1h content cache.
  A `nil` `Deps.Media` (Tunarr not configured) and a live fetch failure
  both return `502`; the latter distinguishes "tunarr unreachable" from
  "tunarr response invalid" (`httpclient.IsDecodeError`) the same way
  `GET /channels` already did.
- These two endpoints are what the blocks editor's library-aware
  autocomplete (see the UI improvement wave below) reads from.

### Fixed - 2026-08-29

#### Tunarr wire format: show/season ID joins, pagination truncation, and a dead search filter

Three bugs against a real Tunarr 1.3.13 instance, all in
`internal/external/tunarr` and `internal/service`: schedularr's Tunarr
client was built against invented response shapes that happened to
satisfy this repo's own test fixtures but never matched what Tunarr
actually sends over the wire. This entry supersedes an earlier same-day
version of itself that claimed an episode result nests its show under a
`show` object -- that claim was based on a spec read, not a live capture,
and was wrong; see "What we got wrong the first time" below.

- **Series-block scheduling now works against a live Tunarr instance.**
  Live-verified this session (transcript in
  `.superpowers/sdd/2026-08-29-deploy/wire-fix-report.md`): a real
  `/api/programs/search` "episode" result carries no flat
  `showTitle`/`rating`/`seasonNumber` key, and does **not** nest a `show`
  object either -- it carries only `showId`/`seasonId` foreign keys. Its
  show is a *separate*, `Type == "show"` entry Tunarr interleaves in the
  *same paginated result stream* as episodes (not co-located on the same
  page as its own episodes, in general), and there is no equivalent
  interleaved entry for seasons at all.
  `internal/service.Runner.hydrateShowsAndSeasons` (schedule.go) is the
  fix: called once per fetch on the *fully accumulated* `[]Program` (after
  every page has been fetched, so a show entry and its episodes are
  guaranteed to have landed in the same slice regardless of which pages
  they arrived on), it (1) joins each episode's `ShowID` against the
  interleaved `Type == "show"` entries to fill `ShowTitle`/`Rating`, and
  (2) resolves each distinct `SeasonID` individually via the new
  `Client.GetSeason` (`GET /api/programming/seasons/{id}`, whose season
  number is the response's `index` field -- also live-verified; a
  no-batch-equivalent check against `POST /api/programming/batch/lookup`
  confirmed it takes external, source-specific IDs and returns an
  unrelated response shape, so it isn't usable here) to fill
  `SeasonNumber`, caching each resolution in Runner's existing 1h content
  cache. `tunarr.Client`'s earlier nested-`show`-object hydration
  (`hydrateEpisodeShowFields`) is kept as a harmless secondary path --
  correct if a richer response shape ever did nest show data -- but does
  not fire against Tunarr 1.3.13 today. A flat `showTitle`/`rating`/
  `seasonNumber` key (this repo's own `testdata/programs/*.json`
  fixtures) still works unchanged: neither hydration path ever overrides
  an already-set flat value.
- **Libraries and searches over 100 programs are no longer silently
  truncated to their first page.** `tunarr.ProgramSearchResponse` modeled
  a `total`/`limit` pair no live response actually sends -- the real
  envelope is `{results, page, totalPages, totalHits,
  facetDistribution}` -- so `resp.Total` always deserialized to `0`, and
  `internal/service.Runner`'s pagination loops
  (`fetchSingleLibrary`/`fetchAllProgramsViaSearch`, schedule.go's two
  `for { ... SearchPrograms ... }` loops) stopped after the very first
  100-program page every time, regardless of how many programs actually
  matched. Replaced `Total`/`Limit` with `TotalPages`/`TotalHits`
  (matching the live envelope; no legacy fields kept) and fixed both loops
  to continue until `page >= resp.TotalPages`. (This part was already
  correct in the earlier same-day version of this entry and needed no
  changes this round.)
- **Removed `tunarr.SearchFilter` (`ProgramSearchQuery.Filter`).** Never
  constructed by any code path in this repo, and wrong besides: the real
  request schema's `query.filter` is an expression-tree shape (`{type:
  "op"|"value", ...}`), nothing like the flat `{type: []string}` this
  struct modeled -- live-verified this session that POSTing the old shape
  against a real instance returns `400 FST_ERR_VALIDATION`. Deleted
  outright (no-legacy) rather than fixed, since nothing used it.

##### What we got wrong the first time

An earlier version of this fix (same day) modeled `tunarr.Program.Show`
(a nested show object) and `hydrateEpisodeShowFields`, claiming that was
what a live Tunarr episode result actually sends, and that fixing it made
series-block scheduling and `GET /media/shows`/`GET /media/meta` work
against real data. That claim was **not backed by a live capture** --
every citation was a read of the vendored `docs/tunarr/openapi.json`
spec, which does describe a nested-`show` `Episode` schema variant, but a
real Tunarr 1.3.13 instance doesn't send it: a live capture this round
(84 episodes, 16 interleaved show entries) found 0 nested `show` objects.
The nested-`show` code and its tests are kept (see above -- harmless,
possibly useful if a richer shape ever ships), but the actual production
fix is the `ShowID`/`SeasonID` join described above. Pagination (the
`resp.Total` -> `resp.TotalPages` fix) was independently live-confirmed
correct in that same earlier round and needed no correction.

Every fix in this entry is pinned by tests running an actual fake-Tunarr
HTTP round trip in the live wire shape (not just Go struct literals
bypassing JSON), plus direct unit tests of the join and season-resolution
functions in isolation: `internal/external/tunarr/client_test.go` decodes
hand-written live-shaped response bodies (envelope, and a `showId`-only
episode alongside an interleaved show entry) and adds `Client.GetSeason`
coverage; `internal/service/schedule_test.go` adds a 250-program/3-page
fetch-truncation test, a pagination+join interaction test (show entry on
page 1, its episode on page 3), a series-block end-to-end test using only
`ShowID`/`SeasonID` FKs plus a fake seasons endpoint, and isolated unit
tests of `hydrateShowTitleAndRating`/`hydrateSeasonNumbers`/
`resolveSeasonNumber`'s caching; `internal/service/media_test.go` adds a
live-shaped fixture variant for `MediaShows`/`MediaMeta`. The join and the
season resolver were each independently disabled and re-verified to
confirm their respective tests fail without them.

#### Round 3: a growing library's scan could still discard every fetched program

The two fixes above were real, but a third, pre-existing bug -- exposed
specifically by a library large enough to reach a "season"-type entry --
still made `fetchSingleLibrary` discard an entire fetch's worth of
already-accumulated pages. Fixed, and verified against the operator's own
live, ~10,600-hit library (not a synthetic fixture) via a scratch
`schedularr serve` run against `https://tunarr.local.geekxflood.io`: see
"Round 3" in `.superpowers/sdd/2026-08-29-deploy/wire-fix-report.md` for
the full transcript, including 493 real show titles and 23 real ratings
returned by `GET /api/v1/media/shows`/`GET /api/v1/media/meta`.

- **`tunarr.Program.Type`'s `validate:"oneof=..."` list was missing
  `"season"`** (and `"album"`/`"artist"`/`"collection"`/`"folder"`/
  `"playlist"`, all live-verified this round as real values a search
  result can carry) -- so once a growing library's search started
  interleaving season-type entries, `validateProgram` rejected every one
  of them, and the single-invalid-entry-aborts-the-whole-page behavior
  (pre-existing, unrelated to Bug 1/2) turned that into
  `fetchSingleLibrary` discarding every already-fetched page, not just the
  bad entry. Fixed both layers: the oneof list is now complete, and
  `SearchPrograms`/`GetFillerPrograms` skip-and-continue instead of
  aborting (new `filterValidPrograms`/`ProgramSearchResponse.DroppedCount`) -- one
  malformed or genuinely-unrecognized entry now costs exactly that one
  entry, logged once per whole fetch (not per page or per entry) via a new
  WARN in `internal/service/schedule.go`.
- **Season resolution now tries a local join first.** A live search
  interleaves `Type == "season"` entries the same way it interleaves show
  entries (live-verified: a 100-item page was observed as 100% season
  entries) -- `hydrateSeasonNumbers` now builds a `SeasonID -> index` map
  from whatever season entries already showed up in the accumulated fetch
  before falling back to the existing per-ID `Client.GetSeason` resolver,
  cutting a large fetch's season-related HTTP calls dramatically.
- **502 wording**: `GET /media/shows`/`GET /media/meta` used to say
  "tunarr unreachable" for every failure, including one where Tunarr was
  reached fine and the problem was a response body that didn't decode into
  the expected shape. New `httpclient.IsDecodeError` distinguishes that
  case (`"tunarr response invalid"` / `"tunarr returned unexpected data"`)
  from genuine connectivity/status failures (unchanged wording).

### Added - 2026-08-29 (UI improvement wave)

#### Smart schedule picker (blocks editor)

- A Simple/Cron mode toggle on the blocks editor's schedule field.
  Simple mode is a frequency select (daily / weekdays / weekly / monthly
  / custom days), day-of-week checkboxes (weekly/custom), and a native
  `<input type="time">`, generating the 5-field cron string live;
  switching from Cron to Simple mode parses the current cron back into
  the picker when the pattern is representable, and locks to Cron mode
  with an inline note otherwise. Storage/API are unaffected -- the cron
  string is still the one value `POST`/`PUT /api/v1/blocks` sends.
- **cronstrue** (vendored, `web/assets/vendor/cronstrue.min.js`, English
  locale, MIT) replaces the editor's earlier hand-rolled `cronHint()` for
  the plain-language readback shown live in both modes, and for the
  blocks table's per-row cron readback -- `cronHint()` only recognized a
  narrow subset of patterns (fixed time, optionally weekday-restricted);
  cronstrue reads any valid 5-field expression.

#### Library-aware autocomplete (blocks editor)

- Series rows' show-title field, and the genre/rating fields on both the
  filter block and the series fallback's filler filter, are now
  `<input list=...>` fields backed by `<datalist>`s populated from `GET
  /api/v1/media/shows`/`GET /api/v1/media/meta`, fetched once per editor
  open and reused across every row. Free text is always accepted
  regardless of fetch outcome; a failed fetch degrades silently (no
  datalist, no warning, never a `.problem` panel).
- A soft, non-blocking amber warning ("Not found in Tunarr's library.")
  appears under a series row whose show title doesn't
  case-insensitively match any loaded show, once the media fetch has
  succeeded.

#### UI audit + copy audit fixes

Implemented every P1/P2 item from this wave's UI and copy audits
(`.superpowers/sdd/2026-08-29-deploy/{ui-audit-impeccable,
copy-audit-stopslop}.md`), plus all P3 items (all effort-S and adjacent
to the above): the blocks editor now returns focus to the triggering
button on close; the "unarmed" token status dot uses `--color-warn`
instead of `--color-danger` (a default first-run state, not an error);
`.btn--sm`/`.toggle` now clear WCAG 2.5.8's 24x24 CSS-px target-size
floor; required fields carry a static `*` marker; series cursor field
errors are wired for assistive tech (`role="alert"`, `aria-describedby`);
the schedule/blocks error panels gained `Retry` actions; the toggle's
"on" state changes its own label color, not just the track; the series
Runs column is right-aligned and numerically formatted; both dialogs'
explanatory text is now linked via `aria-describedby`; assorted P3
polish (404 CRT scanline, `hero-panel__meta` graticule dividers, footer
instrument-legend styling, dead-CSS documentation, real plurals
throughout instead of a mechanical "(s)" suffix). Copy fixes: 404's
"channel" -> "page" (was misleadingly reusing a Tunarr-channel term for
a UI route), "arm" scoped strictly to the token panel (blocks copy now
says "create"), consistent em dash and Title Case button labels, and the
schedule apply-confirm's wording aligned on "Apply"/"Applied" throughout.

#### Fixed

- `GET /channels`'s 502 no longer echoes the wrapped Tunarr connectivity
  error into the response `Detail` (`internal/api/tunarr.go`'s
  `ListChannels`) -- it now logs server-side and returns the same fixed
  "unable to reach tunarr" wording `writeMediaAPIError` already used for
  `/media/shows`/`/media/meta`'s equivalent failure.

### Added - 2026-01-12

#### CUE Schema Integration

- **CUE Schema Files**: Created comprehensive schemas for application and scheduler configurations
  - `cmd/schema/config.cue` - Application configuration schema with validation and defaults
  - `cmd/schema/scheduler.cue` - Scheduler configuration schema with block types and filters
  - Embedded schemas in `internal/cueconfig/schema/` for runtime use
- **Schema Validation Package**: New `internal/cueconfig` package for CUE-based validation
  - `ValidateConfig()` - Validates application configuration against schema
  - `ValidateScheduler()` - Validates scheduler configuration against schema
  - `GenerateConfig()` - Generates config files from schema with defaults
  - `GenerateScheduler()` - Generates scheduler files from schema with defaults
  - Support for both YAML and JSON formats

#### CLI Restructuring

- **New CLI Structure**: Migrated from `cmd/schedularr/main.go` + `internal/cli/` to standard Cobra layout
  - Entry point: `main.go`
  - Commands: `cmd/` package
  - Removed old `internal/cli/` directory
- **New Commands**:
  - `config generate [filename]` - Generate application config from CUE schema
  - `validate <file>` - Validate any config file against CUE schemas
  - `scheduler init [filename]` - Generate scheduler config from CUE schema (updated)
  - `scheduler validate [filename]` - Validate scheduler config
  - `scheduler list [filename]` - List all configured blocks
- **Enhanced Root Command**:
  - Updated descriptions and examples
  - Integrated Viper for configuration management
  - Added `--config` global flag for custom config files
  - Auto-detection of config files in home and current directories

#### Documentation

- **CLI Reference**: Comprehensive CLI documentation in `docs/CLI_REFERENCE.md`
  - Complete command reference with examples
  - Configuration file templates
  - Quick start workflows
  - Exit codes and environment variables
- **Updated README**:
  - New installation instructions
  - Updated quick start guide with new commands
  - Added reference to CLI documentation
  - Updated configuration examples
- **Updated TODO**: Marked Phase 0.1 and 0.2 as completed with detailed notes

### Changed

#### Build Process

- **Build Command**: Updated from `go build -o schedularr cmd/schedularr/main.go` to `go build -o schedularr main.go`
- **Import Paths**: All CLI commands now use `cmd` package instead of `internal/cli`

#### Configuration Generation

- **Scheduler Init**: Changed from template-based to CUE schema-based generation
  - Removed hardcoded templates (basic, advanced, series)
  - Now generates from schema defaults with example blocks
  - Auto-detects output format from file extension

#### Validation

- **Config Loading**: Removed inline CUE validation from config loading
  - Validation now explicit via `validate` command
  - Runtime validation can be added separately if needed

### Removed

- **Old CLI Structure**: Removed `internal/cli/` directory and all files
- **Old Entry Point**: Removed `cmd/schedularr/main.go` and `schedularr/` directory
- **Template Files**: Removed hardcoded scheduler templates from `scheduler.go`
- **Validator File**: Removed `internal/config/validator.go` (replaced by `internal/cueconfig`)

### Technical Details

#### Dependencies

- Added `cuelang.org/go/cue` for CUE schema support
- Added `cuelang.org/go/cue/cuecontext` for CUE context management
- Using `gopkg.in/yaml.v3` for YAML marshaling

#### File Structure

```txt
schedularr/
├── main.go                          # Entry point
├── cmd/
│   ├── root.go                      # Root command
│   ├── config.go                    # Config management (NEW)
│   ├── validate.go                  # Validation (NEW)
│   ├── scheduler.go                 # Scheduler management (UPDATED)
│   ├── generate.go                  # Schedule generation
│   ├── run.go                       # Daemon mode
│   ├── tui.go                       # Interactive TUI
│   ├── channels.go                  # Channel listing
│   └── schema/                      # CUE schemas
│       ├── config.cue               # App config schema
│       └── scheduler.cue            # Scheduler config schema
├── internal/
│   ├── cueconfig/                   # CUE validation (NEW)
│   │   ├── schema.go
│   │   └── schema/
│   │       ├── config.cue           # Embedded
│   │       └── scheduler.cue        # Embedded
│   ├── config/                      # Config loading
│   ├── scheduler/                   # Scheduling engine
│   ├── store/                       # State persistence
│   ├── tui/                         # TUI components
│   └── tunarr/                      # Tunarr API client
└── docs/
    └── CLI_REFERENCE.md             # CLI documentation (NEW)
```

#### Testing

- ✅ All existing tests pass
- ✅ Config generation tested with YAML and JSON
- ✅ Scheduler generation tested with YAML and JSON
- ✅ Validation tested with valid and invalid configs
- ✅ Build succeeds without errors

### Migration Guide

For users upgrading from previous versions:

1. **Update Build Command**:

   ```bash
   # Old
   go build -o schedularr cmd/schedularr/main.go

   # New
   go build -o schedularr main.go
   ```

2. **Generate New Configs**:

   ```bash
   # Generate application config
   schedularr config generate config.yaml

   # Generate scheduler config
   schedularr scheduler init scheduler.yaml
   ```

3. **Validate Existing Configs**:

   ```bash
   schedularr validate ~/.schedularr.yaml
   schedularr validate scheduler.yaml
   ```

4. **Update Scripts**: If you have automation scripts, update command paths and imports

---

### Added - 2026-08-29 (docs site)

- **MkDocs Material documentation site** (`mkdocs.yml`, `docs/*.md`), published
  to GitHub Pages by `.github/workflows/pages.yaml` (push to `main` touching
  `docs/**`/`mkdocs.yml`/`assets/**`, plus `workflow_dispatch`). Nine pages —
  Home, Getting Started, Web UI Guide, Scheduling Concepts, CLI Reference, API
  Reference, Architecture, Design System, Deployment — replace the
  everything-in-README approach. `docs/` is the site's `docs_dir`;
  `docs/superpowers/` and `docs/tunarr/` (internal SDD planning artifacts and a
  captured Tunarr OpenAPI spec) are excluded from the build via
  `exclude_docs` and stay where they were.
- `docs/assets/` — a working copy of `demo.gif`, `cli.gif`, and the four
  `screenshots/*.png` the site's pages embed, so the same relative image
  paths render both on the built site and in GitHub's repo view of
  `docs/*.md`.

### Changed - 2026-08-29 (docs site)

- **README.md**: slimmed to logo/badges/hero GIF, a one-paragraph
  description, a short feature list, a `docker run` quickstart, and links to
  the docs site/chart/releases. Everything else (full config reference,
  Web UI page tour, API endpoint tables, CLI reference, architecture,
  examples) moved to the docs site — each topic now has exactly one home.
- `AGENTS.md`: its two links to README's old `#-docker` section now point at
  the docs site's Deployment page instead.

### Removed - 2026-08-29 (docs site)

- `docs/ARCHITECTURE.md`, `docs/SPECIFICATIONS.md`, `docs/CLI_REFERENCE.md`,
  `docs/SERIES_SCHEDULING_GUIDE.md` — fully absorbed into the docs site
  (`docs/architecture.md`, `docs/scheduling-concepts.md`,
  `docs/cli-reference.md`) with content merged/deduped, not just moved.

### Changed - 2026-08-29 (release prep)

- `assets/demo.gif` and `assets/screenshots/{dashboard,blocks,schedule,
  series}.png` (and their `docs/assets/` copies) re-captured against the UI
  improvement wave: the blocks screenshot/GIF now show the editor open in
  Simple mode (schedule picker) on the series block, with the show-title
  field populated from the library-aware autocomplete's data source.

### Security - 2026-08-29 (release prep)

- Bumped `github.com/getkin/kin-openapi` (transitive, via `oapi-codegen`)
  from `v0.142.0` to `v0.144.0`, closing both open Dependabot alerts: a
  critical fail-open authentication bypass in
  `openapi3filter.ValidationHandler` (`NoopAuthenticationFunc` default) and
  a medium-severity nil-pointer panic in the same package when validating a
  `content` parameter whose media type has no schema. Neither code path is
  reachable from this repo -- `kin-openapi`'s only consumer is
  `internal/api/gen/server.gen.go`'s embedded-spec loader (`openapi3.T` /
  `GetSpec`), which never touches `openapi3filter`. `make generate`'s
  output is unchanged byte-for-byte after the bump (two consecutive runs
  diffed clean against each other and against the pre-bump output).

---

## [0.1.0] - 2026-01-XX (Previous Release)

### Added

- Initial release with basic scheduling functionality
- Tunarr API integration
- Filter-based content selection
- Cron-based scheduling
- Interactive TUI
- CLI commands: channels, generate, run, tui

[Unreleased]: https://github.com/christopherime/schedularr/compare/v0.2.3...HEAD
[0.2.3]: https://github.com/christopherime/schedularr/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/christopherime/schedularr/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/christopherime/schedularr/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/christopherime/schedularr/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/christopherime/schedularr/releases/tag/v0.1.0
