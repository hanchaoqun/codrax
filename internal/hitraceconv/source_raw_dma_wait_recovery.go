package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBRawDMAWaitPrepared struct {
	raw       traceDBRawDMAWaitRecord
	ordinal   int
	thread    traceDBThread
	process   traceDBProcess
	payload   pairRenderPayload
	verdict   tracequery.PairingEndpointVerdict
	lane      string
	published bool
}

func newTraceDBRawDMAWaitRecoveryCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_dma_wait",
		Table:  "__raw_dma_wait__",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"authority":     "complete strict official raw decode ledger; exact dma_fence_wait_start/end descriptor profile only",
			"timestamp":     "exact source raw record timestamp on the official bytrace decimal wire, using nanosecond digits when required",
			"cpu":           "exact source raw page CPU",
			"envelope":      "exact source common_pid/common_flags/common_preempt_count; common_pid must resolve to one canonical public host TID at the exact timestamp",
			"pair_key":      "tracequery.FingerprintPairingEndpoint over exact header TID, driver, timeline, uint32 context and uint32 seqno",
			"pair_topology": "each exact lane must alternate start/end with a positive wire-representable interval; malformed lanes are withheld whole so adjacent endpoints cannot bridge a hole",
			"deduplication": "source publication is wholly withheld when the normalized DB raw-ftrace DMA class emitted any row; high-level dma_fence rows are non-equivalent and never subtracted",
			"namespace":     "no namespace PID rewrite, name alias, or TGID=TID fallback; absent, rejected, or ambiguous public identity poisons only its exact key lane",
		},
		Metadata: map[string]string{
			"publication_state": "unavailable",
		},
	}
}

func publishTraceDBRawDMAWaitRecovery(
	ctx context.Context,
	inventory *traceDBSourceNameInventory,
	sink *traceDBRowSink,
	authority traceDBSchedulerAuthority,
	dbRawCoverage []TraceDBCoverage,
) (TraceDBCoverage, error) {
	out := newTraceDBRawDMAWaitRecoveryCoverage()
	if inventory == nil {
		out.Skipped = "raw DMA wait recovery unavailable: immutable source inventory absent"
		return out, nil
	}
	out.Found = inventory.RawDecode.Found
	if inventory.RawDecode.Metadata["decode_state"] != "strict_target_ledger_complete" {
		out.Metadata["publication_state"] = "withheld_raw_decode_incomplete"
		out.Skipped = "raw DMA wait recovery withheld: strict raw decode ledger incomplete"
		return out, nil
	}
	rawRows := inventory.RawDMAWait
	out.RowsRead = len(rawRows)
	traceDBAddCoverageMetric(&out, "raw_endpoints_retained", int64(len(rawRows)))
	startRecords := inventory.RawDecode.Metrics["target_dma_fence_wait_start_records"]
	endRecords := inventory.RawDecode.Metrics["target_dma_fence_wait_end_records"]
	startAdmitted := inventory.RawDecode.Metrics["target_dma_fence_wait_start_body_admitted"]
	endAdmitted := inventory.RawDecode.Metrics["target_dma_fence_wait_end_body_admitted"]
	if startRecords != startAdmitted || endRecords != endAdmitted ||
		startAdmitted+endAdmitted != int64(len(rawRows)) ||
		inventory.RawDecode.Metrics["target_dma_fence_wait_record_capture_failed"] != 0 {
		out.Metadata["publication_state"] = "withheld_raw_endpoint_census_incomplete"
		out.Skipped = "raw DMA wait recovery withheld: physical/admitted/retained endpoint census mismatch"
		return out, nil
	}
	for _, item := range dbRawCoverage {
		if item.Family == "raw_ftrace" && item.Table == "dma_fence" && item.RowsEmitted > 0 {
			traceDBAddCoverageMetric(&out, "db_raw_dma_rows_emitted", int64(item.RowsEmitted))
			out.Metadata["publication_state"] = "withheld_db_raw_dma_overlap"
			out.Skipped = "raw DMA wait recovery withheld: normalized DB raw-ftrace DMA rows already emitted and no exact cross-source duplicate key is retained"
			return out, nil
		}
	}
	if len(rawRows) == 0 {
		out.Metadata["publication_state"] = "complete_no_raw_dma_wait_endpoint"
		out.Skipped = "raw DMA wait recovery complete: no strict wait endpoint present"
		return out, nil
	}

	prepared := make([]traceDBRawDMAWaitPrepared, 0, len(rawRows))
	laneIndexes := map[string][]int{}
	poisoned := map[string]bool{}
	unkeyed := 0
	for ordinal, raw := range rawRows {
		payload := traceDBRawDMAWaitPairPayload(raw)
		verdict := fingerprintPairingEndpoint(pairPayloadTypedInput(payload, raw.HeaderPID))
		if !verdict.Recognized || verdict.Family != tracequery.PairingEndpointDMAFence ||
			!verdict.KeyKnown || !verdict.PayloadAdmitted ||
			!verdict.EmitterKnown || !verdict.EmitterAdmitted ||
			(verdict.Phase != tracequery.PairingEndpointStart &&
				verdict.Phase != tracequery.PairingEndpointDone) {
			unkeyed++
			continue
		}
		item := traceDBRawDMAWaitPrepared{
			raw: raw, ordinal: ordinal, payload: payload, verdict: verdict,
			lane: verdict.SemanticKey,
		}
		index := len(prepared)
		prepared = append(prepared, item)
		laneIndexes[item.lane] = append(laneIndexes[item.lane], index)

		reason := traceDBRawDMAWaitCoordinateReason(raw)
		if reason == "" {
			item.thread, item.process, reason =
				traceDBResolveRawPublicTID(authority, raw.HeaderPID, int64(raw.TimestampNS))
			if reason == "" && (item.thread.TID != raw.HeaderPID ||
				item.thread.IPID != item.process.IPID) {
				reason = "identity_tuple_mismatch"
			}
		}
		if reason != "" {
			poisoned[item.lane] = true
			traceDBAddCoverageMetric(&out, "raw_endpoints_rejected_"+reason, 1)
			continue
		}
		prepared[index].thread = item.thread
		prepared[index].process = item.process
	}
	if unkeyed != 0 || len(prepared) != len(rawRows) {
		traceDBAddCoverageMetric(&out, "raw_endpoints_without_exact_pair_key", int64(unkeyed))
		out.Metadata["publication_state"] = "withheld_unkeyed_endpoint"
		out.Skipped = "raw DMA wait recovery withheld: at least one physical endpoint lacks the exact source-neutral pair key"
		return out, nil
	}

	traceDBAddCoverageMetric(&out, "pair_lanes", int64(len(laneIndexes)))
	for lane, indexes := range laneIndexes {
		sort.SliceStable(indexes, func(i, j int) bool {
			left, right := prepared[indexes[i]], prepared[indexes[j]]
			if left.raw.TimestampNS != right.raw.TimestampNS {
				return left.raw.TimestampNS < right.raw.TimestampNS
			}
			return left.ordinal < right.ordinal
		})
		var start *traceDBRawDMAWaitPrepared
		lastTimestamp := uint64(0)
		timestampKnown := false
		pairs := 0
		for _, index := range indexes {
			item := &prepared[index]
			if timestampKnown && item.raw.TimestampNS == lastTimestamp {
				poisoned[lane] = true
				traceDBAddCoverageMetric(&out, "pair_lanes_same_timestamp_ambiguous", 1)
				break
			}
			timestampKnown, lastTimestamp = true, item.raw.TimestampNS
			switch item.verdict.Phase {
			case tracequery.PairingEndpointStart:
				if start != nil {
					poisoned[lane] = true
					traceDBAddCoverageMetric(&out, "pair_lanes_nested_or_repeated_start", 1)
					break
				}
				start = item
			case tracequery.PairingEndpointDone:
				if start == nil ||
					!traceDBWireIntervalRepresentable(
						int64(start.raw.TimestampNS), int64(item.raw.TimestampNS)) {
					poisoned[lane] = true
					traceDBAddCoverageMetric(&out, "pair_lanes_unmatched_or_unrepresentable_end", 1)
					break
				}
				start.published, item.published = true, true
				pairs++
				start = nil
			}
			if poisoned[lane] {
				break
			}
		}
		if !poisoned[lane] && start != nil {
			poisoned[lane] = true
			traceDBAddCoverageMetric(&out, "pair_lanes_unmatched_start", 1)
		}
		if poisoned[lane] {
			for _, index := range indexes {
				prepared[index].published = false
			}
			traceDBAddCoverageMetric(&out, "pair_lanes_poisoned", 1)
			traceDBAddCoverageMetric(&out, "raw_endpoints_withheld_poisoned_lane", int64(len(indexes)))
			continue
		}
		traceDBAddCoverageMetric(&out, "pair_lanes_published", 1)
		traceDBAddCoverageMetric(&out, "pairs_published", int64(pairs))
	}

	publishRows := make([]traceDBRawDMAWaitPrepared, 0, len(prepared))
	for _, item := range prepared {
		if item.published {
			publishRows = append(publishRows, item)
		}
	}
	sort.SliceStable(publishRows, func(i, j int) bool {
		if publishRows[i].raw.TimestampNS != publishRows[j].raw.TimestampNS {
			return publishRows[i].raw.TimestampNS < publishRows[j].raw.TimestampNS
		}
		return publishRows[i].ordinal < publishRows[j].ordinal
	})
	if len(publishRows) > 0 && sink == nil {
		err := &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
		out.Error = err.Error()
		return out, err
	}
	if sink != nil && (sink.stats.RowsAccepted < 0 ||
		len(publishRows) > math.MaxInt-sink.stats.RowsAccepted) {
		err := &traceDBOutputInvariantError{Reason: "invalid_raw_dma_wait_recovery_sequence"}
		out.Error = err.Error()
		return out, err
	}
	rendered := make([]renderedRow, 0, len(publishRows))
	for index, item := range publishRows {
		bodyText, ok := renderCanonicalPairPayload(item.payload)
		if !ok {
			return out, &traceDBOutputInvariantError{Reason: "invalid_raw_dma_wait_recovery_body"}
		}
		body := item.raw.Name + ": " + bodyText
		if !traceDBRawPairingWireParity(item.raw.Name, body, item.thread.TID, item.verdict) {
			return out, &traceDBOutputInvariantError{Reason: "raw_dma_wait_typed_wire_parity_mismatch"}
		}
		headerTGID := item.process.PID
		allowUnknownTGID := headerTGID == 0 && item.thread.TID != 0
		row, err := prepareTraceDBRenderedRowEnvelopeContext(
			ctx, int64(item.raw.TimestampNS), sink.stats.RowsAccepted+index,
			traceDBCommName(item.thread.Name, "unknown"),
			item.thread.TID, headerTGID, int64(item.raw.CPU),
			item.raw.Flags, item.raw.PreemptCount, allowUnknownTGID, body)
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
	if out.RowsEmitted == 0 {
		out.Metadata["publication_state"] = "complete_no_clean_pair_lane"
		out.Skipped = "raw DMA wait recovery complete: every exact pair lane was withheld by identity or topology"
	} else {
		out.Metadata["publication_state"] = "published_exact_clean_pair_lanes"
		out.Metadata["publication_contract"] = fmt.Sprintf(
			"endpoints=%d;pairs=%d; exact raw timestamp/cpu/flags/header and strict DMA key",
			out.RowsEmitted, out.Metrics["pairs_published"])
	}
	return out, nil
}

func traceDBRawDMAWaitPairPayload(raw traceDBRawDMAWaitRecord) pairRenderPayload {
	return pairRenderPayload{
		Kind: pairRenderDMAFence,
		Name: raw.Name,
		DMAFence: &pairDMAFencePayload{
			Driver: raw.Driver, DriverKnown: true,
			Timeline: raw.Timeline, TimelineKnown: true,
			NumberBits: 32,
			Context:    raw.Context, ContextKnown: true,
			Seqno: raw.Seqno, SeqnoKnown: true,
		},
	}
}

func traceDBRawDMAWaitCoordinateReason(raw traceDBRawDMAWaitRecord) string {
	if raw.TimestampNS > math.MaxInt64 {
		return "invalid_timestamp"
	}
	if !validTraceDBCPUIndex(int64(raw.CPU)) {
		return "invalid_cpu"
	}
	if raw.HeaderPID <= 0 || raw.HeaderPID > math.MaxInt32 {
		return "invalid_header_tid"
	}
	if raw.Flags < 0 || raw.Flags > math.MaxUint8 ||
		raw.PreemptCount < 0 || raw.PreemptCount > math.MaxUint8 {
		return "invalid_envelope_flags"
	}
	return ""
}
