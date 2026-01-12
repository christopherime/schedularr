#!/usr/bin/env bash
set -euo pipefail

# E2E Test Script for Schedularr
# This script runs end-to-end tests against a Tunarr instance

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BINARY="${PROJECT_ROOT}/bin/schedularr"
CONFIG="${SCRIPT_DIR}/fixtures/test-config.yaml"
SCHEDULER="${SCRIPT_DIR}/fixtures/test-scheduler.yaml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

test_start() {
    TESTS_RUN=$((TESTS_RUN + 1))
    echo ""
    log_info "Test $TESTS_RUN: $1"
}

test_pass() {
    TESTS_PASSED=$((TESTS_PASSED + 1))
    log_info "✓ PASSED"
}

test_fail() {
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "✗ FAILED: $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if [ ! -f "$BINARY" ]; then
        log_error "Binary not found at $BINARY"
        log_info "Run 'make build' first"
        exit 1
    fi
    
    if ! docker ps | grep -q schedularr-tunarr-e2e; then
        log_error "Tunarr container not running"
        log_info "Run 'make e2e-up' first"
        exit 1
    fi
    
    log_info "Prerequisites OK"
}

# Wait for Tunarr to be ready
wait_for_tunarr() {
    log_info "Waiting for Tunarr to be ready..."
    
    MAX_ATTEMPTS=30
    ATTEMPT=0
    
    while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
        if curl -s http://localhost:8000/api/version > /dev/null 2>&1; then
            log_info "Tunarr is ready"
            return 0
        fi
        
        ATTEMPT=$((ATTEMPT + 1))
        echo -n "."
        sleep 2
    done
    
    log_error "Tunarr did not become ready in time"
    exit 1
}

# Test 1: Validate config files
test_validate_configs() {
    test_start "Validate configuration files"
    
    if "$BINARY" validate "$CONFIG" > /dev/null 2>&1; then
        test_pass
    else
        test_fail "Config validation failed"
    fi
    
    test_start "Validate scheduler file"
    
    if "$BINARY" validate "$SCHEDULER" > /dev/null 2>&1; then
        test_pass
    else
        test_fail "Scheduler validation failed"
    fi
}

# Test 2: List channels
test_list_channels() {
    test_start "List Tunarr channels"
    
    if "$BINARY" --config "$CONFIG" channels > /dev/null 2>&1; then
        test_pass
    else
        test_fail "Failed to list channels"
    fi
}

# Test 3: Generate schedule (dry-run)
test_generate_schedule() {
    test_start "Generate schedule (dry-run)"
    
    if "$BINARY" --config "$CONFIG" generate --scheduler "$SCHEDULER" > /dev/null 2>&1; then
        test_pass
    else
        test_fail "Failed to generate schedule"
    fi
}

# Test 4: Scheduler list command
test_scheduler_list() {
    test_start "List scheduler blocks"
    
    if "$BINARY" scheduler list "$SCHEDULER" > /dev/null 2>&1; then
        test_pass
    else
        test_fail "Failed to list scheduler blocks"
    fi
}

# Main test execution
main() {
    log_info "Starting E2E tests for Schedularr"
    echo "========================================"
    
    check_prerequisites
    wait_for_tunarr
    
    # Run tests
    test_validate_configs
    test_list_channels
    test_generate_schedule
    test_scheduler_list
    
    # Summary
    echo ""
    echo "========================================"
    log_info "Test Summary"
    echo "  Total:  $TESTS_RUN"
    echo "  Passed: $TESTS_PASSED"
    echo "  Failed: $TESTS_FAILED"
    echo "========================================"
    
    if [ $TESTS_FAILED -eq 0 ]; then
        log_info "All tests passed! ✓"
        exit 0
    else
        log_error "Some tests failed"
        exit 1
    fi
}

main "$@"

