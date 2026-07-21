package tool

// CMP-6 next-step pins (docs/design/customer_dead_session_audit_20260703.md
// §7 CMP-6, customer artifact custom_compare.txt), gate re-adjudicated by
// NEW-2 (§7.6 回访 2026-07-04): on a cross-trace comparison ledger (≥2
// compiled ACTIVE per-artifact projections — the SAME deterministic gate as
// the comparison overview table; the LLM analyzer predicate was dropped from
// both surfaces in lockstep) the next-step list leads with the fixed
// comparison-oriented guidance rows (per-trace span anchoring,
// window-length-normalized aggregate comparison). Single-projection
// dispatches stay byte-identical to the pre-CMP-6 output (existing berlin /
// emit-tool next-step pins keep guarding that lane).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// cmp6NextStepObs builds one artifact-A state_churn record that carries a
// typed per-record next-step payload, mirroring the berlin fixture shape.
// Each record gets its own line span so the ledger's source-anchored merge
// key keeps the rows apart (they describe different trace segments).
func cmp6NextStepObs(id, kind, prose string, line int) types.ObservationRecord {
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "state_churn",
		Subject: "app-100", Value: "5.000", Unit: "ms",
		Span: types.ObservationSpan{LineStart: line, LineEnd: line + 10},
		SourceRef: types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			Path:         "7.0B30SP22_7315.systrace",
			ArtifactKind: "trace",
		},
		RichNotes: []string{"next_step=" + prose, "next_step_kind=" + kind},
	}
}

// cmp6DirectionObs builds one artifact-A on-chain rank record carrying a typed
// fix_direction so the compiled board publishes a ◎ direction section (A2 件1:
// the next-step trailing lane's population).
func cmp6DirectionObs(id, direction, subject string, eff float64, line int) types.ObservationRecord {
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "root_cause_primary",
		ClaimKey: "root_cause_primary:" + id,
		Subject:  subject, Object: "runnable", Value: fmt.Sprintf("%.3f", eff), Unit: "ms",
		Confidence: 0.9,
		Span:       types.ObservationSpan{LineStart: line, LineEnd: line + 10},
		SourceRef: types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			Path:         "7.0B30SP22_7315.systrace",
			ArtifactKind: "trace",
		},
		RichNotes: []string{
			fmt.Sprintf("impact_ms=%.3f", eff), fmt.Sprintf("cumulative_impact_ms=%.3f", eff),
			fmt.Sprintf("effective_impact_ms=%.3f", eff),
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"dominant_state=runnable", "fix_direction=" + direction,
		},
	}
}

func TestRuntimeTraceNextStepComparisonRowsLeadOnComparisonShape(t *testing.T) {
	bus := compareProjBus(true)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	items := runtimeTraceNextStepItems(doc, bus)
	if len(items) != 4 {
		t.Fatalf("comparison shape without per-record steps must emit the three fixed comparison rows plus the RTC-2 disjoint time-base row: %+v", items)
	}
	// PTV5 C19 (#68): 各自同口径窗口内, never 同窗 (one shared absolute window
	// is impossible across two traces); "running 时间" stays the §7.2 CMP
	// design verbatim. PTV5 Q3: the third fixed row steers per-window causal
	// sampling on dual-/multi-window comparisons.
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 分别执行同口径因果采样(…)后逐窗对比 → 分别做同样的根因分析(…),再逐窗对比;item Label 下一步 → 空(块标题承载) (下一步族)
	if items[0].Text != "对比两 trace 各自同口径窗口内 top 运行线程与进程级 running 时间差异" ||
		items[1].Text != "对齐目标 span 边界后重取两侧聚合指标(按各自窗长归一化后再对比)" ||
		items[2].Text != "双窗/多窗对比时:对每个查询窗分别做同样的根因分析(wakeup_chain/root_cause_rank),再逐窗对比" {
		t.Fatalf("comparison rows must carry the fixed span-anchoring + normalization + per-window sampling guidance: %+v", items)
	}
	if strings.Contains(items[0].Text, "同窗 ") {
		t.Fatalf("the first comparison row must not claim one shared window (同窗): %+v", items[0])
	}
	// RTC-2 (批 #67): the fixture's time bases (3679.x vs 8143.x) are
	// disjoint → the conditional guidance row trails the fixed rows verbatim.
	if items[3].Text != "两 trace 时间基准不相交,无法在同一时间轴直接对齐;对比请以各自窗口内相对指标为准(占窗比例/按窗长归一化)" {
		t.Fatalf("disjoint time bases must append the RTC-2 guidance row verbatim: %+v", items)
	}
	for i, item := range items {
		if item.Label != "" || item.CitationRef != -1 {
			t.Fatalf("comparison row %d must reuse the next-step item shape (empty label + no citation): %+v", i, item)
		}
	}
	if items[0].ID != "runtime_trace_next_step_1" || items[1].ID != "runtime_trace_next_step_2" ||
		items[2].ID != "runtime_trace_next_step_3" || items[3].ID != "runtime_trace_next_step_4" {
		t.Fatalf("comparison rows must continue the next-step id numbering: %+v", items)
	}
}
func TestRuntimeTraceNextStepComparisonRowsCoexistWithRecordRows(t *testing.T) {
	// A2 件1 (§29.174 UX-13, 2026-07-21) EVOLUTION RECORD: the trailing-lane
	// POPULATION changed — the per-record template rows retired and the ◎
	// direction-action rows take the guaranteed PTS-2 floor slots instead.
	// The #69 floor mechanics themselves are unchanged: the comparison family
	// still emits in FULL and the trailing lane keeps its guaranteed
	// runtimeTraceNextStepComparisonRecordFloor slots (coexistence, not
	// squeeze-out). The fixture's two per-artifact rank primaries carry typed
	// fix_direction notes so each artifact's board publishes one direction
	// section.
	bus := compareProjBus(true)
	obs := compareProjTwoTraceObs()
	for i := range obs {
		switch obs[i].ID {
		case "a-run":
			obs[i].RichNotes = append(obs[i].RichNotes,
				"fix_direction=frequency_thermal", "effective_impact_ms=807.276")
		case "b-run":
			obs[i].RichNotes = append(obs[i].RichNotes,
				"fix_direction=lock_priority", "effective_impact_ms=701.000")
		}
	}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	items := runtimeTraceNextStepItems(doc, bus)
	if len(items) != 4+runtimeTraceNextStepComparisonRecordFloor {
		t.Fatalf("disjoint comparison shape must emit the FULL comparison family plus the guaranteed direction floor: %+v", items)
	}
	// Comparison rows LEAD unchanged (CMP-6 headline adjudication): three
	// fixed rows + the RTC-2 disjoint time-base row.
	if !strings.Contains(items[0].Text, "对比两 trace") || !strings.Contains(items[1].Text, "对齐目标 span 边界") ||
		!strings.Contains(items[2].Text, "逐窗对比") || !strings.Contains(items[3].Text, "时间基准不相交") {
		t.Fatalf("comparison rows must lead the list: %+v", items)
	}
	// The floor slots carry the direction-action rows (projection-set order),
	// each with the section's subject and 最大可消 value.
	if !strings.Contains(items[4].Text, "频率与热治理→") || !strings.Contains(items[4].Text, "RSUniRenderThre-1963") {
		t.Fatalf("the first floor slot must carry artifact A direction action: %+v", items)
	}
	if !strings.Contains(items[5].Text, "锁与优先级→") || !strings.Contains(items[5].Text, "OS_FFRT_2_6-18695") {
		t.Fatalf("the second floor slot must carry artifact B direction action: %+v", items)
	}
	// ID numbering stays continuous across the dynamic cap.
	if items[4].ID != "runtime_trace_next_step_5" || items[5].ID != "runtime_trace_next_step_6" {
		t.Fatalf("next-step ids must continue across the extended cap: %+v", items)
	}
}

// comparison rows.
func TestRuntimeTraceNextStepComparisonRowsLeadWithoutAnalyzerPredicate(t *testing.T) {
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, compareProjBus(false))
	if len(items) != 4 ||
		!strings.Contains(items[0].Text, "对比两 trace") ||
		!strings.Contains(items[1].Text, "对齐目标 span 边界") ||
		!strings.Contains(items[2].Text, "逐窗对比") ||
		!strings.Contains(items[3].Text, "时间基准不相交") {
		t.Fatalf("two active projections must emit the comparison rows without the LLM predicate: %+v", items)
	}
}

// PTS-2 突变 pin (#69): the dynamic cap lives INSIDE the comparison gate —
// a non-comparison (single-artifact) ledger flooded with trailing-lane rows
// still caps at the base runtimeTraceNextStepMaxItems. A2 件1 EVOLUTION
// (2026-07-21): the flood population is now ◎ direction-action rows (five
// published sections; the per-record template rows retired).
func TestRuntimeTraceNextStepNonComparisonCapByteIdentical(t *testing.T) {
	bus := compareProjBus(true)
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			cmp6DirectionObs("dir-1", "scheduling_supply", "worker-1", 9.0, 100),
			cmp6DirectionObs("dir-2", "lock_priority", "worker-2", 8.0, 200),
			cmp6DirectionObs("dir-3", "io_dependency", "worker-3", 7.0, 300),
			cmp6DirectionObs("dir-4", "memory", "worker-4", 6.0, 400),
			cmp6DirectionObs("dir-5", "frequency_thermal", "worker-5", 5.0, 500),
		},
	}}
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != runtimeTraceNextStepMaxItems {
		t.Fatalf("non-comparison shapes must keep the base cap byte-identical: %+v", items)
	}
	for _, item := range items {
		if strings.Contains(item.Text, "对比两 trace") {
			t.Fatalf("single-artifact ledger must not emit comparison rows: %+v", items)
		}
	}
}

func TestRuntimeTraceNextStepComparisonRowsAbsentOnSingleArtifact(t *testing.T) {
	// Comparison predicate set but only ONE artifact identity in the ledger →
	// the projection partition compiles a single projection → not the
	// comparison form; only the trailing-lane row renders (A2 件1 EVOLUTION,
	// 2026-07-21: one direction-action row from the single published section
	// — the per-record template lane retired).
	bus := compareProjBus(true)
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			cmp6DirectionObs("dir-1", "lock_priority", "worker-1", 9.0, 100),
		},
	}}
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != 1 || strings.Contains(items[0].Text, "对比两 trace") {
		t.Fatalf("single-artifact ledger must keep the trailing-lane-only list: %+v", items)
	}
	if !strings.Contains(items[0].Text, "锁与优先级→") || !strings.Contains(items[0].Text, "worker-1") {
		t.Fatalf("the single row must be the section's direction action: %+v", items)
	}
	// RTC-2 zero-emission pin: a single partition never claims disjoint time
	// bases, whatever its own span is.
	for _, item := range items {
		if strings.Contains(item.Text, "时间基准") {
			t.Fatalf("single-artifact ledger must not emit the disjoint time-base row: %+v", items)
		}
	}
}

func TestRuntimeTraceNextStepComparisonRowsEnglishSurface(t *testing.T) {
	bus := compareProjBus(true)
	bus.AnalysisIR.AnswerContract.Language = "en"
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != 4 {
		t.Fatalf("EN comparison shape must emit the three fixed comparison rows plus the RTC-2 disjoint row: %+v", items)
	}
	if !strings.Contains(items[0].Text, "top running threads") ||
		!strings.Contains(items[0].Text, "same-caliber windows") ||
		!strings.Contains(items[1].Text, "target span boundaries") ||
		!strings.Contains(items[1].Text, "normalized by each window's own length") ||
		!strings.Contains(items[2].Text, "per query window, then compare window by window") {
		t.Fatalf("EN comparison rows must mirror the span-anchoring + normalization + per-window sampling guidance: %+v", items)
	}
	if items[3].Text != "The two traces' time bases do not overlap and cannot be aligned directly on one shared timeline; compare relative metrics within each trace's own window (window share / normalized by window length)" {
		t.Fatalf("EN disjoint time-base row must render verbatim: %+v", items)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: Label "Next step" → empty (block title carries it) (下一步族 EN)
	if items[0].Label != "" {
		t.Fatalf("EN rows must keep the empty per-item label: %+v", items[0])
	}
}

func TestRuntimeTraceNextStepComparisonRowsMaterializeEndToEnd(t *testing.T) {
	got := compareProjApply(t, compareProjBus(true))
	var next *types.AnswerBlock
	for i := range got.Blocks {
		if got.Blocks[i].ID == "next_steps" {
			next = &got.Blocks[i]
			break
		}
	}
	if next == nil || next.Kind != types.BlockOrderedList {
		t.Fatalf("comparison shape must materialize the next-step block: %+v", got.Blocks)
	}
	if len(next.Items) < 2 || !strings.Contains(next.Items[0].Text, "对比两 trace") ||
		!strings.Contains(next.Items[1].Text, "对齐目标 span 边界") {
		t.Fatalf("materialized next-step block must lead with the comparison rows: %+v", next.Items)
	}
}
