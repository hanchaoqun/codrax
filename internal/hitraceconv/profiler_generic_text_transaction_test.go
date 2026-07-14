package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// profilerGenericTransactionContext counts only polls made while the generic
// Session/bytrace/other-text scanner (or a selected sink transaction) is on the
// stack. That keeps the pin independent of unrelated container/protobuf polls.
type profilerGenericTransactionContext struct {
	context.Context
	targetContains string
	cancelAt       int
	polls          int
	err            error
}

func (ctx *profilerGenericTransactionContext) Err() error {
	if ctx == nil {
		return context.Canceled
	}
	var callers [64]uintptr
	frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers[:])])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ctx.targetContains) {
			ctx.polls++
			if ctx.err != nil && ctx.cancelAt > 0 && ctx.polls >= ctx.cancelAt {
				return ctx.err
			}
			break
		}
		if !more {
			break
		}
	}
	if ctx.Context != nil {
		return ctx.Context.Err()
	}
	return nil
}

type profilerGenericPublisherCase struct {
	name      string
	publisher profilerPairPublisherSlot
	text      bool
}

func profilerGenericPublisherCases() []profilerGenericPublisherCase {
	return []profilerGenericPublisherCase{
		{name: "bytrace", publisher: profilerPairPublisherBytrace, text: true},
		{name: "other", publisher: profilerPairPublisherOtherText, text: true},
		{name: "session", publisher: profilerPairPublisherSession},
	}
}

func beginProfilerGenericPublisher(t testing.TB, sink *traceDBRowSink, publisher profilerGenericPublisherCase) {
	t.Helper()
	if !sink.beginPairRowCensusForPublisher(publisher.publisher) {
		t.Fatalf("begin %s publisher census", publisher.name)
	}
	if publisher.text && !sink.beginProfilerTextMessage() {
		t.Fatalf("begin %s text message", publisher.name)
	}
}

func assertProfilerGenericCurrentMutationAbsent(t testing.TB, sink *traceDBRowSink,
	publisher profilerGenericPublisherCase,
) {
	t.Helper()
	if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.rowIngestOrdinals) != 0 ||
		len(sink.runs) != 0 || sink.nextIngestOrdinal != 0 || sink.bufferedBytes != 0 ||
		len(sink.pairLaneRegistries[pairRenderF2FS].byKey) != 0 ||
		len(sink.pairLaneRegistries[pairRenderF2FS].keys) != 0 ||
		len(sink.pairLaneRegistries[pairRenderF2FS].states) != 0 ||
		sink.activePairCensus[pairRenderF2FS].total != 0 ||
		len(sink.activePairCensus[pairRenderF2FS].byLane) != 0 {
		t.Fatalf("%s cancellation committed current row/delta/registry/census: stats=%+v rows=%d runs=%d next=%d buffered=%d registry=%+v census=%+v",
			publisher.name, sink.stats, len(sink.rows), len(sink.runs), sink.nextIngestOrdinal,
			sink.bufferedBytes, sink.pairLaneRegistries[pairRenderF2FS],
			sink.activePairCensus[pairRenderF2FS])
	}
	if !sink.pairCensusActive || sink.activePairPublisher != publisher.publisher ||
		sink.textMessageActive != publisher.text || sink.activeTextRows != 0 || sink.nextTextMessage != 0 ||
		publisher.text && sink.activeTextMessage != 1 || !publisher.text && sink.activeTextMessage != 0 {
		t.Fatalf("%s cancellation drifted pre-existing publisher/message context: census=%t publisher=%d text=%t active=%d rows=%d next=%d",
			publisher.name, sink.pairCensusActive, sink.activePairPublisher, sink.textMessageActive,
			sink.activeTextMessage, sink.activeTextRows, sink.nextTextMessage)
	}
}

func TestProfilerGenericTextTailSpillFailureKeepsCurrentTransactionWhole(t *testing.T) {
	_, rejectedLane, _ := profilerStrictTextF2FSFixture(t)
	admission := profilerTextPairAdmission(rejectedLane)
	if !admission.Governed || admission.Admitted || !admission.LaneKnown || admission.Lane == "" ||
		admission.Kind != pairRenderF2FS || admission.EndpointSlot != profilerPairEndpointF2FSWriteEnd {
		t.Fatalf("generic transaction fixture is not an exact rejected F2FS lane: %+v", admission)
	}

	for _, publisher := range profilerGenericPublisherCases() {
		for _, point := range []string{"create", "write", "stat"} {
			t.Run(publisher.name+"/"+point, func(t *testing.T) {
				want := errors.New("generic-" + publisher.name + "-" + point + "-sentinel")
				sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 1, traceDBRowSinkOptions{
					bufferBytes: 1 << 20,
					ops: traceDBRowSinkOps{fault: func(got, _ string) error {
						if got == point {
							return want
						}
						return nil
					}},
				})
				if err != nil {
					t.Fatal(err)
				}
				defer sink.cleanup()
				defer sink.abortPairRowCensus()
				beginProfilerGenericPublisher(t, sink, publisher)

				seq := 23
				rows, rowErr := addSystraceRowsFromBytes([]byte(rejectedLane+"\n"), &seq, sink)
				registry := &sink.pairLaneRegistries[pairRenderF2FS]
				laneID, laneFound := registry.idFor(admission.Lane)
				state, stateFound := registry.state(laneID)
				var row traceDBStoredRow
				if len(sink.rows) == 1 {
					row = sink.rows[0]
				}
				provenance := row.profilerProvenance()
				wantFlags := profilerPairRowProvenanceFlags(0)
				wantOrdinal := uint32(0)
				wantTextActive := false
				wantActiveMessage := uint32(0)
				wantActiveRows := 0
				if publisher.text {
					wantFlags = profilerPairRowProvenanceText
					wantOrdinal = 1
					wantTextActive = true
					wantActiveMessage = 1
					wantActiveRows = 1
				}
				census := sink.activePairCensus[pairRenderF2FS]
				if !errors.Is(rowErr, want) || rows != 1 || seq != 24 ||
					sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 || len(sink.runs) != 0 ||
					sink.nextIngestOrdinal != 1 || row.seq != 23 || provenance.PairKind != pairRenderF2FS ||
					provenance.EndpointSlot != admission.EndpointSlot ||
					provenance.PublisherSlot != publisher.publisher || provenance.Flags != wantFlags ||
					provenance.TextMessageOrdinal != wantOrdinal || !laneFound || !stateFound || state == nil ||
					!state.poisoned || laneID == 0 || provenance.LaneID != laneID ||
					census.total != 1 || census.byLane[admission.Lane] != 1 ||
					!sink.pairCensusActive || sink.activePairPublisher != publisher.publisher ||
					sink.textMessageActive != wantTextActive || sink.activeTextMessage != wantActiveMessage ||
					sink.activeTextRows != wantActiveRows || sink.nextTextMessage != 0 {
					t.Fatalf("%s tail %s split current transaction: err=%T %v rows=%d seq=%d stats=%+v buffered_rows=%d runs=%d next=%d row=%+v provenance=%+v lane=(%d,%t,%t,%+v) census=%+v text=%t/%d/%d next_text=%d",
						publisher.name, point, rowErr, rowErr, rows, seq, sink.stats, len(sink.rows),
						len(sink.runs), sink.nextIngestOrdinal, row, provenance, laneID, laneFound, stateFound,
						state, census, sink.textMessageActive,
						sink.activeTextMessage, sink.activeTextRows, sink.nextTextMessage)
				}
			})
		}
	}
}

func TestProfilerGenericTextCancellationLeavesCurrentTransactionUncommitted(t *testing.T) {
	_, rejectedLane, _ := profilerStrictTextF2FSFixture(t)
	admission := profilerTextPairAdmission(rejectedLane)
	if !admission.Governed || admission.Admitted || !admission.LaneKnown || admission.Lane == "" {
		t.Fatalf("generic cancellation fixture drifted: %+v", admission)
	}

	for _, publisher := range profilerGenericPublisherCases() {
		t.Run(publisher.name, func(t *testing.T) {
			calibrationSink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			beginProfilerGenericPublisher(t, calibrationSink, publisher)
			calibration := &profilerGenericTransactionContext{
				Context: context.Background(), targetContains: ".addContext",
			}
			if err := calibrationSink.bindContext(calibration); err != nil {
				t.Fatal(err)
			}
			calibrationSeq := 0
			calibrationRows, calibrationErr := addSystraceRowsFromBytesContext(
				calibration, []byte(rejectedLane+"\n"), &calibrationSeq, calibrationSink)
			calibrationPolls := calibration.polls
			calibrationSink.abortPairRowCensus()
			if cleanupErr := calibrationSink.cleanup(); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
			if calibrationErr != nil || calibrationRows != 1 || calibrationSeq != 1 || calibrationPolls < 3 {
				t.Fatalf("calibrate %s generic row transaction: rows=%d seq=%d polls=%d err=%v",
					publisher.name, calibrationRows, calibrationSeq, calibrationPolls, calibrationErr)
			}

			points := []int{1, (calibrationPolls + 1) / 2, calibrationPolls}
			for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
				seen := map[int]bool{}
				for _, cancelAt := range points {
					if seen[cancelAt] {
						continue
					}
					seen[cancelAt] = true
					t.Run(want.Error()+"/poll-"+strconv.Itoa(cancelAt), func(t *testing.T) {
						sink, err := newTraceDBRowSink(t.TempDir(), 128)
						if err != nil {
							t.Fatal(err)
						}
						defer sink.cleanup()
						defer sink.abortPairRowCensus()
						beginProfilerGenericPublisher(t, sink, publisher)
						ctx := &profilerGenericTransactionContext{
							Context: context.Background(), targetContains: ".addContext",
							cancelAt: cancelAt, err: want,
						}
						if err := sink.bindContext(ctx); err != nil {
							t.Fatal(err)
						}
						seq := 41
						rows, rowErr := addSystraceRowsFromBytesContext(ctx, []byte(rejectedLane+"\n"), &seq, sink)
						if !errors.Is(rowErr, want) || ctx.polls != cancelAt || rows != 0 || seq != 41 {
							t.Fatalf("%s cancel=%d/%d result drifted: polls=%d rows=%d seq=%d err=%T %v want=%v",
								publisher.name, cancelAt, calibrationPolls, ctx.polls, rows, seq, rowErr, rowErr, want)
						}
						assertProfilerGenericCurrentMutationAbsent(t, sink, publisher)
					})
				}
			}
		})
	}
}

type profilerGenericProductionRoute struct {
	name       string
	pluginName string
	publisher  profilerPairPublisherSlot
	session    bool
}

func extractProfilerGenericProductionRoute(t testing.TB, route profilerGenericProductionRoute,
	payload []byte, ctx context.Context,
) (profilerContainerExtraction, *traceDBRowSink, error) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, route.name+".htrace")
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData(route.pluginName, payload))
	if route.session {
		body = []byte(profilerSessionJSONTag + "\n" + string(payload) + "\n")
	}
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.bindContext(ctx); err != nil {
		sink.cleanup()
		t.Fatal(err)
	}
	extracted, extractErr := extractProfilerContainerSystraceRows(ctx, input, int64(len(body)), sink)
	return extracted, sink, extractErr
}

func assertProfilerGenericProductionSinkPristine(t testing.TB, sink *traceDBRowSink) {
	t.Helper()
	if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.runs) != 0 ||
		sink.nextIngestOrdinal != 0 || sink.bufferedBytes != 0 || sink.pairCensusActive ||
		sink.activePairPublisher != profilerPairPublisherNone || sink.textMessageActive ||
		sink.activeTextMessage != 0 || sink.activeTextRows != 0 || sink.nextTextMessage != 0 {
		t.Fatalf("no-row production route mutated sink: stats=%+v rows=%d runs=%d next=%d buffered=%d census=%t publisher=%d text=%t/%d/%d next_text=%d",
			sink.stats, len(sink.rows), len(sink.runs), sink.nextIngestOrdinal, sink.bufferedBytes,
			sink.pairCensusActive, sink.activePairPublisher, sink.textMessageActive,
			sink.activeTextMessage, sink.activeTextRows, sink.nextTextMessage)
	}
	for _, kind := range profilerCaptureKinds {
		if len(sink.pairLaneRegistries[kind].byKey) != 0 || len(sink.pairLaneRegistries[kind].keys) != 0 ||
			len(sink.pairLaneRegistries[kind].states) != 0 {
			t.Fatalf("no-row production route mutated pair kind %d registry=%+v",
				kind, sink.pairLaneRegistries[kind])
		}
	}
}

func TestProfilerGenericTextNoRowPayloadHasBoundedContextCheckpoints(t *testing.T) {
	longBytes := 2*profilerContextByteCheckpointBytes + 17
	routes := []struct {
		route   profilerGenericProductionRoute
		payload []byte
	}{
		{
			route:   profilerGenericProductionRoute{name: "bytrace-large-comment", pluginName: "bytrace_plugin", publisher: profilerPairPublisherBytrace},
			payload: append(append([]byte("# "), bytes.Repeat([]byte{'x'}, longBytes)...), '\n'),
		},
		{
			route:   profilerGenericProductionRoute{name: "other-large-blank", pluginName: "customer_plugin", publisher: profilerPairPublisherOtherText},
			payload: append(bytes.Repeat([]byte{' '}, longBytes), '\n'),
		},
		{
			route:   profilerGenericProductionRoute{name: "session-large-malformed", publisher: profilerPairPublisherSession, session: true},
			payload: append(bytes.Repeat([]byte{'x'}, longBytes), '\n'),
		},
	}

	for _, test := range routes {
		t.Run(test.route.name, func(t *testing.T) {
			calibration := &profilerGenericTransactionContext{
				Context: context.Background(), targetContains: "addSystraceRowsFromBytes",
			}
			extracted, sink, err := extractProfilerGenericProductionRoute(t, test.route, test.payload, calibration)
			if err != nil {
				sink.cleanup()
				t.Fatalf("calibrate no-row route: %v", err)
			}
			assertProfilerGenericProductionSinkPristine(t, sink)
			if cleanupErr := sink.cleanup(); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
			if extracted.TextRows != 0 || calibration.polls < 4 {
				t.Fatalf("no-row route lacked bounded generic checkpoints: extracted=%+v polls=%d want>=4 payload_bytes=%d",
					extracted, calibration.polls, len(test.payload))
			}

			points := []int{1, (calibration.polls + 1) / 2, calibration.polls}
			for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
				seen := map[int]bool{}
				for _, cancelAt := range points {
					if seen[cancelAt] {
						continue
					}
					seen[cancelAt] = true
					ctx := &profilerGenericTransactionContext{
						Context: context.Background(), targetContains: "addSystraceRowsFromBytes",
						cancelAt: cancelAt, err: want,
					}
					_, sink, rowErr := extractProfilerGenericProductionRoute(t, test.route, test.payload, ctx)
					if !errors.Is(rowErr, want) || ctx.polls != cancelAt {
						sink.cleanup()
						t.Fatalf("no-row cancel=%d/%d polls=%d err=%T %v want=%v",
							cancelAt, calibration.polls, ctx.polls, rowErr, rowErr, want)
					}
					assertProfilerGenericProductionSinkPristine(t, sink)
					if cleanupErr := sink.cleanup(); cleanupErr != nil {
						t.Fatal(cleanupErr)
					}
				}
			}
		})
	}
}

func TestProfilerGenericTextTransactionProductionPublisherRoutes(t *testing.T) {
	line := traceDBFormatLine("worker", 7, 7, 1, 1_000_000_000, 0, 0, "print: B|7|Generic")
	routes := []profilerGenericProductionRoute{
		{name: "bytrace", pluginName: "bytrace_plugin", publisher: profilerPairPublisherBytrace},
		{name: "other", pluginName: "customer_plugin", publisher: profilerPairPublisherOtherText},
		{name: "session", publisher: profilerPairPublisherSession, session: true},
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			extracted, sink, err := extractProfilerGenericProductionRoute(
				t, route, []byte(line+"\n"), context.Background())
			if err != nil {
				sink.cleanup()
				t.Fatal(err)
			}
			defer sink.cleanup()
			if extracted.TextRows != 1 || sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 ||
				sink.pairCensusActive || sink.activePairPublisher != profilerPairPublisherNone ||
				sink.textMessageActive || sink.activeTextMessage != 0 || sink.activeTextRows != 0 {
				t.Fatalf("%s production route ledger drifted: extracted=%+v stats=%+v rows=%+v census=%t publisher=%d text=%t/%d/%d",
					route.name, extracted, sink.stats, sink.rows, sink.pairCensusActive,
					sink.activePairPublisher, sink.textMessageActive, sink.activeTextMessage, sink.activeTextRows)
			}
			provenance := sink.rows[0].profilerProvenance()
			wantFlags := profilerPairRowProvenanceFlags(0)
			wantOrdinal := uint32(0)
			wantNext := uint32(0)
			if !route.session {
				wantFlags = profilerPairRowProvenanceText
				wantOrdinal = 1
				wantNext = 1
			}
			if !provenance.valid() || provenance.PublisherSlot != route.publisher ||
				provenance.Flags != wantFlags || provenance.TextMessageOrdinal != wantOrdinal ||
				provenance.PairKind != pairRenderUnknown || provenance.EndpointSlot != profilerPairEndpointNone ||
				sink.nextTextMessage != wantNext {
				t.Fatalf("%s production route provenance=%+v next=%d want publisher=%d flags=%d ordinal=%d next=%d",
					route.name, provenance, sink.nextTextMessage, route.publisher, wantFlags, wantOrdinal, wantNext)
			}
		})
	}
}
