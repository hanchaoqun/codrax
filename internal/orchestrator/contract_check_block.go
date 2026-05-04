package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// V2 block-only carrier validators (B4 落地 — block_only_carrier.md
// §5.4). 4 validators raise SOFT-by-default ViolationKind values
// when the LLM's emitted AnswerDocumentV2 fails to satisfy the
// AnswerSemanticView contract.
//
// Default classification: SOFT (telemetry only) during B4-B5; B6
// promotes to STRICT once V2 is the default carrier. Operators
// override via pipeline_contract_strict_kinds yaml field.
//
// Per the precise-signals-for-hard-gates red line (R2), all 4
// validators read ONLY:
//   - doc.Blocks[i].Kind / .ID / .FacetIDs / .SurfaceRole / .ClaimUses
//   - view.RequiredBlocks[i].Kind / .MinCount / .MaxCount / .Required /
//     .FacetIDs / .AcceptableClaimForms / .SurfaceRoleHint
//   - view.UncertaintyRules[i].TriggerFacet / .ExpectedBlockKind
//   - view.DiagramPlan.Required / .Kind
//   - view.FacetCoverage.Required[i].Kind
// — i.e. typed enum + verbatim string match only. Zero ranker scores
// or fuzzy heuristics.

// validateRequiredBlockCoverage checks each Required=true entry in
// view.RequiredBlocks against doc.Blocks. Counts blocks that match
// Kind, raises ViolBlockCoverageMissing when:
//   - actual count < req.MinCount, OR
//   - req.MaxCount > 0 AND actual count > req.MaxCount
//
// Failure-mode summary: "LLM emitted V2 doc but skipped a required
// block kind OR over-filled a capped one."
func validateRequiredBlockCoverage(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil {
		return nil
	}
	var out []types.Violation
	counts := make(map[types.AnswerBlockKind]int, len(doc.Blocks))
	for _, b := range doc.Blocks {
		counts[b.Kind]++
	}
	for _, req := range view.RequiredBlocks {
		if !req.Required {
			continue
		}
		got := counts[req.Kind]
		if got < req.MinCount {
			out = append(out, types.Violation{
				Kind: types.ViolBlockCoverageMissing,
				Detail: fmt.Sprintf(
					"required block kind=%s appears %d time(s) in answer; the family contract requires at least %d",
					req.Kind, got, req.MinCount),
				Repair: fmt.Sprintf(
					"emit at least %d block(s) of kind=%s. Per the rationale: %s",
					req.MinCount, req.Kind, req.Rationale),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_block_coverage",
					Reason:     "required block kind under-emitted",
					Confidence: 0.85,
				},
				Stage: string(types.StageFinalize),
			})
			continue
		}
		if req.MaxCount > 0 && got > req.MaxCount {
			out = append(out, types.Violation{
				Kind: types.ViolBlockCoverageMissing,
				Detail: fmt.Sprintf(
					"required block kind=%s appears %d time(s); the family contract caps it at %d",
					req.Kind, got, req.MaxCount),
				Repair: fmt.Sprintf(
					"reduce kind=%s blocks to at most %d. Per the rationale: %s",
					req.Kind, req.MaxCount, req.Rationale),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_block_coverage",
					Reason:     "required block kind over-emitted",
					Confidence: 0.7,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}
	return out
}

// validatePrincipalClaimUse checks that every block whose
// SurfaceRole is "principal" (or whose corresponding BlockRequirement
// hint is SurfacePrincipal) carries at least one RenderedClaimUse —
// at the block level OR on at least one item — when the matching
// BlockRequirement's AcceptableClaimForms is non-empty.
//
// Failure-mode summary: "principal block content has no claim_use
// annotation but its family requires one."
func validatePrincipalClaimUse(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil {
		return nil
	}
	// Build req map by Kind (first match wins; multiple requirements
	// of same Kind take the strictest by ANDing AcceptableClaimForms,
	// which would only happen if a family declared duplicate rows —
	// not currently the case).
	reqByKind := make(map[types.AnswerBlockKind]types.BlockRequirement, len(view.RequiredBlocks))
	for _, r := range view.RequiredBlocks {
		if _, ok := reqByKind[r.Kind]; !ok {
			reqByKind[r.Kind] = r
		}
	}
	var out []types.Violation
	for _, b := range doc.Blocks {
		req, ok := reqByKind[b.Kind]
		if !ok {
			continue
		}
		if len(req.AcceptableClaimForms) == 0 {
			// no claim form check requested
			continue
		}
		isPrincipal := b.SurfaceRole == types.SurfacePrincipal ||
			(b.SurfaceRole == "" && req.SurfaceRoleHint == types.SurfacePrincipal)
		if !isPrincipal {
			continue
		}
		if blockHasClaimUse(b) {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolPrincipalClaimUseMissing,
			Detail: fmt.Sprintf(
				"principal block id=%q kind=%s has no claim_use; family requires one of %v",
				b.ID, b.Kind, formNames(req.AcceptableClaimForms)),
			Repair: fmt.Sprintf(
				"emit claim_use on the block (or on at least one item) declaring claim_form ∈ %v so the validator can match the principal payload to its evidence shape",
				formNames(req.AcceptableClaimForms)),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "block_claim_use",
				Reason:     "principal block lacks claim_use annotation",
				Confidence: 0.7,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// validateDiagramEdgeSupport checks each BlockDiagram in doc.Blocks
// against view.DiagramPlan: when DiagramPlan.Required, the diagram
// must exist and its Kind should match (when both view and block
// declare a Kind). The validator is intentionally lenient on edge
// shape introspection (mermaid edge parsing is complex); it raises
// only on the structural mismatches it can detect deterministically.
//
// Failure-mode summary: "V2 doc emitted a diagram block but its
// declared Kind disagrees with the family's DiagramFacetGraph kind,
// OR the family required a diagram and none was emitted."
func validateDiagramEdgeSupport(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || view.DiagramPlan == nil {
		return nil
	}
	plan := view.DiagramPlan
	var diagramBlock *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockDiagram {
			diagramBlock = &doc.Blocks[i]
			break
		}
	}
	if plan.Required && diagramBlock == nil {
		return []types.Violation{{
			Kind: types.ViolDiagramEdgeUnsupported,
			Detail: fmt.Sprintf(
				"family contract requires a diagram of kind=%s but no BlockDiagram is present in the answer",
				plan.Kind),
			Repair: fmt.Sprintf(
				"emit a BlockDiagram (kind=%s) covering node facets %v and edge facets %v",
				plan.Kind, plan.NodeFacets, plan.EdgeFacets),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_block",
				Reason:     "required diagram absent",
				Confidence: 0.85,
			},
			Stage: string(types.StageFinalize),
		}}
	}
	if diagramBlock == nil || diagramBlock.Diagram == nil {
		return nil
	}
	if plan.Kind != types.DiagramNone &&
		diagramBlock.Diagram.Kind != types.DiagramNone &&
		diagramBlock.Diagram.Kind != plan.Kind {
		return []types.Violation{{
			Kind: types.ViolDiagramEdgeUnsupported,
			Detail: fmt.Sprintf(
				"diagram block id=%q declared kind=%s but family contract expects %s",
				diagramBlock.ID, diagramBlock.Diagram.Kind, plan.Kind),
			Repair: fmt.Sprintf(
				"set diagram.kind=%s OR drop the diagram if the family contract should be relaxed",
				plan.Kind),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_kind",
				Reason:     "diagram kind mismatch",
				Confidence: 0.7,
			},
			Stage: string(types.StageFinalize),
		}}
	}
	return nil
}

// validateUncertaintyBlockPresence walks view.UncertaintyRules; for
// each rule whose TriggerFacet appears in view.FacetCoverage's
// Required list, the answer must include at least one block whose
// Kind == rule.ExpectedBlockKind. Empty TriggerFacet rules apply
// unconditionally (their MissingMessage tells the LLM why).
//
// Failure-mode summary: "the family contract demands a caveat /
// disclosure block (e.g. log-source drift), but the answer omitted
// it."
func validateUncertaintyBlockPresence(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || len(view.UncertaintyRules) == 0 {
		return nil
	}
	hasKind := make(map[types.AnswerBlockKind]bool, len(doc.Blocks))
	for _, b := range doc.Blocks {
		hasKind[b.Kind] = true
	}
	requiredFacets := make(map[string]bool)
	if view.FacetCoverage != nil {
		for _, r := range view.FacetCoverage.Required {
			requiredFacets[string(r.Kind)] = true
		}
	}
	var out []types.Violation
	for _, rule := range view.UncertaintyRules {
		// Empty TriggerFacet means "always-required disclosure" —
		// e.g. shape=value families' bounded-scope caveat. Otherwise
		// require the trigger facet to be in the family's required
		// facet set.
		if rule.TriggerFacet != "" && !requiredFacets[rule.TriggerFacet] {
			continue
		}
		if hasKind[rule.ExpectedBlockKind] {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolUncertaintyBlockMissing,
			Detail: fmt.Sprintf(
				"uncertainty rule (trigger=%q) requires a block of kind=%s but none is present",
				rule.TriggerFacet, rule.ExpectedBlockKind),
			Repair: rule.MissingMessage,
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "uncertainty_block",
				Reason:     "required disclosure block absent",
				Confidence: 0.75,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// blockHasClaimUse reports whether a block carries any claim_use
// annotation — either at block level (b.ClaimUses) or on any item
// (b.Items[i].ClaimUse non-nil) or on a diagram (b.Diagram.ClaimUses).
func blockHasClaimUse(b types.AnswerBlock) bool {
	if len(b.ClaimUses) > 0 {
		return true
	}
	for _, it := range b.Items {
		if it.ClaimUse != nil {
			return true
		}
	}
	if b.Diagram != nil && len(b.Diagram.ClaimUses) > 0 {
		return true
	}
	return false
}

// formNames stringifies a ClaimForm slice for error messages.
func formNames(forms []types.ClaimForm) []string {
	out := make([]string, 0, len(forms))
	for _, f := range forms {
		out = append(out, string(f))
	}
	return out
}

// validateFacetCoverage (R2.3 V2 重接, post_shape_residual_audit.md
// 2026-05-04) checks that every Required FacetCoverageContract entry
// (Tier=Hard/Soft) is covered by at least one V2 block whose
// block.FacetIDs[] names the facet's Kind.
//
// Pre-B8-T4 the V1 sibling (runFacetCoverageOracle) walked V1 doc
// payloads and matched the facet's SourceCandidate evidence IDs
// against per-payload citation refs — V2 mirrors that with the
// typed FacetIDs slice the V2 emit_answer_document validator
// already populates on every block. Coverage now = "any block
// declared this FacetID" — a precise, typed signal (R2 red line).
//
// Skip rules:
//   - view == nil OR view.FacetCoverage == nil: family doesn't carry
//     facet obligations.
//   - facet.Tier == TierEnrichment (Optional): handled by
//     validateRichnessRegression below, not here.
//
// Default classification SOFT (covering a facet often requires
// fresh evidence; missed HARD facet = re-explore hint, not finalizer
// self-failure). Operators promote to STRICT via
// pipeline_contract_strict_kinds.
func validateFacetCoverage(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || view.FacetCoverage == nil {
		return nil
	}
	covered := make(map[string]bool, 8)
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			covered[strings.TrimSpace(fid)] = true
		}
		// Items / ClaimUses MAY also carry FacetID via
		// RenderedClaimUse.FacetID — fold those in too.
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
		for _, item := range b.Items {
			if item.ClaimUse != nil && item.ClaimUse.FacetID != "" {
				covered[strings.TrimSpace(item.ClaimUse.FacetID)] = true
			}
		}
	}
	var out []types.Violation
	for _, req := range view.FacetCoverage.Required {
		if req.Tier == types.TierEnrichment {
			continue
		}
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" {
			continue
		}
		if covered[kind] {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolFacetUncovered,
			Detail: fmt.Sprintf(
				"required facet %q (tier=%s) is not covered: no V2 block declared it via block.facet_ids[] or via item.claim_use.facet_id",
				kind, req.Tier),
			Repair: fmt.Sprintf(
				"declare facet_id=%q on at least one block whose payload covers this facet, OR re-investigate to gather evidence whose ClaimForm matches the facet's AcceptableForms (when no current evidence supports the facet).",
				kind),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_facet_coverage",
				Reason:     "FacetCoverageContract.Required entry uncovered by V2 blocks",
				Confidence: 0.7,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// validateRichnessRegression (R2.3 V2 重接, post_shape_residual_audit
// 2026-05-04) records ViolRichnessRegression for each Optional facet
// (Tier=Enrichment) whose SourceCandidate is non-empty but no block
// declared its FacetID. This is the pure-telemetry tier — Phase 5
// design says the kind is SOFT-by-default and explicitly NOT
// promotable to STRICT (richness regression is observation, not a
// correctness gate).
//
// Reads:
//   - view.FacetCoverage.Optional[i].SourceCandidate (non-empty =
//     evidence is available, the answer COULD have surfaced it)
//   - block.FacetIDs / item.ClaimUse.FacetID (coverage signal,
//     same as facet_uncovered above)
func validateRichnessRegression(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || view.FacetCoverage == nil {
		return nil
	}
	if len(view.FacetCoverage.Optional) == 0 {
		return nil
	}
	covered := make(map[string]bool, 8)
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			covered[strings.TrimSpace(fid)] = true
		}
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
		for _, item := range b.Items {
			if item.ClaimUse != nil && item.ClaimUse.FacetID != "" {
				covered[strings.TrimSpace(item.ClaimUse.FacetID)] = true
			}
		}
	}
	var out []types.Violation
	for _, req := range view.FacetCoverage.Optional {
		if len(req.SourceCandidate) == 0 {
			continue // no evidence available — not a regression
		}
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" {
			continue
		}
		if covered[kind] {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolRichnessRegression,
			Detail: fmt.Sprintf(
				"optional richness facet %q has %d evidence candidate(s) but no V2 block surfaced it (telemetry only — answer ships unchanged)",
				kind, len(req.SourceCandidate)),
			Repair: fmt.Sprintf(
				"if the question would benefit from this facet, declare facet_id=%q on a block; otherwise leave as-is (richness regression is informational).",
				kind),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_richness_facet_coverage",
				Reason:     "optional facet with available evidence not surfaced",
				Confidence: 0.5,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// runV2BlockOracles is the single orchestrator-side dispatch entry
// for B4. Returns the union of all 4 V2 validator violations. Caller
// (runContractCheck) appends to the result Violations slice the
// same way Block 2/3 oracles do.
//
// Returns nil when doc or view is nil.
//
// R2.3 (post_shape_residual_audit.md, 2026-05-04): two new V2
// oracles join the dispatch — validateFacetCoverage (HARD/SOFT
// facet uncovered) + validateRichnessRegression (Optional facet
// telemetry). They restore producers for ViolFacetUncovered +
// ViolRichnessRegression, both of which had their V1 dispatch
// deleted at B8-T4.
func runV2BlockOracles(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil {
		return nil
	}
	var out []types.Violation
	out = append(out, validateRequiredBlockCoverage(doc, view)...)
	out = append(out, validatePrincipalClaimUse(doc, view)...)
	out = append(out, validateDiagramEdgeSupport(doc, view)...)
	out = append(out, validateUncertaintyBlockPresence(doc, view)...)
	out = append(out, validateFacetCoverage(doc, view)...)
	out = append(out, validateRichnessRegression(doc, view)...)
	return out
}

// _ keeps strings import used (formNames + Detail strings).
var _ = strings.TrimSpace
