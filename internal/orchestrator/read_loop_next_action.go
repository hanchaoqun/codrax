package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

type readLoopNextActionDecision struct {
	Active          bool
	Action          loopkernel.LoopRecommendedAction
	ReasonCode      string
	ProofState      loopkernel.ProofCoverageAuthorityState
	TruthAction     types.TruthRecommendedAction
	RouteSurface    loopkernel.LoopToolSurface
	RouteReasonCode string
	ToolSuggestions []string
}

func (s *graphState) setReadLoopNextAction(decision readLoopNextActionDecision) {
	if s == nil || !decision.Active {
		return
	}
	s.readLoopNextAction = decision
}

func (s *graphState) consumeReadLoopNextAction() (readLoopNextActionDecision, bool) {
	if s == nil || !s.readLoopNextAction.Active {
		return readLoopNextActionDecision{}, false
	}
	decision := s.readLoopNextAction
	s.readLoopNextAction = readLoopNextActionDecision{}
	return decision, true
}

func (o *Orchestrator) recordReadLoopNextActionForRetry(state *graphState, reason string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || state == nil {
		return
	}
	decision, ok := readLoopNextActionDecisionFromMutable(o.busCtx.Mutable)
	if !ok {
		return
	}
	state.setReadLoopNextAction(decision)
	logging.Debug("[orchestrator] read loop next-action selected for %s: action=%s reason=%s",
		firstNonEmptyRetryString(reason, "retry"), decision.Action, decision.ReasonCode)
}

func applyReadLoopNextActionHint(state *graphState, hint *string, parallelHints []string) {
	if state == nil {
		return
	}
	decision, ok := state.consumeReadLoopNextAction()
	if !ok {
		return
	}
	actionHint := renderReadLoopNextActionDirective(decision)
	if strings.TrimSpace(actionHint) == "" {
		return
	}
	if len(parallelHints) > 0 {
		for i := range parallelHints {
			parallelHints[i] = prependRetryHint(actionHint, parallelHints[i])
		}
		return
	}
	if hint != nil {
		*hint = prependRetryHint(actionHint, *hint)
	}
}

func readLoopNextActionDecisionFromMutable(m *types.MutableState) (readLoopNextActionDecision, bool) {
	guidance, ok := loopkernel.ReadProofGuidanceFromMutable(m)
	return readLoopNextActionDecisionFromGuidance(guidance, ok)
}

func readLoopNextActionDecisionFromGuidance(guidance loopkernel.ReadProofGuidance, ok bool) (readLoopNextActionDecision, bool) {
	if !ok || !guidance.Active || guidance.HardBlock {
		return readLoopNextActionDecision{}, false
	}
	if guidance.RecommendedAction != loopkernel.LoopActionAddProof {
		return readLoopNextActionDecision{}, false
	}
	route := loopkernel.ToolRouteForAction(guidance.RecommendedAction)
	return readLoopNextActionDecision{
		Active:          true,
		Action:          guidance.RecommendedAction,
		ReasonCode:      firstNonEmptyRetryString(guidance.ReasonCode, route.ReasonCode, "proof_weak"),
		ProofState:      guidance.State,
		TruthAction:     guidance.TruthAction,
		RouteSurface:    route.Surface,
		RouteReasonCode: firstNonEmptyRetryString(route.ReasonCode, "loop_tool_route_verification"),
		ToolSuggestions: append([]string(nil), route.ToolSuggestions...),
	}, true
}

func readLoopNextActionDecisionSummary(decision readLoopNextActionDecision) string {
	if !decision.Active {
		return ""
	}
	parts := []string{
		fmt.Sprintf("loop next-action=%s", firstNonEmptyRetryString(string(decision.Action), "none")),
		"source=proof_authority",
		fmt.Sprintf("reason=%s", firstNonEmptyRetryString(decision.ReasonCode, "none")),
		fmt.Sprintf("route_surface=%s", firstNonEmptyRetryString(string(decision.RouteSurface), "none")),
		fmt.Sprintf("route_reason=%s", firstNonEmptyRetryString(decision.RouteReasonCode, "none")),
	}
	if len(decision.ToolSuggestions) > 0 {
		parts = append(parts, "route_tools="+strings.Join(decision.ToolSuggestions, ","))
	}
	return strings.Join(parts, " ")
}

func renderReadLoopNextActionDirective(decision readLoopNextActionDecision) string {
	if !decision.Active {
		return ""
	}
	return strings.Join([]string{
		"Read loop typed next action:",
		readLoopNextActionDecisionSummary(decision) + ".",
		"Use the next explore retry as a narrow proof-collection continuation for already accepted evidence. Do not restart broad repository discovery for this action; collect the smallest additional typed proof available through the current stage's allowed tools, or preserve an explicit unverified-proof caveat if no proof route is available.",
	}, "\n")
}
