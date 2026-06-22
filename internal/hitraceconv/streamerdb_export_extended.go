package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

func exportTraceDBExtendedFamilies(ctx context.Context, tdb *traceDB, sink *traceDBRowSink) ([]TraceDBCoverage, error) {
	var coverage []TraceDBCoverage
	index, threadCoverage, err := tdb.loadThreadIndex(ctx)
	coverage = append(coverage, threadCoverage...)
	if err != nil {
		return coverage, err
	}
	running, runningCoverage, err := tdb.loadRunningIntervals(ctx)
	coverage = append(coverage, runningCoverage)
	if err != nil {
		return coverage, err
	}
	dict, dictCoverage, err := tdb.loadDataDict(ctx)
	coverage = append(coverage, dictCoverage)
	if err != nil {
		return coverage, err
	}
	exporters := []func(context.Context, *traceDB, *traceDBRowSink, traceDBThreadIndex, map[int64][]traceDBRunningInterval, map[int64]string) (TraceDBCoverage, error){
		exportTraceDBCallstack,
		exportTraceDBFrameSlice,
		exportTraceDBDMAFence,
		exportTraceDBSyscall,
		exportTraceDBTaskPool,
		exportTraceDBAppStartup,
		exportTraceDBStaticInitialize,
		exportTraceDBNativeHook,
		exportTraceDBCPUMeasures,
		exportTraceDBClockRates,
		exportTraceDBProcessMeasures,
		exportTraceDBNetwork,
		exportTraceDBDiskIO,
		exportTraceDBCPUUsage,
		exportTraceDBLiveProcess,
		exportTraceDBXPower,
		exportTraceDBLog,
		exportTraceDBHiSysEvent,
	}
	for _, exporter := range exporters {
		item, err := exporter(ctx, tdb, sink, index, running, dict)
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

func exportTraceDBCallstack(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, running map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "callstack", []string{"ts", "dur", "name", "flag", "cookie", "chainId", "callid"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT ts, COALESCE(dur, 0), COALESCE(name, ''),
		       COALESCE(flag, ''), cookie, chainId, callid
		FROM callstack
		WHERE ts IS NOT NULL AND callid IS NOT NULL
		ORDER BY ts
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		var ts, dur, callid int64
		var name, flag string
		var cookie sql.NullString
		var chainID sql.NullString
		if err := rows.Scan(&ts, &dur, &name, &flag, &cookie, &chainID, &callid); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		task, tid, tgid := traceDBThreadLineContext(index, callid)
		cpu := traceDBCPUAt(running, callid, ts, 0)
		asyncCookie := firstNonEmpty(nullString(cookie), nullString(chainID), "0")
		switch flag {
		case "S":
			if err := addTraceDBInstantRow(sink, ts, task, tid, tgid, cpu, fmt.Sprintf("tracing_mark_write: S|%d|%s|%s", tgid, name, asyncCookie)); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		case "C":
			if err := addTraceDBInstantRow(sink, ts, task, tid, tgid, cpu, fmt.Sprintf("tracing_mark_write: F|%d|%s|%s", tgid, name, asyncCookie)); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		default:
			if err := addTraceDBSpanRows(sink, ts, ts+maxInt64(dur, 0), task, tid, tgid, cpu, name); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted += 2
		}
	}
	return coverage, rows.Err()
}

func exportTraceDBFrameSlice(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "frame_slice", []string{"ts", "dur", "type_desc", "vsync", "ipid", "itid"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT ts, dur, COALESCE(type_desc, ''), vsync, COALESCE(ipid, 0), COALESCE(itid, 0) FROM frame_slice WHERE dur > 0 ORDER BY ts")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, dur, ipid, itid int64
		var typ string
		var vsync any
		if err := rows.Scan(&ts, &dur, &typ, &vsync, &ipid, &itid); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		task, tid, tgid := traceDBThreadOrProcessContext(index, itid, ipid, "frame")
		kind := "Expected"
		if typ == "actural" {
			kind = "Actual"
		}
		if err := addTraceDBSpanRows(sink, ts, ts+dur, task, tid, tgid, 0, "Frame"+kind+"-"+traceDBAnyText(vsync, "None")); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
	}
	return coverage, rows.Err()
}

func exportTraceDBDMAFence(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "dma_fence", []string{"ts", "dur", "cat", "driver", "timeline", "context", "seqno"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT ts, COALESCE(dur, 0), cat, COALESCE(driver, ''), COALESCE(timeline, ''), context, seqno FROM dma_fence ORDER BY ts")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, dur int64
		var cat any
		var driver, timeline string
		var contextValue, seqno any
		if err := rows.Scan(&ts, &dur, &cat, &driver, &timeline, &contextValue, &seqno); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		catText := traceDBAnyText(cat, "dma_fence")
		if dur > 0 {
			if err := addTraceDBSpanRows(sink, ts, ts+dur, "<dma_fence>", 0, 0, 0, fmt.Sprintf("%s:%s:%s", catText, timeline, traceDBAnyText(seqno, "None"))); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted += 2
			continue
		}
		body := fmt.Sprintf("%s: driver=%s timeline=%s context=%s seqno=%s", catText, driver, timeline, traceDBAnyText(contextValue, "None"), traceDBAnyText(seqno, "None"))
		if err := addTraceDBInstantRow(sink, ts, "<dma_fence>", 0, 0, 0, body); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	return coverage, rows.Err()
}

func exportTraceDBSyscall(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "syscall", []string{"ts", "dur", "syscall_number", "itid"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT ts, COALESCE(dur, 0), syscall_number, itid FROM syscall WHERE dur >= 0 ORDER BY ts")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, dur, itid int64
		var num any
		if err := rows.Scan(&ts, &dur, &num, &itid); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		task, tid, tgid := traceDBThreadLineContext(index, itid)
		if err := addTraceDBSpanRows(sink, ts, ts+dur, task, tid, tgid, 0, "sys_"+traceDBAnyText(num, "None")); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
	}
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
		       COALESCE(tp.allocation_itid, 0), COALESCE(tp.execute_itid, 0)
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
	for rows.Next() {
		var taskID any
		var allocTS int64
		var execTS sql.NullInt64
		var execDur, allocITID, execITID int64
		if err := rows.Scan(&taskID, &allocTS, &execTS, &execDur, &allocITID, &execITID); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		cookie := traceDBAnyText(taskID, "0")
		name := "TaskPool-" + cookie
		task, tid, tgid := traceDBThreadLineContext(index, allocITID)
		if err := addTraceDBInstantRow(sink, allocTS, task, tid, tgid, 0, fmt.Sprintf("tracing_mark_write: S|%d|%s|%s", tgid, name, cookie)); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
		if execTS.Valid {
			task, tid, tgid = traceDBThreadLineContext(index, execITID)
			if err := addTraceDBInstantRow(sink, execTS.Int64+execDur, task, tid, tgid, 0, fmt.Sprintf("tracing_mark_write: F|%d|%s|%s", tgid, name, cookie)); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	return coverage, rows.Err()
}

func exportTraceDBAppStartup(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, dict map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "app_startup", []string{"start_time", "end_time", "start_name", "ipid"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT start_time, end_time, COALESCE(start_name, 0), COALESCE(ipid, 0) FROM app_startup WHERE end_time > start_time ORDER BY start_time")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var start, end, nameID, ipid int64
		if err := rows.Scan(&start, &end, &nameID, &ipid); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		task, tid, tgid := traceDBThreadOrProcessContext(index, 0, ipid, "startup")
		if err := addTraceDBSpanRows(sink, start, end, task, tid, tgid, 0, "AppStartup:"+firstNonEmpty(dict[nameID], "startup")); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
	}
	return coverage, rows.Err()
}

func exportTraceDBStaticInitialize(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "static_initalize", []string{"start_time", "end_time", "so_name", "ipid", "tid"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT start_time, end_time, so_name, COALESCE(ipid, 0), COALESCE(tid, 0) FROM static_initalize WHERE end_time > start_time ORDER BY start_time")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var start, end, ipid, tid int64
		var so any
		if err := rows.Scan(&start, &end, &so, &ipid, &tid); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		task, _, tgid := traceDBThreadOrProcessContext(index, 0, ipid, "soInit")
		if tid == 0 {
			tid = tgid
		}
		if err := addTraceDBSpanRows(sink, start, end, task, tid, tgid, 0, "SoInit:"+traceDBAnyText(so, "None")); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
	}
	return coverage, rows.Err()
}

func exportTraceDBNativeHook(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "native_hook", []string{"start_ts", "end_ts", "event_type", "all_heap_size", "itid", "ipid"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT start_ts, end_ts, COALESCE(event_type, 'NativeHook'), all_heap_size, COALESCE(itid, 0), COALESCE(ipid, 0) FROM native_hook WHERE end_ts > start_ts ORDER BY start_ts")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var start, end, itid, ipid int64
		var name string
		var heap any
		if err := rows.Scan(&start, &end, &name, &heap, &itid, &ipid); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		task, tid, tgid := traceDBThreadOrProcessContext(index, itid, ipid, "hook")
		if err := addTraceDBSpanRows(sink, start, end, task, tid, tgid, 0, firstNonEmpty(name, "NativeHook")); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
		if text := traceDBAnyText(heap, ""); text != "" {
			if err := addTraceDBInstantRow(sink, start, task, tid, tgid, 0, fmt.Sprintf("tracing_mark_write: C|%d|HeapSize|%s", tgid, text)); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	return coverage, rows.Err()
}

func exportTraceDBCPUMeasures(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "counter", "measure", []string{"ts", "value", "filter_id"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	filterCoverage, err := tdb.inspectCoverage(ctx, "counter", "cpu_measure_filter", []string{"id", "name", "cpu"})
	if err != nil {
		return coverage, err
	}
	if !filterCoverage.Found || len(filterCoverage.ColumnsMissing) > 0 {
		coverage.Skipped = "missing cpu_measure_filter dependency"
		return coverage, nil
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT m.ts, m.value, f.name, COALESCE(f.cpu, 0)
		FROM measure m
		JOIN cpu_measure_filter f ON f.id = m.filter_id
		WHERE f.name IN ('cpu_idle', 'cpu_frequency', 'cpu_frequency_limits_min', 'cpu_frequency_limits_max')
		ORDER BY m.ts
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, cpu int64
		var value any
		var name string
		if err := rows.Scan(&ts, &value, &name, &cpu); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		valueText := traceDBAnyText(value, "0")
		body := ""
		switch name {
		case "cpu_idle":
			body = fmt.Sprintf("cpu_idle: state=%s cpu_id=%d", valueText, cpu)
		case "cpu_frequency":
			body = fmt.Sprintf("cpu_frequency: state=%s cpu_id=%d", valueText, cpu)
		case "cpu_frequency_limits_min":
			body = fmt.Sprintf("cpu_frequency_limits: min=%s max=0 cpu_id=%d", valueText, cpu)
		case "cpu_frequency_limits_max":
			body = fmt.Sprintf("cpu_frequency_limits: min=0 max=%s cpu_id=%d", valueText, cpu)
		}
		if body == "" {
			continue
		}
		if err := addTraceDBInstantRow(sink, ts, "<idle>", 0, 0, cpu, body); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	return coverage, rows.Err()
}

func exportTraceDBClockRates(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "counter", "measure_filter", []string{"id", "name", "type"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT m.ts, m.value, f.name
		FROM measure m
		JOIN measure_filter f ON f.id = m.filter_id
		WHERE f.type = 'clock_rate_filter'
		ORDER BY m.ts
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
		if err := addTraceDBInstantRow(sink, ts, "<kworker>", 0, 0, 0, fmt.Sprintf("clock_set_rate: %s state=%s cpu_id=0", name, traceDBAnyText(value, "0"))); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
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
		SELECT m.ts, m.value, f.name, COALESCE(f.ipid, 0)
		FROM process_measure m
		JOIN process_measure_filter f ON f.id = m.filter_id
		ORDER BY m.ts
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, ipid int64
		var value any
		var name string
		if err := rows.Scan(&ts, &value, &name, &ipid); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		task, tid, tgid := traceDBThreadOrProcessContext(index, 0, ipid, "unknown")
		if err := addTraceDBInstantRow(sink, ts, task, tid, tgid, 0, fmt.Sprintf("tracing_mark_write: C|%d|%s|%s", tgid, name, traceDBAnyText(value, "0"))); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
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

func addTraceDBSpanRows(sink *traceDBRowSink, start, end int64, task string, tid, tgid, cpu int64, name string) error {
	if err := addTraceDBInstantRow(sink, start, task, tid, tgid, cpu, fmt.Sprintf("tracing_mark_write: B|%d|%s", tgid, name)); err != nil {
		return err
	}
	return addTraceDBInstantRow(sink, end, task, tid, tgid, cpu, fmt.Sprintf("tracing_mark_write: E|%d|", tgid))
}

func traceDBThreadLineContext(index traceDBThreadIndex, itid int64) (string, int64, int64) {
	thread, ok := index.ByITID[itid]
	if !ok {
		return "unknown", 0, 0
	}
	process := index.Processes[thread.IPID]
	return traceDBCommName(thread.Name, "unknown"), thread.TID, firstNonZero(process.PID, thread.TID)
}

func traceDBThreadOrProcessContext(index traceDBThreadIndex, itid, ipid int64, fallback string) (string, int64, int64) {
	if itid != 0 {
		return traceDBThreadLineContext(index, itid)
	}
	process := index.Processes[ipid]
	if process.PID != 0 {
		return traceDBCommName(process.Name, fallback), process.PID, process.PID
	}
	return fallback, 0, 0
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
