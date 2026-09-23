package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type documentationHandoffLLM struct {
	traceTeachingCaptureLLM
	params json.RawMessage
}

func (l *documentationHandoffLLM) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	if l.calls == 0 {
		l.calls++
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "documentation", Name: "trace_capabilities", Params: l.params}}, StopReason: "tool_use"}, nil
	}
	return l.traceTeachingCaptureLLM.Chat(ctx, messages, schemas, opts)
}

// Exercise the producer, the actual explorer ParseOutput/TurnA handoff, and
// the downstream agents' first adapter request. A summaries-only unit test
// misses the cross-stage loss which originally changed measurement contracts.
func TestToolDocumentationActualCrossStageHandoff(t *testing.T) {
	for _, params := range []string{`{}`, `{"view":"window_stats","detail":true}`, `{"detail":true}`} {
		t.Run(params, func(t *testing.T) {
			registry := toolpkg.NewRegistry()
			toolpkg.RegisterDefaults(registry)
			bus := traceCapabilitiesDiscoveryBus(t, false)
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			capture := &documentationHandoffLLM{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("catalog collected")}, params: json.RawMessage(params)}
			out, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 2}).Execute(ctx, traceTeachingSkill(t, "explore-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 2 || out == nil {
				t.Fatalf("actual explorer handoff failed: calls=%d out=%v err=%v", capture.calls, out != nil, err)
			}
			ta := bus.Mutable.TurnAArtifacts()
			if ta == nil || len(ta.ToolResults) != 1 || !ta.ToolResults[0].Success {
				t.Fatalf("catalog result missing from actual TurnA: %+v", ta)
			}
			original := ta.ToolResults[0].Summary
			if !json.Valid([]byte(original)) || len(original) < 1000 {
				t.Fatal("producer did not return a complete catalog")
			}
			if len(out.NewFacts) != 0 || len(types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: ta.ToolResults}).Records) != 0 {
				t.Fatal("documentation became repository facts or runtime observations")
			}
			// The immutable typed document, not mutable summary prose, must
			// supply the downstream contract.
			ta.ToolResults[0].Summary = "model-style summary: incomplete and not authoritative"
			bus.Mutable.SetTurnAArtifacts(*ta)
			assertDocumentationDownstreamRequests(t, bus, registry, original)
		})
	}
}

func TestToolDocumentationActualDirectBusHandoff(t *testing.T) {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	bus := traceCapabilitiesDiscoveryBus(t, false)
	result, err := (&toolpkg.TraceCapabilities{}).Execute(bus, json.RawMessage(`{"view":"window_stats","detail":true}`))
	if err != nil || !result.Success {
		t.Fatalf("producer failed: %+v %v", result, err)
	}
	bus.ToolResults = []types.ToolResult{result}
	assertDocumentationDownstreamRequests(t, bus, registry, result.Summary)
}

func assertDocumentationDownstreamRequests(t *testing.T, bus *types.BusContext, registry *toolpkg.Registry, original string) {
	t.Helper()
	for _, stage := range []types.PipelineStage{types.StageExtract, types.StageFinalize} {
		name, skillName := types.AgentExtractor, "extract-skill"
		if stage == types.StageFinalize {
			name, skillName = types.AgentFinalizer, "answer-document-skill"
		}
		ctx := ctxbuilder.BuildAgentContext(bus, name, stage)
		capture := &traceTeachingCaptureLLM{stop: errors.New("captured documentation handoff")}
		deps := &Dependencies{LLM: capture, Tools: registry, MaxIterations: 1}
		agent := NewExtractorAgent(deps)
		if stage == types.StageFinalize {
			agent = NewFinalizerAgent(deps)
		}
		_, err := agent.Execute(ctx, traceTeachingSkill(t, skillName))
		if !errors.Is(err, capture.stop) || capture.calls != 1 {
			t.Fatalf("%s initial request not captured: calls=%d err=%v", stage, capture.calls, err)
		}
		var prompt strings.Builder
		for _, message := range capture.messages {
			prompt.WriteString(message.Content)
		}
		if count := strings.Count(prompt.String(), original); count != 1 {
			t.Errorf("%s received complete producer contract %d times, want once", stage, count)
		}
		for _, boundary := range []string{"not repository files", "not runtime observations", "citations[]"} {
			if !strings.Contains(prompt.String(), boundary) {
				t.Errorf("%s lacks documentation authority/JSON guidance %q", stage, boundary)
			}
		}
	}
}
