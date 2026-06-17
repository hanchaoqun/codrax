package writeflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ApprovalExecutionState is the canonical execution meaning of a plan approval
// record. It is intentionally separate from ChangePlan.Status: status is a
// proposal/execution lifecycle field, while this state answers whether the
// current plan bytes may be applied.
type ApprovalExecutionState string

const (
	ApprovalExecutionMissing             ApprovalExecutionState = "missing_approval"
	ApprovalExecutionAutoAllowed         ApprovalExecutionState = "auto_allowed"
	ApprovalExecutionManualRequired      ApprovalExecutionState = "manual_required"
	ApprovalExecutionManualApproved      ApprovalExecutionState = "manual_approved"
	ApprovalExecutionDenied              ApprovalExecutionState = "denied"
	ApprovalExecutionFingerprintMismatch ApprovalExecutionState = "fingerprint_mismatch"
	ApprovalExecutionIntegrityFailed     ApprovalExecutionState = "integrity_failed"
	ApprovalExecutionUnknownAction       ApprovalExecutionState = "unknown_action"
)

// ApprovalExecutionView is a typed state-kernel fragment consumed by UI,
// adapters, and future scheduler chaining. It reads only the structured
// ChangePlan approval record and plan fingerprint; it never infers authority
// from model prose, logs, summaries, or the plan lifecycle status alone.
type ApprovalExecutionView struct {
	State ApprovalExecutionState `json:"state"`

	Action       ApprovalAction `json:"action,omitempty"`
	UserDecision string         `json:"user_decision,omitempty"`
	ReasonCode   string         `json:"reason_code,omitempty"`

	PlanFingerprint     string `json:"plan_fingerprint,omitempty"`
	ApprovalFingerprint string `json:"approval_fingerprint,omitempty"`

	CanApply     bool `json:"can_apply,omitempty"`
	CanAutoApply bool `json:"can_auto_apply,omitempty"`
	RequiresUser bool `json:"requires_user,omitempty"`
	Denied       bool `json:"denied,omitempty"`
}

// DeriveApprovalExecutionView turns a persisted approval record into the
// authority view for the exact current plan bytes. The result is deliberately
// conservative when the record is missing, tampered, stale, or unknown.
func DeriveApprovalExecutionView(plan *types.ChangePlan) ApprovalExecutionView {
	view := ApprovalExecutionView{State: ApprovalExecutionMissing, ReasonCode: "missing_approval_record"}
	if plan == nil || plan.Approval == nil {
		return view
	}
	approval := plan.Approval
	currentFingerprint := types.PlanFingerprint(plan)
	view.Action = ApprovalAction(strings.TrimSpace(approval.Action))
	view.UserDecision = strings.TrimSpace(approval.UserDecision)
	view.ReasonCode = strings.TrimSpace(approval.ReasonCode)
	view.PlanFingerprint = currentFingerprint
	view.ApprovalFingerprint = strings.TrimSpace(approval.PlanFingerprint)
	if !plan.ApprovalRecordIntegrityOK() {
		view.State = ApprovalExecutionIntegrityFailed
		view.ReasonCode = firstApprovalExecutionReason(view.ReasonCode, "approval_record_integrity_failed")
		view.RequiresUser = true
		return view
	}
	if view.ApprovalFingerprint == "" || view.ApprovalFingerprint != currentFingerprint {
		view.State = ApprovalExecutionFingerprintMismatch
		view.ReasonCode = firstApprovalExecutionReason(view.ReasonCode, "approval_fingerprint_mismatch")
		view.RequiresUser = true
		return view
	}
	switch view.Action {
	case ApprovalActionAutoExecute:
		view.State = ApprovalExecutionAutoAllowed
		view.CanApply = true
		view.CanAutoApply = true
		return view
	case ApprovalActionManual:
		if view.UserDecision == "approved" {
			view.State = ApprovalExecutionManualApproved
			view.CanApply = true
			return view
		}
		view.State = ApprovalExecutionManualRequired
		view.RequiresUser = true
		return view
	case ApprovalActionDeny:
		view.State = ApprovalExecutionDenied
		view.Denied = true
		return view
	default:
		view.State = ApprovalExecutionUnknownAction
		view.ReasonCode = firstApprovalExecutionReason(view.ReasonCode, "unknown_approval_action")
		view.RequiresUser = true
		return view
	}
}

func firstApprovalExecutionReason(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
