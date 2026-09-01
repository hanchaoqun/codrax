package tool

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitAnswerDocumentStoresNormalizedRootCauseReportBesideUnchangedDocument(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	ctx := &types.BusContext{Mutable: mutable}
	raw, err := json.Marshal(map[string]any{
		"blocks": []map[string]any{{
			"id": "summary", "kind": "summary", "text": "original full answer",
		}},
		"trace_root_causes": map[string]any{
			"schema_version": types.TraceRootCauseReportSchemaVersion,
			"root_causes":    []map[string]any{{"candidate_id": "candidate-sched"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("emit failed: result=%+v err=%v", result, err)
	}
	doc := mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "original full answer" {
		t.Fatalf("full answer document changed: %#v", doc)
	}
	report := mutable.TraceRootCauseReport()
	if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("normalized report was not stored: %#v", report)
	}
	for index, cause := range report.RootCauses {
		if cause.Rank != index+1 || cause.ImpactSeconds == nil || *cause.ImpactSeconds <= 0 {
			t.Fatalf("root_causes[%d] was not ranked/quantified: %#v", index, cause)
		}
	}
}

func TestEmitAnswerDocumentKeepsFullAnswerWhenOptionalRootCauseSelectorIsInvalid(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	ctx := &types.BusContext{Mutable: mutable}
	raw, err := json.Marshal(map[string]any{
		"blocks": []map[string]any{{
			"id": "summary", "kind": "summary", "text": "useful model answer survives",
		}},
		"trace_root_causes": map[string]any{
			"schema_version": types.TraceRootCauseReportSchemaVersion,
			"root_causes":    []map[string]any{{"candidate_id": "invented-candidate"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("optional sidecar rejected the full answer: result=%+v err=%v", result, err)
	}
	if doc := mutable.AnswerDocumentV2(); doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "useful model answer survives" {
		t.Fatalf("full answer was lost: %#v", doc)
	}
	if report := mutable.TraceRootCauseReport(); report != nil {
		t.Fatalf("invalid sidecar must not be persisted: %#v", report)
	}
}
