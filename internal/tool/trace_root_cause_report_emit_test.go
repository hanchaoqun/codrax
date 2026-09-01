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

func TestEmitAnswerDocumentRehomesExactMisplacedRootCauseSchemaVersion(t *testing.T) {
	for _, outerVersion := range []any{types.TraceRootCauseReportSchemaVersion, "2"} {
		t.Run("outer_version", func(t *testing.T) {
			mutable := types.NewMutableState("analyze trace root cause")
			mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
			ctx := &types.BusContext{Mutable: mutable}
			raw, err := json.Marshal(map[string]any{
				"blocks": []map[string]any{{
					"id": "summary", "kind": "summary", "text": "model answer remains authoritative",
				}},
				"schema_version": outerVersion,
				"trace_root_causes": map[string]any{
					"root_causes": []map[string]any{{"candidate_id": "candidate-sched"}},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
			if err != nil || !result.Success {
				t.Fatalf("emit failed after lossless carrier repair: result=%+v err=%v", result, err)
			}
			doc := mutable.AnswerDocumentV2()
			if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "model answer remains authoritative" {
				t.Fatalf("visible model answer changed: %#v", doc)
			}
			report := mutable.TraceRootCauseReport()
			if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" {
				t.Fatalf("misplaced exact discriminator did not preserve the selected report: %#v", report)
			}
		})
	}
}

func TestNormalizeMisplacedTraceRootCauseSchemaVersionFailsOpenOnAmbiguity(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "wrong outer version", raw: `{"schema_version":"3","trace_root_causes":{"root_causes":[]}}`},
		{name: "whitespace is not exact", raw: `{"schema_version":" 2 ","trace_root_causes":{"root_causes":[]}}`},
		{name: "nested version already present", raw: `{"schema_version":"2","trace_root_causes":{"schema_version":2,"root_causes":[]}}`},
		{name: "selection carrier absent", raw: `{"schema_version":"2","trace_root_causes":{}}`},
		{name: "optional object absent", raw: `{"schema_version":"2","blocks":[]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			original := json.RawMessage(test.raw)
			repaired, ok := normalizeMisplacedTraceRootCauseSchemaVersion(original, "trace_root_causes")
			if ok || string(repaired) != string(original) {
				t.Fatalf("ambiguous carrier must remain untouched: ok=%v repaired=%s", ok, repaired)
			}
		})
	}
}

func TestNormalizeMisplacedTraceRootCauseSchemaVersionPreservesSelectionOrder(t *testing.T) {
	original := json.RawMessage(`{"schema_version":"2","trace_root_causes":{"root_causes":[{"candidate_id":"first"},{"candidate_id":"second"}]},"blocks":[]}`)
	repaired, ok := normalizeMisplacedTraceRootCauseSchemaVersion(original, "trace_root_causes")
	if !ok {
		t.Fatal("expected exact structural carrier repair")
	}
	var decoded struct {
		SchemaVersion  json.RawMessage `json:"schema_version"`
		TraceRootCause struct {
			SchemaVersion int `json:"schema_version"`
			RootCauses    []struct {
				CandidateID string `json:"candidate_id"`
			} `json:"root_causes"`
		} `json:"trace_root_causes"`
	}
	if err := json.Unmarshal(repaired, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != nil {
		t.Fatalf("top-level discriminator survived: %s", decoded.SchemaVersion)
	}
	if decoded.TraceRootCause.SchemaVersion != types.TraceRootCauseReportSchemaVersion {
		t.Fatalf("nested discriminator = %d", decoded.TraceRootCause.SchemaVersion)
	}
	if got := []string{decoded.TraceRootCause.RootCauses[0].CandidateID, decoded.TraceRootCause.RootCauses[1].CandidateID}; got[0] != "first" || got[1] != "second" {
		t.Fatalf("selection order changed: %v", got)
	}
}

func TestEmitAnswerDocumentPatchRehomesExactMisplacedRootCauseSchemaVersion(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "summary", Kind: types.BlockSummary, Text: "accepted model answer",
		}},
	})
	ctx := &types.BusContext{Mutable: mutable}
	params := json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"schema_version":"2",
		"replace_trace_root_causes":{"root_causes":[{"candidate_id":"candidate-sched"}]}
	}`)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("patch failed after lossless carrier repair: result=%+v err=%v", result, err)
	}
	doc := mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "accepted model answer" {
		t.Fatalf("patch changed the accepted model answer: %#v", doc)
	}
	report := mutable.TraceRootCauseReport()
	if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("patch did not preserve the selected report: %#v", report)
	}
}
