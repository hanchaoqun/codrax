package hitraceconv

import (
	"context"
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
		"cpu_idle: state=1.0 cpu_id=1",
		"cpu_frequency: state=2200000.0 cpu_id=4",
		"clock_set_rate: ddr_freq state=400.0 cpu_id=0",
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
