package hitraceconv

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type profilerAuxCancelAtPollContext struct {
	context.Context
	cancelAt int
	polls    int
	err      error
}

func (ctx *profilerAuxCancelAtPollContext) Err() error {
	ctx.polls++
	if ctx.cancelAt > 0 && ctx.polls >= ctx.cancelAt {
		return ctx.err
	}
	return ctx.Context.Err()
}

func profilerAuxPrintEvent(raw []byte) profilerFtraceEventRecord {
	return profilerFtraceEventRecord{
		Field: 1109, Payload: protoPayload(protoVarint(1, 7), protoBytes(2, raw)),
		TSNS: 1_000, CPU: 1, PID: 7, TGID: 7, Comm: "worker",
	}
}

func profilerAuxF2FSEvent() profilerFtraceEventRecord {
	return profilerFtraceEventRecord{
		Field: 4009, Payload: protoPayload(protoVarint(1, 0x101), protoVarint(2, 0x202)),
		TSNS: 2_000, CPU: 2, PID: 9, TGID: 9, Comm: "fs-worker", HeaderOwnerKnown: true,
	}
}

func profilerAuxMMCStartEvent(name string) profilerFtraceEventRecord {
	return profilerFtraceEventRecord{
		Field: 4016, Payload: protoPayload(protoVarint(24, 0x1234), protoBytes(25, []byte(name))),
		TSNS: 3_000, CPU: 3, PID: 11, TGID: 11, Comm: "mmc-worker",
	}
}

func TestProfilerAuxContextLegacyBackgroundNilParity(t *testing.T) {
	tests := []struct {
		name  string
		event profilerFtraceEventRecord
	}{
		{name: "print", event: profilerAuxPrintEvent([]byte("B|7|compile\n"))},
		{name: "f2fs", event: profilerAuxF2FSEvent()},
		{name: "mmc", event: profilerAuxMMCStartEvent("mmcblk0")},
		{name: "malformed-wire", event: profilerFtraceEventRecord{Field: 4009, Payload: []byte{0x08}}},
		{name: "malformed-print", event: profilerAuxPrintEvent([]byte("bad\nline"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy, legacyErr := decodeProfilerAuxPayloadWithTypedAudit(test.event)
			background, backgroundErr := decodeProfilerAuxPayloadWithTypedAuditContext(context.Background(), test.event)
			nilContext, nilErr := decodeProfilerAuxPayloadWithTypedAuditContext(nil, test.event)
			if legacyErr != backgroundErr || legacyErr != nilErr ||
				!reflect.DeepEqual(legacy, background) || !reflect.DeepEqual(legacy, nilContext) {
				t.Fatalf("decode parity legacy=(%+v,%v) background=(%+v,%v) nil=(%+v,%v)",
					legacy, legacyErr, background, backgroundErr, nilContext, nilErr)
			}

			legacyName, legacyBody, legacyOK, legacyIssues, legacyPair, legacyHandled, legacyRenderErr :=
				renderProfilerFtraceAuxEventWithTypedAudit(test.event)
			for _, candidate := range []context.Context{context.Background(), nil} {
				name, body, ok, issues, pair, handled, err :=
					renderProfilerFtraceAuxEventWithTypedAuditContext(candidate, test.event)
				if name != legacyName || body != legacyBody || ok != legacyOK || handled != legacyHandled ||
					err != legacyRenderErr || !reflect.DeepEqual(issues, legacyIssues) || !reflect.DeepEqual(pair, legacyPair) {
					t.Fatalf("render parity legacy=(%q,%q,%t,%+v,%+v,%t,%v) got=(%q,%q,%t,%+v,%+v,%t,%v)",
						legacyName, legacyBody, legacyOK, legacyIssues, legacyPair, legacyHandled, legacyRenderErr,
						name, body, ok, issues, pair, handled, err)
				}
			}
		})
	}
}

func TestProfilerAuxContextPreCancellationIsAtomic(t *testing.T) {
	event := profilerAuxF2FSEvent()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, deadlineCancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer deadlineCancel()
	for _, test := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{name: "canceled", ctx: canceled, want: context.Canceled},
		{name: "deadline", ctx: deadline, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := decodeProfilerAuxPayloadWithTypedAuditContext(test.ctx, event)
			if err != test.want || !reflect.DeepEqual(result, profilerFtraceAuxTypedResult{Handled: true}) {
				t.Fatalf("decode result=%+v err=%T %v want zero/%v", result, err, err, test.want)
			}
			name, body, ok, issues, pair, handled, err :=
				renderProfilerFtraceAuxEventWithTypedAuditContext(test.ctx, event)
			if err != test.want || name != "" || body != "" || ok || issues != nil || !handled ||
				!reflect.DeepEqual(pair, profilerPairAdmission{}) {
				t.Fatalf("render=(%q,%q,%t,%+v,%+v,%t,%T %v) want atomic %v",
					name, body, ok, issues, pair, handled, err, err, test.want)
			}
		})
	}

	unsupported := profilerFtraceEventRecord{Field: 777}
	result, err := decodeProfilerAuxPayloadWithTypedAuditContext(canceled, unsupported)
	if err != context.Canceled || !reflect.DeepEqual(result, profilerFtraceAuxTypedResult{}) {
		t.Fatalf("unsupported canceled decode result=%+v err=%T %v", result, err, err)
	}
	name, body, ok, issues, pair, handled, err :=
		renderProfilerFtraceAuxEventWithTypedAuditContext(canceled, unsupported)
	if err != context.Canceled || name != "" || body != "" || ok || issues != nil || handled ||
		!reflect.DeepEqual(pair, profilerPairAdmission{}) {
		t.Fatalf("unsupported canceled render=(%q,%q,%t,%+v,%+v,%t,%T %v)",
			name, body, ok, issues, pair, handled, err, err)
	}
}

func TestProfilerAuxContextMidAndFinalCancellationIsProspective(t *testing.T) {
	longPrint := profilerAuxPrintEvent([]byte(strings.Repeat("p", 4*profilerContextByteCheckpointBytes+31)))
	tests := []struct {
		name  string
		event profilerFtraceEventRecord
	}{
		{name: "long-print", event: longPrint},
		{name: "f2fs-pair", event: profilerAuxF2FSEvent()},
		{name: "mmc-name", event: profilerAuxMMCStartEvent(strings.Repeat("m", 256))},
		{name: "malformed-wire", event: profilerFtraceEventRecord{Field: 4009, Payload: []byte{0x08}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calibrateDecode := &profilerAuxCancelAtPollContext{Context: context.Background()}
			if _, err := decodeProfilerAuxPayloadWithTypedAuditContext(calibrateDecode, test.event); err != nil {
				t.Fatalf("calibrate decode: %v", err)
			}
			if calibrateDecode.polls < 2 {
				t.Fatalf("decode exposes only %d cancellation polls", calibrateDecode.polls)
			}
			calibrateRender := &profilerAuxCancelAtPollContext{Context: context.Background()}
			if _, _, _, _, _, _, err := renderProfilerFtraceAuxEventWithTypedAuditContext(calibrateRender, test.event); err != nil {
				t.Fatalf("calibrate render: %v", err)
			}
			if calibrateRender.polls <= calibrateDecode.polls {
				t.Fatalf("render has no finalization boundary: decode=%d render=%d",
					calibrateDecode.polls, calibrateRender.polls)
			}

			invariant := &traceDBOutputInvariantError{Reason: "test_profiler_aux_context_invariant"}
			for _, want := range []error{context.Canceled, context.DeadlineExceeded, invariant} {
				for cancelAt := 1; cancelAt <= calibrateDecode.polls; cancelAt++ {
					ctx := &profilerAuxCancelAtPollContext{
						Context: context.Background(), cancelAt: cancelAt, err: want,
					}
					result, err := decodeProfilerAuxPayloadWithTypedAuditContext(ctx, test.event)
					if err != want || !reflect.DeepEqual(result, profilerFtraceAuxTypedResult{Handled: true}) {
						t.Fatalf("decode cancel=%d/%d polls=%d result=%+v err=%T %v want=%v",
							cancelAt, calibrateDecode.polls, ctx.polls, result, err, err, want)
					}
				}
				for cancelAt := 1; cancelAt <= calibrateRender.polls; cancelAt++ {
					ctx := &profilerAuxCancelAtPollContext{
						Context: context.Background(), cancelAt: cancelAt, err: want,
					}
					name, body, ok, issues, pair, handled, err :=
						renderProfilerFtraceAuxEventWithTypedAuditContext(ctx, test.event)
					if err != want || name != "" || body != "" || ok || issues != nil || !handled ||
						!reflect.DeepEqual(pair, profilerPairAdmission{}) {
						t.Fatalf("render cancel=%d/%d polls=%d got=(%q,%q,%t,%+v,%+v,%t,%T %v) want=%v",
							cancelAt, calibrateRender.polls, ctx.polls, name, body, ok, issues, pair, handled, err, err, want)
					}
				}
			}
		})
	}
}

func TestProfilerAuxMalformedSiblingRemainsLocal(t *testing.T) {
	malformed := profilerAuxPrintEvent([]byte("bad\nline"))
	legal := profilerAuxPrintEvent([]byte("B|7|legal-sibling\n"))
	_, _, malformedOK, malformedIssues, malformedPair, malformedHandled, malformedErr :=
		renderProfilerFtraceAuxEventWithTypedAuditContext(context.Background(), malformed)
	if malformedErr != nil || malformedOK || !malformedHandled || len(malformedIssues) != 1 ||
		malformedIssues[0].Kind != profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf ||
		!reflect.DeepEqual(malformedPair, profilerPairAdmission{}) {
		t.Fatalf("malformed sibling verdict=(ok=%t issues=%+v pair=%+v handled=%t err=%v)",
			malformedOK, malformedIssues, malformedPair, malformedHandled, malformedErr)
	}
	name, body, legalOK, legalIssues, legalPair, legalHandled, legalErr :=
		renderProfilerFtraceAuxEventWithTypedAuditContext(context.Background(), legal)
	if legalErr != nil || !legalOK || !legalHandled || name != "print" || body != "B|7|legal-sibling" ||
		len(legalIssues) != 0 || !reflect.DeepEqual(legalPair, profilerPairAdmission{Admitted: true}) {
		t.Fatalf("legal sibling verdict=(%q,%q,ok=%t issues=%+v pair=%+v handled=%t err=%v)",
			name, body, legalOK, legalIssues, legalPair, legalHandled, legalErr)
	}

	malformedF2FS := profilerAuxF2FSEvent()
	malformedF2FS.Payload = append(malformedF2FS.Payload, protoBytes(3, []byte("wrong-wire"))...)
	_, _, malformedOK, malformedIssues, malformedPair, malformedHandled, malformedErr =
		renderProfilerFtraceAuxEventWithTypedAuditContext(context.Background(), malformedF2FS)
	if malformedErr != nil || malformedOK || !malformedHandled || len(malformedIssues) != 1 ||
		malformedIssues[0].Kind != profilerFtraceEventIssueAuxFieldWrongWire || !malformedPair.Governed ||
		!malformedPair.LaneKnown || malformedPair.Lane == "" || malformedPair.Admitted {
		t.Fatalf("malformed F2FS sibling verdict=(ok=%t issues=%+v pair=%+v handled=%t err=%v)",
			malformedOK, malformedIssues, malformedPair, malformedHandled, malformedErr)
	}
	name, body, legalOK, legalIssues, legalPair, legalHandled, legalErr =
		renderProfilerFtraceAuxEventWithTypedAuditContext(context.Background(), profilerAuxF2FSEvent())
	if legalErr != nil || !legalOK || !legalHandled || name != "f2fs_sync_file_enter" || body == "" ||
		len(legalIssues) != 0 || !legalPair.Governed || !legalPair.LaneKnown || !legalPair.Admitted ||
		legalPair.Lane != malformedPair.Lane {
		t.Fatalf("legal F2FS sibling verdict=(%q,%q,ok=%t issues=%+v pair=%+v handled=%t err=%v)",
			name, body, legalOK, legalIssues, legalPair, legalHandled, legalErr)
	}
}

func TestNormalizeMarkerBufferContextParityAndCancellation(t *testing.T) {
	tests := [][]byte{
		nil, {}, []byte(" "), []byte("value"), []byte("value\n"), []byte("\n"),
		[]byte("value\n\n"), []byte("value\nnext"), []byte("value\r"), []byte("纹理上传"), {0xff},
	}
	for _, raw := range tests {
		legacy, legacyOK := normalizeMarkerBuffer(raw)
		for _, ctx := range []context.Context{context.Background(), nil} {
			got, ok, err := normalizeMarkerBufferContext(ctx, raw)
			if err != nil || got != legacy || ok != legacyOK {
				t.Fatalf("raw=%q context parity=(%q,%t,%v) legacy=(%q,%t)", raw, got, ok, err, legacy, legacyOK)
			}
		}
	}

	raw := []byte(strings.Repeat("x", 3*profilerContextByteCheckpointBytes+17) + "\n")
	calibrate := &profilerAuxCancelAtPollContext{Context: context.Background()}
	got, ok, err := normalizeMarkerBufferContext(calibrate, raw)
	if err != nil || !ok || got != string(raw[:len(raw)-1]) || calibrate.polls < 3 {
		t.Fatalf("calibrate marker got-bytes=%d ok=%t polls=%d err=%v", len(got), ok, calibrate.polls, err)
	}
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= calibrate.polls; cancelAt++ {
			ctx := &profilerAuxCancelAtPollContext{Context: context.Background(), cancelAt: cancelAt, err: want}
			got, ok, err := normalizeMarkerBufferContext(ctx, raw)
			if err != want || got != "" || ok {
				t.Fatalf("cancel=%d/%d got=(bytes=%d,%t,%T %v) want empty/%v",
					cancelAt, calibrate.polls, len(got), ok, err, err, want)
			}
		}
	}
}

func TestProfilerMMCNameContextMatchesLegacyGrammar(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("mmcblk0"), []byte(""), []byte(" "), []byte("mmc blk"), []byte("mmc:0"),
		[]byte("mmc[0]"), []byte("mmc=0"), []byte("mmc|0"), []byte("存储卡"), {0xff},
		[]byte(strings.Repeat("m", 256)), []byte(strings.Repeat("m", 257)),
	} {
		field := profilerCoreProtoField{count: 1, bytesValue: raw}
		got, valid, err := decodeProfilerMMCNameContext(context.Background(), field)
		want := validProfilerMMCName(string(raw))
		if err != nil || valid != want || (valid && got != string(raw)) || (!valid && got != "") {
			t.Fatalf("raw=%q got=(%q,%t,%v) legacy=%t", raw, got, valid, err, want)
		}
	}
}

func TestProfilerAuxContextStructurePinsSingleAuthority(t *testing.T) {
	auxBytes, err := os.ReadFile("profiler_aux_payload.go")
	if err != nil {
		t.Fatal(err)
	}
	aux := string(auxBytes)
	decode := sourceBetween(t, aux,
		"func decodeProfilerAuxPayloadWithTypedAuditContext(",
		"func decodeProfilerAuxPayload(")
	if strings.Count(decode, "walkProfilerProtoFieldsContext(ctx, event.Payload") != 1 ||
		strings.Contains(decode, "walkProtoFields(event.Payload") ||
		strings.Count(decode, "normalizeMarkerBufferContext(ctx, fields[2].bytesValue)") != 1 ||
		strings.Count(decode, "decodeProfilerMMCDoneContext(ctx, &fields)") != 1 ||
		strings.Count(decode, "decodeProfilerMMCStartContext(ctx, &fields)") != 1 {
		t.Fatalf("aux Context decode authority drifted")
	}
	callback := sourceBetween(t, decode,
		"walkProfilerProtoFieldsContext(ctx, event.Payload, func(",
		"if err := ctx.Err(); err != nil")
	if strings.Contains(callback, "string(raw)") || !strings.Contains(callback, "field.bytesValue = raw") {
		t.Fatalf("aux wire callback regained eager string conversion:\n%s", callback)
	}
	legacyDecode := sourceBetween(t, aux,
		"func decodeProfilerAuxPayloadWithTypedAudit(event",
		"func decodeProfilerAuxPayloadWithTypedAuditContext(")
	if strings.Count(legacyDecode,
		"decodeProfilerAuxPayloadWithTypedAuditContext(context.Background(), event)") != 1 ||
		strings.Contains(legacyDecode, "walkProfilerProtoFieldsContext") {
		t.Fatalf("legacy aux decode is not a Background-only wrapper:\n%s", legacyDecode)
	}
	legacyRender := sourceBetween(t, aux,
		"func renderProfilerFtraceAuxEventWithTypedAudit(event",
		"func renderProfilerFtraceAuxEventWithTypedAuditContext(")
	if strings.Count(legacyRender,
		"renderProfilerFtraceAuxEventWithTypedAuditContext(context.Background(), event)") != 1 {
		t.Fatalf("legacy aux render is not a Background-only wrapper:\n%s", legacyRender)
	}
	legacyFinalize := sourceBetween(t, aux,
		"func finalizeProfilerFtraceAuxEventWithTypedAudit(\n",
		"func finalizeProfilerFtraceAuxEventWithTypedAuditContext(")
	if strings.Count(legacyFinalize,
		"finalizeProfilerFtraceAuxEventWithTypedAuditContext(context.Background(), event, result)") != 1 {
		t.Fatalf("legacy aux finalize is not a Background-only wrapper:\n%s", legacyFinalize)
	}
	finalize := sourceBetween(t, aux,
		"func finalizeProfilerFtraceAuxEventWithTypedAuditContext(",
		"func profilerFtraceAuxPairAdmissionValid(")
	if strings.Count(finalize, "renderCanonicalProfilerAuxPayloadContext(ctx, result.Payload)") != 1 ||
		strings.Count(finalize, "profilerCanonicalLineValidContext(ctx, event, result.Payload.Name, body)") != 1 {
		t.Fatalf("aux Context canonical authority drifted")
	}
	mmcName := sourceBetween(t, aux,
		"func decodeProfilerMMCNameContext(",
		"func validProfilerMMCName(")
	if strings.Count(mmcName, "profilerSingleTokenBytesContext(ctx, field.bytesValue)") != 1 ||
		strings.Count(mmcName, "profilerCloneBytesStringContext(ctx, field.bytesValue)") != 1 ||
		strings.Contains(mmcName, "string(field.bytesValue)") || strings.Contains(mmcName, "profilerCoreString(") {
		t.Fatalf("MMC Context name authority drifted:\n%s", mmcName)
	}

	markerBytes, err := os.ReadFile("marker_payload.go")
	if err != nil {
		t.Fatal(err)
	}
	marker := string(markerBytes)
	normalize := sourceBetween(t, marker,
		"func normalizeMarkerBufferContext(",
		"func directMarkerDeclarationCount(")
	if strings.Count(normalize, "profilerSinglePhysicalLineBytesContext(ctx, value, false)") != 1 ||
		strings.Count(normalize, "profilerCloneBytesStringContext(ctx, value)") != 1 ||
		strings.Contains(normalize, "string(raw)") || strings.Contains(normalize, "traceDBSinglePhysicalLine(") {
		t.Fatalf("marker buffer Context authority drifted:\n%s", normalize)
	}
	canonical := sourceBetween(t, marker,
		"func renderCanonicalMarkerPayloadContext(",
		"func normalizeMarkerBuffer(")
	if strings.Count(canonical, "profilerSinglePhysicalLineStringContext(ctx, payload.Buffer, false)") != 1 ||
		strings.Contains(canonical, "traceDBSinglePhysicalLine(") {
		t.Fatalf("marker canonical Context authority drifted:\n%s", canonical)
	}
}
