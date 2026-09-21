package tool

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Discovery and query navigation do not accept a focus. Keep the separate
// completion-selection lane identical in the query schema and tool message.
const traceQueryBusinessRefCompletionTeaching = "Using this reference in trace_query is navigation only; it does not accept a completion focus or prove a root cause. To select that exact instance for automatic supplementation, separately copy the published token into the optional top-level emit_investigation_complete.business_span_ref field. Selection takes effect only after the completion is accepted and its exploration dispatch succeeds."

// These are navigation candidates, not causal or model-selected focus facts.
// Only returned native complete pairs can supply coordinates. The view name
// does not grant or remove a pair; frame summaries and ranking prose cannot
// reconstruct one. Publication still requires the successful read receipt.
func traceQueryBusinessSpanCandidates(_ traceQueryParams, results ...tracequery.Result) []types.TraceBusinessSpanCandidate {
	var out []types.TraceBusinessSpanCandidate
	seen := make(map[types.TraceBusinessSpanCandidate]bool)
	for _, result := range results {
		if len(result.TraceArtifacts) != 1 || result.TraceArtifacts[0].VirtualLineBase != 0 ||
			filepath.Clean(result.TraceArtifacts[0].SourcePath) != filepath.Clean(result.SourcePath) ||
			traceQueryToolViewCancellation(result) != nil || len(result.LifecycleSuppressions) != 0 {
			continue
		}
		spans := append([]tracequery.TraceSpanSummary(nil), result.SpanWindows...)
		if result.WindowStats != nil {
			spans = append(spans, result.WindowStats.TraceSpans...)
		}
		for _, span := range spans {
			if span.Kind != "sync" || span.Thread.PID <= 0 || span.EndLine <= span.StartLine ||
				filepath.Clean(span.SourcePath) != filepath.Clean(result.SourcePath) {
				continue
			}
			start, end := span.StartTs, span.EndTs
			if span.ActualEndTs > span.ActualStartTs {
				start, end = span.ActualStartTs, span.ActualEndTs
			}
			if start < 0 || end <= start || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) {
				continue
			}
			candidate := types.TraceBusinessSpanCandidate{
				Path: result.SourcePath, TID: span.Thread.PID, Thread: span.Thread.Comm, Name: span.Name, Kind: span.Kind,
				StartLine: span.StartLine, EndLine: span.EndLine, StartTs: start, EndTs: end,
			}
			if !seen[candidate] {
				seen[candidate] = true
				out = append(out, candidate)
			}
		}
	}
	return out
}

// A reference always binds the whole tuple. Ordinary explicit queries remain
// available for clipping, dependency-thread drilldown and async observations.
func traceQueryApplyBusinessRef(ctx *types.BusContext, p traceQueryParams) (traceQueryParams, types.TraceBusinessSpanRef, *types.ToolResult) {
	var empty types.TraceBusinessSpanRef
	if strings.TrimSpace(p.BusinessSpanRef) == "" {
		return p, empty, nil
	}
	reject := func(reason string) (traceQueryParams, types.TraceBusinessSpanRef, *types.ToolResult) {
		r := traceQueryBusinessRefFailure(reason)
		return p, empty, &r
	}
	if ctx == nil || ctx.Mutable == nil || traceQuerySourceReadTerminal(ctx) {
		return reject("a current successful discovery in this run is required")
	}
	if p.Source != "" || p.Path != "" || p.PID.Int() != 0 || p.Thread != "" || p.TargetScope != "" ||
		p.TimeStart.Set() || p.TimeEnd.Set() || p.LineStart.Int() != 0 || p.LineEnd.Int() != 0 || p.SpanName != "" {
		return reject("do not combine the instance reference with source/path, thread/pid/target_scope, time/line bounds or span_name; use ordinary explicit parameters without the reference for a different or clipped scope")
	}
	switch tracequery.CanonicalViewName(p.View) {
	case "thread_timeline", "window_stats", "scheduler_latency_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls", "interaction_stats", "ipc_graph", "frame_root_cause_bundle", "trace_perf_bundle", "perf_stats", "perf_timeline", "evidence_pack":
	case "recipe":
		if traceQuerySpanLocateRecipe(p) {
			return reject("the instance is already located; select a follow-up measurement or causal view")
		}
	default:
		return reject("select a follow-up scheduler, IO, performance or causal view; use ordinary explicit parameters for discovery")
	}
	ref, ok := ctx.Mutable.ResolveTraceBusinessSpanRef(strings.TrimSpace(p.BusinessSpanRef))
	if !ok {
		return reject("the reference is unknown, stale or belongs to another capture/run; discover the instance again")
	}
	d := ref.Data()
	profile := traceSupplementRequestedArtifactScope(ctx)
	if profile.HasExplicitTimeWindows() && !profile.ContainsExplicitTimeWindow(d.StartTs, d.EndTs) {
		return reject("the complete instance extends outside the explicitly requested time window; preserve that requested window with ordinary explicit parameters")
	}
	p.Source, p.Path = "path", d.Path
	p.PID, p.Thread, p.TargetScope = FlexInt(d.TID), "", tracequery.TargetScopeThread
	p.TimeStart, p.TimeEnd = traceSecondFromAutoWindow(d.StartTs), traceSecondFromAutoWindow(d.EndTs)
	return p, ref, nil
}

func traceQueryBusinessRefFailure(reason string) types.ToolResult {
	return types.ToolResult{ToolName: "trace_query", Success: false, Timestamp: time.Now(),
		Summary: "trace_query business_span_ref: " + reason}
}

func traceQueryAppendBusinessRefs(result *types.ToolResult) {
	if result == nil || !result.Success || len(result.TraceBusinessSpanRefs) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString("\n## Exact synchronous business-instance query references\nChoose the task-relevant instance, not the first or longest. These returned pairs are not a complete inventory, a causal proof or an accepted completion focus. Follow up with {\"view\":\"window_stats\",\"business_span_ref\":\"<returned reference>\"} (or a scheduler/causal view); omit copied source, thread and window fields. Explicit requested windows still govern.\n")
	b.WriteString(traceQueryBusinessRefCompletionTeaching)
	b.WriteByte('\n')
	for _, ref := range result.TraceBusinessSpanRefs {
		d := ref.Data()
		fmt.Fprintf(&b, "- business_span_ref=%q work=%q thread=%q tid=%d source=%q lines=%d-%d complete_window=%.9f..%.9f seconds\n", ref.Token(), d.Name, d.Thread, d.TID, d.Path, d.StartLine, d.EndLine, d.StartTs, d.EndTs)
	}
	result.Summary += b.String()
}
