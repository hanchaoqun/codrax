package hitraceconv

import (
	"context"
	"fmt"
	"math"
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
		"INSERT INTO thread_state VALUES (2, 900000, 600000, 3, 'Running')",
		"INSERT INTO thread_state VALUES (2, 1500000, 100000, 3, 'Running')",
		"INSERT INTO thread_state VALUES (2, 1790000, 20000, 6, 'Running')",
		"INSERT INTO thread_state VALUES (2, 3000000, 200000, 3, 'Running')",
		"INSERT INTO thread_state VALUES (5, 3150000, 200000, 5, 'Running')",
		"INSERT INTO thread_state VALUES (2, 3500000, 200000, 3, 'Running')",
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
		"CREATE TABLE frame_slice (ts INT, dur INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
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
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	authority := traceDBTestCompleteSchedulerAuthority(index)
	authority.frameProfile = traceDBActivityITIDCanonical
	authority.frameProfileSource = "legacy frame_slice no-id/no-type canonical compatibility profile"
	coverage, err := exportTraceDBExtendedFamilies(context.Background(), tdb, sink,
		authority, syncSpans)
	if err != nil {
		t.Fatalf("export extended families: %v", err)
	}
	coverage, _, _ = finalizeTraceDBTestSyncSpans(t, sink, syncSpans, coverage)
	for _, key := range []struct {
		family string
		table  string
	}{
		{"slice", "callstack"},
		{"slice", "frame_slice"},
		{"resource", "native_hook"},
		{"counter", "measure"},
		{"counter", "process_measure"},
		{"log", "log"},
		{"log", "hisys_all_event"},
	} {
		assertCoverageEmitted(t, coverage, key.family, key.table, 1)
	}
	callstackCoverage := 0
	for _, item := range coverage {
		if item.Family == "slice" && item.Table == "callstack" {
			callstackCoverage++
			if item.RowsEmitted != 10 {
				t.Fatalf("comprehensive callstack endpoints=%d, want exactly 10: %+v", item.RowsEmitted, item)
			}
		}
	}
	if callstackCoverage != 1 {
		t.Fatalf("comprehensive callstack coverage records=%d, want 1: %+v", callstackCoverage, coverage)
	}
	if !coverageHasSkipped(coverage, "slice", "dma_fence", "high_level_rows_withheld=2") {
		t.Fatalf("high-level DMA predecessor deltas must be withheld: %+v", coverage)
	}
	outPath := filepath.Join(t.TempDir(), "extended.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	stats, writeErr := sink.prepareAndWriteForTest(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write extended systrace: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if stats.RowsWritten < 32 || stats.SpillChunks == 0 {
		t.Fatalf("unexpected extended row stats: %+v", stats)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"tracing_mark_write: B|500|DoWork",
		"tracing_mark_write: B|500|AsyncWork",
		"tracing_mark_write: B|500|AllocTask",
		"tracing_mark_write: B|500|ExecTask",
		"tracing_mark_write: S|500|FrameActual-123|hconv-frame-1",
		"tracing_mark_write: F|500|FrameActual-123|hconv-frame-1",
		"tracing_mark_write: B|500|sys_64",
		"tracing_mark_write: S|500|TaskPool-99|99",
		"tracing_mark_write: B|500|AppStartup:coldStart",
		"tracing_mark_write: B|500|SoInit:libfoo.so",
		"tracing_mark_write: I|500|NativeHook:AllocEvent",
		"tracing_mark_write: C|500|HeapSize|8192",
		"cpu_idle: state=1 cpu_id=1",
		"cpu_frequency: state=2200000 cpu_id=4",
		"clock_set_rate: ddr_freq 400",
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
	if strings.Contains(body, "dma_fence") || strings.Contains(body, "<dma_fence>") {
		t.Fatalf("high-level DMA rows must not mint CPU0/PID0 endpoints:\n%s", body)
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
		"CREATE TABLE instant (ts, name, ref, ref_type)",
		"CREATE TABLE sched_slice (ts, dur, itid, end_state)",
		"CREATE TABLE callstack (ts, itid, callid)",
		"CREATE TABLE syscall (ts, itid)",
		"CREATE TABLE native_hook (start_ts, itid)",
		"CREATE TABLE frame_slice (id, type, ts, itid)",
		"CREATE TABLE perf_thread (thread_id INT, process_id INT, thread_name TEXT)",
		"CREATE TABLE perf_sample (callchain_id INT, timestamp_trace INT, thread_id INT, event_count INT, cpu_id INT, event_type_id INT)",
		"CREATE TABLE perf_report (id INT, report_type TEXT, report_value TEXT)",
		"CREATE TABLE perf_callchain (id INT, callchain_id INT, depth INT, vaddr_in_file INT, offset_to_vaddr INT, ip INT, file_id INT, symbol_id INT, line_number INT)",
		"CREATE TABLE perf_files (file_id INT, serial_id INT, symbol TEXT, path TEXT)",
		"CREATE TABLE hmtrace_perf_symbolized_frame (perf_callchain_row_id INT PRIMARY KEY, display_name TEXT NOT NULL, origin_name TEXT NOT NULL, source_file TEXT, source_line INT, source_column INT, symbol_origin TEXT NOT NULL)",
		"INSERT INTO perf_thread VALUES (101, 101, 'demo_main')",
		"INSERT INTO perf_report VALUES (9, 'config_name', 'hw-cache-misses')",
		"INSERT INTO perf_sample VALUES (88, 2200, 101, 3, 2, 9)",
		"INSERT INTO perf_callchain VALUES (1001, 88, 1, 16, 16, 16, 1, 1, 55)",
		"INSERT INTO perf_callchain VALUES (1002, 88, 0, 24, 24, 24, 1, 1, 12)",
		"INSERT INTO perf_files VALUES (1, 1, 'raw_leaf', '/system/lib64/libdemo.so')",
		"INSERT INTO hmtrace_perf_symbolized_frame VALUES (1001, 'runWorkload@entry/src/main/ets/pages/Index.ets:55:28', 'raw_leaf', 'entry/src/main/ets/pages/Index.ets', 55, 28, 'source_map+name_cache')",
		"INSERT INTO hmtrace_perf_symbolized_frame VALUES (1002, 'main', 'main', 'entry/src/main/ets/pages/Index.ets', 12, 1, 'source_map+name_cache')",
	})
	outPath := filepath.Join(t.TempDir(), "perf-db.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export perf DB systrace: %v", err)
	}
	assertCoverageEmitted(t, result.Coverage, "perf", "perf_sample", 1)
	assertCoverageEmitted(t, result.Coverage, "perf", "perf_callchain", 2)
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"perf_sample: cpu=2 cpu_known=true pid=101 tid=101",
		`symbol="runWorkload@entry/src/main/ets/pages/Index.ets:55:28"`,
		`callchain="main;runWorkload@entry/src/main/ets/pages/Index.ets:55:28"`,
		"source=trace_streamer_db",
		"symbolization_status=symbolized",
		"callchain_status=symbolized",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("perf DB systrace missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "tracing_mark_write: B|101|hiperf:") ||
		strings.Contains(body, "tracing_mark_write: C|101|hiperf:") {
		t.Fatalf("instant perf sample must not mint synthetic trace markers:\n%s", body)
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
	for _, span := range stats.TraceSpans {
		if strings.HasPrefix(span.Name, "hiperf:") {
			t.Fatalf("instant perf sample leaked into trace-span lane: %+v", stats.TraceSpans)
		}
	}
	for _, counter := range stats.CounterDeltas {
		if strings.HasPrefix(counter.Name, "hiperf:") {
			t.Fatalf("instant perf sample leaked into counter lane: %+v", stats.CounterDeltas)
		}
	}
}

func TestExportTraceDBPerfSamplesPreferTraceThreadComm(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (62380000000000)",
		"CREATE TABLE process (pid INT, ipid INT, name TEXT)",
		"INSERT INTO process VALUES (62642, 1, 's.watch.meetime')",
		"CREATE TABLE thread (tid INT, itid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT, start_ts INT)",
		"INSERT INTO thread VALUES (62642, 7, 1, 's.watch.meetime', 1, 1, 62380000000000)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (7, 62380027500000, 100000, 1, 'Running')",
		"CREATE TABLE instant (ts, name, ref, ref_type)",
		"CREATE TABLE sched_slice (ts, dur, itid, end_state)",
		"CREATE TABLE callstack (ts, itid, callid)",
		"CREATE TABLE syscall (ts, itid)",
		"CREATE TABLE native_hook (start_ts, itid)",
		"CREATE TABLE frame_slice (id, type, ts, itid)",
		"CREATE TABLE perf_thread (thread_id INT, process_id INT, thread_name TEXT)",
		"INSERT INTO perf_thread VALUES (62642, 62642, 'com.huawei.hmos')",
		"CREATE TABLE perf_sample (callchain_id INT, timestamp_trace INT, thread_id INT, event_count INT, cpu_id INT, event_type_id INT)",
		"CREATE TABLE perf_report (id INT, report_type TEXT, report_value TEXT)",
		"INSERT INTO perf_report VALUES (9, 'config_name', 'hw-cpu-cycles')",
		"CREATE TABLE perf_callchain (callchain_id INT, depth INT, name TEXT, ip INT)",
		"INSERT INTO perf_callchain VALUES (88, 0, 'appspawn+0xc2f4', 372840035060)",
		"INSERT INTO perf_sample VALUES (88, 62380027704000, 62642, 1477992, 1, 9)",
	})
	outPath := filepath.Join(t.TempDir(), "perf-comm.systrace")
	if _, err := exportTraceDBToSystrace(context.Background(), path, outPath); err != nil {
		t.Fatalf("export perf DB systrace: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"s.watch.meetime-62642",
		`thread_comm="s.watch.meetime"`,
		`perf_thread_comm="com.huawei.hmos"`,
		`comm_source=trace_thread`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("perf DB systrace missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "com.huawei.hmos-62642") ||
		strings.Contains(body, `pid=62642 tid=62642 thread_comm="com.huawei.hmos"`) {
		t.Fatalf("perf DB systrace should not use stale perf_thread comm as primary thread label:\n%s", body)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse perf DB output: %v", err)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 62380.027, TimeEnd: 62380.028})
	if stats.PerfSamples == nil || len(stats.PerfSamples.TopThreads) == 0 {
		t.Fatalf("perf DB sample did not reach tracequery stats: %+v", stats.PerfSamples)
	}
	if got := stats.PerfSamples.TopThreads[0].Thread.Comm; got != "s.watch.meetime" {
		t.Fatalf("perf top thread comm = %q, want trace thread comm", got)
	}
}

func TestExportTraceDBPerfSamplesStrictScalarsAndCPUThreeState(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE perf_thread (thread_id INT, process_id INT, thread_name TEXT)",
		"INSERT INTO perf_thread VALUES (101, 101, 'worker')",
		"INSERT INTO perf_thread VALUES (202, -1, 'invalid-pid')",
		"CREATE TABLE perf_sample (callchain_id INT, timestamp_trace INT, thread_id INT, event_count INT, cpu_id INT)",
		"CREATE TABLE perf_callchain (callchain_id INT, depth INT, name TEXT)",
		"INSERT INTO perf_callchain VALUES (1, 0, 'leaf')",
		"INSERT INTO perf_sample VALUES (1, 1000, 101, 3, 0)",
		"INSERT INTO perf_sample VALUES (1, 1100, 101, 4, NULL)",
		"INSERT INTO perf_sample VALUES (1, 1200, 101, 5, -1)",
		"INSERT INTO perf_sample VALUES (1, 1300, 101, 6, 4096)",
		"INSERT INTO perf_sample VALUES (1, -1, 101, 7, 1)",
		"INSERT INTO perf_sample VALUES (1, 1.5, 101, 8, 1)",
		"INSERT INTO perf_sample VALUES (1, 1400, 0, 9, 1)",
		"INSERT INTO perf_sample VALUES (1, 1500, 1.5, 10, 1)",
		"INSERT INTO perf_sample VALUES (-1, 1600, 101, 11, 1)",
		"INSERT INTO perf_sample VALUES (1.5, 1700, 101, 12, 1)",
		"INSERT INTO perf_sample VALUES (1, 1800, 202, 13, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "perf-strict.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export strict perf DB systrace: %v", err)
	}
	var perfCoverage TraceDBCoverage
	for _, item := range result.Coverage {
		if item.Family == "perf" && item.Table == "perf_sample" {
			perfCoverage = item
			break
		}
	}
	if perfCoverage.RowsRead != 11 || perfCoverage.RowsEmitted != 2 {
		t.Fatalf("strict perf coverage mismatch: %+v", perfCoverage)
	}
	for _, want := range []string{
		"invalid_timestamp=2",
		"invalid_thread_id=2",
		"invalid_callchain_id=1",
		"invalid_cpu=1",
		"anonymous_cpu_unclaimed=2",
		"ambiguous_identity=1",
	} {
		if !strings.Contains(perfCoverage.Skipped, want) {
			t.Fatalf("strict perf coverage missing %q: %+v", want, perfCoverage)
		}
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Count(body, "perf_sample:") != 2 || strings.Contains(body, "tracing_mark_write:") {
		t.Fatalf("strict perf export should contain exactly two canonical samples and no markers:\n%s", body)
	}
	for _, want := range []string{
		"perf_sample: cpu=0 cpu_known=true pid=0 tid=0 thread_comm=\"\"",
		"sample_weight=11",
		"thread_identity_known=false resolution=perf_source_only lifecycle_unverified=true perf_source_tid=101",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("strict perf export missing %q:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse strict perf output: %v", err)
	}
	known := 0
	seenCPU := map[int]bool{}
	for _, ev := range idx.Events {
		if ev.Type != tracequery.EventPerfSample || ev.PerfFields == nil || ev.PerfFields.CPUKnown == nil {
			continue
		}
		if *ev.PerfFields.CPUKnown {
			known++
			seenCPU[ev.CPU] = true
		}
	}
	if known != 2 || !seenCPU[0] || !seenCPU[1] {
		t.Fatalf("strict anonymous CPU round trip mismatch: known=%d cpus=%v events=%+v", known, seenCPU, idx.Events)
	}
}

func TestExportTraceDBPerfSamplesMissingCPUColumnRemainsUnknown(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE perf_sample (callchain_id INT, timestamp_trace INT, thread_id INT, event_count INT)",
		"CREATE TABLE perf_callchain (callchain_id INT, depth INT, name TEXT)",
		"INSERT INTO perf_callchain VALUES (1, 0, 'leaf')",
		"INSERT INTO perf_sample VALUES (1, 1000, 101, 3)",
	})
	outPath := filepath.Join(t.TempDir(), "perf-no-cpu.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export perf DB without CPU column: %v", err)
	}
	for _, item := range result.Coverage {
		if item.Family == "perf" && item.Table == "perf_sample" {
			if item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "anonymous_cpu_unclaimed=1") {
				t.Fatalf("missing anonymous CPU did not fail closed: %+v", item)
			}
		}
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("zero-row export unexpectedly materialized a transport CPU: stat err=%v", err)
	}
}

func TestExportTraceDBPerfSamplesIgnoreNonAuthoritativeRegistrationHints(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (pid INT, ipid INT, name TEXT)",
		"INSERT INTO process VALUES (100, 1, 'old-process')",
		"INSERT INTO process VALUES (200, 2, 'new-process')",
		"CREATE TABLE thread (tid INT, itid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT, start_ts INT)",
		"INSERT INTO thread VALUES (42, 2, 2, 'new-thread', 0, 1, 100000)",
		"INSERT INTO thread VALUES (42, 1, 1, 'old-thread', 0, 1, 0)",
		"CREATE TABLE instant (ts, name, ref, ref_type)",
		"CREATE TABLE thread_state (ts, dur, itid, state, cpu)",
		"INSERT INTO thread_state VALUES (0, 200000, 2, 'Running', 1)",
		"CREATE TABLE sched_slice (ts, dur, itid, end_state)",
		"CREATE TABLE callstack (ts, itid, callid)",
		"CREATE TABLE syscall (ts, itid)",
		"CREATE TABLE native_hook (start_ts, itid)",
		"CREATE TABLE frame_slice (id, type, ts, itid)",
		"CREATE TABLE perf_thread (thread_id INT, process_id INT, thread_name TEXT)",
		"INSERT INTO perf_thread VALUES (42, 200, 'perf-new')",
		"CREATE TABLE perf_sample (callchain_id INT, timestamp_trace INT, thread_id INT, event_count INT, cpu_id INT)",
		"CREATE TABLE perf_callchain (callchain_id INT, depth INT, name TEXT)",
		"INSERT INTO perf_callchain VALUES (1, 0, 'leaf')",
		"INSERT INTO perf_sample VALUES (1, 1000, 42, 3, 1)",
		"INSERT INTO perf_sample VALUES (1, 100500, 42, 4, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "perf-tid-reuse.systrace")
	if _, err := exportTraceDBToSystrace(context.Background(), path, outPath); err != nil {
		t.Fatalf("export TID-reuse perf DB: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse TID-reuse perf output: %v", err)
	}
	var samples []tracequery.Event
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventPerfSample {
			samples = append(samples, ev)
		}
	}
	if len(samples) != 2 {
		t.Fatalf("TID-reuse fixture emitted %d perf samples, want 2: %+v", len(samples), samples)
	}
	if samples[0].TGID != 200 || samples[0].PerfFields.Comm != "new-thread" {
		t.Fatalf("registration hint rewrote the exact perf process candidate: event=%+v fields=%q", samples[0], samples[0].FieldText)
	}
	if samples[1].TGID != 200 || samples[1].PerfFields.Comm != "new-thread" {
		t.Fatalf("registration hint changed an otherwise identical hard resolution: %+v", samples[1])
	}
}

func TestTraceDBPerfCandidateResolutionDisclosesConflictAndAmbiguity(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "p100"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 200, Name: "p200-a"}
	index.Processes[3] = traceDBProcess{IPID: 3, PID: 200, Name: "p200-b"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 42, IPID: 1, Name: "trace-old"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 42, IPID: 2, Name: "trace-new-a"}
	index.ByITID[3] = traceDBThread{ITID: 3, TID: 42, IPID: 3, Name: "trace-new-b"}
	index.ByITID[4] = traceDBThread{ITID: 4, TID: 43, IPID: 2, Name: "single"}
	buildTraceDBThreadSecondaryIndexes(&index)

	tests := []struct {
		name           string
		tid            int64
		pid            int64
		pidKnown       bool
		wantResolution traceDBPerfThreadResolution
		wantTask       string
		wantPID        int64
	}{
		{name: "pid mismatch", tid: 42, pid: 300, pidKnown: true, wantResolution: traceDBPerfThreadPIDConflict},
		{name: "same pid multiple canonical candidates", tid: 42, pid: 200, pidKnown: true, wantResolution: traceDBPerfThreadAmbiguous},
		{name: "public tid only ambiguous", tid: 42, wantResolution: traceDBPerfThreadAmbiguous},
		{name: "missing canonical candidate", tid: 44, pid: 400, pidKnown: true, wantResolution: traceDBPerfThreadMissing},
		{name: "exact pid narrows unique candidate", tid: 43, pid: 200, pidKnown: true, wantResolution: traceDBPerfThreadResolved, wantTask: "single", wantPID: 200},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := traceDBResolvePerfSampleIdentity(index, traceDBPerfThreadCatalog{ByTID: map[int64]traceDBPerfThreadCatalogEntry{}, Tainted: map[int64]bool{}},
				test.tid, test.pid, test.pidKnown, "perf-source")
			if got.Resolution != test.wantResolution || got.Task != test.wantTask || got.PID != test.wantPID {
				t.Fatalf("resolution=%+v, want kind=%v task=%q pid=%d", got, test.wantResolution, test.wantTask, test.wantPID)
			}
		})
	}
}

func TestExportTraceDBPerfSampleDoesNotCrossPairWithCallstack(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (pid INT, ipid INT, name TEXT)",
		"INSERT INTO process VALUES (101, 1, 'demo')",
		"CREATE TABLE thread (tid INT, itid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT, start_ts INT)",
		"INSERT INTO thread VALUES (101, 1, 1, 'worker', 1, 1, 0)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 0, 10000, 2, 'Running')",
		"CREATE TABLE perf_thread (thread_id INT, process_id INT, thread_name TEXT)",
		"INSERT INTO perf_thread VALUES (101, 101, 'worker')",
		"CREATE TABLE perf_sample (callchain_id INT, timestamp_trace INT, thread_id INT, event_count INT, cpu_id INT)",
		"CREATE TABLE perf_callchain (callchain_id INT, depth INT, name TEXT)",
		"INSERT INTO perf_callchain VALUES (1, 0, 'VerifyClass Foo')",
		"INSERT INTO perf_sample VALUES (1, 1000, 101, 3, 2)",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie TEXT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1500, 5000, 1, 'DoWork', '', NULL, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "perf-callstack-cross-pair.systrace")
	if _, err := exportTraceDBToSystrace(context.Background(), path, outPath); err != nil {
		t.Fatalf("export perf/callstack interleave DB: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse perf/callstack interleave output: %v", err)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 0, TimeEnd: 0.00002})
	var doWork *tracequery.TraceSpanSummary
	for i := range stats.TraceSpans {
		if stats.TraceSpans[i].Name == "DoWork" {
			doWork = &stats.TraceSpans[i]
		}
		if strings.HasPrefix(stats.TraceSpans[i].Name, "hiperf:") || strings.Contains(stats.TraceSpans[i].Name, "VerifyClass") {
			t.Fatalf("perf sample minted a synthetic span: %+v", stats.TraceSpans)
		}
	}
	if doWork == nil || math.Abs(doWork.DurationMs-0.005) > 0.000001 {
		t.Fatalf("perf sample corrupted the real callstack span: %+v", stats.TraceSpans)
	}
	if stats.PerfSamples == nil || stats.PerfSamples.SampleCount != 1 {
		t.Fatalf("canonical perf sample was lost: %+v", stats.PerfSamples)
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
		{"slice", "callstack", 8},
		{"scheduler", "sched_slice", 1},
		{"scheduler", "instant", 2},
		{"irq", "irq", 4},
		{"counter", "measure", 2},
		{"counter", "measure_filter", 1},
		{"counter", "process_measure", 1},
		{"slice", "frame_slice", 2},
		{"counter", "network", 2},
		{"counter", "diskio", 2},
		{"counter", "cpu_usage", 3},
		{"counter", "live_process", 3},
		{"log", "log", 1},
		{"slice", "syscall", 2},
		{"slice", "task_pool", 2},
		{"slice", "app_startup", 2},
		{"slice", "static_initalize", 2},
		{"resource", "native_hook", 2},
		{"log", "hisys_all_event", 1},
		{"counter", "xpower_measure", 1},
		{"sorter", "__systrace_rows__", result.EventsWritten},
	} {
		assertCoverageEmitted(t, result.Coverage, key.family, key.table, key.min)
	}
	if !coverageHasSkipped(result.Coverage, "slice", "dma_fence", "high_level_rows_withheld=2") {
		t.Fatalf("hmtrace high-level DMA predecessor deltas must be withheld: %+v", result.Coverage)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"tracing_mark_write: B|500|DoWork",
		"tracing_mark_write: B|500|AllocTask",
		"tracing_mark_write: B|700|ExecTask",
		"sched_wakeup: comm=WorkerThread pid=501 prio=42 target_cpu=007",
		"sched_waking: comm=WorkerThread pid=501 prio=42 target_cpu=008",
		"softirq_entry: vec=9 [action=RCU]",
		"cpu_frequency: state=2200000 cpu_id=11",
		"tracing_mark_write: S|500|FrameActual-123|hconv-frame-1",
		"tracing_mark_write: F|500|FrameActual-123|hconv-frame-1",
		"tracing_mark_write: C|500|pss_kb|2048.0",
		"print: [I][TEST] hello world",
		"tracing_mark_write: B|500|AppStartup:coldStart",
		"tracing_mark_write: I|500|NativeHook:AllocEvent",
		"tracing_mark_write: C|500|HeapSize|8192",
		"print: SYS/BATTERY: low power",
		"tracing_mark_write: C|0|xpower_display|12.5",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("hmtrace comprehensive systrace missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "tracing_mark_write: B|0|dma_fence") ||
		strings.Contains(body, "tracing_mark_write: S|0|dma_fence") ||
		strings.Contains(body, "<dma_fence>") {
		t.Fatalf("hmtrace high-level DMA row minted a synthetic endpoint:\n%s", body)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse hmtrace comprehensive output: %v", err)
	}
	if len(idx.Events) < 20 {
		t.Fatalf("tracequery should parse comprehensive fixture rows, got %d", len(idx.Events))
	}
}

func TestExportTraceDBCallstackFailsClosedWithoutCompleteLifecycle(t *testing.T) {
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
	if !coverageHasSkipped(result.Coverage, "scheduler", "sched_slice", "missing table") {
		t.Fatalf("missing sched_slice should be visible without blocking thread_state resolver: %+v", result.Coverage)
	}
	schedulerScope := 0
	extendedScope := 0
	for _, item := range result.Coverage {
		if item.Family == "resolver" && item.Table == "thread_state" {
			switch item.FieldSources["running_consumer_scope"] {
			case "scheduler_lifecycle_gated":
				schedulerScope++
				if item.RowsRead != 2 || item.RowsEmitted != 0 ||
					!strings.Contains(item.Skipped, "scheduler_lifecycle_authority_complete=false") {
					t.Fatalf("incomplete scheduler Running authority failed open: %+v", item)
				}
			case "extended_all_consumers_lifecycle_gated":
				extendedScope++
				if item.RowsRead != 2 || item.RowsEmitted != 1 ||
					!strings.Contains(item.FieldSources["generation_admission"], "perf/raw/callstack/frame/native share one lifecycle-gated typed view") {
					t.Fatalf("extended shared Running compatibility changed or was not disclosed: %+v", item)
				}
			default:
				t.Fatalf("thread_state Running coverage lacks a closed consumer scope: %+v", item)
			}
		}
	}
	if schedulerScope != 1 || extendedScope != 1 {
		t.Fatalf("thread_state Running consumer scopes scheduler=%d extended=%d: %+v", schedulerScope, extendedScope, result.Coverage)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Contains(body, "ThreadStateWork") ||
		!coverageHasSkipped(result.Coverage, "slice", "callstack", "lifecycle_rejected_sync_closed_interval=1") {
		t.Fatalf("incomplete lifecycle authority allowed a callstack span:\n%s\n%+v", body, result.Coverage)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse thread_state resolver output: %v", err)
	}
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventTraceMark && ev.SpanName == "ThreadStateWork" {
			t.Fatalf("tracequery retained lifecycle-unverified callstack span: %+v", idx.Events)
		}
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
		"block_rq_issue: 8,0 R 4096 (READ) 128 + 8",
		"scsi_dispatch_cmd_start: tag=7 dev=8:0 lba=4096 len=8 opcode=READ_10",
		"mm_filemap_add_to_page_cache: dev 260:136 ino 0x3039 pfn=3062260 ofs=0",
		"workqueue_execute_start: work struct 0xabc: function 0xdef",
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
		"CREATE TABLE instant (ts, name, ref, ref_type)",
		"CREATE TABLE sched_slice (ts, dur, itid, end_state)",
		"CREATE TABLE callstack (ts, itid, callid)",
		"CREATE TABLE syscall (ts, itid)",
		"CREATE TABLE native_hook (start_ts, itid)",
		"CREATE TABLE frame_slice (id, type, ts, itid)",
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
		addArg(argset, "s_dev", int64(syntheticDev(260, 136)))
		addArg(argset, "i_ino", 12345)
		addArg(argset, "index", 0)
		addArg(argset, "pfn", 3062260)
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
	addArg(300, "bytes", 4096)
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
		"CREATE TABLE frame_slice (ts INT, dur INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
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
		"INSERT INTO thread_state VALUES (2, 1400, 200, 4, 5, 701, 700, 'Running', 0)",
		"INSERT INTO thread_state VALUES (7, 2700, 200, 6, 2, 501, 500, 'Running', 0)",
		"INSERT INTO thread_state VALUES (5, 3000, 200, 3, 2, 501, 500, 'Running', 0)",
		"INSERT INTO thread_state VALUES (6, 3150, 200, 4, 5, 701, 700, 'Running', 0)",
		"INSERT INTO thread_state VALUES (3, 3500, 200, 3, 2, 501, 500, 'Running', 0)",
		"INSERT INTO thread_state VALUES (4, 200000, 30000, 11, 2, 501, 500, 'Running', 0)",
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
		"INSERT INTO raw VALUES (2, 1510, 'sched_waking', 8, 5)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'handled')",
		"INSERT INTO data_dict VALUES (4, 'vec')",
		"INSERT INTO data_dict VALUES (5, 'coldStart')",
		"INSERT INTO data_dict VALUES (6, 'SYS')",
		"INSERT INTO data_dict VALUES (7, 'BATTERY')",
		"INSERT INTO data_dict VALUES (8, 'RCU')",
		"INSERT INTO args VALUES (1, 1, 0, 32, 10)",
		"INSERT INTO args VALUES (2, 2, 1, 3, 10)",
		"INSERT INTO args VALUES (3, 4, 0, 9, 20)",
		"INSERT INTO args VALUES (4, 2, 1, 8, 20)",
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
		"INSERT INTO frame_slice VALUES (205000, 16000, 'actural', 123, 1, 1, 2)",
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
