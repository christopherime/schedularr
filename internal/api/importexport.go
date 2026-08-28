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
//     with zero writes: this is the transactional-batch alternative Task 9
//     parked (see writeBlockStoreError's doc comment) -- since blocks CRUD
//     writes one row per call with no batch/transaction primitive in
//     internal/store, the only way to guarantee "nothing partially
//     imported" without adding one is to make the collision impossible
//     before the first write.
//  5. dry_run=true (default false) stops here and reports the would-be
//     result without writing anything.
//  6. Otherwise, create every parsed block (uuid.NewString ID, Enabled
//     true -- imported blocks start active, matching Bootstrap's behavior
//     in blockio/bootstrap.go). Given step 4's pre-check, a create failure
//     here is unexpected (e.g. a genuine store/driver fault, or a name
//     collision from a concurrent request racing this one); either way it
//     is logged and reported as a generic 500, matching every other
//     unexpected-store-error path in this package
//     (logAndWriteInternalError). A batch transaction that could roll back
//     a partial write on such a race stays out of scope, per the same
//     parked ruling step 4 references.
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

	dryRun := params.DryRun != nil && *params.DryRun

	if !dryRun {
		for _, b := range blocks {
			rec := &store.BlockRecord{
				ID:      uuid.NewString(),
				Name:    b.Name,
				Enabled: true,
				Spec:    b,
			}
			if err := h.d.Store.CreateBlock(r.Context(), rec); err != nil {
				h.logAndWriteInternalError(w, r, "import_blocks_create", err)
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, gen.ImportResult{
		Imported: len(blocks),
		DryRun:   dryRun,
		Names:    &names,
	})
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
