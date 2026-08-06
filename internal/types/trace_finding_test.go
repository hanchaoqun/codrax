package types_test

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMutableState_FinalAnswerArtifactsAtomic(t *testing.T) {
	m := types.NewMutableState("q")
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "b1", Kind: types.BlockSummary, Text: "hello"}},
	}
	finding := &types.TraceFindingV1{
		SchemaVersion: types.TraceFindingSchemaVersion,
		FindingID:     "f1",
		Unresolved:    &types.TraceUnresolvedDecision{ReasonCode: "insufficient_evidence"},
	}
	m.SetFinalAnswerArtifacts(types.MutationReplaceAll, &types.FinalAnswerArtifactsV1{
		Document:     *doc,
		TraceFinding: finding,
	})
	got := m.FinalAnswerArtifacts()
	if got == nil || got.TraceFinding == nil || got.TraceFinding.FindingID != "f1" {
		t.Fatalf("artifacts = %+v", got)
	}
	if m.AnswerDocumentV2() == nil || len(m.AnswerDocumentV2().Blocks) != 1 {
		t.Fatalf("document missing")
	}
	m.ResetAnswerDocumentV2()
	if m.TraceFinding() != nil || m.AnswerDocumentV2() != nil {
		t.Fatal("reset should clear both document and finding")
	}
}

func TestTraceFindingContract_Active(t *testing.T) {
	if (&types.TraceFindingContract{}).Active() {
		t.Fatal("empty contract should be inactive")
	}
	if !(&types.TraceFindingContract{ShadowOptional: true}).Active() {
		t.Fatal("shadow optional should be active")
	}
}
