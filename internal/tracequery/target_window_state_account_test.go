package tracequery

// target_window_state_account_test.go — §29.27② COV-4 engine pins (ledger
// docs/design/real_trace_campaign_20260705.md, user ruling 2026-07-11): the
// focused thread's full-window state account — five window-clamped lanes with
// TotalMs = Σ(lanes), and the deterministic-running lane as the EXACT
// union(target semantic-span intervals ∩ target running intervals) through
// the shared foldInterval algebra (§29.26③-1 交集机械复用). Wall clock only —
// a converted value can never enter these lanes.

import (
	"math"
	"strings"
	"testing"
)

func cov4Timeline(intervals []Interval) TimelineResult {
	return TimelineResult{
		Thread:    ThreadRef{Comm: "ui", PID: 61},
		Window:    TimeWindow{StartTs: 100.0, EndTs: 100.1},
		Intervals: intervals,
	}
}

func cov4Interval(state ThreadState, start, end float64) Interval {
	return Interval{
		Thread: ThreadRef{Comm: "ui", PID: 61}, State: state,
		StartTs: start, EndTs: end, DurationMs: (end - start) * 1000,
		StartLine: 1, EndLine: 2,
	}
}

func TestTargetWindowStateAccountPartitionLanes(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	tl := cov4Timeline([]Interval{
		cov4Interval(StateRunning, 100.000, 100.060),
		cov4Interval(StateRunnable, 100.060, 100.070),
		cov4Interval(StateSSleep, 100.070, 100.090),
		cov4Interval(StateDSleep, 100.090, 100.095),
		cov4Interval(StateIOWait, 100.095, 100.100),
	})
	account := buildTargetWindowStateAccount(nil, tl, true, tl.Thread, window, nil)
	if account == nil {
		t.Fatalf("a measurable timeline must build an account")
	}
	if math.Abs(account.RunningMs-60) > 1e-6 || math.Abs(account.RunnableMs-10) > 1e-6 ||
		math.Abs(account.SleepMs-20) > 1e-6 || math.Abs(account.DStateMs-5) > 1e-6 ||
		math.Abs(account.IOWaitMs-5) > 1e-6 {
		t.Fatalf("per-lane totals drifted: %+v", account)
	}
	if math.Abs(account.TotalMs-(account.RunningMs+account.RunnableMs+account.SleepMs+account.DStateMs+account.IOWaitMs)) > 1e-6 {
		t.Fatalf("TotalMs must be the Σ of the five lanes: %+v", account)
	}
	if math.Abs(account.WindowMs-100) > 1e-6 {
		t.Fatalf("WindowMs must be the window length: %+v", account)
	}
	if account.WaitOccurrenceStatus != "complete" ||
		account.WaitOccurrenceTotal != 2 ||
		account.WaitOccurrenceEmitted != 2 ||
		len(account.WaitOccurrences) != 2 {
		t.Fatalf("D/io wait intervals must publish as a complete occurrence roster: %+v", account)
	}
	// No stats → no deterministic-running claim (absence never guesses).
	if account.DeterministicRunningMs != 0 {
		t.Fatalf("without a span population the deterministic lane must stay 0: %+v", account)
	}
	// ok=false (no target / no window) builds nothing.
	if got := buildTargetWindowStateAccount(nil, TimelineResult{}, false, tl.Thread, window, nil); got != nil {
		t.Fatalf("an absent timeline must build no account: %+v", got)
	}
}

func TestTargetWindowStateAccountPublishesCompletePerCPURunningRoster(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	intervals := []Interval{
		cov4Interval(StateRunning, 100.000, 100.020),
		cov4Interval(StateRunning, 100.020, 100.050),
		cov4Interval(StateRunning, 100.050, 100.060),
		cov4Interval(StateRunnable, 100.060, 100.100),
	}
	intervals[0].CPU, intervals[0].CPUKnown = 12, true
	intervals[1].CPU, intervals[1].CPUKnown = 4, true
	intervals[2].CPU, intervals[2].CPUKnown = 12, true
	account := buildTargetWindowStateAccount(nil, cov4Timeline(intervals), true, ThreadRef{Comm: "ui", PID: 61}, window, nil)
	if account == nil {
		t.Fatal("a measurable timeline must build an account")
	}
	if account.RunningCPURosterStatus != "complete" ||
		account.RunningCPUAssignmentStatus != "complete" ||
		account.RunningCPURosterTotal != 2 || account.RunningCPURosterEmitted != 2 ||
		len(account.RunningByCPU) != 2 {
		t.Fatalf("target per-CPU roster completeness drifted: %+v", account)
	}
	if account.RunningByCPU[0].CPU != 4 || math.Abs(account.RunningByCPU[0].RunningMs-30) > 1e-6 ||
		account.RunningByCPU[1].CPU != 12 || math.Abs(account.RunningByCPU[1].RunningMs-30) > 1e-6 ||
		account.RunningByCPU[1].SegmentCount != 2 {
		t.Fatalf("per-CPU aggregation must be exact and CPU ordered: %+v", account.RunningByCPU)
	}
	var rosterSum float64
	for _, row := range account.RunningByCPU {
		rosterSum += row.RunningMs
	}
	if math.Abs(rosterSum-account.RunningMs) > 1e-6 ||
		math.Abs(account.RunningCPUKnownMs-account.RunningMs) > 1e-6 ||
		account.RunningCPUUnknownMs != 0 || account.RunningCPUOverflowMs != 0 {
		t.Fatalf("complete roster must conserve the target running account: %+v", account)
	}
}

func TestTargetWindowStateAccountKeepsUnknownCPUTimeExplicit(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	known := cov4Interval(StateRunning, 100.000, 100.030)
	known.CPU, known.CPUKnown = 3, true
	unknown := cov4Interval(StateRunning, 100.030, 100.050)
	// A malformed/legacy negative CPU cannot become a real roster member even
	// if a producer accidentally marked the field known.
	unknown.CPU, unknown.CPUKnown = -1, true
	account := buildTargetWindowStateAccount(nil, cov4Timeline([]Interval{known, unknown}), true, ThreadRef{Comm: "ui", PID: 61}, window, nil)
	if account == nil || account.RunningCPUAssignmentStatus != "partial" ||
		account.RunningCPURosterStatus != "complete" || len(account.RunningByCPU) != 1 ||
		math.Abs(account.RunningCPUKnownMs-30) > 1e-6 || math.Abs(account.RunningCPUUnknownMs-20) > 1e-6 {
		t.Fatalf("unknown CPU running time must stay typed and must not become CPU 0: %+v", account)
	}
	if account.RunningByCPU[0].CPU != 3 {
		t.Fatalf("unknown CPU interval fabricated a roster row: %+v", account.RunningByCPU)
	}
}

func TestTargetWindowWaitOccurrencesAreChronologicalAndBounded(t *testing.T) {
	var intervals []Interval
	for i := targetWindowWaitOccurrenceCap; i >= 0; i-- {
		start := 100.0 + float64(i)*0.001
		intervals = append(intervals, Interval{
			Thread: ThreadRef{Comm: "ui", PID: 61},
			State:  StateIOWait, StartTs: start, EndTs: start + 0.0005, DurationMs: 0.5,
			StartLine: i + 1, EndLine: i + 2,
			BlockedReasonCaller: "wait_fn", BlockedReasonLine: 1000 + i,
			BlockedReasonIOWait: 1, BlockedReasonIOWaitKnown: true,
		})
	}
	account := buildTargetWindowStateAccount(
		nil,
		cov4Timeline(intervals),
		true,
		ThreadRef{Comm: "ui", PID: 61},
		TimeWindow{StartTs: 100.0, EndTs: 100.1},
		nil,
	)
	if account == nil {
		t.Fatal("bounded occurrence fixture must build an account")
	}
	if account.WaitOccurrenceStatus != "incomplete" ||
		account.WaitOccurrenceTotal != targetWindowWaitOccurrenceCap+1 ||
		account.WaitOccurrenceEmitted != targetWindowWaitOccurrenceCap ||
		len(account.WaitOccurrences) != targetWindowWaitOccurrenceCap {
		t.Fatalf("occurrence cap disclosure drifted: %+v", account)
	}
	for i, occurrence := range account.WaitOccurrences {
		if occurrence.Ordinal != i+1 {
			t.Fatalf("occurrence ordinal %d = %d", i, occurrence.Ordinal)
		}
		if i > 0 && occurrence.StartTs < account.WaitOccurrences[i-1].StartTs {
			t.Fatalf("occurrence roster is not chronological: %+v", account.WaitOccurrences)
		}
	}
}

func TestTargetSemanticRunningIntersection(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	target := ThreadRef{Comm: "ui", PID: 61}
	running := []foldInterval{{start: 100.000, end: 100.040}, {start: 100.060, end: 100.080}}
	stats := &WindowStats{TraceSpans: []TraceSpanSummary{
		// Target semantic span overlapping BOTH running intervals and the
		// sleep gap between them: only the running overlap counts
		// (0.030..0.040 = 10ms) + (0.060..0.065 = 5ms) = 15ms.
		{Thread: target, Name: "VerifyClass Foo", SemanticClass: "class_verification",
			StartTs: 100.030, EndTs: 100.065},
		// Same-thread NON-semantic span: never counted.
		{Thread: target, Name: "arbitrary business span", StartTs: 100.000, EndTs: 100.040},
		// OTHER thread's semantic span: never counted.
		{Thread: ThreadRef{Comm: "render", PID: 62}, Name: "Texture upload 64x64",
			SemanticClass: "texture_upload", StartTs: 100.000, EndTs: 100.040},
	}}
	got := targetSemanticRunningMs(stats, target, window, running)
	if math.Abs(got-15) > 1e-6 {
		t.Fatalf("deterministic running must be the exact semantic∩running union: got %.6f want 15", got)
	}
	// Overlapping semantic members count at most once (union algebra).
	stats.TraceSpans = append(stats.TraceSpans, TraceSpanSummary{
		Thread: target, Name: "VerifyClass Bar", SemanticClass: "class_verification",
		StartTs: 100.030, EndTs: 100.040,
	})
	if got := targetSemanticRunningMs(stats, target, window, running); math.Abs(got-15) > 1e-6 {
		t.Fatalf("overlapping members must union, never double-count: got %.6f want 15", got)
	}
	// No running intervals → no claim.
	if got := targetSemanticRunningMs(stats, target, window, nil); got != 0 {
		t.Fatalf("without running intervals the lane must stay 0: got %.6f", got)
	}
}

// TestCov4SleepIOWaitRefinementG12Platform — 复核 A-1 (修前红): on the G12
// §29.13 Harmony platform form (S-opened sleeps + iowait=1 blocked_reason
// markers, the campaign's dominant IO-wait shape) the account's sleep-side IO
// refinement books the paired S wall clock, while G12 single attribution
// stays untouched: the io_wait LANE stays 0 (S never reclassifies), TotalMs
// still excludes the refinement (Σ five lanes only), and DStateTop/IOWaitTop
// book exactly what they booked before (发布面零变负对照).
func TestCov4SleepIOWaitRefinementG12Platform(t *testing.T) {
	idx := g12PlatformTrace(t)
	q := g12PlatformQuery()
	hmfs := ThreadRef{Comm: "hmfs_discard-26", PID: 562}
	window := TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}
	tl, ok := targetWindowTimeline(idx, q, hmfs, window)
	if !ok {
		t.Fatalf("the g12 fixture must produce a timeline for hmfs_discard")
	}
	account := buildTargetWindowStateAccount(idx, tl, ok, hmfs, window, nil)
	if account == nil {
		t.Fatalf("the g12 fixture must build an account for hmfs_discard")
	}
	// The two in-window S sleeps whose wakeups paired iowait=1 markers:
	// .930317→.931770 (1.453ms) and .931802→.938268 (6.466ms) ≈ 7.919ms.
	if account.SleepIOWaitMs <= 0 {
		t.Fatalf("S+iowait sleeps must book into the SleepIOWaitMs refinement (修前红): %+v", account)
	}
	if account.SleepIOWaitMs > account.SleepMs+0.001 {
		t.Fatalf("the refinement is INSIDE the sleep lane, never beyond it: %+v", account)
	}
	if math.Abs(account.SleepIOWaitMs-7.919) > 0.05 {
		t.Fatalf("the refinement books exactly the paired S intervals (~7.919ms): got %.3f", account.SleepIOWaitMs)
	}
	// G12 single attribution: the S form never reclassifies — the io_wait
	// LANE stays zero and TotalMs is the Σ of the five lanes only (the
	// refinement never adds).
	if account.IOWaitMs != 0 {
		t.Fatalf("S+iowait must never enter the io_wait lane (G12): %+v", account)
	}
	lanes := account.RunningMs + account.RunnableMs + account.SleepMs + account.DStateMs + account.IOWaitMs
	if math.Abs(account.TotalMs-lanes) > 1e-6 {
		t.Fatalf("TotalMs must exclude the refinement (Σ five lanes only): %+v", account)
	}
	// 发布面零变负对照: DStateTop/IOWaitTop still never book the S+iowait
	// thread (the same G12 pin, re-asserted against THIS batch's change).
	stats := ComputeWindowStats(idx, q)
	for _, td := range stats.DStateTop {
		if td.Thread.PID == 562 {
			t.Fatalf("DStateTop must stay unchanged (G12 zero-change): %+v", td)
		}
	}
	for _, td := range stats.IOWaitTop {
		if td.Thread.PID == 562 {
			t.Fatalf("IOWaitTop must stay unchanged (G12 zero-change): %+v", td)
		}
	}
}

// §29.27② 常态发布 pin (SMR-1 修复轮 引擎件①, 2026-07-13; 冷读 F-0 放大器:
// the 40422 non-bundle run had no four-state account and the prose「全程
// s_sleep」inversion sailed past): a target-anchored bounded-window
// root_cause_rank run publishes Result.TargetWindowStates.
func TestTargetWindowStatesPublishesOnRankView(t *testing.T) {
	var b strings.Builder
	b.WriteString("        app-100 (100) [001] .... 9.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	b.WriteString("        app-100 (100) [001] .... 10.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n")
	b.WriteString("        dep-200 (100) [000] .... 10.079900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n")
	b.WriteString("        app-100 (100) [001] .... 10.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	b.WriteString("        app-100 (100) [001] .... 10.110000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n")
	idx := buildTraceIndex(t, "cov4_publish_rank.systrace", b.String())
	res := Run(idx, Query{View: "root_cause_rank", PID: 100, TimeStart: 10.0, TimeEnd: 10.1})
	if res.TargetWindowStates == nil {
		t.Fatalf("target-anchored rank run must publish the four-state account (常态发布)")
	}
	if res.TargetWindowStates.TotalMs <= 0 {
		t.Fatalf("account must carry the measured partition: %+v", res.TargetWindowStates)
	}
	// Non-target runs stay silent (absence never fabricates).
	if bare := Run(idx, Query{View: "window_stats", TimeStart: 10.0, TimeEnd: 10.1}); bare.TargetWindowStates != nil {
		t.Fatalf("a run without a target thread must not fabricate an account")
	}

	// A target-scoped window_stats query without explicit endpoints means the
	// immutable whole artifact.  The target account is cap-immune and must not
	// disappear merely because background threads outrank it in Top-N lists.
	whole := Run(idx, Query{View: "window_stats", PID: 100})
	if whole.TargetWindowStates == nil || whole.TargetWindowStates.Thread.PID != 100 {
		t.Fatalf("whole-artifact target stats must publish the target account: %+v", whole.TargetWindowStates)
	}
	if whole.TargetWindowStates.TotalMs <= 0 {
		t.Fatalf("whole-artifact target account must carry measured state time: %+v", whole.TargetWindowStates)
	}

	// An unbounded event cursor keeps the historical narrow result shape.
	if cursor := Run(idx, Query{View: "event_search", PID: 100, Pattern: "sched_switch"}); cursor.TargetWindowStates != nil {
		t.Fatalf("an event cursor must not silently materialize a full state account: %+v", cursor.TargetWindowStates)
	}
}
