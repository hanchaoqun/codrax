package types

// trace_causal_projection_xlane_test.go — XLANE-1 修复轮 P2 silent-face pins
// (对抗复核, 2026-07-16): the two aggregation-side consumers of the
// ChainAnchorRepresentedByChainSeat marker were live but unpinned — removing
// either left the suite green while a merged view silently dropped (P2-①) or
// re-Σ'd (P2-②) the represented account.

import "testing"

// P2-①: the R1 same-fact survivor merge carries the represented marker
// OR-monotone — a survivor absorbing a represented loser (or a represented
// survivor absorbing a plain loser) keeps the marker, so the merged view can
// never resurrect the satellite onto the chain tier wordlessly.
func TestXLANEAbsorbSameFactCarriesRepresentedMarker(t *testing.T) {
	survivor := TraceCausalProjectionNode{Subject: "shadowhook-task-64305", Object: "scheduler_latency"}
	loser := TraceCausalProjectionNode{Subject: "shadowhook-task-64305", Object: "scheduler_latency",
		ChainAnchorRepresentedByChainSeat: true}
	traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
	if !survivor.ChainAnchorRepresentedByChainSeat {
		t.Fatalf("P2-①: the survivor must inherit the represented marker from the absorbed view")
	}
	survivor2 := TraceCausalProjectionNode{Subject: "shadowhook-task-64305", Object: "scheduler_latency",
		ChainAnchorRepresentedByChainSeat: true}
	traceCausalProjectionAbsorbSameFact(&survivor2, TraceCausalProjectionNode{Subject: "shadowhook-task-64305", Object: "scheduler_latency"}, map[string]bool{}, nil)
	if !survivor2.ChainAnchorRepresentedByChainSeat {
		t.Fatalf("P2-①: a represented survivor must keep its marker across the merge")
	}
}

// P2-②: the display-side anchorForm key forks on the represented marker
// (mirror of the engine rootCauseFamilyFoldAnchorFormKey) — pinned both as
// the direct value and behaviorally: a represented row never buckets into a
// plain same-(subject, object) R2 ×N group (re-Σ would double the account
// the chain seat already represents).
func TestXLANEAggregateAnchorFormKeyForksOnRepresentedMarker(t *testing.T) {
	plain := TraceCausalProjectionNode{Subject: "shadowhook-task-64305", Object: "runnable"}
	represented := TraceCausalProjectionNode{Subject: "shadowhook-task-64305", Object: "runnable",
		ChainAnchorRepresentedByChainSeat: true}
	if traceCausalProjectionAnchorFormKey(plain) != "" ||
		traceCausalProjectionAnchorFormKey(represented) != "anchor_represented" {
		t.Fatalf("P2-②: anchor form key drifted: plain=%q represented=%q",
			traceCausalProjectionAnchorFormKey(plain), traceCausalProjectionAnchorFormKey(represented))
	}
	build := func(represented bool, impact float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: "shadowhook-task-64305", Object: "runnable", ChainRelevance: "adjacent",
			ImpactMS: impact, CumulativeImpactMS: impact,
			ChainAnchorRepresentedByChainSeat: represented,
		}
	}
	nodes := []TraceCausalProjectionNode{build(false, 1.0), build(false, 2.0), build(true, 23.471)}
	out := traceCausalProjectionAggregateSameKind(nodes)
	var representedRows, plainRows int
	for _, node := range out {
		if node.ChainAnchorRepresentedByChainSeat {
			representedRows++
			if node.MergedCount > 1 {
				t.Fatalf("P2-②: the represented row must never merge into the plain ×N family: %+v", node)
			}
		} else {
			plainRows++
		}
	}
	if representedRows != 1 || plainRows == 0 {
		t.Fatalf("P2-②: the represented account must stand alone beside the plain rows: %+v", out)
	}
}
