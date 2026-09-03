package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_fold_in5_test.go — fold-in round five of V5-2
// (colleague_merge_audit §40.36 五轮收编, finding BB): the main-snapshot
// baseline row of an expects_baseline_failure probe carries its OWN reason
// lane. The round-four producer copied the inner probe row and only
// overwrote ReasonCode when it was empty, so a baseline_unavailable row kept
// e.g. verification_probe_module_not_found — a member of the
// verification-unavailable reason set — and the ReasonCode pre-check of
// executedCommandUnavailableReasonCode (consulted BEFORE the outcome
// switch) turned the evidence row into an unavailable verification_probe
// capability on a PASSED report and tainted
// ChangeReport.VerificationUnavailableReasonCode(). The round-four pin was
// written against ReasonCode "" (a hand-built row), not the real producer.
//
// This pin drives the REAL producer: RunTests.Execute with a python probe
// whose main snapshot lacks the imported module.

func TestRunTestsBaselineRowKeepsItsOwnReasonLaneWhenMainSnapshotLacksTheModule(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	mainRoot := t.TempDir()   // no widget.py: the probe's import fails on main
	activeRoot := t.TempDir() // the change adds widget.py
	if err := os.WriteFile(filepath.Join(activeRoot, "widget.py"), []byte("VALUE = 2\n"), 0o644); err != nil {
		t.Fatalf("write active source: %v", err)
	}
	mu := types.NewMutableState("probe baseline module missing on main")
	plan := &types.ChangePlan{
		ID:          "plan-probe-baseline-module-missing",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py"},
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "widget-value",
			Kind:     types.WriteBehaviorObservable,
			Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpEquals,
			Expected: "2",
			Required: true,
			Source:   "write_analyzer",
		}},
		VerificationProbes: []types.VerificationProbe{{
			ID:                     "value_contract",
			Language:               "python",
			Code:                   "import widget\nassert widget.VALUE == 2\n",
			ContractRefs:           []string{"widget-value"},
			ChangedSymbolRefs:      []string{"path:widget.py", "widget.VALUE"},
			ExpectsBaselineFailure: true,
		}},
	}
	mu.SetChangePlan(plan)
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      activeRoot,
		MainRepoRoot:  mainRoot,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner":    "python",
		"framework": "unittest",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if !result.Success || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		t.Fatalf("the probe passes after the change; want a passed report, got success=%v status=%s report=%+v", result.Success, report.NormalizeVerificationStatus(), report)
	}
	var baseline *types.ExecutedCommand
	for i := range report.ExecutedCommands {
		cmd := &report.ExecutedCommands[i]
		if strings.TrimSpace(cmd.Source) == verificationProbeBaselineSource {
			baseline = cmd
		}
	}
	if baseline == nil {
		t.Fatalf("typed main-snapshot baseline command missing: %+v", report.ExecutedCommands)
	}
	// The real producer row: the main snapshot could not be probed (module
	// missing) — Outcome baseline_unavailable, ReasonCode on the baseline
	// family lane, the inner probe reason kept as a typed detail.
	if baseline.Outcome != types.ExecutedCommandOutcomeBaselineUnavailable {
		t.Fatalf("baseline outcome = %q, want %q: %+v", baseline.Outcome, types.ExecutedCommandOutcomeBaselineUnavailable, *baseline)
	}
	if baseline.ReasonCode != verificationProbeBaselineUnavailableReasonCode {
		t.Fatalf("baseline ReasonCode = %q, want the baseline family code %q (the inner probe reason belongs to BaselineProbeReasonCode): %+v", baseline.ReasonCode, verificationProbeBaselineUnavailableReasonCode, *baseline)
	}
	if baseline.BaselineProbeReasonCode != "verification_probe_module_not_found" {
		t.Fatalf("baseline BaselineProbeReasonCode = %q, want verification_probe_module_not_found: %+v", baseline.BaselineProbeReasonCode, *baseline)
	}
	// Never an unavailable reason: the passed report stays clean.
	if code := report.VerificationUnavailableReasonCode(); code != "" {
		t.Fatalf("VerificationUnavailableReasonCode() = %q on a passed report; the baseline evidence row must never be an unavailable reason", code)
	}
	if code := types.ExecutedCommandUnavailableReasonCode(*baseline); code != "" {
		t.Fatalf("ExecutedCommandUnavailableReasonCode(baseline row) = %q, want \"\"", code)
	}
	if types.ExecutedCommandFailed(*baseline) {
		t.Fatalf("the baseline evidence row must not be a failed command: %+v", *baseline)
	}
	// Proof ledger: the baseline row is a covered executed_command capability.
	ledger := types.BuildVerificationProofLedger(plan, report, nil)
	if ledger.CapabilityUnavailableCount != 0 || ledger.CapabilityFailedCount != 0 {
		t.Fatalf("ledger counts unavailable=%d failed=%d, want 0/0: %+v", ledger.CapabilityUnavailableCount, ledger.CapabilityFailedCount, ledger.Capabilities)
	}
	baselineCovered := false
	for _, item := range ledger.Capabilities {
		if item.Kind != "executed_command" || item.Source != verificationProbeBaselineSource {
			continue
		}
		if item.Status != types.VerificationProofLedgerItemCovered {
			t.Fatalf("baseline capability status = %s, want covered: %+v", item.Status, item)
		}
		baselineCovered = true
	}
	if !baselineCovered {
		t.Fatalf("baseline capability missing from the ledger: %+v", ledger.Capabilities)
	}
	// The confidence lane is still weakened (probe_baseline unavailable,
	// warning) and names the inner probe reason.
	if !changeReportHasVerificationConfidence(report, "probe_baseline", "unavailable", verificationProbeBaselineUnavailableReasonCode) {
		t.Fatalf("probe_baseline lane must be weakened: %+v", report.VerificationConfidence)
	}
	laneNamesInnerReason := false
	for _, rec := range report.VerificationConfidence {
		if rec.Category == "probe_baseline" && rec.Status == "unavailable" && strings.Contains(rec.Detail, "verification_probe_module_not_found") {
			laneNamesInnerReason = true
		}
	}
	if !laneNamesInnerReason {
		t.Fatalf("probe_baseline lane must carry the inner probe reason in its detail: %+v", report.VerificationConfidence)
	}
	if changeReportHasVerificationConfidence(report, "probe_baseline", "satisfied", "verification_probe_baseline_expected_failure_observed") {
		t.Fatalf("an unavailable baseline must not mint differential authority: %+v", report.VerificationConfidence)
	}
	// No diagnostic is minted from the baseline evidence row.
	for _, diag := range report.VerificationDiagnostics {
		if strings.TrimSpace(diag.Source) == verificationProbeBaselineSource || strings.Contains(diag.Command, "verification_probe_baseline:") {
			t.Fatalf("baseline evidence row must not mint a diagnostic: %+v", diag)
		}
	}
}

// The baseline family codes are the only reason codes a baseline row
// carries, whatever the inner probe reported; the classifiers read the
// outcome switch first, so an out-of-lane ReasonCode on a member row cannot
// re-route it (the inner reason is a typed detail, never the reason lane).
// This table drives the probed lanes (three codes); the family's fourth
// code, verification_probe_baseline_snapshot_unavailable, belongs to the
// no-snapshot arm minted without an inner probe and is pinned by
// run_tests_fold_in6_test.go (fold-in round six).
func TestBaselineRowReasonLaneIsTheBaselineFamilyForEveryInnerProbeOutcome(t *testing.T) {
	for _, tc := range []struct {
		name          string
		inner         types.ExecutedCommand
		report        *types.ChangeReport
		wantOutcome   string
		wantReason    string
		wantInnerCode string
	}{
		{name: "module_missing_on_main", inner: types.ExecutedCommand{Runner: "verification_probe", Outcome: types.ExecutedCommandOutcomeParserError, ExitCode: 1, ReasonCode: "verification_probe_module_not_found"},
			report:      &types.ChangeReport{Passed: false, VerificationStatus: types.VerificationStatusFailed, FailureKind: types.FailureKindParserError},
			wantOutcome: types.ExecutedCommandOutcomeBaselineUnavailable, wantReason: verificationProbeBaselineUnavailableReasonCode, wantInnerCode: "verification_probe_module_not_found"},
		{name: "runner_missing_on_main", inner: types.ExecutedCommand{Runner: "verification_probe", Outcome: types.ExecutedCommandOutcomeRunnerMissing, ExitCode: 127, ReasonCode: "verification_probe_dependency_missing"},
			report:      &types.ChangeReport{Passed: false, VerificationStatus: types.VerificationStatusUnavailable, FailureKind: types.FailureKindRunnerMissing},
			wantOutcome: types.ExecutedCommandOutcomeBaselineUnavailable, wantReason: verificationProbeBaselineUnavailableReasonCode, wantInnerCode: "verification_probe_dependency_missing"},
		{name: "expected_failure_observed", inner: types.ExecutedCommand{Runner: "verification_probe", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 1, ReasonCode: "verification_probe_exception"},
			report:      &types.ChangeReport{Passed: false, VerificationStatus: types.VerificationStatusFailed, FailureKind: types.FailureKindTestsFailed},
			wantOutcome: types.ExecutedCommandOutcomeExpectedFailureObserved, wantReason: "verification_probe_baseline_expected_failure_observed", wantInnerCode: "verification_probe_exception"},
		{name: "expected_failure_not_observed", inner: types.ExecutedCommand{Runner: "verification_probe", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 0},
			report:      &types.ChangeReport{Passed: true, VerificationStatus: types.VerificationStatusPassed},
			wantOutcome: types.ExecutedCommandOutcomeExpectedFailureNotObserved, wantReason: "verification_probe_baseline_expected_failure_not_observed", wantInnerCode: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := verificationProbeBaselineCommand("p", verificationProbeRunResult{Commands: []types.ExecutedCommand{tc.inner}, Report: tc.report})
			if row.Outcome != tc.wantOutcome || row.ReasonCode != tc.wantReason || row.BaselineProbeReasonCode != tc.wantInnerCode {
				t.Fatalf("row = outcome %q reason %q inner %q, want %q / %q / %q", row.Outcome, row.ReasonCode, row.BaselineProbeReasonCode, tc.wantOutcome, tc.wantReason, tc.wantInnerCode)
			}
			if row.Source != verificationProbeBaselineSource || row.Command != "verification_probe_baseline:p" {
				t.Fatalf("row identity = source %q command %q", row.Source, row.Command)
			}
			if types.ExecutedCommandUnavailableReasonCode(row) != "" || types.ExecutedCommandFailed(row) {
				t.Fatalf("baseline row classified as unavailable/failed: %+v", row)
			}
		})
	}
}
