# AGENTS.md

This file provides general guidance for AI coding assistants (Claude Code, GitHub Copilot, Cursor, etc.) when working with the Schedularr codebase.

For Claude Code specific instructions, see [CLAUDE.md](CLAUDE.md).
For Gemini Code Assist specific instructions, see [GEMINI.md](GEMINI.md).
For GitHub Copilot specific instructions, see [.github/copilot-instructions.md](.github/copilot-instructions.md).

## Project Overview

Schedularr is a Go application that automates content scheduling for Tunarr TV channels using cron-based recurring blocks with advanced filtering and series progression.

**Key Technologies:**

- **Language:** Go 1.25.5
- **CLI Framework:** Cobra + Viper
- **TUI Framework:** Bubble Tea
- **Configuration:** CUE schemas with YAML/JSON
- **Database:** SQLite (for state persistence)
- **Scheduling:** robfig/cron for cron expressions

## Architecture Patterns

This project follows architectural patterns from the athena project. Key patterns include:

1. **CUE Schema Validation** - All configurations validated against CUE schemas
2. **Structured Logging** - Use `log/slog` with JSON/text formats
3. **CLI Structure** - `main.go` + `cmd/` package pattern
4. **Code Quality** - Strict linting with golangci-lint
5. **Error Handling** - Always wrap errors with context

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed architecture documentation.

## Development Workflow

### Before Making Changes

1. **Read the context:**
   - Check [TODO.md](TODO.md) for current priorities
   - Review [ROADMAP.md](ROADMAP.md) for project direction
   - Read [CLAUDE.md](CLAUDE.md) for development commands and patterns

2. **Understand the codebase:**
   - Project structure is in [CLAUDE.md](CLAUDE.md) under "Architecture Overview"
   - Key components and data flow are documented
   - Review existing code in the area you're modifying

3. **Plan your approach:**
   - For significant features, consider using plan mode
   - Break down complex tasks into smaller steps
   - Consider impact on existing functionality

### Code Style and Standards

#### Go Code Style

Follow standard Go conventions plus project-specific standards:

```go
// ✅ GOOD - Exported functions have godoc comments
// NewEngine creates a new scheduling engine with the given parameters.
// The logger parameter is optional; if nil, slog.Default() will be used.
func NewEngine(client *tunarr.Client, blocks []Block, store StateStore, logger *slog.Logger) *Engine {
    if logger == nil {
        logger = slog.Default()
    }
    return &Engine{
        client: client,
        blocks: blocks,
        logger: logger,
    }
}

// ✅ GOOD - Structured logging with snake_case fields
e.logger.Info("scheduling conflict resolved",
    "winner_block", winnerBlock.Name,
    "loser_block", loserBlock.Name,
    "priority_diff", winnerBlock.Priority - loserBlock.Priority)

// ✅ GOOD - Error wrapping with context
if err != nil {
    return fmt.Errorf("failed to fetch programs from library %s: %w", libID, err)
}

// ❌ BAD - Missing godoc comment
func NewEngine(client *tunarr.Client, blocks []Block) *Engine {
    // ...
}

// ❌ BAD - Unstructured logging
log.Printf("Conflict: %s wins over %s", winner, loser)

// ❌ BAD - Error without context
if err != nil {
    return err
}
```

#### Complexity Limits

Enforced by golangci-lint:

- **Cyclomatic complexity:** max 15
- **Cognitive complexity:** max 20
- **Nesting depth:** max 5
- **Function results:** max 3
- **Function arguments:** max 5

If you exceed these limits, refactor by:

- Extracting helper functions
- Using early returns to reduce nesting
- Splitting complex functions into smaller pieces

#### Blocked Packages

Never use these packages (enforced by depguard):

- `github.com/pkg/errors` → Use `fmt.Errorf` with `%w`
- `github.com/sirupsen/logrus` → Use `log/slog`
- `crypto/md5`, `crypto/sha1` → Security risk
- `io/ioutil` → Deprecated, use `os` and `io`
- `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2` → Use `gopkg.in/yaml.v3`

### Testing Guidelines

#### Test Structure

Use table-driven tests for comprehensive coverage:

```go
func TestFilterPrograms(t *testing.T) {
    tests := []struct {
        name     string
        programs []tunarr.Program
        filter   Filter
        expected int
        wantErr  bool
    }{
        {
            name: "filter by genre",
            programs: []tunarr.Program{
                {Title: "Show1", Genres: []string{"Comedy"}},
                {Title: "Show2", Genres: []string{"Drama"}},
            },
            filter: Filter{Genres: []string{"Comedy"}},
            expected: 1,
            wantErr: false,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := FilterPrograms(tt.programs, tt.filter)
            if (err != nil) != tt.wantErr {
                t.Errorf("FilterPrograms() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if len(result) != tt.expected {
                t.Errorf("FilterPrograms() returned %d programs, expected %d", len(result), tt.expected)
            }
        })
    }
}
```

#### Test Coverage Goals

- **Target:** >80% coverage across all packages
- **Current Status:** See `go test -cover ./...`
- **Priority Areas:**
  - Core scheduling logic (engine.go)
  - Filter operations (filter.go)
  - State management (state.go)
  - API client (tunarr/client.go)

### Common Development Tasks

#### Adding a New CLI Command

1. Create new file in `cmd/` directory (e.g., `cmd/mycommand.go`)
2. Define command with Cobra:

```go
var myCmd = &cobra.Command{
    Use:   "mycommand [args]",
    Short: "Brief description",
    Long: `Detailed description with examples...`,
    Run: func(cmd *cobra.Command, args []string) {
        // Implementation
    },
}

func init() {
    rootCmd.AddCommand(myCmd)
    myCmd.Flags().StringVar(&myVar, "flag", "default", "Description")
}
```

#### Adding a New Configuration Field

1. Update CUE schema in `cmd/schema/*.cue`
2. Add field to Go struct in `internal/config/config.go` or `internal/scheduler/types.go`
3. Add struct tags: `mapstructure:"field" yaml:"field" json:"field"`
4. Update example configs (if creating new ones)
5. Test validation with `./schedularr validate`

#### Adding Structured Logging

Always use slog with structured fields:

```go
// Create logger from config
logger := logging.NewLogger(cfg.Log.Level, cfg.Log.Format)

// Use throughout code
logger.Info("operation completed",
    "operation", "schedule_generation",
    "duration_ms", duration.Milliseconds(),
    "block_count", len(blocks))

logger.Error("operation failed",
    "operation", "fetch_programs",
    "library_id", libID,
    "error", err)
```

### Quality Checks

Run these before committing:

```bash
# Format code
go fmt ./...

# Run tests
go test ./...

# Run linters
make lint
# or manually:
golangci-lint run
gosec ./...
govulncheck ./...

# Build to ensure it compiles
make build

# Validate configurations
make validate
```

## Commit Message Format

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**

```
feat(scheduler): add support for episode skip in series blocks

Implement episode skipping functionality for series-based scheduling.
Users can now specify episodes to skip in the series configuration.

- Add SkipEpisodes field to SeriesConfig
- Update planSeriesBlock to check skip list
- Add tests for episode skipping logic

Closes #123
```

```
fix(tunarr): handle connection timeout gracefully

Add exponential backoff retry logic for Tunarr API calls.
Prevents crashes when Tunarr is temporarily unavailable.

- Implement retry with exponential backoff
- Add configurable max retries
- Log retry attempts at debug level
```

## Common Pitfalls

### 1. Forgetting to Update Tests

```go
// ❌ BAD - Changed function signature but didn't update tests
func NewEngine(client *tunarr.Client, blocks []Block, store StateStore, logger *slog.Logger) *Engine

// In test file (will fail to compile):
engine := NewEngine(client, blocks, store) // Missing logger parameter
```

**Always update tests when changing function signatures!**

### 2. Not Using CUE Generation

```go
// ❌ BAD - Hardcoding templates
const template = `...yaml template...`

// ✅ GOOD - Use CUE schema to generate configs
validator := cueconfig.NewValidator()
data, err := validator.GenerateScheduler("yaml")
```

### 3. Mixing Logging Styles

```go
// ❌ BAD - Mixing log and slog
log.Printf("Starting operation")
e.logger.Info("operation started")

// ✅ GOOD - Consistent slog usage
e.logger.Info("operation starting")
e.logger.Info("operation completed")
```

### 4. Not Cleaning File Paths

```go
// ❌ BAD - Using user input directly
data, err := os.ReadFile(userPath)

// ✅ GOOD - Clean path first
cleanPath := filepath.Clean(userPath)
// #nosec G304 - user-provided path is intentional
data, err := os.ReadFile(cleanPath)
```

## Useful Resources

- **Go Documentation:** <https://go.dev/doc/>
- **Cobra Guide:** <https://github.com/spf13/cobra>
- **CUE Language:** <https://cuelang.org/>
- **golangci-lint:** <https://golangci-lint.run/>
- **Conventional Commits:** <https://www.conventionalcommits.org/>
- **Tunarr Documentation:** (see docs/TUNARR_API_RESEARCH.md)

## Project-Specific Notes

### State Management

- Series state stored in SQLite database (`schedularr.db`)
- Use pending states pattern for atomic commits
- Always call `engine.Commit()` after successful schedule application
- Call `engine.Rollback()` on errors

### Cron Expressions

- Use 5-field format: `minute hour dom month dow`
- Parser: `cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)`
- Validate in CUE schema, not in Go code

### Tunarr Integration

- All API calls go through `internal/tunarr/client.go`
- Client has built-in retry logic with exponential backoff
- Use optional API key auth via `X-API-Key` header
- Content fetched from libraries, not direct program endpoints

## Questions or Issues?

- Check [TODO.md](TODO.md) for known issues
- Review [ROADMAP.md](ROADMAP.md) for planned features
- See [docs/](docs/) for detailed documentation
- Create a GitHub issue for bugs or feature requests

---

*This guidance is for AI coding assistants. For human developers, also see [CONTRIBUTING.md](CONTRIBUTING.md).*
