package tool

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// trace_finding_retirement_test.go — V1-5 (§40.16): the legacy Required
// `trace_finding` lane was born dead (its only production producer forced
// Required=false), yet its decoder face stayed open and a stray
// `trace_finding` / `replace_trace_finding` key hard-rejected the WHOLE answer
// ("trace_finding is not enabled for this request" → failEmit). A key that no
// schema publishes and no teaching mentions takes the generic unknown-field
// quarantine lane (drop + log; answer accepted) — never a dedicated reject.

func TestStrayTraceFindingFieldTakesTheGenericQuarantineLane(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contract *types.TraceFindingContract
	}{
		{name: "sidecar-enabled trace context", contract: testSelectableTraceRootCauseContract()},
		{name: "bare context", contract: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutable := types.NewMutableState("analyze trace root cause")
			if tc.contract != nil {
				mutable.SetTraceFindingContract(tc.contract)
			}
			ctx := &types.BusContext{Mutable: mutable}
			raw, err := json.Marshal(map[string]any{
				"blocks": []map[string]any{{
					"id": "summary", "kind": "summary", "text": "full answer survives a stray legacy key",
				}},
				"trace_finding": map[string]any{"schema_version": 1, "finding_id": "x"},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
			if err != nil || !result.Success {
				t.Fatalf("a stray untaught top-level key must be quarantined, not reject the answer: result=%+v err=%v", result, err)
			}
			doc := mutable.AnswerDocumentV2()
			if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "full answer survives a stray legacy key" {
				t.Fatalf("answer document was not persisted: %#v", doc)
			}
			if report := mutable.TraceRootCauseReport(); report != nil {
				t.Fatalf("a stray legacy key must not mint a root-cause report: %#v", report)
			}
		})
	}
}

func TestStrayReplaceTraceFindingPatchFieldTakesTheGenericQuarantineLane(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks:        []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "accepted model answer"}},
	})
	ctx := &types.BusContext{Mutable: mutable}
	result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"replace_trace_finding":{"schema_version":1,"finding_id":"x"}
	}`))
	if err != nil || !result.Success {
		t.Fatalf("a stray untaught top-level patch key must be quarantined, not reject the patch: result=%+v err=%v", result, err)
	}
	if doc := mutable.AnswerDocumentV2(); doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "accepted model answer" {
		t.Fatalf("patch changed the accepted model answer: %#v", doc)
	}
}
