# Getting Started

## Prerequisites

- **Go 1.27+** — [download](https://go.dev/dl/)
- **A C toolchain** — the SQLite driver (`mattn/go-sqlite3`) is cgo-based; `CGO_ENABLED=1` (the default when a C compiler is on `PATH`) is required to build
- **A Tunarr instance** — [setup guide](https://tunarr.com/api-docs.html#latest)

## Install

### Option 1: build from source

```bash
git clone https://github.com/christopherime/schedularr.git
cd schedularr

go build -o schedularr main.go

# Optional: install to your PATH
sudo mv schedularr /usr/local/bin/
```

### Option 2: go install

```bash
go install github.com/christopherime/schedularr@latest
```

### Option 3: Docker

No local Go, Hugo, or Node needed — see [Deployment](deployment.md) for the full `docker run` reference.

```bash
docker build -t schedularr:local .
```

## Initial setup

1. Generate configuration files:

   ```bash
   # Application config
   schedularr config generate config.yaml

   # Scheduler config (first-run block import — see Scheduling Concepts)
   schedularr scheduler init scheduler.yaml
   ```

2. Configure the Tunarr connection.

   `config generate` writes `tunarr.url`/`tunarr.api_key` as literal `${SCHEDULARR_TUNARR_URL}`/`${SCHEDULARR_TUNARR_API_KEY}` placeholders (CUE-loaded config files support `${VAR}` env interpolation — see [Deployment's config reference](deployment.md#configuration-reference)). Either export those two environment variables, or edit `config.yaml` directly and replace both with literal values (an unset `${VAR}` simply becomes an empty string):

   ```yaml
   tunarr:
     url: "http://localhost:8000"
     api_key: ""
   log:
     level: "info"
     format: "text"
   ```

3. Validate the configuration:

   ```bash
   schedularr validate config.yaml
   schedularr validate scheduler.yaml
   ```

4. Verify the Tunarr connection:

   ```bash
   schedularr --config config.yaml channels
   ```

## Next steps

- Read [Scheduling Concepts](scheduling-concepts.md) to write your first blocks.
- Run `schedularr generate` to preview a schedule, then `schedularr generate --apply --yes` to push it to Tunarr — see the [CLI Reference](cli-reference.md).
- Run `schedularr serve` for the HTTP API and web UI — see the [Web UI Guide](web-ui-guide.md).
