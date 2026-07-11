package hitraceconv

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBBlockedReasonsSymbolAndOpaqueRoundTrip(t *testing.T) {
	statements := traceDBBlockedFixtureSchema()
	statements = append(statements,
		"INSERT INTO sched_slice VALUES (1, 900000, 100000, 4, 1, 'R', 40)",
		"INSERT INTO sched_slice VALUES (2, 1900000, 100000, 5, 2, 'R', 41)",
		"INSERT INTO sched_slice VALUES (3, 2900000, 100000, 6, 3, 'R', 42)",
		"INSERT INTO sched_slice VALUES (4, 3900000, 100000, 7, 4, 'R', 43)",
		"INSERT INTO thread_state VALUES (1, 1000000, 500000, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (2, 2000000, 250000, NULL, 2, 563, 500, 'D-NIO', 101)",
		"INSERT INTO thread_state VALUES (3, 3000000, 100000, NULL, 3, 564, 500, 'DK-IO', 102)",
		"INSERT INTO thread_state VALUES (4, 4000000, 100000, NULL, 4, 565, 500, 'D-IO', 103)",
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (3, 'delay')",
		"INSERT INTO data_dict VALUES (4, 'pid')",
		"INSERT INTO data_dict VALUES (5, 'caller_str')",
		"INSERT INTO data_dict VALUES (10, 'schedule_timeout+0x10/0x20[kernel]')",
		"INSERT INTO data_dict VALUES (11, '0x11223344')",
		"INSERT INTO data_dict VALUES (12, '0x55667788')",
		"INSERT INTO data_dict VALUES (13, 'io_schedule+0x8/0x20[kernel]')",
		"INSERT INTO data_dict VALUES (14, '0x9999')",
		"INSERT INTO args VALUES (1, 1, 0, 1, 100)",
		"INSERT INTO args VALUES (2, 2, 1, 10, 100)",
		"INSERT INTO args VALUES (3, 3, 0, 50, 100)",
		"INSERT INTO args VALUES (4, 4, 0, 562, 100)",
		"INSERT INTO args VALUES (5, 1, 0, 0, 101)",
		"INSERT INTO args VALUES (6, 2, 1, 11, 101)",
		"INSERT INTO args VALUES (7, 1, 0, 1, 102)",
		"INSERT INTO args VALUES (8, 2, 1, 12, 102)",
		"INSERT INTO args VALUES (9, 1, 0, 1, 103)",
		"INSERT INTO args VALUES (10, 2, 1, 14, 103)",
		"INSERT INTO args VALUES (11, 5, 1, 13, 103)",
	)
	body, coverage, index := exportSchedulerFixture(t, statements)
	for _, want := range []string{
		"[004] ....     0.001000: sched_blocked_reason: pid=562 iowait=1 caller=schedule_timeout+0x10/0x20[kernel] caller_quality=symbolized delay=50",
		"[005] ....     0.002000: sched_blocked_reason: pid=563 iowait=0 caller=unknown caller_raw=0x11223344 caller_quality=opaque",
		"[006] ....     0.003000: sched_blocked_reason: pid=564 iowait=1 caller=unknown caller_raw=0x55667788 caller_quality=opaque",
		"[007] ....     0.004000: sched_blocked_reason: pid=565 iowait=1 caller=io_schedule+0x8/0x20[kernel] caller_raw=0x9999 caller_quality=symbolized",
		"timestamp_source=thread_state_start_projection original_timestamp_known=false header_thread_source=thread_state_subject_projection original_header_thread_known=false header_cpu_source=exact_prev_sched_slice_boundary source=thread_state_argset",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SQL blocked-reason output missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "caller=0x") {
		t.Fatalf("raw blocked caller became semantic caller:\n%s", body)
	}
	reasonCounts := map[string]int{}
	blockedEvents := 0
	for _, event := range index.Events {
		if event.Type != tracequery.EventSchedBlockedReason {
			continue
		}
		blockedEvents++
		reasonCounts[event.Reason]++
	}
	if blockedEvents != 4 || reasonCounts["schedule_timeout+0x10/0x20[kernel]"] != 1 ||
		reasonCounts["io_schedule+0x8/0x20[kernel]"] != 1 || reasonCounts["unknown"] != 2 || len(reasonCounts) != 3 {
		t.Fatalf("opaque SQL callers fragmented tracequery Reason: events=%d reasons=%v", blockedEvents, reasonCounts)
	}
	item := requireBlockedReasonCoverage(t, coverage)
	if item.RowsRead != 4 || item.RowsEmitted != 4 || item.Skipped != "" {
		t.Fatalf("SQL blocked-reason coverage mismatch: %+v", item)
	}
	for key, want := range map[string]string{
		"timestamp":     "thread_state.ts_projection; original_timestamp_known=false",
		"header_cpu":    "unique_same_itid_sched_slice.cpu_where_slice.ts+slice.dur==thread_state.ts",
		"header_thread": "thread_state_subject_projection; original_header_thread_known=false",
		"source":        "thread_state_argset",
	} {
		if item.FieldSources[key] != want {
			t.Fatalf("blocked-reason source %s=%q, want %q: %+v", key, item.FieldSources[key], want, item)
		}
	}
}

func TestTraceDBBlockedReasonOnSStateRemainsZeroDurationMarker(t *testing.T) {
	statements := traceDBBlockedFixtureSchema()
	statements = append(statements,
		"INSERT INTO sched_slice VALUES (1, 900000, 100000, 4, 1, 'S', 40)",
		"INSERT INTO sched_slice VALUES (2, 1000000, 100000, 4, 2, 'R', 41)",
		"INSERT INTO thread_state VALUES (1, 1000000, 500000, NULL, 1, 562, 500, 'S', 110)",
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (10, 'schedule_timeout+0x10/0x20[kernel]')",
		"INSERT INTO args VALUES (1, 1, 0, 1, 110)",
		"INSERT INTO args VALUES (2, 2, 1, 10, 110)",
	)
	body, coverage, index := exportSchedulerFixture(t, statements)
	if !strings.Contains(body, "sched_blocked_reason: pid=562 iowait=1 caller=schedule_timeout+0x10/0x20[kernel]") {
		t.Fatalf("S-state blocked-reason marker was lost:\n%s", body)
	}

	blockedEvents := 0
	for _, event := range index.Events {
		if event.Type == tracequery.EventSchedBlockedReason && event.WakeePID == 562 {
			blockedEvents++
			if event.IOWait != 1 {
				t.Fatalf("S-state marker lost authoritative args iowait: %+v", event)
			}
		}
	}
	if blockedEvents != 1 {
		t.Fatalf("S-state marker count=%d, want 1", blockedEvents)
	}

	query := tracequery.Query{PID: 562, TimeStart: 0.001, TimeEnd: 0.0015}
	timeline := tracequery.ThreadTimeline(index, query)
	sawSleep := false
	for _, interval := range timeline.Intervals {
		switch interval.State {
		case tracequery.StateSSleep:
			sawSleep = true
		case tracequery.StateDSleep, tracequery.StateIOWait:
			t.Fatalf("S-state marker was promoted into a D/IO-wait interval: %+v", timeline.Intervals)
		}
	}
	if !sawSleep {
		t.Fatalf("S-state scheduler interval missing: %+v", timeline.Intervals)
	}
	stats := tracequery.ComputeWindowStats(index, query)
	for _, lane := range [][]tracequery.ThreadDuration{stats.DStateTop, stats.IOWaitTop} {
		for _, duration := range lane {
			if duration.Thread.PID == 562 && duration.DurationMs > 0 {
				t.Fatalf("S-state marker minted D/IO-wait duration: %+v", stats)
			}
		}
	}
	item := requireBlockedReasonCoverage(t, coverage)
	if item.RowsRead != 1 || item.RowsEmitted != 1 || item.Skipped != "" {
		t.Fatalf("S-state blocked marker coverage mismatch: %+v", item)
	}
}

func TestTraceDBBlockedReasonUnsplitDStateFailsClosed(t *testing.T) {
	statements := traceDBBlockedFixtureSchema()
	statements = append(statements,
		"INSERT INTO sched_slice VALUES (1, 900000, 100000, 4, 1, 'D', 40)",
		"INSERT INTO sched_slice VALUES (2, 1900000, 100000, 5, 2, 'DK', 41)",
		"INSERT INTO thread_state VALUES (1, 1000000, 500000, NULL, 1, 562, 500, 'D', 120)",
		"INSERT INTO thread_state VALUES (2, 2000000, 500000, NULL, 2, 563, 500, 'DK', 121)",
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (10, 'schedule_timeout')",
		"INSERT INTO args VALUES (1, 1, 0, 1, 120)",
		"INSERT INTO args VALUES (2, 2, 1, 10, 120)",
		"INSERT INTO args VALUES (3, 1, 0, 0, 121)",
		"INSERT INTO args VALUES (4, 2, 1, 10, 121)",
	)
	body, coverage, _ := exportSchedulerFixture(t, statements)
	if strings.Contains(body, "sched_blocked_reason:") {
		t.Fatalf("upstream-inconsistent unsplit D/DK rows minted blocked markers:\n%s", body)
	}
	item := requireBlockedReasonCoverage(t, coverage)
	if item.RowsRead != 2 || item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "unsplit_blocked_state=2") {
		t.Fatalf("unsplit D/DK invariant was not pinned fail-closed: %+v", item)
	}
}

func TestTraceDBBlockedReasonsSchemaGapsDoNotBreakOldDBs(t *testing.T) {
	t.Run("missing arg_setid column", func(t *testing.T) {
		body, coverage, _ := exportSchedulerFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'App')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (1, 562, 1, 'blocked-562', 0, 0, 1)",
			"CREATE TABLE sched_slice (id INT, ts INT, dur INT, cpu INT, itid INT, end_state TEXT, priority INT)",
			"CREATE TABLE thread_state (id INT, ts INT, dur INT, cpu INT, itid INT, tid INT, pid INT, state TEXT)",
			"CREATE TABLE args (id INT, key INT, datatype INT, value INT, argset INT)",
			"CREATE TABLE data_dict (id INT, data TEXT)",
		})
		if strings.Contains(body, "sched_blocked_reason:") {
			t.Fatalf("old DB without arg_setid minted blocked reason:\n%s", body)
		}
		item := requireBlockedReasonCoverage(t, coverage)
		if !strings.Contains(item.Skipped, "missing thread_state columns arg_setid") {
			t.Fatalf("missing arg_setid was not disclosed: %+v", item)
		}
	})

	t.Run("missing args table", func(t *testing.T) {
		body, coverage, _ := exportSchedulerFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'App')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (1, 562, 1, 'blocked-562', 0, 0, 1)",
			"CREATE TABLE sched_slice (id INT, ts INT, dur INT, cpu INT, itid INT, end_state TEXT, priority INT)",
			"CREATE TABLE thread_state (id INT, ts INT, dur INT, cpu INT, itid INT, tid INT, pid INT, state TEXT, arg_setid INT)",
			"CREATE TABLE data_dict (id INT, data TEXT)",
		})
		if strings.Contains(body, "sched_blocked_reason:") {
			t.Fatalf("old DB without args table minted blocked reason:\n%s", body)
		}
		item := requireBlockedReasonCoverage(t, coverage)
		if item.Found || !strings.Contains(item.Skipped, "missing table args") {
			t.Fatalf("missing args table was not conservatively disclosed: %+v", item)
		}
	})
}

func TestTraceDBBlockedReasonsMalformedRowsFailClosed(t *testing.T) {
	stateOverflowTS := int64(math.MaxInt64 - 5)
	statements := traceDBBlockedFixtureSchema()
	statements = append(statements,
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (3, 'pid')",
		"INSERT INTO data_dict VALUES (10, 'f2fs_wait_on_block')",
		// Missing caller.
		"INSERT INTO args VALUES (1, 1, 0, 1, 200)",
		"INSERT INTO sched_slice VALUES (1, 900000, 100000, 4, 1, 'R', 40)",
		"INSERT INTO thread_state VALUES (1, 1000000, 100000, NULL, 1, 562, 500, 'D-IO', 200)",
		// Exact previous boundary exists, but its CPU is unavailable.
		"INSERT INTO args VALUES (2, 1, 0, 1, 201)",
		"INSERT INTO args VALUES (3, 2, 1, 10, 201)",
		"INSERT INTO sched_slice VALUES (2, 1900000, 100000, NULL, 2, 'R', 41)",
		"INSERT INTO thread_state VALUES (2, 2000000, 100000, NULL, 2, 563, 500, 'D-IO', 201)",
		// thread_state end overflows int64 even though the projected start is finite.
		"INSERT INTO args VALUES (4, 1, 0, 1, 202)",
		"INSERT INTO args VALUES (5, 2, 1, 10, 202)",
		"INSERT INTO sched_slice VALUES (3, "+strconv.FormatInt(stateOverflowTS-100, 10)+", 100, 4, 3, 'R', 42)",
		"INSERT INTO thread_state VALUES (3, "+strconv.FormatInt(stateOverflowTS, 10)+", 10, NULL, 3, 564, 500, 'D-IO', 202)",
		// Optional payload pid conflicts with the authoritative thread-state tid.
		"INSERT INTO args VALUES (6, 1, 0, 1, 203)",
		"INSERT INTO args VALUES (7, 2, 1, 10, 203)",
		"INSERT INTO args VALUES (8, 3, 0, 999, 203)",
		"INSERT INTO sched_slice VALUES (4, 3900000, 100000, 4, 4, 'R', 43)",
		"INSERT INTO thread_state VALUES (4, 4000000, 100000, NULL, 4, 565, 500, 'D-IO', 203)",
		// thread_state.tid conflicts with the thread table for the same itid.
		"INSERT INTO args VALUES (9, 1, 0, 1, 204)",
		"INSERT INTO args VALUES (10, 2, 1, 10, 204)",
		"INSERT INTO sched_slice VALUES (5, 4900000, 100000, 4, 5, 'R', 44)",
		"INSERT INTO thread_state VALUES (5, 5000000, 100000, NULL, 5, 999, 500, 'D-IO', 204)",
		// No sched_slice ends at the blocked-state start.
		"INSERT INTO args VALUES (11, 1, 0, 1, 205)",
		"INSERT INTO args VALUES (12, 2, 1, 10, 205)",
		"INSERT INTO thread_state VALUES (6, 6000000, 100000, NULL, 6, 567, 500, 'D-IO', 205)",
		// Two sched_slice rows claim the same exact end boundary.
		"INSERT INTO args VALUES (13, 1, 0, 1, 206)",
		"INSERT INTO args VALUES (14, 2, 1, 10, 206)",
		"INSERT INTO sched_slice VALUES (6, 6900000, 100000, 4, 7, 'R', 45)",
		"INSERT INTO sched_slice VALUES (7, 6950000, 50000, 5, 7, 'R', 45)",
		"INSERT INTO thread_state VALUES (7, 7000000, 100000, NULL, 7, 568, 500, 'D-IO', 206)",
	)
	body, coverage, _ := exportSchedulerFixture(t, statements)
	if strings.Contains(body, "sched_blocked_reason:") {
		t.Fatalf("malformed blocked rows minted systrace events:\n%s", body)
	}
	item := requireBlockedReasonCoverage(t, coverage)
	if item.RowsRead != 7 || item.RowsEmitted != 0 {
		t.Fatalf("malformed blocked coverage counts mismatch: %+v", item)
	}
	for _, reason := range []string{
		"missing_caller_arg=1",
		"invalid_prev_sched_slice_cpu=1",
		"thread_state_end_overflow=1",
		"arg_pid_mismatch=1",
		"thread_tid_mismatch=1",
		"missing_prev_sched_slice_boundary=1",
		"ambiguous_prev_sched_slice_boundary=1",
	} {
		if !strings.Contains(item.Skipped, reason) {
			t.Fatalf("malformed blocked coverage missing %q: %+v", reason, item)
		}
	}
}

func TestTraceDBBlockedHeaderCPUBoundariesFailClosed(t *testing.T) {
	tests := []struct {
		name     string
		matches  []traceDBBlockedSchedSlice
		overflow bool
		wantCPU  int64
		wantGap  string
	}{
		{name: "exact", matches: []traceDBBlockedSchedSlice{{TS: 900, Dur: 100, CPU: 4095}}, wantCPU: 4095},
		{name: "missing", wantGap: "missing_prev_sched_slice_boundary"},
		{name: "ambiguous", matches: []traceDBBlockedSchedSlice{{TS: 900, Dur: 100, CPU: 2}, {TS: 950, Dur: 50, CPU: 3}}, wantGap: "ambiguous_prev_sched_slice_boundary"},
		{name: "invalid cpu", matches: []traceDBBlockedSchedSlice{{TS: 900, Dur: 100, CPU: 4096}}, wantGap: "invalid_prev_sched_slice_cpu"},
		{name: "boundary overflow", overflow: true, wantGap: "sched_slice_boundary_overflow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cpu, gap := traceDBBlockedHeaderCPU(test.matches, test.overflow)
			if cpu != test.wantCPU || gap != test.wantGap {
				t.Fatalf("header CPU=(%d,%q), want (%d,%q)", cpu, gap, test.wantCPU, test.wantGap)
			}
		})
	}
}

func TestTraceDBBlockedBoundaryScanRetainsOnlyCandidateMatches(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE sched_slice (itid INT, ts INT, dur INT, cpu INT)",
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4)",
		"INSERT INTO sched_slice VALUES (1, " + strconv.FormatInt(math.MaxInt64-5, 10) + ", 10, 5)",
		"INSERT INTO sched_slice VALUES (999, 800, 200, 6)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, err := loadTraceDBBlockedSchedBoundaries(context.Background(), tdb, map[int64]map[int64]bool{
		1: {1000: true, math.MaxInt64: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	key := traceDBBlockedBoundaryKey{ITID: 1, StateStart: 1000}
	if len(index.Matches) != 1 || len(index.Matches[key]) != 1 || index.Matches[key][0].CPU != 4 {
		t.Fatalf("candidate-aware boundary retention mismatch: %+v", index)
	}
	if !index.OverflowITIDs[1] {
		t.Fatalf("candidate ITID overflow was not retained: %+v", index)
	}
	if _, exists := index.Matches[traceDBBlockedBoundaryKey{ITID: 999, StateStart: 1000}]; exists {
		t.Fatalf("unrelated sched_slice was retained: %+v", index)
	}
}

func TestTraceDBBlockedRawCallerPreservesSignedSQLiteBits(t *testing.T) {
	got, ok := traceDBBlockedRawCaller(traceDBBlockedArg{
		DataType: traceDBArgTypeInt,
		Int:      -1,
		Valid:    true,
	})
	if !ok || got != "0xffffffffffffffff" {
		t.Fatalf("signed SQLite caller bits=(%q,%t), want uint64 bit pattern", got, ok)
	}
}

func traceDBBlockedFixtureSchema() []string {
	return []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'App')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 562, 1, 'blocked-562', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 563, 1, 'blocked-563', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 564, 1, 'blocked-564', 0, 0, 1)",
		"INSERT INTO thread VALUES (4, 565, 1, 'blocked-565', 0, 0, 1)",
		"INSERT INTO thread VALUES (5, 566, 1, 'blocked-566', 0, 0, 1)",
		"INSERT INTO thread VALUES (6, 567, 1, 'blocked-567', 0, 0, 1)",
		"INSERT INTO thread VALUES (7, 568, 1, 'blocked-568', 0, 0, 1)",
		"CREATE TABLE sched_slice (id INT, ts INT, dur INT, cpu INT, itid INT, end_state TEXT, priority INT)",
		"CREATE TABLE thread_state (id INT, ts INT, dur INT, cpu INT, itid INT, tid INT, pid INT, state TEXT, arg_setid INT)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT)",
		"CREATE TABLE callstack (ts INT, itid INT, callid INT)",
		"CREATE TABLE syscall (ts INT, itid INT)",
		"CREATE TABLE native_hook (start_ts INT, itid INT)",
		"CREATE TABLE frame_slice (ts INT, itid INT)",
		"CREATE TABLE args (id INT, key INT, datatype INT, value INT, argset INT)",
		"CREATE TABLE data_dict (id INT, data TEXT)",
	}
}

func requireBlockedReasonCoverage(t *testing.T, coverage []TraceDBCoverage) TraceDBCoverage {
	t.Helper()
	for _, item := range coverage {
		if item.Family == "scheduler" && item.Table == "thread_state.arg_setid" {
			return item
		}
	}
	t.Fatalf("scheduler/thread_state.arg_setid coverage missing: %+v", coverage)
	return TraceDBCoverage{}
}
