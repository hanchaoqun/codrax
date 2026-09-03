package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_worktree_fold_in3_test.go — F-run-tests round-three fold-in of
// V5-2 (colleague_merge_audit §40.36 三轮收编):
//   - A: every dependency_lockfile_refresh row carries a DECLARED member of
//     the closed fixed-point set on every producer branch; a mixed refused
//     run stamps unproven_run_refused (never the zero value, which rendered
//     byte-identical to proven) and every surface names it in plain words.
//   - B: the locked re-verify decision keys on the OWNER SEAT's typed facts
//     (infra outcome first, then the seat's own non-zero exit), never on
//     report.Passed — a passing suite whose changed path was merely
//     uncovered gets its cheap locked witness, and a timed-out suite without
//     a passed probe is labelled infra-downgraded, not "failed".
//   - D: one install choke point builds every exit's summary, so the early
//     timeout exit names both audit lanes (structural census: see
//     run_tests_install_choke_point_census_test.go).

func fixedPointRowsByPath(audit *types.VerificationWorktreeAudit) map[string]types.VerificationWorktreeEffect {
	out := map[string]types.VerificationWorktreeEffect{}
	if audit == nil {
		return out
	}
	for _, effect := range audit.Effects {
		out[effect.Path] = effect
	}
	return out
}

func installFakeCargo(t *testing.T, body string) {
	t.Helper()
	fakeBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeBin, "cargo"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func rustFixtureFiles(extra map[string]string) map[string]string {
	files := map[string]string{
		"Cargo.toml": "[package]\nname = \"fixture\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "v1\n",
		"src/lib.rs": "pub fn a() {}\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

func runRustFixture(t *testing.T, root string, targets []string, params string) (types.ToolResult, *types.ChangeReport) {
	t.Helper()
	mu := types.NewMutableState("fold-in three")
	mu.SetChangePlan(&types.ChangePlan{ID: "plan-fold-in-3", Status: types.PlanStatusPending, TargetPaths: targets})
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, json.RawMessage(params))
	if err != nil {
		t.Fatalf("run_tests: %v", err)
	}
	return result, mu.ChangeReport()
}

// A (census over the producers): every branch of the drift gate that can
// leave a dependency_lockfile_refresh row behind stamps a declared member of
// the closed fixed-point set on it; the expected member per branch is pinned.
func TestVerificationWorktreeAuditEveryLockfileRowCarriesADeclaredFixedPoint(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		mutate  []string
		stub    verificationLockedReverifyResult
		report  func() *types.ChangeReport
		seat    types.ExecutedCommand
		want    map[string]types.VerificationLockfileFixedPoint
		status  types.VerificationWorktreeAuditStatus
		reverif int
	}{
		{
			name:  "mixed refused run: lockfile row stays disclosed with unproven_run_refused",
			files: map[string]string{"Cargo.lock": "v1\n", "src/other.rs": "fn b() {}\n"}, mutate: []string{"Cargo.lock", "src/other.rs"},
			stub: verificationLockedReverifyResult{ExitCode: 0}, report: passingVerificationWorktreeReport, seat: executedRunner("rust", "."),
			want: map[string]types.VerificationLockfileFixedPoint{"Cargo.lock": types.VerificationLockfileFixedPointUnprovenRunRefused}, status: types.VerificationWorktreeAuditTrackedDrift,
		},
		{
			name:  "two lockfile owners refused by a third path: both rows unproven_run_refused",
			files: map[string]string{"Cargo.lock": "v1\n", "go.sum": "h1\n", "notes.txt": "n\n"}, mutate: []string{"Cargo.lock", "go.sum", "notes.txt"},
			stub: verificationLockedReverifyResult{ExitCode: 0}, report: passingVerificationWorktreeReport, seat: executedRunner("rust", "."),
			want:   map[string]types.VerificationLockfileFixedPoint{"Cargo.lock": types.VerificationLockfileFixedPointUnprovenRunRefused, "go.sum": types.VerificationLockfileFixedPointUnprovenRunRefused},
			status: types.VerificationWorktreeAuditTrackedDrift,
		},
		{
			name: "disclosed run with a passing locked witness: proven", files: map[string]string{"Cargo.lock": "v1\n"}, mutate: []string{"Cargo.lock"},
			stub: verificationLockedReverifyResult{ExitCode: 0}, report: passingVerificationWorktreeReport, seat: executedRunner("rust", "."),
			want: map[string]types.VerificationLockfileFixedPoint{"Cargo.lock": types.VerificationLockfileFixedPointProven}, status: types.VerificationWorktreeAuditTrackedDriftDisclosed, reverif: 1,
		},
		{
			name: "locked witness failed: disproven (row refused)", files: map[string]string{"Cargo.lock": "v1\n"}, mutate: []string{"Cargo.lock"},
			stub: verificationLockedReverifyResult{ExitCode: 101}, report: passingVerificationWorktreeReport, seat: executedRunner("rust", "."),
			want: map[string]types.VerificationLockfileFixedPoint{"Cargo.lock": types.VerificationLockfileFixedPointDisproven}, status: types.VerificationWorktreeAuditTrackedDrift, reverif: 1,
		},
		{
			name: "owner seat infra-downgraded: unproven_suite_infra_downgraded", files: map[string]string{"Cargo.lock": "v1\n"}, mutate: []string{"Cargo.lock"},
			stub: verificationLockedReverifyResult{ExitCode: 101}, report: passingVerificationWorktreeReport,
			seat: types.ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeTimeout, ExitCode: -1},
			want: map[string]types.VerificationLockfileFixedPoint{"Cargo.lock": types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded}, status: types.VerificationWorktreeAuditTrackedDriftDisclosed,
		},
		{
			name: "owner seat exited non-zero: unproven_suite_failed", files: map[string]string{"Cargo.lock": "v1\n"}, mutate: []string{"Cargo.lock"},
			stub: verificationLockedReverifyResult{ExitCode: 101},
			report: func() *types.ChangeReport {
				return &types.ChangeReport{Passed: false, FailureKind: types.FailureKindTestsFailed, FailureReasonCode: "tests_failed",
					TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "t", Suite: "project", Passed: false}}}
			},
			seat: types.ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 101},
			want: map[string]types.VerificationLockfileFixedPoint{"Cargo.lock": types.VerificationLockfileFixedPointUnprovenSuiteFailed}, status: types.VerificationWorktreeAuditTrackedDriftDisclosed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := driftRepoWithFiles(t, tc.files)
			baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
			for _, rel := range tc.mutate {
				writeVerificationWorktreeFile(t, root, rel, "mutated by verification\n")
			}
			var calls []verificationLockedReverifyRequest
			stubLockedReverify(t, tc.stub, &calls)
			report := tc.report()
			attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, tc.seat, executedRunner("go", ".")))
			audit := report.WorktreeAudit
			if audit == nil || audit.Status != tc.status {
				t.Fatalf("audit status = %+v, want %s", audit, tc.status)
			}
			if len(calls) != tc.reverif {
				t.Fatalf("locked re-verify calls = %d, want %d", len(calls), tc.reverif)
			}
			rows := fixedPointRowsByPath(audit)
			for _, effect := range audit.Effects {
				if effect.DriftClass != types.VerificationWorktreeDriftDependencyLockfileRefresh {
					if effect.LockfileFixedPoint != "" {
						t.Fatalf("row of another class must not carry a fixed point: %+v", effect)
					}
					continue
				}
				if effect.LockfileFixedPoint == "" || !types.VerificationLockfileFixedPointDeclared(effect.LockfileFixedPoint) {
					t.Fatalf("lockfile row %q carries an undeclared fixed point %q (the zero value renders as proven): %+v", effect.Path, effect.LockfileFixedPoint, audit)
				}
			}
			for path, want := range tc.want {
				if rows[path].LockfileFixedPoint != want {
					t.Fatalf("row %q fixed point = %q, want %q (rows=%+v)", path, rows[path].LockfileFixedPoint, want, audit.Effects)
				}
				if rows[path].DriftClass != types.VerificationWorktreeDriftDependencyLockfileRefresh {
					t.Fatalf("row %q must be a lockfile row: %+v", path, rows[path])
				}
			}
		})
	}
}

// A (mixed-run e2e, every surface): the fake cargo refreshes Cargo.lock AND
// rewrites an unowned tracked file. The run is refused for src/other.rs; the
// lockfile row is disclosed with unproven_run_refused, and the run_tests
// summary, the disclosed-row summary, the context pack (token + dedicated
// item) and the final report warning all carry the plain-words phrase — a
// refused run never shows a lockfile row as proven.
func TestRunTestsMixedRefusedRunNamesTheUnprovenLockfileOnEverySurface(t *testing.T) {
	root := driftRepoWithFiles(t, rustFixtureFiles(map[string]string{"src/other.rs": "fn b() {}\n"}))
	installFakeCargo(t, "printf 'v2 refreshed by cargo\\n' > Cargo.lock\nprintf 'rewritten by tests\\n' > src/other.rs\n"+
		"printf 'test a ... ok\\ntest result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out\\n'\n")
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, &calls)
	result, report := runRustFixture(t, root, []string{"src/lib.rs"}, `{}`)
	if result.Success || report == nil || report.Passed || report.FailureKind != types.FailureKindVerificationSideEffect ||
		report.WorktreeAudit == nil || report.WorktreeAudit.Status != types.VerificationWorktreeAuditTrackedDrift {
		t.Fatalf("mixed drift must be refused: result=%+v report=%+v", result, report)
	}
	if len(calls) != 0 {
		t.Fatalf("a refused run must not attempt the locked re-run: %+v", calls)
	}
	rows := fixedPointRowsByPath(report.WorktreeAudit)
	if rows["Cargo.lock"].Disposition != types.VerificationWorktreeEffectDisclosed || rows["Cargo.lock"].LockfileFixedPoint != types.VerificationLockfileFixedPointUnprovenRunRefused ||
		rows["src/other.rs"].Disposition != types.VerificationWorktreeEffectRefused {
		t.Fatalf("rows = %+v", report.WorktreeAudit.Effects)
	}
	phrase := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenRunRefused, false)
	if phrase == "" {
		t.Fatal("unproven_run_refused must carry a plain-words phrase")
	}
	// run_tests summary + disclosed-row summary.
	for _, want := range []string{"verification failed: src/other.rs", "Cargo.lock=dependency_lockfile_refresh(rust) [" + phrase + "]"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("run_tests summary lost %q:\n%s", want, result.Summary)
		}
	}
	if !strings.Contains(renderRunTestsWorktreeAuditSummary(report), phrase) || !strings.Contains(verificationWorktreeDisclosedRowsSummary(report.WorktreeAudit), phrase) {
		t.Fatalf("disclosed-row summary must name the unproven fixed point: %q", renderRunTestsWorktreeAuditSummary(report))
	}
	if strings.Contains(report.FailureSummary, "Cargo.lock") {
		t.Fatalf("the planner-facing failure summary stays refused-rows-only: %q", report.FailureSummary)
	}
	// Context pack: typed token on the effect row + the dedicated disclosure item.
	pack := types.WriteContextPackFromChangeReport(report)
	token, item := false, false
	for _, it := range pack.Items {
		if it.Kind == "verification_worktree_effect" && strings.Contains(it.Text, "lockfile_fixed_point=unproven_run_refused") && strings.HasSuffix(it.Text, "path=Cargo.lock") {
			token = true
		}
		if it.Kind == "verification_lockfile_fixed_point" && strings.Contains(it.Text, phrase) && strings.HasSuffix(it.Text, "path=Cargo.lock") {
			item = true
		}
	}
	if !token || !item {
		t.Fatalf("context pack lost the run-refused disclosure (token=%v item=%v): %+v", token, item, pack.Items)
	}
	// Final report: the disclosed warning beside the refused error carries the phrase.
	final := types.BuildWriteFinalReport(types.WriteFinalReportInput{Report: report})
	warned, errored := false, false
	for _, risk := range final.ResidualRisks {
		if risk.Code == types.VerificationTrackedSideEffectDisclosedReason && risk.Severity == "warning" && strings.Contains(risk.Detail, "Cargo.lock=dependency_lockfile_refresh") && strings.Contains(risk.Detail, phrase) {
			warned = true
		}
		if risk.Code == "verification_worktree_tracked_drift" && risk.Severity == "error" && strings.Contains(risk.Detail, "src/other.rs") && !strings.Contains(risk.Detail, "Cargo.lock") {
			errored = true
		}
	}
	if !warned || !errored {
		t.Fatalf("final report residual risks (warned=%v errored=%v): %+v", warned, errored, final.ResidualRisks)
	}
}

// B(a) on the real Execute path: the suite PASSED (fake cargo exit 0) but a
// changed path outside the rust family is uncovered, so the report verdict is
// verification_incomplete (Passed=false). The owner seat exited 0, so the
// cheap locked witness runs and proves the fixed point; nothing claims "the
// test suite failed".
func TestRunTestsUncoveredChangedPathStillRunsTheLockedWitnessForAPassingSeat(t *testing.T) {
	root := driftRepoWithFiles(t, rustFixtureFiles(nil))
	installFakeCargo(t, "printf 'v2 refreshed by cargo\\n' > Cargo.lock\n"+
		"printf 'test a ... ok\\ntest result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out\\n'\n")
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, &calls)
	result, report := runRustFixture(t, root, []string{"src/lib.rs", "web/app.ts"}, `{}`)
	if report == nil || report.Passed || report.FailureKind != types.FailureKindVerificationIncomplete {
		t.Fatalf("fixture must be an uncovered-path verification_incomplete report: %+v", report)
	}
	if len(calls) != 1 {
		t.Fatalf("a seat that exited 0 must get its locked witness even when the verdict is unavailable for coverage reasons: %+v", calls)
	}
	audit := report.WorktreeAudit
	if audit == nil || audit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed || len(audit.LockedReverify) != 1 ||
		audit.LockedReverify[0].Outcome != types.VerificationLockedReverifyPassed ||
		fixedPointRowsByPath(audit)["Cargo.lock"].LockfileFixedPoint != types.VerificationLockfileFixedPointProven {
		t.Fatalf("proven fixed point expected: %+v", audit)
	}
	if report.FailureKind != types.FailureKindVerificationIncomplete {
		t.Fatalf("the coverage verdict must survive the audit: %+v", report)
	}
	failedPhrase := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenSuiteFailed, false)
	if strings.Contains(result.Summary, failedPhrase) || !strings.Contains(result.Summary, "Cargo.lock=dependency_lockfile_refresh(rust)") {
		t.Fatalf("summary must name the proven lockfile row and never say the suite failed:\n%s", result.Summary)
	}
}

// B(b) + D on the real Execute path: the fake cargo refreshes Cargo.lock,
// leaves junk.out behind and then outlives timeout_seconds=2 with no passed
// probe, so the report is a timeout failure (Passed=false). The owner seat
// is infra-downgraded — evaluated before any report-level check — so the
// record says skipped_suite_infra_downgraded (never "report failed"), the
// row stays disclosed with the infra phrase, and the early timeout exit's
// summary names BOTH audit lanes through the single install choke point.
func TestRunTestsTimeoutExitDisclosesInfraDowngradedLockfileAndUntrackedOutput(t *testing.T) {
	root := driftRepoWithFiles(t, rustFixtureFiles(nil))
	installFakeCargo(t, "printf 'v2 refreshed by cargo\\n' > Cargo.lock\nprintf 'junk\\n' > junk.out\nsleep 8 </dev/null >/dev/null 2>&1\n")
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 101}, &calls)
	result, report := runRustFixture(t, root, []string{"src/lib.rs"}, `{"timeout_seconds": 2}`)
	if report == nil || report.Passed || report.FailureKind != types.FailureKindTimeout {
		t.Fatalf("fixture must be a timeout failure without a passed probe: %+v", report)
	}
	if len(calls) != 0 {
		t.Fatalf("an infra-downgraded seat must not be re-run under the caps that killed it: %+v", calls)
	}
	audit := report.WorktreeAudit
	if audit == nil || audit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed || len(audit.LockedReverify) != 1 ||
		audit.LockedReverify[0].Outcome != types.VerificationLockedReverifySkippedSuiteInfraDowngraded ||
		audit.LockedReverify[0].SuiteOutcome != types.ExecutedCommandOutcomeTimeout {
		t.Fatalf("the infra outcome must be evaluated before any report-level check: %+v", audit)
	}
	rows := fixedPointRowsByPath(audit)
	if rows["Cargo.lock"].Disposition != types.VerificationWorktreeEffectDisclosed || rows["Cargo.lock"].LockfileFixedPoint != types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded {
		t.Fatalf("lockfile row = %+v", rows["Cargo.lock"])
	}
	infraPhrase := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded, false)
	failedPhrase := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenSuiteFailed, false)
	for _, want := range []string{"command timed out after", "Cargo.lock=dependency_lockfile_refresh(rust) [" + infraPhrase + "]", verificationWorktreeUntrackedRetainedClause, "junk.out"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("the timeout exit summary must name both audit lanes (lost %q):\n%s", want, result.Summary)
		}
	}
	if strings.Contains(result.Summary, failedPhrase) {
		t.Fatalf("a cut-short suite must never be called failed:\n%s", result.Summary)
	}
}

// B (seat facts aggregate over the seat's rows in precedence order): the
// pre-suite continuation preview row (exit 0) never hides the real suite's
// non-zero exit, an infra row wins over both, and an executed row with exit 0
// leaves the seat eligible for the locked witness.
func TestVerificationDriftRosterAggregatesSeatFactsInPrecedenceOrder(t *testing.T) {
	root := t.TempDir()
	preview := types.ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeSuiteContinued}
	cases := []struct {
		name       string
		rows       []types.ExecutedCommand
		wantInfra  string
		wantFailed bool
	}{
		{name: "preview then executed exit 101", rows: []types.ExecutedCommand{preview, {Runner: "rust", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 101}}, wantFailed: true},
		{name: "preview then executed exit 0", rows: []types.ExecutedCommand{preview, executedRunner("rust", ".")}},
		{name: "executed exit 101 then timeout row", rows: []types.ExecutedCommand{{Runner: "rust", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 101}, {Runner: "rust", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeOOM, ExitCode: -1}}, wantInfra: types.ExecutedCommandOutcomeOOM, wantFailed: true},
		{name: "runner_missing row is not launched and marks nothing", rows: []types.ExecutedCommand{{Runner: "rust", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeRunnerMissing, ExitCode: 127}, executedRunner("rust", ".")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roster := verificationDriftRoster(driftInput(root, nil, tc.rows...), root)
			if len(roster) != 1 {
				t.Fatalf("roster = %+v", roster)
			}
			if roster[0].suiteInfraOutcome != tc.wantInfra || roster[0].suiteExitFailed != tc.wantFailed {
				t.Fatalf("seat facts = infra=%q failed=%v, want infra=%q failed=%v", roster[0].suiteInfraOutcome, roster[0].suiteExitFailed, tc.wantInfra, tc.wantFailed)
			}
			record, fp := verificationLockedReverifyRecordForOwner(driftInput(root, nil), root, verificationDriftRosterEntry{runner: "rust", dirRel: ".", suiteInfraOutcome: roster[0].suiteInfraOutcome, suiteExitFailed: roster[0].suiteExitFailed})
			switch {
			case tc.wantInfra != "":
				if record.Outcome != types.VerificationLockedReverifySkippedSuiteInfraDowngraded || fp != types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded {
					t.Fatalf("infra seat must be skipped as infra-downgraded first: %+v %s", record, fp)
				}
			case tc.wantFailed:
				if record.Outcome != types.VerificationLockedReverifySkippedReportFailed || fp != types.VerificationLockfileFixedPointUnprovenSuiteFailed {
					t.Fatalf("failed seat must be skipped with the seat-failed state: %+v %s", record, fp)
				}
			default:
				// A seat that exited 0 reaches the locked runner (no locked
				// lane for a bare entry without a framework command ⇒ the
				// real runner reports unavailable ⇒ disproven), proving the
				// decision did not short-circuit on any report-level fact.
				if record.Outcome == types.VerificationLockedReverifySkippedReportFailed || record.Outcome == types.VerificationLockedReverifySkippedSuiteInfraDowngraded {
					t.Fatalf("a seat that exited 0 must reach the locked witness: %+v %s", record, fp)
				}
			}
		})
	}
}

// D (structural): the choke-point pin moved to
// run_tests_install_choke_point_census_test.go (fold-in round four, finding
// N) — it is a go/ast census bound by data flow, not a substring count.
