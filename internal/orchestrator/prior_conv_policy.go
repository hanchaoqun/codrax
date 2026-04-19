package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// priorConvVisibleForStage resolves AgentSettings.PriorConvPolicy into
// a per-stage yes/no visibility decision. Called once per dispatch by
// dispatchAgent after BuildAgentContext and before ag.Execute; the
// resulting boolean is written to AgentContext.PriorConvHidden (with
// the inverted sense, see the type doc).
//
// The policy values and their per-stage semantics:
//
//	always    — every stage sees Prior. Historical behaviour.
//	analyzer  — only StageAnalyze sees Prior. Downstream stages work
//	            off the AnalysisIR that the analyzer populated (which
//	            carries any Prior-derived entity disambiguation) and
//	            off the current-repo code, not the prior transcript.
//	continue  — StageAnalyze always; downstream stages see Prior only
//	            when types.IsContinuation reports the current request
//	            is a continuation of the prior conversation.
//	never     — no stage sees Prior. Extreme isolation.
//
// Unknown / empty policies fall back to "always" — the same
// behaviour the validator in ResolvedAgentSettings would normally
// produce, but we degrade safely here too so this helper can be
// invoked on a partially-initialised config without panicking.
func priorConvVisibleForStage(policy string, stage types.PipelineStage, objective string) bool {
	_, current := types.SplitConversation(objective)
	if types.IsREPLControlInput(current) {
		return false
	}
	switch policy {
	case types.PriorConvPolicyNever:
		return false
	case types.PriorConvPolicyAnalyzer:
		return stage == types.StageAnalyze
	case types.PriorConvPolicyContinue:
		if stage == types.StageAnalyze {
			return true
		}
		prior, current := types.SplitConversation(objective)
		return types.IsContinuation(current, prior)
	case types.PriorConvPolicyAlways, "":
		fallthrough
	default:
		return true
	}
}
