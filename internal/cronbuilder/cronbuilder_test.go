package cronbuilder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	e := New()
	require.NotNil(t, e)
	assert.Equal(t, "*", e.Minute)
	assert.Equal(t, "*", e.Hour)
	assert.Equal(t, "*", e.DayOfMonth)
	assert.Equal(t, "*", e.Month)
	assert.Equal(t, "*", e.DayOfWeek)
	assert.Equal(t, "* * * * *", e.String())
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *Expression
	}{
		{
			name:  "full cron expression",
			input: "0 6 * * 1-5",
			expected: &Expression{
				Minute:     "0",
				Hour:       "6",
				DayOfMonth: "*",
				Month:      "*",
				DayOfWeek:  "1-5",
			},
		},
		{
			name:  "every minute",
			input: "* * * * *",
			expected: &Expression{
				Minute:     "*",
				Hour:       "*",
				DayOfMonth: "*",
				Month:      "*",
				DayOfWeek:  "*",
			},
		},
		{
			name:  "step values",
			input: "*/15 */2 */7 * *",
			expected: &Expression{
				Minute:     "*/15",
				Hour:       "*/2",
				DayOfMonth: "*/7",
				Month:      "*",
				DayOfWeek:  "*",
			},
		},
		{
			name:  "empty string defaults to wildcards",
			input: "",
			expected: &Expression{
				Minute:     "*",
				Hour:       "*",
				DayOfMonth: "*",
				Month:      "*",
				DayOfWeek:  "*",
			},
		},
		{
			name:  "partial expression defaults remaining to wildcards",
			input: "0 6",
			expected: &Expression{
				Minute:     "*",
				Hour:       "*",
				DayOfMonth: "*",
				Month:      "*",
				DayOfWeek:  "*",
			},
		},
		{
			name:  "extra whitespace handled",
			input: "  30   12   1   6   0  ",
			expected: &Expression{
				Minute:     "30",
				Hour:       "12",
				DayOfMonth: "1",
				Month:      "6",
				DayOfWeek:  "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpressionString(t *testing.T) {
	tests := []struct {
		name     string
		expr     *Expression
		expected string
	}{
		{
			name:     "all wildcards",
			expr:     New(),
			expected: "* * * * *",
		},
		{
			name: "mixed values",
			expr: &Expression{
				Minute:     "30",
				Hour:       "9",
				DayOfMonth: "1",
				Month:      "*/3",
				DayOfWeek:  "1-5",
			},
			expected: "30 9 1 */3 1-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.expr.String())
		})
	}
}

func TestField(t *testing.T) {
	e := &Expression{
		Minute:     "0",
		Hour:       "6",
		DayOfMonth: "15",
		Month:      "3",
		DayOfWeek:  "1",
	}

	tests := []struct {
		fieldType FieldType
		expected  string
	}{
		{FieldMinute, "0"},
		{FieldHour, "6"},
		{FieldDayOfMonth, "15"},
		{FieldMonth, "3"},
		{FieldDayOfWeek, "1"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			field := e.Field(tt.fieldType)
			assert.Equal(t, tt.expected, *field)
		})
	}

	// Test default case (invalid field type returns Minute)
	invalidField := e.Field(FieldType(99))
	assert.Equal(t, "0", *invalidField)
}

func TestCycleNext(t *testing.T) {
	tests := []struct {
		name      string
		fieldType FieldType
		initial   string
		expected  string
	}{
		{"minute from wildcard", FieldMinute, "*", "0"},
		{"minute from 0", FieldMinute, "0", "15"},
		{"minute wrap around", FieldMinute, "*/30", "*"},
		{"hour from wildcard", FieldHour, "*", "0"},
		{"day of week from 6", FieldDayOfWeek, "6", "1-5"},
		{"unknown value cycles to first", FieldMinute, "99", "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New()
			*e.Field(tt.fieldType) = tt.initial
			e.CycleNext(tt.fieldType)
			assert.Equal(t, tt.expected, *e.Field(tt.fieldType))
		})
	}
}

func TestCyclePrev(t *testing.T) {
	tests := []struct {
		name      string
		fieldType FieldType
		initial   string
		expected  string
	}{
		{"minute from wildcard", FieldMinute, "*", "*/30"},
		{"minute from 0", FieldMinute, "0", "*"},
		{"minute from 15", FieldMinute, "15", "0"},
		{"hour from wildcard", FieldHour, "*", "*/6"},
		{"unknown value cycles to last", FieldMinute, "99", "*/30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New()
			*e.Field(tt.fieldType) = tt.initial
			e.CyclePrev(tt.fieldType)
			assert.Equal(t, tt.expected, *e.Field(tt.fieldType))
		})
	}
}

func TestAppendDigit(t *testing.T) {
	tests := []struct {
		name      string
		initial   string
		digit     string
		expected  string
	}{
		{"append to wildcard", "*", "5", "5"},
		{"append to empty", "", "3", "3"},
		{"append to existing", "1", "5", "15"},
		{"build multi-digit", "12", "0", "120"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New()
			e.Minute = tt.initial
			e.AppendDigit(FieldMinute, tt.digit)
			assert.Equal(t, tt.expected, e.Minute)
		})
	}
}

func TestBackspace(t *testing.T) {
	tests := []struct {
		name     string
		initial  string
		expected string
	}{
		{"remove from multi-char", "15", "1"},
		{"remove from single char", "5", "*"},
		{"remove from wildcard", "*", "*"},
		{"remove from empty becomes wildcard", "", "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New()
			e.Minute = tt.initial
			e.Backspace(FieldMinute)
			assert.Equal(t, tt.expected, e.Minute)
		})
	}
}

func TestSetWildcard(t *testing.T) {
	e := &Expression{
		Minute:     "30",
		Hour:       "12",
		DayOfMonth: "15",
		Month:      "6",
		DayOfWeek:  "1",
	}

	e.SetWildcard(FieldMinute)
	assert.Equal(t, "*", e.Minute)
	assert.Equal(t, "12", e.Hour) // Others unchanged

	e.SetWildcard(FieldHour)
	e.SetWildcard(FieldDayOfMonth)
	e.SetWildcard(FieldMonth)
	e.SetWildcard(FieldDayOfWeek)
	assert.Equal(t, "* * * * *", e.String())
}

func TestDescribe(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{"every minute", "* * * * *", "every minute"},
		{"specific minute", "30 * * * *", "at minute 30"},
		{"every 5 minutes", "*/5 * * * *", "every 5 minutes"},
		{"hourly at 0", "0 * * * *", "at minute 0"},
		{"daily at 6am", "0 6 * * *", "at minute 0, at hour 6"},
		{"every 2 hours", "0 */2 * * *", "at minute 0, every 2 hours"},
		{"monthly on 1st", "0 0 1 * *", "at minute 0, at hour 0, on day 1"},
		{"every 7 days", "0 0 */7 * *", "at minute 0, at hour 0, every 7 days"},
		{"in january", "0 0 1 1 *", "at minute 0, at hour 0, on day 1, in Jan"},
		{"in june", "0 0 1 6 *", "at minute 0, at hour 0, on day 1, in Jun"},
		{"in december", "0 0 1 12 *", "at minute 0, at hour 0, on day 1, in Dec"},
		{"invalid month", "0 0 1 13 *", "at minute 0, at hour 0, on day 1, in month 13"},
		{"non-numeric month", "0 0 1 */3 *", "at minute 0, at hour 0, on day 1, in month */3"},
		{"on sunday", "0 0 * * 0", "at minute 0, at hour 0, on Sun"},
		{"on monday", "0 0 * * 1", "at minute 0, at hour 0, on Mon"},
		{"on saturday", "0 0 * * 6", "at minute 0, at hour 0, on Sat"},
		{"on weekdays", "0 9 * * 1-5", "at minute 0, at hour 9, on weekdays"},
		{"invalid day", "0 0 * * 7", "at minute 0, at hour 0, on day 7"},
		{"complex pattern", "30 9 15 6 1", "at minute 30, at hour 9, on day 15, in Jun, on Mon"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Parse(tt.expr)
			assert.Equal(t, tt.expected, e.Describe())
		})
	}
}

func TestFields(t *testing.T) {
	fields := Fields()
	require.Len(t, fields, 5)

	expected := []struct {
		fieldType FieldType
		label     string
	}{
		{FieldMinute, "Minute"},
		{FieldHour, "Hour"},
		{FieldDayOfMonth, "Day"},
		{FieldMonth, "Month"},
		{FieldDayOfWeek, "Weekday"},
	}

	for i, exp := range expected {
		assert.Equal(t, exp.fieldType, fields[i].Type)
		assert.Equal(t, exp.label, fields[i].Label)
		assert.NotEmpty(t, fields[i].Desc)
	}
}

func TestCommonPresets(t *testing.T) {
	presets := CommonPresets()
	require.NotEmpty(t, presets)

	// Verify each preset has valid label and expression
	for _, preset := range presets {
		assert.NotEmpty(t, preset.Label)
		assert.NotEmpty(t, preset.Expr)

		// Verify expression parses correctly (5 parts)
		e := Parse(preset.Expr)
		assert.NotEqual(t, "", e.Minute)
		assert.NotEqual(t, "", e.Hour)
	}

	// Check specific presets exist
	labels := make([]string, len(presets))
	for i, p := range presets {
		labels[i] = p.Label
	}
	assert.Contains(t, labels, "Every minute")
	assert.Contains(t, labels, "Hourly")
	assert.Contains(t, labels, "Daily at 6am")
}

func TestFieldPresets(t *testing.T) {
	// Verify all field types have presets
	for _, ft := range []FieldType{FieldMinute, FieldHour, FieldDayOfMonth, FieldMonth, FieldDayOfWeek} {
		presets, ok := FieldPresets[ft]
		assert.True(t, ok, "missing presets for field type %d", ft)
		assert.NotEmpty(t, presets, "empty presets for field type %d", ft)
		assert.Equal(t, "*", presets[0], "first preset should be wildcard for field type %d", ft)
	}
}

func TestRoundTripParseString(t *testing.T) {
	// Test that parsing and converting back to string preserves the expression
	expressions := []string{
		"* * * * *",
		"0 6 * * 1-5",
		"*/15 */2 */7 */3 *",
		"30 12 15 6 0",
	}

	for _, expr := range expressions {
		t.Run(expr, func(t *testing.T) {
			parsed := Parse(expr)
			assert.Equal(t, expr, parsed.String())
		})
	}
}
