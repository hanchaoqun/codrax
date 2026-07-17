package tracequery

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"testing"
)

// TestPriorityPointHardConsumerCallgraphPinned is the mechanical half of the
// priority-proof red line. Nearest-sample helpers remain available to legacy
// display tests, but production code has no call into them. The two raw
// numeric relation helpers may only be reached from functions that have just
// checked priorityPointVerdict hard evidence (or are themselves methods of the
// authority). Any new call site requires an explicit proof review and golden
// update instead of silently reopening a nearest/bare-int hard lane.
func TestPriorityPointHardConsumerCallgraphPinned(t *testing.T) {
	want := map[string][]string{
		"priorityNear":       nil,
		"threadPriorityNear": {"query.go:priorityNear"},
		"priorityRelation":   {"priority_point.go:wakeupRelationAtPoint"},
		"dependencyPriorityRelation": {
			"priority_point.go:dependencyRelationAtPoint",
			"priority_point.go:lowerPriorityRelationSlices",
			"query.go:applyRunnableTopPriorityInversionScopes",
		},
	}
	got := make(map[string][]string, len(want))
	fset := token.NewFileSet()
	for _, filename := range []string{"priority_point.go", "query.go"} {
		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch callee := call.Fun.(type) {
				case *ast.Ident:
					name = callee.Name
				case *ast.SelectorExpr:
					name = callee.Sel.Name
				}
				if _, governed := want[name]; governed {
					got[name] = append(got[name], filepath.Base(filename)+":"+fn.Name.Name)
				}
				return true
			})
		}
	}
	for name := range want {
		if !reflect.DeepEqual(got[name], want[name]) {
			t.Fatalf("priority hard-consumer callgraph drift for %s: got=%v want=%v; review every new caller for exact_at_point/closed_range_stable admission", name, got[name], want[name])
		}
	}
}

// TestPriorityPointAuthorityLookupComplexityPinned protects the resource
// contract mechanically. The runtime scale fixture checks behavior on a
// large ledger; this AST pin prevents a future edit from restoring a linear
// PID-ledger lookup or the former target-range x dependency-range cross join.
func TestPriorityPointAuthorityLookupComplexityPinned(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "priority_point.go", nil, 0)
	if err != nil {
		t.Fatalf("parse priority_point.go: %v", err)
	}
	forbidden := map[string]map[string]bool{
		"pointVerdictAt":              {"endpointsByPID": true, "stableByPID": true},
		"advisoryNearest":             {"endpointsByPID": true, "stableByPID": true},
		"rangeVerdict":                {"endpointsByPID": true, "stableByPID": true},
		"lowerPriorityRelationSlices": {"targetRanges": true, "dependencyRanges": true},
	}
	wantSearchCalls := map[string]int{
		"priorityEndpointSourceBounds":            2,
		"priorityEndpointAtPoint":                 1,
		"priorityStableRangeAtPoint":              1,
		"advisoryNearest":                         1,
		"lowerPriorityRelationSlices":             1,
		"priorityLowerRelationWindowSourceBounds": 2,
	}
	foundFunctions := make(map[string]bool)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		if forbidden[name] == nil && wantSearchCalls[name] == 0 {
			continue
		}
		foundFunctions[name] = true
		searchCalls := 0
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.RangeStmt:
				collection := priorityRangeCollectionName(value.X)
				if forbidden[name][collection] {
					t.Errorf("%s restored a linear range over %s; use the immutable binary index", name, collection)
				}
			case *ast.CallExpr:
				selector, ok := value.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "Search" {
					if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "sort" {
						searchCalls++
					}
				}
			}
			return true
		})
		if want := wantSearchCalls[name]; want > 0 && searchCalls < want {
			t.Errorf("%s has %d sort.Search call(s), want at least %d", name, searchCalls, want)
		}
	}
	for name := range forbidden {
		if !foundFunctions[name] {
			t.Errorf("governed lookup %s is absent", name)
		}
	}
	for name := range wantSearchCalls {
		if !foundFunctions[name] {
			t.Errorf("governed binary helper %s is absent", name)
		}
	}
}

func priorityRangeCollectionName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return priorityRangeCollectionName(value.X)
	default:
		return ""
	}
}
