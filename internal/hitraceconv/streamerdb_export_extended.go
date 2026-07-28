package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func exportTraceDBExtendedFamilies(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, authority traceDBSchedulerAuthority, syncSpans *traceDBSyncSpanAuthority) ([]TraceDBCoverage, error) {
	var coverage []TraceDBCoverage
	if !authority.initialized || syncSpans == nil {
		return coverage, fmt.Errorf("extended export requires the shared scheduler authority")
	}
	index := authority.identities
	running, runningIntegrity, runningCoverage, err := tdb.loadExtendedLegacyRunningIntervals(ctx, index)
	coverage = append(coverage, runningCoverage)
	if err != nil {
		return coverage, err
	}
	lifecycleRunning := newTraceDBSchedulerRunningIndex(authority, running, runningIntegrity, nil)
	dict, dictCoverage, err := tdb.loadDataDict(ctx)
	coverage = append(coverage, dictCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart := time.Now()
	perfCoverage, err := exportTraceDBPerfSamples(ctx, tdb, sink, authority, lifecycleRunning)
	traceDBSetCoverageListElapsed(perfCoverage, stageStart)
	coverage = append(coverage, perfCoverage...)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	rawCoverage, err := exportTraceDBRawFtraceFamilies(ctx, tdb, sink, authority, lifecycleRunning, syncSpans.artifactSource)
	traceDBSetCoverageListElapsed(rawCoverage, stageStart)
	coverage = append(coverage, rawCoverage...)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	rawDMAWaitCoverage, err := publishTraceDBRawDMAWaitRecovery(
		ctx, tdb.sourceNameInventory, sink, authority, rawCoverage)
	traceDBSetCoverageElapsed(&rawDMAWaitCoverage, stageStart)
	coverage = append(coverage, rawDMAWaitCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	measureCoverage, err := exportTraceDBMeasureFamilies(ctx, tdb, sink)
	traceDBSetCoverageListElapsed(measureCoverage, stageStart)
	coverage = append(coverage, measureCoverage...)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	callstackCoverage, err := exportTraceDBCallstack(ctx, tdb, sink, authority, lifecycleRunning, syncSpans)
	traceDBSetCoverageElapsed(&callstackCoverage, stageStart)
	coverage = append(coverage, callstackCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	frameCoverage, emittedFrames, err := exportTraceDBFrameSliceWithRows(ctx, tdb, sink, authority, lifecycleRunning)
	traceDBSetCoverageElapsed(&frameCoverage, stageStart)
	coverage = append(coverage, frameCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	frameMapCoverage, err := exportTraceDBFrameMaps(ctx, tdb, sink, authority, emittedFrames)
	traceDBSetCoverageElapsed(&frameMapCoverage, stageStart)
	coverage = append(coverage, frameMapCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	dmaCoverage, err := exportTraceDBDMAFence(ctx, tdb, sink, index, running, dict)
	traceDBSetCoverageElapsed(&dmaCoverage, stageStart)
	coverage = append(coverage, dmaCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	syscallCoverage, err := exportTraceDBSyscall(ctx, tdb, sink, authority, lifecycleRunning, syncSpans)
	traceDBSetCoverageElapsed(&syscallCoverage, stageStart)
	coverage = append(coverage, syscallCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	taskPoolCoverage, err := exportTraceDBTaskPool(ctx, tdb, sink, index, lifecycleRunning)
	traceDBSetCoverageElapsed(&taskPoolCoverage, stageStart)
	coverage = append(coverage, taskPoolCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	startupCoverage, err := exportTraceDBAppStartup(ctx, tdb, sink, syncSpans, index, dict)
	traceDBSetCoverageElapsed(&startupCoverage, stageStart)
	coverage = append(coverage, startupCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	staticCoverage, err := exportTraceDBStaticInitialize(ctx, tdb, sink, syncSpans, index)
	traceDBSetCoverageElapsed(&staticCoverage, stageStart)
	coverage = append(coverage, staticCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	rawMarkerCoverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, tdb.sourceNameInventory, authority, syncSpans)
	traceDBSetCoverageElapsed(&rawMarkerCoverage, stageStart)
	coverage = append(coverage, rawMarkerCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	nativeCoverage, err := exportTraceDBNativeHook(ctx, tdb, sink, authority, lifecycleRunning)
	traceDBSetCoverageElapsed(&nativeCoverage, stageStart)
	coverage = append(coverage, nativeCoverage)
	if err != nil {
		return coverage, err
	}
	postSyncExporters := []func(context.Context, *traceDB, *traceDBRowSink, traceDBThreadIndex, map[int64][]traceDBRunningInterval, map[int64]string) (TraceDBCoverage, error){
		exportTraceDBProcessMeasures,
		exportTraceDBNetwork,
		exportTraceDBDiskIO,
		exportTraceDBCPUUsage,
		exportTraceDBLiveProcess,
		exportTraceDBXPower,
		exportTraceDBLog,
		exportTraceDBHiSysEvent,
	}
	for _, exporter := range postSyncExporters {
		stageStart = time.Now()
		item, err := exporter(ctx, tdb, sink, index, running, dict)
		traceDBSetCoverageElapsed(&item, stageStart)
		coverage = append(coverage, item)
		if err != nil {
			return coverage, err
		}
	}
	return coverage, nil
}

func (tdb *traceDB) loadDataDict(ctx context.Context) (map[int64]string, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver", "data_dict", []string{"id", "data"})
	out := map[int64]string{}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT id, COALESCE(data, '') FROM data_dict WHERE id IS NOT NULL")
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		out[id] = data
	}
	return out, coverage, rows.Err()
}

type traceDBSyscallRow struct {
	StableID    int64
	TS          int64
	Dur         int64
	End         int64
	Number      int64
	EmitterITID int64
	OwnerIPID   int64
	Task        string
	TID         int64
	TGID        int64
	StartCPU    int64
	EndCPU      int64
}

func exportTraceDBSyscall(ctx context.Context, tdb *traceDB, _ *traceDBRowSink,
	authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex, syncSpans *traceDBSyncSpanAuthority,
) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "syscall", []string{"ts", "dur", "syscall_number", "itid"})
	coverage.FieldSources = map[string]string{
		"wire_laminar":      "strict accepted rows submit typed B/E candidates to the single shared authority; rejected rows declare exact-lane or global source poison; no endpoint is published by this exporter",
		"source_admission":  "full physical-row scan with typeof-pinned bounded INTEGER transport; invalid storage classes remain rejected, with no SQL filtering, coercion, defaulting or per-row identity fallback",
		"internal_identity": "syscall.itid uses the fixed current signed-int32 producer profile; exact positive public TID/PID and canonical owner mapping are required",
		"syscall_number":    "current syscall_number uint32 is exposed through a signed-int32 SQLite projection; -1 sentinel and positive high-half encodings are rejected",
		"lifecycle":         "zero duration uses exact thread/process point admission; positive duration uses closed thread and positive-process generation admission at checked endpoints",
		"cpu":               "start and exact end independently require lifecycle-filtered typed Running witnesses; CPU 0 and CPU 4095 are valid values, never unknown defaults",
	}
	fail := func(err error) (TraceDBCoverage, error) {
		coverage.Error = err.Error()
		return coverage, err
	}
	if err != nil || !coverage.Found {
		return coverage, err
	}
	if syncSpans == nil || !authority.initialized || !running.initialized {
		return fail(&traceDBOutputInvariantError{Reason: "missing_syscall_source_authority"})
	}
	profile, profileSource, err := traceDBActivityProfile(ctx, tdb.db, "syscall")
	if err != nil {
		return fail(err)
	}
	if profile != traceDBActivityITIDSignedInt32 {
		return fail(&traceDBOutputInvariantError{Reason: "unsupported_syscall_identity_profile"})
	}
	coverage.FieldSources["internal_identity"] = profileSource + "; exact positive public TID/PID and canonical owner mapping are required"
	poisonRejected := func(itidRaw any) error {
		itid, itidOK := profile.decode(itidRaw)
		if itidOK && itid > 0 && !authority.identities.AmbiguousITID[itid] {
			thread, found := authority.identities.ByITID[itid]
			if found && thread.ITID == itid && thread.TID > 0 && thread.TID <= math.MaxInt32 {
				return syncSpans.poisonExactLane(ctx, traceDBSyncSpanLanePoison{
					Producer:           traceDBSyncSpanProducerSyscall,
					HeaderTID:          thread.TID,
					CanonicalITID:      itid,
					CanonicalITIDKnown: true,
					Reason:             traceDBSyncSpanLanePoisonRejectedSyscallCandidate,
				})
			}
		}
		return syncSpans.poisonGlobally(ctx, traceDBSyncSpanGlobalPoison{
			Producer: traceDBSyncSpanProducerSyscall,
			Reason:   traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate,
		})
	}
	if len(coverage.ColumnsMissing) > 0 {
		if coverage.RowsRead == 0 {
			return coverage, nil
		}
		hasITID := false
		for _, column := range coverage.ColumnsPresent {
			hasITID = hasITID || column == "itid"
		}
		if !hasITID {
			if err := poisonRejected(nil); err != nil {
				return fail(err)
			}
			traceDBAppendCoverageSkipped(&coverage, "nonempty syscall schema cannot localize rejected rows; global sync source fail-close declared")
			return coverage, nil
		}
		rows, queryErr := tdb.db.QueryContext(ctx, `
			SELECT typeof(itid), CASE WHEN typeof(itid) = 'integer' THEN itid END
			FROM syscall
		`)
		if queryErr != nil {
			return fail(queryErr)
		}
		defer rows.Close()
		scanned := 0
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
			var itidTypeRaw, itidValueRaw any
			if err := rows.Scan(&itidTypeRaw, &itidValueRaw); err != nil {
				return fail(err)
			}
			scanned++
			itidRaw := traceDBBoundedSQLiteIntegerTransport(itidTypeRaw, itidValueRaw)
			if err := poisonRejected(itidRaw); err != nil {
				return fail(err)
			}
		}
		if err := rows.Err(); err != nil {
			return fail(err)
		}
		if scanned != coverage.RowsRead {
			return fail(&traceDBOutputInvariantError{Reason: "syscall_source_scan_count_mismatch"})
		}
		traceDBAppendCoverageSkipped(&coverage,
			fmt.Sprintf("nonempty syscall schema rejected_rows=%d; shared sync poison declared", scanned))
		return coverage, nil
	}
	stableExpr, stableKnown, err := traceDBSyncSpanHiddenRowID(ctx, tdb, &coverage)
	if err != nil {
		return coverage, err
	}
	if !stableKnown {
		if coverage.RowsRead == 0 {
			return coverage, nil
		}
		stableExpr = "NULL"
	}
	query := fmt.Sprintf(`
		SELECT %s,
		       typeof(ts), CASE WHEN typeof(ts) = 'integer' THEN ts END,
		       typeof(dur), CASE WHEN typeof(dur) = 'integer' THEN dur END,
		       typeof(syscall_number), CASE WHEN typeof(syscall_number) = 'integer' THEN syscall_number END,
		       typeof(itid), CASE WHEN typeof(itid) = 'integer' THEN itid END
		FROM syscall
	`, stableExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	skipped := map[string]int{}
	scanned := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var stableRaw, tsTypeRaw, tsValueRaw, durTypeRaw, durValueRaw any
		var numberTypeRaw, numberValueRaw, itidTypeRaw, itidValueRaw any
		if err := rows.Scan(&stableRaw, &tsTypeRaw, &tsValueRaw, &durTypeRaw, &durValueRaw,
			&numberTypeRaw, &numberValueRaw, &itidTypeRaw, &itidValueRaw); err != nil {
			return fail(err)
		}
		scanned++
		tsRaw := traceDBBoundedSQLiteIntegerTransport(tsTypeRaw, tsValueRaw)
		durRaw := traceDBBoundedSQLiteIntegerTransport(durTypeRaw, durValueRaw)
		numberRaw := traceDBBoundedSQLiteIntegerTransport(numberTypeRaw, numberValueRaw)
		itidRaw := traceDBBoundedSQLiteIntegerTransport(itidTypeRaw, itidValueRaw)
		row, reason := prepareTraceDBSyscallRow(authority, running, profile,
			stableRaw, tsRaw, durRaw, numberRaw, itidRaw)
		if reason != "" {
			if err := poisonRejected(itidRaw); err != nil {
				return fail(err)
			}
			skipped[reason]++
			continue
		}
		if err := syncSpans.submit(ctx, traceDBSyncSpanCandidate{
			Producer:           traceDBSyncSpanProducerSyscall,
			StableKind:         traceDBSyncSpanStableSyscallRowID,
			StableID:           row.StableID,
			HeaderTID:          row.TID,
			HeaderTGID:         row.TGID,
			CanonicalITID:      row.EmitterITID,
			CanonicalITIDKnown: true,
			OwnerIPID:          row.OwnerIPID,
			OwnerIPIDKnown:     true,
			Start:              row.TS,
			End:                row.End,
			StartCPU:           row.StartCPU,
			EndCPU:             row.EndCPU,
			StartCPUProvenance: traceDBSyncSpanCPUSyscallTypedRunning,
			EndCPUProvenance:   traceDBSyncSpanCPUSyscallTypedRunning,
			Task:               row.Task,
			Name:               "sys_" + strconv.FormatInt(row.Number, 10),
			NameProvenance:     traceDBSyncSpanNameSyscallNumber,
			DepthProvenance:    traceDBSyncSpanDepthUnknown,
		}); err != nil {
			return fail(err)
		}
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	if scanned != coverage.RowsRead {
		return fail(&traceDBOutputInvariantError{Reason: "syscall_source_scan_count_mismatch"})
	}
	traceDBAppendCoverageSkipped(&coverage, traceDBCountSummary(skipped))
	return coverage, nil
}

func prepareTraceDBSyscallRow(authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex,
	profile traceDBActivityITIDProfile, stableRaw, tsRaw, durRaw, numberRaw, itidRaw any,
) (traceDBSyscallRow, string) {
	var row traceDBSyscallRow
	var ok bool
	row.EmitterITID, ok = profile.decode(itidRaw)
	if !ok || row.EmitterITID <= 0 {
		return row, "invalid_emitter_itid"
	}
	thread, process, resolution := authority.resolveThreadSubject(row.EmitterITID)
	if resolution != traceDBSchedulerThreadResolved || thread.TID <= 0 || thread.TID > math.MaxInt32 ||
		thread.IPID < 0 || process.IPID != thread.IPID || process.PID <= 0 || process.PID > math.MaxInt32 {
		return row, "unresolved_emitter_identity"
	}
	row.OwnerIPID = thread.IPID
	row.TID = thread.TID
	row.TGID = process.PID
	row.Task = traceDBCommName(thread.Name, "unknown")
	row.StableID, ok = traceDBStrictSQLiteInt(stableRaw)
	if !ok {
		return row, "invalid_hidden_rowid"
	}
	row.TS, ok = traceDBStrictSQLiteInt(tsRaw)
	if !ok || row.TS < 0 {
		return row, "invalid_timestamp"
	}
	row.Dur, ok = traceDBStrictSQLiteInt(durRaw)
	if !ok || row.Dur < 0 {
		return row, "invalid_duration"
	}
	// Current Trace Streamer stores syscall_number as uint32_t and exposes it
	// with sqlite3_result_int, exactly the same signed-int32 projection selected
	// for this producer's ITID. In particular, -1 remains the UINT32_MAX
	// sentinel while -2 is the canonical uint32 value 4294967294.
	row.Number, ok = profile.decode(numberRaw)
	if !ok {
		return row, "invalid_syscall_number"
	}
	if row.TS > math.MaxInt64-row.Dur {
		return row, "timestamp_overflow"
	}
	row.End = row.TS + row.Dur
	if traceDBBeforeCaptureStart(authority.identities, row.TS) {
		return row, "before_capture_start"
	}
	if row.Dur == 0 {
		if !authority.threadPointAllows(row.EmitterITID, row.TS) {
			return row, "lifecycle_rejected_sync_point"
		}
	} else if !authority.threadClosedEndpointAllows(row.EmitterITID, row.TS, row.End) ||
		!authority.processClosedEndpointAllows(row.OwnerIPID, row.TS, row.End) {
		return row, "lifecycle_rejected_sync_closed_interval"
	}
	var status traceDBSchedulerRunningLookupStatus
	row.StartCPU, status = running.lookupCPUAt(row.EmitterITID, row.TS)
	if reason := traceDBSyscallRunningRejection(status, "start"); reason != "" {
		return row, reason
	}
	row.EndCPU, status = running.lookupCPUAt(row.EmitterITID, row.End)
	if reason := traceDBSyscallRunningRejection(status, "end"); reason != "" {
		return row, reason
	}
	return row, ""
}

func traceDBSyscallRunningRejection(status traceDBSchedulerRunningLookupStatus, endpoint string) string {
	switch status {
	case traceDBSchedulerRunningKnown:
		return ""
	case traceDBSchedulerRunningSourceTainted:
		return "tainted_running_cpu_witness"
	case traceDBSchedulerRunningLifecycleRejected:
		return "lifecycle_rejected_running_cpu_witness"
	default:
		return "unknown_" + endpoint + "_cpu"
	}
}

func exportTraceDBTaskPool(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	index traceDBThreadIndex, running traceDBSchedulerRunningIndex,
) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "task_pool", []string{"task_id", "allocation_task_row", "execute_task_row", "allocation_itid", "execute_itid"})
	coverage.FieldSources = map[string]string{
		"pairing":  "one task_pool row publishes S/F only when both linked callstack endpoints, identities, owner and checked interval are complete; allocation-only rows never mint an open S endpoint",
		"identity": "strict canonical allocation/execute ITIDs independently preserve both physical emitters; the exact positive allocation TGID is the one logical async payload owner on both endpoints, including cross-process execution",
		"interval": "strict non-negative allocation/execute timestamps plus non-negative execute duration and checked end addition; end must not precede allocation",
		"cpu":      "shared lifecycle-filtered typed Running witnesses at exact allocation and checked execute-end timestamps; CPU 0 is never an unknown fallback",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	callstackCoverage, err := tdb.inspectCoverage(ctx, "slice", "callstack", []string{"id", "ts", "dur"})
	if err != nil {
		return coverage, err
	}
	if !callstackCoverage.Found || len(callstackCoverage.ColumnsMissing) > 0 {
		coverage.Skipped = "missing callstack dependency"
		return coverage, nil
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT tp.task_id, ca.ts, ce.ts, COALESCE(ce.dur, 0),
		       tp.allocation_itid, tp.execute_itid
		FROM task_pool tp
		LEFT JOIN callstack ca ON ca.id = tp.allocation_task_row
		LEFT JOIN callstack ce ON ce.id = tp.execute_task_row
		WHERE ca.ts IS NOT NULL
		ORDER BY ca.ts
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var taskID any
		var allocTS int64
		var execTS sql.NullInt64
		var execDur int64
		var allocITIDRaw, execITIDRaw any
		if err := rows.Scan(&taskID, &allocTS, &execTS, &execDur, &allocITIDRaw, &execITIDRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		if allocTS < 0 {
			skipped["invalid_allocation_timestamp"]++
			continue
		}
		if !execTS.Valid {
			skipped["unpaired_allocation"]++
			continue
		}
		if execTS.Int64 < 0 {
			skipped["invalid_execute_timestamp"]++
			continue
		}
		if execDur < 0 {
			skipped["invalid_execute_duration"]++
			continue
		}
		if execTS.Int64 > math.MaxInt64-execDur {
			skipped["execute_end_overflow"]++
			continue
		}
		execEnd := execTS.Int64 + execDur
		if execEnd < allocTS {
			skipped["invalid_task_interval"]++
			continue
		}
		allocITID, allocITIDOK := traceDBStrictInternalID(allocITIDRaw)
		if !allocITIDOK {
			skipped["invalid_allocation_itid"]++
			continue
		}
		allocTask, allocTID, allocTGID, allocOK := traceDBResolvedThreadLineContext(index, allocITID)
		if !allocOK {
			skipped["unresolved_allocation_identity"]++
			continue
		}
		execITID, execITIDOK := traceDBStrictInternalID(execITIDRaw)
		if !execITIDOK {
			skipped["invalid_execute_itid"]++
			continue
		}
		execTask, execTID, execTGID, execITIDOK :=
			traceDBResolvedThreadLineContext(index, execITID)
		if !execITIDOK {
			skipped["unresolved_execute_identity"]++
			continue
		}
		allocCPU, runningStatus := running.lookupCPUAt(allocITID, allocTS)
		if reason := traceDBSyscallRunningRejection(runningStatus, "allocation"); reason != "" {
			skipped[reason]++
			continue
		}
		execCPU, runningStatus := running.lookupCPUAt(execITID, execEnd)
		if reason := traceDBSyscallRunningRejection(runningStatus, "execute_end"); reason != "" {
			skipped[reason]++
			continue
		}
		cookie := traceDBAnyText(taskID, "0")
		name := "TaskPool-" + cookie
		if err := addTraceDBAsyncSpanEndpointRows(sink, allocTS, execEnd,
			allocTask, allocTID, allocTGID, allocCPU,
			execTask, execTID, execTGID, execCPU,
			allocTGID, name, cookie); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, rows.Err()
}

func exportTraceDBAppStartup(ctx context.Context, tdb *traceDB, _ *traceDBRowSink, syncSpans *traceDBSyncSpanAuthority, index traceDBThreadIndex, dict map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "app_startup", []string{"start_time", "end_time", "start_name", "ipid"})
	coverage.FieldSources = map[string]string{
		"wire_laminar":     "current accepted rows submit typed B/E candidates to the shared authority; no endpoint is published by this exporter",
		"source_admission": "legacy SQL COALESCE/WHERE, scalar, process lifecycle, CPU and anti-rescue correctness remain open as R1b-C",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	stableExpr, stableKnown, err := traceDBSyncSpanHiddenRowID(ctx, tdb, &coverage)
	if err != nil || !stableKnown {
		return coverage, err
	}
	query := fmt.Sprintf("SELECT %s, start_time, end_time, COALESCE(start_name, 0), ipid FROM app_startup WHERE end_time > start_time ORDER BY start_time, %s", stableExpr, stableExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var start, end, nameID int64
		var stableRaw, ipidRaw any
		if err := rows.Scan(&stableRaw, &start, &end, &nameID, &ipidRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		ipid, ok := traceDBStrictInternalID(ipidRaw)
		if !ok {
			skipped["invalid_owner_ipid"]++
			continue
		}
		stableID, stableOK := traceDBStrictSQLiteInt(stableRaw)
		if !stableOK {
			return coverage, &traceDBOutputInvariantError{Reason: "invalid_app_startup_hidden_rowid"}
		}
		task, tid, tgid, ok := traceDBResolvedProcessLineContext(index, ipid, "startup")
		if !ok {
			skipped["unresolved_owner_process"]++
			continue
		}
		if err := syncSpans.submit(ctx, traceDBSyncSpanCandidate{
			Producer:           traceDBSyncSpanProducerAppStartup,
			StableKind:         traceDBSyncSpanStableAppStartupRowID,
			StableID:           stableID,
			HeaderTID:          tid,
			HeaderTGID:         tgid,
			OwnerIPID:          ipid,
			OwnerIPIDKnown:     true,
			Start:              start,
			End:                end,
			StartCPU:           0,
			EndCPU:             0,
			StartCPUProvenance: traceDBSyncSpanCPULegacyUnverified,
			EndCPUProvenance:   traceDBSyncSpanCPULegacyUnverified,
			Task:               task,
			Name:               "AppStartup:" + firstNonEmpty(dict[nameID], "startup"),
			NameProvenance:     traceDBSyncSpanNameAppStartupDictionary,
			DepthProvenance:    traceDBSyncSpanDepthUnknown,
		}); err != nil {
			return coverage, err
		}
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, rows.Err()
}

func exportTraceDBStaticInitialize(ctx context.Context, tdb *traceDB, _ *traceDBRowSink, syncSpans *traceDBSyncSpanAuthority, index traceDBThreadIndex) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "static_initalize", []string{"start_time", "end_time", "so_name", "ipid", "tid"})
	coverage.FieldSources = map[string]string{
		"wire_laminar":     "current accepted rows submit typed B/E candidates to the shared authority; no endpoint is published by this exporter",
		"source_admission": "legacy SQL WHERE/scalar, (ipid,tid)->ITID uniqueness, lifecycle, CPU and anti-rescue correctness remain open as R1b-C",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	stableExpr, stableKnown, err := traceDBSyncSpanHiddenRowID(ctx, tdb, &coverage)
	if err != nil || !stableKnown {
		return coverage, err
	}
	query := fmt.Sprintf("SELECT %s, start_time, end_time, so_name, ipid, tid FROM static_initalize WHERE end_time > start_time ORDER BY start_time, %s", stableExpr, stableExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var start, end int64
		var stableRaw, so, ipidRaw, tidRaw any
		if err := rows.Scan(&stableRaw, &start, &end, &so, &ipidRaw, &tidRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		ipid, ipidOK := traceDBStrictInternalID(ipidRaw)
		if !ipidOK {
			skipped["invalid_owner_ipid"]++
			continue
		}
		stableID, stableOK := traceDBStrictSQLiteInt(stableRaw)
		if !stableOK {
			return coverage, &traceDBOutputInvariantError{Reason: "invalid_static_initialize_hidden_rowid"}
		}
		tid, tidOK := traceDBStrictPublicID(tidRaw)
		if !tidOK || tid <= 0 {
			skipped["invalid_emitter_tid"]++
			continue
		}
		task, _, tgid, processOK := traceDBResolvedProcessLineContext(index, ipid, "soInit")
		if !processOK {
			skipped["unresolved_owner_process"]++
			continue
		}
		if err := syncSpans.submit(ctx, traceDBSyncSpanCandidate{
			Producer:           traceDBSyncSpanProducerStaticInitialize,
			StableKind:         traceDBSyncSpanStableStaticInitializeRowID,
			StableID:           stableID,
			HeaderTID:          tid,
			HeaderTGID:         tgid,
			OwnerIPID:          ipid,
			OwnerIPIDKnown:     true,
			Start:              start,
			End:                end,
			StartCPU:           0,
			EndCPU:             0,
			StartCPUProvenance: traceDBSyncSpanCPULegacyUnverified,
			EndCPUProvenance:   traceDBSyncSpanCPULegacyUnverified,
			Task:               task,
			Name:               "SoInit:" + traceDBAnyText(so, "None"),
			NameProvenance:     traceDBSyncSpanNameStaticObject,
			DepthProvenance:    traceDBSyncSpanDepthUnknown,
		}); err != nil {
			return coverage, err
		}
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, rows.Err()
}

func exportTraceDBProcessMeasures(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "counter", "process_measure", []string{"ts", "value", "filter_id"})
	coverage.SourceTables = []string{"process_measure", "process_measure_filter"}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	filterCoverage, err := tdb.inspectCoverage(ctx, "counter", "process_measure_filter", []string{"id", "name", "ipid"})
	if err != nil {
		return coverage, err
	}
	if !filterCoverage.Found || len(filterCoverage.ColumnsMissing) > 0 {
		coverage.Skipped = "missing process_measure_filter dependency"
		return coverage, nil
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT m.ts, m.value, f.name, f.ipid
		FROM process_measure m
		JOIN process_measure_filter f ON f.id = m.filter_id
		ORDER BY m.ts
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var ts int64
		var value, ipidRaw any
		var name string
		if err := rows.Scan(&ts, &value, &name, &ipidRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		ipid, ok := traceDBStrictInternalID(ipidRaw)
		if !ok {
			skipped["invalid_owner_ipid"]++
			continue
		}
		task, tid, tgid, ok := traceDBResolvedProcessLineContext(index, ipid, "unknown")
		if !ok {
			skipped["unresolved_owner_process"]++
			continue
		}
		if err := addTraceDBInstantRow(sink, ts, task, tid, tgid, 0, fmt.Sprintf("tracing_mark_write: C|%d|%s|%s", tgid, name, traceDBAnyText(value, "0"))); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, rows.Err()
}

func exportTraceDBNetwork(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	return exportTraceDBSimpleCounters(ctx, tdb, sink, "counter", "network", []string{"ts", "tx_speed", "rx_speed"}, "<network>", []traceDBCounterColumn{{"net_tx_speed", "tx_speed"}, {"net_rx_speed", "rx_speed"}})
}

func exportTraceDBDiskIO(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	return exportTraceDBSimpleCounters(ctx, tdb, sink, "counter", "diskio", []string{"ts", "rd_speed", "wr_speed"}, "<diskio>", []traceDBCounterColumn{{"disk_rd_speed", "rd_speed"}, {"disk_wr_speed", "wr_speed"}})
}

func exportTraceDBCPUUsage(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	return exportTraceDBSimpleCounters(ctx, tdb, sink, "counter", "cpu_usage", []string{"ts", "total_load", "user_load", "system_load"}, "<cpu_usage>", []traceDBCounterColumn{{"cpu_total_load", "total_load"}, {"cpu_user_load", "user_load"}, {"cpu_system_load", "system_load"}})
}

func exportTraceDBLiveProcess(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "counter", "live_process", []string{"ts", "process_id", "process_name", "cpu_usage", "pss_info", "thread_num"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT ts, COALESCE(process_id, 0), COALESCE(process_name, 'proc'), cpu_usage, pss_info, thread_num FROM live_process ORDER BY ts")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, pid int64
		var name string
		var cpu, pss, threads any
		if err := rows.Scan(&ts, &pid, &name, &cpu, &pss, &threads); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		for _, item := range []struct {
			name  string
			value any
		}{{"cpu_usage", cpu}, {"pss_kb", pss}, {"thread_num", threads}} {
			if err := addTraceDBInstantRow(sink, ts, name, pid, pid, 0, fmt.Sprintf("tracing_mark_write: C|%d|%s|%s", pid, item.name, traceDBAnyText(item.value, "0"))); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	return coverage, rows.Err()
}

func exportTraceDBXPower(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "counter", "xpower_measure", []string{"ts", "value", "filter_id"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT x.ts, x.value, COALESCE(f.name, 'unknown')
		FROM xpower_measure x
		LEFT JOIN measure_filter f ON f.id = x.filter_id
		ORDER BY x.ts
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts int64
		var value any
		var name string
		if err := rows.Scan(&ts, &value, &name); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		if err := addTraceDBInstantRow(sink, ts, "<xpower>", 0, 0, 0, fmt.Sprintf("tracing_mark_write: C|0|xpower_%s|%s", name, traceDBAnyText(value, "0"))); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	return coverage, rows.Err()
}

func exportTraceDBLog(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "log", "log", []string{"ts", "tid", "level", "tag", "context"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT ts, COALESCE(tid, 0), level, tag, COALESCE(context, '') FROM log ORDER BY ts")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, tid int64
		var level, tag any
		var text string
		if err := rows.Scan(&ts, &tid, &level, &tag, &text); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		msg := fmt.Sprintf("[%s][%s] %s", traceDBAnyText(level, "None"), traceDBAnyText(tag, "None"), strings.ReplaceAll(text, "\n", " "))
		if err := addTraceDBInstantRow(sink, ts, "<hilog>", tid, tid, 0, "print: "+msg); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	return coverage, rows.Err()
}

func exportTraceDBHiSysEvent(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, dict map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "log", "hisys_all_event", []string{"ts", "tid", "domain_id", "event_name_id", "contents"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT ts, COALESCE(tid, 0), COALESCE(domain_id, 0), COALESCE(event_name_id, 0), COALESCE(contents, '') FROM hisys_all_event ORDER BY ts")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, tid, domainID, eventID int64
		var contents string
		if err := rows.Scan(&ts, &tid, &domainID, &eventID, &contents); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		msg := fmt.Sprintf("%s/%s: %s", dict[domainID], dict[eventID], strings.ReplaceAll(contents, "\n", " "))
		if err := addTraceDBInstantRow(sink, ts, "<hisysevent>", tid, tid, 0, "print: "+msg); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	return coverage, rows.Err()
}

type traceDBCounterColumn struct {
	EventName string
	Column    string
}

func exportTraceDBSimpleCounters(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, family, table string, required []string, task string, columns []traceDBCounterColumn) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, family, table, required)
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	var selectCols []string
	for _, column := range columns {
		selectCols = append(selectCols, quoteSQLiteIdent(column.Column))
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT ts, "+strings.Join(selectCols, ", ")+" FROM "+quoteSQLiteIdent(table)+" ORDER BY ts")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		values := make([]any, len(columns))
		scans := make([]any, 0, len(columns)+1)
		var ts int64
		scans = append(scans, &ts)
		for i := range values {
			scans = append(scans, &values[i])
		}
		if err := rows.Scan(scans...); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		for i, column := range columns {
			if err := addTraceDBInstantRow(sink, ts, task, 0, 0, 0, fmt.Sprintf("tracing_mark_write: C|0|%s|%s", column.EventName, traceDBAnyText(values[i], "0"))); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	return coverage, rows.Err()
}

func traceDBResolvedThreadLineContext(index traceDBThreadIndex, itid int64) (string, int64, int64, bool) {
	if itid < 0 || itid > maxTraceDBInternalID || index.AmbiguousITID[itid] {
		return "", 0, 0, false
	}
	thread, ok := index.ByITID[itid]
	if !ok || thread.TID <= 0 || thread.TID > math.MaxInt32 || index.AmbiguousIPID[thread.IPID] {
		return "", 0, 0, false
	}
	_, _, tgid, ok := traceDBResolvedProcessLineContext(index, thread.IPID, "unknown")
	if !ok {
		return "", 0, 0, false
	}
	return traceDBCommName(thread.Name, "unknown"), thread.TID, tgid, true
}

func traceDBResolvedProcessLineContext(index traceDBThreadIndex, ipid int64, fallback string) (string, int64, int64, bool) {
	if ipid < 0 || ipid > maxTraceDBInternalID || index.AmbiguousIPID[ipid] {
		return "", 0, 0, false
	}
	process, ok := index.Processes[ipid]
	if !ok || process.PID <= 0 || process.PID > math.MaxInt32 {
		return "", 0, 0, false
	}
	return traceDBCommName(process.Name, fallback), process.PID, process.PID, true
}

func traceDBAnyText(value any, fallback string) string {
	switch v := value.(type) {
	case nil:
		return fallback
	case []byte:
		if len(v) == 0 {
			return fallback
		}
		return string(v)
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback
		}
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case float64:
		text := strconv.FormatFloat(v, 'f', -1, 64)
		if !strings.ContainsAny(text, ".eE") {
			text += ".0"
		}
		return text
	case sql.NullString:
		if v.Valid {
			return v.String
		}
	case sql.NullInt64:
		if v.Valid {
			return strconv.FormatInt(v.Int64, 10)
		}
	case sql.NullFloat64:
		if v.Valid {
			return traceDBAnyText(v.Float64, fallback)
		}
	}
	return fmt.Sprint(value)
}

func traceDBFirstExistingColumn(ctx context.Context, tdb *traceDB, table string, names ...string) (string, bool, error) {
	for _, name := range names {
		ok, err := tdb.columnExists(ctx, table, name)
		if err != nil {
			return "", false, err
		}
		if ok {
			return name, true, nil
		}
	}
	return "", false, nil
}

func traceDBInt64FromAny(value any, fallback int64) int64 {
	text := strings.TrimSpace(traceDBAnyText(value, ""))
	if text == "" {
		return fallback
	}
	if i, err := strconv.ParseInt(text, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(text, 64); err == nil {
		return int64(f)
	}
	return fallback
}

func traceDBIntegerText(value any, fallback string) string {
	switch v := value.(type) {
	case nil:
		return fallback
	case []byte:
		return traceDBIntegerString(string(v), fallback)
	case string:
		return traceDBIntegerString(v, fallback)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fallback
		}
		return strconv.FormatInt(int64(math.Round(v)), 10)
	case sql.NullString:
		if v.Valid {
			return traceDBIntegerString(v.String, fallback)
		}
	case sql.NullInt64:
		if v.Valid {
			return strconv.FormatInt(v.Int64, 10)
		}
	case sql.NullFloat64:
		if v.Valid && !math.IsNaN(v.Float64) && !math.IsInf(v.Float64, 0) {
			return strconv.FormatInt(int64(math.Round(v.Float64)), 10)
		}
	}
	return fallback
}

func traceDBIntegerString(text, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	if i, err := strconv.ParseInt(text, 10, 64); err == nil {
		return strconv.FormatInt(i, 10)
	}
	if f, err := strconv.ParseFloat(text, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return strconv.FormatInt(int64(math.Round(f)), 10)
	}
	return text
}

func traceDBPerfLabel(raw string) string {
	label := strings.TrimSpace(raw)
	label = strings.ReplaceAll(label, "\n", " ")
	label = strings.ReplaceAll(label, "\r", " ")
	label = strings.ReplaceAll(label, "|", "/")
	label = strings.Join(strings.Fields(label), " ")
	if label == "" {
		return "perf_sample"
	}
	return label
}

func traceDBPerfCallchain(ctx context.Context, frames []traceDBPerfFrame) (string, bool, error) {
	if len(frames) == 0 {
		return "", true, nil
	}
	var builder tracewire.PerfCallchainBuilder
	for _, frame := range frames {
		name := traceDBPerfOptionalLabel(frame.Name)
		if name == "" {
			return "", false, nil
		}
		if err := builder.AppendFrame(ctx, name); err != nil {
			return "", false, err
		}
	}
	return builder.String(), true, nil
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
