# GEMINI.md

This file provides guidance to Gemini Code Assist when working with the Schedularr codebase.

For general AI assistant guidance, see [AGENTS.md](AGENTS.md).
For Claude Code specific instructions, see [CLAUDE.md](CLAUDE.md).

## Project Context

Schedularr automates TV channel scheduling for Tunarr using cron-based blocks with intelligent filtering and series progression tracking.

**Quick Facts:**
- Language: Go 1.25.5
- Pattern: Follows athena project architecture
- Key Files: See [CLAUDE.md](CLAUDE.md) "Project Structure" section
- Current Phase: Phase 4 (Operational Excellence)

## Gemini-Specific Workflow

### When Starting a Task

1. **Understand the request:**
   - Check [TODO.md](TODO.md) for task context and priorities
   - Review related code using the codebase context window
   - Ask clarifying questions if requirements are unclear

2. **Plan your approach:**
   - For complex changes, outline your plan first
   - Break down into logical steps
   - Consider edge cases and error handling

3. **Execute systematically:**
   - Make changes incrementally
   - Test after each significant change
   - Update tests alongside code changes

### Code Generation Best Practices

#### Prefer Specific Context

When Gemini needs to modify code:

```
❌ BAD: "Add logging to the scheduler"
✅ GOOD: "Add structured slog logging to internal/scheduler/engine.go
         in the planFilterBlock function, logging the number of filtered
         candidates and block name"
```

#### Use Project Patterns

Always follow existing patterns in the codebase:

```go
// ✅ GOOD - Follows project logging pattern
e.logger.Info("filtered candidates",
    "original_count", originalCount,
    "filtered_count", len(candidates),
    "block_name", block.Name)

// ❌ BAD - Doesn't match project style  
log.Printf("Filtered %d programs for %s", count, name)
```

#### Leverage Codebase Context

Gemini has excellent multi-file context. Use it to:
- Understand how similar features are implemented
- Follow naming conventions
- Match code style and patterns
- Reuse existing helper functions

### Testing with Gemini

#### Generate Table-Driven Tests

Gemini excels at generating comprehensive test cases:

```go
// Ask: "Generate table-driven tests for FilterPrograms with edge cases"

func TestFilterPrograms(t *testing.T) {
    tests := []struct {
        name     string
        programs []tunarr.Program
        filter   Filter
        want     int
        wantErr  bool
    }{
        {
            name: "empty programs list",
            programs: []tunarr.Program{},
            filter: Filter{Genres: []string{"Comedy"}},
            want: 0,
            wantErr: false,
        },
        {
            name: "filter matches all",
            programs: []tunarr.Program{
                {Title: "Show1", Genres: []string{"Comedy"}},
                {Title: "Show2", Genres: []string{"Comedy"}},
            },
            filter: Filter{Genres: []string{"Comedy"}},
            want: 2,
            wantErr: false,
        },
        // ... more cases
    }
    // ... test execution
}
```

#### Test Coverage Analysis

Ask Gemini to:
- "Identify untested code paths in engine.go"
- "Generate tests for error cases in client.go"
- "Add edge case tests for the filtering logic"

### Documentation with Gemini

#### Generate Godoc Comments

```
Request: "Add godoc comments to all exported functions in scheduler/types.go"

Result:
// Block represents a scheduling block with timing, content filtering, and priority.
// Each block defines when content should be scheduled (via Cron), what content
// to include (via Filter or Series), and how to handle conflicts (via Priority).
type Block struct {
    // Name is a human-readable identifier for this block
    Name string `yaml:"name" json:"name"`
    // ... etc
}
```

#### Update Documentation

Ask Gemini to update docs when code changes:
- "Update ARCHITECTURE.md to reflect the new series state flow"
- "Add examples to CLAUDE.md for the new CLI command"

### Common Gemini Requests

#### Code Refactoring

```
"Refactor resolveConflicts to reduce cognitive complexity below 20
while maintaining the same logic and test compatibility"
```

#### Error Handling

```
"Add proper error wrapping with context to all error returns in
internal/tunarr/client.go following the project's error handling pattern"
```

#### Feature Implementation

```
"Implement episode skipping for series blocks:
1. Add SkipEpisodes []int field to SeriesConfig in types.go
2. Update planSeriesBlock to check and skip episodes in the list
3. Add validation in CUE schema to ensure valid episode numbers
4. Generate table-driven tests covering skip logic"
```

## Gemini Strengths for Schedularr

### 1. Multi-File Refactoring

Gemini can refactor across multiple files simultaneously:
- Update function signatures and all call sites
- Rename types and update all references
- Add parameters to constructors across the codebase

### 2. Pattern Matching

Gemini excels at matching existing patterns:
- Following established code style
- Using consistent naming conventions
- Replicating error handling patterns
- Maintaining test structure

### 3. Documentation Generation

Gemini can generate comprehensive docs:
- API documentation from code
- Usage examples from test cases
- Migration guides when changing APIs
- Architecture diagrams from code flow

### 4. Test Generation

Gemini generates thorough test coverage:
- Table-driven tests with edge cases
- Mock implementations for interfaces
- Integration test scaffolding
- Error case coverage

## Project-Specific Gemini Tips

### Working with CUE Schemas

When modifying configurations:
1. Always update the CUE schema first (`cmd/schema/*.cue`)
2. Then update Go structs to match
3. Test with `./schedularr validate`

Example request:
```
"Add a MaxRetries field to the Tunarr config:
1. Add to cmd/schema/config.cue with default value 3
2. Add to internal/tunarr/config.go
3. Update client.go to use the MaxRetries value"
```

### Understanding State Management

The project uses a pending states pattern. When working with state:

```
"Add a new series state field:
1. Update SeriesState struct in scheduler/state.go
2. Add database migration in store/sqlite.go
3. Update getSeriesState and saveSeriesState methods
4. Add pending state handling in engine.Commit"
```

### CLI Command Development

When adding CLI commands:

```
"Create a new 'schedularr backup' command that:
1. Creates cmd/backup.go following the pattern in cmd/validate.go
2. Backs up the SQLite database and config files
3. Supports --output flag for backup location
4. Includes progress indicators using lipgloss styles
5. Adds help text and examples"
```

## Quality Checks for Gemini

Before considering a task complete, verify:

### Code Quality
- [ ] `go fmt ./...` - Code is formatted
- [ ] `golangci-lint run` - No linting errors
- [ ] `gosec ./...` - No security issues
- [ ] `govulncheck ./...` - No vulnerabilities

### Testing
- [ ] `go test ./...` - All tests pass
- [ ] `go test -cover ./...` - Coverage maintained or improved
- [ ] New tests added for new functionality
- [ ] Edge cases covered

### Documentation
- [ ] Godoc comments on exported functions
- [ ] README/docs updated if user-facing changes
- [ ] CHANGELOG.md entry for significant changes

### Build & Run
- [ ] `make build` - Compiles successfully
- [ ] `./bin/schedularr --help` - CLI works
- [ ] Manual testing of new features

## Gemini Limitations to Note

### 1. Long-Running Operations

Gemini may lose context during very long operations. For large refactorings:
- Break into smaller steps
- Verify each step before proceeding
- Save progress frequently

### 2. CUE Schema Syntax

CUE has unique syntax. When modifying schemas:
- Review existing CUE files for patterns
- Test validation after changes
- Refer to CUE documentation if unsure

### 3. Complex Business Logic

For complex scheduling algorithms:
- Review existing logic carefully
- Add extensive tests
- Consider edge cases explicitly
- Validate with real data if possible

## Example Workflows

### Adding a New Filter Criterion

1. **Update CUE Schema:**
```
"Add actor filtering to scheduler.cue:
- Add actors: [...string] field to #Filter
- Make it optional with default []
- Add example in the schema instance"
```

2. **Update Go Types:**
```
"Add Actors []string field to Filter struct in scheduler/types.go
with appropriate tags"
```

3. **Implement Logic:**
```
"Update FilterPrograms in scheduler/filter.go to filter by actors,
following the existing genre filtering pattern"
```

4. **Add Tests:**
```
"Generate table-driven tests for actor filtering with cases:
- Empty actors list (should match all)
- Single actor match
- Multiple actors (OR logic)
- No match case
- Case insensitive matching"
```

### Debugging with Gemini

When encountering issues:

```
"Analyze why TestPlanBlock_Series is failing:
1. Show the test code
2. Show the implementation
3. Identify the mismatch
4. Suggest a fix with explanation"
```

## Useful Gemini Prompts

### Code Analysis
- "Find all functions with complexity >15 in scheduler package"
- "Identify error paths missing test coverage in tunarr/client.go"
- "Show all places where StateStore interface is used"

### Refactoring
- "Extract duplicate error handling into a helper function"
- "Simplify this if-else chain using early returns"
- "Reduce nesting in this function to max depth 3"

### Documentation
- "Generate ARCHITECTURE.md section describing the filter system"
- "Create usage examples for all scheduler init templates"
- "Write migration guide for v0.4 to v0.5 config changes"

### Testing
- "Add integration tests for the full generate workflow"
- "Create benchmark tests for FilterPrograms with 1000 programs"
- "Generate fuzz tests for cron expression parsing"

## Getting Help

If Gemini is unsure or stuck:
1. Consult [AGENTS.md](AGENTS.md) for general patterns
2. Review [CLAUDE.md](CLAUDE.md) for project specifics
3. Check existing code for similar implementations
4. Refer to [TODO.md](TODO.md) for context
5. Ask the user for clarification

---

*This guidance is optimized for Gemini Code Assist's strengths in pattern matching, multi-file context, and code generation.*
