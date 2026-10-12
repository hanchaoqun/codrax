package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc224PartialPairFinalizerLLM struct{ namedInputHMC223LLM }

func (l *hmc224PartialPairFinalizerLLM) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	if l.calls == 1 {
		l.calls++
		if err := ctx.Err(); err != nil {
			return llm.Response{}, err
		}
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "complete-independent-native-inputs", Name: "emit_investigation_complete", Params: json.RawMessage(`{"reason":"One independently requested input failed preparation. The other input's native records are retained in its own requested window; the failed input has no values, not zero values.","confidence":"high","result_kind":"resolved"}`)}}}, nil
	}
	return l.namedInputHMC223LLM.Chat(ctx, messages, schemas, opts)
}

// Unlike the acquisition-only boundary test, this exercises the actual
// Explorer and Finalizer all the way through Run and rendered native tables.
// No result, source receipt, table or preparation capability is test-minted.
func TestHMC224PartialNamedComparisonReachesRealFinalizer(t *testing.T) {
	for _, failedIndex := range []int{0, 1} {
		t.Run(fmt.Sprintf("failed_side_%d", failedIndex), func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			good, original := hmc223AcceptanceCopyCapture(t, "current.data")
			bad := filepath.Join(t.TempDir(), "empty.htrace")
			if err := os.WriteFile(bad, nil, 0600); err != nil {
				t.Fatal(err)
			}
			paths := [2]string{bad, good}
			if failedIndex == 1 {
				paths = [2]string{good, bad}
			}
			scope := hmc224ComparisonScope(paths)
			if failedIndex == 1 {
				scope.Comparison.Baseline.RuntimeArtifactTimeWindow, scope.Comparison.Current.RuntimeArtifactTimeWindow = scope.Comparison.Current.RuntimeArtifactTimeWindow, scope.Comparison.Baseline.RuntimeArtifactTimeWindow
				scope.Comparison.Baseline.SourceQuote = fmt.Sprintf("`%s` from 4 to 4.5 seconds", good)
				scope.Comparison.Current.SourceQuote = fmt.Sprintf("`%s` from 1 to 2 seconds", bad)
				scope.TimeWindows = []types.RuntimeArtifactTimeWindow{scope.Comparison.Baseline.RuntimeArtifactTimeWindow, scope.Comparison.Current.RuntimeArtifactTimeWindow}
			}
			scope.Confidence = 1
			request := fmt.Sprintf("Show the original measurement records in %s and %s.", scope.Comparison.Baseline.SourceQuote, scope.Comparison.Current.SourceQuote)
			pair := map[string]any{}
			for i, side := range scope.Comparison.Sides() {
				pair[[]string{"baseline", "current"}[i]] = map[string]any{"source": "path", "path": side.Path, "view": "measurements", "time_start": *side.TimeStart, "time_end": *side.TimeEnd}
			}
			params, _ := json.Marshal(map[string]any{"comparison": pair})
			adapter := &hmc224PartialPairFinalizerLLM{namedInputHMC223LLM{params: params}}
			finalAdapter := &namedInputHMC223LLM{finalize: true, params: json.RawMessage(`{"blocks":[{"id":"interpretation","kind":"summary","text":"One requested input is unavailable. The healthy input's original records remain independently available; no zero values or causal explanation are inferred for the failed input."}]}`)}
			registry := tool.NewRegistry()
			registry.Register(&tool.TraceQuery{})
			registry.Register(&tool.EmitInvestigationComplete{})
			registry.Register(&tool.EmitAnswerDocument{})
			deps := func(model llm.Adapter, iterations int) *agent.Dependencies {
				return &agent.Dependencies{LLM: model, Tools: registry, MaxIterations: iterations, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}}
			}
			explorer := agent.NewExplorerAgent(deps(adapter, 2))
			finalizer := agent.NewFinalizerAgent(deps(finalAdapter, 1))
			ir := dagIR(types.AnswerContract{})
			ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
				ExternalObservationPolicy:   &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly},
				RuntimeArtifactScopeProfile: scope,
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
				RequestedAnswerDimensions:   &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionObservedValue, Required: true, Label: "original records", Index: 1}}}}
			ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "records", Type: types.NodeEvidence, Objective: request, OneShot: true}, {ID: "answer", Type: types.NodeFinalize, Objective: "Show original records and unavailable input separately", OneShot: true}}, Edges: []types.TaskEdge{{From: "records", To: "answer", EdgeType: types.EdgeHardDependency}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
			ir.EvidencePlan.Budget.MaxReactIters = 10
			var output *agent.StageOutput
			var finalizerCalls int
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					if len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
						t.Error("input prepared before typed analyzer decision")
					}
					return &agent.StageOutput{AnalysisIR: ir}, nil
				},
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					materials := ctx.TraceInputPreparer.PreparedMaterials()
					if len(materials) != 1 {
						t.Error("healthy input was not independently prepared")
					} else {
						// macOS /var can itself be a symlink. Keep this difference
						// visible so lexical-only origin joins cannot hide in tests.
						t.Logf("healthy requested source=%q prepared original=%q", good, materials[0].SourcePath())
					}
					var err error
					output, err = explorer.Execute(ctx, &skill.Config{Name: "explore-skill", ToolSuggestions: []string{"trace_query", "emit_investigation_complete"}})
					return output, err
				},
				types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					finalizerCalls++
					return finalizer.Execute(ctx, &skill.Config{Name: "answer-document-skill", ToolSuggestions: []string{"emit_answer_document"}})
				},
			})
			o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(t.TempDir())
			bus, err := o.Run(request, repo, "main")
			if err != nil {
				t.Fatalf("complete real Run failed: %v; explorer=%+v finalizer_calls=%d", err, output, finalizerCalls)
			}
			if bus == nil || output == nil || !adapter.visible || !finalAdapter.visible || finalizerCalls != 1 {
				if bus != nil {
					t.Logf("terminal state=%+v result=%q explorer=%+v", bus.TaskState, bus.Mutable.Result(), output)
				}
				t.Fatalf("native handoff did not reach actual Finalizer: explore_visible=%v finalize_visible=%v calls=%d", adapter.visible, finalAdapter.visible, finalizerCalls)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			if doc == nil {
				t.Fatal("actual Finalizer did not emit the answer document")
			}
			rows := map[int]int{}
			var states [][]string
			for _, block := range doc.Blocks {
				if block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() {
					continue
				}
				table := block.RuntimeMeasurement.BoundTable
				rows[len(table.Rows)]++
				if table.View == types.RuntimeMeasurementSummary {
					states = table.Rows
				}
			}
			if rows[8] != 1 || rows[2] != 1 || len(rows) != 2 {
				t.Fatalf("healthy eight records and two-side status did not survive finalization: %v", rows)
			}
			if len(states) != 2 || states[failedIndex][1] != "failed" || states[1-failedIndex][1] != "available" {
				t.Fatalf("final native status lost independent side identity: %+v", states)
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "en"))
			for _, value := range []string{"9007199254740995", "0 (integer)", "\"0\" (text)", "未知（NULL）"} {
				if !strings.Contains(visible, value) {
					t.Errorf("final answer lost native value %q", value)
				}
			}
			if after, err := os.ReadFile(good); err != nil || !bytes.Equal(after, original) {
				t.Fatal("healthy input bytes changed", err)
			}
			if after, err := os.ReadFile(bad); err != nil || len(after) != 0 {
				t.Fatal("failed input changed", err)
			}
		})
	}
}
