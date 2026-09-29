package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type multiCarrierMetadataLLM struct {
	traceTeachingCaptureLLM
	t      *testing.T
	bus    *types.BusContext
	before *types.AnswerDocumentV2
}

func (l *multiCarrierMetadataLLM) Chat(_ context.Context, _ []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{StopReason: "tool_use", ToolCalls: []llm.ToolCall{{ID: "draft", Name: "emit_answer_document", Params: json.RawMessage(`{"blocks":[
		{"id":"summary","kind":"summary","text":"The requested records are below."},
		{"id":"opaque-2","kind":"table","title":"First collection","columns":["Name","Elapsed (ms)"],"items":[{"id":"a","cells":["Alpha","23"]}]},
		{"id":"opaque-1","kind":"table","title":"Second collection","columns":["Name","Time (s)"],"items":[{"id":"b","cells":["Beta","1.2"]}]}
		]}`)}}}, nil
	}
	if l.calls != 2 {
		l.t.Fatalf("unexpected repair count %d", l.calls)
	}
	l.before = l.bus.Mutable.AnswerDocumentV2()
	var targets []string
	for _, schema := range schemas {
		if schema.Name != "emit_answer_document_patch" {
			continue
		}
		var root map[string]any
		if err := json.Unmarshal(schema.Parameters, &root); err != nil {
			l.t.Fatal(err)
		}
		props := root["properties"].(map[string]any)
		branches := props["block_field_edits_v1"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
		for _, b := range branches {
			p := b.(map[string]any)["properties"].(map[string]any)
			if p["field"].(map[string]any)["const"] != "add_facet_id" {
				continue
			}
			if !reflect.DeepEqual(p["value"].(map[string]any)["enum"], []any{"member_set"}) {
				continue
			}
			for _, id := range p["block_id"].(map[string]any)["enum"].([]any) {
				targets = append(targets, id.(string))
			}
		}
	}
	if !reflect.DeepEqual(targets, []string{"opaque-2", "opaque-1"}) {
		l.t.Fatalf("real retry did not publish both choices: %v", targets)
	}
	return llm.Response{StopReason: "tool_use", ToolCalls: []llm.ToolCall{{ID: "metadata", Name: "emit_answer_document_patch", Params: json.RawMessage(`{"block_field_edits_v1":[{"block_id":"opaque-2","field":"add_facet_id","value":"member_set"},{"block_id":"opaque-1","field":"add_facet_id","value":"member_set"}]}`)}}}, nil
}

func TestFinalizerMultiCarrierMetadataRepairPreservesTables(t *testing.T) {
	mu := types.NewMutableState("Show the two record collections.")
	bus := &types.BusContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate, RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
		{Index: 1, Label: "First collection", Role: types.RequestedAnswerDimensionMemberSet, Required: true},
		{Index: 2, Label: "Second collection", Role: types.RequestedAnswerDimensionMemberSet, Required: true},
	}}}}}
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocument{})
	reg.Register(&tool.EmitAnswerDocumentPatch{})
	adapter := &multiCarrierMetadataLLM{t: t, bus: bus}
	a := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: reg, MaxIterations: 3})
	_, err := a.Execute(promptctx.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize), traceTeachingSkill(t, "answer-document-skill"))
	if err != nil || adapter.calls != 2 {
		t.Fatalf("real metadata repair failed: calls=%d err=%v", adapter.calls, err)
	}
	got := mu.AnswerDocumentV2()
	if adapter.before == nil || got == nil || len(got.Blocks) != 3 {
		t.Fatal("missing actual documents")
	}
	for i, before := range adapter.before.Blocks {
		after := got.Blocks[i]
		if i > 0 {
			if !reflect.DeepEqual(after.FacetIDs, []string{"member_set"}) {
				t.Fatalf("model choice not applied: %+v", after)
			}
			after.FacetIDs = before.FacetIDs
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("metadata repair changed visible payload: before=%+v after=%+v", before, after)
		}
	}
}
