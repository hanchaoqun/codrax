package types

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
)

// HMC377 F4 — the hop / classified order of the explicit-window causal
// projection. Two rows that both carry a recorded chain depth keep the
// d5ef1c6fe depth order (shallower hop first) and, inside one depth, the
// d5ef1c6fe value order (cumulative, impact, confidence). Only rows that tie on
// every value at one depth are ordered differently: d5ef1c6fe compared rank
// only when both rows carried one and otherwise fell through to the coordinate
// key; the tie key is now total (semantic record first, ranked rows by rank
// ahead of unranked ones, then the coordinate key). In the classified order
// two different roles of one role rank, which d5ef1c6fe left incomparable at
// every depth and value, now take the hop order (E2-R3). Every row without a
// recorded depth takes the position its own typed fields give it
// (traceCausalProjectionChainOrderPosition: after every recorded depth, a
// producer fold of hops in the first hop tier), so the relation is a strict
// weak order and the order of two published rows never depends on any other
// row.

var hmc377F4Path = []string{"w3-30", "w2-20", "w1-10", "t-1"}

// hmc377F4BaseHopLess / hmc377F4BaseNodeLess / hmc377F4BaseClassifiedLess
// replicate the d5ef1c6fe comparators verbatim (depth arm only when both rows
// carry a depth; rank arm only when both rows carry a rank; two different roles
// of one role rank incomparable) — the reference whose depth and value order
// the both-depth order must keep.
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

func hmc377F4BaseClassifiedLess(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	if ar, br := traceCausalProjectionChainRelevanceRank(a.ChainRelevance), traceCausalProjectionChainRelevanceRank(b.ChainRelevance); ar != br {
		return ar < br
	}
	if a.Role != b.Role {
		return traceCausalProjectionRoleRank(a.Role) < traceCausalProjectionRoleRank(b.Role)
	}
	return hmc377F4BaseHopLess(a, b, pathIndex)
}

// hmc377F4ValueTieAtOneDepth: the two rows share the hop prefix (on-chain,
// in-path, chain depth) and tie on every value key.
func hmc377F4ValueTieAtOneDepth(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	return traceCausalProjectionNodeOnChain(a) == traceCausalProjectionNodeOnChain(b) &&
		traceCausalProjectionNodeInPath(a, pathIndex) == traceCausalProjectionNodeInPath(b, pathIndex) &&
		a.ChainDepth == b.ChainDepth &&
		a.CumulativeImpactMS == b.CumulativeImpactMS && a.ImpactMS == b.ImpactMS && a.Confidence == b.Confidence
}

// hmc377F4BothDepthPairDomain is every combination of the fields the hop and
// classified comparators read, each row carrying a recorded chain depth.
func hmc377F4BothDepthPairDomain() []TraceCausalProjectionNode {
	var nodes []TraceCausalProjectionNode
	for _, cum := range []float64{1, 2} {
		for _, imp := range []float64{1, 2} {
			for _, conf := range []float64{0.78, 0.82} {
				for _, role := range []string{TraceCausalRolePrimaryRootCause, TraceCausalRoleCausalHop, TraceCausalRoleRootCauseContext, TraceCausalRoleSemanticSpan} {
					for _, rank := range []int{0, 1, 2} {
						for _, depth := range []int{1, 2} {
							for _, id := range []string{"trace_query:x#root_cause_rank:1", "trace_query:x#wakeup_causal_impact:1"} {
								for _, relevance := range []string{"on_chain", "adjacent"} {
									for _, subject := range []string{"w1-10", "off-40"} {
										nodes = append(nodes, TraceCausalProjectionNode{CumulativeImpactMS: cum, ImpactMS: imp, Confidence: conf,
											Role: role, Rank: rank, ChainDepth: depth, EvidenceID: id, ChainRelevance: relevance, Subject: subject})
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return nodes
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

// Pin: two rows that both carry a recorded chain depth keep the d5ef1c6fe
// depth order and, inside one depth, its value order. Over every pair of a
// domain of all the fields the comparators read:
//
//   - the hop order differs from d5ef1c6fe only on pairs that tie on every
//     value at one depth (the new total tie key);
//   - the classified order differs only on such value ties, and on pairs of
//     two different roles of one role rank that d5ef1c6fe left incomparable
//     (false both ways, at any depth and value), which now take the hop order
//     (E2-R3);
//   - no pair d5ef1c6fe ordered is reversed outside a value tie at one depth.
//
// Both difference classes must occur, so the pin cannot be read as "the
// both-depth order is the d5ef1c6fe order". On boards where no value tie meets
// a semantic record or a mix of ranked and unranked rows (every row a causal
// hop; a board all ranked with distinct ranks, or all unranked) the flat order
// is exactly the d5ef1c6fe order. On mixed sets every pair of rows that both
// carry a depth keeps the d5ef1c6fe depth and value order, which the d5ef1c6fe
// sort itself did not guarantee once a row without a depth sat between them.
func TestHMC377F4BothDepthRowsKeepTheBaseDepthAndValueOrder(t *testing.T) {
	pathIndex := traceCausalProjectionPathIndex(hmc377F4Path)
	domain := hmc377F4BothDepthPairDomain()
	hopTies, classifiedTies, classifiedSameRankRoles := 0, 0, 0
	for _, a := range domain {
		for _, b := range domain {
			now, base := traceCausalProjectionHopLess(a, b, pathIndex), hmc377F4BaseHopLess(a, b, pathIndex)
			if now != base {
				if !hmc377F4ValueTieAtOneDepth(a, b, pathIndex) {
					t.Fatalf("hop order differs from d5ef1c6fe outside a value tie at one depth: a=%+v b=%+v now=%t base=%t", a, b, now, base)
				}
				hopTies++
			}
			now, base = traceCausalProjectionClassifiedLess(a, b, pathIndex), hmc377F4BaseClassifiedLess(a, b, pathIndex)
			if now == base {
				continue
			}
			sameRelevance := traceCausalProjectionChainRelevanceRank(a.ChainRelevance) == traceCausalProjectionChainRelevanceRank(b.ChainRelevance)
			sameRoleRank := traceCausalProjectionRoleRank(a.Role) == traceCausalProjectionRoleRank(b.Role)
			switch {
			case sameRelevance && sameRoleRank && a.Role != b.Role && !hmc377F4BaseClassifiedLess(b, a, pathIndex) && !base &&
				now == traceCausalProjectionHopLess(a, b, pathIndex):
				classifiedSameRankRoles++
			case sameRelevance && sameRoleRank && hmc377F4ValueTieAtOneDepth(a, b, pathIndex):
				classifiedTies++
			default:
				t.Fatalf("classified order differs from d5ef1c6fe outside a value tie at one depth and outside the same-rank roles it left incomparable: a=%+v b=%+v now=%t base=%t", a, b, now, base)
			}
		}
	}
	if hopTies == 0 || classifiedTies == 0 || classifiedSameRankRoles == 0 {
		t.Fatalf("every difference class must occur on the domain (hop value ties %d, classified value ties %d, classified same-rank roles %d)",
			hopTies, classifiedTies, classifiedSameRankRoles)
	}

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
			t.Fatalf("seed %d: on a board where no value tie meets a semantic record or mixed rank presence, the both-depth order differs from d5ef1c6fe:\n got %s\nwant %s", seed, g, w)
		}
	}

	// Mixed set (Douyin reboot5 [140.911304, 141.835458) TID 11105 in-path
	// hop shape): the analysed thread's own hops carry no depth. No two rows
	// tie on value, so every both-depth pair has a d5ef1c6fe depth or value
	// order to keep.
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
				t.Fatalf("both-depth pair out of the d5ef1c6fe depth and value order: %s before %s", a.EvidenceID, b.EvidenceID)
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

// Pin: rows that tie on every value at one depth take the total tie key —
// semantic record first, then ranked rows by rank ahead of unranked ones, then
// the coordinate key — in the hop order, and in the classified order inside
// one role rank. The tie key never outranks a value or a depth, every input
// order gives the same sorted order, and the pairs where d5ef1c6fe broke the
// same ties another way (rank only when both rows carried one, else the
// coordinate key; same-rank roles incomparable) are witnessed.
func TestHMC377F4ValueTiesAtOneDepthTakeTheTotalTieKey(t *testing.T) {
	pathIndex := traceCausalProjectionPathIndex(hmc377F4Path)
	row := func(label, role, subject, kind string, ordinal, depth, rank int, ms float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{Role: role, Subject: subject, ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			ChainDepth: depth, Rank: rank, CumulativeImpactMS: ms, ImpactMS: ms, Confidence: 0.78,
			EvidenceID: fmt.Sprintf("trace_query:x#%s:%d", kind, ordinal), Summary: label}
	}
	rows := []TraceCausalProjectionNode{
		row("sem3", TraceCausalRoleSemanticSpan, "w2-20", "trace_semantic_span", 1, 2, 3, 5),
		row("sem0", TraceCausalRoleSemanticSpan, "w2-20", "trace_semantic_span", 9, 2, 0, 5),
		row("hop1", TraceCausalRoleCausalHop, "w2-20", "wakeup_causal_impact", 8, 2, 1, 5),
		row("ctx2", TraceCausalRoleRootCauseContext, "w2-20", "root_cause_rank", 2, 2, 2, 5),
		row("ctx0", TraceCausalRoleRootCauseContext, "w2-20", "root_cause_rank", 7, 2, 0, 5),
		row("hop0", TraceCausalRoleCausalHop, "w2-20", "wakeup_causal_impact", 1, 2, 0, 5),
		// A larger value at the same depth and a deeper hop: the tie key
		// never outranks either.
		row("ctxBig", TraceCausalRoleRootCauseContext, "w2-20", "root_cause_rank", 9, 2, 0, 5.001),
		row("deep", TraceCausalRoleSemanticSpan, "w3-30", "trace_semantic_span", 2, 3, 1, 9),
	}
	byLabel := map[string]TraceCausalProjectionNode{}
	for _, n := range rows {
		byLabel[n.Summary] = n
	}
	labels := func(nodes []TraceCausalProjectionNode) string {
		var out []string
		for _, n := range nodes {
			out = append(out, n.Summary)
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct {
		name string
		less func(a, b TraceCausalProjectionNode) bool
		want string
	}{
		{"hop", func(a, b TraceCausalProjectionNode) bool { return traceCausalProjectionHopLess(a, b, pathIndex) },
			"ctxBig,sem3,sem0,hop1,ctx2,ctx0,hop0,deep"},
		{"classified", hmc377F4Classified(pathIndex),
			"hop1,hop0,ctxBig,sem3,sem0,ctx2,ctx0,deep"},
	} {
		for i := range rows {
			for j := range rows {
				if i != j && c.less(rows[i], rows[j]) == c.less(rows[j], rows[i]) {
					t.Fatalf("%s order: %s and %s must be strictly ordered", c.name, rows[i].Summary, rows[j].Summary)
				}
			}
		}
		for seed := int64(1); seed <= 200; seed++ {
			in := append([]TraceCausalProjectionNode(nil), rows...)
			rand.New(rand.NewSource(seed)).Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
			sort.SliceStable(in, func(i, j int) bool { return c.less(in[i], in[j]) })
			if got := labels(in); got != c.want {
				t.Fatalf("%s order, input shuffle %d: got %s want %s", c.name, seed, got, c.want)
			}
		}
	}

	// Witnesses: the same ties were broken another way at d5ef1c6fe.
	baseHop := func(a, b string) bool { return hmc377F4BaseHopLess(byLabel[a], byLabel[b], pathIndex) }
	nowHop := func(a, b string) bool { return traceCausalProjectionHopLess(byLabel[a], byLabel[b], pathIndex) }
	if !baseHop("hop1", "sem3") || !nowHop("sem3", "hop1") {
		t.Fatal("d5ef1c6fe put the lower rank first even against a semantic record; the semantic record now comes first")
	}
	if !baseHop("hop0", "hop1") || !nowHop("hop1", "hop0") {
		t.Fatal("d5ef1c6fe fell through to the coordinate key when only one row carried a rank; the ranked row now comes first")
	}
	if hmc377F4BaseClassifiedLess(byLabel["sem0"], byLabel["ctx2"], pathIndex) || hmc377F4BaseClassifiedLess(byLabel["ctx2"], byLabel["sem0"], pathIndex) ||
		!traceCausalProjectionClassifiedLess(byLabel["sem0"], byLabel["ctx2"], pathIndex) {
		t.Fatal("d5ef1c6fe left two roles of one role rank incomparable; they now take the hop order")
	}
	// Outside the ties the d5ef1c6fe order is kept.
	for _, p := range [][2]string{{"ctxBig", "sem3"}, {"hop0", "deep"}, {"sem3", "deep"}} {
		if !baseHop(p[0], p[1]) || !nowHop(p[0], p[1]) {
			t.Fatalf("value / depth order of %s before %s must be the d5ef1c6fe order", p[0], p[1])
		}
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
