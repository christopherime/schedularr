# Test Fixtures

This directory contains sample data for testing Schedularr functionality.

## Directory Structure

```
testdata/
├── configs/          # Sample configuration files
│   ├── valid_config.yaml        # Valid application config
│   ├── valid_scheduler.yaml     # Valid scheduler with filter blocks
│   ├── series_scheduler.yaml    # Valid scheduler with series blocks
│   └── invalid_config.yaml      # Invalid config for error testing
├── programs/         # Sample program data (Tunarr API format)
│   ├── cartoons.json            # Animated shows for kids
│   ├── sitcoms.json             # Comedy series
│   └── movies.json              # Feature films
└── channels/         # Sample channel data
    └── channels.json            # Channel definitions
```

## Usage in Tests

### Loading Configuration Files

```go
func TestWithFixture(t *testing.T) {
    data, err := os.ReadFile("../../testdata/configs/valid_config.yaml")
    if err != nil {
        t.Fatal(err)
    }
    // Use data in test
}
```

### Loading Program Data

```go
func TestProgramFiltering(t *testing.T) {
    data, err := os.ReadFile("../../testdata/programs/cartoons.json")
    if err != nil {
        t.Fatal(err)
    }

    var programs []tunarr.Program
    if err := json.Unmarshal(data, &programs); err != nil {
        t.Fatal(err)
    }
    // Use programs in test
}
```

## Data Format

### Programs

Program durations are in milliseconds:
- 660000 ms = 11 minutes (typical half-length episode)
- 1320000 ms = 22 minutes (typical sitcom episode)
- 6960000 ms = 116 minutes (typical movie)

### Ratings

- **TV-Y**: All children
- **TV-Y7**: Directed to older children
- **TV-G**: General audience
- **TV-PG**: Parental guidance suggested
- **TV-14**: Parents strongly cautioned
- **PG**: Parental guidance suggested (movies)
- **PG-13**: Parents strongly cautioned (movies)
- **R**: Restricted (movies)

## Adding New Fixtures

When adding new test fixtures:

1. Follow the existing JSON format for programs and channels
2. Use realistic data that matches actual Tunarr API responses
3. Document any special characteristics in this README
4. Keep file sizes reasonable (< 100KB)
