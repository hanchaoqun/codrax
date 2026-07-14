package hitraceconv

import (
	"bytes"
	"encoding/binary"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func profilerRepeatedDiagnosticFrameFixture(message []byte, count int) []byte {
	frameBytes := 4 + len(message)
	body := make([]byte, profilerTraceHeaderSize+count*frameBytes)
	binary.LittleEndian.PutUint64(body[0:8], profilerTraceHeaderMagic)
	binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)))
	binary.LittleEndian.PutUint32(body[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(body[20:24], uint32(count*2))
	binary.LittleEndian.PutUint32(body[56:60], profilerDataTypeProtobuf)
	offset := profilerTraceHeaderSize
	for index := 0; index < count; index++ {
		binary.LittleEndian.PutUint32(body[offset:offset+4], uint32(len(message)))
		offset += 4
		copy(body[offset:offset+len(message)], message)
		offset += len(message)
	}
	return body
}

func profilerDiagnosticCoverage(extracted profilerContainerExtraction, table string) (TraceDBCoverage, int) {
	var found TraceDBCoverage
	count := 0
	for _, item := range extracted.TraceCoverage {
		if item.Family == "builtin_modern_profiler" && item.Table == table {
			found = item
			count++
		}
	}
	return found, count
}

func profilerPublisherCoverageStateForTest(
	t *testing.T,
	extracted profilerContainerExtraction,
) (int, uint64) {
	t.Helper()
	presentSlots := 0
	stagedRows := uint64(0)
	for publisher := profilerPairPublisherSlot(1); publisher < profilerPairPublisherSlotCount; publisher++ {
		coverageIndex, present := extracted.profilerPublisherCoverage.coverageIndex(publisher)
		if !present {
			continue
		}
		presentSlots++
		if coverageIndex < 0 || coverageIndex >= len(extracted.TraceCoverage) {
			t.Fatalf("publisher %d retained invalid fixed coverage index %d: coverage=%+v",
				publisher, coverageIndex, extracted.TraceCoverage)
		}
		for _, kind := range profilerCaptureKinds {
			raw, present := extracted.TraceCoverage[coverageIndex].FieldSources[profilerCoverageStagedRowsKey(kind)]
			if !present {
				continue
			}
			count, err := strconv.ParseUint(raw, 10, 64)
			if err != nil || ^uint64(0)-stagedRows < count {
				t.Fatalf("publisher %d retained invalid staged %s count %q: err=%v",
					publisher, profilerCoverageStagedRowsKey(kind), raw, err)
			}
			stagedRows += count
		}
	}
	return presentSlots, stagedRows
}

func TestProfilerOuterAcceptedDiagnosticsRetainedShapeIsConstant(t *testing.T) {
	message := protoBytes(1, []byte("bytrace_plugin"))
	for _, frameCount := range []int{1, 4_096, 1_000_000} {
		t.Run(strconv.Itoa(frameCount), func(t *testing.T) {
			extracted, sink := extractProfilerCensusFixture(t, profilerRepeatedDiagnosticFrameFixture(message, frameCount))
			defer sink.cleanup()
			coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:bytrace_plugin")
			_, stagedPairRows := profilerPublisherCoverageStateForTest(t, extracted)
			if extracted.Messages != frameCount || extracted.RejectedMessages != 0 || extracted.SourceFailClosed ||
				extracted.PluginMessages["bytrace_plugin"] != frameCount || len(extracted.PluginMessages) != 1 ||
				coverage.RowsRead != frameCount || coverage.RowsEmitted != 0 || entries != 1 ||
				coverage.FieldSources["observed_messages"] != strconv.Itoa(frameCount) ||
				coverage.FieldSources["outcome_empty_payload_frames"] != strconv.Itoa(frameCount) {
				t.Fatalf("accepted diagnostic census drifted at %d frames: extracted=%+v coverage=%+v entries=%d",
					frameCount, extracted, coverage, entries)
			}
			if len(extracted.TraceCoverage) != 1 || len(extracted.Caveats) != 2 || stagedPairRows != 0 {
				t.Fatalf("pair-free retained diagnostics grew at %d frames: caveats=%d coverage=%d staged_pair_rows=%d",
					frameCount, len(extracted.Caveats), len(extracted.TraceCoverage), stagedPairRows)
			}
		})
	}
}

func profilerDistinctNameDiagnosticFixture(count int, reverse bool) []byte {
	messages := make([][]byte, count)
	for index := 0; index < count; index++ {
		value := index
		if reverse {
			value = count - index - 1
		}
		messages[index] = protoBytes(1, []byte("plugin-"+strconv.FormatInt(int64(value+10_000_000), 10)))
	}
	return syntheticProfilerTraceFile(messages...)
}

func TestProfilerOuterDynamicNameSamplesAreStableAndBounded(t *testing.T) {
	const count = 4_096
	var sampleLedger string
	for index, reverse := range []bool{false, true} {
		extracted, sink := extractProfilerCensusFixture(t, profilerDistinctNameDiagnosticFixture(count, reverse))
		coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:__other_text__")
		sink.cleanup()
		if entries != 1 || coverage.RowsRead != count || extracted.PluginMessages["__other_text__"] != count ||
			len(extracted.PluginMessages) != 1 || coverage.FieldSources["identity_compacted"] != "true" ||
			coverage.FieldSources["original_plugin_name_table_key"] != "false" {
			t.Fatalf("dynamic identity bucket drifted: extracted=%+v coverage=%+v", extracted, coverage)
		}
		samples := coverage.FieldSources["plugin_name_samples"]
		if strings.Count(samples, "sha256=") != profilerDiagnosticSampleLimit || len(samples) > profilerDiagnosticSampleLimit*800 {
			t.Fatalf("dynamic identity samples exceeded the fixed K/serialized bound: len=%d samples=%q", len(samples), samples)
		}
		if index == 0 {
			sampleLedger = samples
		} else if samples != sampleLedger {
			t.Fatalf("SHA-min samples changed with input order:\nforward=%s\nreverse=%s", sampleLedger, samples)
		}
	}
}

func TestProfilerOuterRouteBucketsAreClosed(t *testing.T) {
	messages := [][]byte{
		protoBytes(1, []byte("ftrace-plugin")),
		protoBytes(1, []byte("bytrace_plugin")),
		protoBytes(1, []byte("FTRACE-PLUGIN")),
		protoBytes(1, []byte("vendor_plugin")),
	}
	extracted, sink := extractProfilerCensusFixture(t, syntheticProfilerTraceFile(messages...))
	defer sink.cleanup()
	_, stagedPairRows := profilerPublisherCoverageStateForTest(t, extracted)
	want := map[string]string{
		"plugin:ftrace-plugin":           "outcome_unsupported_ftrace_frames",
		"plugin:bytrace_plugin":          "outcome_empty_payload_frames",
		"plugin:__noncanonical_ftrace__": "outcome_noncanonical_ftrace_frames",
		"plugin:__other_text__":          "outcome_empty_payload_frames",
	}
	for table, outcome := range want {
		coverage, entries := profilerDiagnosticCoverage(extracted, table)
		if entries != 1 || coverage.RowsRead != 1 || coverage.FieldSources[outcome] != "1" {
			t.Fatalf("route %s did not enter its one typed bucket/outcome %s: %+v entries=%d", table, outcome, coverage, entries)
		}
	}
	if len(extracted.TraceCoverage) != len(want) || len(extracted.PluginMessages) != len(want) ||
		extracted.UnsupportedFtrace != 2 || stagedPairRows != 0 {
		t.Fatalf("outer route closed set drifted: %+v coverage=%+v", extracted, extracted.TraceCoverage)
	}
}

func TestProfilerOuterExactFtraceOutcomeMatrix(t *testing.T) {
	structured := protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(5_000_000_000, 7, 7, "worker", 1109, protoBytes(2, []byte("B|7|typed"))),
	)
	strict := []byte("worker-7 (7) [001] .... 5.000000: tracing_mark_write: B|7|legacy")
	strictOverlap := []byte("*worker-7 (7) [001] .... 5.000000: tracing_mark_write: B|7|overlap")
	malformed := append(protoBytes(5, []byte{1, 2, 3}), 0)
	for _, test := range []struct {
		name           string
		payload        []byte
		outcomeField   string
		structured     int
		malformed      int
		unsupported    int
		textMessages   int
		textRows       int
		structuredRows int
	}{
		{name: "structured", payload: structured, outcomeField: "outcome_structured_frames", structured: 1, structuredRows: 1},
		{name: "malformed structured", payload: malformed, outcomeField: "outcome_malformed_frames", malformed: 1, unsupported: 1},
		{name: "strict protobuf overlap", payload: strictOverlap, outcomeField: "outcome_strict_legacy_text_frames", textMessages: 1, textRows: 1},
		{name: "strict legacy", payload: strict, outcomeField: "outcome_strict_legacy_text_frames", textMessages: 1, textRows: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			extracted, sink := extractSyntheticProfilerContainer(t, syntheticProfilerPluginData("ftrace-plugin", test.payload))
			defer sink.cleanup()
			coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:ftrace-plugin")
			if entries != 1 || coverage.RowsRead != 1 || coverage.FieldSources[test.outcomeField] != "1" ||
				extracted.StructuredFtrace != test.structured || extracted.MalformedFtrace != test.malformed ||
				extracted.UnsupportedFtrace != test.unsupported || extracted.TextPluginMessages != test.textMessages ||
				extracted.TextRows != test.textRows || extracted.StructuredRows != test.structuredRows {
				t.Fatalf("exact ftrace outcome %s drifted: extracted=%+v coverage=%+v", test.name, extracted, coverage)
			}
		})
	}
}

func TestProfilerOuterRejectedDiagnosticsAggregateBaseReasons(t *testing.T) {
	missingName := protoVarint(2, 1)
	wrongWireName := protoVarint(1, 1)
	valid := protoBytes(1, []byte("bytrace_plugin"))
	extracted, sink := extractProfilerCensusFixture(t, syntheticProfilerTraceFile(missingName, wrongWireName, valid))
	defer sink.cleanup()
	coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:__rejected__")
	if extracted.Messages != 3 || extracted.RejectedMessages != 2 || extracted.PluginMessages["bytrace_plugin"] != 1 ||
		entries != 1 || coverage.RowsRead != 2 ||
		coverage.FieldSources["issue_plugin_name_missing_occurrences"] != "1" ||
		coverage.FieldSources["issue_plugin_field1_wrong_wire_occurrences"] != "1" ||
		coverage.FieldSources["issue_plugin_name_missing_affected_frames"] != "1" ||
		coverage.FieldSources["issue_plugin_field1_wrong_wire_affected_frames"] != "1" {
		t.Fatalf("rejected reason census drifted: extracted=%+v coverage=%+v", extracted, coverage)
	}
	first := int64(profilerTraceHeaderSize)
	last := first + int64(4+len(missingName))
	if coverage.FieldSources["first_offset"] != strconv.FormatInt(first, 10) ||
		coverage.FieldSources["last_offset"] != strconv.FormatInt(last, 10) ||
		!strings.Contains(coverage.Skipped, "plugin_name_missing=1") ||
		!strings.Contains(coverage.Skipped, "plugin_field1_wrong_wire=1") {
		t.Fatalf("rejected offset/summary disclosure drifted: %+v", coverage)
	}
	if len(extracted.Caveats) != 3 || len(extracted.TraceCoverage) != 2 {
		t.Fatalf("rejected per-frame diagnostics escaped aggregation: caveats=%v coverage=%+v", extracted.Caveats, extracted.TraceCoverage)
	}
}

func TestProfilerOuterSameReasonRejectedRetainedShapeIsConstant(t *testing.T) {
	message := protoVarint(2, 1) // complete frame, but required name is absent
	for _, frameCount := range []int{1, 4_096, 1_000_000} {
		t.Run(strconv.Itoa(frameCount), func(t *testing.T) {
			extracted, sink := extractProfilerCensusFixture(t, profilerRepeatedDiagnosticFrameFixture(message, frameCount))
			defer sink.cleanup()
			coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:__rejected__")
			_, stagedPairRows := profilerPublisherCoverageStateForTest(t, extracted)
			if extracted.Messages != frameCount || extracted.RejectedMessages != frameCount || extracted.SourceFailClosed ||
				entries != 1 || coverage.RowsRead != frameCount || coverage.RowsEmitted != 0 ||
				coverage.FieldSources["issue_plugin_name_missing_occurrences"] != strconv.Itoa(frameCount) ||
				coverage.FieldSources["issue_plugin_name_missing_affected_frames"] != strconv.Itoa(frameCount) ||
				len(extracted.Caveats) != 2 || len(extracted.TraceCoverage) != 1 || len(extracted.PluginMessages) != 0 ||
				stagedPairRows != 0 {
				t.Fatalf("same-reason rejected diagnostics grew or drifted at %d: extracted=%+v coverage=%+v entries=%d",
					frameCount, extracted, coverage, entries)
			}
			first := int64(profilerTraceHeaderSize)
			last := first + int64(frameCount-1)*(4+int64(len(message)))
			if coverage.FieldSources["first_offset"] != strconv.FormatInt(first, 10) ||
				coverage.FieldSources["last_offset"] != strconv.FormatInt(last, 10) {
				t.Fatalf("same-reason rejected offsets drifted at %d: %+v", frameCount, coverage)
			}
		})
	}
}

func TestProfilerOuterAcceptedDegradationKeepsIndependentBoundaries(t *testing.T) {
	clean := protoBytes(1, []byte("bytrace_plugin"))
	degraded := protoPayload(protoBytes(1, []byte("bytrace_plugin")), protoBytes(2, nil))
	extracted, sink := extractProfilerCensusFixture(t, syntheticProfilerTraceFile(clean, degraded))
	defer sink.cleanup()
	coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:bytrace_plugin")
	wantOffset := int64(profilerTraceHeaderSize + 4 + len(clean))
	if entries != 1 || coverage.RowsRead != 2 || extracted.RejectedMessages != 0 ||
		coverage.FieldSources["metadata_degraded_frames"] != "1" ||
		coverage.FieldSources["metadata_degraded_first_offset"] != strconv.FormatInt(wantOffset, 10) ||
		coverage.FieldSources["metadata_degraded_last_offset"] != strconv.FormatInt(wantOffset, 10) ||
		coverage.FieldSources["issue_plugin_field2_wrong_wire_occurrences"] != "1" ||
		coverage.FieldSources["issue_plugin_field2_wrong_wire_affected_frames"] != "1" {
		t.Fatalf("accepted degradation boundaries drifted: extracted=%+v coverage=%+v", extracted, coverage)
	}
}

func TestProfilerOuterAcceptedIssueCountersMergeOccurrencesAndFrames(t *testing.T) {
	name := protoBytes(1, []byte("bytrace_plugin"))
	wrong := protoBytes(2, nil)
	first := protoPayload(name, wrong, wrong)
	second := protoPayload(name, wrong)
	clean := name
	extracted, sink := extractProfilerCensusFixture(t, syntheticProfilerTraceFile(first, second, clean))
	defer sink.cleanup()
	coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:bytrace_plugin")
	fields := coverage.FieldSources
	if entries != 1 || coverage.RowsRead != 3 || fields["metadata_degraded_frames"] != "2" ||
		fields["issue_plugin_field2_wrong_wire_occurrences"] != "3" ||
		fields["issue_plugin_field2_wrong_wire_affected_frames"] != "2" ||
		fields["issue_plugin_field2_duplicate_occurrences"] != "1" ||
		fields["issue_plugin_field2_duplicate_affected_frames"] != "1" ||
		fields["issue_plugin_field2_duplicate_excess_occurrences"] != "1" {
		t.Fatalf("accepted issue aggregate units drifted: extracted=%+v coverage=%+v", extracted, coverage)
	}
}

func TestParseProfilerPluginDataWrongWireStormUsesFixedIssueLedger(t *testing.T) {
	const occurrences = 1_000_000
	wrongWire := protoBytes(2, nil)
	input := append([]byte(nil), protoBytes(1, []byte("bytrace_plugin"))...)
	input = append(input, bytes.Repeat(wrongWire, occurrences)...)
	decoded := parseProfilerPluginData(input)
	wrongKind, _ := profilerPluginWrongWireIssue(2)
	duplicateKind, _ := profilerPluginDuplicateIssue(2)
	if !decoded.Accepted || decoded.IssueOverflow || len(decoded.IssueCensus.labels()) != 2 ||
		decoded.IssueCensus.Occurrences[wrongKind] != occurrences ||
		decoded.IssueCensus.AffectedFrames[wrongKind] != 1 ||
		decoded.IssueCensus.Occurrences[duplicateKind] != 1 ||
		decoded.IssueCensus.AffectedFrames[duplicateKind] != 1 ||
		decoded.IssueCensus.DuplicateExcess[1] != occurrences-1 {
		t.Fatalf("wrong-wire storm census drifted: accepted=%t overflow=%t issues=%s census=%+v",
			decoded.Accepted, decoded.IssueOverflow, decoded.IssueCensus.summary(), decoded.IssueCensus)
	}
}

func TestProfilerOuterMetadataAggregationPreservesTypedProvenance(t *testing.T) {
	messages := [][]byte{
		syntheticProfilerPluginDataWithTiming("bytrace_plugin", nil, 1, 12, 34, "v1", 250),
		syntheticProfilerPluginDataWithTiming("bytrace_plugin", nil, 4, 13, 56, "v2", 500),
	}
	extracted, sink := extractProfilerCensusFixture(t, syntheticProfilerTraceFile(messages...))
	defer sink.cleanup()
	coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:bytrace_plugin")
	fields := coverage.FieldSources
	if entries != 1 || coverage.RowsRead != 2 || fields["payload_count"] != "2" ||
		fields["clock_id_1_count"] != "1" || fields["clock_id_4_count"] != "1" ||
		fields["time_tuple_min"] != "12.000000034" || fields["time_tuple_max"] != "13.000000056" ||
		fields["sample_interval_min_ms"] != "250" || fields["sample_interval_max_ms"] != "500" ||
		fields["version_present"] != "2" || strings.Count(fields["version_samples"], "sha256=") != 2 {
		t.Fatalf("metadata aggregation lost typed provenance: %+v", coverage)
	}
}

func TestProfilerOuterVersionSamplesAreStableAndBounded(t *testing.T) {
	const count = 32
	var forward string
	for pass, reverse := range []bool{false, true} {
		messages := make([][]byte, count)
		for index := 0; index < count; index++ {
			value := index
			if reverse {
				value = count - index - 1
			}
			messages[index] = protoPayload(
				protoBytes(1, []byte("bytrace_plugin")),
				protoBytes(7, []byte("version-"+strconv.Itoa(value))),
			)
		}
		extracted, sink := extractProfilerCensusFixture(t, syntheticProfilerTraceFile(messages...))
		coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:bytrace_plugin")
		sink.cleanup()
		samples := coverage.FieldSources["version_samples"]
		if entries != 1 || coverage.FieldSources["version_present"] != strconv.Itoa(count) ||
			strings.Count(samples, "sha256=") != profilerDiagnosticSampleLimit || len(samples) > profilerDiagnosticSampleLimit*800 {
			t.Fatalf("version samples escaped fixed bounds: coverage=%+v", coverage)
		}
		if pass == 0 {
			forward = samples
		} else if samples != forward {
			t.Fatalf("version SHA-min samples changed with input order:\nforward=%s\nreverse=%s", forward, samples)
		}
	}
}

func TestProfilerOuterPairPublishersShareStableAggregateCoverageIndex(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	messages := [][]byte{
		syntheticProfilerPluginData("vendor-a", []byte("io-100 (100) [002] .... 1.000000: mmc_request_start: "+startBody+"\n")),
		syntheticProfilerPluginData("vendor-b", []byte("io-100 (100) [002] .... 1.001000: mmc_request_done: "+doneBody+"\n")),
		protoBytes(1, []byte("vendor-c")), // pair-free frame shares the route only
	}
	extracted, sink := extractProfilerCensusFixture(t, syntheticProfilerTraceFile(messages...))
	defer sink.cleanup()
	coverage, entries := profilerDiagnosticCoverage(extracted, "plugin:__other_text__")
	coverageIndex, publisherPresent := extracted.profilerPublisherCoverage.coverageIndex(
		profilerPairPublisherOtherText,
	)
	publisherSlots, stagedPairRows := profilerPublisherCoverageStateForTest(t, extracted)
	if entries != 1 || coverage.RowsRead != 3 || coverage.RowsEmitted != 2 ||
		coverage.FieldSources[profilerCoverageMMCStagedRows] != "2" ||
		extracted.TextPluginMessages != 2 || extracted.TextRows != 2 ||
		!publisherPresent || publisherSlots != 1 || stagedPairRows != 2 {
		t.Fatalf("pair-bearing publisher census drifted: extracted=%+v coverage=%+v", extracted, coverage)
	}
	if coverageIndex < 0 || coverageIndex >= len(extracted.TraceCoverage) ||
		extracted.TraceCoverage[coverageIndex].Table != "plugin:__other_text__" ||
		extracted.TraceCoverage[coverageIndex].FieldSources[profilerCoverageMMCStagedRows] != "2" {
		t.Fatalf("publisher did not bind to the aggregate route coverage: index=%d coverage=%+v", coverageIndex, extracted.TraceCoverage)
	}
}

func TestProfilerOuterDiagnosticLedgerHasNoDynamicRetainedCollections(t *testing.T) {
	extractionType := reflect.TypeOf(profilerContainerExtraction{})
	for _, removedField := range []string{"pairPublishers", "textMessages"} {
		if field, present := extractionType.FieldByName(removedField); present {
			t.Fatalf("profiler extraction regained legacy per-message field %s kind=%s", removedField, field.Type.Kind())
		}
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "profiler_container_diagnostics.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	retained := map[string]bool{
		"profilerPluginIssueCensus":         true,
		"profilerStableSampleSet":           true,
		"profilerPluginMetadataCensus":      true,
		"profilerPluginBucketCensus":        true,
		"profilerRejectedFrameCensus":       true,
		"profilerContainerDiagnosticLedger": true,
	}
	pluginKeys := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || !retained[typeSpec.Name.Name] {
				continue
			}
			ast.Inspect(typeSpec.Type, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.MapType:
					t.Fatalf("%s regained a dynamic retained map: %s", typeSpec.Name.Name, fset.Position(typed.Pos()))
				case *ast.ArrayType:
					if typed.Len == nil {
						t.Fatalf("%s regained a retained slice: %s", typeSpec.Name.Name, fset.Position(typed.Pos()))
					}
				}
				return true
			})
			delete(retained, typeSpec.Name.Name)
		}
	}
	if len(retained) != 0 {
		t.Fatalf("retained diagnostic type pin drifted: missing=%v", reflect.ValueOf(retained).MapKeys())
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "pluginKey" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if ok && literal.Kind == token.STRING {
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				pluginKeys[value] = true
			}
			return true
		})
	}
	wantPluginKeys := map[string]bool{
		"ftrace-plugin": true, "bytrace_plugin": true, "__noncanonical_ftrace__": true,
		"__other_text__": true, "__invalid_plugin_route__": true,
	}
	if !reflect.DeepEqual(pluginKeys, wantPluginKeys) {
		t.Fatalf("outer plugin route key closed set drifted: got=%v want=%v", pluginKeys, wantPluginKeys)
	}
	prospectiveFound, coverageIndexAssignments, coverageAppends := false, 0, 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "ensurePluginCoverage" {
			t.Fatal("context-free mutating plugin coverage authority was reintroduced")
		}
		if !ok || fn.Name.Name != "observeAcceptedContext" {
			continue
		}
		prospectiveFound = true
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if assign, ok := node.(*ast.AssignStmt); ok {
				for _, lhs := range assign.Lhs {
					if selector, ok := lhs.(*ast.SelectorExpr); ok && selector.Sel.Name == "CoverageIndex" {
						coverageIndexAssignments++
					}
				}
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "append" && len(call.Args) > 0 {
				if selector, ok := call.Args[0].(*ast.SelectorExpr); ok && selector.Sel.Name == "TraceCoverage" {
					coverageAppends++
				}
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Slice" {
				t.Fatalf("observeAcceptedContext must never sort/reorder stable coverage indices")
			}
			return true
		})
	}
	if !prospectiveFound || coverageIndexAssignments != 1 || coverageAppends != 1 {
		t.Fatalf("stable plugin coverage transaction drifted: found=%t index_assignments=%d appends=%d",
			prospectiveFound, coverageIndexAssignments, coverageAppends)
	}

	containerFile, err := parser.ParseFile(token.NewFileSet(), "profiler_container.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	parseFound, extractionFound := false, false
	for _, decl := range containerFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		switch fn.Name.Name {
		case "parseProfilerPluginData":
			parseFound = true
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if ok && ident.Name == "append" {
					t.Fatalf("parseProfilerPluginData regained append-based issue retention")
				}
				return true
			})
		case "extractProfilerTraceFileAtWithFrameLimit":
			extractionFound = true
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok || ident.Name != "append" {
					return true
				}
				for _, arg := range call.Args[1:] {
					ast.Inspect(arg, func(n ast.Node) bool {
						if value, ok := n.(*ast.Ident); ok && (value.Name == "name" || value.Name == "reason") {
							t.Fatalf("outer frame loop regained raw %s-driven retained append", value.Name)
						}
						return true
					})
				}
				return true
			})
		}
	}
	if !parseFound || !extractionFound {
		t.Fatalf("outer diagnostic structure functions missing: parse=%t extraction=%t", parseFound, extractionFound)
	}
}
