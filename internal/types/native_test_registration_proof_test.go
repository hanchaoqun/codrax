package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Consumer protocol fixtures; real dispatch/runner receipts are tested at the
// tool/controller seam. This fixture never purports to execute a native test.
func nativeRegistrationProofFixture(t *testing.T) (*ChangePlan, *ChangeReport) {
	t.Helper()
	plan, report := existingIntentConsumerFixture()
	report.VerificationConfidence = nil
	plan.WriteAnalysisIR = nil
	plan.Status, plan.PersistenceKind = PlanStatusNoChangeRequired, PlanPersistenceNativeTestRegistration
	plan.AppliedCommitSHA, plan.PatchEffect = "", nil
	plan.TargetPaths = []string{"packages/widget/value.py"}
	delivery := VerificationDeliverySnapshot{SourcePlanID: "source-plan", AppliedCommitSHA: strings.Repeat("a", 40), PatchEffect: &PatchEffectRecord{
		PlanID: "source-plan", RecordID: "source-effect", Source: "applied_commit", HeadRef: strings.Repeat("a", 40), DiffFingerprint: strings.Repeat("b", 64),
	}}
	plan.CumulativeVerificationScope = &CumulativeVerificationScope{SourcePlanIDs: []string{delivery.SourcePlanID}, AppliedSources: []VerificationDeliverySnapshot{delivery}, TargetPaths: append([]string(nil), plan.TargetPaths...)}
	plan.BehaviorContracts = []WriteBehaviorContract{{ID: "value", Kind: WriteBehaviorObservable, Required: true, Polarity: WriteBehaviorPolarityExpected, Subject: "widget.VALUE", Operator: WriteBehaviorOpEquals, Expected: "41", Source: "write_analyzer"}}
	row := &report.TestResults[0]
	row.InvocationID = "native-current"
	report.ExecutedCommands[0].InvocationID = row.InvocationID
	plan.ProjectTestObservations = []ProjectTestObservation{{ID: "value-observation", TestPath: report.ExistingTestExecutions[0].TestPath, AssertionSuite: row.Suite, AssertionID: row.AssertionID, ContractRefs: []string{"value"}}}
	plan.NativeTestRegistration = &NativeTestRegistration{Version: 1, PlanID: plan.ID, AuthorizationID: "authorization", RunID: "run", BatchID: "batch", RepositoryRoot: "/repo", Delivery: delivery,
		Tests: []NativeTestFileVersion{{Path: plan.ProjectTestObservations[0].TestPath, SHA256: report.ExistingTestExecutions[0].TestFileSHA256}}}
	sealNativeTestRegistration(plan)
	receipt := &report.ExistingTestExecutions[0]
	receipt.SourcePlanID, receipt.AppliedCommitSHA = delivery.SourcePlanID, delivery.AppliedCommitSHA
	receipt.PatchEffectID, receipt.DiffFingerprint = delivery.PatchEffect.RecordID, delivery.PatchEffect.DiffFingerprint
	receipt.NativeTestRegistrationDigest = NativeTestRegistrationDigest(plan)
	if receipt.NativeTestRegistrationDigest == "" {
		t.Fatal("registration fixture did not seal")
	}
	report.VerificationConfidence = append(report.VerificationConfidence, VerificationConfidenceRecord{Source: "project_test_observation", Category: "project_test_contract_refs", Status: "satisfied", WitnessKind: WriteBehaviorWitnessProjectTest, ContractRefs: []string{"value"}, ReasonCode: "project_test_contract_ref_observed"})
	return plan, report
}

func nativeRegistrationProofBinding(p *ChangePlan) ProjectTestFailureBinding {
	o := p.ProjectTestObservations[0]
	return ProjectTestFailureBinding{ObservationID: o.ID, TestPath: o.TestPath, AssertionSuite: o.AssertionSuite, AssertionID: o.AssertionID, ResultIndex: 0}
}

func nativeRegistrationProofFail(r *ChangeReport) {
	r.Passed, r.VerificationStatus, r.FailureKind = false, VerificationStatusFailed, FailureKindTestsFailed
	r.ExecutedCommands[0].ExitCode = 1
	r.TestResults[0].Passed = false
	r.ExistingTestExecutions[0].FailedAssertionCount = 1
	r.ExistingTestExecutions[0].AssertionDigests[0] = ExistingTestAssertionDigest(r.TestResults[0])
	// A stored label is not used to decide the failure's contract relevance.
	r.VerificationConfidence[0].Status = "failed"
}

func TestNativeRegistrationProofPublicConsumers(t *testing.T) {
	for _, failed := range []bool{false, true} {
		p, r := nativeRegistrationProofFixture(t)
		if failed {
			nativeRegistrationProofFail(r)
		}
		p, r = b1575PairConflictReload(t, p, r)
		before, _ := json.Marshal([]any{p, r})
		if !NativeTestRegistrationAssertionMatches(p, r, p.ProjectTestObservations[0], 0, 0) {
			t.Fatalf("valid file-bound native assertion rejected (failed=%v)", failed)
		}
		if got := BehaviorContractRefHasVerificationWitness(p, r, "value"); got == failed {
			t.Fatalf("success authority=%v failed=%v", got, failed)
		}
		failure := BuildVerifyFailureContractRelevance(r, p, nativeRegistrationProofBinding(p))
		if (len(failure.Hits) == 1) != failed {
			t.Fatalf("failure authority=%+v failed=%v", failure, failed)
		}
		if len(ExistingTestExecutionConfidence(p, r)) != 0 || len(RequiredExistingTestPaths(p)) != 0 {
			t.Fatal("registration invented a user execution requirement")
		}
		ledger := BuildVerificationProofLedger(p, r, nil)
		if !failed && ledger.State != VerificationProofLedgerVerified {
			t.Fatalf("fresh receipt did not close exact contract: %+v", ledger)
		}
		if failed && ledger.State != VerificationProofLedgerFailed {
			t.Fatalf("native failure erased: %+v", ledger)
		}
		after, _ := json.Marshal([]any{p, r})
		if string(before) != string(after) {
			t.Fatal("consumer rewrote historical evidence")
		}
	}
}

func TestNativeRegistrationProofRejectsStaleAndUnboundReceipts(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangePlan, *ChangeReport)
	}{
		{"old_pass_or_failure", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions = nil }},
		{"registration_removed", func(p *ChangePlan, _ *ChangeReport) { p.NativeTestRegistration = nil }},
		{"borrowed_by_ordinary_plan", func(p *ChangePlan, _ *ChangeReport) { p.NativeTestRegistration = nil; p.PersistenceKind = "" }},
		{"contract_body", func(p *ChangePlan, _ *ChangeReport) { p.BehaviorContracts[0].Expected = "42" }},
		{"analysis_body", func(p *ChangePlan, _ *ChangeReport) {
			p.WriteAnalysisIR = &WriteAnalysisIR{Request: WriteRequestModel{Constraints: []WriteConstraint{{Kind: WriteConstraintRunExistingTest, Target: "other.py"}}}}
		}},
		{"new_registration", func(p *ChangePlan, _ *ChangeReport) {
			p.NativeTestRegistration.AuthorizationID = "new"
			sealNativeTestRegistration(p)
		}},
		{"test_version", func(p *ChangePlan, _ *ChangeReport) {
			p.NativeTestRegistration.Tests[0].SHA256 = strings.Repeat("c", 64)
			sealNativeTestRegistration(p)
		}},
		{"observation_mapping", func(p *ChangePlan, _ *ChangeReport) {
			p.ProjectTestObservations[0].ID = "new-observation"
			sealNativeTestRegistration(p)
		}},
		{"receipt_digest", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].NativeTestRegistrationDigest = "old" }},
		{"receipt_sha", func(_ *ChangePlan, r *ChangeReport) {
			r.ExistingTestExecutions[0].TestFileSHA256 = strings.Repeat("c", 64)
		}},
		{"receipt_source", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].SourcePlanID = "other" }},
		{"receipt_commit", func(_ *ChangePlan, r *ChangeReport) {
			r.ExistingTestExecutions[0].AppliedCommitSHA = strings.Repeat("d", 40)
		}},
		{"report_plan", func(_ *ChangePlan, r *ChangeReport) { r.PlanID = "other" }},
		{"legacy_invocation", func(_ *ChangePlan, r *ChangeReport) {
			r.TestResults[0].InvocationID = ""
			r.ExecutedCommands[0].InvocationID = ""
		}},
		{"different_invocation", func(_ *ChangePlan, r *ChangeReport) { r.TestResults[0].InvocationID = "other" }},
		{"duplicate_invocation", func(_ *ChangePlan, r *ChangeReport) {
			r.ExecutedCommands = append(r.ExecutedCommands, r.ExecutedCommands[0])
		}},
		{"aggregate", func(_ *ChangePlan, r *ChangeReport) {
			r.TestResults[0].ObservationScope = TestObservationScopeAggregate
		}},
		{"skipped", func(_ *ChangePlan, r *ChangeReport) {
			r.TestResults[0].ObservationScope = TestObservationScopeNonAsserting
		}},
		{"zero_assertions", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].AssertionCount = 0 }},
		{"different_suite", func(_ *ChangePlan, r *ChangeReport) { r.TestResults[0].Suite = "OtherTest" }},
		{"different_assertion", func(_ *ChangePlan, r *ChangeReport) { r.TestResults[0].AssertionID = "test_other" }},
		{"different_file", func(_ *ChangePlan, r *ChangeReport) {
			r.ExistingTestExecutions[0].TestPath = "packages/widget/tests/test_other.py"
		}},
	} {
		for _, failed := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/pass", true: "/fail"}[failed], func(t *testing.T) {
				p, r := nativeRegistrationProofFixture(t)
				if failed {
					nativeRegistrationProofFail(r)
				}
				tc.edit(p, r)
				before, _ := json.Marshal([]any{p, r})
				if NativeTestRegistrationAssertionMatches(p, r, p.ProjectTestObservations[0], 0, 0) || BehaviorContractRefHasVerificationWitness(p, r, "value") {
					t.Fatal("stale/unbound assertion granted new contract authority")
				}
				if got := BuildVerifyFailureContractRelevance(r, p, nativeRegistrationProofBinding(p)); len(got.Hits) != 0 {
					t.Fatalf("stale failed row can retire contract: %+v", got)
				}
				ledger := BuildVerificationProofLedger(p, r, nil)
				if ledger.State == VerificationProofLedgerVerified {
					t.Fatal("stale receipt published verified ledger")
				}
				after, _ := json.Marshal([]any{p, r})
				if string(before) != string(after) {
					t.Fatal("projection mutated history")
				}
			})
		}
	}
}

func TestNativeRegistrationProofPreservesOrdinaryPlan(t *testing.T) {
	p, r := b1575PairConflictFixture("ordinary")
	before := append([]VerificationConfidenceRecord(nil), r.VerificationConfidence...)
	if !NativeTestRegistrationAssertionMatches(p, r, p.ProjectTestObservations[0], 0, 0) ||
		!reflect.DeepEqual(EffectiveVerificationConfidence(p, r), before) || !BehaviorContractRefHasVerificationWitness(p, r, "value") {
		t.Fatal("ordinary source/test plan protocol changed")
	}
}
