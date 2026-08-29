# syntax=docker/dockerfile:1
#
# Schedularr container image -- four stages, three toolchains.
#
#   1. webtypes  (node:22-alpine)     npm ci, regenerate web/assets/ts/gen/
#                                     types.d.ts from api/openapi.yaml, then
#                                     `tsc --noEmit` -- the same drift/type
#                                     check `make web-check` runs locally.
#   2. hugo      (alpine, pinned)     downloads the pinned Hugo release
#                                     binary, verified by sha256.
#      webui     (FROM hugo)         runs the real `hugo --minify -s web`
#                                     (never the committed placeholder) using
#                                     stage 1's freshly generated TS types.
#   3. builder   (golang:1.27-alpine) CGO_ENABLED=1 build (see below), with
#                                     `web/public` replaced by stage 2's real
#                                     output before `go build` runs, and
#                                     `-ldflags -X .../cmd.Version=$VERSION`.
#   4. final     (alpine:3.20)       non-root runtime, ca-certificates +
#                                     tzdata only.
#
# CGO is not optional here: github.com/mattn/go-sqlite3 (internal/store's
# only persistence backend) is a cgo binding over the sqlite3 C amalgamation
# and ships no pure-Go/CGO_ENABLED=0 build tag. A distroless/static final
# stage is therefore not viable for this binary -- it needs a libc at
# runtime for the linked sqlite3 object, so the build stage links against
# musl (Alpine's gcc default) and the final stage is Alpine too, matching
# libc between build and run.
#
# Hugo acquisition: the pinned official GitHub release binary, not a
# third-party image (e.g. hugomods/hugo). Reasons: (1) exact version *and*
# edition parity with the toolchain `make web-build` requires locally --
# `hugo version` on the dev machine that authored this Dockerfile reports
# `v0.165.0+extended`, HUGO_VERSION below pins the same release; (2) a
# first-party sha256 (from the release's own checksums.txt) to verify
# against, rather than trusting a third party's image build; (3) no
# dependency on hugomods' own image-refresh cadence lagging or diverging
# from upstream Hugo releases. The site itself only uses Hugo Pipes
# (resources.Get/minify/fingerprint) and js.Build (esbuild, pure Go) --
# neither requires the "extended" edition -- but pinning extended anyway
# keeps prod and dev bit-for-bit on tooling rather than "works today."

ARG GO_VERSION=1.27
ARG NODE_VERSION=22
ARG ALPINE_VERSION=3.20
ARG HUGO_VERSION=0.165.0
ARG VERSION=dev

# ---------------------------------------------------------------------------
# Stage 1: TypeScript types + strict type-check (mirrors `make web-check`)
# ---------------------------------------------------------------------------
FROM node:${NODE_VERSION}-alpine AS webtypes
WORKDIR /src
COPY web/package.json web/package-lock.json web/tsconfig.json ./web/
COPY api/openapi.yaml ./api/openapi.yaml
RUN npm ci --prefix web
COPY web/assets/ts ./web/assets/ts
RUN npm run types --prefix web && npm run check --prefix web

# ---------------------------------------------------------------------------
# Stage 2a: fetch + verify the pinned Hugo binary
# ---------------------------------------------------------------------------
FROM alpine:${ALPINE_VERSION} AS hugo
ARG HUGO_VERSION
ARG TARGETARCH
RUN apk add --no-cache curl && \
    case "${TARGETARCH}" in \
      amd64) HUGO_SHA256=f43494894cdf4a8630a201d5c828051c77f523cc66bb3938b30806835470ac20 ;; \
      arm64) HUGO_SHA256=f40ebc44dfda3896cecd3ae7ed44f5c44c4b4a30a2b7d976ece6da62da699a58 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac && \
    curl -fsSLo /tmp/hugo.tgz "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-${TARGETARCH}.tar.gz" && \
    echo "${HUGO_SHA256}  /tmp/hugo.tgz" | sha256sum -c - && \
    tar -xzf /tmp/hugo.tgz -C /usr/local/bin hugo && \
    rm /tmp/hugo.tgz

# ---------------------------------------------------------------------------
# Stage 2b: Hugo build (mirrors `make web-build`), fed by stage 1's types
# ---------------------------------------------------------------------------
FROM hugo AS webui
WORKDIR /src/web
COPY web/hugo.toml ./
COPY web/content ./content
COPY web/layouts ./layouts
COPY web/assets/css ./assets/css
COPY web/assets/vendor ./assets/vendor
COPY --from=webtypes /src/web/assets/ts ./assets/ts
WORKDIR /src
RUN hugo --minify -s web

# ---------------------------------------------------------------------------
# Stage 3: Go build. CGO_ENABLED=1 for mattn/go-sqlite3 (see header note).
# ---------------------------------------------------------------------------
FROM golang:${GO_VERSION}-alpine AS builder
ARG VERSION
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY configs/ ./configs/
COPY web/embed.go web/embed.go
COPY --from=webui /src/web/public web/public
ENV CGO_ENABLED=1 GOOS=linux
RUN go build -trimpath -ldflags "-s -w -X github.com/christopherime/schedularr/cmd.Version=${VERSION}" \
    -o /out/schedularr .

# ---------------------------------------------------------------------------
# Stage 4: runtime
# ---------------------------------------------------------------------------
FROM alpine:${ALPINE_VERSION} AS final
ARG VERSION
LABEL org.opencontainers.image.title="schedularr" \
      org.opencontainers.image.description="Cron-based content scheduling for Tunarr TV channels" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/christopherime/schedularr" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1001 -S schedularr && \
    adduser -u 1001 -S schedularr -G schedularr -h /home/schedularr && \
    mkdir -p /data /home/schedularr && \
    chown -R schedularr:schedularr /data /home/schedularr

COPY --from=builder /out/schedularr /usr/local/bin/schedularr

USER schedularr
WORKDIR /home/schedularr
ENV HOME=/home/schedularr

# 8484 is api.listen's default (configs/config.yaml, cmd/schema/config.cue).
# /data is a suggested mount point for the SQLite DB + scheduler.yaml; the
# actual paths come from the mounted config file's `database` and
# `scheduler_file` keys (both resolved relative to the process's working
# directory, not the config file's -- point them at /data/... explicitly).
EXPOSE 8484
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O- "http://127.0.0.1:8484/healthz" >/dev/null || exit 1

ENTRYPOINT ["schedularr"]
CMD ["serve", "--config", "/etc/schedularr/config.yaml"]
