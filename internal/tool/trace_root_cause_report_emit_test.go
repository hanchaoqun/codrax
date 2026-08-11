package tool

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitAnswerDocumentStoresNormalizedRootCauseReportBesideUnchangedDocument(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(&types.TraceFindingContract{
		RootCauseReportRequired: true,
		CandidateSetID:          "trace-report-test",
	})
	ctx := &types.BusContext{Mutable: mutable}
	raw, err := json.Marshal(map[string]any{
		"blocks": []map[string]any{{
			"id": "summary", "kind": "summary", "text": "original full answer",
		}},
		"trace_root_causes": map[string]any{
			"schema_version": types.TraceRootCauseReportSchemaVersion,
			"root_cause_1": map[string]any{
				"category": "cpu_scheduling_delay", "thread_name": "RenderThread",
				"evidence": []string{"runnable 12.4 ms，期间未获得 CPU"},
			},
			"root_cause_2": nil,
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
	if report == nil || report.RootCause1 == nil || report.RootCause1.Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("normalized report was not stored: %#v", report)
	}
	if report.RootCause2 != nil {
		t.Fatalf("null second cause was not preserved: %#v", report.RootCause2)
	}
}
