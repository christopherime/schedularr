package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/blockio"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// maxImportBodyBytes caps the size of a POST /blocks/import request body.
// The endpoint exists to load a hand-edited or backed-up scheduler.yaml's
// worth of blocks -- a few KB to perhaps a few hundred KB for a very large
// deployment -- so 1MiB is generous headroom while still bounding memory use
// against an oversized or malicious upload; ImportBlocks enforces it via
// http.MaxBytesReader and returns 413 (not 400) when exceeded, since the
// problem is the request's size, not its content.
const maxImportBodyBytes = 1 << 20 // 1MiB

// ImportBlocks implements gen.ServerInterface. The request body is
// untyped in gen/ (oapi-codegen does not generate a Go type for an
// application/yaml requestBody), so the raw bytes are read directly off
// r.Body rather than JSON-decoded.
//
// Binding order:
//  1. Read the body (capped at maxImportBodyBytes).
//  2. blockio.ParseYAML: strict YAML decode + duplicate-in-file name
//     rejection + CUE schema validation. Any failure here -> 400, with
//     ParseYAML's wrapped error (which includes CUE's own detail) as the
//     problem detail.
//  3. validateSeriesShowTitles (blocks.go) per parsed block: closes the
//     same CUE gap blocks CRUD closes -- see that function's doc comment.
//     Any failure -> 400.
//  4. Pre-check every parsed block's name against the store's existing
//     block names. Any collision -> 409 listing the colliding name(s),
//     with zero writes -- the friendly-diagnostic path, since it can name
//     every colliding block at once.
//  5. dry_run=true (default false) stops here and reports the would-be
//     result without writing anything.
//  6. Otherwise, create every parsed block (uuid.NewString ID, Enabled
//     true -- imported blocks start active, matching Bootstrap's behavior
//     in blockio/bootstrap.go) via store.CreateBlocks, a single
//     transaction: all blocks land or none do. Given step 4's pre-check, a
//     failure here is unexpected -- a name collision from a concurrent
//     request racing this one maps to 409 (with nothing imported, thanks
//     to the transaction's rollback); any other store/driver fault is
//     logged and reported as a generic 500, matching every other
//     unexpected-store-error path in this package
//     (logAndWriteInternalError).
func (h *Handlers) ImportBlocks(w http.ResponseWriter, r *http.Request, params gen.ImportBlocksParams) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBodyBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			WriteProblem(w, r, http.StatusRequestEntityTooLarge, "import body too large", err.Error())
			return
		}
		WriteProblem(w, r, http.StatusBadRequest, "failed to read request body", err.Error())
		return
	}

	blocks, err := blockio.ParseYAML(data)
	if err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "invalid block YAML", err.Error())
		return
	}

	if err := validateImportedShowTitles(blocks); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "block validation failed", err.Error())
		return
	}

	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, b.Name)
	}

	existing, err := h.d.Store.ListBlocks(r.Context())
	if err != nil {
		h.logAndWriteInternalError(w, r, "import_blocks_list_existing", err)
		return
	}
	existingNames := make(map[string]bool, len(existing))
	for _, rec := range existing {
		existingNames[rec.Name] = true
	}
	var collisions []string
	for _, name := range names {
		if existingNames[name] {
			collisions = append(collisions, name)
		}
	}
	if len(collisions) > 0 {
		WriteProblem(w, r, http.StatusConflict, "block name already exists",
			"colliding with existing block name(s): "+strings.Join(collisions, ", "))
		return
	}

	// Imported blocks all start enabled, so they must agree with the
	// store's enabled blocks on shared-show on_complete policies
	// (ParseYAML already checked the batch internally). Runs before the
	// dry-run early-out on purpose: a dry run exists to preview exactly
	// this kind of rejection.
	if !h.checkSharedShowPolicies(w, r, blocks, "") {
		return
	}

	dryRun := params.DryRun != nil && *params.DryRun

	if !dryRun && !h.createImportedBlocks(w, r, blocks) {
		return
	}

	writeJSON(w, http.StatusOK, gen.ImportResult{
		Imported: len(blocks),
		DryRun:   dryRun,
		Names:    &names,
	})
}

// createImportedBlocks persists every parsed block in one CreateBlocks
// transaction (step 6 of ImportBlocks' binding order -- all blocks land or
// none do) and reports whether it succeeded, writing the error response
// itself when it didn't. ErrConflict here means a concurrent create raced
// past step 4's pre-existing-name check; the transaction rolled the whole
// batch back, so nothing was imported.
func (h *Handlers) createImportedBlocks(w http.ResponseWriter, r *http.Request, blocks []scheduler.Block) bool {
	recs := make([]*store.BlockRecord, 0, len(blocks))
	for _, b := range blocks {
		recs = append(recs, &store.BlockRecord{
			ID:      uuid.NewString(),
			Name:    b.Name,
			Enabled: true,
			Spec:    b,
		})
	}
	if err := h.d.Store.CreateBlocks(r.Context(), recs); err != nil {
		if errors.Is(err, store.ErrConflict) {
			WriteProblem(w, r, http.StatusConflict, "block name already exists",
				"a block name collided during import; nothing was imported")
			return false
		}
		h.logAndWriteInternalError(w, r, "import_blocks_create", err)
		return false
	}
	return true
}

// validateImportedShowTitles runs validateSeriesShowTitles (blocks.go) over
// every block ParseYAML returned, aggregating every offending block's name
// into one 400 detail rather than stopping at the first. It exists
// alongside validateSeriesShowTitles rather than folding the loop into it
// because ImportBlocks handles a batch and blocks CRUD handles one block at
// a time; both ultimately share the same per-block rule.
func validateImportedShowTitles(blocks []scheduler.Block) error {
	var bad []string
	for _, b := range blocks {
		if err := validateSeriesShowTitles(b); err != nil {
			bad = append(bad, b.Name)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("failed to validate blocks: empty show_title in series block(s): %s", strings.Join(bad, ", "))
	}
	return nil
}

// ExportBlocks implements gen.ServerInterface. It lists every stored block
// -- enabled and disabled alike, since export doubles as a backup mechanism
// and silently dropping disabled blocks would make a restored import lossy
// -- and renders their specs as YAML via blockio.RenderYAML, the same
// renderer ParseYAML's round-trip depends on. The response body is
// untyped in gen/ for the same reason the import request body is: no Go
// type is generated for an application/yaml response.
func (h *Handlers) ExportBlocks(w http.ResponseWriter, r *http.Request) {
	recs, err := h.d.Store.ListBlocks(r.Context())
	if err != nil {
		h.logAndWriteInternalError(w, r, "export_blocks", err)
		return
	}

	specs := make([]scheduler.Block, 0, len(recs))
	for _, rec := range recs {
		specs = append(specs, rec.Spec)
	}

	data, err := blockio.RenderYAML(specs)
	if err != nil {
		h.logAndWriteInternalError(w, r, "export_blocks_render", err)
		return
	}

	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
