package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var retiredModelConclusionProviders = map[string]bool{
	"runProseLexiconBoardCheck":            true,
	"proseLexiconBoardResidualFindings":    true,
	"proseScalarResidualAppendixInputs":    true,
	"proseWallClockConservationFindings":   true,
	"proseHeadlineElimFindings":            true,
	"proseFactJuxtapositionFindings":       true,
	"proseFactJuxtapositionFindingsMode":   true,
	"proseFactEquationFindings":            true,
	"proseFactImplicitSubtractionFindings": true,
}

func functionCallGraphFromSources(t *testing.T, sources map[string]string) map[string]map[string]bool {
	t.Helper()
	graph := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for name, source := range sources {
		file, err := parser.ParseFile(fset, name, source, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if graph[fn.Name.Name] == nil {
				graph[fn.Name.Name] = map[string]bool{}
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := ""
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					callee = fun.Name
				case *ast.SelectorExpr:
					callee = fun.Sel.Name
				}
				if callee != "" {
					graph[fn.Name.Name][callee] = true
				}
				return true
			})
		}
	}
	return graph
}

func reachableRetiredProvider(graph map[string]map[string]bool, roots ...string) (string, bool) {
	seen := map[string]bool{}
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		for callee := range graph[name] {
			if retiredModelConclusionProviders[callee] {
				return callee, true
			}
			if !seen[callee] {
				queue = append(queue, callee)
			}
		}
	}
	return "", false
}

func TestModelConclusionProvidersUnreachableThroughWrappers(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources[path] = string(raw)
	}
	graph := functionCallGraphFromSources(t, sources)
	if provider, ok := reachableRetiredProvider(graph,
		"runContractCheck", "collectSystemCrossCheckFindings", "attachSystemCrossCheckAppendix"); ok {
		t.Fatalf("a production shipping/check root reaches retired model-conclusion provider %s through one or more wrappers", provider)
	}

	// Mutation pin for S4-4: the graph walk, unlike the retired direct-string
	// scan, must catch a provider hidden behind an otherwise innocent wrapper.
	fixture := functionCallGraphFromSources(t, map[string]string{"fixture.go": `package fixture
func collectSystemCrossCheckFindings(){ neutralWrapper() }
func neutralWrapper(){ proseHeadlineElimFindings() }
func proseHeadlineElimFindings(){}
`})
	if provider, ok := reachableRetiredProvider(fixture, "collectSystemCrossCheckFindings"); !ok || provider != "proseHeadlineElimFindings" {
		t.Fatalf("wrapper-bypass mutation was not caught: provider=%q ok=%v graph=%+v", provider, ok, fixture)
	}
}
