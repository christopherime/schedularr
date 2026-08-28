package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/christopherime/schedularr/internal/external/tunarr"
)

// ConnectionStatus represents the connection state to Tunarr.
type ConnectionStatus int

const (
	// StatusDisconnected means no connection to Tunarr.
	StatusDisconnected ConnectionStatus = iota
	// StatusConnecting means actively connecting.
	StatusConnecting
	// StatusConnected means successfully connected.
	StatusConnected
	// StatusError means connection failed with error.
	StatusError
)

// String returns a human-readable status string.
func (s ConnectionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "Disconnected"
	case StatusConnecting:
		return "Connecting..."
	case StatusConnected:
		return "Connected"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ProgramSlot represents a scheduled program slot.
type ProgramSlot struct {
	Title     string
	ShowTitle string // For episodes
	Type      string // movie, episode, flex, redirect, custom
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	BlockName string // Name of the block that generated this (for planned programs)

	// Episode info
	SeasonNumber  int
	EpisodeNumber int

	// Additional metadata
	Year   int
	Rating string
}

// FormatTitle returns a display-friendly title.
func (p ProgramSlot) FormatTitle() string {
	if p.Type == "episode" && p.ShowTitle != "" {
		return fmt.Sprintf("%s S%02dE%02d", p.ShowTitle, p.SeasonNumber, p.EpisodeNumber)
	}
	return p.Title
}

// RemainingTime returns the remaining time for a currently playing program.
func (p ProgramSlot) RemainingTime(now time.Time) time.Duration {
	if now.After(p.EndTime) {
		return 0
	}
	return p.EndTime.Sub(now)
}

// FormatRemainingTime returns a formatted string of remaining time.
func (p ProgramSlot) FormatRemainingTime(now time.Time) string {
	remaining := p.RemainingTime(now)
	if remaining <= 0 {
		return "ended"
	}
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm remaining", hours, minutes)
	}
	return fmt.Sprintf("%dm remaining", minutes)
}

// ChannelView represents a channel with its current and planned programming.
type ChannelView struct {
	Channel         tunarr.Channel
	CurrentPrograms []ProgramSlot // Programs from Tunarr (current schedule)
	PlannedPrograms []ProgramSlot // Programs from scheduler (pending changes)
	PendingChanges  int           // Number of pending changes
	Error           error         // Any error fetching this channel's data
	LastUpdated     time.Time     // When this data was last fetched
}

// GetNowPlaying returns the currently playing program, if any.
func (cv *ChannelView) GetNowPlaying(now time.Time) *ProgramSlot {
	for i := range cv.CurrentPrograms {
		p := &cv.CurrentPrograms[i]
		if now.After(p.StartTime) && now.Before(p.EndTime) {
			return p
		}
	}
	return nil
}

// GetUpNext returns the next program after the current one.
func (cv *ChannelView) GetUpNext(now time.Time) *ProgramSlot {
	for i := range cv.CurrentPrograms {
		p := &cv.CurrentPrograms[i]
		if p.StartTime.After(now) {
			return p
		}
	}
	return nil
}

// HasChanges returns true if there are pending changes for this channel.
func (cv *ChannelView) HasChanges() bool {
	return cv.PendingChanges > 0
}

// convertTunarrProgramming converts Tunarr channel programs to ProgramSlots.
func convertTunarrProgramming(programs []tunarr.ChannelProgram) []ProgramSlot {
	slots := make([]ProgramSlot, 0, len(programs))
	for _, p := range programs {
		startTime := time.UnixMilli(p.StartTimeMs)
		duration := time.Duration(p.Duration) * time.Millisecond
		endTime := startTime.Add(duration)

		slot := ProgramSlot{
			Title:         p.Title,
			ShowTitle:     p.ShowTitle,
			Type:          p.Type,
			StartTime:     startTime,
			EndTime:       endTime,
			Duration:      duration,
			SeasonNumber:  p.SeasonNumber,
			EpisodeNumber: p.EpisodeNumber,
			Rating:        p.Rating,
		}
		if p.Year != nil {
			slot.Year = *p.Year
		}
		slots = append(slots, slot)
	}
	return slots
}

// ============================================================================
// Channel View Rendering
// ============================================================================

// renderChannelView renders the main channel list view.
func (m Model) renderChannelView() string {
	var builder strings.Builder

	// Header with status
	builder.WriteString(m.renderChannelHeader())
	builder.WriteString("\n")

	// Main content area
	if len(m.channels) == 0 {
		builder.WriteString(m.renderNoChannels())
	} else {
		builder.WriteString(m.renderChannelList())
	}

	// Footer with help
	builder.WriteString("\n")
	builder.WriteString(m.renderChannelFooter())

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

// renderChannelHeader renders the header with title and status.
func (m Model) renderChannelHeader() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	var statusStyle lipgloss.Style
	switch m.connectionStatus {
	case StatusConnected:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("34")) // Green
	case StatusConnecting:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // Yellow
	case StatusError, StatusDisconnected:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Red
	}

	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	title := titleStyle.Render("Schedularr - Channels")
	status := statusStyle.Render("[" + m.connectionStatus.String() + "]")
	timeStr := timeStyle.Render(time.Now().Format("15:04:05"))

	// Calculate padding for right-aligned status
	headerLine := fmt.Sprintf("%s %s %s", title, status, timeStr)

	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("═", 72))

	return headerLine + "\n" + divider
}

// renderNoChannels renders the empty state when no channels are available.
func (m Model) renderNoChannels() string {
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var builder strings.Builder
	builder.WriteString("\n")

	if m.connectionStatus == StatusError && m.connectionError != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		builder.WriteString(errorStyle.Render("Connection Error: "+m.connectionError.Error()) + "\n\n")
	}

	builder.WriteString(normalStyle.Render("No channels loaded.\n\n"))
	builder.WriteString(helpStyle.Render("Press 'R' to refresh or check your Tunarr configuration.\n"))
	builder.WriteString(helpStyle.Render("Press 'B' to switch to block editor mode.\n"))

	return builder.String()
}

// renderChannelList renders the list of channels with their current programming.
func (m Model) renderChannelList() string {
	var builder strings.Builder

	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	highlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	changesStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	noChangesStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))

	builder.WriteString("\n")

	// Calculate scroll window
	start, end := calculateScrollWindow(m.channelSelected, len(m.channels), 3, 8)

	for i := start; i < end; i++ {
		cv := m.channels[i]
		isSelected := i == m.channelSelected

		// Channel header line
		prefix := "  "
		style := normalStyle
		if isSelected {
			prefix = "▸ "
			style = highlightStyle
		}

		// Channel name with changes indicator
		chanName := "📺 " + cv.Channel.Name
		if len(chanName) > 40 {
			chanName = chanName[:37] + "..."
		}

		changesIndicator := ""
		if cv.HasChanges() {
			changesIndicator = changesStyle.Render(fmt.Sprintf("[%d pending changes]", cv.PendingChanges))
		} else {
			changesIndicator = noChangesStyle.Render("[No changes]")
		}

		builder.WriteString(style.Render(prefix+chanName) + "  " + changesIndicator + "\n")

		// Now playing / Up next info
		now := time.Now()
		nowPlaying := cv.GetNowPlaying(now)
		upNext := cv.GetUpNext(now)

		if nowPlaying != nil {
			nowStr := fmt.Sprintf("     Now: \"%s\" (%s)",
				truncate(nowPlaying.FormatTitle(), 35),
				nowPlaying.FormatRemainingTime(now))
			builder.WriteString(dimStyle.Render(nowStr) + "\n")
		} else {
			builder.WriteString(dimStyle.Render("     Now: (no program)") + "\n")
		}

		if upNext != nil {
			nextStr := fmt.Sprintf("     Next: \"%s\" @ %s",
				truncate(upNext.FormatTitle(), 35),
				upNext.StartTime.Format("15:04"))
			builder.WriteString(dimStyle.Render(nextStr) + "\n")
		}

		// Error indicator
		if cv.Error != nil {
			errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
			builder.WriteString(errorStyle.Render("     ⚠ Error: "+cv.Error.Error()) + "\n")
		}

		builder.WriteString("\n")
	}

	// Show count
	builder.WriteString(dimStyle.Render(fmt.Sprintf("Showing %d of %d channels", end-start, len(m.channels))))

	return builder.String()
}

// renderChannelFooter renders the footer with key bindings.
func (m Model) renderChannelFooter() string {
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	divider := helpStyle.Render(strings.Repeat("─", 72))

	// Show status message if any
	var statusLine string
	if m.statusMessage != "" {
		statusLine = statusStyle.Render(m.statusMessage) + "\n"
	}

	autoRefreshIndicator := ""
	if m.autoRefresh {
		autoRefreshIndicator = " [Auto-Refresh ON]"
	}

	generatingIndicator := ""
	if m.isGenerating {
		generatingIndicator = " [Generating...]"
	}

	helpText := "[P]review | [A]pply | [G]enerate | [B]locks | [R]efresh | [T]oggle Auto | [?]Help | [Q]uit"

	return statusLine + divider + "\n" + helpStyle.Render(helpText+autoRefreshIndicator+generatingIndicator)
}

// ============================================================================
// Preview View Rendering
// ============================================================================

// renderPreviewView renders the schedule preview/diff view.
func (m Model) renderPreviewView() string {
	var builder strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	addedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))   // Green
	removedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Red
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	builder.WriteString(titleStyle.Render("Schedule Preview - Changes to Apply") + "\n\n")

	if len(m.channels) == 0 {
		builder.WriteString(normalStyle.Render("No channels loaded.\n"))
		builder.WriteString(helpStyle.Render("\nPress 'esc' to return."))
		return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
	}

	totalChanges := 0
	for _, cv := range m.channels {
		if !cv.HasChanges() {
			continue
		}

		totalChanges += cv.PendingChanges
		builder.WriteString(titleStyle.Render("📺 "+cv.Channel.Name) + "\n")
		builder.WriteString(strings.Repeat("─", 50) + "\n")

		// Show current vs planned programs
		if len(cv.PlannedPrograms) > 0 {
			builder.WriteString(addedStyle.Render("+ Programs to add:") + "\n")
			for i, p := range cv.PlannedPrograms {
				if i >= 5 { // Limit display
					builder.WriteString(addedStyle.Render(fmt.Sprintf("  ... and %d more\n", len(cv.PlannedPrograms)-5)))
					break
				}
				builder.WriteString(addedStyle.Render(fmt.Sprintf("  + %s @ %s\n",
					p.FormatTitle(), p.StartTime.Format("15:04"))))
			}
		}

		// Note about replacement
		if len(cv.CurrentPrograms) > 0 {
			builder.WriteString(removedStyle.Render(fmt.Sprintf("- Replacing %d existing programs\n", len(cv.CurrentPrograms))))
		}

		builder.WriteString("\n")
	}

	if totalChanges == 0 {
		builder.WriteString(normalStyle.Render("No changes to preview.\n"))
		builder.WriteString(normalStyle.Render("Press 'G' to generate a new schedule first.\n"))
	} else {
		builder.WriteString(normalStyle.Render(fmt.Sprintf("\nTotal: %d changes across %d channels\n", totalChanges, countChannelsWithChanges(m.channels))))
	}

	builder.WriteString("\n")
	builder.WriteString(helpStyle.Render("[A]pply changes | [esc] Cancel | [?] Help"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

// countChannelsWithChanges counts channels that have pending changes.
func countChannelsWithChanges(channels []ChannelView) int {
	count := 0
	for _, cv := range channels {
		if cv.HasChanges() {
			count++
		}
	}
	return count
}

// ============================================================================
// Apply Progress View
// ============================================================================

// renderApplyingView renders the apply-in-progress view.
func (m Model) renderApplyingView() string {
	var builder strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	builder.WriteString(titleStyle.Render("Applying Schedule Changes") + "\n\n")

	for _, result := range m.applyResults {
		if result.Success {
			builder.WriteString(successStyle.Render(fmt.Sprintf("✓ %s: Applied successfully\n", result.ChannelName)))
		} else if result.Error != nil {
			builder.WriteString(errorStyle.Render(fmt.Sprintf("✗ %s: %v\n", result.ChannelName, result.Error)))
		} else {
			builder.WriteString(progressStyle.Render(fmt.Sprintf("⋯ %s: In progress...\n", result.ChannelName)))
		}
	}

	if m.applyComplete {
		builder.WriteString("\n")
		builder.WriteString(normalStyle.Render("Apply complete. Press any key to continue."))
	}

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

// ApplyResult tracks the result of applying changes to a channel.
type ApplyResult struct {
	ChannelID   string
	ChannelName string
	Success     bool
	Error       error
}
