package types

import (
	"fmt"
	"strings"
	"testing"
)

// HMC377 (found while landing the E2 dependency-credit bound): the
// hop/classified comparator compared chain depth only when BOTH rows carried a
// positive depth and fell back to value otherwise. That relation is not a
// strict weak order — three on-chain rows (depth 2 / 4.269 ms, depth 0 /
// 3.332 ms, depth 1 / 2.215 ms) form a cycle — so a stable sort's output
// depended on which OTHER rows were present: the valueless mirror records
// (never published) reordered the published on-chain rows and moved a
// different row behind the fold cap (tieba CookieMonsterCl-59843 board,
// TestISPGAPMirrorExemptChainedBoardByteIdentical). The order of two
// published rows must depend on those two rows only.

func hmc377OrderNode(id string, depth int, ms float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleCausalHop, EvidenceID: id, Subject: "t-" + id, Object: "runnable",
		ChainRelevance: "on_chain", ChainDepth: depth, ImpactMS: ms, CumulativeImpactMS: ms, Confidence: 0.8,
	}
}

func TestHMC377ClassifiedOrderIsAStrictWeakOrder(t *testing.T) {
	a := hmc377OrderNode("trace_query:x#wakeup_causal_impact:3", 2, 4.269)
	b := hmc377OrderNode("trace_query:x#wakeup_causal_impact:21", 0, 3.332)
	c := hmc377OrderNode("trace_query:x#wakeup_causal_impact:23", 1, 2.215)
	less := func(x, y TraceCausalProjectionNode) bool { return traceCausalProjectionClassifiedLess(x, y, nil) }
	nodes := []TraceCausalProjectionNode{a, b, c}
	for i := range nodes {
		for j := range nodes {
			for k := range nodes {
				if less(nodes[i], nodes[j]) && less(nodes[j], nodes[k]) && !less(nodes[i], nodes[k]) && i != k {
					t.Fatalf("intransitive: %s < %s < %s but not %s < %s", nodes[i].EvidenceID, nodes[j].EvidenceID, nodes[k].EvidenceID, nodes[i].EvidenceID, nodes[k].EvidenceID)
				}
			}
		}
	}
	// Population independence: invisible rows (no relevance) never change the
	// relative order of the published rows.
	order := func(extra int) string {
		in := []TraceCausalProjectionNode{c, a, b}
		for i := 0; i < extra; i++ {
			in = append(in, TraceCausalProjectionNode{Role: TraceCausalRoleRootCauseContext, EvidenceID: fmt.Sprintf("trace_query:x#evidence_fact:%d", i+1), Subject: "m", Object: "runnable", ImpactMS: 0})
		}
		sorted := traceCausalProjectionSortYieldingApart(in, less)
		var ids []string
		for _, n := range sorted {
			if n.ChainRelevance == "on_chain" {
				ids = append(ids, n.EvidenceID)
			}
		}
		return strings.Join(ids, ",")
	}
	base := order(0)
	for extra := 1; extra <= 40; extra++ {
		if got := order(extra); got != base {
			t.Fatalf("published order depends on %d invisible rows: %s vs %s", extra, got, base)
		}
	}
	// Equal values: chain depth still decides (shallower first, a row with no
	// recorded depth after every row with one), then the rank ordinal (ranked
	// before unranked), then the coordinate order.
	d1 := hmc377OrderNode("trace_query:x#wakeup_causal_impact:9", 1, 1.0)
	d2 := hmc377OrderNode("trace_query:x#wakeup_causal_impact:8", 2, 1.0)
	d0 := hmc377OrderNode("trace_query:x#wakeup_causal_impact:7", 0, 1.0)
	if !less(d1, d2) || !less(d2, d0) || !less(d1, d0) {
		t.Fatalf("equal-value rows must order by recorded depth: d1<d2=%v d2<d0=%v d1<d0=%v", less(d1, d2), less(d2, d0), less(d1, d0))
	}
	r1, r0, r2 := d1, d1, d1
	r1.EvidenceID, r1.Rank = "trace_query:x#root_cause_rank:5", 3
	r0.EvidenceID, r0.Rank = "trace_query:x#root_cause_rank:1", 0
	r2.EvidenceID, r2.Rank = "trace_query:x#root_cause_rank:9", 7
	if !traceCausalProjectionNodeLess(r1, r2) || !traceCausalProjectionNodeLess(r2, r0) || !traceCausalProjectionNodeLess(r1, r0) {
		t.Fatalf("equal-value rows must order ranked-before-unranked transitively")
	}
}

// HMC377 E2-R3 — two roles of the same rank (root_cause_context and
// semantic_span) are compared by the hop order, never left incomparable: the
// relative order of two context rows must not depend on an unrelated
// semantic row sorted with them.
func TestHMC377ClassifiedOrderSameRankRolesAreComparable(t *testing.T) {
	a := TraceCausalProjectionNode{Role: TraceCausalRoleRootCauseContext, ChainRelevance: "adjacent", Subject: "a-1", CumulativeImpactMS: 5}
	s := TraceCausalProjectionNode{Role: TraceCausalRoleSemanticSpan, ChainRelevance: "adjacent", Subject: "s-2", CumulativeImpactMS: 3}
	c := TraceCausalProjectionNode{Role: TraceCausalRoleRootCauseContext, ChainRelevance: "adjacent", Subject: "c-3", CumulativeImpactMS: 1}
	less := func(x, y TraceCausalProjectionNode) bool {
		return traceCausalProjectionClassifiedLess(x, y, map[string]int{})
	}
	if !less(a, s) || !less(s, c) || !less(a, c) {
		t.Fatalf("mixed same-rank roles must follow the hop order: a<s=%v s<c=%v a<c=%v", less(a, s), less(s, c), less(a, c))
	}
	order := func(in []TraceCausalProjectionNode) string {
		sorted := traceCausalProjectionSortYieldingApart(in, less)
		var ids []string
		for _, n := range sorted {
			if n.Subject != "s-2" {
				ids = append(ids, n.Subject)
			}
		}
		return strings.Join(ids, ",")
	}
	if with, without := order([]TraceCausalProjectionNode{c, s, a}), order([]TraceCausalProjectionNode{c, a}); with != without {
		t.Fatalf("context order depends on an unrelated semantic row: with=%s without=%s", with, without)
	}
}
