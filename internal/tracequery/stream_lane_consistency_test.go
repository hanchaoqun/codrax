package tracequery

// stream_lane_consistency_test.go — streaming-lane consistency pins (audit
// findings #48-#53, 2026-07-10 STREAM batch): the streaming faces
// (StreamEventSearch / StreamStateCluster / StreamWindowSweep) must agree
// with the indexed lane's published authorities — scheduler head carry
// (applySchedulerHeadEvent), the mixed line+time window convention
// (eventInQueryWindow), parse-quality disclosure (indexed Run caveats), and
// typed head-coverage honesty under selector rejection.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestStreamStateClusterPreWindowWakeupOnlyStarvedThreadParity (audit #48)
// pins the canonical runnable-starvation shape on BOTH lanes: a thread whose
// ONLY pre-window evidence is a normal sched_wakeup (its switch-out predates
// the capture) and that never gets scheduled inside the window. The indexed
// head authority (applySchedulerHeadEvent) publishes a StateRunnable
// checkpoint for that pre-boundary wakeup, so the streaming carry lane must
// account the same full-window runnable first segment instead of silently
// dropping the thread while headCoverage claims "recovered".
func TestStreamStateClusterPreWindowWakeupOnlyStarvedThreadParity(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "starved_prewindow_wakeup.systrace",
		"  other-9 (9) [001] .... 0.050000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=9 next_prio=120",
		" creator-7 (7) [000] .... 0.100000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		"  other-9 (9) [001] .... 1.020000: sched_switch: prev_comm=other prev_pid=9 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120",
	)

	// Streaming lane: index_event_limit fallback face.
	stream, err := StreamStateCluster(t.Context(), path, Query{TimeStart: 1.0, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if stream.WindowStats == nil {
		t.Fatal("stream state cluster returned no window stats")
	}
	streamRunnable := threadDurationForPID(stream.WindowStats.RunnableTop, 42)
	if streamRunnable == nil || !near(streamRunnable.DurationMs, 100, 0.001) {
		t.Fatalf("pre-window wakeup-only starved thread lost its full-window runnable segment on the streaming lane: %+v", stream.WindowStats.RunnableTop)
	}
	coverage := stream.WindowStats.SchedulerHeadCoverage
	if coverage == nil || coverage.Status != "recovered" || coverage.MissingThreadCount != 0 {
		t.Fatalf("with the wakeup consumed as a runnable checkpoint the head coverage must be honestly recovered: %+v", coverage)
	}

	// Indexed lane: same window, published head-carry authority.
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	indexedRunnable := threadDurationForPID(stats.RunnableTop, 42)
	if indexedRunnable == nil {
		t.Fatalf("indexed lane lost the runnable head carry (authority regression): %+v", stats.RunnableTop)
	}
	if !near(indexedRunnable.DurationMs, streamRunnable.DurationMs, 0.001) {
		t.Fatalf("streaming/indexed runnable parity broken for the pre-window wakeup-only thread: indexed=%.3fms stream=%.3fms",
			indexedRunnable.DurationMs, streamRunnable.DurationMs)
	}
}

// TestStreamStateClusterInWindowHeadlessWakeupMintsRunnableKeepsDisclosure
// pins the in-window headless-wakeup semantics after the alignment ruling.
//
// EVOLUTION RECORD (主会话裁定 2026-07-10, §29.26 待落账): this pin previously
// asserted the OPPOSITE shape ("no fabricated interval" — the streaming lane
// dropped an in-window wakeup for a thread with no governing open state and
// only disclosed it as missing). The ruling flipped it to direction (b'):
// the wakeup is a witnessed typed transition — a runnable segment is minted
// from the wakeup timestamp (matching the indexed offCPU face, which always
// minted it, closing the cross-lane number gap on the fallback face) —
// while the missing/partial_unknown disclosure is RETAINED, because the
// prefix before the wakeup remains un-witnessed and the two statements are
// orthogonal.
func TestStreamStateClusterInWindowHeadlessWakeupMintsRunnableKeepsDisclosure(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "inwindow_wakeup_missing_head.systrace",
		"  other-9 (9) [001] .... 0.050000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=9 next_prio=120",
		" creator-7 (7) [000] .... 1.030000: sched_wakeup: comm=late pid=55 prio=120 target_cpu=000",
	)
	stream, err := StreamStateCluster(t.Context(), path, Query{TimeStart: 1.0, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if stream.WindowStats == nil {
		t.Fatal("stream state cluster returned no window stats")
	}
	minted := threadDurationForPID(stream.WindowStats.RunnableTop, 55)
	if minted == nil || !near(minted.DurationMs, 70, 0.001) {
		t.Fatalf("in-window headless wakeup must mint a runnable segment from the wakeup ts (want 70ms): %+v", stream.WindowStats.RunnableTop)
	}
	coverage := stream.WindowStats.SchedulerHeadCoverage
	if coverage == nil || coverage.Status != "partial_unknown" {
		t.Fatalf("the un-witnessed prefix must stay disclosed as partial_unknown despite the minted suffix: %+v", coverage)
	}
	found := false
	for _, pid := range coverage.MissingThreadPIDs {
		if pid == 55 {
			found = true
		}
	}
	if !found {
		t.Fatalf("pid 55 must be listed among the missing head threads: %+v", coverage.MissingThreadPIDs)
	}

	// Indexed parity: the offCPU face has always minted this segment — the
	// fallback face must now give the same number.
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	indexed := threadDurationForPID(stats.RunnableTop, 55)
	if indexed == nil || !near(indexed.DurationMs, minted.DurationMs, 0.001) {
		t.Fatalf("streaming/indexed headless-wakeup runnable parity broken: indexed=%+v stream=%.3fms", indexed, minted.DurationMs)
	}
}

// TestHeadlessWakeupRunnableParityAcrossThreeFaces pins the three-face
// alignment the ruling mandates on a fragmented fixture whose FIRST runnable
// fragment exists only via the headless-wakeup mint: the streaming
// state-cluster face, the indexed offCPU face (RunnableTop), and the indexed
// churn face (state_churn RunnableMs) must publish the same runnable account.
// Before the churn-face alignment the first fragment was silently dropped
// there (15ms instead of 35ms) while offCPU already counted it — an
// indexed-lane internal inconsistency.
func TestHeadlessWakeupRunnableParityAcrossThreeFaces(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "headless_churn_parity.systrace",
		" creator-7 (7) [000] .... 1.010000: sched_wakeup: comm=frag pid=55 prio=120 target_cpu=000",
		"  idle-0 (0) [000] .... 1.030000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
		"   frag-55 (55) [000] .... 1.045000: sched_switch: prev_comm=frag prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" creator-7 (7) [000] .... 1.060000: sched_wakeup: comm=frag pid=55 prio=120 target_cpu=000",
		"  idle-0 (0) [000] .... 1.075000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
		"   frag-55 (55) [000] .... 1.090000: sched_switch: prev_comm=frag prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
	)
	const wantRunnableMs = 35 // headless mint 1.010..1.030 (20ms) + 1.060..1.075 (15ms)
	q := Query{TimeStart: 1.0, TimeEnd: 1.1}

	// Face 1: streaming state cluster (RunnableTop + its churn rows).
	stream, err := StreamStateCluster(t.Context(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if stream.WindowStats == nil {
		t.Fatal("stream state cluster returned no window stats")
	}
	streamTop := threadDurationForPID(stream.WindowStats.RunnableTop, 55)
	if streamTop == nil || !near(streamTop.DurationMs, wantRunnableMs, 0.001) {
		t.Fatalf("streaming RunnableTop must include the headless first fragment (want %dms): %+v", wantRunnableMs, stream.WindowStats.RunnableTop)
	}

	// Face 2: indexed offCPU (ComputeWindowStats RunnableTop) — the
	// pre-ruling authority that always minted.
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	stats := ComputeWindowStats(idx, q)
	indexedTop := threadDurationForPID(stats.RunnableTop, 55)
	if indexedTop == nil || !near(indexedTop.DurationMs, streamTop.DurationMs, 0.001) {
		t.Fatalf("offCPU/streaming runnable parity broken: indexed=%+v stream=%.3fms", indexedTop, streamTop.DurationMs)
	}

	// Face 3: indexed churn (state_churn) — aligned by the ruling.
	churnRunnable := func(rows []ThreadStateChurnSummary, pid int) (float64, bool) {
		for _, row := range rows {
			if row.Thread.PID == pid {
				return row.RunnableMs, true
			}
		}
		return 0, false
	}
	indexedChurn, ok := churnRunnable(stats.StateChurn, 55)
	if !ok {
		t.Fatalf("fragmented fixture must produce an indexed churn row: %+v", stats.StateChurn)
	}
	if !near(indexedChurn, wantRunnableMs, 0.001) {
		t.Fatalf("indexed churn face must count the headless first fragment: got %.3fms want %dms", indexedChurn, wantRunnableMs)
	}
	streamChurn, ok := churnRunnable(stream.WindowStats.StateChurn, 55)
	if !ok {
		t.Fatalf("fragmented fixture must produce a streaming churn row: %+v", stream.WindowStats.StateChurn)
	}
	if !near(streamChurn, indexedChurn, 0.001) {
		t.Fatalf("churn-face runnable parity broken: indexed=%.3fms stream=%.3fms", indexedChurn, streamChurn)
	}

	// Disclosure retained on the fallback face: the prefix before the first
	// wakeup is still un-witnessed.
	coverage := stream.WindowStats.SchedulerHeadCoverage
	if coverage == nil || coverage.Status != "partial_unknown" {
		t.Fatalf("minting must not silence the un-witnessed-prefix disclosure: %+v", coverage)
	}
}

// TestStreamStateClusterPreWindowWakeupRunSleepParity (review F1, shape A):
// pre-window wakeup → pre-window run → pre-window sleep (prev_state=S). The
// thread's governing state at the window head is SLEEP — the carry machine
// must ride through the whole pre-window sequence and land on sleep, never
// leaving a stale runnable account. Both lanes: SleepTop 100ms, RunnableTop
// and TopRunning empty for the pid.
func TestStreamStateClusterPreWindowWakeupRunSleepParity(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "carry_wakeup_run_sleep.systrace",
		" creator-7 (7) [000] .... 0.100000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		"  idle-0 (0) [000] .... 0.200000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
		" target-42 (42) [000] .... 0.300000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
	)
	q := Query{TimeStart: 1.0, TimeEnd: 1.1}

	stream, err := StreamStateCluster(t.Context(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if stream.WindowStats == nil {
		t.Fatal("stream state cluster returned no window stats")
	}
	streamSleep := threadDurationForPID(stream.WindowStats.SleepTop, 42)
	if streamSleep == nil || !near(streamSleep.DurationMs, 100, 0.001) {
		t.Fatalf("carry must land on the final pre-window SLEEP state (want 100ms): %+v", stream.WindowStats.SleepTop)
	}
	if td := threadDurationForPID(stream.WindowStats.RunnableTop, 42); td != nil {
		t.Fatalf("stale runnable account survived the pre-window run+sleep transitions: %+v", td)
	}
	if td := threadDurationForPID(stream.WindowStats.TopRunning, 42); td != nil {
		t.Fatalf("fully pre-window running segment must not leak into the window: %+v", td)
	}

	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	stats := ComputeWindowStats(idx, q)
	indexedSleep := threadDurationForPID(stats.SleepTop, 42)
	if indexedSleep == nil || !near(indexedSleep.DurationMs, streamSleep.DurationMs, 0.001) {
		t.Fatalf("streaming/indexed sleep-carry parity broken: indexed=%+v stream=%.3fms", indexedSleep, streamSleep.DurationMs)
	}
	if td := threadDurationForPID(stats.RunnableTop, 42); td != nil {
		t.Fatalf("indexed lane fabricated a runnable account for a sleeping carry: %+v", td)
	}
}

// TestStreamStateClusterPreWindowWakeupRunPreemptParity (review F1, shape B):
// pre-window wakeup → pre-window run → preempted (prev_state=R). The thread
// is genuinely RUNNABLE at the window head — both lanes account a 100ms
// runnable head segment, and it is a true runnable (preemption), not a stale
// leftover of the earlier wakeup.
func TestStreamStateClusterPreWindowWakeupRunPreemptParity(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "carry_wakeup_run_preempt.systrace",
		" creator-7 (7) [000] .... 0.100000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		"  idle-0 (0) [000] .... 0.200000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
		" target-42 (42) [000] .... 0.300000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=R ==> next_comm=swapper/0 next_pid=0 next_prio=120",
	)
	q := Query{TimeStart: 1.0, TimeEnd: 1.1}

	stream, err := StreamStateCluster(t.Context(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if stream.WindowStats == nil {
		t.Fatal("stream state cluster returned no window stats")
	}
	streamRunnable := threadDurationForPID(stream.WindowStats.RunnableTop, 42)
	if streamRunnable == nil || !near(streamRunnable.DurationMs, 100, 0.001) {
		t.Fatalf("preempted carry must land on a 100ms runnable head segment: %+v", stream.WindowStats.RunnableTop)
	}
	if td := threadDurationForPID(stream.WindowStats.SleepTop, 42); td != nil {
		t.Fatalf("preemption (prev_state=R) must not book sleep: %+v", td)
	}

	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	stats := ComputeWindowStats(idx, q)
	indexedRunnable := threadDurationForPID(stats.RunnableTop, 42)
	if indexedRunnable == nil || !near(indexedRunnable.DurationMs, streamRunnable.DurationMs, 0.001) {
		t.Fatalf("streaming/indexed preempted-carry runnable parity broken: indexed=%+v stream=%.3fms", indexedRunnable, streamRunnable.DurationMs)
	}
}

// TestStreamStateClusterSelectorRejectionPublishesUnknownHeadCoverage (audit
// #49): the ambiguous/unresolved thread-selector rejection arms clear every
// bucket AND the missingHeadThreads set — the typed SchedulerHeadCoverage
// face must publish unknown with the matching typed reason instead of
// letting the emptied set fall through to "recovered" and contradict the
// rejection caveat on the same result.
func TestStreamStateClusterSelectorRejectionPublishesUnknownHeadCoverage(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "selector_rejection_coverage.systrace",
		"app-10 ( 100) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=20",
		"app-20 ( 100) [001] .... 1.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20",
		"app-10 ( 100) [000] .... 1.050000: sched_switch: prev_comm=app prev_pid=10 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120",
		"app-20 ( 100) [001] .... 1.060000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120",
	)

	ambiguous, err := StreamStateCluster(context.Background(), path, Query{Thread: "app", ThreadInput: "app", TimeStart: 1, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.WindowStats == nil || ambiguous.WindowStats.SchedulerHeadCoverage == nil {
		t.Fatalf("head coverage face missing on the ambiguous rejection: %+v", ambiguous.WindowStats)
	}
	if got := ambiguous.WindowStats.SchedulerHeadCoverage; got.Status != "unknown" ||
		got.Reason != "thread_selector_ambiguous" ||
		got.SubjectCensusStatus != "not_evaluated" {
		t.Fatalf("ambiguous selector rejection must publish unknown/thread_selector_ambiguous, got %+v", got)
	}
	if !containsSubstring(ambiguous.Caveats, "thread_selector_ambiguous") {
		t.Fatalf("rejection caveat must stay alongside the typed status: %+v", ambiguous.Caveats)
	}

	unresolved, err := StreamStateCluster(context.Background(), path, Query{Thread: "nosuchthread", ThreadInput: "nosuchthread", TimeStart: 1, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if unresolved.WindowStats == nil || unresolved.WindowStats.SchedulerHeadCoverage == nil {
		t.Fatalf("head coverage face missing on the unresolved rejection: %+v", unresolved.WindowStats)
	}
	if got := unresolved.WindowStats.SchedulerHeadCoverage; got.Status != "unknown" ||
		got.Reason != "thread_selector_unresolved" ||
		got.SubjectCensusStatus != "not_evaluated" {
		t.Fatalf("unresolved selector rejection must publish unknown/thread_selector_unresolved, got %+v", got)
	}

	// Control: an exact pid selector is NOT a rejection — the coverage face
	// keeps its ordinary computation (here: recovered, since pid=10's head
	// state is established by its own in-window open and no missing subject
	// survives the pid filter).
	exact, err := StreamStateCluster(context.Background(), path, Query{PID: 10, TimeStart: 1, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if exact.WindowStats == nil || exact.WindowStats.SchedulerHeadCoverage == nil {
		t.Fatalf("head coverage face missing on the exact-pid control: %+v", exact.WindowStats)
	}
	if got := exact.WindowStats.SchedulerHeadCoverage; got.Status == "unknown" ||
		got.SubjectCensusStatus != "evaluated" {
		t.Fatalf("exact pid selection must not be reported as a selector rejection: %+v", got)
	}
}

// TestStreamEventSearchLineWindowDominatesTimeWindowMatchingIndexedLane
// (audit #50): the indexed lane's eventInQueryWindow convention — "time
// bounds apply only when no line bounds are set" — must govern the streaming
// lane too. With BOTH windows set, the streaming scan used to return the
// line∩time intersection while its zero-match fallback (the indexed path)
// returned line-only rows: same call, flipped window semantics. Both lanes
// must return the identical line-window row set, including rows whose
// timestamps sit outside the time window.
func TestStreamEventSearchLineWindowDominatesTimeWindowMatchingIndexedLane(t *testing.T) {
	lines := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		lines = append(lines, fmt.Sprintf("  worker-4%d (4%d) [000] .... %.6f: sched_wakeup: comm=peer pid=9%d prio=120 target_cpu=000", i, i, 1.0+float64(i)*0.1, i))
	}
	path := writeWindowSweepFixture(t, "line_time_mixed.systrace", lines)

	q := Query{
		View:      "event_search",
		Pattern:   "sched_wakeup",
		LineStart: 3,
		LineEnd:   6,
		TimeStart: 1.35,
		TimeEnd:   1.45,
		Limit:     40,
	}
	resetAnchorCaches()
	stream, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	indexed := Run(idx, q)

	lineSet := func(events []EventView) []int {
		out := make([]int, 0, len(events))
		for _, ev := range events {
			out = append(out, ev.Line)
		}
		return out
	}
	streamLines, indexedLines := lineSet(stream.Events), lineSet(indexed.Events)
	if fmt.Sprint(streamLines) != fmt.Sprint(indexedLines) {
		t.Fatalf("streaming/indexed mixed-window row sets diverged: stream=%v indexed=%v", streamLines, indexedLines)
	}
	// The line window (3..6 → ts 1.2..1.5) contains rows both below TimeStart
	// (1.2, 1.3) and at/above the window edge — line dominance means they ALL
	// return. Pin the two halves separately so a partial (one-sided) time
	// gate cannot pass.
	if len(streamLines) != 4 {
		t.Fatalf("line window 3..6 must return all 4 rows regardless of the time window, got %v", streamLines)
	}
	var sawBelowTimeStart, sawAboveTimeEnd bool
	for _, ev := range stream.Events {
		if ev.Ts < q.TimeStart {
			sawBelowTimeStart = true
		}
		if ev.Ts > q.TimeEnd {
			sawAboveTimeEnd = true
		}
	}
	if !sawBelowTimeStart || !sawAboveTimeEnd {
		t.Fatalf("line-dominant semantics must return rows outside the time window on both sides: below=%v above=%v events=%v",
			sawBelowTimeStart, sawAboveTimeEnd, streamLines)
	}
}

// TestStreamEventSearchSeekReportsActualScannedLines (audit #51): after an
// anchor seek the "scanned N line(s)" caveat and ScannedLineCount must count
// the lines actually read, not the absolute line number — the skipped prefix
// was never scanned, and counting it dilutes the unparsed-ratio denominator.
func TestStreamEventSearchSeekReportsActualScannedLines(t *testing.T) {
	n := 3 * traceAnchorLineInterval
	path := anchorTestTrace(t, n, 0)
	q := Query{
		View:      "event_search",
		Pattern:   "sched_wakeup",
		TimeStart: 100.0 + float64(2*traceAnchorLineInterval)*0.0001,
		TimeEnd:   100.0 + float64(2*traceAnchorLineInterval+2000)*0.0001,
		Limit:     10,
	}
	resetAnchorCaches()
	cold, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}

	canonical := canonicalTraceIndexPath(path)
	info, err := os.Stat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	set := anchorCache.load(traceAnchorKeyForInfo(canonical, info))
	if set == nil || len(set.Anchors) == 0 || !set.FlavorSet {
		t.Fatalf("cold stream must record anchors + flavor, got %+v", set)
	}
	anchor, ok := set.seekAnchorFor(true, q.TimeStart, 0)
	if !ok || anchor.LineNo <= 0 {
		t.Fatalf("expected a usable seek anchor for the warm run, got %+v ok=%v", anchor, ok)
	}

	// Deterministic anchor consumption (ENG review handoff): the warm scan
	// must consume THIS test's cold-run anchors regardless of any shared
	// anchor-cache state drift in between — re-stat and re-store the captured
	// snapshot under the freshly computed key immediately before the warm
	// call, then require the seek to have actually engaged before checking
	// the exact arithmetic.
	warmInfo, err := os.Stat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	anchorCache.store(traceAnchorKeyForInfo(canonical, warmInfo), set)
	warm, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if warm.ScannedLineCount >= warm.LineCount {
		t.Fatalf("warm scan did not consume the recorded anchor (no seek engaged): scanned=%d line_count=%d anchors=%d", warm.ScannedLineCount, warm.LineCount, len(set.Anchors))
	}
	wantScanned := warm.LineCount - anchor.LineNo
	if warm.ScannedLineCount != wantScanned {
		t.Fatalf("seeked scan must report the actual scanned volume (LineCount %d - anchor line %d = %d), got %d",
			warm.LineCount, anchor.LineNo, wantScanned, warm.ScannedLineCount)
	}
	if warm.ScannedLineCount >= cold.ScannedLineCount {
		t.Fatalf("warm seek must scan fewer lines than the cold run: warm=%d cold=%d", warm.ScannedLineCount, cold.ScannedLineCount)
	}
	if !containsSubstring(warm.Caveats, fmt.Sprintf("scanned %d line(s)", warm.ScannedLineCount)) {
		t.Fatalf("scanned caveat must quote the actual scanned volume %d: %+v", warm.ScannedLineCount, warm.Caveats)
	}
}

// TestStreamWindowSweepSeekReportsActualScannedLines (audit #51 twin): the
// window_sweep streaming lane shares the seek and must share the
// actual-scan-count semantics.
func TestStreamWindowSweepSeekReportsActualScannedLines(t *testing.T) {
	n := 3 * traceAnchorLineInterval
	path := anchorTestTrace(t, n, 0)
	q := Query{
		TimeStart:    100.0 + float64(2*traceAnchorLineInterval)*0.0001,
		TimeEnd:      100.0 + float64(2*traceAnchorLineInterval)*0.0001 + 0.2,
		TimeStartSet: true,
		TimeEndSet:   true,
		BucketMs:     100,
	}
	resetAnchorCaches()
	if _, err := StreamWindowSweep(context.Background(), path, q); err != nil {
		t.Fatal(err)
	}
	canonical := canonicalTraceIndexPath(path)
	info, err := os.Stat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	set := anchorCache.load(traceAnchorKeyForInfo(canonical, info))
	if set == nil || !set.FlavorSet {
		t.Fatalf("cold sweep must record anchors + flavor, got %+v", set)
	}
	anchor, ok := set.seekAnchorFor(true, q.TimeStart, 0)
	if !ok || anchor.LineNo <= 0 {
		t.Fatalf("expected a usable seek anchor for the warm sweep, got %+v ok=%v", anchor, ok)
	}
	// Deterministic anchor consumption — same isolation as the event_search
	// twin above: re-store the cold snapshot under a fresh key right before
	// the warm sweep and require the seek to have engaged.
	warmInfo, err := os.Stat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	anchorCache.store(traceAnchorKeyForInfo(canonical, warmInfo), set)
	warm, err := StreamWindowSweep(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if warm.ScannedLineCount >= warm.LineCount {
		t.Fatalf("warm sweep did not consume the recorded anchor (no seek engaged): scanned=%d line_count=%d anchors=%d", warm.ScannedLineCount, warm.LineCount, len(set.Anchors))
	}
	wantScanned := warm.LineCount - anchor.LineNo
	if warm.ScannedLineCount != wantScanned {
		t.Fatalf("seeked sweep must report the actual scanned volume (LineCount %d - anchor line %d = %d), got %d",
			warm.LineCount, anchor.LineNo, wantScanned, warm.ScannedLineCount)
	}
	if !containsSubstring(warm.Caveats, fmt.Sprintf("scanned %d line(s)", warm.ScannedLineCount)) {
		t.Fatalf("sweep scanned caveat must quote the actual scanned volume %d: %+v", warm.ScannedLineCount, warm.Caveats)
	}
}

// TestStreamStateClusterDisclosesUnparsedLinesLikeSiblingLanes (audit #52):
// StreamStateCluster is the index_event_limit fallback — the face serving
// the densest/most degraded traces — yet it was the only lane that neither
// counted unparseable rows nor published the coverage caveat. It must now
// disclose parse quality with the exact sibling-lane wording and fields.
func TestStreamStateClusterDisclosesUnparsedLinesLikeSiblingLanes(t *testing.T) {
	lines := []string{
		" worker-42 (42) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=120",
	}
	for i := 0; i < 7; i++ {
		lines = append(lines, fmt.Sprintf("totally degraded line %d that matches no known trace format", i))
	}
	lines = append(lines, " worker-42 (42) [000] .... 1.050000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120")
	path := writeSchedulerCarryTrace(t, "degraded_cluster.systrace", lines...)

	res, err := StreamStateCluster(context.Background(), path, Query{TimeStart: 1.0, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if res.UnparsedLineCount != 7 {
		t.Fatalf("unparsed in-window lines must be counted (want 7), got %d", res.UnparsedLineCount)
	}
	if !containsSubstring(res.Caveats, "7 of 10 scanned lines did not match any known trace format; coverage may be incomplete") {
		t.Fatalf("majority-unparsed window must carry the sibling-lane coverage caveat: %+v", res.Caveats)
	}
	// The parseable scheduler rows still produce the running interval — the
	// disclosure is additive, never a new fail-close.
	if res.WindowStats == nil || threadDurationForPID(res.WindowStats.TopRunning, 42) == nil {
		t.Fatalf("disclosure must not suppress the parsed scheduler intervals: %+v", res.WindowStats)
	}
}

// TestStreamStateClusterPreWindowCarryKeepsCheapSkipForUntimestampedLines
// (audit #53): a pre-window carry row must not arm the seen-window latch —
// otherwise every later pre-window no-timestamp line (comments,
// continuations, garbage) loses the cheap skip path and runs through the
// full parser. Observable via the audit-#52 counter: pre-window garbage
// after a carry row must NOT be counted as unparsed in-window lines.
func TestStreamStateClusterPreWindowCarryKeepsCheapSkipForUntimestampedLines(t *testing.T) {
	lines := []string{
		" worker-42 (42) [000] .... 0.100000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120",
	}
	for i := 0; i < 3; i++ {
		lines = append(lines, fmt.Sprintf("pre-window continuation garbage %d without any timestamp", i))
	}
	lines = append(lines,
		" other-7 (7) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=7 next_prio=120",
		" other-7 (7) [000] .... 1.050000: sched_switch: prev_comm=other prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120",
	)
	path := writeSchedulerCarryTrace(t, "carry_cheap_skip.systrace", lines...)

	res, err := StreamStateCluster(context.Background(), path, Query{TimeStart: 1.0, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if res.UnparsedLineCount != 0 {
		t.Fatalf("pre-window no-timestamp lines after a carry row must keep the cheap skip path (0 unparsed), got %d", res.UnparsedLineCount)
	}
	// The carry itself must still be consumed (sleep first segment) and the
	// in-window rows still account.
	if res.WindowStats == nil || threadDurationForPID(res.WindowStats.SleepTop, 42) == nil {
		t.Fatalf("carry consumption must survive the cheap-skip fix: %+v", res.WindowStats)
	}
	if strings.TrimSpace(path) == "" {
		t.Fatal("unreachable")
	}
}

// TestWindowSweepSuggestionsCarryClockRegressionSignal (audit #55, engine
// half): the hotspot suggestion surface must consume the same typed
// ClockRegressions signal the result's own caveat is built from. On a
// regressed trace the duration views (window_stats/frame_window/wakeup_chain)
// fail closed by design, so suggesting them burns a guaranteed dead round —
// the regressed arm steers to count/list-shaped views and says why, while
// counts and ranking stay byte-identical (soft-guidance-only red line).
func TestWindowSweepSuggestionsCarryClockRegressionSignal(t *testing.T) {
	buckets := map[int64]*WindowSweepBucketCounts{
		10: {SchedSwitches: 50, SchedWakeups: 40, DStateEntries: 6},
		11: {SchedSwitches: 20, SchedWakeups: 1},
	}
	clean := buildWindowSweepResult(Query{}, buckets, 100, 0, 8, 0, false)
	regressed := buildWindowSweepResult(Query{}, buckets, 100, 0, 8, 0, true)

	if len(clean.Hotspots) == 0 || len(regressed.Hotspots) == 0 {
		t.Fatalf("both arms must rank hotspots: clean=%d regressed=%d", len(clean.Hotspots), len(regressed.Hotspots))
	}
	joinViews := func(h WindowSweepHotspot) string { return strings.Join(h.SuggestedViews, "/") }
	if !strings.Contains(joinViews(clean.Hotspots[0]), "window_stats") || !strings.Contains(joinViews(clean.Hotspots[0]), "frame_window") {
		t.Fatalf("clean trace keeps the duration-view suggestions: %v", clean.Hotspots[0].SuggestedViews)
	}
	for _, view := range regressed.Hotspots[0].SuggestedViews {
		switch view {
		case "window_stats", "frame_window", "wakeup_chain":
			t.Fatalf("regressed trace must not suggest duration views that will fail closed: %v", regressed.Hotspots[0].SuggestedViews)
		}
	}
	if joinViews(regressed.Hotspots[0]) == "" || !strings.Contains(joinViews(regressed.Hotspots[0]), "event_search") {
		t.Fatalf("regressed arm must still suggest actionable listing views: %v", regressed.Hotspots[0].SuggestedViews)
	}
	if !strings.Contains(joinViews(regressed.Hotspots[0]), "critical_blocking_calls") {
		t.Fatalf("above-average D-state density keeps the blocking-calls listing on the regressed arm: %v", regressed.Hotspots[0].SuggestedViews)
	}
	if !strings.Contains(regressed.Hotspots[0].Summary, "timestamp regressions") {
		t.Fatalf("regressed hotspot summary must say WHY the duration views are excluded: %q", regressed.Hotspots[0].Summary)
	}
	if !containsSubstring(regressed.Caveats, "timestamp regressions") {
		t.Fatalf("regressed sweep must carry the suggestion-swap caveat: %+v", regressed.Caveats)
	}
	// Soft-guidance-only: the typed signal must never move counts or ranking.
	for i := range clean.Hotspots {
		c, r := clean.Hotspots[i], regressed.Hotspots[i]
		if c.Rank != r.Rank || c.StartTs != r.StartTs || c.WindowSweepBucketCounts != r.WindowSweepBucketCounts {
			t.Fatalf("regression signal must not reshape counts/ranking: clean=%+v regressed=%+v", c, r)
		}
	}
}

// TestStreamWindowSweepRegressedTraceSwapsSuggestionsEndToEnd (audit #55):
// the ClockRegressions signal must reach the suggestion surface through the
// real streaming scan, not just the pure builder.
func TestStreamWindowSweepRegressedTraceSwapsSuggestionsEndToEnd(t *testing.T) {
	path := writeWindowSweepFixture(t, "sweep_regressed.systrace", []string{
		sweepSwitchLine(2.000, "appa", 21, "S", "appb", 22),
		sweepSwitchLine(2.010, "appb", 22, "S", "appa", 21),
		sweepSwitchLine(1.500, "appa", 21, "S", "appb", 22),
		sweepSwitchLine(2.020, "appb", 22, "S", "appa", 21),
	})
	resetAnchorCaches()
	res, err := StreamWindowSweep(context.Background(), path, Query{BucketMs: 100})
	if err != nil {
		t.Fatal(err)
	}
	if res.ClockRegressions == 0 {
		t.Fatalf("fixture must carry a timestamp regression, got %+v", res.ClockRegressions)
	}
	if res.WindowSweep == nil || len(res.WindowSweep.Hotspots) == 0 {
		t.Fatalf("sweep must still rank hotspots on a regressed trace: %+v", res.WindowSweep)
	}
	for _, hotspot := range res.WindowSweep.Hotspots {
		for _, view := range hotspot.SuggestedViews {
			switch view {
			case "window_stats", "frame_window", "wakeup_chain":
				t.Fatalf("regressed end-to-end sweep must not suggest duration views: %+v", hotspot.SuggestedViews)
			}
		}
	}
}
