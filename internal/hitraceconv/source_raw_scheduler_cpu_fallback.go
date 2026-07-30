package hitraceconv

import (
	"fmt"
	"math"
	"sort"
)

// traceDBRawSchedulerCPUFallback is an independent CPU point authority built
// only from a complete immutable sched_switch_lite family. It never repairs or
// relaxes the DB Running lanes: consumers may use it only when their DB
// witness is unavailable/source-tainted, and a DB/raw disagreement fails
// closed in traceDBSchedulerRunningIndex.lookupCPUAt.
type traceDBRawSchedulerCPUFallback struct {
	intervals map[int64][]traceDBRunningInterval
	coverage  TraceDBCoverage
	enabled   bool
}

type traceDBRawSchedulerCPURecord struct {
	raw   traceDBRawSchedSwitchLiteRecord
	valid bool
}

func newTraceDBRawSchedulerCPUFallback(
	inventory *traceDBSourceNameInventory,
	authority traceDBSchedulerAuthority,
) traceDBRawSchedulerCPUFallback {
	out := traceDBRawSchedulerCPUFallback{
		intervals: map[int64][]traceDBRunningInterval{},
		coverage:  newTraceDBRawSchedulerCPUFallbackCoverage(),
	}
	if inventory == nil || inventory.RawDecode.Role == "" {
		out.coverage.Metadata["authority_state"] = "unavailable_source_raw_decode_ledger"
		out.coverage.Skipped = "raw scheduler CPU fallback unavailable: source raw decode ledger absent"
		return out
	}
	out.coverage.Found = len(inventory.RawSwitchLite) > 0
	out.coverage.RowsRead = len(inventory.RawSwitchLite)
	traceDBAttachRawSchedulerLiteDiagnostics(
		&out.coverage, inventory.RawDecode, "sched_switch_lite")
	traceDBAddCoverageMetric(
		&out.coverage, "raw_records_retained", int64(len(inventory.RawSwitchLite)))
	if !authority.initialized || !authority.complete {
		out.coverage.Metadata["authority_state"] = "withheld_lifecycle_authority_incomplete"
		out.coverage.Skipped = "raw scheduler CPU fallback withheld: scheduler lifecycle authority incomplete"
		return out
	}
	if !traceDBRawDecodeFamilyComplete(
		inventory.RawDecode, traceDBRawRetentionSwitchLite) {
		out.coverage.Metadata["authority_state"] = "withheld_source_raw_decode_incomplete"
		out.coverage.Skipped = "raw scheduler CPU fallback withheld: source raw decode ledger incomplete"
		return out
	}
	records := inventory.RawDecode.Metrics["target_sched_switch_lite_records"]
	admitted := inventory.RawDecode.Metrics["target_sched_switch_lite_body_admitted"]
	rejected := inventory.RawDecode.Metrics["target_sched_switch_lite_body_rejected"]
	traceDBAddCoverageMetric(&out.coverage, "raw_records_censused", records)
	traceDBAddCoverageMetric(&out.coverage, "raw_records_body_admitted", admitted)
	traceDBAddCoverageMetric(&out.coverage, "raw_records_body_rejected", rejected)
	if records != admitted || admitted != int64(len(inventory.RawSwitchLite)) ||
		rejected != 0 ||
		inventory.RawDecode.Metrics["target_sched_switch_lite_record_capture_failed"] != 0 {
		out.coverage.Metadata["authority_state"] = "withheld_incomplete_physical_event_family"
		out.coverage.Skipped = fmt.Sprintf(
			"raw scheduler CPU fallback withheld: records=%d admitted=%d retained=%d rejected=%d capture_failed=%d",
			records, admitted, len(inventory.RawSwitchLite), rejected,
			inventory.RawDecode.Metrics["target_sched_switch_lite_record_capture_failed"])
		return out
	}
	if len(inventory.RawSwitchLite) == 0 {
		out.enabled = true
		out.coverage.Metadata["authority_state"] = "complete_no_source_records"
		out.coverage.Skipped = "raw scheduler CPU fallback complete: no source sched_switch_lite records"
		return out
	}

	byCPU := map[int64][]traceDBRawSchedulerCPURecord{}
	for _, raw := range inventory.RawSwitchLite {
		key, reason := traceDBRawSchedSwitchLiteKeyDecision(raw)
		if reason != "" {
			traceDBAddCoverageMetric(&out.coverage, "raw_records_key_rejected", 1)
			traceDBAddCoverageMetric(
				&out.coverage, "raw_records_key_rejected_"+reason, 1)
			if raw.TimestampNS <= math.MaxInt64 &&
				validTraceDBCPUIndex(int64(raw.CPU)) {
				byCPU[int64(raw.CPU)] = append(
					byCPU[int64(raw.CPU)],
					traceDBRawSchedulerCPURecord{raw: raw})
			}
			continue
		}
		byCPU[key.CPU] = append(byCPU[key.CPU],
			traceDBRawSchedulerCPURecord{raw: raw, valid: true})
	}

	for cpu, rows := range byCPU {
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].raw.TimestampNS < rows[j].raw.TimestampNS
		})
		for index := 0; index < len(rows); {
			end := index + 1
			for end < len(rows) &&
				rows[end].raw.TimestampNS == rows[index].raw.TimestampNS {
				end++
			}
			if end-index > 1 {
				traceDBAddCoverageMetric(
					&out.coverage, "raw_duplicate_coordinate_cohorts", 1)
				traceDBAddCoverageMetric(
					&out.coverage, "raw_duplicate_coordinate_records", int64(end-index))
				for cursor := index; cursor < end; cursor++ {
					rows[cursor].valid = false
				}
			}
			index = end
		}
		for index := 0; index+1 < len(rows); index++ {
			current, next := rows[index], rows[index+1]
			if !current.valid || !next.valid {
				traceDBAddCoverageMetric(
					&out.coverage, "candidate_intervals_withheld_invalid_boundary", 1)
				continue
			}
			start, end := current.raw.TimestampNS, next.raw.TimestampNS
			if start >= end || end > math.MaxInt64 {
				traceDBAddCoverageMetric(
					&out.coverage, "candidate_intervals_withheld_invalid_order", 1)
				continue
			}
			if current.raw.NextTID != next.raw.PrevTID {
				traceDBAddCoverageMetric(
					&out.coverage, "candidate_intervals_withheld_tid_discontinuity", 1)
				continue
			}
			running, reason := traceDBResolveRawSchedSwitchLiteSubject(
				authority, current.raw.NextTID, int64(start))
			if reason != "" {
				traceDBAddCoverageMetric(
					&out.coverage, "candidate_intervals_withheld_start_"+
						traceDBRawDecodeReasonMetric(reason), 1)
				continue
			}
			subject, ok := authority.schedulerSubjectFromExactITID(
				running.ITID, true)
			if !ok || !authority.schedulerSourceIntervalAllows(
				subject, int64(start), int64(end)) {
				traceDBAddCoverageMetric(
					&out.coverage, "candidate_intervals_withheld_lifecycle", 1)
				continue
			}
			out.intervals[running.ITID] = append(
				out.intervals[running.ITID],
				traceDBRunningInterval{
					Start: int64(start), End: int64(end), CPU: cpu,
				})
			out.coverage.RowsEmitted++
		}
	}
	for itid, intervals := range out.intervals {
		sort.SliceStable(intervals, func(i, j int) bool {
			if intervals[i].Start != intervals[j].Start {
				return intervals[i].Start < intervals[j].Start
			}
			if intervals[i].End != intervals[j].End {
				return intervals[i].End < intervals[j].End
			}
			return intervals[i].CPU < intervals[j].CPU
		})
		var prefixMax int64
		for index := range intervals {
			if index == 0 || intervals[index].End > prefixMax {
				prefixMax = intervals[index].End
			}
			intervals[index].PrefixMaxEnd = prefixMax
		}
		out.intervals[itid] = intervals
	}
	traceDBAddCoverageMetric(
		&out.coverage, "canonical_itid_lanes", int64(len(out.intervals)))
	out.enabled = true
	out.coverage.Metadata["authority_state"] =
		"complete_unique_exact_half_open_intervals"
	out.coverage.Metadata["consumer_scope"] = "callstack_and_frame_fallback_only"
	out.coverage.Metadata["publication_contract"] = fmt.Sprintf(
		"intervals=%d; db_known_must_agree; lifecycle_rejected_never_bypassed",
		out.coverage.RowsEmitted)
	return out
}

func newTraceDBRawSchedulerCPUFallbackCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_scheduler_cpu",
		Table:  "__raw_sched_switch_running_intervals__",
		Role:   "query_ready_cpu_fallback_authority",
		FieldSources: map[string]string{
			"source":     "complete immutable sched_switch_lite physical-event family; zero body rejects and retained/admitted/record census equality required",
			"interval":   "half-open [current switch,next switch) on one exact CPU; current.next_tid must equal next.prev_tid and both timestamp/CPU coordinates must be unique",
			"identity":   "current next public TID resolves at interval start to exactly one canonical DB thread/process generation; public TID is never rewritten or guessed under PID namespaces",
			"lifecycle":  "the complete half-open interval passes the shared scheduler thread/process generation authority; an invalid boundary fences both adjacent intervals",
			"precedence": "DB Running CPU remains primary; raw is used only for DB unknown/source-tainted points, DB/raw disagreement fails closed, and lifecycle-rejected DB lanes can never be bypassed",
			"consumer":   "callstack and frame CPU placement only in this version; scheduler publication, perf, raw-ftrace, syscall, native-hook and task-pool semantics are unchanged",
		},
		Metadata: map[string]string{
			"authority_state": "unavailable",
		},
	}
}

func (index traceDBSchedulerRunningIndex) withRawSchedulerCPUFallback(
	fallback traceDBRawSchedulerCPUFallback,
) traceDBSchedulerRunningIndex {
	if !fallback.enabled {
		return index
	}
	index.rawFallbackIntervals = fallback.intervals
	index.rawFallbackEnabled = true
	return index
}
