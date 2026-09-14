package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1671Contract(id string, operator types.WriteBehaviorOperator) types.WriteBehaviorContract {
	return types.WriteBehaviorContract{
		ID: id, Kind: types.WriteBehaviorInvariant, Polarity: types.WriteBehaviorPolarityExpected,
		Operator: operator, Expected: "preserve the requested behavior", Required: true, Source: "write_analyzer",
	}
}

func b1671Prompt(t *testing.T, plan *types.ChangePlan, report *types.ChangeReport) string {
	t.Helper()
	mut := types.NewMutableState("implement the requested change")
	mut.SetChangePlan(plan)
	if report != nil {
		mut.SetChangeReport(report)
	}
	before, err := json.Marshal([]any{plan, report})
	if err != nil {
		t.Fatal(err)
	}
	proofBefore, err := json.Marshal(types.BuildVerificationProofLedger(plan, report, nil))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{Mutable: mut, Mode: types.ModeApply}
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
	if replay := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil); replay != got {
		t.Fatal("controller witness disclosure is not replay-stable")
	}
	after, err := json.Marshal([]any{plan, report})
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("controller disclosure changed plan, model contract prose or report")
	}
	proofAfter, err := json.Marshal(types.BuildVerificationProofLedger(plan, report, nil))
	if err != nil || !bytes.Equal(proofBefore, proofAfter) {
		t.Fatal("controller witness counts changed authoritative proof obligations")
	}
	return got
}

func b1671AssertScope(t *testing.T, got string, hard, hardCovered, soft, softCovered, planning int) {
	t.Helper()
	want := fmt.Sprintf("verification_behavior_witness_scope: hard_required_typed_contracts=%d covered_hard_required_typed_contracts=%d soft_required_typed_contracts=%d covered_soft_required_typed_contracts=%d planning_only_contracts=%d", hard, hardCovered, soft, softCovered, planning)
	if !strings.Contains(got, want) {
		t.Errorf("controller missing exact required witness census %q", want)
	}
	for _, want := range []string{
		"soft-required contracts remain required proof obligations, not optional guidance",
		"not complete proof or workflow completion",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("controller missing required/proof boundary %q", want)
		}
	}
}

func TestB1671ControllerRequiredWitnessCategories(t *testing.T) {
	hard := b1671Contract("hard", types.WriteBehaviorOpEquals)
	soft := b1671Contract("soft", types.WriteBehaviorOpSatisfies)
	planning := b1671Contract("planning", types.WriteBehaviorOpEquals)
	planning.Source += ";" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded
	observed := hard
	observed.ID, observed.Polarity = "observed", types.WriteBehaviorPolarityObserved
	optional := hard
	optional.ID, optional.Required = "optional", false
	for _, tc := range []struct {
		name      string
		contracts []types.WriteBehaviorContract
		hard      int
		soft      int
		planning  int
	}{
		{"hard", []types.WriteBehaviorContract{hard}, 1, 0, 0},
		{"soft", []types.WriteBehaviorContract{soft}, 0, 1, 0},
		{"planning", []types.WriteBehaviorContract{planning}, 0, 0, 1},
		{"mixed", []types.WriteBehaviorContract{hard, soft, planning, observed, optional}, 1, 1, 1},
		{"no_obligation", []types.WriteBehaviorContract{observed, optional}, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &types.ChangePlan{ID: "current", Status: types.PlanStatusApplied, BehaviorContracts: tc.contracts}
			report := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true}
			got := b1671Prompt(t, plan, report)
			b1671AssertScope(t, got, tc.hard, 0, tc.soft, 0, tc.planning)
			if tc.soft != 0 && !strings.Contains(got, "contract_ref=\"soft\"") {
				t.Error("soft required invariant disappeared from actual unresolved proof obligations")
			}
		})
	}
}

func TestB1671ControllerRequiredWitnessMatrix(t *testing.T) {
	for _, tc := range []struct {
		name     string
		category string
		witness  types.WriteBehaviorWitnessKind
		status   string
		hard     int
		soft     int
	}{
		{"source_cannot_witness_runtime", "source_contract_refs", types.WriteBehaviorWitnessSourceText, "satisfied", 1, 0},
		{"executed_probe", "probe_soft_contract_refs", types.WriteBehaviorWitnessVerificationProbe, "satisfied", 2, 1},
		{"executed_project_test", "project_test_contract_refs", types.WriteBehaviorWitnessProjectTest, "satisfied", 2, 1},
		{"mismatched_witness", "source_contract_refs", types.WriteBehaviorWitnessProjectTest, "satisfied", 0, 0},
		{"unsatisfied", "project_test_contract_refs", types.WriteBehaviorWitnessProjectTest, "missing", 0, 0},
		{"non_contract_category", "changed_path_coverage", types.WriteBehaviorWitnessProjectTest, "satisfied", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := b1671Contract("layout", types.WriteBehaviorOpEquals)
			layout.Kind = types.WriteBehaviorFileLayout
			plan := &types.ChangePlan{ID: "current", Status: types.PlanStatusApplied, BehaviorContracts: []types.WriteBehaviorContract{
				b1671Contract("hard", types.WriteBehaviorOpEquals), b1671Contract("soft", types.WriteBehaviorOpSatisfies), layout,
			}}
			report := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true,
				VerificationConfidence: []types.VerificationConfidenceRecord{{
					Source: "verifier", Category: tc.category, WitnessKind: tc.witness, Status: tc.status,
					ContractRefs: []string{"hard", "soft", "layout", "soft", "foreign"},
				}},
			}
			b1671AssertScope(t, b1671Prompt(t, plan, report), 2, tc.hard, 1, tc.soft, 0)
		})
	}
}

func TestB1671ControllerRequiredScopeRetainsHistoryWithoutDuplicates(t *testing.T) {
	current := b1671Contract("same-id", types.WriteBehaviorOpSatisfies)
	planning := b1671Contract("planning", types.WriteBehaviorOpEquals)
	planning.Source = "write_analyzer;" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded + ";other"
	legacy := b1671Contract("legacy-planning", types.WriteBehaviorOpSatisfies)
	legacy.Source = "expected_outcome_fallback"
	plan := &types.ChangePlan{ID: "current", Status: types.PlanStatusApplied,
		BehaviorContracts:             []types.WriteBehaviorContract{current, planning, legacy},
		SupersededBehaviorContractIDs: []string{"retired"},
		CumulativeVerificationScope: &types.CumulativeVerificationScope{SourcePlanIDs: []string{"previous"},
			BehaviorContracts: []types.WriteBehaviorContract{
				b1671Contract("same-id", types.WriteBehaviorOpEquals), b1671Contract("retained", types.WriteBehaviorOpEquals),
				b1671Contract("retired", types.WriteBehaviorOpEquals), planning,
			}},
	}
	report := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source: "verifier", Category: "project_test_contract_refs", Status: "satisfied",
			WitnessKind: types.WriteBehaviorWitnessProjectTest, ContractRefs: []string{"same-id", "retained", "retired", "planning", "legacy-planning"},
		}},
	}
	b1671AssertScope(t, b1671Prompt(t, plan, report), 1, 1, 1, 1, 2)
}

func TestB1671ControllerAbsentAuthoritativeReportDoesNotInventWitnessCoverage(t *testing.T) {
	plan := &types.ChangePlan{ID: "current", Status: types.PlanStatusApplied,
		BehaviorContracts: []types.WriteBehaviorContract{b1671Contract("soft", types.WriteBehaviorOpSatisfies)}}
	for _, tc := range []struct {
		name   string
		report *types.ChangeReport
	}{
		{"nil", nil},
		{"stale", &types.ChangeReport{PlanID: "previous", Channel: types.ChangeReportChannelPostApplyVerify, Passed: true}},
		{"planner_probe", &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPlannerProbe, Passed: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := b1671Prompt(t, plan, tc.report)
			if strings.Contains(got, "verification_behavior_witness_scope:") || strings.Contains(got, "verification_proof_scope:") {
				t.Error("a missing/currently ineligible report fabricated verification coverage")
			}
		})
	}
}

func TestB1671ControllerSoftOnlyWitnessAndNilReport(t *testing.T) {
	plan := &types.ChangePlan{ID: "soft-only", Status: types.PlanStatusApplied,
		BehaviorContracts: []types.WriteBehaviorContract{b1671Contract("soft", types.WriteBehaviorOpSatisfies)}}
	if got := writeControllerBehaviorContractCoverage(plan, nil); got != (writeControllerBehaviorCoverage{soft: 1}) {
		t.Fatalf("missing report changed required population or invented coverage: %+v", got)
	}
	if got := writeControllerBehaviorContractCoverage(nil, nil); got != (writeControllerBehaviorCoverage{}) {
		t.Fatalf("missing plan invented obligations: %+v", got)
	}
	report := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source: "verifier", Category: "project_test_contract_refs", Status: "satisfied",
			WitnessKind: types.WriteBehaviorWitnessProjectTest, ContractRefs: []string{"soft"},
		}},
	}
	b1671AssertScope(t, b1671Prompt(t, plan, report), 0, 0, 1, 1, 0)
}
