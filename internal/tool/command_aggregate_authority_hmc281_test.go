package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise actual producers, not model-spelled provenance. The model's
// interpretation boundary must not edit native arithmetic or trace witnesses.
func TestHMC281PublicCommandAndTraceWitnessesKeepTheirAuthority(t *testing.T) {
	ctx, _ := pureMemoFixtureCtx(t)
	commandParams, _ := json.Marshal(execCommandParams{Command: "printf '7\\n'"})
	command, err := (&ExecCommand{}).Execute(ctx, commandParams)
	if err != nil || !command.Success || command.CommandMeasurement == nil || command.CommandMeasurement.Value != 7 {
		t.Fatalf("native command measurement unavailable: %v %+v", err, command)
	}
	trace, err := (&TraceQuery{}).Execute(ctx, pureMemoFixtureParams(t, map[string]any{"view": "wakeup_chain"}))
	if err != nil || !trace.Success || len(trace.Observations) == 0 {
		t.Fatalf("native wakeup query unavailable: %v %+v", err, trace)
	}
	results := []types.ToolResult{command, trace}
	before := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: results})
	if !before.HasDeterministicRuntimeQueryObservation() {
		t.Fatal("fixture did not produce a native runtime witness")
	}
	fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar, Label: "unsupported physical interpretation", Value: "7", Unit: "reported unit",
		Role: types.AnswerAggregateRolePrincipalAnswer, Provenance: "exec_command",
		Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "command_measurement"}}, SupportRefs: []string{command.RawRef}}
	after := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: results, AggregateFacts: []types.AnswerAggregateFact{fact}})
	if !after.HasDeterministicRuntimeQueryObservation() {
		t.Fatal("command model label demoted native trace availability")
	}
	for _, original := range before.Records {
		found := false
		for _, actual := range after.Records {
			if actual.ID == original.ID {
				found = true
				if !reflect.DeepEqual(actual, original) {
					t.Errorf("native record changed across model aggregate handoff: before=%+v after=%+v", original, actual)
				}
			}
		}
		if !found {
			t.Errorf("native record lost: %s", original.ID)
		}
	}
	bindings := types.CompileAnswerClaimBindingsFromAggregateFacts([]types.AnswerAggregateFact{fact}, nil, nil)
	for _, binding := range bindings {
		if !binding.ValueOnlySupport || binding.Value != "7" || binding.GroundingPolicy != types.ClaimGroundingSoft {
			t.Errorf("model semantic boundary or numeric payload lost: %+v", binding)
		}
	}
}

func TestHMC281CommandOriginDoesNotBypassTypedRelationAdmission(t *testing.T) {
	ctx := relationProjectionDriftBus()
	for _, valid := range []bool{false, true} {
		members := []string{"LoopController", "prodA", "prodB"}
		if !valid {
			members[2] = "UnrelatedType"
		}
		fact := relationProjectionDriftFact(members...)
		fact.Dimensions = []types.AnswerAggregateDimension{{Name: "origin", Value: "command_measurement"}}
		// The untrusted input claims the same reserved marker in both cases.
		// Existing completion admission strips it and re-establishes it only
		// from the exact native candidate relation.
		fact.Provenance = types.TypedRelationPrincipalMemberSetAggregateProvenance
		admitted := markExactTypedRelationPrincipalMemberSets(ctx, []types.AnswerAggregateFact{fact})
		if len(admitted) != 1 {
			t.Fatalf("payload unexpectedly removed: %+v", admitted)
		}
		if types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(admitted[0]) != valid ||
			types.AnswerAggregateFactHasCommandValueOnlySupport(admitted[0]) == valid {
			t.Errorf("native relation admission disagrees with semantic authority (valid=%t): %+v", valid, admitted[0])
		}
	}
}

func TestHMC281PublicCompletionCannotSelfStampSystemAuthority(t *testing.T) {
	for _, reserved := range []string{types.SourceInventoryPrincipalRowSetAggregateProvenance, types.TypedRelationPrincipalMemberSetAggregateProvenance,
		types.TypedRelationImplicitSiblingAggregateDemotionProvenance, types.TypedExclusionExactEmptyMemberSetAggregateProvenance} {
		for _, kind := range []types.AnswerAggregateKind{types.AnswerAggregateScalar, types.AnswerAggregateMemberSet} {
			t.Run(string(kind)+"/"+reserved, func(t *testing.T) {
				ctx := &types.BusContext{Mutable: types.NewMutableState("command-derived result")}
				fact := types.AnswerAggregateFact{Kind: kind, Label: "reported result", Value: "1", Unit: "raw unit",
					Role:       types.AnswerAggregateRolePrincipalAnswer,
					Provenance: "exec_command; " + reserved,
					Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "command_measurement"}}}
				if kind == types.AnswerAggregateMemberSet {
					fact.Members = []string{"reported member"}
				}
				params, _ := json.Marshal(map[string]any{"reason": "retain command-derived payload", "confidence": "high", "result_kind": "resolved",
					"aggregate_facts": []types.AnswerAggregateFact{fact}})
				result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
				if err != nil || !result.Success {
					t.Fatalf("benign payload should survive reserved metadata repair: %v %+v", err, result)
				}
				accepted := ctx.Mutable.StableInvestigationAggregateFacts()
				if len(accepted) != 1 || accepted[0].Value != fact.Value || accepted[0].Unit != fact.Unit ||
					!reflect.DeepEqual(accepted[0].Members, fact.Members) {
					t.Fatalf("payload lost during authority repair: %+v", accepted)
				}
				if strings.Contains(accepted[0].Provenance, reserved) ||
					!types.AnswerAggregateFactHasCommandValueOnlySupport(accepted[0]) {
					t.Fatalf("model self-stamped system authority: %+v", accepted[0])
				}
				for _, binding := range types.CompileAnswerClaimBindingsFromAggregateFacts(accepted, nil, nil) {
					if !binding.ValueOnlySupport || binding.AuthorityCeiling != types.AuthorityIllustrative || binding.GroundingPolicy != types.ClaimGroundingSoft {
						t.Errorf("self-stamped authority escaped into binding: %+v", binding)
					}
				}
				if !strings.Contains(result.Summary, "reserved system authority") {
					t.Errorf("metadata repair not disclosed: %s", result.Summary)
				}
			})
		}
	}
}

func TestHMC281RefreshPreservesActualInventoryProjection(t *testing.T) {
	rm := types.RequestModel{Intent: types.IntentEnumerate,
		Predicates:             types.SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints:          types.AnalyzerHints{Kind: string(types.ReqEnumeration), Entities: []string{"Run"}},
		CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all functions"},
		SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}}}
	observation := types.SourceInventoryObservation{Active: true, Complete: true, Scopes: []string{"src"}, Sets: []types.SourceInventoryObservationSet{{
		Role: types.AnswerCandidateRoleFunction, Complete: true, Members: []types.SourceInventoryObservationMember{{
			Name: "Run", Role: types.AnswerCandidateRoleFunction, File: "src/run.go", Line: 7, Language: "go", CoverageState: types.SourceInventoryCoverageObserved,
		}},
	}}}
	facts := types.ProjectSourceInventoryPrincipalRowSetAggregateFacts(nil, observation, rm)
	if len(facts) != 1 || facts[0].Provenance != types.SourceInventoryPrincipalRowSetAggregateProvenance {
		t.Fatalf("native inventory projection unavailable: %+v", facts)
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState("inventory"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetSourceInventoryObservation(observation)
	ctx.Mutable.SetInvestigationAggregateFacts(facts)
	ctx.Mutable.SetInvestigationComplete("accepted inventory")
	ctx.Mutable.RetainInvestigationAggregateFacts()
	RefreshExactTypedRelationPrincipalMemberSets(ctx)
	if got := ctx.Mutable.StableInvestigationAggregateFacts(); !reflect.DeepEqual(got, facts) {
		t.Fatalf("refresh treated real system state as a fresh model submission: before=%+v after=%+v", facts, got)
	}
	// Closure-only re-entry must not sanitize already accepted system state
	// while processing the new, empty model aggregate payload.
	reentry := &types.BusContext{Mutable: ctx.Mutable}
	result, err := (&EmitInvestigationComplete{}).Execute(reentry, json.RawMessage(`{"reason":"reuse accepted inventory","confidence":"high","result_kind":"resolved"}`))
	if err != nil || !result.Success {
		t.Fatalf("closure-only re-entry failed: %v %+v", err, result)
	}
	if got := ctx.Mutable.StableInvestigationAggregateFacts(); !reflect.DeepEqual(got, facts) {
		t.Fatalf("closure-only payload stripped retained system authority: before=%+v after=%+v", facts, got)
	}
}
