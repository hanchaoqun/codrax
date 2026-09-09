package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	coretypes "github.com/hanchaoqun/codrax/internal/types"
)

// The model cited an annotation line for a definition. A successful read-backed
// relocation must keep the resulting reference a point; it does not acquire
// proof of the annotation's route or of the model's free-form explanation.
func TestB1629ActualReadThenEmitDefinitionPointRelocation(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "eval", "fixtures", "java-annotation-router"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                            string
		annotationLine, declarationLine int
	}{
		{"EchoHandler", 7, 8},
		{"UpperHandler", 9, 10},
		{"StatsHandler", 13, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := "src/main/java/app/handlers/" + tc.name + ".java"
			path := filepath.Join(repo, filepath.FromSlash(file))
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			entries := []FileEntry{{RelPath: file, AbsPath: path, Language: repomaptypes.LangJava, Size: stat.Size()}}
			ctx := &coretypes.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: coretypes.NewMutableState("list implementations")}
			ctx.Mutable.SetSearchGraph(BuildGraph(repo, ParseFiles(entries, repo)))
			readParams, _ := json.Marshal(map[string]any{"path": file})
			read, err := (&tool.ReadFile{}).Execute(ctx, readParams)
			if err != nil || !read.Success {
				t.Fatalf("actual read failed: %v %+v", err, read)
			}
			artifact, err := filepath.Rel(ctx.WorkDir, read.RawRef)
			if err != nil || read.RawRef == "" || artifact == ".." || strings.HasPrefix(artifact, ".."+string(filepath.Separator)) || filepath.IsAbs(artifact) {
				t.Fatalf("read output escaped test work dir: %q relative=%q err=%v", read.RawRef, artifact, err)
			}
			ctx.ToolResults = append(ctx.ToolResults, read)
			summary := tc.name + " declaration"
			params, _ := json.Marshal(map[string]any{"items": []map[string]any{{
				"evidence_kind": "direct", "subject": tc.name, "summary": summary,
				"scope": "line", "source": file, "line_start": tc.annotationLine,
				"anchor_kind": "definition", "anchor_symbol": tc.name,
			}}})
			res, err := (&tool.EmitEvidence{}).Execute(ctx, params)
			if err != nil || !res.Success {
				t.Fatalf("actual emit failed: %v %+v", err, res)
			}
			var submitted []coretypes.EvidenceItem
			for _, candidate := range ctx.Mutable.EmittedEvidence() {
				if candidate.Producer == tool.EmitEvidenceProducer {
					submitted = append(submitted, candidate)
				}
			}
			if len(submitted) != 1 {
				t.Fatalf("wanted one original submitted evidence item, got %+v", submitted)
			}
			item := submitted[0]
			if item.LineStart != tc.declarationLine || item.LineEnd != tc.declarationLine {
				t.Errorf("relocated point = %d-%d; want %d-%d, not stale annotation endpoint %d", item.LineStart, item.LineEnd, tc.declarationLine, tc.declarationLine, tc.annotationLine)
			}
			if item.Source != file || item.Scope != coretypes.ScopeLine || item.AnchorKind != coretypes.AnchorDefinition || item.AnchorSymbol != tc.name || item.Summary != summary || item.Subject != tc.name {
				t.Errorf("source role or model content changed during coordinate repair: %+v", item)
			}
			if item.GroundingStatus != coretypes.GroundingGrounded || item.GroundingTier != coretypes.TierLineText {
				t.Errorf("observed definition should remain read-backed: %+v", item)
			}
			if !strings.Contains(item.Snippet, "class "+tc.name) || strings.Contains(item.Snippet, "@Route") {
				t.Errorf("point proof expanded into neighboring annotation: %q", item.Snippet)
			}
		})
	}
}
