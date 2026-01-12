# Contributing to Schedularr

Thank you for your interest in contributing to Schedularr! This document provides guidelines and information for contributors.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Development Workflow](#development-workflow)
- [Coding Standards](#coding-standards)
- [Commit Message Format](#commit-message-format)
- [Pull Request Process](#pull-request-process)
- [Code Review Checklist](#code-review-checklist)
- [Testing Guidelines](#testing-guidelines)
- [Documentation](#documentation)

## Code of Conduct

This project follows a Code of Conduct to ensure a welcoming environment for all contributors. By participating, you agree to:

- Be respectful and inclusive
- Welcome newcomers and help them learn
- Focus on constructive feedback
- Assume good intentions
- Prioritize the project's best interests

## Getting Started

### Prerequisites

- **Go 1.25.5 or later** - [Install Go](https://go.dev/doc/install)
- **Git** - [Install Git](https://git-scm.com/downloads)
- **golangci-lint** - [Install golangci-lint](https://golangci-lint.run/usage/install/)
- **gosec** - `go install github.com/securego/gosec/v2/cmd/gosec@latest`
- **govulncheck** - `go install golang.org/x/vuln/cmd/govulncheck@latest`

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork:
```bash
git clone https://github.com/YOUR_USERNAME/schedularr.git
cd schedularr
```

3. Add upstream remote:
```bash
git remote add upstream https://github.com/geekxflood/schedularr.git
```

## Development Setup

### Install Dependencies

```bash
# Download Go dependencies
go mod download

# Verify everything works
make build
make test
make lint
```

### Project Structure

```
schedularr/
├── main.go                 # Application entry point
├── cmd/                    # CLI commands (Cobra)
│   ├── root.go
│   ├── generate.go
│   ├── validate.go
│   └── schema/            # CUE schemas
├── internal/              # Private application code
│   ├── config/           # Configuration management
│   ├── scheduler/        # Core scheduling engine
│   ├── tunarr/          # Tunarr API client
│   ├── store/           # SQLite persistence
│   ├── logging/         # Structured logging
│   └── tui/             # Terminal UI
├── docs/                # Documentation
├── Makefile            # Build automation
└── TODO.md             # Development roadmap
```

See [CLAUDE.md](CLAUDE.md) for detailed architecture documentation.

## Development Workflow

### 1. Create a Branch

Create a branch for your work:

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/bug-description
```

Branch naming conventions:
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring
- `test/` - Test additions/improvements

### 2. Make Changes

Follow the [Coding Standards](#coding-standards) below.

### 3. Test Your Changes

```bash
# Run tests
make test

# Run linters
make lint

# Build to ensure it compiles
make build

# Test the binary
./bin/schedularr --help
```

### 4. Commit Your Changes

Follow the [Commit Message Format](#commit-message-format) below.

### 5. Push and Create PR

```bash
git push origin feature/your-feature-name
```

Then create a Pull Request on GitHub.

## Coding Standards

### Go Code Style

Follow standard Go conventions plus project-specific standards:

#### Exported Functions

All exported functions/types must have godoc comments:

```go
// NewEngine creates a new scheduling engine with the given parameters.
// The logger parameter is optional; if nil, slog.Default() will be used.
func NewEngine(client *tunarr.Client, blocks []Block, store StateStore, logger *slog.Logger) *Engine {
    // implementation
}
```

#### Structured Logging

Use `log/slog` with structured fields (snake_case):

```go
logger.Info("schedule generated",
    "block_name", block.Name,
    "program_count", len(programs),
    "duration_minutes", duration)
```

#### Error Handling

Always wrap errors with context:

```go
if err != nil {
    return fmt.Errorf("failed to fetch library %s: %w", libID, err)
}
```

#### Complexity Limits

Keep functions simple and maintainable:
- **Cyclomatic complexity:** max 15
- **Cognitive complexity:** max 20
- **Nesting depth:** max 5
- **Function parameters:** max 5
- **Function return values:** max 3

If you exceed these limits, refactor by extracting helper functions.

#### Blocked Packages

Never use these packages (enforced by linters):
- `github.com/pkg/errors` → Use `fmt.Errorf` with `%w`
- `github.com/sirupsen/logrus` → Use `log/slog`
- `crypto/md5`, `crypto/sha1` → Security risk
- `io/ioutil` → Deprecated, use `os` and `io`
- `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2` → Use v3

### Configuration Changes

When adding configuration fields:

1. Update CUE schema in `cmd/schema/*.cue`
2. Add field to Go struct with proper tags:
```go
Field string `mapstructure:"field" yaml:"field" json:"field"`
```
3. Test with `./schedularr validate`

### Adding CLI Commands

1. Create file in `cmd/` directory
2. Use Cobra command pattern
3. Add to `rootCmd` in `init()` function
4. Include help text and examples

See existing commands for examples.

## Commit Message Format

We use [Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

- `feat` - New feature
- `fix` - Bug fix
- `docs` - Documentation changes
- `style` - Code style changes (formatting, etc.)
- `refactor` - Code refactoring
- `test` - Adding or updating tests
- `chore` - Maintenance tasks
- `perf` - Performance improvements

### Examples

**Simple feature:**
```
feat(scheduler): add episode skip functionality

Allow users to skip specific episodes in series blocks
by specifying episode numbers in the configuration.
```

**Bug fix:**
```
fix(tunarr): handle connection timeout gracefully

Add exponential backoff retry logic for Tunarr API calls
to prevent crashes when Tunarr is temporarily unavailable.

Closes #123
```

**Breaking change:**
```
feat(config)!: migrate to CUE schema validation

BREAKING CHANGE: Configuration format has changed.
Old YAML configs must be migrated using the provided tool.

Migration guide: docs/MIGRATION.md
```

### Commit Message Guidelines

- **Subject line:**
  - Use imperative mood ("add" not "added")
  - Don't capitalize first letter
  - No period at the end
  - Max 72 characters

- **Body (optional):**
  - Explain what and why, not how
  - Wrap at 72 characters
  - Separate from subject with blank line

- **Footer (optional):**
  - Reference issues: `Closes #123` or `Fixes #456`
  - Note breaking changes
  - Co-authored-by for pair programming

## Pull Request Process

### Before Submitting

1. **Sync with upstream:**
```bash
git fetch upstream
git rebase upstream/main
```

2. **Run all checks:**
```bash
make lint
make test
make build
```

3. **Update documentation:**
   - Add/update godoc comments
   - Update README.md if user-facing changes
   - Update TODO.md to mark completed tasks

### PR Title

Use the same format as commit messages:
```
feat(scheduler): add episode skip functionality
```

### PR Description Template

```markdown
## Description
Brief description of the changes.

## Type of Change
- [ ] Bug fix (non-breaking change fixing an issue)
- [ ] New feature (non-breaking change adding functionality)
- [ ] Breaking change (fix or feature causing existing functionality to change)
- [ ] Documentation update

## How Has This Been Tested?
Describe the tests you ran and how to reproduce.

## Checklist
- [ ] Code follows project style guidelines
- [ ] I have performed a self-review
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have updated the documentation
- [ ] My changes generate no new warnings
- [ ] I have added tests that prove my fix/feature works
- [ ] New and existing tests pass locally
- [ ] I have updated TODO.md if applicable

## Related Issues
Closes #(issue number)
```

### PR Size

Keep PRs focused and reasonably sized:
- **Small** (preferred): <200 lines changed
- **Medium**: 200-500 lines changed
- **Large**: >500 lines (consider splitting)

Large PRs may take longer to review and merge.

## Code Review Checklist

### For Reviewers

When reviewing PRs, check:

#### Functionality
- [ ] Code works as described
- [ ] Edge cases are handled
- [ ] Error handling is appropriate
- [ ] No obvious bugs

#### Code Quality
- [ ] Follows Go best practices
- [ ] Complexity limits respected
- [ ] No code duplication
- [ ] Clear naming conventions
- [ ] Appropriate use of comments

#### Testing
- [ ] Tests are included
- [ ] Tests cover edge cases
- [ ] All tests pass
- [ ] Test coverage maintained/improved

#### Documentation
- [ ] Godoc comments on exported items
- [ ] README updated if needed
- [ ] CHANGELOG updated if needed
- [ ] TODO.md updated if applicable

#### Security
- [ ] No security vulnerabilities
- [ ] Input validation where needed
- [ ] No hardcoded secrets
- [ ] gosec passes

### For Authors

When your PR is reviewed:
- **Respond promptly** to feedback
- **Ask questions** if feedback is unclear
- **Make requested changes** or discuss alternatives
- **Mark conversations as resolved** after addressing
- **Be patient** and respectful

## Testing Guidelines

### Writing Tests

Use table-driven tests:

```go
func TestFilterPrograms(t *testing.T) {
    tests := []struct {
        name     string
        programs []tunarr.Program
        filter   Filter
        want     int
        wantErr  bool
    }{
        {
            name: "filter by genre",
            programs: []tunarr.Program{
                {Title: "Show1", Genres: []string{"Comedy"}},
            },
            filter: Filter{Genres: []string{"Comedy"}},
            want: 1,
            wantErr: false,
        },
        // more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FilterPrograms(tt.programs, tt.filter)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if len(got) != tt.want {
                t.Errorf("got %d programs, want %d", len(got), tt.want)
            }
        })
    }
}
```

### Test Coverage

- **Target:** >80% coverage across all packages
- **Check coverage:** `go test -cover ./...`
- **View detailed report:** `go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out`

### Testing Best Practices

- Test public API, not internal implementation
- Use meaningful test names describing the scenario
- Test edge cases and error conditions
- Keep tests simple and focused
- Use test helpers to reduce duplication
- Mock external dependencies (Tunarr API, database)

## Documentation

### When to Update Documentation

Update documentation when:
- Adding new features
- Changing user-facing behavior
- Modifying configuration format
- Adding CLI commands
- Changing API interfaces

### Documentation Files

- **README.md** - Project overview and quick start
- **CLAUDE.md** - Development guide and architecture
- **TODO.md** - Development roadmap and tasks
- **ROADMAP.md** - Project vision and future plans
- **docs/** - Detailed documentation
- **Godoc comments** - In-code documentation

### Writing Good Documentation

- **Be clear and concise** - Use simple language
- **Provide examples** - Show, don't just tell
- **Keep it updated** - Remove outdated information
- **Be consistent** - Follow existing style
- **Consider the audience** - Write for users and developers

## Getting Help

### Resources

- **Documentation:** [docs/](docs/) directory
- **Architecture:** [CLAUDE.md](CLAUDE.md)
- **AI Assistants:** [AGENTS.md](AGENTS.md), [GEMINI.md](GEMINI.md)
- **Project Status:** [TODO.md](TODO.md), [ROADMAP.md](ROADMAP.md)

### Communication

- **Questions:** Open a [GitHub Discussion](https://github.com/geekxflood/schedularr/discussions)
- **Bugs:** Create a [GitHub Issue](https://github.com/geekxflood/schedularr/issues)
- **Features:** Open a discussion first, then create an issue

### Response Time

- Issues and PRs are typically reviewed within 48 hours
- Urgent security issues are prioritized
- Complex changes may take longer to review

## License

By contributing to Schedularr, you agree that your contributions will be licensed under the same license as the project.

## Recognition

All contributors will be recognized in the project. Significant contributions may be highlighted in release notes.

Thank you for contributing to Schedularr! 🎉
