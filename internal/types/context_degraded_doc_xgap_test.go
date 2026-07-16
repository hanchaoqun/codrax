package types

// XGAP-FIX ⑤ carrier pins (§29.104.8): the degraded recovery-lane document
// feeds the post-finalize prose defenses without ever impersonating the
// validated carrier.

import "testing"

func TestShippedAnswerDocumentV2_DegradedLaneAndValidatedSupersede(t *testing.T) {
	mu := NewMutableState("xgap shipped doc")

	if mu.ShippedAnswerDocumentV2() != nil {
		t.Fatal("no document shipped yet → nil")
	}

	degraded := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{
		ID: "d", Kind: BlockSummary, Text: "degraded recovery draft",
	}}}
	mu.SetDegradedRecoveredAnswerDocumentV2(degraded)
	if mu.AnswerDocumentV2() != nil {
		t.Fatal("degraded carrier must never leak into the validated carrier")
	}
	got := mu.ShippedAnswerDocumentV2()
	if got == nil || got.Blocks[0].Text != "degraded recovery draft" {
		t.Fatalf("degraded lane must ship through ShippedAnswerDocumentV2, got %+v", got)
	}

	validated := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{
		ID: "v", Kind: BlockSummary, Text: "validated emit",
	}}}
	mu.SetAnswerDocumentV2WithMutation(MutationReplaceAll, validated)
	if mu.DegradedRecoveredAnswerDocumentV2() != nil {
		t.Fatal("a validated emit must clear the stale degraded snapshot")
	}
	got = mu.ShippedAnswerDocumentV2()
	if got == nil || got.Blocks[0].Text != "validated emit" {
		t.Fatalf("validated carrier must win, got %+v", got)
	}
}

func TestSetDegradedRecoveredAnswerDocumentV2_CloneSemantics(t *testing.T) {
	mu := NewMutableState("clone semantics")
	doc := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{
		ID: "d", Kind: BlockSummary, Text: "original",
	}}}
	mu.SetDegradedRecoveredAnswerDocumentV2(doc)
	doc.Blocks[0].Text = "caller-side mutation"
	if got := mu.DegradedRecoveredAnswerDocumentV2(); got.Blocks[0].Text != "original" {
		t.Fatalf("carrier must store a defensive clone, got %q", got.Blocks[0].Text)
	}
}
