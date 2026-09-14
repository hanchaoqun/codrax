package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type b1677CountTool struct {
	tool.ReadOnly
	tool.EvidenceTool
	calls int
}

type b1677AnswerLoopLLM struct {
	b1675SelectionLoopLLM
	equivalent bool
}

func (l *b1677AnswerLoopLLM) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, options llm.ChatOptions) (llm.Response, error) {
	response, err := l.b1675SelectionLoopLLM.Chat(ctx, messages, schemas, options)
	if err != nil || (l.patch && l.calls != 2) || (!l.patch && l.calls != 1) {
		return response, err
	}
	for i := range response.ToolCalls {
		a := string(response.ToolCalls[i].Params)
		b := a
		if !l.equivalent {
			b = strings.ReplaceAll(a, `"candidate-sched"`, `"unknown-alternative"`)
		}
		response.ToolCalls[i].Params = json.RawMessage(`{"arguments":` + a + `,"arguments":` + b + `}`)
	}
	return response, nil
}

func TestB1677PublicAgentKeepsAnswerWhenWrappedSelectorsConflict(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, patch := range []bool{false, true} {
			for _, equivalent := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/patch=%t/equivalent=%t", mode, patch, equivalent), func(t *testing.T) {
					ctx := b1672RootSelectionLoopContext(false)
					selection := `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`
					if equivalent {
						selection = `{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`
					}
					fake := &b1677AnswerLoopLLM{b1675SelectionLoopLLM: b1675SelectionLoopLLM{patch: patch, selection: selection, repairBody: true}, equivalent: equivalent}
					registry := tool.NewRegistry()
					registry.Register(&tool.EmitAnswerDocument{})
					registry.Register(&tool.EmitAnswerDocumentPatch{})
					base := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 2, ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: mode}}}, &answerDocumentEvaluator{})
					out, err := base.Execute(ctx, &skill.Config{Name: "argument-owner-integrity", ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
					if err != nil || out == nil || out.AnswerDegraded || !strings.Contains(out.FinalAnswer, "The model's original useful answer remains unchanged.") {
						t.Fatalf("optional envelope conflict lost model body: err=%v out=%+v", err, out)
					}
					if ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || !ctx.Mutable.TraceRootCauseSelectorRejected() {
						t.Fatal("agent normalized a competing selector into accepted/pending state")
					}
				})
			}
		}
	}
}

func (t *b1677CountTool) Name() string        { return "b1677_count" }
func (t *b1677CountTool) Description() string { return "Counts actual invocations." }
func (t *b1677CountTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
}
func (t *b1677CountTool) Execute(_ *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	t.calls++
	return types.ToolResult{ToolName: t.Name(), Success: true}, nil
}

func TestB1677AgentRejectsConsumedEnvelopeBeforeAnyInvocation(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, dynamic := range []bool{false, true} {
			for _, function := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/dynamic=%t/function=%t", mode, dynamic, function), func(t *testing.T) {
					counter := &b1677CountTool{}
					registry := tool.NewRegistry()
					registry.Register(counter)
					base := &BaseAgent{name: types.AgentExplorer, deps: &Dependencies{Tools: registry, ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentExplorer: {Mode: mode}}}}
					raw := json.RawMessage(`{"arguments":{"path":"first.go"},"arguments":{"path":"second.go"}}`)
					if function {
						raw = json.RawMessage(`{"function":{"arguments":{"path":"first.go"}},"function":{"arguments":{"path":"second.go"}}}`)
					}
					call := llm.ToolCall{ID: "b1677", Name: counter.Name(), Params: raw}
					if dynamic {
						call = base.normalizeToolCallParams([]llm.ToolCall{call}, []llm.ToolSchema{{Name: counter.Name(), Parameters: counter.Parameters()}})[0]
					}
					if string(call.Params) != string(raw) {
						t.Errorf("normalization selected an ambiguous payload: %s", call.Params)
					}
					res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore, Mutable: types.NewMutableState("argument integrity")}, call)
					if counter.calls != 0 || res == nil || res.Success {
						t.Fatalf("ambiguous intent reached tool: calls=%d result=%+v", counter.calls, res)
					}
				})
			}
		}
	}
}
