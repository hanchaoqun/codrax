package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A producer's recommended parameters must survive the actual consumer's
// stage guard, not merely satisfy a second copy of that guard in tool tests.
func TestSourceInventoryRefinementSurvivesCurrentStageDispatch(t *testing.T) {
	for _, stage := range []types.PipelineStage{types.StageAnalyze, types.StageExplore} {
		t.Run(string(stage), func(t *testing.T) {
			repo := t.TempDir()
			if err := os.Mkdir(filepath.Join(repo, "nested"), 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"top.go", "nested/inner.go"} {
				if err := os.WriteFile(filepath.Join(repo, name), []byte("package example\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			reg := toolpkg.NewRegistry()
			reg.Register(&repomap.RepoMapV2{})
			reg.Register(&toolpkg.ListFiles{})
			name := types.AgentAnalyzer
			if stage == types.StageExplore {
				name = types.AgentExplorer
			}
			base := NewBaseAgent(name, &Dependencies{Tools: reg}, nil)
			ctx := &types.AgentContext{Stage: stage, AgentName: name, RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("bounded discovery")}
			res, _ := base.executeTool(ctx, llm.ToolCall{
				Name: "repo_map", Params: json.RawMessage(`{"path":".","view":"source_inventory","roles":["file"]}`),
			})
			if res == nil || res.Success || res.Refinement == nil || res.Refinement.PreferredNextTool != "list_files" {
				t.Fatalf("expected actual repo_map file-role refusal with refinement: %+v", res)
			}
			recursive, err := strconv.ParseBool(res.Refinement.PreferredParams["recursive"])
			if err != nil {
				t.Fatal(err)
			}
			params, err := json.Marshal(map[string]any{"path": res.Refinement.PreferredParams["path"], "recursive": recursive, "include": "*.go"})
			if err != nil {
				t.Fatal(err)
			}
			call := llm.ToolCall{Name: res.Refinement.PreferredNextTool, Params: params}
			got, _ := base.executeTool(ctx, call)
			if got == nil || !got.Success {
				t.Fatalf("following the actual producer recommendation was rejected by the same stage: hint=%+v result=%+v", res.Refinement, got)
			}
			if !strings.Contains(got.Summary, "top.go") || strings.Contains(got.Summary, "inner.go") != (stage == types.StageExplore) {
				t.Fatalf("recommendation changed shallow/recursive execution scope: %s", got.Summary)
			}
			if stage == types.StageAnalyze {
				// Suggestions confer no authority to bypass a subsequent terminal
				// transition or to run the old recursive parameters.
				fresh := &types.AgentContext{Stage: stage, Mutable: types.NewMutableState("bounded discovery")}
				bad := validateAnalyzerPrescanToolCall(fresh, llm.ToolCall{Name: "list_files", Params: json.RawMessage(`{"path":".","recursive":true}`)})
				if bad == nil || bad.Repair == nil || bad.Repair.Code != analyzerListFilesShallowRequiredCode {
					t.Fatalf("original recursive guard was weakened: %+v", bad)
				}
				ctx.EmitStageRetryAttempt = 1
				terminal, _ := base.executeTool(ctx, call)
				if terminal == nil || terminal.Success || terminal.Repair == nil || terminal.Repair.Code != analyzerPrescanTerminalEmitModeCode {
					t.Fatalf("recommendation bypassed terminal emit-only mode: %+v", terminal)
				}
			}
		})
	}
}
