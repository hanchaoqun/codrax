package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSchedulerIntegrityTrace(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(strings.Join(append(lines, ""), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func schedulerIntegrityFixture(malformed string) []string {
	return []string{
		`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		malformed,
		`idle-0 (0) [000] .... 1.200000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`app-20 (20) [000] .... 1.300000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	}
}

func TestParseLineCriticalSchedulerPIDZeroMustBeExplicitNotInferred(t *testing.T) {
	intern := newStringInterner()
	valid := `idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle/0 next_pid=0 next_prio=120`
	if ev, ok := ParseLine(1, valid, intern); !ok || ev.PrevPID != 0 || ev.NextPID != 0 {
		t.Fatalf("explicit idle PID 0 must remain valid: ok=%v event=%+v", ok, ev)
	}

	for _, tc := range []struct {
		name string
		line string
	}{
		{
			name: "missing_next_pid",
			line: `app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_prio=120`,
		},
		{
			name: "missing_prev_pid",
			line: `app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		},
		{
			name: "invalid_wakeup_pid",
			line: `waker-10 (10) [001] .... 1.100000: sched_wakeup_new: comm=app pid=not-a-pid prio=20 target_cpu=000`,
		},
		{
			name: "missing_migrate_destination",
			line: `app-20 (20) [000] .... 1.100000: sched_migrate_task: comm=app pid=20 prio=20 orig_cpu=0`,
		},
		{
			name: "negative_migrate_destination",
			line: `app-20 (20) [000] .... 1.100000: sched_migrate_task: comm=app pid=20 prio=20 orig_cpu=0 dest_cpu=-1`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ev, ok := ParseLine(2, tc.line, intern); ok {
				t.Fatalf("critical incomplete row must be rejected rather than zero-filled: %+v", ev)
			}
		})
	}
}

func TestMalformedSchedSwitchFailsClosedIndexedWindowAndTimeline(t *testing.T) {
	for _, tc := range []struct {
		name    string
		row     string
		missing string
	}{
		{
			name:    "next_pid",
			row:     `app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_prio=120`,
			missing: "next_pid",
		},
		{
			name:    "prev_pid",
			row:     `app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
			missing: "prev_pid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSchedulerIntegrityTrace(t, tc.name+".systrace", schedulerIntegrityFixture(tc.row)...)
			idx, err := BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if len(idx.schedulerRowIntegrityFailures) != 1 {
				t.Fatalf("expected one typed malformed-row witness, got %+v", idx.schedulerRowIntegrityFailures)
			}
			if got := idx.schedulerRowIntegrityFailures[0].reason(); !strings.Contains(got, tc.missing) || !strings.Contains(got, "line=2") {
				t.Fatalf("typed witness must identify exact field/line: %q", got)
			}

			q := Query{TimeStart: 1.0, TimeEnd: 1.3}
			stats := ComputeWindowStats(idx, q)
			if len(stats.TopRunning) != 0 || len(stats.RunnableTop) != 0 || len(stats.SleepTop) != 0 || len(stats.StateChurn) != 0 {
				t.Fatalf("malformed switch must poison deterministic scheduler duration faces: %+v", stats)
			}
			if !containsSubstring(stats.Caveats, "scheduler_row_parse_incomplete") || !containsSubstring(stats.Caveats, tc.missing) {
				t.Fatalf("model-facing caveat must explain the compatibility failure: %+v", stats.Caveats)
			}

			timeline := ThreadTimeline(idx, Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.3})
			if timeline.IntegrityFailure != "scheduler_row_parse_incomplete" || len(timeline.Intervals) != 0 {
				t.Fatalf("target timeline must fail closed on an unknown scheduler role: %+v", timeline)
			}
		})
	}
}

func TestMalformedSchedulerRowIndexedAndStreamStateParity(t *testing.T) {
	malformed := `app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_prio=120`
	path := writeSchedulerIntegrityTrace(t, "stream_parity.systrace", schedulerIntegrityFixture(malformed)...)
	q := Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.3}

	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1.0, TimeStartSet: true,
		TimeEnd: 1.3, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	indexed := ComputeWindowStats(idx, q)
	if !containsSubstring(indexed.Caveats, "scheduler_row_parse_incomplete") || len(indexed.TopRunning) != 0 {
		t.Fatalf("indexed window did not fail closed: %+v", indexed)
	}

	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil {
		t.Fatal("stream state cluster returned no stats face")
	}
	if len(streamed.WindowStats.TopRunning) != 0 || len(streamed.WindowStats.RunnableTop) != 0 ||
		len(streamed.WindowStats.SleepTop) != 0 || len(streamed.WindowStats.StateChurn) != 0 {
		t.Fatalf("streaming lane published durations rejected by indexed lane: %+v", streamed.WindowStats)
	}
	if !containsSubstring(streamed.Caveats, "scheduler_row_parse_incomplete") ||
		!containsSubstring(streamed.Caveats, "stream_state_cluster_fail_closed=true") {
		t.Fatalf("streaming lane must disclose the same typed reason: %+v", streamed.Caveats)
	}
	if coverage := streamed.WindowStats.SchedulerHeadCoverage; coverage == nil ||
		coverage.Status != "unknown" ||
		coverage.SubjectCensusStatus != "not_evaluated" {
		t.Fatalf("streaming fail-close must not render an unevaluated zero census: %+v", coverage)
	}
}

func TestWarmDerivedWindowPreservesMalformedSchedulerPoison(t *testing.T) {
	malformed := `app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_prio=120`
	path := writeSchedulerIntegrityTrace(t, "warm_derive.systrace", schedulerIntegrityFixture(malformed)...)
	resetAnchorCaches()
	if _, err := BuildIndex(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	warm, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1.0, TimeStartSet: true,
		TimeEnd: 1.3, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warm.schedulerRowIntegrityFailures) != 1 {
		t.Fatalf("warm full-cache derive lost malformed scheduler poison: %+v", warm.schedulerRowIntegrityFailures)
	}
	stats := ComputeWindowStats(warm, Query{TimeStart: 1.0, TimeEnd: 1.3})
	if len(stats.TopRunning) != 0 || !containsSubstring(stats.Caveats, "scheduler_row_parse_incomplete") {
		t.Fatalf("warm derived window reopened fabricated scheduler durations: %+v", stats)
	}
}

func TestMalformedWakeupAndMigrationAreNotZeroFilled(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "other_critical_rows.systrace",
		`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`waker-10 (10) [001] .... 1.050000: sched_wakeup: comm=app prio=20 target_cpu=000`,
		`app-20 (20) [000] .... 1.060000: sched_migrate_task: comm=app pid=20 prio=20 orig_cpu=0`,
		`app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.schedulerRowIntegrityFailures) != 2 {
		t.Fatalf("both critical incomplete rows need typed witnesses: %+v", idx.schedulerRowIntegrityFailures)
	}
	for _, ev := range idx.Events {
		if (ev.Type == EventSchedWakeup && ev.WakeePID == 0) || (ev.Type == EventCPUConstraint && ev.Name == "sched_migrate_task") {
			t.Fatalf("incomplete scheduler row leaked into Events with fabricated zero/defaults: %+v", ev)
		}
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	if !containsSubstring(stats.Caveats, "scheduler_row_parse_incomplete") || len(stats.TopRunning) != 0 {
		t.Fatalf("critical row failures must poison scheduler duration output: %+v", stats)
	}
}
