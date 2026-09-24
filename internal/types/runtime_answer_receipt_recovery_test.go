package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func runtimeReceiptRecoveryFixture() (*AnswerDocumentV2, *AnswerSemanticView) {
	view := &AnswerSemanticView{
		RuntimeMeasurementContract: &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{{
			ObservationID: "io-query/group", View: RuntimeMeasurementSummary, Label: "IO measurements",
			Columns: []string{"requests", "unknown"}, Rows: [][]string{{"0", "unavailable"}}, Notes: []string{"Window: 1..2 seconds"},
		}}},
		RuntimeWorkRelationContract: &RuntimeWorkRelationContract{Rows: []RuntimeWorkRelationRow{{
			ObservationID: "work-query/span", WorkLabel: "Load image", MeasuredDurationMS: 5,
			AllowedConclusions: []RuntimeWorkRelationConclusion{RuntimeWorkRelationConclusionRelationUnproven},
		}}},
	}
	doc := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{
		{ID: "io", Kind: BlockTable, RuntimeMeasurement: &AnswerRuntimeMeasurementReceipt{
			ObservationID: "io-query/group", View: RuntimeMeasurementSummary}},
		{ID: "work", Kind: BlockSection, RuntimeWorkRelation: &AnswerRuntimeWorkRelationReceipt{
			ObservationID: "work-query/span", Conclusion: RuntimeWorkRelationConclusionRelationUnproven}},
	}}
	return doc, view
}

func TestRebindRuntimeAnswerReceiptsAtomicAndIndependent(t *testing.T) {
	doc, view := runtimeReceiptRecoveryFixture()
	if !RebindRuntimeAnswerReceipts(doc, view) {
		t.Fatal("exact current selections did not bind")
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var restored AnswerDocumentV2
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Blocks[0].RuntimeMeasurement.IsBound() || restored.Blocks[1].RuntimeWorkRelation.IsBound() {
		t.Fatal("JSON exposed private evidence authority")
	}
	if !RebindRuntimeAnswerReceipts(&restored, view) || !reflect.DeepEqual(doc, &restored) {
		t.Fatal("saved selectors did not recover exact current tables and work rows")
	}
	restored.Blocks[0].RuntimeMeasurement.BoundTable.Rows[0][0] = "changed"
	restored.Blocks[0].RuntimeMeasurement.BoundTable.Columns[0] = "changed"
	restored.Blocks[0].RuntimeMeasurement.BoundTable.Notes[0] = "changed"
	restored.Blocks[1].RuntimeWorkRelation.BoundRow.AllowedConclusions[0] = RuntimeWorkRelationConclusionCausalContributionSupported
	if view.RuntimeMeasurementContract.Tables[0].Rows[0][0] != "0" ||
		view.RuntimeMeasurementContract.Tables[0].Columns[0] != "requests" ||
		view.RuntimeMeasurementContract.Tables[0].Notes[0] != "Window: 1..2 seconds" ||
		view.RuntimeWorkRelationContract.Rows[0].AllowedConclusions[0] != RuntimeWorkRelationConclusionRelationUnproven ||
		doc.Blocks[0].RuntimeMeasurement.BoundTable.Rows[0][0] != "0" ||
		doc.Blocks[1].RuntimeWorkRelation.BoundRow.AllowedConclusions[0] != RuntimeWorkRelationConclusionRelationUnproven {
		t.Fatal("restored receipt aliases another document or provider")
	}
	// A failed second receipt must not install the successful first rebind.
	before := cloneAnswerDocumentV2(doc)
	measurementPtr, workPtr := doc.Blocks[0].RuntimeMeasurement, doc.Blocks[1].RuntimeWorkRelation
	view.RuntimeMeasurementContract.Tables[0].Rows[0][0] = "new valid provider value"
	view.RuntimeWorkRelationContract.Rows[0].ObservationID = "different-work"
	if RebindRuntimeAnswerReceipts(doc, view) || !reflect.DeepEqual(doc, before) ||
		doc.Blocks[0].RuntimeMeasurement != measurementPtr || doc.Blocks[1].RuntimeWorkRelation != workPtr {
		t.Fatal("failed all-or-nothing rebind modified candidate")
	}
}

func TestRebindRuntimeAnswerReceiptsRequiresCurrentSupply(t *testing.T) {
	for _, name := range []string{"nil_view", "missing_measurement", "missing_work", "different_window_id", "unsupported_conclusion", "conflicting_selectors"} {
		t.Run(name, func(t *testing.T) {
			doc, view := runtimeReceiptRecoveryFixture()
			if !RebindRuntimeAnswerReceipts(doc, view) {
				t.Fatal("fixture did not bind")
			}
			switch name {
			case "nil_view":
				view = nil
			case "missing_measurement":
				view.RuntimeMeasurementContract = nil
			case "missing_work":
				view.RuntimeWorkRelationContract = nil
			case "different_window_id":
				view.RuntimeMeasurementContract.Tables[0].ObservationID = "io-other-window/group"
			case "unsupported_conclusion":
				doc.Blocks[1].RuntimeWorkRelation.Conclusion = RuntimeWorkRelationConclusionCausalContributionSupported
			case "conflicting_selectors":
				doc.Blocks[0].RuntimeWorkRelation = doc.Blocks[1].RuntimeWorkRelation
			}
			before := cloneAnswerDocumentV2(doc)
			if RebindRuntimeAnswerReceipts(doc, view) || !reflect.DeepEqual(doc, before) {
				t.Fatal("unavailable or conflicting selection accepted or candidate mutated")
			}
		})
	}
	if !RebindRuntimeAnswerReceipts(&AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "plain", Text: "Existing answer"}}}, nil) {
		t.Fatal("legacy answers acquired a receipt requirement")
	}
	if RebindRuntimeAnswerReceipts(nil, nil) {
		t.Fatal("nil document is not a recovery candidate")
	}
}
