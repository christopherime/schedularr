# Schedularr

Schedularr is a robust Go application designed to interface with the [Tunarr API](https://tunarr.com/api-docs.html#latest) to automate content scheduling for your TV channels. It allows for complex, rule-based programming schedules, including recurring blocks, content filtering, and automated updates.

## Features

- **Tunarr Integration**: Seamlessly communicates with your Tunarr instance to fetch channels and update programming.
- **Advanced Filtering**: Schedule content based on exact or regex title matching, genres, ratings, release years, and duration constraints.
- **Recurring Scheduling**: Create daily, weekly, or monthly programming blocks using standard Cron syntax.
- **CLI Interface**: Easy-to-use command-line interface for managing and generating schedules.
- **Dry Run Support**: Generate and preview schedules locally before applying them to your channels.

## Installation

### Prerequisites

- Go 1.25.5 or higher
- A running instance of Tunarr

### Build from Source

1. Clone the repository:
   ```bash
   git clone https://github.com/geekxflood/schedularr.git
   cd schedularr
   ```

2. Build the application:
   ```bash
   go build -o schedularr cmd/schedularr/main.go
   ```

## Configuration

Schedularr uses a YAML configuration file (default: `$HOME/.schedularr.yaml` or `./.schedularr.yaml`).

Create a configuration file based on the example:

```bash
cp config.yaml.example .schedularr.yaml
```

### Configuration Structure

```yaml
tunarr:
  url: "http://localhost:8000"  # Your Tunarr API URL
  api_key: "your-api-key"       # API Key if authentication is enabled

log:
  level: "info"                 # debug, info, warn, error
  format: "text"                # text or json

scheduler:
  blocks:
    - name: "Morning Cartoons"
      cron: "0 6 * * *"         # Runs daily at 6:00 AM
      duration: 240             # Duration in minutes (4 hours)
      channel_id: "channel-1"   # Target Tunarr Channel ID
      filter:
        genres: ["Animation", "Family"]
        max_duration: 30        # Max duration per item in minutes
    
    - name: "Evening Movies"
      cron: "0 20 * * *"        # Runs daily at 8:00 PM
      duration: 180             # 3 hours
      channel_id: "channel-1"
      filter:
        genres: ["Movie"]
        min_duration: 80        # Minimum duration in minutes
```

## Usage

### List Channels

View all available channels in your Tunarr instance:

```bash
./schedularr channels
```

### Generate Schedule

Generate a schedule for the next 24 hours based on your configuration rules. By default, this runs in "dry-run" mode and only prints the plan to the console.

```bash
./schedularr generate
```

### Apply Schedule

To push the generated schedule to Tunarr:

```bash
./schedularr generate --apply
```

## Development

Run tests:

```bash
go test ./...
```
