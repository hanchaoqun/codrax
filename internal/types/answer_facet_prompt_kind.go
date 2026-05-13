package types

// BlockKindForFacetForPrompt exposes the canonical fallback surface for a
// facet when a family-specific semantic view forgot to name a block owner.
// It is intentionally prompt-only: validators still read the precise
// AnswerSemanticView / FacetCoverage contracts, while this helper gives the
// finalizer an actionable placement hint instead of making it discover the
// missing facet through a same-dispatch rejection.
func BlockKindForFacetForPrompt(facet AnswerFacetKind) AnswerBlockKind {
	return blockKindForFacet(facet)
}
