package tracequery

import "math"

// TraceMarkerTreeDisplayLess prioritizes complete measured accounts belonging
// to one explicit physical thread before an independent display budget. It is
// not an inventory filter, parent/child proof, or causal/contribution ranking.
// A missing elapsed account stays unknown; it is never treated as zero time.
// A nonpositive target preserves the legacy stable physical-line ordering.
func TraceMarkerTreeDisplayLess(a, b TraceMarkerTreeNode, targetPID int) bool {
	if targetPID <= 0 {
		return a.StartLine < b.StartLine
	}
	aTarget, bTarget := a.Thread.PID == targetPID, b.Thread.PID == targetPID
	if aTarget != bTarget {
		return aTarget
	}
	if aTarget {
		aKnown, bKnown := traceMarkerTreeDisplayDurationKnown(a), traceMarkerTreeDisplayDurationKnown(b)
		if aKnown != bKnown {
			return aKnown
		}
		if aKnown && a.Inclusive.DurationMs != b.Inclusive.DurationMs {
			return a.Inclusive.DurationMs > b.Inclusive.DurationMs
		}
	}
	if a.StartLine != b.StartLine {
		return a.StartLine < b.StartLine
	}
	if a.SourcePath != b.SourcePath {
		return a.SourcePath < b.SourcePath
	}
	return a.ID < b.ID
}

func traceMarkerTreeDisplayDurationKnown(n TraceMarkerTreeNode) bool {
	return n.Closure == "closed" && n.Inclusive != nil && n.Inclusive.DurationMs >= 0 &&
		!math.IsNaN(n.Inclusive.DurationMs) && !math.IsInf(n.Inclusive.DurationMs, 0)
}
