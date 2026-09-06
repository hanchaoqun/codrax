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

// Exercise the schema builder used before each model call, not only the
// availability predicate: removing its visibility hook must expose the broken
// patch surface and fail this test. A newer ambiguous pending draft cannot be
// skipped in favor of an older, otherwise addressable accepted document.
func TestDuplicateIDRecoveryToolSchemasHideUnaddressableCurrentBase(t *testing.T) {
	duplicate := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "lead", Kind: types.BlockSummary, Text: "first account"},
		{ID: "lead", Kind: types.BlockSection, Text: "second account"},
	}}
	for _, lane := range []string{"rejected", "retry", "pending", "pending_over_accepted"} {
		t.Run(lane, func(t *testing.T) {
			mut := types.NewMutableState("duplicate identity recovery")
			ctx := &types.AgentContext{Mutable: mut}
			agent, sk := finalizerSchemaTestAgent(), finalizerSchemaTestSkill()
			if lane == "pending_over_accepted" {
				mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
					DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "lead", Kind: types.BlockSummary, Text: "older accepted answer"}},
				})
				if names := finalizerSchemaToolNames(agent.buildToolSchemas(sk, ctx)); !names["emit_answer_document_patch"] {
					t.Fatal("the valid accepted base must offer patch before the newer draft arrives")
				}
			}
			switch lane {
			case "rejected":
				mut.SetLastRejectedAnswerDocumentV2(duplicate)
			case "retry":
				raw, err := json.Marshal(duplicate)
				if err != nil {
					t.Fatal(err)
				}
				mut.SetRetryState(&types.RetryState{PrevEmitJSON: raw})
			default:
				mut.StageAnswerDocumentPatchGeneration(duplicate, nil)
			}
			schemas := agent.buildToolSchemas(sk, ctx)
			names := finalizerSchemaToolNames(schemas)
			if !names["emit_answer_document"] || names["emit_answer_document_patch"] {
				t.Fatalf("current ambiguous draft requires full emit; must not publish an ID-based patch: %v", names)
			}
			evaluator := &answerDocumentEvaluator{mu: mut, preferPatchNext: true, emitFullDocFailStreak: 2}
			filtered := finalizerSchemaToolNames(evaluator.FilterToolSchemas(ctx, schemas))
			if !filtered["emit_answer_document"] || filtered["emit_answer_document_patch"] {
				t.Fatalf("retry filtering must retain the full repair route: %v", filtered)
			}
			if lane == "pending_over_accepted" {
				if prior := mut.AnswerDocumentV2(); prior == nil || len(prior.Blocks) != 1 || prior.Blocks[0].Text != "older accepted answer" {
					t.Fatalf("hiding patch must not overwrite accepted model content: %+v", prior)
				}
			}
		})
	}
}
