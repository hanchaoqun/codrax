package tracequery

// HMC377 fix round 6 (R6-1) — D/IO cause-partition seats. A thread's D/IO
// ledger group is internally split by proven wait object (§29.50.5): every
// partition mints its own d_state_or_io_wait / io_wait seat. A seat minted
// from proof SLICES carries no dioSegmentIntervals (whole-td members only),
// so before round 6 its share claims carried no instants and resolved by
// value: summing every holder of the thread, role and family across ALL
// partitions — the ◇ twin of partition A said its anchored share was held by
// the chain-side seat while only partition B's clipped seat survived the cap
// (review round 5b, 14 of 81 build-cap × enrich-cap boards).
//
// Round 6: the partition identity is typed on the claim and on the hold, so a
// windowless claim resolves only to a holder of its own partition (or a seat
// holding for the whole thread); a partition whose proof slices carried a
// complete segment list gives its claims instants (compared as instants).
//
// The universe below is the reviewer's (r5b zz_rv5b_partition_test.go),
// with the items stamped as mintRootCauseDIOStateSeat stamps them.

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

var hmc377R6Member = ThreadRef{Comm: "dep", PID: 200}

const (
	hmc377R6CauseA = "cause_a_wait"
	hmc377R6CauseB = "cause_b_wait"
)

type hmc377R6Partition struct {
	cause    string
	io       bool // io_wait partition (else D)
	segs     []foldInterval
	anchored float64
	withInv  bool // the seat carries its own whole-td inventory
	// sliceInstants: the partition's proof slices carried a complete
	// segment list (dioPartitionInstants), dioSegmentIntervals stays empty.
	sliceInstants bool
	// unpartitioned: a pre-round-6 hand-built seat without the typed
	// partition identity (the reviewer's original universe).
	unpartitioned bool
}

func hmc377R6Ms(f []foldInterval) float64 {
	t := 0.0
	for _, iv := range f {
		t += (iv.end - iv.start) * 1000
	}
	return float64(int(t*1000+0.5)) / 1000
}

func hmc377R6PartitionUniverse(parts []hmc377R6Partition) (ChainResult, WindowStats, []RootCauseRankItem) {
	target := ThreadRef{Comm: "app", PID: 100}
	chain := ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 1.0, EndTs: 1.1},
		Nodes: []ChainNode{
			{Thread: target, Depth: 0, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}, Dominant: StateSSleep},
			{Thread: hmc377R6Member, Depth: 1, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}, Dominant: StateDSleep},
		},
		Edges:         []WakeupEdge{{Waker: hmc377R6Member, Wakee: target, WakeupTs: 1.030, WakeupLine: 30}},
		CausalImpacts: []WakeupCausalImpact{{Thread: hmc377R6Member, ChainDepth: 1, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}}},
	}
	chain.directDependencyWindowsBuilt = true
	chain.directDependencyWindows = map[int][]targetWaitDependencyWindow{200: {{startTs: 1.000, endTs: 1.030, wakeupTs: 1.030}}}
	stats := WindowStats{chainAnchorsByPID: chainAnchorWindowsByPID(chain), offCPUProducerDisjoint: true}
	var dAll, ioAll []foldInterval
	dFull, dAnch, ioFull, ioAnch := 0.0, 0.0, 0.0, 0.0
	var items []RootCauseRankItem
	for i, p := range parts {
		full := hmc377R6Ms(p.segs)
		it := RootCauseRankItem{Type: "d_state_or_io_wait", Thread: hmc377R6Member, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateDSleep), DStateMs: full, ImpactMs: full, CumulativeImpactMs: full, EffectiveImpactMs: full,
			Source: "window_stats", Confidence: 0.8, LineStart: 20 + 10*i, LineEnd: 81 + 10*i, StartTs: p.segs[0].start, EndTs: p.segs[len(p.segs)-1].end,
			StatsWindowStartTs: 1.0, StatsWindowEndTs: 1.1, ledgerAnchorStamped: true, ledgerAnchoredDMs: p.anchored,
			BlockedReasonCaller: p.cause}
		if !p.unpartitioned {
			// mintRootCauseDIOStateSeat's stamp.
			it.dioCausePartition = rspaDIOCausePartition{set: true, cause: p.cause}
		}
		if p.io {
			it.Type, it.Source, it.DominantState = "io_wait", "window_stats.io_wait_top", string(StateIOWait)
			it.DStateMs, it.IOWaitMs = 0, full
			it.ledgerAnchoredDMs, it.ledgerAnchoredIOMs = 0, p.anchored
			ioAll = append(ioAll, p.segs...)
			ioFull += full
			ioAnch += p.anchored
		} else {
			dAll = append(dAll, p.segs...)
			dFull += full
			dAnch += p.anchored
		}
		if p.withInv {
			it.dioSegmentIntervals = append([]foldInterval(nil), p.segs...)
			if p.io {
				it.dioSegmentIntervalsIO = append([]foldInterval(nil), p.segs...)
			} else {
				it.dioSegmentIntervalsD = append([]foldInterval(nil), p.segs...)
			}
		}
		if p.sliceInstants {
			it.dioPartitionInstants = append([]foldInterval(nil), p.segs...)
		}
		items = append(items, it)
	}
	sort.Slice(dAll, func(i, j int) bool { return dAll[i].start < dAll[j].start })
	sort.Slice(ioAll, func(i, j int) bool { return ioAll[i].start < ioAll[j].start })
	if len(dAll) > 0 {
		stats.dstateCensus = map[string]ThreadDuration{"200|0": {Thread: hmc377R6Member, DurationMs: dFull, anchoredMs: dAnch, dioIntervals: dAll}}
	}
	if len(ioAll) > 0 {
		stats.iowaitCensus = map[string]ThreadDuration{"200|1": {Thread: hmc377R6Member, DurationMs: ioFull, anchoredMs: ioAnch, dioIntervals: ioAll}}
	}
	return chain, stats, items
}

// hmc377R6Run mirrors query.go: build pass (+edge), build cap (keep1), enrich
// pass (+edge), enrich cap (keep2), then the one resolution pass.
func hmc377R6Run(chain ChainResult, stats WindowStats, items []RootCauseRankItem, keep1, keep2 func(int) bool) (pass1, final []RootCauseRankItem) {
	var carried rspaUnwalkedDecisions
	pass1 = reanchorOnChainStateSeatsCarried(chain, stats, items, &carried)
	pass1 = anchorBareCensusEdgeStateSeats(chain, pass1)
	var input2 []RootCauseRankItem
	for i, it := range pass1 {
		if keep1 == nil || keep1(i) {
			input2 = append(input2, it)
		}
	}
	pass2 := reanchorOnChainStateSeatsCarried(chain, stats, append([]RootCauseRankItem(nil), input2...), &carried)
	pass2 = anchorBareCensusEdgeStateSeats(chain, pass2)
	for i, it := range pass2 {
		if keep2 == nil || keep2(i) {
			final = append(final, it)
		}
	}
	resolveRankShareClaims(final, nil)
	return pass1, final
}

func hmc377R6RowLine(it RootCauseRankItem) string {
	return fmt.Sprintf("%s/%s cause=%s rel=%s d=%.3f io=%.3f anch=%.3f full=%.3f rem=%t :: %s", it.Type, it.Source, it.BlockedReasonCaller, it.ChainRelevance,
		it.DStateMs, it.IOWaitMs, it.ChainAnchoredMs, it.ChainAnchorFullMs, it.ChainAnchorRemainderSeat, it.Summary)
}

func hmc377R6Claims(items []RootCauseRankItem) string {
	var b strings.Builder
	for i, it := range items {
		for _, h := range it.rspaHeld {
			fmt.Fprintf(&b, "      [%d] %s/%s HOLDS %s dio=%t part=%+v ms=%.3f windows=%d\n", i+1, it.Type, it.BlockedReasonCaller, h.role, h.dio, h.partition, h.ms, len(h.windows))
		}
		for _, c := range it.ShareClaims {
			fmt.Fprintf(&b, "      [%d] %s/%s key=%q CLAIM %s/%s part=%+v ms=%.3f windows=%d resolved=%t exact=%t holders=%v\n", i+1, it.Type, it.BlockedReasonCaller, it.ShareHolderKey,
				c.Part, c.Role, c.partition, c.ShareMs, len(c.windows), c.Resolved, c.Exact, c.HolderKeys)
		}
	}
	return b.String()
}

// hmc377R6PartitionTruth: what the sentences say about the partition's own
// twin against what the board holds for THAT partition (the reviewer's
// oracle), plus the typed side: every resolved claim's holders are seats of
// the claim's own partition.
func hmc377R6PartitionTruth(final []RootCauseRankItem) []string {
	clipped, twin := map[string]bool{}, map[string]bool{}
	for _, it := range final {
		if it.Thread.PID != 200 || it.ChainAnchorFullMs <= 0 {
			continue
		}
		if it.ChainAnchorRemainderSeat {
			twin[it.BlockedReasonCaller] = true
		} else if rootCauseItemIsOnChain(it) {
			clipped[it.BlockedReasonCaller] = true
		}
	}
	byKey := map[string]RootCauseRankItem{}
	for _, it := range final {
		if it.ShareHolderKey != "" {
			byKey[it.ShareHolderKey] = it
		}
	}
	var errs []string
	for _, it := range final {
		if it.Thread.PID != 200 || it.ChainAnchorFullMs <= 0 {
			continue
		}
		p := it.BlockedReasonCaller
		if it.ChainAnchorRemainderSeat && it.ChainAnchoredMs > 0 && strings.Contains(it.Summary, rspaSummaryOwnedByChainSeat) && !clipped[p] {
			errs = append(errs, fmt.Sprintf("◇ %s (%s) says its %.3fms anchored share is %q, but partition %s's ⛓ clipped seat is not on the board", it.Type, p, it.ChainAnchoredMs, rspaSummaryOwnedByChainSeat, p))
		}
		if !it.ChainAnchorRemainderSeat && strings.Contains(it.Summary, rspaSummaryRemainderTwinPublished) && !twin[p] {
			errs = append(errs, fmt.Sprintf("⛓ %s (%s) says its %.3fms remainder is %q, but partition %s's ◇ twin is not on the board", it.Type, p, it.ChainAnchorFullMs-it.ChainAnchoredMs, rspaSummaryRemainderTwinPublished, p))
		}
		for _, c := range it.ShareClaims {
			if !c.Resolved {
				continue
			}
			for _, key := range c.HolderKeys {
				if holder := byKey[key]; holder.BlockedReasonCaller != p {
					errs = append(errs, fmt.Sprintf("%s (%s) claim %s/%s resolved to %s, a seat of partition %q", it.Type, p, c.Part, c.Role, key, holder.BlockedReasonCaller))
				}
			}
		}
	}
	return errs
}

func hmc377R6PartitionUniverses() map[string][]hmc377R6Partition {
	aD := []foldInterval{{start: 1.010, end: 1.012, valueMs: 2}, {start: 1.070, end: 1.071, valueMs: 1}}
	bD := []foldInterval{{start: 1.020, end: 1.024, valueMs: 4}, {start: 1.080, end: 1.081, valueMs: 1}}
	bEq := []foldInterval{{start: 1.020, end: 1.022, valueMs: 2}, {start: 1.080, end: 1.082, valueMs: 2}}
	return map[string][]hmc377R6Partition{
		"two D cause partitions (slice members, no seat inventory)":  {{cause: hmc377R6CauseA, segs: aD, anchored: 2}, {cause: hmc377R6CauseB, segs: bD, anchored: 4}},
		"D cause partition + IO cause partition (no seat inventory)": {{cause: hmc377R6CauseA, segs: aD, anchored: 2}, {cause: hmc377R6CauseB, io: true, segs: bD, anchored: 4}},
		"two D cause partitions, equal walked shares (no seat inventory)": {{cause: hmc377R6CauseA, segs: aD, anchored: 2},
			{cause: hmc377R6CauseB, segs: bEq, anchored: 2}},
		"two D cause partitions, equal walked shares (slice instants)": {{cause: hmc377R6CauseA, segs: aD, anchored: 2, sliceInstants: true},
			{cause: hmc377R6CauseB, segs: bEq, anchored: 2, sliceInstants: true}},
		"two D cause partitions (slice instants)":         {{cause: hmc377R6CauseA, segs: aD, anchored: 2, sliceInstants: true}, {cause: hmc377R6CauseB, segs: bD, anchored: 4, sliceInstants: true}},
		"control: two D partitions WITH seat inventories": {{cause: hmc377R6CauseA, segs: aD, anchored: 2, withInv: true}, {cause: hmc377R6CauseB, segs: bD, anchored: 4, withInv: true}},
	}
}

// hmc377R6Enumerate runs every (build cap × enrich cap) truncation of one
// universe and returns the number of boards whose sentences or resolved
// claims cross a partition.
func hmc377R6Enumerate(t *testing.T, name string, parts []hmc377R6Partition, report bool) (fails, combos int) {
	t.Helper()
	chain, stats, items := hmc377R6PartitionUniverse(parts)
	p1, _ := hmc377R6Run(chain, stats, items, nil, nil)
	n1 := len(p1)
	for m1 := 0; m1 < 1<<n1; m1++ {
		chain, stats, items := hmc377R6PartitionUniverse(parts)
		mm1 := m1
		_, full2 := hmc377R6Run(chain, stats, items, func(i int) bool { return mm1&(1<<i) != 0 }, nil)
		for m2 := 0; m2 < 1<<len(full2); m2++ {
			combos++
			chain, stats, items := hmc377R6PartitionUniverse(parts)
			mm2 := m2
			_, final := hmc377R6Run(chain, stats, items, func(i int) bool { return mm1&(1<<i) != 0 }, func(i int) bool { return mm2&(1<<i) != 0 })
			if errs := hmc377R6PartitionTruth(final); len(errs) > 0 {
				fails++
				if report && fails <= 3 {
					var b strings.Builder
					for _, it := range final {
						fmt.Fprintf(&b, "\n      %s", hmc377R6RowLine(it))
					}
					t.Errorf("[%s] keep1=%b keep2=%b:\n   %s\n   board:%s\n%s", name, m1, m2, strings.Join(errs, "\n   "), b.String(), hmc377R6Claims(final))
				}
			}
		}
	}
	return fails, combos
}

// TestHMC377R6CausePartitionClaimsStayInsideTheirPartition — review round 5b
// R6-1: on every build-cap × enrich-cap board, a partition row's sentence
// names its own partition's twin or clipped seat as published only when that
// seat is on the board, and every resolved claim names seats of its own
// partition only.
func TestHMC377R6CausePartitionClaimsStayInsideTheirPartition(t *testing.T) {
	us := hmc377R6PartitionUniverses()
	names := make([]string, 0, len(us))
	for k := range us {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		fails, combos := hmc377R6Enumerate(t, name, us[name], true)
		if combos != 81 {
			t.Fatalf("[%s] the universe must enumerate the reviewer's 81 boards, got %d", name, combos)
		}
		t.Logf("== %s: %d of %d boards cross a partition", name, fails, combos)
	}
}

// TestHMC377R6CausePartitionWitnessNeedsTheTypedIdentity — the reviewer's
// universe WITHOUT the typed partition identity (hand-built pre-round-6
// seats) still resolves across partitions by value: the identity, not a
// value coincidence, keeps the claims inside their partition.
func TestHMC377R6CausePartitionWitnessNeedsTheTypedIdentity(t *testing.T) {
	aD := []foldInterval{{start: 1.010, end: 1.012, valueMs: 2}, {start: 1.070, end: 1.071, valueMs: 1}}
	bD := []foldInterval{{start: 1.020, end: 1.024, valueMs: 4}, {start: 1.080, end: 1.081, valueMs: 1}}
	parts := []hmc377R6Partition{{cause: hmc377R6CauseA, segs: aD, anchored: 2, unpartitioned: true}, {cause: hmc377R6CauseB, segs: bD, anchored: 4, unpartitioned: true}}
	fails, combos := hmc377R6Enumerate(t, "untyped", parts, false)
	if fails == 0 {
		t.Fatalf("the untyped universe must reproduce the cross-partition resolution (witness lost; %d boards)", combos)
	}
}

// TestHMC377R6CausePartitionSliceInstantsAreCompared — with complete slice
// segment lists the partition claims carry instants, so the resolution
// compares instants: every claim and hold of a partition seat carries the
// partition's instants (never another partition's).
func TestHMC377R6CausePartitionSliceInstantsAreCompared(t *testing.T) {
	parts := hmc377R6PartitionUniverses()["two D cause partitions (slice instants)"]
	chain, stats, items := hmc377R6PartitionUniverse(parts)
	_, final := hmc377R6Run(chain, stats, items, nil, nil)
	own := map[string][]foldInterval{hmc377R6CauseA: parts[0].segs, hmc377R6CauseB: parts[1].segs}
	seen := 0
	for _, it := range final {
		if it.Thread.PID != 200 || it.ChainAnchorFullMs <= 0 {
			continue
		}
		inside := rspaFoldCopy(own[it.BlockedReasonCaller])
		for _, h := range it.rspaHeld {
			if !h.dio {
				continue
			}
			seen++
			if len(h.windows) == 0 || math.Abs(foldIntervalIntersectionMs(h.windows, inside)-rspaFoldLengthMs(h.windows)) > 1e-6 {
				t.Fatalf("%s holds %v outside its partition's instants %v", hmc377R6RowLine(it), h.windows, inside)
			}
			if h.partition != (rspaDIOCausePartition{set: true, cause: it.BlockedReasonCaller}) {
				t.Fatalf("%s holds in partition %+v", hmc377R6RowLine(it), h.partition)
			}
		}
		for _, c := range it.ShareClaims {
			if !c.dio || c.Role == RootCauseShareHolderChainSeat {
				continue
			}
			seen++
			if len(c.windows) == 0 || math.Abs(foldIntervalIntersectionMs(c.windows, inside)-rspaFoldLengthMs(c.windows)) > 1e-6 {
				t.Fatalf("%s claims %v outside its partition's instants %v", hmc377R6RowLine(it), c.windows, inside)
			}
			if !c.Resolved || !c.Exact || len(c.HolderKeys) != 1 {
				t.Fatalf("%s: the intact board resolves the claim exactly to its own twin: %+v\n%s", hmc377R6RowLine(it), c, hmc377R6Claims(final))
			}
		}
	}
	if seen < 8 {
		t.Fatalf("both partitions must split into a clipped seat and a twin, each with its claim and hold (saw %d)\n%s", seen, hmc377R6Claims(final))
	}
}

// TestHMC377R6CausePartitionMintCarriesIdentityAndSliceInstants — the real
// ledger: one CPU bucket holds a proven dma_fence slice (D 20ms + iowait
// 30ms) and an unproven 10ms slice. Both partition seats carry their typed
// identity; each carries its OWN slice instants (reproducing its account),
// never the bucket's; the whole-td credential inventory stays absent.
func TestHMC377R6CausePartitionMintCarriesIdentityAndSliceInstants(t *testing.T) {
	body := strings.NewReplacer("[003]", "[002]", "[004]", "[002]", "target_cpu=003", "target_cpu=002", "target_cpu=004", "target_cpu=002").Replace(dioPartitionPartialTrace)
	idx := buildTraceIndex(t, "hmc377_r6_partition.systrace", body)
	q := Query{View: "root_cause_rank", PID: 100, TimeStart: 3, TimeEnd: 3.12, TimeStartSet: true, TimeEndSet: true,
		MaxDepth: 4, MinDurationMs: .05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 32}
	stats := ComputeWindowStats(idx, ensureQueryFlavor(idx, q))
	split := false
	for _, td := range stats.dstateCensus {
		if td.Thread.PID != 200 || len(td.causeSlices) < 2 {
			continue
		}
		split = true
		for cause, slice := range td.causeSlices {
			if slice.intervalsOverflow || !rspaWithinTol(rspaFoldLengthMs(slice.intervals), slice.durMs) {
				t.Fatalf("slice %q: its own segments must reproduce its account %.3fms: %v", cause, slice.durMs, slice.intervals)
			}
		}
	}
	if !split {
		t.Fatal("fixture must exercise a native bucket split into cause slices")
	}
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.12, TimeStartSet: true, TimeEndSet: true,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 32})
	pool := append(append([]RootCauseRankItem(nil), rank.preTruncationItems...), rank.Items...)
	seats := map[string]RootCauseRankItem{}
	for _, it := range pool {
		if it.Thread.PID == 200 && strings.HasPrefix(it.Source, "window_stats") && (it.Type == "d_state_or_io_wait" || it.Type == "io_wait") && it.ChainAnchorFullMs == 0 {
			seats[it.BlockedReasonCaller] = it
		}
	}
	cause, remainder := seats["dma_fence_default_wait"], seats[""]
	if cause.Thread.PID == 0 || remainder.Thread.PID == 0 {
		t.Fatalf("expected the proven cause seat and the unproven remainder seat: %+v", seats)
	}
	for name, seat := range map[string]RootCauseRankItem{"cause": cause, "remainder": remainder} {
		if seat.dioCausePartition != (rspaDIOCausePartition{set: true, cause: seat.BlockedReasonCaller}) {
			t.Fatalf("%s seat: typed partition identity %+v", name, seat.dioCausePartition)
		}
		if len(seat.dioSegmentIntervals) != 0 {
			t.Fatalf("%s seat: a slice-minted seat must not gain the whole-td credential inventory: %v", name, seat.dioSegmentIntervals)
		}
		if got, want := rspaFoldLengthMs(seat.dioPartitionInstants), seat.DStateMs+seat.IOWaitMs; !rspaWithinTol(got, want) {
			t.Fatalf("%s seat: partition instants %.3fms must reproduce its own account %.3fms (%v)", name, got, want, seat.dioPartitionInstants)
		}
	}
	if overlap := foldIntervalIntersectionMs(rspaFoldCopy(cause.dioPartitionInstants), rspaFoldCopy(remainder.dioPartitionInstants)); overlap > 1e-9 {
		t.Fatalf("the two partitions' instants overlap by %.6fms", overlap)
	}
}
