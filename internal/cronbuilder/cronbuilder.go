// Package cronbuilder provides utilities for building and describing cron expressions.
package cronbuilder

import (
	"fmt"
	"strconv"
	"strings"
)

// FieldPresets defines preset values for each cron field.
var FieldPresets = map[FieldType][]string{
	FieldMinute:     {"*", "0", "15", "30", "45", "*/5", "*/10", "*/15", "*/30"},
	FieldHour:       {"*", "0", "6", "9", "12", "15", "18", "21", "*/2", "*/3", "*/6"},
	FieldDayOfMonth: {"*", "1", "15", "*/7"},
	FieldMonth:      {"*", "1", "3", "6", "9", "12"},
	FieldDayOfWeek:  {"*", "0", "1", "2", "3", "4", "5", "6", "1-5"},
}

// FieldType represents a cron expression field.
type FieldType int

// Cron field type constants.
const (
	FieldMinute FieldType = iota
	FieldHour
	FieldDayOfMonth
	FieldMonth
	FieldDayOfWeek
)

// FieldInfo contains metadata about a cron field.
type FieldInfo struct {
	Type  FieldType
	Label string
	Desc  string
}

// Fields returns metadata for all cron fields in order.
func Fields() []FieldInfo {
	return []FieldInfo{
		{FieldMinute, "Minute", "(0-59)"},
		{FieldHour, "Hour", "(0-23)"},
		{FieldDayOfMonth, "Day", "(1-31)"},
		{FieldMonth, "Month", "(1-12)"},
		{FieldDayOfWeek, "Weekday", "(0-6, 0=Sun)"},
	}
}

// Expression represents a cron expression with its 5 fields.
type Expression struct {
	Minute     string
	Hour       string
	DayOfMonth string
	Month      string
	DayOfWeek  string
}

// New creates a new Expression with default wildcard values.
func New() *Expression {
	return &Expression{
		Minute:     "*",
		Hour:       "*",
		DayOfMonth: "*",
		Month:      "*",
		DayOfWeek:  "*",
	}
}

// Parse creates an Expression from a cron string.
func Parse(expr string) *Expression {
	parts := strings.Fields(expr)
	e := New()
	if len(parts) >= 5 {
		e.Minute = parts[0]
		e.Hour = parts[1]
		e.DayOfMonth = parts[2]
		e.Month = parts[3]
		e.DayOfWeek = parts[4]
	}
	return e
}

// String returns the cron expression as a string.
func (e *Expression) String() string {
	return fmt.Sprintf("%s %s %s %s %s", e.Minute, e.Hour, e.DayOfMonth, e.Month, e.DayOfWeek)
}

// Field returns a pointer to the field at the given index.
func (e *Expression) Field(ft FieldType) *string {
	switch ft {
	case FieldMinute:
		return &e.Minute
	case FieldHour:
		return &e.Hour
	case FieldDayOfMonth:
		return &e.DayOfMonth
	case FieldMonth:
		return &e.Month
	case FieldDayOfWeek:
		return &e.DayOfWeek
	}
	return &e.Minute
}

// CycleNext moves the field to the next preset value.
func (e *Expression) CycleNext(ft FieldType) {
	field := e.Field(ft)
	presets := FieldPresets[ft]
	cycleValue(field, presets, 1)
}

// CyclePrev moves the field to the previous preset value.
func (e *Expression) CyclePrev(ft FieldType) {
	field := e.Field(ft)
	presets := FieldPresets[ft]
	cycleValue(field, presets, -1)
}

func cycleValue(field *string, values []string, direction int) {
	for i, v := range values {
		if v == *field {
			newIdx := (i + direction + len(values)) % len(values)
			*field = values[newIdx]
			return
		}
	}
	if direction > 0 {
		*field = values[0]
	} else {
		*field = values[len(values)-1]
	}
}

// AppendDigit adds a digit to the specified field.
func (e *Expression) AppendDigit(ft FieldType, digit string) {
	field := e.Field(ft)
	if *field == "*" || *field == "" {
		*field = digit
	} else {
		*field += digit
	}
}

// Backspace removes the last character from the specified field.
func (e *Expression) Backspace(ft FieldType) {
	field := e.Field(ft)
	if len(*field) > 0 {
		*field = (*field)[:len(*field)-1]
	}
	if *field == "" {
		*field = "*"
	}
}

// SetWildcard sets the specified field to "*".
func (e *Expression) SetWildcard(ft FieldType) {
	*e.Field(ft) = "*"
}

// Describe returns a human-readable description of the cron expression.
func (e *Expression) Describe() string {
	parts := make([]string, 0, 5)
	parts = append(parts, describeMinute(e.Minute)...)
	parts = append(parts, describeHour(e.Hour)...)
	parts = append(parts, describeDayOfMonth(e.DayOfMonth)...)
	parts = append(parts, describeMonth(e.Month)...)
	parts = append(parts, describeDayOfWeek(e.DayOfWeek)...)

	if len(parts) == 0 {
		return "runs continuously"
	}
	return strings.Join(parts, ", ")
}

func describeMinute(m string) []string {
	if m == "*" {
		return []string{"every minute"}
	}
	if strings.HasPrefix(m, "*/") {
		return []string{"every " + strings.TrimPrefix(m, "*/") + " minutes"}
	}
	return []string{"at minute " + m}
}

func describeHour(h string) []string {
	if h == "*" {
		return nil
	}
	if strings.HasPrefix(h, "*/") {
		return []string{"every " + strings.TrimPrefix(h, "*/") + " hours"}
	}
	return []string{"at hour " + h}
}

func describeDayOfMonth(d string) []string {
	if d == "*" {
		return nil
	}
	if strings.HasPrefix(d, "*/") {
		return []string{"every " + strings.TrimPrefix(d, "*/") + " days"}
	}
	return []string{"on day " + d}
}

func describeMonth(m string) []string {
	if m == "*" {
		return nil
	}
	monthNames := []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	if month, err := strconv.Atoi(m); err == nil && month >= 1 && month <= 12 {
		return []string{"in " + monthNames[month]}
	}
	return []string{"in month " + m}
}

func describeDayOfWeek(d string) []string {
	if d == "*" {
		return nil
	}
	if d == "1-5" {
		return []string{"on weekdays"}
	}
	dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	if day, err := strconv.Atoi(d); err == nil && day >= 0 && day <= 6 {
		return []string{"on " + dayNames[day]}
	}
	return []string{"on day " + d}
}

// CommonPresets returns common cron expression examples.
func CommonPresets() []struct {
	Label string
	Expr  string
} {
	return []struct {
		Label string
		Expr  string
	}{
		{"Every minute", "* * * * *"},
		{"Hourly", "0 * * * *"},
		{"Daily at 6am", "0 6 * * *"},
		{"Weekly (Monday)", "0 0 * * 1"},
		{"Monthly (1st)", "0 0 1 * *"},
		{"Weekdays 9am", "0 9 * * 1-5"},
	}
}
