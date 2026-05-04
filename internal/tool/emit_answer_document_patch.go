package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_answer_document_patch — protocol-level retry preservation
// tool. On retry paths the LLM emits a *delta* (this tool) rather
// than the full document (emit_answer_document). The system applies
// the patch to the previous emit (read from
// MutableState.AnswerDocumentV2 OR the typed retry-state snapshot)
// and writes the resulting full doc to Mutable.
//
// Why this tool exists despite emit_answer_document already
// supporting full re-emit: prior real-eval traces show generative
// LLMs ignore "preserve byte-identical" prompt directives roughly
// 50% of the time on retry paths, dropping typed annotation fields
// they already emitted correctly. Protocol-level patch makes the
// preservation **structurally guaranteed** — LLM never has the
// chance to drop a field on a block it didn't touch, because the
// system clones unchanged blocks from prev verbatim.
//
// Tool calling discipline (taught via skill prompt):
//   - First dispatch / no prev emit: use emit_answer_document.
//   - Retry path with prev emit on Mutable: PREFER
//     emit_answer_document_patch when only a few blocks need
//     editing. Fall back to full emit_answer_document for big
//     rewrites.

// EmitAnswerDocumentPatch is the patch tool. Mirrors the shape of
// EmitAnswerDocument (NonEvidenceTool — the payload is the final
// answer slate, not factual claims about the repo) so the agent
// layer registers them uniformly.
type EmitAnswerDocumentPatch struct {
	ReadOnly
	NonEvidenceTool
}

func (t *EmitAnswerDocumentPatch) Name() string { return "emit_answer_document_patch" }

func (t *EmitAnswerDocumentPatch) Description() string {
	return "Emit a DELTA against your previous `emit_answer_document` call instead of re-emitting the whole document. Use ONLY on retry paths (when `## Hard Rule (retry attempt N)` appears in the system prompt and a `## Previous Emit` section is present). On first dispatches, use `emit_answer_document` instead.\n\n" +
		"Patch fields (all optional, but at least one MUST be non-empty):\n\n" +
		"- `unchanged_block_ids`: ids of blocks from the previous emit to copy over byte-identical. Use this to assert preservation of every typed annotation field (claim_use, facet_ids, surface_role, items[].claim_use) on blocks you do NOT need to edit.\n" +
		"- `replace_blocks`: full block payloads that replace the previous emit's block with the same id. Each entry must carry a non-empty id that exists in the previous emit.\n" +
		"- `add_blocks`: new block payloads to append. Each id must NOT already exist in the previous emit.\n" +
		"- `remove_block_ids`: ids of previous-emit blocks to drop.\n" +
		"- `replace_citations`: when present, REPLACES the citation pool entirely. Otherwise the previous citations are inherited.\n" +
		"- `append_citations`: when present and `replace_citations` is absent, appended to the inherited pool.\n" +
		"- `replace_exact_resolution` / `replace_caveats` / `replace_snippets`: when present, replace the corresponding document-level field.\n\n" +
		"Validation: every id named in `unchanged_block_ids` / `replace_blocks` / `remove_block_ids` MUST exist in the previous emit; every `add_blocks` id MUST NOT. Cross-op conflicts (Replace + Remove same id, etc.) are rejected. Block kind is validated against the canonical AnswerBlockKind list. The merged document is written to Mutable as if you had called `emit_answer_document` with the full payload.\n\n" +
		"Empty patches are rejected — every retry MUST declare some change (set `unchanged_block_ids` to assert preservation if no edits are needed)."
}

func (t *EmitAnswerDocumentPatch) Parameters() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "unchanged_block_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Block ids from the previous emit to copy over verbatim. Every id must exist in the previous emit. Use this to assert preservation of typed annotation fields (claim_use / facet_ids / surface_role / items[].claim_use) on blocks you are not editing — the system clones the prev block byte-identical, so the LLM cannot accidentally drop a field."
    },
    "replace_blocks": {
      "type": "array",
      "description": "Block payloads that replace previous-emit blocks with the same id. Each entry has the full AnswerBlock shape (id, kind, title, text, items, diagram, claim_use, claim_uses, facet_ids, surface_role).",
      "items": {"type": "object"}
    },
    "add_blocks": {
      "type": "array",
      "description": "New block payloads to append after the existing blocks. Each entry has the full AnswerBlock shape; id MUST NOT already exist in the previous emit (use replace_blocks for editing).",
      "items": {"type": "object"}
    },
    "remove_block_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Block ids to drop from the previous emit."
    },
    "replace_citations": {
      "type": "array",
      "description": "OPTIONAL. When present, REPLACES the citation pool entirely. Use this when re-picking citations holistically.",
      "items": {"type": "object"}
    },
    "append_citations": {
      "type": "array",
      "description": "OPTIONAL. When present (and replace_citations is absent), appended to the inherited citation pool. Useful for adding a single new cite without rewriting the pool.",
      "items": {"type": "object"}
    },
    "replace_exact_resolution": {"type": "object", "description": "OPTIONAL. When present, replaces previous exact_resolution. Otherwise inherited from previous emit."},
    "replace_caveats":  {"type": "array", "items": {"type": "string"}, "description": "OPTIONAL. When present, replaces previous caveats."},
    "replace_snippets": {"type": "array", "items": {"type": "object"}, "description": "OPTIONAL. When present, replaces previous snippets."}
  }
}`
	return json.RawMessage(schema)
}

// emitAnswerDocumentPatchParams mirrors AnswerDocumentV2Patch
// one-to-one for JSON unmarshalling. CitationRef and AnswerBlockItem
// fields use the same FlexInt typed approach as the V2 emit so
// citation_ref values can be int OR string from the LLM.
type emitAnswerDocumentPatchParams struct {
	UnchangedBlockIDs      []string                  `json:"unchanged_block_ids,omitempty"`
	ReplaceBlocks          []emitAnswerBlockV2       `json:"replace_blocks,omitempty"`
	AddBlocks              []emitAnswerBlockV2       `json:"add_blocks,omitempty"`
	RemoveBlockIDs         []string                  `json:"remove_block_ids,omitempty"`
	ReplaceCitations       []types.Citation          `json:"replace_citations,omitempty"`
	AppendCitations        []types.Citation          `json:"append_citations,omitempty"`
	ReplaceExactResolution *types.AnswerExactResolution `json:"replace_exact_resolution,omitempty"`
	ReplaceCaveats         []string                  `json:"replace_caveats,omitempty"`
	ReplaceSnippets        []types.CodeSnippet       `json:"replace_snippets,omitempty"`
}

// Execute applies the patch to the previous V2 emit. Failure paths
// surface as Success=false ToolResult so the LLM sees the error
// and can retry with corrected params (the patch validator's
// reject messages name the offending id / op verbatim).
func (t *EmitAnswerDocumentPatch) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return failEmit(t.Name(), now,
			"emit_answer_document_patch requires BusContext.Mutable")
	}

	// Locate the previous emit. Prefer Mutable.AnswerDocumentV2()
	// (the live state — most recent successful emit). Fall back to
	// RetryState.PrevEmitJSON (snapshot taken at retry-decision
	// time) when AnswerDocumentV2 has been cleared by ResetForFallback.
	prev := ctx.Mutable.AnswerDocumentV2()
	if prev == nil {
		prev = recoverPrevFromRetryState(ctx.Mutable)
	}
	if prev == nil {
		return failEmit(t.Name(), now,
			"emit_answer_document_patch: no previous emit found. The patch tool is only valid on retry paths after a successful emit_answer_document call. First dispatches must use emit_answer_document.")
	}

	// Decode params.
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitAnswerDocumentPatchParams
	if err := dec.Decode(&p); err != nil {
		return failEmit(t.Name(), now, "invalid params: %v", err)
	}

	// Build typed AnswerDocumentV2Patch from the decoded params.
	patch := &types.AnswerDocumentV2Patch{
		UnchangedBlockIDs:      append([]string(nil), p.UnchangedBlockIDs...),
		RemoveBlockIDs:         append([]string(nil), p.RemoveBlockIDs...),
		ReplaceCitations:       p.ReplaceCitations,
		AppendCitations:        p.AppendCitations,
		ReplaceExactResolution: p.ReplaceExactResolution,
		ReplaceCaveats:         p.ReplaceCaveats,
		ReplaceSnippets:        p.ReplaceSnippets,
	}
	if len(p.ReplaceBlocks) > 0 {
		converted, err := convertEmitBlocksToTyped(t.Name(), p.ReplaceBlocks, "replace_blocks")
		if err != nil {
			return failEmit(t.Name(), now, "%s", err.Error())
		}
		patch.ReplaceBlocks = converted
	}
	if len(p.AddBlocks) > 0 {
		converted, err := convertEmitBlocksToTyped(t.Name(), p.AddBlocks, "add_blocks")
		if err != nil {
			return failEmit(t.Name(), now, "%s", err.Error())
		}
		patch.AddBlocks = converted
	}

	// Apply.
	merged, err := types.ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		return failEmit(t.Name(), now, "patch apply rejected: %v", err)
	}

	// Re-validate every block id is unique in the merged doc (the
	// V2 emit_answer_document Execute also enforces this — patch
	// must produce a doc that would have passed full emit gate).
	seenIDs := make(map[string]bool, len(merged.Blocks))
	for i, b := range merged.Blocks {
		if strings.TrimSpace(b.ID) == "" {
			return failEmit(t.Name(), now,
				"merged blocks[%d]: id is empty after applying patch", i)
		}
		if seenIDs[b.ID] {
			return failEmit(t.Name(), now,
				"merged blocks[%d]: duplicate id %q after applying patch (each block must have a unique id)", i, b.ID)
		}
		seenIDs[b.ID] = true
		if b.Kind == types.BlockDiagram && b.Diagram == nil {
			return failEmit(t.Name(), now,
				"merged blocks[%d]: kind=diagram requires a non-nil diagram payload", i)
		}
	}

	// Write the merged doc.
	ctx.Mutable.SetAnswerDocumentV2(merged)

	logging.Info("[emit_answer_document_patch] applied: %d unchanged + %d replaced + %d added + %d removed → %d blocks total",
		len(patch.UnchangedBlockIDs), len(patch.ReplaceBlocks),
		len(patch.AddBlocks), len(patch.RemoveBlockIDs), len(merged.Blocks))

	return types.ToolResult{
		ToolName: t.Name(),
		Success:  true,
		Summary: fmt.Sprintf(
			"emit_answer_document_patch applied: %d unchanged, %d replaced, %d added, %d removed → %d blocks",
			len(patch.UnchangedBlockIDs), len(patch.ReplaceBlocks),
			len(patch.AddBlocks), len(patch.RemoveBlockIDs), len(merged.Blocks)),
		Timestamp: now,
	}, nil
}

// recoverPrevFromRetryState attempts to decode the prev emit JSON
// stashed by R14 RetryState. Returns nil when not available or
// decode fails.
func recoverPrevFromRetryState(mut *types.MutableState) *types.AnswerDocumentV2 {
	rs := mut.RetryState()
	if rs == nil || len(rs.PrevEmitJSON) == 0 {
		return nil
	}
	var doc types.AnswerDocumentV2
	if err := json.Unmarshal(rs.PrevEmitJSON, &doc); err != nil {
		logging.Warning("[emit_answer_document_patch] RetryState.PrevEmitJSON decode failed: %v", err)
		return nil
	}
	return &doc
}

// convertEmitBlocksToTyped converts the JSON emitAnswerBlockV2
// shape (FlexInt etc.) into the typed AnswerBlock used by the
// patch. Reuses the same per-block validation surface that V2
// emit goes through, so every dimension the V2 carrier rejects
// (invalid kind, duplicate id within emit, etc.) is also rejected
// at patch time.
//
// Returns ([]AnswerBlock, nil) on success; ("", error) names the
// offending block (with field path) so the LLM can fix the patch.
func convertEmitBlocksToTyped(toolName string, in []emitAnswerBlockV2, fieldName string) ([]types.AnswerBlock, error) {
	out := make([]types.AnswerBlock, 0, len(in))
	for i, raw := range in {
		if strings.TrimSpace(raw.ID) == "" {
			return nil, fmt.Errorf("%s: %s[%d]: id is required and must be non-empty", toolName, fieldName, i)
		}
		kind := types.AnswerBlockKind(raw.Kind)
		if !types.IsValidAnswerBlockKind(kind) {
			return nil, fmt.Errorf("%s: %s[%d]: kind=%q is not a valid AnswerBlockKind; allowed values: %v",
				toolName, fieldName, i, raw.Kind, types.AllAnswerBlockKinds())
		}
		blk := types.AnswerBlock{
			ID:          raw.ID,
			Kind:        kind,
			Title:       raw.Title,
			Text:        raw.Text,
			ClaimUses:   raw.ClaimUses,
			FacetIDs:    raw.FacetIDs,
			SurfaceRole: types.SurfaceRole(raw.SurfaceRole),
		}
		if blk.SurfaceRole != "" {
			if _, ok := types.NormalizeSurfaceRole(string(blk.SurfaceRole)); !ok {
				return nil, fmt.Errorf("%s: %s[%d]: surface_role=%q is not a valid SurfaceRole",
					toolName, fieldName, i, raw.SurfaceRole)
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
					ClaimUse:    it.ClaimUse,
				})
			}
		}
		if raw.Diagram != nil {
			diag := &types.AnswerDiagramBlock{
				Kind:      types.DiagramKind(raw.Diagram.Kind),
				Language:  raw.Diagram.Language,
				Body:      raw.Diagram.Body,
				ClaimUses: raw.Diagram.ClaimUses,
			}
			if strings.TrimSpace(diag.Body) == "" {
				return nil, fmt.Errorf("%s: %s[%d]: diagram body is required when diagram is present", toolName, fieldName, i)
			}
			blk.Diagram = diag
		} else if blk.Kind == types.BlockDiagram {
			return nil, fmt.Errorf("%s: %s[%d]: kind=diagram requires a non-nil diagram payload", toolName, fieldName, i)
		}
		out = append(out, blk)
	}
	return out, nil
}
