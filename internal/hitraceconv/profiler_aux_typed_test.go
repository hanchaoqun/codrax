package hitraceconv

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type profilerAuxTypedFixture struct {
	name  string
	event profilerFtraceEventRecord
	issue profilerFtraceEventIssue
}

type profilerAuxTypedTuple struct {
	event int
	issue profilerFtraceEventIssue
}

func profilerAuxTypedRecord(eventField int, payload []byte) profilerFtraceEventRecord {
	return profilerFtraceEventRecord{
		CPU: 2, TSNS: 1_000, TGID: 40, PID: 40, HeaderOwnerKnown: true,
		Comm: "aux-test", Field: eventField, Payload: payload,
	}
}

func profilerAuxTypedPayload(base profilerAuxTestCase, changes map[int]profilerAuxTestValue, omit ...int) []byte {
	values := profilerAuxCloneValues(base.values)
	for field, value := range changes {
		values[field] = value
	}
	for _, field := range omit {
		delete(values, field)
	}
	return profilerAuxEncodeValues(values)
}

func profilerAuxTypedFixedIssue(t *testing.T, eventField int, kind profilerFtraceEventIssueKind) profilerFtraceEventIssue {
	t.Helper()
	issue, ok := profilerFtraceEventFixedIssue(eventField, kind)
	if !ok {
		t.Fatalf("fixture fixed issue rejected: event=%d kind=%d", eventField, kind)
	}
	return issue
}

func profilerAuxTypedPayloadIssue(t *testing.T, eventField int, kind profilerFtraceEventIssueKind, payloadField int) profilerFtraceEventIssue {
	t.Helper()
	issue, ok := profilerFtraceEventPayloadIssue(eventField, kind, payloadField)
	if !ok {
		t.Fatalf("fixture payload issue rejected: event=%d kind=%d payload_field=%d", eventField, kind, payloadField)
	}
	return issue
}

func requireProfilerAuxTypedResult(t *testing.T, fixture profilerAuxTypedFixture) {
	t.Helper()
	name, body, ok, issues, _, handled, err := renderProfilerFtraceAuxEventWithTypedAudit(fixture.event)
	if err != nil || !handled {
		t.Fatalf("aux typed choke failed: fixture=%s field=%d handled=%t err=%v", fixture.name, fixture.event.Field, handled, err)
	}
	wantOK := fixture.issue.Severity == profilerFtraceEventIssueAdmittedDisplay
	wantIssues := []profilerFtraceEventIssue{fixture.issue}
	if ok != wantOK || !reflect.DeepEqual(issues, wantIssues) ||
		!profilerFtraceEventIssueVerdictValid(fixture.event.Field, ok, issues) {
		t.Fatalf("aux typed verdict drifted: fixture=%s field=%d ok=%t want_ok=%t issues=%+v want=%+v name=%q body=%q",
			fixture.name, fixture.event.Field, ok, wantOK, issues, wantIssues, name, body)
	}
	if ok && (name == "" || body == "") {
		t.Fatalf("admitted aux row lost output: fixture=%s name=%q body=%q", fixture.name, name, body)
	}
	if !ok && (name != "" || body != "") {
		t.Fatalf("rejected aux row retained output: fixture=%s name=%q body=%q", fixture.name, name, body)
	}

	typedName, typedBody, typedOK, typedIssues, typedErr := renderProfilerFtraceEventBodyWithTypedAudit(fixture.event)
	if typedErr != nil || typedName != name || typedBody != body || typedOK != ok || !reflect.DeepEqual(typedIssues, issues) {
		t.Fatalf("outer typed/aux parity drifted: fixture=%s\naux=(%q,%q,%t,%+v,%v)\nouter=(%q,%q,%t,%+v,%v)",
			fixture.name, name, body, ok, issues, err, typedName, typedBody, typedOK, typedIssues, typedErr)
	}
	labels, labelsOK := profilerFtraceEventIssueLabels(fixture.event.Field, issues)
	if !labelsOK || len(labels) != 1 {
		t.Fatalf("typed aux issue has no unique compatibility label: fixture=%s issues=%+v labels=%v", fixture.name, issues, labels)
	}
	compatName, compatBody, compatOK, compatLabels := renderProfilerFtraceEventBodyWithAudit(fixture.event)
	if compatName != name || compatBody != body || compatOK != ok || !reflect.DeepEqual(compatLabels, labels) {
		t.Fatalf("direct label/aux parity drifted: fixture=%s\naux=(%q,%q,%t,%v)\ncompat=(%q,%q,%t,%v)",
			fixture.name, name, body, ok, labels, compatName, compatBody, compatOK, compatLabels)
	}
	if label, ok := fixture.issue.label(fixture.event.Field); !ok || label != labels[0] {
		t.Fatalf("aux typed label authority drifted: fixture=%s label=%q issue=%+v direct=(%q,%t)",
			fixture.name, labels[0], fixture.issue, label, ok)
	}
}

func profilerAuxTypedOverflowValue(eventField, payloadField int) uint64 {
	switch eventField {
	case 4009:
		switch payloadField {
		case 4:
			return uint64(math.MaxUint16) + 1
		case 5:
			return uint64(math.MaxInt64) + 1
		case 8:
			return uint64(math.MaxUint8) + 1
		default:
			return uint64(math.MaxUint32) + 1
		}
	case 4010:
		if payloadField >= 3 {
			return uint64(1)<<32 | 1
		}
		return uint64(math.MaxUint32) + 1
	case 4011, 4012:
		if payloadField == 3 {
			return uint64(math.MaxInt64) + 1
		}
		return uint64(math.MaxUint32) + 1
	case 4015:
		switch payloadField {
		case 2, 6, 10, 14, 15, 19, 20:
			return uint64(1)<<32 | 1
		default:
			return uint64(math.MaxUint32) + 1
		}
	case 4016:
		switch payloadField {
		case 17, 21, 22:
			return uint64(1)<<32 | 1
		default:
			return uint64(math.MaxUint32) + 1
		}
	default:
		panic("invalid aux range fixture")
	}
}

func profilerAuxTypedRawFixtures(t *testing.T) []profilerAuxTypedFixture {
	t.Helper()
	base := profilerAuxCasesByField()
	events := make([]int, 0, len(base))
	for eventField := range base {
		events = append(events, eventField)
	}
	sort.Ints(events)
	fixtures := make([]profilerAuxTypedFixture, 0, 228)
	add := func(name string, eventField int, payload []byte, issue profilerFtraceEventIssue) {
		fixtures = append(fixtures, profilerAuxTypedFixture{
			name: name, event: profilerAuxTypedRecord(eventField, payload), issue: issue,
		})
	}

	for _, eventField := range events {
		payload := append(profilerAuxTypedPayload(base[eventField], nil), 0x80)
		add("whole-malformed", eventField, payload,
			profilerAuxTypedFixedIssue(t, eventField, profilerFtraceEventIssueAuxPayloadMalformedWire))
	}

	for _, eventField := range events {
		fields := make([]int, 0, len(profilerStructuredAuxSchemas[eventField]))
		for payloadField := range profilerStructuredAuxSchemas[eventField] {
			if !profilerFtraceAuxResponseField(eventField, payloadField) {
				fields = append(fields, payloadField)
			}
		}
		sort.Ints(fields)
		for _, payloadField := range fields {
			value := base[eventField].values[payloadField]
			wrong := profilerAuxTypedPayload(base[eventField], nil, payloadField)
			wrong = append(wrong, profilerAuxWrongWire(payloadField, value)...)
			add("field-wrong-wire", eventField, wrong,
				profilerAuxTypedPayloadIssue(t, eventField, profilerFtraceEventIssueAuxFieldWrongWire, payloadField))
			duplicate := append(profilerAuxTypedPayload(base[eventField], nil), profilerAuxEncodeField(payloadField, value)...)
			add("field-duplicate", eventField, duplicate,
				profilerAuxTypedPayloadIssue(t, eventField, profilerFtraceEventIssueAuxFieldDuplicate, payloadField))
		}
	}

	for _, eventField := range events {
		for payloadField := 1; payloadField <= 25; payloadField++ {
			if !profilerFtraceAuxRangeFieldKnown(eventField, payloadField) {
				continue
			}
			payload := profilerAuxTypedPayload(base[eventField], map[int]profilerAuxTestValue{
				payloadField: profilerAuxVarint(profilerAuxTypedOverflowValue(eventField, payloadField)),
			})
			add("field-out-of-range", eventField, payload,
				profilerAuxTypedPayloadIssue(t, eventField, profilerFtraceEventIssueAuxFieldOutOfRange, payloadField))
		}
	}

	add("print-invalid", 1109, profilerAuxTypedPayload(base[1109], map[int]profilerAuxTestValue{2: profilerAuxBytes("")}),
		profilerAuxTypedFixedIssue(t, 1109, profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf))
	for _, eventField := range []int{4009, 4010, 4011, 4012} {
		add("f2fs-dev-invalid", eventField,
			profilerAuxTypedPayload(base[eventField], map[int]profilerAuxTestValue{1: profilerAuxVarint(0)}),
			profilerAuxTypedFixedIssue(t, eventField, profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev))
		add("f2fs-ino-invalid", eventField,
			profilerAuxTypedPayload(base[eventField], map[int]profilerAuxTestValue{2: profilerAuxVarint(0)}),
			profilerAuxTypedFixedIssue(t, eventField, profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno))
	}
	for _, item := range []struct {
		event, pointer, name int
	}{
		{4015, 22, 23},
		{4016, 24, 25},
	} {
		add("mmc-pointer-invalid", item.event,
			profilerAuxTypedPayload(base[item.event], map[int]profilerAuxTestValue{item.pointer: profilerAuxVarint(0)}),
			profilerAuxTypedFixedIssue(t, item.event, profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer))
		add("mmc-name-invalid", item.event,
			profilerAuxTypedPayload(base[item.event], map[int]profilerAuxTestValue{item.name: profilerAuxBytes("bad name")}),
			profilerAuxTypedFixedIssue(t, item.event, profilerFtraceEventIssueAuxMissingOrInvalidMMCName))
	}

	for _, payloadField := range []int{3, 7, 11} {
		value := base[4015].values[payloadField]
		wrong := profilerAuxTypedPayload(base[4015], nil, payloadField)
		wrong = append(wrong, profilerAuxWrongWire(payloadField, value)...)
		add("response-wrong-wire", 4015, wrong,
			profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseWrongWire, payloadField))
		duplicate := append(profilerAuxTypedPayload(base[4015], nil), profilerAuxEncodeField(payloadField, value)...)
		add("response-duplicate", 4015, duplicate,
			profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseDuplicate, payloadField))
		drop := profilerAuxTypedPayload(base[4015], map[int]profilerAuxTestValue{
			payloadField: {wire: 2, bytes: make([]byte, maxProfilerMMCResponseBytes+1)},
		})
		add("response-out-of-profile", 4015, drop,
			profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, payloadField))
	}

	add("canonical-line", 1109, protoBytes(2, []byte(strings.Repeat("x", maxTraceDBSystraceLineBytes))),
		profilerAuxTypedFixedIssue(t, 1109, profilerFtraceEventIssueAuxInvalidCanonicalLine))

	if len(fixtures) != 228 {
		t.Fatalf("raw aux typed fixture universe=%d want=228", len(fixtures))
	}
	return fixtures
}

func TestProfilerAuxTypedRawProducerClosureIsExactly228(t *testing.T) {
	fixtures := profilerAuxTypedRawFixtures(t)
	produced := make(map[profilerAuxTypedTuple]string, len(fixtures))
	perEvent := map[int]int{}
	seenKinds := map[profilerFtraceEventIssueKind]bool{}
	for _, fixture := range fixtures {
		requireProfilerAuxTypedResult(t, fixture)
		tuple := profilerAuxTypedTuple{event: fixture.event.Field, issue: fixture.issue}
		if previous, exists := produced[tuple]; exists {
			t.Fatalf("raw aux producer tuple duplicated: event=%d issue=%+v first=%q next=%q",
				tuple.event, tuple.issue, previous, fixture.name)
		}
		produced[tuple] = fixture.name
		perEvent[tuple.event]++
		seenKinds[tuple.issue.Kind] = true
	}

	wantPerEvent := map[int]int{1109: 7, 4009: 24, 4010: 17, 4011: 17, 4012: 17, 4015: 70, 4016: 76}
	if !reflect.DeepEqual(perEvent, wantPerEvent) {
		t.Fatalf("aux raw producer per-event census drifted: got=%v want=%v", perEvent, wantPerEvent)
	}
	if len(seenKinds) != 13 {
		t.Fatalf("aux raw producer kind census=%d want=13: %v", len(seenKinds), seenKinds)
	}

	legal := make(map[profilerAuxTypedTuple]bool, 228)
	for eventField := range wantPerEvent {
		for kind := profilerFtraceEventIssueAuxPayloadMalformedWire; kind <= profilerFtraceEventIssueAuxInvalidCanonicalLine; kind++ {
			for payloadField := 0; payloadField <= math.MaxUint8; payloadField++ {
				for severity := profilerFtraceEventIssueHardReject; severity < profilerFtraceEventIssueSeverityCount; severity++ {
					issue := profilerFtraceEventIssue{Kind: kind, PayloadField: uint8(payloadField), Severity: severity}
					if issue.validFor(eventField) {
						legal[profilerAuxTypedTuple{event: eventField, issue: issue}] = true
					}
				}
			}
		}
	}
	if len(legal) != 228 || len(produced) != len(legal) {
		t.Fatalf("aux legal/source closure cardinality drifted: legal=%d produced=%d", len(legal), len(produced))
	}
	for tuple := range legal {
		if _, ok := produced[tuple]; !ok {
			t.Fatalf("legal aux tuple has no raw producer: event=%d issue=%+v", tuple.event, tuple.issue)
		}
	}
	for tuple, fixture := range produced {
		if !legal[tuple] {
			t.Fatalf("raw aux producer escaped legal closure: fixture=%s event=%d issue=%+v", fixture, tuple.event, tuple.issue)
		}
	}
}

func requireProfilerAuxInvariant(t *testing.T, err error, want string) {
	t.Helper()
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != want {
		t.Fatalf("aux invariant error=%T %v reason=%q want=%q", err, err, reason, want)
	}
}

func TestProfilerAuxTypedIssueSetCapacityAndCorruptionFailClosed(t *testing.T) {
	response := []profilerFtraceEventIssue{
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseWrongWire, 3),
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseDuplicate, 7),
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, 11),
	}
	var full profilerFtraceAuxIssueSet
	for _, issue := range response {
		if err := full.add(4015, issue); err != nil {
			t.Fatalf("valid response issue rejected: issue=%+v err=%v", issue, err)
		}
	}
	if full.Count != profilerFtraceAuxIssuesPerEvent {
		t.Fatalf("aux issue capacity drifted: set=%+v", full)
	}
	before := full
	if err := full.add(4015, response[0]); err == nil || !reflect.DeepEqual(full, before) {
		t.Fatalf("full aux issue set overflow mutated state: err=%v before=%+v after=%+v", err, before, full)
	}

	hard := profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxFieldWrongWire, 1)
	foreign := profilerAuxTypedPayloadIssue(t, 4016, profilerFtraceEventIssueAuxFieldWrongWire, 1)
	foreign.PayloadField = 25
	wrongSeverity := response[0]
	wrongSeverity.Severity = profilerFtraceEventIssueHardReject
	corruptions := []profilerFtraceAuxIssueSet{
		{Count: profilerFtraceAuxIssuesPerEvent + 1},
		{Issues: [profilerFtraceAuxIssuesPerEvent]profilerFtraceEventIssue{response[0]}},
		{Count: 1, Issues: [profilerFtraceAuxIssuesPerEvent]profilerFtraceEventIssue{foreign}},
		{Count: 1, Issues: [profilerFtraceAuxIssuesPerEvent]profilerFtraceEventIssue{wrongSeverity}},
		{Count: 2, Issues: [profilerFtraceAuxIssuesPerEvent]profilerFtraceEventIssue{response[1], response[0]}},
		{Count: 2, Issues: [profilerFtraceAuxIssuesPerEvent]profilerFtraceEventIssue{response[0], response[0]}},
		{Count: 2, Issues: [profilerFtraceAuxIssuesPerEvent]profilerFtraceEventIssue{hard, response[1]}},
		{Count: 1},
	}
	for index := range corruptions {
		if _, err := corruptions[index].checked(4015); err == nil {
			t.Fatalf("corrupt aux issue set %d admitted: %+v", index, corruptions[index])
		}
	}

	for index, pair := range [][2]profilerFtraceEventIssue{
		{response[1], response[0]},
		{response[0], profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseDuplicate, 3)},
		{hard, response[1]},
		{response[0], hard},
	} {
		var set profilerFtraceAuxIssueSet
		if err := set.add(4015, pair[0]); err != nil {
			t.Fatalf("prospective corruption %d first add failed: %v", index, err)
		}
		before := set
		if err := set.add(4015, pair[1]); err == nil || !reflect.DeepEqual(set, before) {
			t.Fatalf("prospective corruption %d mutated state: err=%v before=%+v after=%+v", index, err, before, set)
		}
	}

	base := profilerAuxCasesByField()
	event := profilerAuxTypedRecord(4015, profilerAuxEncodeValues(base[4015].values))
	result, err := decodeProfilerAuxPayloadWithTypedAudit(event)
	if err != nil || result.Admission != bodyAdmitted {
		t.Fatalf("valid aux fixture did not decode: result=%+v err=%v", result, err)
	}
	result.Issues = profilerFtraceAuxIssueSet{Issues: [profilerFtraceAuxIssuesPerEvent]profilerFtraceEventIssue{hard}}
	_, _, _, issues, err := finalizeProfilerFtraceAuxEventWithTypedAudit(event, result)
	if len(issues) != 0 {
		t.Fatalf("corrupt aux set leaked source issue: %+v", issues)
	}
	requireProfilerAuxInvariant(t, err, "profiler_aux_issue_count_invalid")

	result, err = decodeProfilerAuxPayloadWithTypedAudit(event)
	if err != nil {
		t.Fatal(err)
	}
	result.Pair = profilerPairAdmission{Kind: pairRenderF2FS, Governed: true, LaneKnown: true, Lane: "forged", Admitted: true}
	_, _, _, issues, err = finalizeProfilerFtraceAuxEventWithTypedAudit(event, result)
	if len(issues) != 0 {
		t.Fatalf("corrupt aux pair verdict leaked issues: %+v", issues)
	}
	requireProfilerAuxInvariant(t, err, "profiler_aux_pair_verdict_invalid")
}

func TestProfilerAuxTypedDescriptorIsAnInternalInvariant(t *testing.T) {
	base := profilerAuxCasesByField()
	event := profilerAuxTypedRecord(4009, profilerAuxEncodeValues(base[4009].values))
	original := profilerFtraceEventDescriptors[4009]
	for _, test := range []struct {
		name       string
		descriptor profilerFtraceEventDescriptor
		present    bool
		want       string
	}{
		{name: "missing", present: false, want: "missing_aux_descriptor"},
		{name: "field", descriptor: profilerFtraceEventDescriptor{Field: 4010, Family: original.Family, Name: original.Name}, present: true, want: "mismatched_aux_descriptor_field"},
		{name: "name", descriptor: profilerFtraceEventDescriptor{Field: 4009, Family: original.Family, Name: "f2fs_sync_file_exit"}, present: true, want: "mismatched_aux_descriptor_name"},
		{name: "family", descriptor: profilerFtraceEventDescriptor{Field: 4009, Family: "storage", Name: original.Name}, present: true, want: "mismatched_aux_descriptor_family"},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() { profilerFtraceEventDescriptors[4009] = original }()
			if test.present {
				profilerFtraceEventDescriptors[4009] = test.descriptor
			} else {
				delete(profilerFtraceEventDescriptors, 4009)
			}
			_, _, _, issues, _, handled, err := renderProfilerFtraceAuxEventWithTypedAudit(event)
			if !handled || len(issues) != 0 {
				t.Fatalf("descriptor invariant leaked source verdict: handled=%t issues=%+v err=%v", handled, issues, err)
			}
			requireProfilerAuxInvariant(t, err, test.want)
		})
	}

}

func TestProfilerAuxTypedMMCResponseTripleAndHardDominance(t *testing.T) {
	base := profilerAuxCasesByField()[4015]
	values := profilerAuxCloneValues(base.values)
	values[11] = profilerAuxTestValue{wire: 2, bytes: []byte(strings.Repeat("z", maxProfilerMMCResponseBytes+1))}
	payload := profilerAuxEncodeValues(values)
	payload = append(payload, profilerAuxWrongWire(3, base.values[3])...)
	payload = append(payload, profilerAuxEncodeField(7, base.values[7])...)
	event := profilerAuxTypedRecord(4015, payload)
	name, body, ok, issues, pair, handled, err := renderProfilerFtraceAuxEventWithTypedAudit(event)
	want := []profilerFtraceEventIssue{
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseWrongWire, 3),
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseDuplicate, 7),
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, 11),
	}
	if err != nil || !handled || !ok || name != "mmc_request_done" || !reflect.DeepEqual(issues, want) ||
		pair.Governed || !pair.Admitted {
		t.Fatalf("MMC response triple drifted: name=%q ok=%t handled=%t issues=%+v pair=%+v err=%v",
			name, ok, handled, issues, pair, err)
	}
	if body == "" || strings.Contains(body, "ABCD") || strings.Contains(body, "EFGH") ||
		strings.Contains(body, strings.Repeat("z", maxProfilerMMCResponseBytes+1)) || strings.Contains(body, "_resp") {
		t.Fatalf("lossy MMC response bytes escaped canonical body: %q", body)
	}
	labels, labelsOK := profilerFtraceEventIssueLabels(4015, issues)
	wantLabels := []string{"core_field3_wrong_wire", "core_field7_duplicate", "drop_response_field11_out_of_source_profile"}
	if !labelsOK || !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("MMC response triple labels drifted: got=%v ok=%t want=%v", labels, labelsOK, wantLabels)
	}

	hardPayload := append(append([]byte(nil), payload...), profilerAuxWrongWire(1, base.values[1])...)
	hardFixture := profilerAuxTypedFixture{
		name:  "hard-over-response-triple",
		event: profilerAuxTypedRecord(4015, hardPayload),
		issue: profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxFieldWrongWire, 1),
	}
	requireProfilerAuxTypedResult(t, hardFixture)
}

func TestProfilerAuxTypedSchemaOrderIsIndependentOfWireOrder(t *testing.T) {
	base := profilerAuxCasesByField()
	hardParts := [][]byte{
		profilerAuxWrongWire(6, base[4009].values[6]),
		profilerAuxEncodeField(3, base[4009].values[3]),
	}
	wantHard := profilerAuxTypedPayloadIssue(t, 4009, profilerFtraceEventIssueAuxFieldDuplicate, 3)
	for order, parts := range [][][]byte{hardParts, {hardParts[1], hardParts[0]}} {
		payload := append([]byte(nil), profilerAuxEncodeValues(base[4009].values)...)
		payload = append(payload, parts[0]...)
		payload = append(payload, parts[1]...)
		requireProfilerAuxTypedResult(t, profilerAuxTypedFixture{
			name: "hard-schema-order-" + string(rune('0'+order)), event: profilerAuxTypedRecord(4009, payload), issue: wantHard,
		})
	}

	responseParts := [][]byte{
		profilerAuxWrongWire(3, base[4015].values[3]),
		profilerAuxEncodeField(7, base[4015].values[7]),
		profilerAuxEncodeField(7, profilerAuxAlternateValue(base[4015].values[7])),
		profilerAuxEncodeField(11, profilerAuxTestValue{wire: 2, bytes: make([]byte, maxProfilerMMCResponseBytes+1)}),
	}
	reversed := append([][]byte(nil), responseParts...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	wantResponse := []profilerFtraceEventIssue{
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseWrongWire, 3),
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxResponseDuplicate, 7),
		profilerAuxTypedPayloadIssue(t, 4015, profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, 11),
	}
	for order, parts := range [][][]byte{responseParts, reversed} {
		payload := profilerAuxTypedPayload(base[4015], nil, 3, 7, 11)
		for _, part := range parts {
			payload = append(payload, part...)
		}
		name, body, ok, issues, _, handled, err :=
			renderProfilerFtraceAuxEventWithTypedAudit(profilerAuxTypedRecord(4015, payload))
		if err != nil || !handled || !ok || name != "mmc_request_done" || body == "" ||
			!reflect.DeepEqual(issues, wantResponse) {
			t.Fatalf("response schema order %d drifted: name=%q body=%q ok=%t handled=%t issues=%+v want=%+v err=%v",
				order, name, body, ok, handled, issues, wantResponse, err)
		}
	}
}

func TestProfilerAuxTypedF2FSTerminalNonKeyFailureKeepsExactLane(t *testing.T) {
	base := profilerAuxCasesByField()[4012]
	terminalNonKey := profilerAuxTypedPayload(base, nil, 4)
	terminalNonKey = append(terminalNonKey, byte(4<<3), 0x80)
	badEvent := profilerAuxTypedRecord(4012, terminalNonKey)
	_, _, ok, issues, badPair, handled, err := renderProfilerFtraceAuxEventWithTypedAudit(badEvent)
	want := profilerAuxTypedFixedIssue(t, 4012, profilerFtraceEventIssueAuxPayloadMalformedWire)
	if err != nil || !handled || ok || !reflect.DeepEqual(issues, []profilerFtraceEventIssue{want}) ||
		!badPair.Governed || badPair.Kind != pairRenderF2FS || !badPair.LaneKnown || badPair.Lane == "" || badPair.Admitted {
		t.Fatalf("terminal non-key F2FS failure lost exact lane: ok=%t handled=%t issues=%+v pair=%+v err=%v",
			ok, handled, issues, badPair, err)
	}

	goodEvent := profilerAuxTypedRecord(4012, profilerAuxEncodeValues(base.values))
	_, _, goodOK, goodIssues, goodPair, goodHandled, goodErr := renderProfilerFtraceAuxEventWithTypedAudit(goodEvent)
	if goodErr != nil || !goodHandled || !goodOK || len(goodIssues) != 0 ||
		!goodPair.LaneKnown || !goodPair.Admitted || goodPair.Lane != badPair.Lane {
		t.Fatalf("valid F2FS counterpart lane drifted: ok=%t handled=%t issues=%+v pair=%+v err=%v",
			goodOK, goodHandled, goodIssues, goodPair, goodErr)
	}
	sink, sinkErr := newTraceDBRowSink("", 8)
	if sinkErr != nil {
		t.Fatal(sinkErr)
	}
	defer sink.cleanup()
	badPair.poison(sink)
	if sink.poisoned[pairRenderF2FS] || len(sink.poisonedLanes[pairRenderF2FS]) != 1 ||
		!sink.poisonedLanes[pairRenderF2FS][badPair.Lane] {
		t.Fatalf("exact F2FS failure poisoned wrong scope: family=%v lanes=%v", sink.poisoned, sink.poisonedLanes)
	}

	unknownKey := protoVarint(99, 0)
	unknownTerminal := append([]byte(nil), profilerAuxEncodeValues(base.values)...)
	unknownTerminal = append(unknownTerminal, unknownKey[:len(unknownKey)-1]...)
	unknownTerminal = append(unknownTerminal, 0x80)
	_, _, unknownOK, unknownIssues, unknownPair, unknownHandled, unknownErr :=
		renderProfilerFtraceAuxEventWithTypedAudit(profilerAuxTypedRecord(4012, unknownTerminal))
	if unknownErr != nil || !unknownHandled || unknownOK ||
		!reflect.DeepEqual(unknownIssues, []profilerFtraceEventIssue{want}) ||
		!unknownPair.Governed || !unknownPair.LaneKnown || unknownPair.Lane != goodPair.Lane || unknownPair.Admitted {
		t.Fatalf("terminal unknown F2FS tail lost exact lane: ok=%t handled=%t issues=%+v pair=%+v err=%v",
			unknownOK, unknownHandled, unknownIssues, unknownPair, unknownErr)
	}

	keyMalformed := profilerAuxTypedPayload(base, nil, 1)
	keyMalformed = append(keyMalformed, byte(1<<3), 0x80)
	_, _, _, _, keyPair, _, keyErr := renderProfilerFtraceAuxEventWithTypedAudit(profilerAuxTypedRecord(4012, keyMalformed))
	if keyErr != nil || !keyPair.Governed || keyPair.LaneKnown || keyPair.Lane != "" {
		t.Fatalf("malformed F2FS key retained a lane: pair=%+v err=%v", keyPair, keyErr)
	}

	nonTerminal := profilerAuxTypedPayload(base, nil, 4)
	nonTerminal = append(nonTerminal, byte(4<<3))
	nonTerminal = append(nonTerminal, []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}...)
	_, _, _, _, nonTerminalPair, _, nonTerminalErr := renderProfilerFtraceAuxEventWithTypedAudit(profilerAuxTypedRecord(4012, nonTerminal))
	if nonTerminalErr != nil || !nonTerminalPair.Governed || nonTerminalPair.LaneKnown || nonTerminalPair.Lane != "" {
		t.Fatalf("non-terminal F2FS damage retained a lane: pair=%+v err=%v", nonTerminalPair, nonTerminalErr)
	}

	ownerUnknown := goodEvent
	ownerUnknown.HeaderOwnerKnown = false
	_, _, ownerOK, ownerIssues, ownerPair, _, ownerErr := renderProfilerFtraceAuxEventWithTypedAudit(ownerUnknown)
	if ownerErr != nil || !ownerOK || len(ownerIssues) != 0 || !ownerPair.Governed ||
		ownerPair.LaneKnown || ownerPair.Lane != "" || ownerPair.Admitted {
		t.Fatalf("owner-unknown F2FS row invented pair authority: ok=%t issues=%+v pair=%+v err=%v",
			ownerOK, ownerIssues, ownerPair, ownerErr)
	}
}

func TestProfilerAuxTypedCanonicalBoundaryIsPrintOnly(t *testing.T) {
	record := profilerAuxTypedRecord(1109, nil)
	emptyLine := traceDBFormatLine(record.Comm, record.PID, record.TGID, record.CPU, int64(record.TSNS),
		record.CommonFlags, record.CommonPreemptCount, "print: ")
	bufferBytes := maxTraceDBSystraceLineBytes - len(emptyLine)
	if bufferBytes <= 0 {
		t.Fatalf("aux canonical fixture overhead=%d exceeds cap=%d", len(emptyLine), maxTraceDBSystraceLineBytes)
	}
	exact := record
	exact.Payload = protoBytes(2, []byte(strings.Repeat("x", bufferBytes)))
	name, body, ok, issues, _, handled, err := renderProfilerFtraceAuxEventWithTypedAudit(exact)
	if err != nil || !handled || !ok || name != "print" || len(issues) != 0 {
		t.Fatalf("exact-cap aux row rejected: name=%q ok=%t handled=%t issues=%+v err=%v", name, ok, handled, issues, err)
	}
	exactLine := traceDBFormatLine(exact.Comm, exact.PID, exact.TGID, exact.CPU, int64(exact.TSNS),
		exact.CommonFlags, exact.CommonPreemptCount, name+": "+body)
	if len(exactLine) != maxTraceDBSystraceLineBytes {
		t.Fatalf("aux canonical exact-cap calibration drifted: len=%d want=%d", len(exactLine), maxTraceDBSystraceLineBytes)
	}

	tooLong := record
	tooLong.Payload = protoBytes(2, []byte(strings.Repeat("x", bufferBytes+1)))
	requireProfilerAuxTypedResult(t, profilerAuxTypedFixture{
		name: "print-cap-plus-one", event: tooLong,
		issue: profilerAuxTypedFixedIssue(t, 1109, profilerFtraceEventIssueAuxInvalidCanonicalLine),
	})
	for _, eventField := range []int{4009, 4010, 4011, 4012, 4015, 4016} {
		if issue, fixed := profilerFtraceEventFixedIssue(eventField, profilerFtraceEventIssueAuxInvalidCanonicalLine); fixed {
			t.Fatalf("bounded aux event %d minted canonical issue: %+v", eventField, issue)
		}
	}

	healthyPayload := protoBytes(2, []byte("B|41|Keep"))
	structured := protoMessage(2,
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(uint64(tooLong.TSNS), uint64(tooLong.PID), uint64(tooLong.TGID),
			tooLong.Comm, 1109, tooLong.Payload),
		syntheticTracePluginFtraceEvent(2_000, 41, 41, "healthy", 1109, healthyPayload),
	)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", structured),
	)
	defer sink.cleanup()
	coverage, entries := profilerEventCoverageByField(extracted, 1109)
	if entries != 1 || coverage.RowsRead != 2 || coverage.RowsEmitted != 1 ||
		coverage.FieldSources["degraded_invalid_canonical_aux_line_occurrences"] != "1" ||
		coverage.FieldSources["degraded_invalid_canonical_aux_line_affected_frames"] != "1" {
		t.Fatalf("aux canonical local-reject census drifted: entries=%d coverage=%+v", entries, coverage)
	}
	if extracted.SourceFailClosed || extracted.StructuredRows != 1 || sink.publishableRows() != 1 ||
		len(sink.rows) != 1 || !strings.Contains(sink.rows[0].line, "healthy") ||
		!strings.Contains(sink.rows[0].line, "print: B|41|Keep") {
		t.Fatalf("bad aux canonical row contaminated sibling: extracted=%+v sink=%+v rows=%+v",
			extracted, sink.stats, sink.rows)
	}
}
