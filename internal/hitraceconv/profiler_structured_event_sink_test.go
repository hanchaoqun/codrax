package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func profilerStructuredEventTestRow(seq int, kind pairRenderKind, field int, lane, line string) renderedRow {
	return renderedRow{
		tsNS: seqToTS(seq), seq: seq, line: line,
		pairKind: kind, pairLane: lane, pairTable: line,
		structuredPair: true, profilerEventField: field,
	}
}

func seqToTS(seq int) uint64 { return uint64(seq + 1) }

func profilerSinkInvariantReason(t *testing.T, err error) string {
	t.Helper()
	var invariant *traceDBOutputInvariantError
	if !errors.As(err, &invariant) {
		t.Fatalf("expected traceDBOutputInvariantError, got %T: %v", err, err)
	}
	return invariant.Reason
}

func TestProfilerStructuredEventSinkTracksExactFieldAndLane(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	rows := []renderedRow{
		profilerStructuredEventTestRow(1, pairRenderF2FS, 4011, "lane-a", "f2fs_write_begin"),
		profilerStructuredEventTestRow(2, pairRenderF2FS, 4011, "lane-b", "f2fs_write_begin"),
		profilerStructuredEventTestRow(3, pairRenderF2FS, 4012, "lane-a", "f2fs_write_end"),
		profilerStructuredEventTestRow(4, pairRenderMMC, 4015, "mmc", "mmc_request_done"),
		profilerStructuredEventTestRow(5, pairRenderMMC, 4016, "mmc", "mmc_request_start"),
		// A text-compatible row may share the exact rendered table and lane but
		// never contributes to structured profiler-event accounting.
		{tsNS: 7, seq: 6, line: "f2fs_write_begin", pairKind: pairRenderF2FS,
			pairLane: "lane-a", pairTable: "f2fs_write_begin"},
	}
	for _, row := range rows {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	sink.poisonPairLane(pairRenderF2FS, "lane-a")
	if got := sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4011); got != 1 {
		t.Fatalf("field 4011 withheld=%d want=1", got)
	}
	if got := sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4012); got != 1 {
		t.Fatalf("field 4012 withheld=%d want=1", got)
	}
	if got := sink.withheldStructuredPairRowsForKind(pairRenderF2FS); got != 2 {
		t.Fatalf("aggregate structured F2FS withheld=%d want=2", got)
	}
	if got := sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4015); got != 0 {
		t.Fatalf("cross-family event field was accepted: %d", got)
	}

	sink.poisonPairKind(pairRenderMMC)
	if got := sink.withheldStructuredPairRowsForEventField(pairRenderMMC, 4015); got != 1 {
		t.Fatalf("field 4015 family-withheld=%d want=1", got)
	}
	if got := sink.withheldStructuredPairRowsForEventField(pairRenderMMC, 4016); got != 1 {
		t.Fatalf("field 4016 family-withheld=%d want=1", got)
	}
	if sink.structuredEventLanes[pairRenderMMC] != nil ||
		sink.structuredEventRows[pairRenderMMC][4015] != 1 || sink.structuredEventRows[pairRenderMMC][4016] != 1 {
		t.Fatalf("family poison did not release lane proof state and retain exact scalars: lanes=%v totals=%v",
			sink.structuredEventLanes[pairRenderMMC], sink.structuredEventRows[pairRenderMMC])
	}
}

func TestProfilerStructuredEventFieldRequiresMatchingStructuredPair(t *testing.T) {
	tests := []struct {
		name   string
		row    renderedRow
		reason string
	}{
		{
			name: "text row cannot claim profiler field",
			row: renderedRow{tsNS: 1, seq: 1, line: "text", pairKind: pairRenderF2FS,
				pairLane: "lane", profilerEventField: 4011},
			reason: "profiler_event_field_without_structured_pair",
		},
		{
			name: "structured profiler row requires exact event field",
			row: renderedRow{tsNS: 1, seq: 1, line: "missing", pairKind: pairRenderF2FS,
				pairLane: "lane", pairTable: "f2fs_write_begin", structuredPair: true},
			reason: "profiler_structured_pair_missing_event_field",
		},
		{
			name:   "F2FS field cannot claim MMC kind",
			row:    profilerStructuredEventTestRow(1, pairRenderMMC, 4011, "lane", "mismatch"),
			reason: "profiler_event_field_pair_kind_mismatch",
		},
		{
			name:   "unknown profiler field cannot claim F2FS kind",
			row:    profilerStructuredEventTestRow(1, pairRenderF2FS, 9999, "lane", "unknown"),
			reason: "profiler_event_field_pair_kind_mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if reason := profilerSinkInvariantReason(t, sink.add(test.row)); reason != test.reason {
				t.Fatalf("reason=%q want=%q", reason, test.reason)
			}
			if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.structuredEventRows) != 0 {
				t.Fatalf("rejected provenance mutated sink: stats=%+v rows=%d exact=%v",
					sink.stats, len(sink.rows), sink.structuredEventRows)
			}
		})
	}
}

func TestProfilerStructuredEventCountersFailLoudBeforeMutation(t *testing.T) {
	t.Run("field total", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		sink.structuredEventRows[pairRenderMMC] = map[int]int{4015: math.MaxInt}
		row := profilerStructuredEventTestRow(1, pairRenderMMC, 4015, "lane", "overflow")
		if reason := profilerSinkInvariantReason(t, sink.add(row)); reason != "profiler_structured_event_counter_overflow" {
			t.Fatalf("reason=%q", reason)
		}
		if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || sink.structuredEventRows[pairRenderMMC][4015] != math.MaxInt {
			t.Fatalf("field overflow mutated sink: stats=%+v rows=%d totals=%v", sink.stats, len(sink.rows), sink.structuredEventRows)
		}
	})

	t.Run("field lane", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		sink.structuredEventLanes[pairRenderF2FS] = map[int]map[string]int{4011: {"lane": math.MaxInt}}
		row := profilerStructuredEventTestRow(1, pairRenderF2FS, 4011, "lane", "overflow")
		if reason := profilerSinkInvariantReason(t, sink.add(row)); reason != "profiler_structured_event_lane_counter_overflow" {
			t.Fatalf("reason=%q", reason)
		}
		if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 ||
			sink.structuredEventLanes[pairRenderF2FS][4011]["lane"] != math.MaxInt {
			t.Fatalf("lane overflow mutated sink: stats=%+v rows=%d lanes=%v", sink.stats, len(sink.rows), sink.structuredEventLanes)
		}
	})
}

func TestProfilerStructuredEventBudgetFailCloseRetainsExactFieldTotals(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	sink.pairObservationLimit = 1
	sink.pairLaneLimit = 8
	for _, row := range []renderedRow{
		profilerStructuredEventTestRow(1, pairRenderF2FS, 4011, "lane", "begin"),
		profilerStructuredEventTestRow(2, pairRenderF2FS, 4012, "lane", "end"),
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	if !sink.pairBudgetFailed || !sink.poisoned[pairRenderF2FS] || !sink.poisoned[pairRenderMMC] ||
		sink.structuredEventLanes[pairRenderF2FS] != nil ||
		sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4011) != 1 ||
		sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4012) != 1 {
		t.Fatalf("budget fail-close lost exact event totals: failed=%t poisoned=%v totals=%v lanes=%v",
			sink.pairBudgetFailed, sink.poisoned, sink.structuredEventRows, sink.structuredEventLanes)
	}
}

func TestProfilerStructuredEventFieldSurvivesSpillAndFiltering(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	for _, row := range []renderedRow{
		profilerStructuredEventTestRow(1, pairRenderF2FS, 4011, "drop", "structured-drop"),
		profilerStructuredEventTestRow(2, pairRenderF2FS, 4011, "keep", "structured-keep"),
		{tsNS: 4, seq: 3, line: "plain-keep"},
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.chunks) != 3 {
		t.Fatalf("threshold-one sink did not spill every row: chunks=%d", len(sink.chunks))
	}
	reader, err := openTraceDBChunkReader(sink.chunks[0])
	if err != nil {
		t.Fatal(err)
	}
	spilled, ok, readErr := reader.next()
	closeErr := reader.close()
	if readErr != nil || closeErr != nil || !ok || !spilled.structuredPair || spilled.profilerEventField != 4011 {
		t.Fatalf("spilled typed provenance drifted: row=%+v ok=%t read=%v close=%v", spilled, ok, readErr, closeErr)
	}

	sink.poisonPairLane(pairRenderF2FS, "drop")
	if got := sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4011); got != 1 {
		t.Fatalf("spilled exact field withheld=%d want=1", got)
	}
	var out bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if stats.RowsAccepted != 3 || stats.RowsWritten != 2 || stats.RowsWithheld != 1 ||
		strings.Contains(text, "structured-drop") || !strings.Contains(text, "structured-keep") || !strings.Contains(text, "plain-keep") {
		t.Fatalf("spill/filter parity drifted: stats=%+v\n%s", stats, text)
	}
}
