package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// orchestratorPriorConvVisibleForStage is the orchestrator-aware
// variant of priorConvVisibleForStage (commit 48 Gap 1). It
// consults the LLM continuation classifier (cached per-Run); when
// the classifier is unavailable or errors, defaults to "fresh"
// (false) — the safe direction. Pre-commit-48 there was a keyword-
// list fallback (types.IsContinuation), but operator feedback
// (2026-05-01) flagged that as dead code AND a red-line violation
// (feedback_no_custom_keyword_matching.md). cmd/root.go always
// wires the classifier from providers.yaml :: agents.continuation_classifier
// (defaulting to default LLM), so the no-classifier path is
// effectively unreachable in production.
func (o *Orchestrator) orchestratorPriorConvVisibleForStage(policy string, stage types.PipelineStage, objective string) bool {
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
		return o.classifyContinuationCached(prior, current)
	case types.PriorConvPolicyAlways, "":
		fallthrough
	default:
		return true
	}
}

// classifyContinuationCached resolves the LLM continuation
// classification once per Run. Cached on o.continuationClassification;
// the first PolicyContinue invocation under a non-analyze stage
// computes the call, subsequent stages read the cached pointer
// (pointer-tri-state: nil pre-decision, &true / &false post-).
// All failure paths (no classifier, chat error, no tool call,
// unparseable JSON) default to false — pre-commit-48 they fell
// back to the keyword heuristic, which has been removed.
func (o *Orchestrator) classifyContinuationCached(prior, current string) bool {
	if o == nil {
		return false
	}
	if o.continuationClassification != nil {
		return *o.continuationClassification
	}
	if o.continuationClassifier == nil {
		// No classifier wired (test fixture / placeholder
		// adapter). Default to fresh — safest direction.
		f := false
		o.continuationClassification = &f
		return false
	}
	ctx := o.CancelContext()
	got, err := o.continuationClassifier.Classify(ctx, current, prior)
	if err != nil {
		logging.Debug("[continuation_classifier] errored, defaulting to fresh: %v", err)
		f := false
		o.continuationClassification = &f
		return false
	}
	o.continuationClassification = &got
	return got
}
