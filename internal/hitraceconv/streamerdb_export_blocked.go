package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	traceDBArgTypeInt    int64 = 0
	traceDBArgTypeString int64 = 1
)

type traceDBBlockedArg struct {
	DataType int64
	Int      int64
	Text     string
	Valid    bool
}

type traceDBBlockedArgset struct {
	Values      map[string]traceDBBlockedArg
	HasRelevant bool
	Invalid     bool
}

type traceDBBlockedStableSource struct {
	Expr            string
	Label           string
	ProjectedUint32 bool
}

type traceDBBlockedStateRow struct {
	ID       int64
	TS       int64
	Dur      int64
	ITID     int64
	TID      int64
	PID      int64
	State    string
	ArgSetID int64
}

type traceDBBlockedSchedSlice struct {
	TS  int64
	Dur int64
	CPU int64
}

type traceDBPreparedBlockedReason struct {
	Row           traceDBBlockedStateRow
	Thread        traceDBThread
	Process       traceDBProcess
	IOWait        int64
	Caller        string
	CallerRaw     string
	CallerQuality string
	Delay         int64
	DelayPresent  bool
}

type traceDBBlockedBoundaryKey struct {
	ITID       int64
	StateStart int64
}

type traceDBBlockedBoundaryIndex struct {
	Matches               map[traceDBBlockedBoundaryKey][]traceDBBlockedSchedSlice
	LifecycleRejected     map[traceDBBlockedBoundaryKey]bool
	OverflowITIDs         map[int64]bool
	TaintedITIDs          map[int64]bool
	LowerBoundByITID      map[int64]int64
	GlobalBarriers        map[int64]bool
	GlobalLowerBound      int64
	GlobalLowerBoundKnown bool
	GlobalTaint           bool
}

func exportTraceDBBlockedReasons(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, authority traceDBSchedulerAuthority) (TraceDBCoverage, error) {
	if tdb != nil {
		tdb.rawBlockedKeyCoverage = newTraceDBRawBlockedKeyCoverage()
	}
	coverage := TraceDBCoverage{
		Family: "scheduler",
		Table:  "thread_state.arg_setid",
		Role:   "query_ready_export",
		SourceTables: []string{
			"thread_state", "args", "data_dict", "data_type", "sched_slice",
		},
		FieldSources: map[string]string{
			"argset":        "shared strict args/data_dict/data_type resolver; logical identity=(argset,canonical_key)",
			"caller":        "args.caller_str|args.caller; modern_4002_safe_token_gate",
			"caller_raw":    "args.caller_raw|hex_address(args.caller)",
			"delay":         "args.delay",
			"header_cpu":    "unique_same_itid_sched_slice.cpu_where_slice.ts+slice.dur==thread_state.ts",
			"header_thread": "thread_state_subject_projection; original_header_thread_known=false",
			"iowait":        "args.iowait; io_wait_compat_alias; verified_against_split_D/DK_state_when_present; authoritative_for_other_nonempty_states",
			"lifecycle":     "same collector authority; blocked candidate requires non-idle thread and positive-process point admission, and its exact predecessor sched_slice requires closed endpoint admission",
			"pid":           "thread_state.tid; thread_state.tid/pid must exactly match canonical thread/process identity; optional args.pid must match tid",
			"source":        "thread_state_argset",
			"timestamp":     "thread_state.ts_projection; original_timestamp_known=false",
		},
	}

	threadCoverage, err := tdb.inspectCoverage(ctx, "scheduler.blocked_reason.schema", "thread_state",
		[]string{"ts", "dur", "itid", "tid", "pid", "state", "arg_setid"})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	argsCoverage, err := tdb.inspectCoverage(ctx, "scheduler.blocked_reason.schema", "args",
		[]string{"key", "datatype", "value", "argset"})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	dictCoverage, err := tdb.inspectCoverage(ctx, "scheduler.blocked_reason.schema", "data_dict", []string{"id", "data"})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	schedCoverage, err := tdb.inspectCoverage(ctx, "scheduler.blocked_reason.schema", "sched_slice",
		[]string{"ts", "dur", "cpu", "itid"})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	traceDBBlockedMergeSchemaCoverage(&coverage, "thread_state", threadCoverage)
	traceDBBlockedMergeSchemaCoverage(&coverage, "args", argsCoverage)
	traceDBBlockedMergeSchemaCoverage(&coverage, "data_dict", dictCoverage)
	traceDBBlockedMergeSchemaCoverage(&coverage, "sched_slice", schedCoverage)
	coverage.Found = threadCoverage.Found && argsCoverage.Found && dictCoverage.Found && schedCoverage.Found
	if !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		coverage.Skipped = traceDBBlockedSchemaSkip(threadCoverage, argsCoverage, dictCoverage, schedCoverage)
		return coverage, nil
	}
	stateStable, ok, err := traceDBBlockedStableSourceForTable(ctx, tdb, "thread_state")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	if !ok {
		coverage.FieldSources["stable_identity"] = "unavailable: no thread_state.id and no provable SQLite hidden rowid"
		coverage.Skipped = "missing thread_state.id and usable SQLite rowid; blocked rows have no stable source identity/order"
		return coverage, nil
	}
	if stateStable.ProjectedUint32 {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "thread_state.id")
	}
	sort.Strings(coverage.ColumnsPresent)
	coverage.FieldSources["stable_identity"] = stateStable.Label
	if stateStable.ProjectedUint32 {
		coverage.FieldSources["same_timestamp_order"] = "thread_state.ts,canonical_uint32(thread_state.id)"
	} else {
		coverage.FieldSources["same_timestamp_order"] = "thread_state.ts,thread_state.rowid"
	}
	duplicateStateIDs, err := traceDBBlockedDuplicateStableIDs(ctx, tdb, "thread_state", stateStable)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}

	sharedArgsets, _, err := tdb.loadArgsets(ctx)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	argsets := traceDBBlockedArgsetsFromShared(sharedArgsets)
	skipped := map[string]int{}
	prepared, candidateStarts, semanticCohorts, rowsRead, err := loadTraceDBBlockedCandidates(ctx, tdb, authority, argsets, stateStable, duplicateStateIDs, skipped)
	coverage.RowsRead = rowsRead
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	boundaries, err := loadTraceDBBlockedSchedBoundaries(ctx, tdb, authority, candidateStarts)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	emittedCandidates := make([]traceDBPreparedBlockedReason, 0, len(prepared))
	for _, candidate := range prepared {
		row := candidate.Row
		key := traceDBBlockedBoundaryKey{ITID: row.ITID, StateStart: row.TS}
		if semanticCohorts[key] != 1 {
			skipped["ambiguous_thread_state_candidate"]++
			continue
		}
		if boundaryReason := traceDBBlockedBoundaryIntegrityReason(boundaries, key); boundaryReason != "" {
			skipped[boundaryReason]++
			continue
		}
		headerCPU, cpuReason := traceDBBlockedHeaderCPU(boundaries.Matches[key], boundaries.OverflowITIDs[row.ITID])
		if cpuReason != "" {
			skipped[cpuReason]++
			continue
		}
		body := fmt.Sprintf("sched_blocked_reason: pid=%d iowait=%d caller=%s", row.TID, candidate.IOWait, candidate.Caller)
		if candidate.CallerRaw != "" {
			body += " caller_raw=" + candidate.CallerRaw
		}
		body += " caller_quality=" + candidate.CallerQuality
		if candidate.DelayPresent {
			body += fmt.Sprintf(" delay=%d", candidate.Delay)
		}
		body += " timestamp_source=thread_state_start_projection original_timestamp_known=false"
		body += " header_thread_source=thread_state_subject_projection original_header_thread_known=false"
		body += " header_cpu_source=exact_prev_sched_slice_boundary source=thread_state_argset"
		if err := addTraceDBInstantRow(sink, row.TS, traceDBCommName(candidate.Thread.Name, "unknown"), row.TID,
			candidate.Process.PID, headerCPU, body); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
		emittedCandidates = append(emittedCandidates, candidate)
	}
	coverage.Skipped = traceDBBlockedSkipSummary(skipped)
	if tdb != nil {
		tdb.rawBlockedKeyCoverage =
			traceDBRawBlockedKeyCoverage(tdb.sourceNameInventory, emittedCandidates, authority)
	}
	return coverage, nil
}

func traceDBBlockedStableSourceForTable(ctx context.Context, tdb *traceDB, table string) (traceDBBlockedStableSource, bool, error) {
	hasSourceID, err := tdb.columnExists(ctx, table, "id")
	if err != nil {
		return traceDBBlockedStableSource{}, false, err
	}
	if hasSourceID {
		return traceDBBlockedStableSource{
			Expr:            quoteSQLiteIdent("id"),
			Label:           table + ".id with exact full-uint32 signed-int32 projection",
			ProjectedUint32: true,
		}, true, nil
	}
	expr, _, err := traceDBHiddenRowIDExpr(ctx, tdb.db, table)
	if err != nil {
		return traceDBBlockedStableSource{}, false, nil
	}
	return traceDBBlockedStableSource{Expr: expr, Label: table + "." + expr}, true, nil
}

func traceDBBlockedDecodeStableID(source traceDBBlockedStableSource, raw any) (int64, bool) {
	if source.ProjectedUint32 {
		return traceDBStrictStableUint32Projection(raw)
	}
	return traceDBStrictSQLiteInt(raw)
}

func traceDBBlockedStableOrderExpr(source traceDBBlockedStableSource) string {
	return traceDBStableUint32OrderExpr(source.Expr, source.ProjectedUint32)
}

func traceDBBlockedDuplicateStableIDs(ctx context.Context, tdb *traceDB, table string, source traceDBBlockedStableSource) (map[int64]bool, error) {
	duplicates := map[int64]bool{}
	if !source.ProjectedUint32 {
		// A proven SQLite hidden rowid is unique by construction. Avoid an O(N)
		// audit and its matching heap footprint on production-sized state tables.
		return duplicates, nil
	}
	canonical := `CASE WHEN ` + source.Expr + ` < 0 THEN ` + source.Expr + ` + 4294967296 ELSE ` + source.Expr + ` END`
	rows, err := tdb.db.QueryContext(ctx, `SELECT `+canonical+`, COUNT(*) FROM `+quoteSQLiteIdent(table)+
		` WHERE typeof(`+source.Expr+`)='integer' AND `+source.Expr+` BETWEEN -2147483648 AND 4294967295`+
		` GROUP BY `+canonical+` HAVING COUNT(*) > 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var identityRaw, countRaw any
		if err := rows.Scan(&identityRaw, &countRaw); err != nil {
			return nil, err
		}
		identity, identityOK := traceDBStrictSQLiteInt(identityRaw)
		count, countOK := traceDBStrictSQLiteInt(countRaw)
		if !identityOK || identity < 0 || identity > math.MaxUint32 || !countOK || count < 2 {
			return nil, fmt.Errorf("invalid strict duplicate cohort for %s stable identity", table)
		}
		duplicates[identity] = true
	}
	return duplicates, rows.Err()
}

func loadTraceDBBlockedCandidates(
	ctx context.Context,
	tdb *traceDB,
	authority traceDBSchedulerAuthority,
	argsets map[int64]traceDBBlockedArgset,
	stable traceDBBlockedStableSource,
	duplicateStateIDs map[int64]bool,
	skipped map[string]int,
) ([]traceDBPreparedBlockedReason, map[int64]map[int64]bool, map[traceDBBlockedBoundaryKey]int, int, error) {
	var prepared []traceDBPreparedBlockedReason
	candidateStarts := map[int64]map[int64]bool{}
	semanticCohorts := map[traceDBBlockedBoundaryKey]int{}
	rowsRead := 0
	query := `SELECT ` + stable.Expr + `, ts, dur, itid, tid, pid, state, arg_setid FROM thread_state ORDER BY ts, ` + traceDBBlockedStableOrderExpr(stable)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		return prepared, candidateStarts, semanticCohorts, rowsRead, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return prepared, candidateStarts, semanticCohorts, rowsRead, err
		}
		var idRaw, tsRaw, durRaw, itidRaw, tidRaw, pidRaw, stateRaw, argSetRaw any
		if err := rows.Scan(&idRaw, &tsRaw, &durRaw, &itidRaw, &tidRaw, &pidRaw, &stateRaw, &argSetRaw); err != nil {
			return prepared, candidateStarts, semanticCohorts, rowsRead, err
		}

		state, stateTextOK := traceDBBlockedStateText(stateRaw)
		argSetPresent := argSetRaw != nil
		argSetID, argSetOK := traceDBStrictSQLiteInt(argSetRaw)
		argSetOK = argSetOK && argSetID >= 0 && argSetID <= maxTraceDBInternalID
		args, argsFound := argsets[argSetID]
		_, splitState := traceDBBlockedStateIOWait(state)
		unsplitState := traceDBBlockedStateUnsplit(state)
		nearReservedState := traceDBBlockedStateNearReservedToken(state)
		if !splitState && !nearReservedState && !(unsplitState && argSetPresent) && !(argSetOK && argsFound && args.HasRelevant) {
			continue
		}
		rowsRead++
		if cohortITID, ok := traceDBStrictInternalID(itidRaw); ok {
			if cohortTS, ok := traceDBStrictSQLiteInt(tsRaw); ok && cohortTS >= 0 {
				semanticCohorts[traceDBBlockedBoundaryKey{ITID: cohortITID, StateStart: cohortTS}]++
			}
		}

		stableID, stableOK := traceDBBlockedDecodeStableID(stable, idRaw)
		ts, tsOK := traceDBStrictSQLiteInt(tsRaw)
		itid, itidOK := traceDBStrictInternalID(itidRaw)
		tid, tidOK := traceDBStrictSQLiteInt(tidRaw)
		pid, pidOK := traceDBStrictSQLiteInt(pidRaw)
		dur := int64(0)
		durOK := durRaw == nil
		if durRaw != nil {
			dur, durOK = traceDBStrictSQLiteInt(durRaw)
			durOK = durOK && dur >= 0
		}
		if !stableOK || !tsOK || ts < 0 || !durOK || !itidOK ||
			!tidOK || tid <= 0 || tid > math.MaxInt32 || !pidOK || pid <= 0 || pid > math.MaxInt32 ||
			!stateTextOK || nearReservedState || (argSetPresent && !argSetOK) {
			skipped["invalid_thread_state_metadata"]++
			continue
		}
		if duplicateStateIDs[stableID] {
			skipped["duplicate_source_id"]++
			continue
		}
		row := traceDBBlockedStateRow{
			ID: stableID, TS: ts, Dur: dur, ITID: itid,
			TID: tid, PID: pid, State: state, ArgSetID: argSetID,
		}
		if durRaw != nil && row.TS > math.MaxInt64-row.Dur {
			skipped["thread_state_end_overflow"]++
			continue
		}
		if traceDBBlockedStateUnsplit(row.State) {
			// CpuFilter::InsertBlockedReasonEvent rewrites pending D/DK rows to
			// D-IO/D-NIO or DK-IO/DK-NIO. An arg-bearing row that remains exactly
			// D/DK violates that upstream invariant, so do not guess its subtype.
			skipped["unsplit_blocked_state"]++
			continue
		}
		if !argSetPresent {
			skipped["missing_argset_rows"]++
			continue
		}
		expectedIOWait, stateOK := traceDBBlockedStateIOWait(row.State)
		if !argsFound || !args.HasRelevant {
			skipped["missing_argset_rows"]++
			continue
		}
		if args.Invalid {
			skipped["invalid_blocked_argset"]++
			continue
		}
		ioWait, reason := traceDBBlockedIOWait(args)
		if reason != "" {
			skipped[reason]++
			continue
		}
		if stateOK && ioWait != expectedIOWait {
			skipped["iowait_state_mismatch"]++
			continue
		}
		thread, process, resolution := authority.resolveThreadSubject(row.ITID)
		if resolution != traceDBSchedulerThreadResolved {
			if row.ITID == 0 {
				skipped["idle_blocked_candidate_forbidden"]++
			} else if resolution == traceDBSchedulerThreadMissing {
				skipped["missing_thread_identity"]++
			} else if resolution == traceDBSchedulerProcessMissing {
				skipped["missing_process_identity"]++
			} else {
				skipped["invalid_or_ambiguous_thread_process_identity"]++
			}
			continue
		}
		if thread.TID != row.TID {
			skipped["thread_tid_mismatch"]++
			continue
		}
		if process.PID <= 0 {
			skipped["missing_process_identity"]++
			continue
		}
		if process.PID != row.PID {
			skipped["thread_tgid_mismatch"]++
			continue
		}
		if !authority.threadPointAllows(row.ITID, row.TS) {
			skipped["lifecycle_rejected_thread_state_candidate"]++
			continue
		}
		if argPID, present, valid := traceDBBlockedOptionalInt(args, "pid"); present {
			if !valid || argPID != row.TID {
				skipped["arg_pid_mismatch"]++
				continue
			}
		}
		caller, callerRaw, callerQuality, reason := traceDBBlockedCaller(args)
		if reason != "" {
			skipped[reason]++
			continue
		}
		delay, delayPresent, delayValid := traceDBBlockedOptionalInt(args, "delay")
		if delayPresent && (!delayValid || delay < 0 || delay > maxTraceDBInternalID) {
			skipped["invalid_delay_arg"]++
			continue
		}
		prepared = append(prepared, traceDBPreparedBlockedReason{
			Row: row, Thread: thread, Process: process, IOWait: ioWait,
			Caller: caller, CallerRaw: callerRaw, CallerQuality: callerQuality,
			Delay: delay, DelayPresent: delayPresent,
		})
		starts := candidateStarts[row.ITID]
		if starts == nil {
			starts = map[int64]bool{}
			candidateStarts[row.ITID] = starts
		}
		starts[row.TS] = true
	}
	if err := rows.Err(); err != nil {
		return prepared, candidateStarts, semanticCohorts, rowsRead, err
	}
	return prepared, candidateStarts, semanticCohorts, rowsRead, nil
}

func traceDBBlockedArgsetsFromShared(shared traceDBArgsetIndex) map[int64]traceDBBlockedArgset {
	out := map[int64]traceDBBlockedArgset{}
	keys := []string{"iowait", "io_wait", "caller", "caller_str", "caller_raw", "delay", "pid"}
	for argSetID := range shared.Present {
		item := traceDBBlockedArgset{Invalid: shared.Invalid[argSetID]}
		for _, key := range keys {
			if shared.InvalidKeys[argSetID][key] {
				item.HasRelevant = true
				item.Invalid = true
			}
			value, present := shared.Sets[argSetID][key]
			if !present {
				continue
			}
			item.HasRelevant = true
			if item.Values == nil {
				item.Values = map[string]traceDBBlockedArg{}
			}
			arg := traceDBBlockedArg{DataType: value.Datatype, Text: value.Text, Valid: value.Valid}
			if value.Datatype == traceDBArgTypeInt {
				parsed, err := strconv.ParseInt(value.Text, 10, 64)
				if err != nil {
					item.Invalid = true
					arg.Valid = false
				} else {
					arg.Int = parsed
				}
			}
			item.Values[key] = arg
		}
		if item.HasRelevant {
			out[argSetID] = item
		}
	}
	return out
}

func loadTraceDBBlockedSchedBoundaries(ctx context.Context, tdb *traceDB, authority traceDBSchedulerAuthority, candidateStarts map[int64]map[int64]bool) (traceDBBlockedBoundaryIndex, error) {
	out := traceDBBlockedBoundaryIndex{
		Matches:           map[traceDBBlockedBoundaryKey][]traceDBBlockedSchedSlice{},
		LifecycleRejected: map[traceDBBlockedBoundaryKey]bool{},
		OverflowITIDs:     map[int64]bool{},
		TaintedITIDs:      map[int64]bool{},
		LowerBoundByITID:  map[int64]int64{},
		GlobalBarriers:    map[int64]bool{},
	}
	if len(candidateStarts) == 0 {
		return out, nil
	}
	// Stream sched_slice exactly once and retain only exact candidate boundaries.
	// Production traces can have millions of slices; copying that table merely
	// to recover a small blocked-reason header set would make conversion memory
	// scale with the trace instead of with the recovered evidence.
	rows, err := tdb.db.QueryContext(ctx, `SELECT itid, ts, dur, cpu FROM sched_slice`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		var itidRaw, tsRaw, durRaw, cpuRaw any
		if err := rows.Scan(&itidRaw, &tsRaw, &durRaw, &cpuRaw); err != nil {
			return out, err
		}
		itid, itidOK := traceDBStrictInternalID(itidRaw)
		ts, tsOK := traceDBStrictSQLiteInt(tsRaw)
		tsOK = tsOK && ts >= 0
		dur, durOK := traceDBStrictSQLiteInt(durRaw)
		durOK = durOK && dur >= 0

		if !itidOK {
			switch {
			case !tsOK:
				out.GlobalTaint = true
			case durRaw == nil || !durOK:
				traceDBBlockedSetGlobalLowerBound(&out, ts)
			case ts <= math.MaxInt64-dur:
				end := ts + dur
				for _, starts := range candidateStarts {
					if starts[end] {
						out.GlobalBarriers[end] = true
						break
					}
				}
			}
			continue
		}
		starts, relevantITID := candidateStarts[itid]
		if !relevantITID {
			continue
		}
		if !tsOK {
			out.TaintedITIDs[itid] = true
			continue
		}
		if durRaw == nil || !durOK {
			traceDBBlockedSetITIDLowerBound(&out, itid, ts)
			continue
		}
		if ts > math.MaxInt64-dur {
			out.OverflowITIDs[itid] = true
			continue
		}
		end := ts + dur
		if !starts[end] {
			continue
		}
		key := traceDBBlockedBoundaryKey{ITID: itid, StateStart: end}
		if !authority.threadClosedEndpointAllows(itid, ts, end) {
			out.LifecycleRejected[key] = true
			continue
		}
		item := traceDBBlockedSchedSlice{TS: ts, Dur: dur, CPU: -1}
		if cpu, ok := traceDBStrictSQLiteInt(cpuRaw); ok {
			item.CPU = cpu
		}
		out.Matches[key] = append(out.Matches[key], item)
	}
	return out, rows.Err()
}

func traceDBBlockedSetITIDLowerBound(index *traceDBBlockedBoundaryIndex, itid, ts int64) {
	if current, ok := index.LowerBoundByITID[itid]; !ok || ts < current {
		index.LowerBoundByITID[itid] = ts
	}
}

func traceDBBlockedSetGlobalLowerBound(index *traceDBBlockedBoundaryIndex, ts int64) {
	if !index.GlobalLowerBoundKnown || ts < index.GlobalLowerBound {
		index.GlobalLowerBound = ts
		index.GlobalLowerBoundKnown = true
	}
}

func traceDBBlockedBoundaryIntegrityReason(index traceDBBlockedBoundaryIndex, key traceDBBlockedBoundaryKey) string {
	if index.GlobalTaint {
		return "global_prev_sched_slice_taint"
	}
	if index.TaintedITIDs[key.ITID] {
		return "tainted_prev_sched_slice_lane"
	}
	if index.GlobalBarriers[key.StateStart] {
		return "global_prev_sched_slice_barrier"
	}
	if index.GlobalLowerBoundKnown && key.StateStart >= index.GlobalLowerBound {
		return "global_prev_sched_slice_lower_bound"
	}
	if lower, ok := index.LowerBoundByITID[key.ITID]; ok && key.StateStart >= lower {
		return "prev_sched_slice_lower_bound"
	}
	// Preserve the pre-lifecycle physical-source diagnosis when both fail.
	// LifecycleRejected remains a monotonic cohort barrier, but it must not
	// hide a more fundamental malformed/unknown sched_slice lane.
	if index.LifecycleRejected[key] {
		return "lifecycle_rejected_prev_sched_slice_boundary"
	}
	return ""
}

func traceDBBlockedHeaderCPU(matches []traceDBBlockedSchedSlice, overflow bool) (int64, string) {
	if len(matches) == 0 {
		if overflow {
			return 0, "sched_slice_boundary_overflow"
		}
		return 0, "missing_prev_sched_slice_boundary"
	}
	if len(matches) != 1 {
		return 0, "ambiguous_prev_sched_slice_boundary"
	}
	headerCPU := matches[0].CPU
	if !validTraceDBCPUIndex(headerCPU) {
		return 0, "invalid_prev_sched_slice_cpu"
	}
	return headerCPU, ""
}

func traceDBBlockedRelevantArgKey(key string) bool {
	switch key {
	case "iowait", "io_wait", "caller", "caller_str", "caller_raw", "delay", "pid":
		return true
	default:
		return false
	}
}

func traceDBBlockedStateText(raw any) (string, bool) {
	state, ok := traceDBStrictArgText(raw, false)
	return state, ok && len(state) <= 64 && state == strings.TrimSpace(state)
}

func traceDBBlockedStateNearReservedToken(state string) bool {
	for _, reserved := range []string{"S", "D", "DK", "D-IO", "D-NIO", "DK-IO", "DK-NIO"} {
		if state != reserved && strings.EqualFold(strings.TrimSpace(state), reserved) {
			return true
		}
	}
	return false
}

func traceDBBlockedStateIOWait(state string) (int64, bool) {
	switch state {
	case "D-IO", "DK-IO":
		return 1, true
	case "D-NIO", "DK-NIO":
		return 0, true
	default:
		return 0, false
	}
}

func traceDBBlockedStateUnsplit(state string) bool {
	switch state {
	case "D", "DK":
		return true
	default:
		return false
	}
}

func traceDBBlockedIOWait(args traceDBBlockedArgset) (int64, string) {
	canonical, canonicalPresent, canonicalValid := traceDBBlockedOptionalInt(args, "iowait")
	compat, compatPresent, compatValid := traceDBBlockedOptionalInt(args, "io_wait")
	if !canonicalPresent && !compatPresent {
		return 0, "missing_iowait_arg"
	}
	if canonicalPresent && (!canonicalValid || canonical < 0 || canonical > 1) {
		return 0, "invalid_iowait_arg"
	}
	if compatPresent && (!compatValid || compat < 0 || compat > 1) {
		return 0, "invalid_iowait_arg"
	}
	if canonicalPresent && compatPresent && canonical != compat {
		return 0, "conflicting_iowait_aliases"
	}
	if canonicalPresent {
		return canonical, ""
	}
	return compat, ""
}

func traceDBBlockedOptionalInt(args traceDBBlockedArgset, key string) (int64, bool, bool) {
	arg, present := args.Values[key]
	if !present {
		return 0, false, false
	}
	return arg.Int, true, arg.Valid && arg.DataType == traceDBArgTypeInt
}

func traceDBBlockedCaller(args traceDBBlockedArgset) (caller, callerRaw, quality, reason string) {
	callerArg, callerPresent := args.Values["caller"]
	callerStrArg, callerStrPresent := args.Values["caller_str"]
	callerRawArg, callerRawPresent := args.Values["caller_raw"]
	if !callerPresent && !callerStrPresent && !callerRawPresent {
		return "", "", "", "missing_caller_arg"
	}
	if callerPresent && (!callerArg.Valid || (callerArg.DataType != traceDBArgTypeString && callerArg.DataType != traceDBArgTypeInt)) {
		return "", "", "", "invalid_caller_arg"
	}
	if callerStrPresent && (!callerStrArg.Valid || callerStrArg.DataType != traceDBArgTypeString) {
		return "", "", "", "invalid_caller_arg"
	}
	if callerRawPresent && (!callerRawArg.Valid || (callerRawArg.DataType != traceDBArgTypeString && callerRawArg.DataType != traceDBArgTypeInt)) {
		return "", "", "", "invalid_caller_arg"
	}

	if callerRawPresent {
		callerRaw, _ = traceDBBlockedRawCaller(callerRawArg)
		if callerRaw == "" {
			return "", "", "", "invalid_caller_raw_arg"
		}
	}
	if callerRaw == "" && callerPresent {
		callerRaw, _ = traceDBBlockedRawCaller(callerArg)
	}
	if callerRaw == "" && callerStrPresent {
		callerRaw, _ = traceDBBlockedRawCaller(callerStrArg)
	}

	symbolAuthorityPresent := callerStrPresent
	if callerStrPresent {
		if _, raw := normalizeTraceDBBlockedRawString(callerStrArg.Text); !raw {
			if symbol, safe := safeProfilerBlockedCaller(callerStrArg.Text); safe {
				return symbol, callerRaw, "symbolized", ""
			}
		}
	}
	if !symbolAuthorityPresent && callerPresent && callerArg.DataType == traceDBArgTypeString {
		if _, raw := normalizeTraceDBBlockedRawString(callerArg.Text); !raw {
			if symbol, safe := safeProfilerBlockedCaller(callerArg.Text); safe {
				return symbol, callerRaw, "symbolized", ""
			}
		}
	}
	return "unknown", callerRaw, "opaque", ""
}

func traceDBBlockedRawCaller(arg traceDBBlockedArg) (string, bool) {
	if !arg.Valid {
		return "", false
	}
	if arg.DataType == traceDBArgTypeInt {
		// SQLite INTEGER is signed, while kernel caller addresses are uint64.
		// Preserve the two's-complement bit pattern for compatibility with DBs
		// that stored an upstream unsigned address through the INTEGER lane.
		return fmt.Sprintf("0x%x", uint64(arg.Int)), true
	}
	if arg.DataType == traceDBArgTypeString {
		return normalizeTraceDBBlockedRawString(arg.Text)
	}
	return "", false
}

func normalizeTraceDBBlockedRawString(raw string) (string, bool) {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) < 3 || len(raw) > 18 ||
		!strings.HasPrefix(raw, "0x") {
		return "", false
	}
	value, err := strconv.ParseUint(raw[2:], 16, 64)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("0x%x", value), true
}

func traceDBBlockedMergeSchemaCoverage(out *TraceDBCoverage, prefix string, item TraceDBCoverage) {
	for _, column := range item.ColumnsPresent {
		out.ColumnsPresent = append(out.ColumnsPresent, prefix+"."+column)
	}
	for _, column := range item.ColumnsMissing {
		out.ColumnsMissing = append(out.ColumnsMissing, prefix+"."+column)
	}
	sort.Strings(out.ColumnsPresent)
	sort.Strings(out.ColumnsMissing)
}

func traceDBBlockedSchemaSkip(items ...TraceDBCoverage) string {
	var reasons []string
	for _, item := range items {
		if !item.Found {
			reasons = append(reasons, "missing table "+item.Table)
			continue
		}
		if len(item.ColumnsMissing) > 0 {
			reasons = append(reasons, "missing "+item.Table+" columns "+strings.Join(item.ColumnsMissing, ","))
		}
	}
	if len(reasons) == 0 {
		return "blocked reason schema unavailable"
	}
	return strings.Join(reasons, "; ")
}

func traceDBBlockedSkipSummary(skipped map[string]int) string {
	if len(skipped) == 0 {
		return ""
	}
	keys := make([]string, 0, len(skipped))
	total := 0
	for key, count := range skipped {
		if count <= 0 {
			continue
		}
		keys = append(keys, key)
		total += count
	}
	if total == 0 {
		return ""
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, skipped[key]))
	}
	return fmt.Sprintf("%d blocked reason row(s) skipped: %s", total, strings.Join(parts, ","))
}
