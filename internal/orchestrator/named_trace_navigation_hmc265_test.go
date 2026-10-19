package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reproduce the real classifier/analyzer disagreement without overriding the
// independent source outcome. Run, Explorer schema/dispatch, preparation and
// query are real; no attachment, runtime profile or prepared receipt is seeded.
func TestHMC265NamedNavigationPreservesSourceObligation(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy *types.ExternalObservationPolicy
	}{
		{"omitted", nil},
		{"default", &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceDefault}},
		{"allow", &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceAllow}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, anchor := t.TempDir(), t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			path, original := hmc264ZIP(t, false)
			request := "请看 `" + path + "` 里的 captures/device capture.data，在0.9到1.1秒内线程42的采样热点和调用栈。"
			classifierModel := &hmc264NamedZIPAdapter{response: llm.Response{ToolCalls: []llm.ToolCall{{ID: "classify", Name: "emit_turn_policy", Params: json.RawMessage(`{"route":"repo","needs_repo_access":true,"current_source_evidence_mode":"optional","operation":"investigate","source":"artifact","confidence":0.95,"reason":"runtime call stacks","requires_diagram":false,"required_outcomes":["measurement","source_explanation"]}`)}}}}
			classifier := repl.NewChitchatClassifier(classifierModel).(repl.SingleShotTurnPolicyClassifier)
			policy, err := classifier.ClassifyPolicySingleShot(repl.WithNamedInputRoutingContext(t.Context(), request, repo, anchor), request, "", false)
			if err != nil {
				t.Fatal(err)
			}
			policy = repl.ApplyTurnPolicyGuards(policy, false, false)
			hint := repl.TurnRouteHintFromPolicy(policy)
			if hint.Source != "mixed" || !hint.RequiresCurrentSourceEvidence() {
				t.Fatalf("source outcome was lost: %+v", hint)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "archive_member": "captures/device capture.data", "view": "event_search", "pid": 42, "time_start": .9, "time_end": 1.1})
			model := &hmc264NamedZIPAdapter{response: llm.Response{ToolCalls: []llm.ToolCall{{ID: "native-query", Name: "trace_query", Params: params}}}}
			registry := tool.NewRegistry()
			registry.Register(&tool.TraceQuery{})
			explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: model, Tools: registry, MaxIterations: 1, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
			ir := dagIR(types.AnswerContract{})
			ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, ExternalObservationPolicy: test.policy}
			ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "native", Type: types.NodeEvidence, Objective: request, OneShot: true}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
			ir.EvidencePlan.Budget.MaxReactIters = 1
			var output *agent.StageOutput
			var payload *tracequery.Result
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					if ctx.RuntimeArtifactPreflight.HasTraceArtifact() || len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
						t.Error("pre-analysis candidate gained admission")
					}
					return &agent.StageOutput{AnalysisIR: ir}, nil
				},
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					if !ctx.TurnRouteHint.RequiresCurrentSourceEvidence() {
						t.Error("navigation waived current-source obligation")
					}
					if len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 || ctx.AttachedHitrace != "" {
						t.Error("optional navigation eagerly prepared or attached source")
					}
					var err error
					output, err = explorer.Execute(ctx, &skill.Config{Name: "explore-skill", ToolSuggestions: []string{"trace_query"}})
					if output != nil {
						for _, result := range output.ToolResults {
							if result.ToolName == "trace_query" && result.Success {
								payload = hmc265NativePayload(t, result)
							}
						}
					}
					return output, err
				},
			})
			o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(anchor)
			o.SetTurnRouteHint(hint)
			bus, runErr := o.Run(request, repo, "main")
			if bus == nil || output == nil || len(model.requests) == 0 {
				t.Fatalf("did not reach actual Explorer: %v", runErr)
			}
			if !model.visible["trace_query"] || payload == nil {
				t.Fatalf("source obligation hid normal native query: visible=%v results=%+v run=%v", model.visible, output.ToolResults, runErr)
			}
			prompt := hmc264Messages(model.requests[0])
			for _, want := range []string{"Named input reader navigation", `"candidate_count":2`, `"other/独立原件"`, "Existing independent current-source obligations remain unchanged"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("missing navigation %q", want)
				}
			}
			if !bus.TurnRouteHint.RequiresCurrentSourceEvidence() || bus.AnalysisIR.RequestModel.ExternalObservationPolicy != test.policy {
				t.Error("reader availability changed accepted source policy")
			}
			if payload.EventCount != 1 || len(payload.Events) != 1 || payload.Events[0].Type != tracequery.EventPerfSample || payload.TimeStart != .9 || payload.TimeEnd != 1.1 {
				t.Fatalf("native member/window changed: %+v", payload)
			}
			materials := bus.TraceInputPreparer.PreparedMaterials()
			if len(materials) != 1 || !materials[0].MatchesSelection(path, "captures/device capture.data") || materials[0].QueryPath() != payload.SourcePath || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
				t.Fatal("actual read lost member identity or replaced attachment")
			}
			if current, err := os.ReadFile(path); err != nil || !bytes.Equal(current, original) {
				t.Fatal("original changed", err)
			}
		})
	}
}

func hmc265NativePayload(t *testing.T, result types.ToolResult) *tracequery.Result {
	t.Helper()
	ref := result.RawRef
	for _, observation := range result.Observations {
		if observation.SourceRef.PayloadRef != "" {
			ref = observation.SourceRef.PayloadRef
			break
		}
	}
	body, err := os.ReadFile(ref)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return &payload
}

func TestHMC265OptionalNavigationNeverGrantsAdmission(t *testing.T) {
	for _, kind := range []string{"unselected", "missing_member", "bad_crc", "changed_before_query", "unsafe", "unknown", "suffix_unsafe", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			path, original := hmc264ZIP(t, kind == "unsafe" || kind == "suffix_unsafe")
			if kind == "unknown" {
				original = []byte("ordinary source fixture, not a runtime input")
			}
			if kind == "bad_crc" {
				// Preserve the safe namespace and recognized prefix while making
				// full CRC verification fail on the selected first member.
				i := bytes.Index(original, []byte{'P', 'K', 1, 2})
				if i < 0 {
					t.Fatal("ZIP central directory missing")
				}
				original[i+16] ^= 0xff
			}
			if kind == "suffix_unsafe" {
				path = filepath.Join(filepath.Dir(path), "fixture.htrace")
			}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			params := map[string]any{"source": "path", "path": path, "archive_member": "captures/device capture.data", "view": "event_search"}
			if kind == "unselected" {
				delete(params, "archive_member")
			}
			if kind == "missing_member" {
				params["archive_member"] = "absent.data"
			}
			wire, _ := json.Marshal(params)
			adapter := &hmc223AcceptanceAdapter{call: &llm.ToolCall{ID: "normal-native-query", Name: "trace_query", Params: wire}}
			bus, output, runErr := hmc223AcceptanceRun(t, repo, "Inspect `"+path+"` and explain the current converter.", false, adapter, func(ctx *types.AgentContext) {
				if len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
					t.Error("optional navigation eagerly prepared input")
				}
				if kind == "changed_before_query" {
					if err := os.WriteFile(path, []byte("replaced with unrelated bytes"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}, kind == "cancelled")
			if kind == "cancelled" {
				if runErr == nil || output != nil || adapter.visible != nil || bus == nil || bus.RuntimeArtifactPreflight.HasTraceArtifact() {
					t.Fatalf("cancellation published navigation: %v", runErr)
				}
				return
			}
			if bus == nil || output == nil || adapter.visible == nil {
				t.Fatalf("optional fixture blocked ordinary exploration: %v", runErr)
			}
			if (kind == "unknown" || kind == "unsafe") && (adapter.visible["trace_query"] || bus.RuntimeArtifactPreflight.HasTraceArtifact()) {
				t.Fatal("unknown/unsafe candidate manufactured native navigation")
			}
			if kind != "unknown" && kind != "unsafe" && !adapter.visible["trace_query"] {
				t.Fatal("test never exercised the ordinary admission boundary")
			}
			attempted := false
			for _, result := range output.ToolResults {
				if result.ToolName == "trace_query" {
					attempted = true
					if result.Success || result.TraceQuerySourceRead.Path() != "" {
						t.Fatalf("navigation bypassed actual admission: %+v", result)
					}
				}
			}
			if !attempted || len(bus.TraceInputPreparer.PreparedMaterials()) != 0 || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
				t.Fatal("failed query acquired prepared/attachment authority")
			}
			if _, status := bus.Mutable.SuccessfulPreparedArchiveSelection(context.Background()); status != types.TracePreparedSelectionAbsent {
				t.Fatal("failed navigation acquired a supplement selection")
			}
			if kind != "changed_before_query" {
				if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, original) {
					t.Fatal("source mutated", err)
				}
			}
		})
	}
}

// A native-looking broken fixture can offer optional reader navigation without
// forcing runtime_probe_first or preventing the requested source-code read.
func TestHMC265SourceOnlyFixtureKeepsSourceReader(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path := filepath.Join(t.TempDir(), "broken.data")
	original := []byte("PERFILE2this is not a complete perf file")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	request := "Explain the current converter implementation for fixture `" + path + "`."
	params, _ := json.Marshal(map[string]any{"path": filepath.Join(repo, "main.go")})
	adapter := &hmc264NamedZIPAdapter{response: llm.Response{ToolCalls: []llm.ToolCall{{ID: "read-source", Name: "read_file", Params: params}}}}
	registry := tool.NewRegistry()
	registry.Register(&tool.TraceQuery{})
	registry.Register(&tool.ReadFile{})
	explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: adapter, Tools: registry, MaxIterations: 1, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	ir := dagIR(types.AnswerContract{})
	ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceAllow}}
	ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "source", Type: types.NodeEvidence, Objective: request, OneShot: true}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
	ir.EvidencePlan.Budget.MaxReactIters = 1
	var output *agent.StageOutput
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Error("source fixture was eagerly prepared")
			}
			var err error
			output, err = explorer.Execute(ctx, &skill.Config{Name: "source-explore", ToolSuggestions: []string{"read_file", "trace_query"}})
			return output, err
		},
	})
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetTraceRuntimeAnchor(t.TempDir())
	o.SetTurnRouteHint(types.TurnRouteHint{Source: "repo", Route: "repo", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: types.TurnOutcomeSourceExplanation})
	bus, runErr := o.Run(request, repo, "main")
	if bus == nil || output == nil || !adapter.visible["read_file"] || !adapter.visible["trace_query"] {
		t.Fatalf("source/native optional tools not both available: %v %v", adapter.visible, runErr)
	}
	read := false
	for _, result := range output.ToolResults {
		if result.ToolName == "read_file" && result.Success {
			read = true
		}
	}
	if !read || len(bus.TraceInputPreparer.PreparedMaterials()) != 0 || !bus.TurnRouteHint.RequiresCurrentSourceEvidence() {
		t.Fatalf("navigation became runtime hard gate or lost source duty: %+v", output.ToolResults)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, original) {
		t.Fatal("fixture modified", err)
	}
}

type hmc265CompleteAdapter struct {
	hmc264NamedZIPAdapter
	query json.RawMessage
}

func (a *hmc265CompleteAdapter) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.requests = append(a.requests, append([]llm.Message(nil), messages...))
	if len(a.requests) == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "native-query", Name: "trace_query", Params: a.query}}}, nil
	}
	if len(a.requests) == 2 {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "complete", Name: "emit_investigation_complete", Params: json.RawMessage(`{"reason":"The selected original was queried; the original sample facts retain their units and missing scheduler boundary.","confidence":"high","result_kind":"resolved"}`)}}}, nil
	}
	return llm.Response{}, fmt.Errorf("bounded deterministic Run stops after native query and completion")
}

// Exercise the actual accepted-completion scheduler hook, not a direct call to
// the supplement helper. The system must query the successful exact member in
// the explicit requested window without changing the model's tool transcript.
func TestHMC265NormalRunRetainsSelectedMemberSupplement(t *testing.T) {
	logger, err := logging.NewFromFlags(t.TempDir(), "info", true)
	if err != nil {
		t.Fatal(err)
	}
	previous := logging.Default
	logging.SetDefault(logger)
	t.Cleanup(func() { logging.SetDefault(previous); _ = logger.Close() })
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path, original := hmc264ZIP(t, false)
	request := "Inspect `" + path + "` member captures/device capture.data for thread42 from0.95to1.05seconds."
	query, _ := json.Marshal(map[string]any{"source": "path", "path": path, "archive_member": "captures/device capture.data", "view": "event_search", "pid": 42, "time_start": .9, "time_end": 1.1})
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
	var output *agent.StageOutput
	var selected *tracequery.Result
	var supplements []*tracequery.Result
	var supplementMeta *types.SystemTraceSupplementMeta
	finalized := false
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Error("optional named input was prepared before a query")
			}
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
			finalized = true
			supplementMeta = ctx.Mutable.SystemTraceSupplementMeta()
			_, status := ctx.Mutable.SuccessfulPreparedArchiveSelection(ctx.Ctx)
			t.Logf("selection status=%v preflight=%+v", status, ctx.RuntimeArtifactPreflight)
			for _, result := range ctx.Mutable.SystemTraceSupplementResults() {
				if result.Success {
					supplements = append(supplements, hmc265NativePayload(t, result))
				}
			}
			return &agent.StageOutput{FinalAnswer: "Native facts acquired; no causal state inferred from sampling."}, nil
		},
	})
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetTraceRuntimeAnchor(t.TempDir())
	o.SetTurnRouteHint(types.TurnRouteHint{Source: "artifact", Route: "repo", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional, RequiredOutcomes: types.TurnOutcomeMeasurement})
	bus, runErr := o.Run(request, repo, "main")
	if runErr != nil || !finalized || selected == nil || len(supplements) == 0 {
		t.Fatalf("normal completion did not execute supplement: finalized=%t selected=%v supplements=%d meta=%+v calls=%d err=%v", finalized, selected != nil, len(supplements), supplementMeta, len(adapter.requests), runErr)
	}
	for _, result := range supplements {
		if result.SourcePath != selected.SourcePath || result.TimeStart != start || result.TimeEnd != end || result.EventCount != 1 {
			t.Fatalf("supplement source/window/universe mismatch: %+v", result)
		}
	}
	modelQueries := 0
	for _, result := range bus.ToolResults {
		if result.ToolName == "trace_query" {
			modelQueries++
		}
	}
	if modelQueries != 1 || len(bus.TraceInputPreparer.PreparedMaterials()) != 1 {
		t.Fatalf("system supplement overwrote transcript or selected extra members: queries=%d materials=%d", modelQueries, len(bus.TraceInputPreparer.PreparedMaterials()))
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, original) {
		t.Fatal("original changed", err)
	}
}
