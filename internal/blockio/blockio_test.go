package blockio_test

import (
	"strings"
	"testing"

	"github.com/christopherime/schedularr/internal/blockio"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// filterBlock returns a fully-populated filter-type block used across
// round-trip and validation tests.
func filterBlock() scheduler.Block {
	return scheduler.Block{
		Type:      scheduler.BlockTypeFilter,
		Name:      "Morning Cartoons",
		Cron:      "0 6 * * *",
		Duration:  120,
		ChannelID: "channel-1",
		Priority:  10,
		Filter: scheduler.Filter{
			Genres:      []string{"Animation", "Family"},
			MaxDuration: 30,
			Ratings:     []string{"TV-Y", "TV-G"},
			YearFrom:    2000,
		},
	}
}

// seriesBlock returns a fully-populated series-type block used across
// round-trip and validation tests.
func seriesBlock() scheduler.Block {
	return scheduler.Block{
		Type:      scheduler.BlockTypeSeries,
		Name:      "Saturday Night",
		Cron:      "0 20 * * 6",
		Duration:  90,
		ChannelID: "channel-2",
		Priority:  20,
		Series: []scheduler.SeriesConfig{
			{
				ShowTitle:        "Show A",
				EpisodesPerBlock: 2,
				StartSeason:      1,
				StartEpisode:     1,
				OnComplete:       scheduler.CompletionActionContinue,
			},
		},
	}
}

// TestRoundTripFilterBlock verifies that ParseYAML(RenderYAML(x)) preserves
// a filter block's content. Comparison happens at the rendered-YAML level
// rather than via reflect.DeepEqual on the Go struct: yaml.v3 marshals a nil
// slice identically to an empty slice ("[]"), so decoding always yields
// non-nil empty slices for fields that started out nil. That is a property
// of the YAML round-trip (no information is lost), not a discrepancy in
// blockio's behavior, so we assert on the YAML rendering, which is stable.
func TestRoundTripFilterBlock(t *testing.T) {
	original := []scheduler.Block{filterBlock()}

	rendered, err := blockio.RenderYAML(original)
	require.NoError(t, err, "RenderYAML")

	parsed, err := blockio.ParseYAML(rendered)
	require.NoError(t, err, "ParseYAML")
	require.Len(t, parsed, 1)

	reRendered, err := blockio.RenderYAML(parsed)
	require.NoError(t, err, "RenderYAML of parsed result")

	assert.Equal(t, string(rendered), string(reRendered), "round trip should be lossless")

	// Meaningful fields must survive concretely, not just byte-for-byte.
	assert.Equal(t, original[0].Name, parsed[0].Name)
	assert.Equal(t, original[0].Cron, parsed[0].Cron)
	assert.Equal(t, original[0].Duration, parsed[0].Duration)
	assert.Equal(t, original[0].ChannelID, parsed[0].ChannelID)
	assert.Equal(t, original[0].Filter.Genres, parsed[0].Filter.Genres)
}

// TestRoundTripSeriesBlock mirrors TestRoundTripFilterBlock for a
// series-type block.
func TestRoundTripSeriesBlock(t *testing.T) {
	original := []scheduler.Block{seriesBlock()}

	rendered, err := blockio.RenderYAML(original)
	require.NoError(t, err, "RenderYAML")

	parsed, err := blockio.ParseYAML(rendered)
	require.NoError(t, err, "ParseYAML")
	require.Len(t, parsed, 1)

	reRendered, err := blockio.RenderYAML(parsed)
	require.NoError(t, err, "RenderYAML of parsed result")

	assert.Equal(t, string(rendered), string(reRendered), "round trip should be lossless")

	require.Len(t, parsed[0].Series, 1)
	assert.Equal(t, original[0].Name, parsed[0].Name)
	assert.Equal(t, original[0].Series[0].ShowTitle, parsed[0].Series[0].ShowTitle)
	assert.Equal(t, original[0].Series[0].EpisodesPerBlock, parsed[0].Series[0].EpisodesPerBlock)
}

// TestRenderYAML_StableFieldOrder asserts that rendering the same blocks
// twice produces byte-identical output (struct-tag-driven field order, not
// map iteration order).
func TestRenderYAML_StableFieldOrder(t *testing.T) {
	blocks := []scheduler.Block{filterBlock(), seriesBlock()}

	first, err := blockio.RenderYAML(blocks)
	require.NoError(t, err)
	second, err := blockio.RenderYAML(blocks)
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))

	// "blocks:" must be the top-level (and only) key, and "type" must be
	// the first field per Block struct field order.
	assert.True(t, strings.HasPrefix(string(first), "blocks:\n"), "got: %s", first)
	assert.Contains(t, string(first), "\n    - type: filter\n")
}

// TestValidateBlocks_ValidPasses confirms a well-formed block passes CUE
// validation.
func TestValidateBlocks_ValidPasses(t *testing.T) {
	err := blockio.ValidateBlocks([]scheduler.Block{filterBlock()})
	assert.NoError(t, err)
}

// TestValidateBlocks_InvalidDurationFails confirms the CUE "duration > 0"
// constraint is enforced and the resulting error is wrapped (non-nil,
// non-empty message) rather than swallowed.
func TestValidateBlocks_InvalidDurationFails(t *testing.T) {
	b := filterBlock()
	b.Duration = 0

	err := blockio.ValidateBlocks([]scheduler.Block{b})
	require.Error(t, err)
	assert.NotEmpty(t, err.Error())
}

// TestParseYAMLRejectsMissingCron is the brief's canonical example: a block
// missing a required field (cron) must fail CUE validation via ParseYAML.
func TestParseYAMLRejectsMissingCron(t *testing.T) {
	_, err := blockio.ParseYAML([]byte("blocks:\n  - name: x\n    duration: 60\n    channel_id: c\n"))
	if err == nil {
		t.Fatal("expected CUE validation error")
	}
}

// TestParseYAML_StrictDecodeRejectsUnknownTopLevelKey confirms unknown
// top-level keys (outside scheduler.Config's "blocks" field) are rejected
// by strict decoding rather than silently ignored.
func TestParseYAML_StrictDecodeRejectsUnknownTopLevelKey(t *testing.T) {
	data := []byte("blocks: []\nnot_a_real_field: true\n")

	_, err := blockio.ParseYAML(data)
	require.Error(t, err)
}

// TestParseYAML_StrictDecodeRejectsUnknownBlockKey confirms unknown keys
// nested inside a block entry are rejected too.
func TestParseYAML_StrictDecodeRejectsUnknownBlockKey(t *testing.T) {
	data := []byte(`blocks:
  - type: filter
    name: x
    cron: "0 9 * * *"
    duration: 60
    channel_id: c
    typo_field: oops
`)

	_, err := blockio.ParseYAML(data)
	require.Error(t, err)
}

// TestParseYAML_EmptyBlocksIsValid confirms an empty blocks list is not
// itself a validation error.
func TestParseYAML_EmptyBlocksIsValid(t *testing.T) {
	blocks, err := blockio.ParseYAML([]byte("blocks: []\n"))
	require.NoError(t, err)
	assert.Empty(t, blocks)
}

// TestParseYAML_RejectsDuplicateBlockNames confirms two otherwise-valid
// blocks sharing a name are rejected. CUE validates each block against
// #Block independently and has no notion of "unique across the list," so a
// file with two same-named blocks would otherwise sail through CUE
// validation, and then hit store.ErrConflict partway through Bootstrap's
// CreateBlock loop -- after the first of the two had already been written.
// Catching it here, before any store write is attempted, keeps imports
// all-or-nothing without needing store-level transactions.
func TestParseYAML_RejectsDuplicateBlockNames(t *testing.T) {
	data := []byte(`blocks:
  - type: filter
    name: Same Name
    cron: "0 6 * * *"
    duration: 60
    channel_id: channel-1
  - type: filter
    name: Same Name
    cron: "0 7 * * *"
    duration: 60
    channel_id: channel-2
`)

	_, err := blockio.ParseYAML(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Same Name", "error should name the duplicate block")
}
