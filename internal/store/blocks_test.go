package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestStore opens a fresh temp-file backed store for a single test.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleBlock(id, name string) *store.BlockRecord {
	return &store.BlockRecord{
		ID:      id,
		Name:    name,
		Enabled: true,
		Spec: scheduler.Block{
			Type:      scheduler.BlockTypeSeries,
			Name:      name,
			Cron:      "0 20 * * 6",
			Duration:  90,
			ChannelID: "ch1",
			Priority:  10,
			Series: []scheduler.SeriesConfig{
				{ShowTitle: "Show A", EpisodesPerBlock: 1},
			},
		},
	}
}

func TestBlockCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rec := &store.BlockRecord{
		ID: "b1", Name: "saturday-night", Enabled: true,
		Spec: scheduler.Block{
			Type: scheduler.BlockTypeSeries, Name: "saturday-night",
			Cron: "0 20 * * 6", Duration: 90, ChannelID: "ch1", Priority: 10,
			Series: []scheduler.SeriesConfig{{ShowTitle: "Show A", EpisodesPerBlock: 1}},
		},
	}

	require.NoError(t, s.CreateBlock(ctx, rec), "create")

	got, err := s.GetBlock(ctx, "b1")
	require.NoError(t, err, "get")
	require.Len(t, got.Spec.Series, 1)
	assert.Equal(t, "Show A", got.Spec.Series[0].ShowTitle, "spec lost: %+v", got.Spec)
	assert.Equal(t, rec.Name, got.Name)
	assert.True(t, got.Enabled)
	assert.False(t, got.CreatedAt.IsZero(), "CreatedAt should be set")
	assert.False(t, got.UpdatedAt.IsZero(), "UpdatedAt should be set")

	err = s.CreateBlock(ctx, rec)
	require.Error(t, err, "want ErrConflict")
	assert.True(t, errors.Is(err, store.ErrConflict), "want ErrConflict, got %v", err)

	_, err = s.GetBlock(ctx, "nope")
	require.Error(t, err, "want ErrNotFound")
	assert.True(t, errors.Is(err, store.ErrNotFound), "want ErrNotFound, got %v", err)
}

func TestBlockCRUD_SpecRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rec := &store.BlockRecord{
		ID:      "b-full",
		Name:    "full-spec",
		Enabled: true,
		Spec: scheduler.Block{
			Type:      scheduler.BlockTypeSeries,
			Name:      "full-spec",
			Cron:      "0 20 * * 6",
			Duration:  90,
			ChannelID: "ch1",
			Priority:  10,
			Filter: scheduler.Filter{
				TitlePattern: "^The.*",
				Genres:       []string{"drama", "comedy"},
				YearFrom:     1990,
				YearTo:       2020,
			},
			MaxDurationOverflowMinutes: 5,
			Filler: scheduler.FillerConfig{
				Enabled:       true,
				FillerListID:  "filler-1",
				MaxFillerTime: 15,
				MinGapTime:    2,
			},
			Series: []scheduler.SeriesConfig{
				{
					ShowTitle:        "Show A",
					EpisodesPerBlock: 2,
					StartSeason:      1,
					StartEpisode:     3,
					OnComplete:       scheduler.CompletionActionRestart,
					SkipEpisodes:     []string{"S01E05"},
					MaxRuns:          3,
				},
				{ShowTitle: "Show B", EpisodesPerBlock: 1},
			},
			Fallback: scheduler.SeriesFallback{
				Mode: scheduler.FallbackModeFiller,
				FillerFilter: scheduler.Filter{
					Genres: []string{"filler-genre"},
				},
			},
		},
	}

	require.NoError(t, s.CreateBlock(ctx, rec), "create")

	got, err := s.GetBlock(ctx, "b-full")
	require.NoError(t, err, "get")
	assert.Equal(t, rec.Spec, got.Spec, "spec should survive JSON round-trip intact")
}

func TestGetBlock_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetBlock(ctx, "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, store.ErrNotFound))
}

func TestUpdateBlock_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.UpdateBlock(ctx, sampleBlock("missing", "missing-block"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, store.ErrNotFound))
}

func TestDeleteBlock_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.DeleteBlock(ctx, "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, store.ErrNotFound))
}

func TestDeleteBlock(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rec := sampleBlock("b1", "block-one")
	require.NoError(t, s.CreateBlock(ctx, rec))

	require.NoError(t, s.DeleteBlock(ctx, "b1"))

	_, err := s.GetBlock(ctx, "b1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, store.ErrNotFound))
}

func TestUpdateBlock_EnabledToggleAndFieldsPersist(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rec := sampleBlock("b1", "block-one")
	require.NoError(t, s.CreateBlock(ctx, rec))
	createdAt := rec.CreatedAt

	got, err := s.GetBlock(ctx, "b1")
	require.NoError(t, err)
	require.True(t, got.Enabled, "expected enabled true after create")

	got.Enabled = false
	got.Spec.Duration = 120
	got.Spec.Priority = 20
	require.NoError(t, s.UpdateBlock(ctx, got))

	updated, err := s.GetBlock(ctx, "b1")
	require.NoError(t, err)
	assert.False(t, updated.Enabled, "enabled toggle should persist as false")
	assert.Equal(t, 120, updated.Spec.Duration)
	assert.Equal(t, 20, updated.Spec.Priority)
	assert.Equal(t, createdAt.UTC(), updated.CreatedAt.UTC(), "CreatedAt should not change on update")
	assert.False(t, updated.UpdatedAt.Before(createdAt), "UpdatedAt should not go backwards")

	// Toggle back on and verify persistence again.
	updated.Enabled = true
	require.NoError(t, s.UpdateBlock(ctx, updated))

	final, err := s.GetBlock(ctx, "b1")
	require.NoError(t, err)
	assert.True(t, final.Enabled, "enabled toggle should persist as true")
}

func TestUpdateBlock_DuplicateNameConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateBlock(ctx, sampleBlock("b1", "block-one")))
	require.NoError(t, s.CreateBlock(ctx, sampleBlock("b2", "block-two")))

	rec2, err := s.GetBlock(ctx, "b2")
	require.NoError(t, err)
	rec2.Name = "block-one"

	err = s.UpdateBlock(ctx, rec2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, store.ErrConflict), "want ErrConflict, got %v", err)
}

func TestListBlocks_OrderingAndEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	empty, err := s.ListBlocks(ctx)
	require.NoError(t, err)
	assert.Empty(t, empty, "expected no blocks in a fresh store")

	names := []string{"zeta-block", "alpha-block", "mid-block"}
	for i, name := range names {
		rec := sampleBlock(name, name)
		rec.ID = name
		_ = i
		require.NoError(t, s.CreateBlock(ctx, rec))
	}

	list, err := s.ListBlocks(ctx)
	require.NoError(t, err)
	require.Len(t, list, 3)

	got := make([]string, len(list))
	for i, r := range list {
		got[i] = r.Name
	}
	assert.Equal(t, []string{"alpha-block", "mid-block", "zeta-block"}, got, "expected blocks ordered by name")
}

func TestCountBlocks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	count, err := s.CountBlocks(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "expected 0 blocks in fresh store")

	require.NoError(t, s.CreateBlock(ctx, sampleBlock("b1", "block-one")))
	require.NoError(t, s.CreateBlock(ctx, sampleBlock("b2", "block-two")))

	count, err = s.CountBlocks(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "expected 2 blocks after create")

	require.NoError(t, s.DeleteBlock(ctx, "b1"))

	count, err = s.CountBlocks(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "expected 1 block after delete")
}

func TestBlockCRUD_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		setup   func(t *testing.T, s *store.Store, ctx context.Context)
		wantErr error
	}{
		{
			name: "create then get succeeds",
			id:   "tbl-1",
			setup: func(t *testing.T, s *store.Store, ctx context.Context) {
				t.Helper()
				require.NoError(t, s.CreateBlock(ctx, sampleBlock("tbl-1", "tbl-block-1")))
			},
			wantErr: nil,
		},
		{
			name:    "get missing id returns ErrNotFound",
			id:      "tbl-missing",
			setup:   func(t *testing.T, s *store.Store, ctx context.Context) {},
			wantErr: store.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			tt.setup(t, s, ctx)

			_, err := s.GetBlock(ctx, tt.id)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)
		})
	}
}
