package tracequery

import (
	"sort"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

type cpuNativeInterval struct {
	row  tracewire.CPUMeasureInterval
	line int
}

// A bundle is eligible only through the parser's verified held-source ledger
// and an explicit identity clock mapping. One child alone is not that proof.
func cpuStateFrequencySingleSource(idx *Index) bool {
	if idx.Windowed || idx.RelationScoped || len(idx.TraceArtifacts) > 1 {
		return false
	}
	if len(idx.TraceArtifacts) == 0 {
		return true
	}
	s := idx.TraceArtifacts[0]
	if s.SourcePath == idx.Path {
		return true
	}
	return hasNativeCPUIntervals(idx) && s.sourceIdentity.Initialized() && s.CausalCompatible && s.ClockAlignment == TraceClockAlignmentIdentity &&
		s.TimeDomain != "" && sameTraceTimeDomain(s.TimeDomain, s.CanonicalTimeDomain) &&
		traceClockMapIsIdentity(s.ClockOffsetSec, s.ClockSlope) && s.VirtualLineBase == 0 && s.LocalLineCount == idx.LineCount
}

func (c *cpuStateFrequencyCollector) finishNative(idx *Index, q Query) *CPUStateFrequencyResult {
	p := unavailableCPUStateFrequency(idx.Path, q, "no_native_cpu_intervals_before_window_end")
	perCPU := map[int][]cpuNativeInterval{}
	for _, interval := range c.native {
		r := interval.row
		if r.StartNS == nil || float64(*r.StartNS)/1e9 < q.TimeEnd {
			perCPU[r.CPU] = append(perCPU[r.CPU], interval)
		}
	}
	var cpus []int
	for cpu := range perCPU {
		cpus = append(cpus, cpu)
	}
	sort.Ints(cpus)
	if len(cpus) == 0 {
		return p
	}
	p.Status, p.Reason = "available", ""
	p.Caveats = []string{
		"Native SQL CPU state codes are observed opaque values; code 0 and 4294967295 do not inherit ftrace idle/exit meanings. Active/Running/C-state depth is unverified.",
		"Only explicit measure.ts/dur intervals cover time. Missing duration, gaps, zero/unknown frequency and overlapping source records remain unknown; no next-update or trace-end extension is used.",
		"Frequency is kHz. Per-CPU percentages use the full selected wall-clock window; CPU-time sums one full window per observed CPU, not hardware capacity or thread execution.",
		"Native interval observations are not ftrace CPU controls; only interval-aware consumers may use their explicit measured frequency coverage. These observations alone do not establish a response root cause.",
	}
	if q.timeStartBackfilled || q.timeEndBackfilled {
		p.Caveats = append(p.Caveats, "The selected window uses the existing parsed timestamp envelope where no endpoint was requested; measure row starts do not independently establish whole-capture coverage. No duration endpoint extends the capture range.")
	}
	topology := parseCoreTopology(q.CoreTopology)
	for _, cpu := range cpus {
		if q.runCancel.tick() {
			return p
		}
		row := buildNativeCPUStateFrequencyCPU(cpu, perCPU[cpu], q)
		if class := topology[cpu]; class != "" {
			row.CoreClass, row.TopologySource = class, "explicit"
		}
		p.CPUCount++
		p.CPUTimeMs += p.WindowWallMs
		p.KnownJointMs += row.JointKnownMs
		p.UnknownJointMs += row.UnknownJointMs
		if len(p.CPUs) < cpuStateFrequencyCPULimit {
			p.CPUs = append(p.CPUs, row)
		} else {
			p.OmittedCPUs++
		}
	}
	return p
}

type cpuNativeBoundary struct {
	ts    float64
	index int
	enter bool
}

func buildNativeCPUStateFrequencyCPU(cpu int, samples []cpuNativeInterval, q Query, sinks ...func(CPUStateFrequencyInterval)) CPUStateFrequencyCPU {
	row := CPUStateFrequencyCPU{CPU: cpu, CoreClass: "unknown", TopologySource: "unknown", WindowWallMs: (q.TimeEnd - q.TimeStart) * 1000}
	edges := []cpuNativeBoundary{{ts: q.TimeStart, index: -1}, {ts: q.TimeEnd, index: -1}}
	for i, sample := range samples {
		r := sample.row
		// Unpositionable data poisons only its CPU/quantity. A known start with
		// an unknown end masks potentially overlapping observations from that
		// start onward, but never contributes measured coverage itself.
		if r.StartNS == nil || r.Issue == "invalid_filter_id" {
			if r.Kind == "idle" {
				row.IdleUnavailable = "native_interval_unpositionable"
			} else {
				row.FrequencyUnavailable = "native_interval_unpositionable"
			}
			continue
		}
		if r.DurationNS != nil && *r.DurationNS == 0 {
			continue
		}
		start, end := float64(*r.StartNS)/1e9, q.TimeEnd
		if r.DurationNS != nil {
			end = float64(*r.StartNS+*r.DurationNS) / 1e9
			if end == start && start >= q.TimeStart && start < q.TimeEnd {
				if r.Kind == "idle" {
					row.IdleUnavailable = "native_interval_timestamp_precision_unrepresentable"
				} else {
					row.FrequencyUnavailable = "native_interval_timestamp_precision_unrepresentable"
				}
			}
		}
		if end <= q.TimeStart || start >= q.TimeEnd {
			continue
		}
		if start < q.TimeStart {
			start = q.TimeStart
		}
		if end > q.TimeEnd {
			end = q.TimeEnd
		}
		if end <= start {
			continue
		}
		edges = append(edges, cpuNativeBoundary{start, i, true}, cpuNativeBoundary{end, i, false})
	}
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].ts < edges[j].ts })
	idle, frequency := map[int]bool{}, map[int]bool{}
	groups := map[cpuStateFrequencyKey]int{}
	for pos := 0; pos < len(edges); {
		if q.runCancel.tick() {
			break
		}
		start := edges[pos].ts
		next := pos
		for next < len(edges) && edges[next].ts == start {
			e := edges[next]
			if e.index >= 0 {
				active := frequency
				if samples[e.index].row.Kind == "idle" {
					active = idle
				}
				if e.enter {
					active[e.index] = true
				} else {
					delete(active, e.index)
				}
			}
			next++
		}
		if next == len(edges) {
			break
		}
		end := edges[next].ts
		iv := CPUStateFrequencyInterval{CPUStateFrequencyValue: CPUStateFrequencyValue{State: "unknown", StateEncoding: "native_sql_idle"}, StartTs: start, EndTs: end, DurationMs: (end - start) * 1000}
		if row.IdleUnavailable == "" && len(idle) == 1 {
			for i := range idle {
				if r := samples[i].row; r.Value != nil && r.Issue == "" {
					value := uint32(*r.Value)
					iv.State, iv.StateKnown, iv.IdleState, iv.IdleLine = "native_idle", true, &value, samples[i].line
				}
			}
		}
		if row.FrequencyUnavailable == "" && len(frequency) == 1 {
			for i := range frequency {
				if r := samples[i].row; r.Value != nil && *r.Value > 0 && r.Issue == "" {
					value := *r.Value
					iv.FrequencyKnown, iv.FrequencyKHz, iv.FrequencyLine = true, &value, samples[i].line
				}
			}
		}
		appendCPUStateFrequencyInterval(&row, groups, iv)
		for _, sink := range sinks {
			if sink != nil {
				sink(iv)
			}
		}
		pos = next
	}
	finishCPUStateFrequencyGroups(&row)
	return row
}
