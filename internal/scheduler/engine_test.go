package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlotsOverlap(t *testing.T) {
	tests := []struct {
		name     string
		slot1    ScheduledSlot
		slot2    ScheduledSlot
		expected bool
	}{
		{
			name: "overlapping slots",
			slot1: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC),
			},
			slot2: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 13, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
		{
			name: "non-overlapping slots",
			slot1: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
			},
			slot2: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC),
			},
			expected: false,
		},
		{
			name: "slot1 contains slot2",
			slot1: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 14, 0, 0, 0, time.UTC),
			},
			slot2: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 13, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slotsOverlap(tt.slot1, tt.slot2)
			assert.Equal(t, tt.expected, result, "slotsOverlap result mismatch")
		})
	}
}

func TestResolveConflicts(t *testing.T) {
	block1 := Block{Name: "Low Priority", Priority: 10}
	block2 := Block{Name: "High Priority", Priority: 20}

	tests := []struct {
		name            string
		slots           []ScheduledSlot
		expected        int
		expectedDropped int
	}{
		{
			name:     "no conflicts",
			slots:    []ScheduledSlot{},
			expected: 0,
		},
		{
			name: "two non-overlapping slots",
			slots: []ScheduledSlot{
				{
					StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
					Block:     block1,
				},
				{
					StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC),
					Block:     block2,
				},
			},
			expected:        2,
			expectedDropped: 0,
		},
		{
			name: "two overlapping slots - high priority wins",
			slots: []ScheduledSlot{
				{
					StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC),
					Block:     block1,
				},
				{
					StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 11, 13, 0, 0, 0, time.UTC),
					Block:     block2,
				},
			},
			expected:        1, // Only high priority should remain
			expectedDropped: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal engine to call the method
			engine := &Engine{logger: slog.Default()}
			resolved, dropped := engine.resolveConflicts(tt.slots)
			assert.Len(t, resolved, tt.expected, "resolveConflicts returned wrong number of slots")
			assert.Len(t, dropped, tt.expectedDropped, "resolveConflicts returned wrong number of warnings")

			// If there were conflicts, verify high priority won and the
			// warning correctly names the loser and its blocker.
			if tt.name == "two overlapping slots - high priority wins" && len(resolved) > 0 {
				assert.Equal(t, 20, resolved[0].Block.Priority, "Expected high priority block to win")
				require.Len(t, dropped, 1)
				assert.Equal(t, "Low Priority", dropped[0].BlockName)
				assert.Equal(t, "High Priority", dropped[0].BlockingBlockName)
				assert.Equal(t, block1.Name, dropped[0].BlockName)
				assert.True(t, dropped[0].OccurrenceStart.Equal(time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC)))
			}
		})
	}
}

func TestPlanBlock_WithoutFiller(t *testing.T) {
	client := &tunarr.Client{}
	engine := NewEngine(client, []Block{}, NewMockStateStore(), slog.Default(), time.UTC)

	block := Block{
		Name:     "Test Block",
		Duration: 60, // 60 minutes
		Filter: Filter{
			Genres: []string{"Comedy"},
		},
	}

	availablePrograms := []tunarr.Program{
		{
			ID:       "prog1",
			Title:    "Show A",
			Duration: 1800000, // 30 minutes
			Genres:   []tunarr.Genre{{Name: "Comedy"}},
			Type:     "episode",
		},
		{
			ID:       "prog2",
			Title:    "Show B",
			Duration: 1800000, // 30 minutes
			Genres:   []tunarr.Genre{{Name: "Comedy"}},
			Type:     "episode",
		},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")
	require.Len(t, playlist, 2, "Expected 2 programs in playlist")

	// Check total duration
	var totalDuration int64
	for _, p := range playlist {
		totalDuration += p.GetDurationMs()
	}

	// Should be 60 minutes (3600000 ms)
	assert.Equal(t, int64(3600000), totalDuration, "Expected total duration 3600000 ms")
}

func TestPlanBlock_NoMatchingContent(t *testing.T) {
	client := &tunarr.Client{}
	engine := NewEngine(client, []Block{}, NewMockStateStore(), slog.Default(), time.UTC)

	block := Block{
		Name:     "Test Block",
		Duration: 60,
		Filter: Filter{
			Genres: []string{"SciFi"},
		},
	}

	availablePrograms := []tunarr.Program{
		{
			ID:       "prog1",
			Title:    "Show A",
			Duration: 1800000,
			Genres:   []tunarr.Genre{{Name: "Comedy"}},
			Type:     "episode",
		},
	}

	_, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	assert.Error(t, err, "Expected error when no content matches filter")
}

func TestPlanBlock_UsesStoreHistory(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	store.History = []ScheduleHistoryEntry{
		{
			ProgramID:   "prog-1",
			ChannelID:   "channel-1",
			BlockName:   "Recent Block",
			ScheduledAt: time.Now(),
		},
	}
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Name:      "History Block",
		Duration:  30,
		ChannelID: "channel-1",
		Filter:    Filter{},
	}

	availablePrograms := []tunarr.Program{
		{ID: "prog-1", Title: "Recent", Duration: 1800000, Type: "movie"},
		{ID: "prog-2", Title: "Fresh", Duration: 1800000, Type: "movie"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")
	require.Len(t, playlist, 1, "Expected 1 program")
	assert.Equal(t, "prog-2", playlist[0].ID, "Expected prog-2 after filtering")
}

func TestCommit_CleansUpScheduleHistory(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	store.History = []ScheduleHistoryEntry{
		{
			ProgramID:   "old-prog",
			ChannelID:   "channel-1",
			BlockName:   "Old Block",
			ScheduledAt: time.Now().Add(-48 * time.Hour),
		},
		{
			ProgramID:   "new-prog",
			ChannelID:   "channel-1",
			BlockName:   "New Block",
			ScheduledAt: time.Now(),
		},
	}

	engine := NewEngineWithOptions(client, []Block{}, store, EngineOptions{
		HistoryWindow: 24 * time.Hour,
		Logger:        slog.Default(),
		Location:      time.UTC,
	})

	require.NoError(t, engine.Commit(), "Commit returned error")
	require.Len(t, store.History, 1, "Expected 1 history entry after cleanup")
	assert.Equal(t, "new-prog", store.History[0].ProgramID, "Expected new-prog to remain")
}

func TestPlanBlock_Series(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 120,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Show A",
				EpisodesPerBlock: 2,
			},
		},
		Fallback: SeriesFallback{
			Mode: "none", // Disable fallback to test series logic only
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Ep 1", ShowTitle: "Show A", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Ep 2", ShowTitle: "Show A", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1800000, Type: "episode"},
		{ID: "p3", Title: "Ep 3", ShowTitle: "Show A", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1800000, Type: "episode"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")
	require.Len(t, playlist, 2, "Expected 2 episodes")
	assert.Equal(t, 1, playlist[0].EpisodeNumber, "Expected Ep 1")
	assert.Equal(t, 2, playlist[1].EpisodeNumber, "Expected Ep 2")

	// Verify pending state
	state, ok := engine.pendingStates["Show A"]
	require.True(t, ok, "Expected pending state for Show A")
	assert.Equal(t, 3, state.CurrentEpisode, "Expected next episode to be 3")
}

func TestPlanBlock_SeriesMarksCompleteWhenMissing(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Missing Show",
				EpisodesPerBlock: 1,
			},
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Other Show", ShowTitle: "Other Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000, Type: "episode"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")
	require.Empty(t, playlist, "Expected empty playlist")

	state, ok := engine.pendingStates["Missing Show"]
	require.True(t, ok, "Expected pending state for Missing Show")
	assert.True(t, state.Completed, "Expected series to be marked completed")
}

func TestSeriesCompletion_Restart(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Set up initial state at end of series
	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  2,
		CurrentEpisode: 10,
		Completed:      false,
		RunCount:       0,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 1,
				OnComplete:       CompletionActionRestart,
			},
		},
	}

	// No more episodes available - should trigger completion
	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Test Show S01E01", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000, Type: "episode"},
	}

	_, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")

	state, ok := engine.pendingStates["Test Show"]
	require.True(t, ok, "Expected pending state for Test Show")

	assert.False(t, state.Completed, "Expected series to be restarted, not marked completed")
	assert.Equal(t, 1, state.CurrentSeason, "Expected season to be reset to 1")
	assert.Equal(t, 1, state.CurrentEpisode, "Expected episode to be reset to 1")
	assert.Equal(t, 1, state.RunCount, "Expected run count to be 1")
}

func TestSeriesCompletion_Disable(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 10,
		Completed:      false,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 1,
				OnComplete:       CompletionActionDisable,
			},
		},
	}

	availablePrograms := []tunarr.Program{}

	_, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")

	state, ok := engine.pendingStates["Test Show"]
	require.True(t, ok, "Expected pending state for Test Show")

	assert.True(t, state.Completed, "Expected series to be marked completed")
	assert.True(t, state.Disabled, "Expected series to be disabled")
}

func TestSeriesCompletion_MaxRuns(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Series has already run twice
	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 10,
		Completed:      false,
		RunCount:       2,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 1,
				OnComplete:       CompletionActionRestart,
				MaxRuns:          3,
			},
		},
	}

	availablePrograms := []tunarr.Program{}

	_, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")

	state, ok := engine.pendingStates["Test Show"]
	require.True(t, ok, "Expected pending state for Test Show")

	assert.Equal(t, 3, state.RunCount, "Expected run count to be 3")
	assert.True(t, state.Disabled, "Expected series to be disabled after reaching max runs")
}

func TestSeriesEpisodeSkipping(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		Completed:      false,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 90, // 90 minutes
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 3,
				SkipEpisodes:     []string{"S01E02", "S01E04"},
			},
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Test Show S01E01", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Test Show S01E02", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1800000, Type: "episode"},
		{ID: "p3", Title: "Test Show S01E03", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1800000, Type: "episode"},
		{ID: "p4", Title: "Test Show S01E04", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 4, Duration: 1800000, Type: "episode"},
		{ID: "p5", Title: "Test Show S01E05", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 5, Duration: 1800000, Type: "episode"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")

	// Should get E01, skip E02, get E03, get E05 (skipping E02 and E04)
	require.Len(t, playlist, 3, "Expected 3 episodes (E01, E03, E05 - skipping E02 and E04)")

	assert.Equal(t, 1, playlist[0].EpisodeNumber, "Expected first episode to be E01")
	assert.Equal(t, 3, playlist[1].EpisodeNumber, "Expected second episode to be E03 (skipped E02)")
	assert.Equal(t, 5, playlist[2].EpisodeNumber, "Expected third episode to be E05 (skipped E04)")

	state, ok := engine.pendingStates["Test Show"]
	require.True(t, ok, "Expected pending state for Test Show")

	// Should be at E06 (next episode after E05)
	assert.Equal(t, 6, state.CurrentEpisode, "Expected current episode to be 6")
}

func TestSeriesSkipDisabled(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Series is disabled
	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		Disabled:       true,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 1,
			},
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Test Show S01E01", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000, Type: "episode"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock returned error")

	// Should get empty playlist because series is disabled
	assert.Empty(t, playlist, "Expected empty playlist for disabled series")
}

// programIDs extracts each program's GetID() in order, for asserting on
// exact playback order.
func programIDs(programs []tunarr.Program) []string {
	ids := make([]string, len(programs))
	for i := range programs {
		ids[i] = programs[i].GetID()
	}
	return ids
}

// TestPlanBlock_SeriesReorder_ReDerivesInNewOrderWithoutAdvancingCursor is
// the exact regression scenario the idempotent-apply design requires:
// commit a not-yet-aired occurrence with series order [A, B, C], reorder
// the block spec to [C, A, B], re-apply the SAME occurrence -- it must
// re-plan in the new order with the SAME episode picked for each show (no
// cursor advance), and persisted series state must be identical after
// both applies. This is what makes editing a block's series order (or
// adding/removing a series, or episodes_per_block/duration) visibly take
// effect on an occurrence that hasn't aired yet, instead of being frozen
// at first commit -- see PlanBlock's doc comment.
func TestPlanBlock_SeriesReorder_ReDerivesInNewOrderWithoutAdvancingCursor(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	availablePrograms := []tunarr.Program{
		{ID: "a-s1e1", Type: "episode", ShowTitle: "Show A", SeasonNumber: 1, EpisodeNumber: 1, Duration: 600_000},
		{ID: "b-s1e1", Type: "episode", ShowTitle: "Show B", SeasonNumber: 1, EpisodeNumber: 1, Duration: 600_000},
		{ID: "c-s1e1", Type: "episode", ShowTitle: "Show C", SeasonNumber: 1, EpisodeNumber: 1, Duration: 600_000},
	}

	blockABC := Block{
		Name: "Multi-Series Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{
			{ShowTitle: "Show A", EpisodesPerBlock: 1},
			{ShowTitle: "Show B", EpisodesPerBlock: 1},
			{ShowTitle: "Show C", EpisodesPerBlock: 1},
		},
	}

	engine := NewEngine(client, []Block{blockABC}, store, slog.Default(), time.UTC)

	occurrenceStart := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)
	now := occurrenceStart.Add(-1 * time.Hour) // still in the future relative to "now"

	ctx := context.Background()

	// First apply: commits the occurrence with order [A, B, C].
	first, err := engine.PlanBlock(blockABC, availablePrograms, occurrenceStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"a-s1e1", "b-s1e1", "c-s1e1"}, programIDs(first))

	stateA1, err := store.GetSeriesState(ctx, "Show A")
	require.NoError(t, err)
	stateB1, err := store.GetSeriesState(ctx, "Show B")
	require.NoError(t, err)
	stateC1, err := store.GetSeriesState(ctx, "Show C")
	require.NoError(t, err)
	// Sanity check the first apply actually advanced these cursors (past
	// S01E01) -- otherwise "identical after both applies" below would be
	// true trivially, proving nothing.
	require.Equal(t, 2, stateA1.CurrentEpisode)
	require.Equal(t, 2, stateB1.CurrentEpisode)
	require.Equal(t, 2, stateC1.CurrentEpisode)

	// Reorder the block's series to [C, A, B] -- simulating a block edit
	// made through the API before this occurrence airs.
	blockCAB := blockABC
	blockCAB.Series = []SeriesConfig{
		{ShowTitle: "Show C", EpisodesPerBlock: 1},
		{ShowTitle: "Show A", EpisodesPerBlock: 1},
		{ShowTitle: "Show B", EpisodesPerBlock: 1},
	}

	// Second apply, same occurrence, same "now" (still future): must
	// re-derive in the NEW order, picking the SAME episode for each show
	// (from the fixed stored snapshot, not the now-advanced live cursor),
	// and must not touch series state at all.
	second, err := engine.PlanBlock(blockCAB, availablePrograms, occurrenceStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	assert.Equal(t, []string{"c-s1e1", "a-s1e1", "b-s1e1"}, programIDs(second),
		"re-derived occurrence must reflect the new series order, with the same episode picked for each show")

	stateA2, err := store.GetSeriesState(ctx, "Show A")
	require.NoError(t, err)
	stateB2, err := store.GetSeriesState(ctx, "Show B")
	require.NoError(t, err)
	stateC2, err := store.GetSeriesState(ctx, "Show C")
	require.NoError(t, err)

	assert.Equal(t, stateA1, stateA2, "Show A's series state must be identical after both applies")
	assert.Equal(t, stateB1, stateB2, "Show B's series state must be identical after both applies")
	assert.Equal(t, stateC1, stateC2, "Show C's series state must be identical after both applies")
}

// TestPlanBlock_SeriesReorder_TwoOccurrences_ChainsCorrectedBaselineForward
// is TestPlanBlock_SeriesReorder_ReDerivesInNewOrderWithoutAdvancingCursor
// extended to TWO not-yet-aired occurrences of the same block re-derived in
// a single pass -- exactly where the single-occurrence version can't catch
// a chaining bug. Reviewer repro (see planSeriesOccurrences' doc comment):
// block.Series == [A,B,C] with a block duration that fits exactly 2 of the
// 3 shows' single episode each. Before planSeriesOccurrences chained a
// running state across occurrences, re-deriving BOTH occurrences after a
// reorder made occurrence 2 read its OWN stale, pre-reorder snapshot
// (which still thought C had never aired) instead of occurrence 1's
// actual new end state -- so occurrence 2 aired C's episode 1 a SECOND
// time instead of advancing to episode 2, and it's asserted directly
// below.
func TestPlanBlock_SeriesReorder_TwoOccurrences_ChainsCorrectedBaselineForward(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "a-s1e1", Type: "episode", ShowTitle: "Show A", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "a-s1e2", Type: "episode", ShowTitle: "Show A", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
		{ID: "b-s1e1", Type: "episode", ShowTitle: "Show B", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "b-s1e2", Type: "episode", ShowTitle: "Show B", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
		{ID: "c-s1e1", Type: "episode", ShowTitle: "Show C", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "c-s1e2", Type: "episode", ShowTitle: "Show C", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
	}

	blockABC := Block{
		ID: "multi-series-block", Name: "Multi-Series Block", Type: BlockTypeSeries, Duration: 60, ChannelID: "channel-1",
		Series: []SeriesConfig{
			{ShowTitle: "Show A", EpisodesPerBlock: 1},
			{ShowTitle: "Show B", EpisodesPerBlock: 1},
			{ShowTitle: "Show C", EpisodesPerBlock: 1},
		},
	}

	engine := NewEngine(client, []Block{blockABC}, store, slog.Default(), time.UTC)

	now := time.Now()
	occ1Start := now.Add(1 * time.Hour)
	occ2Start := now.Add(2 * time.Hour)
	shells := []ScheduledSlot{{StartTime: occ1Start, Block: blockABC}, {StartTime: occ2Start, Block: blockABC}}

	// First apply, original order [A, B, C], both occurrences planned (and
	// committed) together: 60min block, 30min episodes -- A+B fill it
	// exactly, C never fits.
	firstPlanned, err := engine.planSeriesOccurrences(blockABC, availablePrograms, shells, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"a-s1e1", "b-s1e1"}, programIDs(firstPlanned[0].Programs), "occurrence 1, original order")
	require.Equal(t, []string{"a-s1e2", "b-s1e2"}, programIDs(firstPlanned[1].Programs), "occurrence 2, original order")

	stateA1, err := store.GetSeriesState(ctx, "Show A")
	require.NoError(t, err)
	stateB1, err := store.GetSeriesState(ctx, "Show B")
	require.NoError(t, err)
	stateC1, err := store.GetSeriesState(ctx, "Show C")
	require.NoError(t, err)

	// Reorder to [C, A, B] and re-derive BOTH occurrences together, same
	// "now" (both still not-yet-aired).
	blockCAB := blockABC
	blockCAB.Series = []SeriesConfig{
		{ShowTitle: "Show C", EpisodesPerBlock: 1},
		{ShowTitle: "Show A", EpisodesPerBlock: 1},
		{ShowTitle: "Show B", EpisodesPerBlock: 1},
	}
	reorderedShells := []ScheduledSlot{{StartTime: occ1Start, Block: blockCAB}, {StartTime: occ2Start, Block: blockCAB}}

	secondPlanned, err := engine.planSeriesOccurrences(blockCAB, availablePrograms, reorderedShells, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	assert.Equal(t, []string{"c-s1e1", "a-s1e1"}, programIDs(secondPlanned[0].Programs), "occurrence 1, reordered")
	assert.Equal(t, []string{"c-s1e2", "a-s1e2"}, programIDs(secondPlanned[1].Programs),
		"occurrence 2, reordered -- must chain from occurrence 1's ACTUAL re-derived end state and pick C's SECOND episode, not repeat episode 1")

	// Re-deriving two not-yet-aired occurrences must never touch the real
	// persisted series state, exactly like the single-occurrence version.
	stateA2, err := store.GetSeriesState(ctx, "Show A")
	require.NoError(t, err)
	stateB2, err := store.GetSeriesState(ctx, "Show B")
	require.NoError(t, err)
	stateC2, err := store.GetSeriesState(ctx, "Show C")
	require.NoError(t, err)
	assert.Equal(t, stateA1, stateA2, "Show A's series state must be identical after both applies")
	assert.Equal(t, stateB1, stateB2, "Show B's series state must be identical after both applies")
	assert.Equal(t, stateC1, stateC2, "Show C's series state must be identical after both applies")
}

// TestPlanBlock_SeriesReDerive_DoesNotRepinStartEpisodeOnAlreadyAdvancedSnapshot
// pins finding 1's fix: a reconstructed snapshot used to always leave
// LastAired nil (see SeriesStateSnapshot.Seeded's doc comment), which made
// initializeSeriesState treat every re-derive as "never initialized" and
// re-apply start_episode on every single one -- silently re-pinning
// progress back to the configured start position. Reviewer probe: apply 1
// plans occurrence 2 at episode 6 (continuing on from occurrence 1's real
// start_episode:5); apply 2 re-derives occurrence 2 ALONE (its own stored
// snapshot, not chained with occurrence 1 in the same pass) and, before
// the fix, gets episode 5 again instead of continuing from 6.
func TestPlanBlock_SeriesReDerive_DoesNotRepinStartEpisodeOnAlreadyAdvancedSnapshot(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	availablePrograms := []tunarr.Program{
		{ID: "p-s1e5", Type: "episode", ShowTitle: "Pinned Show", SeasonNumber: 1, EpisodeNumber: 5, Duration: 1_800_000},
		{ID: "p-s1e6", Type: "episode", ShowTitle: "Pinned Show", SeasonNumber: 1, EpisodeNumber: 6, Duration: 1_800_000},
		{ID: "p-s1e7", Type: "episode", ShowTitle: "Pinned Show", SeasonNumber: 1, EpisodeNumber: 7, Duration: 1_800_000},
	}

	block := Block{
		ID: "pinned-block", Name: "Pinned Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Pinned Show", EpisodesPerBlock: 1, StartEpisode: 5}},
	}

	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	now := time.Now()
	occ1Start := now.Add(1 * time.Hour)
	occ2Start := now.Add(2 * time.Hour)

	// First apply: both occurrences planned together in one chained pass.
	// Occurrence 1 applies start_episode:5 for real (genuinely never
	// initialized before); occurrence 2 chains from occurrence 1's actual
	// post-consumption state (episode 6).
	shells := []ScheduledSlot{{StartTime: occ1Start, Block: block}, {StartTime: occ2Start, Block: block}}
	firstPlanned, err := engine.planSeriesOccurrences(block, availablePrograms, shells, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"p-s1e5"}, programIDs(firstPlanned[0].Programs), "occurrence 1 should apply start_episode:5")
	require.Equal(t, []string{"p-s1e6"}, programIDs(firstPlanned[1].Programs), "occurrence 2 should continue to episode 6")

	// Second apply: re-derive occurrence 2 ALONE, via PlanBlock's
	// single-shell delegation -- this time it must read its OWN stored
	// snapshot (chain is nil going in, unlike the batched first apply
	// above), which is exactly the path finding 1's bug lived on.
	second, err := engine.PlanBlock(block, availablePrograms, occ2Start, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	assert.Equal(t, []string{"p-s1e6"}, programIDs(second),
		"re-deriving occurrence 2 alone must NOT re-pin it back to start_episode:5 -- its snapshot already reflects episode 6")
}

// TestPlanBlock_SeriesFillerFallback_TwoApplies_ByteIdenticalContent pins
// finding 5's fix: re-deriving a not-yet-aired occurrence used to re-run
// rand.Shuffle (the global, unseeded math/rand source) for fallback/filler
// content on every apply, so a series block with filler enabled wasn't
// apply-idempotent even though its series episode picks were -- two
// consecutive applies of the same unchanged occurrence could select (or
// order) different filler programs. occurrenceRand seeds shuffling
// deterministically from (block ID, occurrence start) so a re-derive
// reproduces the exact same filler selection and order.
func TestPlanBlock_SeriesFillerFallback_TwoApplies_ByteIdenticalContent(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	availablePrograms := []tunarr.Program{
		{ID: "ep-1", Type: "episode", ShowTitle: "Filler Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 600_000},
		{ID: "filler-1", Type: "movie", Genres: []tunarr.Genre{{Name: "Filler"}}, Duration: 900_000},
		{ID: "filler-2", Type: "movie", Genres: []tunarr.Genre{{Name: "Filler"}}, Duration: 900_000},
		{ID: "filler-3", Type: "movie", Genres: []tunarr.Genre{{Name: "Filler"}}, Duration: 900_000},
		{ID: "filler-4", Type: "movie", Genres: []tunarr.Genre{{Name: "Filler"}}, Duration: 900_000},
		{ID: "filler-5", Type: "movie", Genres: []tunarr.Genre{{Name: "Filler"}}, Duration: 900_000},
	}

	block := Block{
		ID: "filler-fallback-block", Name: "Filler Fallback Block", Type: BlockTypeSeries, Duration: 70, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Filler Show", EpisodesPerBlock: 1}},
		Fallback: SeriesFallback{
			Mode:         FallbackModeFiller,
			FillerFilter: Filter{Genres: []string{"Filler"}},
		},
	}

	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	now := time.Now()
	occurrenceStart := now.Add(1 * time.Hour)

	// First apply: genuinely first-time plan. 70min block - 10min episode
	// = 60min gap; five 15min filler candidates means only four fit
	// (4*15=60), so WHICH four (and in what order) is a real, observable
	// signal of the shuffle, not just a relabeling of an always-identical
	// set.
	first, err := engine.PlanBlock(block, availablePrograms, occurrenceStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Contains(t, programIDs(first), "ep-1", "expected the series episode to be included")
	require.Len(t, first, 5, "expected the episode plus exactly 4 of the 5 filler candidates (60min gap / 15min each)")

	// Second apply, same not-yet-aired occurrence, unchanged spec: must
	// re-derive to EXACTLY the same content, byte for byte -- including
	// filler selection and order.
	second, err := engine.PlanBlock(block, availablePrograms, occurrenceStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	assert.Equal(t, programIDs(first), programIDs(second),
		"re-deriving an unchanged not-yet-aired occurrence with filler fallback enabled must produce byte-identical content, including filler order/selection")
}

// TestPlanSeriesOccurrences_PersistedCursorAdvancesAsOccurrencesAge pins
// round-2 finding 1's fix: chain-mode planning (the scratch
// snapshotSeriesContext path) only ever mutated the local `chain` map,
// never e.pendingStates -- so series_state stayed frozen at whatever the
// very first real plan produced, no matter how many further occurrences
// actually aired afterward (reviewer probe: cursor stuck at S1E2 while
// e1..e4 had genuinely aired). Fixed by syncing e.pendingStates from the
// chain every time planSeriesOccurrences processes an aired/on-air
// occurrence, since that occurrence's content is settled historical fact
// by then.
//
// Simulated across three separate applies of the SAME four-occurrence
// batch, with "now" moving forward each time so one more occurrence ages
// from not-yet-aired into aired -- exactly the reviewer's repro shape,
// not just a single apply.
func TestPlanSeriesOccurrences_PersistedCursorAdvancesAsOccurrencesAge(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "s-e1", Type: "episode", ShowTitle: "Cursor Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "s-e2", Type: "episode", ShowTitle: "Cursor Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
		{ID: "s-e3", Type: "episode", ShowTitle: "Cursor Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1_800_000},
		{ID: "s-e4", Type: "episode", ShowTitle: "Cursor Show", SeasonNumber: 1, EpisodeNumber: 4, Duration: 1_800_000},
		{ID: "s-e5", Type: "episode", ShowTitle: "Cursor Show", SeasonNumber: 1, EpisodeNumber: 5, Duration: 1_800_000},
	}
	block := Block{
		ID: "cursor-block", Name: "Cursor Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Cursor Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	base := time.Now()
	occ1 := base.Add(1 * time.Hour)
	occ2 := base.Add(2 * time.Hour)
	occ3 := base.Add(3 * time.Hour)
	occ4 := base.Add(4 * time.Hour)
	shells := []ScheduledSlot{
		{StartTime: occ1, Block: block}, {StartTime: occ2, Block: block},
		{StartTime: occ3, Block: block}, {StartTime: occ4, Block: block},
	}

	// Apply 1: nothing has aired yet.
	_, err := engine.planSeriesOccurrences(block, availablePrograms, shells, base)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	// Apply 2: occ1 and occ2 have now aired (occ3, occ4 haven't).
	_, err = engine.planSeriesOccurrences(block, availablePrograms, shells, occ2.Add(1*time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	state, err := store.GetSeriesState(ctx, "Cursor Show")
	require.NoError(t, err)
	assert.Equal(t, 3, state.CurrentEpisode,
		"persisted cursor must reflect BOTH aired occurrences (e1, e2 consumed -> next is e3), not stay frozen at whatever the first real plan produced")
	require.NotNil(t, state.LastAired)
	assert.False(t, state.LastAired.IsZero() || state.LastAired.Unix() == 0,
		"round-3 finding 4: LastAired must never be the epoch (1970-01-01) Seeded-marker sentinel")
	assert.True(t, state.LastAired.Equal(occ2),
		"round-3 finding 5: LastAired must be stamped from the most recently aired occurrence's own airtime (occ2), deterministically")
	lastAiredAfterApply2 := *state.LastAired

	// Apply 3: occ3 also ages into aired.
	_, err = engine.planSeriesOccurrences(block, availablePrograms, shells, occ3.Add(1*time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	state, err = store.GetSeriesState(ctx, "Cursor Show")
	require.NoError(t, err)
	assert.Equal(t, 4, state.CurrentEpisode, "persisted cursor must keep advancing as MORE occurrences age into aired")
	require.NotNil(t, state.LastAired)
	assert.True(t, state.LastAired.Equal(occ3), "round-3 finding 5: LastAired must update again (to occ3) as yet another occurrence ages into aired, not stay frozen at apply 2's value")
	assert.False(t, state.LastAired.Equal(lastAiredAfterApply2), "LastAired must have moved forward between applies, not stayed frozen in steady state")
}

// TestSyncPostStates_WritesPostState_StampsLastAiredAndProvenance is
// the direct unit test for the aired-branch persistence primitive:
// syncPostStates writes CurrentSeason/CurrentEpisode, Completed,
// Disabled, and RunCount (via max, never backward) from the stored
// post-state, stamps LastAired from the occurrence's own airtime (a pure
// function of its inputs, so replaying the same occurrence on every
// apply is idempotent -- and never the Seeded pre-state's sentinel), and
// records the plan's provenance (CursorPlanSeq). Round-6 finding 2:
// excluding the completion fields made on_complete:disable never disable
// and max_runs never trip in persisted state -- the operator-wins stamp,
// not a blanket exclusion, is what protects operator PATCHes now.
func TestSyncPostStates_WritesPostState_StampsLastAiredAndProvenance(t *testing.T) {
	store := NewMockStateStore()
	engine := NewEngine(&tunarr.Client{}, nil, store, slog.Default(), time.UTC)
	occurrenceStart := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)

	store.States["Show A"] = &SeriesState{
		ShowTitle: "Show A", CurrentSeason: 1, CurrentEpisode: 1, RunCount: 3, CursorPlanSeq: 10,
	}

	snap := OccurrenceSnapshot{
		PostStates: map[string]SeriesStateSnapshot{
			"Show A": {CurrentSeason: 1, CurrentEpisode: 3, Completed: true, Disabled: true, RunCount: 9, Seeded: true},
		},
		RecordedAt: occurrenceStart.Add(-1 * time.Hour),
		PlanSeq:    20,
	}
	require.NoError(t, engine.syncPostStates(snap, occurrenceStart))

	state := engine.pendingStates["Show A"]
	require.NotNil(t, state, "a newer-plan post-state must be queued for persistence")
	assert.Equal(t, 1, state.CurrentSeason)
	assert.Equal(t, 3, state.CurrentEpisode)
	require.NotNil(t, state.LastAired)
	assert.True(t, state.LastAired.Equal(occurrenceStart), "LastAired must be stamped from the occurrence's own airtime")
	assert.False(t, state.LastAired.Equal(seededMarker), "the Seeded-marker sentinel must never leak into persistence")
	assert.True(t, state.Completed, "Completed must come from the post-state -- plan-time completion is real state")
	assert.True(t, state.Disabled, "Disabled must come from the post-state -- on_complete:disable must persist")
	assert.Equal(t, 9, state.RunCount, "RunCount must adopt the post-state's higher value")
	assert.Equal(t, int64(20), state.CursorPlanSeq, "the winning plan's sequence must become the cursor's provenance")

	// RunCount is max(), never backward: a post-state carrying a LOWER
	// run count than the live row (e.g. live already advanced by a later
	// occurrence whose provenance was since invalidated back) must not
	// regress it.
	store.States["Show B"] = &SeriesState{ShowTitle: "Show B", CurrentSeason: 1, CurrentEpisode: 1, RunCount: 5}
	snapB := OccurrenceSnapshot{
		PostStates: map[string]SeriesStateSnapshot{"Show B": {CurrentSeason: 1, CurrentEpisode: 2, RunCount: 2}},
		RecordedAt: occurrenceStart.Add(-1 * time.Hour),
		PlanSeq:    20,
	}
	require.NoError(t, engine.syncPostStates(snapB, occurrenceStart))
	require.NotNil(t, engine.pendingStates["Show B"])
	assert.Equal(t, 5, engine.pendingStates["Show B"].RunCount, "RunCount must never move backward")
}

// TestSyncPostStates_ProvenanceRejectsStalePlan pins one half of the
// provenance guard (round-6 finding 1's replacement for the value-scoped
// monotonic check): a replay whose plan is OLDER than the plan that last
// wrote the live cursor is rejected outright -- even when its cursor
// value is AHEAD. Two blocks scheduling the same show plan occurrences
// independently; replaying the slower block's on-air occurrence must not
// override what a newer plan already established, in either value
// direction.
func TestSyncPostStates_ProvenanceRejectsStalePlan(t *testing.T) {
	store := NewMockStateStore()
	engine := NewEngine(&tunarr.Client{}, nil, store, slog.Default(), time.UTC)
	occurrenceStart := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)

	store.States["Show A"] = &SeriesState{ShowTitle: "Show A", CurrentSeason: 1, CurrentEpisode: 2, CursorPlanSeq: 100}

	// Value AHEAD (E10 > E2) but plan OLDER (50 < 100): must be skipped.
	snap := OccurrenceSnapshot{
		PostStates: map[string]SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 10}},
		RecordedAt: occurrenceStart.Add(-1 * time.Hour),
		PlanSeq:    50,
	}
	require.NoError(t, engine.syncPostStates(snap, occurrenceStart))
	_, pending := engine.pendingStates["Show A"]
	assert.False(t, pending, "a stale (older-plan) replay must not even queue a write, regardless of cursor direction")

	// An equal sequence (a re-replay of the very plan that wrote the
	// cursor) is also a no-op -- that's what makes repeated applies of
	// the same aired occurrence idempotent.
	snap.PlanSeq = 100
	require.NoError(t, engine.syncPostStates(snap, occurrenceStart))
	_, pending = engine.pendingStates["Show A"]
	assert.False(t, pending, "re-replaying the plan that already wrote the cursor must be a no-op")
}

// TestSyncPostStates_ProvenanceAllowsBackwardWrap pins the other half of
// the provenance guard, the exact case the round-5 value-scoped guard
// got wrong (round-6 finding 1, HIGH): an on_complete:restart wrap
// legitimately moves the cursor BACKWARD (S01E05 -> S01E01), and its
// replay carries a NEWER plan than whatever wrote the live cursor -- so
// it must land. Under the old value guard it was dropped as "backward,"
// freezing the persisted cursor at the pre-wrap high-water mark; the
// next snapshot invalidation then re-derived from the frozen cursor,
// regressing onto already-aired episodes permanently.
func TestSyncPostStates_ProvenanceAllowsBackwardWrap(t *testing.T) {
	store := NewMockStateStore()
	engine := NewEngine(&tunarr.Client{}, nil, store, slog.Default(), time.UTC)
	occurrenceStart := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)

	store.States["Show A"] = &SeriesState{ShowTitle: "Show A", CurrentSeason: 1, CurrentEpisode: 5, CursorPlanSeq: 100}

	snap := OccurrenceSnapshot{
		PostStates: map[string]SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 1, RunCount: 1, Seeded: true}},
		RecordedAt: occurrenceStart.Add(-1 * time.Hour),
		PlanSeq:    200,
	}
	require.NoError(t, engine.syncPostStates(snap, occurrenceStart))

	state := engine.pendingStates["Show A"]
	require.NotNil(t, state, "a newer plan's wrap must land")
	assert.Equal(t, 1, state.CurrentSeason)
	assert.Equal(t, 1, state.CurrentEpisode, "the restart wrap (E5 -> E1) must be written, not dropped as backward")
	assert.Equal(t, 1, state.RunCount)
	assert.Equal(t, int64(200), state.CursorPlanSeq)
}

// TestSyncPostStates_OperatorWriteWins pins the operator-wins guard: a
// show whose series_state carries an operator write NEWER than the
// occurrence's own commit is skipped even when the post-state's plan is
// newer than the cursor's provenance -- exactly the case the provenance
// guard alone cannot cover, and what makes an operator's BACKWARD jump
// (E10 -> E2) stick when the stale snapshot survived (a failed
// invalidation, or a write racing an in-flight apply). An occurrence
// committed AFTER the operator write applies normally -- it was planned
// from the operator's value.
func TestSyncPostStates_OperatorWriteWins(t *testing.T) {
	store := NewMockStateStore()
	engine := NewEngine(&tunarr.Client{}, nil, store, slog.Default(), time.UTC)
	occurrenceStart := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)
	committedAt := occurrenceStart.Add(-1 * time.Hour)
	operatorAt := committedAt.Add(30 * time.Minute) // after the commit

	store.States["Show A"] = &SeriesState{
		ShowTitle: "Show A", CurrentSeason: 1, CurrentEpisode: 2, OperatorUpdatedAt: &operatorAt,
	}

	snap := OccurrenceSnapshot{
		PostStates: map[string]SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 10}},
		RecordedAt: committedAt,
		PlanSeq:    50, // newer than the live cursor's zero provenance
	}
	require.NoError(t, engine.syncPostStates(snap, occurrenceStart))
	_, pending := engine.pendingStates["Show A"]
	assert.False(t, pending, "an occurrence committed BEFORE the operator write must never override it, even from a newer-than-cursor plan")

	// The same post-state from an occurrence committed AFTER the
	// operator write is legitimate -- it was planned from the patched
	// cursor -- and must apply.
	snap.RecordedAt = operatorAt.Add(30 * time.Minute)
	require.NoError(t, engine.syncPostStates(snap, occurrenceStart))
	state := engine.pendingStates["Show A"]
	require.NotNil(t, state, "an occurrence committed after the operator write must apply normally")
	assert.Equal(t, 10, state.CurrentEpisode)
	require.NotNil(t, state.OperatorUpdatedAt)
	assert.True(t, state.OperatorUpdatedAt.Equal(operatorAt), "the operator stamp itself must ride through unchanged")
}

// TestPlanSeriesOccurrences_TripleReapplyDuringOnAir_StableCursorAndNextOccurrence
// pins round-3 finding 1's fix against the reviewer's own probe: chain
// case 2 (establishSeriesChain seeding from live series_state, because
// this occurrence's snapshot is gone but its committed history survives
// -- exactly migration 000005's DROP TABLE on deploy day, or
// CleanupOccurrenceSnapshots' retention GC) already reflects the
// occurrence's OWN prior consumption once synced once; re-deriving it
// again from the SAME frozen content must not advance a second (or
// third) time, and the NEXT (not-yet-aired) occurrence chained after it
// must not churn either.
func TestPlanSeriesOccurrences_TripleReapplyDuringOnAir_StableCursorAndNextOccurrence(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "a-s1e1", Type: "episode", ShowTitle: "Steady Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "a-s1e2", Type: "episode", ShowTitle: "Steady Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
		{ID: "a-s1e3", Type: "episode", ShowTitle: "Steady Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1_800_000},
	}
	block := Block{
		ID: "steady-block", Name: "Steady Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Steady Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	onAirStart := time.Now().Add(-10 * time.Minute) // 10min into a 30min block
	nextStart := onAirStart.Add(1 * time.Hour)      // the next, not-yet-aired occurrence
	shells := []ScheduledSlot{{StartTime: onAirStart, Block: block}, {StartTime: nextStart, Block: block}}

	firstNow := onAirStart.Add(5 * time.Minute)
	first, err := engine.planSeriesOccurrences(block, availablePrograms, shells, firstNow)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"a-s1e1"}, programIDs(first[0].Programs))
	require.Equal(t, []string{"a-s1e2"}, programIDs(first[1].Programs))

	// Simulate migration 000005's DROP TABLE: only the on-air occurrence's
	// snapshot is gone; its own committed history survives.
	require.NoError(t, store.DeleteFutureOccurrenceSnapshots(ctx, block.ID, onAirStart.Add(-time.Second)))

	var lastState *SeriesState
	var lastNext []string
	for i := range 3 {
		reapplyNow := firstNow.Add(time.Duration(i+1) * time.Minute) // still on-air throughout
		result, err := engine.planSeriesOccurrences(block, availablePrograms, shells, reapplyNow)
		require.NoError(t, err)
		require.NoError(t, engine.Commit())

		require.Equal(t, []string{"a-s1e1"}, programIDs(result[0].Programs), "on-air occurrence's content must stay the verbatim replay, apply %d", i)

		state, err := store.GetSeriesState(ctx, "Steady Show")
		require.NoError(t, err)
		if lastState != nil {
			assert.Equal(t, lastState, state, "cursor must not drift on repeated re-applies of the same on-air occurrence, apply %d", i)
		}
		lastState = state

		nextContent := programIDs(result[1].Programs)
		if lastNext != nil {
			assert.Equal(t, lastNext, nextContent, "the NEXT (not-yet-aired) occurrence's content must not churn either, apply %d", i)
		}
		lastNext = nextContent
	}

	assert.Equal(t, 2, lastState.CurrentEpisode, "cursor should settle at episode 2 (one past what actually aired), not walk further (e.g. to 5) on repeated re-applies")
}

// TestPlanSeriesOccurrences_PatchDuringOnAir_CursorAndDisabledSurvive
// pins round-3 finding 2's fix against the reviewer's own probe: an
// operator's PATCH /state/series (current_episode: 20, disabled: true)
// while a show's occurrence is on air must survive the next apply --
// neither field reverts to whatever the on-air occurrence's OWN frozen
// content implies. Mirrors api.PatchSeriesState's store-level effects
// directly (UpdateSeriesState, then DeleteFutureOccurrenceSnapshots with
// a cutoff that also catches the still-on-air occurrence, not just
// future ones -- see store.InvalidateSeriesOccurrenceSnapshots)
// rather than going through the HTTP handler, to isolate the engine-side
// mechanism; TestPatchSeriesState_InvalidatesOnAirOccurrenceSnapshot
// (internal/api/state_test.go) covers the handler's own widened cutoff.
func TestPlanSeriesOccurrences_PatchDuringOnAir_CursorAndDisabledSurvive(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "a-s1e1", Type: "episode", ShowTitle: "Patched Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
	}
	block := Block{
		ID: "patched-block", Name: "Patched Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Patched Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	onAirStart := time.Now().Add(-10 * time.Minute)
	now := onAirStart.Add(5 * time.Minute)

	_, err := engine.PlanBlock(block, availablePrograms, onAirStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	// Operator PATCHes the show while it's still on air.
	require.NoError(t, store.UpdateSeriesState(ctx, &SeriesState{
		ShowTitle: "Patched Show", CurrentSeason: 1, CurrentEpisode: 20, Disabled: true,
	}))
	require.NoError(t, store.DeleteFutureOccurrenceSnapshots(ctx, block.ID, onAirStart.Add(-time.Second)))

	second, err := engine.PlanBlock(block, availablePrograms, onAirStart, now.Add(1*time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	assert.Equal(t, []string{"a-s1e1"}, programIDs(second), "the on-air occurrence's content must still be the verbatim replay")

	state, err := store.GetSeriesState(ctx, "Patched Show")
	require.NoError(t, err)
	assert.Equal(t, 20, state.CurrentEpisode, "the operator's cursor jump must survive the next apply")
	assert.True(t, state.Disabled, "the operator's disable must survive the next apply")
}

// TestPlanSeriesOccurrences_SpecEditAfterAired_AdvancesOnlyByCommittedContent
// pins round-3 finding 3's fix against the reviewer's own probe: editing
// a block's episodes_per_block AFTER an occurrence has already aired must
// not retroactively change how far its cursor advances -- the
// occurrence's own frozen, committed content (not the edited spec) is
// what the persisted cursor derives from.
func TestPlanSeriesOccurrences_SpecEditAfterAired_AdvancesOnlyByCommittedContent(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "a-s1e1", Type: "episode", ShowTitle: "Edited Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 600_000},
		{ID: "a-s1e2", Type: "episode", ShowTitle: "Edited Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 600_000},
		{ID: "a-s1e3", Type: "episode", ShowTitle: "Edited Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 600_000},
		{ID: "a-s1e4", Type: "episode", ShowTitle: "Edited Show", SeasonNumber: 1, EpisodeNumber: 4, Duration: 600_000},
	}
	block := Block{
		ID: "edited-block", Name: "Edited Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Edited Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	occStart := time.Now().Add(-10 * time.Minute)
	now := occStart.Add(5 * time.Minute)

	first, err := engine.PlanBlock(block, availablePrograms, occStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"a-s1e1"}, programIDs(first))

	// Operator edits the block AFTER this occurrence aired: bumps
	// episodes_per_block from 1 to 4, and (mirroring UpdateBlock)
	// invalidates the occurrence's snapshot -- its widened cutoff
	// catching the still-on-air occurrence too.
	editedBlock := block
	editedBlock.Series = []SeriesConfig{{ShowTitle: "Edited Show", EpisodesPerBlock: 4}}
	require.NoError(t, store.DeleteFutureOccurrenceSnapshots(ctx, block.ID, occStart.Add(-time.Second)))

	second, err := engine.PlanBlock(editedBlock, availablePrograms, occStart, now.Add(1*time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	assert.Equal(t, []string{"a-s1e1"}, programIDs(second),
		"an aired occurrence's content must stay the verbatim replay regardless of a later spec edit")

	state, err := store.GetSeriesState(ctx, "Edited Show")
	require.NoError(t, err)
	assert.Equal(t, 2, state.CurrentEpisode,
		"the persisted cursor must advance only by what actually aired (1 episode), not by the edited spec's episodes_per_block (4)")
}

// TestPlanSeriesOccurrences_SnapshotWipedButHistoryRemains_ReplacesNotAppends
// pins round-2 finding 2's fix: establishSeriesChain's "no snapshot"
// branch used to assume that meant "never planned before" unconditionally,
// so it called the append-only recordHistory path -- but
// DeleteFutureOccurrenceSnapshots (an operator's PATCH /state/series, or a
// block edit) deletes ONLY the snapshot, never the schedule_history rows
// a not-yet-aired occurrence's earlier commit also wrote. Re-deriving
// after such a wipe used to append a second, overlapping set of history
// rows on top of the first instead of replacing them (reviewer probe:
// GetCommittedOccurrence returned [a-s1e3, a-s1e2] -- two generations
// mixed together for the same occurrence).
func TestPlanSeriesOccurrences_SnapshotWipedButHistoryRemains_ReplacesNotAppends(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "a-s1e1", Type: "episode", ShowTitle: "Wipe Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "a-s1e2", Type: "episode", ShowTitle: "Wipe Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
		{ID: "a-s1e3", Type: "episode", ShowTitle: "Wipe Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1_800_000},
	}
	block := Block{
		ID: "wipe-block", Name: "Wipe Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Wipe Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	now := time.Now()
	occStart := now.Add(1 * time.Hour) // not yet aired throughout this test

	// First apply: commits e1.
	first, err := engine.PlanBlock(block, availablePrograms, occStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"a-s1e1"}, programIDs(first))

	// Simulate an operator PATCH/block-edit invalidation: wipe just this
	// occurrence's snapshot (now < occStart makes it "future").
	require.NoError(t, store.DeleteFutureOccurrenceSnapshots(ctx, block.ID, now))

	_, hasSnapshot, err := store.GetOccurrenceSnapshot(ctx, block.ID, occStart)
	require.NoError(t, err)
	require.False(t, hasSnapshot, "test setup: snapshot must be wiped")
	_, hasCommitted, err := store.GetCommittedOccurrence(ctx, block.Name, occStart)
	require.NoError(t, err)
	require.True(t, hasCommitted, "test setup: committed history must survive the snapshot wipe")

	// Second apply: same occurrence, same "now" -- re-derive.
	second, err := engine.PlanBlock(block, availablePrograms, occStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	// The critical assertion: exactly what was just re-derived, nothing
	// more -- not the first apply's content ALSO still sitting there.
	committed, ok, err := store.GetCommittedOccurrence(ctx, block.Name, occStart)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, programIDs(second), programIDs(committed),
		"the occurrence's committed assignment must have been REPLACED by the re-derive, not appended to")
	assert.Len(t, committed, 1, "must be exactly one program's worth of committed rows -- a doubled result mixing the old and new generations is exactly the reviewer's probe")
}

// TestPlanSeriesOccurrences_OnAirWithCommittedHistoryButNoSnapshot_NeverRealPlans
// pins round-2 finding 3's fix: establishSeriesChain's "no snapshot"
// branch used to real-plan UNCONDITIONALLY, bypassing PlanBlock's own
// aired guard entirely -- so the first apply after every snapshot was
// wiped (e.g. migration 000005's DROP TABLE on deploy day) real-planned
// even an ON-AIR occurrence, replacing its already-aired, committed
// content with something freshly (and differently) picked, and appending
// a duplicate history row on top of the original. This is the exact
// mid-episode-cutoff bug finding 7 (round 1) fixed, reintroduced by a gap
// in how "no snapshot" was handled. Fix: an occurrence with existing
// committed history is never real-planned, aired or not -- an aired one
// always replays verbatim via airedSeriesOccurrenceContent.
func TestPlanSeriesOccurrences_OnAirWithCommittedHistoryButNoSnapshot_NeverRealPlans(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "a-s1e1", Type: "episode", ShowTitle: "On Air Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "a-s1e2", Type: "episode", ShowTitle: "On Air Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
	}
	block := Block{
		ID: "onair-block", Name: "On Air Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "On Air Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	occStart := time.Now().Add(-10 * time.Minute) // already started
	firstNow := occStart.Add(5 * time.Minute)     // 5min into a 30min block: on air

	first, err := engine.PlanBlock(block, availablePrograms, occStart, firstNow)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"a-s1e1"}, programIDs(first))

	// Simulate migration 000005's DROP TABLE (or any other GC that clears
	// the snapshot but leaves committed history intact): now < occStart
	// makes this occurrence's own snapshot "future" and eligible for the
	// wipe.
	require.NoError(t, store.DeleteFutureOccurrenceSnapshots(ctx, block.ID, occStart.Add(-time.Second)))
	_, hasSnapshot, err := store.GetOccurrenceSnapshot(ctx, block.ID, occStart)
	require.NoError(t, err)
	require.False(t, hasSnapshot, "test setup: snapshot must be gone")

	// Re-apply later, still on air (25min in, still within the 30min
	// block).
	secondNow := occStart.Add(25 * time.Minute)
	second, err := engine.PlanBlock(block, availablePrograms, occStart, secondNow)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	assert.Equal(t, []string{"a-s1e1"}, programIDs(second),
		"an on-air occurrence with committed history must be replayed verbatim, snapshot or not -- never re-planned into DIFFERENT content")

	committed, ok, err := store.GetCommittedOccurrence(ctx, block.Name, occStart)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Len(t, committed, 1, "must not have doubled/duplicated the committed row")
}

// TestPlanSeriesOccurrences_RestartWrap_LandsViaReplayAndSurvivesInvalidation
// is round-6 finding 1's regression, rewritten to discriminate (the
// round-5 version's wrap target coincided with the fresh-default cursor
// and its first apply real-planned the wrap directly into live state, so
// it passed even with the guard removed or the sync stubbed). Here the
// live cursor is first established at the pre-wrap high-water mark
// (S01E04, written by occurrence 1's plan), and the wrap to S01E01
// arrives ONLY via the aired-branch REPLAY of chain-planned occurrence 2
// -- a legitimately BACKWARD cursor move from a NEWER plan. The
// provenance guard must land it (a value-scoped "forward-only" guard
// froze the cursor at the high-water mark), it must stay landed across
// repeated applies, and -- the permanent-damage half of the finding --
// a subsequent snapshot invalidation must re-derive the next occurrence
// from the wrapped cursor, not regress onto the episodes that just
// aired.
func TestPlanSeriesOccurrences_RestartWrap_LandsViaReplayAndSurvivesInvalidation(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "l-s1e1", Type: "episode", ShowTitle: "Loop Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "l-s1e2", Type: "episode", ShowTitle: "Loop Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
		{ID: "l-s1e3", Type: "episode", ShowTitle: "Loop Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1_800_000},
	}
	block := Block{
		ID: "loop-block", Name: "Loop Block", Type: BlockTypeSeries, Duration: 60, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Loop Show", EpisodesPerBlock: 2, OnComplete: CompletionActionRestart}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	base := time.Now()
	occ1 := base.Add(1 * time.Hour)
	occ2 := base.Add(2 * time.Hour)
	occ3 := base.Add(3 * time.Hour)
	shells := []ScheduledSlot{{StartTime: occ1, Block: block}, {StartTime: occ2, Block: block}}

	// Apply 1 (nothing aired): occ1 real-plans [e1,e2] -- live cursor at
	// the S01E03 high-water mark, wait for it to become the pre-wrap
	// baseline -- and occ2 chain-plans [e3] hitting the mid-occurrence
	// restart: content [e3], post-state S01E01 (run 2 queued).
	first, err := engine.planSeriesOccurrences(block, availablePrograms, shells, base)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"l-s1e1", "l-s1e2"}, programIDs(first[0].Programs))
	require.Equal(t, []string{"l-s1e3"}, programIDs(first[1].Programs))

	preWrap, err := store.GetSeriesState(ctx, "Loop Show")
	require.NoError(t, err)
	require.Equal(t, 3, preWrap.CurrentEpisode,
		"test setup: the live cursor must sit at the pre-wrap high-water mark before the wrap replays")

	// Applies 2-4: occ1 and occ2 have aired. occ2's replay must land the
	// wrap in persisted state -- S01E01, run_count 1 -- and hold it
	// stable, with both occurrences' content still replaying verbatim.
	for i := range 3 {
		now := occ2.Add(time.Duration(10+i) * time.Minute)
		result, err := engine.planSeriesOccurrences(block, availablePrograms, shells, now)
		require.NoError(t, err)
		require.NoError(t, engine.Commit())

		require.Equal(t, []string{"l-s1e1", "l-s1e2"}, programIDs(result[0].Programs), "aired content must stay the verbatim replay, apply %d", i)
		require.Equal(t, []string{"l-s1e3"}, programIDs(result[1].Programs), "aired content must stay the verbatim replay, apply %d", i)

		state, err := store.GetSeriesState(ctx, "Loop Show")
		require.NoError(t, err)
		assert.Equal(t, 1, state.CurrentSeason, "apply %d", i)
		assert.Equal(t, 1, state.CurrentEpisode,
			"the restart wrap must LAND in persisted state via the replay -- not freeze at the pre-wrap high-water mark (E3/E4), apply %d", i)
		assert.Equal(t, 1, state.RunCount, "the wrap's run_count must persist too, apply %d", i)
	}

	// The permanent-damage half: wipe every snapshot (an operator write,
	// block edit, or retention GC) and plan the NEXT occurrence. With the
	// wrap correctly persisted it re-derives from S01E01 -> [e1,e2]; a
	// cursor frozen at the high-water mark would have re-derived [e3] --
	// regressing onto the episode that JUST aired in occ2.
	require.NoError(t, store.DeleteFutureOccurrenceSnapshots(ctx, block.ID, base.Add(-time.Hour)))
	withNext := append(shells, ScheduledSlot{StartTime: occ3, Block: block})
	result, err := engine.planSeriesOccurrences(block, availablePrograms, withNext, occ2.Add(20*time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	assert.Equal(t, []string{"l-s1e1", "l-s1e2"}, programIDs(result[2].Programs),
		"after invalidation, the next occurrence must re-derive from the WRAPPED cursor (run 2 from E1), never regress onto the just-aired e3")

	state, err := store.GetSeriesState(ctx, "Loop Show")
	require.NoError(t, err)
	assert.Equal(t, 1, state.CurrentEpisode, "the wrapped cursor must survive the invalidation itself")
}

// TestPlanSeriesOccurrences_StaleOnAirReplay_NeverDragsLiveCursorBack is
// round-4 finding 2's regression: block B's on-air occurrence committed
// while the shared show sat at E1 (content [e1,e2], post-state E3);
// since then another block legitimately advanced the LIVE cursor to E7
// (simulated with a plain, non-operator series_state write carrying a
// NEWER plan provenance than block B's snapshot -- exactly what block
// A's own aired-occurrence sync produces, since block A planned later).
// Re-applying block B must not drag the live cursor back to E3:
// syncPostStates' provenance guard rejects the older plan's replay.
func TestPlanSeriesOccurrences_StaleOnAirReplay_NeverDragsLiveCursorBack(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := make([]tunarr.Program, 0, 8)
	for i := 1; i <= 8; i++ {
		availablePrograms = append(availablePrograms, tunarr.Program{
			ID: fmt.Sprintf("d-s1e%d", i), Type: "episode", ShowTitle: "Dual Show",
			SeasonNumber: 1, EpisodeNumber: i, Duration: 1_800_000,
		})
	}
	blockB := Block{
		ID: "dual-block-b", Name: "Dual Block B", Type: BlockTypeSeries, Duration: 60, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Dual Show", EpisodesPerBlock: 2}},
	}
	engine := NewEngine(client, []Block{blockB}, store, slog.Default(), time.UTC)

	onAirStart := time.Now().Add(-10 * time.Minute)
	now := onAirStart.Add(5 * time.Minute)

	first, err := engine.PlanBlock(blockB, availablePrograms, onAirStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"d-s1e1", "d-s1e2"}, programIDs(first))

	// Block A (another block scheduling the same show) has since
	// advanced the live cursor to E7 -- an engine-style write, NO
	// operator stamp, carrying the newer plan provenance block A's own
	// sync would have written (it planned AFTER block B did), so only
	// the provenance guard protects it.
	staleKey := occurrenceKey{blockName: blockB.ID, startUnixNano: onAirStart.UnixNano()}
	staleSeq := store.Snapshots[staleKey].PlanSeq
	require.NotZero(t, staleSeq, "test setup: block B's snapshot must carry a plan sequence")
	require.NoError(t, store.UpdateSeriesState(ctx, &SeriesState{
		ShowTitle: "Dual Show", CurrentSeason: 1, CurrentEpisode: 7, CursorPlanSeq: staleSeq + 1,
	}))

	second, err := engine.PlanBlock(blockB, availablePrograms, onAirStart, now.Add(1*time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	assert.Equal(t, []string{"d-s1e1", "d-s1e2"}, programIDs(second), "block B's on-air content must stay the verbatim replay")

	state, err := store.GetSeriesState(ctx, "Dual Show")
	require.NoError(t, err)
	assert.Equal(t, 7, state.CurrentEpisode,
		"re-applying block B's stale on-air occurrence (post-state E3) must never drag the live cursor back behind E7")
}

// TestPlanSeriesOccurrences_OnCompleteDisable_PersistsAfterAiring is
// round-6 finding 2's first regression: syncPostStates used to discard
// the post-state's Completed/Disabled entirely, so an
// on_complete:disable decided at plan time in a CHAIN-planned occurrence
// never reached persisted series_state once it aired -- the chain
// re-decided (and re-logged) the disable on every apply while GET
// /state/series showed the show as active forever.
func TestPlanSeriesOccurrences_OnCompleteDisable_PersistsAfterAiring(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "o-s1e1", Type: "episode", ShowTitle: "Once Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "o-s1e2", Type: "episode", ShowTitle: "Once Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
	}
	block := Block{
		ID: "once-block", Name: "Once Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Once Show", EpisodesPerBlock: 1, OnComplete: CompletionActionDisable}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	base := time.Now()
	occ1 := base.Add(1 * time.Hour)
	occ2 := base.Add(2 * time.Hour)
	occ3 := base.Add(3 * time.Hour) // exhausts the show: completion + disable decided here, at plan time
	shells := []ScheduledSlot{
		{StartTime: occ1, Block: block}, {StartTime: occ2, Block: block}, {StartTime: occ3, Block: block},
	}

	_, err := engine.planSeriesOccurrences(block, availablePrograms, shells, base)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	for i := range 2 {
		now := occ3.Add(time.Duration(10+i) * time.Minute) // all three occurrences aired
		_, err := engine.planSeriesOccurrences(block, availablePrograms, shells, now)
		require.NoError(t, err)
		require.NoError(t, engine.Commit())

		state, err := store.GetSeriesState(ctx, "Once Show")
		require.NoError(t, err)
		assert.True(t, state.Completed, "plan-time completion must persist once the deciding occurrence airs, apply %d", i)
		assert.True(t, state.Disabled, "on_complete:disable must persist, not evaporate at the sync, apply %d", i)
		assert.Equal(t, 1, state.RunCount, "the completion's run_count must persist, apply %d", i)
	}
}

// TestPlanSeriesOccurrences_MaxRuns_TripsExactlyOnceAndPersists is
// round-6 finding 2's second regression: with the completion fields
// discarded by the sync, max_runs never tripped in persisted state
// (run_count froze at 0 while the chain kept "disabling" the show on
// every apply). After the run limit trips at plan time and the deciding
// occurrence airs, persisted state must show run_count exactly at
// max_runs and disabled=true, stable across further applies -- RunCount
// syncs via max(), never backward and never double-counted.
func TestPlanSeriesOccurrences_MaxRuns_TripsExactlyOnceAndPersists(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "m-s1e1", Type: "episode", ShowTitle: "Twice Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "m-s1e2", Type: "episode", ShowTitle: "Twice Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
	}
	block := Block{
		ID: "twice-block", Name: "Twice Block", Type: BlockTypeSeries, Duration: 90, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Twice Show", EpisodesPerBlock: 3, OnComplete: CompletionActionRestart, MaxRuns: 2}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	base := time.Now()
	occ1 := base.Add(1 * time.Hour) // run 1: [e1,e2] + restart (run_count 1)
	occ2 := base.Add(3 * time.Hour) // run 2: [e1,e2] + completion trips max_runs (run_count 2, disabled)
	occ3 := base.Add(5 * time.Hour) // disabled: plans nothing
	shells := []ScheduledSlot{
		{StartTime: occ1, Block: block}, {StartTime: occ2, Block: block}, {StartTime: occ3, Block: block},
	}

	_, err := engine.planSeriesOccurrences(block, availablePrograms, shells, base)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	for i := range 3 {
		now := occ3.Add(time.Duration(10+i) * time.Minute) // everything aired
		_, err := engine.planSeriesOccurrences(block, availablePrograms, shells, now)
		require.NoError(t, err)
		require.NoError(t, engine.Commit())

		state, err := store.GetSeriesState(ctx, "Twice Show")
		require.NoError(t, err)
		assert.Equal(t, 2, state.RunCount,
			"run_count must trip to max_runs exactly once and stay there -- never 0 (discarded) and never inflated by re-replays, apply %d", i)
		assert.True(t, state.Disabled, "the max_runs auto-disable must persist, apply %d", i)
	}
}

// TestPlanSeriesOccurrences_OperatorBackwardJump_SticksAndShapesNextOccurrence
// is round-4 finding 3's happy-path regression: an operator jumps the
// cursor BACKWARD (E10 -> E2) mid-air, with the standard invalidation
// (store.InvalidateSeriesOccurrenceSnapshots' widened cutoff catching
// the on-air occurrence too). Across subsequent applies the jump must
// stick -- the aired branch finds no snapshot, re-seeds its chain from
// the freshly patched live state, and persists nothing -- and the next
// not-yet-aired occurrence must re-derive from E2, landing the deferred
// effect exactly where docs/scheduling-concepts.md promises.
func TestPlanSeriesOccurrences_OperatorBackwardJump_SticksAndShapesNextOccurrence(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := make([]tunarr.Program, 0, 12)
	for i := 1; i <= 12; i++ {
		availablePrograms = append(availablePrograms, tunarr.Program{
			ID: fmt.Sprintf("j-s1e%d", i), Type: "episode", ShowTitle: "Jump Show",
			SeasonNumber: 1, EpisodeNumber: i, Duration: 1_800_000,
		})
	}
	block := Block{
		ID: "jump-block", Name: "Jump Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Jump Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	seeded := time.Now().Add(-24 * time.Hour)
	require.NoError(t, store.UpdateSeriesState(ctx, &SeriesState{
		ShowTitle: "Jump Show", CurrentSeason: 1, CurrentEpisode: 9, LastAired: &seeded,
	}))

	onAirStart := time.Now().Add(-10 * time.Minute)
	nextStart := onAirStart.Add(1 * time.Hour)
	shells := []ScheduledSlot{{StartTime: onAirStart, Block: block}, {StartTime: nextStart, Block: block}}

	firstNow := onAirStart.Add(5 * time.Minute)
	first, err := engine.planSeriesOccurrences(block, availablePrograms, shells, firstNow)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"j-s1e9"}, programIDs(first[0].Programs))

	// Operator jumps backward to E2 (stamped), and the API/CLI paths'
	// invalidation wipes every not-yet-FINISHED occurrence snapshot --
	// the widened cutoff reaches the on-air occurrence too.
	operatorAt := time.Now()
	require.NoError(t, store.UpdateSeriesState(ctx, &SeriesState{
		ShowTitle: "Jump Show", CurrentSeason: 1, CurrentEpisode: 2, LastAired: &seeded, OperatorUpdatedAt: &operatorAt,
	}))
	require.NoError(t, store.DeleteFutureOccurrenceSnapshots(ctx, block.ID, onAirStart.Add(-time.Second)))

	for i := range 3 {
		reapplyNow := firstNow.Add(time.Duration(i+1) * time.Minute) // still on air
		result, err := engine.planSeriesOccurrences(block, availablePrograms, shells, reapplyNow)
		require.NoError(t, err)
		require.NoError(t, engine.Commit())

		require.Equal(t, []string{"j-s1e9"}, programIDs(result[0].Programs),
			"the on-air occurrence's already-aired content must stay the verbatim replay, apply %d", i)
		assert.Equal(t, []string{"j-s1e2"}, programIDs(result[1].Programs),
			"the next not-yet-aired occurrence must honor the operator's backward jump, apply %d", i)

		state, err := store.GetSeriesState(ctx, "Jump Show")
		require.NoError(t, err)
		assert.Equal(t, 2, state.CurrentEpisode, "the operator's backward jump must survive apply %d", i)
	}
}

// TestPlanSeriesOccurrences_OperatorBackwardJump_SurvivesFailedInvalidation
// is round-4 finding 3's defense-in-depth regression: same backward jump
// (E10 -> E2), but the snapshot invalidation FAILED (the API/CLI paths
// log-and-continue on that), so the on-air occurrence's stale snapshot
// -- post-state E10, AHEAD of the patched E2 -- is still there. Only
// syncPostStates' operator-wins guard (the occurrence's commit predates
// the operator stamp) keeps the jump from being re-advanced on every
// apply while the occurrence stays on air.
func TestPlanSeriesOccurrences_OperatorBackwardJump_SurvivesFailedInvalidation(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := make([]tunarr.Program, 0, 12)
	for i := 1; i <= 12; i++ {
		availablePrograms = append(availablePrograms, tunarr.Program{
			ID: fmt.Sprintf("g-s1e%d", i), Type: "episode", ShowTitle: "Guard Show",
			SeasonNumber: 1, EpisodeNumber: i, Duration: 1_800_000,
		})
	}
	block := Block{
		ID: "guard-block", Name: "Guard Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Guard Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	seeded := time.Now().Add(-24 * time.Hour)
	require.NoError(t, store.UpdateSeriesState(ctx, &SeriesState{
		ShowTitle: "Guard Show", CurrentSeason: 1, CurrentEpisode: 9, LastAired: &seeded,
	}))

	onAirStart := time.Now().Add(-10 * time.Minute)
	now := onAirStart.Add(5 * time.Minute)
	first, err := engine.PlanBlock(block, availablePrograms, onAirStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"g-s1e9"}, programIDs(first))

	// Backward jump, stamped strictly after the snapshot's commit -- but
	// NO invalidation: the stale snapshot (post-state E10) survives.
	operatorAt := time.Now().Add(time.Second)
	require.NoError(t, store.UpdateSeriesState(ctx, &SeriesState{
		ShowTitle: "Guard Show", CurrentSeason: 1, CurrentEpisode: 2, LastAired: &seeded, OperatorUpdatedAt: &operatorAt,
	}))

	second, err := engine.PlanBlock(block, availablePrograms, onAirStart, now.Add(1*time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"g-s1e9"}, programIDs(second))

	state, err := store.GetSeriesState(ctx, "Guard Show")
	require.NoError(t, err)
	assert.Equal(t, 2, state.CurrentEpisode,
		"the surviving stale snapshot's post-state (E10) is ahead of the jump (E2): only the operator-wins guard keeps the jump in place")
}

// TestPlanSeriesOccurrences_CatalogLosesPrograms_PostStateStillAdvances
// is round-4 finding 6's regression: a committed program that vanished
// from the Tunarr catalog is reconstructed from schedule_history with NO
// season/episode metadata (those columns don't exist there), so any
// advance derived from content silently skipped the show entirely and
// the next occurrence re-aired the same episodes. The stored post-state
// replay never reads content at all, so the cursor keeps advancing even
// when the whole catalog is gone.
func TestPlanSeriesOccurrences_CatalogLosesPrograms_PostStateStillAdvances(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "v-s1e1", Type: "episode", ShowTitle: "Vanish Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "v-s1e2", Type: "episode", ShowTitle: "Vanish Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
		{ID: "v-s1e3", Type: "episode", ShowTitle: "Vanish Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1_800_000},
	}
	block := Block{
		ID: "vanish-block", Name: "Vanish Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Vanish Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	base := time.Now()
	occ1 := base.Add(1 * time.Hour)
	occ2 := base.Add(2 * time.Hour)
	occ3 := base.Add(3 * time.Hour)
	shells := []ScheduledSlot{
		{StartTime: occ1, Block: block}, {StartTime: occ2, Block: block}, {StartTime: occ3, Block: block},
	}

	// Apply 1, full catalog: occ1 -> e1, occ2 -> e2, occ3 -> e3.
	_, err := engine.planSeriesOccurrences(block, availablePrograms, shells, base)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	// Apply 2: occ1 and occ2 have aired, and the ENTIRE catalog is gone.
	result, err := engine.planSeriesOccurrences(block, nil, shells, occ2.Add(1*time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	require.Equal(t, []string{"v-s1e1"}, programIDs(result[0].Programs),
		"aired content must replay verbatim from history even when the catalog lost it")
	require.Equal(t, []string{"v-s1e2"}, programIDs(result[1].Programs))

	state, err := store.GetSeriesState(ctx, "Vanish Show")
	require.NoError(t, err)
	assert.Equal(t, 3, state.CurrentEpisode,
		"the persisted cursor must advance past BOTH aired occurrences via post-state replay -- metadata-less reconstructed content must not stall it")
	require.NotNil(t, state.LastAired)
	assert.True(t, state.LastAired.Equal(occ2), "LastAired must still be stamped from the most recently aired occurrence")
}

// TestPlanSeriesOccurrences_LegacySnapshotWithoutPostState_NoAdvanceNoCrash
// pins migration 000006's graceful-degradation contract at the engine
// level: an aired occurrence whose snapshot predates the post_state_json
// column (PostStates nil) replays its committed content verbatim,
// contributes no cursor advance of its own, and never crashes -- the
// old-code plan that wrote such a row already advanced live state
// itself, so there is nothing left for the replay to do.
func TestPlanSeriesOccurrences_LegacySnapshotWithoutPostState_NoAdvanceNoCrash(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	ctx := context.Background()

	availablePrograms := []tunarr.Program{
		{ID: "y-s1e1", Type: "episode", ShowTitle: "Legacy Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000},
		{ID: "y-s1e2", Type: "episode", ShowTitle: "Legacy Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_800_000},
	}
	block := Block{
		ID: "legacy-block", Name: "Legacy Block", Type: BlockTypeSeries, Duration: 30, ChannelID: "channel-1",
		Series: []SeriesConfig{{ShowTitle: "Legacy Show", EpisodesPerBlock: 1}},
	}
	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	onAirStart := time.Now().Add(-10 * time.Minute)
	now := onAirStart.Add(5 * time.Minute)
	first, err := engine.PlanBlock(block, availablePrograms, onAirStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"y-s1e1"}, programIDs(first))

	// Strip the post-state, turning the row into the pre-migration-000006
	// shape.
	key := occurrenceKey{blockName: block.ID, startUnixNano: onAirStart.UnixNano()}
	legacy := store.Snapshots[key]
	legacy.PostStates = nil
	store.Snapshots[key] = legacy

	second, err := engine.PlanBlock(block, availablePrograms, onAirStart, now.Add(1*time.Minute))
	require.NoError(t, err, "a legacy row must never crash the aired branch")
	require.NoError(t, engine.Commit())
	assert.Equal(t, []string{"y-s1e1"}, programIDs(second), "a legacy occurrence still replays its committed content verbatim")

	state, err := store.GetSeriesState(ctx, "Legacy Show")
	require.NoError(t, err)
	assert.Equal(t, 2, state.CurrentEpisode,
		"live state keeps whatever the original plan wrote (E2) -- the legacy replay contributes no advance of its own")
}

func TestGenerateForTimeRange(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	blocks := []Block{
		{
			Name:      "Morning Block",
			Type:      BlockTypeFilter,
			Cron:      "0 9 * * *", // Daily at 9 AM
			Duration:  60,          // 1 hour
			ChannelID: "channel-1",
			Priority:  10,
			Filter: Filter{
				Genres: []string{"Comedy"},
			},
		},
		{
			Name:      "Evening Block",
			Type:      BlockTypeFilter,
			Cron:      "0 20 * * *", // Daily at 8 PM
			Duration:  120,          // 2 hours
			ChannelID: "channel-1",
			Priority:  10,
			Filter: Filter{
				Genres: []string{"Drama"},
			},
		},
	}

	engine := NewEngine(client, blocks, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Comedy Show", Genres: []tunarr.Genre{{Name: "Comedy"}}, Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Drama Show", Genres: []tunarr.Genre{{Name: "Drama"}}, Duration: 3600000, Type: "episode"},
	}

	// Test a 24-hour period
	start := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	schedule, _, err := engine.GenerateForTimeRange(start, end, availablePrograms)
	require.NoError(t, err, "GenerateForTimeRange returned error")

	// Should have schedule for channel-1
	programs, ok := schedule["channel-1"]
	require.True(t, ok, "Expected schedule for channel-1")

	// Should have programs from both blocks (morning and evening)
	assert.NotEmpty(t, programs, "Expected programs in schedule")
}

// TestGenerateForTimeRange_UsesConfiguredLocationForCronOccurrences is the
// regression test for a bug found during the first live apply against a
// real Tunarr instance (2026-08-29): cron occurrences were computed against
// start's own Location instead of the engine's configured one, so a
// deployment with log.timezone set to anything other than "Local"/UTC (a
// bare time.Now() in a container without TZ set carries UTC) silently
// planned every block's occurrences against UTC wall-clock fields. A cron
// like "30 20 * * *" then fired at 20:30 UTC, not 20:30 in the configured
// zone -- e.g. planning tonight's occurrence even though 20:30 in the real
// zone had already passed.
//
// This uses a synthetic +2h time.FixedZone rather than a real IANA zone
// (time.LoadLocation("Europe/Zurich")) so the test doesn't depend on the
// test environment's tzdata. The offset and start instant are chosen so
// the pre-fix (UTC) and post-fix (configured-location) answers disagree
// about which *day* is next, not just the hour: at the chosen start
// instant, local wall-clock time in the +2h zone is 21:00 -- 20:30 has
// already passed locally, so the correct next occurrence is tomorrow at
// 20:30 local. UTC wall-clock time is still 19:00 (before 20:30), so the
// pre-fix bug -- evaluating the cron against start's UTC fields instead of
// e.location -- would wrongly report *today* at 20:30 UTC as next.
func TestGenerateForTimeRange_UsesConfiguredLocationForCronOccurrences(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	blocks := []Block{
		{
			Name:      "Nightly Block",
			Type:      BlockTypeFilter,
			Cron:      "30 20 * * *", // daily at 20:30 in the engine's configured location
			Duration:  30,
			ChannelID: "channel-1",
			Priority:  10,
			Filter: Filter{
				Genres: []string{"Drama"},
			},
		},
	}

	loc := time.FixedZone("TestZone+2", 2*60*60)
	engine := NewEngine(client, blocks, store, slog.Default(), loc)

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Drama Show", Genres: []tunarr.Genre{{Name: "Drama"}}, Duration: 1_800_000, Type: "episode"},
	}

	start := time.Date(2026, 1, 12, 19, 0, 0, 0, time.UTC) // 21:00 local in loc
	end := start.Add(30 * time.Hour)                       // wide enough for exactly one occurrence either way

	schedule, _, err := engine.GenerateForTimeRange(start, end, availablePrograms)
	require.NoError(t, err, "GenerateForTimeRange returned error")

	slots, ok := schedule["channel-1"]
	require.True(t, ok, "expected a schedule for channel-1")
	require.NotEmpty(t, slots, "expected at least one scheduled occurrence")

	got := slots[0].StartTime
	buggyUTCAnswer := time.Date(2026, 1, 12, 20, 30, 0, 0, time.UTC) // today at 20:30 UTC -- what the pre-fix bug produced
	correctAnswer := time.Date(2026, 1, 13, 18, 30, 0, 0, time.UTC)  // tomorrow at 20:30 in the +2h zone

	assert.False(t, got.Equal(buggyUTCAnswer),
		"next occurrence must not be today's UTC-evaluated 20:30 (%s) -- cron must be evaluated in the engine's configured location, not UTC", buggyUTCAnswer)
	assert.True(t, got.Equal(correctAnswer),
		"expected next occurrence %s (20:30 tomorrow in the configured +2h location), got %s", correctAnswer, got)
}

func TestGenerateForTimeRange_InvalidCron(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	blocks := []Block{
		{
			Name:      "Invalid Block",
			Type:      BlockTypeFilter,
			Cron:      "invalid cron",
			Duration:  60,
			ChannelID: "channel-1",
			Priority:  10,
		},
	}

	engine := NewEngine(client, blocks, store, slog.Default(), time.UTC)

	start := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	_, _, err := engine.GenerateForTimeRange(start, end, []tunarr.Program{})
	assert.Error(t, err, "Expected error for invalid cron expression")
}

func TestGenerateForTimeRange_ConflictResolution(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	blocks := []Block{
		{
			Name:      "Low Priority Block",
			Type:      BlockTypeFilter,
			Cron:      "0 9 * * *",
			Duration:  120, // 2 hours
			ChannelID: "channel-1",
			Priority:  5,
			Filter: Filter{
				Genres: []string{"Comedy"},
			},
		},
		{
			Name:      "High Priority Block",
			Type:      BlockTypeFilter,
			Cron:      "30 9 * * *", // Overlaps with low priority
			Duration:  60,           // 1 hour
			ChannelID: "channel-1",
			Priority:  10,
			Filter: Filter{
				Genres: []string{"Drama"},
			},
		},
	}

	engine := NewEngine(client, blocks, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Comedy Show", Genres: []tunarr.Genre{{Name: "Comedy"}}, Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Drama Show", Genres: []tunarr.Genre{{Name: "Drama"}}, Duration: 1800000, Type: "episode"},
	}

	start := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	schedule, warnings, err := engine.GenerateForTimeRange(start, end, availablePrograms)
	require.NoError(t, err, "GenerateForTimeRange returned error")

	programs, ok := schedule["channel-1"]
	require.True(t, ok, "Expected schedule for channel-1")

	// Should have programs from both blocks, with high priority winning conflicts
	assert.NotEmpty(t, programs, "Expected programs in schedule")

	// The losing occurrence must be reported as a Warning, not just logged.
	require.NotEmpty(t, warnings, "expected a Warning for the low-priority block's dropped occurrence")
	for _, w := range warnings {
		assert.Equal(t, "Low Priority Block", w.BlockName)
		assert.Equal(t, "High Priority Block", w.BlockingBlockName)
	}
}

// TestResolveConflicts_RestartsScanAfterEviction_HandlesThreeMutualOverlaps
// pins finding 6's fix: resolveConflicts used to `break` out of its inner
// scan the moment it evicted ONE lower-priority resolved slot, without
// re-checking the winner against any OTHER already-resolved slot it also
// overlaps. A and B here do NOT overlap each other (so both survive
// independently into `resolved`), but C overlaps BOTH of them and has the
// highest priority -- the old code would evict A, `break`, and then just
// append C, leaving B and C overlapping in the final result (only A would
// be reported dropped). That broken invariant -- resolved slots aren't
// actually non-overlapping -- is exactly what buildAnchoredLineup
// (service/schedule.go) assumes never happens: an undetected overlap
// there produces a negative gap and silent wall-clock drift.
func TestResolveConflicts_RestartsScanAfterEviction_HandlesThreeMutualOverlaps(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	base := time.Date(2026, 1, 12, 9, 0, 0, 0, time.UTC)
	slotA := ScheduledSlot{StartTime: base, EndTime: base.Add(60 * time.Minute), Block: Block{Name: "A", Priority: 1}}
	slotB := ScheduledSlot{StartTime: base.Add(120 * time.Minute), EndTime: base.Add(180 * time.Minute), Block: Block{Name: "B", Priority: 1}}
	// C spans [9:30, 11:30) -- overlaps A's tail (9:30-10:00) AND B's head
	// (11:00-11:30), but A and B themselves don't overlap each other.
	slotC := ScheduledSlot{StartTime: base.Add(30 * time.Minute), EndTime: base.Add(150 * time.Minute), Block: Block{Name: "C", Priority: 5}}

	require.False(t, slotsOverlap(slotA, slotB), "test setup: A and B must not overlap each other")
	require.True(t, slotsOverlap(slotC, slotA), "test setup: C must overlap A")
	require.True(t, slotsOverlap(slotC, slotB), "test setup: C must overlap B")

	resolved, dropped := engine.resolveConflicts([]ScheduledSlot{slotA, slotB, slotC})

	require.Len(t, resolved, 1, "only the highest-priority slot should survive three mutually overlapping slots")
	assert.Equal(t, "C", resolved[0].Block.Name)
	assert.Len(t, dropped, 2, "both A and B must be reported as dropped, not just A")

	for i := 0; i < len(resolved); i++ {
		for j := i + 1; j < len(resolved); j++ {
			assert.False(t, slotsOverlap(resolved[i], resolved[j]), "resolved slots must never overlap")
		}
	}
}

// TestResolveConflicts_EvictThenLose_DoesNotLeaveEarlierEvictionCommitted
// pins round-2 finding 4's fix: the round-1 restart-scan version above
// still processed slots in their ORIGINAL order and evicted immediately,
// so a mid-priority slot could evict a lower-priority one and then, later
// in that same original-order pass, itself lose to a still-higher
// slot -- without ever undoing the eviction it caused. Reviewer probe:
// A(p1) and C(p3) do NOT overlap each other, but both overlap B(p2);
// processed in order [A, C, B], B evicts A, then B itself loses to C.
// The old code left A dropped anyway (despite never actually conflicting
// with the real survivor C) and blamed B -- which itself never
// survived -- in A's Warning. Fixed by processing highest-priority-first
// and never evicting at all: a slot already kept can't be beaten by
// anything considered afterward.
func TestResolveConflicts_EvictThenLose_DoesNotLeaveEarlierEvictionCommitted(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	base := time.Date(2026, 1, 12, 9, 0, 0, 0, time.UTC)
	slotA := ScheduledSlot{StartTime: base, EndTime: base.Add(60 * time.Minute), Block: Block{Name: "A", Priority: 1}}
	slotC := ScheduledSlot{StartTime: base.Add(90 * time.Minute), EndTime: base.Add(150 * time.Minute), Block: Block{Name: "C", Priority: 3}}
	slotB := ScheduledSlot{StartTime: base.Add(30 * time.Minute), EndTime: base.Add(120 * time.Minute), Block: Block{Name: "B", Priority: 2}}

	require.False(t, slotsOverlap(slotA, slotC), "test setup: A and C must not overlap each other")
	require.True(t, slotsOverlap(slotA, slotB), "test setup: A and B must overlap")
	require.True(t, slotsOverlap(slotB, slotC), "test setup: B and C must overlap")

	resolved, dropped := engine.resolveConflicts([]ScheduledSlot{slotA, slotC, slotB})

	names := make([]string, len(resolved))
	for i, s := range resolved {
		names[i] = s.Block.Name
	}
	assert.ElementsMatch(t, []string{"A", "C"}, names,
		"A does not overlap the real survivor C and must be kept, even though B would have evicted it before B itself lost to C")

	require.Len(t, dropped, 1, "only B (which loses to C) should be dropped")
	assert.Equal(t, "B", dropped[0].BlockName)
	assert.Equal(t, "C", dropped[0].BlockingBlockName,
		"the warning must name C -- the actual final blocker -- not A's would-be evictor B, which itself never survived")
}

// TestGenerateForTimeRange_OnAirOccurrence_StaysInLineupAtOriginalStart
// pins finding 7's fix: every apply used to anchor a channel's shells at
// `start` alone, so an occurrence that began airing BEFORE this apply but
// hasn't finished yet -- the thing actually on screen right now -- was
// simply never generated (GenerateForTimeRange's normal sweep only starts
// at or after `start`), and the next pushed lineup replaced it outright.
// The fix adds an on-air shell to phase 1 whenever one exists, keyed by
// its own ORIGINAL StartTime (not `start`) -- see onAirOccurrenceStart.
//
// This uses a filter block, not series, specifically to isolate finding
// 7's shell-injection/anchor mechanism from findings 1/2/5's series-chain
// machinery (already covered by their own tests above): the mechanism
// itself is block-type-agnostic (added in phase 1, before the
// series-vs-filter dispatch in phase 3).
func TestGenerateForTimeRange_OnAirOccurrence_StaysInLineupAtOriginalStart(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	block := Block{
		Name: "Hourly Block", Type: BlockTypeFilter, Cron: "0 * * * *", Duration: 60, ChannelID: "channel-1",
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Movie One", Duration: 1_800_000, Type: "movie"},
		{ID: "p2", Title: "Movie Two", Duration: 1_800_000, Type: "movie"},
	}

	engine := NewEngine(client, []Block{block}, store, slog.Default(), time.UTC)

	onAirStart := time.Now().UTC().Truncate(time.Hour)
	firstNow := onAirStart.Add(20 * time.Minute) // 20min into the on-air occurrence

	firstSchedule, _, err := engine.GenerateForTimeRange(firstNow, firstNow.Add(24*time.Hour), availablePrograms)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())

	firstSlot := findSlotAt(t, firstSchedule["channel-1"], onAirStart)
	require.NotEmpty(t, firstSlot.Programs, "expected the on-air occurrence to have real content on its first apply")

	// Second apply, later -- the SAME occurrence is still on air (45min
	// in, still short of the 60min duration).
	secondNow := onAirStart.Add(45 * time.Minute)
	secondSchedule, _, err := engine.GenerateForTimeRange(secondNow, secondNow.Add(24*time.Hour), availablePrograms)
	require.NoError(t, err)

	secondSlot := findSlotAt(t, secondSchedule["channel-1"], onAirStart)
	assert.True(t, secondSlot.StartTime.Equal(onAirStart),
		"the on-air occurrence must keep its ORIGINAL StartTime as its anchor point -- not be shifted to this apply's own start -- so Tunarr's wall-clock playback formula still lands mid-episode")
	assert.Equal(t, programIDs(firstSlot.Programs), programIDs(secondSlot.Programs),
		"the on-air occurrence's content must be replayed verbatim from what was committed on the first apply, not re-planned")
}

// findSlotAt returns the slot in slots whose StartTime equals start,
// failing the test if none matches.
func findSlotAt(t *testing.T, slots []ScheduledSlot, start time.Time) ScheduledSlot {
	t.Helper()
	for _, s := range slots {
		if s.StartTime.Equal(start) {
			return s
		}
	}
	t.Fatalf("no slot found with StartTime %s", start)
	return ScheduledSlot{}
}

func TestCommit(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Add some pending states
	engine.pendingStates["Show1"] = &SeriesState{
		ShowTitle:      "Show1",
		CurrentSeason:  1,
		CurrentEpisode: 5,
	}
	engine.pendingStates["Show2"] = &SeriesState{
		ShowTitle:      "Show2",
		CurrentSeason:  2,
		CurrentEpisode: 3,
	}

	require.NoError(t, engine.Commit(), "Commit returned error")

	// Verify states were saved to store
	require.Len(t, store.States, 2, "Expected 2 states in store")

	state1, ok := store.States["Show1"]
	require.True(t, ok, "Expected Show1 in store")
	assert.Equal(t, 5, state1.CurrentEpisode, "Expected Show1 episode 5")

	// Verify pending states were cleared
	assert.Empty(t, engine.pendingStates, "Expected pending states to be cleared")
}

func TestFilterByHistory(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	// Add some history
	store.History = []ScheduleHistoryEntry{
		{
			ProgramID:   "p1",
			ChannelID:   "channel-1",
			ScheduledAt: time.Now().Add(-3 * 24 * time.Hour), // 3 days ago
			BlockName:   "Test Block",
		},
		{
			ProgramID:   "p2",
			ChannelID:   "channel-1",
			ScheduledAt: time.Now().Add(-10 * 24 * time.Hour), // 10 days ago
			BlockName:   "Test Block",
		},
	}

	engine := NewEngineWithOptions(client, []Block{}, store, EngineOptions{
		HistoryWindow: 7 * 24 * time.Hour,
		Logger:        slog.Default(),
		Location:      time.UTC,
	})

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Recent Show", Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Old Show", Duration: 1800000, Type: "episode"},
		{ID: "p3", Title: "New Show", Duration: 1800000, Type: "episode"},
	}

	filtered := engine.filterByHistory(availablePrograms, "channel-1")

	// p1 should be filtered out (aired 3 days ago, within 7 day window)
	// p2 should be included (aired 10 days ago, outside 7 day window)
	// p3 should be included (never aired)
	assert.Len(t, filtered, 2, "Expected 2 programs after filtering")

	// Verify p1 was filtered out
	for _, p := range filtered {
		assert.NotEqual(t, "p1", p.ID, "Expected p1 to be filtered out")
	}
}

func TestGetFiller_Success(t *testing.T) {
	// Create a test server that returns filler content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/filler-lists/filler-1/programs", r.URL.Path, "unexpected path")
		fillerContent := []tunarr.Program{
			{ID: "f1", Title: "Filler 1", Duration: 300000, Type: "track"},  // 5 min
			{ID: "f2", Title: "Filler 2", Duration: 600000, Type: "track"},  // 10 min
			{ID: "f3", Title: "Filler 3", Duration: 900000, Type: "track"},  // 15 min
			{ID: "f4", Title: "Filler 4", Duration: 1200000, Type: "track"}, // 20 min
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(fillerContent)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID: "filler-1",
		},
	}

	// Request 30 minutes of filler (1800000 ms)
	filler, err := engine.getFiller(block, 1800000, nil)
	require.NoError(t, err, "getFiller failed")
	assert.NotEmpty(t, filler, "Expected filler programs")

	totalDuration := int64(0)
	for _, f := range filler {
		totalDuration += f.GetDurationMs()
	}

	assert.LessOrEqual(t, totalDuration, int64(1800000), "Filler duration exceeds requested")
}

func TestGetFiller_NoFillerListID(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID: "",
		},
	}

	_, err := engine.getFiller(block, 1800000, nil)
	require.Error(t, err, "Expected error for empty filler list ID")
	assert.Contains(t, err.Error(), "no filler list ID")
}

func TestGetFiller_EmptyFillerList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty array
		err := json.NewEncoder(w).Encode([]tunarr.Program{})
		require.NoError(t, err)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID: "empty-filler",
		},
	}

	_, err := engine.getFiller(block, 1800000, nil)
	require.Error(t, err, "Expected error for empty filler list")
	assert.Contains(t, err.Error(), "is empty")
}

func TestGetFiller_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID: "filler-1",
		},
	}

	_, err := engine.getFiller(block, 1800000, nil)
	require.Error(t, err, "Expected error from API")
	assert.Contains(t, err.Error(), "failed to fetch filler")
}

func TestGetFiller_MaxFillerTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fillerContent := []tunarr.Program{
			{ID: "f1", Title: "Filler 1", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f2", Title: "Filler 2", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f3", Title: "Filler 3", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f4", Title: "Filler 4", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f5", Title: "Filler 5", Duration: 300000, Type: "track"}, // 5 min
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(fillerContent)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID:  "filler-1",
			MaxFillerTime: 10, // 10 minutes max
		},
	}

	// Request 30 minutes, but max filler is 10 minutes
	filler, err := engine.getFiller(block, 1800000, nil)
	require.NoError(t, err, "getFiller failed")

	totalDuration := int64(0)
	for _, f := range filler {
		totalDuration += f.GetDurationMs()
	}

	// Should not exceed 10 minutes (600000 ms)
	assert.LessOrEqual(t, totalDuration, int64(600000), "Filler duration exceeds max filler time")
}

func TestApplyBlockFiller_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fillerContent := []tunarr.Program{
			{ID: "f1", Title: "Filler 1", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f2", Title: "Filler 2", Duration: 300000, Type: "track"}, // 5 min
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(fillerContent)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			Enabled:      true,
			FillerListID: "filler-1",
			MinGapTime:   5, // 5 minutes minimum gap
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show 1", Duration: 1800000, Type: "episode"}, // 30 min
	}
	currentDuration := int64(1800000)
	targetDuration := int64(2400000) // 40 minutes target, 10 minute gap

	playlist, finalDuration := engine.applyBlockFiller(block, initialPlaylist, currentDuration, targetDuration, nil)

	assert.Greater(t, len(playlist), len(initialPlaylist), "Expected filler to be added to playlist")
	assert.Greater(t, finalDuration, currentDuration, "Expected duration to increase after adding filler")
	assert.LessOrEqual(t, finalDuration, targetDuration, "Filler exceeded target duration")
}

func TestApplyBlockFiller_GapTooSmall(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			Enabled:    true,
			MinGapTime: 10, // 10 minutes minimum
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show 1", Duration: 1800000, Type: "episode"}, // 30 min
	}
	currentDuration := int64(1800000)
	targetDuration := int64(2100000) // 35 minutes target, 5 minute gap (less than minimum)

	playlist, finalDuration := engine.applyBlockFiller(block, initialPlaylist, currentDuration, targetDuration, nil)

	// Should not add filler because gap is too small
	assert.Len(t, playlist, len(initialPlaylist), "Expected no filler to be added (gap too small)")
	assert.Equal(t, currentDuration, finalDuration, "Expected duration to remain unchanged")
}

func TestApplyBlockFiller_FillerDisabled(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			Enabled: false,
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show 1", Duration: 1800000, Type: "episode"},
	}
	currentDuration := int64(1800000)
	targetDuration := int64(3600000)

	playlist, finalDuration := engine.applyBlockFiller(block, initialPlaylist, currentDuration, targetDuration, nil)

	assert.Len(t, playlist, len(initialPlaylist), "Expected no filler to be added (filler disabled)")
	assert.Equal(t, currentDuration, finalDuration, "Expected duration to remain unchanged")
}

func TestApplySeriesFallback_FillerMode(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "f1", Title: "Filler Show", Duration: 600000, Type: "episode", Genres: []tunarr.Genre{{Name: "Comedy"}}},
		{ID: "f2", Title: "Another Filler", Duration: 600000, Type: "episode", Genres: []tunarr.Genre{{Name: "Comedy"}}},
	}

	block := Block{
		Fallback: SeriesFallback{
			Mode: FallbackModeFiller,
			FillerFilter: Filter{
				Genres: []string{"Comedy"},
			},
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Series Episode", Duration: 1800000, Type: "episode"},
	}
	currentDuration := int64(1800000)
	targetDuration := int64(3000000) // 50 minutes, needs 20 minutes more

	playlist, finalDuration := engine.applySeriesFallback(block, availablePrograms, initialPlaylist, durationBudget{current: currentDuration, target: targetDuration}, nil)

	assert.Greater(t, len(playlist), len(initialPlaylist), "Expected fallback filler to be added")
	assert.Greater(t, finalDuration, currentDuration, "Expected duration to increase after fallback")
}

func TestApplySeriesFallback_NoFallbackNeeded(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Series Episode", Duration: 1800000, Type: "episode"},
	}
	currentDuration := int64(1800000)
	targetDuration := int64(1800000) // Already at target

	playlist, finalDuration := engine.applySeriesFallback(Block{}, []tunarr.Program{}, initialPlaylist, durationBudget{current: currentDuration, target: targetDuration}, nil)

	assert.Len(t, playlist, len(initialPlaylist), "Expected no fallback when at target duration")
	assert.Equal(t, currentDuration, finalDuration, "Expected duration to remain unchanged")
}

func TestApplySeriesFallback_NotFillerMode(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "f1", Title: "Filler Show", Duration: 600000, Type: "episode"},
	}

	block := Block{
		Fallback: SeriesFallback{
			Mode: "", // Not filler mode
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Series Episode", Duration: 1800000, Type: "episode"},
	}
	currentDuration := int64(1800000)
	targetDuration := int64(3000000)

	playlist, finalDuration := engine.applySeriesFallback(block, availablePrograms, initialPlaylist, durationBudget{current: currentDuration, target: targetDuration}, nil)

	assert.Len(t, playlist, len(initialPlaylist), "Expected no fallback when not in filler mode")
	assert.Equal(t, currentDuration, finalDuration, "Expected duration to remain unchanged")
}

func TestInitializeSeriesState_NewStateWithStartSeasonAndEpisode(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		LastAired:      nil, // nil, indicating new state
	}

	config := SeriesConfig{
		ShowTitle:    "Test Show",
		StartSeason:  3,
		StartEpisode: 5,
	}

	engine.initializeSeriesState(state, config)

	assert.Equal(t, 3, state.CurrentSeason, "Expected CurrentSeason to be 3")
	assert.Equal(t, 5, state.CurrentEpisode, "Expected CurrentEpisode to be 5")
}

func TestInitializeSeriesState_ExistingStateNotModified(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	now := time.Now()
	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  2,
		CurrentEpisode: 7,
		LastAired:      &now, // Non-nil, indicating existing state
	}

	config := SeriesConfig{
		ShowTitle:    "Test Show",
		StartSeason:  3,
		StartEpisode: 5,
	}

	engine.initializeSeriesState(state, config)

	// State should not be modified when LastAired is not zero
	assert.Equal(t, 2, state.CurrentSeason, "Expected CurrentSeason to remain 2")
	assert.Equal(t, 7, state.CurrentEpisode, "Expected CurrentEpisode to remain 7")
}

func TestInitializeSeriesState_StartSeasonOnly(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		LastAired:      nil,
	}

	config := SeriesConfig{
		ShowTitle:   "Test Show",
		StartSeason: 4,
		// StartEpisode not specified (0)
	}

	engine.initializeSeriesState(state, config)

	assert.Equal(t, 4, state.CurrentSeason, "Expected CurrentSeason to be 4")
	assert.Equal(t, 1, state.CurrentEpisode, "Expected CurrentEpisode to remain 1")
}

func TestInitializeSeriesState_StartEpisodeOnly(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		LastAired:      nil,
	}

	config := SeriesConfig{
		ShowTitle:    "Test Show",
		StartEpisode: 10,
		// StartSeason not specified (0)
	}

	engine.initializeSeriesState(state, config)

	assert.Equal(t, 1, state.CurrentSeason, "Expected CurrentSeason to remain 1")
	assert.Equal(t, 10, state.CurrentEpisode, "Expected CurrentEpisode to be 10")
}

func TestInitializeSeriesState_NoStartConfig(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		LastAired:      nil,
	}

	config := SeriesConfig{
		ShowTitle: "Test Show",
		// No StartSeason or StartEpisode
	}

	engine.initializeSeriesState(state, config)

	// State should remain unchanged when no start config is provided
	assert.Equal(t, 1, state.CurrentSeason, "Expected CurrentSeason to remain 1")
	assert.Equal(t, 1, state.CurrentEpisode, "Expected CurrentEpisode to remain 1")
}

func TestNewEngineWithOptions_AllOptionsProvided(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	logger := slog.Default()
	loc := time.FixedZone("Test", 3600)
	historyWindow := 5 * 24 * time.Hour

	opts := EngineOptions{
		Logger:        logger,
		Location:      loc,
		HistoryWindow: historyWindow,
	}

	engine := NewEngineWithOptions(client, []Block{}, store, opts)

	assert.Equal(t, logger, engine.logger, "Expected custom logger to be used")
	assert.Equal(t, loc, engine.location, "Expected custom location to be used")
	assert.Equal(t, historyWindow, engine.history.Window(), "Expected custom history window")
}

func TestNewEngineWithOptions_DefaultLogger(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()

	opts := EngineOptions{
		Logger: nil, // Should use slog.Default()
	}

	engine := NewEngineWithOptions(client, []Block{}, store, opts)

	assert.NotNil(t, engine.logger, "Expected logger to be set to default")
}

func TestNewEngineWithOptions_DefaultLocation(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()

	opts := EngineOptions{
		Location: nil, // Should use time.Local
	}

	engine := NewEngineWithOptions(client, []Block{}, store, opts)

	assert.Equal(t, time.Local, engine.location, "Expected location to be set to time.Local")
}

func TestNewEngineWithOptions_DefaultHistoryWindow(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()

	opts := EngineOptions{
		HistoryWindow: 0, // Should use 7 days default
	}

	engine := NewEngineWithOptions(client, []Block{}, store, opts)

	expectedWindow := 7 * 24 * time.Hour
	assert.Equal(t, expectedWindow, engine.history.Window(), "Expected history window to be 7 days")
}

func TestFindNextSeriesEpisode_FindsCurrent(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 2,
	}

	config := SeriesConfig{
		ShowTitle: "Test Show",
	}

	availablePrograms := []tunarr.Program{
		{ID: "e1", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000},
		{ID: "e2", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1800000},
		{ID: "e3", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1800000},
	}

	ep := engine.findNextSeriesEpisode(engineSeriesContext{engine}, config, state, availablePrograms)

	require.NotNil(t, ep, "Expected to find episode")
	assert.Equal(t, "e2", ep.ID, "Expected episode e2")
}

func TestFindNextSeriesEpisode_SkipsEpisodes(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 2,
	}

	config := SeriesConfig{
		ShowTitle:    "Test Show",
		SkipEpisodes: []string{"S01E02", "S01E03"},
	}

	availablePrograms := []tunarr.Program{
		{ID: "e1", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000},
		{ID: "e2", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1800000},
		{ID: "e3", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 3, Duration: 1800000},
		{ID: "e4", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 4, Duration: 1800000},
	}

	ep := engine.findNextSeriesEpisode(engineSeriesContext{engine}, config, state, availablePrograms)

	require.NotNil(t, ep, "Expected to find episode")
	assert.Equal(t, "e4", ep.ID, "Expected episode e4 after skipping e2 and e3")
	assert.Equal(t, 4, state.CurrentEpisode, "Expected CurrentEpisode to be 4 after skipping")
}

func TestFindNextSeriesEpisode_AdvancesToNextSeason(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 5,
	}

	config := SeriesConfig{
		ShowTitle: "Test Show",
	}

	availablePrograms := []tunarr.Program{
		{ID: "e1", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000},
		{ID: "s2e1", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 2, EpisodeNumber: 1, Duration: 1800000},
		{ID: "s2e2", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 2, EpisodeNumber: 2, Duration: 1800000},
	}

	ep := engine.findNextSeriesEpisode(engineSeriesContext{engine}, config, state, availablePrograms)

	require.NotNil(t, ep, "Expected to find next season episode")
	assert.Equal(t, "s2e1", ep.ID, "Expected season 2 episode 1")
	assert.Equal(t, 2, state.CurrentSeason, "Expected CurrentSeason to be 2")
	assert.Equal(t, 1, state.CurrentEpisode, "Expected CurrentEpisode to be 1 for new season")
}

func TestFindNextSeriesEpisode_MarksCompleteWhenNoneFound(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  3,
		CurrentEpisode: 1,
		Completed:      false,
	}

	config := SeriesConfig{
		ShowTitle: "Test Show",
	}

	availablePrograms := []tunarr.Program{
		{ID: "e1", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000},
		{ID: "s2e1", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 2, EpisodeNumber: 1, Duration: 1800000},
	}

	ep := engine.findNextSeriesEpisode(engineSeriesContext{engine}, config, state, availablePrograms)

	assert.Nil(t, ep, "Expected nil when no episodes found")
	assert.True(t, state.Completed, "Expected series to be marked as completed")
}

func TestFindNextSeriesEpisode_SkipsFirstEpisodeOfNewSeason(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 3,
	}

	config := SeriesConfig{
		ShowTitle:    "Test Show",
		SkipEpisodes: []string{"S02E01"}, // Skip first episode of season 2
	}

	availablePrograms := []tunarr.Program{
		{ID: "s1e1", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1800000},
		{ID: "s2e1", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 2, EpisodeNumber: 1, Duration: 1800000},
		{ID: "s2e2", Type: "episode", ShowTitle: "Test Show", SeasonNumber: 2, EpisodeNumber: 2, Duration: 1800000},
	}

	ep := engine.findNextSeriesEpisode(engineSeriesContext{engine}, config, state, availablePrograms)

	require.NotNil(t, ep, "Expected to find episode after skipping S02E01")
	assert.Equal(t, "s2e2", ep.ID, "Expected S02E02")
	assert.Equal(t, 2, state.CurrentSeason, "Expected season 2")
	assert.Equal(t, 2, state.CurrentEpisode, "Expected episode 2")
}

func TestGetSeriesState_FromPendingStates(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	expectedState := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  2,
		CurrentEpisode: 5,
	}

	engine.pendingStates["Test Show"] = expectedState

	state, err := engine.getSeriesState("Test Show")

	require.NoError(t, err, "Expected no error")
	assert.Equal(t, expectedState, state, "Expected to get pending state")
}

func TestGetSeriesState_FromStore(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Add state to store
	storeState := &SeriesState{
		ShowTitle:      "Stored Show",
		CurrentSeason:  1,
		CurrentEpisode: 3,
	}
	_ = store.UpdateSeriesState(context.Background(), storeState)

	state, err := engine.getSeriesState("Stored Show")

	require.NoError(t, err, "Expected no error")
	assert.Equal(t, "Stored Show", state.ShowTitle, "Expected to get state from store")
	assert.Equal(t, 1, state.CurrentSeason, "Expected season 1")
	assert.Equal(t, 3, state.CurrentEpisode, "Expected episode 3")
}

func TestPlanBlock_HistoryFallbackAllowsRepeats(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	// Set up history so all programs are recently scheduled
	store.History = []ScheduleHistoryEntry{
		{ProgramID: "prog-1", ChannelID: "channel-1", ScheduledAt: time.Now()},
		{ProgramID: "prog-2", ChannelID: "channel-1", ScheduledAt: time.Now()},
	}

	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Name:      "Test Block",
		Duration:  30,
		ChannelID: "channel-1",
		Filter: Filter{
			Genres: []string{"Comedy"},
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "prog-1", Title: "Show A", Duration: 1800000, Genres: []tunarr.Genre{{Name: "Comedy"}}, Type: "episode"},
		{ID: "prog-2", Title: "Show B", Duration: 1800000, Genres: []tunarr.Genre{{Name: "Comedy"}}, Type: "episode"},
	}

	// Should still return content (allows repeats when all filtered)
	playlist, err := engine.PlanBlock(block, availablePrograms, time.Now(), time.Now())
	require.NoError(t, err, "PlanBlock should allow repeats when history filters everything")
	assert.NotEmpty(t, playlist, "Expected playlist to contain repeated programs")
}

func TestApplySeriesFallback_FilterErrorLogged(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Create a block with invalid filter (negative duration range)
	block := Block{
		ChannelID: "channel-1",
		Name:      "Test Block",
		Fallback: SeriesFallback{
			Mode: FallbackModeFiller,
			FillerFilter: Filter{
				MinDuration: 100,
				MaxDuration: 10, // Invalid: min > max
			},
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show", Duration: 1800000, Type: "episode"},
	}

	// Should not panic, just log and return original
	playlist, duration := engine.applySeriesFallback(block, []tunarr.Program{}, initialPlaylist, durationBudget{current: 1800000, target: 3600000}, nil)

	assert.Len(t, playlist, 1, "Expected original playlist returned on filter error")
	assert.Equal(t, int64(1800000), duration, "Expected duration unchanged on filter error")
}

func TestApplySeriesFallback_NoMatchingContent(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Programs don't match the filter
	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Drama Show", Duration: 1800000, Genres: []tunarr.Genre{{Name: "Drama"}}, Type: "episode"},
	}

	block := Block{
		ChannelID: "channel-1",
		Name:      "Test Block",
		Fallback: SeriesFallback{
			Mode: FallbackModeFiller,
			FillerFilter: Filter{
				Genres: []string{"Comedy"}, // Won't match Drama
			},
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p0", Title: "Initial", Duration: 1800000, Type: "episode"},
	}

	playlist, duration := engine.applySeriesFallback(block, availablePrograms, initialPlaylist, durationBudget{current: 1800000, target: 3600000}, nil)

	assert.Len(t, playlist, 1, "Expected original playlist when no content matches filter")
	assert.Equal(t, int64(1800000), duration, "Expected duration unchanged when no content matches")
}

func TestApplyBlockFiller_FetchErrorLogged(t *testing.T) {
	// Create a server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		ChannelID: "channel-1",
		Name:      "Test Block",
		Filler: FillerConfig{
			Enabled:      true,
			FillerListID: "filler-1",
			MinGapTime:   5,
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show", Duration: 1800000, Type: "episode"},
	}

	// Should not panic, just return original playlist
	playlist, duration := engine.applyBlockFiller(block, initialPlaylist, 1800000, 3600000, nil)

	assert.Len(t, playlist, 1, "Expected original playlist returned on filler fetch error")
	assert.Equal(t, int64(1800000), duration, "Expected duration unchanged on filler fetch error")
}

func TestApplyBlockFiller_EmptyFillerList(t *testing.T) {
	// Create a server that returns empty filler list
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tunarr.Program{})
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		ChannelID: "channel-1",
		Name:      "Test Block",
		Filler: FillerConfig{
			Enabled:      true,
			FillerListID: "filler-1",
			MinGapTime:   5,
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show", Duration: 1800000, Type: "episode"},
	}

	// Should not panic, just return original playlist
	playlist, duration := engine.applyBlockFiller(block, initialPlaylist, 1800000, 3600000, nil)

	assert.Len(t, playlist, 1, "Expected original playlist returned on empty filler list")
	assert.Equal(t, int64(1800000), duration, "Expected duration unchanged on empty filler list")
}
