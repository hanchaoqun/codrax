package hitraceconv

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type profilerCoreTypedFixture struct {
	name  string
	event profilerFtraceEventRecord
	issue profilerFtraceEventIssue
}

type profilerCoreTypedTuple struct {
	event int
	issue profilerFtraceEventIssue
}

func profilerCoreTypedHardIssue(t *testing.T, event int, kind profilerFtraceEventIssueKind, payloadField int) profilerFtraceEventIssue {
	t.Helper()
	issue, ok := profilerFtraceEventPayloadIssue(event, kind, payloadField)
	if !ok || issue.Severity != profilerFtraceEventIssueHardReject {
		t.Fatalf("fixture hard issue rejected: event=%d kind=%d payload_field=%d issue=%+v ok=%t",
			event, kind, payloadField, issue, ok)
	}
	return issue
}

func profilerCoreTypedFixedIssue(t *testing.T, event int, kind profilerFtraceEventIssueKind) profilerFtraceEventIssue {
	t.Helper()
	issue, ok := profilerFtraceEventFixedIssue(event, kind)
	if !ok {
		t.Fatalf("fixture fixed issue rejected: event=%d kind=%d issue=%+v", event, kind, issue)
	}
	return issue
}

func profilerCoreTypedBaseCases() map[int]profilerCoreTestCase {
	out := make(map[int]profilerCoreTestCase, len(profilerStructuredCoreSchemas))
	for _, test := range profilerCoreTestCases() {
		out[test.field] = test
	}
	return out
}

func profilerCoreTypedPayload(base profilerCoreTestCase, changes map[int]profilerCoreTestValue, omit ...int) []byte {
	values := make(map[int]profilerCoreTestValue, len(base.values)+len(changes))
	for field, value := range base.values {
		values[field] = value
	}
	for field, value := range changes {
		values[field] = value
	}
	for _, field := range omit {
		delete(values, field)
	}
	return profilerCoreEncodeValues(values)
}

func profilerCoreTypedRecord(event int, payload []byte) profilerFtraceEventRecord {
	return profilerFtraceEventRecord{
		CPU: 2, TSNS: 1_000, TGID: 100, PID: 100, HeaderOwnerKnown: true,
		Comm: "core-test", Field: event, Payload: payload,
	}
}

func profilerCoreTypedRawFixtures(t *testing.T) []profilerCoreTypedFixture {
	t.Helper()
	base := profilerCoreTypedBaseCases()
	fixtures := make([]profilerCoreTypedFixture, 0, 163)
	add := func(name string, event int, payload []byte, issue profilerFtraceEventIssue) {
		fixtures = append(fixtures, profilerCoreTypedFixture{
			name: name, event: profilerCoreTypedRecord(event, payload), issue: issue,
		})
	}

	events := make([]int, 0, len(base))
	for event := range base {
		events = append(events, event)
	}
	sort.Ints(events)

	// Whole-message damage is source-reachable for every governed event.
	for _, event := range events {
		payload := append(profilerCoreTypedPayload(base[event], nil), 0x80)
		add("whole-malformed", event, payload,
			profilerCoreTypedFixedIssue(t, event, profilerFtraceEventIssueCorePayloadMalformedWire))
	}

	// All 41 non-display schema endpoints have exact wrong-wire and duplicate
	// tuples. Duplicate has one tuple regardless of same/conflicting raw value;
	// the legacy shape matrix separately pins both byte forms.
	for _, event := range events {
		test := base[event]
		fields := make([]int, 0, len(profilerStructuredCoreSchemas[event]))
		for field := range profilerStructuredCoreSchemas[event] {
			if !profilerCoreDisplayField(event, field) {
				fields = append(fields, field)
			}
		}
		sort.Ints(fields)
		for _, field := range fields {
			value := test.values[field]
			wrong := profilerCoreTypedPayload(test, nil, field)
			wrong = append(wrong, profilerCoreWrongWire(field, value)...)
			add("wrong-wire", event, wrong,
				profilerCoreTypedHardIssue(t, event, profilerFtraceEventIssueCoreFieldWrongWire, field))

			duplicate := append(profilerCoreTypedPayload(test, nil), profilerCoreEncodeField(field, value)...)
			add("duplicate", event, duplicate,
				profilerCoreTypedHardIssue(t, event, profilerFtraceEventIssueCoreFieldDuplicate, field))
		}
	}

	// Only Binder's seven source-width fields and wakeup success field4 use
	// the generic CoreFieldOutOfRange token.
	for field := 1; field <= 7; field++ {
		payload := profilerCoreTypedPayload(base[113], map[int]profilerCoreTestValue{
			field: profilerCoreVarint(uint64(math.MaxUint32) + 1),
		})
		add("out-of-range", 113, payload,
			profilerCoreTypedHardIssue(t, 113, profilerFtraceEventIssueCoreFieldOutOfRange, field))
	}
	for _, event := range []int{2420, 2421, 2422} {
		payload := profilerCoreTypedPayload(base[event], map[int]profilerCoreTestValue{
			4: profilerCoreVarint(uint64(math.MaxUint32) + 1),
		})
		add("out-of-range", event, payload,
			profilerCoreTypedHardIssue(t, event, profilerFtraceEventIssueCoreFieldOutOfRange, 4))
	}

	semantic := []struct {
		name    string
		event   int
		changes map[int]profilerCoreTestValue
		omit    []int
		kind    profilerFtraceEventIssueKind
	}{
		{name: "transaction-id", event: 113, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(0)}, kind: profilerFtraceEventIssueCoreInvalidTransactionID},
		{name: "transaction-id", event: 119, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(0)}, kind: profilerFtraceEventIssueCoreInvalidTransactionID},
		{name: "transaction-endpoint", event: 113, changes: map[int]profilerCoreTestValue{2: profilerCoreVarint(math.MaxUint64)}, kind: profilerFtraceEventIssueCoreInvalidTransactionEndpoint},
		{name: "reply", event: 113, changes: map[int]profilerCoreTestValue{5: profilerCoreVarint(2)}, kind: profilerFtraceEventIssueCoreInvalidReply},
		{name: "reason", event: 1400, omit: []int{1}, kind: profilerFtraceEventIssueCoreMissingOrInvalidReason},
		{name: "reason", event: 1401, omit: []int{1}, kind: profilerFtraceEventIssueCoreMissingOrInvalidReason},
		{name: "reason", event: 1402, omit: []int{2}, kind: profilerFtraceEventIssueCoreMissingOrInvalidReason},
		{name: "irq", event: 1500, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(math.MaxUint64)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidIRQ},
		{name: "irq", event: 1501, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(math.MaxUint64)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidIRQ},
		{name: "irq-name", event: 1500, omit: []int{2}, kind: profilerFtraceEventIssueCoreMissingOrInvalidIRQName},
		{name: "ret", event: 1501, changes: map[int]profilerCoreTestValue{2: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidRet},
		{name: "vec", event: 1502, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(10)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidVec},
		{name: "vec", event: 1503, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(10)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidVec},
		{name: "vec", event: 1504, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(10)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidVec},
		{name: "state", event: 2003, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidState},
		{name: "state", event: 2005, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidState},
		{name: "cpu", event: 2003, changes: map[int]profilerCoreTestValue{2: profilerCoreVarint(uint64(maxTraceDBCPUIndex + 1))}, kind: profilerFtraceEventIssueCoreMissingOrInvalidCPUID},
		{name: "cpu", event: 2004, changes: map[int]profilerCoreTestValue{3: profilerCoreVarint(uint64(maxTraceDBCPUIndex + 1))}, kind: profilerFtraceEventIssueCoreMissingOrInvalidCPUID},
		{name: "cpu", event: 2005, changes: map[int]profilerCoreTestValue{2: profilerCoreVarint(uint64(maxTraceDBCPUIndex + 1))}, kind: profilerFtraceEventIssueCoreMissingOrInvalidCPUID},
		{name: "limits-profile", event: 2004, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, kind: profilerFtraceEventIssueCoreInvalidLimitsProfile},
		{name: "limits-order", event: 2004, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(2_000_000), 2: profilerCoreVarint(1_000_000)}, kind: profilerFtraceEventIssueCoreInvalidLimitsOrder},
		{name: "pid", event: 2420, changes: map[int]profilerCoreTestValue{2: profilerCoreVarint(math.MaxUint64)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidPID},
		{name: "pid", event: 2421, changes: map[int]profilerCoreTestValue{2: profilerCoreVarint(math.MaxUint64)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidPID},
		{name: "pid", event: 2422, changes: map[int]profilerCoreTestValue{2: profilerCoreVarint(math.MaxUint64)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidPID},
		{name: "pid", event: 4002, changes: map[int]profilerCoreTestValue{1: profilerCoreVarint(math.MaxUint64)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidPID},
		{name: "priority", event: 2420, changes: map[int]profilerCoreTestValue{3: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidPriority},
		{name: "priority", event: 2421, changes: map[int]profilerCoreTestValue{3: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidPriority},
		{name: "priority", event: 2422, changes: map[int]profilerCoreTestValue{3: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidPriority},
		{name: "target-cpu", event: 2420, changes: map[int]profilerCoreTestValue{5: profilerCoreVarint(uint64(maxTraceDBCPUIndex + 1))}, kind: profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU},
		{name: "target-cpu", event: 2421, changes: map[int]profilerCoreTestValue{5: profilerCoreVarint(uint64(maxTraceDBCPUIndex + 1))}, kind: profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU},
		{name: "target-cpu", event: 2422, changes: map[int]profilerCoreTestValue{5: profilerCoreVarint(uint64(maxTraceDBCPUIndex + 1))}, kind: profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU},
		{name: "iowait", event: 4002, changes: map[int]profilerCoreTestValue{3: profilerCoreVarint(2)}, kind: profilerFtraceEventIssueCoreMissingOrInvalidIOWait},
	}
	if len(semantic) != 32 {
		t.Fatalf("semantic fixture universe=%d want=32", len(semantic))
	}
	for _, test := range semantic {
		payload := profilerCoreTypedPayload(base[test.event], test.changes, test.omit...)
		add("semantic-"+test.name, test.event, payload,
			profilerCoreTypedFixedIssue(t, test.event, test.kind))
	}

	for _, event := range []int{2420, 2421, 2422} {
		display := []struct {
			name  string
			value *profilerCoreTestValue
			omit  bool
			kind  profilerFtraceEventIssueKind
		}{
			{name: "wrong", value: coreTypedValuePtr(profilerCoreVarint(1)), kind: profilerFtraceEventIssueCoreDisplayCommWrongWire},
			{name: "invalid", value: coreTypedValuePtr(profilerCoreBytes("bad\ncomm")), kind: profilerFtraceEventIssueCoreDisplayCommInvalid},
			{name: "unavailable", omit: true, kind: profilerFtraceEventIssueCoreDisplayCommUnavailable},
			{name: "out-of-profile", value: coreTypedValuePtr(profilerCoreBytes("1234567890123456")), kind: profilerFtraceEventIssueCoreDisplayCommOutOfProfile},
		}
		for _, test := range display {
			changes := map[int]profilerCoreTestValue{}
			if test.value != nil {
				changes[1] = *test.value
			}
			var omit []int
			if test.omit {
				omit = []int{1}
			}
			add("display-comm-"+test.name, event, profilerCoreTypedPayload(base[event], changes, omit...),
				profilerCoreTypedFixedIssue(t, event, test.kind))
		}
		duplicate := append(profilerCoreTypedPayload(base[event], nil), protoBytes(1, []byte("second"))...)
		add("display-comm-duplicate", event, duplicate,
			profilerCoreTypedFixedIssue(t, event, profilerFtraceEventIssueCoreDisplayCommDuplicate))
	}

	blockedDisplay := []struct {
		name  string
		value profilerCoreTestValue
		kind  profilerFtraceEventIssueKind
	}{
		{name: "wrong", value: profilerCoreVarint(1), kind: profilerFtraceEventIssueCoreDisplayCallerStrWrongWire},
		{name: "invalid", value: profilerCoreBytes("forged"), kind: profilerFtraceEventIssueCoreDisplayCallerStrInvalid},
	}
	for _, test := range blockedDisplay {
		add("display-caller-"+test.name, 4002,
			profilerCoreTypedPayload(base[4002], map[int]profilerCoreTestValue{4: test.value}),
			profilerCoreTypedFixedIssue(t, 4002, test.kind))
	}
	blockedDuplicate := append(profilerCoreTypedPayload(base[4002], nil), protoBytes(4, []byte("second"))...)
	add("display-caller-duplicate", 4002, blockedDuplicate,
		profilerCoreTypedFixedIssue(t, 4002, profilerFtraceEventIssueCoreDisplayCallerStrDuplicate))

	// Only these four unbounded, semantically admitted source strings can make
	// the final canonical line exceed its cap.
	for _, event := range []int{1400, 1401, 1402, 1500} {
		field := 1
		if event == 1402 || event == 1500 {
			field = 2
		}
		payload := profilerCoreTypedPayload(base[event], map[int]profilerCoreTestValue{
			field: profilerCoreBytes(strings.Repeat("x", maxTraceDBSystraceLineBytes)),
		})
		add("canonical-line", event, payload,
			profilerCoreTypedFixedIssue(t, event, profilerFtraceEventIssueCoreInvalidCanonicalLine))
	}

	if len(fixtures) != 163 {
		t.Fatalf("raw core typed fixture universe=%d want=163", len(fixtures))
	}
	return fixtures
}

func coreTypedValuePtr(value profilerCoreTestValue) *profilerCoreTestValue {
	return &value
}

func requireProfilerCoreTypedResult(t *testing.T, event profilerFtraceEventRecord, wantIssue *profilerFtraceEventIssue) {
	t.Helper()
	name, body, ok, issues, handled, err := renderProfilerFtraceCoreEventWithTypedAudit(event)
	if err != nil || !handled {
		t.Fatalf("core typed choke failed: field=%d handled=%t err=%v", event.Field, handled, err)
	}
	wantOK := wantIssue == nil || wantIssue.Severity == profilerFtraceEventIssueAdmittedDisplay
	var wantIssues []profilerFtraceEventIssue
	if wantIssue != nil {
		wantIssues = []profilerFtraceEventIssue{*wantIssue}
	}
	if ok != wantOK || !reflect.DeepEqual(issues, wantIssues) ||
		!profilerFtraceEventIssueVerdictValid(event.Field, ok, issues) {
		t.Fatalf("core typed verdict drifted: field=%d ok=%t want_ok=%t issues=%+v want=%+v name=%q body=%q",
			event.Field, ok, wantOK, issues, wantIssues, name, body)
	}
	if !ok && (name != "" || body != "") {
		t.Fatalf("rejected core row retained output: field=%d name=%q body=%q issues=%+v", event.Field, name, body, issues)
	}
	if ok && (name == "" || body == "") {
		t.Fatalf("admitted core row lost output: field=%d name=%q body=%q issues=%+v", event.Field, name, body, issues)
	}

	typedName, typedBody, typedOK, typedIssues, typedErr := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if typedErr != nil || typedName != name || typedBody != body || typedOK != ok || !reflect.DeepEqual(typedIssues, issues) {
		t.Fatalf("outer typed/core parity drifted: field=%d\ncore=(%q,%q,%t,%+v,%v)\nouter=(%q,%q,%t,%+v,%v)",
			event.Field, name, body, ok, issues, err, typedName, typedBody, typedOK, typedIssues, typedErr)
	}
	labels, labelsOK := profilerFtraceEventIssueLabels(event.Field, issues)
	if !labelsOK {
		t.Fatalf("typed core issue has no compatibility label: field=%d issues=%+v", event.Field, issues)
	}
	compatName, compatBody, compatOK, compatLabels := renderProfilerFtraceEventBodyWithAudit(event)
	if compatName != name || compatBody != body || compatOK != ok || !reflect.DeepEqual(compatLabels, labels) {
		t.Fatalf("direct label/core parity drifted: field=%d\ncore=(%q,%q,%t,%v)\ncompat=(%q,%q,%t,%v)",
			event.Field, name, body, ok, labels, compatName, compatBody, compatOK, compatLabels)
	}
}

func TestProfilerCoreTypedValidGoldenAndDirectParity(t *testing.T) {
	for _, test := range profilerCoreTestCases() {
		t.Run(test.name, func(t *testing.T) {
			event := profilerCoreTypedRecord(test.field, profilerCoreEncodeValues(test.values))
			name, body, ok, issues, handled, err := renderProfilerFtraceCoreEventWithTypedAudit(event)
			if err != nil || !handled || !ok || name != test.name || body != test.want || len(issues) != 0 {
				t.Fatalf("valid core golden drifted: handled=%t ok=%t name=%q body=%q issues=%+v err=%v",
					handled, ok, name, body, issues, err)
			}
			requireProfilerCoreTypedResult(t, event, nil)
		})
	}
	unsupported := profilerCoreTypedRecord(410, protoPayload(protoBytes(1, []byte("clk")), protoVarint(2, 2)))
	if _, _, _, issues, handled, err := renderProfilerFtraceCoreEventWithTypedAudit(unsupported); err != nil || handled || len(issues) != 0 {
		t.Fatalf("non-core event entered core choke: handled=%t issues=%+v err=%v", handled, issues, err)
	}
}

func TestProfilerCoreTypedRawProducerClosureIsExactly163(t *testing.T) {
	fixtures := profilerCoreTypedRawFixtures(t)
	produced := make(map[profilerCoreTypedTuple]profilerCoreTypedFixture, len(fixtures))
	seenKinds := [profilerFtraceEventIssueKindCount]bool{}
	for _, fixture := range fixtures {
		key := profilerCoreTypedTuple{event: fixture.event.Field, issue: fixture.issue}
		if previous, exists := produced[key]; exists {
			t.Fatalf("raw producer tuple duplicated: event=%d issue=%+v first=%q next=%q",
				key.event, key.issue, previous.name, fixture.name)
		}
		if !fixture.issue.validFor(fixture.event.Field) ||
			fixture.issue.Kind < profilerFtraceEventIssueCorePayloadMalformedWire ||
			fixture.issue.Kind > profilerFtraceEventIssueCoreInvalidCanonicalLine {
			t.Fatalf("raw fixture carries illegal core issue: event=%d issue=%+v", fixture.event.Field, fixture.issue)
		}
		produced[key] = fixture
		seenKinds[fixture.issue.Kind] = true
	}

	legal := make(map[profilerCoreTypedTuple]bool, 163)
	maxPerEvent := 0
	for _, test := range profilerCoreTestCases() {
		perEvent := 0
		for kind := profilerFtraceEventIssueCorePayloadMalformedWire; kind <= profilerFtraceEventIssueCoreInvalidCanonicalLine; kind++ {
			for payloadField := 0; payloadField <= 255; payloadField++ {
				for severity := profilerFtraceEventIssueSeverity(0); severity < profilerFtraceEventIssueSeverityCount; severity++ {
					issue := profilerFtraceEventIssue{Kind: kind, PayloadField: uint8(payloadField), Severity: severity}
					if !issue.validFor(test.field) {
						continue
					}
					key := profilerCoreTypedTuple{event: test.field, issue: issue}
					legal[key] = true
					perEvent++
				}
			}
		}
		if perEvent > maxPerEvent {
			maxPerEvent = perEvent
		}
	}
	if len(legal) != 163 || maxPerEvent != 25 {
		t.Fatalf("legal core issue universe drifted: total=%d want=163 max_per_event=%d want=25", len(legal), maxPerEvent)
	}
	for key := range legal {
		if _, ok := produced[key]; !ok {
			t.Fatalf("legal core issue has no raw producer witness: event=%d issue=%+v", key.event, key.issue)
		}
	}
	for key := range produced {
		if !legal[key] {
			t.Fatalf("raw producer minted tuple outside legal universe: event=%d issue=%+v", key.event, key.issue)
		}
	}
	for kind := profilerFtraceEventIssueCorePayloadMalformedWire; kind <= profilerFtraceEventIssueCoreInvalidCanonicalLine; kind++ {
		if !seenKinds[kind] {
			t.Fatalf("core issue kind has no raw witness: kind=%d", kind)
		}
	}

	for _, fixture := range fixtures {
		fixture := fixture
		label, _ := fixture.issue.label(fixture.event.Field)
		t.Run(fmt.Sprintf("field%d/%s/%s", fixture.event.Field, fixture.name, label), func(t *testing.T) {
			requireProfilerCoreTypedResult(t, fixture.event, &fixture.issue)
		})
	}
}

func TestProfilerCoreTypedIssueSetCapacityOneAndCorruptionFailClosed(t *testing.T) {
	if profilerFtraceCoreIssuesPerEvent != 1 {
		t.Fatalf("core issue capacity=%d want=1", profilerFtraceCoreIssuesPerEvent)
	}
	hard := profilerCoreTypedHardIssue(t, 2420, profilerFtraceEventIssueCoreFieldWrongWire, 2)
	semantic := profilerCoreTypedFixedIssue(t, 2420, profilerFtraceEventIssueCoreMissingOrInvalidPID)
	display := profilerCoreTypedFixedIssue(t, 2420, profilerFtraceEventIssueCoreDisplayCommInvalid)
	whole := profilerCoreTypedFixedIssue(t, 1400, profilerFtraceEventIssueCorePayloadMalformedWire)
	canonical := profilerCoreTypedFixedIssue(t, 1400, profilerFtraceEventIssueCoreInvalidCanonicalLine)

	valid := []struct {
		event int
		issue profilerFtraceEventIssue
	}{
		{event: 2420, issue: hard},
		{event: 2420, issue: semantic},
		{event: 2420, issue: display},
		{event: 1400, issue: whole},
		{event: 1400, issue: canonical},
	}
	for _, test := range valid {
		var set profilerFtraceCoreIssueSet
		if err := set.add(test.event, test.issue); err != nil {
			t.Fatalf("valid issue rejected: event=%d issue=%+v err=%v", test.event, test.issue, err)
		}
		got, err := set.checked(test.event)
		if err != nil || !reflect.DeepEqual(got, []profilerFtraceEventIssue{test.issue}) {
			t.Fatalf("valid issue set drifted: event=%d issue=%+v got=%+v err=%v", test.event, test.issue, got, err)
		}
		before := set
		if err := set.add(test.event, test.issue); err == nil || !reflect.DeepEqual(set, before) {
			t.Fatalf("duplicate/full add mutated set: event=%d err=%v before=%+v after=%+v",
				test.event, err, before, set)
		}
	}

	// Capacity one is the existing public first-reason contract. Any attempt to
	// cross diagnostic arms must fail without replacing the already selected
	// dominant issue, in either insertion direction.
	conflicts := []struct {
		event int
		left  profilerFtraceEventIssue
		right profilerFtraceEventIssue
	}{
		{event: 2420, left: hard, right: semantic},
		{event: 2420, left: hard, right: display},
		{event: 2420, left: semantic, right: display},
		{event: 1400, left: whole, right: canonical},
		{event: 1400, left: profilerCoreTypedHardIssue(t, 1400, profilerFtraceEventIssueCoreFieldWrongWire, 1), right: canonical},
		{event: 1400, left: profilerCoreTypedFixedIssue(t, 1400, profilerFtraceEventIssueCoreMissingOrInvalidReason), right: canonical},
	}
	for index, conflict := range conflicts {
		for order, pair := range [][2]profilerFtraceEventIssue{{conflict.left, conflict.right}, {conflict.right, conflict.left}} {
			var set profilerFtraceCoreIssueSet
			if err := set.add(conflict.event, pair[0]); err != nil {
				t.Fatalf("conflict fixture %d/%d first rejected: %v", index, order, err)
			}
			before := set
			if err := set.add(conflict.event, pair[1]); err == nil || !reflect.DeepEqual(set, before) {
				t.Fatalf("cross-arm add %d/%d mutated selected issue: err=%v before=%+v after=%+v",
					index, order, err, before, set)
			}
		}
	}

	foreign, ok := profilerFtraceEventFixedIssue(2420, profilerFtraceEventIssueEnvelopeCommInvalid)
	if !ok {
		t.Fatal("fixture envelope issue rejected")
	}
	wrongEvent := profilerCoreTypedFixedIssue(t, 2003, profilerFtraceEventIssueCoreMissingOrInvalidState)
	wrongPayload := hard
	wrongPayload.PayloadField = 8
	wrongSeverity := hard
	wrongSeverity.Severity = profilerFtraceEventIssueAdmittedDisplay
	corruptions := []profilerFtraceCoreIssueSet{
		{Count: profilerFtraceCoreIssuesPerEvent + 1},
		{Count: 0, Issues: [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue{hard}},
		{Count: 1, Issues: [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue{foreign}},
		{Count: 1, Issues: [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue{wrongEvent}},
		{Count: 1, Issues: [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue{wrongPayload}},
		{Count: 1, Issues: [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue{wrongSeverity}},
		{Count: 1, Issues: [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue{{Kind: profilerFtraceEventIssueKindCount, Severity: profilerFtraceEventIssueHardReject}}},
		{Count: 1},
	}
	for index := range corruptions {
		if _, err := corruptions[index].checked(2420); err == nil {
			t.Fatalf("corrupt core issue set %d admitted: %+v", index, corruptions[index])
		}
	}

	invalidAdds := []profilerFtraceEventIssue{foreign, wrongEvent, wrongPayload, wrongSeverity}
	for index, issue := range invalidAdds {
		var set profilerFtraceCoreIssueSet
		before := set
		if err := set.add(2420, issue); err == nil || !reflect.DeepEqual(set, before) {
			t.Fatalf("invalid prospective add %d mutated set: issue=%+v err=%v before=%+v after=%+v",
				index, issue, err, before, set)
		}
	}
}

func requireProfilerCoreInvariant(t *testing.T, err error, want string) {
	t.Helper()
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != want {
		t.Fatalf("core invariant error=%T %v reason=%q want=%q", err, err, reason, want)
	}
}

func TestProfilerCoreTypedInternalInvariantsDominateSourceIssues(t *testing.T) {
	base := profilerCoreTypedBaseCases()
	event := profilerCoreTypedRecord(1400, profilerCoreTypedPayload(base[1400], nil))
	payload := coreRenderPayload{
		Kind: coreRenderInterrupt, Name: "ipi_entry",
		Interrupt: &coreInterruptPayload{Reason: strings.Repeat("r", maxTraceDBSystraceLineBytes)},
	}
	hard := profilerCoreTypedFixedIssue(t, 1400, profilerFtraceEventIssueCoreMissingOrInvalidReason)
	display := profilerCoreTypedFixedIssue(t, 2420, profilerFtraceEventIssueCoreDisplayCommInvalid)

	// A corrupt inactive tail must fail before the oversized source string can
	// be converted into a canonical-line issue.
	corrupt := profilerFtraceCoreIssueSet{Issues: [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue{hard}}
	_, _, _, issues, err := finalizeProfilerFtraceCoreEventWithTypedAudit(event, payload, bodyAdmitted, corrupt)
	if len(issues) != 0 {
		t.Fatalf("corrupt typed set leaked source issues: %+v", issues)
	}
	requireProfilerCoreInvariant(t, err, "profiler_core_issue_count_invalid")

	var hardSet profilerFtraceCoreIssueSet
	if err := hardSet.add(1400, hard); err != nil {
		t.Fatal(err)
	}
	_, _, _, issues, err = finalizeProfilerFtraceCoreEventWithTypedAudit(event, payload, bodyAdmitted, hardSet)
	if len(issues) != 0 {
		t.Fatalf("admitted hard verdict leaked issues: %+v", issues)
	}
	requireProfilerCoreInvariant(t, err, "profiler_core_admitted_verdict_invalid")

	var displaySet profilerFtraceCoreIssueSet
	if err := displaySet.add(2420, display); err != nil {
		t.Fatal(err)
	}
	wakeEvent := profilerCoreTypedRecord(2420, profilerCoreTypedPayload(base[2420], nil))
	_, _, _, issues, err = finalizeProfilerFtraceCoreEventWithTypedAudit(
		wakeEvent, coreRenderPayload{}, bodyRejected, displaySet,
	)
	if len(issues) != 0 {
		t.Fatalf("rejected display verdict leaked issues: %+v", issues)
	}
	requireProfilerCoreInvariant(t, err, "profiler_core_rejected_verdict_invalid")

	_, _, _, issues, err = finalizeProfilerFtraceCoreEventWithTypedAudit(
		event, coreRenderPayload{Kind: coreRenderInterrupt, Name: "ipi_entry"}, bodyAdmitted, profilerFtraceCoreIssueSet{},
	)
	if len(issues) != 0 {
		t.Fatalf("invalid canonical payload leaked issues: %+v", issues)
	}
	requireProfilerCoreInvariant(t, err, "invalid_canonical_core_payload")

	_, _, _, issues, err = finalizeProfilerFtraceCoreEventWithTypedAudit(
		event, coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{},
	)
	if len(issues) != 0 {
		t.Fatalf("invalid admission leaked issues: %+v", issues)
	}
	requireProfilerCoreInvariant(t, err, "profiler_core_admission_invalid")
}

func TestProfilerCoreTypedDescriptorAndDispatchFailuresAreInternal(t *testing.T) {
	base := profilerCoreTypedBaseCases()[113]
	event := profilerCoreTypedRecord(113, profilerCoreTypedPayload(base, nil))
	original := profilerFtraceEventDescriptors[113]
	defer func() { profilerFtraceEventDescriptors[113] = original }()

	delete(profilerFtraceEventDescriptors, 113)
	_, _, ok, issues, handled, err := renderProfilerFtraceCoreEventWithTypedAudit(event)
	if !handled || ok || len(issues) != 0 {
		t.Fatalf("missing descriptor escaped as source verdict: handled=%t ok=%t issues=%+v", handled, ok, issues)
	}
	requireProfilerCoreInvariant(t, err, "missing_core_descriptor")

	mismatch := original
	mismatch.Field = 119
	profilerFtraceEventDescriptors[113] = mismatch
	_, _, ok, issues, handled, err = renderProfilerFtraceCoreEventWithTypedAudit(event)
	if !handled || ok || len(issues) != 0 {
		t.Fatalf("mismatched descriptor escaped as source verdict: handled=%t ok=%t issues=%+v", handled, ok, issues)
	}
	requireProfilerCoreInvariant(t, err, "mismatched_core_descriptor_field")

	badName := original
	badName.Name = "future_core_name"
	profilerFtraceEventDescriptors[113] = badName
	_, _, ok, issues, handled, err = renderProfilerFtraceCoreEventWithTypedAudit(event)
	if !handled || ok || len(issues) != 0 {
		t.Fatalf("invalid descriptor name escaped as source verdict: handled=%t ok=%t issues=%+v", handled, ok, issues)
	}
	requireProfilerCoreInvariant(t, err, "invalid_core_descriptor_name")
	profilerFtraceEventDescriptors[113] = original

	const unhandledField = 9999
	profilerStructuredCoreSchemas[unhandledField] = map[int]int{1: 0}
	profilerFtraceEventDescriptors[unhandledField] = profilerFtraceEventDescriptor{
		Field: unhandledField, Family: "test", Name: "cpu_idle",
	}
	defer func() {
		delete(profilerStructuredCoreSchemas, unhandledField)
		delete(profilerFtraceEventDescriptors, unhandledField)
	}()
	unhandled := profilerCoreTypedRecord(unhandledField, nil)
	_, _, ok, issues, handled, err = renderProfilerFtraceCoreEventWithTypedAudit(unhandled)
	if !handled || ok || len(issues) != 0 {
		t.Fatalf("unhandled dispatch escaped as source verdict: handled=%t ok=%t issues=%+v", handled, ok, issues)
	}
	requireProfilerCoreInvariant(t, err, "unhandled_core_descriptor")
}

func TestProfilerCoreTypedDominanceAndSchemaFirstOrderAreWireOrderStable(t *testing.T) {
	base := profilerCoreTypedBaseCases()
	wakeHard := [][]byte{
		protoBytes(1, []byte("bad\ncomm")), protoBytes(2, []byte{20}), protoVarint(3, 159),
		protoVarint(4, 0), protoVarint(5, 2),
	}
	wakeSemantic := [][]byte{
		protoBytes(1, []byte("bad\ncomm")), protoVarint(2, math.MaxUint64), protoVarint(3, 159),
		protoVarint(4, 0), protoVarint(5, 2),
	}
	binderFirst := [][]byte{
		protoVarint(1, 12), protoVarint(1, 13), protoBytes(2, []byte{1}), protoVarint(3, 1),
		protoVarint(4, 0), protoVarint(5, 0), protoVarint(6, 0), protoVarint(7, 0),
	}
	binderSemantic := [][]byte{
		protoVarint(1, 0), protoVarint(2, math.MaxUint64), protoVarint(3, 1), protoVarint(4, 0),
		protoVarint(5, 2), protoVarint(6, 0), protoVarint(7, 0),
	}
	displayWrongDominatesDuplicate := [][]byte{
		protoBytes(1, []byte("worker")), protoBytes(1, []byte("worker2")), protoVarint(1, 1),
		protoVarint(2, 20), protoVarint(3, 159), protoVarint(4, 0), protoVarint(5, 2),
	}

	tests := []struct {
		name  string
		event int
		parts [][]byte
		issue profilerFtraceEventIssue
	}{
		{name: "hard-over-display", event: 2420, parts: wakeHard,
			issue: profilerCoreTypedHardIssue(t, 2420, profilerFtraceEventIssueCoreFieldWrongWire, 2)},
		{name: "semantic-over-display", event: 2420, parts: wakeSemantic,
			issue: profilerCoreTypedFixedIssue(t, 2420, profilerFtraceEventIssueCoreMissingOrInvalidPID)},
		{name: "schema-first-wire-hard", event: 113, parts: binderFirst,
			issue: profilerCoreTypedHardIssue(t, 113, profilerFtraceEventIssueCoreFieldDuplicate, 1)},
		{name: "semantic-first", event: 113, parts: binderSemantic,
			issue: profilerCoreTypedFixedIssue(t, 113, profilerFtraceEventIssueCoreInvalidTransactionID)},
		{name: "display-wrong-over-duplicate", event: 2420, parts: displayWrongDominatesDuplicate,
			issue: profilerCoreTypedFixedIssue(t, 2420, profilerFtraceEventIssueCoreDisplayCommWrongWire)},
	}
	for _, test := range tests {
		reverse := append([][]byte(nil), test.parts...)
		for left, right := 0, len(reverse)-1; left < right; left, right = left+1, right-1 {
			reverse[left], reverse[right] = reverse[right], reverse[left]
		}
		for order, parts := range [][][]byte{test.parts, reverse} {
			t.Run(fmt.Sprintf("%s/order%d", test.name, order), func(t *testing.T) {
				event := profilerCoreTypedRecord(test.event, protoPayload(parts...))
				requireProfilerCoreTypedResult(t, event, &test.issue)
			})
		}
	}

	// Once framing loses endpoint identity, no prior field/display observation
	// may survive as a more precise issue.
	wholePayload := append(profilerCoreTypedPayload(base[2420], map[int]profilerCoreTestValue{
		1: profilerCoreVarint(1), 2: {wire: 2, bytes: []byte{20}},
	}), 0x80)
	whole := profilerCoreTypedFixedIssue(t, 2420, profilerFtraceEventIssueCorePayloadMalformedWire)
	requireProfilerCoreTypedResult(t, profilerCoreTypedRecord(2420, wholePayload), &whole)
}

func TestProfilerCoreTypedCanonicalBoundaryAndContainerLocality(t *testing.T) {
	base := profilerCoreTypedBaseCases()[1400]
	record := profilerCoreTypedRecord(1400, nil)
	emptyLine := traceDBFormatLine(record.Comm, record.PID, record.TGID, record.CPU, int64(record.TSNS),
		record.CommonFlags, record.CommonPreemptCount, "ipi_entry: ()")
	reasonBytes := maxTraceDBSystraceLineBytes - len(emptyLine)
	if reasonBytes <= 0 {
		t.Fatalf("canonical fixture overhead=%d exceeds cap=%d", len(emptyLine), maxTraceDBSystraceLineBytes)
	}
	exactReason := strings.Repeat("r", reasonBytes)
	exact := profilerCoreTypedRecord(1400, profilerCoreTypedPayload(base, map[int]profilerCoreTestValue{
		1: profilerCoreBytes(exactReason),
	}))
	name, body, ok, issues, handled, err := renderProfilerFtraceCoreEventWithTypedAudit(exact)
	if err != nil || !handled || !ok || len(issues) != 0 {
		t.Fatalf("exact-cap canonical row rejected: handled=%t ok=%t issues=%+v err=%v", handled, ok, issues, err)
	}
	exactLine := traceDBFormatLine(exact.Comm, exact.PID, exact.TGID, exact.CPU, int64(exact.TSNS),
		exact.CommonFlags, exact.CommonPreemptCount, name+": "+body)
	if len(exactLine) != maxTraceDBSystraceLineBytes {
		t.Fatalf("canonical exact-cap calibration drifted: len=%d want=%d", len(exactLine), maxTraceDBSystraceLineBytes)
	}
	requireProfilerCoreTypedResult(t, exact, nil)

	tooLong := profilerCoreTypedRecord(1400, profilerCoreTypedPayload(base, map[int]profilerCoreTestValue{
		1: profilerCoreBytes(exactReason + "r"),
	}))
	canonical := profilerCoreTypedFixedIssue(t, 1400, profilerFtraceEventIssueCoreInvalidCanonicalLine)
	requireProfilerCoreTypedResult(t, tooLong, &canonical)

	healthyPayload := profilerCoreTypedPayload(base, nil)
	structured := protoMessage(2,
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 100, 100, "bad", 1400, tooLong.Payload),
		syntheticTracePluginFtraceEvent(2_000, 101, 101, "healthy", 1400, healthyPayload),
	)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", structured),
	)
	defer sink.cleanup()
	coverage, entries := profilerEventCoverageByField(extracted, 1400)
	if entries != 1 || coverage.RowsRead != 2 || coverage.RowsEmitted != 1 ||
		coverage.FieldSources["degraded_invalid_canonical_core_line_occurrences"] != "1" ||
		coverage.FieldSources["degraded_invalid_canonical_core_line_affected_frames"] != "1" {
		t.Fatalf("core canonical local-reject census drifted: entries=%d coverage=%+v", entries, coverage)
	}
	if extracted.SourceFailClosed || extracted.StructuredRows != 1 || sink.publishableRows() != 1 ||
		len(sink.rows) != 1 || !strings.Contains(sink.rows[0].line, "healthy") ||
		!strings.Contains(sink.rows[0].line, "ipi_entry: (Rescheduling interrupts)") {
		t.Fatalf("bad core canonical row contaminated sibling: extracted=%+v sink=%+v rows=%+v",
			extracted, sink.stats, sink.rows)
	}
}

func TestProfilerCoreTypedContainerParityAcrossDiagnosticArms(t *testing.T) {
	base := profilerCoreTypedBaseCases()
	binderWrong := append(profilerCoreTypedPayload(base[113], nil, 1), protoBytes(1, []byte{1})...)
	limitsOrder := profilerCoreTypedPayload(base[2004], map[int]profilerCoreTestValue{
		1: profilerCoreVarint(2_000_000), 2: profilerCoreVarint(1_000_000),
	})
	wakeDisplay := profilerCoreTypedPayload(base[2420], map[int]profilerCoreTestValue{
		1: profilerCoreBytes("bad\ncomm"),
	})
	softirqWhole := append(profilerCoreTypedPayload(base[1502], nil), 0x80)
	structured := protoMessage(2,
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 100, 100, "binder-bad", 113, binderWrong),
		syntheticTracePluginFtraceEvent(2_000, 100, 100, "binder-ok", 113, profilerCoreTypedPayload(base[113], nil)),
		syntheticTracePluginFtraceEvent(3_000, 100, 100, "limits-bad", 2004, limitsOrder),
		syntheticTracePluginFtraceEvent(4_000, 100, 100, "limits-ok", 2004, profilerCoreTypedPayload(base[2004], nil)),
		syntheticTracePluginFtraceEvent(5_000, 100, 100, "wake-bad", 2420, wakeDisplay),
		syntheticTracePluginFtraceEvent(6_000, 100, 100, "wake-ok", 2420, profilerCoreTypedPayload(base[2420], nil)),
		syntheticTracePluginFtraceEvent(7_000, 100, 100, "softirq-bad", 1502, softirqWhole),
		syntheticTracePluginFtraceEvent(8_000, 100, 100, "softirq-ok", 1502, profilerCoreTypedPayload(base[1502], nil)),
	)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", structured),
	)
	defer sink.cleanup()
	checks := []struct {
		field       int
		label       string
		rowsEmitted int
	}{
		{field: 113, label: "core_field1_wrong_wire", rowsEmitted: 1},
		{field: 2004, label: "invalid_limits_order", rowsEmitted: 1},
		{field: 2420, label: "display_comm_invalid", rowsEmitted: 2},
		{field: 1502, label: "core_payload_malformed_wire", rowsEmitted: 1},
	}
	for _, check := range checks {
		coverage, entries := profilerEventCoverageByField(extracted, check.field)
		if entries != 1 || coverage.RowsRead != 2 || coverage.RowsEmitted != check.rowsEmitted ||
			coverage.FieldSources["degraded_"+check.label+"_occurrences"] != "1" ||
			coverage.FieldSources["degraded_"+check.label+"_affected_frames"] != "1" {
			t.Fatalf("container/direct issue parity drifted: field=%d label=%q entries=%d coverage=%+v",
				check.field, check.label, entries, coverage)
		}
	}
	if extracted.SourceFailClosed || extracted.StructuredRows != 5 || sink.publishableRows() != 5 || len(sink.rows) != 5 {
		t.Fatalf("row-local core issues contaminated container: extracted=%+v sink=%+v rows=%+v",
			extracted, sink.stats, sink.rows)
	}
}

func TestProfilerCoreTypedCanonicalBoundaryAllReachableEvents(t *testing.T) {
	base := profilerCoreTypedBaseCases()
	for _, eventField := range []int{1400, 1401, 1402, 1500} {
		t.Run(fmt.Sprintf("field%d", eventField), func(t *testing.T) {
			stringField := 1
			if eventField == 1402 || eventField == 1500 {
				stringField = 2
			}
			probe := profilerCoreTypedRecord(eventField, profilerCoreTypedPayload(base[eventField], map[int]profilerCoreTestValue{
				stringField: profilerCoreBytes("x"),
			}))
			name, body, ok, issues, handled, err := renderProfilerFtraceCoreEventWithTypedAudit(probe)
			if err != nil || !handled || !ok || len(issues) != 0 {
				t.Fatalf("canonical probe rejected: handled=%t ok=%t issues=%+v err=%v", handled, ok, issues, err)
			}
			probeLine := traceDBFormatLine(probe.Comm, probe.PID, probe.TGID, probe.CPU, int64(probe.TSNS),
				probe.CommonFlags, probe.CommonPreemptCount, name+": "+body)
			valueBytes := maxTraceDBSystraceLineBytes - (len(probeLine) - 1)
			if valueBytes <= 0 {
				t.Fatalf("canonical overhead=%d exceeds cap=%d", len(probeLine)-1, maxTraceDBSystraceLineBytes)
			}

			exact := profilerCoreTypedRecord(eventField, profilerCoreTypedPayload(base[eventField], map[int]profilerCoreTestValue{
				stringField: profilerCoreBytes(strings.Repeat("x", valueBytes)),
			}))
			name, body, ok, issues, handled, err = renderProfilerFtraceCoreEventWithTypedAudit(exact)
			if err != nil || !handled || !ok || len(issues) != 0 {
				t.Fatalf("exact-cap canonical row rejected: handled=%t ok=%t issues=%+v err=%v", handled, ok, issues, err)
			}
			exactLine := traceDBFormatLine(exact.Comm, exact.PID, exact.TGID, exact.CPU, int64(exact.TSNS),
				exact.CommonFlags, exact.CommonPreemptCount, name+": "+body)
			if len(exactLine) != maxTraceDBSystraceLineBytes {
				t.Fatalf("canonical exact-cap drifted: len=%d want=%d", len(exactLine), maxTraceDBSystraceLineBytes)
			}

			tooLong := profilerCoreTypedRecord(eventField, profilerCoreTypedPayload(base[eventField], map[int]profilerCoreTestValue{
				stringField: profilerCoreBytes(strings.Repeat("x", valueBytes+1)),
			}))
			canonical := profilerCoreTypedFixedIssue(t, eventField, profilerFtraceEventIssueCoreInvalidCanonicalLine)
			requireProfilerCoreTypedResult(t, tooLong, &canonical)
		})
	}
}

func TestProfilerCoreCanonicalCannotBeMintedFromBoundedHeaderOrEnvelopeFailure(t *testing.T) {
	base := profilerCoreTypedBaseCases()[113]
	payload := profilerCoreTypedPayload(base, nil)
	overlongComm := strings.Repeat("c", maxTraceDBSystraceLineBytes)
	direct := profilerCoreTypedRecord(113, payload)
	direct.Comm = overlongComm
	requireProfilerCoreTypedResult(t, direct, nil)

	structured := protoMessage(2,
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 100, 100, overlongComm, 113, payload),
	)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", structured),
	)
	coverage, entries := profilerEventCoverageByField(extracted, 113)
	if entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != 1 ||
		coverage.FieldSources["degraded_invalid_canonical_core_line_occurrences"] != "" ||
		extracted.SourceFailClosed || extracted.StructuredRows != 1 || sink.publishableRows() != 1 {
		sink.cleanup()
		t.Fatalf("bounded envelope comm minted core canonical: entries=%d coverage=%+v extracted=%+v sink=%+v",
			entries, coverage, extracted, sink.stats)
	}
	sink.cleanup()

	overflowTimestamp := uint64(math.MaxInt64) + 1
	structured = protoMessage(2,
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(overflowTimestamp, 100, 100, "bad-ts", 113, payload),
	)
	extracted, sink = extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", structured),
	)
	defer sink.cleanup()
	coverage, entries = profilerEventCoverageByField(extracted, 113)
	if entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["degraded_envelope_timestamp_out_of_range_occurrences"] != "1" ||
		coverage.FieldSources["degraded_invalid_canonical_core_line_occurrences"] != "" ||
		extracted.SourceFailClosed || extracted.StructuredRows != 0 || sink.publishableRows() != 0 {
		t.Fatalf("envelope timestamp failure was misclassified as core canonical: entries=%d coverage=%+v extracted=%+v sink=%+v",
			entries, coverage, extracted, sink.stats)
	}
}

func TestProfilerCoreTypedProductionBypassesLegacyReasonBridge(t *testing.T) {
	adapter := mustReadRendererSource(t, "profiler_core_payload.go")
	renderer := mustReadRendererSource(t, "profiler_ftrace_render.go")
	typedDecode := sourceBetween(t, adapter,
		"func decodeProfilerCorePayloadWithTypedAuditContext(", "func decodeProfilerCorePayload(")
	if strings.Count(typedDecode, "walkProfilerProtoFieldsContext(ctx, event.Payload") != 1 ||
		strings.Contains(typedDecode, "walkProtoFields(event.Payload") ||
		!strings.Contains(typedDecode, "var fields [8]profilerCoreProtoField") ||
		!strings.Contains(typedDecode, "set.addFixed(event.Field") ||
		!strings.Contains(typedDecode, "set.addPayload(event.Field") {
		t.Fatalf("typed core decoder lost its single fixed-state issue authority:\n%s", typedDecode)
	}
	for _, forbidden := range []string{
		`fmt.Sprintf("core_field`, `"display_" +`, "[]string", ".Error()",
		"profilerFtraceEventIssueFromLegacy(", "decodeProfilerCorePayload(event)",
		"decodeProfilerCorePayloadWithTypedAudit(event)",
	} {
		if strings.Contains(typedDecode, forbidden) {
			t.Fatalf("typed core producer restored dynamic/legacy authority %q:\n%s", forbidden, typedDecode)
		}
	}
	if !strings.Contains(adapter, "const profilerFtraceCoreIssuesPerEvent = 1") ||
		!strings.Contains(adapter, "Issues [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue") {
		t.Fatal("typed core issue set is not fixed capacity one")
	}
	decodeCompat := sourceBetween(t, adapter,
		"func decodeProfilerCorePayloadWithTypedAudit(", "func decodeProfilerCorePayloadWithTypedAuditContext(")
	if strings.Count(decodeCompat, "decodeProfilerCorePayloadWithTypedAuditContext(context.Background(), event)") != 1 {
		t.Fatal("core decode compatibility entry is not a Background-only adapter over the Context authority")
	}

	coreRender := sourceBetween(t, adapter,
		"func renderProfilerFtraceCoreEventWithTypedAuditContext(", "func finalizeProfilerFtraceCoreEventWithTypedAudit(")
	if strings.Count(coreRender, "decodeProfilerCorePayloadWithTypedAuditContext(ctx, event)") != 1 ||
		strings.Contains(coreRender, "decodeProfilerCorePayload(event)") ||
		strings.Contains(coreRender, "decodeProfilerCorePayloadWithTypedAudit(event)") ||
		strings.Contains(coreRender, "profilerFtraceEventIssueFromLegacy(") {
		t.Fatalf("typed core render choke restored legacy decode/bridge:\n%s", coreRender)
	}

	compat := sourceBetween(t, renderer,
		"func renderProfilerFtraceEventBodyWithAudit(", "func renderProfilerFtraceEventBodyWithTypedAudit(")
	if strings.Count(compat, "renderProfilerFtraceEventBodyWithTypedAudit(event)") != 1 ||
		!strings.Contains(compat, "profilerFtraceEventIssueLabels(event.Field, issues)") ||
		strings.Contains(compat, "renderProfilerFtraceCoreEventWithTypedAudit(event)") ||
		strings.Contains(compat, "decodeProfilerCorePayload(event)") ||
		strings.Contains(compat, "profilerFtraceEventIssueFromLegacy(") {
		t.Fatalf("direct compatibility path is not a typed-issue-to-label adapter:\n%s", compat)
	}

	typedEntry := sourceBetween(t, renderer,
		"func renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(", "const profilerFtraceGenericIssuesPerEvent")
	coreAt := strings.Index(typedEntry, "renderProfilerFtraceCoreEventWithTypedAuditContext(ctx, event)")
	genericAt := strings.Index(typedEntry, "renderProfilerFtraceGenericEventWithTypedAuditContext(ctx, event)")
	legacyAt := strings.Index(typedEntry, "renderProfilerFtraceEventBodyWithAudit(event)")
	if coreAt < 0 || genericAt < 0 || legacyAt >= 0 || coreAt >= genericAt {
		t.Fatalf("typed entry order drifted: core=%d generic=%d legacy=%d", coreAt, genericAt, legacyAt)
	}
	coreArm := sourceBetween(t, typedEntry,
		"renderProfilerFtraceCoreEventWithTypedAuditContext(ctx, event)",
		"renderProfilerFtraceGenericEventWithTypedAuditContext(ctx, event)")
	if strings.Contains(coreArm, "profilerFtraceEventIssueFromLegacy(") ||
		strings.Contains(coreArm, "renderProfilerFtraceEventBodyWithAudit(event)") ||
		strings.Contains(coreArm, "renderProfilerFtraceCoreEventWithTypedAudit(event)") {
		t.Fatalf("production core arm still enters the reverse bridge:\n%s", coreArm)
	}
}
