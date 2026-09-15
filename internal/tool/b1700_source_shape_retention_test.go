package tool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1700UnprovedDerivedValueRetentionDoesNotGrantPrincipalAuthority(t *testing.T) {
	for _, test := range []struct {
		name, aggregateValue string
		wantFixed, wantRef   int
	}{
		{"matching-derived-value", "50", 0, 0},
		{"different-derived-value", "51", 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			mu := types.NewMutableState("derived value retention")
			mu.AppendEvidence([]types.EvidenceItem{
				{ID: "source-role", Kind: types.EvidenceRelationship, Scope: types.ScopeLine, Source: "registry.go", LineStart: 4,
					AnchorKind: types.AnchorCall, Subject: "Register", Object: "Worker", Snippet: "register(Worker)", GroundingStatus: types.GroundingGrounded},
				{ID: "source-literal", Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: "limits.go", LineStart: 8,
					AnchorKind: types.AnchorDefinition, AnchorSymbol: "Limit", Snippet: "const Limit = 50", GroundingStatus: types.GroundingGrounded},
			})
			fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
				Label: "model-derived count", Value: test.aggregateValue, Members: []string{"Ghost"}, SupportRefs: []string{"registry.go:4"}}
			mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
			mu.SetInvestigationComplete("retained model proposal")
			ctx := &types.BusContext{Mutable: mu}
			if types.AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, nil, types.AnswerAggregateSourceContextFromBusContext(ctx)) {
				t.Fatal("fixture must retain an unproved source member")
			}
			before, _ := json.Marshal(mu.StableInvestigationAggregateFacts())
			doc := &types.AnswerDocumentV2{Citations: []types.Citation{
				{File: "registry.go", Line: 4, Quote: "register(Worker)"}, {File: "limits.go", Line: 8, Quote: "const Limit = 50"},
			}, Blocks: []types.AnswerBlock{{ID: "count", Kind: types.BlockScalar, Text: "50",
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimLiteralValueFact}}, Items: []types.AnswerBlockItem{{ID: "value", CitationRef: 0}}}}}
			if fixed := normalizeScalarLiteralCitationRefsWithContext(doc, ctx, nil); fixed != test.wantFixed || doc.Blocks[0].Items[0].CitationRef != test.wantRef {
				t.Fatalf("retention repair=%d ref=%d want=%d/%d", fixed, doc.Blocks[0].Items[0].CitationRef, test.wantFixed, test.wantRef)
			}
			after, _ := json.Marshal(mu.StableInvestigationAggregateFacts())
			if string(before) != string(after) || doc.Blocks[0].Text != "50" || len(doc.Citations) != 2 {
				t.Fatal("retention changed model facts, value text, or citation pool")
			}
		})
	}
}

func TestB1700RequestedSourceLocationSkipsOnlyInheritedCitationRoles(t *testing.T) {
	for _, test := range []struct {
		name     string
		edit     func(*types.RequestModel, *types.AnswerBlock, *types.Citation)
		wantHint bool
	}{
		{"files", nil, false},
		{"sites", func(rm *types.RequestModel, block *types.AnswerBlock, _ *types.Citation) {
			rm.ChangeImpactProfile.RequestedOutput = types.ImpactOutputSites
			block.Items[0].Label = "src/worker.go:10"
		}, false},
		{"wrong-file", func(_ *types.RequestModel, _ *types.AnswerBlock, cit *types.Citation) { cit.File = "src/other.go" }, true},
		{"wrong-site", func(rm *types.RequestModel, block *types.AnswerBlock, _ *types.Citation) {
			rm.ChangeImpactProfile.RequestedOutput = types.ImpactOutputSites
			block.Items[0].Label = "src/worker.go:11"
		}, true},
		{"non-path-label", func(_ *types.RequestModel, block *types.AnswerBlock, _ *types.Citation) {
			block.Items[0].Label = "Worker"
		}, true},
		{"inactive-profile", func(rm *types.RequestModel, _ *types.AnswerBlock, _ *types.Citation) {
			rm.ChangeImpactProfile.IsChangeImpact = false
		}, true},
		{"unknown-output", func(rm *types.RequestModel, _ *types.AnswerBlock, _ *types.Citation) {
			rm.ChangeImpactProfile.RequestedOutput = types.ImpactOutputUnknown
		}, true},
		{"explicit-claim-use", func(_ *types.RequestModel, block *types.AnswerBlock, _ *types.Citation) {
			block.ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}}
		}, true},
		{"explicit-definition-keeps-its-own-role", func(_ *types.RequestModel, block *types.AnswerBlock, _ *types.Citation) {
			block.ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}}
		}, false},
		{"explicit-principal-path", func(_ *types.RequestModel, block *types.AnswerBlock, _ *types.Citation) {
			block.FacetIDs = []string{string(types.FacetPrincipalPathEdge)}
		}, true},
		{"explicit-current-code-path", func(_ *types.RequestModel, block *types.AnswerBlock, _ *types.Citation) {
			block.FacetIDs = []string{string(types.FacetCurrentCodePath)}
		}, true},
		{"matching-explicit-edge", func(_ *types.RequestModel, block *types.AnswerBlock, cit *types.Citation) {
			block.ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}}
			block.Items[0].Label, cit.File, cit.Line = "src/edge.go", "src/edge.go", 20
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rm := types.RequestModel{Intent: types.IntentEnumerate,
				ChangeImpactProfile: &types.ChangeImpactProfile{IsChangeImpact: true, RequestedOutput: types.ImpactOutputFiles}}
			block := types.AnswerBlock{ID: "files", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
				FacetIDs: []string{string(types.FacetEnumerationItem)}, Items: []types.AnswerBlockItem{{
					ID: "worker", Label: "src/worker.go", Text: "Before -> After", CitationRef: 0,
				}}}
			cit := types.Citation{File: "src/worker.go", Line: 10}
			if test.edit != nil {
				test.edit(&rm, &block, &cit)
			}
			mu := types.NewMutableState("file-shaped source proposal")
			mu.AppendEvidence([]types.EvidenceItem{{ID: "real-edge", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
				Source: "src/edge.go", LineStart: 20, AnchorKind: types.AnchorCall, Subject: "Before", Object: "After", GroundingStatus: types.GroundingGrounded}})
			fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
				Value: "1", Members: []string{"src/worker.go:10"}, SupportRefs: []string{"src/worker.go:10"}}
			mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
			mu.SetInvestigationComplete("model-proposed file")
			ctx := &types.BusContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
			if types.AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, types.AnswerAggregateSourceContextFromBusContext(ctx)) {
				t.Fatal("shape fixture acquired source membership authority")
			}
			doc := &types.AnswerDocumentV2{Citations: []types.Citation{cit}, Blocks: []types.AnswerBlock{block}}
			before, _ := json.Marshal(doc)
			view := &types.AnswerSemanticView{Family: types.QFEnumeration, RequiredBlocks: []types.BlockRequirement{{
				Kind: types.BlockOrderedList, Required: true, AcceptableClaimForms: []types.ClaimForm{types.ClaimCallEdge}, SurfaceRoleHint: types.SurfacePrincipal,
			}}}
			hints := preCheckCallChainItemCitationRoleAlignment(doc, view, ctx)
			if (len(hints) > 0) != test.wantHint {
				t.Fatalf("role-alignment hints=%+v wantHint=%v", hints, test.wantHint)
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) || !reflect.DeepEqual(mu.StableInvestigationAggregateFacts(), []types.AnswerAggregateFact{fact}) {
				t.Fatal("shape-only role validation rewrote the model document or fact")
			}
		})
	}
}
