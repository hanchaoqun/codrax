package types

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
)

// HMC377 F4 — the hop / classified order of the explicit-window causal
// projection. Two rows that both carry a recorded chain depth keep the order
// they had at d5ef1c6fe (shallower hop first, value inside one depth); every
// row without a recorded depth takes the position its own typed fields give it
// (traceCausalProjectionChainOrderPosition: after every recorded depth, a
// producer fold of hops in the first hop tier), so the relation is a strict
// weak order and the order of two published rows never depends on any other
// row.

var hmc377F4Path = []string{"w3-30", "w2-20", "w1-10", "t-1"}

// hmc377F4BaseHopLess / hmc377F4BaseNodeLess replicate the d5ef1c6fe
// comparators verbatim (depth arm only when both rows carry a depth; rank arm
// only when both rows carry a rank) — the reference the both-depth order must
// match.
func hmc377F4BaseNodeLess(a, b TraceCausalProjectionNode) bool {
	if a.CumulativeImpactMS != b.CumulativeImpactMS {
		return a.CumulativeImpactMS > b.CumulativeImpactMS
	}
	if a.ImpactMS != b.ImpactMS {
		return a.ImpactMS > b.ImpactMS
	}
	if a.Confidence != b.Confidence {
		return a.Confidence > b.Confidence
	}
	if a.Rank > 0 && b.Rank > 0 && a.Rank != b.Rank {
		return a.Rank < b.Rank
	}
	return TraceObservationCoordinateLess(a.EvidenceID, b.EvidenceID)
}

func hmc377F4BaseHopLess(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	if ao, bo := traceCausalProjectionNodeOnChain(a), traceCausalProjectionNodeOnChain(b); ao != bo {
		return ao
	}
	if ai, bi := traceCausalProjectionNodeInPath(a, pathIndex), traceCausalProjectionNodeInPath(b, pathIndex); ai != bi {
		return ai
	}
	if a.ChainDepth > 0 && b.ChainDepth > 0 && a.ChainDepth != b.ChainDepth {
		return a.ChainDepth < b.ChainDepth
	}
	return hmc377F4BaseNodeLess(a, b)
}

// hmc377F4Row draws one projection row. Small value / depth / rank alphabets
// make ties, mixed depth presence and every typed position class frequent.
func hmc377F4Row(r *rand.Rand, id int) TraceCausalProjectionNode {
	roles := []string{TraceCausalRolePrimaryRootCause, TraceCausalRoleCausalHop, TraceCausalRoleCausalHop, TraceCausalRoleRootCauseContext, TraceCausalRoleSemanticSpan}
	relevances := []string{"on_chain", "on_chain", "on_chain", "adjacent", "background", ""}
	causalities := []string{"on_wakeup_chain", "on_wakeup_chain", "", "self_deterministic", "self_wall_clock", "adjacent_to_wakeup_chain"}
	subjects := append(append([]string{}, hmc377F4Path...), "off-40", "off-50", "")
	values := []float64{0, 1, 2, 2.5, 5, 9}
	kinds := []string{"wakeup_causal_impact", "root_cause_rank", "trace_semantic_span", "critical_blocking", "root_evidence:long_sleep"}
	n := TraceCausalProjectionNode{
		Role:               roles[r.Intn(len(roles))],
		ChainRelevance:     relevances[r.Intn(len(relevances))],
		Causality:          causalities[r.Intn(len(causalities))],
		Subject:            subjects[r.Intn(len(subjects))],
		ChainDepth:         r.Intn(5),
		CumulativeImpactMS: values[r.Intn(len(values))],
		Confidence:         []float64{0.78, 0.82}[r.Intn(2)],
		Rank:               r.Intn(4),
		EvidenceID:         fmt.Sprintf("trace_query:x#%s:%d", kinds[r.Intn(len(kinds))], id),
	}
	n.ImpactMS = n.CumulativeImpactMS
	if r.Intn(8) == 0 {
		n.Subject, n.ChainDepth, n.MergedWireFold, n.MergedCount = "", 0, true, 3
	}
	return n
}

func hmc377F4Classified(pathIndex map[string]int) func(a, b TraceCausalProjectionNode) bool {
	return func(a, b TraceCausalProjectionNode) bool { return traceCausalProjectionClassifiedLess(a, b, pathIndex) }
}

// hmc377F4StrictWeakOrderViolation returns a description of the first
// violation of irreflexivity, asymmetry, transitivity or transitivity of
// incomparability, "" when there is none.
func hmc377F4StrictWeakOrderViolation(nodes []TraceCausalProjectionNode, less func(a, b TraceCausalProjectionNode) bool) string {
	inc := func(x, y TraceCausalProjectionNode) bool { return !less(x, y) && !less(y, x) }
	for i := range nodes {
		if less(nodes[i], nodes[i]) {
			return fmt.Sprintf("reflexive: %s", nodes[i].EvidenceID)
		}
		for j := range nodes {
			if less(nodes[i], nodes[j]) && less(nodes[j], nodes[i]) {
				return fmt.Sprintf("asymmetry: %s / %s", nodes[i].EvidenceID, nodes[j].EvidenceID)
			}
			for k := range nodes {
				a, b, c := nodes[i], nodes[j], nodes[k]
				if less(a, b) && less(b, c) && !less(a, c) {
					return fmt.Sprintf("intransitive: %s < %s < %s", a.EvidenceID, b.EvidenceID, c.EvidenceID)
				}
				if inc(a, b) && inc(b, c) && !inc(a, c) {
					return fmt.Sprintf("incomparability not transitive: %s ~ %s ~ %s", a.EvidenceID, b.EvidenceID, c.EvidenceID)
				}
			}
		}
	}
	return ""
}

func hmc377F4IDs(nodes []TraceCausalProjectionNode, keep func(TraceCausalProjectionNode) bool) string {
	var ids []string
	for _, n := range nodes {
		if keep(n) {
			ids = append(ids, n.EvidenceID)
		}
	}
	return strings.Join(ids, ",")
}

// Property: over random row sets the classified and hop comparators are
// strict weak orders, and invisible rows (the valueless evidence_fact
// mirrors, never published) never change the relative order of the
// published rows. The generator is checked to have teeth: it finds the
// d5ef1c6fe comparator's cycles.
func TestHMC377F4ProjectionOrderIsAStrictWeakOrderOverRandomRows(t *testing.T) {
	pathIndex := traceCausalProjectionPathIndex(hmc377F4Path)
	classified := hmc377F4Classified(pathIndex)
	hop := func(a, b TraceCausalProjectionNode) bool { return traceCausalProjectionHopLess(a, b, pathIndex) }
	base := func(a, b TraceCausalProjectionNode) bool { return hmc377F4BaseHopLess(a, b, pathIndex) }
	baseViolations := 0
	for seed := int64(1); seed <= 400; seed++ {
		r := rand.New(rand.NewSource(seed))
		nodes := make([]TraceCausalProjectionNode, 9)
		for i := range nodes {
			nodes[i] = hmc377F4Row(r, i+1)
		}
		for name, less := range map[string]func(a, b TraceCausalProjectionNode) bool{"classified": classified, "hop": hop} {
			if v := hmc377F4StrictWeakOrderViolation(nodes, less); v != "" {
				t.Fatalf("seed %d %s comparator: %s", seed, name, v)
			}
		}
		if hmc377F4StrictWeakOrderViolation(nodes, base) != "" {
			baseViolations++
		}
		published := func(n TraceCausalProjectionNode) bool { return !strings.Contains(n.EvidenceID, "#evidence_fact:") }
		want := hmc377F4IDs(traceCausalProjectionSortYieldingApart(append([]TraceCausalProjectionNode(nil), nodes...), classified), published)
		for extra := 1; extra <= 12; extra++ {
			// Invisible rows enter at random places; the published rows keep
			// their own publication order (the input order of tied rows).
			in := append([]TraceCausalProjectionNode(nil), nodes...)
			for i := 0; i < extra; i++ {
				mirror := TraceCausalProjectionNode{Role: TraceCausalRoleRootCauseContext, Subject: hmc377F4Path[r.Intn(len(hmc377F4Path))],
					ChainDepth: r.Intn(4), EvidenceID: fmt.Sprintf("trace_query:x#evidence_fact:root_cause_%d", i+1)}
				at := r.Intn(len(in) + 1)
				in = append(in[:at], append([]TraceCausalProjectionNode{mirror}, in[at:]...)...)
			}
			if got := hmc377F4IDs(traceCausalProjectionSortYieldingApart(in, classified), published); got != want {
				t.Fatalf("seed %d: %d invisible rows reordered the published rows:\n got %s\nwant %s", seed, extra, got, want)
			}
		}
	}
	if baseViolations == 0 {
		t.Fatal("the generator must find the d5ef1c6fe comparator's cycles (else this property test has no teeth)")
	}
}

// Pin: on row sets where every row carries a recorded chain depth, the flat
// order is exactly the d5ef1c6fe order (on- and off-path rows, ranked and
// unranked boards). On mixed sets every pair of rows that both carry a depth
// keeps the d5ef1c6fe pairwise order, which the d5ef1c6fe sort itself did not
// guarantee once a row without a depth sat between them.
func TestHMC377F4BothDepthOrderIsTheBaseOrder(t *testing.T) {
	pathIndex := traceCausalProjectionPathIndex(hmc377F4Path)
	for seed := int64(1); seed <= 400; seed++ {
		r := rand.New(rand.NewSource(seed))
		ranked := seed%2 == 0
		var nodes []TraceCausalProjectionNode
		for i := 0; i < 14; i++ {
			n := hmc377F4Row(r, i+1)
			n.Role, n.ChainRelevance, n.Causality, n.MergedWireFold, n.MergedCount = TraceCausalRoleCausalHop, "on_chain", "on_wakeup_chain", false, 0
			if n.Subject == "" {
				n.Subject = "off-60"
			}
			n.ChainDepth = 1 + r.Intn(4)
			// A board is either all ranked or all unranked, so the
			// d5ef1c6fe rank arm is itself well defined on the set.
			n.Rank = 0
			if ranked {
				n.Rank = i + 1
			}
			nodes = append(nodes, n)
		}
		r.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
		got := append([]TraceCausalProjectionNode(nil), nodes...)
		want := append([]TraceCausalProjectionNode(nil), nodes...)
		sort.SliceStable(got, func(i, j int) bool { return traceCausalProjectionHopLess(got[i], got[j], pathIndex) })
		sort.SliceStable(want, func(i, j int) bool { return hmc377F4BaseHopLess(want[i], want[j], pathIndex) })
		all := func(TraceCausalProjectionNode) bool { return true }
		if g, w := hmc377F4IDs(got, all), hmc377F4IDs(want, all); g != w {
			t.Fatalf("seed %d: both-depth order differs from d5ef1c6fe:\n got %s\nwant %s", seed, g, w)
		}
	}

	// Mixed set (Douyin reboot5 [140.911304, 141.835458) TID 11105 in-path
	// hop shape): the analysed thread's own hops carry no depth.
	path := []string{"d3-317", "d2-280", "d1-293", "t-105"}
	pi := traceCausalProjectionPathIndex(path)
	hopRow := func(id, subject string, depth int, ms float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{Role: TraceCausalRoleCausalHop, EvidenceID: "trace_query:x#wakeup_causal_impact:" + id, Subject: subject,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: depth, CumulativeImpactMS: ms, ImpactMS: ms, Confidence: 0.78}
	}
	mixed := []TraceCausalProjectionNode{
		hopRow("1", "t-105", 0, 83.393), hopRow("2", "d1-293", 1, 28.105), hopRow("3", "d2-280", 2, 21.734),
		hopRow("4", "d3-317", 3, 19.726), hopRow("18", "t-105", 0, 7.187), hopRow("32", "d2-280", 2, 1.965), hopRow("31", "t-105", 0, 1.341),
	}
	base := append([]TraceCausalProjectionNode(nil), mixed...)
	sort.SliceStable(base, func(i, j int) bool { return hmc377F4BaseHopLess(base[i], base[j], pi) })
	if got := hmc377F4IDs(base, func(TraceCausalProjectionNode) bool { return true }); !strings.Contains(got, ":4,trace_query:x#wakeup_causal_impact:18,trace_query:x#wakeup_causal_impact:32") {
		t.Fatalf("witness: the d5ef1c6fe sort listed the depth-3 hop before the depth-2 hop on this set: %s", got)
	}
	now := append([]TraceCausalProjectionNode(nil), mixed...)
	sort.SliceStable(now, func(i, j int) bool { return traceCausalProjectionHopLess(now[i], now[j], pi) })
	for i := range now {
		for j := i + 1; j < len(now); j++ {
			if a, b := now[i], now[j]; a.ChainDepth > 0 && b.ChainDepth > 0 && hmc377F4BaseHopLess(b, a, pi) {
				t.Fatalf("both-depth pair out of the d5ef1c6fe order: %s before %s", a.EvidenceID, b.EvidenceID)
			}
		}
	}
	// The hops by depth, then the analysed thread's own (depthless) hops by
	// value.
	want := "2,3,32,4,1,18,31"
	var got []string
	for _, n := range now {
		got = append(got, n.EvidenceID[strings.LastIndex(n.EvidenceID, ":")+1:])
	}
	if strings.Join(got, ",") != want {
		t.Fatalf("mixed in-path order: got %s want %s", strings.Join(got, ","), want)
	}
}

// Each depthless producer class sits at its documented position.
func TestHMC377F4DepthlessRowsTakeTheirTypedPosition(t *testing.T) {
	// NetEase reboot5 [153.763595, 153.887340) TID 11077 (protected)
	// classified on-chain context tier: the depth-1 and depth-2 rank rows keep
	// their order; the analysed thread's GC wait marker, the depth-1 member's
	// D/IO state seats and the analysed thread's own-work rows (no recorded
	// depth) follow by value. (The rendered projection is unchanged: the tree
	// places these rows by chain structure and lists flat seats by value.)
	path := []string{"wk-6934", "htd-11098", "app-11077"}
	pi := traceCausalProjectionPathIndex(path)
	row := func(id, role, subject, causality string, depth int, ms float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{Role: role, EvidenceID: "trace_query:x#" + id, Subject: subject, ChainRelevance: "on_chain",
			Causality: causality, ChainDepth: depth, CumulativeImpactMS: ms, ImpactMS: ms, Confidence: 0.82}
	}
	rows := []TraceCausalProjectionNode{
		row("root_cause_rank:7", TraceCausalRoleRootCauseContext, "app-11077", "self_wall_clock", 0, 0.015),
		row("root_cause_rank:5", TraceCausalRoleRootCauseContext, "htd-11098", "on_wakeup_chain", 0, 0.907),
		row("root_cause_rank:3", TraceCausalRoleRootCauseContext, "wk-6934", "on_wakeup_chain", 2, 3.813),
		row("root_cause_rank:6", TraceCausalRoleRootCauseContext, "app-11077", "self_wall_clock", 0, 0.727),
		row("root_cause_rank:2", TraceCausalRoleRootCauseContext, "htd-11098", "on_wakeup_chain", 1, 115.347),
		row("root_cause_rank:4", TraceCausalRoleRootCauseContext, "htd-11098", "on_wakeup_chain", 0, 1.800),
		row("trace_semantic_span:1", TraceCausalRoleSemanticSpan, "app-11077", "on_wakeup_chain", 0, 123.089),
	}
	sort.SliceStable(rows, func(i, j int) bool { return traceCausalProjectionClassifiedLess(rows[i], rows[j], pi) })
	var got []string
	for _, n := range rows {
		got = append(got, fmt.Sprintf("%.3f", n.CumulativeImpactMS))
	}
	if want := "115.347,3.813,123.089,1.800,0.907,0.727,0.015"; strings.Join(got, ",") != want {
		t.Fatalf("NetEase context-tier order: got %s want %s", strings.Join(got, ","), want)
	}
	after := traceCausalProjectionPositiveOrderKey(0)
	fold := TraceCausalProjectionNode{Role: TraceCausalRoleCausalHop, ChainRelevance: "on_chain", Causality: "on_wakeup_chain", MergedWireFold: true, MergedCount: 3}
	offChainFold := fold
	offChainFold.ChainRelevance, offChainFold.Causality = "background", ""
	for _, c := range []struct {
		name string
		node TraceCausalProjectionNode
		want int
	}{
		{"recorded depth", row("a", TraceCausalRoleCausalHop, "off-1", "on_wakeup_chain", 3, 1), 3},
		{"analysed thread's own hop", row("b", TraceCausalRoleCausalHop, "app-11077", "on_wakeup_chain", 0, 1), after},
		{"chain member's state seat", row("c", TraceCausalRoleRootCauseContext, "htd-11098", "on_wakeup_chain", 0, 1), after},
		{"own work", row("d", TraceCausalRoleRootCauseContext, "app-11077", "self_deterministic", 0, 1), after},
		{"off-path credential row", row("e", TraceCausalRoleRootCauseContext, "off-1", "on_wakeup_chain", 0, 1), after},
		{"producer fold of hops", fold, 1},
		{"off-chain fold", offChainFold, after},
	} {
		if got := traceCausalProjectionChainOrderPosition(c.node); got != c.want {
			t.Fatalf("%s: position %d, want %d", c.name, got, c.want)
		}
	}
	// A semantic record and its rank-row twin tie on every value: the
	// semantic record (the family's one seat) is listed first.
	sem := row("trace_semantic_span:1", TraceCausalRoleSemanticSpan, "app-11077", "self_deterministic", 0, 3.868)
	twin := row("root_cause_rank:2", TraceCausalRoleRootCauseContext, "app-11077", "self_deterministic", 0, 3.868)
	twin.Rank = 2
	if !traceCausalProjectionClassifiedLess(sem, twin, pi) || traceCausalProjectionClassifiedLess(twin, sem, pi) {
		t.Fatal("a semantic record must precede its rank-row twin of equal value")
	}
}
