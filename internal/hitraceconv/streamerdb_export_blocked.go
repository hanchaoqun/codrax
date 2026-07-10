package hitraceconv

import (
	"context"
	"database/sql"
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
	Duplicate   bool
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
	Matches       map[traceDBBlockedBoundaryKey][]traceDBBlockedSchedSlice
	OverflowITIDs map[int64]bool
}

func exportTraceDBBlockedReasons(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex) (TraceDBCoverage, error) {
	coverage := TraceDBCoverage{
		Family: "scheduler",
		Table:  "thread_state.arg_setid",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"caller":        "args.caller_str|args.caller; modern_4002_safe_token_gate",
			"caller_raw":    "args.caller_raw|hex_address(args.caller)",
			"delay":         "args.delay",
			"header_cpu":    "unique_same_itid_sched_slice.cpu_where_slice.ts+slice.dur==thread_state.ts",
			"header_thread": "thread_state_subject_projection; original_header_thread_known=false",
			"iowait":        "args.iowait; io_wait_compat_alias; verified_against_split_D/DK_state_when_present; authoritative_for_other_nonempty_states",
			"pid":           "thread_state.tid; verified_against_thread.tid_and_optional_args.pid",
			"source":        "thread_state_argset",
			"timestamp":     "thread_state.ts_projection; original_timestamp_known=false",
		},
	}

	threadCoverage, err := tdb.inspectCoverage(ctx, "scheduler.blocked_reason.schema", "thread_state",
		[]string{"id", "ts", "dur", "itid", "tid", "pid", "state", "arg_setid"})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	argsCoverage, err := tdb.inspectCoverage(ctx, "scheduler.blocked_reason.schema", "args",
		[]string{"id", "key", "datatype", "value", "argset"})
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

	argsets, err := loadTraceDBBlockedArgsets(ctx, tdb)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	skipped := map[string]int{}
	prepared, candidateStarts, rowsRead, err := loadTraceDBBlockedCandidates(ctx, tdb, index, argsets, skipped)
	coverage.RowsRead = rowsRead
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	boundaries, err := loadTraceDBBlockedSchedBoundaries(ctx, tdb, candidateStarts)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	for _, candidate := range prepared {
		row := candidate.Row
		key := traceDBBlockedBoundaryKey{ITID: row.ITID, StateStart: row.TS}
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
		if err := sink.add(renderedRow{
			tsNS: uint64(row.TS),
			seq:  sink.stats.RowsAccepted,
			line: traceDBFormatLine(traceDBCommName(candidate.Thread.Name, "unknown"), row.TID,
				firstNonZero(candidate.Process.PID, row.PID, row.TID), headerCPU, row.TS, body),
		}); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	coverage.Skipped = traceDBBlockedSkipSummary(skipped)
	return coverage, nil
}

func loadTraceDBBlockedCandidates(ctx context.Context, tdb *traceDB, index traceDBThreadIndex, argsets map[int64]traceDBBlockedArgset, skipped map[string]int) ([]traceDBPreparedBlockedReason, map[int64]map[int64]bool, int, error) {
	var prepared []traceDBPreparedBlockedReason
	candidateStarts := map[int64]map[int64]bool{}
	rowsRead := 0
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT id, ts, dur, itid, tid, pid, state, arg_setid
		FROM thread_state
		WHERE arg_setid IS NOT NULL
		ORDER BY id
	`)
	if err != nil {
		return prepared, candidateStarts, rowsRead, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return prepared, candidateStarts, rowsRead, err
		}
		var id, ts, dur, itid, tid, pid, argSetID sql.NullInt64
		var state sql.NullString
		if err := rows.Scan(&id, &ts, &dur, &itid, &tid, &pid, &state, &argSetID); err != nil {
			return prepared, candidateStarts, rowsRead, err
		}
		args, argsFound := argsets[argSetID.Int64]
		_, blockedState := traceDBBlockedStateIOWait(state.String)
		if !blockedState && !(argsFound && args.HasRelevant) {
			continue
		}
		rowsRead++
		if !id.Valid || !ts.Valid || !dur.Valid || !itid.Valid || !tid.Valid || !pid.Valid ||
			!state.Valid || !argSetID.Valid || id.Int64 < 0 || ts.Int64 < 0 || dur.Int64 < 0 ||
			itid.Int64 < 0 || tid.Int64 <= 0 || pid.Int64 < 0 || argSetID.Int64 < 0 ||
			strings.TrimSpace(state.String) == "" {
			skipped["invalid_thread_state_metadata"]++
			continue
		}
		row := traceDBBlockedStateRow{
			ID: id.Int64, TS: ts.Int64, Dur: dur.Int64, ITID: itid.Int64,
			TID: tid.Int64, PID: pid.Int64, State: state.String, ArgSetID: argSetID.Int64,
		}
		if row.TS > math.MaxInt64-row.Dur {
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
		expectedIOWait, stateOK := traceDBBlockedStateIOWait(row.State)
		if !argsFound || !args.HasRelevant {
			skipped["missing_argset_rows"]++
			continue
		}
		if args.Duplicate {
			skipped["duplicate_blocked_arg"]++
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
		thread, ok := index.ByITID[row.ITID]
		if !ok {
			skipped["missing_thread_identity"]++
			continue
		}
		if thread.TID != row.TID {
			skipped["thread_tid_mismatch"]++
			continue
		}
		process := index.Processes[thread.IPID]
		if process.PID > 0 && row.PID > 0 && process.PID != row.PID {
			skipped["thread_tgid_mismatch"]++
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
		if delayPresent && (!delayValid || delay < 0 || delay > math.MaxUint32) {
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
		return prepared, candidateStarts, rowsRead, err
	}
	return prepared, candidateStarts, rowsRead, nil
}

func loadTraceDBBlockedArgsets(ctx context.Context, tdb *traceDB) (map[int64]traceDBBlockedArgset, error) {
	out := map[int64]traceDBBlockedArgset{}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT a.argset, key_dict.data, a.datatype, a.value, value_dict.data
		FROM args a
		LEFT JOIN data_dict key_dict ON key_dict.id = a.key
		LEFT JOIN data_dict value_dict ON value_dict.id = a.value
		WHERE key_dict.data IN ('iowait', 'io_wait', 'caller', 'caller_str', 'caller_raw', 'delay', 'pid')
		ORDER BY a.argset, a.id
	`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		var argSetID, dataType, numeric sql.NullInt64
		var key, text sql.NullString
		if err := rows.Scan(&argSetID, &key, &dataType, &numeric, &text); err != nil {
			return out, err
		}
		if !argSetID.Valid || argSetID.Int64 < 0 || !key.Valid || !traceDBBlockedRelevantArgKey(key.String) {
			continue
		}
		item := out[argSetID.Int64]
		if item.Values == nil {
			item.Values = map[string]traceDBBlockedArg{}
		}
		item.HasRelevant = true
		if _, exists := item.Values[key.String]; exists {
			item.Duplicate = true
			out[argSetID.Int64] = item
			continue
		}
		arg := traceDBBlockedArg{}
		if dataType.Valid {
			arg.DataType = dataType.Int64
			switch dataType.Int64 {
			case traceDBArgTypeInt:
				if numeric.Valid {
					arg.Int = numeric.Int64
					arg.Valid = true
				}
			case traceDBArgTypeString:
				if text.Valid {
					arg.Text = text.String
					arg.Valid = true
				}
			}
		}
		item.Values[key.String] = arg
		out[argSetID.Int64] = item
	}
	return out, rows.Err()
}

func loadTraceDBBlockedSchedBoundaries(ctx context.Context, tdb *traceDB, candidateStarts map[int64]map[int64]bool) (traceDBBlockedBoundaryIndex, error) {
	out := traceDBBlockedBoundaryIndex{
		Matches:       map[traceDBBlockedBoundaryKey][]traceDBBlockedSchedSlice{},
		OverflowITIDs: map[int64]bool{},
	}
	if len(candidateStarts) == 0 {
		return out, nil
	}
	// Stream sched_slice exactly once and retain only exact candidate boundaries.
	// Production traces can have millions of slices; copying that table merely
	// to recover a small blocked-reason header set would make conversion memory
	// scale with the trace instead of with the recovered evidence.
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT itid, ts, dur, cpu
		FROM sched_slice
		WHERE itid IS NOT NULL AND ts IS NOT NULL AND dur IS NOT NULL
	`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		var itid, ts, dur, cpu sql.NullInt64
		if err := rows.Scan(&itid, &ts, &dur, &cpu); err != nil {
			return out, err
		}
		if !itid.Valid || !ts.Valid || !dur.Valid || itid.Int64 < 0 || ts.Int64 < 0 || dur.Int64 < 0 {
			continue
		}
		starts, relevantITID := candidateStarts[itid.Int64]
		if !relevantITID {
			continue
		}
		if ts.Int64 > math.MaxInt64-dur.Int64 {
			out.OverflowITIDs[itid.Int64] = true
			continue
		}
		end := ts.Int64 + dur.Int64
		if !starts[end] {
			continue
		}
		item := traceDBBlockedSchedSlice{TS: ts.Int64, Dur: dur.Int64, CPU: -1}
		if cpu.Valid {
			item.CPU = cpu.Int64
		}
		key := traceDBBlockedBoundaryKey{ITID: itid.Int64, StateStart: end}
		out.Matches[key] = append(out.Matches[key], item)
	}
	return out, rows.Err()
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

func traceDBBlockedStateIOWait(state string) (int64, bool) {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "D-IO", "DK-IO":
		return 1, true
	case "D-NIO", "DK-NIO":
		return 0, true
	default:
		return 0, false
	}
}

func traceDBBlockedStateUnsplit(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
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
