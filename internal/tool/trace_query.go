package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

type TraceQuery struct {
	ReadOnly
	EvidenceTool
}

type traceQueryParams struct {
	Source             string   `json:"source,omitempty"`
	Path               string   `json:"path,omitempty"`
	View               string   `json:"view,omitempty"`
	Thread             string   `json:"thread,omitempty"`
	PID                int      `json:"pid,omitempty"`
	TimeStart          float64  `json:"time_start,omitempty"`
	TimeEnd            float64  `json:"time_end,omitempty"`
	LineStart          int      `json:"line_start,omitempty"`
	LineEnd            int      `json:"line_end,omitempty"`
	EventTypes         []string `json:"event_types,omitempty"`
	MaxDepth           int      `json:"max_depth,omitempty"`
	MaxBranches        int      `json:"max_branches,omitempty"`
	MinDurationMs      float64  `json:"min_duration_ms,omitempty"`
	IncludeWindowStats *bool    `json:"include_window_stats,omitempty"`
	Limit              int      `json:"limit,omitempty"`
}

func (t *TraceQuery) Name() string { return "trace_query" }

func (t *TraceQuery) Description() string {
	return "Deterministically queries large runtime trace/log artifacts for scheduler timelines, wakeup chains, same-window resource stats, structured event search, and line-backed evidence packs. Trace timestamps are seconds (for example 928.081774 means seconds in the trace clock; only durations are rendered in ms). For HarmonyOS/hitrace user-space priority, larger numeric priority means higher priority: 1-40=CFS, 41-139=RT. Use this before ad-hoc grep/awk for ftrace/systrace/hitrace time-window causality questions; keep grep/read_file as fallback for unsupported formats."
}

func (t *TraceQuery) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "source": {"type":"string","enum":["path","attached_trace"],"description":"Use attached_trace for the current --htrace/--atrace blob; use path for an explicit workspace/repo file."},
    "path": {"type":"string","description":"Repo/workspace-relative or absolute trace/log path when source=path."},
    "view": {"type":"string","enum":["event_search","thread_timeline","window_stats","wakeup_chain","evidence_pack"],"description":"The deterministic trace view to compute."},
    "thread": {"type":"string","description":"Thread name or substring to resolve when pid is unknown."},
    "pid": {"type":"integer","description":"Thread pid to analyze when known."},
    "time_start": {"type":"number","description":"Trace timestamp window start in seconds, e.g. 928.081774 means seconds on the trace clock."},
    "time_end": {"type":"number","description":"Trace timestamp window end in seconds, e.g. 928.081774 means seconds on the trace clock."},
    "line_start": {"type":"integer","description":"Optional artifact line window start for bounded search."},
    "line_end": {"type":"integer","description":"Optional artifact line window end for bounded search."},
    "event_types": {"type":"array","items":{"type":"string"},"description":"Optional event filters such as sched_switch, sched_wakeup, cpu_idle, block_rq_issue, block_bio_remap, binder_transaction."},
    "max_depth": {"type":"integer","description":"wakeup_chain recursion limit; default 6."},
    "max_branches": {"type":"integer","description":"Maximum branches to report; default 8."},
    "min_duration_ms": {"type":"number","description":"Ignore intervals shorter than this; default 1ms."},
    "include_window_stats": {"type":"boolean","description":"For wakeup_chain, include same-window CPU/IO/binder/irq stats; default true."},
    "limit": {"type":"integer","description":"event_search inline row cap; default 40."}
  }
}`)
}

func (t *TraceQuery) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())
	var p traceQueryParams
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return failStrictDecodeWithError(t.Name(), time.Now(), err, nil)
	}
	path, sourceLabel, reject := resolveTraceQuerySource(ctx, p)
	if reject != nil {
		return *reject, nil
	}
	idx, err := tracequery.BuildIndex(contextFromBus(ctx), path)
	if err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("trace_query failed to parse %s: %v", path, err),
			Timestamp: time.Now(),
		}, nil
	}
	q := tracequery.Query{
		View:               p.View,
		Thread:             p.Thread,
		PID:                p.PID,
		TimeStart:          p.TimeStart,
		TimeEnd:            p.TimeEnd,
		LineStart:          p.LineStart,
		LineEnd:            p.LineEnd,
		EventTypes:         parseTraceQueryEventTypes(p.EventTypes),
		MaxDepth:           p.MaxDepth,
		MaxBranches:        p.MaxBranches,
		MinDurationMs:      p.MinDurationMs,
		Limit:              p.Limit,
		IncludeWindowStats: p.IncludeWindowStats != nil && *p.IncludeWindowStats,
	}
	if p.IncludeWindowStats == nil && strings.TrimSpace(p.View) == "wakeup_chain" {
		q.IncludeWindowStats = true
	}
	result := tracequery.Run(idx, q)
	payload, _ := json.MarshalIndent(result, "", "  ")
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	summary := traceQuerySummary(result, p, sourceLabel, payloadRef)
	preview, rawRef := StoreBlob(ctx, t.Name(), summary)
	if rawRef == "" {
		rawRef = payloadRef
	}
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   preview,
		RawRef:    rawRef,
		Timestamp: time.Now(),
	}, nil
}

func resolveTraceQuerySource(ctx *types.BusContext, p traceQueryParams) (string, string, *types.ToolResult) {
	source := strings.TrimSpace(p.Source)
	if source == "" {
		if strings.TrimSpace(p.Path) == "" {
			source = "attached_trace"
		} else {
			source = "path"
		}
	}
	if source == "attached_trace" {
		if ctx != nil && strings.TrimSpace(ctx.WorkDir) != "" {
			blob := filepath.Join(ctx.WorkDir, promptctx.AttachedTraceBlobName)
			if _, err := os.Stat(blob); err == nil {
				return blob, "attached_trace", nil
			}
		}
		if ctx != nil && strings.TrimSpace(ctx.AttachedHitrace) != "" {
			ref := StoreBlobArtifact(ctx.WorkDir, "trace_query", promptctx.AttachedTraceBlobName, ctx.AttachedHitrace)
			if strings.TrimSpace(ref) != "" {
				return ref, "attached_trace", nil
			}
		}
		return "", source, &types.ToolResult{
			ToolName:  "trace_query",
			Success:   false,
			Summary:   "trace_query requires an attached trace blob, but none is available. Use source=\"path\" with an explicit trace file, or attach one via --htrace/--atrace.",
			Timestamp: time.Now(),
		}
	}
	if strings.TrimSpace(p.Path) == "" {
		return "", source, &types.ToolResult{
			ToolName:  "trace_query",
			Success:   false,
			Summary:   "trace_query source=path requires a non-empty path",
			Timestamp: time.Now(),
		}
	}
	return resolveToolPath(ctx, p.Path), "path", nil
}

func parseTraceQueryEventTypes(raw []string) []tracequery.EventType {
	out := make([]tracequery.EventType, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, tracequery.EventType(item))
	}
	return out
}

func traceQuerySummary(result tracequery.Result, p traceQueryParams, sourceLabel, payloadRef string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[trace_query params: view=%s source=%s path=%s origin=runtime_artifact artifact_id=%s artifact_kind=trace line_start=%s line_end=%s time_start=%s time_end=%s payload_ref=%s]\n",
		firstNonEmptyTraceString(result.View, p.View, "event_search"),
		sourceLabel,
		sanitizeForBanner(result.SourcePath),
		traceQueryArtifactID(sourceLabel),
		positiveIntBannerValue(p.LineStart),
		positiveIntBannerValue(p.LineEnd),
		floatBannerValue(p.TimeStart),
		floatBannerValue(p.TimeEnd),
		sanitizeForBanner(payloadRef),
	)
	fmt.Fprintf(&b, "# Trace Query: %s\n\n", result.View)
	fmt.Fprintf(&b, "source=%s lines=%d parsed_events=%d timestamp_unit=%s selected_window=%.6f..%.6f seconds\n", result.SourcePath, result.LineCount, result.EventCount, firstNonEmptyTraceString(result.TimeUnit, "seconds"), result.TimeStart, result.TimeEnd)
	if result.PrioritySemantics != "" {
		fmt.Fprintf(&b, "priority_semantics=%s\n", result.PrioritySemantics)
	}
	b.WriteString("\n")
	if payloadRef != "" {
		fmt.Fprintf(&b, "payload_ref=%s\n\n", payloadRef)
	}
	if result.WakeupChain != nil {
		b.WriteString("## Wakeup chain\n")
		for _, edge := range result.WakeupChain.Edges {
			fmt.Fprintf(&b, "- %s -> %s at %.6f line %d (latency %.3fms)\n",
				traceThreadLabel(edge.Waker), traceThreadLabel(edge.Wakee), edge.WakeupTs, edge.WakeupLine, edge.LatencyMs)
		}
		for _, root := range result.WakeupChain.RootEvidence {
			fmt.Fprintf(&b, "- root_evidence=%s thread=%s duration=%.3fms lines=%d-%d confidence=%.2f — %s\n",
				root.Type, traceThreadLabel(root.Thread), root.DurationMs, root.LineStart, root.LineEnd, root.Confidence, root.Summary)
		}
		b.WriteString("\n")
	}
	if result.Timeline != nil {
		b.WriteString("## Thread timeline\n")
		for i, it := range result.Timeline.Intervals {
			if i >= 12 {
				fmt.Fprintf(&b, "... omitted %d interval(s); see payload_ref\n", len(result.Timeline.Intervals)-i)
				break
			}
			fmt.Fprintf(&b, "- %s %.6f..%.6f %.3fms lines=%d-%d wake_line=%d\n",
				it.State, it.StartTs, it.EndTs, it.DurationMs, it.StartLine, it.EndLine, it.WakeupLine)
		}
		b.WriteString("\n")
	}
	if result.WindowStats != nil {
		b.WriteString("## Window stats\n")
		for _, cpu := range result.WindowStats.CPU {
			fmt.Fprintf(&b, "- cpu=%d busy=%.3fms idle=%.3fms freq=%d\n", cpu.CPU, cpu.BusyMs, cpu.IdleMs, cpu.Frequency)
		}
		for _, td := range result.WindowStats.TopRunning {
			fmt.Fprintf(&b, "- top_running %s %.3fms %s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), td.LineStart, td.LineEnd)
		}
		for _, td := range result.WindowStats.RunnableTop {
			fmt.Fprintf(&b, "- top_runnable %s %.3fms %s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), td.LineStart, td.LineEnd)
		}
		for _, td := range result.WindowStats.DStateTop {
			fmt.Fprintf(&b, "- top_d_state %s %.3fms %s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), td.LineStart, td.LineEnd)
		}
		fmt.Fprintf(&b, "- counts block_issue=%d block_remap=%d block_complete=%d binder=%d irq=%d memory=%d\n\n",
			result.WindowStats.BlockIssueCount, result.WindowStats.BlockRemapCount, result.WindowStats.BlockCompleteCount, result.WindowStats.BinderCount, result.WindowStats.IRQCount, result.WindowStats.MemoryEventCount)
	}
	if len(result.Events) > 0 {
		b.WriteString("## Events\n")
		for _, ev := range result.Events {
			fmt.Fprintf(&b, "- line=%d ts=%.6f type=%s thread=%s raw=%s\n", ev.Line, ev.Ts, ev.Type, traceThreadLabel(tracequery.ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}), strings.TrimSpace(ev.Raw))
		}
		b.WriteString("\n")
	}
	if len(result.EvidencePack) > 0 {
		b.WriteString("## Evidence pack\n")
		for i, fact := range result.EvidencePack {
			if i >= 16 {
				fmt.Fprintf(&b, "... omitted %d fact(s); see payload_ref\n", len(result.EvidencePack)-i)
				break
			}
			fmt.Fprintf(&b, "- %s %s %s lines=%d-%d confidence=%.2f — %s\n",
				fact.Subject, fact.Predicate, fact.Object, fact.LineStart, fact.LineEnd, fact.Confidence, fact.Summary)
		}
	}
	for _, caveat := range result.Caveats {
		fmt.Fprintf(&b, "caveat=%s\n", caveat)
	}
	return b.String()
}

func traceQueryArtifactID(sourceLabel string) string {
	if strings.TrimSpace(sourceLabel) == "attached_trace" {
		return "attached_trace"
	}
	return "trace_query"
}

func tracePriorityDetail(td tracequery.ThreadDuration) string {
	if td.Priority <= 0 {
		return ""
	}
	if strings.TrimSpace(td.PriorityClass) == "" {
		return fmt.Sprintf("prio=%d", td.Priority)
	}
	return fmt.Sprintf("prio=%d/%s", td.Priority, td.PriorityClass)
}

func contextFromBus(ctx *types.BusContext) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx.Context()
}

func ctxWorkDir(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	return ctx.WorkDir
}

func firstNonEmptyTraceString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func floatBannerValue(v float64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%.6f", v)
}

func traceThreadLabel(t tracequery.ThreadRef) string {
	switch {
	case t.Comm != "" && t.PID > 0:
		return fmt.Sprintf("%s-%d", t.Comm, t.PID)
	case t.Comm != "":
		return t.Comm
	case t.PID > 0:
		return fmt.Sprintf("pid=%d", t.PID)
	default:
		return "unknown-thread"
	}
}
