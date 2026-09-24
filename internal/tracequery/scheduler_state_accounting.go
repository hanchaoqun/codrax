package tracequery

import "github.com/hanchaoqun/codrax/internal/types"

// SchedulerStateAccounting is shared with the observation ledger. It describes
// the accepted cumulative contributions, never a new physical interval or
// permission to join a causal chain.
type SchedulerStateAccounting = types.TraceSchedulerStateAccounting

// CPU running arithmetic consumes a query-clipped switch list. Recover only
// accounting metadata from the already-built native physical endpoint index;
// do not turn the legacy clipped boundary into a physical switch-out.
func schedulerRunningAccountingSegment(c *schedulerConcurrencyCollector, ev Event, segment schedulerMeasurementSegment) (schedulerMeasurementSegment, bool) {
	if c == nil {
		return segment, false
	}
	if i, ok := c.endpointEvents[ev.Line]; ok {
		segment.ActualStartTs = c.idx.Events[i].Ts
	} else if c.head != nil {
		if head, ok := c.head.CPUs[ev.CPU]; ok && head.Line == ev.Line && head.Thread.PID == ev.NextPID {
			segment.ActualStartTs = head.StartTs
		}
	}
	ordinal, found := c.nextSwitch[ev.Line]
	if !found && !c.seenSwitchLines[ev.Line] {
		for _, i := range c.switches[ev.CPU] {
			next := c.idx.Events[i]
			if next.Line > ev.Line && next.Ts >= segment.ActualStartTs {
				ordinal, found = i, true
				break
			}
		}
	}
	if !found {
		return segment, false
	}
	next := c.idx.Events[ordinal]
	segment.ActualEndTs, segment.EndLine, segment.Closure = next.Ts, next.Line, string(EventSchedSwitch)
	startSource, startOK := tracePairingSourceIdentity(c.idx, ev)
	endSource, endOK := tracePairingSourceIdentity(c.idx, next)
	valid := next.PrevPID == ev.NextPID && next.Ts >= segment.EndTs && startOK && endOK && startSource == endSource &&
		!c.cpuSources[ev.CPU].conflict && !c.tidSources[ev.NextPID].conflict
	return segment, valid
}

func addSchedulerStateAccounting(dst **SchedulerStateAccounting, segment schedulerMeasurementSegment, observedEnd bool) {
	if segment.DurationMs <= 0 || !schedulerMeasurementFinite(segment.DurationMs) {
		return
	}
	if *dst == nil {
		*dst = &SchedulerStateAccounting{State: string(segment.State), Caliber: "cumulative_segments"}
	}
	a := *dst
	a.SegmentCount++
	if segment.ActualStartTs < segment.StartTs {
		a.StartClippedCount++
	}
	// A synthetic open-tail endpoint is not an observed end to compare with
	// the query. Its separate open-tail account already declares that limit.
	if observedEnd && segment.EndTs < segment.ActualEndTs {
		a.EndClippedCount++
	}
	switch {
	case observedEnd && segment.EndLine > 0:
		a.ObservedEndCount++
		a.ObservedEndMs += segment.DurationMs
		if segment.Closure == runnableCPUContinuityBoundaryMigration {
			a.BoundaryContinuationCount++
		}
	case segment.EndLine == 0 && segment.Closure == runnableCPUContinuityBoundaryWindowEnd:
		a.OpenTailCount++
		a.OpenTailMs += segment.DurationMs
	default:
		a.UnknownClosureCount++
		a.UnknownClosureMs += segment.DurationMs
	}
}

// Only the original native close token and original endpoint qualify. Public
// ThreadDuration.LineEnd may have been filled from an opening/reason line.
func schedulerStateObservedEnd(idx *Index, segment schedulerMeasurementSegment) bool {
	if segment.EndLine <= 0 {
		return false
	}
	startSource, startOK := tracePairingSourceIdentity(idx, Event{Line: segment.StartLine})
	endSource, endOK := tracePairingSourceIdentity(idx, Event{Line: segment.EndLine})
	if !startOK || !endOK || startSource != endSource {
		return false
	}
	switch segment.Closure {
	case string(EventSchedSwitch), string(EventSchedWakeup), string(EventSchedWaking),
		runnableCPUContinuityBoundarySchedIn, runnableCPUContinuityBoundaryMigration:
		return true
	default:
		return false
	}
}

func mergeSchedulerStateAccounting(a, b *SchedulerStateAccounting) *SchedulerStateAccounting {
	// Unknown membership is sticky. A legacy cumulative row cannot be counted
	// as one invented segment or repaired by a later row with metadata.
	if a == nil || b == nil || a.State != b.State || a.Caliber != b.Caliber {
		return nil
	}
	a.SegmentCount += b.SegmentCount
	a.ObservedEndCount += b.ObservedEndCount
	a.ObservedEndMs += b.ObservedEndMs
	a.OpenTailCount += b.OpenTailCount
	a.OpenTailMs += b.OpenTailMs
	a.UnknownClosureCount += b.UnknownClosureCount
	a.UnknownClosureMs += b.UnknownClosureMs
	a.StartClippedCount += b.StartClippedCount
	a.EndClippedCount += b.EndClippedCount
	a.BoundaryContinuationCount += b.BoundaryContinuationCount
	return a
}

type schedulerStateAccounts map[ThreadState]*SchedulerStateAccounting

func (accounts schedulerStateAccounts) add(segment schedulerMeasurementSegment, observedEnd bool) {
	value := accounts[segment.State]
	addSchedulerStateAccounting(&value, segment, observedEnd)
	accounts[segment.State] = value
}

func (accounts schedulerStateAccounts) finish() []SchedulerStateAccounting {
	var result []SchedulerStateAccounting
	for _, state := range []ThreadState{StateRunning, StateRunnable, StateSSleep, StateDSleep, StateIOWait} {
		if value := accounts[state]; value != nil {
			result = append(result, *value)
		}
	}
	return result
}

func schedulerStateAccountingForState(accounts []SchedulerStateAccounting, state string) *SchedulerStateAccounting {
	for i := range accounts {
		if accounts[i].State == state {
			return types.CloneTraceSchedulerStateAccounting(&accounts[i])
		}
	}
	return nil
}
