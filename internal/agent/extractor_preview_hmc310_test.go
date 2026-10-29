package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC310ExtractorDoesNotLoadUnseenPreviewLiteral(t *testing.T) {
	root := t.TempDir()
	body := `var hidden = "unseen-literal"; // ` + strings.Repeat("x", tool.MaxInlineBytes*2) + "\n"
	if err := os.WriteFile(filepath.Join(root, "long.go"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("list literals")}
	params, _ := json.Marshal(map[string]any{"path": "long.go", "limit": 1})
	out, err := (&tool.ReadFile{}).Execute(bus, params)
	if err != nil || !out.Success || out.ReadCoverage == nil || out.RawRef == "" {
		t.Fatalf("public successful zero-row preview premise failed: %+v %v", out, err)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{out}})
	ctx := &types.AgentContext{RepoRoot: root, Mutable: bus.Mutable}
	if got := extractorReadFileLineIndex(ctx); len(got) != 0 {
		t.Fatalf("unseen raw artifact literal acquired line authority: %+v", got)
	}
}
