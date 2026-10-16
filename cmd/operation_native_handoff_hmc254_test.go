package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Classifier/planner responses are scripted; the entrypoint, source preparation
// and native query are real. This is not a claim about natural model accuracy.
func TestHMC254SingleShotOperationHandoffReachesRealNativePreparation(t *testing.T) {
	oldApp, oldRepo := app, flagRepo
	t.Cleanup(func() { app, flagRepo = oldApp, oldRepo })
	paths := [2]string{}
	for i, name := range []string{"baseline.data", "current.data"} {
		p, err := filepath.Abs("../eval/fixtures/hmosperf_dual_measurements/" + name)
		if err != nil {
			t.Fatal(err)
		}
		paths[i] = p
	}
	request := "Compare `" + paths[0] + "` and `" + paths[1] + "` from 1 to 2 seconds and keep the evidence package."
	classifier := &summarizerStubAdapter{resp: llm.Response{ToolCalls: []llm.ToolCall{{Name: "emit_turn_policy", Params: []byte(`{"route":"operation","operation":"investigate","source":"artifact","needs_repo_access":true,"needs_operation_access":true,"current_source_evidence_mode":"optional","write_intent":"analysis_only","confidence":0.95,"risk_level":"low","target_surface":"file_artifact","side_effects":["local_file_write"],"requires_diagram":false,"required_outcomes":["measurement","external_artifact"]}`)}}}}
	raw, _ := json.Marshal(map[string]any{"status": "needs_native_reader", "risk_level": "none", "requires_confirmation": false, "input_paths": paths[:], "steps": []any{}})
	planner := &summarizerStubAdapter{resp: llm.Response{ToolCalls: []llm.ToolCall{{Name: "emit_command_operation_plan", Params: raw}}}}
	ar, sr := agent.NewRegistry(), skill.NewRegistry()
	for _, name := range []string{"analysis-skill", "answer-document-skill"} {
		sr.Register(&skill.Config{Name: name, Goal: name})
	}
	start, end := 1., 2.
	var queried bool
	ar.Register(&hmc221ContinuationAgent{name: types.AgentAnalyzer, run: func(ctx *types.AgentContext) (*agent.StageOutput, error) {
		if ctx.TurnRouteHint.Route != "repo" || ctx.TurnRouteHint.NeedsOperationAccess || ctx.TurnRouteHint.CurrentSourceEvidenceMode != types.TurnRouteCurrentSourceEvidenceOptional || !ctx.TurnRouteHint.RequiredOutcomes.Has(types.TurnOutcomeExternalArtifact) || ctx.AttachedTraceMaterial != nil {
			t.Fatalf("handoff missing or promoted: %+v", ctx.TurnRouteHint)
		}
		return &agent.StageOutput{AnalysisIR: &types.AnalysisIR{Version: types.AnalysisIRVersion, RequestModel: types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "from 1 to 2 seconds", Comparison: &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[0]}}, Current: types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[1]}}}}}, TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{{ID: "finalize", Type: types.NodeFinalize, Objective: "read both inputs", OneShot: true}}}}}, nil
	}})
	ar.Register(&hmc221ContinuationAgent{name: types.AgentFinalizer, run: func(ctx *types.AgentContext) (*agent.StageOutput, error) {
		bus := types.ToolBusContext(ctx, types.AgentFinalizer)
		if bus.TraceInputPreparer == nil || len(bus.TraceInputPreparer.PreparedMaterials()) != 2 {
			t.Fatal("native preparation not reached")
		}
		args, _ := json.Marshal(map[string]any{"comparison": map[string]any{"baseline": map[string]any{"source": "path", "path": paths[0], "view": "measurements"}, "current": map[string]any{"source": "path", "path": paths[1], "view": "measurements"}}})
		out, err := (&tool.TraceQuery{}).Execute(bus, args)
		if err != nil || !out.Success {
			t.Fatalf("native query: %v %s", err, out.Summary)
		}
		if _, ok := out.RuntimeMeasurementPair.Report(); !ok {
			t.Fatal("missing native pair")
		}
		queried = true
		return &agent.StageOutput{FinalAnswer: "Native measurements acquired."}, nil
	}})
	app.orch = orchestrator.New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, agent.NewSubAgentRegistry())
	app.orch.SetTraceRuntimeAnchor(t.TempDir())
	app.renderer = render.New(io.Discard, false)
	app.userMode = repl.UserModeAuto
	app.chitchatClassifier = repl.NewChitchatClassifier(classifier)
	app.operationPlanner = repl.NewCommandOperationPlanner(planner)
	app.operationRouteEnabled = true
	flagRepo = t.TempDir()
	if err := os.WriteFile(filepath.Join(flagRepo, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runSingleShot(nil, request); err != nil {
		t.Fatal(err)
	}
	if !queried || len(planner.lastMessages) == 0 {
		t.Fatal("operation navigation did not reach native query")
	}
}
