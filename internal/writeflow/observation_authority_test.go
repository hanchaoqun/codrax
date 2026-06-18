package writeflow

import (
	"errors"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDeriveObservationAuthorityFromReport(t *testing.T) {
	cases := []struct {
		name       string
		report     *types.ChangeReport
		err        error
		wantState  ObservationAuthorityState
		wantAction WorkflowAction
		wantReason string
		wantFinish bool
	}{
		{
			name:       "missing report requires observe retry",
			err:        errors.New("tool failed"),
			wantState:  ObservationAuthorityMissing,
			wantAction: ActionVerifyBatch,
			wantReason: "verify_tool_not_called",
		},
		{
			name:       "runner missing is unverified not code failure",
			report:     &types.ChangeReport{FailureKind: types.FailureKindRunnerMissing},
			wantState:  ObservationAuthorityUnverified,
			wantAction: ActionFinish,
			wantReason: "runner_missing",
			wantFinish: true,
		},
		{
			name:       "no tests is unverified",
			report:     &types.ChangeReport{Passed: true, NoTestsRunners: []string{"pytest"}},
			wantState:  ObservationAuthorityUnverified,
			wantAction: ActionFinish,
			wantReason: "no_tests",
			wantFinish: true,
		},
		{
			name: "red tests outrank secondary no-tests evidence",
			report: &types.ChangeReport{
				Passed:         false,
				FailureKind:    types.FailureKindTestsFailed,
				NoTestsRunners: []string{"make"},
				TestResults: []types.TestResult{{
					Kind:        types.TestResultKindUnit,
					AssertionID: "TestWidget",
					Suite:       "python",
					Passed:      false,
				}},
			},
			wantState:  ObservationAuthorityFailed,
			wantAction: ActionReplanBatch,
			wantReason: "tests_failed",
		},
		{
			name:       "build failure requires replan",
			report:     &types.ChangeReport{Passed: false, BuildFailed: true},
			wantState:  ObservationAuthorityFailed,
			wantAction: ActionReplanBatch,
			wantReason: "build_failed",
		},
		{
			name: "build surface with verifier unavailable reason can finish unverified",
			report: &types.ChangeReport{
				Passed:            false,
				BuildFailed:       true,
				FailureKind:       types.FailureKindBuildFailure,
				FailureReasonCode: "verification_probe_module_not_found",
			},
			wantState:  ObservationAuthorityUnverified,
			wantAction: ActionFinish,
			wantReason: "verification_probe_module_not_found",
			wantFinish: true,
		},
		{
			name:       "passed report can finish",
			report:     &types.ChangeReport{Passed: true},
			wantState:  ObservationAuthorityVerified,
			wantAction: ActionFinish,
			wantReason: "tests_passed",
			wantFinish: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveObservationAuthorityFromReport(tc.report, tc.err)
			if got.State != tc.wantState || got.RecommendedAction != tc.wantAction ||
				got.ReasonCode != tc.wantReason || got.FinishAllowed != tc.wantFinish {
				t.Fatalf("authority = %+v; want state=%s action=%s reason=%s finish=%v",
					got, tc.wantState, tc.wantAction, tc.wantReason, tc.wantFinish)
			}
		})
	}
}

func TestDeriveObservationAuthorityFromAttempt(t *testing.T) {
	cases := []struct {
		name                string
		attempt             *types.WriteWorkflowAttempt
		wantState           ObservationAuthorityState
		wantReason          string
		wantFinish          bool
		wantDispositionOnly bool
		wantReplan          bool
		wantObserve         bool
	}{
		{
			name:        "nil attempt requires observe",
			wantState:   ObservationAuthorityMissing,
			wantReason:  "missing_verify_attempt",
			wantObserve: true,
		},
		{
			name:       "passed attempt can finish",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "passed", ReasonCode: "tests_passed"},
			wantState:  ObservationAuthorityVerified,
			wantReason: "tests_passed",
			wantFinish: true,
		},
		{
			name:       "unverified unavailable can finish with caveat",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "unverified", ReasonCode: "parser_error"},
			wantState:  ObservationAuthorityUnverified,
			wantReason: "parser_error",
			wantFinish: true,
		},
		{
			name:       "probe import unavailable can finish with caveat",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "unverified", ReasonCode: "verification_probe_module_not_found"},
			wantState:  ObservationAuthorityUnverified,
			wantReason: "verification_probe_module_not_found",
			wantFinish: true,
		},
		{
			name:       "make target missing unavailable can finish with caveat",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "unverified", ReasonCode: "make_target_missing"},
			wantState:  ObservationAuthorityUnverified,
			wantReason: "make_target_missing",
			wantFinish: true,
		},
		{
			name:       "make python dependency missing unavailable can finish with caveat",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "unverified", ReasonCode: "make_python_module_missing"},
			wantState:  ObservationAuthorityUnverified,
			wantReason: "make_python_module_missing",
			wantFinish: true,
		},
		{
			name:                "legacy failed unavailable needs typed disposition",
			attempt:             &types.WriteWorkflowAttempt{Kind: "verify", Status: "failed", ReasonCode: "runner_missing"},
			wantState:           ObservationAuthorityUnverified,
			wantReason:          "runner_missing",
			wantDispositionOnly: true,
		},
		{
			name:                "legacy failed probe import unavailable needs typed disposition",
			attempt:             &types.WriteWorkflowAttempt{Kind: "verify", Status: "failed", ReasonCode: "verification_probe_import_error"},
			wantState:           ObservationAuthorityUnverified,
			wantReason:          "verification_probe_import_error",
			wantDispositionOnly: true,
		},
		{
			name:        "code failure requires replan",
			attempt:     &types.WriteWorkflowAttempt{Kind: "verify", Status: "failed", ReasonCode: "tests_failed"},
			wantState:   ObservationAuthorityFailed,
			wantReason:  "tests_failed",
			wantReplan:  true,
			wantObserve: false,
		},
		{
			name:        "inconsistent unverified code failure stays failed",
			attempt:     &types.WriteWorkflowAttempt{Kind: "verify", Status: "unverified", ReasonCode: "tests_failed"},
			wantState:   ObservationAuthorityFailed,
			wantReason:  "tests_failed",
			wantReplan:  true,
			wantObserve: false,
		},
		{
			name:        "infra error requires observe retry",
			attempt:     &types.WriteWorkflowAttempt{Kind: "verify", Status: "infra_error", ReasonCode: "verify_tool_not_called"},
			wantState:   ObservationAuthorityMissing,
			wantReason:  "verify_tool_not_called",
			wantObserve: true,
		},
		{
			name:       "explicit skip verify is unverified",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "skipped", ReasonCode: "skip_verify"},
			wantState:  ObservationAuthorityUnverified,
			wantReason: "skip_verify",
			wantFinish: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveObservationAuthorityFromAttempt(tc.attempt)
			if got.State != tc.wantState || got.ReasonCode != tc.wantReason ||
				got.FinishAllowed != tc.wantFinish ||
				got.FinishRequiresDisposition != tc.wantDispositionOnly ||
				got.MustReplan != tc.wantReplan ||
				got.MustObserve != tc.wantObserve {
				t.Fatalf("authority = %+v; want state=%s reason=%s finish=%v disposition=%v replan=%v observe=%v",
					got, tc.wantState, tc.wantReason, tc.wantFinish, tc.wantDispositionOnly, tc.wantReplan, tc.wantObserve)
			}
		})
	}
}
