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

func exportTraceDBNativeHook(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, running map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resource", "native_hook", []string{"start_ts", "end_ts", "event_type", "all_heap_size", "itid", "ipid"})
	coverage.FieldSources = map[string]string{
		"event_semantics": "start_ts is a resource event instant; nullable end_ts is release metadata, never thread execution duration",
		"event_type":      "closed OpenHarmony TraceStreamer 260b028b resource event registry plus exact legacy aliases malloc/free/mmap/munmap",
		"cpu":             "the unique untainted Running thread_state interval covering start_ts",
		"identity":        "exact non-ambiguous thread.itid and process.ipid generation",
		"row_order":       "strict native_hook.id when present; otherwise a provable SQLite hidden rowid",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	hasSourceID, err := tdb.columnExists(ctx, "native_hook", "id")
	if err != nil {
		return coverage, err
	}
	stableExpr := ""
	stableSource := ""
	duplicateSourceIDs := map[int64]bool{}
	if hasSourceID {
		stableExpr = quoteSQLiteIdent("id")
		stableSource = "native_hook.id"
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "id")
		sort.Strings(coverage.ColumnsPresent)
		duplicateSourceIDs, err = traceDBDuplicateStrictSourceIDs(ctx, tdb, "native_hook", "id")
		if err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
	} else {
		stableExpr, stableSource, err = traceDBHiddenRowIDExpr(ctx, tdb.db, "native_hook")
		if err != nil {
			coverage.FieldSources["row_order"] = "unavailable: no native_hook.id and no provable SQLite hidden rowid"
			coverage.Skipped = "stable_row_identity_unavailable=1"
			return coverage, nil
		}
	}
	coverage.FieldSources["row_order"] = stableSource + "; same-timestamp rows retain stable source order"
	query := fmt.Sprintf(`
		SELECT %s, start_ts, end_ts, event_type, all_heap_size, itid, ipid
		FROM native_hook
		ORDER BY %s
	`, stableExpr, stableExpr)
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
		var stableRaw, startRaw, endRaw, eventTypeRaw, heapRaw, itidRaw, ipidRaw any
		if err := rows.Scan(&stableRaw, &startRaw, &endRaw, &eventTypeRaw, &heapRaw, &itidRaw, &ipidRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		event, reason := prepareTraceDBNativeHookEvent(index, running, hasSourceID, duplicateSourceIDs,
			stableRaw, startRaw, endRaw, eventTypeRaw, heapRaw, itidRaw, ipidRaw)
		if reason != "" {
			skipped[reason]++
			continue
		}
		instantName := "NativeHook:" + event.EventType
		if err := addTraceDBInstantRow(sink, event.TS, event.Task, event.TID, event.TGID, event.CPU,
			fmt.Sprintf("tracing_mark_write: I|%d|%s", event.TGID, instantName)); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
		if !event.HeapValid {
			skipped["invalid_all_heap_size"]++
			continue
		}
		if err := addTraceDBInstantRow(sink, event.TS, event.Task, event.TID, event.TGID, event.CPU,
			fmt.Sprintf("tracing_mark_write: C|%d|%s|%s", event.TGID, event.Counter, strconv.FormatInt(event.HeapSize, 10))); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, nil
}

func prepareTraceDBNativeHookEvent(index traceDBThreadIndex, running map[int64][]traceDBRunningInterval,
	hasSourceID bool, duplicateSourceIDs map[int64]bool,
	stableRaw, startRaw, endRaw, eventTypeRaw, heapRaw, itidRaw, ipidRaw any,
) (traceDBNativeHookEvent, string) {
	var event traceDBNativeHookEvent
	var ok bool
	if event.StableID, ok = traceDBStrictSQLiteInt(stableRaw); !ok {
		return event, "invalid_row_identity"
	}
	if hasSourceID && (event.StableID < 0 || event.StableID > maxTraceDBInternalID) {
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
	if event.EmitterITID, ok = traceDBStrictSQLiteInt(itidRaw); !ok || event.EmitterITID <= 0 || event.EmitterITID > maxTraceDBInternalID {
		return event, "invalid_emitter_itid"
	}
	if event.OwnerIPID, ok = traceDBStrictSQLiteInt(ipidRaw); !ok || event.OwnerIPID <= 0 || event.OwnerIPID > maxTraceDBInternalID {
		return event, "invalid_owner_ipid"
	}
	thread, exists := index.ByITID[event.EmitterITID]
	if !exists || index.AmbiguousITID[event.EmitterITID] || thread.TID <= 0 || thread.TID > math.MaxInt32 {
		return event, "unresolved_emitter_thread"
	}
	if thread.IPID != event.OwnerIPID {
		return event, "owner_identity_mismatch"
	}
	if event.TS < index.TraceStart {
		return event, "before_capture_start"
	}
	if index.AmbiguousIPID[event.OwnerIPID] {
		return event, "ambiguous_owner_process"
	}
	process, exists := index.Processes[event.OwnerIPID]
	if !exists || process.PID <= 0 || process.PID > math.MaxInt32 {
		return event, "unresolved_owner_process"
	}
	if index.RunningGlobalTaint || index.RunningTaintedITID[event.EmitterITID] {
		return event, "tainted_running_cpu_witness"
	}
	if event.CPU, ok = traceDBKnownCPUAt(running, event.EmitterITID, event.TS); !ok {
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
