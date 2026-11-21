package tracequery

// HMC377 fix round 7 (R7-3, review round 6 finding 2; pre-existing at
// d5ef1c6fe) — a partition's claim never resolves to a whole-thread hold of
// another role. rspaDIOCausePartition.admits let EVERY hold without a
// partition satisfy a partition's claim; the round-6 comment meant only the
// seats that hold for the whole thread (the thread-level chain seat and the
// thread's own unwalked seat). The pid-level fragmented_d_state_or_io_wait
// state-churn ◇ twin also holds a remainder without a partition, so a
// partition's ⛓ clipped seat said its ◇ remainder was published while only
// the churn twin (another account) was on the board — 32 of 243 build-cap ×
// enrich-cap boards in the reviewer's round-6 universes.
//
// The universe and its hand oracle are the reviewer's round-6 universes
// (zz_rv6_universe_test.go): expected holders are computed from the universe
// definition (partition p's twin / clipped seat), never from rspaHeld.

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

var hmc377R7Member = ThreadRef{Comm: "dep", PID: 200}

type hmc377R7Part struct {
	cause    string
	io       bool
	segs     []foldInterval
	anchored float64
	inst     bool // complete slice instants (dioPartitionInstants)
}

func hmc377R7Ms(f []foldInterval) float64 {
	t := 0.0
	for _, iv := range f {
		t += (iv.end - iv.start) * 1000
	}
	return float64(int(t*1000+0.5)) / 1000
}

func hmc377R7Universe(parts []hmc377R7Part, churn bool) (ChainResult, WindowStats, []RootCauseRankItem) {
	target := ThreadRef{Comm: "app", PID: 100}
	chain := ChainResult{
		Target: target, Window: TimeWindow{StartTs: 1.0, EndTs: 1.1},
		Nodes: []ChainNode{
			{Thread: target, Depth: 0, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}, Dominant: StateSSleep},
			{Thread: hmc377R7Member, Depth: 1, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}, Dominant: StateDSleep},
		},
		Edges:         []WakeupEdge{{Waker: hmc377R7Member, Wakee: target, WakeupTs: 1.030, WakeupLine: 30}},
		CausalImpacts: []WakeupCausalImpact{{Thread: hmc377R7Member, ChainDepth: 1, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}}},
	}
	chain.directDependencyWindowsBuilt = true
	chain.directDependencyWindows = map[int][]targetWaitDependencyWindow{200: {{startTs: 1.000, endTs: 1.030, wakeupTs: 1.030}}}
	stats := WindowStats{chainAnchorsByPID: chainAnchorWindowsByPID(chain), offCPUProducerDisjoint: true}
	var dAll, ioAll []foldInterval
	dFull, dAnch, ioFull, ioAnch := 0.0, 0.0, 0.0, 0.0
	var items []RootCauseRankItem
	for i, p := range parts {
		full := hmc377R7Ms(p.segs)
		it := RootCauseRankItem{Type: "d_state_or_io_wait", Thread: hmc377R7Member, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateDSleep), DStateMs: full, ImpactMs: full, CumulativeImpactMs: full, EffectiveImpactMs: full,
			Source: "window_stats", Confidence: 0.8, LineStart: 20 + 10*i, LineEnd: 81 + 10*i, StartTs: p.segs[0].start, EndTs: p.segs[len(p.segs)-1].end,
			StatsWindowStartTs: 1.0, StatsWindowEndTs: 1.1, ledgerAnchorStamped: true, ledgerAnchoredDMs: p.anchored,
			BlockedReasonCaller: p.cause}
		// As mintRootCauseDIOStateSeat stamps a cause-partition seat.
		it.dioCausePartition = rspaDIOCausePartition{set: true, cause: p.cause}
		if p.inst {
			it.dioPartitionInstants = append([]foldInterval(nil), p.segs...)
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
		items = append(items, it)
	}
	sort.Slice(dAll, func(i, j int) bool { return dAll[i].start < dAll[j].start })
	sort.Slice(ioAll, func(i, j int) bool { return ioAll[i].start < ioAll[j].start })
	if len(dAll) > 0 {
		stats.dstateCensus = map[string]ThreadDuration{"200|0": {Thread: hmc377R7Member, DurationMs: dFull, anchoredMs: dAnch, dioIntervals: dAll}}
	}
	if len(ioAll) > 0 {
		stats.iowaitCensus = map[string]ThreadDuration{"200|1": {Thread: hmc377R7Member, DurationMs: ioFull, anchoredMs: ioAnch, dioIntervals: ioAll}}
	}
	if churn {
		full := dFull + ioFull
		items = append(items, RootCauseRankItem{Type: "fragmented_d_state_or_io_wait", Thread: hmc377R7Member, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateDSleep), DStateMs: dFull, IOWaitMs: ioFull, ImpactMs: full, CumulativeImpactMs: full, EffectiveImpactMs: full,
			Source: "window_stats.state_churn", Confidence: 0.7, LineStart: 10, LineEnd: 200, StartTs: 1.0, EndTs: 1.1, StatsWindowStartTs: 1.0, StatsWindowEndTs: 1.1})
	}
	return chain, stats, items
}

// hmc377R7Run: both rank passes, a build-cap mask between them and an
// enrich-cap mask after the second, then the one resolution pass on the
// published board.
func hmc377R7Run(chain ChainResult, stats WindowStats, items []RootCauseRankItem, keep1, keep2 func(int) bool) (pass1, final []RootCauseRankItem) {
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
	rspaPatchSummariesForTwinVisibility(final)
	return pass1, final
}

// hmc377R7Oracle is the hand expectation: partition p's ◇ twin says its
// anchored share is owned iff partition p's ⛓ clipped seat is on the board;
// p's ⛓ clipped seat says its remainder is published iff p's ◇ twin is on the
// board; the pid-level churn twin says the anchored share is owned iff every
// partition with an anchored share has its clipped seat on the board.
func hmc377R7Oracle(parts []hmc377R7Part, final []RootCauseRankItem) []string {
	clipped, twin := map[string]bool{}, map[string]bool{}
	for _, it := range final {
		if it.Thread.PID != 200 || it.ChainAnchorFullMs <= 0 || it.Type == "fragmented_d_state_or_io_wait" {
			continue
		}
		if it.ChainAnchorRemainderSeat {
			twin[it.BlockedReasonCaller] = true
		} else if rootCauseItemIsOnChain(it) {
			clipped[it.BlockedReasonCaller] = true
		}
	}
	allClipped := true
	for _, p := range parts {
		if p.anchored > 0 && !clipped[p.cause] {
			allClipped = false
		}
	}
	var errs []string
	for _, it := range final {
		if it.Thread.PID != 200 || it.ChainAnchorFullMs <= 0 {
			continue
		}
		p := it.BlockedReasonCaller
		owned, ownedNot := strings.Contains(it.Summary, rspaSummaryOwnedByChainSeat), strings.Contains(it.Summary, rspaSummaryOwnedByChainSeatUnpublished)
		pub, pubNot := strings.Contains(it.Summary, rspaSummaryRemainderTwinPublished), strings.Contains(it.Summary, rspaSummaryRemainderTwinUnpublished)
		switch {
		case it.Type == "fragmented_d_state_or_io_wait":
			if owned && !allClipped {
				errs = append(errs, fmt.Sprintf("churn twin says owned while some partition's clipped seat is gone (clipped=%v)", clipped))
			}
			if ownedNot && allClipped {
				errs = append(errs, "churn twin says the owner is gone while every partition's clipped seat is on the board")
			}
		case it.ChainAnchorRemainderSeat && it.ChainAnchoredMs > 0:
			if owned && !clipped[p] {
				errs = append(errs, fmt.Sprintf("◇ %q says owned, partition %q clipped seat absent", p, p))
			}
			if ownedNot && clipped[p] {
				errs = append(errs, fmt.Sprintf("◇ %q says owner gone, partition %q clipped seat present", p, p))
			}
		case !it.ChainAnchorRemainderSeat:
			if pub && !twin[p] {
				errs = append(errs, fmt.Sprintf("⛓ %q says twin published, partition %q twin absent", p, p))
			}
			if pubNot && twin[p] {
				errs = append(errs, fmt.Sprintf("⛓ %q says twin gone, partition %q twin present", p, p))
			}
		}
	}
	return errs
}

func hmc377R7Enumerate(t *testing.T, name string, parts []hmc377R7Part, churn bool) (fails, combos int) {
	t.Helper()
	chain, stats, items := hmc377R7Universe(parts, churn)
	p1, _ := hmc377R7Run(chain, stats, items, nil, nil)
	var first string
	for m1 := 0; m1 < 1<<len(p1); m1++ {
		chain, stats, items := hmc377R7Universe(parts, churn)
		mm1 := m1
		_, full2 := hmc377R7Run(chain, stats, items, func(i int) bool { return mm1&(1<<i) != 0 }, nil)
		for m2 := 0; m2 < 1<<len(full2); m2++ {
			combos++
			chain, stats, items := hmc377R7Universe(parts, churn)
			mm2 := m2
			_, final := hmc377R7Run(chain, stats, items, func(i int) bool { return mm1&(1<<i) != 0 }, func(i int) bool { return mm2&(1<<i) != 0 })
			if errs := hmc377R7Oracle(parts, final); len(errs) > 0 {
				fails++
				if first == "" {
					var b strings.Builder
					for _, it := range final {
						fmt.Fprintf(&b, "\n      %s/%s cause=%q rel=%s d=%.3f io=%.3f anch=%.3f full=%.3f rem=%t :: %s", it.Type, it.Source, it.BlockedReasonCaller,
							it.ChainRelevance, it.DStateMs, it.IOWaitMs, it.ChainAnchoredMs, it.ChainAnchorFullMs, it.ChainAnchorRemainderSeat, it.Summary)
					}
					first = fmt.Sprintf("keep1=%b keep2=%b: %s%s", m1, m2, strings.Join(errs, "; "), b.String())
				}
			}
		}
	}
	if fails > 0 {
		t.Errorf("[%s] %d of %d build-cap × enrich-cap boards contradict the hand oracle; first: %s", name, fails, combos, first)
	}
	return fails, combos
}

func TestHMC377R7PartitionClaimsNeverResolveToTheChurnTwin(t *testing.T) {
	a := []foldInterval{{start: 1.010, end: 1.012}, {start: 1.070, end: 1.071}}
	b := []foldInterval{{start: 1.020, end: 1.024}, {start: 1.080, end: 1.081}}
	c := []foldInterval{{start: 1.025, end: 1.026}, {start: 1.090, end: 1.093}}
	bEq := []foldInterval{{start: 1.020, end: 1.022}, {start: 1.080, end: 1.082}}
	cEq := []foldInterval{{start: 1.026, end: 1.028}, {start: 1.090, end: 1.092}}
	us := []struct {
		name  string
		parts []hmc377R7Part
		churn bool
	}{
		{"3 partitions A(inst) B(windowless) ''(inst)", []hmc377R7Part{{cause: "a", segs: a, anchored: 2, inst: true}, {cause: "b", segs: b, anchored: 4}, {cause: "", segs: c, anchored: 1, inst: true}}, false},
		{"3 partitions all windowless, equal walked shares", []hmc377R7Part{{cause: "a", segs: a, anchored: 2}, {cause: "b", segs: bEq, anchored: 2}, {cause: "", segs: cEq, anchored: 2}}, false},
		{"3 partitions all windowed, equal walked shares", []hmc377R7Part{{cause: "a", segs: a, anchored: 2, inst: true}, {cause: "b", segs: bEq, anchored: 2, inst: true}, {cause: "", segs: cEq, anchored: 2, inst: true}}, false},
		{"D(a,inst) + IO(b,windowless) + D(''inst)", []hmc377R7Part{{cause: "a", segs: a, anchored: 2, inst: true}, {cause: "b", io: true, segs: b, anchored: 4}, {cause: "", segs: c, anchored: 1, inst: true}}, false},
		{"2 partitions windowless + churn twin", []hmc377R7Part{{cause: "a", segs: a, anchored: 2}, {cause: "b", segs: bEq, anchored: 2}}, true},
		{"2 partitions windowed + churn twin", []hmc377R7Part{{cause: "a", segs: a, anchored: 2, inst: true}, {cause: "b", segs: bEq, anchored: 2, inst: true}}, true},
	}
	total := 0
	for _, u := range us {
		_, combos := hmc377R7Enumerate(t, u.name, u.parts, u.churn)
		total += combos
	}
	if total != 4*729+2*243 {
		t.Fatalf("universe size moved: %d boards", total)
	}
}

// The whole-thread roles keep resolving a partition's claim: the
// thread-level chain seat and the thread's own unwalked seat hold for every
// partition; a hold of any other role without a partition (the churn twin's
// remainder) never does.
func TestHMC377R7PartitionAdmitsOnlyWholeThreadRoles(t *testing.T) {
	part := rspaDIOCausePartition{set: true, cause: "a"}
	for _, tc := range []struct {
		hold rspaHeldShare
		want bool
	}{
		{rspaHeldShare{role: RootCauseShareHolderChainSeat, dio: true}, true},
		{rspaHeldShare{role: RootCauseShareHolderUnwalkedSeat, dio: true}, true},
		{rspaHeldShare{role: RootCauseShareHolderRemainderSeat, dio: true}, false},
		{rspaHeldShare{role: RootCauseShareHolderClippedSeat, dio: true}, false},
		{rspaHeldShare{role: RootCauseShareHolderResidualSeat, dio: true}, false},
		{rspaHeldShare{role: RootCauseShareHolderRemainderSeat, dio: true, partition: part}, true},
		{rspaHeldShare{role: RootCauseShareHolderClippedSeat, dio: true, partition: rspaDIOCausePartition{set: true, cause: "b"}}, false},
	} {
		if got := part.admits(tc.hold); got != tc.want {
			t.Errorf("partition claim admits %+v = %t, want %t", tc.hold, got, tc.want)
		}
	}
	// A whole-thread claim (the churn twin's own) is held by every
	// partition's seats together.
	whole := rspaDIOCausePartition{}
	for _, h := range []rspaHeldShare{{role: RootCauseShareHolderClippedSeat, dio: true, partition: part}, {role: RootCauseShareHolderRemainderSeat, dio: true}} {
		if !whole.admits(h) {
			t.Errorf("a whole-thread claim refuses %+v", h)
		}
	}
}
