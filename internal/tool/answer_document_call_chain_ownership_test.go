package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A call-chain semantic view currently carries symbol identities, not typed
// source/sink roles. Pre-emit normalization therefore must not reinterpret two
// visible anchors as endpoints and replace the model's conclusion. Individual
// structured edges remain subject to the ordinary typed evidence validators.
func TestNormalizeAnswerDocumentForPreEmit_CallChainConclusionRemainsModelOwned(t *testing.T) {
	mu := types.NewMutableState("narrative call-chain model ownership")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "explored call-chain nodes",
		Value: "5",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"VisitController.create",
			"VisitService.schedule",
			"VisitRepository.countOpenVisits",
			"VisitRepository.insert",
			"AuditLog.record",
		},
	}})
	mu.SetInvestigationComplete("narrative call-chain exploration complete")
	ctx := &types.BusContext{
		Language: "zh",
		Mutable:  mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			Language:      "zh",
			Predicates: types.SemanticPredicates{
				IsRelationalLookup: true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{
			{Text: "VisitController", Kind: types.ContractTermSymbol},
			{Text: "VisitController.create", Kind: types.ContractTermSymbol},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "模型根据逐边证据归纳出的调用链结论。"},
		{
			ID:          "path",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetCurrentCodePath), string(types.FacetPrincipalPathEdge)},
			Items: []types.AnswerBlockItem{
				{ID: "controller", Label: "VisitController.create", Text: "调用服务层。", CitationRef: -1},
				{ID: "service", Label: "VisitService.schedule", Text: "完成容量检查并调用仓储层。", CitationRef: -1},
				{ID: "repository", Label: "VisitRepository.insert", Text: "写入后记录审计。", CitationRef: -1},
			},
		},
	}}
	want, err := json.Marshal(doc.Blocks)
	if err != nil {
		t.Fatalf("marshal precondition: %v", err)
	}

	normalizeAnswerDocumentForPreEmit("emit_answer_document", doc, view, ctx, newPreEmitCheckContext(ctx))

	got, err := json.Marshal(doc.Blocks)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("pre-emit must preserve model-authored call-chain conclusion and path when no typed endpoint-role carrier exists:\n got=%s\nwant=%s", got, want)
	}
	if hints := preCheckAggregateMemberSetCoverage(doc, ctx); len(hints) != 0 {
		t.Fatalf("relation-only call-chain exploration member_set must not become a hard missing-row obligation: %+v", hints)
	}
	if hints := preCheckRelationMemberSetAnswerShape(doc, ctx); len(hints) != 0 {
		t.Fatalf("relation-only call-chain exploration member_set must not become a hard relation-table obligation: %+v", hints)
	}
}
