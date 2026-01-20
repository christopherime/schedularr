package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/geekxflood/schedularr/internal/schedule"
)

// Calendar month view styles
var (
	calMonthTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				Padding(0, 1)

	calMonthHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Bold(true)

	calDayCellStyle = lipgloss.NewStyle().
			Width(4).
			Height(1).
			Align(lipgloss.Center)

	calDaySelectedStyle = lipgloss.NewStyle().
				Width(4).
				Height(1).
				Align(lipgloss.Center).
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("255")).
				Bold(true)

	calDayTodayStyle = lipgloss.NewStyle().
				Width(4).
				Height(1).
				Align(lipgloss.Center).
				Foreground(lipgloss.Color("214")).
				Bold(true)

	calDayEmptyStyle = lipgloss.NewStyle().
				Width(4).
				Height(1).
				Align(lipgloss.Center).
				Foreground(lipgloss.Color("240"))

	calDayScheduledStyle = lipgloss.NewStyle().
				Width(4).
				Height(1).
				Align(lipgloss.Center).
				Foreground(lipgloss.Color("42"))

	calDayFullStyle = lipgloss.NewStyle().
			Width(4).
			Height(1).
			Align(lipgloss.Center).
			Foreground(lipgloss.Color("35")).
			Bold(true)

	calMonthHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Padding(1, 0)

	calMonthStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205"))
)

// updateCalendarMonth handles key events in calendar month view.
func (m Model) updateCalendarMonth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "left", "h":
		// Move to previous day
		m.calendarDate = m.calendarDate.AddDate(0, 0, -1)
		m.calendarSelection.Day = m.calendarDate.Day()
		return m, nil

	case "right", "l":
		// Move to next day
		m.calendarDate = m.calendarDate.AddDate(0, 0, 1)
		m.calendarSelection.Day = m.calendarDate.Day()
		return m, nil

	case "up", "k":
		// Move up one week
		m.calendarDate = m.calendarDate.AddDate(0, 0, -7)
		m.calendarSelection.Day = m.calendarDate.Day()
		return m, nil

	case "down", "j":
		// Move down one week
		m.calendarDate = m.calendarDate.AddDate(0, 0, 7)
		m.calendarSelection.Day = m.calendarDate.Day()
		return m, nil

	case "[":
		// Previous month
		m.calendarDate = m.calendarDate.AddDate(0, -1, 0)
		// Clamp day if needed
		m.calendarSelection.Day = m.calendarDate.Day()
		return m, nil

	case "]":
		// Next month
		m.calendarDate = m.calendarDate.AddDate(0, 1, 0)
		m.calendarSelection.Day = m.calendarDate.Day()
		return m, nil

	case "t":
		// Jump to today
		m.calendarDate = time.Now().In(m.timezone)
		m.calendarSelection.Day = m.calendarDate.Day()
		return m, nil

	case "enter":
		// Go to week view for selected date
		m.state = stateCalendarWeek
		m.calendarViewMode = CalendarViewWeek
		return m, nil

	case "b":
		// Go to blocks view
		m.state = stateListView
		return m, nil

	case "c":
		// Go to channels view (if available)
		if m.tunarrClient != nil {
			m.state = stateChannelView
			return m, m.fetchChannelsCmd()
		}
		return m, nil

	case "?":
		// Show help
		m.prevState = m.state
		m.state = stateHelp
		return m, nil
	}

	return m, nil
}

// renderCalendarMonth renders the calendar month grid view.
func (m Model) renderCalendarMonth() string {
	var b strings.Builder

	// Get current month/year from calendarDate
	year := m.calendarDate.Year()
	month := m.calendarDate.Month()

	// Title with navigation arrows
	title := fmt.Sprintf("  %s %d  ", month.String(), year)
	b.WriteString(calMonthTitleStyle.Render(title))
	b.WriteString("\n\n")

	// Day headers (Monday first)
	headers := []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	var headerRow strings.Builder
	for _, h := range headers {
		headerRow.WriteString(calMonthHeaderStyle.Render(fmt.Sprintf(" %s ", h)))
	}
	b.WriteString(headerRow.String())
	b.WriteString("\n")

	// Calculate first day of month and days in month
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, m.timezone)
	lastDay := firstDay.AddDate(0, 1, -1)
	daysInMonth := lastDay.Day()

	// Find which weekday the month starts on (Monday = 0)
	startWeekday := int(firstDay.Weekday())
	if startWeekday == 0 {
		startWeekday = 7 // Sunday becomes 7
	}
	startWeekday-- // Make Monday = 0

	// Get today for highlighting
	today := time.Now().In(m.timezone)
	isCurrentMonth := today.Year() == year && today.Month() == month

	// Load schedule data for the month
	weekSchedules := m.loadMonthSchedules(year, month)

	// Render calendar grid
	day := 1
	for week := 0; week < 6; week++ {
		var row strings.Builder

		for weekday := 0; weekday < 7; weekday++ {
			cellPos := week*7 + weekday

			if cellPos < startWeekday || day > daysInMonth {
				// Empty cell
				row.WriteString(calDayEmptyStyle.Render("  "))
			} else {
				// Calculate fill percentage for this day
				dayDate := time.Date(year, month, day, 0, 0, 0, 0, m.timezone)
				fillPct := m.getDayFillPercentage(dayDate, weekSchedules)

				// Determine cell style
				dayStr := fmt.Sprintf("%2d", day)
				isSelected := day == m.calendarSelection.Day
				isToday := isCurrentMonth && day == today.Day()

				var cellStyle lipgloss.Style
				switch {
				case isSelected:
					cellStyle = calDaySelectedStyle
				case isToday:
					cellStyle = calDayTodayStyle
				case fillPct >= 0.8:
					cellStyle = calDayFullStyle
				case fillPct > 0:
					cellStyle = calDayScheduledStyle
				default:
					cellStyle = calDayCellStyle
				}

				row.WriteString(cellStyle.Render(dayStr))
				day++
			}
		}

		b.WriteString(row.String())
		b.WriteString("\n")

		// Stop if we've rendered all days
		if day > daysInMonth {
			break
		}
	}

	// Legend
	b.WriteString("\n")
	legend := fmt.Sprintf("Legend: %s Scheduled  %s Full  %s Today",
		calDayScheduledStyle.Render("*"),
		calDayFullStyle.Render("*"),
		calDayTodayStyle.Render("*"))
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(legend))

	// Status message
	if m.statusMessage != "" {
		b.WriteString("\n")
		b.WriteString(calMonthStatusStyle.Render(m.statusMessage))
	}

	// Help
	b.WriteString("\n\n")
	help := "[/] prev/next month | [hjkl] navigate | [t] today | [enter] week view | [b] blocks | [?] help | [q] quit"
	b.WriteString(calMonthHelpStyle.Render(help))

	return b.String()
}

// loadMonthSchedules loads all week schedules that overlap with the given month.
func (m *Model) loadMonthSchedules(year int, month time.Month) map[string]*schedule.WeekSchedule {
	if m.scheduleManager == nil {
		return nil
	}

	// Get first and last day of month
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, m.timezone)
	lastDay := firstDay.AddDate(0, 1, -1)

	// Get all week IDs that overlap with this month
	weekIDs := m.scheduleManager.GetWeeksInRange(firstDay, lastDay)

	// Load schedules
	schedules := make(map[string]*schedule.WeekSchedule)
	for _, weekID := range weekIDs {
		// Check if already in loadedWeeks cache
		if sched, ok := m.loadedWeeks[weekID]; ok {
			schedules[weekID] = sched
			continue
		}

		// Load from manager (which has its own LRU cache)
		sched, err := m.scheduleManager.GetWeek(weekID)
		if err != nil {
			m.logger.Debug("failed to load week schedule", "week_id", weekID, "error", err)
			continue
		}

		// Cache in model for quick access
		m.loadedWeeks[weekID] = sched
		schedules[weekID] = sched
	}

	return schedules
}

// getDayFillPercentage returns the fill percentage for a specific day.
func (m *Model) getDayFillPercentage(day time.Time, weekSchedules map[string]*schedule.WeekSchedule) float64 {
	weekID := schedule.WeekID(day)
	sched, ok := weekSchedules[weekID]
	if !ok || sched == nil {
		return 0.0
	}
	return sched.FillPercentage(day)
}
