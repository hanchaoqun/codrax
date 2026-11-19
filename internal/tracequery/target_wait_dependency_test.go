package tracequery

// target_wait_dependency_test.go — HMC377 E2 (owner ruling 2026-11-18): the
// carrier of the target's dependency windows, the partition helper, the
// fail-closed twins of the re-based literal universes, and the structural
// bound over real fixtures.

import (
	"context"
	"math"
	"sort"
	"strings"
	"testing"
)

// e2CarrierTrace: target app-100 on CPU1, five waits in [1.000, 1.100):
//
//	S  [1.010, 1.012] ended by waker-200
//	D  [1.020, 1.025] ended by io-300 (uninterruptible, no IO flag)
//	IO [1.030, 1.036] ended by io-300 (sched_blocked_reason iowait=1)
//	S  [1.040, 1.041] ended by app-100 itself (a self row: mints nothing)
//	S  [1.050, 1.060] ended by waker-200
//
// plus a wakeup row from waker-200 at 1.015 that finds app-100 running
// (closes no wait).
const e2CarrierTrace = `        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     waker-200 (200) [002] .... 1.012000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.012100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
     waker-200 (200) [002] .... 1.015000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
        io-300 (300) [003] .... 1.025000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.025100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.030000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
        io-300 (300) [003] .... 1.036000: sched_blocked_reason: pid=100 iowait=1 caller=fscache_page_wait_o+0x110/0x250[sysmgr.elf] delay=6000000
        io-300 (300) [003] .... 1.036000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.036100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.040000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        app-100 (100) [002] .... 1.041000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.041100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.050000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     waker-200 (200) [002] .... 1.060000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.060100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.099000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func e2WindowPairs(windows []targetWaitDependencyWindow) [][3]float64 {
	out := make([][3]float64, 0, len(windows))
	for _, w := range windows {
		out = append(out, [3]float64{w.startTs, w.endTs, w.wakeupTs})
	}
	return out
}

func e2SameWindows(t *testing.T, label string, got []targetWaitDependencyWindow, want [][3]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: windows %v want %v", label, e2WindowPairs(got), want)
	}
	for i := range want {
		if math.Abs(got[i].startTs-want[i][0]) > 1e-9 || math.Abs(got[i].endTs-want[i][1]) > 1e-9 || math.Abs(got[i].wakeupTs-want[i][2]) > 1e-9 {
			t.Fatalf("%s: windows %v want %v", label, e2WindowPairs(got), want)
		}
	}
}

// TestHMC377TargetWaitDependencyCarrier — S, D and IO waits each open the
// window [wait start, wakeup] for the thread whose recorded wakeup ended
// them; a wakeup that finds the target running, and a self row, open
// nothing; a wait that began before the window is clipped to the window.
func TestHMC377TargetWaitDependencyCarrier(t *testing.T) {
	idx := buildTraceIndex(t, "e2_carrier.systrace", e2CarrierTrace)
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.100, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	chain := BuildWakeupChain(idx, q)
	if !chain.directDependencyWindowsBuilt {
		t.Fatalf("carrier must be built on a resolved chain: %+v", chain.Caveats)
	}
	// Precondition: the IO wait is typed as IO wait by the timeline (the
	// blocked-reason marker), so the carrier covers all three wait states.
	tl := ThreadTimeline(idx, Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.100, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	states := map[string]bool{}
	for _, iv := range tl.Intervals {
		states[string(iv.State)] = true
	}
	if !states[string(StateSSleep)] || !states[string(StateDSleep)] || !states[string(StateIOWait)] {
		t.Fatalf("fixture must type S, D and IO waits: %v", states)
	}
	e2SameWindows(t, "waker-200", chain.directDependencyWindows[200], [][3]float64{{1.010, 1.012, 1.012}, {1.050, 1.060, 1.060}})
	e2SameWindows(t, "io-300", chain.directDependencyWindows[300], [][3]float64{{1.020, 1.025, 1.025}, {1.030, 1.036, 1.036}})
	if w, ok := chain.directDependencyWindows[100]; ok {
		t.Fatalf("a self row must open no window: %v", e2WindowPairs(w))
	}
	if len(chain.directDependencyWindows) != 2 {
		t.Fatalf("only the two waking threads hold windows: %v", chain.directDependencyWindows)
	}
	// Window clipped by the query start: the first S wait began at 1.010.
	clipped := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 1.011, TimeEnd: 1.100, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	e2SameWindows(t, "waker-200 (window from 1.011)", clipped.directDependencyWindows[200], [][3]float64{{1.011, 1.012, 1.012}, {1.050, 1.060, 1.060}})
	// The sum bound by construction: Σ windows ≤ the target's waiting time.
	sum := 0.0
	for _, windows := range chain.directDependencyWindows {
		sum += dependencyWindowsTotalMsForTest(windows)
	}
	if math.Abs(sum-(2+10+5+6)) > 1e-6 {
		t.Fatalf("windows must add up to the waits other threads ended (23ms): %.6f", sum)
	}
}

func dependencyWindowsTotalMsForTest(windows []targetWaitDependencyWindow) float64 {
	total := 0.0
	for _, w := range windows {
		total += (w.endTs - w.startTs) * 1000
	}
	return total
}

// TestHMC377TargetWaitDependencyCarrierFailClosed — an unresolved target, a
// degenerate window and a literal / JSON-roundtripped chain hold no carrier,
// and no carrier prices nothing (there is no fallback to the old reading).
func TestHMC377TargetWaitDependencyCarrierFailClosed(t *testing.T) {
	idx := buildTraceIndex(t, "e2_carrier_fc.systrace", e2CarrierTrace)
	if chain := BuildWakeupChain(idx, Query{PID: 999, TimeStart: 1.000, TimeEnd: 1.100, TraceFlavorHint: TraceFlavorHarmonyHitrace}); len(chain.directDependencyWindows) != 0 {
		t.Fatalf("a thread with no waits in the capture holds no window: %v", chain.directDependencyWindows)
	}
	if chain := BuildWakeupChain(idx, Query{PID: 100, TraceFlavorHint: TraceFlavorHarmonyHitrace}); chain.Window.EndTs <= chain.Window.StartTs && chain.directDependencyWindowsBuilt {
		t.Fatalf("a degenerate chain window must build no carrier")
	}
	var res ChainResult
	attachTargetWaitDependencyWindows(nil, Query{}, &res)
	if res.directDependencyWindowsBuilt {
		t.Fatalf("no cache, no carrier")
	}
	literal := r3SyntheticChain()
	literal.directDependencyWindows, literal.directDependencyWindowsBuilt = nil, false
	anchor, ok := hostSemanticSpanEdgeAnchor(&literal, ThreadRef{Comm: "hostw", PID: 300})
	if !ok || len(anchor.windows) != 0 {
		t.Fatalf("an absent carrier keeps the credential and prices nothing: %+v ok=%v", anchor, ok)
	}
	if _, _, ok := hostSemanticSpanDependencySplit(&literal, ThreadRef{Comm: "hostw", PID: 300}, 6.001, 6.005); ok {
		t.Fatalf("an absent carrier must never fall back to the [window start, latest edge] reading")
	}
}

// TestHMC377HostEdgeWithoutClosedWaitPricesNothing — the fail-closed twin of
// every re-based literal universe: the census pair is declared, the carrier
// is built, but no wait of the target was closed by the host (its wakeup
// found the target not waiting). Every host-edge lane refuses and the row is
// byte-identical to a host with no wakeup at all.
func TestHMC377HostEdgeWithoutClosedWaitPricesNothing(t *testing.T) {
	withPair := r3SyntheticChain()
	withPair.directDependencyWindows = map[int][]targetWaitDependencyWindow{}
	noPair := withPair
	noPair.WakeupEdgeCensus = nil
	seat := func() RootCauseRankItem {
		item := rootCauseItem("runnable_wait", ThreadRef{Comm: "hostw", PID: 300}, 3.0, 0.8, 10, 20, "window_stats", "hostw-300 runnable")
		item.ChainRelevance = "background"
		item.Causality = "background"
		item.DominantState = string(StateRunnable)
		item.RunnableMs = 3.0
		item.runnableIntervals = []foldInterval{{start: 6.001, end: 6.004}}
		item.memberSegmentsProducerDisjoint = true
		return item
	}
	a := anchorBareCensusEdgeStateSeats(withPair, []RootCauseRankItem{seat()})
	b := anchorBareCensusEdgeStateSeats(noPair, []RootCauseRankItem{seat()})
	if len(a) != 1 || len(b) != 1 || a[0].Summary != b[0].Summary || a[0].ChainRelevance != b[0].ChainRelevance ||
		a[0].EffectiveImpactMs != b[0].EffectiveImpactMs || a[0].OnChainBasis != "" || a[0].HostWakeupEdgeAnchorTs != 0 {
		t.Fatalf("a wakeup that closed no wait must leave the row byte-identical to a host without a wakeup: %+v vs %+v", a, b)
	}
	if set := bareCensusEdgeHostRunnableMintSet(withPair, wakeupChainThreadSet(withPair),
		map[string]ThreadDuration{"hostw": {Thread: ThreadRef{Comm: "hostw", PID: 300}, runnableIntervals: []foldInterval{{start: 6.001, end: 6.004}}}}, true); len(set) != 0 {
		t.Fatalf("the mint widening must not reach a host with no closed wait: %v", set)
	}
	if _, _, ok := hostSemanticSpanDependencySplit(&withPair, ThreadRef{Comm: "hostw", PID: 300}, 6.001, 6.005); ok {
		t.Fatalf("a span of a host with no closed wait must keep its non-chain lane")
	}
	// The inversion arm stamps no refusal record either.
	inv := seat()
	inv.Type = "priority_inversion_runnable_wait"
	if out := anchorBareCensusEdgeStateSeats(withPair, []RootCauseRankItem{inv}); out[0].GatedCompositeEdgePreShareMs != 0 || out[0].GatedCompositeEdgeAnchorTs != 0 {
		t.Fatalf("no inside share, no refusal record: %+v", out[0])
	}
}

// TestHMC377PartitionPieces — several windows and several intervals: the
// partition is exact, never apportioned, and names only the windows that
// hold a piece.
func TestHMC377PartitionPieces(t *testing.T) {
	windows := []targetWaitDependencyWindow{{startTs: 1, endTs: 2, wakeupTs: 2}, {startTs: 4, endTs: 5, wakeupTs: 5.000001}, {startTs: 7, endTs: 8, wakeupTs: 8}}
	split := splitFoldIntervalsByDependencyWindows([]foldInterval{{start: 0.5, end: 4.5}, {start: 6, end: 6.5}}, windows)
	if math.Abs(split.preMs-1500) > 1e-6 || math.Abs(split.postMs-3000) > 1e-6 {
		t.Fatalf("inside 1.5s (1..2 + 4..4.5), outside 3s: %v / %v", split.preMs, split.postMs)
	}
	if split.waitCount != 2 || math.Abs(split.waitMs-2000) > 1e-6 || split.lastWakeupTs != 5.000001 {
		t.Fatalf("only the two windows holding a piece count, the last one's wakeup is disclosed: %+v", split)
	}
	if len(split.pre) != 2 || len(split.post) != 3 {
		t.Fatalf("pieces: inside %v outside %v", split.pre, split.post)
	}
}

// TestHMC377PricedHostEdgeSegmentsLieInsideClosedWaits — the structural bound
// over real fixtures: every priced segment of a host-edge seat (published or
// pooled) lies inside a wait of the target closed by that seat's own thread;
// the shares priced on the state lanes never exceed the waits those threads
// ended; and a seat carries a positive effective value exactly when it holds
// a positive inside share.
func TestHMC377PricedHostEdgeSegmentsLieInsideClosedWaits(t *testing.T) {
	cases := []struct {
		trace string
		q     Query
	}{
		{r3TiebaTrace, r3RankQuery(59566, 34579.450627, 34579.595184)},
		{r3TiebaTrace, r3RankQuery(59566, 34579.490, 34579.500)},
		{r3TiebaTrace, Query{PID: 59843, TimeStart: 34579.450627, TimeEnd: 34579.595184, TraceFlavorHint: TraceFlavorHarmonyHitrace}},
		{r3TiebaTrace, Query{PID: 60560, TimeStart: 34579.450627, TimeEnd: 34579.595184, TraceFlavorHint: TraceFlavorHarmonyHitrace}},
		{r3DonghuTrace, r3RankQuery(17267, 13762.791708, 13763.024898)},
		{r3DonghuTrace, r3RankQuery(2955, 13762.791708, 13763.024898)},
	}
	indexes := map[string]*Index{}
	priced := 0
	for _, c := range cases {
		idx := indexes[c.trace]
		if idx == nil {
			var err error
			idx, err = BuildIndex(context.Background(), c.trace)
			if err != nil {
				t.Fatal(err)
			}
			indexes[c.trace] = idx
		}
		rank := BuildRootCauseRank(idx, c.q)
		chain := BuildWakeupChain(idx, c.q)
		if !chain.directDependencyWindowsBuilt {
			t.Fatalf("%d: carrier missing", c.q.PID)
		}
		perHost := map[int]float64{}
		seen := map[string]bool{}
		for _, items := range [][]RootCauseRankItem{rank.Items, rank.preTruncationItems} {
			for _, item := range items {
				if item.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState && item.OnChainBasis != RootCauseOnChainBasisHostWakeupEdge {
					continue
				}
				windows := chain.directDependencyWindows[item.Thread.PID]
				support := e2PricedSupport(item)
				if len(support) == 0 {
					t.Fatalf("%d: host-edge seat without a priced inventory: %+v", c.q.PID, item)
				}
				inside := 0.0
				for _, iv := range support {
					if !e2InsideOneWindow(windows, iv) {
						t.Fatalf("%d: %s-%d priced segment [%.6f, %.6f] lies outside every wait its wakeup ended %v",
							c.q.PID, item.Thread.Comm, item.Thread.PID, iv.start, iv.end, e2WindowPairs(windows))
					}
					inside += (iv.end - iv.start) * 1000
				}
				if (inside > rspaAnchorIdentityTolMs) != (item.EffectiveImpactMs > 0) {
					t.Fatalf("%d: positive inside share ⇔ positive effective broken: inside %.6f eff %.6f %+v", c.q.PID, inside, item.EffectiveImpactMs, item)
				}
				key := strings.Join([]string{item.Type, threadKey(item.Thread), item.SpanName}, "|")
				if item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState && !seen[key] {
					seen[key] = true
					perHost[item.Thread.PID] += inside
				}
				priced++
			}
		}
		pids := make([]int, 0, len(perHost))
		for pid := range perHost {
			pids = append(pids, pid)
		}
		sort.Ints(pids)
		for _, pid := range pids {
			if limit := dependencyWindowsTotalMsForTest(chain.directDependencyWindows[pid]); perHost[pid] > limit*3+1e-6 {
				// Three state families (ready, D, IO) of one host can each fill
				// the same window at most once.
				t.Fatalf("%d: host %d priced %.6fms on the state lanes, more than its waits allow (%.6fms per state)", c.q.PID, pid, perHost[pid], limit)
			}
		}
	}
	t.Logf("checked %d priced host-edge seats", priced)
}

// e2PricedSupport returns the exact priced inventory of a host-edge seat.
func e2PricedSupport(item RootCauseRankItem) []foldInterval {
	switch {
	case len(item.runnableIntervals) > 0 && strings.EqualFold(item.DominantState, string(StateRunnable)):
		return item.runnableIntervals
	case len(item.dioSegmentIntervals) > 0:
		return item.dioSegmentIntervals
	case len(item.semanticMemberIntervals) > 0:
		return item.semanticMemberIntervals
	case item.EndTs > item.StartTs:
		return []foldInterval{{start: item.StartTs, end: item.EndTs}}
	}
	return nil
}

func e2InsideOneWindow(windows []targetWaitDependencyWindow, iv foldInterval) bool {
	for _, w := range windows {
		if iv.start >= w.startTs-1e-9 && iv.end <= w.endTs+1e-9 {
			return true
		}
	}
	return false
}
