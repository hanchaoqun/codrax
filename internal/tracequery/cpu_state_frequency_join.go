package tracequery

import (
	"math"
	"sort"
)

func buildCPUStateFrequencyCPU(cpu int, lane *cpuStateFrequencyLane, q Query) CPUStateFrequencyCPU {
	row := CPUStateFrequencyCPU{CPU: cpu, CoreClass: "unknown", TopologySource: "unknown", WindowWallMs: (q.TimeEnd - q.TimeStart) * 1000,
		IdleUnavailable: lane.idleIssue, FrequencyUnavailable: lane.freqIssue}
	var freqs []CPUFrequencyResidency
	if lane.freqIssue == "" {
		events := make([]Event, 0, len(lane.freq))
		for _, s := range lane.freq {
			events = append(events, Event{Type: EventCPUFrequency, Name: "cpu_frequency", Ts: s.ts, Line: s.line,
				CPUForField: cpu, CPUForFieldValid: true, Frequency: int64(s.value)})
		}
		freqs, _ = computeCPUFrequencyResidency(events, q)
	}
	// Independent state and frequency boundaries preserve every positive-width
	// gap. Values at the right boundary do not govern this half-open window.
	boundaries := []float64{q.TimeStart, q.TimeEnd}
	if lane.idleIssue == "" {
		for _, s := range lane.idle {
			if s.ts > q.TimeStart && s.ts < q.TimeEnd {
				boundaries = append(boundaries, s.ts)
			}
		}
	}
	for _, f := range freqs {
		boundaries = append(boundaries, f.StartTs, f.EndTs)
	}
	sort.Float64s(boundaries)
	idlePos, freqPos := 0, 0
	var idle *cpuStateFrequencySample
	groups := map[cpuStateFrequencyKey]int{}
	for i := 0; i+1 < len(boundaries); i++ {
		if q.runCancel.tick() {
			return row
		}
		start, end := boundaries[i], boundaries[i+1]
		if end <= start {
			continue
		}
		v := CPUStateFrequencyValue{State: "unknown"}
		interval := CPUStateFrequencyInterval{StartTs: start, EndTs: end, DurationMs: (end - start) * 1000}
		if lane.idleIssue == "" {
			for idlePos < len(lane.idle) && lane.idle[idlePos].ts <= start {
				idle = &lane.idle[idlePos]
				idlePos++
			}
			if idle != nil && idle.known {
				v.StateKnown = true
				interval.IdleLine = idle.line
				if idle.value == math.MaxUint32 {
					v.State = "active"
				} else {
					v.State = "idle"
					state := idle.value
					v.IdleState = &state
				}
			}
		}
		for freqPos < len(freqs) && freqs[freqPos].EndTs <= start {
			freqPos++
		}
		if freqPos < len(freqs) && freqs[freqPos].StartTs <= start && freqs[freqPos].EndTs >= end {
			freq := freqs[freqPos].Frequency
			v.FrequencyKnown = true
			v.FrequencyKHz = &freq
			interval.FrequencyLine = freqs[freqPos].LineStart
		}
		interval.CPUStateFrequencyValue = v
		if v.StateKnown {
			row.IdleKnownMs += interval.DurationMs
		}
		if v.FrequencyKnown {
			row.FrequencyKnownMs += interval.DurationMs
		}
		if v.StateKnown && v.FrequencyKnown {
			row.JointKnownMs += interval.DurationMs
		} else {
			row.UnknownJointMs += interval.DurationMs
		}
		key := cpuStateFrequencyValueKey(v)
		g, exists := groups[key]
		if !exists {
			g = len(row.Groups)
			groups[key] = g
			row.Groups = append(row.Groups, CPUStateFrequencyGroup{CPUStateFrequencyValue: v})
		}
		row.Groups[g].DurationMs += interval.DurationMs
		row.TotalIntervals++
		if len(row.Intervals) < cpuStateFrequencyIntervalLimit {
			row.Intervals = append(row.Intervals, interval)
		} else {
			row.OmittedIntervals++
		}
	}
	for i := range row.Groups {
		row.Groups[i].WindowPct = row.Groups[i].DurationMs / row.WindowWallMs * 100
	}
	// High-value combinations first; all totals are computed before display.
	sort.SliceStable(row.Groups, func(i, j int) bool { return row.Groups[i].DurationMs > row.Groups[j].DurationMs })
	row.GroupCount = len(row.Groups)
	if row.GroupCount > cpuStateFrequencyGroupLimit {
		row.OmittedGroups = row.GroupCount - cpuStateFrequencyGroupLimit
		row.Groups = row.Groups[:cpuStateFrequencyGroupLimit]
	}
	return row
}

type cpuStateFrequencyKey struct {
	state string
	idle  uint32
	freq  int64
}

func cpuStateFrequencyValueKey(v CPUStateFrequencyValue) cpuStateFrequencyKey {
	k := cpuStateFrequencyKey{state: v.State}
	if v.IdleState != nil {
		k.idle = *v.IdleState
	}
	if v.FrequencyKHz != nil {
		k.freq = *v.FrequencyKHz
	}
	return k
}
