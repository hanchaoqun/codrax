package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

type profilerGenericContextResult struct {
	Name    string
	Body    string
	OK      bool
	Issues  []profilerFtraceEventIssue
	Handled bool
	Pair    profilerPairAdmission
}

func profilerGenericContextFixture(raw string) profilerFtraceEventRecord {
	return profilerGenericWireTestRecord(410, protoPayload(
		protoBytes(1, []byte(raw)),
		protoVarint(2, 1),
	))
}

func profilerGenericContextRender(ctx context.Context, event profilerFtraceEventRecord) (profilerGenericContextResult, error) {
	name, body, ok, issues, pair, err := renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(ctx, event)
	return profilerGenericContextResult{Name: name, Body: body, OK: ok, Issues: issues, Handled: true, Pair: pair}, err
}

func TestProfilerTypedGenericContextCompatibilityParity(t *testing.T) {
	fixtures := []profilerFtraceEventRecord{
		profilerGenericContextFixture("cpu_clk"),
		profilerGenericWireTestRecord(410, append(protoBytes(1, []byte("cpu_clk")), 0x80)),
	}
	for index, event := range fixtures {
		legacyName, legacyBody, legacyOK, legacyIssues, legacyPair, legacyErr :=
			renderProfilerFtraceEventBodyWithTypedAuditAndPair(event)
		want := profilerGenericContextResult{
			Name: legacyName, Body: legacyBody, OK: legacyOK, Issues: legacyIssues,
			Handled: true, Pair: legacyPair,
		}
		for _, item := range []struct {
			name string
			ctx  context.Context
		}{{"background", context.Background()}, {"nil", nil}} {
			got, err := profilerGenericContextRender(item.ctx, event)
			if !profilerSummaryMetadataErrorsMatch(err, legacyErr) || !reflect.DeepEqual(got, want) {
				t.Fatalf("fixture=%d lane=%s parity drifted:\n got=%+v err=%v\nwant=%+v err=%v",
					index, item.name, got, err, want, legacyErr)
			}
		}
	}
}

func TestProfilerTypedGenericCancellationIsExactAndProspective(t *testing.T) {
	longToken := strings.Repeat("x", 4*profilerContextByteCheckpointBytes+31)
	event := profilerGenericContextFixture(longToken)
	preCanceled, cancel := context.WithCancel(context.Background())
	cancel()

	probe := &profilerSummaryMetadataCancelContext{
		Context: context.Background(), targetSuffix: ".renderProfilerFtraceGenericEventWithTypedAuditContext",
	}
	if _, err := profilerGenericContextRender(probe, profilerGenericContextFixture("cpu_clk")); err != nil {
		t.Fatal(err)
	}
	if probe.polls < 2 {
		t.Fatalf("final cancellation calibration saw only %d polls", probe.polls)
	}

	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"pre-canceled", preCanceled, context.Canceled},
		{"mid-canceled", &profilerSummaryMetadataCancelContext{
			Context: context.Background(), targetSuffix: ".profilerPhysicalRuneFactsContext",
			ancestorSuffix: ".renderProfilerFtraceGenericEventWithTypedAuditContext", cancelAt: 2, err: context.Canceled,
		}, context.Canceled},
		{"final-deadline", &profilerSummaryMetadataCancelContext{
			Context: context.Background(), targetSuffix: ".renderProfilerFtraceGenericEventWithTypedAuditContext",
			cancelAt: probe.polls, err: context.DeadlineExceeded,
		}, context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := event
			if test.name == "final-deadline" {
				fixture = profilerGenericContextFixture("cpu_clk")
			}
			got, err := profilerGenericContextRender(test.ctx, fixture)
			if !errors.Is(err, test.want) || err != test.want {
				t.Fatalf("error identity drifted: got=%T %v want=%T %v", err, err, test.want, test.want)
			}
			if got.Name != "" || got.Body != "" || got.OK || len(got.Issues) != 0 || got.Pair != (profilerPairAdmission{}) {
				t.Fatalf("canceled generic event leaked a verdict: %+v", got)
			}
		})
	}
}

func TestProfilerTypedGenericMalformedWalkCancellationPrecedesSourceIssue(t *testing.T) {
	event := profilerGenericWireTestRecord(410,
		append(protoBytes(1, []byte("cpu_clk")), 0x80))
	calibration := &profilerSummaryMetadataCancelContext{
		Context: context.Background(), targetSuffix: ".renderProfilerFtraceGenericEventWithTypedAuditContext",
	}
	_, _, _, _, handled, err := renderProfilerFtraceGenericEventWithTypedAuditContext(calibration, event)
	if err != nil || !handled || calibration.polls < 2 {
		t.Fatalf("malformed generic cancellation calibration failed: polls=%d handled=%t err=%v",
			calibration.polls, handled, err)
	}
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		ctx := &profilerSummaryMetadataCancelContext{
			Context:      context.Background(),
			targetSuffix: ".renderProfilerFtraceGenericEventWithTypedAuditContext",
			cancelAt:     calibration.polls,
			err:          want,
		}
		name, body, ok, issues, handled, err :=
			renderProfilerFtraceGenericEventWithTypedAuditContext(ctx, event)
		if err != want || ctx.polls != calibration.polls || name != "" || body != "" || ok || issues != nil || !handled {
			t.Fatalf("malformed walk cancellation lost precedence: polls=%d/%d name=%q body=%q ok=%t issues=%+v handled=%t err=%T %v",
				ctx.polls, calibration.polls, name, body, ok, issues, handled, err, err)
		}
	}
}

func TestProfilerTypedEventEnvelopeCommCancellationReturnsZeroRecord(t *testing.T) {
	longComm := []byte(strings.Repeat("c", 4*profilerContextByteCheckpointBytes+31))
	raw := protoPayload(
		protoVarint(1, 1_000),
		protoVarint(2, 7),
		protoBytes(3, longComm),
		protoMessage(50, protoVarint(4, 7)),
		protoMessage(410, protoPayload(protoBytes(1, []byte("cpu_clk")), protoVarint(2, 1))),
	)
	ctx := &profilerSummaryMetadataCancelContext{
		Context: context.Background(), targetSuffix: ".profilerPhysicalRuneFactsContext",
		ancestorSuffix: ".decodeProfilerFtraceEventRecordContext", cancelAt: 2, err: context.DeadlineExceeded,
	}
	record, err := decodeProfilerFtraceEventRecordContext(ctx, 2, raw)
	if err != context.DeadlineExceeded || !reflect.DeepEqual(record, profilerFtraceEventRecord{}) {
		t.Fatalf("comm cancellation leaked record: record=%+v err=%T %v polls=%d", record, err, err, ctx.polls)
	}
}

func profilerTypedEventContextResult(events ...[]byte) profilerTracePluginResult {
	detail := protoPayload(append([][]byte{protoVarint(1, 2)}, events...)...)
	return decodeProfilerTracePluginResult(protoBytes(2, detail))
}

func TestProfilerTypedEventCancellationDoesNotCommitCurrentEventLedgers(t *testing.T) {
	longToken := strings.Repeat("x", 4*profilerContextByteCheckpointBytes+31)
	result := profilerTypedEventContextResult(
		syntheticTracePluginFtraceEvent(1_000, 7, 7, "worker", 410,
			protoPayload(protoBytes(1, []byte(longToken)), protoVarint(2, 1))),
	)
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 17
	var batch profilerFtraceEventBatchCensus
	ctx := &profilerSummaryMetadataCancelContext{
		Context: context.Background(), targetSuffix: ".profilerPhysicalRuneFactsContext",
		ancestorSuffix: ".renderProfilerFtraceGenericEventWithTypedAuditContext", cancelAt: 2, err: context.Canceled,
	}
	rows, coverage, summary, recognized, err := renderProfilerFtraceStructuredResultConsumerContext(
		ctx, result, &seq, sink, false, &batch, false)
	if err != context.Canceled || rows != 0 || seq != 17 || len(coverage) != 0 || recognized ||
		!reflect.DeepEqual(summary, profilerFtraceSummary{}) {
		t.Fatalf("canceled row result drifted: rows=%d seq=%d coverage=%+v summary=%+v recognized=%t err=%T %v",
			rows, seq, coverage, summary, recognized, err, err)
	}
	if !reflect.DeepEqual(batch, profilerFtraceEventBatchCensus{}) || len(sink.rows) != 0 || sink.stats.RowsAccepted != 0 {
		t.Fatalf("canceled current event mutated ledgers: batch=%+v rows=%d stats=%+v", batch, len(sink.rows), sink.stats)
	}
}

func TestProfilerTypedEventCancellationRetainsOnlyCompletedPrefix(t *testing.T) {
	longToken := strings.Repeat("x", 4*profilerContextByteCheckpointBytes+31)
	result := profilerTypedEventContextResult(
		syntheticTracePluginFtraceEvent(1_000, 7, 7, "first", 410,
			protoPayload(protoBytes(1, []byte("cpu_clk")), protoVarint(2, 1))),
		syntheticTracePluginFtraceEvent(2_000, 7, 7, "second", 410,
			protoPayload(protoBytes(1, []byte(longToken)), protoVarint(2, 2))),
	)
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	var batch profilerFtraceEventBatchCensus
	ctx := &profilerSummaryMetadataCancelContext{
		Context: context.Background(), targetSuffix: ".profilerPhysicalRuneFactsContext",
		ancestorSuffix: ".renderProfilerFtraceGenericEventWithTypedAuditContext", cancelAt: 7, err: context.Canceled,
	}
	rows, _, _, _, err := renderProfilerFtraceStructuredResultConsumerContext(ctx, result, &seq, sink, false, &batch, false)
	if err != context.Canceled || rows != 1 || seq != 1 || len(sink.rows) != 1 || sink.stats.RowsAccepted != 1 {
		t.Fatalf("completed-prefix conservation drifted: rows=%d seq=%d buffered=%d stats=%+v polls=%d err=%T %v",
			rows, seq, len(sink.rows), sink.stats, ctx.polls, err, err)
	}
	slot := batch.Slots[profilerFtraceEventSlot(410)]
	if slot.RowsRead != 1 || slot.RowsEmitted != 1 {
		t.Fatalf("canceled current event entered batch census: %+v", slot)
	}
}

func TestProfilerTypedF2FSTopDispatcherCancellationReturnsZeroPair(t *testing.T) {
	event := profilerAuxF2FSEvent()
	calibration := &profilerAuxCancelAtPollContext{Context: context.Background()}
	if _, _, _, _, _, err := renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(calibration, event); err != nil {
		t.Fatal(err)
	}
	if calibration.polls < 2 {
		t.Fatalf("dispatcher cancellation calibration saw only %d polls", calibration.polls)
	}
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= calibration.polls; cancelAt++ {
			ctx := &profilerAuxCancelAtPollContext{
				Context: context.Background(), cancelAt: cancelAt, err: want,
			}
			name, body, ok, issues, pair, err :=
				renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(ctx, event)
			if err != want || name != "" || body != "" || ok || issues != nil || pair != (profilerPairAdmission{}) {
				t.Fatalf("cancel=%d/%d leaked dispatcher verdict: name=%q body=%q ok=%t issues=%+v pair=%+v err=%T %v",
					cancelAt, calibration.polls, name, body, ok, issues, pair, err, err)
			}
		}
	}
}

func TestProfilerTypedDispatcherEnvelopeEarlyReturnHasFinalProspectivePoll(t *testing.T) {
	event := profilerGenericContextFixture("cpu_clk")
	if err := event.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeTimestampDuplicate); err != nil {
		t.Fatal(err)
	}
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		ctx := &profilerSummaryMetadataCancelContext{
			Context:      context.Background(),
			targetSuffix: ".renderProfilerFtraceEventBodyWithTypedAuditAndPairContext",
			cancelAt:     2,
			err:          want,
		}
		name, body, ok, issues, pair, err :=
			renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(ctx, event)
		if err != want || ctx.polls != 2 || name != "" || body != "" || ok || issues != nil ||
			pair != (profilerPairAdmission{}) {
			t.Fatalf("envelope early return escaped final prospective poll: polls=%d name=%q body=%q ok=%t issues=%+v pair=%+v err=%T %v",
				ctx.polls, name, body, ok, issues, pair, err, err)
		}
	}
}

func TestProfilerTypedUnsupportedRenderersStillHonorPreCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, deadlineCancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer deadlineCancel()
	unsupported := profilerFtraceEventRecord{Field: 777}
	for _, test := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{name: "canceled", ctx: canceled, want: context.Canceled},
		{name: "deadline", ctx: deadline, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertUnsupported := func(lane string, name, body string, ok bool,
				issues []profilerFtraceEventIssue, handled bool, err error,
			) {
				t.Helper()
				if err != test.want || name != "" || body != "" || ok || issues != nil || handled {
					t.Fatalf("%s unsupported cancellation drifted: name=%q body=%q ok=%t issues=%+v handled=%t err=%T %v",
						lane, name, body, ok, issues, handled, err, err)
				}
			}
			name, body, ok, issues, handled, err :=
				renderProfilerFtraceCoreEventWithTypedAuditContext(test.ctx, unsupported)
			assertUnsupported("core", name, body, ok, issues, handled, err)
			name, body, ok, issues, handled, err =
				renderProfilerFtraceFilemapEventWithTypedAuditContext(test.ctx, unsupported)
			assertUnsupported("filemap", name, body, ok, issues, handled, err)
			name, body, ok, issues, handled, err =
				renderProfilerFtraceGenericEventWithTypedAuditContext(test.ctx, unsupported)
			assertUnsupported("generic", name, body, ok, issues, handled, err)
		})
	}
}

func profilerTypedEventTransactionRow(tsNS uint64, seq int, label string) renderedRow {
	return renderedRow{tsNS: tsNS, seq: seq, line: "worker-7 [002] .... " + label}
}

func requireProfilerTypedEventTransactionEmpty(t *testing.T, sink *traceDBRowSink) {
	t.Helper()
	if len(sink.rows) != 0 || len(sink.rowIngestOrdinals) != 0 || sink.bufferedBytes != 0 ||
		sink.nextIngestOrdinal != 0 || len(sink.runs) != 0 || sink.stats.RowsAccepted != 0 ||
		sink.opaque[pairRenderF2FS] || sink.poisoned[pairRenderF2FS] ||
		sink.pairRows[pairRenderF2FS] != 0 || sink.structuredPairRows[pairRenderF2FS] != 0 ||
		sink.pairCensusActive || sink.activePairPublisher != profilerPairPublisherNone ||
		sink.textMessageActive || sink.activeTextMessage != 0 || sink.activeTextRows != 0 ||
		sink.nextTextMessage != 0 {
		t.Fatalf("current event escaped transaction: rows=%d ordinals=%d bytes=%d next=%d runs=%d stats=%+v opaque=%t poisoned=%t pair_rows=%d structured=%d",
			len(sink.rows), len(sink.rowIngestOrdinals), sink.bufferedBytes, sink.nextIngestOrdinal,
			len(sink.runs), sink.stats, sink.opaque[pairRenderF2FS],
			sink.poisoned[pairRenderF2FS], sink.pairRows[pairRenderF2FS], sink.structuredPairRows[pairRenderF2FS])
	}
	for _, kind := range profilerCaptureKinds {
		registry := sink.pairLaneRegistries[kind]
		if len(registry.byKey) != 0 || len(registry.keys) != 0 || len(registry.states) != 0 ||
			len(sink.pairLaneRows[kind]) != 0 || len(sink.pairTableRows[kind]) != 0 ||
			len(sink.poisonedLanes[kind]) != 0 || len(sink.structuredLaneRows[kind]) != 0 ||
			len(sink.structuredEventLanes[kind]) != 0 || len(sink.activePairCensus[kind].byLane) != 0 {
			t.Fatalf("current event escaped typed lane transaction for kind %d: registry=%+v lanes=%v tables=%v poisoned=%v structured=%v events=%v census=%+v",
				kind, registry, sink.pairLaneRows[kind], sink.pairTableRows[kind], sink.poisonedLanes[kind],
				sink.structuredLaneRows[kind], sink.structuredEventLanes[kind], sink.activePairCensus[kind])
		}
	}
}

func TestProfilerTypedEventSinkAddCancellationIsProspectiveAtEveryPoll(t *testing.T) {
	row := profilerTypedEventTransactionRow(1_000, 1, "candidate")
	var delta traceDBProfilerEventDelta
	delta.markOpaque(pairRenderF2FS)

	calibrationSink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	calibration := &profilerAuxCancelAtPollContext{Context: context.Background()}
	if err := calibrationSink.addProfilerEventContext(calibration, row, delta); err != nil {
		t.Fatal(err)
	}
	calibrationPolls := calibration.polls
	if cleanupErr := calibrationSink.cleanup(); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	if calibrationPolls < 2 {
		t.Fatalf("sink transaction exposes only %d cancellation polls", calibrationPolls)
	}

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= calibrationPolls; cancelAt++ {
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &profilerAuxCancelAtPollContext{
				Context: context.Background(), cancelAt: cancelAt, err: want,
			}
			err = sink.addProfilerEventContext(ctx, row, delta)
			if err != want {
				t.Fatalf("cancel=%d/%d error identity drifted: got=%T %v want=%T %v",
					cancelAt, calibrationPolls, err, err, want, want)
			}
			requireProfilerTypedEventTransactionEmpty(t, sink)
			if cleanupErr := sink.cleanup(); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
		}
	}
}

func TestProfilerTypedEventBatchStageCancellationIsProspectiveAtEveryPoll(t *testing.T) {
	unknownIssue, ok := profilerFtraceEventFixedIssue(987654, profilerFtraceEventIssueUnmappedField)
	if !ok {
		t.Fatal("unknown issue fixture rejected")
	}
	knownIssue, ok := profilerFtraceEventPayloadIssue(2003, profilerFtraceEventIssueCoreFieldWrongWire, 1)
	if !ok {
		t.Fatal("known issue fixture rejected")
	}
	fixtures := []struct {
		name        string
		field       int
		publishable bool
		issues      []profilerFtraceEventIssue
		emitted     bool
	}{
		{name: "known-issue", field: 2003, issues: []profilerFtraceEventIssue{knownIssue}},
		{name: "unknown-k8", field: 987654, issues: []profilerFtraceEventIssue{unknownIssue}},
		{name: "emitted-tail", field: 410, publishable: true, emitted: true},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			var calibrationBatch profilerFtraceEventBatchCensus
			calibration := &profilerAuxCancelAtPollContext{Context: context.Background()}
			delta, err := stageProfilerFtraceEventBatchDeltaContext(
				calibration, &calibrationBatch, fixture.field, fixture.publishable, fixture.issues, fixture.emitted)
			if err != nil || delta.batch != &calibrationBatch {
				t.Fatalf("stage calibration failed: polls=%d delta=%+v err=%v", calibration.polls, delta, err)
			}
			if calibrationBatch != (profilerFtraceEventBatchCensus{}) || calibration.polls < 2 {
				t.Fatalf("stage calibration mutated source or exposed too few polls: polls=%d batch=%+v",
					calibration.polls, calibrationBatch)
			}
			for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
				for cancelAt := 1; cancelAt <= calibration.polls; cancelAt++ {
					var batch profilerFtraceEventBatchCensus
					before := batch
					ctx := &profilerAuxCancelAtPollContext{
						Context: context.Background(), cancelAt: cancelAt, err: want,
					}
					got, err := stageProfilerFtraceEventBatchDeltaContext(
						ctx, &batch, fixture.field, fixture.publishable, fixture.issues, fixture.emitted)
					if err != want || got.batch != nil || ctx.polls != cancelAt {
						t.Fatalf("cancel=%d/%d stage identity/delta drifted: polls=%d delta=%+v err=%T %v",
							cancelAt, calibration.polls, ctx.polls, got, err, err)
					}
					if !reflect.DeepEqual(batch, before) {
						t.Fatalf("cancel=%d/%d partially committed batch:\n got=%+v\nwant=%+v",
							cancelAt, calibration.polls, batch, before)
					}
				}
			}
		})
	}
}

func TestProfilerTypedEventProductionAddCancellationLeavesStagedBatchAndPairUncommitted(t *testing.T) {
	fixture := profilerAuxF2FSEvent()
	result := profilerTypedEventContextResult(
		syntheticTracePluginFtraceEvent(1_000, 9, 9, "f2fs", fixture.Field, fixture.Payload),
	)
	calibrationSink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	var calibrationBatch profilerFtraceEventBatchCensus
	calibrationSeq := 0
	calibration := &profilerSummaryMetadataCancelContext{
		Context: context.Background(), targetSuffix: ".addContext",
	}
	if _, _, _, _, err := renderProfilerFtraceStructuredResultConsumerContext(
		calibration, result, &calibrationSeq, calibrationSink, false, &calibrationBatch, false); err != nil {
		t.Fatal(err)
	}
	calibrationPolls := calibration.polls
	if cleanupErr := calibrationSink.cleanup(); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	if calibrationPolls < 2 {
		t.Fatalf("production sink transaction exposes only %d polls", calibrationPolls)
	}

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= calibrationPolls; cancelAt++ {
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			var batch profilerFtraceEventBatchCensus
			ctx := &profilerSummaryMetadataCancelContext{
				Context: context.Background(), targetSuffix: ".addContext", cancelAt: cancelAt, err: want,
			}
			rows, coverage, summary, recognized, err := renderProfilerFtraceStructuredResultConsumerContext(
				ctx, result, &seq, sink, false, &batch, false)
			if err != want || ctx.polls != cancelAt || rows != 0 || seq != 0 || len(coverage) != 0 ||
				recognized || summary != (profilerFtraceSummary{}) {
				t.Fatalf("cancel=%d/%d production add result drifted: polls=%d rows=%d seq=%d coverage=%+v summary=%+v recognized=%t err=%T %v",
					cancelAt, calibrationPolls, ctx.polls, rows, seq, coverage, summary, recognized, err, err)
			}
			if batch != (profilerFtraceEventBatchCensus{}) {
				t.Fatalf("cancel=%d/%d committed staged batch: %+v", cancelAt, calibrationPolls, batch)
			}
			requireProfilerTypedEventTransactionEmpty(t, sink)
			if cleanupErr := sink.cleanup(); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
		}
	}
}

func seedProfilerTypedEventDeferredPrefix(t *testing.T, sink *traceDBRowSink) traceDBStoredRow {
	t.Helper()
	prefix := profilerTypedEventTransactionRow(1_000, 1, "completed-prefix")
	if err := sink.addProfilerEventContext(context.Background(), prefix, traceDBProfilerEventDelta{}); err != nil {
		t.Fatalf("seed completed prefix: %v", err)
	}
	if len(sink.rows) != 1 || sink.stats.RowsAccepted != 1 || sink.nextIngestOrdinal != 1 || len(sink.runs) != 0 {
		t.Fatalf("completed prefix was not deferred intact: rows=%d stats=%+v next=%d runs=%d",
			len(sink.rows), sink.stats, sink.nextIngestOrdinal, len(sink.runs))
	}
	return compactTraceDBStoredRow(prefix)
}

func assertProfilerTypedEventCurrentDeltaAbsent(t *testing.T, sink *traceDBRowSink) {
	t.Helper()
	if sink.opaque[pairRenderF2FS] || sink.poisoned[pairRenderF2FS] || sink.pairRows[pairRenderF2FS] != 0 ||
		sink.structuredPairRows[pairRenderF2FS] != 0 {
		t.Fatalf("current pair delta escaped cancellation: opaque=%t poisoned=%t pair_rows=%d structured=%d",
			sink.opaque[pairRenderF2FS], sink.poisoned[pairRenderF2FS],
			sink.pairRows[pairRenderF2FS], sink.structuredPairRows[pairRenderF2FS])
	}
}

func TestProfilerTypedEventNextEventPreflushCancellationKeepsOnlyCompletedPrefix(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= 3; cancelAt++ {
			t.Run(want.Error()+"/poll-"+strconv.Itoa(cancelAt), func(t *testing.T) {
				tempDir := t.TempDir()
				sink, err := newTraceDBRowSink(tempDir, 1)
				if err != nil {
					t.Fatal(err)
				}
				defer sink.cleanup()
				prefix := seedProfilerTypedEventDeferredPrefix(t, sink)
				var delta traceDBProfilerEventDelta
				delta.markOpaque(pairRenderF2FS)
				ctx := &profilerSummaryMetadataCancelContext{
					Context: context.Background(), targetSuffix: ".flushChunkContext", cancelAt: cancelAt, err: want,
				}
				err = sink.addProfilerEventContext(ctx,
					profilerTypedEventTransactionRow(2_000, 2, "current"), delta)
				if err != want || ctx.polls != cancelAt {
					t.Fatalf("preflush cancellation identity drifted: polls=%d cancel_at=%d err=%T %v want=%T %v",
						ctx.polls, cancelAt, err, err, want, want)
				}
				if len(sink.rows) != 1 || sink.rows[0] != prefix || sink.stats.RowsAccepted != 1 ||
					sink.nextIngestOrdinal != 1 || len(sink.runs) != 0 || sink.stats.SpillChunks != 0 ||
					sink.activeTempBytes != 0 || sink.liveTempBytes != 0 {
					t.Fatalf("preflush cancellation changed completed/current boundary: rows=%+v stats=%+v next=%d runs=%d active=%d live=%d",
						sink.rows, sink.stats, sink.nextIngestOrdinal, len(sink.runs), sink.activeTempBytes, sink.liveTempBytes)
				}
				assertProfilerTypedEventCurrentDeltaAbsent(t, sink)
				entries, readErr := os.ReadDir(tempDir)
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("canceled spill retained temp files: entries=%v err=%v", entries, readErr)
				}
			})
		}
	}
}

func TestProfilerTypedEventCompletedPrefixMaySpillBeforeCurrentValidationCancellation(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seedProfilerTypedEventDeferredPrefix(t, sink)
			var delta traceDBProfilerEventDelta
			delta.markOpaque(pairRenderF2FS)
			ctx := &profilerSummaryMetadataCancelContext{
				Context: context.Background(), targetSuffix: ".profilerSinglePhysicalLineStringContext",
				cancelAt: 1, err: want,
			}
			err = sink.addProfilerEventContext(ctx,
				profilerTypedEventTransactionRow(2_000, 2, "current"), delta)
			if err != want || ctx.polls != 1 {
				t.Fatalf("current validation cancellation identity drifted: polls=%d err=%T %v", ctx.polls, err, err)
			}
			if len(sink.rows) != 0 || sink.stats.RowsAccepted != 1 || sink.nextIngestOrdinal != 1 ||
				len(sink.runs) != 1 || sink.stats.SpillChunks != 1 {
				t.Fatalf("completed-prefix spill/current rollback drifted: rows=%d stats=%+v next=%d runs=%d",
					len(sink.rows), sink.stats, sink.nextIngestOrdinal, len(sink.runs))
			}
			assertProfilerTypedEventCurrentDeltaAbsent(t, sink)
		})
	}
}

func TestProfilerTypedEventTailFlushCancellationDoesNotPublishManifest(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= 3; cancelAt++ {
			t.Run(want.Error()+"/poll-"+strconv.Itoa(cancelAt), func(t *testing.T) {
				tempDir := t.TempDir()
				sink, err := newTraceDBRowSink(tempDir, 1)
				if err != nil {
					t.Fatal(err)
				}
				defer sink.cleanup()
				prefix := seedProfilerTypedEventDeferredPrefix(t, sink)
				ctx := &profilerSummaryMetadataCancelContext{
					Context: context.Background(), targetSuffix: ".flushChunkContext", cancelAt: cancelAt, err: want,
				}
				err = sink.flushTriggeredProfilerEventContext(ctx)
				if err != want || ctx.polls != cancelAt {
					t.Fatalf("tail cancellation identity drifted: polls=%d cancel_at=%d err=%T %v",
						ctx.polls, cancelAt, err, err)
				}
				if len(sink.rows) != 1 || sink.rows[0] != prefix || len(sink.runs) != 0 ||
					sink.stats.RowsAccepted != 1 || sink.stats.SpillChunks != 0 ||
					sink.activeTempBytes != 0 || sink.liveTempBytes != 0 {
					t.Fatalf("canceled tail flush published or lost prefix: rows=%+v runs=%d stats=%+v active=%d live=%d",
						sink.rows, len(sink.runs), sink.stats, sink.activeTempBytes, sink.liveTempBytes)
				}
				entries, readErr := os.ReadDir(tempDir)
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("canceled tail flush retained temp files: entries=%v err=%v", entries, readErr)
				}
			})
		}
	}
}

func TestProfilerTypedEventBelowThresholdFlushStillHonorsCancellation(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		sink, err := newTraceDBRowSink(t.TempDir(), 128)
		if err != nil {
			t.Fatal(err)
		}
		ctx := &profilerAuxCancelAtPollContext{
			Context: context.Background(), cancelAt: 1, err: want,
		}
		err = sink.flushTriggeredProfilerEventContext(ctx)
		if err != want || ctx.polls != 1 {
			t.Fatalf("below-threshold flush swallowed cancellation: polls=%d err=%T %v want=%T %v",
				ctx.polls, err, err, want, want)
		}
		requireProfilerTypedEventTransactionEmpty(t, sink)
		if cleanupErr := sink.cleanup(); cleanupErr != nil {
			t.Fatal(cleanupErr)
		}
	}
}

func TestProfilerTypedEventNoRowDeltaFinalCancellationIsProspective(t *testing.T) {
	result := profilerTypedEventContextResult(
		syntheticTracePluginFtraceEvent(1_000, 7, 7, "f2fs", 4009, []byte{0x08}),
	)
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= 2; cancelAt++ {
			t.Run(want.Error()+"/poll-"+strconv.Itoa(cancelAt), func(t *testing.T) {
				sink, err := newTraceDBRowSink(t.TempDir(), 128)
				if err != nil {
					t.Fatal(err)
				}
				defer sink.cleanup()
				seq := 0
				var batch profilerFtraceEventBatchCensus
				ctx := &profilerSummaryMetadataCancelContext{
					Context: context.Background(), targetSuffix: ".commitProfilerEventDeltaContext",
					cancelAt: cancelAt, err: want,
				}
				rows, coverage, summary, recognized, err := renderProfilerFtraceStructuredResultConsumerContext(
					ctx, result, &seq, sink, false, &batch, false)
				if err != want || ctx.polls != cancelAt || rows != 0 || seq != 0 || len(coverage) != 0 || recognized ||
					summary != (profilerFtraceSummary{}) {
					t.Fatalf("no-row cancellation result drifted: polls=%d cancel_at=%d rows=%d seq=%d coverage=%+v summary=%+v recognized=%t err=%T %v",
						ctx.polls, cancelAt, rows, seq, coverage, summary, recognized, err, err)
				}
				if batch != (profilerFtraceEventBatchCensus{}) {
					t.Fatalf("no-row cancellation committed batch census: %+v", batch)
				}
				requireProfilerTypedEventTransactionEmpty(t, sink)
			})
		}
	}
}

func TestProfilerTypedEventBatchOverflowDoesNotCommitSinkOrPartialBatch(t *testing.T) {
	fixture := profilerAuxF2FSEvent()
	result := profilerTypedEventContextResult(
		syntheticTracePluginFtraceEvent(1_000, 9, 9, "f2fs", fixture.Field, fixture.Payload),
	)
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	var batch profilerFtraceEventBatchCensus
	batch.Slots[profilerFtraceEventSlot(fixture.Field)].RowsRead = math.MaxUint64
	before := batch
	rows, _, _, _, err := renderProfilerFtraceStructuredResultConsumerContext(
		context.Background(), result, &seq, sink, false, &batch, false)
	reason, invariant := traceDBOutputInvariantReason(err)
	if !invariant || reason != "profiler_event_batch_counter_overflow" || rows != 0 || seq != 0 {
		t.Fatalf("batch overflow result drifted: reason=%q invariant=%t rows=%d seq=%d err=%T %v",
			reason, invariant, rows, seq, err, err)
	}
	if !reflect.DeepEqual(batch, before) {
		t.Fatalf("batch overflow partially committed slot/sample state:\n got=%+v\nwant=%+v", batch, before)
	}
	requireProfilerTypedEventTransactionEmpty(t, sink)
}

type profilerTypedEventPairParity struct {
	rows                int
	seq                 int
	batch               profilerFtraceEventBatchCensus
	pairRows            int
	structuredPairRows  int
	pairTableRows       int
	pairLaneRows        int
	structuredEventRows [2]int
	proofObservations   int64
	proofLaneKeys       int64
	poisoned            bool
	output              []byte
	ingestSpillChunks   int
	finalRowsWritten    int
	finalRowsWithheld   int
}

func runProfilerTypedEventPairParity(t *testing.T, result profilerTracePluginResult, threshold int) profilerTypedEventPairParity {
	t.Helper()
	sink, err := newTraceDBRowSink(t.TempDir(), threshold)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	var batch profilerFtraceEventBatchCensus
	rows, coverage, _, recognized, err := renderProfilerFtraceStructuredResultConsumerContext(
		context.Background(), result, &seq, sink, false, &batch, false)
	if err != nil || len(coverage) != 0 || recognized {
		t.Fatalf("pair parity ingest failed: rows=%d coverage=%+v recognized=%t err=%v", rows, coverage, recognized, err)
	}
	parity := profilerTypedEventPairParity{
		rows: rows, seq: seq, batch: batch,
		pairRows: sink.pairRows[pairRenderF2FS], structuredPairRows: sink.structuredPairRows[pairRenderF2FS],
		structuredEventRows: [2]int{
			sink.structuredEventRows[pairRenderF2FS][4009], sink.structuredEventRows[pairRenderF2FS][4010],
		},
		proofObservations: sink.legacyPairProof.observations,
		proofLaneKeys:     sink.legacyPairProof.laneKeys,
		poisoned:          sink.poisoned[pairRenderF2FS], ingestSpillChunks: sink.stats.SpillChunks,
	}
	for _, count := range sink.pairLaneRows[pairRenderF2FS] {
		parity.pairLaneRows += count
	}
	for _, lanes := range sink.pairTableRows[pairRenderF2FS] {
		for _, count := range lanes {
			parity.pairTableRows += count
		}
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatalf("pair parity publish: %v", err)
	}
	parity.output = append([]byte(nil), output.Bytes()...)
	parity.finalRowsWritten = stats.RowsWritten
	parity.finalRowsWithheld = stats.RowsWithheld
	return parity
}

func TestProfilerTypedEventThresholdOnePreservesPairBatchAndOutputParity(t *testing.T) {
	base := profilerAuxCasesByField()
	result := profilerTypedEventContextResult(
		syntheticTracePluginFtraceEvent(1_000, 40, 40, "f2fs", 4009, profilerAuxEncodeValues(base[4009].values)),
		syntheticTracePluginFtraceEvent(2_000, 40, 40, "f2fs", 4010, profilerAuxEncodeValues(base[4010].values)),
	)
	high := runProfilerTypedEventPairParity(t, result, 128)
	tiny := runProfilerTypedEventPairParity(t, result, 1)
	if high.rows != 2 || high.seq != 2 || high.pairRows != 2 || high.structuredPairRows != 2 ||
		high.pairTableRows != 2 || high.pairLaneRows != 2 || high.structuredEventRows != [2]int{1, 1} ||
		high.poisoned || high.finalRowsWritten != 2 || high.finalRowsWithheld != 0 {
		t.Fatalf("high-threshold pair fixture drifted: %+v", high)
	}
	highSpills, tinySpills := high.ingestSpillChunks, tiny.ingestSpillChunks
	high.ingestSpillChunks, tiny.ingestSpillChunks = 0, 0
	if !reflect.DeepEqual(tiny, high) {
		t.Fatalf("threshold=1 changed pair/batch/output transaction:\n high=%+v\n tiny=%+v", high, tiny)
	}
	if highSpills != 0 || tinySpills == 0 {
		t.Fatalf("parity lanes did not exercise distinct spill paths: high=%d tiny=%d", highSpills, tinySpills)
	}
}
