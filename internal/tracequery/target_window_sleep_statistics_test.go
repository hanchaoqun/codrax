package tracequery

import (
	"context"
	"math"
	"path/filepath"
	"testing"
)

func TestSleepStatisticsFullPopulationAndWindowRuler(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_summary/events.systrace")
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			result := Run(idx, Query{View: view, PID: 41, TimeStart: 10.001, TimeEnd: 10.012, MinDurationMs: 100, MaxBranches: 1, MaxDepth: 1, Limit: 1})
			inventory := b1607InventoryResultAccount(t, result).SleepInventory
			if len(inventory.StateStatistics) != 2 || inventory.Total != 3 || math.Abs(inventory.TotalMs-7) > 1e-6 {
				t.Fatalf("unexpected inventory: %+v", inventory)
			}
			want := []TargetWindowSleepStateStats{
				{State: StateSSleep, IntervalCount: 2, IntervalSumMs: 4, MeanMs: 2, MaxMs: 3},
				{State: StateIOWait, IntervalCount: 1, IntervalSumMs: 3, MeanMs: 3, MaxMs: 3},
			}
			for i, s := range inventory.StateStatistics {
				w := want[i]
				if s.State != w.State || s.IntervalCount != w.IntervalCount || math.Abs(s.IntervalSumMs-w.IntervalSumMs) > 1e-6 || math.Abs(s.MeanMs-w.MeanMs) > 1e-6 || math.Abs(s.MaxMs-w.MaxMs) > 1e-6 {
					t.Errorf("got %+v want %+v", s, w)
				}
			}
			b1607AssertUnassessed(t, inventory)
		})
	}
}

func TestSleepStatisticsBeforeCapsAndNotUnionMean(t *testing.T) {
	idx := buildTraceIndex(t, "full-population.ftrace", b1607ManySleepsTrace(39))
	i := b1607InventoryResultAccount(t, Run(idx, Query{View: "wakeup_chain", PID: 41, TimeStart: 10, TimeEnd: 10.041, Limit: 1, MinDurationMs: 100})).SleepInventory
	if i.Emitted != 32 || len(i.StateStatistics) != 3 {
		t.Fatalf("cap fixture: %+v", i)
	}
	for _, s := range i.StateStatistics {
		if s.IntervalCount != 13 || math.Abs(s.MeanMs-.2) > 1e-6 || math.Abs(s.MaxMs-.2) > 1e-6 {
			t.Fatalf("stats derived from display: %+v", s)
		}
	}
	tl := cov4Timeline([]Interval{cov4Interval(StateSSleep, 100.01, 100.04), cov4Interval(StateSSleep, 100.02, 100.05)})
	i = buildTargetWindowSleepInventory(tl, TimeWindow{StartTs: 100, EndTs: 100.1})
	s := i.StateStatistics[0]
	if math.Abs(i.TotalMs-40) > 1e-6 || math.Abs(s.IntervalSumMs-60) > 1e-6 || math.Abs(s.MeanMs-30) > 1e-6 {
		t.Fatalf("union and interval mean conflated: %+v %+v", i, s)
	}
}
