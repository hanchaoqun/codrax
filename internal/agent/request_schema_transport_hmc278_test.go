package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc278TransportLLM struct {
	mixedContentToolLLM
	call           llm.ToolCall
	calls          int
	wire           json.RawMessage
	messages       []llm.Message
	window, output int
	secondCall     *llm.ToolCall
}

func (l *hmc278TransportLLM) MaxContextTokens() int { return l.window }
func (l *hmc278TransportLLM) MaxOutputTokens() int  { return l.output }
func (l *hmc278TransportLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	l.messages = append([]llm.Message(nil), messages...)
	for _, schema := range schemas {
		if schema.Name == l.call.Name {
			l.wire = append(json.RawMessage(nil), schema.Parameters...)
		}
	}
	call := l.call
	if l.calls > 1 && l.secondCall != nil {
		call = *l.secondCall
	}
	return llm.Response{ToolCalls: []llm.ToolCall{call}}, nil
}

func hmc278RepeatedSchema(t *testing.T) json.RawMessage {
	t.Helper()
	leaf := map[string]any{"type": "object", "description": strings.Repeat("Exact query identity and field semantics. ", 100), "properties": map[string]any{
		"choices": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"allowed", "other"}}},
	}, "required": []string{"choices"}, "additionalProperties": false}
	properties := map[string]any{}
	for i := 0; i < 8; i++ {
		properties[fmt.Sprintf("slot%d", i)] = leaf
	}
	raw, err := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": []string{"slot0"}, "additionalProperties": false})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The assistant history is captured before registry execution. Merely checking
// execution success would miss a wrong wire-schema normalizer followed by the
// registry's canonical-schema fallback.
func TestHMC278PublicExecuteUsesCanonicalNormalizationAfterWireCompaction(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprintf("valid=%t", valid), func(t *testing.T) {
			canonical := hmc278RepeatedSchema(t)
			original := append(json.RawMessage(nil), canonical...)
			capture := &b1677RouteTool{name: "capture_hmc278", schema: canonical}
			registry := tool.NewRegistry()
			registry.Register(capture)
			raw := json.RawMessage(`{"slot0":{"choices":"[\"allowed\"]"}}`)
			if !valid {
				raw = json.RawMessage(`{"slot0":{"choices":"[\"unknown\"]"}}`)
			}
			fake := &hmc278TransportLLM{call: llm.ToolCall{ID: "hmc278", Name: capture.Name(), Params: raw}, window: 128000, output: 4096}
			eval := &stubEvaluator{}
			mu := types.NewMutableState("canonical tool transport")
			accepted := gapcG13SummaryDoc("Previously accepted answer remains unchanged.")
			mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, accepted)
			before := mu.AnswerDocumentV2()
			base := NewBaseAgent(types.AgentExplorer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 1,
				ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentExplorer: {Mode: types.ToolParamCompatRepair}}}, eval)
			out, err := base.Execute(&types.AgentContext{Stage: types.StageExplore, Mutable: mu}, &skill.Config{ToolSuggestions: []string{capture.Name()}})
			if err != nil || out == nil || fake.calls != 1 {
				t.Fatalf("Execute: calls=%d err=%v out=%+v", fake.calls, err, out)
			}
			if !bytes.Contains(fake.wire, []byte(`"$defs"`)) || !bytes.Contains(fake.wire, []byte(`"$ref"`)) {
				t.Fatalf("LLM did not receive compact schema (%d bytes)", len(fake.wire))
			}
			var seen *llm.ToolCall
			for _, message := range eval.messages {
				if message.Role == "assistant" {
					for _, call := range message.ToolCalls {
						if call.ID == "hmc278" {
							c := call
							seen = &c
						}
					}
				}
			}
			if seen == nil {
				t.Fatal("assistant history missing normalized call")
			}
			if valid && (seen.ParamSchemaFingerprint != toolParamSchemaFingerprint(canonical) || seen.ParamSchemaFingerprint == toolParamSchemaFingerprint(fake.wire)) {
				t.Fatalf("history fingerprint did not use canonical schema: %+v", seen)
			}
			if valid {
				var got struct {
					Slot0 struct {
						Choices []string `json:"choices"`
					} `json:"slot0"`
				}
				if err := json.Unmarshal(seen.Params, &got); err != nil || !reflect.DeepEqual(got.Slot0.Choices, []string{"allowed"}) || seen.ParamSchemaValidationError != "" || capture.calls != 1 {
					t.Fatalf("nested repair failed before dispatch: err=%v call=%+v executions=%d", err, seen, capture.calls)
				}
			} else if seen.ParamSchemaValidationError != "$.slot0.choices[0]: value is not one of the schema enum members" || seen.ParamSchemaFingerprint != "" || capture.calls != 0 {
				t.Fatalf("invalid nested enum reached tool: error=%q calls=%d", seen.ParamSchemaValidationError, capture.calls)
			}
			if !bytes.Equal(canonical, original) || !reflect.DeepEqual(before, mu.AnswerDocumentV2()) {
				t.Fatal("transport mutated canonical schema or previously accepted document")
			}
		})
	}
}

func TestHMC278AcceptedPressureKeeperRequiresCurrentAcceptedAdvisory(t *testing.T) {
	e, ctx, mu := postEmitAdvisoryTwoLaneFixture(t)
	ctx.Stage = types.StageFinalize
	before := mu.AnswerDocumentV2()
	if e.keepAcceptedAnswerUnderPressure(ctx) {
		t.Fatal("accepted answer without delivered advisory bypassed normal turn")
	}
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); !sig.HintRequested {
		t.Fatalf("fixture did not deliver real advisory: %+v", sig)
	}
	if !e.keepAcceptedAnswerUnderPressure(ctx) {
		t.Fatal("current accepted advisory was not preserved")
	}
	other := ctx.ShallowClone()
	other.Mutable = types.NewMutableState("other dispatch")
	other.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, before)
	if e.keepAcceptedAnswerUnderPressure(other) {
		t.Fatal("another dispatch reused advisory qualification")
	}
	other = ctx.ShallowClone()
	other.Stage = types.StageExplore
	if e.keepAcceptedAnswerUnderPressure(other) {
		t.Fatal("non-finalizer reused advisory qualification")
	}
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, nil)
	mu.SetPendingAnswerDocumentPatchBase(before)
	if e.keepAcceptedAnswerUnderPressure(ctx) {
		t.Fatal("pending/unaccepted draft became accepted under pressure")
	}
}

func TestHMC278PublicExecuteHardPressureKeepsCallableTools(t *testing.T) {
	capture := &b1677RouteTool{name: "capture_hmc278", schema: json.RawMessage(`{"type":"object","properties":{}}`)}
	registry := tool.NewRegistry()
	registry.Register(capture)
	fake := &hmc278TransportLLM{call: llm.ToolCall{ID: "last", Name: capture.Name(), Params: json.RawMessage(`{}`)}, window: 10, output: 1}
	eval := &stubEvaluator{}
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 3, AgentSettings: types.AgentSettings{ContextPressureHardRatio: 0.9}}, eval)
	_, err := base.Execute(&types.AgentContext{Stage: types.StageExplore, Language: "zh", Mutable: types.NewMutableState("普通查询")}, &skill.Config{ToolSuggestions: []string{capture.Name()}})
	if err != nil || fake.calls != 1 || capture.calls != 1 || len(fake.wire) == 0 {
		t.Fatalf("hard-pressure final turn lost tools or repeated: err=%v llm=%d executions=%d", err, fake.calls, capture.calls)
	}
	found := false
	for _, m := range fake.messages {
		found = found || strings.Contains(m.Content, "Context window approaching the model's limit")
	}
	if !found {
		t.Fatal("final callable turn omitted pressure teaching")
	}
}

// Use the real emit tool and real evaluator latch: a rejected draft may never
// acquire the pressure escape that belongs to an already accepted answer.
type hmc278GrowingSchemaEvaluator struct {
	*answerDocumentEvaluator
	filters              int
	acceptedBeforeRepair *types.AnswerDocumentV2
	parsed               bool
}

func (e *hmc278GrowingSchemaEvaluator) FilterToolSchemas(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema {
	e.filters++
	out := e.answerDocumentEvaluator.FilterToolSchemas(ctx, schemas)
	if e.filters > 1 && len(out) > 0 {
		e.acceptedBeforeRepair = ctx.Mutable.AnswerDocumentV2()
		out = append([]llm.ToolSchema(nil), out...)
		out[0].Description += strings.Repeat("Exact selector identity, source and boundary teaching. ", 12000)
	}
	return out
}

func (e *hmc278GrowingSchemaEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, results []types.ToolResult, mcp []types.MCPResponse) (*StageOutput, error) {
	e.parsed = true
	return e.answerDocumentEvaluator.ParseOutput(ctx, messages, results, mcp)
}

func TestHMC278PublicExecutePressureSkipsOnlyAcceptedOptionalRepair(t *testing.T) {
	for _, tc := range []struct {
		name          string
		accepted      bool
		window, calls int
	}{
		{"accepted_schema_pressure", true, 128000, 1},
		{"accepted_room_for_repair", true, 1000000, 2},
		{"unaccepted_schema_pressure", false, 128000, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu := types.NewMutableState("列出参与线程")
			ctx := &types.AgentContext{Stage: types.StageFinalize, Language: "zh", Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent: types.IntentExplain, RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Label: "涉及线程清单", Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 1}}},
			}}}
			raw := json.RawMessage(`{"document_model":"v2","blocks":[{"id":"summary","kind":"summary","text":"已有材料尚不足以列出参与线程。"}]}`)
			if !tc.accepted {
				raw = json.RawMessage(`{"document_model":"v2","blocks":[]}`)
			}
			fake := &hmc278TransportLLM{call: llm.ToolCall{ID: "emit", Name: "emit_answer_document", Params: raw}, window: tc.window, output: 4096}
			if tc.accepted {
				fake.secondCall = &llm.ToolCall{ID: "patch", Name: "emit_answer_document_patch", Params: json.RawMessage(`{"unchanged_block_ids":["summary"]}`)}
			}
			registry := tool.NewRegistry()
			registry.Register(&tool.EmitAnswerDocument{})
			registry.Register(&tool.EmitAnswerDocumentPatch{})
			eval := &hmc278GrowingSchemaEvaluator{answerDocumentEvaluator: &answerDocumentEvaluator{}}
			base := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 3, AgentSettings: types.AgentSettings{ContextPressureHardRatio: 0.9}}, eval)
			out, err := base.Execute(ctx, &skill.Config{ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
			if eval.filters != 2 || !eval.parsed {
				t.Fatalf("did not exercise second catalog and ParseOutput: filters=%d parsed=%t", eval.filters, eval.parsed)
			}
			if tc.accepted {
				doc := mu.AnswerDocumentV2()
				if err != nil || out == nil || fake.calls != tc.calls || !eval.postEmitAdvisoryDelivered || doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "已有材料尚不足以列出参与线程。" || !reflect.DeepEqual(eval.acceptedBeforeRepair, doc) {
					t.Fatalf("accepted repair not skipped safely: err=%v calls=%d advisory=%t doc=%+v", err, fake.calls, eval.postEmitAdvisoryDelivered, doc)
				}
			} else if fake.calls != 2 || eval.postEmitAdvisoryDelivered || mu.AnswerDocumentV2() != nil {
				t.Fatalf("unaccepted draft escaped ordinary last-turn validation: calls=%d advisory=%t doc=%+v err=%v", fake.calls, eval.postEmitAdvisoryDelivered, mu.AnswerDocumentV2(), err)
			}
		})
	}
}
