package tool

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// traceQueryCPUStateFrequencyReceipt owns the tabular projection of validated
// native measurements. No model cells, prose, or abbreviated RichNotes supply
// quantities here. These tables grant display authority only.
func traceQueryCPUStateFrequencyReceipt(r types.ObservationRecord, p tracequery.CPUStateFrequencyResult) string {
	start, end, known := types.TraceObservationContinuousQueryWindow(r.SourceRef)
	if !tracequery.ValidCPUStateFrequency(p) || !known || p.SourcePath != r.SourceRef.Path ||
		p.Window.StartTs != start || p.Window.EndTs != end || p.Status != r.Object ||
		p.Window.StartTs != r.Span.StartTs || p.Window.EndTs != r.Span.EndTs ||
		r.SourceRef.QueryTargetPID != 0 || r.SourceRef.QueryTargetThread != "" {
		return ""
	}
	notes := []string{
		fmt.Sprintf("Window [%s, %s) seconds; wall-clock duration %.9g ms.", traceQueryDisplaySeconds(start), traceQueryDisplaySeconds(end), p.WindowWallMs),
		"Source: " + p.SourcePath,
		"State/frequency residency does not establish thread execution, power, compute shortage or a response root cause. Unknown is not zero; CPU-time is not wall time. Native SQL CPU state codes retain unverified meaning; raw idle 0 remains idle and active requires an observed idle exit.",
	}
	notes = append(notes, p.Caveats...)
	table := func(view types.RuntimeMeasurementView, columns []string, rows [][]string, extra ...string) types.RuntimeMeasurementTable {
		return types.RuntimeMeasurementTable{ObservationID: r.ID, View: view, Label: "CPU state and frequency — " + string(view),
			Columns: columns, Rows: rows, Notes: append(append([]string(nil), notes...), extra...)}
	}
	var tables []types.RuntimeMeasurementTable
	if p.Status == "unavailable" {
		// The result's empty counters are absence, not measured CPU-time zeros.
		tables = []types.RuntimeMeasurementTable{table(types.RuntimeMeasurementSummary,
			[]string{"Measurement", "Availability", "Reason"}, [][]string{{"CPU state × frequency residency", "unavailable; quantities unknown", p.Reason}})}
	} else {
		notes = append(notes, fmt.Sprintf("Observed CPUs %d; full-window CPU-time %.9g CPU-ms; jointly known %.9g CPU-ms (%.9g%%); jointly unknown %.9g CPU-ms (%.9g%%). Overall percentages use all observed CPUs × the complete window (%.9g CPU-ms); every per-CPU percentage uses the entire %.9g ms wall-clock window, not just known coverage or displayed rows.", p.CPUCount, p.CPUTimeMs, p.KnownJointMs, p.KnownJointMs/p.CPUTimeMs*100, p.UnknownJointMs, p.UnknownJointMs/p.CPUTimeMs*100, p.CPUTimeMs, p.WindowWallMs),
			fmt.Sprintf("Producer rows: CPUs displayed %d of %d; omitted %d. Tables retain all rows available in the query result, independently of the shorter handoff preview; omitted rows are not reconstructed and displayed rows need not sum to the full totals.", len(p.CPUs), p.CPUCount, p.OmittedCPUs))
		var summary, distribution, timeline [][]string
		var coverage, groupOmissions, intervalOmissions []string
		groups, intervals := 0, 0
		number := func(v float64) string { return strconv.FormatFloat(v, 'g', 9, 64) }
		line := func(v int) string {
			if v <= 0 {
				return "unknown"
			}
			return strconv.Itoa(v)
		}
		for _, cpu := range p.CPUs {
			id := fmt.Sprintf("CPU%d", cpu.CPU)
			summary = append(summary, []string{id, cpu.CoreClass, cpu.TopologySource, number(cpu.WindowWallMs), number(cpu.IdleKnownMs), number(cpu.FrequencyKnownMs), number(cpu.JointKnownMs), number(cpu.UnknownJointMs)})
			if cpu.IdleUnavailable != "" || cpu.FrequencyUnavailable != "" {
				coverage = append(coverage, fmt.Sprintf("%s: state unavailable reason %q; frequency unavailable reason %q.", id, cpu.IdleUnavailable, cpu.FrequencyUnavailable))
			}
			groups += cpu.GroupCount
			intervals += cpu.TotalIntervals
			if cpu.OmittedGroups > 0 {
				groupOmissions = append(groupOmissions, fmt.Sprintf("%s groups displayed %d of %d, omitted %d", id, len(cpu.Groups), cpu.GroupCount, cpu.OmittedGroups))
			}
			if cpu.OmittedIntervals > 0 {
				intervalOmissions = append(intervalOmissions, fmt.Sprintf("%s intervals displayed %d of %d, omitted %d", id, len(cpu.Intervals), cpu.TotalIntervals, cpu.OmittedIntervals))
			}
			for _, group := range cpu.Groups {
				state, frequency := cpuStateFrequencyLabels(group.CPUStateFrequencyValue)
				distribution = append(distribution, []string{id, state, frequency, number(group.DurationMs), number(group.WindowPct)})
			}
			for _, interval := range cpu.Intervals {
				state, frequency := cpuStateFrequencyLabels(interval.CPUStateFrequencyValue)
				timeline = append(timeline, []string{id, traceQueryDisplaySeconds(interval.StartTs), traceQueryDisplaySeconds(interval.EndTs), state, frequency, number(interval.DurationMs), line(interval.IdleLine), line(interval.FrequencyLine)})
			}
		}
		notes = append(notes, coverage...)
		countNote := func(kind string, shown, total int, omissions []string) string {
			note := fmt.Sprintf("Across retained CPUs: %s displayed %d of %d; omitted %d. Counts do not include omitted CPUs.", kind, shown, total, total-shown)
			if len(omissions) > 0 {
				note += " " + strings.Join(omissions, "; ") + "."
			}
			return note
		}
		tables = []types.RuntimeMeasurementTable{
			table(types.RuntimeMeasurementSummary, []string{"CPU", "Core class", "Topology source", "Full window (ms)", "State-known (ms)", "Frequency-known (ms)", "Jointly known (ms)", "Jointly unknown (ms)"}, summary),
			table(types.RuntimeMeasurementDistribution, []string{"CPU", "Observed state", "Frequency (kHz)", "Duration (ms)", "Full CPU window (%)"}, distribution, countNote("groups", len(distribution), groups, groupOmissions)),
			table(types.RuntimeMeasurementTimeline, []string{"CPU", "Start inclusive (s)", "End exclusive (s)", "Observed state", "Frequency (kHz)", "Duration (ms)", "State source line", "Frequency source line"}, timeline, countNote("intervals", len(timeline), intervals, intervalOmissions)),
		}
	}
	publication := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: tables}
	data, err := json.Marshal(publication)
	if err != nil {
		return ""
	}
	return types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)
}
