package hitraceconv

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

type profilerGenericWireTypedTestResult struct {
	name   string
	body   string
	ok     bool
	issues []profilerFtraceEventIssue
	labels []string
}

func profilerGenericWireTestRecord(field int, payload []byte) profilerFtraceEventRecord {
	return profilerFtraceEventRecord{
		CPU: 2, TSNS: 1_000, TGID: 100, PID: 100, HeaderOwnerKnown: true,
		Comm: "wire-test", Field: field, Payload: payload,
	}
}

func profilerGenericWireRenderTypedForTest(t *testing.T, field int, payload []byte) profilerGenericWireTypedTestResult {
	t.Helper()
	record := profilerGenericWireTestRecord(field, payload)
	name, body, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(record)
	if err != nil {
		t.Fatalf("typed generic render field=%d: %v", field, err)
	}
	if !profilerFtraceEventIssueVerdictValid(field, ok, issues) {
		t.Fatalf("typed generic verdict invalid: field=%d ok=%t issues=%+v", field, ok, issues)
	}
	labels, labelsOK := profilerFtraceEventIssueLabels(field, issues)
	if !labelsOK {
		t.Fatalf("typed generic issues have no labels: field=%d issues=%+v", field, issues)
	}

	compatName, compatBody, compatOK, compatLabels := renderProfilerFtraceEventBodyWithAudit(record)
	if compatName != name || compatBody != body || compatOK != ok || !reflect.DeepEqual(compatLabels, labels) {
		t.Fatalf("typed/direct compatibility drift: field=%d\n typed=(%q,%q,%t,%v)\ncompat=(%q,%q,%t,%v)",
			field, name, body, ok, labels, compatName, compatBody, compatOK, compatLabels)
	}
	return profilerGenericWireTypedTestResult{name: name, body: body, ok: ok, issues: issues, labels: labels}
}

func requireProfilerGenericWireTypedResult(t *testing.T, field int, payload []byte, wantOK bool,
	wantIssues []profilerFtraceEventIssue, wantLabels []string,
) profilerGenericWireTypedTestResult {
	t.Helper()
	got := profilerGenericWireRenderTypedForTest(t, field, payload)
	issuesEqual := len(got.issues) == len(wantIssues)
	for index := 0; issuesEqual && index < len(got.issues); index++ {
		issuesEqual = got.issues[index] == wantIssues[index]
	}
	labelsEqual := len(got.labels) == len(wantLabels)
	for index := 0; labelsEqual && index < len(got.labels); index++ {
		labelsEqual = got.labels[index] == wantLabels[index]
	}
	if got.ok != wantOK || !issuesEqual || !labelsEqual {
		t.Fatalf("generic typed tuple drift: field=%d ok=%t want_ok=%t\nissues=%+v\nwant=%+v\nlabels=%v want=%v",
			field, got.ok, wantOK, got.issues, wantIssues, got.labels, wantLabels)
	}
	if !wantOK && (got.name != "" || got.body != "") {
		t.Fatalf("hard-rejected generic row retained output: field=%d name=%q body=%q", field, got.name, got.body)
	}
	return got
}

func profilerGenericWireHardIssue(kind profilerFtraceEventIssueKind, payloadField uint8) profilerFtraceEventIssue {
	return profilerFtraceEventIssue{Kind: kind, PayloadField: payloadField, Severity: profilerFtraceEventIssueHardReject}
}

func profilerGenericWireDisplayIssue(kind profilerFtraceEventIssueKind, payloadField uint8) profilerFtraceEventIssue {
	return profilerFtraceEventIssue{Kind: kind, PayloadField: payloadField, Severity: profilerFtraceEventIssueAdmittedDisplay}
}

func profilerGenericWireClockPayload(extra ...[]byte) []byte {
	parts := [][]byte{protoBytes(1, []byte("clk")), protoVarint(2, 2)}
	parts = append(parts, extra...)
	return protoPayload(parts...)
}

func profilerGenericWireSchedPayload(extra ...[]byte) []byte {
	parts := [][]byte{
		protoBytes(1, []byte("prev")), protoVarint(2, 100), protoVarint(3, 120), protoVarint(4, 1),
		protoBytes(5, []byte("next")), protoVarint(6, 101), protoVarint(7, 118),
	}
	parts = append(parts, extra...)
	return protoPayload(parts...)
}

type profilerGenericWireFieldFixture struct {
	field  int
	string bool
	wire   []byte
}

func profilerGenericWireValidFieldFixtures(eventField int) []profilerGenericWireFieldFixture {
	switch eventField {
	case 410:
		return []profilerGenericWireFieldFixture{
			{field: 1, string: true, wire: protoBytes(1, []byte("clk"))},
			{field: 2, wire: protoVarint(2, 2)},
		}
	case 2002:
		return []profilerGenericWireFieldFixture{
			{field: 1, string: true, wire: protoBytes(1, []byte("clk"))},
			{field: 2, wire: protoVarint(2, 2)},
			{field: 3, wire: protoVarint(3, 7)},
		}
	case 2417:
		return []profilerGenericWireFieldFixture{
			{field: 1, string: true, wire: protoBytes(1, []byte("prev"))},
			{field: 2, wire: protoVarint(2, 100)},
			{field: 3, wire: protoVarint(3, 120)},
			{field: 4, wire: protoVarint(4, 1)},
			{field: 5, string: true, wire: protoBytes(5, []byte("next"))},
			{field: 6, wire: protoVarint(6, 101)},
			{field: 7, wire: protoVarint(7, 118)},
			{field: 8, wire: protoVarint(8, 1)},
		}
	default:
		return nil
	}
}

func profilerGenericWirePayloadReplacing(eventField, targetField int, replacement []byte) []byte {
	parts := make([][]byte, 0, 8)
	for _, field := range profilerGenericWireValidFieldFixtures(eventField) {
		if field.field == targetField {
			if replacement != nil {
				parts = append(parts, replacement)
			}
			continue
		}
		parts = append(parts, field.wire)
	}
	return protoPayload(parts...)
}

func profilerGenericWireValidPayload(eventField int) []byte {
	return profilerGenericWirePayloadReplacing(eventField, -1, nil)
}

func TestProfilerGenericWireRawProducerExactTypedMatrix(t *testing.T) {
	field5Truncated := protoPayload(
		protoBytes(1, []byte("prev")), protoVarint(2, 100), protoVarint(3, 120), protoVarint(4, 1),
		protoVarint(6, 101), protoVarint(7, 118), []byte{0x2a, 0x02, 'x'},
	)
	field6OutOfRange := protoPayload(
		protoBytes(1, []byte("prev")), protoVarint(2, 100), protoVarint(3, 120), protoVarint(4, 1),
		protoBytes(5, []byte("next")), protoVarint(6, math.MaxInt32+1), protoVarint(7, 118),
	)

	tests := []struct {
		name       string
		field      int
		payload    []byte
		wantOK     bool
		wantIssue  profilerFtraceEventIssue
		wantLabel  string
		forbidBody string
	}{
		{
			name: "410 localized truncated scalar", field: 410,
			payload:   protoPayload(protoBytes(1, []byte("clk")), []byte{0x10, 0x80}),
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldMalformedWire, 2),
			wantLabel: "core_field2_malformed_wire",
		},
		{
			name: "410 localized unsupported known wire", field: 410,
			payload:   protoPayload(protoBytes(1, []byte("clk")), []byte{0x13}),
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldMalformedWire, 2),
			wantLabel: "core_field2_malformed_wire",
		},
		{
			name: "410 string wrong wire", field: 410,
			payload:   protoPayload(protoVarint(1, 1), protoVarint(2, 2)),
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldWrongWire, 1),
			wantLabel: "core_field1_wrong_wire",
		},
		{
			name: "410 scalar duplicate", field: 410,
			payload:   profilerGenericWireClockPayload(protoVarint(2, 3)),
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldDuplicate, 2),
			wantLabel: "core_field2_duplicate",
		},
		{
			name: "2002 required name missing", field: 2002,
			payload:   protoVarint(2, 2),
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldMissingOrInvalid, 1),
			wantLabel: "core_field1_missing_or_invalid",
		},
		{
			name: "2417 localized truncated string", field: 2417, payload: field5Truncated,
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldMalformedWire, 5),
			wantLabel: "core_field5_malformed_wire",
		},
		{
			name: "2417 pid out of range", field: 2417, payload: field6OutOfRange,
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldOutOfRange, 6),
			wantLabel: "core_field6_out_of_range",
		},
		{
			name: "2002 cpu malformed", field: 2002,
			payload: protoPayload(profilerGenericWireClockPayload(), []byte{0x18, 0x80}), wantOK: true,
			wantIssue: profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireCPUIDMalformedWire, 3),
			wantLabel: "cpu_id_malformed_wire", forbidBody: "cpu_id=",
		},
		{
			name: "2002 cpu wrong wire", field: 2002,
			payload: profilerGenericWireClockPayload(protoBytes(3, []byte{7})), wantOK: true,
			wantIssue: profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireCPUIDWrongWire, 3),
			wantLabel: "cpu_id_wrong_wire", forbidBody: "cpu_id=",
		},
		{
			name: "2002 cpu duplicate", field: 2002,
			payload: profilerGenericWireClockPayload(protoVarint(3, 7), protoVarint(3, 8)), wantOK: true,
			wantIssue: profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireCPUIDDuplicate, 3),
			wantLabel: "cpu_id_duplicate", forbidBody: "cpu_id=",
		},
		{
			name: "2002 cpu out of range", field: 2002,
			payload: profilerGenericWireClockPayload(protoVarint(3, uint64(maxTraceDBCPUIndex+1))), wantOK: true,
			wantIssue: profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireCPUIDOutOfRange, 3),
			wantLabel: "cpu_id_out_of_range", forbidBody: "cpu_id=",
		},
		{
			name: "2417 next info malformed", field: 2417,
			payload: protoPayload(profilerGenericWireSchedPayload(), []byte{0x40, 0x80}), wantOK: true,
			wantIssue: profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireNextInfoMalformedWire, 8),
			wantLabel: "next_info_malformed_wire", forbidBody: "next_info=",
		},
		{
			name: "2417 next info wrong wire", field: 2417,
			payload: profilerGenericWireSchedPayload(protoBytes(8, []byte{1})), wantOK: true,
			wantIssue: profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireNextInfoWrongWire, 8),
			wantLabel: "next_info_wrong_wire", forbidBody: "next_info=",
		},
		{
			name: "2417 next info duplicate", field: 2417,
			payload: profilerGenericWireSchedPayload(protoVarint(8, 1), protoVarint(8, 2)), wantOK: true,
			wantIssue: profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireNextInfoDuplicate, 8),
			wantLabel: "next_info_duplicate", forbidBody: "next_info=",
		},
		{
			name: "410 malformed key is whole payload", field: 410,
			payload:   protoPayload(profilerGenericWireClockPayload(), []byte{0x80}),
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWirePayloadMalformedWire, 0),
			wantLabel: "wire_payload_malformed_wire",
		},
		{
			name: "2002 unknown truncated endpoint is whole payload", field: 2002,
			payload:   protoPayload(profilerGenericWireClockPayload(), []byte{0x48, 0x80}),
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWirePayloadMalformedWire, 0),
			wantLabel: "wire_payload_malformed_wire",
		},
		{
			name: "2417 malformed key is whole payload", field: 2417,
			payload:   protoPayload(profilerGenericWireSchedPayload(), []byte{0x80}),
			wantIssue: profilerGenericWireHardIssue(profilerFtraceEventIssueWirePayloadMalformedWire, 0),
			wantLabel: "wire_payload_malformed_wire",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := requireProfilerGenericWireTypedResult(t, test.field, test.payload, test.wantOK,
				[]profilerFtraceEventIssue{test.wantIssue}, []string{test.wantLabel})
			if test.forbidBody != "" && strings.Contains(got.body, test.forbidBody) {
				t.Fatalf("degraded display endpoint escaped into body: body=%q forbidden=%q", got.body, test.forbidBody)
			}
		})
	}
}

func TestProfilerGenericWireRawProducerClosedUniverse54(t *testing.T) {
	type rawCase struct {
		name    string
		event   int
		payload []byte
		ok      bool
		issue   profilerFtraceEventIssue
	}
	var cases []rawCase
	add := func(name string, event int, payload []byte, ok bool, issue profilerFtraceEventIssue) {
		cases = append(cases, rawCase{name: name, event: event, payload: payload, ok: ok, issue: issue})
	}

	// 11 hard endpoints x malformed/wrong/duplicate = 33 exact tuples. The
	// malformed replacement deliberately leaves later bytes behind; a known
	// hard endpoint must still remain the sole localized issue.
	hardEndpoints := []struct {
		event  int
		fields []int
	}{
		{event: 410, fields: []int{1, 2}},
		{event: 2002, fields: []int{1, 2}},
		{event: 2417, fields: []int{1, 2, 3, 4, 5, 6, 7}},
	}
	for _, endpointSet := range hardEndpoints {
		fixtures := profilerGenericWireValidFieldFixtures(endpointSet.event)
		for _, payloadField := range endpointSet.fields {
			var fieldFixture profilerGenericWireFieldFixture
			for _, fixture := range fixtures {
				if fixture.field == payloadField {
					fieldFixture = fixture
					break
				}
			}
			malformed := []byte{byte(payloadField<<3 | 3)}
			wrong := protoBytes(payloadField, []byte{1})
			if fieldFixture.string {
				wrong = protoVarint(payloadField, 1)
			}
			add("malformed", endpointSet.event,
				profilerGenericWirePayloadReplacing(endpointSet.event, payloadField, malformed), false,
				profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldMalformedWire, uint8(payloadField)))
			add("wrong-wire", endpointSet.event,
				profilerGenericWirePayloadReplacing(endpointSet.event, payloadField, wrong), false,
				profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldWrongWire, uint8(payloadField)))
			add("duplicate", endpointSet.event,
				protoPayload(profilerGenericWireValidPayload(endpointSet.event), fieldFixture.wire), false,
				profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldDuplicate, uint8(payloadField)))
		}
	}

	// Four scheduler identity/priority range endpoints.
	belowInt32 := int64(math.MinInt32) - 1
	for _, rangeCase := range []struct {
		field int
		value uint64
	}{
		{field: 2, value: math.MaxInt32 + 1},
		{field: 3, value: math.MaxInt32 + 1},
		{field: 6, value: math.MaxInt32 + 1},
		{field: 7, value: uint64(belowInt32)},
	} {
		add("out-of-range", 2417,
			profilerGenericWirePayloadReplacing(2417, rangeCase.field, protoVarint(rangeCase.field, rangeCase.value)), false,
			profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldOutOfRange, uint8(rangeCase.field)))
	}

	// Four required string endpoints, covering both physical absence and unsafe
	// source text while retaining the same exact missing-or-invalid kind.
	for _, stringCase := range []struct {
		event       int
		field       int
		replacement []byte
	}{
		{event: 410, field: 1},
		{event: 2002, field: 1, replacement: protoBytes(1, []byte("bad clock"))},
		{event: 2417, field: 1},
		{event: 2417, field: 5, replacement: protoBytes(5, []byte("bad\ncomm"))},
	} {
		add("required-string", stringCase.event,
			profilerGenericWirePayloadReplacing(stringCase.event, stringCase.field, stringCase.replacement), false,
			profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldMissingOrInvalid, uint8(stringCase.field)))
	}

	// Four CPU display tuples and three packed-next-info display tuples.
	for _, displayCase := range []struct {
		name        string
		replacement []byte
		kind        profilerFtraceEventIssueKind
	}{
		{name: "malformed", replacement: []byte{0x18, 0x80}, kind: profilerFtraceEventIssueWireCPUIDMalformedWire},
		{name: "wrong-wire", replacement: protoBytes(3, []byte{7}), kind: profilerFtraceEventIssueWireCPUIDWrongWire},
		{name: "duplicate", replacement: protoPayload(protoVarint(3, 7), protoVarint(3, 8)), kind: profilerFtraceEventIssueWireCPUIDDuplicate},
		{name: "out-of-range", replacement: protoVarint(3, uint64(maxTraceDBCPUIndex+1)), kind: profilerFtraceEventIssueWireCPUIDOutOfRange},
	} {
		add("cpu-"+displayCase.name, 2002,
			profilerGenericWirePayloadReplacing(2002, 3, displayCase.replacement), true,
			profilerGenericWireDisplayIssue(displayCase.kind, 3))
	}
	for _, displayCase := range []struct {
		name        string
		replacement []byte
		kind        profilerFtraceEventIssueKind
	}{
		{name: "malformed", replacement: []byte{0x40, 0x80}, kind: profilerFtraceEventIssueWireNextInfoMalformedWire},
		{name: "wrong-wire", replacement: protoBytes(8, []byte{1}), kind: profilerFtraceEventIssueWireNextInfoWrongWire},
		{name: "duplicate", replacement: protoPayload(protoVarint(8, 1), protoVarint(8, 2)), kind: profilerFtraceEventIssueWireNextInfoDuplicate},
	} {
		add("next-info-"+displayCase.name, 2417,
			profilerGenericWirePayloadReplacing(2417, 8, displayCase.replacement), true,
			profilerGenericWireDisplayIssue(displayCase.kind, 8))
	}

	// Whole-payload and final canonical-line failures are valid for each of the
	// three event schemas: 3 + 3 = 6 fixed-field tuples.
	for _, eventField := range []int{410, 2002, 2417} {
		add("whole-payload", eventField,
			protoPayload(profilerGenericWireValidPayload(eventField), []byte{0x80}), false,
			profilerGenericWireHardIssue(profilerFtraceEventIssueWirePayloadMalformedWire, 0))
		oversizedField := 1
		add("canonical-line", eventField,
			profilerGenericWirePayloadReplacing(eventField, oversizedField,
				protoBytes(oversizedField, []byte(strings.Repeat("x", maxTraceDBSystraceLineBytes)))), false,
			profilerGenericWireHardIssue(profilerFtraceEventIssueWireInvalidCanonicalLine, 0))
	}

	if len(cases) != 54 {
		t.Fatalf("raw producer universe=%d want=54", len(cases))
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			label, labelOK := test.issue.label(test.event)
			if !labelOK {
				t.Fatalf("fixture issue is not legal: event=%d issue=%+v", test.event, test.issue)
			}
			requireProfilerGenericWireTypedResult(t, test.event, test.payload, test.ok,
				[]profilerFtraceEventIssue{test.issue}, []string{label})
		})
	}
}

func TestProfilerGenericWireHardAndDisplayArmsAreMutuallyExclusive(t *testing.T) {
	wantIssue := profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldWrongWire, 2)
	wantLabels := []string{"core_field2_wrong_wire"}
	for _, payload := range [][]byte{
		protoPayload(protoBytes(1, []byte("clk")), protoBytes(2, []byte{2}), protoBytes(3, []byte{3})),
		protoPayload(protoBytes(3, []byte{3}), protoBytes(2, []byte{2}), protoBytes(1, []byte("clk"))),
	} {
		got := requireProfilerGenericWireTypedResult(t, 2002, payload, false,
			[]profilerFtraceEventIssue{wantIssue}, wantLabels)
		for _, issue := range got.issues {
			if issue.Severity != profilerFtraceEventIssueHardReject ||
				issue.Kind == profilerFtraceEventIssueWireCPUIDWrongWire {
				t.Fatalf("display arm survived hard rejection: %+v", got.issues)
			}
		}
	}

	// Once framing loses endpoint identity, the whole-payload hard issue is the
	// only honest result. A previously seen display issue must not survive it.
	wholeDominates := protoPayload(
		protoBytes(1, []byte("clk")), protoVarint(2, 2), protoBytes(3, []byte{3}), []byte{0x80},
	)
	requireProfilerGenericWireTypedResult(t, 2002, wholeDominates, false,
		[]profilerFtraceEventIssue{profilerGenericWireHardIssue(profilerFtraceEventIssueWirePayloadMalformedWire, 0)},
		[]string{"wire_payload_malformed_wire"})
}

func TestProfilerGenericWireField410NeverAcquiresCPUAuthority(t *testing.T) {
	for _, extra := range [][]byte{
		protoVarint(3, 7),
		protoBytes(3, []byte{7}),
		protoPayload(protoVarint(3, 7), protoBytes(3, []byte{7})),
	} {
		got := requireProfilerGenericWireTypedResult(t, 410,
			profilerGenericWireClockPayload(extra), true, nil, nil)
		if got.name != "clock_set_rate" || got.body != "clk state=2" || strings.Contains(got.body, "cpu_id=") {
			t.Fatalf("field410 acquired CPU semantics: name=%q body=%q issues=%+v", got.name, got.body, got.issues)
		}
	}

	// The same complete field-3 bytes are a display degradation only in the
	// power.proto field-2002 schema.
	requireProfilerGenericWireTypedResult(t, 2002,
		profilerGenericWireClockPayload(protoBytes(3, []byte{7})), true,
		[]profilerFtraceEventIssue{profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireCPUIDWrongWire, 3)},
		[]string{"cpu_id_wrong_wire"})

	// An incomplete unknown field still breaks protobuf framing, but field410
	// must classify it as whole-payload damage rather than invent a CPU endpoint.
	requireProfilerGenericWireTypedResult(t, 410,
		protoPayload(profilerGenericWireClockPayload(), []byte{0x18, 0x80}), false,
		[]profilerFtraceEventIssue{profilerGenericWireHardIssue(profilerFtraceEventIssueWirePayloadMalformedWire, 0)},
		[]string{"wire_payload_malformed_wire"})

	for _, kind := range []profilerFtraceEventIssueKind{
		profilerFtraceEventIssueWireCPUIDMalformedWire,
		profilerFtraceEventIssueWireCPUIDWrongWire,
		profilerFtraceEventIssueWireCPUIDDuplicate,
		profilerFtraceEventIssueWireCPUIDOutOfRange,
	} {
		if issue, ok := profilerFtraceEventFixedIssue(410, kind); ok || issue.validFor(410) {
			t.Fatalf("field410 minted CPU issue kind=%d: issue=%+v ok=%t", kind, issue, ok)
		}
		tampered := profilerGenericWireDisplayIssue(kind, 3)
		if tampered.validFor(410) {
			t.Fatalf("field410 accepted tampered CPU issue: %+v", tampered)
		}
	}
}

func TestProfilerGenericWireMaximumIssueSetIsSevenAndSuppressesDisplay(t *testing.T) {
	forward := [][]byte{
		protoVarint(1, 1), protoBytes(2, []byte{2}), protoBytes(3, []byte{3}), protoBytes(4, []byte{4}),
		protoVarint(5, 5), protoBytes(6, []byte{6}), protoBytes(7, []byte{7}), protoBytes(8, []byte{8}),
	}
	reverse := make([][]byte, len(forward))
	for index := range forward {
		reverse[index] = forward[len(forward)-index-1]
	}
	wantIssues := make([]profilerFtraceEventIssue, 0, 7)
	wantLabels := make([]string, 0, 7)
	// The compatibility display order retains the pre-migration string arm
	// (fields 1,5) followed by the scalar arm. It must not depend on wire order.
	for _, field := range []int{1, 5, 2, 3, 4, 6, 7} {
		wantIssues = append(wantIssues,
			profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldWrongWire, uint8(field)))
		wantLabels = append(wantLabels, "core_field"+string(rune('0'+field))+"_wrong_wire")
	}
	if len(wantIssues) != 7 {
		t.Fatalf("fixture maximum=%d want=7", len(wantIssues))
	}
	for _, parts := range [][][]byte{forward, reverse} {
		got := requireProfilerGenericWireTypedResult(t, 2417, protoPayload(parts...), false, wantIssues, wantLabels)
		if len(got.issues) != 7 {
			t.Fatalf("generic fixed maximum=%d want=7: %+v", len(got.issues), got.issues)
		}
		for _, issue := range got.issues {
			if issue.PayloadField == 8 || issue.Severity != profilerFtraceEventIssueHardReject {
				t.Fatalf("field8 display issue survived maximum hard set: %+v", got.issues)
			}
		}
	}
}

func TestProfilerGenericWireCanonicalFailureRejectsOnlyBadRowInContainer(t *testing.T) {
	overlongPayload := protoPayload(
		protoBytes(1, []byte(strings.Repeat("x", maxTraceDBSystraceLineBytes))), protoVarint(2, 2),
	)
	requireProfilerGenericWireTypedResult(t, 410, overlongPayload, false,
		[]profilerFtraceEventIssue{profilerGenericWireHardIssue(profilerFtraceEventIssueWireInvalidCanonicalLine, 0)},
		[]string{"invalid_canonical_wire_line"})

	healthyPayload := profilerGenericWireClockPayload()
	structured := protoMessage(2,
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 100, 100, "bad", 410, overlongPayload),
		syntheticTracePluginFtraceEvent(2_000, 101, 101, "healthy", 410, healthyPayload),
	)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", structured),
	)
	defer sink.cleanup()

	coverage, entries := profilerEventCoverageByField(extracted, 410)
	if entries != 1 || coverage.RowsRead != 2 || coverage.RowsEmitted != 1 ||
		coverage.FieldSources["degraded_invalid_canonical_wire_line_occurrences"] != "1" ||
		coverage.FieldSources["degraded_invalid_canonical_wire_line_affected_frames"] != "1" {
		t.Fatalf("canonical local-reject census drifted: entries=%d coverage=%+v", entries, coverage)
	}
	if extracted.SourceFailClosed || extracted.StructuredRows != 1 || sink.publishableRows() != 1 ||
		len(sink.rows) != 1 || !strings.Contains(sink.rows[0].line, "healthy") ||
		!strings.Contains(sink.rows[0].line, "clock_set_rate: clk state=2") {
		t.Fatalf("bad canonical row contaminated healthy sibling: extracted=%+v sink=%+v rows=%+v",
			extracted, sink.stats, sink.rows)
	}
}

func TestProfilerGenericWireDisplayMalformedAmbiguousTailFailsWholePayload(t *testing.T) {
	overflowingValue := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}
	payload := protoPayload(
		profilerGenericWireClockPayload(),
		append([]byte{0x18}, overflowingValue...),
		protoBytes(2, []byte{2}), // potentially hidden hard-field damage
	)
	requireProfilerGenericWireTypedResult(t, 2002, payload, false,
		[]profilerFtraceEventIssue{profilerGenericWireHardIssue(profilerFtraceEventIssueWirePayloadMalformedWire, 0)},
		[]string{"wire_payload_malformed_wire"})
}

func TestProfilerGenericWireLocalizedHardFailureDoesNotMintUnscannedFields(t *testing.T) {
	for _, payload := range [][]byte{
		{0x10, 0x80}, // field2 malformed before either required comm
		protoPayload(protoBytes(1, []byte("prev")), []byte{0x10, 0x80}),
		// Unsupported known wire with an ambiguous tail is still an exact hard
		// endpoint: the row is rejected and no later field identity is guessed.
		protoPayload(protoBytes(1, []byte("prev")), []byte{0x13, 0x2a, 0x01, 'x'}),
	} {
		requireProfilerGenericWireTypedResult(t, 2417, payload, false,
			[]profilerFtraceEventIssue{profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldMalformedWire, 2)},
			[]string{"core_field2_malformed_wire"})
	}
}

func TestProfilerGenericWireCanonicalBoundaryAndDisplayPrecedence(t *testing.T) {
	record := profilerGenericWireTestRecord(410, nil)
	emptyClockBody := "clock_set_rate:  state=2"
	overhead := len(traceDBFormatLine(record.Comm, record.PID, record.TGID, record.CPU, int64(record.TSNS),
		record.CommonFlags, record.CommonPreemptCount, emptyClockBody))
	clockBytes := maxTraceDBSystraceLineBytes - overhead
	if clockBytes <= 0 {
		t.Fatalf("canonical fixture overhead=%d exceeds cap=%d", overhead, maxTraceDBSystraceLineBytes)
	}
	exactName := strings.Repeat("x", clockBytes)
	exact := requireProfilerGenericWireTypedResult(t, 410,
		protoPayload(protoBytes(1, []byte(exactName)), protoVarint(2, 2)), true, nil, nil)
	exactLine := traceDBFormatLine(record.Comm, record.PID, record.TGID, record.CPU, int64(record.TSNS),
		record.CommonFlags, record.CommonPreemptCount, exact.name+": "+exact.body)
	if len(exactLine) != maxTraceDBSystraceLineBytes {
		t.Fatalf("canonical exact-cap calibration drifted: len=%d want=%d", len(exactLine), maxTraceDBSystraceLineBytes)
	}

	tooLongName := exactName + "x"
	canonical := profilerGenericWireHardIssue(profilerFtraceEventIssueWireInvalidCanonicalLine, 0)
	requireProfilerGenericWireTypedResult(t, 410,
		protoPayload(protoBytes(1, []byte(tooLongName)), protoVarint(2, 2)), false,
		[]profilerFtraceEventIssue{canonical}, []string{"invalid_canonical_wire_line"})
	// A display-only CPU fault is not allowed to survive a stronger canonical
	// line rejection, even when it appears before the oversized name on wire.
	requireProfilerGenericWireTypedResult(t, 2002,
		protoPayload(protoBytes(3, []byte{7}), protoBytes(1, []byte(tooLongName)), protoVarint(2, 2)), false,
		[]profilerFtraceEventIssue{canonical}, []string{"invalid_canonical_wire_line"})
}

func TestProfilerGenericWireIssueSetFailsClosedWithoutMutation(t *testing.T) {
	valid, ok := profilerFtraceEventPayloadIssue(2417, profilerFtraceEventIssueWireFieldWrongWire, 1)
	if !ok {
		t.Fatal("fixture issue rejected")
	}
	var set profilerFtraceGenericIssueSet
	if err := set.add(2417, valid); err != nil {
		t.Fatalf("valid issue rejected: %v", err)
	}
	before := set
	if err := set.add(2417, valid); err == nil || !reflect.DeepEqual(set, before) {
		t.Fatalf("duplicate issue did not fail without mutation: err=%v before=%+v after=%+v", err, before, set)
	}

	var full profilerFtraceGenericIssueSet
	for _, field := range []int{1, 5, 2, 3, 4, 6, 7} {
		if err := full.addPayload(2417, profilerFtraceEventIssueWireFieldWrongWire, field); err != nil {
			t.Fatalf("fill field%d: %v", field, err)
		}
	}
	fullBefore := full
	if err := full.addFixed(2417, profilerFtraceEventIssueWireNextInfoWrongWire); err == nil || !reflect.DeepEqual(full, fullBefore) {
		t.Fatalf("overflow did not fail without mutation: err=%v", err)
	}

	hardWrong := profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldWrongWire, 1)
	hardDuplicate := profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldDuplicate, 1)
	hardMalformed := profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldMalformedWire, 1)
	hardWrongField2 := profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldWrongWire, 2)
	hardRangeField2 := profilerGenericWireHardIssue(profilerFtraceEventIssueWireFieldOutOfRange, 2)
	whole := profilerGenericWireHardIssue(profilerFtraceEventIssueWirePayloadMalformedWire, 0)
	canonical := profilerGenericWireHardIssue(profilerFtraceEventIssueWireInvalidCanonicalLine, 0)
	displayWrong := profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireCPUIDWrongWire, 3)
	displayDuplicate := profilerGenericWireDisplayIssue(profilerFtraceEventIssueWireCPUIDDuplicate, 3)
	conflicts := []struct {
		event int
		first profilerFtraceEventIssue
		next  profilerFtraceEventIssue
	}{
		{event: 2002, first: hardWrong, next: hardDuplicate},       // one endpoint, two precedence states
		{event: 2002, first: whole, next: canonical},               // two sole whole-message arms
		{event: 2002, first: whole, next: hardWrong},               // whole-message plus field-hard
		{event: 2002, first: hardWrong, next: displayWrong},        // hard plus admitted-display
		{event: 2002, first: displayWrong, next: displayDuplicate}, // multiple display issues
		{event: 2417, first: hardMalformed, next: hardWrongField2}, // localized malformed must be sole
		{event: 2417, first: hardRangeField2, next: hardWrong},     // range arm follows only a clean hard audit
	}
	for index, conflict := range conflicts {
		for order, insertion := range [][2]profilerFtraceEventIssue{
			{conflict.first, conflict.next},
			{conflict.next, conflict.first},
		} {
			var candidate profilerFtraceGenericIssueSet
			if err := candidate.add(conflict.event, insertion[0]); err != nil {
				t.Fatalf("insertion fixture %d/%d first issue rejected: %v", index, order, err)
			}
			candidateBefore := candidate
			if err := candidate.add(conflict.event, insertion[1]); err == nil || !reflect.DeepEqual(candidate, candidateBefore) {
				t.Fatalf("incompatible insertion %d/%d mutated set: err=%v before=%+v after=%+v",
					index, order, err, candidateBefore, candidate)
			}
		}
	}

	corruptions := []struct {
		event int
		set   profilerFtraceGenericIssueSet
	}{
		{event: 2417, set: profilerFtraceGenericIssueSet{Count: profilerFtraceGenericIssuesPerEvent + 1}},
		{event: 2417, set: profilerFtraceGenericIssueSet{Count: 1, Issues: [profilerFtraceGenericIssuesPerEvent]profilerFtraceEventIssue{{Kind: profilerFtraceEventIssueEnvelopeEventMalformedWire}}}},
		{event: 2417, set: profilerFtraceGenericIssueSet{Count: 2, Issues: [profilerFtraceGenericIssuesPerEvent]profilerFtraceEventIssue{valid, valid}}},
		{event: 2417, set: profilerFtraceGenericIssueSet{Count: 0, Issues: [profilerFtraceGenericIssuesPerEvent]profilerFtraceEventIssue{valid}}},
	}
	for _, conflict := range conflicts {
		for _, order := range [][2]profilerFtraceEventIssue{
			{conflict.first, conflict.next},
			{conflict.next, conflict.first},
		} {
			corruptions = append(corruptions, struct {
				event int
				set   profilerFtraceGenericIssueSet
			}{event: conflict.event, set: profilerFtraceGenericIssueSet{
				Count: 2, Issues: [profilerFtraceGenericIssuesPerEvent]profilerFtraceEventIssue{order[0], order[1]},
			}})
		}
	}
	severityFlip := valid
	severityFlip.Severity = profilerFtraceEventIssueAdmittedDisplay
	corruptions = append(corruptions, struct {
		event int
		set   profilerFtraceGenericIssueSet
	}{event: 2417, set: profilerFtraceGenericIssueSet{
		Count: 1, Issues: [profilerFtraceGenericIssuesPerEvent]profilerFtraceEventIssue{severityFlip},
	},
	})
	for index := range corruptions {
		if _, err := corruptions[index].set.checked(corruptions[index].event); err == nil {
			t.Fatalf("corrupt issue set %d admitted: %+v", index, corruptions[index])
		}
	}
}
