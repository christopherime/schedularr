# GitHub Copilot Instructions for Schedularr

This file provides context and guidelines for GitHub Copilot when suggesting code completions and generating code for the Schedularr project.

For general AI assistant guidance, see [AGENTS.md](../AGENTS.md).

## Project Context

**Schedularr** automates TV channel scheduling for Tunarr using cron-based blocks with intelligent content filtering and series progression.

**Tech Stack:**

- Go 1.25.5
- Cobra (CLI) + Viper (config)
- Bubble Tea (TUI)
- CUE (schema validation)
- SQLite (state persistence)
- robfig/cron (scheduling)

## Code Style Rules

### Package Structure

```go
// ✅ Correct package comment format
// Package scheduler provides the core scheduling engine for Schedularr.
package scheduler

// ❌ Incorrect - missing "Package" prefix
// Scheduler provides the core scheduling engine.
package scheduler
```

### Logging Style

Always use structured logging with `log/slog`:

```go
// ✅ Correct - structured logging with snake_case fields
logger.Info("schedule generated",
    "block_name", block.Name,
    "program_count", len(programs),
    "duration_minutes", duration)

// ❌ Incorrect - printf-style logging
log.Printf("Generated %d programs for %s (%d min)", len(programs), block.Name, duration)
```

### Error Handling

Always wrap errors with context:

```go
// ✅ Correct - error wrapping with context
if err != nil {
    return fmt.Errorf("failed to fetch library %s: %w", libID, err)
}

// ❌ Incorrect - returning raw error
if err != nil {
    return err
}
```

### Function Signatures

Exported functions need godoc comments:

```go
// ✅ Correct - godoc comment present
// FilterPrograms filters the given programs based on filter criteria.
// Returns an error if the filter is invalid or malformed.
func FilterPrograms(programs []tunarr.Program, filter Filter) ([]tunarr.Program, error) {
    // implementation
}

// ❌ Incorrect - missing godoc
func FilterPrograms(programs []tunarr.Program, filter Filter) ([]tunarr.Program, error) {
    // implementation
}
```

## Common Patterns

### Table-Driven Tests

Use this pattern for test generation:

```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name: "descriptive test case name",
            input: InputType{/* ... */},
            want: OutputType{/* ... */},
            wantErr: false,
        },
        // more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("FunctionName() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("FunctionName() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### CLI Command Structure

When suggesting CLI commands:

```go
var commandCmd = &cobra.Command{
    Use:   "command [args]",
    Short: "Brief one-line description",
    Long: `Detailed description with:
- Feature list
- Usage examples
- Important notes`,
    Run: func(cmd *cobra.Command, args []string) {
        // Implementation
    },
}

func init() {
    rootCmd.AddCommand(commandCmd)
    commandCmd.Flags().StringVar(&variable, "flag", "default", "Description")
}
```

### Configuration Struct Tags

Always include all three tags:

```go
type Config struct {
    FieldName string `mapstructure:"field_name" yaml:"field_name" json:"field_name"`
    // mapstructure: for Viper
    // yaml: for YAML marshaling
    // json: for JSON marshaling
}
```

## Complexity Limits

Copilot should suggest refactoring when:

- Cyclomatic complexity > 15
- Cognitive complexity > 20
- Nesting depth > 5
- Function has > 5 parameters
- Function returns > 3 values

Suggest extracting:

- Helper functions for complex logic
- Early returns to reduce nesting
- Separate functions for different responsibilities

## Blocked Patterns

Never suggest these (enforced by depguard):

```go
// ❌ NEVER use these packages
import "github.com/pkg/errors"          // Use fmt.Errorf with %w
import "github.com/sirupsen/logrus"     // Use log/slog
import "crypto/md5"                      // Security risk
import "crypto/sha1"                     // Security risk
import "io/ioutil"                       // Deprecated
import "gopkg.in/yaml.v2"                // Use v3

// ✅ Use these instead
import (
    "fmt"
    "log/slog"
    "os"
    "gopkg.in/yaml.v3"
)
```

## File Operations

Always clean user-provided paths:

```go
// ✅ Correct - clean path before use
func readConfig(filePath string) error {
    cleanPath := filepath.Clean(filePath)
    // #nosec G304 - user-provided path is intentional
    data, err := os.ReadFile(cleanPath)
    // ...
}

// ❌ Incorrect - using raw user input
func readConfig(filePath string) error {
    data, err := os.ReadFile(filePath)
    // ...
}
```

## Testing Conventions

### Mock Interfaces

When suggesting mocks for interfaces:

```go
// ✅ Correct mock pattern
type MockStateStore struct {
    GetSeriesStateFunc  func(showTitle string) (*SeriesState, error)
    SaveSeriesStateFunc func(state *SeriesState) error
}

func (m *MockStateStore) GetSeriesState(showTitle string) (*SeriesState, error) {
    if m.GetSeriesStateFunc != nil {
        return m.GetSeriesStateFunc(showTitle)
    }
    return nil, nil
}
```

### Test Helpers

Suggest helper functions for common test setups:

```go
func createTestEngine(t *testing.T) *Engine {
    t.Helper()
    client := &tunarr.Client{}
    store := NewMockStateStore()
    logger := slog.Default()
    return NewEngine(client, []Block{}, store, logger)
}
```

## CUE Schema Patterns

When working with CUE schemas:

```go
// CUE schema pattern for optional fields with defaults
#Config: {
    field: type | *default_value  // Optional with default
    required: type                 // Required field
    nested: {
        inner: string | *"default"
    }
}

// Instance with defaults applied
Config: #Config & {
    required: "value"
    // field and nested.inner will use defaults
}
```

## Copilot-Specific Tips

### Context Files

Copilot should prioritize these files for context:

1. `AGENTS.md` - General patterns and standards
2. `CLAUDE.md` - Project structure and commands
3. `TODO.md` - Current priorities and tasks
4. Files in the same package
5. Related test files

### Autocomplete Priority

When suggesting completions:

1. Follow existing patterns in the same file
2. Match naming conventions from similar functions
3. Use struct field names from the project
4. Suggest error handling for all fallible operations
5. Include logging for significant operations

### Code Generation

When generating new functions:

1. Include godoc comments
2. Add error handling
3. Use structured logging where appropriate
4. Follow complexity limits
5. Suggest accompanying tests

## Common Completions

### Fetching from Tunarr

```go
// When seeing Tunarr client usage:
programs, err := client.GetLibraryPrograms(libraryID)
if err != nil {
    return fmt.Errorf("failed to fetch programs from library %s: %w", libraryID, err)
}
logger.Info("fetched programs", "library_id", libraryID, "count", len(programs))
```

### State Management

```go
// When seeing series state operations:
state, err := e.getSeriesState(showTitle)
if err != nil {
    e.logger.Error("failed to get series state",
        "show_title", showTitle,
        "error", err)
    return nil, fmt.Errorf("failed to get series state for %s: %w", showTitle, err)
}
```

### Schedule Generation

```go
// When seeing schedule generation code:
plan, err := engine.GenerateForTimeRange(start, end, programs)
if err != nil {
    logger.Error("schedule generation failed",
        "start", start.Format(time.RFC3339),
        "end", end.Format(time.RFC3339),
        "error", err)
    return fmt.Errorf("failed to generate schedule: %w", err)
}
logger.Info("schedule generated",
    "start", start.Format(time.RFC3339),
    "end", end.Format(time.RFC3339),
    "channel_count", len(plan))
```

## Documentation Suggestions

When suggesting comments:

- Explain _why_, not _what_ (code shows what)
- Document non-obvious behavior
- Explain performance considerations
- Note any limitations or edge cases

```go
// ✅ Good comment - explains why
// Use pending states to ensure atomic commits.
// If schedule application fails, we can rollback without
// corrupting the database.
e.pendingStates[showTitle] = state

// ❌ Bad comment - just repeats code
// Set the pending state for the show title
e.pendingStates[showTitle] = state
```

## Integration Points

### With CUE Validation

```go
// When validating configs, use CUE validator
validator := cueconfig.NewValidator()
if err := validator.ValidateScheduler(data, "yaml"); err != nil {
    return fmt.Errorf("scheduler validation error: %w", err)
}
```

### With SQLite Store

```go
// Always defer Close() for store
store, err := store.New("schedularr.db")
if err != nil {
    return fmt.Errorf("failed to open database: %w", err)
}
defer store.Close()
```

### With Logging

```go
// Create logger from config at startup
logger := logging.NewLogger(cfg.Log.Level, cfg.Log.Format)
logging.SetDefault(logger)  // Set as default for the app
```

## Quick Reference

**Key Directories:**

- `cmd/` - CLI commands
- `internal/scheduler/` - Core scheduling logic
- `internal/tunarr/` - Tunarr API client
- `internal/config/` - Configuration loading
- `internal/store/` - SQLite persistence
- `internal/logging/` - Structured logging

**Key Files:**

- `main.go` - Entry point
- `internal/scheduler/engine.go` - Main scheduling engine
- `internal/scheduler/filter.go` - Content filtering
- `cmd/schema/*.cue` - Configuration schemas

**Run Commands:**

- `make build` - Build binary
- `make test` - Run tests
- `make lint` - Run linters
- `go test ./...` - Quick test
- `./bin/schedularr --help` - CLI help

---

_For detailed guidance, see [AGENTS.md](../AGENTS.md) and [CLAUDE.md](../CLAUDE.md)._
