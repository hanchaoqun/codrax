package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func profilerBlockContextSentinelPair() profilerPairAdmission {
	return profilerPairAdmission{
		Kind: pairRenderF2FS, Governed: true, Admitted: true, LaneKnown: true,
		Lane: "caller-owned-sentinel", HeaderOwnerKnown: true,
	}
}

func TestProfilerBlockContextWrappersPreserveBackgroundAndNilParity(t *testing.T) {
	malformedPayload := profilerBlockTypedPayload(210, nil, 6)
	malformedPayload = append(malformedPayload, profilerBlockTypedMalformedField(6, 2)...)
	for _, fixture := range []struct {
		name  string
		event profilerFtraceEventRecord
	}{
		{name: "legal", event: profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))},
		{name: "malformed-display-sibling", event: profilerBlockTypedRecord(210, malformedPayload)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			legacyPair := profilerBlockContextSentinelPair()
			legacyPayload, legacyAdmission, legacySet, legacyHandled, legacyErr :=
				decodeProfilerBlockPayloadWithTypedAuditInto(fixture.event, &legacyPair)
			for _, test := range []struct {
				name string
				ctx  context.Context
			}{
				{name: "background", ctx: context.Background()},
				{name: "nil"},
			} {
				t.Run(test.name, func(t *testing.T) {
					pair := profilerBlockContextSentinelPair()
					payload, admission, set, handled, err :=
						decodeProfilerBlockPayloadWithTypedAuditIntoContext(test.ctx, fixture.event, &pair)
					if err != legacyErr || payload != legacyPayload || admission != legacyAdmission ||
						set != legacySet || handled != legacyHandled || pair != legacyPair {
						t.Fatalf("decode parity drifted: got=(%+v,%d,%+v,%t,%+v,%v) legacy=(%+v,%d,%+v,%t,%+v,%v)",
							payload, admission, set, handled, pair, err,
							legacyPayload, legacyAdmission, legacySet, legacyHandled, legacyPair, legacyErr)
					}

					name, body, ok, issues, renderHandled, renderErr :=
						renderProfilerFtraceBlockEventWithTypedAuditContext(test.ctx, fixture.event)
					legacyName, legacyBody, legacyOK, legacyIssues, legacyRenderHandled, legacyRenderErr :=
						renderProfilerFtraceBlockEventWithTypedAudit(fixture.event)
					if name != legacyName || body != legacyBody || ok != legacyOK ||
						!reflect.DeepEqual(issues, legacyIssues) || renderHandled != legacyRenderHandled || renderErr != legacyRenderErr {
						t.Fatalf("render parity drifted: got=(%q,%q,%t,%+v,%t,%v) legacy=(%q,%q,%t,%+v,%t,%v)",
							name, body, ok, issues, renderHandled, renderErr,
							legacyName, legacyBody, legacyOK, legacyIssues, legacyRenderHandled, legacyRenderErr)
					}
				})
			}
		})
	}
}

func TestProfilerBlockContextMalformedDisplayPreservesHealthySibling(t *testing.T) {
	payload := profilerBlockTypedPayload(211, nil, 6)
	payload = append(payload, profilerBlockTypedMalformedField(6, 2)...)
	event := profilerBlockTypedRecord(211, payload)
	pair := profilerBlockContextSentinelPair()
	decoded, admission, set, handled, err :=
		decodeProfilerBlockPayloadWithTypedAuditIntoContext(context.Background(), event, &pair)
	wantIssue := profilerBlockTypedFixedIssue(t, 211, profilerFtraceEventIssueBlockCommMalformedWire)
	issues, issuesErr := set.checked(event.Field)
	if err != nil || issuesErr != nil || !handled || admission != bodyAdmitted ||
		decoded.comm != "" || decoded.cmd != "READ" || decoded.rwbs != "R" ||
		!reflect.DeepEqual(issues, []profilerFtraceEventIssue{wantIssue}) || !pair.Admitted || !pair.LaneKnown {
		t.Fatalf("malformed display erased healthy sibling: decoded=%+v admission=%d issues=%+v handled=%t pair=%+v err=(%v,%v)",
			decoded, admission, issues, handled, pair, err, issuesErr)
	}
}

func TestProfilerBlockContextPairPublicationIsProspective(t *testing.T) {
	valid := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))
	pair := profilerBlockContextSentinelPair()
	_, admission, _, handled, err :=
		decodeProfilerBlockPayloadWithTypedAuditIntoContext(context.Background(), valid, &pair)
	if err != nil || !handled || admission != bodyAdmitted || !pair.Governed || !pair.Admitted || !pair.LaneKnown ||
		pair == profilerBlockContextSentinelPair() {
		t.Fatalf("admitted source verdict did not publish once: admission=%d handled=%t pair=%+v err=%v",
			admission, handled, pair, err)
	}

	rejected := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211,
		map[int]profilerBlockTypedValue{5: {wire: 2, text: "R|W"}}))
	pair = profilerBlockContextSentinelPair()
	_, admission, _, handled, err =
		decodeProfilerBlockPayloadWithTypedAuditIntoContext(context.Background(), rejected, &pair)
	if err != nil || !handled || admission != bodyRejected || !pair.Governed || pair.Admitted || pair.LaneKnown ||
		pair == profilerBlockContextSentinelPair() {
		t.Fatalf("rejected source verdict did not publish closed pair: admission=%d handled=%t pair=%+v err=%v",
			admission, handled, pair, err)
	}

	unsupported := profilerBlockTypedRecord(777, nil)
	pair = profilerBlockContextSentinelPair()
	_, admission, _, handled, err =
		decodeProfilerBlockPayloadWithTypedAuditIntoContext(context.Background(), unsupported, &pair)
	if err != nil || handled || admission != bodyUnsupported || pair != (profilerPairAdmission{}) {
		t.Fatalf("unsupported source verdict did not publish zero family: admission=%d handled=%t pair=%+v err=%v",
			admission, handled, pair, err)
	}
}

func TestProfilerBlockContextCancellationLeavesNoResultOrPairMutation(t *testing.T) {
	longComm := string(bytes.Repeat([]byte{'c'}, 4*profilerContextByteCheckpointBytes+17))
	event := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211,
		map[int]profilerBlockTypedValue{6: {wire: 2, text: longComm}}))
	calibration := &profilerByteCancelAfterPollContext{Context: context.Background()}
	pair := profilerBlockContextSentinelPair()
	payload, admission, set, handled, err :=
		decodeProfilerBlockPayloadWithTypedAuditIntoContext(calibration, event, &pair)
	if err != nil || !handled || admission != bodyAdmitted || payload == (blockRenderPayload{}) ||
		set != (profilerFtraceBlockIssueSet{}) || !pair.Admitted || calibration.polls < 12 {
		t.Fatalf("cancellation calibration failed: polls=%d payload=%+v admission=%d set=%+v handled=%t pair=%+v err=%v",
			calibration.polls, payload, admission, set, handled, pair, err)
	}

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 2; cancelAt <= calibration.polls; cancelAt++ {
			ctx := &profilerByteCancelAfterPollContext{
				Context: context.Background(), cancelAt: cancelAt, err: want,
			}
			pair = profilerBlockContextSentinelPair()
			payload, admission, set, handled, err =
				decodeProfilerBlockPayloadWithTypedAuditIntoContext(ctx, event, &pair)
			if err != want || payload != (blockRenderPayload{}) || admission != bodyUnsupported ||
				set != (profilerFtraceBlockIssueSet{}) || !handled || pair != profilerBlockContextSentinelPair() {
				t.Fatalf("cancelAt=%d/%d want=%v got payload=%+v admission=%d set=%+v handled=%t pair=%+v err=%T %v",
					cancelAt, calibration.polls, want, payload, admission, set, handled, pair, err, err)
			}
		}
	}

	preCanceled, cancel := context.WithCancel(context.Background())
	cancel()
	preDeadline, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer deadlineCancel()
	for _, pre := range []struct {
		ctx  context.Context
		want error
	}{
		{ctx: preCanceled, want: context.Canceled},
		{ctx: preDeadline, want: context.DeadlineExceeded},
	} {
		pair = profilerBlockContextSentinelPair()
		payload, admission, set, handled, err =
			decodeProfilerBlockPayloadWithTypedAuditIntoContext(pre.ctx, event, &pair)
		if err != pre.want || payload != (blockRenderPayload{}) || admission != bodyUnsupported ||
			set != (profilerFtraceBlockIssueSet{}) || !handled || pair != profilerBlockContextSentinelPair() {
			t.Fatalf("pre-cancel mutated output: want=%v payload=%+v admission=%d set=%+v handled=%t pair=%+v err=%T %v",
				pre.want, payload, admission, set, handled, pair, err, err)
		}
	}
}

func TestProfilerBlockContextPreCanceledRendererRetainsOnlyGovernedRouteOwnership(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
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
			for _, fixture := range []struct {
				name        string
				event       profilerFtraceEventRecord
				wantHandled bool
			}{
				{name: "governed", event: profilerBlockTypedRecord(211, nil), wantHandled: true},
				{name: "unsupported", event: profilerBlockTypedRecord(777, nil)},
			} {
				t.Run(fixture.name, func(t *testing.T) {
					name, body, ok, issues, handled, err :=
						renderProfilerFtraceBlockEventWithTypedAuditContext(test.ctx, fixture.event)
					if err != test.want || name != "" || body != "" || ok || issues != nil || handled != fixture.wantHandled {
						t.Fatalf("pre-canceled renderer leaked or lost route ownership: name=%q body=%q ok=%t issues=%+v handled=%t err=%T %v",
							name, body, ok, issues, handled, err, err)
					}
				})
			}
		})
	}
}

func TestProfilerBlockContextInvariantIdentityLeavesPairUntouched(t *testing.T) {
	event := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))
	want := &traceDBOutputInvariantError{Reason: "test_profiler_block_context_invariant"}
	ctx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 4, err: want,
	}
	pair := profilerBlockContextSentinelPair()
	payload, admission, set, handled, err :=
		decodeProfilerBlockPayloadWithTypedAuditIntoContext(ctx, event, &pair)
	if err != want || payload != (blockRenderPayload{}) || admission != bodyUnsupported ||
		set != (profilerFtraceBlockIssueSet{}) || !handled || pair != profilerBlockContextSentinelPair() {
		t.Fatalf("invariant identity/mutation drifted: payload=%+v admission=%d set=%+v handled=%t pair=%+v err=%T %v",
			payload, admission, set, handled, pair, err, err)
	}
}

func TestProfilerBlockFinalizeContextCancellationIsFailClosed(t *testing.T) {
	event := profilerBlockTypedRecord(211, profilerBlockTypedPayload(211, nil))
	payload, admission, set, handled, err := decodeProfilerBlockPayloadWithTypedAudit(event)
	if err != nil || !handled || admission != bodyAdmitted {
		t.Fatalf("finalize fixture decode failed: admission=%d handled=%t err=%v", admission, handled, err)
	}
	calibration := &profilerByteCancelAfterPollContext{Context: context.Background()}
	name, body, ok, issues, err := finalizeProfilerFtraceBlockEventWithTypedAuditContext(
		calibration, event, payload, admission, set)
	if err != nil || !ok || name == "" || body == "" || len(issues) != 0 || calibration.polls < 2 {
		t.Fatalf("finalize calibration failed: polls=%d name=%q body=%q ok=%t issues=%+v err=%v",
			calibration.polls, name, body, ok, issues, err)
	}
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= calibration.polls; cancelAt++ {
			ctx := &profilerByteCancelAfterPollContext{
				Context: context.Background(), cancelAt: cancelAt, err: want,
			}
			name, body, ok, issues, err = finalizeProfilerFtraceBlockEventWithTypedAuditContext(
				ctx, event, payload, admission, set)
			if err != want || name != "" || body != "" || ok || issues != nil {
				t.Fatalf("cancelAt=%d/%d want=%v got=(%q,%q,%t,%+v,%T %v)",
					cancelAt, calibration.polls, want, name, body, ok, issues, err, err)
			}
		}
	}
}

func TestProfilerBlockContextStructurePinsSingleAuthority(t *testing.T) {
	body, err := os.ReadFile("block_render.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	decode := sourceBetween(t, source,
		"func decodeProfilerBlockPayloadWithTypedAuditIntoContext(",
		"// decodeProfilerBlockPayloadWithTypedAuditInto preserves")
	if strings.Count(decode, "walkProfilerProtoFieldsContext(ctx, event.Payload") != 1 ||
		strings.Contains(decode, "walkProtoFields(event.Payload") ||
		strings.Count(decode, "*pairOut = prospectivePair") != 1 ||
		strings.Count(decode, "profilerBlockRWBSBytesContext(ctx, state.RawValue)") != 1 ||
		strings.Count(decode, "profilerBlockDisplayBytesContext(ctx, state.RawValue") != 2 ||
		strings.Count(decode, "profilerCloneBytesStringContext(ctx, state.RawValue)") != 3 {
		t.Fatalf("block Context authority structure drifted")
	}
	callback := sourceBetween(t, decode,
		"walkProfilerProtoFieldsContext(ctx, event.Payload, func(",
		"if errors.Is(walkErr")
	if strings.Contains(callback, "string(raw)") || !strings.Contains(callback, "state.RawValue = raw") {
		t.Fatalf("block wire callback regained string allocation or lost raw state:\n%s", callback)
	}
	legacy := sourceBetween(t, source,
		"func decodeProfilerBlockPayloadWithTypedAuditInto(event",
		"// decodeProfilerBlockPayloadWithTypedAudit preserves")
	if !strings.Contains(legacy, "context.Background()") || strings.Contains(legacy, "walkProfilerProtoFieldsContext") {
		t.Fatalf("legacy block decoder is not a Background-only wrapper:\n%s", legacy)
	}
	if !strings.Contains(source, "profilerCanonicalLineValidContext(ctx, event, canonicalName, body)") ||
		strings.Contains(decode, "string(raw)") {
		t.Fatal("block canonical/context byte authority pin drifted")
	}
}

func TestProfilerBlockDisplayByteContextMatchesLegacyGrammar(t *testing.T) {
	values := []string{
		"", "io", "READ", " bad", "bad ", "\u00a0bad", "bad\u3000", "bad]", "bad)",
		"内核线程", "a\tb", "a\nb", string([]byte{0xff, 0xfe}),
	}
	for _, value := range values {
		for _, forbidden := range []byte{']', ')'} {
			got, err := profilerBlockDisplayBytesContext(context.Background(), []byte(value), forbidden)
			if err != nil {
				t.Fatalf("value=%q forbidden=%q: %v", value, forbidden, err)
			}
			want := value == strings.TrimSpace(value) && traceDBSinglePhysicalLine(value, true) &&
				!strings.ContainsRune(value, rune(forbidden))
			if got != want {
				t.Fatalf("value=%q forbidden=%q got=%t want=%t", value, forbidden, got, want)
			}
		}
	}
	for _, value := range []string{"R", "RW", "R|W", " R", "R1", "写"} {
		got, err := profilerBlockRWBSBytesContext(context.Background(), []byte(value))
		if err != nil || got != validBlockRWBS(value) {
			t.Fatalf("rwbs=%q got=%t err=%v want=%t", value, got, err, validBlockRWBS(value))
		}
	}
}
