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
	}
	if !measurable {
		return nil
	}
	out.Total = len(selected)
	out.TotalMs, _ = foldIntervalUnionMs(all)
	out.SleepMs, _ = foldIntervalUnionMs(sleep)
	out.DStateMs, _ = foldIntervalUnionMs(dState)
	out.IOWaitMs, _ = foldIntervalUnionMs(ioWait)
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
