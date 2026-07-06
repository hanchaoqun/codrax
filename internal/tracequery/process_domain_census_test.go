package tracequery

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// WSR §8 b3 donghu-shaped fixture (real_trace_campaign_20260705 §8.1): a
// 200ms window (10.000..10.200) on 4 CPUs where the target process
// com.baidu.tieba (tgid 59566) has 39 window-observed threads but its
// fragmented cross-CPU runner NetworkService-60595 never survives the global
// top-8 running roster:
//
//   - 7 hog processes monopolize the global top-8 buckets (69..140ms each);
//     the 8th surviving bucket is tb1's single 6.0ms segment.
//   - NetworkService runs 5.000ms on cpu0 + 4.635ms on cpu1 + 3.500ms on
//     cpu2 = 13.135ms merged (the customer's 碎跑 shape: its largest single
//     (pid,comm,cpu) bucket is 5.0ms < tb1's 6.0ms, so the per-bucket global
//     roster dilutes it below the cut even though its merged total is #1
//     inside the process).
//   - tb1..tb9 run 6/4/3/4/3/4/4/4/4 ms; tb1..tb4 also have distinct
//     runnable waits (5/4/3/2.5ms) so exactly four tieba threads survive into
//     ThreadCPULoad → the legacy process_cpu_load face reports the b3
//     masquerade "threads=4".
//   - The census truth: 39 threads = main 59566 + NetworkService + tb1..tb9
//     + 28 census-only workers observed via print C-marks; 10 of them ran.
func wsrB3CensusTrace() string {
	var b strings.Builder
	b.WriteString(`          <idle>-0 (-----) [000] .... 10.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hog1 next_pid=70001 next_prio=20
          <idle>-0 (-----) [001] .... 10.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hog3 next_pid=70003 next_prio=20
          <idle>-0 (-----) [002] .... 10.000000: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hog5 next_pid=70005 next_prio=20
          <idle>-0 (-----) [003] .... 10.000000: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hog7 next_pid=70007 next_prio=20
        hog5-70005 (70005) [002] .... 10.071000: sched_switch: prev_comm=hog5 prev_pid=70005 prev_prio=20 prev_state=S ==> next_comm=hog6 next_pid=70006 next_prio=20
        hog3-70003 (70003) [001] .... 10.076000: sched_switch: prev_comm=hog3 prev_pid=70003 prev_prio=20 prev_state=S ==> next_comm=hog4 next_pid=70004 next_prio=20
        hog1-70001 (70001) [000] .... 10.080000: sched_switch: prev_comm=hog1 prev_pid=70001 prev_prio=20 prev_state=S ==> next_comm=hog2 next_pid=70002 next_prio=20
        hog7-70007 (70007) [003] .... 10.140000: sched_switch: prev_comm=hog7 prev_pid=70007 prev_prio=20 prev_state=S ==> next_comm=tb7 next_pid=59573 next_prio=20
        hog2-70002 (70002) [000] .... 10.144000: sched_wakeup: comm=tb1 pid=59567 prio=20 target_cpu=000
         tb7-59573 (59566) [003] .... 10.144000: sched_switch: prev_comm=tb7 prev_pid=59573 prev_prio=20 prev_state=S ==> next_comm=tb8 next_pid=59574 next_prio=20
         tb8-59574 (59566) [003] .... 10.148000: sched_switch: prev_comm=tb8 prev_pid=59574 prev_prio=20 prev_state=S ==> next_comm=tb9 next_pid=59575 next_prio=20
        hog2-70002 (70002) [000] .... 10.149000: sched_switch: prev_comm=hog2 prev_pid=70002 prev_prio=20 prev_state=S ==> next_comm=tb1 next_pid=59567 next_prio=20
        hog4-70004 (70004) [001] .... 10.152000: sched_wakeup: comm=tb3 pid=59569 prio=20 target_cpu=001
         tb9-59575 (59566) [003] .... 10.152000: sched_switch: prev_comm=tb9 prev_pid=59575 prev_prio=20 prev_state=S ==> next_comm=swapper/3 next_pid=0 next_prio=120
         tb1-59567 (59566) [000] .... 10.155000: sched_switch: prev_comm=tb1 prev_pid=59567 prev_prio=20 prev_state=S ==> next_comm=NetworkService next_pid=60595 next_prio=20
        hog4-70004 (70004) [001] .... 10.155000: sched_switch: prev_comm=hog4 prev_pid=70004 prev_prio=20 prev_state=S ==> next_comm=tb3 next_pid=59569 next_prio=20
         tb3-59569 (59566) [001] .... 10.155500: sched_wakeup: comm=tb4 pid=59570 prio=20 target_cpu=001
NetworkService-60595 (59566) [000] .... 10.156000: sched_wakeup: comm=tb2 pid=59568 prio=20 target_cpu=000
         tb3-59569 (59566) [001] .... 10.158000: sched_switch: prev_comm=tb3 prev_pid=59569 prev_prio=20 prev_state=S ==> next_comm=tb4 next_pid=59570 next_prio=20
NetworkService-60595 (59566) [000] .... 10.160000: sched_switch: prev_comm=NetworkService prev_pid=60595 prev_prio=20 prev_state=S ==> next_comm=tb2 next_pid=59568 next_prio=20
        hog6-70006 (70006) [002] .... 10.161000: sched_switch: prev_comm=hog6 prev_pid=70006 prev_prio=20 prev_state=S ==> next_comm=tb5 next_pid=59571 next_prio=20
         tb4-59570 (59566) [001] .... 10.162000: sched_switch: prev_comm=tb4 prev_pid=59570 prev_prio=20 prev_state=S ==> next_comm=NetworkService next_pid=60595 next_prio=20
         tb2-59568 (59566) [000] .... 10.164000: sched_switch: prev_comm=tb2 prev_pid=59568 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
         tb5-59571 (59566) [002] .... 10.164000: sched_switch: prev_comm=tb5 prev_pid=59571 prev_prio=20 prev_state=S ==> next_comm=tb6 next_pid=59572 next_prio=20
NetworkService-60595 (59566) [001] .... 10.166635: sched_switch: prev_comm=NetworkService prev_pid=60595 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
         tb6-59572 (59566) [002] .... 10.168000: sched_switch: prev_comm=tb6 prev_pid=59572 prev_prio=20 prev_state=S ==> next_comm=NetworkService next_pid=60595 next_prio=20
NetworkService-60595 (59566) [002] .... 10.171500: sched_switch: prev_comm=NetworkService prev_pid=60595 prev_prio=20 prev_state=S ==> next_comm=swapper/2 next_pid=0 next_prio=120
com.baidu.tieba-59566 (59566) [000] .... 10.180000: print: C|59566|wsr_census_probe|1
`)
	// 28 census-only workers: observed in the window scope with the (59566)
	// TGID column, never scheduled — they exist only in the true census.
	for i := 1; i <= 28; i++ {
		fmt.Fprintf(&b, "         tbw%02d-%d (59566) [001] .... 10.%06d: print: C|59566|wsr_census_probe|1\n", i, 59575+i, 180000+i*100)
	}
	return b.String()
}

func wsrB3CensusQuery(pid int) Query {
	return Query{PID: pid, TimeStart: 10.0, TimeEnd: 10.2, TraceFlavorHint: TraceFlavorHarmonyHitrace}
}

// TestProcessDomainCensusDonghuShapePin pins the WSR §8 b3 lane end to end on
// the donghu shape:
//   - lane exists on a pid query and anchors the tgid-level process
//     (车道摘除 mutation → census nil → red);
//   - threads= is the TRUE census (39), while the legacy process_cpu_load
//     face still reports the surviving-roster 4 in the SAME run (threads=
//     回退幸存者数 mutation → 39 becomes 4 → red; legacy face rewired to the
//     full buckets → 4 becomes 39 → red);
//   - NetworkService-60595 leads the roster with its cross-CPU merged
//     13.135ms even though it is absent from every global display face;
//   - roster cap 8 + PTS fold discipline (count + aggregate) for the two
//     3.0ms threads beyond the cap.
func TestProcessDomainCensusDonghuShapePin(t *testing.T) {
	idx := buildTraceIndex(t, "wsr_b3_census.systrace", wsrB3CensusTrace())
	stats := ComputeWindowStats(idx, wsrB3CensusQuery(59566))
	census := stats.ProcessDomainCensus
	if census == nil {
		t.Fatal("pid-scoped window_stats must publish the process-domain census lane (WSR §8 b3)")
	}
	if census.Process.PID != 59566 || census.Process.Comm != "com.baidu.tieba" {
		t.Fatalf("census process = %+v, want com.baidu.tieba-59566", census.Process)
	}
	if census.ThreadCount != 39 {
		t.Fatalf("census ThreadCount = %d, want the true 39-thread census (survivor-count regression would report 4)", census.ThreadCount)
	}
	if census.RunningThreadCount != 10 {
		t.Fatalf("census RunningThreadCount = %d, want 10", census.RunningThreadCount)
	}
	approxEq(t, "TotalRunningMs", census.TotalRunningMs, 49.135)

	// NetworkService: cross-CPU merged 5.000+4.635+3.500 = 13.135ms, roster #1.
	if len(census.TopThreads) != 8 {
		t.Fatalf("census roster = %d rows, want the shared 8-slot roster cap: %+v", len(census.TopThreads), census.TopThreads)
	}
	ns := census.TopThreads[0]
	if ns.Thread.Comm != "NetworkService" || ns.Thread.PID != 60595 {
		t.Fatalf("census roster #1 = %+v, want NetworkService-60595", ns.Thread)
	}
	approxEq(t, "NetworkService merged RunningMs", ns.RunningMs, 13.135)
	if !reflect.DeepEqual(ns.CPUs, []int{0, 1, 2}) {
		t.Fatalf("NetworkService census CPUs = %v, want the merged [0 1 2]", ns.CPUs)
	}
	if census.TopThreads[1].Thread.PID != 59567 {
		t.Fatalf("census roster #2 = %+v, want tb1-59567 (6.0ms)", census.TopThreads[1].Thread)
	}
	approxEq(t, "tb1 RunningMs", census.TopThreads[1].RunningMs, 6.0)

	// PTS fold discipline: the two 3.0ms threads beyond the cap are counted
	// and aggregated, never silently dropped.
	if census.FoldedThreadCount != 2 {
		t.Fatalf("FoldedThreadCount = %d, want 2", census.FoldedThreadCount)
	}
	approxEq(t, "FoldedRunningMs", census.FoldedRunningMs, 6.0)
	for _, td := range census.TopThreads {
		if td.RunningMs < 3.5 {
			t.Fatalf("roster row %v below the fold boundary leaked into the top list", td)
		}
	}
	if !reflect.DeepEqual(census.CPUs, []int{0, 1, 2, 3}) {
		t.Fatalf("census CPUs = %v, want [0 1 2 3]", census.CPUs)
	}

	// Same-run legacy contrast: the global process face still rolls up ONLY
	// the surviving display roster (threads=4, the b3 masquerade shape) and
	// NetworkService stays absent from every global display face.
	var legacy *ProcessCPULoadSummary
	for i := range stats.ProcessCPULoad {
		if stats.ProcessCPULoad[i].Process.PID == 59566 {
			legacy = &stats.ProcessCPULoad[i]
		}
	}
	if legacy == nil {
		t.Fatalf("expected the legacy tieba process_cpu_load row: %+v", stats.ProcessCPULoad)
	}
	if legacy.ThreadCount != 4 {
		t.Fatalf("legacy process_cpu_load ThreadCount = %d, want the surviving-roster 4 (the legacy lane must NOT be rewired to the census)", legacy.ThreadCount)
	}
	approxEq(t, "legacy tieba RunningMs", legacy.RunningMs, 6.0)
	approxEq(t, "legacy tieba RunnableWaitMs", legacy.RunnableWaitMs, 14.5)
	for _, td := range stats.TopRunning {
		if td.Thread.PID == 60595 {
			t.Fatalf("NetworkService must stay diluted out of the global top-8 running roster (largest bucket 5.0ms < tb1's 6.0ms): %+v", stats.TopRunning)
		}
	}
	for _, load := range stats.ThreadCPULoad {
		if load.Thread.PID == 60595 {
			t.Fatalf("NetworkService must not appear in the global thread_cpu_load survivors: %+v", stats.ThreadCPULoad)
		}
	}

	// Honesty caveats (WSR 核验 F2): the load-bearing clauses exist, zh/en
	// ride separate entries, and every entry fits the 200-byte tool banner
	// value cap (sanitizeForBanner) so no consumer lane can truncate a
	// clause away — re-merging into a >200B mega-caveat goes red here.
	joined := strings.Join(census.Caveats, "\n")
	for _, clause := range []string{
		"threads=39 is the true in-window census",
		"counts survivors, not the process",
		"非展示名册幸存者数",
		"cpu·ms",
		"不可当作墙钟耗时",
	} {
		if !strings.Contains(joined, clause) {
			t.Fatalf("census caveats missing load-bearing clause %q: %v", clause, census.Caveats)
		}
	}
	for _, caveat := range census.Caveats {
		if len(caveat) >= 200 {
			t.Fatalf("census caveat exceeds the 200-byte banner value cap and would be truncated on the rendered face (%d bytes): %s", len(caveat), caveat)
		}
	}
}

// TestProcessDomainCensusTimeWindowCaliberPin is the WSR 核验 F1 pin: the
// census counts only threads OBSERVED inside the line+time window (same
// admission predicate as the running side). The catalog is line-window
// scoped, so without the time predicate a thread whose only events sit
// before/after the time window still inflates threads= (Efw-802 shape).
func TestProcessDomainCensusTimeWindowCaliberPin(t *testing.T) {
	// Two extra tgid-59566 threads observed ONLY outside the 10.0..10.2 time
	// window: tby pre-window (9.9), tbx post-window (10.9). Both enter the
	// line-scope catalog; neither may enter the census.
	content := "         tby-59651 (59566) [000] .... 9.900000: print: C|59566|wsr_census_probe|1\n" +
		wsrB3CensusTrace() +
		"         tbx-59650 (59566) [000] .... 10.900000: print: C|59566|wsr_census_probe|1\n"
	idx := buildTraceIndex(t, "wsr_b3_census_timewindow.systrace", content)
	stats := ComputeWindowStats(idx, wsrB3CensusQuery(59566))
	census := stats.ProcessDomainCensus
	if census == nil {
		t.Fatal("expected the census lane")
	}
	if census.ThreadCount != 39 {
		t.Fatalf("census ThreadCount = %d, want 39: out-of-time-window threads (tby@9.9 / tbx@10.9) must not be counted", census.ThreadCount)
	}

	// Efw-802 shape: the target process exists in the trace but ALL its
	// events sit before the queried time window → no fabricated census.
	const preWindowOnly = `          <idle>-0 (-----) [000] .... 9.850000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=efw next_pid=802 next_prio=20
         efw-802 (  800) [000] .... 9.900000: sched_switch: prev_comm=efw prev_pid=802 prev_prio=20 prev_state=S ==> next_comm=efw2 next_pid=803 next_prio=20
        efw2-803 (  800) [000] .... 9.950000: sched_switch: prev_comm=efw2 prev_pid=803 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`
	preIdx := buildTraceIndex(t, "wsr_b3_census_prewindow.systrace", preWindowOnly)
	preStats := ComputeWindowStats(preIdx, wsrB3CensusQuery(802))
	if preStats.ProcessDomainCensus != nil {
		t.Fatalf("all-pre-window process must not publish an in-window census (Efw-802 shape): %+v", preStats.ProcessDomainCensus)
	}
}

// TestProcessDomainCensusGlobalFacesUntouchedPin is the additive-lane pin:
// the pid enters ONLY the census lane — the global top-8 leaderboard and the
// survivor chain built from it (TopRunning → ThreadCPULoad → ProcessCPULoad,
// plus RunnableTop) are typed-identical with and without the pid, and the
// census itself exists only on the pid run.
func TestProcessDomainCensusGlobalFacesUntouchedPin(t *testing.T) {
	idx := buildTraceIndex(t, "wsr_b3_census_faces.systrace", wsrB3CensusTrace())
	withPID := ComputeWindowStats(idx, wsrB3CensusQuery(59566))
	noPID := ComputeWindowStats(idx, wsrB3CensusQuery(0))
	if noPID.ProcessDomainCensus != nil {
		t.Fatalf("target-less window_stats must not publish a census: %+v", noPID.ProcessDomainCensus)
	}
	if withPID.ProcessDomainCensus == nil {
		t.Fatal("pid window_stats must publish the census lane")
	}
	if !reflect.DeepEqual(withPID.TopRunning, noPID.TopRunning) {
		t.Fatalf("global top_running face must be identical with/without pid:\nwith: %+v\nwithout: %+v", withPID.TopRunning, noPID.TopRunning)
	}
	if !reflect.DeepEqual(withPID.RunnableTop, noPID.RunnableTop) {
		t.Fatalf("global runnable face must be identical with/without pid:\nwith: %+v\nwithout: %+v", withPID.RunnableTop, noPID.RunnableTop)
	}
	if !reflect.DeepEqual(withPID.ThreadCPULoad, noPID.ThreadCPULoad) {
		t.Fatalf("global thread_cpu_load face must be identical with/without pid:\nwith: %+v\nwithout: %+v", withPID.ThreadCPULoad, noPID.ThreadCPULoad)
	}
	if !reflect.DeepEqual(withPID.ProcessCPULoad, noPID.ProcessCPULoad) {
		t.Fatalf("global process_cpu_load face must be identical with/without pid:\nwith: %+v\nwithout: %+v", withPID.ProcessCPULoad, noPID.ProcessCPULoad)
	}
}

// TestProcessDomainCensusChildTidAnchorsProcess: anchoring on a member tid
// (NetworkService's own pid) resolves the SAME tgid-level process domain as
// anchoring on the main thread.
func TestProcessDomainCensusChildTidAnchorsProcess(t *testing.T) {
	idx := buildTraceIndex(t, "wsr_b3_census_child.systrace", wsrB3CensusTrace())
	stats := ComputeWindowStats(idx, wsrB3CensusQuery(60595))
	census := stats.ProcessDomainCensus
	if census == nil {
		t.Fatal("member-tid query must anchor the process-domain census")
	}
	if census.Process.PID != 59566 {
		t.Fatalf("census process = %+v, want the tgid-level com.baidu.tieba-59566", census.Process)
	}
	if census.ThreadCount != 39 {
		t.Fatalf("census ThreadCount = %d, want 39", census.ThreadCount)
	}
	if census.Target.PID != 60595 {
		t.Fatalf("census target = %+v, want the anchoring tid 60595", census.Target)
	}
}

// TestProcessDomainCensusUnresolvableProcessStaysNil: a target with no tgid
// linkage anywhere in the window must NOT fabricate a "threads=1 process" —
// the lane stays nil and the legacy faces stand alone.
func TestProcessDomainCensusUnresolvableProcessStaysNil(t *testing.T) {
	const trace = `          <idle>-0 (-----) [000] .... 10.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=lone next_pid=777 next_prio=20
        lone-777 (-----) [000] .... 10.050000: sched_switch: prev_comm=lone prev_pid=777 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`
	idx := buildTraceIndex(t, "wsr_b3_census_notgid.systrace", trace)
	stats := ComputeWindowStats(idx, wsrB3CensusQuery(777))
	if stats.ProcessDomainCensus != nil {
		t.Fatalf("no tgid linkage → no census (must not fabricate a 1-thread process): %+v", stats.ProcessDomainCensus)
	}
}
