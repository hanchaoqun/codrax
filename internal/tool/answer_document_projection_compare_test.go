package tool

// CMP-A renderer pins (docs/design/customer_dead_session_audit_20260703.md
// §7.2, customer artifact custom_compare.txt — 7.0 vs 6.0 bindApplication):
//   CMP-1 — a multi-artifact ledger renders ONE projection section per trace
//           artifact (per-artifact title, tree, detail table, evidence index,
//           bar scale); ≥2 active projections (deterministic gate, NEW-2 §7.6)
//           add a compact per-artifact overview table BEFORE the sections;
//           identity-less observations surface only in the partition caveat;
//   CMP-2 — each section's window line comes from that artifact's own
//           selected-window anchor (no more "关注窗口起止未采集" when the
//           precise query window was published);
//   CMP-3 — cross-thread cumulative aggregates (supply_pressure et al) draw no
//           bar, never anchor the bar scale, and their value carries the
//           "(跨线程累计,非墙钟)" unit annotation + normalized density.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func compareProjObs(id, artifact, predicate, claimKey, subject, object, value string, impact float64, lineStart, lineEnd int, span types.ObservationSpan, notes ...string) types.ObservationRecord {
	base := []string{fmt.Sprintf("impact_ms=%.3f", impact), fmt.Sprintf("cumulative_impact_ms=%.3f", impact)}
	record := types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: predicate, ClaimKey: claimKey,
		Subject: subject, Object: object, Value: value, Unit: "ms", Confidence: 0.8,
		Span:      span,
		RichNotes: append(base, notes...),
	}
	if record.Span.LineStart == 0 {
		record.Span.LineStart, record.Span.LineEnd = lineStart, lineEnd
	}
	if artifact != "" {
		record.SourceRef = types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			Path:         artifact,
			ArtifactKind: "trace",
		}
		record.SupportRefs = []string{fmt.Sprintf("%s:%d-%d", artifact, lineStart, lineEnd)}
	}
	return record
}

// compareProjTwoTraceObs mirrors the custom_compare shape: per artifact a
// rank=1 wall-clock primary (with the CMP-2/F1 typed selected_window note +
// its own precise span), a supply_pressure aggregate primary (typed
// subject_kind, with the full duration quad for the F6 mirror) and one
// identity-less relevant record for the caveat.
func compareProjTwoTraceObs() []types.ObservationRecord {
	const artifactA = "7.0B30SP22_7315.systrace"
	const artifactB = "6.0B138_3900.sys.systrace"
	return []types.ObservationRecord{
		compareProjObs("a-run", artifactA, "root_cause_primary", "root_cause_primary:a-run",
			"RSUniRenderThre-1963", "running", "807.276", 807.276, 32642, 199899,
			types.ObservationSpan{LineStart: 32642, LineEnd: 199899, StartTs: 3679.899, EndTs: 3681.129},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"dominant_state=running", "actual_impact_ms=810.000",
			"selected_window=3679.899000..3681.129000"),
		compareProjObs("a-supply", artifactA, "root_cause_primary", "root_cause_primary:a-supply",
			"unknown-thread", "supply_pressure", "101084.884", 101084.884, 32642, 199899,
			types.ObservationSpan{LineStart: 32642, LineEnd: 199899},
			"rank=2", "tier=secondary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"subject_kind=aggregate_metric", "type=supply_pressure",
			"effective_impact=101084.884", "actual_impact=101084.884"),
		compareProjObs("b-run", artifactB, "root_cause_primary", "root_cause_primary:b-run",
			"OS_FFRT_2_6-18695", "sleep_wait", "701.000", 701.0, 31022, 123248,
			types.ObservationSpan{LineStart: 31022, LineEnd: 123248, StartTs: 8143.800, EndTs: 8144.501},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"dominant_state=s_sleep", "actual_impact_ms=1070.028",
			"selected_window=8143.800000..8144.501000"),
		compareProjObs("b-supply", artifactB, "root_cause_primary", "root_cause_primary:b-supply",
			"unknown-thread", "supply_pressure", "46318.120", 46318.120, 31022, 123248,
			types.ObservationSpan{LineStart: 31022, LineEnd: 123248},
			"rank=2", "tier=secondary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"subject_kind=aggregate_metric", "type=supply_pressure"),
		compareProjObs("keyless", "", "root_cause_context", "root_cause_context:keyless",
			"keyless-thread-1", "gc_pause", "3.000", 3.0, 10, 20,
			types.ObservationSpan{LineStart: 10, LineEnd: 20},
			"chain_relevance=background", "causality=background"),
	}
}

func compareProjBus(historicalRegression bool) *types.BusContext {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:         true,
			HistoricalRegression: historicalRegression,
		},
	}}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: compareProjTwoTraceObs(),
	}}
	return bus
}

func compareProjApply(t *testing.T, bus *types.BusContext) *types.AnswerDocumentV2 {
	t.Helper()
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockSummary, Text: "对比案例摘要。"},
	}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	return bus.Mutable.AnswerDocumentV2()
}

func TestTraceProjectionMultiArtifactRendersPerArtifactSections(t *testing.T) {
	got := compareProjApply(t, compareProjBus(true))

	sectionA := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_a1")
	sectionB := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_a2")
	if sectionA == nil || sectionB == nil {
		t.Fatalf("multi-artifact ledger must render one section per artifact: %+v", got.Blocks)
	}
	if projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection") != nil {
		t.Fatalf("multi mode must not also render the single-artifact section")
	}
	if sectionA.Title != "Trace 因果投影 — 7.0B30SP22_7315.systrace" ||
		sectionB.Title != "Trace 因果投影 — 6.0B138_3900.sys.systrace" {
		t.Fatalf("section titles must carry the artifact basename: %q / %q", sectionA.Title, sectionB.Title)
	}
	// Structural invariant: one section, one artifact — no foreign threads.
	if !strings.Contains(sectionA.Text, "RSUniRenderThre-1963") || strings.Contains(sectionA.Text, "OS_FFRT_2_6-18695") {
		t.Fatalf("section A must contain only artifact A's observations:\n%s", sectionA.Text)
	}
	if !strings.Contains(sectionB.Text, "OS_FFRT_2_6-18695") || strings.Contains(sectionB.Text, "RSUniRenderThre-1963") {
		t.Fatalf("section B must contain only artifact B's observations:\n%s", sectionB.Text)
	}
	// Per-artifact conclusion line (the former single global line is gone in
	// multi mode by construction — every section reuses the V1 lane).
	if !strings.Contains(sectionA.Text, "**主根因:** RSUniRenderThre-1963") {
		t.Fatalf("section A must carry its own V1-lane conclusion:\n%s", sectionA.Text)
	}
	if !strings.Contains(sectionB.Text, "**主根因:** OS_FFRT_2_6-18695") {
		t.Fatalf("section B must carry its own V1-lane conclusion:\n%s", sectionB.Text)
	}
	// CMP-2: each section anchors its OWN artifact's selected window — the
	// "关注窗口起止未采集" fallback must be gone.
	if !strings.Contains(sectionA.Text, "关注窗口 3679.899s → 3681.129s,共 1230.000ms") ||
		!strings.Contains(sectionA.Text, "满格=窗口1230.000ms") {
		t.Fatalf("section A must anchor artifact A's window:\n%s", sectionA.Text)
	}
	if !strings.Contains(sectionB.Text, "关注窗口 8143.800s → 8144.501s,共 701.000ms") ||
		!strings.Contains(sectionB.Text, "满格=窗口701.000ms") {
		t.Fatalf("section B must anchor artifact B's window:\n%s", sectionB.Text)
	}
	for _, section := range []*types.AnswerBlock{sectionA, sectionB} {
		if strings.Contains(section.Text, "窗口起止未采集") {
			t.Fatalf("the missing-window fallback must not render when the selected window is published:\n%s", section.Text)
		}
	}
	// CMP-3: the supply_pressure aggregate draws NO bar, carries the unit
	// annotation + normalized density, and never anchors the bar scale.
	if !strings.Contains(sectionA.Text, "101084.884ms(跨线程累计,非墙钟)·≈平均排队深度 82.2") {
		t.Fatalf("cross-thread aggregate must carry the unit annotation + density:\n%s", sectionA.Text)
	}
	if !strings.Contains(sectionB.Text, "46318.120ms(跨线程累计,非墙钟)·≈平均排队深度 66.1") {
		t.Fatalf("artifact B's aggregate must carry its own density:\n%s", sectionB.Text)
	}
	for _, banned := range []string{"▒▒▒▒▒▒▒▒▒▒ 101084.884ms", "█░░░░░░░░░ 101084.884ms"} {
		if strings.Contains(sectionA.Text, banned) {
			t.Fatalf("cross-thread aggregate must not draw a bar (%q):\n%s", banned, sectionA.Text)
		}
	}
	// Per-artifact detail tables and evidence indexes exist with labeled titles.
	detailA := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_a1_detail")
	evidenceB := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_a2_evidence")
	if detailA == nil || !strings.Contains(detailA.Title, "7.0B30SP22_7315.systrace") {
		t.Fatalf("per-artifact detail table missing/mislabeled: %+v", detailA)
	}
	if evidenceB == nil || !strings.Contains(evidenceB.Title, "6.0B138_3900.sys.systrace") {
		t.Fatalf("per-artifact evidence index missing/mislabeled: %+v", evidenceB)
	}
	// CMP-3 mirror on the lossless detail surface — F6: ALL duration columns
	// of the aggregate row (窗口投影/链上累计/有效归因/实际状态) carry the
	// annotation, not just the window projection.
	var aggregateRow []string
	for _, item := range detailA.Items {
		if len(item.Cells) >= 4 && item.Cells[3] == "supply_pressure" {
			aggregateRow = item.Cells
			break
		}
	}
	if aggregateRow == nil {
		t.Fatalf("aggregate detail row missing: %+v", detailA.Items)
	}
	// Columns: ... shape(5), 窗口投影(6), 链上累计(7), 有效归因(8), 实际状态(9).
	for _, col := range []int{6, 7, 8, 9} {
		if !strings.Contains(aggregateRow[col], "101084.884ms(跨线程累计,非墙钟)") {
			t.Fatalf("detail column %d must mirror the cross-thread annotation (F6): %q\nrow: %v",
				col, aggregateRow[col], aggregateRow)
		}
	}
	// The identity-less record renders only through the partition caveat.
	caveat := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_partition")
	if caveat == nil || caveat.Kind != types.BlockCaveat ||
		!strings.Contains(caveat.Text, "1 条观测无工件归属,未纳入投影") {
		t.Fatalf("identity-less observations must surface as the partition caveat: %+v", caveat)
	}
	for _, section := range []*types.AnswerBlock{sectionA, sectionB} {
		if strings.Contains(section.Text, "keyless-thread-1") {
			t.Fatalf("identity-less observations must not blend into a section:\n%s", section.Text)
		}
	}
	// Whole-document sanity: two independent text fences, zero mermaid.
	md := render.RenderAnswerDocument(got, "zh")
	if strings.Count(md, "```text") != 2 || strings.Contains(md, "```mermaid") {
		t.Fatalf("multi-artifact render must emit exactly one tree fence per artifact:\n%s", md)
	}
}

func TestTraceProjectionMultiArtifactComparisonOverviewTable(t *testing.T) {
	got := compareProjApply(t, compareProjBus(true))
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil || compare.Kind != types.BlockTable {
		t.Fatalf("typed comparison shape must render the overview table: %+v", got.Blocks)
	}
	// The overview precedes both artifact sections.
	indexOf := func(id string) int {
		for i, block := range got.Blocks {
			if block.ID == id {
				return i
			}
		}
		return -1
	}
	if !(indexOf("runtime_trace_causal_projection_compare") < indexOf("runtime_trace_causal_projection_a1")) {
		t.Fatalf("the comparison overview must render before the first artifact section")
	}
	if len(compare.Items) != 3 {
		t.Fatalf("one overview row per artifact plus the F3 window note row: %+v", compare.Items)
	}
	rowA := strings.Join(compare.Items[0].Cells, " | ")
	rowB := strings.Join(compare.Items[1].Cells, " | ")
	// Typed-field cells: artifact, V1-lane rank=1 primary, background pressure
	// with the normalized density as PRIMARY content and the raw cross-thread
	// sum demoted to the parenthetical note (§7.3 裁定2 / F3; CMP-9: never a
	// naked cross-window cpu·ms figure), and the per-artifact projection
	// window. Densities: 101084.884/1230.000 → 82.2, 46318.120/701.000 → 66.1.
	for _, want := range []string{
		"7.0B30SP22_7315.systrace",
		"RSUniRenderThre-1963 · 运行 807.276ms",
		"≈平均排队深度 82.2(累计 101084.884ms,跨线程累计,非墙钟)",
		"3679.899s → 3681.129s",
	} {
		if !strings.Contains(rowA, want) {
			t.Fatalf("overview row A missing %q:\n%s", want, rowA)
		}
	}
	if strings.Contains(compare.Items[0].Cells[4], "101084.884ms(跨线程累计,非墙钟)·≈") {
		t.Fatalf("the raw sum must no longer lead the background cell (F3):\n%s", rowA)
	}
	for _, want := range []string{
		"6.0B138_3900.sys.systrace",
		"OS_FFRT_2_6-18695 · 睡眠等待 701.000ms",
		"≈平均排队深度 66.1(累计 46318.120ms,跨线程累计,非墙钟)",
		"8143.800s → 8144.501s",
	} {
		if !strings.Contains(rowB, want) {
			t.Fatalf("overview row B missing %q:\n%s", want, rowB)
		}
	}
	// F3 forced note: 1230.000ms vs 701.000ms differ by 43% (>10%), so the
	// table must close with the unequal-window normalization note row.
	noteRow := strings.Join(compare.Items[2].Cells, " | ")
	if !strings.Contains(noteRow, "两侧投影窗长不等,背景压力已按各自窗长归一化") {
		t.Fatalf("unequal projection windows must force the normalization note row:\n%s", noteRow)
	}
	for _, item := range compare.Items {
		if item.CitationRef != -1 {
			t.Fatalf("system-injected overview rows must carry CitationRef=-1: %+v", item)
		}
	}
}

func TestTraceProjectionComparisonOverviewSkipsWindowNoteWhenWindowsMatch(t *testing.T) {
	// Same fixture, but artifact B's selected window is stretched to the same
	// 1230ms length as A's → |w1-w2|/max = 0 ≤ 0.1 → no note row (the note is
	// a precise comparison, not a fixed table decoration).
	bus := compareProjBus(true)
	obs := compareProjTwoTraceObs()
	for i := range obs {
		if obs[i].ID == "b-run" {
			for j, note := range obs[i].RichNotes {
				if note == "selected_window=8143.800000..8144.501000" {
					obs[i].RichNotes[j] = "selected_window=8143.800000..8145.030000"
				}
			}
		}
	}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	got := compareProjApply(t, bus)
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil || len(compare.Items) != 2 {
		t.Fatalf("equal-length windows must not add the note row: %+v", compare)
	}
	for _, item := range compare.Items {
		if strings.Contains(strings.Join(item.Cells, " "), "窗长不等") {
			t.Fatalf("no note content expected on equal windows: %+v", item.Cells)
		}
	}
}

// NEW-2 pin (§7.6 对比场景客户回访 2026-07-04) — REWRITTEN from the former
// "WithoutComparisonShapeSkipsOverview" pin, by adjudication: the analyzer
// predicate (historical_regression / is_cross_component) is an LLM-emitted
// classification with run-to-run variance, and gating the overview on it made
// the whole table + supply column vanish on a rerun of the SAME two-trace
// question. The overview gate is now the deterministic ≥2-active-projection
// count alone, so a two-projection ledger WITHOUT any analyzer predicate must
// render the overview table (the revisit shape).
func TestTraceProjectionMultiArtifactOverviewRendersWithoutAnalyzerPredicate(t *testing.T) {
	got := compareProjApply(t, compareProjBus(false))
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil || compare.Kind != types.BlockTable {
		t.Fatalf("two active projections must render the overview table without the LLM predicate: %+v", got.Blocks)
	}
	if len(compare.Items) < 2 {
		t.Fatalf("overview must carry one row per artifact: %+v", compare.Items)
	}
	if projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_a1") == nil ||
		projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_a2") == nil {
		t.Fatalf("per-artifact sections must still render alongside the overview")
	}
}

// --- F2: cap degrade / idempotence / single-active caveat -------------------------

func TestTraceProjectionCapDegradePicksProjectionSectionNotCompare(t *testing.T) {
	// F2a: pre-fill the doc so the full multi-artifact cluster (compare + two
	// 3-block sections + caveat) cannot fit but ONE block still can. The
	// degrade survivor must be the first projection section lead — never the
	// compare overview, whose cells reference the sections that were dropped.
	bus := compareProjBus(true)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	for i := 0; i < maxBlocksPerDoc-1; i++ {
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("filler_%d", i), Kind: types.BlockSection, Text: "x",
		})
	}
	if !materializeRuntimeTraceCausalProjectionBlock(doc, bus) {
		t.Fatalf("cap degrade must still emit the lead section")
	}
	if len(doc.Blocks) != maxBlocksPerDoc {
		t.Fatalf("degrade must add exactly one block: %d", len(doc.Blocks))
	}
	if projectionClusterBlock(doc.Blocks, "runtime_trace_causal_projection_compare") != nil {
		t.Fatalf("the compare overview must be dropped on degrade — its cells cite dropped sections")
	}
	lead := projectionClusterBlock(doc.Blocks, "runtime_trace_causal_projection_a1")
	if lead == nil || !strings.Contains(lead.Title, "7.0B30SP22_7315.systrace") {
		t.Fatalf("degrade survivor must be the first projection section lead: %+v", lead)
	}
}

func TestTraceProjectionFamilyIdempotenceGuard(t *testing.T) {
	// F2b matcher: the exact prefix+suffix id family, digits-only artifact tag.
	for _, id := range []string{
		"runtime_trace_causal_projection",
		"runtime_trace_causal_projection_detail",
		"runtime_trace_causal_projection_evidence",
		"runtime_trace_causal_projection_compare",
		"runtime_trace_causal_projection_coverage",
		"runtime_trace_causal_projection_partition",
		"runtime_trace_causal_projection_a1",
		"runtime_trace_causal_projection_a12_detail",
		"runtime_trace_causal_projection_a2_evidence",
	} {
		if !runtimeTraceCausalProjectionFamilyBlockID(id) {
			t.Fatalf("family id %q must match", id)
		}
	}
	for _, id := range []string{
		"runtime_trace_causal_projection_a",
		"runtime_trace_causal_projection_ax",
		"runtime_trace_causal_projection_a1x",
		"runtime_trace_causal_projection_summary",
		"runtime_trace_causal_projectionx",
		"other_block",
	} {
		if runtimeTraceCausalProjectionFamilyBlockID(id) {
			t.Fatalf("non-family id %q must not match", id)
		}
	}
	// F2b materialize-level rerun: a document holding ONLY a family leftover
	// (e.g. a cap-degrade survivor or the caveat) must not grow a second
	// section — the old guard only knew the base/_a1/_coverage ids.
	for _, leftover := range []string{
		"runtime_trace_causal_projection_compare",
		"runtime_trace_causal_projection_a2",
		"runtime_trace_causal_projection_a3_detail",
		"runtime_trace_causal_projection_partition",
	} {
		bus := compareProjBus(true)
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "摘要"},
			{ID: leftover, Kind: types.BlockSection, Text: "leftover"},
		}}
		if materializeRuntimeTraceCausalProjectionBlock(doc, bus) {
			t.Fatalf("leftover %q must trip the idempotence guard", leftover)
		}
		if len(doc.Blocks) != 2 {
			t.Fatalf("guarded doc must stay unchanged for %q: %d blocks", leftover, len(doc.Blocks))
		}
	}
}

func TestTraceProjectionSingleActivePartitionKeepsPartitionCaveat(t *testing.T) {
	// F2c: TWO artifact identities, but artifact B publishes only an inert row
	// (classifies into no projection bucket) → exactly one Active projection →
	// the SINGLE-projection render lane. The unattributed caveat must still
	// render instead of silently dropping with the multi lane.
	bus := compareProjBus(true)
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			compareProjObs("a-run", "7.0B30SP22_7315.systrace", "root_cause_primary", "root_cause_primary:a-run",
				"RSUniRenderThre-1963", "running", "807.276", 807.276, 32642, 199899,
				types.ObservationSpan{LineStart: 32642, LineEnd: 199899},
				"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
				"dominant_state=running"),
			compareProjObs("b-fact", "6.0B138_3900.sys.systrace", "evidence_fact", "evidence_fact:b",
				"some-thread-1", "observed", "1.000", 1.0, 10, 20,
				types.ObservationSpan{LineStart: 10, LineEnd: 20}),
			compareProjObs("keyless", "", "root_cause_context", "root_cause_context:keyless",
				"keyless-thread-1", "gc_pause", "3.000", 3.0, 10, 20,
				types.ObservationSpan{LineStart: 10, LineEnd: 20},
				"chain_relevance=background", "causality=background"),
		},
	}}
	got := compareProjApply(t, bus)
	if projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection") == nil {
		t.Fatalf("one Active projection must render on the single-projection lane: %+v", got.Blocks)
	}
	if projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_a1") != nil {
		t.Fatalf("single Active projection must not render per-artifact sections")
	}
	caveat := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_partition")
	if caveat == nil || caveat.Kind != types.BlockCaveat ||
		!strings.Contains(caveat.Text, "1 条观测无工件归属,未纳入投影") {
		t.Fatalf("the partition caveat must survive the single-projection lane (F2c): %+v", caveat)
	}
}

// --- CMP-3 unit pins -------------------------------------------------------------

func compareProjAggregateNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Object: "supply_pressure", TypeToken: "supply_pressure",
		SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
		ImpactMS:    101084.884, CumulativeImpactMS: 101084.884,
		ChainRelevance: "background",
		StartTs:        3679.899, EndTs: 3681.129,
	}
}

func TestRuntimeTraceProjCrossThreadAggregateTypeMembership(t *testing.T) {
	if !runtimeTraceProjCrossThreadAggregateType(compareProjAggregateNode()) {
		t.Fatalf("supply_pressure aggregate-metric row must classify as cross-thread cumulative")
	}
	for _, token := range []string{"cpu_pressure", "io_pressure", "irq_burst", "irq_activity", "ipi_activity", "cpu_frequency_limit"} {
		node := types.TraceCausalProjectionNode{
			Object:      token,
			SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
		}
		if !runtimeTraceProjCrossThreadAggregateType(node) {
			t.Fatalf("aggregate token %q must classify as cross-thread cumulative", token)
		}
	}
	// The H8 shape — a burst row with a REAL thread subject (no typed
	// subject_kind) — keeps its existing bar + >100% annotation lane.
	h8 := types.TraceCausalProjectionNode{Subject: "irq/151-dpu", Object: "irq_burst", ImpactMS: 204.382}
	if runtimeTraceProjCrossThreadAggregateType(h8) {
		t.Fatalf("thread-subject burst rows must not classify as cross-thread aggregates")
	}
	// Aggregate-metric rows outside the cumulative token set stay untouched.
	other := types.TraceCausalProjectionNode{
		Object: "workqueue_activity", SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
	}
	if runtimeTraceProjCrossThreadAggregateType(other) {
		t.Fatalf("non-cumulative aggregate tokens must not classify")
	}
}

func TestRuntimeTraceProjModelMaxImpactExcludesCrossThreadAggregates(t *testing.T) {
	model := runtimeTraceProjTreeModel{
		Background: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: compareProjAggregateNode()},
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "worker-1", Object: "unknown-thread", ImpactMS: 807.276}},
		},
	}
	if got := runtimeTraceProjModelMaxImpact(model); got != 807.276 {
		t.Fatalf("bar scale must anchor wall-clock values only, got %v", got)
	}
	// Fail-open: an all-aggregate batch keeps the batch max so the scale note
	// never claims a 0.000ms full bar.
	allAggregate := runtimeTraceProjTreeModel{
		Background: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: compareProjAggregateNode()},
		},
	}
	if got := runtimeTraceProjModelMaxImpact(allAggregate); got != 101084.884 {
		t.Fatalf("all-aggregate batches fail open to the batch max, got %v", got)
	}
}

func TestRuntimeTraceProjCrossThreadAggregateRowRendering(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: compareProjAggregateNode(),
	}
	line := runtimeTraceProjStanzaRowLine(row, runtimeTraceProjTreeLabelWidth, 1230.0, true, true)
	// Density from the node's OWN span: 101084.884 / 1230ms ≈ 82.2 (exact
	// division, %.1f display).
	if !strings.Contains(line, "101084.884ms(跨线程累计,非墙钟)·≈平均排队深度 82.2") {
		t.Fatalf("aggregate row must carry annotation + density:\n%s", line)
	}
	for _, glyph := range []string{"█", "▒", "░"} {
		if strings.Contains(line, glyph) {
			t.Fatalf("aggregate row must not draw a bar glyph %q:\n%s", glyph, line)
		}
	}
	if strings.Contains(line, "%") {
		t.Fatalf("aggregate row must not claim a window share:\n%s", line)
	}
	// EN surface forks the same way.
	en := runtimeTraceProjStanzaRowLine(row, runtimeTraceProjTreeLabelWidth, 1230.0, true, false)
	if !strings.Contains(en, "(cross-thread cumulative, not wall clock) ≈avg queue depth 82.2") {
		t.Fatalf("EN aggregate annotation missing:\n%s", en)
	}
	// Non-queue-depth cumulative tokens use the neutral mean wording.
	irq := row
	irq.Node.Object, irq.Node.TypeToken = "irq_activity", "irq_activity"
	irq.Node.ImpactMS, irq.Node.CumulativeImpactMS = 106.05, 106.05
	irqLine := runtimeTraceProjStanzaRowLine(irq, runtimeTraceProjTreeLabelWidth, 1230.0, true, true)
	if !strings.Contains(irqLine, "(跨线程累计,非墙钟)·≈均值 0.1") {
		t.Fatalf("non-pressure aggregates use the mean wording:\n%s", irqLine)
	}
	// Without any window the density is omitted — never estimated.
	bare := row
	bare.Node.StartTs, bare.Node.EndTs = 0, 0
	bareLine := runtimeTraceProjStanzaRowLine(bare, runtimeTraceProjTreeLabelWidth, 0, false, true)
	if !strings.Contains(bareLine, "(跨线程累计,非墙钟)") || strings.Contains(bareLine, "≈") {
		t.Fatalf("windowless aggregate must omit the density:\n%s", bareLine)
	}
}

// F1 pin (review 2026-07-04, evidence-index face): the synthetic-line display
// locator rejects lane placeholder tokens as artifact names — the caller then
// keeps the legacy line display instead of rendering a bare lane token.
func TestRuntimeTraceCausalProjectionSyntheticLocatorRejectsPlaceholderTokens(t *testing.T) {
	for _, ref := range []string{"attached_trace:44", "trace_query:44", "runtime_artifact:44"} {
		got := runtimeTraceCausalProjectionSyntheticEvidenceLocator(runtimeTraceCausalProjectionEvidenceEntry{Ref: ref})
		if got != "" {
			t.Fatalf("placeholder ref %q must not produce a stripped locator, got %q", ref, got)
		}
	}
	got := runtimeTraceCausalProjectionSyntheticEvidenceLocator(runtimeTraceCausalProjectionEvidenceEntry{Ref: "seven.systrace:44"})
	if got == "" || strings.Contains(got, ":44") {
		t.Fatalf("real artifact ref must strip the synthetic line suffix, got %q", got)
	}
}
