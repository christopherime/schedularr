// Package scheduler provides the core scheduling engine for Schedularr.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/geekxflood/schedularr/internal/tunarr"
	"github.com/robfig/cron/v3"
)

// Engine is the scheduling engine that generates programming schedules.
type Engine struct {
	client        *tunarr.Client
	blocks        []Block
	parser        cron.Parser
	history       *ScheduleHistory
	store         StateStore
	pendingStates map[string]*SeriesState
	logger        *slog.Logger
}

// ScheduledSlot represents a scheduled time slot with its block and priority
type ScheduledSlot struct {
	StartTime time.Time
	EndTime   time.Time
	Block     Block
	Programs  []tunarr.Program
}

// NewEngine creates a new scheduling engine with the given Tunarr client and scheduling blocks.
func NewEngine(client *tunarr.Client, blocks []Block, store StateStore, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		client:        client,
		blocks:        blocks,
		parser:        cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		history:       NewScheduleHistory(7 * 24 * time.Hour), // Track last 7 days by default
		store:         store,
		pendingStates: make(map[string]*SeriesState),
		logger:        logger,
	}
}

// NewEngineWithHistory creates a new scheduling engine with a custom history window
func NewEngineWithHistory(client *tunarr.Client, blocks []Block, historyWindow time.Duration, store StateStore, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		client:        client,
		blocks:        blocks,
		parser:        cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		history:       NewScheduleHistory(historyWindow),
		store:         store,
		pendingStates: make(map[string]*SeriesState),
		logger:        logger,
	}
}

// GenerateForTimeRange generates a schedule for the given window with priority-based conflict resolution.
// It returns a map of ChannelID -> []Program.
func (e *Engine) GenerateForTimeRange(start, end time.Time, availablePrograms []tunarr.Program) (map[string][]tunarr.Program, error) {
	// First, generate all potential slots
	channelSlots := make(map[string][]ScheduledSlot)

	for _, block := range e.blocks {
		scheduleObj, err := e.parser.Parse(block.Cron)
		if err != nil {
			return nil, fmt.Errorf("invalid cron '%s' for block %s: %w", block.Cron, block.Name, err)
		}

		// Find occurrences in range
		nextTime := scheduleObj.Next(start.Add(-1 * time.Second))
		for !nextTime.After(end) {
			// Generate content for this slot
			planned, err := e.PlanBlock(block, availablePrograms)
			if err != nil {
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
	resolvedSchedule := make(map[string][]tunarr.Program)
	for channelID, slots := range channelSlots {
		resolvedSlots := e.resolveConflicts(slots)

		// Flatten slots into program list
		for _, slot := range resolvedSlots {
			resolvedSchedule[channelID] = append(resolvedSchedule[channelID], slot.Programs...)
		}
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
	// Clear pending states after commit
	e.pendingStates = make(map[string]*SeriesState)
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
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no content matches filter for block %s", block.Name)
	}

	// Filter out recently scheduled programs to prevent repetition
	originalCount := len(candidates)
	candidates = e.history.FilterByHistory(candidates, block.ChannelID)

	if len(candidates) < originalCount {
		e.logger.Debug("filtered out recently scheduled programs",
			"filtered_count", originalCount-len(candidates),
			"block_name", block.Name)
	}

	// If we filtered everything out, fall back to all candidates
	// (better to repeat than have no content)
	if len(candidates) == 0 {
		e.logger.Warn("all candidates recently scheduled, allowing repeats",
			"block_name", block.Name)
		candidates, _ = FilterPrograms(availablePrograms, block.Filter)
	}

	var playlist []tunarr.Program
	var currentDuration int64 = 0
	targetDuration := int64(block.Duration) * 60000 // ms

	// Simple random shuffle and fill
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	for _, p := range candidates {
		if currentDuration+p.Duration <= targetDuration {
			playlist = append(playlist, p)
			currentDuration += p.Duration
		}
		if currentDuration >= targetDuration {
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
				currentDuration += f.Duration
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
	e.history.RecordPrograms(playlist, block.ChannelID, block.Name, time.Now())

	return playlist, nil
}

func (e *Engine) planSeriesBlock(block Block, availablePrograms []tunarr.Program) ([]tunarr.Program, error) {
	var playlist []tunarr.Program
	var currentDuration int64
	targetDuration := int64(block.Duration) * 60000 // ms

	for _, seriesConf := range block.Series {
		// Get state
		state, err := e.getSeriesState(seriesConf.ShowTitle)
		if err != nil {
			e.logger.Error("failed to get series state",
				"show_title", seriesConf.ShowTitle,
				"error", err)
			continue
		}

		// Initialize state from config if it's new
		if state.LastAired.IsZero() {
			if seriesConf.StartSeason > 0 {
				state.CurrentSeason = seriesConf.StartSeason
			}
			if seriesConf.StartEpisode > 0 {
				state.CurrentEpisode = seriesConf.StartEpisode
			}
		}

		episodesAdded := 0
		for episodesAdded < seriesConf.EpisodesPerBlock {
			if currentDuration >= targetDuration {
				break
			}

			// Find next episode
			ep := findEpisode(availablePrograms, seriesConf.ShowTitle, state.CurrentSeason, state.CurrentEpisode)
			
			// Handle season rollover if not found
			if ep == nil {
				// Try next season, episode 1
				ep = findEpisode(availablePrograms, seriesConf.ShowTitle, state.CurrentSeason+1, 1)
				if ep != nil {
					state.CurrentSeason++
					state.CurrentEpisode = 1
				} else {
					// Series potentially complete or missing content
					state.Completed = true
					break
				}
			}

			if ep != nil {
				if currentDuration+ep.Duration <= targetDuration {
					playlist = append(playlist, *ep)
					currentDuration += ep.Duration
					state.CurrentEpisode++
					state.LastAired = time.Now()
					episodesAdded++
					
					// Update pending state
					// We need to store a COPY or valid pointer. state is a pointer to e.pendingStates or new from store.
					// If it was new from store, we need to add it to pendingStates.
					e.pendingStates[seriesConf.ShowTitle] = state
				} else {
					// No time for this episode
					break
				}
			}
		}
	}

	// Handle Fallback if time remains
	if currentDuration < targetDuration {
		if block.Fallback.Mode == FallbackModeFiller {
			candidates, err := FilterPrograms(availablePrograms, block.Fallback.FillerFilter)
			if err == nil && len(candidates) > 0 {
				rand.Shuffle(len(candidates), func(i, j int) {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				})
				for _, p := range candidates {
					if currentDuration+p.Duration <= targetDuration {
						playlist = append(playlist, p)
						currentDuration += p.Duration
					}
					if currentDuration >= targetDuration {
						break
					}
				}
			}
		}
		// Redistribute mode is implicit (just moves to next series in loop if implemented that way, 
		// but here we loop sequentially once. Redistribute would mean looping again?)
		// For now, Filler mode is implemented.
	}
	
	// Add gap filling/bumper logic similar to filter block? 
	// The prompt implies Fallback handles it. 
	// But standard "Filler" (bumpers) might still be desired.
	// block.Filler (bumpers) vs block.Fallback (filling empty space).
	// Let's also apply standard filler (bumpers) if configured.
	
	gapDuration := targetDuration - currentDuration
	gapMinutes := int(gapDuration / 60000)
	
	if block.Filler.Enabled && gapMinutes >= block.Filler.MinGapTime {
		fillerPrograms, err := e.getFiller(block, gapDuration)
		if err == nil && len(fillerPrograms) > 0 {
			playlist = append(playlist, fillerPrograms...)
		}
	}

	e.history.RecordPrograms(playlist, block.ChannelID, block.Name, time.Now())
	return playlist, nil
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
		if p.Type == "episode" && p.ShowTitle == title && p.Season == season && p.Episode == episode {
			return p
		}
	}
	return nil
}

// getFiller retrieves filler content to fill the remaining time
func (e *Engine) getFiller(block Block, remainingDuration int64) ([]tunarr.Program, error) {
	if block.Filler.FillerListID == "" {
		return nil, errors.New("no filler list ID specified")
	}

	// Fetch filler content from the specified list
	fillerContent, err := e.client.GetFillerContent(context.Background(), block.Filler.FillerListID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch filler content: %w", err)
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
		if fillerDuration+f.Duration <= maxFillerDuration {
			fillerPlaylist = append(fillerPlaylist, f)
			fillerDuration += f.Duration
		}
		if fillerDuration >= maxFillerDuration {
			break
		}
	}

	return fillerPlaylist, nil
}
