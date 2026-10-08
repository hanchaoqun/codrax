package hitraceconv

import (
	"context"
	"fmt"
)

// Protocol witness: OpenHarmony developtools_smartperf_host at
// 5c5afb0c479b070148d8a6e336120638a1a03930, table/ftrace/
// so_static_initalization_table.cpp and filter/app_start_filter.cpp.
// The producer derives these intervals from thread-owned dlopen callstack
// slices. IPID is uint32 exposed as int64; TID is the public thread ID.
// Its process-relative display depth is not a physical thread-stack depth.
func exportTraceDBStaticInitialize(ctx context.Context, tdb *traceDB, _ *traceDBRowSink,
	syncSpans *traceDBSyncSpanAuthority, authority traceDBSchedulerAuthority,
	running traceDBSchedulerRunningIndex,
) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "static_initalize", []string{"start_time", "end_time", "so_name", "ipid", "tid"})
	coverage.FieldSources = map[string]string{
		"wire_laminar":     "strict accepted rows submit typed B/E candidates to the shared authority; no endpoint is published by this exporter",
		"source_admission": "complete physical-row scan with storage-class-pinned bounded scalars; exact unique (internal process ID, public thread ID) owner and shared closed-endpoint lifecycle admission",
		"identity_profile": "official uint32 IPID exposed as int64, public TID; signed-int32 aliases, missing and ambiguous owners are not guessed",
		"cpu":              "independent exact start/end typed Running witnesses; unavailable placement uses the typed CPU-unavailable lane, never CPU 0",
		"source_semantics": "derived dlopen callstack interval; source depth is process-relative presentation only; an existing unique admitted callstack with identical canonical owner, endpoints and exact source name retains the original interval instead of a duplicate static projection; conflicting derived projections are rejected without deleting original evidence",
		"rejection_scope":  "invalid derived rows remain rejected with explicit source counts; no row-name/time rescue and no reverse poisoning of independent callstack/raw evidence; accepted intervals do not prove a complete call tree",
	}
	fail := func(err error) (TraceDBCoverage, error) { coverage.Error = err.Error(); return coverage, err }
	if err != nil || !coverage.Found {
		return coverage, err
	}
	if syncSpans == nil || !authority.initialized || !running.initialized {
		return fail(&traceDBOutputInvariantError{Reason: "missing_static_initialize_source_authority"})
	}
	stable, stableKnown, err := traceDBSyncSpanHiddenRowID(ctx, tdb, &coverage)
	if err != nil {
		return fail(err)
	}
	if coverage.RowsRead == 0 {
		return coverage, nil
	}
	if !stableKnown {
		stable = "NULL"
	}
	columns := map[string]bool{}
	for _, column := range coverage.ColumnsPresent {
		columns[column] = true
	}
	integer := func(column string) string {
		if !columns[column] {
			return "NULL, NULL"
		}
		return fmt.Sprintf("typeof(%s), CASE WHEN typeof(%s)='integer' THEN %s END", column, column, column)
	}
	name := "NULL"
	if columns["so_name"] {
		// Do not materialize arbitrary BLOBs/oversize source text. A rejected
		// row remains in the source census and the source-admission fence.
		name = fmt.Sprintf("CASE WHEN typeof(so_name)='text' AND length(CAST(so_name AS BLOB))<=%d THEN so_name END", maxTraceDBCallstackTokenBytes-len("SoInit:"))
	}
	query := fmt.Sprintf("SELECT %s,%s,%s,%s,%s,%s FROM static_initalize", stable,
		integer("start_time"), integer("end_time"), name, integer("ipid"), integer("tid"))
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	scanned, skipped := 0, map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var rowID, startType, startValue, endType, endValue, so, ownerType, ownerValue, tidType, tidValue any
		if err := rows.Scan(&rowID, &startType, &startValue, &endType, &endValue, &so, &ownerType, &ownerValue, &tidType, &tidValue); err != nil {
			return fail(err)
		}
		scanned++
		start := traceDBBoundedSQLiteIntegerTransport(startType, startValue)
		end := traceDBBoundedSQLiteIntegerTransport(endType, endValue)
		owner := traceDBBoundedSQLiteIntegerTransport(ownerType, ownerValue)
		tid := traceDBBoundedSQLiteIntegerTransport(tidType, tidValue)
		candidate, reason := prepareTraceDBStaticInitializeRow(authority, running, rowID, start, end, so, owner, tid)
		if len(coverage.ColumnsMissing) > 0 {
			reason = "incomplete_source_schema"
		}
		if reason != "" {
			skipped[reason]++
			continue
		}
		// A static row is a projection of callstack, not independent work.
		// Reuse the indexed exact source-identity census, never a name search or
		// temporal-neighborhood match. Rejected/unavailable source candidates
		// cannot be rescued by a second derived producer either.
		original := candidate
		original.Name = candidate.Name[len("SoInit:"):]
		census, complete, err := syncSpans.censusCandidateCollisions(ctx, original)
		if err != nil {
			return fail(err)
		}
		if !complete {
			skipped["source_collision_audit_unavailable"]++
			continue
		}
		local := census.LocallyAdmittedInterval
		// Only already-admitted original callstack sources outrank this
		// derived table. Do not turn two conflicting static rows into a
		// first-row-wins policy: those still enter the shared conflict audit.
		if census.IntervalTotal > 0 && census.IntervalTotal == local.CallstackCPUKnown+local.CallstackCPUUnavailable {
			if census.IntervalTotal == 1 && census.SemanticTotal == 1 && census.LocallyAdmittedSemanticTotal == 1 &&
				local.Total == 1 && local.CallstackCPUKnown+local.CallstackCPUUnavailable == 1 {
				skipped["duplicate_callstack_projection"]++
			} else {
				skipped["conflicting_derived_projection"]++
			}
			continue
		}
		if err := syncSpans.submit(ctx, candidate); err != nil {
			return fail(err)
		}
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	if scanned != coverage.RowsRead {
		return fail(&traceDBOutputInvariantError{Reason: "static_initialize_source_scan_count_mismatch"})
	}
	traceDBAppendCoverageSkipped(&coverage, traceDBCountSummary(skipped))
	return coverage, nil
}

func traceDBStaticInitializeSubject(authority traceDBSchedulerAuthority, ownerRaw, tidRaw any) (traceDBThread, traceDBProcess, bool) {
	owner, ownerOK := traceDBStrictInternalID(ownerRaw)
	tid, tidOK := traceDBStrictPublicID(tidRaw)
	if !ownerOK || !tidOK || tid <= 0 || authority.identities.RejectedPublicTID[tid] {
		return traceDBThread{}, traceDBProcess{}, false
	}
	var selected traceDBThread
	count := 0
	for _, thread := range authority.identities.ByTIDCandidates[tid] {
		if thread.IPID == owner {
			selected = thread
			count++
		}
	}
	if count != 1 {
		return traceDBThread{}, traceDBProcess{}, false
	}
	thread, process, status := authority.resolveThreadSubject(selected.ITID)
	return thread, process, status == traceDBSchedulerThreadResolved && thread.TID == tid && thread.IPID == owner && process.PID > 0
}

func prepareTraceDBStaticInitializeRow(authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex,
	rowID, startRaw, endRaw, nameRaw, ownerRaw, tidRaw any,
) (traceDBSyncSpanCandidate, string) {
	c := traceDBSyncSpanCandidate{Producer: traceDBSyncSpanProducerStaticInitialize,
		StableKind: traceDBSyncSpanStableStaticInitializeRowID, NameProvenance: traceDBSyncSpanNameStaticObject}
	thread, process, found := traceDBStaticInitializeSubject(authority, ownerRaw, tidRaw)
	if !found {
		return c, "unresolved_static_owner_thread"
	}
	c.HeaderTID, c.HeaderTGID = thread.TID, process.PID
	c.CanonicalITID, c.CanonicalITIDKnown = thread.ITID, true
	c.OwnerIPID, c.OwnerIPIDKnown = thread.IPID, true
	c.Task = traceDBCommName(authority.threadDisplayName(thread), "unknown")
	var ok bool
	if c.StableID, ok = traceDBStrictSQLiteInt(rowID); !ok {
		return c, "invalid_hidden_rowid"
	}
	if c.Start, ok = traceDBStrictSQLiteInt(startRaw); !ok || c.Start < 0 {
		return c, "invalid_timestamp"
	}
	if c.End, ok = traceDBStrictSQLiteInt(endRaw); !ok || c.End < c.Start {
		return c, "invalid_end_timestamp"
	}
	if traceDBBeforeCaptureStart(authority.identities, c.Start) {
		return c, "before_capture_start"
	}
	name, reason := traceDBCallstackName(nameRaw)
	if reason != "" {
		return c, reason
	}
	c.Name = "SoInit:" + name
	if !traceDBCallstackSpanName(c.Name) {
		return c, "invalid_name_oversize"
	}
	if c.Start == c.End {
		if !authority.threadPointAllows(thread.ITID, c.Start) {
			return c, "lifecycle_rejected_sync_point"
		}
	} else if !authority.threadClosedEndpointAllows(thread.ITID, c.Start, c.End) ||
		!authority.processClosedEndpointAllows(thread.IPID, c.Start, c.End) {
		return c, "lifecycle_rejected_sync_closed_interval"
	}
	var startStatus, endStatus traceDBSchedulerRunningLookupStatus
	c.StartCPU, startStatus = running.lookupCPUAt(thread.ITID, c.Start)
	c.EndCPU, endStatus = running.lookupCPUAt(thread.ITID, c.End)
	switch {
	case startStatus == traceDBSchedulerRunningSourceTainted || endStatus == traceDBSchedulerRunningSourceTainted:
		c.CPUPlacement = traceDBSyncSpanCPUPlacementSourceTainted
	case startStatus == traceDBSchedulerRunningLifecycleRejected || endStatus == traceDBSchedulerRunningLifecycleRejected:
		c.CPUPlacement = traceDBSyncSpanCPUPlacementLifecycleRejected
	case startStatus != traceDBSchedulerRunningKnown:
		c.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownStart
	case endStatus != traceDBSchedulerRunningKnown:
		c.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownEnd
	}
	c.StartCPUProvenance, c.EndCPUProvenance = traceDBSyncSpanCPUStaticTypedRunning, traceDBSyncSpanCPUStaticTypedRunning
	if c.CPUPlacement != traceDBSyncSpanCPUPlacementKnown {
		c.StartCPU, c.EndCPU = 0, 0
		c.StartCPUProvenance, c.EndCPUProvenance = traceDBSyncSpanCPUStaticUnavailable, traceDBSyncSpanCPUStaticUnavailable
	}
	return c, ""
}
