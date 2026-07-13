package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const profilerOuterCancellationLongLineBytes = 1 << 20

type profilerOuterCancellationContext struct {
	context.Context
	targetSuffix string
	cancelAt     int
	polls        int
	err          error
}

type profilerOuterDirectCallerCancellationContext struct {
	context.Context
	targetSuffix string
	polls        int
	err          error
}

func (ctx *profilerOuterCancellationContext) Err() error {
	if ctx == nil {
		return context.Canceled
	}
	var callers [48]uintptr
	frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers[:])])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ctx.targetSuffix) {
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

func (ctx *profilerOuterDirectCallerCancellationContext) Err() error {
	if ctx == nil {
		return context.Canceled
	}
	var callers [4]uintptr
	count := runtime.Callers(2, callers[:])
	if count > 0 {
		frame, _ := runtime.CallersFrames(callers[:count]).Next()
		if strings.HasSuffix(frame.Function, ctx.targetSuffix) {
			ctx.polls++
			if ctx.err != nil {
				return ctx.err
			}
		}
	}
	if ctx.Context != nil {
		return ctx.Context.Err()
	}
	return nil
}

func profilerOuterCancellationFixture() []byte {
	const unknownOccurrences = 4_096
	unknown := protoVarint(99, 0)
	var payload bytes.Buffer
	for index := 0; index < 256; index++ {
		payload.Write(unknown)
	}
	payload.Write(protoBytes(1, []byte("ftrace-plugin")))
	payload.Write(protoBytes(3, []byte("structured-payload")))
	for index := 256; index < unknownOccurrences; index++ {
		payload.Write(unknown)
	}
	return payload.Bytes()
}

func profilerOuterCancellationBlockLine() string {
	return traceDBFormatLine("worker", 40, 40, 2, 5_001_000_000, 0, 0,
		"block_rq_issue: 0,1 R 4 () 2 + 3 []")
}

func profilerOuterCancellationRejectedFrame(data []byte) []byte {
	return protoPayload(
		protoBytes(1, []byte("bytrace_plugin")),
		protoBytes(1, []byte("duplicate-name")),
		protoBytes(3, data),
	)
}

func profilerOuterScannerBaselinePolls(t *testing.T, exact []byte) int {
	t.Helper()
	ctx := &profilerOuterCancellationContext{
		Context: context.Background(), targetSuffix: ".scanProfilerStrictSystracePayloadContext",
	}
	scan, err := scanProfilerStrictSystracePayloadContext(ctx, exact, nil)
	if err != nil || scan.rejected || !scan.originText || !scan.observed[pairRenderBlock] || ctx.polls == 0 {
		t.Fatalf("calibrate strict scanner polls: polls=%d scan=%+v err=%v", ctx.polls, scan, err)
	}
	return ctx.polls
}

func extractProfilerOuterCancellationFixture(t *testing.T, ctx context.Context, message []byte) (
	profilerContainerExtraction, *traceDBRowSink, error,
) {
	t.Helper()
	body := syntheticProfilerTraceFile(message)
	dir := t.TempDir()
	input := filepath.Join(dir, "outer-cancel.htrace")
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	header, ok, err := readProfilerTraceHeaderAtPath(input, 0, info.Size())
	if err != nil || !ok {
		t.Fatalf("read profiler cancellation header: ok=%t err=%v", ok, err)
	}
	sink, err := newTraceDBRowSink(filepath.Join(dir, "sink"), 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sink.cleanup(); err != nil {
			t.Errorf("cleanup profiler cancellation sink: %v", err)
		}
	})
	extracted, extractErr := extractProfilerTraceFileWithFrameLimit(
		ctx, input, info.Size(), header, sink, maxProfilerPluginFrameBytes)
	return extracted, sink, extractErr
}

func assertProfilerOuterCancellationSinkPristine(t *testing.T, sink *traceDBRowSink) {
	t.Helper()
	if sink == nil {
		t.Fatal("canceled profiler extraction returned a nil sink")
	}
	if !reflect.DeepEqual(sink.stats, traceDBRowSortStats{}) || sink.publishableRows() != 0 ||
		len(sink.rows) != 0 || len(sink.rowIngestOrdinals) != 0 || len(sink.runs) != 0 ||
		len(sink.artifacts) != 0 || sink.bufferedBytes != 0 || sink.activeTempBytes != 0 || sink.liveTempBytes != 0 ||
		sink.pairCensusActive || !reflect.DeepEqual(sink.activePairCensus, profilerPairCensusSet{}) ||
		sink.allRowsFailClosed || sink.pairAuthorityFailure != "" {
		t.Fatalf("canceled profiler extraction mutated row state: stats=%+v rows=%d runs=%d artifacts=%d "+
			"buffered=%d active_temp=%d live_temp=%d census_active=%t census=%+v fail_closed=%t authority=%q",
			sink.stats, len(sink.rows), len(sink.runs), len(sink.artifacts), sink.bufferedBytes,
			sink.activeTempBytes, sink.liveTempBytes, sink.pairCensusActive, sink.activePairCensus,
			sink.allRowsFailClosed, sink.pairAuthorityFailure)
	}
	for _, kind := range profilerCaptureKinds {
		if sink.pairRows[kind] != 0 || sink.poisoned[kind] || sink.opaque[kind] ||
			sink.structuredPairRows[kind] != 0 || len(sink.pairLaneRows[kind]) != 0 ||
			len(sink.pairTableRows[kind]) != 0 || len(sink.poisonedLanes[kind]) != 0 {
			t.Fatalf("canceled profiler extraction mutated pair kind %d state: rows=%d poisoned=%t opaque=%t "+
				"structured=%d lanes=%v tables=%v poisoned_lanes=%v", kind,
				sink.pairRows[kind], sink.poisoned[kind], sink.opaque[kind], sink.structuredPairRows[kind],
				sink.pairLaneRows[kind], sink.pairTableRows[kind], sink.poisonedLanes[kind])
		}
	}
}

func profilerOuterCancellationDiagnosticSeed(t *testing.T) (
	profilerContainerDiagnosticLedger, profilerContainerExtraction,
) {
	t.Helper()
	ledger := newProfilerContainerDiagnosticLedger()
	out := profilerContainerExtraction{}
	seeds := []struct {
		route   profilerPluginRoute
		rawName string
		plugin  profilerPluginData
		outcome profilerPluginOutcome
		offset  int64
	}{
		{
			route: profilerPluginRouteExactFtrace, rawName: "ftrace-plugin",
			plugin:  profilerPluginData{Name: "ftrace-plugin", Data: []byte("seed-structured")},
			outcome: profilerPluginOutcomeStructured, offset: 100,
		},
		{
			route: profilerPluginRouteOtherText, rawName: "seed-plugin",
			plugin:  profilerPluginData{Name: "seed-plugin", Data: []byte("seed-text")},
			outcome: profilerPluginOutcomeNoTextRows, offset: 200,
		},
	}
	for _, seed := range seeds {
		if _, ok := ledger.observeAccepted(&out, seed.route, seed.rawName, seed.plugin,
			profilerPluginIssueCensus{}, seed.offset, seed.outcome, 0, profilerPairCensusSet{}); !ok {
			t.Fatalf("seed profiler diagnostic route=%d failed: ledger=%+v out=%+v", seed.route, ledger, out)
		}
	}
	return ledger, out
}

func TestProfilerPluginDataContextCancellationIsAtomic(t *testing.T) {
	fixture := profilerOuterCancellationFixture()
	wantData := []byte("structured-payload")
	decoded, err := parseProfilerPluginDataContext(context.Background(), fixture)
	if err != nil || !decoded.Accepted || decoded.Plugin.Name != "ftrace-plugin" ||
		!bytes.Equal(decoded.Plugin.Data, wantData) || !decoded.IssueCensus.empty() || decoded.IssueOverflow {
		t.Fatalf("outer parser Background control drifted: decoded=%+v err=%v", decoded, err)
	}
	if legacy := parseProfilerPluginData(fixture); !reflect.DeepEqual(legacy, decoded) {
		t.Fatalf("legacy Background adapter drifted from Context parser:\nlegacy=%+v\ncontext=%+v", legacy, decoded)
	}

	preCanceled, cancel := context.WithCancel(context.Background())
	cancel()
	preDeadline, cancelDeadline := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancelDeadline()
	for _, test := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{name: "pre-canceled", ctx: preCanceled, want: context.Canceled},
		{name: "pre-deadline", ctx: preDeadline, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseProfilerPluginDataContext(test.ctx, fixture)
			if err != test.want || !reflect.DeepEqual(got, profilerPluginDataDecode{}) {
				t.Fatalf("pre-canceled outer parser leaked decode: got=%+v err=%T %v want=%v", got, err, err, test.want)
			}
		})
	}

	for _, test := range []struct {
		name string
		want error
	}{
		{name: "mid-canceled", want: context.Canceled},
		{name: "mid-deadline", want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &profilerOuterCancellationContext{
				Context: context.Background(), targetSuffix: ".parseProfilerPluginDataContext",
				cancelAt: 4, err: test.want,
			}
			got, err := parseProfilerPluginDataContext(ctx, fixture)
			if err != test.want || !reflect.DeepEqual(got, profilerPluginDataDecode{}) || ctx.polls < ctx.cancelAt {
				t.Fatalf("mid-walk outer cancellation leaked decode: polls=%d got=%+v err=%T %v want=%v",
					ctx.polls, got, err, err, test.want)
			}
		})
	}
}

func TestProfilerStrictAndBlockProvenanceCancellationIdentity(t *testing.T) {
	exact := []byte(profilerOuterCancellationBlockLine() + "\n")
	longTail := bytes.Repeat([]byte{'x'}, profilerOuterCancellationLongLineBytes)
	payload := append(append([]byte(nil), exact...), longTail...)
	rejected := profilerOuterCancellationRejectedFrame(payload)
	baseline := profilerOuterScannerBaselinePolls(t, exact)

	if found, err := profilerPayloadContainsExactBlockEndpointContext(context.Background(), exact); err != nil || !found {
		t.Fatalf("noncanonical Block provenance Background control: found=%t err=%v", found, err)
	}
	if found, err := profilerRejectedPluginFrameContainsExactBlockEndpointContext(context.Background(),
		profilerOuterCancellationRejectedFrame(exact)); err != nil || !found {
		t.Fatalf("rejected Block provenance Background control: found=%t err=%v", found, err)
	}

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		name := "canceled"
		if errors.Is(want, context.DeadlineExceeded) {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			t.Run("strict-scan", func(t *testing.T) {
				ctx := &profilerOuterCancellationContext{
					Context: context.Background(), targetSuffix: ".scanProfilerStrictSystracePayloadContext",
					cancelAt: baseline + 1, err: want,
				}
				visits := 0
				scan, err := scanProfilerStrictSystracePayloadContext(ctx, payload,
					func(renderedRow, profilerPairAdmission) { visits++ })
				if err != want || !reflect.DeepEqual(scan, profilerStrictSystracePayloadScan{}) ||
					visits != 1 || ctx.polls < ctx.cancelAt {
					t.Fatalf("strict scan cancellation drifted: polls=%d baseline=%d visits=%d scan=%+v err=%T %v want=%v",
						ctx.polls, baseline, visits, scan, err, err, want)
				}
			})

			t.Run("noncanonical-probe", func(t *testing.T) {
				ctx := &profilerOuterCancellationContext{
					Context: context.Background(), targetSuffix: ".scanProfilerStrictSystracePayloadContext",
					cancelAt: baseline + 1, err: want,
				}
				found, err := profilerPayloadContainsExactBlockEndpointContext(ctx, payload)
				if err != want || found || ctx.polls < ctx.cancelAt {
					t.Fatalf("noncanonical provenance cancellation drifted: polls=%d baseline=%d found=%t err=%T %v want=%v",
						ctx.polls, baseline, found, err, err, want)
				}
			})

			t.Run("rejected-probe", func(t *testing.T) {
				ctx := &profilerOuterCancellationContext{
					Context: context.Background(), targetSuffix: ".scanProfilerStrictSystracePayloadContext",
					cancelAt: baseline + 1, err: want,
				}
				found, err := profilerRejectedPluginFrameContainsExactBlockEndpointContext(ctx, rejected)
				if err != want || found || ctx.polls < ctx.cancelAt {
					t.Fatalf("rejected provenance cancellation drifted: polls=%d baseline=%d found=%t err=%T %v want=%v",
						ctx.polls, baseline, found, err, err, want)
				}
			})
		})
	}
}

func TestProfilerStrictScanManyBlankLinesCancellationIdentity(t *testing.T) {
	blankLines := bytes.Repeat([]byte{'\n'}, profilerContextByteCheckpointBytes*4)
	scan, err := scanProfilerStrictSystracePayloadContext(context.Background(), blankLines, nil)
	if err != nil || !reflect.DeepEqual(scan, profilerStrictSystracePayloadScan{}) {
		t.Fatalf("blank-line Background control drifted: scan=%+v err=%v", scan, err)
	}
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		name := "canceled"
		if errors.Is(want, context.DeadlineExceeded) {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			ctx := &profilerOuterCancellationContext{
				Context: context.Background(), targetSuffix: ".scanProfilerStrictSystracePayloadContext",
				cancelAt: 4, err: want,
			}
			visits := 0
			got, err := scanProfilerStrictSystracePayloadContext(ctx, blankLines,
				func(renderedRow, profilerPairAdmission) { visits++ })
			if err != want || !reflect.DeepEqual(got, profilerStrictSystracePayloadScan{}) ||
				visits != 0 || ctx.polls < ctx.cancelAt {
				t.Fatalf("blank-line cancellation drifted: polls=%d visits=%d scan=%+v err=%T %v want=%v",
					ctx.polls, visits, got, err, err, want)
			}
		})
	}
}

func TestProfilerOuterFinalCancellationIdentity(t *testing.T) {
	hardRejected := protoBytes(3, []byte("payload-without-name"))
	decoded, err := parseProfilerPluginDataContext(context.Background(), hardRejected)
	if err != nil || decoded.Accepted || decoded.IssueCensus.empty() || decoded.IssueOverflow {
		t.Fatalf("hard-rejected Background control drifted: decoded=%+v err=%v", decoded, err)
	}

	exact := []byte(profilerOuterCancellationBlockLine() + "\n")
	rejectedWithMalformedTail := append(profilerOuterCancellationRejectedFrame(exact), 0x80)
	if found, err := profilerRejectedPluginFrameContainsExactBlockEndpointContext(
		context.Background(), rejectedWithMalformedTail); err != nil || !found {
		t.Fatalf("rejected provenance malformed-tail control drifted: found=%t err=%v", found, err)
	}

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		name := "canceled"
		if errors.Is(want, context.DeadlineExceeded) {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			t.Run("hard-rejected-parser", func(t *testing.T) {
				ctx := &profilerOuterDirectCallerCancellationContext{
					Context: context.Background(), targetSuffix: ".parseProfilerPluginDataContext", err: want,
				}
				got, err := parseProfilerPluginDataContext(ctx, hardRejected)
				if err != want || !reflect.DeepEqual(got, profilerPluginDataDecode{}) || ctx.polls != 1 {
					t.Fatalf("hard-rejected final cancellation drifted: polls=%d got=%+v err=%T %v want=%v",
						ctx.polls, got, err, err, want)
				}
			})
			t.Run("rejected-provenance", func(t *testing.T) {
				ctx := &profilerOuterDirectCallerCancellationContext{
					Context:      context.Background(),
					targetSuffix: ".profilerRejectedPluginFrameContainsExactBlockEndpointContext", err: want,
				}
				found, err := profilerRejectedPluginFrameContainsExactBlockEndpointContext(ctx, rejectedWithMalformedTail)
				if err != want || found || ctx.polls != 1 {
					t.Fatalf("rejected provenance final cancellation drifted: polls=%d found=%t err=%T %v want=%v",
						ctx.polls, found, err, err, want)
				}
			})
		})
	}
}

func TestProfilerObserveAcceptedContextCancellationIsProspective(t *testing.T) {
	large := strings.Repeat("v", maxTraceDBSystraceLineBytes)
	tests := []struct {
		name    string
		route   profilerPluginRoute
		rawName string
		plugin  profilerPluginData
		outcome profilerPluginOutcome
	}{
		{
			name: "exact-version", route: profilerPluginRouteExactFtrace, rawName: "ftrace-plugin",
			plugin: profilerPluginData{
				Name: "ftrace-plugin", Data: []byte("structured"), Version: large, VersionPresent: true,
			},
			outcome: profilerPluginOutcomeStructured,
		},
		{
			name: "other-text-name", route: profilerPluginRouteOtherText, rawName: large,
			plugin:  profilerPluginData{Name: large, Data: []byte("text")},
			outcome: profilerPluginOutcomeNoTextRows,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contextLedger, contextOut := profilerOuterCancellationDiagnosticSeed(t)
			legacyLedger, legacyOut := profilerOuterCancellationDiagnosticSeed(t)
			contextIndex, contextOK, contextErr := contextLedger.observeAcceptedContext(context.Background(),
				&contextOut, test.route, test.rawName, test.plugin, profilerPluginIssueCensus{},
				300, test.outcome, 0, profilerPairCensusSet{})
			legacyIndex, legacyOK := legacyLedger.observeAccepted(&legacyOut, test.route, test.rawName,
				test.plugin, profilerPluginIssueCensus{}, 300, test.outcome, 0, profilerPairCensusSet{})
			if contextErr != nil || !contextOK || !legacyOK || contextIndex != legacyIndex ||
				!reflect.DeepEqual(contextLedger, legacyLedger) || !reflect.DeepEqual(contextOut, legacyOut) {
				t.Fatalf("Background/legacy diagnostic parity drifted: context=(%d,%t,%v) legacy=(%d,%t)\n"+
					"context ledger=%+v\nlegacy ledger=%+v\ncontext out=%+v\nlegacy out=%+v",
					contextIndex, contextOK, contextErr, legacyIndex, legacyOK,
					contextLedger, legacyLedger, contextOut, legacyOut)
			}

			for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
				name := "canceled"
				if errors.Is(want, context.DeadlineExceeded) {
					name = "deadline"
				}
				t.Run(name, func(t *testing.T) {
					ledger, out := profilerOuterCancellationDiagnosticSeed(t)
					wantLedger, wantOut := profilerOuterCancellationDiagnosticSeed(t)
					ctx := &profilerOuterCancellationContext{
						Context: context.Background(), targetSuffix: ".observeStringContext",
						cancelAt: 4, err: want,
					}
					index, ok, err := ledger.observeAcceptedContext(ctx, &out, test.route, test.rawName,
						test.plugin, profilerPluginIssueCensus{}, 300, test.outcome, 0, profilerPairCensusSet{})
					if err != want || ok || index != -1 || ctx.polls < ctx.cancelAt ||
						!reflect.DeepEqual(ledger, wantLedger) || !reflect.DeepEqual(out, wantOut) {
						t.Fatalf("diagnostic cancellation was not prospective: index=%d ok=%t polls=%d err=%T %v want=%v\n"+
							"ledger=%+v\nwant ledger=%+v\nout=%+v\nwant out=%+v",
							index, ok, ctx.polls, err, err, want, ledger, wantLedger, out, wantOut)
					}
				})
			}
		})
	}
}

func TestProfilerAcceptedDiagnosticHashCancellationRollsBackConvertFile(t *testing.T) {
	largeVersion := strings.Repeat("v", maxTraceDBSystraceLineBytes)
	detail := protoPayload(
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 7, 7, "worker", 1109,
			protoBytes(2, []byte("I|7|staged-before-diagnostic-cancel"))),
	)
	structured := protoBytes(2, detail)
	message := syntheticProfilerPluginDataWithTiming(
		"ftrace-plugin", structured, 7, 1, 2, largeVersion, 0)
	body := syntheticProfilerTraceFile(message)

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		name := "canceled"
		if errors.Is(want, context.DeadlineExceeded) {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "late-diagnostic.htrace")
			output := filepath.Join(dir, "late-diagnostic.systrace")
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			header, ok, err := readProfilerTraceHeaderAtPath(input, 0, before.Size())
			if err != nil || !ok {
				t.Fatalf("read late-diagnostic header: ok=%t err=%v", ok, err)
			}

			probeSink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			probeCtx := &profilerOuterCancellationContext{
				Context: context.Background(), targetSuffix: ".observeStringContext",
				cancelAt: 4, err: want,
			}
			extracted, probeErr := extractProfilerTraceFileWithFrameLimit(
				probeCtx, input, before.Size(), header, probeSink, maxProfilerPluginFrameBytes)
			accepted := probeSink.stats.RowsAccepted
			publishable := probeSink.publishableRows()
			cleanupErr := probeSink.cleanup()
			if probeErr != want || !reflect.DeepEqual(extracted, profilerContainerExtraction{}) ||
				probeCtx.polls < probeCtx.cancelAt || accepted != 1 || publishable != 1 || cleanupErr != nil {
				t.Fatalf("late diagnostic probe did not cancel after one staged row: polls=%d accepted=%d "+
					"publishable=%d extracted=%+v err=%T %v cleanup=%v want=%v",
					probeCtx.polls, accepted, publishable, extracted, probeErr, probeErr, cleanupErr, want)
			}

			convertCtx := &profilerOuterCancellationContext{
				Context: context.Background(), targetSuffix: ".observeStringContext",
				cancelAt: 4, err: want,
			}
			result, convertErr := ConvertFile(convertCtx, Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
			})
			if convertErr != want || !reflect.DeepEqual(result, Result{}) || convertCtx.polls < convertCtx.cancelAt {
				t.Fatalf("late diagnostic conversion cancellation identity/result drifted: polls=%d result=%+v "+
					"err=%T %v want=%v", convertCtx.polls, result, convertErr, convertErr, want)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("canceled late diagnostic conversion left output: %v", err)
			}
			after, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			gotBody, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() ||
				!bytes.Equal(gotBody, body) {
				t.Fatalf("canceled late diagnostic conversion changed protected input: same=%t mode=%v/%v "+
					"size=%d/%d bytes_equal=%t", os.SameFile(before, after), before.Mode(), after.Mode(),
					before.Size(), after.Size(), bytes.Equal(gotBody, body))
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
				t.Fatalf("canceled late diagnostic conversion left customer-visible artifacts: %+v", entries)
			}
		})
	}
}

func TestProfilerOuterCancellationLeavesExtractionAndPairStatePristine(t *testing.T) {
	outerFixture := profilerOuterCancellationFixture()
	exact := []byte(profilerOuterCancellationBlockLine() + "\n")
	longTail := bytes.Repeat([]byte{'x'}, profilerOuterCancellationLongLineBytes)
	payload := append(append([]byte(nil), exact...), longTail...)
	baseline := profilerOuterScannerBaselinePolls(t, exact)

	for _, test := range []struct {
		name         string
		message      []byte
		targetSuffix string
		cancelAt     int
		want         error
	}{
		{
			name: "outer-parse-canceled", message: outerFixture,
			targetSuffix: ".parseProfilerPluginDataContext", cancelAt: 4, want: context.Canceled,
		},
		{
			name: "strict-stage-deadline", message: syntheticProfilerPluginData("ftrace-plugin", payload),
			targetSuffix: ".scanProfilerStrictSystracePayloadContext", cancelAt: baseline + 1, want: context.DeadlineExceeded,
		},
		{
			name: "noncanonical-provenance-canceled", message: syntheticProfilerPluginData("FTRACE-PLUGIN", payload),
			targetSuffix: ".scanProfilerStrictSystracePayloadContext", cancelAt: baseline + 1, want: context.Canceled,
		},
		{
			name: "rejected-provenance-deadline", message: profilerOuterCancellationRejectedFrame(payload),
			targetSuffix: ".scanProfilerStrictSystracePayloadContext", cancelAt: baseline + 1, want: context.DeadlineExceeded,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &profilerOuterCancellationContext{
				Context: context.Background(), targetSuffix: test.targetSuffix,
				cancelAt: test.cancelAt, err: test.want,
			}
			extracted, sink, err := extractProfilerOuterCancellationFixture(t, ctx, test.message)
			if err != test.want || !reflect.DeepEqual(extracted, profilerContainerExtraction{}) || ctx.polls < ctx.cancelAt {
				t.Fatalf("canceled extraction escaped atomic return: polls=%d extracted=%+v err=%T %v want=%v",
					ctx.polls, extracted, err, err, test.want)
			}
			assertProfilerOuterCancellationSinkPristine(t, sink)
		})
	}

	t.Run("hard-rejected-final-canceled", func(t *testing.T) {
		ctx := &profilerOuterDirectCallerCancellationContext{
			Context: context.Background(), targetSuffix: ".parseProfilerPluginDataContext", err: context.Canceled,
		}
		extracted, sink, err := extractProfilerOuterCancellationFixture(t, ctx,
			protoBytes(3, []byte("payload-without-name")))
		if err != context.Canceled || !reflect.DeepEqual(extracted, profilerContainerExtraction{}) || ctx.polls != 1 {
			t.Fatalf("hard-rejected extraction cancellation escaped atomic return: polls=%d extracted=%+v err=%T %v",
				ctx.polls, extracted, err, err)
		}
		assertProfilerOuterCancellationSinkPristine(t, sink)
	})

	t.Run("rejected-provenance-final-deadline", func(t *testing.T) {
		exactFrame := profilerOuterCancellationRejectedFrame([]byte(profilerOuterCancellationBlockLine() + "\n"))
		message := append(exactFrame, 0x80)
		ctx := &profilerOuterDirectCallerCancellationContext{
			Context:      context.Background(),
			targetSuffix: ".profilerRejectedPluginFrameContainsExactBlockEndpointContext",
			err:          context.DeadlineExceeded,
		}
		extracted, sink, err := extractProfilerOuterCancellationFixture(t, ctx, message)
		if err != context.DeadlineExceeded || !reflect.DeepEqual(extracted, profilerContainerExtraction{}) || ctx.polls != 1 {
			t.Fatalf("rejected provenance extraction cancellation escaped atomic return: polls=%d extracted=%+v err=%T %v",
				ctx.polls, extracted, err, err)
		}
		assertProfilerOuterCancellationSinkPristine(t, sink)
	})
}

func TestProfilerOuterCancellationProductionContextTopology(t *testing.T) {
	container := mustReadRendererSource(t, "profiler_container.go")
	extract := sourceBetween(t, container,
		"func extractProfilerTraceFileAtWithFrameLimit(",
		"func appendProfilerZeroFrameCensus(")
	if strings.Count(extract, "parseProfilerPluginDataContext(ctx, msg)") != 1 ||
		strings.Contains(extract, "parseProfilerPluginData(msg)") {
		t.Fatalf("production extraction lost its unique outer Context parser:\n%s", extract)
	}
	if strings.Count(extract, "diagnostics.observeAcceptedContext(ctx, &out") != 1 ||
		strings.Contains(extract, "diagnostics.observeAccepted(&out") {
		t.Fatalf("production extraction lost its unique prospective Context diagnostics path:\n%s", extract)
	}
	for _, call := range []string{
		"stageProfilerStrictSystracePayloadContext(ctx, plugin.Data)",
		"profilerPayloadContainsExactBlockEndpointContext(ctx, plugin.Data)",
		"profilerRejectedPluginFrameContainsExactBlockEndpointContext(ctx, msg)",
	} {
		if strings.Count(extract, call) != 1 {
			t.Fatalf("production extraction Context provenance call %q count=%d, want 1:\n%s",
				call, strings.Count(extract, call), extract)
		}
	}
	for _, legacy := range []string{
		"stageProfilerStrictSystracePayload(plugin.Data)",
		"profilerPayloadContainsExactBlockEndpoint(plugin.Data)",
		"profilerRejectedPluginFrameContainsExactBlockEndpoint(msg)",
	} {
		if strings.Contains(extract, legacy) {
			t.Fatalf("production extraction regained context-free outer/provenance call %q:\n%s", legacy, extract)
		}
	}

	legacy := sourceBetween(t, container,
		"func parseProfilerPluginData(data []byte) profilerPluginDataDecode {",
		"func parseProfilerPluginDataContext(")
	if strings.Count(legacy, "parseProfilerPluginDataContext(context.Background(), data)") != 1 ||
		strings.Contains(legacy, "walkProtoFields") || strings.Contains(legacy, "walkProfilerProtoFieldsContext") {
		t.Fatalf("legacy outer parser is not a Background-only compatibility adapter:\n%s", legacy)
	}
	contextParser := sourceBetween(t, container,
		"func parseProfilerPluginDataContext(",
		"// profilerPayloadContainsExactBlockEndpoint")
	if strings.Count(contextParser, "walkProfilerProtoFieldsContext(ctx, data") != 1 ||
		strings.Contains(contextParser, "walkProtoFields(data") {
		t.Fatalf("outer Context parser regained a context-free protobuf walk:\n%s", contextParser)
	}
	if strings.Count(contextParser, "profilerSingleTokenBytesContext(ctx, byteValues[1])") != 1 ||
		strings.Count(contextParser, "profilerSinglePhysicalLineBytesContext(ctx, byteValues[7], true)") != 1 ||
		strings.Count(contextParser, "profilerCloneBytesStringContext(ctx, byteValues[") != 2 ||
		strings.Contains(contextParser, "traceDBSingleToken(") ||
		strings.Contains(contextParser, "traceDBSinglePhysicalLine(") ||
		strings.Contains(contextParser, "string(byteValues[") {
		t.Fatalf("outer Context parser regained a context-free long-string validator/clone:\n%s", contextParser)
	}
	finalCheckAt := strings.LastIndex(contextParser, "if err := ctx.Err(); err != nil")
	hardReturnAt := strings.Index(contextParser, "if hardRejected || decoded.IssueOverflow")
	if finalCheckAt < 0 || hardReturnAt < 0 || finalCheckAt > hardReturnAt {
		t.Fatalf("outer parser can return hard-rejected/IssueOverflow state without a final cancellation check:\n%s",
			contextParser)
	}
	rejectedContext := sourceBetween(t, container,
		"func profilerRejectedPluginFrameContainsExactBlockEndpointContext(",
		"func addSystraceRowsFromBytes(")
	if !strings.Contains(rejectedContext, "if err := ctx.Err(); err != nil") {
		t.Fatalf("rejected Block provenance probe lost its final cancellation check:\n%s", rejectedContext)
	}

	authority := mustReadRendererSource(t, "profiler_ftrace_authority.go")
	scanContext := sourceBetween(t, authority,
		"func scanProfilerStrictSystracePayloadContext(",
		"// profilerTextCommentPrefix")
	if !strings.Contains(scanContext, "profilerContextByteCheckpointBytes") ||
		!strings.Contains(scanContext, "ctx.Err()") {
		t.Fatalf("strict text Context scan lost its in-line byte checkpoint:\n%s", scanContext)
	}
	if strings.Contains(scanContext, "bytes.Trim(") ||
		strings.Count(scanContext, "profilerTrimASCIISpacesBytesContext(ctx, part)") != 1 ||
		strings.Count(scanContext, "profilerTextCommentPrefixContext(ctx, part)") != 1 {
		t.Fatalf("strict text Context scan regained an unbounded trim/comment-prefix path:\n%s", scanContext)
	}
	if strings.Contains(authority, "func profilerTextPhysicalRunesSafe(") {
		t.Fatalf("strict text authority regained a second context-free rune validator")
	}
}
