package tracequery

// blocking_span_xerr1_test.go — XERR1-FIX engine pins (§29.104.3/.4/.5,
// 2026-07-15; local acceptance anchor §29.104.9 形④).
//
// Customer lesion (cust_err1): a payload-less blocking_span row published the
// traversal span's WINDOW-ENVELOPE projection (199.992ms = 100% of the
// analysis window) as 「阻塞等待(对端 RenderThread)」 while the same report's
// four-state account said running 54%. Local natural witness: tieba
// (donghu_tieba_frame.systrace) window 34579.540..580 tid 8091 — the span
// "H:Native a**sync** work complete…" mis-admitted by the free `sync`
// substring, envelope 29.843ms vs 修向真值 Σ(sleep+D+io)=6.637ms (4.5×).

import (
	"context"
	"os"
	"strings"
	"testing"
)

const xerr1TiebaFixture = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"

// TestBlockingLikeTextSyncWordBoundaryXERR1 — 件4 noise-screen purification:
// the `sync` token matches only at a word head. Both misfire families are
// pinned (customer "resynced", local "async"), and the regression arm keeps
// every genuine blocking shape admitted (ART/OHOS lock payloads, sync
// primitives, the other token families).
func TestBlockingLikeTextSyncWordBoundaryXERR1(t *testing.T) {
	rejected := []string{
		// 件4 misfire family ① (local tieba natural witness, §29.104.9 形④).
		"H:Native async work complete callback, name:FileIOStat#388929095296, traceid:0x0",
		// 件4 misfire family ② (customer cust_err1 span, §29.104.4 ①).
		"H:traversal resynced to 58563",
		// ALL-CAPS run is one word, never a camel head.
		"ASYNC_COMMIT flush",
		// Pre-existing vsync-cadence exemption stays (boundary-passing forms).
		"Choreographer#onVsync",
		"jank_event_sync",
		"H:ReceiveVsync name:rs_receiver",
	}
	for _, name := range rejected {
		if isBlockingLikeText(name) {
			t.Fatalf("件4: %q must NOT be admitted as blocking-like", name)
		}
	}
	admitted := []string{
		// ART/OHOS structured lock payloads (照 admit — lock/contention tokens).
		"Lock contention on a monitor lock (owner tid: 5)",
		"monitor contention with owner Worker (42067) at void j.l.Object.wait()(Object.java:100) waiters=1",
		// Genuine sync-primitive vocabulary at word heads.
		"SyncFence merge wait",
		"data_sync flush",
		"sync barrier",
		"FENCE_SYNC signal",
		// Trailing side deliberately free: synchronized/synchronization match.
		"monitor synchronized block",
		// Other token families untouched.
		"futex wait", "binder transaction", "mutex_lock slowpath", "semaphore down",
	}
	for _, name := range admitted {
		if !isBlockingLikeText(name) {
			t.Fatalf("件4 regression: %q must stay admitted as blocking-like", name)
		}
	}
}

// TestBlockingSpanTiebaAnchorConvergesXERR1 — the §29.104.9 形④ local
// acceptance anchor, 件1+件3+件4 on the REAL fixture. 件4 keeps the async
// span out of the end-to-end blocking lane; the convergence machinery,
// exercised on the hand-carved candidate of the same physical span, converges
// 29.843 → 6.637 = Σ(sleep 4.084 + D 2.553 + io 0.000) with the budget
// marker (non-running 16.572, running 13.271) — hand-recomputable from the
// raw trace.
func TestBlockingSpanTiebaAnchorConvergesXERR1(t *testing.T) {
	if _, err := os.Stat(xerr1TiebaFixture); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := BuildIndex(context.Background(), xerr1TiebaFixture)
	if err != nil {
		t.Fatal(err)
	}
	q := normalizeQuery(idx, Query{PID: 8091, TimeStart: 34579.540, TimeEnd: 34579.580, MinDurationMs: 0.001})
	stats := ComputeWindowStats(idx, q)
	var span *TraceSpanSummary
	for i := range stats.TraceSpans {
		if stats.TraceSpans[i].Thread.PID == 8091 && strings.Contains(stats.TraceSpans[i].Name, "Native async work complete") {
			span = &stats.TraceSpans[i]
			break
		}
	}
	if span == nil {
		t.Fatalf("fixture drifted: the async span is missing from the window")
	}
	if !near(span.DurationMs, 29.843, 0.001) {
		t.Fatalf("fixture drifted: span envelope %.3f, want 29.843", span.DurationMs)
	}
	// 件4 admit-face verdict: the span is NOT admitted (报告论证 branch 不 admit).
	if _, ok := blockingSpanCandidateFromTraceSpan(*span); ok {
		t.Fatalf("件4: the async span must not carve a blocking candidate")
	}
	rows := collectBlockingSpanRows(idx, q, stats)
	for _, row := range rows {
		if strings.Contains(row.spanName, "Native async") {
			t.Fatalf("件4: the async span leaked into the blocking-span lane: %+v", row.cand)
		}
	}
	// 件1 value convergence on the same physical span (hand-carved candidate —
	// the pre-件4 admit shape), against real trace truth.
	row := blockingSpanRow{
		cand: CriticalBlockingCandidate{
			Type: "blocking_span", Thread: span.Thread, DurationMs: span.DurationMs,
			StartTs: span.StartTs, EndTs: span.EndTs,
			LineStart: span.StartLine, LineEnd: span.EndLine, Confidence: 0.72,
			Summary: "blocking-like trace span lasted 29.843ms",
		},
		spanName: span.Name,
	}
	convergeBlockingSpanRowValue(idx, q, &row)
	cand := row.cand
	if cand.BlockingValueBasis != BlockingValueBasisWaitSegments {
		t.Fatalf("件1: basis must be wait_segments, got %q", cand.BlockingValueBasis)
	}
	if !near(cand.DurationMs, 6.637, 0.001) || !near(cand.WaitSegmentMs, 6.637, 0.001) {
		t.Fatalf("件1 anchor: value must converge 29.843→6.637, got dur=%.3f Σ=%.3f", cand.DurationMs, cand.WaitSegmentMs)
	}
	if !near(cand.WaitSleepMs, 4.084, 0.001) || !near(cand.WaitDStateMs, 2.553, 0.001) || cand.WaitIOWaitMs != 0 {
		t.Fatalf("件1 anchor: Σ decomposition drifted: sleep=%.3f d=%.3f io=%.3f", cand.WaitSleepMs, cand.WaitDStateMs, cand.WaitIOWaitMs)
	}
	if !near(cand.SpanEnvelopeMs, 29.843, 0.001) {
		t.Fatalf("件1: the envelope must be preserved on SpanEnvelopeMs, got %.3f", cand.SpanEnvelopeMs)
	}
	// Σ(wait segments) is a strict subset of non-running — the runnable share
	// (16.572−6.637=9.935) never enters blocking semantics.
	if !cand.WaitBudgetExceeded || !near(cand.WaitBudgetNonRunningMs, 16.572, 0.001) || !near(cand.WaitBudgetRunningMs, 13.271, 0.001) {
		t.Fatalf("件3 anchor: budget marker drifted: exceeded=%v nonRun=%.3f run=%.3f",
			cand.WaitBudgetExceeded, cand.WaitBudgetNonRunningMs, cand.WaitBudgetRunningMs)
	}
	// 件1 LLM-visible Summary 换径: the head speaks the converged value; the
	// envelope appears only as disclosure.
	if !strings.Contains(cand.Summary, "Σ(sleep+d_state+io_wait)=6.637ms") ||
		!strings.Contains(cand.Summary, "span envelope 29.843ms contains run time") {
		t.Fatalf("件1: Summary head must speak the converged value with the envelope demoted to disclosure: %q", cand.Summary)
	}
	if strings.Contains(cand.Summary, "lasted 29.843ms") {
		t.Fatalf("件1: the legacy envelope head must not survive convergence: %q", cand.Summary)
	}
}

// TestBlockingSpanSyntheticE1ConvergesEndToEndXERR1 — the customer E1
// synthetic twin (span = the WHOLE window, running 54%), end-to-end through
// buildCriticalBlockingCallsFromStats: the published row value is the
// non-running WAIT-segment total (runnable excluded), the envelope moves to
// disclosure, and the 件3 sanity marker rides the row (值+预算随行, no clamp,
// no reject — the row still publishes).
func TestBlockingSpanSyntheticE1ConvergesEndToEndXERR1(t *testing.T) {
	// app-100 over window 1.000..1.200 (200ms): running 108ms (54%),
	// sleep 60ms, runnable 20ms, D 12ms — a whole-window "traversal wait"
	// span dressed as blocking (admitted via the `wait` token).
	idx := buildTraceIndex(t, "xerr1_e1_shape.systrace", `
       app-100 (100) [001] .... 0.900000: print: B|100|H:TraversalWait frame
       idle-0 (0) [001] .... 0.950000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 1.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-300 (300) [000] .... 1.080000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
       idle-0 (0) [001] .... 1.100000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 1.188000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-300 (300) [000] .... 1.200000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
       app-100 (100) [001] .... 1.250000: print: E|100
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MinDurationMs: 0.001})
	stats := ComputeWindowStats(idx, q)
	res := buildCriticalBlockingCallsFromStats(idx, q, stats, nil)
	var row *CriticalBlockingCandidate
	for i := range res.Items {
		if res.Items[i].Type == "blocking_span" && res.Items[i].Thread.PID == 100 {
			row = &res.Items[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("fixture drifted: no blocking_span row minted: %+v", res.Items)
	}
	if row.BlockingValueBasis != BlockingValueBasisWaitSegments {
		t.Fatalf("件1: basis must be wait_segments, got %q", row.BlockingValueBasis)
	}
	// Timeline in-window: running 1.000-1.020 + 1.100-1.188 = 108ms (54%),
	// sleep 1.020-1.080 = 60ms, runnable 1.080-1.100 = 20ms, D 1.188-1.200 =
	// 12ms. Converged value = sleep+D = 72ms; runnable NEVER enters.
	if !near(row.DurationMs, 72, 0.01) || !near(row.WaitSleepMs, 60, 0.01) || !near(row.WaitDStateMs, 12, 0.01) {
		t.Fatalf("件1: converged value must be Σ(sleep+D+io)=72ms, got dur=%.3f sleep=%.3f d=%.3f io=%.3f",
			row.DurationMs, row.WaitSleepMs, row.WaitDStateMs, row.WaitIOWaitMs)
	}
	if !near(row.SpanEnvelopeMs, 200, 0.01) {
		t.Fatalf("件1: the whole-window envelope must be preserved, got %.3f", row.SpanEnvelopeMs)
	}
	// 件3: envelope 200 > non-running 92 (sleep 60 + runnable 20 + D 12),
	// running 108 — the marker rides with value AND budget, no clamp.
	if !row.WaitBudgetExceeded || !near(row.WaitBudgetNonRunningMs, 92, 0.01) || !near(row.WaitBudgetRunningMs, 108, 0.01) {
		t.Fatalf("件3: sanity marker must ride the row: exceeded=%v nonRun=%.3f run=%.3f",
			row.WaitBudgetExceeded, row.WaitBudgetNonRunningMs, row.WaitBudgetRunningMs)
	}
}

// TestBlockingSpanPayloadTypedConvergesXERR1EXT — XERR1-EXT 件1/件4①
// (§29.104.17 裁定⑤, user ruling 2026-07-16「同意,按推荐的来」): the
// cust_err1 复刻形 payload variant — a 200ms-envelope ART true-lock row now
// CONVERGES exactly like the payload-less lane (supersedes the XERR1-FIX
// 首批只披露不改值 pin; 改值通道且改榜序=用户已准). In-window timeline of the
// waiter: running 1.000-1.050 (50ms) + 1.160-1.200 (40ms) = 90ms, sleep
// 1.050-1.150 = 100ms, runnable 1.150-1.160 = 10ms — hand-recomputable.
func TestBlockingSpanPayloadTypedConvergesXERR1EXT(t *testing.T) {
	idx := buildTraceIndex(t, "xerr1_payload_typed.systrace", `
       idle-0 (0) [001] .... 0.950000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 1.000000: print: B|100|Lock contention on a monitor lock (owner tid: 200)
       app-100 (100) [001] .... 1.050000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      owner-200 (200) [000] .... 1.150000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
       idle-0 (0) [001] .... 1.160000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 1.200000: print: E|100
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MinDurationMs: 0.001})
	stats := ComputeWindowStats(idx, q)
	rows := collectBlockingSpanRows(idx, q, stats)
	var lockRow *CriticalBlockingCandidate
	for i := range rows {
		if rows[i].cand.BlockingKind != "" {
			lockRow = &rows[i].cand
			break
		}
	}
	if lockRow == nil {
		t.Fatalf("fixture drifted: no payload-typed contention row: %+v", rows)
	}
	if lockRow.BlockingValueBasis != BlockingValueBasisWaitSegments {
		t.Fatalf("EXT 件1: payload-typed basis must be wait_segments, got %q", lockRow.BlockingValueBasis)
	}
	// Converged value = Σ(sleep 100 + D 0 + io 0) = 100ms; runnable (10ms)
	// never enters blocking semantics; the 200ms whole-wait envelope moves to
	// SpanEnvelopeMs.
	if !near(lockRow.DurationMs, 100, 0.01) || !near(lockRow.WaitSegmentMs, 100, 0.01) ||
		!near(lockRow.WaitSleepMs, 100, 0.01) || lockRow.WaitDStateMs != 0 || lockRow.WaitIOWaitMs != 0 {
		t.Fatalf("EXT 件1: converged value must be Σ=100ms, got dur=%.3f Σ=%.3f sleep=%.3f d=%.3f io=%.3f",
			lockRow.DurationMs, lockRow.WaitSegmentMs, lockRow.WaitSleepMs, lockRow.WaitDStateMs, lockRow.WaitIOWaitMs)
	}
	if !near(lockRow.SpanEnvelopeMs, 200, 0.01) {
		t.Fatalf("EXT 件1: the whole-wait envelope must be preserved on SpanEnvelopeMs, got %.3f", lockRow.SpanEnvelopeMs)
	}
	// 件3 rides unchanged (envelope 200 > non-running 110, running 90).
	if !lockRow.WaitBudgetExceeded || !near(lockRow.WaitBudgetNonRunningMs, 110, 0.01) || !near(lockRow.WaitBudgetRunningMs, 90, 0.01) {
		t.Fatalf("件3: payload-typed budget marker drifted: exceeded=%v nonRun=%.3f run=%.3f",
			lockRow.WaitBudgetExceeded, lockRow.WaitBudgetNonRunningMs, lockRow.WaitBudgetRunningMs)
	}
	// Full coverage (90+100+10 = 200ms) → no 件F lower-bound marker.
	if lockRow.WaitCoveragePartial || lockRow.WaitAccountCoveredMs != 0 {
		t.Fatalf("件F 负向: full coverage must not mint the marker: %+v", lockRow)
	}
	// Summary 换径: the head speaks the converged value, the typed lock suffix
	// survives verbatim, the envelope demotes to disclosure and the legacy
	// "lasted <envelope>" head is gone.
	for _, want := range []string{
		"Σ(sleep+d_state+io_wait)=100.000ms",
		"span envelope 200.000ms contains run time",
		"blocking_kind=monitor_contention",
		"owner=pid=200",
		"exceeds the waiter's non-running total",
	} {
		if !strings.Contains(lockRow.Summary, want) {
			t.Fatalf("EXT 件1: Summary must carry %q: %q", want, lockRow.Summary)
		}
	}
	if strings.Contains(lockRow.Summary, "lasted 200.000ms") {
		t.Fatalf("EXT 件1: the legacy envelope head must not survive convergence: %q", lockRow.Summary)
	}
	// LOCKNS 面零触: the payload owner rides the holder lane untouched (value
	// lane ⊥ holder lane).
	if lockRow.Peer.PID != 200 {
		t.Fatalf("LOCKNS 零触: the payload owner tid must ride the peer lane untouched, got %+v", lockRow.Peer)
	}
}

// TestBlockingSpanBudgetForbidsPartialCoverageVerdictXERR1 — F-2 同基 gate:
// when the waiter's state account does NOT fully cover the span window
// (timeline coverage gap), the budget verdict is FORBIDDEN (禁判) — an
// undercounted budget must never false-fire the ⚠ marker.
func TestBlockingSpanBudgetForbidsPartialCoverageVerdictXERR1(t *testing.T) {
	// The thread's first event is at 1.150 — nothing tiles 1.000..1.150, so
	// the account covers only 50ms of the 200ms span window.
	idx := buildTraceIndex(t, "xerr1_partial_coverage.systrace", `
       app-100 (100) [001] .... 0.900000: print: B|100|H:TraversalWait frame
       app-100 (100) [001] .... 1.150000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-300 (300) [000] .... 1.190000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
       app-100 (100) [001] .... 1.250000: print: E|100
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MinDurationMs: 0.001})
	stats := ComputeWindowStats(idx, q)
	rows := collectBlockingSpanRows(idx, q, stats)
	for _, row := range rows {
		if row.cand.Type != "blocking_span" {
			continue
		}
		if row.cand.WaitBudgetExceeded {
			t.Fatalf("F-2 禁判: partial-coverage account must not mint the budget marker: %+v", row.cand)
		}
		return
	}
	t.Fatalf("fixture drifted: no blocking_span row: %+v", rows)
}

// TestRunnableWakeupDelaySegmentsMergeXERR1XCPU — the XCPU rider acceptance
// (§29.104.5 + §29.104.9 形④ 同窗): tieba R3 sentinel window
// (34579.490..500, target 59566). Raw-trace hand account: five wakeup-delay
// segments (23+14+12+38+34µs = 0.121ms) whose closing switch-ins all landed
// on cpu[001] while the wakeup rows printed target_cpu=000, plus one 0.062ms
// preemption segment on cpu[001]. The engine used to withdraw the five
// (cpu=unknown, sched_in_cpu_mismatch); they now stamp the switch-in CPU and
// the whole account merges into ONE (thread,cpu=1) member worth 0.183ms.
// 负向 pin: nothing may attribute to cpu 0 (the stale wakeup target_cpu).
func TestRunnableWakeupDelaySegmentsMergeXERR1XCPU(t *testing.T) {
	if _, err := os.Stat(xerr1TiebaFixture); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := BuildIndex(context.Background(), xerr1TiebaFixture)
	if err != nil {
		t.Fatal(err)
	}
	q := normalizeQuery(idx, Query{PID: 59566, TimeStart: 34579.490, TimeEnd: 34579.500, MinDurationMs: 0.001})
	stats := ComputeWindowStats(idx, q)
	var total float64
	migrated := 0
	byCPU := map[int]float64{}
	for _, seg := range stats.runnableSegments {
		if seg.thread.PID != 59566 {
			continue
		}
		total += seg.durationMs
		byCPU[seg.cpu] += seg.durationMs
		if !seg.cpuKnown {
			t.Fatalf("XCPU: an in-window sched_in-closed segment kept cpu=unknown: %+v", seg)
		}
		if seg.cpu == 0 {
			t.Fatalf("负向 pin: attribution took the stale wakeup target_cpu=0: %+v", seg)
		}
		if seg.cpuContinuity == RunnableCPUContinuitySchedInMigrated {
			migrated++
		}
	}
	if migrated != 5 {
		t.Fatalf("XCPU: the five wakeup-delay segments must carry sched_in_migrated, got %d", migrated)
	}
	if !near(total, 0.183, 0.0006) || !near(byCPU[1], 0.183, 0.0006) || len(byCPU) != 1 {
		t.Fatalf("XCPU: the account must merge into one cpu=1 member worth 0.183ms, got total=%.4f byCPU=%v", total, byCPU)
	}
	// The merged (thread,cpu) census bucket is the single E4 member the
	// display consumes (合并单成员形).
	buckets := 0
	for _, td := range stats.runnableCensus {
		if td.Thread.PID != 59566 {
			continue
		}
		buckets++
		if td.CPU != 1 || !near(td.DurationMs, 0.183, 0.0006) {
			t.Fatalf("XCPU: census bucket drifted: cpu=%d dur=%.4f", td.CPU, td.DurationMs)
		}
	}
	if buckets != 1 {
		t.Fatalf("XCPU: expected exactly one merged census bucket, got %d", buckets)
	}
	if c := stats.RunnableCPUContinuity; c != nil && c.MismatchSegments > 0 {
		t.Fatalf("XCPU: no withdrawn-mismatch census may remain on this window: %+v", c)
	}
}

// xerr1FixAFoldStats builds the 件A witness span pair (对抗复核 P1-1): the
// SAME waiter's dual print forms of ONE lock — the information-RICH form
// (owner comm + holder site, richness winner → fold survivor) over the
// NARROW interval 100.05..100.15 (envelope 100ms), and the tid-only form
// (richness 0) over the WIDE interval 100.0..100.2 (envelope 200ms, the
// take-MAX VALUE winner).
func xerr1FixAFoldStats() WindowStats {
	return WindowStats{TraceSpans: []TraceSpanSummary{
		{
			Name:    "monitor contention with owner #Worker (200) at void j.l.Object.wait()(Object.java:100) waiters=1",
			Thread:  ThreadRef{Comm: "app", PID: 100},
			StartTs: 100.05, EndTs: 100.15, DurationMs: 100,
			StartLine: 10, EndLine: 20,
		},
		{
			Name:    "Lock contention on a monitor lock (owner tid: 200)",
			Thread:  ThreadRef{Comm: "app", PID: 100},
			StartTs: 100.0, EndTs: 100.2, DurationMs: 200,
			StartLine: 12, EndLine: 22,
		},
	}}
}

// TestBlockingSpanFoldBudgetWindowsOnValueWinnerIntervalXERR1FixA — 件A
// (对抗复核 P1-1): the take-MAX fold puts the tid-only form's 200ms envelope
// on the rich survivor whose OWN interval is only 100ms wide — the 件3 budget
// must window the waiter's account on the VALUE-WINNING form's interval
// (100.0..100.2), never the survivor's. Witness: the waiter sleeps the whole
// 100.0..100.2 (zero running) — 200 ≤ 200 on the value-winner interval, so NO
// marker; the pre-件A survivor-interval budget minted the false
// ⚠ exceeded=true nonRun=100 run=0.000 (「含 running」 said of a waiter that
// never ran).
func TestBlockingSpanFoldBudgetWindowsOnValueWinnerIntervalXERR1FixA(t *testing.T) {
	idx := buildTraceIndex(t, "xerr1_fixa_sleeping_waiter.systrace", `
       app-100 (100) [001] .... 99.900000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-300 (300) [000] .... 100.300000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 100.0, TimeEnd: 100.2, MinDurationMs: 0.001})
	rows := collectBlockingSpanRows(idx, q, xerr1FixAFoldStats())
	if len(rows) != 1 {
		t.Fatalf("fixture drifted: the dual print forms must fold into ONE row, got %d: %+v", len(rows), rows)
	}
	cand := rows[0].cand
	if cand.HolderSite == "" {
		t.Fatalf("fixture drifted: the information-rich form must survive the fold: %+v", cand)
	}
	// XERR1-EXT 件1: the fully-sleeping waiter converges on the VALUE-WINNER
	// interval (100.0..100.2): Σ(sleep)=200ms == the take-MAX envelope, so the
	// published value is numerically unchanged while the basis turns typed.
	if !near(cand.DurationMs, 200, 0.001) {
		t.Fatalf("the converged value must be Σ(sleep)=200ms on the value-winner interval, got %.3f", cand.DurationMs)
	}
	if cand.BlockingValueBasis != BlockingValueBasisWaitSegments ||
		!near(cand.WaitSegmentMs, 200, 0.001) || !near(cand.WaitSleepMs, 200, 0.001) ||
		!near(cand.SpanEnvelopeMs, 200, 0.001) {
		t.Fatalf("EXT 件1: fold-winner-interval convergence drifted: basis=%q Σ=%.3f sleep=%.3f env=%.3f",
			cand.BlockingValueBasis, cand.WaitSegmentMs, cand.WaitSleepMs, cand.SpanEnvelopeMs)
	}
	if !near(cand.StartTs, 100.05, 1e-9) || !near(cand.EndTs, 100.15, 1e-9) {
		t.Fatalf("件A: the survivor's display interval stays its own, got %.6f..%.6f", cand.StartTs, cand.EndTs)
	}
	if cand.WaitBudgetExceeded {
		t.Fatalf("件A: the budget must window on the value-winner interval (200 ≤ 200, waiter fully sleeping) — false ⚠ minted: nonRun=%.3f run=%.3f",
			cand.WaitBudgetNonRunningMs, cand.WaitBudgetRunningMs)
	}
}

// TestBlockingSpanFoldBudgetStillFiresOnValueWinnerIntervalXERR1FixA — 件A
// 正向臂: the SAME fold shape, but the waiter genuinely RAN for half of the
// value-winner interval — the budget verdict windows on 100.0..100.2 and
// STILL fires (envelope 200 > non-running 100), so 件A is an interval fix,
// never a marker retreat.
func TestBlockingSpanFoldBudgetStillFiresOnValueWinnerIntervalXERR1FixA(t *testing.T) {
	idx := buildTraceIndex(t, "xerr1_fixa_running_waiter.systrace", `
       app-100 (100) [001] .... 99.900000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 100.100000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 100.250000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 100.0, TimeEnd: 100.2, MinDurationMs: 0.001})
	rows := collectBlockingSpanRows(idx, q, xerr1FixAFoldStats())
	if len(rows) != 1 {
		t.Fatalf("fixture drifted: the dual print forms must fold into ONE row, got %d: %+v", len(rows), rows)
	}
	cand := rows[0].cand
	if !cand.WaitBudgetExceeded || !near(cand.WaitBudgetNonRunningMs, 100, 0.01) || !near(cand.WaitBudgetRunningMs, 100, 0.01) {
		t.Fatalf("件A 正向: a true over-budget on the value-winner interval must still fire: exceeded=%v nonRun=%.3f run=%.3f",
			cand.WaitBudgetExceeded, cand.WaitBudgetNonRunningMs, cand.WaitBudgetRunningMs)
	}
	// XERR1-EXT 件1: the half-running waiter converges 200 → Σ(sleep
	// 100.0-100.1)=100ms on the value-winner interval; the take-MAX envelope
	// is preserved on SpanEnvelopeMs.
	if cand.BlockingValueBasis != BlockingValueBasisWaitSegments ||
		!near(cand.DurationMs, 100, 0.01) || !near(cand.WaitSleepMs, 100, 0.01) ||
		!near(cand.SpanEnvelopeMs, 200, 0.01) {
		t.Fatalf("EXT 件1: fold convergence drifted: basis=%q dur=%.3f sleep=%.3f env=%.3f",
			cand.BlockingValueBasis, cand.DurationMs, cand.WaitSleepMs, cand.SpanEnvelopeMs)
	}
}

// TestBlockingSpanPartialCoverageLowerBoundXERR1FixF — 件F (冷读 P3-3): when
// the waiter's account does not tile the span window (the F-2 禁判 shape),
// the converged value itself now DISCLOSES it is a proven lower bound —
// typed marker + covered total + the LLM-visible Summary sentence. 负向臂:
// a full-coverage account mints nothing.
func TestBlockingSpanPartialCoverageLowerBoundXERR1FixF(t *testing.T) {
	// Positive arm — account covers only 1.150..1.200 (50ms) of the 200ms
	// span window; converged value = sleep 1.150..1.190 = 40ms.
	idx := buildTraceIndex(t, "xerr1_fixf_partial.systrace", `
       app-100 (100) [001] .... 0.900000: print: B|100|H:TraversalWait frame
       app-100 (100) [001] .... 1.150000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-300 (300) [000] .... 1.190000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
       app-100 (100) [001] .... 1.250000: print: E|100
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MinDurationMs: 0.001})
	stats := ComputeWindowStats(idx, q)
	rows := collectBlockingSpanRows(idx, q, stats)
	var cand *CriticalBlockingCandidate
	for i := range rows {
		if rows[i].cand.Type == "blocking_span" && rows[i].cand.BlockingKind == "" {
			cand = &rows[i].cand
			break
		}
	}
	if cand == nil {
		t.Fatalf("fixture drifted: no payload-less blocking_span row: %+v", rows)
	}
	if cand.BlockingValueBasis != BlockingValueBasisWaitSegments || !near(cand.DurationMs, 40, 0.01) {
		t.Fatalf("fixture drifted: converged value must be Σ=40ms on wait_segments, got basis=%q dur=%.3f", cand.BlockingValueBasis, cand.DurationMs)
	}
	if !cand.WaitCoveragePartial || !near(cand.WaitAccountCoveredMs, 50, 0.01) {
		t.Fatalf("件F: partial coverage must mint the typed lower-bound marker: partial=%v covered=%.3f",
			cand.WaitCoveragePartial, cand.WaitAccountCoveredMs)
	}
	if cand.WaitBudgetExceeded {
		t.Fatalf("F-2 禁判 must survive 件F: %+v", cand)
	}
	if !strings.Contains(cand.Summary, "proven lower bound") ||
		!strings.Contains(cand.Summary, "covers only 50.000ms of the 200.000ms span window") {
		t.Fatalf("件F: the Summary must carry the lower-bound sentence: %q", cand.Summary)
	}

	// Negative arm — the E1 full-coverage shape mints NO coverage marker.
	idxFull := buildTraceIndex(t, "xerr1_fixf_full.systrace", `
       app-100 (100) [001] .... 0.900000: print: B|100|H:TraversalWait frame
       idle-0 (0) [001] .... 0.950000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 1.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-300 (300) [000] .... 1.080000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
       idle-0 (0) [001] .... 1.100000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 1.188000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-300 (300) [000] .... 1.200000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
       app-100 (100) [001] .... 1.250000: print: E|100
	`)
	qFull := normalizeQuery(idxFull, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MinDurationMs: 0.001})
	rowsFull := collectBlockingSpanRows(idxFull, qFull, ComputeWindowStats(idxFull, qFull))
	for _, row := range rowsFull {
		if row.cand.Type != "blocking_span" || row.cand.BlockingKind != "" {
			continue
		}
		if row.cand.WaitCoveragePartial || row.cand.WaitAccountCoveredMs != 0 {
			t.Fatalf("件F 负向: a full-coverage account must not mint the marker: %+v", row.cand)
		}
		if strings.Contains(row.cand.Summary, "lower bound") {
			t.Fatalf("件F 负向: no lower-bound sentence on full coverage: %q", row.cand.Summary)
		}
		return
	}
	t.Fatalf("fixture drifted: no payload-less blocking_span row in the negative arm: %+v", rowsFull)
}

// TestBlockingSpanPureRunningRunnableConvergesToZeroXERR1FixG — 件G① (冷读
// P3-4): a waiter that spends the whole span window running+runnable (zero
// sleep/D/io) converges to an HONEST 0 — no panic, basis wait_segments, the
// envelope preserved, and the 件3 marker firing with TRUE numbers (the
// runnable share is non-running budget; the disclosure is honest, not 假).
func TestBlockingSpanPureRunningRunnableConvergesToZeroXERR1FixG(t *testing.T) {
	idx := buildTraceIndex(t, "xerr1_fixg_zero_wait.systrace", `
       idle-0 (0) [001] .... 0.950000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 0.960000: print: B|100|H:TraversalWait frame
      rival-200 (200) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=R ==> next_comm=rival next_pid=200 next_prio=110
       idle-0 (0) [001] .... 1.250000: sched_switch: prev_comm=rival prev_pid=200 prev_prio=110 prev_state=S ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 1.260000: print: E|100
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MinDurationMs: 0.001})
	rows := collectBlockingSpanRows(idx, q, ComputeWindowStats(idx, q))
	var cand *CriticalBlockingCandidate
	for i := range rows {
		if rows[i].cand.Type == "blocking_span" && rows[i].cand.BlockingKind == "" {
			cand = &rows[i].cand
			break
		}
	}
	if cand == nil {
		t.Fatalf("fixture drifted: no payload-less blocking_span row: %+v", rows)
	}
	if cand.BlockingValueBasis != BlockingValueBasisWaitSegments {
		t.Fatalf("件G①: full-coverage running+runnable timeline must converge (wait_segments), got %q", cand.BlockingValueBasis)
	}
	if cand.DurationMs != 0 || cand.WaitSegmentMs != 0 || cand.WaitSleepMs != 0 || cand.WaitDStateMs != 0 || cand.WaitIOWaitMs != 0 {
		t.Fatalf("件G①: the converged value must be an honest 0, got dur=%.3f Σ=%.3f", cand.DurationMs, cand.WaitSegmentMs)
	}
	if !near(cand.SpanEnvelopeMs, 200, 0.01) {
		t.Fatalf("件G①: the envelope must be preserved, got %.3f", cand.SpanEnvelopeMs)
	}
	if !strings.Contains(cand.Summary, "Σ(sleep+d_state+io_wait)=0.000ms") {
		t.Fatalf("件G①: the Summary head must speak the honest 0: %q", cand.Summary)
	}
	// 件3 rides with TRUE numbers: running 1.0-1.1=100ms, runnable
	// 1.1-1.2=100ms → non-running 100 < envelope 200 (honest disclosure).
	if !cand.WaitBudgetExceeded || !near(cand.WaitBudgetNonRunningMs, 100, 0.01) || !near(cand.WaitBudgetRunningMs, 100, 0.01) {
		t.Fatalf("件G①: the budget marker must fire with true numbers: exceeded=%v nonRun=%.3f run=%.3f",
			cand.WaitBudgetExceeded, cand.WaitBudgetNonRunningMs, cand.WaitBudgetRunningMs)
	}
	if cand.WaitCoveragePartial {
		t.Fatalf("件G①: full coverage must not mint the 件F marker: %+v", cand)
	}
}
