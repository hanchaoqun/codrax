package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerificationWorktreeAuditDisclosesOnlyNewUntrackedOutput(t *testing.T) {
	root := newVerificationWorktreeRepo(t)
	writeVerificationWorktreeFile(t, root, "customer.tmp", "before\n")
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "customer.tmp", "after\n")
	writeVerificationWorktreeFile(t, root, "generated.bin", "artifact\n")

	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root)
	if report.WorktreeAudit == nil || report.WorktreeAudit.Status != types.VerificationWorktreeAuditUntrackedSideEffects {
		t.Fatalf("untracked audit = %+v", report.WorktreeAudit)
	}
	if !report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		t.Fatalf("untracked output must preserve test pass: %+v", report)
	}
	if len(report.WorktreeAudit.Effects) != 1 || report.WorktreeAudit.Effects[0].Path != "generated.bin" ||
		report.WorktreeAudit.Effects[0].Action != "retained_not_committed_not_auto_deleted" {
		t.Fatalf("new output roster = %+v", report.WorktreeAudit.Effects)
	}
	if _, err := os.Stat(filepath.Join(root, "generated.bin")); err != nil {
		t.Fatalf("audit deleted unowned output: %v", err)
	}
}

func TestVerificationWorktreeAuditFailsTrackedDrift(t *testing.T) {
	root := newVerificationWorktreeRepo(t)
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "tracked.txt", "mutated by tests\n")

	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root)
	if report.WorktreeAudit == nil || report.WorktreeAudit.Status != types.VerificationWorktreeAuditTrackedDrift ||
		report.WorktreeAudit.TrackedEffectCount != 1 {
		t.Fatalf("tracked audit = %+v", report.WorktreeAudit)
	}
	if report.Passed || report.FailureKind != types.FailureKindVerificationSideEffect ||
		report.FailureReasonCode != verificationWorktreeTrackedDriftReason ||
		report.NormalizeVerificationStatus() != types.VerificationStatusFailed {
		t.Fatalf("tracked drift did not fail verification: %+v", report)
	}
	if len(report.TestResults) != 2 || report.TestResults[1].AssertionID != "verification_worktree_integrity" || report.TestResults[1].Passed {
		t.Fatalf("integrity witness missing: %+v", report.TestResults)
	}
}

func TestVerificationWorktreeAuditCleanAndNonGitBoundaries(t *testing.T) {
	root := newVerificationWorktreeRepo(t)
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root)
	if report.WorktreeAudit == nil || report.WorktreeAudit.Status != types.VerificationWorktreeAuditClean || !report.Passed {
		t.Fatalf("clean audit = report=%+v audit=%+v", report, report.WorktreeAudit)
	}

	nonGit := t.TempDir()
	nonGitBaseline := captureVerificationWorktreeSnapshot(context.Background(), nonGit)
	nonGitReport := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), nonGitReport, nonGitBaseline, nonGit)
	if nonGitReport.WorktreeAudit != nil || !nonGitReport.Passed {
		t.Fatalf("non-git test fixture should keep prior behavior: %+v", nonGitReport)
	}
}

func TestRunTestsPublishesUntrackedVerificationOutput(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make unavailable")
	}
	root := newVerificationWorktreeRepo(t)
	writeVerificationWorktreeFile(t, root, "main.c", "int main(void) { return 0; }\n")
	writeVerificationWorktreeFile(t, root, "Makefile", "test:\n\tcp main.c generated.bin\n")
	runVerificationWorktreeGit(t, root, "add", "main.c", "Makefile")
	runVerificationWorktreeGit(t, root, "commit", "-q", "-m", "fixture")

	mu := types.NewMutableState("verification output audit")
	mu.SetChangePlan(&types.ChangePlan{
		ID: "plan-worktree-audit", Status: types.PlanStatusPending,
		TargetPaths: []string{"main.c"},
	})
	ctx := &types.BusContext{
		Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify,
		RepoRoot: root, MainRepoRoot: root,
	}
	result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("run_tests: %v", err)
	}
	report := mu.ChangeReport()
	if !result.Success || report == nil || !report.Passed {
		t.Fatalf("passing make target lost test authority: result=%+v report=%+v", result, report)
	}
	if report.WorktreeAudit == nil || report.WorktreeAudit.Status != types.VerificationWorktreeAuditUntrackedSideEffects ||
		report.WorktreeAudit.UntrackedEffectCount != 1 || report.WorktreeAudit.Effects[0].Path != "generated.bin" {
		t.Fatalf("run_tests did not publish generated output: %+v", report.WorktreeAudit)
	}
	if !strings.Contains(result.Summary, "generated.bin") {
		t.Fatalf("tool summary hid generated output: %s", result.Summary)
	}
}

func newVerificationWorktreeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runVerificationWorktreeGit(t, root, "init", "-q")
	runVerificationWorktreeGit(t, root, "config", "user.email", "codrax-test@example.invalid")
	runVerificationWorktreeGit(t, root, "config", "user.name", "Codrax Test")
	writeVerificationWorktreeFile(t, root, "tracked.txt", "baseline\n")
	runVerificationWorktreeGit(t, root, "add", "tracked.txt")
	runVerificationWorktreeGit(t, root, "commit", "-q", "-m", "baseline")
	return root
}

func runVerificationWorktreeGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeVerificationWorktreeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func passingVerificationWorktreeReport() *types.ChangeReport {
	return &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{{
			Kind: types.TestResultKindUnit, AssertionID: "project-tests", Suite: "project", Passed: true,
		}},
	}
}
