package tool

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_worktree_drift_test.go — V5-2 (colleague_merge_audit §40.11):
// the tracked-drift gate classifies each drifted path into a closed owner set
// from typed facts (executed-runner roster × closed runner manifest × plan
// output_path contracts × formatter fixed point), discloses the owned classes
// after a locked re-verify, and refuses only unclassified drift.

func driftRepoWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := newVerificationWorktreeRepo(t)
	for rel, body := range files {
		writeVerificationWorktreeFile(t, root, rel, body)
		runVerificationWorktreeGit(t, root, "add", rel)
	}
	runVerificationWorktreeGit(t, root, "commit", "-q", "-m", "fixture")
	return root
}

func driftInput(root string, plan *types.ChangePlan, executed ...types.ExecutedCommand) verificationWorktreeDriftInput {
	return verificationWorktreeDriftInput{plan: plan, executed: executed, repoRoot: root, mainRoot: root}
}

func executedRunner(runner, dir string) types.ExecutedCommand {
	return types.ExecutedCommand{Runner: runner, WorkingDir: dir, Outcome: "executed", ExitCode: 0}
}

func stubLockedReverify(t *testing.T, result verificationLockedReverifyResult, calls *[]verificationLockedReverifyRequest) {
	t.Helper()
	prev := verificationLockedReverifyHook
	verificationLockedReverifyHook = func(req verificationLockedReverifyRequest) verificationLockedReverifyResult {
		if calls != nil {
			*calls = append(*calls, req)
		}
		return result
	}
	t.Cleanup(func() { verificationLockedReverifyHook = prev })
}

func TestVerificationWorktreeAuditDisclosesDependencyLockfileRefresh(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.toml": "[package]\n", "Cargo.lock": "v1\n", "src/lib.rs": "fn a() {}\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2 refreshed by cargo\n")
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, &calls)

	report := passingVerificationWorktreeReport()
	plan := &types.ChangePlan{ID: "p", TargetPaths: []string{"Cargo.toml", "src/lib.rs"}}
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, plan, executedRunner("rust", ".")))
	audit := report.WorktreeAudit
	if audit == nil || audit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed || audit.ReasonCode != types.VerificationTrackedSideEffectDisclosedReason {
		t.Fatalf("lockfile refresh must be disclosed: %+v", audit)
	}
	if !report.Passed || report.FailureKind != "" || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		t.Fatalf("disclosed drift must keep the verdict: %+v", report)
	}
	if len(audit.Effects) != 1 || audit.Effects[0].DriftClass != types.VerificationWorktreeDriftDependencyLockfileRefresh ||
		audit.Effects[0].OwnerRunner != "rust" || audit.Effects[0].Disposition != types.VerificationWorktreeEffectDisclosed ||
		audit.DisclosedTrackedEffectCount != 1 || audit.RefusedTrackedEffectCount != 0 {
		t.Fatalf("effect row = %+v (audit %+v)", audit.Effects, audit)
	}
	if len(calls) != 1 || calls[0].Runner != "rust" || calls[0].Command != "cargo test --locked" {
		t.Fatalf("locked re-verify must run the runner's locked form: %+v", calls)
	}
	if len(audit.LockedReverify) != 1 || audit.LockedReverify[0].Outcome != "passed" {
		t.Fatalf("locked re-verify record = %+v", audit.LockedReverify)
	}
	found := false
	for _, rec := range report.VerificationConfidence {
		found = found || (rec.Status == "advisory" && rec.ReasonCode == types.VerificationTrackedSideEffectDisclosedReason)
	}
	if !found {
		t.Fatalf("disclosure must land as an advisory confidence record: %+v", report.VerificationConfidence)
	}
	for _, tr := range report.TestResults {
		if tr.AssertionID == "verification_worktree_integrity" {
			t.Fatalf("disclosed drift must not mint the failing integrity witness: %+v", report.TestResults)
		}
	}
}

func TestVerificationWorktreeAuditRefusesUnclassifiedDrift(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n", "src/other.rs": "fn b() {}\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "src/other.rs", "rewritten by a test\n")
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, &calls)

	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, &types.ChangePlan{ID: "p", TargetPaths: []string{"src/lib.rs"}}, executedRunner("rust", ".")))
	audit := report.WorktreeAudit
	if audit == nil || audit.Status != types.VerificationWorktreeAuditTrackedDrift || report.Passed ||
		report.FailureKind != types.FailureKindVerificationSideEffect || report.FailureReasonCode != verificationWorktreeTrackedDriftReason {
		t.Fatalf("unclassified drift must still refuse: report=%+v audit=%+v", report, audit)
	}
	if len(audit.Effects) != 1 || audit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified ||
		audit.Effects[0].Disposition != types.VerificationWorktreeEffectRefused || audit.RefusedTrackedEffectCount != 1 {
		t.Fatalf("effect row = %+v", audit.Effects)
	}
	if !strings.Contains(report.FailureSummary, "src/other.rs") || len(calls) != 0 {
		t.Fatalf("refused run must name the path and never re-verify: %q calls=%d", report.FailureSummary, len(calls))
	}
}

func TestVerificationWorktreeAuditLockfileWithoutOwnerRunnerIsUnclassified(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, nil)
	// A make runner executed, not cargo: the path name alone selects nothing.
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("make", ".")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("owner must come from the typed roster: %+v", report.WorktreeAudit)
	}
	// Sibling working dir: the lockfile's directory is not an ancestor.
	root = driftRepoWithFiles(t, map[string]string{"other/Cargo.lock": "v1\n", "crates/a/Cargo.toml": "[package]\n"})
	baseline = captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "other/Cargo.lock", "v2\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", "crates/a")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("sibling lockfile must not be owned: %+v", report.WorktreeAudit)
	}
	// Workspace root: the lockfile sits above the member crate.
	root = driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n", "crates/a/Cargo.toml": "[package]\n"})
	baseline = captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", "crates/a")))
	if !report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftDependencyLockfileRefresh {
		t.Fatalf("workspace-root lockfile must be owned by the member runner: %+v", report.WorktreeAudit)
	}
}

func TestVerificationWorktreeAuditLockedReverifyFailureRefuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result verificationLockedReverifyResult
		want   string
	}{
		{"exit", verificationLockedReverifyResult{ExitCode: 101}, "failed"},
		{"drift recurred", verificationLockedReverifyResult{ExitCode: 0, DriftedPaths: []string{"Cargo.lock"}}, "drift_recurred"},
		{"unavailable", verificationLockedReverifyResult{Unavailable: true}, "unavailable"},
	} {
		root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
		baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
		writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
		stubLockedReverify(t, tc.result, nil)
		report := passingVerificationWorktreeReport()
		attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", ".")))
		audit := report.WorktreeAudit
		if report.Passed || audit.Status != types.VerificationWorktreeAuditTrackedDrift ||
			!strings.Contains(report.FailureReasonCode, verificationWorktreeTrackedDriftReason) ||
			!strings.Contains(report.FailureReasonCode, types.VerificationLockedReverifyFailedReason) {
			t.Fatalf("%s: a failed locked re-verify must refuse: report=%+v audit=%+v", tc.name, report, audit)
		}
		if len(audit.LockedReverify) != 1 || audit.LockedReverify[0].Outcome != tc.want ||
			audit.Effects[0].Disposition != types.VerificationWorktreeEffectRefused || audit.DisclosedTrackedEffectCount != 0 {
			t.Fatalf("%s: re-verify record/disposition = %+v / %+v", tc.name, audit.LockedReverify, audit.Effects)
		}
	}
}

func TestVerificationWorktreeAuditMixedDriftStaysRefusedButKeepsDisclosedRow(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n", "src/other.rs": "fn b() {}\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	writeVerificationWorktreeFile(t, root, "src/other.rs", "rewritten\n")
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, &calls)
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", ".")))
	audit := report.WorktreeAudit
	if report.Passed || audit.Status != types.VerificationWorktreeAuditTrackedDrift || len(calls) != 0 {
		t.Fatalf("mixed drift must refuse without re-verifying: %+v", audit)
	}
	byPath := map[string]types.VerificationWorktreeEffect{}
	for _, effect := range audit.Effects {
		byPath[effect.Path] = effect
	}
	if byPath["Cargo.lock"].DriftClass != types.VerificationWorktreeDriftDependencyLockfileRefresh || byPath["Cargo.lock"].Disposition != types.VerificationWorktreeEffectDisclosed ||
		byPath["src/other.rs"].Disposition != types.VerificationWorktreeEffectRefused {
		t.Fatalf("rows = %+v", audit.Effects)
	}
	if strings.Contains(report.FailureSummary, "Cargo.lock") || !strings.Contains(report.FailureSummary, "src/other.rs") {
		t.Fatalf("summary must name refused rows only: %q", report.FailureSummary)
	}
}

func TestVerificationWorktreeAuditDisclosesFormatterFixedPoint(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"go.mod": "module x\n", "main.go": "package x\nfunc  a(){}\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "main.go", "package x\n\nfunc a() {}\n")
	prev := verificationFormatterHook
	verificationFormatterHook = func(argv []string, input []byte) ([]byte, bool) {
		if argv[0] != "gofmt" || string(input) != "package x\nfunc  a(){}\n" {
			t.Fatalf("formatter must see the pre-run blob via gofmt: %v %q", argv, input)
		}
		return []byte("package x\n\nfunc a() {}\n"), true
	}
	t.Cleanup(func() { verificationFormatterHook = prev })
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("go", ".")))
	audit := report.WorktreeAudit
	if !report.Passed || audit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed ||
		audit.Effects[0].DriftClass != types.VerificationWorktreeDriftFormatterNoSemanticDiff || audit.Effects[0].OwnerRunner != "go" || len(audit.LockedReverify) != 0 {
		t.Fatalf("formatter fixed point must be disclosed without a locked re-verify: %+v", audit)
	}
	// Semantic change: the formatter output differs from the post-run bytes.
	verificationFormatterHook = func(argv []string, input []byte) ([]byte, bool) { return []byte("package x\n\nfunc a() {}\n"), true }
	writeVerificationWorktreeFile(t, root, "main.go", "package x\n\nfunc a() { panic(1) }\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("go", ".")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("a semantic change must stay unclassified: %+v", report.WorktreeAudit)
	}
	// Formatter unavailable ⇒ cannot prove ⇒ unclassified.
	verificationFormatterHook = func(argv []string, input []byte) ([]byte, bool) { return nil, false }
	writeVerificationWorktreeFile(t, root, "main.go", "package x\n\nfunc a() {}\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("go", ".")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("an unavailable formatter must not disclose: %+v", report.WorktreeAudit)
	}
}

func TestVerificationWorktreeAuditDisclosesPlanDeclaredGeneratedOutput(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"gen/out.txt": "old\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "gen/out.txt", "regenerated\n")
	plan := &types.ChangePlan{ID: "p", BehaviorContracts: []types.WriteBehaviorContract{{
		ID: "gen", Kind: types.WriteBehaviorOutputPath, Polarity: types.WriteBehaviorPolarityExpected,
		Operator: types.WriteBehaviorOpExists, Subject: "gen/out.txt", Required: true,
	}}}
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, plan, executedRunner("make", ".")))
	if !report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftDeclaredGeneratedOutput {
		t.Fatalf("typed output_path contract must own the generated path: %+v", report.WorktreeAudit)
	}
	// An undeclared sibling stays unclassified even with the contract present.
	root = driftRepoWithFiles(t, map[string]string{"gen/out.txt": "old\n", "gen/other.txt": "old\n"})
	baseline = captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "gen/other.txt", "regenerated\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, plan, executedRunner("make", ".")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("undeclared output must refuse: %+v", report.WorktreeAudit)
	}
}

func TestBuildLockedRunCommand(t *testing.T) {
	cases := []struct {
		runner, suite, wantCmd string
		wantEnv                []string
		ok                     bool
	}{
		{"rust", "", "cargo test --locked", nil, true},
		{"rust", "foo", `cargo test --locked "foo"`, nil, true},
		{"go", "", "go test -mod=readonly -json ./...", nil, true},
		{"swift", "", "swift test --force-resolved-versions", nil, true},
		{"ruby", "", "bundle exec rspec --format json", []string{"BUNDLE_FROZEN=true"}, true},
		{"node", "", "", nil, false},
		{"python", "", "", nil, false},
		{"java", "", "", nil, false},
		{"make", "", "", nil, false},
	}
	for _, tc := range cases {
		cmd, env, ok := buildLockedRunCommand(tc.runner, "", tc.suite, t.TempDir(), "")
		if ok != tc.ok || cmd != tc.wantCmd || strings.Join(env, ",") != strings.Join(tc.wantEnv, ",") {
			t.Fatalf("%s/%q: got (%q, %v, %v) want (%q, %v, %v)", tc.runner, tc.suite, cmd, env, ok, tc.wantCmd, tc.wantEnv, tc.ok)
		}
	}
}

func TestRunnerSideEffectManifestCensus(t *testing.T) {
	manifests := runnerSideEffectManifests()
	if len(manifests) != len(allowedRunners) {
		t.Fatalf("manifest keys %d != allowedRunners %d", len(manifests), len(allowedRunners))
	}
	for runner := range allowedRunners {
		m, ok := manifests[runner]
		if !ok || m.Runner != runner {
			t.Fatalf("runner %q missing or mis-keyed in the side-effect manifest", runner)
		}
		if len(m.LockfileBasenames) > 0 && !m.hasLockedLane() {
			t.Fatalf("runner %q declares lockfiles without a locked re-verify lane", runner)
		}
		if (len(m.Formatter) == 0) != (len(m.FormatterExts) == 0) {
			t.Fatalf("runner %q formatter and extensions must be declared together", runner)
		}
		for _, ext := range m.FormatterExts {
			if !strings.HasPrefix(ext, ".") {
				t.Fatalf("runner %q formatter ext %q must start with '.'", runner, ext)
			}
		}
		for _, base := range m.LockfileBasenames {
			if strings.ContainsAny(base, "/*?[") {
				t.Fatalf("runner %q lockfile %q must be an exact basename", runner, base)
			}
		}
	}
	for base, owners := range runnerSideEffectLockfileOwners() {
		if len(owners) != 1 {
			t.Fatalf("lockfile %q owned by more than one runner: %v", base, owners)
		}
	}
}

// ---- review fold-in pins (§40.36 复核) ----

// A failing primary report never triggers the locked re-run and never has its
// own failure kind overwritten by the lockfile lane.
func TestVerificationWorktreeAuditFailedReportSkipsLockedReverify(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	var calls []verificationLockedReverifyRequest
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 101}, &calls)
	report := &types.ChangeReport{Passed: false, FailureKind: types.FailureKindTestsFailed, FailureReasonCode: "tests_failed", FailureSummary: "2 tests failed",
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "t", Suite: "project", Passed: false}}}
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", ".")))
	audit := report.WorktreeAudit
	if len(calls) != 0 || report.FailureKind != types.FailureKindTestsFailed || report.FailureReasonCode != "tests_failed" || report.FailureSummary != "2 tests failed" {
		t.Fatalf("a failing suite must keep its own failure and never re-run locked: calls=%d report=%+v", len(calls), report)
	}
	if audit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed || audit.Effects[0].Disposition != types.VerificationWorktreeEffectDisclosed ||
		len(audit.LockedReverify) != 1 || audit.LockedReverify[0].Outcome != "skipped_report_failed" {
		t.Fatalf("lockfile rows stay disclosed with the re-verify recorded as skipped: %+v", audit)
	}
}

// Only the owner whose locked re-run failed is refused; a proven fixed point
// stays disclosed and is never named in the failure summary.
func TestVerificationWorktreeAuditRefusesOnlyTheFailingLockfileOwner(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n", "gomod/go.mod": "module x\n", "gomod/go.sum": "h1\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	writeVerificationWorktreeFile(t, root, "gomod/go.sum", "h2\n")
	prev := verificationLockedReverifyHook
	verificationLockedReverifyHook = func(req verificationLockedReverifyRequest) verificationLockedReverifyResult {
		if req.Runner == "go" {
			return verificationLockedReverifyResult{ExitCode: 0}
		}
		return verificationLockedReverifyResult{ExitCode: 101}
	}
	t.Cleanup(func() { verificationLockedReverifyHook = prev })
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", "."), executedRunner("go", "gomod")))
	audit := report.WorktreeAudit
	byPath := map[string]types.VerificationWorktreeEffect{}
	for _, effect := range audit.Effects {
		byPath[effect.Path] = effect
	}
	if report.Passed || byPath["Cargo.lock"].Disposition != types.VerificationWorktreeEffectRefused || byPath["gomod/go.sum"].Disposition != types.VerificationWorktreeEffectDisclosed {
		t.Fatalf("only the failing owner's row is refused: %+v", audit.Effects)
	}
	if strings.Contains(report.FailureSummary, "go.sum") || !strings.Contains(report.FailureSummary, "Cargo.lock") || audit.DisclosedTrackedEffectCount != 1 || audit.RefusedTrackedEffectCount != 1 {
		t.Fatalf("summary/counts must name the failing owner only: %q %+v", report.FailureSummary, audit)
	}
}

// A disclosed run still names its retained untracked outputs.
func TestVerificationWorktreeAuditDisclosedRunKeepsUntrackedDisclosure(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	writeVerificationWorktreeFile(t, root, "generated.bin", "artifact\n")
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, nil)
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", ".")))
	audit := report.WorktreeAudit
	if !report.Passed || audit.Status != types.VerificationWorktreeAuditTrackedDriftDisclosed || audit.UntrackedEffectCount != 1 {
		t.Fatalf("audit = %+v", audit)
	}
	summary := renderRunTestsWorktreeAuditSummary(report)
	if !strings.Contains(summary, "generated.bin") || !strings.Contains(summary, "Cargo.lock=dependency_lockfile_refresh(rust)") {
		t.Fatalf("summary must name both the disclosed row and the retained untracked output: %q", summary)
	}
	found := false
	for _, diag := range report.VerificationDiagnostics {
		found = found || (diag.Outcome == "tracked_drift_disclosed" && strings.Contains(diag.Detail, "generated.bin"))
	}
	if !found {
		t.Fatalf("disclosed diagnostic must name the untracked output: %+v", report.VerificationDiagnostics)
	}
}

// The toolchain reads the NEAREST lockfile: a same-named lockfile further up
// belongs to another project and is never owned by this runner.
func TestVerificationWorktreeAuditNearestLockfileOwnsOnly(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.toml": "[package]\n", "Cargo.lock": "root v1\n", "tools/b/Cargo.toml": "[package]\n", "tools/b/Cargo.lock": "b v1\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "JUNK written by a test in tools/b\n")
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, nil)
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", "tools/b")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("the root lockfile belongs to another project: %+v", report.WorktreeAudit)
	}
	// The runner's own (nearest) lockfile is owned.
	baseline = captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "tools/b/Cargo.lock", "b v2\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("rust", "tools/b")))
	byPath := map[string]types.VerificationWorktreeEffect{}
	for _, effect := range report.WorktreeAudit.Effects {
		byPath[effect.Path] = effect
	}
	if byPath["tools/b/Cargo.lock"].DriftClass != types.VerificationWorktreeDriftDependencyLockfileRefresh {
		t.Fatalf("the nearest lockfile must be owned: %+v", report.WorktreeAudit.Effects)
	}
}

// The roster is the launch fact, not the post-hoc verdict label; a runner
// whose output the parser could not read still refreshed its lockfile.
func TestVerificationWorktreeAuditRosterUsesLaunchedCommands(t *testing.T) {
	for _, outcome := range []string{"parser_error", "zero_tests", "timeout"} {
		root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
		baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
		writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
		stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, nil)
		report := &types.ChangeReport{Passed: false, FailureKind: types.FailureKindParserError}
		attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, types.ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: outcome, ExitCode: 1}))
		if report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftDependencyLockfileRefresh || report.FailureKind != types.FailureKindParserError {
			t.Fatalf("outcome %s: the launched runner owns its lockfile and the primary kind stays: %+v %s", outcome, report.WorktreeAudit.Effects, report.FailureKind)
		}
	}
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, types.ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: "suite_skipped"}))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("a runner that never launched owns nothing: %+v", report.WorktreeAudit)
	}
}

// Only an authoritative expected-polarity exists/equals/contains output_path
// contract declares a generated output, and only through its Subject.
func TestVerificationWorktreeAuditDeclaredOutputRequiresAuthoritativeContract(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"README.md": "old\n", "dist/secret.txt": "s\n", "gen/opt.txt": "o\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "README.md", "clobbered\n")
	writeVerificationWorktreeFile(t, root, "dist/secret.txt", "rewritten\n")
	writeVerificationWorktreeFile(t, root, "gen/opt.txt", "rewritten\n")
	plan := &types.ChangePlan{ID: "p", BehaviorContracts: []types.WriteBehaviorContract{
		{ID: "content", Kind: types.WriteBehaviorOutputPath, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpContains, Subject: "gen/out.txt", Expected: "README.md", Required: true},
		{ID: "forbidden", Kind: types.WriteBehaviorOutputPath, Polarity: types.WriteBehaviorPolarityForbidden, Operator: types.WriteBehaviorOpNotExists, Subject: "dist/secret.txt", Required: true},
		{ID: "optional", Kind: types.WriteBehaviorOutputPath, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpExists, Subject: "gen/opt.txt", Required: false},
	}}
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, plan, executedRunner("make", ".")))
	for _, effect := range report.WorktreeAudit.Effects {
		if effect.DriftClass != types.VerificationWorktreeDriftUnclassified {
			t.Fatalf("%s must not be owned by a content operand, a forbidden contract or a non-required one: %+v", effect.Path, effect)
		}
	}
	if report.Passed {
		t.Fatal("undeclared rewrites must refuse")
	}
}

// go.mod is the dependency manifest, not a lockfile; a plan target is not a class.
func TestVerificationWorktreeAuditGoModAndPlanTargetsAreNotClasses(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"go.mod": "module x\n", "go.sum": "h1\n", "src/lib.rs": "fn a() {}\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "go.mod", "module x\nreplace a => ./b\n")
	writeVerificationWorktreeFile(t, root, "src/lib.rs", "rewritten by tests\n")
	stubLockedReverify(t, verificationLockedReverifyResult{ExitCode: 0}, nil)
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, &types.ChangePlan{ID: "p", TargetPaths: []string{"src/lib.rs"}}, executedRunner("go", "."), executedRunner("rust", ".")))
	for _, effect := range report.WorktreeAudit.Effects {
		if effect.DriftClass != types.VerificationWorktreeDriftUnclassified {
			t.Fatalf("%s must be unclassified: %+v", effect.Path, effect)
		}
	}
	if report.Passed {
		t.Fatal("must refuse")
	}
}

// Formatter-lane guards: dirty at baseline, extension not owned, file outside
// the runner's directory — each alone denies the class.
func TestVerificationWorktreeAuditFormatterLaneGuards(t *testing.T) {
	prev := verificationFormatterHook
	verificationFormatterHook = func(argv []string, input []byte) ([]byte, bool) { return []byte("package x\n\nfunc a() {}\n"), true }
	t.Cleanup(func() { verificationFormatterHook = prev })
	// dirty at baseline
	root := driftRepoWithFiles(t, map[string]string{"main.go": "package x\nfunc  a(){}\n"})
	writeVerificationWorktreeFile(t, root, "main.go", "package x\nfunc  a(){} // dirty\n")
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "main.go", "package x\n\nfunc a() {}\n")
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("go", ".")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("dirty-at-baseline must deny the formatter class: %+v", report.WorktreeAudit)
	}
	// extension not owned by the runner's formatter
	root = driftRepoWithFiles(t, map[string]string{"main.rs": "fn  a(){}\n"})
	baseline = captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "main.rs", "package x\n\nfunc a() {}\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("go", ".")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("an extension the runner's formatter does not own must deny: %+v", report.WorktreeAudit)
	}
	// file outside the runner's working dir
	root = driftRepoWithFiles(t, map[string]string{"other/main.go": "package x\nfunc  a(){}\n", "svc/go.mod": "module svc\n"})
	baseline = captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "other/main.go", "package x\n\nfunc a() {}\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("go", "svc")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("a file outside the runner's directory must deny: %+v", report.WorktreeAudit)
	}
}

// The real formatter path (no hook): gofmt from the Go toolchain proves a
// pure formatting rewrite; a semantic change is refused.
func TestVerificationWorktreeAuditRealGofmtFixedPoint(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt unavailable")
	}
	root := driftRepoWithFiles(t, map[string]string{"go.mod": "module x\n", "main.go": "package x\nfunc  a(){\nreturn }\n"})
	baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "main.go", "package x\n\nfunc a() {\n\treturn\n}\n")
	report := passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("go", ".")))
	if !report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftFormatterNoSemanticDiff {
		t.Fatalf("real gofmt must prove the fixed point: %+v", report.WorktreeAudit)
	}
	baseline = captureVerificationWorktreeSnapshot(context.Background(), root)
	writeVerificationWorktreeFile(t, root, "main.go", "package x\n\nfunc a() {\n\tpanic(1)\n}\n")
	report = passingVerificationWorktreeReport()
	attachVerificationWorktreeAudit(context.Background(), report, baseline, root, driftInput(root, nil, executedRunner("go", ".")))
	if report.Passed || report.WorktreeAudit.Effects[0].DriftClass != types.VerificationWorktreeDriftUnclassified {
		t.Fatalf("a semantic change under real gofmt must refuse: %+v", report.WorktreeAudit)
	}
}

// The real locked executor (no hook): exit code and drift recurrence are
// observed from a real shell command run in the audit root.
func TestVerificationDriftExecuteLockedObservesExitAndDrift(t *testing.T) {
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	in := driftInput(root, nil)
	run := verificationDriftExecuteLocked(in)
	res := run(verificationLockedReverifyRequest{Runner: "rust", WorkingDir: ".", Command: "true", AuditRoot: root})
	if res.ExitCode != 0 || len(res.DriftedPaths) != 0 || res.Unavailable {
		t.Fatalf("clean locked run = %+v", res)
	}
	res = run(verificationLockedReverifyRequest{Runner: "rust", WorkingDir: ".", Command: "false", AuditRoot: root})
	if res.ExitCode == 0 {
		t.Fatalf("failing locked run must report a non-zero exit: %+v", res)
	}
	res = run(verificationLockedReverifyRequest{Runner: "rust", WorkingDir: ".", Command: "printf v3 > Cargo.lock", AuditRoot: root})
	if res.ExitCode != 0 || len(res.DriftedPaths) != 1 || res.DriftedPaths[0] != "Cargo.lock" {
		t.Fatalf("a re-run that rewrites the lockfile must report the recurrence: %+v", res)
	}
}

// The run_tests call site hands the gate the typed inputs the classifier
// needs (source pin: the wiring is not exercised by any unit fixture).
func TestRunTestsWiresTheDriftGateWithPlanAndRoster(t *testing.T) {
	src, err := os.ReadFile("run_tests.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"attachVerificationWorktreeAudit(context.Background(), report, worktreeAuditBaseline, ctx.RepoRoot, verificationWorktreeDriftInput{", "plan: authorityPlan", "executed: report.ExecutedCommands", "caps: verifyResourceCaps()", "timeout: runTestsDefaultTimeout()"} {
		if !strings.Contains(string(src), want) {
			t.Fatalf("run_tests.go must wire the drift gate with %q", want)
		}
	}
}
