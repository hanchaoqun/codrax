package tool

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// trace_root_cause_report_reemit_test.go — eval witness
// trace_query_donghu_real_frame_multicausal (2026-09-02, colleague_merge_audit
// §40.29.1 ★17): after an accepted selection, the finalizer's later FULL
// re-emit (issued to revise answer blocks) omitted `trace_root_causes` and the
// sidecar shipped as `unavailable`. Omission is not a withdrawal: an emit that
// omits the selector — patch or full — keeps the previously accepted report;
// the model withdraws with an explicit empty selection.
func TestEmitAnswerDocumentReemitWithoutSelectorKeepsAcceptedReport(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	ctx := &types.BusContext{Mutable: mutable}
	emit := func(payload map[string]any) {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
		if err != nil || !result.Success {
			t.Fatalf("emit failed: result=%+v err=%v", result, err)
		}
	}
	// iter 1: the selection is accepted beside the answer.
	emit(map[string]any{
		"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "first full answer"}},
		"trace_root_causes": map[string]any{
			"schema_version": types.TraceRootCauseReportSchemaVersion,
			"root_causes":    []map[string]any{{"candidate_id": "candidate-sched"}},
		},
	})
	accepted := mutable.TraceRootCauseReport()
	if accepted == nil || len(accepted.RootCauses) != 1 {
		t.Fatalf("first emit must store the selection: %#v", accepted)
	}
	// iter 2: a full re-emit that only revises blocks and OMITS the selector.
	emit(map[string]any{
		"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "revised full answer"}},
	})
	if doc := mutable.AnswerDocumentV2(); doc == nil || doc.Blocks[0].Text != "revised full answer" {
		t.Fatalf("the re-emit must replace the answer blocks: %#v", doc)
	}
	if got := mutable.TraceRootCauseReport(); !reflect.DeepEqual(got, accepted) {
		t.Fatalf("omitting the selector on a full re-emit must keep the accepted report, got %#v", got)
	}
	// iter 3: an explicit empty selection withdraws it.
	emit(map[string]any{
		"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "withdrawn"}},
		"trace_root_causes": map[string]any{
			"schema_version": types.TraceRootCauseReportSchemaVersion,
			"root_causes":    []map[string]any{},
		},
	})
	if got := mutable.TraceRootCauseReport(); got == nil || len(got.RootCauses) != 0 || reflect.DeepEqual(got, accepted) {
		t.Fatalf("an explicit empty selection must replace the accepted report with the empty form, got %#v", got)
	}
}

// §40.29.1 ★20: the whole report body lifted to the document top level
// (`root_causes` beside the exact `schema_version`, no carrier) is re-homed
// losslessly; a shape that is not a closed {candidate_id} list is left to the
// unknown-field quarantine (fail open, no invention).
func TestEmitAnswerDocumentRehomesTopLevelRootCausesSelection(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	ctx := &types.BusContext{Mutable: mutable}
	raw, err := json.Marshal(map[string]any{
		"blocks":         []map[string]any{{"id": "summary", "kind": "summary", "text": "flattened by a local model"}},
		"schema_version": types.TraceRootCauseReportSchemaVersion,
		"root_causes":    []map[string]any{{"candidate_id": "candidate-sched"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("emit failed: result=%+v err=%v", result, err)
	}
	if report := mutable.TraceRootCauseReport(); report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("top-level root_causes selection must be re-homed and bound: %#v", report)
	}
	// Negative: items without candidate_id are not a selector — nothing moves.
	mutable = types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	ctx = &types.BusContext{Mutable: mutable}
	raw, _ = json.Marshal(map[string]any{
		"blocks":      []map[string]any{{"id": "summary", "kind": "summary", "text": "prose root causes"}},
		"root_causes": []map[string]any{{"summary": "RenderThread"}},
	})
	result, err = executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("full answer must survive the quarantined field: result=%+v err=%v", result, err)
	}
	if report := mutable.TraceRootCauseReport(); report != nil {
		t.Fatalf("a non-selector top-level root_causes must not mint a report: %#v", report)
	}
}

// §40.31.1 ★16 (eval witness trace_query_frame_semantic_span_optimization,
// 2026-09-02): the model's VALID selection rode a full emit that the answer
// contract rejected; the accepted follow-up patch omitted the selector and
// the sidecar shipped as `unavailable`. A validly bound selector on a
// rejected emit is staged, and an accepted emit/patch that omits the selector
// inherits it; storing any report clears the stage.
func TestEmitAnswerDocumentStagedSelectorSurvivesStructuralRejection(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	ctx := &types.BusContext{Mutable: mutable}
	contract := mutable.TraceFindingContract()
	staged, err := tracefinding.BindRootCauseReportSelection(&types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses:    []*types.TraceRootCauseItemV2{{CandidateID: "candidate-sched"}},
	}, contract)
	if err != nil || staged == nil || len(staged.RootCauses) != 1 {
		t.Fatalf("fixture: bind the selection: %v %+v", err, staged)
	}
	// The mechanism the rejected-emit paths call (full emit + all three patch
	// persist sites): stage the bound selection.
	mutable.SetPendingTraceRootCauseReport(staged)
	if mutable.TraceRootCauseReport() != nil {
		t.Fatal("staging must not publish the report")
	}
	// The accepted patch omits the selector ⇒ inherits the staged selection.
	raw, _ := json.Marshal(map[string]any{
		"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "accepted full answer"}},
	})
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("emit failed: result=%+v err=%v", result, err)
	}
	if got := mutable.TraceRootCauseReport(); got == nil || len(got.RootCauses) != 1 || got.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("an accepted emit that omits the selector must inherit the staged selection: %#v", got)
	}
	if mutable.PendingTraceRootCauseReport() != nil {
		t.Fatal("storing the report must clear the stage")
	}
	// A new finalize dispatch resets the stage with the report.
	mutable.SetPendingTraceRootCauseReport(staged)
	mutable.ResetActiveAnswerDocumentV2ForFinalizeDispatch()
	if mutable.PendingTraceRootCauseReport() != nil || mutable.TraceRootCauseReport() != nil {
		t.Fatal("dispatch reset must clear both the report and the stage")
	}
}
