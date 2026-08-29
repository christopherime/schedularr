# Schedularr Container, CI & Cluster Deployment — Design

**Date:** 2026-08-29
**Status:** Approved (design review with operator)
**Sub-project:** 4 of 4 brought forward (API core ✓ → Web UI ✓ → **Deploy** → Tag/metadata engine)

## Context

Sub-projects 1–2 shipped a complete API server + embedded web UI on
`main`. Nothing runs in the cluster yet: no image, no CI, no chart, no
Application. This sub-project ships schedularr to the GXF cluster next
to Tunarr, via the established GitOps flow (public `helm-charts` chart,
private `applicationset` Application), and pays the deployment debts
recorded in TODO.md.

## Decisions (settled with operator)

1. **Semver releases**: CI builds/tests every push; a `v*` git tag
   builds and pushes `ghcr.io/christopherime/schedularr:vX.Y.Z` (plus
   `latest`); the ArgoCD Application pins the exact version.
2. **Public GHCR package on the private repo**: the container package is
   made public (one-time GitHub settings action if the API can't do it —
   surfaced to the operator when reached); no pull secrets to maintain.
   Source stays private.
3. **`media` namespace**, beside Tunarr: Tunarr URL is same-namespace
   (`http://tunarr:8000`), storage/secret conventions shared.
4. **Token-only auth, LAN-only exposure**: HTTPRoute
   `schedularr.local.geekxflood.io` via cilium-gateway (never public —
   house rule); oauth2-proxy/Keycloak SSO deferred as its own change.
5. **SealedSecret** for `SCHEDULARR_API_TOKEN`
   (`manifests/schedularr/sealed-secret.yaml`, meilisearch precedent),
   recorded in the wiki secrets inventory. **No Tunarr credential
   exists** — Tunarr's API is unauthenticated (upstream #1032), so the
   config carries only its URL.
6. **Dockerfile does the real work**: multi-stage — node (npm ci, types
   generation, `tsc --noEmit`) → Hugo (real `make web` equivalent,
   pinned Hugo version) → Go build with `-X
   github.com/christopherime/schedularr/cmd.Version=<tag>` (fixing the
   dead `main.Version` ldflag). Release binaries can never contain the
   placeholder page.
7. **TODO.md debts paid here**: placeholder untracked (a `web-presence`
   Make target generates it when missing); CI enforces codegen drift
   (`make generate` + `git diff --exit-code internal/api/gen/`) and
   TS-types drift (`make web-types` + diff) on every push.
8. **Two workflows** in `christopherime/schedularr/.github/workflows/`:
   `ci.yaml` (push/PR: golangci-lint, gosec, govulncheck, race tests,
   both drift checks, tsc, docker build without push) and
   `release.yaml` (on `v*` tag: build + push image; `permissions:
   packages: write`, plain docker login/build/push with GITHUB_TOKEN —
   the bench-repo house pattern).
9. **GitOps side follows the house directives** (applicationset
   CLAUDE.md): chart validated by `helm-chart-validator`, wiki by
   `wiki-doc-synchronizer`, changelog by `change-tracker`, all in the
   same commit set as the chart/Application.

## Architecture (three-repo change map)

```txt
christopherime/schedularr (private)      geekxflood/helm-charts (public)
├── Dockerfile        (rework, item 6)   └── charts/schedularr/
├── .github/workflows/ci.yaml                ├── Chart.yaml (v0.1.0, appVersion v0.1.0)
├── .github/workflows/release.yaml           ├── values.yaml (agnostic defaults)
├── Makefile web-presence target             └── templates/ (deployment, service,
└── web/public/index.html UNTRACKED              httproute, pvc, configmap for
                                                 config.yaml, serviceaccount,
geekxflood/applicationset (private)              _helpers, NOTES)
├── argocd/apps/media/schedularr.yaml
├── manifests/schedularr/sealed-secret.yaml
├── wiki/docs/applications/media/schedularr.md (+ indexes, nav, secrets inventory)
└── CHANGELOG.md
```

- Chart config model: `config.yaml` rendered from values into a
  ConfigMap (log level, listen, cron_interval, tunarr URL,
  scheduler_file, history retention); `SCHEDULARR_API_TOKEN` via
  `envFrom`/`existingSecret`; SQLite on a chart-managed PVC (2Gi,
  storageClass from Application values); checksum annotation restarts
  pods on config change.
- Probes: `/healthz` liveness, `/readyz` readiness (both exist).
  `/metrics` scraped per house monitoring conventions if a
  ServiceMonitor pattern applies (check at chart time; optional value).
- Resources: requests-only baseline (house-precedented), no GPU.

## Verification gates

- CI green on the repo before the first tag; `v0.1.0` tag publishes the
  image; anonymous `docker pull` (or crane) proves the package is
  public before the Application lands.
- `helm-chart-validator` on the chart + Application; `mkdocs build
  --strict`; live gates after ArgoCD sync: pod Running, `/healthz` 200
  via the HTTPRoute, `/api/v1/status` shows tunarr_reachable:true,
  UI loads, one dry-run `POST /generate` succeeds against live Tunarr.

## Non-goals

- SSO fronting (future small change); SP3 tags; multi-arch images
  beyond linux/amd64 (cluster is amd64; arm64 later if wanted);
  autoscaling/HA (SQLite, single replica, `Recreate` strategy).
