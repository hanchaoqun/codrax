package hitraceconv

import (
	"math"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func decodeProfilerBlockPairForTest(t *testing.T, event profilerFtraceEventRecord) (profilerPairAdmission, bodyAdmission) {
	t.Helper()
	var pair profilerPairAdmission
	_, admission, _, handled, err := decodeProfilerBlockPayloadWithTypedAuditInto(event, &pair)
	if err != nil || !handled {
		t.Fatalf("structured Block decode failed: handled=%t admission=%d pair=%+v err=%v", handled, admission, pair, err)
	}
	return pair, admission
}

func TestProfilerStructuredBlockEndpointRosterIsSingleClosedAuthority(t *testing.T) {
	t.Parallel()
	fields := profilerStructuredPairEventFields(pairRenderBlock)
	want := map[int]bool{202: true, 204: true, 209: true, 211: true}
	if len(fields) != len(want) {
		t.Fatalf("Block endpoint roster=%v", fields)
	}
	for _, field := range fields {
		if !want[field] || !profilerStructuredPairEventField(pairRenderBlock, field) ||
			profilerPairFamilyForField(field) != pairCriticalFormatFamilyBlock ||
			!profilerStructuredBlockPairFamily(field).Governed {
			t.Fatalf("Block endpoint roster consumer drift: field=%d fields=%v", field, fields)
		}
	}
	for _, inventory := range []int{205, 210, 212} {
		if profilerStructuredPairEventField(pairRenderBlock, inventory) ||
			profilerPairFamilyForField(inventory)&pairCriticalFormatFamilyBlock != 0 ||
			profilerStructuredBlockPairFamily(inventory).Governed {
			t.Fatalf("Block inventory field %d entered endpoint roster", inventory)
		}
	}
}

func TestProfilerBlockPairExactEndpointRosterAndTypedTextParity(t *testing.T) {
	t.Parallel()
	for _, eventField := range []int{202, 204, 209, 211} {
		eventField := eventField
		t.Run(profilerFtraceEventDescriptors[eventField].Name, func(t *testing.T) {
			t.Parallel()
			wantSlot, found := profilerPairEndpointForStructuredField(eventField)
			if !found {
				t.Fatalf("field %d has no typed endpoint", eventField)
			}
			event := profilerBlockTypedRecord(eventField, profilerBlockTypedPayload(eventField, nil))
			name, body, ok, issues, pair, err := renderProfilerFtraceEventBodyWithTypedAuditAndPair(event)
			if err != nil || !ok || len(issues) != 0 || !pair.Governed || pair.Kind != pairRenderBlock ||
				!pair.HeaderOwnerKnown || !pair.LaneKnown || !pair.Admitted {
				t.Fatalf("exact endpoint not admitted: name=%q body=%q ok=%t issues=%+v pair=%+v err=%v",
					name, body, ok, issues, pair, err)
			}
			if pair.Verdict.Family != tracequery.PairingEndpointBlock || !pair.Verdict.KeyKnown ||
				!pair.Verdict.PayloadAdmitted || !pair.Verdict.EmitterKnown || !pair.Verdict.EmitterAdmitted {
				t.Fatalf("typed verdict incomplete: %+v", pair.Verdict)
			}
			line := traceDBFormatLine(event.Comm, event.PID, event.TGID, event.CPU, int64(event.TSNS),
				event.CommonFlags, event.CommonPreemptCount, name+": "+body)
			wire := profilerTextPairAdmission(line)
			if pair.EndpointSlot != wantSlot || !wire.Governed || wire.Kind != pairRenderBlock || !wire.Admitted ||
				wire.EndpointSlot != wantSlot || wire.Lane != pair.Lane || wire.Verdict != pair.Verdict {
				t.Fatalf("typed/text verdict drift: typed=%+v text=%+v line=%q", pair, wire, line)
			}
		})
	}
	for _, inventoryField := range []int{205, 210, 212} {
		event := profilerBlockTypedRecord(inventoryField, profilerBlockTypedPayload(inventoryField, nil))
		pair, admission := decodeProfilerBlockPairForTest(t, event)
		if admission != bodyAdmitted || pair.Governed || pair.Kind != pairRenderUnknown ||
			pair.EndpointSlot != profilerPairEndpointNone {
			t.Fatalf("inventory field %d entered elapsed barrier: admission=%d pair=%+v", inventoryField, admission, pair)
		}
	}
}

func TestProfilerBlockTextRosterRejectsNearAndInventoryNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"BLOCK_RQ_ISSUE", "block_rq_issue_extra", "block_rq_insert",
		"block_bio_remap", "block_bio_queue_extra",
	} {
		line := "worker-40 [002] d..2 5.000000: " + name + ": 8,0 R 4096 () 32 + 8 []"
		if pair := profilerTextPairAdmission(line); pair.Governed || pair.EndpointSlot != profilerPairEndpointNone {
			t.Fatalf("near/inventory name entered Block barrier: name=%q pair=%+v", name, pair)
		}
	}
	for _, line := range []string{
		"worker-40 [002] d..2 5.000000: block_rq_issue 8,0 R 4096 () 32 + 8 []",
		"worker-40 [002] d..2 5.000000: block_rq_issue : 8,0 R 4096 () 32 + 8 []",
	} {
		if pair := profilerTextPairAdmission(line); pair.Governed {
			t.Fatalf("delimiter drift entered Block barrier: line=%q pair=%+v", line, pair)
		}
		if !profilerTextPairNormalizationCollision(line) {
			t.Fatalf("delimiter drift can be republished as a normalized endpoint: %q", line)
		}
	}
	canonical := "worker-40 [002] d..2 5.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 []"
	if profilerTextPairNormalizationCollision(canonical) {
		t.Fatalf("canonical endpoint flagged as normalization collision: %q", canonical)
	}
}

func TestProfilerBlockTextOwnerAndHeaderFailuresAreFamilyScoped(t *testing.T) {
	t.Parallel()
	body := "block_rq_issue: 0,1 R 4 () 2 + 3 []"
	validIdle := "<idle>-0 [002] d..2 5.000000: " + body
	if pair := profilerTextPairAdmission(validIdle); !pair.Governed || !pair.HeaderOwnerKnown || !pair.Admitted || !pair.LaneKnown {
		t.Fatalf("known idle text endpoint rejected: %+v", pair)
	}
	for _, line := range []string{
		"worker-bad [002] d..2 5.000000: " + body,
		"worker-40 [bad] d..2 5.000000: " + body,
		"worker-40 [002] d..2 NaN: " + body,
		"worker [002] d..2 5.000000: " + body,
	} {
		pair := profilerTextPairAdmission(line)
		if !pair.Governed || pair.Kind != pairRenderBlock || pair.LaneKnown || pair.Admitted || pair.HeaderOwnerKnown {
			t.Fatalf("malformed header gained exact Block lane: line=%q pair=%+v", line, pair)
		}
	}
}

func TestProfilerBlockPairRequiresExplicitSingularHardKeyAndOwner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*profilerFtraceEventRecord)
	}{
		{name: "missing dev", mutate: func(event *profilerFtraceEventRecord) {
			event.Payload = profilerBlockTypedPayload(211, nil, 1)
		}},
		{name: "wrong wire sector", mutate: func(event *profilerFtraceEventRecord) {
			event.Payload = profilerBlockTypedPayload(211, map[int]profilerBlockTypedValue{2: {wire: 2, text: "2"}})
		}},
		{name: "duplicate nr sector", mutate: func(event *profilerFtraceEventRecord) {
			event.Payload = append(event.Payload, protoVarint(3, 3)...)
		}},
		{name: "malformed nr sector", mutate: func(event *profilerFtraceEventRecord) {
			event.Payload = profilerBlockTypedPayload(211, nil, 3)
			event.Payload = append(event.Payload, profilerBlockTypedMalformedField(3, 0)...)
		}},
		{name: "dev out of range", mutate: func(event *profilerFtraceEventRecord) {
			event.Payload = profilerBlockTypedPayload(211, map[int]profilerBlockTypedValue{1: {wire: 0, u64: math.MaxUint32 + 1}})
		}},
		{name: "invalid operation", mutate: func(event *profilerFtraceEventRecord) {
			event.Payload = profilerBlockTypedPayload(211, map[int]profilerBlockTypedValue{5: {wire: 2, text: "R|W"}})
		}},
		{name: "common pid absent", mutate: func(event *profilerFtraceEventRecord) {
			event.HeaderOwnerPresent = false
		}},
		{name: "common pid unknown", mutate: func(event *profilerFtraceEventRecord) {
			event.HeaderOwnerKnown = false
		}},
		{name: "common pid out of range", mutate: func(event *profilerFtraceEventRecord) {
			event.PID = math.MaxInt32 + 1
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			event := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))
			test.mutate(&event)
			pair, _ := decodeProfilerBlockPairForTest(t, event)
			if !pair.Governed || pair.Kind != pairRenderBlock || pair.LaneKnown || pair.Admitted {
				t.Fatalf("unproven key/owner did not fail family-closed: %+v", pair)
			}
		})
	}
}

func TestProfilerBlockPairLocalizesOnlyProvenNonKeyFailure(t *testing.T) {
	t.Parallel()
	base := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))

	wrongBytes := base
	wrongBytes.Payload = profilerBlockTypedPayload(211, map[int]profilerBlockTypedValue{4: {wire: 2, text: "4096"}})
	pair, admission := decodeProfilerBlockPairForTest(t, wrongBytes)
	if admission != bodyRejected || !pair.LaneKnown || pair.Admitted || !pair.Verdict.KeyKnown {
		t.Fatalf("non-key wrong wire did not retain exact lane: admission=%d pair=%+v", admission, pair)
	}

	terminal := profilerBlockTypedRecord(209, profilerBlockTypedPayload(209, nil, 4))
	terminal.Payload = append(terminal.Payload, profilerBlockTypedMalformedField(4, 0)...)
	pair, admission = decodeProfilerBlockPairForTest(t, terminal)
	if admission != bodyRejected || !pair.LaneKnown || pair.Admitted || !pair.Verdict.KeyKnown {
		t.Fatalf("terminal non-key malformed value did not retain exact lane: admission=%d pair=%+v", admission, pair)
	}

	nonTerminal := profilerBlockTypedRecord(209, profilerBlockTypedPayload(209, nil))
	nonTerminal.Payload = append(nonTerminal.Payload, profilerBlockTypedRawKey(6, 3)...)
	nonTerminal.Payload = append(nonTerminal.Payload, protoVarint(8, 1)...)
	pair, admission = decodeProfilerBlockPairForTest(t, nonTerminal)
	if admission != bodyRejected || pair.LaneKnown || pair.Admitted {
		t.Fatalf("non-terminal malformed suffix escaped family close: admission=%d pair=%+v", admission, pair)
	}
}

func TestProfilerBlockPairOwnerPolicyAndFlushIdentity(t *testing.T) {
	t.Parallel()
	basePayload := profilerBlockTypedPayload(211, nil)
	idle := profilerBlockTypedRecord(211, basePayload)
	idle.PID = 0
	idle.TGID = 0
	idlePair, admission := decodeProfilerBlockPairForTest(t, idle)
	if admission != bodyAdmitted || !idlePair.HeaderOwnerKnown || !idlePair.Admitted || !idlePair.Verdict.IdleAllowed {
		t.Fatalf("explicit idle owner rejected: admission=%d pair=%+v", admission, idlePair)
	}
	other := profilerBlockTypedRecord(211, basePayload)
	other.PID = 41
	otherPair, _ := decodeProfilerBlockPairForTest(t, other)
	if !otherPair.Admitted || otherPair.Lane != idlePair.Lane {
		t.Fatalf("cross-emitter Block request split: idle=%+v other=%+v", idlePair, otherPair)
	}
	bio := profilerBlockTypedRecord(204, profilerBlockTypedPayload(204, nil))
	bioPair, _ := decodeProfilerBlockPairForTest(t, bio)
	if !bioPair.Admitted || bioPair.Lane == idlePair.Lane {
		t.Fatalf("RQ/BIO families collapsed for the same tuple: rq=%+v bio=%+v", idlePair, bioPair)
	}

	flushLanes := map[string]bool{}
	for _, operation := range []string{"F", "FS", "f", "fS"} {
		flush := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, map[int]profilerBlockTypedValue{
			2: {wire: 0, u64: 0}, 3: {wire: 0, u64: 0}, 5: {wire: 2, text: operation},
		}))
		flushPair, flushAdmission := decodeProfilerBlockPairForTest(t, flush)
		if flushAdmission != bodyAdmitted || !flushPair.Admitted || !flushPair.LaneKnown {
			t.Fatalf("valid zero-length flush %q rejected: admission=%d pair=%+v", operation, flushAdmission, flushPair)
		}
		if flushLanes[flushPair.Lane] {
			t.Fatalf("case-distinct flush operation collapsed lane: operation=%q pair=%+v", operation, flushPair)
		}
		flushLanes[flushPair.Lane] = true
	}
}

func TestProfilerBlockPairDistinguishesAbsentCommonPIDFromExplicitIdle(t *testing.T) {
	t.Parallel()
	payload := profilerBlockTypedPayload(211, nil)
	decode := func(t *testing.T, common []byte) profilerFtraceEventRecord {
		t.Helper()
		wire := testProfilerFtraceEnvelopeEvent(
			protoVarint(1, 1_000), protoVarint(2, 0), protoBytes(3, []byte("block")), common,
			protoMessage(211, payload),
		)
		record, err := decodeProfilerFtraceEventRecord(2, wire)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}

	absent := decode(t, protoMessage(50, nil))
	if !absent.HeaderOwnerKnown || absent.HeaderOwnerPresent {
		t.Fatalf("renderer compatibility/presence split lost: %+v", absent)
	}
	absentPair, _ := decodeProfilerBlockPairForTest(t, absent)
	if absentPair.LaneKnown || absentPair.Admitted || absentPair.HeaderOwnerKnown {
		t.Fatalf("absent common_pid became idle witness: %+v", absentPair)
	}

	explicit := decode(t, protoMessage(50, protoVarint(4, 0)))
	if !explicit.HeaderOwnerKnown || !explicit.HeaderOwnerPresent || explicit.PID != 0 {
		t.Fatalf("explicit idle owner not preserved: %+v", explicit)
	}
	explicitPair, _ := decodeProfilerBlockPairForTest(t, explicit)
	if !explicitPair.HeaderOwnerKnown || !explicitPair.LaneKnown || !explicitPair.Admitted {
		t.Fatalf("explicit idle owner not admitted: %+v", explicitPair)
	}
}

func TestProfilerBlockPairPredecodesBeforeEnvelopeFailClose(t *testing.T) {
	t.Parallel()
	keyKnown := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))
	if err := keyKnown.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeTimestampDuplicate); err != nil {
		t.Fatal(err)
	}
	_, _, ok, issues, pair, err := renderProfilerFtraceEventBodyWithTypedAuditAndPair(keyKnown)
	if err != nil || ok || len(issues) != 1 || !pair.LaneKnown || pair.Lane == "" || !pair.Verdict.KeyKnown {
		t.Fatalf("non-owner envelope failure lost exact Block lane: ok=%t issues=%+v pair=%+v err=%v", ok, issues, pair, err)
	}

	ownerUnknown := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))
	ownerUnknown.HeaderOwnerKnown = false
	if err := ownerUnknown.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeCommonPIDWrongWire); err != nil {
		t.Fatal(err)
	}
	_, _, ok, issues, pair, err = renderProfilerFtraceEventBodyWithTypedAuditAndPair(ownerUnknown)
	if err != nil || ok || len(issues) != 1 || pair.LaneKnown || pair.Admitted || pair.HeaderOwnerKnown {
		t.Fatalf("owner envelope failure escaped Block family: ok=%t issues=%+v pair=%+v err=%v", ok, issues, pair, err)
	}
}
