package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc221ContinuationAgent struct {
	name types.AgentName
	run  func(*types.AgentContext) (*agent.StageOutput, error)
}

func (a *hmc221ContinuationAgent) Name() types.AgentName { return a.name }
func (a *hmc221ContinuationAgent) Execute(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
	return a.run(ctx)
}

// Classifier navigation is not source admission. This follows the actual CLI
// classifier adapter into Run's normal preflight and a public path query. The
// structured classifier/analyzer choices are stubs, not a claim of live-model
// routing accuracy; the fixed natural live case must establish that separately.
func TestHMC221NamedInputHintReadPreflightActualQuery(t *testing.T) {
	oldApp, oldRepo := app, flagRepo
	t.Cleanup(func() { app, flagRepo = oldApp, oldRepo })
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	paths := [2]string{}
	for i, name := range []string{"baseline.data", "current.data"} {
		path, err := filepath.Abs("../eval/fixtures/hmosperf_dual_measurements/" + name)
		if err != nil {
			t.Fatal(err)
		}
		paths[i] = path
	}
	request := "帮我并排看一下 " + paths[0] + " 的 1 到 2 秒和 " + paths[1] + " 的 4 到 4.5 秒量测记录：分别有哪些数据序列，原始值和时间区间是什么？"
	adapter := &summarizerStubAdapter{resp: llm.Response{ToolCalls: []llm.ToolCall{{Name: "emit_turn_policy", Params: []byte(`{"route":"repo","needs_repo_access":true,"operation":"investigate","source":"current_message","confidence":0.95,"reason":"read native measurements","requires_diagram":false}`)}}}}
	app.chitchatClassifier, app.userMode = repl.NewChitchatClassifier(adapter), repl.UserModeAuto
	app.dataTaskPlanner = repl.NewDataTaskPlanner(adapter)
	flagRepo = repo
	policy, ok := classifySingleShotRoutePolicy(request)
	if !ok || policy.Route != repl.RouteRepo {
		t.Fatalf("typed read route not preserved: %+v", policy)
	}
	var classifierContext string
	for _, message := range adapter.lastMessages {
		if message.Role == "user" {
			classifierContext = message.Content
		}
	}
	if !strings.Contains(classifierContext, "## current_named_inputs") || !strings.Contains(classifierContext, "schema_inspected") || !strings.Contains(classifierContext, "trace_query") {
		t.Fatal("actual classifier did not receive content navigation")
	}
	ar, sr := agent.NewRegistry(), skill.NewRegistry()
	for _, name := range []string{"analysis-skill", "answer-document-skill"} {
		sr.Register(&skill.Config{Name: name, Goal: name})
	}
	ar.Register(&hmc221ContinuationAgent{name: types.AgentAnalyzer, run: func(ctx *types.AgentContext) (*agent.StageOutput, error) {
		if ctx.TurnRouteHint.Route != "repo" || ctx.RuntimeArtifactPreflight.HasTraceArtifact() || ctx.AttachedTraceMaterial != nil || ctx.AttachedHitrace != "" {
			t.Fatal("navigation candidate was lost or promoted to attached/source authority")
		}
		return &agent.StageOutput{AnalysisIR: &types.AnalysisIR{Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
				ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly}},
			TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{{ID: "finalize", Type: types.NodeFinalize, Objective: "read selected native measurements", OneShot: true}}}}}, nil
	}})
	var actual types.ToolResult
	ar.Register(&hmc221ContinuationAgent{name: types.AgentFinalizer, run: func(ctx *types.AgentContext) (*agent.StageOutput, error) {
		bus := types.ToolBusContext(ctx, types.AgentFinalizer)
		if bus.TraceInputPreparer == nil || len(bus.TraceInputPreparer.PreparedMaterials()) != 0 {
			t.Fatal("preflight must neither lose the preparer nor prepare .data from routing hints")
		}
		args, _ := json.Marshal(map[string]any{"comparison": map[string]any{
			"baseline": map[string]any{"source": "path", "path": paths[0], "view": "measurements", "time_start": 1, "time_end": 2},
			"current":  map[string]any{"source": "path", "path": paths[1], "view": "measurements", "time_start": 4, "time_end": 4.5},
		}})
		var err error
		actual, err = (&tool.TraceQuery{}).Execute(bus, args)
		if err != nil || !actual.Success {
			t.Fatalf("public query following routing/preflight: %v %s", err, actual.Summary)
		}
		return &agent.StageOutput{Error: "acceptance stops after native acquisition, before model answer"}, nil
	}})
	o := orchestrator.New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, agent.NewSubAgentRegistry())
	o.SetTraceRuntimeAnchor(t.TempDir())
	o.SetTurnRouteHint(repl.TurnRouteHintFromPolicy(policy))
	bus, _ := o.Run(request, repo, "main")
	if bus == nil || !actual.Success {
		t.Fatal("read pipeline did not reach public path query")
	}
	report, valid := actual.RuntimeMeasurementPair.Report()
	if !valid || report.Sides[0].Status != "available" || report.Sides[1].Status != "available" {
		t.Fatalf("path query did not independently prepare both inputs: valid=%t statuses=%s,%s", valid, report.Sides[0].Status, report.Sides[1].Status)
	}
	if bus.RuntimeArtifactPreflight.HasTraceArtifact() || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
		t.Fatal("explicit path query replaced sticky attachment or preflight authority")
	}
}
