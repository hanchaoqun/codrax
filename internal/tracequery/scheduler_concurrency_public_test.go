package tracequery

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// Exercise physical text -> BuildIndex -> Run, not a hand-made interval ledger.
func concurrencyPublicTrace(rows ...string) string {
	sort.SliceStable(rows, func(i, j int) bool {
		a, _ := ParseLine(i+1, rows[i], nil)
		b, _ := ParseLine(j+1, rows[j], nil)
		return a.Ts < b.Ts
	})
	return strings.Join(rows, "\n") + "\n"
}

func concurrencyPublicWake(ts float64, pid int) string {
	return fmt.Sprintf("waker-9 (9) [000] .... %.9f: sched_wakeup: comm=worker pid=%d prio=120 target_cpu=000", ts, pid)
}

func concurrencyPublicSwitch(ts float64, cpu, prev, next int, state string) string {
	return fmt.Sprintf("worker-%d (%d) [%03d] .... %.9f: sched_switch: prev_comm=worker prev_pid=%d prev_prio=120 prev_state=%s ==> next_comm=worker next_pid=%d next_prio=120", prev, prev, cpu, ts, prev, state, next)
}

func TestSchedulerConcurrencyPublicCompleteIntervals(t *testing.T) {
	idx := buildTraceIndex(t, "concurrency.systrace", concurrencyPublicTrace(
		concurrencyPublicWake(1.001, 10),
		concurrencyPublicWake(1.003, 10), // duplicate wake cannot start another wait
		concurrencyPublicWake(1.002, 11),
		concurrencyPublicWake(1.008, 12),
		concurrencyPublicSwitch(1.006, 0, 0, 10, "S"),
		concurrencyPublicSwitch(1.004, 1, 0, 11, "S"),
		concurrencyPublicSwitch(1.009, 2, 0, 12, "S"),
		concurrencyPublicSwitch(1.012, 0, 10, 0, "S"),
		concurrencyPublicSwitch(1.012, 1, 11, 0, "S"),
		concurrencyPublicSwitch(1.012, 2, 12, 0, "S"),
	))
	res := Run(idx, Query{View: "window_stats", PID: 10, TimeStart: 1, TimeEnd: 1.010, TimeStartSet: true, TimeEndSet: true})
	data, err := json.Marshal(res.WindowStats)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		SchedulerConcurrency *struct {
			Population  string `json:"population"`
			ThreadScope string `json:"thread_scope"`
			Groups      []struct {
				State  string `json:"state"`
				Values *struct {
					PeakThreads int     `json:"peak_threads"`
					MeanThreads float64 `json:"mean_threads"`
					BusyMs      float64 `json:"busy_ms"`
					ThreadMs    float64 `json:"thread_ms"`
				} `json:"values"`
			} `json:"groups"`
		} `json:"scheduler_concurrency"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.SchedulerConcurrency == nil {
		t.Fatal("confirmed scheduler intervals have no scheduler_concurrency public measurement")
	}
	if wire.SchedulerConcurrency.Population != "accepted_closed_intervals" || wire.SchedulerConcurrency.ThreadScope != "all_positive_tids" {
		t.Fatalf("wrong population/scope: %+v", wire.SchedulerConcurrency)
	}
	found := false
	for _, group := range wire.SchedulerConcurrency.Groups {
		if group.State != "runnable" {
			continue
		}
		found = true
		if group.Values == nil || group.Values.PeakThreads != 2 {
			t.Fatalf("runnable peak: %+v", group)
		}
		for key, pair := range map[string][2]float64{"mean": {group.Values.MeanThreads, .8}, "busy": {group.Values.BusyMs, 6}, "area": {group.Values.ThreadMs, 8}} {
			if math.Abs(pair[0]-pair[1]) > 1e-6 {
				t.Errorf("%s=%g want=%g", key, pair[0], pair[1])
			}
		}
	}
	if !found {
		t.Fatal("runnable group absent")
	}
}
