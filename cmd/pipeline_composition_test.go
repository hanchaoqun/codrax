package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The read pipeline a command-line run asks questions through and the one a
// reproduction of a recorded run asks them through are the same construction:
// initApp may add provider-built wiring around it, but the settings an absent
// settings file resolves to, the skills, the tools, the agents and the
// orchestrator all come from the functions below. A second construction next
// to them would let the two drift without any test noticing.

func initAppCalls(t *testing.T) map[string]int {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "root.go", nil, 0)
	if err != nil {
		t.Fatalf("parse root.go: %v", err)
	}
	calls := map[string]int{}
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "initApp" {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				calls[fun.Name]++
			case *ast.SelectorExpr:
				if pkg, ok := fun.X.(*ast.Ident); ok {
					calls[pkg.Name+"."+fun.Sel.Name]++
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("root.go no longer declares initApp")
	}
	return calls
}

func TestInitAppBuildsThePipelineThroughTheSharedComposition(t *testing.T) {
	calls := initAppCalls(t)
	for _, shared := range []string{
		"resolvePipelineSettings", "resolveLogTriageSettings", "resolvePerfTriageSettings",
		"newPipelineSkillRegistry", "composePipeline",
	} {
		if calls[shared] != 1 {
			t.Errorf("initApp calls %s %d times, want exactly once", shared, calls[shared])
		}
	}
	for _, own := range []string{
		"orchestrator.New", "agent.NewRegistry", "agent.RegisterDefaults",
		"agent.NewSubAgentRegistry", "agent.RegisterDefaultSubAgents",
		"tool.NewRegistry", "tool.RegisterDefaults",
		"skill.NewRegistry", "skill.RegisterDefaults",
		"types.ResolvedAgentSettings", "types.ResolvedExploreHeuristics",
		"agent.DefaultLogTriageSettings", "agent.DefaultPerfTriageSettings",
	} {
		if calls[own] != 0 {
			t.Errorf("initApp builds part of the pipeline itself through %s; that belongs to the shared composition", own)
		}
	}
}

func TestPipelineSettingsWithoutASettingsFileAreTheCodeDefaults(t *testing.T) {
	none := func(string) bool { return false }
	got := resolvePipelineSettings(nil, none, nil)
	if got.MaxRetriesPerStage != 3 || got.WriteRetryBudget != 3 || got.TransientRetryBudget != 3 ||
		got.MaxUpstreamFallbacksPerRun != 2 || got.MaxParallelism != types.DefaultPipelineMaxParallelism ||
		got.MaxStageVisits != 0 || got.WriteWorkflowEngine != types.WriteWorkflowEngineController {
		t.Fatalf("seeded pipeline defaults changed: %+v", got)
	}
	if !reflect.DeepEqual(got.ViolationBudget, types.DefaultViolationBudgetSettings()) {
		t.Fatalf("violation budget = %+v, want the default table", got.ViolationBudget)
	}
	if want := types.ResolvedAgentSettings(types.AgentSettings{}); !reflect.DeepEqual(got.Agent, want) {
		t.Fatalf("agent settings without a settings file are not the resolved defaults:\n got %+v\nwant %+v", got.Agent, want)
	}
	if want := types.ResolvedExploreHeuristics(types.ExploreHeuristics{}); !reflect.DeepEqual(got.Explore.Heuristics, want) {
		t.Fatalf("explore heuristics without a settings file are not the resolved defaults:\n got %+v\nwant %+v", got.Explore.Heuristics, want)
	}

	// The two command-line overrides apply only when their flag was passed,
	// and only with a positive value.
	oldRetries, oldVisits := flagMaxRetries, flagMaxStageVisits
	t.Cleanup(func() { flagMaxRetries, flagMaxStageVisits = oldRetries, oldVisits })
	flagMaxRetries, flagMaxStageVisits = 5, 7
	if got := resolvePipelineSettings(nil, none, nil); got.MaxRetriesPerStage != 3 || got.MaxStageVisits != 0 {
		t.Fatalf("flag values applied without the flags being passed: retries=%d visits=%d", got.MaxRetriesPerStage, got.MaxStageVisits)
	}
	var asked []string
	passed := func(name string) bool { asked = append(asked, name); return true }
	if got := resolvePipelineSettings(nil, passed, nil); got.MaxRetriesPerStage != 5 || got.MaxStageVisits != 7 {
		t.Fatalf("passed flags did not override: retries=%d visits=%d", got.MaxRetriesPerStage, got.MaxStageVisits)
	}
	sort.Strings(asked)
	if want := []string{"pipeline-max-retries", "pipeline-max-stage-visits"}; !reflect.DeepEqual(asked, want) {
		t.Fatalf("flags consulted without a settings file = %v, want %v", asked, want)
	}
	onlyRetries := func(name string) bool { return name == "pipeline-max-retries" }
	if got := resolvePipelineSettings(nil, onlyRetries, nil); got.MaxRetriesPerStage != 5 || got.MaxStageVisits != 0 {
		t.Fatalf("one passed flag moved the other knob: retries=%d visits=%d", got.MaxRetriesPerStage, got.MaxStageVisits)
	}
	flagMaxRetries, flagMaxStageVisits = 0, 0
	if got := resolvePipelineSettings(nil, passed, nil); got.MaxRetriesPerStage != 3 || got.MaxStageVisits != 0 {
		t.Fatalf("a passed flag with no positive value overrode the default: retries=%d visits=%d", got.MaxRetriesPerStage, got.MaxStageVisits)
	}
}

func TestTriageSettingsWithoutASettingsFileAreTheCodeDefaults(t *testing.T) {
	if got, want := resolveLogTriageSettings(nil), agent.DefaultLogTriageSettings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("log triage settings = %+v, want %+v", got, want)
	}
	if got, want := resolvePerfTriageSettings(nil), agent.DefaultPerfTriageSettings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("perf triage settings = %+v, want %+v", got, want)
	}
}

// compositionProbeLLM is one agent's adapter in a composition test. It
// declares its own sizing and reports every request it is asked.
type compositionProbeLLM struct {
	agentName types.AgentName
	window    int
	onChat    func(name types.AgentName, messages []llm.Message, tools []llm.ToolSchema) (llm.Response, error)
}

func (p *compositionProbeLLM) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	return p.onChat(p.agentName, messages, tools)
}
func (p *compositionProbeLLM) ModelID() string               { return "composition-probe" }
func (p *compositionProbeLLM) MaxContextTokens() int         { return p.window }
func (p *compositionProbeLLM) MaxOutputTokens() int          { return 0 }
func (p *compositionProbeLLM) RequestTimeout() time.Duration { return time.Minute }
func (p *compositionProbeLLM) RetryMaxAttempts() int         { return 1 }

const composeProbeLanguage = "en"

func composeProbePipeline(t *testing.T, onChat func(types.AgentName, []llm.Message, []llm.ToolSchema) (llm.Response, error), clock agent.DynamicContextClock) (pipelineComposition, []types.AgentName) {
	t.Helper()
	return composeProbePipelineIn(t, onChat, clock, "")
}

func composeProbePipelineIn(t *testing.T, onChat func(types.AgentName, []llm.Message, []llm.ToolSchema) (llm.Response, error), clock agent.DynamicContextClock, blobSessionDir string) (pipelineComposition, []types.AgentName) {
	t.Helper()
	var mu sync.Mutex
	var resolved []types.AgentName
	renderer := render.New(io.Discard, false)
	composed := composePipeline(pipelineCompositionInputs{
		Settings:   resolvePipelineSettings(nil, func(string) bool { return false }, nil),
		DefaultLLM: &compositionProbeLLM{agentName: "default", window: 200000, onChat: onChat},
		Resolver: func(name types.AgentName) llm.Adapter {
			mu.Lock()
			defer mu.Unlock()
			resolved = append(resolved, name)
			return &compositionProbeLLM{agentName: name, window: 200000, onChat: onChat}
		},
		MCPServers:                   mcp.NewRegistry(),
		Skills:                       newPipelineSkillRegistry(),
		Emit:                         renderer.Emitter(),
		LogTriage:                    resolveLogTriageSettings(nil),
		PerfTriage:                   resolvePerfTriageSettings(nil),
		MaxSteps:                     6,
		Language:                     composeProbeLanguage,
		BlobSessionDir:               blobSessionDir,
		RuntimeAnchor:                t.TempDir(),
		FinalizerDynamicContextClock: clock,
	})
	return composed, resolved
}

func TestComposePipelineBuildsEveryAgentFromTheResolver(t *testing.T) {
	refuse := func(types.AgentName, []llm.Message, []llm.ToolSchema) (llm.Response, error) {
		return llm.Response{}, errors.New("no request expected")
	}
	composed, resolved := composeProbePipeline(t, refuse, nil)
	if composed.Orchestrator == nil || composed.Agents == nil || composed.Tools == nil || composed.SubAgents == nil || composed.ReasoningCollector == nil {
		t.Fatalf("composition is incomplete: %+v", composed)
	}
	registered := composed.Agents.List()
	sort.Slice(registered, func(i, j int) bool { return registered[i] < registered[j] })
	sort.Slice(resolved, func(i, j int) bool { return resolved[i] < resolved[j] })
	if !reflect.DeepEqual(registered, resolved) {
		t.Fatalf("agents registered and adapters resolved differ:\nregistered %v\n  resolved %v", registered, resolved)
	}
	for _, name := range []types.AgentName{types.AgentAnalyzer, types.AgentExplorer, types.AgentExtractor, types.AgentFinalizer, types.AgentPerfTriager} {
		if _, err := composed.Agents.Get(name); err != nil {
			t.Errorf("read pipeline agent %s is not registered: %v", name, err)
		}
	}

	// The explorer's closing tools are registered at run time and offered
	// through its skill; the answer tools exist for the finalizer.
	skills := newPipelineSkillRegistry()
	plain, err := skills.Get("explore-skill")
	if err != nil {
		t.Fatalf("explore-skill: %v", err)
	}
	offered := strings.Join(plain.ToolSuggestions, ",")
	if strings.Contains(offered, "emit_investigation_complete") || strings.Contains(offered, "emit_evidence") {
		t.Fatalf("a freshly built skill registry already offers the run-time explorer tools: %s", offered)
	}
	composedAgain := composePipeline(pipelineCompositionInputs{
		Settings: resolvePipelineSettings(nil, func(string) bool { return false }, nil),
		Resolver: func(types.AgentName) llm.Adapter { return nil }, MCPServers: mcp.NewRegistry(), Skills: skills,
		Emit: render.New(io.Discard, false).Emitter(), RuntimeAnchor: t.TempDir(),
	})
	suggestions := map[string]int{}
	for _, name := range plain.ToolSuggestions {
		suggestions[name]++
	}
	if suggestions["emit_evidence"] != 1 || suggestions["emit_investigation_complete"] != 1 {
		t.Fatalf("composition did not offer the explorer its closing tools exactly once: %v", plain.ToolSuggestions)
	}
	for _, name := range []string{
		"emit_evidence", "emit_investigation_complete", "emit_answer_symbol", "emit_hypothesis_verdict",
		"emit_answer_document", "emit_answer_document_patch", "emit_log_triage", "emit_log_segmentation",
		"emit_perf_trace", "emit_perf_segmentation", "emit_write_analysis", "emit_write_workflow_decision",
		"repo_map", "propose_sub_agents",
	} {
		if _, err := composedAgain.Tools.Get(name); err != nil {
			t.Errorf("tool %s is not registered by the composition: %v", name, err)
		}
	}
}

// A request reaches the adapter the resolver supplied for that agent, and an
// agent built by the composition observes a cancellation of the orchestrator
// that was created after it: once the run is cancelled the agent does not ask
// its model again, even though its first response left work to do.
func TestComposedPipelineAsksTheResolvedAdapterAndObservesCancellation(t *testing.T) {
	var mu sync.Mutex
	var asked []types.AgentName
	var composed pipelineComposition
	onChat := func(name types.AgentName, messages []llm.Message, tools []llm.ToolSchema) (llm.Response, error) {
		mu.Lock()
		asked = append(asked, name)
		first := len(asked) == 1
		mu.Unlock()
		if len(messages) == 0 || len(tools) == 0 {
			t.Errorf("agent %s asked with %d messages and %d tools", name, len(messages), len(tools))
		}
		if !first {
			return llm.Response{}, errors.New("composition probe answers one request only")
		}
		composed.Orchestrator.Cancel("composition probe saw the first request")
		return llm.Response{StopReason: "tool_calls", ToolCalls: []llm.ToolCall{{
			ID: "probe-1", Name: "grep", Params: json.RawMessage(`{"pattern":"main","files_only":true}`),
		}}}, nil
	}
	session := t.TempDir()
	composed, _ = composeProbePipelineIn(t, onChat, nil, session)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	var bus *types.BusContext
	go func() {
		var err error
		bus, err = composed.Orchestrator.Run("这个仓库的入口函数在哪里？", repo, "main")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled run reported success")
		}
	case <-time.After(2 * time.Minute):
		t.Fatal("the run did not end after the orchestrator was cancelled")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(asked) != 1 || asked[0] != types.AgentAnalyzer {
		t.Fatalf("requests after a cancellation at the first one = %v, want exactly one from the analyzer", asked)
	}
	// The run used the language and the session directory the composition
	// was given, not a default of its own.
	if bus == nil || bus.Language != composeProbeLanguage || bus.WorkDir != session {
		t.Fatalf("run did not carry the composition's language and session directory: %+v", bus)
	}
}

// The finalizer the composition registers reads its dynamic-context budget
// through the clock the composition was given, so a reproduction decides at
// which unit the budget passes; a composition without one asks the wall clock.
func TestComposedFinalizerReadsItsDynamicContextBudgetFromTheInjectedClock(t *testing.T) {
	dispatch := func(clock agent.DynamicContextClock) (requests int) {
		t.Helper()
		onChat := func(name types.AgentName, _ []llm.Message, _ []llm.ToolSchema) (llm.Response, error) {
			if name != types.AgentFinalizer {
				t.Errorf("the finalizer's request reached the adapter of %s", name)
			}
			requests++
			return llm.Response{}, errors.New("composition probe stops at the first request")
		}
		composed, _ := composeProbePipeline(t, onChat, clock)
		finalizer, err := composed.Agents.Get(types.AgentFinalizer)
		if err != nil {
			t.Fatal(err)
		}
		answerSkill, err := newPipelineSkillRegistry().Get("answer-document-skill")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = finalizer.Execute(&types.AgentContext{AgentName: types.AgentFinalizer, Stage: types.StageFinalize}, answerSkill)
		return requests
	}
	var seen []agent.DynamicContextCheckpoint
	if got := dispatch(func(at agent.DynamicContextCheckpoint) time.Duration {
		seen = append(seen, at)
		return 0
	}); got != 1 {
		t.Fatalf("finalizer requests with an injected clock = %d, want 1", got)
	}
	if len(seen) == 0 {
		t.Fatal("the composed finalizer never asked the injected clock")
	}
	if got := dispatch(nil); got != 1 {
		t.Fatalf("finalizer requests without an injected clock = %d, want 1", got)
	}
}
