package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeRepairDiagnostics284(t *testing.T, result types.ToolResult) []map[string]any {
	t.Helper()
	if result.Success || result.Repair == nil {
		t.Fatalf("expected structured rejection: %+v", result)
	}
	var diagnostics []map[string]any
	if err := json.Unmarshal([]byte(result.Repair.Metadata["runtime_measurement_diagnostics"]), &diagnostics); err != nil || len(diagnostics) == 0 {
		t.Fatalf("missing native repair diagnostics: %v %+v", err, result)
	}
	return diagnostics
}

func TestHMC284NativeSelectorPublicRepairKeepsExactIdentity(t *testing.T) {
	ctx, record := eventDisplayPublicContext236(t)
	wrong := strings.TrimSuffix(record.ID, "#event_search_inventory")
	if wrong == record.ID {
		t.Fatal("fixture must reproduce the missing native inventory suffix")
	}
	selection := map[string]any{"field_keys": []string{"jank.appid"}, "row_ids": []string{"event-2"}}
	bad := eventBlock236("facts", "table", wrong, selection)
	lead := map[string]any{"id": "lead", "kind": "summary", "text": "Observed application events."}
	result := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": []any{lead, bad}}, false)
	diagnostics := nativeRepairDiagnostics284(t, result)
	if diagnostics[0]["field"] != "blocks[1].runtime_measurement.observation_id" || diagnostics[0]["problem"] != "unpublished_observation_id" {
		t.Fatalf("wrong selector location/reason: %+v", diagnostics)
	}
	wire, _ := json.Marshal(diagnostics)
	if !strings.Contains(string(wire), record.ID) || !strings.Contains(result.Summary, record.ID) {
		t.Fatalf("exact current selector is unavailable to repair: %s", result.Summary)
	}
	if doc := ctx.Mutable.AnswerDocumentV2(); doc != nil {
		t.Fatal("a similar prefix was silently granted native authority")
	}
	// A repeated wrong selector remains wrong after the rejected full draft
	// becomes an addressable patch base. No prefix or suffix is guessed.
	result = b1659bExecuteAnswer(t, ctx, map[string]any{"replace_blocks": []any{bad}}, true)
	nativeRepairDiagnostics284(t, result)
	if doc := ctx.Mutable.AnswerDocumentV2(); doc != nil {
		t.Fatal("wrong selector acquired authority through patch recovery")
	}
	result = b1659bExecuteAnswer(t, ctx, map[string]any{"replace_blocks": []any{eventBlock236("facts", "table", record.ID, selection)}}, true)
	if !result.Success {
		t.Fatal(result.Summary)
	}
	block := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
	if !block.RuntimeMeasurement.IsBound() || block.RuntimeMeasurement.ObservationID != record.ID || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, [][]string{{"621"}}) {
		t.Fatalf("explicit exact repair changed source/member facts: %+v", block.RuntimeMeasurement)
	}
}

func TestHMC284NativeSelectionShapePublicFullPatchAtomic(t *testing.T) {
	for _, operation := range []string{"blocks", "replace_blocks", "add_blocks"} {
		t.Run(operation, func(t *testing.T) {
			ctx, record := eventDisplayPublicContext236(t)
			valid := eventBlock236("facts", "summary", record.ID, map[string]any{"field_keys": []string{"jank.appid"}})
			patch := operation != "blocks"
			if patch {
				result := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": []any{valid}}, false)
				if !result.Success {
					t.Fatal(result.Summary)
				}
			}
			before, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			bad := eventBlock236("facts", "summary", record.ID, map[string]any{
				"field_keys": map[string]any{"item": []string{"jank.appid"}},
				"row_ids":    map[string]any{"item": []string{"event-2"}},
			})
			if operation == "add_blocks" {
				bad["id"] = "new-facts"
			}
			result := b1659bExecuteAnswer(t, ctx, map[string]any{operation: []any{bad}}, patch)
			diagnostics := nativeRepairDiagnostics284(t, result)
			if len(diagnostics) != 2 {
				t.Fatalf("all malformed native arrays must be diagnosed together: %+v", diagnostics)
			}
			for i, field := range []string{"row_ids", "field_keys"} {
				if diagnostics[i]["field"] != operation+"[0].runtime_measurement.event_selection."+field || diagnostics[i]["expected_shape"] != "array<string>" || diagnostics[i]["actual_kind"] != "object" {
					t.Fatalf("missing indexed shape diagnostic: %+v", diagnostics[i])
				}
			}
			if !strings.Contains(result.Summary, "array<string>") || strings.Contains(result.Repair.Hint, "jank.appid") {
				t.Fatalf("repair must teach shape, not author selected answer values: %+v", result)
			}
			after, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			if string(before) != string(after) || patch && !strings.Contains(result.Summary, "this call was not staged") {
				t.Fatalf("malformed arrays changed live accepted/patch state: %s", result.Summary)
			}
			// Correct arrays, including explicit empty arrays, retain the native
			// selection contract; the rejected wrappers are never unboxed.
			fixed := eventBlock236("facts", "summary", record.ID, map[string]any{"field_keys": []string{"jank.appid"}, "row_ids": []string{}})
			if operation == "add_blocks" {
				fixed["id"] = "new-facts"
				fixed["kind"] = "table"
			}
			result = b1659bExecuteAnswer(t, ctx, map[string]any{operation: []any{fixed}}, patch)
			if !result.Success {
				t.Fatal(result.Summary)
			}
		})
	}
}

func TestHMC284NativeShapeDoesNotRelabelUnrelatedDecodeError(t *testing.T) {
	ctx, record := eventDisplayPublicContext236(t)
	input := map[string]any{"blocks": []any{eventBlock236("facts", "summary", record.ID, map[string]any{"field_keys": []string{"jank.appid"}})}, "exact_resolution": "not-a-boolean"}
	result := b1659bExecuteAnswer(t, ctx, input, false)
	if result.Success || result.Repair != nil && result.Repair.Metadata["runtime_measurement_diagnostics"] != "" {
		t.Fatalf("ordinary decode error was mislabeled as native selection: %+v", result)
	}
}

func TestHMC284NativePairDiagnosticsCurrentScopeAndBudget(t *testing.T) {
	view := runtimeMeasurementTestView()
	base := view.RuntimeMeasurementContract.Tables[0].Clone()
	for i := 2; i < 22; i++ {
		table := base.Clone()
		table.ObservationID = fmt.Sprintf("source-A:window-%d:block:R", i)
		view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, table)
	}
	// Ambiguous current pairs are not repair candidates, even when their text
	// would look like the missing ID. Choices, schema and binding agree.
	duplicate := base.Clone()
	duplicate.ObservationID = "ambiguous-current"
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, duplicate, duplicate)
	block := types.AnswerBlock{ID: "metrics", Kind: types.BlockTable, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: "source-A:window-999:block:R", View: types.RuntimeMeasurementSummary}}
	d := runtimeMeasurementPairDiagnostic(block, view, "blocks[0].runtime_measurement")
	if d == nil || d.PairCount != 22 || len(d.CurrentPairs) != 8 || d.PairsOmitted != 14 {
		t.Fatalf("candidate count/bounds: %+v", d)
	}
	for _, pair := range d.CurrentPairs {
		ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: pair.ObservationID, View: pair.View}
		if !types.BindRuntimeMeasurementReceipt(ref, view.RuntimeMeasurementContract) || pair.ObservationID == "ambiguous-current" {
			t.Fatalf("unpublished candidate: %+v", pair)
		}
	}
	if runtimeMeasurementBindingValid(block, view) {
		t.Fatal("candidate disclosure healed an unpublished window")
	}
	block.RuntimeMeasurement.ObservationID = base.ObservationID
	block.RuntimeMeasurement.View = types.RuntimeMeasurementMembers
	d = runtimeMeasurementPairDiagnostic(block, view, "blocks[0].runtime_measurement")
	if d == nil || d.Field != "blocks[0].runtime_measurement.view" || d.Problem != "unpublished_observation_view_pair" || d.PairCount != 2 {
		t.Fatalf("view repair borrowed sibling source/window: %+v", d)
	}
	for _, pair := range d.CurrentPairs {
		if pair.ObservationID != base.ObservationID {
			t.Fatalf("view repair changed identity: %+v", pair)
		}
	}
	block.RuntimeMeasurement.View = types.RuntimeMeasurementSummary
	if runtimeMeasurementPairDiagnostic(block, view, "blocks[0].runtime_measurement") != nil {
		t.Fatal("exact valid pair was diagnosed as missing")
	}
	d = runtimeMeasurementPairDiagnostic(block, nil, "blocks[0].runtime_measurement")
	if d == nil || d.PairCount != 0 || d.Problem != "no_current_measurement_pairs" || !strings.Contains(runtimeMeasurementPairRepairAction(d), "omit the optional") {
		t.Fatalf("no supply must not invent candidates: %+v", d)
	}
}

func TestHMC284NativeShapeIndexedItemsAndDecodePrecedence(t *testing.T) {
	ctx, record := eventDisplayPublicContext236(t)
	bad := eventBlock236("facts", "summary", record.ID, map[string]any{
		"row_ids": []any{"event-1", 4}, "field_keys": []any{"jank.appid", true}, "group_by": map[string]any{"item": []any{}},
	})
	result := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": []any{bad}}, false)
	diagnostics := nativeRepairDiagnostics284(t, result)
	if len(diagnostics) != 3 {
		t.Fatalf("incomplete element diagnostics: %+v", diagnostics)
	}
	for i, want := range []struct{ path, kind, shape string }{
		{"row_ids[1]", "number", "string"}, {"field_keys[1]", "boolean", "string"}, {"group_by", "object", "array<string>"},
	} {
		if diagnostics[i]["field"] != "blocks[0].runtime_measurement.event_selection."+want.path || diagnostics[i]["actual_kind"] != want.kind || diagnostics[i]["expected_shape"] != want.shape {
			t.Fatalf("wrong indexed diagnostic: %+v", diagnostics[i])
		}
	}
	// The decoder's first unrelated error must retain precedence, even when a
	// later block also contains malformed native arrays.
	blockJSON, _ := json.Marshal(bad)
	raw := json.RawMessage(`{"exact_resolution":"bad","blocks":[` + string(blockJSON) + `]}`)
	result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || result.Success || result.Repair != nil && result.Repair.Metadata[runtimeMeasurementDiagnosticsKey] != "" {
		t.Fatalf("sibling decoder error was hijacked: %v %+v", err, result)
	}
}

func TestHMC284NativeRepairWholeResponseBudget(t *testing.T) {
	view := runtimeMeasurementTestView()
	longID := "current-source:" + strings.Repeat("exact-identity-", 85) + "#inventory"
	view.RuntimeMeasurementContract.Tables[0].ObservationID = longID
	view.RuntimeMeasurementContract.Tables[1].ObservationID = "current-source:" + strings.Repeat("too-long-", 1024)
	doc := &types.AnswerDocumentV2{}
	for i := 0; i < 80; i++ {
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: fmt.Sprintf("bad-%d", i), Kind: types.BlockTable, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: "missing", View: types.RuntimeMeasurementSummary}})
	}
	projection := runtimeMeasurementPairDiagnostics(doc, view)
	if projection.Total != 80 || len(projection.Entries) >= 80 {
		t.Fatalf("missing global diagnostic omission: %+v", projection)
	}
	encoded, _ := json.Marshal(projection.Entries)
	if len(encoded) > runtimeMeasurementDiagnosticJSONLimit || !strings.Contains(string(encoded), longID) || strings.Contains(string(encoded), "too-long-") {
		t.Fatal("whole projection must preserve or omit complete IDs within its budget")
	}
	hints := preCheckRuntimeMeasurementBindings(doc, view)
	text := formatEmitFixHints(hints)
	if len(hints) != 80 || strings.Count(text, "Native selection diagnostics (") != 1 || strings.Count(text, longID) != 1 {
		t.Fatal("per-block failures must retain their fields but share one candidate roster")
	}
	// Shape fields are bounded too; counting omissions must not allocate or
	// publish a repair field per malformed element in a hostile large array.
	ctx, record := eventDisplayPublicContext236(t)
	badItems := make([]any, 400)
	for i := range badItems {
		badItems[i] = i
	}
	bad := eventBlock236("facts", "summary", record.ID, map[string]any{"field_keys": badItems})
	result := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": []any{bad}}, false)
	diagnostics := nativeRepairDiagnostics284(t, result)
	omitted, err := strconv.Atoi(result.Repair.Metadata["runtime_measurement_diagnostics_omitted"])
	if err != nil || omitted+len(diagnostics) != 400 || len(result.Repair.Fields) != len(diagnostics) || len(diagnostics) > runtimeMeasurementDiagnosticCountLimit {
		t.Fatalf("unbounded fields or inaccurate omissions: %v %+v", err, result.Repair)
	}
	if len(result.Summary)+len(result.Repair.Metadata[runtimeMeasurementDiagnosticsKey])+len(strings.Join(result.Repair.Fields, ",")) > 8192 {
		t.Fatal("shared summary/metadata/field diagnostic budget exceeded")
	}
}
