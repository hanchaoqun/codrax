package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestChangedPathCoverageAcceptsSameLanguageProjectRunner(t *testing.T) {
	ctx := changedPathCoverageTestContext([]string{"src/main/java/example/Widget.java"})
	report := &types.ChangeReport{
		Passed: true,
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:     "java",
			WorkingDir: ".",
			Outcome:    "executed",
			ExitCode:   0,
			Source:     "test_surface_default",
		}},
	}

	applyChangedPathVerificationCoverage(ctx, report)

	if !report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		t.Fatalf("same-language project runner should cover Java path: %+v", report)
	}
	if len(report.ChangedPathCoverage) != 1 ||
		report.ChangedPathCoverage[0].Status != types.ChangedPathVerificationCovered ||
		report.ChangedPathCoverage[0].Caliber != types.ChangedPathVerificationProjectRunner {
		t.Fatalf("project runner coverage missing: %+v", report.ChangedPathCoverage)
	}
	if got := report.ExecutedCommands[0].CoveredPaths; len(got) != 1 || got[0] != "src/main/java/example/Widget.java" {
		t.Fatalf("command exact covered_paths missing: %+v", got)
	}
}

func TestChangedPathCoverageRejectsCrossLanguageSuccess(t *testing.T) {
	ctx := changedPathCoverageTestContext([]string{
		"src/main/java/example/Widget.java",
		"tools/check.py",
	})
	report := &types.ChangeReport{
		Passed: true,
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:     "python",
			WorkingDir: ".",
			Outcome:    "executed",
			ExitCode:   0,
			Source:     "test_surface_default",
		}},
	}

	applyChangedPathVerificationCoverage(ctx, report)

	if report.Passed ||
		report.FailureKind != types.FailureKindVerificationIncomplete ||
		report.FailureReasonCode != changedPathVerificationUncoveredReasonCode ||
		report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
		t.Fatalf("Python success must not authorize Java path: %+v", report)
	}
	if len(report.ChangedPathCoverage) != 2 {
		t.Fatalf("coverage rows=%d, want 2: %+v", len(report.ChangedPathCoverage), report.ChangedPathCoverage)
	}
	if report.ChangedPathCoverage[0].Status != types.ChangedPathVerificationUncovered ||
		report.ChangedPathCoverage[1].Status != types.ChangedPathVerificationCovered {
		t.Fatalf("mixed-language per-path verdict wrong: %+v", report.ChangedPathCoverage)
	}
}

func TestChangedPathCoverageAcceptsExactSameLanguageSourceCheck(t *testing.T) {
	ctx := changedPathCoverageTestContext([]string{"pkg/client.py"})
	report := &types.ChangeReport{
		Passed: true,
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:       "python",
			Outcome:      "syntax_check_fallback",
			ExitCode:     0,
			CoveredPaths: []string{"pkg/client.py"},
		}},
	}

	applyChangedPathVerificationCoverage(ctx, report)

	if !report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		t.Fatalf("exact same-language source check should cover path: %+v", report)
	}
	if report.ChangedPathCoverage[0].Caliber != types.ChangedPathVerificationSourceCheck {
		t.Fatalf("source-check caliber missing: %+v", report.ChangedPathCoverage)
	}
}

func TestChangedPathCoverageRejectsCrossLanguageClaimedSourceCheck(t *testing.T) {
	ctx := changedPathCoverageTestContext([]string{"src/Widget.java"})
	report := &types.ChangeReport{
		Passed: true,
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:       "python",
			Outcome:      "syntax_check_fallback",
			ExitCode:     0,
			CoveredPaths: []string{"src/Widget.java"},
		}},
	}

	applyChangedPathVerificationCoverage(ctx, report)

	if report.Passed || report.ChangedPathCoverage[0].Status != types.ChangedPathVerificationUncovered {
		t.Fatalf("cross-language source-check claim must fail closed: %+v", report)
	}
}

func TestChangedPathCoverageAcceptsPathBoundSameLanguageProbe(t *testing.T) {
	ctx := changedPathCoverageTestContext([]string{"widget.py"})
	plan := ctx.Mutable.ChangePlan()
	plan.VerificationProbes = []types.VerificationProbe{{
		ID:                "widget_value",
		Language:          "python",
		ChangedSymbolRefs: []string{"path:widget.py"},
	}}
	ctx.Mutable.SetChangePlan(plan)
	report := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{{
			AssertionID: "widget_value",
			Suite:       "verification_probe/python",
			Passed:      true,
		}},
	}

	applyChangedPathVerificationCoverage(ctx, report)

	if !report.Passed ||
		len(report.ChangedPathCoverage) != 1 ||
		report.ChangedPathCoverage[0].Caliber != types.ChangedPathVerificationProbe {
		t.Fatalf("path-bound same-language probe should cover path: %+v", report)
	}
}

func TestChangedPathCoverageIgnoresConfigOnlyTargets(t *testing.T) {
	ctx := changedPathCoverageTestContext([]string{"pom.xml", ".github/workflows/ci.yml"})
	report := &types.ChangeReport{Passed: true}

	applyChangedPathVerificationCoverage(ctx, report)

	if !report.Passed || len(report.ChangedPathCoverage) != 0 {
		t.Fatalf("config-only plans are outside source-path coverage gate: %+v", report)
	}
}

func changedPathCoverageTestContext(paths []string) *types.BusContext {
	mu := types.NewMutableState("changed-path-coverage")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-changed-path-coverage",
		TargetPaths: append([]string(nil), paths...),
	})
	return &types.BusContext{
		Mutable:      mu,
		RepoRoot:     "/repo",
		MainRepoRoot: "/repo",
	}
}
