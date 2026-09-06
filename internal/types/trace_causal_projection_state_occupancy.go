package types

import (
	"math"
	"strconv"
	"strings"
)

// The bit order is the existing scheduler-state account order. It preserves
// measured zero without allocating shared mutable maps or pointer fields.
const (
	traceStateOccupancyRunning uint8 = 1 << iota
	traceStateOccupancyRunnable
	traceStateOccupancySleep
	traceStateOccupancyDState
	traceStateOccupancyIOWait
)

func traceCausalProjectionStateOccupancyPresence(notes []string) uint8 {
	var present uint8
	for i, key := range []string{TraceNoteKeyRunning, TraceNoteKeyRunnable, TraceNoteKeySleep, TraceNoteKeyDState, TraceNoteKeyIOWait} {
		raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, key))
		raw = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(raw), "ms"))
		raw = strings.TrimSpace(strings.TrimSuffix(raw, "毫秒"))
		value, err := strconv.ParseFloat(raw, 64)
		if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
			present |= 1 << i
		}
	}
	return present
}

// PublishedStateOccupancy returns only this row's original state account.
// A priced/composite impact, a chain cumulative total, a supply-fold equation,
// or a physical interval extending beyond the row cannot supply a missing
// measurement. Display folds do not preserve every member's state account;
// do not present their retained seed account as the whole folded population.
func (n TraceCausalProjectionNode) PublishedStateOccupancy() (float64, bool) {
	if n.MergedCount > 1 || n.OnChainOverflowFold {
		return 0, false
	}
	var value float64
	var bit uint8
	switch strings.ToLower(strings.TrimSpace(n.StateKind)) {
	case "running", "fragmented_running":
		value, bit = n.RunningMS, traceStateOccupancyRunning
	case "runnable", "runnable_wait", "fragmented_runnable_wait", "scheduler_latency":
		value, bit = n.RunnableMS, traceStateOccupancyRunnable
	case "s_sleep", "sleep", "sleep_wait", "fragmented_sleep_wait":
		value, bit = n.SleepMS, traceStateOccupancySleep
	case "d", "d_sleep", "d_state":
		value, bit = n.DStateSplitMS, traceStateOccupancyDState
	case "io_wait":
		value, bit = n.IOWaitSplitMS, traceStateOccupancyIOWait
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		// This named lane is the engine's D/IO partition, not a priced
		// composite. Both original components must be present; absent IO is
		// not inferred to be zero simply because D was published.
		dState, ioWait := n, n
		dState.StateKind, ioWait.StateKind = "d_state", "io_wait"
		d, dOK := dState.PublishedStateOccupancy()
		io, ioOK := ioWait.PublishedStateOccupancy()
		if dOK && ioOK && !math.IsInf(d+io, 0) {
			return TraceUninterruptibleWaitMS(d, io), true
		}
		return 0, false
	default:
		// Device IO latency and unknown labels are not scheduler states.
		return 0, false
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	return value, value > 0 || n.StateOccupancyPresence&bit != 0
}
