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

func TestTraceDBWakeupsBerlinMigrationKeepsEmitterAndTargetCPUsDistinct(t *testing.T) {
	body, coverage, index := exportSchedulerFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (4999000)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'App')",
		"INSERT INTO process VALUES (2, 200, 'Worker')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'app-100', 4999000, 1, 1)",
		"INSERT INTO thread VALUES (2, 200, 2, 'worker-200', 4999000, 1, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (5000200, 1000, 9, 'R', 52, 1)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (5000000, 'sched_waking', 1, 2, 'itid', NULL)",
		"INSERT INTO instant VALUES (5000100, 'sched_wakeup', 1, 2, 'itid', NULL)",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (10, 5000000, 'sched_waking', 5, 2)",
		"INSERT INTO raw VALUES (11, 5000100, 'sched_wakeup', 9, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 4999000, 3000, 2, 'Running')",
	})

	for _, want := range []string{
		"[002] ....     0.005000: sched_waking: comm=app-100 pid=100 prio=52 target_cpu=005",
		"[002] ....     0.005000: sched_wakeup: comm=app-100 pid=100 prio=52 target_cpu=009",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Berlin migration output missing %q:\n%s", want, body)
		}
	}
	var waking, wakeup *tracequery.Event
	for i := range index.Events {
		event := &index.Events[i]
		switch event.Type {
		case tracequery.EventSchedWaking:
			waking = event
		case tracequery.EventSchedWakeup:
			wakeup = event
		}
	}
	if waking == nil || wakeup == nil || waking.CPU != 2 || wakeup.CPU != 2 ||
		waking.TargetCPU != 5 || wakeup.TargetCPU != 9 {
		t.Fatalf("Berlin migration CPU semantics regressed: waking=%+v wakeup=%+v", waking, wakeup)
	}
	wakeupCoverage := requireWakeupCoverage(t, coverage)
	if wakeupCoverage.RowsRead != 2 || wakeupCoverage.RowsEmitted != 2 || wakeupCoverage.Skipped != "" {
		t.Fatalf("Berlin migration coverage mismatch: %+v", wakeupCoverage)
	}
	if wakeupCoverage.FieldSources["header_cpu"] != "thread_state.Running.cpu" ||
		wakeupCoverage.FieldSources["target_cpu"] != "raw.cpu" ||
		wakeupCoverage.FieldSources["priority"] != "inferred_next_sched_slice" ||
		wakeupCoverage.FieldSources["raw_identity.sched_waking"] != "raw.itid==instant.wakeup_from" ||
		!strings.Contains(wakeupCoverage.FieldSources["raw_identity.sched_wakeup"], "unique_bipartite_matching") {
		t.Fatalf("wakeup field provenance missing: %+v", wakeupCoverage)
	}
}

func TestTraceDBWakeupsSameTimestampSameNameUseTypedIdentity(t *testing.T) {
	body, coverage, _ := exportSchedulerFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (900)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'App')",
		"INSERT INTO process VALUES (2, 200, 'Workers')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'app-a', 900, 0, 1)",
		"INSERT INTO thread VALUES (2, 201, 2, 'waker-a', 900, 0, 1)",
		"INSERT INTO thread VALUES (3, 102, 1, 'app-b', 900, 0, 1)",
		"INSERT INTO thread VALUES (4, 202, 2, 'waker-b', 900, 0, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (2000, 100, 7, 'R', 40, 1)",
		"INSERT INTO sched_slice VALUES (2000, 100, 8, 'R', 41, 3)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (1000, 'sched_wakeup', 1, 2, 'itid', NULL)",
		"INSERT INTO instant VALUES (1000, 'sched_wakeup', 3, 4, 'itid', NULL)",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (20, 1000, 'sched_wakeup', 7, 1)",
		"INSERT INTO raw VALUES (21, 1000, 'sched_wakeup', 8, 3)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 900, 200, 2, 'Running')",
		"INSERT INTO thread_state VALUES (4, 900, 200, 3, 'Running')",
	})
	for _, want := range []string{
		"[002] ....     0.000001: sched_wakeup: comm=app-a pid=101 prio=40 target_cpu=007",
		"[003] ....     0.000001: sched_wakeup: comm=app-b pid=102 prio=41 target_cpu=008",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("typed same-timestamp pairing missing %q:\n%s", want, body)
		}
	}
	wakeupCoverage := requireWakeupCoverage(t, coverage)
	if wakeupCoverage.RowsRead != 2 || wakeupCoverage.RowsEmitted != 2 || wakeupCoverage.Skipped != "" {
		t.Fatalf("same-timestamp typed pairing coverage mismatch: %+v", wakeupCoverage)
	}
}

func TestTraceDBWakeupsFailClosedWhenEmitterCPUIsNotProvable(t *testing.T) {
	body, coverage, _ := exportSchedulerFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (900)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'App')",
		"INSERT INTO process VALUES (2, 200, 'Worker')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'app', 900, 1, 1)",
		"INSERT INTO thread VALUES (2, 200, 2, 'waker', 900, 1, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', 42, 1)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (1000, 'sched_wakeup', 1, 2, 'itid', NULL)",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (1, 1000, 'sched_wakeup', 7, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 900, 300, 2, 'Running')",
		"INSERT INTO thread_state VALUES (2, 950, 200, 3, 'Running')",
	})
	if strings.Contains(body, "sched_wakeup:") {
		t.Fatalf("ambiguous emitter CPU must not mint a wakeup row:\n%s", body)
	}
	wakeupCoverage := requireWakeupCoverage(t, coverage)
	if wakeupCoverage.RowsRead != 1 || wakeupCoverage.RowsEmitted != 0 ||
		!strings.Contains(wakeupCoverage.Skipped, "missing_or_ambiguous_emitter_running_cpu=1") {
		t.Fatalf("unknown emitter CPU coverage mismatch: %+v", wakeupCoverage)
	}
}

func TestTraceDBWakeupsEnforceTraceCPUIdentityDomain(t *testing.T) {
	tests := []struct {
		name       string
		targetCPU  int
		headerCPU  int
		wantEmit   bool
		wantSkip   string
		wantRawGap bool
	}{
		{name: "upper boundary", targetCPU: 4095, headerCPU: 4095, wantEmit: true},
		{name: "target above boundary", targetCPU: 4096, headerCPU: 2, wantSkip: "raw_instant_count_mismatch=1", wantRawGap: true},
		{name: "header above boundary", targetCPU: 7, headerCPU: 4096, wantSkip: "tainted_emitter_running_cpu=1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, coverage, _ := exportSchedulerFixture(t, singleWakeupCPUFixtureStatements(test.targetCPU, test.headerCPU))
			item := requireWakeupCoverage(t, coverage)
			if test.wantEmit {
				if item.RowsEmitted != 1 || !strings.Contains(body, "[4095] ....") || !strings.Contains(body, "target_cpu=4095") {
					t.Fatalf("valid CPU upper boundary was not preserved: coverage=%+v\n%s", item, body)
				}
				return
			}
			if strings.Contains(body, "sched_wakeup:") || item.RowsEmitted != 0 || !strings.Contains(item.Skipped, test.wantSkip) {
				t.Fatalf("out-of-domain CPU was not rejected before systrace minting: coverage=%+v\n%s", item, body)
			}
			if test.wantRawGap {
				rawGap := false
				for _, candidate := range coverage {
					if candidate.Family == "resolver" && candidate.Table == "raw" &&
						candidate.RowsRead == 1 && candidate.RowsEmitted == 0 && strings.Contains(candidate.Skipped, "invalid") {
						rawGap = true
					}
				}
				if !rawGap {
					t.Fatalf("malformed target CPU was not disclosed in raw coverage: %+v", coverage)
				}
			}
		})
	}
}

func TestTraceDBWakeupsFailClosedOnAmbiguousOrWrongRawIdentity(t *testing.T) {
	t.Run("ambiguous wakeup", func(t *testing.T) {
		body, coverage, _ := exportSchedulerFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (900)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 100, 'Pair')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (1, 101, 1, 'a', 900, 0, 1)",
			"INSERT INTO thread VALUES (2, 102, 1, 'b', 900, 0, 1)",
			"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
			"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', 40, 1)",
			"INSERT INTO sched_slice VALUES (1200, 100, 8, 'R', 41, 2)",
			"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
			"INSERT INTO instant VALUES (1000, 'sched_wakeup', 1, 2, 'itid', NULL)",
			"INSERT INTO instant VALUES (1000, 'sched_wakeup', 2, 1, 'itid', NULL)",
			"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
			"INSERT INTO raw VALUES (1, 1000, 'sched_wakeup', 7, 1)",
			"INSERT INTO raw VALUES (2, 1000, 'sched_wakeup', 8, 2)",
			"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
			"INSERT INTO thread_state VALUES (1, 900, 200, 1, 'Running')",
			"INSERT INTO thread_state VALUES (2, 900, 200, 2, 'Running')",
		})
		if strings.Contains(body, "sched_wakeup:") {
			t.Fatalf("ambiguous raw identity must not mint rows:\n%s", body)
		}
		item := requireWakeupCoverage(t, coverage)
		if item.RowsRead != 2 || item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "ambiguous_raw_identity_mapping=2") {
			t.Fatalf("ambiguous identity coverage mismatch: %+v", item)
		}
	})

	t.Run("waking raw row names wakee instead of waker", func(t *testing.T) {
		body, coverage, _ := exportSchedulerFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (900)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 100, 'App')",
			"INSERT INTO process VALUES (2, 200, 'Worker')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (1, 100, 1, 'app', 900, 1, 1)",
			"INSERT INTO thread VALUES (2, 200, 2, 'waker', 900, 1, 1)",
			"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
			"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', 42, 1)",
			"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
			"INSERT INTO instant VALUES (1000, 'sched_waking', 1, 2, 'itid', NULL)",
			"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
			"INSERT INTO raw VALUES (1, 1000, 'sched_waking', 7, 1)",
			"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
			"INSERT INTO thread_state VALUES (2, 900, 200, 2, 'Running')",
		})
		if strings.Contains(body, "sched_waking:") {
			t.Fatalf("wrong waking raw identity must not mint a row:\n%s", body)
		}
		item := requireWakeupCoverage(t, coverage)
		if item.RowsRead != 1 || item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "raw_identity_mismatch=1") {
			t.Fatalf("waking identity rejection coverage mismatch: %+v", item)
		}
	})
}

func exportSchedulerFixture(t *testing.T, statements []string) (string, []TraceDBCoverage, *tracequery.Index) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open scheduler fixture: %v", err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBSchedulerFamilies(context.Background(), tdb, sink)
	if err != nil {
		t.Fatalf("export scheduler fixture: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "scheduler.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.writeTo(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write scheduler fixture: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery scheduler fixture: %v", err)
	}
	return string(bodyBytes), coverage, index
}

func requireWakeupCoverage(t *testing.T, coverage []TraceDBCoverage) TraceDBCoverage {
	t.Helper()
	for _, item := range coverage {
		if item.Family == "scheduler" && item.Table == "instant" {
			return item
		}
	}
	t.Fatalf("scheduler/instant coverage missing: %+v", coverage)
	return TraceDBCoverage{}
}

func singleWakeupCPUFixtureStatements(targetCPU, headerCPU int) []string {
	return []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (900)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'App')",
		"INSERT INTO process VALUES (2, 200, 'Worker')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'app', 900, 1, 1)",
		"INSERT INTO thread VALUES (2, 200, 2, 'waker', 900, 1, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (1200, 100, 7, 'R', 42, 1)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (1000, 'sched_wakeup', 1, 2, 'itid', NULL)",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
		fmt.Sprintf("INSERT INTO raw VALUES (1, 1000, 'sched_wakeup', %d, 1)", targetCPU),
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		fmt.Sprintf("INSERT INTO thread_state VALUES (2, 900, 200, %d, 'Running')", headerCPU),
	}
}
