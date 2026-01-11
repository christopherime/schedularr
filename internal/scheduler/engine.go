// Package scheduler provides the core scheduling engine for Schedularr.
package scheduler

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/geekxflood/schedularr/internal/tunarr"
	"github.com/robfig/cron/v3"
)

// Engine is the scheduling engine that generates programming schedules.
type Engine struct {
	client *tunarr.Client
	blocks []Block
	parser cron.Parser
}

// ScheduledSlot represents a scheduled time slot with its block and priority
type ScheduledSlot struct {
	StartTime time.Time
	EndTime   time.Time
	Block     Block
	Programs  []tunarr.Program
}

// NewEngine creates a new scheduling engine with the given Tunarr client and scheduling blocks.
func NewEngine(client *tunarr.Client, blocks []Block) *Engine {
	return &Engine{
		client: client,
		blocks: blocks,
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
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
		resolvedSlots := resolveConflicts(slots)

		// Flatten slots into program list
		for _, slot := range resolvedSlots {
			resolvedSchedule[channelID] = append(resolvedSchedule[channelID], slot.Programs...)
		}
	}

	return resolvedSchedule, nil
}

// resolveConflicts resolves overlapping slots by priority
func resolveConflicts(slots []ScheduledSlot) []ScheduledSlot {
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
					log.Printf("Conflict: '%s' (priority %d) overrides '%s' (priority %d) at %s",
						slots[i].Block.Name, slots[i].Block.Priority,
						resolved[j].Block.Name, resolved[j].Block.Priority,
						slots[i].StartTime.Format("2006-01-02 15:04"))
					resolved = append(resolved[:j], resolved[j+1:]...)
					break
				} else {
					log.Printf("Conflict: '%s' (priority %d) blocked by '%s' (priority %d) at %s",
						slots[i].Block.Name, slots[i].Block.Priority,
						resolved[j].Block.Name, resolved[j].Block.Priority,
						slots[i].StartTime.Format("2006-01-02 15:04"))
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
		log.Printf("Resolved %d scheduling conflict(s) using priority", conflicts)
	}

	return resolved
}

// slotsOverlap returns true if two slots overlap in time
func slotsOverlap(a, b ScheduledSlot) bool {
	return a.StartTime.Before(b.EndTime) && a.EndTime.After(b.StartTime)
}

// PlanBlock generates a list of programs to fill the block's duration
func (e *Engine) PlanBlock(block Block, availablePrograms []tunarr.Program) ([]tunarr.Program, error) {
	candidates, err := FilterPrograms(availablePrograms, block.Filter)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no content matches filter for block %s", block.Name)
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
		// Relaxed fit? Or strict?
		// If strict, we might stop. If we want to fill exactly, we need a better algo (knapsack).
		// For TV, usually "close enough" or filler.
		if currentDuration >= targetDuration {
			break
		}
	}

	return playlist, nil
}
