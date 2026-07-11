package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
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
	rawCoverage, err := exportTraceDBRawFtraceFamilies(ctx, tdb, sink, authority, lifecycleRunning)
	traceDBSetCoverageListElapsed(rawCoverage, stageStart)
	coverage = append(coverage, rawCoverage...)
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
	frameCoverage, err := exportTraceDBFrameSlice(ctx, tdb, sink, authority, lifecycleRunning)
	traceDBSetCoverageElapsed(&frameCoverage, stageStart)
	coverage = append(coverage, frameCoverage)
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
	syscallCoverage, err := exportTraceDBSyscall(ctx, tdb, sink, syncSpans, index)
	traceDBSetCoverageElapsed(&syscallCoverage, stageStart)
	coverage = append(coverage, syscallCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	taskPoolCoverage, err := exportTraceDBTaskPool(ctx, tdb, sink, index, running, dict)
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

func exportTraceDBSyscall(ctx context.Context, tdb *traceDB, _ *traceDBRowSink, syncSpans *traceDBSyncSpanAuthority, index traceDBThreadIndex) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "syscall", []string{"ts", "dur", "syscall_number", "itid"})
	coverage.FieldSources = map[string]string{
		"wire_laminar":     "current accepted rows submit typed B/E candidates to the shared authority; no endpoint is published by this exporter",
		"source_admission": "legacy SQL COALESCE/WHERE, scalar, lifecycle, CPU and anti-rescue correctness remain open as R1b-C",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	stableExpr, stableKnown, err := traceDBSyncSpanHiddenRowID(ctx, tdb, &coverage)
	if err != nil || !stableKnown {
		return coverage, err
	}
	query := fmt.Sprintf("SELECT %s, ts, COALESCE(dur, 0), syscall_number, itid FROM syscall WHERE dur >= 0 ORDER BY ts, %s", stableExpr, stableExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var ts, dur int64
		var stableRaw, num, itidRaw any
		if err := rows.Scan(&stableRaw, &ts, &dur, &num, &itidRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		itid, ok := traceDBStrictInternalID(itidRaw)
		if !ok {
			skipped["invalid_emitter_itid"]++
			continue
		}
		stableID, stableOK := traceDBStrictSQLiteInt(stableRaw)
		if !stableOK {
			return coverage, &traceDBOutputInvariantError{Reason: "invalid_syscall_hidden_rowid"}
		}
		task, tid, tgid, ok := traceDBResolvedThreadLineContext(index, itid)
		if !ok {
			skipped["unresolved_emitter_identity"]++
			continue
		}
		thread := index.ByITID[itid]
		if err := syncSpans.submit(ctx, traceDBSyncSpanCandidate{
			Producer:           traceDBSyncSpanProducerSyscall,
			StableKind:         traceDBSyncSpanStableSyscallRowID,
			StableID:           stableID,
			HeaderTID:          tid,
			HeaderTGID:         tgid,
			CanonicalITID:      itid,
			CanonicalITIDKnown: true,
			OwnerIPID:          thread.IPID,
			OwnerIPIDKnown:     true,
			Start:              ts,
			End:                ts + dur,
			StartCPU:           0,
			EndCPU:             0,
			StartCPUProvenance: traceDBSyncSpanCPULegacyUnverified,
			EndCPUProvenance:   traceDBSyncSpanCPULegacyUnverified,
			Task:               task,
			Name:               "sys_" + traceDBAnyText(num, "None"),
			NameProvenance:     traceDBSyncSpanNameSyscallNumber,
			DepthProvenance:    traceDBSyncSpanDepthUnknown,
		}); err != nil {
			return coverage, err
		}
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, rows.Err()
}

func exportTraceDBTaskPool(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "task_pool", []string{"task_id", "allocation_task_row", "execute_task_row", "allocation_itid", "execute_itid"})
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
		var execTask string
		var execTID, execTGID int64
		if execTS.Valid {
			execITID, execITIDOK := traceDBStrictInternalID(execITIDRaw)
			if !execITIDOK {
				skipped["invalid_execute_itid"]++
				continue
			}
			execTask, execTID, execTGID, execITIDOK = traceDBResolvedThreadLineContext(index, execITID)
			if !execITIDOK {
				skipped["unresolved_execute_identity"]++
				continue
			}
		}
		cookie := traceDBAnyText(taskID, "0")
		name := "TaskPool-" + cookie
		if err := addTraceDBInstantRow(sink, allocTS, allocTask, allocTID, allocTGID, 0, fmt.Sprintf("tracing_mark_write: S|%d|%s|%s", allocTGID, name, cookie)); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
		if execTS.Valid {
			if err := addTraceDBInstantRow(sink, execTS.Int64+execDur, execTask, execTID, execTGID, 0, fmt.Sprintf("tracing_mark_write: F|%d|%s|%s", execTGID, name, cookie)); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
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

func traceDBPerfCallchain(frames []traceDBPerfFrame) (string, bool) {
	if len(frames) == 0 {
		return "", true
	}
	var out strings.Builder
	for index, frame := range frames {
		name := traceDBPerfOptionalLabel(frame.Name)
		if name == "" {
			return "", false
		}
		extra := len(name)
		if index > 0 {
			extra++
		}
		if out.Len()+extra > maxTraceDBIdentityDisplayBytes {
			return "", false
		}
		if index > 0 {
			out.WriteByte(';')
		}
		out.WriteString(name)
	}
	return out.String(), true
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
