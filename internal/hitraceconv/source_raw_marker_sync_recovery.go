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

const (
	traceDBRawMarkerLocalValidationWitnessPerReasonCap = 4
	traceDBRawMarkerLocalValidationReasonCap           = 8
	traceDBRawMarkerLocalFenceWitnessCap               = 8
)

type traceDBCallstackNullDurationClosureKey struct {
	HeaderTID     int64
	HeaderTGID    int64
	MarkerPID     int64
	CanonicalITID int64
	OwnerIPID     int64
	Start         int64
	Name          string
}

type traceDBRawMarkerPhysicalStartKey struct {
	HeaderPID int64
	MarkerPID int64
	Start     uint64
	Name      string
}

type traceDBRawMarkerRejectedClosedStart struct {
	CountByReason map[string]int
}

func newTraceDBRawMarkerSyncCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_marker_sync",
		Table:  "__raw_marker_sync__",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"authority":     "complete strict raw marker ledger over the physical print plus tracing_mark_write carrier census",
			"grammar":       "tracequery.DecodeTraceMarkEndpointPayload is the sole complete-payload B/E verdict",
			"stack":         "one exact LIFO stack per physical common_pid emitter and ordered local segment; orphan ends, trailing open begins, invalid/overflowed closed intervals and already-paired validation failures are withheld locally; one classified rejected endpoint/carrier is a hard local segment fence which withholds the rejected row plus the open stack and forbids pairing across it, while invalid physical ordering or an unclassified admitted action keeps whole-lane fail-closed scope",
			"identity":      "common_pid independently resolves to the same canonical host thread/process at both exact endpoints; payload PID remains marker namespace data",
			"zero_pid":      "only the exact official OpenHarmony pid/name/start producer may interpret source payload PID zero as no TGID override; both endpoints must agree, canonical common_pid ownership must remain stable, and the public standard payload uses that proven host TGID while nonzero namespace PID remains verbatim",
			"name":          "an exact strictly decoded OpenHarmony print or tracing_mark_write body follows the shared official PrintEventParser semantics by removing only trailing ASCII U+0020 from the public B name when the resulting nonempty name passes the complete span-name predicate; source text remains retained in the raw ledger and every other event family/edge/control/text shape stays withheld",
			"deduplication": "an exact bounded semantic index suppresses a raw pair only when an equal host TID/TGID, marker PID, canonical owner, interval and name DB candidate survives its producer-local fence/poison gate; one unique locally admitted CPU-unavailable callstack collision, or a name-drift collision whose CPU-known DB name is not losslessly standard-representable, is candidate-superseded and replaced by the exact raw B/E pair; a locally suppressed DB candidate cannot erase the raw alternative",
			"diagnostics":   "closed raw pairs expose exact zero-start, long-duration and bounded longest-pair witnesses before publication decisions; these observations never admit or reject a pair",
			"name_drift":    "a raw pair sharing exact host/payload/canonical identity and interval with a locally admitted DB candidate but not its name is withheld locally unless that collision is one unique CPU-unavailable callstack candidate or one unique CPU-known callstack candidate whose DB name cannot round-trip through the standard trace-mark grammar; only those candidate-level cases are superseded and replaced by the authoritative raw name/envelopes, while a standard-representable DB name remains authoritative",
			"publication":   "DB-disjoint clean pairs submit to the existing single sync-span laminar authority; this exporter never writes B/E rows directly",
			"envelope":      "raw page CPU, common_flags and common_preempt_count are retained independently at begin and end",
			"validation":    "post-pair endpoint validation reports one closed first-failure typed reason for header/payload/action/name/timestamp/CPU/flags/preempt fields; no reason changes admission",
			"local_fence":   "bounded exact emitter/physical-ordinal/timestamp/reason witnesses diagnose rejected endpoint/carrier segment fences; witnesses never admit a row or bridge an open stack",
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
	if !traceDBRawDecodeFamilyComplete(
		inventory.RawDecode, traceDBRawRetentionMarker) {
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
		if err := traceDBApplyNullDurationRawClosureCensus(
			&out, syncSpans, nil, nil, nil); err != nil {
			out.Error = err.Error()
			return out, err
		}
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
	localValidationWitnessTotals := map[string]int{}
	localValidationWitnesses := map[string][]string{}
	localFenceWitnesses := make([]string, 0, traceDBRawMarkerLocalFenceWitnessCap)
	localFenceWitnessesTotal := 0
	rejectedClosedStarts :=
		map[traceDBRawMarkerPhysicalStartKey]traceDBRawMarkerRejectedClosedStart{}
	openBegins := map[traceDBRawMarkerPhysicalStartKey]int{}
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
				localizedWithholding = true
				reason := firstNonEmpty(row.RejectReason, "rejected_endpoint_or_carrier")
				reasonMetric := traceDBRawDecodeReasonMetric(reason)
				traceDBAddCoverageMetric(&out, "raw_marker_local_fences", 1)
				traceDBAddCoverageMetric(
					&out, "raw_marker_local_fences_"+reasonMetric, 1)
				traceDBAddCoverageMetric(
					&out, "raw_records_withheld_local_fence", 1)
				localFenceWitnessesTotal++
				if len(localFenceWitnesses) < traceDBRawMarkerLocalFenceWitnessCap {
					localFenceWitnesses = append(localFenceWitnesses,
						fmt.Sprintf("emitter=%d/ordinal=%d/timestamp_ns=%d/reason=%s",
							emitter, row.PhysicalOrdinal, row.TimestampNS, reasonMetric))
				}
				if len(stack) > 0 {
					traceDBAddCoverageMetric(
						&out, "raw_open_begins_withheld", int64(len(stack)))
					traceDBAddCoverageMetric(
						&out, "raw_open_begins_withheld_at_local_fence", int64(len(stack)))
					for _, begin := range stack {
						openBegins[traceDBRawMarkerPhysicalStartKeyFromRecord(begin)]++
					}
					stack = stack[:0]
				}
				continue
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
			for _, begin := range stack {
				openBegins[traceDBRawMarkerPhysicalStartKeyFromRecord(begin)]++
			}
		}

		laneCandidates := make([]traceDBSyncSpanCandidate, 0, len(pairs))
		for _, pair := range pairs {
			candidate, reason := traceDBRawMarkerSyncCandidate(pair, authority)
			if reason != "" {
				localizedWithholding = true
				reasonMetric := traceDBRawDecodeReasonMetric(reason)
				localValidationWitnessTotals[reasonMetric]++
				if len(localValidationWitnesses[reasonMetric]) <
					traceDBRawMarkerLocalValidationWitnessPerReasonCap {
					localValidationWitnesses[reasonMetric] = append(
						localValidationWitnesses[reasonMetric],
						traceDBRawMarkerLocalValidationWitness(pair, reason))
				}
				key := traceDBRawMarkerPhysicalStartKeyFromRecord(pair.begin)
				disposition := rejectedClosedStarts[key]
				if disposition.CountByReason == nil {
					disposition.CountByReason = map[string]int{}
				}
				disposition.CountByReason[reasonMetric]++
				rejectedClosedStarts[key] = disposition
				traceDBAddCoverageMetric(&out, "raw_pairs_withheld_local_validation", 1)
				traceDBAddCoverageMetric(&out,
					"raw_pairs_withheld_"+traceDBRawDecodeReasonMetric(reason), 1)
				continue
			}
			if pair.begin.PayloadPID == 0 {
				traceDBAddCoverageMetric(&out,
					"raw_pairs_official_zero_pid_header_identity_normalized", 1)
			}
			if pair.begin.Name != candidate.Name {
				traceDBAddCoverageMetric(&out,
					"raw_pairs_official_trailing_space_name_normalized", 1)
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
	if len(localFenceWitnesses) > 0 {
		out.Metadata["raw_marker_local_fence_witnesses"] =
			strings.Join(localFenceWitnesses, ";")
		traceDBAddCoverageMetric(&out,
			"raw_marker_local_fence_witnesses_emitted",
			int64(len(localFenceWitnesses)))
		if omitted := localFenceWitnessesTotal - len(localFenceWitnesses); omitted > 0 {
			traceDBAddCoverageMetric(&out,
				"raw_marker_local_fence_witnesses_omitted", int64(omitted))
		}
	}
	if len(longestPairs) > 0 {
		out.Metadata["raw_marker_longest_pair_witnesses"] =
			traceDBRawMarkerPairWitnesses(longestPairs)
	}
	if len(localValidationWitnesses) > 0 {
		reasons := make([]string, 0, len(localValidationWitnesses))
		for reason := range localValidationWitnesses {
			reasons = append(reasons, reason)
		}
		sort.Strings(reasons)
		if len(reasons) > traceDBRawMarkerLocalValidationReasonCap {
			traceDBAddCoverageMetric(&out,
				"raw_marker_local_validation_witness_reason_classes_omitted",
				int64(len(reasons)-traceDBRawMarkerLocalValidationReasonCap))
			reasons = reasons[:traceDBRawMarkerLocalValidationReasonCap]
		}
		flattened := make([]string, 0,
			len(reasons)*traceDBRawMarkerLocalValidationWitnessPerReasonCap)
		for _, reason := range reasons {
			witnesses := localValidationWitnesses[reason]
			flattened = append(flattened, witnesses...)
			traceDBAddCoverageMetric(&out,
				"raw_marker_local_validation_witnesses_"+reason+"_emitted",
				int64(len(witnesses)))
			if omitted := localValidationWitnessTotals[reason] - len(witnesses); omitted > 0 {
				traceDBAddCoverageMetric(&out,
					"raw_marker_local_validation_witnesses_"+reason+"_omitted",
					int64(omitted))
			}
		}
		out.Metadata["raw_marker_local_validation_witnesses"] =
			strings.Join(flattened, ";")
		traceDBAddCoverageMetric(&out,
			"raw_marker_local_validation_witnesses_emitted",
			int64(len(flattened)))
		total := 0
		for _, count := range localValidationWitnessTotals {
			total += count
		}
		if omitted := total - len(flattened); omitted > 0 {
			traceDBAddCoverageMetric(&out,
				"raw_marker_local_validation_witnesses_omitted", int64(omitted))
		}
		out.FieldSources["local_validation_witnesses"] =
			"bounded per exact closed first-failure reason class, then physical raw pair; diagnostic only and never alternate PID/name/span admission authority"
	}
	if err := traceDBApplyNullDurationRawClosureCensus(
		&out, syncSpans, candidates, rejectedClosedStarts, openBegins); err != nil {
		out.Error = err.Error()
		return out, err
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
		collisions, complete, err :=
			syncSpans.censusCandidateCollisions(ctx, candidate)
		if err != nil {
			out.Error = err.Error()
			return out, err
		}
		if !complete {
			out.Metadata["publication_state"] = "withheld_cross_source_index_incomplete"
			out.Skipped = "raw marker sync recovery withheld: bounded combined DB collision census incomplete"
			return out, nil
		}
		traceDBAddCoverageMetric(
			&out, "raw_collision_combined_census_requests", 1)
		if collisions.SemanticTotal > 0 {
			if collisions.LocallyAdmittedSemanticTotal > 0 {
				census := collisions.LocallyAdmittedInterval
				if censusErr := traceDBRecordRawMarkerCollisionCensus(
					&out, census, "exact_semantic"); censusErr != nil {
					out.Error = censusErr.Error()
					return out, censusErr
				}
				replaced, replaceErr := traceDBReplaceRawMarkerAuthoritativeCollision(
					ctx, &out, syncSpans, candidate, census, true,
					rawIntervalCount, "exact_semantic")
				if replaceErr != nil {
					out.Error = replaceErr.Error()
					return out, replaceErr
				}
				if replaced {
					continue
				}
				if census.Total == 0 {
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
		} else if collisions.IntervalTotal > 0 {
			if collisions.LocallyAdmittedInterval.Total > 0 {
				census := collisions.LocallyAdmittedInterval
				if censusErr := traceDBRecordRawMarkerCollisionCensus(
					&out, census, "name_drift"); censusErr != nil {
					out.Error = censusErr.Error()
					return out, censusErr
				}
				replaced, replaceErr := traceDBReplaceRawMarkerAuthoritativeCollision(
					ctx, &out, syncSpans, candidate, census, true,
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
		"local_segment_fences":           int(out.Metrics["raw_marker_local_fences"]),
		"open_begins":                    int(out.Metrics["raw_open_begins_withheld"]),
		"orphan_endpoints":               int(out.Metrics["raw_orphan_endpoints_withheld"]),
		"poisoned_emitter_lanes":         int(out.Metrics["raw_emitter_lanes_poisoned"]),
		"unrepresentable_interval_pairs": int(out.Metrics["raw_pairs_withheld_unrepresentable_interval"]),
	})
	return out, nil
}

func traceDBApplyNullDurationRawClosureCensus(
	out *TraceDBCoverage,
	syncSpans *traceDBSyncSpanAuthority,
	candidates []traceDBSyncSpanCandidate,
	rejectedClosedStarts map[traceDBRawMarkerPhysicalStartKey]traceDBRawMarkerRejectedClosedStart,
	openBegins map[traceDBRawMarkerPhysicalStartKey]int,
) error {
	if out == nil || syncSpans == nil {
		return &traceDBOutputInvariantError{
			Reason: "missing_null_duration_raw_closure_census_authority",
		}
	}
	hints, total, complete, err := syncSpans.nullDurationHintSnapshot()
	if err != nil {
		return err
	}
	traceDBAddCoverageMetric(out, "null_duration_fence_hints_total", int64(total))
	traceDBAddCoverageMetric(out, "null_duration_fence_hints_retained", int64(len(hints)))
	if omitted := total - len(hints); omitted > 0 {
		traceDBAddCoverageMetric(out, "null_duration_fence_hints_omitted", int64(omitted))
	}
	out.FieldSources["null_duration_raw_closure"] =
		"diagnostic-only correlation of exact NULL-duration DB start identity/name with independently decoded valid B/E candidates, locally rejected closed pairs and trailing open begins; no raw disposition is admitted as DB duration or fence authority"
	switch {
	case !complete:
		out.Metadata["null_duration_raw_closure_census"] = "incomplete_hint_cap"
		out.Metadata["null_duration_raw_disposition_census"] = "incomplete_hint_cap"
	case total == 0:
		out.Metadata["null_duration_raw_closure_census"] = "complete_no_exact_hint"
		out.Metadata["null_duration_raw_disposition_census"] = "complete_no_exact_hint"
	default:
		out.Metadata["null_duration_raw_closure_census"] = "complete"
		out.Metadata["null_duration_raw_disposition_census"] = "complete"
	}
	if len(hints) == 0 {
		return nil
	}

	exact := map[traceDBCallstackNullDurationClosureKey]int{}
	withoutMarker := map[traceDBCallstackNullDurationClosureKey]int{}
	withoutName := map[traceDBCallstackNullDurationClosureKey]int{}
	physicalStart := map[traceDBCallstackNullDurationClosureKey]int{}
	wantedPhysicalStarts := make(
		map[traceDBCallstackNullDurationClosureKey]struct{}, len(hints))
	for _, hint := range hints {
		wantedPhysicalStarts[traceDBCallstackNullDurationClosureKey{
			HeaderTID: hint.HeaderTID, HeaderTGID: hint.HeaderTGID,
			CanonicalITID: hint.CanonicalITID, OwnerIPID: hint.OwnerIPID,
			Start: hint.Start,
		}] = struct{}{}
	}
	for _, candidate := range candidates {
		key, ok := traceDBCallstackNullDurationClosureKeyFromCandidate(candidate)
		if !ok {
			continue
		}
		noMarker := key
		noMarker.MarkerPID = 0
		physical := noMarker
		physical.Name = ""
		if _, wanted := wantedPhysicalStarts[physical]; !wanted {
			continue
		}
		exact[key]++
		withoutMarker[noMarker]++
		noName := key
		noName.Name = ""
		withoutName[noName]++
		physicalStart[physical]++
	}
	for _, hint := range hints {
		key := traceDBCallstackNullDurationClosureKey{
			HeaderTID: hint.HeaderTID, HeaderTGID: hint.HeaderTGID,
			MarkerPID: hint.MarkerPID, CanonicalITID: hint.CanonicalITID,
			OwnerIPID: hint.OwnerIPID, Start: hint.Start, Name: hint.Name,
		}
		physicalKey := traceDBRawMarkerPhysicalStartKey{
			HeaderPID: hint.HeaderTID, MarkerPID: hint.MarkerPID,
			Start: uint64(hint.Start), Name: hint.Name,
		}
		rejectedCount := 0
		if rejected := rejectedClosedStarts[physicalKey]; len(rejected.CountByReason) > 0 {
			for reason, count := range rejected.CountByReason {
				rejectedCount += count
				traceDBAddCoverageMetric(out,
					"null_duration_hints_exact_raw_rejected_closed_pair_"+
						traceDBRawDecodeReasonMetric(reason), int64(count))
			}
			switch rejectedCount {
			case 1:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_unique_exact_raw_rejected_closed_pair", 1)
			default:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_ambiguous_exact_raw_rejected_closed_pair", 1)
			}
		}
		openCount := openBegins[physicalKey]
		switch {
		case openCount == 1:
			traceDBAddCoverageMetric(out,
				"null_duration_hints_unique_exact_raw_open_begin", 1)
		case openCount > 1:
			traceDBAddCoverageMetric(out,
				"null_duration_hints_ambiguous_exact_raw_open_begin", 1)
		}
		validCount := exact[key]
		dispositionKinds := 0
		if validCount > 0 {
			dispositionKinds++
		}
		if rejectedCount > 0 {
			dispositionKinds++
		}
		if openCount > 0 {
			dispositionKinds++
		}
		switch dispositionKinds {
		case 0:
			traceDBAddCoverageMetric(out,
				"null_duration_hints_no_exact_raw_start_disposition", 1)
		case 1:
			switch {
			case validCount > 0:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_disposition_valid_closure", 1)
			case rejectedCount > 0:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_disposition_rejected_closed_pair", 1)
			default:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_disposition_open_begin", 1)
			}
		default:
			traceDBAddCoverageMetric(out,
				"null_duration_hints_conflicting_exact_raw_disposition_kinds", 1)
		}
		traceDBAddCoverageMetric(out,
			"null_duration_hints_exact_raw_disposition_accounted", 1)
		switch {
		case validCount == 1:
			traceDBAddCoverageMetric(out,
				"null_duration_hints_unique_exact_raw_closure", 1)
		case validCount > 1:
			traceDBAddCoverageMetric(out,
				"null_duration_hints_ambiguous_exact_raw_closure", 1)
		default:
			noMarker := key
			noMarker.MarkerPID = 0
			noName := key
			noName.Name = ""
			physical := noMarker
			physical.Name = ""
			switch {
			case withoutMarker[noMarker] > 0:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_raw_payload_pid_mismatch_or_ambiguous", 1)
			case withoutName[noName] > 0:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_raw_name_mismatch_or_ambiguous", 1)
			case physicalStart[physical] > 0:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_raw_marker_and_name_mismatch", 1)
			default:
				traceDBAddCoverageMetric(out,
					"null_duration_hints_without_valid_raw_closure", 1)
			}
		}
	}
	return nil
}

func traceDBRawMarkerPhysicalStartKeyFromRecord(
	record traceDBRawMarkerRecord,
) traceDBRawMarkerPhysicalStartKey {
	return traceDBRawMarkerPhysicalStartKey{
		HeaderPID: record.HeaderPID, MarkerPID: record.PayloadPID,
		Start: record.TimestampNS, Name: record.Name,
	}
}

func traceDBCallstackNullDurationClosureKeyFromCandidate(
	candidate traceDBSyncSpanCandidate,
) (traceDBCallstackNullDurationClosureKey, bool) {
	if candidate.Producer != traceDBSyncSpanProducerSourceRawMarker ||
		candidate.HeaderTID <= 0 || candidate.HeaderTGID <= 0 ||
		!candidate.MarkerPIDKnown || candidate.MarkerPID <= 0 ||
		!candidate.CanonicalITIDKnown || candidate.CanonicalITID <= 0 ||
		!candidate.OwnerIPIDKnown || candidate.OwnerIPID <= 0 ||
		candidate.Start < 0 || candidate.End <= candidate.Start ||
		!traceDBCallstackSpanName(candidate.Name) {
		return traceDBCallstackNullDurationClosureKey{}, false
	}
	return traceDBCallstackNullDurationClosureKey{
		HeaderTID: candidate.HeaderTID, HeaderTGID: candidate.HeaderTGID,
		MarkerPID: candidate.MarkerPID, CanonicalITID: candidate.CanonicalITID,
		OwnerIPID: candidate.OwnerIPID, Start: candidate.Start, Name: candidate.Name,
	}, true
}

func traceDBRawMarkerLocalValidationWitness(
	pair traceDBRawMarkerPair,
	reason string,
) string {
	return fmt.Sprintf(
		"emitter=%d/start_ns=%d/end_ns=%d/reason=%s/begin_payload_pid=%d/end_payload_pid=%d/name=%s",
		pair.begin.HeaderPID, pair.begin.TimestampNS, pair.end.TimestampNS,
		traceDBRawDecodeReasonMetric(reason), pair.begin.PayloadPID,
		pair.end.PayloadPID, traceDBRawMarkerNameWitness(pair.begin.Name))
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
	out *TraceDBCoverage,
	census traceDBSyncSpanIntervalCollisionCensus,
	shape string,
) error {
	if out == nil ||
		shape != "exact_semantic" && shape != "name_drift" {
		return &traceDBOutputInvariantError{
			Reason: "invalid_raw_marker_collision_census_request",
		}
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
	return nil
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
	officialPrintParserProfile :=
		begin.OpenHarmonyPrintParserProfile &&
			end.OpenHarmonyPrintParserProfile
	zeroPIDHeaderIdentity :=
		begin.PayloadPID == 0 && end.PayloadPID == 0 &&
			begin.ZeroPIDUsesHeaderIdentity && end.ZeroPIDUsesHeaderIdentity
	candidateName := begin.Name
	trailingSpaceNormalized := false
	if officialPrintParserProfile {
		trimmed := strings.TrimRight(candidateName, " ")
		if trimmed != candidateName && traceDBCallstackSpanName(trimmed) {
			candidateName = trimmed
			trailingSpaceNormalized = true
		}
	}
	switch {
	case begin.HeaderPID <= 0:
		return traceDBSyncSpanCandidate{}, "invalid_begin_header_pid"
	case end.HeaderPID <= 0:
		return traceDBSyncSpanCandidate{}, "invalid_end_header_pid"
	case begin.HeaderPID != end.HeaderPID:
		return traceDBSyncSpanCandidate{}, "header_pid_mismatch"
	case begin.PayloadPID < 0 || begin.PayloadPID > math.MaxInt32:
		return traceDBSyncSpanCandidate{}, "invalid_begin_payload_pid"
	case end.PayloadPID < 0 || end.PayloadPID > math.MaxInt32:
		return traceDBSyncSpanCandidate{}, "invalid_end_payload_pid"
	case begin.PayloadPID == 0 && end.PayloadPID != 0:
		return traceDBSyncSpanCandidate{}, "invalid_begin_payload_pid"
	case end.PayloadPID == 0 && begin.PayloadPID != 0:
		return traceDBSyncSpanCandidate{}, "invalid_end_payload_pid"
	case begin.PayloadPID == 0 && !zeroPIDHeaderIdentity:
		return traceDBSyncSpanCandidate{}, "zero_payload_pid_without_official_header_identity"
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
	case begin.PayloadPID != end.PayloadPID:
		return traceDBSyncSpanCandidate{}, "payload_pid_mismatch"
	case !traceDBCallstackSpanName(candidateName):
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
	markerPID, markerPIDKnown := begin.PayloadPID, true
	startMarkerBody, endMarkerBody := begin.Buffer, end.Buffer
	if trailingSpaceNormalized {
		var ok bool
		startMarkerBody, ok = traceDBRawMarkerNormalizeBeginName(
			startMarkerBody, beginVerdict, candidateName)
		if !ok {
			return traceDBSyncSpanCandidate{}, "begin_name_normalization_failed"
		}
	}
	if zeroPIDHeaderIdentity {
		markerPID, markerPIDKnown = 0, false
		var ok bool
		startMarkerBody, ok = traceDBRawMarkerNormalizeZeroPIDBody(
			startMarkerBody, "B", beginProcess.PID)
		if !ok {
			return traceDBSyncSpanCandidate{}, "zero_begin_payload_normalization_failed"
		}
		endMarkerBody, ok = traceDBRawMarkerNormalizeZeroPIDBody(
			endMarkerBody, "E", beginProcess.PID)
		if !ok {
			return traceDBSyncSpanCandidate{}, "zero_end_payload_normalization_failed"
		}
	}
	candidate := traceDBSyncSpanCandidate{
		Producer:           traceDBSyncSpanProducerSourceRawMarker,
		StableKind:         traceDBSyncSpanStableSourceRawOrdinal,
		StableID:           begin.PhysicalOrdinal,
		HeaderTID:          beginThread.TID,
		HeaderTGID:         beginProcess.PID,
		MarkerPID:          markerPID,
		MarkerPIDKnown:     markerPIDKnown,
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
		StartMarkerBody:    startMarkerBody,
		EndMarkerBody:      endMarkerBody,
		CPUPlacement:       traceDBSyncSpanCPUPlacementKnown,
		StartCPUProvenance: traceDBSyncSpanCPUSourceRawPage,
		EndCPUProvenance:   traceDBSyncSpanCPUSourceRawPage,
		Task:               traceDBCommName(beginThread.Name, "unknown"),
		Name:               candidateName,
		NameProvenance:     traceDBSyncSpanNameSourceRawMarker,
	}
	if err := validateTraceDBSyncSpanCandidate(candidate); err != nil {
		return traceDBSyncSpanCandidate{}, "candidate_validation_failed"
	}
	return candidate, ""
}

// traceDBRawMarkerNormalizeBeginName rewrites only the exact name field that
// the shared complete-payload parser already admitted. In particular, an
// official Harmony metadata suffix such as "|D0001" is producer data after
// the name and must survive byte-for-byte; trimming the far right of the body
// cannot normalize a name before that suffix. Re-decoding the reconstructed
// payload proves that no action, PID, track, value, or suffix boundary gained
// authority from this display-name repair.
func traceDBRawMarkerNormalizeBeginName(
	body string,
	admitted tracequery.TraceMarkPayloadVerdict,
	normalizedName string,
) (string, bool) {
	if !admitted.Admitted || admitted.Action != "B" ||
		admitted.SpanPID < 0 || admitted.SpanPID > math.MaxInt32 ||
		admitted.Name == normalizedName ||
		strings.TrimRight(admitted.Name, " ") != normalizedName {
		return "", false
	}
	prefix := "B|" + strconv.Itoa(admitted.SpanPID) + "|"
	if !strings.HasPrefix(body, prefix) {
		return "", false
	}
	tail := strings.TrimPrefix(body, prefix)
	if !strings.HasPrefix(tail, admitted.Name) {
		return "", false
	}
	suffix := strings.TrimPrefix(tail, admitted.Name)
	if suffix != "" && !strings.HasPrefix(suffix, "|") {
		return "", false
	}
	normalized := prefix + normalizedName + suffix
	verdict := tracequery.DecodeTraceMarkEndpointPayload(normalized)
	if !verdict.Admitted || verdict.Action != admitted.Action ||
		verdict.SpanPID != admitted.SpanPID ||
		verdict.Name != normalizedName ||
		verdict.Track != admitted.Track ||
		verdict.Value != admitted.Value {
		return "", false
	}
	return normalized, true
}

func traceDBRawMarkerNormalizeZeroPIDBody(body, action string, markerPID int64) (string, bool) {
	if markerPID <= 0 || markerPID > math.MaxInt32 ||
		(action != "B" && action != "E") {
		return "", false
	}
	prefix := action + "|0|"
	if !strings.HasPrefix(body, prefix) {
		return "", false
	}
	return action + "|" + strconv.FormatInt(markerPID, 10) + "|" +
		strings.TrimPrefix(body, prefix), true
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
