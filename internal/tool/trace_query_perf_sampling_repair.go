package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The payload remains the original unavailable receipt. A companion tree
// cannot fail a legacy view whose own filters and statistics are valid.
func traceQuerySamplingTreeRepairResult(out types.ToolResult, result tracequery.Result, q tracequery.Query) types.ToolResult {
	if !out.Success || out.Repair != nil || tracequery.CanonicalViewName(q.View) != tracequery.ViewPerfSamplingTree || result.PerfSamplingTree == nil {
		return out
	}
	p := result.PerfSamplingTree
	fields := tracequery.PerfSamplingTreeUnsupportedSelectors(q)
	if p.Status != "unavailable" || p.Reason != "requires_time_and_thread_selectors_only" || len(fields) == 0 {
		return out
	}
	hint := "perf_sampling_tree supports time_start/time_end and pid/thread. Unsupported fields in this call: " + strings.Join(fields, ", ") + ". Keep the same capture, requested time window and explicit thread; time bounds are supported. No filters were removed or ignored. Unavailable is not a measured zero-sample population. If the unsupported field was intended only to select a thread, express that selection with pid/thread and explicitly retry. If content/event/line filtering is intentional, preserve it in a compatible query; its rows are not a complete sampling-tree denominator."
	out.Success = false
	out.Repair = &types.ToolRepair{Code: "trace_query_sampling_selector_unsupported", Fields: fields, Hint: hint, Metadata: map[string]string{"view": tracequery.ViewPerfSamplingTree, "retry_scope": "same_source_and_window", "next_tool": "trace_query", "status": types.ToolRepairStatusActionRequired}}
	out.Summary = hint + "\n\n" + out.Summary
	return out
}
