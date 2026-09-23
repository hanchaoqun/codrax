package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreReadPhysicalLinesAndEmptyCoverage(t *testing.T) {
	for _, tc := range []struct {
		content string
		total   int
	}{{"", 0}, {"line\n", 1}, {"line\n\n", 2}, {"line\r\nlast", 2}} {
		t.Run(fmt.Sprintf("%q", tc.content), func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "source.go"), []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			ctx := &types.AgentContext{RepoRoot: root, Mutable: types.NewMutableState("observe source")}
			c := ctx.Mutable.EvidenceClosure()
			physicalRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			c.AppendScopedReadCoverageCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneForcedReadCoverage}, []types.ScopedReadCoverage{{RepositoryRoot: physicalRoot, Path: "source.go"}})
			got := preReadRequiredFilesTracked(ctx, root, []string{"source.go"}, 1, 100, nil)
			if !strings.Contains(got, fmt.Sprintf("full file, %d lines", tc.total)) || c.FileTotalLines("source.go") != tc.total || !c.HasFullyRead("source.go") {
				t.Fatalf("pre-read physical coverage drift: %q %+v", got, c.FileTotalLinesSnapshot())
			}
			if tc.total == 0 && (c.HasReadLine("source.go", 1) || len(c.CurrentCompletionCaveats()) != 0 || strings.Contains(got, "1\t")) {
				t.Fatalf("empty pre-read fabricated a line or lost whole-file observation: %q", got)
			}
			_, _, _, lines, _, _, _, _ := ctx.Mutable.GroundingContextSnapshot()
			if len(lines["source.go"]) != tc.total {
				t.Fatalf("gutter/source index disagree: %+v", lines)
			}
		})
	}
}

func TestPreReadPhysicalEmptyKeepsLineDebt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.go"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{RepoRoot: root, Mutable: types.NewMutableState("observe source")}
	c := ctx.Mutable.EvidenceClosure()
	c.AppendScopedReadCoverageCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneForcedReadCoverage}, []types.ScopedReadCoverage{{RepositoryRoot: physicalRoot, Path: "source.go", LineRanges: []types.LineRange{{Start: 1, End: 1}}}})
	preReadRequiredFilesTracked(ctx, root, []string{"source.go"}, 1, 100, nil)
	if len(c.CurrentCompletionCaveats()) == 0 || c.HasReadLine("source.go", 1) {
		t.Fatal("empty pre-read satisfied nonexistent line debt")
	}
}

func TestReadFilePhysicalEmptyRuntimeGuidance(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codrax"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codrax", "empty.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("read runtime")}
	p, _ := json.Marshal(map[string]any{"path": ".codrax/empty.txt"})
	r, err := (&tool.ReadFile{}).Execute(ctx, p)
	if err != nil || !r.Success {
		t.Fatalf("read failed: %+v %v", r, err)
	}
	win, ok := runtimeArtifactReadWindowFromResult(r)
	if !ok || !win.typed || win.broadHeader || win.start != 0 || win.end != 0 {
		t.Fatalf("empty runtime misclassified: %+v", win)
	}
	for _, hint := range []string{renderRuntimeArtifactReadOnlyHint(win), renderCompactRuntimeArtifactReadHint(win)} {
		if !strings.Contains(hint, "empty (0 lines)") || strings.Contains(hint, "lines 0-0") || strings.Contains(hint, "lines 1-1") {
			t.Fatalf("empty artifact guidance invents a range: %s", hint)
		}
	}
}
