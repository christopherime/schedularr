# Deployment

## Docker

`docker build` runs the same pipeline as `make web && make build`, inside the image, from a bare checkout — no local Hugo, Node, or Go required. `Dockerfile` is four stages: `node:22-alpine` regenerates and type-checks the TypeScript, a pinned Hugo release binary runs `hugo --minify -s web`, `golang:1.27-alpine` builds the binary against that real UI output, and `alpine:3.20` is the runtime. The image never embeds the `web/public/index.html` placeholder used for local `go build` without Hugo — the Hugo stage always builds the real site first.

**CGO is required.** `internal/store` persists through [`mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3), a cgo binding over the SQLite C amalgamation with no pure-Go build tag, so `CGO_ENABLED=0` isn't an option. The build stage links against musl (Alpine's default gcc target); the final stage is Alpine too, matching libc between build and run. A `distroless/static` final image is **not** viable with this dependency.

```bash
# Build (VERSION stamps GET /api/v1/status and cmd.Version, default "dev")
docker build --build-arg VERSION=v1.2.3 -t schedularr:v1.2.3 .
# or: make docker-build VERSION=v1.2.3

# Run: mount a config file and pass the API token via env, never the config
docker run -d --name schedularr \
  -p 8484:8484 \
  -v "$(pwd)/config.yaml:/etc/schedularr/config.yaml:ro" \
  -v schedularr-data:/data \
  -e SCHEDULARR_API_TOKEN="$(openssl rand -hex 32)" \
  schedularr:v1.2.3
```

The image's default `CMD` is `serve --config /etc/schedularr/config.yaml` (`ENTRYPOINT ["schedularr"]`, so `docker run ... schedularr:tag <args>` replaces it with another subcommand instead). Point the mounted config's `database` and `scheduler_file` keys at paths under `/data` explicitly — both resolve relative to the process's working directory, not the config file's location. The container runs as a non-root `schedularr` user (uid 1001), exposes `8484` (`api.listen`'s default), and its `HEALTHCHECK` polls `GET /healthz` on that port.

| Build arg | Default | Purpose |
| --- | --- | --- |
| `VERSION` | `dev` | Stamped via `-ldflags -X .../cmd.Version=...`; reported by `GET /api/v1/status` |
| `GO_VERSION` | `1.27` | `golang:${GO_VERSION}-alpine` build stage |
| `NODE_VERSION` | `22` | `node:${NODE_VERSION}-alpine` TypeScript stage |
| `ALPINE_VERSION` | `3.20` | Hugo-fetch and final runtime stage |
| `HUGO_VERSION` | `0.165.0` | Pinned Hugo release, downloaded and sha256-verified in-stage |

Pre-built images publish to GHCR on tagged releases:

```text
ghcr.io/christopherime/schedularr:<tag>
ghcr.io/christopherime/schedularr:latest
```

## Configuration reference

`schedularr` resolves the app config file path in this order:

1. `--config <file>` (global flag)
2. `SCHEDULARR_CONFIG` environment variable
3. Legacy locations, in order: `./config.yaml`, `./.schedularr.yaml`, `~/.config/.schedularr.yaml`, `~/.schedularr.yaml`
4. `~/.schedularr/config.yaml` (default)

The file is validated against the CUE schema in `cmd/schema/config.cue`. String values support `${VAR}` environment variable interpolation, applied after parsing: an unset variable becomes an empty string (never `null`), and a variable's value is never re-parsed as YAML syntax.

```yaml
tunarr:
  url: "" # Tunarr API endpoint (required)
  api_key: "" # Optional API key
  timeout: "30s"

log:
  level: "info" # debug, info, warn, error
  format: "text" # text, json
  timezone: "Local" # IANA time zone name

database: "schedularr.db" # SQLite database path (opened with _busy_timeout=5000&_journal_mode=WAL --
# expect -shm/-wal sidecar files alongside it)
scheduler_file: "scheduler.yaml" # First-run block import file -- see Scheduling Concepts

maintenance:
  history_retention: "168h" # How long schedule_history rows are kept; also bounds GET /history?days=N
  cleanup_enabled: true

cron_interval: "6h" # `serve`'s cron loop cadence; `serve --interval`/`-i` overrides it. The apply window scales with it (floor(interval/24h)+1 days), so >24h intervals stay safe

api: # the `serve` command's HTTP server
  listen: ":8484"
  token: "" # Bearer token for /api/v1/*; SCHEDULARR_API_TOKEN env var wins when set
  insecure_no_auth: false # Skip bearer auth entirely -- local development only
```

There is no `metrics_port` config key: `schedularr serve` exposes Prometheus metrics at `GET /metrics` on the same listener as everything else (`--listen`/`api.listen`, default `:8484`).

| Config key | Env var | Default | Description |
| --- | --- | --- | --- |
| `api.listen` | — | `:8484` | Same as `--listen`; the flag wins when explicitly passed |
| `api.token` | `SCHEDULARR_API_TOKEN` | `""` | Bearer token required on `/api/v1/*`. The env var always wins over this key when both are set |
| `api.insecure_no_auth` | — | `false` | Same as `--insecure-no-auth`; either source turning it on disables auth |
| `cron_interval` | — | `6h` | Same as `--interval`/`-i`; the flag wins when explicitly passed. Top-level key, not under `api.*` — it governs the cron loop, not the HTTP server. Each tick's apply window is derived from it (floor(interval/24h)+1 days) so the pushed lineup always outlasts the gap between ticks |

`schedularr serve` refuses to start if the effective token is empty (or shorter than 32 characters) and `--insecure-no-auth`/`api.insecure_no_auth` is not set.

Generate a starting file with `schedularr config generate config.yaml` — see [Getting Started](getting-started.md) and the [CLI Reference](cli-reference.md#config-generate-filename). Never commit real Tunarr credentials; keep redacted examples in `configs/` and store machine-specific overrides outside the repository.

## Helm chart

A generic Helm chart for Schedularr is maintained at [geekxflood/helm-charts](https://github.com/geekxflood/helm-charts). It packages the `docker run` invocation above as a Kubernetes `Deployment`/`Service`, with the config file and `SCHEDULARR_API_TOKEN` supplied through standard chart values (a `ConfigMap`/`Secret`, or your own). Consult that repository for the chart's own values reference and version history — this page intentionally stays cluster-agnostic; hostnames, storage classes, and ingress configuration are specific to each deployment and belong in your own cluster's configuration, not here.
