package hitraceconv

import (
	"context"
	"fmt"
	"math"
)

type traceDBRawSchedWakeupLiteJoinKey struct {
	TimestampNS int64
	WakerTID    int64
	WakeeTID    int64
	TargetCPU   int64
}

type traceDBRawSchedWakeupLiteJoin struct {
	coverage TraceDBCoverage
	rawByKey map[traceDBRawSchedWakeupLiteJoinKey][]traceDBRawSchedWakeupLiteRecord
	dbByKey  map[traceDBRawSchedWakeupLiteJoinKey]int
	eligible map[traceDBRawSchedWakeupLiteJoinKey]traceDBRawSchedWakeupLiteRecord
	consumed map[traceDBRawSchedWakeupLiteJoinKey]bool
	ready    bool
	dbReady  bool
}

func newTraceDBRawSchedWakeupLiteJoin(
	inventory *traceDBSourceNameInventory,
) *traceDBRawSchedWakeupLiteJoin {
	join := &traceDBRawSchedWakeupLiteJoin{
		coverage: newTraceDBRawSchedWakeupLiteJoinCoverage(),
		rawByKey: map[traceDBRawSchedWakeupLiteJoinKey][]traceDBRawSchedWakeupLiteRecord{},
		dbByKey:  map[traceDBRawSchedWakeupLiteJoinKey]int{},
		eligible: map[traceDBRawSchedWakeupLiteJoinKey]traceDBRawSchedWakeupLiteRecord{},
		consumed: map[traceDBRawSchedWakeupLiteJoinKey]bool{},
	}
	if inventory != nil {
		traceDBAttachRawSchedulerLiteDiagnostics(
			&join.coverage, inventory.RawDecode, "sched_wakeup_lite")
		join.coverage.Found = len(inventory.RawWakeupLite) > 0
		join.coverage.RowsRead = len(inventory.RawWakeupLite)
		traceDBAddCoverageMetric(&join.coverage, "raw_records_retained", int64(len(inventory.RawWakeupLite)))
	}
	// The class gate (source_raw_lane_gate.go) splits absent/non-official
	// source (not applicable) from an official census that did not close
	// (census incomplete); an unrecognized ledger shape fails loud on the
	// coverage and the join stays not ready.
	if stop, _ := traceDBApplySourceRawLaneGateKeyed(&join.coverage, inventory,
		traceDBSourceRawLaneStateKeyJoin, "scheduler-lite wakeup join"); stop {
		return join
	}
	// Past the gate the census closed; the family predicate can only fail on
	// the family's own retention store having been withdrawn by byte budget.
	if !traceDBRawDecodeFamilyComplete(
		inventory.RawDecode, traceDBRawRetentionWakeupLite) {
		join.coverage.Metadata["join_state"] = traceDBSourceRawLaneFamilyRetentionWithdrawnState
		join.coverage.Skipped = "scheduler-lite wakeup join withheld: retained family record store exceeded its byte budget"
		return join
	}
	// TraceStreamer aliases exact and lite inputs into the same DB event name.
	// If an exact sched_wakeup physical record exists, the DB cannot prove which
	// source format produced a same-shaped edge, so the lite join is withdrawn.
	if inventory.RawDecode.Metrics["target_sched_wakeup_records"] != 0 {
		join.coverage.Metadata["join_state"] = "withheld_exact_sched_wakeup_source_present"
		join.coverage.Skipped = "scheduler-lite wakeup join withheld: exact sched_wakeup source records also present"
		return join
	}
	admitted := inventory.RawDecode.Metrics["target_sched_wakeup_lite_body_admitted"]
	if admitted != int64(len(inventory.RawWakeupLite)) ||
		inventory.RawDecode.Metrics["target_sched_wakeup_lite_record_capture_failed"] != 0 {
		join.coverage.Metadata["join_state"] = traceDBSourceRawLaneRetainedRecordCensusMismatchState
		join.coverage.Skipped = "scheduler-lite wakeup join withheld: retained/admitted record census mismatch"
		return join
	}
	for _, raw := range inventory.RawWakeupLite {
		key, ok := traceDBRawSchedWakeupLiteKey(raw)
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
	if len(inventory.RawWakeupLite) == 0 {
		join.coverage.Metadata["join_state"] = "complete_no_source_records"
		join.coverage.Skipped = "scheduler-lite wakeup join complete: no source sched_wakeup_lite records"
	} else {
		join.coverage.Metadata["join_state"] = "ready_for_db_edge_census"
	}
	return join
}

func newTraceDBRawSchedWakeupLiteJoinCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_scheduler_lite_join",
		Table:  "__raw_vs_db_sched_wakeup__",
		Role:   "query_ready_enrichment",
		FieldSources: map[string]string{
			"admission":     "one raw sched_wakeup_lite record and one already-unique instant/raw DB edge must share exact timestamp, canonical public endpoints and target CPU",
			"identity":      "raw common_pid must equal canonical DB waker TID and raw payload pid must equal canonical DB wakee TID; no host/namespace rewrite or comm matching",
			"db_shape":      "rawtrace-only TraceStreamer shape: instant.name=sched_wakeup and raw.itid=instant.ref; sched_wakeup_new and bytrace waker-shaped rows are ineligible",
			"source_cpu":    "exact raw page CPU replaces Running-interval inference only for a unique exact lite match",
			"target_cpu":    "raw lite target_cpu must equal the already-paired DB raw.cpu",
			"priority":      "exact positive signed-16 raw lite priority; nonpositive values remain diagnostic because trace-query wakeup authority requires a present positive priority",
			"envelope":      "exact raw common_flags/common_preempt_count replace only the header defaults on the already-existing DB wakeup row",
			"deduplication": "raw or DB key multiplicity other than one is ineligible; enrichment never emits a second scheduler event",
		},
		Metadata: map[string]string{
			"join_state":              traceDBSourceRawLanePlaceholderState,
			"physical_event_contract": "enrich_existing_db_wakeup_only; duplicate_events=0",
		},
	}
}

func traceDBRawSchedWakeupLiteKey(
	raw traceDBRawSchedWakeupLiteRecord,
) (traceDBRawSchedWakeupLiteJoinKey, bool) {
	if raw.TimestampNS > math.MaxInt64 ||
		!validTraceDBCPUIndex(int64(raw.CPU)) ||
		raw.HeaderPID <= 0 || raw.HeaderPID > math.MaxInt32 ||
		raw.TargetTID <= 0 || raw.TargetTID > math.MaxInt32 ||
		raw.Priority <= 0 || raw.Priority >= math.MaxInt32 ||
		!validTraceDBCPUIndex(raw.TargetCPU) ||
		raw.Flags < 0 || raw.Flags > math.MaxUint8 ||
		raw.PreemptCount < 0 || raw.PreemptCount > math.MaxUint8 {
		return traceDBRawSchedWakeupLiteJoinKey{}, false
	}
	return traceDBRawSchedWakeupLiteJoinKey{
		TimestampNS: int64(raw.TimestampNS),
		WakerTID:    raw.HeaderPID,
		WakeeTID:    raw.TargetTID,
		TargetCPU:   raw.TargetCPU,
	}, true
}

func traceDBSchedWakeupLiteDBKey(
	instant traceDBWakeupInstant,
	raw traceDBRawWakeup,
	waker, wakee traceDBThread,
) (traceDBRawSchedWakeupLiteJoinKey, bool) {
	if instant.Name != "sched_wakeup" || raw.Name != "sched_wakeup" ||
		instant.TS < 0 || raw.TS != instant.TS ||
		raw.ITID != instant.Ref ||
		waker.ITID != instant.WakeupFrom || wakee.ITID != instant.Ref ||
		waker.TID <= 0 || waker.TID > math.MaxInt32 ||
		wakee.TID <= 0 || wakee.TID > math.MaxInt32 ||
		!validTraceDBCPUIndex(raw.TargetCPU) {
		return traceDBRawSchedWakeupLiteJoinKey{}, false
	}
	return traceDBRawSchedWakeupLiteJoinKey{
		TimestampNS: instant.TS,
		WakerTID:    waker.TID,
		WakeeTID:    wakee.TID,
		TargetCPU:   raw.TargetCPU,
	}, true
}

func (join *traceDBRawSchedWakeupLiteJoin) auditDBEdges(
	ctx context.Context,
	groups map[traceDBWakeupKey][]traceDBWakeupInstant,
	rawGroups map[traceDBWakeupKey][]traceDBRawWakeup,
	authority traceDBSchedulerAuthority,
) error {
	if join == nil || !join.ready {
		return nil
	}
	if len(join.rawByKey) == 0 {
		join.dbReady = true
		return nil
	}
	for key, instants := range groups {
		if err := ctx.Err(); err != nil {
			return err
		}
		raws := rawGroups[key]
		matching, reason := traceDBUniqueWakeupMatching(instants, raws)
		if reason != "" {
			traceDBAddCoverageMetric(&join.coverage, "db_edges_pairing_rejected", int64(len(instants)))
			continue
		}
		for instantIndex, rawIndex := range matching {
			instant := instants[instantIndex]
			dbRaw := raws[rawIndex]
			wakee, _, wakeeResolution := authority.resolveThreadSubject(instant.Ref)
			waker, _, wakerResolution := authority.resolveThreadSubject(instant.WakeupFrom)
			if wakeeResolution != traceDBSchedulerThreadResolved ||
				wakerResolution != traceDBSchedulerThreadResolved ||
				!authority.threadPointAllows(instant.Ref, instant.TS) ||
				!authority.threadPointAllows(instant.WakeupFrom, instant.TS) {
				traceDBAddCoverageMetric(&join.coverage, "db_edges_identity_or_lifecycle_rejected", 1)
				continue
			}
			dbKey, ok := traceDBSchedWakeupLiteDBKey(instant, dbRaw, waker, wakee)
			if !ok {
				traceDBAddCoverageMetric(&join.coverage, "db_edges_shape_rejected", 1)
				continue
			}
			join.dbByKey[dbKey]++
			traceDBAddCoverageMetric(&join.coverage, "db_edges_key_admitted", 1)
		}
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
			traceDBAddCoverageMetric(&join.coverage, "db_ambiguous_edges", int64(dbCount))
		}
	}
	traceDBAddCoverageMetric(&join.coverage, "eligible_unique_edges", int64(len(join.eligible)))
	join.coverage.Metadata["join_state"] = "db_edge_census_complete"
	join.dbReady = true
	return nil
}

func (join *traceDBRawSchedWakeupLiteJoin) match(
	instant traceDBWakeupInstant,
	dbRaw traceDBRawWakeup,
	waker, wakee traceDBThread,
) *traceDBRawSchedWakeupLiteRecord {
	if join == nil || !join.ready {
		return nil
	}
	key, ok := traceDBSchedWakeupLiteDBKey(instant, dbRaw, waker, wakee)
	if !ok || join.consumed[key] {
		return nil
	}
	raw, ok := join.eligible[key]
	if !ok {
		return nil
	}
	join.consumed[key] = true
	traceDBAddCoverageMetric(&join.coverage, "db_edges_enriched", 1)
	return &raw
}

func (join *traceDBRawSchedWakeupLiteJoin) finalize() (TraceDBCoverage, error) {
	if join == nil {
		return newTraceDBRawSchedWakeupLiteJoinCoverage(), nil
	}
	enriched := join.coverage.Metrics["db_edges_enriched"]
	eligible := join.coverage.Metrics["eligible_unique_edges"]
	if enriched != eligible {
		err := &traceDBOutputInvariantError{Reason: "scheduler_lite_wakeup_join_publication_mismatch"}
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
		join.coverage.Skipped = "scheduler-lite wakeup join withheld: audited DB edge census unavailable"
		return join.coverage, nil
	}
	if join.coverage.Metadata["join_state"] == "complete_no_source_records" {
		return join.coverage, nil
	}
	traceDBAddCoverageMetric(&join.coverage, "raw_unique_records_unmatched",
		join.coverage.Metrics["raw_records_key_admitted"]-
			join.coverage.Metrics["raw_ambiguous_records"]-enriched)
	traceDBAddCoverageMetric(&join.coverage, "db_edges_unmatched",
		join.coverage.Metrics["db_edges_key_admitted"]-enriched)
	if enriched == 0 {
		join.coverage.Metadata["join_state"] = "complete_no_unique_match"
		join.coverage.Skipped = "scheduler-lite wakeup join complete: no unique exact raw/DB edge match"
	} else {
		join.coverage.Metadata["join_state"] = "published_unique_exact_enrichment"
		join.coverage.Metadata["publication_contract"] = fmt.Sprintf(
			"enriched_edges=%d; additional_physical_events=0", enriched)
	}
	return join.coverage, nil
}
