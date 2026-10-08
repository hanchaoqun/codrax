package tracequery

import (
	"math"
	"sort"
)

// nativeCPUFrequencyWindow adapts explicit source intervals for the existing
// window consumers. It never inserts synthetic controls into Index.Events or
// makes an interval endpoint a hardware transition/root-cause observation.
type nativeCPUFrequencyWindow struct {
	byCPU  map[int][]CPUFrequencyResidency
	reason string
}

func nativeCPUFrequencyForWindow(idx *Index, q Query) *nativeCPUFrequencyWindow {
	if idx == nil {
		return nil
	}
	present := idx.CPUIntervalMalformed > 0
	for _, ev := range idx.Events {
		if present || ev.Type == EventCPUMeasureInterval {
			present = true
			break
		}
	}
	if !present {
		return nil
	}
	c := newCPUStateFrequencyCollector()
	for _, ev := range idx.Events {
		if !c.observe(ev) {
			break
		}
		if q.runCancel.tick() {
			return &nativeCPUFrequencyWindow{reason: "native_frequency_scan_cancelled"}
		}
	}
	w := &nativeCPUFrequencyWindow{byCPU: map[int][]CPUFrequencyResidency{}}
	// Window statistics may name a thread, but CPU frequency remains a
	// CPU-owned measurement. All other source/selection gates are unchanged.
	cpuQ := q
	cpuQ.PID, cpuQ.Thread, cpuQ.ThreadInput, cpuQ.TargetScope = 0, "", "", ""
	if validateCPUStateFrequencyQuery(cpuQ) != nil || q.TimeEnd <= q.TimeStart {
		w.reason = "native_frequency_requires_unfiltered_finite_window"
	} else if !cpuStateFrequencySingleSource(idx) {
		w.reason = "native_frequency_requires_complete_single_source"
	} else if c.overflow {
		w.reason = "cpu_control_sample_limit"
	} else if c.nativeInvalid || idx.CPUIntervalMalformed > 0 {
		w.reason = "malformed_native_cpu_interval_carrier"
	}
	if w.reason != "" {
		return w
	}
	byCPU := map[int][]cpuNativeInterval{}
	for _, interval := range c.native {
		if interval.row.StartNS == nil || float64(*interval.row.StartNS)/1e9 < q.TimeEnd {
			byCPU[interval.row.CPU] = append(byCPU[interval.row.CPU], interval)
		}
	}
	for cpu, intervals := range byCPU {
		buildNativeCPUStateFrequencyCPU(cpu, intervals, cpuQ, func(iv CPUStateFrequencyInterval) {
			if !iv.FrequencyKnown || iv.FrequencyKHz == nil {
				return
			}
			rows := w.byCPU[cpu]
			if n := len(rows); n > 0 && rows[n-1].Frequency == *iv.FrequencyKHz && rows[n-1].EndTs == iv.StartTs {
				rows[n-1].EndTs, rows[n-1].LineEnd = iv.EndTs, iv.FrequencyLine
				rows[n-1].DurationMs += iv.DurationMs
			} else {
				rows = append(rows, CPUFrequencyResidency{Frequency: *iv.FrequencyKHz, DurationMs: iv.DurationMs,
					StartTs: iv.StartTs, EndTs: iv.EndTs, LineStart: iv.FrequencyLine, LineEnd: iv.FrequencyLine})
			}
			w.byCPU[cpu] = rows
		})
	}
	return w
}

func (w *nativeCPUFrequencyWindow) at(cpu int, ts float64) int64 {
	rows := w.byCPU[cpu]
	i := sort.Search(len(rows), func(i int) bool { return rows[i].EndTs > ts })
	if i < len(rows) && rows[i].StartTs <= ts {
		return rows[i].Frequency
	}
	return 0
}

// Full coverage is required for an existing segment-level scalar. A gap
// stays unknown rather than diluting an average or borrowing the next value.
func (w *nativeCPUFrequencyWindow) segment(cpu int, start, end float64) frequencySegmentStats {
	var out frequencySegmentStats
	if end <= start || w.reason != "" {
		return out
	}
	rows := w.byCPU[cpu]
	i := sort.Search(len(rows), func(i int) bool { return rows[i].EndTs > start })
	cursor, weighted := start, 0.0
	for ; i < len(rows) && cursor < end; i++ {
		r := rows[i]
		if r.StartTs > cursor {
			return frequencySegmentStats{}
		}
		stop := math.Min(end, r.EndTs)
		weighted += float64(r.Frequency) * (stop - cursor)
		out.observedMaxKHz = max(out.observedMaxKHz, r.Frequency)
		cursor = stop
	}
	if cursor < end {
		return frequencySegmentStats{}
	}
	out.weightedKHz, out.known = weighted/(end-start), true
	return out
}

func (w *nativeCPUFrequencyWindow) applyResidency(in []CPUStats, q Query, unavailableReason string) []CPUStats {
	for i := range in {
		in[i].Frequency, in[i].FrequencyResidency = 0, nil
	}
	for cpu, rows := range w.byCPU {
		if len(rows) == 0 {
			continue
		}
		pos := -1
		for i := range in {
			if in[i].CPU == cpu {
				pos = i
				break
			}
		}
		if pos < 0 {
			in = append(in, CPUStats{CPU: cpu, BusyIdleStatus: CPUBusyIdleStatusUnavailable, BusyIdleReason: unavailableReason})
			pos = len(in) - 1
		}
		in[pos].FrequencyResidency = append([]CPUFrequencyResidency(nil), rows...)
		last := rows[len(rows)-1]
		if last.EndTs == q.TimeEnd {
			in[pos].Frequency = last.Frequency
		}
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].CPU < in[j].CPU })
	return in
}

func (w *nativeCPUFrequencyWindow) supplyUnavailableReason(supply map[int]*cpuSupplyAcc, maxima map[int]int64) string {
	if w.reason != "" {
		return w.reason
	}
	for cpu, s := range supply {
		if s.runningMs > 0 && (maxima[cpu] <= 0 || math.Abs(s.runningMs-s.freqKnownMs) > 1e-6) {
			return "native_frequency_does_not_cover_running_intervals"
		}
	}
	return ""
}
