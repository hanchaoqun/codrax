package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A separate opt-in preserves both historical planner-only evaluations. Source
// preparation and native execution are operator-controlled public fixtures;
// parent and child workflow decisions, and the registration planner, use real
// model adapters. This is not a full CLI/code-authoring evaluation.
func TestNativeRegistrationLiveNaturalControllerTerminal(t *testing.T) {
	runNativeRegistrationLiveEvaluation(t, os.Getenv("CODRAX_LIVE_REGISTRATION_NATURAL_EVAL_DIR"), true, true)
}

func nativeRegistrationControllerAgent(adapter llm.Adapter) agent.Agent {
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitWriteWorkflowDecision{})
	return agent.NewWriteControllerAgent(&agent.Dependencies{
		Tools: registry, LLM: adapter, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {},
	})
}

func installNativeRegistrationController(ar *agent.Registry, sr *skill.Registry, adapter llm.Adapter) {
	ar.Register(nativeRegistrationControllerAgent(adapter))
	skill.RegisterDefaults(sr)
}

func installNativeRegistrationChildController(t *testing.T, input registrationRestartInput, ar *agent.Registry, sr *skill.Registry) {
	t.Helper()
	if !input.SourceTerminal || !filepath.IsAbs(input.ProvidersPath) || !filepath.IsAbs(input.AuditDir) {
		t.Fatal("natural controller child requires explicit terminal fixture and absolute provider/audit paths")
	}
	logger, err := logging.NewFromFlags(filepath.Join(input.AuditDir, "child-logs"), "debug", false)
	if err != nil {
		t.Fatal(err)
	}
	previous := logging.Default
	logging.SetDefault(logger)
	t.Cleanup(func() { logging.SetDefault(previous); _ = logger.Close() })
	cfg, err := config.LoadProviders(input.ProvidersPath)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := llm.NewFromConfig(config.ResolveProvider(cfg, string(types.AgentWriteController)))
	if err != nil {
		t.Fatal(err)
	}
	installNativeRegistrationController(ar, sr, adapter)
	t.Logf("restored controller uses real model adapter %s with default write-controller-skill", adapter.ModelID())
}

// These deterministic tests exercise the exact real-agent/default-skill/tool
// wiring without loading a provider or invoking a paid model. The test adapter
// is deliberately not used by either opt-in live path or its child.
type nativeRegistrationControllerWiringAdapter struct {
	*controllerRegistrationLLM
	actions []string
	calls   int
}

func (l *nativeRegistrationControllerWiringAdapter) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	var text strings.Builder
	for _, message := range messages {
		text.WriteString(message.Content)
	}
	if !strings.Contains(text.String(), "Choose the next bounded write workflow action from typed artifacts") || !strings.Contains(text.String(), "Typed write task") {
		return llm.Response{}, fmt.Errorf("real controller skill and typed context were not delivered")
	}
	if len(schemas) != 1 || schemas[0].Name != "emit_write_workflow_decision" {
		return llm.Response{}, fmt.Errorf("unexpected controller tool surface: %+v", schemas)
	}
	if l.calls > len(l.actions) {
		return llm.Response{}, fmt.Errorf("unexpected controller call %d", l.calls)
	}
	params, _ := json.Marshal(map[string]string{"action": l.actions[l.calls-1], "reason_code": "deterministic_wiring_check"})
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("wiring-%d", l.calls), Name: "emit_write_workflow_decision", Params: params}}}, nil
}

func TestNativeRegistrationNaturalControllerParentWiring(t *testing.T) {
	f := newControllerRegistrationFixture(t)
	registry := tool.NewRegistry()
	registry.Register(&tool.ReadFile{})
	registry.Register(&tool.EmitChangePlan{})
	plannerAdapter := &controllerRegistrationLLM{t: t, entry: "emit_change_plan"}
	planner := agent.NewPlannerAgent(&agent.Dependencies{Tools: registry, LLM: plannerAdapter, MaxIterations: 12, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	skills := skill.NewRegistry()
	skill.RegisterDefaults(skills)
	planning, err := skills.Get("change-plan-skill")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &nativeRegistrationControllerWiringAdapter{controllerRegistrationLLM: &controllerRegistrationLLM{t: t}, actions: []string{"verify_batch", "finish", "finish", "finish", "finish", "finish"}}
	runNativeRegistrationTerminalPlanning(t, f, planner, planning, nil, nativeRegistrationControllerAgent(adapter))
	if adapter.calls < 2 || plannerAdapter.round != 2 || !types.IsPersistedNativeTestRegistrationPlan(f.o.busCtx.Mutable.ChangePlan()) {
		t.Fatal("real controller agent did not reach the public registration boundary")
	}
	// Existing process-boundary regression still verifies source/proof/run
	// terminals. This deterministic child remains explicitly scripted.
	f.restartExpectingSourceTerminal(t, true)
}

func TestNativeRegistrationNaturalControllerChildWiring(t *testing.T) {
	ar, sr, _ := buildRegistries(nil)
	adapter := &nativeRegistrationControllerWiringAdapter{controllerRegistrationLLM: &controllerRegistrationLLM{t: t}, actions: []string{"block"}}
	installNativeRegistrationController(ar, sr, adapter)
	controller, err := ar.Get(types.AgentWriteController)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := sr.Get("write-controller-skill")
	if err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("verify retained implementation")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "verify retained implementation"}}})
	_, err = controller.Execute(&types.AgentContext{Mode: types.ModeApply, Stage: types.StageWriteController, AgentName: types.AgentWriteController, Mutable: mu}, sk)
	if err != nil || adapter.calls != 1 || !strings.Contains(string(mu.WriteWorkflowDecisionJSON()), `"block"`) {
		t.Fatalf("model block was replaced or controller wiring failed: calls=%d err=%v decision=%s", adapter.calls, err, mu.WriteWorkflowDecisionJSON())
	}
}
