package context

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// trace_wait_evidence_summary_test.go — EVID-BR 件① pins (§29.55.4 F1/R2-F3,
// docs/design/real_trace_campaign_20260705.md, 2026-07-13): the typed kernel
// wait-object (sched_blocked_reason) + wakeup-source (sched_wakeup) evidence
// summary reaches the model face — donghu 四跑四答案 (the appendix carried
// dma_fence_default_w every run while the model never saw it) and the waker
// inversion (D-entry switch next_comm listed as "waker" 11/11; the real
// sched_wakeup source was gpu-token-id4-2931).

func traceWaitTestRecord(id, subject, object, predicate, value string, notes ...string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:        id,
		Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		Value:     value,
		RichNotes: notes,
	}
}

func traceWaitTestLedger() types.ObservationLedger {
	return types.ObservationLedger{Records: []types.ObservationRecord{
		// donghu shape: the D seat with the unanimous kernel caller symbol.
		traceWaitTestRecord("trace_query:t#root_cause_rank:1", "CompThread_0-2955", "d_state_or_io_wait", "root_cause_primary", "36.757",
			"rank=1", "effective_impact_ms=36.757",
			types.TraceNoteKeyMemberCount+"=4",
			types.TraceNoteKeyBlockedReasonCaller+"=dma_fence_default_w"),
		// tieba shape: a proof-partition cause seat + the honest
		// cause-unproven remainder seat on the same thread.
		traceWaitTestRecord("trace_query:t#root_cause_rank:2", "ThreadPoolForeg-60555", "io_wait", "root_cause_secondary", "7.386",
			"rank=2", "effective_impact_ms=7.386",
			types.TraceNoteKeyMemberCount+"=17",
			types.TraceNoteKeyBlockedReasonCaller+"=fscache_page_wait_o"),
		traceWaitTestRecord("trace_query:t#root_cause_rank:3", "ThreadPoolForeg-60555", "io_wait", "root_cause_tertiary", "10.433",
			"rank=3", "effective_impact_ms=10.433",
			types.TraceNoteKeyMemberCount+"=3",
			types.TraceNoteKeyDStateCauseUnprovenRemainder+"=true"),
		// unconsumed in-window markers (CR-3 件② P10 lane).
		traceWaitTestRecord("trace_query:t#root_cause_rank:4", ".ugc.aweme.lite-17267", "io_wait", "root_cause_target_self_state", "1.354",
			types.TraceNoteKeyBlockedReasonWindowCount+"=6",
			types.TraceNoteKeyBlockedReasonWindowCaller+"=fscache_page_get_an/hmfs_read"),
		// measured wakeup edges: one real waker + an identical republication.
		traceWaitTestRecord("trace_query:t#wakeup_chain_edge:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_chain_edge", "0.123",
			"wakeup_ts=13762.801234", "latency=0.123"),
		traceWaitTestRecord("trace_query:t#wakeup_chain_edge:2", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_chain_edge", "0.123",
			"wakeup_ts=13762.801234", "latency=0.123"),
		traceWaitTestRecord("trace_query:t#wakeup_chain_edge:3", "binder:642_10-1385", "gpu-token-id4-2931", "wakeup_chain_edge", "",
			"wakeup_ts=13762.800001"),
	}}
}

// TestTraceWaitEvidence_BlockedReasonFacts — the wait-call-site lane: per-caller
// symbol × count × Σms verbatim, the honest cause-unproven remainder, and the
// unconsumed window-marker lane.
func TestTraceWaitEvidence_BlockedReasonFacts(t *testing.T) {
	summary := formatTraceWaitWakeEvidenceFromLedger(traceWaitTestLedger(), nil)
	if summary == "" {
		t.Fatalf("blocked_reason typed notes must render a wait-evidence summary")
	}
	for _, want := range []string{
		"Kernel-recorded wait evidence (independent typed seats):",
		"Unbound thread-window inventory (separate context; never a ranked-seat attribute):",
		"subject=CompThread_0-2955; seat_type=blocked_reason_callsite; caller=dma_fence_default_w",
		"state=d_state_or_io_wait; value_ms=36.757; members=4",
		"subject=ThreadPoolForeg-60555; seat_type=blocked_reason_callsite; caller=fscache_page_wait_o",
		"state=io_wait; value_ms=7.386; members=17",
		"subject=ThreadPoolForeg-60555; seat_type=cause_unproven_remainder; caller=not_provided",
		"state=io_wait; value_ms=10.433; members=3",
		"subject=.ugc.aweme.lite-17267; seat_type=thread_window_record_inventory; blocked_reason_records=6; seat_binding=not_provided; rank_binding=not_provided; caller_roster=fscache_page_get_an/hmfs_read",
		"rank=#2; row_identity=trace_query:t#root_cause_rank:2",
		"rank=#3; row_identity=trace_query:t#root_cause_rank:3",
		"cross_seat_aggregation_authority=forbidden",
		"ranked_seat_transfer=forbidden; cross_section_binding=forbidden",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("wait-evidence summary missing %q:\n%s", want, summary)
		}
	}
	// The consumption preamble keeps the caller in its actual role: a kernel
	// wait call-site, never an inferred resource object or holder identity.
	for _, want := range []string{
		"Each row below is one independent typed seat",
		"sched_blocked_reason caller is only the kernel wait call-site/symbol recorded for that row",
		"resource, lock, owner, holder, and root-cause identity require a separate typed relation",
		"Thread-window record inventory and census rows describe that thread's selected window and bind to no individual cause seat",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("wait-evidence preamble missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "Kernel-recorded wait objects") {
		t.Fatalf("blocked_reason caller regressed to a resource-object role:\n%s", summary)
	}
	unboundAt := strings.Index(summary, "Unbound thread-window inventory (separate context; never a ranked-seat attribute):")
	if unboundAt < 0 {
		t.Fatalf("missing separate unbound inventory section:\n%s", summary)
	}
	if before := summary[:unboundAt]; strings.Contains(before, "seat_type=thread_window_record_inventory") ||
		strings.Contains(before, "seat_type=thread_window_blocked_reason_census") {
		t.Fatalf("unbound window inventory remained interleaved with ranked seats:\n%s", summary)
	}
	if after := summary[unboundAt:]; !strings.Contains(after, "seat_type=thread_window_record_inventory") {
		t.Fatalf("separate unbound inventory lost its original rows:\n%s", summary)
	}
}

func TestTraceWaitEvidence_FinalizerScopesUnboundInventoryByTypedQuestion(t *testing.T) {
	causal := &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeCausalDiagnosis,
	}}}
	if traceWaitEvidenceIncludeUnboundWindowInventory(types.StageFinalize, causal) {
		t.Fatal("causal-diagnosis finalizer must not receive detailed unbound census roster")
	}
	if !traceWaitEvidenceIncludeUnboundWindowInventory(types.StageExplore, causal) {
		t.Fatal("explorer must retain the lossless unbound inventory for investigation")
	}

	boundedReason := &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactRecordedReason},
	}}}
	if !traceWaitEvidenceIncludeUnboundWindowInventory(types.StageFinalize, boundedReason) {
		t.Fatal("explicit bounded recorded-reason lookup must retain the census")
	}
	boundedWaker := &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactDirectWaker},
	}}}
	if traceWaitEvidenceIncludeUnboundWindowInventory(types.StageFinalize, boundedWaker) {
		t.Fatal("direct-waker fact lookup does not request a blocked-reason census")
	}

	scoped := formatTraceWaitWakeEvidenceFromLedgerWithOptions(traceWaitTestLedger(), nil, traceWaitEvidenceSummaryOptions{})
	if strings.Contains(scoped, "Unbound thread-window inventory") || strings.Contains(scoped, "seat_type=thread_window_record_inventory") {
		t.Fatalf("scoped finalizer summary leaked unbound rows:\n%s", scoped)
	}
	for _, keep := range []string{
		"seat_type=blocked_reason_callsite; caller=fscache_page_wait_o",
		"seat_type=cause_unproven_remainder; caller=not_provided",
		"Measured wakeup edges (sched_wakeup",
	} {
		if !strings.Contains(scoped, keep) {
			t.Fatalf("scoping unbound inventory dropped ranked/wakeup evidence %q:\n%s", keep, scoped)
		}
	}
}

// TestTraceWaitEvidence_WakeupEdges — the waker lane: sched_wakeup edges with
// waker/wakee/timestamp; identical republications collapse; ts order.
func TestTraceWaitEvidence_WakeupEdges(t *testing.T) {
	summary := formatTraceWaitWakeEvidenceFromLedger(traceWaitTestLedger(), nil)
	if !strings.Contains(summary, "Measured wakeup edges (sched_wakeup; waker → wakee at timestamp; pre-wakeup wait is sleep/blocking start → sched_wakeup, never sched_wakeup → switch-in scheduling delay):") {
		t.Fatalf("wakeup edge lane missing:\n%s", summary)
	}
	want := "- gpu-token-id4-2931 → CompThread_0-2955 at 13762.801234 (pre-wakeup wait: sleep/blocking start → sched_wakeup 0.123ms; latency_caliber=sleep_start_to_sched_wakeup)"
	if got := strings.Count(summary, want); got != 1 {
		t.Fatalf("identical edge republications must collapse to one row (got %d):\n%s", got, summary)
	}
	// ts order: the upstream 13762.800001 edge precedes the 13762.801234 edge.
	up := strings.Index(summary, "binder:642_10-1385 → gpu-token-id4-2931 at 13762.800001")
	down := strings.Index(summary, want)
	if up < 0 || down < 0 || up > down {
		t.Fatalf("wakeup edges must render in timestamp order:\n%s", summary)
	}
	if !strings.Contains(summary, "When the question asks WHO woke a thread (and when), use the sched_wakeup edge below") {
		t.Fatalf("the waker consumption preamble is missing:\n%s", summary)
	}
	if strings.Contains(summary, "wakeup latency") {
		t.Fatalf("pre-wakeup sleep/blocking time must never be mislabeled as post-wakeup scheduler latency:\n%s", summary)
	}
}

// TestTraceWaitEvidence_SilentWithoutTypedNotes — zero-emission anti-noise:
// a trace run whose records carry no blocked_reason/wakeup-edge typed notes,
// and a non-trace run, both stay silent.
func TestTraceWaitEvidence_SilentWithoutTypedNotes(t *testing.T) {
	plain := types.ObservationLedger{Records: []types.ObservationRecord{
		traceWaitTestRecord("trace_query:t#root_cause_rank:1", "keva-1-17437", "sleep_wait", "root_cause_secondary", "3.399",
			"rank=2", "effective_impact_ms=3.399"),
	}}
	if got := formatTraceWaitWakeEvidenceFromLedger(plain, nil); got != "" {
		t.Fatalf("runs without wait/wakeup typed notes must stay silent: %q", got)
	}
	nonTrace := types.ObservationLedger{Records: []types.ObservationRecord{{
		ID: "current", Origin: types.AnswerEvidenceOriginCurrentSource, Producer: "read_file", Subject: "sym",
	}}}
	if got := formatTraceWaitWakeEvidenceFromLedger(nonTrace, nil); got != "" {
		t.Fatalf("non-trace runs must stay silent: %q", got)
	}
}

// TestTraceWaitEvidence_TypedCensusNote — 件1 census 根修: the PRIMARY
// census source is the engine's typed blocked_reason_census note (full
// per-caller enumeration 符号×count×Σms off the full accumulator — E1-F1:
// the banner arm fed top-1 callers only). When the note is present the
// banner-parse fallback stays OFF (the banner is a top-8 display view).
func TestTraceWaitEvidence_TypedCensusNote(t *testing.T) {
	ledger := traceWaitTestLedger()
	// Production invariant (BlockedReasonPIDCensus.Count): Value is the
	// pid's TOTAL in-window record count; the enumerated per-caller counts
	// (17+1+1=19) sum BELOW it exactly when CallerOverflow>0. The caller
	// enumeration is the per-pid TOP list, so a capped-out symbol never
	// holds more records than the smallest enumerated count (=1 here):
	// the two overflow symbols hold exactly one record each → total 21.
	ledger.Records = append(ledger.Records,
		traceWaitTestRecord("trace_query:t#blocked_reason_census:1", "ThreadPoolForeg-60555", "blocked_reason", "blocked_reason_census", "21",
			types.TraceNoteKeyBlockedReasonCensus+"=fscache_page_wait_o×17(Σ13.905ms)/hmfs_read×1(Σ0.145ms)/hmfs_get_dnode×1",
			types.TraceNoteKeyBlockedReasonCensusOverflow+"=2"),
		// Republication with a smaller stale count — per-symbol MAX wins.
		traceWaitTestRecord("trace_query:t#blocked_reason_census:2", "ThreadPoolForeg-60555", "blocked_reason", "blocked_reason_census", "5",
			types.TraceNoteKeyBlockedReasonCensus+"=fscache_page_wait_o×5"),
	)
	// A banner row that CONTRADICTS the note (display-truncation shape): the
	// fallback must not engage while the typed note is present.
	results := []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Summary: "- blocked_reason ThreadPoolForeg-60555 iowait=1 count=3 line=100 caller=fscache_page_wait_o+0x110/0x250[sysmgr.elf]\n" +
			"- blocked_reason ghost-thread-1 iowait=0 count=4 line=200 caller=ghost_sym+0x10[m.elf]\n",
	}}
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, results)
	for _, want := range []string{
		// Full per-caller enumeration with Σms, count desc, overflow tail —
		// and (PROSE-RC ③) the engine's own published total verbatim in the
		// lead, as a directly quotable count.
		"subject=ThreadPoolForeg-60555; seat_type=thread_window_blocked_reason_census; seat_binding=not_provided; rank_binding=not_provided; total_records=21; caller_census=fscache_page_wait_o ×17(Σ13.905ms) / hmfs_read ×1(Σ0.145ms) / hmfs_get_dnode ×1 / (+2 more caller symbol(s))",
		// The census keying is stated as a data label (the counter-face to
		// attributing a record to the thread whose line it printed on), and
		// the Σdelay caliber label is always-on (件C: self-reported delay=,
		// may include pre-window accumulation — Σ>窗长 forms stay honest).
		"describe that thread's selected window",
		"self-reported delay= field and may include pre-window accumulation",
		"seat_type=cause_unproven_remainder; caller=not_provided",
		"caller_share_relation=outside_disjoint; arithmetic_recomposition=forbidden; member_scope=this_seat_all_members; member_rebinding=forbidden",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("typed census summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "ghost-thread-1") || strings.Contains(summary, "ghost_sym") {
		t.Fatalf("the banner fallback must stay off while typed census notes exist:\n%s", summary)
	}
	if strings.Contains(summary, "×22") || strings.Contains(summary, "×5") {
		t.Fatalf("republication must MAX, never sum or regress:\n%s", summary)
	}
	// PROSE-RC ③: the stale republication total (Value=5) must never
	// regress (total 5) or sum (21+5=26) the published total.
	if strings.Contains(summary, "total 5 ") || strings.Contains(summary, "total 26 ") {
		t.Fatalf("census total must MAX across republications, never regress or sum:\n%s", summary)
	}
}

// TestTraceWaitEvidence_BannerCensusFallback — no typed census note in the
// ledger → the banner-parse fallback engages (degraded lane, count only;
// h2 冷读 witness: the pid=357 kthread_worker_fn rows PRINTED on the
// target's context lines never land on the target's own census line).
func TestTraceWaitEvidence_BannerCensusFallback(t *testing.T) {
	results := []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Summary: "- blocked_reason CompThread_0-2955 iowait=0 count=12 line=133 caller=dma_fence_default_w+0x260/0x4dc[devhost.elf]\n" +
			"- blocked_reason kworker/u16:3-357 iowait=0 count=11 line=1967 caller=kthread_worker_fn+0x14c/0x1ec[devhost.elf]\n",
	}, {
		ToolName: "trace_query", Success: true,
		// Republication of the same census row — MAX, never summed.
		Summary: "- blocked_reason CompThread_0-2955 iowait=0 count=12 line=133 caller=dma_fence_default_w+0x260/0x4dc[devhost.elf]\n",
	}}
	summary := formatTraceWaitWakeEvidenceFromLedger(traceWaitTestLedger(), results)
	for _, want := range []string{
		"subject=CompThread_0-2955; seat_type=thread_window_blocked_reason_census; seat_binding=not_provided; rank_binding=not_provided; caller_census=dma_fence_default_w ×12",
		"subject=kworker/u16:3-357; seat_type=thread_window_blocked_reason_census; seat_binding=not_provided; rank_binding=not_provided; caller_census=kthread_worker_fn ×11",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("fallback census summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "×24") {
		t.Fatalf("republication must never sum the census counts:\n%s", summary)
	}
	// The pid=357 symbol never lands on the target's own line.
	for _, line := range strings.Split(summary, "\n") {
		if strings.HasPrefix(line, "- CompThread_0-2955") && strings.Contains(line, "kthread_worker_fn") {
			t.Fatalf("another pid's census must not ride the target's line: %s", line)
		}
	}
	// PROSE-RC ③: the banner fallback carries per-bucket display counts
	// only — it must never mint a quotable total.
	if strings.Contains(summary, "use this total verbatim") {
		t.Fatalf("the banner fallback must not mint a census total:\n%s", summary)
	}
}

// TestTraceWaitEvidence_AnchorThreadNeverEvicted — 件5: the analysis
// target (target_window_states / tier=target_self_state) rides the P10
// marker lane with orderMS=0 and must NOT fall to the thread cap while
// higher-valued background threads fill it; the cap discloses overflow.
func TestTraceWaitEvidence_AnchorThreadNeverEvicted(t *testing.T) {
	ledger := traceWaitTestLedger()
	// The anchor: a target_window_states account + a window-marker-only
	// wait lane (orderMS stays 0 — the eviction witness shape).
	ledger.Records = append(ledger.Records,
		traceWaitTestRecord("trace_query:t#target_window_states", "anchor-thread-9999", "state_partition", "target_window_states", "114.940"),
		traceWaitTestRecord("trace_query:t#root_cause_rank:anchor", "anchor-thread-9999", "io_wait", "root_cause_target_self_state", "",
			types.TraceNoteKeyBlockedReasonWindowCount+"=3",
			types.TraceNoteKeyBlockedReasonWindowCaller+"=some_sym"),
	)
	// Eight background threads with big published values crowd the cap.
	for i := 0; i < 8; i++ {
		ledger.Records = append(ledger.Records,
			traceWaitTestRecord(fmt.Sprintf("trace_query:t#root_cause_rank:bg%d", i), fmt.Sprintf("bg-thread-%d", 100+i), "d_state", "root_cause_secondary", fmt.Sprintf("%d.000", 90-i),
				"effective_impact_ms="+fmt.Sprintf("%d.000", 90-i),
				types.TraceNoteKeyBlockedReasonCaller+"=bg_sym"),
		)
	}
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	if !strings.Contains(summary, "- subject=anchor-thread-9999; seat_type=thread_window_record_inventory") {
		t.Fatalf("the anchor thread must keep its seat past the cap:\n%s", summary)
	}
	if !strings.Contains(summary, "more threads with blocked_reason evidence") {
		t.Fatalf("the thread cap must disclose its overflow:\n%s", summary)
	}
}

// TestTraceWaitEvidence_EdgeCapHeadTailSampling — 件5: past the edge cap
// the selection samples the window HEAD and TAIL (答案常在窗尾 — the old
// earliest-12 cut dropped the tail wholesale) and the overflow discloses.
func TestTraceWaitEvidence_EdgeCapHeadTailSampling(t *testing.T) {
	ledger := traceWaitTestLedger()
	for i := 0; i < 20; i++ {
		ts := fmt.Sprintf("13762.%06d", 100000+i*1000)
		ledger.Records = append(ledger.Records,
			traceWaitTestRecord(fmt.Sprintf("trace_query:t#wakeup_chain_edge:x%d", i), fmt.Sprintf("waker-%d", i), "wakee-thread-7", "wakeup_chain_edge", "",
				"wakeup_ts="+ts),
		)
	}
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	if !strings.Contains(summary, "- waker-0 → wakee-thread-7 at 13762.100000") {
		t.Fatalf("the head edge must stay:\n%s", summary)
	}
	if !strings.Contains(summary, "- waker-19 → wakee-thread-7 at 13762.119000") {
		t.Fatalf("the tail edge must stay (head-tail sampling):\n%s", summary)
	}
	if !strings.Contains(summary, "more wakeup edges; head and tail of the window are sampled above") {
		t.Fatalf("the edge cap must disclose its overflow:\n%s", summary)
	}
}

// TestTraceWaitEvidence_WakerCountFacts — PROSE-RC ① (§29.57 残余, 2026-07-13):
// per-waker observed-edge counts are NAMED facts aggregated over the FULL
// deduplicated edge inventory — the witness summary self-counted "8×"
// against its own 12-row list. The count must cover edges past the row cap,
// collapse identical republications, order count-desc, and carry the
// observed-edge caliber label (never a whole-window caliber: the minted
// edge inventory is itself capped upstream).
func TestTraceWaitEvidence_WakerCountFacts(t *testing.T) {
	ledger := traceWaitTestLedger()
	// 12 more distinct-ts edges on the same waker → wakee pair. With the
	// base edge (whose identical republication collapses) the pair holds 13
	// observed edges while the row cap lists only 12 rows.
	for i := 0; i < 12; i++ {
		ts := fmt.Sprintf("13762.9%05d", i*100)
		ledger.Records = append(ledger.Records,
			traceWaitTestRecord(fmt.Sprintf("trace_query:t#wakeup_chain_edge:g%d", i), "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_chain_edge", "",
				"wakeup_ts="+ts))
	}
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	for _, want := range []string{
		// The named count covers the full inventory (13), not the 12 listed
		// rows, and not 14 (the identical republication collapsed).
		"- gpu-token-id4-2931 → CompThread_0-2955 ×13 observed wakeup edge(s)",
		"- binder:642_10-1385 → gpu-token-id4-2931 ×1 observed wakeup edge(s)",
		// Consumption + caliber label: quote, never re-count; observed-edge
		// caliber only.
		"use these counts verbatim instead of re-counting rows",
		"they count observed wakeup edges only",
		// The row cap itself still discloses.
		"more wakeup edges; head and tail of the window are sampled above",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("waker count facts missing %q:\n%s", want, summary)
		}
	}
	// count desc: the ×13 pair line precedes the ×1 pair line.
	big := strings.Index(summary, "×13 observed wakeup edge(s)")
	small := strings.Index(summary, "×1 observed wakeup edge(s)")
	if big < 0 || small < 0 || big > small {
		t.Fatalf("waker count lines must order count-desc:\n%s", summary)
	}
}

// TestTraceWaitEvidence_WakerCountCapOverflow — PROSE-RC ①: past the
// per-waker count cap the uncovered edges are disclosed as a NAMED
// remainder (counts + pairs), and equal-count ties order deterministically
// by waker then wakee.
func TestTraceWaitEvidence_WakerCountCapOverflow(t *testing.T) {
	ledger := types.ObservationLedger{}
	// One dominant pair (×4) …
	for i := 0; i < 4; i++ {
		ledger.Records = append(ledger.Records,
			traceWaitTestRecord(fmt.Sprintf("trace_query:t#wakeup_chain_edge:a%d", i), "waker-a", "wakee-1", "wakeup_chain_edge", "",
				fmt.Sprintf("wakeup_ts=13762.0%05d", i*10)))
	}
	// … plus nine ×1 pairs: 10 pairs total against a cap of 8 lines, so
	// 2 pairs (2 edges) fall to the named remainder. Inserted in REVERSE
	// lexicographic ts order (waker-j earliest) so the first-appearance /
	// ts order and the lexicographic tie key DISAGREE — the b..h roster
	// below holds only if the deterministic waker tie key does the work
	// (复核 F2: with aligned orders, deleting the tie key stayed green).
	for i := 0; i < 9; i++ {
		ledger.Records = append(ledger.Records,
			traceWaitTestRecord(fmt.Sprintf("trace_query:t#wakeup_chain_edge:b%d", i), fmt.Sprintf("waker-%c", 'j'-i), "wakee-1", "wakeup_chain_edge", "",
				fmt.Sprintf("wakeup_ts=13762.1%05d", i*10)))
	}
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	for _, want := range []string{
		"- waker-a → wakee-1 ×4 observed wakeup edge(s)",
		// Deterministic ties: waker-b .. waker-h fill the remaining 7 seats.
		"- waker-b → wakee-1 ×1 observed wakeup edge(s)",
		"- waker-h → wakee-1 ×1 observed wakeup edge(s)",
		// The named remainder: 2 uncovered edges across 2 unlisted pairs.
		"- (+2 more observed wakeup edge(s) across 2 more waker → wakee pair(s) beyond the counts above)",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("waker count cap overflow missing %q:\n%s", want, summary)
		}
	}
	// The evicted tie-tail pairs must not hold count lines.
	for _, banned := range []string{
		"- waker-i → wakee-1 ×1",
		"- waker-j → wakee-1 ×1",
	} {
		if strings.Contains(summary, banned) {
			t.Fatalf("pairs past the count cap must fall to the named remainder, found %q:\n%s", banned, summary)
		}
	}
}

// TestTraceWaitEvidence_UnprovenRemainderFact — the cause-unproven remainder
// is an independent typed seat. Compact enum fields preserve the partition,
// membership and arithmetic boundaries without co-locating sibling caller or
// census facts on the same subject line.
func TestTraceWaitEvidence_UnprovenRemainderFact(t *testing.T) {
	ledgerWithRepublication := traceWaitTestLedger()
	ledgerWithRepublication.Records = append(ledgerWithRepublication.Records,
		traceWaitTestRecord("trace_query:replay#root_cause_rank:3", "ThreadPoolForeg-60555", "io_wait", "root_cause_tertiary", "10.433",
			"rank=3", "effective_impact_ms=10.433",
			types.TraceNoteKeyMemberCount+"=3",
			types.TraceNoteKeyDStateCauseUnprovenRemainder+"=true"))
	summary := formatTraceWaitWakeEvidenceFromLedger(ledgerWithRepublication, nil)
	if got := strings.Count(summary, "subject=ThreadPoolForeg-60555; seat_type=cause_unproven_remainder"); got != 1 {
		t.Fatalf("idempotent query republications must keep one typed seat, got %d:\n%s", got, summary)
	}
	for _, want := range []string{
		"subject=ThreadPoolForeg-60555; seat_type=cause_unproven_remainder; caller=not_provided; caller_role=not_provided; blocking_reason_authority=not_provided_by_this_seat",
		"state=io_wait; value_ms=10.433; members=3",
		"value_scope=this_seat_entire_published_share",
		"caller_share_relation=outside_disjoint",
		"member_scope=this_seat_all_members",
		"member_rebinding=forbidden",
		"arithmetic_recomposition=forbidden",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("unproven remainder fact missing %q:\n%s", want, summary)
		}
	}
	for _, line := range strings.Split(summary, "\n") {
		if strings.Contains(line, "subject=ThreadPoolForeg-60555; seat_type=cause_unproven_remainder") {
			if strings.Contains(line, "fscache_page_wait_o") || strings.Contains(line, "caller_census=") {
				t.Fatalf("an unproven seat must not co-locate sibling caller/census evidence: %s", line)
			}
		}
		if strings.Contains(line, "seat_type=thread_window_blocked_reason_census") &&
			!strings.Contains(line, "seat_binding=not_provided") {
			t.Fatalf("a thread-window census must remain unbound to cause seats: %s", line)
		}
	}
	// No remainder share on the thread → no unproven seat minted for it.
	if strings.Contains(summary, "subject=CompThread_0-2955; seat_type=cause_unproven_remainder") {
		t.Fatalf("threads without an unproven share must not mint a remainder fact:\n%s", summary)
	}
	// The remainder fact never falls to the caller cap: push the unproven
	// seat past traceWaitEvidenceCallerCap proven seats.
	ledger := traceWaitTestLedger()
	for i := 0; i < 6; i++ {
		ledger.Records = append(ledger.Records,
			traceWaitTestRecord(fmt.Sprintf("trace_query:t#root_cause_rank:cap%d", i), "capped-thread-77", "d_state", "root_cause_secondary", fmt.Sprintf("%d.100", 20-i),
				"effective_impact_ms="+fmt.Sprintf("%d.100", 20-i),
				types.TraceNoteKeyBlockedReasonCaller+fmt.Sprintf("=cap_sym_%d", i)))
	}
	ledger.Records = append(ledger.Records,
		traceWaitTestRecord("trace_query:t#root_cause_rank:capr", "capped-thread-77", "d_state", "root_cause_secondary", "3.333",
			"effective_impact_ms=3.333",
			types.TraceNoteKeyDStateCauseUnprovenRemainder+"=true"))
	capped := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	if !strings.Contains(capped, "subject=capped-thread-77; seat_type=cause_unproven_remainder; caller=not_provided") ||
		!strings.Contains(capped, "state=d_state; value_ms=3.333") {
		t.Fatalf("the remainder fact must never fall to the caller cap:\n%s", capped)
	}
	if !strings.Contains(capped, "seat_type=blocked_reason_callsite_overflow; omitted_rows=1") {
		t.Fatalf("caller-seat overflow must remain explicit:\n%s", capped)
	}
	// A remainder WITHOUT a published member_count must not invent one.
	for _, line := range strings.Split(capped, "\n") {
		if strings.Contains(line, "subject=capped-thread-77; seat_type=cause_unproven_remainder") &&
			(strings.Contains(line, "members=") || strings.Contains(line, "member_scope=") || strings.Contains(line, "member_rebinding=")) {
			t.Fatalf("a member-less remainder must not mint a member count: %s", line)
		}
	}
}

// TestTraceWaitEvidence_WakeCensusCounts — WAKE-CENSUS (§29.58, 2026-07-13):
// with typed wakeup_edge_census records on the ledger the count lane mints
// from the census values VERBATIM (whole-inventory caliber + sched_wakeup
// direction pin + complete-list absence property), takes the per-pair MAX
// across republications, and the PROSE-RC ① fallback lane (observed-edges-
// only caliber) stays silent. PRC-F1 witness: the model invented
// 「OS_IPC_14_34911 ×4」for a pair whose only raw edge ran the OPPOSITE
// direction — the census count (12 vs the 3 fed edge rows) plus the absence
// sentence close both fabrication directions.
func TestTraceWaitEvidence_WakeCensusCounts(t *testing.T) {
	ledger := traceWaitTestLedger()
	ledger.Records = append(ledger.Records,
		// 修复轮 件4 (P3-1): the STALE lower count arrives FIRST — with the
		// low-count-LAST order the pinned ×12 was satisfiable by a first-wins
		// mutant too (M4 存活); this order splits MAX from first-wins
		// (first-wins reads ×9 here and reds).
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:1b", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "9",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.801234",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.955555"),
		// The engine census: 12 measured edges while only 3 edge rows were fed.
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "12",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.801234",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.998765"),
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:2", "binder:642_10-1385", "gpu-token-id4-2931", "wakeup_edge_census", "1",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.800001",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.800001"),
	)
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	for _, want := range []string{
		"Measured wakeup-edge counts per waker (full-inventory census:",
		// Census values verbatim — ×12 (MAX over the stale ×9 that arrived
		// FIRST), never the 3-row re-count of the fed edge list.
		"- gpu-token-id4-2931 → CompThread_0-2955 ×12 measured wakeup edge(s) (first at 13762.801234, last at 13762.998765)",
		"- binder:642_10-1385 → gpu-token-id4-2931 ×1 measured wakeup edge(s)",
		// Direction pin (PRC-F1 方向假) + absence property (PRC-F1 造数);
		// 件2 (P2-1) narrowed descriptive half = exactly the census caliber
		// (bundle runs hold engine-measured edges that published no per-pair
		// count, so the whole-run zero-edges form over-claimed); normative
		// half unchanged.
		"never reverse a pair's direction",
		"These pairs are the COMPLETE list of per-pair counted waker → wakee pairs from the wakeup-chain analyses above",
		"was never measured with a per-pair count here",
		"never report a wakeup count for an absent pair",
		// WC-F1: the scope label closes the invented-kernel-mechanism lane.
		"An absence here reflects only the measured set's scope — it is not a kernel scheduling behavior and needs no mechanism explanation.",
		// Honest caliber: measured edges only, the raw trace may hold more.
		"the raw trace may still hold wakeups outside the measured set",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("census count lane missing %q:\n%s", want, summary)
		}
	}
	// 件2 negative pin: the retired whole-run zero-edges claim never returns.
	if strings.Contains(summary, "ZERO measured wakeup edges in this run") {
		t.Fatalf("the whole-run zero-edges over-claim must stay retired:\n%s", summary)
	}
	// The fallback lane's caliber label must NOT co-render with the census.
	for _, banned := range []string{
		"observed wakeup edge(s)",
		"each count tallies ALL measured wakeup-edge records of this run",
	} {
		if strings.Contains(summary, banned) {
			t.Fatalf("the fallback count lane must stay silent when census records exist, found %q:\n%s", banned, summary)
		}
	}
	// count desc: ×12 precedes ×1.
	big := strings.Index(summary, "×12 measured wakeup edge(s)")
	small := strings.Index(summary, "×1 measured wakeup edge(s)")
	if big < 0 || small < 0 || big > small {
		t.Fatalf("census count lines must order count-desc:\n%s", summary)
	}
	// The stale republication's window bounds must not survive (whole-entry MAX).
	if strings.Contains(summary, "13762.955555") {
		t.Fatalf("a stale lower-count republication must not contribute its window bounds:\n%s", summary)
	}
}

// TestTraceWaitEvidence_WakeCensusWindowTotalCaliber — WAKE-CENSUS-D 2A
// (§29.58.4, RANK-U Stage 1 commit B, 2026-07-13): a census whose EVERY pair
// carries the typed exit split (2A provenance — count>0 always emits at least
// one split note) speaks the window-total caliber: the header names the raw
// direct-count source and the chain-thread wakee scope, each pair line carries
// the sleep/D/other exit split with the measurement-fact label, and the
// absence property strengthens to "ZERO raw sched_wakeup rows inside the
// analysis window" while KEEPING the scope sentence and the extended WC-F1
// label with the D-causality pointer (双重归因防护). The donghu waker witness
// form: gpu-token ×12 all D exits — the §29.58.4 structurally absent pair.
func TestTraceWaitEvidence_WakeCensusWindowTotalCaliber(t *testing.T) {
	ledger := traceWaitTestLedger()
	ledger.Records = append(ledger.Records,
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "12",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.801234",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.998765",
			types.TraceNoteKeyWakeupEdgeCensusDExit+"=12",
			types.TraceNoteKeySelectedWindow+"=13762.791708..13763.024898"),
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:2", "binder:642_10-1385", "gpu-token-id4-2931", "wakeup_edge_census", "1",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.800001",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.800001",
			types.TraceNoteKeyWakeupEdgeCensusSleepExit+"=1",
			types.TraceNoteKeySelectedWindow+"=13762.791708..13763.024898"),
	)
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	for _, want := range []string{
		"Measured wakeup counts per waker (window-total census:",
		"counted directly from the raw event inventory, independently of the causal-chain expansion",
		// The counted-scope sentence (范围句保留).
		"The counted wakee set is the chain's threads (analysis target and chain nodes) — wakees outside that set were not counted",
		// The donghu D-exit pair with its typed split (双加恒等式 12+0+0? no —
		// 0+12+0: zero-dropped sleep/other notes read back as 0).
		"- gpu-token-id4-2931 → CompThread_0-2955 ×12 raw wakeup(s) in the analysis window [exits: sleep=0, D-state/IO=12, other/unclassified=0 — measurement facts about which state the wakee left, never causal attribution] (first at 13762.801234, last at 13762.998765)",
		"- binder:642_10-1385 → gpu-token-id4-2931 ×1 raw wakeup(s) in the analysis window [exits: sleep=1, D-state/IO=0, other/unclassified=0",
		// The strong window-total absence property + scope + WC-F1 D pointer.
		"a pair absent from this list has ZERO raw sched_wakeup rows waking that counted wakee inside its analysis window (window-total caliber)",
		"Wakees OUTSIDE the chain-thread set were not counted — never claim any count, including zero, for them",
		"never causal attribution — for WHY a D-state/uninterruptible wait happened, read the sched_blocked_reason evidence, not this census",
		// 件1 (修复轮, 冷读 RU-F1): the census population PROPERTY sentence —
		// run6 witness extrapolated「整个窗口内所有配对中唯一大于 0 的 d_exit」
		// while 38 out-of-population D-exit pairs sat in the raw window. Both
		// languages + the explicit uniqueness/zero negative example.
		"- Census population property: this census counts wakeups of the chain-thread wakee set ONLY",
		"never call a listed count \"the only non-zero D-exit pair in the window\"",
		"never claim zero (or uniqueness) for any out-of-population pair",
		"本 census 种群=分析目标线程∪链节点线程;种群外线程之间的配对未测量——禁止据此作全窗/全部配对宣称(包括「窗口内唯一」「种群外为零」类)。",
		// 总数导语 (RANK-U Stage 1 复放实锤): the quotable per-wakee TOTAL —
		// both replay runs fabricated a derived total (12-vs-17 / 121-vs-29)
		// while quoting every per-pair count verbatim; a complete single-scope
		// window-total enumeration now publishes the additive total itself.
		"- TOTAL for wakee CompThread_0-2955 in window 13762.791708..13763.024898: 12 raw wakeup(s) across the 1 listed waker pair(s) [exits: sleep=0, D-state/IO=12, other/unclassified=0] — quote this total verbatim; never sum or subtract pair counts yourself.",
		"- TOTAL for wakee gpu-token-id4-2931 in window 13762.791708..13763.024898: 1 raw wakeup(s) across the 1 listed waker pair(s) [exits: sleep=1, D-state/IO=0, other/unclassified=0]",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("window-total census lane missing %q:\n%s", want, summary)
		}
	}
	// The legacy narrow forms must not co-render on a 2A-provenanced census.
	for _, banned := range []string{
		"full-inventory census",
		"measured wakeup edge(s) (first at",
		"was never measured with a per-pair count here",
	} {
		if strings.Contains(summary, banned) {
			t.Fatalf("legacy census wording must not render on 2A provenance, found %q:\n%s", banned, summary)
		}
	}
	// Cross-WINDOW pair gate: the SAME pair republished with a DIFFERENT
	// measured window keeps its MAX pair line but loses its TOTAL lead —
	// cross-window sums are unsound; wakees whose pairs stay one-window keep
	// their totals.
	mixed := traceWaitTestLedger()
	mixed.Records = append(mixed.Records, ledger.Records...)
	mixed.Records = append(mixed.Records,
		traceWaitTestRecord("trace_query:u#wakeup_edge_census:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "9",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.80",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.90",
			types.TraceNoteKeyWakeupEdgeCensusDExit+"=9",
			types.TraceNoteKeySelectedWindow+"=13762.800000..13762.900000"),
	)
	mixedSummary := formatTraceWaitWakeEvidenceFromLedger(mixed, nil)
	if strings.Contains(mixedSummary, "- TOTAL for wakee CompThread_0-2955") {
		t.Fatalf("a cross-window pair must suppress its wakee's TOTAL lead:\n%s", mixedSummary)
	}
	if !strings.Contains(mixedSummary, "- TOTAL for wakee gpu-token-id4-2931") {
		t.Fatalf("one-window wakees keep their TOTAL lead beside a cross-window sibling:\n%s", mixedSummary)
	}
	// Target-wakee completeness under scope overflow (the donghu shape: 83
	// pairs, 67 beyond the engine cap, all target pairs listed via cap
	// immunity): the pair carrying the per-RESULT target_wakee marker keeps
	// its TOTAL lead while a non-target wakee of the same overflowed scope
	// stays total-less.
	overflowed := traceWaitTestLedger()
	overflowed.Records = append(overflowed.Records,
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "12",
			types.TraceNoteKeyWakeupEdgeCensusDExit+"=12",
			types.TraceNoteKeyWakeupEdgeCensusTargetWakee+"=true",
			types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=67",
			types.TraceNoteKeyWakeupEdgeCensusOverflowEdges+"=206",
			types.TraceNoteKeySelectedWindow+"=13762.791708..13763.024898"),
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:2", "logd.writer-9163", "logd.reader.per-9522", "wakeup_edge_census", "57",
			types.TraceNoteKeyWakeupEdgeCensusSleepExit+"=56",
			types.TraceNoteKeyWakeupEdgeCensusOtherExit+"=1",
			types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=67",
			types.TraceNoteKeyWakeupEdgeCensusOverflowEdges+"=206",
			types.TraceNoteKeySelectedWindow+"=13762.791708..13763.024898"),
	)
	overflowSummary := formatTraceWaitWakeEvidenceFromLedger(overflowed, nil)
	if !strings.Contains(overflowSummary, "- TOTAL for wakee CompThread_0-2955 in window 13762.791708..13763.024898: 12 raw wakeup(s)") {
		t.Fatalf("the marked target wakee's TOTAL must survive scope overflow (cap-immune pair set):\n%s", overflowSummary)
	}
	if strings.Contains(overflowSummary, "- TOTAL for wakee logd.reader.per-9522") {
		t.Fatalf("a non-target wakee under scope overflow must stay total-less:\n%s", overflowSummary)
	}
	// 复核 F1 (修复轮 件2): the SESSION-global anchor flag must NOT vouch for
	// another result's trimmed pair set — same overflowed shape, marker
	// REMOVED, session anchor record PRESENT: no TOTAL may mint.
	crossScope := traceWaitTestLedger()
	crossScope.Records = append(crossScope.Records,
		// T1 marks CompThread as the session's anchor thread (件5 lane)…
		traceWaitTestRecord("trace_query:t1#target_window_states", "CompThread_0-2955", "state_partition", "target_window_states", "233.190"),
		// …while T2's census (overflowed, CompThread NOT its target) lists a
		// CompThread pair WITHOUT the per-result marker.
		traceWaitTestRecord("trace_query:t2#wakeup_edge_census:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "12",
			types.TraceNoteKeyWakeupEdgeCensusDExit+"=12",
			types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=67",
			types.TraceNoteKeyWakeupEdgeCensusOverflowEdges+"=206",
			types.TraceNoteKeySelectedWindow+"=13762.791708..13763.024898"),
	)
	crossScopeSummary := formatTraceWaitWakeEvidenceFromLedger(crossScope, nil)
	if strings.Contains(crossScopeSummary, "- TOTAL for wakee") {
		t.Fatalf("a session anchor must not vouch for another result's trimmed pair set:\n%s", crossScopeSummary)
	}
	// Same-window republication across TWO result scopes (the common
	// wakeup_chain + frame-bundle session shape): idempotent — the TOTAL
	// lead survives.
	repub := traceWaitTestLedger()
	repub.Records = append(repub.Records, ledger.Records...)
	repub.Records = append(repub.Records,
		traceWaitTestRecord("trace_query:v#wakeup_edge_census:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "12",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.801234",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.998765",
			types.TraceNoteKeyWakeupEdgeCensusDExit+"=12",
			types.TraceNoteKeySelectedWindow+"=13762.791708..13763.024898"),
	)
	repubSummary := formatTraceWaitWakeEvidenceFromLedger(repub, nil)
	if !strings.Contains(repubSummary, "- TOTAL for wakee CompThread_0-2955 in window 13762.791708..13763.024898: 12 raw wakeup(s)") {
		t.Fatalf("same-window republication must keep the TOTAL lead (idempotent):\n%s", repubSummary)
	}
	// Provenance gate (fail-open direction): ONE legacy pair without a split
	// demotes the whole lane back to the first-batch wording — window-total
	// may never over-claim an edge-fold count.
	ledger.Records = append(ledger.Records,
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:3", "legacy-waker-9", "wakee-9", "wakeup_edge_census", "2",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.5",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.6"),
	)
	summary = formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	if !strings.Contains(summary, "full-inventory census") ||
		strings.Contains(summary, "window-total census") {
		t.Fatalf("a split-less legacy record must demote the lane to the first-batch caliber:\n%s", summary)
	}
	// The quotable TOTAL is a window-total-only face: the demoted legacy lane
	// must not mint a definite-looking total over edge-fold counts.
	if strings.Contains(summary, "- TOTAL for wakee") {
		t.Fatalf("the per-wakee TOTAL must not render on legacy provenance:\n%s", summary)
	}
	// 件1: the population property sentence claims the window-total caliber's
	// population — the demoted legacy lane keeps its own weaker scope wording.
	if strings.Contains(summary, "Census population property") {
		t.Fatalf("the population property sentence must not render on legacy provenance:\n%s", summary)
	}
}

// TestTraceWaitEvidence_WakeCensusOverflow — a census pair-cap overflow
// (typed notes) suppresses the complete-list sentence and mints the named
// unlisted remainder instead — absence claims only on complete enumerations.
func TestTraceWaitEvidence_WakeCensusOverflow(t *testing.T) {
	ledger := traceWaitTestLedger()
	ledger.Records = append(ledger.Records,
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "12",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.801234",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.998765",
			types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=3",
			types.TraceNoteKeyWakeupEdgeCensusOverflowEdges+"=7"),
	)
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	if !strings.Contains(summary, "- (+3 more waker → wakee pair(s) carrying 7 more measured wakeup edge(s) are not listed here — their per-pair counts are unpublished, so never guess or invent a count for an unlisted pair)") {
		t.Fatalf("census overflow must mint the named unlisted remainder:\n%s", summary)
	}
	if strings.Contains(summary, "COMPLETE list of per-pair counted waker → wakee pairs") {
		t.Fatalf("an overflowing census must never claim completeness:\n%s", summary)
	}
}

// TestTraceWaitEvidence_WakeCensusListingCapFoldsIntoRemainder — a
// multi-query census union past the listing cap trims deterministically and
// folds the trimmed pairs (with their KNOWN counts) into the named
// remainder; completeness is not claimed.
func TestTraceWaitEvidence_WakeCensusListingCapFoldsIntoRemainder(t *testing.T) {
	ledger := types.ObservationLedger{}
	// 18 pairs against the listing cap of 16: the two lowest-order tie pairs
	// (waker-q, waker-r) fall to the remainder, carrying 2×2 edges.
	for i := 0; i < 18; i++ {
		waker := fmt.Sprintf("waker-%c", 'a'+i)
		ledger.Records = append(ledger.Records,
			traceWaitTestRecord(fmt.Sprintf("trace_query:t#wakeup_edge_census:%d", i+1), waker, "wakee-1", "wakeup_edge_census", "2",
				types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.000001",
				types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.900009"))
	}
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	for _, want := range []string{
		"- waker-a → wakee-1 ×2 measured wakeup edge(s)",
		"- waker-p → wakee-1 ×2 measured wakeup edge(s)",
		"- (+2 more waker → wakee pair(s) carrying 4 more measured wakeup edge(s) are not listed here",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("census listing-cap fold missing %q:\n%s", want, summary)
		}
	}
	for _, banned := range []string{
		"- waker-q → wakee-1",
		"- waker-r → wakee-1",
		"COMPLETE list of per-pair counted waker → wakee pairs",
	} {
		if strings.Contains(summary, banned) {
			t.Fatalf("pairs past the listing cap must fall to the remainder without a completeness claim, found %q:\n%s", banned, summary)
		}
	}
}

// TestTraceWaitEvidence_WakeCensusScopeUnionOverflow — 修复轮 件3 (P2-2):
// overflow disclosures are per-RESULT facts. Within ONE result scope a
// republication MAX-collapses (numeric remainder stays exact); across TWO
// result scopes the numbers are not soundly combinable, so the remainder
// line de-numberizes instead of minting a definite-looking union number.
func TestTraceWaitEvidence_WakeCensusScopeUnionOverflow(t *testing.T) {
	// Single scope, republished overflow 3/7 twice → MAX, never 6/14.
	single := types.ObservationLedger{Records: []types.ObservationRecord{
		traceWaitTestRecord("trace_query:a#wakeup_edge_census:1", "waker-a", "wakee-1", "wakeup_edge_census", "2",
			types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=3",
			types.TraceNoteKeyWakeupEdgeCensusOverflowEdges+"=7"),
		traceWaitTestRecord("trace_query:a#wakeup_edge_census:1", "waker-a", "wakee-1", "wakeup_edge_census", "2",
			types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=3",
			types.TraceNoteKeyWakeupEdgeCensusOverflowEdges+"=7"),
	}}
	summary := formatTraceWaitWakeEvidenceFromLedger(single, nil)
	if !strings.Contains(summary, "- (+3 more waker → wakee pair(s) carrying 7 more measured wakeup edge(s) are not listed here") {
		t.Fatalf("single-scope republication must keep the exact MAX remainder (3/7):\n%s", summary)
	}
	// Two scopes, each with its own overflow → the union remainder carries
	// no arithmetic; the normative half stays.
	multi := types.ObservationLedger{Records: []types.ObservationRecord{
		traceWaitTestRecord("trace_query:a#wakeup_edge_census:1", "waker-a", "wakee-1", "wakeup_edge_census", "2",
			types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=3",
			types.TraceNoteKeyWakeupEdgeCensusOverflowEdges+"=7"),
		traceWaitTestRecord("trace_query:b#wakeup_edge_census:1", "waker-b", "wakee-2", "wakeup_edge_census", "4",
			types.TraceNoteKeyWakeupEdgeCensusOverflowPairs+"=2",
			types.TraceNoteKeyWakeupEdgeCensusOverflowEdges+"=5"),
	}}
	summary = formatTraceWaitWakeEvidenceFromLedger(multi, nil)
	if !strings.Contains(summary, "- (additional measured waker → wakee pairs beyond those listed exist across the combined analyses — their per-pair counts are unpublished, so never guess or invent a count for an unlisted pair)") {
		t.Fatalf("a multi-scope union must de-numberize its remainder:\n%s", summary)
	}
	for _, banned := range []string{
		"more waker → wakee pair(s) carrying", // any definite union number
		"COMPLETE list of per-pair counted waker → wakee pairs",
	} {
		if strings.Contains(summary, banned) {
			t.Fatalf("multi-scope union must neither number the remainder nor claim completeness, found %q:\n%s", banned, summary)
		}
	}
}

// TestTraceWaitEvidence_WakeCensusBundleMixedAbsenceCaliber — 修复轮 件2
// (P2-1): bundle-mixed witness shape — the ledger holds a wakeup_chain PATH
// record naming a pair (engine-measured inside a frame_root_cause_bundle)
// that published neither an edge record nor a census count. The absence
// sentence must claim exactly the census caliber (never "zero measured
// wakeup edges in this run" — the model holds engine-measured pairs beyond
// the counted set), while the normative never-report half stays.
func TestTraceWaitEvidence_WakeCensusBundleMixedAbsenceCaliber(t *testing.T) {
	ledger := traceWaitTestLedger()
	ledger.Records = append(ledger.Records,
		// Complete census (zero overflow) for ONE pair …
		traceWaitTestRecord("trace_query:t#wakeup_edge_census:1", "gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "12",
			types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.801234",
			types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.998765"),
		// … while a bundle path record names ANOTHER measured pair with no
		// per-pair count anywhere.
		traceWaitTestRecord("trace_query:t#wakeup_chain:path:1", "CompThread_0-2955", "RSUniRenderThre-2188 -> CompThread_0-2955", "wakeup_chain", "",
			"branch=1"),
	)
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	for _, want := range []string{
		"These pairs are the COMPLETE list of per-pair counted waker → wakee pairs from the wakeup-chain analyses above",
		"was never measured with a per-pair count here",
		"never report a wakeup count for an absent pair",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("bundle-mixed absence sentence missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "ZERO measured wakeup edges in this run") {
		t.Fatalf("the whole-run zero-edges claim over-claims against bundle-measured pairs:\n%s", summary)
	}
}

// TestTraceWaitEvidence_WakeCensusFallbackSelfAdmits — with NO census record
// on the ledger the count lane keeps the PROSE-RC ① observed-edges-only
// caliber verbatim (降级自认) and never borrows the census wording.
func TestTraceWaitEvidence_WakeCensusFallbackSelfAdmits(t *testing.T) {
	summary := formatTraceWaitWakeEvidenceFromLedger(traceWaitTestLedger(), nil)
	if !strings.Contains(summary, "they count observed wakeup edges only") {
		t.Fatalf("the fallback lane must keep its observed-edges-only caliber label:\n%s", summary)
	}
	for _, banned := range []string{
		"full-inventory census",
		"measured wakeup edge(s) (first at",
		"COMPLETE list of per-pair counted waker → wakee pairs",
	} {
		if strings.Contains(summary, banned) {
			t.Fatalf("census-lane wording must not render without census records, found %q:\n%s", banned, summary)
		}
	}
}

// TestBuildPromptContext_TraceWaitEvidenceSection — the summary rides the
// investigation and answer-rendering dispatches only (same stage gate as the
// CR-1 board summary).
// --- INV-SUPPLY 件② (§29.61.11/.11a, 2026-07-14) -------------------------------

// traceWaitInvSupplySeatRecord builds the 090607 witness ➊ seat record shape:
// a seated inversion rank row carrying the gated split, the supply-fold
// notes and the witnessed thermal cap.
func traceWaitInvSupplySeatRecord() types.ObservationRecord {
	return traceWaitTestRecord("trace_query:t#root_cause_rank:9", "CompThread_0-2955", "priority_inversion_candidate", "root_cause_primary", "8.294",
		"rank=1", "effective_impact_ms=7.081",
		types.TraceNoteKeyPriorityInversionCandidate+"=true",
		types.TraceNoteKeyGatedRunnable+"=0.109",
		types.TraceNoteKeyGatedRunningDeficit+"=6.972",
		types.TraceNoteKeySupplyFoldDeficitMS+"=7.296",
		types.TraceNoteKeySupplyFoldIdealMS+"=0.998",
		types.TraceNoteKeyFoldBasis+"=known=8.294ms,unknown=0.000ms",
		types.TraceNoteKeyGovernanceCapKHz+"=1530000",
		types.TraceNoteKeyGovernanceCapMechanism+"=thermal_rail",
		types.TraceNoteKeyGovernanceCapWitnessed+"=true")
}

// TestTraceWaitEvidence_SeatCompositionFact — the compound seat's composition
// reaches the model face as ONE named fact: badge + subject + compound type
// word, both factors verbatim (反转等待 全额 + running 折算), the supply-gap
// lower bound and the witnessed thermal cap — 两因并提,引用勿推导.
func TestTraceWaitEvidence_SeatCompositionFact(t *testing.T) {
	ledger := traceWaitTestLedger()
	ledger.Records = append(ledger.Records, traceWaitInvSupplySeatRecord())
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	want := "- 席位构成(➊ CompThread_0-2955 优先级反转候选·供给缺口主导): 反转等待(全额) 0.109ms + running 折算 6.972ms(供给缺口 7.296ms 下界为主,明确热控轨上限 1.53GHz)——两因并提,引用勿推导"
	if !strings.Contains(summary, want) {
		t.Fatalf("seat composition fact missing/mutated:\n%s", summary)
	}
	if !strings.Contains(summary, "state BOTH factors together") {
		t.Fatalf("the section's consumption preamble is missing:\n%s", summary)
	}
	// 复放轮强化 (run-1 witness): the imperative is bilingual — the zh
	// anti-compression clause must ride the lead (EVID-1/PROSE-RC sister-
	// sentence discipline: the quoted answer language cannot lose it).
	if !strings.Contains(summary, "必须两因并提") || !strings.Contains(summary, "禁止把该席压缩为只提「优先级反转」的单因词形") {
		t.Fatalf("the zh anti-compression imperative is missing:\n%s", summary)
	}
	// QH2-B (§29.79 观察续档): the caliber words are bound INTO the named
	// fact — word+value quoted as one unit, the never-published near-synonym
	// named as the concrete wrong form. Bilingual (sister-sentence
	// discipline), word bytes from the tracefence Table ③c single source.
	for _, clause := range []string{
		"The caliber word attached to each value is PART of the fact",
		"quote the word and its value together exactly as printed (反转等待(全额) X / running 折算 Y / …下界)",
		"never replace a caliber word with a near-synonym this report does not publish (e.g. 满额 — the published word is 全额)",
		"口径词与数值同为具名事实:引用时连词带值整体照抄(「反转等待(全额) X」「running 折算 Y」「…下界」)",
		"禁止改写口径词,或以「满额」等未发布近义词替换「全额」等发布词",
	} {
		if !strings.Contains(summary, clause) {
			t.Fatalf("QH2-B caliber-word binding imperative clause missing %q:\n%s", clause, summary)
		}
	}
	// Cross-face containment: the feed's literal caliber words are members
	// of the tracefence Table ③c closed set (the answer-side audit's single
	// source), and the imperative's wrong-form example is the first
	// never-published word — the faces cannot drift apart.
	for _, word := range []string{tracefence.CaliberWordFullZH, tracefence.CaliberWordFoldedZH, tracefence.CaliberWordLowerBoundZH} {
		if !strings.Contains(want, word) && !strings.Contains(summary, word) {
			t.Fatalf("closed-set word %q missing from the seat feed face", word)
		}
	}
	if !strings.Contains(summary, tracefence.CaliberWordNeverPublishedZH()[0]) {
		t.Fatalf("the never-published example word must come from the Table ③c list")
	}
	// Identical republications collapse to one line.
	ledger.Records = append(ledger.Records, traceWaitInvSupplySeatRecord())
	if got := strings.Count(formatTraceWaitWakeEvidenceFromLedger(ledger, nil), "席位构成(➊ CompThread_0-2955"); got != 1 {
		t.Fatalf("identical seat republications must collapse (got %d lines)", got)
	}
}

// TestTraceWaitEvidence_SeatCompositionGates — the fact emits ONLY on the
// typed criterion (the SAME shared inequality the display compound judges):
// sub-threshold deficit, missing fold, missing running component and
// non-inversion seats all stay silent; the unwitnessed thermal cap keeps the
// honest 限压原因未见证 wording; a rank>5 seat wears #N instead of a badge.
func TestTraceWaitEvidence_SeatCompositionGates(t *testing.T) {
	render := func(mutate func(*types.ObservationRecord)) string {
		record := traceWaitInvSupplySeatRecord()
		mutate(&record)
		ledger := traceWaitTestLedger()
		ledger.Records = append(ledger.Records, record)
		return formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	}
	replaceNote := func(record *types.ObservationRecord, key, value string) {
		var notes []string
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, key+"=") {
				continue
			}
			notes = append(notes, note)
		}
		if value != "" {
			notes = append(notes, key+"="+value)
		}
		record.RichNotes = notes
	}
	// Sub-threshold (3.0 < 7.081×0.50): no fact line.
	if got := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeySupplyFoldDeficitMS, "3.000") }); strings.Contains(got, "席位构成") {
		t.Fatalf("sub-threshold seat must not feed a composition fact:\n%s", got)
	}
	// No fold basis → structurally out (criterion mirrors SupplyFoldComputed).
	if got := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyFoldBasis, "") }); strings.Contains(got, "席位构成") {
		t.Fatalf("a seat without a fold must not feed a composition fact:\n%s", got)
	}
	// No running component → the template cannot render (never fabricated).
	if got := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyGatedRunningDeficit, "") }); strings.Contains(got, "席位构成") {
		t.Fatalf("a seat without the running component must not feed the fact:\n%s", got)
	}
	// Non-inversion seat (pure running) → out (census conclusion: its own
	// type word already speaks the supply family).
	if got := render(func(r *types.ObservationRecord) {
		r.Object = "running"
		replaceNote(r, types.TraceNoteKeyPriorityInversionCandidate, "")
	}); strings.Contains(got, "席位构成") {
		t.Fatalf("a non-inversion seat must not feed the fact:\n%s", got)
	}
	// Unwitnessed thermal cap → honest wording, no 热限压 claim ON THE FACT
	// LINE (the bilingual section lead legitimately names the 热限压 factor).
	unwitnessed := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyGovernanceCapWitnessed, "false") })
	factLine := ""
	for _, line := range strings.Split(unwitnessed, "\n") {
		if strings.Contains(line, "席位构成(") {
			factLine = line
		}
	}
	if !strings.Contains(factLine, ",治理上限 1.53GHz(所选上限的窗内原因事件未见证))") || strings.Contains(factLine, "热控") || strings.Contains(factLine, "运行于") {
		t.Fatalf("unwitnessed cap must keep the honest wording:\n%s", unwitnessed)
	}
	// A witnessed generic cpu_frequency_limits ceiling remains policy-only:
	// it must not be upgraded to a thermal mechanism or observed frequency.
	policy := render(func(r *types.ObservationRecord) {
		replaceNote(r, types.TraceNoteKeyGovernanceCapMechanism, tracequery.SupplyFoldGovernanceCapPolicyLimit)
	})
	policyFactLine := ""
	for _, line := range strings.Split(policy, "\n") {
		if strings.Contains(line, "席位构成(") {
			policyFactLine = line
		}
	}
	if !strings.Contains(policyFactLine, ",策略频率上限 1.53GHz(不单独证明热机制或实际绑定影响))") {
		t.Fatalf("policy ceiling must carry its exact authority caveat:\n%s", policy)
	}
	for _, banned := range []string{"明确热控轨上限", "运行于 1.53GHz"} {
		if strings.Contains(policyFactLine, banned) {
			t.Fatalf("policy ceiling must not be promoted to %q:\n%s", banned, policyFactLine)
		}
	}
	// Absent thermal cap → no frequency clause at all.
	bare := render(func(r *types.ObservationRecord) {
		replaceNote(r, types.TraceNoteKeyGovernanceCapKHz, "")
		replaceNote(r, types.TraceNoteKeyGovernanceCapMechanism, "")
		replaceNote(r, types.TraceNoteKeyGovernanceCapWitnessed, "")
	})
	if !strings.Contains(bare, "(供给缺口 7.296ms 下界为主)") {
		t.Fatalf("capless seat must state the gap without a frequency claim:\n%s", bare)
	}
	// Runnable component absent → single-term composition (no fabricated 0)
	// ON THE FACT LINE (QH2-B: the bilingual section lead legitimately names
	// the 反转等待(全额) X quote pattern — same fact-line scoping as the
	// unwitnessed-cap pin above).
	single := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyGatedRunnable, "") })
	singleFactLine := ""
	for _, line := range strings.Split(single, "\n") {
		if strings.Contains(line, "席位构成(") {
			singleFactLine = line
		}
	}
	if !strings.Contains(singleFactLine, "): running 折算 6.972ms(") || strings.Contains(singleFactLine, "反转等待(全额)") {
		t.Fatalf("runnable-less seat must render the single running term:\n%s", single)
	}
	// Rank 6 → #6 head (badges are seats 1..5 only).
	rank6 := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyRank, "6") })
	if !strings.Contains(rank6, "席位构成(#6 CompThread_0-2955") || strings.Contains(rank6, "❻") {
		t.Fatalf("a rank-6 seat wears #6, never a badge:\n%s", rank6)
	}
}

// TestTraceWaitEvidence_SeatCompositionCapOverflow — 收尾件3 (P3-3): the
// seat-composition lane is capped at traceWaitEvidenceSeatCompositionCap
// lines in seat order; the remainder folds into ONE named overflow line
// (帽外具名余数,照 feed 惯例) — never silent, never an extra fact line.
func TestTraceWaitEvidence_SeatCompositionCapOverflow(t *testing.T) {
	ledger := traceWaitTestLedger()
	for i := 1; i <= traceWaitEvidenceSeatCompositionCap+2; i++ {
		record := traceWaitInvSupplySeatRecord()
		record.ID = fmt.Sprintf("trace_query:t#root_cause_rank:%d0", i)
		record.Subject = fmt.Sprintf("inv-thread-%d", i)
		var notes []string
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, "rank=") {
				note = fmt.Sprintf("rank=%d", i)
			}
			notes = append(notes, note)
		}
		record.RichNotes = notes
		ledger.Records = append(ledger.Records, record)
	}
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	if got := strings.Count(summary, "- 席位构成("); got != traceWaitEvidenceSeatCompositionCap {
		t.Fatalf("the fact lines must cap at %d (got %d):\n%s", traceWaitEvidenceSeatCompositionCap, got, summary)
	}
	if !strings.Contains(summary, "- (+2 more supply-gap-dominant seat(s); see the measured observations)") {
		t.Fatalf("the capped remainder must fold into one named overflow line:\n%s", summary)
	}
	// Seat order: the cap keeps the LOWEST ranks (seat order, not arrival).
	for i := 1; i <= traceWaitEvidenceSeatCompositionCap; i++ {
		if !strings.Contains(summary, fmt.Sprintf("inv-thread-%d ", i)) {
			t.Fatalf("seat %d must survive the cap in seat order:\n%s", i, summary)
		}
	}
	if strings.Contains(summary, fmt.Sprintf("inv-thread-%d ", traceWaitEvidenceSeatCompositionCap+1)) {
		t.Fatalf("a beyond-cap seat must never render its own fact line:\n%s", summary)
	}
}

func TestBuildPromptContext_TraceWaitEvidenceSection(t *testing.T) {
	for _, tc := range []struct {
		stage types.PipelineStage
		want  bool
	}{
		{types.StageExplore, true},
		{types.StageFinalize, true},
		{types.StageExtract, false},
	} {
		ac := &types.AgentContext{
			Stage:             tc.stage,
			TraceWaitEvidence: "Kernel-recorded wait call-sites (sched_blocked_reason):\n- CompThread_0-2955 — caller=dma_fence_default_w",
		}
		pc := BuildPromptContext(ac, &skill.Config{Name: "any-skill"})
		sec := findSectionTitle(pc, SectionTraceWaitEvidence)
		if tc.want && sec == nil {
			t.Fatalf("stage %s must carry the wait-evidence section", tc.stage)
		}
		if !tc.want && sec != nil {
			t.Fatalf("stage %s must not carry the wait-evidence section", tc.stage)
		}
		if tc.want && !strings.Contains(sec.Content, "dma_fence_default_w") {
			t.Fatalf("stage %s wait-evidence content lost: %q", tc.stage, sec.Content)
		}
	}
}

// --- FREQDIR-1 件2 (§29.149 修向②, 2026-07-19) --------------------------------

// traceWaitFreqDirSeatRecord builds the 95946 witness E1 seat record shape:
// the #1 NON-inversion chain running seat owning the 58.320ms supply-fold
// deficit with the witnessed thermal cap (run log
// codrax-20260719-123952-000-95946.log — the seat the inversion-gated
// composition arm structurally skipped).
func traceWaitFreqDirSeatRecord() types.ObservationRecord {
	return traceWaitTestRecord("trace_query:t#root_cause_rank:21", ".ugc.aweme.lite-17267", "running", "root_cause_primary", "157.248",
		"rank=1", "chain_relevance=on_chain", "effective_impact_ms=58.320",
		types.TraceNoteKeySupplyFoldDeficitMS+"=58.320",
		types.TraceNoteKeySupplyFoldIdealMS+"=98.928",
		types.TraceNoteKeyFoldBasis+"=known=157.248ms,unknown=0.000ms",
		types.TraceNoteKeyGovernanceCapKHz+"=1530000",
		types.TraceNoteKeyGovernanceCapMechanism+"=thermal_rail",
		types.TraceNoteKeyGovernanceCapWitnessed+"=true")
}

// TestTraceWaitEvidence_SupplyDeficitFact — FREQDIR-1 件2 positive pin: the
// non-inversion #1 running seat with a published deficit feeds ONE named
// fact carrying the deficit with its caliber words embedded (口径词嵌串防
// 加和) and the witnessed thermal cap — and NEVER a fabricated gated split.
func TestTraceWaitEvidence_SupplyDeficitFact(t *testing.T) {
	ledger := traceWaitTestLedger()
	ledger.Records = append(ledger.Records, traceWaitFreqDirSeatRecord())
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	t.Logf("FREQDIR-1 件2 witness named-fact render:\n%s", summary)
	want := "- 供给折算(➊ .ugc.aweme.lite-17267 running): 供给折算缺口 58.320ms(运行频点非最高,明确热控轨上限 1.53GHz)——独立折算口径,不与墙钟(全额)值相加、不计入四态合计;连口径词与数值整体照抄,勿推导"
	if !strings.Contains(summary, want) {
		t.Fatalf("supply-fold deficit fact missing/mutated, want\n%q\nin:\n%s", want, summary)
	}
	// The section preamble carries the bilingual anti-summing imperative.
	for _, clause := range []string{
		"Supply-fold deficit facts (typed, per-seat)",
		"never adds to any wall-clock (全额) value",
		"折算值不与任何墙钟(全额)值相加、不计入四态合计",
		"未列出的席位即未发布缺口,勿代算",
	} {
		if !strings.Contains(summary, clause) {
			t.Fatalf("supply-deficit preamble clause missing %q:\n%s", clause, summary)
		}
	}
	// 禁伪造 gated 拆分: the seat published no gated split, so its fact line
	// must carry neither split term and never the composition family lead.
	factLine := ""
	for _, line := range strings.Split(summary, "\n") {
		if strings.Contains(line, "供给折算(") {
			factLine = line
		}
	}
	for _, banned := range []string{"反转等待(全额)", "running 折算", "席位构成"} {
		if strings.Contains(factLine, banned) {
			t.Fatalf("the deficit fact must not fabricate a split (%q):\n%s", banned, factLine)
		}
	}
	// Identical republications collapse to one line.
	ledger.Records = append(ledger.Records, traceWaitFreqDirSeatRecord())
	if got := strings.Count(formatTraceWaitWakeEvidenceFromLedger(ledger, nil), "- 供给折算(➊"); got != 1 {
		t.Fatalf("identical seat republications must collapse (got %d lines)", got)
	}
}

// TestTraceWaitEvidence_SupplyDeficitFactSilence — FREQDIR-1 件2 silence
// pins (absence stays absent): no deficit note, a zero deficit, a missing
// fold basis, an adjacent-channel seat and a seatless row all publish
// NOTHING; a composition-arm seat never double-publishes on this lane; an
// unwitnessed thermal cap keeps the honest 限压原因未见证 wording.
func TestTraceWaitEvidence_SupplyDeficitFactSilence(t *testing.T) {
	render := func(mutate func(*types.ObservationRecord)) string {
		record := traceWaitFreqDirSeatRecord()
		mutate(&record)
		ledger := traceWaitTestLedger()
		ledger.Records = append(ledger.Records, record)
		return formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	}
	replaceNote := func(record *types.ObservationRecord, key, value string) {
		var notes []string
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, key+"=") {
				continue
			}
			notes = append(notes, note)
		}
		if value != "" {
			notes = append(notes, key+"="+value)
		}
		record.RichNotes = notes
	}
	if got := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeySupplyFoldDeficitMS, "") }); strings.Contains(got, "供给折算(") {
		t.Fatalf("a seat without a deficit note must stay silent:\n%s", got)
	}
	if got := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeySupplyFoldDeficitMS, "0.000") }); strings.Contains(got, "供给折算(") {
		t.Fatalf("a zero deficit must stay silent (no 运行频点非最高 minting):\n%s", got)
	}
	if got := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyFoldBasis, "") }); strings.Contains(got, "供给折算(") {
		t.Fatalf("a seat without a fold must stay silent:\n%s", got)
	}
	if got := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyChainRelevance, "adjacent") }); strings.Contains(got, "供给折算(") {
		t.Fatalf("an adjacent-channel seat must stay off the chain lane:\n%s", got)
	}
	if got := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyRank, "") }); strings.Contains(got, "供给折算(") {
		t.Fatalf("a seatless row must stay silent:\n%s", got)
	}
	// A composition-arm seat (inversion + gated split + dominant deficit)
	// keeps its 席位构成 fact and never double-publishes here.
	ledger := traceWaitTestLedger()
	ledger.Records = append(ledger.Records, traceWaitInvSupplySeatRecord())
	summary := formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	if !strings.Contains(summary, "席位构成(➊ CompThread_0-2955") || strings.Contains(summary, "供给折算(") {
		t.Fatalf("a composition-fed seat must not double-publish a deficit fact:\n%s", summary)
	}
	// 返工 P1 (双复核对抗官探针形): a SUB-DOMINANT inversion seat — eff=20 =
	// 反转等待(全额)15 + running折算5, deficit 5 < 0.5×20 — fails the
	// composition arm's dominance gate AND must stay OFF the deficit lane
	// (!inversion): on an inversion row the deficit IS the counted running
	// component (同源同值, §29.88.12 R5 retired the 独立口径 face there), so
	// re-minting 「独立折算口径,不与墙钟(全额)值相加」 here would revive the
	// retired lie. Silence on BOTH lanes is the honest outcome.
	subDominant := traceWaitTestRecord("trace_query:t#root_cause_rank:23", "InvProbe-1234", "priority_inversion_candidate", "root_cause_secondary", "20.000",
		"rank=2", "chain_relevance=on_chain", "effective_impact_ms=20.000",
		types.TraceNoteKeyPriorityInversionCandidate+"=true",
		types.TraceNoteKeyGatedRunnable+"=15.000",
		types.TraceNoteKeyGatedRunningDeficit+"=5.000",
		types.TraceNoteKeySupplyFoldDeficitMS+"=5.000",
		types.TraceNoteKeyFoldBasis+"=known=5.000ms,unknown=0.000ms",
		types.TraceNoteKeyGovernanceCapKHz+"=1530000",
		types.TraceNoteKeyGovernanceCapMechanism+"=thermal_rail",
		types.TraceNoteKeyGovernanceCapWitnessed+"=true")
	invLedger := traceWaitTestLedger()
	invLedger.Records = append(invLedger.Records, subDominant)
	invSummary := formatTraceWaitWakeEvidenceFromLedger(invLedger, nil)
	if strings.Contains(invSummary, "席位构成(") || strings.Contains(invSummary, "供给折算(") {
		t.Fatalf("a sub-dominant inversion seat must stay silent on BOTH supply lanes:\n%s", invSummary)
	}
	// Same discipline for an inversion seat missing the gated-split note
	// entirely (dominant deficit, no runningRaw): the composition arm cannot
	// render and the deficit lane must not catch the fall-through.
	splitless := traceWaitTestRecord("trace_query:t#root_cause_rank:24", "InvProbe-5678", "priority_inversion_candidate", "root_cause_secondary", "8.000",
		"rank=3", "chain_relevance=on_chain", "effective_impact_ms=8.000",
		types.TraceNoteKeyPriorityInversionCandidate+"=true",
		types.TraceNoteKeySupplyFoldDeficitMS+"=7.000",
		types.TraceNoteKeyFoldBasis+"=known=8.000ms,unknown=0.000ms")
	invLedger = traceWaitTestLedger()
	invLedger.Records = append(invLedger.Records, splitless)
	invSummary = formatTraceWaitWakeEvidenceFromLedger(invLedger, nil)
	if strings.Contains(invSummary, "席位构成(") || strings.Contains(invSummary, "供给折算(") {
		t.Fatalf("a split-less inversion seat must stay silent on BOTH supply lanes:\n%s", invSummary)
	}
	// Cross-record coverage: a republication of the SAME seat missing the
	// gated split must not re-enter through the deficit lane.
	republished := traceWaitInvSupplySeatRecord()
	republished.ID = "trace_query:t#root_cause_rank:22"
	var notes []string
	for _, note := range republished.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyGatedRunningDeficit+"=") {
			continue
		}
		notes = append(notes, note)
	}
	republished.RichNotes = notes
	ledger.Records = append(ledger.Records, republished)
	summary = formatTraceWaitWakeEvidenceFromLedger(ledger, nil)
	if strings.Contains(summary, "供给折算(") {
		t.Fatalf("a composition-covered seat republication must not re-enter the deficit lane:\n%s", summary)
	}
	// Unwitnessed thermal cap → the honest wording on the fact line.
	unwitnessed := render(func(r *types.ObservationRecord) { replaceNote(r, types.TraceNoteKeyGovernanceCapWitnessed, "false") })
	if !strings.Contains(unwitnessed, ",治理上限 1.53GHz(所选上限的窗内原因事件未见证))") {
		t.Fatalf("unwitnessed cap must keep the honest wording:\n%s", unwitnessed)
	}
}
