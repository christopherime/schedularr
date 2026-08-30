// Package blockio provides CUE-validated YAML import/export for scheduling
// blocks, plus first-run bootstrap of a static scheduler file into the
// block store (see Bootstrap in bootstrap.go).
package blockio

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/christopherime/schedularr/internal/cueconfig"
	"github.com/christopherime/schedularr/internal/scheduler"
	"gopkg.in/yaml.v3"
)

// ValidateBlocks validates blocks against the scheduler CUE schema. It
// renders the blocks to YAML (via RenderYAML) and runs the result through
// cueconfig's scheduler validator, wrapping any resulting CUE error.
func ValidateBlocks(blocks []scheduler.Block) error {
	data, err := RenderYAML(blocks)
	if err != nil {
		return fmt.Errorf("failed to render blocks for validation: %w", err)
	}

	validator := cueconfig.NewValidator()
	if err := validator.ValidateScheduler(data, "yaml"); err != nil {
		return fmt.Errorf("failed to validate blocks: %w", err)
	}

	return nil
}

// ParseYAML validates YAML data against the CUE scheduler schema and
// strictly decodes it into scheduling blocks. Decoding is strict: unknown
// fields anywhere in the document are rejected rather than silently
// ignored.
//
// CUE validation runs on the RAW input bytes, before the Go-struct decode
// -- not on a re-render of the decoded structs (what ValidateBlocks does,
// and what an earlier version of this function called). The ordering
// matters for CUE's `*default` semantics: a default applies only to an
// ABSENT field, and decoding into a Go struct first turns an omitted
// `type:` into an explicit `""`, which then fails #Block's
// `"filter" | "series" | *"filter"` disjunction on the re-render even
// though the original file was valid. Validating the original bytes lets a
// genuinely-omitted field stay absent, so the default applies exactly as
// the schema declares; the decode below then mirrors that default into the
// struct (Type "" -> "filter"). An EXPLICIT `type: ""` in the file still
// fails the disjunction, as it should.
//
// ParseYAML also rejects blocks that share a name. This lives here rather
// than in the CUE schema or the store: CUE validates each block against
// #Block independently and has no notion of "list of blocks with unique
// names," so two same-named blocks in one file would otherwise sail
// through validation cleanly. Catching it here -- before any block from
// this file is written anywhere -- protects every YAML-driven import path
// (Bootstrap, POST /blocks/import) uniformly: a duplicate never gets far
// enough to hit store.ErrConflict partway through a batch of writes.
func ParseYAML(data []byte) ([]scheduler.Block, error) {
	validator := cueconfig.NewValidator()
	if err := validator.ValidateScheduler(data, "yaml"); err != nil {
		return nil, fmt.Errorf("failed to validate blocks: %w", err)
	}

	var cfg scheduler.Config

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode blocks YAML: %w", err)
	}

	if dupes := duplicateBlockNames(cfg.Blocks); len(dupes) > 0 {
		return nil, fmt.Errorf("failed to validate blocks: duplicate block name(s): %s", strings.Join(dupes, ", "))
	}

	// Mirror #Block's `*"filter"` default into the decoded structs: raw
	// validation above accepted an omitted `type`, so fill in what the
	// schema says an absent field means (matching the JSON API path's
	// normalization in internal/api's fromGen).
	for i := range cfg.Blocks {
		if cfg.Blocks[i].Type == "" {
			cfg.Blocks[i].Type = scheduler.BlockTypeFilter
		}
	}

	return cfg.Blocks, nil
}

// duplicateBlockNames returns the names that appear more than once in
// blocks, in first-duplicated order, deduplicated. An empty result means
// every block has a unique name.
func duplicateBlockNames(blocks []scheduler.Block) []string {
	seen := make(map[string]int, len(blocks))
	var dupes []string
	for _, b := range blocks {
		seen[b.Name]++
		if seen[b.Name] == 2 {
			dupes = append(dupes, b.Name)
		}
	}
	return dupes
}

// RenderYAML renders blocks as YAML, wrapped in the scheduler.Config
// envelope used by scheduler files. Because it marshals a typed struct
// (scheduler.Config) rather than a map, field order is stable across calls:
// it always follows the struct's declared field order, not map iteration
// order.
func RenderYAML(blocks []scheduler.Block) ([]byte, error) {
	cfg := scheduler.Config{Blocks: blocks}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to render blocks to YAML: %w", err)
	}

	return data, nil
}
