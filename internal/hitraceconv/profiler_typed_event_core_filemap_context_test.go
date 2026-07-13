package hitraceconv

import (
	"context"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type profilerCoreFilemapCancelContext struct {
	context.Context
	cancelAt int
	polls    int
	err      error
}

func (ctx *profilerCoreFilemapCancelContext) Err() error {
	ctx.polls++
	if ctx.cancelAt > 0 && ctx.polls >= ctx.cancelAt {
		return ctx.err
	}
	if ctx.Context == nil {
		return nil
	}
	return ctx.Context.Err()
}

type profilerTypedEventRenderVerdict struct {
	Name    string
	Body    string
	OK      bool
	Issues  []profilerFtraceEventIssue
	Handled bool
	Err     error
}

type profilerCoreDecodeVerdict struct {
	Payload   coreRenderPayload
	Admission bodyAdmission
	Set       profilerFtraceCoreIssueSet
	Handled   bool
	Err       error
}

type profilerFilemapDecodeVerdict struct {
	Payload   filemapRenderPayload
	Admission bodyAdmission
	Set       profilerFtraceFilemapIssueSet
	Handled   bool
	Err       error
}

func profilerCoreContextTestDecode(ctx context.Context, event profilerFtraceEventRecord) profilerCoreDecodeVerdict {
	payload, admission, set, handled, err := decodeProfilerCorePayloadWithTypedAuditContext(ctx, event)
	return profilerCoreDecodeVerdict{payload, admission, set, handled, err}
}

func profilerCoreLegacyTestDecode(event profilerFtraceEventRecord) profilerCoreDecodeVerdict {
	payload, admission, set, handled, err := decodeProfilerCorePayloadWithTypedAudit(event)
	return profilerCoreDecodeVerdict{payload, admission, set, handled, err}
}

func profilerFilemapContextTestDecode(ctx context.Context, event profilerFtraceEventRecord) profilerFilemapDecodeVerdict {
	payload, admission, set, handled, err := decodeProfilerFilemapPayloadWithTypedAuditContext(ctx, event)
	return profilerFilemapDecodeVerdict{payload, admission, set, handled, err}
}

func profilerFilemapLegacyTestDecode(event profilerFtraceEventRecord) profilerFilemapDecodeVerdict {
	payload, admission, set, handled, err := decodeProfilerFilemapPayloadWithTypedAudit(event)
	return profilerFilemapDecodeVerdict{payload, admission, set, handled, err}
}

func profilerCoreContextTestVerdict(ctx context.Context, event profilerFtraceEventRecord) profilerTypedEventRenderVerdict {
	name, body, ok, issues, handled, err := renderProfilerFtraceCoreEventWithTypedAuditContext(ctx, event)
	return profilerTypedEventRenderVerdict{name, body, ok, issues, handled, err}
}

func profilerCoreLegacyTestVerdict(event profilerFtraceEventRecord) profilerTypedEventRenderVerdict {
	name, body, ok, issues, handled, err := renderProfilerFtraceCoreEventWithTypedAudit(event)
	return profilerTypedEventRenderVerdict{name, body, ok, issues, handled, err}
}

func profilerFilemapContextTestVerdict(ctx context.Context, event profilerFtraceEventRecord) profilerTypedEventRenderVerdict {
	name, body, ok, issues, handled, err := renderProfilerFtraceFilemapEventWithTypedAuditContext(ctx, event)
	return profilerTypedEventRenderVerdict{name, body, ok, issues, handled, err}
}

func profilerFilemapLegacyTestVerdict(event profilerFtraceEventRecord) profilerTypedEventRenderVerdict {
	name, body, ok, issues, handled, err := renderProfilerFtraceFilemapEventWithTypedAudit(event)
	return profilerTypedEventRenderVerdict{name, body, ok, issues, handled, err}
}

func profilerAssertTypedEventContextError(t *testing.T, verdict profilerTypedEventRenderVerdict, want error) {
	t.Helper()
	if verdict.Name != "" || verdict.Body != "" || verdict.OK || len(verdict.Issues) != 0 || !verdict.Handled ||
		verdict.Err != want {
		t.Fatalf("context verdict was not zero or lost its governed route: got=%+v want_err=%v", verdict, want)
	}
}

func TestProfilerCoreFilemapContextLegacyBackgroundNilParity(t *testing.T) {
	coreBase := profilerCoreTypedBaseCases()[1400]
	coreLegal := profilerCoreTypedRecord(1400, profilerCoreTypedPayload(coreBase, nil))
	coreMalformed := coreLegal
	coreMalformed.Payload = append(append([]byte(nil), coreLegal.Payload...), 0x80)
	filemapLegal := profilerFilemapTestEvent(1000, profilerFilemapTestBasePayload())
	filemapMalformed := filemapLegal
	filemapMalformed.Payload = append(append([]byte(nil), filemapLegal.Payload...), 0x80)

	for _, test := range []struct {
		name    string
		legacy  profilerTypedEventRenderVerdict
		withCtx func(context.Context) profilerTypedEventRenderVerdict
	}{
		{"core/legal", profilerCoreLegacyTestVerdict(coreLegal), func(ctx context.Context) profilerTypedEventRenderVerdict {
			return profilerCoreContextTestVerdict(ctx, coreLegal)
		}},
		{"core/malformed", profilerCoreLegacyTestVerdict(coreMalformed), func(ctx context.Context) profilerTypedEventRenderVerdict {
			return profilerCoreContextTestVerdict(ctx, coreMalformed)
		}},
		{"filemap/legal", profilerFilemapLegacyTestVerdict(filemapLegal), func(ctx context.Context) profilerTypedEventRenderVerdict {
			return profilerFilemapContextTestVerdict(ctx, filemapLegal)
		}},
		{"filemap/malformed", profilerFilemapLegacyTestVerdict(filemapMalformed), func(ctx context.Context) profilerTypedEventRenderVerdict {
			return profilerFilemapContextTestVerdict(ctx, filemapMalformed)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.legacy.Err != nil {
				t.Fatalf("legacy verdict failed: %+v", test.legacy)
			}
			for _, ctx := range []context.Context{context.Background(), nil} {
				got := test.withCtx(ctx)
				if !reflect.DeepEqual(got, test.legacy) {
					t.Fatalf("context parity drifted: got=%+v legacy=%+v", got, test.legacy)
				}
			}
		})
	}

	for _, ctx := range []context.Context{context.Background(), nil} {
		for _, event := range []profilerFtraceEventRecord{coreLegal, coreMalformed} {
			got, legacy := profilerCoreContextTestDecode(ctx, event), profilerCoreLegacyTestDecode(event)
			if !reflect.DeepEqual(got, legacy) {
				t.Fatalf("core decoder context parity drifted: got=%+v legacy=%+v", got, legacy)
			}
		}
		for _, event := range []profilerFtraceEventRecord{filemapLegal, filemapMalformed} {
			got, legacy := profilerFilemapContextTestDecode(ctx, event), profilerFilemapLegacyTestDecode(event)
			if !reflect.DeepEqual(got, legacy) {
				t.Fatalf("filemap decoder context parity drifted: got=%+v legacy=%+v", got, legacy)
			}
		}
	}
}

func TestProfilerCoreFilemapContextPreCancellationReturnsZeroVerdict(t *testing.T) {
	coreBase := profilerCoreTypedBaseCases()[119]
	core := profilerCoreTypedRecord(119, profilerCoreTypedPayload(coreBase, nil))
	filemap := profilerFilemapTestEvent(1000, profilerFilemapTestBasePayload())
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()

	for _, test := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"canceled", canceled, context.Canceled},
		{"deadline", expired, context.DeadlineExceeded},
	} {
		t.Run(test.name+"/core", func(t *testing.T) {
			profilerAssertTypedEventContextError(t, profilerCoreContextTestVerdict(test.ctx, core), test.want)
			payload, admission, set, handled, err := decodeProfilerCorePayloadWithTypedAuditContext(test.ctx, core)
			if payload != (coreRenderPayload{}) || admission != bodyUnsupported || set != (profilerFtraceCoreIssueSet{}) ||
				!handled || err != test.want {
				t.Fatalf("core decoder leaked verdict: payload=%+v admission=%d set=%+v handled=%v err=%v",
					payload, admission, set, handled, err)
			}
		})
		t.Run(test.name+"/filemap", func(t *testing.T) {
			profilerAssertTypedEventContextError(t, profilerFilemapContextTestVerdict(test.ctx, filemap), test.want)
			payload, admission, set, handled, err := decodeProfilerFilemapPayloadWithTypedAuditContext(test.ctx, filemap)
			if payload != (filemapRenderPayload{}) || admission != bodyUnsupported || set != (profilerFtraceFilemapIssueSet{}) ||
				!handled || err != test.want {
				t.Fatalf("filemap decoder leaked verdict: payload=%+v admission=%d set=%+v handled=%v err=%v",
					payload, admission, set, handled, err)
			}
		})
	}
}

func TestProfilerCoreContextLongStringsCancelWithoutVerdict(t *testing.T) {
	large := strings.Repeat("x", 4*profilerContextByteCheckpointBytes)
	base := profilerCoreTypedBaseCases()
	fixtures := []struct {
		name  string
		field int
		raw   []byte
	}{
		{"ipi_reason", 1400, profilerCoreTypedPayload(base[1400], map[int]profilerCoreTestValue{1: profilerCoreBytes(large)})},
		{"irq_name", 1500, profilerCoreTypedPayload(base[1500], map[int]profilerCoreTestValue{2: profilerCoreBytes(large)})},
		{"wakeup_comm", 2420, profilerCoreTypedPayload(base[2420], map[int]profilerCoreTestValue{1: profilerCoreBytes(large)})},
		{"blocked_caller", 4002, profilerCoreTypedPayload(base[4002], map[int]profilerCoreTestValue{4: profilerCoreBytes(large)})},
	}
	for _, fixture := range fixtures {
		for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
			name := fixture.name + "/" + strings.TrimPrefix(want.Error(), "context ")
			t.Run(name, func(t *testing.T) {
				ctx := &profilerCoreFilemapCancelContext{
					Context: context.Background(), cancelAt: 10, err: want,
				}
				profilerAssertTypedEventContextError(t,
					profilerCoreContextTestVerdict(ctx, profilerCoreTypedRecord(fixture.field, fixture.raw)), want)
				if ctx.polls != ctx.cancelAt {
					t.Fatalf("cancellation was not observed inside bounded string work: polls=%d cancel_at=%d",
						ctx.polls, ctx.cancelAt)
				}
			})
		}
	}
}

func TestProfilerCoreBlockedCallerOverProfilePreservesLegacyVerdict(t *testing.T) {
	base := profilerCoreTypedBaseCases()[4002]
	event := profilerCoreTypedRecord(4002, profilerCoreTypedPayload(base,
		map[int]profilerCoreTestValue{4: profilerCoreBytes(strings.Repeat("a", 513))}))
	legacy := profilerCoreLegacyTestVerdict(event)
	got := profilerCoreContextTestVerdict(context.Background(), event)
	if !reflect.DeepEqual(got, legacy) {
		t.Fatalf("over-profile caller Context/legacy parity drifted:\n got=%+v\nlegacy=%+v", got, legacy)
	}
	if legacy.Err != nil || !legacy.Handled || !legacy.OK || len(legacy.Issues) != 1 ||
		legacy.Issues[0].Kind != profilerFtraceEventIssueCoreDisplayCallerStrInvalid {
		t.Fatalf("over-profile caller no longer follows the pinned display-omission verdict: %+v", legacy)
	}
}

func TestProfilerFilemapContextMidWalkCancellationReturnsZeroVerdict(t *testing.T) {
	parts := make([][]byte, 0, 605)
	for index := 0; index < 600; index++ {
		parts = append(parts, profilerFilemapTestProtoVarint(6, uint64(index)))
	}
	parts = append(parts, profilerFilemapTestBasePayload())
	event := profilerFilemapTestEvent(1000, profilerFilemapTestPayload(parts...))
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(strings.TrimPrefix(want.Error(), "context "), func(t *testing.T) {
			ctx := &profilerCoreFilemapCancelContext{
				Context: context.Background(), cancelAt: 5, err: want,
			}
			profilerAssertTypedEventContextError(t, profilerFilemapContextTestVerdict(ctx, event), want)
			if ctx.polls != ctx.cancelAt {
				t.Fatalf("cancellation was not observed during scalar wire walk: polls=%d cancel_at=%d",
					ctx.polls, ctx.cancelAt)
			}
		})
	}
}

func TestProfilerCoreFilemapContextFinalCancellationPrecedesCanonicalIssue(t *testing.T) {
	coreBase := profilerCoreTypedBaseCases()[119]
	core := profilerCoreTypedRecord(119, profilerCoreTypedPayload(coreBase, nil))
	corePayload, coreAdmission, coreSet, handled, err := decodeProfilerCorePayloadWithTypedAuditContext(context.Background(), core)
	if err != nil || !handled || coreAdmission != bodyAdmitted {
		t.Fatalf("core setup failed: handled=%v admission=%d err=%v", handled, coreAdmission, err)
	}
	filemap := profilerFilemapTestEvent(1000, profilerFilemapTestBasePayload())
	filemapPayload, filemapAdmission, filemapSet, handled, err := decodeProfilerFilemapPayloadWithTypedAuditContext(context.Background(), filemap)
	if err != nil || !handled || filemapAdmission != bodyAdmitted {
		t.Fatalf("filemap setup failed: handled=%v admission=%d err=%v", handled, filemapAdmission, err)
	}
	core.TSNS, filemap.TSNS = math.MaxInt64+1, math.MaxInt64+1

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(strings.TrimPrefix(want.Error(), "context ")+"/core", func(t *testing.T) {
			ctx := &profilerCoreFilemapCancelContext{Context: context.Background(), cancelAt: 4, err: want}
			name, body, ok, issues, gotErr := finalizeProfilerFtraceCoreEventWithTypedAuditContext(
				ctx, core, corePayload, coreAdmission, coreSet,
			)
			if name != "" || body != "" || ok || len(issues) != 0 || gotErr != want || ctx.polls != ctx.cancelAt {
				t.Fatalf("core canonical issue beat context: name=%q body=%q ok=%v issues=%+v err=%v polls=%d",
					name, body, ok, issues, gotErr, ctx.polls)
			}
		})
		t.Run(strings.TrimPrefix(want.Error(), "context ")+"/filemap", func(t *testing.T) {
			ctx := &profilerCoreFilemapCancelContext{Context: context.Background(), cancelAt: 4, err: want}
			name, body, ok, issues, gotErr := finalizeProfilerFtraceFilemapEventWithTypedAuditContext(
				ctx, filemap, filemapPayload, filemapAdmission, filemapSet,
			)
			if name != "" || body != "" || ok || len(issues) != 0 || gotErr != want || ctx.polls != ctx.cancelAt {
				t.Fatalf("filemap canonical issue beat context: name=%q body=%q ok=%v issues=%+v err=%v polls=%d",
					name, body, ok, issues, gotErr, ctx.polls)
			}
		})
	}
}

func TestProfilerCoreFilemapMalformedSiblingLocality(t *testing.T) {
	coreBase := profilerCoreTypedBaseCases()[119]
	coreLegal := profilerCoreTypedRecord(119, profilerCoreTypedPayload(coreBase, nil))
	coreMalformed := coreLegal
	coreMalformed.Payload = append(append([]byte(nil), coreLegal.Payload...), 0x80)
	filemapLegal := profilerFilemapTestEvent(1000, profilerFilemapTestBasePayload())
	filemapMalformed := filemapLegal
	filemapMalformed.Payload = append(append([]byte(nil), filemapLegal.Payload...), 0x80)

	for _, test := range []struct {
		name      string
		malformed profilerTypedEventRenderVerdict
		legal     profilerTypedEventRenderVerdict
	}{
		{"core", profilerCoreContextTestVerdict(context.Background(), coreMalformed), profilerCoreContextTestVerdict(context.Background(), coreLegal)},
		{"filemap", profilerFilemapContextTestVerdict(context.Background(), filemapMalformed), profilerFilemapContextTestVerdict(context.Background(), filemapLegal)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.malformed.Err != nil || !test.malformed.Handled || test.malformed.OK || len(test.malformed.Issues) != 1 {
				t.Fatalf("malformed sibling escaped localized issue: %+v", test.malformed)
			}
			if test.legal.Err != nil || !test.legal.Handled || !test.legal.OK || len(test.legal.Issues) != 0 {
				t.Fatalf("healthy sibling was poisoned: %+v", test.legal)
			}
		})
	}
}

func TestProfilerCoreFilemapContextSingleAuthorityStructure(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	core := read("profiler_core_payload.go")
	coreLegacyDecode := sourceBetween(t, core,
		"func decodeProfilerCorePayloadWithTypedAudit(",
		"func decodeProfilerCorePayloadWithTypedAuditContext(")
	coreDecode := sourceBetween(t, core,
		"func decodeProfilerCorePayloadWithTypedAuditContext(",
		"func decodeProfilerCorePayload(event")
	if strings.Count(coreLegacyDecode,
		"decodeProfilerCorePayloadWithTypedAuditContext(context.Background(), event)") != 1 ||
		strings.Contains(coreLegacyDecode, "walkProtoFields(") ||
		strings.Contains(coreLegacyDecode, "walkProfilerProtoFieldsContext(") {
		t.Fatalf("legacy core decoder is not a Background-only wrapper:\n%s", coreLegacyDecode)
	}
	if strings.Count(coreDecode, "walkProfilerProtoFieldsContext(ctx, event.Payload") != 1 ||
		strings.Contains(coreDecode, "walkProtoFields(event.Payload") ||
		strings.Contains(coreDecode, "string(field.bytesValue)") ||
		strings.Contains(coreDecode, "string(raw)") {
		t.Fatalf("Context core decoder lost its single byte-backed authority:\n%s", coreDecode)
	}
	coreLegacyRender := sourceBetween(t, core,
		"func renderProfilerFtraceCoreEventWithTypedAudit(",
		"func renderProfilerFtraceCoreEventWithTypedAuditContext(")
	coreRender := sourceBetween(t, core,
		"func renderProfilerFtraceCoreEventWithTypedAuditContext(",
		"func finalizeProfilerFtraceCoreEventWithTypedAudit(")
	coreLegacyFinalize := sourceBetween(t, core,
		"func finalizeProfilerFtraceCoreEventWithTypedAudit(",
		"func finalizeProfilerFtraceCoreEventWithTypedAuditContext(")
	coreFinalize := sourceBetween(t, core,
		"func finalizeProfilerFtraceCoreEventWithTypedAuditContext(",
		"func profilerCoreDisplayField(")
	if strings.Count(coreLegacyRender,
		"renderProfilerFtraceCoreEventWithTypedAuditContext(context.Background(), event)") != 1 ||
		strings.Count(coreRender, "decodeProfilerCorePayloadWithTypedAuditContext(ctx, event)") != 1 ||
		strings.Count(coreRender, "finalizeProfilerFtraceCoreEventWithTypedAuditContext(ctx, event") != 1 ||
		strings.Count(coreLegacyFinalize, "context.Background(), event, payload, admission, set") != 1 ||
		strings.Count(coreFinalize, "profilerCanonicalLineValidContext(ctx, event, payload.Name, body)") != 1 {
		t.Fatal("core render/finalize Context choke drifted")
	}

	filemap := read("filemap_render.go")
	filemapLegacyDecode := sourceBetween(t, filemap,
		"func decodeProfilerFilemapPayloadWithTypedAudit(",
		"func decodeProfilerFilemapPayloadWithTypedAuditContext(")
	filemapDecode := sourceBetween(t, filemap,
		"func decodeProfilerFilemapPayloadWithTypedAuditContext(",
		"func renderProfilerFtraceFilemapEventWithTypedAudit(")
	if strings.Count(filemapLegacyDecode,
		"decodeProfilerFilemapPayloadWithTypedAuditContext(context.Background(), event)") != 1 ||
		strings.Contains(filemapLegacyDecode, "walkProtoFields(") ||
		strings.Count(filemapDecode, "walkProfilerProtoFieldsContext(ctx, event.Payload") != 1 ||
		strings.Contains(filemapDecode, "walkProtoFields(event.Payload") {
		t.Fatal("filemap decoder lost its Background wrapper or Context single-walk authority")
	}
	filemapLegacyRender := sourceBetween(t, filemap,
		"func renderProfilerFtraceFilemapEventWithTypedAudit(",
		"func renderProfilerFtraceFilemapEventWithTypedAuditContext(")
	filemapRender := sourceBetween(t, filemap,
		"func renderProfilerFtraceFilemapEventWithTypedAuditContext(",
		"func finalizeProfilerFtraceFilemapEventWithTypedAudit(")
	filemapLegacyFinalize := sourceBetween(t, filemap,
		"func finalizeProfilerFtraceFilemapEventWithTypedAudit(",
		"func finalizeProfilerFtraceFilemapEventWithTypedAuditContext(")
	filemapFinalize := sourceBetween(t, filemap,
		"func finalizeProfilerFtraceFilemapEventWithTypedAuditContext(",
		"func decodeProfilerFilemapPayload(event")
	if strings.Count(filemapLegacyRender,
		"renderProfilerFtraceFilemapEventWithTypedAuditContext(context.Background(), event)") != 1 ||
		strings.Count(filemapRender, "decodeProfilerFilemapPayloadWithTypedAuditContext(ctx, event)") != 1 ||
		strings.Count(filemapRender, "finalizeProfilerFtraceFilemapEventWithTypedAuditContext(") != 1 ||
		strings.Count(filemapLegacyFinalize, "context.Background(), event, payload, admission, set") != 1 ||
		strings.Count(filemapFinalize, "profilerCanonicalLineValidContext(ctx, event, payload.Name, body)") != 1 {
		t.Fatal("filemap render/finalize Context choke drifted")
	}
}
