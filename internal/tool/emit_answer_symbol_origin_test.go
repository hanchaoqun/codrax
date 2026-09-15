package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1604ActualSymbolProducerRejectsUnwitnessedMaterialization(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	explicit := json.RawMessage(`{"items":[{"name":"Output","file":"src/output.go","line":8,"kind":"type"}],"completeness":"complete"}`)
	res, err := tool.Execute(ctx, explicit)
	if err != nil || !res.Success {
		t.Fatalf("explicit producer failed: %+v %v", res, err)
	}
	syms, claim, origin := ctx.Mutable.EmittedAnswerSymbolsWithOrigin()
	if len(syms) != 1 || syms[0].Name != "Output" || claim != types.CompletenessComplete || origin != types.AnswerSymbolSelectionExplicitItems {
		t.Fatalf("wrong explicit source: %+v %s %s", syms, claim, origin)
	}
	res, err = tool.Execute(ctx, json.RawMessage(`{"items":[{"name":"Bad","file":"src/bad.go","line":0,"kind":"type"}],"completeness":"complete"}`))
	if err != nil || res.Success {
		t.Fatalf("invalid symbol must fail: %+v %v", res, err)
	}
	syms, _, origin = ctx.Mutable.EmittedAnswerSymbolsWithOrigin()
	if len(syms) != 1 || syms[0].Name != "Output" || origin != types.AnswerSymbolSelectionExplicitItems {
		t.Fatal("failed emit replaced accepted slate/source")
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleType}, Confidence: .95},
	}}
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
		Label: "source declarations", Value: "2", Provenance: "system:source_inventory", Members: []string{"First", "Second"}, SupportRefs: []string{"First: src/first.go:8", "Second: src/second.go:9"},
	}})
	ctx.Mutable.RetainInvestigationAggregateFacts()
	res, err = tool.Execute(ctx, json.RawMessage(`{"items":[],"completeness":"unknown"}`))
	if err != nil || !res.Success {
		t.Fatalf("empty slate call failed: %+v %v", res, err)
	}
	syms, claim, origin = ctx.Mutable.EmittedAnswerSymbolsWithOrigin()
	// B1700 preserves the model payload above, but it contains no independent
	// source witness. The earlier explicit slate must not be replaced with
	// a falsely system-materialized roster. Native-positive origin/fork
	// behavior is exercised by the actual source inventory fallback tests.
	if len(syms) != 0 || claim != types.CompletenessUnknown || origin != types.AnswerSymbolSelectionUnknown {
		t.Fatalf("unwitnessed model refs became system materialization: %+v %s %s", syms, claim, origin)
	}
	// A later profile change cannot convert the rejected model source claim
	// into a historical system slate or an explicit model selection.
	ctx.AnalysisIR.RequestModel.SourceInventoryProfile = nil
	_, _, origin = ctx.Mutable.ForkForExploreDispatch().EmittedAnswerSymbolsWithOrigin()
	if origin != types.AnswerSymbolSelectionUnknown {
		t.Fatal("profile change minted source ownership for an empty slate")
	}
	res, err = tool.Execute(ctx, json.RawMessage(`{"items":[],"completeness":"unknown"}`))
	if err != nil || !res.Success {
		t.Fatalf("empty slate failed unexpectedly: %+v %v", res, err)
	}
	syms, _, origin = ctx.Mutable.EmittedAnswerSymbolsWithOrigin()
	if len(syms) != 0 || origin != types.AnswerSymbolSelectionUnknown {
		t.Fatalf("empty accepted slate invented member authority: %+v %s", syms, origin)
	}
}
