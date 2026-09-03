package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_controller_scheduler_witness_test.go — V5-1 (§40.10 / §40.35 复核):
// the verify coverage projection applies the contract-kind → witness-kind
// matrix when the plan is known — a legacy unstamped source_contract_refs
// receipt covers a file_layout contract but not an observable one — and a
// disclosed source absence for a runtime kind keeps the ref unverified.
func TestVerifyCoverageConfidenceForPlanAppliesWitnessMatrix(t *testing.T) {
	plan := &types.ChangePlan{ID: "plan-projection", BehaviorContracts: []types.WriteBehaviorContract{
		{ID: "obs", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpContains, Expected: "0", Required: true},
		{ID: "layout", Kind: types.WriteBehaviorFileLayout, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals, Expected: "retries = 0;", Required: true},
	}}
	report := &types.ChangeReport{VerificationConfidence: []types.VerificationConfidenceRecord{
		{Source: "post_apply_source_observation", Category: "source_contract_refs", Status: "satisfied", ContractRefs: []string{"obs", "layout"}},
	}}
	legacy := verifyCoverageConfidenceFromReport(report)
	if !legacy.CoveredContracts["obs"] || !legacy.CoveredContracts["layout"] {
		t.Fatalf("records-only projection counts by category (pinned as the legacy shape): %+v", legacy.CoveredContracts)
	}
	conf := verifyCoverageConfidenceFromReportForPlan(plan, report)
	if conf.CoveredContracts["obs"] || !conf.CoveredContracts["layout"] {
		t.Fatalf("matrix must reject the source witness for the observable contract only: %+v", conf.CoveredContracts)
	}
	report.VerificationConfidence = append(report.VerificationConfidence,
		types.VerificationConfidenceRecord{Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied", ContractRefs: []string{"obs"}, WitnessKind: types.WriteBehaviorWitnessVerificationProbe})
	if conf = verifyCoverageConfidenceFromReportForPlan(plan, report); !conf.CoveredContracts["obs"] {
		t.Fatalf("an executed probe covers the observable contract: %+v", conf.CoveredContracts)
	}
	absent := &types.ChangeReport{VerificationConfidence: []types.VerificationConfidenceRecord{
		{Source: "post_apply_source_observation", Category: "source_text_presence", Status: "advisory", ReasonCode: "post_apply_source_text_absent", ContractRefs: []string{"obs"}, WitnessKind: types.WriteBehaviorWitnessSourceText},
		{Source: "post_apply_source_observation", Category: "source_text_presence", Status: "advisory", ReasonCode: "post_apply_source_text_present", ContractRefs: []string{"layout"}, WitnessKind: types.WriteBehaviorWitnessSourceText},
	}}
	conf = verifyCoverageConfidenceFromReportForPlan(plan, absent)
	if !conf.MissingContracts["obs"] || conf.CoveredContracts["obs"] || conf.MissingContracts["layout"] || conf.CoveredContracts["layout"] {
		t.Fatalf("disclosed absence keeps the runtime-kind ref unverified; presence projects nothing: %+v", conf)
	}
}
