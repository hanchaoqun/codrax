package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilerFtraceEnvelopePreservesCPUFlagsPreemptAndUnknownTGID(t *testing.T) {
	knownTGID := testProfilerFtraceEnvelopeEvent(
		protoVarint(1, 0),
		protoVarint(2, 100),
		protoBytes(3, []byte("worker-v1")),
		testProfilerCommonFields(90, 0x0d, 2, 100),
		protoMessage(1501, protoPayload(protoVarint(1, 7), protoVarint(2, 1))),
	)
	unknownTGID := testProfilerFtraceEnvelopeEvent(
		protoVarint(1, 1_000),
		// FtraceParser only sets TGID when its process map resolves it. A
		// missing proto3 scalar is the honest unknown value, not TGID=TID.
		protoBytes(3, []byte("worker-v2")),
		testProfilerCommonFields(90, 0x10, 3, 101),
		protoMessage(1500, protoPayload(protoVarint(1, 8), protoBytes(2, []byte("timer")))),
	)
	structured := protoMessage(2,
		// Event-before-CPU is legal protobuf ordering. The decoder must audit
		// the complete message before binding either event to CPU 0.
		protoMessage(2, knownTGID),
		protoMessage(2, unknownTGID),
		protoVarint(1, 0),
	)
	sink, err := newTraceDBRowSink(t.TempDir(), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(structured, &seq, sink)
	if err != nil || rows != 2 || len(sink.rows) != 2 {
		t.Fatalf("render structured envelope: rows=%d sink=%+v coverage=%+v err=%v", rows, sink.rows, coverage, err)
	}
	first := sink.rows[0].line
	second := sink.rows[1].line
	for _, want := range []string{"worker-v1", "-100", "(  100)", "[000] dnh2", "0.000000", "irq_handler_exit"} {
		if !strings.Contains(first, want) {
			t.Fatalf("typed structured header lost %q:\n%s", want, first)
		}
	}
	for _, want := range []string{"worker-v2", "-101", "(-----)", "[000] ..s3", "0.000001", "irq_handler_entry"} {
		if !strings.Contains(second, want) {
			t.Fatalf("unknown-TGID structured header lost %q:\n%s", want, second)
		}
	}
	if strings.Contains(second, "(  101)") || strings.Contains(second, "(    0)") {
		t.Fatalf("unknown TGID was fabricated from TID or rendered as process zero:\n%s", second)
	}
}

func TestProfilerFtraceEnvelopeRejectsAmbiguousWireAndKeepsSibling(t *testing.T) {
	validPayload := protoMessage(1501, protoPayload(protoVarint(1, 7), protoVarint(2, 1)))
	validCommon := testProfilerCommonFields(90, 0, 0, 100)
	valid := func() []byte {
		return testProfilerFtraceEnvelopeEvent(
			protoVarint(1, 1_000), protoVarint(2, 100), protoBytes(3, []byte("valid")), validCommon, validPayload,
		)
	}
	tests := []struct {
		name   string
		event  []byte
		reason string
	}{
		{name: "timestamp wrong wire", event: testProfilerFtraceEnvelopeEvent(protoBytes(1, []byte{1}), protoVarint(2, 100), protoBytes(3, []byte("x")), validCommon, validPayload), reason: "envelope_timestamp_wrong_wire"},
		{name: "timestamp duplicate", event: testProfilerFtraceEnvelopeEvent(protoPayload(protoVarint(1, 1), protoVarint(1, 2)), protoVarint(2, 100), protoBytes(3, []byte("x")), validCommon, validPayload), reason: "envelope_timestamp_duplicate"},
		{name: "timestamp output overflow", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, math.MaxUint64), protoVarint(2, 100), protoBytes(3, []byte("x")), validCommon, validPayload), reason: "envelope_timestamp_out_of_range"},
		{name: "tgid wrong wire", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoBytes(2, []byte{1}), protoBytes(3, []byte("x")), validCommon, validPayload), reason: "envelope_tgid_wrong_wire"},
		{name: "tgid duplicate", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoPayload(protoVarint(2, 100), protoVarint(2, 101)), protoBytes(3, []byte("x")), validCommon, validPayload), reason: "envelope_tgid_duplicate"},
		{name: "tgid out of range", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, math.MaxInt32+1), protoBytes(3, []byte("x")), validCommon, validPayload), reason: "envelope_tgid_out_of_range"},
		{name: "comm wrong wire", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), protoVarint(3, 1), validCommon, validPayload), reason: "envelope_comm_wrong_wire"},
		{name: "comm duplicate", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), protoPayload(protoBytes(3, []byte("a")), protoBytes(3, []byte("b"))), validCommon, validPayload), reason: "envelope_comm_duplicate"},
		{name: "comm physical line injection", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), protoBytes(3, []byte("a\nb")), validCommon, validPayload), reason: "envelope_comm_invalid"},
		{name: "common missing", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), nil, nil, nil, validPayload), reason: "envelope_common_fields_missing"},
		{name: "common wrong wire", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), nil, nil, protoVarint(50, 1), validPayload), reason: "envelope_common_fields_wrong_wire"},
		{name: "common duplicate", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), nil, nil, protoPayload(validCommon, validCommon), validPayload), reason: "envelope_common_fields_duplicate"},
		{name: "common pid wrong wire", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, protoMessage(50, protoBytes(4, []byte{1})), validPayload), reason: "envelope_common_pid_wrong_wire"},
		{name: "common pid duplicate", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, protoMessage(50, protoPayload(protoVarint(4, 100), protoVarint(4, 101))), validPayload), reason: "envelope_common_pid_duplicate"},
		{name: "common pid out of range", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, protoMessage(50, protoVarint(4, math.MaxInt32+1)), validPayload), reason: "envelope_common_pid_out_of_range"},
		{name: "common type source width", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, testProfilerCommonFields(math.MaxUint16+1, 0, 0, 100), validPayload), reason: "envelope_common_type_source_width"},
		{name: "common flags source width", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, testProfilerCommonFields(1, math.MaxUint8+1, 0, 100), validPayload), reason: "envelope_common_flags_source_width"},
		{name: "common preempt source width", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, testProfilerCommonFields(1, 0, math.MaxUint8+1, 100), validPayload), reason: "envelope_common_preempt_count_source_width"},
		{name: "common malformed", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), nil, nil, protoMessage(50, []byte{0x80}), validPayload), reason: "envelope_common_fields_malformed_wire"},
		{name: "oneof missing", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, validCommon), reason: "envelope_oneof_missing"},
		{name: "oneof wrong wire", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, validCommon, protoVarint(1501, 1)), reason: "envelope_oneof_wrong_wire"},
		{name: "oneof multiple types", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, validCommon, validPayload, protoMessage(1500, protoVarint(1, 8))), reason: "envelope_oneof_multiple"},
		{name: "oneof duplicate same type", event: testProfilerFtraceEnvelopeEvent(protoVarint(1, 1), protoVarint(2, 100), nil, validCommon, validPayload, validPayload), reason: "envelope_oneof_multiple"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := decodeProfilerFtraceEventRecord(0, tt.event)
			if err != nil {
				t.Fatal(err)
			}
			if !stringSliceContains(record.EnvelopeDegradations, tt.reason) {
				t.Fatalf("degradations=%v, want %q", record.EnvelopeDegradations, tt.reason)
			}
			if _, _, known, got := renderProfilerFtraceEventBodyWithAudit(record); known || !stringSliceContains(got, tt.reason) {
				t.Fatalf("invalid envelope escaped body audit: known=%v degradations=%v", known, got)
			}
		})
	}

	invalid := testProfilerFtraceEnvelopeEvent(
		protoVarint(1, 500), protoVarint(2, 100), protoBytes(3, []byte("bad")), validCommon,
		validPayload, protoMessage(1500, protoVarint(1, 8)),
	)
	structured := protoMessage(2,
		protoVarint(1, 2),
		protoMessage(2, invalid),
		protoMessage(2, valid()),
	)
	sink, err := newTraceDBRowSink(t.TempDir(), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(structured, &seq, sink)
	if err != nil || rows != 1 || len(sink.rows) != 1 || !strings.Contains(sink.rows[0].line, "valid") {
		t.Fatalf("bad sibling killed or contaminated valid row: rows=%d sink=%+v coverage=%+v err=%v", rows, sink.rows, coverage, err)
	}
	envelopeCoverage := coverageForTable(coverage, "__event_envelope__")
	if envelopeCoverage == nil || envelopeCoverage.RowsRead != 1 || envelopeCoverage.RowsEmitted != 0 ||
		!strings.Contains(envelopeCoverage.Skipped, "envelope_oneof_multiple=1") {
		t.Fatalf("invalid oneof coverage missing: %+v", envelopeCoverage)
	}
}

func TestProfilerFtraceCPUEnvelopeStrictPresenceAndOrdering(t *testing.T) {
	event := testProfilerFtraceEnvelopeEvent(
		protoVarint(1, 1), protoVarint(2, 100), protoBytes(3, []byte("worker")),
		testProfilerCommonFields(90, 0, 0, 100), protoMessage(1501, protoPayload(protoVarint(1, 7), protoVarint(2, 1))),
	)
	tests := []struct {
		name    string
		detail  []byte
		cpu     int64
		reason  string
		records int
	}{
		{name: "absent is exact zero", detail: protoMessage(2, event), cpu: 0, records: 1},
		{name: "explicit zero", detail: protoPayload(protoVarint(1, 0), protoMessage(2, event)), cpu: 0, records: 1},
		{name: "event before maximum cpu", detail: protoPayload(protoMessage(2, event), protoVarint(1, uint64(maxTraceDBCPUIndex))), cpu: maxTraceDBCPUIndex, records: 1},
		{name: "wrong wire", detail: protoPayload(protoBytes(1, []byte{1}), protoMessage(2, event)), reason: "envelope_cpu_wrong_wire", records: 1},
		{name: "duplicate", detail: protoPayload(protoVarint(1, 1), protoVarint(1, 2), protoMessage(2, event)), reason: "envelope_cpu_duplicate", records: 1},
		{name: "out of range", detail: protoPayload(protoVarint(1, uint64(maxTraceDBCPUIndex+1)), protoMessage(2, event)), reason: "envelope_cpu_out_of_range", records: 1},
		{name: "invalid cpu without event remains auditable", detail: protoVarint(1, uint64(maxTraceDBCPUIndex+1)), reason: "envelope_cpu_out_of_range", records: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, err := decodeProfilerFtraceCPUDetailEvents(tt.detail)
			if err != nil || len(records) != tt.records {
				t.Fatalf("records=%+v err=%v", records, err)
			}
			if tt.reason == "" {
				if records[0].CPU != tt.cpu || len(records[0].EnvelopeDegradations) != 0 {
					t.Fatalf("valid CPU profile changed: %+v", records[0])
				}
			} else if !stringSliceContains(records[0].EnvelopeDegradations, tt.reason) {
				t.Fatalf("CPU degradation=%v want=%q", records[0].EnvelopeDegradations, tt.reason)
			}
		})
	}
}

func TestDirectFtraceCommonEnvelopeFailsClosedAndPreservesUnknownBodyHeader(t *testing.T) {
	baseFormat := testDirectEnvelopeFormat("irq_handler_exit")
	content := make([]byte, 16)
	binary.LittleEndian.PutUint16(content[0:2], 90)
	content[2] = 0x0d
	content[3] = 2
	binary.LittleEndian.PutUint32(content[4:8], 100)
	binary.LittleEndian.PutUint32(content[8:12], 7)
	binary.LittleEndian.PutUint32(content[12:16], 1)
	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "worker-v1"}, tgids: map[int]int{100: 0}}, 1_000, 2, baseFormat, content)
	if !known || !strings.Contains(line, "worker-v1") || !strings.Contains(line, "-100") || !strings.Contains(line, "(-----)") || !strings.Contains(line, "[002] dnh2") {
		t.Fatalf("valid direct envelope changed: known=%v line=%q", known, line)
	}
	line2, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "worker-v2\nforged"}}, 1_000, 2, baseFormat, content)
	if !known || strings.Contains(line2, "forged") || !strings.Contains(line2, "<...>-100") || !strings.Contains(line2, "(-----)") {
		t.Fatalf("display-only name altered identity or escaped line validation: known=%v line=%q", known, line2)
	}
	idleContent := append([]byte(nil), content...)
	binary.LittleEndian.PutUint32(idleContent[4:8], 0)
	idleLine, idleKnown := renderEventLine(renderContext{tgids: map[int]int{0: 123}}, 1_000, 2, baseFormat, idleContent)
	if !idleKnown || !strings.Contains(idleLine, "<idle>-0") || !strings.Contains(idleLine, "(-----)") || strings.Contains(idleLine, "(  123)") {
		t.Fatalf("idle PID0 inherited an impossible TGID mapping: known=%v line=%q", idleKnown, idleLine)
	}

	tests := []struct {
		name   string
		format eventFormat
		body   []byte
	}{
		{name: "missing pid", format: testDirectEnvelopeWithout(baseFormat, "common_pid"), body: content},
		{name: "missing flags", format: testDirectEnvelopeWithout(baseFormat, "common_flags"), body: content},
		{name: "missing preempt", format: testDirectEnvelopeWithout(baseFormat, "common_preempt_count"), body: content},
		{name: "duplicate pid", format: testDirectEnvelopeAppend(baseFormat, eventField{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true}), body: content},
		{name: "duplicate flags", format: testDirectEnvelopeAppend(baseFormat, eventField{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1}), body: content},
		{name: "duplicate preempt", format: testDirectEnvelopeAppend(baseFormat, eventField{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1}), body: content},
		{name: "pid wrong type", format: testDirectEnvelopeMutate(baseFormat, "common_pid", func(field *eventField) { field.Type = "char" }), body: content},
		{name: "pid wrong width", format: testDirectEnvelopeMutate(baseFormat, "common_pid", func(field *eventField) { field.Size = 2 }), body: content},
		{name: "pid wrong sign", format: testDirectEnvelopeMutate(baseFormat, "common_pid", func(field *eventField) { field.Signed = false }), body: content},
		{name: "pid wrong offset", format: testDirectEnvelopeMutate(baseFormat, "common_pid", func(field *eventField) { field.Offset = 5 }), body: content},
		{name: "flags wrong type", format: testDirectEnvelopeMutate(baseFormat, "common_flags", func(field *eventField) { field.Type = "unsigned int" }), body: content},
		{name: "flags wrong width", format: testDirectEnvelopeMutate(baseFormat, "common_flags", func(field *eventField) { field.Size = 2 }), body: content},
		{name: "flags wrong sign", format: testDirectEnvelopeMutate(baseFormat, "common_flags", func(field *eventField) { field.Signed = true }), body: content},
		{name: "preempt wrong offset", format: testDirectEnvelopeMutate(baseFormat, "common_preempt_count", func(field *eventField) { field.Offset = 2 }), body: content},
		{name: "truncated pid bytes", format: baseFormat, body: content[:7]},
		{name: "negative pid", format: baseFormat, body: func() []byte {
			b := append([]byte(nil), content...)
			binary.LittleEndian.PutUint32(b[4:8], math.MaxUint32)
			return b
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, known := renderEventLine(renderContext{}, 1_000, 2, tt.format, tt.body)
			if known || line != "" {
				t.Fatalf("invalid common envelope minted a row: known=%v line=%q", known, line)
			}
			if header := renderEventHeaderLine(renderContext{}, 1_000, 2, tt.format, tt.body); header != "" {
				t.Fatalf("invalid common envelope fell back to header-only PID0: %q", header)
			}
		})
	}

	unknownFormat := baseFormat
	unknownFormat.Name = "vendor_unknown"
	unknownLine, unknownKnown := renderEventLine(renderContext{}, 1_000, 2, unknownFormat, content)
	if unknownKnown || unknownLine == "" || renderEventHeaderLine(renderContext{}, 1_000, 2, unknownFormat, content) == "" {
		t.Fatalf("valid envelope with unknown body must retain header-only compatibility: known=%v line=%q", unknownKnown, unknownLine)
	}
}

func TestDirectFtraceDescriptorBoundsCannotPanicOrPoisonValidSibling(t *testing.T) {
	format := testDirectEnvelopeFormat("irq_handler_exit")
	maxInt := int(^uint(0) >> 1)
	format.Fields = append(format.Fields,
		eventField{Type: "unsigned long", Name: "overflow", Offset: maxInt, Size: 2},
		eventField{Type: "unsigned long", Name: "huge", Offset: maxInt - 1, Size: 1},
	)
	content := make([]byte, 16)
	binary.LittleEndian.PutUint16(content[0:2], 90)
	binary.LittleEndian.PutUint32(content[4:8], 100)
	binary.LittleEndian.PutUint32(content[8:12], 7)
	binary.LittleEndian.PutUint32(content[12:16], 1)
	line, known := renderEventLine(renderContext{}, 1_000, 2, format, content)
	if !known || !strings.Contains(line, "irq_handler_exit: irq=7 ret=handled") {
		t.Fatalf("out-of-range descriptor poisoned a valid sibling: known=%v line=%q", known, line)
	}
	decoded := decodeEvent(format, content)
	if hasField(decoded, "overflow") || hasField(decoded, "huge") {
		t.Fatal("out-of-range descriptor was materialized")
	}
	if rebuilt := eventContent(decoded); len(rebuilt) != len(content) {
		t.Fatalf("safe event-content reconstruction changed valid bytes or followed poison offsets: len=%d", len(rebuilt))
	}
}

func TestDirectFtraceTimestampZeroRemainsFirstResultTimestamp(t *testing.T) {
	formatText := strings.Join([]string{
		"name: irq_handler_exit",
		"ID: 90",
		"format:",
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;",
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;",
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;",
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;",
		"\tfield:int irq;\toffset:8;\tsize:4;\tsigned:1;",
		"\tfield:int ret;\toffset:12;\tsize:4;\tsigned:1;",
		`print fmt: "irq=%d ret=%s"`,
		"",
	}, "\n")
	content := make([]byte, 16)
	binary.LittleEndian.PutUint32(content[4:8], 100)
	binary.LittleEndian.PutUint32(content[8:12], 7)
	binary.LittleEndian.PutUint32(content[12:16], 1)
	page := syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 90, OffsetNS: 0, Content: content},
		{EventID: 90, OffsetNS: 1_000, Content: content},
	})
	binary.LittleEndian.PutUint64(page[0:8], 0)
	var binaryTrace bytes.Buffer
	writeFileHeader(&binaryTrace, 1)
	writeSegment(&binaryTrace, segmentEventsFormat, []byte(formatText))
	writeSegment(&binaryTrace, segmentRawTrace, page)
	dir := t.TempDir()
	input := filepath.Join(dir, "timestamp-zero.sys")
	if err := os.WriteFile(input, binaryTrace.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 2 || result.FirstTimestampSec != 0 || result.LastTimestampSec != 0.000001 {
		t.Fatalf("timestamp zero was treated as an uninitialized sentinel: %+v", result)
	}
}

func TestDirectFtraceInvalidCommonEnvelopeIsSuppressedWithCaveat(t *testing.T) {
	formatText := strings.Join([]string{
		"name: sched_wakeup",
		"ID: 10",
		"format:",
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;",
		// common_flags is deliberately absent.
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;",
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;",
		"\tfield:char comm[16];\toffset:8;\tsize:16;\tsigned:0;",
		"\tfield:int pid;\toffset:24;\tsize:4;\tsigned:1;",
		"\tfield:int prio;\toffset:28;\tsize:4;\tsigned:1;",
		"\tfield:int target_cpu;\toffset:32;\tsize:4;\tsigned:1;",
		`print fmt: "comm=%s pid=%d prio=%d target_cpu=%03d"`,
		"",
	}, "\n")
	var binaryTrace bytes.Buffer
	writeFileHeader(&binaryTrace, 1)
	writeSegment(&binaryTrace, segmentEventsFormat, []byte(formatText))
	writeSegment(&binaryTrace, segmentRawTrace, syntheticRawPageForEventID(10))
	dir := t.TempDir()
	input := filepath.Join(dir, "synthetic-invalid-envelope.sys")
	if err := os.WriteFile(input, binaryTrace.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 0 || result.UnknownEventCount != 0 || !strings.Contains(strings.Join(result.Caveats, " "), "suppressed without fabricating an idle/PID0 header") {
		t.Fatalf("invalid raw envelope was silent or became header-only: %+v", result)
	}
	body, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "sched_wakeup:") || strings.Contains(string(body), "<idle>-0") {
		t.Fatalf("invalid raw envelope escaped to output:\n%s", body)
	}
}

func TestFtraceHeaderCanonicalPIDWidthDirectAndStructured(t *testing.T) {
	for _, test := range []struct {
		name string
		pid  uint32
		want string
	}{
		{
			name: "five digit pid",
			pid:  17267,
			want: "            task-17267 (17267) [002] dnh2     0.000001: irq_handler_exit: irq=7 ret=handled",
		},
		{
			name: "six digit pid keeps separator",
			pid:  123456,
			want: "            task-123456 (123456) [002] dnh2     0.000001: irq_handler_exit: irq=7 ret=handled",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			format := testDirectEnvelopeFormat("irq_handler_exit")
			content := make([]byte, 16)
			binary.LittleEndian.PutUint16(content[0:2], 90)
			content[2] = 0x0d
			content[3] = 2
			binary.LittleEndian.PutUint32(content[4:8], test.pid)
			binary.LittleEndian.PutUint32(content[8:12], 7)
			binary.LittleEndian.PutUint32(content[12:16], 1)
			ctx := renderContext{
				cmdlines: map[int]string{int(test.pid): "task"},
				tgids:    map[int]int{int(test.pid): int(test.pid)},
			}
			direct, known := renderEventLine(ctx, 1_000, 2, format, content)
			if !known || direct != test.want {
				t.Fatalf("direct canonical header mismatch:\n got %q\nwant %q", direct, test.want)
			}
			structured, err := prepareTraceDBRenderedRowWithTraceFlags(1_000, 0, "task", int64(test.pid), int64(test.pid), 2, 0x0d, 2,
				"irq_handler_exit: irq=7 ret=handled")
			if err != nil || structured.line != test.want {
				t.Fatalf("structured canonical header mismatch: err=%v\n got %q\nwant %q", err, structured.line, test.want)
			}
		})
	}
}

func TestFtraceHeaderDirectStructuredLongCommParityAndDisplayOnlyRename(t *testing.T) {
	format := testDirectEnvelopeFormat("irq_handler_exit")
	content := make([]byte, 16)
	binary.LittleEndian.PutUint16(content[0:2], 90)
	content[2] = 0x0d
	content[3] = 2
	binary.LittleEndian.PutUint32(content[4:8], 100)
	binary.LittleEndian.PutUint32(content[8:12], 7)
	binary.LittleEndian.PutUint32(content[12:16], 1)
	for _, test := range []struct {
		name string
		comm string
		want string
	}{
		{name: "ASCII", comm: "abcdefghijklmnop-renamed", want: "abcdefghijklmno"},
		{name: "CJK", comm: "甲乙丙丁戊己庚辛壬癸子丑寅卯辰巳", want: "甲乙丙丁戊己庚辛壬癸子丑寅卯辰"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := renderContext{cmdlines: map[int]string{100: test.comm}, tgids: map[int]int{100: 100}}
			direct, known := renderEventLine(ctx, 1_000, 2, format, content)
			structured, err := prepareTraceDBRenderedRowWithTraceFlags(1_000, 0, test.comm, 100, 100, 2, 0x0d, 2,
				"irq_handler_exit: irq=7 ret=handled")
			if !known || err != nil || direct != structured.line {
				t.Fatalf("direct/structured long-comm parity: known=%v err=%v\ndirect=%q\nstructured=%q", known, err, direct, structured.line)
			}
			if !strings.Contains(direct, test.want+"-100") || strings.Contains(direct, test.comm+"-100") {
				t.Fatalf("comm was not clamped to 15 runes: %q", direct)
			}
		})
	}

	lineA, _ := renderEventLine(renderContext{cmdlines: map[int]string{100: "first-name"}, tgids: map[int]int{100: 100}}, 1_000, 2, format, content)
	lineB, _ := renderEventLine(renderContext{cmdlines: map[int]string{100: "second-name"}, tgids: map[int]int{100: 100}}, 1_000, 2, format, content)
	suffixA := lineA[strings.Index(lineA, "-100"):]
	suffixB := lineB[strings.Index(lineB, "-100"):]
	if suffixA != suffixB || suffixA == "" {
		t.Fatalf("same typed PID/TGID changed under display-only rename: A=%q B=%q", lineA, lineB)
	}
}

func TestPerfTraceHeaderCommClampKeepsFullBodyInventory(t *testing.T) {
	for _, test := range []struct {
		name string
		comm string
		want string
	}{
		{name: "ASCII", comm: "abcdefghijklmnop-renamed", want: "abcdefghijklmno"},
		{name: "CJK", comm: "甲乙丙丁戊己庚辛壬癸子丑寅卯辰巳", want: "甲乙丙丁戊己庚辛壬癸子丑寅卯辰"},
	} {
		t.Run(test.name, func(t *testing.T) {
			full := sanitizePerfTraceComm(test.comm)
			header := perfTraceHeaderComm(full)
			if header != test.want || header == full {
				t.Fatalf("perf header comm was not display-clamped: full=%q header=%q", full, header)
			}
			body := "thread_comm=" + quoteTraceValue(full)
			if !strings.Contains(body, full) {
				t.Fatalf("full perf thread inventory was lost from body: %q", body)
			}
		})
	}
}

func testProfilerFtraceEnvelopeEvent(parts ...[]byte) []byte {
	return protoPayload(parts...)
}

func testProfilerCommonFields(eventType, flags, preempt, pid uint64) []byte {
	return protoMessage(50, protoPayload(
		protoVarint(1, eventType),
		protoVarint(2, flags),
		protoVarint(3, preempt),
		protoVarint(4, pid),
	))
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func testDirectEnvelopeFormat(name string) eventFormat {
	return eventFormat{ID: 90, Name: name, Fields: []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
		{Type: "int", Name: "irq", Offset: 8, Size: 4, Signed: true},
		{Type: "int", Name: "ret", Offset: 12, Size: 4, Signed: true},
	}}
}

func testDirectEnvelopeWithout(format eventFormat, name string) eventFormat {
	out := format
	out.Fields = nil
	for _, field := range format.Fields {
		if field.Name != name {
			out.Fields = append(out.Fields, field)
		}
	}
	return out
}

func testDirectEnvelopeAppend(format eventFormat, field eventField) eventFormat {
	out := format
	out.Fields = append(append([]eventField(nil), format.Fields...), field)
	return out
}

func testDirectEnvelopeMutate(format eventFormat, name string, mutate func(*eventField)) eventFormat {
	out := format
	out.Fields = append([]eventField(nil), format.Fields...)
	for i := range out.Fields {
		if out.Fields[i].Name == name {
			mutate(&out.Fields[i])
		}
	}
	return out
}
