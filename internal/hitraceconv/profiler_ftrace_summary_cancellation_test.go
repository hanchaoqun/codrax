package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const profilerSummaryMetadataLongBytes = 4*profilerContextByteCheckpointBytes + 31

type profilerSummaryMetadataTimeSpec struct {
	sec  uint64
	nsec uint64
}

type profilerSummaryMetadataCancelContext struct {
	context.Context
	targetSuffix   string
	ancestorSuffix string
	cancelAt       int
	polls          int
	err            error
}

func (ctx *profilerSummaryMetadataCancelContext) Err() error {
	if ctx == nil {
		return context.Canceled
	}
	var callers [64]uintptr
	frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers[:])])
	targetFound := false
	ancestorFound := ctx.ancestorSuffix == ""
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ctx.targetSuffix) {
			targetFound = true
		}
		if ctx.ancestorSuffix != "" && strings.HasSuffix(frame.Function, ctx.ancestorSuffix) {
			ancestorFound = true
		}
		if !more {
			break
		}
	}
	if targetFound && ancestorFound {
		ctx.polls++
		if ctx.err != nil && ctx.cancelAt > 0 && ctx.polls >= ctx.cancelAt {
			return ctx.err
		}
	}
	if ctx.Context != nil {
		return ctx.Context.Err()
	}
	return nil
}

func profilerSummaryMetadataStatsPayload(clock []byte) []byte {
	return protoPayload(protoVarint(1, 1), protoBytes(3, clock))
}

func profilerSummaryMetadataSymbolPayload(name []byte) []byte {
	return protoPayload(protoVarint(1, 0x1234), protoBytes(2, name))
}

func profilerSummaryMetadataTimeSpecPayload(sec, nsec uint64) []byte {
	return protoPayload(protoVarint(1, sec), protoVarint(2, nsec))
}

func profilerSummaryMetadataClockPayload(timePayload, resPayload []byte) []byte {
	parts := [][]byte{protoVarint(1, 4)}
	if timePayload != nil {
		parts = append(parts, protoBytes(2, timePayload))
	}
	if resPayload != nil {
		parts = append(parts, protoBytes(3, resPayload))
	}
	return protoPayload(parts...)
}

func profilerSummaryMetadataCommPayload(comm []byte) []byte {
	return protoPayload(protoVarint(1, 7), protoBytes(2, comm))
}

func profilerSummaryMetadataDeepTimeSpec(sec, nsec uint64) []byte {
	unknown := bytes.Repeat(protoVarint(127, 1), 4_096)
	return protoPayload(unknown, protoVarint(1, sec), protoVarint(2, nsec))
}

func profilerSummaryMetadataGoodTopFields() [][]byte {
	return [][]byte{
		protoBytes(1, profilerSummaryMetadataStatsPayload([]byte("boot"))),
		protoBytes(5, profilerSummaryMetadataSymbolPayload([]byte("schedule"))),
		protoBytes(6, profilerSummaryMetadataClockPayload(
			profilerSummaryMetadataTimeSpecPayload(10, 20),
			profilerSummaryMetadataTimeSpecPayload(0, 1))),
		protoBytes(7, []byte("trace-plugin-v1")),
		protoBytes(8, profilerSummaryMetadataCommPayload([]byte("worker"))),
	}
}

func profilerSummaryMetadataResult(parts ...[]byte) profilerTracePluginResult {
	return decodeProfilerTracePluginResult(protoPayload(parts...))
}

func profilerSummaryMetadataErrorsMatch(left, right error) bool {
	if left == nil || right == nil {
		return left == right
	}
	return reflect.TypeOf(left) == reflect.TypeOf(right) && left.Error() == right.Error()
}

func TestProfilerSummaryMetadataContextWrappersPreserveParity(t *testing.T) {
	exact := bytes.Repeat([]byte{'x'}, maxTraceDBSystraceLineBytes)
	over := append(append([]byte(nil), exact...), 'x')
	tests := []struct {
		name    string
		wantErr bool
		legacy  func() (any, error)
		withCtx func(context.Context) (any, error)
	}{
		{
			name: "cpu-stats-legal",
			legacy: func() (any, error) {
				return decodeProfilerFtraceCPUStats(profilerSummaryMetadataStatsPayload([]byte("boot")))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return decodeProfilerFtraceCPUStatsContext(ctx, profilerSummaryMetadataStatsPayload([]byte("boot")))
			},
		},
		{
			name: "cpu-stats-exact-cap",
			legacy: func() (any, error) {
				return decodeProfilerFtraceCPUStats(profilerSummaryMetadataStatsPayload(exact))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return decodeProfilerFtraceCPUStatsContext(ctx, profilerSummaryMetadataStatsPayload(exact))
			},
		},
		{
			name: "cpu-stats-over-cap", wantErr: true,
			legacy: func() (any, error) {
				return decodeProfilerFtraceCPUStats(profilerSummaryMetadataStatsPayload(over))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return decodeProfilerFtraceCPUStatsContext(ctx, profilerSummaryMetadataStatsPayload(over))
			},
		},
		{
			name: "symbol-legal",
			legacy: func() (any, error) {
				return decodeProfilerFtraceSymbolDetail(profilerSummaryMetadataSymbolPayload([]byte("schedule")))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return decodeProfilerFtraceSymbolDetailContext(ctx, profilerSummaryMetadataSymbolPayload([]byte("schedule")))
			},
		},
		{
			name: "symbol-exact-cap",
			legacy: func() (any, error) {
				return decodeProfilerFtraceSymbolDetail(profilerSummaryMetadataSymbolPayload(exact))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return decodeProfilerFtraceSymbolDetailContext(ctx, profilerSummaryMetadataSymbolPayload(exact))
			},
		},
		{
			name: "symbol-over-cap", wantErr: true,
			legacy: func() (any, error) {
				return decodeProfilerFtraceSymbolDetail(profilerSummaryMetadataSymbolPayload(over))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return decodeProfilerFtraceSymbolDetailContext(ctx, profilerSummaryMetadataSymbolPayload(over))
			},
		},
		{
			name: "clock-legal",
			legacy: func() (any, error) {
				return decodeProfilerFtraceClockDetail(profilerSummaryMetadataClockPayload(
					profilerSummaryMetadataTimeSpecPayload(10, 20), profilerSummaryMetadataTimeSpecPayload(0, 1)))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return decodeProfilerFtraceClockDetailContext(ctx, profilerSummaryMetadataClockPayload(
					profilerSummaryMetadataTimeSpecPayload(10, 20), profilerSummaryMetadataTimeSpecPayload(0, 1)))
			},
		},
		{
			name: "clock-malformed", wantErr: true,
			legacy: func() (any, error) {
				return decodeProfilerFtraceClockDetail(profilerSummaryMetadataClockPayload(
					profilerSummaryMetadataTimeSpecPayload(10, 1_000_000_000), nil))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return decodeProfilerFtraceClockDetailContext(ctx, profilerSummaryMetadataClockPayload(
					profilerSummaryMetadataTimeSpecPayload(10, 1_000_000_000), nil))
			},
		},
		{
			name: "timespec-legal",
			legacy: func() (any, error) {
				sec, nsec, err := decodeProfilerFtraceTimeSpec(profilerSummaryMetadataTimeSpecPayload(10, 20))
				return profilerSummaryMetadataTimeSpec{sec: sec, nsec: nsec}, err
			},
			withCtx: func(ctx context.Context) (any, error) {
				sec, nsec, err := decodeProfilerFtraceTimeSpecContext(ctx, profilerSummaryMetadataTimeSpecPayload(10, 20))
				return profilerSummaryMetadataTimeSpec{sec: sec, nsec: nsec}, err
			},
		},
		{
			name: "timespec-malformed", wantErr: true,
			legacy: func() (any, error) {
				sec, nsec, err := decodeProfilerFtraceTimeSpec(profilerSummaryMetadataTimeSpecPayload(10, 1_000_000_000))
				return profilerSummaryMetadataTimeSpec{sec: sec, nsec: nsec}, err
			},
			withCtx: func(ctx context.Context) (any, error) {
				sec, nsec, err := decodeProfilerFtraceTimeSpecContext(ctx, profilerSummaryMetadataTimeSpecPayload(10, 1_000_000_000))
				return profilerSummaryMetadataTimeSpec{sec: sec, nsec: nsec}, err
			},
		},
		{
			name: "comm-legal",
			legacy: func() (any, error) {
				return struct{}{}, decodeProfilerFtraceCommDict(profilerSummaryMetadataCommPayload([]byte("worker")))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return struct{}{}, decodeProfilerFtraceCommDictContext(ctx, profilerSummaryMetadataCommPayload([]byte("worker")))
			},
		},
		{
			name: "comm-exact-cap",
			legacy: func() (any, error) {
				return struct{}{}, decodeProfilerFtraceCommDict(profilerSummaryMetadataCommPayload(exact))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return struct{}{}, decodeProfilerFtraceCommDictContext(ctx, profilerSummaryMetadataCommPayload(exact))
			},
		},
		{
			name: "comm-over-cap", wantErr: true,
			legacy: func() (any, error) {
				return struct{}{}, decodeProfilerFtraceCommDict(profilerSummaryMetadataCommPayload(over))
			},
			withCtx: func(ctx context.Context) (any, error) {
				return struct{}{}, decodeProfilerFtraceCommDictContext(ctx, profilerSummaryMetadataCommPayload(over))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacyValue, legacyErr := test.legacy()
			backgroundValue, backgroundErr := test.withCtx(context.Background())
			nilValue, nilErr := test.withCtx(nil)
			if !reflect.DeepEqual(backgroundValue, legacyValue) ||
				!profilerSummaryMetadataErrorsMatch(backgroundErr, legacyErr) ||
				!reflect.DeepEqual(nilValue, legacyValue) ||
				!profilerSummaryMetadataErrorsMatch(nilErr, legacyErr) {
				t.Fatalf("legacy/Background/nil parity drifted:\nlegacy=%+v err=%T %v\nbackground=%+v err=%T %v\nnil=%+v err=%T %v",
					legacyValue, legacyErr, legacyErr, backgroundValue, backgroundErr, backgroundErr,
					nilValue, nilErr, nilErr)
			}
			if (legacyErr != nil) != test.wantErr {
				t.Fatalf("error presence=%t want=%t err=%v", legacyErr != nil, test.wantErr, legacyErr)
			}
		})
	}
}

func TestProfilerStableSampleStringPartsParityAndCancellation(t *testing.T) {
	manyParts := make([]string, 300)
	manyParts[0] = "head"
	manyParts[255] = "middle"
	manyParts[299] = "tail"
	tests := []struct {
		domain string
		parts  []string
	}{
		{domain: "empty"},
		{domain: "one-empty", parts: []string{""}},
		{domain: "symbol", parts: []string{"0x", "1234", "=", "schedule"}},
		{domain: "utf8", parts: []string{"时", "钟", "=", "单调"}},
		{domain: "large", parts: []string{
			strings.Repeat("a", profilerContextByteCheckpointBytes+7),
			strings.Repeat("b", 2*profilerContextByteCheckpointBytes+11),
		}},
		{domain: "many-parts", parts: manyParts},
	}
	var got, want profilerStableSampleSet
	for _, test := range tests {
		if err := got.observeStringPartsContext(context.Background(), test.domain, test.parts...); err != nil {
			t.Fatalf("parts domain=%q: %v", test.domain, err)
		}
		want.observe(test.domain, []byte(strings.Join(test.parts, "")))
		if got != want {
			t.Fatalf("segmented sample parity drifted at domain=%q:\ngot=%+v\nwant=%+v", test.domain, got, want)
		}
	}

	var seeded profilerStableSampleSet
	seeded.observe("seed", []byte("existing"))
	before := seeded
	large := strings.Repeat("z", profilerSummaryMetadataLongBytes)
	mid := &profilerByteCancelAfterPollContext{Context: context.Background(), cancelAt: 4, err: context.Canceled}
	if err := seeded.observeStringPartsContext(mid, "symbol", "0x", "1234", "=", large); err != context.Canceled ||
		seeded != before || mid.polls < mid.cancelAt {
		t.Fatalf("mid segmented cancellation was not atomic: polls=%d err=%T %v mutated=%t",
			mid.polls, err, err, seeded != before)
	}
	final := &profilerByteCancelAfterPollContext{Context: context.Background(), cancelAt: 3, err: context.DeadlineExceeded}
	if err := seeded.observeStringPartsContext(final, "symbol", "short"); err != context.DeadlineExceeded ||
		seeded != before || final.polls < final.cancelAt {
		t.Fatalf("final segmented cancellation was not atomic: polls=%d err=%T %v mutated=%t",
			final.polls, err, err, seeded != before)
	}
	emptyParts := make([]string, 300)
	occurrence := &profilerByteCancelAfterPollContext{Context: context.Background(), cancelAt: 3, err: context.Canceled}
	if err := seeded.observeStringPartsContext(occurrence, "symbol", emptyParts...); err != context.Canceled ||
		seeded != before || occurrence.polls < occurrence.cancelAt {
		t.Fatalf("segmented occurrence cancellation was not atomic: polls=%d err=%T %v mutated=%t",
			occurrence.polls, err, err, seeded != before)
	}

	name := strings.Repeat("s", profilerSummaryMetadataLongBytes)
	summary, recognized, err := decodeProfilerFtraceSummaryResultContext(context.Background(),
		profilerSummaryMetadataResult(protoBytes(5, profilerSummaryMetadataSymbolPayload([]byte(name)))))
	if err != nil || !recognized || summary.SymbolCount != 1 || summary.SymbolNamedCount != 1 {
		t.Fatalf("nonzero-address symbol summary failed: recognized=%t err=%v summary=%+v", recognized, err, summary)
	}
	var expected profilerStableSampleSet
	expected.observe("profiler-ftrace-summary-symbol", []byte("0x1234="+name))
	if summary.SymbolSamples != expected {
		t.Fatalf("nonzero-address segmented symbol sample drifted:\ngot=%+v\nwant=%+v", summary.SymbolSamples, expected)
	}
}

func TestProfilerSummaryMetadataMalformedSiblingLocality(t *testing.T) {
	good := profilerSummaryMetadataGoodTopFields()
	clean, recognized, err := decodeProfilerFtraceSummaryResultContext(context.Background(), profilerSummaryMetadataResult(good...))
	if err != nil || !recognized || !clean.Issues.empty() || clean.IssueOverflow {
		t.Fatalf("clean metadata control failed: recognized=%t err=%v summary=%+v", recognized, err, clean)
	}
	tests := []struct {
		name  string
		index int
		bad   []byte
		issue profilerFtraceSummaryIssueKind
	}{
		{name: "cpu-stats", index: 0, bad: protoBytes(1, profilerSummaryMetadataStatsPayload([]byte("bad\nclock"))), issue: profilerFtraceSummaryIssueCPUStatsMalformed},
		{name: "symbol", index: 1, bad: protoBytes(5, profilerSummaryMetadataSymbolPayload([]byte("bad\nsymbol"))), issue: profilerFtraceSummaryIssueSymbolMalformed},
		{name: "clock", index: 2, bad: protoBytes(6, profilerSummaryMetadataClockPayload(profilerSummaryMetadataTimeSpecPayload(10, 1_000_000_000), nil)), issue: profilerFtraceSummaryIssueClockMalformed},
		{name: "version", index: 3, bad: protoBytes(7, []byte("bad\nversion")), issue: profilerFtraceSummaryIssueVersionInvalid},
		{name: "comm", index: 4, bad: protoBytes(8, profilerSummaryMetadataCommPayload([]byte("bad\ncomm"))), issue: profilerFtraceSummaryIssueCommMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parts := append([][]byte(nil), good...)
			parts[test.index] = test.bad
			result := profilerSummaryMetadataResult(parts...)
			summary, gotRecognized, gotErr := decodeProfilerFtraceSummaryResultContext(context.Background(), result)
			legacy, legacyRecognized, legacyErr := decodeProfilerFtraceSummaryResult(result)
			if gotErr != nil || !gotRecognized || legacyErr != nil || !legacyRecognized || !reflect.DeepEqual(summary, legacy) {
				t.Fatalf("malformed sibling did not retain Background parity: recognized=%t/%t err=%v/%v\ncontext=%+v\nlegacy=%+v",
					gotRecognized, legacyRecognized, gotErr, legacyErr, summary, legacy)
			}
			for kind := profilerFtraceSummaryIssueKind(0); kind < profilerFtraceSummaryIssueKindCount; kind++ {
				want := uint64(0)
				if kind == test.issue {
					want = 1
				}
				if summary.Issues.Occurrences[int(kind)] != want || summary.Issues.AffectedFrames[int(kind)] != want {
					t.Fatalf("issue locality drifted at %s: occurrences=%d affected=%d want=%d summary=%+v",
						kind.label(), summary.Issues.Occurrences[int(kind)], summary.Issues.AffectedFrames[int(kind)], want, summary)
				}
			}
			wantStats, wantSymbol, wantClock, wantVersion := uint64(1), uint64(1), uint64(1), uint64(1)
			switch test.issue {
			case profilerFtraceSummaryIssueCPUStatsMalformed:
				wantStats = 0
			case profilerFtraceSummaryIssueSymbolMalformed:
				wantSymbol = 0
			case profilerFtraceSummaryIssueClockMalformed:
				wantClock = 0
			case profilerFtraceSummaryIssueVersionInvalid:
				wantVersion = 0
			}
			if summary.IssueOverflow || summary.StatsMessages != wantStats || summary.EndStats != wantStats ||
				summary.TraceClockObserved != wantStats || summary.SymbolCount != wantSymbol ||
				summary.SymbolNamedCount != wantSymbol || summary.ClockDetailCount != wantClock ||
				summary.VersionObservations != wantVersion {
				t.Fatalf("malformed field contaminated sibling counters: %+v", summary)
			}
			if (test.issue != profilerFtraceSummaryIssueCPUStatsMalformed && summary.TraceClockSamples != clean.TraceClockSamples) ||
				(test.issue == profilerFtraceSummaryIssueCPUStatsMalformed && summary.TraceClockSamples.Used != 0) ||
				(test.issue != profilerFtraceSummaryIssueSymbolMalformed && summary.SymbolSamples != clean.SymbolSamples) ||
				(test.issue == profilerFtraceSummaryIssueSymbolMalformed && summary.SymbolSamples.Used != 0) ||
				(test.issue != profilerFtraceSummaryIssueClockMalformed && summary.ClockDetailSamples != clean.ClockDetailSamples) ||
				(test.issue == profilerFtraceSummaryIssueClockMalformed && summary.ClockDetailSamples.Used != 0) ||
				(test.issue != profilerFtraceSummaryIssueVersionInvalid && summary.VersionSamples != clean.VersionSamples) ||
				(test.issue == profilerFtraceSummaryIssueVersionInvalid && summary.VersionSamples.Used != 0) {
				t.Fatalf("malformed field contaminated sibling samples:\nclean=%+v\ngot=%+v", clean, summary)
			}
		})
	}
}

func TestProfilerSummaryMetadataMalformedSameFieldSiblingLocality(t *testing.T) {
	goodSymbol := protoBytes(5, profilerSummaryMetadataSymbolPayload([]byte("schedule")))
	badSymbol := protoBytes(5, profilerSummaryMetadataSymbolPayload([]byte("bad\nsymbol")))
	goodClock := protoBytes(6, profilerSummaryMetadataClockPayload(
		profilerSummaryMetadataTimeSpecPayload(10, 20), profilerSummaryMetadataTimeSpecPayload(0, 1)))
	badClock := protoBytes(6, profilerSummaryMetadataClockPayload(
		profilerSummaryMetadataTimeSpecPayload(10, 1_000_000_000), nil))
	goodComm := protoBytes(8, profilerSummaryMetadataCommPayload([]byte("worker")))
	badComm := protoBytes(8, profilerSummaryMetadataCommPayload([]byte("bad\ncomm")))
	tests := []struct {
		name       string
		good       []byte
		bad        []byte
		issue      profilerFtraceSummaryIssueKind
		assertGood func(*testing.T, profilerFtraceSummary)
	}{
		{
			name: "symbol", good: goodSymbol, bad: badSymbol, issue: profilerFtraceSummaryIssueSymbolMalformed,
			assertGood: func(t *testing.T, summary profilerFtraceSummary) {
				if summary.SymbolCount != 1 || summary.SymbolNamedCount != 1 || summary.SymbolSamples.Used != 1 {
					t.Fatalf("legal symbol sibling was starved: %+v", summary)
				}
			},
		},
		{
			name: "clock", good: goodClock, bad: badClock, issue: profilerFtraceSummaryIssueClockMalformed,
			assertGood: func(t *testing.T, summary profilerFtraceSummary) {
				if summary.ClockDetailCount != 1 || summary.ClockDetailSamples.Used != 1 {
					t.Fatalf("legal clock sibling was starved: %+v", summary)
				}
			},
		},
		{
			name: "comm", good: goodComm, bad: badComm, issue: profilerFtraceSummaryIssueCommMalformed,
			assertGood: func(t *testing.T, summary profilerFtraceSummary) {
				// CommDict intentionally has no positive disclosure surface. Reaching
				// the legal second decoder is proved below with its Context poll census.
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var first profilerFtraceSummary
			for order, parts := range [][][]byte{{test.bad, test.good}, {test.good, test.bad}} {
				summary, recognized, err := decodeProfilerFtraceSummaryResultContext(context.Background(),
					profilerSummaryMetadataResult(parts...))
				if err != nil || !recognized || summary.IssueOverflow ||
					summary.Issues.Occurrences[int(test.issue)] != 1 ||
					summary.Issues.AffectedFrames[int(test.issue)] != 1 {
					t.Fatalf("order=%d same-field locality failed: recognized=%t err=%v summary=%+v",
						order, recognized, err, summary)
				}
				for kind := profilerFtraceSummaryIssueKind(0); kind < profilerFtraceSummaryIssueKindCount; kind++ {
					if kind != test.issue && summary.Issues.Occurrences[int(kind)] != 0 {
						t.Fatalf("order=%d same-field issue leaked to %s: %+v", order, kind.label(), summary)
					}
				}
				test.assertGood(t, summary)
				if order == 0 {
					first = summary
				} else if !reflect.DeepEqual(summary, first) {
					t.Fatalf("same-field physical order changed fixed summary:\nbad-good=%+v\ngood-bad=%+v", first, summary)
				}
			}
			if test.name == "comm" {
				badOnlyCtx := &profilerSummaryMetadataCancelContext{
					Context: context.Background(), targetSuffix: ".decodeProfilerFtraceCommDictContext",
				}
				_, _, err := decodeProfilerFtraceSummaryResultContext(badOnlyCtx,
					profilerSummaryMetadataResult(test.bad))
				if err != nil {
					t.Fatalf("bad-only comm calibration: %v", err)
				}
				badGoodCtx := &profilerSummaryMetadataCancelContext{
					Context: context.Background(), targetSuffix: ".decodeProfilerFtraceCommDictContext",
				}
				_, _, err = decodeProfilerFtraceSummaryResultContext(badGoodCtx,
					profilerSummaryMetadataResult(test.bad, test.good))
				if err != nil || badGoodCtx.polls <= badOnlyCtx.polls {
					t.Fatalf("legal comm sibling was not decoded after malformed peer: bad=%d bad+good=%d err=%v",
						badOnlyCtx.polls, badGoodCtx.polls, err)
				}
			}
		})
	}
}

func profilerSummaryMetadataResultWithEvent() profilerTracePluginResult {
	detail := protoPayload(
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 7, 7, "worker", 1109,
			protoBytes(2, []byte("I|7|pre-cancel-must-not-visit"))),
	)
	parts := append([][]byte{protoBytes(2, detail)}, profilerSummaryMetadataGoodTopFields()...)
	return profilerSummaryMetadataResult(parts...)
}

func TestProfilerSummaryMetadataPreCancellationIsZero(t *testing.T) {
	tests := []struct {
		name string
		want error
		make func() (context.Context, context.CancelFunc)
	}{
		{
			name: "canceled", want: context.Canceled,
			make: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
		},
		{
			name: "deadline", want: context.DeadlineExceeded,
			make: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Unix(1, 0))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.make()
			defer cancel()
			visits := 0
			summary, recognized, err := consumeProfilerTracePluginResultContext(ctx,
				profilerSummaryMetadataResultWithEvent(), true, func(profilerFtraceEventRecord) error {
					visits++
					return nil
				})
			if err != test.want || recognized || !reflect.DeepEqual(summary, profilerFtraceSummary{}) || visits != 0 {
				t.Fatalf("pre-cancellation leaked work: visits=%d recognized=%t err=%T %v want=%v summary=%+v",
					visits, recognized, err, err, test.want, summary)
			}
		})
	}
}

func TestProfilerSummaryMetadataSuccessfulTailCancellationIsZero(t *testing.T) {
	results := []struct {
		name   string
		result profilerTracePluginResult
	}{
		{name: "structured", result: profilerTracePluginResult{Disposition: profilerFtracePayloadStructured}},
		{name: "not-structured", result: profilerTracePluginResult{Disposition: profilerFtracePayloadNotStructured}},
		{name: "malformed", result: profilerTracePluginResult{Disposition: profilerFtracePayloadMalformed}},
	}
	for _, test := range results {
		for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
			name := test.name + "/canceled"
			if want == context.DeadlineExceeded {
				name = test.name + "/deadline"
			}
			t.Run(name, func(t *testing.T) {
				ctx := &profilerSummaryMetadataCancelContext{
					Context: context.Background(), targetSuffix: ".consumeProfilerTracePluginResultContext",
					cancelAt: 2, err: want,
				}
				summary, recognized, err := consumeProfilerTracePluginResultContext(ctx, test.result, true, nil)
				if err != want || recognized || !reflect.DeepEqual(summary, profilerFtraceSummary{}) || ctx.polls != ctx.cancelAt {
					t.Fatalf("successful tail cancellation escaped: polls=%d/%d recognized=%t err=%T %v want=%v summary=%+v",
						ctx.polls, ctx.cancelAt, recognized, err, err, want, summary)
				}
			})
		}
	}

	for _, test := range []struct {
		name     string
		payload  []byte
		cancelAt int
		visits   int
	}{
		{name: "empty", cancelAt: 2},
		{name: "short", payload: protoVarint(1, 1), cancelAt: 3, visits: 1},
	} {
		t.Run("walker/"+test.name, func(t *testing.T) {
			ctx := &profilerByteCancelAfterPollContext{
				Context: context.Background(), cancelAt: test.cancelAt, err: context.DeadlineExceeded,
			}
			visits := 0
			err := walkProfilerProtoFieldsContext(ctx, test.payload, func(int, int, []byte, uint64) error {
				visits++
				return nil
			})
			if err != context.DeadlineExceeded || visits != test.visits || ctx.polls != test.cancelAt {
				t.Fatalf("walker tail cancellation escaped: visits=%d/%d polls=%d/%d err=%T %v",
					visits, test.visits, ctx.polls, test.cancelAt, err, err)
			}
		})
	}
}

type profilerSummaryMetadataMidCase struct {
	name             string
	result           profilerTracePluginResult
	targetSuffix     string
	ancestorSuffix   string
	cancelAt         int
	prefixPollResult *profilerTracePluginResult
}

func profilerSummaryMetadataTargetPolls(t *testing.T, test profilerSummaryMetadataMidCase, result profilerTracePluginResult) int {
	t.Helper()
	ctx := &profilerSummaryMetadataCancelContext{
		Context: context.Background(), targetSuffix: test.targetSuffix, ancestorSuffix: test.ancestorSuffix,
	}
	summary, recognized, err := decodeProfilerFtraceSummaryResultContext(ctx, result)
	if err != nil || !recognized || ctx.polls == 0 || summary.IssueOverflow || !summary.Issues.empty() {
		t.Fatalf("calibrate %s: polls=%d recognized=%t err=%v summary=%+v", test.name, ctx.polls, recognized, err, summary)
	}
	return ctx.polls
}

func TestProfilerSummaryMetadataMidCancellationIsProspective(t *testing.T) {
	longToken := bytes.Repeat([]byte{'c'}, profilerSummaryMetadataLongBytes)
	longLine := bytes.Repeat([]byte{'s'}, profilerSummaryMetadataLongBytes)
	shortTime := profilerSummaryMetadataTimeSpecPayload(10, 20)
	deepTime := profilerSummaryMetadataDeepTimeSpec(10, 20)
	deepRes := profilerSummaryMetadataDeepTimeSpec(0, 1)
	resPrefix := profilerSummaryMetadataResult(protoBytes(6,
		profilerSummaryMetadataClockPayload(shortTime, nil)))
	tests := []profilerSummaryMetadataMidCase{
		{name: "cpu-clock-validate", result: profilerSummaryMetadataResult(protoBytes(1, profilerSummaryMetadataStatsPayload(longToken))), targetSuffix: ".profilerPhysicalRuneFactsBytesContext", ancestorSuffix: ".decodeProfilerFtraceCPUStatsContext"},
		{name: "cpu-clock-clone", result: profilerSummaryMetadataResult(protoBytes(1, profilerSummaryMetadataStatsPayload(longToken))), targetSuffix: ".profilerCloneBytesStringContext", ancestorSuffix: ".decodeProfilerFtraceCPUStatsContext"},
		{name: "cpu-clock-sample", result: profilerSummaryMetadataResult(protoBytes(1, profilerSummaryMetadataStatsPayload(longToken))), targetSuffix: ".observeStringPartsContext", ancestorSuffix: ".consumeProfilerTracePluginResultContext"},
		{name: "symbol-validate", result: profilerSummaryMetadataResult(protoBytes(5, profilerSummaryMetadataSymbolPayload(longLine))), targetSuffix: ".profilerPhysicalRuneFactsBytesContext", ancestorSuffix: ".decodeProfilerFtraceSymbolDetailContext"},
		{name: "symbol-clone", result: profilerSummaryMetadataResult(protoBytes(5, profilerSummaryMetadataSymbolPayload(longLine))), targetSuffix: ".profilerCloneBytesStringContext", ancestorSuffix: ".decodeProfilerFtraceSymbolDetailContext"},
		{name: "symbol-sample", result: profilerSummaryMetadataResult(protoBytes(5, profilerSummaryMetadataSymbolPayload(longLine))), targetSuffix: ".observeStringPartsContext", ancestorSuffix: ".consumeProfilerTracePluginResultContext"},
		{name: "clock-time", result: profilerSummaryMetadataResult(protoBytes(6, profilerSummaryMetadataClockPayload(deepTime, nil))), targetSuffix: ".decodeProfilerFtraceTimeSpecContext", ancestorSuffix: ".decodeProfilerFtraceClockDetailContext"},
		{name: "clock-res", result: profilerSummaryMetadataResult(protoBytes(6, profilerSummaryMetadataClockPayload(shortTime, deepRes))), targetSuffix: ".decodeProfilerFtraceTimeSpecContext", ancestorSuffix: ".decodeProfilerFtraceClockDetailContext", prefixPollResult: &resPrefix},
		{name: "clock-sample", result: profilerSummaryMetadataResult(protoBytes(6, profilerSummaryMetadataClockPayload(shortTime, profilerSummaryMetadataTimeSpecPayload(0, 1)))), targetSuffix: ".observeStringPartsContext", ancestorSuffix: ".consumeProfilerTracePluginResultContext"},
		{name: "version-validate", result: profilerSummaryMetadataResult(protoBytes(7, longLine)), targetSuffix: ".profilerPhysicalRuneFactsBytesContext", ancestorSuffix: ".consumeProfilerTracePluginResultContext"},
		{name: "version-sample", result: profilerSummaryMetadataResult(protoBytes(7, longLine)), targetSuffix: ".observeContext", ancestorSuffix: ".consumeProfilerTracePluginResultContext"},
		{name: "comm-validate", result: profilerSummaryMetadataResult(protoBytes(8, profilerSummaryMetadataCommPayload(longLine))), targetSuffix: ".profilerPhysicalRuneFactsBytesContext", ancestorSuffix: ".decodeProfilerFtraceCommDictContext"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseline := profilerSummaryMetadataTargetPolls(t, test, test.result)
			cancelAt := test.cancelAt
			if cancelAt == 0 {
				cancelAt = max(2, (baseline+1)/2)
			}
			if test.prefixPollResult != nil {
				prefix := profilerSummaryMetadataTargetPolls(t, test, *test.prefixPollResult)
				if baseline <= prefix {
					t.Fatalf("res fixture did not add a second nested context path: prefix=%d total=%d", prefix, baseline)
				}
				cancelAt = prefix + max(1, (baseline-prefix)/2)
			}
			if cancelAt > baseline {
				t.Fatalf("cancelAt=%d exceeds calibrated polls=%d", cancelAt, baseline)
			}
			for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
				name := "canceled"
				if want == context.DeadlineExceeded {
					name = "deadline"
				}
				t.Run(name, func(t *testing.T) {
					ctx := &profilerSummaryMetadataCancelContext{
						Context: context.Background(), targetSuffix: test.targetSuffix,
						ancestorSuffix: test.ancestorSuffix, cancelAt: cancelAt, err: want,
					}
					summary, recognized, err := decodeProfilerFtraceSummaryResultContext(ctx, test.result)
					if err != want || recognized || !reflect.DeepEqual(summary, profilerFtraceSummary{}) || ctx.polls < cancelAt {
						t.Fatalf("mid-cancellation escaped prospectively: polls=%d/%d recognized=%t err=%T %v want=%v summary=%+v",
							ctx.polls, cancelAt, recognized, err, err, want, summary)
					}
				})
			}
		})
	}
}

func profilerSummaryMetadataLateCancellationFixture() []byte {
	detail := protoPayload(
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 7, 7, "worker", 1109,
			protoBytes(2, []byte("I|7|staged-before-summary-cancel"))),
	)
	longName := bytes.Repeat([]byte{'s'}, profilerSummaryMetadataLongBytes)
	structured := protoPayload(
		protoBytes(2, detail),
		protoBytes(5, profilerSummaryMetadataSymbolPayload(longName)),
	)
	return syntheticProfilerTraceFile(syntheticProfilerPluginData("ftrace-plugin", structured))
}

func TestProfilerSummaryMetadataLateCancellationRollsBackConvertFile(t *testing.T) {
	body := profilerSummaryMetadataLateCancellationFixture()
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		name := "canceled"
		if want == context.DeadlineExceeded {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "summary-cancel.htrace")
			output := filepath.Join(dir, "summary-cancel.systrace")
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			header, ok, err := readProfilerTraceHeaderAtPath(input, 0, before.Size())
			if err != nil || !ok {
				t.Fatalf("read summary-cancel header: ok=%t err=%v", ok, err)
			}

			probeSink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			probeCtx := &profilerSummaryMetadataCancelContext{
				Context: context.Background(), targetSuffix: ".observeStringPartsContext",
				ancestorSuffix: ".consumeProfilerTracePluginResultContext", cancelAt: 3, err: want,
			}
			extracted, probeErr := extractProfilerTraceFileWithFrameLimit(
				probeCtx, input, before.Size(), header, probeSink, maxProfilerPluginFrameBytes)
			accepted := probeSink.stats.RowsAccepted
			publishable := probeSink.publishableRows()
			failedClosed := probeSink.allRowsFailClosed
			pairActive := probeSink.pairCensusActive
			pairMutated := false
			for _, kind := range profilerCaptureKinds {
				pairMutated = pairMutated || probeSink.pairRows[kind] != 0 || probeSink.poisoned[kind] ||
					probeSink.opaque[kind] || probeSink.structuredPairRows[kind] != 0
			}
			cleanupErr := probeSink.cleanup()
			if probeErr != want || !reflect.DeepEqual(extracted, profilerContainerExtraction{}) ||
				probeCtx.polls < probeCtx.cancelAt || accepted != 1 || publishable != 1 || failedClosed ||
				pairActive || pairMutated || cleanupErr != nil {
				t.Fatalf("late summary cancellation did not retain exactly one private row: polls=%d/%d accepted=%d "+
					"publishable=%d fail_closed=%t pair_active=%t pair_mutated=%t extracted=%+v err=%T %v cleanup=%v want=%v",
					probeCtx.polls, probeCtx.cancelAt, accepted, publishable, failedClosed, pairActive, pairMutated,
					extracted, probeErr, probeErr, cleanupErr, want)
			}

			convertCtx := &profilerSummaryMetadataCancelContext{
				Context: context.Background(), targetSuffix: ".observeStringPartsContext",
				ancestorSuffix: ".consumeProfilerTracePluginResultContext", cancelAt: 3, err: want,
			}
			result, convertErr := ConvertFile(convertCtx, Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
			})
			if convertErr != want || !reflect.DeepEqual(result, Result{}) || convertCtx.polls < convertCtx.cancelAt {
				t.Fatalf("late summary ConvertFile cancellation drifted: polls=%d/%d result=%+v err=%T %v want=%v",
					convertCtx.polls, convertCtx.cancelAt, result, convertErr, convertErr, want)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("canceled summary conversion left output: %v", err)
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
				t.Fatalf("canceled summary conversion changed input: same=%t mode=%v/%v size=%d/%d equal=%t",
					os.SameFile(before, after), before.Mode(), after.Mode(), before.Size(), after.Size(), bytes.Equal(gotBody, body))
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
				t.Fatalf("canceled summary conversion left customer-visible artifacts: %+v", entries)
			}
		})
	}
}

func TestProfilerSummaryMetadataProductionContextTopology(t *testing.T) {
	container := mustReadRendererSource(t, "profiler_container.go")
	processor := sourceBetween(t, container,
		"func consumeProfilerTracePluginResultContext(",
		"func (summary *profilerFtraceSummary) observeDecodeIssue(")
	for call, want := range map[string]int{
		"decodeProfilerFtraceCPUStatsContext(ctx, raw)":          1,
		"decodeProfilerFtraceSymbolDetailContext(ctx, raw)":      1,
		"decodeProfilerFtraceClockDetailContext(ctx, raw)":       1,
		"decodeProfilerFtraceCommDictContext(ctx, raw)":          1,
		"profilerSinglePhysicalLineBytesContext(ctx, raw, true)": 1,
		"summary.TraceClockSamples.observeStringContext(ctx,":    1,
		"summary.SymbolSamples.observeStringPartsContext(ctx,":   1,
		"summary.SymbolSamples.observeStringContext(ctx,":        1,
		"summary.ClockDetailSamples.observeStringContext(ctx,":   1,
		"summary.VersionSamples.observeContext(ctx,":             1,
		"summary.observeDecodeIssue(profilerFtraceSummaryIssue":  4,
	} {
		if count := strings.Count(processor, call); count != want {
			t.Fatalf("summary Context call %q count=%d want=%d:\n%s", call, count, want, processor)
		}
	}
	for _, forbidden := range []string{
		"decodeProfilerFtraceCPUStats(raw)",
		"decodeProfilerFtraceSymbolDetail(raw)",
		"decodeProfilerFtraceClockDetail(raw)",
		"decodeProfilerFtraceCommDict(raw)",
		"traceDBSinglePhysicalLine(",
		"traceDBSingleToken(",
		"string(raw)",
		"fmt.Sprintf(\"0x%x=%s\"",
	} {
		if strings.Contains(processor, forbidden) {
			t.Fatalf("summary production path regained context-free/full-copy operation %q:\n%s", forbidden, processor)
		}
	}

	type wrapperPin struct {
		name        string
		start       string
		end         string
		delegation  string
		contextEnd  string
		contextPins []string
		forbidden   []string
	}
	wrapperPins := []wrapperPin{
		{
			name: "cpu-stats", start: "func decodeProfilerFtraceCPUStats(data []byte)",
			end:         "func decodeProfilerFtraceCPUStatsContext(",
			delegation:  "decodeProfilerFtraceCPUStatsContext(context.Background(), data)",
			contextEnd:  "func decodeProfilerFtracePerCPUStats(data []byte)",
			contextPins: []string{"walkProfilerProtoFieldsContext(ctx, data", "profilerSingleTokenBytesContext(ctx, clockRaw)", "profilerCloneBytesStringContext(ctx, clockRaw)"},
			forbidden:   []string{"traceDBSingleToken(", "stats.Clock = string(raw)"},
		},
		{
			name: "symbol", start: "func decodeProfilerFtraceSymbolDetail(data []byte)",
			end:         "func decodeProfilerFtraceSymbolDetailContext(",
			delegation:  "decodeProfilerFtraceSymbolDetailContext(context.Background(), data)",
			contextEnd:  "func decodeProfilerFtraceClockDetail(data []byte)",
			contextPins: []string{"walkProfilerProtoFieldsContext(ctx, data", "profilerSinglePhysicalLineBytesContext(ctx, nameRaw, true)", "profilerCloneBytesStringContext(ctx, nameRaw)"},
			forbidden:   []string{"walkProtoFields(data", "traceDBSinglePhysicalLine(", "symbol.Name = string(raw)"},
		},
		{
			name: "clock", start: "func decodeProfilerFtraceClockDetail(data []byte)",
			end:         "func decodeProfilerFtraceClockDetailContext(",
			delegation:  "decodeProfilerFtraceClockDetailContext(context.Background(), data)",
			contextEnd:  "func decodeProfilerFtraceTimeSpec(data []byte)",
			contextPins: []string{"walkProfilerProtoFieldsContext(ctx, data", "decodeProfilerFtraceTimeSpecContext(ctx, raw)"},
			forbidden:   []string{"walkProtoFields(data", "decodeProfilerFtraceTimeSpec(raw)"},
		},
		{
			name: "timespec", start: "func decodeProfilerFtraceTimeSpec(data []byte)",
			end:         "func decodeProfilerFtraceTimeSpecContext(",
			delegation:  "decodeProfilerFtraceTimeSpecContext(context.Background(), data)",
			contextEnd:  "func decodeProfilerFtraceCommDict(data []byte)",
			contextPins: []string{"walkProfilerProtoFieldsContext(ctx, data"},
			forbidden:   []string{"walkProtoFields(data"},
		},
		{
			name: "comm", start: "func decodeProfilerFtraceCommDict(data []byte)",
			end:         "func decodeProfilerFtraceCommDictContext(",
			delegation:  "decodeProfilerFtraceCommDictContext(context.Background(), data)",
			contextEnd:  "func (totals *profilerFtraceCPUTotals) add(",
			contextPins: []string{"walkProfilerProtoFieldsContext(ctx, data", "profilerSinglePhysicalLineBytesContext(ctx, commRaw, true)"},
			forbidden:   []string{"walkProtoFields(data", "traceDBSinglePhysicalLine(", "comm = string(raw)"},
		},
	}
	for _, pin := range wrapperPins {
		legacy := sourceBetween(t, container, pin.start, pin.end)
		if strings.Count(legacy, pin.delegation) != 1 || strings.Contains(legacy, "walkProtoFields") ||
			strings.Contains(legacy, "walkProfilerProtoFieldsContext") {
			t.Fatalf("%s legacy decoder is not a Background-only adapter:\n%s", pin.name, legacy)
		}
		contextBody := sourceBetween(t, container, pin.end, pin.contextEnd)
		for _, expected := range pin.contextPins {
			want := 1
			if pin.name == "clock" && expected == "decodeProfilerFtraceTimeSpecContext(ctx, raw)" {
				want = 2
			}
			if count := strings.Count(contextBody, expected); count != want {
				t.Fatalf("%s Context decoder call %q count=%d want=%d:\n%s", pin.name, expected, count, want, contextBody)
			}
		}
		for _, forbidden := range pin.forbidden {
			if strings.Contains(contextBody, forbidden) {
				t.Fatalf("%s Context decoder regained %q:\n%s", pin.name, forbidden, contextBody)
			}
		}
	}

	diagnostics := mustReadRendererSource(t, "profiler_container_diagnostics.go")
	stringWrapper := sourceBetween(t, diagnostics,
		"func (samples *profilerStableSampleSet) observeStringContext(",
		"// observeStringPartsContext")
	if strings.Count(stringWrapper, "samples.observeStringPartsContext(ctx, domain, raw)") != 1 {
		t.Fatalf("string sample wrapper lost segmented authority:\n%s", stringWrapper)
	}
	partsBody := sourceBetween(t, diagnostics,
		"func (samples *profilerStableSampleSet) observeStringPartsContext(",
		"func (samples *profilerStableSampleSet) insertDigest(")
	for _, expected := range []string{"profilerByteContextCheckpoint(ctx", "partIndex&255", "samples.insertDigest(digest, inputLen"} {
		if !strings.Contains(partsBody, expected) {
			t.Fatalf("segmented sample authority lost %q:\n%s", expected, partsBody)
		}
	}
	for _, forbidden := range []string{"strings.Join(", "fmt.Sprintf("} {
		if strings.Contains(partsBody, forbidden) {
			t.Fatalf("segmented sample authority regained full concatenation %q:\n%s", forbidden, partsBody)
		}
	}
}

var _ context.Context = (*profilerSummaryMetadataCancelContext)(nil)
