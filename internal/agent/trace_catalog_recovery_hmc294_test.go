package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

type catalogRecoveryCapture294 struct {
	traceTeachingCaptureLLM
	t   *testing.T
	ctx *types.AgentContext
}

func (l *catalogRecoveryCapture294) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "conflicting-plan", Name: "trace_catalog", Params: json.RawMessage(`{"action":"discover","root":".","request_plan":{"view":"resource_stack"},"representatives":{"per_cluster":1}}`)}}, StopReason: "tool_use"}, nil
	}
	for _, message := range messages {
		if message.Role != "tool" || message.ToolCallID != "conflicting-plan" {
			continue
		}
		var result struct {
			ReasonCode     string          `json:"reason_code"`
			NavigationOnly bool            `json:"navigation_only"`
			NextCall       json.RawMessage `json:"next_call"`
		}
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil || result.ReasonCode != "trace_catalog_request_plan_conflict" || !result.NavigationOnly {
			l.t.Fatalf("actual next model request lost structured failed navigation: %v %s", err, message.Content)
		}
		if err := toolparam.Validate(result.NextCall, (&toolpkg.TraceCatalog{}).Parameters()); err != nil {
			l.t.Fatalf("actual model cannot reuse candidate JSON: %v %s", err, result.NextCall)
		}
		if len(l.ctx.Mutable.TraceCatalogs()) != 0 || l.ctx.Mutable.TraceQueryRuntimeObservationCount() != 0 {
			l.t.Fatal("failed navigation created catalog or runtime evidence")
		}
		results := l.ctx.Mutable.DispatchToolResults()
		if len(results) != 1 || results[0].Success || results[0].Summary != message.Content {
			l.t.Fatal("failure status or exact message changed before next model turn")
		}
		return llm.Response{}, l.stop
	}
	l.t.Fatal("failed tool response absent from actual next model request")
	return llm.Response{}, l.stop
}

func TestHMC294CatalogRecoveryReachesActualExplorerRequest(t *testing.T) {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	bus := traceCapabilitiesDiscoveryBus(t, false)
	start, end := 10., 10.05
	rm := types.RequestModel{
		RuntimeArtifactWorkProfile:  &types.RuntimeArtifactWorkProfile{Mode: types.RuntimeArtifactWorkRepresentative, PerSpecificationLimit: 2, SourceQuote: "two per group"},
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "thread 101"},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10 to 10.05 seconds"},
	}
	bus.Mutable.SetRequestModel(rm)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	capture := &catalogRecoveryCapture294{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured actual recovery request")}, t: t, ctx: ctx}
	explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 2})
	_, err := explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 2 {
		t.Fatalf("actual explorer loop not reached: %v, calls=%d", err, capture.calls)
	}
}
