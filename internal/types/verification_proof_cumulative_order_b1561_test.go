package types

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Public projection regression: cumulative path closure and the existing
// non-authoritative comparator disposition must see the same final obligation
// state. No probe is made successful, and historical real failures are not
// superseded merely because another project suite passed.
func TestBuildVerificationProofLedgerCumulativeProbeDispositionB1561(t *testing.T) {
	for _, tc := range []struct {
		name          string
		mutate        func(*b1561CumulativeOrderFixture)
		wantAdvisory  bool
		wantOldFailed int
		wantPath      VerificationProofLedgerItemStatus
		wantState     VerificationProofLedgerState
		wantWeak      bool
	}{
		{name: "late_path_closure_keeps_old_real_failures", wantAdvisory: true, wantOldFailed: 2, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerFailed},
		{name: "isolated_late_path_closure_can_finish", mutate: (*b1561CumulativeOrderFixture).clearOldFailure, wantAdvisory: true, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerVerified},
		{name: "missing_scope", mutate: func(f *b1561CumulativeOrderFixture) { f.Plan.CumulativeVerificationScope = nil }, wantOldFailed: 2, wantPath: VerificationProofLedgerItemUnverified, wantState: VerificationProofLedgerFailed},
		{name: "wrong_source_plan", mutate: func(f *b1561CumulativeOrderFixture) {
			f.Plan.CumulativeVerificationScope.SourcePlanIDs = []string{"unrelated-plan"}
		}, wantOldFailed: 2, wantPath: VerificationProofLedgerItemUnverified, wantState: VerificationProofLedgerFailed},
		{name: "wrong_scoped_path", mutate: func(f *b1561CumulativeOrderFixture) {
			f.Plan.CumulativeVerificationScope.TargetPaths = []string{"other.py"}
		}, wantOldFailed: 2, wantPath: VerificationProofLedgerItemUnverified, wantState: VerificationProofLedgerFailed},
		{name: "wrong_primary_report_id", mutate: func(f *b1561CumulativeOrderFixture) { f.Report.PlanID = "unrelated-report" }, wantOldFailed: 2, wantPath: VerificationProofLedgerItemUnverified, wantState: VerificationProofLedgerFailed},
		{name: "missing_continuation", mutate: func(f *b1561CumulativeOrderFixture) {
			f.Report.ExecutedCommands = append(f.Report.ExecutedCommands[:1], f.Report.ExecutedCommands[2:]...)
		}, wantOldFailed: 2, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerFailed},
		{name: "wrong_continuation_reason", mutate: func(f *b1561CumulativeOrderFixture) { f.Report.ExecutedCommands[1].ReasonCode = "different_reason" }, wantOldFailed: 2, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerFailed},
		{name: "aggregate_without_concrete_assertion", mutate: func(f *b1561CumulativeOrderFixture) {
			f.Report.TestResults = []TestResult{{Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAggregate, Passed: true}}
		}, wantOldFailed: 2, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerFailed},
		{name: "unclosed_behavior_contract", mutate: func(f *b1561CumulativeOrderFixture) {
			f.Plan.BehaviorContracts = []WriteBehaviorContract{{ID: "required-value", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected, Subject: "core.value", Operator: WriteBehaviorOpEquals, Expected: "required result", Required: true}}
		}, wantOldFailed: 2, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerFailed},
		{name: "failed_primary_report", mutate: func(f *b1561CumulativeOrderFixture) {
			f.Report.Passed, f.Report.VerificationStatus = false, VerificationStatusFailed
			f.Report.FailureKind, f.Report.FailureReasonCode = FailureKindTestsFailed, "native_assertion_failed"
			f.Report.TestResults[0].Passed = false
			f.Report.ExecutedCommands[2].ExitCode = 1
		}, wantOldFailed: 2, wantPath: VerificationProofLedgerItemUnverified, wantState: VerificationProofLedgerFailed},
		{name: "failed_project_command_never_downgraded", mutate: func(f *b1561CumulativeOrderFixture) { f.Report.ExecutedCommands[2].ExitCode = 1 }, wantAdvisory: true, wantOldFailed: 2, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerFailed},
		{name: "unclassified_probe_source", mutate: func(f *b1561CumulativeOrderFixture) { f.Report.ExecutedCommands[0].Source = "another_execution_lane" }, wantOldFailed: 2, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerFailed},
		{name: "source_static_remains_weak_after_advisory", mutate: func(f *b1561CumulativeOrderFixture) {
			f.clearOldFailure()
			f.Report.ChangedPathCoverage[0].Caliber = ChangedPathVerificationSourceCheck
			f.Report.ChangedPathCoverage[0].Capability = VerificationCapabilitySourceStatic
		}, wantAdvisory: true, wantPath: VerificationProofLedgerItemCovered, wantState: VerificationProofLedgerLowConfidence, wantWeak: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, currentFirst := range []bool{false, true} {
				f := newB1561CumulativeOrderFixture()
				if tc.mutate != nil {
					tc.mutate(f)
				}
				// Use the durable JSON shape even for this small generic fixture.
				wire := b1561OrderJSON(t, f)
				var restored b1561CumulativeOrderFixture
				if err := json.Unmarshal(wire, &restored); err != nil {
					t.Fatal(err)
				}
				f = &restored
				artifacts := []VerificationProofArtifact{{Plan: f.OldPlan, Report: f.OldReport}, {Plan: f.Plan, Report: f.Report}}
				if currentFirst {
					artifacts[0], artifacts[1] = artifacts[1], artifacts[0]
				}
				ledger := BuildVerificationProofLedger(f.Plan, f.Report, artifacts)
				final := BuildWriteFinalReport(WriteFinalReportInput{Plan: f.Plan, Report: f.Report, ProofArtifacts: artifacts})
				if !bytes.Equal(b1561OrderJSON(t, ledger), b1561OrderJSON(t, final.ProofLedger)) {
					t.Errorf("currentFirst=%t: public final report did not reuse the same ledger projection", currentFirst)
				}
				if ledger.State != tc.wantState {
					t.Errorf("currentFirst=%t: state=%s want=%s current failed=%d", currentFirst, ledger.State, tc.wantState, ledger.CapabilityFailedCount)
				}
				if tc.wantWeak && (final.Proof.Status != VerificationProofWeak || final.ProofLedger.State == VerificationProofLedgerVerified) {
					t.Errorf("source-static proof was promoted: profile=%s ledger=%s", final.Proof.Status, final.ProofLedger.State)
				}
				if final.Verification.Passed != f.Report.Passed || final.Verification.TestCount != len(f.Report.TestResults) {
					t.Error("final display changed the original local report verdict or result count")
				}
				oldFailures, currentProbeRows, oldPathRows := 0, 0, 0
				for _, item := range final.ProofLedger.Capabilities {
					if item.ReportPlanID == f.OldReport.PlanID && item.Status == VerificationProofLedgerItemFailed {
						oldFailures++
					}
					if item.Kind == "executed_command" && item.ReportPlanID == f.Report.PlanID && item.Detail == f.Report.ExecutedCommands[0].Command {
						currentProbeRows++
						want := VerificationProofLedgerItemFailed
						if tc.wantAdvisory {
							want = VerificationProofLedgerItemAdvisory
						}
						if item.Status != want {
							t.Errorf("currentFirst=%t: retained current probe=%s want=%s; reason=%s", currentFirst, item.Status, want, item.ReasonCode)
						}
						if tc.wantAdvisory && item.ReasonCode != "non_authoritative_probe_superseded_by_project_pass" {
							t.Errorf("missing exact advisory disposition: %+v", item)
						}
					}
					if item.Kind == "executed_command" && item.ReportPlanID == f.Report.PlanID && item.Detail == "go test ./..." && f.Report.ExecutedCommands[len(f.Report.ExecutedCommands)-1].ExitCode != 0 && item.Status != VerificationProofLedgerItemFailed {
						t.Errorf("actual native command failure was hidden: %+v", item)
					}
				}
				for _, item := range final.ProofLedger.Obligations {
					if item.PlanID == f.OldPlan.ID && item.Kind == "changed_file" && item.Path == "core.py" {
						oldPathRows++
						if item.Status != tc.wantPath {
							t.Errorf("currentFirst=%t: old scoped path=%s want=%s", currentFirst, item.Status, tc.wantPath)
						}
						if tc.wantPath == VerificationProofLedgerItemCovered && item.ReasonCode != "resolved_by_terminal_cumulative_changed_path" {
							t.Errorf("path was not closed by the exact cumulative receipt: %+v", item)
						}
					}
				}
				if oldFailures != tc.wantOldFailed || currentProbeRows != 1 || oldPathRows != 1 {
					t.Errorf("currentFirst=%t: rows lost or manufactured: oldFailures=%d want=%d currentProbe=%d oldPath=%d", currentFirst, oldFailures, tc.wantOldFailed, currentProbeRows, oldPathRows)
				}
				if !bytes.Equal(wire, b1561OrderJSON(t, f)) {
					t.Error("public projections mutated raw plan/report/commands/observations")
				}
			}
		})
	}
}

type b1561CumulativeOrderFixture struct {
	OldPlan   *ChangePlan
	OldReport *ChangeReport
	Plan      *ChangePlan
	Report    *ChangeReport
}

func newB1561CumulativeOrderFixture() *b1561CumulativeOrderFixture {
	const oldID, currentID = "source-plan", "current-plan"
	return &b1561CumulativeOrderFixture{
		OldPlan: &ChangePlan{ID: oldID, ImpactAnalysis: &ImpactAnalysisResult{PlanID: oldID, VerificationTargets: []ImpactVerificationTarget{{
			ID: "retained-path", Kind: "changed_file", Path: "core.py", Source: "patch_effect", CoverageStatus: "unverified",
		}}}},
		OldReport: &ChangeReport{
			PlanID: oldID, VerificationStatus: VerificationStatusFailed, FailureKind: FailureKindTestsFailed, FailureReasonCode: "verification_probe_exception",
			TestResults:         []TestResult{{Kind: TestResultKindUnit, AssertionID: "model-check", Suite: "verification_probe/python", FailureDetail: "module initialization failed"}},
			ExecutedCommands:    []ExecutedCommand{{Runner: "verification_probe", Framework: "python", WorkingDir: ".", Command: "python -c <verification_probe:model-check>", Source: "pre_suite_verification_probe", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 1, ReasonCode: "verification_probe_exception"}},
			ChangedPathCoverage: []ChangedPathVerificationCoverage{{Path: "core.py", Status: ChangedPathVerificationUncovered}},
		},
		Plan: &ChangePlan{ID: currentID, CumulativeVerificationScope: &CumulativeVerificationScope{SourcePlanIDs: []string{oldID}, TargetPaths: []string{"core.py"}}},
		Report: &ChangeReport{
			PlanID: currentID, Passed: true, VerificationStatus: VerificationStatusPassed,
			TestResults: []TestResult{{Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAssertion, AssertionID: "TestNative", Suite: "example.com/project", Passed: true}},
			ExecutedCommands: []ExecutedCommand{
				{Runner: "verification_probe", Framework: "python", WorkingDir: ".", Command: "python -c <verification_probe:model-check>", Source: "pre_suite_verification_probe", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 1},
				{Runner: "go", WorkingDir: ".", Source: "probe_primary_suite_continued", Outcome: ExecutedCommandOutcomeSuiteContinued, ReasonCode: "probe_non_authoritative", ExitCode: 0},
				{Runner: "go", WorkingDir: ".", Command: "go test ./...", Source: "test_surface_default", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 0},
			},
			ChangedPathCoverage:     []ChangedPathVerificationCoverage{{Path: "core.py", LanguageFamilies: []VerificationLanguageFamily{"python"}, Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior}},
			VerificationDiagnostics: []VerificationDiagnostic{{Source: "pre_suite_verification_probe", Category: "probe_comparator_authority", ReasonCode: "model_authored_probe_comparator_unverified", Runner: "verification_probe", Outcome: "observed_failure", FailureObservations: []VerificationFailureObservation{{AssertionID: "model-check", Suite: "verification_probe/python", FailureDetail: "original comparator observation", OutputRef: "saved-output.txt"}}}},
		},
	}
}

func (f *b1561CumulativeOrderFixture) clearOldFailure() {
	f.OldReport.Passed, f.OldReport.VerificationStatus = true, VerificationStatusPassed
	f.OldReport.FailureKind, f.OldReport.FailureReasonCode = "", ""
	f.OldReport.TestResults, f.OldReport.ExecutedCommands = nil, nil
}

func b1561OrderJSON(t *testing.T, value any) []byte {
	t.Helper()
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}
