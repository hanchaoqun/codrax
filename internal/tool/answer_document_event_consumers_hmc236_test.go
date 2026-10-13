package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeEventConsumerFixtureHMC236(t *testing.T) (*types.BusContext, *types.AnswerSemanticView, string) {
	t.Helper()
	dir := t.TempDir()
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("附件事件记录"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	bus.Mutable.SetRequestModel(rm)
	path, err := filepath.Abs("../../eval/fixtures/trace_query_jank_field_inventory/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 100})
	result, err := (&TraceQuery{}).Execute(bus, args)
	if err != nil || !result.Success {
		t.Fatalf("native query: %v / %s", err, result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.EventData != nil && len(table.EventData.Rows) > 1 {
			return bus, view, table.ObservationID
		}
	}
	t.Fatal("native query did not publish event choices")
	return nil, nil, ""
}

func TestNativeEventRecoveryAndPatchPreserveSelectionHMC236(t *testing.T) {
	_, view, id := nativeEventConsumerFixtureHMC236(t)
	selection := &types.RuntimeEventSelection{RowIDs: []string{"event-1"}, FieldKeys: []string{"jank.appid", "jank.start_ts_ns"}}
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		t.Run(string(kind), func(t *testing.T) {
			rec := textRecoveryWithBadSibling(t, map[string]any{"id": "selected", "kind": kind, "runtime_measurement": map[string]any{
				"observation_id": id, "view": "members", "event_selection": selection,
				"bound_table": map[string]any{"Rows": [][]string{{"forged"}}},
			}})
			block := blockByID(t, rec.Document, "selected")
			if block.Kind != kind || block.RuntimeMeasurement.IsBound() || block.RuntimeMeasurement == nil || !reflect.DeepEqual(block.RuntimeMeasurement.EventSelection, selection) {
				t.Fatalf("recovery changed selector/surface or trusted JSON: %+v", block)
			}
			if !types.RebindRuntimeAnswerReceipts(rec.Document, view) {
				t.Fatal("current native selection did not rebind")
			}
			block = blockByID(t, rec.Document, "selected")
			wire := emitAnswerBlockFromTyped(block)
			if wire.RuntimeMeasurement.BoundTable != nil || !reflect.DeepEqual(wire.RuntimeMeasurement.EventSelection, selection) {
				t.Fatal("patch lost selection or serialized private authority")
			}
			wire.RuntimeMeasurement.EventSelection.RowIDs[0] = "event-999"
			if !block.RuntimeMeasurement.IsBound() || block.RuntimeMeasurement.EventSelection.RowIDs[0] != "event-1" {
				t.Fatal("patch selection aliases previous answer")
			}
			raw := json.RawMessage(`{"replace_snippets":[{"block_id":"selected","title":"Display placement"}]}`)
			patched, _, violation := normalizeMisroutedPatchBlockOperations(raw, rec.Document)
			if violation != "" {
				t.Fatal(violation)
			}
			var envelope struct {
				ReplaceBlocks []emitAnswerBlockV2 `json:"replace_blocks"`
			}
			if err := json.Unmarshal(patched, &envelope); err != nil || len(envelope.ReplaceBlocks) != 1 {
				t.Fatalf("patch decode: %v / %s", err, patched)
			}
			if got := envelope.ReplaceBlocks[0].RuntimeMeasurement; got == nil || got.IsBound() || !reflect.DeepEqual(got.EventSelection, selection) {
				t.Fatalf("metadata-only patch widened selection: %+v", got)
			}
		})
	}
}

func TestNativeEventDefaultDisplayNeedsFullTableHMC236(t *testing.T) {
	bus, view, id := nativeEventConsumerFixtureHMC236(t)
	// Event inventories expose explicit selectors without appending a wide
	// automatic table. Exercise coverage independently using a provider that
	// explicitly opts into a complete default projection.
	baseline := &types.AnswerDocumentV2{}
	materializeNativeFactDisplay(baseline, bus, view)
	if len(baseline.Blocks) != 0 {
		t.Fatal("event provider unexpectedly appended a wide default table")
	}
	var allIDs []string
	for i := range view.RuntimeMeasurementContract.Tables {
		table := &view.RuntimeMeasurementContract.Tables[i]
		if table.ObservationID == id {
			table.DefaultPresentation = true
			for _, row := range table.EventData.Rows {
				allIDs = append(allIDs, row.ID)
			}
		}
	}
	for _, tc := range []struct {
		name       string
		kind       types.AnswerBlockKind
		selection  *types.RuntimeEventSelection
		wantBlocks int
	}{
		{"legacy full table", types.BlockTable, nil, 1},
		{"full event table", types.BlockTable, &types.RuntimeEventSelection{}, 1},
		{"explicit all members", types.BlockTable, &types.RuntimeEventSelection{RowIDs: allIDs}, 1},
		{"member subset", types.BlockTable, &types.RuntimeEventSelection{RowIDs: []string{"event-1"}}, 2},
		{"field subset", types.BlockTable, &types.RuntimeEventSelection{FieldKeys: []string{"jank.appid"}}, 2},
		{"grouped table", types.BlockTable, &types.RuntimeEventSelection{GroupBy: []string{"jank.appid"}}, 2},
		{"full summary", types.BlockSummary, &types.RuntimeEventSelection{}, 2},
		{"full fact diagram", types.BlockDiagram, &types.RuntimeEventSelection{}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "selected", Kind: tc.kind, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{
				ObservationID: id, View: types.RuntimeMeasurementMembers, EventSelection: tc.selection,
			}}}}
			for repeat := 0; repeat < 2; repeat++ {
				materializeNativeFactDisplay(doc, bus, view)
				if len(doc.Blocks) != tc.wantBlocks {
					t.Fatalf("pass %d: selected projection suppressed/duplicated full records: %+v", repeat, doc.Blocks)
				}
				if tc.wantBlocks == 2 {
					full := doc.Blocks[1]
					if full.SystemGeneratedKind != types.AnswerSystemGeneratedNativeFacts || !full.RuntimeMeasurement.IsBound() || len(full.RuntimeMeasurement.BoundTable.Rows) != len(allIDs) {
						t.Fatal("default display is not the full native member table")
					}
				}
			}
		})
	}
}
