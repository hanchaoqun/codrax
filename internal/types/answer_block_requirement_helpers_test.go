package types

import "testing"

func TestCountAnswerBlocksForRequirementScopesSharedKindsByFacet(t *testing.T) {
	req := BlockRequirement{
		Kind:             BlockTable,
		AlternativeKinds: []AnswerBlockKind{BlockOrderedList},
		MinCount:         1,
		MaxCount:         1,
		Required:         true,
		FacetIDs:         []string{string(FacetConfigPrecedenceRole)},
	}
	blocks := []AnswerBlock{
		{ID: "precedence", Kind: BlockTable, FacetIDs: []string{string(FacetConfigPrecedenceRole)}},
		{ID: "topic-a", Kind: BlockOrderedList, FacetIDs: []string{string(FacetEnumerationItem)}},
		{ID: "topic-b", Kind: BlockOrderedList, FacetIDs: []string{string(FacetCurrentCodePath)}},
	}
	if got := CountAnswerBlocksForRequirement(blocks, req); got != 1 {
		t.Fatalf("typed requirement count=%d, want 1; sibling dimensions sharing list/table kinds must not consume the cap", got)
	}
	if got := CountAnswerBlocksForRequirementKinds(blocks, req); got != 3 {
		t.Fatalf("kind-only ownership floor count=%d, want 3", got)
	}
}

func TestCountAnswerBlocksForRequirementStillCountsSameFacetAlternatives(t *testing.T) {
	req := BlockRequirement{
		Kind:             BlockTable,
		AlternativeKinds: []AnswerBlockKind{BlockOrderedList},
		FacetIDs:         []string{string(FacetConfigPrecedenceRole)},
	}
	blocks := []AnswerBlock{
		{Kind: BlockTable, FacetIDs: []string{string(FacetConfigPrecedenceRole)}},
		{Kind: BlockOrderedList, FacetIDs: []string{string(FacetConfigPrecedenceRole)}},
	}
	if got := CountAnswerBlocksForRequirement(blocks, req); got != 2 {
		t.Fatalf("same-facet alternative carriers must still trigger MaxCount: got %d", got)
	}
}

func TestCountAnswerBlocksForRequirementAcceptsClaimUseFacetOwnership(t *testing.T) {
	req := BlockRequirement{
		Kind:     BlockTable,
		FacetIDs: []string{string(FacetConfigPrecedenceRole)},
	}
	blocks := []AnswerBlock{{
		Kind: BlockTable,
		ClaimUses: []RenderedClaimUse{{
			FacetID:   string(FacetConfigPrecedenceRole),
			ClaimForm: ClaimDefinitionFact,
		}},
	}}
	if got := CountAnswerBlocksForRequirement(blocks, req); got != 1 {
		t.Fatalf("claim-use facet ownership must count in the same carrier domain: got %d", got)
	}
}

func TestCountAnswerBlocksForRequirementWithoutFacetsKeepsKindOnlyBehavior(t *testing.T) {
	req := BlockRequirement{Kind: BlockSummary}
	blocks := []AnswerBlock{
		{Kind: BlockSummary},
		{Kind: BlockSummary, FacetIDs: []string{string(FacetCurrentCodePath)}},
		{Kind: BlockTable},
	}
	if got := CountAnswerBlocksForRequirement(blocks, req); got != 2 {
		t.Fatalf("facetless requirement count=%d, want historical kind-only count 2", got)
	}
}
