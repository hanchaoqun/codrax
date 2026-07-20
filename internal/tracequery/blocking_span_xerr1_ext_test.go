package tracequery

// blocking_span_xerr1_ext_test.go — XERR1-EXT engine pins (§29.104.17 裁定⑤,
// user ruling 2026-07-16「同意,按推荐的来」; spec home §29.107 基建).
//
// The extension: payload-TYPED contention rows (ART monitor/mutex, OHOS
// structured forms) converge their published value to the WAITER's
// Σ(sleep+D+io_wait) inside span∩window exactly like the payload-less lane —
// windowed on the fold VALUE-WINNER interval (件A 值胜出区间纪律), envelope
// preserved on SpanEnvelopeMs + disclosed, F-2 budget gate and 件F coverage
// discipline shared. 改值通道且改榜序=用户已准. The LOCKNS holder lanes
// (derivation ladder / confidence / presence / sentinel disclosures) are
// pinned byte-stable — value lane ⊥ holder lane.

import (
	"context"
	"strings"
	"testing"
)

// TestBlockingSpanPayloadNoTimelineFailOpenXERR1EXT — fail-open arm ②: a
// payload-typed row whose waiter has NO scheduler timeline inside the span
// window keeps the envelope value with typed basis=span_envelope + the
// Summary disclosure; the legacy "lasted <envelope>" head survives (the value
// really IS the envelope) and the lock word family is untouched.
func TestBlockingSpanPayloadNoTimelineFailOpenXERR1EXT(t *testing.T) {
	// The waiter (pid 100) emits the contention span but has zero sched
	// events anywhere — buildCriticalBlockingThreadState returns nil.
	idx := buildTraceIndex(t, "xerr1_ext_no_timeline.systrace", `
       app-100 (100) [001] .... 1.000000: print: B|100|Lock contention on a monitor lock (owner tid: 200)
       app-100 (100) [001] .... 1.200000: print: E|100
      other-300 (300) [000] .... 1.100000: sched_wakeup: comm=other pid=300 prio=120 target_cpu=000
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MinDurationMs: 0.001})
	rows := collectBlockingSpanRows(idx, q, ComputeWindowStats(idx, q))
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
	if lockRow.BlockingValueBasis != BlockingValueBasisSpanEnvelope {
		t.Fatalf("fail-open arm ②: basis must be span_envelope, got %q", lockRow.BlockingValueBasis)
	}
	if !near(lockRow.DurationMs, 200, 0.01) || !near(lockRow.SpanEnvelopeMs, 200, 0.01) {
		t.Fatalf("fail-open arm ②: the value must stay the envelope: dur=%.3f env=%.3f", lockRow.DurationMs, lockRow.SpanEnvelopeMs)
	}
	if lockRow.WaitSegmentMs != 0 || lockRow.WaitBudgetExceeded || lockRow.WaitCoveragePartial {
		t.Fatalf("fail-open arm ②: no convergence account and no ⚠ may mint: %+v", lockRow)
	}
	if !strings.Contains(lockRow.Summary, "lasted 200.000ms") ||
		!strings.Contains(lockRow.Summary, "value basis: span envelope (the waiter has no scheduler timeline inside the span window") {
		t.Fatalf("fail-open arm ②: the legacy head + the basis disclosure must both ride: %q", lockRow.Summary)
	}
}

// TestBlockingSpanPayloadZeroWidthSpanFailOpenXERR1EXT — fail-open arm ①
// (live tieba 形B shape: three zero-width contention prints in the census): a
// zero-width span has NO usable value interval — 同基 convergence is
// forbidden, the basis says span_envelope with the empty-interval sentence,
// and no ⚠ may mint (the budget was 禁判ed by the same missing interval).
func TestBlockingSpanPayloadZeroWidthSpanFailOpenXERR1EXT(t *testing.T) {
	idx := buildTraceIndex(t, "xerr1_ext_zero_width.systrace", `
       idle-0 (0) [001] .... 0.950000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
       app-100 (100) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
	q := normalizeQuery(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MinDurationMs: 0.001})
	row := blockingSpanRow{
		cand: CriticalBlockingCandidate{
			Type: "blocking_span", Thread: ThreadRef{Comm: "app", PID: 100},
			BlockingKind: "lock_contention", DurationMs: 0,
			StartTs: 1.05, EndTs: 1.05, LineStart: 10, LineEnd: 10, Confidence: 0.72,
			Summary: `blocking-like trace span "Lock contention on thread suspend count lock (owner tid: 0)" lasted 0.000ms blocking_kind=lock_contention`,
		},
		spanName: "Lock contention on thread suspend count lock (owner tid: 0)",
	}
	convergeBlockingSpanRowValue(idx, q, &row)
	cand := row.cand
	if cand.BlockingValueBasis != BlockingValueBasisSpanEnvelope {
		t.Fatalf("fail-open arm ①: basis must be span_envelope, got %q", cand.BlockingValueBasis)
	}
	if cand.DurationMs != 0 || cand.WaitSegmentMs != 0 || cand.WaitBudgetExceeded {
		t.Fatalf("fail-open arm ①: value untouched, no account, no ⚠: %+v", cand)
	}
	if !strings.Contains(cand.Summary, "value basis: span envelope (the value's own span∩window interval is empty or unavailable") {
		t.Fatalf("fail-open arm ①: the empty-interval disclosure must ride: %q", cand.Summary)
	}
	if !strings.Contains(cand.Summary, "lasted 0.000ms blocking_kind=lock_contention") {
		t.Fatalf("fail-open arm ①: the legacy head must survive untouched: %q", cand.Summary)
	}
}

// TestLocknsDonghuContainerFormAValueConvergesXERR1EXT — 件4③ (donghu 形A
// container form): the LOCKNS derivation chain is byte-stable — rung ②
// PROCESS-level × rung ③ closing-waker corroboration resolving to host
// ransmitThread-18130 at the 0.67 ns-span grade — while ONLY the value lane
// re-calibers: envelope 0.295ms → Σ(sleep)=0.185ms. Account closure against
// the 件3 budget trio (same builder, same interval): envelope 0.295 =
// non-running 0.214 + running 0.081, and Σ(sleep) 0.185 = non-running 0.214 −
// runnable 0.029.
func TestLocknsDonghuContainerFormAValueConvergesXERR1EXT(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Skipf("donghu fixture not present: %v", err)
	}
	q := Query{PID: 17267, TimeStart: 13762.9805, TimeEnd: 13762.9820, MinDurationMs: 0.0001}
	spans, _, _ := computeTraceMarks(idx, q, 64)
	var contention []TraceSpanSummary
	for _, sp := range spans {
		if strings.Contains(sp.Name, "contention") {
			contention = append(contention, sp)
		}
	}
	rows := collectBlockingSpanRows(idx, q, WindowStats{TraceSpans: contention})
	var mon *CriticalBlockingCandidate
	for i := range rows {
		if rows[i].cand.BlockingKind == blockingKindMonitorContention && rows[i].cand.Thread.PID == 17585 {
			mon = &rows[i].cand
			break
		}
	}
	if mon == nil {
		t.Fatalf("LegoHandler monitor-contention row missing: %d rows", len(rows))
	}
	// LOCKNS 面零触 (byte-stable derivation chain — the §29.111 pins' facts).
	if mon.Peer.PID != 18130 || mon.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("LOCKNS 零触: derivation must stay →ransmitThread-18130 via corroborated closing waker, got source=%q peer=%+v", mon.HolderSource, mon.Peer)
	}
	if diff := mon.Confidence - counterpartNsSpanDerivationConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("LOCKNS 零触: the 0.67 ns-span grade must hold, got %.3f", mon.Confidence)
	}
	if !strings.Contains(mon.HolderHostProcess, "tgid=17267") || !strings.Contains(mon.Summary, "corroborate") {
		t.Fatalf("LOCKNS 零触: process-level identity + corroboration disclosure must survive: %q / %q", mon.HolderHostProcess, mon.Summary)
	}
	if mon.OwnerTidRaw != 38414 {
		t.Fatalf("LOCKNS 零触: the container owner tid audit field must hold, got %d", mon.OwnerTidRaw)
	}
	// Value lane 换径 only.
	if mon.BlockingValueBasis != BlockingValueBasisWaitSegments {
		t.Fatalf("EXT 件1: basis must be wait_segments, got %q", mon.BlockingValueBasis)
	}
	if !near(mon.DurationMs, 0.185, 0.001) || !near(mon.WaitSleepMs, 0.185, 0.001) || !near(mon.SpanEnvelopeMs, 0.295, 0.001) {
		t.Fatalf("EXT 件1: donghu 形A must converge 0.295→0.185(sleep), got dur=%.3f sleep=%.3f env=%.3f",
			mon.DurationMs, mon.WaitSleepMs, mon.SpanEnvelopeMs)
	}
	if !mon.WaitBudgetExceeded || !near(mon.WaitBudgetNonRunningMs, 0.214, 0.001) || !near(mon.WaitBudgetRunningMs, 0.081, 0.001) {
		t.Fatalf("件3: budget trio drifted: exceeded=%v nonRun=%.3f run=%.3f",
			mon.WaitBudgetExceeded, mon.WaitBudgetNonRunningMs, mon.WaitBudgetRunningMs)
	}
	// Account closure: envelope = non-running + running (same interval, same
	// builder — hand-recomputable from the budget trio).
	if !near(mon.SpanEnvelopeMs, mon.WaitBudgetNonRunningMs+mon.WaitBudgetRunningMs, 0.001) {
		t.Fatalf("closure: envelope %.3f != nonRun %.3f + run %.3f",
			mon.SpanEnvelopeMs, mon.WaitBudgetNonRunningMs, mon.WaitBudgetRunningMs)
	}
	if !strings.Contains(mon.Summary, "Σ(sleep+d_state+io_wait)=0.185ms") ||
		!strings.Contains(mon.Summary, "span envelope 0.295ms contains run time") ||
		!strings.Contains(mon.Summary, "holder_site=") {
		t.Fatalf("EXT 件1: converged Summary head + typed lock suffix must ride: %q", mon.Summary)
	}
}

// TestBlockingSpanTiebaSentinelConvergesXERR1EXT — 件4② slice (tieba 形B
// sentinel window): the ownerless-sentinel µs spin rows (waiter RUNS the
// whole tiny span) converge to an HONEST 0.000 while every holder-lane fact —
// sentinel no-attribution disclosure, wait_object, 0.72 span-typing grade —
// stays byte-stable, and the rows still PUBLISH (µs 级行地板不误伤: the
// zero-value row keeps its line identity through the critical_blocking add
// gate).
func TestBlockingSpanTiebaSentinelConvergesXERR1EXT(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Skipf("tieba fixture not present: %v", err)
	}
	q := Query{PID: 59566, TimeStart: 34579.4530, TimeEnd: 34579.4560, MinDurationMs: 0.0001}
	spans, _, _ := computeTraceMarks(idx, q, 128)
	var contention []TraceSpanSummary
	for _, sp := range spans {
		if strings.Contains(sp.Name, "contention") {
			contention = append(contention, sp)
		}
	}
	rows := collectBlockingSpanRows(idx, q, WindowStats{TraceSpans: contention})
	if len(rows) == 0 {
		t.Fatalf("expected sentinel contention rows in the probe window")
	}
	for _, row := range rows {
		c := row.cand
		// LOCKNS 面零触 (the §29.111 sentinel pins' facts).
		if c.BlockingKind != blockingKindLockContention || c.WaitObject != "ClassLinker classes lock" {
			t.Fatalf("LOCKNS 零触: sentinel typed semantics drifted: kind=%q obj=%q", c.BlockingKind, c.WaitObject)
		}
		if c.Peer.PID != 0 || c.OwnerTidRaw != 0 {
			t.Fatalf("LOCKNS 零触: sentinel must never mint an owner: %+v", c.Peer)
		}
		if !strings.Contains(c.Summary, "holder unresolved") {
			t.Fatalf("LOCKNS 零触: the sentinel disclosure must survive convergence: %s", c.Summary)
		}
		if c.Confidence != 0.72 {
			t.Fatalf("LOCKNS 零触: the 0.72 span-typing grade must hold, got %.2f", c.Confidence)
		}
		// Value lane: the µs spin rows (pure running) converge to an honest 0
		// with the envelope preserved; the row still publishes with its line
		// identity (地板不误伤).
		if c.BlockingValueBasis != BlockingValueBasisWaitSegments {
			t.Fatalf("EXT 件1: sentinel row basis must be wait_segments, got %q", c.BlockingValueBasis)
		}
		if c.DurationMs != 0 || c.SpanEnvelopeMs <= 0 {
			t.Fatalf("EXT 件1: pure-spin sentinel must converge to honest 0 with envelope preserved: dur=%.3f env=%.3f", c.DurationMs, c.SpanEnvelopeMs)
		}
		if c.LineStart <= 0 {
			t.Fatalf("地板: the zero-value row must keep its line identity: %+v", c)
		}
	}
}

// TestLockSeatCapFailLoudDisclosureXERR1EXTFixA — 修补 件A e2e (冷读 P1
// 处置=披露道, §29.104.13 只披露不硬拦 + 排除≠消失).
//
// EVOLUTION RECORD (CAPFIX-1 件1, §29.150① user ruling 2026-07-19): the
// original 正臂 pinned the converged 0.066ms lock seat DYING at the default
// 12-seat cap at pool position #14 — behind TWELVE ▒ background rows on a
// chainless-sorted micro-window board (chain threads=1 → channel-blind sort)
// while the seat carried the typed §12.3 resolved-pair ON-CHAIN credential.
// That death shape is exactly what the CAPFIX-1 cross-lane cap survival
// retires (链上有值席不得在背景行占位时帽亡): the default board now PUBLISHES
// the lock seat on a chain ordinal and the smallest background rows fall to
// the disclosed pool instead. R-2's fail-loud disclosure machinery is NOT
// retired — it owns the residual form (the seat dying to HIGHER-eff on-chain
// seats), pinned below through the same production truncation wiring.
func TestLockSeatCapFailLoudDisclosureXERR1EXTFixA(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Skipf("tieba fixture not present: %v", err)
	}
	countPrefix := func(caveats []string) int {
		n := 0
		for _, c := range caveats {
			if strings.HasPrefix(c, lockSeatRankFailLoudCaveatPrefix) {
				n++
			}
		}
		return n
	}
	// CAPFIX-1 正臂 (链上凭证席帽内): default parameters — the resolved lock
	// seat survives the cap on its on-chain credential; the board carries
	// ZERO fail-loud disclosures and the displaced background rows are
	// disclosed with per-lane counts on the compaction caveat (件2).
	rank := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.4605, TimeEnd: 34579.4615})
	lockSeat := false
	for _, item := range rank.Items {
		if item.Type == "blocking_span" && item.BlockingKind != "" {
			if item.Rank <= 0 || !rootCauseItemIsOnChain(item) {
				t.Fatalf("CAPFIX-1: the surviving lock seat must hold an on-chain ordinal seat: %+v", item)
			}
			lockSeat = true
		}
	}
	if !lockSeat {
		t.Fatalf("CAPFIX-1 件1: the on-chain resolved lock seat must survive the default cap (pre-CAPFIX death shape: #14 behind 12 background rows): %v", rank.Items)
	}
	if got := countPrefix(rank.Caveats); got != 0 {
		t.Fatalf("a board WITH a published lock seat must carry zero disclosures, got %d: %v", got, rank.Caveats)
	}
	capDeathDisclosed := false
	for _, c := range rank.Caveats {
		if strings.Contains(c, "did not enter the published board (on_chain=0/") &&
			strings.Contains(c, "另有") {
			capDeathDisclosed = true
		}
	}
	if !capDeathDisclosed {
		t.Fatalf("件2: the displaced background rows must be value-disclosed with honest per-lane counts (on_chain=0): %v", rank.Caveats)
	}
	// R-2 残余形 正臂 (production truncation wiring): the same typed resolved
	// pair dying to TWELVE higher-eff ON-CHAIN seats — the lane R-2 still
	// owns. The fail-loud caveat must disclose the silent whole-seat loss and
	// point at critical_blocking_calls.
	var pool []RootCauseRankItem
	for i := 0; i < 12; i++ {
		pool = append(pool, RootCauseRankItem{
			Type: "d_state_or_io_wait", Thread: ThreadRef{Comm: "io", PID: 300 + i},
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			DStateMs: 5 - float64(i)*0.1, ImpactMs: 5 - float64(i)*0.1,
			EffectiveImpactMs: 5 - float64(i)*0.1, CumulativeImpactMs: 5 - float64(i)*0.1,
		})
	}
	pool = append(pool, RootCauseRankItem{
		Type: "blocking_span", Thread: ThreadRef{Comm: "T7@ZeusThreadPo", PID: 61839},
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
		BlockingKind: "lock_contention", BlockingPeer: ThreadRef{Comm: "com.baidu.tieba", PID: 59566},
		ImpactMs: 0.066, EffectiveImpactMs: 0.066, CumulativeImpactMs: 0.066,
	})
	sortRootCauseRankItems(pool, true)
	published, _, _, _, _, _ := truncateRootCauseRankCandidatesAndSideRows(pool, 12)
	caveat, ok := lockSeatRankFailLoudCaveat(pool, published)
	if !ok {
		t.Fatalf("R-2 残余形: the lock seat dying to higher-eff chain seats must fail-loud")
	}
	if !strings.Contains(caveat, "largest 0.066ms at pool position") ||
		!strings.Contains(caveat, "see critical_blocking_calls") {
		t.Fatalf("件A: the disclosure must name the largest dropped seat and the follow-up view: %q", caveat)
	}
	// 负臂 (帽内形): fine parameters — the lock seat publishes at #1, zero
	// disclosure.
	fine := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.4605, TimeEnd: 34579.4615, MinDurationMs: 0.0001, Limit: 16})
	lockPublished := false
	for _, item := range fine.Items {
		if item.Type == "blocking_span" && item.BlockingKind != "" {
			lockPublished = true
			break
		}
	}
	if !lockPublished {
		t.Fatalf("fixture drifted: the fine-parameter board must publish the lock seat: %+v", fine.Items)
	}
	if got := countPrefix(fine.Caveats); got != 0 {
		t.Fatalf("件A 负臂: a board WITH a published lock seat must carry zero disclosures, got %d: %v", got, fine.Caveats)
	}
}

// TestLockSeatRankFailLoudCaveatShapesXERR1EXTFixA — 修补 件A unit shapes:
// the 同板矛盾形 (a published seat wears the 锁等待主导 demotion note that
// references a resolved contention while NO lock seat is on the board) MUST
// carry the disclosure — the demotion's own precondition (a resolved
// contention span) guarantees the pool holds the resolved lock candidate, so
// the caveat condition is implied. 负臂: published lock seat silences;
// unresolved-only pool never fires (the pool signal is the SAME §12.3 typed
// admission pair).
func TestLockSeatRankFailLoudCaveatShapesXERR1EXTFixA(t *testing.T) {
	resolvedLock := RootCauseRankItem{
		Type: "blocking_span", BlockingKind: "monitor_contention",
		Thread:       ThreadRef{Comm: "holder", PID: 200},
		BlockingPeer: ThreadRef{Comm: "waiter", PID: 100},
		ImpactMs:     0.5,
	}
	demotedInversion := RootCauseRankItem{
		Type: "priority_inversion_candidate", Thread: ThreadRef{Comm: "waiter", PID: 100},
		PriorityInversionLockDominated: true, ImpactMs: 0.4,
	}
	// 同板矛盾形: demotion note published, lock seat absent → fires.
	caveat, ok := lockSeatRankFailLoudCaveat(
		[]RootCauseRankItem{resolvedLock, demotedInversion},
		[]RootCauseRankItem{demotedInversion})
	if !ok || !strings.Contains(caveat, "largest 0.500ms at pool position 1 of 2") {
		t.Fatalf("件A 矛盾形: the disclosure must fire with the pool witness, got ok=%v %q", ok, caveat)
	}
	// 负臂 ①: the lock seat IS published → silent.
	if _, ok := lockSeatRankFailLoudCaveat(
		[]RootCauseRankItem{resolvedLock},
		[]RootCauseRankItem{resolvedLock}); ok {
		t.Fatalf("件A 负臂: a published lock seat must silence the disclosure")
	}
	// 负臂 ②: unresolved-only pool (typed pair reads false) → never fires.
	unresolved := resolvedLock
	unresolved.BlockingPeer = ThreadRef{}
	if _, ok := lockSeatRankFailLoudCaveat(
		[]RootCauseRankItem{unresolved},
		nil); ok {
		t.Fatalf("件A 负臂: an unresolved lock candidate must not fire the disclosure")
	}
	// Dedupe sentinel: the enrich lane never doubles the sentence.
	if !hasLockSeatRankFailLoudCaveat([]string{caveat}) || hasLockSeatRankFailLoudCaveat([]string{"other caveat"}) {
		t.Fatalf("件A: the dedupe sentinel drifted")
	}
}

// TestBlockingSpanSpanEnvelopeRankCaveatXERR1EXTFixC — 修补 件C rider (冷读
// P3-2): the span_envelope fail-open row's rank summary head still speaks the
// envelope figure — the caliber caveat aligns the rank face with the blocking
// face. 负臂: a basis-less legacy row appends nothing.
func TestBlockingSpanSpanEnvelopeRankCaveatXERR1EXTFixC(t *testing.T) {
	row := blockingSpanRow{
		cand: CriticalBlockingCandidate{
			Type: "blocking_span", Thread: ThreadRef{Comm: "app", PID: 100},
			BlockingKind: "lock_contention", DurationMs: 0.5,
			BlockingValueBasis: BlockingValueBasisSpanEnvelope, SpanEnvelopeMs: 0.5,
			StartTs: 1.0, EndTs: 1.0005, LineStart: 10, LineEnd: 11, Confidence: 0.72,
		},
		spanName: "Lock contention on thread suspend count lock (owner tid: 0)",
	}
	item := rootCauseItemFromLockContentionCandidate(Query{TimeStart: 1.0, TimeEnd: 1.2}, nil, false, row)
	if !strings.Contains(item.Summary, "value = the span envelope 0.500ms (contains run time; the waiter's wait segments could not be derived — not a measured blocking wait)") {
		t.Fatalf("件C: the envelope caliber caveat must ride the rank summary: %q", item.Summary)
	}
	legacy := row
	legacy.cand.BlockingValueBasis = ""
	legacy.cand.SpanEnvelopeMs = 0
	if got := rootCauseItemFromLockContentionCandidate(Query{TimeStart: 1.0, TimeEnd: 1.2}, nil, false, legacy); strings.Contains(got.Summary, "value = the span envelope") {
		t.Fatalf("件C 负臂: a basis-less legacy row must append nothing: %q", got.Summary)
	}
}

// TestBlockingSpanRankLaneFollowsConvergedValueXERR1EXT — 件3 榜序验收 (the
// user-approved core face: 改值通道且改榜序=用户已准): on the tieba
// suspend-count contention window the lock rank seat keeps its #1 board
// position with the HONEST converged value — impact 0.141(envelope) →
// 0.066(Σ sleep), Score = 0.066 × 0.67 × 1.35 (resolved-lock weight, §12.3
// typed-pair admission unchanged and value-independent). Hand closure of the
// displaced share: envelope 0.141 = running 0.049 + sleep 0.066 + runnable
// 0.026 — and the excluded runnable share equals the SAME waiter's runnable
// seats (#2/#3, 0.026) on the same board, published where runnable belongs.
func TestBlockingSpanRankLaneFollowsConvergedValueXERR1EXT(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Skipf("tieba fixture not present: %v", err)
	}
	q := normalizeQuery(idx, Query{PID: 59566, TimeStart: 34579.4605, TimeEnd: 34579.4615, MinDurationMs: 0.0001})
	stats := ComputeWindowStats(idx, q)
	chain := BuildWakeupChain(idx, q)
	res := buildRootCauseRankFrom(idx, q, chain, stats)
	if len(res.Items) == 0 {
		t.Fatalf("fixture drifted: empty rank board")
	}
	lock := res.Items[0]
	if lock.Type != "blocking_span" || lock.BlockingKind != "lock_contention" {
		t.Fatalf("榜序: the lock seat must hold board position #1, got #1=%s kind=%q", lock.Type, lock.BlockingKind)
	}
	if lock.Thread.PID != 61839 {
		t.Fatalf("榜序: the holder-subject seat drifted: %+v", lock.Thread)
	}
	if !near(lock.ImpactMs, 0.066, 0.001) || !near(lock.CumulativeImpactMs, 0.066, 0.001) {
		t.Fatalf("EXT 件1: the rank value must follow the converged DurationMs (0.141→0.066), got impact=%.3f cum=%.3f",
			lock.ImpactMs, lock.CumulativeImpactMs)
	}
	// Score = impact × confidence × resolved-lock weight (§12.3 1.35 lane, no
	// cached pre-convergence value anywhere).
	if !near(lock.Score, lock.ImpactMs*lock.Confidence*1.35, 0.0005) {
		t.Fatalf("EXT: Score must derive from the converged impact: score=%.4f impact=%.3f conf=%.2f",
			lock.Score, lock.ImpactMs, lock.Confidence)
	}
	if lock.Tier != "primary" || lock.ChainRelevance != "on_chain" {
		t.Fatalf("§12.3 admission unchanged: tier=%q rel=%q", lock.Tier, lock.ChainRelevance)
	}
	if !strings.Contains(lock.Summary, "value = the waiter's wait segments Σ(sleep+d_state+io_wait)=0.066ms") ||
		!strings.Contains(lock.Summary, "span envelope 0.141ms contains run time") {
		t.Fatalf("EXT 件2: the rank summary must disclose the converged caliber + envelope: %q", lock.Summary)
	}
	// The excluded runnable share is not lost — it lives on the waiter's own
	// runnable seats (same board, honest channel).
	foundRunnable := false
	for _, item := range res.Items[1:] {
		if item.Type == "runnable_wait" && item.Thread.PID == 59566 && near(item.ImpactMs, 0.026, 0.001) {
			foundRunnable = true
			break
		}
	}
	if !foundRunnable {
		t.Fatalf("榜序 closure: the waiter's 0.026 runnable seat must stand beside the converged lock seat: %+v", res.Items)
	}
}
