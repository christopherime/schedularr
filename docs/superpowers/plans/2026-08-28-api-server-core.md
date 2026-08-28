# API Server Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn schedularr into a secure, contract-first REST API server (Tunarr-only integrations, blocks in SQLite) with all legacy code removed.

**Architecture:** Single Go binary. `api/openapi.yaml` is the authoritative contract; `oapi-codegen` generates chi server interfaces into `internal/api/gen/`; thin handlers in `internal/api/` call the existing scheduler engine and store. Blocks move from `scheduler.yaml` to a new SQLite table with CUE validation on every write; a new `serve` command co-hosts the cron engine and the HTTP API.

**Tech Stack:** Go 1.27, chi v5, oapi-codegen v2, sqlx+sqlite (existing), CUE (existing), slog (existing).

**Spec:** `docs/superpowers/specs/2026-08-28-api-server-core-design.md`

## Global Constraints

- Go toolchain: `go 1.27` in go.mod (installed: go1.27.0).
- Module path: `github.com/christopherime/schedularr` (renamed in Task 1; every new file imports this path).
- Dev on `main` only, one task at a time, commit at the end of every task, push after every task.
- Conventional Commits messages.
- `make lint` (golangci-lint + gosec + govulncheck) and `make test` (race detector) must pass before every commit.
- No-legacy policy: superseded code, config keys, commands, and go.mod deps are deleted in the same task — no deprecation aliases, no commented-out code.
- Blocked packages (repo policy): `github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`. Use `gopkg.in/yaml.v3` (already a dependency) for YAML.
- Error wrapping style: `fmt.Errorf("failed to <verb>: %w", err)`. Logging via `slog`, snake_case keys.
- API listens on `:8484` by default. Auth: bearer token, min 32 chars, constant-time compare. Unauthenticated paths: `/healthz`, `/readyz`, `/metrics`, `/openapi.json` only.
- Errors to clients: RFC 7807 `application/problem+json` with `request_id`.

---

### Task 1: Module rename, Go 1.27, origin repoint

**Files:**
- Modify: `go.mod` (module line + go directive)
- Modify: every `.go` file importing `github.com/geekxflood/schedularr/...` (mechanical rewrite)
- Modify: `Dockerfile`, `README.md`, `.golangci.yml` (any hardcoded module-path references)

**Interfaces:**
- Consumes: nothing.
- Produces: module `github.com/christopherime/schedularr`; all later tasks import under this path.

- [ ] **Step 1: Repoint origin and confirm clean tree**

```bash
cd /Users/christophe/schedularr
git remote set-url origin git@github.com:christopherime/schedularr.git
git remote -v && git status --short   # expect: new URL, empty status
```

- [ ] **Step 2: Rewrite module path and bump Go**

```bash
go mod edit -module github.com/christopherime/schedularr
go mod edit -go=1.27
grep -rl 'github.com/geekxflood/schedularr' --include='*.go' . \
  | xargs sed -i '' 's|github.com/geekxflood/schedularr|github.com/christopherime/schedularr|g'
grep -rn 'geekxflood/schedularr' --include='*.go' . ; # expect: no output
grep -rn 'geekxflood' README.md Dockerfile .golangci.yml Makefile || true
```

Fix any non-Go references found by the last grep (badges, image labels) to `christopherime`.

- [ ] **Step 3: Verify**

Run: `make build && make test && make lint`
Expected: all pass. If `golangci-lint` complains about the Go version, update its `run.go` setting in `.golangci.yml` to `"1.27"`.

- [ ] **Step 4: Commit and push**

```bash
git add -A
git commit -m "chore: rename module to christopherime/schedularr, require Go 1.27"
git push origin main
```

---

### Task 2: Delete the TUI and all charmbracelet dependencies

**Files:**
- Delete: `internal/tui/` (entire package), `cmd/tui.go`
- Modify: `cmd/root.go` (remove `runTUI` at ~line 112 and the tui command registration in `init()`)
- Modify: `cmd/generate.go`, `cmd/validate.go` (replace `charmbracelet/huh` interactive prompts)
- Modify: `go.mod` (all `charmbracelet/*` deps gone)

**Interfaces:**
- Consumes: nothing.
- Produces: `generate --apply` requires explicit `--yes` when it would prompt; no interactive code remains.

- [ ] **Step 1: Delete TUI package and command**

```bash
git rm -r internal/tui cmd/tui.go
```

Remove from `cmd/root.go`: the `runTUI()` function and any `AddCommand`/case referencing it. Build to find every dangling reference: `go build ./... 2>&1 | head`.

- [ ] **Step 2: Replace huh prompts with a --yes flag**

Inspect prompts: `grep -n 'huh\.' cmd/generate.go cmd/validate.go`. For each confirmation form, replace with this pattern (add the flag once per command):

```go
// in the command's init():
generateCmd.Flags().BoolVar(&assumeYes, "yes", false, "Skip confirmation prompts")

// at the former prompt site:
if !assumeYes {
    return fmt.Errorf("refusing to apply without --yes (interactive prompts were removed)")
}
```

Non-confirmation huh usage (pickers/inputs) becomes a required flag or positional arg with the same error-if-missing pattern. Delete the huh imports.

- [ ] **Step 3: Prune dependencies and verify**

```bash
go mod tidy
grep -n charmbracelet go.mod   # expect: no output
make build && make test && make lint
```

Update `README.md`: remove the `tui` command section.

- [ ] **Step 4: Commit and push**

```bash
git add -A
git commit -m "feat!: remove TUI and interactive prompts (no-legacy policy)"
git push origin main
```

---

### Task 3: Remove Jellyfin, Sonarr, and Radarr integrations

**Files:**
- Delete: `internal/external/jellyfin/`, `internal/external/sonarr/`, `internal/external/radarr/`, `cmd/content_sources.go`
- Modify: `cmd/generate.go` (remove guide-refresh hook + availability filtering wiring)
- Modify: `internal/config/config.go` + `cmd/schema/config.cue` (drop jellyfin/sonarr/radarr config sections)
- Modify: `internal/scheduler/engine.go` / `filter.go` only if they reference *arr availability (verify with grep; the engine consumes `[]tunarr.Program`, so likely untouched)

**Interfaces:**
- Consumes: nothing.
- Produces: `config.Config` carries Tunarr + database + logging only; the engine's inputs are unchanged (`[]tunarr.Program`).

- [ ] **Step 1: Delete packages and command**

```bash
git rm -r internal/external/jellyfin internal/external/sonarr internal/external/radarr cmd/content_sources.go
go build ./... 2>&1 | head -30
```

- [ ] **Step 2: Excise references**

For each build error site (expected: `cmd/generate.go`, `internal/config/config.go`, `cmd/root.go` command registration): delete the jellyfin guide-refresh call, the sonarr/radarr availability-filter plumbing, and the config struct fields/parsing (`getStringFromMap` blocks for those sources in `internal/config/config.go` ~lines 95–125). Remove the matching sections from `cmd/schema/config.cue` so CUE validation rejects the old keys.

- [ ] **Step 3: Fix tests, verify, update docs**

```bash
grep -rln 'jellyfin\|sonarr\|radarr' --include='*.go' . || echo CLEAN
go mod tidy && make build && make test && make lint
```

Fix/delete tests that exercised removed code (e.g. `cmd/content_sources_test.go`). Remove the three integrations from `README.md` and `CLAUDE.md`'s architecture tree.

- [ ] **Step 4: Commit and push**

```bash
git add -A
git commit -m "feat!: remove jellyfin/sonarr/radarr - Tunarr is the sole integration"
git push origin main
```

---

### Task 4: Blocks table + store CRUD

**Files:**
- Create: `internal/store/migrations/000002_blocks.up.sql`, `internal/store/migrations/000002_blocks.down.sql`
- Create: `internal/store/blocks.go`
- Test: `internal/store/blocks_test.go`

**Interfaces:**
- Consumes: `store.New(dsn string) (*Store, error)` (existing), `scheduler.Block` (existing).
- Produces (used by Tasks 6, 7, 11, 16):

```go
type BlockRecord struct {
    ID        string          // uuid string
    Name      string          // unique
    Enabled   bool
    Spec      scheduler.Block // full block definition
    CreatedAt time.Time
    UpdatedAt time.Time
}
func (s *Store) ListBlocks(ctx context.Context) ([]BlockRecord, error)
func (s *Store) GetBlock(ctx context.Context, id string) (*BlockRecord, error)      // nil, ErrNotFound if missing
func (s *Store) CreateBlock(ctx context.Context, rec *BlockRecord) error            // sets timestamps; ErrConflict on dup name
func (s *Store) UpdateBlock(ctx context.Context, rec *BlockRecord) error            // ErrNotFound if missing
func (s *Store) DeleteBlock(ctx context.Context, id string) error                   // ErrNotFound if missing
func (s *Store) CountBlocks(ctx context.Context) (int64, error)
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
```

- [ ] **Step 1: Write the migration**

`internal/store/migrations/000002_blocks.up.sql`:

```sql
-- Blocks move from scheduler.yaml to the store (API-editable source of truth)
CREATE TABLE IF NOT EXISTS blocks (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  enabled    INTEGER NOT NULL DEFAULT 1,
  spec_json  TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
```

`000002_blocks.down.sql`:

```sql
DROP TABLE IF EXISTS blocks;
```

- [ ] **Step 2: Write failing tests**

`internal/store/blocks_test.go` (follow the existing `sqlite_test.go` pattern for opening a temp-file store). Table-driven tests covering: create+get round-trip (Spec JSON survives intact incl. nested `SeriesConfig`), duplicate name → `ErrConflict`, get/update/delete of missing id → `ErrNotFound`, list ordering by name, count, enabled toggle persists.

```go
func TestBlockCRUD(t *testing.T) {
    s := newTestStore(t) // helper: store.New(filepath.Join(t.TempDir(), "test.db"))
    ctx := context.Background()
    rec := &store.BlockRecord{
        ID: "b1", Name: "saturday-night", Enabled: true,
        Spec: scheduler.Block{
            Type: scheduler.BlockTypeSeries, Name: "saturday-night",
            Cron: "0 20 * * 6", Duration: 90, ChannelID: "ch1", Priority: 10,
            Series: []scheduler.SeriesConfig{{ShowTitle: "Show A", EpisodesPerBlock: 1}},
        },
    }
    if err := s.CreateBlock(ctx, rec); err != nil { t.Fatalf("create: %v", err) }
    got, err := s.GetBlock(ctx, "b1")
    if err != nil { t.Fatalf("get: %v", err) }
    if got.Spec.Series[0].ShowTitle != "Show A" { t.Fatalf("spec lost: %+v", got.Spec) }
    if err := s.CreateBlock(ctx, rec); !errors.Is(err, store.ErrConflict) { t.Fatalf("want ErrConflict, got %v", err) }
    if _, err := s.GetBlock(ctx, "nope"); !errors.Is(err, store.ErrNotFound) { t.Fatalf("want ErrNotFound, got %v", err) }
}
```

- [ ] **Step 3: Run tests, verify FAIL** — `go test -race ./internal/store/ -run TestBlock -v` → compile error (methods undefined).

- [ ] **Step 4: Implement `internal/store/blocks.go`** — sqlx queries; marshal `Spec` with `encoding/json`; map `sqlite3.ErrConstraintUnique` (or the `UNIQUE constraint failed` error string, matching how `sqlite.go` handles errors) to `ErrConflict`; `sql.ErrNoRows` to `ErrNotFound`; timestamps `time.Now().UTC()` set in Create/Update.

- [ ] **Step 5: Run tests green, lint** — `go test -race ./internal/store/... -v && make lint`

- [ ] **Step 6: Commit and push**

```bash
git add internal/store
git commit -m "feat(store): add blocks table and CRUD (source of truth moves to sqlite)"
git push origin main
```

---

### Task 5: Block validation + YAML import/export + bootstrap (`internal/blockio`)

**Files:**
- Create: `internal/blockio/blockio.go`, `internal/blockio/bootstrap.go`
- Test: `internal/blockio/blockio_test.go`, `internal/blockio/bootstrap_test.go`

**Interfaces:**
- Consumes: `cueconfig.NewValidator().ValidateScheduler(data []byte, format string) error` (existing), Task 4 store methods, `scheduler.Config{Blocks []Block}` (existing yaml tags).
- Produces (used by Tasks 11, 15, 16):

```go
func ValidateBlocks(blocks []scheduler.Block) error                    // marshal to YAML, ValidateScheduler; wraps CUE errors
func ParseYAML(data []byte) ([]scheduler.Block, error)                 // strict yaml.v3 decode + ValidateBlocks
func RenderYAML(blocks []scheduler.Block) ([]byte, error)              // scheduler.Config wrapper, stable field order
func Bootstrap(ctx context.Context, s *store.Store, path string, logger *slog.Logger) (int, error)
// Bootstrap: if CountBlocks==0 and path exists → ParseYAML, create one BlockRecord per
// block (ID = uuid, Name = block.Name, Enabled = true), log loudly, return count.
// DB non-empty or file missing → (0, nil). Invalid YAML → error (refuse to start half-imported).
```

- [ ] **Step 1: Write failing tests** — round-trip (`ParseYAML(RenderYAML(x)) == x` for a filter block and a series block), CUE rejection (block missing `cron` → error mentioning the field), strict decode (unknown YAML key → error), bootstrap: empty DB + valid file imports N and is idempotent on second call (returns 0); non-empty DB never imports; missing file → (0, nil); invalid file → error.

```go
func TestParseYAMLRejectsMissingCron(t *testing.T) {
    _, err := blockio.ParseYAML([]byte("blocks:\n  - name: x\n    duration: 60\n    channel_id: c\n"))
    if err == nil { t.Fatal("expected CUE validation error") }
}
```

- [ ] **Step 2: Run, verify FAIL** — `go test -race ./internal/blockio/... -v`
- [ ] **Step 3: Implement** — `yaml.v3` with `dec.KnownFields(true)`; uuid via `github.com/google/uuid` (add dep; it is on the allowed side of the policy — tiny, no transitive deps).
- [ ] **Step 4: Run green + lint** — `go test -race ./internal/blockio/... -v && make lint`
- [ ] **Step 5: Commit and push**

```bash
git add internal/blockio go.mod go.sum
git commit -m "feat(blockio): CUE-validated YAML import/export and first-run bootstrap"
git push origin main
```

---

### Task 6: Engine reads blocks from the store

**Files:**
- Modify: `cmd/generate.go` (load blocks via `store.ListBlocks` instead of `config.LoadScheduler`; keep `--scheduler` flag ONLY as import hint removal — delete the flag, ~line 621)
- Modify: `internal/config` (delete `LoadScheduler` and its tests if nothing else uses it)
- Test: adjust `cmd/scheduler_test.go` / config tests

**Interfaces:**
- Consumes: Task 4 `ListBlocks`; Task 5 `Bootstrap`.
- Produces: `loadActiveBlocks(ctx context.Context, s *store.Store) ([]scheduler.Block, error)` in `cmd/generate.go` — filters `Enabled`, maps `BlockRecord.Spec`; reused by Task 16's serve loop.

- [ ] **Step 1: Write the helper + failing test** (test lives beside existing cmd tests; use a temp store, insert 2 records — one disabled — assert only enabled block returned).
- [ ] **Step 2: Wire generate flow** — where `runGenerate` currently builds `[]scheduler.Block` from the YAML config, call `Bootstrap` (so first run imports `scheduler.yaml` if present) then `loadActiveBlocks`. Delete `LoadScheduler` usage; `schedularr validate <file>` stays — it now validates *import* files via `blockio.ParseYAML`.
- [ ] **Step 3: Verify** — `make build && make test && make lint`; manual: `./bin/schedularr generate` against a temp config dir with a `scheduler.yaml` → log shows bootstrap import, plan uses those blocks.
- [ ] **Step 4: Commit and push**

```bash
git add -A
git commit -m "feat!: blocks load from store; scheduler.yaml becomes import-only"
git push origin main
```

---

### Task 7: OpenAPI contract + codegen scaffolding + 501 stubs

**Files:**
- Create: `api/openapi.yaml`, `api/oapi-codegen.yaml`, `tools/tools.go`
- Create: `internal/api/gen/server.gen.go` (generated, committed)
- Create: `internal/api/server.go` (Handlers struct, every operation → 501 problem)
- Create: `internal/api/problem.go`
- Modify: `Makefile` (add `generate` target), `go.mod`
- Test: `internal/api/problem_test.go`

**Interfaces:**
- Consumes: nothing yet.
- Produces (used by Tasks 8–15):

```go
// internal/api/problem.go
type Problem struct {
    Type string `json:"type"`; Title string `json:"title"`; Status int `json:"status"`
    Detail string `json:"detail,omitempty"`; RequestID string `json:"request_id,omitempty"`
}
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string)
// internal/api/server.go
type Deps struct {
    Store  *store.Store
    Tunarr TunarrAPI          // defined in Task 12
    Sched  ScheduleRunner     // defined in Task 13
    Logger *slog.Logger
    Version string
}
type Handlers struct { d Deps }
func NewHandlers(d Deps) *Handlers  // implements gen.ServerInterface
```

- [ ] **Step 1: Write `api/openapi.yaml`** — OpenAPI 3.0.3, exactly these operations (operationIds fixed here; later tasks implement them one group at a time):

```yaml
openapi: 3.0.3
info: { title: Schedularr API, version: 1.0.0 }
servers: [ { url: /api/v1 } ]
security: [ { bearerAuth: [] } ]
paths:
  /blocks:
    get:  { operationId: listBlocks, responses: { "200": { $ref: "#/components/responses/BlockList" } } }
    post: { operationId: createBlock, requestBody: { $ref: "#/components/requestBodies/BlockWrite" },
            responses: { "201": { $ref: "#/components/responses/BlockItem" }, "400": { $ref: "#/components/responses/Problem" }, "409": { $ref: "#/components/responses/Problem" } } }
  /blocks/{id}:
    parameters: [ { name: id, in: path, required: true, schema: { type: string } } ]
    get:    { operationId: getBlock, responses: { "200": { $ref: "#/components/responses/BlockItem" }, "404": { $ref: "#/components/responses/Problem" } } }
    put:    { operationId: updateBlock, requestBody: { $ref: "#/components/requestBodies/BlockWrite" },
              responses: { "200": { $ref: "#/components/responses/BlockItem" }, "400": { $ref: "#/components/responses/Problem" }, "404": { $ref: "#/components/responses/Problem" } } }
    delete: { operationId: deleteBlock, responses: { "204": { description: deleted }, "404": { $ref: "#/components/responses/Problem" } } }
  /blocks/import:
    post: { operationId: importBlocks, requestBody: { required: true, content: { application/yaml: { schema: { type: string } } } },
            parameters: [ { name: dry_run, in: query, schema: { type: boolean, default: false } } ],
            responses: { "200": { description: result, content: { application/json: { schema: { $ref: "#/components/schemas/ImportResult" } } } }, "400": { $ref: "#/components/responses/Problem" } } }
  /blocks/export:
    get: { operationId: exportBlocks, responses: { "200": { description: yaml, content: { application/yaml: { schema: { type: string } } } } } }
  /generate:
    post: { operationId: generateSchedule, requestBody: { content: { application/json: { schema: { $ref: "#/components/schemas/GenerateRequest" } } } },
            responses: { "200": { description: plan, content: { application/json: { schema: { $ref: "#/components/schemas/PlanResult" } } } }, "502": { $ref: "#/components/responses/Problem" } } }
  /apply:
    post: { operationId: applySchedule, requestBody: { content: { application/json: { schema: { $ref: "#/components/schemas/GenerateRequest" } } } },
            responses: { "200": { description: applied plan, content: { application/json: { schema: { $ref: "#/components/schemas/PlanResult" } } } }, "502": { $ref: "#/components/responses/Problem" } } }
  /schedule:
    get: { operationId: getSchedule, parameters: [ { name: days, in: query, schema: { type: integer, default: 7, minimum: 1, maximum: 30 } } ],
           responses: { "200": { description: plan, content: { application/json: { schema: { $ref: "#/components/schemas/PlanResult" } } } }, "502": { $ref: "#/components/responses/Problem" } } }
  /history:
    get: { operationId: getHistory, parameters: [ { name: days, in: query, schema: { type: integer, default: 7, minimum: 1, maximum: 90 } } ],
           responses: { "200": { description: history, content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/HistoryEntry" } } } } } } }
  /state/series:
    get: { operationId: listSeriesState, responses: { "200": { description: states, content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/SeriesState" } } } } } } }
  /state/series/{show_title}:
    parameters: [ { name: show_title, in: path, required: true, schema: { type: string } } ]
    patch: { operationId: patchSeriesState, requestBody: { required: true, content: { application/json: { schema: { $ref: "#/components/schemas/SeriesStatePatch" } } } },
             responses: { "200": { description: updated, content: { application/json: { schema: { $ref: "#/components/schemas/SeriesState" } } } }, "404": { $ref: "#/components/responses/Problem" } } }
  /channels:
    get: { operationId: listChannels, responses: { "200": { description: channels, content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/Channel" } } } } }, "502": { $ref: "#/components/responses/Problem" } } }
  /status:
    get: { operationId: getStatus, responses: { "200": { description: status, content: { application/json: { schema: { $ref: "#/components/schemas/Status" } } } } } }
components:
  securitySchemes: { bearerAuth: { type: http, scheme: bearer } }
  requestBodies:
    BlockWrite: { required: true, content: { application/json: { schema: { $ref: "#/components/schemas/BlockWrite" } } } }
  responses:
    Problem:  { description: error, content: { application/problem+json: { schema: { $ref: "#/components/schemas/Problem" } } } }
    BlockList: { description: blocks, content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/BlockRecord" } } } } }
    BlockItem: { description: block, content: { application/json: { schema: { $ref: "#/components/schemas/BlockRecord" } } } }
  schemas:
    Problem:
      type: object
      required: [type, title, status]
      properties: { type: {type: string}, title: {type: string}, status: {type: integer}, detail: {type: string}, request_id: {type: string} }
    Filter:
      type: object
      properties:
        title_pattern: {type: string}
        genres: {type: array, items: {type: string}}
        ratings: {type: array, items: {type: string}}
        year_from: {type: integer}
        year_to: {type: integer}
        min_duration: {type: integer}
        max_duration: {type: integer}
        tags: {type: array, items: {type: string}}
    FillerConfig:
      type: object
      properties: { enabled: {type: boolean}, filler_list_id: {type: string}, max_filler_time: {type: integer}, min_gap_time: {type: integer} }
    SeriesConfig:
      type: object
      required: [show_title, episodes_per_block]
      properties:
        show_title: {type: string}
        episodes_per_block: {type: integer, minimum: 1}
        start_season: {type: integer}
        start_episode: {type: integer}
        on_complete: {type: string, enum: [continue, restart, disable]}
        skip_episodes: {type: array, items: {type: string}}
        max_runs: {type: integer}
    SeriesFallback:
      type: object
      properties: { mode: {type: string, enum: [redistribute, filler]}, filler_filter: { $ref: "#/components/schemas/Filter" } }
    BlockSpec:
      type: object
      required: [name, cron, duration, channel_id]
      properties:
        type: {type: string, enum: [filter, series], default: filter}
        name: {type: string}
        cron: {type: string}
        duration: {type: integer, minimum: 1}
        channel_id: {type: string}
        priority: {type: integer}
        max_duration_overflow_minutes: {type: integer}
        filter: { $ref: "#/components/schemas/Filter" }
        filler: { $ref: "#/components/schemas/FillerConfig" }
        series: { type: array, items: { $ref: "#/components/schemas/SeriesConfig" } }
        fallback: { $ref: "#/components/schemas/SeriesFallback" }
    BlockWrite:
      type: object
      required: [spec]
      properties: { enabled: {type: boolean, default: true}, spec: { $ref: "#/components/schemas/BlockSpec" } }
    BlockRecord:
      type: object
      required: [id, name, enabled, spec, created_at, updated_at]
      properties:
        id: {type: string}
        name: {type: string}
        enabled: {type: boolean}
        spec: { $ref: "#/components/schemas/BlockSpec" }
        created_at: {type: string, format: date-time}
        updated_at: {type: string, format: date-time}
    ImportResult:
      type: object
      required: [imported, dry_run]
      properties: { imported: {type: integer}, dry_run: {type: boolean}, names: {type: array, items: {type: string}} }
    GenerateRequest:
      type: object
      properties: { days: {type: integer, default: 7, minimum: 1, maximum: 30}, channel_id: {type: string} }
    ScheduledSlot:
      type: object
      properties:
        block_name: {type: string}
        channel_id: {type: string}
        start_time: {type: string, format: date-time}
        end_time: {type: string, format: date-time}
        programs: {type: array, items: {type: object, additionalProperties: true}}
    PlanResult:
      type: object
      required: [applied, channels]
      properties:
        applied: {type: boolean}
        channels:
          type: object
          additionalProperties: { type: array, items: { $ref: "#/components/schemas/ScheduledSlot" } }
    HistoryEntry:
      type: object
      properties: { program_id: {type: string}, channel_id: {type: string}, block_name: {type: string}, scheduled_at: {type: string, format: date-time} }
    SeriesState:
      type: object
      required: [show_title, current_season, current_episode]
      properties:
        show_title: {type: string}
        current_season: {type: integer}
        current_episode: {type: integer}
        completed: {type: boolean}
        disabled: {type: boolean}
        run_count: {type: integer}
        last_aired: {type: string, format: date-time, nullable: true}
    SeriesStatePatch:
      type: object
      properties:
        current_season: {type: integer, minimum: 1}
        current_episode: {type: integer, minimum: 1}
        completed: {type: boolean}
        disabled: {type: boolean}
    Channel:
      type: object
      properties: { id: {type: string}, name: {type: string}, number: {type: integer} }
    Status:
      type: object
      required: [version, tunarr_reachable]
      properties: { version: {type: string}, tunarr_reachable: {type: boolean}, tunarr_error: {type: string}, blocks: {type: integer} }
```

Adjust `ScheduledSlot`/`HistoryEntry` property names to match the real `scheduler.ScheduledSlot` / `scheduler.ScheduleHistoryEntry` JSON tags (read them in `internal/scheduler/` first; the spec must mirror the engine's types, not invent new ones).

- [ ] **Step 2: Codegen tooling**

`api/oapi-codegen.yaml`:

```yaml
package: gen
generate: { chi-server: true, models: true, embedded-spec: true }
output: internal/api/gen/server.gen.go
```

`tools/tools.go`:

```go
//go:build tools
package tools
import _ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
```

Makefile target:

```make
generate: ## Regenerate OpenAPI server code
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
```

Run `go get github.com/oapi-codegen/oapi-codegen/v2 github.com/go-chi/chi/v5 && make generate`.

- [ ] **Step 3: Problem writer + 501 stubs**

`internal/api/problem.go` per the Produces block (`WriteProblem` sets `Content-Type: application/problem+json`, pulls request id from context — Task 8 middleware provides it; empty until then). `internal/api/server.go`: `NewHandlers(d Deps)` returning a struct with every `gen.ServerInterface` method implemented as:

```go
func (h *Handlers) ListBlocks(w http.ResponseWriter, r *http.Request) {
    WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "listBlocks pending")
}
```

Test `problem_test.go`: `WriteProblem` sets status, content type, and valid JSON body.

- [ ] **Step 4: Verify + commit**

```bash
make generate && git diff --exit-code internal/api/gen/ # codegen is deterministic & committed
go test -race ./internal/api/... -v && make build && make lint
git add api tools internal/api Makefile go.mod go.sum
git commit -m "feat(api): OpenAPI v1 contract, oapi-codegen scaffolding, 501 stubs"
git push origin main
```

---

### Task 8: Middleware — request-id, logging, recovery, bearer auth

**Files:**
- Create: `internal/api/middleware/requestid.go`, `logging.go`, `recovery.go`, `auth.go`
- Test: `internal/api/middleware/middleware_test.go`

**Interfaces:**
- Consumes: `api.WriteProblem` (Task 7).
- Produces (used by Task 16):

```go
func RequestID(next http.Handler) http.Handler            // sets X-Request-Id header + context key
func RequestIDFrom(ctx context.Context) string            // used by WriteProblem
func Logging(l *slog.Logger) func(http.Handler) http.Handler
func Recovery(l *slog.Logger) func(http.Handler) http.Handler   // panic → 500 problem
func BearerAuth(token string) (func(http.Handler) http.Handler, error)
// BearerAuth: error at construction if len(token) < 32.
// Compares sha256 digests with crypto/subtle.ConstantTimeCompare; 401 problem on mismatch/missing.
```

- [ ] **Step 1: Failing tests** — auth: no header → 401 problem+json; wrong token → 401; valid → 200 and inner handler sees request; constructor rejects 31-char token. request-id: response carries `X-Request-Id`; provided inbound id is NOT trusted (always generate). recovery: panicking handler → 500 problem, process survives. logging: log line contains method, path, status, duration_ms, request_id (capture with `slog.NewJSONHandler` into a buffer).

```go
func TestBearerAuthRejectsMissingToken(t *testing.T) {
    mw, err := middleware.BearerAuth(strings.Repeat("x", 32))
    if err != nil { t.Fatal(err) }
    rr := httptest.NewRecorder()
    mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).
        ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/blocks", nil))
    if rr.Code != 401 { t.Fatalf("want 401, got %d", rr.Code) }
    if ct := rr.Header().Get("Content-Type"); ct != "application/problem+json" { t.Fatalf("ct=%s", ct) }
}
```

- [ ] **Step 2: Run FAIL → implement → run PASS** — `go test -race ./internal/api/middleware/ -v`
- [ ] **Step 3: Wire `WriteProblem` to `RequestIDFrom`** (fill the `request_id` field), re-run Task 7 tests.
- [ ] **Step 4: Commit and push**

```bash
git add internal/api
git commit -m "feat(api): request-id, logging, recovery, constant-time bearer auth"
git push origin main
```

---

### Task 9: Blocks CRUD handlers

**Files:**
- Modify: `internal/api/server.go` (replace 5 block stubs)
- Create: `internal/api/blocks.go`
- Test: `internal/api/blocks_test.go`

**Interfaces:**
- Consumes: Task 4 store CRUD, Task 5 `blockio.ValidateBlocks`, Task 7 gen types.
- Produces: JSON wire shapes exactly as the spec's `BlockRecord`/`BlockWrite`.

- [ ] **Step 1: Failing tests** — run against `gen.HandlerFromMux(NewHandlers(deps), chi.NewRouter())` with a real temp-dir store (cheap, avoids mock drift): create → 201 + body matches; create with invalid spec (no cron) → 400 problem citing validation; duplicate name → 409; get missing → 404; put changes cron → 200 and store reflects it; delete → 204 then 404; list returns both fixtures sorted by name. PUT with mismatched `spec.name` vs existing record renames the block (name uniqueness enforced → 409 on collision).
- [ ] **Step 2: Run FAIL → implement `blocks.go`** — decode via gen types, map to `store.BlockRecord{Spec: fromGen(body.Spec)}`, call `blockio.ValidateBlocks([]scheduler.Block{rec.Spec})` before Create/Update, map `store.ErrNotFound`→404, `ErrConflict`→409. Conversion helpers `fromGen(gen.BlockSpec) scheduler.Block` / `toGen(store.BlockRecord) gen.BlockRecord` live in `blocks.go` and are reused by Task 15.
- [ ] **Step 3: PASS + lint → commit and push**

```bash
git add internal/api
git commit -m "feat(api): blocks CRUD with CUE validation"
git push origin main
```

---

### Task 10: Series state handlers

**Files:**
- Modify: `internal/api/server.go` (2 stubs), Create: `internal/api/state.go`, Test: `internal/api/state_test.go`
- Modify: `internal/store/sqlite.go` — add `ListSeriesStates(ctx) ([]scheduler.SeriesState, error)` (thin wrapper over existing `ExportAllSeriesStates`) if not already exported under that name; reuse existing `SetSeriesState`/`UpdateSeriesState`.

**Interfaces:**
- Consumes: existing store series-state methods (`GetSeriesState`, `UpdateSeriesState`, `ExportAllSeriesStates`).
- Produces: PATCH semantics — only fields present in `SeriesStatePatch` change; unknown show_title → 404.

- [ ] **Step 1: Failing tests** — list returns seeded states; patch season/episode persists; patch `disabled:true` persists; patch unknown title → 404 problem; patch with no fields → 400.
- [ ] **Step 2: Implement → PASS + lint** — `go test -race ./internal/api/ -run TestState -v`
- [ ] **Step 3: Commit and push**

```bash
git add internal/api internal/store
git commit -m "feat(api): series state list and patch"
git push origin main
```

---

### Task 11: History endpoint + store query

**Files:**
- Modify: `internal/store/sqlite.go` (add `ListScheduleHistory`), Test: extend `internal/store/blocks_test.go` sibling file
- Modify: `internal/api/server.go` (1 stub), Create: `internal/api/history.go`, Test: `internal/api/history_test.go`

**Interfaces:**
- Produces: `func (s *Store) ListScheduleHistory(ctx context.Context, since time.Time) ([]scheduler.ScheduleHistoryEntry, error)` ordered by `scheduled_at DESC`.

- [ ] **Step 1: Failing store test** (seed via existing `RecordScheduleHistory`, query with `since` cutting off older entries) → FAIL → implement → PASS.
- [ ] **Step 2: Failing handler test** (`GET /api/v1/history?days=7` maps to `since = now-7d`; `days=200` → 400 via spec validation) → implement → PASS + lint.
- [ ] **Step 3: Commit and push**

```bash
git add internal/store internal/api
git commit -m "feat(api): schedule history endpoint"
git push origin main
```

---

### Task 12: Channels + status handlers (Tunarr boundary)

**Files:**
- Create: `internal/api/tunarr.go` (interface + impl adapter), Modify: `internal/api/server.go` (2 stubs), Test: `internal/api/tunarr_test.go`

**Interfaces:**
- Consumes: existing `tunarr.Client.GetChannels(ctx) ([]tunarr.Channel, error)`.
- Produces:

```go
type TunarrAPI interface { GetChannels(ctx context.Context) ([]tunarr.Channel, error) }
// Deps.Tunarr is this interface; production passes *tunarr.Client, tests pass a fake.
```

- [ ] **Step 1: Failing tests with a fake TunarrAPI** — `/channels`: maps id/name/number; Tunarr error → 502 problem (`title: "tunarr unreachable"`). `/status`: reachable fake → `tunarr_reachable:true`, `blocks` = store count, `version` = injected `Deps.Version`; failing fake → 200 with `tunarr_reachable:false` + `tunarr_error` string (status endpoint itself never 5xxs).
- [ ] **Step 2: Implement → PASS + lint → commit and push**

```bash
git add internal/api
git commit -m "feat(api): tunarr channels and status endpoints"
git push origin main
```

---

### Task 13: Schedule service extraction + generate/apply/schedule handlers

**Files:**
- Create: `internal/service/schedule.go`, Test: `internal/service/schedule_test.go`
- Modify: `cmd/generate.go` (its core becomes a call into the service — delete the duplicated logic)
- Modify: `internal/api/server.go` (3 stubs), Create: `internal/api/schedule.go`, Test: `internal/api/schedule_test.go`

**Interfaces:**
- Consumes: `scheduler.NewEngine(client, blocks, store, logger, loc)`, `Engine.GenerateForTimeRange(start, end, programs)`, `Engine.Commit()`, `tunarr.Client.SearchPrograms`, `tunarr.Client.UpdateSchedule`, Task 6 `loadActiveBlocks` (move it into this package as `ActiveBlocks(ctx, store)`).
- Produces:

```go
type Options struct { Days int; ChannelID string; Apply bool }   // Days validated 1..30 upstream
type Result struct { Applied bool; Channels map[string][]scheduler.ScheduledSlot }
type Runner struct { /* store, tunarr client, logger, location */ }
func NewRunner(st *store.Store, tc *tunarr.Client, l *slog.Logger, loc *time.Location) *Runner
func (r *Runner) Run(ctx context.Context, o Options) (*Result, error)
// Run: ActiveBlocks → fetch available programs (mirror cmd/generate.go's current
// SearchPrograms usage exactly) → NewEngine → GenerateForTimeRange(now, now+Days) →
// if Apply: UpdateSchedule per channel + Engine.Commit(). ChannelID filters the
// result map when set. Never mutates state on dry-run (Commit only under Apply).
type ScheduleRunner interface { Run(ctx context.Context, o Options) (*Result, error) }  // Deps.Sched
```

- [ ] **Step 1: Extract with a characterization test first** — before moving code, write `schedule_test.go` exercising `Runner.Run` dry-run against a fake Tunarr HTTP server (use `httptest.NewServer` with canned `/api/channels` + program-search responses copied from `internal/external/tunarr/client_test.go` fixtures) asserting: slots come back for an enabled block, `Applied=false`, and series state in the store is untouched. Run it against the NEW Runner (FAIL: doesn't exist), then port the logic out of `cmd/generate.go` until PASS.
- [ ] **Step 2: Rewire `cmd/generate.go`** to `service.NewRunner(...).Run(ctx, Options{Apply: applyFlag && assumeYes, ...})`; delete the now-duplicated code; `make test` (existing cmd tests must stay green).
- [ ] **Step 3: Handler tests with a fake ScheduleRunner** — `POST /generate` → 200 PlanResult, `Applied:false` regardless of body; `POST /apply` → runner called with `Apply:true`; `GET /schedule?days=3` → runner called with `{Days:3, Apply:false}`; runner error → 502 problem.
- [ ] **Step 4: Implement handlers → PASS + `make test && make lint` → commit and push**

```bash
git add internal/service internal/api cmd
git commit -m "feat(api): schedule service extraction; generate/apply/schedule endpoints"
git push origin main
```

---

### Task 14: Import/export handlers

**Files:**
- Modify: `internal/api/server.go` (2 stubs), Create: `internal/api/importexport.go`, Test: `internal/api/importexport_test.go`

**Interfaces:**
- Consumes: `blockio.ParseYAML`, `blockio.RenderYAML`, store CRUD, `toGen`/`fromGen` (Task 9).

- [ ] **Step 1: Failing tests** — import valid YAML (2 blocks) → 200 `{imported:2, dry_run:false, names:[...]}` and records exist; `?dry_run=true` → counts but store unchanged; invalid YAML → 400 problem with CUE detail; name collision with existing block → 409 problem, nothing partially imported (wrap in a transaction or pre-check all names). Export: seeded store → 200 `application/yaml` body that `blockio.ParseYAML` round-trips.
- [ ] **Step 2: Implement → PASS + lint → commit and push**

```bash
git add internal/api
git commit -m "feat(api): YAML block import/export endpoints"
git push origin main
```

---

### Task 15: `serve` command — router assembly, cron co-hosting, graceful shutdown; delete `run`

**Files:**
- Create: `cmd/serve.go`, `internal/api/router.go`
- Delete: `cmd/run.go` (its cron loop moves into serve; no alias — no-legacy policy)
- Modify: `cmd/root.go` (register serveCmd, drop runCmd), `internal/config` + `cmd/schema/config.cue` (add `api: { listen, token, insecure_no_auth }` keys; token also read from `SCHEDULARR_API_TOKEN` env, env wins)
- Test: `internal/api/router_test.go`, `cmd/serve_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces:

```go
// internal/api/router.go
type Config struct { Token string; InsecureNoAuth bool }
func NewRouter(cfg Config, d Deps) (http.Handler, error)
// chi router: /healthz /readyz /metrics /openapi.json unauthenticated;
// /api/v1/* wrapped in RequestID→Logging→Recovery→BearerAuth (skipped only
// when InsecureNoAuth, which logs a WARN at startup);
// /metrics = promhttp.Handler() (reuse internal/metrics registry);
// /openapi.json serves gen.GetSwagger() JSON; /readyz pings the store (SELECT 1).
// Error if Token=="" && !InsecureNoAuth, or token shorter than 32 chars.
```

- [ ] **Step 1: Router tests** — unauthenticated paths reachable without token; `/api/v1/blocks` without token → 401; with token → 200; `NewRouter` with empty token and `InsecureNoAuth:false` → error; `--insecure-no-auth` path serves 200 without header.
- [ ] **Step 2: Implement router → PASS.**
- [ ] **Step 3: `cmd/serve.go`** — flags `--listen :8484`, `--insecure-no-auth`; builds config/store/tunarr client/Runner; runs `blockio.Bootstrap`; moves the cron loop from `cmd/run.go` (same tick behavior, now calling `service.Runner.Run(ctx, Options{Apply:true})` per due block schedule); `http.Server` with `ReadHeaderTimeout: 10 * time.Second`; `signal.NotifyContext(SIGINT, SIGTERM)` → `server.Shutdown(ctx)` with 15s timeout, then cron stop, then store close. Delete `cmd/run.go`. Serve test: start on `:0`, hit `/healthz`, send SIGTERM equivalent (cancel context), assert clean exit.
- [ ] **Step 4: Full suite + manual smoke**

```bash
make build && make test && make lint
SCHEDULARR_API_TOKEN=$(openssl rand -hex 32) ./bin/schedularr serve --listen :8484 &
curl -s localhost:8484/healthz            # 200
curl -s localhost:8484/api/v1/blocks      # 401 problem+json
curl -s -H "Authorization: Bearer $SCHEDULARR_API_TOKEN" localhost:8484/api/v1/blocks  # 200 []
kill %1
```

- [ ] **Step 5: Commit and push**

```bash
git add -A
git commit -m "feat!: serve command hosts API + cron; run command removed"
git push origin main
```

---

### Task 16: Documentation + changelog sweep

**Files:**
- Modify: `README.md` (commands, API section with auth + curl examples, config keys incl. `SCHEDULARR_API_TOKEN`, remove all TUI/*arr/jellyfin content)
- Modify: `CLAUDE.md` (architecture tree: add `api/`, `internal/api/`, `internal/service/`, `internal/blockio/`; remove tui/external clients; update commands)
- Modify: `CHANGELOG.md` (one entry per breaking change: module rename, TUI removal, integration removals, blocks-in-store, serve/run)

- [ ] **Step 1: Update all three docs.** Every command shown must be copy-paste runnable against the final binary.
- [ ] **Step 2: Verify docs honesty** — run every command/curl in README against a fresh build.
- [ ] **Step 3: Commit and push**

```bash
git add README.md CLAUDE.md CHANGELOG.md
git commit -m "docs: align README/CLAUDE/CHANGELOG with API server core"
git push origin main
```

---

## Self-review notes (resolved inline)

- Spec coverage: auth (T8/T15), blocks CRUD+CUE (T4/T5/T9), generate/apply/schedule/history (T11/T13), series state (T10), channels/status (T12), import/export (T5/T14), serve+graceful+metrics+openapi.json (T15), bootstrap (T5/T6), removals (T2/T3), module/Go (T1), docs (T16). RFC 7807 everywhere via `WriteProblem` (T7).
- CORS: same-origin means no CORS middleware at all (no cross-origin headers emitted) — nothing to build; recorded here so nobody adds one.
- `ScheduledSlot`/`HistoryEntry` spec schemas must be checked against the real engine JSON tags in T7 Step 1 (explicit instruction in task).
- Type consistency: `Deps{Store, Tunarr TunarrAPI, Sched ScheduleRunner, Logger, Version}` defined T7, populated T12/T13, consumed T15. `store.ErrNotFound/ErrConflict` defined T4, mapped T9/T10/T14.
