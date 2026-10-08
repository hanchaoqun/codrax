package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The new static submit site is allowed to submit a prepared candidate instead
// of an inline literal only through this complete, single typed admission path.
func TestTraceDBStaticInitializeSourceAuthorityIsStructurallyPinned(t *testing.T) {
	source, err := os.ReadFile("streamerdb_static_initialize.go")
	if err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "static.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]map[string]int{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		calls[fn.Name.Name] = map[string]int{}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch c := call.Fun.(type) {
			case *ast.Ident:
				name = c.Name
			case *ast.SelectorExpr:
				name = c.Sel.Name
			}
			calls[fn.Name.Name][name]++
			switch name {
			case "traceDBCPUAt", "traceDBKnownCPUAt", "traceDBAnyText", "traceDBAnyInt64", "traceDBResolvedProcessLineContext", "poisonGlobally", "poisonExactLane", "fenceExactLane":
				t.Fatalf("static source bypass or unscoped rejection %s", name)
			}
			return true
		})
	}
	for fn, wants := range map[string]map[string]int{
		"exportTraceDBStaticInitialize":     {"prepareTraceDBStaticInitializeRow": 1, "submit": 1, "traceDBBoundedSQLiteIntegerTransport": 4, "Scan": 1},
		"prepareTraceDBStaticInitializeRow": {"traceDBStaticInitializeSubject": 1, "threadPointAllows": 1, "threadClosedEndpointAllows": 1, "processClosedEndpointAllows": 1, "lookupCPUAt": 2},
		"traceDBStaticInitializeSubject":    {"resolveThreadSubject": 1, "traceDBStrictInternalID": 1, "traceDBStrictPublicID": 1},
	} {
		for name, want := range wants {
			if calls[fn][name] != want {
				t.Fatalf("%s calls %s=%d want=%d", fn, name, calls[fn][name], want)
			}
		}
	}
	if strings.Contains(string(source), "FROM static_initalize WHERE") {
		t.Fatal("source scan silently filters invalid rows")
	}
}
