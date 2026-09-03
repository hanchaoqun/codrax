package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

// trace_convert_diagnostic_advertisement_test.go — fold-in round four
// (colleague_merge_audit §40.38 四轮收编, finding O): every typed line the
// diagnostic report body can emit is advertised in the build-advertisement
// roster, so a report without the line can be told apart from an older
// build. The census binds by data flow, not by a hand-copied list:
//   - every `traceConvertDiagnosticJSONLine("typed_error_<name>", …)`
//     producer in trace_convert_diagnostic.go — and every other string
//     literal starting with "typed_error_" outside the advertisement map
//     itself (a label built for fmt.Sprintf is a line too) — is a key of
//     traceConvertDiagnosticTypedLineAdvertisements;
//   - every key of that map is produced (stale advertisement red);
//   - every advertised entry is a member of traceConvertDiagnosticCapabilities;
//   - every member of hitraceconv.AllTraceEventInvalidKinds() (the `kind`
//     value of the typed_error_event_invalid line) is advertised, and the
//     record-sequence foreign-row split has its own entry.

const typedLineAdvertisementMapName = "traceConvertDiagnosticTypedLineAdvertisements"

// typedErrorLineLabelsIn returns label → position of every typed_error_
// literal in the file that is not part of the advertisement map itself.
func typedErrorLineLabelsIn(fset *token.FileSet, file *ast.File) map[string]string {
	labels := map[string]string{}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			isMap := false
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range vs.Names {
						if name.Name == typedLineAdvertisementMapName {
							isMap = true
						}
					}
				}
			}
			if isMap {
				continue
			}
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil || !strings.HasPrefix(value, "typed_error_") {
				return true
			}
			if _, seen := labels[value]; !seen {
				labels[value] = fset.Position(lit.Pos()).String()
			}
			return true
		})
	}
	return labels
}

// typedLineAdvertisementProblems applies the census rules.
func typedLineAdvertisementProblems(produced map[string]string, advertisements map[string]string, roster []string) []string {
	inRoster := map[string]bool{}
	for _, entry := range roster {
		inRoster[entry] = true
	}
	var problems []string
	for label, pos := range produced {
		entry, ok := advertisements[label]
		if !ok {
			problems = append(problems, pos+": typed line "+label+" is emitted without a capability advertisement")
			continue
		}
		if !inRoster[entry] {
			problems = append(problems, "typed line "+label+" advertises "+entry+", which is not in traceConvertDiagnosticCapabilities")
		}
	}
	for label := range advertisements {
		if _, ok := produced[label]; !ok {
			problems = append(problems, "advertised typed line "+label+" has no producer (stale advertisement)")
		}
	}
	sort.Strings(problems)
	return problems
}

func TestTraceConvertDiagnosticTypedLinesAreAdvertisedCapabilities(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "trace_convert_diagnostic.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	produced := typedErrorLineLabelsIn(fset, file)
	if len(produced) < 7 {
		t.Fatalf("expected the seven typed_error lines in the source, got %v", produced)
	}
	for _, problem := range typedLineAdvertisementProblems(produced, traceConvertDiagnosticTypedLineAdvertisements, traceConvertDiagnosticCapabilities) {
		t.Error(problem)
	}
	// Kind split: every closed kind is advertised; the foreign-row split has
	// its own entry; every entry is in the roster.
	inRoster := map[string]bool{}
	for _, entry := range traceConvertDiagnosticCapabilities {
		if inRoster[entry] {
			t.Errorf("capability %q is listed twice", entry)
		}
		inRoster[entry] = true
	}
	kinds := hitraceconv.AllTraceEventInvalidKinds()
	if len(kinds) != len(traceConvertDiagnosticEventInvalidKindAdvertisements) {
		t.Errorf("kind advertisements cover %d kinds, the closed set has %d", len(traceConvertDiagnosticEventInvalidKindAdvertisements), len(kinds))
	}
	for _, kind := range kinds {
		entry, ok := traceConvertDiagnosticEventInvalidKindAdvertisements[kind]
		if !ok {
			t.Errorf("event_invalid kind %q is not advertised", kind)
			continue
		}
		if !inRoster[entry] {
			t.Errorf("event_invalid kind %q advertises %q, which is not in the roster", kind, entry)
		}
	}
	if traceConvertDiagnosticEventInvalidKindAdvertisements[hitraceconv.TraceEventInvalidTraceDBRecordSequenceForeignRow] != "trace_db_record_sequence_foreign_row_v1" ||
		traceConvertDiagnosticTypedLineAdvertisements["typed_error_event_invalid"] != "event_invalid_first_witness_v1" {
		t.Fatalf("the event_invalid line and the foreign-row kind split must carry their own advertisements: %v / %v",
			traceConvertDiagnosticTypedLineAdvertisements, traceConvertDiagnosticEventInvalidKindAdvertisements)
	}
	// The advertisement entries are rendered on the build_advertisement
	// line of the report body (deliberate golden update, EVOLUTION RECORD:
	// three entries appended — conversion_typed_error_graph_v1,
	// event_invalid_first_witness_v1, trace_db_record_sequence_foreign_row_v1).
	body := string(traceConvertDiagnosticReportBody(hitraceconv.Options{InputPath: "capture.sys"}, hitraceconv.Result{InputPath: "capture.sys"}, traceConvertDiagnosticProgressLog{}, nil))
	for _, entry := range []string{"conversion_typed_error_graph_v1", "event_invalid_first_witness_v1", "trace_db_record_sequence_foreign_row_v1"} {
		if !strings.Contains(body, `"`+entry+`"`) {
			t.Errorf("report body does not advertise %q", entry)
		}
	}
	assertTraceConvertDiagnosticCapabilitiesAreBuildAdvertisement(t, body)
}

// Self-red: a new typed line (JSON-line producer or a Sprintf label) is red
// until it joins the map; a stale map key is red; an entry outside the
// roster is red.
func TestTraceConvertDiagnosticTypedLineCensusSelfRed(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "probe.go", `package cmd

var traceConvertDiagnosticTypedLineAdvertisements = map[string]string{
	"typed_error_only_in_map": "entry_v1",
}

func probe(lines *traceConvertDiagnosticLineSet) {
	lines.Add(traceConvertDiagnosticJSONLine("typed_error_future_witness", map[string]any{"x": 1}))
	lines.Add(fmt.Sprintf("%s=%d", "typed_error_future_scalar", 1))
	lines.Add(traceConvertDiagnosticJSONLine("typed_error_known", nil))
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	produced := typedErrorLineLabelsIn(fset, file)
	if _, leaked := produced["typed_error_only_in_map"]; leaked {
		t.Fatalf("map keys are not producers: %v", produced)
	}
	problems := typedLineAdvertisementProblems(produced,
		map[string]string{"typed_error_known": "not_in_roster_v1", "typed_error_only_in_map": "entry_v1"},
		[]string{"entry_v1"})
	joined := strings.Join(problems, "\n")
	for _, want := range []string{
		"typed line typed_error_future_witness is emitted without a capability advertisement",
		"typed line typed_error_future_scalar is emitted without a capability advertisement",
		"typed line typed_error_known advertises not_in_roster_v1, which is not in traceConvertDiagnosticCapabilities",
		"advertised typed line typed_error_only_in_map has no producer (stale advertisement)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("self-red missed %q: %v", want, problems)
		}
	}
}
