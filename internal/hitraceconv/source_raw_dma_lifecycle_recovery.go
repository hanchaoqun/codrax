package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
)

func newTraceDBRawDMALifecycleRecoveryCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_dma_lifecycle",
		Table:  "__raw_dma_lifecycle__",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"authority":     "complete strict official raw decode ledger; exact dma_fence_init/destroy/enable_signal/signaled descriptor profiles only",
			"semantics":     "official SmartPerf CpuDetailParser publishes each record as a dma_fence point row with driver/timeline/context/seqno; no B/E phase or wait duration is inferred",
			"payload":       "official lifecycle profile retains an exact empty driver C string but requires a nonempty timeline; DMA wait pairing keeps both hard keys nonempty",
			"timestamp":     "exact source raw record timestamp on the official bytrace decimal wire, using nanosecond digits when required",
			"cpu":           "exact source raw page CPU",
			"envelope":      "exact source common_pid/common_flags/common_preempt_count; unresolved comm/TGID remains typed unknown and never suppresses an otherwise exact point event",
			"namespace":     "physical common_pid is copied verbatim; no namespace PID, host PID, TGID, comm, or incarnation rewrite is attempted",
			"deduplication": "source publication is wholly withheld when the normalized DB raw-ftrace DMA class emitted any row; the high-level dma_fence predecessor-delta table is non-equivalent and does not suppress exact source points",
		},
		Metadata: map[string]string{
			"publication_state":  traceDBSourceRawLanePlaceholderState,
			"official_semantics": "point_event_not_interval",
		},
	}
}

func publishTraceDBRawDMALifecycleRecovery(
	ctx context.Context,
	inventory *traceDBSourceNameInventory,
	sink *traceDBRowSink,
	dbRawCoverage []TraceDBCoverage,
) (TraceDBCoverage, error) {
	out := newTraceDBRawDMALifecycleRecoveryCoverage()
	if stop, err := traceDBApplySourceRawLaneGate(&out, inventory, "raw DMA lifecycle recovery"); stop {
		return out, err
	}
	// Past the gate the census closed; the family predicate can only fail on
	// the family's own retention store having been withdrawn by byte budget.
	if !traceDBRawDecodeFamilyComplete(
		inventory.RawDecode, traceDBRawRetentionDMALifecycle) {
		out.Metadata["publication_state"] = traceDBSourceRawLaneFamilyRetentionWithdrawnState
		out.Skipped = "raw DMA lifecycle recovery withheld: retained family record store exceeded its byte budget"
		return out, nil
	}
	rows := inventory.RawDMALifecycle
	out.RowsRead = len(rows)
	traceDBAddCoverageMetric(&out, "raw_points_retained", int64(len(rows)))
	expected := int64(0)
	for _, name := range []string{
		"dma_fence_destroy",
		"dma_fence_enable_signal",
		"dma_fence_init",
		"dma_fence_signaled",
	} {
		records := inventory.RawDecode.Metrics["target_"+name+"_records"]
		admitted := inventory.RawDecode.Metrics["target_"+name+"_body_admitted"]
		traceDBAddCoverageMetric(&out, name+"_records", records)
		if records != admitted {
			out.Metadata["publication_state"] =
				"withheld_raw_point_census_incomplete"
			out.Skipped = "raw DMA lifecycle recovery withheld: physical/admitted point census mismatch"
			return out, nil
		}
		expected += admitted
	}
	if expected != int64(len(rows)) ||
		inventory.RawDecode.Metrics["target_dma_fence_lifecycle_record_capture_failed"] != 0 {
		out.Metadata["publication_state"] =
			"withheld_raw_point_census_incomplete"
		out.Skipped = "raw DMA lifecycle recovery withheld: admitted/retained point census mismatch"
		return out, nil
	}
	// Overlap is measured on the exact governed event names (§40.42 ④a): the
	// dma_fence SQL class also carries DB-only names that never overlap this
	// source family.
	if supersession, ok := traceDBRawSourceSupersessionForFamily(traceDBRawRetentionDMALifecycle); ok {
		if publishable := traceDBRawSourceGovernedRowsPublishable(dbRawCoverage, supersession); publishable > 0 {
			traceDBAddCoverageMetric(&out, "db_raw_governed_dma_lifecycle_rows_publishable", publishable)
			out.Metadata["publication_state"] = "withheld_db_raw_dma_overlap"
			out.Skipped = "raw DMA lifecycle recovery withheld: normalized DB raw-ftrace rows of governed DMA lifecycle event names already emitted"
			return out, nil
		}
	}
	if len(rows) == 0 {
		out.Metadata["publication_state"] =
			"complete_no_raw_dma_lifecycle_point"
		out.Skipped = "raw DMA lifecycle recovery complete: no strict lifecycle point present"
		return out, nil
	}
	if sink == nil {
		err := &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
		out.Error = err.Error()
		return out, err
	}
	if sink.stats.RowsAccepted < 0 ||
		len(rows) > math.MaxInt-sink.stats.RowsAccepted {
		err := &traceDBOutputInvariantError{
			Reason: "invalid_raw_dma_lifecycle_recovery_sequence"}
		out.Error = err.Error()
		return out, err
	}
	type orderedPoint struct {
		record  traceDBRawDMALifecycleRecord
		ordinal int
	}
	ordered := make([]orderedPoint, len(rows))
	for index, row := range rows {
		ordered[index] = orderedPoint{record: row, ordinal: index}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].record.TimestampNS != ordered[j].record.TimestampNS {
			return ordered[i].record.TimestampNS <
				ordered[j].record.TimestampNS
		}
		return ordered[i].ordinal < ordered[j].ordinal
	})
	rendered := make([]renderedRow, 0, len(ordered))
	for index, item := range ordered {
		raw := item.record
		if !traceDBRawDMALifecycleName(raw.Name) ||
			raw.TimestampNS > math.MaxInt64 ||
			!validTraceDBCPUIndex(int64(raw.CPU)) ||
			raw.HeaderPID < 0 || raw.HeaderPID > math.MaxInt32 ||
			raw.Flags < 0 || raw.Flags > math.MaxUint8 ||
			raw.PreemptCount < 0 ||
			raw.PreemptCount > math.MaxUint8 {
			out.Metadata["publication_state"] =
				"withheld_invalid_point_envelope"
			out.Skipped = "raw DMA lifecycle recovery withheld: retained point envelope invalid"
			return out, nil
		}
		bodyText, ok := renderCanonicalDMAFenceLifecycleFields(
			&pairDMAFencePayload{
				Driver: raw.Driver, DriverKnown: true,
				Timeline: raw.Timeline, TimelineKnown: true,
				NumberBits: 32,
				Context:    raw.Context, ContextKnown: true,
				Seqno: raw.Seqno, SeqnoKnown: true,
			})
		if !ok {
			return out, &traceDBOutputInvariantError{
				Reason: "invalid_raw_dma_lifecycle_body"}
		}
		row, err := prepareTraceDBRenderedRowEnvelopeContext(
			ctx, int64(raw.TimestampNS),
			sink.stats.RowsAccepted+index, "unknown",
			raw.HeaderPID, 0, int64(raw.CPU), raw.Flags,
			raw.PreemptCount, raw.HeaderPID != 0,
			raw.Name+": "+bodyText)
		if err != nil {
			out.Error = err.Error()
			return out, err
		}
		rendered = append(rendered, row)
	}
	for _, row := range rendered {
		if err := sink.addContext(ctx, row, nil, false); err != nil {
			out.Error = err.Error()
			return out, err
		}
		out.RowsEmitted++
	}
	out.Metadata["publication_state"] =
		"published_exact_official_point_events"
	out.Metadata["publication_contract"] = fmt.Sprintf(
		"points=%d; exact raw timestamp/cpu/flags/header and strict DMA tuple; duration=not_constructed",
		out.RowsEmitted)
	return out, nil
}
