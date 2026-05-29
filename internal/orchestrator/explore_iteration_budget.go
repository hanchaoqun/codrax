package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

func applyExploreIterationScalingForRequest(agentCtx *types.AgentContext, rm types.RequestModel, agentCfg types.AgentSettings) (base int, adjusted int, reason string, applied bool) {
	if agentCtx == nil {
		return 0, 0, "", false
	}
	base = agentCfg.MaxIterations
	if base <= 0 {
		base = 20
	}
	ceil := agentCfg.ExplorerScaledIterMax
	if ceil <= 0 {
		ceil = 35
	}
	adjusted = base
	if nSub := len(rm.SubTopics); nSub > 1 {
		extra := nSub * agentCfg.SubTopicExplorerBudgetExtra
		adjusted = base + extra
		reason = "multi-topic"
	} else if shouldScaleSingleTopicExplore(rm) {
		adjusted = ceil
		reason = "single-topic-complex-chain"
	}
	if adjusted > ceil {
		adjusted = ceil
	}
	if adjusted <= base {
		return base, adjusted, reason, false
	}
	agentCtx.MaxIterOverride = adjusted
	return base, adjusted, reason, true
}

func shouldScaleSingleTopicExplore(rm types.RequestModel) bool {
	family := types.ResolveQuestionFamily(rm)
	if family == types.QFRootCauseTrace {
		return rm.Complexity == types.ComplexityModerate ||
			rm.Complexity == types.ComplexityComplex ||
			rm.HasExternalOnlyRuntimeArtifact() ||
			rm.HasObservationOnlyRuntimeArtifact()
	}
	if family == types.QFCallChain && rm.Complexity == types.ComplexityComplex {
		return true
	}
	if rm.DiagramHint != nil {
		switch rm.DiagramHint.Kind {
		case types.DiagramSequence, types.DiagramCallDAG:
			return rm.Complexity == types.ComplexityComplex || rm.Intent == types.IntentRootCause || rm.Scenario == types.ScenarioRootCause
		}
	}
	return false
}
