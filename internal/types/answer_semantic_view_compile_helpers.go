package types

import "strings"

// Shared helpers used by every compile_<family>.go file. Per the
// docs/migration plan §5.2 B2 design, each family produces its own
// BlockRequirement set; these helpers exist so common shapes are
// defined once. Adding a new family does not touch this file —
// only its own compile_<family>.go.

// requireSummaryBlock returns the canonical "every answer carries one
// opening summary block" requirement. Reused across all 7 families.
func requireSummaryBlock(rationale string) BlockRequirement {
	return BlockRequirement{
		Kind:            BlockSummary,
		MinCount:        1,
		MaxCount:        1,
		Required:        true,
		Rationale:       rationale,
		SurfaceRoleHint: SurfacePrincipal,
	}
}

// applyExactAbsenceSummaryLead rewrites scalar-first families into a
// summary-led contract when the compiled surface plan has already
// concluded the exact target is absent.
//
// Why this exists:
//   - exact-absence questions often need the principal answer to lead
//     with the absence itself ("no such key / symbol / binding"), not a
//     synthetic scalar literal like "(missing)"
//   - the evaluator prompt already instructs the finalizer to avoid a
//     fabricated scalar in these cases
//   - without a typed compile-side rewrite, the runtime still requires
//     BlockScalar, so the finalizer gets contradictory instructions
//
// The rewrite is intentionally narrow and typed:
//   - only applies when the compiled preferred exact-resolution status
//     is absent
//   - only affects families that currently require a principal scalar
//     carrying FacetResolvedLiteralOrSymbol
//   - promotes the principal Summary block into the carrier of the
//     resolved-literal facet via ClaimAbsenceFact, while making the
//     scalar block optional support rather than required payload
func applyExactAbsenceSummaryLead(view *AnswerSemanticView, plan *AnswerSurfacePlan) {
	if view == nil || plan == nil || plan.PreferredExactResolution == nil {
		return
	}
	if plan.PreferredExactResolution.Status != AnswerExactResolutionAbsent {
		return
	}
	summaryIdx, scalarIdx := -1, -1
	for i := range view.RequiredBlocks {
		req := view.RequiredBlocks[i]
		if !req.Required || req.SurfaceRoleHint != SurfacePrincipal {
			continue
		}
		switch req.Kind {
		case BlockSummary:
			summaryIdx = i
		case BlockScalar:
			if containsString(req.FacetIDs, string(FacetResolvedLiteralOrSymbol)) {
				scalarIdx = i
			}
		}
	}
	if summaryIdx < 0 || scalarIdx < 0 {
		return
	}

	summary := &view.RequiredBlocks[summaryIdx]
	summary.FacetIDs = appendUniqueStr(summary.FacetIDs, string(FacetResolvedLiteralOrSymbol))
	summary.AcceptableClaimForms = appendUniqueClaimForms(summary.AcceptableClaimForms, ClaimAbsenceFact)
	switch view.Family {
	case QFConfigPrecedence:
		summary.Rationale = strings.TrimSpace(summary.Rationale +
			" When the exact config target is absent, this summary block becomes the principal carrier of the absence conclusion. " +
			"Lead with the absent key finding, then explain any grounded precedence / lineage context without fabricating a missing scalar literal.")
	case QFRoleLookup:
		summary.Rationale = strings.TrimSpace(summary.Rationale +
			" When the exact role target is absent, this summary block becomes the principal carrier of the absence conclusion. " +
			"Lead with the absent lookup result instead of fabricating a placeholder scalar literal.")
	default:
		summary.Rationale = strings.TrimSpace(summary.Rationale +
			" When the exact target is absent, this summary block becomes the principal carrier of the absence conclusion. " +
			"Do not fabricate a placeholder scalar literal.")
	}

	scalar := &view.RequiredBlocks[scalarIdx]
	scalar.Required = false
	scalar.MinCount = 0
	scalar.SurfaceRoleHint = SurfaceSupport
	scalar.Rationale = strings.TrimSpace(scalar.Rationale +
		" Optional only when a grounded literal truly exists; when the exact target is absent, do not emit a synthetic `(missing)` / `(不存在)` scalar.")
}

// optionalCaveatBlock returns a caveat-style block requirement that
// is NOT required but counts toward richness. Used for drift /
// scope / external-source disclosures.
func optionalCaveatBlock(rationale string, facetIDs ...string) BlockRequirement {
	return BlockRequirement{
		Kind:            BlockCaveat,
		MinCount:        0,
		MaxCount:        0, // 0 = no upper limit (R5: no layer assumption)
		Required:        false,
		Rationale:       rationale,
		FacetIDs:        appendUniqueStr(nil, facetIDs...),
		SurfaceRoleHint: SurfaceProseOnly,
	}
}

// diagramPlanFor builds a DiagramFacetGraph when the family carries
// a diagram contract. Returns nil when the underlying surface plan
// did not request a diagram, so the V2 validator can short-circuit
// the diagram check.
//
// Maps the ClaimForm pool through to AcceptableClaimForms only when
// downstream B4 validators need to fence which forms are legal for
// node vs edge claim_uses.
func diagramPlanFor(plan *AnswerSurfacePlan, kind DiagramKind, nodeFacets []string, edgeFacets []string, edgeRelations []DiagramEdgeRelationContract) *DiagramFacetGraph {
	if plan == nil || plan.Diagram == nil {
		return nil
	}
	contract := plan.Diagram
	if !contract.Required && len(contract.PreferredKinds) == 0 {
		return nil
	}
	resolvedKind := kind
	if resolvedKind == DiagramNone && len(contract.PreferredKinds) > 0 {
		resolvedKind = contract.PreferredKinds[0]
	}
	return &DiagramFacetGraph{
		Required:      contract.Required,
		Kind:          resolvedKind,
		NodeFacets:    nodeFacets,
		EdgeFacets:    edgeFacets,
		EdgeRelations: append([]DiagramEdgeRelationContract(nil), edgeRelations...),
	}
}

// DefaultEdgeRelationsForKind returns the canonical typed relation
// contract for the SST diagram-kind mapping. Families that do not
// need a specialised contract should pass the result of this helper
// to diagramPlanFor; families with extra typed expectations should
// extend the returned slice rather than build from scratch.
//
// Mapping (Phase 3-C4 §6.2.4):
//   - DiagramFlow         → guard (Min:1, ClaimGuardCondition)
//   - DiagramSequence     → call  (Min:1, ClaimCallEdge)
//   - DiagramCallDAG      → call  (Min:1, ClaimCallEdge)
//   - DiagramArchitecture → contain (Min:0, ClaimUnknown — block-level)
//   - DiagramNone         → nil
//
// The function returns a fresh slice so callers may safely append.
func DefaultEdgeRelationsForKind(kind DiagramKind) []DiagramEdgeRelationContract {
	switch kind {
	case DiagramFlow:
		return []DiagramEdgeRelationContract{
			{Kind: DiagramRelGuard, Min: 1, ClaimForm: ClaimGuardCondition},
		}
	case DiagramSequence:
		return []DiagramEdgeRelationContract{
			{Kind: DiagramRelCall, Min: 1, ClaimForm: ClaimCallEdge},
		}
	case DiagramCallDAG:
		return []DiagramEdgeRelationContract{
			{Kind: DiagramRelCall, Min: 1, ClaimForm: ClaimCallEdge},
		}
	case DiagramArchitecture:
		return []DiagramEdgeRelationContract{
			{Kind: DiagramRelContain, Min: 0, ClaimForm: ClaimUnknown},
		}
	}
	return nil
}

// uncertaintyRuleForObservedArtifact returns the canonical "log /
// perf trace observed external state — disclose drift in a caveat
// block" rule. Triggered when FacetObservedArtifactFact is in the
// FacetCoverage required list. Used by QFRootCauseTrace +
// QFCallChain (any family whose answer integrates external trace
// observations).
func uncertaintyRuleForObservedArtifact() UncertaintyRule {
	return UncertaintyRule{
		TriggerFacet:      string(FacetObservedArtifactFact),
		ExpectedBlockKind: BlockCaveat,
		MissingMessage: "Your answer integrates evidence from an attached log or perf trace; " +
			"emit at least one caveat block disclosing which observations come from the external trace " +
			"versus the current code so the reader knows which lines are from the runtime that produced " +
			"the trace versus the source you can read today.",
	}
}

// richnessCandidatesFromOptionalFacets pulls FacetCoverageContract.
// Optional[] entries through to RichnessCandidate slots. Each
// optional facet gets a candidate slot of the most natural block
// kind for that facet (heuristic table, not keyword based).
func richnessCandidatesFromOptionalFacets(fc *FacetCoverageContract) []RichnessCandidate {
	if fc == nil || len(fc.Optional) == 0 {
		return nil
	}
	out := make([]RichnessCandidate, 0, len(fc.Optional))
	for _, opt := range fc.Optional {
		out = append(out, RichnessCandidate{
			Kind:     blockKindForFacet(opt.Kind),
			FacetID:  string(opt.Kind),
			Optional: true,
		})
	}
	return out
}

// blockKindForFacet returns the natural BlockKind a facet would
// surface as if emitted. Pure typed-enum mapping — NOT a keyword
// table (R3): we read the AnswerFacetKind enum value and return a
// canonical block kind.
//
// Adding a new facet kind requires updating this switch; the
// TestBlockKindForFacetCovers structural test (added in B2-T4)
// enforces it.
func blockKindForFacet(facet AnswerFacetKind) AnswerBlockKind {
	switch facet {
	case FacetObservedArtifactFact:
		return BlockCaveat
	case FacetCurrentCodePath:
		return BlockOrderedList
	case FacetNearestMechanism:
		return BlockSection
	case FacetUncertaintyBoundary:
		return BlockCaveat
	case FacetConfigPrecedenceRole:
		return BlockTable
	case FacetResolvedLiteralOrSymbol:
		return BlockScalar
	case FacetEnumerationItem:
		return BlockOrderedList
	case FacetBucketLabel:
		return BlockSection
	case FacetPrincipalPathEdge:
		return BlockOrderedList
	case FacetBranchGuard:
		return BlockCaveat
	case FacetComponentRelation:
		return BlockSection
	case FacetDiagramSpine:
		return BlockDiagram
	}
	// Unknown facet → safe fallback: a generic Section block is
	// always a valid surface, no V1 leak.
	return BlockSection
}

// markGlaringFacets sets Strength=EnrichmentGlaring on every
// FacetCoverage.Optional entry whose Kind matches one of the given
// kinds. Idempotent — calling twice with the same arguments is safe.
// No-op when view.FacetCoverage is nil.
//
// v3 B2 (2026-05-04). Each compile_<family>.go calls this helper
// after setting view.FacetCoverage to mark which optional facets the
// family considers "glaring richness" — uncovered + threshold-met
// evidence triggers ViolRichnessGlaringGap.
func markGlaringFacets(view *AnswerSemanticView, kinds ...AnswerFacetKind) {
	if view == nil || view.FacetCoverage == nil {
		return
	}
	wanted := make(map[AnswerFacetKind]bool, len(kinds))
	for _, k := range kinds {
		wanted[k] = true
	}
	for i := range view.FacetCoverage.Optional {
		if wanted[view.FacetCoverage.Optional[i].Kind] {
			view.FacetCoverage.Optional[i].Strength = EnrichmentGlaring
		}
	}
}

// familyGlaringEvidenceThreshold returns the per-family floor on
// SourceCandidate count above which an optional facet marked
// EnrichmentGlaring promotes to ViolRichnessGlaringGap (Medium /
// retry-eligible). v3 B2 (2026-05-04).
//
// Family-level principle (cross-repo, cross-language):
//   - QFArchitecture, QFRootCauseTrace — high-richness families
//     where component / mechanism context is the answer's reason
//     to exist; demand at least 3 typed evidence rows before
//     promoting.
//   - QFCallChain, QFConfigPrecedence — chain / precedence content
//     where 2 rows is enough to anchor a useful supplementary
//     surface.
//   - default (other families) — conservative floor of 4 so the
//     advisory does not fire on shallow evidence pools.
//
// Adding a family: extend the switch with a principled threshold
// derived from cross-language richness norms — never from eval-case
// content.
func familyGlaringEvidenceThreshold(family QuestionFamily) int {
	switch family {
	case QFArchitecture, QFRootCauseTrace:
		return 3
	case QFCallChain, QFConfigPrecedence:
		return 2
	}
	return 4
}

// appendUniqueStr appends `add` strings to `dst` skipping duplicates
// and empties. Stable order; mirrors helpers used elsewhere in
// internal/types.
func appendUniqueStr(dst []string, add ...string) []string {
	seen := make(map[string]bool, len(dst)+len(add))
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range add {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		dst = append(dst, s)
	}
	return dst
}

func appendUniqueClaimForms(dst []ClaimForm, add ...ClaimForm) []ClaimForm {
	seen := make(map[ClaimForm]bool, len(dst)+len(add))
	for _, f := range dst {
		seen[f] = true
	}
	for _, f := range add {
		if f == ClaimUnknown || seen[f] {
			continue
		}
		seen[f] = true
		dst = append(dst, f)
	}
	return dst
}

func containsString(items []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}
