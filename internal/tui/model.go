package tui

import (
	"fmt"

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

func (i item) Title() string       { return i.block.Name }
func (i item) Description() string { return fmt.Sprintf("%s | %d min | %s", i.block.Cron, i.block.Duration, i.block.ChannelID) }
func (i item) FilterValue() string { return i.block.Name }

type Model struct {
	cfg        *config.Config
	list       list.Model
	inputs     []textinput.Model
	focusIndex int
	state      sessionState
	selected   int // index of block being edited
}

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

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := lipgloss.NewStyle().Margin(1, 2).GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		switch m.state {
		case stateListView:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
						case "enter":
							if len(m.cfg.Scheduler.Blocks) == 0 {
								return m, nil
							}
							m.state = stateEditBlock
							m.selected = m.list.Index()
							
							// Load selected block into inputs
							b := m.cfg.Scheduler.Blocks[m.selected]
							m.inputs[0].SetValue(b.Name)
							m.inputs[1].SetValue(b.Cron)
							m.inputs[2].SetValue(fmt.Sprintf("%d", b.Duration))
							m.inputs[3].SetValue(b.ChannelID)
							
							m.resetInputs()
							return m, nil
						case "n": // New block
							m.state = stateEditBlock
							m.selected = -1 // New
							for i := range m.inputs {
								m.inputs[i].SetValue("")
							}
							m.resetInputs()
							return m, nil
			
			}
			m.list, cmd = m.list.Update(msg)
			return m, cmd

		case stateEditBlock:
			switch msg.String() {
			case "esc":
				m.state = stateListView
				return m, nil
			case "tab", "shift+tab", "enter", "up", "down":
				s := msg.String()

				if s == "enter" && m.focusIndex == len(m.inputs) {
					// Save
					m.saveBlock()
					m.state = stateListView
					return m, nil
				}

				if s == "up" || s == "shift+tab" {
					m.focusIndex--
				} else {
					m.focusIndex++
				}

				if m.focusIndex > len(m.inputs) {
					m.focusIndex = 0
				} else if m.focusIndex < 0 {
					m.focusIndex = len(m.inputs)
				}

				cmds = make([]tea.Cmd, len(m.inputs))
				for i := 0; i <= len(m.inputs)-1; i++ {
					if i == m.focusIndex {
						// Set focused state
						cmds[i] = m.inputs[i].Focus()
						m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
						m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
						continue
					}
					// Remove focused state
					m.inputs[i].Blur()
					m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
					m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
				}
				return m, tea.Batch(cmds...)
			}
			
			// Only update the focused input
			if m.focusIndex < len(m.inputs) {
				m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
				return m, cmd
			}
		}
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
	// Simple parsing, assuming valid inputs for MVP
	// In real app, valid integers, cron, etc.
	
	name := m.inputs[0].Value()
	cronExp := m.inputs[1].Value()
	duration := 0 // todo parse int
	fmt.Sscanf(m.inputs[2].Value(), "%d", &duration)
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

func (m Model) View() string {
	if m.state == stateListView {
		return lipgloss.NewStyle().Margin(1, 2).Render(m.list.View())
	}

	if m.state == stateEditBlock {
		var s string
		s += "Edit Block\n\n"

		for i := range m.inputs {
			s += m.inputs[i].View() + "\n"
		}

		button := "[ Save ]"
		if m.focusIndex == len(m.inputs) {
			button = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("[ Save ]")
		}
		s += "\n" + button + "\n\n(esc to cancel)"

		return lipgloss.NewStyle().Margin(1, 2).Render(s)
	}

	return "Unknown state"
}
