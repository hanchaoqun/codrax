package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This adapter chooses actions, not results or authority. Every reference below
// is read from an actual public tool response in the current Explorer run.
type hmc270RepresentativeAdapter struct {
	hmc264NamedZIPAdapter
	step        int
	catalogID   string
	selectionID string
	queryCalls  map[string]json.RawMessage
	status      json.RawMessage
}

func hmc270ToolPage(messages []llm.Message, callID string) (map[string]json.RawMessage, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" && messages[i].ToolCallID == callID {
			var page map[string]json.RawMessage
			if err := json.Unmarshal([]byte(messages[i].Content), &page); err != nil {
				return nil, fmt.Errorf("actual %s response is not navigation JSON: %w; %s", callID, err, messages[i].Content)
			}
			return page, nil
		}
	}
	return nil, fmt.Errorf("actual tool response %q not delivered", callID)
}

func (a *hmc270RepresentativeAdapter) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	if err := ctx.Err(); err != nil {
		return llm.Response{}, err
	}
	available := map[string]bool{}
	for _, schema := range schemas {
		available[schema.Name] = true
	}
	call := func(id, name string, params json.RawMessage) (llm.Response, error) {
		if !available[name] {
			return llm.Response{}, fmt.Errorf("step %d actual tool surface omits %s", a.step, name)
		}
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: id, Name: name, Params: params}}}, nil
	}
	a.step++
	switch a.step {
	case 1:
		return call("discover", "trace_catalog", json.RawMessage(`{"root":"captures","queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":202,"time_start":10,"time_end":10.05}]}`))
	case 2:
		page, err := hmc270ToolPage(messages, "discover")
		if err != nil {
			return llm.Response{}, err
		}
		var next map[string]any
		if err := json.Unmarshal(page["representatives_call"], &next); err != nil {
			return llm.Response{}, fmt.Errorf("current public plan omitted representative navigation: %w", err)
		}
		next["representatives"] = map[string]any{"per_cluster": 2, "max_clusters": 2}
		params, _ := json.Marshal(next)
		return call("select", "trace_catalog", params)
	case 3:
		page, err := hmc270ToolPage(messages, "select")
		if err != nil {
			return llm.Response{}, err
		}
		if err := json.Unmarshal(page["query_calls"], &a.queryCalls); err != nil || len(a.queryCalls) != 4 {
			return llm.Response{}, fmt.Errorf("selection must offer four exact current calls: %d %v", len(a.queryCalls), err)
		}
		_ = json.Unmarshal(page["catalog_id"], &a.catalogID)
		_ = json.Unmarshal(page["selection_id"], &a.selectionID)
		if a.catalogID == "" || a.selectionID == "" || !available["trace_query"] {
			return llm.Response{}, fmt.Errorf("current selection did not expose ordinary queries: catalog=%q selection=%q visible=%t", a.catalogID, a.selectionID, available["trace_query"])
		}
		ids := make([]string, 0, len(a.queryCalls))
		for id := range a.queryCalls {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		var calls []llm.ToolCall
		for _, id := range ids {
			calls = append(calls, llm.ToolCall{ID: "query-" + id, Name: "trace_query", Params: a.queryCalls[id]})
		}
		return llm.Response{ToolCalls: calls}, nil
	case 4:
		params, _ := json.Marshal(map[string]any{"action": "representative_status", "catalog_id": a.catalogID})
		return call("status", "trace_catalog", params)
	case 5:
		page, err := hmc270ToolPage(messages, "status")
		if err != nil {
			return llm.Response{}, err
		}
		var selectionID string
		_ = json.Unmarshal(page["selection_id"], &selectionID)
		if selectionID != a.selectionID {
			return llm.Response{}, fmt.Errorf("status reselected current representatives")
		}
		a.status, _ = json.Marshal(page)
		return call("complete", "emit_investigation_complete", json.RawMessage(`{"reason":"The selected queries were executed and their source-local results and current selection coverage are available; unselected captures remain unassessed.","confidence":"high","result_kind":"resolved"}`))
	default:
		return llm.Response{}, fmt.Errorf("unexpected representative script step %d", a.step)
	}
}

type hmc270RepresentativeFinalizer struct {
	namedInputHMC223LLM
	messages []llm.Message
}

func (a *hmc270RepresentativeFinalizer) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, options llm.ChatOptions) (llm.Response, error) {
	a.messages = append(a.messages, messages...)
	return a.namedInputHMC223LLM.Chat(ctx, messages, schemas, options)
}

// Supplied analyzer/action decisions are scripted; Run, BaseAgent schemas and
// dispatch, SQLite readers, preparation, receipts and finalizer context are real.
// No catalog, representative snapshot, native result or permission is installed
// by the fixture. This is not a natural-model acceptance test.
func TestHMC270NormalRunRepresentativeSelectionNativeHandoff(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	originals := map[string][]byte{}
	for i, session := range []string{"session-a", "session-b", "session-c", "session-d"} {
		fixture := "../../eval/fixtures/hmosperf_catalog_objects/captures/capture.data"
		if i%2 == 1 {
			fixture = "../../eval/fixtures/hmosperf_catalog_objects/captures/nested/capture.data"
		}
		body, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(repo, "captures", session, "capture.data")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0400); err != nil {
			t.Fatal(err)
		}
		originals[path] = body
	}
	request := "请对 captures 目录中的采集做一次抽样对照：按线程和检查窗口归类，每类挑两份代表，比较 10.000 到 10.050 秒内线程 101 和 202 的分配、释放及调用位置，并概括各组的检查进度。"
	model := &hmc270RepresentativeAdapter{}
	finalModel := &hmc270RepresentativeFinalizer{namedInputHMC223LLM: namedInputHMC223LLM{
		finalize: true,
		params:   json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"Selected source-local query results are available. The fixed representative subset does not establish facts for the unselected captures or a common cause."}]}`),
	}}
	registry := tool.NewRegistry()
	registry.Register(&tool.TraceCatalog{})
	registry.Register(&tool.TraceQuery{})
	registry.Register(&tool.EmitInvestigationComplete{})
	registry.Register(&tool.EmitAnswerDocument{})
	deps := func(model llm.Adapter, iterations int) *agent.Dependencies {
		return &agent.Dependencies{LLM: model, Tools: registry, MaxIterations: iterations, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}}
	}
	explorer := agent.NewExplorerAgent(deps(model, 5))
	finalizer := agent.NewFinalizerAgent(deps(finalModel, 1))
	defaultSkills := skill.NewRegistry()
	skill.RegisterDefaults(defaultSkills)
	exploreSkill, err := defaultSkills.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 10.0, 10.05
	ir := dagIR(types.AnswerContract{})
	ir.RequestModel = types.RequestModel{
		RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly},
		RuntimeTargets: []types.RuntimeTarget{
			{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit", Confidence: 1},
			{Kind: types.RuntimeTargetKindThread, PID: 202, Source: "user_explicit", Confidence: 1},
		},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.000 到 10.050 秒"},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
		RequestedAnswerDimensions:   &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionObservedValue, Required: true, Label: "分配、释放及调用位置", Index: 1}}},
	}
	ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "representatives", Type: types.NodeEvidence, Objective: request, OneShot: true}, {ID: "answer", Type: types.NodeFinalize, Objective: "Compare selected source-local facts and retain unselected coverage", OneShot: true}}, Edges: []types.TaskEdge{{From: "representatives", To: "answer", EdgeType: types.EdgeHardDependency}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
	ir.EvidencePlan.Budget.MaxReactIters = 12
	var output *agent.StageOutput
	finalized := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if len(ctx.Mutable.TraceCatalogs()) != 0 || len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Error("fixture preinstalled catalog or preparation authority")
			}
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			var err error
			output, err = explorer.Execute(ctx, exploreSkill)
			return output, err
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalized++
			return finalizer.Execute(ctx, &skill.Config{Name: "answer-document-skill", ToolSuggestions: []string{"emit_answer_document"}})
		},
	})
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetTraceRuntimeAnchor(t.TempDir())
	bus, runErr := o.Run(request, repo, "main")
	if runErr != nil || output == nil || finalized != 1 || model.step != 5 || !finalModel.visible {
		t.Fatalf("normal Run representative handoff: err=%v output=%+v steps=%d finalizers=%d surface=%t", runErr, output, model.step, finalized, finalModel.visible)
	}
	if bus.Mutable.AnswerDocumentV2() == nil {
		t.Fatal("real Finalizer did not emit the answer document")
	}
	catalogs := bus.Mutable.TraceCatalogs()
	if len(catalogs) != 1 {
		t.Fatalf("normal Run retained %d catalogs, want one exact root", len(catalogs))
	}
	current := catalogs[0].CurrentExecution()
	selected, ok := catalogs[0].Representatives()
	if !ok || selected.SelectionID != model.selectionID || len(selected.Clusters) != 2 || selected.Coverage.Selected != 4 || selected.Coverage.Members != 8 {
		t.Fatalf("current selection lost fixed group/member population: %+v", selected)
	}
	if current.Coverage.DeclaredPlans != 8 || current.Coverage.Attempts != 4 || current.Coverage.ObservedQueries != 0 || current.Coverage.ResumedPlans != 0 {
		t.Fatalf("current execution counts are not real current attempts: %+v", current.Coverage)
	}
	queries := map[string]tracecatalog.QueryRecord{}
	artifacts := map[string]string{}
	for _, artifact := range current.Snapshot.Artifacts {
		artifacts[artifact.ID] = artifact.Path
	}
	for _, q := range current.Snapshot.Queries {
		queries[q.ID] = q
	}
	selectedPayloads := map[string]tracecatalog.QueryRecord{}
	selectedSources := map[string]bool{}
	for _, group := range selected.Clusters {
		for _, member := range group.Members {
			q := queries[member.QueryID]
			wantAttempts := 0
			if member.Selected {
				wantAttempts = 1
				if q.Outcome != tracecatalog.OutcomeSuccess && q.Outcome != tracecatalog.OutcomeEmpty {
					t.Errorf("selected actual query failed: %+v", q)
				}
				if len(q.Attempts) == 1 && q.Attempts[0].PayloadRef != "" {
					if _, duplicate := selectedPayloads[q.Attempts[0].PayloadRef]; duplicate {
						t.Fatal("independent selected plans shared a payload identity")
					}
					selectedPayloads[q.Attempts[0].PayloadRef] = q
					selectedSources[artifacts[q.ArtifactID]] = true
				}
			}
			if len(q.Attempts) != wantAttempts {
				t.Errorf("selected=%t query %s attempts=%d want=%d", member.Selected, q.ID, len(q.Attempts), wantAttempts)
			}
		}
	}
	queryResults := 0
	actualQueries, actualSources := map[string]bool{}, map[string]bool{}
	var nativeDisplays []string
	for _, result := range output.ToolResults {
		if result.ToolName != "trace_query" {
			continue
		}
		queryResults++
		if !result.Success {
			t.Fatalf("ordinary selected query failed: %s", result.Summary)
		}
		matched := false
		for _, observation := range result.Observations {
			resource, ok := tool.DecodeTraceResourceStack(observation)
			if !ok {
				continue
			}
			q, found := selectedPayloads[observation.SourceRef.PayloadRef]
			if !found || actualQueries[q.ID] {
				t.Fatalf("actual native payload is not a unique selected plan: %q", observation.SourceRef.PayloadRef)
			}
			originalPath := artifacts[q.ArtifactID]
			owned := false
			for _, material := range bus.TraceInputPreparer.PreparedMaterials() {
				if material.SourcePath() == originalPath && material.OwnsQueryArtifact(resource.SourcePath) {
					owned = true
				}
			}
			if !owned {
				t.Fatalf("native result lost exact selected original ownership: original=%s query=%s", originalPath, resource.SourcePath)
			}
			var plan struct {
				PID int `json:"pid"`
			}
			if err := json.Unmarshal(q.Parameters, &plan); err != nil || plan.PID != resource.TargetPID {
				t.Fatalf("native target differs from selected plan %s: %+v %v", q.ID, plan, err)
			}
			actualQueries[q.ID], actualSources[originalPath] = true, true
			wantEvents, wantFrames := 0, 0
			switch resource.TargetPID {
			case 101:
				wantEvents, wantFrames = 3, 8
			case 202:
				wantEvents, wantFrames = 1, 3
			default:
				t.Fatalf("query changed requested thread: %d", resource.TargetPID)
			}
			if strings.Contains(originalPath, "session-b") || strings.Contains(originalPath, "session-d") {
				wantEvents, wantFrames = 1, 3
				if resource.TargetPID == 202 {
					wantEvents, wantFrames = 0, 0
				}
			}
			frames := 0
			for _, event := range resource.Events {
				frames += len(event.Frames)
			}
			if resource.MatchedEvents != wantEvents || resource.OmittedEvents != 0 || frames != wantFrames || resource.Window.StartTs != start || resource.Window.EndTs != end || resource.Window.EndInclusive {
				t.Fatalf("source-local native oracle mismatch: %+v frames=%d want=%d/%d", resource, frames, wantEvents, wantFrames)
			}
			members := make([]int, len(resource.Events))
			for i := range members {
				members[i] = i
			}
			nativeDisplays = append(nativeDisplays, tool.TraceResourceStackSelectedText(resource, members, 12))
			matched = true
		}
		if !matched {
			t.Fatal("actual query omitted native resource result")
		}
	}
	if queryResults != 4 || len(actualQueries) != 4 || len(actualSources) != len(selectedSources) {
		t.Fatalf("actual selected calls=%d distinct plans=%d sources=%d selected sources=%d", queryResults, len(actualQueries), len(actualSources), len(selectedSources))
	}
	t.Logf("actual execution: 8 declared plans, 4 selected plans, 4 attempts, %d distinct original captures; not four completed captures", len(actualSources))
	var finalPrompt strings.Builder
	for _, message := range finalModel.messages {
		finalPrompt.WriteString(message.Content)
		finalPrompt.WriteByte('\n')
	}
	for _, want := range []string{selected.SelectionID, `"declared_plans":8`, `"attempts":4`, `"members":8`, `"selected":4`, `"budget_not_selected":4`, "Representative comparison", "resource_stack"} {
		if !strings.Contains(finalPrompt.String(), want) {
			t.Errorf("actual finalizer context omitted %q", want)
		}
	}
	for _, display := range nativeDisplays {
		if display == "" || !strings.Contains(finalPrompt.String(), display) {
			t.Errorf("actual finalizer lost a complete selected native event/frame display: %s", display)
		}
	}
	for _, group := range selected.Clusters {
		for _, member := range group.Members {
			if !strings.Contains(finalPrompt.String(), member.QueryID) || !strings.Contains(finalPrompt.String(), member.Path) {
				t.Errorf("finalizer omitted current member source/navigation %s %s", member.QueryID, member.Path)
			}
			if member.Selected && (member.LatestAttempt == nil || member.LatestAttempt.PayloadRef == "" || !strings.Contains(finalPrompt.String(), member.LatestAttempt.PayloadRef)) {
				t.Errorf("finalizer omitted selected native payload reference: %+v", member)
			}
		}
	}
	for path, original := range originals {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, original) {
			t.Fatalf("original capture changed: %s %v", path, err)
		}
	}
}
