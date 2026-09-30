package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAggregateCardinalityDoesNotCaptureIndependentMeasurement(t *testing.T) {
	mu := types.NewMutableState("mixed measurement and mechanism")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateScalar, Label: "measured objects", Value: "389", Provenance: "command_measurement"},
		{Kind: types.AnswerAggregateMemberSet, Label: "mechanism nodes", Value: "2", Members: []string{"producer", "consumer"}, Role: types.AnswerAggregateRolePrincipalAnswer},
	})
	mu.SetInvestigationComplete("independent measurement and explanation retained")
	ctx := &types.BusContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentReturnValue, Predicates: types.SemanticPredicates{IsCountQuestion: true, IsScalarAnswer: true}}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "measured", Kind: types.BlockScalar, Text: "389"}}}
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) != 0 {
		t.Fatalf("independent scalar rebound to explanation nodes: %+v", got)
	}
	doc.Blocks[0] = types.AnswerBlock{ID: "nodes", Kind: types.BlockSummary, Text: "mechanism nodes共有 3 个"}
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) == 0 {
		t.Fatal("explicit scoped mismatch disappeared")
	}
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateMemberSet, Label: "counted objects", Value: "2", Members: []string{"one", "two"}, Role: types.AnswerAggregateRolePrincipalAnswer},
	})
	doc.Blocks[0] = types.AnswerBlock{ID: "count", Kind: types.BlockScalar, Text: "3"}
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) == 0 {
		t.Fatal("unambiguous set count mismatch disappeared")
	}
}
