package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestShouldSuppressVerifyRetry covers the predicate that gates the
// verify→plan retry cycle. Locks the contract: only
// FailureKindRunnerMissing suppresses retry; every other failure
// kind (tests_failed / build_failure / oom / cpu_limit / timeout /
// crash) falls through to the budget check.
func TestShouldSuppressVerifyRetry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		report  *types.ChangeReport
		suppress bool
	}{
		{
			name:     "nil report — apply error path, defer to budget",
			report:   nil,
			suppress: false,
		},
		{
			name:     "tests_failed — real red tests, retry helps",
			report:   &types.ChangeReport{Passed: false, FailureKind: types.FailureKindTestsFailed},
			suppress: false,
		},
		{
			name:     "build_failure — compile broke, planner can fix",
			report:   &types.ChangeReport{Passed: false, FailureKind: types.FailureKindBuildFailure},
			suppress: false,
		},
		{
			name:     "oom — code allocated unboundedly, planner can fix",
			report:   &types.ChangeReport{Passed: false, FailureKind: types.FailureKindOOM},
			suppress: false,
		},
		{
			name:     "cpu_limit — infinite loop, planner can fix",
			report:   &types.ChangeReport{Passed: false, FailureKind: types.FailureKindCPULimit},
			suppress: false,
		},
		{
			name:     "timeout — wall-clock fired, retry could help",
			report:   &types.ChangeReport{Passed: false, FailureKind: types.FailureKindTimeout},
			suppress: false,
		},
		{
			name:     "crash — unexpected exit, retry could help",
			report:   &types.ChangeReport{Passed: false, FailureKind: types.FailureKindCrash},
			suppress: false,
		},
		{
			name:     "runner_missing — env issue, retry burns budget",
			report:   &types.ChangeReport{Passed: false, FailureKind: types.FailureKindRunnerMissing},
			suppress: true,
		},
		{
			name:     "passed — successful verify, never retries",
			report:   &types.ChangeReport{Passed: true},
			suppress: false,
		},
	} {
		got := shouldSuppressVerifyRetry(tc.report)
		if got != tc.suppress {
			t.Errorf("%s: shouldSuppressVerifyRetry = %v, want %v",
				tc.name, got, tc.suppress)
		}
	}
}
