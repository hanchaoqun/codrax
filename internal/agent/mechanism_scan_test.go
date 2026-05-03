package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// writeTempFile creates a file under a temp dir and returns its path
// (relative to the dir) plus the absolute root. The relative form is
// what scanMechanismEvidence expects in graph symbols.
func writeTempFile(t *testing.T, dir, relPath, content string) string {
	t.Helper()
	abs := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return relPath
}

// makeGraphWithSymbol builds a minimal repomap.Graph that contains the
// given symbol. Used to drive scanMechanismEvidence without spinning up
// the full keyword search pipeline.
func makeGraphWithSymbol(file string, sym repomap.Symbol) *repomap.Graph {
	fi := &repomap.FileInfo{
		RelPath: file,
		Symbols: []repomap.Symbol{sym},
	}
	g := &repomap.Graph{
		SymbolDefs: make(map[string][]*repomap.Symbol),
		FileIndex:  map[string]*repomap.FileInfo{file: fi},
		Files:      []*repomap.FileInfo{fi},
	}
	g.SymbolDefs[sym.Name] = []*repomap.Symbol{&fi.Symbols[0]}
	return g
}

func TestScanMechanismEvidence_NoMechanismReqs(t *testing.T) {
	// Empty req list, non-mechanism reqs → returns nil (no work).
	out := scanMechanismEvidence(nil, &repomap.Graph{}, "/tmp")
	if out != nil {
		t.Errorf("nil reqs: want nil, got %v", out)
	}
	out = scanMechanismEvidence([]EvidenceRequirement{
		{Kind: "return_value", Entities: []string{"X"}},
	}, &repomap.Graph{}, "/tmp")
	if out != nil {
		t.Errorf("non-mechanism reqs: want nil, got %v", out)
	}
}

func TestScanMechanismEvidence_NilGraph(t *testing.T) {
	out := scanMechanismEvidence([]EvidenceRequirement{
		{Kind: "mechanism", Entities: []string{"X"}},
	}, nil, "/tmp")
	if out != nil {
		t.Errorf("nil graph: want nil, got %v", out)
	}
}

func TestScanMechanismEvidence_ProducesItemsForLongFunction(t *testing.T) {
	// Build a fake source file with a long function containing three
	// section-comment delimited blocks. Verify that scanMechanismEvidence
	// returns three EvidenceMechanism items, one per block, with the
	// expected predicate and subject.
	dir := t.TempDir()
	source := `package fake

// ContinuationPrompt drives the explorer's continuation logic.
func ContinuationPrompt() string {
	// Function-boundary read guidance.
	// Detects partial reads and pushes a finish-the-function hint.
	if hasPartial {
		return "finish reading"
	}

	// Enumeration completeness push.
	// Fires when discovered set isn't fully read.
	if !covered {
		return "read more files"
	}

	// Idle termination.
	// Two consecutive idle rounds end the loop.
	if idleStreak > 1 {
		return ""
	}

	return "default"
}
`
	rel := writeTempFile(t, dir, "fake.go", source)

	graph := makeGraphWithSymbol(rel, repomap.Symbol{
		Name:    "ContinuationPrompt",
		Kind:    "function",
		File:    rel,
		Line:    4,  // 1-based: "func ContinuationPrompt..."
		EndLine: 24, // closing brace
	})
	// Phase 6 stage 18 (2026-05-03) — the typed-only contract
	// requires LineFeatures to drive extractDecisionBlocks. Real
	// production runs get this from repomap's tree-sitter parse;
	// this test fakes the relevant return-statement lines so the
	// typed contract sees four block terminators (lines 8, 13, 18,
	// 23 in `source` — 1-based), matching the four `return …`
	// statements in the fake function body.
	if fi, ok := graph.FileIndex[rel]; ok && fi != nil {
		fi.LineFeatures = map[int][]repomap.LineFeature{
			8:  {repomap.LineFeatureReturnStmt},
			13: {repomap.LineFeatureReturnStmt},
			18: {repomap.LineFeatureReturnStmt},
			23: {repomap.LineFeatureReturnStmt},
		}
	}

	reqs := []EvidenceRequirement{
		{Kind: "mechanism", Entities: []string{"ContinuationPrompt"}},
	}
	out := scanMechanismEvidence(reqs, graph, dir)
	if len(out) == 0 {
		t.Fatalf("expected ≥1 evidence item, got 0")
	}
	for _, ev := range out {
		if ev.Kind != types.EvidenceMechanism {
			t.Errorf("evidence Kind = %s, want %s", ev.Kind, types.EvidenceMechanism)
		}
		if ev.Predicate != "step" {
			t.Errorf("evidence Predicate = %s, want step", ev.Predicate)
		}
		if ev.Subject != "ContinuationPrompt" {
			t.Errorf("evidence Subject = %s, want ContinuationPrompt", ev.Subject)
		}
		if ev.Object == "" {
			t.Errorf("evidence Object empty (block label missing)")
		}
		if ev.LineStart < 4 || ev.LineEnd > 24 {
			t.Errorf("evidence line range [%d,%d] outside function body [4,24]",
				ev.LineStart, ev.LineEnd)
		}
	}
	t.Logf("produced %d mechanism evidence items", len(out))
}

func TestScanMechanismEvidence_SkipsTrivialFunction(t *testing.T) {
	// Functions shorter than mechanismMinBodyLines must be skipped —
	// they have no internal structure worth extracting.
	dir := t.TempDir()
	source := `package fake

func Trivial() bool {
	return true
}
`
	rel := writeTempFile(t, dir, "fake.go", source)
	graph := makeGraphWithSymbol(rel, repomap.Symbol{
		Name:    "Trivial",
		Kind:    "function",
		File:    rel,
		Line:    3,
		EndLine: 5,
	})
	out := scanMechanismEvidence([]EvidenceRequirement{
		{Kind: "mechanism", Entities: []string{"Trivial"}},
	}, graph, dir)
	if out != nil {
		t.Errorf("trivial function: expected nil (skipped), got %d items", len(out))
	}
}

func TestSelectMechanismFunctions_DirectMatch(t *testing.T) {
	graph := makeGraphWithSymbol("foo.go", repomap.Symbol{
		Name:    "ContinuationPrompt",
		Kind:    "function",
		File:    "foo.go",
		Line:    1,
		EndLine: 30,
	})
	got := selectMechanismFunctions(graph, "ContinuationPrompt")
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
	if got[0].Name != "ContinuationPrompt" {
		t.Errorf("matched wrong symbol: %s", got[0].Name)
	}
}

func TestSelectMechanismFunctions_NoMatch(t *testing.T) {
	graph := makeGraphWithSymbol("foo.go", repomap.Symbol{
		Name:    "Foo",
		Kind:    "function",
		File:    "foo.go",
		Line:    1,
		EndLine: 30,
	})
	got := selectMechanismFunctions(graph, "Unrelated")
	if got != nil {
		t.Errorf("expected nil for unrelated entity, got %v", got)
	}
}
