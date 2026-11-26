package tool

import (
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Publish operands from the native result already in memory. Only single
// physical captures have source-local lines in this comparison contract.
func traceQueryComparisonMeasurements(result tracequery.Result, ref types.ObservationSourceRef) []types.TraceComparisonMeasurement {
	if len(result.TraceArtifacts) != 1 || result.TraceArtifacts[0].VirtualLineBase != 0 ||
		!types.RuntimeArtifactComparisonPathEqual(result.TraceArtifacts[0].SourcePath, result.SourcePath, "") || result.ViewCancellation != nil {
		return nil
	}
	var measurements []types.TraceComparisonMeasurement
	add := func(name, caliber, coverage string, tid, startLine, endLine int, start, end float64, value *int64) {
		a, aok := types.TraceComparisonTimeNS(start)
		b, bok := types.TraceComparisonTimeNS(end)
		if !aok || !bok || b < a || tid <= 0 || strings.TrimSpace(name) == "" || len(name) > 192 {
			return
		}
		operand := types.TraceComparisonMeasurement{Metric: name, Caliber: caliber, Coverage: coverage, ThreadID: tid, StartLine: startLine, EndLine: endLine, StartNS: a, EndNS: b, Value: value}
		if lo, hi, ok := types.TraceObservationContinuousQueryWindow(ref); ok {
			l, lok := types.TraceComparisonTimeNS(lo)
			h, hok := types.TraceComparisonTimeNS(hi)
			if lok && hok {
				operand.QueryStartNS, operand.QueryEndNS = &l, &h
			}
		}
		measurements = append(measurements, operand)
	}
	duration := func(start, end float64) *int64 {
		a, ok := types.TraceComparisonTimeNS(start)
		b, bok := types.TraceComparisonTimeNS(end)
		if !ok || !bok || b <= a {
			return nil
		}
		d := b - a
		return &d
	}
	for _, e := range result.Events {
		if (e.SpanAction != "B" && e.SpanAction != "I") || e.LocalLine <= 0 || !types.RuntimeArtifactComparisonPathEqual(e.SourcePath, result.SourcePath, "") {
			continue
		}
		caliber := "marker_begin"
		if e.SpanAction == "I" {
			caliber = "marker_instant"
		}
		add(types.TraceComparisonMarkerName(e.SpanName), caliber, "endpoint_observed", e.PID, e.LocalLine, 0, e.SourceTs, e.SourceTs, nil)
	}
	spans := append([]tracequery.TraceSpanSummary(nil), result.SpanWindows...)
	if result.WindowStats != nil {
		spans = append(spans, result.WindowStats.TraceSpans...)
	}
	for _, s := range spans {
		if s.Kind != "sync" || s.StartLine <= 0 || s.EndLine <= s.StartLine || !types.RuntimeArtifactComparisonPathEqual(s.SourcePath, result.SourcePath, "") {
			continue
		}
		lo, hi := s.StartTs, s.EndTs
		if s.ActualEndTs > s.ActualStartTs {
			lo, hi = s.ActualStartTs, s.ActualEndTs
		}
		if v := duration(lo, hi); v != nil {
			add(types.TraceComparisonMarkerName(s.Name), "closed_sync_span", "complete", s.Thread.PID, s.StartLine, s.EndLine, lo, hi, v)
		}
	}
	if result.WindowStats != nil && result.WindowStats.BusinessTree != nil {
		for _, s := range result.WindowStats.BusinessTree.Nodes {
			if s.Closure != "closed" || s.ActualEndTs == nil || s.StartLine <= 0 || s.EndLine <= s.StartLine || !types.RuntimeArtifactComparisonPathEqual(s.SourcePath, result.SourcePath, "") {
				continue
			}
			if v := duration(s.ActualStartTs, *s.ActualEndTs); v != nil {
				add(types.TraceComparisonMarkerName(s.Name), "closed_sync_span", "complete", s.Thread.PID, s.StartLine, s.EndLine, s.ActualStartTs, *s.ActualEndTs, v)
			}
		}
	}
	if s := traceQueryTargetWindowStatesAccount(result); s != nil && s.Window.EndTs > s.Window.StartTs && s.WindowMs > 0 && math.Abs(s.WindowMs-s.TotalMs) < 0.000001 {
		for _, m := range []struct {
			name string
			ms   float64
		}{
			{"thread.running_wall_time", s.RunningMs}, {"thread.runnable_wall_time", s.RunnableMs}, {"thread.sleep_wall_time", s.SleepMs}, {"thread.d_state_excluding_io_wall_time", s.DStateMs}, {"thread.scheduler_marked_io_wait_wall_time", s.IOWaitMs},
		} {
			if ns, ok := types.TraceComparisonTimeNS(m.ms / 1000); ok {
				add(m.name, "target_thread_state_partition", "complete", s.Thread.PID, s.LineStart, s.LineEnd, s.Window.StartTs, s.Window.EndTs, &ns)
			}
		}
	}
	return measurements
}
