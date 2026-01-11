# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies for CGO (go-sqlite3)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build application
RUN CGO_ENABLED=1 go build -ldflags="-w -s" -o schedularr cmd/schedularr/main.go

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache tzdata ca-certificates

# Copy binary from builder
COPY --from=builder /app/schedularr .

# Copy example configs
COPY configs/config.example.yaml config.yaml.example
COPY configs/scheduler.example.yaml scheduler.yaml.example

# Create data directory for SQLite DB
RUN mkdir -p /data
VOLUME /data

# Set entrypoint
ENTRYPOINT ["./schedularr"]

# Default command: run as daemon
CMD ["run"]
