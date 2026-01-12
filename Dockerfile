# Dynamic Go version support:
# Option 1: Auto-detect and build with:
#   docker build --build-arg GO_VERSION=$(grep '^go ' go.mod | sed 's/go //' | sed 's/\.[0-9]*$//' | head -1) .
# Option 2: Manual override:
#   docker build --build-arg GO_VERSION=1.25 .
# Option 3: Use default (will show warning if mismatch)
#
# Build arguments with smart defaults
ARG IMAGE_SOURCE
ARG GO_VERSION=1.25
ARG VERSION=dev
ARG BUILD_DATE=""
ARG GIT_COMMIT=""
ARG GIT_BRANCH=""
ARG GOPROXY=https://proxy.golang.org
ARG GOPROXY_ADDR=""
ARG PKI_ROOT_CA=""
ARG PKI_WEB_01_CA=""
ARG GOPRIVATE=""
ARG GIT_REWORK=""

# # --- Frontend Build Stage ---
# FROM node:25-alpine AS frontend-builder
# WORKDIR /app/web
# COPY web/package*.json ./
# RUN npm ci
# COPY web/ ./
# RUN npm run build

# --- Backend Build Stage ---
# Build stage with dynamic Go version - use IMAGE_SOURCE if provided, otherwise default
FROM ${IMAGE_SOURCE:-golang:${GO_VERSION}-alpine} AS builder

# Re-declare build arguments for this stage (required after FROM)
ARG VERSION=dev
ARG BUILD_DATE=""
ARG GIT_COMMIT=""
ARG GIT_BRANCH=""
ARG GOPROXY=https://proxy.golang.org
ARG GOPROXY_ADDR=""
ARG PKI_ROOT_CA=""
ARG PKI_WEB_01_CA=""
ARG GOPRIVATE=""
ARG GIT_REWORK=""

# Install build dependencies and add custom CA certificates if provided
RUN apk add --no-cache \
  git \
  ca-certificates \
  upx \
  && rm -rf /var/cache/apk/*

# Set working directory
WORKDIR /app

# Copy go module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (excluding web directory as it's built separately)
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY configs/ ./configs/

# Set Go environment - use GOPROXY_ADDR if provided, otherwise direct
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

# Build the Go binary
RUN mkdir -p bin && \
  go build -ldflags="-w -s -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.GitCommit=${GIT_COMMIT}" \
  -o bin/schedularr ./cmd/schedularr

# Compress the binary for smaller image size
RUN upx --best --lzma bin/schedularr

# Runtime stage
FROM alpine:3

# Build arguments for labels with defaults
ARG VERSION=dev
ARG BUILD_DATE=""
ARG GIT_COMMIT=""
ARG GIT_BRANCH=""

# Install runtime dependencies
RUN apk add --no-cache \
  ca-certificates \
  tzdata \
  && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1001 -S schedularr && \
  adduser -u 1001 -S schedularr -G schedularr

# Create necessary directories
RUN mkdir -p /app/logs /app/web/dist && \
  chown -R schedularr:schedularr /app

# Set working directory
WORKDIR /app

# Copy binary from builder stage (from bin/ directory following project conventions)
COPY --from=builder /app/bin/schedularr /usr/local/bin/schedularr
# # Copy frontend assets from frontend-builder stage
# COPY --from=frontend-builder /app/web/dist /app/web/dist

RUN chmod +x /usr/local/bin/schedularr

# Switch to non-root user
USER schedularr

EXPOSE 9600/tcp

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD /usr/local/bin/schedularr --help > /dev/null || exit 1

# Default command - users should mount their own config
ENTRYPOINT ["/usr/local/bin/schedularr"]
CMD ["--config", "/etc/schedularr/config.yaml"]
