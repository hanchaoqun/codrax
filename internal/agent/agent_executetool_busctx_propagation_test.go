package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// captureBusCtxTool is a minimal Tool implementation whose Execute
// records the BusContext it was handed. Used to assert the fields
// agent.go::executeTool must propagate from AgentContext to the
// narrow busCtx the tool sees.
type captureBusCtxTool struct {
	tool.ReadOnly
	tool.NonEvidenceTool
	captured *types.BusContext
}

func (t *captureBusCtxTool) Name() string                { return "capture_busctx" }
func (t *captureBusCtxTool) Description() string         { return "test capture" }
func (t *captureBusCtxTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (t *captureBusCtxTool) Execute(ctx *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	t.captured = ctx
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: "ok"}, nil
}

// stubMemoryReader is the minimal MemoryReader fake for the
// propagation test — Search returns a canned IndexEntry the
// assertion can detect.
type stubMemoryReader struct{ tag string }

func (s *stubMemoryReader) Search(query string, _ types.MemorySearchOpts) []types.MemoryIndexEntry {
	return []types.MemoryIndexEntry{{ID: s.tag, Topic: query}}
}

// TestExecuteTool_PropagatesReadOnlyFieldsToBusCtx pins the
// regression fix shipped in this commit: agent.go::executeTool MUST
// copy Memory / EnvFacts / EnvRecommendSettings / Language /
// Preferences / MainRepoRoot from AgentContext into the narrow
// busCtx the tool sees. Pre-fix only RepoRoot/Branch/Commit/WorkDir/
// Mutable/AnalysisIR were copied, silently nullifying recall_memory
// and the env_recommend integration in agent dispatch paths.
func TestExecuteTool_PropagatesReadOnlyFieldsToBusCtx(t *testing.T) {
	mem := &stubMemoryReader{tag: "MEM-EXECTOOL"}
	facts := &types.EnvFacts{OS: "linux"}
	settings := types.EnvRecommendSettings{Enabled: true}

	cap := &captureBusCtxTool{}
	reg := tool.NewRegistry()
	reg.Register(cap)

	b := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{Tools: reg},
	}

	ctx := &types.AgentContext{
		AgentName:            types.AgentExplorer,
		Stage:                types.StageExplore,
		RepoRoot:             "/tmp/repo",
		Branch:               "main",
		Commit:               "abc",
		WorkDir:              "/tmp/wd",
		MainRepoRoot:         "/home/user/orig-repo",
		Memory:               mem,
		EnvFacts:             facts,
		EnvRecommendSettings: settings,
		Language:             "zh",
		Preferences:          []string{"verbose"},
	}

	tc := llm.ToolCall{Name: "capture_busctx", Params: json.RawMessage(`{}`)}
	res, _ := b.executeTool(ctx, tc)
	if res == nil || !res.Success {
		t.Fatalf("capture tool execution failed: %+v", res)
	}
	if cap.captured == nil {
		t.Fatal("capture tool did not record the busCtx")
	}
	bc := cap.captured

	// Memory: the canonical regression — recall_memory's
	// `ctx.Memory == nil` early-return triggers when this is dropped.
	if bc.Memory == nil {
		t.Errorf("Memory dropped at agent boundary — recall_memory would return 'unavailable'")
	} else if got := bc.Memory.Search("probe", types.MemorySearchOpts{}); len(got) != 1 || got[0].ID != "MEM-EXECTOOL" {
		t.Errorf("Memory adapter identity lost: %+v", got)
	}

	// EnvFacts + EnvRecommendSettings: env_recommend integration in
	// run_tests.go reads both. Pre-fix the integration was DOA in
	// agent paths.
	if bc.EnvFacts == nil || bc.EnvFacts.OS != "linux" {
		t.Errorf("EnvFacts dropped: got %+v", bc.EnvFacts)
	}
	if !bc.EnvRecommendSettings.Enabled {
		t.Errorf("EnvRecommendSettings dropped: got %+v", bc.EnvRecommendSettings)
	}

	// Language + Preferences: bilingual rendering / user prefs in
	// run_tests reports + emit_answer_document language detection.
	if bc.Language != "zh" {
		t.Errorf("Language dropped: got %q", bc.Language)
	}
	if len(bc.Preferences) != 1 || bc.Preferences[0] != "verbose" {
		t.Errorf("Preferences dropped: got %+v", bc.Preferences)
	}

	// MainRepoRoot: env_recommend bare-dir authorization message
	// references it.
	if bc.MainRepoRoot != "/home/user/orig-repo" {
		t.Errorf("MainRepoRoot dropped: got %q", bc.MainRepoRoot)
	}

	// Sanity: the fields that WERE propagated pre-fix are still
	// propagated. Lock the existing 5 + Mutable so a future refactor
	// that accidentally reorders the struct literal can't regress.
	if bc.RepoRoot != "/tmp/repo" {
		t.Errorf("RepoRoot lost: got %q", bc.RepoRoot)
	}
	if bc.Branch != "main" {
		t.Errorf("Branch lost: got %q", bc.Branch)
	}
	if bc.Commit != "abc" {
		t.Errorf("Commit lost: got %q", bc.Commit)
	}
	if bc.WorkDir != "/tmp/wd" {
		t.Errorf("WorkDir lost: got %q", bc.WorkDir)
	}
}
