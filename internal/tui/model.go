// Package tui provides a terminal user interface for managing Schedularr configuration.
package tui

import (
	"context"
	"errors"
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
	"github.com/charmbracelet/huh"
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
	stateSeriesEdit
)

type item struct {
	block scheduler.Block
}

func (i item) Title() string { return i.block.Name }
func (i item) Description() string {
	return fmt.Sprintf("%s | %d min | %s | pri:%d", i.block.Cron, i.block.Duration, i.block.ChannelID, i.block.Priority)
}
func (i item) FilterValue() string { return i.block.Name }

// Model is the Bubble Tea model for the TUI.
type Model struct {
	cfg            *config.Config
	store          *store.Store
	list           list.Model
	state          sessionState
	prevState      sessionState // previous state before help
	selected       int          // index of block being edited
	schedulerFile  string       // path to scheduler file for saving
	statusMessage  string       // status message to display
	hasUnsavedChanges bool      // track if there are unsaved changes
	// Block edit form (huh)
	blockForm     *huh.Form
	formName      string
	formCron      string
	formDuration  string
	formChannelID string
	// Series progress state
	seriesStates []scheduler.SeriesState
	seriesScroll int
	// Cron builder state
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
	// Series Edit state
	seriesEditSeason   string // Season number being edited
	seriesEditEpisode  string // Episode number being edited
	seriesEditField    int    // Currently focused field (0=season, 1=episode, 2=save)
	seriesEditShowTitle string // Title of series being edited
}

// NewModel creates a new TUI model with the given configuration and optional store.
// The schedulerFile parameter is the path to save scheduler config changes.
func NewModel(cfg *config.Config, st *store.Store, schedulerFile string) Model {
	items := make([]list.Item, len(cfg.Scheduler.Blocks))
	for i, b := range cfg.Scheduler.Blocks {
		items[i] = item{block: b}
	}

	// Setup list
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Scheduling Blocks"

	return Model{
		cfg:           cfg,
		store:         st,
		list:          l,
		state:         stateListView,
		schedulerFile: schedulerFile,
	}
}

// createBlockForm creates a new huh form for editing a block.
func (m *Model) createBlockForm() {
	// Cron validator
	cronValidator := func(s string) error {
		if s == "" {
			return errors.New("cron expression is required")
		}
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(s); err != nil {
			return fmt.Errorf("invalid cron: %w", err)
		}
		return nil
	}

	// Duration validator
	durationValidator := func(s string) error {
		if s == "" {
			return errors.New("duration is required")
		}
		dur, err := strconv.Atoi(s)
		if err != nil || dur <= 0 {
			return errors.New("duration must be a positive number")
		}
		return nil
	}

	// Required validator
	requiredValidator := func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("this field is required")
		}
		return nil
	}

	m.blockForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Block Name").
				Description("A descriptive name for this scheduling block").
				Value(&m.formName).
				Validate(requiredValidator),
			huh.NewInput().
				Title("Cron Expression").
				Description("e.g., 0 20 * * * (daily at 8 PM)").
				Value(&m.formCron).
				Validate(cronValidator),
			huh.NewInput().
				Title("Duration (minutes)").
				Description("How long this block runs").
				Value(&m.formDuration).
				Validate(durationValidator),
			huh.NewInput().
				Title("Channel ID").
				Description("Target Tunarr channel ID").
				Value(&m.formChannelID).
				Validate(requiredValidator),
		),
	).WithTheme(huh.ThemeDracula())
}

// Init initializes the TUI model.
func (m Model) Init() tea.Cmd {
	return nil
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
	case stateSeriesEdit:
		return m.updateSeriesEdit(msg)
	default:
		return m, nil
	}
}

func (m Model) updateListView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle block operations
	if handled, model, cmd := m.handleBlockOperations(msg); handled {
		return model, cmd
	}

	// Handle navigation and view switching
	if handled, model := m.handleViewSwitch(msg); handled {
		return model, nil
	}

	// Clear status message on any other key
	m.statusMessage = ""

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// handleBlockOperations handles block CRUD operations in list view.
func (m Model) handleBlockOperations(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		if m.hasUnsavedChanges {
			m.statusMessage = "Warning: Unsaved changes will be lost"
		}
		return true, m, tea.Quit
	case "enter":
		if len(m.cfg.Scheduler.Blocks) == 0 {
			return true, m, nil
		}
		m.startEditBlock(m.list.Index())
		return true, m, nil
	case "n":
		m.startEditBlock(-1)
		return true, m, nil
	case "d", "delete":
		if len(m.cfg.Scheduler.Blocks) == 0 {
			return true, m, nil
		}
		m.selected = m.list.Index()
		m.state = stateConfirmDelete
		return true, m, nil
	case "D":
		m.duplicateBlock()
		return true, m, nil
	case "ctrl+s":
		m.saveConfigToDisk()
		return true, m, nil
	case "+", "=":
		m.adjustPriority(1)
		return true, m, nil
	case "-", "_":
		m.adjustPriority(-1)
		return true, m, nil
	}
	return false, m, nil
}

// handleViewSwitch handles switching between different views.
// Returns (handled, model) - cmd is always nil for view switches.
func (m Model) handleViewSwitch(msg tea.KeyMsg) (bool, tea.Model) {
	switch msg.String() {
	case "s":
		if m.store != nil {
			m.loadSeriesStates()
			m.state = stateSeriesProgress
		}
		return true, m
	case "S":
		m.loadSeriesStates()
		m.initSeriesSelector()
		m.state = stateSeriesSelector
		return true, m
	case "f":
		m.initFileBrowser()
		m.state = stateFileBrowser
		return true, m
	}
	return false, m
}

func (m Model) updateEditBlock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle special key bindings before passing to form
	switch msg.String() {
	case "ctrl+b":
		// Open cron builder - update form value first
		m.initCronBuilder()
		m.state = stateCronBuilder
		return m, nil
	case "ctrl+f":
		// Open filter builder
		m.initFilterBuilder()
		m.state = stateFilterBuilder
		return m, nil
	}

	// Pass all other messages to the form
	form, cmd := m.blockForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.blockForm = f
	}

	// Check if form was completed or canceled
	if m.blockForm.State == huh.StateCompleted {
		m.saveBlock()
		m.state = stateListView
		return m, nil
	}
	if m.blockForm.State == huh.StateAborted {
		m.state = stateListView
		return m, nil
	}

	return m, cmd
}

func (m *Model) startEditBlock(index int) {
	m.state = stateEditBlock
	m.selected = index

	// Populate form values
	if index >= 0 && index < len(m.cfg.Scheduler.Blocks) {
		b := m.cfg.Scheduler.Blocks[index]
		m.formName = b.Name
		m.formCron = b.Cron
		m.formDuration = strconv.Itoa(b.Duration)
		m.formChannelID = b.ChannelID
	} else {
		m.formName = ""
		m.formCron = ""
		m.formDuration = ""
		m.formChannelID = ""
	}

	// Create fresh form with populated values
	m.createBlockForm()
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
	// Form validation is handled by huh, so values are already validated
	name := strings.TrimSpace(m.formName)
	cronExp := strings.TrimSpace(m.formCron)
	duration, _ := strconv.Atoi(strings.TrimSpace(m.formDuration))
	channelID := strings.TrimSpace(m.formChannelID)

	newBlock := scheduler.Block{
		Name:      name,
		Cron:      cronExp,
		Duration:  duration,
		ChannelID: channelID,
	}

	if m.selected == -1 {
		m.cfg.Scheduler.Blocks = append(m.cfg.Scheduler.Blocks, newBlock)
		m.list.InsertItem(len(m.list.Items()), item{block: newBlock})
	} else {
		// Preserve existing filter, series config, and other fields
		existingBlock := m.cfg.Scheduler.Blocks[m.selected]
		newBlock.Type = existingBlock.Type
		newBlock.Filter = existingBlock.Filter
		newBlock.Priority = existingBlock.Priority
		newBlock.Series = existingBlock.Series
		newBlock.Fallback = existingBlock.Fallback
		newBlock.Filler = existingBlock.Filler
		newBlock.MaxDurationOverflowMinutes = existingBlock.MaxDurationOverflowMinutes
		m.cfg.Scheduler.Blocks[m.selected] = newBlock
		// Update list item
		m.list.SetItem(m.selected, item{block: newBlock})
	}
	m.hasUnsavedChanges = true
}

// duplicateBlock creates a copy of the currently selected block.
func (m *Model) duplicateBlock() {
	if len(m.cfg.Scheduler.Blocks) == 0 {
		return
	}

	idx := m.list.Index()
	if idx < 0 || idx >= len(m.cfg.Scheduler.Blocks) {
		return
	}

	// Create a deep copy of the block
	original := m.cfg.Scheduler.Blocks[idx]
	duplicate := scheduler.Block{
		Type:                       original.Type,
		Name:                       original.Name + " (copy)",
		Cron:                       original.Cron,
		Duration:                   original.Duration,
		ChannelID:                  original.ChannelID,
		Priority:                   original.Priority,
		MaxDurationOverflowMinutes: original.MaxDurationOverflowMinutes,
		Filter:                     original.Filter,
		Filler:                     original.Filler,
		Fallback:                   original.Fallback,
	}

	// Deep copy series config
	if len(original.Series) > 0 {
		duplicate.Series = make([]scheduler.SeriesConfig, len(original.Series))
		copy(duplicate.Series, original.Series)
	}

	// Insert after the current block
	insertIdx := idx + 1
	m.cfg.Scheduler.Blocks = append(m.cfg.Scheduler.Blocks[:insertIdx],
		append([]scheduler.Block{duplicate}, m.cfg.Scheduler.Blocks[insertIdx:]...)...)
	m.list.InsertItem(insertIdx, item{block: duplicate})

	m.hasUnsavedChanges = true
	m.statusMessage = fmt.Sprintf("Duplicated '%s'", original.Name)
}

// saveConfigToDisk saves the current configuration to the scheduler file.
func (m *Model) saveConfigToDisk() {
	if err := config.SaveSchedulerConfig(&m.cfg.Scheduler, m.schedulerFile); err != nil {
		m.statusMessage = fmt.Sprintf("Error saving: %v", err)
		return
	}

	m.hasUnsavedChanges = false
	path := m.schedulerFile
	if path == "" {
		path = "scheduler.yaml"
	}
	m.statusMessage = "Saved to " + path
}

// adjustPriority changes the priority of the currently selected block.
func (m *Model) adjustPriority(delta int) {
	if len(m.cfg.Scheduler.Blocks) == 0 {
		return
	}

	idx := m.list.Index()
	if idx < 0 || idx >= len(m.cfg.Scheduler.Blocks) {
		return
	}

	// Adjust priority (minimum 0)
	newPriority := m.cfg.Scheduler.Blocks[idx].Priority + delta
	if newPriority < 0 {
		newPriority = 0
	}

	m.cfg.Scheduler.Blocks[idx].Priority = newPriority
	m.list.SetItem(idx, item{block: m.cfg.Scheduler.Blocks[idx]})

	m.hasUnsavedChanges = true
	m.statusMessage = fmt.Sprintf("Priority set to %d", newPriority)
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
		// Edit selected series state
		if len(m.seriesStates) > 0 && m.seriesScroll < len(m.seriesStates) {
			m.initSeriesEdit()
			m.state = stateSeriesEdit
		}
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
	case stateSeriesEdit:
		return m.renderSeriesEdit()
	case stateHelp:
		return m.renderHelp()
	default:
		return "Unknown state"
	}
}

func (m Model) renderListView() string {
	var builder strings.Builder
	builder.WriteString(m.list.View())

	// Show status message if any
	if m.statusMessage != "" {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
		builder.WriteString("\n\n")
		builder.WriteString(statusStyle.Render(m.statusMessage))
	}

	// Show unsaved changes indicator
	if m.hasUnsavedChanges {
		unsavedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		builder.WriteString("\n")
		builder.WriteString(unsavedStyle.Render("[unsaved changes - ctrl+s to save]"))
	}

	helpText := "\n\n'n' new | 'D' dup | 'd' del | enter edit | +/- pri | ctrl+s save | '?' help | 'q' quit"
	if m.store != nil {
		helpText = "\n\n'n' new | 'D' dup | 'd' del | enter edit | +/- pri | 's' series | ctrl+s save | '?' help | 'q' quit"
	}
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(helpText)
	builder.WriteString(help)

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

func (m Model) renderEditBlock() string {
	var builder strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	if m.selected == -1 {
		builder.WriteString(titleStyle.Render("New Block") + "\n\n")
	} else {
		builder.WriteString(titleStyle.Render("Edit Block") + "\n\n")
	}

	// Render the huh form
	builder.WriteString(m.blockForm.View())
	builder.WriteString("\n\n")
	builder.WriteString(helpStyle.Render("ctrl+b: cron builder | ctrl+f: filter builder | ?: help"))

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
	builder.WriteString(helpStyle.Render("↑/↓, j/k: Navigate | e: Edit | r: Refresh | esc/q: Back | ?: Help"))

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
	// Parse existing cron expression from the form field
	m.cronExpr = cronbuilder.Parse(m.formCron)
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
		// Apply the built cron expression to form field and recreate form
		m.formCron = m.cronExpr.String()
		m.createBlockForm()
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
		builder.WriteString(keyStyle.Render("  e") + descStyle.Render("          Edit selected series position\n"))
		builder.WriteString(keyStyle.Render("  r") + descStyle.Render("          Refresh series states from database\n"))
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

	case stateSeriesEdit:
		builder.WriteString(titleStyle.Render("Edit Series Progress") + "\n\n")
		builder.WriteString(keyStyle.Render("  ↑/↓, tab") + descStyle.Render("  Navigate between fields\n"))
		builder.WriteString(keyStyle.Render("  0-9") + descStyle.Render("        Enter season/episode numbers\n"))
		builder.WriteString(keyStyle.Render("  backspace") + descStyle.Render("  Delete last character\n"))
		builder.WriteString(keyStyle.Render("  enter") + descStyle.Render("      Save changes (when on Save button)\n"))
		builder.WriteString(keyStyle.Render("  esc, q") + descStyle.Render("     Cancel and return to series list\n\n"))
		builder.WriteString(descStyle.Render("Edit the current episode position for a series.\n"))
		builder.WriteString(descStyle.Render("This resets the series state, clearing completion and disabled flags.\n"))

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
// Series Edit Interface
// ============================================================================

// initSeriesEdit initializes the series edit state with current values
func (m *Model) initSeriesEdit() {
	if m.seriesScroll >= len(m.seriesStates) {
		return
	}

	state := m.seriesStates[m.seriesScroll]
	m.seriesEditShowTitle = state.ShowTitle
	m.seriesEditSeason = strconv.Itoa(state.CurrentSeason)
	m.seriesEditEpisode = strconv.Itoa(state.CurrentEpisode)
	m.seriesEditField = 0
}

// updateSeriesEdit handles key events in the series edit state
func (m Model) updateSeriesEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = stateSeriesProgress
		return m, nil
	case "enter":
		if m.seriesEditField == 2 { // Save button
			m.saveSeriesEdit()
			m.loadSeriesStates() // Refresh states
			m.state = stateSeriesProgress
		}
		return m, nil
	case "tab", "down", "j":
		m.seriesEditField = (m.seriesEditField + 1) % 3
		return m, nil
	case "shift+tab", "up", "k":
		m.seriesEditField = (m.seriesEditField - 1 + 3) % 3
		return m, nil
	case "backspace":
		m.handleSeriesEditBackspace()
		return m, nil
	default:
		m.handleSeriesEditInput(msg.String())
		return m, nil
	}
}

// handleSeriesEditBackspace handles backspace in series edit fields
func (m *Model) handleSeriesEditBackspace() {
	switch m.seriesEditField {
	case 0: // Season
		if len(m.seriesEditSeason) > 0 {
			m.seriesEditSeason = m.seriesEditSeason[:len(m.seriesEditSeason)-1]
		}
	case 1: // Episode
		if len(m.seriesEditEpisode) > 0 {
			m.seriesEditEpisode = m.seriesEditEpisode[:len(m.seriesEditEpisode)-1]
		}
	}
}

// handleSeriesEditInput handles numeric input in series edit fields
func (m *Model) handleSeriesEditInput(input string) {
	if len(input) != 1 || input < "0" || input > "9" {
		return
	}

	switch m.seriesEditField {
	case 0: // Season
		if len(m.seriesEditSeason) < 3 {
			m.seriesEditSeason += input
		}
	case 1: // Episode
		if len(m.seriesEditEpisode) < 4 {
			m.seriesEditEpisode += input
		}
	}
}

// saveSeriesEdit saves the edited series state to the store
func (m *Model) saveSeriesEdit() {
	if m.store == nil {
		m.statusMessage = "No database connected"
		return
	}

	season, err := strconv.Atoi(m.seriesEditSeason)
	if err != nil || season < 1 {
		m.statusMessage = "Invalid season number"
		return
	}

	episode, err := strconv.Atoi(m.seriesEditEpisode)
	if err != nil || episode < 1 {
		m.statusMessage = "Invalid episode number"
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.store.SetSeriesState(ctx, m.seriesEditShowTitle, season, episode); err != nil {
		m.statusMessage = fmt.Sprintf("Error saving: %v", err)
		return
	}

	m.statusMessage = fmt.Sprintf("Updated %s to S%02dE%02d", m.seriesEditShowTitle, season, episode)
}

// renderSeriesEdit renders the series edit view
func (m Model) renderSeriesEdit() string {
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

	builder.WriteString(titleStyle.Render("Edit Series Progress") + "\n\n")
	builder.WriteString(labelStyle.Render("Series: "))
	builder.WriteString(valueStyle.Render(m.seriesEditShowTitle) + "\n\n")

	// Season field
	seasonStyle := inactiveStyle
	if m.seriesEditField == 0 {
		seasonStyle = activeStyle
	}
	seasonValue := m.seriesEditSeason
	if seasonValue == "" {
		seasonValue = "_"
	}
	builder.WriteString(seasonStyle.Render("  Season:  "+seasonValue) + "\n")

	// Episode field
	episodeStyle := inactiveStyle
	if m.seriesEditField == 1 {
		episodeStyle = activeStyle
	}
	episodeValue := m.seriesEditEpisode
	if episodeValue == "" {
		episodeValue = "_"
	}
	builder.WriteString(episodeStyle.Render("  Episode: "+episodeValue) + "\n\n")

	// Save button
	saveStyle := inactiveStyle
	if m.seriesEditField == 2 {
		saveStyle = activeStyle
	}
	builder.WriteString(saveStyle.Render("  [ Save Changes ]") + "\n\n")

	// Status message
	if m.statusMessage != "" {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		builder.WriteString(statusStyle.Render(m.statusMessage) + "\n\n")
	}

	builder.WriteString(helpStyle.Render("↑/↓, tab: Navigate | 0-9: Enter numbers | backspace: Delete | enter: Save | esc/q: Cancel"))

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
