package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"testing"
)

// Routing never reads rows. Pin its narrow schema candidates to the real
// exporters' metadata calls so changing a reader cannot silently leave an
// independent, stale claim in the initial classifier context.
func TestHMC221RoutingCandidateSchemasMatchNativeExporters(t *testing.T) {
	for _, target := range []struct {
		view, file, function string
		tables               []string
	}{
		{"measurements", "streamerdb_measure_intervals.go", "exportTraceDBMeasureIntervals", []string{"measure"}},
		{"measurements", "streamerdb_measure_intervals.go", "loadMeasureFilters", []string{"measure_filter"}},
		{"process_measurements", "streamerdb_process_measure_intervals.go", "exportTraceDBProcessMeasureIntervals", []string{"process_measure"}},
		{"process_measurements", "streamerdb_process_measure_intervals.go", "loadProcessMeasureFilters", []string{"process_measure_filter"}},
		{"cpu_state_frequency", "streamerdb_export_measure.go", "exportTraceDBMeasureFamilies", []string{"measure", "cpu_measure_filter"}},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), target.file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		actual := map[string][]string{}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != target.function {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) < 2 {
					return true
				}
				name := ""
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					name = f.Sel.Name
				case *ast.Ident:
					name = f.Name
				}
				if name != "inspectCoverage" && name != "inspectTraceDBMeasureCoverage" {
					return true
				}
				table, ok := call.Args[len(call.Args)-2].(*ast.BasicLit)
				if !ok {
					return true
				}
				key, err := strconv.Unquote(table.Value)
				if err != nil {
					return true
				}
				cols, ok := call.Args[len(call.Args)-1].(*ast.CompositeLit)
				if !ok {
					return true
				}
				for _, elt := range cols.Elts {
					if lit, ok := elt.(*ast.BasicLit); ok {
						v, err := strconv.Unquote(lit.Value)
						if err == nil {
							actual[key] = append(actual[key], v)
						}
					}
				}
				return true
			})
		}
		found := false
		for _, family := range nativeRoutingSchemaFamilies() {
			if family.view != target.view {
				continue
			}
			found = true
			for _, table := range target.tables {
				if len(actual[table]) == 0 || !reflect.DeepEqual(actual[table], family.tables[table]) {
					t.Fatalf("%s/%s routing=%v exporter %s=%v", target.view, table, family.tables[table], target.function, actual[table])
				}
			}
		}
		if !found {
			t.Fatalf("missing routing family %s", target.view)
		}
	}
}
