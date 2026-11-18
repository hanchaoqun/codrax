package types

// trace_measured_state_fact_census_test.go — who may read the two state-account
// identity fields.
//
// One producer hash travels on two fields that answer different questions:
// StateAccountKey ("priced exactly at this account", the one-seat convergence
// credential) and MeasuredStateAccountKey ("shows this measured account").
// A display consumer that reads the pricing field directly asks the wrong
// question and shows a priced-differently seat twice; a seat/merge decision
// that reads the measured field would fold an estimate into a measurement.
//
// The census is structural: every non-test Go file under internal/ that
// selects either field must be on its list, and every listed file must still
// select it. Display consumers go through MeasuredStateFactKey() and therefore
// appear on neither list.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var stateAccountIdentityFieldReaders = map[string][]string{
	// Producer mint and stamp, the wire emission, the projection compile, the
	// three seat/containment decisions, and the one display accessor.
	"StateAccountKey": {
		"tool/trace_query.go",
		"tracequery/state_account_identity.go",
		"types/trace_causal_projection.go",
		"types/trace_causal_projection_oneseat.go",
		"types/trace_causal_projection_sleep_instance.go",
		"types/trace_causal_projection_state_cover.go",
		"types/trace_measured_state_fact.go",
	},
	// Producer stamp, the wire emission, the projection compile, and the one
	// display accessor. No seat, rank, merge or causal decision is listed.
	"MeasuredStateAccountKey": {
		"tool/trace_query.go",
		"tracequery/state_account_identity.go",
		"types/trace_causal_projection.go",
		"types/trace_measured_state_fact.go",
	},
	// Producer stamp, the wire emission, the projection compile, the ×N fold
	// that keeps it only on unanimity, and the display accessor.
	"MeasuredOccurrenceGroupKey": {
		"tool/trace_query.go",
		"tracequery/state_account_identity.go",
		"types/trace_causal_projection.go",
		"types/trace_causal_projection_aggregate.go",
		"types/trace_measured_state_fact.go",
	},
}

func TestStateAccountIdentityFieldsHaveDeclaredReaders(t *testing.T) {
	got := map[string]map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, ".."+string(filepath.Separator)))
		ast.Inspect(file, func(n ast.Node) bool {
			if selector, ok := n.(*ast.SelectorExpr); ok {
				if _, tracked := stateAccountIdentityFieldReaders[selector.Sel.Name]; tracked {
					if got[selector.Sel.Name] == nil {
						got[selector.Sel.Name] = map[string]bool{}
					}
					got[selector.Sel.Name][rel] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range stateAccountIdentityFieldReaders {
		var files []string
		for file := range got[field] {
			files = append(files, file)
		}
		sort.Strings(files)
		if len(files) == 0 {
			t.Fatalf("%s: the scan matched no reader; a census that matches nothing checks nothing", field)
		}
		if strings.Join(files, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s readers drifted.\n got: %s\nwant: %s\n"+
				"A display consumer must ask MeasuredStateFactKey(); a seat or merge decision must read StateAccountKey only. Review the new reader, then update this list.",
				field, strings.Join(files, ", "), strings.Join(want, ", "))
		}
	}
}
