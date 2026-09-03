package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_contract_retirement_framing_test.go — V5-3 / V5-4 framing pins
// (colleague_merge_audit §40.23 item 3, §40.24 item 1): the planner and the
// controller render the ACTIVE contract generation — retired ids appear only
// as retired lines that carry reason and evidence, never as required refs.

func retirementFramingIR() *types.WriteAnalysisIR {
	return &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "repair"},
		BehaviorContracts: []types.WriteBehaviorContract{
			{ID: "hard-api", Kind: types.WriteBehaviorInvariant, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals, Expected: "public API remains compatible", Required: true, Source: "write_analyzer"},
			{ID: "stale-soft", Kind: types.WriteBehaviorInvariant, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies, Expected: "the rejected implementation shape", Required: true, Source: "write_analyzer"},
			{ID: "sibling-soft", Kind: types.WriteBehaviorInvariant, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies, Expected: "unrelated soft expectation", Required: true, Source: "write_analyzer"},
		}}}
}

func retirementFramingHandoff() *types.VerifyFailureHandoff {
	return &types.VerifyFailureHandoff{
		PlanID: "plan-1", BatchID: "batch-1", Attempt: 1, FailureKind: types.FailureKindTestsFailed,
		ContractRelevance: &types.VerifyFailureContractRelevance{Status: types.VerifyFailureContractRelevanceAvailable, ReasonCode: "typed_failed_rows_joined", Hits: []types.VerifyFailureContractHit{{
			ContractID: "stale-soft", Reason: types.WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:shape_probe"},
		}}},
	}
}

func TestPlannerFramingRequiredRefsExcludeRetiredIDs(t *testing.T) {
	mu := types.NewMutableState("repair")
	mu.SetWriteAnalysisIR(retirementFramingIR())
	mu.SetVerifyFailureHandoff(retirementFramingHandoff())
	got := (&plannerEvaluator{}).buildTaskFramingSection(&types.AgentContext{Mutable: mu})
	if !strings.Contains(got, "required contract_refs for a verified close: hard-api, sibling-soft\n") {
		t.Fatalf("required refs must be the active generation (retired id excluded, sibling retained):\n%s", got)
	}
	if !strings.Contains(got, "retired contract id (do not reference): stale-soft — retired by failed verification evidence after verification attempt 1 of plan plan-1 failed (tests_failed); evidence probe:shape_probe") {
		t.Fatalf("retired line missing or unworded:\n%s", got)
	}
	if strings.Contains(got, "id=stale-soft kind=") {
		t.Fatalf("retired contract still rendered as active:\n%s", got)
	}
	// Without a handoff the framing is the analyzer snapshot unchanged.
	mu.ResetVerifyFailureHandoff()
	plain := (&plannerEvaluator{}).buildTaskFramingSection(&types.AgentContext{Mutable: mu})
	if strings.Contains(plain, "retired contract id") || !strings.Contains(plain, "required contract_refs for a verified close: hard-api, sibling-soft, stale-soft") {
		t.Fatalf("plain round drifted:\n%s", plain)
	}
}

func TestPlannerHandoffSectionRendersContractRelevance(t *testing.T) {
	mu := types.NewMutableState("repair")
	mu.SetVerifyFailureHandoff(retirementFramingHandoff())
	got := (&plannerEvaluator{}).buildVerifyFailureHandoffSection(&types.AgentContext{Mutable: mu})
	if !strings.Contains(got, "- contract_relevance: status=available reason_code=typed_failed_rows_joined hits=stale-soft(failed_verification_probe; probe:shape_probe)") {
		t.Fatalf("relevance line missing:\n%s", got)
	}
}

func TestWriteControllerTaskSectionRetiresOnFirstReplan(t *testing.T) {
	mu := types.NewMutableState("repair")
	mu.SetWriteAnalysisIR(retirementFramingIR())
	// The failed plan is still generation 0 on the first replan decision.
	mu.SetChangePlan(&types.ChangePlan{ID: "plan-1", BehaviorContracts: retirementFramingIR().Request.BehaviorContracts})
	mu.SetVerifyFailureHandoff(retirementFramingHandoff())
	got := renderWriteControllerTaskSection(&types.AgentContext{Mutable: mu})
	if !strings.Contains(got, "- superseded_behavior_contract: stale-soft (retired by failed verification evidence after verification attempt 1 of plan plan-1 failed (tests_failed); evidence probe:shape_probe)\n") {
		t.Fatalf("controller section must show the retirement on the first replan:\n%s", got)
	}
	if strings.Contains(got, "id=stale-soft") || !strings.Contains(got, "id=sibling-soft") {
		t.Fatalf("controller section must render the active generation only:\n%s", got)
	}
}

// ---- §40.46 fold-in pins (C3 second-replan agreement, C1 pack face).

func retirementFramingRebasedPlan2() *types.ChangePlan {
	return &types.ChangePlan{ID: "plan-2", BehaviorContractGeneration: types.WriteBehaviorContractGenerationPlanAcceptanceRebase,
		BehaviorContracts:           []types.WriteBehaviorContract{retirementFramingIR().Request.BehaviorContracts[0], retirementFramingIR().Request.BehaviorContracts[2]},
		SupersededBehaviorContracts: []types.WriteBehaviorContractTombstone{{ID: "stale-soft", Reason: types.WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:shape_probe"}, PlanID: "plan-1", Attempt: 1, FailureKind: types.FailureKindTestsFailed}}}
}

func TestPlannerAndControllerAgreeOnSecondReplan(t *testing.T) {
	mu := types.NewMutableState("repair")
	mu.SetWriteAnalysisIR(retirementFramingIR())
	// Round 1 produced the rebased plan-2 (its tombstone seeds the ledger);
	// plan-2 then failed with an unrelated build failure, and the scheduler
	// replaced the handoff with one carrying no hits.
	mu.SetChangePlan(retirementFramingRebasedPlan2())
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{PlanID: "plan-2", BatchID: "batch-1", Attempt: 2, FailureKind: types.FailureKindBuildFailure,
		ContractRelevance: &types.VerifyFailureContractRelevance{Status: types.VerifyFailureContractRelevanceAvailable, ReasonCode: "typed_failed_rows_joined"}})
	retiredLine := "stale-soft"
	controller := renderWriteControllerTaskSection(&types.AgentContext{Mutable: mu})
	if !strings.Contains(controller, "- superseded_behavior_contract: stale-soft (retired by failed verification evidence after verification attempt 1 of plan plan-1 failed (tests_failed); evidence probe:shape_probe)\n") || strings.Contains(controller, "id=stale-soft") {
		t.Fatalf("controller section on the second replan:\n%s", controller)
	}
	for _, reset := range []bool{false, true} {
		if reset {
			mu.ResetChangePlan() // prepareControllerPlanningState
		}
		planner := (&plannerEvaluator{}).buildTaskFramingSection(&types.AgentContext{Mutable: mu})
		if !strings.Contains(planner, "required contract_refs for a verified close: hard-api, sibling-soft\n") || strings.Contains(planner, "id="+retiredLine+" kind=") {
			t.Fatalf("planner framing (plan reset=%v) re-advertised the retired id:\n%s", reset, planner)
		}
		if !strings.Contains(planner, "retired contract id (do not reference): stale-soft — retired by failed verification evidence after verification attempt 1 of plan plan-1 failed (tests_failed); evidence probe:shape_probe") {
			t.Fatalf("planner framing (plan reset=%v) lost the retired line:\n%s", reset, planner)
		}
	}
	// A cleared handoff (green verify) does not reinstate either.
	mu.ResetVerifyFailureHandoff()
	planner := (&plannerEvaluator{}).buildTaskFramingSection(&types.AgentContext{Mutable: mu})
	if strings.Contains(planner, "id=stale-soft kind=") || !strings.Contains(planner, "retired contract id (do not reference): stale-soft") {
		t.Fatalf("handoff reset reinstated the retired id:\n%s", planner)
	}
}

// TestPlannerContextPackRendersRetiredContractOnFirstReplan — §40.46 C1: on
// the round that PLANS the repair (no rebased plan exists yet) the merged
// pack section of the planner prompt shows the retired analyzer contract as
// retired, same-sourced with the framing, never as a live soft_required row.
func TestPlannerContextPackRendersRetiredContractOnFirstReplan(t *testing.T) {
	mu := types.NewMutableState("repair")
	ir := retirementFramingIR()
	mu.SetWriteAnalysisIR(ir)
	mu.MergeWriteContextPack(types.WriteContextPackFromWriteAnalysisIR(ir))
	mu.MergeWriteContextPack(types.WriteContextPackFromChangePlan(&types.ChangePlan{ID: "plan-1", BehaviorContracts: ir.Request.BehaviorContracts}))
	mu.SetVerifyFailureHandoff(retirementFramingHandoff())
	got := buildWriteContextPackPromptSection(&types.AgentContext{Mutable: mu}, types.WriteConsumerPlanner, "", 40)
	if strings.Contains(got, "id=stale-soft kind=") || strings.Contains(got, "behavior_contract [write_analysis/stale-soft]") || strings.Contains(got, "behavior_contract [plan/stale-soft]") {
		t.Fatalf("pack still renders the retired id as a live contract:\n%s", got)
	}
	if !strings.Contains(got, "behavior_contract_retired [write_analysis/stale-soft]: id=stale-soft reason=failed_verification_probe evidence=probe:shape_probe failed_plan_id=plan-1 attempt=1") {
		t.Fatalf("pack lacks the retired item:\n%s", got)
	}
	if !strings.Contains(got, "behavior_contract [write_analysis/sibling-soft]: id=sibling-soft") {
		t.Fatalf("active sibling dropped from the pack:\n%s", got)
	}
	framing := (&plannerEvaluator{}).buildTaskFramingSection(&types.AgentContext{Mutable: mu})
	if !strings.Contains(framing, "retired contract id (do not reference): stale-soft") {
		t.Fatalf("framing and pack must agree:\n%s", framing)
	}
}
