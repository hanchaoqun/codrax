package tool

// answer_document_projection_self_symptom_onchain_fold_test.go — ANSWERFACE-1
// 件4 (§29.140 G8, 2026-07-19): the SELF-TWIN fold covers the ON-CHAIN lane.
// Witness (semantic_span E1/E2): a root_cause_target_self_state sleep row
// published with chain_relevance=on_chain walks the chain-universe self pass
// (not the adjacent/background relocation lane the original fold guarded), so
// the same physical sleep segment rendered as two byte-identical 自身·sleep
// rows with zero mutual reference. Same typed matcher, same fail-open
// discipline as the relocation-lane fold (answer_document_projection_
// self_symptom_fold_test.go pins that lane).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// selfSymptomOnChainFoldWitness is the semantic_span-isomorphic shape: the
// symptom row rides ON-CHAIN (OnChainCauses bucket) instead of adjacent.
func selfSymptomOnChainFoldWitness() types.TraceCausalProjection {
	projection := selfSymptomFoldWitness()
	symptom := projection.AdjacentCauses[0]
	symptom.ChainRelevance = "on_chain"
	symptom.Causality = "self_wall_clock"
	projection.AdjacentCauses = nil
	// Order matters for the sequential in-order fold: the scheduler-state
	// carrier precedes the symptom row (the compile emits wakeup_causal rows
	// before rank rows — the semantic_span production order).
	projection.OnChainCauses = append(append([]types.TraceCausalProjectionNode(nil),
		projection.OnChainCauses...), symptom)
	return projection
}

func TestFocusedSleepOnChainSymptomSharesOneDisplaySeat(t *testing.T) {
	projection := selfSymptomOnChainFoldWitness()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(model.SelfRows) != 2 {
		t.Fatalf("runnable + one physical sleep published %d self rows, want two: %+v", len(model.SelfRows), model.SelfRows)
	}
	var row runtimeTraceProjTreeRow
	for _, candidate := range model.SelfRows {
		if candidate.Node.StateKind == "s_sleep" {
			row = candidate
		}
	}
	if row.Node.Predicate != "wakeup_causal_impact" {
		t.Fatalf("the scheduler-state row must stay the single seat: %+v", row.Node)
	}
	if row.SelfSymptomRelocated || len(row.SelfSymptomFoldPeers) != 1 {
		t.Fatalf("the on-chain symptom must fold as an evidence-only peer: %+v", row)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if got := strings.Count(fence, "自身·sleep 5.000ms"); got != 1 {
		t.Fatalf("same sleep segment must render once, got %d:\n%s", got, fence)
	}
	if !strings.Contains(fence, "[E2+E3]") {
		t.Fatalf("the folded row must carry both evidence tags:\n%s", fence)
	}
}

func TestFocusedSleepOnChainSymptomFoldFailsOpenOnMismatch(t *testing.T) {
	projection := selfSymptomOnChainFoldWitness()
	// A diverging display caliber is a DIFFERENT account — never fold (W-A
	// 不同账目绝不折); both rows stay auditable.
	projection.OnChainCauses = append([]types.TraceCausalProjectionNode(nil), projection.OnChainCauses...)
	projection.OnChainCauses[len(projection.OnChainCauses)-1].ImpactMS = 4.9
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(model.SelfRows) != 3 {
		t.Fatalf("mismatched views must remain two auditable rows, got %d: %+v", len(model.SelfRows), model.SelfRows)
	}
}
