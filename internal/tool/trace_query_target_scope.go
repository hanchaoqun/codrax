package tool

import "github.com/hanchaoqun/codrax/internal/tracequery"

// Shared ordinary-query admission predicate. Planning may retain an
// unsupported process/view request as pending, never silently change its kind.
func traceQueryViewSupportsProcessScope(view string) bool {
	switch tracequery.CanonicalViewName(view) {
	case "span_window", "frame_window", "render_pipeline", "frame_timeline", "frame_flow", "frame_root_cause_bundle", tracequery.ViewRenderingCandidates, tracequery.ViewProcessMeasurements, tracequery.ViewPreferredFrameRate, tracequery.ViewTransactionHandoffs:
		return true
	default:
		return false
	}
}
