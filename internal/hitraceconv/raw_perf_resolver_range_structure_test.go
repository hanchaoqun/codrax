package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestRawPerfResolverRangeArithmeticHasSingleCheckedAuthority(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "raw_perfdata.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	functions := make(map[string]*ast.FuncDecl)
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name != nil {
			functions[function.Name.Name] = function
		}
	}
	require := func(name string) *ast.FuncDecl {
		t.Helper()
		function := functions[name]
		if function == nil || function.Body == nil {
			t.Fatalf("resolver arithmetic authority %s is missing", name)
		}
		return function
	}

	containment := require("rawPerfMappingContainment")
	bestMapping := require("rawPerfBestMapping")
	translation := require("rawPerfMappedVirtualAddress")
	symbolLookup := require("rawPerfSymbolForIP")
	resolveFrame := require("rawPerfResolveFrame")

	if got := rawPerfResolverCallCount(bestMapping.Body, "rawPerfMappingContainment"); got != 1 {
		t.Fatalf("best-mapping containment authority calls=%d want=1", got)
	}
	if got := rawPerfResolverCallCount(translation.Body, "rawPerfMappingContainment"); got != 1 {
		t.Fatalf("mapped-vaddr containment authority calls=%d want=1", got)
	}
	if got := rawPerfResolverCallCount(translation.Body, "rawPerfCheckedAddUint64"); got != 2 {
		t.Fatalf("mapped-vaddr checked additions=%d want=2", got)
	}
	if got := rawPerfResolverCallCount(symbolLookup.Body, "rawPerfMappedVirtualAddress"); got != 1 {
		t.Fatalf("symbol lookup translation authority calls=%d want=1", got)
	}
	if got := rawPerfResolverCallCount(resolveFrame.Body, "rawPerfBestMapping"); got != 1 {
		t.Fatalf("frame best-mapping authority calls=%d want=1", got)
	}
	if got := rawPerfResolverCallCount(resolveFrame.Body, "rawPerfSymbolForIP"); got != 1 {
		t.Fatalf("frame symbol authority calls=%d want=1", got)
	}

	for name, function := range map[string]*ast.FuncDecl{
		"rawPerfMappingContainment":   containment,
		"rawPerfBestMapping":          bestMapping,
		"rawPerfMappedVirtualAddress": translation,
		"rawPerfSymbolForIP":          symbolLookup,
	} {
		ast.Inspect(function.Body, func(node ast.Node) bool {
			binary, ok := node.(*ast.BinaryExpr)
			if !ok || binary.Op != token.ADD {
				return true
			}
			if rawPerfResolverExpressionReadsRangeField(binary) {
				t.Errorf("%s restored unchecked range-field addition at %s", name, fset.Position(binary.Pos()))
			}
			return true
		})
	}
}

func rawPerfResolverCallCount(node ast.Node, name string) int {
	count := 0
	ast.Inspect(node, func(candidate ast.Node) bool {
		call, ok := candidate.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := call.Fun.(*ast.Ident)
		if ok && identifier.Name == name {
			count++
		}
		return true
	})
	return count
}

func rawPerfResolverExpressionReadsRangeField(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		selector, ok := candidate.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil {
			return true
		}
		base, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch {
		case base.Name == "mapping" && (selector.Sel.Name == "Addr" || selector.Sel.Name == "Len" || selector.Sel.Name == "Pgoff"):
			found = true
		case base.Name == "file" && (selector.Sel.Name == "TextExecVaddr" || selector.Sel.Name == "TextExecVaddrFileOffset"):
			found = true
		}
		return !found
	})
	return found
}
