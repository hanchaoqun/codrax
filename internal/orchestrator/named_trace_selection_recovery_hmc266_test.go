package orchestrator

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

type hmc266RecoveryAdapter struct {
	hmc264NamedZIPAdapter
	queries   []json.RawMessage
	sameBatch bool
}

func (a *hmc266RecoveryAdapter) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.requests = append(a.requests, append([]llm.Message(nil), messages...))
	i := len(a.requests) - 1
	if a.sameBatch {
		if i == 0 {
			var calls []llm.ToolCall
			for j, query := range a.queries {
				calls = append(calls, llm.ToolCall{ID: fmt.Sprintf("query-%d", j), Name: "trace_query", Params: query})
			}
			return llm.Response{ToolCalls: calls}, nil
		}
		i += len(a.queries) - 1
	}
	if i < len(a.queries) {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("query-%d", i), Name: "trace_query", Params: a.queries[i]}}}, nil
	}
	if i == len(a.queries) {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "complete", Name: "emit_investigation_complete", Params: json.RawMessage(`{"reason":"The intended original member was queried; sampling facts do not establish an execution state.","confidence":"high","result_kind":"resolved"}`)}}}, nil
	}
	return llm.Response{}, fmt.Errorf("bounded public recovery fixture completed")
}

// Normal Run, real Explorer/BaseAgent dispatch, real preparation and query.
// A direct TraceQuery.Execute retry cannot exercise the run-terminal latch.
func TestHMC266NormalRunRecoversMemberSelectionAndSupplements(t *testing.T) {
	for _, test := range []struct {
		name, firstMember string
		sameBatch, nonZIP bool
	}{
		{"missing_selection", "", false, false},
		{"wrong_member", "missing.data", false, false},
		{"same_batch_missing_selection", "", true, false},
		{"same_batch_wrong_member", "missing.data", true, false},
		{"invalid_selector_syntax", "../capture.data", false, false},
		{"non_zip_selector", "captures/device capture.data", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			path, original := hmc264ZIP(t, false)
			correctMember := "captures/device capture.data"
			if test.nonZIP {
				archive, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
				if err != nil {
					t.Fatal(err)
				}
				reader, err := archive.File[0].Open()
				if err != nil {
					t.Fatal(err)
				}
				original, err = io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				if err = reader.Close(); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(t.TempDir(), "capture.data")
				if err = os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
				correctMember = ""
			}
			request := "Inspect `" + path + "` " + correctMember + " for thread42 from0.95to1.05seconds."
			query := func(member string) json.RawMessage {
				params := map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 42, "time_start": .9, "time_end": 1.1}
				if member != "" {
					params["archive_member"] = member
				}
				wire, _ := json.Marshal(params)
				return wire
			}
			adapter := &hmc266RecoveryAdapter{queries: []json.RawMessage{query(test.firstMember), query(correctMember)}, sameBatch: test.sameBatch}
			registry := tool.NewRegistry()
			registry.Register(&tool.TraceQuery{})
			registry.Register(&tool.EmitInvestigationComplete{})
			explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: adapter, Tools: registry, MaxIterations: 3, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
			start, end := .95, 1.05
			ir := dagIR(types.AnswerContract{})
			ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentRootCause, Scenario: types.ScenarioGeneric,
				RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Source: "user_explicit", Confidence: 1}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "from0.95to1.05seconds"}}
			ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "native", Type: types.NodeEvidence, Objective: request, OneShot: true}, {ID: "answer", Type: types.NodeFinalize, Objective: "Show native facts", OneShot: true}}, Edges: []types.TaskEdge{{From: "native", To: "answer", EdgeType: types.EdgeHardDependency}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
			ir.EvidencePlan.Budget.MaxReactIters = 10
			var output *agent.StageOutput
			var selected *tracequery.Result
			var supplements []*tracequery.Result
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
					return &agent.StageOutput{AnalysisIR: ir}, nil
				},
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					var err error
					output, err = explorer.Execute(ctx, &skill.Config{Name: "native-explore", ToolSuggestions: []string{"trace_query", "emit_investigation_complete"}})
					if output != nil {
						for _, result := range output.ToolResults {
							if result.ToolName == "trace_query" && result.Success {
								selected = hmc265NativePayload(t, result)
							}
						}
					}
					return output, err
				},
				types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					for _, result := range ctx.Mutable.SystemTraceSupplementResults() {
						if result.Success {
							supplements = append(supplements, hmc265NativePayload(t, result))
						}
					}
					return &agent.StageOutput{FinalAnswer: "Native facts acquired without inferring an execution state."}, nil
				},
			})
			o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(t.TempDir())
			o.SetTurnRouteHint(types.TurnRouteHint{Source: "artifact", Route: "repo", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional, RequiredOutcomes: types.TurnOutcomeMeasurement})
			bus, runErr := o.Run(request, repo, "main")
			if bus == nil || output == nil {
				t.Fatalf("actual Explorer not reached: %v", runErr)
			}
			var queries []types.ToolResult
			for _, result := range output.ToolResults {
				if result.ToolName == "trace_query" {
					queries = append(queries, result)
				}
			}
			if len(queries) != 2 || queries[0].Success || !queries[1].Success || selected == nil {
				t.Fatalf("correct member retry blocked after recoverable selector: queries=%+v err=%v", queries, runErr)
			}
			if repair := queries[0].Repair; repair == nil || repair.Metadata["stage"] != types.ToolRepairStageTraceSourceSelection || repair.Metadata["status"] != types.ToolRepairStatusActionRecommended || len(queries[0].Observations) != 0 || queries[0].RawRef != "" {
				t.Fatalf("selector failure gained evidence or physical-terminal repair: %+v", queries[0])
			}
			if runErr != nil || len(supplements) == 0 {
				t.Fatalf("recovery lost normal supplement: count=%d err=%v", len(supplements), runErr)
			}
			for _, result := range supplements {
				if result.SourcePath != selected.SourcePath || result.TimeStart != start || result.TimeEnd != end || result.EventCount != 1 {
					t.Fatalf("explicit requested window/member changed: %+v", result)
				}
			}
			materials := bus.TraceInputPreparer.PreparedMaterials()
			if len(materials) != 1 || !materials[0].MatchesSelection(path, correctMember) || materials[0].QueryPath() != selected.SourcePath || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
				t.Fatal("recovery selected another member or acquired sticky authority")
			}
			if current, err := os.ReadFile(path); err != nil || !bytes.Equal(current, original) {
				t.Fatal("source changed", err)
			}
		})
	}
}

// Full CRC failure is not a selector repair. Even an independently valid
// neighbor must remain unexecuted once the real Explorer arms the run latch.
func TestHMC266NormalRunPhysicalFailureStillBlocksHealthyNeighbor(t *testing.T) {
	for _, sameBatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_batch_%t", sameBatch), func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			bad, body := hmc264ZIP(t, false)
			i := bytes.Index(body, []byte{'P', 'K', 1, 2})
			if i < 0 {
				t.Fatal("no ZIP directory")
			}
			body[i+16] ^= 0xff
			if err := os.WriteFile(bad, body, 0600); err != nil {
				t.Fatal(err)
			}
			good, _ := hmc264ZIP(t, false)
			query := func(path string) json.RawMessage {
				raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "archive_member": "captures/device capture.data", "view": "event_search"})
				return raw
			}
			adapter := &hmc266RecoveryAdapter{queries: []json.RawMessage{query(bad), query(good)}, sameBatch: sameBatch}
			registry := tool.NewRegistry()
			registry.Register(&tool.TraceQuery{})
			registry.Register(&tool.EmitInvestigationComplete{})
			explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: adapter, Tools: registry, MaxIterations: 3, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
			request := "Inspect `" + bad + "` and `" + good + "` captures."
			ir := dagIR(types.AnswerContract{})
			ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric}
			ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "native", Type: types.NodeEvidence, Objective: request, OneShot: true}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
			ir.EvidencePlan.Budget.MaxReactIters = 10
			var output *agent.StageOutput
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
					return &agent.StageOutput{AnalysisIR: ir}, nil
				},
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					var err error
					output, err = explorer.Execute(ctx, &skill.Config{Name: "native-explore", ToolSuggestions: []string{"trace_query", "emit_investigation_complete"}})
					return output, err
				},
			})
			o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(t.TempDir())
			o.SetTurnRouteHint(types.TurnRouteHint{Source: "artifact", Route: "repo", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional, RequiredOutcomes: types.TurnOutcomeMeasurement})
			bus, err := o.Run(request, repo, "main")
			if bus == nil || output == nil {
				t.Fatalf("Explorer not reached: %v", err)
			}
			var results []types.ToolResult
			for _, result := range output.ToolResults {
				if result.ToolName == "trace_query" {
					results = append(results, result)
				}
			}
			if len(results) != 2 {
				t.Fatalf("expected two public tool attempts: %+v", results)
			}
			for _, result := range results {
				if result.Success || len(result.Observations) != 0 || result.RawRef != "" {
					t.Fatal("failed input or healthy neighbor acquired evidence", result)
				}
			}
			if results[0].Repair == nil || results[0].Repair.Metadata["stage"] != types.ToolRepairStageTraceInputAdmission || results[0].Repair.Metadata["status"] != types.ToolRepairStatusActionRequired {
				t.Fatal("real CRC downgraded", results[0])
			}
			if results[1].Repair == nil || results[1].Repair.Metadata["policy"] != "trace_input_admission_terminal" || len(bus.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Fatal("healthy neighbor bypassed terminal latch", results[1])
			}
		})
	}
}
