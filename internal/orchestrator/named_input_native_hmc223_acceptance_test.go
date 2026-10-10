package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This independent harness enters the real Run and Explorer tool catalog. The
// analyzer choice is scripted; neither materials nor tool results are minted
// by the test. It stops after acquisition and does not claim answer acceptance.
type hmc223AcceptanceAdapter struct {
	call    *llm.ToolCall
	visible map[string]bool
}

func (a *hmc223AcceptanceAdapter) Chat(_ context.Context, _ []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.visible = map[string]bool{}
	for _, schema := range schemas {
		a.visible[schema.Name] = true
	}
	if a.call == nil {
		return llm.Response{}, errors.New("acceptance stops after observing actual tool surface")
	}
	call := *a.call
	a.call = nil
	return llm.Response{ToolCalls: []llm.ToolCall{call}}, nil
}
func (*hmc223AcceptanceAdapter) ModelID() string               { return "hmc223-independent-acceptance" }
func (*hmc223AcceptanceAdapter) MaxContextTokens() int         { return 128000 }
func (*hmc223AcceptanceAdapter) MaxOutputTokens() int          { return 4096 }
func (*hmc223AcceptanceAdapter) RequestTimeout() time.Duration { return 0 }
func (*hmc223AcceptanceAdapter) RetryMaxAttempts() int         { return 0 }

func hmc223AcceptanceRun(t *testing.T, repo, request string, external bool, adapter *hmc223AcceptanceAdapter, beforeExplore func(*types.AgentContext), cancelAfterAnalyze bool) (*types.BusContext, *agent.StageOutput, error) {
	t.Helper()
	ir := dagIR(types.AnswerContract{})
	ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric}
	if external {
		ir.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly}
	}
	ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "native-boundary", Type: types.NodeEvidence, Objective: request, OneShot: true}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
	ir.EvidencePlan.Budget.MaxReactIters = 1
	registry := tool.NewRegistry()
	registry.Register(&tool.TraceQuery{})
	registry.Register(&tool.TraceCatalog{})
	explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: adapter, Tools: registry, MaxIterations: 1, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	var output *agent.StageOutput
	var o *Orchestrator
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Error("named input was prepared before typed intent")
			}
			if cancelAfterAnalyze {
				o.Cancel("cancel before named-input preparation")
			}
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if beforeExplore != nil {
				beforeExplore(ctx)
			}
			var err error
			output, err = explorer.Execute(ctx, &skill.Config{Name: "explore-skill", ToolSuggestions: []string{"trace_query", "trace_catalog"}})
			return output, err
		},
	})
	o = New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetTraceRuntimeAnchor(t.TempDir())
	bus, err := o.Run(request, repo, "main")
	return bus, output, err
}

func hmc223AcceptanceCopyCapture(t *testing.T, name string) (string, []byte) {
	t.Helper()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_measurements/" + name)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return path, body
}

func TestHMC223NamedInputDomainAndIntentAreIndependent(t *testing.T) {
	for _, name := range []string{"source_converter_native_fixture", "source_converter_bad_binary", "ordinary_business_sqlite"} {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			path, original := hmc223AcceptanceCopyCapture(t, "baseline.data")
			external := false
			if name == "source_converter_bad_binary" {
				original = []byte{0xce, 0x0a, 1, 0, 1, 0, 0, 0}
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if name == "ordinary_business_sqlite" {
				external = true
				path = filepath.Join(t.TempDir(), "orders.data")
				if out, err := exec.Command("sqlite3", path, "CREATE TABLE orders(ts,value,customer_id); INSERT INTO orders VALUES(1,0,7);").CombinedOutput(); err != nil {
					t.Fatalf("create ordinary SQLite: %v %s", err, out)
				}
				var err error
				original, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			request := fmt.Sprintf("Explain the source converter handling the fixture `%s`.", path)
			if external {
				request = fmt.Sprintf("List the records in `%s`.", path)
			}
			adapter := &hmc223AcceptanceAdapter{}
			bus, _, err := hmc223AcceptanceRun(t, repo, request, external, adapter, nil, false)
			if bus == nil || adapter.visible == nil {
				t.Fatalf("ordinary investigation was blocked: %v", err)
			}
			if len(bus.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Fatal("content candidate became admitted runtime input without both domain and intent")
			}
			// Known binary magic can already provide optional navigation before
			// analysis. That old surface is not admission and must not be removed
			// merely because this turn asks about converter source code.
			if external && (adapter.visible["trace_query"] || bus.RuntimeArtifactPreflight.HasTraceArtifact()) {
				t.Fatal("ordinary business SQLite became native trace navigation")
			}
			if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, original) {
				t.Fatal("input changed", err)
			}
		})
	}
}

func TestHMC223NamedInputCancellationBeforePreparation(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path, _ := hmc223AcceptanceCopyCapture(t, "baseline.data")
	adapter := &hmc223AcceptanceAdapter{}
	bus, output, err := hmc223AcceptanceRun(t, repo, "Read the measurement records in `"+path+"`.", true, adapter, nil, true)
	if err == nil || bus == nil || output != nil || adapter.visible != nil || len(bus.TraceInputPreparer.PreparedMaterials()) != 0 {
		t.Fatalf("cancelled input crossed preparation/exploration: output=%v visible=%v err=%v", output, adapter.visible, err)
	}
}

func TestHMC223PreparedNamedInputDoesNotAuthorizeParentCatalog(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path, _ := hmc223AcceptanceCopyCapture(t, "baseline.data")
	params, _ := json.Marshal(map[string]any{"root": filepath.Dir(path)})
	adapter := &hmc223AcceptanceAdapter{call: &llm.ToolCall{ID: "unauthorized-parent", Name: "trace_catalog", Params: params}}
	bus, output, err := hmc223AcceptanceRun(t, repo, "Read the measurement records in `"+path+"`.", true, adapter, func(ctx *types.AgentContext) {
		if len(ctx.TraceInputPreparer.PreparedMaterials()) != 1 {
			t.Error("test did not reach the new prepared-input boundary")
		}
	}, false)
	if bus == nil || output == nil || !adapter.visible["trace_query"] {
		t.Fatalf("prepared path did not reach actual tool catalog: %v", err)
	}
	for _, result := range output.ToolResults {
		if result.ToolName == "trace_catalog" {
			if result.Success || len(bus.Mutable.TraceCatalogs()) != 0 {
				t.Fatal("preparing one named file expanded authority to its parent directory")
			}
			return
		}
	}
	t.Fatal("catalog boundary was never actually exercised")
}

func TestHMC223PreparedNamedInputReplacementCannotReuseOldFacts(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	left, body := hmc223AcceptanceCopyCapture(t, "baseline.data")
	right, _ := hmc223AcceptanceCopyCapture(t, "current.data")
	params, _ := json.Marshal(map[string]any{"comparison": map[string]any{
		"baseline": map[string]any{"source": "path", "path": left, "view": "measurements", "time_start": 1, "time_end": 2},
		"current":  map[string]any{"source": "path", "path": right, "view": "measurements", "time_start": 4, "time_end": 4.5},
	}})
	adapter := &hmc223AcceptanceAdapter{call: &llm.ToolCall{ID: "after-source-replacement", Name: "trace_query", Params: params}}
	request := fmt.Sprintf("Read `%s` from 1 to 2 seconds and `%s` from 4 to 4.5 seconds.", left, right)
	_, output, err := hmc223AcceptanceRun(t, repo, request, true, adapter, func(ctx *types.AgentContext) {
		if len(ctx.TraceInputPreparer.PreparedMaterials()) != 2 {
			t.Error("test must replace the original after both actual preparations")
		}
		replacement := left + ".replacement"
		if err := os.WriteFile(replacement, body, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, left); err != nil {
			t.Fatal(err)
		}
	}, false)
	if output == nil {
		t.Fatalf("no actual query dispatch: %v", err)
	}
	for _, result := range output.ToolResults {
		if report, ok := result.RuntimeMeasurementPair.Report(); ok {
			if report.Sides[0].Status != "failed" || len(report.Sides[0].Publications) != 0 || report.Sides[1].Status != "available" {
				t.Fatalf("old-source facts survived or healthy independent side lost: %+v", report)
			}
			rows := 0
			for _, publication := range report.Sides[1].Publications {
				for _, table := range publication.Tables {
					if table.View == types.RuntimeMeasurementMembers {
						rows += len(table.Rows)
					}
				}
			}
			if rows != 8 {
				t.Fatalf("healthy side retained %d rows, want the complete independent 8", rows)
			}
			return
		}
	}
	t.Fatal("actual comparison did not publish source-specific states")
}
