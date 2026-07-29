package hitraceconv

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBRawMarkerPair struct {
	begin traceDBRawMarkerRecord
	end   traceDBRawMarkerRecord
}

type traceDBRawMarkerPairWitness struct {
	emitter  int64
	start    uint64
	end      uint64
	duration uint64
	name     string
}

func newTraceDBRawMarkerSyncCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_marker_sync",
		Table:  "__raw_marker_sync__",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"authority":     "complete strict raw marker ledger over the physical print plus tracing_mark_write carrier census",
			"grammar":       "tracequery.DecodeTraceMarkEndpointPayload is the sole complete-payload B/E verdict",
			"stack":         "one exact LIFO stack per physical common_pid emitter; orphan ends, trailing open begins, invalid/overflowed closed intervals and already-paired validation failures are withheld locally, while invalid physical ordering or an unclassified/rejected carrier keeps whole-lane fail-closed scope",
			"identity":      "common_pid independently resolves to the same canonical host thread/process at both exact endpoints; payload PID remains marker namespace data",
			"deduplication": "an exact bounded semantic index suppresses a raw pair only when an equal host TID/TGID, marker PID, canonical owner, interval and name DB candidate survives its producer-local fence/poison gate; one unique locally admitted CPU-unavailable callstack collision, or a name-drift collision whose CPU-known DB name is not losslessly standard-representable, is candidate-superseded and replaced by the exact raw B/E pair; a locally suppressed DB candidate cannot erase the raw alternative",
			"diagnostics":   "closed raw pairs expose exact zero-start, long-duration and bounded longest-pair witnesses before publication decisions; these observations never admit or reject a pair",
			"name_drift":    "a raw pair sharing exact host/payload/canonical identity and interval with a locally admitted DB candidate but not its name is withheld locally unless that collision is one unique CPU-unavailable callstack candidate or one unique CPU-known callstack candidate whose DB name cannot round-trip through the standard trace-mark grammar; only those candidate-level cases are superseded and replaced by the authoritative raw name/envelopes, while a standard-representable DB name remains authoritative",
			"publication":   "DB-disjoint clean pairs submit to the existing single sync-span laminar authority; this exporter never writes B/E rows directly",
			"envelope":      "raw page CPU, common_flags and common_preempt_count are retained independently at begin and end",
			"validation":    "post-pair endpoint validation reports one closed first-failure typed reason for header/payload/action/name/timestamp/CPU/flags/preempt fields; no reason changes admission",
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
	if geometry := inventory.RawDecode.Metadata["marker_format_geometry_witnesses"]; geometry != "" {
		out.Metadata["marker_format_geometry_witnesses"] = geometry
	}
	if inventory.RawDecode.Metadata["decode_state"] != "strict_target_ledger_complete" {
		out.Metadata["publication_state"] = "withheld_raw_decode_incomplete"
		out.Skipped = "raw marker sync recovery withheld: strict raw decode ledger incomplete"
		return out, nil
	}
	if !authority.initialized || !authority.complete || syncSpans == nil {
		return out, &traceDBOutputInvariantError{Reason: "missing_raw_marker_sync_authority"}
	}
	rows := traceDBRawMarkerSyncRows(inventory.RawMarkers)
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
	markerFirstTS, markerLastTS := rows[0].TimestampNS, rows[0].TimestampNS
	for _, row := range rows[1:] {
		if row.TimestampNS < markerFirstTS {
			markerFirstTS = row.TimestampNS
		}
		if row.TimestampNS > markerLastTS {
			markerLastTS = row.TimestampNS
		}
	}
	out.Metadata["raw_marker_first_timestamp_ns"] = strconv.FormatUint(markerFirstTS, 10)
	out.Metadata["raw_marker_last_timestamp_ns"] = strconv.FormatUint(markerLastTS, 10)
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
	longestPairs := make([]traceDBRawMarkerPairWitness, 0, 8)
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
		localizedWithholding := false
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
					localizedWithholding = true
					traceDBAddCoverageMetric(&out, "raw_orphan_endpoints_withheld", 1)
					continue
				}
				begin := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				traceDBAddCoverageMetric(&out, "raw_pairs_structurally_closed", 1)
				if begin.TimestampNS == 0 {
					traceDBAddCoverageMetric(&out, "raw_pairs_begin_timestamp_zero", 1)
				}
				if begin.TimestampNS == markerFirstTS {
					traceDBAddCoverageMetric(&out, "raw_pairs_begin_at_marker_first_timestamp", 1)
				}
				if row.TimestampNS >= begin.TimestampNS {
					duration := row.TimestampNS - begin.TimestampNS
					switch {
					case duration >= 1_000_000_000:
						traceDBAddCoverageMetric(&out, "raw_pairs_duration_ge_1s", 1)
						fallthrough
					case duration >= 100_000_000:
						traceDBAddCoverageMetric(&out, "raw_pairs_duration_ge_100ms", 1)
					}
					markerWindow := markerLastTS - markerFirstTS
					if markerWindow > 0 &&
						duration >= markerWindow/2+markerWindow%2 {
						traceDBAddCoverageMetric(&out, "raw_pairs_cover_at_least_half_marker_window", 1)
					}
					longestPairs = traceDBRawMarkerRetainLongestPair(longestPairs,
						traceDBRawMarkerPairWitness{
							emitter: emitter, start: begin.TimestampNS,
							end: row.TimestampNS, duration: duration, name: begin.Name,
						}, 8)
				}
				if begin.TimestampNS > math.MaxInt64 || row.TimestampNS > math.MaxInt64 ||
					!traceDBWireIntervalRepresentable(
						int64(begin.TimestampNS), int64(row.TimestampNS)) {
					localizedWithholding = true
					traceDBAddCoverageMetric(
						&out, "raw_pairs_withheld_unrepresentable_interval", 1)
					continue
				}
				pairs = append(pairs, traceDBRawMarkerPair{begin: begin, end: row})
			default:
				poisonReason = "invalid_sync_action"
			}
			if poisonReason != "" {
				break
			}
		}
		if poisonReason != "" {
			traceDBAddCoverageMetric(&out, "raw_emitter_lanes_poisoned", 1)
			traceDBAddCoverageMetric(&out,
				"raw_emitter_lanes_poisoned_"+traceDBRawDecodeReasonMetric(poisonReason), 1)
			traceDBAddCoverageMetric(&out, "raw_endpoints_withheld_poisoned_lane", int64(len(lane)))
			continue
		}
		if len(stack) != 0 {
			localizedWithholding = true
			traceDBAddCoverageMetric(&out, "raw_open_begins_withheld", int64(len(stack)))
		}

		laneCandidates := make([]traceDBSyncSpanCandidate, 0, len(pairs))
		for _, pair := range pairs {
			candidate, reason := traceDBRawMarkerSyncCandidate(pair, authority)
			if reason != "" {
				localizedWithholding = true
				traceDBAddCoverageMetric(&out, "raw_pairs_withheld_local_validation", 1)
				traceDBAddCoverageMetric(&out,
					"raw_pairs_withheld_"+traceDBRawDecodeReasonMetric(reason), 1)
				continue
			}
			laneCandidates = append(laneCandidates, candidate)
		}
		if localizedWithholding {
			traceDBAddCoverageMetric(&out, "raw_emitter_lanes_partial", 1)
			if len(laneCandidates) > 0 {
				traceDBAddCoverageMetric(&out, "raw_emitter_lanes_partially_salvaged", 1)
			}
		} else {
			traceDBAddCoverageMetric(&out, "raw_emitter_lanes_clean", 1)
		}
		candidates = append(candidates, laneCandidates...)
	}
	if len(longestPairs) > 0 {
		out.Metadata["raw_marker_longest_pair_witnesses"] =
			traceDBRawMarkerPairWitnesses(longestPairs)
	}

	rawIntervalMultiplicity := traceDBRawMarkerIntervalMultiplicity(candidates)
	for _, candidate := range candidates {
		rawIntervalKey, rawIntervalKeyOK :=
			traceDBSyncSpanCandidateSemanticKey(candidate)
		rawIntervalKey.Name = ""
		rawIntervalCount := rawIntervalMultiplicity[rawIntervalKey]
		if !rawIntervalKeyOK {
			rawIntervalCount = 0
		}
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
			locallyAdmitted, admittedComplete, admittedErr :=
				syncSpans.hasLocallyAdmittedSemanticCandidate(ctx, candidate)
			if admittedErr != nil {
				out.Error = admittedErr.Error()
				return out, admittedErr
			}
			if !admittedComplete {
				out.Metadata["publication_state"] = "withheld_cross_source_index_incomplete"
				out.Skipped = "raw marker sync recovery withheld: bounded DB local-admission index incomplete"
				return out, nil
			}
			if locallyAdmitted {
				census, complete, censusErr := traceDBRecordRawMarkerCollisionCensus(
					ctx, &out, syncSpans, candidate, "exact_semantic")
				if censusErr != nil {
					out.Error = censusErr.Error()
					return out, censusErr
				}
				replaced, replaceErr := traceDBReplaceRawMarkerAuthoritativeCollision(
					ctx, &out, syncSpans, candidate, census, complete,
					rawIntervalCount, "exact_semantic")
				if replaceErr != nil {
					out.Error = replaceErr.Error()
					return out, replaceErr
				}
				if replaced {
					continue
				}
				if !complete {
					traceDBAddCoverageMetric(&out,
						"raw_pairs_exact_semantic_collision_census_incomplete", 1)
				}
				if complete && census.Total == 0 {
					err := &traceDBOutputInvariantError{
						Reason: "raw_marker_exact_collision_census_empty",
					}
					out.Error = err.Error()
					return out, err
				}
				traceDBAddCoverageMetric(&out, "raw_pairs_existing_db_candidate", 1)
				continue
			}
			traceDBAddCoverageMetric(
				&out, "raw_pairs_existing_db_candidate_locally_suppressed", 1)
		}
		if !exists {
			intervalCollision, intervalComplete, intervalErr :=
				syncSpans.hasIntervalIdentityCandidate(ctx, candidate)
			if intervalErr != nil {
				out.Error = intervalErr.Error()
				return out, intervalErr
			}
			if !intervalComplete {
				out.Metadata["publication_state"] = "withheld_cross_source_index_incomplete"
				out.Skipped = "raw marker sync recovery withheld: bounded exact DB interval index incomplete"
				return out, nil
			}
			if intervalCollision {
				locallyAdmitted, admittedComplete, admittedErr :=
					syncSpans.hasLocallyAdmittedIntervalIdentityCandidate(ctx, candidate)
				if admittedErr != nil {
					out.Error = admittedErr.Error()
					return out, admittedErr
				}
				if !admittedComplete {
					out.Metadata["publication_state"] = "withheld_cross_source_index_incomplete"
					out.Skipped = "raw marker sync recovery withheld: bounded DB interval local-admission index incomplete"
					return out, nil
				}
				if locallyAdmitted {
					census, complete, censusErr := traceDBRecordRawMarkerCollisionCensus(
						ctx, &out, syncSpans, candidate, "name_drift")
					if censusErr != nil {
						out.Error = censusErr.Error()
						return out, censusErr
					}
					replaced, replaceErr := traceDBReplaceRawMarkerAuthoritativeCollision(
						ctx, &out, syncSpans, candidate, census, complete,
						rawIntervalCount, "name_drift")
					if replaceErr != nil {
						out.Error = replaceErr.Error()
						return out, replaceErr
					}
					if replaced {
						continue
					}
					traceDBAddCoverageMetric(
						&out, "raw_pairs_withheld_exact_interval_name_drift", 1)
					continue
				}
				traceDBAddCoverageMetric(
					&out, "raw_pairs_interval_collision_locally_suppressed", 1)
			}
		}
		if err := syncSpans.submit(ctx, candidate); err != nil {
			out.Error = err.Error()
			return out, err
		}
		traceDBAddCoverageMetric(&out, "raw_pairs_submitted", 1)
	}
	out.Metadata["publication_state"] = "submitted_to_shared_sync_authority"
	out.Skipped = traceDBCountSummary(map[string]int{
		"exact_interval_name_drift":      int(out.Metrics["raw_pairs_withheld_exact_interval_name_drift"]),
		"existing_db_candidates":         int(out.Metrics["raw_pairs_existing_db_candidate"]),
		"locally_suppressed_db_exact":    int(out.Metrics["raw_pairs_existing_db_candidate_locally_suppressed"]),
		"locally_suppressed_db_interval": int(out.Metrics["raw_pairs_interval_collision_locally_suppressed"]),
		"local_validation_pairs":         int(out.Metrics["raw_pairs_withheld_local_validation"]),
		"open_begins":                    int(out.Metrics["raw_open_begins_withheld"]),
		"orphan_endpoints":               int(out.Metrics["raw_orphan_endpoints_withheld"]),
		"poisoned_emitter_lanes":         int(out.Metrics["raw_emitter_lanes_poisoned"]),
		"unrepresentable_interval_pairs": int(out.Metrics["raw_pairs_withheld_unrepresentable_interval"]),
	})
	return out, nil
}

func traceDBRawMarkerIntervalMultiplicity(
	candidates []traceDBSyncSpanCandidate,
) map[traceDBSyncSpanSemanticKey]int {
	out := make(map[traceDBSyncSpanSemanticKey]int, len(candidates))
	for _, candidate := range candidates {
		key, ok := traceDBSyncSpanCandidateSemanticKey(candidate)
		if !ok {
			continue
		}
		key.Name = ""
		out[key]++
	}
	return out
}

func traceDBRecordRawMarkerCollisionCensus(
	ctx context.Context,
	out *TraceDBCoverage,
	syncSpans *traceDBSyncSpanAuthority,
	candidate traceDBSyncSpanCandidate,
	shape string,
) (traceDBSyncSpanIntervalCollisionCensus, bool, error) {
	if out == nil || syncSpans == nil ||
		shape != "exact_semantic" && shape != "name_drift" {
		return traceDBSyncSpanIntervalCollisionCensus{}, false,
			&traceDBOutputInvariantError{Reason: "invalid_raw_marker_collision_census_request"}
	}
	census, complete, err :=
		syncSpans.censusLocallyAdmittedIntervalIdentityCandidates(ctx, candidate)
	if err != nil {
		return traceDBSyncSpanIntervalCollisionCensus{}, false, err
	}
	if !complete {
		traceDBAddCoverageMetric(out, "raw_collision_census_incomplete", 1)
		return traceDBSyncSpanIntervalCollisionCensus{}, false, nil
	}
	traceDBAddCoverageMetric(out, "raw_collision_candidate_rows", census.Total)
	traceDBAddCoverageMetric(out,
		"raw_collision_callstack_cpu_known_candidate_rows", census.CallstackCPUKnown)
	traceDBAddCoverageMetric(out,
		"raw_collision_callstack_cpu_unavailable_candidate_rows",
		census.CallstackCPUUnavailable)
	traceDBAddCoverageMetric(out, "raw_collision_other_candidate_rows", census.Other())
	switch {
	case census.Total == 1 && census.CallstackCPUUnavailable == 1:
		traceDBAddCoverageMetric(out,
			"raw_pairs_"+shape+"_unique_cpu_unavailable_callstack_candidate", 1)
		traceDBAddCoverageMetric(out,
			"raw_pairs_unique_cpu_unavailable_callstack_candidate", 1)
	case census.Total == 1 && census.CallstackCPUKnown == 1:
		traceDBAddCoverageMetric(out,
			"raw_pairs_"+shape+"_unique_cpu_known_callstack_candidate", 1)
		traceDBAddCoverageMetric(out,
			"raw_pairs_unique_cpu_known_callstack_candidate", 1)
	case census.Total == 1 && census.Other() == 1:
		traceDBAddCoverageMetric(out,
			"raw_pairs_"+shape+"_unique_other_candidate", 1)
		traceDBAddCoverageMetric(out, "raw_pairs_unique_other_candidate", 1)
	case census.Total > 1:
		traceDBAddCoverageMetric(out,
			"raw_pairs_"+shape+"_ambiguous_candidate_set", 1)
		traceDBAddCoverageMetric(out, "raw_pairs_ambiguous_candidate_set", 1)
	default:
		traceDBAddCoverageMetric(out,
			"raw_pairs_"+shape+"_collision_census_empty", 1)
		traceDBAddCoverageMetric(out, "raw_pairs_collision_census_empty", 1)
	}
	return census, true, nil
}

func traceDBReplaceRawMarkerAuthoritativeCollision(
	ctx context.Context,
	out *TraceDBCoverage,
	syncSpans *traceDBSyncSpanAuthority,
	candidate traceDBSyncSpanCandidate,
	census traceDBSyncSpanIntervalCollisionCensus,
	complete bool,
	rawIntervalCount int,
	shape string,
) (bool, error) {
	if !complete || census.Total != 1 {
		return false, nil
	}
	if rawIntervalCount != 1 {
		traceDBAddCoverageMetric(out,
			"raw_pairs_authoritative_replacement_withheld_ambiguous_raw_interval", 1)
		return false, nil
	}
	replacementKind := ""
	var replaced, replacementComplete bool
	var err error
	switch {
	case census.CallstackCPUUnavailable == 1:
		replacementKind = "cpu_unavailable"
		replaced, replacementComplete, err =
			syncSpans.supersedeUniqueLocallyAdmittedCPUUnavailableCallstackCandidate(
				ctx, candidate)
	case shape == "name_drift" && census.CallstackCPUKnown == 1:
		replacementKind = "name_unrepresentable"
		replaced, replacementComplete, err =
			syncSpans.supersedeUniqueLocallyAdmittedNameUnrepresentableCallstackCandidate(
				ctx, candidate)
	default:
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !replacementComplete {
		traceDBAddCoverageMetric(out,
			"raw_authoritative_replacement_incomplete", 1)
		return false, nil
	}
	if !replaced {
		if replacementKind == "name_unrepresentable" {
			traceDBAddCoverageMetric(out,
				"raw_pairs_name_drift_cpu_known_standard_representable_withheld", 1)
			return false, nil
		}
		return false, &traceDBOutputInvariantError{
			Reason: "raw_cpu_unavailable_replacement_candidate_disappeared",
		}
	}
	if err := syncSpans.submit(ctx, candidate); err != nil {
		return false, err
	}
	traceDBAddCoverageMetric(out,
		"raw_pairs_"+shape+"_"+replacementKind+"_callstack_replaced", 1)
	traceDBAddCoverageMetric(out,
		"raw_pairs_"+replacementKind+"_callstack_replaced", 1)
	traceDBAddCoverageMetric(out, "raw_pairs_submitted", 1)
	return true, nil
}

func traceDBRawMarkerSyncRows(rows []traceDBRawMarkerRecord) []traceDBRawMarkerRecord {
	out := make([]traceDBRawMarkerRecord, 0, len(rows))
	for _, row := range rows {
		if row.Action == "B" || row.Action == "E" || !row.Admitted {
			out = append(out, row)
		}
	}
	return out
}

func traceDBRawMarkerSyncCandidate(
	pair traceDBRawMarkerPair,
	authority traceDBSchedulerAuthority,
) (traceDBSyncSpanCandidate, string) {
	begin, end := pair.begin, pair.end
	beginVerdict := tracequery.DecodeTraceMarkEndpointPayload(begin.Buffer)
	endVerdict := tracequery.DecodeTraceMarkEndpointPayload(end.Buffer)
	switch {
	case begin.HeaderPID <= 0:
		return traceDBSyncSpanCandidate{}, "invalid_begin_header_pid"
	case end.HeaderPID <= 0:
		return traceDBSyncSpanCandidate{}, "invalid_end_header_pid"
	case begin.HeaderPID != end.HeaderPID:
		return traceDBSyncSpanCandidate{}, "header_pid_mismatch"
	case begin.PayloadPID <= 0 || begin.PayloadPID > math.MaxInt32:
		return traceDBSyncSpanCandidate{}, "invalid_begin_payload_pid"
	case end.PayloadPID <= 0 || end.PayloadPID > math.MaxInt32:
		return traceDBSyncSpanCandidate{}, "invalid_end_payload_pid"
	case begin.Action != "B":
		return traceDBSyncSpanCandidate{}, "invalid_begin_action"
	case end.Action != "E":
		return traceDBSyncSpanCandidate{}, "invalid_end_action"
	case !beginVerdict.Admitted:
		return traceDBSyncSpanCandidate{}, "begin_payload_not_admitted"
	case beginVerdict.Action != "B":
		return traceDBSyncSpanCandidate{}, "begin_payload_action_mismatch"
	case int64(beginVerdict.SpanPID) != begin.PayloadPID:
		return traceDBSyncSpanCandidate{}, "begin_payload_pid_mismatch"
	case beginVerdict.Name != begin.Name:
		return traceDBSyncSpanCandidate{}, "begin_payload_name_mismatch"
	case !endVerdict.Admitted:
		return traceDBSyncSpanCandidate{}, "end_payload_not_admitted"
	case endVerdict.Action != "E":
		return traceDBSyncSpanCandidate{}, "end_payload_action_mismatch"
	case int64(endVerdict.SpanPID) != end.PayloadPID:
		return traceDBSyncSpanCandidate{}, "end_payload_pid_mismatch"
	case !traceDBCallstackSpanName(begin.Name):
		return traceDBSyncSpanCandidate{}, "invalid_span_name"
	case begin.TimestampNS > math.MaxInt64:
		return traceDBSyncSpanCandidate{}, "begin_timestamp_overflow"
	case end.TimestampNS > math.MaxInt64:
		return traceDBSyncSpanCandidate{}, "end_timestamp_overflow"
	case !validTraceDBCPUIndex(int64(begin.CPU)):
		return traceDBSyncSpanCandidate{}, "invalid_begin_cpu"
	case !validTraceDBCPUIndex(int64(end.CPU)):
		return traceDBSyncSpanCandidate{}, "invalid_end_cpu"
	case begin.Flags < 0 || begin.Flags > math.MaxUint8:
		return traceDBSyncSpanCandidate{}, "invalid_begin_flags"
	case end.Flags < 0 || end.Flags > math.MaxUint8:
		return traceDBSyncSpanCandidate{}, "invalid_end_flags"
	case begin.PreemptCount < 0 || begin.PreemptCount > math.MaxUint8:
		return traceDBSyncSpanCandidate{}, "invalid_begin_preempt_count"
	case end.PreemptCount < 0 || end.PreemptCount > math.MaxUint8:
		return traceDBSyncSpanCandidate{}, "invalid_end_preempt_count"
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

func traceDBRawMarkerRetainLongestPair(
	current []traceDBRawMarkerPairWitness,
	candidate traceDBRawMarkerPairWitness,
	limit int,
) []traceDBRawMarkerPairWitness {
	if limit <= 0 {
		return current
	}
	current = append(current, candidate)
	sort.SliceStable(current, func(i, j int) bool {
		if current[i].duration != current[j].duration {
			return current[i].duration > current[j].duration
		}
		if current[i].start != current[j].start {
			return current[i].start < current[j].start
		}
		if current[i].emitter != current[j].emitter {
			return current[i].emitter < current[j].emitter
		}
		return current[i].name < current[j].name
	})
	if len(current) > limit {
		current = current[:limit]
	}
	return current
}

func traceDBRawMarkerPairWitnesses(items []traceDBRawMarkerPairWitness) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf(
			"emitter=%d/start_ns=%d/end_ns=%d/duration_ns=%d/name=%s",
			item.emitter, item.start, item.end, item.duration,
			traceDBRawMarkerNameWitness(item.name)))
	}
	return strings.Join(parts, ";")
}

func traceDBRawMarkerNameWitness(name string) string {
	if len(name) <= 128 && traceDBSinglePhysicalLine(name, false) {
		return strconv.Quote(name)
	}
	sum := sha256.Sum256([]byte(name))
	return fmt.Sprintf("sha256:%x/bytes:%d", sum, len(name))
}
