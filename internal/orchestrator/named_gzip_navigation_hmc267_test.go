package orchestrator

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc267NamedGZIP(t *testing.T) (string, []byte) {
	t.Helper()
	_, archive := hmc264ZIP(t, false)
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	r, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	w.Name = "../../name-is-not-authority.txt"
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "设备采集.data")
	if err := os.WriteFile(path, out.Bytes(), 0400); err != nil {
		t.Fatal(err)
	}
	return path, out.Bytes()
}

func TestHMC267NamedGZIPNavigationKeepsIndependentSourceObligations(t *testing.T) {
	for _, mode := range []types.ExternalObservationCurrentSourceMode{"", types.ExternalObservationCurrentSourceDefault, types.ExternalObservationCurrentSourceAllow} {
		t.Run(string(mode), func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			path, original := hmc267NamedGZIP(t)
			request := "Inspect `" + path + "` and explain the current source."
			query, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 42, "time_start": .9, "time_end": 1.1})
			adapter := &hmc264NamedZIPAdapter{response: llm.Response{ToolCalls: []llm.ToolCall{{ID: "native", Name: "trace_query", Params: query}}}}
			registry := tool.NewRegistry()
			registry.Register(&tool.TraceQuery{})
			explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: adapter, Tools: registry, MaxIterations: 1, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
			ir := dagIR(types.AnswerContract{})
			ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric}
			if mode != "" {
				ir.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{CurrentSourceMode: mode}
			}
			ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "native", Type: types.NodeEvidence, Objective: request, OneShot: true}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
			ir.EvidencePlan.Budget.MaxReactIters = 1
			var selected *tracequery.Result
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
					return &agent.StageOutput{AnalysisIR: ir}, nil
				},
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					if !ctx.TurnRouteHint.RequiresCurrentSourceEvidence() || len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
						t.Error("navigation erased source duty or forced preparation")
					}
					out, err := explorer.Execute(ctx, &skill.Config{Name: "mixed", ToolSuggestions: []string{"trace_query"}})
					if out != nil {
						for _, result := range out.ToolResults {
							if result.ToolName == "trace_query" && result.Success {
								selected = hmc265NativePayload(t, result)
							}
						}
					}
					return out, err
				},
			})
			o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(t.TempDir())
			o.SetTurnRouteHint(types.TurnRouteHint{Source: "mixed", Route: "repo", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation})
			bus, err := o.Run(request, repo, "main")
			if bus == nil || !adapter.visible["trace_query"] || selected == nil || selected.EventCount != 1 || !bus.TurnRouteHint.RequiresCurrentSourceEvidence() {
				t.Fatalf("mixed optional gzip navigation failed: selected=%v visible=%v err=%v", selected != nil, adapter.visible, err)
			}
			if after, err := os.ReadFile(path); err != nil || !bytes.Equal(original, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestHMC267NamedGZIPSourceOnlyDoesNotForceFullPreparation(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path, wire := hmc267NamedGZIP(t)
	wire[len(wire)-8] ^= 0xff
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, wire, 0400); err != nil {
		t.Fatal(err)
	}
	request := "Explain the current converter implementation for fixture `" + path + "`."
	params, _ := json.Marshal(map[string]any{"path": filepath.Join(repo, "main.go")})
	adapter := &hmc264NamedZIPAdapter{response: llm.Response{ToolCalls: []llm.ToolCall{{ID: "source", Name: "read_file", Params: params}}}}
	registry := tool.NewRegistry()
	registry.Register(&tool.TraceQuery{})
	registry.Register(&tool.ReadFile{})
	explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: adapter, Tools: registry, MaxIterations: 1, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	ir := dagIR(types.AnswerContract{})
	ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric}
	ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "source", Type: types.NodeEvidence, Objective: request, OneShot: true}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
	ir.EvidencePlan.Budget.MaxReactIters = 1
	read := false
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			out, err := explorer.Execute(ctx, &skill.Config{Name: "source", ToolSuggestions: []string{"read_file", "trace_query"}})
			if out != nil {
				for _, r := range out.ToolResults {
					if r.ToolName == "read_file" && r.Success {
						read = true
					}
				}
			}
			return out, err
		},
	})
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetTraceRuntimeAnchor(t.TempDir())
	o.SetTurnRouteHint(types.TurnRouteHint{Source: "repo", Route: "repo", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: types.TurnOutcomeSourceExplanation})
	bus, err := o.Run(request, repo, "main")
	if bus == nil || !read || !adapter.visible["trace_query"] || len(bus.TraceInputPreparer.PreparedMaterials()) != 0 || !bus.TurnRouteHint.RequiresCurrentSourceEvidence() {
		t.Fatalf("bad-CRC optional fixture blocked source or gained authority: read=%t visible=%v err=%v", read, adapter.visible, err)
	}
}

// Normal Run reaches real Explorer/BaseAgent, shared preparation, native query,
// and the scheduler's system supplement. The model fixture only scripts calls.
func TestHMC267NamedGZIPNormalRunQueriesAndSupplementsExplicitWindow(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path, original := hmc267NamedGZIP(t)
	request := "Inspect `" + path + "` for thread42 from0.95to1.05seconds."
	query, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 42, "time_start": .9, "time_end": 1.1})
	adapter := &hmc265CompleteAdapter{query: query}
	registry := tool.NewRegistry()
	registry.Register(&tool.TraceQuery{})
	registry.Register(&tool.EmitInvestigationComplete{})
	explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: adapter, Tools: registry, MaxIterations: 2, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	start, end := .95, 1.05
	ir := dagIR(types.AnswerContract{})
	ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentRootCause, Scenario: types.ScenarioGeneric,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "from0.95to1.05seconds"}}
	ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "native", Type: types.NodeEvidence, Objective: request, OneShot: true}, {ID: "answer", Type: types.NodeFinalize, Objective: "Show native facts", OneShot: true}}, Edges: []types.TaskEdge{{From: "native", To: "answer", EdgeType: types.EdgeHardDependency}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
	ir.EvidencePlan.Budget.MaxReactIters = 10
	var selected *tracequery.Result
	var supplements []*tracequery.Result
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Error("optional navigation eagerly prepared input")
			}
			out, err := explorer.Execute(ctx, &skill.Config{Name: "native-explore", ToolSuggestions: []string{"trace_query", "emit_investigation_complete"}})
			if out != nil {
				for _, result := range out.ToolResults {
					if result.ToolName == "trace_query" && result.Success {
						selected = hmc265NativePayload(t, result)
					}
				}
			}
			return out, err
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			for _, result := range ctx.Mutable.SystemTraceSupplementResults() {
				if result.Success {
					supplements = append(supplements, hmc265NativePayload(t, result))
				}
			}
			return &agent.StageOutput{FinalAnswer: "Native events acquired without inferring causal execution state."}, nil
		},
	})
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetTraceRuntimeAnchor(t.TempDir())
	o.SetTurnRouteHint(types.TurnRouteHint{Source: "artifact", Route: "repo", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional, RequiredOutcomes: types.TurnOutcomeMeasurement})
	bus, runErr := o.Run(request, repo, "main")
	if runErr != nil || selected == nil || len(supplements) == 0 {
		t.Fatalf("ordinary gzip did not reach real query/supplement: selected=%t supplements=%d err=%v", selected != nil, len(supplements), runErr)
	}
	for _, result := range supplements {
		if result.SourcePath != selected.SourcePath || result.TimeStart != start || result.TimeEnd != end || result.EventCount != 1 {
			t.Fatalf("supplement lost exact original/window/universe: %+v", result)
		}
	}
	materials := bus.TraceInputPreparer.PreparedMaterials()
	if len(materials) != 1 || !materials[0].MatchesSelection(path, "") || materials[0].QueryPath() != selected.SourcePath || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
		t.Fatal("gzip query acquired incorrect/sticky authority")
	}
	if current, err := os.ReadFile(path); err != nil || !bytes.Equal(current, original) {
		t.Fatal("source changed", err)
	}
}
