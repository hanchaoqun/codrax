package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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
//     producer in EVERY non-test file of the cmd package (fold-in round
//     five, finding HH — the round-four census parsed only
//     trace_convert_diagnostic.go, so a typed_error_ literal or constant
//     routed from another cmd file escaped it) — and every other string
//     literal starting with "typed_error_" outside the advertisement map
//     itself (a label built for fmt.Sprintf is a line too, and a constant
//     declared elsewhere is caught at its declaring literal) — is a key of
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
	collect := func(n ast.Node) bool {
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
	}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			// The exemption covers ONLY the advertisement map's own
			// ValueSpec (fold-in round six) — every sibling spec of a
			// grouped var block is still a producer position.
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				isMap := false
				for _, name := range vs.Names {
					if name.Name == typedLineAdvertisementMapName {
						isMap = true
					}
				}
				if isMap {
					continue
				}
				ast.Inspect(vs, collect)
			}
			continue
		}
		ast.Inspect(decl, collect)
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

// typedErrorLineLabelsInFiles unions the producer labels of several files
// (fold-in round five, finding HH: the census scans every non-test cmd
// file, not just trace_convert_diagnostic.go).
func typedErrorLineLabelsInFiles(fset *token.FileSet, files []*ast.File) map[string]string {
	labels := map[string]string{}
	for _, file := range files {
		for label, pos := range typedErrorLineLabelsIn(fset, file) {
			if _, seen := labels[label]; !seen {
				labels[label] = pos
			}
		}
	}
	return labels
}

func parseCmdPackageNonTestFiles(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	if len(files) < 5 {
		t.Fatalf("expected the cmd package's non-test files, parsed %d", len(files))
	}
	return fset, files
}

func TestTraceConvertDiagnosticTypedLinesAreAdvertisedCapabilities(t *testing.T) {
	fset, files := parseCmdPackageNonTestFiles(t)
	produced := typedErrorLineLabelsInFiles(fset, files)
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
	// Fold-in round six: the advertisement-map exemption covers ONLY the
	// map's own ValueSpec — a typed_error_ literal declared in the SAME var
	// block is still a producer (the round-five census exempted the whole
	// GenDecl, so a label riding in the map's var block escaped).
	file, err := parser.ParseFile(fset, "probe.go", `package cmd

var (
	typedErrorSharedBlock                         = "typed_error_shared_block_witness"
	traceConvertDiagnosticTypedLineAdvertisements = map[string]string{
		"typed_error_only_in_map": "entry_v1",
	}
)

func probe(lines *traceConvertDiagnosticLineSet) {
	lines.Add(traceConvertDiagnosticJSONLine(typedErrorSharedBlock, nil))
	lines.Add(traceConvertDiagnosticJSONLine("typed_error_future_witness", map[string]any{"x": 1}))
	lines.Add(fmt.Sprintf("%s=%d", "typed_error_future_scalar", 1))
	lines.Add(traceConvertDiagnosticJSONLine("typed_error_known", nil))
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Fold-in round five, finding HH: a typed_error_ literal or a constant
	// declared in ANOTHER cmd file is a producer too — the census unions
	// every non-test file.
	otherFile, err := parser.ParseFile(fset, "probe_other.go", `package cmd

const typedErrorRoutedFromElsewhere = "typed_error_routed_from_elsewhere"

func probeOther(lines *traceConvertDiagnosticLineSet) {
	lines.Add(traceConvertDiagnosticJSONLine("typed_error_second_file_witness", nil))
	lines.Add(traceConvertDiagnosticJSONLine(typedErrorRoutedFromElsewhere, nil))
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	produced := typedErrorLineLabelsInFiles(fset, []*ast.File{file, otherFile})
	if _, leaked := produced["typed_error_only_in_map"]; leaked {
		t.Fatalf("map keys are not producers: %v", produced)
	}
	if _, ok := produced["typed_error_shared_block_witness"]; !ok {
		t.Fatalf("self-red: a typed_error_ literal declared in the advertisement map's var block escaped the census: %v", produced)
	}
	for _, label := range []string{"typed_error_second_file_witness", "typed_error_routed_from_elsewhere"} {
		if _, ok := produced[label]; !ok {
			t.Fatalf("self-red: the multi-file census missed the second-file producer %q: %v", label, produced)
		}
	}
	secondFileProblems := typedLineAdvertisementProblems(produced,
		map[string]string{"typed_error_known": "entry_v1", "typed_error_future_witness": "entry_v1", "typed_error_future_scalar": "entry_v1"},
		[]string{"entry_v1"})
	joinedSecond := strings.Join(secondFileProblems, "\n")
	for _, want := range []string{
		"typed line typed_error_second_file_witness is emitted without a capability advertisement",
		"typed line typed_error_routed_from_elsewhere is emitted without a capability advertisement",
	} {
		if !strings.Contains(joinedSecond, want) {
			t.Fatalf("self-red missed %q: %v", want, secondFileProblems)
		}
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
