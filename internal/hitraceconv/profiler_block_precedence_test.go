package hitraceconv

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

// requireProfilerBlockPrecedence pins the complete source verdict. A
// precedence test is useful only when the losing diagnostics cannot leak into
// the fixed-capacity set or alter admission/output.
func requireProfilerBlockPrecedence(
	t *testing.T,
	event profilerFtraceEventRecord,
	wantOK bool,
	wantIssues []profilerFtraceEventIssue,
) (string, string) {
	t.Helper()
	name, body, ok, issues, handled, err := renderProfilerFtraceBlockEventWithTypedAudit(event)
	if err != nil || !handled || ok != wantOK || !reflect.DeepEqual(issues, wantIssues) {
		t.Fatalf("block precedence verdict drifted: name=%q body=%q ok=%t want_ok=%t handled=%t issues=%+v want=%+v err=%v",
			name, body, ok, wantOK, handled, issues, wantIssues, err)
	}
	if !profilerFtraceEventIssueVerdictValid(event.Field, ok, issues) {
		t.Fatalf("block precedence emitted invalid verdict: field=%d ok=%t issues=%+v", event.Field, ok, issues)
	}
	if wantOK {
		if name == "" || body == "" {
			t.Fatalf("admitted block precedence row lost output: name=%q body=%q", name, body)
		}
	} else if name != "" || body != "" {
		t.Fatalf("rejected block precedence row retained output: name=%q body=%q", name, body)
	}
	return name, body
}

func profilerBlockPrecedenceAppend(payload []byte, fields ...[]byte) []byte {
	for _, field := range fields {
		payload = append(payload, field...)
	}
	return payload
}

func TestProfilerBlockPrecedenceWholeAndLocalizedFraming(t *testing.T) {
	whole := []profilerFtraceEventIssue{
		profilerBlockTypedFixedIssue(t, 210, profilerFtraceEventIssueBlockPayloadMalformedWire),
	}
	// Put completed hard/display failures before each framing failure. Whole
	// payload provenance must dominate every endpoint diagnosis already seen.
	base := profilerBlockTypedPayload(210, nil, 1, 6, 7)
	preexisting := func() []byte {
		return profilerBlockPrecedenceAppend(append([]byte(nil), base...),
			profilerBlockTypedWrongWire(1, 0),
			profilerBlockTypedWrongWire(7, 2),
			profilerBlockTypedWrongWire(6, 2),
		)
	}
	tests := []struct {
		name    string
		framing []byte
	}{
		{
			name: "unknown endpoint malformed tail",
			framing: profilerBlockPrecedenceAppend(
				profilerBlockTypedRawKey(99, 0), []byte{0x80},
			),
		},
		{name: "field zero framing", framing: []byte{0x00}},
		{name: "illegal field framing", framing: profilerBlockTypedRawKey(1<<29, 0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := append(preexisting(), test.framing...)
			requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, payload), false, whole)
		})
	}

	// A known non-display endpoint remains exactly local even when its malformed
	// value is nonterminal. Unlike an optional display, trailing bytes cannot
	// hide a harder endpoint than the endpoint already being rejected.
	localizedPayload := profilerBlockTypedPayload(210, nil, 1)
	localizedPayload = profilerBlockPrecedenceAppend(localizedPayload,
		profilerBlockTypedWrongWire(1, 0),
		profilerBlockTypedRawKey(1, 3),
		protoVarint(99, 1),
	)
	requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, localizedPayload), false,
		[]profilerFtraceEventIssue{
			profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldMalformedWire, 1),
		})
}

func TestProfilerBlockPrecedenceMalformedWrongDuplicateAndSchemaOrder(t *testing.T) {
	malformed := profilerBlockTypedPayload(210, nil, 1)
	malformed = profilerBlockPrecedenceAppend(malformed,
		profilerBlockTypedEncodeField(1, profilerBlockTypedBaseValues(210)[1]),
		profilerBlockTypedWrongWire(1, 0),
		profilerBlockTypedMalformedField(1, 0),
	)
	requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, malformed), false,
		[]profilerFtraceEventIssue{
			profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldMalformedWire, 1),
		})

	// Wrong wire dominates duplicate at the same completed endpoint regardless
	// of which physical occurrence arrived first.
	for _, fields := range [][][]byte{
		{
			profilerBlockTypedEncodeField(1, profilerBlockTypedBaseValues(210)[1]),
			profilerBlockTypedWrongWire(1, 0),
		},
		{
			profilerBlockTypedWrongWire(1, 0),
			profilerBlockTypedEncodeField(1, profilerBlockTypedBaseValues(210)[1]),
		},
	} {
		payload := profilerBlockTypedPayload(210, nil, 1)
		payload = profilerBlockPrecedenceAppend(payload, fields...)
		requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, payload), false,
			[]profilerFtraceEventIssue{
				profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldWrongWire, 1),
			})
	}

	// Completed hard failures are elected in schema-field order, never physical
	// wire order. The two payloads reverse the bad f1/f2 occurrences but must
	// publish the same exact f1 diagnosis.
	for _, fields := range [][][]byte{
		{profilerBlockTypedWrongWire(1, 0), profilerBlockTypedWrongWire(2, 0)},
		{profilerBlockTypedWrongWire(2, 0), profilerBlockTypedWrongWire(1, 0)},
	} {
		payload := profilerBlockTypedPayload(210, nil, 1, 2)
		payload = profilerBlockPrecedenceAppend(payload, fields...)
		requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, payload), false,
			[]profilerFtraceEventIssue{
				profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldWrongWire, 1),
			})
	}

	for _, fields := range [][][]byte{
		{
			profilerBlockTypedEncodeField(1, profilerBlockTypedBaseValues(210)[1]),
			profilerBlockTypedEncodeField(2, profilerBlockTypedBaseValues(210)[2]),
		},
		{
			profilerBlockTypedEncodeField(2, profilerBlockTypedBaseValues(210)[2]),
			profilerBlockTypedEncodeField(1, profilerBlockTypedBaseValues(210)[1]),
		},
	} {
		payload := profilerBlockTypedPayload(210, nil)
		payload = profilerBlockPrecedenceAppend(payload, fields...)
		requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, payload), false,
			[]profilerFtraceEventIssue{
				profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldDuplicate, 1),
			})
	}
}

func TestProfilerBlockPrecedenceCompletedHardRangeSemanticAndDisplay(t *testing.T) {
	lowerChanges := map[int]profilerBlockTypedValue{
		2: {wire: 0, u64: uint64(math.MaxInt64) + 1},
		5: {wire: 2, text: "R|W"},
	}

	// A completed hard endpoint suppresses range, semantic, and both display
	// degradations. The display endpoints arrive first physically to prove the
	// result is not an encounter-order accident.
	hard := profilerBlockTypedPayload(210, lowerChanges, 1, 6, 7)
	hard = profilerBlockPrecedenceAppend(hard,
		profilerBlockTypedWrongWire(7, 2),
		profilerBlockTypedWrongWire(6, 2),
		profilerBlockTypedWrongWire(1, 0),
	)
	requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, hard), false,
		[]profilerFtraceEventIssue{
			profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldWrongWire, 1),
		})

	// Range validation precedes semantic RWBS validation, and both precede
	// optional display admission.
	rangeFault := profilerBlockTypedPayload(210, lowerChanges, 6, 7)
	rangeFault = profilerBlockPrecedenceAppend(rangeFault,
		profilerBlockTypedWrongWire(7, 2),
		profilerBlockTypedWrongWire(6, 2),
	)
	requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, rangeFault), false,
		[]profilerFtraceEventIssue{
			profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldOutOfRange, 2),
		})

	semantic := profilerBlockTypedPayload(210, map[int]profilerBlockTypedValue{
		5: {wire: 2, text: "R|W"},
	}, 6, 7)
	semantic = profilerBlockPrecedenceAppend(semantic,
		profilerBlockTypedWrongWire(7, 2),
		profilerBlockTypedWrongWire(6, 2),
	)
	requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, semantic), false,
		[]profilerFtraceEventIssue{
			profilerBlockTypedPayloadIssue(t, 210, profilerFtraceEventIssueBlockFieldMissingOrInvalid, 5),
		})

	// With every harder arm clean, independently bad display endpoints coexist
	// in comm->cmd schema order even though their physical wire order is reversed.
	display := profilerBlockTypedPayload(210, nil, 6, 7)
	display = profilerBlockPrecedenceAppend(display,
		profilerBlockTypedWrongWire(7, 2),
		profilerBlockTypedWrongWire(6, 2),
	)
	name, body := requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(210, display), true,
		[]profilerFtraceEventIssue{
			profilerBlockTypedFixedIssue(t, 210, profilerFtraceEventIssueBlockCommWrongWire),
			profilerBlockTypedFixedIssue(t, 210, profilerFtraceEventIssueBlockCmdWrongWire),
		})
	if name != "block_rq_insert" || body != "0,1 R 4 () 2 + 3 []" {
		t.Fatalf("display-only block output drifted: name=%q body=%q", name, body)
	}
}

func TestProfilerBlockPrecedenceEveryDisplayEndpointCompositeOrder(t *testing.T) {
	type endpoint struct {
		event, field                         int
		unsafe                               string
		malformed, wrong, duplicate, omitted profilerFtraceEventIssueKind
	}
	endpoints := []endpoint{
		{204, 5, "bad]", profilerFtraceEventIssueBlockCommMalformedWire, profilerFtraceEventIssueBlockCommWrongWire, profilerFtraceEventIssueBlockCommDuplicate, profilerFtraceEventIssueBlockCommUnsafeOmitted},
		{209, 6, "bad)", profilerFtraceEventIssueBlockCmdMalformedWire, profilerFtraceEventIssueBlockCmdWrongWire, profilerFtraceEventIssueBlockCmdDuplicate, profilerFtraceEventIssueBlockCmdUnsafeOmitted},
		{210, 6, "bad]", profilerFtraceEventIssueBlockCommMalformedWire, profilerFtraceEventIssueBlockCommWrongWire, profilerFtraceEventIssueBlockCommDuplicate, profilerFtraceEventIssueBlockCommUnsafeOmitted},
		{210, 7, "bad)", profilerFtraceEventIssueBlockCmdMalformedWire, profilerFtraceEventIssueBlockCmdWrongWire, profilerFtraceEventIssueBlockCmdDuplicate, profilerFtraceEventIssueBlockCmdUnsafeOmitted},
		{211, 6, "bad]", profilerFtraceEventIssueBlockCommMalformedWire, profilerFtraceEventIssueBlockCommWrongWire, profilerFtraceEventIssueBlockCommDuplicate, profilerFtraceEventIssueBlockCommUnsafeOmitted},
		{211, 7, "bad)", profilerFtraceEventIssueBlockCmdMalformedWire, profilerFtraceEventIssueBlockCmdWrongWire, profilerFtraceEventIssueBlockCmdDuplicate, profilerFtraceEventIssueBlockCmdUnsafeOmitted},
	}
	for _, endpoint := range endpoints {
		unsafeField := profilerBlockTypedEncodeField(endpoint.field, profilerBlockTypedValue{wire: 2, text: endpoint.unsafe})
		wrongField := profilerBlockTypedWrongWire(endpoint.field, 2)
		malformedField := profilerBlockTypedMalformedField(endpoint.field, 2)
		cases := []struct {
			name   string
			fields [][]byte
			kind   profilerFtraceEventIssueKind
		}{
			{name: "unsafe", fields: [][]byte{unsafeField}, kind: endpoint.omitted},
			{name: "duplicate-over-unsafe", fields: [][]byte{unsafeField, unsafeField}, kind: endpoint.duplicate},
			{name: "wrong-over-duplicate-unsafe", fields: [][]byte{unsafeField, unsafeField, wrongField}, kind: endpoint.wrong},
			// Malformed must be the physical tail: a terminal display value can
			// be localized only after the one-walk decoder proves no hidden hard
			// endpoint remains after it.
			{name: "malformed-over-wrong-duplicate-unsafe", fields: [][]byte{unsafeField, unsafeField, wrongField, malformedField}, kind: endpoint.malformed},
		}
		for _, test := range cases {
			t.Run(fmt.Sprintf("event%d/field%d/%s", endpoint.event, endpoint.field, test.name), func(t *testing.T) {
				payload := profilerBlockTypedPayload(endpoint.event, nil, endpoint.field)
				payload = profilerBlockPrecedenceAppend(payload, test.fields...)
				requireProfilerBlockPrecedence(t, profilerBlockTypedRecord(endpoint.event, payload), true,
					[]profilerFtraceEventIssue{
						profilerBlockTypedFixedIssue(t, endpoint.event, test.kind),
					})
			})
		}
	}
}

func TestProfilerBlockPrecedenceCanonicalReplacesDisplayWithSoleHardIssue(t *testing.T) {
	for _, eventField := range []int{210, 211} {
		t.Run(fmt.Sprintf("event%d", eventField), func(t *testing.T) {
			// The valid cmd is calibrated three bytes past the line cap while
			// comm still contains "io". Appending a wrong-wire comm both creates
			// an admitted display issue and removes those two display bytes, so
			// the real canonical line remains exactly cap+1 under a valid envelope.
			event := profilerBlockTypedCanonicalFixture(t, eventField, 7, 3)
			event.Payload = append(event.Payload, profilerBlockTypedWrongWire(6, 2)...)

			decoded, admission, set, handled, err := decodeProfilerBlockPayloadWithTypedAudit(event)
			if err != nil || !handled || admission != bodyAdmitted || decoded == (blockRenderPayload{}) {
				t.Fatalf("canonical replacement fixture did not reach admitted display arm: admission=%d handled=%t payload=%+v err=%v",
					admission, handled, decoded, err)
			}
			displayIssues, err := set.checked(event.Field)
			wantDisplay := []profilerFtraceEventIssue{
				profilerBlockTypedFixedIssue(t, eventField, profilerFtraceEventIssueBlockCommWrongWire),
			}
			if err != nil || !reflect.DeepEqual(displayIssues, wantDisplay) {
				t.Fatalf("canonical fixture lost pre-finalize display issue: got=%+v want=%+v err=%v",
					displayIssues, wantDisplay, err)
			}
			canonicalBody := renderCanonicalBlockPayload(decoded)
			_, canonicalName, governed := blockRenderKindForProfilerField(eventField)
			if !governed {
				t.Fatalf("canonical fixture event escaped block governance: %d", eventField)
			}
			canonicalLine := traceDBFormatLine(event.Comm, event.PID, event.TGID, event.CPU, int64(event.TSNS),
				event.CommonFlags, event.CommonPreemptCount, canonicalName+": "+canonicalBody)
			if len(canonicalLine) != maxTraceDBSystraceLineBytes+1 {
				t.Fatalf("composite canonical fixture is not exact cap+1: len=%d want=%d",
					len(canonicalLine), maxTraceDBSystraceLineBytes+1)
			}

			wantCanonical := []profilerFtraceEventIssue{
				profilerBlockTypedFixedIssue(t, eventField, profilerFtraceEventIssueBlockInvalidCanonicalLine),
			}
			requireProfilerBlockPrecedence(t, event, false, wantCanonical)
			name, body, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
			if err != nil || name != "" || body != "" || ok || !reflect.DeepEqual(issues, wantCanonical) {
				t.Fatalf("outer typed canonical replacement drifted: name=%q body=%q ok=%t issues=%+v want=%+v err=%v",
					name, body, ok, issues, wantCanonical, err)
			}
		})
	}
}
