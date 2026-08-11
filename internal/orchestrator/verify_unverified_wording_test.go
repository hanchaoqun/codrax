package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// zodShapeUnavailableReport reproduces the eval-audit 20260719 G3/GAP-5
// witness (zod_prefault_symptom run-1/run-2 final answers): the
// re-verify ran the fabricated `make make` target, the parser condemned
// verification as unavailable (make_target_missing), while the typed
// test surface still held the real, never-tried `make check` candidate.
func zodShapeUnavailableReport() *types.ChangeReport {
	return &types.ChangeReport{
		PlanID:            "plan-zod-shape",
		Passed:            false,
		FailureKind:       types.FailureKindParserError,
		FailureReasonCode: "make_target_missing",
		FailureSummary:    "[make] make target unavailable: make: *** No rule to make target `make'.  Stop.",
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:     "make",
			WorkingDir: ".",
			Suite:      "make",
			Command:    "make make",
			ExitCode:   2,
			Outcome:    "parser_error",
			ReasonCode: "make_target_missing",
			Source:     "verify_failure_handoff",
		}},
		TestSurface: &types.TestSurface{
			SelectedID: "make@.",
			Candidates: []types.TestSurfaceCandidate{{
				ID:            "make@.",
				Runner:        "make",
				WorkingDir:    ".",
				Command:       "make check",
				Source:        "Makefile",
				MakeTarget:    "check",
				HasTestSignal: true,
			}},
		},
	}
}

// TestRenderVerifyUnverifiedDoesNotLieAboutEnvironment is the sick-
// wording red pin for 件3 (G3): when a runnable candidate remains
// untried, the unverified report must say the verification COMMAND
// failed (with the real error), never that the environment lacks the
// test runner or dependencies — make and the check target both existed.
func TestRenderVerifyUnverifiedDoesNotLieAboutEnvironment(t *testing.T) {
	report := zodShapeUnavailableReport()
	if !reportIndicatesVerificationUnavailable(report) {
		t.Fatal("fixture must sit on the typed unavailable lane (witness shape)")
	}
	for _, lang := range []string{"zh", "en"} {
		got := renderVerifyUnverified(report, lang)
		if strings.Contains(got, "缺少测试运行器") || strings.Contains(got, "missing the test runner") {
			t.Fatalf("[%s] unverified wording claims a missing environment while the surface has an untried candidate (G3 lie):\n%s", lang, got)
		}
		if !strings.Contains(got, "验证命令失败") && !strings.Contains(got, "verification command failed") {
			t.Fatalf("[%s] unverified wording must state the honest command failure:\n%s", lang, got)
		}
		if !strings.Contains(got, "No rule to make target") {
			t.Fatalf("[%s] unverified wording must carry the real error:\n%s", lang, got)
		}
		if !strings.Contains(got, "make@.") {
			t.Fatalf("[%s] unverified wording should name the untried candidate:\n%s", lang, got)
		}
		if strings.Contains(got, "补齐本地验证环境") || strings.Contains(got, "install the local verification environment") {
			t.Fatalf("[%s] suggestions must not advise installing an environment that is not missing:\n%s", lang, got)
		}
	}
}

// TestRenderVerifyUnverifiedKeepsEnvironmentWordingWhenNothingUntried
// is the negative arm: when the surface candidate's own target WAS
// tried (or no runnable candidate exists), the unavailable wording is
// legitimate and must stay.
func TestRenderVerifyUnverifiedKeepsEnvironmentWordingWhenNothingUntried(t *testing.T) {
	// Arm 1: the candidate's real target ran and hit a genuine
	// environment gap (missing python module inside make).
	ran := zodShapeUnavailableReport()
	ran.FailureReasonCode = "make_python_module_missing"
	ran.FailureSummary = "[make] make target unavailable: /usr/bin/python3: No module named pytest"
	ran.ExecutedCommands[0].Command = "make check"
	ran.ExecutedCommands[0].ReasonCode = "make_python_module_missing"
	got := renderVerifyUnverified(ran, "zh")
	if !strings.Contains(got, "缺少测试运行器或依赖") {
		t.Fatalf("genuine environment gap must keep the unavailable wording:\n%s", got)
	}
	if strings.Contains(got, "验证命令失败") {
		t.Fatalf("tried-candidate environment gap must not switch to command-failed wording:\n%s", got)
	}

	// Arm 2: no runnable candidate at all (runner truly missing).
	bare := &types.ChangeReport{
		PlanID:            "plan-bare",
		Passed:            false,
		FailureKind:       types.FailureKindRunnerMissing,
		FailureReasonCode: string(types.FailureKindRunnerMissing),
		FailureSummary:    "pytest binary not found on PATH",
	}
	if !reportIndicatesVerificationUnavailable(bare) {
		t.Fatal("bare runner-missing report must sit on the unavailable lane")
	}
	got = renderVerifyUnverified(bare, "en")
	if !strings.Contains(got, "missing the test runner") {
		t.Fatalf("runner-missing with no candidates must keep the unavailable wording:\n%s", got)
	}
}

func TestRenderVerifyUnverifiedPreservesPassingPartialEvidence(t *testing.T) {
	report := &types.ChangeReport{
		PlanID:             "plan-partial",
		Passed:             false,
		VerificationStatus: types.VerificationStatusUnavailable,
		FailureKind:        types.FailureKindRunnerMissing,
		FailureReasonCode:  "verification_probe_runner_missing",
		FailureSummary:     "Java runtime unavailable",
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindUnit,
			AssertionID: "make-test",
			Suite:       "check",
			Passed:      true,
		}},
	}

	zh := renderVerifyUnverified(report, "zh")
	for _, want := range []string{"未完全验证", "已有 1 项本地检查通过、0 项失败", "局部证据保留", "Java runtime unavailable"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh partial-evidence wording missing %q:\n%s", want, zh)
		}
	}
	if strings.Contains(zh, "没有断言验证过") {
		t.Fatalf("zh partial-evidence wording must not erase a passing result:\n%s", zh)
	}

	en := renderVerifyUnverified(report, "en")
	for _, want := range []string{"Partially verified (unverified)", "1 local check(s) passed and 0 failed", "useful partial evidence", "Java runtime unavailable"} {
		if !strings.Contains(en, want) {
			t.Fatalf("en partial-evidence wording missing %q:\n%s", want, en)
		}
	}
	if strings.Contains(en, "no assertion verified") {
		t.Fatalf("en partial-evidence wording must not erase a passing result:\n%s", en)
	}
}

// TestReportUntriedRunnableCandidateShapes pins the precise-signal
// helper directly: candidate-never-ran and make-target-mismatch both
// count as untried; a run of the candidate's own target does not.
func TestReportUntriedRunnableCandidateShapes(t *testing.T) {
	report := zodShapeUnavailableReport()
	cand := reportUntriedRunnableCandidate(report)
	if cand == nil || cand.ID != "make@." {
		t.Fatalf("make-target-mismatch must count as untried; got %+v", cand)
	}
	report.ExecutedCommands = append(report.ExecutedCommands, types.ExecutedCommand{
		Runner: "make", WorkingDir: ".", Command: "make check", Outcome: "executed", ExitCode: 2,
	})
	if got := reportUntriedRunnableCandidate(report); got != nil {
		t.Fatalf("candidate's own target ran — nothing untried; got %+v", got)
	}
	// A never-executed second candidate is untried regardless of runner.
	report.TestSurface.Candidates = append(report.TestSurface.Candidates, types.TestSurfaceCandidate{
		ID: "go@sub", Runner: "go", WorkingDir: "sub", HasTestSignal: true,
	})
	if got := reportUntriedRunnableCandidate(report); got == nil || got.ID != "go@sub" {
		t.Fatalf("never-executed candidate must count as untried; got %+v", got)
	}
	// Synthetic rows do not count as trying a candidate.
	report.ExecutedCommands = append(report.ExecutedCommands, types.ExecutedCommand{
		Runner: "go", WorkingDir: "sub", Outcome: "synthetic_no_tests",
	})
	if got := reportUntriedRunnableCandidate(report); got == nil || got.ID != "go@sub" {
		t.Fatalf("synthetic_no_tests must not count as a try; got %+v", got)
	}
	if got := executedMakeCommandTarget("make check"); got != "check" {
		t.Fatalf("executedMakeCommandTarget = %q, want check", got)
	}
	if got := executedMakeCommandTarget("ctest --test-dir build"); got != "" {
		t.Fatalf("executedMakeCommandTarget non-make = %q, want empty", got)
	}
	// Negative-arm status-quo pin (rework P3-②): `make -C <dir> <target>`
	// is NOT understood — the first non-flag token wins, so the -C
	// argument is mistaken for the target. Unreachable today because
	// buildRunCommand renders exactly "make <target>"; if a -C shape
	// ever becomes reachable, this extractor (and its twin
	// makeTargetFromCommand in internal/tool) must learn flag arity
	// first. This pin documents the limitation, it does not bless it.
	if got := executedMakeCommandTarget("make -C sub check"); got != "sub" {
		t.Fatalf("executedMakeCommandTarget make -C status quo = %q, want sub (see comment)", got)
	}
}

// TestReportUntriedRunnableCandidateRunnerMissingIsNotUntried is the
// rework P1-1 red pin (reverse lie): when the candidate's key holds a
// typed environment-refusal row (runner_missing / not_configured), the
// candidate must NOT count as untried — pre-fix those rows were
// silently dropped, the candidate read as untried, and the wording
// asserted "the environment is not missing" while the runner binary
// was genuinely absent.
func TestReportUntriedRunnableCandidateRunnerMissingIsNotUntried(t *testing.T) {
	// Sick shape 1: the ONLY candidate's runner binary is missing.
	report := &types.ChangeReport{
		PlanID:            "plan-runner-gone",
		Passed:            false,
		FailureKind:       types.FailureKindRunnerMissing,
		FailureReasonCode: string(types.FailureKindRunnerMissing),
		FailureSummary:    "cargo binary not found on PATH",
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "cargo", WorkingDir: ".", Outcome: "runner_missing",
		}},
		TestSurface: &types.TestSurface{
			SelectedID: "cargo@.",
			Candidates: []types.TestSurfaceCandidate{{
				ID: "cargo@.", Runner: "cargo", WorkingDir: ".", HasTestSignal: true,
			}},
		},
	}
	if got := reportUntriedRunnableCandidate(report); got != nil {
		t.Fatalf("runner_missing at the candidate's key must disqualify it from untried; got %+v", got)
	}
	if !reportIndicatesVerificationUnavailable(report) {
		t.Fatal("fixture must sit on the typed unavailable lane")
	}
	for _, lang := range []string{"zh", "en"} {
		got := renderVerifyUnverified(report, lang)
		if strings.Contains(got, "并未缺失") || strings.Contains(got, "is not missing") {
			t.Fatalf("[%s] wording claims the environment is intact while the runner is missing (reverse lie):\n%s", lang, got)
		}
		if !strings.Contains(got, "缺少测试运行器") && !strings.Contains(got, "missing the test runner") {
			t.Fatalf("[%s] genuine runner gap must keep the honest environment wording:\n%s", lang, got)
		}
	}

	// not_configured sits on the same environment-refusal lane.
	report.ExecutedCommands[0].Outcome = "not_configured"
	if got := reportUntriedRunnableCandidate(report); got != nil {
		t.Fatalf("not_configured must disqualify the candidate from untried; got %+v", got)
	}

	// Sick shape 2 (multi-candidate escalation wash): candidate A ran its
	// own target; candidate B's runner is missing. B must not resurface
	// as "untried" and wash the real environment gap.
	multi := &types.ChangeReport{
		PlanID:            "plan-escalated",
		Passed:            false,
		FailureKind:       types.FailureKindRunnerMissing,
		FailureReasonCode: string(types.FailureKindRunnerMissing),
		FailureSummary:    "pytest binary not found on PATH",
		ExecutedCommands: []types.ExecutedCommand{
			{Runner: "make", WorkingDir: ".", Command: "make check", ExitCode: 2, Outcome: "executed"},
			{Runner: "python", WorkingDir: ".", Outcome: "runner_missing"},
		},
		TestSurface: &types.TestSurface{
			SelectedID: "make@.",
			Candidates: []types.TestSurfaceCandidate{
				{ID: "make@.", Runner: "make", WorkingDir: ".", Command: "make check", MakeTarget: "check", HasTestSignal: true},
				{ID: "python@.", Runner: "python", WorkingDir: ".", HasTestSignal: true},
			},
		},
	}
	if got := reportUntriedRunnableCandidate(multi); got != nil {
		t.Fatalf("escalation shape: tried A + runner-missing B leaves nothing untried; got %+v", got)
	}
	got := renderVerifyUnverified(multi, "zh")
	if strings.Contains(got, "并未缺失") {
		t.Fatalf("escalation shape must not wash the real environment gap:\n%s", got)
	}
}

// TestReportUntriedRunnableCandidateUnknownOutcomeIsConservative pins
// the default lane (rework P1-1): an Outcome value this helper does not
// recognise neither counts as a try NOR licences the "environment is
// not missing" claim — the candidate simply drops out of the untried
// wording.
func TestReportUntriedRunnableCandidateUnknownOutcomeIsConservative(t *testing.T) {
	report := &types.ChangeReport{
		PlanID: "plan-unknown-outcome",
		Passed: false,
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "go", WorkingDir: ".", Outcome: "some_future_outcome",
		}},
		TestSurface: &types.TestSurface{
			SelectedID: "go@.",
			Candidates: []types.TestSurfaceCandidate{{
				ID: "go@.", Runner: "go", WorkingDir: ".", HasTestSignal: true,
			}},
		},
	}
	if got := reportUntriedRunnableCandidate(report); got != nil {
		t.Fatalf("unknown Outcome must not feed the untried claim; got %+v", got)
	}
	// The witness lane must NOT regress: parser_error still counts as a
	// genuine try (zod shape: `make make` ran and the parser condemned
	// it) so the target-mismatch untried claim keeps firing.
	zod := zodShapeUnavailableReport()
	if got := reportUntriedRunnableCandidate(zod); got == nil || got.ID != "make@." {
		t.Fatalf("parser_error row must still count as ran (witness shape); got %+v", got)
	}
	// 复审 F-N1 arm: the default lane must ALSO disqualify on the
	// make-target-mismatch path. A make candidate whose only executed row
	// carries an unknown Outcome AND a mismatched command (`make make`)
	// must NOT surface as untried — otherwise a future Outcome enum value
	// would silently re-open the reverse-lie (mutation: default→ran flips
	// this to a non-nil candidate).
	makeUnknown := &types.ChangeReport{
		PlanID: "plan-unknown-make",
		Passed: false,
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "make", WorkingDir: ".", Command: "make make",
			Outcome: "some_future_outcome",
		}},
		TestSurface: &types.TestSurface{
			SelectedID: "make@.",
			Candidates: []types.TestSurfaceCandidate{{
				ID: "make@.", Runner: "make", WorkingDir: ".",
				HasTestSignal: true, MakeTarget: "check",
			}},
		},
	}
	if got := reportUntriedRunnableCandidate(makeUnknown); got != nil {
		t.Fatalf("unknown Outcome on the make-mismatch path must stay conservative; got %+v", got)
	}
}

// TestRenderSessionAppliedRefsLine pins the multi-plan landing guidance
// (件3 companion, audit G3: the zod final answer pointed only at the
// last plan's ref).
func TestRenderSessionAppliedRefsLine(t *testing.T) {
	if got := renderSessionAppliedRefsLine(nil, true); got != "" {
		t.Fatalf("no refs → no line; got %q", got)
	}
	if got := renderSessionAppliedRefsLine([]string{"refs/codrax/applied/a"}, true); got != "" {
		t.Fatalf("single ref → no line; got %q", got)
	}
	got := renderSessionAppliedRefsLine([]string{"refs/codrax/applied/a", "refs/codrax/applied/b"}, true)
	ia := strings.Index(got, "refs/codrax/applied/a")
	ib := strings.Index(got, "refs/codrax/applied/b")
	if ia < 0 || ib < 0 || ia > ib {
		t.Fatalf("multi-ref line must list refs in apply order:\n%s", got)
	}
	if !strings.Contains(got, "按顺序") {
		t.Fatalf("zh multi-ref line must instruct ordered cherry-pick:\n%s", got)
	}
	en := renderSessionAppliedRefsLine([]string{"refs/codrax/applied/a", "refs/codrax/applied/b"}, false)
	if !strings.Contains(en, "in order") {
		t.Fatalf("en multi-ref line must instruct ordered cherry-pick:\n%s", en)
	}
}
