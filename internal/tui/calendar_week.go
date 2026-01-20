package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/geekxflood/schedularr/internal/schedule"
)

// Calendar week view styles
var (
	calWeekTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				Padding(0, 1)

	calWeekDayHeaderStyle = lipgloss.NewStyle().
				Width(12).
				Align(lipgloss.Center).
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("62"))

	calWeekDaySelectedStyle = lipgloss.NewStyle().
				Width(12).
				Align(lipgloss.Center).
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("205"))

	calWeekDayBoxStyle = lipgloss.NewStyle().
				Width(12).
				Height(5).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)

	calWeekDayBoxSelectedStyle = lipgloss.NewStyle().
					Width(12).
					Height(5).
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("205")).
					Padding(0, 1)

	calWeekFillBarEmpty = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	calWeekFillBarFilled = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	calWeekHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Padding(1, 0)

	calWeekInfoStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))
)

// updateCalendarWeek handles key events in calendar week view.
func (m Model) updateCalendarWeek(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "left", "h":
		// Move to previous day
		m.calendarDate = m.calendarDate.AddDate(0, 0, -1)
		return m, nil

	case "right", "l":
		// Move to next day
		m.calendarDate = m.calendarDate.AddDate(0, 0, 1)
		return m, nil

	case "[":
		// Previous week
		m.calendarDate = m.calendarDate.AddDate(0, 0, -7)
		return m, nil

	case "]":
		// Next week
		m.calendarDate = m.calendarDate.AddDate(0, 0, 7)
		return m, nil

	case "t":
		// Jump to today
		m.calendarDate = time.Now().In(m.timezone)
		return m, nil

	case "enter":
		// Go to day view for selected date
		m.state = stateCalendarDay
		m.calendarViewMode = CalendarViewDay
		m.calendarSelection.SlotIndex = 0
		return m, nil

	case "m":
		// Return to month view
		m.state = stateCalendarMonth
		m.calendarViewMode = CalendarViewMonth
		return m, nil

	case "g":
		// Generate schedule from blocks for this week
		return m, m.generateWeekFromBlocksCmd()

	case "b":
		// Go to blocks view
		m.state = stateListView
		return m, nil

	case "?":
		// Show help
		m.prevState = m.state
		m.state = stateHelp
		return m, nil
	}

	return m, nil
}

// renderCalendarWeek renders the calendar week view.
func (m Model) renderCalendarWeek() string {
	var b strings.Builder

	// Get the week containing the current date
	weekStart, weekEnd := schedule.WeekBounds(m.calendarDate, m.timezone)
	weekID := schedule.WeekID(m.calendarDate)

	// Title
	title := fmt.Sprintf("Week %s: %s - %s",
		weekID[5:], // Extract WNN part
		weekStart.Format("Jan 2"),
		weekEnd.Format("Jan 2, 2006"))
	b.WriteString(calWeekTitleStyle.Render(title))
	b.WriteString("\n\n")

	// Load week schedule
	var weekSchedule *schedule.WeekSchedule
	if m.scheduleManager != nil {
		var err error
		weekSchedule, err = m.scheduleManager.GetWeek(weekID)
		if err != nil {
			m.logger.Debug("failed to load week schedule", "week_id", weekID, "error", err)
		}
	}

	// Get today for highlighting
	today := time.Now().In(m.timezone)
	todayDateStr := today.Format("2006-01-02")

	// Build day columns
	days := make([]string, 7)
	dayHeaders := make([]string, 7)
	dayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

	for i := 0; i < 7; i++ {
		dayDate := weekStart.AddDate(0, 0, i)
		dayDateStr := dayDate.Format("2006-01-02")
		isSelected := dayDate.Format("2006-01-02") == m.calendarDate.Format("2006-01-02")
		isToday := dayDateStr == todayDateStr

		// Header (day name + date)
		headerText := fmt.Sprintf("%s %d", dayNames[i], dayDate.Day())
		if isSelected {
			dayHeaders[i] = calWeekDaySelectedStyle.Render(headerText)
		} else if isToday {
			todayStyle := calWeekDayHeaderStyle.Background(lipgloss.Color("214"))
			dayHeaders[i] = todayStyle.Render(headerText)
		} else {
			dayHeaders[i] = calWeekDayHeaderStyle.Render(headerText)
		}

		// Day content box
		var content strings.Builder

		// Get schedule info for this day
		var programCount int
		var fillPct float64
		var timeRange string

		if weekSchedule != nil {
			daySlots := weekSchedule.GetSlotsForDay(dayDate)
			for _, slots := range daySlots {
				programCount += len(slots)
			}
			fillPct = weekSchedule.FillPercentage(dayDate)
			timeRange = getDayTimeRange(weekSchedule, dayDate)
		}

		// Fill bar
		fillBar := renderFillBar(fillPct, 8)
		content.WriteString(fillBar)
		content.WriteString("\n")

		// Program count
		if programCount > 0 {
			content.WriteString(fmt.Sprintf("%d progs", programCount))
			content.WriteString("\n")
			content.WriteString(timeRange)
		} else {
			content.WriteString("empty")
			content.WriteString("\n")
			content.WriteString("--:-- -")
		}

		// Use selected or normal box style
		if isSelected {
			days[i] = calWeekDayBoxSelectedStyle.Render(content.String())
		} else {
			days[i] = calWeekDayBoxStyle.Render(content.String())
		}
	}

	// Render headers row
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, dayHeaders...))
	b.WriteString("\n")

	// Render day boxes row
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, days...))
	b.WriteString("\n")

	// Week summary
	if weekSchedule != nil {
		totalPrograms := weekSchedule.TotalProgramCount()
		channelCount := weekSchedule.ChannelCount()
		b.WriteString("\n")
		summary := fmt.Sprintf("Total: %d programs across %d channels", totalPrograms, channelCount)
		b.WriteString(calWeekInfoStyle.Render(summary))
	}

	// Status message
	if m.statusMessage != "" {
		b.WriteString("\n")
		b.WriteString(calMonthStatusStyle.Render(m.statusMessage))
	}

	// Help
	b.WriteString("\n\n")
	help := "[/] prev/next week | [hl] navigate days | [t] today | [enter] day view | [m] month | [g] generate | [?] help"
	b.WriteString(calWeekHelpStyle.Render(help))

	return b.String()
}

// renderFillBar renders a visual fill bar showing schedule coverage.
func renderFillBar(pct float64, width int) string {
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}

	var bar strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			bar.WriteString(calWeekFillBarFilled.Render("*"))
		} else {
			bar.WriteString(calWeekFillBarEmpty.Render("-"))
		}
	}
	return bar.String()
}

// getDayTimeRange returns a string showing the time range of programs for a day.
func getDayTimeRange(weekSchedule *schedule.WeekSchedule, day time.Time) string {
	daySlots := weekSchedule.GetSlotsForDay(day)
	if len(daySlots) == 0 {
		return "--:-- -"
	}

	var earliest, latest time.Time
	first := true

	for _, slots := range daySlots {
		for _, slot := range slots {
			if first || slot.StartTime.Before(earliest) {
				earliest = slot.StartTime
			}
			endTime := slot.EndTime()
			if first || endTime.After(latest) {
				latest = endTime
			}
			first = false
		}
	}

	if first {
		return "--:-- -"
	}

	return fmt.Sprintf("%s-%s",
		earliest.Format("15:04"),
		latest.Format("15:04"))
}

// generateWeekFromBlocksCmd returns a command to generate schedule from blocks.
func (m Model) generateWeekFromBlocksCmd() tea.Cmd {
	return func() tea.Msg {
		// This will be implemented later with full block-based generation
		// For now, just update status
		return scheduleGeneratedMsg{
			plan: nil,
			err:  nil,
		}
	}
}
