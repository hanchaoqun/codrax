package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConflictingBlockIDsRetainDraftAndRequestFullEmit(t *testing.T) {
	ctx := newV2TestBusContext()
	result, err := (&EmitAnswerDocument{}).Execute(ctx, json.RawMessage(`{"blocks":[{"id":"lead","kind":"summary","text":"first account"},{"id":"lead","kind":"summary","text":"second account"}]}`))
	if err != nil || result.Success || !strings.Contains(result.Summary, "duplicate") || !strings.Contains(result.Summary, "emit_answer_document") {
		t.Fatalf("need precise actionable identity error: %+v / %v", result, err)
	}
	draft := ctx.Mutable.LastRejectedAnswerDocumentV2()
	if draft == nil || len(draft.Blocks) != 2 || draft.Blocks[0].Text != "first account" || draft.Blocks[1].Text != "second account" {
		t.Fatalf("rejected model content was lost: %+v", draft)
	}
	result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, json.RawMessage(`{"model_block_order":["lead"]}`))
	if err != nil || result.Success || !strings.Contains(result.Summary, "duplicate") || !strings.Contains(result.Summary, "emit_answer_document") {
		t.Fatalf("direct patch must reject before identity-dependent normalization: %+v / %v", result, err)
	}
	result, err = (&EmitAnswerDocument{}).Execute(ctx, json.RawMessage(`{"blocks":[{"id":"lead","kind":"summary","text":"second account"},{"id":"details","kind":"section","text":"first account"}]}`))
	if err != nil || !result.Success {
		t.Fatalf("model-authored full repair must succeed: %+v / %v", result, err)
	}
	if got := ctx.Mutable.AnswerDocumentV2(); got == nil || len(got.Blocks) != 2 || got.Blocks[0].Text != "second account" || got.Blocks[1].Text != "first account" {
		t.Fatalf("full repair changed model content: %+v", got)
	}
}
