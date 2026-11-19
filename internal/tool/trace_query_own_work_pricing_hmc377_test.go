package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 own-work pricing (lanes E1+E6), public trace_query entry, complete
// original captures (opt-in; skipped without the variable, failing when the
// file is not the pinned original). Every expected number comes from the
// independent raw oracle kept with the batch design
// (hmc377/design/own-work-pricing/oracle/ownwork_oracle.py), never from this
// engine.

// hmc377ListedSampleCapture: main-100 verifies 20 classes of 1 ms each while
// running; 15 other threads of the process each verify one class for 30 ms, so
// the 16 listing seats hold the 15 long ones and one of main's. main then
// sleeps and is woken by waker-150 (a dependency chain exists).
func hmc377ListedSampleCapture(t *testing.T) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	for i := 0; i < 15; i++ {
		c.span(10.001000+float64(i)*1e-5, 10.031000+float64(i)*1e-5, fmt.Sprintf("verify-%d", 300+i), 300+i, 100, 2+i%3, fmt.Sprintf("VerifyClass com.example.Other%d", i))
	}
	for i := 0; i < 20; i++ {
		start := 10.100000 + float64(i)*0.002
		c.span(start, start+0.001, "main", 100, 100, 1, fmt.Sprintf("VerifyClass com.example.Own%d", i))
	}
	c.run(10.000100, "waker", 150, 100, 5)
	c.sleep(10.150000, "main", 100, 100, 1)
	c.add(10.250000, "waker", 150, 100, 5, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.250100, "main", 100, 100, 1)
	c.sleep(10.300000, "main", 100, 100, 1)
	return c.write(t)
}

// E1 at the public entry: the main thread's own verification row is the 20
// members and their 20.000 ms, not the one member the listing shows.
func TestHMC377OwnWorkRowIsNotTheListedSample(t *testing.T) {
	rank := hmc376Query(t, hmc377ListedSampleCapture(t), "root_cause_rank", 100, 10.0, 10.32).RootCauseRank
	if rank == nil {
		t.Fatal("no ranking was published")
	}
	row := hmc377OwnRow(rank, 100, "class_verification")
	if row == nil || row.ChainRelevance != "on_chain" || row.MemberCount != 20 || !hmc376Near(row.EffectiveImpactMs, 20.0) {
		t.Fatalf("the main thread's own verification must be its 20 members / 20.000 ms:\n%s", hmc376Board(rank))
	}
}

// hmc377OwnRunningChainCapture: main-100 only runs (one short preemption), so
// the chain's only window on it is its longest running segment; it runs one
// 30 ms collection that straddles the preemption.
func hmc377OwnRunningChainCapture(t *testing.T) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.span(10.010000, 10.040000, "main", 100, 100, 1, "H:FullGC::RunPhases;GCReason2")
	c.add(10.020000, "main", 100, 100, 1, "sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=R+ ==> next_comm=other next_pid=200 next_prio=120")
	c.add(10.020100, "other", 200, 200, 1, "sched_switch: prev_comm=other prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=main next_pid=100 next_prio=120")
	c.sleep(10.050000, "main", 100, 100, 1)
	return c.write(t)
}

// E6 at the public entry: the collection the thread runs is priced by its
// whole in-window extent (30.000 ms) on the own-work lane, not by its overlap
// with the thread's own running segment the chain happened to name.
func TestHMC377OwnSpanIsNotPricedByTheThreadsOwnRunningSegment(t *testing.T) {
	rank := hmc376Query(t, hmc377OwnRunningChainCapture(t), "root_cause_rank", 100, 10.0, 10.06).RootCauseRank
	if rank == nil {
		t.Fatal("no ranking was published")
	}
	var gc *tracequery.RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Thread.PID == 100 && rank.Items[i].Type == "gc_pause" {
			if gc != nil {
				t.Fatalf("one collection, one row:\n%s", hmc376Board(rank))
			}
			gc = &rank.Items[i]
		}
	}
	if gc == nil || gc.ChainRelevance != "on_chain" || gc.OnChainBasis != tracequery.RootCauseOnChainBasisSelfDeterministicSpan ||
		!hmc376Near(gc.EffectiveImpactMs, 30.0) || gc.OverlapMs != 0 {
		t.Fatalf("the collection is 30.000 ms of the thread's own work:\n%s", hmc376Board(rank))
	}
}

func hmc377OwnRow(rank *tracequery.RootCauseRankResult, tid int, typ string) *tracequery.RootCauseRankItem {
	for i := range rank.Items {
		row := &rank.Items[i]
		if row.Thread.PID == tid && row.Type == typ && row.OnChainBasis == tracequery.RootCauseOnChainBasisSelfDeterministicSpan {
			return row
		}
	}
	return nil
}

// Douyin Lite, device-reboot cold start 5, the closed bindApplication marker:
// the main thread verifies 127 classes, 92.266 ms in the window, 80.201 ms of
// it on a CPU, largest 42.895 ms. Before HMC377 the row was folded from the
// listed sample: 5 members, 60.118 ms (the five largest).
func TestHMC377WholeOriginalDouyinRebootOwnWorkIsComplete(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_DOUYIN_REBOOT",
		"8087138a5810347675fe7764974f5103cad4a9a71e56d7d1702d65da2256892e", 138438125)
	const tid, start, end = 11105, 140.911304, 141.835458
	published, result := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": tid, "target_scope": "thread"}, start, end)
	states := result.TargetWindowStates
	if states == nil || !hmc376Near(states.RunningMs, 584.023) || !hmc376Near(states.RunnableMs, 24.510) ||
		!hmc376Near(states.SleepMs, 273.049) || !hmc376Near(states.DStateMs+states.IOWaitMs, 42.572) {
		t.Fatalf("the scheduler partition disagrees with the raw oracle: %+v", states)
	}
	if !hmc376Near(states.DeterministicRunningMs, 80.201) {
		t.Fatalf("the running time spent in the thread's own verification is 80.201 ms: %.3f", states.DeterministicRunningMs)
	}
	rank := result.RootCauseRank
	if rank == nil {
		t.Fatal("no ranking was published")
	}
	row := hmc377OwnRow(rank, tid, "class_verification")
	if row == nil {
		t.Fatalf("the main thread's own class verification is not a ranked on-chain cause:\n%s", hmc376Board(rank))
	}
	if row.ChainRelevance != "on_chain" || row.Rank <= 0 || row.MemberCount != 127 || !hmc376Near(row.EffectiveImpactMs, 92.266) ||
		!hmc376Near(row.CumulativeImpactMs, 92.266) || !hmc376Near(row.OwnWorkOnCPUMs, 80.201) || !hmc376Near(row.MemberMaxMs, 42.895) ||
		row.MemberNotListedCount != 95 || len(row.MemberRoster) != 32 {
		t.Fatalf("the own verification row must be complete (127 members, 92.266 ms, 80.201 ms on a CPU, 95 not listed): %s members=%d on_cpu=%.3f not_listed=%d roster=%d",
			hmc376RowLabel(*row), row.MemberCount, row.OwnWorkOnCPUMs, row.MemberNotListedCount, len(row.MemberRoster))
	}
	if !strings.Contains(published.Summary, "member_count=127 member_not_listed=95 own_work_on_cpu=80.201ms") {
		t.Fatal("the model-facing board row must state the complete count, the members not listed and the on-CPU share")
	}
	// EVOLUTION RECORD (HMC377 E1E6-R4): the bound caveat keeps the
	// lower-bound word for the one face still aggregated from the listed
	// spans (trace_mark_category), by name; no semantic family total is
	// called a lower bound.
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "Published semantic family totals are lower bounds") ||
			(strings.Contains(caveat, "lower bound") && !strings.Contains(caveat, "trace_mark_category totals of this result are still computed from the listed spans only")) {
			t.Fatalf("no semantic family total is a lower bound any more: %s", caveat)
		}
	}
	record := false
	for _, obs := range published.Observations {
		if obs.Predicate != "trace_semantic_span" || !strings.HasSuffix(obs.Subject, fmt.Sprintf("-%d", tid)) {
			continue
		}
		if hmc376RecordNote(obs, types.TraceNoteKeyMemberCount) != "127" || obs.Value != "92.266" ||
			hmc376RecordNote(obs, types.TraceNoteKeyMemberNotListed) != "95" {
			t.Fatalf("the semantic record must agree with the board row: value=%s notes=%v", obs.Value, obs.RichNotes)
		}
		record = true
	}
	if !record {
		t.Fatal("the main thread's verification record is missing from the semantic channel")
	}
}

// Douyin Lite, container restart 1 (the gallery service capture): the service's
// main thread runs one full collection, 75.656 ms wall, 73.164 ms on a CPU.
// On the natural windows (the running burst, and the collection marker
// itself) the only chain window on the thread is one of its own running
// segments (15.938 ms); before HMC377 the collection was priced by that
// fragment and ranked behind the plain running row.
func TestHMC377WholeOriginalGalleryCollectionIsPricedOnNaturalWindows(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_GALLERY_FULLGC",
		"d27ff641787dfa0768928e0ce3d11a7d2a6caa0f2910c941fae3e97f2256e22e", 245420208)
	const tid = 55300
	for _, w := range []struct {
		name             string
		start, end       float64
		runnable, onCPU  float64
		affinityRunnable float64
	}{
		{"running burst", 2493.678757, 2493.756365, 1.845, 73.164, 1.845},
		{"collection marker", 2493.679887, 2493.755543, 1.566, 73.164, 1.566},
	} {
		t.Run(w.name, func(t *testing.T) {
			rank := hmc376Query(t, path, "root_cause_rank", tid, w.start, w.end).RootCauseRank
			if rank == nil {
				t.Fatal("no ranking was published")
			}
			gc := hmc377OwnRow(rank, tid, "gc_pause")
			if gc == nil || gc.Rank != 1 || gc.Tier != "primary" || gc.ChainRelevance != "on_chain" ||
				!hmc376Near(gc.EffectiveImpactMs, 75.656) || !hmc376Near(gc.OwnWorkOnCPUMs, w.onCPU) || gc.OverlapMs != 0 {
				t.Fatalf("the collection the thread runs is the first on-chain cause at 75.656 ms (73.164 on a CPU):\n%s", hmc376Board(rank))
			}
			for _, row := range rank.Items {
				if row.Thread.PID == tid && row.Type == "gc_pause" && row.OnChainBasis == "" {
					t.Fatalf("the thread's own running segment must not price its own collection: %s", hmc376RowLabel(row))
				}
			}
			// The thread's scheduling-supply rows stay on the chain.
			affinity := false
			for _, row := range rank.Items {
				if row.Thread.PID == tid && row.Type == "cpu_affinity_or_cpuset" {
					affinity = row.ChainRelevance == "on_chain" && hmc376Near(row.EffectiveImpactMs, w.affinityRunnable)
				}
			}
			if !affinity {
				t.Fatalf("the thread's restricted-CPU scheduling row must stay an on-chain cause:\n%s", hmc376Board(rank))
			}
		})
	}
}

// A marker that records a wait for a collection stays the wait row, priced by
// the sleep, on both NetEase windows; it never gains an own-work row.
func TestHMC377WholeOriginalRecordedCollectionWaitsStayWaits(t *testing.T) {
	for _, c := range []struct {
		name, env, digest string
		size              int64
		tid               int
		start, end        float64
		wait, marker      float64
	}{
		{"killed-in-background cold start", "CODRAX_HMC376_NETEASE_GCWAIT", "ae52285a00c4afce43e48bae519615110973052370537e7c53f5e4351073f2e0", 106869666,
			34044, 5775.523119, 5775.790999, 247.304, 247.466},
		{"device-reboot cold start", "CODRAX_HMC376_NETEASE_REBOOT_FRAME", "48cf2fd8b91a81f2276a16a5988ef14729c078e5d1f23615eb166d2f6e2c8cbc", 104144939,
			11077, 153.763595, 153.887340, 123.003, 123.089},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := hmc376Original(t, c.env, c.digest, c.size)
			rank := hmc376Query(t, path, "root_cause_rank", c.tid, c.start, c.end).RootCauseRank
			if rank == nil {
				t.Fatal("no ranking was published")
			}
			var wait *tracequery.RootCauseRankItem
			for i := range rank.Items {
				row := &rank.Items[i]
				if row.Thread.PID != c.tid || row.Type != "gc_pause" {
					continue
				}
				if row.OnChainBasis == tracequery.RootCauseOnChainBasisSelfDeterministicSpan || row.OwnWorkOnCPUMs != 0 {
					t.Fatalf("a recorded wait gained an own-work row: %s", hmc376RowLabel(*row))
				}
				wait = row
			}
			if wait == nil || wait.Rank != 1 || wait.DominantState != "s_sleep" || !hmc376Near(wait.EffectiveImpactMs, c.wait) || !hmc376Near(wait.ActualImpactMs, c.marker) {
				t.Fatalf("the wait for the collection stays the first cause, priced by the sleep:\n%s", hmc376Board(rank))
			}
		})
	}
}
