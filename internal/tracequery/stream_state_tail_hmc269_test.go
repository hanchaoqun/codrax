package tracequery

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHMC269StreamStatePhysicalTailNotSelectedTail(t *testing.T) {
	start := "target-41 (40) [001] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n"
	sleep := "target-41 (40) [001] .... 1.040000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	later := "noise-9 (9) [002] .... 1.200000: cpu_frequency: state=1000000 cpu_id=2\n"
	for _, tc := range []struct {
		name, content string
		wantMS        float64
		uncovered     bool
	}{
		{"later_physical_event", start + sleep + later, 50, false},
		{"later_event_precedes_target_in_file", later + start + sleep, 50, false},
		{"real_eof", start + sleep, 40, true},
		{"text_timestamp_is_not_physical_boundary", start + sleep + "# mentioned 2.000000: not an event\n", 40, true},
		{"partial_timestamp_header_is_not_physical_boundary", start + sleep + "noise-9 (9) [002] .... 2.000000", 40, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tail.ftrace")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				got, err := StreamStateCluster(context.Background(), path, Query{PID: 41, TimeStart: 1, TimeEnd: 1.05, TimeStartSet: true, TimeEndSet: true}, 20)
				if err != nil {
					t.Fatal(err)
				}
				if got.WindowStats == nil || got.TimeEnd != 1.05 {
					t.Fatalf("requested scope lost: %+v", got)
				}
				var total float64
				for _, rows := range [][]ThreadDuration{got.WindowStats.TopRunning, got.WindowStats.RunnableTop, got.WindowStats.SleepTop, got.WindowStats.DStateTop, got.WindowStats.IOWaitTop} {
					for _, row := range rows {
						if row.Thread.PID == 41 {
							total += row.DurationMs
						}
					}
				}
				if math.Abs(total-tc.wantMS) > 1e-6 {
					t.Fatalf("attempt=%d physical tail, not selected event tail, bounds state coverage: got %.6fms want %.6fms; %+v", attempt, total, tc.wantMS, got.WindowStats)
				}
				if strings.Contains(strings.Join(got.WindowStats.Caveats, "\n"), "trace_artifact_tail_uncovered=true") != tc.uncovered {
					t.Fatalf("attempt=%d wrong EOF coverage: %v", attempt, got.WindowStats.Caveats)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := StreamStateCluster(ctx, path, Query{TimeStart: 1, TimeEnd: 1.05, TimeStartSet: true, TimeEndSet: true}, 20); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation cannot mint tail coverage: %v", err)
			}
		})
	}
}
