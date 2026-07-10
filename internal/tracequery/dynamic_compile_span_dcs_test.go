package tracequery

// dynamic_compile_span_dcs_test.go — DCS 修复批 engine pins (ledger
// real_trace_campaign_20260705.md §23/§23.1, 2026-07-08):
//
//   E1  reserved seats for 窗内∧链上 semantic spans; current SEM-LEAD-P0
//       makes them ordinary primary/secondary/tertiary election candidates.
//   E1b non-chain spans join the background composite board ordered by the
//       window-share basis (占窗比 — wall-clock clipped ÷ window for spans,
//       own-caliber cumulative ÷ window for everything else; NEVER raw Score
//       across calibers, §7.30 S1), plus one guaranteed board seat.
//   E2  mint fall-through (chain-exists-no-overlap → typed non-chain row) and
//       the opened PID gate (external-process spans mint; cmp_01 E2 witness);
//       道别红线: enrichment thread-membership can never flip a mint-time
//       non-chain span onto the on-chain lane.
//   E3  fail-loud caveat: classified semantic spans present but 0 semantic
//       rank rows published (precise count comparison).
//   E4  cross-window-boundary B/E pairs mint window-clipped spans with typed
//       actual_* extent; dangling semantic-word-surface pairs caveat.
//   E2E cmp_01 6.0-shape fixture: 16-span zero-chain window with a saturated
//       board → background seat keeps the top span visible; the E6 mention
//       gate reads its typed background_rank.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// --- E1: tier assignment + on-chain election -----------------------------------

func TestDCSAssignTiersOnChainSemanticParticipatesInElection(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "jit_compile", ImpactMs: 5, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "runnable_wait", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "supply_pressure", ImpactMs: 40, ChainRelevance: "background"},
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != "primary" {
		t.Fatalf("the highest on-chain semantic span must be a primary candidate: %+v", items[0])
	}
	if items[0].BackgroundRank != 0 {
		t.Fatalf("on-chain semantic span is not on the background board: %+v", items[0])
	}
	if items[1].Tier != "primary" {
		t.Fatalf("the existing on-chain runnable co-primary rule must remain: %+v", items[1])
	}
	if items[2].Tier != "tertiary" {
		t.Fatalf("the ordinary ladder must include the semantic row: %+v", items[2])
	}
	// 复核 F-2 (ledger §23.2): the position COUNTS every non-on-chain row but
	// the FIELD is stamped on semantic rows only — a non-semantic background
	// row stays 0 so the JSON payload of semantic-free traces is byte-stable.
	if items[2].BackgroundRank != 0 {
		t.Fatalf("non-semantic rows never carry the background_rank field: %+v", items[2])
	}
}

// 复核 F-4 (ledger §23.1 ruling ②实施收口): a NON-chain semantic span is just
// as transparent to the positional election ladder as the on-chain tier rows —
// in the degenerate zero-aggregate board it must NOT occupy slot 0 and wear
// tier=primary (its predicate face would read root_cause_primary and the
// display would crown a 主根因), and its counted board position still rides
// background_rank.
//
// EVOLUTION RECORD: pre-F-4 the electionPos skip covered only on-chain
// semantic rows; §23.1② (非链上不入链上 tier,入背景综合排序) makes the
// non-chain lane's board identity background_rank, never an election slot.
func TestDCSNonChainSemanticNeverTakesPrimarySlot(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "jit_compile", ImpactMs: 83.893, SpanName: "JIT compiling method00"},
		{Type: "runnable_wait", ImpactMs: 9},
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier == "primary" || items[0].Tier == RootCauseTierDeterministicOptimization {
		t.Fatalf("a non-chain semantic span must never wear primary or the on-chain tier: %+v", items[0])
	}
	if items[0].Tier != "tertiary" {
		t.Fatalf("the non-chain semantic span wears the supporting band word: %+v", items[0])
	}
	if items[0].BackgroundRank != 1 {
		t.Fatalf("its board identity is the typed background position: %+v", items[0])
	}
	// The FIRST non-semantic row still takes the primary slot (ladder not
	// consumed by the transparent span).
	if items[1].Tier != "primary" {
		t.Fatalf("the election ladder must not be consumed by the semantic row: %+v", items[1])
	}
	// Fully degenerate board: ONLY the semantic row — still never primary.
	solo := []RootCauseRankItem{{Type: "shader_compile", ImpactMs: 20.5, SpanName: "shader_compile warmup"}}
	assignRootCauseRanksAndTiers(solo)
	if solo[0].Tier != "tertiary" || solo[0].BackgroundRank != 1 {
		t.Fatalf("degenerate single-span board: %+v", solo[0])
	}
}

func TestDCSSemanticSpanTypesUsePositionalElectionNotBlanketCoPrimary(t *testing.T) {
	// Semantic work competes through assignRootCauseRanksAndTiers. It does not
	// use the blanket co-primary promotion, which would incorrectly crown every
	// on-chain optimization span regardless of sorted position.
	for _, typ := range []string{"jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause"} {
		item := RootCauseRankItem{Type: typ, ImpactMs: 50, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"}
		if rootCauseShouldBeCoPrimary(item) {
			t.Fatalf("%s must use positional election, not blanket co-primary", typ)
		}
	}
	// Control: the on-chain hard precondition and the non-semantic whitelist
	// are untouched.
	if !rootCauseShouldBeCoPrimary(RootCauseRankItem{Type: "runnable_wait", ImpactMs: 5, ChainRelevance: "on_chain"}) {
		t.Fatalf("runnable_wait co-primary lane must stay")
	}
	if rootCauseShouldBeCoPrimary(RootCauseRankItem{Type: "runnable_wait", ImpactMs: 5, ChainRelevance: "background"}) {
		t.Fatalf("the on-chain hard precondition must stay")
	}
}

// --- E1/E1b: reserved seats inside the truncation ------------------------------

func dcsSeatItems(nonSemantic int) []RootCauseRankItem {
	items := make([]RootCauseRankItem, 0, nonSemantic)
	for i := 0; i < nonSemantic; i++ {
		items = append(items, RootCauseRankItem{
			Type: "runnable_wait", ImpactMs: float64(100 - i), CumulativeImpactMs: float64(100 - i),
			Score: float64(100 - i),
		})
	}
	return items
}

func TestDCSTruncationKeepsOnChainSemanticReservedSeats(t *testing.T) {
	items := dcsSeatItems(12)
	for i := 0; i < 4; i++ {
		items = append(items, RootCauseRankItem{
			Type: "jit_compile", ImpactMs: 3 + float64(i), CumulativeImpactMs: 3 + float64(i),
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Score: 2,
			SpanName: fmt.Sprintf("JIT compiling seat%d", i),
		})
	}
	out := truncateRootCauseRankItemsWithSemanticSeats(items, 12)
	if len(out) != 12 {
		t.Fatalf("truncation must emit exactly the limit: got %d", len(out))
	}
	kept := 0
	for _, item := range out {
		if rootCauseItemIsSemanticSpanWork(item) && rootCauseItemIsOnChain(item) {
			kept++
		}
	}
	// E1 正向: exactly the reserved seat budget survives; the 4th on-chain
	// span stays out (seats are a guarantee, not an unbounded bypass).
	if kept != rootCauseSemanticOnChainReservedSeats {
		t.Fatalf("expected %d on-chain semantic seats, got %d: %+v", rootCauseSemanticOnChainReservedSeats, kept, out)
	}
	// The evicted rows are the LOWEST non-semantic rows; order is preserved.
	for i := 1; i < len(out); i++ {
		if !rootCauseItemIsSemanticSpanWork(out[i]) && !rootCauseItemIsSemanticSpanWork(out[i-1]) &&
			out[i].Score > out[i-1].Score {
			t.Fatalf("relative sorted order must be preserved: %+v", out)
		}
	}
}

func TestDCSTruncationGuaranteesTopBackgroundSemanticSeat(t *testing.T) {
	items := dcsSeatItems(12)
	items = append(items, RootCauseRankItem{
		Type: "jit_compile", ImpactMs: 83.893, CumulativeImpactMs: 83.893, Score: 70,
		SpanName: "JIT compiling android.content.pm.Foo",
	})
	out := truncateRootCauseRankItemsWithSemanticSeats(items, 12)
	if len(out) != 12 {
		t.Fatalf("truncation must emit exactly the limit: got %d", len(out))
	}
	found := false
	for _, item := range out {
		if item.SpanName == "JIT compiling android.content.pm.Foo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the top non-chain semantic span must keep its guaranteed board seat (E1b): %+v", out)
	}
}

// --- E1b: background composite ordering basis ----------------------------------

func TestDCSBackgroundPlacementUsesWindowShareBasisNotScore(t *testing.T) {
	q := Query{TimeStart: 1.0, TimeEnd: 1.101}
	agg := RootCauseRankItem{Type: "supply_pressure", ImpactMs: 28914, CumulativeImpactMs: 28914, Score: 12078}
	run1 := RootCauseRankItem{Type: "runnable_wait", ImpactMs: 101, CumulativeImpactMs: 101, Score: 90}
	run2 := RootCauseRankItem{Type: "runnable_wait", ImpactMs: 60, CumulativeImpactMs: 60, Score: 55}
	// The span's Score (95) would beat run1's (90) on the raw Score channel —
	// exactly the cross-caliber race the S1 precedent forbids. Its SHARE
	// basis (83.893ms wall-clock ÷ window) sits below run1's (101ms).
	span := RootCauseRankItem{Type: "jit_compile", ImpactMs: 83.893, CumulativeImpactMs: 83.893, Score: 95,
		SpanName: "JIT compiling android.content.pm.Foo"}
	items := placeNonChainSemanticSpanRows(q, []RootCauseRankItem{agg, span, run1, run2})
	if items[1].Type != "runnable_wait" || items[2].Type != "jit_compile" {
		t.Fatalf("placement must follow the window-share basis (below the 101ms runnable), never raw Score: %+v", items)
	}
	if items[3].Type != "runnable_wait" || items[3].ImpactMs != 60 {
		t.Fatalf("the span must still sit above smaller-share rows: %+v", items)
	}
	// 无窗控制 (absence never guesses): without a bounded window there is no
	// share basis and the Score order stands.
	unmoved := placeNonChainSemanticSpanRows(Query{}, []RootCauseRankItem{agg, span, run1, run2})
	if unmoved[1].Type != "jit_compile" {
		t.Fatalf("without a window the Score order must stand: %+v", unmoved)
	}
}

func TestDCSBackgroundPlacementNeverCrossesOnChainTiers(t *testing.T) {
	q := Query{TimeStart: 1.0, TimeEnd: 1.101}
	onChain := RootCauseRankItem{Type: "io_wait", ImpactMs: 2, CumulativeImpactMs: 2, Score: 3,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain"}
	span := RootCauseRankItem{Type: "jit_compile", ImpactMs: 83.893, CumulativeImpactMs: 83.893, Score: 70,
		ChainRelevance: "background", Causality: "background"}
	items := placeNonChainSemanticSpanRows(q, []RootCauseRankItem{onChain, span})
	if items[0].Type != "io_wait" {
		t.Fatalf("a non-chain span must never place above an on-chain row: %+v", items)
	}
}

// --- E3: fail-loud caveat -------------------------------------------------------

func TestDCSSemanticSpanRankFailLoudCaveat(t *testing.T) {
	stats := WindowStats{TraceSpans: []TraceSpanSummary{
		{Name: "JIT compiling x", SemanticClass: "jit_compile", DurationMs: 5},
		{Name: "VerifyClass y", SemanticClass: "class_verification", DurationMs: 3},
		{Name: "Choreographer#doFrame", DurationMs: 9},
	}}
	// 正向: classified spans present, zero semantic rank rows published.
	caveat, ok := semanticSpanRankFailLoudCaveat(stats, []RootCauseRankItem{{Type: "runnable_wait"}})
	if !ok {
		t.Fatalf("expected the fail-loud caveat")
	}
	if !strings.Contains(caveat, "2 classified semantic optimization span(s)") ||
		!strings.Contains(caveat, "0 semantic rows") {
		t.Fatalf("caveat must carry the precise counts: %q", caveat)
	}
	// 负向 1: any published semantic row silences it.
	if _, ok := semanticSpanRankFailLoudCaveat(stats, []RootCauseRankItem{{Type: "jit_compile"}}); ok {
		t.Fatalf("a published semantic row must silence the caveat")
	}
	// 负向 2: no classified spans → silent.
	if _, ok := semanticSpanRankFailLoudCaveat(WindowStats{TraceSpans: []TraceSpanSummary{{Name: "generic", DurationMs: 4}}}, nil); ok {
		t.Fatalf("unclassified spans must not trigger the semantic fail-loud caveat")
	}
}

// --- E4: cross-window-boundary clipping + incomplete-pair caveat ----------------

func TestDCSCrossBoundarySpanMintsWindowClippedWithActualExtent(t *testing.T) {
	idx := buildTraceIndex(t, "dcs_boundary_span.systrace", `
     worker-200 (200) [002] .... 4.000000: tracing_mark_write: B|200|JIT compiling long com.example.Foo
     worker-200 (200) [002] .... 4.500000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 6.000000: tracing_mark_write: E|200
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.0, TimeEnd: 5.5})
	if len(stats.TraceSpans) != 1 {
		t.Fatalf("boundary-straddling span must mint (clipped), got %+v", stats.TraceSpans)
	}
	span := stats.TraceSpans[0]
	if span.StartTs != 5.0 || span.EndTs != 5.5 {
		t.Fatalf("span must be clipped to the query window: %+v", span)
	}
	if span.DurationMs < 499.9 || span.DurationMs > 500.1 {
		t.Fatalf("published duration must be the in-window projection: %+v", span)
	}
	if span.ActualStartTs != 4.0 || span.ActualEndTs != 6.0 || span.ActualDurationMs < 1999.9 {
		t.Fatalf("the physical B/E extent must ride the typed actual_* fields: %+v", span)
	}
	if span.SemanticClass != "jit_compile" {
		t.Fatalf("the clipped span must still classify: %+v", span)
	}
	// Control: a fully in-window span carries NO actual fields (absence is
	// the precise "not clipped" signal).
	inWindow := buildTraceIndex(t, "dcs_inwindow_span.systrace", `
     worker-200 (200) [002] .... 5.100000: tracing_mark_write: B|200|JIT compiling long com.example.Foo
     worker-200 (200) [002] .... 5.200000: tracing_mark_write: E|200
`)
	stats = ComputeWindowStats(inWindow, Query{TimeStart: 5.0, TimeEnd: 5.5})
	if len(stats.TraceSpans) != 1 || stats.TraceSpans[0].ActualDurationMs != 0 {
		t.Fatalf("fully in-window span must not carry actual_* fields: %+v", stats.TraceSpans)
	}
}

func TestDCSDanglingSemanticSpanCaveat(t *testing.T) {
	idx := buildTraceIndex(t, "dcs_dangling_semantic.systrace", `
     worker-200 (200) [002] .... 5.100000: tracing_mark_write: B|200|JIT compiling void a.b
     worker-201 (201) [002] .... 5.110000: tracing_mark_write: B|201|RandomPhaseWork
     worker-202 (202) [002] .... 5.120000: sched_switch: prev_comm=worker prev_pid=202 prev_prio=120 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.0, TimeEnd: 5.5})
	joined := strings.Join(stats.Caveats, "\n")
	if !strings.Contains(joined, "JIT compiling void a.b") || !strings.Contains(joined, "never closed") {
		t.Fatalf("dangling semantic-word-surface pair must caveat: %v", stats.Caveats)
	}
	// 负向: the non-semantic dangling name stays out of the caveat.
	if strings.Contains(joined, "RandomPhaseWork") {
		t.Fatalf("non-semantic dangling pairs must stay silent: %v", stats.Caveats)
	}
}

// --- E4 复核 F-1: lock lane carries the clipped/actual dual basis ----------------

// A monitor-contention span whose B side sits BEFORE the query window must
// publish the window-clipped duration on both lock-lane faces
// (critical_blocking candidate + blocking_span rank row) WITH its physical
// extent on the actual_* dual-basis lanes and the disclosure clause in the
// summaries — pre-F-1 the lane read only the clipped values and wrote
// "lasted <clipped>ms" with no hint the physical span was longer.
func TestDCSCrossBoundaryLockContentionCarriesActualExtent(t *testing.T) {
	trace := `
        aweme-41999 (41905) [002] .... 4.900000: print: B|41905|` + lockContentionCustomerMonitorPayload + `
          other-777 (777) [003] .... 5.010000: sched_wakeup: comm=NetworkKit_AssetsUtil_Operate_0 pid=42067 prio=120 target_cpu=003
        aweme-41999 (41905) [002] .... 5.113000: print: E|41905
	`
	idx := buildTraceIndex(t, "dcs_boundary_contention.systrace", trace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 5.0, TimeEnd: 5.3})
	if res.CriticalBlocking == nil {
		t.Fatalf("expected critical blocking result")
	}
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("monitor contention candidate missing: %+v", res.CriticalBlocking.Items)
	}
	if monitor.DurationMs < 112.9 || monitor.DurationMs > 113.1 {
		t.Fatalf("candidate duration must be the window-clipped projection (~113ms): %+v", monitor)
	}
	if monitor.ActualStartTs != 4.9 || monitor.ActualEndTs != 5.113 ||
		monitor.ActualDurationMs < 212.9 || monitor.ActualDurationMs > 213.1 {
		t.Fatalf("candidate must port the physical B/E extent (~213ms): %+v", monitor)
	}
	if !strings.Contains(monitor.Summary, "window-clipped; actual_span=213.000ms") {
		t.Fatalf("candidate summary must disclose the dual basis: %q", monitor.Summary)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 41905, TimeStart: 5.0, TimeEnd: 5.3, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var lockRow *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "blocking_span" {
			lockRow = &rank.Items[i]
			break
		}
	}
	if lockRow == nil {
		t.Fatalf("blocking_span rank row missing: %+v", rank.Items)
	}
	if lockRow.ActualImpactMs < 212.9 || lockRow.ActualImpactMs > 213.1 ||
		lockRow.ActualTotalMs < 212.9 || lockRow.ActualStartTs != 4.9 || lockRow.ActualEndTs != 5.113 {
		t.Fatalf("rank row must ride the actual_* dual-basis lanes (~213ms): %+v", lockRow)
	}
	if !strings.Contains(lockRow.Summary, "window-clipped, actual_span=213.000ms") {
		t.Fatalf("rank summary must disclose the dual basis: %q", lockRow.Summary)
	}
	// Control (既有精确信号): the fully in-window run keeps every actual_*
	// lane zero and both summaries free of the clipped clause.
	res = Run(idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.5, TimeEnd: 5.3})
	for i := range res.CriticalBlocking.Items {
		item := res.CriticalBlocking.Items[i]
		if item.BlockingKind != "monitor_contention" {
			continue
		}
		if item.ActualDurationMs != 0 || item.ActualStartTs != 0 || item.ActualEndTs != 0 {
			t.Fatalf("in-window contention must not carry actual_* fields: %+v", item)
		}
		if strings.Contains(item.Summary, "window-clipped") {
			t.Fatalf("in-window contention summary must stay clause-free: %q", item.Summary)
		}
	}
}

// --- 复核 F-2: background_rank JSON payload stability -----------------------------

// The background_rank FIELD is stamped on semantic compile span rows only —
// a semantic-free trace's marshalled rank payload must not contain the key at
// all (the tool's JSON payload face marshals this struct verbatim; pre-F-2
// every background row grew the field and broke byte stability for every
// trace).
func TestDCSBackgroundRankAbsentFromNonSemanticRankPayload(t *testing.T) {
	idx := buildTraceIndex(t, "dcs_json_stability.systrace", `
     bg00-300 (300) [000] .... 5.010000: sched_switch: prev_comm=bg00 prev_pid=300 prev_prio=120 prev_state=R ==> next_comm=idle/0 next_pid=0 next_prio=120
     bg00-300 (300) [000] .... 5.090000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=bg00 next_pid=300 next_prio=120
`)
	rank := BuildRootCauseRank(idx, Query{TimeStart: 5.0, TimeEnd: 5.1, Limit: 12})
	if len(rank.Items) == 0 {
		t.Fatalf("expected background rows: %+v", rank)
	}
	payload, err := json.Marshal(rank)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(payload), "background_rank") {
		t.Fatalf("semantic-free rank payload must stay byte-stable (no background_rank key):\n%s", payload)
	}
}

// --- E2: fall-through lane + 道别红线 -------------------------------------------

func TestDCSSameThreadNoOverlapSpanStaysNonChainAdjacent(t *testing.T) {
	// worker-200 is an on-chain node (it wakes the target at 5.006). The
	// FIRST span overlaps the chain-node window → on-chain lane; the SECOND
	// span sits on the SAME thread after the wakeup (no node/impact window
	// overlap) → mint-time non-chain, and enrichment's thread membership must
	// NOT flip it on-chain (道别=重叠谓词; the huadong E21 adjacent precedent).
	idx := buildTraceIndex(t, "dcs_lane_authority.systrace", `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000400: tracing_mark_write: B|200|VerifyClass com.example.Foo
     worker-200 (100) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 5.006200: tracing_mark_write: E|200
        app-100 (100) [001] .... 5.006500: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
     worker-200 (100) [002] .... 5.006600: tracing_mark_write: B|200|JIT compiling void tail.work
     worker-200 (100) [002] .... 5.006900: tracing_mark_write: E|200
	`)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var onChainSpan, tailSpan *RootCauseRankItem
	for i := range rank.Items {
		switch rank.Items[i].SpanName {
		case "VerifyClass com.example.Foo":
			onChainSpan = &rank.Items[i]
		case "JIT compiling void tail.work":
			tailSpan = &rank.Items[i]
		}
	}
	if onChainSpan == nil || onChainSpan.ChainRelevance != "on_chain" || onChainSpan.Tier != "primary" {
		t.Fatalf("the overlapping span must stay on-chain and participate in the primary election: %+v", onChainSpan)
	}
	if tailSpan == nil {
		t.Fatalf("the no-overlap span must still mint a typed row (E2 fall-through): %+v", rank.Items)
	}
	if tailSpan.Type != "jit_compile" {
		t.Fatalf("the fall-through row must be typed, never a generic trace_span: %+v", tailSpan)
	}
	if tailSpan.ChainRelevance == "on_chain" || tailSpan.Causality == "on_wakeup_chain" {
		t.Fatalf("道别红线: thread membership without overlap must never publish on-chain: %+v", tailSpan)
	}
	if tailSpan.ChainRelevance != "adjacent" {
		t.Fatalf("the same-thread no-overlap span keeps honest adjacency context: %+v", tailSpan)
	}
	if tailSpan.Tier == RootCauseTierDeterministicOptimization || tailSpan.Tier == "primary" {
		t.Fatalf("a non-chain span never wears the on-chain tier or primary: %+v", tailSpan)
	}
}

// --- E2E: cmp_01 6.0-shape fixture ----------------------------------------------

// TestDCSZeroChainSixteenSpanWindowEndToEnd is the witness-shape fixture
// (cust_trace_cmp_01.txt 6.0B138 window 8144.608–8144.708): 16 semantic
// compile spans hosted on NON-target processes inside a zero-chain window
// whose board is saturated by runnable/cpu-pressure noise. Pre-DCS every
// span died at the PID gate / the 12-seat cap and the rank published zero
// semantic rows with zero warning.
func TestDCSZeroChainSixteenSpanWindowEndToEnd(t *testing.T) {
	var b strings.Builder
	// Target process exists but never sleeps/wakes inside the window — the
	// chain degenerates to the target-only node (zero usable chain).
	b.WriteString("        app-100 (100) [001] .... 8144.609000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=R ==> next_comm=idle/1 next_pid=0 next_prio=120\n")
	// Board saturation: 10 background threads runnable for ~90ms each.
	// Keep every CPU lane monotonic: a grouped start/end pair per thread would
	// move a reused CPU backwards when the next sibling starts and correctly
	// trigger the scheduler-duration fail-closed gate.
	for i := 0; i < 10; i++ {
		pid := 300 + i
		cpu := i % 8
		fmt.Fprintf(&b, "     bg%02d-%d (%d) [00%d] .... 8144.610000: sched_switch: prev_comm=bg%02d prev_pid=%d prev_prio=120 prev_state=R ==> next_comm=idle/%d next_pid=0 next_prio=120\n",
			i, pid, pid, cpu, i, pid, cpu)
	}
	for i := 0; i < 10; i++ {
		pid := 300 + i
		cpu := i % 8
		fmt.Fprintf(&b, "     bg%02d-%d (%d) [00%d] .... 8144.700000: sched_switch: prev_comm=idle/%d prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=bg%02d next_pid=%d next_prio=120\n",
			i, pid, pid, cpu, cpu, i, pid)
	}
	b.WriteString("        app-100 (100) [001] .... 8144.700000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	// 16 semantic spans hosted on external processes (the cmp_01 E2 shape:
	// the biggest JIT lives in another process entirely).
	durations := []float64{83.893, 20.529, 19.294, 13.982, 6.727, 5.277, 5.132, 3.226, 2.865, 1.124, 0.903, 0.875, 0.701, 0.657, 0.584, 0.427}
	for i, ms := range durations {
		pid := 900 + i
		start := 8144.6100 + float64(i)*0.0001
		end := start + ms/1000
		fmt.Fprintf(&b, "     jit%02d-%d (%d) [007] .... %.6f: tracing_mark_write: B|%d|JIT compiling method%02d\n", i, pid, pid, start, pid, i)
		fmt.Fprintf(&b, "     jit%02d-%d (%d) [007] .... %.6f: tracing_mark_write: E|%d\n", i, pid, pid, end, pid)
	}
	idx := buildTraceIndex(t, "dcs_zero_chain_16_spans.systrace", b.String())
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 8144.608, TimeEnd: 8144.708, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	if len(rank.Items) > 12 {
		t.Fatalf("the 12-seat capacity must hold: %d rows", len(rank.Items))
	}
	var top *RootCauseRankItem
	semanticPublished := 0
	for i := range rank.Items {
		if rootCauseItemIsSemanticSpanWork(rank.Items[i]) {
			semanticPublished++
			if rank.Items[i].SpanName == "JIT compiling method00" {
				top = &rank.Items[i]
			}
		}
		if rank.Items[i].Type == "trace_span" && rank.Items[i].SemanticClass != "" {
			t.Fatalf("classified spans must never degrade to generic trace_span rows: %+v", rank.Items[i])
		}
	}
	// E1b 背景榜可见: the saturated board still publishes the top span via
	// the guaranteed background seat (mutation: plain [:limit] truncation
	// bites here).
	if semanticPublished == 0 {
		t.Fatalf("the background seat must keep at least one semantic span visible: %+v", rank.Items)
	}
	if top == nil {
		t.Fatalf("the TOP span by window share (83.893ms) must be the seated one: %+v", rank.Items)
	}
	if top.ChainRelevance == "on_chain" || top.Tier == RootCauseTierDeterministicOptimization || top.Tier == "primary" {
		t.Fatalf("an external-process zero-chain span stays on the non-chain lane: %+v", top)
	}
	if top.BackgroundRank <= 0 {
		t.Fatalf("the published span must carry its typed background board position: %+v", top)
	}
	// E6 按榜位提及判定: the aggregates/pressure/runnable rows above it hold
	// the top board positions, so the typed gate says NO prose mention
	// (background_rank>3) — exactly the §23.1 ruling-④ expectation for the
	// witness; the LEAD-SEM conclusion fallback still names it display-side.
	if top.BackgroundRank <= 3 {
		t.Fatalf("witness shape: the span must sit below the TOP-3 mention line, got background_rank=%d (board: %+v)", top.BackgroundRank, rank.Items)
	}
	// E3 negative: semantic rows published → no fail-loud caveat.
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "0 semantic rows") {
			t.Fatalf("fail-loud caveat must stay silent when semantic rows published: %q", caveat)
		}
	}
}
