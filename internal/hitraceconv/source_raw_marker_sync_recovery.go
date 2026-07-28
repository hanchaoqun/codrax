package hitraceconv

import (
	"context"
	"math"
	"sort"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBRawMarkerPair struct {
	begin traceDBRawMarkerRecord
	end   traceDBRawMarkerRecord
}

func newTraceDBRawMarkerSyncCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_marker_sync",
		Table:  "__raw_marker_sync__",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"authority":     "complete strict raw marker ledger over the physical print plus tracing_mark_write carrier census",
			"grammar":       "tracequery.DecodeTraceMarkEndpointPayload is the sole complete-payload B/E verdict",
			"stack":         "one exact LIFO stack per physical common_pid emitter; any rejected carrier/endpoint, orphan end, open begin, invalid interval or identity drift withholds that raw emitter lane whole",
			"identity":      "common_pid independently resolves to the same canonical host thread/process at both exact endpoints; payload PID remains marker namespace data",
			"deduplication": "an exact bounded semantic index suppresses only a raw pair already represented by an earlier DB candidate with equal host TID/TGID, marker PID, canonical owner, interval and name",
			"publication":   "DB-disjoint clean pairs submit to the existing single sync-span laminar authority; this exporter never writes B/E rows directly",
			"envelope":      "raw page CPU, common_flags and common_preempt_count are retained independently at begin and end",
		},
		Metadata: map[string]string{
			"publication_state": "unavailable",
		},
	}
}

func submitTraceDBRawMarkerSyncRecovery(
	ctx context.Context,
	inventory *traceDBSourceNameInventory,
	authority traceDBSchedulerAuthority,
	syncSpans *traceDBSyncSpanAuthority,
) (TraceDBCoverage, error) {
	out := newTraceDBRawMarkerSyncCoverage()
	if inventory == nil {
		out.Skipped = "raw marker sync recovery unavailable: immutable source inventory absent"
		return out, nil
	}
	out.Found = inventory.RawDecode.Found
	if inventory.RawDecode.Metadata["decode_state"] != "strict_target_ledger_complete" {
		out.Metadata["publication_state"] = "withheld_raw_decode_incomplete"
		out.Skipped = "raw marker sync recovery withheld: strict raw decode ledger incomplete"
		return out, nil
	}
	if !authority.initialized || !authority.complete || syncSpans == nil {
		return out, &traceDBOutputInvariantError{Reason: "missing_raw_marker_sync_authority"}
	}
	rows := inventory.RawMarkers
	out.RowsRead = len(rows)
	retained := inventory.RawDecode.Metrics["target_marker_sync_records_retained"] +
		inventory.RawDecode.Metrics["target_marker_sync_poison_records_retained"] +
		inventory.RawDecode.Metrics["target_marker_carrier_rejections_retained"]
	if retained != int64(len(rows)) {
		out.Metadata["publication_state"] = "withheld_raw_marker_census_mismatch"
		out.Skipped = "raw marker sync recovery withheld: retained endpoint/poison census mismatch"
		return out, nil
	}
	if len(rows) == 0 {
		out.Metadata["publication_state"] = "complete_no_sync_endpoint"
		out.Skipped = "raw marker sync recovery complete: no retained B/E endpoint"
		return out, nil
	}
	byEmitter := map[int64][]traceDBRawMarkerRecord{}
	for _, row := range rows {
		byEmitter[row.HeaderPID] = append(byEmitter[row.HeaderPID], row)
	}
	emitters := make([]int64, 0, len(byEmitter))
	for emitter := range byEmitter {
		emitters = append(emitters, emitter)
	}
	sort.Slice(emitters, func(i, j int) bool { return emitters[i] < emitters[j] })
	traceDBAddCoverageMetric(&out, "raw_emitter_lanes", int64(len(emitters)))

	candidates := make([]traceDBSyncSpanCandidate, 0, len(rows)/2)
	for _, emitter := range emitters {
		lane := byEmitter[emitter]
		sort.SliceStable(lane, func(i, j int) bool {
			if lane[i].TimestampNS != lane[j].TimestampNS {
				return lane[i].TimestampNS < lane[j].TimestampNS
			}
			return lane[i].PhysicalOrdinal < lane[j].PhysicalOrdinal
		})
		stack := make([]traceDBRawMarkerRecord, 0, 8)
		pairs := make([]traceDBRawMarkerPair, 0, len(lane)/2)
		poisonReason := ""
		lastTimestamp := uint64(0)
		lastOrdinal := int64(0)
		haveLast := false
		for _, row := range lane {
			if row.PhysicalOrdinal <= 0 ||
				haveLast && row.TimestampNS == lastTimestamp &&
					row.PhysicalOrdinal <= lastOrdinal {
				poisonReason = "invalid_physical_order"
				break
			}
			lastTimestamp, lastOrdinal, haveLast =
				row.TimestampNS, row.PhysicalOrdinal, true
			if !row.Admitted || row.RejectReason != "" {
				poisonReason = "rejected_endpoint_or_carrier"
				break
			}
			switch row.Action {
			case "B":
				stack = append(stack, row)
			case "E":
				if len(stack) == 0 {
					poisonReason = "orphan_end"
					break
				}
				begin := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if begin.TimestampNS > math.MaxInt64 || row.TimestampNS > math.MaxInt64 ||
					!traceDBWireIntervalRepresentable(
						int64(begin.TimestampNS), int64(row.TimestampNS)) {
					poisonReason = "unrepresentable_interval"
					break
				}
				pairs = append(pairs, traceDBRawMarkerPair{begin: begin, end: row})
			default:
				poisonReason = "invalid_sync_action"
			}
			if poisonReason != "" {
				break
			}
		}
		if poisonReason == "" && len(stack) != 0 {
			poisonReason = "open_begin"
		}
		if poisonReason != "" {
			traceDBAddCoverageMetric(&out, "raw_emitter_lanes_poisoned", 1)
			traceDBAddCoverageMetric(&out,
				"raw_emitter_lanes_poisoned_"+traceDBRawDecodeReasonMetric(poisonReason), 1)
			traceDBAddCoverageMetric(&out, "raw_endpoints_withheld_poisoned_lane", int64(len(lane)))
			continue
		}

		laneCandidates := make([]traceDBSyncSpanCandidate, 0, len(pairs))
		for _, pair := range pairs {
			candidate, reason := traceDBRawMarkerSyncCandidate(pair, authority)
			if reason != "" {
				poisonReason = reason
				break
			}
			laneCandidates = append(laneCandidates, candidate)
		}
		if poisonReason != "" {
			traceDBAddCoverageMetric(&out, "raw_emitter_lanes_poisoned", 1)
			traceDBAddCoverageMetric(&out,
				"raw_emitter_lanes_poisoned_"+traceDBRawDecodeReasonMetric(poisonReason), 1)
			traceDBAddCoverageMetric(&out, "raw_endpoints_withheld_poisoned_lane", int64(len(lane)))
			continue
		}
		traceDBAddCoverageMetric(&out, "raw_emitter_lanes_clean", 1)
		candidates = append(candidates, laneCandidates...)
	}

	for _, candidate := range candidates {
		exists, complete, err := syncSpans.hasSemanticCandidate(ctx, candidate)
		if err != nil {
			out.Error = err.Error()
			return out, err
		}
		if !complete {
			out.Metadata["publication_state"] = "withheld_cross_source_index_incomplete"
			out.Skipped = "raw marker sync recovery withheld: bounded exact DB candidate index incomplete"
			return out, nil
		}
		if exists {
			traceDBAddCoverageMetric(&out, "raw_pairs_existing_db_candidate", 1)
			continue
		}
		if err := syncSpans.submit(ctx, candidate); err != nil {
			out.Error = err.Error()
			return out, err
		}
		traceDBAddCoverageMetric(&out, "raw_pairs_submitted", 1)
	}
	out.Metadata["publication_state"] = "submitted_to_shared_sync_authority"
	out.Skipped = traceDBCountSummary(map[string]int{
		"existing_db_candidates": int(out.Metrics["raw_pairs_existing_db_candidate"]),
		"poisoned_emitter_lanes": int(out.Metrics["raw_emitter_lanes_poisoned"]),
	})
	return out, nil
}

func traceDBRawMarkerSyncCandidate(
	pair traceDBRawMarkerPair,
	authority traceDBSchedulerAuthority,
) (traceDBSyncSpanCandidate, string) {
	begin, end := pair.begin, pair.end
	beginVerdict := tracequery.DecodeTraceMarkEndpointPayload(begin.Buffer)
	endVerdict := tracequery.DecodeTraceMarkEndpointPayload(end.Buffer)
	if begin.HeaderPID <= 0 || begin.HeaderPID != end.HeaderPID ||
		begin.PayloadPID <= 0 || begin.PayloadPID > math.MaxInt32 ||
		begin.Action != "B" || end.Action != "E" ||
		!beginVerdict.Admitted || beginVerdict.Action != "B" ||
		int64(beginVerdict.SpanPID) != begin.PayloadPID ||
		beginVerdict.Name != begin.Name ||
		!endVerdict.Admitted || endVerdict.Action != "E" ||
		int64(endVerdict.SpanPID) != end.PayloadPID ||
		!traceDBCallstackSpanName(begin.Name) ||
		begin.TimestampNS > math.MaxInt64 || end.TimestampNS > math.MaxInt64 ||
		!validTraceDBCPUIndex(int64(begin.CPU)) ||
		!validTraceDBCPUIndex(int64(end.CPU)) ||
		begin.Flags < 0 || begin.Flags > math.MaxUint8 ||
		end.Flags < 0 || end.Flags > math.MaxUint8 ||
		begin.PreemptCount < 0 || begin.PreemptCount > math.MaxUint8 ||
		end.PreemptCount < 0 || end.PreemptCount > math.MaxUint8 {
		return traceDBSyncSpanCandidate{}, "invalid_endpoint"
	}
	start, finish := int64(begin.TimestampNS), int64(end.TimestampNS)
	beginThread, beginProcess, reason :=
		traceDBResolveRawPublicTID(authority, begin.HeaderPID, start)
	if reason != "" {
		return traceDBSyncSpanCandidate{}, "begin_" + reason
	}
	endThread, endProcess, reason :=
		traceDBResolveRawPublicTID(authority, end.HeaderPID, finish)
	if reason != "" {
		return traceDBSyncSpanCandidate{}, "end_" + reason
	}
	if beginThread.ITID != endThread.ITID ||
		beginThread.IPID != endThread.IPID ||
		beginProcess.IPID != endProcess.IPID ||
		beginProcess.PID <= 0 || beginProcess.PID != endProcess.PID {
		return traceDBSyncSpanCandidate{}, "identity_drift"
	}
	if !authority.threadSourceIntervalAllows(beginThread.ITID, start, finish) {
		return traceDBSyncSpanCandidate{}, "lifecycle_interval_rejected"
	}
	candidate := traceDBSyncSpanCandidate{
		Producer:           traceDBSyncSpanProducerSourceRawMarker,
		StableKind:         traceDBSyncSpanStableSourceRawOrdinal,
		StableID:           begin.PhysicalOrdinal,
		HeaderTID:          beginThread.TID,
		HeaderTGID:         beginProcess.PID,
		MarkerPID:          begin.PayloadPID,
		MarkerPIDKnown:     true,
		CanonicalITID:      beginThread.ITID,
		CanonicalITIDKnown: true,
		OwnerIPID:          beginProcess.IPID,
		OwnerIPIDKnown:     true,
		Start:              start,
		End:                finish,
		StartCPU:           int64(begin.CPU),
		EndCPU:             int64(end.CPU),
		StartFlags:         begin.Flags,
		EndFlags:           end.Flags,
		StartPreemptCount:  begin.PreemptCount,
		EndPreemptCount:    end.PreemptCount,
		StartMarkerBody:    begin.Buffer,
		EndMarkerBody:      end.Buffer,
		CPUPlacement:       traceDBSyncSpanCPUPlacementKnown,
		StartCPUProvenance: traceDBSyncSpanCPUSourceRawPage,
		EndCPUProvenance:   traceDBSyncSpanCPUSourceRawPage,
		Task:               traceDBCommName(beginThread.Name, "unknown"),
		Name:               begin.Name,
		NameProvenance:     traceDBSyncSpanNameSourceRawMarker,
	}
	if err := validateTraceDBSyncSpanCandidate(candidate); err != nil {
		return traceDBSyncSpanCandidate{}, "candidate_validation_failed"
	}
	return candidate, ""
}
