package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func testAgentSettingsForExploreBudget() types.AgentSettings {
	return types.AgentSettings{
		MaxIterations:               20,
		SubTopicExplorerBudgetExtra: 3,
		ExplorerScaledIterMax:       35,
	}
}

func TestApplyExploreIterationScalingForSingleComplexRootCauseTrace(t *testing.T) {
	ctx := &types.AgentContext{}
	base, adjusted, reason, ok := applyExploreIterationScalingForRequest(ctx, types.RequestModel{
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexityComplex,
	}, testAgentSettingsForExploreBudget())
	if !ok || base != 20 || adjusted != 35 || ctx.MaxIterOverride != 35 {
		t.Fatalf("single complex root-cause trace scaling = ok:%t base:%d adjusted:%d override:%d", ok, base, adjusted, ctx.MaxIterOverride)
	}
	if reason != "single-topic-complex-chain" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestApplyExploreIterationScalingForSimpleGenericDoesNotRaiseBudget(t *testing.T) {
	ctx := &types.AgentContext{}
	_, _, _, ok := applyExploreIterationScalingForRequest(ctx, types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioGeneric,
		Complexity: types.ComplexitySimple,
	}, testAgentSettingsForExploreBudget())
	if ok || ctx.MaxIterOverride != 0 {
		t.Fatalf("simple generic request should keep default budget, ok=%t override=%d", ok, ctx.MaxIterOverride)
	}
}

func TestApplyExploreIterationScalingForMultiTopicStillUsesSharedHelper(t *testing.T) {
	ctx := &types.AgentContext{}
	base, adjusted, reason, ok := applyExploreIterationScalingForRequest(ctx, types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		SubTopics: []types.SubTopic{
			{Summary: "A"},
			{Summary: "B"},
			{Summary: "C"},
		},
	}, testAgentSettingsForExploreBudget())
	if !ok || base != 20 || adjusted != 29 || ctx.MaxIterOverride != 29 {
		t.Fatalf("multi-topic scaling = ok:%t base:%d adjusted:%d override:%d", ok, base, adjusted, ctx.MaxIterOverride)
	}
	if reason != "multi-topic" {
		t.Fatalf("reason=%q", reason)
	}
}
