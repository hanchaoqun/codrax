package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProofPlanningAssertionWitnessUnavailable(t *testing.T) {
	debt := func(kind, ref string) types.VerificationProofLedgerItem {
		return types.VerificationProofLedgerItem{Kind: kind, ContractRef: ref, Status: types.VerificationProofLedgerItemMissing}
	}
	observation := types.ProjectTestObservation{ID: "native", TestPath: "tests/test_value.py", AssertionSuite: "ValueTest", AssertionID: "test_value", ContractRefs: []string{"value"}}
	for _, tc := range []struct {
		name             string
		paths            []string
		items            []types.VerificationProofLedgerItem
		plan             *types.ChangePlan
		unavailable      bool
		capabilityFailed bool
		obligationFailed bool
		want             bool
	}{
		{name: "behavior execution-only", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}, want: true},
		{name: "placement execution-only", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("rendered_text_placement_contract", "value")}, want: true},
		{name: "declarations are not probe receipts", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}, plan: &types.ChangePlan{VerificationProbes: []types.VerificationProbe{{Language: "python", ContractRefs: []string{"value"}}}}, want: true},
		{name: "changed execution remains useful", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("changed_symbol", "")}},
		{name: "mixed debt retains useful execution", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value"), debt("changed_symbol", "")}},
		{name: "failed probe capability can still be corrected", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}, capabilityFailed: true},
		{name: "failed obligation is not missing assertion only", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}, obligationFailed: true},
		{name: "declared native assertion remains available", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}, plan: &types.ChangePlan{ProjectTestObservations: []types.ProjectTestObservation{observation}}},
		{name: "retained native assertion remains available", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}, plan: &types.ChangePlan{CumulativeVerificationScope: &types.CumulativeVerificationScope{ProjectTestObservations: []types.ProjectTestObservation{observation}}}},
		{name: "unrelated native assertion is not an escape", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "other")}, plan: &types.ChangePlan{ProjectTestObservations: []types.ProjectTestObservation{observation}}, want: true},
		{name: "native behavior declaration is not placement proof", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("rendered_text_placement_contract", "value")}, plan: &types.ChangePlan{ProjectTestObservations: []types.ProjectTestObservation{observation}}, want: true},
		{name: "javascript contract path stays open", paths: []string{"src/value.ts"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}},
		{name: "mixed runtime paths stay open", paths: []string{"pkg/value.py", "src/value.js"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}},
		{name: "runtime unavailable handled elsewhere", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}, unavailable: true},
		{name: "unknown debt is not classified", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("future_obligation", "value")}},
		{name: "missing identity is not classified", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "")}},
		{name: "unknown contract is not classified", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "unknown")}},
		{name: "source shape retains its separate witness path", paths: []string{"pkg/value.py"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}, plan: &types.ChangePlan{BehaviorContracts: []types.WriteBehaviorContract{{ID: "value", Kind: types.WriteBehaviorFileLayout, Required: true}}}},
		{name: "no source target", paths: []string{"README.md"}, items: []types.VerificationProofLedgerItem{debt("behavior_contract", "value")}},
		{name: "no debt", paths: []string{"pkg/value.py"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := tc.plan
			if plan == nil {
				plan = &types.ChangePlan{ID: "applied"}
			}
			if plan.BehaviorContracts == nil {
				plan.BehaviorContracts = []types.WriteBehaviorContract{{ID: "value", Kind: types.WriteBehaviorObservable, Required: true}, {ID: "other", Kind: types.WriteBehaviorObservable, Required: true}}
			}
			ledger := types.VerificationProofLedger{Obligations: tc.items}
			if tc.capabilityFailed {
				ledger.CapabilityFailedCount = 1
			}
			if tc.obligationFailed {
				ledger.FailedCount = 1
			}
			before, _ := json.Marshal([]any{plan, ledger})
			got := proofPlanningAssertionWitnessUnavailable(plan, ledger, tc.paths, func(string) bool { return !tc.unavailable })
			if got != tc.want {
				t.Errorf("suppressed=%v want %v", got, tc.want)
			}
			after, _ := json.Marshal([]any{plan, ledger})
			if string(before) != string(after) {
				t.Error("dispatch guard modified proof artifacts")
			}
		})
	}
}
