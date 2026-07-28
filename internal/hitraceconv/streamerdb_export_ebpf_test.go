package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestExportTraceDBEBPFIntervalsOfficialSchemas(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE ebpf_callstack (id INTEGER, callchain_id INTEGER, depth INTEGER, ip TEXT, symbols_id INTEGER, file_path_id INTEGER)",
		"INSERT INTO ebpf_callstack VALUES (0, 5, 0, '0x1', 1, 2)",
		"INSERT INTO ebpf_callstack VALUES (1, 5, 1, '0x2', 3, 4)",
		"INSERT INTO ebpf_callstack VALUES (2, 6, 1, '0x3', 5, 6)",
		"CREATE TABLE file_system_sample (id INTEGER, callchain_id INTEGER, type INTEGER, ipid INTEGER, itid INTEGER, start_ts INTEGER, end_ts INTEGER, dur INTEGER, return_value TEXT, error_code TEXT, fd INTEGER, file_id INTEGER, size INTEGER, first_argument TEXT, second_argument TEXT, third_argument TEXT, fourth_argument TEXT)",
		"INSERT INTO file_system_sample VALUES (0, 5, 2, 1, 2, 100, 150, 50, '4', NULL, 3, 9, 4096, 'buf', 'len', NULL, NULL)",
		"INSERT INTO file_system_sample VALUES (1, 6, 3, 1, 2, 160, 180, 20, '4', NULL, 3, 9, 4, 'buf', NULL, NULL, NULL)",
		"INSERT INTO file_system_sample VALUES (2, 5, 2, 1, 2, 190, 180, 10, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL)",
		"CREATE TABLE paged_memory_sample (id INTEGER, callchain_id INTEGER, type INTEGER, ipid INTEGER, itid INTEGER, start_ts INTEGER, end_ts INTEGER, dur INTEGER, size INTEGER, addr TEXT)",
		"INSERT INTO paged_memory_sample VALUES (0, -1, 1, 1, 2, 200, 210, 10, 8192, '0xabc')",
		"CREATE TABLE bio_latency_sample (id INTEGER, callchain_id INTEGER, type INTEGER, ipid INTEGER, itid INTEGER, start_ts INTEGER, end_ts INTEGER, latency_dur INTEGER, tier INTEGER, size INTEGER, block_number TEXT, path_id TEXT, dur_per_4k INTEGER)",
		"INSERT INTO bio_latency_sample VALUES (0, 5, 1, 1, 2, 220, 260, 40, 2, 4096, '0x10', '9', 40)",
	})
	ctx := context.Background()
	tdb, err := openTraceDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "app"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 200, IPID: 1, Name: "worker"}
	buildTraceDBThreadSecondaryIndexes(&index)
	authority := traceDBTestCompleteSchedulerAuthority(index)
	sink, err := newTraceDBRowSink(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := exportTraceDBEBPFIntervals(ctx, tdb, sink, authority)
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 4 || coverage[0].RowsEmitted != 1 ||
		coverage[1].RowsEmitted != 2 || coverage[2].RowsEmitted != 1 || coverage[3].RowsEmitted != 1 {
		t.Fatalf("eBPF coverage drifted: %+v", coverage)
	}
	if !strings.Contains(coverage[0].Skipped, "incomplete_or_ambiguous_callchains=1") ||
		!strings.Contains(coverage[1].Skipped, "invalid_end=1") ||
		coverage[1].Metrics["unavailable_callchain_endpoints"] != 1 {
		t.Fatalf("eBPF fail-closed diagnostics drifted: %+v", coverage)
	}
	outPath := filepath.Join(t.TempDir(), "ebpf.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.prepareAndWriteForTest(ctx, out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "# codrax_ebpf_interval/v1 ") != 4 {
		t.Fatalf("eBPF typed row count drifted:\n%s", body)
	}
	idx, err := tracequery.BuildIndex(ctx, outPath)
	if err != nil {
		t.Fatal(err)
	}
	typeCounts := map[tracequery.EventType]int{}
	callStatuses := map[string]int{}
	for _, event := range idx.Events {
		typeCounts[event.Type]++
		if event.PluginFields != nil && event.PluginFields.EBPFInterval != nil {
			fields := event.PluginFields.EBPFInterval
			if fields.IdentityStatus != "resolved" || event.PID != 200 || event.TGID != 100 {
				t.Fatalf("eBPF identity drifted: %+v", event)
			}
			callStatuses[fields.CallchainStatus]++
		}
	}
	if typeCounts[tracequery.EventEBPFInterval] != 4 ||
		callStatuses["available"] != 2 || callStatuses["unavailable"] != 1 ||
		callStatuses["absent"] != 1 {
		t.Fatalf("eBPF typed event census drifted: types=%v calls=%v events=%+v",
			typeCounts, callStatuses, idx.Events)
	}
}
