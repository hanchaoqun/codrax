package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPriorConvVisibleForStage_Always — historical behaviour: every
// stage sees Prior Conversation under PriorConvPolicyAlways.
func TestPriorConvVisibleForStage_Always(t *testing.T) {
	for _, stage := range []types.PipelineStage{
		types.StageAnalyze, types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		if !priorConvVisibleForStage(types.PriorConvPolicyAlways, stage, "## Prior conversation\nx\n\n## Current request\ny") {
			t.Errorf("always policy must keep Prior visible for stage=%s", stage)
		}
	}
}

// TestPriorConvVisibleForStage_Analyzer — default: only analyzer
// sees Prior; explorer/extractor/finalizer are blind.
func TestPriorConvVisibleForStage_Analyzer(t *testing.T) {
	obj := "## Prior conversation\nold\n\n## Current request\nhow many X"
	if !priorConvVisibleForStage(types.PriorConvPolicyAnalyzer, types.StageAnalyze, obj) {
		t.Error("analyzer stage must see Prior under policy=analyzer")
	}
	for _, stage := range []types.PipelineStage{
		types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		if priorConvVisibleForStage(types.PriorConvPolicyAnalyzer, stage, obj) {
			t.Errorf("policy=analyzer must hide Prior from stage=%s", stage)
		}
	}
}

// TestPriorConvVisibleForStage_Continue — analyzer always; downstream
// stages see Prior only on explicit continuation cues.
func TestPriorConvVisibleForStage_Continue(t *testing.T) {
	cont := "## Prior conversation\nold\n\n## Current request\n再详细讲讲那个流程"
	fresh := "## Prior conversation\nold\n\n## Current request\nhow does FooBar work"

	// Analyzer: always visible.
	if !priorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageAnalyze, cont) {
		t.Error("continue+analyzer must see Prior (continuation)")
	}
	if !priorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageAnalyze, fresh) {
		t.Error("continue+analyzer must see Prior even for fresh question")
	}

	// Downstream stages: visible only on continuation.
	if !priorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageExplore, cont) {
		t.Error("continue+explore on continuation cue must see Prior")
	}
	if priorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageExplore, fresh) {
		t.Error("continue+explore on fresh question must hide Prior")
	}
	if priorConvVisibleForStage(types.PriorConvPolicyContinue, types.StageFinalize, fresh) {
		t.Error("continue+finalize on fresh question must hide Prior")
	}
}

// TestPriorConvVisibleForStage_Never — no stage sees Prior.
func TestPriorConvVisibleForStage_Never(t *testing.T) {
	obj := "## Prior conversation\nx\n\n## Current request\n再继续"
	for _, stage := range []types.PipelineStage{
		types.StageAnalyze, types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		if priorConvVisibleForStage(types.PriorConvPolicyNever, stage, obj) {
			t.Errorf("policy=never must hide Prior for stage=%s", stage)
		}
	}
}

// TestPriorConvVisibleForStage_UnknownFallback — unknown / empty
// policies fall back to "always" so a partially-initialised config
// doesn't accidentally blind every stage.
func TestPriorConvVisibleForStage_UnknownFallback(t *testing.T) {
	if !priorConvVisibleForStage("", types.StageExplore, "x") {
		t.Error("empty policy must default to visible (always fallback)")
	}
	if !priorConvVisibleForStage("unknown-value", types.StageExplore, "x") {
		t.Error("unknown policy must default to visible (always fallback)")
	}
}

func TestPriorConvVisibleForStage_ControlInputAlwaysHidden(t *testing.T) {
	obj := "## Prior conversation\nold\n\n## Current request\n\\q"
	for _, stage := range []types.PipelineStage{
		types.StageAnalyze, types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		if priorConvVisibleForStage(types.PriorConvPolicyAlways, stage, obj) {
			t.Errorf("control input must hide Prior for stage=%s", stage)
		}
	}
}
