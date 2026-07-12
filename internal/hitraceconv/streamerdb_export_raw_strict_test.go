package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTraceDBRawAuthorityFixture(t *testing.T, statements []string) string {
	t.Helper()
	out := append([]string(nil), statements...)
	hasTable := func(name string) bool {
		needle := "create table " + name + " "
		for _, statement := range out {
			if strings.Contains(strings.ToLower(statement), needle) {
				return true
			}
		}
		return false
	}
	for _, schema := range []struct {
		table string
		ddl   string
	}{
		{table: "instant", ddl: "CREATE TABLE instant (ts, name, ref, ref_type)"},
		{table: "sched_slice", ddl: "CREATE TABLE sched_slice (ts, dur, itid, end_state)"},
		{table: "thread_state", ddl: "CREATE TABLE thread_state (itid, ts, dur, cpu, state)"},
		{table: "callstack", ddl: "CREATE TABLE callstack (ts, itid, callid)"},
		{table: "syscall", ddl: "CREATE TABLE syscall (ts, itid)"},
		{table: "native_hook", ddl: "CREATE TABLE native_hook (start_ts, itid)"},
		{table: "frame_slice", ddl: "CREATE TABLE frame_slice (id, type, ts, itid)"},
	} {
		if !hasTable(schema.table) {
			out = append(out, schema.ddl)
		}
	}
	return createTraceDBFixture(t, out)
}

func TestExportTraceDBRawFtraceStrictScalarsCPU0ArgsetsAndTIDReuse(t *testing.T) {
	path := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'old-process')",
		"INSERT INTO process VALUES (2, 200, 'new-process')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 42, 1, 'old-name', 0, 0, 1)",
		"INSERT INTO thread VALUES (1, 42, 1, 'old-renamed', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 42, 2, 'new-name', 2000000, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 0, 1500000, 7, 'Running')",
		"INSERT INTO thread_state VALUES (2, 2000000, 2000000, 8, 'Running')",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'work')",
		"INSERT INTO data_dict VALUES (2, 'function')",
		"INSERT INTO data_dict VALUES (3, ' Work ')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (0, 1, 0, 2748)",
		"INSERT INTO args VALUES (0, 2, 0, 3567)",
		"INSERT INTO args VALUES (1, 1, 0, 2748)",
		"INSERT INTO args VALUES (1, 2, 0, 3567)",
		"INSERT INTO args VALUES (2, 1, 0, 1)",
		"INSERT INTO args VALUES (2, 3, 0, 2)",
		"INSERT INTO args VALUES (2, 2, 0, 3)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, tid, pid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000000, 'workqueue_execute_start', 0, 1, 42, 100, 0)",
		"INSERT INTO raw VALUES (2, 1100000, 'workqueue_execute_end', NULL, 1, 42, 100, 1)",
		"INSERT INTO raw VALUES (3, 2500000, 'workqueue_execute_start', NULL, NULL, 42, 200, 0)",
		"INSERT INTO raw VALUES (4, 2600000, 'workqueue_execute_end', NULL, NULL, 42, 200, 1)",
		"INSERT INTO raw VALUES (5, 2700000, 'workqueue_execute_start', 8, 2, 42, 200, NULL)",
		"INSERT INTO raw VALUES (6, 2800000, 'workqueue_execute_start', 8, 2, 42, 200, 2)",
		"INSERT INTO raw VALUES (7, 1.5, 'workqueue_execute_start', 8, 2, 42, 200, 0)",
		"INSERT INTO raw VALUES (8, 2900000, 'workqueue_execute_start', 1.5, 2, 42, 200, 0)",
		"INSERT INTO raw VALUES (9, 3000000, 'workqueue_execute_start', 8, 1.5, 42, 200, 0)",
		"INSERT INTO raw VALUES (10, 3100000, 'workqueue_execute_start', 8, NULL, 42, 100, 0)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-strict.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export strict raw ftrace: %v", err)
	}
	var schema, workqueue TraceDBCoverage
	for _, item := range result.Coverage {
		switch {
		case item.Family == "raw_ftrace" && item.Table == "raw":
			schema = item
		case item.Family == "raw_ftrace" && item.Table == "workqueue":
			workqueue = item
		}
	}
	if schema.RowsRead != 10 || schema.RowsEmitted != 0 {
		t.Fatalf("raw schema coverage double-counted or mis-accounted: %+v", schema)
	}
	for _, want := range []string{"missing_argset=1", "missing_required_args=1", "invalid_cpu=1", "invalid_itid=1"} {
		if !strings.Contains(workqueue.Skipped, want) {
			t.Fatalf("workqueue coverage missing %q: %+v", want, workqueue)
		}
	}
	if !strings.Contains(schema.Skipped, "invalid_timestamp=1") {
		t.Fatalf("schema coverage missing strict timestamp rejection: %+v", schema)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Contains(body, "workqueue_execute_") || !strings.Contains(schema.Skipped, "pairing_or_duplicate_rows_fail_closed") {
		t.Fatalf("unknown-key/malformed WQ rows rescued neighboring endpoints:\ncoverage=%+v\n%s", schema, body)
	}
}

func TestTraceDBRawExactITIDDominatesReusedPublicTIDCandidates(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "old-process"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 200, Name: "new-process"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 42, IPID: 1, Name: "old-thread"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 42, IPID: 2, Name: "new-thread"}
	buildTraceDBThreadSecondaryIndexes(&index)
	authority := traceDBTestCompleteSchedulerAuthority(index)

	tests := []struct {
		name     string
		raw      traceDBRawEvent
		wantOK   bool
		wantTID  int64
		wantTGID int64
		wantITID int64
	}{
		{name: "exact itid with agreeing tid", raw: traceDBRawEvent{TS: 10, ITID: 2, ITIDKnown: true, TID: 42, TIDKnown: true},
			wantOK: true, wantTID: 42, wantTGID: 200, wantITID: 2},
		{name: "exact itid with conflicting tid", raw: traceDBRawEvent{TS: 10, ITID: 2, ITIDKnown: true, TID: 43, TIDKnown: true}},
		{name: "exact itid with conflicting pid", raw: traceDBRawEvent{TS: 10, ITID: 2, ITIDKnown: true, TID: 42, TIDKnown: true, PID: 100, PIDKnown: true}},
		{name: "tid only remains ambiguous", raw: traceDBRawEvent{TS: 10, TID: 42, TIDKnown: true}},
		{name: "exact pid narrows tid only", raw: traceDBRawEvent{TS: 10, TID: 42, TIDKnown: true, PID: 200, PIDKnown: true},
			wantOK: true, wantTID: 42, wantTGID: 200, wantITID: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subject, reason := traceDBResolveRawSubject(test.raw, authority, true, true)
			ok := reason == ""
			tid, tgid, itid := subject.TID, subject.TGID, subject.ITID
			if ok != test.wantOK || ok && (tid != test.wantTID || tgid != test.wantTGID || itid != test.wantITID) {
				t.Fatalf("context=(tid=%d tgid=%d itid=%d ok=%t reason=%q), want (%d,%d,%d,%t)",
					tid, tgid, itid, ok, reason, test.wantTID, test.wantTGID, test.wantITID, test.wantOK)
			}
		})
	}
}

func TestExportTraceDBRawFtraceRunningTaintBlocksInferenceButNotExplicitCPU(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 0, 5000000, 7, 'Running')",
		"INSERT INTO thread_state VALUES (1, 6000000, 1000000, 4096, 'Running')",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'work')",
		"INSERT INTO data_dict VALUES (2, 'function')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 0, 1)",
		"INSERT INTO args VALUES (1, 2, 0, 2)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000000, 'workqueue_execute_start', NULL, 1, 1)",
		"INSERT INTO raw VALUES (2, 2000000, 'workqueue_execute_start', 0, 1, 1)",
		"CREATE TABLE callstack (id, ts, dur, callid, name, flag, cookie, chainId)",
		"INSERT INTO callstack VALUES (1, 3000000, 100000, 1, 'tainted-callstack', '', NULL, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-running-taint.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export tainted running fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Contains(body, "workqueue_execute_start:") {
		t.Fatalf("tainted endpoint was deleted without quarantining its exact WQ lane:\n%s", body)
	}
	if strings.Contains(body, "tainted-callstack") {
		t.Fatalf("callstack consumed a tainted Running CPU witness:\n%s", body)
	}
	if !coverageHasSkipped(result.Coverage, "slice", "callstack", "tainted_running_cpu_witness") {
		t.Fatalf("callstack running taint was not disclosed: %+v", result.Coverage)
	}
}

func TestExportTraceDBRawFtraceConflictingIdleIdentityCannotMintCPU(t *testing.T) {
	path := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (0, 0, 'kernel')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (0, 0, 0, 'swapper', 0, 0, 1)",
		"INSERT INTO thread VALUES (0, 1, 0, 'forged-idle', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		"INSERT INTO thread_state VALUES (0, 0, 5000000, 7, 'Running', 0, 0)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'work')",
		"INSERT INTO data_dict VALUES (2, 'function')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 0, 1)",
		"INSERT INTO args VALUES (1, 2, 0, 2)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000000, 'workqueue_execute_start', NULL, 0, 1)",
		"INSERT INTO raw VALUES (2, 2000000, 'workqueue_execute_start', 0, 0, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-idle-running-taint.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export conflicting idle fixture: %v", err)
	}
	bodyBytes, readErr := os.ReadFile(outPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if strings.Contains(string(bodyBytes), "workqueue_execute_start:") || result.EventsWritten != 0 {
		t.Fatalf("conflicting idle identity must block both inferred and explicit CPU rows: events=%d\n%s",
			result.EventsWritten, bodyBytes)
	}
	if !coverageHasSkipped(result.Coverage, "resolver", "thread_state", "ambiguous_idle_identity=1") {
		t.Fatalf("idle Running identity conflict was not disclosed: %+v", result.Coverage)
	}
	if !coverageHasSkipped(result.Coverage, "raw_ftrace", "workqueue", "idle_lifecycle_rejected=2") {
		t.Fatalf("idle lifecycle rejection was not disclosed: %+v", result.Coverage)
	}
}

func TestExportTraceDBRawFtracePerKeyPoisonAndWorkqueueEndShape(t *testing.T) {
	path := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 42, 1, 'worker', 0, 1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'work')",
		"INSERT INTO data_dict VALUES (2, 'function')",
		"INSERT INTO data_dict VALUES (3, 'vendor_noise')",
		"INSERT INTO data_dict VALUES (4, 'Work')",
		"INSERT INTO data_dict VALUES (5, 'addr')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 0, 2748)",
		"INSERT INTO args VALUES (1, 2, 0, 3567)",
		"INSERT INTO args VALUES (1, 3, 2, 1.5)",
		"INSERT INTO args VALUES (2, 1, 0, 2748)",
		"INSERT INTO args VALUES (3, 1, 0, 1)",
		"INSERT INTO args VALUES (3, 5, 0, 2)",
		"INSERT INTO args VALUES (3, 2, 0, 3)",
		"INSERT INTO args VALUES (4, 4, 0, 1)",
		"INSERT INTO args VALUES (4, 2, 0, 3)",
		"INSERT INTO args VALUES (5, 1, 0, 0)",
		"INSERT INTO args VALUES (5, 2, 0, 3)",
		"CREATE TABLE raw (id, ts, name, cpu, tid, pid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000, 'workqueue_execute_start', 0, 42, 100, 1)",
		"INSERT INTO raw VALUES (2, 1100, 'workqueue_execute_end', 0, 42, 100, 2)",
		"INSERT INTO raw VALUES (3, 1200, 'workqueue_execute_start', 0, 42, 100, 3)",
		"INSERT INTO raw VALUES (4, 1300, 'workqueue_execute_start', 0, 42, 100, 4)",
		"INSERT INTO raw VALUES (5, 1400, 'workqueue_execute_start', 0, 42, 100, 5)",
		"INSERT INTO raw VALUES (6, 1500, 'workqueue_queue_work', 0, 42, 100, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-arg-poison.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export raw per-key fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Contains(body, "workqueue_execute_") || strings.Contains(body, "workqueue_queue_work:") {
		t.Fatalf("unknown/ambiguous WQ hard keys failed to close the governed family:\n%s", body)
	}
	if !coverageHasSkipped(result.Coverage, "raw_ftrace", "workqueue", "missing_required_args=3") {
		t.Fatalf("required-key poison/alias/pointer rejections not disclosed: %+v", result.Coverage)
	}
}

func TestExportTraceDBRawFtraceStableIDOrderAndDuplicatePoison(t *testing.T) {
	path := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 42, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 42, 1, 'worker', 1, 1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'work')",
		"INSERT INTO data_dict VALUES (2, 'function')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (10, 1, 0, 16)",
		"INSERT INTO args VALUES (10, 2, 0, 256)",
		"INSERT INTO args VALUES (20, 1, 0, 32)",
		"INSERT INTO args VALUES (20, 2, 0, 512)",
		"INSERT INTO args VALUES (30, 1, 0, 48)",
		"INSERT INTO args VALUES (30, 2, 0, 768)",
		"INSERT INTO args VALUES (40, 1, 0, 64)",
		"INSERT INTO args VALUES (40, 2, 0, 1024)",
		"CREATE TABLE raw (id, ts, name, cpu, tid, argsetid)",
		"INSERT INTO raw VALUES (20, 1000, 'workqueue_execute_start', 1, 42, 20)",
		"INSERT INTO raw VALUES (10, 1000, 'workqueue_execute_start', 1, 42, 10)",
		"INSERT INTO raw VALUES (30, 1000, 'workqueue_execute_start', 1, 42, 30)",
		"INSERT INTO raw VALUES (30, 1001, 'workqueue_execute_start', 1, 42, 30)",
		"INSERT INTO raw VALUES (NULL, 1002, 'workqueue_execute_start', 1, 42, 40)",
		"INSERT INTO raw VALUES (40, 1003, 'Workqueue_execute_start', 1, 42, 10)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-stable-id.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export raw stable-id fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	first := strings.Index(body, "work=0x10 function=0x100")
	second := strings.Index(body, "work=0x20 function=0x200")
	if first < 0 || second < 0 || first >= second {
		t.Fatalf("same-timestamp raw rows were not ordered by raw.id:\n%s", body)
	}
	if strings.Contains(body, "work=0x30") {
		t.Fatalf("duplicate raw.id rows were not poisoned as a cohort:\n%s", body)
	}
	var schema TraceDBCoverage
	for _, item := range result.Coverage {
		if item.Family == "raw_ftrace" && item.Table == "raw" {
			schema = item
		}
	}
	for _, want := range []string{"duplicate_source_id=2", "invalid_stable_id=1", "invalid_event_name=1"} {
		if !strings.Contains(schema.Skipped, want) {
			t.Fatalf("stable-id coverage missing %q: %+v", want, schema)
		}
	}
	if schema.FieldSources["stable_identity"] != "raw.id" ||
		schema.FieldSources["stable_identity_projection"] != "raw.id exact full-uint32 signed-int32 projection" ||
		schema.FieldSources["same_timestamp_order"] != "raw.ts,canonical_uint32(raw.id)" {
		t.Fatalf("stable source provenance missing: %+v", schema)
	}
}

func TestExportTraceDBRawFtraceRequiresCanonicalThreadIdentityAndSupportsWithoutRowIDArgsets(t *testing.T) {
	path := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 42, 1, 'worker', 1, 1)",
		"CREATE TABLE data_dict (id INTEGER PRIMARY KEY, data TEXT) WITHOUT ROWID",
		"INSERT INTO data_dict VALUES (1, 'work')",
		"INSERT INTO data_dict VALUES (2, 'function')",
		"CREATE TABLE args (argset INTEGER, key INTEGER, datatype INTEGER, value INTEGER, PRIMARY KEY (argset, key)) WITHOUT ROWID",
		"INSERT INTO args VALUES (1, 1, 0, 1)",
		"INSERT INTO args VALUES (1, 2, 0, 2)",
		"CREATE TABLE raw (id, ts, name, cpu, tid, pid, callid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000, 'workqueue_execute_start', 0, 42, 100, NULL, 1)",
		"INSERT INTO raw VALUES (2, 1100, 'workqueue_execute_start', 0, NULL, 100, NULL, 1)",
		"INSERT INTO raw VALUES (3, 1200, 'workqueue_execute_start', 0, NULL, NULL, 1, 1)",
		"INSERT INTO raw VALUES (4, 1300, 'workqueue_execute_start', 0, 2147483648, 100, NULL, 1)",
		"INSERT INTO raw VALUES (5, 1400, 'workqueue_execute_start', 0, 42, 2147483648, NULL, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-identity-without-rowid.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export raw identity fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Contains(body, "workqueue_execute_start:") {
		t.Fatalf("unknown header owner rescued a canonical sibling in the same WQ family:\n%s", body)
	}
	for _, want := range []string{"missing_thread_identity=2", "invalid_tid=1", "invalid_pid=1"} {
		if !coverageHasSkipped(result.Coverage, "raw_ftrace", "workqueue", want) {
			t.Fatalf("identity boundary coverage missing %q: %+v", want, result.Coverage)
		}
	}
}

func TestTraceDBRawRequiredArgsTypedEndpointMatrix(t *testing.T) {
	intValue := func(text string) traceDBValue { return traceDBValue{Valid: true, Text: text, Datatype: 0} }
	textValue := func(text string) traceDBValue { return traceDBValue{Valid: true, Text: text, Datatype: 1} }

	binder := map[string]traceDBValue{
		"transaction": intValue("42"), "dest_node": intValue("9"), "dest_proc": intValue("500"),
		"dest_thread": intValue("700"), "reply": intValue("0"), "flags": intValue("18"), "code": intValue("4"),
	}
	if !traceDBRawRequiredArgs("binder_transaction", binder, nil) {
		t.Fatal("typed binder endpoint was rejected")
	}
	binder["transaction"] = textValue("not-an-id")
	if traceDBRawRequiredArgs("binder_transaction", binder, nil) {
		t.Fatal("text binder transaction escaped strict endpoint validation")
	}

	block := map[string]traceDBValue{
		"dev": textValue("8,0"), "rwbs": textValue("R"), "cmd": textValue("READ"),
		"sector": intValue("128"), "nr_sector": intValue("8"), "bytes": intValue("4096"),
	}
	if !traceDBRawRequiredArgs("block_rq_issue", block, nil) {
		t.Fatal("typed block request was rejected")
	}
	delete(block, "nr_sector")
	block["bytes"] = intValue("4096")
	if traceDBRawRequiredArgs("block_rq_issue", block, nil) {
		t.Fatal("byte count was misinterpreted as sector count")
	}
	delete(block, "bytes")
	block["nr_sector"] = intValue("0")
	block["bytes"] = intValue("0")
	block["rwbs"] = textValue("FS")
	if !traceDBRawRequiredArgs("block_rq_issue", block, nil) {
		t.Fatal("witnessed zero-sector flush request was rejected")
	}
	block["rwbs"] = textValue("R")
	if !traceDBRawRequiredArgs("block_rq_issue", block, nil) {
		t.Fatal("typed zero-sector non-flush inventory was rejected by the converter")
	}

	dma := map[string]traceDBValue{
		"driver": textValue("drv"), "timeline": textValue("tl"), "context": intValue("1"), "seqno": intValue("2"),
	}
	if !traceDBRawRequiredArgs("dma_fence_signaled", dma, nil) {
		t.Fatal("typed dma fence endpoint was rejected")
	}
	dma["context"] = textValue("1")
	if traceDBRawRequiredArgs("dma_fence_signaled", dma, nil) {
		t.Fatal("string dma context escaped integer validation")
	}
}

func TestExportTraceDBRawBlockOptionalGrammarFailureIsDisclosed(t *testing.T) {
	path := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'io', 0, 1, 1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'dev')",
		"INSERT INTO data_dict VALUES (2, 'sector')",
		"INSERT INTO data_dict VALUES (3, 'nr_sector')",
		"INSERT INTO data_dict VALUES (4, 'bytes')",
		"INSERT INTO data_dict VALUES (5, 'rwbs')",
		"INSERT INTO data_dict VALUES (6, 'cmd')",
		"INSERT INTO data_dict VALUES (10, '8,0')",
		"INSERT INTO data_dict VALUES (11, 'R')",
		"INSERT INTO data_dict VALUES (12, 'READ')",
		"INSERT INTO data_dict VALUES (13, 'READ) 0 + 1')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 1, 10)",
		"INSERT INTO args VALUES (1, 2, 0, 128)",
		"INSERT INTO args VALUES (1, 3, 0, 8)",
		"INSERT INTO args VALUES (1, 4, 0, 4096)",
		"INSERT INTO args VALUES (1, 5, 1, 11)",
		"INSERT INTO args VALUES (1, 6, 1, 12)",
		"INSERT INTO args VALUES (2, 1, 1, 10)",
		"INSERT INTO args VALUES (2, 2, 0, 129)",
		"INSERT INTO args VALUES (2, 3, 0, 8)",
		"INSERT INTO args VALUES (2, 4, 0, 4096)",
		"INSERT INTO args VALUES (2, 5, 1, 11)",
		"INSERT INTO args VALUES (2, 6, 1, 13)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000, 'block_rq_issue', 0, 1, 1)",
		"INSERT INTO raw VALUES (2, 2000, 'block_rq_issue', 0, 1, 2)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-block-optional.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export strict optional block fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Count(body, "block_rq_issue:") != 1 || !strings.Contains(body, "8,0 R 4096 (READ) 128 + 8 []") || strings.Contains(body, "READ) 0 + 1") {
		t.Fatalf("optional block grammar rejection leaked or removed the valid sibling:\n%s", body)
	}
	if !coverageHasSkipped(result.Coverage, "raw_ftrace", "block_storage", "missing_required_args=1") {
		t.Fatalf("optional block grammar rejection was not disclosed: %+v", result.Coverage)
	}
}
