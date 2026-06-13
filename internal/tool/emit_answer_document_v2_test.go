package tool

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── B3-T5 emit_answer_document V2 路径单测 ─────────────────────────

// helper: build a fresh BusContext with a non-nil Mutable so
// emit_answer_document_v2 has somewhere to write.
func newV2TestBusContext() *types.BusContext {
	return &types.BusContext{
		Mutable: &types.MutableState{},
	}
}

// helper: minimal V2 emit JSON with one summary block.
func minimalV2EmitJSON() json.RawMessage {
	return json.RawMessage(`{
		"blocks": [
			{"id": "b1", "kind": "summary", "text": "Hello"}
		]
	}`)
}

func TestEmitAnswerDocumentV2_LegacyV1PayloadDoesNotWriteV2(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Legacy V1-shaped emit. Must NOT write the V2 carrier. The
	// deciding factor is the retired V1 top-level payload, not the
	// absence of any document_model field.
	v1JSON := json.RawMessage(`{"shape": "explanation", "summary": "hi"}`)
	res, err := tool.Execute(bus, v1JSON)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	_ = res
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Errorf("legacy emit must not write V2 carrier; got %+v", bus.Mutable.AnswerDocumentV2())
	}
}

func TestEmitAnswerDocumentV2_LegacyHybridPayloadDoesNotWriteV2(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Even if a legacy caller includes an empty document_model field,
	// the retired V1 top-level payload still must not write V2.
	emptyJSON := json.RawMessage(`{"document_model": "", "shape": "explanation", "summary": "hi"}`)
	res, err := tool.Execute(bus, emptyJSON)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	_ = res
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Errorf("legacy hybrid payload must not write V2 carrier; got %+v", bus.Mutable.AnswerDocumentV2())
	}
}

func TestEmitAnswerDocumentV2_AcceptsValidV2(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, minimalV2EmitJSON())
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("V2 doc not written to Mutable")
	}
	if doc.DocumentModel != "v2" {
		t.Errorf("DocumentModel=%q want \"v2\"", doc.DocumentModel)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "b1" || doc.Blocks[0].Kind != types.BlockSummary {
		t.Errorf("blocks not deserialised: got %+v", doc.Blocks)
	}
}

func TestEmitAnswerDocumentV2_MaterializesRuntimeTraceStructuredFacts(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{
			{
				Kind:    "priority_semantics",
				Summary: "Harmony priority semantics: 数值越大优先级越高; 1-40=CFS, 41-139=RT. Observed classes: prio=120/ohos_rt.",
			},
			{
				Kind:    "time_semantics",
				Summary: "Trace timestamps are seconds; attached excerpt spans 168758.662877s..168758.663329s = 0.452ms (452us).",
			},
		},
	})
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "ACCS0 与 Binder:924_3 呈现同步 IPC handoff。"},
			{"id": "scope", "kind": "caveat", "text": "仅限该 trace 片段。"}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("runtime trace supplement should be inserted before caveat, got %+v", doc)
	}
	if doc.Blocks[1].ID != "runtime_trace_facts" || doc.Blocks[1].Kind != types.BlockBulletList {
		t.Fatalf("missing runtime trace facts block: %+v", doc.Blocks)
	}
	visible := types.AnswerBlockVisibleSurface(doc.Blocks[1])
	if !strings.Contains(visible, "数值越大优先级越高") || !strings.Contains(visible, "prio=120/ohos_rt") {
		t.Fatalf("priority semantics not materialized: %q", visible)
	}
	if !strings.Contains(visible, "timestamps are seconds") {
		t.Fatalf("time semantics not materialized: %q", visible)
	}
}

func TestEmitAnswerDocumentV2_MaterializesRuntimeTraceNextStepFromTypedObservation(t *testing.T) {
	bus := newV2TestBusContext()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:       "trace_query:state_churn:1",
			Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query",
			RichNotes: []string{
				"running=3.500",
				"next_step=inspect rival-30 on same CPU cpu=1 for CPU pressure/time-slice competition",
			},
		}},
	}}
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "app-20 dominant_state=runnable。"},
			{"id": "metrics", "kind": "ordered_list", "items": [
				{"id": "m1", "label": "fragments", "text": "21 段", "citation_ref": -1}
			]},
			{"id": "scope", "kind": "caveat", "text": "仅限该 trace 窗口。"}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 4 {
		t.Fatalf("runtime trace next-step block should be inserted before caveat, got %+v", doc)
	}
	next := doc.Blocks[2]
	if next.ID != "next_steps" || next.Kind != types.BlockOrderedList {
		t.Fatalf("missing next_steps block: %+v", doc.Blocks)
	}
	if len(next.Items) != 1 || next.Items[0].Label != "下一步" ||
		!strings.Contains(next.Items[0].Text, "rival-30") ||
		!strings.Contains(next.Items[0].Text, "CPU pressure") {
		t.Fatalf("next step item did not preserve typed note: %+v", next.Items)
	}
	if len(next.ClaimUses) != 1 || next.ClaimUses[0].ClaimForm != types.ClaimExternalObservation {
		t.Fatalf("next_steps block must stay in external-observation lane: %+v", next.ClaimUses)
	}
	if len(doc.Citations) != 0 || next.Items[0].CitationRef != -1 {
		t.Fatalf("runtime next step must not create repo citations: citations=%+v item=%+v", doc.Citations, next.Items[0])
	}
}

func TestEmitAnswerDocumentV2_MaterializesRuntimeTraceMetricSnapshotFromTypedObservation(t *testing.T) {
	bus := newV2TestBusContext()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:state_churn:1",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			Subject:   "app-20",
			Predicate: "state_churn",
			ClaimKey:  "state_churn:runnable",
			RichNotes: []string{
				"dominant_state=runnable",
				"running=3.500",
				"runnable=5.000",
				"sleep=0.000",
				"d_state=0.000",
				"io_wait=0.000",
				"fragments=21",
				"switches=20",
				"max_segment=0.500",
				"p95_segment=0.500",
			},
		}},
	}}
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "app-20 dominant_state=runnable。"},
			{"id": "scope", "kind": "caveat", "text": "仅限该 trace 窗口。"}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("runtime trace metric snapshot block should be inserted before caveat, got %+v", doc)
	}
	snapshot := doc.Blocks[1]
	if snapshot.ID != "runtime_trace_metric_snapshot" || snapshot.Kind != types.BlockBulletList {
		t.Fatalf("missing metric snapshot block: %+v", doc.Blocks)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].Label != "app-20 state_churn" {
		t.Fatalf("unexpected metric snapshot items: %+v", snapshot.Items)
	}
	line := snapshot.Items[0].Text
	for _, want := range []string{
		"running=3.500ms",
		"runnable=5.000ms",
		"sleep=0.000ms",
		"d_state=0.000ms",
		"io_wait=0.000ms",
		"fragments=21",
		"switches=20",
		"max_segment=0.500ms",
		"p95_segment=0.500ms",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("metric snapshot missing %q:\n%s", want, line)
		}
	}
	if len(snapshot.ClaimUses) != 1 || snapshot.ClaimUses[0].ClaimForm != types.ClaimExternalObservation {
		t.Fatalf("snapshot block must stay in external-observation lane: %+v", snapshot.ClaimUses)
	}
	if len(doc.Citations) != 0 || snapshot.Items[0].CitationRef != -1 {
		t.Fatalf("runtime metric snapshot must not create repo citations: citations=%+v item=%+v", doc.Citations, snapshot.Items[0])
	}
}

func TestEmitAnswerDocumentV2_DoesNotDuplicateExistingRuntimeTraceNextStepsBlock(t *testing.T) {
	bus := newV2TestBusContext()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:state_churn:1",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			RichNotes: []string{"next_step=inspect rival-30 on same CPU cpu=1"},
		}},
	}}
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "app-20 dominant_state=runnable。"},
			{"id": "next_steps", "kind": "ordered_list", "items": [
				{"id": "n1", "label": "下一步", "text": "查看 rival-30 的 wakeup 链", "citation_ref": -1}
			]},
			{"id": "scope", "kind": "caveat", "text": "仅限该 trace 窗口。"}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	count := 0
	for _, block := range doc.Blocks {
		if block.ID == "next_steps" {
			count++
		}
	}
	if count != 1 || len(doc.Blocks) != 3 {
		t.Fatalf("existing next_steps block should be preserved without duplication: %+v", doc.Blocks)
	}
}

func TestEmitAnswerDocumentV2_DoesNotMaterializeRuntimeFactsForCodeOnlyDoc(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, minimalV2EmitJSON())
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("code-only doc should not get runtime trace supplement: %+v", doc)
	}
}

func TestEmitAnswerDocumentV2_MixedSourceAndTraceKeepsLanesSeparate(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Kind:    "priority_semantics",
			Summary: "Harmony priority semantics: 数值越大优先级越高; 1-40=CFS, 41-139=RT. Observed classes: prio=120/ohos_rt.",
		}},
	})
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"blocks": [{
			"id": "source_path",
			"kind": "ordered_list",
			"surface_role": "principal",
			"claim_uses": [{"claim_form": "call_edge"}],
			"items": [{"id": "i1", "label": "source hop", "text": "当前源码调用链", "citation_ref": 0}]
		}],
		"citations": [{"file": "internal/example.go", "line": 12}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("mixed source+trace doc should append one trace support block: %+v", doc)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 0 {
		t.Fatalf("source citation ref should be preserved, got %d", got)
	}
	if len(doc.Citations) != 1 || doc.Citations[0].File != "internal/example.go" {
		t.Fatalf("source citation pool should be preserved: %+v", doc.Citations)
	}
	traceBlock := doc.Blocks[1]
	if traceBlock.ID != "runtime_trace_facts" || traceBlock.SurfaceRole == types.SurfacePrincipal {
		t.Fatalf("trace facts must stay support-only external observation lane: %+v", traceBlock)
	}
	if len(traceBlock.Items) != 1 || traceBlock.Items[0].CitationRef != -1 {
		t.Fatalf("trace support block must not create source citations; got %+v", traceBlock.Items)
	}
	if len(traceBlock.ClaimUses) != 1 || traceBlock.ClaimUses[0].ClaimForm != types.ClaimExternalObservation {
		t.Fatalf("trace support block must declare external observation: %+v", traceBlock.ClaimUses)
	}
}

func TestEmitAnswerDocumentV2_PrunesScalarItemFragmentsWhenItemObjectsPreserveDisplay(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"blocks": [{
			"id": "hops",
			"kind": "ordered_list",
			"items": [
				{"id": "h1", "label": "进入睡眠", "text": "com.tencent.mm-36379 通过 sched_switch 进入 S 状态", "citation_ref": 0},
				"{\t\t]\t}",
				{"id": "h2", "label": "唤醒调度", "text": "[GT]ColdPool#5 通过 sched_wakeup 唤醒目标线程", "citation_ref": 1},
				"citation_ref:8"
			]
		}],
		"citations": [
			{"file": ".codrax/blob/attached_trace.txt", "line": 1102717},
			{"file": ".codrax/blob/attached_trace.txt", "line": 1139180}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("full emit should prune structural item fragments when item objects preserve display: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("missing emitted doc: %+v", doc)
	}
	if got := len(doc.Blocks[0].Items); got != 2 {
		t.Fatalf("expected two item objects after pruning scalar fragments, got %d (%+v)", got, doc.Blocks[0].Items)
	}
	if doc.Blocks[0].Items[0].Text == "" || doc.Blocks[0].Items[1].Text == "" {
		t.Fatalf("visible item text must be preserved: %+v", doc.Blocks[0].Items)
	}
}

func TestEmitAnswerDocumentV2_DoesNotPruneMeaningfulScalarItemTextWithoutCarrier(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"blocks": [{
			"id": "hops",
			"kind": "ordered_list",
			"items": [
				{"id": "h1", "citation_ref": 0},
				"this is visible prose that cannot be silently dropped"
			]
		}],
		"citations": [{"file": "trace.txt", "line": 1}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatalf("meaningful scalar item text without a visible carrier must not be silently pruned")
	}
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Fatalf("rejected malformed doc must not be persisted: %+v", bus.Mutable.AnswerDocumentV2())
	}
}

func TestEmitAnswerDocumentV2_EmptyBlocksRejectsWithStructuredRepair(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, json.RawMessage(`{"blocks":[],"citations":[{"file":"x.go","line":1}]}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatal("empty blocks[] must reject")
	}
	if res.Repair == nil {
		t.Fatalf("empty blocks[] rejection should carry structured repair metadata: %+v", res)
	}
	if res.Repair.Code != "answer_doc_blocks_required" {
		t.Fatalf("repair code = %q", res.Repair.Code)
	}
	for _, want := range []string{"blocks", "non-empty `blocks[]`", "Preserve the same answer facts"} {
		joined := strings.Join(append(res.Repair.Fields, res.Repair.Hint), "\n")
		if !strings.Contains(joined, want) {
			t.Fatalf("repair metadata missing %q: %+v", want, res.Repair)
		}
	}
}

func TestEmitAnswerDocumentV2_PreEmitSoftHintsDoNotReject(t *testing.T) {
	bus := newV2TestBusContext()
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
		},
	}
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "Only a partial first draft."}
		]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("soft pre-emit structural hints should not fail the tool call: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) < 1 || doc.Blocks[0].ID != "s1" {
		t.Fatalf("document should persist despite soft pre-emit hints: %+v", doc)
	}
	if len(doc.Blocks) > 1 {
		last := doc.Blocks[len(doc.Blocks)-1]
		if last.Kind != types.BlockCaveat || !slices.Contains(last.FacetIDs, string(types.FacetUncertaintyBoundary)) {
			t.Fatalf("additional soft repair block should be an uncertainty caveat, got %+v", last)
		}
	}
}

func TestEmitAnswerDocumentV2_DiagramBodyStripsNestedFences(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	params, err := json.Marshal(map[string]any{
		"blocks": []map[string]any{
			{"id": "s1", "kind": "summary", "text": "Architecture flow."},
			{"id": "d1", "kind": "diagram", "diagram": map[string]any{
				"kind":     "flow",
				"language": "mermaid",
				"body":     "```mermaid\n```mermaid\nflowchart TD\n  A --> B\n```\n```",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected nested diagram fences to be normalized, got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || doc.Blocks[1].Diagram == nil {
		t.Fatalf("diagram block not written: %+v", doc)
	}
	if got := strings.TrimSpace(doc.Blocks[1].Diagram.Body); got != "flowchart TD\n  A --> B" {
		t.Fatalf("diagram body still has wrapper fences: %q", got)
	}
}

func TestEmitAnswerDocumentV2_AcceptsStructuredTableColumnsCells(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "Hello"},
			{"id": "t1", "kind": "table", "columns": ["维度", "codrax", "opencode"], "items": [
				{"id": "r1", "cells": ["证据追踪", "citations[]", "none"], "citation_ref": -1}
			]}
		]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("structured table emit should succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("doc not written: %+v", doc)
	}
	table := doc.Blocks[1]
	if len(table.Columns) != 3 || table.Columns[1] != "codrax" {
		t.Fatalf("columns not preserved: %+v", table.Columns)
	}
	if len(table.Items) != 1 || len(table.Items[0].Cells) != 3 || table.Items[0].Cells[0] != "证据追踪" {
		t.Fatalf("cells not preserved: %+v", table.Items)
	}
}

func TestEmitAnswerDocumentV2_QuarantinesUnknownSchemaMetadata(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"claim_uses": [{"claim_form": "definition"}],
		"metadata": {"model": "local"},
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "Hello", "metadata": {"dropped": true}},
			{"id": "l1", "kind": "ordered_list", "items": [
				{"id": "i1", "label": "Item", "text": "Detail", "citation_ref": 0, "debug": "drop me"}
			]}
		],
		"citations": [
			{"file": "x.go", "line": 10, "quote": "func x()", "confidence": 0.95}
		]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unknown schema metadata should be quarantined instead of forcing retry: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Citations) != 1 {
		t.Fatalf("core document was not preserved after quarantine: %+v", doc)
	}
	if got := doc.Blocks[1].Items[0].CitationRef; got != 0 {
		t.Fatalf("known item fields should survive quarantine, citation_ref=%d", got)
	}
}

func TestEmitAnswerDocumentV2_RepairsSingleUnknownItemTextAlias(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "最近提交概览。"},
			{"id": "l1", "kind": "ordered_list", "items": [
				{"id": "i1", "label": "333707a", "tool": "标记 deterministic command measurements 的稳定化与计量路径", "citation_ref": -1}
			]}
		]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("single unknown item text alias should be repaired, got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].Items) != 1 {
		t.Fatalf("document not written: %+v", doc)
	}
	if got := doc.Blocks[1].Items[0].Text; !strings.Contains(got, "deterministic command") {
		t.Fatalf("unknown item text alias was not preserved as visible text: %q", got)
	}
}

func TestEmitAnswerDocumentV2_DoesNotGuessAmbiguousUnknownItemTextAliases(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "最近提交概览。"},
			{"id": "l1", "kind": "ordered_list", "items": [
				{"id": "i1", "label": "333707a", "tool": "first", "detail": "second", "citation_ref": -1}
			]}
		]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ambiguous unknown item fields should still quarantine instead of retrying: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].Items) != 1 {
		t.Fatalf("document not written: %+v", doc)
	}
	if got := doc.Blocks[1].Items[0].Text; got != "" {
		t.Fatalf("ambiguous unknown item text aliases must not be guessed, got %q", got)
	}
}

func TestEmitAnswerDocumentV2_CaveatAliasAcceptedForCaveatBlock(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary", "text": "Hello"},
			{"id": "c1", "kind": "caveat", "caveat": "Only files read in this run were considered."}
		]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("V2 doc not written correctly: %+v", doc)
	}
	if doc.Blocks[1].Kind != types.BlockCaveat || doc.Blocks[1].Text != "Only files read in this run were considered." {
		t.Fatalf("caveat alias not normalized: %+v", doc.Blocks[1])
	}
}

func TestEmitAnswerDocumentV2_LeavesMissingModelSurfaceTermAsAdvisory(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-index",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "Index",
		AnchorSymbol: "Index",
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"Index.ets"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "entry_list",
			"kind": "ordered_list",
			"items": [{
				"id": "e1",
				"label": "Index",
				"text": "struct Index with @Entry + @Component",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			"line": 7
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected missing surface term to stay advisory, got %q", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].Items) != 1 ||
		strings.Contains(doc.Blocks[0].Items[0].Text, "Index.ets") {
		t.Fatalf("model surface term should not be patched into visible item text: %+v", doc)
	}
	hints := preCheckModelSurfaceTerms(doc, bus)
	if len(hints) == 0 || !strings.Contains(formatEmitFixHints(hints), "Index.ets") {
		t.Fatalf("missing surface term should remain advisory, got %+v", hints)
	}
}

func TestEmitAnswerDocumentV2_AcceptsPreservedModelSurfaceTerm(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-index",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "Index",
		AnchorSymbol: "Index",
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"Index.ets"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "entry_list",
			"kind": "ordered_list",
			"items": [{
				"id": "e1",
				"label": "Index",
				"text": "struct Index, original source label Index.ets, with @Entry + @Component",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			"line": 7
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected surface term preserving emit to succeed; got %+v", res)
	}
}

func TestEmitAnswerDocumentV2_DoesNotRequireUnrelatedPathSurfaceTerm(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-normalize",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "analyzerGraphForNormalize",
		AnchorSymbol: "analyzerGraphForNormalize",
		Source:       "internal/agent/analyzer.go",
		LineStart:    1625,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"internal/types/enumeration_boundary.go"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "trace_steps",
			"kind": "ordered_list",
			"items": [{
				"id": "e1",
				"label": "analyzerGraphForNormalize",
				"text": "calls normalizer.Normalize and returns a TermGraph.",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "internal/agent/analyzer.go",
			"line": 1625
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unrelated path-like surface term should not be hard-required; got %+v", res)
	}
}

func TestEmitAnswerDocumentV2_LeavesMissingPathSurfaceTermAsAdvisory(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-widget",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "Widget",
		AnchorSymbol: "Widget",
		Source:       "src/components/Widget.ets",
		LineStart:    3,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"src/components/Widget.ets"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "entry_list",
			"kind": "ordered_list",
			"items": [{
				"id": "e1",
				"label": "Widget",
				"text": "declares the component entry.",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "src/components/Widget.ets",
			"line": 3
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected item-identifying path surface term to stay advisory, got %q", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].Items) != 1 ||
		strings.Contains(doc.Blocks[0].Items[0].Text, "src/components/Widget.ets") {
		t.Fatalf("path-like model surface term should not be patched into visible item text: %+v", doc)
	}
	hints := preCheckModelSurfaceTerms(doc, bus)
	if len(hints) == 0 || !strings.Contains(formatEmitFixHints(hints), "src/components/Widget.ets") {
		t.Fatalf("missing path surface term should remain advisory, got %+v", hints)
	}
}

func TestEmitAnswerDocumentV2_DoesNotPatchHiddenTableItemsForSurfaceTerms(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-index",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "Index",
		AnchorSymbol: "Index",
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"Index.ets"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "comparison_table",
			"kind": "table",
			"text": "| Item | Detail |\\n|---|---|\\n| Index | struct with @Entry + @Component |",
			"items": [{
				"id": "e1",
				"label": "Index",
				"text": "hidden row support",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			"line": 7
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("table markdown text should not hard-retry on hidden row surface terms; got %q", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].Items) != 1 {
		t.Fatalf("V2 doc not written correctly: %+v", doc)
	}
	if strings.Contains(doc.Blocks[0].Items[0].Text, "Index.ets") {
		t.Fatalf("surface term was patched into a hidden table item instead of respecting table text: %+v", doc.Blocks[0].Items[0])
	}
}

func TestEmitAnswerDocumentV2_RejectsV1FieldsAtTopLevel(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Legacy top-level V1 field mixed into the block-only carrier → reject.
	mixed := json.RawMessage(`{
		"shape": "explanation",
		"blocks": [{"id":"b1","kind":"summary","text":"x"}]
	}`)
	res, _ := tool.Execute(bus, mixed)
	if res.Success {
		t.Fatal("expected mixed V1+V2 emit to be rejected")
	}
	// v3.1 (2026-05-05): the rejection text no longer mentions
	// internal version concepts ("V1") — it only names the offending
	// top-level field and explains the answer must live in blocks[].
	if !strings.Contains(res.Summary, "shape") || !strings.Contains(res.Summary, "blocks[]") {
		t.Errorf("error message should name the offending field and the blocks-only contract; got %q", res.Summary)
	}
}

// v3.1 (2026-05-05) — TestEmitAnswerDocumentV2_RejectsUnknownDocumentModel
// removed. The system runs only one carrier and document_model is no
// longer surfaced to the LLM nor validated by the executor; whatever
// value the LLM emits (or omits) is silently accepted. Future
// migration to a second carrier would re-introduce a typed rejection.

func TestEmitAnswerDocumentV2_RejectsEmptyBlocks(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	emptyBlocks := json.RawMessage(`{"blocks": []}`)
	res, _ := tool.Execute(bus, emptyBlocks)
	if res.Success {
		t.Fatal("expected empty blocks[] to be rejected")
	}
}

func TestEmitAnswerDocumentV2_RejectsMissingBlockID(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	bad := json.RawMessage(`{
		"blocks": [{"kind": "summary", "text": "x"}]
	}`)
	res, _ := tool.Execute(bus, bad)
	if res.Success {
		t.Fatal("expected missing block id to be rejected")
	}
	if !strings.Contains(res.Summary, "id is required") {
		t.Errorf("error message should mention id requirement; got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_RejectsInvalidBlockKind(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	bad := json.RawMessage(`{
		"blocks": [{"id":"b1","kind":"bogus_kind","text":"x"}]
	}`)
	res, _ := tool.Execute(bus, bad)
	if res.Success {
		t.Fatal("expected invalid block kind to be rejected")
	}
	// G2 (post_v2_runtime_gap_remediation, 2026-05-04): the unified
	// NormalizeEmitAnswerBlock helper uses LLM-facing prose
	// ("block kind" instead of the Go type name "AnswerBlockKind",
	// per R4). The error message MUST still name the offending kind
	// verbatim AND the allowed list, so the LLM can fix the emit.
	if !strings.Contains(res.Summary, `"bogus_kind"`) || !strings.Contains(res.Summary, "block kind") {
		t.Errorf("error message must name the bad value + the contract concept; got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_RejectsDuplicateBlockIDs(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	dup := json.RawMessage(`{
		"blocks": [
			{"id":"x","kind":"summary","text":"a"},
			{"id":"x","kind":"summary","text":"b"}
		]
	}`)
	res, _ := tool.Execute(bus, dup)
	if res.Success {
		t.Fatal("expected duplicate block ids to be rejected")
	}
}

func TestEmitAnswerDocumentV2_NormalizesWhitespaceAndIdenticalDuplicateBlockIDs(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	dup := json.RawMessage(`{
		"blocks": [
			{
				"id":" lead ",
				"kind":"summary",
				"surface_role":"principal",
				"text":"same answer"
			},
			{
				"id":"lead",
				"kind":"summary",
				"surface_role":"principal",
				"text":"same answer"
			}
		]
	}`)
	res, err := tool.Execute(bus, dup)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("identical duplicate block ids should be normalized instead of forcing retry: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("duplicate block should be coalesced, got %+v", doc)
	}
	if doc.Blocks[0].ID != "lead" || doc.Blocks[0].Text != "same answer" {
		t.Fatalf("normalized block should preserve visible content and trimmed id, got %+v", doc.Blocks[0])
	}
}

func TestEmitAnswerDocumentV2_DiagramBlockRequiresPayload(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	noPayload := json.RawMessage(`{
		"blocks": [{"id":"d1","kind":"diagram","text":"x"}]
	}`)
	res, _ := tool.Execute(bus, noPayload)
	if res.Success {
		t.Fatal("expected diagram block without payload to be rejected")
	}
}

// V1/V2 mutual exclusion: V2 emit must not pollute V1's
// AnswerDocument field, AND V1 emit (with shape) must not pollute
// V2's AnswerDocumentV2 field.
func TestEmitAnswerDocumentV2_MutualExclusionWithV1(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}

	// 1. V2 emit → V1 must remain nil.
	if _, err := tool.Execute(bus, minimalV2EmitJSON()); err != nil {
		t.Fatal(err)
	}
	if bus.Mutable.AnswerDocumentV2() == nil {
		t.Error("V2 emit did not write V2 carrier")
	}

	// 2. New bus, V1 emit → V2 must remain nil.
	bus2 := newV2TestBusContext()
	v1 := json.RawMessage(`{"shape":"explanation","summary":"text"}`)
	if _, err := tool.Execute(bus2, v1); err != nil {
		t.Fatal(err)
	}
	if bus2.Mutable.AnswerDocumentV2() != nil {
		t.Error("V1 emit polluted V2 carrier")
	}
}

// ── B5-F1 flat-mode tolerance: blocks[] arrived as JSON-encoded string ─

// TestEmitAnswerDocumentV2_FlatModeBlocksAsString covers the
// streaming-artefact path where the LLM stringified the blocks
// array. The repair re-parses the embedded JSON array and the emit
// is accepted byte-equivalently.
func TestEmitAnswerDocumentV2_FlatModeBlocksAsString(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// blocks rendered as a JSON-encoded string instead of an array.
	flat := json.RawMessage(`{
		"document_model": "v2",
		"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\",\"text\":\"Hello\"}]"
	}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("flat-mode emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("flat-mode emit should succeed via repair; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("flat-mode repair did not write V2 carrier")
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "b1" || doc.Blocks[0].Kind != types.BlockSummary {
		t.Errorf("flat-mode block did not deserialise: %+v", doc.Blocks)
	}
}

func TestEmitAnswerDocumentV2_FlatModeTopLevelCitationsAsString(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{
		"blocks": [{"id":"b1","kind":"summary","text":"Hello"}],
		"citations": "[{\"file\":\"internal/agent/sub_explorer.go\",\"line\":31}]"
	}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("flat-mode emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("top-level citations string should succeed via repair; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("flat-mode repair did not write V2 carrier")
	}
	if len(doc.Citations) != 1 || doc.Citations[0].File != "internal/agent/sub_explorer.go" || doc.Citations[0].Line != 31 {
		t.Fatalf("citations did not deserialise from JSON string: %+v", doc.Citations)
	}
}

func TestEmitAnswerDocumentV2_FlatModeTopLevelCitationsStringWithExtraCloser(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{
		"blocks": [{"id":"b1","kind":"summary","text":"Hello"}],
		"citations": "[{\"file\":\"internal/agent/sub_explorer.go\",\"line\":31}]]"
	}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("flat-mode emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("overclosed top-level citations string should succeed via repair; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 || doc.Citations[0].Line != 31 {
		t.Fatalf("citations did not deserialise from overclosed JSON string: %+v", doc)
	}
}

// TestRepairBlocksAsString_NoTrigger pins the negative cases — the
// repair must NOT fire when blocks is already an array, when the
// embedded string is not a JSON array, or when blocks is absent.
func TestRepairBlocksAsString_NoTrigger(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"already_array", `{"blocks":[{"id":"b1","kind":"summary"}]}`},
		{"absent", `{"document_model":"v2"}`},
		{"string_not_array", `{"blocks":"hello"}`},
		{"empty", ``},
		{"malformed_outer", `{not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := repairBlocksAsString(json.RawMessage(tc.raw))
			if ok {
				t.Errorf("%s: repair fired unexpectedly", tc.name)
			}
		})
	}
}

// TestRepairBlocksAsString_PreservesOtherFields confirms the patch
// surgically swaps blocks while every other top-level key passes
// through verbatim.
func TestRepairBlocksAsString_PreservesOtherFields(t *testing.T) {
	raw := json.RawMessage(`{
		"caveats": ["a","b"],
		"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\"}]"
	}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("repair did not fire")
	}
	if !strings.Contains(string(patched), `"caveats":["a","b"]`) {
		t.Errorf("caveats lost: %s", patched)
	}
	// Critically: blocks must now be a real JSON array.
	if !strings.Contains(string(patched), `"blocks":[`) {
		t.Errorf("blocks not converted to array: %s", patched)
	}
}

// TestRepairBlocksAsString_WholeDocumentStringify covers the v2 repair
// extension (Plan 2 v2 follow-up, 2026-05-05) — the LLM JSON.stringify'd
// the entire document body, producing
//
//	{"blocks": "[blocks_array], \"citations\": [...], \"caveats\": [...]"}
//
// where the encoded string contains the blocks array followed by the
// rest of the top-level fields. Recovery: wrap the encoded string with
// `{"blocks": ... }` so the whole-doc payload becomes a parseable
// top-level object, then merge.
func TestRepairBlocksAsString_WholeDocumentStringify(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\",\"text\":\"hi\"}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], \"caveats\": [\"c1\"]"}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("repair did not fire on whole-document stringify")
	}
	if !strings.Contains(string(patched), `"blocks":[`) {
		t.Errorf("blocks not converted to array: %s", patched)
	}
	if !strings.Contains(string(patched), `"citations":[`) {
		t.Errorf("citations not lifted to top level: %s", patched)
	}
	if !strings.Contains(string(patched), `"caveats":["c1"]`) {
		t.Errorf("caveats not lifted to top level: %s", patched)
	}
	// Validate full Decode round-trip.
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("repaired payload no longer parses: %v", err)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].ID != "b1" {
		t.Errorf("blocks lost during repair: %+v", p.Blocks)
	}
	if len(p.Citations) != 1 {
		t.Errorf("citations lost during repair: %+v", p.Citations)
	}
}

func TestRepairBlocksAsString_WholeDocumentStringifyWithTrailingBrace(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\",\"text\":\"hi\"}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], \"caveats\": [\"c1\"]}"}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("repair did not fire on whole-document stringify with trailing object brace")
	}
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("repaired payload no longer parses: %v\npayload=%s", err, patched)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].ID != "b1" {
		t.Fatalf("blocks lost during repair: %+v", p.Blocks)
	}
	if len(p.Citations) != 1 || p.Citations[0].File != "x.go" || p.Citations[0].Line != 1 {
		t.Fatalf("citations not lifted to typed citation pool: %+v", p.Citations)
	}
	if len(p.Caveats) != 1 || p.Caveats[0] != "c1" {
		t.Fatalf("caveats not lifted to top level: %+v", p.Caveats)
	}
}

func TestRepairBlocksAsString_StringifiedBlocksMissingOuterBlockClose(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"hi\"},{\"id\":\"d1\",\"kind\":\"diagram\",\"diagram\":{\"kind\":\"flow\",\"language\":\"mermaid\",\"body\":\"flowchart TD\\n    A --> B\"}, {\"id\":\"c1\",\"kind\":\"caveat\",\"text\":\"keep this caveat\"}]"}`)
	patched, report, ok := repairBlocksAsStringDetailed(raw)
	if !ok {
		t.Fatal("repair did not fire on missing outer block close")
	}
	if !report.Lossless {
		t.Fatalf("missing outer block close should be a lossless structural repair, got %+v", report)
	}
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("repaired payload no longer parses: %v\npayload=%s", err, patched)
	}
	if len(p.Blocks) != 3 {
		t.Fatalf("expected summary, diagram, caveat blocks; got %+v", p.Blocks)
	}
	if p.Blocks[1].Diagram == nil || !strings.Contains(p.Blocks[1].Diagram.Body, "flowchart TD") {
		t.Fatalf("diagram body not preserved: %+v", p.Blocks[1])
	}
	if p.Blocks[2].Kind != string(types.BlockCaveat) || !strings.Contains(p.Blocks[2].Text, "keep this caveat") {
		t.Fatalf("caveat block was not preserved: %+v", p.Blocks[2])
	}
}

func TestEmitAnswerDocumentV2_StringifiedBlocksMissingOuterBlockCloseAccepted(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	raw := json.RawMessage(`{
		"blocks": "[{\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"hi\"},{\"id\":\"d1\",\"kind\":\"diagram\",\"diagram\":{\"kind\":\"flow\",\"language\":\"mermaid\",\"body\":\"flowchart TD\\n    A --> B\"}, {\"id\":\"c1\",\"kind\":\"caveat\",\"text\":\"keep this caveat\"}]"
	}`)
	res, err := tool.Execute(bus, raw)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("lossless stringified blocks repair should avoid finalizer retry, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("repaired doc not persisted: %+v", doc)
	}
	if got := bus.Mutable.AnswerDisplayAttachments(); len(got) != 0 {
		t.Fatalf("lossless repair should not create fallback display attachments: %+v", got)
	}
}

func TestRepairBlocksAsString_BraceFallbackSkipsCitationObjects(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\",\"text\":\"hi\"}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], trailing"}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("brace fallback did not recover the valid answer block")
	}
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("brace fallback produced invalid payload: %v\npayload=%s", err, patched)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].ID != "b1" {
		t.Fatalf("brace fallback should keep only block-shaped objects, got %+v", p.Blocks)
	}
}

func TestRepairBlocksAsString_BraceFallbackReportsDroppedDiagram(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"sum1\",\"kind\":\"summary\",\"text\":\"hi\"},{\"id\":\"diag1\",\"kind\":\"diagram\",\"diagram\":{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\n    User->>Agent: \"ask\"}}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], trailing"}`)
	patched, report, ok := repairBlocksAsStringDetailed(raw)
	if !ok {
		t.Fatal("brace fallback did not recover the valid answer block")
	}
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("brace fallback produced invalid payload: %v\npayload=%s", err, patched)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].ID != "sum1" {
		t.Fatalf("expected only valid summary block to survive structured recovery, got %+v", p.Blocks)
	}
	if len(p.Citations) != 1 || p.Citations[0].File != "x.go" {
		t.Fatalf("brace fallback should lift sibling citations from the stringified document, got %+v", p.Citations)
	}
	if report.Lossless {
		t.Fatalf("malformed diagram recovery must be marked lossy: %+v", report)
	}
	if !report.DroppedVisiblePayload {
		t.Fatalf("dropped visible payload not reported: %+v", report)
	}
	if len(report.Attachments) != 1 {
		t.Fatalf("expected recovered diagram attachment, got %+v", report.Attachments)
	}
	if got := report.Attachments[0].Body; !strings.Contains(got, "sequenceDiagram") || !strings.Contains(got, "User->>Agent") {
		t.Fatalf("diagram body not preserved in attachment: %q", got)
	}
}

func TestEmitAnswerDocumentV2_BraceFallbackPreservesDroppedDiagramAttachment(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"sum1\",\"kind\":\"summary\",\"text\":\"hi\"},{\"id\":\"diag1\",\"kind\":\"diagram\",\"diagram\":{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\n    User->>Agent: \"ask\"}}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], trailing"}`)
	res, err := tool.Execute(bus, raw)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("lossy brace fallback should still accept the valid structured doc with attachment preserved; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Kind != types.BlockSummary {
		t.Fatalf("structured summary doc not written: %+v", doc)
	}
	attachments := bus.Mutable.AnswerDisplayAttachments()
	if len(attachments) != 1 {
		t.Fatalf("recovered display attachment not stored: %+v", attachments)
	}
	if !strings.Contains(attachments[0].Body, "sequenceDiagram") {
		t.Fatalf("stored attachment lost diagram body: %+v", attachments[0])
	}
}

func TestEmitAnswerDocumentV2_BraceFallbackRejectsUnattachedDroppedBlock(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"sum1\",\"kind\":\"summary\",\"text\":\"hi\"},{\"id\":\"list1\",\"kind\":\"ordered_list\",\"items\":[{\"id\":\"i1\",\"label\":\"broken \"quote\"}]}], trailing"}`)
	res, err := tool.Execute(bus, raw)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if res.Success {
		t.Fatalf("lossy brace fallback that drops a visible list block must fail, got %+v", res)
	}
	if !strings.Contains(res.Summary, "could not preserve every visible blocks[] item") {
		t.Fatalf("failure should explain lossy structured recovery, got %q", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "answer_doc_lossy_blocks_string_recovery" {
		t.Fatalf("lossy blocks[] recovery should carry structured repair metadata, got %+v", res.Repair)
	}
	if got := res.Repair.Metadata["candidate_blocks"]; got != "2" {
		t.Fatalf("repair metadata candidate_blocks = %q, want 2; repair=%+v", got, res.Repair)
	}
	if got := res.Repair.Metadata["recovered_blocks"]; got != "1" {
		t.Fatalf("repair metadata recovered_blocks = %q, want 1; repair=%+v", got, res.Repair)
	}
	if !strings.Contains(res.Repair.Hint, "native JSON array") {
		t.Fatalf("repair hint should explain native blocks[] array, got %q", res.Repair.Hint)
	}
	if doc := bus.Mutable.AnswerDocumentV2(); doc != nil {
		t.Fatalf("partial recovered document must not be published, got %+v", doc)
	}
}

func TestEmitAnswerDocumentV2_BlocksStringStripsDanglingCompositeQuote(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	raw := json.RawMessage(`{"blocks":"[{\"id\":\"sum1\",\"kind\":\"summary\",\"text\":\"hi\"},{\"id\":\"list1\",\"kind\":\"ordered_list\",\"items\":[{\"id\":\"i1\",\"label\":\"step\",\"text\":\"detail\"}]\"},{\"id\":\"caveat1\",\"kind\":\"caveat\",\"text\":\"boundary\"}]"}`)
	res, err := tool.Execute(bus, raw)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("dangling quote after items[] should be repaired without retry, got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("answer document was not persisted")
	}
	if got := len(doc.Blocks); got != 3 {
		t.Fatalf("recovered block count = %d, want 3; doc=%+v", got, doc.Blocks)
	}
	if doc.Blocks[1].Kind != types.BlockOrderedList || len(doc.Blocks[1].Items) != 1 {
		t.Fatalf("ordered list items were not preserved: %+v", doc.Blocks[1])
	}
}

func TestEmitAnswerDocumentV2_PromotesRecoveredDiagramWhenRequired(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks:        []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "answer"}},
	}
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{{
			Kind:     types.BlockDiagram,
			Required: true,
			MinCount: 1,
			FacetIDs: []string{"diagram_spine"},
		}},
	}
	report := answerDocumentRecoveryReport{
		Attachments: []types.AnswerDisplayAttachment{{
			Kind:     types.AnswerDisplayAttachmentDiagram,
			Language: "mermaid",
			Body:     "flowchart TD\n  A --> B",
		}},
	}
	if fixed := promoteRecoveredDiagramBlocks(doc, view, report); fixed != 1 {
		t.Fatalf("expected recovered diagram promoted, got %d doc=%+v", fixed, doc)
	}
	if len(doc.Blocks) != 2 || doc.Blocks[1].Kind != types.BlockDiagram || doc.Blocks[1].Diagram == nil {
		t.Fatalf("diagram block not added: %+v", doc.Blocks)
	}
	if got := doc.Blocks[1].FacetIDs; len(got) != 1 || got[0] != "diagram_spine" {
		t.Fatalf("required diagram facets not carried onto promoted block: %+v", got)
	}
}

func TestEmitAnswerDocumentV2_CarryForwardCitationsFromRejectedDraft(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "a.go", Line: 10},
			{File: "b.go", Line: 20},
		},
		Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "old"}},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:    "list",
			Kind:  types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "i", Label: "B", CitationRef: 1}},
		}},
	}
	if fixed := carryForwardCitationsFromRejectedDraft(doc, bus); fixed != 2 {
		t.Fatalf("expected two citations restored, got %d doc=%+v", fixed, doc)
	}
	if len(doc.Citations) != 2 || doc.Citations[1].File != "b.go" {
		t.Fatalf("citations not restored from previous rejected draft: %+v", doc.Citations)
	}
}

func TestEmitAnswerDocumentV2_CarryForwardCitationsFromLiveDraft(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "a.go", Line: 10},
			{File: "b.go", Line: 20},
			{File: "c.go", Line: 30},
		},
		Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "old"}},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "a.go", Line: 10}},
		Blocks: []types.AnswerBlock{{
			ID:    "list",
			Kind:  types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "i", Label: "C", CitationRef: 2}},
		}},
	}
	if fixed := carryForwardCitationsFromRejectedDraft(doc, bus); fixed != 3 {
		t.Fatalf("expected live citation pool restored to ref max, got %d doc=%+v", fixed, doc)
	}
	if len(doc.Citations) != 3 || doc.Citations[2].File != "c.go" {
		t.Fatalf("citations not restored from live answer draft: %+v", doc.Citations)
	}
}

func TestEmitAnswerDocumentV2_MaterializesRequiredCaveatWhenOnlyMissing(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{
			ExpectedBlockKind: types.BlockCaveat,
			MissingMessage:    "emit a caveat",
		}},
	}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks:        []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "answer"}},
	}
	hints := runPreEmitChecks(doc, view, nil)
	if len(hints) != 1 || hints[0].Field != "blocks[].kind=caveat" {
		t.Fatalf("expected single caveat hint before materialization, got %+v", hints)
	}
	hints = append(hints, emitFixHint{
		Field:         "blocks[].claim_uses",
		ExpectedShape: "synthetic sibling advisory",
		Reason:        "materializing the scope caveat must not depend on being the only hint",
	})
	bus := &types.BusContext{Language: "zh"}
	if fixed := materializeRequiredCaveatWhenOnlyMissing(doc, view, hints, bus); fixed != 1 {
		t.Fatalf("expected caveat materialized, got %d doc=%+v", fixed, doc)
	}
	if got := doc.Blocks[len(doc.Blocks)-1].Title; got != "范围说明" {
		t.Fatalf("materialized caveat title = %q, want zh copy", got)
	}
	if got := doc.Blocks[len(doc.Blocks)-1].FacetIDs; !slices.Contains(got, string(types.FacetUncertaintyBoundary)) {
		t.Fatalf("materialized caveat should cover uncertainty boundary facet, got %+v", got)
	}
	if got := doc.Blocks[len(doc.Blocks)-1].Text; strings.Contains(got, "Scope note") || strings.Contains(got, "This answer is limited") {
		t.Fatalf("materialized zh caveat leaked English copy: %q", got)
	}
	if hints = runPreEmitChecks(doc, view, nil); len(hints) != 0 {
		t.Fatalf("materialized caveat should satisfy uncertainty precheck, got %+v", hints)
	}
}

func TestNormalizeClaimUseEvidenceIDsByProjection_DetachesMismatchedCitationBackedAnnotation(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:         "ev-def",
		Source:     "internal/demo.go",
		LineStart:  10,
		AnchorKind: types.AnchorDefinition,
	}})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "internal/demo.go", Line: 10}},
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm:  types.ClaimCallEdge,
				EvidenceID: "ev-def",
			}},
			Items: []types.AnswerBlockItem{{ID: "cite", CitationRef: 0}},
		}},
	}

	if fixed := normalizeClaimUseEvidenceIDsByProjection(doc, bus); fixed != 1 {
		t.Fatalf("expected one incompatible claim_use evidence_id detached, got %d doc=%+v", fixed, doc.Blocks[0].ClaimUses)
	}
	got := doc.Blocks[0].ClaimUses[0]
	if got.EvidenceID != "" || got.ClaimForm != types.ClaimCallEdge {
		t.Fatalf("claim_use should preserve claim form and detach only evidence_id, got %+v", got)
	}
}

func TestNormalizeClaimUseEvidenceIDsByProjection_SkipsUncitedBlocks(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:         "ev-def",
		Source:     "internal/demo.go",
		LineStart:  10,
		AnchorKind: types.AnchorDefinition,
	}})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm:  types.ClaimCallEdge,
				EvidenceID: "ev-def",
			}},
		}},
	}

	if fixed := normalizeClaimUseEvidenceIDsByProjection(doc, bus); fixed != 0 {
		t.Fatalf("uncited block should not be silently repaired, got %d", fixed)
	}
	if got := doc.Blocks[0].ClaimUses[0].EvidenceID; got != "ev-def" {
		t.Fatalf("uncited block claim_use evidence_id should remain intact, got %q", got)
	}
}

func TestEmitAnswerDocumentV2_RejectedAttemptsAccumulateVisibleDiagrams(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}

	first := json.RawMessage(`{
		"blocks": [
			{"id":"d1","kind":"diagram","diagram":{"kind":"flow","language":"mermaid","body":"flowchart TD\n  A --> B"}},
			{"id":"bad1","kind":"diagram"}
		]
	}`)
	res, err := tool.Execute(bus, first)
	if err != nil {
		t.Fatalf("first emit error: %v", err)
	}
	if res.Success {
		t.Fatalf("first malformed emit should fail")
	}

	second := json.RawMessage(`{
		"blocks": [
			{"id":"d2","kind":"diagram","diagram":{"kind":"sequence","language":"mermaid","body":"sequenceDiagram\n  User->>Agent: ask"}},
			{"id":"bad2","kind":"diagram"}
		]
	}`)
	res, err = tool.Execute(bus, second)
	if err != nil {
		t.Fatalf("second emit error: %v", err)
	}
	if res.Success {
		t.Fatalf("second malformed emit should fail")
	}

	attachments := bus.Mutable.AnswerDisplayAttachments()
	if len(attachments) != 2 {
		t.Fatalf("rejected attempts should accumulate unique diagrams, got %+v", attachments)
	}
	if !strings.Contains(attachments[0].Body, "flowchart TD") || !strings.Contains(attachments[1].Body, "sequenceDiagram") {
		t.Fatalf("visible diagram bodies not preserved in order: %+v", attachments)
	}
}

func TestEmitAnswerDocumentV2_RejectedAttemptPreservesVisibleDraftText(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	raw := json.RawMessage(`{
		"blocks": [
			{"id":"s1","kind":"summary","text":"Visible answer body"},
			{"id":"s2","kind":"not_a_real_kind","text":"Malformed but visible"}
		],
		"citations": [{"file":"internal/agent/sub_explorer.go","line":24}]
	}`)
	res, err := tool.Execute(bus, raw)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if res.Success {
		t.Fatal("invalid block kind should reject the structured emit")
	}
	attachments := bus.Mutable.AnswerDisplayAttachments()
	if len(attachments) == 0 {
		t.Fatal("rejected visible draft should be preserved as a display attachment")
	}
	joined := ""
	for _, att := range attachments {
		joined += "\n" + att.Body
	}
	for _, want := range []string{
		"Visible answer body",
		"Malformed but visible",
		"internal/agent/sub_explorer.go:24",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("recovered draft lost %q; attachments=%+v", want, attachments)
		}
	}
}

// TestEmitAnswerDocumentV2_WholeDocumentStringifyAccepted is the
// end-to-end version: a finalizer-shaped LLM emit where the whole
// answer body was JSON.stringify'd into the blocks key MUST now be
// accepted instead of failing 6 retries before answer quality
// degrades (qf_arch run-2 forensic, 2026-05-05).
func TestEmitAnswerDocumentV2_WholeDocumentStringifyAccepted(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{"blocks": "[{\"id\":\"sum1\",\"kind\":\"summary\",\"text\":\"abc\"}], \"citations\": [{\"file\":\"x.go\",\"line\":1}]"}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("whole-document stringify should be repaired and accepted; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("answer document not written: %+v", doc)
	}
	if len(doc.Citations) != 1 {
		t.Errorf("citations not preserved through repair: %+v", doc.Citations)
	}
}

// TestEmitAnswerDocumentV2_FlatModeRejectsMixedV1FieldsAfterRepair
// confirms the repair does NOT bypass downstream V1↔V2 mutual
// exclusion: even if blocks-as-string is repaired, top-level V1
// fields still trigger rejection.
func TestEmitAnswerDocumentV2_FlatModeRejectsMixedV1FieldsAfterRepair(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	mixed := json.RawMessage(`{
		"shape": "explanation",
		"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\"}]"
	}`)
	res, _ := tool.Execute(bus, mixed)
	if res.Success {
		t.Fatal("flat-mode emit with V1 'shape' field must still reject")
	}
}

// ── R4.1 nested-array string-mode tolerance tests ──────────────

// TestRepairNestedArraysAsString_ItemsAsString covers the
// per-block items[] string-encoded case (extending B5-F1's
// blocks[] coverage).
func TestRepairNestedArraysAsString_ItemsAsString(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"A\"},{\"id\":\"i2\",\"label\":\"B\"}]"}
		]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].items" {
		t.Errorf("paths = %v, want [blocks[0].items]", paths)
	}
	// Confirm blocks[0].items is now a real array (not string)
	if !strings.Contains(string(patched), `"items":[{`) {
		t.Errorf("items[] not converted to array: %s", patched)
	}
}

func TestRepairNestedArraysAsString_ItemsAsStringWithExtraCloser(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"A\"}]]"}
		]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].items" {
		t.Errorf("paths = %v, want [blocks[0].items]", paths)
	}
	if !strings.Contains(string(patched), `"items":[{`) {
		t.Errorf("items[] not converted to array: %s", patched)
	}
}

// TestRepairNestedArraysAsString_ClaimUsesAsString covers the
// per-block claim_uses[] string-encoded case.
func TestRepairNestedArraysAsString_ClaimUsesAsString(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "s1", "kind": "summary",
			 "claim_uses": "[{\"claim_form\":\"definition_fact\"}]"}
		]
	}`)
	_, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].claim_uses" {
		t.Errorf("paths = %v, want [blocks[0].claim_uses]", paths)
	}
}

func TestRepairNestedArraysAsString_FacetIDsAndEdgeAnchorsAsString(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "d1", "kind": "diagram",
			 "facet_ids": "[\"current_code_path\"]",
			 "edge_anchors": "[{\"from_node\":\"A\",\"to_node\":\"B\",\"relation_kind\":\"call\"}]"}
		]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 repair paths; got %v", paths)
	}
	if !strings.Contains(string(patched), `"facet_ids":["current_code_path"]`) ||
		!strings.Contains(string(patched), `"edge_anchors":[{`) {
		t.Fatalf("structured block arrays not repaired: %s", patched)
	}
}

func TestRepairNestedArraysAsString_SingletonArrayShapes(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [{
			"id": "d1",
			"kind": "diagram",
			"items": {"id":"i1","label":"A"},
			"columns": "Component",
			"facet_ids": "current_code_path",
			"claim_uses": {"claimForm":"definitionFact","facetId":"current_code_path"},
			"edge_anchors": "{\"fromNode\":\"A\",\"toNode\":\"B\",\"relationKind\":\"Call\"}"
		}]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("singleton object/string array-shape repair must fire")
	}
	for _, want := range []string{
		"blocks[0].items",
		"blocks[0].columns",
		"blocks[0].facet_ids",
		"blocks[0].claim_uses",
		"blocks[0].edge_anchors",
		"blocks[0].claim_uses[0].claimForm->claim_form",
		"blocks[0].claim_uses[0].facetId->facet_id",
		"blocks[0].edge_anchors[0].fromNode->from_node",
		"blocks[0].edge_anchors[0].toNode->to_node",
		"blocks[0].edge_anchors[0].relationKind->relation_kind",
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("repair paths missing %q in %v; patched=%s", want, paths, patched)
		}
	}
	if !strings.Contains(string(patched), `"facet_ids":["current_code_path"]`) ||
		!strings.Contains(string(patched), `"columns":["Component"]`) ||
		!strings.Contains(string(patched), `"items":[{`) ||
		!strings.Contains(string(patched), `"claim_uses":[{`) ||
		!strings.Contains(string(patched), `"edge_anchors":[{`) {
		t.Fatalf("singleton array-shape fields not repaired: %s", patched)
	}
}

func TestRepairStringWrappedArrayFields_TopLevelSingletonShapes(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": {"id":"s1","kind":"summary","text":"lead"},
		"citations": {"file":"a.go","line":1},
		"caveats": "bounded scope"
	}`)
	patched, paths, ok := repairStringWrappedArrayFields(raw)
	if !ok {
		t.Fatal("top-level singleton array-shape repair must fire")
	}
	for _, want := range []string{"blocks", "citations", "caveats"} {
		if !slices.Contains(paths, want) {
			t.Fatalf("repair paths missing %q in %v; patched=%s", want, paths, patched)
		}
	}
	if !strings.Contains(string(patched), `"blocks":[{`) ||
		!strings.Contains(string(patched), `"citations":[{`) ||
		!strings.Contains(string(patched), `"caveats":["bounded scope"]`) {
		t.Fatalf("top-level singleton fields not repaired: %s", patched)
	}
}

func TestRepairStringWrappedArrayFields_RepairsUnescapedQuotesInsideSnippet(t *testing.T) {
	raw := json.RawMessage(`{
		"items": "[{\"anchor_kind\":\"definition\",\"anchor_symbol\":\"slotFor\",\"snippet\":\"focus = \"default\"\\nif prefix == \"\" {\",\"source\":\"internal/agent/explorer.go\",\"line_start\":15760}]"
	}`)
	patched, paths, ok := repairStringWrappedArrayFields(raw)
	if !ok {
		t.Fatal("string-wrapped items array with snippet quotes must be repaired")
	}
	if !slices.Contains(paths, "items") {
		t.Fatalf("repair paths missing items in %v; patched=%s", paths, patched)
	}
	var decoded struct {
		Items []struct {
			Snippet string `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(patched, &decoded); err != nil {
		t.Fatalf("patched payload must decode: %v\n%s", err, patched)
	}
	if len(decoded.Items) != 1 || !strings.Contains(decoded.Items[0].Snippet, `focus = "default"`) {
		t.Fatalf("snippet quote content not preserved: %+v", decoded.Items)
	}
}

func TestRepairNestedArraysAsString_DiagramObjectAsString(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "d1", "kind": "diagram",
			 "diagram": "{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\nA->>B: hi\"}"}
		]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].diagram" {
		t.Errorf("paths = %v, want [blocks[0].diagram]", paths)
	}
	if !strings.Contains(string(patched), `"diagram":{`) {
		t.Fatalf("diagram object not repaired: %s", patched)
	}
}

// TestRepairNestedArraysAsString_MultipleAtOnce confirms the
// repair handles multiple nested-string sites in one emit.
func TestRepairNestedArraysAsString_MultipleAtOnce(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"A\"}]",
			 "claim_uses": "[{\"claim_form\":\"call_edge\"}]"},
			{"id": "list2", "kind": "bullet_list",
			 "items": "[{\"id\":\"i2\",\"label\":\"B\"}]"}
		]
	}`)
	_, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 3 {
		t.Errorf("expected 3 repair paths; got %v", paths)
	}
}

func TestEmitAnswerDocumentV2_RepairsAnnotationCamelCaseShape(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "lead",
			"claim_uses": [{"claimForm": "assignmentFact", "facetId": "current_code_path", "evidenceId": "ev1"}]
		}, {
			"id": "d1",
			"kind": "diagram",
			"diagram": {"kind": "sequence", "language": "mermaid", "body": "sequenceDiagram\nA->>B: hi"},
			"edge_anchors": [{"fromNode": "A", "toNode": "B", "relationKind": "Call", "claimForm": "callEdge"}]
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("camelCase annotation shape should be repaired; got %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("expected repaired doc with two blocks; got %+v", doc)
	}
	if got := doc.Blocks[0].ClaimUses; len(got) != 1 ||
		got[0].ClaimForm != types.ClaimAssignmentFact ||
		got[0].FacetID != "current_code_path" ||
		got[0].EvidenceID != "ev1" {
		t.Fatalf("claim_uses not normalized: %+v", got)
	}
	if got := doc.Blocks[1].EdgeAnchors; len(got) != 1 ||
		got[0].FromNode != "A" ||
		got[0].ToNode != "B" ||
		got[0].RelationKind != types.DiagramRelCall ||
		got[0].ClaimForm != types.ClaimCallEdge {
		t.Fatalf("edge_anchors not normalized: %+v", got)
	}
}

func TestEmitAnswerDocumentV2_RepairsSingletonArrayShapes(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {"kind": "sequence", "language": "mermaid", "body": "sequenceDiagram\nA->>B: hi"},
			"items": {"id": "i1", "label": "A", "text": "entry"},
			"columns": "Component",
			"facet_ids": "current_code_path",
			"claim_uses": {"claimForm": "definitionFact", "facetId": "current_code_path"},
			"edge_anchors": {"fromNode": "A", "toNode": "B", "relationKind": "Call"}
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("singleton array shapes should be repaired; got %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("expected persisted doc, got %+v", doc)
	}
	got := doc.Blocks[0]
	if len(got.Items) != 1 || len(got.Columns) != 1 ||
		len(got.FacetIDs) != 1 || len(got.ClaimUses) != 1 ||
		len(got.EdgeAnchors) != 1 {
		t.Fatalf("singleton fields not repaired into arrays: %+v", got)
	}
	if got.ClaimUses[0].ClaimForm != types.ClaimDefinitionFact ||
		got.EdgeAnchors[0].RelationKind != types.DiagramRelCall {
		t.Fatalf("singleton annotation aliases not normalized: %+v / %+v", got.ClaimUses, got.EdgeAnchors)
	}
}

func TestEmitAnswerDocumentV2_RepairsTopLevelSingletonArrayShapes(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": {"id": "s1", "kind": "summary", "text": "lead", "items": {"id": "c0", "citation_ref": 0}},
		"citations": {"file": "a.go", "line": 1},
		"caveats": "bounded scope"
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("top-level singleton array shapes should be repaired; got %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Citations) != 1 || len(doc.Caveats) != 1 {
		t.Fatalf("top-level singleton fields not persisted as arrays: %+v", doc)
	}
}

func TestEmitAnswerDocumentV2_HoistsItemLevelClaimUses(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "list",
			"kind": "ordered_list",
			"items": [{
				"id": "i1",
				"label": "ContractCheck",
				"text": "checks the final answer contract",
				"claim_uses": [{"claimForm": "definitionFact", "facetId": "current_code_path"}]
			}]
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("item-level claim_uses should be hoisted, got %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("expected persisted doc, got %+v", doc)
	}
	if got := doc.Blocks[0].ClaimUses; len(got) != 1 ||
		got[0].ClaimForm != types.ClaimDefinitionFact ||
		got[0].FacetID != "current_code_path" {
		t.Fatalf("item-level claim_uses were not hoisted and normalized: %+v", got)
	}
}

func TestEmitAnswerDocumentV2_RejectsConflictingAnnotationAlias(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "lead",
			"claim_uses": [{"claim_form": "call_edge", "claimForm": "returnFact"}]
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatal("conflicting alias/canonical fields must stay rejected")
	}
	if !strings.Contains(res.Summary, `claimForm`) {
		t.Fatalf("rejection should name unrepaired alias; got %s", res.Summary)
	}
}

// TestRepairNestedArraysAsString_NoTrigger confirms the negative
// cases (already arrays, absent fields, non-array strings, etc.)
// don't trigger the repair.
func TestRepairNestedArraysAsString_NoTrigger(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"already_array", `{"blocks":[{"id":"b1","kind":"summary","items":[{"id":"i1"}]}]}`},
		{"items_absent", `{"blocks":[{"id":"b1","kind":"summary"}]}`},
		{"items_not_string", `{"blocks":[{"id":"b1","kind":"summary","items":42}]}`},
		{"items_string_not_array", `{"blocks":[{"id":"b1","kind":"summary","items":"plain text"}]}`},
		{"empty", ``},
		{"malformed", `{not json`},
		{"blocks_string_top_level", `{"blocks":"[{}]"}`}, // covered by repairBlocksAsString, NOT this one
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := repairNestedArraysAsString(json.RawMessage(tc.raw))
			if ok {
				t.Errorf("repair fired unexpectedly for %s", tc.name)
			}
		})
	}
}

func TestEmitAnswerDocumentV2_StringWrappedExactResolutionAndDiagram(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"exact_resolution": "{\"status\":\"exact_match\",\"anchor\":\"SubExplorer.Name\"}",
		"blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": "{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\nA->>B: hi\"}"
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("string-wrapped object fields should be repaired and accepted; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || doc.ExactResolution == nil || doc.ExactResolution.Anchor != "SubExplorer.Name" {
		t.Fatalf("exact_resolution not repaired: %+v", doc)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Diagram == nil || doc.Blocks[0].Diagram.Body == "" {
		t.Fatalf("diagram not repaired: %+v", doc)
	}
}

func TestEmitAnswerDocumentV2_StringCitationAndSnippetLineNumbers(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{"id": "s1", "kind": "summary", "text": "Hello"}],
		"citations": [{"file": "internal/agent/sub_explorer.go", "line": "31", "line_end": "33"}],
		"snippets": [{"file": "internal/agent/sub_explorer.go", "start_line": "31", "end_line": "33", "code": "func (s *SubExplorer) Name() string { return \"explorer\" }"}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("string line numbers should be repaired and accepted; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 || doc.Citations[0].Line != 31 || doc.Citations[0].LineEnd != 33 {
		t.Fatalf("citation line numbers not parsed: %+v", doc)
	}
	if len(doc.Snippets) != 1 || doc.Snippets[0].StartLine != 31 || doc.Snippets[0].EndLine != 33 {
		t.Fatalf("snippet line numbers not parsed: %+v", doc.Snippets)
	}
}

// TestEmitAnswerDocumentV2_NestedFlatModeAccepts pins end-to-end
// behaviour: tool Execute path repairs nested string + completes
// the emit successfully.
func TestEmitAnswerDocumentV2_NestedFlatModeAccepts(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"first\"},{\"id\":\"i2\",\"label\":\"second\"}]"}
		]
	}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("flat-mode nested emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("nested string repair must succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("doc not written; got %+v", doc)
	}
	if got := len(doc.Blocks[0].Items); got != 2 {
		t.Errorf("items not deserialised; got %d, want 2", got)
	}
}

// TestEmitAnswerDocumentV2_CitationRefInsideClaimUseRemapped pins
// Phase 1-B (V2 runtime eval followup, 2026-05-04): when the LLM
// places `citation_ref` inside `claim_use` (the u3a-1 forensic
// outlier), the strict-decode error MUST surface the correct
// paths so the next retry sees concrete relocation guidance. A
// bare `unknown field "citation_ref"` reject was burning 5-7 retry
// iters; the remap reduces that to ≤ 2.
func TestEmitAnswerDocumentV2_CitationRefInsideClaimUseRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"blocks": [{
			"id": "b1",
			"kind": "summary",
			"text": "x",
			"claim_uses": [{"claim_form": "definition_fact", "citation_ref": 0}]
		}],
		"citations": [{"file": "f.go", "line": 1}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced citation_ref reject")
	}
	for _, want := range []string{
		`field "citation_ref"`,
		"items[i].citation_ref",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q (Phase 1-B remap broken); got: %s", want, res.Summary)
		}
	}
}

// TestEmitAnswerDocumentV2_FacetIDsInsideClaimUseSingleValueRepairs pins the
// recurring V2 retry failure where the model copies block.facet_ids into
// claim_uses[] as a plural field. When the plural field has exactly one value,
// the schema intent is unambiguous, so the local compatibility layer repairs it
// to singular facet_id instead of forcing a finalizer retry.
func TestEmitAnswerDocumentV2_FacetIDsInsideClaimUseSingleValueRepairs(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"blocks": [{
			"id": "b1",
			"kind": "summary",
			"text": "x",
			"claim_uses": [{"claim_form": "definition_fact", "facet_ids": ["resolved_literal_or_symbol"]}]
		}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if !res.Success {
		t.Fatalf("single facet_ids value inside claim_uses should be repaired, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].ClaimUses) != 1 {
		t.Fatalf("expected repaired claim use, got %+v", doc)
	}
	got := doc.Blocks[0].ClaimUses[0].FacetID
	if got != "resolved_literal_or_symbol" {
		t.Fatalf("facet_id = %q, want resolved_literal_or_symbol", got)
	}
}

func TestEmitAnswerDocumentV2_FacetIDsInsideClaimUseMultiValueStillRejects(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"blocks": [{
			"id": "b1",
			"kind": "summary",
			"text": "x",
			"claim_uses": [{"claim_form": "definition_fact", "facet_ids": ["current_code_path", "diagram_spine"]}]
		}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("multi-value claim_uses[].facet_ids must stay rejected; the system cannot choose one facet for the model")
	}
	if !strings.Contains(res.Summary, `field "facet_ids"`) {
		t.Fatalf("rejection should still name ambiguous facet_ids; got: %s", res.Summary)
	}
}

// TestEmitAnswerDocumentV2_ValueFieldInScalarBlockRemapped pins
// V1-carrier-residue bug fix (u3a forensic, 2026-05-04): scalar
// blocks no longer carry block.value{literal, citation_ref};
// literal goes in block.text and citation in items[0].citation_ref.
// LLMs trained on V1 mental model still emit value{...} on a
// scalar block — strict-decode rejects "unknown field value" and
// the remap MUST direct to the V2 location.
func TestEmitAnswerDocumentV2_ValueFieldInScalarBlockRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{
			"id": "v1",
			"kind": "scalar",
			"surface_role": "principal",
			"value": {"literal": "42", "citation_ref": 0}
		}],
		"citations": [{"file": "f.go", "line": 1}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced value field reject")
	}
	for _, want := range []string{
		`field "value"`,
		"blocks[i].text",
		"blocks[i].items=",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q; got: %s", want, res.Summary)
		}
	}
}

// Boolean/decision block parallel: LLM emits boolean{...} at block
// level (V1 mental model); V2 needs verdict in block.text and
// citation in items[0].
func TestEmitAnswerDocumentV2_BooleanFieldInDecisionBlockRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{
			"id": "d1",
			"kind": "decision",
			"surface_role": "principal",
			"boolean": {"decision": "yes", "rationale": "because", "citation_ref": 0}
		}],
		"citations": [{"file": "f.go", "line": 1}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced boolean field reject")
	}
	for _, want := range []string{
		`field "boolean"`,
		"blocks[i].text",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q; got: %s", want, res.Summary)
		}
	}
}

// TestEmitAnswerDocumentV2_FromNodeInsideClaimUseRemapped pins
// Phase 1-B source-fix follow-on (2026-05-04): from_node /
// to_node moved out of RenderedClaimUse into block-level
// edge_anchors[]. LLMs trained on the old shape may still try to
// place them inside claim_use; the remap directs to the new path.
func TestEmitAnswerDocumentV2_FromNodeInsideClaimUseRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {"kind":"sequence","language":"mermaid","body":"sequenceDiagram\n  A->>B: invoke\n"},
			"claim_uses": [{"claim_form": "call_edge", "from_node": "A", "to_node": "B"}]
		}],
		"citations": []
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced from_node/to_node reject")
	}
	for _, want := range []string{
		`field "from_node"`,
		"blocks[i].edge_anchors[j].from_node",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q; got: %s", want, res.Summary)
		}
	}
}

func TestEmitAnswerDocumentV2_DiagramEdgeAnchorsPromotedToBlock(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	params := json.RawMessage(`{
		"blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {
				"kind": "sequence",
				"language": "mermaid",
				"body": "sequenceDiagram\nA->>B: call",
				"edge_anchors": [{"from_node": "A", "to_node": "B", "relation_kind": "call"}]
			}
		}],
		"citations": []
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("diagram.edge_anchors should be structurally promoted, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("promoted edge anchors not persisted: %+v", doc)
	}
	if got := doc.Blocks[0].EdgeAnchors[0]; got.FromNode != "A" || got.ToNode != "B" || got.RelationKind != "call" {
		t.Fatalf("edge anchor changed during promotion: %+v", got)
	}
}

// TestRepairBlocksAsString_Path_C_ControlCharRecovery confirms
// Path C's structured fallback: when the LLM's stringified blocks
// array contains UNESCAPED control characters inside string values
// (raw \n in a diagram body, etc.), Path A/B both fail because
// json.Unmarshal rejects raw control bytes inside string literals.
// Path C runs a deterministic state-machine pass that re-escapes
// the control chars and retries the same parse.
//
// This test simulates the MiniMax streaming bug where the model
// stringified an array but emitted single-escape \n instead of
// double-escape \\n inside the string.
func TestRepairBlocksAsString_Path_C_ControlCharRecovery(t *testing.T) {
	// Crafted payload: blocks key holds a JSON-encoded string whose
	// content is a valid JSON array EXCEPT one diagram body has a
	// raw newline (0x0A) inside a string literal — invalid per RFC
	// 7159 §7.
	rawWithControlChar := []byte(`{"blocks": "[{\"id\":\"d1\",\"kind\":\"diagram\",\"diagram\":{\"body\":\"flowchart TD
    A --> B\"}}]"}`)
	// Sanity: confirm Path A/B would reject this (raw newline inside
	// the inner JSON string makes json.Unmarshal fail).
	repaired, ok := repairBlocksAsString(rawWithControlChar)
	if !ok {
		t.Fatalf("Path C must rescue control-char-corrupted blocks; got ok=false")
	}
	// The repaired payload must round-trip through the strict V2
	// schema decoder.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(repaired, &probe); err != nil {
		t.Fatalf("repaired payload is not valid JSON: %v\n%s", err, string(repaired))
	}
	blocks, ok := probe["blocks"]
	if !ok || len(blocks) == 0 || blocks[0] != '[' {
		t.Fatalf("repaired blocks is not a real array: got %s", string(blocks))
	}
	var inner []json.RawMessage
	if err := json.Unmarshal(blocks, &inner); err != nil {
		t.Fatalf("repaired blocks array does not parse: %v", err)
	}
	if len(inner) != 1 {
		t.Fatalf("expected 1 block, got %d", len(inner))
	}
	// And the diagram body must round-trip with the newline preserved
	// (escaped as \n in the JSON value, but a literal newline in the
	// decoded Go string).
	var blk struct {
		Diagram struct {
			Body string `json:"body"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(inner[0], &blk); err != nil {
		t.Fatalf("decode block: %v", err)
	}
	if !strings.Contains(blk.Diagram.Body, "flowchart TD") || !strings.Contains(blk.Diagram.Body, "A --> B") {
		t.Errorf("diagram body lost content; got %q", blk.Diagram.Body)
	}
	if !strings.Contains(blk.Diagram.Body, "\n") {
		t.Errorf("diagram body must preserve the newline as a real \\n char; got %q", blk.Diagram.Body)
	}
}

// TestNormalizeControlCharsInJSONStrings_Atomic exercises the
// state-machine in isolation. The recovery contract:
//
//   - raw \n / \r / \t / \f / \b inside a string become escape pairs
//   - other 0x00–0x1F bytes inside strings become \uXXXX
//   - bytes OUTSIDE strings pass through verbatim (so structural
//     braces / brackets / commas are unaffected)
//   - a string literal already-escaped inputs are byte-identical and
//     `changed=false` (fast path)
func TestNormalizeControlCharsInJSONStrings_Atomic(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		change bool
	}{
		{
			name:   "no controls — fast path",
			in:     `[{"a":"b"}]`,
			want:   `[{"a":"b"}]`,
			change: false,
		},
		{
			name:   "raw newline inside string",
			in:     "{\"body\":\"line1\nline2\"}",
			want:   `{"body":"line1\nline2"}`,
			change: true,
		},
		{
			name:   "tab + carriage return",
			in:     "{\"body\":\"a\tb\rc\"}",
			want:   `{"body":"a\tb\rc"}`,
			change: true,
		},
		{
			name:   "newline outside string preserved",
			in:     "[\n  {\"a\":\"b\"}\n]",
			want:   "[\n  {\"a\":\"b\"}\n]",
			change: false,
		},
		{
			name:   "escaped quote inside string ignored",
			in:     `{"a":"x\"y"}`,
			want:   `{"a":"x\"y"}`,
			change: false,
		},
		{
			name:   "low byte gets uXXXX escape",
			in:     "{\"a\":\"x\x07y\"}",
			want:   `{"a":"x\u0007y"}`,
			change: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := normalizeControlCharsInJSONStrings(tc.in)
			if got != tc.want {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
			if changed != tc.change {
				t.Errorf("changed=%v want %v", changed, tc.change)
			}
		})
	}
}
