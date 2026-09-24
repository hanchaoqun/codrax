package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeMeasurementTestView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{{Kind: types.BlockSummary, Required: true}},
		OptionalBlocks: []types.BlockRequirement{{Kind: types.BlockTable}},
		RuntimeMeasurementContract: &types.RuntimeMeasurementContract{Tables: []types.RuntimeMeasurementTable{
			{ObservationID: "source-A:window-1:block:R", View: types.RuntimeMeasurementSummary, Label: "Read requests", Columns: []string{"peak", "busy ms", "unknown"}, Rows: [][]string{{"0", "2.75", "unknown"}}, Notes: []string{"selected window; noncausal"}},
			{ObservationID: "source-A:window-1:block:R", View: types.RuntimeMeasurementTimeline, Label: "Read timeline", Columns: []string{"start s", "end s", "requests"}, Rows: [][]string{{"0", "1", "0"}, {"1", "2", "1"}}},
		}},
	}
}

func TestRuntimeMeasurementProjectedSchemaFullPatchParity(t *testing.T) {
	view := runtimeMeasurementTestView()
	valid := map[string]any{"id": "metrics", "kind": "table", "runtime_measurement": map[string]any{"observation_id": "source-A:window-1:block:R", "view": "summary"}}
	for _, patch := range []bool{false, true} {
		schema := BuildAnswerDocumentParametersFor(view)
		payload := map[string]any{"blocks": []any{valid}}
		if patch {
			schema = BuildAnswerDocumentPatchParametersFor(view)
			payload = map[string]any{"replace_blocks": []any{valid}}
		}
		raw, _ := json.Marshal(payload)
		if err := toolparam.Validate(raw, schema); err != nil {
			t.Fatalf("valid measured selector rejected patch=%t: %v", patch, err)
		}
		for _, field := range []string{"text", "items", "columns", "kind", "runtime_measurement", "unsupported_view", "forged_bound_table"} {
			copy := make(map[string]any, len(valid))
			for k, v := range valid {
				copy[k] = v
			}
			switch field {
			case "text":
				copy[field] = "| wrong |\n|---|\n|999|"
			case "items":
				copy[field] = []any{map[string]any{"cells": []string{"999"}}}
			case "columns":
				copy[field] = []string{"wrong"}
			case "kind":
				copy[field] = "summary"
			case "runtime_measurement":
				copy[field] = map[string]any{"observation_id": "source-A:window-2:block:R", "view": "summary"}
			case "unsupported_view":
				copy["runtime_measurement"] = map[string]any{"observation_id": "source-A:window-1:block:R", "view": "members"}
			case "forged_bound_table":
				copy["runtime_measurement"] = map[string]any{"observation_id": "source-A:window-1:block:R", "view": "summary", "bound_table": map[string]any{"rows": []any{[]string{"999"}}}}
			}
			if patch {
				payload["replace_blocks"] = []any{copy}
			} else {
				payload["blocks"] = []any{copy}
			}
			raw, _ = json.Marshal(payload)
			if err := toolparam.Validate(raw, schema); err == nil {
				t.Fatalf("invalid %s admitted patch=%t", field, patch)
			}
		}
	}
	view.RuntimeMeasurementContract = nil
	_, props := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	if _, exists := props["runtime_measurement"]; exists {
		t.Fatal("empty supply increased schema burden")
	}
	legacy := []byte(`{"blocks":[{"id":"summary","kind":"summary","text":"The selected trace does not establish the requested measurement."}]}`)
	if err := toolparam.Validate(legacy, BuildAnswerDocumentParametersFor(view)); err != nil {
		t.Fatal("optional measurement feature imposed a new presence obligation", err)
	}
}

func TestRuntimeMeasurementSchemaDoesNotBorrowSourceInventoryItems(t *testing.T) {
	view := runtimeMeasurementTestView()
	view.SourceInventoryRowIdentityAvailable = true
	measurement := []byte(`{"blocks":[{"id":"m","kind":"table","surface_role":"principal","runtime_measurement":{"observation_id":"source-A:window-1:block:R","view":"summary"}}]}`)
	schema := BuildAnswerDocumentParametersFor(view)
	if err := toolparam.Validate(measurement, schema); err != nil {
		t.Fatal("runtime measurement table was forced to manufacture source-inventory items", err)
	}
	sourceTable := []byte(`{"blocks":[{"id":"s","kind":"table","surface_role":"principal","text":"| source |\\n|---|\\n|x|"}]}`)
	if err := toolparam.Validate(sourceTable, schema); err == nil {
		t.Fatal("ordinary principal source table lost its row-identity requirement")
	}
}

func TestRuntimeMeasurementKeepsWorkRelationInSeparateBlocks(t *testing.T) {
	view := runtimeMeasurementTestView()
	view.RuntimeWorkRelationContract = &types.RuntimeWorkRelationContract{Rows: []types.RuntimeWorkRelationRow{{
		ObservationID: "work-A", AllowedConclusions: []types.RuntimeWorkRelationConclusion{types.RuntimeWorkRelationConclusionRelationUnproven},
	}}}
	const table = `{"id":"m","kind":"table","runtime_measurement":{"observation_id":"source-A:window-1:block:R","view":"summary"},"runtime_work_relation":{"observation_id":"work-A","conclusion":"relation_unproven"}}`
	for _, patch := range []bool{false, true} {
		schema, payload := BuildAnswerDocumentParametersFor(view), `{"blocks":[`+table+`]}`
		if patch {
			schema, payload = BuildAnswerDocumentPatchParametersFor(view), `{"replace_blocks":[`+table+`]}`
		}
		if err := toolparam.Validate([]byte(payload), schema); err == nil {
			t.Fatal("independently available measured and causal selectors mixed into one table")
		}
	}
	var raw emitAnswerBlockV2
	if err := json.Unmarshal([]byte(table), &raw); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeEmitAnswerBlock(raw, "blocks[0]"); err == nil {
		t.Fatal("direct emit normalization waived the exact selector conflict")
	}
	block := types.AnswerBlock{Kind: types.BlockTable, RuntimeMeasurement: raw.RuntimeMeasurement,
		RuntimeWorkRelation: &types.AnswerRuntimeWorkRelationReceipt{ObservationID: "work-A", Conclusion: types.RuntimeWorkRelationConclusionRelationUnproven}}
	if runtimeMeasurementBindingValid(block, view) {
		t.Fatal("typed patch retained a mixed selector block")
	}
}

func TestRuntimeMeasurementNormalizerAndBindingAreStructural(t *testing.T) {
	view := runtimeMeasurementTestView()
	raw := emitAnswerBlockV2{ID: "metrics", Kind: "table", RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: "source-A:window-1:block:R", View: types.RuntimeMeasurementSummary}}
	block, err := NormalizeEmitAnswerBlock(raw, "blocks[0]")
	if err != nil {
		t.Fatal(err)
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block, {ID: "explanation", Kind: types.BlockSummary, Text: "Interpretation stays model-owned, including 999."}}}
	before := doc.Blocks[1]
	if hints := preCheckExactReceiptBindings(doc, view); len(hints) != 0 {
		t.Fatalf("valid receipt: %+v", hints)
	}
	if doc.Blocks[0].RuntimeMeasurement.IsBound() {
		t.Fatal("precheck mutated draft")
	}
	if err := bindRuntimeMeasurementReceipts(doc, view); err != nil {
		t.Fatal(err)
	}
	if !doc.Blocks[0].RuntimeMeasurement.IsBound() || !reflect.DeepEqual(doc.Blocks[1], before) {
		t.Fatal("binding changed prose or failed to bind")
	}
	for _, bad := range []emitAnswerBlockV2{
		{ID: "bad", Kind: "summary", RuntimeMeasurement: raw.RuntimeMeasurement},
		{ID: "bad", Kind: "table", Text: "999", RuntimeMeasurement: raw.RuntimeMeasurement},
		{ID: "bad", Kind: "table", Columns: []string{"count"}, RuntimeMeasurement: raw.RuntimeMeasurement},
		{ID: "bad", Kind: "table", Items: []emitAnswerBlockItemV2{{Cells: []string{"999"}}}, RuntimeMeasurement: raw.RuntimeMeasurement},
	} {
		if _, err := NormalizeEmitAnswerBlock(bad, "blocks[0]"); err == nil {
			t.Fatal("competing structural payload accepted")
		}
	}
	doc.Blocks[0].RuntimeMeasurement.ObservationID = "source-B:window-1:block:R"
	hints := preCheckExactReceiptBindings(doc, view)
	if len(hints) != 1 || !hints[0].ForceHard || !strings.Contains(hints[0].Field, "runtime_measurement") || strings.Contains(hints[0].ExpectedShape, "conclusion") {
		t.Fatalf("bad selector feedback: %+v", hints)
	}
	if err := bindRuntimeMeasurementReceipts(doc, view); err == nil {
		t.Fatal("forged scope admitted")
	}
	if hints := preCheckExactReceiptBindings(doc, nil); len(hints) != 1 {
		t.Fatal("nil view waived measurement validation")
	}
}

func TestRuntimeMeasurementCompatibilityConversionDropsBoundAuthorityOnly(t *testing.T) {
	view := runtimeMeasurementTestView()
	receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: view.RuntimeMeasurementContract.Tables[0].ObservationID, View: types.RuntimeMeasurementSummary}
	types.BindRuntimeMeasurementReceipt(receipt, view.RuntimeMeasurementContract)
	block := types.AnswerBlock{ID: "m", Kind: types.BlockTable, RuntimeMeasurement: receipt}
	wire := emitAnswerBlockFromTyped(block)
	if wire.RuntimeMeasurement == nil || wire.RuntimeMeasurement.BoundTable != nil || wire.RuntimeMeasurement.ObservationID != receipt.ObservationID {
		t.Fatal("compat wire lost selector or leaked bound data")
	}
	got, err := NormalizeEmitAnswerBlock(wire, "blocks[0]")
	if err != nil || got.RuntimeMeasurement == nil || got.RuntimeMeasurement.IsBound() {
		t.Fatal("compat decode must preserve selector and require fresh bind", err)
	}
}
