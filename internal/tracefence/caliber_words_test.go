package tracefence

// caliber_words_test.go — QH2-B Table ③c pins (§29.79 观察续档, 2026-07-15).
//
//  1. closed-set golden: the published caliber-word faces are a deliberate
//     closed set — any change is a wordface decision, not a drive-by;
//  2. display-face tie: every closed-set word appears in the causal-
//     projection emission source (answer_document_mutation_runtime_rcr.go),
//     so the constant can never drift from the face it mirrors;
//  3. never-published sweep: no production emission source in the trace
//     wordface packages prints a never-published near-synonym — the
//     premise that makes the answer-side arm A directly decidable.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCaliberWordFaces_ClosedSetGolden(t *testing.T) {
	want := []string{"全额", "折算", "下界", "原始", "计入", "单次最大"}
	if got := CaliberWordFacesZH(); !reflect.DeepEqual(got, want) {
		t.Fatalf("published caliber-word closed set changed (a wordface decision — update the QH2-B consumers deliberately): got %v want %v", got, want)
	}
	if got, want := CaliberWordNeverPublishedZH(), []string{"满额", "足额"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("never-published near-synonym list changed: got %v want %v", got, want)
	}
}

func TestCaliberWordFaces_DisplayFaceTie(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "tool", "answer_document_mutation_runtime_rcr.go"))
	if err != nil {
		t.Fatalf("read display emission source: %v", err)
	}
	for _, word := range CaliberWordFacesZH() {
		if !strings.Contains(string(src), word) {
			t.Fatalf("closed-set word %q no longer appears in the display emission face — the Table ③c mirror drifted", word)
		}
	}
}

// TestCaliberWordFaces_NeverPublishedSweep — no PRODUCTION string literal in
// the trace wordface packages carries a never-published near-synonym (AST
// literal scan: comments may legitimately DISCUSS the word; only a printable
// literal can publish it). Test files are exempt (they carry the disease
// forms as fixtures); the Table ③c definition site is exempt (the list
// itself is the single source).
func TestCaliberWordFaces_NeverPublishedSweep(t *testing.T) {
	fset := token.NewFileSet()
	// V1-1 (§40.25): the customer-facing sidecar binder (../analysis/
	// tracefinding) and the model-facing roster renderer (../agent) joined the
	// sweep — both print caliber words next to magnitudes.
	for _, dir := range []string{".", "../tool", "../tracequery", "../context", "../types", "../render", "../skill", "../tracediag", "../analysis/tracefinding", "../agent"} {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				filepath.Base(path) == "display_tables.go" {
				return err
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				for _, word := range CaliberWordNeverPublishedZH() {
					if strings.Contains(lit.Value, word) {
						t.Errorf("production string literal in %s carries never-published word %q — either it became a published face (remove it from the list) or it is a stray print", path, word)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("sweep %s: %v", dir, err)
		}
	}
}
