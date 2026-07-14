package hitraceconv

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func traceDBPerfB3AIdentity() traceDBThreadIndex {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 500, Name: "proc"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 50, IPID: 1, Name: "trace-worker"}
	index.ObservedPublicTID[50] = true
	buildTraceDBThreadSecondaryIndexes(&index)
	return index
}

func exportTraceDBPerfB3AFixture(t *testing.T, statements []string, index traceDBThreadIndex,
	lifecycle traceDBLifecycleIndex, complete bool, intervals map[int64][]traceDBRunningInterval,
) ([]TraceDBCoverage, string) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	authority := traceDBSchedulerAuthority{identities: index, lifecycle: lifecycle, initialized: true, complete: complete}
	running := newTraceDBSchedulerRunningIndex(authority, intervals, traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	sink, err := newTraceDBRowSink(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := exportTraceDBPerfSamples(context.Background(), tdb, sink, authority, running)
	if err != nil {
		t.Fatalf("export strict perf fixture: %v coverage=%+v", err, coverage)
	}
	rows := append([]traceDBStoredRow(nil), sink.rows...)
	sortTraceDBStoredRows(rows)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.line)
	}
	return coverage, strings.Join(lines, "\n")
}

func traceDBPerfCoverage(t *testing.T, coverage []TraceDBCoverage, table string) TraceDBCoverage {
	t.Helper()
	for _, item := range coverage {
		if item.Table == table && (item.Family == "perf" || item.Family == "resolver.perf") {
			return item
		}
	}
	t.Fatalf("missing perf coverage for %s: %+v", table, coverage)
	return TraceDBCoverage{}
}

func TestTraceDBPerfB3ASampleKindAndTypedCPUAuthority(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'perf-worker')",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, -1, 1000, 50, 1, -1, 'Running')",
		"INSERT INTO perf_sample VALUES (2, -1, 1001, 50, 2, 2, '-')",
		"INSERT INTO perf_sample VALUES (3, -1, 1002, 50, 3, 3, '-')",
		"INSERT INTO perf_sample VALUES (4, -1, 1003, 50, 4, 3, 'Running')",
		"INSERT INTO perf_sample VALUES (5, -1, 1004, 50, 5, -1, 'Suspend')",
		"INSERT INTO perf_sample VALUES (6, -1, 1005, 50, 6, 4, 'Suspend')",
		"INSERT INTO perf_sample VALUES (7, -1, 1006, 50, 7, 2, 'Bogus')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	item := traceDBPerfCoverage(t, coverage, "perf_sample")
	if item.RowsRead != 7 || item.RowsEmitted != 5 {
		t.Fatalf("sample kind/CPU coverage mismatch: %+v\n%s", item, body)
	}
	for _, token := range []string{
		"sample_weight=1", "sample_weight=2", "sample_weight=3", "sample_weight=6", "sample_weight=7",
		"sample_kind=on_cpu", "sample_kind_source=scheduler_running", "sample_kind=unknown", "sample_kind=off_cpu",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("strict sample kind output missing %q:\n%s", token, body)
		}
	}
	for _, token := range []string{"cpu_conflict_with_running=1", "off_cpu_cpu_unclaimed=1"} {
		if !strings.Contains(item.Skipped, token) {
			t.Fatalf("strict sample kind coverage missing %q: %+v", token, item)
		}
	}
	if !strings.Contains(item.Skipped, "thread_state_metadata_degraded=1") {
		t.Fatalf("bad thread_state metadata was not disclosed: %+v", item)
	}
	if strings.Contains(body, "sample_weight=4") || strings.Contains(body, "sample_weight=5") {
		t.Fatalf("rejected kind/CPU rows leaked:\n%s", body)
	}
}

func TestTraceDBPerfB3AAnonymousLifecycleAndRejectedRoster(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "old"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 200, Name: "new"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 42, IPID: 1, Name: "old"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 42, IPID: 2, Name: "new"}
	index.ObservedPublicTID[42] = true
	index.ObservedPublicTID[45] = true
	index.RejectedPublicTID[45] = true
	buildTraceDBThreadSecondaryIndexes(&index)
	old := traceDBLifecycleBoundary{TS: 0, NewITID: 1, NewIPID: 1}
	cut := traceDBLifecycleBoundary{TS: 1000, NewITID: 2, NewIPID: 2}
	lifecycle := traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{42: {Cuts: []traceDBLifecycleBoundary{old, cut}}},
		ByPID: map[int64]traceDBLifecycleLane{200: {Cuts: []traceDBLifecycleBoundary{cut}}},
	}
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (42, 200, 'source-new')",
		"INSERT INTO perf_thread VALUES (44, 400, 'source-only')",
		"INSERT INTO perf_thread VALUES (45, 450, 'rejected-source')",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, -1, 999, 42, 1, 1, 'Running')",
		"INSERT INTO perf_sample VALUES (2, -1, 1000, 42, 2, 1, 'Running')",
		"INSERT INTO perf_sample VALUES (3, -1, 1001, 44, 3, 3, '-')",
		"INSERT INTO perf_sample VALUES (4, -1, 1002, 45, 4, 3, '-')",
	}, index, lifecycle, true, nil)
	item := traceDBPerfCoverage(t, coverage, "perf_sample")
	if item.RowsEmitted != 2 || !strings.Contains(item.Skipped, "lifecycle_rejected=1") || !strings.Contains(item.Skipped, "ambiguous_identity=1") {
		t.Fatalf("lifecycle/anonymous resolution mismatch: %+v\n%s", item, body)
	}
	for _, token := range []string{
		"sample_weight=2", "thread_identity_known=true resolution=resolved lifecycle_unverified=false",
		"sample_weight=3", "perf-unverified-0", "pid=0 tid=0 thread_comm=\"\"",
		"thread_identity_known=false resolution=perf_source_only lifecycle_unverified=true",
		"perf_source_tid=44 perf_source_pid=400 perf_source_comm=\"source-only\"",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("identity wire missing %q:\n%s", token, body)
		}
	}
	if strings.Contains(body, "rejected-source") || strings.Contains(body, "sample_weight=1") || strings.Contains(body, "sample_weight=4") {
		t.Fatalf("lifecycle/rejected identity row leaked:\n%s", body)
	}
}

func TestTraceDBPerfB3AResolverMaterializationAndUint64Frames(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'a-name')",
		"INSERT INTO perf_thread VALUES (50, 500, 'z-name')",
		"CREATE TABLE perf_report (id, report_type, report_value)",
		"INSERT INTO perf_report VALUES (9, 'config_name', 'cycles')",
		"INSERT INTO perf_report VALUES (9, 'config_name', 'instructions')",
		"INSERT INTO perf_report VALUES (10, 'event_name', 'must-not-label')",
		"CREATE TABLE perf_files (file_id, serial_id, symbol, path)",
		"INSERT INTO perf_files VALUES (7, -1, NULL, '/system/lib64/libx.so')",
		"CREATE TABLE perf_callchain (id, callchain_id, depth, name, ip, file_id, symbol_id)",
		"INSERT INTO perf_callchain VALUES (2, 10, 0, 'root', 16, -1, -1)",
		"INSERT INTO perf_callchain VALUES (3, 10, 2, -1, -1, 7, -1)",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, event_type_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, 10, 1000, 50, 9, 2, 9, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	item := traceDBPerfCoverage(t, coverage, "perf_sample")
	if item.RowsEmitted != 1 || strings.Count(body, "perf_sample:") != 1 {
		t.Fatalf("resolver fanout duplicated or lost sample: %+v\n%s", item, body)
	}
	for _, token := range []string{
		`event="perf"`, `symbol="0xffffffffffffffff"`, `dso="/system/lib64/libx.so"`, `ip="0xffffffffffffffff"`,
		`callchain="root;0xffffffffffffffff"`, "symbolization_status=partial", "callchain_status=partial", "sample_weight=9",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("uint64/path-only frame output missing %q:\n%s", token, body)
		}
	}
	if got := traceDBPerfCoverage(t, coverage, "perf_callchain"); got.RowsRead != 2 || got.RowsEmitted != 2 {
		t.Fatalf("callchain materialization mismatch: %+v", got)
	}
	if got := traceDBPerfCoverage(t, coverage, "perf_thread"); got.RowsRead != 2 || got.RowsEmitted != 1 {
		t.Fatalf("perf thread rename cohort fanned out: %+v", got)
	}
	if got := traceDBPerfCoverage(t, coverage, "perf_report"); got.RowsEmitted != 0 || !strings.Contains(got.Skipped, "non_config_name=1") || !strings.Contains(got.Skipped, "duplicate_id=2") {
		t.Fatalf("non-config/conflicting report metadata became an event label: %+v", got)
	}
}

func TestTraceDBPerfB3ANegativeCallchainRowIDDegradesMetadata(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
		"CREATE TABLE perf_callchain (id, callchain_id, depth, name, ip, file_id, symbol_id)",
		"INSERT INTO perf_callchain VALUES (-1, 11, 0, 'must-not-resolve', -1, -1, -1)",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, 11, 1000, 50, 1, 2, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	if item := traceDBPerfCoverage(t, coverage, "perf_sample"); item.RowsEmitted != 1 {
		t.Fatalf("metadata-only resolver failure dropped sample: %+v\n%s", item, body)
	}
	if item := traceDBPerfCoverage(t, coverage, "perf_callchain"); item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "invalid_stable_id=1") {
		t.Fatalf("negative canonical callchain row id was accepted: %+v", item)
	}
	if !strings.Contains(body, `symbol="perf_sample"`) || strings.Contains(body, "must-not-resolve") {
		t.Fatalf("invalid callchain metadata did not degrade to unsymbolized inventory:\n%s", body)
	}
	if !strings.Contains(body, "symbolization_status=unsymbolized") || !strings.Contains(body, "callchain_status=missing") {
		t.Fatalf("missing resolver metadata was mislabeled symbolized:\n%s", body)
	}
}

func TestTraceDBPerfB3APureIPCallchainIsNotSymbolized(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	_, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
		"CREATE TABLE perf_callchain (id, callchain_id, depth, name, ip, file_id, symbol_id)",
		"INSERT INTO perf_callchain VALUES (1, 12, 0, -1, 4660, -1, -1)",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, 12, 1000, 50, 1, 2, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	for _, token := range []string{`symbol="0x1234"`, `callchain="0x1234"`, "symbolization_status=unsymbolized", "callchain_status=ip_only"} {
		if !strings.Contains(body, token) {
			t.Fatalf("pure-IP callchain missing %q or mislabeled:\n%s", token, body)
		}
	}
}

func TestTraceDBPerfB3ADuplicateDepthPoisonsResolverOnly(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
		"CREATE TABLE perf_callchain (id, callchain_id, depth, name, ip)",
		"INSERT INTO perf_callchain VALUES (1, 13, 0, 'same', 16)",
		"INSERT INTO perf_callchain VALUES (2, 13, 0, 'same', 16)",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, 13, 1000, 50, 1, 2, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	if item := traceDBPerfCoverage(t, coverage, "perf_callchain"); item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "conflicting_depth=1") {
		t.Fatalf("duplicate depth survived unique materialization: %+v", item)
	}
	if item := traceDBPerfCoverage(t, coverage, "perf_sample"); item.RowsEmitted != 1 {
		t.Fatalf("resolver conflict deleted core sample: %+v", item)
	}
	if !strings.Contains(body, `symbol="perf_sample"`) || !strings.Contains(body, "callchain_status=missing") {
		t.Fatalf("duplicate depth did not degrade metadata only:\n%s", body)
	}
}

func TestTraceDBPerfB3AResolverTaintCannotRescueLowerPrioritySymbol(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (5, 'dict-first')",
		"INSERT INTO data_dict VALUES (5, 'dict-second')",
		"CREATE TABLE perf_files (file_id, serial_id, symbol, path)",
		"INSERT INTO perf_files VALUES (7, -1, 'file-first', '/first.so')",
		"INSERT INTO perf_files VALUES (7, -1, 'file-second', '/second.so')",
		"INSERT INTO perf_files VALUES (8, 1, 'file-rescue', '/trusted-path.so')",
		"CREATE TABLE hmtrace_perf_symbolized_frame (perf_callchain_row_id, display_name)",
		"INSERT INTO hmtrace_perf_symbolized_frame VALUES (1, 'symbol-first')",
		"INSERT INTO hmtrace_perf_symbolized_frame VALUES (1, 'symbol-second')",
		"CREATE TABLE perf_callchain (id, callchain_id, depth, name, ip, file_id, symbol_id)",
		"INSERT INTO perf_callchain VALUES (1, 20, 0, 'raw-rescue', 16, -1, -1)",
		"INSERT INTO perf_callchain VALUES (2, 21, 0, 5, 32, 8, 1)",
		"INSERT INTO perf_callchain VALUES (3, 22, 0, 'raw-trusted', 48, 7, -1)",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, 20, 1000, 50, 1, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (2, 21, 1001, 50, 2, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (3, 22, 1002, 50, 3, 2, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	if item := traceDBPerfCoverage(t, coverage, "perf_sample"); item.RowsEmitted != 3 {
		t.Fatalf("resolver taint deleted core sample: %+v\n%s", item, body)
	}
	for _, forbidden := range []string{"raw-rescue", "symbol-first", "symbol-second", "dict-first", "dict-second", "file-rescue", "file-first", "file-second", "/first.so", "/second.so"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("tainted resolver was rescued through lower-priority metadata %q:\n%s", forbidden, body)
		}
	}
	for _, token := range []string{
		`sample_weight=1 event="perf" symbol="0x10"`,
		`sample_weight=2 event="perf" symbol="0x20" dso="/trusted-path.so"`,
		`sample_weight=3 event="perf" symbol="raw-trusted" dso="unknown"`,
		"symbolization_status=partial", "callchain_status=partial",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("taint-preserving metadata output missing %q:\n%s", token, body)
		}
	}
	for _, table := range []string{"data_dict", "perf_files", "hmtrace_perf_symbolized_frame"} {
		if item := traceDBPerfCoverage(t, coverage, table); len(item.Skipped) == 0 {
			t.Fatalf("resolver taint missing coverage for %s: %+v", table, item)
		}
	}
}

func TestTraceDBPerfB3AOfficialAddressLabelsRemainUnsymbolized(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	_, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
		"CREATE TABLE perf_callchain (id, callchain_id, depth, name, ip)",
		"INSERT INTO perf_callchain VALUES (1, 30, 0, '@0x123', NULL)",
		"INSERT INTO perf_callchain VALUES (2, 30, 1, '+0x456', NULL)",
		"INSERT INTO perf_callchain VALUES (3, 30, 2, '0x789', NULL)",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, 30, 1000, 50, 1, 2, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	for _, token := range []string{`symbol="0x789"`, `callchain="@0x123;+0x456;0x789"`, "symbolization_status=unsymbolized", "callchain_status=ip_only"} {
		if !strings.Contains(body, token) {
			t.Fatalf("official address-only label missing %q or mislabeled:\n%s", token, body)
		}
	}
	if traceDBPerfAddressOnlyLabel("Render+0x10") || traceDBPerfAddressOnlyLabel("0xRenderer") || !traceDBPerfAddressOnlyLabel("@0xabcdef") {
		t.Fatal("address-only closed vocabulary drifted")
	}
}

func TestTraceDBPerfB3AResolverTextLimitDegradesWithoutFatalWire(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	long := strings.Repeat("x", maxTraceDBIdentityDisplayBytes+1)
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
		"CREATE TABLE perf_report (id, report_type, report_value)",
		fmt.Sprintf("INSERT INTO perf_report VALUES (9, 'config_name', '%s')", long),
		"CREATE TABLE hmtrace_perf_symbolized_frame (perf_callchain_row_id, display_name)",
		fmt.Sprintf("INSERT INTO hmtrace_perf_symbolized_frame VALUES (1, '%s')", long),
		"CREATE TABLE perf_callchain (id, callchain_id, depth, name, ip)",
		"INSERT INTO perf_callchain VALUES (1, 31, 0, 'raw-must-not-rescue', 64)",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, event_type_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, 31, 1000, 50, 1, 2, 9, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	if item := traceDBPerfCoverage(t, coverage, "perf_sample"); item.RowsEmitted != 1 {
		t.Fatalf("oversize resolver text turned metadata degradation into fatal sample loss: %+v", item)
	}
	if strings.Contains(body, long) || strings.Contains(body, "raw-must-not-rescue") || !strings.Contains(body, `event="perf" symbol="0x40"`) {
		t.Fatalf("oversize resolver text leaked or was rescued:\n%s", body)
	}
	for _, table := range []string{"perf_report", "hmtrace_perf_symbolized_frame"} {
		if item := traceDBPerfCoverage(t, coverage, table); len(item.Skipped) == 0 {
			t.Fatalf("oversize %s metadata was not disclosed: %+v", table, item)
		}
	}
}

func TestTraceDBPerfB3ASymbolizationFourStateClosedSet(t *testing.T) {
	tests := []struct {
		name          string
		frames        []traceDBPerfFrame
		symbolization string
		callchain     string
		wire          string
		complete      bool
	}{
		{name: "missing", symbolization: "unsymbolized", callchain: "missing", complete: true},
		{name: "ip only", frames: []traceDBPerfFrame{{Name: "0x1", IP: "0x1"}}, symbolization: "unsymbolized", callchain: "ip_only", wire: "0x1", complete: true},
		{name: "official address", frames: []traceDBPerfFrame{{Name: "@0x2", AddressOnly: true}}, symbolization: "unsymbolized", callchain: "ip_only", wire: "@0x2", complete: true},
		{name: "ip plus dso", frames: []traceDBPerfFrame{{Name: "0x1", IP: "0x1", DSO: "lib.so", DSOKnown: true}}, symbolization: "partial", callchain: "partial", wire: "0x1", complete: true},
		{name: "dso only", frames: []traceDBPerfFrame{{DSO: "lib.so", DSOKnown: true}}, symbolization: "partial", callchain: "partial", complete: false},
		{name: "fully symbolized", frames: []traceDBPerfFrame{{Name: "root", Symbolized: true}, {Name: "leaf", Symbolized: true}}, symbolization: "symbolized", callchain: "symbolized", wire: "root;leaf", complete: true},
		{name: "symbol plus missing", frames: []traceDBPerfFrame{{Name: "root", Symbolized: true}, {}}, symbolization: "partial", callchain: "partial", complete: false},
		{name: "tainted ip", frames: []traceDBPerfFrame{{Name: "0x1", IP: "0x1", Degraded: true}}, symbolization: "partial", callchain: "partial", wire: "0x1", complete: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			symbolization, callchain := traceDBPerfSymbolizationStatus(test.frames)
			wire, complete := traceDBPerfCallchain(test.frames)
			if symbolization != test.symbolization || callchain != test.callchain || wire != test.wire || complete != test.complete {
				t.Fatalf("status=(%q,%q) wire=(%q,%t), want (%q,%q,%q,%t)", symbolization, callchain, wire, complete,
					test.symbolization, test.callchain, test.wire, test.complete)
			}
		})
	}
	tooWide := []traceDBPerfFrame{
		{Name: strings.Repeat("a", 3000), Symbolized: true},
		{Name: strings.Repeat("b", 3000), Symbolized: true},
	}
	if wire, complete := traceDBPerfCallchain(tooWide); wire != "" || complete {
		t.Fatalf("aggregate resolver text escaped the %d-byte callchain bound: (%d,%t)", maxTraceDBIdentityDisplayBytes, len(wire), complete)
	}
}

func TestTraceDBPerfB3ACPUWitnessStatusMatrix(t *testing.T) {
	identity := traceDBPerfIdentityResult{
		Resolution: traceDBPerfThreadResolved,
		Thread:     traceDBThread{ITID: 1, TID: 50, IPID: 1},
		Process:    traceDBProcess{IPID: 1, PID: 500},
	}
	known := traceDBSchedulerRunningIndex{
		intervals:    map[int64][]traceDBRunningInterval{1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}}},
		rejectedITID: map[int64]bool{}, sourceTaintedITID: map[int64]bool{}, lifecycleRejectedITID: map[int64]bool{}, initialized: true,
	}
	tests := []struct {
		name      string
		raw       any
		hasColumn bool
		kind      traceDBPerfSampleKind
		running   traceDBSchedulerRunningIndex
		cpu       int64
		known     bool
		wantKind  traceDBPerfSampleKind
		reason    string
	}{
		{name: "minus one no claim", raw: int64(-1), hasColumn: true, kind: traceDBPerfSampleKindOnCPU, running: known, cpu: 2, known: true, wantKind: traceDBPerfSampleKindOnCPU},
		{name: "null no claim", raw: nil, hasColumn: true, kind: traceDBPerfSampleKindOnCPU, running: known, cpu: 2, known: true, wantKind: traceDBPerfSampleKindOnCPU},
		{name: "missing column no claim", hasColumn: false, kind: traceDBPerfSampleKindOnCPU, running: known, cpu: 2, known: true, wantKind: traceDBPerfSampleKindOnCPU},
		{name: "source tainted", raw: int64(-1), hasColumn: true, kind: traceDBPerfSampleKindOnCPU, running: traceDBSchedulerRunningIndex{sourceTaintedITID: map[int64]bool{1: true}, lifecycleRejectedITID: map[int64]bool{}, intervals: map[int64][]traceDBRunningInterval{}, initialized: true}, wantKind: traceDBPerfSampleKindOnCPU, reason: "tainted_running_cpu_witness"},
		{name: "lifecycle rejected", raw: int64(-1), hasColumn: true, kind: traceDBPerfSampleKindOnCPU, running: traceDBSchedulerRunningIndex{sourceTaintedITID: map[int64]bool{}, lifecycleRejectedITID: map[int64]bool{1: true}, intervals: map[int64][]traceDBRunningInterval{}, initialized: true}, wantKind: traceDBPerfSampleKindOnCPU, reason: "lifecycle_rejected_running_cpu_witness"},
		{name: "unknown running", raw: int64(-1), hasColumn: true, kind: traceDBPerfSampleKindOnCPU, running: traceDBSchedulerRunningIndex{sourceTaintedITID: map[int64]bool{}, lifecycleRejectedITID: map[int64]bool{}, intervals: map[int64][]traceDBRunningInterval{}, initialized: true}, wantKind: traceDBPerfSampleKindOnCPU, reason: "unknown_running_cpu_witness"},
		{name: "invalid explicit never falls back", raw: int64(-2), hasColumn: true, kind: traceDBPerfSampleKindOnCPU, running: known, wantKind: traceDBPerfSampleKindOnCPU, reason: "invalid_cpu"},
		{name: "CPU0 independent", raw: int64(0), hasColumn: true, kind: traceDBPerfSampleKindOnCPU, running: traceDBSchedulerRunningIndex{sourceTaintedITID: map[int64]bool{}, lifecycleRejectedITID: map[int64]bool{}, intervals: map[int64][]traceDBRunningInterval{}, initialized: true}, cpu: 0, known: true, wantKind: traceDBPerfSampleKindOnCPU},
		{name: "unknown upgraded", raw: int64(2), hasColumn: true, kind: traceDBPerfSampleKindUnknown, running: known, cpu: 2, known: true, wantKind: traceDBPerfSampleKindOnCPU},
		{name: "unknown CPU conflict stays unknown", raw: int64(3), hasColumn: true, kind: traceDBPerfSampleKindUnknown, running: known, cpu: 3, known: true, wantKind: traceDBPerfSampleKindUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cpu, cpuKnown, kind, _, reason := traceDBResolvePerfSampleCPU(test.raw, test.hasColumn, test.kind, identity, test.running, 1000)
			if cpu != test.cpu || cpuKnown != test.known || kind != test.wantKind || reason != test.reason {
				t.Fatalf("got cpu=(%d,%t) kind=%v reason=%q, want (%d,%t,%v,%q)", cpu, cpuKnown, kind, reason,
					test.cpu, test.known, test.wantKind, test.reason)
			}
		})
	}
}

func TestTraceDBPerfB3AAuthorityPointMatrix(t *testing.T) {
	base := traceDBPerfB3AIdentity()
	boundary := func(itid, ipid int64) traceDBLifecycleBoundary {
		return traceDBLifecycleBoundary{TS: 1000, NewITID: itid, NewIPID: ipid}
	}
	tests := []struct {
		name      string
		mutate    func(*traceDBThreadIndex)
		lifecycle traceDBLifecycleIndex
		complete  bool
		emit      int
		reason    string
	}{
		{name: "clean", complete: true, emit: 1},
		{name: "incomplete", reason: "lifecycle_rejected=1"},
		{name: "capture before", complete: true, mutate: func(index *traceDBThreadIndex) { index.TraceStart = 1001 }, reason: "before_capture_start=1"},
		{name: "global taint", complete: true, lifecycle: traceDBLifecycleIndex{GlobalTaint: true}, reason: "lifecycle_rejected=1"},
		{name: "global poison", complete: true, lifecycle: traceDBLifecycleIndex{GlobalPoison: []int64{1000}}, reason: "lifecycle_rejected=1"},
		{name: "thread poison", complete: true, lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{50: {PoisonPoints: []int64{1000}}}}, reason: "lifecycle_rejected=1"},
		{name: "process poison", complete: true, lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{500: {PoisonPoints: []int64{1000}}}}, reason: "lifecycle_rejected=1"},
		{name: "process cut to other", complete: true, lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{500: {Cuts: []traceDBLifecycleBoundary{boundary(2, 2)}}}}, reason: "lifecycle_rejected=1"},
		{name: "same identity cut", complete: true, lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{50: {Cuts: []traceDBLifecycleBoundary{boundary(1, 1)}}}, ByPID: map[int64]traceDBLifecycleLane{500: {Cuts: []traceDBLifecycleBoundary{boundary(1, 1)}}}}, emit: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index := base
			if test.mutate != nil {
				test.mutate(&index)
			}
			coverage, _ := exportTraceDBPerfB3AFixture(t, []string{
				"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
				"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
				"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
				"INSERT INTO perf_sample VALUES (1, -1, 1000, 50, 1, 2, 'Running')",
			}, index, test.lifecycle, test.complete, nil)
			item := traceDBPerfCoverage(t, coverage, "perf_sample")
			if item.RowsEmitted != test.emit || test.reason != "" && !strings.Contains(item.Skipped, test.reason) {
				t.Fatalf("authority matrix mismatch: %+v want emit=%d reason=%q", item, test.emit, test.reason)
			}
		})
	}
}

func TestTraceDBPerfB3AThreadCatalogOrderAndMixedPID(t *testing.T) {
	load := func(t *testing.T, rows []string) traceDBPerfThreadCatalog {
		t.Helper()
		statements := []string{"CREATE TABLE perf_thread (thread_id, process_id, thread_name)"}
		statements = append(statements, rows...)
		path := createTraceDBFixture(t, statements)
		tdb, err := openTraceDB(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer tdb.close()
		catalog, _, err := tdb.loadStrictPerfThreadCatalog(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return catalog
	}
	forward := load(t, []string{
		"INSERT INTO perf_thread VALUES (50, 500, 'a-name')",
		"INSERT INTO perf_thread VALUES (50, 500, 'z-name')",
		"INSERT INTO perf_thread VALUES (51, 500, 'mixed-a')",
		"INSERT INTO perf_thread VALUES (51, 501, 'mixed-b')",
	})
	reverse := load(t, []string{
		"INSERT INTO perf_thread VALUES (51, 501, 'mixed-b')",
		"INSERT INTO perf_thread VALUES (51, 500, 'mixed-a')",
		"INSERT INTO perf_thread VALUES (50, 500, 'z-name')",
		"INSERT INTO perf_thread VALUES (50, 500, 'a-name')",
	})
	if forward.ByTID[50] != reverse.ByTID[50] || forward.ByTID[50].Name != "z-name" || !forward.Tainted[51] || !reverse.Tainted[51] {
		t.Fatalf("perf_thread catalog depends on row order or rescued mixed PID: forward=%+v reverse=%+v", forward, reverse)
	}
}

func TestTraceDBPerfB3ACurrentTimestampStableIDAndWeight(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (2, -1, 1000, 50, 2, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (1, -1, 1000, 50, 1, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (3, -1, 1000, 50, 3, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (3, -1, 1000, 50, 4, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (-1, -1, 1000, 50, 5, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (4, -1, 1000, 50, NULL, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (5, -1, 1000, 50, 0, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (6, -1, 1000, 50, 1.0, 2, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	item := traceDBPerfCoverage(t, coverage, "perf_sample")
	if item.RowsRead != 8 || item.RowsEmitted != 3 || !strings.Contains(item.Skipped, "duplicate_stable_id=2") || !strings.Contains(item.Skipped, "invalid_weight=3") {
		t.Fatalf("stable/weight coverage mismatch: %+v\n%s", item, body)
	}
	first := strings.Index(body, "sample_weight=1")
	second := strings.Index(body, "sample_weight=2")
	last := strings.Index(body, "sample_weight=5")
	if first < 0 || second <= first || last <= second {
		t.Fatalf("same-ts canonical stable order mismatch: first=%d second=%d last=%d\n%s", first, second, last, body)
	}

	coverage, _ = exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_sample (id, callchain_id, ts, thread_id, cpu_id)",
		"INSERT INTO perf_sample VALUES (1, -1, 1000, 50, 2)",
	}, index, traceDBLifecycleIndex{}, true, nil)
	item = traceDBPerfCoverage(t, coverage, "perf_sample")
	if item.RowsEmitted != 0 || !strings.Contains(strings.Join(item.ColumnsMissing, ","), "timestamp_trace") {
		t.Fatalf("current profile accepted legacy timestamp alias: %+v", item)
	}
	coverage, _ = exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_sample (callchain_id, timeStamp, thread_id, cpu_id)",
		"INSERT INTO perf_sample VALUES (-1, 1000, 50, 2)",
	}, index, traceDBLifecycleIndex{}, true, nil)
	item = traceDBPerfCoverage(t, coverage, "perf_sample")
	if item.RowsEmitted != 0 || !strings.Contains(strings.Join(item.ColumnsMissing, ","), "timestamp_trace") {
		t.Fatalf("id-less raw-clock profile was promoted without attestation: %+v", item)
	}
}

func TestTraceDBPerfB3ACallchainIDProducerDomain(t *testing.T) {
	index := traceDBPerfB3AIdentity()
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (50, 500, 'worker')",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, 4294967294, 1000, 50, 1, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (2, 4294967295, 1001, 50, 2, 2, 'Running')",
		"INSERT INTO perf_sample VALUES (3, 9223372036854775807, 1002, 50, 3, 2, 'Running')",
	}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
		1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
	})
	item := traceDBPerfCoverage(t, coverage, "perf_sample")
	if item.RowsEmitted != 1 || !strings.Contains(item.Skipped, "invalid_callchain_id=2") || !strings.Contains(body, "sample_weight=1") || strings.Contains(body, "sample_weight=2") || strings.Contains(body, "sample_weight=3") {
		t.Fatalf("callchain uint32 producer domain drifted: %+v\n%s", item, body)
	}
}

func TestTraceDBThreadIndexRetainsRejectedPublicTIDRoster(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 500, 'proc')",
		"CREATE TABLE thread (itid, tid, ipid, name, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 50, 1, 'good', 0, 1)",
		"INSERT INTO thread VALUES (2, 55, 999, 'bad-owner', 0, 1)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !index.ObservedPublicTID[55] || !index.RejectedPublicTID[55] || len(index.ByTIDCandidates[55]) != 0 {
		t.Fatalf("rejected public TID was erased into false absence: %+v", index)
	}
	if index.RejectedPublicTID[50] || len(index.ByTIDCandidates[50]) != 1 {
		t.Fatalf("valid sibling was tainted by rejected roster: %+v", index)
	}
}
