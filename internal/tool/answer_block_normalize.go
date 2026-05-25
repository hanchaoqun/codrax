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
