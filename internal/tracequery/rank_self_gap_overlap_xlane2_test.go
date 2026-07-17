package tracequery

// rank_self_gap_overlap_xlane2_test.go — XLANE-2 件2 engine pins (user ruling
// §29.104.17 ④ 「披露式拆分」, §29.104.2 定谳⑥ 红格 自身缺口席×自身语义 span,
// 2026-07-17): the self running supply-fold deficit seat's semantic-overlap
// disclosure — exact typed interval intersection, per-partner, values
// untouched everywhere.

import (
	"context"
	"fmt"
	"testing"
)

// xlane2SelfGapFixtureItems builds the production shapes: the self-gap seat
// with its typed running inventory plus the target's own semantic seats with
// complete member inventories (the mint-site stashes, verbatim field combos).
func xlane2SelfGapFixtureItems() []RootCauseRankItem {
	return []RootCauseRankItem{
		{
			Type: "running", Thread: ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
			Source: "thread_timeline.self_running_fold", SubjectIsAnalysisTarget: true,
			ChainRelevance: "on_chain", Causality: RootCauseCausalitySelfWallClock,
			OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval,
			ImpactMs:     70.0, CumulativeImpactMs: 70.0, EffectiveImpactMs: 20.0,
			SupplyFoldDeficitMs: 20.0, Rank: 4, Tier: "secondary",
			LineStart: 1, LineEnd: 500,
			selfGapRunningIntervals: []foldInterval{{start: 1.000, end: 1.050}, {start: 1.100, end: 1.120}},
		},
		{
			// Semantic family seat of the target (complete member inventory):
			// [1.010,1.020] ⊂ running, [1.045,1.055] half inside → X = 15ms.
			Type: "class_verification", Thread: ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
			Source: "window_stats.trace_spans.semantic", SubjectIsAnalysisTarget: true,
			SemanticClass: "class_verification", MemberCount: 2,
			ChainRelevance: "on_chain", Causality: RootCauseCausalitySelfDeterministic,
			OnChainBasis: RootCauseOnChainBasisSelfDeterministicSpan,
			ImpactMs:     20.0, CumulativeImpactMs: 20.0, EffectiveImpactMs: 20.0,
			LineStart: 10, LineEnd: 20,
			semanticMemberIntervals: []foldInterval{{start: 1.010, end: 1.020}, {start: 1.045, end: 1.055}},
		},
		{
			// Single-span semantic seat (no family): its own [StartTs, EndTs)
			// is the typed inventory → [1.102,1.107] fully inside → X = 5ms.
			Type: "class_verification", Thread: ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
			Source: "window_stats.trace_spans.semantic", SubjectIsAnalysisTarget: true,
			SemanticClass:  "class_verification",
			ChainRelevance: "on_chain", Causality: RootCauseCausalitySelfDeterministic,
			OnChainBasis: RootCauseOnChainBasisSelfDeterministicSpan,
			ImpactMs:     5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
			StartTs: 1.102, EndTs: 1.107, LineStart: 30, LineEnd: 35,
		},
		{
			// A NON-target semantic seat never partners (its spans bill the
			// host's time, not the target's running).
			Type: "jit_compilation", Thread: ThreadRef{Comm: "Jit thread pool", PID: 64988},
			Source:         "window_stats.trace_spans.semantic",
			SemanticClass:  "jit_compilation",
			ChainRelevance: "on_chain", ImpactMs: 3.0, CumulativeImpactMs: 3.0, EffectiveImpactMs: 3.0,
			StartTs: 1.011, EndTs: 1.014, LineStart: 40, LineEnd: 45,
		},
	}
}

// 件2 core: exact X per partner, overlap-DESC order, values untouched.
func TestXLANE2SelfGapOverlapDisclosureStampsExactIntersections(t *testing.T) {
	items := xlane2SelfGapFixtureItems()
	stampSelfGapSemanticOverlapDisclosure(items)
	seat := &items[0]
	if len(seat.SelfGapSemanticOverlaps) != 2 {
		t.Fatalf("件2: exactly the two target-self semantic partners must stamp: %+v", seat.SelfGapSemanticOverlaps)
	}
	got := fmt.Sprintf("%.3f@%d..%d|%.3f@%d..%d",
		seat.SelfGapSemanticOverlaps[0].OverlapMs, seat.SelfGapSemanticOverlaps[0].LineStart, seat.SelfGapSemanticOverlaps[0].LineEnd,
		seat.SelfGapSemanticOverlaps[1].OverlapMs, seat.SelfGapSemanticOverlaps[1].LineStart, seat.SelfGapSemanticOverlaps[1].LineEnd)
	if got != "15.000@10..20|5.000@30..35" {
		t.Fatalf("件2: hand-computed intersections drifted (15=10+5 family, 5 single span): %s", got)
	}
	// 主值零动: every value channel of every row byte-identical.
	if seat.EffectiveImpactMs != 20.0 || seat.SupplyFoldDeficitMs != 20.0 || seat.ImpactMs != 70.0 {
		t.Fatalf("件2: the seat's value channels must stay untouched: %+v", seat)
	}
	for i := 1; i < len(items); i++ {
		if len(items[i].SelfGapSemanticOverlaps) != 0 {
			t.Fatalf("件2: only the self-gap seat carries the disclosure: %+v", items[i])
		}
	}
}

// 件2 fail-open arms: incomplete family inventory / missing line envelope /
// zero overlap / non-self rows — each drops its clause, never guesses.
func TestXLANE2SelfGapOverlapDisclosureFailOpenArms(t *testing.T) {
	items := xlane2SelfGapFixtureItems()
	// Family partner loses its complete inventory (mint-side all-or-nothing).
	items[1].semanticMemberIntervals = nil
	// Single-span partner loses its line envelope.
	items[2].LineStart, items[2].LineEnd = 0, 0
	stampSelfGapSemanticOverlapDisclosure(items)
	if len(items[0].SelfGapSemanticOverlaps) != 0 {
		t.Fatalf("fail-open: no complete typed inventory → no clause: %+v", items[0].SelfGapSemanticOverlaps)
	}
	// Zero physical overlap → zero entries (负向).
	items = xlane2SelfGapFixtureItems()
	items[1].semanticMemberIntervals = []foldInterval{{start: 2.000, end: 2.010}}
	items[2].StartTs, items[2].EndTs = 2.100, 2.110
	stampSelfGapSemanticOverlapDisclosure(items)
	if len(items[0].SelfGapSemanticOverlaps) != 0 {
		t.Fatalf("负向: zero overlap must stamp zero entries: %+v", items[0].SelfGapSemanticOverlaps)
	}
}

// 件2 roster cap: 7 partners keep the 6 largest (every emitted clause stays
// independently true; no Σ claim rides the roster).
func TestXLANE2SelfGapOverlapDisclosureCap(t *testing.T) {
	items := xlane2SelfGapFixtureItems()[:1]
	for i := 0; i < 7; i++ {
		items = append(items, RootCauseRankItem{
			Type: "class_verification", Thread: ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
			SubjectIsAnalysisTarget: true, SemanticClass: "class_verification",
			StartTs: 1.000 + float64(i)*0.002, EndTs: 1.001 + float64(i)*0.002,
			LineStart: 100 + i*10, LineEnd: 105 + i*10,
		})
	}
	stampSelfGapSemanticOverlapDisclosure(items)
	if len(items[0].SelfGapSemanticOverlaps) != SelfGapSemanticOverlapPartnerCap {
		t.Fatalf("cap: %d partners must keep %d clauses, got %d",
			7, SelfGapSemanticOverlapPartnerCap, len(items[0].SelfGapSemanticOverlaps))
	}
}

// 件2 call-site pin: the disclosure pass rides the SHARED finalize tail
// (attachPerfContextToRootCauseRank — every rank lane ships it, the same
// placement discipline as the R8 wire-face pass beside it). Killing the call
// site must red here even while the direct-unit pins stay green.
func TestXLANE2SelfGapOverlapStampsOnFinalizeTail(t *testing.T) {
	rank := attachPerfContextToRootCauseRank(nil, Query{},
		RootCauseRankResult{Items: xlane2SelfGapFixtureItems()}, WindowStats{})
	if len(rank.Items[0].SelfGapSemanticOverlaps) != 2 {
		t.Fatalf("the finalize tail must stamp the disclosure: %+v", rank.Items[0].SelfGapSemanticOverlaps)
	}
}

// 件2 real-trace negative arm (donghu flagship): the self-gap seat mints with
// its full running inventory, the window holds NO target-self semantic seat,
// and the disclosure stays byte-absent (zero entries, zero wire note).
func TestXLANE2SelfGapOverlapDonghuFlagshipHonestAbsence(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	rank := BuildRootCauseRank(idx, elimSelfQuery(17267, 13762.791708, 13763.024898))
	seat := findSelfRunningSeat(rank.Items, 17267)
	if seat == nil {
		t.Fatalf("the flagship self-gap seat must mint")
	}
	if len(seat.selfGapRunningIntervals) == 0 {
		t.Fatalf("the seat must keep its typed running inventory for the disclosure pass")
	}
	for _, it := range rank.Items {
		if it.SubjectIsAnalysisTarget && it.SemanticClass != "" {
			t.Skipf("fixture evolved: a target-self semantic seat appeared — re-pin the positive form")
		}
	}
	if len(seat.SelfGapSemanticOverlaps) != 0 {
		t.Fatalf("honest absence: no target-self semantic seat → zero overlap entries: %+v", seat.SelfGapSemanticOverlaps)
	}
}

// 修补轮 件7 (件1 mint-side hardening, 2026-07-17): a family whose members
// carry a DUPLICATED line range mints NO line-range set at all (fail-open 保
// 原状) — the display judgment works on a SET, so duplicates would silently
// collapse (8 entries → 7 distinct) and a multiplicity-blind subset verdict
// is a false-pointer risk. Distinct ranges keep the complete ordered set.
func TestXLANE2MemberLineRangesDuplicateFailOpen(t *testing.T) {
	dup := SemanticSpanFamily{Members: []TraceSpanSummary{
		{Name: "a", StartLine: 100, EndLine: 110},
		{Name: "b", StartLine: 100, EndLine: 110},
	}}
	if got := dup.MemberLineRangeEntries(); got != nil {
		t.Fatalf("duplicated member line ranges must mint NO set (fail-open), got %v", got)
	}
	distinct := SemanticSpanFamily{Members: []TraceSpanSummary{
		{Name: "a", StartLine: 100, EndLine: 110},
		{Name: "b", StartLine: 120, EndLine: 130},
	}}
	got := distinct.MemberLineRangeEntries()
	if len(got) != 2 || got[0] != "100..110" || got[1] != "120..130" {
		t.Fatalf("distinct member line ranges must mint the complete ordered set, got %v", got)
	}
}
