package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func selfSymptomFoldWitness() types.TraceCausalProjection {
	const windowStart, windowEnd = 5.0, 5.007
	state := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "state",
		Subject: "app-100", Object: "s_sleep", StateKind: "s_sleep",
		Predicate: "wakeup_causal_impact", ChainRelevance: "on_chain",
		Causality: "on_wakeup_chain", ImpactMS: 5, CumulativeImpactMS: 5,
		StartTs: 5, EndTs: 5.005, LineStart: 3, LineEnd: 9,
		QueryWindowStartTs: windowStart, QueryWindowEndTs: windowEnd,
	}
	symptom := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "symptom",
		Subject: "app-100", Object: "sleep_wait", StateKind: "s_sleep",
		Predicate: "root_cause_target_self_state", Tier: types.TraceCausalTierTargetSelfState,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		ImpactMS: 5, CumulativeImpactMS: 5, LineStart: 3, LineEnd: 6,
		QueryWindowStartTs: windowStart, QueryWindowEndTs: windowEnd,
	}
	runnable := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "runnable",
		Subject: "app-100", Object: "runnable_wait", StateKind: "runnable",
		Predicate: "root_cause_primary", ChainRelevance: "on_chain",
		ImpactMS: 0.8, CumulativeImpactMS: 0.8, LineStart: 6, LineEnd: 9,
		QueryWindowStartTs: windowStart, QueryWindowEndTs: windowEnd,
	}
	return types.TraceCausalProjection{
		WakeupPath:     []string{"worker-200", "app-100"},
		WindowStartTs:  windowStart,
		WindowEndTs:    windowEnd,
		OnChainCauses:  []types.TraceCausalProjectionNode{runnable, state},
		AdjacentCauses: []types.TraceCausalProjectionNode{symptom},
	}
}

func TestFocusedSleepStateAndTargetSymptomShareOneDisplaySeat(t *testing.T) {
	projection := selfSymptomFoldWitness()
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)
	if len(model.SelfRows) != 2 {
		t.Fatalf("runnable + one physical sleep published %d self rows, want two: %+v", len(model.SelfRows), model.SelfRows)
	}
	var row runtimeTraceProjTreeRow
	for _, candidate := range model.SelfRows {
		if candidate.Node.StateKind == "s_sleep" {
			row = candidate
		}
	}
	if row.SelfSymptomRelocated || len(row.SelfSymptomFoldPeers) != 1 {
		t.Fatalf("scheduler-state row must carry one evidence-only symptom peer: %+v", row)
	}
	if got := runtimeTraceProjTargetSymptomMS(model); got != 5.8 {
		t.Fatalf("display fold changed the symptom denominator: got %.3fms, want 5.800ms", got)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if got := strings.Count(fence, "自身·sleep 5.000ms"); got != 1 {
		t.Fatalf("same sleep segment must render once, got %d:\n%s", got, fence)
	}
	for _, want := range []string{"症状而非根因", "[E2+E3]"} {
		if !strings.Contains(fence, want) {
			t.Fatalf("folded sleep row missing %q:\n%s", want, fence)
		}
	}
}

func TestFocusedSleepSymptomFoldFailsOpenOnAnyHardIdentityMismatch(t *testing.T) {
	base := selfSymptomFoldWitness()
	tests := []struct {
		name   string
		mutate func(*types.TraceCausalProjectionNode)
	}{
		{"different impact", func(n *types.TraceCausalProjectionNode) { n.ImpactMS = 4.9 }},
		{"different cumulative", func(n *types.TraceCausalProjectionNode) { n.CumulativeImpactMS = 4.9 }},
		{"different state", func(n *types.TraceCausalProjectionNode) { n.StateKind = "runnable" }},
		{"different segment start", func(n *types.TraceCausalProjectionNode) { n.LineStart = 4 }},
		{"different query window", func(n *types.TraceCausalProjectionNode) { n.QueryWindowStartTs = 5.1; n.QueryWindowEndTs = 5.2 }},
		{"missing query window", func(n *types.TraceCausalProjectionNode) { n.QueryWindowStartTs = 0; n.QueryWindowEndTs = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := base
			projection.AdjacentCauses = append([]types.TraceCausalProjectionNode(nil), base.AdjacentCauses...)
			test.mutate(&projection.AdjacentCauses[0])
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
			if len(model.SelfRows) != 3 {
				t.Fatalf("mismatched views must remain two auditable rows, got %d: %+v", len(model.SelfRows), model.SelfRows)
			}
		})
	}
}

func TestFocusedSleepSymptomFoldRejectsAmbiguousCarrier(t *testing.T) {
	projection := selfSymptomFoldWitness()
	second := projection.OnChainCauses[1]
	second.EvidenceID = "second-state"
	projection.OnChainCauses = append(projection.OnChainCauses, second)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(model.SelfRows) != 4 {
		t.Fatalf("two possible state carriers must fail open, got %d: %+v", len(model.SelfRows), model.SelfRows)
	}
}
