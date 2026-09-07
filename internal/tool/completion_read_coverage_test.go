package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCompletionReadCoverage_RealCompletionThenPagedRead(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte("one\ntwo\nthree"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("explain")}
	closure := ctx.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{File: "a.go", Origin: "phase1_unread"})
	closure.AddPendingRead(types.PendingRead{File: "b.go", Origin: "phase1_unread", LineRanges: []types.LineRange{{Start: 2, End: 3}}})
	_ = preCompleteContractCheck(ctx, "evidence-bounded completion")
	if len(closure.CompletionCaveats()) == 0 || len(closure.CurrentCompletionCaveats()) == 0 {
		t.Fatal("real completion must disclose demoted unread sources")
	}
	read := func(file string, offset, limit int) types.ToolReadCoverage {
		t.Helper()
		params, _ := json.Marshal(map[string]any{"path": file, "line_offset": offset, "limit": limit})
		result, err := (&ReadFile{}).Execute(ctx, params)
		if err != nil || !result.Success || result.ReadCoverage == nil || result.RawRef == "" {
			t.Fatalf("read: %+v %v", result, err)
		}
		return *result.ReadCoverage
	}
	partial := read("a.go", 1, 1)
	if partial.LineStart != 2 || partial.LineEnd != 2 || partial.TotalLines != 3 {
		t.Fatalf("actual entry must exercise a partial read, got %+v", partial)
	}
	if len(closure.CurrentCompletionCaveats()) == 0 {
		t.Fatal("partial first file must not settle two sources")
	}
	read("b.go", 1, 2)
	if len(closure.CurrentCompletionCaveats()) == 0 {
		t.Fatal("first source still has missing ranges after second source is settled")
	}
	read("a.go", 0, 0)
	if got := closure.CurrentCompletionCaveats(); len(got) != 0 {
		t.Fatalf("later actual reads must settle current display debt, got %+v", got)
	}
	if len(closure.CompletionCaveats()) == 0 {
		t.Fatal("historical completion boundary must remain")
	}
}

func TestCompletionReadCoverage_RejectsNonSourceAndForeignReads(t *testing.T) {
	root, foreign := t.TempDir(), t.TempDir()
	for _, dir := range []string{root, foreign} {
		if err := os.WriteFile(filepath.Join(dir, "same.go"), []byte("one"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("explain")}
	appendAdvisoryReadCoverageCaveat(ctx, []types.PendingRead{{File: "same.go"}})
	good := types.ToolResult{ToolName: "read_file", Success: true, RawRef: "proof", ReadCoverage: &types.ToolReadCoverage{Path: "same.go", LineStart: 1, LineEnd: 1, TotalLines: 1, RawRef: "proof"}}
	failed, grep, mismatch, artifact := good, good, good, good
	failed.Success = false
	grep.ToolName = "grep"
	mismatch.RawRef = "different"
	artifact.RuntimeArtifactRead = &types.ToolRuntimeArtifactRead{}
	for _, result := range []types.ToolResult{failed, grep, mismatch, artifact} {
		recordCompletionReadCoverage(ctx, filepath.Join(root, "same.go"), result)
	}
	recordCompletionReadCoverage(ctx, filepath.Join(foreign, "same.go"), good)
	other := &types.BusContext{RepoRoot: foreign, Mutable: ctx.Mutable}
	recordCompletionReadCoverage(other, filepath.Join(foreign, "same.go"), good)
	if len(ctx.Mutable.EvidenceClosure().CurrentCompletionCaveats()) == 0 {
		t.Fatal("non-source or foreign reads must not settle local source")
	}
}
