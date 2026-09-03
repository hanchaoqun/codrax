package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_worktree_fold_in_test.go — F-run-tests fold-in of V5-2
// (colleague_merge_audit §40.36 复核收编): the worktree audit runs exactly
// once per run_tests call and inherits the caller's timeout (F1/F2); the
// locked re-verify gate reads "report.Passed and owner suite not
// infra-downgraded" so an infra-downgraded suite is never re-run under the
// caps that killed it and its lockfile fixed point is disclosed as UNPROVEN
// on every surface (F3), while a Passed zero-test report runs the cheap
// lockfile-only witness (F4); the formatter resolves against the runner env
// before the command is constructed (F5); and the untracked lane is
// disclosed independently of the tracked lane's disposition (F6).

// F1 + F2: one rust project whose (fake) cargo refreshes Cargo.lock. The
// mid-loop provisional ledger must not audit the worktree, so the locked
// re-verify hook fires once (untouched code: once per passing plan plus the
// final report = 2 for one plan), and the request carries the caller's
// timeout_seconds rather than the default.
func TestRunTestsRunsTheWorktreeAuditOnceWithTheCallerTimeout(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"fixture\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "v1\n",
		"src/lib.rs": "pub fn a() {}\n",
	})
	fakeBin := t.TempDir()
	script := "#!/bin/sh\nprintf 'v2 refreshed by cargo\\n' > Cargo.lock\n" +
		"printf 'test a ... ok\\ntest result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out\\n'\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "cargo"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, &calls)

	mu := types.NewMutableState("lockfile refresh audited once")
	mu.SetChangePlan(&types.ChangePlan{ID: "plan-lockfile-once", Status: types.PlanStatusPending, TargetPaths: []string{"src/lib.rs"}})
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"timeout_seconds": 77}`))
	if err != nil {
		t.Fatalf("run_tests: %v", err)
	}
	report := mu.ChangeReport()
	if !result.Success || report == nil || !report.Passed || report.WorktreeAudit == nil ||
		report.WorktreeAudit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed {
		t.Fatalf("lockfile refresh must be disclosed on a passing run: result=%+v report=%+v", result, report)
	}
	if len(calls) != 1 {
		t.Fatalf("the locked re-verify must run exactly once per run_tests call (final report only), got %d: %+v", len(calls), calls)
	}
	if calls[0].Timeout != 77*time.Second {
		t.Fatalf("the locked re-verify must inherit the caller's timeout_seconds, got %v", calls[0].Timeout)
	}
	if len(report.WorktreeAudit.LockedReverify) != 1 || report.WorktreeAudit.LockedReverify[0].Outcome != types.VerificationLockedReverifyPassed ||
		report.WorktreeAudit.Effects[0].LockfileFixedPoint != types.VerificationLockfileFixedPointProven {
		t.Fatalf("proven fixed point must be typed on the row and the record: %+v", report.WorktreeAudit)
	}
}

// F3: when the owning runner's primary suite was cut short by an
// infrastructure cap (timeout / oom / cpu_limit — the launched-outcome
// roster's infra subset), the locked re-run is NOT executed, the row stays
// disclosed (never refused, even with a stub that would fail), the record
// says skipped_suite_infra_downgraded, and every run_tests-side surface
// names the fixed point as UNPROVEN in plain words.
func TestVerificationWorktreeAuditInfraDowngradedSuiteLeavesLockfileFixedPointUnproven(t *testing.T) {
	for _, outcome := range verificationDriftSuiteInfraOutcomes {
		t.Run(outcome, func(t *testing.T) {
			root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
			baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
			writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
			var calls []verificationLockedReverifyRequest
			stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 101}, &calls)
			// probePrimarySuiteInfraReport shape: Passed=true with the probe's
			// results, no failure kind, and a launched suite row carrying the
			// infra outcome.
			report := passingVerificationWorktreeReport()
			cut := types.ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: outcome, ExitCode: -1}
			attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, cut))
			audit := report.WorktreeAudit
			if len(calls) != 0 {
				t.Fatalf("an infra-downgraded suite must not be re-run under the same caps: %+v", calls)
			}
			if audit == nil || audit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed || !report.Passed || report.FailureKind != "" {
				t.Fatalf("the disclosed lane must be kept, never refused: report=%+v audit=%+v", report, audit)
			}
			if len(audit.LockedReverify) != 1 || audit.LockedReverify[0].Outcome != types.VerificationLockedReverifySkippedSuiteInfraDowngraded ||
				audit.LockedReverify[0].SuiteOutcome != outcome {
				t.Fatalf("locked re-verify record = %+v", audit.LockedReverify)
			}
			if len(audit.Effects) != 1 || audit.Effects[0].Disposition != types.VerificationWorktreeEffectDisclosed ||
				audit.Effects[0].LockfileFixedPoint != types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded {
				t.Fatalf("row must carry the typed unproven fixed point: %+v", audit.Effects)
			}
			phrase := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded, false)
			found := false
			for _, diag := range report.VerificationDiagnostics {
				if diag.ReasonCode == types.VerificationTrackedSideEffectDisclosedReason && strings.Contains(diag.Detail, phrase) {
					found = true
				}
			}
			if !found {
				t.Fatalf("the disclosed diagnostic must say the fixed point is unproven: %+v", report.VerificationDiagnostics)
			}
			if summary := renderRunTestsWorktreeAuditSummary(report); !strings.Contains(summary, "Cargo.lock=dependency_lockfile_refresh(rust)") || !strings.Contains(summary, phrase) {
				t.Fatalf("run_tests summary must name the unproven fixed point on the row: %q", summary)
			}
		})
	}
}

// F4: a Passed report with zero executed tests (NoTestsRunners) is still a
// passing report — the locked re-run is a cheap lockfile-only witness and
// executes (the seat exited 0). A seat whose own suite failed keeps the
// accurate skipped_report_failed label with its typed fixed point.
func TestVerificationWorktreeAuditPassedZeroTestReportRunsLockedReverify(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, &calls)
	report := &types.ChangeReport{Passed: true, NoTestsRunners: []string{"rust"}}
	zero := types.ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: "zero_tests", ExitCode: 0}
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, zero))
	audit := report.WorktreeAudit
	if len(calls) != 1 || calls[0].Command != "cargo test --locked" {
		t.Fatalf("a Passed zero-test report must run the lockfile-only locked witness: %+v", calls)
	}
	if audit == nil || audit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed || !report.Passed ||
		len(audit.LockedReverify) != 1 || audit.LockedReverify[0].Outcome != types.VerificationLockedReverifyPassed ||
		audit.Effects[0].LockfileFixedPoint != types.VerificationLockfileFixedPointProven {
		t.Fatalf("proven fixed point expected: report=%+v audit=%+v", report, audit)
	}

	// Accurate label: only a seat whose own suite failed (non-zero exit) is
	// skipped_report_failed (EVOLUTION RECORD §40.36 三轮收编 finding B: the
	// decision keys on the seat, so the failing suite carries cargo's exit 101).
	root = driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	baseline = captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	calls = nil
	failed := &types.ChangeReport{Passed: false, FailureKind: types.FailureKindTestsFailed, FailureReasonCode: "tests_failed",
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "t", Suite: "project", Passed: false}}}
	failedSeat := types.ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 101}
	attachVerificationWorktreeAudit(context.Background(), failed, baseline, root, driftInput(root, nil, failedSeat))
	if len(calls) != 0 || failed.WorktreeAudit.LockedReverify[0].Outcome != types.VerificationLockedReverifySkippedReportFailed ||
		failed.WorktreeAudit.Effects[0].LockfileFixedPoint != types.VerificationLockfileFixedPointUnprovenSuiteFailed {
		t.Fatalf("a failed report keeps skipped_report_failed with the typed unproven state: calls=%d audit=%+v", len(calls), failed.WorktreeAudit)
	}
}

// F5: a formatter present only on the runner environment's PATH (not the
// codrax process PATH) must run. Untouched code constructed the command with
// the bare name first, which stamped cmd.Err=ErrNotFound and never started it.
func TestVerificationDriftRunFormatterResolvesAgainstRunnerEnvPath(t *testing.T) {
	if _, err := exec.LookPath("codrax-fake-formatter"); err == nil {
		t.Skip("codrax-fake-formatter unexpectedly on the process PATH")
	}
	binDir := t.TempDir()
	// The runner env carries only PATH=binDir, so the script must not rely
	// on PATH lookups itself.
	script := "#!/bin/sh\n/bin/cat\nprintf 'formatted\\n'\n"
	if err := os.WriteFile(filepath.Join(binDir, "codrax-fake-formatter"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out, ok := verificationDriftRunFormatter([]string{"codrax-fake-formatter"}, []byte("in\n"), t.TempDir(), []string{"PATH=" + binDir})
	if !ok || string(out) != "in\nformatted\n" {
		t.Fatalf("formatter resolved through the runner env must run: ok=%v out=%q", ok, out)
	}
	if _, ok := verificationDriftRunFormatter([]string{"codrax-fake-formatter"}, []byte("in\n"), t.TempDir(), []string{"PATH=" + t.TempDir()}); ok {
		t.Fatalf("a formatter absent from the runner env PATH must stay unavailable")
	}
}

// F6 (run_tests surfaces): a refused run (tracked_drift) still discloses its
// retained untracked outputs in the tool summary and in the integrity
// diagnostic, while the planner-facing failure summary stays refused-rows-only.
func TestVerificationWorktreeAuditRefusedRunKeepsUntrackedDisclosure(t *testing.T) {
	root := newVerificationWorktreeRepo(t)
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "tracked.txt", "mutated by tests\n")
	writeVerificationWorktreeFile(t, root, "generated.bin", "artifact\n")
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, verificationWorktreeDriftInput{})
	audit := report.WorktreeAudit
	if audit == nil || audit.Status != types.VerificationWorktreeAuditTrackedDrift || audit.UntrackedEffectCount != 1 {
		t.Fatalf("fixture must be a refused run with one untracked output: %+v", audit)
	}
	found := false
	for _, diag := range report.VerificationDiagnostics {
		if diag.ReasonCode == verificationWorktreeTrackedDriftReason && strings.Contains(diag.Detail, "generated.bin") &&
			strings.Contains(diag.Detail, verificationWorktreeUntrackedRetainedClause) {
			found = true
		}
	}
	if !found {
		t.Fatalf("the refused diagnostic must disclose the untracked output: %+v", report.VerificationDiagnostics)
	}
	if strings.Contains(report.FailureSummary, "generated.bin") {
		t.Fatalf("the planner-facing failure summary stays refused-rows-only: %q", report.FailureSummary)
	}
	summary := renderRunTestsWorktreeAuditSummary(report)
	if !strings.Contains(summary, "verification failed: tracked.txt") || !strings.Contains(summary, "generated.bin") {
		t.Fatalf("run_tests summary must name both lanes on a refused run: %q", summary)
	}
}

// Census: the infra-downgrade subset of the launched-outcome roster is
// exactly the set of launched outcomes that makeResourceExhaustionReport
// types as a resource-exhaustion FailureKind (timeout / oom / cpu_limit);
// adding an outcome to one table without the other goes red here.
func TestVerificationDriftSuiteInfraOutcomesAreTheLaunchedResourceKinds(t *testing.T) {
	resourceKinds := map[types.FailureKind]bool{types.FailureKindTimeout: true, types.FailureKindOOM: true, types.FailureKindCPULimit: true}
	seenKinds := map[types.FailureKind]bool{}
	for _, outcome := range verificationDriftSuiteInfraOutcomes {
		if !verificationDriftCommandLaunched(types.ExecutedCommand{Outcome: outcome}) {
			t.Fatalf("infra outcome %q must be a launched outcome", outcome)
		}
		if got := verificationDriftCommandSuiteInfraOutcome(types.ExecutedCommand{Outcome: outcome}); got != outcome {
			t.Fatalf("infra outcome %q must round-trip, got %q", outcome, got)
		}
		kind := makeResourceExhaustionReport(outcome, "x").FailureKind
		if !resourceKinds[kind] {
			t.Fatalf("infra outcome %q types as %q, not a resource-exhaustion kind", outcome, kind)
		}
		seenKinds[kind] = true
	}
	if len(seenKinds) != len(resourceKinds) {
		t.Fatalf("infra outcomes cover %d resource kinds, want %d", len(seenKinds), len(resourceKinds))
	}
	for _, outcome := range verificationDriftLaunchedOutcomes {
		infra := verificationDriftCommandSuiteInfraOutcome(types.ExecutedCommand{Outcome: outcome}) != ""
		resource := resourceKinds[makeResourceExhaustionReport(outcome, "x").FailureKind]
		if infra != resource {
			t.Fatalf("launched outcome %q: infra=%v but resource-kind=%v — the two tables disagree", outcome, infra, resource)
		}
	}
	for _, fp := range types.AllVerificationLockfileFixedPoints() {
		if strings.HasPrefix(string(fp), "unproven_") != types.VerificationLockfileFixedPointUnproven(fp) {
			t.Fatalf("fixed point %q: unproven predicate disagrees with the closed set", fp)
		}
	}
}
