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
			"root_causes": []map[string]any{
				{
					"category": "cpu_scheduling_delay", "thread_name": "RenderThread",
					"impact_seconds": 0.0124,
					"evidence":       []string{"runnable 12.4 ms，期间未获得 CPU"},
				},
				{
					"category": "lock_contention", "resource_name": "ClassLinker classes lock",
					"impact_seconds": 0.0081,
					"evidence":       []string{"Worker 等待该锁 8.1 ms"},
				},
				{
					"category": "synchronous_binder", "thread_name": "UIThread",
					"impact_seconds": 0.003,
					"evidence":       []string{"同步 Binder 等待 3 ms"},
				},
			},
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
	if report == nil || len(report.RootCauses) != 3 || report.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("normalized report was not stored: %#v", report)
	}
	for index, cause := range report.RootCauses {
		if cause.Rank != index+1 || cause.ImpactSeconds == nil || *cause.ImpactSeconds <= 0 {
			t.Fatalf("root_causes[%d] was not ranked/quantified: %#v", index, cause)
		}
	}
}
