package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1575ControllerActualPromptUsesEffectiveProbeAuthority(t *testing.T) {
	for _, lane := range []string{"python", "native", "source_text", "mixed"} {
		t.Run(lane, func(t *testing.T) {
			plan := &types.ChangePlan{ID: "applied", Status: types.PlanStatusApplied}
			report := &types.ChangeReport{
				PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true,
				VerificationStatus: types.VerificationStatusPassed,
				TestResults:        []types.TestResult{{AssertionID: "plain", Suite: "verification_probe/python", Passed: true}},
				ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
					Path: "pkg/client.py", Status: types.ChangedPathVerificationCovered,
					Caliber: types.ChangedPathVerificationProbe, Capability: types.VerificationCapabilityTargetBehavior,
					Runner: "verification_probe", Source: "plain", LanguageFamilies: []types.VerificationLanguageFamily{types.VerificationLanguagePython},
				}},
			}
			add := func(id string, kind types.WriteBehaviorContractKind, witness types.WriteBehaviorWitnessKind, category string) {
				plan.BehaviorContracts = append(plan.BehaviorContracts, types.WriteBehaviorContract{
					ID: id, Kind: kind, Polarity: types.WriteBehaviorPolarityExpected,
					Operator: types.WriteBehaviorOpEquals, Expected: "42", Required: true, Source: "write_analyzer",
				})
				report.VerificationConfidence = append(report.VerificationConfidence, types.VerificationConfidenceRecord{
					Source: "verifier", Category: category, Status: "satisfied", Severity: "info",
					WitnessKind: witness, ContractRefs: []string{id},
				})
			}
			covered := 0
			if lane == "python" || lane == "mixed" {
				add("runtime-plain", types.WriteBehaviorObservable, types.WriteBehaviorWitnessVerificationProbe, "probe_contract_refs")
			}
			if lane == "native" || lane == "mixed" {
				add("runtime-native", types.WriteBehaviorObservable, types.WriteBehaviorWitnessProjectTest, "project_test_contract_refs")
				covered++
			}
			if lane == "source_text" || lane == "mixed" {
				add("layout", types.WriteBehaviorFileLayout, types.WriteBehaviorWitnessSourceText, "source_contract_refs")
				covered++
			}
			mu := types.NewMutableState("controller scope projection")
			mu.SetChangePlan(plan)
			mu.SetChangeReport(report)
			before, err := json.Marshal([]any{plan, report})
			if err != nil {
				t.Fatal(err)
			}
			ctx := &types.AgentContext{Mutable: mu, Mode: types.ModeApply}
			got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{
				fmt.Sprintf("required_typed_contracts=%d covered_required_typed_contracts=%d", len(plan.BehaviorContracts), covered),
				"changed_path_verification: path=pkg/client.py status=uncovered caliber=verification_probe capability=unknown",
				"verification_evidence: status=passed passed_results=1 failed_results=0 total_results=1",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("actual controller prompt missing %q:\n%s", want, got)
				}
			}
			// Direct consumers of this compact counter use the same projection,
			// even when the selected report was not projected by prompt rendering.
			if hard, count, _ := writeControllerBehaviorContractCoverage(plan, report); hard != len(plan.BehaviorContracts) || count != covered {
				t.Errorf("direct counter hard=%d covered=%d; want %d/%d", hard, count, len(plan.BehaviorContracts), covered)
			}
			if next := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil); next != got {
				t.Error("effective prompt is not replay-stable")
			}
			after, err := json.Marshal([]any{plan, report})
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rendering changed original report/plan or process pass")
			}
		})
	}
}
