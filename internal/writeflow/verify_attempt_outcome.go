package writeflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// VerifyAttemptOutcomeKind is the typed scheduler-facing result of one
// post-apply verify executor attempt. It is derived from ChangeReport presence
// and typed report fields; model prose and user text never affect it.
type VerifyAttemptOutcomeKind string

const (
	VerifyOutcomeReportPassed  VerifyAttemptOutcomeKind = "report_passed"
	VerifyOutcomeReportFailed  VerifyAttemptOutcomeKind = "report_failed"
	VerifyOutcomeToolNotCalled VerifyAttemptOutcomeKind = "tool_not_called"
	VerifyOutcomeRunnerMissing VerifyAttemptOutcomeKind = "runner_missing"
	VerifyOutcomeNoTests       VerifyAttemptOutcomeKind = "no_tests"
)

// VerifyAttemptOutcome tells the controller scheduler whether an executor
// failure should retry verification or return to planning/blocking.
type VerifyAttemptOutcome struct {
	Kind              VerifyAttemptOutcomeKind
	Retryable         bool
	RecommendedAction WorkflowAction
	ReasonCode        string
}

// ClassifyVerifyAttemptOutcome converts typed verify artifacts into a scheduler
// outcome. The err argument only signals that the executor failed; its message
// is not parsed.
func ClassifyVerifyAttemptOutcome(report *types.ChangeReport, err error) VerifyAttemptOutcome {
	if report == nil {
		return VerifyAttemptOutcome{
			Kind:              VerifyOutcomeToolNotCalled,
			Retryable:         true,
			RecommendedAction: ActionVerifyBatch,
			ReasonCode:        "verify_tool_not_called",
		}
	}
	if report.FailureKind == types.FailureKindRunnerMissing {
		return VerifyAttemptOutcome{
			Kind:              VerifyOutcomeRunnerMissing,
			Retryable:         false,
			RecommendedAction: ActionFinish,
			ReasonCode:        string(types.FailureKindRunnerMissing),
		}
	}
	if err != nil || !report.Passed {
		return VerifyAttemptOutcome{
			Kind:              VerifyOutcomeReportFailed,
			Retryable:         false,
			RecommendedAction: ActionReplanBatch,
			ReasonCode:        verifyFailureReasonCode(report),
		}
	}
	if len(report.NoTestsRunners) > 0 {
		return VerifyAttemptOutcome{
			Kind:              VerifyOutcomeNoTests,
			Retryable:         false,
			RecommendedAction: ActionFinish,
			ReasonCode:        "no_tests",
		}
	}
	return VerifyAttemptOutcome{
		Kind:              VerifyOutcomeReportPassed,
		Retryable:         false,
		RecommendedAction: ActionFinish,
		ReasonCode:        "tests_passed",
	}
}

func verifyFailureReasonCode(report *types.ChangeReport) string {
	if report == nil {
		return "verify_tool_not_called"
	}
	if report.FailureKind != "" {
		return string(report.FailureKind)
	}
	if report.BuildFailed {
		return "build_failed"
	}
	if strings.TrimSpace(report.FailureSummary) != "" || !report.Passed {
		return "tests_failed"
	}
	return "verify_error"
}
