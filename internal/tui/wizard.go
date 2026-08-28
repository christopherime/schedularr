package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/christopherime/schedularr/internal/cronbuilder"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
)

// ContentAnalysis contains analysis of available content in Tunarr.
type ContentAnalysis struct {
	TotalPrograms int
	Genres        map[string]int // Genre -> count
	Ratings       map[string]int // Rating -> count
	Types         map[string]int // Type (movie/episode) -> count
	YearRange     [2]int         // Min and max years
	DurationRange [2]int         // Min and max duration in minutes
}

// BlockAnalysis contains analysis of a block's viability.
type BlockAnalysis struct {
	Block           scheduler.Block
	TargetChannel   string // Channel name (resolved from ID)
	MatchingCount   int    // Number of programs matching filter
	Issues          []string
	CanGenerate     bool
	SuggestedFixes  []string
}

// analyzeContent analyzes the available programs and returns content statistics.
func analyzeContent(programs []tunarr.Program) *ContentAnalysis {
	analysis := &ContentAnalysis{
		TotalPrograms: len(programs),
		Genres:        make(map[string]int),
		Ratings:       make(map[string]int),
		Types:         make(map[string]int),
		YearRange:     [2]int{9999, 0},
		DurationRange: [2]int{999999, 0},
	}

	for _, p := range programs {
		// Count types
		if p.Type != "" {
			analysis.Types[p.Type]++
		}

		// Count genres
		for _, g := range p.Genres {
			if g.Name != "" {
				analysis.Genres[g.Name]++
			}
		}

		// Count ratings
		if p.Rating != "" {
			analysis.Ratings[p.Rating]++
		}

		// Track year range
		year := p.GetYear()
		if year > 0 {
			if year < analysis.YearRange[0] {
				analysis.YearRange[0] = year
			}
			if year > analysis.YearRange[1] {
				analysis.YearRange[1] = year
			}
		}

		// Track duration range (convert ms to minutes)
		durationMin := int(p.GetDurationMs() / 60000)
		if durationMin > 0 {
			if durationMin < analysis.DurationRange[0] {
				analysis.DurationRange[0] = durationMin
			}
			if durationMin > analysis.DurationRange[1] {
				analysis.DurationRange[1] = durationMin
			}
		}
	}

	// Handle case where no valid years/durations found
	if analysis.YearRange[0] == 9999 {
		analysis.YearRange[0] = 0
	}
	if analysis.DurationRange[0] == 999999 {
		analysis.DurationRange[0] = 0
	}

	return analysis
}

// analyzeBlocks analyzes each block's viability against available content.
func (m *Model) analyzeBlocks(programs []tunarr.Program) []BlockAnalysis {
	analyses := make([]BlockAnalysis, 0, len(m.blocks))

	// Build channel ID -> name map
	channelNames := make(map[string]string)
	for _, cv := range m.channels {
		channelNames[cv.Channel.ID] = cv.Channel.Name
	}

	for _, block := range m.blocks {
		analysis := BlockAnalysis{
			Block:         block,
			TargetChannel: channelNames[block.ChannelID],
			CanGenerate:   true,
		}

		// Check if channel exists
		if analysis.TargetChannel == "" {
			analysis.TargetChannel = block.ChannelID + " (not found)"
			analysis.Issues = append(analysis.Issues, "Channel not found in Tunarr")
			analysis.SuggestedFixes = append(analysis.SuggestedFixes,
				fmt.Sprintf("Update channel_id to one of: %s", m.getChannelIDList()))
			analysis.CanGenerate = false
		}

		// Count matching programs
		matching := filterProgramsForBlock(programs, block)
		analysis.MatchingCount = len(matching)

		if analysis.MatchingCount == 0 {
			analysis.CanGenerate = false
			analysis.Issues = append(analysis.Issues, "No programs match the filter criteria")

			// Provide specific suggestions based on filter
			if len(block.Filter.Genres) > 0 {
				availableGenres := m.getAvailableGenresMatching(programs, block.Filter.Genres)
				if len(availableGenres) == 0 {
					analysis.SuggestedFixes = append(analysis.SuggestedFixes,
						fmt.Sprintf("Genres %v not found. Available: %s",
							block.Filter.Genres, m.getTopGenres(programs, 5)))
				}
			}
			if len(block.Filter.Ratings) > 0 {
				availableRatings := m.getAvailableRatingsMatching(programs, block.Filter.Ratings)
				if len(availableRatings) == 0 {
					analysis.SuggestedFixes = append(analysis.SuggestedFixes,
						fmt.Sprintf("Ratings %v not found. Available: %s",
							block.Filter.Ratings, m.getTopRatings(programs, 5)))
				}
			}
			if block.Filter.YearFrom > 0 || block.Filter.YearTo > 0 {
				yearRange := m.getYearRange(programs)
				analysis.SuggestedFixes = append(analysis.SuggestedFixes,
					fmt.Sprintf("Year filter %d-%d. Available range: %d-%d",
						block.Filter.YearFrom, block.Filter.YearTo, yearRange[0], yearRange[1]))
			}
		}

		analyses = append(analyses, analysis)
	}

	return analyses
}

// filterProgramsForBlock filters programs based on block criteria.
func filterProgramsForBlock(programs []tunarr.Program, block scheduler.Block) []tunarr.Program {
	var matching []tunarr.Program

	for _, p := range programs {
		if matchesFilter(p, block.Filter) {
			matching = append(matching, p)
		}
	}

	return matching
}

// matchesFilter checks if a program matches the given filter.
func matchesFilter(p tunarr.Program, f scheduler.Filter) bool {
	// Genre filter
	if len(f.Genres) > 0 {
		hasGenre := false
		programGenres := p.GetGenreNames()
		for _, fg := range f.Genres {
			for _, pg := range programGenres {
				if strings.EqualFold(fg, pg) {
					hasGenre = true
					break
				}
			}
			if hasGenre {
				break
			}
		}
		if !hasGenre {
			return false
		}
	}

	// Rating filter
	if len(f.Ratings) > 0 {
		hasRating := false
		for _, r := range f.Ratings {
			if strings.EqualFold(r, p.Rating) {
				hasRating = true
				break
			}
		}
		if !hasRating {
			return false
		}
	}

	// Year filter
	year := p.GetYear()
	if f.YearFrom > 0 && year < f.YearFrom {
		return false
	}
	if f.YearTo > 0 && year > f.YearTo {
		return false
	}

	// Duration filter (convert ms to minutes)
	durationMin := int(p.GetDurationMs() / 60000)
	if f.MinDuration > 0 && durationMin < f.MinDuration {
		return false
	}
	if f.MaxDuration > 0 && durationMin > f.MaxDuration {
		return false
	}

	return true
}

// Helper functions for analysis
func (m *Model) getChannelIDList() string {
	var ids []string
	for _, cv := range m.channels {
		ids = append(ids, cv.Channel.ID)
	}
	if len(ids) == 0 {
		return "(no channels)"
	}
	return strings.Join(ids, ", ")
}

func (m *Model) getAvailableGenresMatching(programs []tunarr.Program, wanted []string) []string {
	genreSet := make(map[string]bool)
	for _, p := range programs {
		for _, g := range p.GetGenreNames() {
			for _, w := range wanted {
				if strings.EqualFold(g, w) {
					genreSet[g] = true
				}
			}
		}
	}
	var result []string
	for g := range genreSet {
		result = append(result, g)
	}
	return result
}

func (m *Model) getAvailableRatingsMatching(programs []tunarr.Program, wanted []string) []string {
	ratingSet := make(map[string]bool)
	for _, p := range programs {
		for _, w := range wanted {
			if strings.EqualFold(p.Rating, w) {
				ratingSet[p.Rating] = true
			}
		}
	}
	var result []string
	for r := range ratingSet {
		result = append(result, r)
	}
	return result
}

func (m *Model) getTopGenres(programs []tunarr.Program, n int) string {
	counts := make(map[string]int)
	for _, p := range programs {
		for _, g := range p.GetGenreNames() {
			counts[g]++
		}
	}
	return topN(counts, n)
}

func (m *Model) getTopRatings(programs []tunarr.Program, n int) string {
	counts := make(map[string]int)
	for _, p := range programs {
		if p.Rating != "" {
			counts[p.Rating]++
		}
	}
	return topN(counts, n)
}

func (m *Model) getYearRange(programs []tunarr.Program) [2]int {
	minYear, maxYear := 9999, 0
	for _, p := range programs {
		year := p.GetYear()
		if year > 0 {
			if year < minYear {
				minYear = year
			}
			if year > maxYear {
				maxYear = year
			}
		}
	}
	if minYear == 9999 {
		minYear = 0
	}
	return [2]int{minYear, maxYear}
}

func topN(counts map[string]int, n int) string {
	type kv struct {
		Key   string
		Value int
	}
	var sorted []kv
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	var result []string
	for i := 0; i < n && i < len(sorted); i++ {
		result = append(result, sorted[i].Key)
	}
	if len(result) == 0 {
		return "(none)"
	}
	return strings.Join(result, ", ")
}

// ============================================================================
// Generate Wizard Rendering
// ============================================================================

func (m Model) renderGenerateWizard() string {
	// Color palette - vibrant and modern
	pink := lipgloss.Color("205")
	cyan := lipgloss.Color("86")
	purple := lipgloss.Color("99")
	green := lipgloss.Color("42")
	yellow := lipgloss.Color("220")
	red := lipgloss.Color("196")
	white := lipgloss.Color("255")
	gray := lipgloss.Color("245")
	darkGray := lipgloss.Color("238")

	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(pink).
		Bold(true).
		Padding(0, 1).
		Background(lipgloss.Color("236"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(purple).
		Padding(0, 1)

	sectionHeaderStyle := lipgloss.NewStyle().
		Foreground(cyan).
		Bold(true)

	statLabelStyle := lipgloss.NewStyle().Foreground(gray)
	statValueStyle := lipgloss.NewStyle().Foreground(white).Bold(true)
	successStyle := lipgloss.NewStyle().Foreground(green)
	warnStyle := lipgloss.NewStyle().Foreground(yellow)
	errorStyle := lipgloss.NewStyle().Foreground(red)
	dimStyle := lipgloss.NewStyle().Foreground(darkGray)
	highlightStyle := lipgloss.NewStyle().Foreground(pink).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(gray)

	buttonStyle := lipgloss.NewStyle().
		Foreground(white).
		Background(lipgloss.Color("62")).
		Padding(0, 2).
		MarginRight(1)

	selectedButtonStyle := buttonStyle.
		Background(pink).
		Bold(true)

	var builder strings.Builder

	// Header with fancy title
	header := titleStyle.Render("  ✨ Schedule Generation Wizard ✨  ")
	builder.WriteString(header)
	builder.WriteString("\n\n")

	// Content Analysis Section in a box
	if m.contentAnalysis != nil {
		var statsContent strings.Builder
		statsContent.WriteString(sectionHeaderStyle.Render("📊 Library Content"))
		statsContent.WriteString("\n\n")

		// Stats in a nice grid format
		statsContent.WriteString(statLabelStyle.Render("  Total Programs  "))
		statsContent.WriteString(statValueStyle.Render(fmt.Sprintf("%d", m.contentAnalysis.TotalPrograms)))
		statsContent.WriteString("\n")

		// Types with icons
		if len(m.contentAnalysis.Types) > 0 {
			for t, c := range m.contentAnalysis.Types {
				icon := "📄"
				if t == "movie" {
					icon = "🎬"
				} else if t == "episode" {
					icon = "📺"
				}
				statsContent.WriteString(statLabelStyle.Render(fmt.Sprintf("  %s %s  ", icon, t)))
				statsContent.WriteString(statValueStyle.Render(fmt.Sprintf("%d", c)))
				statsContent.WriteString("\n")
			}
		}

		statsContent.WriteString("\n")

		// Genres with visual bar
		if len(m.contentAnalysis.Genres) > 0 {
			statsContent.WriteString(statLabelStyle.Render("  🎭 Genres  "))
			statsContent.WriteString(dimStyle.Render(topN(m.contentAnalysis.Genres, 4)))
			statsContent.WriteString("\n")
		}

		// Ratings
		if len(m.contentAnalysis.Ratings) > 0 {
			statsContent.WriteString(statLabelStyle.Render("  ⭐ Ratings "))
			statsContent.WriteString(dimStyle.Render(topN(m.contentAnalysis.Ratings, 5)))
			statsContent.WriteString("\n")
		}

		// Year range with visual
		if m.contentAnalysis.YearRange[0] > 0 {
			statsContent.WriteString(statLabelStyle.Render("  📅 Years   "))
			statsContent.WriteString(dimStyle.Render(fmt.Sprintf("%d ━━━━━━ %d",
				m.contentAnalysis.YearRange[0], m.contentAnalysis.YearRange[1])))
			statsContent.WriteString("\n")
		}

		builder.WriteString(boxStyle.Render(statsContent.String()))
		builder.WriteString("\n\n")
	}

	// Block Analysis Section
	var blocksContent strings.Builder
	blocksContent.WriteString(sectionHeaderStyle.Render("📋 Scheduling Blocks"))
	blocksContent.WriteString("\n\n")

	if len(m.blockAnalysis) == 0 {
		blocksContent.WriteString(warnStyle.Render("  ⚠ No blocks configured"))
		blocksContent.WriteString("\n")
		blocksContent.WriteString(dimStyle.Render("  Press 'n' to create your first scheduling block"))
		blocksContent.WriteString("\n")
	} else {
		workingCount := 0
		for _, ba := range m.blockAnalysis {
			if ba.CanGenerate {
				workingCount++
			}
		}

		// Summary bar
		summaryColor := green
		if workingCount == 0 {
			summaryColor = red
		} else if workingCount < len(m.blockAnalysis) {
			summaryColor = yellow
		}
		blocksContent.WriteString(lipgloss.NewStyle().Foreground(summaryColor).Render(
			fmt.Sprintf("  %d/%d blocks ready", workingCount, len(m.blockAnalysis))))
		blocksContent.WriteString("\n\n")

		for i, ba := range m.blockAnalysis {
			isSelected := i == m.wizardSelectedIdx && m.wizardSelectedIdx < len(m.blockAnalysis)

			// Status indicator
			var statusIcon string
			var statusStyle lipgloss.Style
			if ba.CanGenerate {
				statusIcon = "● "
				statusStyle = successStyle
			} else {
				statusIcon = "○ "
				statusStyle = errorStyle
			}

			// Selection indicator
			prefix := "  "
			nameStyle := normalStyle
			if isSelected {
				prefix = " ▸"
				nameStyle = highlightStyle
			}

			blocksContent.WriteString(prefix)
			blocksContent.WriteString(statusStyle.Render(statusIcon))
			blocksContent.WriteString(nameStyle.Render(ba.Block.Name))
			if ba.TargetChannel != "" && !strings.Contains(ba.TargetChannel, "not found") {
				blocksContent.WriteString(dimStyle.Render(" → " + ba.TargetChannel))
			}
			blocksContent.WriteString("\n")

			// Match count with mini progress bar
			if ba.CanGenerate {
				bar := strings.Repeat("█", min(ba.MatchingCount/50, 10))
				if bar == "" {
					bar = "▏"
				}
				blocksContent.WriteString(fmt.Sprintf("     %s %s",
					successStyle.Render(bar),
					dimStyle.Render(fmt.Sprintf("%d matches", ba.MatchingCount))))
				blocksContent.WriteString("\n")
			} else {
				blocksContent.WriteString(errorStyle.Render("     ✗ Cannot generate"))
				blocksContent.WriteString("\n")
			}

			// Issues (collapsed, show only first)
			if len(ba.Issues) > 0 && !ba.CanGenerate {
				blocksContent.WriteString(warnStyle.Render(fmt.Sprintf("     └─ %s", ba.Issues[0])))
				blocksContent.WriteString("\n")
				if len(ba.SuggestedFixes) > 0 {
					blocksContent.WriteString(dimStyle.Render(fmt.Sprintf("        💡 %s", ba.SuggestedFixes[0])))
					blocksContent.WriteString("\n")
				}
			}
		}
	}

	builder.WriteString(boxStyle.Render(blocksContent.String()))
	builder.WriteString("\n\n")

	// Actions as fancy buttons
	builder.WriteString(sectionHeaderStyle.Render("⚡ Actions"))
	builder.WriteString("\n\n")

	actions := []struct {
		key   string
		label string
		icon  string
		idx   int
	}{
		{"G", "Generate", "🚀", len(m.blockAnalysis)},
		{"N", "New Block", "➕", len(m.blockAnalysis) + 1},
		{"E", "Edit Block", "✏️", len(m.blockAnalysis) + 2},
	}

	builder.WriteString("  ")
	for _, a := range actions {
		style := buttonStyle
		if m.wizardSelectedIdx == a.idx {
			style = selectedButtonStyle
		}
		builder.WriteString(style.Render(fmt.Sprintf("%s %s [%s]", a.icon, a.label, a.key)))
		builder.WriteString(" ")
	}
	builder.WriteString("\n\n")

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(darkGray).
		Italic(true)
	builder.WriteString(helpStyle.Render("  ↑↓ navigate • enter select • esc back"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

// ============================================================================
// Block Creation Wizard Rendering
// ============================================================================

func (m Model) renderCreateBlock() string {
	// Color palette
	pink := lipgloss.Color("205")
	cyan := lipgloss.Color("86")
	purple := lipgloss.Color("99")
	green := lipgloss.Color("42")
	white := lipgloss.Color("255")
	gray := lipgloss.Color("245")
	darkGray := lipgloss.Color("238")

	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(pink).
		Bold(true).
		Padding(0, 1).
		Background(lipgloss.Color("236"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(purple).
		Padding(1, 2)

	headerStyle := lipgloss.NewStyle().Foreground(cyan).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(gray)
	dimStyle := lipgloss.NewStyle().Foreground(darkGray)
	highlightStyle := lipgloss.NewStyle().Foreground(pink).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(white).Bold(true)

	optionStyle := lipgloss.NewStyle().
		Foreground(gray).
		Padding(0, 1)

	selectedOptionStyle := lipgloss.NewStyle().
		Foreground(white).
		Background(lipgloss.Color("62")).
		Padding(0, 1).
		Bold(true)

	var builder strings.Builder

	// Header
	builder.WriteString(titleStyle.Render("  ➕ Create New Block  "))
	builder.WriteString("\n\n")

	// Progress steps with fancy styling
	steps := []struct {
		name string
		icon string
	}{
		{"Channel", "📺"},
		{"Time", "🕐"},
		{"Duration", "⏱️"},
		{"Filter", "🎯"},
		{"Confirm", "✅"},
	}

	builder.WriteString("  ")
	for i, step := range steps {
		stepStyle := dimStyle
		connector := " ─── "
		if i == m.newBlockStep {
			stepStyle = highlightStyle
		} else if i < m.newBlockStep {
			stepStyle = selectedStyle
		}

		builder.WriteString(stepStyle.Render(fmt.Sprintf("%s %s", step.icon, step.name)))
		if i < len(steps)-1 {
			if i < m.newBlockStep {
				builder.WriteString(selectedStyle.Render(connector))
			} else {
				builder.WriteString(dimStyle.Render(connector))
			}
		}
	}
	builder.WriteString("\n\n")

	// Step content in a box
	var content strings.Builder

	switch m.newBlockStep {
	case 0: // Channel selection
		content.WriteString(headerStyle.Render("📺 Select Target Channel"))
		content.WriteString("\n\n")

		if len(m.channels) == 0 {
			content.WriteString(dimStyle.Render("  No channels available"))
			content.WriteString("\n")
		} else {
			for _, cv := range m.channels {
				isSelected := cv.Channel.ID == m.newBlock.ChannelID
				style := optionStyle
				prefix := "  ○ "
				if isSelected {
					style = selectedOptionStyle
					prefix = "  ● "
				}
				content.WriteString(style.Render(fmt.Sprintf("%s%s", prefix, cv.Channel.Name)))
				content.WriteString("\n")
			}
		}
		content.WriteString("\n")
		content.WriteString(dimStyle.Render("  ↑↓ to select • Enter to continue"))

	case 1: // Time selection
		content.WriteString(headerStyle.Render("🕐 Set Schedule Time"))
		content.WriteString("\n\n")

		content.WriteString(normalStyle.Render("  Current: "))
		if m.newBlock.Cron == "" {
			content.WriteString(dimStyle.Render("not set"))
		} else {
			content.WriteString(valueStyle.Render(m.newBlock.Cron))
		}
		content.WriteString("\n\n")

		content.WriteString(normalStyle.Render("  Quick presets:"))
		content.WriteString("\n")

		presets := []struct {
			key  string
			desc string
			icon string
		}{
			{"1", "6:00 AM daily", "🌅"},
			{"2", "12:00 PM daily", "☀️"},
			{"3", "6:00 PM daily", "🌆"},
			{"4", "8:00 PM daily", "🌙"},
			{"5", "8:00 AM weekends", "🎉"},
			{"C", "Custom cron...", "⚙️"},
		}

		for _, p := range presets {
			content.WriteString(dimStyle.Render(fmt.Sprintf("    %s  [%s] %s", p.icon, p.key, p.desc)))
			content.WriteString("\n")
		}

	case 2: // Duration
		content.WriteString(headerStyle.Render("⏱️ Set Block Duration"))
		content.WriteString("\n\n")

		content.WriteString(normalStyle.Render("  Current: "))
		if m.newBlock.Duration == 0 {
			content.WriteString(dimStyle.Render("not set"))
		} else {
			content.WriteString(valueStyle.Render(formatDuration(m.newBlock.Duration)))
		}
		content.WriteString("\n\n")

		content.WriteString(normalStyle.Render("  Duration presets:"))
		content.WriteString("\n")

		durations := []struct {
			key  string
			desc string
			bar  string
		}{
			{"1", "1 hour", "██"},
			{"2", "2 hours", "████"},
			{"3", "3 hours", "██████"},
			{"4", "4 hours", "████████"},
			{"5", "6 hours", "████████████"},
			{"6", "8 hours", "████████████████"},
		}

		for _, d := range durations {
			content.WriteString(fmt.Sprintf("    %s [%s] %s",
				dimStyle.Render(d.bar),
				highlightStyle.Render(d.key),
				dimStyle.Render(d.desc)))
			content.WriteString("\n")
		}

	case 3: // Content filter
		content.WriteString(headerStyle.Render("🎯 Configure Content Filter"))
		content.WriteString("\n\n")

		if m.contentAnalysis != nil && len(m.contentAnalysis.Genres) > 0 {
			content.WriteString(normalStyle.Render("  Select genres (space to toggle):"))
			content.WriteString("\n\n")

			genres := getSortedKeys(m.contentAnalysis.Genres)
			for i, g := range genres {
				if i >= len(m.newBlockGenres) || i >= 12 { // Limit to 12 genres
					break
				}
				isSelected := m.newBlockGenres[i]
				isFocused := i == m.filterFieldIndex

				checkbox := "○"
				style := dimStyle
				if isSelected {
					checkbox = "●"
					style = selectedStyle
				}
				if isFocused {
					style = highlightStyle
					if isSelected {
						checkbox = "◉"
					} else {
						checkbox = "◎"
					}
				}

				count := m.contentAnalysis.Genres[g]
				content.WriteString(style.Render(fmt.Sprintf("    %s %s", checkbox, g)))
				content.WriteString(dimStyle.Render(fmt.Sprintf(" (%d)", count)))
				content.WriteString("\n")
			}
			if len(genres) > 12 {
				content.WriteString(dimStyle.Render(fmt.Sprintf("\n    ... and %d more genres", len(genres)-12)))
				content.WriteString("\n")
			}
		} else {
			content.WriteString(dimStyle.Render("  No genres available"))
			content.WriteString("\n")
		}
		content.WriteString("\n")
		content.WriteString(dimStyle.Render("  ↑↓ navigate • space toggle • Enter continue"))

	case 4: // Confirm
		content.WriteString(headerStyle.Render("✅ Confirm New Block"))
		content.WriteString("\n\n")

		// Generate name if empty
		blockName := m.newBlock.Name
		if blockName == "" {
			if len(m.newBlock.Filter.Genres) > 0 {
				blockName = strings.Join(m.newBlock.Filter.Genres[:min(2, len(m.newBlock.Filter.Genres))], "/")
			} else {
				blockName = fmt.Sprintf("Block %d", len(m.blocks)+1)
			}
		}

		summaryStyle := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(green).
			Padding(0, 1)

		var summary strings.Builder
		summary.WriteString(fmt.Sprintf("  📝 Name:     %s\n", valueStyle.Render(blockName)))
		summary.WriteString(fmt.Sprintf("  📺 Channel:  %s\n", valueStyle.Render(m.newBlock.ChannelID)))
		summary.WriteString(fmt.Sprintf("  🕐 Schedule: %s\n", valueStyle.Render(m.newBlock.Cron)))
		summary.WriteString(fmt.Sprintf("  ⏱️  Duration: %s\n", valueStyle.Render(formatDuration(m.newBlock.Duration))))
		if len(m.newBlock.Filter.Genres) > 0 {
			summary.WriteString(fmt.Sprintf("  🎭 Genres:   %s\n", valueStyle.Render(strings.Join(m.newBlock.Filter.Genres, ", "))))
		}

		content.WriteString(summaryStyle.Render(summary.String()))
		content.WriteString("\n\n")

		content.WriteString(selectedStyle.Render("  Press Enter to create block"))
	}

	builder.WriteString(boxStyle.Render(content.String()))
	builder.WriteString("\n\n")

	// Help footer
	helpStyle := lipgloss.NewStyle().Foreground(darkGray).Italic(true)
	builder.WriteString(helpStyle.Render("  tab/enter next • shift+tab back • esc cancel"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

func formatDuration(minutes int) string {
	hours := minutes / 60
	mins := minutes % 60
	if hours > 0 && mins > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	} else if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", mins)
}

func getSortedKeys(m map[string]int) []string {
	type kv struct {
		Key   string
		Value int
	}
	var sorted []kv
	for k, v := range m {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})
	var keys []string
	for _, kv := range sorted {
		keys = append(keys, kv.Key)
	}
	return keys
}

// ============================================================================
// Wizard Update Handlers
// ============================================================================

func (m Model) updateGenerateWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxIdx := len(m.blockAnalysis) + 2 // blocks + 3 actions

	switch msg.String() {
	case "q", "esc":
		m.state = stateChannelView
		return m, nil

	case "up", "k":
		if m.wizardSelectedIdx > 0 {
			m.wizardSelectedIdx--
		}
		return m, nil

	case "down", "j":
		if m.wizardSelectedIdx < maxIdx {
			m.wizardSelectedIdx++
		}
		return m, nil

	case "enter":
		return m.handleWizardSelection()

	case "g", "G":
		// Generate with working blocks
		return m.startGenerationFromWizard()

	case "n", "N":
		// Create new block
		return m.startBlockCreation()

	case "e", "E":
		// Edit selected block
		if m.wizardSelectedIdx < len(m.blockAnalysis) {
			m.selected = m.wizardSelectedIdx
			m.state = stateListView
			// Trigger edit
			m.createBlockForm()
			m.state = stateEditBlock
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleWizardSelection() (tea.Model, tea.Cmd) {
	if m.wizardSelectedIdx < len(m.blockAnalysis) {
		// Selected a block - edit it
		m.selected = m.wizardSelectedIdx
		m.state = stateListView
		m.createBlockForm()
		m.state = stateEditBlock
		return m, nil
	}

	actionIdx := m.wizardSelectedIdx - len(m.blockAnalysis)
	switch actionIdx {
	case 0: // Generate
		return m.startGenerationFromWizard()
	case 1: // Create new
		return m.startBlockCreation()
	case 2: // Edit selected
		if len(m.blockAnalysis) > 0 {
			m.selected = 0
			m.state = stateListView
			m.createBlockForm()
			m.state = stateEditBlock
		}
	}

	return m, nil
}

func (m Model) startGenerationFromWizard() (tea.Model, tea.Cmd) {
	// Check if any blocks can generate
	canGenerate := false
	for _, ba := range m.blockAnalysis {
		if ba.CanGenerate {
			canGenerate = true
			break
		}
	}

	if !canGenerate {
		m.statusMessage = "No blocks can generate content. Create a new block or fix existing ones."
		return m, nil
	}

	m.state = stateChannelView
	m.isGenerating = true
	m.statusMessage = "Generating schedule..."
	return m, m.generateScheduleFromProgramsCmd()
}

func (m Model) startBlockCreation() (tea.Model, tea.Cmd) {
	// Initialize new block with defaults
	m.newBlock = scheduler.Block{
		Type:     scheduler.BlockTypeFilter,
		Priority: 10,
	}

	// Set default channel if only one exists
	if len(m.channels) == 1 {
		m.newBlock.ChannelID = m.channels[0].Channel.ID
	}

	// Initialize genre/rating selection arrays
	if m.contentAnalysis != nil {
		m.newBlockGenres = make([]bool, len(m.contentAnalysis.Genres))
		m.newBlockRatings = make([]bool, len(m.contentAnalysis.Ratings))
	}

	m.newBlockStep = 0
	m.state = stateCreateBlock
	return m, nil
}

func (m Model) updateCreateBlock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateGenerateWizard
		return m, nil

	case "tab", "enter":
		// Move to next step
		if m.newBlockStep < 4 {
			// Validate current step before moving
			if m.validateBlockStep() {
				m.newBlockStep++
			}
		} else {
			// Final step - create the block
			return m.finalizeBlockCreation()
		}
		return m, nil

	case "shift+tab":
		if m.newBlockStep > 0 {
			m.newBlockStep--
		}
		return m, nil
	}

	// Handle step-specific input
	switch m.newBlockStep {
	case 0: // Channel selection
		return m.handleChannelSelection(msg)
	case 1: // Time selection
		return m.handleTimeSelection(msg)
	case 2: // Duration
		return m.handleDurationSelection(msg)
	case 3: // Filter
		return m.handleFilterSelection(msg)
	}

	return m, nil
}

func (m Model) validateBlockStep() bool {
	switch m.newBlockStep {
	case 0:
		return m.newBlock.ChannelID != ""
	case 1:
		return m.newBlock.Cron != ""
	case 2:
		return m.newBlock.Duration > 0
	}
	return true
}

func (m Model) handleChannelSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		for i, cv := range m.channels {
			if cv.Channel.ID == m.newBlock.ChannelID && i > 0 {
				m.newBlock.ChannelID = m.channels[i-1].Channel.ID
				break
			}
		}
	case "down", "j":
		for i, cv := range m.channels {
			if cv.Channel.ID == m.newBlock.ChannelID && i < len(m.channels)-1 {
				m.newBlock.ChannelID = m.channels[i+1].Channel.ID
				break
			}
		}
		if m.newBlock.ChannelID == "" && len(m.channels) > 0 {
			m.newBlock.ChannelID = m.channels[0].Channel.ID
		}
	}
	return m, nil
}

func (m Model) handleTimeSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "1":
		m.newBlock.Cron = "0 6 * * *"
	case "2":
		m.newBlock.Cron = "0 12 * * *"
	case "3":
		m.newBlock.Cron = "0 18 * * *"
	case "4":
		m.newBlock.Cron = "0 20 * * *"
	case "5":
		m.newBlock.Cron = "0 8 * * 6,0"
	case "c", "C":
		// Open cron builder
		m.cronExpr = &cronbuilder.Expression{}
		m.cronFieldIndex = 0
		m.prevState = stateCreateBlock
		m.state = stateCronBuilder
	}
	return m, nil
}

func (m Model) handleDurationSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "1":
		m.newBlock.Duration = 60
	case "2":
		m.newBlock.Duration = 120
	case "3":
		m.newBlock.Duration = 180
	case "4":
		m.newBlock.Duration = 240
	case "5":
		m.newBlock.Duration = 360
	case "6":
		m.newBlock.Duration = 480
	}
	return m, nil
}

func (m Model) handleFilterSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.contentAnalysis == nil {
		return m, nil
	}

	genres := getSortedKeys(m.contentAnalysis.Genres)
	maxIdx := len(genres) - 1

	switch msg.String() {
	case "up", "k":
		if m.filterFieldIndex > 0 {
			m.filterFieldIndex--
		}
	case "down", "j":
		if m.filterFieldIndex < maxIdx {
			m.filterFieldIndex++
		}
	case " ":
		// Toggle genre selection
		if m.filterFieldIndex < len(m.newBlockGenres) {
			m.newBlockGenres[m.filterFieldIndex] = !m.newBlockGenres[m.filterFieldIndex]
		}
	}
	return m, nil
}

func (m Model) finalizeBlockCreation() (tea.Model, tea.Cmd) {
	// Build filter from selections
	if m.contentAnalysis != nil {
		genres := getSortedKeys(m.contentAnalysis.Genres)
		for i, selected := range m.newBlockGenres {
			if selected && i < len(genres) {
				m.newBlock.Filter.Genres = append(m.newBlock.Filter.Genres, genres[i])
			}
		}
	}

	// Generate a name if not set
	if m.newBlock.Name == "" {
		m.newBlock.Name = fmt.Sprintf("Block %d", len(m.blocks)+1)
		if len(m.newBlock.Filter.Genres) > 0 {
			m.newBlock.Name = strings.Join(m.newBlock.Filter.Genres[:min(2, len(m.newBlock.Filter.Genres))], "/")
		}
	}

	// Add block to list
	m.blocks = append(m.blocks, m.newBlock)
	m.hasUnsavedChanges = true

	// Update list model
	items := make([]list.Item, len(m.blocks))
	for i, b := range m.blocks {
		items[i] = item{block: b}
	}
	m.list.SetItems(items)

	// Re-analyze blocks
	m.blockAnalysis = m.analyzeBlocks(m.availablePrograms)

	m.statusMessage = fmt.Sprintf("Created block '%s'. Press Ctrl+S to save.", m.newBlock.Name)
	m.state = stateGenerateWizard

	return m, nil
}

