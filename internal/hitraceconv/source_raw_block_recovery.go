package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func newTraceDBRawBlockRecoveryCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_block",
		Table:  "__raw_block__",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"authority":     "complete strict official raw decode ledger; exact block_rq_* and block_bio_* descriptor profiles only",
			"timestamp":     "exact source raw record timestamp on the standard ftrace wire, preserving nanosecond digits",
			"cpu":           "exact source raw page CPU",
			"envelope":      "exact source common_pid/common_flags/common_preempt_count",
			"namespace":     "physical common_pid is copied verbatim; no namespace PID, host PID, TGID, comm, or incarnation rewrite is attempted",
			"payload":       "sole governed direct block decoder and canonical OpenHarmony/Linux ftrace field order",
			"pair_key":      "tracequery.DecodePairingEndpoint over exact request device, operation, sector and length; issue/complete may have different emitter threads",
			"duration":      "this exporter preserves physical point rows only; request residence and response IO wait require exact downstream issue/complete closure and never arise from proximity",
			"deduplication": "a complete source family supersedes normalized SQLite raw rows of the exact governed block event names before DB publication; otherwise source publication is withheld if any DB row of a governed block event name kept publication authority; rows of other block_storage class names are never part of this overlap",
		},
		Metadata: map[string]string{
			"publication_state": traceDBSourceRawLanePlaceholderState,
		},
	}
}

// traceDBRawBlockFamilyAuthorityEligible is intentionally stronger than a
// non-empty retained slice. It is the source-precedence gate used before the
// SQLite raw exporter, so every physical governed row must have an admitted
// body and one retained immutable record.
func traceDBRawBlockFamilyAuthorityEligible(inventory *traceDBSourceNameInventory) bool {
	if inventory == nil || len(inventory.RawBlock) == 0 ||
		!traceDBRawDecodeFamilyComplete(inventory.RawDecode, traceDBRawRetentionBlock) ||
		inventory.RawDecode.Metrics["target_block_storage_record_capture_failed"] != 0 {
		return false
	}
	physical, admitted := int64(0), int64(0)
	for _, name := range traceDBRawBlockTargetNames() {
		physical += inventory.RawDecode.Metrics["target_"+name+"_records"]
		admitted += inventory.RawDecode.Metrics["target_"+name+"_body_admitted"]
	}
	return physical > 0 && physical == admitted &&
		admitted == int64(len(inventory.RawBlock)) &&
		inventory.RawDecode.Metrics["target_block_storage_records_retained"] == admitted
}

func publishTraceDBRawBlockRecovery(
	ctx context.Context,
	inventory *traceDBSourceNameInventory,
	sink *traceDBRowSink,
	dbRawCoverage []TraceDBCoverage,
) (TraceDBCoverage, error) {
	out := newTraceDBRawBlockRecoveryCoverage()
	if inventory != nil {
		out.RowsRead = len(inventory.RawBlock)
		traceDBAddCoverageMetric(&out, "raw_block_rows_retained", int64(len(inventory.RawBlock)))
	}
	if stop, err := traceDBApplySourceRawLaneGate(&out, inventory, "raw block recovery"); stop {
		return out, err
	}
	if !traceDBRawBlockFamilyAuthorityEligible(inventory) {
		if traceDBRawDecodeFamilyComplete(inventory.RawDecode, traceDBRawRetentionBlock) &&
			len(inventory.RawBlock) == 0 {
			out.Metadata["publication_state"] = "complete_no_raw_block_event"
			out.Skipped = "raw block recovery complete: no strict block request/BIO event present"
		} else {
			out.Metadata["publication_state"] = "withheld_raw_block_census_incomplete"
			out.Skipped = "raw block recovery withheld: physical/admitted/retained family census is not exact"
		}
		return out, nil
	}
	// Overlap is measured on the exact governed event names, never on the SQL
	// raw class: block_storage also carries MMC/UFS/SCSI rows the DB lane
	// legitimately publishes alongside this source family.
	supersession, registered := traceDBRawSourceSupersessionForFamily(traceDBRawRetentionBlock)
	if !registered {
		err := &traceDBOutputInvariantError{Reason: "source_supersession_family_unregistered"}
		out.Error = err.Error()
		return out, err
	}
	if publishable := traceDBRawSourceGovernedRowsPublishable(dbRawCoverage, supersession); publishable > 0 {
		traceDBAddCoverageMetric(&out, "db_raw_governed_block_rows_publishable", publishable)
		out.Metadata["publication_state"] = "withheld_db_raw_block_overlap"
		out.Skipped = "raw block recovery withheld: normalized DB raw-ftrace rows of governed block event names kept DB publication authority"
		return out, nil
	}
	if sink == nil {
		err := &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
		out.Error = err.Error()
		return out, err
	}
	if sink.stats.RowsAccepted < 0 ||
		len(inventory.RawBlock) > math.MaxInt-sink.stats.RowsAccepted {
		err := &traceDBOutputInvariantError{Reason: "invalid_raw_block_recovery_sequence"}
		out.Error = err.Error()
		return out, err
	}

	type orderedBlock struct {
		record traceDBRawBlockRecord
		index  int
	}
	ordered := make([]orderedBlock, len(inventory.RawBlock))
	ordinals := make(map[int64]bool, len(inventory.RawBlock))
	for index, record := range inventory.RawBlock {
		if record.PhysicalOrdinal <= 0 || ordinals[record.PhysicalOrdinal] ||
			record.TimestampNS > math.MaxInt64 ||
			!validTraceDBCPUIndex(int64(record.CPU)) ||
			record.HeaderPID < 0 || record.HeaderPID > math.MaxInt32 ||
			record.Flags < 0 || record.Flags > math.MaxUint8 ||
			record.PreemptCount < 0 || record.PreemptCount > math.MaxUint8 ||
			!directBlockNameGoverned(record.Name) ||
			record.Body == "" || !traceDBSinglePhysicalLine(record.Body, false) {
			out.Metadata["publication_state"] = "withheld_invalid_retained_envelope"
			out.Skipped = "raw block recovery withheld: retained event envelope or body is invalid"
			return out, nil
		}
		ordinals[record.PhysicalOrdinal] = true
		ordered[index] = orderedBlock{record: record, index: index}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].record.TimestampNS != ordered[j].record.TimestampNS {
			return ordered[i].record.TimestampNS < ordered[j].record.TimestampNS
		}
		if ordered[i].record.PhysicalOrdinal != ordered[j].record.PhysicalOrdinal {
			return ordered[i].record.PhysicalOrdinal < ordered[j].record.PhysicalOrdinal
		}
		return ordered[i].index < ordered[j].index
	})

	rendered := make([]renderedRow, 0, len(ordered))
	endpointRows := int64(0)
	for index, item := range ordered {
		record := item.record
		body := record.Name + ": " + record.Body
		if directBlockPairEndpointName(record.Name) {
			verdict := tracequery.DecodePairingEndpoint(
				record.Name, record.Body, record.HeaderPID)
			if !verdict.Recognized || verdict.Family != tracequery.PairingEndpointBlock ||
				!verdict.KeyKnown || !verdict.PayloadAdmitted ||
				!verdict.EmitterKnown || !verdict.EmitterAdmitted ||
				!traceDBRawPairingWireParity(record.Name, body, record.HeaderPID, verdict) {
				out.Metadata["publication_state"] = "withheld_pairing_wire_mismatch"
				out.Skipped = "raw block recovery withheld: retained endpoint disagrees with the source-neutral pairing authority"
				return out, nil
			}
			endpointRows++
		}
		task := "unknown"
		if name := inventory.Names[record.HeaderPID]; name != "" {
			task = name
		}
		row, err := prepareTraceDBRenderedRowEnvelopeContext(
			ctx, int64(record.TimestampNS), sink.stats.RowsAccepted+index,
			task, record.HeaderPID, 0, int64(record.CPU), record.Flags,
			record.PreemptCount, record.HeaderPID != 0, body)
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
	traceDBAddCoverageMetric(&out, "pairing_endpoint_rows_published", endpointRows)
	out.Metadata["publication_state"] = "published_complete_exact_source_family"
	out.Metadata["publication_contract"] = fmt.Sprintf(
		"events=%d; endpoints=%d; exact raw timestamp/cpu/flags/common_pid and canonical block payload; interval authority remains completion-closed",
		out.RowsEmitted, endpointRows)
	return out, nil
}
