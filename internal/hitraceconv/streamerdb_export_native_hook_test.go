package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBNativeHookResourceLifetimeNeverBecomesExecutionSpan(t *testing.T) {
	body, coverage, outPath := exportTraceDBNativeHookFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 501, 1, 'worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 0, 100, 2, 'Running')",
		"CREATE TABLE native_hook (start_ts INT, end_ts INT, event_type TEXT, all_heap_size INT, itid INT, ipid INT)",
		"INSERT INTO native_hook VALUES (0, NULL, 'AllocEvent', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (10, NULL, 'FreeEvent', 80, 2, 1)",
		"INSERT INTO native_hook VALUES (20, 90, 'MmapEvent', 200, 2, 1)",
		"INSERT INTO native_hook VALUES (30, NULL, 'MunmapEvent', 0, 2, 1)",
		"INSERT INTO native_hook VALUES (40, 80, 'malloc', 60, 2, 1)",
		"INSERT INTO native_hook VALUES (50, NULL, 'ARKTS_HEAP_Alloc_Event', 300, 2, 1)",
	})
	if coverage.Family != "resource" || coverage.Table != "native_hook" || coverage.RowsEmitted != 12 || coverage.Skipped != "" {
		t.Fatalf("unexpected native-hook coverage: %+v", coverage)
	}
	if strings.Contains(body, "tracing_mark_write: B|") || strings.Contains(body, "tracing_mark_write: E|") {
		t.Fatalf("resource lifetime must not become a thread execution span:\n%s", body)
	}
	for _, want := range []string{
		"[002] ....     0.000000: tracing_mark_write: I|500|NativeHook:AllocEvent",
		"tracing_mark_write: I|500|NativeHook:FreeEvent",
		"tracing_mark_write: I|500|NativeHook:MmapEvent",
		"tracing_mark_write: I|500|NativeHook:MunmapEvent",
		"tracing_mark_write: I|500|NativeHook:ARKTS_HEAP_Alloc_Event",
		"tracing_mark_write: C|500|HeapSize|100",
		"tracing_mark_write: C|500|MmapSize|200",
		"tracing_mark_write: C|500|NativeHook_ARKTS_HEAP_Active|300",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("native-hook output missing %q:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse native-hook output: %v", err)
	}
	instants, counters, durations := 0, 0, 0
	for _, event := range idx.Events {
		if event.Type != tracequery.EventTraceMark {
			continue
		}
		switch event.SpanAction {
		case "I":
			instants++
		case "C":
			counters++
		case "B", "E", "S", "F", "G", "H":
			durations++
		}
	}
	if instants != 6 || counters != 6 || durations != 0 {
		t.Fatalf("native-hook round trip minted duration evidence: instants=%d counters=%d durations=%d events=%+v", instants, counters, durations, idx.Events)
	}
}

func TestTraceDBNativeHookStrictRowsSkipLocallyAndKeepValidSiblings(t *testing.T) {
	body, coverage, _ := exportTraceDBNativeHookFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 501, 1, 'worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 0, 100, 2, 'Running')",
		"CREATE TABLE native_hook (start_ts, end_ts, event_type, all_heap_size, itid, ipid)",
		"INSERT INTO native_hook VALUES (0, NULL, 'AllocEvent', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (-1, 10, 'AllocEvent', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (1.5, 10, 'AllocEvent', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (NULL, 10, 'AllocEvent', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (2, -1, 'AllocEvent', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (3, 2, 'AllocEvent', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (4, 10, 'OtherEvent', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (5, 10, 'AllocEvent\nFORGED-sched_switch', 100, 2, 1)",
		"INSERT INTO native_hook VALUES (6, 10, 'AllocEvent', 100, 2.5, 1)",
		"INSERT INTO native_hook VALUES (7, 10, 'AllocEvent', 100, 2, 2)",
		"INSERT INTO native_hook VALUES (8, 10, 'AllocEvent', 12.5, 2, 1)",
		"INSERT INTO native_hook VALUES (9, 0, 'FreeEvent', 90, 2, 1)",
	})
	if coverage.RowsEmitted != 5 {
		t.Fatalf("valid siblings and row-local counter rejection should emit 5 rows, got %+v\n%s", coverage, body)
	}
	for _, want := range []string{
		"invalid_timestamp=3",
		"invalid_resource_end=1",
		"invalid_resource_lifetime=1",
		"invalid_event_type=2",
		"invalid_emitter_itid=1",
		"owner_identity_mismatch=1",
		"invalid_all_heap_size=1",
	} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("native-hook skip ledger missing %q: %+v", want, coverage)
		}
	}
	if strings.Contains(body, "FORGED-sched_switch") || strings.Count(body, "NativeHook:AllocEvent") != 2 || strings.Count(body, "NativeHook:FreeEvent") != 1 {
		t.Fatalf("malformed row escaped or valid siblings were lost:\n%s", body)
	}
}

func TestTraceDBNativeHookRequiresExactCPUAndStableRowIdentity(t *testing.T) {
	t.Run("unknown CPU", func(t *testing.T) {
		body, coverage, _ := exportTraceDBNativeHookFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'app')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (2, 501, 1, 'worker', 0, 0, 1)",
			"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
			"CREATE TABLE native_hook (start_ts INT, end_ts INT, event_type TEXT, all_heap_size INT, itid INT, ipid INT)",
			"INSERT INTO native_hook VALUES (0, 10, 'AllocEvent', 100, 2, 1)",
		})
		if strings.Contains(body, "tracing_mark_write:") || coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "unknown_event_cpu=1") {
			t.Fatalf("unknown CPU must fail closed instead of becoming CPU0: coverage=%+v body=%q", coverage, body)
		}
	})

	t.Run("source id supports without rowid", func(t *testing.T) {
		body, coverage, _ := exportTraceDBNativeHookFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'app')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (2, 501, 1, 'worker', 0, 0, 1)",
			"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
			"INSERT INTO thread_state VALUES (2, 0, 100, 2, 'Running')",
			"CREATE TABLE native_hook (id INT PRIMARY KEY, start_ts INT, end_ts INT, event_type TEXT, all_heap_size INT, itid INT, ipid INT) WITHOUT ROWID",
			"INSERT INTO native_hook VALUES (7, 0, 10, 'AllocEvent', 100, 2, 1)",
		})
		if coverage.RowsEmitted != 2 || coverage.Skipped != "" || !strings.Contains(body, "NativeHook:AllocEvent") ||
			coverage.FieldSources["row_order"] != "native_hook.id; same-timestamp rows retain stable source order" {
			t.Fatalf("strict source id should support a WITHOUT ROWID table: coverage=%+v body=%q", coverage, body)
		}
	})

	t.Run("duplicate source id", func(t *testing.T) {
		body, coverage, _ := exportTraceDBNativeHookFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'app')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (2, 501, 1, 'worker', 0, 0, 1)",
			"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
			"INSERT INTO thread_state VALUES (2, 0, 100, 2, 'Running')",
			"CREATE TABLE native_hook (id, start_ts, end_ts, event_type, all_heap_size, itid, ipid)",
			"INSERT INTO native_hook VALUES (7, 0, NULL, 'AllocEvent', 100, 2, 1)",
			"INSERT INTO native_hook VALUES (7, 1, NULL, 'FreeEvent', 90, 2, 1)",
		})
		if strings.Contains(body, "tracing_mark_write:") || coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "duplicate_row_identity=2") {
			t.Fatalf("duplicate source identities must fail closed as a cohort: coverage=%+v body=%q", coverage, body)
		}
	})
}

func TestTraceDBNativeHookCurrentOpenHarmonyEventRegistry(t *testing.T) {
	for _, eventType := range []string{"AllocEvent", "FreeEvent", "MmapEvent", "MunmapEvent", "malloc", "free", "mmap", "munmap"} {
		if _, _, ok := traceDBNativeHookEventType(eventType); !ok {
			t.Fatalf("expected classic or legacy event type %q to be accepted", eventType)
		}
	}
	for _, family := range []string{
		"GPU_VK", "GPU_GLES", "GPU_CL", "OTHER", "ARKTS_HEAP", "JS_HEAP", "KMP_HEAP",
		"RN_HERMES_HEAP", "DART_HEAP", "ASHMEM", "ION", "SO", "ARK_GLOBAL_HANDLE",
		"ARK_LOCAL_HANDLE", "ARKTS_STATIC_HEAP",
	} {
		for _, suffix := range []string{"_Alloc_Event", "_Free_Event"} {
			eventType := family + suffix
			gotType, gotCounter, ok := traceDBNativeHookEventType(eventType)
			if !ok || gotType != eventType || gotCounter != "NativeHook_"+family+"_Active" {
				t.Fatalf("unexpected registry mapping for %q: type=%q counter=%q ok=%v", eventType, gotType, gotCounter, ok)
			}
		}
	}
	for _, eventType := range []string{"FD_Open_Event", "FD_Close_Event", "THREAD_Create_Event", "THREAD_Destroy_Event", "Thread_Create_Event", "Thread_Destroy_Event"} {
		if _, _, ok := traceDBNativeHookEventType(eventType); !ok {
			t.Fatalf("expected official or exact historical event type %q to be accepted", eventType)
		}
	}
	for _, eventType := range []string{"", "GPU_VK_Move_Event", "VMA_ARKWEB_Alloc_Event", "ARKTS_HEAP_Alloc_Event\nFORGED"} {
		if _, _, ok := traceDBNativeHookEventType(eventType); ok {
			t.Fatalf("unregistered event type %q must fail closed", eventType)
		}
	}
}

func exportTraceDBNativeHookFixture(t *testing.T, statements []string) (string, TraceDBCoverage, string) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open trace db: %v", err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatalf("load thread index: %v", err)
	}
	running, integrity, _, err := tdb.loadRunningIntervals(context.Background())
	if err != nil {
		t.Fatalf("load running intervals: %v", err)
	}
	index.RunningTaintedITID = integrity.TaintedITIDs
	index.RunningGlobalTaint = integrity.GlobalTaint
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBNativeHook(context.Background(), tdb, sink, index, running, nil)
	if err != nil {
		t.Fatalf("export native_hook: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "native-hook.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.writeTo(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write native-hook output: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(bodyBytes), coverage, outPath
}
