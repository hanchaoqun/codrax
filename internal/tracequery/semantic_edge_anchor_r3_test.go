package tracequery

// semantic_edge_anchor_r3_test.go — R3-IMPL acceptance pins (§29.88.1 user
// ruling 2026-07-14; SCAN-3 sentinel pair §29.88.8; R4 排他通则 §29.88.2).
//
// Real-fixture sentinels (donghu_tieba_frame.systrace / donghu.ftrace, both
// in-repo):
//
//	正臂  tieba 59566 window 34579.490..34579.500 — the 61839→59566 裸边 at
//	      34579.496810 (line 5639) is IN window and the VerifyClass span
//	      (34579.495841..34579.496126, 0.285ms) lies entirely before it →
//	      the span seats ON-CHAIN at 0.285ms on the typed host-edge basis.
//	负臂  tieba 59566 window 34579.466..34579.4965 — every 61839→59566 edge
//	      is OUT of window (…465879 before the window head-adjacent region,
//	      496810 past the window end) → the span must NOT seat on-chain.
//	翻道  moving ONLY the window end 34579.4965 → 34579.4969 (+0.4ms) admits
//	      the 496810 edge and flips the lane (边界敏感双向).
//	诱错  donghu 17267 window 13762.890..13762.900 — span2 (jit on 17284)
//	      straddles SOMEONE ELSE's edge (binder:496_9→17267 at 13762.895420)
//	      while its host 17284 holds no edge → no on-chain semantic seat
//	      (the credential is the host's OWN edge, never window co-presence).
//	SELF  donghu 17284 window 13762.845..13762.900 — the SELF-SEM jit family
//	      seat (rank#2, 2.388, self basis) stays byte-identical (§29.61.1
//	      carve; §29.88.8 ③).
//
// Synthetic pins: bisection (跨边按边界二分 — pre-edge ⛓ seat + post-edge ◇
// remainder clone partitioning the span exactly) and the multi-hop chain-edge
// credential (60595 depth-2 判例形; no natural in-repo witness for either —
// 如实注, same discipline as the RSPA io_burst synthetic arm).

import (
	"context"
	"math"
	"strings"
	"testing"
)

const r3TiebaTrace = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
const r3DonghuTrace = "../../eval/fixtures/real_traces/donghu.ftrace"

func r3RankQuery(pid int, start, end float64) Query {
	return Query{PID: pid, TimeStart: start, TimeEnd: end,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
}

func r3SemanticRows(rank RootCauseRankResult) (onChain, offChain []RootCauseRankItem) {
	for _, item := range rank.Items {
		if !rootCauseItemIsSemanticSpanWork(item) {
			continue
		}
		if rootCauseItemIsOnChain(item) {
			onChain = append(onChain, item)
		} else {
			offChain = append(offChain, item)
		}
	}
	return onChain, offChain
}

func TestR3TiebaSentinelPositiveArm(t *testing.T) {
	idx, err := BuildIndex(context.Background(), r3TiebaTrace)
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, r3RankQuery(59566, 34579.490, 34579.500))
	onChain, _ := r3SemanticRows(rank)
	if len(onChain) != 1 {
		t.Fatalf("正臂 must seat exactly ONE on-chain semantic row: %+v", rank.Items)
	}
	seat := onChain[0]
	if seat.Thread.PID != 61839 || seat.Type != "class_verification" {
		t.Fatalf("正臂 seat identity drifted: %+v", seat)
	}
	if seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdge {
		t.Fatalf("正臂 seat must ride the typed host-edge basis: %+v", seat)
	}
	if seat.Causality != "on_wakeup_chain" || seat.ChainRelevance != "on_chain" {
		t.Fatalf("正臂 seat must speak the honest edge token on the chain channel: %+v", seat)
	}
	// CROWNSEM-1 (§40.28 ①, restoring R3): the whole 0.285ms span lies before
	// the edge — its pre-edge share IS the priced on-chain effective.
	if math.Abs(seat.EffectiveImpactMs-0.285) > 0.002 || math.Abs(seat.ProjectedImpactMs-0.285) > 0.002 ||
		math.Abs(seat.CumulativeImpactMs-0.285) > 0.002 || seat.Rank <= 0 || seat.Tier == RootCauseTierContextOnly {
		t.Fatalf("正臂 must price its 0.285ms pre-edge share on-chain: %+v", seat)
	}
	// The anchor boundary is the raw 裸边 line 5639 timestamp, µs-exact.
	if math.Abs(seat.HostWakeupEdgeAnchorTs-34579.496810) > 0.0000005 {
		t.Fatalf("正臂 anchor boundary must be the 34579.496810 edge: %+v", seat)
	}
	if seat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect {
		t.Fatalf("正臂 credential inventory must be the direct census edge: %+v", seat)
	}
	// Fully pre-edge: no bipartition pair, no ◇ remainder clone.
	if seat.ChainAnchorFullMs != 0 || seat.ChainAnchorRemainderSeat {
		t.Fatalf("正臂 fully pre-edge seat must carry no bipartition pair: %+v", seat)
	}
	// No fabricated overlap; the R4-family credential sentence rides Summary.
	if seat.OverlapMs != 0 {
		t.Fatalf("正臂 seat must not fabricate a chain-window overlap: %+v", seat)
	}
	if !strings.Contains(seat.Summary, "pre-edge=effective") ||
		!strings.Contains(seat.Summary, "mechanism unproven") {
		t.Fatalf("正臂 seat summary must speak the R4 credential rule and keep the mechanism disclosure: %q", seat.Summary)
	}
}

func TestR3TiebaSentinelNegativeArmAndBoundaryFlip(t *testing.T) {
	idx, err := BuildIndex(context.Background(), r3TiebaTrace)
	if err != nil {
		t.Fatal(err)
	}
	// 负臂: every 61839→59566 edge sits outside 34579.466..34579.4965 —
	// the span must NOT seat on-chain (◇/▒ per the legacy lane, unchanged).
	rank := BuildRootCauseRank(idx, r3RankQuery(59566, 34579.466, 34579.4965))
	onChain, _ := r3SemanticRows(rank)
	if len(onChain) != 0 {
		t.Fatalf("负臂 must seat ZERO on-chain semantic rows: %+v", onChain)
	}
	// 翻道 (边界敏感 pin, 双向): end +0.4ms admits the 496810 edge — the
	// same span flips onto the chain tier; the 正臂 window ending 34579.500
	// above is the other direction of the same boundary sensitivity.
	flipped := BuildRootCauseRank(idx, r3RankQuery(59566, 34579.466, 34579.4969))
	onChainFlipped, _ := r3SemanticRows(flipped)
	if len(onChainFlipped) != 1 || onChainFlipped[0].Thread.PID != 61839 ||
		onChainFlipped[0].OnChainBasis != RootCauseOnChainBasisHostWakeupEdge {
		t.Fatalf("翻道 window (+0.4ms end) must seat the 61839 span on-chain: %+v", onChainFlipped)
	}
	if math.Abs(onChainFlipped[0].EffectiveImpactMs-0.285) > 0.002 || math.Abs(onChainFlipped[0].ProjectedImpactMs-0.285) > 0.002 {
		t.Fatalf("翻道 must price the 0.285ms pre-edge share: %+v", onChainFlipped[0])
	}
}

// TestR3DonghuDecoyForeignEdgeEarnsNothing — R4 排他 negative pin (§29.88.2 +
// §29.88.8 ④): span2 straddles binder:496_9→17267 (someone ELSE's edge);
// host 17284 holds no in-window edge toward 17267 → no on-chain seat.
func TestR3DonghuDecoyForeignEdgeEarnsNothing(t *testing.T) {
	idx, err := BuildIndex(context.Background(), r3DonghuTrace)
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, r3RankQuery(17267, 13762.890, 13762.900))
	onChain, _ := r3SemanticRows(rank)
	for _, seat := range onChain {
		if seat.Thread.PID == 17284 {
			t.Fatalf("诱错: a span straddling a FOREIGN edge must earn no seat: %+v", seat)
		}
	}
	// Fixture-activation fail-loud: the decoy span must still exist in the
	// window inventory (otherwise this negative pin is vacuous).
	stats := ComputeWindowStats(idx, r3RankQuery(17267, 13762.890, 13762.900))
	sawSpan := false
	for _, span := range stats.TraceSpans {
		if span.Thread.PID == 17284 && span.SemanticClass == "jit_compile" {
			sawSpan = true
		}
	}
	if !sawSpan {
		t.Fatalf("fixture drifted: the 17284 jit span left the decoy window: %+v", stats.TraceSpans)
	}
}

// TestR3SelfSemSeatByteStable — SELF carve regression (§29.61.1 / §29.88.8 ③):
// the target's own jit family seat keeps rank#2 / 2.388 / self basis.
func TestR3SelfSemSeatByteStable(t *testing.T) {
	idx, err := BuildIndex(context.Background(), r3DonghuTrace)
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, r3RankQuery(17284, 13762.845, 13762.900))
	var self *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "jit_compile" && rank.Items[i].Thread.PID == 17284 {
			self = &rank.Items[i]
		}
	}
	if self == nil {
		t.Fatalf("SELF-SEM seat missing: %+v", rank.Items)
	}
	if self.Rank != 2 || self.OnChainBasis != RootCauseOnChainBasisSelfDeterministicSpan ||
		self.Causality != RootCauseCausalitySelfDeterministic {
		t.Fatalf("SELF-SEM seat drifted: %+v", self)
	}
	if math.Abs(self.EffectiveImpactMs-2.388) > 0.002 {
		t.Fatalf("SELF-SEM seat value drifted: %+v", self)
	}
	if self.HostWakeupEdgeAnchorTs != 0 || self.HostWakeupEdgeAnchorVia != "" {
		t.Fatalf("SELF-SEM seat must never carry the host-edge pair: %+v", self)
	}
}

// --- synthetic pins (no natural in-repo witness — 如实注) --------------------

// r3SyntheticChain builds a chain universe with target app-100, a depth-1
// chain node relay-200 (its own chain edge at 6.004 — the multi-hop
// credential), and a DIRECT census edge host-300→app-100 at 6.006.
func r3SyntheticChain() ChainResult {
	target := ThreadRef{Comm: "app", PID: 100}
	relay := ThreadRef{Comm: "relay", PID: 200}
	host := ThreadRef{Comm: "hostw", PID: 300}
	return ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 6.000, EndTs: 6.010},
		Nodes: []ChainNode{
			{Thread: target, Depth: 0, Branch: 1, Window: TimeWindow{StartTs: 6.000, EndTs: 6.0045}, Dominant: StateSSleep},
			{Thread: relay, Depth: 1, Branch: 1, Window: TimeWindow{StartTs: 6.000, EndTs: 6.004}, Dominant: StateRunnable},
		},
		Edges: []WakeupEdge{
			{Waker: relay, Wakee: target, WakeupTs: 6.0045, WakeupLine: 10, Branch: 1},
		},
		WakeupEdgeCensus: []WakeupEdgeCensusPair{
			{Waker: host, Wakee: target, Count: 1, FirstTs: 6.006, LastTs: 6.006},
			{Waker: relay, Wakee: target, Count: 1, FirstTs: 6.0045, LastTs: 6.0045},
		},
	}
}

func TestR3HostEdgeAnchorCredentialInventories(t *testing.T) {
	chain := r3SyntheticChain()
	// Direct census credential.
	anchor, ok := hostSemanticSpanEdgeAnchor(&chain, ThreadRef{Comm: "hostw", PID: 300})
	if !ok || !anchor.direct || anchor.viaChainHop || anchor.boundaryTs != 6.006 {
		t.Fatalf("direct census credential drifted: %+v ok=%v", anchor, ok)
	}
	if anchor.via() != HostWakeupEdgeAnchorViaDirect {
		t.Fatalf("via word drifted: %q", anchor.via())
	}
	// Multi-hop: the relay's OWN chain edge (凭证沿链传递, 60595 depth-2 形).
	// The relay also wakes the target directly here, so both inventories fire.
	anchor, ok = hostSemanticSpanEdgeAnchor(&chain, ThreadRef{Comm: "relay", PID: 200})
	if !ok || !anchor.direct || !anchor.viaChainHop || anchor.boundaryTs != 6.0045 {
		t.Fatalf("relay credential drifted: %+v ok=%v", anchor, ok)
	}
	if anchor.via() != HostWakeupEdgeAnchorViaDirectChainHop {
		t.Fatalf("via word drifted: %q", anchor.via())
	}
	// Chain-hop-only form: strip the relay's census pair.
	chainHopOnly := r3SyntheticChain()
	chainHopOnly.WakeupEdgeCensus = chainHopOnly.WakeupEdgeCensus[:1]
	anchor, ok = hostSemanticSpanEdgeAnchor(&chainHopOnly, ThreadRef{Comm: "relay", PID: 200})
	if !ok || anchor.direct || !anchor.viaChainHop || anchor.via() != HostWakeupEdgeAnchorViaChainHop {
		t.Fatalf("chain-hop-only credential drifted: %+v ok=%v", anchor, ok)
	}
	// R4 carve: the TARGET itself never takes this lane.
	if _, ok := hostSemanticSpanEdgeAnchor(&chain, ThreadRef{Comm: "app", PID: 100}); ok {
		t.Fatalf("the target must never earn a host-edge credential")
	}
	// No credential: a thread with neither census pair nor chain edge.
	if _, ok := hostSemanticSpanEdgeAnchor(&chain, ThreadRef{Comm: "bystander", PID: 400}); ok {
		t.Fatalf("an edge-less host must earn nothing")
	}
	// Out-of-window edges grant nothing (负臂 semantics).
	clipped := r3SyntheticChain()
	clipped.Window = TimeWindow{StartTs: 6.000, EndTs: 6.0055}
	if _, ok := hostSemanticSpanEdgeAnchor(&clipped, ThreadRef{Comm: "hostw", PID: 300}); ok {
		t.Fatalf("an out-of-window edge must grant no credential")
	}
	// 修复轮件5 (冷读观察①): a degenerate/absent chain window proves nothing
	// — the credential gate fails CLOSED (the former ts>0 fallback was a
	// fail-open door on a hard gate).
	degenerate := r3SyntheticChain()
	degenerate.Window = TimeWindow{}
	if _, ok := hostSemanticSpanEdgeAnchor(&degenerate, ThreadRef{Comm: "hostw", PID: 300}); ok {
		t.Fatalf("a degenerate chain window must grant no credential (fail-closed)")
	}
}

// TestR3FamilyGrainEdgeAnchorFold — 修复轮件3 (对抗 P2-1): the family-grain
// edge-anchored form on a synthetic two-member family (one member fully
// pre-edge, one straddling the boundary): the fold's pre/post member unions,
// the family mint's bipartition pair, and the ◇ remainder clone partition the
// complete member union exactly (hull≡union for disjoint members; no natural
// in-repo witness — 如实注).
func TestR3FamilyGrainEdgeAnchorFold(t *testing.T) {
	chain := r3SyntheticChain() // boundary for hostw-300 = the 6.006 direct edge
	spanA := TraceSpanSummary{
		Thread: ThreadRef{Comm: "hostw", PID: 300},
		Name:   "VerifyClass com.example.PreEdge",
		Kind:   "sync", SemanticClass: "class_verification",
		StartTs: 6.001, EndTs: 6.002, DurationMs: 1.0,
		StartLine: 30, EndLine: 32,
	}
	spanB := TraceSpanSummary{
		Thread: ThreadRef{Comm: "hostw", PID: 300},
		Name:   "VerifyClass com.example.Straddle",
		Kind:   "sync", SemanticClass: "class_verification",
		StartTs: 6.005, EndTs: 6.008, DurationMs: 3.0,
		StartLine: 40, EndLine: 44,
	}
	fams := FoldSemanticSpanFamilies(&chain, []TraceSpanSummary{spanA, spanB})
	if len(fams) != 1 || len(fams[0].Members) != 2 {
		t.Fatalf("both members must fold into ONE edge-anchored family: %+v", fams)
	}
	fam := fams[0]
	if !fam.OnChain || fam.OnChainBasis != RootCauseOnChainBasisHostWakeupEdge {
		t.Fatalf("family lane drifted: %+v", fam)
	}
	if fam.EdgeAnchorBoundaryTs != 6.006 || fam.EdgeAnchorVia != HostWakeupEdgeAnchorViaDirect {
		t.Fatalf("family anchor drifted: %+v", fam)
	}
	// Pre-edge union: [6.001,6.002] ∪ [6.005,6.006] = 2.000ms; post-edge
	// union: [6.006,6.008] = 2.000ms; complete member union = 4.000ms.
	if math.Abs(fam.ProjectedImpactMs-2.0) > 0.0005 || math.Abs(fam.EdgeAnchorRemainderMs-2.0) > 0.0005 {
		t.Fatalf("family pre/post unions drifted: %+v", fam)
	}
	if math.Abs(fam.TotalMs-4.0) > 0.0005 {
		t.Fatalf("complete member union drifted: %+v", fam)
	}
	if math.Abs(fam.ProjectedImpactMs+fam.EdgeAnchorRemainderMs-fam.TotalMs) > 0.0005 {
		t.Fatalf("boundary split must partition the member union exactly: %+v", fam)
	}
	if fam.EdgeAnchorRemainderStartTs != 6.006 || fam.EdgeAnchorRemainderEndTs != 6.008 {
		t.Fatalf("remainder extent drifted: %+v", fam)
	}
	if fam.DominantState != "" || fam.ChainDepth != 0 {
		t.Fatalf("edge lane must not fabricate overlap state/depth: %+v", fam)
	}
	// Family mint: bipartition pair + ◇ remainder clone.
	q := Query{PID: 100, TimeStart: 6.000, TimeEnd: 6.010}
	seat, ok := rootCauseItemFromSemanticSpanFamily(q, fam, true)
	if !ok {
		t.Fatalf("edge-anchored family must mint")
	}
	if seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdge || seat.ChainRelevance != "on_chain" ||
		seat.Causality != "on_wakeup_chain" {
		t.Fatalf("family seat lane drifted: %+v", seat)
	}
	if seat.OverlapMs != 0 {
		t.Fatalf("family seat must not fabricate a chain-window overlap: %+v", seat)
	}
	if math.Abs(seat.EffectiveImpactMs-2.0) > 0.0005 || math.Abs(seat.ProjectedImpactMs-2.0) > 0.0005 || math.Abs(seat.ChainAnchoredMs-2.0) > 0.0005 ||
		math.Abs(seat.ChainAnchorFullMs-4.0) > 0.0005 {
		t.Fatalf("family seat bipartition drifted (pre-edge share priced, R3): %+v", seat)
	}
	if seat.HostWakeupEdgeAnchorTs != 6.006 || seat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect {
		t.Fatalf("family seat anchor disclosure drifted: %+v", seat)
	}
	rem, ok := semanticEdgeAnchorRemainderSeat(seat)
	if !ok {
		t.Fatalf("straddling family must clone the ◇ remainder")
	}
	if rem.ChainRelevance != "adjacent" || !rem.ChainAnchorRemainderSeat {
		t.Fatalf("family remainder lane drifted: %+v", rem)
	}
	if rem.EffectiveImpactMs != 0 || math.Abs(rem.ProjectedImpactMs-2.0) > 0.0005 {
		t.Fatalf("family remainder must preserve raw share without pricing it: %+v", rem)
	}
	// 克隆原始账恒等式: projected pre + projected post == complete union.
	if math.Abs(seat.ProjectedImpactMs+rem.ProjectedImpactMs-fam.TotalMs) > 0.0005 {
		t.Fatalf("family raw bisection must partition the union: %.3f + %.3f != %.3f",
			seat.ProjectedImpactMs, rem.ProjectedImpactMs, fam.TotalMs)
	}
	if math.Abs(rem.ChainAnchorFullMs-seat.ChainAnchorFullMs) > 1e-9 {
		t.Fatalf("both family halves must carry ONE full account: %v vs %v",
			seat.ChainAnchorFullMs, rem.ChainAnchorFullMs)
	}
}

func TestR3BisectionSplitArithmetic(t *testing.T) {
	// Straddling interval: [5, 9] against boundary 7 → pre [5,7] + post [7,9].
	preS, preE, postS, postE := semanticEdgeAnchorSplit(5, 9, 7)
	if preS != 5 || preE != 7 || postS != 7 || postE != 9 {
		t.Fatalf("straddle split drifted: %v %v %v %v", preS, preE, postS, postE)
	}
	// Fully pre-edge: no post share.
	preS, preE, postS, postE = semanticEdgeAnchorSplit(5, 6, 7)
	if preS != 5 || preE != 6 || postS != 0 || postE != 0 {
		t.Fatalf("pre-edge split drifted: %v %v %v %v", preS, preE, postS, postE)
	}
	// Fully post-edge (边后=解除): no pre share.
	preS, preE, postS, postE = semanticEdgeAnchorSplit(8, 9, 7)
	if preS != 0 || preE != 0 || postS != 8 || postE != 9 {
		t.Fatalf("post-edge split drifted: %v %v %v %v", preS, preE, postS, postE)
	}
}

// TestR3BisectedSpanMintsSeatPlusRemainder — the 跨边按边界二分 pin at the
// mint grain: one straddling span mints the ⛓ pre-edge seat AND the ◇
// remainder clone; the two partition the span's in-window projection exactly
// (同源二分 shared semantics, RSPA typed trio reused).
func TestR3BisectedSpanMintsSeatPlusRemainder(t *testing.T) {
	chain := r3SyntheticChain()
	span := TraceSpanSummary{
		Thread: ThreadRef{Comm: "hostw", PID: 300},
		Name:   "VerifyClass com.example.Straddle",
		Kind:   "sync", SemanticClass: "class_verification",
		StartTs: 6.005, EndTs: 6.008, DurationMs: 3.0,
		StartLine: 40, EndLine: 44,
	}
	q := Query{PID: 100, TimeStart: 6.000, TimeEnd: 6.010}
	seat, ok := rootCauseItemFromSemanticTraceSpan(q, chain, span, true)
	if !ok {
		t.Fatalf("straddling span must mint")
	}
	if seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdge || seat.ChainRelevance != "on_chain" {
		t.Fatalf("seat lane drifted: %+v", seat)
	}
	// Pre-edge share: 6.005..6.006 = 1.000ms of the 3.000ms span — priced (R3).
	if math.Abs(seat.EffectiveImpactMs-1.0) > 0.0005 || math.Abs(seat.ProjectedImpactMs-1.0) > 0.0005 {
		t.Fatalf("pre-edge share must be priced on-chain: %+v", seat)
	}
	if math.Abs(seat.ChainAnchoredMs-1.0) > 0.0005 || math.Abs(seat.ChainAnchorFullMs-3.0) > 0.0005 {
		t.Fatalf("bipartition pair drifted: %+v", seat)
	}
	rem, ok := semanticEdgeAnchorRemainderSeat(seat)
	if !ok {
		t.Fatalf("straddling span must clone the ◇ remainder")
	}
	if rem.ChainRelevance != "adjacent" || rem.Causality != "adjacent_to_wakeup_chain" ||
		!rem.ChainAnchorRemainderSeat || rem.OnChainBasis != "" {
		t.Fatalf("remainder lane drifted: %+v", rem)
	}
	if rem.EffectiveImpactMs != 0 || math.Abs(rem.ProjectedImpactMs-2.0) > 0.0005 {
		t.Fatalf("remainder raw share must survive without priced attribution: %+v", rem)
	}
	// Exact raw partition: pre + post == the span's in-window projection.
	if math.Abs(seat.ProjectedImpactMs+rem.ProjectedImpactMs-3.0) > 0.0005 {
		t.Fatalf("raw bisection must partition the span exactly: %.3f + %.3f != 3.000",
			seat.ProjectedImpactMs, rem.ProjectedImpactMs)
	}
	if math.Abs(rem.ChainAnchorFullMs-seat.ChainAnchorFullMs) > 1e-9 {
		t.Fatalf("both halves must carry ONE full account: %+v vs %+v", seat.ChainAnchorFullMs, rem.ChainAnchorFullMs)
	}
	// The remainder clone keeps the semantic identity (E34-E40 ◇ row form).
	if rem.SemanticClass != "class_verification" || rem.SpanName != span.Name {
		t.Fatalf("remainder must keep the span identity: %+v", rem)
	}
	// A fully pre-edge seat clones nothing.
	preOnly := span
	preOnly.StartTs, preOnly.EndTs, preOnly.DurationMs = 6.001, 6.002, 1.0
	preSeat, ok := rootCauseItemFromSemanticTraceSpan(q, chain, preOnly, true)
	if !ok {
		t.Fatalf("pre-edge span must mint")
	}
	if _, cloned := semanticEdgeAnchorRemainderSeat(preSeat); cloned {
		t.Fatalf("a fully pre-edge seat must not clone a remainder")
	}
}

// TestB829HostEdgeDoesNotInventSemanticCompletionCausality mirrors the
// production failure geometry: the target wakeup happens at 5.005000 while
// the semantic span continues until 5.005400. The host edge proves relation,
// but time order alone cannot prove that completing the span triggered or
// delayed the wakeup. Both raw halves survive and neither receives a priced
// root-cause seat.
func TestB829HostEdgeDoesNotInventSemanticCompletionCausality(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	host := ThreadRef{Comm: "worker", PID: 200}
	chain := ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 5.000000, EndTs: 5.010000},
		Nodes: []ChainNode{{Thread: target, Depth: 0, Branch: 1,
			Window: TimeWindow{StartTs: 5.000000, EndTs: 5.005000}, Dominant: StateSSleep}},
		WakeupEdgeCensus: []WakeupEdgeCensusPair{{Waker: host, Wakee: target, Count: 1,
			FirstTs: 5.005000, LastTs: 5.005000}},
	}
	span := TraceSpanSummary{
		Thread: host, Name: "VerifyClass com.example.AfterWake", Kind: "sync",
		SemanticClass: "class_verification",
		StartTs:       5.000400, EndTs: 5.005400, DurationMs: 5.000,
		StartLine: 20, EndLine: 30,
	}
	seat, ok := rootCauseItemFromSemanticTraceSpan(
		Query{PID: target.PID, TimeStart: 5.000000, TimeEnd: 5.010000}, chain, span, true)
	if !ok {
		t.Fatal("straddling semantic observation must mint")
	}
	// CROWNSEM-1 (§40.28 ①): the r506 shape under R3 — the 4.600ms PRE-edge
	// share (5.000400..5.005000) is priced on-chain; the 0.400ms post-edge
	// remainder (5.005000..5.005400) is the ◇ remainder and stays unpriced.
	// B829 read the post-edge end ("the span finishes after the wakeup") as
	// grounds to zero the whole span; the ruled bisection prices each half.
	if got, want := seat.ProjectedImpactMs, 4.600; math.Abs(got-want) > 0.0005 {
		t.Fatalf("pre-edge share got %.3f want %.3f", got, want)
	}
	if math.Abs(seat.EffectiveImpactMs-4.600) > 0.0005 || seat.Score <= 0 {
		t.Fatalf("pre-edge share must be priced with a live score: %+v", seat)
	}
	rem, ok := semanticEdgeAnchorRemainderSeat(seat)
	if !ok || math.Abs(rem.ProjectedImpactMs-0.400) > 0.0005 || rem.EffectiveImpactMs != 0 {
		t.Fatalf("post-edge remainder must stay an unpriced ◇ share: %+v ok=%v", rem, ok)
	}
	items := []RootCauseRankItem{seat, rem}
	assignRootCauseRanksAndTiers(items, true, false)
	if items[0].Rank <= 0 || items[0].Tier == RootCauseTierContextOnly {
		t.Fatalf("priced pre-edge seat must hold an ordinal: %+v", items[0])
	}
	if items[1].Rank != 0 {
		t.Fatalf("◇ remainder must not hold an ordinal: %+v", items[1])
	}
}

// TestR3EnrichKeepsMintVerdicts — the enrich pass must keep both mint-time
// lanes (the host is NOT a chain node, so the candidate context would demote
// the seat to background and erase the remainder's honest ◇ relation).
func TestR3EnrichKeepsMintVerdicts(t *testing.T) {
	chain := r3SyntheticChain()
	span := TraceSpanSummary{
		Thread: ThreadRef{Comm: "hostw", PID: 300},
		Name:   "VerifyClass com.example.Straddle",
		Kind:   "sync", SemanticClass: "class_verification",
		StartTs: 6.005, EndTs: 6.008, DurationMs: 3.0,
		StartLine: 40, EndLine: 44,
	}
	q := Query{PID: 100, TimeStart: 6.000, TimeEnd: 6.010}
	seat, _ := rootCauseItemFromSemanticTraceSpan(q, chain, span, true)
	rem, _ := semanticEdgeAnchorRemainderSeat(seat)
	items := enrichRootCauseItemsWithChainContext(chain, []RootCauseRankItem{seat, rem})
	if items[0].ChainRelevance != "on_chain" || items[0].Causality != "on_wakeup_chain" {
		t.Fatalf("enrich must keep the host-edge on-chain verdict: %+v", items[0])
	}
	if items[1].ChainRelevance != "adjacent" || items[1].Causality != "adjacent_to_wakeup_chain" {
		t.Fatalf("enrich must keep the remainder's ◇ verdict: %+v", items[1])
	}
}
