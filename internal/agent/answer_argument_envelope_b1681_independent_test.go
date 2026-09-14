package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type b1681IndependentOwnerLLM struct {
	b1677RouteLLM
	selectorOffered bool
}

func (l *b1681IndependentOwnerLLM) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	for _, schema := range schemas {
		if schema.Name != l.call.Name {
			continue
		}
		var root struct{ Properties map[string]json.RawMessage }
		if json.Unmarshal(schema.Parameters, &root) == nil {
			field := "trace_root_causes"
			if l.call.Name == "emit_answer_document_patch" {
				field = "replace_trace_root_causes"
			}
			l.selectorOffered = len(root.Properties[field]) > 0
		}
	}
	return l.b1677RouteLLM.Chat(ctx, messages, schemas, opts)
}

func b1681IndependentOwnerRun(t *testing.T, ctx *types.AgentContext, patch bool, mode, blocks, selection string) *StageOutput {
	t.Helper()
	name, bodyField, selectorField := "emit_answer_document", "blocks", "trace_root_causes"
	if patch {
		name, bodyField, selectorField = "emit_answer_document_patch", "replace_blocks", "replace_trace_root_causes"
	}
	raw := json.RawMessage(`{"` + bodyField + `":` + blocks + `,"` + selectorField + `":` + selection + `}`)
	fake := &b1681IndependentOwnerLLM{b1677RouteLLM: b1677RouteLLM{call: llm.ToolCall{ID: "b1681-owner", Name: name, Params: raw}}}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 1,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: mode}}}, &stubEvaluator{})
	out, err := base.Execute(ctx, &skill.Config{ToolSuggestions: []string{name}})
	if err != nil || out == nil || fake.calls != 1 || !fake.offered || !fake.selectorOffered || len(out.ToolResults) != 1 {
		t.Fatalf("invalid dynamic-schema→registry→owner route: err=%v calls=%d offered=%t selector=%t out=%+v", err, fake.calls, fake.offered, fake.selectorOffered, out)
	}
	return out
}

func b1681IndependentOwnerContext() *types.AgentContext {
	ctx := b1672RootSelectionLoopContext(false)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "The previously accepted model answer."}}})
	return ctx
}

func TestB1681IndependentAgentNestedBodyStagesValidSelection(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, patch := range []bool{false, true} {
			for _, encodedArray := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/patch=%t/string-array=%t", mode, patch, encodedArray), func(t *testing.T) {
					ctx := b1681IndependentOwnerContext()
					previous := ctx.Mutable.AnswerDocumentV2()
					blocks := `[{"arguments":{"id":"summary","kind":"summary","text":"First candidate body."},"arguments":{"id":"summary","kind":"summary","text":"Second candidate body."}}]`
					if encodedArray {
						blocks = string(b1675QuoteJSON(json.RawMessage(blocks), 1))
					}
					out := b1681IndependentOwnerRun(t, ctx, patch, mode, blocks, `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched","description":"CPU waiting is the model's exact selection."}]}`)
					if out.ToolResults[0].Success || !reflect.DeepEqual(previous, ctx.Mutable.AnswerDocumentV2()) {
						t.Errorf("nested body ambiguity acquired answer ownership: %+v", out.ToolResults)
					}
					pending := ctx.Mutable.PendingTraceRootCauseReport()
					if pending == nil || len(pending.RootCauses) != 1 || pending.RootCauses[0].Description != "CPU waiting is the model's exact selection." || ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.TraceRootCauseSelectorRejected() || len(out.ToolResults[0].OptionalCarrierOutcomes) != 0 {
						t.Fatalf("agent rejected entire transaction before valid selector could stage: pending=%+v result=%+v", pending, out.ToolResults)
					}
				})
			}
		}
	}
}

// A report/item JSON string is a supported schema-driven decoding route, not
// arbitrary prose. Its original nested argument alternatives must not be lost
// when the provider's dynamic schema repairs the body or selector first.
func TestB1681IndependentAgentNestedSelectorPreservesBody(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, patch := range []bool{false, true} {
			for _, shape := range []string{"report", "item", "string-report", "string-item"} {
				t.Run(fmt.Sprintf("%s/patch=%t/%s", mode, patch, shape), func(t *testing.T) {
					ctx := b1681IndependentOwnerContext()
					selection := `{"arguments":{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]},"arguments":{"schema_version":2,"root_causes":[]}}`
					if shape == "item" || shape == "string-item" {
						item := `{"arguments":{"candidate_id":"unknown-candidate"},"arguments":{"candidate_id":"candidate-sched"}}`
						if shape == "string-item" {
							item = string(b1675QuoteJSON(json.RawMessage(item), 1))
						}
						selection = `{"schema_version":2,"root_causes":[` + item + `]}`
					} else if shape == "string-report" {
						selection = string(b1675QuoteJSON(json.RawMessage(selection), 1))
					}
					out := b1681IndependentOwnerRun(t, ctx, patch, mode, `[{"id":"summary","kind":"summary","text":"Useful model answer, unaffected by optional selection errors."}]`, selection)
					if !out.ToolResults[0].Success || ctx.Mutable.AnswerDocumentV2().Blocks[0].Text != "Useful model answer, unaffected by optional selection errors." {
						t.Fatalf("optional nested selector caused whole-answer rejection: %+v", out.ToolResults)
					}
					if ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || !ctx.Mutable.TraceRootCauseSelectorRejected() || len(out.ToolResults[0].OptionalCarrierOutcomes) != 1 {
						t.Fatalf("dynamic normalization laundered a nested competing selector: report=%+v pending=%+v result=%+v", ctx.Mutable.TraceRootCauseReport(), ctx.Mutable.PendingTraceRootCauseReport(), out.ToolResults)
					}
				})
			}
		}
	}
}
