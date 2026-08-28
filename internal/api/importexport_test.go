package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/blockio"
)

// doYAMLRequest sends method/path with an application/yaml body through h
// and returns the recorded response. Unlike doRequest (blocks_test.go),
// which JSON-encodes a Go value, the import endpoint's request body is
// untyped in gen/ (see importexport.go), so callers hand it raw YAML text.
func doYAMLRequest(t *testing.T, h http.Handler, method, path, yamlBody string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(yamlBody))
	req.Header.Set("Content-Type", "application/yaml")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func decodeImportResult(t *testing.T, w *httptest.ResponseRecorder) gen.ImportResult {
	t.Helper()
	var res gen.ImportResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&res), "body: %s", w.Body.String())
	return res
}

const twoValidBlocksYAML = `blocks:
  - type: filter
    name: import-block-one
    cron: "0 6 * * *"
    duration: 60
    channel_id: channel-1
  - type: filter
    name: import-block-two
    cron: "0 7 * * *"
    duration: 45
    channel_id: channel-1
`

func TestImportBlocks_Success(t *testing.T) {
	h := newTestServer(t)

	w := doYAMLRequest(t, h, http.MethodPost, "/blocks/import", twoValidBlocksYAML)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	res := decodeImportResult(t, w)
	assert.Equal(t, 2, res.Imported)
	assert.False(t, res.DryRun)
	require.NotNil(t, res.Names)
	assert.ElementsMatch(t, []string{"import-block-one", "import-block-two"}, *res.Names)

	wl := doRequest(t, h, http.MethodGet, "/blocks", nil)
	require.Equal(t, http.StatusOK, wl.Code)
	var list gen.BlockList
	require.NoError(t, json.NewDecoder(wl.Body).Decode(&list))
	require.Len(t, list, 2, "imported records should exist in the store")
	names := []string{list[0].Name, list[1].Name}
	assert.ElementsMatch(t, []string{"import-block-one", "import-block-two"}, names)
	for _, rec := range list {
		assert.True(t, rec.Enabled, "imported blocks should default to enabled")
	}
}

func TestImportBlocks_DryRunLeavesStoreUnchanged(t *testing.T) {
	h := newTestServer(t)

	w := doYAMLRequest(t, h, http.MethodPost, "/blocks/import?dry_run=true", twoValidBlocksYAML)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	res := decodeImportResult(t, w)
	assert.Equal(t, 2, res.Imported)
	assert.True(t, res.DryRun)
	require.NotNil(t, res.Names)
	assert.ElementsMatch(t, []string{"import-block-one", "import-block-two"}, *res.Names)

	wl := doRequest(t, h, http.MethodGet, "/blocks", nil)
	require.Equal(t, http.StatusOK, wl.Code)
	var list gen.BlockList
	require.NoError(t, json.NewDecoder(wl.Body).Decode(&list))
	assert.Empty(t, list, "dry_run must not write anything to the store")
}

func TestImportBlocks_InvalidYAML_ReturnsCUEDetail(t *testing.T) {
	h := newTestServer(t)

	badYAML := `blocks:
  - type: filter
    name: bad-duration-block
    cron: "0 6 * * *"
    duration: 0
    channel_id: channel-1
`
	w := doYAMLRequest(t, h, http.MethodPost, "/blocks/import", badYAML)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
	assert.Contains(t, strings.ToLower(p.Detail), "duration", "detail should carry the CUE validation failure")
}

func TestImportBlocks_CollisionWithExisting_ImportsNothing(t *testing.T) {
	h := newTestServer(t)

	seed := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("dup-block", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, seed.Code, seed.Body.String())

	collidingYAML := `blocks:
  - type: filter
    name: dup-block
    cron: "0 8 * * *"
    duration: 30
    channel_id: channel-1
  - type: filter
    name: fresh-block
    cron: "0 9 * * *"
    duration: 30
    channel_id: channel-1
`
	w := doYAMLRequest(t, h, http.MethodPost, "/blocks/import", collidingYAML)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusConflict, p.Status)
	assert.Contains(t, p.Detail, "dup-block")

	wl := doRequest(t, h, http.MethodGet, "/blocks", nil)
	require.Equal(t, http.StatusOK, wl.Code)
	var list gen.BlockList
	require.NoError(t, json.NewDecoder(wl.Body).Decode(&list))
	require.Len(t, list, 1, "a colliding import must write nothing, not even the non-colliding block")
	assert.Equal(t, "dup-block", list[0].Name)
}

func TestImportBlocks_SeriesEmptyShowTitle_Returns400(t *testing.T) {
	h := newTestServer(t)

	// Every SeriesConfig field besides show_title is spelled out explicitly:
	// scheduler.SeriesConfig's yaml tags (internal/scheduler/types.go) lack
	// `omitempty`, so blockio.ParseYAML's CUE-validation re-marshal (via
	// RenderYAML) always emits them, even as Go zero values -- and CUE's
	// defaults ("*continue", "*1", "*1") only apply to an *absent* field,
	// not an explicit zero value (mirrors fromGen's doc comment in
	// blocks.go). Leaving them out here would fail CUE validation on
	// on_complete/start_season/start_episode instead of the show_title gap
	// this test targets, same as internal/blockio/bootstrap_test.go's fixture.
	badSeriesYAML := `blocks:
  - type: series
    name: bad-series-import
    cron: "0 20 * * 6"
    duration: 90
    channel_id: channel-2
    series:
      - show_title: ""
        episodes_per_block: 2
        start_season: 1
        start_episode: 1
        on_complete: continue
`
	w := doYAMLRequest(t, h, http.MethodPost, "/blocks/import", badSeriesYAML)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
	assert.Contains(t, strings.ToLower(p.Detail), "show_title")

	wl := doRequest(t, h, http.MethodGet, "/blocks", nil)
	require.Equal(t, http.StatusOK, wl.Code)
	var list gen.BlockList
	require.NoError(t, json.NewDecoder(wl.Body).Decode(&list))
	assert.Empty(t, list, "a rejected import must not write anything")
}

func TestImportBlocks_OversizedBody_Returns413(t *testing.T) {
	h := newTestServer(t)

	oversized := "# " + strings.Repeat("a", maxImportBodyBytes+1024) + "\nblocks: []\n"
	w := doYAMLRequest(t, h, http.MethodPost, "/blocks/import", oversized)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusRequestEntityTooLarge, p.Status)
}

func TestExportBlocks_RoundTripsAndIncludesDisabled(t *testing.T) {
	h := newTestServer(t)

	wEnabled := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("export-enabled", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, wEnabled.Code, wEnabled.Body.String())

	disabledBody := filterBlockWrite("export-disabled", "0 7 * * *")
	disabled := false
	disabledBody.Enabled = &disabled
	wDisabled := doRequest(t, h, http.MethodPost, "/blocks", disabledBody)
	require.Equal(t, http.StatusCreated, wDisabled.Code, wDisabled.Body.String())

	w := doRequest(t, h, http.MethodGet, "/blocks/export", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/yaml", w.Header().Get("Content-Type"))

	blocks, err := blockio.ParseYAML(w.Body.Bytes())
	require.NoError(t, err, "exported YAML must round-trip through blockio.ParseYAML")
	require.Len(t, blocks, 2, "export must include disabled blocks, not just enabled ones")

	names := []string{blocks[0].Name, blocks[1].Name}
	assert.ElementsMatch(t, []string{"export-enabled", "export-disabled"}, names)
}

func TestExportBlocks_Empty(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodGet, "/blocks/export", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/yaml", w.Header().Get("Content-Type"))

	blocks, err := blockio.ParseYAML(w.Body.Bytes())
	require.NoError(t, err)
	assert.Empty(t, blocks)
}
