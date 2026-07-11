package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// frame_slice is an analytical frame interval, not a thread call stack.  Its
// intervals may overlap or cross, so exporting them as B/E would corrupt the
// shared per-emitter nesting stack.  A stable source-row identity provides an
// independent async S/F lane for every frame.
type traceDBFrameSliceRow struct {
	StableID int64
	TS       int64
	End      int64
	ITID     int64
	IPID     int64
	Task     string
	TID      int64
	TGID     int64
	StartCPU int64
	EndCPU   int64
	Name     string
	Cookie   string
}

func exportTraceDBFrameSlice(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, running map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "frame_slice", []string{"ts", "dur", "type_desc", "vsync", "flag", "ipid", "itid"})
	coverage.FieldSources = map[string]string{
		"stable_identity": "frame_slice.id when present; legacy materialized DBs may use a provable SQLite hidden rowid only when id is absent",
		"schema_profile":  "closed profiles: current=id+type+flag; legacy=no id/type plus required flag",
		"frame_kind":      "closed producer enum: type=0/type_desc=actural or type=1/type_desc=expect; legacy schemas without type retain the same closed type_desc vocabulary",
		"interval":        "strict integer ts and positive dur with checked signed addition; sub-microsecond or same-rendered-timestamp intervals fail closed",
		"identity":        "strict frame-row itid/ipid joined to non-ambiguous canonical identities and public tid/pid; start/end metadata never selects a generation",
		"header_cpu":      "unique untainted Running witnesses at ts and end_ts-1; CPU 0 is never an unknown fallback; headers preserve endpoint provenance rather than claiming interval-long execution",
		"wire_pairing":    "atomic async S/F keyed by hconv-frame-<stable_identity>; frame intervals never occupy the shared B/E stack",
		"frame_flag":      "closed joint enum: Actual requires integer 0/1/3, Expected requires NULL, and integer 2 is always suppressed as erased/do-not-draw",
	}
	if err != nil || !coverage.Found {
		return coverage, err
	}
	if len(coverage.ColumnsMissing) > 0 {
		coverage.Skipped = fmt.Sprintf("unsupported_schema_profile=%d; missing_required_columns=%s", coverage.RowsRead, strings.Join(coverage.ColumnsMissing, ","))
		return coverage, nil
	}

	hasSourceID, err := tdb.columnExists(ctx, "frame_slice", "id")
	if err != nil {
		return coverage, err
	}
	hasType, err := tdb.columnExists(ctx, "frame_slice", "type")
	if err != nil {
		return coverage, err
	}
	for column, present := range map[string]bool{"id": hasSourceID, "type": hasType} {
		if present {
			coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, column)
		}
	}
	sort.Strings(coverage.ColumnsPresent)
	if hasSourceID != hasType {
		coverage.Skipped = fmt.Sprintf("unsupported_schema_profile=%d; id_and_type_must_be_both_present_or_both_absent=true", coverage.RowsRead)
		return coverage, nil
	}

	stableExpr := ""
	stableSource := ""
	duplicateSourceIDs := map[int64]bool{}
	if hasSourceID {
		stableExpr = quoteSQLiteIdent("id")
		stableSource = "frame_slice.id"
		duplicateSourceIDs, err = traceDBDuplicateStrictSourceIDs(ctx, tdb, "frame_slice", "id")
		if err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
	} else {
		stableExpr, stableSource, err = traceDBHiddenRowIDExpr(ctx, tdb.db, "frame_slice")
		if err != nil {
			coverage.FieldSources["stable_identity"] = "unavailable: no frame_slice.id and no provable SQLite hidden rowid"
			if coverage.RowsRead > 0 {
				coverage.Skipped = fmt.Sprintf("stable_row_identity_unavailable=%d", coverage.RowsRead)
			}
			return coverage, nil
		}
	}
	coverage.FieldSources["stable_identity"] = stableSource

	typeExpr := "NULL"
	if hasType {
		typeExpr = quoteSQLiteIdent("type")
	}
	query := fmt.Sprintf(`
		SELECT %s, ts, dur, %s, type_desc, vsync, %s, ipid, itid
		FROM frame_slice
		ORDER BY ts, %s
	`, stableExpr, typeExpr, quoteSQLiteIdent("flag"), stableExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()

	skipped := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		var stableRaw, tsRaw, durRaw, typeRaw, typeDescRaw, vsyncRaw, flagRaw, ipidRaw, itidRaw any
		if err := rows.Scan(&stableRaw, &tsRaw, &durRaw, &typeRaw, &typeDescRaw, &vsyncRaw, &flagRaw, &ipidRaw, &itidRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		frame, reason := prepareTraceDBFrameSliceRow(index, running, hasSourceID, hasType, duplicateSourceIDs,
			stableRaw, tsRaw, durRaw, typeRaw, typeDescRaw, vsyncRaw, flagRaw, ipidRaw, itidRaw)
		if reason != "" {
			skipped[reason]++
			continue
		}
		if err := addTraceDBAsyncSpanRows(sink, frame.TS, frame.End, frame.Task, frame.TID, frame.TGID,
			frame.StartCPU, frame.EndCPU, frame.Name, frame.Cookie); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, nil
}

func prepareTraceDBFrameSliceRow(index traceDBThreadIndex, running map[int64][]traceDBRunningInterval,
	hasSourceID, hasType bool, duplicateSourceIDs map[int64]bool,
	stableRaw, tsRaw, durRaw, typeRaw, typeDescRaw, vsyncRaw, flagRaw, ipidRaw, itidRaw any,
) (traceDBFrameSliceRow, string) {
	var frame traceDBFrameSliceRow
	var ok bool
	if frame.StableID, ok = traceDBStrictSQLiteInt(stableRaw); !ok {
		return frame, "invalid_row_identity"
	}
	if hasSourceID && (frame.StableID < 0 || frame.StableID > maxTraceDBInternalID) {
		return frame, "invalid_row_identity"
	}
	if duplicateSourceIDs[frame.StableID] {
		return frame, "duplicate_row_identity"
	}
	if frame.TS, ok = traceDBStrictSQLiteInt(tsRaw); !ok || frame.TS < 0 {
		return frame, "invalid_timestamp"
	}
	dur, ok := traceDBStrictSQLiteInt(durRaw)
	if !ok || dur <= 0 {
		return frame, "invalid_duration"
	}
	if frame.TS > math.MaxInt64-dur {
		return frame, "interval_overflow"
	}
	frame.End = frame.TS + dur
	if !traceDBWireIntervalRepresentable(frame.TS, frame.End) {
		return frame, "wire_interval_collapsed"
	}

	kind, reason := traceDBFrameKind(hasType, typeRaw, typeDescRaw)
	if reason != "" {
		return frame, reason
	}
	if reason := traceDBFrameFlagKind(kind, flagRaw); reason != "" {
		return frame, reason
	}

	vsync := "None"
	if vsyncRaw != nil {
		value, valid := traceDBStrictSQLiteInt(vsyncRaw)
		if !valid || value < 0 || value > maxTraceDBInternalID {
			return frame, "invalid_vsync"
		}
		vsync = strconv.FormatInt(value, 10)
	}
	frame.Name = "Frame" + kind + "-" + vsync
	frame.Cookie = "hconv-frame-" + strconv.FormatInt(frame.StableID, 10)

	if frame.IPID, ok = traceDBStrictSQLiteInt(ipidRaw); !ok || frame.IPID <= 0 || frame.IPID > maxTraceDBInternalID {
		return frame, "invalid_owner_ipid"
	}
	if frame.ITID, ok = traceDBStrictSQLiteInt(itidRaw); !ok || frame.ITID <= 0 || frame.ITID > maxTraceDBInternalID {
		return frame, "invalid_emitter_itid"
	}
	thread, exists := index.ByITID[frame.ITID]
	if !exists || index.AmbiguousITID[frame.ITID] || thread.TID <= 0 || thread.TID > math.MaxInt32 {
		return frame, "unresolved_emitter_thread"
	}
	if thread.IPID != frame.IPID {
		return frame, "owner_identity_mismatch"
	}
	if traceDBBeforeCaptureStart(index, frame.TS) {
		return frame, "before_capture_start"
	}
	if index.AmbiguousIPID[frame.IPID] {
		return frame, "ambiguous_owner_process"
	}
	process, exists := index.Processes[frame.IPID]
	if !exists || process.PID <= 0 || process.PID > math.MaxInt32 {
		return frame, "unresolved_owner_process"
	}
	frame.Task = traceDBCommName(thread.Name, "frame")
	if _, valid := traceDBCallstackText(thread.Name, true); !valid || !traceDBSinglePhysicalLine(frame.Task, true) {
		return frame, "invalid_emitter_comm"
	}
	if index.RunningGlobalTaint || index.RunningTaintedITID[frame.ITID] {
		return frame, "tainted_running_cpu_witness"
	}
	if frame.StartCPU, ok = traceDBKnownCPUAt(running, frame.ITID, frame.TS); !ok {
		return frame, "unknown_start_cpu"
	}
	if frame.EndCPU, ok = traceDBKnownCPUAt(running, frame.ITID, frame.End-1); !ok {
		return frame, "unknown_end_cpu"
	}
	frame.TID = thread.TID
	frame.TGID = process.PID
	return frame, ""
}

func traceDBFrameKind(hasType bool, typeRaw, typeDescRaw any) (string, string) {
	typeDesc, ok := typeDescRaw.(string)
	if !ok {
		return "", "invalid_frame_kind"
	}
	wantType := int64(-1)
	kind := ""
	switch typeDesc {
	case "actural":
		wantType = 0
		kind = "Actual"
	case "expect":
		wantType = 1
		kind = "Expected"
	default:
		return "", "invalid_frame_kind"
	}
	if !hasType {
		return kind, ""
	}
	typeValue, valid := traceDBStrictSQLiteInt(typeRaw)
	if !valid || typeValue != wantType {
		return "", "frame_kind_mismatch"
	}
	return kind, ""
}

func traceDBFrameFlagKind(kind string, flagRaw any) string {
	if flagRaw == nil {
		if kind == "Expected" {
			return ""
		}
		return "frame_flag_kind_mismatch"
	}
	flag, ok := traceDBStrictSQLiteInt(flagRaw)
	if !ok {
		return "invalid_frame_flag"
	}
	if flag == 2 {
		return "suppressed_frame_flag"
	}
	if flag != 0 && flag != 1 && flag != 3 {
		return "invalid_frame_flag"
	}
	if kind != "Actual" {
		return "frame_flag_kind_mismatch"
	}
	return ""
}
