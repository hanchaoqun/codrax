package types

import (
	"bytes"
	"encoding/json"
	"testing"
)

// These are typed project-test receipts, not a Python plain probe or a claim
// that the test launches a native runner. The public runner/sync seam is pinned
// separately in the orchestrator test; this file exercises durable reloads and
// the exported profile/ledger consumers after that seam has lost its err value.
func b1575PairConflictFixture(id string) (*ChangePlan, *ChangeReport) {
	plan := &ChangePlan{
		ID: id, Status: PlanStatusApplied,
		BehaviorContracts: []WriteBehaviorContract{{
			ID: "value", Kind: WriteBehaviorObservable, Required: true,
			Polarity: WriteBehaviorPolarityExpected, Subject: "widget.VALUE",
			Operator: WriteBehaviorOpEquals, Expected: "41", Source: "write_analyzer",
		}},
		ProjectTestObservations: []ProjectTestObservation{{
			ID: "native-value", TestPath: "tests/test_value.py", AssertionSuite: "ValueTest",
			AssertionID: "test_value", ContractRefs: []string{"value"},
		}},
		ImpactAnalysis: &ImpactAnalysisResult{PlanID: id, VerificationTargets: []ImpactVerificationTarget{{
			ID: "value-target", Kind: "behavior_contract", ContractRef: "value", EvidenceRef: "value",
			CoverageStatus: "unverified", Source: "change_plan_slice",
		}}},
		PatchReview: &PatchReviewRecord{PlanID: id, Findings: []PatchReviewFinding{{
			Code: "behavior_contract_without_verify_coverage", Category: PatchReviewCategorySemanticCoverage,
			ImpactKind: PatchReviewImpactKindBehaviorContract, EvidenceRef: "value",
			Severity: PatchReviewSeverityWarning, CoverageStatus: PatchReviewCoverageUnverified,
		}}},
	}
	report := &ChangeReport{
		PlanID: id, Passed: true, VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{{
			Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAssertion,
			Suite: "tests.test_value.ValueTest", AssertionID: "test_value", Passed: true,
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner: "python", Framework: "unittest", Suite: "tests/test_value.py",
			Command: "python3 -m unittest tests.test_value -v", Source: "declared_coverage_test_surface",
			Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 0,
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source: "project_test_observation", Category: "project_test_contract_refs",
			WitnessKind: WriteBehaviorWitnessProjectTest, Status: "satisfied",
			ReasonCode: "project_test_contract_ref_observed", ContractRefs: []string{"value"},
		}},
	}
	return plan, report
}

func b1575PairConflictJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func b1575PairConflictReload(t *testing.T, plan *ChangePlan, report *ChangeReport) (*ChangePlan, *ChangeReport) {
	t.Helper()
	var loadedPlan ChangePlan
	var loadedReport ChangeReport
	if err := json.Unmarshal(b1575PairConflictJSON(t, plan), &loadedPlan); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b1575PairConflictJSON(t, report), &loadedReport); err != nil {
		t.Fatal(err)
	}
	return &loadedPlan, &loadedReport
}

func TestB1575BehaviorPairConflictPublicProfileAndLedger(t *testing.T) {
	for _, tc := range []struct {
		name         string
		planStatus   string
		reportFailed bool
		wantCovered  bool
		wantProfile  VerificationProofStatus
	}{
		{name: "persisted_external_error_and_passed_report", planStatus: PlanStatusVerifyFailed, wantProfile: VerificationProofWeak},
		{name: "ordinary_passed_pair", planStatus: PlanStatusApplied, wantCovered: true, wantProfile: VerificationProofStrong},
		{name: "failed_report_keeps_independent_passed_assertion", planStatus: PlanStatusVerifyFailed, reportFailed: true, wantCovered: true, wantProfile: VerificationProofFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, report := b1575PairConflictFixture("current")
			plan.Status = tc.planStatus
			if tc.reportFailed {
				report.Passed = false
				report.VerificationStatus = VerificationStatusFailed
				report.FailureKind = FailureKindTestsFailed
				report.TestResults = append(report.TestResults, TestResult{
					Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAssertion,
					Suite: "tests.test_value.ValueTest", AssertionID: "test_other", Passed: false,
					FailureDetail: "independent assertion failed",
				})
				report.ExecutedCommands[0].ExitCode = 1
			}
			original := b1575PairConflictJSON(t, []any{plan, report})
			plan, report = b1575PairConflictReload(t, plan, report)
			before := b1575PairConflictJSON(t, []any{plan, report})
			profile := BuildVerificationProofProfile(plan, report)
			ledger := BuildVerificationProofLedger(plan, report, nil)
			if profile.Status != tc.wantProfile || ledger.ProfileStatus != tc.wantProfile {
				t.Errorf("reloaded pair authority: profile=%s ledger_profile=%s want=%s reasons=%v", profile.Status, ledger.ProfileStatus, tc.wantProfile, profile.ReasonCodes)
			}
			seen := 0
			for _, item := range ledger.Obligations {
				if item.Kind != "behavior_contract" || (item.Source != "change_plan_slice" && item.Source != "patch_review") {
					continue
				}
				seen++
				if (item.Status == VerificationProofLedgerItemCovered) != tc.wantCovered {
					t.Errorf("reloaded derived item lost its pair authority: %+v wantCovered=%v", item, tc.wantCovered)
				}
			}
			if seen != 2 {
				t.Fatalf("expected both retained derived rows, got %d: %+v", seen, ledger.Obligations)
			}
			if tc.reportFailed && ledger.State != VerificationProofLedgerFailed {
				t.Errorf("independent positive observation erased overall native failure: %+v", ledger)
			}
			if !bytes.Equal(before, b1575PairConflictJSON(t, []any{plan, report})) || !bytes.Equal(original, before) {
				t.Error("read-only proof projection rewrote persisted plan/report bytes")
			}
			if report.VerificationConfidence[0].Status != "satisfied" || !report.TestResults[0].Passed {
				t.Error("underlying passed assertion was rewritten instead of projecting authority")
			}
		})
	}
}

func TestB1575ConflictingPairCannotBeCumulativeContractDonor(t *testing.T) {
	debtPlan, debtReport := b1575PairConflictFixture("historical-debt")
	debtReport.VerificationConfidence = nil
	donorPlan, donorReport := b1575PairConflictFixture("conflicting-donor")
	donorPlan.Status = PlanStatusVerifyFailed
	debtPlan, debtReport = b1575PairConflictReload(t, debtPlan, debtReport)
	donorPlan, donorReport = b1575PairConflictReload(t, donorPlan, donorReport)
	before := b1575PairConflictJSON(t, []any{debtPlan, debtReport, donorPlan, donorReport})
	for _, donorPrimary := range []bool{false, true} {
		primaryPlan, primaryReport := debtPlan, debtReport
		history := []VerificationProofArtifact{{Plan: donorPlan, Report: donorReport}}
		if donorPrimary {
			primaryPlan, primaryReport = donorPlan, donorReport
			history = []VerificationProofArtifact{{Plan: debtPlan, Report: debtReport}}
		}
		ledger := BuildVerificationProofLedger(primaryPlan, primaryReport, history)
		seenRequired, seenDerived := 0, 0
		for _, item := range ledger.Obligations {
			if item.Kind != "behavior_contract" || item.PlanID != debtPlan.ID {
				continue
			}
			switch item.Source {
			case "change_plan_behavior_contract":
				seenRequired++
			case "change_plan_slice", "patch_review":
				seenDerived++
			default:
				continue
			}
			if item.Status == VerificationProofLedgerItemCovered {
				t.Errorf("conflicting report pair signed another plan's contract donorPrimary=%v: %+v", donorPrimary, item)
			}
		}
		if seenRequired != 1 || seenDerived != 2 {
			t.Errorf("historical obligations disappeared donorPrimary=%v: required=%d derived=%d", donorPrimary, seenRequired, seenDerived)
		}
		if ledger.State == VerificationProofLedgerVerified || ledger.ProfileStatus == VerificationProofStrong {
			t.Errorf("conflicting cumulative pair published complete proof donorPrimary=%v: %+v", donorPrimary, ledger)
		}
	}
	if !bytes.Equal(before, b1575PairConflictJSON(t, []any{debtPlan, debtReport, donorPlan, donorReport})) {
		t.Error("cumulative projection rewrote raw artifact evidence")
	}
}

func TestB1575PairConflictWithoutDerivedRowsKeepsOverallBoundary(t *testing.T) {
	for _, conflicting := range []bool{false, true} {
		for _, donorPrimary := range []bool{false, true} {
			debtPlan, debtReport := b1575PairConflictFixture("plain-contract-plan")
			debtPlan.ImpactAnalysis, debtPlan.PatchReview = nil, nil
			debtReport.VerificationConfidence = nil
			donorPlan, donorReport := b1575PairConflictFixture("plain-contract-donor")
			donorPlan.ImpactAnalysis, donorPlan.PatchReview = nil, nil
			if conflicting {
				donorPlan.Status = PlanStatusVerifyFailed
			}
			debtPlan, debtReport = b1575PairConflictReload(t, debtPlan, debtReport)
			donorPlan, donorReport = b1575PairConflictReload(t, donorPlan, donorReport)
			before := b1575PairConflictJSON(t, []any{debtPlan, debtReport, donorPlan, donorReport})
			primaryPlan, primaryReport := debtPlan, debtReport
			history := []VerificationProofArtifact{{Plan: donorPlan, Report: donorReport}}
			if donorPrimary {
				primaryPlan, primaryReport = donorPlan, donorReport
				history = []VerificationProofArtifact{{Plan: debtPlan, Report: debtReport}}
			}
			profile := BuildCumulativeVerificationProofProfile(primaryPlan, primaryReport, history)
			ledger := BuildVerificationProofLedger(primaryPlan, primaryReport, history)
			// The native assertion is still an observed partial result. Do not
			// require erasing it or forcing every contract row to missing; the
			// independently failed verifier attempt must prevent a full green.
			if conflicting {
				if profile.Status == VerificationProofStrong || ledger.State == VerificationProofLedgerVerified {
					t.Errorf("no-derived conflict became overall verified donorPrimary=%v: profile=%+v ledger=%+v", donorPrimary, profile, ledger)
				}
			} else if profile.Status != VerificationProofStrong || ledger.State != VerificationProofLedgerVerified {
				t.Errorf("ordinary exact native cumulative proof was lost donorPrimary=%v: profile=%+v ledger=%+v", donorPrimary, profile, ledger)
			}
			if !bytes.Equal(before, b1575PairConflictJSON(t, []any{debtPlan, debtReport, donorPlan, donorReport})) || !donorReport.TestResults[0].Passed || donorReport.VerificationConfidence[0].Status != "satisfied" {
				t.Error("overall conflict projection rewrote the original partial assertion")
			}
		}
	}
}
