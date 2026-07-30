package hitraceconv

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const traceDBRawSchedulerCPULookupWitnessCap = 4

// traceDBRawSchedulerCPUFallback is an independent CPU point authority built
// only from a complete immutable sched_switch_lite family. It never repairs or
// relaxes the DB Running lanes: consumers may use it only when their DB
// witness is unavailable/source-tainted, and a DB/raw disagreement fails
// closed in traceDBSchedulerRunningIndex.lookupCPUAt.
type traceDBRawSchedulerCPUFallback struct {
	intervals map[int64][]traceDBRunningInterval
	coverage  TraceDBCoverage
	audit     *traceDBRawSchedulerCPULookupAudit
	enabled   bool
}

type traceDBRawSchedulerCPUConsumer uint8

const (
	traceDBRawSchedulerCPUConsumerUnspecified traceDBRawSchedulerCPUConsumer = iota
	traceDBRawSchedulerCPUConsumerCallstack
	traceDBRawSchedulerCPUConsumerFrame
	traceDBRawSchedulerCPUConsumerCount
)

type traceDBRawSchedulerCPURawLookupStatus uint8

const (
	traceDBRawSchedulerCPURawNotConsulted traceDBRawSchedulerCPURawLookupStatus = iota
	traceDBRawSchedulerCPURawKnown
	traceDBRawSchedulerCPURawKnownAgree
	traceDBRawSchedulerCPURawKnownConflict
	traceDBRawSchedulerCPURawMissLaneAbsent
	traceDBRawSchedulerCPURawMissBeforeFirst
	traceDBRawSchedulerCPURawMissAfterLast
	traceDBRawSchedulerCPURawMissGap
	traceDBRawSchedulerCPURawMissOverlapConflict
	traceDBRawSchedulerCPURawMissInvalidCPU
	traceDBRawSchedulerCPURawLookupStatusCount
)

type traceDBRawSchedulerCPULookupWitness struct {
	ITID      int64
	TID       int64
	Timestamp int64
	DBCPU     int64
	RawCPU    int64
}

type traceDBRawSchedulerCPULookupAudit struct {
	counts    [traceDBRawSchedulerCPUConsumerCount][traceDBSchedulerRunningLifecycleRejected + 1][traceDBRawSchedulerCPURawLookupStatusCount]int64
	witnesses [traceDBRawSchedulerCPUConsumerCount][traceDBSchedulerRunningLifecycleRejected + 1][traceDBRawSchedulerCPURawLookupStatusCount][]traceDBRawSchedulerCPULookupWitness
	publicTID map[int64]int64
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
		audit: &traceDBRawSchedulerCPULookupAudit{
			publicTID: map[int64]int64{},
		},
	}
	for itid, thread := range authority.identities.ByITID {
		if thread.ITID == itid && thread.TID >= 0 && thread.TID <= math.MaxInt32 {
			out.audit.publicTID[itid] = thread.TID
		}
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
		ordered := true
		for index := 1; index < len(rows); index++ {
			if rows[index].raw.TimestampNS < rows[index-1].raw.TimestampNS {
				ordered = false
				break
			}
		}
		if ordered {
			traceDBAddCoverageMetric(
				&out.coverage, "raw_cpu_lanes_already_time_ordered", 1)
		} else {
			sort.SliceStable(rows, func(i, j int) bool {
				return rows[i].raw.TimestampNS < rows[j].raw.TimestampNS
			})
			traceDBAddCoverageMetric(
				&out.coverage, "raw_cpu_lanes_stably_reordered", 1)
		}
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
		first := rows[0]
		switch {
		case !authority.identities.TraceStartKnown:
			traceDBAddCoverageMetric(
				&out.coverage, "capture_head_audit_trace_start_unavailable", 1)
		case !first.valid:
			traceDBAddCoverageMetric(
				&out.coverage, "capture_head_audit_invalid_first_boundary", 1)
		case authority.identities.TraceStart > int64(first.raw.TimestampNS):
			traceDBAddCoverageMetric(
				&out.coverage, "capture_head_audit_invalid_order", 1)
		case authority.identities.TraceStart == int64(first.raw.TimestampNS):
			traceDBAddCoverageMetric(
				&out.coverage, "capture_head_audit_empty_at_first_boundary", 1)
		default:
			traceDBAddCoverageMetric(
				&out.coverage, "capture_head_candidates_global_start_only", 1)
			traceDBAddCoverageMetric(
				&out.coverage,
				"capture_head_withheld_per_cpu_retention_start_unavailable", 1)
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
			"source":       "complete immutable sched_switch_lite physical-event family; zero body rejects and retained/admitted/record census equality required",
			"interval":     "half-open [current switch,next switch) on one exact CPU; current.next_tid must equal next.prev_tid and both timestamp/CPU coordinates must be unique",
			"capture_head": "withdrawn numeric authority: trace_range.start_ts is only a global DB bound and does not prove the per-CPU raw ring-buffer retention start; first-switch prev_tid head candidates are counted but always withheld until an exact per-CPU retention-start authority exists",
			"identity":     "current next public TID resolves at interval start to exactly one canonical DB thread/process generation; public TID is never rewritten or guessed under PID namespaces",
			"lifecycle":    "the complete half-open interval passes the shared scheduler thread/process generation authority; an invalid boundary fences both adjacent intervals",
			"precedence":   "DB Running CPU remains primary; raw is used only for DB unknown/source-tainted points, DB/raw disagreement fails closed, and lifecycle-rejected DB lanes can never be bypassed",
			"consumer":     "callstack and frame CPU placement only in this version; scheduler publication, perf, raw-ftrace, syscall, native-hook and task-pool semantics are unchanged",
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
	index.rawFallbackAudit = fallback.audit
	return index
}

func (index traceDBSchedulerRunningIndex) withRawSchedulerCPUConsumer(
	consumer traceDBRawSchedulerCPUConsumer,
) traceDBSchedulerRunningIndex {
	index.rawFallbackConsumer = consumer
	return index
}

func (audit *traceDBRawSchedulerCPULookupAudit) record(
	consumer traceDBRawSchedulerCPUConsumer,
	dbStatus traceDBSchedulerRunningLookupStatus,
	rawStatus traceDBRawSchedulerCPURawLookupStatus,
	itid, timestamp, dbCPU, rawCPU int64,
) {
	if audit == nil ||
		consumer >= traceDBRawSchedulerCPUConsumerCount ||
		dbStatus > traceDBSchedulerRunningLifecycleRejected ||
		rawStatus >= traceDBRawSchedulerCPURawLookupStatusCount {
		return
	}
	audit.counts[consumer][dbStatus][rawStatus]++
	slot := &audit.witnesses[consumer][dbStatus][rawStatus]
	if len(*slot) >= traceDBRawSchedulerCPULookupWitnessCap {
		return
	}
	*slot = append(*slot, traceDBRawSchedulerCPULookupWitness{
		ITID: itid, TID: audit.publicTID[itid], Timestamp: timestamp,
		DBCPU: dbCPU, RawCPU: rawCPU,
	})
}

func (fallback traceDBRawSchedulerCPUFallback) finalCoverage() TraceDBCoverage {
	out := fallback.coverage
	out.Metrics = cloneTraceDBInt64Map(out.Metrics)
	out.Metadata = cloneTraceDBStringMap(out.Metadata)
	if fallback.audit == nil {
		return out
	}
	var total int64
	for consumer := traceDBRawSchedulerCPUConsumer(0); consumer < traceDBRawSchedulerCPUConsumerCount; consumer++ {
		for dbStatus := traceDBSchedulerRunningLookupStatus(0); dbStatus <= traceDBSchedulerRunningLifecycleRejected; dbStatus++ {
			for rawStatus := traceDBRawSchedulerCPURawLookupStatus(0); rawStatus < traceDBRawSchedulerCPURawLookupStatusCount; rawStatus++ {
				count := fallback.audit.counts[consumer][dbStatus][rawStatus]
				if count == 0 {
					continue
				}
				total += count
				key := traceDBRawSchedulerCPULookupMetric(
					consumer, dbStatus, rawStatus)
				traceDBAddCoverageMetric(&out, key, count)
				witnesses :=
					fallback.audit.witnesses[consumer][dbStatus][rawStatus]
				if len(witnesses) == 0 {
					continue
				}
				parts := make([]string, 0, len(witnesses))
				for _, witness := range witnesses {
					part := fmt.Sprintf(
						"itid=%d/tid=%d/ts_ns=%d",
						witness.ITID, witness.TID, witness.Timestamp)
					if dbStatus == traceDBSchedulerRunningKnown {
						part += fmt.Sprintf("/db_cpu=%d", witness.DBCPU)
					}
					switch rawStatus {
					case traceDBRawSchedulerCPURawKnown,
						traceDBRawSchedulerCPURawKnownAgree,
						traceDBRawSchedulerCPURawKnownConflict:
						part += fmt.Sprintf("/raw_cpu=%d", witness.RawCPU)
					}
					parts = append(parts, part)
				}
				out.Metadata[key+"_witnesses"] = strings.Join(parts, ";")
				traceDBAddCoverageMetric(
					&out, key+"_witnesses_emitted", int64(len(witnesses)))
				if omitted := count - int64(len(witnesses)); omitted > 0 {
					traceDBAddCoverageMetric(
						&out, key+"_witnesses_omitted", omitted)
				}
			}
		}
	}
	traceDBAddCoverageMetric(&out, "lookup_calls_total", total)
	if total > 0 {
		out.Metadata["lookup_census_state"] =
			"complete_after_callstack_and_frame_consumers"
		out.FieldSources = cloneTraceDBStringMap(out.FieldSources)
		out.FieldSources["lookup_census"] =
			"exact authorized callstack/frame CPU point lookups over the shared DB-primary/raw-fallback index; counts are lookup attempts, not unique spans"
		out.FieldSources["lookup_witnesses"] =
			"bounded exact canonical itid/public tid/timestamp and known CPU values; diagnostic only and never alternate CPU, identity or lifecycle authority"
	}
	return out
}

func traceDBRawSchedulerCPULookupMetric(
	consumer traceDBRawSchedulerCPUConsumer,
	dbStatus traceDBSchedulerRunningLookupStatus,
	rawStatus traceDBRawSchedulerCPURawLookupStatus,
) string {
	return "lookup_" + traceDBRawSchedulerCPUConsumerName(consumer) +
		"_db_" + traceDBSchedulerRunningLookupStatusName(dbStatus) +
		"_raw_" + traceDBRawSchedulerCPURawLookupStatusName(rawStatus)
}

func traceDBRawSchedulerCPUConsumerName(
	consumer traceDBRawSchedulerCPUConsumer,
) string {
	switch consumer {
	case traceDBRawSchedulerCPUConsumerCallstack:
		return "callstack"
	case traceDBRawSchedulerCPUConsumerFrame:
		return "frame"
	default:
		return "unspecified"
	}
}

func traceDBSchedulerRunningLookupStatusName(
	status traceDBSchedulerRunningLookupStatus,
) string {
	switch status {
	case traceDBSchedulerRunningKnown:
		return "known"
	case traceDBSchedulerRunningSourceTainted:
		return "source_tainted"
	case traceDBSchedulerRunningLifecycleRejected:
		return "lifecycle_rejected"
	default:
		return "unknown"
	}
}

func traceDBRawSchedulerCPURawLookupStatusName(
	status traceDBRawSchedulerCPURawLookupStatus,
) string {
	switch status {
	case traceDBRawSchedulerCPURawKnown:
		return "known"
	case traceDBRawSchedulerCPURawKnownAgree:
		return "known_agree"
	case traceDBRawSchedulerCPURawKnownConflict:
		return "known_conflict"
	case traceDBRawSchedulerCPURawMissLaneAbsent:
		return "miss_lane_absent"
	case traceDBRawSchedulerCPURawMissBeforeFirst:
		return "miss_before_first_interval"
	case traceDBRawSchedulerCPURawMissAfterLast:
		return "miss_after_last_interval"
	case traceDBRawSchedulerCPURawMissGap:
		return "miss_between_intervals"
	case traceDBRawSchedulerCPURawMissOverlapConflict:
		return "miss_overlap_cpu_conflict"
	case traceDBRawSchedulerCPURawMissInvalidCPU:
		return "miss_invalid_cpu"
	default:
		return "not_consulted"
	}
}

func traceDBRawSchedulerCPUAt(
	intervals map[int64][]traceDBRunningInterval,
	itid, timestamp int64,
) (int64, traceDBRawSchedulerCPURawLookupStatus) {
	entries := intervals[itid]
	if len(entries) == 0 {
		return 0, traceDBRawSchedulerCPURawMissLaneAbsent
	}
	index := sort.Search(len(entries), func(i int) bool {
		return entries[i].Start > timestamp
	})
	if index == 0 {
		return 0, traceDBRawSchedulerCPURawMissBeforeFirst
	}
	var cpu int64
	found := false
	for cursor := index - 1; cursor >= 0; cursor-- {
		entry := entries[cursor]
		if entry.Start <= timestamp && timestamp < entry.End {
			if !validTraceDBCPUIndex(entry.CPU) {
				return 0, traceDBRawSchedulerCPURawMissInvalidCPU
			}
			if found && cpu != entry.CPU {
				return 0, traceDBRawSchedulerCPURawMissOverlapConflict
			}
			cpu = entry.CPU
			found = true
		}
		if cursor == 0 || entries[cursor-1].PrefixMaxEnd <= timestamp {
			break
		}
	}
	if found {
		return cpu, traceDBRawSchedulerCPURawKnown
	}
	if timestamp >= entries[len(entries)-1].End {
		return 0, traceDBRawSchedulerCPURawMissAfterLast
	}
	return 0, traceDBRawSchedulerCPURawMissGap
}

func cloneTraceDBInt64Map(source map[string]int64) map[string]int64 {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]int64, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func cloneTraceDBStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
