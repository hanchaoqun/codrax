package hitraceconv

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type traceDBRawEvent struct {
	TS       int64
	Name     string
	CPU      int64
	ITID     int64
	TID      int64
	PID      int64
	ArgSetID int64
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
	cpuExpr, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "0", "cpu")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	itidExpr, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "0", "itid", "callid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	tidExpr, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "0", "tid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	pidExpr, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "0", "pid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	query := fmt.Sprintf(`
		SELECT ts, name, %s, %s, %s, %s, COALESCE(%s, 0)
		FROM raw
		WHERE ts IS NOT NULL AND name IS NOT NULL
		ORDER BY ts
	`, cpuExpr, itidExpr, tidExpr, pidExpr, traceDBQuoteIdent(argsetColumn))
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	defer rows.Close()
	classCoverage := map[string]*TraceDBCoverage{}
	unsupported := TraceDBCoverage{Family: "raw_ftrace", Table: "unsupported", Found: true, Skipped: "unsupported raw ftrace event family"}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return outCoverage, err
		}
		var raw traceDBRawEvent
		var tsRaw, nameRaw, cpuRaw, itidRaw, tidRaw, pidRaw, argsetRaw any
		if err := rows.Scan(&tsRaw, &nameRaw, &cpuRaw, &itidRaw, &tidRaw, &pidRaw, &argsetRaw); err != nil {
			schemaCoverage.Error = err.Error()
			return []TraceDBCoverage{schemaCoverage}, err
		}
		raw.TS = traceDBInt64FromAny(tsRaw, 0)
		raw.Name = traceDBAnyText(nameRaw, "")
		raw.CPU = traceDBInt64FromAny(cpuRaw, 0)
		raw.ITID = traceDBInt64FromAny(itidRaw, 0)
		raw.TID = traceDBInt64FromAny(tidRaw, 0)
		raw.PID = traceDBInt64FromAny(pidRaw, 0)
		raw.ArgSetID = traceDBInt64FromAny(argsetRaw, 0)
		schemaCoverage.RowsRead++
		class := traceDBRawFtraceClass(raw.Name)
		if class == "" {
			unsupported.RowsRead++
			continue
		}
		item := traceDBRawClassCoverage(classCoverage, class)
		item.RowsRead++
		args := argsets[raw.ArgSetID]
		body, ok := traceDBRenderRawFtrace(raw.Name, args)
		if !ok {
			continue
		}
		task, tid, tgid := traceDBRawLineContext(raw, args, index)
		if raw.CPU == 0 && raw.ITID != 0 {
			raw.CPU = traceDBCPUAt(running, raw.ITID, raw.TS, raw.CPU)
		}
		if err := addTraceDBInstantRow(sink, raw.TS, task, tid, tgid, raw.CPU, body); err != nil {
			return outCoverage, err
		}
		item.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	outCoverage[0] = schemaCoverage
	for _, key := range sortedRawFtraceCoverageKeys(classCoverage) {
		outCoverage = append(outCoverage, *classCoverage[key])
	}
	if unsupported.RowsRead > 0 {
		outCoverage = append(outCoverage, unsupported)
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

func traceDBRawOptionalExpr(ctx context.Context, tdb *traceDB, table, fallback string, names ...string) (string, error) {
	name, ok, err := traceDBFirstExistingColumn(ctx, tdb, table, names...)
	if err != nil {
		return "", err
	}
	if !ok {
		return fallback, nil
	}
	return "COALESCE(" + traceDBQuoteIdent(name) + ", " + fallback + ")", nil
}

func traceDBQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func traceDBRawClassCoverage(items map[string]*TraceDBCoverage, class string) *TraceDBCoverage {
	item := items[class]
	if item != nil {
		return item
	}
	item = &TraceDBCoverage{Family: "raw_ftrace", Table: class, Found: true}
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
	case strings.HasPrefix(lower, "workqueue_"):
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
	case strings.HasPrefix(lower, "workqueue_"):
		return traceDBRenderRawWorkqueue(name, args), true
	case strings.HasPrefix(lower, "dma_fence"):
		return traceDBRenderRawDMAFence(name, args), true
	default:
		return "", false
	}
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
		return fmt.Sprintf("%s: transaction=%s debug_id=%s data_size=%s offsets_size=%s extra_buffers_size=%s",
			name,
			traceDBRawArg(args, "0", "transaction", "debug_id", "transaction_id"),
			traceDBRawArg(args, "0", "debug_id", "transaction"),
			traceDBRawArg(args, "0", "data_size"),
			traceDBRawArg(args, "0", "offsets_size"),
			traceDBRawArg(args, "0", "extra_buffers_size", "extra_size")), true
	case "binder_transaction_reply", "binder_reply", "binder_transaction_lock", "binder_lock", "binder_transaction_locked", "binder_locked", "binder_transaction_unlock", "binder_unlock":
		return fmt.Sprintf("%s: transaction=%s debug_id=%s tag=%s",
			name,
			traceDBRawArg(args, "0", "transaction", "debug_id", "transaction_id"),
			traceDBRawArg(args, "0", "debug_id", "transaction"),
			traceDBRawArg(args, "", "tag", "lock", "name")), true
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
	parts = appendRawKV(parts, "ino", traceDBRawArg(args, "", "ino", "inode", "i_ino", "index"))
	parts = appendRawKV(parts, "entry_name", traceDBRawArg(args, "", "entry_name", "name", "file", "filename"))
	parts = appendRawKV(parts, "offset", traceDBRawArg(args, "", "offset", "ofs", "pos", "index"))
	parts = appendRawKV(parts, "bytes", traceDBRawArg(args, "", "bytes", "len", "length", "size"))
	return name + ": " + strings.Join(parts, " ")
}

func traceDBRenderRawWorkqueue(name string, args map[string]traceDBValue) string {
	return fmt.Sprintf("%s: work=%s function=%s",
		name,
		traceDBRawHexArg(args, "0x0", "work", "addr", "address"),
		traceDBRawHexArg(args, "0x0", "function", "func"))
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
	for key, value := range args {
		if !value.Valid || !strings.EqualFold(strings.TrimSpace(key), name) {
			continue
		}
		if text := strings.TrimSpace(value.Text); text != "" {
			return text
		}
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

func traceDBRawLineContext(raw traceDBRawEvent, args map[string]traceDBValue, index traceDBThreadIndex) (string, int64, int64) {
	if raw.ITID != 0 {
		return traceDBThreadLineContext(index, raw.ITID)
	}
	if raw.TID != 0 {
		return traceDBThreadLineContextByTID(index, raw.TID)
	}
	if raw.PID != 0 {
		return traceDBThreadLineContextByTID(index, raw.PID)
	}
	if pid := traceDBInt64FromAny(traceDBRawArg(args, "", "pid", "tid"), 0); pid != 0 {
		return traceDBThreadLineContextByTID(index, pid)
	}
	return "unknown", 0, 0
}

func traceDBThreadLineContextByTID(index traceDBThreadIndex, tid int64) (string, int64, int64) {
	thread, ok := index.ByTID[tid]
	if ok {
		process := index.Processes[thread.IPID]
		return traceDBCommName(thread.Name, "unknown"), thread.TID, firstNonZero(process.PID, thread.TID)
	}
	for _, process := range index.Processes {
		if process.PID == tid {
			return traceDBCommName(process.Name, "unknown"), process.PID, process.PID
		}
	}
	return "unknown", tid, tid
}
