package tracequery

// HMC377 round 10 (R9P-1) — the analysed thread's own D/IO wait RootEvidence
// row and the formal D/IO family seats of the same thread carry one account
// per cause.
//
// Real shape (105 元宝 应用后台销毁后冷启动 #2, TID 58533 bindApplication
// [10834.099821, 10834.620005)): eight other threads hold larger D groups
// than any (thread, cpu) D group of the analysed thread, so the thread is
// absent from the top-8 DStateTop display list; the full pre-cap census holds
// its 4.256 ms D account, split by proven wait object into nine partition
// seats (vfs_path_open_ng 3.435 ms, …). The thread's own chain walked its
// 1.666 ms vfs_path_open_ng D wait and published it as a RootEvidence row.
// The ownership predicate read the display list, answered "not owned", and
// the RootEvidence row minted a second D seat (rank 10, source wakeup_chain)
// for instants the 3.435 ms partition seat already holds; the projection's
// same-kind merge then showed 5.922 ms against a raw D+IO of 4.631 ms.
//
// The fixtures below are synthetic traces built to that shape and run through
// the whole engine (index → census → chain → rank): the analysed thread's
// groups sit below the eight-group display cap, its account is split by two
// proven callers, and its own chain walks its waits — once in the D ledger
// (the witness) and once in the iowait ledger (the family answers for D and
// iowait together).

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

const (
	r9p1Target     = 100
	r9p1CauseOpen  = "vfs_path_open_ng"
	r9p1CauseCache = "fscache_page_wait_o"
)

type r9p1Event struct {
	ts   float64
	line string
}

type r9p1Trace struct{ events []r9p1Event }

func (c *r9p1Trace) add(ts float64, format string, args ...any) {
	c.events = append(c.events, r9p1Event{ts, fmt.Sprintf(format, args...) + "\n"})
}

func (c *r9p1Trace) sw(cpu int, ts float64, prevComm string, prevPID int, prevState, nextComm string, nextPID int) {
	host, hostPID := prevComm, prevPID
	c.add(ts, " %s-%d (%d) [%03d] .... %.6f: sched_switch: prev_comm=%s prev_pid=%d prev_prio=120 prev_state=%s ==> next_comm=%s next_pid=%d next_prio=120",
		host, hostPID, hostPID, cpu, ts, prevComm, prevPID, prevState, nextComm, nextPID)
}

func (c *r9p1Trace) blocked(ts float64, pid, iowait int, caller string) {
	c.add(ts, " kworker-400 (400) [000] .... %.6f: sched_blocked_reason: pid=%d iowait=%d caller=%s", ts, pid, iowait, caller)
}

func (c *r9p1Trace) wake(ts float64, comm string, pid, targetCPU int) {
	c.add(ts, " kworker-400 (400) [000] .... %.6f: sched_wakeup: comm=%s pid=%d prio=120 target_cpu=%03d", ts, comm, pid, targetCPU)
}

func (c *r9p1Trace) text() string {
	sort.SliceStable(c.events, func(i, j int) bool { return c.events[i].ts < c.events[j].ts })
	var b strings.Builder
	for _, e := range c.events {
		b.WriteString(e.line)
	}
	return b.String()
}

type r9p1Wait struct {
	start, end float64
	caller     string
}

// The analysed thread app-100 on CPU 1 waits three times in D inside
// [10.000, 10.300): twice on vfs_path_open_ng (2 ms, 3 ms) and once on
// fscache_page_wait_o (1 ms) — one census group of 6 ms on CPU 1, split by
// two proven causes. Eight other threads each hold one ≥10 ms group of the
// same ledger, so the analysed thread's group is ninth: below the display
// cap. iowait selects the ledger (0: D, 1: iowait) through the blocked
// reason, as the census does.
var r9p1Waits = []r9p1Wait{
	{10.010, 10.012, r9p1CauseOpen + "+0x1f14/0xc15c[sysmgr.elf]"},
	{10.050, 10.053, r9p1CauseOpen + "+0x1f14/0xc15c[sysmgr.elf]"},
	{10.100, 10.101, r9p1CauseCache + "+0x110/0x250[sysmgr.elf]"},
}

func r9p1DIOTrace(iowait int) string {
	var c r9p1Trace
	c.sw(1, 10.000, "idle/1", 0, "R", "app", r9p1Target)
	for _, w := range r9p1Waits {
		c.sw(1, w.start, "app", r9p1Target, "D", "idle/1", 0)
		c.blocked(w.end, r9p1Target, iowait, w.caller)
		c.wake(w.end, "app", r9p1Target, 1)
		c.sw(1, w.end+0.00001, "idle/1", 0, "R", "app", r9p1Target)
	}
	for i := 0; i < 8; i++ {
		pid, cpu, name := 301+i, 2+i, fmt.Sprintf("filler%d", i)
		start := 10.150
		end := start + 0.010 + 0.0001*float64(i)
		c.sw(cpu, start-0.001, "idle", 0, "R", name, pid)
		c.sw(cpu, start, name, pid, "D", "idle", 0)
		c.blocked(end, pid, iowait, "filler_wait+0x10/0x20[sysmgr.elf]")
		c.wake(end, name, pid, cpu)
		c.sw(cpu, end+0.00001, "idle", 0, "R", name, pid)
	}
	c.sw(1, 10.299, "app", r9p1Target, "S", "idle/1", 0)
	return c.text()
}

func r9p1Query() Query {
	return Query{PID: r9p1Target, TimeStart: 10.000, TimeEnd: 10.300, TraceFlavorHint: TraceFlavorHarmonyHitrace}
}

func r9p1Chain(idx *Index) ChainResult {
	nq := ensureQueryFlavor(idx, normalizeQuery(idx, r9p1Query()))
	return buildWakeupChainWithCache(idx, nq, newChainQueryCache(idx, nq.runCancel))
}

func r9p1IsDIO(item RootCauseRankItem) bool {
	return item.Type == "d_state_or_io_wait" || item.Type == "io_wait"
}

func r9p1Board(rank RootCauseRankResult) string {
	var b strings.Builder
	for _, item := range append(append([]RootCauseRankItem(nil), rank.Items...), rank.AbsorbedItems...) {
		if item.Thread.PID == r9p1Target {
			fmt.Fprintf(&b, "\n  rank=%d type=%s source=%s d=%.3f io=%.3f runnable=%.3f effective=%.3f cause=%q absorbed=%t",
				item.Rank, item.Type, item.Source, item.DStateMs, item.IOWaitMs, item.RunnableMs, item.EffectiveImpactMs,
				item.dioCausePartition.cause, item.AbsorbedByRankFamily)
		}
	}
	return b.String()
}

func r9p1CensusTotal(census map[string]ThreadDuration) float64 {
	total := 0.0
	for _, td := range census {
		if td.Thread.PID == r9p1Target {
			total += td.DurationMs
		}
	}
	return total
}

func r9p1Covers(instants []foldInterval, start, end float64) bool {
	for _, iv := range instants {
		if iv.start <= start+1e-9 && iv.end >= end-1e-9 {
			return true
		}
	}
	return false
}

func TestHMC377R9P1TargetOwnDIOWaitHasOneAccountPerCause(t *testing.T) {
	for _, tc := range []struct {
		name   string
		iowait int
	}{{"d_state", 0}, {"io_wait", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			r9p1AssertOneAccountPerCause(t, tc.iowait)
		})
	}
}

func r9p1AssertOneAccountPerCause(t *testing.T, iowait int) {
	idx := buildTraceIndex(t, "r9p1_target_dio.systrace", r9p1DIOTrace(iowait))
	q := r9p1Query()
	stats := ComputeWindowStats(idx, ensureQueryFlavor(idx, normalizeQuery(idx, q)))
	display, ledger, other := stats.DStateTop, stats.dstateCensus, stats.iowaitCensus
	value := func(item RootCauseRankItem) float64 { return item.DStateMs }
	if iowait == 1 {
		display, ledger, other = stats.IOWaitTop, stats.iowaitCensus, stats.dstateCensus
		value = func(item RootCauseRankItem) float64 { return item.IOWaitMs }
	}
	// The real shape: the analysed thread's account is held by the full
	// census and absent from the capped display list.
	for _, td := range display {
		if td.Thread.PID == r9p1Target {
			t.Fatalf("fixture premise: the analysed thread must sit below the display cap, the display list holds it: %+v", td)
		}
	}
	census := r9p1CensusTotal(ledger)
	if math.Abs(census-6.0) > 1e-6 || r9p1CensusTotal(other) != 0 {
		t.Fatalf("fixture premise: one census ledger holds the analysed thread's whole 6 ms account, got %.6f (other ledger %.6f)", census, r9p1CensusTotal(other))
	}
	chain := r9p1Chain(idx)
	var walked []RootEvidence
	for _, root := range chain.RootEvidence {
		if root.Thread.PID == r9p1Target && (root.Type == "d_state_or_io_wait" || root.Type == "io_wait") {
			walked = append(walked, root)
		}
	}
	if len(walked) == 0 {
		t.Fatalf("fixture premise: the analysed thread's chain walks its own waits: %+v", chain.RootEvidence)
	}

	rank := BuildRootCauseRank(idx, q)
	board := r9p1Board(rank)
	dioSum := 0.0
	partitions := map[string][]RootCauseRankItem{}
	for _, item := range rank.Items {
		if item.Thread.PID != r9p1Target || !r9p1IsDIO(item) {
			continue
		}
		if strings.HasPrefix(item.Source, "wakeup_chain") {
			t.Fatalf("the analysed thread's own walked wait minted a second D/IO seat beside the partition seats that hold its instants:%s", board)
		}
		dioSum += item.DStateMs + item.IOWaitMs
		partitions[item.dioCausePartition.cause] = append(partitions[item.dioCausePartition.cause], item)
	}
	for _, item := range rank.AbsorbedItems {
		if item.Thread.PID == r9p1Target && r9p1IsDIO(item) && strings.HasPrefix(item.Source, "wakeup_chain") {
			t.Fatalf("the walked wait is not a rank row at all (it stays on the wakeup_chain view):%s", board)
		}
	}
	// Σ of the thread's published D/IO accounts never exceeds its raw D+IO.
	raw := r9p1CensusTotal(stats.dstateCensus) + r9p1CensusTotal(stats.iowaitCensus)
	if dioSum > raw+0.001 {
		t.Fatalf("the analysed thread's D/IO seats sum to %.3f ms, its raw D+IO is %.3f ms:%s", dioSum, raw, board)
	}
	// The partition seats keep the whole typed account: one seat per proven
	// cause, together the census total.
	open, cache := partitions[r9p1CauseOpen], partitions[r9p1CauseCache]
	if len(open) != 1 || len(cache) != 1 || len(partitions) != 2 {
		t.Fatalf("one D/IO seat per proven cause expected (%s, %s):%s", r9p1CauseOpen, r9p1CauseCache, board)
	}
	if math.Abs(value(open[0])-5.0) > 1e-6 || math.Abs(value(cache[0])-1.0) > 1e-6 || math.Abs(dioSum-census) > 1e-6 {
		t.Fatalf("the partition seats must keep the whole account (5 + 1 = %.3f):%s", census, board)
	}
	// Every walked wait's instants are held by exactly one published seat: the
	// partition of its own proven cause.
	checked := 0
	for _, impact := range chain.CausalImpacts {
		if impact.Thread.PID != r9p1Target || impact.ChainDepth != 0 || !dominantStateIsDStateOrIOWait(impact.DominantState) {
			continue
		}
		holders := 0
		for _, items := range partitions {
			instants := append(append([]foldInterval(nil), items[0].dioSegmentIntervals...), items[0].dioPartitionInstants...)
			if r9p1Covers(instants, impact.Window.StartTs, impact.Window.EndTs) {
				holders++
			}
		}
		if holders != 1 {
			t.Fatalf("the walked wait [%.6f, %.6f) must be held by exactly one partition seat, found %d:%s", impact.Window.StartTs, impact.Window.EndTs, holders, board)
		}
		checked++
	}
	if checked != len(walked) {
		t.Fatalf("every walked wait must be checked against the partition instants: %d of %d", checked, len(walked))
	}
}

// Must-not-lose: when no formal family seat holds the analysed thread's
// state, its own walked wait is the account and keeps its seat. (The ready
// wait row is listed as the control of the arm this lane leaves unchanged:
// the RULER2-1 two-ruler publication, §29.150②.)
func TestHMC377R9P1TargetOwnWaitKeepsItsSeatWhenNoFamilySeatHoldsIt(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: r9p1Target}
	other := ThreadRef{Comm: "filler", PID: 301}
	chain := ChainResult{
		Target: target,
		Nodes:  []ChainNode{{ID: "n1", Thread: target, Window: TimeWindow{StartTs: 10.0, EndTs: 10.3}}},
		RootEvidence: []RootEvidence{
			{Type: "d_state_or_io_wait", Thread: target, DurationMs: 3, LineStart: 6, LineEnd: 7, Summary: "thread slept in D state", Confidence: 0.88, DominantState: string(StateDSleep)},
			{Type: "io_wait", Thread: target, DurationMs: 2, LineStart: 16, LineEnd: 17, Summary: "thread slept in D state", Confidence: 0.88, DominantState: string(StateDSleep)},
			{Type: "runnable_wait", Thread: target, DurationMs: 4, LineStart: 26, LineEnd: 27, Summary: "thread was runnable", Confidence: 0.8, DominantState: string(StateRunnable)},
		},
	}
	for name, stats := range map[string]WindowStats{
		"no_census": {},
		"census_of_another_thread": {
			dstateCensus:   map[string]ThreadDuration{"pid:301/cpu:2": {Thread: other, CPU: 2, DurationMs: 10}},
			iowaitCensus:   map[string]ThreadDuration{"pid:301/cpu:2": {Thread: other, CPU: 2, DurationMs: 10}},
			runnableCensus: map[string]ThreadDuration{"pid:301/cpu:2": {Thread: other, CPU: 2, DurationMs: 10}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			rank := buildRootCauseRankFrom(nil, Query{PID: r9p1Target, TimeStart: 10.0, TimeEnd: 10.3, Limit: 12}, chain, stats)
			minted := map[string]bool{}
			for _, item := range rank.Items {
				if item.Thread.PID == r9p1Target && strings.HasPrefix(item.Source, "wakeup_chain") {
					minted[item.Type] = true
				}
			}
			for _, typ := range []string{"d_state_or_io_wait", "io_wait", "runnable_wait"} {
				if !minted[typ] {
					t.Fatalf("the walked %s wait is the thread's only account and must keep its seat:%s", typ, r9p1Board(rank))
				}
			}
		})
	}
}
