package types

import "testing"

// TestMutableState_StageAnswerDocumentPatchGeneration (V2-2 §40.18 P3): the
// staged base and its lease are installed under one lock by the single
// staging entry point; lease == nil discharges the live lease into the staged
// base; the locked success epilogue clears both.
func TestMutableState_StageAnswerDocumentPatchGeneration(t *testing.T) {
	m := NewMutableState("stage generation")
	base := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{ID: "flow", Kind: BlockDiagram}}}
	lease := &AnswerDiagramRelationRepairLease{Version: 1, Failures: []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
	}}}

	m.StageAnswerDocumentPatchGeneration(base, lease)
	if got := m.PendingAnswerDocumentPatchBase(); got == nil || len(got.Blocks) != 1 || got.Blocks[0].ID != "flow" {
		t.Fatalf("staged base was not installed: %+v", got)
	}
	if got := m.AnswerDiagramRelationRepairLease(); got == nil || len(got.Failures) != 1 {
		t.Fatalf("staged lease was not installed: %+v", got)
	}
	base.Blocks[0].ID = "mutated"
	lease.Failures[0].FromNode = "mutated"
	if got := m.PendingAnswerDocumentPatchBase(); got.Blocks[0].ID != "flow" {
		t.Fatal("staged base must be a defensive copy")
	}
	if got := m.AnswerDiagramRelationRepairLease(); got.Failures[0].FromNode != "A" {
		t.Fatal("staged lease must be a defensive copy")
	}

	m.SetAnswerDiagramRelationRepairLease(&AnswerDiagramRelationRepairLease{Version: 1, Failures: []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "C", ToNode: "D",
	}}})
	m.StageAnswerDocumentPatchGeneration(&AnswerDocumentV2{DocumentModel: "v2"}, nil)
	if got := m.PendingAnswerDocumentPatchBase(); got == nil || len(got.Blocks) != 0 {
		t.Fatalf("staging with a nil lease must still install the base: %+v", got)
	}
	if got := m.AnswerDiagramRelationRepairLease(); got != nil {
		t.Fatalf("staging with a nil lease must discharge the live lease: %+v", got)
	}

	m.StageAnswerDocumentPatchGeneration(base, lease)
	m.SetAnswerDocumentV2WithMutation(MutationPartial, &AnswerDocumentV2{DocumentModel: "v2"})
	if m.PendingAnswerDocumentPatchBase() != nil || m.AnswerDiagramRelationRepairLease() != nil {
		t.Fatal("locked success epilogue must clear the staged generation")
	}
	if m.AnswerDocumentV2() == nil {
		t.Fatal("epilogue must commit the accepted document")
	}
}
