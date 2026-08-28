package blockio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/christopherime/schedularr/internal/store"
	"github.com/google/uuid"
)

// Bootstrap performs first-run import of a static scheduler YAML file into
// the block store.
//
// Behavior:
//   - If the store already has one or more blocks, Bootstrap is a no-op and
//     returns (0, nil): the store is the source of truth once it has been
//     populated, and a static file must never silently re-seed or duplicate
//     data on every restart.
//   - If the store is empty but path does not exist, Bootstrap is a no-op
//     and returns (0, nil): there is nothing to import.
//   - If the store is empty and path exists, Bootstrap parses and validates
//     the file (via ParseYAML) and creates one BlockRecord per block, with a
//     generated UUID as the ID, the block's Name, and Enabled set to true.
//     It returns the number of blocks imported.
//   - If the file exists but fails to parse or validate, Bootstrap returns
//     an error and imports nothing: validation happens on the whole file
//     before any block is written, so a bad file can never leave the store
//     half-imported.
func Bootstrap(ctx context.Context, s *store.Store, path string, logger *slog.Logger) (int, error) {
	if logger == nil {
		logger = slog.Default()
	}

	existing, err := s.CountBlocks(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count existing blocks: %w", err)
	}
	if existing != 0 {
		logger.Debug("block bootstrap skipped: store already has blocks", "existing_count", existing)
		return 0, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Debug("block bootstrap skipped: no bootstrap file", "path", path)
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read bootstrap file %q: %w", path, err)
	}

	blocks, err := ParseYAML(data)
	if err != nil {
		return 0, fmt.Errorf("failed to parse bootstrap file %q: %w", path, err)
	}

	imported := 0
	for _, block := range blocks {
		rec := &store.BlockRecord{
			ID:      uuid.NewString(),
			Name:    block.Name,
			Enabled: true,
			Spec:    block,
		}
		if err := s.CreateBlock(ctx, rec); err != nil {
			return imported, fmt.Errorf("failed to import block %q from %q: %w", block.Name, path, err)
		}
		imported++
	}

	logger.Info("bootstrap imported scheduler blocks",
		"path", path,
		"block_count", imported)

	return imported, nil
}
