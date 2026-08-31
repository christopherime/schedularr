# Station Terminology, Any-Media Scheduling, and the History Page — Design Specification

**Status:** proposal. Three operator-directed product streams for the road to
v1.0.0, planned 2026-08-30. Nothing here is implemented; this document is the
design and the ladder placement, and it ends in Open Questions the operator
answers before any slice starts.
**Commit path:** `docs/superpowers/specs/2026-08-30-v1-station-terminology-media-history-design.md`
**Companion:** the v0.5 web overhaul spec
(`2026-08-30-v0.5-web-overhaul-design.md`) stays authoritative for the web
train; Stream 3 amends its §2 and §3.7 and extends its Memory slice.

Every capability claim about today's code in this document was read out of the
source and is cited by file and line. Where a design choice depends on
something not yet verified, it is marked **ASSUMPTION**.

---

## Stream 1 — Station lingo

### 1.1 Research: is "block" real?

Yes. "Block programming" is standard broadcast vocabulary in US and UK usage,
in trade and academic sources alike.

> "Block programming…the arrangement of programs on radio or television so that
> those of a particular genre, theme, or target audience are united."
> — [Wikipedia, *Block programming*](https://en.wikipedia.org/wiki/Block_programming)

> "A particularly long program block, especially one that does not air on a
> regular schedule, is known as a marathon."
> — [Wikipedia, *Block programming*](https://en.wikipedia.org/wiki/Block_programming)

The same source records the UK equivalent: block programming "is also known as
a **strand** in British broadcasting". Two facts matter for Schedularr:

1. A block is a **multi-programme, multi-hour, themed grouping** — not a single
   show slot. That is exactly what a Schedularr block produces: one cron
   occurrence fills `duration` minutes with several programmes chosen by one
   rule. The word fits the object.
2. The industry's block is the grouping *on air*; Schedularr's `Block` is the
   recurring *rule* that generates those groupings. The code already names the
   dated instance an **occurrence**, which keeps the two apart. No change
   needed.

**Verdict: keep "block".** It is authentic, it is the right shape, and it is
already in the operator's head, the UI, the CLI, the store, and the wire. The
adjacent candidates are worse for this object: *strand* is the UK synonym and
buys nothing; *daypart* is a division of the broadcast day
([Wikipedia, *Dayparting*](https://en.wikipedia.org/wiki/Dayparting)), not a
programme grouping; *lineup* is already taken by the Tunarr wire object the
engine builds (`buildAnchoredLineup`, `tunarr.LineupItem`); *slot* is already
taken by `ScheduledSlot` and by the guide's rendered cell; *clock*/*format*
is the radio hour template ([Wikipedia, *Broadcast
clock*](https://en.wikipedia.org/wiki/Broadcast_clock)), which describes a
whole-hour layout, not one rule.

### 1.2 Research: what do broadcasters call criteria-based selection?

There is no single cross-industry noun. The evidence splits by domain:

| Domain                                                   | Term for "pool of eligible content, order not predetermined"                  | Source                                                                                                                                                                                                                                                                                                          |
| -------------------------------------------------------- | ----------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Radio automation (RCS GSelector, Broadcast Radio Myriad) | **Category** (the pool) cycled by **Rotation** rules, called from a **Clock** | [RCS](https://www.rcsworks.com/gselector-priority-list-scheduling-tips/), [Broadcast Radio](https://help.broadcastradio.com/support/solutions/articles/101000563883-category-analysis-myriad-schedule-pro-only-), [Wikipedia, *Music scheduling system*](https://en.wikipedia.org/wiki/Music_scheduling_system) |
| IPTV playout (ErsatzTV)                                  | **Search** content source; the saved/named form is a **Smart Collection**     | [ErsatzTV](https://ersatztv.org/docs/scheduling/sequential/content/)                                                                                                                                                                                                                                            |
| IPTV playout (Tunarr — the integration target)           | **Random** slots (with a *Uniform* weighting sub-mode)                        | [Tunarr](https://tunarr.com/configure/scheduling/random-slots/)                                                                                                                                                                                                                                                 |
| Ad sales                                                 | **Run-of-schedule (ROS)** — *not* this concept: it is an ad-placement term    | [AllBusiness](https://www.allbusiness.com/dictionary-run-of-schedule-ros-4965120-1.html)                                                                                                                                                                                                                        |

> "'Search' content sources use ErsatzTV's search engine to dynamically find
> content matching your criteria (query)."
> — [ErsatzTV docs](https://ersatztv.org/docs/scheduling/sequential/content/)

"Pool" and "wheel" turned up no authoritative attestation and are dropped.

### 1.3 Research: what do they call fixed-order, position-remembering play?

Tunarr itself has first-class vocabulary for exactly Schedularr's cursor:

> "Sequential (or None) chooses slots in the order they are defined in the
> editor itself." — [Tunarr docs](https://tunarr.com/configure/scheduling/random-slots/)

> "The shared iterator advances every time any group member plays."
> — [Tunarr docs, *Continue Mode*](https://tunarr.com/configure/scheduling/concepts/)

ErsatzTV expresses the same as a **Playlist**/**Show** source with a
*chronological* **Playback Order**. Broader broadcast has no ubiquitous word for
"remembered position, advances one step per airing" — *stripping* is the
across-the-board daily placement
([Wikipedia, *Strip programming*](https://en.wikipedia.org/wiki/Strip_programming)),
*serial* describes the content, not the mechanism.

**"Sequence" is defensible and Tunarr-aligned.** One flagged collision: in film
and video, a *sequence* is an edited run of shots, and that is the first meaning
a media-literate reader reaches for. Tunarr sidesteps it by using "sequential"
as an adjective and "iterator" as the noun. Mitigation, not a re-litigation (the
operator fixed this choice): the UI and docs always say **"sequence block"** or
**"the sequence"** in a scheduling context, never a bare "sequence" as a
content noun.

### 1.4 Proposed names

| Today                            | Proposed                   | Why                                                                                                                                 |
| -------------------------------- | -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| block (concept)                  | **block** — unchanged      | Validated above.                                                                                                                    |
| `type: "series"`                 | `type: "sequence"`         | Operator-chosen, fixed. Aligns with Tunarr's "sequential"/iterator vocabulary.                                                      |
| `type: "filter"`                 | `type: "rotation"`         | Recommended; ranked against alternates below.                                                                                       |
| `filter: {…}` (the criteria bag) | `criteria: {…}`            | Under the no-legacy policy a half-rename leaves the confusing word in the schema. Ranked as a separate decision — Open Question Q3. |
| `fallback.filler_filter`         | `fallback.filler_criteria` | Follows the same decision.                                                                                                          |

**Ranked candidates for the `filter` block type:**

1. **`rotation`** — recommended. A real radio-automation term for exactly this
   object: a pool of eligible content cycled by rules. It reads as a
   *programming* noun rather than a mechanism, it contrasts cleanly with
   `sequence` (unordered-but-cycled vs ordered-and-remembered), and it is one
   short word in a `type:` enum. **Cost:** it is radio-native, and it implies
   stronger repeat protection than the engine gives — today's engine shuffles
   with global `rand` and excludes recently-scheduled programmes via the
   history window, then falls back to allowing repeats when the pool is
   exhausted (`internal/scheduler/engine.go:1210-1248`). That is a rotation
   with a soft guard, not a strict one. Document it where the word lands.
2. **`selection`** — plain English, matches the operator's phrasing ("program a
   selection by criteria") exactly. **Cost:** no industry attestation found; it
   is a generic word doing a specific job.
3. **`category`** — real (radio automation), but it collides head-on with
   Stream 2, which introduces genre/tag *categories* as filter criteria in the
   same release train. Rejected on that ground alone.
4. **`search`** / **`smart`** — real in ErsatzTV, names the mechanism (a query)
   rather than the programming. Rejected: Schedularr's object is a programming
   rule, and the mechanism word ages badly once tags and metadata arrive.
5. **`random`** — Tunarr's own word, so it would align vocabularies with the
   integration target. Rejected: it describes only the selection order, says
   nothing about the criteria that are the point of the block type, and reads
   like a defect in an operator-facing enum.

### 1.5 Blast radius

Measured by grep on 2026-08-30, non-test files unless noted.

| Surface                                        | What changes                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | Scale                                                                 |
| ---------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| OpenAPI (`api/openapi.yaml`)                   | `BlockSpec.type` enum (line 387); schemas `Filter` → `Criteria`, `SeriesConfig` → `SequenceConfig`, `SeriesFallback` → `SequenceFallback`, `SeriesState` → `SequenceState`, `SeriesStatePatch`; paths `/state/series`, `/state/series/{show_title}`                                                                                                                                                                                                                                                                   | 11 `Series` hits, 7 `Filter` hits                                     |
| Generated code                                 | `internal/api/gen/server.gen.go` (oapi-codegen) and `web/assets/ts/gen/types.d.ts` regenerate; CI's codegen-drift job is the gate                                                                                                                                                                                                                                                                                                                                                                                     | 2 artifacts                                                           |
| CUE (`cmd/schema/config.cue`, `scheduler.cue`) | `#Block.type` disjunction and default (`config.cue:90`), `#Filter` → `#Criteria`, `#SeriesConfig` → `#SequenceConfig`, `#FallbackConfig` field; the `scheduler.cue` example                                                                                                                                                                                                                                                                                                                                           | 11 hits                                                               |
| Go domain                                      | `BlockType{Filter,Series}`, `Filter`, `SeriesConfig`, `SeriesFallback`, `Block.Filter/.Series`, `SeriesState`, `SeriesStateSnapshot`, `FilterPrograms`, `planFilterBlock`, `planSeriesBlock`, `establishSeriesChain`, …                                                                                                                                                                                                                                                                                               | 348 `Series` + 50 `Filter` identifier hits across `internal/`, `cmd/` |
| Store schema                                   | `series_state` → `sequence_state`, `series_occurrence_snapshots` → `sequence_occurrence_snapshots`, plus their indexes. SQLite `ALTER TABLE … RENAME TO` in a new migration                                                                                                                                                                                                                                                                                                                                           | 35 SQL hits                                                           |
| Store data                                     | `blocks.spec_json` holds the block spec as JSON with `type`, `filter`, `series`, `fallback.filler_filter` keys (`internal/store/blocks.go:39,107`). A **data** migration must rewrite every row's JSON, not just the DDL. **ASSUMPTION:** the SQLite driver in use exposes the JSON1 functions (`json_set`/`json_remove`/`json_extract`) so this can be a SQL migration; verify before planning, else it becomes a Go-side migration step, which `golang-migrate`'s SQL-file setup does not currently have a slot for | 1 table, N rows                                                       |
| `scheduler.yaml` import format                 | `type:`, `filter:`, `series:`, `fallback.filler_filter:` keys. Import-only, so no in-place data migration — but `scheduler init`'s template, `testdata/configs/*.yaml`, and `e2e/fixtures/test-scheduler.yaml` all change                                                                                                                                                                                                                                                                                             | 4 fixture files                                                       |
| Web UI                                         | `web/assets/ts/**`, `web/layouts/**` copy, type badges, the `/series/` route (which Stream 3 deletes anyway)                                                                                                                                                                                                                                                                                                                                                                                                          | 185 case-insensitive hits                                             |
| Docs                                           | `scheduling-concepts.md` (the concept page, rewritten), `api-reference.md`, `cli-reference.md`, `web-ui-guide.md`, `metadata.md`, `architecture.md`, `README.md`, `PRODUCT.md`                                                                                                                                                                                                                                                                                                                                        | 104 `series` + 49 `filter` hits                                       |

Under the house no-legacy policy this is a hard swap: no aliases, no dual
acceptance, no redirect stubs. It therefore **must** land before the v0.9.0
freeze, and — see §4 — before the media-sequence work, so the generalized
concept is authored once under its final name.

---

## Stream 2 — Schedule any Tunarr media kind

### 2.1 What filter blocks do for movies **today** (verified, not assumed)

- **The candidate pool already contains movies.** `Runner.fetchPrograms` pulls
  every programme from every library of every media source
  (`internal/service/schedule.go:585-655`: `fetchLibraryPrograms` →
  `SearchPrograms` per library, all pages), falling back to an unscoped search.
  Nothing filters by `Library.MediaType`, so movie libraries are in the pool.
- **`matchesFilter` matches on five things and nothing else**
  (`internal/scheduler/filter.go:30-49`): the compiled title regex, `Genres`
  (any-of, case-insensitive, against `Program.GetGenreNames()` — the raw
  strings the source scraper wrote), `Ratings` (any-of), the year range, and
  the duration range in minutes.
- **So a movie already airs in a filter block whenever it matches.** A block
  with `min_duration: 80` on a mixed library is de-facto movies-only today.
  That is a heuristic, not a guarantee.

Six gaps follow, each verified:

| #   | Gap                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Evidence                            |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| G1  | **No media-kind criterion.** `tunarr.Program.Type` carries `movie`/`episode`/… (`internal/external/tunarr/models.go:76`) and `matchesFilter` never reads it. There is no way to say "movies only".                                                                                                                                                                                                                                                                                                            | `filter.go:30-49`, `types.go:24-33` |
| G2  | **`filter.tags` is dead code, and the docs claim otherwise.** `Filter.Tags` exists in Go (`types.go:32`), CUE (`config.cue:143`), and OpenAPI (`openapi.yaml:336`), and round-trips through `internal/api/blocks.go:473,592,629`. `matchesFilter` never evaluates it: a block with `tags: [x]` behaves exactly as if the field were absent. `docs/scheduling-concepts.md:39,49` documents it as working AND-logic. The roadmap's "nothing populates tags today" understates it — nothing *reads* them either. | `filter.go:30-49`                   |
| G3  | **Genres are raw.** `internal/metadata`'s canonical vocabulary is unwired (`docs/metadata.md`'s own admonition); `f.Genres` is compared against whatever the source library published.                                                                                                                                                                                                                                                                                                                        | `filter.go:35`                      |
| G4  | **No order, no cursor for non-episodic content.** Filter content is shuffled with global `rand` (`engine.go:1246`) and frozen verbatim after first commit (`docs/scheduling-concepts.md:142`). Nothing can express "Movie A this week, Movie B next".                                                                                                                                                                                                                                                         | `engine.go:1199-1301`               |
| G5  | **No movie discovery endpoint.** `GET /media/shows` groups only `Type == "episode"` programmes — its own doc says "a movie has no show to belong to" (`internal/service/media.go`). There is no `/media/movies`, so a UI cannot offer a movie picker. `GET /media/meta` does already return the distinct genre and rating values across movies *and* episodes.                                                                                                                                                | `openapi.yaml:241-263`              |
| G6  | **The metadata `Provider` interface is show-only.** `LookupShow(ctx, title, year)` is the whole interface (`internal/metadata/types.go:64-75`); both clients implement TV endpoints only (`/search/tv` + `/tv/{id}`; `/search?type=series` + `/series/{id}/extended`).                                                                                                                                                                                                                                        | `docs/metadata.md`                  |

### 2.2 Convergence: the v0.4 metadata theme and this stream are one thing

The roadmap's v0.4.0 theme ("enrich what the filters and the guide can see")
and the operator's "movie tagged Funny" request are the same work seen from two
ends. Four layers, in strict dependency order:

**L1 — Enrichment pipeline (closes G6, completes v0.4's "wiring that calls a
provider at all").**

- `Provider` gains a movie lookup. Recommended interface shape: one method with
  a kind, not two parallel methods —

  ```go
  type Kind string // "show" | "movie"

  type Provider interface {
      Name() string
      Lookup(ctx context.Context, kind Kind, title string, year int) (*Metadata, error)
  }
  ```

  `ShowMetadata` becomes `Metadata` with a `Kind` field. Rationale: the
  canonical genre vocabulary is already "TMDB's movie genre list plus the
  television-only entries" (`docs/metadata.md`), so one vocabulary serves both
  kinds, and one method keeps the enrichment pass from branching per kind. The
  two sentinels (`ErrNotFound` skip, `ErrUnauthorized` abort) are unchanged.
  **ASSUMPTION:** TheTVDB v4's movie routes are adequate for the fields
  `Metadata` carries; if not, the TVDB client returns `ErrNotFound` for
  `KindMovie` and TMDB is the movie provider. Verify against the v4 API before
  implementing.
- **Store:** new `media_metadata` table (migration `0000NN_media_metadata`),
  keyed by `(kind, title, year)` — *not* by Tunarr programme UUID. The join key
  the engine actually holds at filter time is the programme's title and year
  (`Program.Title`, `Program.GetYear()`), and a UUID key would store one row per
  episode instead of one per show. Columns: normalized genres (JSON array),
  normalized rating, provider name, external IDs (JSON), `fetched_at`.
- **Provider cache:** the roadmap's remaining v0.4 item (on-disk cache; only
  TMDB's genre table is cached today, in memory).
- **Key sourcing:** unchanged policy — keys arrive from the environment at
  wiring time, never from a config file (`docs/metadata.md`, "Key sourcing").
  The config schema carries provider *enablement* and non-secret options only.
- **The pass:** iterate the same `fetchPrograms` pool, one lookup per distinct
  `(kind, title, year)`, skip on `ErrNotFound`, abort on `ErrUnauthorized`.
  Trigger: on demand (an endpoint) plus the maintenance loop. Sizing note: a
  400-show library is 400 lookups, which the httpclient's 429-backoff already
  paces.

**L2 — Normalized criteria in the engine (closes G3, closes v0.4's felt gap).**
`matchesFilter` consults `media_metadata` for the programme's normalized genres
and rating, so `f.Genres` finally means the canonical vocabulary. Rating
normalization (v0.4's other remaining half) lands here. Precedence between
enriched and raw values is Open Question **Q6**.

**L3 — Tags (closes G2, delivers "tagged Funny").** A `media_tags` table
(`kind, title, year, tag`), an operator-editable vocabulary in the UI, and —
the part that is missing today, not merely unpopulated — an actual `Tags` branch
in `matchesFilter` implementing the documented AND semantics. Until L3 ships,
`docs/scheduling-concepts.md`'s tags rows are wrong and should be corrected in
place (see Q7).

**L4 — Media-kind criterion (closes G1).** `Criteria.kinds: []string`, values
drawn from `tunarr.Program.Type`. Additive to OpenAPI and CUE. With L3 and L4,
the operator's example is one rotation block and no new concept:

```yaml
- name: "Funny Tuesdays"
  type: rotation
  cron: "0 20 * * 2"
  duration: 120
  channel_id: "…"
  criteria:
    kinds: ["movie"]
    tags: ["funny"]
```

### 2.3 Media sequences: the Sequence concept generalized

A series block today is a per-show cursor keyed by `show_title`, ordered by
`(season, episode)`, with `on_complete` ∈ {continue, restart, disable} and
`max_runs`, plus per-occurrence pre/post snapshots. **A movie sequence is the
same machine with a different ordering domain**: an explicit ordered list of
titles, cursor = index into that list. That is the whole idea; everything below
is the delta needed to express it.

**Spec (contract level).** One config shape with a variant, not a second
sibling list:

```text
SequenceConfig {
  # exactly one of:
  show_title?:  string        # episodic: ordered by (season, episode)
  items?:       MediaRef[]    # ordered list: ordered by position
  list_id?:     string        # stable identity for the items variant (see Q8)

  items_per_occurrence: int   # today's episodes_per_block, renamed for kind-neutrality
  start_at?:    {season, episode} | int   # today's start_season/start_episode, or a list index
  on_complete?: continue | restart | disable   # unchanged vocabulary
  max_runs?:    int                            # unchanged
  skip?:        string[]                       # today's skip_episodes, kind-neutral
}
MediaRef { kind, title, year? }
```

Rationale for one shape: one `on_complete` vocabulary, one fallback machinery,
one snapshot/provenance path, one UI. A parallel `list:` block field would fork
all four.

**State store.** `sequence_state`'s primary key is `show_title` today
(`000001_initial_schema.up.sql`), and v0.3.0 already plans re-keying to
`(channel_id, show_title)` (roadmap; TODO.md's deferred item). A movie list has
no show title. Proposal — do both re-keys in one migration:

- key `(channel_id, sequence_key)`, where `sequence_key` is the show title for
  the episodic variant and the stable `list_id` for the items variant;
- add `kind` (episodic | list) and `cursor_index` (meaningful for lists;
  `current_season`/`current_episode` stay meaningful for episodic).

Migration name: `0000NN_sequence_state`. It is one migration doing three things
(rename, re-key, widen) — acceptable because all three are breaking and pre-1.0,
and splitting them means migrating the same table three times.

**Snapshots.** `OccurrenceSnapshot.PreStates`/`PostStates` are
`map[string]SeriesStateSnapshot` serialized into
`series_occurrence_snapshots.snapshot_json` (`internal/scheduler/state.go:67-114`).
The map key becomes `sequence_key`; the value gains `cursor_index`. Because
`snapshot_json` is opaque JSON, this is a decode-shape change and not a column
change, and the existing legacy tolerance (a nil `PostStates` marks a
pre-000006 row that contributes no advance) is the precedent for handling rows
written under the old key shape.

**Two engine primitives fork; the rest do not.**

- `cursorBehind` (`engine.go:1154`) compares `(season, episode)`. A list cursor
  needs an index comparison. This is the single ordering primitive that must
  become kind-aware — and it matters, because the backward-move CAS guard in
  `syncPostStates` (`engine.go:1128-1133`) is built on it.
- `findNextSeriesEpisode` (`engine.go:1537`) gains a sibling
  `findNextListItem`; `planSeriesForConfig` dispatches on the variant;
  `initializeSeriesState`'s start-position handling (`engine.go:1524`) takes an
  index for lists.

Everything else carries unchanged, because it keys off the state row rather than
the episode numbering: plan-sequence provenance (`PlanSeq` vs `CursorPlanSeq`),
the operator-wins stamp (`OperatorUpdatedAt`), the backward-move baseline
agreement, per-occurrence freeze-on-air, and `on_complete`/`max_runs`.

**One thing that must not be forgotten.** `blockReferencesShow`
(`internal/store/sqlite.go:444`) scans `spec.Series` for a show title, and
`InvalidateSeriesOccurrenceSnapshots` uses it to decide which blocks' pending
occurrences to invalidate on an operator cursor write. It must learn the items
variant, or a cursor edit on a movie list will silently fail to invalidate its
pending occurrences and the edit will appear not to take.

**"Monthly runs" needs nothing new.** `cron: "0 20 1 * *"` already gives one
occurrence a month, and the advance is already one step per occurrence.

### 2.4 Contract deltas (design level)

| Surface                                        | Delta                                                                                                                                                    |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Criteria` (was `Filter`)                      | `+ kinds: string[]`; `tags` becomes load-bearing rather than ignored                                                                                     |
| `SequenceConfig` (was `SeriesConfig`)          | `+ items: MediaRef[]`, `+ list_id`; `episodes_per_block` → `items_per_occurrence`; `start_season`/`start_episode` → `start_at`; `skip_episodes` → `skip` |
| `SequenceState` (was `SeriesState`)            | `+ kind`, `+ cursor_index`, `+ channel_id`; `show_title` → `sequence_key`                                                                                |
| `GET /media/movies` (new)                      | `MediaMovie {title, year, duration_ms}` — the symmetric counterpart of `/media/shows`, required for a movie picker                                       |
| `GET /media/meta`                              | unchanged (already spans movies and episodes)                                                                                                            |
| `POST /metadata/refresh` (new)                 | trigger an enrichment pass; `202` + a run summary                                                                                                        |
| `GET /metadata/{kind}/{title}` (new, optional) | read one enriched record, for the UI's tag editor                                                                                                        |
| `GET/PUT /tags` (new)                          | operator tag vocabulary and per-title assignment                                                                                                         |
| Migrations                                     | `0000NN_media_metadata`, `0000NN_media_tags`, `0000NN_sequence_state` (rename + re-key + widen)                                                          |
| Config schema                                  | provider enablement + cache options; **no keys in config**                                                                                               |

---

## Stream 3 — `/series` becomes `/history`

### 3.1 What exists today (verified)

- **`/series/`** is one table of `SeriesState` rows with inline cursor editing
  via `PATCH /state/series/{show_title}` (`web/layouts/series/list.html`,
  `openapi.yaml:191-215`). It shows cursor, completed, disabled, run count,
  last aired.
- **`GET /history?days=N`** returns `HistoryEntry {program_id, channel_id,
  block_name, scheduled_at}` — UUID-headed, no title — even though migration
  `000003` already stores `title`, `type`, `duration_ms`, `occurrence_start`,
  and `sequence` on every row (`internal/api/history.go:61-73`).
- **Retention is one global knob.** `maintenance.history_retention` (default
  `168h`, `cmd/schema/config.cue:81`) prunes `schedule_history` *and*
  `series_occurrence_snapshots` on every successful apply
  (`engine.go:486-490`). So `?days=90` returns seven days of rows unless the
  operator raises the setting — already documented at
  `docs/scheduling-concepts.md:278`.
- **Nothing deletes by title or by range.** There is no `DeleteSeriesState`, no
  per-title history delete, and no date-range delete anywhere in
  `internal/store/` (full method census run 2026-08-30).
- **The v0.5 spec already plans `/log/`** (§2, §3.7): enriched airings + apply
  runs + persisted warnings, `GET /applies`, `HistoryEntry` gains `title, type,
  duration_ms, occurrence_start, sequence, run_id`, `Warning` gains
  `channel_id` and `duration_minutes`, 90-day run retention.

### 3.2 Recommendation: one page, `/history/`, absorbing `/log/`

The industry itself models this as one record in two states: a **traffic/station
log** of what is scheduled, and an **as-run log** of what actually aired.

> "A copy of the log after the fact (sometimes called an AsRun Report) is used…
> to determine what actually aired."
> — [Wikipedia, *Traffic (broadcasting)*](https://en.wikipedia.org/wiki/Traffic_(broadcasting))

That is the operator's ask verbatim ("one searchable place showing what is
scheduled and what has aired"). Two pages would force a guess about which one
holds the answer, and both are backed by the same rows (`schedule_history`
joined to the coming `apply_runs`) with the same filters (window, channel,
block, title).

**Shape.** One route, three panes behind a segmented control, one shared filter
toolbar, deep-linkable by query (`/history/?view=asrun&block=…`):

| Pane        | Content                                                                                                                                               | Replaces                              |
| ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| **TRACKED** | One row per sequence (show or movie list): cursor, completed/disabled, run count, last aired. Edit cursor, reset, **remove**. Searchable by title.    | the whole `/series/` page             |
| **AS-RUN**  | Chronological airings grouped by day: local time, channel plate, block link, programme title + SxxEyy. Search across scheduled and played.            | the v0.5 spec's Log "AIRED" rows      |
| **RUNS**    | Apply-run cards: timestamp, source badge (UI/CRON/CLI), scope, counts, expandable persisted warnings naming loser, winner, and would-have-aired time. | the v0.5 spec's Log "APPLY RUN" cards |

Nav becomes `GUIDE · BLOCKS · HISTORY` once draft-and-apply kills `/schedule/`
— three items, room for a fourth later. `/series/`, `/dashboard/`, and the
planned `/log/` route all cease to exist (no-legacy: no redirect stubs; the 404
page carries the nav).

The v0.5 spec's §2 and §3.7 are amended accordingly, and its "series desk"
slice becomes the History desk's power-tools slice on the same route.

**The alternative, kept on the table (Q4):** keep `/log/` for runs and warnings
(a record about the *system*) and make `/history/` the title-centric desk (a
record about the *content*). The argument for it is that a run card and an
airing row answer different questions and mixing them makes the feed noisy. It
is ranked second because the operator asked for one place, and because the
segmented control already separates the two without a second route.

### 3.3 Removal semantics vs. the snapshot/provenance engine

"Remove a show/movie from history entirely" intersects the occurrence-snapshot
replay machinery in five places. Each is a verified invariant a deletion path
must respect, not a hypothetical.

**I1 — Pending snapshots resurrect a deleted cursor.**
`establishSeriesChain` seeds the chain from the occurrence's stored `PreStates`
when a snapshot exists (`engine.go:783-789`). Deleting the `sequence_state` row
alone leaves every not-yet-aired occurrence's snapshot still carrying the title,
so the very next apply re-derives it at the old cursor and re-creates the row.
**Requirement:** the removal must invalidate every not-yet-*finished* snapshot
for every block referencing the title, exactly as
`InvalidateSeriesOccurrenceSnapshots` does today
(`internal/store/sqlite.go:406-440`), using `InvalidationCutoff`
(`sqlite.go:401`) so an occurrence currently on air is included rather than
excluded by an `occurrence_start > now` comparison.

**I2 — Aired snapshots resurrect it too, through `syncPostStates`.**
An aired occurrence replays its stored `PostStates` into `pendingStates`, and
`Commit` writes them with `UpdateSeriesState`, which is an upsert
(`sqlite.go:155-160`) — so a deleted row comes back. Neither existing guard
helps: `OperatorUpdatedAt` and `CursorPlanSeq` both live *on the row that was
just deleted* (`engine.go:1122-1127`). **Requirement:** the removal must also
strip the title from aired snapshots' `PreStates`/`PostStates` maps. Deleting
those snapshot rows wholesale would cost *the other titles in the same
occurrence* their verbatim replay, because a snapshot is per-occurrence and not
per-title — so scrubbing the map key is the correct operation. Decision surfaced
as **Q5**.

**I3 — Deleting `schedule_history` rows rewrites the past lineup.**
`GetCommittedOccurrence` reconstructs an aired occurrence's content from those
rows (`sqlite.go:229`), and the engine replays it verbatim, with an explicit
documented behavior when it has been pruned: the occurrence contributes empty
content, because "what actually aired is historical fact a later apply cannot
invent" (`engine.go:715-717`). Since an apply is a **full channel replacement**
(`docs/scheduling-concepts.md`, Channel ownership), removing a title's rows makes
the next apply push a *shorter* past window for that channel. In the finished
past that is cosmetic. For the **on-air** occurrence it is not: it changes what
is playing right now. **Requirement:** either refuse a removal that intersects
a currently-on-air occurrence, or exclude that occurrence's rows from the
delete. Decision surfaced as **Q9**.

**I4 — State, snapshots, and history must go together or the removal looks
broken.** `establishSeriesChain`'s case 2 (`engine.go:791-801`) triggers when the
snapshot is gone but `GetCommittedOccurrence` still has rows: the occurrence is
treated as already-planned and is *not* re-planned, seeding the chain from live
state instead. After a removal, live state is `GetSeriesState`'s fabricated
S01E01 default (`sqlite.go:117-134`) — which is exactly the desired re-add-later
behavior *if* the history rows are gone too. If they survive, the occurrence
stays committed and the operator sees no effect. **Requirement:** one
transaction across `sequence_state`, the snapshot scrub, and `schedule_history`.
Note that `Engine.Commit` is itself not transactional today (TODO.md, v0.3.0
review, MAJOR-class); the deletion path must not inherit that shape.

**I5 — Deleting rows can lower the plan-sequence floor.**
`MaxPlanSeq` derives the engine's allocator floor at construction from
`MAX(snapshots.plan_seq, series_state.cursor_plan_seq)` (`sqlite.go:457-468`).
Delete the rows holding that maximum and the floor drops on the next restart; a
newly allocated sequence can then be ≤ a surviving row's `CursorPlanSeq`, and
`syncPostStates`'s provenance guard (`snap.PlanSeq <= existing.CursorPlanSeq`,
`engine.go:1125`) silently drops a legitimate replay **for an unrelated title**.
**Requirement, and a prerequisite for both deletion features:** persist the
allocator floor monotonically instead of deriving it. `app_meta` (migration
`000009`) already exists precisely as the store for "a value that only ever
moves forward" and is the natural home (key `max_plan_seq`).

### 3.4 Retention and cleanup

**What actually grows.** `schedule_history` and `series_occurrence_snapshots`
are both pruned to `history_retention` on every successful apply
(`engine.go:486-490`), so at the 168h default they are bounded and small. The
coming `apply_runs` table gets its own 90-day prune. `sequence_state` is one row
per tracked title and does not grow meaningfully. **The honest statement to put
in the docs: the DB is already bounded; the cleanup tools are for taking a
specific thing out, and for operators who raised `history_retention` to make
`?days=90` useful and now want the space back.**

**Endpoint.** `DELETE /api/v1/history` with mutually exclusive window forms and
optional narrowing:

```text
DELETE /history?before=<RFC3339>[&channel_id=&block_name=&title=][&dry_run=true]
DELETE /history?from=<RFC3339>&to=<RFC3339>[&…][&dry_run=true]
  200 → {deleted: n, dry_run: bool}
  400 Problem on both/neither window form, or from > to
  409 Problem when the range intersects a currently-on-air occurrence (per Q9)
```

`dry_run` returning counts only mirrors the house import pattern
(`POST /blocks/import`, and the v0.5 spec's series import). Per-title removal
gets its own route so it reads as one action rather than a crafted range:
`DELETE /api/v1/state/sequences/{key}?purge_history=bool`.

Both paths run the I1–I5 guards in one transaction.

**UX.** A `STORAGE` strip on `/history/`: row counts per table, the effective
retention window, and two actions — `PRUNE TO RETENTION` (runs the existing
policy now instead of waiting for the next apply) and `DELETE RANGE` (the shared
confirm dialog naming the exact count from the dry run). Removal from the
TRACKED pane is a row action with the same confirm, naming what it takes with
it. Both confirms say plainly that this is unbackfillable — the same honesty the
Log's empty state already carries.

---

## 4. Ladder placement

### 4.1 A correction first: v0.5.4 already shipped, and it was not draft-and-apply

The v0.5 spec's renumber note (§9) shifted draft-and-apply to v0.5.4 and Memory
to v0.5.5. Since then a **third** operator-directed insertion shipped:
`[0.5.4] - 2026-08-30` in the CHANGELOG is the theme-fit brand mark
(commit `a89eb31`), which did not touch `docs/roadmap.md`. So the ladder is one
number further along than both the roadmap's "v0.5.4+ — pending" line and
`web/layouts/partials/nav.html`'s comments claim. This plan records that
insertion and shifts the pending slices by one more (Q2 offers the operator the
alternative of re-cutting instead).

### 4.2 Proposed ladder

| Number     | Theme                                                       | Change from today's plan                                                                                                                                                                                         |
| ---------- | ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| v0.5.5     | Draft & apply on the Guide                                  | shifted +1 by the brand-mark insertion                                                                                                                                                                           |
| **v0.5.6** | **Memory, landing as `/history/`**                          | the spec's Memory slice, amended by Stream 3: the page ships as `/history/` with TRACKED/AS-RUN/RUNS panes; `/series/` and `/dashboard/` deleted; nav becomes `GUIDE · BLOCKS · HISTORY`                         |
| v0.5.7     | Live link (SSE)                                             | shifted +1                                                                                                                                                                                                       |
| v0.5.8     | Block power tools                                           | shifted +1                                                                                                                                                                                                       |
| **v0.5.9** | **History desk power tools** (was "series desk")            | bulk cursor ops, import/export, **remove-from-history**, **range cleanup**, the STORAGE strip. Engine-first: the monotonic plan-seq floor in `app_meta` (I5) and the transactional deletion path (I4)            |
| v0.5.10    | Polish pass                                                 | shifted +1                                                                                                                                                                                                       |
| v0.4.x     | **Metadata engine + any-media criteria** — Stream 2's L1–L4 | the existing v0.4 theme, restructured into ordered slices: enrichment wiring → normalized genres/ratings in the engine → tags (with the missing `matchesFilter` branch) → media-kind criterion + `/media/movies` |
| **v0.6.0** | **Station terminology** — Stream 1's hard rename            | **new**; the old v0.6 moves to v0.7.0                                                                                                                                                                            |
| **v0.6.1** | **Media sequences** — Stream 2's §2.3                       | **new**; lands after the rename so the generalized concept is born as `Sequence`                                                                                                                                 |
| v0.7.0     | Operations and observability                                | renumbered from v0.6.0                                                                                                                                                                                           |
| v0.9.0     | Freeze candidate                                            | unchanged; the rename is upstream of it by construction                                                                                                                                                          |

### 4.3 Why the rename gets its own minor, and sits there

- It is a pure breaking change across the wire, the config schema, the import
  format, and the DB, with a data migration over `blocks.spec_json`. That
  deserves its own CHANGELOG headline rather than hiding inside a feature slice.
- It must precede v0.9.0 — the operator's only hard constraint.
- It should **follow** the v0.5 web train, because every remaining v0.5 slice
  touches UI copy and type badges; renaming mid-train doubles the churn on a
  train already five slices deep.
- It should **precede** the media-sequence work (v0.6.1), so the generalized
  concept is authored once under its final name instead of being renamed twice.
- Stream 2's L1–L4 filter-side work is not gated on it: those slices add fields
  to whatever the criteria object is called at the time, and the v0.6.0 sweep
  renames them with everything else. If v0.4.x ships first, it ships in today's
  vocabulary; that is a cost of one extra rename hop on a handful of new field
  names, and it is cheaper than blocking the metadata work on a repo-wide sweep.

**Alternative considered and rejected:** folding the rename into the v0.5 polish
slice. It would drag a store migration, an OpenAPI break, and a CLI change into
a UI polish slice, and would present a breaking change to the operator under the
word "polish".

---

## 5. Open Questions for the Operator

**Q1 — The `filter` block-type replacement.** Ranked in §1.4:
`rotation` (recommended — real radio-automation term, contrasts cleanly with
`sequence`, one word; cost: radio-native, and it implies stronger repeat
protection than the engine's soft history guard actually gives),
`selection` (matches your phrasing exactly; no industry attestation),
`category` (real, but collides with Stream 2's genre/tag categories in the same
train), `search`/`smart` (ErsatzTV's word; names the mechanism, not the
programming), `random` (Tunarr's own word; describes only the ordering and reads
like a defect in an enum). Pick one, or veto all five.

**Q2 — The v0.5.4 correction.** v0.5.4 shipped as the brand-mark slice, so
draft-and-apply and everything after it move one number down (§4.1). Confirm the
shift, or say you would rather re-cut the brand-mark release as v0.5.3.1 and
keep the spec's numbering.

**Q3 — Rename depth for the criteria object.** Recommended: `filter:` →
`criteria:` and `fallback.filler_filter` → `fallback.filler_criteria`, so the
word "filter" leaves the vocabulary entirely (no-legacy). Alternative: rename
only the block *type* and keep `filter:` as the field name, which is a smaller
diff but leaves the confusing word in the schema you just renamed away from.

**Q4 — `/history` vs `/log`.** Recommended: one page at `/history/` with
TRACKED / AS-RUN / RUNS panes, absorbing the v0.5 spec's planned `/log/` (§3.2).
Alternative: two pages — `/history/` title-centric, `/log/` for runs and
warnings. Confirm the merge, since it amends the v0.5 spec's §2 and §3.7 and
deletes a route that was already signed off.

**Q5 — Deletion depth in aired snapshots.** Recommended: **scrub the title's
key** out of aired occurrences' `PreStates`/`PostStates` maps, preserving the
other titles' verbatim replay in the same occurrence. Alternative: delete those
snapshot rows outright — simpler code, but it costs every co-scheduled title in
that occurrence its exact replay (I2).

**Q6 — Enriched vs raw genre precedence.** Recommended: prefer the normalized
genres from `media_metadata` and **fall back** to Tunarr's raw genres when a
title has not been enriched — no regression for a library the pass has not
reached. Alternative: enriched-only, which is predictable but silently empties
every genre filter until the first pass completes.

**Q7 — The dead `tags` field.** `filter.tags` is accepted everywhere and
evaluated nowhere (G2), and `docs/scheduling-concepts.md` documents it as
working. Recommended: correct the doc now as a patch (it is a factual error
today), and implement the criterion with the tag engine in v0.4.x. Alternative:
implement it immediately against operator-typed tags with no metadata backing.

**Q8 — Movie-sequence identity.** Recommended: a stable `list_id` minted at
block create and carried in the spec, so the list's cursor survives a block
rename. Alternative: derive identity from the block ID — simpler, and block IDs
never change, but duplicating a block (a v0.5.8 feature) would then either share
one cursor or need a special case.

**Q9 — On-air guard for deletions.** Recommended: **refuse** a removal or range
delete that intersects a currently-on-air occurrence (`409`), telling the
operator when it will be safe. It is the one case where deleting history changes
what is playing right now (I3). Alternative: allow it behind a loud confirm.

**Q10 — Retention granularity.** Today one knob (`history_retention`) governs
both `schedule_history` and `series_occurrence_snapshots`, and the coming
`apply_runs` gets a fixed 90 days. Keep the single knob, or split it per table
now, while the config schema is still breakable?

**Q11 — "Sequence" and the editing-term collision (FYI, not a re-open).**
Research flags that in film and video a *sequence* is an edited run of shots,
which is the first meaning a media-literate reader reaches. Tunarr avoids it by
using "sequential" as an adjective and "iterator" as the noun. Your choice
stands; the mitigation is that UI and docs always say "sequence block", never a
bare "sequence" as a content noun. Confirm that mitigation, or accept the bare
noun.
