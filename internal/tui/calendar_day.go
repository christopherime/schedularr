package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/geekxflood/schedularr/internal/schedule"
)

// Calendar day view styles
var (
	calDayTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				Padding(0, 1)

	calDayTimeStyle = lipgloss.NewStyle().
			Width(6).
			Foreground(lipgloss.Color("245"))

	calDayTimelineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	calDaySlotStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	calDaySlotSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("255"))

	calDaySlotPastStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Faint(true)

	calDayEmptySlotStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	calDayHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(1, 0)

	calDayInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
)

// updateCalendarDay handles key events in calendar day view.
func (m Model) updateCalendarDay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "left", "h":
		// Previous day
		m.calendarDate = m.calendarDate.AddDate(0, 0, -1)
		m.calendarSelection.SlotIndex = 0
		return m, nil

	case "right", "l":
		// Next day
		m.calendarDate = m.calendarDate.AddDate(0, 0, 1)
		m.calendarSelection.SlotIndex = 0
		return m, nil

	case "up", "k":
		// Move selection up
		if m.calendarSelection.SlotIndex > 0 {
			m.calendarSelection.SlotIndex--
		}
		return m, nil

	case "down", "j":
		// Move selection down
		slots := m.getDaySlotsForView()
		if m.calendarSelection.SlotIndex < len(slots)-1 {
			m.calendarSelection.SlotIndex++
		}
		return m, nil

	case "enter":
		// Edit selected slot (only if future)
		slots := m.getDaySlotsForView()
		if m.calendarSelection.SlotIndex < len(slots) {
			slot := slots[m.calendarSelection.SlotIndex]
			now := time.Now().In(m.timezone)
			if slot.IsFuture(now) {
				m.editingSlot = &slot
				m.state = stateCalendarEdit
				return m, nil
			}
			m.statusMessage = "Cannot edit past programs"
		}
		return m, nil

	case "n":
		// Add new slot
		now := time.Now().In(m.timezone)
		dayDate := m.calendarDate
		// Only allow adding to future days or today
		if dayDate.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, m.timezone)) {
			m.statusMessage = "Cannot add programs to past days"
			return m, nil
		}
		m.editingSlot = &schedule.ProgramSlot{
			StartTime: time.Date(dayDate.Year(), dayDate.Month(), dayDate.Day(), 12, 0, 0, 0, m.timezone),
			Duration:  30 * time.Minute,
			Type:      schedule.ProgramTypeMovie,
		}
		m.state = stateCalendarEdit
		return m, nil

	case "d":
		// Delete selected slot (only if future)
		slots := m.getDaySlotsForView()
		if m.calendarSelection.SlotIndex < len(slots) {
			slot := slots[m.calendarSelection.SlotIndex]
			now := time.Now().In(m.timezone)
			if slot.IsFuture(now) {
				// Delete the slot from the week schedule
				m.deleteSlotFromWeek(slot)
				m.statusMessage = "Slot deleted"
			} else {
				m.statusMessage = "Cannot delete past programs"
			}
		}
		return m, nil

	case "w":
		// Return to week view
		m.state = stateCalendarWeek
		m.calendarViewMode = CalendarViewWeek
		return m, nil

	case "m":
		// Return to month view
		m.state = stateCalendarMonth
		m.calendarViewMode = CalendarViewMonth
		return m, nil

	case "g":
		// Generate schedule from blocks for this day
		m.statusMessage = "Generating from blocks..."
		return m, nil

	case "?":
		// Show help
		m.prevState = m.state
		m.state = stateHelp
		return m, nil
	}

	return m, nil
}

// renderCalendarDay renders the calendar day timeline view.
func (m Model) renderCalendarDay() string {
	var b strings.Builder

	// Title
	title := m.calendarDate.Format("Monday, January 2, 2006")
	b.WriteString(calDayTitleStyle.Render(title))
	b.WriteString("\n\n")

	// Get slots for the day
	slots := m.getDaySlotsForView()
	now := time.Now().In(m.timezone)
	isToday := m.calendarDate.Format("2006-01-02") == now.Format("2006-01-02")

	// Render 24-hour timeline
	maxWidth := 50
	if m.windowWidth > 70 {
		maxWidth = m.windowWidth - 20
	}

	// Create hour markers
	for hour := 0; hour < 24; hour++ {
		hourTime := time.Date(m.calendarDate.Year(), m.calendarDate.Month(), m.calendarDate.Day(),
			hour, 0, 0, 0, m.timezone)

		// Time label
		timeLabel := calDayTimeStyle.Render(fmt.Sprintf("%02d:00", hour))
		b.WriteString(timeLabel)

		// Find slots that start or span this hour
		var hourSlots []schedule.ProgramSlot
		for _, slot := range slots {
			slotStart := slot.StartTime
			slotEnd := slot.EndTime()
			hourStart := hourTime
			hourEnd := hourTime.Add(time.Hour)

			// Check if slot overlaps with this hour
			if slotStart.Before(hourEnd) && slotEnd.After(hourStart) {
				hourSlots = append(hourSlots, slot)
			}
		}

		// Render timeline bar
		b.WriteString(" ")
		if len(hourSlots) == 0 {
			// Empty hour
			emptyBar := strings.Repeat("-", maxWidth)
			b.WriteString(calDayEmptySlotStyle.Render(emptyBar))
		} else {
			// Show slot info
			for i, slot := range hourSlots {
				if i > 0 {
					b.WriteString("\n")
					b.WriteString(strings.Repeat(" ", 7)) // Align with time column
				}

				// Determine if this slot is selected
				isSelected := false
				for idx, s := range slots {
					if s.ProgramID == slot.ProgramID && s.StartTime.Equal(slot.StartTime) {
						isSelected = idx == m.calendarSelection.SlotIndex
						break
					}
				}

				// Determine style based on past/future and selection
				isPast := slot.IsPast(now)
				isOngoing := slot.IsOngoing(now)

				// Build slot display
				durationMin := int(slot.Duration.Minutes())
				slotInfo := fmt.Sprintf("%s (%dm)", truncateString(slot.Title, 30), durationMin)

				var slotStyle lipgloss.Style
				switch {
				case isSelected:
					slotStyle = calDaySlotSelectedStyle
				case isPast:
					slotStyle = calDaySlotPastStyle
				case isOngoing && isToday:
					slotStyle = calDaySlotStyle.Bold(true)
				default:
					slotStyle = calDaySlotStyle
				}

				// Visual indicator
				indicator := " "
				if slot.StartTime.Hour() == hour && slot.StartTime.Minute() < 30 {
					indicator = ">"
				}

				b.WriteString(indicator)
				b.WriteString(slotStyle.Render(slotInfo))
			}
		}

		b.WriteString("\n")
	}

	// Summary
	b.WriteString("\n")
	totalSlots := len(slots)
	futureSlots := 0
	pastSlots := 0
	for _, slot := range slots {
		if slot.IsPast(now) {
			pastSlots++
		} else {
			futureSlots++
		}
	}
	summary := fmt.Sprintf("Total: %d programs (%d past, %d future/editable)", totalSlots, pastSlots, futureSlots)
	b.WriteString(calDayInfoStyle.Render(summary))

	// Status message
	if m.statusMessage != "" {
		b.WriteString("\n")
		b.WriteString(calMonthStatusStyle.Render(m.statusMessage))
	}

	// Help
	b.WriteString("\n\n")
	help := "[hl] prev/next day | [jk] select slot | [enter] edit | [n] new | [d] delete | [w] week | [m] month | [?] help"
	b.WriteString(calDayHelpStyle.Render(help))

	return b.String()
}

// getDaySlotsForView returns all program slots for the current day, sorted by start time.
func (m *Model) getDaySlotsForView() []schedule.ProgramSlot {
	if m.scheduleManager == nil {
		return nil
	}

	weekID := schedule.WeekID(m.calendarDate)
	weekSchedule, err := m.scheduleManager.GetWeek(weekID)
	if err != nil {
		m.logger.Debug("failed to load week schedule", "week_id", weekID, "error", err)
		return nil
	}

	daySlots := weekSchedule.GetSlotsForDay(m.calendarDate)

	// Flatten all channel slots into a single sorted list
	var allSlots []schedule.ProgramSlot
	for _, slots := range daySlots {
		allSlots = append(allSlots, slots...)
	}

	// Sort by start time
	sort.Slice(allSlots, func(i, j int) bool {
		return allSlots[i].StartTime.Before(allSlots[j].StartTime)
	})

	return allSlots
}

// deleteSlotFromWeek removes a slot from the week schedule.
func (m *Model) deleteSlotFromWeek(slot schedule.ProgramSlot) {
	if m.scheduleManager == nil {
		return
	}

	weekID := schedule.WeekID(slot.StartTime)
	weekSchedule, err := m.scheduleManager.GetWeek(weekID)
	if err != nil {
		m.logger.Error("failed to load week schedule for deletion", "week_id", weekID, "error", err)
		return
	}

	// Find and remove the slot from the channel
	channelSlots := weekSchedule.ChannelSlots[slot.ChannelID]
	var newSlots []schedule.ProgramSlot
	for _, s := range channelSlots {
		if s.ProgramID != slot.ProgramID || !s.StartTime.Equal(slot.StartTime) {
			newSlots = append(newSlots, s)
		}
	}
	weekSchedule.ChannelSlots[slot.ChannelID] = newSlots

	// Save the updated schedule
	if err := m.scheduleManager.SaveWeek(weekSchedule); err != nil {
		m.logger.Error("failed to save week schedule after deletion", "week_id", weekID, "error", err)
		m.statusMessage = "Failed to save: " + err.Error()
	}
}

// truncateString truncates a string to maxLen, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
