package tool

// CMP-6 next-step pins (docs/design/customer_dead_session_audit_20260703.md
// §7 CMP-6, customer artifact custom_compare.txt): on a typed cross-trace
// comparison (analyzer historical_regression / is_cross_component boolean +
// ≥2 compiled per-artifact projections — the SAME gate as the comparison
// overview table) the next-step list leads with the fixed comparison-oriented
// guidance rows (per-trace span anchoring, window-length-normalized aggregate
// comparison). Non-comparison dispatches stay byte-identical to the pre-CMP-6
// output (existing berlin / emit-tool next-step pins keep guarding that lane).

import (
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

func TestRuntimeTraceNextStepComparisonRowsLeadOnComparisonShape(t *testing.T) {
	bus := compareProjBus(true)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	items := runtimeTraceNextStepItems(doc, bus)
	if len(items) != 2 {
		t.Fatalf("comparison shape without per-record steps must emit exactly the two comparison rows: %+v", items)
	}
	if items[0].Text != "对比两 trace 同窗 top 运行线程与进程级 running 时间差异" ||
		items[1].Text != "对齐目标 span 边界后重取两侧聚合指标(按各自窗长归一化后再对比)" {
		t.Fatalf("comparison rows must carry the fixed span-anchoring + normalization guidance: %+v", items)
	}
	for i, item := range items {
		if item.Label != "下一步" || item.CitationRef != -1 {
			t.Fatalf("comparison row %d must reuse the next-step item shape (label + no citation): %+v", i, item)
		}
	}
	if items[0].ID != "runtime_trace_next_step_1" || items[1].ID != "runtime_trace_next_step_2" {
		t.Fatalf("comparison rows must continue the next-step id numbering: %+v", items)
	}
}

func TestRuntimeTraceNextStepComparisonRowsLeadBeforeRecordRowsAndShareCap(t *testing.T) {
	bus := compareProjBus(true)
	obs := compareProjTwoTraceObs()
	obs = append(obs,
		cmp6NextStepObs("ns-1", "s_sleep", "inspect the peer thread waking it repeatedly", 100),
		cmp6NextStepObs("ns-2", "d_sleep_io", "inspect sched_blocked_reason and block IO evidence", 200),
		cmp6NextStepObs("ns-3", "running", "inspect the thread own span CPU work", 300),
	)
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	items := runtimeTraceNextStepItems(doc, bus)
	if len(items) != 4 {
		t.Fatalf("the shared item cap must bound comparison + record rows together: %+v", items)
	}
	// Comparison rows LEAD; per-record rows follow in ledger order; the third
	// record row falls off the shared cap.
	if !strings.Contains(items[0].Text, "对比两 trace") || !strings.Contains(items[1].Text, "对齐目标 span 边界") {
		t.Fatalf("comparison rows must lead the list: %+v", items)
	}
	if items[2].Text != "排查反复唤醒它的对端线程、binder等待、锁与条件变量等待" ||
		items[3].Text != "排查 sched_blocked_reason、块设备IO、文件系统、缺页与内存回收证据" {
		t.Fatalf("per-record rows must keep their typed ZH rendering after the comparison rows: %+v", items)
	}
}

func TestRuntimeTraceNextStepComparisonRowsAbsentWithoutComparisonPredicate(t *testing.T) {
	// Two artifacts but no typed comparison predicate → no comparison rows,
	// and (no per-record payloads) no items at all — the block stays absent.
	if items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, compareProjBus(false)); len(items) != 0 {
		t.Fatalf("non-comparison dispatch must not grow comparison rows: %+v", items)
	}
}

func TestRuntimeTraceNextStepComparisonRowsAbsentOnSingleArtifact(t *testing.T) {
	// Comparison predicate set but only ONE artifact identity in the ledger →
	// the projection partition compiles a single projection → not the
	// comparison form; only the per-record row renders.
	bus := compareProjBus(true)
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			cmp6NextStepObs("ns-1", "s_sleep", "inspect the peer thread waking it repeatedly", 100),
		},
	}}
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != 1 || strings.Contains(items[0].Text, "对比两 trace") {
		t.Fatalf("single-artifact ledger must keep the pre-CMP-6 record-only list: %+v", items)
	}
}

func TestRuntimeTraceNextStepComparisonRowsEnglishSurface(t *testing.T) {
	bus := compareProjBus(true)
	bus.AnalysisIR.AnswerContract.Language = "en"
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != 2 {
		t.Fatalf("EN comparison shape must emit the two comparison rows: %+v", items)
	}
	if !strings.Contains(items[0].Text, "top running threads") ||
		!strings.Contains(items[0].Text, "same-caliber windows") ||
		!strings.Contains(items[1].Text, "target span boundaries") ||
		!strings.Contains(items[1].Text, "normalized by each window's own length") {
		t.Fatalf("EN comparison rows must mirror the span-anchoring + normalization guidance: %+v", items)
	}
	if items[0].Label != "Next step" {
		t.Fatalf("EN rows must keep the EN label: %+v", items[0])
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
