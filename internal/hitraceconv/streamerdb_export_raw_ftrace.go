package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type traceDBRawEvent struct {
	StableID  int64
	TS        int64
	Name      string
	CPU       int64
	CPUKnown  bool
	ITID      int64
	ITIDKnown bool
	TID       int64
	TIDKnown  bool
	PID       int64
	PIDKnown  bool
	ArgSetID  int64
}

func exportTraceDBRawFtraceFamilies(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, running map[int64][]traceDBRunningInterval) ([]TraceDBCoverage, error) {
	schemaCoverage, err := tdb.inspectCoverage(ctx, "raw_ftrace", "raw", []string{"ts", "name"})
	if err != nil || !schemaCoverage.Found || len(schemaCoverage.ColumnsMissing) > 0 {
		return []TraceDBCoverage{schemaCoverage}, err
	}
	argsetColumn, ok, err := traceDBFirstExistingColumn(ctx, tdb, "raw", "argset", "argsetid", "argset_id", "arg_set_id")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	if !ok {
		schemaCoverage.Skipped = "missing argset/argsetid column; raw ftrace rows cannot be rendered safely"
		return []TraceDBCoverage{schemaCoverage}, nil
	}
	argsets, resolverCoverage, err := tdb.loadArgsets(ctx)
	outCoverage := append([]TraceDBCoverage{schemaCoverage}, resolverCoverage...)
	if err != nil {
		return outCoverage, err
	}
	if !traceDBRawArgsetsReady(resolverCoverage) {
		schemaCoverage.Skipped = "missing args/data_dict dependency; raw ftrace rows cannot be rendered safely"
		outCoverage[0] = schemaCoverage
		return outCoverage, nil
	}
	hasSourceID, err := tdb.columnExists(ctx, "raw", "id")
	if err != nil {
		return outCoverage, err
	}
	stableExpr := "rowid"
	stableSource := "raw.rowid"
	duplicateStableIDs := map[int64]bool{}
	if hasSourceID {
		stableExpr = traceDBQuoteIdent("id")
		stableSource = "raw.id"
	} else if err := traceDBRequireRowID(ctx, tdb, "raw"); err != nil {
		schemaCoverage.Skipped = "missing raw.id and usable SQLite rowid; no stable source identity/order"
		outCoverage[0] = schemaCoverage
		return outCoverage, nil
	}
	duplicateStableIDs, err = traceDBRawDuplicateStableIDs(ctx, tdb, stableExpr)
	if err != nil {
		schemaCoverage.Error = err.Error()
		outCoverage[0] = schemaCoverage
		return outCoverage, err
	}
	if schemaCoverage.FieldSources == nil {
		schemaCoverage.FieldSources = map[string]string{}
	}
	schemaCoverage.FieldSources["stable_identity"] = stableSource
	schemaCoverage.FieldSources["same_timestamp_order"] = "raw.ts," + stableSource
	schemaCoverage.FieldSources["header_cpu"] = "raw.cpu when present; otherwise exact untainted thread_state.Running witness"
	schemaCoverage.FieldSources["header_identity"] = "typed raw.itid/raw.tid/raw.pid plus timestamped thread/process incarnation; names are display-only"
	cpuExpr, hasCPU, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "cpu")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	// raw.itid is the only proven internal-thread identity.  "callid" is
	// table-family overloaded in Trace Streamer (thread in callstack, CPU in
	// irq), so it must never be generalized into a raw thread identity.
	itidExpr, hasITID, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "itid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	tidExpr, hasTID, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "tid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	pidExpr, hasPID, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "pid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	query := fmt.Sprintf(`
		SELECT %s, ts, name, %s, %s, %s, %s, %s
		FROM raw
		ORDER BY ts, %s
	`, stableExpr, cpuExpr, itidExpr, tidExpr, pidExpr, traceDBQuoteIdent(argsetColumn), stableExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	defer rows.Close()
	classCoverage := map[string]*TraceDBCoverage{}
	classSkipped := map[string]map[string]int{}
	schemaSkipped := map[string]int{}
	unsupported := TraceDBCoverage{Family: "raw_ftrace", Table: "unsupported", Role: "unsupported_input", Found: true, Skipped: "unsupported raw ftrace event family"}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return outCoverage, err
		}
		var raw traceDBRawEvent
		var stableIDRaw, tsRaw, nameRaw, cpuRaw, itidRaw, tidRaw, pidRaw, argsetRaw any
		if err := rows.Scan(&stableIDRaw, &tsRaw, &nameRaw, &cpuRaw, &itidRaw, &tidRaw, &pidRaw, &argsetRaw); err != nil {
			schemaCoverage.Error = err.Error()
			return []TraceDBCoverage{schemaCoverage}, err
		}
		var ok bool
		if raw.StableID, ok = traceDBStrictSQLiteInt(stableIDRaw); !ok || raw.StableID < 0 || (!hasSourceID && raw.StableID == 0) {
			schemaSkipped["invalid_stable_id"]++
			continue
		}
		if duplicateStableIDs[raw.StableID] {
			schemaSkipped["duplicate_source_id"]++
			continue
		}
		if raw.TS, ok = traceDBStrictSQLiteInt(tsRaw); !ok || raw.TS < 0 {
			schemaSkipped["invalid_timestamp"]++
			continue
		}
		if raw.Name, ok = traceDBStrictArgText(nameRaw, false); !ok || strings.TrimSpace(raw.Name) != raw.Name || strings.ToLower(raw.Name) != raw.Name {
			schemaSkipped["invalid_event_name"]++
			continue
		}
		class := traceDBRawFtraceClass(raw.Name)
		if class == "" {
			unsupported.RowsRead++
			continue
		}
		item := traceDBRawClassCoverage(classCoverage, class)
		item.RowsRead++
		if argsetRaw == nil {
			traceDBRawCountSkip(classSkipped, class, "missing_argset")
			continue
		}
		raw.ArgSetID, ok = traceDBStrictSQLiteInt(argsetRaw)
		if !ok || raw.ArgSetID < 0 {
			traceDBRawCountSkip(classSkipped, class, "invalid_argset_id")
			continue
		}
		if !argsets.Present[raw.ArgSetID] {
			traceDBRawCountSkip(classSkipped, class, "missing_argset")
			continue
		}
		if argsets.Invalid[raw.ArgSetID] {
			traceDBRawCountSkip(classSkipped, class, "invalid_argset")
			continue
		}
		args := argsets.Sets[raw.ArgSetID]
		if !traceDBRawRequiredArgs(raw.Name, args, argsets.InvalidKeys[raw.ArgSetID]) {
			traceDBRawCountSkip(classSkipped, class, "missing_required_args")
			continue
		}
		if raw.ITID, raw.ITIDKnown, ok = traceDBRawOptionalID(itidRaw, hasITID, maxTraceDBInternalID, true); !ok {
			traceDBRawCountSkip(classSkipped, class, "invalid_itid")
			continue
		}
		if raw.TID, raw.TIDKnown, ok = traceDBRawOptionalID(tidRaw, hasTID, math.MaxInt32, false); !ok {
			traceDBRawCountSkip(classSkipped, class, "invalid_tid")
			continue
		}
		if raw.PID, raw.PIDKnown, ok = traceDBRawOptionalID(pidRaw, hasPID, math.MaxInt32, false); !ok {
			traceDBRawCountSkip(classSkipped, class, "invalid_pid")
			continue
		}
		task, tid, tgid, effectiveITID, identityOK := traceDBRawLineContext(raw, index)
		if !identityOK {
			traceDBRawCountSkip(classSkipped, class, "identity_conflict_or_ambiguous")
			continue
		}
		if hasCPU && cpuRaw != nil {
			raw.CPU, ok = traceDBStrictSQLiteInt(cpuRaw)
			if !ok || !validTraceDBCPUIndex(raw.CPU) {
				traceDBRawCountSkip(classSkipped, class, "invalid_cpu")
				continue
			}
			raw.CPUKnown = true
		} else if effectiveITID >= 0 && !index.RunningGlobalTaint && !index.RunningTaintedITID[effectiveITID] {
			raw.CPU, raw.CPUKnown = traceDBKnownCPUAt(running, effectiveITID, raw.TS)
		}
		if !raw.CPUKnown {
			traceDBRawCountSkip(classSkipped, class, "unknown_cpu")
			continue
		}
		body, ok := traceDBRenderRawFtrace(raw.Name, args)
		if !ok {
			continue
		}
		if err := addTraceDBInstantRow(sink, raw.TS, task, tid, tgid, raw.CPU, body); err != nil {
			return outCoverage, err
		}
		item.RowsEmitted++
		schemaCoverage.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	outCoverage[0] = schemaCoverage
	for _, key := range sortedRawFtraceCoverageKeys(classCoverage) {
		if summary := traceDBCountSummary(classSkipped[key]); summary != "" {
			classCoverage[key].Skipped = summary
		}
		outCoverage = append(outCoverage, *classCoverage[key])
	}
	if unsupported.RowsRead > 0 {
		outCoverage = append(outCoverage, unsupported)
	}
	if summary := traceDBCountSummary(schemaSkipped); summary != "" {
		outCoverage[0].Skipped = summary
	}
	return outCoverage, nil
}

func traceDBRawArgsetsReady(coverage []TraceDBCoverage) bool {
	for _, item := range coverage {
		if item.Table != "args" && item.Table != "data_dict" {
			continue
		}
		if !item.Found || len(item.ColumnsMissing) > 0 {
			return false
		}
	}
	return true
}

func traceDBRawOptionalExpr(ctx context.Context, tdb *traceDB, table string, names ...string) (string, bool, error) {
	name, ok, err := traceDBFirstExistingColumn(ctx, tdb, table, names...)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "NULL", false, nil
	}
	return traceDBQuoteIdent(name), true, nil
}

func traceDBRawOptionalID(value any, columnPresent bool, maxValue int64, zeroIsKnown bool) (int64, bool, bool) {
	if !columnPresent || value == nil {
		return 0, false, true
	}
	parsed, ok := traceDBStrictSQLiteInt(value)
	if !ok || parsed < 0 || parsed > maxValue {
		return 0, false, false
	}
	if parsed == 0 {
		return 0, zeroIsKnown, true
	}
	return parsed, true, true
}

func traceDBRequireRowID(ctx context.Context, tdb *traceDB, table string) error {
	rows, err := tdb.db.QueryContext(ctx, "SELECT rowid FROM "+traceDBQuoteIdent(table)+" LIMIT 0")
	if err != nil {
		return err
	}
	return rows.Close()
}

func traceDBRawDuplicateStableIDs(ctx context.Context, tdb *traceDB, stableExpr string) (map[int64]bool, error) {
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT `+stableExpr+`, COUNT(1)
		FROM raw
		GROUP BY `+stableExpr+`
		HAVING COUNT(1) > 1 OR `+stableExpr+` IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var identityRaw, countRaw any
		if err := rows.Scan(&identityRaw, &countRaw); err != nil {
			return nil, err
		}
		identity, identityOK := traceDBStrictSQLiteInt(identityRaw)
		count, countOK := traceDBStrictSQLiteInt(countRaw)
		if identityOK && identity >= 0 && countOK && count > 1 {
			out[identity] = true
		}
	}
	return out, rows.Err()
}

func traceDBRawCountSkip(items map[string]map[string]int, class, reason string) {
	if items[class] == nil {
		items[class] = map[string]int{}
	}
	items[class][reason]++
}

func traceDBQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func traceDBRawClassCoverage(items map[string]*TraceDBCoverage, class string) *TraceDBCoverage {
	item := items[class]
	if item != nil {
		return item
	}
	item = &TraceDBCoverage{Family: "raw_ftrace", Table: class, Role: "query_ready_export", Found: true}
	items[class] = item
	return item
}

func sortedRawFtraceCoverageKeys(items map[string]*TraceDBCoverage) []string {
	out := make([]string, 0, len(items))
	for key := range items {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func traceDBRawFtraceClass(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "binder_"):
		return "binder"
	case lower == "block_rq_issue" || lower == "block_rq_insert" || lower == "block_rq_complete" || lower == "block_bio_remap" ||
		strings.HasPrefix(lower, "ufshcd_") || strings.HasPrefix(lower, "mmc_request_") || strings.HasPrefix(lower, "scsi_dispatch_cmd"):
		return "block_storage"
	case strings.HasPrefix(lower, "android_fs_dataread") || strings.HasPrefix(lower, "android_fs_datawrite") ||
		strings.HasPrefix(lower, "f2fs_direct_io") || strings.HasPrefix(lower, "f2fs_sync_file"):
		return "file_io"
	case strings.HasPrefix(lower, "mm_filemap_") || strings.HasPrefix(lower, "filemap_set_wb_err"):
		return "page_cache"
	case lower == "workqueue_execute_start" || lower == "workqueue_execute_end":
		return "workqueue"
	case strings.HasPrefix(lower, "dma_fence"):
		return "dma_fence"
	default:
		return ""
	}
}

func traceDBRenderRawFtrace(name string, args map[string]traceDBValue) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "binder_"):
		return traceDBRenderRawBinder(name, args)
	case lower == "block_rq_issue" || lower == "block_rq_insert" || lower == "block_rq_complete":
		return traceDBRenderRawBlockRequest(name, args), true
	case lower == "block_bio_remap":
		return traceDBRenderRawBlockRemap(name, args), true
	case strings.HasPrefix(lower, "android_fs_dataread") || strings.HasPrefix(lower, "android_fs_datawrite"):
		return traceDBRenderRawFileIO(name, args, "bytes"), true
	case strings.HasPrefix(lower, "f2fs_direct_io") || strings.HasPrefix(lower, "f2fs_sync_file"):
		return traceDBRenderRawFileIO(name, args, "len"), true
	case strings.HasPrefix(lower, "scsi_dispatch_cmd"):
		return traceDBRenderRawSCSI(name, args), true
	case strings.HasPrefix(lower, "mmc_request_start"):
		return traceDBRenderRawMMCRequestStart(args), true
	case strings.HasPrefix(lower, "mmc_request_done"):
		return traceDBRenderRawMMCRequestDone(args), true
	case strings.HasPrefix(lower, "ufshcd_"):
		return traceDBRenderRawStorageKV(name, args), true
	case strings.HasPrefix(lower, "mm_filemap_") || strings.HasPrefix(lower, "filemap_set_wb_err"):
		return traceDBRenderRawPageCache(name, args), true
	case lower == "workqueue_execute_start" || lower == "workqueue_execute_end":
		return traceDBRenderRawWorkqueue(name, args), true
	case strings.HasPrefix(lower, "dma_fence"):
		return traceDBRenderRawDMAFence(name, args), true
	default:
		return "", false
	}
}

func traceDBRawRequiredArgs(name string, args map[string]traceDBValue, invalidKeys map[string]bool) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	require := func(groups ...[]string) bool {
		for _, names := range groups {
			if _, ok := traceDBRawValidatedAlias(args, invalidKeys, true, names...); !ok {
				return false
			}
		}
		return true
	}
	optional := func(groups ...[]string) bool {
		for _, names := range groups {
			if _, ok := traceDBRawValidatedAlias(args, invalidKeys, false, names...); !ok {
				return false
			}
		}
		return true
	}
	requireAny := func(names ...string) bool {
		found := false
		for _, name := range names {
			key := strings.ToLower(strings.TrimSpace(name))
			if invalidKeys[key] {
				return false
			}
			if value, exists := args[key]; exists {
				if !value.Valid || strings.TrimSpace(value.Text) == "" {
					return false
				}
				found = true
			}
		}
		return found
	}
	switch lower {
	case "binder_transaction":
		return require([]string{"transaction", "debug_id", "transaction_id"}, []string{"dest_node", "target_node"},
			[]string{"dest_proc", "target_proc"}, []string{"dest_thread", "target_thread"}, []string{"reply"}, []string{"flags"}, []string{"code"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 1, math.MaxInt64, "transaction", "debug_id", "transaction_id") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "dest_node", "target_node") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt32, "dest_proc", "target_proc") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt32, "dest_thread", "target_thread") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, 1, "reply") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "flags") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "code")
	case "binder_transaction_received":
		return require([]string{"transaction", "debug_id", "transaction_id"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 1, math.MaxInt64, "transaction", "debug_id", "transaction_id")
	case "binder_transaction_alloc_buf", "binder_alloc_buf":
		return require([]string{"transaction", "debug_id", "transaction_id"}, []string{"data_size"}, []string{"offsets_size"}) &&
			optional([]string{"extra_buffers_size", "extra_size"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 1, math.MaxInt64, "transaction", "debug_id", "transaction_id") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "data_size") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "offsets_size") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "extra_buffers_size", "extra_size")
	case "binder_transaction_reply", "binder_reply":
		return require([]string{"transaction", "debug_id", "transaction_id"}) && optional([]string{"tag"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 1, math.MaxInt64, "transaction", "debug_id", "transaction_id")
	case "binder_transaction_lock", "binder_lock", "binder_transaction_locked", "binder_locked", "binder_transaction_unlock", "binder_unlock":
		return require([]string{"tag"}) && traceDBRawWireTextAlias(args, invalidKeys, true, "tag")
	case "block_rq_issue", "block_rq_insert":
		return require([]string{"dev", "dev_t"}, []string{"sector", "lba"}, []string{"nr_sector", "nr_sectors", "sectors"}) &&
			requireAny("rwbs", "rw", "op") && optional([]string{"rwbs", "rw"}, []string{"cmd", "opcode"}, []string{"comm"}) &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "dev_t") && traceDBRawAnyWireText(args, invalidKeys, "rwbs", "rw", "op") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "sector", "lba") &&
			traceDBRawBlockSectorCountValid(args, invalidKeys)
	case "block_rq_complete":
		return require([]string{"dev", "dev_t"}, []string{"sector", "lba"}, []string{"nr_sector", "nr_sectors", "sectors"},
			[]string{"error", "ret", "res"}) && requireAny("rwbs", "rw", "op") &&
			optional([]string{"rwbs", "rw"}, []string{"cmd", "opcode"}) &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "dev_t") && traceDBRawAnyWireText(args, invalidKeys, "rwbs", "rw", "op") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "sector", "lba") &&
			traceDBRawBlockSectorCountValid(args, invalidKeys) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, math.MinInt64, math.MaxInt64, "error", "ret", "res")
	case "block_bio_remap":
		return require([]string{"dev", "dev_t"}, []string{"sector"}, []string{"nr_sector", "nr_sectors", "sectors"},
			[]string{"old_dev", "from"}, []string{"old_sector", "from_sector"}) &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "dev_t") && traceDBRawDeviceAlias(args, invalidKeys, "old_dev", "from") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "sector") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "nr_sector", "nr_sectors", "sectors") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "old_sector", "from_sector")
	case "mmc_request_start":
		return require([]string{"name", "dev_name"}, []string{"tag"}, []string{"cmd_opcode", "opcode"},
			[]string{"blocks"}, []string{"block_size"}, []string{"blk_addr", "lba"}) &&
			traceDBRawWireTextAlias(args, invalidKeys, true, "name", "dev_name") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "tag") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "cmd_opcode", "opcode") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "blocks") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "block_size") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "blk_addr", "lba")
	case "mmc_request_done":
		return require([]string{"name", "dev_name"}, []string{"tag"}, []string{"cmd_opcode", "opcode"},
			[]string{"bytes_xfered", "bytes", "len"}, []string{"ret", "cmd_err", "data_err"}) &&
			traceDBRawWireTextAlias(args, invalidKeys, true, "name", "dev_name") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "tag") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "cmd_opcode", "opcode") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "bytes_xfered", "bytes", "len") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, math.MinInt64, math.MaxInt64, "ret", "cmd_err", "data_err")
	case "workqueue_execute_start":
		return traceDBRawNonZeroPointer(args, invalidKeys, "work", "addr", "address") &&
			traceDBRawNonZeroPointer(args, invalidKeys, "function", "func")
	case "workqueue_execute_end":
		return traceDBRawNonZeroPointer(args, invalidKeys, "work", "addr", "address") &&
			optional([]string{"function", "func"})
	}
	switch {
	case strings.HasPrefix(lower, "android_fs_dataread"), strings.HasPrefix(lower, "android_fs_datawrite"):
		return require([]string{"dev", "s_dev", "dev_t"}, []string{"ino", "inode", "i_ino"}, []string{"bytes", "len", "length", "size"}) &&
			optional([]string{"entry_name", "name", "file", "filename"}, []string{"offset", "ofs", "pos", "off"},
				[]string{"rw", "rwbs", "op", "operation"}, []string{"ret", "res", "error", "err"},
				[]string{"latency_us", "duration_us", "time_us", "usecs"}) &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "s_dev", "dev_t") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "ino", "inode", "i_ino") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "bytes", "len", "length", "size") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "offset", "ofs", "pos", "off") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret", "res", "error", "err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "latency_us", "duration_us", "time_us", "usecs")
	case strings.HasPrefix(lower, "f2fs_direct_io"), strings.HasPrefix(lower, "f2fs_sync_file"):
		return require([]string{"dev", "s_dev", "dev_t"}, []string{"ino", "inode", "i_ino"}) &&
			optional([]string{"entry_name", "name", "file", "filename"}, []string{"offset", "ofs", "pos", "off"},
				[]string{"bytes", "len", "length", "size"}, []string{"rw", "rwbs", "op", "operation"},
				[]string{"ret", "res", "error", "err"}, []string{"latency_us", "duration_us", "time_us", "usecs"}) &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "s_dev", "dev_t") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "ino", "inode", "i_ino") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "offset", "ofs", "pos", "off") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "bytes", "len", "length", "size") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret", "res", "error", "err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "latency_us", "duration_us", "time_us", "usecs")
	case strings.HasPrefix(lower, "scsi_dispatch_cmd"):
		return require([]string{"tag"}, []string{"dev", "sdev", "dev_t"}, []string{"lba", "sector"},
			[]string{"len", "length", "bytes", "transfer_len"}) && requireAny("opcode", "op", "rw", "rwbs") &&
			optional([]string{"ret", "res", "error", "err"}, []string{"latency_us", "duration_us", "time_us", "usecs"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "tag") &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "sdev", "dev_t") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "lba", "sector") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "len", "length", "bytes", "transfer_len") &&
			traceDBRawAnyWireText(args, invalidKeys, "opcode", "op", "rw", "rwbs") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret", "res", "error", "err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "latency_us", "duration_us", "time_us", "usecs")
	case strings.HasPrefix(lower, "ufshcd_"):
		return require([]string{"tag"}) && requireAny("opcode", "op", "rw", "rwbs") &&
			optional([]string{"dev", "dev_name", "devname"}, []string{"lba", "sector"},
				[]string{"len", "length", "bytes", "transfer_len", "size"}, []string{"ret", "res", "error", "err"},
				[]string{"latency_us", "duration_us", "time_us", "usecs"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "tag") &&
			traceDBRawAnyWireText(args, invalidKeys, "opcode", "op", "rw", "rwbs") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "lba", "sector") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "len", "length", "bytes", "transfer_len", "size") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret", "res", "error", "err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "latency_us", "duration_us", "time_us", "usecs")
	case strings.HasPrefix(lower, "mm_filemap_"), strings.HasPrefix(lower, "filemap_set_wb_err"):
		return require([]string{"dev", "s_dev", "dev_t"}, []string{"ino", "inode", "i_ino"}) &&
			optional([]string{"entry_name", "name", "file", "filename"}, []string{"offset", "ofs", "pos", "index"},
				[]string{"bytes", "len", "length", "size"}) &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "s_dev", "dev_t") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "ino", "inode", "i_ino") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "offset", "ofs", "pos", "index") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "bytes", "len", "length", "size")
	case strings.HasPrefix(lower, "dma_fence"):
		return require([]string{"driver"}, []string{"timeline"}, []string{"context"}, []string{"seqno"}) &&
			traceDBRawWireTextAlias(args, invalidKeys, true, "driver") &&
			traceDBRawWireTextAlias(args, invalidKeys, true, "timeline") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "context") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "seqno")
	}
	return false
}

func traceDBRawValidatedAlias(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) (string, bool) {
	valueText := ""
	var datatype int64
	found := false
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || invalidKeys[key] {
			return "", false
		}
		value, exists := args[key]
		if !exists {
			continue
		}
		text := strings.TrimSpace(value.Text)
		if !value.Valid || text == "" || value.Text != text {
			return "", false
		}
		if found && (text != valueText || value.Datatype != datatype) {
			return "", false
		}
		valueText = text
		datatype = value.Datatype
		found = true
	}
	if required && !found {
		return "", false
	}
	return valueText, true
}

func traceDBRawIntegerAlias(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, minValue, maxValue int64, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, required, names...)
	if !ok || text == "" {
		return ok && !required
	}
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists && value.Datatype != 0 {
			return false
		}
	}
	value, err := strconv.ParseInt(text, 10, 64)
	return err == nil && value >= minValue && value <= maxValue
}

func traceDBRawWireTextAlias(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, required, names...)
	if !ok || text == "" {
		return ok && !required
	}
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists && value.Datatype != 1 {
			return false
		}
	}
	return !strings.ContainsAny(text, " \t\r\n=")
}

func traceDBRawAnyWireText(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) bool {
	found := false
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if invalidKeys[key] {
			return false
		}
		value, exists := args[key]
		if !exists {
			continue
		}
		text := strings.TrimSpace(value.Text)
		if !value.Valid || value.Datatype != 1 || text == "" || value.Text != text || strings.ContainsAny(text, " \t\r\n=") {
			return false
		}
		found = true
	}
	return found
}

func traceDBRawDeviceAlias(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, true, names...)
	if !ok {
		return false
	}
	var datatype int64 = -1
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists {
			datatype = value.Datatype
			break
		}
	}
	if datatype == 0 {
		value, err := strconv.ParseInt(text, 10, 64)
		return err == nil && value >= 0
	}
	if datatype != 1 || strings.ContainsAny(text, " \t\r\n=") {
		return false
	}
	separator := ""
	if strings.Count(text, ":") == 1 && !strings.Contains(text, ",") {
		separator = ":"
	} else if strings.Count(text, ",") == 1 && !strings.Contains(text, ":") {
		separator = ","
	}
	if separator == "" {
		_, err := strconv.ParseUint(text, 10, 64)
		return err == nil
	}
	major, minor, _ := strings.Cut(text, separator)
	_, majorErr := strconv.ParseUint(major, 10, 32)
	_, minorErr := strconv.ParseUint(minor, 10, 32)
	return majorErr == nil && minorErr == nil
}

func traceDBRawBlockSectorCountValid(args map[string]traceDBValue, invalidKeys map[string]bool) bool {
	if !traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "nr_sector", "nr_sectors", "sectors") {
		return false
	}
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, true, "nr_sector", "nr_sectors", "sectors")
	if !ok {
		return false
	}
	count, err := strconv.ParseInt(text, 10, 64)
	if err != nil || count < 0 {
		return false
	}
	if count > 0 {
		return true
	}
	operation := strings.ToUpper(traceDBRawArg(args, "", "rwbs", "rw", "op"))
	return strings.Contains(operation, "F")
}

func traceDBRawNonZeroPointer(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, true, names...)
	if !ok {
		return false
	}
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists && value.Datatype != 0 {
			return false
		}
	}
	base := 10
	valueText := text
	if strings.HasPrefix(valueText, "0x") || strings.HasPrefix(valueText, "0X") {
		base = 16
		valueText = valueText[2:]
	}
	value, err := strconv.ParseUint(valueText, base, 64)
	return err == nil && value > 0
}

func traceDBRenderRawBinder(name string, args map[string]traceDBValue) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "binder_transaction":
		return fmt.Sprintf("%s: transaction=%s dest_node=%s dest_proc=%s dest_thread=%s reply=%s flags=%s code=%s",
			name,
			traceDBRawArg(args, "0", "transaction", "debug_id", "transaction_id"),
			traceDBRawArg(args, "0", "dest_node", "target_node"),
			traceDBRawArg(args, "0", "dest_proc", "target_proc"),
			traceDBRawArg(args, "0", "dest_thread", "target_thread"),
			traceDBRawArg(args, "0", "reply"),
			traceDBRawHexArg(args, "0x0", "flags"),
			traceDBRawHexArg(args, "0x0", "code")), true
	case "binder_transaction_received":
		return fmt.Sprintf("%s: transaction=%s", name, traceDBRawArg(args, "0", "transaction", "debug_id", "transaction_id")), true
	case "binder_transaction_alloc_buf", "binder_alloc_buf":
		parts := []string{
			name + ":",
			"transaction=" + traceDBRawArg(args, "", "transaction", "debug_id", "transaction_id"),
			"debug_id=" + traceDBRawArg(args, "", "debug_id", "transaction"),
			"data_size=" + traceDBRawArg(args, "", "data_size"),
			"offsets_size=" + traceDBRawArg(args, "", "offsets_size"),
		}
		if extra := traceDBRawArg(args, "", "extra_buffers_size", "extra_size"); extra != "" {
			parts = append(parts, "extra_buffers_size="+extra)
		}
		return strings.Join(parts, " "), true
	case "binder_transaction_reply", "binder_reply":
		parts := []string{
			name + ":",
			"transaction=" + traceDBRawArg(args, "", "transaction", "debug_id", "transaction_id"),
			"debug_id=" + traceDBRawArg(args, "", "debug_id", "transaction"),
		}
		if tag := traceDBRawArg(args, "", "tag"); tag != "" {
			parts = append(parts, "tag="+tag)
		}
		return strings.Join(parts, " "), true
	case "binder_transaction_lock", "binder_lock", "binder_transaction_locked", "binder_locked", "binder_transaction_unlock", "binder_unlock":
		return fmt.Sprintf("%s: tag=%s", name, traceDBRawArg(args, "", "tag")), true
	default:
		return "", false
	}
}

func traceDBRenderRawBlockRequest(name string, args map[string]traceDBValue) string {
	dev := traceDBRawDevArg(args, ",", "dev", "dev_t")
	op := traceDBRawArg(args, "RW", "rwbs", "rw", "op", "cmd")
	sector := traceDBRawArg(args, "0", "sector", "lba")
	blocks := traceDBRawArg(args, "0", "nr_sector", "nr_sectors", "sectors", "len", "length", "bytes", "nr_bytes")
	cmd := traceDBRawArg(args, "", "cmd", "opcode")
	if strings.EqualFold(name, "block_rq_complete") {
		errText := traceDBRawArg(args, "0", "error", "ret", "res")
		return fmt.Sprintf("%s: %s %s (%s) %s + %s [%s]", name, dev, op, cmd, sector, blocks, errText)
	}
	return fmt.Sprintf("%s: %s %s 0 (%s) %s + %s [%s]", name, dev, op, cmd, sector, blocks, traceDBRawArg(args, "", "comm"))
}

func traceDBRenderRawBlockRemap(name string, args map[string]traceDBValue) string {
	dev := traceDBRawDevArg(args, ",", "dev", "dev_t")
	sector := traceDBRawArg(args, "0", "sector")
	blocks := traceDBRawArg(args, "0", "nr_sector", "nr_sectors", "len", "length")
	oldDev := traceDBRawDevArg(args, ",", "old_dev", "from")
	oldSector := traceDBRawArg(args, "0", "old_sector", "from_sector")
	return fmt.Sprintf("%s: %s %s + %s <- (%s) %s", name, dev, sector, blocks, oldDev, oldSector)
}

func traceDBRenderRawFileIO(name string, args map[string]traceDBValue, sizeKey string) string {
	parts := []string{name + ":"}
	parts = appendRawKV(parts, "dev", traceDBRawDevArg(args, ":", "dev", "s_dev", "dev_t"))
	parts = appendRawKV(parts, "ino", traceDBRawArg(args, "", "ino", "inode", "i_ino"))
	parts = appendRawKV(parts, "entry_name", traceDBRawArg(args, "", "entry_name", "name", "file", "filename"))
	parts = appendRawKV(parts, "offset", traceDBRawArg(args, "", "offset", "ofs", "pos", "off"))
	parts = appendRawKV(parts, sizeKey, traceDBRawArg(args, "", "bytes", "len", "length", "size"))
	parts = appendRawKV(parts, "rw", firstNonEmpty(traceDBRawArg(args, "", "rw", "rwbs", "op", "operation"), traceIOOperationFromName(name)))
	parts = appendRawKV(parts, "ret", traceDBRawArg(args, "", "ret", "res", "error", "err"))
	parts = appendRawKV(parts, "latency_us", traceDBRawArg(args, "", "latency_us", "duration_us", "time_us", "usecs"))
	return strings.Join(parts, " ")
}

func traceDBRenderRawSCSI(name string, args map[string]traceDBValue) string {
	parts := []string{name + ":"}
	parts = appendRawKV(parts, "tag", traceDBRawArg(args, "", "tag"))
	parts = appendRawKV(parts, "dev", traceDBRawDevArg(args, ":", "dev", "sdev", "dev_t"))
	parts = appendRawKV(parts, "lba", traceDBRawArg(args, "", "lba", "sector"))
	parts = appendRawKV(parts, "len", traceDBRawArg(args, "", "len", "length", "bytes", "transfer_len"))
	parts = appendRawKV(parts, "opcode", traceDBRawArg(args, "", "opcode", "op", "rw", "rwbs"))
	parts = appendRawKV(parts, "ret", traceDBRawArg(args, "", "ret", "res", "error", "err"))
	parts = appendRawKV(parts, "latency_us", traceDBRawArg(args, "", "latency_us", "duration_us", "time_us", "usecs"))
	return strings.Join(parts, " ")
}

func traceDBRenderRawMMCRequestStart(args map[string]traceDBValue) string {
	name := traceDBRawArg(args, "mmc0", "name", "dev_name")
	return fmt.Sprintf("mmc_request_start: %s tag=%s opcode=%s blocks=%s block_size=%s blk_addr=%s",
		name,
		traceDBRawArg(args, "0", "tag"),
		traceDBRawArg(args, "0", "cmd_opcode", "opcode"),
		traceDBRawArg(args, "0", "blocks"),
		traceDBRawArg(args, "0", "block_size"),
		traceDBRawArg(args, "0", "blk_addr", "lba"))
}

func traceDBRenderRawMMCRequestDone(args map[string]traceDBValue) string {
	name := traceDBRawArg(args, "mmc0", "name", "dev_name")
	return fmt.Sprintf("mmc_request_done: %s tag=%s opcode=%s bytes_xfered=%s ret=%s",
		name,
		traceDBRawArg(args, "0", "tag"),
		traceDBRawArg(args, "0", "cmd_opcode", "opcode"),
		traceDBRawArg(args, "0", "bytes_xfered", "bytes", "len"),
		traceDBRawArg(args, "0", "ret", "cmd_err", "data_err"))
}

func traceDBRenderRawStorageKV(name string, args map[string]traceDBValue) string {
	parts := []string{name + ":"}
	for _, item := range []struct {
		key   string
		names []string
	}{
		{"dev", []string{"dev", "dev_name", "devname"}},
		{"tag", []string{"tag"}},
		{"lba", []string{"lba", "sector"}},
		{"len", []string{"len", "length", "bytes", "transfer_len", "size"}},
		{"opcode", []string{"opcode", "op", "rw", "rwbs"}},
		{"ret", []string{"ret", "res", "error", "err"}},
		{"latency_us", []string{"latency_us", "duration_us", "time_us", "usecs"}},
	} {
		parts = appendRawKV(parts, item.key, traceDBRawArg(args, "", item.names...))
	}
	return strings.Join(parts, " ")
}

func traceDBRenderRawPageCache(name string, args map[string]traceDBValue) string {
	parts := []string{"dev=" + traceDBRawDevArg(args, ":", "dev", "s_dev", "dev_t")}
	parts = appendRawKV(parts, "ino", traceDBRawArg(args, "", "ino", "inode", "i_ino"))
	parts = appendRawKV(parts, "entry_name", traceDBRawArg(args, "", "entry_name", "name", "file", "filename"))
	parts = appendRawKV(parts, "offset", traceDBRawArg(args, "", "offset", "ofs", "pos", "index"))
	parts = appendRawKV(parts, "bytes", traceDBRawArg(args, "", "bytes", "len", "length", "size"))
	return name + ": " + strings.Join(parts, " ")
}

func traceDBRenderRawWorkqueue(name string, args map[string]traceDBValue) string {
	parts := []string{name + ":", "work=" + traceDBRawHexArg(args, "", "work", "addr", "address")}
	if function := traceDBRawHexArg(args, "", "function", "func"); function != "" {
		parts = append(parts, "function="+function)
	}
	return strings.Join(parts, " ")
}

func traceDBRenderRawDMAFence(name string, args map[string]traceDBValue) string {
	parts := []string{}
	parts = appendRawKV(parts, "driver", traceDBRawArg(args, "", "driver"))
	parts = appendRawKV(parts, "timeline", traceDBRawArg(args, "", "timeline"))
	parts = appendRawKV(parts, "context", traceDBRawArg(args, "", "context"))
	parts = appendRawKV(parts, "seqno", traceDBRawArg(args, "", "seqno"))
	return name + ": " + strings.Join(parts, " ")
}

func appendRawKV(parts []string, key, value string) []string {
	if strings.TrimSpace(value) == "" {
		return parts
	}
	return append(parts, key+"="+value)
}

func traceDBRawArg(args map[string]traceDBValue, fallback string, names ...string) string {
	for _, name := range names {
		if value := traceDBRawArgExact(args, name); value != "" {
			return value
		}
	}
	return fallback
}

func traceDBRawArgExact(args map[string]traceDBValue, name string) string {
	value := args[strings.ToLower(strings.TrimSpace(name))]
	if value.Valid {
		return strings.TrimSpace(value.Text)
	}
	return ""
}

func traceDBRawHexArg(args map[string]traceDBValue, fallback string, names ...string) string {
	text := traceDBRawArg(args, "", names...)
	if text == "" {
		return fallback
	}
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		return text
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return fmt.Sprintf("0x%x", value)
	}
	return text
}

func traceDBRawDevArg(args map[string]traceDBValue, sep string, names ...string) string {
	text := traceDBRawArg(args, "", names...)
	if text == "" {
		return ""
	}
	if strings.ContainsAny(text, ":,") {
		return text
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return devMajorMinor(value, sep)
	}
	return text
}

func traceDBRawLineContext(raw traceDBRawEvent, index traceDBThreadIndex) (string, int64, int64, int64, bool) {
	var thread traceDBThread
	threadKnown := false
	if raw.ITIDKnown {
		if raw.ITID == 0 {
			if raw.TIDKnown || raw.PIDKnown {
				return "", 0, 0, -1, false
			}
			return "swapper", 0, 0, 0, true
		}
		candidate, ok := index.ByITID[raw.ITID]
		if !ok || index.AmbiguousITID[raw.ITID] || !traceDBRawThreadIdentityValid(candidate) || raw.TS < candidate.StartTS {
			return "", 0, 0, -1, false
		}
		thread, threadKnown = candidate, true
	}
	if raw.TIDKnown {
		if len(index.ByTIDIncarnation[raw.TID]) == 0 {
			if threadKnown {
				return "", 0, 0, -1, false
			}
			tgid := raw.TID
			if raw.PIDKnown {
				tgid = raw.PID
			}
			return "<raw>", raw.TID, tgid, -1, true
		}
		candidate, ok := traceDBRawThreadAt(index, raw.TID, raw.TS, raw.PID, raw.PIDKnown)
		if !ok || (threadKnown && candidate.ITID != thread.ITID) {
			return "", 0, 0, -1, false
		}
		thread, threadKnown = candidate, true
	}
	if threadKnown {
		process, processKnown := index.Processes[thread.IPID]
		if index.AmbiguousIPID[thread.IPID] {
			return "", 0, 0, -1, false
		}
		if processKnown && (process.PID <= 0 || process.PID > math.MaxInt32) {
			return "", 0, 0, -1, false
		}
		if raw.PIDKnown && (!processKnown || process.PID != raw.PID) {
			return "", 0, 0, -1, false
		}
		tgid := thread.TID
		if processKnown {
			tgid = process.PID
		}
		name := thread.Name
		if _, ok := traceDBStrictArgText(name, true); !ok {
			name = "unknown"
		}
		return traceDBCommName(name, "unknown"), thread.TID, tgid, thread.ITID, true
	}
	// PID identifies a process, never the emitting thread.  Thread-scoped raw
	// families must not be projected onto PID-as-TID or pid0/swapper.
	return "", 0, 0, -1, false
}

func traceDBRawThreadIdentityValid(thread traceDBThread) bool {
	return thread.ITID > 0 && thread.ITID <= maxTraceDBInternalID &&
		thread.TID > 0 && thread.TID <= math.MaxInt32 && thread.IPID >= 0 &&
		thread.IPID <= maxTraceDBInternalID && thread.StartTS >= 0
}

func traceDBRawThreadAt(index traceDBThreadIndex, tid, ts, pid int64, pidKnown bool) (traceDBThread, bool) {
	items := index.ByTIDIncarnation[tid]
	latestStart := int64(math.MinInt64)
	var candidates []traceDBThread
	for _, item := range items {
		if !traceDBRawThreadIdentityValid(item) || item.StartTS > ts || item.StartTS < latestStart {
			continue
		}
		if item.StartTS > latestStart {
			latestStart = item.StartTS
			candidates = candidates[:0]
		}
		candidates = append(candidates, item)
	}
	if pidKnown {
		filtered := candidates[:0]
		for _, item := range candidates {
			if process, ok := index.Processes[item.IPID]; ok && process.PID > 0 && process.PID <= math.MaxInt32 &&
				process.PID == pid && !index.AmbiguousIPID[item.IPID] {
				filtered = append(filtered, item)
			}
		}
		candidates = filtered
	}
	if len(candidates) != 1 || index.AmbiguousITID[candidates[0].ITID] {
		return traceDBThread{}, false
	}
	return candidates[0], true
}
