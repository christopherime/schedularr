package scheduler

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/geekxflood/schedularr/internal/tunarr"
	"github.com/robfig/cron/v3"
)

type Engine struct {
	client *tunarr.Client
	blocks []Block
	parser cron.Parser
}

func NewEngine(client *tunarr.Client, blocks []Block) *Engine {
	return &Engine{
		client: client,
		blocks: blocks,
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// GenerateForTimeRange generates a schedule for the given window.
// It returns a map of ChannelID -> []Program.
func (e *Engine) GenerateForTimeRange(start, end time.Time, availablePrograms []tunarr.Program) (map[string][]tunarr.Program, error) {
	schedule := make(map[string][]tunarr.Program)

	for _, block := range e.blocks {
		scheduleObj, err := e.parser.Parse(block.Cron)
		if err != nil {
			return nil, fmt.Errorf("invalid cron '%s' for block %s: %w", block.Cron, block.Name, err)
		}

		// Find occurrences in range
		nextTime := scheduleObj.Next(start.Add(-1 * time.Second))
		for {
			if nextTime.After(end) {
				break
			}

			// Generate content for this slot
			planned, err := e.PlanBlock(block, availablePrograms)
			if err != nil {
				return nil, fmt.Errorf("failed to plan block %s: %w", block.Name, err)
			}

			schedule[block.ChannelID] = append(schedule[block.ChannelID], planned...)

			nextTime = scheduleObj.Next(nextTime)
		}
	}

	return schedule, nil
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
