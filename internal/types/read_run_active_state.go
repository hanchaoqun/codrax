package types

import "strings"

const (
	ReadRunActiveStateValid                  = "active_state_valid"
	ReadRunActiveStateInactive               = "active_state_inactive"
	ReadRunActiveStateUnsupportedAction      = "unsupported_active_action"
	ReadRunActiveStatePolicyMismatch         = "active_policy_mismatch"
	ReadRunActiveStateMalformedScope         = "active_policy_scope_malformed"
	ReadRunActiveStateUnsupportedProofState  = "unsupported_proof_state"
	ReadRunActiveStateUnsupportedTruthAction = "unsupported_truth_action"

	ReadRunTransientRetryReasonCheckpoint = "transient_retry_checkpoint"
)

// ReadRunActiveState is the snapshot-safe, typed surface for scheduler-local
// read state that is needed to resume a run without replaying raw prompt text.
// It may carry hashes of model-facing retry hints for audit, but never stores
// the hint body, model rationale, final-answer prose, or user-keyword-derived
// routing signals.
type ReadRunActiveState struct {
	TransientRetryPending    bool                   `json:"transient_retry_pending,omitempty"`
	TransientRetryReasonCode string                 `json:"transient_retry_reason_code,omitempty"`
	TransientRetryHintHash   string                 `json:"transient_retry_hint_hash,omitempty"`
	TransientRetryHintBytes  int                    `json:"transient_retry_hint_bytes,omitempty"`
	ReadLoopNextAction       ReadRunNextActionState `json:"read_loop_next_action,omitempty"`
	ReadDispatchPolicy       ReadDispatchPolicy     `json:"read_dispatch_policy,omitempty"`
}

type ReadRunNextActionState struct {
	Active          bool                   `json:"active,omitempty"`
	Action          string                 `json:"action,omitempty"`
	ReasonCode      string                 `json:"reason_code,omitempty"`
	ProofState      string                 `json:"proof_state,omitempty"`
	TruthAction     TruthRecommendedAction `json:"truth_action,omitempty"`
	RouteSurface    string                 `json:"route_surface,omitempty"`
	RouteReasonCode string                 `json:"route_reason_code,omitempty"`
	ToolSuggestions []string               `json:"tool_suggestions,omitempty"`
}

type ReadRunActiveStateValidation struct {
	Valid      bool   `json:"valid"`
	ReasonCode string `json:"reason_code,omitempty"`
	Field      string `json:"field,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

func (s ReadRunActiveState) IsActive() bool {
	s = NormalizeReadRunActiveState(s)
	return s.TransientRetryPending || s.ReadLoopNextAction.Active || s.ReadDispatchPolicy.Active
}

func NormalizeReadRunActiveState(s ReadRunActiveState) ReadRunActiveState {
	s.TransientRetryReasonCode = strings.TrimSpace(s.TransientRetryReasonCode)
	s.TransientRetryHintHash = strings.TrimSpace(s.TransientRetryHintHash)
	if s.TransientRetryHintBytes < 0 {
		s.TransientRetryHintBytes = 0
	}
	if s.TransientRetryPending && s.TransientRetryReasonCode == "" {
		s.TransientRetryReasonCode = ReadRunTransientRetryReasonCheckpoint
	}
	s.ReadLoopNextAction = NormalizeReadRunNextActionState(s.ReadLoopNextAction)
	s.ReadDispatchPolicy = NormalizeReadDispatchPolicy(s.ReadDispatchPolicy)
	if !s.ReadLoopNextAction.Active && !s.ReadDispatchPolicy.Active && !s.TransientRetryPending {
		s = ReadRunActiveState{}
	}
	return s
}

func NormalizeReadRunNextActionState(a ReadRunNextActionState) ReadRunNextActionState {
	a.Action = strings.TrimSpace(a.Action)
	a.ReasonCode = strings.TrimSpace(a.ReasonCode)
	a.ProofState = normalizeReadRunProofState(a.ProofState)
	a.TruthAction = normalizeReadRunTruthActionForStorage(a.TruthAction)
	a.RouteSurface = strings.TrimSpace(a.RouteSurface)
	a.RouteReasonCode = strings.TrimSpace(a.RouteReasonCode)
	a.ToolSuggestions = normalizePolicyTools(a.ToolSuggestions)
	a.Active = a.Active && a.Action != ""
	if !a.Active {
		a = ReadRunNextActionState{}
	}
	return a
}

func ReadRunTransientRetryHintAudit(hint string) (hash string, bytes int) {
	if strings.TrimSpace(hint) == "" {
		return "", 0
	}
	return readRunSHA256Hex(hint), len([]byte(hint))
}

func ValidateReadRunActiveState(s ReadRunActiveState, repoRoot string) ReadRunActiveStateValidation {
	rawAction := s.ReadLoopNextAction
	rawProofState := strings.TrimSpace(rawAction.ProofState)
	rawTruthAction := rawAction.TruthAction
	s = NormalizeReadRunActiveState(s)
	if !s.IsActive() {
		return ReadRunActiveStateValidation{Valid: true, ReasonCode: ReadRunActiveStateInactive}
	}
	if s.ReadLoopNextAction.Active {
		if s.ReadLoopNextAction.Action != ReadDispatchPolicyActionAddProof {
			return ReadRunActiveStateValidation{ReasonCode: ReadRunActiveStateUnsupportedAction, Field: "read_loop_next_action.action", Detail: s.ReadLoopNextAction.Action}
		}
		if rawProofState != "" && !readRunProofStateSupported(rawProofState) {
			return ReadRunActiveStateValidation{ReasonCode: ReadRunActiveStateUnsupportedProofState, Field: "read_loop_next_action.proof_state", Detail: rawProofState}
		}
		if rawTruthAction != "" && NormalizeTruthRecommendedAction(rawTruthAction) == "" {
			return ReadRunActiveStateValidation{ReasonCode: ReadRunActiveStateUnsupportedTruthAction, Field: "read_loop_next_action.truth_action", Detail: string(rawTruthAction)}
		}
	}
	if s.ReadDispatchPolicy.Active {
		if s.ReadDispatchPolicy.Action != ReadDispatchPolicyActionAddProof {
			return ReadRunActiveStateValidation{ReasonCode: ReadRunActiveStatePolicyMismatch, Field: "read_dispatch_policy.action", Detail: s.ReadDispatchPolicy.Action}
		}
		if s.ReadLoopNextAction.Active && s.ReadDispatchPolicy.Action != s.ReadLoopNextAction.Action {
			return ReadRunActiveStateValidation{ReasonCode: ReadRunActiveStatePolicyMismatch, Field: "read_dispatch_policy.action", Detail: s.ReadDispatchPolicy.Action}
		}
		scopeDecision := ValidateReadDispatchPolicyScopePaths(s.ReadDispatchPolicy, repoRoot)
		if scopeDecision.Blocked() {
			return ReadRunActiveStateValidation{ReasonCode: ReadRunActiveStateMalformedScope, Field: scopeDecision.Field, Detail: scopeDecision.Value}
		}
	}
	if s.ReadLoopNextAction.Active && !s.ReadDispatchPolicy.Active {
		return ReadRunActiveStateValidation{ReasonCode: ReadRunActiveStatePolicyMismatch, Field: "read_dispatch_policy", Detail: "missing policy for active next action"}
	}
	return ReadRunActiveStateValidation{Valid: true, ReasonCode: ReadRunActiveStateValid}
}

func normalizeReadRunProofState(state string) string {
	return strings.ToLower(strings.TrimSpace(state))
}

func readRunProofStateSupported(state string) bool {
	switch normalizeReadRunProofState(state) {
	case "", "missing", "covered", "weak", "unavailable", "failed":
		return true
	default:
		return false
	}
}

func normalizeReadRunTruthActionForStorage(action TruthRecommendedAction) TruthRecommendedAction {
	normalized := NormalizeTruthRecommendedAction(action)
	if normalized != "" {
		return normalized
	}
	return TruthRecommendedAction(strings.ToLower(strings.TrimSpace(string(action))))
}
