// Package scheduler provides the core scheduling engine for Schedularr.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/robfig/cron/v3"
)

// Engine is the scheduling engine that generates programming schedules.
type Engine struct {
	client         *tunarr.Client
	blocks         []Block
	parser         cron.Parser
	location       *time.Location // Location for cron parsing
	history        *ScheduleHistory
	store          StateStore
	pendingStates  map[string]*SeriesState
	pendingHistory []ScheduleHistoryEntry
	logger         *slog.Logger
}

// ScheduledSlot represents a scheduled time slot with its block and priority
type ScheduledSlot struct {
	StartTime time.Time
	EndTime   time.Time
	Block     Block
	Programs  []tunarr.Program
}

// EngineOptions contains optional configuration for the scheduling engine.
type EngineOptions struct {
	// HistoryWindow bounds both the in-memory dedup check (filterByHistory)
	// and, via Commit's CleanupScheduleHistory call, how much of the
	// persisted schedule_history table survives each apply. service.Runner
	// sets this from the maintenance.history_retention config key so the
	// two stay coherent: GET /history?days=N (api/openapi.yaml, 1..90) can
	// only ever return data as far back as this window allows -- a wider
	// history_retention is required to actually query a wider days range.
	// Zero uses defaultHistoryWindow (7 days).
	HistoryWindow time.Duration
	Logger        *slog.Logger
	Location      *time.Location
}

// defaultHistoryWindow is the schedule-history retention/dedup window used
// when neither NewEngine's caller nor EngineOptions.HistoryWindow specify
// one. It matches cmd/schema/config.cue's maintenance.history_retention
// CUE default (168h / 7 days) -- see EngineOptions's doc comment.
const defaultHistoryWindow = 7 * 24 * time.Hour

// NewEngine creates a new scheduling engine with the given Tunarr client and
// scheduling blocks, using the default 7-day history window. Callers that
// need a configured window (e.g. service.NewRunner, which threads through
// maintenance.history_retention) should use NewEngineWithOptions instead.
func NewEngine(client *tunarr.Client, blocks []Block, store StateStore, logger *slog.Logger, loc *time.Location) *Engine {
	return NewEngineWithOptions(client, blocks, store, EngineOptions{Logger: logger, Location: loc})
}

// NewEngineWithOptions creates a new scheduling engine with optional
// configuration. Any zero-valued field in opts falls back to the same
// default NewEngine uses: opts.Logger -> slog.Default(), opts.Location ->
// time.Local, opts.HistoryWindow -> defaultHistoryWindow (7 days).
func NewEngineWithOptions(client *tunarr.Client, blocks []Block, store StateStore, opts EngineOptions) *Engine {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	historyWindow := opts.HistoryWindow
	if historyWindow == 0 {
		historyWindow = defaultHistoryWindow
	}
	return &Engine{
		client:         client,
		blocks:         blocks,
		parser:         cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
		location:       loc,
		history:        NewScheduleHistory(historyWindow),
		store:          store,
		pendingStates:  make(map[string]*SeriesState),
		pendingHistory: nil,
		logger:         logger,
	}
}

// GenerateForTimeRange generates a schedule for the given window with priority-based conflict resolution.
// It returns a map of ChannelID -> []ScheduledSlot.
func (e *Engine) GenerateForTimeRange(start, end time.Time, availablePrograms []tunarr.Program) (map[string][]ScheduledSlot, error) {
	timer := prometheus.NewTimer(metrics.ScheduleGenerationDurationSeconds)
	defer timer.ObserveDuration()

	// First, generate all potential slots
	channelSlots := make(map[string][]ScheduledSlot)

	for _, block := range e.blocks {
		metrics.SchedulesGeneratedTotal.WithLabelValues(block.ChannelID, block.Name).Inc()
		scheduleObj, err := e.parser.Parse(block.Cron)
		if err != nil {
			metrics.ScheduleErrorsTotal.WithLabelValues("cron_parse_error").Inc()
			return nil, fmt.Errorf("invalid cron '%s' for block %s: %w", block.Cron, block.Name, err)
		}

		// Find occurrences in range
		nextTime := scheduleObj.Next(start.Add(-1 * time.Second))
		for !nextTime.After(end) {
			// Generate content for this slot
			planned, err := e.PlanBlock(block, availablePrograms)
			if err != nil {
				metrics.ScheduleErrorsTotal.WithLabelValues("plan_block_error").Inc()
				return nil, fmt.Errorf("failed to plan block %s: %w", block.Name, err)
			}

			slot := ScheduledSlot{
				StartTime: nextTime,
				EndTime:   nextTime.Add(time.Duration(block.Duration) * time.Minute),
				Block:     block,
				Programs:  planned,
			}

			channelSlots[block.ChannelID] = append(channelSlots[block.ChannelID], slot)

			nextTime = scheduleObj.Next(nextTime)
		}
	}

	// Resolve conflicts using priority
	resolvedSchedule := make(map[string][]ScheduledSlot)
	for channelID, slots := range channelSlots {
		resolvedSlots := e.resolveConflicts(slots)
		resolvedSchedule[channelID] = resolvedSlots
	}

	return resolvedSchedule, nil
}

// Commit persists all pending state changes to the store.
func (e *Engine) Commit() error {
	ctx := context.Background()
	for _, state := range e.pendingStates {
		if err := e.store.UpdateSeriesState(ctx, state); err != nil {
			return fmt.Errorf("failed to update state for %s: %w", state.ShowTitle, err)
		}
	}
	if len(e.pendingHistory) > 0 {
		if err := e.store.RecordScheduleHistory(ctx, e.pendingHistory); err != nil {
			return fmt.Errorf("failed to record schedule history: %w", err)
		}
	}
	if _, err := e.store.CleanupScheduleHistory(ctx, e.history.Window()); err != nil {
		return fmt.Errorf("failed to cleanup schedule history: %w", err)
	}
	// Clear pending states after commit
	e.pendingStates = make(map[string]*SeriesState)
	e.pendingHistory = nil
	return nil
}

// resolveConflicts resolves overlapping slots by priority
func (e *Engine) resolveConflicts(slots []ScheduledSlot) []ScheduledSlot {
	if len(slots) == 0 {
		return slots
	}

	var resolved []ScheduledSlot
	conflicts := 0

	for i := range slots {
		shouldInclude := true

		// Check against already resolved slots
		for j := range resolved {
			if slotsOverlap(slots[i], resolved[j]) {
				conflicts++
				// Higher priority wins (higher number = higher priority)
				if slots[i].Block.Priority > resolved[j].Block.Priority {
					metrics.ConflictsResolvedTotal.Inc()
					// Remove the lower priority slot and add the higher one
					e.logger.Info("scheduling conflict resolved by priority",
						"winner_block", slots[i].Block.Name,
						"winner_priority", slots[i].Block.Priority,
						"loser_block", resolved[j].Block.Name,
						"loser_priority", resolved[j].Block.Priority,
						"start_time", slots[i].StartTime.Format("2006-01-02 15:04"))
					resolved = append(resolved[:j], resolved[j+1:]...)
					break
				} else {
					e.logger.Info("scheduling conflict - block blocked by higher priority",
						"blocked_block", slots[i].Block.Name,
						"blocked_priority", slots[i].Block.Priority,
						"blocking_block", resolved[j].Block.Name,
						"blocking_priority", resolved[j].Block.Priority,
						"start_time", slots[i].StartTime.Format("2006-01-02 15:04"))
					shouldInclude = false
					break
				}
			}
		}

		if shouldInclude {
			resolved = append(resolved, slots[i])
		}
	}

	if conflicts > 0 {
		e.logger.Info("resolved scheduling conflicts using priority", "conflict_count", conflicts)
	}

	return resolved
}

// slotsOverlap returns true if two slots overlap in time
func slotsOverlap(a, b ScheduledSlot) bool {
	return a.StartTime.Before(b.EndTime) && a.EndTime.After(b.StartTime)
}

// PlanBlock generates a list of programs to fill the block's duration
func (e *Engine) PlanBlock(block Block, availablePrograms []tunarr.Program) ([]tunarr.Program, error) {
	if block.Type == BlockTypeSeries {
		return e.planSeriesBlock(block, availablePrograms)
	}
	return e.planFilterBlock(block, availablePrograms)
}

func (e *Engine) planFilterBlock(block Block, availablePrograms []tunarr.Program) ([]tunarr.Program, error) {
	candidates, err := FilterPrograms(availablePrograms, block.Filter)
	if err != nil {
		metrics.ScheduleErrorsTotal.WithLabelValues("filter_programs_error").Inc()
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no content matches filter for block %s", block.Name)
	}

	// Filter out recently scheduled programs to prevent repetition
	originalCount := len(candidates)
	candidates = e.filterByHistory(candidates, block.ChannelID)

	if len(candidates) < originalCount {
		e.logger.Debug("filtered out recently scheduled programs",
			"filtered_count", originalCount-len(candidates),
			"block_name", block.Name)
	}

	// If we filtered everything out, fall back to all candidates
	// (better to repeat than have no content)
	if len(candidates) == 0 {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "history_exhausted").Inc()
		e.logger.Warn("all candidates recently scheduled, allowing repeats",
			"block_name", block.Name,
			"original_count", originalCount)
		var filterErr error
		candidates, filterErr = FilterPrograms(availablePrograms, block.Filter)
		if filterErr != nil {
			e.logger.Error("failed to re-filter programs during fallback",
				"block_name", block.Name,
				"error", filterErr)
			return nil, fmt.Errorf("history fallback failed for block %s: %w", block.Name, filterErr)
		}
	}

	var playlist []tunarr.Program
	var currentDuration int64 = 0
	targetDuration := int64(block.Duration) * 60000 // ms

	maxOverflowMs := int64(block.MaxDurationOverflowMinutes) * time.Minute.Milliseconds()
	allowedDurationWithOverflow := targetDuration + maxOverflowMs

	// Simple random shuffle and fill
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	for _, p := range candidates {
		if currentDuration+p.GetDurationMs() <= allowedDurationWithOverflow {
			playlist = append(playlist, p)
			currentDuration += p.GetDurationMs()
			metrics.ProgramsScheduledTotal.WithLabelValues(block.ChannelID, block.Name, p.Type).Inc()
		} else {
			break
		}
	}

	// Gap filling with filler content
	gapDuration := targetDuration - currentDuration
	gapMinutes := int(gapDuration / 60000)

	// Check if we should add filler content
	if block.Filler.Enabled && gapMinutes >= block.Filler.MinGapTime {
		fillerPrograms, err := e.getFiller(block, gapDuration)
		if err != nil {
			e.logger.Warn("failed to get filler for block",
				"block_name", block.Name,
				"error", err)
		} else if len(fillerPrograms) > 0 {
			e.logger.Info("adding filler programs to fill gap",
				"filler_count", len(fillerPrograms),
				"gap_minutes", gapMinutes,
				"block_name", block.Name)
			playlist = append(playlist, fillerPrograms...)
			for _, f := range fillerPrograms {
				currentDuration += f.GetDurationMs()
			}
		}
	}

	// Log if there's still a significant gap
	finalGap := targetDuration - currentDuration
	finalGapMinutes := int(finalGap / 60000)
	if finalGapMinutes > 5 {
		e.logger.Info("block has remaining gap after filling",
			"block_name", block.Name,
			"gap_minutes", finalGapMinutes)
	}

	// Record scheduled programs in history
	e.recordHistory(playlist, block.ChannelID, block.Name, time.Now())

	return playlist, nil
}

func (e *Engine) planSeriesBlock(block Block, availablePrograms []tunarr.Program) ([]tunarr.Program, error) {
	var playlist []tunarr.Program
	var currentDuration int64
	targetDuration := int64(block.Duration) * 60000 // ms

	for _, seriesConf := range block.Series {
		seriesPlaylist, durationAdded := e.planSeriesForConfig(block, seriesConf, availablePrograms, targetDuration-currentDuration)
		playlist = append(playlist, seriesPlaylist...)
		currentDuration += durationAdded

		if currentDuration >= targetDuration {
			break
		}
	}

	playlist, currentDuration = e.applySeriesFallback(block, availablePrograms, playlist, currentDuration, targetDuration)
	playlist, _ = e.applyBlockFiller(block, playlist, currentDuration, targetDuration)

	e.recordHistory(playlist, block.ChannelID, block.Name, time.Now())
	return playlist, nil
}

func (e *Engine) planSeriesForConfig(block Block, seriesConf SeriesConfig, availablePrograms []tunarr.Program, remainingDuration int64) ([]tunarr.Program, int64) {
	if remainingDuration <= 0 {
		return nil, 0
	}

	state, err := e.getSeriesState(seriesConf.ShowTitle)
	if err != nil {
		metrics.ScheduleErrorsTotal.WithLabelValues("get_series_state_error").Inc()
		e.logger.Error("failed to get series state",
			"show_title", seriesConf.ShowTitle,
			"error", err)
		return nil, 0
	}

	// Skip disabled series
	if state.Disabled {
		e.logger.Debug("skipping disabled series",
			"show_title", seriesConf.ShowTitle)
		return nil, 0
	}

	e.initializeSeriesState(state, seriesConf)

	var playlist []tunarr.Program
	var currentDuration int64
	episodesAdded := 0

	maxOverflowMs := int64(block.MaxDurationOverflowMinutes) * time.Minute.Milliseconds()
	allowedDurationWithOverflow := remainingDuration + maxOverflowMs

	for episodesAdded < seriesConf.EpisodesPerBlock {
		ep := e.findNextSeriesEpisode(seriesConf, state, availablePrograms)
		if ep == nil {
			break
		}

		if currentDuration+ep.GetDurationMs() <= allowedDurationWithOverflow {
			playlist = append(playlist, *ep)
			currentDuration += ep.GetDurationMs()
			state.CurrentEpisode++
			now := time.Now()
			state.LastAired = &now
			episodesAdded++
			e.pendingStates[seriesConf.ShowTitle] = state
			metrics.ProgramsScheduledTotal.WithLabelValues(block.ChannelID, block.Name, ep.Type).Inc()
			metrics.SeriesStateUpdatesTotal.WithLabelValues(seriesConf.ShowTitle).Inc()
		} else {
			break
		}
	}

	return playlist, currentDuration
}

func (e *Engine) initializeSeriesState(state *SeriesState, seriesConf SeriesConfig) {
	if state.LastAired != nil && !state.LastAired.IsZero() {
		return
	}

	if seriesConf.StartSeason > 0 {
		state.CurrentSeason = seriesConf.StartSeason
	}
	if seriesConf.StartEpisode > 0 {
		state.CurrentEpisode = seriesConf.StartEpisode
	}
}

func (e *Engine) findNextSeriesEpisode(seriesConf SeriesConfig, state *SeriesState, availablePrograms []tunarr.Program) *tunarr.Program {
	// Skip episodes if configured
	for e.shouldSkipEpisode(seriesConf, state) {
		e.logger.Debug("skipping episode",
			"show_title", seriesConf.ShowTitle,
			"season", state.CurrentSeason,
			"episode", state.CurrentEpisode)
		state.CurrentEpisode++
	}

	ep := findEpisode(availablePrograms, seriesConf.ShowTitle, state.CurrentSeason, state.CurrentEpisode)
	if ep != nil {
		return ep
	}

	// Try next season
	ep = findEpisode(availablePrograms, seriesConf.ShowTitle, state.CurrentSeason+1, 1)
	if ep != nil {
		state.CurrentSeason++
		state.CurrentEpisode = 1
		// Check if first episode of new season should be skipped
		if e.shouldSkipEpisode(seriesConf, state) {
			return e.findNextSeriesEpisode(seriesConf, state, availablePrograms)
		}
		return ep
	}

	e.markSeriesCompleted(seriesConf, state)
	return nil
}

func (e *Engine) shouldSkipEpisode(seriesConf SeriesConfig, state *SeriesState) bool {
	if len(seriesConf.SkipEpisodes) == 0 {
		return false
	}

	episodeID := fmt.Sprintf("S%02dE%02d", state.CurrentSeason, state.CurrentEpisode)
	for _, skip := range seriesConf.SkipEpisodes {
		if skip == episodeID {
			return true
		}
	}
	return false
}

func (e *Engine) markSeriesCompleted(seriesConf SeriesConfig, state *SeriesState) {
	if !state.Completed {
		e.logger.Info("series completed all episodes",
			"show_title", seriesConf.ShowTitle,
			"season", state.CurrentSeason,
			"episode", state.CurrentEpisode,
			"on_complete", seriesConf.OnComplete)
	}

	state.Completed = true
	state.RunCount++
	metrics.SeriesStateUpdatesTotal.WithLabelValues(seriesConf.ShowTitle).Inc()

	// Handle completion action
	action := seriesConf.OnComplete
	if action == "" {
		action = CompletionActionContinue // Default
	}

	switch action {
	case CompletionActionRestart:
		// Check if max runs is set and exceeded
		if seriesConf.MaxRuns > 0 && state.RunCount >= seriesConf.MaxRuns {
			e.logger.Info("series reached max runs, disabling",
				"show_title", seriesConf.ShowTitle,
				"run_count", state.RunCount,
				"max_runs", seriesConf.MaxRuns)
			state.Disabled = true
		} else {
			e.logger.Info("restarting series from beginning",
				"show_title", seriesConf.ShowTitle,
				"run_count", state.RunCount)
			state.CurrentSeason = 1
			state.CurrentEpisode = 1
			state.Completed = false
		}

	case CompletionActionDisable:
		e.logger.Info("disabling completed series",
			"show_title", seriesConf.ShowTitle)
		state.Disabled = true

	case CompletionActionContinue:
		// Do nothing, just mark as completed
		e.logger.Debug("series marked as completed, will continue in block",
			"show_title", seriesConf.ShowTitle)
	}

	e.pendingStates[seriesConf.ShowTitle] = state
}

func (e *Engine) applySeriesFallback(block Block, availablePrograms []tunarr.Program, playlist []tunarr.Program, currentDuration, targetDuration int64) ([]tunarr.Program, int64) {
	if currentDuration >= targetDuration {
		return playlist, currentDuration
	}

	if block.Fallback.Mode != FallbackModeFiller {
		return playlist, currentDuration
	}

	candidates, err := FilterPrograms(availablePrograms, block.Fallback.FillerFilter)
	if err != nil {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "fallback_filter_error").Inc()
		e.logger.Warn("series fallback filter failed",
			"block_name", block.Name,
			"error", err)
		return playlist, currentDuration
	}
	if len(candidates) == 0 {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "fallback_no_content").Inc()
		e.logger.Debug("no content matches series fallback filter",
			"block_name", block.Name,
			"gap_minutes", int((targetDuration-currentDuration)/60000))
		return playlist, currentDuration
	}

	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	for _, p := range candidates {
		if currentDuration+p.GetDurationMs() <= targetDuration {
			playlist = append(playlist, p)
			currentDuration += p.GetDurationMs()
		}
		if currentDuration >= targetDuration {
			break
		}
	}

	return playlist, currentDuration
}

func (e *Engine) applyBlockFiller(block Block, playlist []tunarr.Program, currentDuration, targetDuration int64) ([]tunarr.Program, int64) {
	gapDuration := targetDuration - currentDuration
	gapMinutes := int(gapDuration / 60000)
	if !block.Filler.Enabled || gapMinutes < block.Filler.MinGapTime {
		return playlist, currentDuration
	}

	fillerPrograms, err := e.getFiller(block, gapDuration)
	if err != nil {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "filler_fetch_error").Inc()
		e.logger.Warn("failed to fetch filler content for series block",
			"block_name", block.Name,
			"filler_list_id", block.Filler.FillerListID,
			"gap_minutes", gapMinutes,
			"error", err)
		return playlist, currentDuration
	}
	if len(fillerPrograms) == 0 {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "filler_empty").Inc()
		e.logger.Debug("no filler programs fit remaining gap",
			"block_name", block.Name,
			"gap_minutes", gapMinutes)
		return playlist, currentDuration
	}

	playlist = append(playlist, fillerPrograms...)
	for _, filler := range fillerPrograms {
		currentDuration += filler.GetDurationMs()
	}

	return playlist, currentDuration
}

func (e *Engine) getSeriesState(title string) (*SeriesState, error) {
	if state, ok := e.pendingStates[title]; ok {
		return state, nil
	}
	return e.store.GetSeriesState(context.Background(), title)
}

func findEpisode(programs []tunarr.Program, title string, season, episode int) *tunarr.Program {
	for i := range programs {
		p := &programs[i]
		if p.Type == "episode" && p.ShowTitle == title && p.SeasonNumber == season && p.EpisodeNumber == episode {
			return p
		}
	}
	return nil
}

func (e *Engine) recordHistory(programs []tunarr.Program, channelID, blockName string, scheduledAt time.Time) {
	e.history.RecordPrograms(programs, channelID, blockName, scheduledAt)
	e.pendingHistory = append(e.pendingHistory, makeHistoryEntries(programs, channelID, blockName, scheduledAt)...)
}

func makeHistoryEntries(programs []tunarr.Program, channelID, blockName string, scheduledAt time.Time) []ScheduleHistoryEntry {
	entries := make([]ScheduleHistoryEntry, 0, len(programs))
	for _, program := range programs {
		entries = append(entries, ScheduleHistoryEntry{
			ProgramID:   program.GetID(),
			ChannelID:   channelID,
			BlockName:   blockName,
			ScheduledAt: scheduledAt,
		})
	}
	return entries
}

func (e *Engine) filterByHistory(programs []tunarr.Program, channelID string) []tunarr.Program {
	filtered := make([]tunarr.Program, 0, len(programs))
	window := e.history.Window()
	ctx := context.Background()

	for _, program := range programs {
		programID := program.GetID()
		if e.history.WasRecentlyScheduled(programID, channelID) {
			continue
		}
		if e.store != nil {
			recent, err := e.store.WasRecentlyScheduled(ctx, programID, channelID, window)
			if err != nil {
				e.logger.Warn("failed to check schedule history",
					"program_id", programID,
					"channel_id", channelID,
					"error", err)
			} else if recent {
				continue
			}
		}
		filtered = append(filtered, program)
	}

	return filtered
}

// getFiller retrieves filler content to fill the remaining time
func (e *Engine) getFiller(block Block, remainingDuration int64) ([]tunarr.Program, error) {
	if block.Filler.FillerListID == "" {
		return nil, errors.New("no filler list ID specified")
	}

	// Fetch filler programs from the specified list
	fillerContent, err := e.client.GetFillerPrograms(context.Background(), block.Filler.FillerListID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch filler programs: %w", err)
	}

	if len(fillerContent) == 0 {
		return nil, fmt.Errorf("filler list %s is empty", block.Filler.FillerListID)
	}

	var fillerPlaylist []tunarr.Program
	var fillerDuration int64 = 0
	maxFillerDuration := remainingDuration

	// If max filler time is set, respect it
	if block.Filler.MaxFillerTime > 0 {
		maxFillerMs := int64(block.Filler.MaxFillerTime) * 60000
		if maxFillerMs < remainingDuration {
			maxFillerDuration = maxFillerMs
		}
	}

	// Shuffle filler content for variety
	rand.Shuffle(len(fillerContent), func(i, j int) {
		fillerContent[i], fillerContent[j] = fillerContent[j], fillerContent[i]
	})

	// Fill remaining time with filler content
	for _, f := range fillerContent {
		if fillerDuration+f.GetDurationMs() <= maxFillerDuration {
			fillerPlaylist = append(fillerPlaylist, f)
			fillerDuration += f.GetDurationMs()
		}
		if fillerDuration >= maxFillerDuration {
			break
		}
	}

	return fillerPlaylist, nil
}
