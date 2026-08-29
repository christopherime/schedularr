package scheduler

import (
	"context"
	"encoding/json"
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
