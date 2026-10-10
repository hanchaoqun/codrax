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
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type namedInputHMC223LLM struct {
	params   json.RawMessage
	visible  bool
	calls    int
	finalize bool
}

func (l *namedInputHMC223LLM) Chat(ctx context.Context, _ []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if err := ctx.Err(); err != nil {
		return llm.Response{}, err
	}
	toolName := "trace_query"
	if l.finalize {
		toolName = "emit_answer_document"
	}
	for _, schema := range schemas {
		if schema.Name == toolName {
			l.visible = true
		}
	}
	if !l.visible {
		return llm.Response{}, fmt.Errorf("named inputs did not reach the actual %s tool surface", toolName)
	}
	if l.calls == 2 && !l.finalize {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "complete-native-pair", Name: "emit_investigation_complete", Params: json.RawMessage(`{"reason":"Both native sources were read in their selected windows; the system-owned tables retain the records.","confidence":"high","result_kind":"resolved"}`)}}}, nil
	}
	if l.calls > 1 {
		return llm.Response{}, fmt.Errorf("test acquired one native pair; no further model work requested")
	}
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: toolName + "-public", Name: toolName, Params: l.params}}}, nil
}
func (*namedInputHMC223LLM) ModelID() string               { return "named-input-public-handoff" }
func (*namedInputHMC223LLM) MaxContextTokens() int         { return 128000 }
func (*namedInputHMC223LLM) MaxOutputTokens() int          { return 4096 }
func (*namedInputHMC223LLM) RequestTimeout() time.Duration { return 0 }
func (*namedInputHMC223LLM) RetryMaxAttempts() int         { return 0 }

// The analyzer decision is supplied, but Run, preparation, the Explorer's
// BaseAgent tool catalog, executeTool, and both native readers are real. No
// direct TraceQuery.Execute or manually installed trace carrier is used.
func TestHMC223NamedInputsReachRealExplorerAndDualMeasurement(t *testing.T) {
	repo, inputs := t.TempDir(), t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	paths, originals := [2]string{}, [2][]byte{}
	for i, name := range []string{"baseline.data", "current.data"} {
		body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_measurements/" + name)
		if err != nil {
			t.Fatal(err)
		}
		paths[i], originals[i] = filepath.Join(inputs, name), body
		if err := os.WriteFile(paths[i], body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := fmt.Sprintf("Show the measurement records in `%s` from 1 to 2 seconds and `%s` from 4 to 4.5 seconds.", paths[0], paths[1])
	params, _ := json.Marshal(map[string]any{"comparison": map[string]any{
		"baseline": map[string]any{"source": "path", "path": paths[0], "view": "measurements", "time_start": 1, "time_end": 2},
		"current":  map[string]any{"source": "path", "path": paths[1], "view": "measurements", "time_start": 4, "time_end": 4.5},
	}})
	adapter := &namedInputHMC223LLM{params: params}
	registry := tool.NewRegistry()
	registry.Register(&tool.TraceQuery{})
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitInvestigationComplete{})
	explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: adapter, Tools: registry, MaxIterations: 2, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	finalAdapter := &namedInputHMC223LLM{finalize: true, params: json.RawMessage(`{"blocks":[{"id":"interpretation","kind":"summary","text":"Each capture retains its own original records; identities, units and causal relationships not established by these values remain unknown."}]}`)}
	finalizer := agent.NewFinalizerAgent(&agent.Dependencies{LLM: finalAdapter, Tools: registry, MaxIterations: 1, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	var output *agent.StageOutput
	ir := dagIR(types.AnswerContract{})
	ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionObservedValue, Required: true, Label: "original records", Index: 1}}},
		RuntimeQuestionProfile:    &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
	firstStart, firstEnd, secondStart, secondEnd := 1.0, 2.0, 4.0, 4.5
	ir.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, Confidence: 1,
		TimeWindows: []types.RuntimeArtifactTimeWindow{
			{TimeStart: &firstStart, TimeEnd: &firstEnd, SourceQuote: "from 1 to 2 seconds"},
			{TimeStart: &secondStart, TimeEnd: &secondEnd, SourceQuote: "from 4 to 4.5 seconds"},
		}}
	ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "records", Type: types.NodeEvidence, Objective: "Read the two native measurement sources", OneShot: true}, {ID: "answer", Type: types.NodeFinalize, Objective: "Show the original records", OneShot: true}}, Edges: []types.TaskEdge{{From: "records", To: "answer", EdgeType: types.EdgeHardDependency}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
	// This is the scheduler's whole-pipeline budget, not the Explorer's
	// two-round limit: leave room for its ordinary extract/finalize handoff.
	ir.EvidencePlan.Budget.MaxReactIters = 10
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if ctx.RuntimeArtifactPreflight.HasTraceArtifact() || len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Error("candidate was prepared or promoted before typed analyzer intent")
			}
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if len(ctx.TraceInputPreparer.PreparedMaterials()) != 2 {
				t.Error("real preparation was not complete before the actual Explorer")
			}
			var err error
			output, err = explorer.Execute(ctx, &skill.Config{Name: "explore-skill", ToolSuggestions: []string{"trace_query", "emit_investigation_complete"}})
			return output, err
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return finalizer.Execute(ctx, &skill.Config{Name: "answer-document-skill", ToolSuggestions: []string{"emit_answer_document"}})
		},
	})
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetTraceRuntimeAnchor(t.TempDir())
	bus, runErr := o.Run(request, repo, "main")
	if runErr != nil {
		t.Fatalf("real Run failed: %v", runErr)
	}
	if !adapter.visible || output == nil {
		t.Fatalf("actual Explorer did not receive native tools: visible=%v output=%v run=%v", adapter.visible, output, runErr)
	}
	var result *types.ToolResult
	for i := range output.ToolResults {
		if _, ok := output.ToolResults[i].RuntimeMeasurementPair.Report(); ok {
			result = &output.ToolResults[i]
			break
		}
	}
	if result == nil || !result.Success {
		t.Fatalf("actual executeTool did not publish the pair: output=%+v run=%v", output, runErr)
	}
	report, ok := result.RuntimeMeasurementPair.Report()
	if !ok {
		t.Fatal("native pair has no actual producer receipt")
	}
	for i, want := range []int{13, 8} {
		if report.Sides[i].Status != "available" {
			t.Fatalf("side %d unavailable: %+v", i, report.Sides[i])
		}
		got := 0
		for _, publication := range report.Sides[i].Publications {
			for _, table := range publication.Tables {
				if table.View == types.RuntimeMeasurementMembers {
					got += len(table.Rows)
				}
			}
		}
		if got != want {
			t.Fatalf("side %d records=%d want %d", i, got, want)
		}
		body, err := os.ReadFile(paths[i])
		if err != nil || !bytes.Equal(body, originals[i]) {
			t.Fatal("native input bytes changed", err)
		}
	}
	if bus == nil || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" || len(bus.TraceInputPreparer.PreparedMaterials()) != 2 {
		t.Fatal("named captures became attachments or lost preparation identity")
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if !finalAdapter.visible || doc == nil {
		t.Fatalf("actual Finalizer lost native handoff: visible=%v calls=%d state=%+v result=%q", finalAdapter.visible, adapter.calls, bus.TaskState, bus.Mutable.Result())
	}
	rows := map[int]int{}
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.IsBound() {
			rows[len(block.RuntimeMeasurement.BoundTable.Rows)]++
		}
	}
	if rows[13] != 1 || rows[8] != 1 || rows[2] != 1 || len(rows) != 3 {
		t.Fatalf("actual final native tables lost/duplicated records: %v", rows)
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(doc, "en"))
	for _, value := range []string{"9007199254740993", "9007199254740995", "0 (integer)", "未知（NULL）"} {
		if !strings.Contains(visible, value) {
			t.Errorf("actual final answer lost %q", value)
		}
	}
}
