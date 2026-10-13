package tool

import (
	"encoding/json"
	"html"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func eventDisplayPublicContext236(t *testing.T) (*types.BusContext, types.ObservationRecord) {
	t.Helper()
	ctx := eventSearchInventoryPublicContext(t)
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{}}
	ctx.Mutable = types.NewMutableState("List the jank events and group the counts by application")
	ctx.Mutable.SetRequestModel(rm)
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
	result, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"path":"events.systrace","view":"event_search","event_field_filters":[{"field":"jank_frames","op":"gte","value":"2"}]}`))
	if err != nil || !result.Success {
		t.Fatalf("native query: %v %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return ctx, requireEventSearchInventory(t, result)
}

func eventBlock236(id, kind, observation string, selection map[string]any) map[string]any {
	return map[string]any{"id": id, "kind": kind, "runtime_measurement": map[string]any{
		"observation_id": observation, "view": "members", "event_selection": selection,
	}}
}

func TestRuntimeEventLookupWindowRepublication236(t *testing.T) {
	_, record := eventDisplayPublicContext236(t)
	before, _ := json.Marshal(record.EventSearchInventory)
	records := []types.ObservationRecord{record}
	traceQueryAnnotateLookupWindowContract(records, traceQueryNormalizedWindow{
		RequestedStart: 1.2, RequestedEnd: 1.2, LookupStart: 1.199995, LookupEnd: 1.200005, LookupCaveat: "lookup tolerance",
	})
	publication, valid := types.DecodeRuntimeMeasurementPublication(records[0])
	if !valid {
		t.Fatal("post-query window annotation invalidated native display publication")
	}
	joined := strings.Join(publication.Tables[0].Notes, "\n")
	for _, want := range []string{"requested_window=1.200000..1.200000", "matching_window=1.199995..1.200005", "matching_window_policy=lookup_only_boundary_tolerance", "metric_denominator_window=requested_window"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("native surfaces lost %s", want)
		}
	}
	after, _ := json.Marshal(records[0].EventSearchInventory)
	if string(before) != string(after) || strings.Count(strings.Join(records[0].RichNotes, "\n"), types.TraceNoteKeyRuntimeMeasurement+"=") != 1 {
		t.Fatal("window annotation changed members or duplicated publication")
	}
}

func TestRuntimeEventPublicPublicationEmitPatch236(t *testing.T) {
	ctx, record := eventDisplayPublicContext236(t)
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if !runtimeEventChoicesAvailable(view) {
		t.Fatal("real native inventory not selectable")
	}
	makeBlock := func(kind string) map[string]any {
		return eventBlock236("facts", kind, record.ID, map[string]any{"field_keys": []string{"jank.appid", "jank.start_ts_ns", "jank.jank_frames"}})
	}
	for _, patch := range []bool{false, true} {
		kind := "table"
		if patch {
			kind = "summary"
		}
		input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "Observed application events."}, makeBlock(kind)}}
		if patch {
			input = map[string]any{"replace_blocks": []any{makeBlock(kind)}, "remove_block_ids": []string{"lead"}}
		}
		result := b1659bExecuteAnswer(t, ctx, input, patch)
		if !result.Success {
			t.Fatalf("public emit/patch=%t: %s", patch, result.Summary)
		}
		block := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
		want := [][]string{{"620", "9007199254740993", "2"}, {"621", "9007199254940993", "9"}, {"620", "9007199255740993", "7"}, {"620", "9007199256740993", "4"}}
		if !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, want) {
			t.Fatalf("cross-row app/value mismatch: %+v", block.RuntimeMeasurement)
		}
		visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en"))
		for _, row := range want {
			for _, cell := range row {
				if !strings.Contains(visible, cell) {
					t.Fatalf("native %s invisible: %s", cell, visible)
				}
			}
		}
	}
	input := map[string]any{"replace_blocks": []any{eventBlock236("facts", "summary", record.ID, map[string]any{"group_by": []string{"jank.appid"}})}}
	if result := b1659bExecuteAnswer(t, ctx, input, true); !result.Success {
		t.Fatal(result.Summary)
	}
	block := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
	if !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, [][]string{{"620", "3"}, {"621", "1"}}) {
		t.Fatalf("wrong same-source count: %+v", block.RuntimeMeasurement.BoundTable.Rows)
	}
}

func TestRuntimeEventSchemaNormalizeAllSurfaces236(t *testing.T) {
	ctx, record := eventDisplayPublicContext236(t)
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	view.RequiredBlocks = []types.BlockRequirement{{Kind: types.BlockSummary, Required: true}}
	view.OptionalBlocks = []types.BlockRequirement{{Kind: types.BlockTable}, {Kind: types.BlockDiagram}}
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	for _, kind := range []string{"summary", "table", "diagram"} {
		for _, patch := range []bool{false, true} {
			block := eventBlock236("facts", kind, record.ID, map[string]any{"row_ids": []string{"event-2"}, "field_keys": []string{"jank.appid", "jank.start_ts_ns"}})
			schema, key := BuildAnswerDocumentParametersFor(view), "blocks"
			if patch {
				schema, key = BuildAnswerDocumentPatchParametersFor(view), "replace_blocks"
			}
			validate := func(b map[string]any) error {
				raw, _ := json.Marshal(map[string]any{key: []any{b}})
				return toolparam.Validate(raw, schema)
			}
			if err := validate(block); err != nil {
				t.Fatalf("kind=%s patch=%t: %v", kind, patch, err)
			}
			raw, _ := json.Marshal(block)
			var wire emitAnswerBlockV2
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			canonical, err := NormalizeEmitAnswerBlock(wire, "blocks[0]")
			if err != nil || !runtimeMeasurementBindingValid(canonical, view) {
				t.Fatalf("shape/bind: %v %+v", err, canonical)
			}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{canonical}}
			if err := bindRuntimeMeasurementReceipts(doc, view); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []map[string]any{
				{"row_ids": []string{"event-999"}}, {"field_keys": []string{"made_up"}},
				{"group_by": []string{"jank.appid"}, "field_keys": []string{"jank.appid"}}, {"field_keys": []string{"jank.appid"}, "values": []string{"wrong"}},
			} {
				if validate(eventBlock236("bad", kind, record.ID, bad)) == nil {
					t.Fatalf("bad selector accepted: %+v", bad)
				}
			}
			duplicate := canonical
			duplicate.RuntimeMeasurement = canonical.RuntimeMeasurement.Clone()
			duplicate.RuntimeMeasurement.EventSelection.RowIDs = []string{"event-1", "event-1"}
			if runtimeMeasurementBindingValid(duplicate, view) {
				t.Fatal("duplicate native row acquired bound authority")
			}
			block["text"] = "app 999"
			if validate(block) == nil {
				t.Fatal("competing free text accepted")
			}
		}
	}
	legacy := runtimeMeasurementTestView()
	bad := eventBlock236("bad", "summary", legacy.RuntimeMeasurementContract.Tables[0].ObservationID, map[string]any{})
	bad["runtime_measurement"].(map[string]any)["view"] = "summary"
	raw, _ := json.Marshal(map[string]any{"blocks": []any{bad}})
	if toolparam.Validate(raw, BuildAnswerDocumentParametersFor(legacy)) == nil {
		t.Fatal("legacy measure gained native event authority")
	}
}

func TestRuntimeEventPrincipalSummaryCannotAcquireCausalCaliber236(t *testing.T) {
	ctx, record := eventDisplayPublicContext236(t)
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	view.TraceCausalClaimContract = &types.TraceCausalClaimContract{Allowed: []types.TraceCausalClaimCaliber{types.TraceCausalClaimNoConclusion, types.TraceCausalClaimTypedChain}, Ceiling: types.TraceCausalClaimTypedChain}
	for _, caliber := range []types.TraceCausalClaimCaliber{types.TraceCausalClaimNoConclusion, types.TraceCausalClaimTypedChain} {
		b := eventBlock236("facts", "summary", record.ID, map[string]any{"field_keys": []string{"jank.appid"}})
		b["surface_role"], b["trace_causal_claim_caliber"] = "principal", string(caliber)
		raw, _ := json.Marshal(map[string]any{"blocks": []any{b}})
		err := toolparam.Validate(raw, BuildAnswerDocumentParametersFor(view))
		if (err == nil) != (caliber == types.TraceCausalClaimNoConclusion) {
			t.Fatalf("caliber=%s inconsistent schema: %v", caliber, err)
		}
		blockRaw, _ := json.Marshal(b)
		var wire emitAnswerBlockV2
		if err := json.Unmarshal(blockRaw, &wire); err != nil {
			t.Fatal(err)
		}
		_, err = NormalizeEmitAnswerBlock(wire, "blocks[0]")
		if (err == nil) != (caliber == types.TraceCausalClaimNoConclusion) {
			t.Fatalf("caliber=%s inconsistent normalize: %v", caliber, err)
		}
	}
}
