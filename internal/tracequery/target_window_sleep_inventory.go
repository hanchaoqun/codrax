package tracequery

import (
	"math"
	"sort"
)

// TargetWindowSleepInventory enumerates every positive S/D/IO interval in
// the already constructed target timeline, independently of causal-chain
// branch/depth/minimum-duration limits. ScanStatus describes this input slice,
// NOT complete artifact coverage, a closed sleep, or a Binder wait census.
// TotalMs is the union of window-clamped intervals before the output cap.
// SleepMs/DStateMs/IOWaitMs are the corresponding per-state unions; an IO marker
// on an S interval remains an overlay and never moves it into the IO partition.
type TargetWindowSleepInventory struct {
	Thread                  ThreadRef                     `json:"thread"`
	Window                  TimeWindow                    `json:"window"`
	Scope                   string                        `json:"scope"`
	ScanStatus              string                        `json:"scan_status"`
	OutputStatus            string                        `json:"output_status"`
	Total                   int                           `json:"total"`
	Emitted                 int                           `json:"emitted"`
	TotalMs                 float64                       `json:"total_ms"`
	SleepMs                 float64                       `json:"sleep_ms"`
	DStateMs                float64                       `json:"d_state_ms"`
	IOWaitMs                float64                       `json:"io_wait_ms"`
	StateStatistics         []TargetWindowSleepStateStats `json:"state_statistics,omitempty"`
	HeadState               *TimelineHeadState            `json:"head_state,omitempty"`
	StateClosureStatus      string                        `json:"state_closure_status"`
	BinderAssociationStatus string                        `json:"binder_association_status"`
	CausalAttributionStatus string                        `json:"causal_attribution_status"`
	Occurrences             []TargetWindowSleepOccurrence `json:"occurrences"`
}

// TargetWindowSleepOccurrence preserves the original interval's clamped and
// actual ledgers, source coordinates, and interval-local IO marker. ActualEndTs
// can be a query/EOF flush and EndLine can be a blocked-reason locator: neither
// certifies a physical ending wakeup. Closure and causal attribution remain
// explicitly unassessed on the enclosing inventory.
type TargetWindowSleepOccurrence struct {
	Ordinal int `json:"ordinal"`
	Interval
}

const targetWindowSleepInventoryCap = 32

// Per-interval statistics are calculated before either publication or chain
// limits. Durations are window-clipped scheduler intervals, not completed wait
// latencies. IntervalSumMs may exceed the union if input intervals overlap.
type TargetWindowSleepStateStats struct {
	State         ThreadState `json:"state"`
	IntervalCount int         `json:"interval_count"`
	IntervalSumMs float64     `json:"interval_sum_ms"`
	MeanMs        float64     `json:"mean_ms"`
	MaxMs         float64     `json:"max_ms"`
}

func buildTargetWindowSleepInventory(tl TimelineResult, window TimeWindow) *TargetWindowSleepInventory {
	if tl.IntegrityFailure != "" || len(tl.Intervals) == 0 ||
		!finiteSleepInventoryTime(window.StartTs) || !finiteSleepInventoryTime(window.EndTs) || window.EndTs <= window.StartTs {
		return nil
	}
	out := &TargetWindowSleepInventory{
		Thread: tl.Thread, Window: window,
		Scope: "constructed_target_timeline", ScanStatus: "complete", OutputStatus: "complete",
		StateClosureStatus: "not_assessed", BinderAssociationStatus: "not_assessed", CausalAttributionStatus: "not_assessed",
		Occurrences: make([]TargetWindowSleepOccurrence, 0),
	}
	if tl.HeadState != nil {
		head := *tl.HeadState
		out.HeadState = &head
	}
	var all, sleep, dState, ioWait []foldInterval
	var selected []int
	stats := map[ThreadState]*TargetWindowSleepStateStats{}
	measurable := false
	for i, it := range tl.Intervals {
		// The producer owns scheduler integrity. Defensive invalid-input
		// handling must not turn an unusable timeline into a measured zero.
		if !finiteSleepInventoryTime(it.StartTs) || !finiteSleepInventoryTime(it.EndTs) || it.EndTs < it.StartTs {
			return nil
		}
		if it.EndTs == it.StartTs {
			continue
		}
		span := foldInterval{start: it.StartTs, end: it.EndTs}
		switch it.State {
		case StateRunning, StateRunnable:
			measurable = true
			continue
		case StateSSleep:
			sleep = append(sleep, span)
		case StateDSleep:
			dState = append(dState, span)
		case StateIOWait:
			ioWait = append(ioWait, span)
		default:
			continue
		}
		measurable = true
		all = append(all, span)
		selected = append(selected, i)
		if stats[it.State] == nil {
			stats[it.State] = &TargetWindowSleepStateStats{State: it.State}
		}
		s := stats[it.State]
		ms := (it.EndTs - it.StartTs) * 1000
		s.IntervalCount++
		s.IntervalSumMs += ms
		s.MaxMs = math.Max(s.MaxMs, ms)
	}
	if !measurable {
		return nil
	}
	out.Total = len(selected)
	out.TotalMs, _ = foldIntervalUnionMs(all)
	out.SleepMs, _ = foldIntervalUnionMs(sleep)
	out.DStateMs, _ = foldIntervalUnionMs(dState)
	out.IOWaitMs, _ = foldIntervalUnionMs(ioWait)
	for _, state := range []ThreadState{StateSSleep, StateDSleep, StateIOWait} {
		if s := stats[state]; s != nil {
			s.MeanMs = s.IntervalSumMs / float64(s.IntervalCount)
			out.StateStatistics = append(out.StateStatistics, *s)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		a, b := tl.Intervals[selected[i]], tl.Intervals[selected[j]]
		if a.StartTs != b.StartTs {
			return a.StartTs < b.StartTs
		}
		if a.EndTs != b.EndTs {
			return a.EndTs < b.EndTs
		}
		return a.StartLine < b.StartLine
	})
	if len(selected) > targetWindowSleepInventoryCap {
		out.OutputStatus = "incomplete"
		selected = selected[:targetWindowSleepInventoryCap]
	}
	for i, selectedIndex := range selected {
		out.Occurrences = append(out.Occurrences, TargetWindowSleepOccurrence{Ordinal: i + 1, Interval: tl.Intervals[selectedIndex]})
	}
	out.Emitted = len(out.Occurrences)
	return out
}

func finiteSleepInventoryTime(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
