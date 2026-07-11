package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestTraceDBSchedStartsStrictCohortsAndHarmonyRTPriorities(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid)",
		"INSERT INTO process VALUES (1, 1, 500)",
		"CREATE TABLE thread (id, itid, tid, ipid, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (10, 10, 1010, 1, 0, 0, 1)",
		"INSERT INTO thread VALUES (11, 11, 1011, 1, 0, 0, 1)",
		"INSERT INTO thread VALUES (12, 12, 1012, 1, 0, 0, 1)",
		"INSERT INTO thread VALUES (13, 13, 1013, 1, 0, 0, 1)",
		"INSERT INTO thread VALUES (14, 14, 1014, 1, 0, 0, 1)",
		"INSERT INTO thread VALUES (15, 15, 1015, 1, 0, 0, 1)",
		"CREATE TABLE sched_slice (itid, ts, cpu, priority)",
		"INSERT INTO sched_slice VALUES (0, 0, 0, -1)",
		"INSERT INTO sched_slice VALUES (10, 100, 4095, 140)",
		"INSERT INTO sched_slice VALUES (10, 200, 3, 159)",
		"INSERT INTO sched_slice VALUES (10, 300, 3, 160)",
		"INSERT INTO sched_slice VALUES (10, 400, 3, 2147483646)",
		"INSERT INTO sched_slice VALUES (11, 100, 2, NULL)",
		"INSERT INTO sched_slice VALUES (12, 100, 2, 2147483647)",
		"INSERT INTO sched_slice VALUES (13, 100, CAST(2 AS TEXT), 77)",
		"INSERT INTO sched_slice VALUES (14, 100, 2, CAST(77 AS TEXT))",
		"INSERT INTO sched_slice VALUES (15, 100, 2, 77)",
		"INSERT INTO sched_slice VALUES (15, 100, 2, 78)",
		"INSERT INTO sched_slice VALUES (10, 500, 4, 42)",
		"INSERT INTO sched_slice VALUES (10, 500, 4, 42)",
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
	starts, coverage, err := tdb.loadSchedStarts(context.Background(), index)
	if err != nil {
		t.Fatal(err)
	}
	assertSchedPriority := func(itid, ts, want int64) {
		t.Helper()
		got, known := traceDBNextSchedPriority(starts, itid, ts)
		if !known || got != want {
			t.Fatalf("priority itid=%d ts=%d got=(%d,%t), want=(%d,true); starts=%+v coverage=%+v", itid, ts, got, known, want, starts, coverage)
		}
	}
	assertSchedPriority(0, 0, -1)
	assertSchedPriority(10, 0, 140)
	assertSchedPriority(10, 101, 159)
	assertSchedPriority(10, 201, 160)
	assertSchedPriority(10, 301, math.MaxInt32-1)
	assertSchedPriority(10, 401, 42)
	for _, itid := range []int64{11, 12, 13, 14, 15} {
		if got, known := traceDBNextSchedPriority(starts, itid, 0); known {
			t.Fatalf("invalid/poisoned sched cohort itid=%d became authoritative priority=%d starts=%+v coverage=%+v", itid, got, starts, coverage)
		}
	}
	if coverage.RowsRead != 13 || coverage.RowsEmitted != 6 || coverage.Skipped == "" {
		t.Fatalf("strict sched-start coverage mismatch: %+v", coverage)
	}
}

func TestTraceDBSchedStartPoisonIsANextSliceBarrier(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid)",
		"INSERT INTO process VALUES (1, 500)",
		"CREATE TABLE thread (itid, tid, ipid, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (10, 1010, 1, 0, 0, 1)",
		"CREATE TABLE sched_slice (itid, ts, cpu, priority)",
		"INSERT INTO sched_slice VALUES (10, 400, 2, NULL)",
		"INSERT INTO sched_slice VALUES (10, 500, 3, 77)",
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
	starts, _, err := tdb.loadSchedStarts(context.Background(), index)
	if err != nil {
		t.Fatal(err)
	}
	if got, known := traceDBNextSchedPriority(starts, 10, 350); known {
		t.Fatalf("poisoned nearest sched point was skipped in favor of later priority=%d: %+v", got, starts)
	}
	if got, known := traceDBNextSchedPriority(starts, 10, 401); !known || got != 77 {
		t.Fatalf("barrier before query must not poison a later exact next point: got=(%d,%t) starts=%+v", got, known, starts)
	}
}

func TestTraceDBActiveThreadIDsUseAuditedCanonicalIdentity(t *testing.T) {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid)",
		"INSERT INTO process VALUES (1, 1, 500)",
		"CREATE TABLE thread (id, itid, tid, ipid, start_ts, is_main_thread, switch_count)",
	}
	for itid := int64(1); itid <= 16; itid++ {
		statements = append(statements, fmt.Sprintf("INSERT INTO thread VALUES (%d, %d, %d, 1, 0, 1, 0)", itid, itid, 1000+itid))
	}
	statements = append(statements, "INSERT INTO thread VALUES (4294967294, 4294967294, 2000, 1, 0, 1, 0)")
	statements = append(statements,
		"CREATE TABLE callstack (itid, callid)",
		"INSERT INTO callstack VALUES (2, 2)",
		"INSERT INTO callstack VALUES (3, 4)",
		"INSERT INTO callstack VALUES (5, NULL)",
		"INSERT INTO callstack VALUES (NULL, 6)",
		"INSERT INTO callstack VALUES (CAST(7 AS TEXT), NULL)",
		"INSERT INTO callstack VALUES (NULL, 8.0)",
		"CREATE TABLE sched_slice (itid)",
		"INSERT INTO sched_slice VALUES (0)",
		"INSERT INTO sched_slice VALUES (9)",
		"INSERT INTO sched_slice VALUES (CAST(10 AS TEXT))",
		"INSERT INTO sched_slice VALUES (11.0)",
		"INSERT INTO sched_slice VALUES (x'3132')",
		"INSERT INTO sched_slice VALUES (4294967295)",
		"INSERT INTO sched_slice VALUES (4294967294)",
		"INSERT INTO sched_slice VALUES (-1)",
		"INSERT INTO sched_slice VALUES (4294967296)",
		"INSERT INTO sched_slice VALUES (NULL)",
		"CREATE TABLE thread_state (itid)",
		"INSERT INTO thread_state VALUES (13)",
		"CREATE TABLE syscall (itid)",
		"INSERT INTO syscall VALUES (14)",
		"CREATE TABLE native_hook (itid)",
		"INSERT INTO native_hook VALUES (15)",
		"CREATE TABLE frame_slice (itid)",
		"INSERT INTO frame_slice VALUES (16)",
	)
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	active, coverage, err := tdb.loadActiveThreadIDs(context.Background(), index)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]bool{0: true, 2: true, 5: true, 6: true, 9: true, 13: true, 14: true, 15: true, 16: true, 4294967294: true}
	if !reflect.DeepEqual(active, want) {
		t.Fatalf("active IDs bypassed audited canonical identity:\n got=%+v\nwant=%+v\ncoverage=%+v", active, want, coverage)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	registrationCoverage, err := exportTraceDBThreadRegistrations(context.Background(), sink, index, active)
	if err != nil {
		t.Fatal(err)
	}
	if registrationCoverage.RowsEmitted != (len(want)-1)*3 || sink.stats.RowsAccepted != (len(want)-1)*3 {
		t.Fatalf("malformed activity reference registered dormant main: active=%+v coverage=%+v sink=%+v", active, registrationCoverage, sink.stats)
	}
}

func TestTraceDBSchedStartsAreOrderIndependent(t *testing.T) {
	rows := []string{
		"INSERT INTO sched_slice VALUES (10, 100, 0, 140)",
		"INSERT INTO sched_slice VALUES (10, 100, 0, 140)",
		"INSERT INTO sched_slice VALUES (11, 110, 1, 140)",
		"INSERT INTO sched_slice VALUES (11, 110, 1, 159)",
		"INSERT INTO sched_slice VALUES (12, 120, 2, NULL)",
		"INSERT INTO sched_slice VALUES (12, 120, 2, 120)",
		"INSERT INTO sched_slice VALUES (13, 200, 3, 160)",
		"INSERT INTO sched_slice VALUES (CAST(10 AS TEXT), 150, 3, 77)",
		"INSERT INTO sched_slice VALUES (14, CAST(170 AS TEXT), 3, 77)",
	}
	load := func(t *testing.T, reverse bool) (traceDBSchedStartIndex, TraceDBCoverage) {
		t.Helper()
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid)", "INSERT INTO process VALUES (1, 500)",
			"CREATE TABLE thread (itid, tid, ipid, start_ts, is_main_thread, switch_count)",
		}
		for itid := 10; itid <= 14; itid++ {
			statements = append(statements, fmt.Sprintf("INSERT INTO thread VALUES (%d, %d, 1, 0, 0, 1)", itid, 1000+itid))
		}
		statements = append(statements, "CREATE TABLE sched_slice (itid, ts, cpu, priority)")
		ordered := append([]string(nil), rows...)
		if reverse {
			for left, right := 0, len(ordered)-1; left < right; left, right = left+1, right-1 {
				ordered[left], ordered[right] = ordered[right], ordered[left]
			}
		}
		statements = append(statements, ordered...)
		path := createTraceDBFixture(t, statements)
		tdb, err := openTraceDB(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer tdb.close()
		index, _, err := tdb.loadThreadIndex(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		starts, coverage, err := tdb.loadSchedStarts(context.Background(), index)
		if err != nil {
			t.Fatal(err)
		}
		coverage.ElapsedUS = 0
		return starts, coverage
	}
	forward, forwardCoverage := load(t, false)
	reverse, reverseCoverage := load(t, true)
	if !reflect.DeepEqual(forward, reverse) || !reflect.DeepEqual(forwardCoverage, reverseCoverage) {
		t.Fatalf("sched-start authority depends on row order:\nforward=%+v %+v\nreverse=%+v %+v", forward, forwardCoverage, reverse, reverseCoverage)
	}
	if priority, known := traceDBNextSchedPriority(forward, 10, 0); !known || priority != 140 {
		t.Fatalf("exact duplicate lost authoritative value: got=(%d,%t) %+v", priority, known, forward)
	}
	if _, known := traceDBNextSchedPriority(forward, 11, 0); known {
		t.Fatalf("priority conflict did not poison exact key: %+v", forward)
	}
	if _, known := traceDBNextSchedPriority(forward, 12, 0); known {
		t.Fatalf("NULL+valid sibling did not poison exact key: %+v", forward)
	}
	if _, known := traceDBNextSchedPriority(forward, 13, 0); known {
		t.Fatalf("unknown-identity timestamp barrier was skipped: %+v", forward)
	}
	if priority, known := traceDBNextSchedPriority(forward, 13, 151); !known || priority != 160 {
		t.Fatalf("past global barrier poisoned later exact point: got=(%d,%t) %+v", priority, known, forward)
	}
	if _, known := traceDBNextSchedPriority(forward, 14, 1000); known || !forward.TaintedITIDs[14] {
		t.Fatalf("known itid with unplaceable timestamp did not taint its lane: %+v", forward)
	}
}

func TestTraceDBSchedStartUnplaceableIdentityAndTimestampTaintsGlobally(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid)", "INSERT INTO process VALUES (1, 500)",
		"CREATE TABLE thread (itid, tid, ipid, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (10, 1010, 1, 0, 0, 1)",
		"CREATE TABLE sched_slice (itid, ts, cpu, priority)",
		"INSERT INTO sched_slice VALUES (10, 100, 1, 140)",
		"INSERT INTO sched_slice VALUES (CAST(10 AS TEXT), CAST(90 AS TEXT), 1, 140)",
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
	starts, coverage, err := tdb.loadSchedStarts(context.Background(), index)
	if err != nil {
		t.Fatal(err)
	}
	if !starts.GlobalTaint || coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "global_unplaceable_taint=1") {
		t.Fatalf("unplaceable malformed sched row did not globally fail closed: starts=%+v coverage=%+v", starts, coverage)
	}
	if _, known := traceDBNextSchedPriority(starts, 10, 0); known {
		t.Fatalf("global taint still returned an authoritative priority: %+v", starts)
	}
}

func TestTraceDBActiveCallstackIdentityProfiles(t *testing.T) {
	tests := []struct {
		name         string
		threadDDL    string
		threadRow    string
		callstackDDL string
		callstackRow string
	}{
		{name: "current callid only", threadDDL: "CREATE TABLE thread (id, itid, tid, ipid, start_ts, is_main_thread, switch_count)", threadRow: "INSERT INTO thread VALUES (2, 2, 102, 1, 0, 0, 0)", callstackDDL: "CREATE TABLE callstack (callid)", callstackRow: "INSERT INTO callstack VALUES (2)"},
		{name: "current itid only", threadDDL: "CREATE TABLE thread (id, itid, tid, ipid, start_ts, is_main_thread, switch_count)", threadRow: "INSERT INTO thread VALUES (2, 2, 102, 1, 0, 0, 0)", callstackDDL: "CREATE TABLE callstack (itid)", callstackRow: "INSERT INTO callstack VALUES (2)"},
		{name: "current convergent dual", threadDDL: "CREATE TABLE thread (id, itid, tid, ipid, start_ts, is_main_thread, switch_count)", threadRow: "INSERT INTO thread VALUES (2, 2, 102, 1, 0, 0, 0)", callstackDDL: "CREATE TABLE callstack (itid, callid)", callstackRow: "INSERT INTO callstack VALUES (2, 2)"},
		{name: "idless callid compatibility", threadDDL: "CREATE TABLE thread (itid, tid, ipid, start_ts, is_main_thread, switch_count)", threadRow: "INSERT INTO thread VALUES (2, 102, 1, 0, 0, 0)", callstackDDL: "CREATE TABLE callstack (callid)", callstackRow: "INSERT INTO callstack VALUES (2)"},
		{name: "uppercase itid", threadDDL: "CREATE TABLE THREAD (ITID, TID, IPID, START_TS, IS_MAIN_THREAD, SWITCH_COUNT)", threadRow: "INSERT INTO THREAD VALUES (2, 102, 1, 0, 0, 0)", callstackDDL: "CREATE TABLE CALLSTACK (ITID)", callstackRow: "INSERT INTO CALLSTACK VALUES (2)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := createTraceDBFixture(t, []string{
				"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
				"CREATE TABLE process (ipid, pid)", "INSERT INTO process VALUES (1, 500)",
				test.threadDDL, test.threadRow, test.callstackDDL, test.callstackRow,
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
			active, coverage, err := tdb.loadActiveThreadIDs(context.Background(), index)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(active, map[int64]bool{2: true}) || coverage[0].RowsEmitted != 1 || coverage[0].Skipped != "" {
				t.Fatalf("active callstack profile mismatch: active=%+v coverage=%+v", active, coverage)
			}
		})
	}
}

func TestTraceDBR1aBResolverHasNoSQLRepairOrPreDedupBypass(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	dir := filepath.Dir(current)
	for _, name := range []string{"streamerdb_core.go", "streamerdb_sched_identity.go"} {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, forbidden := range []string{"COALESCE(priority, 120)", "COALESCE(priority,120)", "SELECT DISTINCT"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s reopened strict resolver bypass %q", name, forbidden)
			}
		}
	}
}

func TestTraceDBActiveThreadIDsDoNotDependOnSQLiteDistinctStorageClass(t *testing.T) {
	load := func(t *testing.T, reverse bool) (map[int64]bool, []TraceDBCoverage) {
		t.Helper()
		rows := []string{
			"INSERT INTO sched_slice VALUES (10)",
			"INSERT INTO sched_slice VALUES (10.0)",
		}
		if reverse {
			rows[0], rows[1] = rows[1], rows[0]
		}
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid)", "INSERT INTO process VALUES (1, 500)",
			"CREATE TABLE thread (itid, tid, ipid, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (10, 1010, 1, 0, 0, 1)",
			"CREATE TABLE sched_slice (itid)",
		}
		statements = append(statements, rows...)
		path := createTraceDBFixture(t, statements)
		tdb, err := openTraceDB(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer tdb.close()
		index, _, err := tdb.loadThreadIndex(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		active, coverage, err := tdb.loadActiveThreadIDs(context.Background(), index)
		if err != nil {
			t.Fatal(err)
		}
		return active, coverage
	}
	forward, forwardCoverage := load(t, false)
	reverse, reverseCoverage := load(t, true)
	if !reflect.DeepEqual(forward, map[int64]bool{10: true}) || !reflect.DeepEqual(forward, reverse) {
		t.Fatalf("active set depends on SQLite DISTINCT storage-class winner: forward=%+v reverse=%+v", forward, reverse)
	}
	if len(forwardCoverage) != len(reverseCoverage) || !strings.Contains(forwardCoverage[1].Skipped, "invalid") ||
		forwardCoverage[1].Skipped != reverseCoverage[1].Skipped {
		t.Fatalf("active coverage is not order-independent:\nforward=%+v\nreverse=%+v", forwardCoverage, reverseCoverage)
	}
}

func TestTraceDBWakeupPriorityProvenanceAndFieldLevelUnknown(t *testing.T) {
	tests := []struct {
		name         string
		schedRows    []string
		wantPriority string
		wantSource   string
		wantGap      string
	}{
		{name: "Harmony RT 140", schedRows: []string{"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', 140, 1)"}, wantPriority: "140", wantSource: "inferred_next_sched_slice"},
		{name: "Harmony RT 159", schedRows: []string{"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', 159, 1)"}, wantPriority: "159", wantSource: "inferred_next_sched_slice"},
		{name: "unknown nearest point keeps edge", schedRows: []string{
			"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', NULL, 1)",
			"INSERT INTO sched_slice VALUES (1300, 100, 7, 'R', 77, 1)",
		}, wantSource: "unknown", wantGap: "priority_unknown_edges_preserved=1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := []string{
				"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (900)",
				"CREATE TABLE process (ipid, pid, name)",
				"INSERT INTO process VALUES (1, 100, 'App')", "INSERT INTO process VALUES (2, 200, 'Worker')",
				"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
				"INSERT INTO thread VALUES (1, 100, 1, 'app', 900, 1, 1)",
				"INSERT INTO thread VALUES (2, 200, 2, 'waker', 900, 1, 1)",
				"CREATE TABLE sched_slice (ts, dur, cpu, end_state, priority, itid)",
			}
			statements = append(statements, test.schedRows...)
			statements = append(statements,
				"CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type)",
				"INSERT INTO instant VALUES (1000, 'sched_wakeup', 1, 2, 'itid')",
				"CREATE TABLE raw (id, ts, name, cpu, itid)",
				"INSERT INTO raw VALUES (1, 1000, 'sched_wakeup', 7, 1)",
				"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
				"INSERT INTO thread_state VALUES (2, 900, 200, 2, 'Running')",
			)
			body, coverage, index := exportSchedulerFixture(t, statements)
			item := requireWakeupCoverage(t, coverage)
			if item.RowsEmitted != 1 || !strings.Contains(body, "codrax_prio_source="+test.wantSource) {
				t.Fatalf("wakeup priority provenance/edge mismatch: coverage=%+v\n%s", item, body)
			}
			var wakeupSource string
			var wakeupPriority int
			for _, event := range index.Events {
				if event.Type == "sched_wakeup" {
					wakeupSource = event.WakeePrioritySource()
					wakeupPriority = event.WakeePrio
				}
			}
			if wakeupSource != test.wantSource {
				t.Fatalf("tracequery lost wakeup priority provenance: source=%q priority=%d", wakeupSource, wakeupPriority)
			}
			if test.wantPriority == "" {
				if strings.Contains(body, " prio=") || wakeupPriority != 0 || !strings.Contains(item.Skipped, test.wantGap) {
					t.Fatalf("unknown priority was fabricated or edge suppressed: priority=%d coverage=%+v\n%s", wakeupPriority, item, body)
				}
			} else if !strings.Contains(body, " prio="+test.wantPriority+" ") || fmt.Sprint(wakeupPriority) != test.wantPriority || item.Skipped != "" {
				t.Fatalf("valid inferred priority was not preserved with provenance: priority=%d coverage=%+v\n%s", wakeupPriority, item, body)
			}
		})
	}
}

func TestTraceDBWakeupNewPairsAgainstRawWakeupWithoutRenamingOutput(t *testing.T) {
	body, coverage, index := exportSchedulerFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (900)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'App')", "INSERT INTO process VALUES (2, 200, 'Creator')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 100, 1, 'new-app', 900, 1, 1)",
		"INSERT INTO thread VALUES (2, 200, 2, 'creator', 900, 1, 1)",
		"CREATE TABLE sched_slice (ts, dur, cpu, end_state, priority, itid)",
		"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', 140, 1)",
		"CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type)",
		"INSERT INTO instant VALUES (1000, 'sched_wakeup_new', 1, 2, 'itid')",
		"CREATE TABLE raw (id, ts, name, cpu, itid)",
		"INSERT INTO raw VALUES (1, 1000, 'sched_wakeup', 7, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (2, 900, 200, 2, 'Running')",
	})
	if !strings.Contains(body, "sched_wakeup_new: comm=new-app pid=100 prio=140 target_cpu=007") {
		t.Fatalf("wakeup_new was not paired with the canonical raw wakeup shape: coverage=%+v\n%s", coverage, body)
	}
	item := requireWakeupCoverage(t, coverage)
	if item.RowsEmitted != 1 || item.Skipped != "" {
		t.Fatalf("wakeup_new coverage mismatch: %+v", item)
	}
	found := false
	for _, event := range index.Events {
		if event.Type == "sched_wakeup" && event.Name == "sched_wakeup_new" && event.WakeePID == 100 {
			found = true
		}
	}
	if !found {
		t.Fatalf("tracequery did not retain the wakeup_new event identity: %+v", index.Events)
	}
}

func TestTraceDBWakeupSignedUint32ProjectionPreservesHighInternalIDs(t *testing.T) {
	const targetITID = int64(2147483648)
	const wakerITID = int64(4294967294)
	body, coverage, index := exportSchedulerFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)", "INSERT INTO process VALUES (1, 500, 'HighIDs')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2147483648, 101, 1, 'target-high', 0, 0, 1)",
		"INSERT INTO thread VALUES (4294967294, 202, 1, 'waker-high', 0, 0, 1)",
		"CREATE TABLE sched_slice (ts, dur, cpu, end_state, priority, itid)",
		"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', 159, 2147483648)",
		"CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type)",
		"INSERT INTO instant VALUES (1000, 'sched_wakeup', -2147483648, -2, 'itid')",
		"CREATE TABLE raw (id, ts, name, cpu, itid)",
		"INSERT INTO raw VALUES (-2, 1000, 'sched_wakeup', 7, -2147483648)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (4294967294, 900, 200, 2, 'Running')",
	})
	if !strings.Contains(body, "sched_wakeup: comm=target-high pid=101 prio=159 target_cpu=007") {
		t.Fatalf("signed uint32 projection lost a valid high internal identity:\n%s", body)
	}
	item := requireWakeupCoverage(t, coverage)
	if item.RowsEmitted != 1 || item.Skipped != "" {
		t.Fatalf("high-ID wakeup coverage mismatch: %+v", item)
	}
	found := false
	for _, event := range index.Events {
		if event.Type == "sched_wakeup" && event.WakeePID == 101 && event.PID == 202 {
			found = true
		}
	}
	if !found {
		t.Fatalf("high-ID wakeup did not survive tracequery round-trip: %+v", index.Events)
	}
	for raw, want := range map[int64]int64{-2147483648: targetITID, -2: wakerITID, 0: 0, 2147483647: 2147483647, 4294967294: 4294967294} {
		if got, ok := traceDBStrictSignedUint32Projection(raw); !ok || got != want {
			t.Fatalf("signed uint32 projection raw=%d got=(%d,%t), want=(%d,true)", raw, got, ok, want)
		}
	}
	for _, raw := range []any{int64(-1), int64(-2147483649), int64(4294967295), "-2", float64(-2)} {
		if got, ok := traceDBStrictSignedUint32Projection(raw); ok {
			t.Fatalf("invalid/sentinel signed projection raw=%v became %d", raw, got)
		}
	}
}

func TestTraceDBRawStableIDUsesFullUint32DomainAndCanonicalOrder(t *testing.T) {
	for raw, want := range map[int64]int64{
		math.MinInt32:  2147483648,
		-1:             math.MaxUint32,
		0:              0,
		math.MaxInt32:  math.MaxInt32,
		math.MaxUint32: math.MaxUint32,
	} {
		if got, ok := traceDBStrictRawStableIDProjection(raw); !ok || got != want {
			t.Fatalf("raw stable projection raw=%d got=(%d,%t), want=(%d,true)", raw, got, ok, want)
		}
	}
	for _, raw := range []any{int64(math.MinInt32 - 1), int64(math.MaxUint32 + 1), "-1", float64(-1), []byte("-1")} {
		if got, ok := traceDBStrictRawStableIDProjection(raw); ok {
			t.Fatalf("invalid raw stable projection raw=%v became %d", raw, got)
		}
	}

	path := createTraceDBFixture(t, []string{
		"CREATE TABLE raw (id, ts, name, cpu, itid)",
		"INSERT INTO raw VALUES (-2147483648, 1000, 'sched_wakeup', 2, 10)",
		"INSERT INTO raw VALUES (2147483647, 1000, 'sched_wakeup', 1, 10)",
		"INSERT INTO raw VALUES (-1, 1000, 'sched_wakeup', 3, 10)",
		"INSERT INTO raw VALUES (4294967295, 1000, 'sched_wakeup', 4, 10)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	wakeups, coverage, err := tdb.loadRawWakeups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(wakeups) != 2 || wakeups[0].RowID != math.MaxInt32 || wakeups[1].RowID != int64(math.MaxInt32)+1 {
		t.Fatalf("raw stable IDs were not canonically ordered across the int32 boundary: wakeups=%+v coverage=%+v", wakeups, coverage)
	}
	if coverage.RowsRead != 4 || coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "2 raw wakeup row(s) skipped") {
		t.Fatalf("signed/canonical aliases -1 and 4294967295 were not poisoned as one duplicate cohort: %+v", coverage)
	}
	if got := coverage.FieldSources["same_timestamp_order"]; got != "raw.ts then canonical uint32(raw.id)" {
		t.Fatalf("raw canonical ordering provenance=%q", got)
	}
}
