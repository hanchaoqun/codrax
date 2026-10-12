package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc228CLIDataPlanner struct {
	paths []string
	calls int
}

func (p *hmc228CLIDataPlanner) PlanDataTask(_ context.Context, _, _ string, _ repl.TurnPolicy, candidates []dataquery.CandidateFile) (dataquery.TaskPlan, error) {
	p.calls++
	return dataquery.TaskPlan{Status: "needs_native_reader", InputPaths: p.paths}, nil
}

// This exercises the real single-shot dispatch, not just the handoff predicate.
// Stub stages isolate routing; preparation and TraceQuery are real producers.
func TestHMC228SingleShotDataHandoffReachesRealNativePreparation(t *testing.T) {
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
	request := "Compare `" + paths[0] + "` and `" + paths[1] + "` from 1 to 2 seconds."
	p := &hmc228CLIDataPlanner{paths: paths[:]}
	ar, sr := agent.NewRegistry(), skill.NewRegistry()
	for _, name := range []string{"analysis-skill", "answer-document-skill"} {
		sr.Register(&skill.Config{Name: name, Goal: name})
	}
	start, end := 1., 2.
	var queried bool
	ar.Register(&hmc221ContinuationAgent{name: types.AgentAnalyzer, run: func(ctx *types.AgentContext) (*agent.StageOutput, error) {
		if ctx.TurnRouteHint.Route != "repo" || ctx.TurnRouteHint.DataTaskKind != "" || ctx.TurnRouteHint.NeedsDataAccess || ctx.AttachedTraceMaterial != nil {
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
			t.Fatal("missing actual native pair")
		}
		queried = true
		return &agent.StageOutput{FinalAnswer: "Native measurements acquired."}, nil
	}})
	app.orch = orchestrator.New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, agent.NewSubAgentRegistry())
	app.orch.SetTraceRuntimeAnchor(t.TempDir())
	app.renderer = render.New(io.Discard, false)
	app.userMode = repl.UserModeData
	app.dataTaskPlanner = p
	flagRepo = t.TempDir()
	if err := os.WriteFile(filepath.Join(flagRepo, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runSingleShot(nil, request); err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 || !queried {
		t.Fatalf("planner=%d queried=%t", p.calls, queried)
	}
}
