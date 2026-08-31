# End-to-End Testing

This directory contains the E2E testing infrastructure for Schedularr.

## Overview

The E2E tests verify that Schedularr works correctly with a real Tunarr instance running in Docker.

## Prerequisites

- Docker and Docker Compose installed
- Make (for running Makefile targets)

## Quick Start

```bash
# Start the E2E environment
make e2e-up

# Wait for Tunarr to be healthy (check with docker ps)
docker ps

# Run E2E tests
make e2e-test

# Stop the E2E environment
make e2e-down
```

## Environment

The E2E environment includes:

- **Tunarr**: Latest version running on port 8000
- **Test Fixtures**: Sample configuration files in `fixtures/`

## Test Fixtures

The `fixtures/` directory contains:

- Sample scheduler configurations
- Test channel configurations
- Mock program data (if needed)

## Manual Testing

You can also manually test against the E2E environment:

```bash
# Start environment
make e2e-up

# Wait for Tunarr to be ready
sleep 30

# Generate a test config
./bin/schedularr config generate e2e/test-config.yaml

# Edit the config to point to localhost:8000, and set scheduler_file to
# e2e/fixtures/test-scheduler.yaml (the import file is a config key, not a
# `generate` flag -- it's bootstrapped into the block store on first run)
vim e2e/test-config.yaml

# Test channel listing
./bin/schedularr --config e2e/test-config.yaml channels

# Test schedule generation
./bin/schedularr --config e2e/test-config.yaml generate

# Clean up
make e2e-down
```

## Troubleshooting

### Tunarr not starting

Check logs:

```bash
docker logs schedularr-tunarr-e2e
```

### Port already in use

If port 8000 is already in use, you can modify the port mapping in `docker-compose.yaml`:

```yaml
ports:
  - "8001:8000"  # Change 8000 to 8001 or another free port
```

### Healthcheck failing

The healthcheck verifies Tunarr is responding. If it fails:

1. Check Tunarr logs
2. Verify the `/api/version` endpoint is accessible
3. Increase `start_period` in docker-compose.yaml if Tunarr takes longer to start

## Cleanup

To completely remove all E2E data:

```bash
# Stop and remove containers, networks, and volumes
make e2e-clean

# Or manually:
docker-compose -f e2e/docker-compose.yaml down -v
```
