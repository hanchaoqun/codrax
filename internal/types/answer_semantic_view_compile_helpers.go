package types

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
func diagramPlanFor(plan *AnswerSurfacePlan, kind DiagramKind, nodeFacets []string, edgeFacets []string) *DiagramFacetGraph {
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
		Required:   contract.Required,
		Kind:       resolvedKind,
		NodeFacets: nodeFacets,
		EdgeFacets: edgeFacets,
	}
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
