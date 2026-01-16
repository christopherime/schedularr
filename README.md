<div align="center">
<img src="assets/logo.svg" alt="Schedularr Logo" width="150"/>

# Schedularr

## Intelligent Content Scheduling for Tunarr

[![Go Version](https://img.shields.io/badge/Go-1.25.5+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/geekxflood/schedularr/pulls)

**Automate your TV channel programming with powerful rule-based scheduling, advanced content filtering, and seamless Tunarr integration.**

[Features](#-features) • [Quick Start](#-quick-start) • [Configuration](#️-configuration) • [Examples](#-examples) • [Contributing](#-contributing)

</div>

---

## 🎯 Overview

Schedularr is a sophisticated Go application that transforms how you manage content scheduling for [Tunarr](https://tunarr.com). Say goodbye to manual programming and hello to intelligent, automated channel management with cron-based recurring blocks, multi-criteria content filtering, and a beautiful terminal UI.

### Why Schedularr?

- 🤖 **Set It and Forget It**: Define your programming rules once, let Schedularr handle the rest
- 🎨 **Smart Filtering**: Match content by title patterns, genres, ratings, release years, and duration
- ⏰ **Flexible Scheduling**: Use familiar cron syntax for daily, weekly, or custom recurring blocks
- 🖥️ **Interactive TUI**: Manage your schedules with an intuitive terminal interface
- 🔍 **Dry Run Mode**: Preview schedules before applying them to your channels
- 🚀 **Lightweight & Fast**: Built in Go for performance and reliability

---

## ✨ Features

### Core Capabilities

| Feature                      | Description                                                                   |
| ---------------------------- | ----------------------------------------------------------------------------- |
| **🔌 Tunarr Integration**   | Seamless API communication with your Tunarr instance                          |
| **🎯 Advanced Filtering**   | Regex title matching, genre/rating filters, year ranges, duration constraints |
| **📅 Cron Scheduling**      | Standard cron expressions for flexible recurring programming                  |
| **🎨 Terminal UI**          | Beautiful interactive interface built with Bubble Tea                         |
| **⚡ CLI Commands**          | Powerful command-line tools for automation and scripting                      |
| **🔍 Dry Run Mode**         | Test and preview schedules before applying changes                            |
| **📊 Priority System**      | Handle overlapping blocks with configurable priorities                        |
| **🏷️ Tag Support**        | Organize and filter content using custom tags                                 |
| **🎞️ Radarr/Sonarr Sync** | Filter schedules using Radarr/Sonarr availability                             |
| **📺 Jellyfin Refresh**     | Optional Live TV guide refresh after schedule updates                         |

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.25.5+** - [Download](https://go.dev/dl/)
- **Tunarr Instance** - [Setup Guide](https://tunarr.com/api-docs.html#latest)

### Installation

#### Option 1: Build from Source

```bash
# Clone the repository
git clone https://github.com/geekxflood/schedularr.git
cd schedularr

# Build the binary
go build -o schedularr main.go

# Optional: Install to your PATH
sudo mv schedularr /usr/local/bin/
```

#### Option 2: Using Go Install

```bash
go install github.com/geekxflood/schedularr@latest
```

### Initial Setup

1. **Generate configuration files:**

```bash
# Generate application config
schedularr config generate config.yaml

# Generate scheduler config
schedularr scheduler init scheduler.yaml
```

1. **Configure Tunarr connection:**

Edit `config.yaml` and set your Tunarr instance details:

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: "your-api-key-here"  # Optional, if authentication is enabled
log:
  level: "info"
  format: "text"
```

1. **Validate your configuration:**

```bash
# Validate both configs
schedularr validate config.yaml
schedularr validate scheduler.yaml
```

1. **Verify Tunarr connection:**

```bash
schedularr --config config.yaml channels
```

---

## ⚙️ Configuration

Schedularr uses a YAML configuration file located at:

- `~/.schedularr.yaml` (user-level)
- `./.schedularr.yaml` (project-level)

### Configuration Reference

#### Tunarr Connection

```yaml
tunarr:
  url: "http://localhost:8000"  # Tunarr API endpoint
  api_key: ""                   # Optional API key for authentication
```

#### Radarr Connection (Optional)

```yaml
radarr:
  url: "http://localhost:7878"  # Radarr API endpoint
  api_key: ""                   # Optional API key for authentication
```

#### Sonarr Connection (Optional)

```yaml
sonarr:
  url: "http://localhost:8989"  # Sonarr API endpoint
  api_key: ""                   # Optional API key for authentication
```

#### Jellyfin Connection (Optional)

```yaml
jellyfin:
  url: "http://localhost:8096"  # Jellyfin API endpoint
  api_key: ""                   # Optional API key for authentication
  sync_live_tv: false           # Refresh Live TV guide after schedule apply
```

Radarr and Sonarr are used to filter Tunarr programs by availability; scheduling still applies to Tunarr channel IDs.

#### Logging

```yaml
log:
  level: "info"    # Options: debug, info, warn, error
  format: "text"   # Options: text, json
```

#### Scheduling Blocks

```yaml
scheduler:
  blocks:
    - name: "Morning Cartoons"
      cron: "0 6 * * *"           # Daily at 6:00 AM
      duration: 240               # 4 hours (in minutes)
      channel_id: "channel-1"     # Target channel
      priority: 10                # Higher = more important
      filter:
        genres: ["Animation", "Family"]
        max_duration: 30          # Max 30 min per show
        ratings: ["TV-Y", "TV-G"]
        year_from: 2000

    - name: "Prime Time Movies"
      cron: "0 20 * * *"          # Daily at 8:00 PM
      duration: 180               # 3 hours
      channel_id: "channel-1"
      priority: 20
      filter:
        genres: ["Action", "Drama"]
        min_duration: 90          # Feature-length films
        year_from: 2010
        ratings: ["PG-13", "R"]
```

### Filter Options

| Field           | Type     | Description                      | Example                   |
| --------------- | -------- | -------------------------------- | ------------------------- |
| `title_pattern` | string   | Regex pattern for title matching | `"^Star.*"`               |
| `genres`        | []string | List of genres to include        | `["Comedy", "Drama"]`     |
| `ratings`       | []string | Content ratings filter           | `["PG", "PG-13"]`         |
| `year_from`     | int      | Minimum release year             | `2000`                    |
| `year_to`       | int      | Maximum release year             | `2023`                    |
| `min_duration`  | int      | Minimum duration (minutes)       | `90`                      |
| `max_duration`  | int      | Maximum duration (minutes)       | `120`                     |
| `tags`          | []string | Custom tags filter               | `["favorite", "classic"]` |

---

## 📖 Usage

### Command Overview

```bash
schedularr [command] [flags]
```

**📚 For complete CLI documentation, see [CLI Reference](docs/CLI_REFERENCE.md)**

### Available Commands

#### ⚙️ Configuration Management

Generate and validate configuration files:

```bash
# Generate application config
schedularr config generate [filename]

# Generate scheduler config
schedularr scheduler init [filename]

# Validate any config file
schedularr validate <file>

# List scheduler blocks
schedularr scheduler list [filename]
```

#### 📋 List Channels

View all available channels from your Tunarr instance:

```bash
schedularr channels
```

**Output:**

```txt
ID              Number  Name                    Enabled
channel-1       1       Classic Movies          true
channel-2       2       Kids Programming        true
channel-3       3       Sports & News           false
```

#### 🎬 Generate Schedule

Generate a schedule for the next 24 hours based on your configuration:

```bash
# Dry run (preview only)
schedularr generate

# Apply to Tunarr
schedularr generate --apply
```

**Example Output:**

```txt
Channel channel-1: 12 items scheduled
 - The Incredibles (115 min)
 - Finding Nemo (100 min)
 - Toy Story (81 min)
 ...
```

#### 🖥️ Interactive TUI

Launch the beautiful terminal user interface:

```bash
schedularr tui
```

**Features:**

- ✏️ Create and edit scheduling blocks
- 📊 View current configuration
- 💾 Save changes directly to config file
- 🎨 Syntax highlighting and validation

#### 🔧 Configuration Management

```bash
# Validate configuration
schedularr validate

# Show current configuration
schedularr config show

# Edit configuration in $EDITOR
schedularr config edit
```

---

## 💡 Examples

### Example 1: Weekend Movie Marathon

```yaml
scheduler:
  blocks:
    - name: "Saturday Night Sci-Fi"
      cron: "0 20 * * 6"  # Saturdays at 8 PM
      duration: 360       # 6 hours
      channel_id: "channel-1"
      filter:
        genres: ["Science Fiction"]
        min_duration: 90
        year_from: 1980
```

### Example 2: Weekday Morning Kids Block

```yaml
scheduler:
  blocks:
    - name: "Weekday Morning Cartoons"
      cron: "0 7 * * 1-5"  # Monday-Friday at 7 AM
      duration: 120
      channel_id: "channel-2"
      filter:
        genres: ["Animation"]
        ratings: ["TV-Y", "TV-Y7"]
        max_duration: 30
```

### Example 3: Classic Film Noir Night

```yaml
scheduler:
  blocks:
    - name: "Film Noir Fridays"
      cron: "0 22 * * 5"  # Fridays at 10 PM
      duration: 240
      channel_id: "channel-1"
      filter:
        title_pattern: ".*Noir.*|.*Detective.*"
        year_from: 1940
        year_to: 1959
        genres: ["Crime", "Mystery"]
```

### Example 4: Holiday Special Programming

```yaml
scheduler:
  blocks:
    - name: "Christmas Movies"
      cron: "0 18 1-25 12 *"  # Dec 1-25 at 6 PM
      duration: 180
      channel_id: "channel-1"
      priority: 100  # Override other blocks
      filter:
        title_pattern: ".*Christmas.*|.*Holiday.*"
        genres: ["Family", "Comedy"]
```

---

## 🏗️ Architecture

### Project Structure

```txt
schedularr/
├── cmd/
│   └── schedularr/          # Application entry point
│       └── main.go
├── internal/
│   ├── cli/                 # CLI commands (Cobra)
│   │   ├── root.go
│   │   ├── channels.go
│   │   ├── generate.go
│   │   └── tui.go
│   ├── config/              # Configuration management (Viper)
│   │   └── config.go
│   ├── scheduler/           # Core scheduling engine
│   │   ├── engine.go
│   │   ├── filter.go
│   │   └── types.go
│   ├── tunarr/              # Tunarr API client
│   │   ├── client.go
│   │   └── types.go
│   └── tui/                 # Terminal UI (Bubble Tea)
│       └── model.go
├── configs/
│   └── config.yaml          # Example configuration
└── docs/
    ├── ARCHITECTURE.md
    └── SPECIFICATIONS.md
```

### Key Technologies

- **Language**: Go 1.25.5
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra)
- **Configuration**: [Viper](https://github.com/spf13/viper)
- **Cron Parsing**: [robfig/cron](https://github.com/robfig/cron)
- **Terminal UI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea)

---

## 🧪 Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/scheduler/...
```

### Building

```bash
# Development build
go build -o schedularr cmd/schedularr/main.go

# Production build with optimizations
go build -ldflags="-s -w" -o schedularr cmd/schedularr/main.go

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o schedularr-linux cmd/schedularr/main.go
```

### Code Quality

```bash
# Format code
go fmt ./...

# Lint
golangci-lint run

# Vet
go vet ./...
```

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Workflow

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Guidelines

- Write tests for new features
- Follow Go best practices and idioms
- Update documentation as needed
- Ensure all tests pass before submitting PR

---

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- [Tunarr](https://tunarr.com) - The amazing TV channel management platform
- [Cobra](https://github.com/spf13/cobra) - Powerful CLI framework
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Delightful TUI framework

---

## 📞 Support

- 🐛 **Issues**: [GitHub Issues](https://github.com/geekxflood/schedularr/issues)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/geekxflood/schedularr/discussions)
- 📧 **Email**: [Contact](mailto:christopherime@me.com)

---

<div align="center">

**Made with ❤️ by [Geekxflood](https://github.com/geekxflood)**

⭐ Star this repo if you find it useful!

</div>
