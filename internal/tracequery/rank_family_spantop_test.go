package tracequery

// rank_family_spantop_test.go — SPANTOP-1 件1 engine pins (§29.131, user
// ruling 2026-07-18): the complete per-member wall-clock carrier
// (MemberWallMsEntries / RootCauseRankItem.MemberWallMs) minted all-or-nothing
// beside member_line_ranges, and its death on the generic same-thread re-fold.

import (
	"fmt"
	"testing"
)

func TestSpanTopMemberWallMsEntriesAllOrNothing(t *testing.T) {
	// Complete positive members → the complete ordered "%.3f" list.
	fam := SemanticSpanFamily{Members: []TraceSpanSummary{
		{Name: "a", DurationMs: 1.781},
		{Name: "b", DurationMs: 0.607},
	}}
	got := fam.MemberWallMsEntries()
	if len(got) != 2 || got[0] != "1.781" || got[1] != "0.607" {
		t.Fatalf("complete positive members must mint the ordered list, got %v", got)
	}
	// Any non-positive member kills the WHOLE list (a partial Σ could fake
	// the seat identity downstream).
	zero := SemanticSpanFamily{Members: []TraceSpanSummary{
		{Name: "a", DurationMs: 1.781},
		{Name: "b", DurationMs: 0},
	}}
	if got := zero.MemberWallMsEntries(); got != nil {
		t.Fatalf("a non-positive member must mint NO list, got %v", got)
	}
	// Empty family → nil.
	if got := (SemanticSpanFamily{}).MemberWallMsEntries(); got != nil {
		t.Fatalf("an empty family must mint NO list, got %v", got)
	}
	// Over the shared line-range cap → nil (same all-or-nothing bound).
	var big SemanticSpanFamily
	for i := 0; i < rootCauseFamilyMemberLineRangeCap+1; i++ {
		big.Members = append(big.Members, TraceSpanSummary{Name: fmt.Sprintf("m%d", i), DurationMs: 0.5})
	}
	if got := big.MemberWallMsEntries(); got != nil {
		t.Fatalf("an over-cap family must mint NO list, got %v", got)
	}
}

func TestSpanTopSemanticRosterBoundIsLineRangeCap(t *testing.T) {
	// SPANTOP-1 件2: the SEMANTIC roster bound rose to the line-range cap so
	// the detail face can hold the complete inventory for every block-eligible
	// family (树 top3+明细全量 两面互指); beyond the cap the counted-excerpt
	// honesty stays.
	var fam SemanticSpanFamily
	for i := 0; i < rootCauseFamilyMemberLineRangeCap+3; i++ {
		fam.Members = append(fam.Members, TraceSpanSummary{Name: fmt.Sprintf("span%d", i), DurationMs: 0.5})
	}
	if got := len(fam.MemberRosterEntries()); got != rootCauseFamilyMemberLineRangeCap {
		t.Fatalf("semantic roster bound must be the line-range cap (%d), got %d",
			rootCauseFamilyMemberLineRangeCap, got)
	}
	within := SemanticSpanFamily{}
	for i := 0; i < 12; i++ {
		within.Members = append(within.Members, TraceSpanSummary{Name: fmt.Sprintf("span%d", i), DurationMs: 0.5})
	}
	if got := len(within.MemberRosterEntries()); got != 12 {
		t.Fatalf("a family within the cap must publish its complete roster, got %d", got)
	}
}

func TestSpanTopFamilyMintStampsWallList(t *testing.T) {
	// The semantic family mint stamps the wall list beside the line ranges,
	// order-aligned with Members (largest first) — and on the sum_disjoint
	// caliber the Σ equals the published participation exactly (TotalMs is
	// ASSIGNED from SumMs there, so the display µs identity holds by
	// construction).
	spans := []TraceSpanSummary{
		{Name: "VerifyClass A", Thread: ThreadRef{Comm: "worker", PID: 9},
			StartTs: 10.000, EndTs: 10.003, DurationMs: 3.0, StartLine: 100, EndLine: 120},
		{Name: "VerifyClass B", Thread: ThreadRef{Comm: "worker", PID: 9},
			StartTs: 10.010, EndTs: 10.012, DurationMs: 2.0, StartLine: 130, EndLine: 150},
		{Name: "VerifyClass C", Thread: ThreadRef{Comm: "worker", PID: 9},
			StartTs: 10.020, EndTs: 10.021, DurationMs: 1.0, StartLine: 160, EndLine: 180},
	}
	fams := FoldSemanticSpanFamilies(nil, spans)
	if len(fams) != 1 {
		t.Fatalf("one (thread,class) family expected, got %d", len(fams))
	}
	fam := fams[0]
	if fam.FoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("disjoint members must fold sum_disjoint, got %q", fam.FoldCaliber)
	}
	if fam.TotalMs != fam.SumMs {
		t.Fatalf("sum_disjoint publishes TotalMs == SumMs exactly (%v != %v)", fam.TotalMs, fam.SumMs)
	}
	item, ok := rootCauseItemFromSemanticSpanFamily(Query{}, fam, false)
	if !ok {
		t.Fatalf("family mint must succeed")
	}
	if len(item.MemberWallMs) != 3 || item.MemberWallMs[0] != "3.000" ||
		item.MemberWallMs[1] != "2.000" || item.MemberWallMs[2] != "1.000" {
		t.Fatalf("the mint must stamp the ordered wall list, got %v", item.MemberWallMs)
	}
	if len(item.MemberLineRanges) != len(item.MemberWallMs) {
		t.Fatalf("wall list and line ranges must stay aligned: %v vs %v",
			item.MemberWallMs, item.MemberLineRanges)
	}
}

func TestSpanTopGenericRefoldClearsWallList(t *testing.T) {
	// The generic same-(thread,type) re-fold rebuilds a BOUNDED roster — the
	// seed's complete wall-clock claim dies with the line ranges (fail-open;
	// absence never judges).
	items := []RootCauseRankItem{
		{Rank: 1, Type: "io_latency", Thread: ThreadRef{Comm: "w", PID: 7},
			Source: "wakeup_chain.blocking", ImpactMs: 2.0, CumulativeImpactMs: 2.0,
			EffectiveImpactMs: 2.0, Confidence: 0.8, LineStart: 10, LineEnd: 20,
			StartTs: 1.0, EndTs: 1.002, Score: 2.0,
			MemberWallMs: []string{"2.000"}, MemberLineRanges: []string{"10..20"}},
		{Rank: 2, Type: "io_latency", Thread: ThreadRef{Comm: "w", PID: 7},
			Source: "wakeup_chain.blocking", ImpactMs: 1.0, CumulativeImpactMs: 1.0,
			EffectiveImpactMs: 1.0, Confidence: 0.8, LineStart: 30, LineEnd: 40,
			StartTs: 2.0, EndTs: 2.001, Score: 1.0,
			MemberWallMs: []string{"1.000"}, MemberLineRanges: []string{"30..40"}},
	}
	folded := foldSameThreadTypeRankFamilies(Query{}, false, items)
	var merged *RootCauseRankItem
	for i := range folded {
		if folded[i].MemberCount > 1 {
			merged = &folded[i]
		}
	}
	if merged == nil {
		t.Fatalf("the two same-(thread,type) rows must re-fold: %+v", folded)
	}
	if merged.MemberWallMs != nil || merged.MemberLineRanges != nil {
		t.Fatalf("the re-fold must clear both complete carriers, got wall=%v ranges=%v",
			merged.MemberWallMs, merged.MemberLineRanges)
	}
}
