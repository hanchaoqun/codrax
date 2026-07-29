package hitraceconv

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestPrepareTraceDBRenderedRowStrictBoundaries(t *testing.T) {
	if _, err := prepareTraceDBRenderedRow(0, -1, "task", 1, 1, 0, "print: x"); err == nil {
		t.Fatal("negative internal sequence must be rejected")
	} else if reason, ok := traceDBOutputInvariantReason(err); !ok || reason != "invalid_sequence" {
		t.Fatalf("unexpected negative sequence rejection: reason=%q typed=%v err=%v", reason, ok, err)
	}
	valid := []struct {
		name               string
		ts, tid, tgid, cpu int64
		task, body         string
	}{
		{name: "zero time and CPU-only pseudo identity", ts: 0, tid: 0, tgid: 0, cpu: 0, task: "<idle>", body: "cpu_idle: state=0 cpu_id=0"},
		{name: "thread identity", ts: 1, tid: 7, tgid: 3, cpu: 2, task: "worker", body: "print: ok"},
		{name: "maximum scalar bounds", ts: math.MaxInt64, tid: math.MaxInt32, tgid: math.MaxInt32, cpu: 4095, task: "max", body: "print: max"},
		{name: "blank task canonicalizes", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "  ", body: "print: blank-task"},
		{name: "marker separators remain legal body grammar", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "tracing_mark_write: I|1|point"},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			row, err := prepareTraceDBRenderedRow(tc.ts, 9, tc.task, tc.tid, tc.tgid, tc.cpu, tc.body)
			if err != nil {
				t.Fatalf("valid endpoint rejected: %v", err)
			}
			if row.tsNS != traceDBStandardWireTimestampNS(tc.ts) || row.seq != 9 || !strings.Contains(row.line, tc.body) {
				t.Fatalf("unexpected prepared row: %+v", row)
			}
		})
	}

	invalidUTF8 := string([]byte{0xff, 'x'})
	invalid := []struct {
		name               string
		ts, tid, tgid, cpu int64
		task, body         string
		reason             string
	}{
		{name: "negative timestamp", ts: -1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "print: x", reason: "invalid_timestamp"},
		{name: "negative CPU", ts: 1, tid: 1, tgid: 1, cpu: -1, task: "task", body: "print: x", reason: "invalid_cpu"},
		{name: "large CPU", ts: 1, tid: 1, tgid: 1, cpu: 4096, task: "task", body: "print: x", reason: "invalid_cpu"},
		{name: "negative TID", ts: 1, tid: -1, tgid: 1, cpu: 0, task: "task", body: "print: x", reason: "invalid_tid"},
		{name: "large TID", ts: 1, tid: math.MaxInt32 + 1, tgid: 1, cpu: 0, task: "task", body: "print: x", reason: "invalid_tid"},
		{name: "negative TGID", ts: 1, tid: 1, tgid: -1, cpu: 0, task: "task", body: "print: x", reason: "invalid_tgid"},
		{name: "large TGID", ts: 1, tid: 1, tgid: math.MaxInt32 + 1, cpu: 0, task: "task", body: "print: x", reason: "invalid_tgid"},
		{name: "missing TID", ts: 1, tid: 0, tgid: 1, cpu: 0, task: "task", body: "print: x", reason: "incomplete_header_identity"},
		{name: "missing TGID", ts: 1, tid: 1, tgid: 0, cpu: 0, task: "task", body: "print: x", reason: "incomplete_header_identity"},
		{name: "task LF", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "bad\nname", body: "print: x", reason: "invalid_task"},
		{name: "task invalid UTF8", ts: 1, tid: 1, tgid: 1, cpu: 0, task: invalidUTF8, body: "print: x", reason: "invalid_task"},
		{name: "body LF", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "print: a\nb", reason: "invalid_body"},
		{name: "body CR", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "print: a\rb", reason: "invalid_body"},
		{name: "body tab", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "print: a\tb", reason: "invalid_body"},
		{name: "body NUL", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "print: a\x00b", reason: "invalid_body"},
		{name: "body DEL", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "print: a\x7fb", reason: "invalid_body"},
		{name: "body line separator", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "print: a\u2028b", reason: "invalid_body"},
		{name: "body paragraph separator", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: "print: a\u2029b", reason: "invalid_body"},
		{name: "body invalid UTF8", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: invalidUTF8, reason: "invalid_body"},
		{name: "blank body", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: " ", reason: "invalid_body"},
		{name: "rendered line cap", ts: 1, tid: 1, tgid: 1, cpu: 0, task: "task", body: strings.Repeat("x", maxTraceDBSystraceLineBytes), reason: "line_too_long"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			_, err := prepareTraceDBRenderedRow(tc.ts, 0, tc.task, tc.tid, tc.tgid, tc.cpu, tc.body)
			if reason, ok := traceDBOutputInvariantReason(err); !ok || reason != tc.reason {
				t.Fatalf("expected reason %q, got reason=%q typed=%v err=%v", tc.reason, reason, ok, err)
			}
		})
	}
}

func TestTraceDBMixedWirePrecisionSortRemainsMonotonic(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()

	standardDown, err := prepareTraceDBRenderedRow(1_499, 0, "standard-down", 11, 11, 0,
		"tracing_mark_write: I|11|standard-down")
	if err != nil {
		t.Fatal(err)
	}
	exactEarly, err := prepareTraceDBFrameMapRow(1_200, 1, 1, 10, 11, 900)
	if err != nil {
		t.Fatal(err)
	}
	standardUp, err := prepareTraceDBRenderedRow(1_501, 2, "standard-up", 12, 12, 0,
		"tracing_mark_write: I|12|standard-up")
	if err != nil {
		t.Fatal(err)
	}
	exactLate, err := prepareTraceDBFrameMapRow(1_800, 3, 2, 12, 13, 1_300)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []renderedRow{standardUp, exactLate, standardDown, exactEarly} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}

	var output strings.Builder
	if _, err := sink.prepareAndWriteForTest(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mixed-wire-precision.systrace")
	if err := os.WriteFile(path, []byte(output.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	var timestamps []float64
	idx, err := tracequery.StreamScan(context.Background(), path, tracequery.TraceFlavorAuto, func(event tracequery.Event) bool {
		timestamps = append(timestamps, event.Ts)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if idx.ClockRegressions != 0 {
		t.Fatalf("wire timestamp merge regressed: regressions=%d output=%q", idx.ClockRegressions, output.String())
	}
	want := []float64{0.0000012, 0.000001499, 0.000001501, 0.0000018}
	if len(timestamps) != len(want) {
		t.Fatalf("unexpected parsed timestamp count: got=%v want=%v output=%q", timestamps, want, output.String())
	}
	for index := range want {
		if timestamps[index] != want[index] {
			t.Fatalf("timestamp[%d]=%.10f want=%.10f all=%v output=%q",
				index, timestamps[index], want[index], timestamps, output.String())
		}
	}
}

func TestTraceDBPublishFrozenRawRecordUsesRenderedWireTimestamp(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()

	line := traceDBFormatLine("raw", 9, 9, 0, 1_499, 0, 0, "tracing_mark_write: I|9|raw")
	if err := traceDBPublishFrozenRawRecord(sink, 1_499, line); err != nil {
		t.Fatal(err)
	}
	if len(sink.rows) != 1 || sink.rows[0].tsNS != 1_000 {
		t.Fatalf("frozen raw sorter key must equal parsed wire timestamp: rows=%+v", sink.rows)
	}
}

func TestPrepareTraceDBRenderedRowWithTraceFlagsContextParityAndCancellation(t *testing.T) {
	args := struct {
		tsNS, tid, tgid, cpu, flags, preempt int64
		seq                                  int
		task, body                           string
	}{
		tsNS: 1_000, seq: 7, task: "worker", tid: 10, tgid: 10, cpu: 2,
		flags: 0x0d, preempt: 2, body: "print: " + strings.Repeat("x", 4*profilerContextByteCheckpointBytes+31),
	}
	legacy, legacyErr := prepareTraceDBRenderedRowWithTraceFlags(args.tsNS, args.seq, args.task,
		args.tid, args.tgid, args.cpu, args.flags, args.preempt, args.body)
	background, backgroundErr := prepareTraceDBRenderedRowWithTraceFlagsContext(context.Background(), args.tsNS, args.seq,
		args.task, args.tid, args.tgid, args.cpu, args.flags, args.preempt, args.body)
	nilContext, nilErr := prepareTraceDBRenderedRowWithTraceFlagsContext(nil, args.tsNS, args.seq,
		args.task, args.tid, args.tgid, args.cpu, args.flags, args.preempt, args.body)
	if legacyErr != nil || backgroundErr != nil || nilErr != nil || legacy != background || legacy != nilContext {
		t.Fatalf("row endpoint parity legacy=(%+v,%v) background=(%+v,%v) nil=(%+v,%v)",
			legacy, legacyErr, background, backgroundErr, nilContext, nilErr)
	}

	calibration := &profilerByteCancelAfterPollContext{Context: context.Background()}
	if _, err := prepareTraceDBRenderedRowWithTraceFlagsContext(calibration, args.tsNS, args.seq,
		args.task, args.tid, args.tgid, args.cpu, args.flags, args.preempt, args.body); err != nil || calibration.polls < 5 {
		t.Fatalf("row endpoint cancellation calibration polls=%d err=%v", calibration.polls, err)
	}
	for _, test := range []struct {
		name     string
		cancelAt int
		want     error
	}{
		{name: "pre-canceled", cancelAt: 1, want: context.Canceled},
		{name: "mid-body", cancelAt: max(2, calibration.polls/2), want: context.DeadlineExceeded},
		{name: "final-check", cancelAt: calibration.polls, want: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &profilerByteCancelAfterPollContext{
				Context: context.Background(), cancelAt: test.cancelAt, err: test.want,
			}
			row, err := prepareTraceDBRenderedRowWithTraceFlagsContext(ctx, args.tsNS, args.seq,
				args.task, args.tid, args.tgid, args.cpu, args.flags, args.preempt, args.body)
			if row != (renderedRow{}) || err != test.want || ctx.polls != test.cancelAt {
				t.Fatalf("row endpoint cancellation row=%+v polls=%d/%d err=%T %v want=%v",
					row, ctx.polls, test.cancelAt, err, err, test.want)
			}
		})
	}
}

func TestTraceDBEndpointRejectionDoesNotMutateSink(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	before := sink.stats
	if err := addTraceDBInstantRow(sink, -1, "task", 1, 1, 0, "print: bad"); err == nil {
		t.Fatal("negative timestamp should be rejected")
	}
	if sink.stats != before || len(sink.rows) != 0 || len(sink.runs) != 0 {
		t.Fatalf("rejected endpoint mutated sink: before=%+v after=%+v rows=%d runs=%d", before, sink.stats, len(sink.rows), len(sink.runs))
	}
	if err := sink.add(renderedRow{tsNS: 1, seq: 0, line: "forged\nsecond-line"}); err == nil {
		t.Fatal("sink must retain a final single-physical-line defense")
	}
	if sink.stats != before || len(sink.rows) != 0 || len(sink.runs) != 0 {
		t.Fatalf("rejected rendered row mutated sink: before=%+v after=%+v", before, sink.stats)
	}
}

func TestTraceDBSpanEndpointsValidateAtomically(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, end int64
		span       string
		reason     string
	}{
		{name: "reverse interval", start: 2, end: 1, span: "work", reason: "invalid_interval"},
		{name: "negative start", start: -1, end: 1, span: "work", reason: "invalid_timestamp"},
		{name: "marker injection", start: 1, end: 2, span: "work|fake", reason: "invalid_span_name"},
		{name: "line injection", start: 1, end: 2, span: "work\nE|1", reason: "invalid_span_name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 4)
			if err != nil {
				t.Fatal(err)
			}
			syncSpans := newTraceDBTestSyncSpanAuthority(t)
			err = syncSpans.submit(context.Background(), traceDBSyncSpanCandidate{
				Producer:           traceDBSyncSpanProducerSyscall,
				StableKind:         traceDBSyncSpanStableSyscallRowID,
				StableID:           1,
				HeaderTID:          1,
				HeaderTGID:         1,
				CanonicalITID:      1,
				CanonicalITIDKnown: true,
				OwnerIPID:          1,
				OwnerIPIDKnown:     true,
				Start:              tc.start,
				End:                tc.end,
				StartCPU:           0,
				EndCPU:             0,
				StartCPUProvenance: traceDBSyncSpanCPULegacyUnverified,
				EndCPUProvenance:   traceDBSyncSpanCPULegacyUnverified,
				Task:               "task",
				Name:               tc.span,
				NameProvenance:     traceDBSyncSpanNameSyscallNumber,
				DepthProvenance:    traceDBSyncSpanDepthUnknown,
			})
			if reason, ok := traceDBOutputInvariantReason(err); !ok || reason != tc.reason {
				t.Fatalf("expected %q, got reason=%q typed=%v err=%v", tc.reason, reason, ok, err)
			}
			if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 {
				t.Fatalf("invalid span published a partial endpoint: stats=%+v rows=%+v", sink.stats, sink.rows)
			}
		})
	}
}

func TestTraceDBAsyncSpanEndpointsValidateAtomically(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		start, end, owner, startCPU, endCPU int64
		span, cookie                        string
		reason                              string
	}{
		{name: "reverse interval", start: 2, end: 1, owner: 1, span: "work", cookie: "row-1", reason: "invalid_interval"},
		{name: "missing owner", start: 1, end: 2, owner: 0, span: "work", cookie: "row-1", reason: "invalid_async_owner"},
		{name: "name injection", start: 1, end: 2, owner: 1, span: "work|fake", cookie: "row-1", reason: "invalid_span_name"},
		{name: "cookie injection", start: 1, end: 2, owner: 1, span: "work", cookie: "row|fake", reason: "invalid_span_cookie"},
		{name: "invalid start CPU", start: 1, end: 2, owner: 1, startCPU: -1, span: "work", cookie: "row-1", reason: "invalid_cpu"},
		{name: "invalid end CPU", start: 1, end: 2, owner: 1, endCPU: -1, span: "work", cookie: "row-1", reason: "invalid_cpu"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 4)
			if err != nil {
				t.Fatal(err)
			}
			err = addTraceDBAsyncSpanRows(sink, tc.start, tc.end, "task", 1, tc.owner, tc.startCPU, tc.endCPU, tc.span, tc.cookie)
			if reason, ok := traceDBOutputInvariantReason(err); !ok || reason != tc.reason {
				t.Fatalf("expected %q, got reason=%q typed=%v err=%v", tc.reason, reason, ok, err)
			}
			if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 {
				t.Fatalf("invalid async span published a partial endpoint: stats=%+v rows=%+v", sink.stats, sink.rows)
			}
		})
	}
}

func TestTraceDBExactNanosecondStandardSpanRoundTrip(t *testing.T) {
	const (
		start = int64(69_326_000_000_100)
		end   = start + 500
	)
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := addTraceDBTestSyncSpanRows(
		sink, start, end, "worker", 10, 10, 2, "sub-us-official-viewer",
	); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if _, err := sink.prepareAndWriteForTest(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, timestamp := range []string{"69326.000000100", "69326.000000600"} {
		if !strings.Contains(body, timestamp) {
			t.Fatalf("exact standard timestamp %q missing:\n%s", timestamp, body)
		}
	}
	path := filepath.Join(t.TempDir(), "exact-nanosecond-standard-span.systrace")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
	if len(stats.TraceSpans) != 1 ||
		stats.TraceSpans[0].Name != "sub-us-official-viewer" ||
		math.Abs(stats.TraceSpans[0].DurationMs-0.0005) > 0.0000001 {
		t.Fatalf("exact standard B/E span did not round-trip: %+v", stats.TraceSpans)
	}
}

func TestTraceDBWireIntervalRepresentableAtNanosecondBoundary(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, end int64
		want       bool
	}{
		{name: "zero duration", start: 0, end: 0, want: false},
		{name: "one nanosecond", start: 0, end: 1, want: true},
		{name: "sub microsecond same former bucket", start: 0, end: 499, want: true},
		{name: "sub microsecond crosses former rounding boundary", start: 499, end: 500, want: true},
		{name: "999ns crosses former rounding boundary", start: 499, end: 1498, want: true},
		{name: "one microsecond at rounding boundary", start: 499, end: 1499, want: true},
		{name: "one microsecond aligned", start: 1000, end: 2000, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := traceDBWireIntervalRepresentable(tc.start, tc.end); got != tc.want {
				t.Fatalf("traceDBWireIntervalRepresentable(%d,%d)=%v, want %v", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

func TestTraceDBStaticInitializeIncompleteIdentitySkipsLocally(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 501, 1, 'worker', 0, 0, 1)",
		"CREATE TABLE static_initalize (start_time INT, end_time INT, so_name TEXT, ipid INT, tid INT)",
		"INSERT INTO static_initalize VALUES (1, 2, 'valid.so', 1, 501)",
		"INSERT INTO static_initalize VALUES (3, 4, 'missing-process.so', 0, 502)",
		"INSERT INTO static_initalize VALUES (5, 6, 'missing-thread.so', 1, 0)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := exportTraceDBStaticInitialize(context.Background(), tdb, sink, syncSpans, index)
	if err != nil {
		t.Fatalf("incomplete producer identity should be a row-local skip: %v", err)
	}
	items, _, _ := finalizeTraceDBTestSyncSpans(t, sink, syncSpans, []TraceDBCoverage{coverage})
	coverage = items[0]
	if coverage.RowsEmitted != 2 || len(sink.rows) != 2 ||
		!strings.Contains(coverage.Skipped, "invalid_emitter_tid=1") ||
		!strings.Contains(coverage.Skipped, "unresolved_owner_process=1") {
		t.Fatalf("unexpected static-init identity account: coverage=%+v rows=%+v", coverage, sink.rows)
	}
}

func TestTraceDBEndpointZeroTimeRoundTrip(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := addTraceDBInstantRow(sink, 0, "task", 1, 1, 0, "tracing_mark_write: I|1|zero"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "zero.systrace")
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.prepareAndWriteForTest(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range idx.Events {
		if event.Type == tracequery.EventTraceMark && event.SpanAction == "I" && event.SpanPID == 1 && event.SpanName == "zero" && event.Ts == 0 {
			return
		}
	}
	t.Fatalf("t=0 instant did not survive round trip: %+v", idx.Events)
}

func TestTraceDBFormatterHasOneValidatedProductionCaller(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	dir := filepath.Dir(current)
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	callFile := ""
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if ok && ident.Name == "traceDBFormatLine" {
				calls++
				callFile = filepath.Base(path)
			}
			return true
		})
	}
	if calls != 1 || callFile != "streamerdb_endpoint.go" {
		t.Fatalf("traceDBFormatLine must only be called by the validated primitive, calls=%d file=%q", calls, callFile)
	}
}
