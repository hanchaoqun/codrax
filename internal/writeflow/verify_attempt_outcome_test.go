package writeflow

import (
	"errors"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestClassifyVerifyAttemptOutcome(t *testing.T) {
	cases := []struct {
		name       string
		report     *types.ChangeReport
		err        error
		wantKind   VerifyAttemptOutcomeKind
		wantRetry  bool
		wantAction WorkflowAction
		wantReason string
	}{
		{
			name:       "missing report executor error retries verify",
			err:        errors.New("executor failed"),
			wantKind:   VerifyOutcomeToolNotCalled,
			wantRetry:  true,
			wantAction: ActionVerifyBatch,
			wantReason: "verify_tool_not_called",
		},
		{
			name:       "runner missing finishes unverified",
			report:     &types.ChangeReport{FailureKind: types.FailureKindRunnerMissing},
			wantKind:   VerifyOutcomeRunnerMissing,
			wantAction: ActionFinish,
			wantReason: "runner_missing",
		},
		{
			name:       "parser error finishes unverified",
			report:     &types.ChangeReport{FailureKind: types.FailureKindParserError},
			wantKind:   VerifyOutcomeParserError,
			wantAction: ActionFinish,
			wantReason: "parser_error",
		},
		{
			name:       "failed report replans",
			report:     &types.ChangeReport{Passed: false, BuildFailed: true},
			wantKind:   VerifyOutcomeReportFailed,
			wantAction: ActionReplanBatch,
			wantReason: "build_failed",
		},
		{
			name:       "no tests finish with caveat",
			report:     &types.ChangeReport{Passed: true, NoTestsRunners: []string{"python"}},
			wantKind:   VerifyOutcomeNoTests,
			wantAction: ActionFinish,
			wantReason: "no_tests",
		},
		{
			name:       "legacy no tests false report still finishes",
			report:     &types.ChangeReport{Passed: false, NoTestsRunners: []string{"python"}},
			wantKind:   VerifyOutcomeNoTests,
			wantAction: ActionFinish,
			wantReason: "no_tests",
		},
		{
			name:       "passed report finishes",
			report:     &types.ChangeReport{Passed: true},
			wantKind:   VerifyOutcomeReportPassed,
			wantAction: ActionFinish,
			wantReason: "tests_passed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyVerifyAttemptOutcome(tc.report, tc.err)
			if got.Kind != tc.wantKind || got.Retryable != tc.wantRetry ||
				got.RecommendedAction != tc.wantAction || got.ReasonCode != tc.wantReason {
				t.Fatalf("outcome = %+v; want kind=%s retry=%v action=%s reason=%s",
					got, tc.wantKind, tc.wantRetry, tc.wantAction, tc.wantReason)
			}
		})
	}
}
