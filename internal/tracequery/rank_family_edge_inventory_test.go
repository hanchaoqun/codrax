package tracequery

import (
	"math"
	"testing"
)

// rank_family_edge_inventory_test.go — CROWNSEM-1 复核收编 (batch-one
// adversarial review, 2026-09-02): a host-edge semantic FAMILY exposes exactly
// its priced pre-edge share as direction support (P3M / 件2-件3 unions consume
// rootCauseItemDirectionSupport), and its ◇ remainder clone exposes exactly the
// released post-edge share — the two inventories partition the family union
// and never double-book (the single-span lane already behaved this way; the
// family lane handed out the UNCLIPPED member spans).
func TestHostEdgeFamilySupportInventoryIsTheBisectedShare(t *testing.T) {
	chain := r3SyntheticChain() // host 300 → target 100 credential edge at 6.006
	host := ThreadRef{Comm: "hostw", PID: 300}
	spans := []TraceSpanSummary{
		rcmSpan(host, "VerifyClass com.example.A", 6.0050, 6.0080, 10, 11), // straddles the edge: 1ms pre + 2ms post
		rcmSpan(host, "VerifyClass com.example.B", 6.0010, 6.0020, 12, 13), // fully pre-edge: 1ms
	}
	var fam *SemanticSpanFamily
	for i, f := range FoldSemanticSpanFamilies(&chain, spans) {
		if f.Thread.PID == 300 {
			fam = &FoldSemanticSpanFamilies(&chain, spans)[i]
		}
	}
	if fam == nil || fam.OnChainBasis != RootCauseOnChainBasisHostWakeupEdge {
		t.Fatalf("fixture must fold a host-edge family: %+v", fam)
	}
	item, ok := rootCauseItemFromSemanticSpanFamily(Query{PID: 100, TimeStart: 6.000, TimeEnd: 6.010}, *fam, true)
	if !ok || math.Abs(item.EffectiveImpactMs-2.0) > 0.001 || math.Abs(item.ChainAnchorFullMs-4.0) > 0.001 {
		t.Fatalf("family must price the 2.000ms pre-edge share of its 4.000ms union: ok=%v eff=%.3f full=%.3f", ok, item.EffectiveImpactMs, item.ChainAnchorFullMs)
	}
	sumMs := func(intervals []foldInterval) float64 {
		merged, _ := foldIntervalUnionWithDisjoint(intervals)
		total := 0.0
		for _, iv := range merged {
			total += (iv.end - iv.start) * 1000
		}
		return total
	}
	support, basis := rootCauseItemDirectionSupport(&item)
	if basis != RootCauseDirectionBasisSemanticMembers || math.Abs(sumMs(support)-2.0) > 0.001 {
		t.Fatalf("the seat's direction support must be exactly its priced pre-edge share (2.000ms), got %.3f via %s: %+v", sumMs(support), basis, support)
	}
	for _, iv := range support {
		if iv.end > 6.006+1e-9 {
			t.Fatalf("no pre-edge support interval may cross the credential edge: %+v", support)
		}
	}
	rem, ok := semanticEdgeAnchorRemainderSeat(item)
	if !ok || math.Abs(rem.ProjectedImpactMs-2.0) > 0.001 || rem.EffectiveImpactMs != 0 {
		t.Fatalf("the ◇ clone must carry the released 2.000ms post-edge share: ok=%v %+v", ok, rem)
	}
	remSupport, _ := rootCauseItemDirectionSupport(&rem)
	if math.Abs(sumMs(remSupport)-2.0) > 0.001 {
		t.Fatalf("the ◇ clone's direction support must be exactly the post-edge share (2.000ms), got %.3f: %+v", sumMs(remSupport), remSupport)
	}
	for _, iv := range remSupport {
		if iv.start < 6.006-1e-9 {
			t.Fatalf("no post-edge support interval may precede the credential edge: %+v", remSupport)
		}
	}
	// Retired vocabulary never rides the pair (the seat pin lives in the
	// CROWNSEM-1 tripwire; the clone is pinned here).
	for _, bad := range []string{"edge=relation credential", "neither half", "raw share"} {
		if containsFold(rem.Summary, bad) {
			t.Fatalf("◇ clone summary still speaks the retired caliber %q: %s", bad, rem.Summary)
		}
	}
	if !containsFold(rem.Summary, "edge=credential, pre-edge=effective, post-edge=released") {
		t.Fatalf("◇ clone summary must speak the single credential rule: %s", rem.Summary)
	}
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
