package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	coretypes "github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the real source parser and read/emit boundary. The original fixture
// stays unchanged; read_file's output artifacts go only into the test work dir.
func TestB1628ActualRunnerReadThenEmit(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "eval", "fixtures", "python-plugin-mro"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []FileEntry
	for _, file := range []string{"pipeline/runner.py", "pipeline/registry.py", "pipeline/plugins.py", "pipeline/base.py"} {
		path := filepath.Join(repo, file)
		stat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, FileEntry{RelPath: file, AbsPath: path, Language: repomaptypes.LangPython, Size: stat.Size()})
	}
	ctx := &coretypes.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: coretypes.NewMutableState("trace run_pipeline")}
	ctx.Mutable.SetSearchGraph(BuildGraph(repo, ParseFiles(entries, repo)))
	for _, file := range []string{"pipeline/runner.py", "pipeline/registry.py"} {
		params, _ := json.Marshal(map[string]any{"path": file})
		res, err := (&tool.ReadFile{}).Execute(ctx, params)
		if err != nil || !res.Success {
			t.Fatalf("actual read failed: %v %+v", err, res)
		}
		artifact, err := filepath.Rel(ctx.WorkDir, res.RawRef)
		if err != nil || res.RawRef == "" || artifact == ".." || strings.HasPrefix(artifact, ".."+string(filepath.Separator)) || filepath.IsAbs(artifact) {
			t.Fatalf("read artifact must stay in test work dir: ref=%q relative=%q err=%v", res.RawRef, artifact, err)
		}
		ctx.ToolResults = append(ctx.ToolResults, res)
	}
	gc := ground.BuildContext(ctx)
	rows := gc.LineIndex["pipeline/runner.py"]
	if got := rows[15]; got != "    plugin = resolve(kind)" {
		t.Fatalf("actual source gutter mismatch: %q", got)
	}
	for _, line := range []int{10, 12, 14} {
		if !ground.LineLooksCommentOnly(rows, line, "pipeline/runner.py") {
			t.Errorf("observed docstring line %d must remain illustrative", line)
		}
	}
	if ground.LineLooksCommentOnly(rows, 15, "pipeline/runner.py") {
		t.Error("actual executable line after a closed docstring must not be comment-only")
	}
	// Same factual coordinates and anchor shape as the observed live submission.
	params := json.RawMessage(`{"items":[{"anchor_kind":"call","anchor_symbol":"resolve","evidence_kind":"relationship","line_start":15,"object":"resolve","predicate":"calls","scope":"line","source":"pipeline/runner.py","subject":"run_pipeline","summary":"run_pipeline calls resolve(kind) and receives the selected plugin instance"}]}`)
	res, err := (&tool.EmitEvidence{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("actual emit failed: %v %+v", err, res)
	}
	for _, item := range ctx.Mutable.EmittedEvidence() {
		if item.Source != "pipeline/runner.py" || item.LineStart != 15 || item.AnchorKind != coretypes.AnchorCall {
			continue
		}
		if item.LineEnd != 15 || item.Subject != "run_pipeline" || item.Object != "resolve" {
			t.Fatalf("source relation was relocated or rewritten: %+v", item)
		}
		if item.ContextRole == coretypes.EvidenceContextRoleIllustrativeOnly {
			t.Errorf("actual executable call after closed docstring was demoted: %s", res.Summary)
		}
		if item.GroundingStatus != coretypes.GroundingGrounded || item.GroundingTier != coretypes.TierLineText {
			t.Errorf("observed executable call must ground directly from the read source: %+v", item)
		}
		return
	}
	t.Fatalf("original call not preserved: %s", res.Summary)
}
