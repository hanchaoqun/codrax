package hitraceconv

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func mustProfilerFtraceIssueForTest(t *testing.T, eventField int, kind profilerFtraceEventIssueKind, payloadField int) profilerFtraceEventIssue {
	t.Helper()
	var (
		issue profilerFtraceEventIssue
		ok    bool
	)
	if profilerFtraceEventIssueParameterizedKind(kind) {
		issue, ok = profilerFtraceEventPayloadIssue(eventField, kind, payloadField)
	} else {
		issue, ok = profilerFtraceEventFixedIssue(eventField, kind)
	}
	if !ok {
		t.Fatalf("fixture issue rejected: event=%d kind=%d payload_field=%d", eventField, kind, payloadField)
	}
	return issue
}

func TestProfilerFtraceEventIssueTypedConstructorLiteralGolden(t *testing.T) {
	tests := []struct {
		name       string
		eventField int
		kind       profilerFtraceEventIssueKind
		field      int
		wantLabel  string
		wantSource profilerFtraceEventDegradationKind
		wantSev    profilerFtraceEventIssueSeverity
		wantField  uint8
	}{
		{name: "event envelope hard reject", eventField: 0, kind: profilerFtraceEventIssueEnvelopeEventMalformedWire,
			wantLabel: "envelope_event_malformed_wire", wantSource: profilerFtraceEventDegradationEnvelope},
		{name: "nested common fields hard reject", eventField: 2003, kind: profilerFtraceEventIssueEnvelopeCommonFieldsWrongWire,
			wantLabel: "envelope_common_fields_wrong_wire", wantSource: profilerFtraceEventDegradationEnvelope, wantField: 50},
		{name: "core scalar hard reject", eventField: 2420, kind: profilerFtraceEventIssueCoreFieldWrongWire, field: 2,
			wantLabel: "core_field2_wrong_wire", wantSource: profilerFtraceEventDegradationCorePayload, wantField: 2},
		{name: "core display admitted", eventField: 2420, kind: profilerFtraceEventIssueCoreDisplayCommWrongWire,
			wantLabel: "display_comm_wrong_wire", wantSource: profilerFtraceEventDegradationCoreDisplay,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 1},
		{name: "mmc display admitted", eventField: 4015, kind: profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, field: 7,
			wantLabel: "drop_response_field7_out_of_source_profile", wantSource: profilerFtraceEventDegradationAuxDisplay,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 7},
		{name: "block whole payload hard reject", eventField: 202, kind: profilerFtraceEventIssueBlockPayloadMalformedWire,
			wantLabel: "block_payload_malformed_wire", wantSource: profilerFtraceEventDegradationBlockPayload},
		{name: "block canonical hard reject", eventField: 210, kind: profilerFtraceEventIssueBlockInvalidCanonicalLine,
			wantLabel: "invalid_canonical_block_line", wantSource: profilerFtraceEventDegradationBlockPayload},
		{name: "block display admitted", eventField: 211, kind: profilerFtraceEventIssueBlockCmdUnsafeOmitted,
			wantLabel: "cmd_unsafe_omitted", wantSource: profilerFtraceEventDegradationBlockDisplay,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 7},
		{name: "generic cpu field audit admitted", eventField: 2002, kind: profilerFtraceEventIssueWireCPUIDWrongWire,
			wantLabel: "cpu_id_wrong_wire", wantSource: profilerFtraceEventDegradationFieldAudit,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 3},
		{name: "generic clock scalar hard reject", eventField: 410, kind: profilerFtraceEventIssueWireFieldWrongWire, field: 1,
			wantLabel: "core_field1_wrong_wire", wantSource: profilerFtraceEventDegradationWireAudit, wantField: 1},
		{name: "filemap field hard reject", eventField: 1000, kind: profilerFtraceEventIssueFilemapDeviceInvalid,
			wantLabel: "filemap_device_invalid", wantSource: profilerFtraceEventDegradationFilemapPayload, wantField: 4},
		{name: "unknown event hard reject", eventField: 987654, kind: profilerFtraceEventIssueUnmappedField,
			wantLabel: "unmapped structured ftrace event field", wantSource: profilerFtraceEventDegradationUnmappedField},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issue := mustProfilerFtraceIssueForTest(t, test.eventField, test.kind, test.field)
			if issue.Severity != test.wantSev || issue.PayloadField != test.wantField ||
				issue.sourceClass() != test.wantSource {
				t.Fatalf("typed issue drifted: issue=%+v source=%v want_severity=%v want_field=%d want_source=%v",
					issue, issue.sourceClass(), test.wantSev, test.wantField, test.wantSource)
			}
			if got, ok := issue.label(test.eventField); !ok || got != test.wantLabel {
				t.Fatalf("label=(%q,%t) want=(%q,true) issue=%+v", got, ok, test.wantLabel, issue)
			}
		})
	}
}

func TestProfilerFtraceEventIssueTypedConstructorsKeepSchemaAuthority(t *testing.T) {
	fixedRejects := []struct {
		event int
		kind  profilerFtraceEventIssueKind
	}{
		{410, profilerFtraceEventIssueWireCPUIDWrongWire},
		{202, profilerFtraceEventIssueBlockCommWrongWire},
		{202, profilerFtraceEventIssueBlockInvalidCanonicalLine},
		{4016, profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev},
		{2420, profilerFtraceEventIssueUnmappedField},
		{987654, profilerFtraceEventIssueCoreDisplayCommWrongWire},
		{2420, profilerFtraceEventIssueCoreFieldWrongWire},
	}
	for _, test := range fixedRejects {
		if issue, ok := profilerFtraceEventFixedIssue(test.event, test.kind); ok {
			t.Fatalf("foreign/fixed-invalid tuple admitted: event=%d kind=%d issue=%+v", test.event, test.kind, issue)
		}
	}
	payloadRejects := []struct {
		event int
		kind  profilerFtraceEventIssueKind
		field int
	}{
		{2420, profilerFtraceEventIssueCoreFieldWrongWire, 0},
		{2420, profilerFtraceEventIssueCoreFieldWrongWire, 9},
		{4016, profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, 3},
		{204, profilerFtraceEventIssueBlockFieldMissingOrInvalid, 1},
		{204, profilerFtraceEventIssueBlockFieldOutOfRange, 4},
		{410, profilerFtraceEventIssueWireFieldOutOfRange, 2},
		{410, profilerFtraceEventIssueWirePayloadMalformedWire, 1},
		{2420, profilerFtraceEventIssueCoreFieldWrongWire, 256},
	}
	for _, test := range payloadRejects {
		if issue, ok := profilerFtraceEventPayloadIssue(test.event, test.kind, test.field); ok {
			t.Fatalf("foreign/parameterized-invalid tuple admitted: event=%d kind=%d field=%d issue=%+v",
				test.event, test.kind, test.field, issue)
		}
	}
}

func TestProfilerFtraceEventIssueDisplayFieldWhitelists(t *testing.T) {
	tests := []struct {
		name  string
		event int
		kind  profilerFtraceEventIssueKind
		field int
		want  bool
	}{
		{name: "mmc done response 3", event: 4015, kind: profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, field: 3, want: true},
		{name: "mmc done response 7", event: 4015, kind: profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, field: 7, want: true},
		{name: "mmc done response 11", event: 4015, kind: profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, field: 11, want: true},
		{name: "mmc unlisted response", event: 4015, kind: profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, field: 1},
		{name: "mmc start has no response drop", event: 4016, kind: profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, field: 3},
		{name: "block bio queue comm", event: 204, kind: profilerFtraceEventIssueBlockCommWrongWire, want: true},
		{name: "block rq complete cmd", event: 209, kind: profilerFtraceEventIssueBlockCmdDuplicate, want: true},
		{name: "block rq insert comm", event: 210, kind: profilerFtraceEventIssueBlockCommUnsafeOmitted, want: true},
		{name: "block rq insert cmd", event: 210, kind: profilerFtraceEventIssueBlockCmdWrongWire, want: true},
		{name: "block bio complete has no comm", event: 202, kind: profilerFtraceEventIssueBlockCommWrongWire},
		{name: "block bio queue has no cmd", event: 204, kind: profilerFtraceEventIssueBlockCmdWrongWire},
		{name: "non block event cannot mint block display", event: 4015, kind: profilerFtraceEventIssueBlockCmdWrongWire},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				issue profilerFtraceEventIssue
				ok    bool
			)
			if profilerFtraceEventIssueParameterizedKind(test.kind) {
				issue, ok = profilerFtraceEventPayloadIssue(test.event, test.kind, test.field)
			} else {
				issue, ok = profilerFtraceEventFixedIssue(test.event, test.kind)
			}
			if ok != test.want {
				t.Fatalf("admitted=%t want=%t event=%d kind=%d field=%d issue=%+v", ok, test.want, test.event, test.kind, test.field, issue)
			}
			if ok && issue.Severity != profilerFtraceEventIssueAdmittedDisplay {
				t.Fatalf("display tuple minted severity=%v issue=%+v", issue.Severity, issue)
			}
		})
	}
}

func TestProfilerFtraceEventIssueHardFieldWhitelists(t *testing.T) {
	tests := []struct {
		name  string
		event int
		kind  profilerFtraceEventIssueKind
		field int
		want  bool
	}{
		{name: "binder field range", event: 113, kind: profilerFtraceEventIssueCoreFieldOutOfRange, field: 7, want: true},
		{name: "binder received range is semantic", event: 119, kind: profilerFtraceEventIssueCoreFieldOutOfRange, field: 1},
		{name: "wakeup success range", event: 2420, kind: profilerFtraceEventIssueCoreFieldOutOfRange, field: 4, want: true},
		{name: "wakeup pid range has semantic token", event: 2420, kind: profilerFtraceEventIssueCoreFieldOutOfRange, field: 2},
		{name: "f2fs dev range", event: 4010, kind: profilerFtraceEventIssueAuxFieldOutOfRange, field: 1, want: true},
		{name: "clock name missing", event: 410, kind: profilerFtraceEventIssueWireFieldMissingOrInvalid, field: 1, want: true},
		{name: "clock rate never missing", event: 410, kind: profilerFtraceEventIssueWireFieldMissingOrInvalid, field: 2},
		{name: "block rwbs semantic", event: 204, kind: profilerFtraceEventIssueBlockFieldMissingOrInvalid, field: 4, want: true},
		{name: "block dev never missing", event: 204, kind: profilerFtraceEventIssueBlockFieldMissingOrInvalid, field: 1},
		{name: "block rwbs never range", event: 204, kind: profilerFtraceEventIssueBlockFieldOutOfRange, field: 4},
		{name: "block dev range", event: 204, kind: profilerFtraceEventIssueBlockFieldOutOfRange, field: 1, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issue, ok := profilerFtraceEventPayloadIssue(test.event, test.kind, test.field)
			if ok != test.want {
				t.Fatalf("admitted=%t want=%t event=%d kind=%d field=%d issue=%+v", ok, test.want, test.event, test.kind, test.field, issue)
			}
		})
	}
}

func TestProfilerFtraceEventIssueFixedPayloadFieldGolden(t *testing.T) {
	tests := []struct {
		event int
		kind  profilerFtraceEventIssueKind
		field uint8
	}{
		{0, profilerFtraceEventIssueEnvelopeEventMalformedWire, 0},
		{2003, profilerFtraceEventIssueEnvelopeCommonFieldsWrongWire, 50},
		{2420, profilerFtraceEventIssueCoreDisplayCommWrongWire, 1},
		{4002, profilerFtraceEventIssueCoreDisplayCallerStrInvalid, 4},
		{4015, profilerFtraceEventIssueAuxPayloadMalformedWire, 0},
		{4016, profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer, 24},
		{1000, profilerFtraceEventIssueFilemapDeviceInvalid, 4},
		{202, profilerFtraceEventIssueBlockPayloadMalformedWire, 0},
		{210, profilerFtraceEventIssueBlockInvalidCanonicalLine, 0},
		{210, profilerFtraceEventIssueBlockCommWrongWire, 6},
		{210, profilerFtraceEventIssueBlockCmdWrongWire, 7},
		{2002, profilerFtraceEventIssueWireCPUIDWrongWire, 3},
		{2417, profilerFtraceEventIssueWireNextInfoDuplicate, 8},
		{410, profilerFtraceEventIssueWireInvalidCanonicalLine, 0},
		{9_999, profilerFtraceEventIssueUnmappedField, 0},
	}
	for _, test := range tests {
		issue, ok := profilerFtraceEventFixedIssue(test.event, test.kind)
		if !ok || issue.PayloadField != test.field {
			t.Fatalf("fixed payload field drifted: event=%d kind=%d issue=%+v ok=%t want=%d",
				test.event, test.kind, issue, ok, test.field)
		}
	}
}
func TestProfilerFtraceEventIssueVerdictConservation(t *testing.T) {
	display := mustProfilerFtraceIssueForTest(t, 2002, profilerFtraceEventIssueWireCPUIDWrongWire, 0)
	hard := mustProfilerFtraceIssueForTest(t, 2420, profilerFtraceEventIssueCoreFieldWrongWire, 2)
	coreDisplay := mustProfilerFtraceIssueForTest(t, 2420, profilerFtraceEventIssueCoreDisplayCommWrongWire, 0)
	unknown := mustProfilerFtraceIssueForTest(t, 987654, profilerFtraceEventIssueUnmappedField, 0)

	tests := []struct {
		name        string
		eventField  int
		publishable bool
		issues      []profilerFtraceEventIssue
		want        bool
	}{
		{name: "clean known row publishes", eventField: 2420, publishable: true, want: true},
		{name: "display issue requires published row", eventField: 2002, publishable: true, issues: []profilerFtraceEventIssue{display}, want: true},
		{name: "display issue cannot reject row", eventField: 2002, publishable: false, issues: []profilerFtraceEventIssue{display}, want: false},
		{name: "hard issue requires rejected row", eventField: 2420, publishable: false, issues: []profilerFtraceEventIssue{hard}, want: true},
		{name: "hard issue cannot publish row", eventField: 2420, publishable: true, issues: []profilerFtraceEventIssue{hard}, want: false},
		{name: "mixed verdict cannot publish", eventField: 2420, publishable: true, issues: []profilerFtraceEventIssue{hard, coreDisplay}, want: false},
		{name: "mixed verdict cannot reject", eventField: 2420, publishable: false, issues: []profilerFtraceEventIssue{hard, coreDisplay}, want: false},
		{name: "rejected row needs typed issue", eventField: 2420, publishable: false, want: false},
		{name: "unknown event requires rejection", eventField: 987654, publishable: false, issues: []profilerFtraceEventIssue{unknown}, want: true},
		{name: "unknown event cannot publish", eventField: 987654, publishable: true, issues: []profilerFtraceEventIssue{unknown}, want: false},
		{name: "event envelope cannot clean publish", eventField: 0, publishable: true, want: false},
		{name: "cpu detail envelope cannot clean publish", eventField: profilerFtraceCPUDetailEnvelopeField, publishable: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := profilerFtraceEventIssueVerdictValid(test.eventField, test.publishable, test.issues); got != test.want {
				t.Fatalf("verdict=%t want=%t field=%d publishable=%t issues=%+v", got, test.want, test.eventField, test.publishable, test.issues)
			}
		})
	}
}

func TestProfilerFtraceEventIssueCannotBeRelabeledOrSeverityFlipped(t *testing.T) {
	cpuDisplay := mustProfilerFtraceIssueForTest(t, 2002, profilerFtraceEventIssueWireCPUIDWrongWire, 0)
	if cpuDisplay.validFor(410) {
		t.Fatal("field-2002 CPU issue became valid for same-name field 410")
	}
	if label, labelOK := cpuDisplay.label(410); labelOK || label != "" {
		t.Fatalf("field-2002 CPU issue relabeled for field 410: label=(%q,%t)", label, labelOK)
	}

	tamperedDisplay := cpuDisplay
	tamperedDisplay.Severity = profilerFtraceEventIssueHardReject
	if tamperedDisplay.validFor(2002) {
		t.Fatalf("display issue admitted after hard-reject severity flip: %+v", tamperedDisplay)
	}
	if label, labelOK := tamperedDisplay.label(2002); labelOK || label != "" {
		t.Fatalf("severity-flipped display issue produced label=(%q,%t)", label, labelOK)
	}

	hard := mustProfilerFtraceIssueForTest(t, 2420, profilerFtraceEventIssueCoreFieldWrongWire, 2)
	tamperedHard := hard
	tamperedHard.Severity = profilerFtraceEventIssueAdmittedDisplay
	if tamperedHard.validFor(2420) {
		t.Fatalf("hard issue admitted after display severity flip: %+v", tamperedHard)
	}
}

func TestProfilerFtraceEventTypedEnvelopeFailsClosedWithoutStringFallback(t *testing.T) {
	event := profilerFtraceEventRecord{Field: 2003, EnvelopeIssueCount: 1}
	event.EnvelopeIssues[0] = profilerFtraceEventIssue{
		Kind: profilerFtraceEventIssueKindCount, Severity: profilerFtraceEventIssueHardReject,
	}
	if _, _, ok, reasons := renderProfilerFtraceEventBodyWithAudit(event); ok || len(reasons) != 0 {
		t.Fatalf("invalid typed envelope escaped direct fail-close: ok=%t reasons=%v", ok, reasons)
	}
	_, _, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if ok || len(issues) != 0 || err == nil {
		t.Fatalf("typed choke admitted invalid envelope issue: ok=%t issues=%+v err=%v", ok, issues, err)
	}
	invariant, typed := err.(*traceDBOutputInvariantError)
	if !typed || invariant.Reason != "profiler_event_envelope_issue_invalid" {
		t.Fatalf("typed choke error=%T %v", err, err)
	}
}

func TestProfilerFtraceEnvelopeIssueSetIsCheckedAndCapacityExact(t *testing.T) {
	const (
		maxCPUIssues            = 1
		maxTimestampIssues      = 1
		maxTGIDOrIdentityIssues = 1
		maxCommIssues           = 1
		maxCommonFieldIssues    = 4
		maxOneofIssues          = 1
	)
	if independentMax := maxCPUIssues + maxTimestampIssues + maxTGIDOrIdentityIssues +
		maxCommIssues + maxCommonFieldIssues + maxOneofIssues; independentMax != profilerFtraceEnvelopeIssuesPerEvent {
		t.Fatalf("envelope capacity=%d independent dimensional maximum=%d",
			profilerFtraceEnvelopeIssuesPerEvent, independentMax)
	}
	common := protoMessage(50, protoPayload(
		protoVarint(1, 1), protoVarint(1, 2),
		protoVarint(2, 0), protoVarint(2, math.MaxUint8+1),
		protoVarint(3, 0), protoVarint(3, math.MaxUint8+1),
		protoBytes(4, []byte{1}),
	))
	eventRaw := testProfilerFtraceEnvelopeEvent(
		protoVarint(1, 1), protoVarint(1, 2),
		protoVarint(2, 100), protoVarint(2, 101),
		protoBytes(3, []byte("first")), protoBytes(3, []byte("second")),
		common,
		protoMessage(1501, protoPayload(protoVarint(1, 7), protoVarint(2, 1))),
		protoMessage(1500, protoPayload(protoVarint(1, 8), protoBytes(2, []byte("timer")))),
	)
	detail := protoPayload(protoVarint(1, 1), protoVarint(1, 2), protoMessage(2, eventRaw))
	records, err := decodeProfilerFtraceCPUDetailEvents(detail)
	if err != nil || len(records) != 1 {
		t.Fatalf("decode max issue set: records=%+v err=%v", records, err)
	}
	record := records[0]
	issues, err := record.checkedEnvelopeIssues()
	if err != nil || len(issues) != profilerFtraceEnvelopeIssuesPerEvent || int(record.EnvelopeIssueCount) != len(record.EnvelopeIssues) {
		t.Fatalf("max issue set drifted: count=%d issues=%+v err=%v capacity=%d",
			record.EnvelopeIssueCount, issues, err, len(record.EnvelopeIssues))
	}
	err = record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeTimestampWrongWire)
	invariant, invariantOK := err.(*traceDBOutputInvariantError)
	if err == nil || !invariantOK || invariant.Reason != "profiler_event_envelope_issue_overflow" {
		t.Fatalf("capacity overflow did not fail closed: %T %v", err, err)
	}

	duplicate := profilerFtraceEventRecord{Field: 2003}
	if err := duplicate.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeCommInvalid); err != nil {
		t.Fatal(err)
	}
	err = duplicate.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeCommInvalid)
	invariant, invariantOK = err.(*traceDBOutputInvariantError)
	if err == nil || !invariantOK || invariant.Reason != "profiler_event_envelope_issue_duplicate" {
		t.Fatalf("duplicate issue did not fail closed: %T %v", err, err)
	}

	corruptCount := profilerFtraceEventRecord{Field: 2003, EnvelopeIssueCount: profilerFtraceEnvelopeIssuesPerEvent + 1}
	_, err = corruptCount.checkedEnvelopeIssues()
	invariant, invariantOK = err.(*traceDBOutputInvariantError)
	if err == nil || !invariantOK || invariant.Reason != "profiler_event_envelope_issue_count_invalid" {
		t.Fatalf("corrupt count did not fail closed: %T %v", err, err)
	}
	foreign, ok := profilerFtraceEventFixedIssue(2003, profilerFtraceEventIssueCoreMissingOrInvalidState)
	if !ok {
		t.Fatal("fixture: core issue")
	}
	foreignRecord := profilerFtraceEventRecord{Field: 2003, EnvelopeIssueCount: 1}
	foreignRecord.EnvelopeIssues[0] = foreign
	_, err = foreignRecord.checkedEnvelopeIssues()
	invariant, invariantOK = err.(*traceDBOutputInvariantError)
	if err == nil || !invariantOK || invariant.Reason != "profiler_event_envelope_issue_invalid" {
		t.Fatalf("foreign source entered envelope set: %T %v", err, err)
	}
}

func TestProfilerFtraceEnvelopeProducerTypedStructurePinned(t *testing.T) {
	for _, file := range []string{
		"profiler_ftrace_render.go", "profiler_ftrace_authority.go", "profiler_container.go",
	} {
		source := mustReadRendererSource(t, file)
		if strings.Contains(source, "EnvelopeDegradations") {
			t.Fatalf("%s restored dynamic envelope reason retention", file)
		}
	}
	renderer := mustReadRendererSource(t, "profiler_ftrace_render.go")
	record := sourceBetween(t, renderer, "type profilerFtraceEventRecord struct {", "func (record *profilerFtraceEventRecord) appendEnvelopeIssue(")
	if strings.Contains(record, "[]profilerFtraceEventIssue") ||
		!strings.Contains(record, "EnvelopeIssueCount uint8") ||
		!strings.Contains(record, "[profilerFtraceEnvelopeIssuesPerEvent]profilerFtraceEventIssue") {
		t.Fatalf("event envelope issue set is not fixed-cardinality:\n%s", record)
	}
	producer := sourceBetween(t, renderer, "type profilerFtraceCPUDetailAuthority struct {", "func profilerPairFamiliesFromCPUDetail(")
	for _, forbidden := range []string{
		"[]string", "prefix string", `"envelope_"`, "profilerFtraceEventIssueFromLegacy(",
	} {
		if strings.Contains(producer, forbidden) {
			t.Fatalf("typed envelope producer restored %q:\n%s", forbidden, producer)
		}
	}
	directLoop := sourceBetween(t, renderer, "func renderProfilerFtraceStructuredResultWithEnvelopeCoverageContext(", "type profilerFtraceCPUDetailAuthority struct {")
	if !strings.Contains(directLoop, "renderProfilerFtraceEventBodyWithTypedAuditAndPair(event)") ||
		strings.Contains(directLoop, "renderProfilerFtraceEventBodyWithAudit(event)") {
		t.Fatalf("direct structured entry bypasses typed invariant path:\n%s", directLoop)
	}
	for _, file := range []string{"profiler_ftrace_authority.go", "profiler_ftrace_render.go"} {
		source := mustReadRendererSource(t, file)
		for _, forbidden := range []string{"[]profilerFtraceEventRecord", "eventPayloads", "profilerTracePluginResultEventsContext"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s restored total event materializer %q", file, forbidden)
			}
		}
	}
}

func TestProfilerFtraceEventTypedChokeRejectsOutOfProtobufFieldDomain(t *testing.T) {
	event := profilerFtraceEventRecord{Field: profilerFtraceUnknownEventAggregateField + 1}
	_, _, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if ok || len(issues) != 0 || err == nil {
		t.Fatalf("out-of-domain event field escaped typed choke: ok=%t issues=%+v err=%v", ok, issues, err)
	}
	invariant, typed := err.(*traceDBOutputInvariantError)
	if !typed || invariant.Reason != "profiler_event_field_domain_invalid" {
		t.Fatalf("out-of-domain error=%T %v", err, err)
	}
}

func TestProfilerFtraceEventUnknownRetainsEnvelopeAndUnmappedIssuesInFixedBucket(t *testing.T) {
	event := profilerFtraceEventRecord{Field: 9_999}
	if err := event.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeCPUDuplicate); err != nil {
		t.Fatalf("fixture: append envelope issue: %v", err)
	}
	if _, _, ok, reasons := renderProfilerFtraceEventBodyWithAudit(event); ok ||
		len(reasons) != 1 || reasons[0] != "envelope_cpu_duplicate" {
		t.Fatalf("direct unknown envelope compatibility drifted: ok=%t reasons=%v", ok, reasons)
	}
	_, _, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if err != nil || ok || len(issues) != 2 ||
		issues[0].Kind != profilerFtraceEventIssueEnvelopeCPUDuplicate ||
		issues[1].Kind != profilerFtraceEventIssueUnmappedField {
		t.Fatalf("typed unknown did not retain envelope plus unmapped issues: ok=%t issues=%+v err=%v", ok, issues, err)
	}
	envelope, envelopeOK := profilerFtraceEventFixedIssue(event.Field, profilerFtraceEventIssueEnvelopeCPUDuplicate)
	if !envelopeOK || profilerFtraceEventIssueVerdictValid(event.Field, false, []profilerFtraceEventIssue{envelope}) ||
		!profilerFtraceEventIssueVerdictValid(event.Field, false, issues) {
		t.Fatalf("unknown slot envelope/unmapped conservation drifted: envelope=%+v ok=%t issues=%+v", envelope, envelopeOK, issues)
	}
	var batch profilerFtraceEventBatchCensus
	if !batch.observeRead(event.Field) || !batch.observeIssues(event.Field, false, issues) {
		t.Fatal("observe normalized unknown issue")
	}
	var ledger profilerFtraceEventDiagnosticLedger
	if !ledger.merge(batch) {
		t.Fatal("merge normalized unknown issue")
	}
	out := profilerContainerExtraction{}
	if !ledger.materialize(&out) {
		t.Fatal("materialize normalized unknown issue")
	}
	coverage, entries := profilerUnknownEventCoverage(out)
	if entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["degraded_unmapped_field_occurrences"] != "1" ||
		coverage.FieldSources["degraded_envelope_occurrences"] != "1" ||
		coverage.FieldSources["degraded_envelope_cpu_duplicate_occurrences"] != "1" {
		t.Fatalf("unknown fixed bucket issue drifted: entries=%d coverage=%+v", entries, coverage)
	}
}

func TestProfilerFtraceEventIssueFiniteUniverseHasClosedLabelsAndFitsFixedCensus(t *testing.T) {
	eventFields := []int{profilerFtraceCPUDetailEnvelopeField, 0, 100, 9_999, profilerFtraceUnknownEventAggregateField}
	for _, descriptor := range profilerFtraceEventDescriptorList {
		eventFields = append(eventFields, descriptor.Field)
	}
	seenKinds := [profilerFtraceEventIssueKindCount]bool{}
	labels := map[string]profilerFtraceEventIssue{}
	total := 0
	maxPerEvent := 0
	maxPerEventField := 0
	for _, eventField := range eventFields {
		perEvent := 0
		for kind := profilerFtraceEventIssueKind(0); kind < profilerFtraceEventIssueKindCount; kind++ {
			for payloadField := 0; payloadField <= 255; payloadField++ {
				for severity := profilerFtraceEventIssueSeverity(0); severity < profilerFtraceEventIssueSeverityCount; severity++ {
					issue := profilerFtraceEventIssue{Kind: kind, PayloadField: uint8(payloadField), Severity: severity}
					if !issue.validFor(eventField) {
						continue
					}
					unknownSlot := profilerFtraceEventSlot(eventField) == profilerFtraceUnknownEventSlot
					if unknownSlot {
						if issue.Kind != profilerFtraceEventIssueUnmappedField &&
							issue.sourceClass() != profilerFtraceEventDegradationEnvelope {
							continue
						}
					} else {
						publishable := severity == profilerFtraceEventIssueAdmittedDisplay
						if !profilerFtraceEventIssueVerdictValid(eventField, publishable, []profilerFtraceEventIssue{issue}) {
							continue
						}
					}
					label, ok := issue.label(eventField)
					if !ok || label == "" {
						t.Fatalf("valid issue has no label: field=%d issue=%+v", eventField, issue)
					}
					key := fmt.Sprintf("%d/%d/%s", eventField, issue.sourceClass(), label)
					if previous, exists := labels[key]; exists && previous != issue {
						t.Fatalf("typed label collision at %s: first=%+v second=%+v", key, previous, issue)
					}
					labels[key] = issue
					seenKinds[kind] = true
					perEvent++
					total++
				}
			}
		}
		if perEvent > profilerFtraceEventIssuesPerSlot {
			t.Fatalf("field %d legal issue universe=%d exceeds fixed census=%d", eventField, perEvent, profilerFtraceEventIssuesPerSlot)
		}
		if perEvent > maxPerEvent {
			maxPerEvent = perEvent
			maxPerEventField = eventField
		}
	}
	for kind, seen := range seenKinds {
		if !seen {
			t.Fatalf("closed issue kind %d has no legal event tuple", kind)
		}
	}
	// Literal totals pin schema expansion and force an explicit census-capacity
	// review. Update only with the independent event/payload golden matrices.
	if total != 1_814 || maxPerEvent != 105 {
		t.Fatalf("finite issue universe drifted: total=%d want=1814 max_per_event=%d field=%d want=105", total, maxPerEvent, maxPerEventField)
	}
}

func TestProfilerFtraceEventFixedLabelAuthorityIsClosedAndCollisionFree(t *testing.T) {
	wantFixed := 0
	seenLabels := make(map[string]profilerFtraceEventIssueKind)
	for kind := profilerFtraceEventIssueKind(0); kind < profilerFtraceEventIssueKindCount; kind++ {
		label, present := profilerFtraceEventFixedIssueLabels[kind]
		if profilerFtraceEventIssueParameterizedKind(kind) {
			if present {
				t.Fatalf("parameterized kind entered fixed label authority: kind=%d label=%q", kind, label)
			}
			continue
		}
		wantFixed++
		if !present || label == "" {
			t.Fatalf("fixed kind lacks label authority: kind=%d label=%q present=%t", kind, label, present)
		}
		if previous, duplicate := seenLabels[label]; duplicate {
			t.Fatalf("fixed label collision: label=%q first_kind=%d second_kind=%d", label, previous, kind)
		}
		seenLabels[label] = kind
	}
	if len(profilerFtraceEventFixedIssueLabels) != wantFixed {
		t.Fatalf("fixed label authority size=%d want=%d", len(profilerFtraceEventFixedIssueLabels), wantFixed)
	}
}

func TestProfilerFtraceCoreCanonicalIssueHasExactSourceReachability(t *testing.T) {
	reachable := map[int]bool{1400: true, 1401: true, 1402: true, 1500: true}
	for _, test := range profilerCoreTestCases() {
		issue, fixedOK := profilerFtraceEventFixedIssue(test.field, profilerFtraceEventIssueCoreInvalidCanonicalLine)
		if fixedOK != reachable[test.field] {
			t.Fatalf("canonical reachability drifted: field=%d fixed_ok=%t want=%t fixed=%+v",
				test.field, fixedOK, reachable[test.field], issue)
		}
		if !reachable[test.field] {
			continue
		}
		if issue.PayloadField != 0 ||
			issue.Severity != profilerFtraceEventIssueHardReject || !issue.validFor(test.field) {
			t.Fatalf("reachable canonical tuple invalid: field=%d fixed=%+v", test.field, issue)
		}
	}
}
