package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// native_hook.end_ts is the release timestamp of an allocation/mapping and
// native_hook.dur is its resource lifetime.  Neither value is evidence that
// the emitting thread executed for that interval.  Export each hook record as
// a process instant and keep all_heap_size as a counter; never mint B/E spans
// from the resource lifetime.
type traceDBNativeHookEvent struct {
	StableID    int64
	TS          int64
	End         int64
	EventType   string
	Counter     string
	HeapSize    int64
	HeapValid   bool
	EmitterITID int64
	OwnerIPID   int64
	Task        string
	TID         int64
	TGID        int64
	CPU         int64
}

func exportTraceDBNativeHook(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex,
) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resource", "native_hook", []string{"start_ts", "end_ts", "event_type", "all_heap_size", "itid", "ipid"})
	coverage.FieldSources = map[string]string{
		"event_semantics":   "start_ts is the only emitter lifecycle point; nullable end_ts and any upstream dur derived from it are resource metadata and never enter thread lifecycle or CPU admission",
		"event_type":        "closed OpenHarmony TraceStreamer 260b028b resource event registry plus exact legacy aliases malloc/free/mmap/munmap",
		"cpu":               "shared lifecycle-filtered typed Running witness covering exact start_ts",
		"identity":          "canonical native_hook.itid/ipid joined to an exact positive owner; emitter origin passes the shared thread+process point gate",
		"row_order":         "signed-int32 native_hook.id row identity when present; otherwise a provable SQLite hidden rowid",
		"resource_metadata": "instant-only resource metadata, not execution: resource_end_ts_ns preserves nullable end_ts (NULL/0 do not prove release); source_heap_size preserves nonnegative INTEGER heap_size without negating free events or assuming all resource families use bytes; source_callchain_id is an opaque source INTEGER, not a resolved stack or causal edge; absent optional columns stay absent",
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
	hasSourceID, err := tdb.columnExists(ctx, "native_hook", "id")
	if err != nil {
		return fail(err)
	}
	stableExpr := ""
	stableOrderExpr := ""
	stableSource := ""
	duplicateSourceIDs := map[int64]bool{}
	if hasSourceID {
		stableExpr = quoteSQLiteIdent("id")
		stableOrderExpr = fmt.Sprintf("CASE WHEN %s < 0 THEN 1 ELSE 0 END, %s", stableExpr, stableExpr)
		stableSource = "native_hook.id signed-int32 projection to uint32 row identity"
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "id")
		sort.Strings(coverage.ColumnsPresent)
		duplicateSourceIDs, err = traceDBDuplicateSourceIDs(ctx, tdb, "native_hook", "id", traceDBActivityITIDSignedInt32.decodeStableRowID)
		if err != nil {
			return fail(err)
		}
	} else {
		stableExpr, stableSource, err = traceDBHiddenRowIDExpr(ctx, tdb.db, "native_hook")
		if err != nil {
			coverage.FieldSources["row_order"] = "unavailable: no native_hook.id and no provable SQLite hidden rowid"
			coverage.Skipped = "stable_row_identity_unavailable=1"
			return coverage, nil
		}
		stableOrderExpr = stableExpr
	}
	coverage.FieldSources["row_order"] = stableSource + "; same-timestamp rows retain stable source order"
	// Optional resource observations must not become admission requirements for
	// an otherwise valid instant/counter. The exact all-table fidelity lane also
	// preserves unsupported address/storage forms without interpreting them.
	metadataColumns := []struct {
		column, label string
		nonnegative   bool
		present       bool
	}{
		{column: "heap_size", label: "source_heap_size", nonnegative: true},
		{column: "callchain_id", label: "source_callchain_id"},
	}
	metadataSQL := []string{"NULL", "NULL"}
	for i := range metadataColumns {
		field := &metadataColumns[i]
		field.present, err = tdb.columnExists(ctx, "native_hook", field.column)
		if err != nil {
			return fail(err)
		}
		if field.present {
			metadataSQL[i] = quoteSQLiteIdent(field.column)
			coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, field.column)
		}
	}
	sort.Strings(coverage.ColumnsPresent)
	query := fmt.Sprintf(`
		SELECT %s, start_ts, end_ts, event_type, all_heap_size, itid, ipid, %s
		FROM native_hook
		ORDER BY %s
	`, stableExpr, strings.Join(metadataSQL, ", "), stableOrderExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var stableRaw, startRaw, endRaw, eventTypeRaw, heapRaw, itidRaw, ipidRaw any
		var metadataRaw [2]any
		if err := rows.Scan(&stableRaw, &startRaw, &endRaw, &eventTypeRaw, &heapRaw, &itidRaw, &ipidRaw, &metadataRaw[0], &metadataRaw[1]); err != nil {
			return fail(err)
		}
		event, reason := prepareTraceDBNativeHookEvent(authority, running, hasSourceID, duplicateSourceIDs,
			stableRaw, startRaw, endRaw, eventTypeRaw, heapRaw, itidRaw, ipidRaw)
		if reason != "" {
			skipped[reason]++
			continue
		}
		endText := "null"
		if endRaw != nil {
			endText = strconv.FormatInt(event.End, 10)
		}
		instantName := "NativeHook:" + event.EventType + " resource_end_ts_ns=" + endText
		for i, field := range metadataColumns {
			if !field.present {
				continue
			}
			value := "null"
			if metadataRaw[i] != nil {
				integer, valid := traceDBStrictSQLiteInt(metadataRaw[i])
				if !valid || (field.nonnegative && integer < 0) {
					skipped["invalid_optional_"+field.column]++
					continue
				}
				value = strconv.FormatInt(integer, 10)
			}
			instantName += " " + field.label + "=" + value
		}
		if err := addTraceDBInstantRow(sink, event.TS, event.Task, event.TID, event.TGID, event.CPU,
			fmt.Sprintf("tracing_mark_write: I|%d|%s", event.TGID, instantName)); err != nil {
			return fail(err)
		}
		coverage.RowsEmitted++
		if !event.HeapValid {
			skipped["invalid_all_heap_size"]++
			continue
		}
		if err := addTraceDBInstantRow(sink, event.TS, event.Task, event.TID, event.TGID, event.CPU,
			fmt.Sprintf("tracing_mark_write: C|%d|%s|%s", event.TGID, event.Counter, strconv.FormatInt(event.HeapSize, 10))); err != nil {
			return fail(err)
		}
		coverage.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, nil
}

func prepareTraceDBNativeHookEvent(authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex,
	hasSourceID bool, duplicateSourceIDs map[int64]bool,
	stableRaw, startRaw, endRaw, eventTypeRaw, heapRaw, itidRaw, ipidRaw any,
) (traceDBNativeHookEvent, string) {
	var event traceDBNativeHookEvent
	var ok bool
	if hasSourceID {
		event.StableID, ok = traceDBActivityITIDSignedInt32.decodeStableRowID(stableRaw)
	} else {
		event.StableID, ok = traceDBStrictSQLiteInt(stableRaw)
	}
	if !ok {
		return event, "invalid_row_identity"
	}
	if duplicateSourceIDs[event.StableID] {
		return event, "duplicate_row_identity"
	}
	if event.TS, ok = traceDBStrictSQLiteInt(startRaw); !ok || event.TS < 0 {
		return event, "invalid_timestamp"
	}
	if endRaw != nil {
		if event.End, ok = traceDBStrictSQLiteInt(endRaw); !ok || event.End < 0 {
			return event, "invalid_resource_end"
		}
		if event.End != 0 && event.End < event.TS {
			return event, "invalid_resource_lifetime"
		}
	}
	if event.EventType, event.Counter, ok = traceDBNativeHookEventType(eventTypeRaw); !ok {
		return event, "invalid_event_type"
	}
	if event.EmitterITID, ok = traceDBStrictInternalID(itidRaw); !ok || event.EmitterITID <= 0 {
		return event, "invalid_emitter_itid"
	}
	if event.OwnerIPID, ok = traceDBStrictInternalID(ipidRaw); !ok || event.OwnerIPID <= 0 {
		return event, "invalid_owner_ipid"
	}
	thread, process, resolution := authority.resolveThreadSubject(event.EmitterITID)
	if resolution != traceDBSchedulerThreadResolved || thread.TID > math.MaxInt32 {
		return event, "unresolved_emitter_thread"
	}
	if thread.IPID != event.OwnerIPID {
		return event, "owner_identity_mismatch"
	}
	if traceDBBeforeCaptureStart(authority.identities, event.TS) {
		return event, "before_capture_start"
	}
	if process.IPID != event.OwnerIPID || process.PID <= 0 || process.PID > math.MaxInt32 {
		return event, "unresolved_owner_process"
	}
	if !authority.threadPointAllows(event.EmitterITID, event.TS) {
		return event, "lifecycle_rejected_event_origin"
	}
	var runningStatus traceDBSchedulerRunningLookupStatus
	event.CPU, runningStatus = running.lookupCPUAt(event.EmitterITID, event.TS)
	if runningStatus == traceDBSchedulerRunningSourceTainted {
		return event, "tainted_running_cpu_witness"
	}
	if runningStatus == traceDBSchedulerRunningLifecycleRejected {
		return event, "lifecycle_rejected_running_cpu_witness"
	}
	if runningStatus != traceDBSchedulerRunningKnown {
		return event, "unknown_event_cpu"
	}
	if _, ok := traceDBCallstackText(thread.Name, true); !ok {
		return event, "invalid_emitter_comm"
	}
	event.Task = traceDBCommName(thread.Name, "hook")
	event.TID = thread.TID
	event.TGID = process.PID
	if event.HeapSize, ok = traceDBStrictSQLiteInt(heapRaw); ok && event.HeapSize >= 0 {
		event.HeapValid = true
	}
	return event, ""
}

func traceDBNativeHookEventType(value any) (eventType, counter string, ok bool) {
	text, ok := traceDBCallstackText(value, false)
	if !ok {
		return "", "", false
	}
	switch text {
	case "AllocEvent", "malloc":
		return "AllocEvent", "HeapSize", true
	case "FreeEvent", "free":
		return "FreeEvent", "HeapSize", true
	case "MmapEvent", "mmap":
		return "MmapEvent", "MmapSize", true
	case "MunmapEvent", "munmap":
		return "MunmapEvent", "MmapSize", true
	}
	if family, found := traceDBNativeHookAllocFreeFamily(text); found {
		return text, "NativeHook_" + family + "_Active", true
	}
	switch text {
	case "FD_Open_Event", "FD_Close_Event":
		return text, "NativeHook_FD_Active", true
	case "THREAD_Create_Event", "THREAD_Destroy_Event":
		return text, "NativeHook_THREAD_Active", true
	case "Thread_Create_Event":
		return "THREAD_Create_Event", "NativeHook_THREAD_Active", true
	case "Thread_Destroy_Event":
		return "THREAD_Destroy_Event", "NativeHook_THREAD_Active", true
	default:
		return "", "", false
	}
}

func traceDBNativeHookAllocFreeFamily(eventType string) (string, bool) {
	var family string
	switch {
	case strings.HasSuffix(eventType, "_Alloc_Event"):
		family = strings.TrimSuffix(eventType, "_Alloc_Event")
	case strings.HasSuffix(eventType, "_Free_Event"):
		family = strings.TrimSuffix(eventType, "_Free_Event")
	default:
		return "", false
	}
	switch family {
	case "GPU_VK", "GPU_GLES", "GPU_CL", "OTHER", "ARKTS_HEAP", "JS_HEAP", "KMP_HEAP",
		"RN_HERMES_HEAP", "DART_HEAP", "ASHMEM", "ION", "SO", "ARK_GLOBAL_HANDLE",
		"ARK_LOCAL_HANDLE", "ARKTS_STATIC_HEAP":
		return family, true
	default:
		return "", false
	}
}
