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
	perfCoverage, err := exportTraceDBPerfSamples(ctx, tdb, sink)
	coverage = append(coverage, perfCoverage...)
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

type traceDBPerfFrame struct {
	Name string
	DSO  string
	IP   string
}

func exportTraceDBPerfSamples(ctx context.Context, tdb *traceDB, sink *traceDBRowSink) ([]TraceDBCoverage, error) {
	sampleCoverage, err := tdb.inspectCoverage(ctx, "perf", "perf_sample", []string{"callchain_id", "thread_id"})
	if err != nil || !sampleCoverage.Found || len(sampleCoverage.ColumnsMissing) > 0 {
		return []TraceDBCoverage{sampleCoverage}, err
	}
	callchainCoverage, err := tdb.inspectCoverage(ctx, "perf", "perf_callchain", []string{"callchain_id", "depth"})
	if err != nil || !callchainCoverage.Found || len(callchainCoverage.ColumnsMissing) > 0 {
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
	}
	tsColumn, ok, err := traceDBFirstExistingColumn(ctx, tdb, "perf_sample", "timestamp_trace", "timeStamp", "timestamp", "ts")
	if err != nil {
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
	}
	if !ok {
		sampleCoverage.ColumnsMissing = append(sampleCoverage.ColumnsMissing, "timestamp_trace|timeStamp|timestamp|ts")
		sampleCoverage.Skipped = "missing required columns: " + strings.Join(sampleCoverage.ColumnsMissing, ",")
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, nil
	}
	cpuExpr := "0"
	if column, ok, err := traceDBFirstExistingColumn(ctx, tdb, "perf_sample", "cpu_id", "cpu"); err != nil {
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
	} else if ok {
		cpuExpr = "COALESCE(s." + quoteSQLiteIdent(column) + ", 0)"
	}
	eventCountExpr := "1"
	if column, ok, err := traceDBFirstExistingColumn(ctx, tdb, "perf_sample", "event_count", "period", "sample_period", "sample_weight"); err != nil {
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
	} else if ok {
		eventCountExpr = "COALESCE(s." + quoteSQLiteIdent(column) + ", 1)"
	}
	threadJoin := ""
	pidExpr := "s." + quoteSQLiteIdent("thread_id")
	threadNameExpr := "CAST(s." + quoteSQLiteIdent("thread_id") + " AS TEXT)"
	if hasThread, err := tdb.tableExists(ctx, "perf_thread"); err != nil {
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
	} else if hasThread {
		threadJoin = "LEFT JOIN perf_thread pt ON pt.thread_id = s." + quoteSQLiteIdent("thread_id")
		if ok, err := tdb.columnExists(ctx, "perf_thread", "process_id"); err != nil {
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		} else if ok {
			pidExpr = "COALESCE(pt.process_id, s." + quoteSQLiteIdent("thread_id") + ")"
		}
		if ok, err := tdb.columnExists(ctx, "perf_thread", "thread_name"); err != nil {
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		} else if ok {
			threadNameExpr = "COALESCE(NULLIF(pt.thread_name, ''), CAST(s." + quoteSQLiteIdent("thread_id") + " AS TEXT))"
		}
	}
	eventTypeJoin := ""
	eventTypeExpr := "'perf'"
	if hasReport, err := tdb.tableExists(ctx, "perf_report"); err != nil {
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
	} else if hasReport {
		if hasEventTypeID, err := tdb.columnExists(ctx, "perf_sample", "event_type_id"); err != nil {
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		} else if hasEventTypeID {
			eventTypeJoin = "LEFT JOIN perf_report pr ON pr.id = s.event_type_id"
			if ok, err := tdb.columnExists(ctx, "perf_report", "report_value"); err != nil {
				return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
			} else if ok {
				eventTypeExpr = "COALESCE(NULLIF(pr.report_value, ''), 'perf')"
			}
		}
	}
	frames, frameCoverage, err := tdb.loadPerfFrames(ctx)
	if err != nil {
		return []TraceDBCoverage{sampleCoverage, callchainCoverage, frameCoverage}, err
	}
	callchainCoverage.RowsEmitted = frameCoverage.RowsEmitted
	query := fmt.Sprintf(`
		SELECT COALESCE(s.%s, 0), %s, s.%s, %s, %s, %s, %s, s.%s
		FROM perf_sample s
		%s
		%s
		WHERE s.%s != -1
	`, quoteSQLiteIdent(tsColumn), cpuExpr, quoteSQLiteIdent("thread_id"), pidExpr, threadNameExpr, eventCountExpr, eventTypeExpr, quoteSQLiteIdent("callchain_id"), threadJoin, eventTypeJoin, quoteSQLiteIdent("callchain_id"))
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		sampleCoverage.Error = err.Error()
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		}
		var ts, cpu, tid, pid, callchainID int64
		var threadName, eventType string
		var eventCountRaw any
		if err := rows.Scan(&ts, &cpu, &tid, &pid, &threadName, &eventCountRaw, &eventType, &callchainID); err != nil {
			sampleCoverage.Error = err.Error()
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		}
		eventCount := traceDBInt64FromAny(eventCountRaw, 1)
		if eventCount <= 0 {
			eventCount = 1
		}
		sampleFrames := frames[callchainID]
		leaf := traceDBPerfFrame{Name: "perf_sample"}
		if len(sampleFrames) > 0 {
			leaf = sampleFrames[0]
		}
		symbol := firstNonEmpty(leaf.Name, "perf_sample")
		dso := firstNonEmpty(leaf.DSO, "unknown")
		ip := firstNonEmpty(leaf.IP, "")
		callchain := traceDBPerfCallchain(sampleFrames)
		callchainStatus := "missing"
		symbolizationStatus := "unsymbolized"
		if callchain != "" {
			callchainStatus = "symbolized"
			symbolizationStatus = "symbolized"
		}
		label := traceDBPerfLabel("hiperf:" + firstNonEmpty(eventType, "perf") + ":" + symbol)
		counter := traceDBPerfLabel("hiperf:" + firstNonEmpty(eventType, "perf") + ":event_count")
		task := traceDBCommName(threadName, "perf")
		if err := addTraceDBInstantRow(sink, ts, task, tid, pid, cpu, fmt.Sprintf("tracing_mark_write: B|%d|%s", pid, label)); err != nil {
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		}
		sampleCoverage.RowsEmitted++
		if err := addTraceDBInstantRow(sink, ts+1_000, task, tid, pid, cpu, fmt.Sprintf("tracing_mark_write: E|%d|", pid)); err != nil {
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		}
		sampleCoverage.RowsEmitted++
		if err := addTraceDBInstantRow(sink, ts, task, tid, pid, cpu, fmt.Sprintf("tracing_mark_write: C|%d|%s|%d", pid, counter, eventCount)); err != nil {
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		}
		sampleCoverage.RowsEmitted++
		body := fmt.Sprintf("perf_sample: cpu=%d cpu_known=true pid=%d tid=%d thread_comm=%s sample_weight=%d event=%s symbol=%s dso=%s ip=%s callchain=%s source=trace_streamer_db sample_kind=on_cpu symbolization_status=%s clock=trace_streamer_db clock_confidence=calibrated callchain_status=%s",
			cpu, pid, tid, quoteTraceValue(task), eventCount, quoteTraceValue(firstNonEmpty(eventType, "perf")),
			quoteTraceValue(symbol), quoteTraceValue(dso), quoteTraceValue(ip), quoteTraceValue(callchain),
			symbolizationStatus, callchainStatus)
		if err := addTraceDBInstantRow(sink, ts, task, tid, pid, cpu, body); err != nil {
			return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
		}
		sampleCoverage.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		sampleCoverage.Error = err.Error()
		return []TraceDBCoverage{sampleCoverage, callchainCoverage}, err
	}
	return []TraceDBCoverage{sampleCoverage, callchainCoverage}, nil
}

func (tdb *traceDB) loadPerfFrames(ctx context.Context) (map[int64][]traceDBPerfFrame, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "perf", "perf_callchain", []string{"callchain_id", "depth"})
	out := map[int64][]traceDBPerfFrame{}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	hasID, err := tdb.columnExists(ctx, "perf_callchain", "id")
	if err != nil {
		return out, coverage, err
	}
	hasName, err := tdb.columnExists(ctx, "perf_callchain", "name")
	if err != nil {
		return out, coverage, err
	}
	hasIP, err := tdb.columnExists(ctx, "perf_callchain", "ip")
	if err != nil {
		return out, coverage, err
	}
	hasFileID, err := tdb.columnExists(ctx, "perf_callchain", "file_id")
	if err != nil {
		return out, coverage, err
	}
	hasSymbolID, err := tdb.columnExists(ctx, "perf_callchain", "symbol_id")
	if err != nil {
		return out, coverage, err
	}
	idExpr := "NULL"
	if hasID {
		idExpr = "c.id"
	}
	nameExpr := "NULL"
	if hasName {
		nameExpr = "c.name"
	}
	ipExpr := "NULL"
	if hasIP {
		ipExpr = "c.ip"
	}
	symbolJoin := ""
	symbolExpr := "NULL"
	if hasID {
		if hasSymbols, err := tdb.tableExists(ctx, "hmtrace_perf_symbolized_frame"); err != nil {
			return out, coverage, err
		} else if hasSymbols {
			symbolJoin = "LEFT JOIN hmtrace_perf_symbolized_frame sf ON sf.perf_callchain_row_id = c.id"
			symbolExpr = "sf.display_name"
		}
	}
	dictJoin := ""
	dictExpr := "NULL"
	if hasName {
		if hasDict, err := tdb.tableExists(ctx, "data_dict"); err != nil {
			return out, coverage, err
		} else if hasDict {
			dictJoin = "LEFT JOIN data_dict name_dict ON name_dict.id = c.name"
			dictExpr = "name_dict.data"
		}
	}
	fileJoin := ""
	fileSymbolExpr := "NULL"
	filePathExpr := "NULL"
	if hasFileID && hasSymbolID {
		if hasFiles, err := tdb.tableExists(ctx, "perf_files"); err != nil {
			return out, coverage, err
		} else if hasFiles {
			fileJoin = "LEFT JOIN perf_files pf ON pf.file_id = c.file_id AND pf.serial_id = c.symbol_id"
			if ok, err := tdb.columnExists(ctx, "perf_files", "symbol"); err != nil {
				return out, coverage, err
			} else if ok {
				fileSymbolExpr = "pf.symbol"
			}
			if ok, err := tdb.columnExists(ctx, "perf_files", "path"); err != nil {
				return out, coverage, err
			} else if ok {
				filePathExpr = "pf.path"
			}
		}
	}
	query := fmt.Sprintf(`
		SELECT c.callchain_id, COALESCE(c.depth, 0), %s, %s, %s, %s, %s, %s, %s
		FROM perf_callchain c
		%s
		%s
		%s
		ORDER BY c.callchain_id, COALESCE(c.depth, 0)
	`, idExpr, nameExpr, ipExpr, symbolExpr, dictExpr, fileSymbolExpr, filePathExpr, symbolJoin, dictJoin, fileJoin)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var callchainID int64
		var depth int64
		var idRaw, nameRaw, ipRaw, symbolizedRaw, dictRaw, fileSymbolRaw, filePathRaw any
		if err := rows.Scan(&callchainID, &depth, &idRaw, &nameRaw, &ipRaw, &symbolizedRaw, &dictRaw, &fileSymbolRaw, &filePathRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		_ = idRaw
		name := firstNonEmpty(
			traceDBAnyText(symbolizedRaw, ""),
			traceDBAnyText(dictRaw, ""),
			traceDBAnyText(nameRaw, ""),
			traceDBAnyText(fileSymbolRaw, ""),
			"perf_sample",
		)
		out[callchainID] = append(out[callchainID], traceDBPerfFrame{
			Name: traceDBPerfLabel(name),
			DSO:  traceDBAnyText(filePathRaw, ""),
			IP:   traceDBAnyText(ipRaw, ""),
		})
		coverage.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	return out, coverage, nil
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

func traceDBPerfCallchain(frames []traceDBPerfFrame) string {
	if len(frames) == 0 {
		return ""
	}
	parts := make([]string, 0, len(frames))
	for _, frame := range frames {
		name := traceDBPerfLabel(frame.Name)
		if name == "" {
			continue
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ";")
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
