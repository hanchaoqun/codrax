package tool

// RTC-2 display pins (docs/design/real_trace_campaign_20260705.md §4 案 e2,
// 批 #67): on a ≥2-partition comparison whose typed time-base spans are
// pairwise disjoint (e2: 34579.x vs 2942.x), BOTH comparison surfaces grow a
// soft-guidance row in lockstep —
//   (a) the comparison-overview table closes with the disjoint time-base
//       note row (labels + verbatim envelopes + user-word guidance);
//   (b) the CMP-6 next-step comparison lane appends the corresponding
//       guidance row.
// Zero emission everywhere else: single partition, ANY span intersection, or
// ANY span-less projection (fail closed). No hard gate consumes the signal —
// the overview table and the fixed comparison rows render regardless.
// Inverting the disjointness comparison flips the positive pins (rows vanish)
// AND the negative pins (rows appear on overlap) — mutation must go red.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// rtc2TwoTraceObs mirrors the e2 shape: the donghu long capture
// (34579.450627..34579.595184) vs the short excerpt (2942.244845..2942.245401)
// — completely unrelated clock bases. Span/selected_window for artifact B are
// parameterized so the negative cases can pull it onto A's time base.
func rtc2TwoTraceObs(bStart, bEnd float64, bNotes ...string) []types.ObservationRecord {
	aNotes := []string{"rank=1", "tier=primary", "chain_relevance=on_chain",
		"causality=on_wakeup_chain", "dominant_state=running",
		"selected_window=34579.450627..34579.595184"}
	bBase := []string{"rank=1", "tier=primary", "chain_relevance=on_chain",
		"causality=on_wakeup_chain", "dominant_state=running"}
	return []types.ObservationRecord{
		compareProjObs("a-run", "donghu.systrace", "root_cause_primary", "root_cause_primary:a-run",
			"main-59566", "running", "144.557", 144.557, 100, 200,
			types.ObservationSpan{LineStart: 100, LineEnd: 200, StartTs: 34579.450627, EndTs: 34579.595184},
			aNotes...),
		compareProjObs("b-run", "donghu_short.systrace", "root_cause_primary", "root_cause_primary:b-run",
			"ColdPool-6", "running", "0.367", 0.367, 10, 20,
			types.ObservationSpan{LineStart: 10, LineEnd: 20, StartTs: bStart, EndTs: bEnd},
			append(bBase, bNotes...)...),
	}
}

func rtc2Bus(obs []types.ObservationRecord) *types.BusContext {
	bus := compareProjBus(false)
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	return bus
}

// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 两工件/各工件 → 两份 trace/各份 trace;⚠ note moved from table Items into block Text lines (对比总览族)
const (
	rtc2TableNoteZH = "⚠ 两份 trace 时间基准不相交(donghu.systrace 34579.451s→34579.595s,donghu_short.systrace 2942.245s→2942.245s),不可直接在同一时间轴对齐;对比请以各自窗口内相对指标为准"
	rtc2TableNoteEN = "⚠ The two trace files' time bases do not overlap (donghu.systrace 34579.451s→34579.595s, donghu_short.systrace 2942.245s→2942.245s); they cannot be aligned directly on one shared timeline — compare relative metrics within each trace file's own window"
	rtc2StepZH      = "两 trace 时间基准不相交,无法在同一时间轴直接对齐;对比请以各自窗口内相对指标为准(占窗比例/按窗长归一化)"
	rtc2StepEN      = "The two traces' time bases do not overlap and cannot be aligned directly on one shared timeline; compare relative metrics within each trace's own window (window share / normalized by window length)"
)

func rtc2NextStepBlock(t *testing.T, doc *types.AnswerDocumentV2) *types.AnswerBlock {
	t.Helper()
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == "next_steps" {
			return &doc.Blocks[i]
		}
	}
	return nil
}

func TestTraceProjectionDisjointTimeBaseRowsRTC2ChineseBothSurfaces(t *testing.T) {
	got := compareProjApply(t, rtc2Bus(rtc2TwoTraceObs(2942.244845, 2942.245401,
		"selected_window=2942.244845..2942.245401")))
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil || len(compare.Items) < 2 {
		t.Fatalf("two disjoint partitions must render the overview with both data rows: %+v", compare)
	}
	// PTV8-RCR-B: the ⚠ note now renders as its own block-Text line, not a
	// table row.
	noteLine := false
	for _, line := range strings.Split(compare.Text, "\n") {
		if line == rtc2TableNoteZH {
			noteLine = true
		}
	}
	if !noteLine {
		t.Fatalf("the block text must carry the verbatim ZH disjoint note on its own line:\n got: %q\nwant: %q",
			compare.Text, rtc2TableNoteZH)
	}
	for _, item := range compare.Items {
		if strings.Contains(strings.Join(item.Cells, " "), "时间基准") {
			t.Fatalf("the disjoint note must no longer occupy a table row: %+v", item.Cells)
		}
	}
	// Lockstep with the CMP-6 directive surface: the SAME document carries the
	// next-step guidance row (对比行⟺总览表, extended to the disjoint pair of
	// rows by construction — one typed signal, two surfaces).
	next := rtc2NextStepBlock(t, got)
	if next == nil {
		t.Fatalf("comparison shape must materialize the next-step block: %+v", got.Blocks)
	}
	found := false
	for _, item := range next.Items {
		if item.Text == rtc2StepZH {
			found = true
		}
	}
	if !found {
		t.Fatalf("the next-step lane must carry the verbatim ZH disjoint guidance row: %+v", next.Items)
	}
}

func TestTraceProjectionDisjointTimeBaseRowsRTC2EnglishBothSurfaces(t *testing.T) {
	bus := rtc2Bus(rtc2TwoTraceObs(2942.244845, 2942.245401,
		"selected_window=2942.244845..2942.245401"))
	bus.AnalysisIR.AnswerContract.Language = "en"
	got := compareProjApply(t, bus)
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil || len(compare.Items) < 2 {
		t.Fatalf("EN surface must render the overview with both data rows: %+v", compare)
	}
	// PTV8-RCR-B: EN note wording unchanged, but it now lives on its own
	// block-Text line instead of a table row.
	noteLine := false
	for _, line := range strings.Split(compare.Text, "\n") {
		if line == rtc2TableNoteEN {
			noteLine = true
		}
	}
	if !noteLine {
		t.Fatalf("EN block text must carry the verbatim disjoint note on its own line:\n got: %q\nwant: %q",
			compare.Text, rtc2TableNoteEN)
	}
	for _, item := range compare.Items {
		if strings.Contains(strings.Join(item.Cells, " "), "time bases") {
			t.Fatalf("EN disjoint note must no longer occupy a table row: %+v", item.Cells)
		}
	}
	next := rtc2NextStepBlock(t, got)
	if next == nil {
		t.Fatalf("comparison shape must materialize the next-step block: %+v", got.Blocks)
	}
	found := false
	for _, item := range next.Items {
		if item.Text == rtc2StepEN {
			found = true
		}
	}
	if !found {
		t.Fatalf("the EN next-step lane must carry the verbatim disjoint guidance row: %+v", next.Items)
	}
}

func rtc2AssertNoDisjointRows(t *testing.T, got *types.AnswerDocumentV2, shape string) {
	t.Helper()
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil {
		t.Fatalf("%s: the overview table itself must keep rendering (no hard gate): %+v", shape, got.Blocks)
	}
	for _, item := range compare.Items {
		if strings.Contains(strings.Join(item.Cells, " "), "时间基准") {
			t.Fatalf("%s: the disjoint note row must not render: %+v", shape, item.Cells)
		}
	}
	// PTV8-RCR-B: notes migrated into block Text — zero emission must hold
	// there too.
	if strings.Contains(compare.Text, "时间基准") {
		t.Fatalf("%s: the disjoint note must not render in the block text: %q", shape, compare.Text)
	}
	next := rtc2NextStepBlock(t, got)
	if next == nil || len(next.Items) == 0 ||
		!strings.Contains(next.Items[0].Text, "对比两 trace") {
		t.Fatalf("%s: the fixed comparison next-step rows must keep rendering: %+v", shape, next)
	}
	for _, item := range next.Items {
		if strings.Contains(item.Text, "时间基准") {
			t.Fatalf("%s: the disjoint guidance row must not render: %+v", shape, next.Items)
		}
	}
}

func TestTraceProjectionDisjointTimeBaseRowsAbsentOnOverlappingSpans(t *testing.T) {
	// Artifact B pulled onto A's time base (34579.500..34579.600 overlaps
	// 34579.450627..34579.595184) → intersection non-empty → zero emission on
	// BOTH surfaces while every other comparison surface stays.
	got := compareProjApply(t, rtc2Bus(rtc2TwoTraceObs(34579.500, 34579.600,
		"selected_window=34579.500000..34579.600000")))
	rtc2AssertNoDisjointRows(t, got, "overlapping spans")
}

func TestTraceProjectionDisjointTimeBaseRowsFailClosedOnSpanlessProjection(t *testing.T) {
	// Artifact B compiles ACTIVE but carries NO time evidence (line span only,
	// no selected_window) → the time base is UNKNOWN, never assumed disjoint.
	got := compareProjApply(t, rtc2Bus(rtc2TwoTraceObs(0, 0)))
	rtc2AssertNoDisjointRows(t, got, "span-less projection")
}

func TestRuntimeTraceProjCompareDisjointTimeBaseNoteMultiArtifactWording(t *testing.T) {
	spanProjection := func(label string, start, end float64) types.TraceCausalProjection {
		return types.TraceCausalProjection{
			ArtifactLabel: label,
			WindowStartTs: start,
			WindowEndTs:   end,
		}
	}
	three := []types.TraceCausalProjection{
		spanProjection("a.systrace", 100, 200),
		spanProjection("b.systrace", 300, 400),
		spanProjection("c.systrace", 500, 600),
	}
	zh := runtimeTraceProjCompareDisjointTimeBaseNote(three, true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 各工件 → 各份 trace (对比总览族)
	if !strings.HasPrefix(zh, "⚠ 各份 trace 时间基准两两不相交(a.systrace 100.000s→200.000s,b.systrace 300.000s→400.000s,c.systrace 500.000s→600.000s)") ||
		!strings.HasSuffix(zh, "不可直接在同一时间轴对齐;对比请以各自窗口内相对指标为准") {
		t.Fatalf(">2 partitions must use the pairwise ZH wording: %q", zh)
	}
	en := runtimeTraceProjCompareDisjointTimeBaseNote(three, false)
	if !strings.HasPrefix(en, "⚠ The trace files' time bases are pairwise disjoint (a.systrace 100.000s→200.000s, b.systrace 300.000s→400.000s, c.systrace 500.000s→600.000s)") {
		t.Fatalf(">2 partitions must use the pairwise EN wording: %q", en)
	}
	// The >2 next-step variant forks the same way (ledger-level entry).
	ledger := types.ObservationLedger{Records: append(
		rtc2TwoTraceObs(2942.244845, 2942.245401, "selected_window=2942.244845..2942.245401"),
		compareProjObs("c-run", "third.systrace", "root_cause_primary", "root_cause_primary:c-run",
			"worker-1", "running", "1.000", 1.0, 5, 9,
			types.ObservationSpan{LineStart: 5, LineEnd: 9, StartTs: 9000.0, EndTs: 9000.5},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain"),
	)}
	if got := runtimeTraceNextStepDisjointTimeBaseStep(ledger, true); got != "各 trace 时间基准两两不相交,无法在同一时间轴直接对齐;对比请以各自窗口内相对指标为准(占窗比例/按窗长归一化)" {
		t.Fatalf(">2 partitions must use the pairwise ZH step wording: %q", got)
	}
	if got := runtimeTraceNextStepDisjointTimeBaseStep(ledger, false); got != "The traces' time bases are pairwise disjoint and cannot be aligned directly on one shared timeline; compare relative metrics within each trace's own window (window share / normalized by window length)" {
		t.Fatalf(">2 partitions must use the pairwise EN step wording: %q", got)
	}
}
