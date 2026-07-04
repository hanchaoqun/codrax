package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// CMP-B pins (customer compare audit 2026-07-03, docs/design/
// customer_dead_session_audit_20260703.md §7 CMP-4/CMP-7):
//   - metric snapshot candidate priority (chain > analyzer entity > rest, with
//     unrelated threads excluded whenever an on-chain candidate exists);
//   - observed-span-vs-projection-window mismatch annotation (exact >2×);
//   - per-artifact snapshot label prefix on multi-artifact ledgers;
//   - flat-fallback renders never label rows/audits "on-chain";
//   - missing_wakeup (absence) evidence locators drop the synthetic line
//     number and keep the artifact name.

// cmpbSnapshotObservation builds a snapshot-eligible observation (full typed
// state-churn metric set + positive impact). sleepMS varies per fixture thread
// because the snapshot's raw key=value dedupe key deliberately excludes the
// subject — identical metric sets would collapse to one row.
func cmpbSnapshotObservation(id, subject, sleepMS, predicate, claimKey string, notes ...string) types.ObservationRecord {
	rn := append([]string{
		"dominant_state=s_sleep",
		"running=1.000",
		"runnable=2.000",
		"sleep=" + sleepMS,
		"d_state=0.000",
		"io_wait=0.000",
		"fragments=5",
		"switches=4",
		"max_segment=1.500",
		"p95_segment=1.200",
	}, notes...)
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       predicate,
		ClaimKey:        claimKey,
		Subject:         subject,
		Value:           sleepMS,
		Unit:            "ms",
		RichNotes:       rn,
		Confidence:      0.8,
	}
}

func cmpbSnapshotDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks:        []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "observed."}},
	}
}

// CMP-4a pin: an on-chain thread (member of the compiled projection's tree)
// wins the snapshot slot, and unrelated threads are NOT selected at all while
// a chain candidate exists — even with a free slot.
func TestRuntimeTraceMetricSnapshot_ChainThreadsPreemptUnrelatedThreads(t *testing.T) {
	bus := newBusForMutationTest()
	noisy := cmpbSnapshotObservation("noisy", "noisy-9", "3.000", "state_drilldown", "state_drilldown:noisy-9:s_sleep")
	chain := cmpbSnapshotObservation("chain-imp", "chainy-2", "4.000", "wakeup_causal_impact", "wakeup_causal_impact:chainy-2",
		"chain_relevance=on_chain", "impact_ms=3.000")
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		// Ledger order puts the unrelated thread FIRST — the legacy first-two
		// selection would have burned a slot on it.
		Observations: []types.ObservationRecord{noisy, chain},
	}}
	items := runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 1 {
		t.Fatalf("expected exactly the chain-thread snapshot row (unrelated excluded), got %+v", items)
	}
	if items[0].Label != "chainy-2 state_churn" {
		t.Fatalf("chain thread must win the snapshot slot: %+v", items[0])
	}
	for _, item := range items {
		if strings.Contains(item.Label, "noisy-9") || strings.Contains(item.Text, "noisy-9") {
			t.Fatalf("unrelated thread must not enter the snapshot while a chain candidate exists: %+v", item)
		}
	}
}

// CMP-4a pin: without chain candidates, analyzer-entity threads (verbatim
// name/pid lanes) rank above the rest; the rest stays eligible.
func TestRuntimeTraceMetricSnapshot_EntityThreadsRankAboveRest(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"ent"}},
	}}
	noisy := cmpbSnapshotObservation("noisy", "noisy-9", "3.000", "state_drilldown", "state_drilldown:noisy-9:s_sleep")
	entity := cmpbSnapshotObservation("entity", "ent-7", "5.000", "state_drilldown", "state_drilldown:ent-7:s_sleep")
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{noisy, entity},
	}}
	items := runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 2 {
		t.Fatalf("expected entity + rest snapshot rows, got %+v", items)
	}
	if items[0].Label != "ent-7 state_churn" || items[1].Label != "noisy-9 state_churn" {
		t.Fatalf("analyzer-entity thread must rank first: %+v", items)
	}
}

// CMP-4b pin: a snapshot thread whose own observed span exceeds TWICE the
// projection window carries the mismatch note; a within-2× thread does not.
func TestRuntimeTraceMetricSnapshot_SpanMismatchAnnotation(t *testing.T) {
	bus := newBusForMutationTest()
	anchor := types.ObservationRecord{
		ID: "frame", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "frame_target_resolution", ClaimKey: "frame_target_resolution",
		Span:      types.ObservationSpan{StartTs: 100.0, EndTs: 101.0},
		RichNotes: []string{"window_source=query_window"},
	}
	active := traceProjectionObservation("root-busy", "busy-1", "running", "4.000", "4.000", 1)
	watchdog := cmpbSnapshotObservation("wd", "watchdog-3", "2500.000", "state_drilldown", "state_drilldown:watchdog-3:s_sleep", "total=2500.000")
	worker := cmpbSnapshotObservation("wk", "worker-1", "1400.000", "state_drilldown", "state_drilldown:worker-1:s_sleep", "total=1500.000")
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{anchor, active, watchdog, worker},
	}}
	items := runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 2 {
		t.Fatalf("expected two snapshot rows, got %+v", items)
	}
	if !strings.Contains(items[0].Text, "(观测跨度 2.5s,远超投影窗,仅供背景参考)") {
		t.Fatalf("2500ms span vs 1000ms window must carry the mismatch note:\n%s", items[0].Text)
	}
	if strings.Contains(items[1].Text, "观测跨度") {
		t.Fatalf("1500ms span vs 1000ms window (within 2x) must NOT carry the note:\n%s", items[1].Text)
	}
}

// CMP-4c pin: multi-artifact ledgers prefix each snapshot row with the same
// artifact basename the projection sections are titled with.
func TestRuntimeTraceMetricSnapshot_MultiArtifactLabelPrefix(t *testing.T) {
	bus := newBusForMutationTest()
	recA := cmpbSnapshotObservation("a", "alpha-1", "6.000", "wakeup_causal_impact", "wakeup_causal_impact:alpha-1", "impact_ms=3.000")
	recA.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "traces/one.trace"}
	recB := cmpbSnapshotObservation("b", "beta-2", "7.000", "wakeup_causal_impact", "wakeup_causal_impact:beta-2", "impact_ms=3.000")
	recB.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "traces/two.trace"}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{recA, recB},
	}}
	items := runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 2 {
		t.Fatalf("expected one snapshot row per artifact, got %+v", items)
	}
	if items[0].Label != "one.trace · alpha-1 state_churn" || items[1].Label != "two.trace · beta-2 state_churn" {
		t.Fatalf("multi-artifact snapshot rows must carry the artifact prefix: %+v", items)
	}
}

func cmpbFlatProjectionObservations() []types.ObservationRecord {
	imp := types.ObservationRecord{
		ID: "imp", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		Role: types.AnswerAggregateRolePrincipalAnswer, GroundingPolicy: types.ClaimGroundingHard,
		Predicate: "wakeup_causal_impact", ClaimKey: "wakeup_causal_impact:OS_FFRT_2_3-49706",
		Subject: "OS_FFRT_2_3-49706", Object: "runnable", Value: "807.276", Unit: "ms",
		RichNotes:   []string{"causality=on_wakeup_chain", "chain_relevance=on_chain", "impact_ms=807.276", "dominant_state=runnable"},
		SupportRefs: []string{"seven.systrace:100-200"},
		Span:        types.ObservationSpan{LineStart: 100, LineEnd: 200},
		Confidence:  0.78,
	}
	miss := types.ObservationRecord{
		ID: "miss", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		ClaimKey:        "root_evidence:missing_wakeup", Predicate: "missing_wakeup",
		Subject: "OS_FFRT_2_3-49706", Value: "701.000", Unit: "ms",
		Span:        types.ObservationSpan{LineStart: 44},
		SupportRefs: []string{"seven.systrace:44"},
		Summary:     "sleep interval has no matching sched_wakeup row in the selected trace window",
		Confidence:  0.70,
	}
	return []types.ObservationRecord{imp, miss}
}

// CMP-7a + CMP-7b pin (flat side): when the tree renders flat ("按层级平铺",
// no traceable wakeup chain), neither the 因果位置 cell nor the audit summary
// may still claim on-chain; and the missing_wakeup (absence) evidence locator
// drops its synthetic line number, keeping the artifact name only.
func TestRuntimeTraceProjFlatFallback_NoOnChainLabelAndSyntheticLocator(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: cmpbFlatProjectionObservations(),
	}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "observed."}}}
	if _, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	rendered := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
	if !strings.Contains(rendered, "按层级平铺") {
		t.Fatalf("fixture must take the flat fallback:\n%s", rendered)
	}
	// CMP-7a: 因果位置 shows the flat form, never "on-chain · 重点关注".
	if !strings.Contains(rendered, "平铺(链不可上溯) · 重点关注") {
		t.Fatalf("flat render must label the causal position 平铺(链不可上溯):\n%s", rendered)
	}
	if strings.Contains(rendered, "on-chain · 重点关注") {
		t.Fatalf("flat render must not claim on-chain in the causal-position column:\n%s", rendered)
	}
	// CMP-7a: the audit summary swaps the on-chain causality claim for the
	// typed flat token (raw record keeps the verbatim causality note).
	if strings.Contains(rendered, "causality=on_wakeup_chain") {
		t.Fatalf("flat render must not claim on-chain causality in the audit summary:\n%s", rendered)
	}
	if !strings.Contains(rendered, "chain_shape=flat_untraceable") {
		t.Fatalf("flat render audit summary must carry the typed flat token:\n%s", rendered)
	}
	// CMP-7b: the absence observation's synthetic ":44" never renders; the
	// artifact name stays.
	if strings.Contains(rendered, ":44") {
		t.Fatalf("missing_wakeup synthetic line locator must not render:\n%s", rendered)
	}
	if !strings.Contains(rendered, "seven.systrace") {
		t.Fatalf("missing_wakeup locator must keep the artifact name:\n%s", rendered)
	}
}

// CMP-7a negative pin: with a resolved wakeup chain the on-chain labels stay
// exactly as before (no flat rewrite).
func TestRuntimeTraceProjResolvedChain_KeepsOnChainLabels(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	observations := append(cmpbFlatProjectionObservations(), types.ObservationRecord{
		ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
		Object: "upstream-1 -> OS_FFRT_2_3-49706",
	})
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: observations,
	}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "observed."}}}
	if _, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	rendered := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
	if !strings.Contains(rendered, "causality=on_wakeup_chain") ||
		!strings.Contains(rendered, "on-chain · 重点关注") {
		t.Fatalf("resolved-chain render keeps the on-chain labels:\n%s", rendered)
	}
	if strings.Contains(rendered, "平铺(链不可上溯)") || strings.Contains(rendered, "chain_shape=flat_untraceable") {
		t.Fatalf("resolved-chain render must not carry the flat rewrite:\n%s", rendered)
	}
}

// NEW-8 pins (账本 §7.6): the snapshot window-basis line renders the
// selected-window endpoints on BOTH language surfaces when the record's own
// typed selected_window note parses via the shared strict parser, and falls
// back to the byte-identical legacy wording when the note is malformed or
// absent — the endpoint values track the note (= q window) at %.3f rounding.
func TestRuntimeTraceMetricSnapshotDisplayText_SelectedWindowEndpoints(t *testing.T) {
	record := types.ObservationRecord{
		Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query",
		Subject:  "app-20",
		Value:    "5.000",
		RichNotes: []string{
			"dominant_state=runnable",
			"running=3.500", "runnable=5.000", "sleep=0.000", "d_state=0.000", "io_wait=0.000",
			"fragments=21", "switches=20", "max_segment=0.500", "p95_segment=0.500",
			"total=8.500",
			"actual_impact=6.000ms",
			"selected_window=3679.899436..3681.129875",
		},
	}
	if zhText := runtimeTraceMetricSnapshotDisplayText(record, true); !strings.Contains(zhText, "窗口基准: 选定窗 3679.899s–3681.130s(实际对齐窗数值见原始 trace_query 记录)") {
		t.Fatalf("ZH snapshot basis must carry the selected-window endpoints:\n%s", zhText)
	}
	if enText := runtimeTraceMetricSnapshotDisplayText(record, false); !strings.Contains(enText, "window basis: selected window 3679.899s–3681.130s (aligned actual-window values remain in the raw trace_query record)") {
		t.Fatalf("EN snapshot basis must carry the selected-window endpoints:\n%s", enText)
	}

	// Malformed note (end < start): strict parser rejects → legacy wording
	// byte-identical, no fabricated endpoints.
	record.RichNotes[len(record.RichNotes)-1] = "selected_window=3681.129875..3679.899436"
	zhText := runtimeTraceMetricSnapshotDisplayText(record, true)
	if !strings.Contains(zhText, "窗口基准: 选定窗(实际对齐窗数值见原始 trace_query 记录)") {
		t.Fatalf("malformed note must fall back to the legacy ZH basis wording:\n%s", zhText)
	}
	if strings.Contains(zhText, "选定窗 3") {
		t.Fatalf("malformed note must not render endpoints:\n%s", zhText)
	}
	enText := runtimeTraceMetricSnapshotDisplayText(record, false)
	if !strings.Contains(enText, "window basis: selected window (aligned actual-window values remain in the raw trace_query record)") {
		t.Fatalf("malformed note must fall back to the legacy EN basis wording:\n%s", enText)
	}
}
