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
	VerifyOutcomeParserError   VerifyAttemptOutcomeKind = "parser_error"
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
	authority := DeriveObservationAuthorityFromReport(report, err)
	switch authority.State {
	case ObservationAuthorityMissing:
		return VerifyAttemptOutcome{
			Kind:              VerifyOutcomeToolNotCalled,
			Retryable:         authority.Retryable,
			RecommendedAction: authority.RecommendedAction,
			ReasonCode:        authority.ReasonCode,
		}
	case ObservationAuthorityFailed:
		return VerifyAttemptOutcome{
			Kind:              VerifyOutcomeReportFailed,
			Retryable:         authority.Retryable,
			RecommendedAction: authority.RecommendedAction,
			ReasonCode:        authority.ReasonCode,
		}
	case ObservationAuthorityUnverified:
		kind := VerifyOutcomeNoTests
		switch authority.ReasonCode {
		case string(types.FailureKindRunnerMissing):
			kind = VerifyOutcomeRunnerMissing
		case string(types.FailureKindParserError):
			kind = VerifyOutcomeParserError
		}
		return VerifyAttemptOutcome{
			Kind:              kind,
			Retryable:         authority.Retryable,
			RecommendedAction: authority.RecommendedAction,
			ReasonCode:        authority.ReasonCode,
		}
	default:
		return VerifyAttemptOutcome{
			Kind:              VerifyOutcomeReportPassed,
			Retryable:         authority.Retryable,
			RecommendedAction: authority.RecommendedAction,
			ReasonCode:        authority.ReasonCode,
		}
	}
}

func verifyFailureReasonCode(report *types.ChangeReport) string {
	if report == nil {
		return "verify_tool_not_called"
	}
	switch report.FailureKind {
	case types.FailureKindBuildFailure:
		return string(types.FailureKindBuildFailure)
	case types.FailureKindTestsFailed, types.FailureKindTimeout, types.FailureKindOOM,
		types.FailureKindCPULimit, types.FailureKindCrash:
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
