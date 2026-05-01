package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// freshOrchestrator builds a minimal Orchestrator with the
// continuation classifier left nil so the policy logic
// exercises the no-classifier default-to-fresh path.
func freshOrchestrator() *Orchestrator {
	return New(types.PipelineSettings{}, nil, nil, nil)
}

// TestPriorConvVisibleForStage_Always — historical behaviour: every
// stage sees Prior Conversation under PriorConvPolicyAlways.
func TestPriorConvVisibleForStage_Always(t *testing.T) {
	o := freshOrchestrator()
	for _, stage := range []types.PipelineStage{
		types.StageAnalyze, types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		if !o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyAlways, stage, "## Prior conversation\nx\n\n## Current request\ny") {
			t.Errorf("always policy must keep Prior visible for stage=%s", stage)
		}
	}
}

// TestPriorConvVisibleForStage_Analyzer — default: only analyzer
// sees Prior; explorer/extractor/finalizer are blind.
func TestPriorConvVisibleForStage_Analyzer(t *testing.T) {
	o := freshOrchestrator()
	obj := "## Prior conversation\nold\n\n## Current request\nhow many X"
	if !o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyAnalyzer, types.StageAnalyze, obj) {
		t.Error("analyzer stage must see Prior under policy=analyzer")
	}
	for _, stage := range []types.PipelineStage{
		types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		if o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyAnalyzer, stage, obj) {
			t.Errorf("policy=analyzer must hide Prior from stage=%s", stage)
		}
	}
}

// TestPriorConvVisibleForStage_Continue — analyzer always sees Prior;
// downstream stages default to "fresh" (false) when no LLM
// classifier is wired (commit 48 simplification: keyword fallback
// removed). The LLM-classifier-takes-priority path is covered by
// TestOrchestratorPriorConvVisibleForStage_LLMTakesPriority.
func TestPriorConvVisibleForStage_Continue(t *testing.T) {
	o := freshOrchestrator()
	cont := "## Prior conversation\nold\n\n## Current request\n再详细讲讲那个流程"
	fresh := "## Prior conversation\nold\n\n## Current request\nhow does FooBar work"

	// Analyzer: always visible.
	if !o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageAnalyze, cont) {
		t.Error("continue+analyzer must see Prior")
	}
	if !o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageAnalyze, fresh) {
		t.Error("continue+analyzer must see Prior even for fresh question")
	}

	// Downstream stages, no classifier: default to fresh (hidden).
	// Pre-commit-48 these tested the keyword-list path; commit 48
	// removed it so default is false regardless of cue.
	if o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageExplore, cont) {
		t.Error("no-classifier continue+explore must default to fresh (hidden)")
	}
	if o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageExplore, fresh) {
		t.Error("no-classifier continue+explore on fresh question must hide Prior")
	}
}

// TestPriorConvVisibleForStage_Never — no stage sees Prior.
func TestPriorConvVisibleForStage_Never(t *testing.T) {
	o := freshOrchestrator()
	obj := "## Prior conversation\nx\n\n## Current request\n再继续"
	for _, stage := range []types.PipelineStage{
		types.StageAnalyze, types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		if o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyNever, stage, obj) {
			t.Errorf("policy=never must hide Prior for stage=%s", stage)
		}
	}
}

// TestPriorConvVisibleForStage_UnknownFallback — unknown / empty
// policies fall back to "always" so a partially-initialised config
// doesn't accidentally blind every stage.
func TestPriorConvVisibleForStage_UnknownFallback(t *testing.T) {
	o := freshOrchestrator()
	if !o.orchestratorPriorConvVisibleForStage("", types.StageExplore, "x") {
		t.Error("empty policy must default to visible (always fallback)")
	}
	if !o.orchestratorPriorConvVisibleForStage("unknown-value", types.StageExplore, "x") {
		t.Error("unknown policy must default to visible (always fallback)")
	}
}

func TestPriorConvVisibleForStage_ControlInputAlwaysHidden(t *testing.T) {
	o := freshOrchestrator()
	obj := "## Prior conversation\nold\n\n## Current request\n\\q"
	for _, stage := range []types.PipelineStage{
		types.StageAnalyze, types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		if o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyAlways, stage, obj) {
			t.Errorf("control input must hide Prior for stage=%s", stage)
		}
	}
}
