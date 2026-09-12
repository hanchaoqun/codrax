package types

import (
	"bytes"
	"encoding/json"
	"testing"
)

// The persisted legacy reason and today's assertion-scoped reason express the
// same missing contract-ref debt. Closing it requires exact admitted receipts,
// not merely a successful command or the existence of a verification probe.
func TestVerificationProofProjectReasonExactReceiptResolution(t *testing.T) {
	for _, reason := range []string{"project_test_assertion_not_observed", "project_test_observation_not_executed"} {
		for _, cumulative := range []bool{false, true} {
			for _, tc := range []struct {
				name    string
				status  string
				refs    []string
				witness WriteBehaviorWitnessKind
				closed  bool
			}{
				{name: "all exact refs", status: "satisfied", refs: []string{"value", "boundary"}, witness: WriteBehaviorWitnessVerificationProbe, closed: true},
				{name: "legacy witness category", status: "satisfied", refs: []string{"value", "boundary"}, closed: true},
				{name: "partial refs", status: "satisfied", refs: []string{"value"}, witness: WriteBehaviorWitnessVerificationProbe},
				{name: "different refs", status: "satisfied", refs: []string{"other"}, witness: WriteBehaviorWitnessVerificationProbe},
				{name: "no refs", status: "satisfied", witness: WriteBehaviorWitnessVerificationProbe},
				{name: "failed witness", status: "failed", refs: []string{"value", "boundary"}, witness: WriteBehaviorWitnessVerificationProbe},
				{name: "unavailable witness", status: "unavailable", refs: []string{"value", "boundary"}, witness: WriteBehaviorWitnessVerificationProbe},
				{name: "advisory witness", status: "advisory", refs: []string{"value", "boundary"}, witness: WriteBehaviorWitnessVerificationProbe},
				{name: "source text cannot prove runtime", status: "satisfied", refs: []string{"value", "boundary"}, witness: WriteBehaviorWitnessSourceText},
			} {
				lane := "same report"
				if cumulative {
					lane = "cumulative reports"
				}
				t.Run(reason+"/"+lane+"/"+tc.name, func(t *testing.T) {
					plan, report := verificationProjectReasonFixture(reason)
					// B1575: exact-ref reconciliation uses a native assertion
					// witness. Plain Python refs remain an explicit negative in
					// the target-execution/granularity tests.
					installNative := func(r *ChangeReport) {
						r.ExecutedCommands = []ExecutedCommand{{Runner: "pytest", Framework: "pytest", Suite: "tests/test_values.py", Outcome: ExecutedCommandOutcomeExecuted, Source: "declared_coverage_test_surface"}}
						r.TestResults = []TestResult{{AssertionID: "test_values", Suite: "tests/test_values.py", ObservationScope: TestObservationScopeAssertion, Passed: tc.status == "satisfied"}}
					}
					installNative(report)
					kind := tc.witness
					if kind == WriteBehaviorWitnessVerificationProbe {
						kind = WriteBehaviorWitnessProjectTest
					}
					witness := VerificationConfidenceRecord{
						Source: "project_test_observation", Category: "project_test_contract_refs", Status: tc.status,
						ReasonCode: "project_test_contract_ref_observed", ContractRefs: tc.refs, WitnessKind: kind,
					}
					if tc.witness == WriteBehaviorWitnessSourceText {
						witness.Source, witness.Category = "post_apply_source_observation", "source_contract_refs"
					}
					var artifacts []VerificationProofArtifact
					if cumulative {
						otherPlan, otherReport := verificationProjectReasonFixture(reason)
						installNative(otherReport)
						otherPlan.ID, otherReport.PlanID = "plan-related", "plan-related"
						otherReport.VerificationConfidence = []VerificationConfidenceRecord{witness}
						artifacts = []VerificationProofArtifact{{Plan: otherPlan, Report: otherReport}}
					} else {
						report.VerificationConfidence = append(report.VerificationConfidence, witness)
					}
					before, err := json.Marshal(report)
					if err != nil {
						t.Fatal(err)
					}
					profile := BuildCumulativeVerificationProofProfile(plan, report, artifacts)
					ledger := BuildVerificationProofLedger(plan, report, artifacts)
					if tc.closed {
						if profile.Status != VerificationProofStrong || verificationProofHasReason(profile, reason) {
							t.Fatalf("exact admitted receipts left stale project debt: %+v", profile)
						}
						if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 || ledger.UnavailableCount != 0 || ledger.FailedCount != 0 {
							t.Fatalf("profile and closed ledger disagree: %+v", ledger)
						}
					} else if profile.Status != VerificationProofWeak || !verificationProofHasReason(profile, reason) {
						t.Fatalf("missing exact/admitted receipts must remain weak: %+v", profile)
					}
					if !verificationProofHasReason(VerificationProofProfile{ReasonCodes: profile.ConfidenceReasonCodes}, reason) {
						t.Fatalf("resolution must retain original confidence history: %+v", profile)
					}
					after, err := json.Marshal(report)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(before, after) {
						t.Fatal("proof projection mutated the original report")
					}
				})
			}
		}
	}
}

func TestVerificationProofProjectReasonDoesNotDischargeUnknownWarning(t *testing.T) {
	const reason = "project_test_other_warning"
	plan, report := verificationProjectReasonFixture(reason)
	report.VerificationConfidence = append(report.VerificationConfidence, VerificationConfidenceRecord{
		Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied",
		ReasonCode: "verification_probe_contract_ref_covered", ContractRefs: []string{"value", "boundary"}, WitnessKind: WriteBehaviorWitnessVerificationProbe,
	})
	profile := BuildVerificationProofProfile(plan, report)
	if profile.Status != VerificationProofWeak || !verificationProofHasReason(profile, reason) {
		t.Fatalf("exact receipts cannot discharge an unknown warning class: %+v", profile)
	}
}

func TestVerificationProofProjectReasonPreservesTerminalFailure(t *testing.T) {
	for _, cumulative := range []bool{false, true} {
		for _, status := range []VerificationStatus{VerificationStatusFailed, VerificationStatusUnavailable} {
			t.Run(string(status)+map[bool]string{false: "/same", true: "/cumulative"}[cumulative], func(t *testing.T) {
				plan, report := verificationProjectReasonFixture("project_test_assertion_not_observed")
				report.Passed, report.VerificationStatus = false, status
				report.FailureKind = FailureKindTestsFailed
				if status == VerificationStatusUnavailable {
					report.FailureKind = FailureKindRunnerMissing
				}
				witness := VerificationConfidenceRecord{
					Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied",
					ReasonCode: "verification_probe_contract_ref_covered", ContractRefs: []string{"value", "boundary"}, WitnessKind: WriteBehaviorWitnessVerificationProbe,
				}
				var artifacts []VerificationProofArtifact
				if cumulative {
					otherPlan, otherReport := verificationProjectReasonFixture("project_test_assertion_not_observed")
					otherPlan.ID, otherReport.PlanID = "plan-related", "plan-related"
					otherReport.VerificationConfidence = []VerificationConfidenceRecord{witness}
					artifacts = []VerificationProofArtifact{{Plan: otherPlan, Report: otherReport}}
				} else {
					report.VerificationConfidence = append(report.VerificationConfidence, witness)
				}
				profile := BuildCumulativeVerificationProofProfile(plan, report, artifacts)
				want := VerificationProofFailed
				if status == VerificationStatusUnavailable {
					want = VerificationProofUnavailable
				}
				if profile.Status != want || profile.VerificationStatus != status {
					t.Fatalf("receipt resolution overrode failed/unavailable terminal report: %+v", profile)
				}
			})
		}
	}
}

func verificationProjectReasonFixture(reason string) (*ChangePlan, *ChangeReport) {
	plan := &ChangePlan{ID: "plan-project-reason"}
	for _, id := range []string{"value", "boundary"} {
		plan.BehaviorContracts = append(plan.BehaviorContracts, WriteBehaviorContract{
			ID: id, Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpSatisfies, Expected: "runtime behavior", Required: true,
		})
	}
	report := &ChangeReport{
		PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{Runner: "verification_probe", Suite: "verification_probe/python", Outcome: ExecutedCommandOutcomeExecuted}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source: "project_test_observation", Category: "project_test_contract_refs", Status: "missing", Severity: "warning",
			ReasonCode: reason, ContractRefs: []string{"value", "boundary"},
		}},
	}
	return plan, report
}
