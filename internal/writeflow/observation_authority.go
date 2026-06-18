package writeflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ObservationAuthorityState is the controller-facing verdict for a post-apply
// observation. It is derived from typed ChangeReport / WriteWorkflowAttempt
// fields only; model prose and progress text never influence it.
type ObservationAuthorityState string

const (
	ObservationAuthorityMissing    ObservationAuthorityState = "missing"
	ObservationAuthorityVerified   ObservationAuthorityState = "verified"
	ObservationAuthorityUnverified ObservationAuthorityState = "unverified"
	ObservationAuthorityFailed     ObservationAuthorityState = "failed"
)

// ObservationAuthorityView is the single typed gate shared by verify outcome
// classification, finish gating, completion derivation, and state rendering.
type ObservationAuthorityView struct {
	State             ObservationAuthorityState            `json:"state"`
	Status            string                               `json:"status,omitempty"`
	ReasonCode        string                               `json:"reason_code,omitempty"`
	FailureReasonCode string                               `json:"failure_reason_code,omitempty"`
	CompletionVerdict types.WriteWorkflowCompletionVerdict `json:"completion_verdict,omitempty"`
	RecommendedAction WorkflowAction                       `json:"recommended_action,omitempty"`
	Retryable         bool                                 `json:"retryable,omitempty"`
	MustObserve       bool                                 `json:"must_observe,omitempty"`
	MustReplan        bool                                 `json:"must_replan,omitempty"`

	// FinishAllowed means finish is safe without extra disposition.
	FinishAllowed bool `json:"finish_allowed,omitempty"`
	// FinishAllowedWithAcceptUnverified is true for unavailable local verifier
	// states that may be explicitly accepted through the typed disposition.
	FinishAllowedWithAcceptUnverified bool `json:"finish_allowed_with_accept_unverified,omitempty"`
	FinishRequiresDisposition         bool `json:"finish_requires_disposition,omitempty"`

	ReportID    string `json:"report_id,omitempty"`
	ArtifactRef string `json:"artifact_ref,omitempty"`
	SurfaceRef  string `json:"surface_ref,omitempty"`
}

// DeriveObservationAuthorityFromReport classifies one verifier report for the
// scheduler. The err value contributes only executor failure presence; its text
// is never parsed.
func DeriveObservationAuthorityFromReport(report *types.ChangeReport, err error) ObservationAuthorityView {
	if report == nil {
		return observationMissing("verify_tool_not_called")
	}
	status := report.NormalizeVerificationStatus()
	failureReason := strings.TrimSpace(report.FailureReasonCode)
	switch {
	case report.FailureKind == types.FailureKindRunnerMissing:
		return observationUnverified(string(types.FailureKindRunnerMissing), failureReason, false)
	case report.FailureKind == types.FailureKindParserError:
		return observationUnverified(string(types.FailureKindParserError), failureReason, false)
	case err != nil || status == types.VerificationStatusFailed:
		return observationFailed(verifyFailureReasonCode(report), failureReason)
	case report.FailureKind == types.FailureKindNoTests || len(report.NoTestsRunners) > 0:
		return observationUnverified(string(types.FailureKindNoTests), failureReason, false)
	case status == types.VerificationStatusUnavailable:
		return observationUnverified(observationUnavailableReportReason(report), failureReason, false)
	default:
		return observationVerified("tests_passed", failureReason)
	}
}

// DeriveObservationAuthorityFromAttempt classifies the latest durable verify
// attempt. Unknown statuses deny finish by default and ask for observation.
func DeriveObservationAuthorityFromAttempt(attempt *types.WriteWorkflowAttempt) ObservationAuthorityView {
	if attempt == nil {
		return observationMissing("missing_verify_attempt")
	}
	status := strings.TrimSpace(attempt.Status)
	reason := strings.TrimSpace(attempt.ReasonCode)
	if reason == "" {
		reason = status
	}
	view := ObservationAuthorityView{
		Status:            status,
		ReasonCode:        reason,
		FailureReasonCode: strings.TrimSpace(attempt.FailureReasonCode),
		ReportID:          strings.TrimSpace(attempt.ReportID),
		ArtifactRef:       strings.TrimSpace(attempt.ArtifactRef),
		SurfaceRef:        strings.TrimSpace(attempt.SurfaceRef),
	}
	switch status {
	case "passed":
		view = mergeObservationRefs(view, observationVerified(reason, view.FailureReasonCode))
	case "unverified":
		if observationReasonIsCodeFailure(reason) {
			view = mergeObservationRefs(view, observationFailed(reason, view.FailureReasonCode))
		} else {
			view = mergeObservationRefs(view, observationUnverified(reason, view.FailureReasonCode, false))
		}
	case "failed":
		if observationReasonIsUnavailable(reason) {
			view = mergeObservationRefs(view, observationUnverified(reason, view.FailureReasonCode, true))
		} else {
			view = mergeObservationRefs(view, observationFailed(reason, view.FailureReasonCode))
		}
	case "skipped":
		if reason == "" || reason == "skipped" {
			reason = "skip_verify"
		}
		view = mergeObservationRefs(view, observationUnverified(reason, view.FailureReasonCode, false))
	case "infra_error", "missing_report", "":
		if reason == "" {
			reason = "verify_tool_not_called"
		}
		view = mergeObservationRefs(view, observationMissing(reason))
	default:
		view = mergeObservationRefs(view, observationMissing("unknown_verify_attempt_status"))
	}
	return view
}

func LatestBatchObservationAuthority(batch types.WriteWorkflowBatch) ObservationAuthorityView {
	return DeriveObservationAuthorityFromAttempt(latestAttemptOfKind(batch, "verify"))
}

func observationMissing(reason string) ObservationAuthorityView {
	reason = firstObservationReason(reason, "verify_tool_not_called")
	return ObservationAuthorityView{
		State:             ObservationAuthorityMissing,
		Status:            "missing",
		ReasonCode:        reason,
		RecommendedAction: ActionVerifyBatch,
		Retryable:         true,
		MustObserve:       true,
	}
}

func observationVerified(reason, failureReason string) ObservationAuthorityView {
	reason = firstObservationReason(reason, "tests_passed")
	return ObservationAuthorityView{
		State:             ObservationAuthorityVerified,
		Status:            "passed",
		ReasonCode:        reason,
		FailureReasonCode: strings.TrimSpace(failureReason),
		CompletionVerdict: types.WriteWorkflowCompletionVerified,
		RecommendedAction: ActionFinish,
		FinishAllowed:     true,
	}
}

func observationUnverified(reason, failureReason string, requiresDisposition bool) ObservationAuthorityView {
	reason = firstObservationReason(reason, string(types.FailureKindNoTests))
	return ObservationAuthorityView{
		State:                             ObservationAuthorityUnverified,
		Status:                            "unverified",
		ReasonCode:                        reason,
		FailureReasonCode:                 strings.TrimSpace(failureReason),
		CompletionVerdict:                 types.WriteWorkflowCompletionUnverified,
		RecommendedAction:                 ActionFinish,
		FinishAllowed:                     !requiresDisposition,
		FinishAllowedWithAcceptUnverified: true,
		FinishRequiresDisposition:         requiresDisposition,
	}
}

func observationFailed(reason, failureReason string) ObservationAuthorityView {
	reason = firstObservationReason(reason, string(types.FailureKindTestsFailed))
	return ObservationAuthorityView{
		State:             ObservationAuthorityFailed,
		Status:            "failed",
		ReasonCode:        reason,
		FailureReasonCode: strings.TrimSpace(failureReason),
		RecommendedAction: ActionReplanBatch,
		MustReplan:        true,
	}
}

func mergeObservationRefs(base, derived ObservationAuthorityView) ObservationAuthorityView {
	derived.ReportID = base.ReportID
	derived.ArtifactRef = base.ArtifactRef
	derived.SurfaceRef = base.SurfaceRef
	if derived.FailureReasonCode == "" {
		derived.FailureReasonCode = base.FailureReasonCode
	}
	return derived
}

func firstObservationReason(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func observationUnavailableReportReason(report *types.ChangeReport) string {
	if report == nil {
		return "verify_tool_not_called"
	}
	if report.FailureKind != "" {
		return string(report.FailureKind)
	}
	if len(report.NoTestsRunners) > 0 {
		return string(types.FailureKindNoTests)
	}
	return "verification_unavailable"
}

func observationReasonIsUnavailable(reason string) bool {
	switch strings.TrimSpace(reason) {
	case string(types.FailureKindNoTests),
		string(types.FailureKindRunnerMissing),
		string(types.FailureKindParserError),
		"skip_verify",
		"accepted_without_local_verify",
		"make_python_module_missing",
		"make_target_missing",
		"verification_probe_import_error",
		"verification_probe_module_not_found",
		"verification_probe_name_error",
		"verification_probe_syntax_error":
		return true
	default:
		return false
	}
}

func observationReasonIsCodeFailure(reason string) bool {
	switch strings.TrimSpace(reason) {
	case string(types.FailureKindTestsFailed),
		string(types.FailureKindBuildFailure),
		string(types.FailureKindTimeout),
		string(types.FailureKindOOM),
		string(types.FailureKindCPULimit),
		string(types.FailureKindCrash),
		"build_failed",
		"verify_error",
		"verification_probe_exception",
		"verification_probe_expected_stdout_missing",
		"verification_probe_runtime_exception",
		"verification_probe_timeout":
		return true
	default:
		return false
	}
}
