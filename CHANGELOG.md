# Changelog

All notable changes to Schedularr will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.1] - 2026-08-29

Two bugs found during the first live `--apply` against a real Tunarr
1.3.13 instance and this deployment's configured (non-UTC) timezone.

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
  now POSTs `{"type": "manual", "lineup": [{"type": "content", "id":
  ..., "duration": ...}, ...], "append": false}`
  (`types/src/api/index.ts`'s `ManualProgramLineupSchema`), matching
  what `server/src/db/channel/LineupRepository.ts`'s `updateLineup`
  actually consumes. `internal/service/schedule_test.go`'s fake Tunarr
  server now mirrors live semantics (404 on PUT, decodes and validates
  the POST body) so a regression back to PUT fails a test instead of
  silently passing.
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

[Unreleased]: https://github.com/christopherime/schedularr/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/christopherime/schedularr/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/christopherime/schedularr/releases/tag/v0.1.0
