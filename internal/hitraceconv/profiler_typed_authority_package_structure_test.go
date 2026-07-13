package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProfilerTypedIssueAuthorityIsPackageWideAndOneWay freezes the B2-b5
// boundary across every production file in this package.  In particular, it
// must not regress to the former text -> typed-issue bridge merely by moving
// that bridge out of one of the renderer files covered by the lane-specific
// structure tests.
func TestProfilerTypedIssueAuthorityIsPackageWideAndOneWay(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read hitraceconv package: %v", err)
	}

	fset := token.NewFileSet()
	productionFiles := 0
	typedEntries := 0
	sawIssueKindDeclaration := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		productionFiles++
		path := filepath.Clean(entry.Name())
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse production source %s: %v", path, parseErr)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			switch item := node.(type) {
			case *ast.Ident:
				switch item.Name {
				case "profilerFtraceEventIssueFromLegacy",
					"profilerFtraceEventParameterizedToken",
					"protoScalarUint",
					"protoScalarString",
					"protoScalarState":
					t.Errorf("production typed authority restored forbidden identifier %q at %s", item.Name, fset.Position(item.Pos()))
				}
			case *ast.MapType:
				if profilerTypedAuthorityIsIdent(item.Key, "string") &&
					profilerTypedAuthorityIsIdent(item.Value, "profilerFtraceEventIssueKind") {
					t.Errorf("production restored string -> profiler issue-kind map at %s", fset.Position(item.Pos()))
				}
			}
			return true
		})

		for _, declaration := range file.Decls {
			switch item := declaration.(type) {
			case *ast.GenDecl:
				for _, spec := range item.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if ok && typeSpec.Name.Name == "profilerFtraceEventIssueKind" {
						sawIssueKindDeclaration = true
					}
				}
			case *ast.FuncDecl:
				if profilerTypedAuthorityAcceptsText(item.Type) && profilerTypedAuthorityReturnsIssue(item.Type) {
					t.Errorf("text-consuming function %s at %s can mint profiler typed issue authority", item.Name.Name, fset.Position(item.Pos()))
				}
				if item.Body == nil || !strings.Contains(item.Name.Name, "WithTypedAudit") {
					continue
				}
				typedEntries++
				ast.Inspect(item.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					callee := profilerTypedAuthorityCalleeName(call.Fun)
					if strings.HasSuffix(callee, "WithAudit") && !strings.Contains(callee, "WithTypedAudit") {
						t.Errorf("typed entry %s calls compatibility audit %s at %s", item.Name.Name, callee, fset.Position(call.Pos()))
					}
					return true
				})
			}
		}
	}

	if productionFiles == 0 || !sawIssueKindDeclaration || typedEntries == 0 {
		t.Fatalf("package-wide typed-authority scan was incomplete: production_files=%d issue_kind=%t typed_entries=%d",
			productionFiles, sawIssueKindDeclaration, typedEntries)
	}
}

func profilerTypedAuthorityIsIdent(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func profilerTypedAuthorityAcceptsText(function *ast.FuncType) bool {
	return profilerTypedAuthorityFieldListContains(function.Params, "string")
}

func profilerTypedAuthorityReturnsIssue(function *ast.FuncType) bool {
	return profilerTypedAuthorityFieldListContains(function.Results, "profilerFtraceEventIssue") ||
		profilerTypedAuthorityFieldListContains(function.Results, "profilerFtraceEventIssueKind")
}

func profilerTypedAuthorityFieldListContains(fields *ast.FieldList, identifier string) bool {
	if fields == nil {
		return false
	}
	found := false
	for _, field := range fields.List {
		ast.Inspect(field.Type, func(node ast.Node) bool {
			name, ok := node.(*ast.Ident)
			if ok && name.Name == identifier {
				found = true
				return false
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func profilerTypedAuthorityCalleeName(expression ast.Expr) string {
	switch callee := expression.(type) {
	case *ast.Ident:
		return callee.Name
	case *ast.SelectorExpr:
		return callee.Sel.Name
	default:
		return ""
	}
}
