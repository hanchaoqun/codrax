package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reuse only a current source receipt and the same effective selector. A
// different scope or any new query parameter still runs the normal view.
func reuseTraceFrameBundleView(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string, replay json.RawMessage) (types.ToolResult, bool) {
	view := tracequery.CanonicalViewName(p.View)
	if sourceLabel != "attached_trace" || (view != "window_stats" && view != "root_cause_rank" && view != "wakeup_chain") ||
		ctx == nil || ctx.PipelineStage != types.StageExplore || ctx.Mutable == nil || !ctx.AttachedTraceMaterial.SinglePhysicalSource() ||
		ctx.Mutable.SystemTraceSupplementInProgress() || contextFromBus(ctx).Err() != nil {
		return types.ToolResult{}, false
	}
	pid, start, end, known := types.KnownSingleFrameTraceTarget(ctx.Mutable.RequestModel())
	if !known || p.PID.Int() != pid || p.TimeStart.Seconds() != start || p.TimeEnd.Seconds() != end {
		return types.ToolResult{}, false
	}
	var wanted map[string]json.RawMessage
	if json.Unmarshal(replay, &wanted) != nil {
		return types.ToolResult{}, false
	}
	physical, ok := traceQueryRequestWindowPathIdentity(ctx, path)
	if !ok {
		return types.ToolResult{}, false
	}
	delete(wanted, "view")
	delete(wanted, "path") // Compared using the current native identity below.
	for _, previous := range ctx.Mutable.DispatchToolResults() {
		if !slices.Contains(previous.TraceFrameBundleCoveredViews, view) {
			continue
		}
		actual, raw, current := ctx.Mutable.ResolveTraceQueryWindowReplay(previous.TraceQueryWindowReplay)
		if !current || actual != physical {
			continue
		}
		var existing map[string]json.RawMessage
		if json.Unmarshal(raw, &existing) != nil {
			continue
		}
		delete(existing, "view")
		delete(existing, "path")
		if !maps.EqualFunc(wanted, existing, func(a, b json.RawMessage) bool { return bytes.Equal(a, b) }) {
			continue
		}
		return types.ToolResult{
			ToolName: "trace_query",
			Success:  true,
			Summary: fmt.Sprintf("Reused %s evidence from the existing frame_root_cause_bundle for thread %d at %g..%g seconds. Use the original observations and citations in the bundle result (raw_ref=%s). Only query a different scope or an uncovered fact; if the request is covered, complete the investigation.",
				view, pid, start, end, previous.RawRef),
			RawRef:    previous.RawRef,
			Timestamp: time.Now(),
		}, true
	}
	return types.ToolResult{}, false
}
