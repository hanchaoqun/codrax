package binder

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRelevanceIgnoresNodeObjectiveSurfaceText(t *testing.T) {
	h := types.Hypothesis{
		ID:        "h1",
		Statement: "Registry membership is resolved through SubAgentRuntime",
	}
	withSurface := types.TaskNode{
		ID:        "n1",
		Type:      types.NodeFinalize,
		Objective: "Render Registry membership and SubAgentRuntime details.",
	}
	withoutSurface := types.TaskNode{
		ID:        "n2",
		Type:      types.NodeFinalize,
		Objective: "Render the final answer.",
	}

	gotWithSurface := relevance(h, withSurface)
	gotWithoutSurface := relevance(h, withoutSurface)
	if gotWithSurface != gotWithoutSurface {
		t.Fatalf("node objective prose must not affect binder relevance: with=%v without=%v", gotWithSurface, gotWithoutSurface)
	}
}

func TestRelevanceUsesTypedSearchHints(t *testing.T) {
	h := types.Hypothesis{
		ID:        "h1",
		Statement: "Registry membership is resolved through SubAgentRuntime",
	}
	withHint := types.TaskNode{
		ID:   "n1",
		Type: types.NodeEvidence,
		SearchHints: types.SearchHints{
			EntityIDs: []string{"registry", "subagentruntime"},
		},
	}
	withoutHint := types.TaskNode{
		ID:   "n2",
		Type: types.NodeEvidence,
	}

	if relevance(h, withHint) <= relevance(h, withoutHint) {
		t.Fatalf("typed search hints should increase binder relevance: with=%v without=%v", relevance(h, withHint), relevance(h, withoutHint))
	}
}
