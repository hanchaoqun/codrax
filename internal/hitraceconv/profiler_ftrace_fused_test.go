package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type profilerFusedLaneResult struct {
	Summary    profilerFtraceSummary
	Recognized bool
	Rows       int
	Seq        int
	Published  int
	Batch      profilerFtraceEventBatchCensus
	PairState  profilerFusedPairState
	Output     string
}

type profilerFusedPairState struct {
	Poisoned           [pairRenderKindCount]bool
	Opaque             [pairRenderKindCount]bool
	Withheld           [pairRenderKindCount]int
	StructuredWithheld [pairRenderKindCount]int
}

func runProfilerFusedLane(t *testing.T, result profilerTracePluginResult, summarize bool) profilerFusedLaneResult {
	t.Helper()
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	var batch profilerFtraceEventBatchCensus
	seq := 0
	rows, coverage, summary, recognized, err := renderProfilerFtraceStructuredResultConsumerContext(
		context.Background(), result, &seq, sink, false, &batch, summarize)
	if err != nil {
		t.Fatalf("run fused lane summarize=%t: %v", summarize, err)
	}
	if len(coverage) != 0 {
		t.Fatalf("container-style fused lane unexpectedly materialized direct coverage: %+v", coverage)
	}
	published := sink.publishableRows()
	var output bytes.Buffer
	if _, err := sink.prepareAndWriteForTest(context.Background(), &output); err != nil {
		t.Fatalf("publish fused lane summarize=%t: %v", summarize, err)
	}
	var pairState profilerFusedPairState
	for kind := pairRenderKind(0); kind < pairRenderKindCount; kind++ {
		pairState.Poisoned[kind] = sink.pairKindPoisoned(kind)
		pairState.Opaque[kind] = sink.opaque[kind]
		pairState.Withheld[kind] = sink.withheldPairRowsForKind(kind)
		pairState.StructuredWithheld[kind] = sink.withheldStructuredPairRowsForKind(kind)
	}
	return profilerFusedLaneResult{
		Summary: summary, Recognized: recognized, Rows: rows, Seq: seq, Published: published,
		Batch: batch, PairState: pairState, Output: output.String(),
	}
}

func assertProfilerFusedThreeLaneParity(t *testing.T, result profilerTracePluginResult) (profilerFusedLaneResult, profilerFusedLaneResult) {
	t.Helper()
	wantSummary, wantRecognized, err := decodeProfilerFtraceSummaryResultContext(context.Background(), result)
	if err != nil {
		t.Fatalf("summary-only lane: %v", err)
	}
	renderOnly := runProfilerFusedLane(t, result, false)
	fused := runProfilerFusedLane(t, result, true)
	if fused.Recognized != wantRecognized || !reflect.DeepEqual(fused.Summary, wantSummary) {
		t.Fatalf("fused summary lane drifted:\nwant_recognized=%t got_recognized=%t\nwant=%+v\ngot=%+v",
			wantRecognized, fused.Recognized, wantSummary, fused.Summary)
	}
	if renderOnly.Recognized || renderOnly.Rows != fused.Rows || renderOnly.Seq != fused.Seq ||
		renderOnly.Published != fused.Published || renderOnly.Output != fused.Output ||
		!reflect.DeepEqual(renderOnly.Batch, fused.Batch) || !reflect.DeepEqual(renderOnly.PairState, fused.PairState) {
		t.Fatalf("fused renderer lane drifted:\nrender=%+v\nfused=%+v", renderOnly, fused)
	}
	return renderOnly, fused
}

func profilerFusedMixedResult() profilerTracePluginResult {
	eventA := syntheticTracePluginFtraceEvent(1_000, 7, 7, "worker", 1109, protoBytes(2, []byte("B|7|A")))
	eventB := syntheticTracePluginFtraceEvent(2_000, 7, 7, "worker", 1109, protoBytes(2, []byte("E|7")))
	eventC := syntheticTracePluginFtraceEvent(3_000, 8, 8, "helper", 1109, protoBytes(2, []byte("I|8|C")))
	return decodeProfilerTracePluginResult(protoPayload(
		protoMessage(1,
			protoVarint(1, 1),
			protoMessage(2, protoVarint(1, 2), protoVarint(2, 11)),
			protoBytes(3, []byte("boot")),
		),
		protoMessage(2, protoVarint(1, 2), eventA, eventB, protoVarint(3, 1)),
		protoMessage(5, protoVarint(1, 0x1234), protoBytes(2, []byte("schedule"))),
		protoMessage(2, protoVarint(1, 3), eventC),
		protoMessage(6, protoVarint(1, 1), protoMessage(2, protoVarint(1, 10))),
		protoBytes(7, []byte("trace-plugin-v1")),
		protoMessage(8, protoVarint(1, 7), protoBytes(2, []byte("worker"))),
	))
}

func TestProfilerFtraceFusedSummaryAndRendererParity(t *testing.T) {
	result := profilerFusedMixedResult()
	_, fused := assertProfilerFusedThreeLaneParity(t, result)
	if fused.Rows != 3 || fused.Seq != 3 || fused.Summary.DetailMessages != 2 || fused.Summary.DetailEventCount != 3 {
		t.Fatalf("mixed fused fixture census drifted: %+v", fused)
	}
}

func TestProfilerFtraceFusedDamagedEnvelopeThreeLaneParity(t *testing.T) {
	event := syntheticTracePluginFtraceEvent(1_000, 7, 7, "worker", 1109, protoBytes(2, []byte("I|7|event")))
	validTop := protoMessage(2, protoVarint(1, 2), event)
	malformedDetail := append(protoPayload(protoVarint(1, 2), event), 0x80)
	tests := []struct {
		name             string
		payload          []byte
		wantRecognized   bool
		wantDetails      uint64
		wantDetailEvents uint64
		wantRows         int
		wantDegraded     bool
		wantOverwriteOK  bool
	}{
		{
			name: "cpu wrong wire", payload: protoMessage(2, protoBytes(1, nil), event),
			wantRecognized: true, wantDegraded: true, wantOverwriteOK: true,
		},
		{
			name: "cpu out of range", payload: protoMessage(2, protoVarint(1, uint64(maxTraceDBCPUIndex)+1), event),
			wantRecognized: true, wantDegraded: true, wantOverwriteOK: true,
		},
		{
			name: "cpu detail malformed tail", payload: protoBytes(2, malformedDetail),
			wantRecognized: true, wantDegraded: true, wantOverwriteOK: true,
		},
		{
			name:           "event container wrong wire with legal sibling",
			payload:        protoMessage(2, protoVarint(1, 2), protoVarint(2, 1), event),
			wantRecognized: true, wantDetails: 1, wantDetailEvents: 1, wantRows: 1, wantDegraded: true, wantOverwriteOK: true,
		},
		{
			name: "overwrite duplicate", payload: protoMessage(2,
				protoVarint(1, 2), event, protoVarint(3, 1), protoVarint(3, 2)),
			wantRecognized: true, wantDetails: 1, wantDetailEvents: 1, wantRows: 1, wantDegraded: true,
		},
		{
			name: "top malformed tail", payload: append(append([]byte(nil), validTop...), 0x80),
			wantDegraded: true, wantOverwriteOK: true,
		},
		{
			name: "top wrong wire with legal sibling", payload: protoPayload(protoVarint(2, 1), validTop),
			wantRecognized: true, wantDetails: 1, wantDetailEvents: 1, wantRows: 1, wantOverwriteOK: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, fused := assertProfilerFusedThreeLaneParity(t, decodeProfilerTracePluginResult(test.payload))
			if fused.Recognized != test.wantRecognized || fused.Summary.DetailMessages != test.wantDetails ||
				fused.Summary.DetailEventCount != test.wantDetailEvents || fused.Rows != test.wantRows ||
				fused.Batch.degraded() != test.wantDegraded || fused.Summary.DetailOverwriteOK != test.wantOverwriteOK {
				t.Fatalf("damaged three-lane fixture verdict drifted: got=%+v degraded=%t", fused, fused.Batch.degraded())
			}
		})
	}
}

func profilerFusedMalformedDetailWithEvent(event []byte) []byte {
	detail := append(protoPayload(protoVarint(1, 2), event), 0x80)
	return protoBytes(2, detail)
}

func TestProfilerFtraceFusedPairBarrierThreeLaneParity(t *testing.T) {
	cases := profilerAuxCasesByField()
	tests := []struct {
		name    string
		kind    pairRenderKind
		payload []byte
	}{
		{
			name: "f2fs", kind: pairRenderF2FS,
			payload: protoPayload(
				profilerF2FSTestStructuredMessage(4011, cases[4011].values, 1_000),
				profilerFusedMalformedDetailWithEvent(syntheticTracePluginFtraceEvent(
					2_000, 40, 40, "f2fs", 4012, profilerAuxEncodeValues(cases[4012].values))),
			),
		},
		{
			name: "mmc", kind: pairRenderMMC,
			payload: protoPayload(
				profilerMMCTestStructuredMessage(4016, cases[4016].values, 1_000),
				profilerFusedMalformedDetailWithEvent(syntheticTracePluginFtraceEvent(
					2_000, 40, 40, "mmc", 4015, profilerAuxEncodeValues(cases[4015].values))),
			),
		},
		{
			name: "block", kind: pairRenderBlock,
			payload: protoPayload(
				profilerBlockTestStructuredMessage(211, profilerBlockTypedPayload(211, nil), 1_000, 40),
				profilerFusedMalformedDetailWithEvent(syntheticTracePluginFtraceEvent(
					2_000, 41, 41, "block", 209, profilerBlockTypedPayload(209, nil))),
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, fused := assertProfilerFusedThreeLaneParity(t, decodeProfilerTracePluginResult(test.payload))
			state := fused.PairState
			if !state.Poisoned[test.kind] || !state.Opaque[test.kind] || state.Withheld[test.kind] != 1 ||
				state.StructuredWithheld[test.kind] != 1 || fused.Published != 0 {
				t.Fatalf("fused pair barrier lost %s state: poisoned=%t opaque=%t withheld=%d structured=%d published=%d rows=%d",
					test.name, state.Poisoned[test.kind], state.Opaque[test.kind], state.Withheld[test.kind],
					state.StructuredWithheld[test.kind], fused.Published, fused.Rows)
			}
		})
	}
}

func TestProfilerFtraceFusedInvalidCPUStillReachesRenderer(t *testing.T) {
	badEvent := syntheticTracePluginFtraceEvent(1_000, 7, 7, "bad", 1109, protoBytes(2, []byte("I|7|bad")))
	goodEvent := syntheticTracePluginFtraceEvent(2_000, 8, 8, "good", 1109, protoBytes(2, []byte("I|8|good")))
	result := decodeProfilerTracePluginResult(protoPayload(
		protoMessage(2, badEvent, protoVarint(1, 1), protoVarint(1, 2)),
		protoMessage(2, protoVarint(1, 3), goodEvent),
	))
	fused := runProfilerFusedLane(t, result, true)
	slot := profilerFtraceEventSlot(1109)
	census := fused.Batch.Slots[slot]
	if !fused.Recognized || fused.Summary.DetailMessages != 1 || fused.Summary.DetailEventCount != 1 ||
		!fused.Summary.DetailCPUs.contains(3) || fused.Summary.DetailCPUs.contains(2) ||
		fused.Rows != 1 || fused.Seq != 1 || census.RowsRead != 2 || census.RowsEmitted != 1 || census.IssueCount == 0 {
		t.Fatalf("invalid CPU summary lane suppressed renderer evidence or polluted summary: fused=%+v slot=%+v", fused, census)
	}
}

func TestProfilerFtraceRenderOnlyIgnoresMalformedMetadata(t *testing.T) {
	event := syntheticTracePluginFtraceEvent(1_000, 7, 7, "worker", 1109, protoBytes(2, []byte("I|7|event")))
	basePayload := protoMessage(2, protoVarint(1, 2), event)
	base := runProfilerFusedLane(t, decodeProfilerTracePluginResult(basePayload), false)
	damagedResult := decodeProfilerTracePluginResult(protoPayload(
		basePayload,
		protoBytes(5, []byte{0x80}),
		protoBytes(6, []byte{0x80}),
		protoBytes(8, []byte{0x80}),
	))
	damagedRender := runProfilerFusedLane(t, damagedResult, false)
	damagedFused := runProfilerFusedLane(t, damagedResult, true)
	if base.Rows != damagedRender.Rows || base.Seq != damagedRender.Seq || base.Output != damagedRender.Output ||
		!reflect.DeepEqual(base.Batch, damagedRender.Batch) {
		t.Fatalf("render-only lane became dependent on malformed metadata:\nbase=%+v\ndamaged=%+v", base, damagedRender)
	}
	if damagedFused.Summary.Issues.empty() || damagedFused.Rows != base.Rows || damagedFused.Output != base.Output {
		t.Fatalf("fused metadata degradation lost summary issue or changed rows: %+v", damagedFused)
	}
}

func TestProfilerFtraceRenderOnlyNotStructuredPreservesPreCancelIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := decodeProfilerTracePluginResult([]byte("plain text, not protobuf"))
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, _, _, recognized, err := renderProfilerFtraceStructuredResultConsumerContext(ctx, result, &seq, sink, true, nil, false)
	if !errors.Is(err, context.Canceled) || recognized || rows != 0 || seq != 0 || sink.stats.RowsAccepted != 0 {
		t.Fatalf("NotStructured render pre-cancel identity drifted: rows=%d seq=%d recognized=%t accepted=%d err=%v",
			rows, seq, recognized, sink.stats.RowsAccepted, err)
	}
}

func TestProfilerFtraceFrameLedgerCommitIsAtomicAcrossSummaryAndEvents(t *testing.T) {
	slot := profilerFtraceEventSlot(1109)
	ledger := newProfilerContainerDiagnosticLedger()
	ledger.FtraceEvents.Slots[slot].RowsRead = math.MaxUint64
	summary := profilerFtraceSummary{
		recognizedMessage: true,
		StartTotalsValid:  true,
		EndTotalsValid:    true,
		DetailOverwriteOK: true,
		DetailMessages:    1,
	}
	var batch profilerFtraceEventBatchCensus
	batch.Slots[slot].RowsRead = 1
	before := ledger
	if ledger.observeFtraceFrame(&summary, true, batch, 44) {
		t.Fatal("cross-ledger observation succeeded despite event counter overflow")
	}
	if !reflect.DeepEqual(ledger, before) {
		t.Fatalf("failed cross-ledger observation left a half-frame commit:\nbefore=%+v\nafter=%+v", before, ledger)
	}
}

func TestProfilerFtraceFrameLedgerCancellationIsAtomicAcrossSummaryAndEvents(t *testing.T) {
	ledger := newProfilerContainerDiagnosticLedger()
	ledger.FtraceSummary.Frames = 7
	ledger.FtraceEvents.Slots[profilerFtraceEventSlot(1109)].RowsRead = 11
	summary := profilerFtraceSummary{
		recognizedMessage: true,
		StartTotalsValid:  true,
		EndTotalsValid:    true,
		DetailOverwriteOK: true,
		DetailMessages:    1,
	}
	var batch profilerFtraceEventBatchCensus
	batch.Slots[profilerFtraceEventSlot(1109)].RowsRead = 1
	before := ledger
	ctx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 2, err: context.DeadlineExceeded,
	}
	ok, err := ledger.observeFtraceFrameContext(ctx, &summary, true, batch, 44)
	if ok || !errors.Is(err, context.DeadlineExceeded) || ctx.polls != ctx.cancelAt {
		t.Fatalf("frame cancellation result=(%t,%v) polls=%d", ok, err, ctx.polls)
	}
	if !reflect.DeepEqual(ledger, before) {
		t.Fatalf("canceled cross-ledger observation mutated state:\nbefore=%+v\nafter=%+v", before, ledger)
	}
}

func TestProfilerFtraceFusedStructurePin(t *testing.T) {
	container := mustReadRendererSource(t, "profiler_container.go")
	processor := sourceBetween(t, container,
		"func consumeProfilerTracePluginResultContext(",
		"func (summary *profilerFtraceSummary) observeCPUDetail(")
	for call, want := range map[string]int{
		"visitProfilerTracePluginResult(ctx, result":              1,
		"auditProfilerFtraceCPUDetail(ctx, raw)":                  1,
		"consumeProfilerFtraceCPUDetailAuthorityContext(ctx,":     1,
		"decodeProfilerFtraceCPUDetailContext(ctx, raw)":          0,
		"renderProfilerFtraceStructuredResultForContainerContext": 0,
	} {
		if got := strings.Count(processor, call); got != want {
			t.Fatalf("fused processor callsite %q count=%d want=%d\n%s", call, got, want, processor)
		}
	}
	detailConsumer := sourceBetween(t, container,
		"func consumeProfilerFtraceCPUDetailAuthorityContext(",
		"func decodeProfilerFtraceSymbolDetail(")
	if strings.Count(detailConsumer, "visitProfilerFtraceCPUDetailEvents(ctx, authority") != 1 {
		t.Fatalf("detail fused consumer must own one event visitor:\n%s", detailConsumer)
	}
	summaryAt := strings.Index(detailConsumer, "detail.observeSummaryEvent(record)")
	renderAt := strings.Index(detailConsumer, "return visit(record)")
	if summaryAt < 0 || renderAt < 0 || summaryAt >= renderAt {
		t.Fatalf("same-event summary observation must precede renderer callback:\n%s", detailConsumer)
	}

	renderer := mustReadRendererSource(t, "profiler_ftrace_render.go")
	renderCore := sourceBetween(t, renderer,
		"func renderProfilerFtraceStructuredResultConsumerContext(",
		"type profilerFtraceCPUDetailAuthority struct {")
	if strings.Count(renderCore, "consumeProfilerTracePluginResultContext(ctx, result") != 1 ||
		strings.Contains(renderCore, "visitProfilerTracePluginResultEventsContext(ctx, result") {
		t.Fatalf("renderer core regained a second top event pass:\n%s", renderCore)
	}
	fusedWrapper := sourceBetween(t, renderer,
		"func renderProfilerFtraceStructuredResultForContainerFusedContext(",
		"func renderProfilerFtraceStructuredResultWithEnvelopeCoverage(")
	if strings.Count(fusedWrapper, "renderProfilerFtraceStructuredResultConsumerContext(") != 1 {
		t.Fatalf("container fused wrapper must delegate to the shared core exactly once:\n%s", fusedWrapper)
	}
}

type profilerCancelInsideFusedDetailContext struct {
	context.Context
	polls    int
	cancelAt int
	err      error
}

func (ctx *profilerCancelInsideFusedDetailContext) Err() error {
	if ctx == nil {
		return context.Canceled
	}
	var callers [32]uintptr
	frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers[:])])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".consumeProfilerFtraceCPUDetailAuthorityContext") {
			ctx.polls++
			if ctx.polls >= ctx.cancelAt {
				if ctx.err != nil {
					return ctx.err
				}
				return context.Canceled
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

func profilerFusedCancellationFixture(events int) []byte {
	var detail bytes.Buffer
	detail.Write(protoVarint(1, 2))
	for index := 0; index < events; index++ {
		detail.Write(syntheticTracePluginFtraceEvent(
			uint64(1_000+index), 7, 7, "worker", 1109,
			protoBytes(2, []byte("I|7|event_"+strconv.Itoa(index))),
		))
	}
	message := syntheticProfilerPluginData("ftrace-plugin", protoBytes(2, detail.Bytes()))
	return syntheticProfilerTraceFile(message)
}

func TestProfilerFtraceFusedMidstreamCancellationLeavesNoCustomerArtifact(t *testing.T) {
	// The typed-event Context authority intentionally adds several governed
	// polls per event. Keep this probe past the first committed event while
	// still stopping deep inside the 2,048-event frame.
	const cancelAt = 512
	body := profilerFusedCancellationFixture(2_048)
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(strings.TrimPrefix(want.Error(), "context "), func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "midstream.htrace")
			output := filepath.Join(dir, "midstream.systrace")
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			header, ok, err := readProfilerTraceHeaderAtPath(input, 0, before.Size())
			if err != nil || !ok {
				t.Fatalf("read profiler cancellation fixture: ok=%t err=%v", ok, err)
			}

			probeSink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			probeCtx := &profilerCancelInsideFusedDetailContext{
				Context: context.Background(), cancelAt: cancelAt, err: want,
			}
			_, probeErr := extractProfilerTraceFileWithFrameLimit(
				probeCtx, input, before.Size(), header, probeSink, maxProfilerPluginFrameBytes)
			accepted := probeSink.stats.RowsAccepted
			cleanupErr := probeSink.cleanup()
			if probeErr != want || cleanupErr != nil || accepted == 0 || accepted >= 2_048 || probeCtx.polls < cancelAt {
				t.Fatalf("cancellation probe did not stop after a staged prefix: accepted=%d polls=%d err=%T %v cleanup=%v",
					accepted, probeCtx.polls, probeErr, probeErr, cleanupErr)
			}

			convertCtx := &profilerCancelInsideFusedDetailContext{
				Context: context.Background(), cancelAt: cancelAt, err: want,
			}
			result, convertErr := ConvertFile(convertCtx, Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
			})
			if convertErr != want || !reflect.DeepEqual(result, Result{}) || convertCtx.polls < cancelAt {
				t.Fatalf("midstream conversion cancellation identity/result drifted: polls=%d result=%+v err=%T %v",
					convertCtx.polls, result, convertErr, convertErr)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("canceled conversion left customer output: %v", err)
			}
			after, err := os.Stat(input)
			if err != nil {
				t.Fatalf("stat protected input after cancellation: %v", err)
			}
			if !os.SameFile(before, after) || after.Mode() != before.Mode() || after.Size() != before.Size() {
				t.Fatalf("canceled conversion changed input identity/mode/size: same=%t before=(%v,%d) after=(%v,%d)",
					os.SameFile(before, after), before.Mode(), before.Size(), after.Mode(), after.Size())
			}
			if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, body) {
				t.Fatalf("canceled conversion changed protected input bytes: equal=%t err=%v", bytes.Equal(got, body), err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
				t.Fatalf("canceled conversion left customer-visible artifacts: %+v", entries)
			}
		})
	}
}
