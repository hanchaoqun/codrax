package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc241SelectorTransport(t *testing.T, patch, function, encodedBody, ambiguous bool, mode string) (*BaseAgent, *types.AgentContext, llm.ToolCall, []llm.ToolSchema) {
	t.Helper()
	ctx := b1672RootSelectionLoopContext(false)
	name, key, bodyKey := "emit_answer_document", "trace_root_causes", "blocks"
	body := json.RawMessage(`[{"id":"summary","kind":"summary","text":"Original model answer."}]`)
	if patch {
		name, key, bodyKey = "emit_answer_document_patch", "replace_trace_root_causes", "unchanged_block_ids"
		body = json.RawMessage(`["summary"]`)
		ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "Original model answer."}}})
	}
	if encodedBody {
		body, _ = json.Marshal(string(body))
	}
	selection := `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`
	if ambiguous {
		selection = `{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"unknown","candidate_id":"candidate-sched"}]}`
	}
	inner := json.RawMessage(`{"` + bodyKey + `":` + string(body) + `,"` + key + `":` + selection + `}`)
	wrapped := json.RawMessage(`{"arguments":` + string(inner) + `}`)
	if function {
		encoded, _ := json.Marshal(string(inner))
		wrapped = json.RawMessage(`{"function":{"arguments":` + string(encoded) + `}}`)
	}
	raw, _ := json.Marshal(string(wrapped))
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewBaseAgent(types.AgentFinalizer, &Dependencies{Tools: registry, ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: mode}}}, &answerDocumentEvaluator{})
	schemas := base.buildToolSchemas(&skill.Config{ToolSuggestions: []string{name}}, ctx)
	return base, ctx, llm.ToolCall{ID: "hmc241-selector-transport", Name: name, Params: raw}, schemas
}

func TestHMC241SelectorTransportOneRepairAndCurrentSchemaExecution(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, function := range []bool{false, true} {
			for _, encodedBody := range []bool{false, true} {
				for _, ambiguous := range []bool{false, true} {
					t.Run(fmt.Sprintf("patch=%t/function=%t/string-body=%t/ambiguous=%t", patch, function, encodedBody, ambiguous), func(t *testing.T) {
						base, ctx, call, schemas := hmc241SelectorTransport(t, patch, function, encodedBody, ambiguous, types.ToolParamCompatRepair)
						before, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
						normalized := base.normalizeToolCallParamsWithContext(ctx, []llm.ToolCall{call}, schemas)[0]
						if normalized.ParamSchemaFingerprint == "" || normalized.ParamSchemaValidationError != "" {
							t.Fatalf("approved body transport did not obtain current-schema normalization: %+v", normalized)
						}
						if ambiguous {
							// This owner must normalize and restore at one coordinate.
							// Ordinary valid transports keep their existing tool-boundary
							// normalization path, exercised by the execution below.
							second := base.normalizeToolCallParamsWithContext(ctx, []llm.ToolCall{normalized}, schemas)[0]
							if string(second.Params) != string(normalized.Params) {
								t.Fatalf("one repair must be a fixed point: first=%s second=%s", normalized.Params, second.Params)
							}
						}
						result, _ := base.executeTool(ctx, normalized)
						if result == nil || !result.Success || ctx.Mutable.AnswerDocumentV2() == nil || ctx.Mutable.AnswerDocumentV2().Blocks[0].Text != "Original model answer." {
							t.Fatalf("same-schema dispatch lost the body: payload=%s result=%+v", normalized.Params, result)
						}
						if ambiguous {
							if !ctx.Mutable.TraceRootCauseSelectorRejected() || ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || len(result.OptionalCarrierOutcomes) != 1 {
								t.Fatalf("optional selector was laundered or rejected the answer: %+v", result)
							}
						} else if report := ctx.Mutable.TraceRootCauseReport(); report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "RenderThread" || len(result.OptionalCarrierOutcomes) != 0 {
							t.Fatalf("unambiguous selector lost existing semantics: %+v", result)
						}
						after, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
						if string(before) != string(after) {
							t.Fatal("transport repair modified candidate authority")
						}
					})
				}
			}
		}
	}
}

func TestHMC241SelectorTransportOffAuditAndAmbiguousEnvelopeUnchanged(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, patch := range []bool{false, true} {
			for _, function := range []bool{false, true} {
				for _, encodedBody := range []bool{false, true} {
					t.Run(fmt.Sprintf("mode=%s/patch=%t/function=%t/string-body=%t", mode, patch, function, encodedBody), func(t *testing.T) {
						base, ctx, call, schemas := hmc241SelectorTransport(t, patch, function, encodedBody, true, mode)
						if mode != types.ToolParamCompatRepair {
							got := base.normalizeToolCallParamsWithContext(ctx, []llm.ToolCall{call}, schemas)[0]
							if string(got.Params) != string(call.Params) {
								t.Fatal("off/audit compatibility rewrote the caller payload")
							}
						}
						key := "blocks"
						if patch {
							key = "replace_blocks"
						}
						call.Params = json.RawMessage(`{"arguments":{"` + key + `":[{"id":"summary","kind":"summary","text":"First"}]},"arguments":{"` + key + `":[{"id":"summary","kind":"summary","text":"Second"}]}}`)
						previous := ctx.Mutable.AnswerDocumentV2()
						got := base.normalizeToolCallParamsWithContext(ctx, []llm.ToolCall{call}, schemas)[0]
						if string(got.Params) != string(call.Params) {
							t.Fatal("ambiguous transport was mapped to one model body")
						}
						result, _ := base.executeTool(ctx, got)
						if result == nil || result.Success || !reflect.DeepEqual(previous, ctx.Mutable.AnswerDocumentV2()) {
							t.Fatalf("ambiguous envelope mutated the accepted body: %+v", result)
						}
					})
				}
			}
		}
	}
}
