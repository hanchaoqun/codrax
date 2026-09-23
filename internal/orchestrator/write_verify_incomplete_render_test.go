package orchestrator

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerifyIncompleteRenderExistingExecutionProjection(t *testing.T) {
	// A successful native invocation is partial evidence, not the exact-file
	// execution receipt required by the pinned request. Exercise the production
	// projection rather than manufacturing its unavailable status in the test.
	plan := &types.ChangePlan{ID: "render-execution-debt", WriteAnalysisIR: &types.WriteAnalysisIR{}}
	plan.WriteAnalysisIR.Request.Constraints = []types.WriteConstraint{{Kind: types.WriteConstraintRunExistingTest, Target: "tests/test_required.py"}}
	report := &types.ChangeReport{
		PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify,
		Passed: true, VerificationStatus: types.VerificationStatusPassed,
		ExecutedCommands: []types.ExecutedCommand{{Runner: "python", Framework: "unittest", WorkingDir: ".", Suite: "tests", Command: "python3 -m unittest discover tests", Outcome: types.ExecutedCommandOutcomeExecuted}},
		TestSurface:      &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{ID: "python@.", Runner: "python", Framework: "unittest", WorkingDir: ".", HasTestSignal: true}}},
	}
	for i := 0; i < 4; i++ {
		report.TestResults = append(report.TestResults, types.TestResult{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, Suite: "tests.test_required.Cases", AssertionID: fmt.Sprintf("test_%d", i), Passed: true})
	}
	effective := types.EffectiveExistingTestExecutionReport(plan, report)
	if effective.Passed || effective.VerificationStatus != types.VerificationStatusUnavailable || effective.FailureKind != types.FailureKindVerificationIncomplete || effective.FailureReasonCode != "required_existing_test_not_executed" {
		t.Fatalf("fixture did not reach the actual incomplete-execution projection: %+v", effective)
	}
	if reportUntriedRunnableCandidate(effective) != nil {
		t.Fatal("fixture must not be rescued by the independent untried-candidate wording")
	}
	snapshot := func() []byte {
		return b1673JSON(t, []any{plan, report, effective, types.BuildVerificationProofLedger(plan, effective, nil), types.EffectiveVerificationConfidence(plan, effective)})
	}
	before := snapshot()
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			got := renderVerifyUnverified(effective, lang)
			assertIncompleteNotEnvironment(t, got)
			for _, want := range map[string][]string{
				"zh": {"已有 4 项本地检查通过、0 项失败", "必要验证", "当前授权范围", "仍不能标记为“已验证”"},
				"en": {"4 local check(s) passed and 0 failed", "required verification", "current authorized scope", "still not verified"},
			}[lang] {
				if !strings.Contains(got, want) {
					t.Errorf("missing truthful partial-evidence disclosure %q: %s", want, got)
				}
			}
			if !bytes.Equal(before, snapshot()) {
				t.Fatal("render changed execution debt, native results, plan or verification proof")
			}
		})
	}
}

func TestVerifyIncompleteRenderUsesTypedClassNotSummaryOrSingleReason(t *testing.T) {
	for _, code := range []string{"required_existing_test_not_executed", "changed_path_verification_uncovered", "future_required_proof_missing", ""} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(code+"/"+lang, func(t *testing.T) {
				report := &types.ChangeReport{FailureKind: types.FailureKindVerificationIncomplete, VerificationStatus: types.VerificationStatusUnavailable, FailureReasonCode: code, FailureSummary: "runner_missing; missing the test runner; 环境缺失"}
				before := b1673JSON(t, report)
				got := renderVerifyUnverified(report, lang)
				assertIncompleteNotEnvironment(t, got)
				if !strings.Contains(got, "/verify") || !strings.Contains(got, "unverified") || !bytes.Equal(before, b1673JSON(t, report)) {
					t.Fatalf("typed incompleteness or report preservation lost: %s", got)
				}
			})
		}
	}
}

func TestVerifyIncompleteRenderPreservesTypedEnvironmentRefusal(t *testing.T) {
	for _, tc := range []struct {
		kind types.FailureKind
		code string
	}{
		{types.FailureKindRunnerMissing, "runner_missing"},
		{types.FailureKindParserError, "make_python_module_missing"},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(string(tc.kind)+"/"+lang, func(t *testing.T) {
				report := &types.ChangeReport{FailureKind: tc.kind, FailureReasonCode: tc.code, VerificationStatus: types.VerificationStatusUnavailable, FailureSummary: "required_existing_test_not_executed; missing verification evidence"}
				before := b1673JSON(t, report)
				got := renderVerifyUnverified(report, lang)
				wants := map[string][]string{"zh": {"缺少测试运行器或依赖", "补齐本地验证环境后 /verify"}, "en": {"missing the test runner or dependencies", "install the local verification environment and /verify"}}
				for _, want := range wants[lang] {
					if !strings.Contains(got, want) {
						t.Errorf("typed environment refusal lost %q: %s", want, got)
					}
				}
				if !bytes.Equal(before, b1673JSON(t, report)) {
					t.Fatal("environment rendering changed the typed report")
				}
			})
		}
	}
}

func assertIncompleteNotEnvironment(t *testing.T, got string) {
	t.Helper()
	for _, falseClaim := range []string{"缺少测试运行器", "missing the test runner", "补齐本地验证环境", "install the local verification environment", "环境并未缺失", "the environment itself is not missing"} {
		if strings.Contains(got, falseClaim) {
			t.Errorf("incomplete proof must not infer environment absence or health (%s): %s", falseClaim, got)
		}
	}
}
