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
	Source               string          `json:"source,omitempty"`
	Path                 string          `json:"path,omitempty"`
	View                 string          `json:"view,omitempty"`
	Thread               string          `json:"thread,omitempty"`
	PID                  FlexInt         `json:"pid,omitempty"`
	TimeStart            TraceSecond     `json:"time_start,omitempty"`
	TimeEnd              TraceSecond     `json:"time_end,omitempty"`
	LineStart            FlexInt         `json:"line_start,omitempty"`
	LineEnd              FlexInt         `json:"line_end,omitempty"`
	EventTypes           TraceEventTypes `json:"event_types,omitempty"`
	SpanName             string          `json:"span_name,omitempty"`
	InteractionDirection string          `json:"interaction_direction,omitempty"`
	MaxDepth             FlexInt         `json:"max_depth,omitempty"`
	MaxBranches          FlexInt         `json:"max_branches,omitempty"`
	MinDurationMs        FlexFloat       `json:"min_duration_ms,omitempty"`
	IncludeWindowStats   *FlexBool       `json:"include_window_stats,omitempty"`
	Limit                FlexInt         `json:"limit,omitempty"`
	TraceFlavor          string          `json:"trace_flavor,omitempty"`
	Platform             string          `json:"platform,omitempty"`
}

func (t *TraceQuery) Name() string { return "trace_query" }

func (t *TraceQuery) Description() string {
	return "Deterministically queries large runtime trace/log artifacts for scheduler timelines, trace span windows, ranked root causes, wakeup chains, binder IPC graphs, interaction Top-N, same-window resource stats, structured event search, and line-backed evidence packs. Trace timestamps are seconds end-to-end: 928.081774 means 928 seconds + 0.081774 seconds; with six fractional digits, the fractional part is microsecond-precision (81774 us), not a separate millisecond field. Only derived durations are rendered in ms. Trace flavor is auto-detected as harmony_hitrace, android_atrace, or generic_ftrace; pass trace_flavor/platform when the user names a producer. Explicit user intent such as Harmony/鸿蒙/东湖/OHOS or Android/安卓 wins for the current call and is not auto-corrected, though content signals remain in caveats for audit. For HarmonyOS/hitrace user-space priority, larger numeric priority means higher priority: 1-40=CFS, 41-139=RT. Android/generic ftrace keeps raw scheduler priority and does not apply Harmony ranges. Thread selectors accept pid plus common ftrace/hitrace labels such as com.tencent.mm-36379, com.tencent.mm 36379, com.tencent.mm [36379], [GT]ColdPool#5-36624, binder:486_1-10803, or pid=36379; pass pid directly when known. Use this before ad-hoc grep/awk for ftrace/systrace/hitrace time-window causality questions; keep grep/read_file as fallback for unsupported formats."
}

func (t *TraceQuery) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
	    "source": {"type":"string","enum":["path","attached_trace"],"description":"Use attached_trace for the current --htrace/--atrace blob; use path for an explicit workspace/repo file."},
	    "path": {"type":"string","description":"Repo/workspace-relative or absolute trace/log path when source=path."},
	    "trace_flavor": {"type":"string","enum":["auto","harmony_hitrace","android_atrace","generic_ftrace"],"description":"Optional producer/platform flavor. Defaults to auto detection. Use harmony_hitrace for HarmonyOS HiTrace priority semantics, android_atrace for Android/Linux atrace raw scheduler priorities, and generic_ftrace when uncertain."},
	    "platform": {"type":"string","description":"Alias for trace_flavor; accepted for model compatibility. If the user explicitly says Harmony/鸿蒙/东湖/OHOS, pass harmony_hitrace; for Android/安卓/atrace, pass android_atrace."},
	    "view": {"type":"string","enum":["event_search","span_window","thread_timeline","window_stats","ipc_graph","wakeup_chain","root_cause_rank","interaction_stats","evidence_pack"],"description":"The deterministic trace view to compute. Use span_window to turn a unique B/E trace span into a time window, root_cause_rank for primary/secondary/tertiary cause candidates, interaction_stats for target-thread wakeup/binder interaction Top-N, and ipc_graph for binder transaction send/receive causality."},
	    "thread": {"type":"string","description":"Thread name, substring, or ftrace/hitrace task label to resolve when pid is unknown. Accepts forms like \"com.tencent.mm-36379\", \"com.tencent.mm 36379\", \"com.tencent.mm [36379]\", \"[GT]ColdPool#5-36624\", \"binder:486_1-10803\", or \"pid=36379\"; pid is preferred when known."},
    "pid": {"type":"integer","description":"Thread pid to analyze when known."},
    "time_start": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window start in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\" or \"928.081774 秒\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "time_end": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window end in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\" or \"928.081774 秒\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "line_start": {"type":"integer","description":"Optional artifact line window start for bounded search."},
    "line_end": {"type":"integer","description":"Optional artifact line window end for bounded search."},
    "event_types": {"type":"array","items":{"type":"string"},"description":"Optional event filters such as sched_switch, sched_wakeup, sched_blocked_reason, cpu_idle, cpu_frequency, clock_set_rate, block_rq_issue, block_bio_remap, binder_transaction, binder_transaction_received."},
    "span_name": {"type":"string","description":"Optional trace B/E span name substring. For span_window, returns matching span windows. For wakeup_chain/root_cause_rank/evidence_pack without explicit time_start/time_end, a unique matching span derives the selected window."},
    "interaction_direction": {"type":"string","enum":["both","incoming","outgoing"],"description":"For interaction_stats: both is default; incoming counts peers waking/calling the target, outgoing counts target waking/calling peers."},
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
	timeStart, timeEnd, timeCaveat := normalizedTraceQueryWindow(p)
	q := tracequery.Query{
		View:                 p.View,
		Thread:               p.Thread,
		ThreadInput:          p.Thread,
		PID:                  p.PID.Int(),
		TimeStart:            timeStart,
		TimeEnd:              timeEnd,
		LineStart:            p.LineStart.Int(),
		LineEnd:              p.LineEnd.Int(),
		EventTypes:           parseTraceQueryEventTypes(p.EventTypes.Strings()),
		SpanName:             p.SpanName,
		InteractionDirection: p.InteractionDirection,
		MaxDepth:             p.MaxDepth.Int(),
		MaxBranches:          p.MaxBranches.Int(),
		MinDurationMs:        p.MinDurationMs.Float64(),
		Limit:                p.Limit.Int(),
		IncludeWindowStats:   p.IncludeWindowStats != nil && p.IncludeWindowStats.Bool(),
	}
	q.TraceFlavorHint, q.TraceFlavorHintSource = traceFlavorHintForQuery(ctx, p, sourceLabel)
	if p.IncludeWindowStats == nil && strings.TrimSpace(p.View) == "wakeup_chain" {
		q.IncludeWindowStats = true
	}
	result := tracequery.Run(idx, q)
	if timeCaveat != "" {
		result.Caveats = append(result.Caveats, timeCaveat)
	}
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

func traceFlavorHintForQuery(ctx *types.BusContext, p traceQueryParams, sourceLabel string) (tracequery.TraceFlavor, string) {
	if flavor := tracequery.NormalizeTraceFlavor(firstNonEmptyTraceString(p.TraceFlavor, p.Platform)); flavor != "" && flavor != tracequery.TraceFlavorAuto {
		return flavor, "tool_param"
	}
	if strings.TrimSpace(sourceLabel) == "attached_trace" && ctx != nil {
		if flavor := tracequery.NormalizeTraceFlavor(ctx.AttachedHitraceSource); flavor != "" && flavor != tracequery.TraceFlavorAuto {
			return flavor, "attached_source"
		}
	}
	return tracequery.TraceFlavorAuto, ""
}

func normalizedTraceQueryWindow(p traceQueryParams) (float64, float64, string) {
	start := p.TimeStart.Seconds()
	end := p.TimeEnd.Seconds()
	startTol := p.TimeStart.QueryToleranceSeconds()
	endTol := p.TimeEnd.QueryToleranceSeconds()
	if p.TimeStart.Set() && startTol > 0 {
		start -= startTol
		if start < 0 {
			start = 0
		}
	}
	if p.TimeEnd.Set() && endTol > 0 {
		end += endTol
	}
	if startTol == 0 && endTol == 0 && !traceSecondNeedsNormalizationNote(p.TimeStart) && !traceSecondNeedsNormalizationNote(p.TimeEnd) {
		return start, end, ""
	}
	var parts []string
	if p.TimeStart.Set() {
		parts = append(parts, fmt.Sprintf("time_start=%s normalized=%.6f", sanitizeForBanner(p.TimeStart.Raw()), p.TimeStart.Seconds()))
	}
	if p.TimeEnd.Set() {
		parts = append(parts, fmt.Sprintf("time_end=%s normalized=%.6f", sanitizeForBanner(p.TimeEnd.Raw()), p.TimeEnd.Seconds()))
	}
	if startTol > 0 || endTol > 0 {
		parts = append(parts, fmt.Sprintf("query_tolerance_seconds=start±%.9f/end±%.9f", startTol, endTol))
	}
	return start, end, "trace timestamp strings were normalized to seconds; shortened fractional timestamps get a tiny bounded tolerance: " + strings.Join(parts, ", ")
}

func traceSecondNeedsNormalizationNote(v TraceSecond) bool {
	if !v.Set() {
		return false
	}
	raw := strings.TrimSpace(v.Raw())
	if raw == "" {
		return false
	}
	if strings.ContainsAny(raw, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ秒微毫µ") {
		return true
	}
	return false
}

func traceQuerySummary(result tracequery.Result, p traceQueryParams, sourceLabel, payloadRef string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[trace_query params: view=%s source=%s path=%s origin=runtime_artifact artifact_id=%s artifact_kind=trace thread=%s pid=%s line_start=%s line_end=%s time_start=%s time_end=%s span_name=%s interaction_direction=%s trace_flavor=%s trace_flavor_confidence=%.2f payload_ref=%s]\n",
		firstNonEmptyTraceString(result.View, p.View, "event_search"),
		sourceLabel,
		sanitizeForBanner(result.SourcePath),
		traceQueryArtifactID(sourceLabel),
		sanitizeForBanner(p.Thread),
		positiveIntBannerValue(p.PID.Int()),
		positiveIntBannerValue(p.LineStart.Int()),
		positiveIntBannerValue(p.LineEnd.Int()),
		traceSecondBannerValue(p.TimeStart),
		traceSecondBannerValue(p.TimeEnd),
		sanitizeForBanner(p.SpanName),
		sanitizeForBanner(p.InteractionDirection),
		sanitizeForBanner(result.TraceFlavor),
		result.FlavorConfidence,
		sanitizeForBanner(payloadRef),
	)
	fmt.Fprintf(&b, "# Trace Query: %s\n\n", result.View)
	fmt.Fprintf(&b, "source=%s lines=%d parsed_events=%d timestamp_unit=%s selected_window=%.6f..%.6f seconds\n", result.SourcePath, result.LineCount, result.EventCount, firstNonEmptyTraceString(result.TimeUnit, "seconds"), result.TimeStart, result.TimeEnd)
	if result.TraceFlavor != "" {
		fmt.Fprintf(&b, "trace_flavor=%s confidence=%.2f\n", result.TraceFlavor, result.FlavorConfidence)
	}
	if len(result.FlavorSignals) > 0 {
		fmt.Fprintf(&b, "trace_flavor_signals=%s\n", sanitizeForBanner(strings.Join(result.FlavorSignals, ",")))
	}
	if result.View == "event_search" {
		fmt.Fprintf(&b, "matched_events=%d\n", len(result.Events))
	}
	if result.PrioritySemantics != "" {
		fmt.Fprintf(&b, "priority_semantics=%s\n", result.PrioritySemantics)
	}
	b.WriteString("\n")
	if payloadRef != "" {
		fmt.Fprintf(&b, "payload_ref=%s\n\n", payloadRef)
	}
	if len(result.SpanWindows) > 0 {
		b.WriteString("## Span windows\n")
		for _, span := range result.SpanWindows {
			fmt.Fprintf(&b, "- span %s %q %.6f..%.6f duration=%.3fms lines=%d-%d\n",
				traceThreadLabel(span.Thread), span.Name, span.StartTs, span.EndTs, span.DurationMs, span.StartLine, span.EndLine)
		}
		b.WriteString("\n")
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
		for _, wait := range result.WakeupChain.BinderWaits {
			fmt.Fprintf(&b, "- binder_wait transaction=%d %s -> %s duration=%.3fms send_line=%d receive_line=%d sleep_line=%d wake_line=%d confidence=%.2f — %s\n",
				wait.TransactionID, traceThreadLabel(wait.Thread), traceThreadLabel(wait.Peer), wait.DurationMs, wait.SendLine, wait.ReceiveLine, wait.SleepLine, wait.WakeupLine, wait.Confidence, wait.Summary)
		}
		writeTraceIPCEdges(&b, result.WakeupChain.IPCEdges)
		b.WriteString("\n")
	}
	if result.RootCauseRank != nil {
		b.WriteString("## Root cause rank\n")
		for _, item := range result.RootCauseRank.Items {
			fmt.Fprintf(&b, "- rank=%d tier=%s type=%s thread=%s impact=%.3fms score=%.3f confidence=%.2f lines=%d-%d source=%s — %s\n",
				item.Rank, item.Tier, item.Type, traceThreadLabel(item.Thread), item.ImpactMs, item.Score, item.Confidence, item.LineStart, item.LineEnd, item.Source, item.Summary)
		}
		for _, caveat := range result.RootCauseRank.Caveats {
			fmt.Fprintf(&b, "- root_cause_caveat=%s\n", caveat)
		}
		b.WriteString("\n")
	}
	if result.InteractionStats != nil {
		b.WriteString("## Interaction stats\n")
		for _, item := range result.InteractionStats.Items {
			fmt.Fprintf(&b, "- peer=%s total=%d wake_to_target=%d wake_from_target=%d binder_to_target=%d binder_from_target=%d lines=%d-%d window=%.6f..%.6f — %s\n",
				traceThreadLabel(item.Peer), item.TotalInteractions, item.WakeupsToTarget, item.WakeupsFromTarget, item.BinderToTarget, item.BinderFromTarget, item.FirstLine, item.LastLine, item.FirstTs, item.LastTs, item.Summary)
		}
		for _, caveat := range result.InteractionStats.Caveats {
			fmt.Fprintf(&b, "- interaction_caveat=%s\n", caveat)
		}
		b.WriteString("\n")
	}
	if result.IPCGraph != nil {
		b.WriteString("## IPC graph\n")
		writeTraceIPCEdges(&b, result.IPCGraph.Edges)
		for _, caveat := range result.IPCGraph.Caveats {
			fmt.Fprintf(&b, "- ipc_caveat=%s\n", caveat)
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
			fmt.Fprintf(&b, "- cpu=%d busy=%.3fms idle=%.3fms freq=%d%s\n", cpu.CPU, cpu.BusyMs, cpu.IdleMs, cpu.Frequency, traceFrequencyResidencySummary(cpu.FrequencyResidency))
		}
		for _, td := range result.WindowStats.TopRunning {
			fmt.Fprintf(&b, "- top_running %s %.3fms %s%s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), traceThreadDurationLocation(td), td.LineStart, td.LineEnd)
		}
		for _, td := range result.WindowStats.RunnableTop {
			fmt.Fprintf(&b, "- top_runnable %s %.3fms %s%s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), traceThreadDurationLocation(td), td.LineStart, td.LineEnd)
		}
		for _, td := range result.WindowStats.DStateTop {
			fmt.Fprintf(&b, "- top_d_state %s %.3fms %s%s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), traceThreadDurationLocation(td), td.LineStart, td.LineEnd)
		}
		for _, br := range result.WindowStats.BlockedReasons {
			fmt.Fprintf(&b, "- blocked_reason %s iowait=%d count=%d line=%d caller=%s\n", traceThreadLabel(br.Thread), br.IOWait, br.Count, br.Line, br.Reason)
		}
		for _, io := range result.WindowStats.IOLatencies {
			fmt.Fprintf(&b, "- io_latency dev=%s op=%s sector=%d len=%d duration=%.3fms issue=%s complete=%s lines=%d-%d\n",
				io.Dev, io.Op, io.Sector, io.Len, io.DurationMs, traceThreadLabel(io.IssueThread), traceThreadLabel(io.CompleteThread), io.IssueLine, io.CompleteLine)
		}
		for _, pressure := range result.WindowStats.CPUPressure {
			fmt.Fprintf(&b, "- cpu_pressure cpu=%d runnable_wait=%.3fms running=%.3fms high_prio_running=%.3fms runnable_events=%d\n",
				pressure.CPU, pressure.RunnableWaitMs, pressure.RunningMs, pressure.HighPriorityRunningMs, pressure.RunnableEvents)
		}
		for _, span := range result.WindowStats.TraceSpans {
			fmt.Fprintf(&b, "- trace_span %s %q duration=%.3fms lines=%d-%d\n",
				traceThreadLabel(span.Thread), span.Name, span.DurationMs, span.StartLine, span.EndLine)
		}
		for _, counter := range result.WindowStats.TraceCounters {
			fmt.Fprintf(&b, "- trace_counter %s %q value=%s count=%d line=%d\n",
				traceThreadLabel(counter.Thread), counter.Name, counter.Value, counter.Count, counter.Line)
		}
		for _, burst := range result.WindowStats.IRQBursts {
			fmt.Fprintf(&b, "- irq_burst cpu=%d irq=%d name=%s count=%d duration=%.3fms lines=%d-%d\n",
				burst.CPU, burst.IRQ, burst.Name, burst.Count, burst.DurationMs, burst.LineStart, burst.LineEnd)
		}
		for _, mem := range result.WindowStats.MemoryKinds {
			fmt.Fprintf(&b, "- memory_kind kind=%s count=%d line=%d\n", mem.Kind, mem.Count, mem.Line)
		}
		for _, drift := range result.WindowStats.ThreadDrifts {
			fmt.Fprintf(&b, "- thread_identity_caveat pid=%d names=%s tgids=%v lines=%d-%d\n",
				drift.PID, strings.Join(drift.Names, ","), drift.TGIDs, drift.LineStart, drift.LineEnd)
		}
		fmt.Fprintf(&b, "- counts block_issue=%d block_remap=%d block_complete=%d binder=%d binder_received=%d irq=%d memory=%d blocked_reason=%d iowait_blocked=%d\n\n",
			result.WindowStats.BlockIssueCount, result.WindowStats.BlockRemapCount, result.WindowStats.BlockCompleteCount, result.WindowStats.BinderCount, result.WindowStats.BinderReceivedCount, result.WindowStats.IRQCount, result.WindowStats.MemoryEventCount, result.WindowStats.BlockedReasonCount, result.WindowStats.IOWaitBlockedCount)
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

func writeTraceIPCEdges(b *strings.Builder, edges []tracequery.IPCEdge) {
	for i, edge := range edges {
		if i >= 12 {
			fmt.Fprintf(b, "... omitted %d IPC edge(s); see payload_ref\n", len(edges)-i)
			break
		}
		fmt.Fprintf(b, "- ipc transaction=%d %s -> %s send_line=%d receive_line=%d latency=%.3fms reply=%d flags=%s code=%s confidence=%.2f\n",
			edge.TransactionID,
			traceThreadLabel(edge.Sender),
			traceThreadLabel(edge.Receiver),
			edge.SendLine,
			edge.ReceiveLine,
			edge.LatencyMs,
			edge.Reply,
			edge.Flags,
			edge.Code,
			edge.Confidence,
		)
		for _, caveat := range edge.Caveats {
			fmt.Fprintf(b, "  caveat=%s\n", caveat)
		}
	}
}

func traceFrequencyResidencySummary(items []tracequery.CPUFrequencyResidency) string {
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for i, item := range items {
		if i >= 4 {
			parts = append(parts, fmt.Sprintf("+%d", len(items)-i))
			break
		}
		parts = append(parts, fmt.Sprintf("%dkHz/%.3fms", item.Frequency, item.DurationMs))
	}
	return " freq_residency=" + strings.Join(parts, ",")
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

func traceThreadDurationLocation(td tracequery.ThreadDuration) string {
	parts := []string{fmt.Sprintf("cpu=%d", td.CPU)}
	if td.Frequency > 0 {
		parts = append(parts, fmt.Sprintf("freq=%dkHz", td.Frequency))
	}
	return " " + strings.Join(parts, " ")
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

func traceSecondBannerValue(v TraceSecond) string {
	if !v.Set() || v.Seconds() == 0 {
		return ""
	}
	return fmt.Sprintf("%.6f", v.Seconds())
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
