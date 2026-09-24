package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func measurementTestTable() RuntimeMeasurementTable {
	return RuntimeMeasurementTable{ObservationID: "trace:A:window1:disk", View: RuntimeMeasurementSummary,
		Label: "Disk requests", Columns: []string{"count", "busy ms", "unavailable"},
		Rows: [][]string{{"0", "2.750", "unknown"}}, Notes: []string{"source A, 1–2 s; no causal authority"}}
}

func TestRuntimeMeasurementReceiptExactBindingAndIsolation(t *testing.T) {
	contract := &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{measurementTestTable()}}
	receipt := &AnswerRuntimeMeasurementReceipt{ObservationID: contract.Tables[0].ObservationID, View: RuntimeMeasurementSummary}
	if !BindRuntimeMeasurementReceipt(receipt, contract) || !receipt.IsBound() {
		t.Fatal("valid measured table did not bind")
	}
	contract.Tables[0].Rows[0][0] = "999"
	contract.Tables[0].Columns[0] = "altered"
	contract.Tables[0].Notes[0] = "altered"
	if receipt.BoundTable.Rows[0][0] != "0" || receipt.BoundTable.Columns[0] != "count" || strings.Contains(receipt.BoundTable.Notes[0], "altered") {
		t.Fatal("bound snapshot aliased the provider contract")
	}
	wire, _ := json.Marshal(receipt)
	if string(wire) != `{"observation_id":"trace:A:window1:disk","view":"summary"}` {
		t.Fatalf("model wire leaked bound values or acquired another required field: %s", wire)
	}
	for _, stale := range []AnswerRuntimeMeasurementReceipt{
		{ObservationID: "trace:B:window1:disk", View: RuntimeMeasurementSummary},
		{ObservationID: "trace:A:window2:disk", View: RuntimeMeasurementSummary},
		{ObservationID: receipt.ObservationID, View: RuntimeMeasurementTimeline},
	} {
		stale.BoundTable = receipt.BoundTable
		if BindRuntimeMeasurementReceipt(&stale, contract) || stale.IsBound() || stale.BoundTable != nil {
			t.Fatal("stale source/window/view retained display authority")
		}
	}
	if BindRuntimeMeasurementReceipt(receipt, nil) || receipt.IsBound() {
		t.Fatal("unavailable supply reused a previous binding")
	}
}

func TestRuntimeMeasurementContractRejectsAmbiguousOrMalformedTables(t *testing.T) {
	table := measurementTestTable()
	for _, malformed := range []RuntimeMeasurementTable{
		{ObservationID: table.ObservationID, View: "root_cause", Columns: table.Columns, Rows: table.Rows},
		{ObservationID: table.ObservationID, View: table.View, Columns: []string{""}},
		{ObservationID: table.ObservationID, View: table.View, Columns: []string{"count"}, Rows: [][]string{{"0", "1"}}},
	} {
		if (&RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{malformed}}).Active() {
			t.Fatal("malformed table became selectable")
		}
	}
	contract := &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{table, table}}
	if contract.Active() {
		t.Fatal("duplicate exact keys must not silently select a publication")
	}
}

func TestRuntimeMeasurementSurvivesMutableSnapshotsAndContractClone(t *testing.T) {
	table := measurementTestTable()
	contract := &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{table}}
	view := cloneAnswerSemanticView(&AnswerSemanticView{RuntimeMeasurementContract: contract})
	if view == nil || !view.RuntimeMeasurementContract.Active() {
		t.Fatal("semantic view clone lost measurement contract")
	}
	view.RuntimeMeasurementContract.Tables[0].Rows[0][0] = "888"
	if contract.Tables[0].Rows[0][0] != "0" {
		t.Fatal("view clone aliases measurement rows")
	}
	receipt := &AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
	BindRuntimeMeasurementReceipt(receipt, contract)
	state := NewMutableState("measurement test")
	state.SetAnswerDocumentV2WithMutation(MutationReplaceAll, &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "m", Kind: BlockTable, RuntimeMeasurement: receipt}}})
	receipt.BoundTable.Rows[0][0] = "777"
	first := state.AnswerDocumentV2()
	if got := first.Blocks[0].RuntimeMeasurement; !got.IsBound() || got.BoundTable.Rows[0][0] != "0" {
		t.Fatal("accepted document snapshot lost/aliased its trusted table")
	}
	first.Blocks[0].RuntimeMeasurement.BoundTable.Notes[0] = "mutated"
	if state.AnswerDocumentV2().Blocks[0].RuntimeMeasurement.BoundTable.Notes[0] == "mutated" {
		t.Fatal("getter permits mutation of accepted data")
	}
}

func TestRuntimeMeasurementPatchClonesEverySelectedTable(t *testing.T) {
	for _, operation := range []string{"unchanged", "replace", "add"} {
		t.Run(operation, func(t *testing.T) {
			table := measurementTestTable()
			receipt := &AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
			BindRuntimeMeasurementReceipt(receipt, &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{table}})
			block := AnswerBlock{ID: "m", Kind: BlockTable, RuntimeMeasurement: receipt}
			previous := &AnswerDocumentV2{Blocks: []AnswerBlock{block}}
			patch := &AnswerDocumentV2Patch{}
			switch operation {
			case "unchanged":
				patch.UnchangedBlockIDs = []string{"m"}
			case "replace":
				patch.ReplaceBlocks = []AnswerBlock{block}
			case "add":
				previous.Blocks = []AnswerBlock{{ID: "lead", Kind: BlockSummary, Text: "Keep model interpretation."}}
				patch.AddBlocks = []AnswerBlock{block}
			}
			merged, err := ApplyAnswerDocumentV2Patch(previous, patch)
			if err != nil {
				t.Fatal(err)
			}
			mergedReceipt := merged.Blocks[len(merged.Blocks)-1].RuntimeMeasurement
			if !mergedReceipt.IsBound() {
				t.Fatal("patch lost the selected trusted snapshot")
			}
			mergedReceipt.BoundTable.Rows[0][0] = "999"
			mergedReceipt.ObservationID = "changed"
			if receipt.ObservationID != table.ObservationID || receipt.BoundTable.Rows[0][0] != "0" {
				t.Fatal("patch result aliases the caller's input receipt")
			}
		})
	}
}

func TestRuntimeMeasurementShapeTeachingKeepsOrdinaryRowsSeparate(t *testing.T) {
	for _, want := range []string{"Ordinary visible list/table rows", "When the projected schema offers runtime_measurement", "without text/items/columns", "interpretation belongs in a separate block"} {
		if !strings.Contains(AnswerDocumentJSONShapeFirstTeaching, want) {
			t.Fatalf("shared shape teaching lost the optional provider-owned alternative: %s", want)
		}
	}
	if strings.Count(AnswerDocumentJSONShapeFirstTeaching, AnswerDocumentItemCitationCarrierTeaching) != 1 {
		t.Fatal("ordinary item evidence teaching was duplicated or lost")
	}
}
