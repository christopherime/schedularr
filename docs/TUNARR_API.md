# Tunarr API Integration

This document describes how Schedularr integrates with the Tunarr API and provides reference information for developers working on the integration.

## Tunarr Resources

- **Repository**: [github.com/chrisbenincasa/tunarr](https://github.com/chrisbenincasa/tunarr)
- **Documentation**: [tunarr.com](https://tunarr.com)
- **API Docs**: [tunarr.com/api-docs.html](https://tunarr.com/api-docs.html) (Scalar-based, requires JavaScript)
- **Discord**: [discord.gg/svgSBYkEK5](https://discord.gg/svgSBYkEK5)
- **Technology**: Node.js 22+ (TypeScript), React/Vite, SQLite

## Overview

Schedularr communicates with Tunarr through its REST API to:

- Fetch channel information
- Retrieve available media content (programs, shows, episodes)
- Update channel programming schedules

The integration is implemented in the `internal/tunarr` package.

## Authentication

Tunarr supports optional API key authentication via the `X-API-Key` header.

**Configuration:**

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: "your-api-key-here"  # Optional
```

## API Client

The `tunarr.Client` struct provides methods for interacting with the Tunarr API.

### Creating a Client

```go
import "github.com/geekxflood/schedularr/internal/tunarr"

// Create config
cfg := &tunarr.Config{
    URL:    "http://localhost:8000",
    APIKey: "your-api-key",  // Optional
}

// Create client
client := tunarr.NewClient(cfg, logger)
```

### Client Configuration

The client includes:

- **Retry Logic**: Automatic retry with exponential backoff (3 retries, 1s initial backoff)
- **Timeout**: 30-second timeout for all requests
- **Metrics**: Prometheus metrics for all API calls

## API Endpoints

### Channels

#### Get All Channels

Retrieves all available channels from Tunarr.

```go
channels, err := client.GetChannels(ctx)
```

**Endpoint:** `GET /api/channels`

**Response:** Array of `Channel` objects

**Channel Fields:**

- `ID` (string): Unique channel identifier
- `Name` (string): Channel display name
- `Number` (int): Channel number
- `Duration` (int64): Total programming duration in milliseconds

### Programs

#### Get All Programs

Retrieves all available programs/media from Tunarr.

```go
programs, err := client.GetPrograms(ctx)
```

**Endpoint:** `GET /api/programs`

**Response:** Array of `Program` objects

**Program Fields:**

- `ID` (string): Unique program identifier
- `Title` (string): Program title
- `Duration` (int64): Duration in milliseconds
- `Year` (int): Release year
- `Rating` (string): Content rating (e.g., "PG", "PG-13", "TV-Y")
- `Genres` ([]string): Array of genre tags
- `Type` (string): Content type - "movie", "episode", or "track"
- `ShowTitle` (string): For episodes: parent show name
- `Season` (int): For episodes: season number
- `Episode` (int): For episodes: episode number
- `Summary` (string): Program description/synopsis
- `OriginalAirDate` (string): Original broadcast date (ISO 8601 format)

### Libraries

#### Get All Libraries

Retrieves all media libraries configured in Tunarr.

```go
libraries, err := client.GetLibraries(ctx)
```

**Endpoint:** `GET /api/libraries`

**Response:** Array of `Library` objects

**Library Fields:**

- `ID` (string): Unique library identifier
- `Name` (string): Library display name
- `Type` (string): Library type (e.g., "plex", "jellyfin")

#### Get Library Programs

Retrieves all programs/media from a specific library.

```go
programs, err := client.GetLibraryPrograms(ctx, libraryID)
```

**Endpoint:** `GET /api/libraries/{libraryID}/programs`

**Parameters:**

- `libraryID` (string): ID of the library to query

**Response:** Array of `Program` objects

### Shows

#### Get All Shows

Retrieves all TV shows from Tunarr.

```go
shows, err := client.GetShows(ctx)
```

**Endpoint:** `GET /api/shows`

**Response:** Array of `Show` objects

**Show Fields:**

- `ID` (string): Unique show identifier
- `Title` (string): Show title
- `Summary` (string): Show description
- `Genres` ([]string): Array of genre tags

#### Get Show Episodes

Retrieves episodes for a specific show, optionally filtered by season.

```go
// Get all episodes
episodes, err := client.GetShowEpisodes(ctx, showID, 0)

// Get specific season
episodes, err := client.GetShowEpisodes(ctx, showID, 2)
```

**Endpoint:** `GET /api/shows/{showID}/episodes`

**Query Parameters:**

- `season` (int, optional): Filter by season number (0 = all seasons)

**Response:** Array of `Program` objects with `Type="episode"`

### Search

#### Search Programs

Searches for programs by title using a query string.

```go
results, err := client.SearchPrograms(ctx, "Star Trek")
```

**Endpoint:** `GET /api/programs/search`

**Query Parameters:**

- `q` (string, required): Search query

**Response:** Array of `Program` objects matching the query

### Filler Content

#### Get Filler Lists

Retrieves all available filler content lists (bumpers, commercials, etc.).

```go
fillerLists, err := client.GetFillerLists(ctx)
```

**Endpoint:** `GET /api/filler-lists`

**Response:** Array of `FillerList` objects

**FillerList Fields:**

- `ID` (string): Unique filler list identifier
- `Name` (string): Filler list display name
- `Count` (int): Number of items in the list

#### Get Filler Content

Retrieves programs from a specific filler list.

```go
programs, err := client.GetFillerContent(ctx, fillerListID)
```

**Endpoint:** `GET /api/filler-lists/{fillerListID}/content`

**Parameters:**

- `fillerListID` (string): ID of the filler list

**Response:** Array of `Program` objects

### Schedule Updates

#### Update Channel Schedule

Updates the programming schedule for a specific channel.

```go
schedule := []tunarr.Program{
    // Array of programs to schedule
}
err := client.UpdateSchedule(ctx, channelID, schedule)
```

**Endpoint:** `POST /api/channels/{channelID}/schedule`

**Parameters:**

- `channelID` (string): ID of the channel to update

**Request Body:** JSON array of `Program` objects

**Response:** Success/error status

## Error Handling

The client returns structured errors with context:

```go
programs, err := client.GetPrograms(ctx)
if err != nil {
    // Error contains details about the failure
    log.Error("Failed to fetch programs", "error", err)
    return err
}
```

### Error Types

Errors are wrapped with context using `fmt.Errorf`:

```go
fmt.Errorf("failed to fetch library %s: %w", libID, err)
```

### Retry Behavior

The client automatically retries failed requests with exponential backoff:

- **Initial Backoff:** 1 second
- **Max Retries:** 3 attempts
- **Backoff Multiplier:** 2x per retry

Retries occur for:

- Network errors
- Temporary server errors (5xx status codes)
- Timeout errors

## Metrics

All API calls are instrumented with Prometheus metrics:

### Counters

**schedularr_tunarr_api_calls_total**

- Total number of API calls
- Labels: `endpoint`, `method`

**schedularr_tunarr_api_errors_total**

- Total number of API errors
- Labels: `endpoint`, `method`, `error_type`

### Histograms

**schedularr_tunarr_api_call_duration_seconds**

- Duration of API calls in seconds
- Labels: `endpoint`, `method`
- Buckets: Default Prometheus buckets

## Usage Examples

### Fetching Content for Scheduling

```go
// Create client
client := tunarr.NewClient(cfg, logger)

// Fetch all programs
programs, err := client.GetPrograms(ctx)
if err != nil {
    return fmt.Errorf("failed to fetch programs: %w", err)
}

// Filter programs (using scheduler.FilterPrograms)
filtered := scheduler.FilterPrograms(programs, filter)

// Schedule the programs
schedule := buildSchedule(filtered)
err = client.UpdateSchedule(ctx, channelID, schedule)
```

### Working with Series

```go
// Get all shows
shows, err := client.GetShows(ctx)
if err != nil {
    return err
}

// Find specific show
var targetShow *tunarr.Show
for _, show := range shows {
    if show.Title == "Star Trek: The Next Generation" {
        targetShow = &show
        break
    }
}

// Get episodes for the show
episodes, err := client.GetShowEpisodes(ctx, targetShow.ID, 0)
if err != nil {
    return err
}

// Schedule next N episodes
nextEpisodes := episodes[:episodesPerBlock]
err = client.UpdateSchedule(ctx, channelID, nextEpisodes)
```

### Using Library Content

```go
// Get all libraries
libraries, err := client.GetLibraries(ctx)
if err != nil {
    return err
}

// Fetch content from each library
var allPrograms []tunarr.Program
for _, lib := range libraries {
    programs, err := client.GetLibraryPrograms(ctx, lib.ID)
    if err != nil {
        log.Warn("Failed to fetch library content",
            "library", lib.Name, "error", err)
        continue
    }
    allPrograms = append(allPrograms, programs...)
}
```

### Search and Filter

```go
// Search for programs
results, err := client.SearchPrograms(ctx, "action")
if err != nil {
    return err
}

// Apply additional filtering
actionMovies := scheduler.FilterPrograms(results, scheduler.Filter{
    Type:   []string{"movie"},
    Genres: []string{"Action"},
    YearFrom: 2010,
})
```

## Best Practices

### Context Management

Always pass a context with timeout for API calls:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

programs, err := client.GetPrograms(ctx)
```

### Error Handling

Check errors and provide context:

```go
programs, err := client.GetPrograms(ctx)
if err != nil {
    return fmt.Errorf("failed to fetch programs for scheduling: %w", err)
}
```

### Content Caching

The client does not cache responses. Implement caching at the application level if needed:

```go
// Cache programs for reuse
var programCache []tunarr.Program
var cacheTime time.Time
const cacheDuration = 5 * time.Minute

func getPrograms(ctx context.Context) ([]tunarr.Program, error) {
    if time.Since(cacheTime) < cacheDuration && len(programCache) > 0 {
        return programCache, nil
    }

    programs, err := client.GetPrograms(ctx)
    if err != nil {
        return nil, err
    }

    programCache = programs
    cacheTime = time.Now()
    return programs, nil
}
```

### Rate Limiting

The client includes retry logic but not rate limiting. Implement rate limiting at the application level if needed:

```go
import "golang.org/x/time/rate"

limiter := rate.NewLimiter(rate.Every(time.Second), 10) // 10 req/sec

func rateLimitedFetch(ctx context.Context) error {
    if err := limiter.Wait(ctx); err != nil {
        return err
    }
    return client.GetPrograms(ctx)
}
```

## Testing

The `internal/tunarr` package includes comprehensive tests:

```bash
# Run tunarr client tests
go test ./internal/tunarr/...

# Run with coverage
go test -cover ./internal/tunarr/...
```

### Mocking the Client

For testing code that uses the Tunarr client:

```go
type MockTunarrClient struct {
    Programs []tunarr.Program
    Err      error
}

func (m *MockTunarrClient) GetPrograms(ctx context.Context) ([]tunarr.Program, error) {
    if m.Err != nil {
        return nil, m.Err
    }
    return m.Programs, nil
}

// Use in tests
func TestScheduling(t *testing.T) {
    mockClient := &MockTunarrClient{
        Programs: []tunarr.Program{
            {ID: "1", Title: "Test Show", Duration: 1800000},
        },
    }

    // Test your code with mockClient
}
```

## Troubleshooting

### Connection Issues

```
Error: failed to fetch programs: Get "http://localhost:8000/api/programs": dial tcp [::1]:8000: connect: connection refused
```

**Solution:** Verify Tunarr is running and accessible at the configured URL.

### Authentication Issues

```
Error: API returned status 401: Unauthorized
```

**Solution:** Check that your API key is correct in the configuration.

### Empty Results

```
Got 0 programs from Tunarr
```

**Solutions:**

1. Verify Tunarr has media sources configured
2. Check that media libraries are scanned and have content
3. Verify the endpoint is correct for your Tunarr version

### Timeout Issues

```
Error: context deadline exceeded
```

**Solutions:**

1. Increase the timeout in the client configuration
2. Check network latency to Tunarr server
3. Verify Tunarr is not overloaded

## Version Compatibility

This documentation is current as of:

- **Schedularr:** v0.1.0+
- **Tunarr:** v0.x (API endpoints may vary)

**Note:** Tunarr API endpoints and response formats may change between versions. Always verify compatibility with your Tunarr installation.

## Additional Resources

- [Tunarr Documentation](https://github.com/chrisbenincasa/tunarr)
- [Tunarr API Source Code](https://github.com/chrisbenincasa/tunarr/tree/main/server/src/api)
- [Schedularr Architecture](./ARCHITECTURE.md)
- [Schedularr Specifications](./SPECIFICATIONS.md)
