package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDuplicateIDRecoveryKeepsFullEmitterAvailable(t *testing.T) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "lead", Kind: types.BlockSummary, Text: "first"},
		{ID: "lead", Kind: types.BlockSummary, Text: "second"},
	}}
	for _, lane := range []string{"rejected", "retry", "pending"} {
		t.Run(lane, func(t *testing.T) {
			mut := types.NewMutableState("answer")
			switch lane {
			case "rejected":
				mut.SetLastRejectedAnswerDocumentV2(doc)
			case "retry":
				raw, _ := json.Marshal(doc)
				mut.SetRetryState(&types.RetryState{PrevEmitJSON: raw})
			case "pending":
				mut.StageAnswerDocumentPatchGeneration(doc, nil)
			}
			ctx := &types.AgentContext{Mutable: mut}
			if answerDocumentPatchBaseAvailable(ctx, nil) {
				t.Fatal("duplicate-id draft advertised as patchable")
			}
			e := &answerDocumentEvaluator{mu: mut, preferPatchNext: true, emitFullDocFailStreak: 2}
			got := e.FilterToolSchemas(ctx, []llm.ToolSchema{{Name: "emit_answer_document"}, {Name: "emit_answer_document_patch"}})
			full := false
			for _, schema := range got {
				full = full || schema.Name == "emit_answer_document"
			}
			if !full {
				t.Fatal("full emission removed despite unaddressable patch base")
			}
		})
	}
}
