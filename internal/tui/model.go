// Package tui provides a terminal user interface for managing Schedularr configuration.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/cronbuilder"
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
	stateSeriesSelector
	stateFilterBuilder
	stateFileBrowser
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
	cronExpr       *cronbuilder.Expression
	cronFieldIndex int
	// Series Selector state
	seriesSearchInput textinput.Model
	seriesSearchList  []string // List of all available series
	seriesFiltered    []string // Filtered series based on search
	seriesSelectIdx   int      // Currently selected series in list
	// Filter Builder state
	filterFieldIndex int      // Currently focused filter field
	filterGenres     []string // Available genres for selection
	filterRatings    []string // Available ratings for selection
	filterYearFrom   string
	filterYearTo     string
	filterMinDur     string
	filterMaxDur     string
	filterTitlePat   string
	// File Browser state
	schedulerFiles     []string // List of scheduler files found
	fileBrowserIdx     int      // Currently selected file
	fileBrowserDir     string   // Current directory being browsed
	fileBrowserMessage string   // Status message for file browser
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
	case stateSeriesSelector:
		return m.updateSeriesSelector(msg)
	case stateFilterBuilder:
		return m.updateFilterBuilder(msg)
	case stateFileBrowser:
		return m.updateFileBrowser(msg)
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
	case "S":
		// Series selector with search
		m.loadSeriesStates()
		m.initSeriesSelector()
		m.state = stateSeriesSelector
		return m, nil
	case "f":
		// File browser
		m.initFileBrowser()
		m.state = stateFileBrowser
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
	case "ctrl+f":
		// Open filter builder
		m.initFilterBuilder()
		m.state = stateFilterBuilder
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
	switch m.state {
	case stateListView:
		return m.renderListView()
	case stateEditBlock:
		return m.renderEditBlock()
	case stateConfirmDelete:
		return m.renderConfirmDelete()
	case stateSeriesProgress:
		return m.renderSeriesProgress()
	case stateCronBuilder:
		return m.renderCronBuilder()
	case stateSeriesSelector:
		return m.renderSeriesSelector()
	case stateFilterBuilder:
		return m.renderFilterBuilder()
	case stateFileBrowser:
		return m.renderFileBrowser()
	case stateHelp:
		return m.renderHelp()
	default:
		return "Unknown state"
	}
}

func (m Model) renderListView() string {
	helpText := "\n\n'n' new | 'd' delete | enter edit | 'f' files | 'S' series search | '?' help | 'q' quit"
	if m.store != nil {
		helpText = "\n\n'n' new | 'd' delete | enter edit | 's' series | 'S' search | 'f' files | '?' help | 'q' quit"
	}
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(helpText)
	return lipgloss.NewStyle().Margin(1, 2).Render(m.list.View() + help)
}

func (m Model) renderEditBlock() string {
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
	builder.WriteString("\n\n(esc cancel | ctrl+b cron builder | ctrl+f filter builder | ? help)")

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

func (m Model) renderConfirmDelete() string {
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
		if state.LastAired != nil && !state.LastAired.IsZero() {
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
	cronExprStr := m.inputs[1].Value()
	m.cronExpr = cronbuilder.Parse(cronExprStr)
	m.cronFieldIndex = 0
}

// updateCronBuilder handles key events in the cron builder state
func (m Model) updateCronBuilder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ft := cronbuilder.FieldType(m.cronFieldIndex)
	switch msg.String() {
	case "esc", "q":
		// Cancel and return to edit block
		m.state = stateEditBlock
		return m, nil
	case "enter":
		// Apply the built cron expression
		m.inputs[1].SetValue(m.cronExpr.String())
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
		m.cronExpr.CycleNext(ft)
		return m, nil
	case "down", "j":
		// Decrement current field value
		m.cronExpr.CyclePrev(ft)
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
		// Direct input - build number
		m.cronExpr.AppendDigit(ft, msg.String())
		return m, nil
	case "backspace":
		// Remove last character from current field
		m.cronExpr.Backspace(ft)
		return m, nil
	case "*":
		// Set current field to wildcard
		m.cronExpr.SetWildcard(ft)
		return m, nil
	}
	return m, nil
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

	// Field labels using cronbuilder.Fields()
	fields := cronbuilder.Fields()
	for i, field := range fields {
		var fieldStyle lipgloss.Style
		if i == m.cronFieldIndex {
			fieldStyle = activeStyle
		} else {
			fieldStyle = inactiveStyle
		}

		value := *m.cronExpr.Field(field.Type)
		builder.WriteString(labelStyle.Render(fmt.Sprintf("%-10s", field.Label)))
		builder.WriteString(" ")
		builder.WriteString(fieldStyle.Render(fmt.Sprintf(" %-15s ", value)))
		builder.WriteString(" ")
		builder.WriteString(helpStyle.Render(field.Desc))
		builder.WriteString("\n")
	}

	// Preview
	builder.WriteString("\n")
	builder.WriteString(labelStyle.Render("Preview: "))
	builder.WriteString(activeStyle.Render(m.cronExpr.String()))
	builder.WriteString("\n\n")

	// Human-readable description
	builder.WriteString(labelStyle.Render("Description: "))
	builder.WriteString(m.cronExpr.Describe())
	builder.WriteString("\n\n")

	// Common presets
	builder.WriteString(titleStyle.Render("Common Presets:") + "\n")
	for _, preset := range cronbuilder.CommonPresets() {
		builder.WriteString(helpStyle.Render(fmt.Sprintf("  %-18s %s\n", preset.Label+":", preset.Expr)))
	}
	builder.WriteString("\n")

	// Help
	builder.WriteString(helpStyle.Render("Navigation: tab/shift+tab, ←/→, h/l\n"))
	builder.WriteString(helpStyle.Render("Change value: ↑/↓, j/k (cycle presets)\n"))
	builder.WriteString(helpStyle.Render("Direct input: 0-9, * (wildcard), backspace\n"))
	builder.WriteString(helpStyle.Render("Apply: enter | Cancel: esc/q | Help: ?"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
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
		builder.WriteString(keyStyle.Render("  S") + descStyle.Render("          Search series (series selector)\n"))
		builder.WriteString(keyStyle.Render("  f") + descStyle.Render("          Browse scheduler files\n"))
		builder.WriteString(keyStyle.Render("  q, ctrl+c") + descStyle.Render("  Quit application\n"))

	case stateEditBlock:
		builder.WriteString(titleStyle.Render("Block Editor") + "\n\n")
		builder.WriteString(keyStyle.Render("  tab") + descStyle.Render("         Move to next field\n"))
		builder.WriteString(keyStyle.Render("  shift+tab") + descStyle.Render("   Move to previous field\n"))
		builder.WriteString(keyStyle.Render("  ↑/↓") + descStyle.Render("         Navigate between fields\n"))
		builder.WriteString(keyStyle.Render("  ctrl+b") + descStyle.Render("      Open visual cron builder (on cron field)\n"))
		builder.WriteString(keyStyle.Render("  ctrl+f") + descStyle.Render("      Open filter builder\n"))
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

	case stateSeriesSelector:
		builder.WriteString(titleStyle.Render("Series Selector") + "\n\n")
		builder.WriteString(keyStyle.Render("  type") + descStyle.Render("        Search for series by name\n"))
		builder.WriteString(keyStyle.Render("  ↑/↓, j/k") + descStyle.Render("  Navigate through results\n"))
		builder.WriteString(keyStyle.Render("  enter") + descStyle.Render("       Select highlighted series\n"))
		builder.WriteString(keyStyle.Render("  esc, q") + descStyle.Render("     Cancel and return to list\n\n"))
		builder.WriteString(descStyle.Render("Series List:\n"))
		builder.WriteString(descStyle.Render("  • Shows series from your scheduling blocks\n"))
		builder.WriteString(descStyle.Render("  • Includes series from schedule history\n"))
		builder.WriteString(descStyle.Render("  • Type to filter results in real-time\n"))

	case stateFilterBuilder:
		builder.WriteString(titleStyle.Render("Filter Builder") + "\n\n")
		builder.WriteString(keyStyle.Render("  ↑/↓, j/k") + descStyle.Render("  Navigate between fields\n"))
		builder.WriteString(keyStyle.Render("  tab") + descStyle.Render("         Move to next field\n"))
		builder.WriteString(keyStyle.Render("  space") + descStyle.Render("       Add genre/rating (cycles through options)\n"))
		builder.WriteString(keyStyle.Render("  backspace") + descStyle.Render("   Remove last item or character\n"))
		builder.WriteString(keyStyle.Render("  0-9") + descStyle.Render("         Enter year/duration values\n"))
		builder.WriteString(keyStyle.Render("  enter") + descStyle.Render("       Apply filter (when on Apply button)\n"))
		builder.WriteString(keyStyle.Render("  esc, q") + descStyle.Render("     Cancel and return to editor\n\n"))
		builder.WriteString(descStyle.Render("Filter Fields:\n"))
		builder.WriteString(descStyle.Render("  • Title Pattern: Regex to match content titles\n"))
		builder.WriteString(descStyle.Render("  • Genres: Filter by genre categories\n"))
		builder.WriteString(descStyle.Render("  • Ratings: Filter by content ratings\n"))
		builder.WriteString(descStyle.Render("  • Year From/To: Release year range\n"))
		builder.WriteString(descStyle.Render("  • Min Duration: Minimum content length (minutes)\n"))

	case stateFileBrowser:
		builder.WriteString(titleStyle.Render("Scheduler File Browser") + "\n\n")
		builder.WriteString(keyStyle.Render("  ↑/↓, j/k") + descStyle.Render("  Navigate through files\n"))
		builder.WriteString(keyStyle.Render("  enter") + descStyle.Render("       Select file\n"))
		builder.WriteString(keyStyle.Render("  r") + descStyle.Render("           Refresh file list\n"))
		builder.WriteString(keyStyle.Render("  esc, q") + descStyle.Render("     Cancel and return to list\n\n"))
		builder.WriteString(descStyle.Render("Search Locations:\n"))
		builder.WriteString(descStyle.Render("  • Current working directory\n"))
		builder.WriteString(descStyle.Render("  • Home directory\n"))
		builder.WriteString(descStyle.Render("  • ~/.config/schedularr/\n"))
		builder.WriteString(descStyle.Render("  • /etc/schedularr/\n\n"))
		builder.WriteString(descStyle.Render("File Patterns: scheduler.yaml, scheduler.yml, *.scheduler.yaml\n"))

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

// ============================================================================
// Series Selector with Search
// ============================================================================

// initSeriesSelector initializes the series selector state
func (m *Model) initSeriesSelector() {
	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Search series..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 40
	m.seriesSearchInput = ti

	// Collect all unique series from series states and block configurations
	seriesSet := make(map[string]struct{})

	// From series states in the store
	for _, state := range m.seriesStates {
		if state.ShowTitle != "" {
			seriesSet[state.ShowTitle] = struct{}{}
		}
	}

	// From existing block configurations
	for _, block := range m.cfg.Scheduler.Blocks {
		for _, s := range block.Series {
			if s.ShowTitle != "" {
				seriesSet[s.ShowTitle] = struct{}{}
			}
		}
	}

	// Convert to sorted slice
	m.seriesSearchList = make([]string, 0, len(seriesSet))
	for title := range seriesSet {
		m.seriesSearchList = append(m.seriesSearchList, title)
	}
	sort.Strings(m.seriesSearchList)

	m.seriesFiltered = m.seriesSearchList
	m.seriesSelectIdx = 0
}

// updateSeriesSelector handles key events in the series selector state
func (m Model) updateSeriesSelector(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = stateListView
		return m, nil
	case "enter":
		// Select the current series and return to list view
		// Could be extended to add series to a block or view details
		m.state = stateListView
		return m, nil
	case "up", "k":
		if m.seriesSelectIdx > 0 {
			m.seriesSelectIdx--
		}
		return m, nil
	case "down", "j":
		if m.seriesSelectIdx < len(m.seriesFiltered)-1 {
			m.seriesSelectIdx++
		}
		return m, nil
	default:
		// Update search input
		var cmd tea.Cmd
		m.seriesSearchInput, cmd = m.seriesSearchInput.Update(msg)
		// Filter series based on search
		m.filterSeriesList()
		return m, cmd
	}
}

// filterSeriesList filters the series list based on search input
func (m *Model) filterSeriesList() {
	query := strings.ToLower(m.seriesSearchInput.Value())
	if query == "" {
		m.seriesFiltered = m.seriesSearchList
		m.seriesSelectIdx = 0
		return
	}

	m.seriesFiltered = make([]string, 0)
	for _, title := range m.seriesSearchList {
		if strings.Contains(strings.ToLower(title), query) {
			m.seriesFiltered = append(m.seriesFiltered, title)
		}
	}

	// Reset selection if out of bounds
	if m.seriesSelectIdx >= len(m.seriesFiltered) {
		m.seriesSelectIdx = 0
	}
}

// renderSeriesList renders the series list with scrolling
func (m Model) renderSeriesList(builder *strings.Builder, normalStyle, highlightStyle lipgloss.Style) {
	if len(m.seriesSearchList) == 0 {
		builder.WriteString(normalStyle.Render("No series found.\n"))
		builder.WriteString(normalStyle.Render("Series are added when you configure series-based scheduling blocks.\n"))
		return
	}

	if len(m.seriesFiltered) == 0 {
		builder.WriteString(normalStyle.Render("No series match your search.\n"))
		return
	}

	// Show filtered series with scrolling
	start, end := calculateScrollWindow(m.seriesSelectIdx, len(m.seriesFiltered), 10, 20)

	for i := start; i < end; i++ {
		prefix := "  "
		style := normalStyle
		if i == m.seriesSelectIdx {
			prefix = "▸ "
			style = highlightStyle
		}
		builder.WriteString(style.Render(prefix+m.seriesFiltered[i]) + "\n")
	}

	builder.WriteString("\n")
	builder.WriteString(normalStyle.Render(fmt.Sprintf("Showing %d of %d series", len(m.seriesFiltered), len(m.seriesSearchList))))
}

// calculateScrollWindow calculates start and end indices for scrolling lists
func calculateScrollWindow(selectedIdx, listLen, scrollOffset, windowSize int) (start, end int) {
	start = 0
	if selectedIdx > scrollOffset {
		start = selectedIdx - scrollOffset
	}
	end = start + windowSize
	if end > listLen {
		end = listLen
	}
	return start, end
}

// renderFileList renders the file browser list with scrolling
func (m Model) renderFileList(builder *strings.Builder, normalStyle, highlightStyle, helpStyle lipgloss.Style) {
	if len(m.schedulerFiles) == 0 {
		builder.WriteString(normalStyle.Render("No scheduler files found.\n\n"))
		builder.WriteString(normalStyle.Render("Searched locations:\n"))
		builder.WriteString(helpStyle.Render("  • Current directory\n"))
		builder.WriteString(helpStyle.Render("  • Home directory\n"))
		builder.WriteString(helpStyle.Render("  • ~/.config/schedularr/\n"))
		builder.WriteString(helpStyle.Render("  • /etc/schedularr/\n\n"))
		builder.WriteString(normalStyle.Render("Expected file names:\n"))
		builder.WriteString(helpStyle.Render("  • scheduler.yaml\n"))
		builder.WriteString(helpStyle.Render("  • scheduler.yml\n"))
		builder.WriteString(helpStyle.Render("  • *.scheduler.yaml\n"))
		return
	}

	builder.WriteString(normalStyle.Render("Found scheduler files:\n\n"))

	start, end := calculateScrollWindow(m.fileBrowserIdx, len(m.schedulerFiles), 8, 15)

	for i := start; i < end; i++ {
		prefix := "  "
		style := normalStyle
		if i == m.fileBrowserIdx {
			prefix = "▸ "
			style = highlightStyle
		}

		// Show relative path if possible, otherwise full path
		displayPath := m.schedulerFiles[i]
		if rel, err := filepath.Rel(m.fileBrowserDir, displayPath); err == nil && !strings.HasPrefix(rel, "..") {
			displayPath = "./" + rel
		}

		builder.WriteString(style.Render(prefix+displayPath) + "\n")
	}

	builder.WriteString("\n")
	builder.WriteString(normalStyle.Render(fmt.Sprintf("Showing %d-%d of %d files\n", start+1, end, len(m.schedulerFiles))))
}

// renderSeriesSelector renders the series selector view
func (m Model) renderSeriesSelector() string {
	var builder strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	highlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Background(lipgloss.Color("236")).
		Bold(true)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	builder.WriteString(titleStyle.Render("Series Selector") + "\n\n")
	builder.WriteString(normalStyle.Render("Search: "))
	builder.WriteString(m.seriesSearchInput.View())
	builder.WriteString("\n\n")

	m.renderSeriesList(&builder, normalStyle, highlightStyle)

	builder.WriteString("\n\n")
	builder.WriteString(helpStyle.Render("↑/↓, j/k: Navigate | enter: Select | esc/q: Cancel | ?: Help"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

// ============================================================================
// Filter Builder Interface
// ============================================================================

// Common genre options
var commonGenres = []string{
	"Action", "Adventure", "Animation", "Comedy", "Crime", "Documentary",
	"Drama", "Family", "Fantasy", "History", "Horror", "Music", "Mystery",
	"Romance", "Science Fiction", "Thriller", "War", "Western",
}

// Common rating options
var commonRatings = []string{
	"G", "PG", "PG-13", "R", "NC-17", "TV-Y", "TV-Y7", "TV-G", "TV-PG", "TV-14", "TV-MA",
}

// initFilterBuilder initializes the filter builder state
func (m *Model) initFilterBuilder() {
	m.filterFieldIndex = 0

	// Clear all filter fields first
	m.filterTitlePat = ""
	m.filterGenres = nil
	m.filterRatings = nil
	m.filterYearFrom = ""
	m.filterYearTo = ""
	m.filterMinDur = ""
	m.filterMaxDur = ""

	// Load from current block's filter if editing existing block
	if m.selected < 0 || m.selected >= len(m.cfg.Scheduler.Blocks) {
		return
	}

	filter := m.cfg.Scheduler.Blocks[m.selected].Filter
	m.filterTitlePat = filter.TitlePattern
	m.filterGenres = filter.Genres
	m.filterRatings = filter.Ratings
	m.filterYearFrom = intToStringOrEmpty(filter.YearFrom)
	m.filterYearTo = intToStringOrEmpty(filter.YearTo)
	m.filterMinDur = intToStringOrEmpty(filter.MinDuration)
	m.filterMaxDur = intToStringOrEmpty(filter.MaxDuration)
}

// intToStringOrEmpty converts an int to string, returning empty string for zero
func intToStringOrEmpty(n int) string {
	if n > 0 {
		return strconv.Itoa(n)
	}
	return ""
}

// updateFilterBuilder handles key events in the filter builder state
func (m Model) updateFilterBuilder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = stateEditBlock
		return m, nil
	case "enter":
		if m.filterFieldIndex == 6 { // Save button
			m.applyFilter()
			m.state = stateEditBlock
		}
		return m, nil
	case "tab", "down", "j":
		m.filterFieldIndex = (m.filterFieldIndex + 1) % 7
		return m, nil
	case "shift+tab", "up", "k":
		m.filterFieldIndex = (m.filterFieldIndex - 1 + 7) % 7
		return m, nil
	case " ":
		// Toggle genre/rating selection
		m.toggleFilterSelection()
		return m, nil
	case "backspace":
		m.handleFilterBackspace()
		return m, nil
	default:
		// Handle numeric/text input for appropriate fields
		m.handleFilterInput(msg.String())
		return m, nil
	}
}

// toggleFilterSelection toggles genre/rating selection
func (m *Model) toggleFilterSelection() {
	switch m.filterFieldIndex {
	case 1: // Genres - cycle through common genres
		m.filterGenres = toggleSliceItem(m.filterGenres, commonGenres)
	case 2: // Ratings
		m.filterRatings = toggleSliceItem(m.filterRatings, commonRatings)
	}
}

// toggleSliceItem adds the next item from options to items, or removes it if already present
func toggleSliceItem(items []string, options []string) []string {
	if len(items) >= len(options) {
		return items
	}
	nextItem := options[len(items)%len(options)]

	// Check if already present and remove
	for i, item := range items {
		if item == nextItem {
			return append(items[:i], items[i+1:]...)
		}
	}

	// Not present, add it
	return append(items, nextItem)
}

// handleFilterBackspace handles backspace in filter fields
func (m *Model) handleFilterBackspace() {
	switch m.filterFieldIndex {
	case 0: // Title pattern
		if len(m.filterTitlePat) > 0 {
			m.filterTitlePat = m.filterTitlePat[:len(m.filterTitlePat)-1]
		}
	case 1: // Genres - remove last genre
		if len(m.filterGenres) > 0 {
			m.filterGenres = m.filterGenres[:len(m.filterGenres)-1]
		}
	case 2: // Ratings - remove last rating
		if len(m.filterRatings) > 0 {
			m.filterRatings = m.filterRatings[:len(m.filterRatings)-1]
		}
	case 3: // Year from
		if len(m.filterYearFrom) > 0 {
			m.filterYearFrom = m.filterYearFrom[:len(m.filterYearFrom)-1]
		}
	case 4: // Year to
		if len(m.filterYearTo) > 0 {
			m.filterYearTo = m.filterYearTo[:len(m.filterYearTo)-1]
		}
	case 5: // Min duration
		if len(m.filterMinDur) > 0 {
			m.filterMinDur = m.filterMinDur[:len(m.filterMinDur)-1]
		}
	}
}

// handleFilterInput handles text input in filter fields
func (m *Model) handleFilterInput(input string) {
	// Only accept single printable characters
	if len(input) != 1 {
		return
	}

	switch m.filterFieldIndex {
	case 0: // Title pattern - accept any character
		m.filterTitlePat += input
	case 3: // Year from - only digits
		if input >= "0" && input <= "9" && len(m.filterYearFrom) < 4 {
			m.filterYearFrom += input
		}
	case 4: // Year to - only digits
		if input >= "0" && input <= "9" && len(m.filterYearTo) < 4 {
			m.filterYearTo += input
		}
	case 5: // Min duration - only digits
		if input >= "0" && input <= "9" && len(m.filterMinDur) < 4 {
			m.filterMinDur += input
		}
	}
}

// applyFilter saves the filter to the current block
func (m *Model) applyFilter() {
	if m.selected < 0 || m.selected >= len(m.cfg.Scheduler.Blocks) {
		return
	}

	filter := scheduler.Filter{
		TitlePattern: m.filterTitlePat,
		Genres:       m.filterGenres,
		Ratings:      m.filterRatings,
	}

	if m.filterYearFrom != "" {
		if year, err := strconv.Atoi(m.filterYearFrom); err == nil {
			filter.YearFrom = year
		}
	}
	if m.filterYearTo != "" {
		if year, err := strconv.Atoi(m.filterYearTo); err == nil {
			filter.YearTo = year
		}
	}
	if m.filterMinDur != "" {
		if dur, err := strconv.Atoi(m.filterMinDur); err == nil {
			filter.MinDuration = dur
		}
	}
	if m.filterMaxDur != "" {
		if dur, err := strconv.Atoi(m.filterMaxDur); err == nil {
			filter.MaxDuration = dur
		}
	}

	m.cfg.Scheduler.Blocks[m.selected].Filter = filter
}

// renderFilterBuilder renders the filter builder view
func (m Model) renderFilterBuilder() string {
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

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86"))

	builder.WriteString(titleStyle.Render("Filter Builder") + "\n\n")
	builder.WriteString(labelStyle.Render("Build filter criteria for content selection.\n"))
	builder.WriteString(labelStyle.Render("All criteria use AND logic (all must match).\n\n"))

	// Field definitions
	fields := []struct {
		label string
		value string
		hint  string
	}{
		{"Title Pattern", m.filterTitlePat, "Regex pattern to match titles"},
		{"Genres", strings.Join(m.filterGenres, ", "), "Press space to add, backspace to remove"},
		{"Ratings", strings.Join(m.filterRatings, ", "), "Press space to add, backspace to remove"},
		{"Year From", m.filterYearFrom, "Minimum release year"},
		{"Year To", m.filterYearTo, "Maximum release year"},
		{"Min Duration", m.filterMinDur, "Minimum duration in minutes"},
		{"[ Apply Filter ]", "", ""},
	}

	for i, field := range fields {
		var style lipgloss.Style
		if i == m.filterFieldIndex {
			style = activeStyle
		} else {
			style = inactiveStyle
		}

		if i == 6 { // Save button
			builder.WriteString("\n")
			builder.WriteString(style.Render(field.label))
			builder.WriteString("\n")
		} else {
			builder.WriteString(style.Render(fmt.Sprintf("%-15s", field.label)))
			builder.WriteString(" ")
			displayValue := field.value
			if displayValue == "" {
				displayValue = "(not set)"
			}
			builder.WriteString(valueStyle.Render(displayValue))
			builder.WriteString("\n")
			if i == m.filterFieldIndex && field.hint != "" {
				builder.WriteString(helpStyle.Render("  " + field.hint))
				builder.WriteString("\n")
			}
		}
	}

	builder.WriteString("\n")
	builder.WriteString(helpStyle.Render("Available genres: " + strings.Join(commonGenres[:6], ", ") + "...\n"))
	builder.WriteString(helpStyle.Render("Available ratings: " + strings.Join(commonRatings[:6], ", ") + "...\n\n"))
	builder.WriteString(helpStyle.Render("↑/↓, j/k: Navigate | space: Toggle | enter: Apply | esc/q: Cancel | ?: Help"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

// ============================================================================
// Scheduler File Browser
// ============================================================================

// initFileBrowser initializes the file browser state
func (m *Model) initFileBrowser() {
	m.fileBrowserIdx = 0
	m.fileBrowserMessage = ""

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		m.fileBrowserDir = "."
	} else {
		m.fileBrowserDir = cwd
	}

	m.scanForSchedulerFiles()
}

// scanForSchedulerFiles scans common locations for scheduler files
func (m *Model) scanForSchedulerFiles() {
	m.schedulerFiles = make([]string, 0)
	searchPaths := []string{
		m.fileBrowserDir,
	}

	// Add home directory
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, home)
		searchPaths = append(searchPaths, filepath.Join(home, ".config", "schedularr"))
	}

	// Add common config locations
	searchPaths = append(searchPaths, "/etc/schedularr")

	patterns := []string{"scheduler.yaml", "scheduler.yml", "*.scheduler.yaml", "*.scheduler.yml"}

	seen := make(map[string]struct{})
	for _, dir := range searchPaths {
		for _, pattern := range patterns {
			matches, err := filepath.Glob(filepath.Join(dir, pattern))
			if err != nil {
				continue
			}
			for _, match := range matches {
				absPath, err := filepath.Abs(match)
				if err != nil {
					absPath = match
				}
				if _, exists := seen[absPath]; !exists {
					seen[absPath] = struct{}{}
					m.schedulerFiles = append(m.schedulerFiles, absPath)
				}
			}
		}
	}

	sort.Strings(m.schedulerFiles)
}

// updateFileBrowser handles key events in the file browser state
func (m Model) updateFileBrowser(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = stateListView
		return m, nil
	case "enter":
		if len(m.schedulerFiles) > 0 && m.fileBrowserIdx < len(m.schedulerFiles) {
			selectedFile := m.schedulerFiles[m.fileBrowserIdx]
			m.fileBrowserMessage = "Selected: " + selectedFile
			// In a full implementation, this would load the scheduler file
			// For now, just show a confirmation message
		}
		return m, nil
	case "up", "k":
		if m.fileBrowserIdx > 0 {
			m.fileBrowserIdx--
		}
		return m, nil
	case "down", "j":
		if m.fileBrowserIdx < len(m.schedulerFiles)-1 {
			m.fileBrowserIdx++
		}
		return m, nil
	case "r":
		// Refresh file list
		m.scanForSchedulerFiles()
		m.fileBrowserMessage = "File list refreshed"
		return m, nil
	}
	return m, nil
}

// renderFileBrowser renders the file browser view
func (m Model) renderFileBrowser() string {
	var builder strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	highlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Background(lipgloss.Color("236")).
		Bold(true)

	pathStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("34"))

	builder.WriteString(titleStyle.Render("Scheduler File Browser") + "\n\n")
	builder.WriteString(normalStyle.Render("Current directory: "))
	builder.WriteString(pathStyle.Render(m.fileBrowserDir) + "\n\n")

	m.renderFileList(&builder, normalStyle, highlightStyle, helpStyle)

	if m.fileBrowserMessage != "" {
		builder.WriteString("\n")
		builder.WriteString(messageStyle.Render(m.fileBrowserMessage))
		builder.WriteString("\n")
	}

	builder.WriteString("\n")
	builder.WriteString(helpStyle.Render("↑/↓, j/k: Navigate | enter: Select | r: Refresh | esc/q: Cancel | ?: Help"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}
