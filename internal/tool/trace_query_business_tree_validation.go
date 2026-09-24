package tool

import (
	"math"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Exact typed measurements, not text heuristics. A mismatched note must not
// teach a different value from the observation's generic display face.
func traceBusinessTreeFactValid(f TraceBusinessTreeFact) bool {
	n := f.Node
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if n.ID == "" || f.IndexPath == "" || n.Thread.PID <= 0 || n.StartLine <= 0 || n.SourcePath == "" ||
		!finite(n.ActualStartTs) || !finite(f.Window.StartTs) || !finite(f.Window.EndTs) || f.Window.EndTs < f.Window.StartTs ||
		f.NodeCount <= 0 || f.OmittedNodes < 0 || f.OmittedNodes >= f.NodeCount || n.DirectChildCount < 0 {
		return false
	}
	switch n.ParentStatus {
	case "observed_parent":
		if n.ParentID == "" || n.ParentID == n.ID {
			return false
		}
	case "observed_root", "unknown_prefix":
		if n.ParentID != "" {
			return false
		}
	default:
		return false
	}
	switch n.Closure {
	case "open", "invalidated":
		return n.Inclusive == nil && n.Self == nil && n.ActualEndTs == nil
	case "closed":
		if n.ActualEndTs == nil || !finite(*n.ActualEndTs) || *n.ActualEndTs < n.ActualStartTs || n.EndLine < n.StartLine || n.Inclusive == nil || n.Self == nil {
			return false
		}
	default:
		return false
	}
	span, _ := traceBusinessTreeFactProjection(f)
	w := tracequery.TimeWindow{StartTs: span.StartTs, EndTs: span.EndTs}
	if len(n.Inclusive.Segments) != 1 || n.Inclusive.OmittedSegments != 0 || w.StartTs < n.ActualStartTs || w.EndTs > *n.ActualEndTs {
		return false
	}
	if f.WindowUnavailableReason == "" && (w.StartTs != math.Max(f.Window.StartTs, n.ActualStartTs) || w.EndTs != math.Min(f.Window.EndTs, *n.ActualEndTs)) {
		return false
	}
	return w.EndTs >= w.StartTs && math.Abs(n.Inclusive.DurationMs-(w.EndTs-w.StartTs)*1000) <= 1e-6 &&
		n.Self.DurationMs <= n.Inclusive.DurationMs+1e-6 && traceBusinessTreeAccountValid(n.Inclusive, w) && traceBusinessTreeAccountValid(n.Self, w)
}

func traceBusinessTreeAccountValid(a *tracequery.TraceMarkerTreeAccount, w tracequery.TimeWindow) bool {
	valid := func(v float64) bool { return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !valid(a.DurationMs) || a.OmittedSegments < 0 {
		return false
	}
	shown, end := 0.0, w.StartTs
	for _, s := range a.Segments {
		if math.IsNaN(s.StartTs) || math.IsNaN(s.EndTs) || s.StartTs < end || s.EndTs < s.StartTs || s.EndTs > w.EndTs {
			return false
		}
		shown += (s.EndTs - s.StartTs) * 1000
		end = s.EndTs
	}
	if shown > a.DurationMs+1e-6 || a.OmittedSegments == 0 && math.Abs(shown-a.DurationMs) > 1e-6 {
		return false
	}
	s := a.States
	if s == nil {
		return true
	}
	if !valid(s.UnknownMs) || s.UnknownMs > a.DurationMs+1e-6 {
		return false
	}
	if s.Values == nil {
		return s.Coverage == "unavailable" && math.Abs(s.UnknownMs-a.DurationMs) <= 1e-6
	}
	v := s.Values
	total := 0.0
	for _, part := range []float64{v.RunningMs, v.RunnableMs, v.SleepMs, v.DStateMs, v.IOWaitMs, v.StoppedMs, v.DeadMs} {
		if !valid(part) {
			return false
		}
		total += part
	}
	return (s.Coverage == "partial" || s.Coverage == "complete" && s.UnknownMs == 0) && valid(v.AccountedMs) && valid(v.SleepIOWaitMs) && v.SleepIOWaitMs <= v.SleepMs+1e-6 &&
		math.Abs(total-v.AccountedMs) <= 1e-6 && math.Abs(total+s.UnknownMs-a.DurationMs) <= 1e-6
}
