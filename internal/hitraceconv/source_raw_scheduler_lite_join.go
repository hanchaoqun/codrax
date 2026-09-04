package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
)

type traceDBRawSchedSwitchLiteJoinKey struct {
	TimestampNS  int64
	CPU          int64
	PrevTID      int64
	PrevState    string
	NextTID      int64
	NextPriority int64
}

type traceDBRawSchedSwitchLiteCoordinate struct {
	TimestampNS int64
	CPU         int64
}

type traceDBRawSchedSwitchLiteJoin struct {
	coverage              TraceDBCoverage
	rawByKey              map[traceDBRawSchedSwitchLiteJoinKey][]traceDBRawSchedSwitchLiteRecord
	rawByCoordinate       map[traceDBRawSchedSwitchLiteCoordinate][]traceDBRawSchedSwitchLiteRecord
	dbByKey               map[traceDBRawSchedSwitchLiteJoinKey]int
	dbEmittedByCoordinate map[traceDBRawSchedSwitchLiteCoordinate]int
	eligible              map[traceDBRawSchedSwitchLiteJoinKey]traceDBRawSchedSwitchLiteRecord
	consumed              map[traceDBRawSchedSwitchLiteJoinKey]bool
	ready                 bool
	dbReady               bool
}

func newTraceDBRawSchedSwitchLiteJoin(
	inventory *traceDBSourceNameInventory,
) *traceDBRawSchedSwitchLiteJoin {
	join := &traceDBRawSchedSwitchLiteJoin{
		coverage:              newTraceDBRawSchedSwitchLiteJoinCoverage(),
		rawByKey:              map[traceDBRawSchedSwitchLiteJoinKey][]traceDBRawSchedSwitchLiteRecord{},
		rawByCoordinate:       map[traceDBRawSchedSwitchLiteCoordinate][]traceDBRawSchedSwitchLiteRecord{},
		dbByKey:               map[traceDBRawSchedSwitchLiteJoinKey]int{},
		dbEmittedByCoordinate: map[traceDBRawSchedSwitchLiteCoordinate]int{},
		eligible:              map[traceDBRawSchedSwitchLiteJoinKey]traceDBRawSchedSwitchLiteRecord{},
		consumed:              map[traceDBRawSchedSwitchLiteJoinKey]bool{},
	}
	if inventory != nil {
		traceDBAttachRawSchedulerLiteDiagnostics(
			&join.coverage, inventory.RawDecode, "sched_switch_lite")
		join.coverage.Found = len(inventory.RawSwitchLite) > 0
		join.coverage.RowsRead = len(inventory.RawSwitchLite)
		traceDBAddCoverageMetric(&join.coverage, "raw_records_retained", int64(len(inventory.RawSwitchLite)))
	}
	// The class gate (source_raw_lane_gate.go) splits absent/non-official
	// source (not applicable) from an official census that did not close
	// (census incomplete); an unrecognized ledger shape fails loud on the
	// coverage and the join stays not ready.
	if stop, _ := traceDBApplySourceRawLaneGateKeyed(&join.coverage, inventory,
		traceDBSourceRawLaneStateKeyJoin, "scheduler-lite switch join"); stop {
		return join
	}
	// Past the gate the census closed; the family predicate can only fail on
	// the family's own retention store having been withdrawn by byte budget.
	if !traceDBRawDecodeFamilyComplete(
		inventory.RawDecode, traceDBRawRetentionSwitchLite) {
		join.coverage.Metadata["join_state"] = traceDBSourceRawLaneFamilyRetentionWithdrawnState
		join.coverage.Skipped = "scheduler-lite switch join withheld: retained family record store exceeded its byte budget"
		return join
	}
	admitted := inventory.RawDecode.Metrics["target_sched_switch_lite_body_admitted"]
	if admitted != int64(len(inventory.RawSwitchLite)) ||
		inventory.RawDecode.Metrics["target_sched_switch_lite_record_capture_failed"] != 0 {
		join.coverage.Metadata["join_state"] = "withheld_retained_record_census_mismatch"
		join.coverage.Skipped = "scheduler-lite switch join withheld: retained/admitted record census mismatch"
		return join
	}
	for _, raw := range inventory.RawSwitchLite {
		if raw.HeaderPID != raw.PrevTID {
			traceDBAddCoverageMetric(
				&join.coverage, "raw_records_common_pid_differs_from_prev_tid", 1)
		}
		key, reason := traceDBRawSchedSwitchLiteKeyDecision(raw)
		if reason != "" {
			traceDBAddCoverageMetric(&join.coverage, "raw_records_key_rejected", 1)
			traceDBAddCoverageMetric(
				&join.coverage, "raw_records_key_rejected_"+reason, 1)
			continue
		}
		join.rawByKey[key] = append(join.rawByKey[key], raw)
		coordinate := traceDBRawSchedSwitchLiteCoordinate{
			TimestampNS: key.TimestampNS,
			CPU:         key.CPU,
		}
		join.rawByCoordinate[coordinate] = append(
			join.rawByCoordinate[coordinate], raw)
		traceDBAddCoverageMetric(&join.coverage, "raw_records_key_admitted", 1)
	}
	for _, cohort := range join.rawByKey {
		if len(cohort) > 1 {
			traceDBAddCoverageMetric(&join.coverage, "raw_ambiguous_key_cohorts", 1)
			traceDBAddCoverageMetric(&join.coverage, "raw_ambiguous_records", int64(len(cohort)))
		}
	}
	for _, cohort := range join.rawByCoordinate {
		if len(cohort) > 1 {
			traceDBAddCoverageMetric(&join.coverage, "raw_ambiguous_coordinate_cohorts", 1)
			traceDBAddCoverageMetric(
				&join.coverage, "raw_ambiguous_coordinate_records", int64(len(cohort)))
		}
	}
	join.ready = true
	if len(inventory.RawSwitchLite) == 0 {
		join.coverage.Metadata["join_state"] = "complete_no_source_records"
		join.coverage.Skipped = "scheduler-lite switch join complete: no source sched_switch_lite records"
	} else {
		join.coverage.Metadata["join_state"] = "ready_for_db_boundary_census"
	}
	return join
}

func newTraceDBRawSchedSwitchLiteJoinCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_scheduler_lite_join",
		Table:  "__raw_vs_db_sched_switch__",
		Role:   "query_ready_enrichment",
		FieldSources: map[string]string{
			"admission":             "one raw sched_switch_lite record and one audited DB sched_slice boundary must share one exact timestamp/CPU/canonical-public-PID/state/next-priority key",
			"identity":              "raw payload prev/next PIDs must equal canonical DB public TIDs; common_pid is retained only as an independently counted event-envelope observation and never becomes switch identity, host/namespace mapping or comm authority",
			"state":                 "raw prev_state mapped through the closed TraceStreamer EndState table and compared exactly with sched_slice.end_state",
			"priority":              "next priority must equal the next sched_slice priority; raw prev priority is the exact switch-out snapshot because sched_slice retains the earlier switch-in priority",
			"next_info":             "full authoritative packed uint64 retained as a raw hex receipt; only the currently documented prefix through cgroup ID is rendered semantically",
			"envelope":              "exact raw common_flags/common_preempt_count replace only the header defaults on the already-existing DB boundary row; common_pid is not rendered",
			"deduplication":         "raw or DB key multiplicity other than one is ineligible; an exact timestamp/CPU coordinate already emitted from audited DB sched_slice always wins and raw never emits a duplicate",
			"raw_unmatched":         "one unique raw timestamp/CPU record may publish only when no audited DB boundary is emitted there and both raw prev/next public TIDs resolve to one exact canonical thread/process generation at that point; exact idle zero is admitted only through canonical idle authority",
			"raw_db_time_alignment": "exact timestamp+CPU coordinate overlap is an observational alignment witness; zero overlap with nonempty admitted raw and audited DB lanes is advisory only and does not prove a clock-domain offset or gate publication",
		},
		Metadata: map[string]string{
			"join_state":              traceDBSourceRawLanePlaceholderState,
			"physical_event_contract": "enrich_existing_db_boundary_or_publish_unique_lifecycle_proven_raw_unmatched; duplicate_events=0",
		},
	}
}

func traceDBRawSchedSwitchLiteKey(
	raw traceDBRawSchedSwitchLiteRecord,
) (traceDBRawSchedSwitchLiteJoinKey, bool) {
	key, reason := traceDBRawSchedSwitchLiteKeyDecision(raw)
	return key, reason == ""
}

func traceDBRawSchedSwitchLiteKeyDecision(
	raw traceDBRawSchedSwitchLiteRecord,
) (traceDBRawSchedSwitchLiteJoinKey, string) {
	state, ok := traceDBRawSchedSwitchLiteEndState(raw.PrevState)
	if !ok {
		return traceDBRawSchedSwitchLiteJoinKey{}, "unsupported_prev_state"
	}
	if raw.TimestampNS > math.MaxInt64 {
		return traceDBRawSchedSwitchLiteJoinKey{}, "timestamp_out_of_range"
	}
	if !validTraceDBCPUIndex(int64(raw.CPU)) {
		return traceDBRawSchedSwitchLiteJoinKey{}, "cpu_out_of_range"
	}
	if raw.PrevTID < 0 || raw.PrevTID > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteJoinKey{}, "prev_tid_out_of_range"
	}
	if raw.NextTID < 0 || raw.NextTID > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteJoinKey{}, "next_tid_out_of_range"
	}
	if raw.Flags < 0 || raw.Flags > math.MaxUint8 {
		return traceDBRawSchedSwitchLiteJoinKey{}, "flags_out_of_range"
	}
	if raw.PreemptCount < 0 || raw.PreemptCount > math.MaxUint8 {
		return traceDBRawSchedSwitchLiteJoinKey{}, "preempt_count_out_of_range"
	}
	if raw.PrevPriority < math.MinInt32 || raw.PrevPriority >= math.MaxInt32 {
		return traceDBRawSchedSwitchLiteJoinKey{}, "prev_priority_out_of_range"
	}
	if raw.NextPriority < math.MinInt32 || raw.NextPriority >= math.MaxInt32 {
		return traceDBRawSchedSwitchLiteJoinKey{}, "next_priority_out_of_range"
	}
	return traceDBRawSchedSwitchLiteJoinKey{
		TimestampNS:  int64(raw.TimestampNS),
		CPU:          int64(raw.CPU),
		PrevTID:      raw.PrevTID,
		PrevState:    state,
		NextTID:      raw.NextTID,
		NextPriority: raw.NextPriority,
	}, ""
}

func traceDBSchedSwitchLiteBoundaryKey(
	prev, next traceDBSchedSlice,
) (traceDBRawSchedSwitchLiteJoinKey, bool) {
	if prev.CPU != next.CPU || prev.TS < 0 || prev.Dur < 0 || next.TS < 0 ||
		prev.TS > math.MaxInt64-prev.Dur || prev.TS+prev.Dur != next.TS ||
		!validTraceDBCPUIndex(prev.CPU) ||
		prev.TID < 0 || prev.TID > math.MaxInt32 ||
		next.TID < 0 || next.TID > math.MaxInt32 ||
		next.Priority < math.MinInt32 || next.Priority >= math.MaxInt32 ||
		prev.EndState == "" {
		return traceDBRawSchedSwitchLiteJoinKey{}, false
	}
	return traceDBRawSchedSwitchLiteJoinKey{
		TimestampNS:  next.TS,
		CPU:          prev.CPU,
		PrevTID:      prev.TID,
		PrevState:    prev.EndState,
		NextTID:      next.TID,
		NextPriority: next.Priority,
	}, true
}

func (join *traceDBRawSchedSwitchLiteJoin) auditDBBoundaries(
	ctx context.Context,
	tdb *traceDB,
	authority traceDBSchedulerAuthority,
	audit traceDBSchedAudit,
) error {
	if join == nil {
		return nil
	}
	traceDBAddCoverageMetric(&join.coverage, "db_boundaries_audited", int64(audit.ExpectedBoundaryRows))
	if !join.ready {
		return nil
	}
	if len(join.rawByKey) == 0 {
		join.dbReady = true
		return nil
	}
	rows, err := queryTraceDBSchedSliceRows(ctx, tdb)
	if err != nil {
		return err
	}
	defer rows.Close()
	previous := map[int64]traceDBSchedSourceRow{}
	counted := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := scanTraceDBSchedSourceRow(rows, authority)
		if err != nil {
			return err
		}
		if !current.CPUAssigned || !current.CPUInRange {
			continue
		}
		cpuAudit := audit.CPUs[current.Slice.CPU]
		if cpuAudit == nil || len(cpuAudit.Reasons) > 0 {
			continue
		}
		if len(current.ValidationGaps) > 0 {
			return fmt.Errorf("sched_slice changed after scheduler-lite join audit on cpu %d", current.Slice.CPU)
		}
		if prev, ok := previous[current.Slice.CPU]; ok {
			key, keyOK := traceDBSchedSwitchLiteBoundaryKey(prev.Slice, current.Slice)
			if !keyOK {
				return fmt.Errorf("sched_slice boundary changed after scheduler-lite join audit on cpu %d", current.Slice.CPU)
			}
			join.dbByKey[key]++
			join.dbEmittedByCoordinate[traceDBRawSchedSwitchLiteCoordinate{
				TimestampNS: key.TimestampNS,
				CPU:         key.CPU,
			}]++
			counted++
		}
		previous[current.Slice.CPU] = current
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if counted != audit.ExpectedBoundaryRows {
		return fmt.Errorf("scheduler-lite DB boundary census mismatch: counted=%d audited=%d",
			counted, audit.ExpectedBoundaryRows)
	}
	for coordinate, rawCohort := range join.rawByCoordinate {
		dbCount := join.dbEmittedByCoordinate[coordinate]
		if dbCount == 0 {
			continue
		}
		traceDBAddCoverageMetric(
			&join.coverage, "raw_db_exact_coordinate_overlap_cohorts", 1)
		traceDBAddCoverageMetric(
			&join.coverage, "raw_db_exact_coordinate_overlap_raw_records", int64(len(rawCohort)))
		traceDBAddCoverageMetric(
			&join.coverage, "raw_db_exact_coordinate_overlap_db_boundaries", int64(dbCount))
	}
	for key, rawCohort := range join.rawByKey {
		dbCount := join.dbByKey[key]
		switch {
		case len(rawCohort) != 1:
			// Counted during raw indexing.
		case dbCount == 1:
			join.eligible[key] = rawCohort[0]
		case dbCount > 1:
			traceDBAddCoverageMetric(&join.coverage, "db_ambiguous_key_cohorts", 1)
			traceDBAddCoverageMetric(&join.coverage, "db_ambiguous_boundaries", int64(dbCount))
		}
	}
	traceDBAddCoverageMetric(&join.coverage, "eligible_unique_boundaries", int64(len(join.eligible)))
	join.coverage.Metadata["join_state"] = "db_boundary_census_complete"
	join.dbReady = true
	return nil
}

func (join *traceDBRawSchedSwitchLiteJoin) match(
	prev, next traceDBSchedSlice,
) *traceDBRawSchedSwitchLiteRecord {
	if join == nil || !join.ready {
		return nil
	}
	key, ok := traceDBSchedSwitchLiteBoundaryKey(prev, next)
	if !ok || join.consumed[key] {
		return nil
	}
	raw, ok := join.eligible[key]
	if !ok {
		return nil
	}
	join.consumed[key] = true
	traceDBAddCoverageMetric(&join.coverage, "db_boundaries_enriched", 1)
	return &raw
}

func (join *traceDBRawSchedSwitchLiteJoin) publishRawUnmatched(
	sink *traceDBRowSink,
	authority traceDBSchedulerAuthority,
) error {
	if join == nil || !join.ready || !join.dbReady || sink == nil {
		return nil
	}
	coordinates := make([]traceDBRawSchedSwitchLiteCoordinate, 0, len(join.rawByCoordinate))
	for coordinate := range join.rawByCoordinate {
		coordinates = append(coordinates, coordinate)
	}
	sort.Slice(coordinates, func(i, j int) bool {
		if coordinates[i].TimestampNS != coordinates[j].TimestampNS {
			return coordinates[i].TimestampNS < coordinates[j].TimestampNS
		}
		return coordinates[i].CPU < coordinates[j].CPU
	})
	for _, coordinate := range coordinates {
		cohort := join.rawByCoordinate[coordinate]
		if len(cohort) != 1 {
			continue
		}
		if join.dbEmittedByCoordinate[coordinate] != 0 {
			traceDBAddCoverageMetric(
				&join.coverage, "raw_unmatched_withheld_db_coordinate_present", 1)
			continue
		}
		raw := cohort[0]
		key, reason := traceDBRawSchedSwitchLiteKeyDecision(raw)
		if reason != "" {
			return &traceDBOutputInvariantError{
				Reason: "scheduler_lite_raw_unmatched_key_changed_after_census",
			}
		}
		if join.consumed[key] {
			return &traceDBOutputInvariantError{
				Reason: "scheduler_lite_raw_unmatched_already_consumed",
			}
		}
		prev, reason := traceDBResolveRawSchedSwitchLiteSubject(
			authority, raw.PrevTID, coordinate.TimestampNS)
		if reason != "" {
			traceDBAddCoverageMetric(
				&join.coverage, "raw_unmatched_withheld_prev_"+
					traceDBRawDecodeReasonMetric(reason), 1)
			continue
		}
		next, reason := traceDBResolveRawSchedSwitchLiteSubject(
			authority, raw.NextTID, coordinate.TimestampNS)
		if reason != "" {
			traceDBAddCoverageMetric(
				&join.coverage, "raw_unmatched_withheld_next_"+
					traceDBRawDecodeReasonMetric(reason), 1)
			continue
		}
		if err := emitTraceDBRawSchedSwitchLite(sink, raw, prev, next); err != nil {
			return err
		}
		join.consumed[key] = true
		traceDBAddCoverageMetric(&join.coverage, "raw_unmatched_published", 1)
	}
	return nil
}

func traceDBResolveRawSchedSwitchLiteSubject(
	authority traceDBSchedulerAuthority,
	tid, timestamp int64,
) (traceDBSchedSlice, string) {
	if traceDBBeforeCaptureStart(authority.identities, timestamp) {
		return traceDBSchedSlice{}, "pre_capture_timestamp"
	}
	if tid == 0 {
		subject, ok := authority.schedulerSubjectFromExactITID(0, true)
		if !ok || !authority.schedulerPointAllows(subject, timestamp) {
			return traceDBSchedSlice{}, "idle_lifecycle_rejected"
		}
		return traceDBSchedSlice{
			TS: timestamp, TID: 0, TGID: 0, ITID: 0, IPID: 0, Name: "swapper",
		}, ""
	}
	thread, process, reason := traceDBResolveRawPublicTID(authority, tid, timestamp)
	if reason != "" {
		return traceDBSchedSlice{}, reason
	}
	return traceDBSchedSlice{
		TS: timestamp, TID: thread.TID, TGID: firstNonZero(process.PID, thread.TID),
		ITID: thread.ITID, IPID: thread.IPID, Name: authority.threadDisplayName(thread),
	}, ""
}

func (join *traceDBRawSchedSwitchLiteJoin) finalize() (TraceDBCoverage, error) {
	if join == nil {
		return newTraceDBRawSchedSwitchLiteJoinCoverage(), nil
	}
	enriched := join.coverage.Metrics["db_boundaries_enriched"]
	rawUnmatched := join.coverage.Metrics["raw_unmatched_published"]
	eligible := join.coverage.Metrics["eligible_unique_boundaries"]
	if enriched != eligible {
		err := &traceDBOutputInvariantError{Reason: "scheduler_lite_switch_join_publication_mismatch"}
		join.coverage.Error = err.Error()
		join.coverage.Metadata["join_state"] = "failed_publication_mismatch"
		return join.coverage, err
	}
	join.coverage.RowsEmitted = int(enriched + rawUnmatched)
	if !join.ready {
		return join.coverage, nil
	}
	if !join.dbReady {
		join.coverage.Metadata["join_state"] = "withheld_db_scheduler_census_unavailable"
		join.coverage.Skipped = "scheduler-lite switch join withheld: audited DB boundary census unavailable"
		return join.coverage, nil
	}
	rawAdmitted := join.coverage.Metrics["raw_records_key_admitted"]
	dbAudited := join.coverage.Metrics["db_boundaries_audited"]
	exactCoordinateOverlap :=
		join.coverage.Metrics["raw_db_exact_coordinate_overlap_cohorts"]
	switch {
	case rawAdmitted > 0 && dbAudited > 0 && exactCoordinateOverlap == 0:
		join.coverage.Metadata["raw_db_time_alignment_observation"] =
			"unproven_no_exact_timestamp_cpu_overlap"
	case rawAdmitted > 0 && dbAudited > 0:
		join.coverage.Metadata["raw_db_time_alignment_observation"] =
			"observed_exact_timestamp_cpu_overlap"
	default:
		join.coverage.Metadata["raw_db_time_alignment_observation"] =
			"not_applicable_empty_admitted_lane"
	}
	if join.coverage.Metadata["join_state"] == "complete_no_source_records" {
		return join.coverage, nil
	}
	traceDBAddCoverageMetric(&join.coverage, "raw_unique_records_unmatched",
		join.coverage.Metrics["raw_records_key_admitted"]-
			join.coverage.Metrics["raw_ambiguous_records"]-enriched-rawUnmatched)
	traceDBAddCoverageMetric(&join.coverage, "db_boundaries_unmatched",
		join.coverage.Metrics["db_boundaries_audited"]-enriched)
	if enriched == 0 && rawUnmatched == 0 {
		join.coverage.Metadata["join_state"] = "complete_no_unique_match"
		join.coverage.Skipped = "scheduler-lite switch join complete: no unique exact raw/DB boundary match"
	} else {
		join.coverage.Metadata["join_state"] = "published_unique_exact_scheduler_lite"
		join.coverage.Metadata["publication_contract"] = fmt.Sprintf(
			"enriched_boundaries=%d; raw_unmatched_physical_events=%d; duplicate_events=0",
			enriched, rawUnmatched)
	}
	return join.coverage, nil
}

func traceDBRawSchedSwitchLiteEndState(raw uint64) (string, bool) {
	switch raw {
	case 0:
		return "R", true
	case 1:
		return "S", true
	case 2:
		return "D", true
	case 21:
		return "D-IO", true
	case 22:
		return "D-NIO", true
	case 3:
		return "Running", true
	case 4:
		return "T", true
	case 8:
		return "t", true
	case 16:
		return "X", true
	case 32:
		return "Z", true
	case 64:
		return "P", true
	case 128:
		return "I", true
	case 130:
		return "DK", true
	case 131:
		return "DK-IO", true
	case 132:
		return "DK-NIO", true
	case 136:
		return "TK", true
	case 256, 2048:
		return "R+", true
	case 2049:
		return "R-B", true
	case 4096:
		return "S", true
	case 0x8000:
		return "U", true
	default:
		return "", false
	}
}
