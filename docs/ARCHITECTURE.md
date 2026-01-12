# Architecture Alignment with Athena Project

This document describes the architectural patterns adopted from the athena project to improve Schedularr's code quality, maintainability, and operational excellence.

## Overview

The athena project is a well-structured Go application that follows industry best practices for configuration management, CLI design, testing, and deployment. Schedularr is adopting these patterns to ensure consistency across projects and leverage proven architectural decisions.

## Key Patterns Adopted

### 1. CUE Schema Validation

**Pattern:** Use CUE language for configuration validation with type safety and default values.

**Implementation:**

- `configs/schema/config.cue` - Application configuration schema
- `configs/schema/scheduler.cue` - Scheduler configuration schema
- Integration with config loading to validate on startup
- Detailed error messages with line numbers and suggestions

**Benefits:**

- Type-safe configuration with compile-time validation
- Default values defined in schema
- Self-documenting configuration structure
- Prevents invalid configurations from starting the application

**Example from athena:**

```cue
#Config: {
    server: {
        port:          int | *9600
        read_timeout:  string | *"30s"
        write_timeout: string | *"30s"
    }
    logging: {
        level:  "debug" | "info" | "warn" | "error" | *"info"
        format: "json" | "text" | *"json"
    }
}
```

### 2. CLI Command Structure

**Pattern:** Standardized command structure with clear separation of concerns.

**Commands:**

- **Root (no verb):** `./schedularr` - Default behavior launches TUI
- **validate:** `./schedularr validate [file]` - Validate configuration files
- **generate:** `./schedularr generate [options]` - Generate configuration templates
- **run:** `./schedularr run [options]` - Start the scheduling daemon

**Benefits:**

- Intuitive command structure
- Clear separation between validation, generation, and execution
- Consistent with other CLI tools in the ecosystem

### 3. Code Quality Standards

**Pattern:** Strict linting rules and coding standards enforced via golangci-lint.

**Standards:**

- Cyclomatic complexity: max 15
- Cognitive complexity: max 20
- Nesting depth: max 5
- Function results: max 3
- Arguments: max 5

**Blocked Packages:**

- `github.com/pkg/errors` (use stdlib `fmt.Errorf`)
- `logrus` (use `log/slog`)
- `crypto/md5`, `crypto/sha1` (security)
- `io/ioutil` (deprecated)
- `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2` (use v3)

**Benefits:**

- Consistent code style across projects
- Reduced cognitive load when reading code
- Prevention of common security issues
- Use of modern Go idioms

### 4. Structured Logging

**Pattern:** Use `log/slog` for structured logging with JSON output.

**Implementation:**

- JSON format for production
- Text format for development
- Context fields (channel_id, block_name, etc.)
- snake_case for log field names

**Benefits:**

- Machine-parseable logs
- Easy integration with log aggregation systems
- Consistent log format across services
- Better debugging with structured context

### 5. Error Handling

**Pattern:** Always wrap errors with context using `fmt.Errorf`.

**Implementation:**

```go
if err != nil {
    return fmt.Errorf("failed to query tunarr: %w", err)
}
```

**Benefits:**

- Clear error context throughout the call stack
- Easy to trace error origins
- Supports error unwrapping with `errors.Is` and `errors.As`

### 6. Documentation Structure

**Pattern:** Comprehensive documentation following a standard structure.

**Documents:**

- `docs/ARCHITECTURE.md` - System architecture and data flow
- `docs/SPECIFICATIONS.md` - Detailed specifications and formats
- `CLAUDE.md` - AI assistant guidance and development commands
- `ROADMAP.md` - Project vision and development phases
- `CONTRIBUTING.md` - Contribution guidelines and PR process

**Benefits:**

- Easy onboarding for new contributors
- Clear project vision and roadmap
- Consistent documentation across projects
- AI-friendly development guidance

### 7. Build Tooling

**Pattern:** Makefile-based build system with standard targets.

**Targets:**

- `make build` - Build binary to `./bin/schedularr`
- `make test` - Run tests with race detector
- `make lint` - Run golangci-lint
- `make clean` - Remove build artifacts
- `make validate` - Validate all config files
- `make e2e-up` - Start E2E test environment
- `make e2e-down` - Stop E2E test environment

**Benefits:**

- Consistent build commands across projects
- Easy CI/CD integration
- Simplified local development workflow

### 8. Testing Infrastructure

**Pattern:** Comprehensive testing with unit, integration, and E2E tests.

**Implementation:**

- Table-driven tests for core functions
- Integration tests with mocked dependencies
- E2E tests with docker-compose
- Test fixtures and sample data
- >80% code coverage target

**Benefits:**

- High confidence in code changes
- Easy to add new test cases
- Reproducible test environments
- Catches regressions early

## Migration Path

The migration to these patterns is organized in Phase 0 of the TODO.md file. The recommended order is:

1. **CUE Schema Integration** - Establish configuration validation
2. **CLI Command Restructuring** - Align command structure
3. **Code Quality Alignment** - Update linting and logging
4. **Documentation** - Create comprehensive docs
5. **Build Tooling** - Standardize build process

## References

- Athena project: `/Users/christophe/athena`
- CUE language: <https://cuelang.org/>
- golangci-lint: <https://golangci-lint.run/>
- Conventional Commits: <https://www.conventionalcommits.org/>
