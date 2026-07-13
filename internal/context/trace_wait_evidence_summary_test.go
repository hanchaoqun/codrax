package context

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
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

// TestTraceWaitEvidence_BlockedReasonFacts — the wait-object lane: per-caller
// symbol × count × Σms verbatim, the honest cause-unproven remainder, and the
// unconsumed window-marker lane.
func TestTraceWaitEvidence_BlockedReasonFacts(t *testing.T) {
	summary := formatTraceWaitWakeEvidenceFromLedger(traceWaitTestLedger(), nil)
	if summary == "" {
		t.Fatalf("blocked_reason typed notes must render a wait-evidence summary")
	}
	for _, want := range []string{
		"Kernel-recorded wait objects (sched_blocked_reason):",
		"CompThread_0-2955 — caller=dma_fence_default_w · d_state_or_io_wait 36.757ms · members=4",
		"caller=fscache_page_wait_o · io_wait 7.386ms · members=17",
		"cause-unproven remainder (no blocked_reason record backs this share) · io_wait 10.433ms · members=3",
		"window holds 6 blocked_reason record(s) (caller=fscache_page_get_an/hmfs_read)",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("wait-evidence summary missing %q:\n%s", want, summary)
		}
	}
	// The consumption preamble: wait-object questions answer with the caller
	// symbols verbatim; a cause-unproven share never gets an invented object.
	for _, want := range []string{
		"the kernel's own record is the blocked_reason caller symbol",
		"never invent one",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("wait-evidence preamble missing %q:\n%s", want, summary)
		}
	}
}

// TestTraceWaitEvidence_WakeupEdges — the waker lane: sched_wakeup edges with
// waker/wakee/timestamp; identical republications collapse; ts order.
func TestTraceWaitEvidence_WakeupEdges(t *testing.T) {
	summary := formatTraceWaitWakeEvidenceFromLedger(traceWaitTestLedger(), nil)
	if !strings.Contains(summary, "Measured wakeup edges (sched_wakeup; waker → wakee at timestamp):") {
		t.Fatalf("wakeup edge lane missing:\n%s", summary)
	}
	want := "- gpu-token-id4-2931 → CompThread_0-2955 at 13762.801234 (wakeup latency 0.123ms)"
	if got := strings.Count(summary, want); got != 1 {
		t.Fatalf("identical edge republications must collapse to one row (got %d):\n%s", got, summary)
	}
	// ts order: the upstream 13762.800001 edge precedes the 13762.801234 edge.
	up := strings.Index(summary, "binder:642_10-1385 → gpu-token-id4-2931 at 13762.800001")
	down := strings.Index(summary, want)
	if up < 0 || down < 0 || up > down {
		t.Fatalf("wakeup edges must render in timestamp order:\n%s", summary)
	}
	if !strings.Contains(summary, "answer with the sched_wakeup edge below: its waker thread and its wakeup timestamp") {
		t.Fatalf("the waker consumption preamble is missing:\n%s", summary)
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
		"kernel blocked_reason record census for THIS thread — total 21 blocked_reason record(s) in its selected window, use this total verbatim: fscache_page_wait_o ×17(Σ13.905ms) / hmfs_read ×1(Σ0.145ms) / hmfs_get_dnode ×1 / (+2 more caller symbol(s))",
		// The census keying is stated as a data label (the counter-face to
		// attributing a record to the thread whose line it printed on), and
		// the Σdelay caliber label is always-on (件C: self-reported delay=,
		// may include pre-window accumulation — Σ>窗长 forms stay honest).
		"keyed by the waiting thread itself",
		"self-reported delay= field and may include pre-window accumulation",
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
		"kernel blocked_reason record census for THIS thread: dma_fence_default_w ×12",
		"kworker/u16:3-357 — kernel blocked_reason record census for THIS thread: kthread_worker_fn ×11",
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
	if !strings.Contains(summary, "- anchor-thread-9999 — ") {
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

// TestTraceWaitEvidence_UnprovenRemainderFact — PROSE-RC ② (§29.57 残余):
// the cause-unproven remainder is a standalone NAMED fact line — verbatim
// seat value, zero recompute — carrying the typed partition property
// (cause shares = proven part only, disjoint, never subtracted against
// each other). Witness: prose minted "2.731ms" = 10.433 − 7.702, a
// cross-caliber subtraction against the very value the report published.
func TestTraceWaitEvidence_UnprovenRemainderFact(t *testing.T) {
	summary := formatTraceWaitWakeEvidenceFromLedger(traceWaitTestLedger(), nil)
	for _, want := range []string{
		"cause-unproven remainder fact for ThreadPoolForeg-60555: the io_wait cause-unproven share is 10.433ms",
		"this published value already IS the entire unproven share — use it verbatim",
		// 收尾件3 (冷读姊妹形 054419): the subtraction ban re-routed the
		// re-derivation urge into BINDING (the remainder seat moved whole
		// under the fscache caller's name) — the sister property closes the
		// binding direction too.
		"It has NO kernel-recorded caller and must never be attributed to any caller-named proven cause.",
		"disjoint from this remainder",
		"never subtract caller-share values from this remainder",
		// 复放新形 (tieba 052947): prose re-scoped 原因未证 onto ONE member
		// segment of the remainder — the typed membership property must be
		// stated whenever the seat publishes member_count.
		"Its 3 member segment(s) are ALL inside the unproven share together — no single member segment alone is the unproven part.",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("unproven remainder fact missing %q:\n%s", want, summary)
		}
	}
	// No remainder share on the thread → no fact line minted for it.
	if strings.Contains(summary, "cause-unproven remainder fact for CompThread_0-2955") {
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
	if !strings.Contains(capped, "cause-unproven remainder fact for capped-thread-77: the d_state cause-unproven share is 3.333ms") {
		t.Fatalf("the remainder fact must never fall to the caller cap:\n%s", capped)
	}
}

// TestBuildPromptContext_TraceWaitEvidenceSection — the summary rides the
// investigation and answer-rendering dispatches only (same stage gate as the
// CR-1 board summary).
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
			TraceWaitEvidence: "Kernel-recorded wait objects (sched_blocked_reason):\n- CompThread_0-2955 — caller=dma_fence_default_w",
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
