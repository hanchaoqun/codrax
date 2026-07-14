package hitraceconv

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

type profilerCompactStorageFieldPin struct {
	name     string
	typeName string
	tag      string
}

func TestProfilerCompactStorageHasOneStaticAuthority(t *testing.T) {
	forbiddenIdentifiers := map[string]bool{
		"profilerPairRowMapping":                         true,
		"pairRowMappings":                                true,
		"pairRowCapacity":                                true,
		"validatePreparedProfilerPairStorage":            true,
		"profilerSourceOrderDispositionWithLegacyParity": true,
	}
	wantStructs := map[string][]profilerCompactStorageFieldPin{
		"traceDBStoredRow": {
			{name: "tsNS", typeName: "uint64"},
			{name: "seq", typeName: "int"},
			{name: "line", typeName: "string"},
			{name: "provenance", typeName: "profilerPairRowProvenance"},
		},
		"traceDBChunkRow": {
			{name: "Line", typeName: "string", tag: "`json:\"line\"`"},
			{name: "TSNS", typeName: "uint64", tag: "`json:\"ts_ns\"`"},
			{name: "IngestOrdinal", typeName: "uint64", tag: "`json:\"ingest_ordinal\"`"},
			{name: "Seq", typeName: "int", tag: "`json:\"seq\"`"},
			{name: "ProfilerProvenance", typeName: "profilerPairRowProvenance", tag: "`json:\"p\"`"},
		},
		"traceDBChunkWireRow": {
			{name: "Line", typeName: "*string", tag: "`json:\"line\"`"},
			{name: "TSNS", typeName: "*uint64", tag: "`json:\"ts_ns\"`"},
			{name: "Seq", typeName: "*int", tag: "`json:\"seq\"`"},
			{name: "IngestOrdinal", typeName: "*uint64", tag: "`json:\"ingest_ordinal\"`"},
			{name: "ProfilerProvenance", typeName: "*profilerPairRowProvenance", tag: "`json:\"p\"`"},
		},
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	fset := token.NewFileSet()
	definitions := make(map[string]int, len(wantStructs))
	sidecarBuilders := 0
	preflightAuthenticators := 0
	activeVerdictBranches := 0
	sqlExportFunctions := 0
	sqlOrdinaryConstructors := 0
	sqlGenericConstructors := 0
	payloadCardinalityNames := map[string]bool{
		"RowsAccepted": true, "expectedCount": true, "rowCount": true, "acceptedRows": true,
	}
	containsPayloadCardinality := func(expression ast.Expr) bool {
		found := false
		ast.Inspect(expression, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.Ident:
				found = found || payloadCardinalityNames[typed.Name]
			case *ast.SelectorExpr:
				found = found || payloadCardinalityNames[typed.Sel.Name]
			}
			return !found
		})
		return found
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse production source %s: %v", name, parseErr)
		}
		if name == "profiler_source_order_sidecar.go" || name == "profiler_source_order_proof.go" {
			ast.Inspect(file, func(node ast.Node) bool {
				switch expression := node.(type) {
				case *ast.MapType:
					t.Errorf("fixed source-order authority %s regained dynamic map storage at %s",
						name, fset.Position(expression.Pos()))
				case *ast.CallExpr:
					callee, ok := expression.Fun.(*ast.Ident)
					if ok && callee.Name == "make" {
						t.Errorf("fixed source-order authority %s regained dynamic make storage at %s",
							name, fset.Position(expression.Pos()))
					}
				}
				return true
			})
		}
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && forbiddenIdentifiers[identifier.Name] {
				t.Errorf("production source %s reintroduced retired B-d1 identifier %q", name, identifier.Name)
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee, ok := call.Fun.(*ast.Ident)
			if !ok || callee.Name != "make" {
				return true
			}
			for _, argument := range call.Args[1:] {
				if containsPayloadCardinality(argument) {
					t.Errorf("production source %s allocated make storage from row cardinality at %s",
						name, fset.Position(call.Pos()))
				}
			}
			return true
		})
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok {
						continue
					}
					wantFields, pinned := wantStructs[typeSpec.Name.Name]
					if !pinned {
						continue
					}
					definitions[typeSpec.Name.Name]++
					structure, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						t.Errorf("%s in %s is no longer a struct", typeSpec.Name.Name, name)
						continue
					}
					gotFields := profilerCompactStorageStructFields(t, fset, typeSpec.Name.Name, structure)
					if !profilerCompactStorageFieldsEqual(gotFields, wantFields) {
						t.Errorf("%s in %s escaped compact storage contract:\n got: %+v\nwant: %+v",
							typeSpec.Name.Name, name, gotFields, wantFields)
					}
				}
			case *ast.FuncDecl:
				if typed.Name.Name == "exportTraceDBToSystraceWithLedger" {
					sqlExportFunctions++
					ast.Inspect(typed.Body, func(node ast.Node) bool {
						call, ok := node.(*ast.CallExpr)
						if !ok {
							return true
						}
						callee, ok := call.Fun.(*ast.Ident)
						if !ok {
							return true
						}
						switch callee.Name {
						case "newTraceDBInactiveOrdinaryRowSink":
							sqlOrdinaryConstructors++
						case "newTraceDBRowSink", "newTraceDBRowSinkWithOptions":
							sqlGenericConstructors++
						}
						return true
					})
				}
				if typed.Name.Name == "authenticatePreparedFinalRun" {
					preflightAuthenticators++
					allowedCalls := map[string]bool{
						"len": true, "uint64": true, "traceDBRunInputIntegrity": true,
						"openAuthenticatedRunReader": true, "next": true,
						"traceDBJoinPreservingSingle": true, "close": true,
					}
					ast.Inspect(typed.Body, func(node ast.Node) bool {
						switch expression := node.(type) {
						case *ast.MapType:
							t.Errorf("inactive final-run preflight regained dynamic map at %s",
								fset.Position(expression.Pos()))
						case *ast.CallExpr:
							callee := ""
							switch function := expression.Fun.(type) {
							case *ast.Ident:
								callee = function.Name
							case *ast.SelectorExpr:
								callee = function.Sel.Name
							}
							if !allowedCalls[callee] {
								t.Errorf("inactive final-run preflight acquired unreviewed call %q at %s",
									callee, fset.Position(expression.Pos()))
							}
						}
						return true
					})
				}
				if typed.Name.Name == "writeTo" {
					ast.Inspect(typed.Body, func(node ast.Node) bool {
						branch, ok := node.(*ast.IfStmt)
						if !ok || profilerCompactStorageCallCount(branch.Body, "verifyRunRecord") == 0 {
							return true
						}
						activeVerdictBranches++
						if profilerCompactStorageCallCount(branch.Body, "rowPublishable") != 0 ||
							profilerCompactStorageCallCount(branch.Else, "rowPublishable") != 1 {
							t.Errorf("active typed verdict branch regained rowPublishable fallback at %s",
								fset.Position(branch.Pos()))
						}
						return true
					})
				}
				if typed.Name.Name != "buildProfilerSourceOrderSidecar" {
					continue
				}
				sidecarBuilders++
				ast.Inspect(typed.Body, func(node ast.Node) bool {
					switch expression := node.(type) {
					case *ast.MapType:
						t.Errorf("buildProfilerSourceOrderSidecar in %s regained a dynamic map at %s",
							name, fset.Position(expression.Pos()))
					case *ast.CallExpr:
						callee, ok := expression.Fun.(*ast.Ident)
						if ok && callee.Name == "make" {
							t.Errorf("buildProfilerSourceOrderSidecar in %s regained a payload-sized make at %s",
								name, fset.Position(expression.Pos()))
						}
					}
					return true
				})
			}
		}
	}
	for name := range wantStructs {
		if definitions[name] != 1 {
			t.Errorf("compact storage struct %s definitions=%d want=1", name, definitions[name])
		}
	}
	if sidecarBuilders != 1 {
		t.Errorf("buildProfilerSourceOrderSidecar definitions=%d want=1", sidecarBuilders)
	}
	if preflightAuthenticators != 1 {
		t.Errorf("authenticatePreparedFinalRun definitions=%d want=1", preflightAuthenticators)
	}
	if activeVerdictBranches != 1 {
		t.Errorf("active typed publication verdict branches=%d want=1", activeVerdictBranches)
	}
	if sqlExportFunctions != 1 || sqlOrdinaryConstructors != 1 || sqlGenericConstructors != 0 {
		t.Errorf("SQL production sink constructor authority drifted: functions=%d ordinary=%d generic=%d",
			sqlExportFunctions, sqlOrdinaryConstructors, sqlGenericConstructors)
	}
}

func profilerCompactStorageCallCount(node ast.Node, name string) int {
	count := 0
	ast.Inspect(node, func(candidate ast.Node) bool {
		call, ok := candidate.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.Ident:
			if function.Name == name {
				count++
			}
		case *ast.SelectorExpr:
			if function.Sel.Name == name {
				count++
			}
		}
		return true
	})
	return count
}

func profilerCompactStorageStructFields(t *testing.T, fset *token.FileSet, typeName string,
	structure *ast.StructType,
) []profilerCompactStorageFieldPin {
	t.Helper()
	fields := make([]profilerCompactStorageFieldPin, 0, len(structure.Fields.List))
	for _, field := range structure.Fields.List {
		if len(field.Names) != 1 {
			t.Errorf("%s contains an embedded or grouped storage field", typeName)
			continue
		}
		var rendered bytes.Buffer
		if err := format.Node(&rendered, fset, field.Type); err != nil {
			t.Fatalf("render %s.%s type: %v", typeName, field.Names[0].Name, err)
		}
		tag := ""
		if field.Tag != nil {
			tag = field.Tag.Value
		}
		fields = append(fields, profilerCompactStorageFieldPin{
			name: field.Names[0].Name, typeName: rendered.String(), tag: tag,
		})
	}
	return fields
}

func profilerCompactStorageFieldsEqual(left, right []profilerCompactStorageFieldPin) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
