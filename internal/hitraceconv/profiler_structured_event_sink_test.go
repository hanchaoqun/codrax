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
	pairTable := ""
	if slot, ok := profilerPairEndpointForStructuredField(field); ok {
		if descriptor, found := slot.descriptor(); found {
			pairTable = descriptor.name
		}
	}
	return renderedRow{
		tsNS: seqToTS(seq), seq: seq, line: line,
		pairKind: kind, pairLane: lane, pairTable: pairTable,
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
	if profilerTestStructuredEventLanes(sink)[pairRenderMMC] != nil ||
		profilerTestStructuredEventRows(sink)[pairRenderMMC][4015] != 1 || profilerTestStructuredEventRows(sink)[pairRenderMMC][4016] != 1 {
		t.Fatalf("family poison did not release lane proof state and retain exact scalars: lanes=%v totals=%v",
			profilerTestStructuredEventLanes(sink)[pairRenderMMC], profilerTestStructuredEventRows(sink)[pairRenderMMC])
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
			if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(profilerTestStructuredEventRows(sink)) != 0 {
				t.Fatalf("rejected provenance mutated sink: stats=%+v rows=%d exact=%v",
					sink.stats, len(sink.rows), profilerTestStructuredEventRows(sink))
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
		sink.pairFixedLedger.families[pairRenderMMC].profilerPairFixedCounts = profilerPairFixedCounts{
			staged: math.MaxInt, structured: math.MaxInt,
		}
		sink.pairFixedLedger.endpoints[profilerPairEndpointMMCRequestDone] = profilerPairFixedCounts{
			staged: math.MaxInt, structured: math.MaxInt,
		}
		before := sink.pairFixedLedger
		row := profilerStructuredEventTestRow(1, pairRenderMMC, 4015, "lane", "overflow")
		if reason := profilerSinkInvariantReason(t, sink.add(row)); reason != "profiler_pair_fixed_ledger_plan_invalid" {
			t.Fatalf("reason=%q", reason)
		}
		if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || sink.pairFixedLedger != before {
			t.Fatalf("field overflow mutated sink: stats=%+v rows=%d ledger=%+v", sink.stats, len(sink.rows), sink.pairFixedLedger)
		}
	})

	t.Run("field lane", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		id, ok := sink.pairLaneRegistries[pairRenderF2FS].intern("lane")
		if !ok {
			t.Fatal("failed to seed exact lane")
		}
		state, ok := sink.pairLaneRegistries[pairRenderF2FS].state(id)
		if !ok {
			t.Fatal("seed exact lane state missing")
		}
		ordinal, ok := profilerPairEndpointF2FSWriteBegin.familyOrdinal(pairRenderF2FS)
		if !ok {
			t.Fatal("F2FS write-begin ordinal missing")
		}
		state.endpointCounts[ordinal] = profilerPairLaneEndpointCounts{
			rows: uint32(profilerPairBarrierMaxObservations), structuredRows: uint32(profilerPairBarrierMaxObservations),
		}
		before := *state
		row := profilerStructuredEventTestRow(1, pairRenderF2FS, 4011, "lane", "overflow")
		if reason := profilerSinkInvariantReason(t, sink.add(row)); reason != "profiler_pair_fixed_lane_plan_invalid" {
			t.Fatalf("reason=%q", reason)
		}
		if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || *state != before {
			t.Fatalf("lane overflow mutated sink: stats=%+v rows=%d state=%+v", sink.stats, len(sink.rows), *state)
		}
	})
}

func TestProfilerStructuredEventBudgetFailCloseRetainsExactFieldTotals(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	sink.legacyPairProof.maxObservations = 1
	sink.legacyPairProof.maxLaneKeys = 8
	for _, row := range []renderedRow{
		profilerStructuredEventTestRow(1, pairRenderF2FS, 4011, "lane", "begin"),
		profilerStructuredEventTestRow(2, pairRenderF2FS, 4012, "lane", "end"),
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	if sink.legacyPairProof.failureReason == "" || !sink.poisoned[pairRenderF2FS] || !sink.poisoned[pairRenderMMC] ||
		profilerTestStructuredEventLanes(sink)[pairRenderF2FS] != nil ||
		sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4011) != 1 ||
		sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4012) != 1 {
		t.Fatalf("budget fail-close lost exact event totals: failed=%t poisoned=%v totals=%v lanes=%v",
			sink.legacyPairProof.failureReason != "", sink.poisoned, profilerTestStructuredEventRows(sink), profilerTestStructuredEventLanes(sink))
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
	if len(sink.runs) != 3 {
		t.Fatalf("threshold-one sink did not spill every row: runs=%d", len(sink.runs))
	}
	reader, err := sink.openAuthenticatedRunReader(sink.runs[0])
	if err != nil {
		t.Fatal(err)
	}
	record, ok, readErr := reader.next(context.Background())
	_, more, eofErr := reader.next(context.Background())
	closeErr := reader.close()
	spilled := record.row
	if readErr != nil || eofErr != nil || closeErr != nil || !ok || more ||
		spilled.provenance.Flags != profilerPairRowProvenanceStructured ||
		spilled.provenance.EndpointSlot != profilerPairEndpointF2FSWriteBegin {
		t.Fatalf("spilled typed provenance drifted: row=%+v ok=%t more=%t read=%v eof=%v close=%v",
			spilled, ok, more, readErr, eofErr, closeErr)
	}

	sink.poisonPairLane(pairRenderF2FS, "drop")
	if got := sink.withheldStructuredPairRowsForEventField(pairRenderF2FS, 4011); got != 1 {
		t.Fatalf("spilled exact field withheld=%d want=1", got)
	}
	var out bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if stats.RowsAccepted != 3 || stats.RowsWritten != 2 || stats.RowsWithheld != 1 ||
		strings.Contains(text, "structured-drop") || !strings.Contains(text, "structured-keep") || !strings.Contains(text, "plain-keep") {
		t.Fatalf("spill/filter parity drifted: stats=%+v\n%s", stats, text)
	}
}
