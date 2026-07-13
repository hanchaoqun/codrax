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
		"cause-unproven remainder (no blocked_reason record backs this share) · io_wait 10.433ms",
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
	ledger.Records = append(ledger.Records,
		traceWaitTestRecord("trace_query:t#blocked_reason_census:1", "ThreadPoolForeg-60555", "blocked_reason", "blocked_reason_census", "19",
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
		// Full per-caller enumeration with Σms, count desc, overflow tail.
		"kernel blocked_reason record census for THIS thread: fscache_page_wait_o ×17(Σ13.905ms) / hmfs_read ×1(Σ0.145ms) / hmfs_get_dnode ×1 / (+2 more caller symbol(s))",
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
