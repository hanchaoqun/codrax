package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func profilerFixedSummaryEvent(field int) []byte {
	return protoMessage(2,
		protoMessage(50, protoVarint(4, 1)),
		protoMessage(field),
	)
}

func TestProfilerFtraceFixedSummaryMillionLegalEventsRetainFixedShape(t *testing.T) {
	const occurrences = 1_000_000
	event := profilerFixedSummaryEvent(1109)
	detailPayload := append(protoVarint(1, uint64(maxTraceDBCPUIndex)), bytes.Repeat(event, occurrences)...)
	result := decodeProfilerTracePluginResult(protoBytes(2, detailPayload))
	summary, recognized, err := decodeProfilerFtraceSummaryResultContext(context.Background(), result)
	if err != nil || !recognized {
		t.Fatalf("decode million-event summary: recognized=%t err=%v", recognized, err)
	}
	if summary.DetailMessages != 1 || summary.DetailEventCount != occurrences ||
		summary.DetailCPUs.count() != 1 || !summary.DetailCPUs.contains(uint64(maxTraceDBCPUIndex)) ||
		profilerSummaryKnownEventCountForTest(t, summary, 1109) != occurrences ||
		summary.UnknownEventCount != 0 || summary.UnknownEventFieldSamples.Used != 0 {
		t.Fatalf("million-event fixed summary census drifted: %+v", summary)
	}

	for _, value := range []any{summary, mustDecodeProfilerFtraceCPUDetailForFixedSummaryTest(t, detailPayload)} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			if field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.String {
				t.Fatalf("%s retains dynamic repeated/string state in %s %s", typeOf.Name(), field.Name, field.Type)
			}
		}
	}
}

func mustDecodeProfilerFtraceCPUDetailForFixedSummaryTest(t *testing.T, data []byte) profilerFtraceCPUDetail {
	t.Helper()
	detail, err := decodeProfilerFtraceCPUDetailContext(context.Background(), data)
	if err != nil {
		t.Fatalf("decode fixed CPU detail summary: %v", err)
	}
	return detail
}

func TestProfilerFtraceFixedSummaryCPUSetBoundaries(t *testing.T) {
	stats := protoMessage(1,
		protoVarint(1, 1),
		protoMessage(2, protoVarint(1, 0)),
		protoMessage(2, protoVarint(1, uint64(maxTraceDBCPUIndex))),
	)
	result := decodeProfilerTracePluginResult(protoPayload(
		stats,
		protoMessage(2, protoVarint(1, 0)),
		protoMessage(2, protoVarint(1, uint64(maxTraceDBCPUIndex))),
	))
	summary, recognized, err := decodeProfilerFtraceSummaryResultContext(context.Background(), result)
	if err != nil || !recognized {
		t.Fatalf("decode CPU boundary summary: recognized=%t err=%v", recognized, err)
	}
	for label, set := range map[string]profilerSummaryCPUSet{
		"stats":  summary.StatsCPUs,
		"detail": summary.DetailCPUs,
	} {
		if set.count() != 2 || !set.contains(0) || !set.contains(uint64(maxTraceDBCPUIndex)) {
			t.Fatalf("%s CPU boundary bitset drifted: count=%d first=%t last=%t", label, set.count(), set.contains(0), set.contains(uint64(maxTraceDBCPUIndex)))
		}
		if maxTraceDBCPUIndex > 1 && set.contains(1) {
			t.Fatalf("%s CPU bitset fabricated an interior CPU", label)
		}
	}
	var rejected profilerSummaryCPUSet
	if rejected.observe(uint64(maxTraceDBCPUIndex+1)) || rejected.count() != 0 {
		t.Fatalf("CPU bitset admitted out-of-range CPU %d", maxTraceDBCPUIndex+1)
	}
}

func profilerUnknownFixedSummary(t *testing.T, reverse bool) profilerFtraceSummary {
	t.Helper()
	const count = 32
	var detail bytes.Buffer
	detail.Write(protoVarint(1, 7))
	for index := 0; index < count; index++ {
		value := index
		if reverse {
			value = count - index - 1
		}
		detail.Write(profilerFixedSummaryEvent(5_000 + value))
	}
	summary, recognized, err := decodeProfilerFtraceSummary(protoBytes(2, detail.Bytes()))
	if err != nil || !recognized {
		t.Fatalf("decode unknown-event summary: recognized=%t err=%v", recognized, err)
	}
	return summary
}

func TestProfilerFtraceFixedSummaryUnknownSamplesAreBoundedAndOrderIndependent(t *testing.T) {
	forward := profilerUnknownFixedSummary(t, false)
	reverse := profilerUnknownFixedSummary(t, true)
	if forward.UnknownEventCount != 32 || reverse.UnknownEventCount != 32 ||
		forward.UnknownEventFieldSamples.Used != profilerDiagnosticSampleLimit ||
		forward.UnknownEventFieldSamples != reverse.UnknownEventFieldSamples {
		t.Fatalf("unknown event fixed sample census/order drifted: forward=%+v reverse=%+v", forward, reverse)
	}
	caveat := profilerFtraceSummaryCaveat(forward)
	reverseCaveat := profilerFtraceSummaryCaveat(reverse)
	if !strings.Contains(caveat, "unknown_event_records=32") ||
		!strings.Contains(caveat, "unknown_event_field_samples=") ||
		!strings.Contains(caveat, "sample_policy=sha256_min_k8_prefix96_bounded_examples_not_complete_inventory") {
		t.Fatalf("unknown event bounded disclosure drifted: %s", caveat)
	}
	if caveat != reverseCaveat {
		t.Fatalf("unknown event caveat depends on physical order:\nforward=%s\nreverse=%s", caveat, reverseCaveat)
	}
}

func profilerScalarFixedSummary(t *testing.T, reverse bool) profilerFtraceSummary {
	t.Helper()
	const count = 16
	parts := make([][]byte, 0, count*3+1)
	for index := 0; index < count; index++ {
		value := index
		if reverse {
			value = count - index - 1
		}
		parts = append(parts,
			protoMessage(1, protoVarint(1, 1), protoBytes(3, []byte("clock_"+strconv.Itoa(value)))),
			protoMessage(5, protoVarint(1, uint64(value+1)), protoBytes(2, []byte("symbol_"+strconv.Itoa(value)))),
			protoMessage(6, protoVarint(1, uint64(value%7)), protoMessage(2, protoVarint(1, uint64(value)))),
		)
	}
	parts = append(parts, protoBytes(7, []byte("trace-plugin-v1")))
	summary, recognized, err := decodeProfilerFtraceSummary(protoPayload(parts...))
	if err != nil || !recognized {
		t.Fatalf("decode scalar summary: recognized=%t err=%v", recognized, err)
	}
	return summary
}

func TestProfilerFtraceFixedSummaryScalarSamplesAreBoundedAndOrderIndependent(t *testing.T) {
	forward := profilerScalarFixedSummary(t, false)
	reverse := profilerScalarFixedSummary(t, true)
	if forward.TraceClockObserved != 16 || forward.SymbolCount != 16 || forward.SymbolNamedCount != 16 ||
		forward.ClockDetailCount != 16 || forward.VersionObservations != 1 ||
		forward.TraceClockSamples.Used != profilerDiagnosticSampleLimit ||
		forward.SymbolSamples.Used != profilerDiagnosticSampleLimit ||
		forward.ClockDetailSamples.Used != profilerDiagnosticSampleLimit ||
		forward.VersionSamples.Used != 1 || !forward.SymbolTruncated || !forward.ClockTruncated {
		t.Fatalf("fixed scalar summary census/truncation drifted: %+v", forward)
	}
	if forward.TraceClockSamples != reverse.TraceClockSamples ||
		forward.SymbolSamples != reverse.SymbolSamples ||
		forward.ClockDetailSamples != reverse.ClockDetailSamples ||
		forward.VersionSamples != reverse.VersionSamples {
		t.Fatalf("fixed scalar samples depend on physical order:\nforward=%+v\nreverse=%+v", forward, reverse)
	}
}

func TestProfilerFtraceFixedSummaryEmptyVersionPreservesNoDisclosureParity(t *testing.T) {
	summary, recognized, err := decodeProfilerFtraceSummary(protoBytes(7, nil))
	if err != nil || !recognized || summary.VersionObservations != 0 || summary.VersionSamples.Used != 0 {
		t.Fatalf("empty version summary parity drifted: recognized=%t err=%v summary=%+v", recognized, err, summary)
	}
	caveat := profilerFtraceSummaryCaveat(summary)
	if strings.Contains(caveat, "version=") || strings.Contains(caveat, "version_samples=") || strings.Contains(caveat, "version_observations=") {
		t.Fatalf("empty version minted a customer-visible version claim: %s", caveat)
	}
}

func TestProfilerFtraceFixedSummaryLedgerCounterOverflowIsAtomic(t *testing.T) {
	slot, ok := profilerFtraceEventDescriptorSlot(1109)
	if !ok {
		t.Fatal("print descriptor slot missing")
	}
	tests := []struct {
		name    string
		prepare func(*profilerFtraceSummaryDiagnosticLedger, *profilerFtraceSummary)
	}{
		{
			name: "known event",
			prepare: func(ledger *profilerFtraceSummaryDiagnosticLedger, summary *profilerFtraceSummary) {
				ledger.KnownEventCounts[slot] = math.MaxUint64
				summary.KnownEventCounts[slot] = 1
			},
		},
		{
			name: "unknown event",
			prepare: func(ledger *profilerFtraceSummaryDiagnosticLedger, summary *profilerFtraceSummary) {
				ledger.UnknownEventCount = math.MaxUint64
				summary.UnknownEventCount = 1
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger := profilerFtraceSummaryDiagnosticLedger{Frames: 3, FirstOffset: 10, LastOffset: 30}
			summary := profilerFtraceSummary{recognizedMessage: true, StartTotalsValid: true, EndTotalsValid: true, DetailOverwriteOK: true}
			test.prepare(&ledger, &summary)
			before := ledger
			if ledger.observe(&summary, 40) {
				t.Fatal("overflowing summary ledger observation succeeded")
			}
			if !reflect.DeepEqual(ledger, before) {
				t.Fatalf("failed summary observation partially mutated ledger:\nbefore=%+v\nafter=%+v", before, ledger)
			}
		})
	}
}

func TestProfilerFtraceFixedSummaryCancellationIdentity(t *testing.T) {
	event := profilerFixedSummaryEvent(1109)
	detail := append(protoVarint(1, 7), bytes.Repeat(event, 4_096)...)
	result := decodeProfilerTracePluginResult(protoBytes(2, detail))
	ctx := &profilerCancelAfterPollContext{Context: context.Background(), cancelAt: 32}
	summary, recognized, err := decodeProfilerFtraceSummaryResultContext(ctx, result)
	if !errors.Is(err, context.Canceled) || recognized || ctx.polls < ctx.cancelAt {
		t.Fatalf("summary cancellation identity drifted: polls=%d recognized=%t err=%v summary=%+v", ctx.polls, recognized, err, summary)
	}
}
