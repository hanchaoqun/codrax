package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPatchRootCauseFullFieldCarrierDoesNotLoseExactSelection(t *testing.T) {
	for _, test := range []struct {
		name    string
		version any
		carrier any
	}{
		{"native full object", nil, map[string]any{"schema_version": 2, "root_causes": []map[string]any{{"candidate_id": "candidate-sched"}}}},
		{"string full object", nil, `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`},
		{"native object outer version", "2", map[string]any{"root_causes": []map[string]any{{"candidate_id": "candidate-sched"}}}},
		{"string object outer version", "2", `{"root_causes":[{"candidate_id":"candidate-sched"}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutable := types.NewMutableState("trace")
			mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
			mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2",
				Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "unchanged model conclusion"}}})
			params := map[string]any{"unchanged_block_ids": []string{"summary"}, "trace_root_causes": test.carrier}
			if test.version != nil {
				params["schema_version"] = test.version
			}
			raw, _ := json.Marshal(params)
			result, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mutable}, raw)
			if err != nil || !result.Success {
				t.Fatalf("patch: %v %+v", err, result)
			}
			report := mutable.TraceRootCauseReport()
			if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "RenderThread" {
				t.Fatalf("unambiguous model selection disappeared: %+v", report)
			}
			if mutable.AnswerDocumentV2().Blocks[0].Text != "unchanged model conclusion" {
				t.Fatal("repair changed model conclusion")
			}
		})
	}
}

func TestPatchRootCauseCarrierRepairDoesNotChooseOrInvent(t *testing.T) {
	for _, raw := range []string{
		`{"replace_trace_root_causes":null,"trace_root_causes":{"root_causes":[]}}`,
		`{"replace_trace_root_causes":{},"trace_root_causes":{"root_causes":[]}}`,
		`{"trace_root_causes":{"root_causes":[]},"trace_root_causes":{"root_causes":[{"candidate_id":"other"}]}}`,
		`{"trace_root_causes":{"root_causes":[]},"replace_blocks":[],"replace_blocks":[]}`,
		`{"trace_root_causes":{"root_causes":[],"root_causes":[{"candidate_id":"other"}]}}`,
		`{"trace_root_causes":{"schema_version":2,"schema_version":3,"root_causes":[]}}`,
		`{"trace_root_causes":"{broken"}`,
		`{"trace_root_causes":"[]"}`,
		`{"trace_root_causes":null}`,
		`{"trace_root_causes":{"schema_version":2}}`,
		`{"replace_blocks":[]}`,
	} {
		got, repaired := normalizeMisroutedTraceRootCausePatchField(json.RawMessage(raw))
		if repaired || string(got) != raw {
			t.Fatalf("ambiguous/incomplete input changed: %s -> %s", raw, got)
		}
	}
	raw := json.RawMessage(`{"trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"second"},{"candidate_id":"first"}]},"unchanged_block_ids":["summary"]}`)
	got, repaired := normalizeMisroutedTraceRootCausePatchField(raw)
	if !repaired {
		t.Fatal("exact field move failed")
	}
	var decoded struct {
		Report types.TraceRootCauseReportV2 `json:"replace_trace_root_causes"`
		Old    json.RawMessage              `json:"trace_root_causes"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Old != nil || decoded.Report.SchemaVersion != 2 || len(decoded.Report.RootCauses) != 2 ||
		decoded.Report.RootCauses[0].CandidateID != "second" || decoded.Report.RootCauses[1].CandidateID != "first" {
		t.Fatalf("repair rewrote selection: %s", got)
	}
}

func TestPatchRootCauseCarrierCannotOverrideExplicitCanonicalOrPriorReport(t *testing.T) {
	mutable := types.NewMutableState("trace")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "model conclusion"}}})
	ctx := &types.BusContext{Mutable: mutable}
	first := json.RawMessage(`{"unchanged_block_ids":["summary"],"replace_trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}}`)
	if result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, first); err != nil || !result.Success {
		t.Fatalf("seed: %v %+v", err, result)
	}
	want, _ := json.Marshal(mutable.TraceRootCauseReport())
	for _, fields := range []string{
		`"replace_trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"unknown"}]},"trace_root_causes":{"schema_version":2,"root_causes":[]}`,
		`"trace_root_causes":{"schema_version":3,"root_causes":[]}`,
		`"trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"unknown"}]}`,
	} {
		params := json.RawMessage(`{"unchanged_block_ids":["summary"],` + fields + `}`)
		result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, params)
		if err != nil || !result.Success {
			t.Fatalf("optional selector blocked answer: %v %+v", err, result)
		}
		got, _ := json.Marshal(mutable.TraceRootCauseReport())
		if string(got) != string(want) || mutable.AnswerDocumentV2().Blocks[0].Text != "model conclusion" {
			t.Fatalf("invalid/ambiguous selector changed accepted artifacts: %s", got)
		}
	}
}
