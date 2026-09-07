package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1604MemberCoverageValidatorDistinguishesCandidatesFromAcceptedSet(t *testing.T) {
	rm := types.RequestModel{Intent: types.IntentEnumerate, PredicateAxis: types.AxisImplement,
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true}}
	evidence := []types.EvidenceItem{
		{ID: "member", Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: "src/output.rs", LineStart: 8, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Output", Subject: "Output", Snippet: "struct Output;", GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo},
		{ID: "base", Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: "src/base.rs", LineStart: 4, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Base", Subject: "Base", Snippet: "trait Base {}", GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo},
	}
	m := types.NewMutableState("")
	buildSupport := func() *types.AnswerSupportPlan {
		return types.BuildAnswerSupportPlanForAgentContext(&types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: m, EvidenceItems: evidence})
	}
	doc := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "src/output.rs", Line: 8}}, Blocks: []types.AnswerBlock{{ID: "members", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		FacetIDs: []string{string(types.FacetEnumerationItem)}, Items: []types.AnswerBlockItem{{ID: "output", Label: "Output", CitationRef: 0}}}}}
	support := buildSupport()
	if got := validatePrincipalSupportMemberCoverage(doc, support); len(got) != 0 {
		t.Fatalf("base/helper candidates became missing members: %+v", got)
	}
	m.SetEmittedAnswerSymbolsWithOrigin([]types.AnswerSymbol{{Name: "Output", File: "src/output.rs", Line: 8, Kind: types.KindType}}, types.CompletenessComplete, types.AnswerSymbolSelectionExplicitItems)
	support = buildSupport()
	if len(types.PrincipalSupportMemberObligations(support)) != 1 {
		t.Fatalf("explicit member proof lost: %+v", support)
	}
	if got := validatePrincipalSupportMemberCoverage(doc, support); len(got) != 0 {
		t.Fatalf("properly cited explicit member rejected: %+v", got)
	}
	doc.Blocks[0].Items = nil
	if got := validatePrincipalSupportMemberCoverage(doc, support); len(got) != 1 || got[0].Kind != types.ViolPrincipalSupportMemberOmitted {
		t.Fatalf("real explicit member omission no longer reported: %+v", got)
	}
	// The alternate authority is the existing typed relation member-set
	// receipt, not symbol selection. Its omission must remain load-bearing.
	m.ResetEmittedAnswerSymbols()
	m.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer, Label: "proved relation members", Value: "1",
		Members: []string{"Output"}, SupportRefs: []string{"Output @ src/output.rs:8"}, Provenance: types.TypedRelationPrincipalMemberSetAggregateProvenance}})
	m.RetainInvestigationAggregateFacts()
	support = buildSupport()
	if got := validatePrincipalSupportMemberCoverage(doc, support); len(got) != 1 || got[0].Kind != types.ViolPrincipalSupportMemberOmitted {
		t.Fatalf("typed relation set omission no longer reported: %+v", got)
	}
}
