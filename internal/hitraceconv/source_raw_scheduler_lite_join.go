package hitraceconv

import (
	"context"
	"fmt"
	"math"
)

type traceDBRawSchedSwitchLiteJoinKey struct {
	TimestampNS  int64
	CPU          int64
	PrevTID      int64
	PrevState    string
	NextTID      int64
	NextPriority int64
}

type traceDBRawSchedSwitchLiteJoin struct {
	coverage TraceDBCoverage
	rawByKey map[traceDBRawSchedSwitchLiteJoinKey][]traceDBRawSchedSwitchLiteRecord
	dbByKey  map[traceDBRawSchedSwitchLiteJoinKey]int
	eligible map[traceDBRawSchedSwitchLiteJoinKey]traceDBRawSchedSwitchLiteRecord
	consumed map[traceDBRawSchedSwitchLiteJoinKey]bool
	ready    bool
	dbReady  bool
}

func newTraceDBRawSchedSwitchLiteJoin(
	inventory *traceDBSourceNameInventory,
) *traceDBRawSchedSwitchLiteJoin {
	join := &traceDBRawSchedSwitchLiteJoin{
		coverage: newTraceDBRawSchedSwitchLiteJoinCoverage(),
		rawByKey: map[traceDBRawSchedSwitchLiteJoinKey][]traceDBRawSchedSwitchLiteRecord{},
		dbByKey:  map[traceDBRawSchedSwitchLiteJoinKey]int{},
		eligible: map[traceDBRawSchedSwitchLiteJoinKey]traceDBRawSchedSwitchLiteRecord{},
		consumed: map[traceDBRawSchedSwitchLiteJoinKey]bool{},
	}
	if inventory == nil || inventory.RawDecode.Role == "" {
		join.coverage.Metadata["join_state"] = "unavailable_source_raw_decode_ledger"
		join.coverage.Skipped = "scheduler-lite switch join unavailable: source raw decode ledger absent"
		return join
	}
	join.coverage.Found = len(inventory.RawSwitchLite) > 0
	join.coverage.RowsRead = len(inventory.RawSwitchLite)
	traceDBAddCoverageMetric(&join.coverage, "raw_records_retained", int64(len(inventory.RawSwitchLite)))
	if inventory.RawDecode.Metadata["decode_state"] != "strict_target_ledger_complete" {
		join.coverage.Metadata["join_state"] = "withheld_source_raw_decode_incomplete"
		join.coverage.Skipped = "scheduler-lite switch join withheld: source raw decode ledger incomplete"
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
		key, ok := traceDBRawSchedSwitchLiteKey(raw)
		if !ok {
			traceDBAddCoverageMetric(&join.coverage, "raw_records_key_rejected", 1)
			continue
		}
		join.rawByKey[key] = append(join.rawByKey[key], raw)
		traceDBAddCoverageMetric(&join.coverage, "raw_records_key_admitted", 1)
	}
	for _, cohort := range join.rawByKey {
		if len(cohort) > 1 {
			traceDBAddCoverageMetric(&join.coverage, "raw_ambiguous_key_cohorts", 1)
			traceDBAddCoverageMetric(&join.coverage, "raw_ambiguous_records", int64(len(cohort)))
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
			"admission":     "one raw sched_switch_lite record and one audited DB sched_slice boundary must share one exact timestamp/CPU/canonical-public-PID/state/next-priority key",
			"identity":      "raw common_pid must equal raw prev_pid; raw public prev/next PIDs must equal canonical DB public TIDs; no host/namespace rewrite or comm matching",
			"state":         "raw prev_state mapped through the closed TraceStreamer EndState table and compared exactly with sched_slice.end_state",
			"priority":      "next priority must equal the next sched_slice priority; raw prev priority is the exact switch-out snapshot because sched_slice retains the earlier switch-in priority",
			"next_info":     "full authoritative packed uint64 retained as a raw hex receipt; only the currently documented prefix through cgroup ID is rendered semantically",
			"envelope":      "exact raw common_flags/common_preempt_count replace only the header defaults on the already-existing DB boundary row",
			"deduplication": "raw or DB key multiplicity other than one is ineligible; enrichment never emits a second scheduler event",
		},
		Metadata: map[string]string{
			"join_state":              "unavailable",
			"physical_event_contract": "enrich_existing_db_boundary_only; duplicate_events=0",
		},
	}
}

func traceDBRawSchedSwitchLiteKey(
	raw traceDBRawSchedSwitchLiteRecord,
) (traceDBRawSchedSwitchLiteJoinKey, bool) {
	state, ok := traceDBRawSchedSwitchLiteEndState(raw.PrevState)
	if !ok || raw.TimestampNS > math.MaxInt64 || !validTraceDBCPUIndex(int64(raw.CPU)) ||
		raw.PrevTID < 0 || raw.PrevTID > math.MaxInt32 ||
		raw.NextTID < 0 || raw.NextTID > math.MaxInt32 ||
		raw.HeaderPID != raw.PrevTID ||
		raw.Flags < 0 || raw.Flags > math.MaxUint8 ||
		raw.PreemptCount < 0 || raw.PreemptCount > math.MaxUint8 ||
		raw.PrevPriority < math.MinInt32 || raw.PrevPriority >= math.MaxInt32 ||
		raw.NextPriority < math.MinInt32 || raw.NextPriority >= math.MaxInt32 {
		return traceDBRawSchedSwitchLiteJoinKey{}, false
	}
	return traceDBRawSchedSwitchLiteJoinKey{
		TimestampNS:  int64(raw.TimestampNS),
		CPU:          int64(raw.CPU),
		PrevTID:      raw.PrevTID,
		PrevState:    state,
		NextTID:      raw.NextTID,
		NextPriority: raw.NextPriority,
	}, true
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

func (join *traceDBRawSchedSwitchLiteJoin) finalize() (TraceDBCoverage, error) {
	if join == nil {
		return newTraceDBRawSchedSwitchLiteJoinCoverage(), nil
	}
	enriched := join.coverage.Metrics["db_boundaries_enriched"]
	eligible := join.coverage.Metrics["eligible_unique_boundaries"]
	if enriched != eligible {
		err := &traceDBOutputInvariantError{Reason: "scheduler_lite_switch_join_publication_mismatch"}
		join.coverage.Error = err.Error()
		join.coverage.Metadata["join_state"] = "failed_publication_mismatch"
		return join.coverage, err
	}
	join.coverage.RowsEmitted = int(enriched)
	if !join.ready {
		return join.coverage, nil
	}
	if !join.dbReady {
		join.coverage.Metadata["join_state"] = "withheld_db_scheduler_census_unavailable"
		join.coverage.Skipped = "scheduler-lite switch join withheld: audited DB boundary census unavailable"
		return join.coverage, nil
	}
	if join.coverage.Metadata["join_state"] == "complete_no_source_records" {
		return join.coverage, nil
	}
	traceDBAddCoverageMetric(&join.coverage, "raw_unique_records_unmatched",
		join.coverage.Metrics["raw_records_key_admitted"]-
			join.coverage.Metrics["raw_ambiguous_records"]-enriched)
	traceDBAddCoverageMetric(&join.coverage, "db_boundaries_unmatched",
		join.coverage.Metrics["db_boundaries_audited"]-enriched)
	if enriched == 0 {
		join.coverage.Metadata["join_state"] = "complete_no_unique_match"
		join.coverage.Skipped = "scheduler-lite switch join complete: no unique exact raw/DB boundary match"
	} else {
		join.coverage.Metadata["join_state"] = "published_unique_exact_enrichment"
		join.coverage.Metadata["publication_contract"] = fmt.Sprintf(
			"enriched_boundaries=%d; additional_physical_events=0", enriched)
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
