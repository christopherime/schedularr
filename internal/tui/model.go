// Package tui provides a terminal user interface for managing Schedularr configuration.
package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/robfig/cron/v3"
)

type sessionState int

const (
	stateListView sessionState = iota
	stateEditBlock
	stateConfirmDelete
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
	list           list.Model
	inputs         []textinput.Model
	validationErrs []string // validation errors for each input field
	focusIndex     int
	state          sessionState
	selected       int // index of block being edited
}

// NewModel creates a new TUI model with the given configuration.
func NewModel(cfg *config.Config) Model {
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
	switch m.state {
	case stateListView:
		return m.updateListView(msg)
	case stateEditBlock:
		return m.updateEditBlock(msg)
	case stateConfirmDelete:
		return m.updateConfirmDelete(msg)
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

// View renders the TUI.
func (m Model) View() string {
	if m.state == stateListView {
		help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
			"\n\nPress 'n' to create new block, 'd' to delete, 'enter' to edit, 'q' to quit",
		)
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
		builder.WriteString("\n\n(esc to cancel)")

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

	return "Unknown state"
}
