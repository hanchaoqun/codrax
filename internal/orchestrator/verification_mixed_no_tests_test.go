package orchestrator

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestMixedNoTestsConsumersShareReportStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report types.ChangeReport
		want   string
	}{
		{"native_pass", types.ChangeReport{Passed: true, TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, AssertionID: "test_widget", Suite: "nested.WidgetTest", Passed: true}}}, types.PlanStatusApplied},
		{"legacy_native_fail", types.ChangeReport{Passed: false, FailureKind: types.FailureKindTestsFailed, TestResults: []types.TestResult{{AssertionID: "test_widget", Suite: "nested.WidgetTest", Passed: false}}}, types.PlanStatusVerifyFailed},
		{"build_fail", types.ChangeReport{Passed: false, BuildFailed: true, FailureKind: types.FailureKindBuildFailure}, types.PlanStatusVerifyFailed},
		{"timeout", types.ChangeReport{Passed: false, FailureKind: types.FailureKindTimeout}, types.PlanStatusVerifyFailed},
		{"uncovered_path", types.ChangeReport{Passed: false, FailureKind: types.FailureKindVerificationIncomplete, FailureReasonCode: "changed_path_verification_uncovered"}, types.PlanStatusUnverified},
		{"runner_missing", types.ChangeReport{Passed: false, FailureKind: types.FailureKindRunnerMissing}, types.PlanStatusUnverified},
		{"legacy_empty", types.ChangeReport{Passed: false, FailureKind: types.FailureKindTestsFailed}, types.PlanStatusUnverified},
		{"plain_probe", types.ChangeReport{Passed: true, TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, Suite: "verification_probe/python", AssertionID: "probe", Passed: true}}}, types.PlanStatusUnverified},
	} {
		for _, lane := range []string{"controller_sync", "stage_post_hook"} {
			t.Run(tc.name+"/"+lane, func(t *testing.T) {
				dir := t.TempDir()
				plan := &types.ChangePlan{ID: "mixed", Status: types.PlanStatusAppliedPendingVerify, Changes: []types.FileChange{{Path: "packages/widget/widget.py", Kind: "modify"}}}
				planPath := filepath.Join(dir, "mixed.json")
				if err := types.WritePlanToFile(plan, planPath); err != nil {
					t.Fatal(err)
				}
				r := tc.report
				r.PlanID, r.Channel = plan.ID, types.ChangeReportChannelPostApplyVerify
				r.NoTestsRunners = []string{"python"}
				r.ExecutedCommands = append(r.ExecutedCommands, types.ExecutedCommand{Runner: "python", Framework: "unittest", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeZeroTests})
				mu := types.NewMutableState("mixed no tests")
				mu.SetChangePlan(plan)
				mu.SetChangeReport(&r)
				o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, PlanPath: planPath, WorkDir: dir, Language: "en"}}
				var verifyErr error
				out := &agent.StageOutput{}
				if tc.want == types.PlanStatusVerifyFailed {
					verifyErr = errors.New("executor failed")
					out.Error = verifyErr.Error()
				}
				if lane == "controller_sync" {
					o.syncMutablePlanStatusAfterVerify(&r, verifyErr)
				} else if err := verifyPostHook(o, out); err != nil {
					t.Fatal(err)
				}
				if got := mu.ChangePlan().Status; got != tc.want {
					t.Fatalf("status=%s want=%s", got, tc.want)
				}
				persisted, err := types.LoadChangePlanFromFile(planPath)
				if err != nil || persisted.Status != tc.want {
					t.Fatalf("persisted=%+v err=%v want=%s", persisted, err, tc.want)
				}
				if got := shouldSuppressVerifyRetry(&r); got != (tc.want == types.PlanStatusUnverified) {
					t.Fatalf("suppressRetry=%t want=%t", got, tc.want == types.PlanStatusUnverified)
				}
				authority := writeflow.DeriveObservationAuthorityFromReport(&r, verifyErr)
				wantAuthority := writeflow.ObservationAuthorityVerified
				if tc.want == types.PlanStatusUnverified {
					wantAuthority = writeflow.ObservationAuthorityUnverified
				}
				if tc.want == types.PlanStatusVerifyFailed {
					wantAuthority = writeflow.ObservationAuthorityFailed
				}
				if authority.State != wantAuthority {
					t.Fatalf("authority=%+v want=%s", authority, wantAuthority)
				}
			})
		}
	}
}

func TestMixedNoTestsUnverifiedRenderingDoesNotErasePassedAssertions(t *testing.T) {
	r := &types.ChangeReport{Passed: true, NoTestsRunners: []string{"python"}, TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, AssertionID: "test_widget", Suite: "nested.WidgetTest", Passed: true}}, ExecutedCommands: []types.ExecutedCommand{{Runner: "python", Framework: "unittest", WorkingDir: ".", Outcome: types.ExecutedCommandOutcomeZeroTests}}}
	for _, lang := range []string{"zh", "en"} {
		text := renderVerifyUnverified(r, lang)
		for _, falseClaim := range []string{"没有发现任何测试", "discovered zero tests for the changed code", "missing the test runner", "install the local verification environment", "补齐本地验证环境"} {
			if strings.Contains(text, falseClaim) {
				t.Fatalf("%s erased native assertions: %s", lang, text)
			}
		}
	}
	r.FailureKind, r.FailureReasonCode = types.FailureKindVerificationIncomplete, "changed_path_verification_uncovered"
	for _, lang := range []string{"zh", "en"} {
		text := renderVerifyUnverified(r, lang)
		for _, falseClaim := range []string{"缺少测试运行器", "missing the test runner", "install the local verification environment", "补齐本地验证环境"} {
			if strings.Contains(text, falseClaim) {
				t.Fatalf("coverage gap is not environment absence: %s", text)
			}
		}
	}
	r.FailureKind, r.FailureReasonCode = "", ""
	r.FailureKind = types.FailureKindNoTests
	for _, lang := range []string{"zh", "en"} {
		text := renderVerifyUnverified(r, lang)
		if strings.Contains(text, "尚未获得原生测试断言") || strings.Contains(text, "no native test assertion was obtained") {
			t.Fatalf("explicit unavailability erased independent passed assertions: %s", text)
		}
	}
	r.NoTestsRunners = nil
	if text := renderVerifyUnverified(r, "en"); !strings.Contains(text, "scope not recorded") {
		t.Fatalf("explicit no-tests invented or omitted unavailable invocation scope: %s", text)
	}
	r.NoTestsRunners, r.FailureKind = []string{"python"}, ""
	r.TestResults = nil
	text := renderVerifyUnverified(r, "en")
	if !strings.Contains(text, "python/unittest@.") || strings.Contains(text, "missing the test runner") {
		t.Fatalf("zero tests must name actual invocation without inventing environment absence: %s", text)
	}
}
