package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func syntheticProfilerEventResult(field int, payload []byte) []byte {
	return protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(5_000_000_000, 7, 7, "worker", field, payload),
	)
}

func profilerEventCoverageByField(extracted profilerContainerExtraction, field int) (TraceDBCoverage, int) {
	want := strconv.Itoa(field)
	var found TraceDBCoverage
	entries := 0
	for _, coverage := range extracted.TraceCoverage {
		if coverage.FieldSources["event_field_id"] == want {
			found = coverage
			entries++
		}
	}
	return found, entries
}

func profilerUnknownEventCoverage(extracted profilerContainerExtraction) (TraceDBCoverage, int) {
	var found TraceDBCoverage
	entries := 0
	for _, coverage := range extracted.TraceCoverage {
		if coverage.Family == "builtin_modern_ftrace:unknown" && coverage.Table == "__unknown_event_field__" {
			found = coverage
			entries++
		}
	}
	return found, entries
}

func TestProfilerEventDiagnosticsMillionObservationsUseOneFixedSlot(t *testing.T) {
	const observations = 1_000_000
	var batch profilerFtraceEventBatchCensus
	for index := 0; index < observations; index++ {
		if !batch.observeRead(2003) || !batch.observeEmitted(2003) {
			t.Fatalf("event batch overflowed at %d", index)
		}
	}
	var ledger profilerFtraceEventDiagnosticLedger
	if !ledger.merge(batch) {
		t.Fatal("merge million-event batch")
	}
	out := profilerContainerExtraction{}
	if !ledger.materialize(&out) {
		t.Fatal("materialize million-event ledger")
	}
	coverage, entries := profilerEventCoverageByField(out, 2003)
	if entries != 1 || len(out.TraceCoverage) != 1 || coverage.RowsRead != observations || coverage.RowsEmitted != observations || coverage.Skipped != "" {
		t.Fatalf("million observations grew/drifted fixed slot: entries=%d coverage=%+v out=%+v", entries, coverage, out.TraceCoverage)
	}
}

func TestProfilerEventExactIssueMillionOccurrencesKeepOneAffectedFrame(t *testing.T) {
	const observations = 1_000_000
	issue, ok := profilerFtraceEventIssueFromLegacy(2003, profilerFtraceEventDegradationCorePayload,
		"core_field1_wrong_wire")
	if !ok {
		t.Fatal("fixture exact issue rejected")
	}
	var first profilerFtraceEventBatchCensus
	for index := 0; index < observations; index++ {
		if !first.observeRead(2003) || !first.observeIssues(2003, false, []profilerFtraceEventIssue{issue}) {
			t.Fatalf("exact issue batch overflowed at %d", index)
		}
	}
	var ledger profilerFtraceEventDiagnosticLedger
	if !ledger.merge(first) {
		t.Fatal("merge million exact issues")
	}
	var second profilerFtraceEventBatchCensus
	if !second.observeRead(2003) || !second.observeIssues(2003, false, []profilerFtraceEventIssue{issue}) || !ledger.merge(second) {
		t.Fatal("merge second affected frame")
	}
	out := profilerContainerExtraction{}
	if !ledger.materialize(&out) {
		t.Fatal("materialize exact issue ledger")
	}
	coverage, entries := profilerEventCoverageByField(out, 2003)
	if entries != 1 || coverage.RowsRead != observations+1 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["degraded_core_field1_wrong_wire_occurrences"] != "1000001" ||
		coverage.FieldSources["degraded_core_field1_wrong_wire_affected_frames"] != "2" ||
		coverage.FieldSources["degraded_core_payload_occurrences"] != "1000001" ||
		coverage.FieldSources["degraded_core_payload_affected_frames"] != "2" {
		t.Fatalf("exact issue occurrence/affected units drifted: entries=%d coverage=%+v", entries, coverage)
	}
}

func TestProfilerEventAllKnownDescriptorsMaterializeOnce(t *testing.T) {
	var batch profilerFtraceEventBatchCensus
	for _, descriptor := range profilerFtraceEventDescriptorList {
		if !batch.observeRead(descriptor.Field) {
			t.Fatalf("observe known field %d", descriptor.Field)
		}
	}
	var ledger profilerFtraceEventDiagnosticLedger
	if !ledger.merge(batch) {
		t.Fatal("merge known descriptor batch")
	}
	out := profilerContainerExtraction{}
	if !ledger.materialize(&out) || len(out.TraceCoverage) != len(profilerFtraceEventDescriptorList) {
		t.Fatalf("known descriptor coverage shape drifted: coverage=%d want=%d", len(out.TraceCoverage), len(profilerFtraceEventDescriptorList))
	}
	for _, descriptor := range profilerFtraceEventDescriptorList {
		coverage, entries := profilerEventCoverageByField(out, descriptor.Field)
		if entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != 0 {
			t.Fatalf("known field %d did not materialize exactly once: entries=%d coverage=%+v", descriptor.Field, entries, coverage)
		}
	}
}

func TestProfilerEventContainerAggregatesKnownFieldAcrossFrames(t *testing.T) {
	payload := syntheticProfilerEventResult(2003, protoPayload(protoVarint(1, 1_200_000), protoVarint(2, 0)))
	message := syntheticProfilerPluginData("ftrace-plugin", payload)
	for _, frameCount := range []int{1, 4_096} {
		t.Run(strconv.Itoa(frameCount), func(t *testing.T) {
			extracted, sink := extractProfilerCensusFixture(t, profilerRepeatedDiagnosticFrameFixture(message, frameCount))
			defer sink.cleanup()
			coverage, entries := profilerEventCoverageByField(extracted, 2003)
			if entries != 1 || coverage.RowsRead != frameCount || coverage.RowsEmitted != frameCount || coverage.Skipped != "" ||
				extracted.StructuredRows != frameCount || extracted.UnsupportedFtrace != 0 ||
				len(extracted.pairPublishers) != 0 || len(extracted.textMessages) != 0 {
				t.Fatalf("known event aggregation drifted at %d frames: extracted=%+v coverage=%+v entries=%d",
					frameCount, extracted, coverage, entries)
			}
		})
	}
}

func TestProfilerEventClockSetRateFieldsRemainDistinctSlots(t *testing.T) {
	field410 := syntheticProfilerPluginData("ftrace-plugin", syntheticProfilerEventResult(410,
		protoPayload(protoBytes(1, []byte("cpu_clk")), protoVarint(2, 800_000))))
	field2002 := syntheticProfilerPluginData("ftrace-plugin", syntheticProfilerEventResult(2002,
		protoPayload(protoBytes(1, []byte("cpu_clk")), protoVarint(2, 900_000), protoVarint(3, 2))))
	extracted, sink := extractSyntheticProfilerContainer(t, field410, field2002)
	defer sink.cleanup()
	clk, clkEntries := profilerEventCoverageByField(extracted, 410)
	power, powerEntries := profilerEventCoverageByField(extracted, 2002)
	if clkEntries != 1 || powerEntries != 1 || clk.Table != "clock_set_rate" || power.Table != "clock_set_rate" ||
		clk.RowsEmitted != 1 || power.RowsEmitted != 1 || strings.Contains(clk.FieldSources["schema_profile"], "cpu_id") ||
		!strings.Contains(power.FieldSources["schema_profile"], "cpu_id=3") || clk.FieldSources["cpu_id"] == power.FieldSources["cpu_id"] {
		t.Fatalf("field410/2002 same-name slots merged or crossed schema: clk=%+v power=%+v", clk, power)
	}
}

func TestProfilerEventUnknownFieldsUseOneStableBucket(t *testing.T) {
	fixture := func(reverse bool) []byte {
		messages := make([][]byte, 32)
		for index := range messages {
			value := index
			if reverse {
				value = len(messages) - index - 1
			}
			field := 5_000 + value
			messages[index] = syntheticProfilerPluginData("ftrace-plugin", syntheticProfilerEventResult(field, protoVarint(1, uint64(value))))
		}
		return syntheticProfilerTraceFile(messages...)
	}
	var baseline string
	for index, reverse := range []bool{false, true} {
		extracted, sink := extractProfilerCensusFixture(t, fixture(reverse))
		coverage, entries := profilerUnknownEventCoverage(extracted)
		sink.cleanup()
		if entries != 1 || coverage.RowsRead != 32 || coverage.RowsEmitted != 0 ||
			coverage.FieldSources["event_field_samples"] == "" || strings.Contains(coverage.Table, "5000") {
			t.Fatalf("unknown fields escaped fixed bucket: entries=%d coverage=%+v", entries, coverage)
		}
		for _, item := range extracted.TraceCoverage {
			if strings.HasPrefix(item.Table, "event_field:") {
				t.Fatalf("container regained dynamic unknown event table: %+v", item)
			}
		}
		if index == 0 {
			baseline = coverage.FieldSources["event_field_samples"]
		} else if coverage.FieldSources["event_field_samples"] != baseline {
			t.Fatalf("unknown field samples depend on frame order:\nforward=%s\nreverse=%s", baseline, coverage.FieldSources["event_field_samples"])
		}
	}

	message := syntheticProfilerPluginData("ftrace-plugin", syntheticProfilerEventResult(9_999, protoVarint(1, 1)))
	extracted, sink := extractProfilerCensusFixture(t, profilerRepeatedDiagnosticFrameFixture(message, 4_096))
	defer sink.cleanup()
	coverage, entries := profilerUnknownEventCoverage(extracted)
	if entries != 1 || coverage.RowsRead != 4_096 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["degraded_unmapped_field_occurrences"] != "4096" ||
		coverage.FieldSources["degraded_unmapped_field_affected_frames"] != "4096" {
		t.Fatalf("same unknown field did not retain fixed exact census: entries=%d coverage=%+v", entries, coverage)
	}
}

func TestProfilerEventAffectedFrameCountIsNotDegradationOccurrenceCount(t *testing.T) {
	var batch profilerFtraceEventBatchCensus
	for _, field := range []int{9_998, 9_999} {
		issue, ok := profilerFtraceEventIssueFromLegacy(field, profilerFtraceEventDegradationUnmappedField,
			"unmapped structured ftrace event field")
		if !ok || !batch.observeRead(field) || !batch.observeIssues(field, false, []profilerFtraceEventIssue{issue}) {
			t.Fatalf("observe degraded field %d", field)
		}
	}
	var ledger profilerFtraceEventDiagnosticLedger
	if !ledger.merge(batch) {
		t.Fatal("merge same-frame degradation batch")
	}
	out := profilerContainerExtraction{}
	if !ledger.materialize(&out) {
		t.Fatal("materialize same-frame degradation batch")
	}
	coverage, entries := profilerUnknownEventCoverage(out)
	if entries != 1 || coverage.FieldSources["degraded_unmapped_field_occurrences"] != "2" ||
		coverage.FieldSources["degraded_unmapped_field_affected_frames"] != "1" {
		t.Fatalf("event degradation occurrence/affected-frame units merged: entries=%d coverage=%+v", entries, coverage)
	}
}

func TestProfilerEventSameClassExactIssuesKeepClassAffectedFrameUnion(t *testing.T) {
	first, ok := profilerFtraceEventIssueFromLegacy(113, profilerFtraceEventDegradationCorePayload,
		"core_field1_wrong_wire")
	if !ok {
		t.Fatal("fixture first exact issue rejected")
	}
	second, ok := profilerFtraceEventIssueFromLegacy(113, profilerFtraceEventDegradationCorePayload,
		"core_field2_duplicate")
	if !ok {
		t.Fatal("fixture second exact issue rejected")
	}
	var ledger profilerFtraceEventDiagnosticLedger
	for frame := 0; frame < 2; frame++ {
		var batch profilerFtraceEventBatchCensus
		if !batch.observeRead(113) || !batch.observeIssues(113, false, []profilerFtraceEventIssue{first, second}) ||
			!ledger.merge(batch) {
			t.Fatalf("merge exact issue frame %d", frame)
		}
	}
	out := profilerContainerExtraction{}
	if !ledger.materialize(&out) {
		t.Fatal("materialize same-class exact issues")
	}
	coverage, entries := profilerEventCoverageByField(out, 113)
	if entries != 1 || coverage.FieldSources["degraded_core_field1_wrong_wire_occurrences"] != "2" ||
		coverage.FieldSources["degraded_core_field1_wrong_wire_affected_frames"] != "2" ||
		coverage.FieldSources["degraded_core_field2_duplicate_occurrences"] != "2" ||
		coverage.FieldSources["degraded_core_field2_duplicate_affected_frames"] != "2" ||
		coverage.FieldSources["degraded_core_payload_occurrences"] != "4" ||
		coverage.FieldSources["degraded_core_payload_affected_frames"] != "2" {
		t.Fatalf("same-class exact/class affected union drifted: entries=%d coverage=%+v", entries, coverage)
	}
}

func TestProfilerEventEnvelopeSlotsRemainSeparate(t *testing.T) {
	var batch profilerFtraceEventBatchCensus
	for field, reason := range map[int]string{
		0:                                    "envelope_oneof_missing",
		profilerFtraceCPUDetailEnvelopeField: "envelope_cpu_detail_malformed_wire",
	} {
		issue, ok := profilerFtraceEventIssueFromLegacy(field, profilerFtraceEventDegradationEnvelope, reason)
		if !ok || !batch.observeRead(field) || !batch.observeIssues(field, false, []profilerFtraceEventIssue{issue}) {
			t.Fatalf("observe envelope field %d", field)
		}
	}
	var ledger profilerFtraceEventDiagnosticLedger
	if !ledger.merge(batch) {
		t.Fatal("merge envelope slots")
	}
	out := profilerContainerExtraction{}
	if !ledger.materialize(&out) {
		t.Fatal("materialize envelope slots")
	}
	eventEnvelope, eventEntries := profilerEventCoverageByField(out, 0)
	detailEnvelope, detailEntries := profilerEventCoverageByField(out, profilerFtraceCPUDetailEnvelopeField)
	if eventEntries != 1 || detailEntries != 1 || eventEnvelope.Table != "__event_envelope__" ||
		detailEnvelope.Table != "__cpu_detail_envelope__" {
		t.Fatalf("event/cpu-detail envelope slots merged: event=%+v detail=%+v", eventEnvelope, detailEnvelope)
	}
}

func TestProfilerEventTypedVerdictRemainsFrameLocal(t *testing.T) {
	unknown := syntheticProfilerPluginData("ftrace-plugin", syntheticProfilerEventResult(9_999, protoVarint(1, 1)))
	clean := syntheticProfilerPluginData("ftrace-plugin", syntheticProfilerEventResult(2003,
		protoPayload(protoVarint(1, 1_200_000), protoVarint(2, 0))))
	extracted, sink := extractSyntheticProfilerContainer(t, unknown, clean)
	defer sink.cleanup()
	plugin, entries := profilerDiagnosticCoverage(extracted, "plugin:ftrace-plugin")
	unknownCoverage, unknownEntries := profilerUnknownEventCoverage(extracted)
	if entries != 1 || unknownEntries != 1 || unknownCoverage.RowsRead != 1 ||
		extracted.StructuredFtrace != 2 || extracted.UnsupportedFtrace != 1 ||
		plugin.FieldSources["outcome_structured_degraded_frames"] != "1" ||
		plugin.FieldSources["outcome_structured_frames"] != "1" {
		t.Fatalf("aggregate event verdict contaminated clean sibling: extracted=%+v plugin=%+v unknown=%+v",
			extracted, plugin, unknownCoverage)
	}
}

func TestProfilerEventDirectUnknownCoverageCompatibility(t *testing.T) {
	result := decodeProfilerTracePluginResult(syntheticProfilerEventResult(9_999, protoVarint(1, 1)))
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredResult(result, &seq, sink)
	if err != nil || rows != 0 {
		t.Fatalf("direct unknown render: rows=%d err=%v coverage=%+v", rows, err, coverage)
	}
	item := coverageForTable(coverage, "event_field:9999")
	if item == nil || item.Skipped != "unmapped structured ftrace event field" ||
		item.FieldSources["degraded_unmapped_field_rows"] != "" {
		t.Fatalf("direct unknown coverage compatibility drifted: %+v", item)
	}
}

func TestProfilerEventCoverageMaterializesBeforeSourceFailClose(t *testing.T) {
	payload := syntheticProfilerEventResult(2003, protoPayload(protoVarint(1, 1_200_000), protoVarint(2, 0)))
	prefix := syntheticProfilerPluginData("ftrace-plugin", payload)
	maxFrame := uint64(len(prefix) + 16)
	oversized := make([]byte, int(maxFrame+1))
	body := profilerResourceTraceFile(
		profilerResourceFrame{declared: uint32(len(prefix)), payload: prefix},
		profilerResourceFrame{declared: uint32(len(oversized)), payload: oversized},
	)
	extracted, sink := extractProfilerResourceTraceFile(t, body, maxFrame)
	defer sink.cleanup()
	coverage, entries := profilerEventCoverageByField(extracted, 2003)
	if !extracted.SourceFailClosed || extracted.SourceFailReason != "plugin_frame_size_budget_exceeded" ||
		entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["profiler_trace_body_source_fail_closed"] != "plugin_frame_size_budget_exceeded" {
		t.Fatalf("event coverage escaped/lost source fail-close: extracted=%+v coverage=%+v", extracted, coverage)
	}
}

func TestProfilerEventHardIssueMaterializesBeforeSourceFailClose(t *testing.T) {
	payload := syntheticProfilerEventResult(2003,
		protoPayload(protoBytes(1, []byte("wrong-wire")), protoVarint(2, 0)))
	prefix := syntheticProfilerPluginData("ftrace-plugin", payload)
	maxFrame := uint64(len(prefix) + 16)
	oversized := make([]byte, int(maxFrame+1))
	body := profilerResourceTraceFile(
		profilerResourceFrame{declared: uint32(len(prefix)), payload: prefix},
		profilerResourceFrame{declared: uint32(len(oversized)), payload: oversized},
	)
	extracted, sink := extractProfilerResourceTraceFile(t, body, maxFrame)
	defer sink.cleanup()
	coverage, entries := profilerEventCoverageByField(extracted, 2003)
	if !extracted.SourceFailClosed || entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["degraded_core_field1_wrong_wire_occurrences"] != "1" ||
		coverage.FieldSources["degraded_core_field1_wrong_wire_affected_frames"] != "1" ||
		coverage.FieldSources["profiler_trace_body_source_fail_closed"] != "plugin_frame_size_budget_exceeded" {
		t.Fatalf("typed hard issue lost before source fail-close: extracted=%+v coverage=%+v", extracted, coverage)
	}
}

func TestProfilerEventIssueCountCorruptionFailsClosedWithoutPanic(t *testing.T) {
	corrupt := profilerFtraceEventSlotCensus{RowsRead: 1, IssueCount: profilerFtraceEventIssuesPerSlot + 1}
	batch := profilerFtraceEventBatchCensus{}
	batch.Slots[profilerFtraceEventSlot(2003)] = corrupt
	var ledger profilerFtraceEventDiagnosticLedger
	if ledger.merge(batch) {
		t.Fatal("corrupt batch issue count merged")
	}
	ledger.Slots[profilerFtraceEventSlot(2003)] = corrupt
	if ledger.materialize(&profilerContainerExtraction{}) {
		t.Fatal("corrupt ledger issue count materialized")
	}
}

func TestProfilerEventDiagnosticStructurePin(t *testing.T) {
	if len(profilerFtraceEventDescriptorList) != 36 || profilerFtraceEventSlotCount != 39 ||
		profilerFtraceEventSlot(410) == profilerFtraceEventSlot(2002) {
		t.Fatalf("fixed event slot authority drifted: descriptors=%d slots=%d 410=%d 2002=%d",
			len(profilerFtraceEventDescriptorList), profilerFtraceEventSlotCount,
			profilerFtraceEventSlot(410), profilerFtraceEventSlot(2002))
	}
	diagnostics, err := parser.ParseFile(token.NewFileSet(), "profiler_ftrace_event_diagnostics.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	issues, err := parser.ParseFile(token.NewFileSet(), "profiler_ftrace_event_issue.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]bool{
		"profilerFtraceEventIssue":            false,
		"profilerFtraceEventIssueCensus":      false,
		"profilerFtraceEventSlotCensus":       false,
		"profilerFtraceEventBatchCensus":      false,
		"profilerFtraceEventDiagnosticLedger": false,
		"profilerFtraceEventCoverageIndexes":  false,
	}
	for _, parsed := range []*ast.File{diagnostics, issues} {
		for _, declaration := range parsed.Decls {
			gen, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, ok := targets[typeSpec.Name.Name]; !ok {
					continue
				}
				targets[typeSpec.Name.Name] = true
				if structType, ok := typeSpec.Type.(*ast.StructType); ok {
					for _, field := range structType.Fields.List {
						for _, name := range field.Names {
							switch name.Name {
							case "Reason", "ReasonSamples", "Degradations":
								t.Fatalf("%s regained legacy %s authority", typeSpec.Name.Name, name.Name)
							}
						}
					}
				}
				ast.Inspect(typeSpec.Type, func(node ast.Node) bool {
					switch value := node.(type) {
					case *ast.MapType:
						t.Fatalf("%s regained retained map", typeSpec.Name.Name)
					case *ast.ArrayType:
						if value.Len == nil {
							t.Fatalf("%s regained retained slice", typeSpec.Name.Name)
						}
					case *ast.Ident:
						if value.Name == "string" {
							t.Fatalf("%s regained retained string authority", typeSpec.Name.Name)
						}
					}
					return true
				})
			}
		}
	}
	for target, found := range targets {
		if !found {
			t.Fatalf("event diagnostic type pin target missing: %s", target)
		}
	}
	for _, parsed := range []*ast.File{diagnostics, issues} {
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			switch function.Name.Name {
			case "observeDegradations", "profilerFtraceEventDegradations":
				t.Fatalf("legacy event degradation helper returned: %s", function.Name.Name)
			}
		}
	}

	container, err := parser.ParseFile(token.NewFileSet(), "profiler_container.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range container.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || (function.Name.Name != "extractProfilerTraceFileAtWithFrameLimit" && function.Name.Name != "reconcileProfilerPairCoverage") {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				name = fun.Name
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			}
			if name == "profilerFtraceCoverageHasSkipped" || name == "withheldPairRowsForTable" {
				t.Fatalf("%s regained string/table-driven event authority via %s", function.Name.Name, name)
			}
			return true
		})
	}
}
