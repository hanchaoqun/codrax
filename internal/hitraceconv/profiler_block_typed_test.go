package hitraceconv

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type profilerBlockTypedValue struct {
	wire int
	u64  uint64
	text string
}

type profilerBlockTypedFixture struct {
	name  string
	event profilerFtraceEventRecord
	issue profilerFtraceEventIssue
}

type profilerBlockTypedTuple struct {
	event int
	issue profilerFtraceEventIssue
}

func profilerBlockTypedRecord(eventField int, payload []byte) profilerFtraceEventRecord {
	return profilerFtraceEventRecord{
		CPU: 2, TSNS: 1_000, TGID: 40, PID: 40, HeaderOwnerKnown: true, HeaderOwnerPresent: true,
		Comm: "block-test", Field: eventField, Payload: payload,
	}
}

func profilerBlockTypedBaseValues(eventField int) map[int]profilerBlockTypedValue {
	values := map[int]profilerBlockTypedValue{
		1: {wire: 0, u64: 1},
		2: {wire: 0, u64: 2},
		3: {wire: 0, u64: 3},
	}
	switch eventField {
	case 202:
		values[4] = profilerBlockTypedValue{wire: 0}
		values[5] = profilerBlockTypedValue{wire: 2, text: "R"}
	case 204:
		values[4] = profilerBlockTypedValue{wire: 2, text: "R"}
		values[5] = profilerBlockTypedValue{wire: 2, text: "io"}
	case 205:
		values[4] = profilerBlockTypedValue{wire: 0, u64: 4}
		values[5] = profilerBlockTypedValue{wire: 0, u64: 5}
		values[6] = profilerBlockTypedValue{wire: 2, text: "R"}
	case 209:
		values[4] = profilerBlockTypedValue{wire: 0}
		values[5] = profilerBlockTypedValue{wire: 2, text: "R"}
		values[6] = profilerBlockTypedValue{wire: 2, text: "READ"}
	case 210, 211:
		values[4] = profilerBlockTypedValue{wire: 0, u64: 4}
		values[5] = profilerBlockTypedValue{wire: 2, text: "R"}
		values[6] = profilerBlockTypedValue{wire: 2, text: "io"}
		values[7] = profilerBlockTypedValue{wire: 2, text: "READ"}
	case 212:
		values[4] = profilerBlockTypedValue{wire: 0, u64: 4}
		values[5] = profilerBlockTypedValue{wire: 0, u64: 5}
		values[6] = profilerBlockTypedValue{wire: 0, u64: 6}
		values[7] = profilerBlockTypedValue{wire: 2, text: "R"}
	default:
		panic("invalid profiler block test event")
	}
	return values
}

func profilerBlockTypedEncodeField(payloadField int, value profilerBlockTypedValue) []byte {
	switch value.wire {
	case 0:
		return protoVarint(payloadField, value.u64)
	case 2:
		return protoBytes(payloadField, []byte(value.text))
	default:
		panic("invalid profiler block test wire")
	}
}

func profilerBlockTypedPayload(eventField int, changes map[int]profilerBlockTypedValue, omit ...int) []byte {
	values := profilerBlockTypedBaseValues(eventField)
	for payloadField, value := range changes {
		values[payloadField] = value
	}
	for _, payloadField := range omit {
		delete(values, payloadField)
	}
	fields := make([]int, 0, len(values))
	for payloadField := range values {
		fields = append(fields, payloadField)
	}
	sort.Ints(fields)
	var payload []byte
	for _, payloadField := range fields {
		payload = append(payload, profilerBlockTypedEncodeField(payloadField, values[payloadField])...)
	}
	return payload
}

func profilerBlockTypedRawKey(payloadField, wire int) []byte {
	value := uint64(payloadField<<3 | wire)
	var out []byte
	for value >= 0x80 {
		out = append(out, byte(value&0x7f)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}

func profilerBlockTypedMalformedField(payloadField, wire int) []byte {
	return append(profilerBlockTypedRawKey(payloadField, wire), 0x80)
}

func profilerBlockTypedWrongWire(payloadField, expectedWire int) []byte {
	if expectedWire == 0 {
		return protoBytes(payloadField, []byte{1})
	}
	return protoVarint(payloadField, 1)
}

func profilerBlockTypedFixedIssue(t *testing.T, eventField int, kind profilerFtraceEventIssueKind) profilerFtraceEventIssue {
	t.Helper()
	issue, ok := profilerFtraceEventFixedIssue(eventField, kind)
	if !ok {
		t.Fatalf("fixture fixed block issue rejected: event=%d kind=%d", eventField, kind)
	}
	return issue
}

func profilerBlockTypedPayloadIssue(t *testing.T, eventField int, kind profilerFtraceEventIssueKind, payloadField int) profilerFtraceEventIssue {
	t.Helper()
	issue, ok := profilerFtraceEventPayloadIssue(eventField, kind, payloadField)
	if !ok {
		t.Fatalf("fixture payload block issue rejected: event=%d kind=%d payload_field=%d", eventField, kind, payloadField)
	}
	return issue
}

func requireProfilerBlockTypedResult(t *testing.T, fixture profilerBlockTypedFixture) {
	t.Helper()
	name, body, ok, issues, handled, err := renderProfilerFtraceBlockEventWithTypedAudit(fixture.event)
	if err != nil || !handled {
		t.Fatalf("block typed choke failed: fixture=%s field=%d handled=%t err=%v",
			fixture.name, fixture.event.Field, handled, err)
	}
	wantOK := fixture.issue.Severity == profilerFtraceEventIssueAdmittedDisplay
	wantIssues := []profilerFtraceEventIssue{fixture.issue}
	if ok != wantOK || !reflect.DeepEqual(issues, wantIssues) ||
		!profilerFtraceEventIssueVerdictValid(fixture.event.Field, ok, issues) {
		t.Fatalf("block typed verdict drifted: fixture=%s field=%d ok=%t want_ok=%t issues=%+v want=%+v name=%q body=%q",
			fixture.name, fixture.event.Field, ok, wantOK, issues, wantIssues, name, body)
	}
	if ok && (name == "" || body == "") {
		t.Fatalf("admitted block row lost output: fixture=%s name=%q body=%q", fixture.name, name, body)
	}
	if !ok && (name != "" || body != "") {
		t.Fatalf("rejected block row retained output: fixture=%s name=%q body=%q", fixture.name, name, body)
	}

	typedName, typedBody, typedOK, typedIssues, typedErr := renderProfilerFtraceEventBodyWithTypedAudit(fixture.event)
	if typedErr != nil || typedName != name || typedBody != body || typedOK != ok || !reflect.DeepEqual(typedIssues, issues) {
		t.Fatalf("outer typed/block parity drifted: fixture=%s\nblock=(%q,%q,%t,%+v,%v)\nouter=(%q,%q,%t,%+v,%v)",
			fixture.name, name, body, ok, issues, err, typedName, typedBody, typedOK, typedIssues, typedErr)
	}
	labels, labelsOK := profilerFtraceEventIssueLabels(fixture.event.Field, issues)
	if !labelsOK || len(labels) != 1 {
		t.Fatalf("typed block issue has no unique compatibility label: fixture=%s issues=%+v labels=%v",
			fixture.name, issues, labels)
	}
	compatName, compatBody, compatOK, compatLabels := renderProfilerFtraceEventBodyWithAudit(fixture.event)
	if compatName != name || compatBody != body || compatOK != ok || !reflect.DeepEqual(compatLabels, labels) {
		t.Fatalf("compat label/block parity drifted: fixture=%s\nblock=(%q,%q,%t,%v)\ncompat=(%q,%q,%t,%v)",
			fixture.name, name, body, ok, labels, compatName, compatBody, compatOK, compatLabels)
	}
}

func profilerBlockTypedOverflowValue(role profilerFtraceBlockFieldRole) uint64 {
	switch role {
	case profilerFtraceBlockFieldSector, profilerFtraceBlockFieldOldSector:
		return uint64(math.MaxInt64) + 1
	case profilerFtraceBlockFieldError:
		return uint64(math.MaxInt32) + 1
	case profilerFtraceBlockFieldDev, profilerFtraceBlockFieldNRSector, profilerFtraceBlockFieldBytes,
		profilerFtraceBlockFieldOldDev, profilerFtraceBlockFieldNRBios:
		return uint64(math.MaxUint32) + 1
	default:
		panic("invalid profiler block range fixture")
	}
}

func profilerBlockTypedRawFixtures(t *testing.T) []profilerBlockTypedFixture {
	t.Helper()
	events := []int{202, 204, 205, 209, 210, 211, 212}
	fixtures := make([]profilerBlockTypedFixture, 0, 183)
	add := func(name string, eventField int, payload []byte, issue profilerFtraceEventIssue) {
		fixtures = append(fixtures, profilerBlockTypedFixture{
			name: name, event: profilerBlockTypedRecord(eventField, payload), issue: issue,
		})
	}

	for _, eventField := range events {
		add("whole-malformed", eventField, append(profilerBlockTypedPayload(eventField, nil), 0x80),
			profilerBlockTypedFixedIssue(t, eventField, profilerFtraceEventIssueBlockPayloadMalformedWire))
		for payloadField := 1; payloadField <= 7; payloadField++ {
			role, wire, known := profilerFtraceBlockFieldSchema(eventField, payloadField)
			if !known || profilerFtraceBlockDisplayRole(role) {
				continue
			}
			malformed := profilerBlockTypedPayload(eventField, nil, payloadField)
			malformed = append(malformed, profilerBlockTypedMalformedField(payloadField, wire)...)
			add("hard-malformed", eventField, malformed,
				profilerBlockTypedPayloadIssue(t, eventField, profilerFtraceEventIssueBlockFieldMalformedWire, payloadField))

			wrong := profilerBlockTypedPayload(eventField, nil, payloadField)
			wrong = append(wrong, profilerBlockTypedWrongWire(payloadField, wire)...)
			add("hard-wrong-wire", eventField, wrong,
				profilerBlockTypedPayloadIssue(t, eventField, profilerFtraceEventIssueBlockFieldWrongWire, payloadField))

			duplicate := profilerBlockTypedPayload(eventField, nil)
			duplicate = append(duplicate, profilerBlockTypedEncodeField(payloadField,
				profilerBlockTypedBaseValues(eventField)[payloadField])...)
			add("hard-duplicate", eventField, duplicate,
				profilerBlockTypedPayloadIssue(t, eventField, profilerFtraceEventIssueBlockFieldDuplicate, payloadField))

			if role == profilerFtraceBlockFieldRWBS {
				invalid := profilerBlockTypedPayload(eventField, map[int]profilerBlockTypedValue{
					payloadField: {wire: 2, text: "R|W"},
				})
				add("rwbs-semantic", eventField, invalid,
					profilerBlockTypedPayloadIssue(t, eventField, profilerFtraceEventIssueBlockFieldMissingOrInvalid, payloadField))
				continue
			}
			rangePayload := profilerBlockTypedPayload(eventField, map[int]profilerBlockTypedValue{
				payloadField: {wire: 0, u64: profilerBlockTypedOverflowValue(role)},
			})
			add("hard-range", eventField, rangePayload,
				profilerBlockTypedPayloadIssue(t, eventField, profilerFtraceEventIssueBlockFieldOutOfRange, payloadField))
		}

		for payloadField := 1; payloadField <= 7; payloadField++ {
			role, wire, known := profilerFtraceBlockFieldSchema(eventField, payloadField)
			if !known || !profilerFtraceBlockDisplayRole(role) {
				continue
			}
			var malformedKind, wrongKind, duplicateKind, unsafeKind profilerFtraceEventIssueKind
			unsafe := "bad]"
			if role == profilerFtraceBlockFieldComm {
				malformedKind = profilerFtraceEventIssueBlockCommMalformedWire
				wrongKind = profilerFtraceEventIssueBlockCommWrongWire
				duplicateKind = profilerFtraceEventIssueBlockCommDuplicate
				unsafeKind = profilerFtraceEventIssueBlockCommUnsafeOmitted
			} else {
				malformedKind = profilerFtraceEventIssueBlockCmdMalformedWire
				wrongKind = profilerFtraceEventIssueBlockCmdWrongWire
				duplicateKind = profilerFtraceEventIssueBlockCmdDuplicate
				unsafeKind = profilerFtraceEventIssueBlockCmdUnsafeOmitted
				unsafe = "bad)"
			}

			malformed := profilerBlockTypedPayload(eventField, nil, payloadField)
			malformed = append(malformed, profilerBlockTypedMalformedField(payloadField, wire)...)
			add("display-malformed", eventField, malformed, profilerBlockTypedFixedIssue(t, eventField, malformedKind))

			wrong := profilerBlockTypedPayload(eventField, nil, payloadField)
			wrong = append(wrong, profilerBlockTypedWrongWire(payloadField, wire)...)
			add("display-wrong-wire", eventField, wrong, profilerBlockTypedFixedIssue(t, eventField, wrongKind))

			duplicate := profilerBlockTypedPayload(eventField, nil)
			duplicate = append(duplicate, profilerBlockTypedEncodeField(payloadField,
				profilerBlockTypedBaseValues(eventField)[payloadField])...)
			add("display-duplicate", eventField, duplicate, profilerBlockTypedFixedIssue(t, eventField, duplicateKind))

			unsafePayload := profilerBlockTypedPayload(eventField, map[int]profilerBlockTypedValue{
				payloadField: {wire: 2, text: unsafe},
			})
			add("display-unsafe", eventField, unsafePayload, profilerBlockTypedFixedIssue(t, eventField, unsafeKind))
		}
	}

	for _, item := range []struct {
		event, display int
	}{{204, 5}, {209, 6}, {210, 6}, {211, 6}} {
		payload := profilerBlockTypedPayload(item.event, map[int]profilerBlockTypedValue{
			item.display: {wire: 2, text: strings.Repeat("x", maxTraceDBSystraceLineBytes)},
		})
		add("canonical-line", item.event, payload,
			profilerBlockTypedFixedIssue(t, item.event, profilerFtraceEventIssueBlockInvalidCanonicalLine))
	}

	if len(fixtures) != 183 {
		t.Fatalf("raw block typed fixture universe=%d want=183", len(fixtures))
	}
	return fixtures
}

func TestProfilerBlockTypedCleanHelperAndCompatibilityParity(t *testing.T) {
	wantBodies := map[int]string{
		202: "0,1 R 2 + 3 [0]",
		204: "0,1 R 2 + 3 [io]",
		205: "0,1 R 2 + 3 <- (0,4) 5",
		209: "0,1 R (READ) 2 + 3 [0]",
		210: "0,1 R 4 (READ) 2 + 3 [io]",
		211: "0,1 R 4 (READ) 2 + 3 [io]",
		212: "0,1 R 2 + 3 <- (0,4) 5 6",
	}
	for eventField, wantBody := range wantBodies {
		event := profilerBlockTypedRecord(eventField, profilerBlockTypedPayload(eventField, nil))
		name, body, ok, issues, handled, err := renderProfilerFtraceBlockEventWithTypedAudit(event)
		_, wantName, _ := blockRenderKindForProfilerField(eventField)
		if err != nil || !handled || !ok || name != wantName || body != wantBody || len(issues) != 0 {
			t.Fatalf("clean typed block drifted: event=%d name=%q body=%q ok=%t handled=%t issues=%+v err=%v",
				eventField, name, body, ok, handled, issues, err)
		}
		outerName, outerBody, outerOK, outerIssues, outerErr := renderProfilerFtraceEventBodyWithTypedAudit(event)
		if outerErr != nil || outerName != name || outerBody != body || outerOK != ok || !reflect.DeepEqual(outerIssues, issues) {
			t.Fatalf("clean outer/block parity drifted: event=%d outer=(%q,%q,%t,%+v,%v)",
				eventField, outerName, outerBody, outerOK, outerIssues, outerErr)
		}
		compatName, compatBody, compatOK, labels := renderProfilerFtraceEventBodyWithAudit(event)
		if compatName != name || compatBody != body || compatOK != ok || len(labels) != 0 {
			t.Fatalf("clean compatibility/block parity drifted: event=%d compat=(%q,%q,%t,%v)",
				eventField, compatName, compatBody, compatOK, labels)
		}
	}
	if _, _, _, issues, handled, err := renderProfilerFtraceBlockEventWithTypedAudit(
		profilerBlockTypedRecord(201, nil)); err != nil || handled || len(issues) != 0 {
		t.Fatalf("foreign event entered block authority: handled=%t issues=%+v err=%v", handled, issues, err)
	}
}

func TestProfilerBlockTypedRawProducerClosureIsExactly183(t *testing.T) {
	fixtures := profilerBlockTypedRawFixtures(t)
	produced := make(map[profilerBlockTypedTuple]string, len(fixtures))
	perEvent := map[int]int{}
	seenKinds := map[profilerFtraceEventIssueKind]bool{}
	kindCounts := map[profilerFtraceEventIssueKind]int{}
	for _, fixture := range fixtures {
		requireProfilerBlockTypedResult(t, fixture)
		tuple := profilerBlockTypedTuple{event: fixture.event.Field, issue: fixture.issue}
		if previous, exists := produced[tuple]; exists {
			t.Fatalf("raw block producer tuple duplicated: event=%d issue=%+v first=%q next=%q",
				tuple.event, tuple.issue, previous, fixture.name)
		}
		produced[tuple] = fixture.name
		perEvent[tuple.event]++
		seenKinds[tuple.issue.Kind] = true
		kindCounts[tuple.issue.Kind]++
	}
	wantPerEvent := map[int]int{202: 21, 204: 22, 205: 25, 209: 26, 210: 30, 211: 30, 212: 29}
	if !reflect.DeepEqual(perEvent, wantPerEvent) {
		t.Fatalf("block raw producer per-event census drifted: got=%v want=%v", perEvent, wantPerEvent)
	}
	if len(seenKinds) != 15 {
		t.Fatalf("block raw producer kind census=%d want=15: %v", len(seenKinds), seenKinds)
	}
	wantKindCounts := map[profilerFtraceEventIssueKind]int{
		profilerFtraceEventIssueBlockPayloadMalformedWire:  7,
		profilerFtraceEventIssueBlockFieldMalformedWire:    37,
		profilerFtraceEventIssueBlockFieldWrongWire:        37,
		profilerFtraceEventIssueBlockFieldDuplicate:        37,
		profilerFtraceEventIssueBlockFieldOutOfRange:       30,
		profilerFtraceEventIssueBlockFieldMissingOrInvalid: 7,
		profilerFtraceEventIssueBlockInvalidCanonicalLine:  4,
		profilerFtraceEventIssueBlockCommMalformedWire:     3,
		profilerFtraceEventIssueBlockCommWrongWire:         3,
		profilerFtraceEventIssueBlockCommDuplicate:         3,
		profilerFtraceEventIssueBlockCommUnsafeOmitted:     3,
		profilerFtraceEventIssueBlockCmdMalformedWire:      3,
		profilerFtraceEventIssueBlockCmdWrongWire:          3,
		profilerFtraceEventIssueBlockCmdDuplicate:          3,
		profilerFtraceEventIssueBlockCmdUnsafeOmitted:      3,
	}
	if !reflect.DeepEqual(kindCounts, wantKindCounts) {
		t.Fatalf("block raw producer kind cardinality drifted: got=%v want=%v", kindCounts, wantKindCounts)
	}

	legal := make(map[profilerBlockTypedTuple]bool, 183)
	for eventField := range wantPerEvent {
		for kind := profilerFtraceEventIssueBlockPayloadMalformedWire; kind <= profilerFtraceEventIssueBlockCmdUnsafeOmitted; kind++ {
			for payloadField := 0; payloadField <= math.MaxUint8; payloadField++ {
				for severity := profilerFtraceEventIssueHardReject; severity < profilerFtraceEventIssueSeverityCount; severity++ {
					issue := profilerFtraceEventIssue{Kind: kind, PayloadField: uint8(payloadField), Severity: severity}
					if issue.validFor(eventField) {
						legal[profilerBlockTypedTuple{event: eventField, issue: issue}] = true
					}
				}
			}
		}
	}
	if len(legal) != 183 || len(produced) != len(legal) {
		t.Fatalf("block legal/source closure cardinality drifted: legal=%d produced=%d", len(legal), len(produced))
	}
	for tuple := range legal {
		if _, ok := produced[tuple]; !ok {
			t.Fatalf("legal block tuple has no raw producer: event=%d issue=%+v", tuple.event, tuple.issue)
		}
	}
	for tuple, fixture := range produced {
		if !legal[tuple] {
			t.Fatalf("raw block producer escaped legal closure: fixture=%s event=%d issue=%+v", fixture, tuple.event, tuple.issue)
		}
	}
}

func requireProfilerBlockInvariant(t *testing.T, err error, want string) {
	t.Helper()
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != want {
		t.Fatalf("block invariant error=%T %v reason=%q want=%q", err, err, reason, want)
	}
}

func TestProfilerBlockTypedIssueSetCapacityTwoAndCorruptionFailClosed(t *testing.T) {
	if profilerFtraceBlockIssuesPerEvent != 2 {
		t.Fatalf("block issue capacity=%d want=2", profilerFtraceBlockIssuesPerEvent)
	}
	comm := profilerBlockTypedFixedIssue(t, 210, profilerFtraceEventIssueBlockCommWrongWire)
	cmd := profilerBlockTypedFixedIssue(t, 210, profilerFtraceEventIssueBlockCmdDuplicate)
	hard := profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldWrongWire, 1)
	var full profilerFtraceBlockIssueSet
	if err := full.add(210, comm); err != nil {
		t.Fatal(err)
	}
	if err := full.add(210, cmd); err != nil {
		t.Fatal(err)
	}
	if full.Count != 2 {
		t.Fatalf("valid dual-display issue set drifted: %+v", full)
	}
	// Physical wire order is not diagnostic order. The producer must emit the
	// two independently degraded display endpoints in schema order f6 then f7.
	dualPayload := profilerBlockTypedPayload(210, nil, 6, 7)
	dualPayload = append(dualPayload, protoVarint(7, 1)...)
	dualPayload = append(dualPayload, protoVarint(6, 1)...)
	name, body, ok, produced, handled, producedErr := renderProfilerFtraceBlockEventWithTypedAudit(
		profilerBlockTypedRecord(210, dualPayload))
	wantProduced := []profilerFtraceEventIssue{
		profilerBlockTypedFixedIssue(t, 210, profilerFtraceEventIssueBlockCommWrongWire),
		profilerBlockTypedFixedIssue(t, 210, profilerFtraceEventIssueBlockCmdWrongWire),
	}
	if producedErr != nil || !handled || !ok || name != "block_rq_insert" ||
		body != "0,1 R 4 () 2 + 3 []" || !reflect.DeepEqual(produced, wantProduced) {
		t.Fatalf("dual-display producer order drifted: name=%q body=%q ok=%t handled=%t issues=%+v want=%+v err=%v",
			name, body, ok, handled, produced, wantProduced, producedErr)
	}
	checked, err := full.checked(210)
	if err != nil || !reflect.DeepEqual(checked, []profilerFtraceEventIssue{comm, cmd}) {
		t.Fatalf("dual-display checked set drifted: got=%+v err=%v", checked, err)
	}
	checked[0] = cmd
	if full.Issues[0] != comm {
		t.Fatal("checked block issues alias fixed storage")
	}
	before := full
	if err := full.add(210, hard); err == nil {
		t.Fatal("full block issue set accepted overflow")
	} else {
		requireProfilerBlockInvariant(t, err, "profiler_block_issue_overflow")
	}
	if full != before {
		t.Fatalf("overflow partially mutated block set: before=%+v after=%+v", before, full)
	}

	foreign, ok := profilerFtraceEventFixedIssue(210, profilerFtraceEventIssueEnvelopeCommInvalid)
	if !ok {
		t.Fatal("foreign envelope fixture rejected")
	}
	wrongSeverity := comm
	wrongSeverity.Severity = profilerFtraceEventIssueHardReject
	wrongEndpoint := profilerBlockTypedFixedIssue(t, 204, profilerFtraceEventIssueBlockCommWrongWire)
	endpointConflict := profilerBlockTypedFixedIssue(t, 210, profilerFtraceEventIssueBlockCommDuplicate)
	corruptions := []struct {
		name   string
		set    profilerFtraceBlockIssueSet
		event  int
		reason string
	}{
		{"count", profilerFtraceBlockIssueSet{Count: 3}, 210, "profiler_block_issue_count_invalid"},
		{"tail", profilerFtraceBlockIssueSet{Issues: [2]profilerFtraceEventIssue{comm}}, 210, "profiler_block_issue_count_invalid"},
		{"foreign event", profilerFtraceBlockIssueSet{}, 201, "profiler_block_issue_schema_invalid"},
		{"foreign tuple", profilerFtraceBlockIssueSet{Count: 1, Issues: [2]profilerFtraceEventIssue{foreign}}, 210, "profiler_block_issue_schema_invalid"},
		{"wrong event endpoint", profilerFtraceBlockIssueSet{Count: 1, Issues: [2]profilerFtraceEventIssue{wrongEndpoint}}, 210, "profiler_block_issue_schema_invalid"},
		{"severity", profilerFtraceBlockIssueSet{Count: 1, Issues: [2]profilerFtraceEventIssue{wrongSeverity}}, 210, "profiler_block_issue_schema_invalid"},
		{"order", profilerFtraceBlockIssueSet{Count: 2, Issues: [2]profilerFtraceEventIssue{cmd, comm}}, 210, "profiler_block_issue_order_invalid"},
		{"duplicate", profilerFtraceBlockIssueSet{Count: 2, Issues: [2]profilerFtraceEventIssue{comm, comm}}, 210, "profiler_block_issue_duplicate"},
		{"endpoint", profilerFtraceBlockIssueSet{Count: 2, Issues: [2]profilerFtraceEventIssue{comm, endpointConflict}}, 210, "profiler_block_issue_endpoint_conflict"},
		{"cross arm", profilerFtraceBlockIssueSet{Count: 2, Issues: [2]profilerFtraceEventIssue{hard, cmd}}, 210, "profiler_block_issue_arm_invalid"},
		{"zero issue", profilerFtraceBlockIssueSet{Count: 1}, 210, "profiler_block_issue_schema_invalid"},
	}
	for _, test := range corruptions {
		t.Run(test.name, func(t *testing.T) {
			requireProfilerBlockInvariant(t, test.set.validate(test.event), test.reason)
		})
	}

	prospective := []struct {
		name   string
		first  profilerFtraceEventIssue
		second profilerFtraceEventIssue
	}{
		{"order", cmd, comm},
		{"endpoint", comm, endpointConflict},
		{"cross hard-display", hard, cmd},
		{"cross display-hard", comm, hard},
		{"duplicate", comm, comm},
	}
	for _, test := range prospective {
		t.Run("prospective/"+test.name, func(t *testing.T) {
			var set profilerFtraceBlockIssueSet
			if err := set.add(210, test.first); err != nil {
				t.Fatalf("first add failed: %v", err)
			}
			before := set
			if err := set.add(210, test.second); err == nil || set != before {
				t.Fatalf("invalid prospective add mutated block set: err=%v before=%+v after=%+v", err, before, set)
			}
		})
	}
	for _, issue := range []profilerFtraceEventIssue{foreign, wrongSeverity, wrongEndpoint} {
		var set profilerFtraceBlockIssueSet
		before := set
		if err := set.add(210, issue); err == nil || set != before {
			t.Fatalf("invalid empty-set add mutated block set: issue=%+v err=%v before=%+v after=%+v",
				issue, err, before, set)
		}
	}
}

func TestProfilerBlockTypedTerminalDisplayMalformedAndNonterminalWhole(t *testing.T) {
	for _, item := range []struct {
		event, payloadField int
		kind                profilerFtraceEventIssueKind
	}{
		{204, 5, profilerFtraceEventIssueBlockCommMalformedWire},
		{209, 6, profilerFtraceEventIssueBlockCmdMalformedWire},
		{210, 6, profilerFtraceEventIssueBlockCommMalformedWire},
		{210, 7, profilerFtraceEventIssueBlockCmdMalformedWire},
		{211, 6, profilerFtraceEventIssueBlockCommMalformedWire},
		{211, 7, profilerFtraceEventIssueBlockCmdMalformedWire},
	} {
		t.Run(fmt.Sprintf("field%d/payload%d", item.event, item.payloadField), func(t *testing.T) {
			_, wire, _ := profilerFtraceBlockFieldSchema(item.event, item.payloadField)
			terminal := profilerBlockTypedPayload(item.event, nil, item.payloadField)
			terminal = append(terminal, profilerBlockTypedMalformedField(item.payloadField, wire)...)
			requireProfilerBlockTypedResult(t, profilerBlockTypedFixture{
				name: "terminal-display", event: profilerBlockTypedRecord(item.event, terminal),
				issue: profilerBlockTypedFixedIssue(t, item.event, item.kind),
			})

			nonterminal := profilerBlockTypedPayload(item.event, nil, item.payloadField)
			nonterminal = append(nonterminal, profilerBlockTypedRawKey(item.payloadField, 3)...)
			nonterminal = append(nonterminal, protoVarint(99, 1)...)
			requireProfilerBlockTypedResult(t, profilerBlockTypedFixture{
				name: "nonterminal-display", event: profilerBlockTypedRecord(item.event, nonterminal),
				issue: profilerBlockTypedFixedIssue(t, item.event, profilerFtraceEventIssueBlockPayloadMalformedWire),
			})
		})
	}
}

func TestProfilerBlockTypedDescriptorFailuresRemainInternal(t *testing.T) {
	event := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))
	original := profilerFtraceEventDescriptors[211]
	for _, test := range []struct {
		name       string
		descriptor profilerFtraceEventDescriptor
		present    bool
		reason     string
	}{
		{name: "missing", present: false, reason: "missing_block_descriptor"},
		{name: "field", present: true, descriptor: profilerFtraceEventDescriptor{
			Field: 210, Family: original.Family, Name: original.Name,
		}, reason: "mismatched_block_descriptor_field"},
		{name: "family", present: true, descriptor: profilerFtraceEventDescriptor{
			Field: 211, Family: "storage", Name: original.Name,
		}, reason: "invalid_block_descriptor"},
		{name: "name", present: true, descriptor: profilerFtraceEventDescriptor{
			Field: 211, Family: original.Family, Name: "block_rq_insert",
		}, reason: "invalid_block_descriptor"},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() { profilerFtraceEventDescriptors[211] = original }()
			if test.present {
				profilerFtraceEventDescriptors[211] = test.descriptor
			} else {
				delete(profilerFtraceEventDescriptors, 211)
			}
			name, body, ok, issues, handled, err := renderProfilerFtraceBlockEventWithTypedAudit(event)
			if !handled || ok || name != "" || body != "" || len(issues) != 0 {
				t.Fatalf("descriptor invariant leaked a source verdict: name=%q body=%q ok=%t handled=%t issues=%+v err=%v",
					name, body, ok, handled, issues, err)
			}
			requireProfilerBlockInvariant(t, err, test.reason)
		})
	}
}

func profilerBlockTypedCanonicalFixture(t *testing.T, eventField, displayField, extra int) profilerFtraceEventRecord {
	t.Helper()
	probe := profilerBlockTypedRecord(eventField, profilerBlockTypedPayload(eventField,
		map[int]profilerBlockTypedValue{displayField: {wire: 2, text: "x"}}))
	name, body, ok, issues, handled, err := renderProfilerFtraceBlockEventWithTypedAudit(probe)
	if err != nil || !handled || !ok || len(issues) != 0 {
		t.Fatalf("block canonical probe rejected: event=%d handled=%t ok=%t issues=%+v err=%v",
			eventField, handled, ok, issues, err)
	}
	probeLine := traceDBFormatLine(probe.Comm, probe.PID, probe.TGID, probe.CPU, int64(probe.TSNS),
		probe.CommonFlags, probe.CommonPreemptCount, name+": "+body)
	valueBytes := maxTraceDBSystraceLineBytes - (len(probeLine) - 1) + extra
	if valueBytes <= 0 || valueBytes > maxTraceDBSystraceLineBytes {
		t.Fatalf("block canonical calibration invalid: event=%d bytes=%d probe_len=%d", eventField, valueBytes, len(probeLine))
	}
	return profilerBlockTypedRecord(eventField, profilerBlockTypedPayload(eventField,
		map[int]profilerBlockTypedValue{displayField: {wire: 2, text: strings.Repeat("x", valueBytes)}}))
}

func TestProfilerBlockTypedCanonicalBoundaryAllReachableAndContainerLocality(t *testing.T) {
	items := []struct {
		event, display int
	}{{204, 5}, {209, 6}, {210, 6}, {211, 6}}
	type pair struct {
		exact, over profilerFtraceEventRecord
	}
	fixtures := make(map[int]pair, len(items))
	for _, item := range items {
		exact := profilerBlockTypedCanonicalFixture(t, item.event, item.display, 0)
		name, body, ok, issues, handled, err := renderProfilerFtraceBlockEventWithTypedAudit(exact)
		if err != nil || !handled || !ok || len(issues) != 0 {
			t.Fatalf("exact-cap block row rejected: event=%d handled=%t ok=%t issues=%+v err=%v",
				item.event, handled, ok, issues, err)
		}
		exactLine := traceDBFormatLine(exact.Comm, exact.PID, exact.TGID, exact.CPU, int64(exact.TSNS),
			exact.CommonFlags, exact.CommonPreemptCount, name+": "+body)
		if len(exactLine) != maxTraceDBSystraceLineBytes {
			t.Fatalf("block exact-cap drifted: event=%d len=%d want=%d", item.event, len(exactLine), maxTraceDBSystraceLineBytes)
		}
		over := profilerBlockTypedCanonicalFixture(t, item.event, item.display, 1)
		requireProfilerBlockTypedResult(t, profilerBlockTypedFixture{
			name: "cap-plus-one", event: over,
			issue: profilerBlockTypedFixedIssue(t, item.event, profilerFtraceEventIssueBlockInvalidCanonicalLine),
		})
		fixtures[item.event] = pair{exact: exact, over: over}
	}
	for _, eventField := range []int{202, 205, 212} {
		if issue, fixed := profilerFtraceEventFixedIssue(eventField, profilerFtraceEventIssueBlockInvalidCanonicalLine); fixed {
			t.Fatalf("bounded block event %d minted canonical issue: %+v", eventField, issue)
		}
	}

	parts := [][]byte{protoVarint(1, 2)}
	for index, item := range items {
		fixture := fixtures[item.event]
		parts = append(parts,
			syntheticTracePluginFtraceEvent(uint64(index*2+1)*1_000, 40, 40, "bad", item.event, fixture.over.Payload),
			syntheticTracePluginFtraceEvent(uint64(index*2+2)*1_000, 40, 40, "healthy", item.event,
				profilerBlockTypedPayload(item.event, nil)),
		)
	}
	structured := protoMessage(2, parts...)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", structured),
	)
	defer sink.cleanup()
	for _, item := range items {
		coverage, entries := profilerEventCoverageByField(extracted, item.event)
		if entries != 1 || coverage.RowsRead != 2 || coverage.RowsEmitted != 1 ||
			coverage.FieldSources["degraded_invalid_canonical_block_line_occurrences"] != "1" ||
			coverage.FieldSources["degraded_invalid_canonical_block_line_affected_frames"] != "1" {
			t.Fatalf("block canonical local-reject census drifted: field=%d entries=%d coverage=%+v",
				item.event, entries, coverage)
		}
	}
	// Canonical cap+1 is a hard endpoint hole. The new complete-capture
	// barrier must therefore quarantine the healthy row on the same RQ/BIO
	// lane; inventory-only rq_insert remains publishable.
	if extracted.SourceFailClosed || extracted.StructuredRows != 1 || sink.publishableRows() != 1 ||
		len(sink.rows) != 4 || sink.poisoned[pairRenderBlock] || len(profilerTestPoisonedLanes(sink)[pairRenderBlock]) != 2 {
		t.Fatalf("canonical Block hole did not isolate pair lanes and retain inventory: extracted=%+v sink=%+v rows=%+v lanes=%v",
			extracted, sink.stats, sink.rows, profilerTestPoisonedLanes(sink))
	}
	published := 0
	for _, row := range sink.rows {
		if !strings.Contains(row.line, "healthy") {
			t.Fatalf("non-healthy block row survived local rejection: %q", row.line)
		}
		if sink.rowPublishable(row) {
			published++
			if !strings.Contains(row.line, "block_rq_insert:") {
				t.Fatalf("pair endpoint survived its canonical capture hole: %q", row.line)
			}
		}
	}
	if published != 1 {
		t.Fatalf("publishable inventory count=%d rows=%+v", published, sink.rows)
	}
}

func TestProfilerBlockTypedSingleWalkAndNoDynamicReasonAST(t *testing.T) {
	source, err := os.ReadFile("block_render.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "block_render.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var decode *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "decodeProfilerBlockPayloadWithTypedAuditIntoContext" {
			decode = function
			break
		}
	}
	if decode == nil || decode.Body == nil {
		t.Fatal("typed block decoder AST missing")
	}
	walkCalls := 0
	forbidden := map[string]int{}
	ast.Inspect(decode.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch callee := call.Fun.(type) {
		case *ast.Ident:
			switch callee.Name {
			case "walkProfilerProtoFieldsContext":
				walkCalls++
			case "protoScalarUint", "protoScalarString", "profilerFtraceEventIssueFromLegacy":
				forbidden[callee.Name]++
			}
		case *ast.SelectorExpr:
			if ident, ok := callee.X.(*ast.Ident); ok && ident.Name == "fmt" && callee.Sel.Name == "Sprintf" {
				forbidden["fmt.Sprintf"]++
			}
		}
		return true
	})
	if walkCalls != 1 || len(forbidden) != 0 {
		t.Fatalf("typed block decoder AST authority drifted: walk_calls=%d forbidden=%v", walkCalls, forbidden)
	}
	decodeText := string(source[decode.Pos()-1 : decode.End()-1])
	if !strings.Contains(decodeText, "var fields [8]profilerFtraceBlockFieldState") ||
		strings.Contains(decodeText, "[]string") || strings.Contains(decodeText, `fmt.Sprintf("core_field`) {
		t.Fatalf("typed block decoder lost fixed state or restored dynamic reasons:\n%s", decodeText)
	}
}
