// Package tui provides a terminal user interface for managing Schedularr configuration.
package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/store"
	"github.com/robfig/cron/v3"
)

type sessionState int

const (
	stateListView sessionState = iota
	stateEditBlock
	stateConfirmDelete
	stateHelp
	stateSeriesProgress
	stateCronBuilder
)

type item struct {
	block scheduler.Block
}

func (i item) Title() string { return i.block.Name }
func (i item) Description() string {
	return fmt.Sprintf("%s | %d min | %s", i.block.Cron, i.block.Duration, i.block.ChannelID)
}
func (i item) FilterValue() string { return i.block.Name }

// Model is the Bubble Tea model for the TUI.
type Model struct {
	cfg            *config.Config
	store          *store.Store
	list           list.Model
	inputs         []textinput.Model
	validationErrs []string // validation errors for each input field
	focusIndex     int
	state          sessionState
	prevState      sessionState // previous state before help
	selected       int          // index of block being edited
	seriesStates   []scheduler.SeriesState
	seriesScroll   int
	cronMinute     string
	cronHour       string
	cronDayOfMonth string
	cronMonth      string
	cronDayOfWeek  string
	cronFieldIndex int
}

// NewModel creates a new TUI model with the given configuration and optional store.
func NewModel(cfg *config.Config, st *store.Store) Model {
	items := make([]list.Item, len(cfg.Scheduler.Blocks))
	for i, b := range cfg.Scheduler.Blocks {
		items[i] = item{block: b}
	}

	// Setup list
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Scheduling Blocks"

	// Setup inputs for editing
	inputs := make([]textinput.Model, 4)
	var t textinput.Model
	for i := range inputs {
		t = textinput.New()
		t.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
		t.CharLimit = 100

		switch i {
		case 0:
			t.Placeholder = "Block Name"
			t.Focus()
			t.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
			t.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
		case 1:
			t.Placeholder = "Cron Expression (e.g. 0 20 * * *)"
		case 2:
			t.Placeholder = "Duration (minutes)"
		case 3:
			t.Placeholder = "Channel ID"
		}
		inputs[i] = t
	}

	return Model{
		cfg:            cfg,
		store:          st,
		list:           l,
		inputs:         inputs,
		validationErrs: make([]string, 4),
		state:          stateListView,
	}
}

// Init initializes the TUI model.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles Bubble Tea messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.updateWindowSize(msg)
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	return m, nil
}

func (m *Model) updateWindowSize(msg tea.WindowSizeMsg) {
	h, v := lipgloss.NewStyle().Margin(1, 2).GetFrameSize()
	m.list.SetSize(msg.Width-h, msg.Height-v)
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global help key
	if msg.String() == "?" && m.state != stateHelp {
		m.prevState = m.state
		m.state = stateHelp
		return m, nil
	}

	switch m.state {
	case stateListView:
		return m.updateListView(msg)
	case stateEditBlock:
		return m.updateEditBlock(msg)
	case stateConfirmDelete:
		return m.updateConfirmDelete(msg)
	case stateHelp:
		return m.updateHelp(msg)
	case stateSeriesProgress:
		return m.updateSeriesProgress(msg)
	case stateCronBuilder:
		return m.updateCronBuilder(msg)
	default:
		return m, nil
	}
}

func (m Model) updateListView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter":
		if len(m.cfg.Scheduler.Blocks) == 0 {
			return m, nil
		}
		m.startEditBlock(m.list.Index())
		return m, nil
	case "n":
		m.startEditBlock(-1)
		return m, nil
	case "d", "delete":
		if len(m.cfg.Scheduler.Blocks) == 0 {
			return m, nil
		}
		m.selected = m.list.Index()
		m.state = stateConfirmDelete
		return m, nil
	case "s":
		if m.store != nil {
			m.loadSeriesStates()
			m.state = stateSeriesProgress
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateEditBlock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateListView
		return m, nil
	case "ctrl+b":
		// Open cron builder when focused on cron field (index 1)
		if m.focusIndex == 1 {
			m.initCronBuilder()
			m.state = stateCronBuilder
		}
		return m, nil
	case "tab", "shift+tab", "enter", "up", "down":
		return m.handleEditNavigation(msg.String())
	default:
		return m.updateFocusedInput(msg)
	}
}

func (m Model) handleEditNavigation(key string) (tea.Model, tea.Cmd) {
	if key == "enter" && m.focusIndex == len(m.inputs) {
		m.saveBlock()
		m.state = stateListView
		return m, nil
	}

	delta := 1
	if key == "up" || key == "shift+tab" {
		delta = -1
	}

	m.moveFocus(delta)
	cmd := m.updateInputFocusStyles()
	return m, cmd
}

func (m *Model) startEditBlock(index int) {
	m.state = stateEditBlock
	m.selected = index

	if index >= 0 && index < len(m.cfg.Scheduler.Blocks) {
		b := m.cfg.Scheduler.Blocks[index]
		m.inputs[0].SetValue(b.Name)
		m.inputs[1].SetValue(b.Cron)
		m.inputs[2].SetValue(strconv.Itoa(b.Duration))
		m.inputs[3].SetValue(b.ChannelID)
	} else {
		for i := range m.inputs {
			m.inputs[i].SetValue("")
		}
	}

	m.resetInputs()
}

func (m *Model) moveFocus(delta int) {
	m.focusIndex += delta
	if m.focusIndex > len(m.inputs) {
		m.focusIndex = 0
	} else if m.focusIndex < 0 {
		m.focusIndex = len(m.inputs)
	}
}

func (m *Model) updateInputFocusStyles() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		if i == m.focusIndex {
			cmds[i] = m.inputs[i].Focus()
			m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
			m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
			continue
		}
		m.inputs[i].Blur()
		m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	}
	return tea.Batch(cmds...)
}

func (m Model) updateFocusedInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focusIndex < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
		// Perform real-time validation on the focused field
		m.validateField(m.focusIndex)
		return m, cmd
	}
	return m, nil
}

func (m *Model) validateField(index int) {
	if index >= len(m.inputs) {
		return
	}

	switch index {
	case 0:
		m.validateName()
	case 1:
		m.validateCron()
	case 2:
		m.validateDuration()
	case 3:
		m.validateChannelID()
	}
}

func (m *Model) validateName() {
	name := strings.TrimSpace(m.inputs[0].Value())
	if name == "" {
		m.validationErrs[0] = "Name is required"
	} else {
		m.validationErrs[0] = ""
	}
}

func (m *Model) validateCron() {
	cronExp := strings.TrimSpace(m.inputs[1].Value())
	if cronExp == "" {
		m.validationErrs[1] = "Cron expression is required"
		return
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(cronExp); err != nil {
		m.validationErrs[1] = fmt.Sprintf("Invalid cron: %v", err)
	} else {
		m.validationErrs[1] = ""
	}
}

func (m *Model) validateDuration() {
	durationStr := strings.TrimSpace(m.inputs[2].Value())
	if durationStr == "" {
		m.validationErrs[2] = "Duration is required"
		return
	}

	duration, err := strconv.Atoi(durationStr)
	if err != nil || duration <= 0 {
		m.validationErrs[2] = "Duration must be a positive number"
	} else {
		m.validationErrs[2] = ""
	}
}

func (m *Model) validateChannelID() {
	channelID := strings.TrimSpace(m.inputs[3].Value())
	if channelID == "" {
		m.validationErrs[3] = "Channel ID is required"
	} else {
		m.validationErrs[3] = ""
	}
}

func (m *Model) resetInputs() {
	m.focusIndex = 0
	for i := range m.inputs {
		if i == 0 {
			m.inputs[i].Focus()
			m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
			m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
		} else {
			m.inputs[i].Blur()
			m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		}
	}
}

func (m *Model) validateInputs() bool {
	valid := true

	// Validate name (required, non-empty)
	name := strings.TrimSpace(m.inputs[0].Value())
	if name == "" {
		m.validationErrs[0] = "Name is required"
		valid = false
	} else {
		m.validationErrs[0] = ""
	}

	// Validate cron expression
	cronExp := strings.TrimSpace(m.inputs[1].Value())
	if cronExp == "" {
		m.validationErrs[1] = "Cron expression is required"
		valid = false
	} else {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(cronExp); err != nil {
			m.validationErrs[1] = fmt.Sprintf("Invalid cron: %v", err)
			valid = false
		} else {
			m.validationErrs[1] = ""
		}
	}

	// Validate duration (positive integer)
	durationStr := strings.TrimSpace(m.inputs[2].Value())
	if durationStr == "" {
		m.validationErrs[2] = "Duration is required"
		valid = false
	} else {
		duration, err := strconv.Atoi(durationStr)
		if err != nil || duration <= 0 {
			m.validationErrs[2] = "Duration must be a positive number"
			valid = false
		} else {
			m.validationErrs[2] = ""
		}
	}

	// Validate channel ID (required, non-empty)
	channelID := strings.TrimSpace(m.inputs[3].Value())
	if channelID == "" {
		m.validationErrs[3] = "Channel ID is required"
		valid = false
	} else {
		m.validationErrs[3] = ""
	}

	return valid
}

func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// Confirm deletion
		if m.selected >= 0 && m.selected < len(m.cfg.Scheduler.Blocks) {
			// Remove from config
			m.cfg.Scheduler.Blocks = append(
				m.cfg.Scheduler.Blocks[:m.selected],
				m.cfg.Scheduler.Blocks[m.selected+1:]...,
			)
			// Remove from list
			m.list.RemoveItem(m.selected)
		}
		m.state = stateListView
		return m, nil
	case "n", "N", "esc":
		// Cancel deletion
		m.state = stateListView
		return m, nil
	}
	return m, nil
}

func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "?":
		// Return to previous state
		m.state = m.prevState
		return m, nil
	}
	return m, nil
}

func (m *Model) saveBlock() {
	// Validate all inputs before saving
	if !m.validateInputs() {
		return
	}

	name := strings.TrimSpace(m.inputs[0].Value())
	cronExp := strings.TrimSpace(m.inputs[1].Value())
	duration, _ := strconv.Atoi(strings.TrimSpace(m.inputs[2].Value()))
	channelID := strings.TrimSpace(m.inputs[3].Value())

	newBlock := scheduler.Block{
		Name:      name,
		Cron:      cronExp,
		Duration:  duration,
		ChannelID: channelID,
		// Preserve filter if editing? For now, we overwrite or keep simple
	}

	if m.selected == -1 {
		m.cfg.Scheduler.Blocks = append(m.cfg.Scheduler.Blocks, newBlock)
		m.list.InsertItem(len(m.list.Items()), item{block: newBlock})
	} else {
		// Preserve existing filter
		newBlock.Filter = m.cfg.Scheduler.Blocks[m.selected].Filter
		m.cfg.Scheduler.Blocks[m.selected] = newBlock
		// Update list item
		m.list.SetItem(m.selected, item{block: newBlock})
	}
}

func (m *Model) loadSeriesStates() {
	if m.store == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	states, err := m.store.ExportAllSeriesStates(ctx)
	if err != nil {
		m.seriesStates = nil
		return
	}

	m.seriesStates = states
	m.seriesScroll = 0
}

func (m Model) updateSeriesProgress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.state = stateListView
		return m, nil
	case "up", "k":
		if m.seriesScroll > 0 {
			m.seriesScroll--
		}
		return m, nil
	case "down", "j":
		if m.seriesScroll < len(m.seriesStates)-1 {
			m.seriesScroll++
		}
		return m, nil
	case "e":
		// Edit series state - future feature
		return m, nil
	case "r":
		m.loadSeriesStates()
		return m, nil
	}
	return m, nil
}

// View renders the TUI.
func (m Model) View() string {
	if m.state == stateListView {
		helpText := "\n\nPress 'n' to create new block, 'd' to delete, 'enter' to edit, '?' for help, 'q' to quit"
		if m.store != nil {
			helpText = "\n\nPress 'n' to create, 'd' to delete, 'enter' to edit, 's' for series progress, '?' for help, 'q' to quit"
		}
		help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(helpText)
		return lipgloss.NewStyle().Margin(1, 2).Render(m.list.View() + help)
	}

	if m.state == stateEditBlock {
		var builder strings.Builder
		builder.WriteString("Edit Block\n\n")

		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

		for i := range m.inputs {
			builder.WriteString(m.inputs[i].View())
			builder.WriteString("\n")

			// Display validation error if present
			if m.validationErrs[i] != "" {
				builder.WriteString(errorStyle.Render("  ⚠ " + m.validationErrs[i]))
				builder.WriteString("\n")
			}
		}

		button := "[ Save ]"
		if m.focusIndex == len(m.inputs) {
			button = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("[ Save ]")
		}
		builder.WriteString("\n")
		builder.WriteString(button)
		builder.WriteString("\n\n(esc to cancel, ? for help)")

		return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
	}

	if m.state == stateConfirmDelete {
		var builder strings.Builder

		warningStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

		blockName := ""
		if m.selected >= 0 && m.selected < len(m.cfg.Scheduler.Blocks) {
			blockName = m.cfg.Scheduler.Blocks[m.selected].Name
		}

		builder.WriteString(warningStyle.Render("⚠ Delete Block\n\n"))
		builder.WriteString(fmt.Sprintf("Are you sure you want to delete the block '%s'?\n\n", blockName))
		builder.WriteString("This action cannot be undone.\n\n")
		builder.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("[Y]es") + " / ")
		builder.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[N]o"))

		return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
	}

	if m.state == stateSeriesProgress {
		return m.renderSeriesProgress()
	}

	if m.state == stateCronBuilder {
		return m.renderCronBuilder()
	}

	if m.state == stateHelp {
		return m.renderHelp()
	}

	return "Unknown state"
}

func (m Model) renderSeriesProgress() string {
	var builder strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	highlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	disabledStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	completedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("34"))

	builder.WriteString(titleStyle.Render("Series Progress Viewer") + "\n\n")

	if len(m.seriesStates) == 0 {
		builder.WriteString(normalStyle.Render("No series states found.\n\n"))
		builder.WriteString(normalStyle.Render("Series states are created when series-based scheduling blocks run.\n"))
		builder.WriteString(normalStyle.Render("Press 'esc' or 'q' to return to the main menu."))
		return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
	}

	// Header
	builder.WriteString(headerStyle.Render(fmt.Sprintf("%-40s %-12s %-12s %-10s\n",
		"Series Title", "Episode", "Last Aired", "Status")))
	builder.WriteString(strings.Repeat("─", 80) + "\n")

	// Show series with scrolling
	start := m.seriesScroll
	end := start + 15
	if end > len(m.seriesStates) {
		end = len(m.seriesStates)
	}

	for i := start; i < end; i++ {
		state := m.seriesStates[i]

		// Determine style based on state
		style := normalStyle
		if i == m.seriesScroll {
			style = highlightStyle
		} else if state.Disabled {
			style = disabledStyle
		} else if state.Completed {
			style = completedStyle
		}

		// Format episode info
		episodeInfo := fmt.Sprintf("S%02dE%02d", state.CurrentSeason, state.CurrentEpisode)
		if state.RunCount > 0 {
			episodeInfo = fmt.Sprintf("%s (R%d)", episodeInfo, state.RunCount)
		}

		// Format last aired
		lastAired := "Never"
		if !state.LastAired.IsZero() {
			lastAired = state.LastAired.Format("2006-01-02")
		}

		// Status
		status := "Active"
		if state.Disabled {
			status = "Disabled"
		} else if state.Completed {
			status = "Completed"
		}

		// Calculate completion percentage (approximate based on current episode)
		totalEps := state.CurrentSeason*20 + state.CurrentEpisode // rough estimate
		percentage := 0
		if totalEps > 0 {
			percentage = int(float64(totalEps) / float64(totalEps+10) * 100)
		}

		line := fmt.Sprintf("%-40s %-12s %-12s %-10s %3d%%",
			truncate(state.ShowTitle, 40),
			episodeInfo,
			lastAired,
			status,
			percentage)

		builder.WriteString(style.Render(line) + "\n")
	}

	builder.WriteString("\n")
	builder.WriteString(normalStyle.Render(fmt.Sprintf("Showing %d-%d of %d series\n\n", start+1, end, len(m.seriesStates))))

	// Help text
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	builder.WriteString(helpStyle.Render("↑/↓, j/k: Navigate | e: Edit (future) | r: Refresh | esc/q: Back | ?: Help"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// initCronBuilder initializes the cron builder state by parsing the current cron expression
func (m *Model) initCronBuilder() {
	// Parse existing cron expression from the input field
	cronExpr := m.inputs[1].Value()
	parts := strings.Fields(cronExpr)

	// Initialize with defaults or parsed values
	m.cronMinute = "*"
	m.cronHour = "*"
	m.cronDayOfMonth = "*"
	m.cronMonth = "*"
	m.cronDayOfWeek = "*"
	m.cronFieldIndex = 0

	if len(parts) >= 5 {
		m.cronMinute = parts[0]
		m.cronHour = parts[1]
		m.cronDayOfMonth = parts[2]
		m.cronMonth = parts[3]
		m.cronDayOfWeek = parts[4]
	}
}

// updateCronBuilder handles key events in the cron builder state
func (m Model) updateCronBuilder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// Cancel and return to edit block
		m.state = stateEditBlock
		return m, nil
	case "enter":
		// Apply the built cron expression
		m.applyCronExpression()
		m.state = stateEditBlock
		return m, nil
	case "tab", "right", "l":
		// Move to next field
		m.cronFieldIndex = (m.cronFieldIndex + 1) % 5
		return m, nil
	case "shift+tab", "left", "h":
		// Move to previous field
		m.cronFieldIndex = (m.cronFieldIndex - 1 + 5) % 5
		return m, nil
	case "up", "k":
		// Increment current field value
		m.incrementCronField()
		return m, nil
	case "down", "j":
		// Decrement current field value
		m.decrementCronField()
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
		// Direct input - build number
		m.appendToCronField(msg.String())
		return m, nil
	case "backspace":
		// Remove last character from current field
		m.backspaceCronField()
		return m, nil
	case "*":
		// Set current field to wildcard
		m.setCronFieldWildcard()
		return m, nil
	}
	return m, nil
}

// applyCronExpression builds the final cron expression and applies it to the input field
func (m *Model) applyCronExpression() {
	cronExpr := fmt.Sprintf("%s %s %s %s %s",
		m.cronMinute, m.cronHour, m.cronDayOfMonth, m.cronMonth, m.cronDayOfWeek)
	m.inputs[1].SetValue(cronExpr)
}

// incrementCronField cycles through common values for the current field
func (m *Model) incrementCronField() {
	switch m.cronFieldIndex {
	case 0: // Minute
		m.cycleCronValue(&m.cronMinute, []string{"*", "0", "15", "30", "45", "*/5", "*/10", "*/15", "*/30"})
	case 1: // Hour
		m.cycleCronValue(&m.cronHour, []string{"*", "0", "6", "9", "12", "15", "18", "21", "*/2", "*/3", "*/6"})
	case 2: // Day of month
		m.cycleCronValue(&m.cronDayOfMonth, []string{"*", "1", "15", "*/7"})
	case 3: // Month
		m.cycleCronValue(&m.cronMonth, []string{"*", "1", "3", "6", "9", "12"})
	case 4: // Day of week
		m.cycleCronValue(&m.cronDayOfWeek, []string{"*", "0", "1", "2", "3", "4", "5", "6", "1-5"})
	}
}

// decrementCronField cycles through common values in reverse
func (m *Model) decrementCronField() {
	switch m.cronFieldIndex {
	case 0: // Minute
		m.reverseCycleCronValue(&m.cronMinute, []string{"*", "0", "15", "30", "45", "*/5", "*/10", "*/15", "*/30"})
	case 1: // Hour
		m.reverseCycleCronValue(&m.cronHour, []string{"*", "0", "6", "9", "12", "15", "18", "21", "*/2", "*/3", "*/6"})
	case 2: // Day of month
		m.reverseCycleCronValue(&m.cronDayOfMonth, []string{"*", "1", "15", "*/7"})
	case 3: // Month
		m.reverseCycleCronValue(&m.cronMonth, []string{"*", "1", "3", "6", "9", "12"})
	case 4: // Day of week
		m.reverseCycleCronValue(&m.cronDayOfWeek, []string{"*", "0", "1", "2", "3", "4", "5", "6", "1-5"})
	}
}

// cycleCronValue moves forward through preset values
func (m *Model) cycleCronValue(field *string, values []string) {
	for i, v := range values {
		if v == *field {
			*field = values[(i+1)%len(values)]
			return
		}
	}
	*field = values[0]
}

// reverseCycleCronValue moves backward through preset values
func (m *Model) reverseCycleCronValue(field *string, values []string) {
	for i, v := range values {
		if v == *field {
			*field = values[(i-1+len(values))%len(values)]
			return
		}
	}
	*field = values[len(values)-1]
}

// appendToCronField adds a digit to the current field
func (m *Model) appendToCronField(digit string) {
	field := m.getCurrentCronField()
	if *field == "*" || *field == "" {
		*field = digit
	} else {
		*field += digit
	}
}

// backspaceCronField removes the last character
func (m *Model) backspaceCronField() {
	field := m.getCurrentCronField()
	if len(*field) > 0 {
		*field = (*field)[:len(*field)-1]
	}
	if *field == "" {
		*field = "*"
	}
}

// setCronFieldWildcard sets the current field to *
func (m *Model) setCronFieldWildcard() {
	field := m.getCurrentCronField()
	*field = "*"
}

// getCurrentCronField returns a pointer to the currently selected field
func (m *Model) getCurrentCronField() *string {
	switch m.cronFieldIndex {
	case 0:
		return &m.cronMinute
	case 1:
		return &m.cronHour
	case 2:
		return &m.cronDayOfMonth
	case 3:
		return &m.cronMonth
	case 4:
		return &m.cronDayOfWeek
	}
	return &m.cronMinute
}

// renderCronBuilder displays the interactive cron expression builder
func (m Model) renderCronBuilder() string {
	var builder strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Background(lipgloss.Color("236")).
		Bold(true)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	builder.WriteString(titleStyle.Render("Visual Cron Expression Builder") + "\n\n")

	// Field labels
	fields := []struct {
		label string
		value *string
		desc  string
	}{
		{"Minute", &m.cronMinute, "(0-59)"},
		{"Hour", &m.cronHour, "(0-23)"},
		{"Day", &m.cronDayOfMonth, "(1-31)"},
		{"Month", &m.cronMonth, "(1-12)"},
		{"Weekday", &m.cronDayOfWeek, "(0-6, 0=Sun)"},
	}

	for i, field := range fields {
		var fieldStyle lipgloss.Style
		if i == m.cronFieldIndex {
			fieldStyle = activeStyle
		} else {
			fieldStyle = inactiveStyle
		}

		builder.WriteString(labelStyle.Render(fmt.Sprintf("%-10s", field.label)))
		builder.WriteString(" ")
		builder.WriteString(fieldStyle.Render(fmt.Sprintf(" %-15s ", *field.value)))
		builder.WriteString(" ")
		builder.WriteString(helpStyle.Render(field.desc))
		builder.WriteString("\n")
	}

	// Preview
	builder.WriteString("\n")
	builder.WriteString(labelStyle.Render("Preview: "))
	cronExpr := fmt.Sprintf("%s %s %s %s %s",
		m.cronMinute, m.cronHour, m.cronDayOfMonth, m.cronMonth, m.cronDayOfWeek)
	builder.WriteString(activeStyle.Render(cronExpr))
	builder.WriteString("\n\n")

	// Human-readable description
	builder.WriteString(labelStyle.Render("Description: "))
	builder.WriteString(m.describeCronExpression())
	builder.WriteString("\n\n")

	// Common presets
	builder.WriteString(titleStyle.Render("Common Presets:") + "\n")
	builder.WriteString(helpStyle.Render("  Every minute:      * * * * *\n"))
	builder.WriteString(helpStyle.Render("  Hourly:            0 * * * *\n"))
	builder.WriteString(helpStyle.Render("  Daily at 6am:      0 6 * * *\n"))
	builder.WriteString(helpStyle.Render("  Weekly (Monday):   0 0 * * 1\n"))
	builder.WriteString(helpStyle.Render("  Monthly (1st):     0 0 1 * *\n"))
	builder.WriteString(helpStyle.Render("  Weekdays 9am:      0 9 * * 1-5\n\n"))

	// Help
	builder.WriteString(helpStyle.Render("Navigation: tab/shift+tab, ←/→, h/l\n"))
	builder.WriteString(helpStyle.Render("Change value: ↑/↓, j/k (cycle presets)\n"))
	builder.WriteString(helpStyle.Render("Direct input: 0-9, * (wildcard), backspace\n"))
	builder.WriteString(helpStyle.Render("Apply: enter | Cancel: esc/q | Help: ?"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

// describeCronExpression provides a human-readable description
func (m Model) describeCronExpression() string {
	parts := make([]string, 0, 5)

	// Minute
	parts = append(parts, m.describeMinuteField()...)
	// Hour
	parts = append(parts, m.describeHourField()...)
	// Day of month
	parts = append(parts, m.describeDayOfMonthField()...)
	// Month
	parts = append(parts, m.describeMonthField()...)
	// Day of week
	parts = append(parts, m.describeDayOfWeekField()...)

	if len(parts) == 0 {
		return "runs continuously"
	}

	return strings.Join(parts, ", ")
}

// describeMinuteField returns description for minute field
func (m Model) describeMinuteField() []string {
	if m.cronMinute == "*" {
		return []string{"every minute"}
	}
	if strings.HasPrefix(m.cronMinute, "*/") {
		return []string{"every " + strings.TrimPrefix(m.cronMinute, "*/") + " minutes"}
	}
	return []string{"at minute " + m.cronMinute}
}

// describeHourField returns description for hour field
func (m Model) describeHourField() []string {
	if m.cronHour == "*" {
		return nil
	}
	if strings.HasPrefix(m.cronHour, "*/") {
		return []string{"every " + strings.TrimPrefix(m.cronHour, "*/") + " hours"}
	}
	return []string{"at hour " + m.cronHour}
}

// describeDayOfMonthField returns description for day of month field
func (m Model) describeDayOfMonthField() []string {
	if m.cronDayOfMonth == "*" {
		return nil
	}
	if strings.HasPrefix(m.cronDayOfMonth, "*/") {
		return []string{"every " + strings.TrimPrefix(m.cronDayOfMonth, "*/") + " days"}
	}
	return []string{"on day " + m.cronDayOfMonth}
}

// describeMonthField returns description for month field
func (m Model) describeMonthField() []string {
	if m.cronMonth == "*" {
		return nil
	}

	monthNames := []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	if m.cronMonth >= "1" && m.cronMonth <= "12" {
		if month, err := strconv.Atoi(m.cronMonth); err == nil && month >= 1 && month <= 12 {
			return []string{"in " + monthNames[month]}
		}
	}
	return []string{"in month " + m.cronMonth}
}

// describeDayOfWeekField returns description for day of week field
func (m Model) describeDayOfWeekField() []string {
	if m.cronDayOfWeek == "*" {
		return nil
	}

	if m.cronDayOfWeek == "1-5" {
		return []string{"on weekdays"}
	}

	dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	if m.cronDayOfWeek >= "0" && m.cronDayOfWeek <= "6" {
		if day, err := strconv.Atoi(m.cronDayOfWeek); err == nil && day >= 0 && day <= 6 {
			return []string{"on " + dayNames[day]}
		}
	}
	return []string{"on day " + m.cronDayOfWeek}
}

func (m Model) renderHelp() string {
	var builder strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205"))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	builder.WriteString(titleStyle.Render("Schedularr TUI - Help") + "\n\n")

	// Context-sensitive help based on previous state
	switch m.prevState {
	case stateListView:
		builder.WriteString(titleStyle.Render("Block List View") + "\n\n")
		builder.WriteString(keyStyle.Render("  ↑/↓, j/k") + descStyle.Render("  Navigate through blocks\n"))
		builder.WriteString(keyStyle.Render("  enter") + descStyle.Render("      Edit selected block\n"))
		builder.WriteString(keyStyle.Render("  n") + descStyle.Render("          Create new block\n"))
		builder.WriteString(keyStyle.Render("  d, delete") + descStyle.Render("  Delete selected block\n"))
		builder.WriteString(keyStyle.Render("  /") + descStyle.Render("          Search/filter blocks\n"))
		if m.store != nil {
			builder.WriteString(keyStyle.Render("  s") + descStyle.Render("          View series progress\n"))
		}
		builder.WriteString(keyStyle.Render("  q, ctrl+c") + descStyle.Render("  Quit application\n"))

	case stateEditBlock:
		builder.WriteString(titleStyle.Render("Block Editor") + "\n\n")
		builder.WriteString(keyStyle.Render("  tab") + descStyle.Render("         Move to next field\n"))
		builder.WriteString(keyStyle.Render("  shift+tab") + descStyle.Render("   Move to previous field\n"))
		builder.WriteString(keyStyle.Render("  ↑/↓") + descStyle.Render("         Navigate between fields\n"))
		builder.WriteString(keyStyle.Render("  ctrl+b") + descStyle.Render("      Open visual cron builder (on cron field)\n"))
		builder.WriteString(keyStyle.Render("  enter") + descStyle.Render("       Save block (when on Save button)\n"))
		builder.WriteString(keyStyle.Render("  esc") + descStyle.Render("         Cancel and return to list\n\n"))
		builder.WriteString(descStyle.Render("Fields are validated in real-time:\n"))
		builder.WriteString(descStyle.Render("  • Name: Required, non-empty\n"))
		builder.WriteString(descStyle.Render("  • Cron: Valid cron expression (e.g., '0 20 * * *')\n"))
		builder.WriteString(descStyle.Render("  • Duration: Positive number (minutes)\n"))
		builder.WriteString(descStyle.Render("  • Channel ID: Required, non-empty\n"))

	case stateConfirmDelete:
		builder.WriteString(titleStyle.Render("Delete Confirmation") + "\n\n")
		builder.WriteString(keyStyle.Render("  y, Y") + descStyle.Render("  Confirm deletion\n"))
		builder.WriteString(keyStyle.Render("  n, N") + descStyle.Render("  Cancel deletion\n"))
		builder.WriteString(keyStyle.Render("  esc") + descStyle.Render("    Cancel deletion\n"))

	case stateSeriesProgress:
		builder.WriteString(titleStyle.Render("Series Progress Viewer") + "\n\n")
		builder.WriteString(keyStyle.Render("  ↑/↓, j/k") + descStyle.Render("  Navigate through series\n"))
		builder.WriteString(keyStyle.Render("  r") + descStyle.Render("          Refresh series states from database\n"))
		builder.WriteString(keyStyle.Render("  e") + descStyle.Render("          Edit series state (future feature)\n"))
		builder.WriteString(keyStyle.Render("  esc, q") + descStyle.Render("     Return to block list\n\n"))
		builder.WriteString(descStyle.Render("Series Display:\n"))
		builder.WriteString(descStyle.Render("  • Shows current episode (S##E##) for each series\n"))
		builder.WriteString(descStyle.Render("  • (R#) indicates restart count when series completes\n"))
		builder.WriteString(descStyle.Render("  • Last aired date shows when episode was scheduled\n"))
		builder.WriteString(descStyle.Render("  • Status: Active, Completed, or Disabled\n"))
		builder.WriteString(descStyle.Render("  • Percentage shows approximate progress\n"))

	case stateCronBuilder:
		builder.WriteString(titleStyle.Render("Visual Cron Expression Builder") + "\n\n")
		builder.WriteString(keyStyle.Render("  tab, →, l") + descStyle.Render("     Move to next field\n"))
		builder.WriteString(keyStyle.Render("  shift+tab, ←, h") + descStyle.Render(" Move to previous field\n"))
		builder.WriteString(keyStyle.Render("  ↑, k") + descStyle.Render("           Cycle through preset values (forward)\n"))
		builder.WriteString(keyStyle.Render("  ↓, j") + descStyle.Render("           Cycle through preset values (backward)\n"))
		builder.WriteString(keyStyle.Render("  0-9") + descStyle.Render("            Direct numeric input\n"))
		builder.WriteString(keyStyle.Render("  *") + descStyle.Render("              Set field to wildcard (any value)\n"))
		builder.WriteString(keyStyle.Render("  backspace") + descStyle.Render("      Remove last character\n"))
		builder.WriteString(keyStyle.Render("  enter") + descStyle.Render("          Apply cron expression and return to editor\n"))
		builder.WriteString(keyStyle.Render("  esc, q") + descStyle.Render("         Cancel and return to editor\n\n"))
		builder.WriteString(descStyle.Render("Cron Fields:\n"))
		builder.WriteString(descStyle.Render("  • Minute: 0-59 or * for every minute\n"))
		builder.WriteString(descStyle.Render("  • Hour: 0-23 or * for every hour\n"))
		builder.WriteString(descStyle.Render("  • Day: 1-31 or * for every day\n"))
		builder.WriteString(descStyle.Render("  • Month: 1-12 or * for every month\n"))
		builder.WriteString(descStyle.Render("  • Weekday: 0-6 (0=Sun) or * for any day\n\n"))
		builder.WriteString(descStyle.Render("Special Values:\n"))
		builder.WriteString(descStyle.Render("  • */N: Every N units (e.g., */5 = every 5 minutes)\n"))
		builder.WriteString(descStyle.Render("  • X-Y: Range (e.g., 1-5 = Monday through Friday)\n"))
		builder.WriteString(descStyle.Render("  • *: Wildcard matching any value\n"))

	default:
		builder.WriteString(titleStyle.Render("General Commands") + "\n\n")
		builder.WriteString(keyStyle.Render("  ?") + descStyle.Render("  Show this help\n"))
		builder.WriteString(keyStyle.Render("  q") + descStyle.Render("  Quit application\n"))
	}

	builder.WriteString("\n")
	builder.WriteString(titleStyle.Render("Cron Expression Examples:") + "\n\n")
	builder.WriteString(descStyle.Render("  0 20 * * *     Every day at 8:00 PM\n"))
	builder.WriteString(descStyle.Render("  0 6 * * 1-5    Weekdays at 6:00 AM\n"))
	builder.WriteString(descStyle.Render("  0 0 * * 0      Sundays at midnight\n"))
	builder.WriteString(descStyle.Render("  */30 * * * *   Every 30 minutes\n"))
	builder.WriteString(descStyle.Render("  0 9-17 * * *   Every hour from 9 AM to 5 PM\n"))

	builder.WriteString("\n")
	builder.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Press ?, q, or esc to close help"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}
