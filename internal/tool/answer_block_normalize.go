// Package tool — answer_block_normalize.go
//
// G2 (post_v2_runtime_gap_remediation, 2026-05-04). Single source-of-
// truth converter from the JSON-shape emitAnswerBlockV2 to the typed
// types.AnswerBlock. Both full emit
// (executeAnswerDocumentV2 in emit_answer_document_v2.go) and patch
// emit (convertEmitBlocksToTyped in emit_answer_document_patch.go)
// MUST go through this normalizer so a typed annotation field added
// to AnswerBlock is automatically picked up by both paths.
//
// Pre-G2 the two callers maintained PARALLEL copies of the same
// per-block validation + conversion loop. The patch copy
// silently dropped EdgeAnchors (a typed annotation field added by
// the Phase 1-B source-fix); the full-emit copy included it. That is
// EXACTLY the failure mode G2 prevents — a single refactor surface
// keeps the parallel paths in lock-step.
//
// Red lines:
//   - Every exported type-asserted field on AnswerBlock MUST flow
//     through here. The TestNormalizeEmitAnswerBlock_AllFieldsPropagate
//     reflection test pins this so a future field addition fails the
//     test until it is wired.
//   - Error messages name the offending field via fieldPath so the
//     LLM-facing failEmit / err message routes (full + patch) surface
//     identical wording for the same fault.

package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// NormalizeEmitAnswerBlock converts one emitAnswerBlockV2 record into
// the typed types.AnswerBlock, applying every per-block validation
// the V2 carrier requires (kind whitelist, surface_role enum, diagram
// body presence, item conversion). fieldPath is the JSON-pointer-
// style prefix the caller passes for error messages — e.g.
// "blocks[3]" (full emit) or "replace_blocks[1]" (patch emit).
//
// Returns (block, nil) on success; (zero-value, err) on the first
// detected validation failure. The caller is responsible for
// failEmit-style result wrapping (full emit) or fmt.Errorf wrapping
// (patch emit) — this function only carries the typed projection
// + per-record validation.
func NormalizeEmitAnswerBlock(raw emitAnswerBlockV2, fieldPath string) (types.AnswerBlock, error) {
	if strings.TrimSpace(raw.ID) == "" {
		return types.AnswerBlock{}, fmt.Errorf("%s: id is required and must be non-empty", fieldPath)
	}
	kind := types.AnswerBlockKind(raw.Kind)
	if !types.IsValidAnswerBlockKind(kind) {
		return types.AnswerBlock{}, fmt.Errorf("%s: kind=%q is not a valid block kind; allowed values: %v",
			fieldPath, raw.Kind, types.AllAnswerBlockKinds())
	}
	blk := types.AnswerBlock{
		ID:          raw.ID,
		Kind:        kind,
		Title:       raw.Title,
		Text:        raw.Text,
		ClaimUses:   raw.ClaimUses,
		EdgeAnchors: raw.EdgeAnchors,
		FacetIDs:    raw.FacetIDs,
		SurfaceRole: types.SurfaceRole(raw.SurfaceRole),
	}
	if blk.SurfaceRole != "" {
		if _, ok := types.NormalizeSurfaceRole(string(blk.SurfaceRole)); !ok {
			return types.AnswerBlock{}, fmt.Errorf("%s: surface_role=%q is not a valid surface role",
				fieldPath, raw.SurfaceRole)
		}
	}
	if len(raw.Items) > 0 {
		blk.Items = make([]types.AnswerBlockItem, 0, len(raw.Items))
		for _, it := range raw.Items {
			blk.Items = append(blk.Items, types.AnswerBlockItem{
				ID:          it.ID,
				Label:       it.Label,
				Text:        it.Text,
				CitationRef: int(it.CitationRef),
			})
		}
	}
	if raw.Diagram != nil {
		diag := &types.AnswerDiagramBlock{
			Kind:     types.DiagramKind(raw.Diagram.Kind),
			Language: raw.Diagram.Language,
			Body:     raw.Diagram.Body,
		}
		if strings.TrimSpace(diag.Body) == "" {
			return types.AnswerBlock{}, fmt.Errorf("%s: diagram body is required when diagram is present", fieldPath)
		}
		blk.Diagram = diag
	} else if blk.Kind == types.BlockDiagram {
		return types.AnswerBlock{}, fmt.Errorf("%s: kind=diagram requires a non-nil diagram payload", fieldPath)
	}
	return blk, nil
}
