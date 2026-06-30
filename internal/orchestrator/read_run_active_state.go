package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

func readRunActiveStateFromGraphState(state *graphState) types.ReadRunActiveState {
	if state == nil {
		return types.ReadRunActiveState{}
	}
	out := types.ReadRunActiveState{}
	if hint := strings.TrimSpace(state.transientRetryHint); hint != "" {
		hash, bytes := types.ReadRunTransientRetryHintAudit(hint)
		out.TransientRetryPending = true
		out.TransientRetryReasonCode = types.ReadRunTransientRetryReasonCheckpoint
		out.TransientRetryHintHash = hash
		out.TransientRetryHintBytes = bytes
	}
	if decision := state.readLoopNextAction; decision.Active {
		out.ReadLoopNextAction = readRunNextActionStateFromDecision(decision)
		out.ReadDispatchPolicy = decision.Policy
	}
	return types.NormalizeReadRunActiveState(out)
}

func readRunNextActionStateFromDecision(decision readLoopNextActionDecision) types.ReadRunNextActionState {
	if !decision.Active {
		return types.ReadRunNextActionState{}
	}
	return types.NormalizeReadRunNextActionState(types.ReadRunNextActionState{
		Active:          true,
		Action:          strings.TrimSpace(string(decision.Action)),
		ReasonCode:      decision.ReasonCode,
		ProofState:      strings.TrimSpace(string(decision.ProofState)),
		TruthAction:     decision.TruthAction,
		RouteSurface:    strings.TrimSpace(string(decision.RouteSurface)),
		RouteReasonCode: decision.RouteReasonCode,
		ToolSuggestions: append([]string(nil), decision.ToolSuggestions...),
	})
}

func readLoopNextActionDecisionFromReadRunActiveState(active types.ReadRunActiveState) (readLoopNextActionDecision, bool) {
	active = types.NormalizeReadRunActiveState(active)
	if !active.ReadLoopNextAction.Active {
		return readLoopNextActionDecision{}, false
	}
	next := active.ReadLoopNextAction
	decision := readLoopNextActionDecision{
		Active:          true,
		Action:          loopkernel.LoopRecommendedAction(next.Action),
		ReasonCode:      next.ReasonCode,
		ProofState:      loopkernel.ProofCoverageAuthorityState(next.ProofState),
		TruthAction:     next.TruthAction,
		RouteSurface:    loopkernel.LoopToolSurface(next.RouteSurface),
		RouteReasonCode: next.RouteReasonCode,
		ToolSuggestions: append([]string(nil), next.ToolSuggestions...),
		Policy:          types.NormalizeReadDispatchPolicy(active.ReadDispatchPolicy),
	}
	if !decision.Policy.IsActive() {
		decision.Policy = readDispatchPolicyForNextAction(decision, nil)
	}
	return decision, true
}

func (s *graphState) applyReadRunActiveStateSeed(active types.ReadRunActiveState) {
	if s == nil {
		return
	}
	active = types.NormalizeReadRunActiveState(active)
	if decision, ok := readLoopNextActionDecisionFromReadRunActiveState(active); ok {
		s.setReadLoopNextAction(decision)
	}
	if active.TransientRetryPending {
		s.setTransientRetryHint(readRunTransientRetryResumeHint(active), types.RetryHintKindDurableProgressContinuation)
	}
}

func readRunTransientRetryResumeHint(active types.ReadRunActiveState) string {
	active = types.NormalizeReadRunActiveState(active)
	if !active.TransientRetryPending {
		return ""
	}
	return fmt.Sprintf("Read run snapshot resumed a typed transient-retry checkpoint: reason=%s hint_hash=%s hint_bytes=%d. Continue from durable typed state and avoid restarting broad repository discovery unless a typed gate requires it.",
		firstNonEmptyRetryString(active.TransientRetryReasonCode, types.ReadRunTransientRetryReasonCheckpoint),
		firstNonEmptyRetryString(active.TransientRetryHintHash, "none"),
		active.TransientRetryHintBytes)
}

func (o *Orchestrator) captureReadRunActiveStateFromGraphState(state *graphState) {
	if o == nil {
		return
	}
	o.readRunActiveState = readRunActiveStateFromGraphState(state)
}
