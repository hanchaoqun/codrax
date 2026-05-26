package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestRuntimeHintsDoNotExposeMidLoopMechanismLabel(t *testing.T) {
	for _, path := range []string{
		"explorer.go",
		"../tool/emit_evidence.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(src), "MID-LOOP CHECK:") {
			t.Fatalf("%s contains LLM-facing MID-LOOP CHECK label; use a neutral progress label instead", path)
		}
	}
}

func TestLLMFacingStringsAvoidRelationMorphologyOverfit(t *testing.T) {
	files := []string{
		"analyzer.go",
		"explorer.go",
		"extractor.go",
		"sub_explorer.go",
		"answer_document_evaluator.go",
		"capability_surface.go",
	}
	forbidden := []string{
		"same-named function",
		"same-name function",
		"same named function",
		"same-named file",
		"same-name file",
		"relation must be a function",
		"relations must be functions",
		"同名函数",
		"同名文件",
		"关系必须是函数",
	}
	assertStringLiteralsAvoid(t, files, forbidden)
}

func TestLLMFacingRepoLensBoundaryDoesNotScareAwayNavigationFacts(t *testing.T) {
	files := []string{
		"explorer.go",
		"extractor.go",
		"sub_explorer.go",
		"answer_document_evaluator.go",
		"../tool/source_inventory_reconcile.go",
		"../tool/repomap/render/render.go",
	}
	forbidden := []string{
		"not final-answer evidence",
		"not final answer evidence",
		"not repository evidence",
	}
	assertStringLiteralsAvoid(t, files, forbidden)
}

func assertStringLiteralsAvoid(t *testing.T, files, forbidden []string) {
	t.Helper()
	for _, path := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			lower := strings.ToLower(value)
			for _, term := range forbidden {
				needle := strings.ToLower(term)
				if strings.Contains(lower, needle) {
					pos := fset.Position(lit.Pos())
					t.Errorf("%s:%d LLM-facing string contains over-strong prompt term %q: %s",
						pos.Filename, pos.Line, term, promptHygienePreview(value, needle))
				}
			}
			return true
		})
	}
}

func promptHygienePreview(value, needle string) string {
	lower := strings.ToLower(value)
	idx := strings.Index(lower, needle)
	if idx < 0 {
		idx = 0
	}
	start := idx - 60
	if start < 0 {
		start = 0
	}
	end := idx + len(needle) + 60
	if end > len(value) {
		end = len(value)
	}
	return strings.ReplaceAll(value[start:end], "\n", " ")
}
