package types

import "testing"

// WO-D1③ (SMR-1 批 SMR-S9, smr_audit_report §②, 2026-07-12; witnesses 42729
// E18(+5)/56643 E20(+5): the 「其余N项(链上折叠)」 pool seated flat
// re-publications of already-rendered rows and its headline re-published a
// rendered value to the µs — an extra 37% ghost bar; 31552 E25: the headline
// ×2 aggregate re-published two rendered rows' combined time). The pool
// membership check runs at the fold: full-fingerprint matches ABSORB into the
// rendered row (E# joins the bracket), the ×N headline shape carries the
// multi-ref mirror ids for the display tag.

func smr1PoolNode(id string, impact float64, line int) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleCausalHop, EvidenceID: id,
		Subject: "cookiemonstercl-59843", Object: "sleep_wait", StateKind: "s_sleep",
		ChainRelevance: "on_chain",
		ImpactMS:       impact, CumulativeImpactMS: impact,
		Confidence: 0.8, LineStart: line, LineEnd: line + 10,
	}
}

func TestSMR1D1PoolAbsorbsFullFingerprintMatches(t *testing.T) {
	// kept[0] is the rendered trunk sleep row (42.131); the overflow contains
	// its flat re-publication plus two genuine rows.
	nodes := []TraceCausalProjectionNode{
		smr1PoolNode("e3", 42.131, 100),
		smr1PoolNode("k2", 30.000, 200),
		// overflow (limit 2):
		smr1PoolNode("flat", 42.131, 300),
		smr1PoolNode("g1", 9.000, 400),
		smr1PoolNode("g2", 8.000, 500),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	if len(got) != 3 {
		t.Fatalf("want kept 2 + fold 1, got %d rows: %+v", len(got), got)
	}
	kept := got[0]
	found := false
	for _, id := range kept.MergedEvidenceIDs {
		if id == "flat" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the flat re-publication must absorb into its rendered row (E# joins bracket): %+v", kept.MergedEvidenceIDs)
	}
	fold := got[2]
	if !fold.OnChainOverflowFold || fold.MergedCount != 2 {
		t.Fatalf("the pool honestly shrinks to the two genuine rows: %+v", fold)
	}
	if fold.ImpactMS != 9.000 {
		t.Fatalf("the ghost headline dies — max over GENUINE members only, got %v", fold.ImpactMS)
	}
}

// A near-but-not-µs-equal value stays in the pool (禁用裸成员盘存重叠判).
func TestSMR1D1PoolNearValueStaysPooled(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		smr1PoolNode("e3", 42.131, 100),
		smr1PoolNode("k2", 30.000, 200),
		smr1PoolNode("near", 42.140, 300),
		smr1PoolNode("g1", 9.000, 400),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	fold := got[len(got)-1]
	if !fold.OnChainOverflowFold || fold.MergedCount != 2 || fold.ImpactMS != 42.140 {
		t.Fatalf("non-identical values never absorb: %+v", fold)
	}
}

// 31552 E25 shape: the ×2 aggregate headline whose derivable members each
// µs-match a rendered row carries the multi-ref mirror ids.
func TestSMR1D1PoolHeadlineMultiRefMirrorIDs(t *testing.T) {
	agg := smr1PoolNode("e25", 20.816, 300)
	agg.MergedCount = 2
	agg.MergedMinMS = 5.251
	agg.MergedMaxMS = 15.565
	nodes := []TraceCausalProjectionNode{
		smr1PoolNode("e5", 15.565, 100),
		smr1PoolNode("e10", 5.251, 200),
		agg,
		smr1PoolNode("g1", 9.000, 400),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	fold := got[len(got)-1]
	if !fold.OnChainOverflowFold {
		t.Fatalf("expected the fold row, got %+v", fold)
	}
	// Member-value order (min first): 5.251→e10, 15.565→e5.
	if len(fold.OverflowMirrorEvidenceIDs) != 2 ||
		fold.OverflowMirrorEvidenceIDs[0] != "e10" || fold.OverflowMirrorEvidenceIDs[1] != "e5" {
		t.Fatalf("headline multi-ref ids must name the rendered rows, got %+v", fold.OverflowMirrorEvidenceIDs)
	}
}

// 2609 复放追修 pin: the rendered host can live in a SIBLING chain-universe
// bucket (the 42.131 trunk sleep hop in SupportingHops) — the absorption
// searches across the rendered buckets, never only the local kept slice.
func TestSMR1D1PoolAbsorbsAcrossSiblingBuckets(t *testing.T) {
	hops := []TraceCausalProjectionNode{smr1PoolNode("e2-hop", 42.131, 90)}
	nodes := []TraceCausalProjectionNode{
		smr1PoolNode("k1", 30.000, 100),
		smr1PoolNode("k2", 29.000, 200),
		smr1PoolNode("flat", 42.131, 300),
		smr1PoolNode("g1", 9.000, 400),
		smr1PoolNode("g2", 8.000, 500),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil, hops)
	fold := got[len(got)-1]
	if !fold.OnChainOverflowFold || fold.MergedCount != 2 || fold.ImpactMS != 9.000 {
		t.Fatalf("the flat copy must absorb into the SIBLING-bucket host (ghost headline dies): %+v", fold)
	}
	found := false
	for _, id := range hops[0].MergedEvidenceIDs {
		if id == "flat" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the absorbed E# must join the sibling host's bracket: %+v", hops[0].MergedEvidenceIDs)
	}
}

// 8869 复放追修 pin: the host's deliberate cross-bucket copy (same EvidenceID
// in OnChainCauses AND SupportingHops) is ONE host — never an ambiguity that
// fails the absorption open.
func TestSMR1D1PoolCrossBucketSameIDHostIsOneHost(t *testing.T) {
	host := smr1PoolNode("e2", 42.131, 90)
	nodes := []TraceCausalProjectionNode{
		host,
		smr1PoolNode("k2", 30.000, 200),
		smr1PoolNode("flat", 42.131, 300),
		smr1PoolNode("g1", 9.000, 400),
		smr1PoolNode("g2", 8.000, 500),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil,
		[]TraceCausalProjectionNode{host}) // the deliberate hops copy
	fold := got[len(got)-1]
	if !fold.OnChainOverflowFold || fold.ImpactMS != 9.000 {
		t.Fatalf("same-id cross-bucket copies are ONE host — the ghost headline must die: %+v", fold)
	}
}

// 14047 复放追修 pin: TWO different-id rendered hosts sharing the member's
// full fingerprint are one physical time on two lanes (the ValueMirror
// semantics) — the first host absorbs; the ghost headline never re-seats.
func TestSMR1D1PoolFingerprintEqualHostsAbsorbIntoFirst(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		smr1PoolNode("e3-trunk", 25.558, 100),
		smr1PoolNode("e3-agg", 25.558, 150), // the value-mirror twin lane
		smr1PoolNode("flat", 25.558, 300),
		smr1PoolNode("g1", 9.000, 400),
		smr1PoolNode("g2", 8.000, 500),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	fold := got[len(got)-1]
	if !fold.OnChainOverflowFold || fold.ImpactMS != 9.000 {
		t.Fatalf("fingerprint-equal hosts must not fail the absorption open: %+v", fold)
	}
	if len(got[0].MergedEvidenceIDs) == 0 || got[0].MergedEvidenceIDs[0] != "flat" {
		t.Fatalf("the FIRST rendered host takes the absorption: %+v", got[0].MergedEvidenceIDs)
	}
}

// 23245 复放追修 pin: the HOPS overflow runs the same absorption arm — a flat
// re-publication overflowing on the hops bucket absorbs into its rendered
// host (kept hops or the on-chain surface); the ghost headline dies there too.
func TestSMR1D1HopsPoolAbsorbsIntoRenderedHost(t *testing.T) {
	onChain := []TraceCausalProjectionNode{smr1PoolNode("e2", 42.131, 90)}
	hops := []TraceCausalProjectionNode{
		smr1PoolNode("h1", 30.000, 100),
		smr1PoolNode("h2", 29.000, 200),
		smr1PoolNode("flat", 42.131, 300),
		smr1PoolNode("g1", 9.000, 400),
		smr1PoolNode("g2", 8.000, 500),
	}
	got := traceCausalProjectionLimitHopsFold(hops, onChain, 2)
	fold := got[len(got)-1]
	if !fold.OnChainOverflowFold || fold.ImpactMS != 9.000 {
		t.Fatalf("the hops-pool flat copy must absorb (ghost headline dies): %+v", fold)
	}
	found := false
	for _, id := range onChain[0].MergedEvidenceIDs {
		if id == "flat" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the absorbed E# must join the on-chain host's bracket: %+v", onChain[0].MergedEvidenceIDs)
	}
}

// P2-2 lane (a) pin (冷读 F-4, tieba E21 形): the pool's contents are the
// rendered row's occurrence PROJECTIONS — Σ(members) µs-equals its display →
// the fold carries the projection host id (tag-only).
func TestSMR1RepairPoolProjectionSumLane(t *testing.T) {
	host := smr1PoolNode("e11", 17.819, 90)
	nodes := []TraceCausalProjectionNode{
		host,
		smr1PoolNode("k2", 30.000, 200),
		smr1PoolNode("p1", 6.936, 300),
		smr1PoolNode("p2", 6.325, 400),
		smr1PoolNode("p3", 4.558, 500),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	fold := got[len(got)-1]
	if !fold.OnChainOverflowFold {
		t.Fatalf("expected the fold row: %+v", fold)
	}
	if fold.OverflowProjectionEvidenceID != "e11" {
		t.Fatalf("Σ(6.936+6.325+4.558)=17.819 µs-eq the rendered host — the projection id must mint, got %q",
			fold.OverflowProjectionEvidenceID)
	}
}

// P2-2 lane (b) pin (冷读 F-5, donghu E26 形): the pool headline µs-equals a
// rendered row's PUBLISHED effective attribution.
func TestSMR1RepairPoolProjectionEffLane(t *testing.T) {
	host := smr1PoolNode("e13", 2.579, 90)
	host.EffectiveImpactMS = 3.183
	nodes := []TraceCausalProjectionNode{
		host,
		smr1PoolNode("k2", 30.000, 200),
		smr1PoolNode("p1", 3.183, 300),
		smr1PoolNode("p2", 1.100, 400),
		smr1PoolNode("p3", 0.900, 500),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	fold := got[len(got)-1]
	if fold.OverflowProjectionEvidenceID != "e13" {
		t.Fatalf("headline 3.183 µs-eq the rendered eff — the projection id must mint, got %q",
			fold.OverflowProjectionEvidenceID)
	}
}

// A pool with neither identity stays untagged (fail-open).
func TestSMR1RepairPoolProjectionFailsOpen(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		smr1PoolNode("k1", 30.000, 100),
		smr1PoolNode("k2", 29.000, 200),
		smr1PoolNode("p1", 9.100, 300),
		smr1PoolNode("p2", 8.200, 400),
		smr1PoolNode("p3", 7.300, 500),
	}
	got := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	fold := got[len(got)-1]
	if fold.OverflowProjectionEvidenceID != "" {
		t.Fatalf("no identity = no tag, got %q", fold.OverflowProjectionEvidenceID)
	}
}

// P3 negative pin: the V4 token-absence relaxation NEVER fires on a REAL
// (known) object — single-side absence with a resolved peer stays two rows.
func TestSMR1RepairD3RealObjectSingleSideAbsenceStaysTwoRows(t *testing.T) {
	a := smr1TokenForkRecord("E9", "", 4.884, 8712, 15131)
	b := smr1TokenForkRecord("E15", "d_state_or_io_wait", 4.884, 8714, 15120)
	a.Object = "udk-irq-3-65"
	b.Object = "udk-irq-3-65"
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{a, b})
	if len(got.OnChainCauses) != 2 {
		t.Fatalf("real-object pairs keep two rows (the relaxation is sentinel-only), got %d", len(got.OnChainCauses))
	}
}
