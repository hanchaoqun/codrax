package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// loadTraceDBFrameRelationRoster admits only the official frame row identity
// and timestamp. It is deliberately independent of frame span lifecycle/CPU
// admission: exact SQL foreign-key relations remain useful even when a frame
// cannot be rendered as a physical S/F lane. Capture-window admission remains
// mandatory, however. A producer row before the authoritative trace start is
// retained by the SQL fidelity carrier, but must not become a query-visible
// timestamp that expands the converted trace envelope.
func loadTraceDBFrameRelationRoster(ctx context.Context, tdb *traceDB,
	authority traceDBSchedulerAuthority,
) (TraceDBCoverage, map[int64]traceDBFrameSliceRow, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.frame", "frame_slice", []string{"id", "ts"})
	coverage.FieldSources = map[string]string{
		"stable_identity": "official frame_slice.id decoded by the collector-selected frame uint32 profile",
		"timestamp":       "strict frame_slice.ts; relation anchor only",
		"admission":       "capture-window admitted; independent of frame S/F lifecycle, CPU, duration, type, flag, and identity admission",
		"callstack":       "optional official frame_slice.callstack_id retained for exact endpoint validation",
	}
	out := map[int64]traceDBFrameSliceRow{}
	fail := func(cause error) (TraceDBCoverage, map[int64]traceDBFrameSliceRow, error) {
		if cause != nil {
			coverage.Error = cause.Error()
		}
		return coverage, out, cause
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return fail(err)
	}
	if authority.frameProfile != traceDBActivityITIDSignedInt32 &&
		authority.frameProfile != traceDBActivityITIDCanonical {
		coverage.Skipped = fmt.Sprintf("unsupported_schema_profile=%d; collector_profile=%s",
			coverage.RowsRead, authority.frameProfileSource)
		return coverage, out, nil
	}
	hasCallstack, err := tdb.columnExists(ctx, "frame_slice", "callstack_id")
	if err != nil {
		return fail(err)
	}
	callstackExpr := "NULL"
	if hasCallstack {
		callstackExpr = quoteSQLiteIdent("callstack_id")
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "callstack_id")
		sort.Strings(coverage.ColumnsPresent)
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, ts, %s FROM frame_slice ORDER BY ts, CASE WHEN id < 0 THEN 1 ELSE 0 END, id`,
		callstackExpr))
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	counts := map[int64]int{}
	candidates := map[int64]traceDBFrameSliceRow{}
	skipped := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var idRaw, tsRaw, callstackRaw any
		if err := rows.Scan(&idRaw, &tsRaw, &callstackRaw); err != nil {
			return fail(err)
		}
		id, ok := authority.frameProfile.decodeStableRowID(idRaw)
		if !ok {
			skipped["invalid_frame_row"]++
			continue
		}
		counts[id]++
		ts, ok := traceDBStrictSQLiteInt(tsRaw)
		if !ok || ts < 0 {
			skipped["invalid_frame_timestamp"]++
			continue
		}
		if traceDBBeforeCaptureStart(authority.identities, ts) {
			skipped["before_capture_start"]++
			continue
		}
		frame := traceDBFrameSliceRow{StableID: id, TS: ts}
		if callstackRaw != nil {
			frame.CallstackPresent = true
			frame.CallstackID, frame.CallstackValid = traceDBStrictSQLiteInt(callstackRaw)
			frame.CallstackValid = frame.CallstackValid && frame.CallstackID >= 0
		}
		candidates[id] = frame
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	for id, frame := range candidates {
		if counts[id] != 1 {
			skipped["duplicate_frame_row"]++
			continue
		}
		out[id] = frame
	}
	coverage.RowsEmitted = len(out)
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, out, nil
}

func exportTraceDBFrameCallstackRelations(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	frames map[int64]traceDBFrameSliceRow,
) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "relation", "frame_slice_callstack",
		[]string{"id", "ts", "callstack_id"})
	coverage.SourceTables = []string{"frame_slice", "callstack"}
	coverage.FieldSources = map[string]string{
		"source":    "official frame_slice.callstack_id",
		"endpoint":  "exact unique callstack.id row",
		"timestamp": "referenced frame_slice.ts anchor; no interval/CPU/thread claim",
		"semantics": "typed foreign-key edge only; NULL means producer supplied no relation",
	}
	fail := func(cause error) (TraceDBCoverage, error) {
		if cause != nil {
			coverage.Error = cause.Error()
		}
		return coverage, cause
	}
	frameFound, err := tdb.tableExists(ctx, "frame_slice")
	if err != nil {
		return fail(err)
	}
	coverage.Found = frameFound
	if !frameFound {
		coverage.Skipped = "missing table"
		return coverage, nil
	}
	if coverage.RowsRead, err = tdb.rowCount(ctx, "frame_slice"); err != nil {
		return fail(err)
	}
	for _, column := range []string{"id", "ts", "callstack_id"} {
		present, columnErr := tdb.columnExists(ctx, "frame_slice", column)
		if columnErr != nil {
			return fail(columnErr)
		}
		if present {
			coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, column)
		} else {
			coverage.ColumnsMissing = append(coverage.ColumnsMissing, column)
		}
	}
	sort.Strings(coverage.ColumnsMissing)
	if len(coverage.ColumnsMissing) > 0 {
		coverage.Skipped = "missing required columns: " + strings.Join(coverage.ColumnsMissing, ",")
		return coverage, nil
	}
	ids, idCounts, endpointCoverage, err := loadTraceDBExactRowIDs(ctx, tdb, "callstack")
	coverage.Metrics = map[string]int64{
		"callstack_rows":          int64(endpointCoverage.RowsRead),
		"unique_callstack_rows":   int64(len(ids)),
		"duplicate_callstack_ids": int64(endpointCoverage.Metrics["duplicate_ids"]),
	}
	if err != nil {
		return fail(err)
	}
	if !endpointCoverage.Found || len(endpointCoverage.ColumnsMissing) > 0 {
		coverage.Skipped = "callstack endpoint unavailable: " + endpointCoverage.Skipped
		return coverage, nil
	}
	skipped := map[string]int{}
	frameIDs := sortedTraceDBFrameIDs(frames)
	for _, frameID := range frameIDs {
		frame := frames[frameID]
		if !frame.CallstackPresent {
			coverage.Metrics["frame_rows_without_callstack"]++
			continue
		}
		if !frame.CallstackValid {
			skipped["invalid_callstack_row"]++
			continue
		}
		if idCounts[frame.CallstackID] != 1 || !ids[frame.CallstackID] {
			skipped["unavailable_callstack_endpoint"]++
			continue
		}
		line, err := tracequery.FormatFrameCallstackRelation(tracequery.FrameCallstackRelation{
			TimestampNS: uint64(frame.TS), FrameRow: uint32(frame.StableID),
			CallstackRow: uint64(frame.CallstackID),
		})
		if err != nil {
			return fail(&traceDBOutputInvariantError{Reason: "invalid_frame_callstack_relation", Cause: err})
		}
		if err := addTraceDBTypedCommentRow(sink, frame.TS, line); err != nil {
			return fail(err)
		}
		coverage.RowsEmitted++
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, nil
}

func exportTraceDBFrameGPURelations(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority, frames map[int64]traceDBFrameSliceRow,
) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resource.relation", "gpu_slice",
		[]string{"id", "frame_row", "dur"})
	coverage.SourceTables = []string{"gpu_slice", "frame_slice"}
	coverage.FieldSources = map[string]string{
		"stable_identity": "official gpu_slice.id signed-int32 projection to uint32",
		"frame_endpoint":  "official gpu_slice.frame_row joined to exact unique frame_slice.id",
		"duration":        "strict non-negative gpu_slice.dur resource duration",
		"timestamp":       "referenced frame start anchor only; producer exposes no GPU start timestamp",
		"semantics":       "never synthesized into a B/E span or assigned a CPU/thread",
	}
	fail := func(cause error) (TraceDBCoverage, error) {
		if cause != nil {
			coverage.Error = cause.Error()
		}
		return coverage, cause
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return fail(err)
	}
	if authority.frameProfile != traceDBActivityITIDSignedInt32 &&
		authority.frameProfile != traceDBActivityITIDCanonical {
		coverage.Skipped = fmt.Sprintf("unsupported_schema_profile=%d", coverage.RowsRead)
		return coverage, nil
	}
	rows, err := tdb.db.QueryContext(ctx, `SELECT id, frame_row, dur FROM gpu_slice ORDER BY id`)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	type candidate struct {
		id, frame int64
		dur       uint64
	}
	candidates := map[int64]candidate{}
	counts := map[int64]int{}
	invalid := map[int64]bool{}
	skipped := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var idRaw, frameRaw, durRaw any
		if err := rows.Scan(&idRaw, &frameRaw, &durRaw); err != nil {
			return fail(err)
		}
		id, idOK := traceDBActivityITIDSignedInt32.decodeStableRowID(idRaw)
		frame, frameOK := authority.frameProfile.decodeStableRowID(frameRaw)
		durSigned, durOK := traceDBStrictSQLiteInt(durRaw)
		dur := uint64(durSigned)
		if idOK {
			counts[id]++
		}
		if !idOK || !frameOK || !durOK {
			if idOK {
				invalid[id] = true
			}
			skipped["invalid_gpu_scalar"]++
			continue
		}
		candidates[id] = candidate{id: id, frame: frame, dur: dur}
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	ids := make([]int64, 0, len(candidates))
	for id, candidate := range candidates {
		if counts[id] != 1 || invalid[id] {
			skipped["duplicate_or_invalid_gpu_row"]++
			continue
		}
		if _, ok := frames[candidate.frame]; !ok {
			skipped["unavailable_frame_endpoint"]++
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := frames[candidates[ids[i]].frame], frames[candidates[ids[j]].frame]
		if left.TS != right.TS {
			return left.TS < right.TS
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		item := candidates[id]
		frame := frames[item.frame]
		line, err := tracequery.FormatFrameGPURelation(tracequery.FrameGPURelation{
			TimestampNS: uint64(frame.TS), GPURow: uint32(item.id),
			FrameRow: uint32(item.frame), DurationNS: item.dur,
		})
		if err != nil {
			return fail(&traceDBOutputInvariantError{Reason: "invalid_frame_gpu_relation", Cause: err})
		}
		if err := addTraceDBTypedCommentRow(sink, frame.TS, line); err != nil {
			return fail(err)
		}
		coverage.RowsEmitted++
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, nil
}

type traceDBPerfNAPIEndpoint struct {
	ts, cpu, tid, callchain, count, eventType int64
}

func exportTraceDBPerfNAPIAsyncRelations(ctx context.Context, tdb *traceDB,
	sink *traceDBRowSink,
) (TraceDBCoverage, error) {
	required := []string{"id", "ts", "traceid", "cpu_id", "thread_id", "process_id",
		"caller_callchainid", "callee_callchainid", "perf_sample_id", "event_count", "event_type_id"}
	coverage, err := tdb.inspectCoverage(ctx, "perf.relation", "perf_napi_async", required)
	coverage.SourceTables = []string{"perf_napi_async", "perf_sample", "perf_thread"}
	coverage.FieldSources = map[string]string{
		"semantics":       "official ParseNapiAsync point correlation; no duration and no synthetic span",
		"sample_endpoint": "perf_sample_id exact unique row with timestamp/cpu/tid/callee-callchain/count/event-type convergence",
		"identity":        "perf_thread exact unique tid->pid convergence",
		"caller":          "native-hook callchain identity; preserved as producer scalar without pretending it belongs to perf_callchain",
		"traceid":         "exact SQL TEXT bytes carried as canonical base64url in the comment wire",
	}
	fail := func(cause error) (TraceDBCoverage, error) {
		if cause != nil {
			coverage.Error = cause.Error()
		}
		return coverage, cause
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return fail(err)
	}
	samples, sampleCounts, sampleCoverage, err := loadTraceDBPerfNAPIEndpoints(ctx, tdb)
	coverage.Metrics = map[string]int64{
		"perf_sample_rows":        int64(sampleCoverage.RowsRead),
		"unique_perf_sample_rows": int64(len(samples)),
	}
	if err != nil {
		return fail(err)
	}
	if !sampleCoverage.Found || len(sampleCoverage.ColumnsMissing) > 0 {
		coverage.Skipped = "perf_sample endpoint unavailable: " + sampleCoverage.Skipped
		return coverage, nil
	}
	threadPIDs, threadCounts, threadCoverage, err := loadTraceDBPerfThreadPIDEndpoints(ctx, tdb)
	coverage.Metrics["perf_thread_rows"] = int64(threadCoverage.RowsRead)
	if err != nil {
		return fail(err)
	}
	if !threadCoverage.Found || len(threadCoverage.ColumnsMissing) > 0 {
		coverage.Skipped = "perf_thread endpoint unavailable: " + threadCoverage.Skipped
		return coverage, nil
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT id, ts, traceid, cpu_id, thread_id, process_id, caller_callchainid,
		       callee_callchainid, perf_sample_id, event_count, event_type_id
		FROM perf_napi_async ORDER BY ts, id`)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	type candidate struct {
		id, ts, cpu, tid, pid, caller, callee, sample, count, eventType int64
		traceID                                                         string
	}
	candidates := map[int64]candidate{}
	rowCounts := map[int64]int{}
	invalid := map[int64]bool{}
	skipped := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var raw [11]any
		if err := rows.Scan(&raw[0], &raw[1], &raw[2], &raw[3], &raw[4], &raw[5],
			&raw[6], &raw[7], &raw[8], &raw[9], &raw[10]); err != nil {
			return fail(err)
		}
		item := candidate{}
		var ok bool
		item.id, ok = traceDBStrictSQLiteInt(raw[0])
		if ok {
			rowCounts[item.id]++
		}
		item.traceID, _ = raw[2].(string)
		values := []*int64{&item.ts, &item.cpu, &item.tid, &item.pid, &item.caller,
			&item.callee, &item.sample, &item.count, &item.eventType}
		rawIndexes := []int{1, 3, 4, 5, 6, 7, 8, 9, 10}
		for i, target := range values {
			*target, ok = traceDBStrictSQLiteInt(raw[rawIndexes[i]])
			if !ok {
				break
			}
		}
		valid := ok && item.id >= 0 && item.id <= math.MaxUint32 &&
			item.ts >= 0 && validTraceDBCPUIndex(item.cpu) &&
			item.tid > 0 && item.tid <= math.MaxInt32 &&
			item.pid > 0 && item.pid <= math.MaxInt32 &&
			item.caller >= 0 && item.caller <= math.MaxUint32 &&
			item.callee >= 0 && item.callee <= math.MaxUint32 &&
			item.sample >= 0 && item.sample <= math.MaxUint32 && item.traceID != ""
		if !valid {
			if item.id >= 0 {
				invalid[item.id] = true
			}
			skipped["invalid_napi_scalar"]++
			continue
		}
		candidates[item.id] = item
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	ids := make([]int64, 0, len(candidates))
	for id, item := range candidates {
		if rowCounts[id] != 1 || invalid[id] {
			skipped["duplicate_or_invalid_napi_row"]++
			continue
		}
		sample, ok := samples[item.sample]
		if !ok || sampleCounts[item.sample] != 1 {
			skipped["unavailable_perf_sample_endpoint"]++
			continue
		}
		if sample.ts != item.ts || sample.cpu != item.cpu || sample.tid != item.tid ||
			sample.callchain != item.callee || sample.count != item.count ||
			sample.eventType != item.eventType {
			skipped["perf_sample_endpoint_mismatch"]++
			continue
		}
		if threadCounts[item.tid] != 1 || threadPIDs[item.tid] != item.pid {
			skipped["perf_thread_endpoint_mismatch"]++
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := candidates[ids[i]], candidates[ids[j]]
		if left.ts != right.ts {
			return left.ts < right.ts
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		item := candidates[id]
		line, err := tracequery.FormatPerfNAPIAsyncRelation(tracequery.PerfNAPIAsyncRelation{
			TimestampNS: uint64(item.ts), RowID: uint32(item.id), CPU: int(item.cpu),
			TID: int(item.tid), PID: int(item.pid), CallerCallchain: uint32(item.caller),
			CalleeCallchain: uint32(item.callee), PerfSample: uint32(item.sample),
			EventCount: uint64(item.count), EventType: uint64(item.eventType), TraceID: item.traceID,
		})
		if err != nil {
			return fail(&traceDBOutputInvariantError{Reason: "invalid_perf_napi_async_relation", Cause: err})
		}
		if err := addTraceDBTypedCommentRow(sink, item.ts, line); err != nil {
			return fail(err)
		}
		coverage.RowsEmitted++
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, nil
}

func loadTraceDBExactRowIDs(ctx context.Context, tdb *traceDB, table string) (
	map[int64]bool, map[int64]int, TraceDBCoverage, error,
) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.relation", table, []string{"id"})
	ids := map[int64]bool{}
	counts := map[int64]int{}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return ids, counts, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT id FROM "+quoteSQLiteIdent(table))
	if err != nil {
		coverage.Error = err.Error()
		return ids, counts, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return ids, counts, coverage, err
		}
		id, ok := traceDBStrictSQLiteInt(raw)
		if !ok || id < 0 {
			continue
		}
		counts[id]++
	}
	if err := rows.Err(); err != nil {
		return ids, counts, coverage, err
	}
	for id, count := range counts {
		if count == 1 {
			ids[id] = true
		}
	}
	coverage.RowsEmitted = len(ids)
	coverage.Metrics = map[string]int64{"duplicate_ids": int64(len(counts) - len(ids))}
	return ids, counts, coverage, nil
}

func loadTraceDBPerfNAPIEndpoints(ctx context.Context, tdb *traceDB) (
	map[int64]traceDBPerfNAPIEndpoint, map[int64]int, TraceDBCoverage, error,
) {
	required := []string{"id", "timestamp_trace", "cpu_id", "thread_id",
		"callchain_id", "event_count", "event_type_id"}
	coverage, err := tdb.inspectCoverage(ctx, "resolver.perf", "perf_sample", required)
	out := map[int64]traceDBPerfNAPIEndpoint{}
	counts := map[int64]int{}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, counts, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT id, timestamp_trace, cpu_id, thread_id, callchain_id, event_count, event_type_id
		FROM perf_sample`)
	if err != nil {
		coverage.Error = err.Error()
		return out, counts, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw [7]any
		if err := rows.Scan(&raw[0], &raw[1], &raw[2], &raw[3], &raw[4], &raw[5], &raw[6]); err != nil {
			return out, counts, coverage, err
		}
		id, idOK := traceDBActivityITIDSignedInt32.decodeStableRowID(raw[0])
		if idOK {
			counts[id]++
		}
		values := make([]int64, 6)
		ok := idOK
		for i := range values {
			values[i], ok = traceDBStrictSQLiteInt(raw[i+1])
			if !ok {
				break
			}
		}
		if !ok || values[0] < 0 || !validTraceDBCPUIndex(values[1]) ||
			values[2] <= 0 || values[2] > math.MaxInt32 ||
			values[3] < 0 || values[3] > math.MaxUint32 {
			continue
		}
		out[id] = traceDBPerfNAPIEndpoint{
			ts: values[0], cpu: values[1], tid: values[2], callchain: values[3],
			count: values[4], eventType: values[5],
		}
	}
	if err := rows.Err(); err != nil {
		return out, counts, coverage, err
	}
	for id, count := range counts {
		if count != 1 {
			delete(out, id)
		}
	}
	coverage.RowsEmitted = len(out)
	return out, counts, coverage, nil
}

func loadTraceDBPerfThreadPIDEndpoints(ctx context.Context, tdb *traceDB) (
	map[int64]int64, map[int64]int, TraceDBCoverage, error,
) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.perf", "perf_thread",
		[]string{"thread_id", "process_id"})
	out := map[int64]int64{}
	counts := map[int64]int{}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, counts, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `SELECT thread_id, process_id FROM perf_thread`)
	if err != nil {
		coverage.Error = err.Error()
		return out, counts, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var tidRaw, pidRaw any
		if err := rows.Scan(&tidRaw, &pidRaw); err != nil {
			return out, counts, coverage, err
		}
		tid, tidOK := traceDBStrictSQLiteInt(tidRaw)
		pid, pidOK := traceDBStrictSQLiteInt(pidRaw)
		if !tidOK || !pidOK || tid <= 0 || tid > math.MaxInt32 ||
			pid <= 0 || pid > math.MaxInt32 {
			continue
		}
		counts[tid]++
		out[tid] = pid
	}
	if err := rows.Err(); err != nil {
		return out, counts, coverage, err
	}
	for tid, count := range counts {
		if count != 1 {
			delete(out, tid)
		}
	}
	coverage.RowsEmitted = len(out)
	return out, counts, coverage, nil
}

func sortedTraceDBFrameIDs(frames map[int64]traceDBFrameSliceRow) []int64 {
	ids := make([]int64, 0, len(frames))
	for id := range frames {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := frames[ids[i]], frames[ids[j]]
		if left.TS != right.TS {
			return left.TS < right.TS
		}
		return ids[i] < ids[j]
	})
	return ids
}

func addTraceDBTypedCommentRow(sink *traceDBRowSink, ts int64, line string) error {
	if ts < 0 {
		return &traceDBOutputInvariantError{Reason: "invalid_timestamp"}
	}
	if len(line) > maxTraceDBSystraceLineBytes {
		return &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	return sink.add(renderedRow{tsNS: uint64(ts), seq: sink.stats.RowsAccepted, line: line})
}
