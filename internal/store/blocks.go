package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/christopherime/schedularr/internal/scheduler"
	sqlite3 "github.com/mattn/go-sqlite3"
)

// ErrNotFound indicates the requested record does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict indicates a uniqueness constraint was violated (e.g. duplicate name).
var ErrConflict = errors.New("conflict")

// BlockRecord is the persisted representation of a scheduling block.
// Blocks are the API-editable source of truth for scheduling, replacing
// the static blocks previously defined in scheduler.yaml.
type BlockRecord struct {
	ID        string // uuid string
	Name      string // unique
	Enabled   bool
	Spec      scheduler.Block // full block definition
	CreatedAt time.Time
	UpdatedAt time.Time
}

// blockRow is the sqlx-scannable row shape for the blocks table.
type blockRow struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	Enabled   bool      `db:"enabled"`
	SpecJSON  string    `db:"spec_json"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (r blockRow) toRecord() (*BlockRecord, error) {
	var spec scheduler.Block
	if err := json.Unmarshal([]byte(r.SpecJSON), &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block spec: %w", err)
	}
	return &BlockRecord{
		ID:        r.ID,
		Name:      r.Name,
		Enabled:   r.Enabled,
		Spec:      spec,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}, nil
}

// ListBlocks returns all blocks ordered by name.
func (s *Store) ListBlocks(ctx context.Context) ([]BlockRecord, error) {
	var rows []blockRow
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT id, name, enabled, spec_json, created_at, updated_at
		FROM blocks ORDER BY name`); err != nil {
		return nil, fmt.Errorf("failed to list blocks: %w", err)
	}

	records := make([]BlockRecord, 0, len(rows))
	for _, row := range rows {
		rec, err := row.toRecord()
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

// GetBlock retrieves a block by ID. Returns ErrNotFound if it does not exist.
func (s *Store) GetBlock(ctx context.Context, id string) (*BlockRecord, error) {
	var row blockRow
	err := s.db.GetContext(ctx, &row, `
		SELECT id, name, enabled, spec_json, created_at, updated_at
		FROM blocks WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}
	return row.toRecord()
}

// CreateBlock inserts a new block, setting CreatedAt/UpdatedAt.
// Returns ErrConflict if a block with the same name already exists.
func (s *Store) CreateBlock(ctx context.Context, rec *BlockRecord) error {
	specJSON, err := json.Marshal(rec.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal block spec: %w", err)
	}

	now := time.Now().UTC()
	rec.CreatedAt = now
	rec.UpdatedAt = now

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO blocks (id, name, enabled, spec_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Name, rec.Enabled, string(specJSON), rec.CreatedAt, rec.UpdatedAt)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrConflict
		}
		return fmt.Errorf("failed to create block: %w", err)
	}
	return nil
}

// UpdateBlock updates an existing block's fields and bumps UpdatedAt.
// Returns ErrNotFound if no block with the given ID exists, or ErrConflict
// if the update would violate the unique name constraint.
func (s *Store) UpdateBlock(ctx context.Context, rec *BlockRecord) error {
	specJSON, err := json.Marshal(rec.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal block spec: %w", err)
	}

	rec.UpdatedAt = time.Now().UTC()

	result, err := s.db.ExecContext(ctx, `
		UPDATE blocks SET name = ?, enabled = ?, spec_json = ?, updated_at = ?
		WHERE id = ?`,
		rec.Name, rec.Enabled, string(specJSON), rec.UpdatedAt, rec.ID)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrConflict
		}
		return fmt.Errorf("failed to update block: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check update result: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBlock removes a block by ID. Returns ErrNotFound if it does not exist.
func (s *Store) DeleteBlock(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM blocks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete block: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check delete result: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountBlocks returns the total number of blocks.
func (s *Store) CountBlocks(ctx context.Context) (int64, error) {
	var count int64
	if err := s.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM blocks`); err != nil {
		return 0, fmt.Errorf("failed to count blocks: %w", err)
	}
	return count, nil
}

// isUniqueConstraintErr reports whether err is a SQLite UNIQUE constraint violation.
func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
