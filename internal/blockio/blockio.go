// Package blockio provides CUE-validated YAML import/export for scheduling
// blocks, plus first-run bootstrap of a static scheduler file into the
// block store (see Bootstrap in bootstrap.go).
package blockio

import (
	"bytes"
	"fmt"

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

// ParseYAML strictly decodes YAML data into scheduling blocks and validates
// the result against the CUE schema via ValidateBlocks. Decoding is strict:
// unknown fields anywhere in the document are rejected rather than silently
// ignored.
func ParseYAML(data []byte) ([]scheduler.Block, error) {
	var cfg scheduler.Config

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode blocks YAML: %w", err)
	}

	if err := ValidateBlocks(cfg.Blocks); err != nil {
		return nil, err
	}

	return cfg.Blocks, nil
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
