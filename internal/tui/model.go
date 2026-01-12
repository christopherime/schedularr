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
)

type sessionState int

const (
	stateListView sessionState = iota
	stateEditBlock
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
	cfg        *config.Config
	list       list.Model
	inputs     []textinput.Model
	focusIndex int
	state      sessionState
	selected   int // index of block being edited
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
		cfg:    cfg,
		list:   l,
		inputs: inputs,
		state:  stateListView,
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
		return m, cmd
	}
	return m, nil
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

func (m *Model) saveBlock() {
	name := m.inputs[0].Value()
	cronExp := m.inputs[1].Value()
	duration, _ := strconv.Atoi(m.inputs[2].Value())
	channelID := m.inputs[3].Value()

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
		return lipgloss.NewStyle().Margin(1, 2).Render(m.list.View())
	}

	if m.state == stateEditBlock {
		var builder strings.Builder
		builder.WriteString("Edit Block\n\n")

		for i := range m.inputs {
			builder.WriteString(m.inputs[i].View())
			builder.WriteString("\n")
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

	return "Unknown state"
}
