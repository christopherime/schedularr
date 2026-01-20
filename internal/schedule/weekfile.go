// Package schedule provides week-based schedule file management for Schedularr.
package schedule

import (
	"fmt"
	"time"
)

// ProgramType represents the type of a program slot.
type ProgramType string

// Program type constants.
const (
	ProgramTypeMovie   ProgramType = "movie"
	ProgramTypeEpisode ProgramType = "episode"
	ProgramTypeFlex    ProgramType = "flex"
	ProgramTypeFiller  ProgramType = "filler"
)

// ProgramSlot represents a single scheduled program within a week.
type ProgramSlot struct {
	ProgramID   string        `yaml:"program_id"`
	Title       string        `yaml:"title"`
	StartTime   time.Time     `yaml:"start_time"`
	Duration    time.Duration `yaml:"duration"`
	Type        ProgramType   `yaml:"type"`
	BlockName   string        `yaml:"block_name,omitempty"`
	ChannelID   string        `yaml:"channel_id"`
	ShowTitle   string        `yaml:"show_title,omitempty"`
	SeasonNum   int           `yaml:"season_num,omitempty"`
	EpisodeNum  int           `yaml:"episode_num,omitempty"`
	EpisodeName string        `yaml:"episode_name,omitempty"`
	Year        int           `yaml:"year,omitempty"`
	Genres      []string      `yaml:"genres,omitempty"`
	Rating      string        `yaml:"rating,omitempty"`
	ManualEdit  bool          `yaml:"manual_edit,omitempty"`
}

// EndTime returns the end time of the program slot.
func (p *ProgramSlot) EndTime() time.Time {
	return p.StartTime.Add(p.Duration)
}

// IsPast returns true if the program has already ended.
func (p *ProgramSlot) IsPast(now time.Time) bool {
	return p.EndTime().Before(now)
}

// IsOngoing returns true if the program is currently airing.
func (p *ProgramSlot) IsOngoing(now time.Time) bool {
	return !p.StartTime.After(now) && p.EndTime().After(now)
}

// IsFuture returns true if the program hasn't started yet.
func (p *ProgramSlot) IsFuture(now time.Time) bool {
	return p.StartTime.After(now)
}

// WeekSchedule represents a week's worth of programming across channels.
type WeekSchedule struct {
	WeekID       string                   `yaml:"week_id"`
	WeekStart    time.Time                `yaml:"week_start"`
	WeekEnd      time.Time                `yaml:"week_end"`
	ChannelSlots map[string][]ProgramSlot `yaml:"channel_slots"`
	CreatedAt    time.Time                `yaml:"created_at"`
	ModifiedAt   time.Time                `yaml:"modified_at"`
	Timezone     string                   `yaml:"timezone"`
}

// NewWeekSchedule creates a new empty week schedule for the given week.
func NewWeekSchedule(weekID string, start, end time.Time, tz *time.Location) *WeekSchedule {
	tzName := "UTC"
	if tz != nil {
		tzName = tz.String()
	}
	now := time.Now()
	return &WeekSchedule{
		WeekID:       weekID,
		WeekStart:    start,
		WeekEnd:      end,
		ChannelSlots: make(map[string][]ProgramSlot),
		CreatedAt:    now,
		ModifiedAt:   now,
		Timezone:     tzName,
	}
}

// AddSlot adds a program slot to the specified channel.
func (w *WeekSchedule) AddSlot(channelID string, slot ProgramSlot) {
	if w.ChannelSlots == nil {
		w.ChannelSlots = make(map[string][]ProgramSlot)
	}
	w.ChannelSlots[channelID] = append(w.ChannelSlots[channelID], slot)
	w.ModifiedAt = time.Now()
}

// GetSlotsForChannel returns all program slots for a channel.
func (w *WeekSchedule) GetSlotsForChannel(channelID string) []ProgramSlot {
	return w.ChannelSlots[channelID]
}

// GetSlotsForDay returns all program slots for a specific day across all channels.
func (w *WeekSchedule) GetSlotsForDay(day time.Time) map[string][]ProgramSlot {
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	result := make(map[string][]ProgramSlot)
	for channelID, slots := range w.ChannelSlots {
		var daySlots []ProgramSlot
		for _, slot := range slots {
			// Include if slot overlaps with the day
			if slot.StartTime.Before(dayEnd) && slot.EndTime().After(dayStart) {
				daySlots = append(daySlots, slot)
			}
		}
		if len(daySlots) > 0 {
			result[channelID] = daySlots
		}
	}
	return result
}

// TotalProgramCount returns the total number of programs across all channels.
func (w *WeekSchedule) TotalProgramCount() int {
	count := 0
	for _, slots := range w.ChannelSlots {
		count += len(slots)
	}
	return count
}

// ChannelCount returns the number of channels with programming.
func (w *WeekSchedule) ChannelCount() int {
	return len(w.ChannelSlots)
}

// IsEmpty returns true if the schedule has no programs.
func (w *WeekSchedule) IsEmpty() bool {
	return w.TotalProgramCount() == 0
}

// FillPercentage returns the approximate fill percentage for a day (0.0 to 1.0).
// This is a rough estimate based on total scheduled time vs 24 hours.
func (w *WeekSchedule) FillPercentage(day time.Time) float64 {
	daySlots := w.GetSlotsForDay(day)
	if len(daySlots) == 0 {
		return 0.0
	}

	var totalDuration time.Duration
	for _, slots := range daySlots {
		for _, slot := range slots {
			totalDuration += slot.Duration
		}
	}

	// Compare against 24 hours * number of channels with content
	maxDuration := time.Duration(len(daySlots)) * 24 * time.Hour
	if maxDuration == 0 {
		return 0.0
	}

	percentage := float64(totalDuration) / float64(maxDuration)
	if percentage > 1.0 {
		percentage = 1.0
	}
	return percentage
}

// WeekID returns the ISO week identifier (YYYY-WNN) for a given time.
func WeekID(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// WeekBounds returns the Monday 00:00 start and Sunday 23:59:59 end of the week
// containing the given time, in the specified timezone.
func WeekBounds(t time.Time, tz *time.Location) (start, end time.Time) {
	if tz == nil {
		tz = time.UTC
	}

	// Convert to the target timezone
	t = t.In(tz)

	// Find Monday of this week
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	daysFromMonday := int(weekday) - 1

	// Get Monday at 00:00:00
	year, month, day := t.Date()
	monday := time.Date(year, month, day-daysFromMonday, 0, 0, 0, 0, tz)

	// Get Sunday at 23:59:59.999999999
	sunday := monday.Add(7*24*time.Hour - time.Nanosecond)

	return monday, sunday
}

// ParseWeekID parses a week ID string (YYYY-WNN) and returns the year and week number.
func ParseWeekID(weekID string) (year, week int, err error) {
	_, err = fmt.Sscanf(weekID, "%d-W%02d", &year, &week)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid week ID format: %w", err)
	}
	if week < 1 || week > 53 {
		return 0, 0, fmt.Errorf("week number must be between 1 and 53, got %d", week)
	}
	return year, week, nil
}

// WeekStartFromID returns the Monday start time for a given week ID.
func WeekStartFromID(weekID string, tz *time.Location) (time.Time, error) {
	year, week, err := ParseWeekID(weekID)
	if err != nil {
		return time.Time{}, err
	}

	if tz == nil {
		tz = time.UTC
	}

	// Find the first day of the year
	firstDay := time.Date(year, time.January, 1, 0, 0, 0, 0, tz)

	// Find what weekday Jan 1 falls on (Monday=1, Sunday=7)
	firstWeekday := int(firstDay.Weekday())
	if firstWeekday == 0 {
		firstWeekday = 7
	}

	// ISO week 1 is the week containing the first Thursday of the year
	// Calculate days to add to get to Monday of week 1
	var daysToWeek1Monday int
	if firstWeekday <= 4 {
		// Jan 1 is Mon-Thu, so week 1 starts on the Monday of this week
		daysToWeek1Monday = 1 - firstWeekday
	} else {
		// Jan 1 is Fri-Sun, so week 1 starts on the next Monday
		daysToWeek1Monday = 8 - firstWeekday
	}

	week1Monday := firstDay.AddDate(0, 0, daysToWeek1Monday)

	// Add weeks to get to the target week
	targetMonday := week1Monday.AddDate(0, 0, (week-1)*7)

	return targetMonday, nil
}
