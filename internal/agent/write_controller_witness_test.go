package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_controller_witness_test.go — V5-1 (colleague_merge_audit §40.10): the
// controller's compact coverage row applies the contract-kind → witness-kind
// matrix; a source reading counts only for kinds that admit it.
func TestWriteControllerBehaviorContractCoverageIgnoresSourceWitnessForBehaviorKinds(t *testing.T) {
	plan := &types.ChangePlan{ID: "plan-witness", BehaviorContracts: []types.WriteBehaviorContract{
		{ID: "obs", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpContains, Expected: "0", Required: true},
		{ID: "layout", Kind: types.WriteBehaviorFileLayout, Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpEquals, Expected: "retries = 0;", Required: true},
	}}
	report := &types.ChangeReport{VerificationConfidence: []types.VerificationConfidenceRecord{{
		Source: "post_apply_source_observation", Category: "source_contract_refs", Status: "satisfied",
		ContractRefs: []string{"obs", "layout"}, WitnessKind: types.WriteBehaviorWitnessSourceText,
	}}}
	hard, covered, _ := writeControllerBehaviorContractCoverage(plan, report)
	if hard != 2 || covered != 1 {
		t.Fatalf("source witness must cover file_layout only: hard=%d covered=%d", hard, covered)
	}
	report.VerificationConfidence = append(report.VerificationConfidence, types.VerificationConfidenceRecord{
		Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied",
		ContractRefs: []string{"obs"}, WitnessKind: types.WriteBehaviorWitnessVerificationProbe,
	})
	if _, covered, _ = writeControllerBehaviorContractCoverage(plan, report); covered != 2 {
		t.Fatalf("an executed probe covers the observable contract: covered=%d", covered)
	}
}
