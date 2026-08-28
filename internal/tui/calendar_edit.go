package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/christopherime/schedularr/internal/schedule"
)

// Edit modal field indices
const (
	editFieldTitle = iota
	editFieldStartHour
	editFieldStartMin
	editFieldDurationHour
	editFieldDurationMin
	editFieldType
	editFieldSave
	editFieldCancel
	editFieldDelete
	editFieldCount
)

// Calendar edit modal styles
var (
	calEditModalStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205")).
				Padding(1, 2).
				Width(40)

	calEditTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				Align(lipgloss.Center).
				Width(36)

	calEditLabelStyle = lipgloss.NewStyle().
				Width(12).
				Foreground(lipgloss.Color("245"))

	calEditInputStyle = lipgloss.NewStyle().
				Width(20).
				Foreground(lipgloss.Color("255"))

	calEditInputSelectedStyle = lipgloss.NewStyle().
					Width(20).
					Background(lipgloss.Color("62")).
					Foreground(lipgloss.Color("255"))

	calEditButtonStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Foreground(lipgloss.Color("245"))

	calEditButtonSelectedStyle = lipgloss.NewStyle().
					Padding(0, 2).
					Background(lipgloss.Color("62")).
					Foreground(lipgloss.Color("255")).
					Bold(true)

	calEditDeleteButtonStyle = lipgloss.NewStyle().
					Padding(0, 2).
					Foreground(lipgloss.Color("196"))

	calEditDeleteButtonSelectedStyle = lipgloss.NewStyle().
						Padding(0, 2).
						Background(lipgloss.Color("196")).
						Foreground(lipgloss.Color("255")).
						Bold(true)

	calEditErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	calEditHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Padding(1, 0)
)

// editFormState tracks the edit form state
type editFormState struct {
	title        string
	startHour    string
	startMin     string
	durationHour string
	durationMin  string
	programType  int // index into program types
	fieldIndex   int
	isNew        bool
	errorMsg     string
}

var programTypes = []string{"movie", "episode", "flex", "filler"}

// updateCalendarEdit handles key events in calendar edit modal.
func (m Model) updateCalendarEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Initialize form state if not set
	if m.editingSlot == nil {
		m.state = stateCalendarDay
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c", "esc":
		// Cancel editing
		m.editingSlot = nil
		m.state = stateCalendarDay
		return m, nil

	case "tab", "down", "j":
		// Move to next field
		m.calendarSelection.SlotIndex = (m.calendarSelection.SlotIndex + 1) % editFieldCount
		return m, nil

	case "shift+tab", "up", "k":
		// Move to previous field
		m.calendarSelection.SlotIndex = (m.calendarSelection.SlotIndex - 1 + editFieldCount) % editFieldCount
		return m, nil

	case "enter":
		// Execute action based on current field
		return m.handleEditFieldAction()

	case "left", "h":
		// For type field, cycle backward
		if m.calendarSelection.SlotIndex == editFieldType {
			currentType := getTypeIndex(m.editingSlot.Type)
			newType := (currentType - 1 + len(programTypes)) % len(programTypes)
			m.editingSlot.Type = schedule.ProgramType(programTypes[newType])
		}
		return m, nil

	case "right", "l":
		// For type field, cycle forward
		if m.calendarSelection.SlotIndex == editFieldType {
			currentType := getTypeIndex(m.editingSlot.Type)
			newType := (currentType + 1) % len(programTypes)
			m.editingSlot.Type = schedule.ProgramType(programTypes[newType])
		}
		return m, nil

	case "backspace":
		// Handle backspace for text fields
		m.handleEditBackspace()
		return m, nil

	default:
		// Handle character input for text fields
		m.handleEditCharInput(msg.String())
		return m, nil
	}
}

// handleEditFieldAction handles enter key on a field.
func (m Model) handleEditFieldAction() (tea.Model, tea.Cmd) {
	switch m.calendarSelection.SlotIndex {
	case editFieldSave:
		// Validate and save
		if err := m.validateAndSaveSlot(); err != nil {
			m.statusMessage = err.Error()
			return m, nil
		}
		m.statusMessage = "Slot saved"
		m.editingSlot = nil
		m.state = stateCalendarDay
		return m, nil

	case editFieldCancel:
		// Cancel
		m.editingSlot = nil
		m.state = stateCalendarDay
		return m, nil

	case editFieldDelete:
		// Delete the slot
		if m.editingSlot.ProgramID != "" {
			m.deleteSlotFromWeek(*m.editingSlot)
			m.statusMessage = "Slot deleted"
		}
		m.editingSlot = nil
		m.state = stateCalendarDay
		return m, nil
	}

	// Move to next field
	m.calendarSelection.SlotIndex = (m.calendarSelection.SlotIndex + 1) % editFieldCount
	return m, nil
}

// handleEditBackspace handles backspace in text fields.
func (m *Model) handleEditBackspace() {
	if m.calendarSelection.SlotIndex == editFieldTitle && len(m.editingSlot.Title) > 0 {
		m.editingSlot.Title = m.editingSlot.Title[:len(m.editingSlot.Title)-1]
	}
}

// handleEditCharInput handles character input in text fields.
func (m *Model) handleEditCharInput(char string) {
	// Only handle single printable characters
	if len(char) != 1 || char[0] < 32 || char[0] > 126 {
		return
	}

	switch m.calendarSelection.SlotIndex {
	case editFieldTitle:
		if len(m.editingSlot.Title) < 50 {
			m.editingSlot.Title += char
		}
	case editFieldStartHour:
		if char >= "0" && char <= "9" {
			m.editingSlot.StartTime = adjustHour(m.editingSlot.StartTime, char)
		}
	case editFieldStartMin:
		if char >= "0" && char <= "9" {
			m.editingSlot.StartTime = adjustMinute(m.editingSlot.StartTime, char)
		}
	case editFieldDurationHour:
		if char >= "0" && char <= "9" {
			m.editingSlot.Duration = adjustDurationHour(m.editingSlot.Duration, char)
		}
	case editFieldDurationMin:
		if char >= "0" && char <= "9" {
			m.editingSlot.Duration = adjustDurationMin(m.editingSlot.Duration, char)
		}
	}
}

// adjustHour adjusts the hour based on input.
func adjustHour(t time.Time, char string) time.Time {
	current := t.Hour()
	digit, _ := strconv.Atoi(char)

	// Shift left and add new digit
	newHour := (current%10)*10 + digit
	if newHour > 23 {
		newHour = digit
	}

	return time.Date(t.Year(), t.Month(), t.Day(), newHour, t.Minute(), 0, 0, t.Location())
}

// adjustMinute adjusts the minute based on input.
func adjustMinute(t time.Time, char string) time.Time {
	current := t.Minute()
	digit, _ := strconv.Atoi(char)

	// Shift left and add new digit
	newMin := (current%10)*10 + digit
	if newMin > 59 {
		newMin = digit
	}

	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), newMin, 0, 0, t.Location())
}

// adjustDurationHour adjusts the duration hour component.
func adjustDurationHour(d time.Duration, char string) time.Duration {
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	digit, _ := strconv.Atoi(char)

	// Shift left and add new digit
	newHours := (hours%10)*10 + digit
	if newHours > 24 {
		newHours = digit
	}

	return time.Duration(newHours)*time.Hour + time.Duration(mins)*time.Minute
}

// adjustDurationMin adjusts the duration minute component.
func adjustDurationMin(d time.Duration, char string) time.Duration {
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	digit, _ := strconv.Atoi(char)

	// Shift left and add new digit
	newMins := (mins%10)*10 + digit
	if newMins > 59 {
		newMins = digit
	}

	return time.Duration(hours)*time.Hour + time.Duration(newMins)*time.Minute
}

// getTypeIndex returns the index of a program type.
func getTypeIndex(t schedule.ProgramType) int {
	for i, pt := range programTypes {
		if string(t) == pt {
			return i
		}
	}
	return 0
}

// validateAndSaveSlot validates the slot and saves it to the week schedule.
func (m *Model) validateAndSaveSlot() error {
	if m.editingSlot == nil {
		return errors.New("no slot to save")
	}

	// Validate title
	if strings.TrimSpace(m.editingSlot.Title) == "" {
		return errors.New("title is required")
	}

	// Validate duration
	if m.editingSlot.Duration < time.Minute {
		return errors.New("duration must be at least 1 minute")
	}

	// Validate start time is in the future
	now := time.Now().In(m.timezone)
	if m.editingSlot.StartTime.Before(now) {
		return errors.New("start time must be in the future")
	}

	// Get or create week schedule
	if m.scheduleManager == nil {
		return errors.New("schedule manager not initialized")
	}

	weekID := schedule.WeekID(m.editingSlot.StartTime)
	weekSchedule, err := m.scheduleManager.GetWeek(weekID)
	if err != nil {
		return fmt.Errorf("failed to load week: %w", err)
	}

	// Set channel ID if not set
	if m.editingSlot.ChannelID == "" {
		// Use first channel or default
		if len(m.channels) > 0 {
			m.editingSlot.ChannelID = m.channels[0].Channel.ID
		} else {
			m.editingSlot.ChannelID = "default"
		}
	}

	// Generate program ID if new
	if m.editingSlot.ProgramID == "" {
		m.editingSlot.ProgramID = fmt.Sprintf("manual-%d", time.Now().UnixNano())
	}

	// Mark as manually edited
	m.editingSlot.ManualEdit = true

	// Add or update the slot
	weekSchedule.AddSlot(m.editingSlot.ChannelID, *m.editingSlot)

	// Save the week schedule
	if err := m.scheduleManager.SaveWeek(weekSchedule); err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

// renderCalendarEdit renders the edit modal.
func (m Model) renderCalendarEdit() string {
	if m.editingSlot == nil {
		return "No slot being edited"
	}

	var b strings.Builder

	// Modal title
	modalTitle := "Edit Program Slot"
	if m.editingSlot.ProgramID == "" {
		modalTitle = "New Program Slot"
	}
	b.WriteString(calEditTitleStyle.Render(modalTitle))
	b.WriteString("\n\n")

	// Title field
	b.WriteString(m.renderEditField(editFieldTitle, "Title:", m.editingSlot.Title))
	b.WriteString("\n")

	// Start time fields
	startHour := fmt.Sprintf("%02d", m.editingSlot.StartTime.Hour())
	startMin := fmt.Sprintf("%02d", m.editingSlot.StartTime.Minute())
	b.WriteString(m.renderEditField(editFieldStartHour, "Start Hour:", startHour))
	b.WriteString(m.renderEditField(editFieldStartMin, "Start Min:", startMin))
	b.WriteString("\n")

	// Duration fields
	durationHours := int(m.editingSlot.Duration.Hours())
	durationMins := int(m.editingSlot.Duration.Minutes()) % 60
	b.WriteString(m.renderEditField(editFieldDurationHour, "Dur Hours:", fmt.Sprintf("%02d", durationHours)))
	b.WriteString(m.renderEditField(editFieldDurationMin, "Dur Mins:", fmt.Sprintf("%02d", durationMins)))
	b.WriteString("\n")

	// Type field
	typeStr := fmt.Sprintf("< %s >", m.editingSlot.Type)
	b.WriteString(m.renderEditField(editFieldType, "Type:", typeStr))
	b.WriteString("\n\n")

	// Buttons
	var buttons []string

	// Save button
	if m.calendarSelection.SlotIndex == editFieldSave {
		buttons = append(buttons, calEditButtonSelectedStyle.Render("[Save]"))
	} else {
		buttons = append(buttons, calEditButtonStyle.Render("[Save]"))
	}

	// Cancel button
	if m.calendarSelection.SlotIndex == editFieldCancel {
		buttons = append(buttons, calEditButtonSelectedStyle.Render("[Cancel]"))
	} else {
		buttons = append(buttons, calEditButtonStyle.Render("[Cancel]"))
	}

	// Delete button (only for existing slots)
	if m.editingSlot.ProgramID != "" {
		if m.calendarSelection.SlotIndex == editFieldDelete {
			buttons = append(buttons, calEditDeleteButtonSelectedStyle.Render("[Delete]"))
		} else {
			buttons = append(buttons, calEditDeleteButtonStyle.Render("[Delete]"))
		}
	}

	b.WriteString(strings.Join(buttons, "  "))

	// Error message
	if m.statusMessage != "" {
		b.WriteString("\n\n")
		b.WriteString(calEditErrorStyle.Render(m.statusMessage))
	}

	// Help
	b.WriteString("\n\n")
	help := "[tab/jk] navigate | [enter] confirm | [hl] type | [esc] cancel"
	b.WriteString(calEditHelpStyle.Render(help))

	// Wrap in modal box
	return calEditModalStyle.Render(b.String())
}

// renderEditField renders a single edit field.
func (m Model) renderEditField(fieldIndex int, label, value string) string {
	labelStr := calEditLabelStyle.Render(label)

	var valueStr string
	if m.calendarSelection.SlotIndex == fieldIndex {
		valueStr = calEditInputSelectedStyle.Render(value)
	} else {
		valueStr = calEditInputStyle.Render(value)
	}

	return labelStr + valueStr + "\n"
}
