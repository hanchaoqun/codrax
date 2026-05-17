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
		"- `unchanged_block_ids`: ids of blocks from the previous emit to copy over byte-identical. Use this to assert preservation of every typed annotation field (claim_uses, edge_anchors, facet_ids, surface_role, items[].candidate_role) on blocks you do NOT need to edit.\n" +
		"- `replace_blocks`: full block payloads that replace the previous emit's block with the same id. Each entry must carry a non-empty id that exists in the previous emit. Block payload shape matches the canonical block contract — see below.\n" +
		"- `add_blocks`: new block payloads to append. Each id must NOT already exist in the previous emit. Block payload shape matches the canonical block contract — see below.\n" +
		"- `remove_block_ids`: ids of previous-emit blocks to drop.\n" +
		"- `replace_citations`: when present, REPLACES the citation pool entirely. Otherwise the previous citations are inherited. Prefer `append_citations` for additive citation repairs. If you accidentally replace the pool while preserving previous citation-bearing blocks, the tool will keep the previous pool, append genuinely new citations, and remap citation_ref values inside your replace/add blocks.\n" +
		"- `append_citations`: when present and `replace_citations` is absent, appended to the inherited pool.\n" +
		"- `replace_exact_resolution` / `replace_missing_requested_roles` / `replace_caveats` / `replace_snippets`: when present, replace the corresponding document-level field.\n\n" +
		"Validation: every id named in `unchanged_block_ids` / `replace_blocks` / `remove_block_ids` MUST exist in the previous emit; every `add_blocks` id MUST NOT. Cross-op conflicts (Replace + Remove same id, etc.) are rejected. Block kind is validated against the canonical block-kind enum. The merged document is stored as if you had called `emit_answer_document` with the full payload.\n\n" +
		"Empty patches are rejected — every retry MUST declare some change (set `unchanged_block_ids` to assert preservation if no edits are needed).\n\n" +
		"BLOCK CONTRACT (same shape replace_blocks / add_blocks payloads must follow as a full emit):\n\n" +
		BuildAnswerDocumentSemanticContractDescription()
}

func (t *EmitAnswerDocumentPatch) Parameters() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "unchanged_block_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Block ids from the previous emit to copy over verbatim. Every id must exist in the previous emit. Use this to assert preservation of typed annotation fields (claim_uses / edge_anchors / facet_ids / surface_role / items[].candidate_role) on blocks you are not editing — the system clones the prev block byte-identical, so the LLM cannot accidentally drop a field."
    },
    "replace_blocks": {
      "type": "array",
      "description": "Block payloads that replace previous-emit blocks with the same id. Each entry has the full block shape (id, kind, title, text, items, diagram, claim_uses, edge_anchors, facet_ids, surface_role).",
      "items": {"type": "object"}
    },
    "add_blocks": {
      "type": "array",
      "description": "New block payloads to append after the existing blocks. Each entry has the full block shape (id, kind, title, text, items, diagram, claim_uses, edge_anchors, facet_ids, surface_role); id MUST NOT already exist in the previous emit (use replace_blocks for editing).",
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
    "replace_missing_requested_roles": {
      "type": "array",
      "description": "OPTIONAL. When present, replaces previous missing_requested_roles[]. Use this when a retry needs to add / remove / correct typed missing requested precedence layers for an exact-absence config-precedence answer.",
      "items": {
        "type": "object",
        "properties": {
          "role": {"type": "string", "enum": ["default", "config", "runtime", "override"]},
          "label": {"type": "string"}
        },
        "required": ["role"]
      }
    },
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
	UnchangedBlockIDs            []string                           `json:"unchanged_block_ids,omitempty"`
	ReplaceBlocks                []emitAnswerBlockV2                `json:"replace_blocks,omitempty"`
	AddBlocks                    []emitAnswerBlockV2                `json:"add_blocks,omitempty"`
	RemoveBlockIDs               []string                           `json:"remove_block_ids,omitempty"`
	ReplaceCitations             []emitAnswerCitationV2             `json:"replace_citations,omitempty"`
	AppendCitations              []emitAnswerCitationV2             `json:"append_citations,omitempty"`
	ReplaceExactResolution       *types.AnswerExactResolution       `json:"replace_exact_resolution,omitempty"`
	ReplaceMissingRequestedRoles []types.AnswerMissingRequestedRole `json:"replace_missing_requested_roles,omitempty"`
	ReplaceCaveats               []string                           `json:"replace_caveats,omitempty"`
	ReplaceSnippets              []emitCodeSnippetV2                `json:"replace_snippets,omitempty"`
}

// Execute applies the patch to the previous V2 emit. Failure paths
// surface as Success=false ToolResult so the LLM sees the error
// and can retry with corrected params (the patch validator's
// reject messages name the offending id / op verbatim).
func (t *EmitAnswerDocumentPatch) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return failEmit(t.Name(), now,
			"emit_answer_document_patch requires a writable context")
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
		prev = recoverPrevFromRejectedDraft(ctx.Mutable)
		if prev != nil {
			logging.Warning("[emit_answer_document_patch] using previous rejected answer draft as patch base; merged document will be fully revalidated")
		}
	}
	if prev == nil {
		return failEmit(t.Name(), now,
			"emit_answer_document_patch: no previous emit found. The patch tool is only valid on retry paths after a successful emit_answer_document call. First dispatches must use emit_answer_document.")
	}

	// Flat-mode tolerance for the streaming-bug pattern where an LLM
	// stringifies an array field instead of emitting a real JSON
	// array. Mirrors the protection emit_answer_document already has
	// — applied here so add_blocks / replace_blocks / replace_citations
	// / append_citations / replace_missing_requested_roles /
	// replace_caveats / replace_snippets / unchanged_block_ids /
	// remove_block_ids all get the same Path A/C recovery without
	// forcing an LLM retry round-trip.
	if repaired, fields, ok := repairStringWrappedArrayFields(params); ok {
		logging.Warning("[emit_answer_document_patch] string-wrapped array field(s) re-parsed via flat-mode tolerance: %s",
			strings.Join(fields, ", "))
		params = repaired
	}
	if repaired, fields, ok := repairStringWrappedObjectFields(params, "replace_exact_resolution"); ok {
		logging.Warning("[emit_answer_document_patch] string-wrapped object field(s) re-parsed via flat-mode tolerance: %s",
			strings.Join(fields, ", "))
		params = repaired
	}
	if repaired, paths, ok := repairNestedArraysInPatch(params); ok {
		logging.Warning("[emit_answer_document_patch] nested block fields normalized via local-model JSON tolerance: %s",
			strings.Join(paths, ", "))
		params = repaired
	}

	// Decode params.
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitAnswerDocumentPatchParams
	if err := dec.Decode(&p); err != nil {
		// Reuse the V2 emit's misplaced-field hint table (same
		// schema for blocks); R4 sanitization always applies.
		err = RemapStrictDecodeError(err, answerDocumentV2MisplacedHints)
		return failEmit(t.Name(), now, "invalid params: %v", err)
	}

	// Build typed AnswerDocumentV2Patch from the decoded params.
	patch := &types.AnswerDocumentV2Patch{
		UnchangedBlockIDs:            append([]string(nil), p.UnchangedBlockIDs...),
		RemoveBlockIDs:               append([]string(nil), p.RemoveBlockIDs...),
		ReplaceCitations:             convertEmitCitationsToTyped(p.ReplaceCitations),
		AppendCitations:              convertEmitCitationsToTyped(p.AppendCitations),
		ReplaceExactResolution:       p.ReplaceExactResolution,
		ReplaceMissingRequestedRoles: p.ReplaceMissingRequestedRoles,
		ReplaceCaveats:               p.ReplaceCaveats,
		ReplaceSnippets:              convertEmitCodeSnippetsToTyped(p.ReplaceSnippets),
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
	if changed, fields := normalizeAnswerDocumentPatchBlockOps(prev, patch); changed {
		logging.Warning("[emit_answer_document_patch] block op(s) normalized via prev-id tolerance: %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := normalizeAnswerDocumentPatchCitationOps(prev, patch); changed {
		logging.Warning("[emit_answer_document_patch] citation op(s) normalized via preserved-pool tolerance: %s",
			strings.Join(fields, ", "))
	}

	// v3 B4 (2026-05-04): route the patch-emit write through the
	// unified mutation runtime — same chokepoint as the full path.
	// Partial Apply runs ApplyAnswerDocumentV2Patch internally;
	// merged-doc invariants (id uniqueness / diagram payload /
	// max blocks) live in ApplyAndPersistMutation.
	mutation := types.NewPartialMutation(patch)

	// P1 (2026-05-10) — emit-time pre-validation chokepoint, mirror
	// of the full-emit path. Run on the merged doc shape that the
	// patch produces: dry-run Apply once, run pre-emit checks, then
	// hand off to ApplyAndPersistMutation which re-runs Apply
	// internally. Apply is pure (no side effects on the doc clone)
	// so the dry-run is safe.
	if view := types.BuildAnswerSemanticViewForBusContext(ctx); view != nil {
		if merged, applyErr := mutation.Apply(prev); applyErr == nil && merged != nil {
			canonicalizeSummaryLeadBlock(merged)
			normalizeAggregateMemberSetCarriers(merged, ctx)
			normalizePrincipalSupportMemberCarriers(merged, types.BuildAnswerSupportPlanForBusContext(ctx))
			normalizeViewCompatibleAnswerDocument(merged, view)
			if hints := runPreEmitChecks(merged, view, preEmitOracleFromCtx(ctx), ctx); len(hints) > 0 {
				return failEmit(t.Name(), now, "%s", formatEmitFixHints(hints))
			}
		}
	}

	return ApplyAndPersistMutation(ctx, t.Name(), mutation, prev, now)
}

func normalizeAnswerDocumentPatchBlockOps(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if prev == nil || patch == nil {
		return false, nil
	}
	prevIDs := make(map[string]bool, len(prev.Blocks))
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id != "" {
			prevIDs[id] = true
		}
	}
	if len(prevIDs) == 0 {
		return false, nil
	}
	removeIDs := make(map[string]bool, len(patch.RemoveBlockIDs))
	for _, id := range patch.RemoveBlockIDs {
		removeIDs[strings.TrimSpace(id)] = true
	}
	addIDs := make(map[string]bool, len(patch.AddBlocks))
	for _, block := range patch.AddBlocks {
		addIDs[strings.TrimSpace(block.ID)] = true
	}
	replaceIDs := make(map[string]bool, len(patch.ReplaceBlocks))
	for _, block := range patch.ReplaceBlocks {
		replaceIDs[strings.TrimSpace(block.ID)] = true
	}

	var fields []string
	var keptReplace []types.AnswerBlock
	for _, block := range patch.ReplaceBlocks {
		id := strings.TrimSpace(block.ID)
		if id != "" && !prevIDs[id] && !addIDs[id] && !removeIDs[id] {
			patch.AddBlocks = append(patch.AddBlocks, block)
			addIDs[id] = true
			fields = append(fields, fmt.Sprintf("replace_blocks[%q]→add_blocks", id))
			continue
		}
		keptReplace = append(keptReplace, block)
	}
	patch.ReplaceBlocks = keptReplace

	var keptAdd []types.AnswerBlock
	for _, block := range patch.AddBlocks {
		id := strings.TrimSpace(block.ID)
		if id != "" && prevIDs[id] && !replaceIDs[id] && !removeIDs[id] {
			patch.ReplaceBlocks = append(patch.ReplaceBlocks, block)
			replaceIDs[id] = true
			fields = append(fields, fmt.Sprintf("add_blocks[%q]→replace_blocks", id))
			continue
		}
		keptAdd = append(keptAdd, block)
	}
	patch.AddBlocks = keptAdd

	return len(fields) > 0, fields
}

func normalizeAnswerDocumentPatchCitationOps(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if prev == nil || patch == nil || patch.ReplaceCitations == nil || len(patch.AppendCitations) > 0 {
		return false, nil
	}
	if !patchPreservesCitationBearingBlock(prev, patch) {
		return false, nil
	}
	mergedPool := append([]types.Citation(nil), prev.Citations...)
	remap := make(map[int]int, len(patch.ReplaceCitations))
	var appendCitations []types.Citation
	for i, cit := range patch.ReplaceCitations {
		idx := findEquivalentCitation(mergedPool, cit)
		if idx < 0 {
			mergedPool = append(mergedPool, cit)
			appendCitations = append(appendCitations, cit)
			idx = len(mergedPool) - 1
		}
		remap[i] = idx
	}
	remapped := remapPatchBlockCitationRefs(patch.ReplaceBlocks, remap) +
		remapPatchBlockCitationRefs(patch.AddBlocks, remap)
	patch.ReplaceCitations = nil
	patch.AppendCitations = appendCitations
	fields := []string{"replace_citations→append_citations"}
	if remapped > 0 {
		fields = append(fields, fmt.Sprintf("items[].citation_ref remapped=%d", remapped))
	}
	return true, fields
}

func patchPreservesCitationBearingBlock(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) bool {
	removed := make(map[string]bool, len(patch.RemoveBlockIDs))
	for _, id := range patch.RemoveBlockIDs {
		removed[strings.TrimSpace(id)] = true
	}
	replaced := make(map[string]bool, len(patch.ReplaceBlocks))
	for _, block := range patch.ReplaceBlocks {
		replaced[strings.TrimSpace(block.ID)] = true
	}
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if removed[id] || replaced[id] {
			continue
		}
		if answerBlockHasCitationRefsForPatchTool(block) {
			return true
		}
	}
	return false
}

func answerBlockHasCitationRefsForPatchTool(block types.AnswerBlock) bool {
	for _, item := range block.Items {
		if item.CitationRef >= 0 {
			return true
		}
	}
	return false
}

func findEquivalentCitation(pool []types.Citation, cit types.Citation) int {
	for i, existing := range pool {
		if equivalentAnswerCitation(existing, cit) {
			return i
		}
	}
	return -1
}

func equivalentAnswerCitation(a, b types.Citation) bool {
	return normalizePatchCitationFile(a.File) == normalizePatchCitationFile(b.File) &&
		a.Line == b.Line &&
		a.LineEnd == b.LineEnd &&
		strings.TrimSpace(a.Quote) == strings.TrimSpace(b.Quote) &&
		a.Scope == b.Scope &&
		strings.TrimSpace(a.SectionPath) == strings.TrimSpace(b.SectionPath) &&
		a.FileRoleLabel == b.FileRoleLabel &&
		strings.TrimSpace(a.CrossfileSummary) == strings.TrimSpace(b.CrossfileSummary) &&
		strings.TrimSpace(a.NegativePattern) == strings.TrimSpace(b.NegativePattern) &&
		strings.TrimSpace(a.EnclosingFunction) == strings.TrimSpace(b.EnclosingFunction)
}

func normalizePatchCitationFile(file string) string {
	return strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
}

func remapPatchBlockCitationRefs(blocks []types.AnswerBlock, remap map[int]int) int {
	changed := 0
	for bi := range blocks {
		for ii := range blocks[bi].Items {
			ref := blocks[bi].Items[ii].CitationRef
			if ref < 0 {
				continue
			}
			mapped, ok := remap[ref]
			if !ok || mapped == ref {
				continue
			}
			blocks[bi].Items[ii].CitationRef = mapped
			changed++
		}
	}
	return changed
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
	if len(doc.Blocks) == 0 {
		return nil
	}
	return &doc
}

func recoverPrevFromRejectedDraft(mut *types.MutableState) *types.AnswerDocumentV2 {
	if mut == nil {
		return nil
	}
	doc := mut.LastRejectedAnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 {
		return nil
	}
	return doc
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
// convertEmitBlocksToTyped routes through the unified
// NormalizeEmitAnswerBlock so the patch path picks up every typed
// annotation field automatically (G2 post_v2_runtime_gap_remediation,
// 2026-05-04 — pre-G2 this loop silently dropped EdgeAnchors).
func convertEmitBlocksToTyped(toolName string, in []emitAnswerBlockV2, fieldName string) ([]types.AnswerBlock, error) {
	out := make([]types.AnswerBlock, 0, len(in))
	for i, raw := range in {
		blk, err := NormalizeEmitAnswerBlock(raw, fmt.Sprintf("%s: %s[%d]", toolName, fieldName, i))
		if err != nil {
			return nil, err
		}
		out = append(out, blk)
	}
	return out, nil
}
