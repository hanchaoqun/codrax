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

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
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
	if caveat := strings.TrimSpace(raw.Caveat); caveat != "" {
		if kind != types.BlockCaveat {
			return types.AnswerBlock{}, fmt.Errorf("%s: caveat is only accepted as a local-model compatibility alias on kind=caveat blocks; use block.text for the visible prose",
				fieldPath)
		}
		if strings.TrimSpace(raw.Text) == "" {
			raw.Text = raw.Caveat
		}
	}
	blk := types.AnswerBlock{
		ID:          raw.ID,
		Kind:        kind,
		Title:       raw.Title,
		Text:        raw.Text,
		Columns:     normalizeTableStringSlice(raw.Columns),
		ClaimUses:   raw.ClaimUses,
		EdgeAnchors: raw.EdgeAnchors,
		FacetIDs:    raw.FacetIDs,
		SurfaceRole: types.SurfaceRole(raw.SurfaceRole),
	}
	if raw.ErrorGranularityVerdict != "" {
		verdict, ok := types.NormalizeErrorGranularityVerdict(raw.ErrorGranularityVerdict)
		if !ok || verdict == types.ErrorGranularityUnknown {
			return types.AnswerBlock{}, fmt.Errorf("%s: error_granularity_verdict=%q is not a valid error granularity verdict",
				fieldPath, raw.ErrorGranularityVerdict)
		}
		if kind != types.BlockDecision {
			return types.AnswerBlock{}, fmt.Errorf("%s: error_granularity_verdict is only valid on kind=decision blocks",
				fieldPath)
		}
		blk.ErrorGranularityVerdict = verdict
	}
	if raw.CurrentStatusVerdict != "" {
		verdict, ok := types.NormalizeCurrentStatusVerdict(raw.CurrentStatusVerdict)
		if !ok || verdict == types.CurrentStatusUnknown {
			return types.AnswerBlock{}, fmt.Errorf("%s: current_status_verdict=%q is not a valid current status verdict",
				fieldPath, raw.CurrentStatusVerdict)
		}
		if kind != types.BlockDecision {
			return types.AnswerBlock{}, fmt.Errorf("%s: current_status_verdict is only valid on kind=decision blocks",
				fieldPath)
		}
		blk.CurrentStatusVerdict = verdict
	}
	if raw.ScopeDisclosure != "" {
		disclosure, ok := types.NormalizeScopeDisclosureKind(raw.ScopeDisclosure)
		if !ok || disclosure == types.ScopeDisclosureUnknown {
			return types.AnswerBlock{}, fmt.Errorf("%s: scope_disclosure=%q is not a valid scope disclosure kind",
				fieldPath, raw.ScopeDisclosure)
		}
		blk.ScopeDisclosure = disclosure
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
			candidateRole, ok := types.NormalizeAnswerCandidateRole(it.CandidateRole)
			if !ok {
				candidateRole = types.AnswerCandidateRoleOther
			}
			blk.Items = append(blk.Items, types.AnswerBlockItem{
				ID:            it.ID,
				Label:         it.Label,
				Text:          it.Text,
				Cells:         normalizeTableStringSlice(it.Cells),
				CandidateRole: candidateRole,
				CitationRef:   int(it.CitationRef),
			})
		}
	}
	if raw.Diagram != nil {
		if blk.Kind != types.BlockDiagram {
			// A non-empty typed diagram sibling is a precise schema signal:
			// the model already chose the diagram carrier and only left the
			// discriminator stale. Correct the discriminator locally instead
			// of spending a finalizer retry on a lossless shape repair. Do
			// not infer diagrams from prose/text here; only an explicit
			// raw.Diagram payload is eligible.
			blk.Kind = types.BlockDiagram
		}
		normalizeEmitAnswerDiagram(raw.Diagram)
		diag := &types.AnswerDiagramBlock{
			Kind:     types.DiagramKind(raw.Diagram.Kind),
			Language: raw.Diagram.Language,
			Body:     raw.Diagram.Body,
		}
		if strings.TrimSpace(diag.Body) == "" {
			return types.AnswerBlock{}, fmt.Errorf("%s: diagram.body is empty — set diagram.body to the raw Mermaid source (the part inside the ```mermaid fences; the renderer adds the fences itself). diagram.body is the only place the diagram source lives — do not put it in the block-level text field", fieldPath)
		}
		blk.Diagram = diag
	} else if blk.Kind == types.BlockDiagram {
		return types.AnswerBlock{}, fmt.Errorf("%s: kind=diagram requires the sibling `diagram` object {kind: <flow|sequence|architecture|call_dag>, language: \"mermaid\", body: <raw mermaid source>}. If the diagram body is currently in the block-level `text` field, move it into `diagram.body` and set diagram.kind to the SEMANTIC family the contract names (NOT the Mermaid keyword)", fieldPath)
	}
	return blk, nil
}

func normalizeTableStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.TrimSpace(s)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeEmitAnswerDiagram(diag *emitAnswerDiagramV2) {
	if diag == nil {
		return
	}
	diag.Body = stripOuterDiagramFence(diag.Body)
	diag.Body = mermaidcompat.NormalizeSourceForMarkdown(diag.Body)
	family := types.MermaidBodySyntaxFamily(diag.Body)
	if family == types.MermaidSyntaxUnknown || family == types.MermaidSyntaxUnsupported {
		return
	}
	if strings.TrimSpace(diag.Language) == "" {
		diag.Language = "mermaid"
	}
	if !strings.EqualFold(strings.TrimSpace(diag.Language), "mermaid") {
		return
	}
	kind := types.DiagramKind(strings.TrimSpace(diag.Kind))
	if types.DiagramKindAllowsMermaidSyntax(kind, family) {
		return
	}
	switch family {
	case types.MermaidSyntaxSequence:
		diag.Kind = string(types.DiagramSequence)
	case types.MermaidSyntaxFlow:
		diag.Kind = string(types.DiagramFlow)
	}
}

func stripOuterDiagramFence(body string) string {
	out := body
	for i := 0; i < 4; i++ {
		trimmed := strings.TrimSpace(out)
		if !strings.HasPrefix(trimmed, "```") {
			return out
		}
		lines := strings.Split(trimmed, "\n")
		if len(lines) < 3 {
			return out
		}
		if strings.TrimSpace(lines[len(lines)-1]) != "```" {
			return out
		}
		info := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "```")))
		switch info {
		case "", "mermaid", "text":
		default:
			return out
		}
		next := strings.Join(lines[1:len(lines)-1], "\n")
		if strings.TrimSpace(next) == strings.TrimSpace(out) {
			return out
		}
		out = next
	}
	return out
}

// splitFusedDiagramBlocks expands every FUSED block — a valid
// non-diagram kind that carries BOTH visible rows (items / columns)
// AND a renderable diagram payload — into two sibling blocks before
// NormalizeEmitAnswerBlock's discriminator repair runs. Without the
// split, the repair overwrites the declared kind to diagram and the
// renderer (which dispatches on Kind alone) silently drops the rows —
// the model's table content evaporates even though it was emitted
// correctly (2026-06-12 read_combo_pipeline_sequence_table forensics).
//
// Partition: the visible half keeps the declared kind plus
// id/title/text/columns/items/claim_uses/facet_ids/surface_role and
// every verdict field; the diagram half carries ONLY the diagram
// payload + edge_anchors under a derived unique id, inserted
// immediately after the visible half so narrative order survives.
//
// Trigger is a precise typed signal (no prose inspection):
// diagram payload present AND non-empty body AND declared kind is a
// valid non-diagram kind AND rows present. Anything else passes
// through untouched — in particular an EMPTY diagram body keeps the
// existing single-block hard-reject path so the model gets one
// retryable error instead of a half-accepted document.
//
// Doctrine note: this mirrors the no-rewrite rule from
// answer_document_table_compile.go — the system never rewrites
// model-authored surface content; it only re-homes the two payloads
// the model fused so BOTH stay visible.
func splitFusedDiagramBlocks(logLabel string, blocks []emitAnswerBlockV2) []emitAnswerBlockV2 {
	fused := 0
	for _, b := range blocks {
		if isFusedDiagramBlock(b) {
			fused++
		}
	}
	if fused == 0 {
		return blocks
	}
	used := make(map[string]bool, len(blocks)+fused)
	for _, b := range blocks {
		used[strings.TrimSpace(b.ID)] = true
	}
	out := make([]emitAnswerBlockV2, 0, len(blocks)+fused)
	split := 0
	for _, b := range blocks {
		if !isFusedDiagramBlock(b) || len(out)+2 > maxBlocksPerDoc {
			out = append(out, b)
			continue
		}
		visible := b
		visible.Diagram = nil
		visible.EdgeAnchors = nil
		diagramHalf := emitAnswerBlockV2{
			ID:          deriveSplitDiagramBlockID(b.ID, used),
			Kind:        string(types.BlockDiagram),
			Diagram:     b.Diagram,
			EdgeAnchors: b.EdgeAnchors,
		}
		out = append(out, visible, diagramHalf)
		split++
	}
	if split > 0 {
		logging.Warning("[%s] split %d fused diagram block(s): declared kind and visible rows preserved alongside the diagram payload", logLabel, split)
	}
	return out
}

// isFusedDiagramBlock reports whether raw fuses a visible-row block
// with a diagram payload. All four conjuncts are typed-field checks.
func isFusedDiagramBlock(raw emitAnswerBlockV2) bool {
	if raw.Diagram == nil || strings.TrimSpace(raw.Diagram.Body) == "" {
		return false
	}
	kind := types.AnswerBlockKind(strings.TrimSpace(raw.Kind))
	if kind == types.BlockDiagram || !types.IsValidAnswerBlockKind(kind) {
		return false
	}
	return len(raw.Items) > 0 || len(raw.Columns) > 0
}

// deriveSplitDiagramBlockID returns a block id unique within the
// emit for the split-out diagram half. The suffixed form keeps the
// pairing visible in logs and downstream telemetry.
func deriveSplitDiagramBlockID(baseID string, used map[string]bool) string {
	base := strings.TrimSpace(baseID)
	if base == "" {
		base = "block"
	}
	candidate := base + "_diagram"
	for i := 2; used[candidate] && i < 100; i++ {
		candidate = fmt.Sprintf("%s_diagram%d", base, i)
	}
	used[candidate] = true
	return candidate
}

// splitFusedDiagramPatchBlocks applies the fused-block split to the
// patch emit's two raw lists. replace_blocks merges strictly one
// block per replaced id, so the diagram half of a fused REPLACE
// entry cannot stay in replace_blocks — it is appended to add_blocks
// instead (the merged doc places adds at the tail; losing adjacency
// beats losing the payload). Fused ADD entries split in place.
func splitFusedDiagramPatchBlocks(logLabel string, replaceBlocks, addBlocks []emitAnswerBlockV2) ([]emitAnswerBlockV2, []emitAnswerBlockV2) {
	fused := 0
	for _, b := range replaceBlocks {
		if isFusedDiagramBlock(b) {
			fused++
		}
	}
	for _, b := range addBlocks {
		if isFusedDiagramBlock(b) {
			fused++
		}
	}
	if fused == 0 {
		return replaceBlocks, addBlocks
	}
	used := make(map[string]bool, len(replaceBlocks)+len(addBlocks)+fused)
	for _, b := range replaceBlocks {
		used[strings.TrimSpace(b.ID)] = true
	}
	for _, b := range addBlocks {
		used[strings.TrimSpace(b.ID)] = true
	}
	outReplace := make([]emitAnswerBlockV2, 0, len(replaceBlocks))
	outAdd := make([]emitAnswerBlockV2, 0, len(addBlocks)+fused)
	split := 0
	for _, b := range replaceBlocks {
		if !isFusedDiagramBlock(b) {
			outReplace = append(outReplace, b)
			continue
		}
		visible := b
		visible.Diagram = nil
		visible.EdgeAnchors = nil
		outReplace = append(outReplace, visible)
		outAdd = append(outAdd, emitAnswerBlockV2{
			ID:          deriveSplitDiagramBlockID(b.ID, used),
			Kind:        string(types.BlockDiagram),
			Diagram:     b.Diagram,
			EdgeAnchors: b.EdgeAnchors,
		})
		split++
	}
	for _, b := range addBlocks {
		if !isFusedDiagramBlock(b) {
			outAdd = append(outAdd, b)
			continue
		}
		visible := b
		visible.Diagram = nil
		visible.EdgeAnchors = nil
		outAdd = append(outAdd, visible, emitAnswerBlockV2{
			ID:          deriveSplitDiagramBlockID(b.ID, used),
			Kind:        string(types.BlockDiagram),
			Diagram:     b.Diagram,
			EdgeAnchors: b.EdgeAnchors,
		})
		split++
	}
	if split > 0 {
		logging.Warning("[%s] split %d fused diagram block(s) across patch ops: declared kind and visible rows preserved alongside the diagram payload", logLabel, split)
	}
	return outReplace, outAdd
}
