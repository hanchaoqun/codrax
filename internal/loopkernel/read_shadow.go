package loopkernel

// ReadLoopShadowComparison compares loopkernel's typed read-mode guidance with
// the imperative scheduler action currently being taken. It is telemetry/shadow
// only: callers must not use this view as a hard gate until a later cutover.
type ReadLoopShadowComparison struct {
	Active            bool                        `json:"active,omitempty"`
	RecommendedAction LoopRecommendedAction       `json:"recommended_action,omitempty"`
	ImperativeAction  LoopRecommendedAction       `json:"imperative_action,omitempty"`
	Match             bool                        `json:"match,omitempty"`
	ReasonCode        string                      `json:"reason_code,omitempty"`
	ProofState        ProofCoverageAuthorityState `json:"proof_state,omitempty"`
	TruthAction       string                      `json:"truth_action,omitempty"`
	HardBlock         bool                        `json:"hard_block,omitempty"`
	Advisory          bool                        `json:"advisory,omitempty"`
}

// CompareReadLoopShadow returns a typed shadow comparison for read-mode
// scheduler decisions. Empty guidance or empty imperative action produces
// ok=false so callers can omit the audit row.
func CompareReadLoopShadow(guidance ReadProofGuidance, imperative LoopRecommendedAction) (ReadLoopShadowComparison, bool) {
	recommended := normalizeLoopAction(guidance.RecommendedAction)
	imperative = normalizeLoopAction(imperative)
	if !guidance.Active || recommended == "" || imperative == "" {
		return ReadLoopShadowComparison{}, false
	}
	comparison := ReadLoopShadowComparison{
		Active:            true,
		RecommendedAction: recommended,
		ImperativeAction:  imperative,
		ReasonCode:        guidance.ReasonCode,
		ProofState:        guidance.State,
		TruthAction:       string(guidance.TruthAction),
		HardBlock:         guidance.HardBlock,
		Advisory:          guidance.Advisory,
	}
	comparison.Match = readLoopActionsCompatible(comparison.RecommendedAction, comparison.ImperativeAction)
	return comparison, true
}

func readLoopActionsCompatible(recommended, imperative LoopRecommendedAction) bool {
	recommended = normalizeLoopAction(recommended)
	imperative = normalizeLoopAction(imperative)
	if recommended == "" || imperative == "" {
		return false
	}
	if recommended == imperative {
		return true
	}
	// Read mode treats unavailable proof as an allowed unverified continuation.
	// A caller choosing continue while loopkernel recommends add_proof is still a
	// shadow mismatch: that is exactly the audit signal M2d-A wants to expose.
	return false
}

func normalizeLoopAction(action LoopRecommendedAction) LoopRecommendedAction {
	switch action {
	case LoopActionContinue, LoopActionLocalize, LoopActionVerify, LoopActionAddProof, LoopActionRepair, LoopActionAskApproval, LoopActionBlock:
		return action
	default:
		return ""
	}
}
