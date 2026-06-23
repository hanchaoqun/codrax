package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestExportTraceDBExtendedFamiliesComprehensiveFixture(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'MainApp')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 501, 1, 'WorkerThread', 100, 0, 1)",
		"INSERT INTO thread VALUES (5, 701, 1, 'ExecThread', 100, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 900000, 500000, 3, 'Running')",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"INSERT INTO data_dict VALUES (5, 'coldStart')",
		"INSERT INTO data_dict VALUES (6, 'SYS')",
		"INSERT INTO data_dict VALUES (7, 'BATTERY')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie TEXT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1000000, 200000, 2, 'DoWork', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (2, 1300000, 0, 2, 'AsyncWork', 'S', NULL, 'chain-123')",
		"INSERT INTO callstack VALUES (3, 1400000, 0, 2, 'AsyncWork', 'C', NULL, 'chain-123')",
		"INSERT INTO callstack VALUES (4, 3100000, 0, 2, 'AllocTask', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (5, 3200000, 70000, 5, 'ExecTask', '', NULL, NULL)",
		"CREATE TABLE frame_slice (ts INT, dur INT, type_desc TEXT, vsync INT, flag TEXT, ipid INT, itid INT)",
		"INSERT INTO frame_slice VALUES (1500000, 16000, 'actural', 123, 1, 1, 2)",
		"CREATE TABLE dma_fence (ts INT, dur INT, cat TEXT, driver TEXT, timeline TEXT, context INT, seqno INT)",
		"INSERT INTO dma_fence VALUES (1600000, 0, 'dma_fence_signaled', 'drv', 'tl', 1, 2)",
		"INSERT INTO dma_fence VALUES (1700000, 30000, 'dma_fence_wait', 'drv', 'tl', 3, 4)",
		"CREATE TABLE syscall (ts INT, dur INT, syscall_number INT, args TEXT, ret INT, itid INT)",
		"INSERT INTO syscall VALUES (1800000, 5000, 64, 'x', 0, 2)",
		"CREATE TABLE task_pool (task_id INT, allocation_task_row INT, execute_task_row INT, allocation_itid INT, execute_itid INT)",
		"INSERT INTO task_pool VALUES (99, 4, 5, 2, 5)",
		"CREATE TABLE app_startup (start_time INT, end_time INT, start_name INT, ipid INT)",
		"INSERT INTO app_startup VALUES (3300000, 3400000, 5, 1)",
		"CREATE TABLE static_initalize (start_time INT, end_time INT, so_name TEXT, ipid INT, tid INT)",
		"INSERT INTO static_initalize VALUES (3500000, 3510000, 'libfoo.so', 1, 501)",
		"CREATE TABLE native_hook (start_ts INT, end_ts INT, event_type TEXT, heap_size INT, all_heap_size INT, itid INT, ipid INT)",
		"INSERT INTO native_hook VALUES (3600000, 3610000, 'malloc', 64, 8192, 2, 1)",
		"CREATE TABLE measure (ts INT, value REAL, filter_id INT)",
		"CREATE TABLE cpu_measure_filter (id INT, name TEXT, cpu INT)",
		"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_idle', 1)",
		"INSERT INTO cpu_measure_filter VALUES (2, 'cpu_frequency', 4)",
		"INSERT INTO measure VALUES (1900000, 1, 1)",
		"INSERT INTO measure VALUES (1910000, 2200000, 2)",
		"CREATE TABLE measure_filter (id INT, name TEXT, type TEXT)",
		"INSERT INTO measure_filter VALUES (10, 'ddr_freq', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (11, 'display', 'power')",
		"INSERT INTO measure VALUES (1920000, 400, 10)",
		"CREATE TABLE process_measure_filter (id INT, name TEXT, ipid INT)",
		"INSERT INTO process_measure_filter VALUES (1, 'H:Heap size (KB)', 1)",
		"CREATE TABLE process_measure (ts INT, value REAL, filter_id INT)",
		"INSERT INTO process_measure VALUES (1950000, 4096, 1)",
		"CREATE TABLE network (ts INT, tx_speed REAL, rx_speed REAL)",
		"INSERT INTO network VALUES (2000000, 1.5, 2.5)",
		"CREATE TABLE diskio (ts INT, rd_speed REAL, wr_speed REAL)",
		"INSERT INTO diskio VALUES (2100000, 10, 20)",
		"CREATE TABLE cpu_usage (ts INT, total_load REAL, user_load REAL, system_load REAL)",
		"INSERT INTO cpu_usage VALUES (2200000, 70, 30, 40)",
		"CREATE TABLE live_process (ts INT, process_id INT, process_name TEXT, cpu_usage REAL, pss_info REAL, thread_num REAL)",
		"INSERT INTO live_process VALUES (2300000, 500, 'MainApp', 15, 2048, 2)",
		"CREATE TABLE xpower_measure (ts INT, value REAL, filter_id INT)",
		"INSERT INTO xpower_measure VALUES (2400000, 12.5, 11)",
		"CREATE TABLE log (ts INT, pid INT, tid INT, level TEXT, tag TEXT, context TEXT)",
		"INSERT INTO log VALUES (2500000, 500, 501, 'I', 'TEST', 'hello\nworld')",
		"CREATE TABLE hisys_all_event (ts INT, pid INT, tid INT, domain_id INT, event_name_id INT, contents TEXT)",
		"INSERT INTO hisys_all_event VALUES (2600000, 500, 501, 6, 7, 'low\npower')",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open trace db: %v", err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBExtendedFamilies(context.Background(), tdb, sink)
	if err != nil {
		t.Fatalf("export extended families: %v", err)
	}
	for _, key := range []struct {
		family string
		table  string
	}{
		{"slice", "callstack"},
		{"slice", "frame_slice"},
		{"slice", "native_hook"},
		{"counter", "measure"},
		{"counter", "process_measure"},
		{"log", "log"},
		{"log", "hisys_all_event"},
	} {
		assertCoverageEmitted(t, coverage, key.family, key.table, 1)
	}
	outPath := filepath.Join(t.TempDir(), "extended.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	stats, writeErr := sink.writeTo(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write extended systrace: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if stats.RowsWritten < 35 || stats.SpillChunks == 0 {
		t.Fatalf("unexpected extended row stats: %+v", stats)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"tracing_mark_write: B|500|DoWork",
		"tracing_mark_write: S|500|AsyncWork|chain-123",
		"tracing_mark_write: B|500|FrameActual-123",
		"dma_fence_signaled: driver=drv timeline=tl context=1 seqno=2",
		"tracing_mark_write: B|500|sys_64",
		"tracing_mark_write: S|500|TaskPool-99|99",
		"tracing_mark_write: B|500|AppStartup:coldStart",
		"tracing_mark_write: B|500|SoInit:libfoo.so",
		"tracing_mark_write: B|500|malloc",
		"tracing_mark_write: C|500|HeapSize|8192",
		"cpu_idle: state=1 cpu_id=1",
		"cpu_frequency: state=2200000 cpu_id=4",
		"clock_set_rate: ddr_freq state=400 cpu_id=0",
		"tracing_mark_write: C|500|H:Heap size (KB)|4096.0",
		"tracing_mark_write: C|0|net_tx_speed|1.5",
		"tracing_mark_write: C|0|disk_wr_speed|20.0",
		"tracing_mark_write: C|0|cpu_total_load|70.0",
		"tracing_mark_write: C|500|pss_kb|2048.0",
		"tracing_mark_write: C|0|xpower_display|12.5",
		"print: [I][TEST] hello world",
		"print: SYS/BATTERY: low power",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("extended systrace missing %q:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse extended DB output: %v", err)
	}
	traceMarks := 0
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventTraceMark {
			traceMarks++
		}
	}
	if traceMarks == 0 {
		t.Fatalf("tracequery should retain trace marker spans")
	}
}

func TestExportTraceDBPerfSamplesRoundTripToTraceQuery(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (1000)",
		"CREATE TABLE process (pid INT, ipid INT, name TEXT)",
		"INSERT INTO process VALUES (101, 1, 'demo_process')",
		"CREATE TABLE thread (tid INT, itid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT, start_ts INT)",
		"INSERT INTO thread VALUES (101, 1, 1, 'demo_main', 1, 1, 1000)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 1000, 2000, 0, 'Running')",
		"CREATE TABLE perf_thread (thread_id INT, process_id INT, thread_name TEXT)",
		"CREATE TABLE perf_sample (callchain_id INT, timestamp_trace INT, thread_id INT, event_count INT, cpu_id INT, event_type_id INT)",
		"CREATE TABLE perf_report (id INT, report_value TEXT)",
		"CREATE TABLE perf_callchain (id INT, callchain_id INT, depth INT, vaddr_in_file INT, offset_to_vaddr INT, ip INT, file_id INT, symbol_id INT, line_number INT)",
		"CREATE TABLE perf_files (file_id INT, serial_id INT, symbol TEXT, path TEXT)",
		"CREATE TABLE hmtrace_perf_symbolized_frame (perf_callchain_row_id INT PRIMARY KEY, display_name TEXT NOT NULL, origin_name TEXT NOT NULL, source_file TEXT, source_line INT, source_column INT, symbol_origin TEXT NOT NULL)",
		"INSERT INTO perf_thread VALUES (101, 101, 'demo_main')",
		"INSERT INTO perf_report VALUES (9, 'hw-cache-misses')",
		"INSERT INTO perf_sample VALUES (88, 2200, 101, 3, 2, 9)",
		"INSERT INTO perf_callchain VALUES (1001, 88, 0, 16, 16, 16, 1, 1, 55)",
		"INSERT INTO perf_callchain VALUES (1002, 88, 1, 24, 24, 24, 1, 1, 12)",
		"INSERT INTO perf_files VALUES (1, 1, 'raw_leaf', '/system/lib64/libdemo.so')",
		"INSERT INTO hmtrace_perf_symbolized_frame VALUES (1001, 'runWorkload@entry/src/main/ets/pages/Index.ets:55:28', 'raw_leaf', 'entry/src/main/ets/pages/Index.ets', 55, 28, 'source_map+name_cache')",
		"INSERT INTO hmtrace_perf_symbolized_frame VALUES (1002, 'main', 'main', 'entry/src/main/ets/pages/Index.ets', 12, 1, 'source_map+name_cache')",
	})
	outPath := filepath.Join(t.TempDir(), "perf-db.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export perf DB systrace: %v", err)
	}
	assertCoverageEmitted(t, result.Coverage, "perf", "perf_sample", 4)
	assertCoverageEmitted(t, result.Coverage, "perf", "perf_callchain", 2)
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"tracing_mark_write: B|101|hiperf:hw-cache-misses:runWorkload@entry/src/main/ets/pages/Index.ets:55:28",
		"tracing_mark_write: C|101|hiperf:hw-cache-misses:event_count|3",
		"perf_sample: cpu=2 cpu_known=true pid=101 tid=101",
		`symbol="runWorkload@entry/src/main/ets/pages/Index.ets:55:28"`,
		`callchain="runWorkload@entry/src/main/ets/pages/Index.ets:55:28;main"`,
		"source=trace_streamer_db",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("perf DB systrace missing %q:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse perf DB output: %v", err)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 0, TimeEnd: 0.00001})
	if stats.PerfSamples == nil || len(stats.PerfSamples.TopSymbols) == 0 ||
		stats.PerfSamples.TopSymbols[0].Symbol != "runWorkload@entry/src/main/ets/pages/Index.ets:55:28" ||
		stats.PerfSamples.TopSymbols[0].Period != 3 {
		t.Fatalf("perf DB sample did not reach tracequery stats: %+v", stats.PerfSamples)
	}
}

func TestExportTraceDBHmtraceComprehensiveFixtureSchema(t *testing.T) {
	path := createTraceDBFixture(t, hmtraceComprehensiveFixtureStatements())
	outPath := filepath.Join(t.TempDir(), "hmtrace-comprehensive.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export hmtrace comprehensive fixture: %v", err)
	}
	for _, key := range []struct {
		family string
		table  string
		min    int
	}{
		{"metadata", "thread", 9},
		{"slice", "callstack", 6},
		{"scheduler", "sched_slice", 2},
		{"scheduler", "instant", 2},
		{"irq", "irq", 4},
		{"counter", "measure", 2},
		{"counter", "measure_filter", 1},
		{"counter", "process_measure", 1},
		{"slice", "frame_slice", 2},
		{"slice", "dma_fence", 3},
		{"counter", "network", 2},
		{"counter", "diskio", 2},
		{"counter", "cpu_usage", 3},
		{"counter", "live_process", 3},
		{"log", "log", 1},
		{"slice", "syscall", 2},
		{"slice", "task_pool", 2},
		{"slice", "app_startup", 2},
		{"slice", "static_initalize", 2},
		{"slice", "native_hook", 3},
		{"log", "hisys_all_event", 1},
		{"counter", "xpower_measure", 1},
		{"sorter", "__systrace_rows__", result.EventsWritten},
	} {
		assertCoverageEmitted(t, result.Coverage, key.family, key.table, key.min)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"tracing_mark_write: B|500|DoWork",
		"sched_wakeup: comm=WorkerThread pid=501 prio=42 target_cpu=011",
		"sched_waking: comm=WorkerThread pid=501 prio=42 target_cpu=011",
		"softirq_entry: vec=9 [action=RCU]",
		"cpu_frequency: state=2200000 cpu_id=11",
		"tracing_mark_write: B|500|FrameActual-123",
		"tracing_mark_write: B|0|dma_fence_wait:tl:4",
		"tracing_mark_write: C|500|pss_kb|2048.0",
		"print: [I][TEST] hello world",
		"tracing_mark_write: B|500|AppStartup:coldStart",
		"tracing_mark_write: C|500|HeapSize|8192",
		"print: SYS/BATTERY: low power",
		"tracing_mark_write: C|0|xpower_display|12.5",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("hmtrace comprehensive systrace missing %q:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse hmtrace comprehensive output: %v", err)
	}
	if len(idx.Events) < 20 {
		t.Fatalf("tracequery should parse comprehensive fixture rows, got %d", len(idx.Events))
	}
}

func TestExportTraceDBThreadStateResolverWithoutSchedSlice(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'MainApp')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (10, 501, 1, 'WorkerThread', 100, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (10, 900, 700, 7, 'Running')",
		"INSERT INTO thread_state VALUES (10, 1700, 200, 2, 'Runnable')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie TEXT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1000, 400, 10, 'ThreadStateWork', '', NULL, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "thread-state-resolver.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export thread_state resolver fixture: %v", err)
	}
	assertCoverageEmitted(t, result.Coverage, "resolver", "thread_state", 1)
	if !coverageHasSkipped(result.Coverage, "scheduler", "sched_slice", "missing table") {
		t.Fatalf("missing sched_slice should be visible without blocking thread_state resolver: %+v", result.Coverage)
	}
	for _, item := range result.Coverage {
		if item.Family == "resolver" && item.Table == "thread_state" {
			if item.RowsRead != 2 || item.RowsEmitted != 1 {
				t.Fatalf("thread_state coverage should expose read rows and emitted running windows: %+v", item)
			}
		}
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	want := "[007] ....     0.000001: tracing_mark_write: B|500|ThreadStateWork"
	if !strings.Contains(body, want) {
		t.Fatalf("callstack span should inherit CPU from thread_state, missing %q:\n%s", want, body)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse thread_state resolver output: %v", err)
	}
	traceMarks := 0
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventTraceMark && ev.SpanName == "ThreadStateWork" && ev.CPU == 7 {
			traceMarks++
		}
	}
	if traceMarks == 0 {
		t.Fatalf("tracequery should retain thread_state-placed callstack span: %+v", idx.Events)
	}
}

func TestExportTraceDBRawFtraceRootCauseEvidence(t *testing.T) {
	path := createTraceDBFixture(t, rawFtraceRootCauseFixtureStatements())
	outPath := filepath.Join(t.TempDir(), "raw-ftrace.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export raw ftrace fixture: %v", err)
	}
	for _, key := range []struct {
		table string
		min   int
	}{
		{"binder", 3},
		{"block_storage", 4},
		{"file_io", 2},
		{"page_cache", 2},
		{"workqueue", 2},
		{"dma_fence", 1},
	} {
		assertCoverageEmitted(t, result.Coverage, "raw_ftrace", key.table, key.min)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"binder_transaction: transaction=42 dest_node=9 dest_proc=500 dest_thread=700 reply=0 flags=0x12 code=0x4",
		"android_fs_dataread_start: dev=260:136 ino=12345 entry_name=foo.db offset=0 bytes=4096 rw=read",
		"block_rq_issue: 8,0 R 0 (READ) 128 + 8",
		"scsi_dispatch_cmd_start: tag=7 dev=8:0 lba=4096 len=8 opcode=READ_10",
		"mm_filemap_add_to_page_cache: dev=260:136 ino=12345",
		"workqueue_execute_start: work=0xabc function=0xdef",
		"dma_fence_signaled: driver=drv timeline=tl context=1 seqno=2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("raw ftrace systrace missing %q:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse raw ftrace output: %v", err)
	}
	ipc := tracequery.BuildIPCGraph(idx, tracequery.Query{})
	if len(ipc.Edges) != 1 || ipc.Edges[0].TransactionID != 42 || ipc.Edges[0].DestThread != 700 || ipc.Edges[0].Flags != "0x12" {
		t.Fatalf("raw binder rows did not become IPC edge: %+v", ipc)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{})
	if len(stats.FileIOByInode) == 0 || stats.FileIOByInode[0].Inode != "12345" || stats.FileIOByInode[0].Bytes != 4096 {
		t.Fatalf("raw file IO did not aggregate by inode: %+v", stats.FileIOByInode)
	}
	if len(stats.PageCacheByInode) == 0 || stats.PageCacheByInode[0].Churn < 2 {
		t.Fatalf("raw page cache did not aggregate by inode: %+v", stats.PageCacheByInode)
	}
	if len(stats.StorageLatencyByLayer) == 0 || stats.StorageLatencyByLayer[0].PairedCount == 0 {
		t.Fatalf("raw storage events did not pair latency: %+v", stats.StorageLatencyByLayer)
	}
	if stats.IOPressureSummary == nil || stats.IOPressureSummary.TopInode != "12345" {
		t.Fatalf("raw IO pressure summary missing hot inode: %+v", stats.IOPressureSummary)
	}
	if len(stats.WorkqueueActivity) == 0 || stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("raw workqueue rows did not pair: %+v", stats.WorkqueueActivity)
	}
	foundDMAFence := false
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventDMAFence && ev.Name == "dma_fence_signaled" {
			foundDMAFence = true
			break
		}
	}
	if !foundDMAFence {
		t.Fatalf("raw dma_fence row did not remain queryable: %+v", idx.Events)
	}
}

func TestExportTraceDBRawFtraceMissingArgsetCoverage(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE raw (ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (1000, 'binder_transaction', 1, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-no-argset.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export raw ftrace missing argset fixture: %v", err)
	}
	if !coverageHasSkipped(result.Coverage, "raw_ftrace", "raw", "missing argset") {
		t.Fatalf("missing raw argset coverage not exposed: %+v", result.Coverage)
	}
}

func TestExportTraceDBRawFtraceMissingArgsDependencyCoverage(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE raw (ts INT, name TEXT, cpu INT, itid INT, argsetid INT)",
		"INSERT INTO raw VALUES (1000, 'binder_transaction', 1, 1, 7)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-no-args.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export raw ftrace missing args fixture: %v", err)
	}
	if !coverageHasSkipped(result.Coverage, "raw_ftrace", "raw", "missing args/data_dict") {
		t.Fatalf("missing args dependency coverage not exposed: %+v", result.Coverage)
	}
}

func rawFtraceRootCauseFixtureStatements() []string {
	stmts := []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'MainApp')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 500, 1, 'MainApp', 100, 1, 1)",
		"INSERT INTO thread VALUES (2, 700, 1, 'BinderPeer', 100, 0, 1)",
		"INSERT INTO thread VALUES (3, 800, 1, 'IOThread', 100, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 900, 20000, 1, 'Running')",
		"INSERT INTO thread_state VALUES (2, 900, 20000, 2, 'Running')",
		"INSERT INTO thread_state VALUES (3, 900, 20000, 3, 'Running')",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"CREATE TABLE args (argset INT, key INT, datatype INT, value INT)",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT, argsetid INT)",
	}
	nextDict := 1
	dict := map[string]int{}
	dictID := func(text string) int {
		if id, ok := dict[text]; ok {
			return id
		}
		id := nextDict
		nextDict++
		dict[text] = id
		stmts = append(stmts, fmt.Sprintf("INSERT INTO data_dict VALUES (%d, '%s')", id, strings.ReplaceAll(text, "'", "''")))
		return id
	}
	addArg := func(argset int, key string, value int64) {
		stmts = append(stmts, fmt.Sprintf("INSERT INTO args VALUES (%d, %d, 0, %d)", argset, dictID(key), value))
	}
	addTextArg := func(argset int, key, value string) {
		stmts = append(stmts, fmt.Sprintf("INSERT INTO args VALUES (%d, %d, 1, %d)", argset, dictID(key), dictID(value)))
	}
	addRaw := func(id int, ts int64, name string, cpu, itid, argset int) {
		stmts = append(stmts, fmt.Sprintf("INSERT INTO raw VALUES (%d, %d, '%s', %d, %d, %d)", id, ts, name, cpu, itid, argset))
	}

	addArg(100, "transaction", 42)
	addArg(100, "dest_node", 9)
	addArg(100, "dest_proc", 500)
	addArg(100, "dest_thread", 700)
	addArg(100, "reply", 0)
	addArg(100, "flags", 18)
	addArg(100, "code", 4)
	addRaw(1, 1000, "binder_transaction", 1, 1, 100)

	addArg(101, "transaction", 42)
	addRaw(2, 1200, "binder_transaction_received", 2, 2, 101)

	addArg(102, "transaction", 42)
	addArg(102, "debug_id", 42)
	addArg(102, "data_size", 128)
	addArg(102, "offsets_size", 16)
	addArg(102, "extra_buffers_size", 0)
	addRaw(3, 1300, "binder_transaction_alloc_buf", 1, 1, 102)

	for _, argset := range []int{200, 201} {
		addTextArg(argset, "dev", "260:136")
		addArg(argset, "ino", 12345)
		addTextArg(argset, "entry_name", "foo.db")
		addArg(argset, "offset", 0)
		addArg(argset, "bytes", 4096)
		addTextArg(argset, "rw", "read")
	}
	addRaw(4, 3000, "android_fs_dataread_start", 3, 3, 200)
	addArg(201, "ret", 0)
	addArg(201, "latency_us", 800)
	addRaw(5, 3600, "android_fs_dataread_end", 3, 3, 201)

	for _, argset := range []int{210, 211} {
		addTextArg(argset, "dev", "260:136")
		addArg(argset, "ino", 12345)
		addArg(argset, "offset", 0)
		addArg(argset, "bytes", 4096)
	}
	addRaw(6, 3900, "mm_filemap_add_to_page_cache", 3, 3, 210)
	addRaw(7, 4000, "mm_filemap_delete_from_page_cache", 3, 3, 211)

	for _, argset := range []int{300, 301} {
		addTextArg(argset, "dev", "8,0")
		addTextArg(argset, "rwbs", "R")
		addTextArg(argset, "cmd", "READ")
		addArg(argset, "sector", 128)
		addArg(argset, "nr_sector", 8)
	}
	addRaw(8, 5000, "block_rq_issue", 3, 3, 300)
	addArg(301, "error", 0)
	addRaw(9, 9000, "block_rq_complete", 3, 3, 301)

	for _, argset := range []int{310, 311} {
		addArg(argset, "tag", 7)
		addTextArg(argset, "dev", "8:0")
		addArg(argset, "lba", 4096)
		addArg(argset, "len", 8)
		addTextArg(argset, "opcode", "READ_10")
	}
	addRaw(10, 6000, "scsi_dispatch_cmd_start", 3, 3, 310)
	addArg(311, "ret", 0)
	addArg(311, "latency_us", 900)
	addRaw(11, 9600, "scsi_dispatch_cmd_done", 3, 3, 311)

	for _, argset := range []int{400, 401} {
		addArg(argset, "work", 0xabc)
		addArg(argset, "function", 0xdef)
	}
	addRaw(12, 10000, "workqueue_execute_start", 3, 3, 400)
	addRaw(13, 15000, "workqueue_execute_end", 3, 3, 401)

	addTextArg(500, "driver", "drv")
	addTextArg(500, "timeline", "tl")
	addArg(500, "context", 1)
	addArg(500, "seqno", 2)
	addRaw(14, 16000, "dma_fence_signaled", 3, 3, 500)

	return stmts
}

func hmtraceComprehensiveFixtureStatements() []string {
	return []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE process (id INT, ipid INT, pid INT, name TEXT, start_ts INT, switch_count INT, thread_count INT, slice_count INT, mem_count INT)",
		"CREATE TABLE thread (id INT, itid INT, tid INT, name TEXT, start_ts INT, end_ts INT, ipid INT, is_main_thread INT, switch_count INT)",
		"CREATE TABLE thread_state (id INT, ts INT, dur INT, cpu INT, itid INT, tid INT, pid INT, state TEXT, arg_setid INT)",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, cat TEXT, name TEXT, depth INT, cookie INT, parent_id INT, argsetid INT, chainId TEXT, spanId TEXT, parentSpanId TEXT, flag TEXT, trace_level TEXT, trace_tag TEXT, custom_category TEXT, custom_args TEXT, child_callid INT)",
		"CREATE TABLE sched_slice (id INT, ts INT, dur INT, ts_end INT, cpu INT, itid INT, ipid INT, end_state TEXT, priority INT, arg_setid INT)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"CREATE TABLE args (id INT, key INT, datatype INT, value INT, argset INT)",
		"CREATE TABLE irq (id INT, ts INT, dur INT, callid INT, cat TEXT, name TEXT, depth INT, cookie INT, parent_id INT, argsetid INT, flag TEXT)",
		"CREATE TABLE measure (ts INT, value REAL, filter_id INT)",
		"CREATE TABLE cpu_measure_filter (id INT, name TEXT, cpu INT)",
		"CREATE TABLE measure_filter (id INT, name TEXT, type TEXT)",
		"CREATE TABLE process_measure_filter (id INT, name TEXT, ipid INT)",
		"CREATE TABLE process_measure (ts INT, value REAL, filter_id INT)",
		"CREATE TABLE frame_slice (ts INT, dur INT, type_desc TEXT, vsync INT, flag TEXT, ipid INT, itid INT)",
		"CREATE TABLE dma_fence (ts INT, dur INT, cat TEXT, driver TEXT, timeline TEXT, context INT, seqno INT)",
		"CREATE TABLE network (ts INT, tx_speed REAL, rx_speed REAL)",
		"CREATE TABLE diskio (ts INT, rd_speed REAL, wr_speed REAL)",
		"CREATE TABLE cpu_usage (ts INT, total_load REAL, user_load REAL, system_load REAL)",
		"CREATE TABLE live_process (ts INT, process_id INT, process_name TEXT, cpu_usage REAL, pss_info REAL, thread_num REAL)",
		"CREATE TABLE log (ts INT, pid INT, tid INT, level TEXT, tag TEXT, context TEXT)",
		"CREATE TABLE syscall (ts INT, dur INT, syscall_number INT, args TEXT, ret INT, itid INT)",
		"CREATE TABLE task_pool (task_id INT, allocation_task_row INT, execute_task_row INT, allocation_itid INT, execute_itid INT)",
		"CREATE TABLE app_startup (start_time INT, end_time INT, start_name INT, ipid INT)",
		"CREATE TABLE static_initalize (start_time INT, end_time INT, so_name TEXT, ipid INT, tid INT)",
		"CREATE TABLE native_hook (start_ts INT, end_ts INT, event_type TEXT, heap_size INT, all_heap_size INT, itid INT, ipid INT)",
		"CREATE TABLE hisys_all_event (ts INT, pid INT, tid INT, domain_id INT, event_name_id INT, contents TEXT)",
		"CREATE TABLE xpower_measure (ts INT, value REAL, filter_id INT)",
		"INSERT INTO process VALUES (1, 1, 500, '500', 100, 0, 2, 1, 0)",
		"INSERT INTO process VALUES (2, 2, 1531, NULL, 100, 0, 2, 1, 0)",
		"INSERT INTO process VALUES (3, 3, 700, '700', 100, 0, 1, 1, 0)",
		"INSERT INTO thread VALUES (1, 1, 500, 'MainApp', 100, 0, 1, 1, 1)",
		"INSERT INTO thread VALUES (2, 2, 501, 'WorkerThread', 100, 0, 1, 0, 1)",
		"INSERT INTO thread VALUES (3, 3, 1531, NULL, 100, 0, 2, 1, 0)",
		"INSERT INTO thread VALUES (4, 4, 1782, 'DnsMgerListen', 100, 0, 2, 0, 1)",
		"INSERT INTO thread VALUES (5, 5, 701, 'Waker', 100, 0, 3, 1, 1)",
		"INSERT INTO thread_state VALUES (1, 900, 1000, 3, 2, 501, 500, 'Running', 0)",
		"INSERT INTO callstack VALUES (1, 1000, 200, 2, '', 'DoWork', 0, NULL, 0, 0, '', '', '', '', '', '', '', '', 0)",
		"INSERT INTO callstack VALUES (2, 1100, 0, 2, '', 'AsyncWork', 0, NULL, 0, 0, 'chain-123', '', '', 'S', '', '', '', '', 0)",
		"INSERT INTO callstack VALUES (3, 1150, 0, 2, '', 'AsyncWork', 0, NULL, 0, 0, 'chain-123', '', '', 'C', '', '', '', '', 0)",
		"INSERT INTO callstack VALUES (4, 3100, 0, 2, '', 'AllocTask', 0, NULL, 0, 0, '', '', '', '', '', '', '', '', 0)",
		"INSERT INTO callstack VALUES (5, 3200, 70, 5, '', 'ExecTask', 0, NULL, 0, 0, '', '', '', '', '', '', '', '', 0)",
		"INSERT INTO sched_slice VALUES (1, 2000, 100, 2100, 11, 2, 1, 'S', 42, 0)",
		"INSERT INTO sched_slice VALUES (2, 2100, 120, 2220, 11, 5, 3, 'R', 20, 0)",
		"INSERT INTO instant VALUES (1500, 'sched_wakeup', 2, 5, 'itid', NULL)",
		"INSERT INTO instant VALUES (1510, 'sched_waking', 2, 5, 'itid', NULL)",
		"INSERT INTO raw VALUES (1, 1500, 'sched_wakeup', 7, 2)",
		"INSERT INTO raw VALUES (2, 1510, 'sched_waking', 8, 2)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'handled')",
		"INSERT INTO data_dict VALUES (4, 'vec')",
		"INSERT INTO data_dict VALUES (5, 'coldStart')",
		"INSERT INTO data_dict VALUES (6, 'SYS')",
		"INSERT INTO data_dict VALUES (7, 'BATTERY')",
		"INSERT INTO args VALUES (1, 1, 0, 32, 10)",
		"INSERT INTO args VALUES (2, 2, 1, 3, 10)",
		"INSERT INTO args VALUES (3, 4, 0, 9, 20)",
		"INSERT INTO irq VALUES (1, 1600, 10, 4, 'irq', 'uart', 0, NULL, NULL, 10, '1')",
		"INSERT INTO irq VALUES (2, 1700, 20, 6, 'softirq', 'RCU', 0, NULL, NULL, 20, '')",
		"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_idle', 1)",
		"INSERT INTO cpu_measure_filter VALUES (2, 'cpu_frequency', 11)",
		"INSERT INTO measure VALUES (1800, 1, 1)",
		"INSERT INTO measure VALUES (1810, 2200000, 2)",
		"INSERT INTO measure_filter VALUES (10, 'ddr_freq', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (11, 'display', 'power')",
		"INSERT INTO measure VALUES (1900, 400, 10)",
		"INSERT INTO process_measure_filter VALUES (1, 'H:Heap size (KB)', 1)",
		"INSERT INTO process_measure VALUES (1950, 4096, 1)",
		"INSERT INTO frame_slice VALUES (2050, 16, 'actural', 123, 1, 1, 2)",
		"INSERT INTO dma_fence VALUES (2200, 0, 'dma_fence_signaled', 'drv', 'tl', 1, 2)",
		"INSERT INTO dma_fence VALUES (2210, 30, 'dma_fence_wait', 'drv', 'tl', 3, 4)",
		"INSERT INTO network VALUES (2300, 1.5, 2.5)",
		"INSERT INTO diskio VALUES (2400, 10, 20)",
		"INSERT INTO cpu_usage VALUES (2500, 70, 30, 40)",
		"INSERT INTO live_process VALUES (2600, 500, 'MainApp', 15, 2048, 2)",
		"INSERT INTO log VALUES (2700, 500, 501, 'I', 'TEST', 'hello\nworld')",
		"INSERT INTO syscall VALUES (2800, 5, 64, 'x', 0, 2)",
		"INSERT INTO task_pool VALUES (99, 4, 5, 2, 5)",
		"INSERT INTO app_startup VALUES (3300, 3400, 5, 1)",
		"INSERT INTO static_initalize VALUES (3500, 3510, 'libfoo.so', 1, 501)",
		"INSERT INTO native_hook VALUES (3600, 3610, 'malloc', 64, 8192, 2, 1)",
		"INSERT INTO hisys_all_event VALUES (3700, 500, 501, 6, 7, 'low\npower')",
		"INSERT INTO xpower_measure VALUES (3800, 12.5, 11)",
	}
}
