package tracequery

// rank_family_fold_intersection_test.go — 审计 #62 ③ partial-overlap pins
// (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): the on-chain semantic
// family's PARTICIPATION/published effective is the exact member∩chain
// intersection union (交集<union form pinned POSITIVELY — the pre-existing
// full-overlap fixtures could not distinguish the §24.10 union caliber from
// the intersection caliber, 批验收话术可掩盖行为反转 lesson), while the
// complete selected-window member union stays lossless on the
// cumulative/actual disclosure lanes. NEGATIVE pin: an on-chain family whose
// intersection is empty NEVER leaks the union back into the causal lane
// (fail-closed, no rank contender).

import (
	"math"
	"strings"
	"testing"
)

// semLeadPartialOverlapFamilyTrace: the second texture span crosses the
// sched_wakeup (5.006) and ends at 5.0098 — union 2.100+7.200=9.300ms,
// member∩chain intersection 2.100+3.400=5.500ms.
const semLeadPartialOverlapFamilyTrace = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000400: tracing_mark_write: B|200|Texture upload(15573) 1140x1856
     worker-200 (100) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.002500: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.002600: tracing_mark_write: B|200|Texture upload(15563) 1140x1140
     worker-200 (100) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006500: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
     worker-200 (100) [002] .... 5.009800: tracing_mark_write: E|200
`

func semLeadRound3Close(a, b float64) bool {
	return math.Round(a*1000) == math.Round(b*1000)
}

func TestSemLeadFamilyPartialOverlapPublishesIntersectionKeepsUnionDisclosed(t *testing.T) {
	idx := buildTraceIndex(t, "semlead_partial_overlap_family.systrace", semLeadPartialOverlapFamilyTrace)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4,
		MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var fam *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "texture_upload" {
			fam = &rank.Items[i]
			break
		}
	}
	if fam == nil || fam.MemberCount != 2 || fam.ChainRelevance != "on_chain" {
		t.Fatalf("expected the on-chain ×2 texture family: %+v", rank.Items)
	}
	// CROWNSEM-1 (§40.28 ①, restoring R4): the exact member∩chain intersection
	// IS the family's priced effective attribution (interval-proven credential).
	if !semLeadRound3Close(fam.ProjectedImpactMs, 5.5) || !semLeadRound3Close(fam.EffectiveImpactMs, 5.5) ||
		!semLeadRound3Close(fam.ImpactMs, 5.5) {
		t.Fatalf("on-chain intersection must be priced at 5.500ms: proj=%.3f eff=%.3f impact=%.3f",
			fam.ProjectedImpactMs, fam.EffectiveImpactMs, fam.ImpactMs)
	}
	// The complete selected-window member union stays LOSSLESS on the
	// cumulative/actual disclosure lanes (§24.10 窗口投影合计 verbatim).
	if !semLeadRound3Close(fam.CumulativeImpactMs, 9.3) || !semLeadRound3Close(fam.ActualImpactMs, 9.3) {
		t.Fatalf("the complete union must stay on the disclosure lanes: cum=%.3f actual=%.3f",
			fam.CumulativeImpactMs, fam.ActualImpactMs)
	}
	// The summary discloses BOTH calibers (intersection participation + the
	// complete union it was attributed from).
	if !strings.Contains(fam.Summary, "carried 5.500ms exact overlap with typed chain intervals") ||
		!strings.Contains(fam.Summary, "interval-proven credential priced on-chain per R4") ||
		!strings.Contains(fam.Summary, "9.300ms complete selected-window span union") {
		t.Fatalf("the family summary must disclose both calibers: %q", fam.Summary)
	}
}

// Negative (fail-closed): an on-chain family with an EMPTY intersection never
// mints a rank contender — the complete union must not leak back into the
// causal lane through the fallback.
func TestSemLeadFamilyOnChainEmptyIntersectionFailsClosed(t *testing.T) {
	fam := SemanticSpanFamily{
		Thread:        ThreadRef{Comm: "worker", PID: 200},
		SemanticClass: "texture_upload",
		OnChain:       true,
		TotalMs:       9.3,
		SumMs:         9.3,
		MaxMs:         7.2,
		MinMs:         2.1,
		FoldCaliber:   RootCauseMemberFoldCaliberSumDisjoint,
		Members: []TraceSpanSummary{
			{Name: "Texture upload(15563) 1140x1140", Thread: ThreadRef{Comm: "worker", PID: 200},
				StartTs: 5.0026, EndTs: 5.0098, DurationMs: 7.2, StartLine: 5, EndLine: 8},
			{Name: "Texture upload(15573) 1140x1856", Thread: ThreadRef{Comm: "worker", PID: 200},
				StartTs: 5.0004, EndTs: 5.0025, DurationMs: 2.1, StartLine: 2, EndLine: 4},
		},
		// ProjectedImpactMs deliberately zero: the on-chain lane found no
		// member∩chain overlap magnitude.
	}
	if item, ok := rootCauseItemFromSemanticSpanFamily(Query{}, fam, true); ok {
		t.Fatalf("an on-chain family with an empty intersection must fail closed, got %+v", item)
	}
}
