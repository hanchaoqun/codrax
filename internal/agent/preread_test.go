package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

func itoa(n int) string { return strconv.Itoa(n) }

func TestSelectAnchorsInFile_ExactBeatsSubstr(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {
				RelPath: "a.go",
				Symbols: []repomap.Symbol{
					{Name: "RegisterDefaultSubAgents", Kind: "function", Line: 50, EndLine: 70, Exported: true, File: "a.go"},
					{Name: "SubAgent", Kind: "type", Line: 10, EndLine: 12, Exported: true, File: "a.go"},
					{Name: "helper", Kind: "function", Line: 100, EndLine: 102, File: "a.go"},
				},
			},
		},
	}
	got := selectAnchorsInFile(graph, "a.go", []string{"SubAgent"}, 2)
	if len(got) == 0 {
		t.Fatal("expected at least one anchor")
	}
	// "SubAgent" exact-matches the second symbol; RegisterDefaultSubAgents
	// contains "subagent" (substring). The exact hit must win the #1 slot.
	if got[0].Sym.Name != "SubAgent" {
		t.Errorf("expected SubAgent top-ranked, got %s (score=%f)", got[0].Sym.Name, got[0].Score)
	}
	if got[0].Score <= got[1].Score {
		t.Errorf("exact match must beat substring: exact=%f substr=%f", got[0].Score, got[1].Score)
	}
}

func TestSelectAnchorsInFile_NoMatchReturnsNil(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {
				RelPath: "a.go",
				Symbols: []repomap.Symbol{
					{Name: "helper", Kind: "function", Line: 10, EndLine: 15, File: "a.go"},
				},
			},
		},
	}
	if got := selectAnchorsInFile(graph, "a.go", []string{"NoSuchThing"}, 2); got != nil {
		t.Errorf("expected nil for zero matches, got %+v", got)
	}
	if got := selectAnchorsInFile(nil, "a.go", []string{"X"}, 2); got != nil {
		t.Error("nil graph must return nil")
	}
	if got := selectAnchorsInFile(graph, "a.go", nil, 2); got != nil {
		t.Error("empty entities must return nil")
	}
}

func TestSelectAnchorsInFile_RespectsMaxAnchors(t *testing.T) {
	// Three symbols all match "Process" as substring. MaxAnchors=2
	// must cap.
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {
				RelPath: "a.go",
				Symbols: []repomap.Symbol{
					{Name: "ProcessRequest", Kind: "function", Line: 10, EndLine: 50, Exported: true, File: "a.go"},
					{Name: "ProcessReply", Kind: "function", Line: 60, EndLine: 100, Exported: true, File: "a.go"},
					{Name: "Process", Kind: "function", Line: 200, EndLine: 250, Exported: true, File: "a.go"},
				},
			},
		},
	}
	got := selectAnchorsInFile(graph, "a.go", []string{"Process"}, 2)
	if len(got) != 2 {
		t.Errorf("expected 2 anchors, got %d", len(got))
	}
	// Exact match "Process" must be top.
	if got[0].Sym.Name != "Process" {
		t.Errorf("expected Process top-ranked (exact), got %s", got[0].Sym.Name)
	}
}

func TestPlanAnchorWindows_SmallFileReturnsFullFile(t *testing.T) {
	got := planAnchorWindows(100, nil)
	if len(got) != 1 || got[0].Start != 1 || got[0].End != 100 {
		t.Errorf("small file: got %+v, want [{1, 100}]", got)
	}
	if got[0].Reason != "full file" {
		t.Errorf("reason: got %q, want 'full file'", got[0].Reason)
	}
}

func TestPlanAnchorWindows_NoAnchorsReturnsHeadSlice(t *testing.T) {
	got := planAnchorWindows(500, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 window, got %d", len(got))
	}
	if got[0].Start != 1 || got[0].End != preReadLineBudgetPerFile {
		t.Errorf("head window: got [%d, %d], want [1, %d]", got[0].Start, got[0].End, preReadLineBudgetPerFile)
	}
}

func TestPlanAnchorWindows_SingleAnchorHeadPlusWindow(t *testing.T) {
	anchors := []anchorSymbol{{
		Sym:   repomap.Symbol{Name: "Foo", Kind: "function", Line: 300, EndLine: 320},
		Score: 5,
	}}
	got := planAnchorWindows(500, anchors)
	if len(got) != 2 {
		t.Fatalf("expected 2 windows (head + anchor), got %d", len(got))
	}
	if got[0].Start != 1 || got[0].End != preReadHeadLinesSingle {
		t.Errorf("head: got [%d, %d], want [1, %d]", got[0].Start, got[0].End, preReadHeadLinesSingle)
	}
	if got[1].Start > 300 || got[1].End < 320 {
		t.Errorf("anchor window must cover [300, 320], got [%d, %d]", got[1].Start, got[1].End)
	}
	if !strings.Contains(got[1].Reason, "Foo") {
		t.Errorf("reason should mention Foo, got %q", got[1].Reason)
	}
}

func TestPlanAnchorWindows_MergesOverlapping(t *testing.T) {
	// Anchor symbol sits at line 35, context-before 8 → window starts
	// at 27, which overlaps the head window [1, 30]. Merged into one.
	anchors := []anchorSymbol{{
		Sym:   repomap.Symbol{Name: "EarlyFunc", Kind: "function", Line: 35, EndLine: 50},
		Score: 5,
	}}
	got := planAnchorWindows(500, anchors)
	if len(got) != 1 {
		t.Fatalf("expected 1 merged window, got %d: %+v", len(got), got)
	}
	if got[0].Start != 1 {
		t.Errorf("merged window must start at 1, got %d", got[0].Start)
	}
	if !strings.Contains(got[0].Reason, "head") || !strings.Contains(got[0].Reason, "EarlyFunc") {
		t.Errorf("merged reason should combine both: %q", got[0].Reason)
	}
}

func TestPlanAnchorWindows_CapsAnchorTail(t *testing.T) {
	// Anchor symbol with missing EndLine (0) and giant file → window
	// must be capped at preReadAnchorCapTail lines.
	anchors := []anchorSymbol{{
		Sym:   repomap.Symbol{Name: "Big", Kind: "function", Line: 500, EndLine: 0},
		Score: 5,
	}}
	got := planAnchorWindows(5000, anchors)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 windows, got %d", len(got))
	}
	anchorWin := got[len(got)-1]
	span := anchorWin.End - anchorWin.Start + 1
	if span > preReadAnchorCapTail {
		t.Errorf("anchor window span %d exceeds cap %d", span, preReadAnchorCapTail)
	}
}

func TestPlanAnchorWindows_RespectsBudget(t *testing.T) {
	// Two anchors far apart + multi-head → total must fit budget.
	anchors := []anchorSymbol{
		{Sym: repomap.Symbol{Name: "A", Kind: "function", Line: 200, EndLine: 350}, Score: 5},
		{Sym: repomap.Symbol{Name: "B", Kind: "function", Line: 800, EndLine: 950}, Score: 4},
	}
	got := planAnchorWindows(2000, anchors)
	total := 0
	for _, w := range got {
		total += w.End - w.Start + 1
	}
	if total > preReadLineBudgetPerFile {
		t.Errorf("total %d exceeds budget %d", total, preReadLineBudgetPerFile)
	}
}

func TestRenderPreReadFile_IncludesGutterAndGap(t *testing.T) {
	allLines := make([]string, 400)
	for i := range allLines {
		allLines[i] = "line" + itoa(i+1)
	}
	windows := []anchorWindow{
		{Start: 1, End: 30, Reason: "head"},
		{Start: 200, End: 220, Reason: "anchor `Foo` (function)"},
	}
	got := renderPreReadFile("a.go", allLines, windows)
	if !strings.Contains(got, "1\tline1") {
		t.Errorf("render missing head start line")
	}
	if !strings.Contains(got, "200\tline200") {
		t.Errorf("render missing anchor start line")
	}
	if !strings.Contains(got, "gap: lines 31-199") {
		t.Errorf("render missing gap marker")
	}
	if !strings.Contains(got, "tail: lines 221-400") {
		t.Errorf("render missing tail marker")
	}
	if !strings.Contains(got, "2 slices of 400 total lines") {
		t.Errorf("render missing slice-count banner")
	}
}

func TestSymbolGuidedPreRead_ProducesInjectionsWithRanges(t *testing.T) {
	// Build a 400-line temp Go file with a function around line 100.
	dir := t.TempDir()
	var sb strings.Builder
	sb.WriteString("package foo\n\n")
	for i := 0; i < 97; i++ {
		sb.WriteString("// filler line " + itoa(i) + "\n")
	}
	sb.WriteString("func TargetSymbol() int {\n")
	sb.WriteString("\treturn 1\n")
	sb.WriteString("}\n")
	for i := 0; i < 300; i++ {
		sb.WriteString("// tail filler " + itoa(i) + "\n")
	}
	filePath := filepath.Join(dir, "target.go")
	if err := os.WriteFile(filePath, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Graph must know about target.go + TargetSymbol line 100.
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"target.go": {
				RelPath: "target.go",
				Symbols: []repomap.Symbol{
					{Name: "TargetSymbol", Kind: "function", Line: 100, EndLine: 102, Exported: true, File: "target.go"},
				},
			},
		},
	}
	rendered, injs := symbolGuidedPreRead(dir, []string{"target.go"}, graph, []string{"TargetSymbol"}, 1)
	if rendered == "" {
		t.Fatal("expected non-empty render")
	}
	if len(injs) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(injs))
	}
	if injs[0].Path != "target.go" {
		t.Errorf("path: %q", injs[0].Path)
	}
	// Anchor line 100 must sit inside some range.
	covered := false
	for _, r := range injs[0].Ranges {
		if r.Start <= 100 && r.End >= 100 {
			covered = true
		}
	}
	if !covered {
		t.Errorf("anchor line 100 not covered by any range: %+v", injs[0].Ranges)
	}
}

func TestSymbolGuidedPreRead_DegradeWhenGraphNil(t *testing.T) {
	rendered, injs := symbolGuidedPreRead("somewhere", []string{"a.go"}, nil, []string{"X"}, 1)
	if rendered != "" || injs != nil {
		t.Errorf("nil graph must return empty result")
	}
}

// TestSymbolGuidedPreRead_EmptyEntitiesDegradesToHeadOnly: when the
// analyzer left Entities empty, the strategy still injects head-only
// slices — same content the old preReadRequiredFiles would produce,
// so the caller sees identical behaviour to the pre-Commit-B fallback
// path. The test build a large file to force head-slice truncation
// and checks the window covers [1, preReadLineBudgetPerFile].
func TestSymbolGuidedPreRead_EmptyEntitiesDegradesToHeadOnly(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("line " + itoa(i) + "\n")
	}
	fp := filepath.Join(dir, "a.go")
	if err := os.WriteFile(fp, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	graph := &repomap.Graph{}
	rendered, injs := symbolGuidedPreRead(dir, []string{"a.go"}, graph, nil, 1)
	if rendered == "" {
		t.Fatal("empty entities should still produce head-only output")
	}
	if len(injs) != 1 || len(injs[0].Ranges) != 1 {
		t.Fatalf("expected single head range, got %+v", injs)
	}
	if injs[0].Ranges[0].Start != 1 || injs[0].Ranges[0].End != preReadLineBudgetPerFile {
		t.Errorf("head range: got %+v, want [1, %d]", injs[0].Ranges[0], preReadLineBudgetPerFile)
	}
}

func TestDedupeAnchorFilesWithLines_KeepsFirstNonZeroLine(t *testing.T) {
	files, lines := dedupeAnchorFilesWithLines(
		anchorFileLine{File: "a.go", Line: 0},
		anchorFileLine{File: "a.go", Line: 42},
		anchorFileLine{File: "b.go", Line: 10},
		anchorFileLine{File: "a.go", Line: 99}, // dup — ignored
	)
	wantFiles := []string{"a.go", "b.go"}
	wantLines := []int{42, 10}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Errorf("files: got %v, want %v", files, wantFiles)
	}
	if !reflect.DeepEqual(lines, wantLines) {
		t.Errorf("lines: got %v, want %v", lines, wantLines)
	}
}
