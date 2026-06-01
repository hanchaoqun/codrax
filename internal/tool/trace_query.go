package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	Pattern              string          `json:"pattern,omitempty"`
	SpanName             string          `json:"span_name,omitempty"`
	InteractionDirection string          `json:"interaction_direction,omitempty"`
	RecipeName           string          `json:"recipe_name,omitempty"`
	MaxDepth             FlexInt         `json:"max_depth,omitempty"`
	MaxBranches          FlexInt         `json:"max_branches,omitempty"`
	MinDurationMs        FlexFloat       `json:"min_duration_ms,omitempty"`
	IncludeWindowStats   *FlexBool       `json:"include_window_stats,omitempty"`
	Limit                FlexInt         `json:"limit,omitempty"`
	CoreTopology         string          `json:"core_topology,omitempty"`
	TraceFlavor          string          `json:"trace_flavor,omitempty"`
	Platform             string          `json:"platform,omitempty"`
}

var (
	traceQueryLargeRecipeDiscoveryMinBytes int64 = 128 << 20
	traceQueryObjectiveKVTokenRE                 = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_./:-]*=[^\s,，。；;"'）)]+`)
	traceQueryTimestampRE                        = regexp.MustCompile(`\s([0-9]+(?:\.[0-9]+)?):\s+`)
)

func (t *TraceQuery) Name() string { return "trace_query" }

func (t *TraceQuery) Description() string {
	return "Deterministically queries large runtime trace/log artifacts for scheduler timelines, scheduler latency stats, trace span/frame windows, frame timelines/flows, render pipelines, ranked root causes, wakeup chains, binder IPC graphs, critical blocking calls, interaction Top-N, same-window resource stats, recipes, structured event search, and line-backed evidence packs. window_stats/event_search can filter or summarize scheduler, binder transaction/received/lock/alloc/reply rows, CPU idle/frequency/frequency-limit, block IO, IRQ/softirq, storage, filesystem, power, Ability/XPower/HiSystemEvent resource observations, workqueue, DMA fence, memory-like events, and SmartPerf-style eBPF BIO/FileSystem/PageFault resource rows when converted to text key/value fields. For frame/span discovery, use view=event_search with pattern as a case-insensitive literal substring, not a regex; it is best for frame ids, jank ids, span labels, trace marker labels, or one exact timestamp/event token before broad grep. If multiple span windows or zero rows come back, narrow with the returned line/time windows, a shorter literal pattern, event_types=[\"trace_mark\"], pid/thread, or span_window before running recipe/root-cause views. For big/middle/small core analysis, pass core_topology like \"small=0-3,middle=4-7,big=8-11\"; if omitted the tool only infers classes from observed CPU frequencies and reports that caveat. For very large traces, an unbounded jank recipe without time_start/time_end, line_start/line_end, span_name, pid, or thread first returns bounded marker discovery and next-call hints instead of expanding expensive full-trace root-cause/resource views; rerun with the selected frame/span time or line window. Trace timestamps are seconds end-to-end: 928.081774 means 928 seconds + 0.081774 seconds; with six fractional digits, the fractional part is microsecond-precision (81774 us), not a separate millisecond field. Only derived durations are rendered in ms. Trace flavor is auto-detected as harmony_hitrace, android_atrace, or generic_ftrace; pass trace_flavor/platform when the user names a producer. Explicit user intent such as Harmony/鸿蒙/东湖/OHOS or Android/安卓 wins for the current call and is not auto-corrected, though content signals remain in caveats for audit. Auto detection may report platform_candidate=mixed_harmony_base when Harmony-base trace signals coexist with Android-framework process surfaces; this uses Donghu/Harmony scheduler priority semantics, not Android priority semantics. Donghu/东湖 uses Harmony/OpenHarmony trace scheduler semantics with process-isolated Android-framework and Harmony-framework surfaces; priority and timestamp semantics still follow Harmony. For HarmonyOS/hitrace user-space priority, larger numeric priority means higher priority: 1-40=CFS, 41-139=RT. Android/generic ftrace keeps raw scheduler priority and does not apply Harmony ranges. Thread selectors accept pid plus common ftrace/hitrace labels such as com.tencent.mm-36379, com.tencent.mm 36379, com.tencent.mm [36379], [GT]ColdPool#5-36624, binder:486_1-10803, or pid=36379; pass pid directly when known. Use this before ad-hoc grep/awk for ftrace/systrace/hitrace time-window causality questions; keep grep/read_file as fallback for unsupported formats."
}

func (t *TraceQuery) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
	    "source": {"type":"string","enum":["path","attached_trace"],"description":"Use attached_trace for the current --htrace/--atrace blob; use path for an explicit workspace/repo file."},
	    "path": {"type":"string","description":"Repo/workspace-relative or absolute trace/log path when source=path."},
	    "trace_flavor": {"type":"string","enum":["auto","harmony_hitrace","android_atrace","generic_ftrace"],"description":"Optional producer/platform flavor. Defaults to auto detection. Use harmony_hitrace for HarmonyOS HiTrace priority semantics, android_atrace for Android/Linux atrace raw scheduler priorities, and generic_ftrace when uncertain."},
	    "platform": {"type":"string","enum":["auto","donghu","harmony","harmony_hitrace","android","android_atrace","generic","generic_ftrace"],"description":"Optional platform hint. Use donghu when the user says 东湖: scheduler/time/priority semantics follow Harmony/OpenHarmony, while Android-framework and Harmony-framework processes may coexist at process boundaries. harmony/harmony_hitrace selects Harmony semantics; android/android_atrace selects Android raw scheduler priority semantics."},
	    "view": {"type":"string","enum":["event_search","span_window","frame_window","render_pipeline","frame_timeline","frame_flow","thread_timeline","window_stats","scheduler_latency_stats","ipc_graph","wakeup_chain","root_cause_rank","critical_blocking_calls","interaction_stats","recipe","evidence_pack"],"description":"The deterministic trace view to compute. Use span_window to turn a unique B/E trace span into a time window; frame_window/render_pipeline for Choreographer/RenderFrame/VSYNC/draw/present spans; frame_timeline/frame_flow for Expected/Actual/Jank/GPU/RS/UI phase summaries and cross-thread frame flows; scheduler_latency_stats for runnable wait p95/p99/max and CPU competition; critical_blocking_calls for futex/lock/sync/binder/IO/D-state candidates; root_cause_rank for primary/secondary/tertiary cause candidates; interaction_stats for target-thread wakeup/binder interaction Top-N; recipe for standard evidence packs; and ipc_graph for binder transaction send/receive causality."},
	    "thread": {"type":"string","description":"Thread name, substring, or ftrace/hitrace task label to resolve when pid is unknown. Accepts forms like \"com.tencent.mm-36379\", \"com.tencent.mm 36379\", \"com.tencent.mm [36379]\", \"[GT]ColdPool#5-36624\", \"binder:486_1-10803\", or \"pid=36379\"; pid is preferred when known."},
    "pid": {"type":"integer","description":"Thread pid to analyze when known."},
    "time_start": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window start in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\" or \"928.081774 秒\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "time_end": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window end in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\" or \"928.081774 秒\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "line_start": {"type":"integer","description":"Optional artifact line window start for bounded search."},
    "line_end": {"type":"integer","description":"Optional artifact line window end for bounded search."},
	    "event_types": {"type":"array","items":{"type":"string"},"x-codrax-split-string-array":true,"description":"Optional event filters such as sched_switch, sched_wakeup, sched_blocked_reason, cpu_idle, cpu_frequency, cpu_frequency_limits, clock_set_rate, block_rq_issue, block_bio_remap, binder_transaction, binder_transaction_received, binder_transaction_alloc_buf, binder_lock, binder_locked, binder_unlock, binder_reply, irq, softirq, storage, filesystem, power, ability_monitor, xpower, hi_sysevent, workqueue, dma_fence. The JSON repair layer also accepts a comma/semicolon separated string for this field."},
    "pattern": {"type":"string","description":"For event_search, optional case-insensitive literal substring matched against parsed event text, span names, thread labels, scheduler roles, resource fields, and raw-like field text. Use this for frame ids such as \"1917295\", jank ids such as \"jank_frames=7\", exact timestamps, or trace labels such as \"Choreographer#doFrame\"; it is not a regex. Start with one exact token, then add event_types/time/line/thread filters after the first hit."},
    "span_name": {"type":"string","description":"Optional trace B/E span name substring. For span_window, returns matching span windows. For wakeup_chain/root_cause_rank/evidence_pack without explicit time_start/time_end, a unique matching span derives the selected window."},
    "interaction_direction": {"type":"string","enum":["both","incoming","outgoing"],"description":"For interaction_stats: both is default; incoming counts peers waking/calling the target, outgoing counts target waking/calling peers."},
    "recipe_name": {"type":"string","enum":["auto","sleep_root_cause","jank","runnable_delay","binder_wait","io_wait","cpu_supply"],"description":"For view=recipe: choose a standard deterministic evidence pack. auto picks from span_name/event_types/question-shape hints; recipes remain advisory and line-backed."},
    "max_depth": {"type":"integer","description":"wakeup_chain recursion limit; default 6."},
    "max_branches": {"type":"integer","description":"Maximum branches to report; default 8."},
    "min_duration_ms": {"type":"number","description":"Ignore intervals shorter than this; default 1ms."},
    "include_window_stats": {"type":"boolean","description":"For wakeup_chain, include same-window CPU/IO/binder/irq stats; default true."},
    "core_topology": {"type":"string","description":"Optional CPU core class map for compute-supply evaluation, e.g. \"small=0-3,middle=4-7,big=8-11\" or \"little=0-3,big=4-7\". If omitted, classes are inferred from observed CPU frequency tiers when possible."},
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
	if discovery, ok := t.maybeLargeRecipeDiscovery(ctx, p, path, sourceLabel); ok {
		return discovery, nil
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
		Pattern:              p.Pattern,
		SpanName:             p.SpanName,
		InteractionDirection: p.InteractionDirection,
		RecipeName:           p.RecipeName,
		MaxDepth:             p.MaxDepth.Int(),
		MaxBranches:          p.MaxBranches.Int(),
		MinDurationMs:        p.MinDurationMs.Float64(),
		Limit:                p.Limit.Int(),
		CoreTopology:         p.CoreTopology,
		IncludeWindowStats:   p.IncludeWindowStats != nil && p.IncludeWindowStats.Bool(),
	}
	q.TracePlatformHint, q.TracePlatformSource = tracePlatformHintForQuery(ctx, p, sourceLabel, path)
	q.TraceFlavorHint, q.TraceFlavorHintSource = traceFlavorHintForQuery(ctx, p, sourceLabel, path, q.TracePlatformHint, q.TracePlatformSource)
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

func traceFlavorHintForQuery(ctx *types.BusContext, p traceQueryParams, sourceLabel, resolvedPath string, platform tracequery.TracePlatform, platformSource string) (tracequery.TraceFlavor, string) {
	if platform != "" && platform != tracequery.TracePlatformAuto {
		if flavor := tracequery.FlavorForPlatform(platform); flavor != "" && flavor != tracequery.TraceFlavorAuto {
			return flavor, platformSource
		}
	}
	if flavor := tracequery.NormalizeTraceFlavor(p.TraceFlavor); flavor != "" && flavor != tracequery.TraceFlavorAuto {
		return flavor, "tool_param"
	}
	if flavor := traceFlavorHintFromUserRequest(ctx); flavor != "" && flavor != tracequery.TraceFlavorAuto {
		return flavor, "user_request"
	}
	if ctx != nil && (strings.TrimSpace(sourceLabel) == "attached_trace" || traceQueryPathIsAttachedTraceBlob(ctx, resolvedPath)) {
		if flavor := tracequery.NormalizeTraceFlavor(ctx.AttachedHitraceSource); flavor != "" && flavor != tracequery.TraceFlavorAuto {
			return flavor, "attached_source"
		}
	}
	return tracequery.TraceFlavorAuto, ""
}

func tracePlatformHintForQuery(ctx *types.BusContext, p traceQueryParams, sourceLabel, resolvedPath string) (tracequery.TracePlatform, string) {
	if platform := tracequery.NormalizeTracePlatform(p.Platform); platform != "" && platform != tracequery.TracePlatformAuto {
		return platform, "tool_param"
	}
	if platform := tracePlatformHintFromUserRequest(ctx); platform != "" && platform != tracequery.TracePlatformAuto {
		return platform, "user_request"
	}
	if ctx != nil && (strings.TrimSpace(sourceLabel) == "attached_trace" || traceQueryPathIsAttachedTraceBlob(ctx, resolvedPath)) {
		if platform := tracequery.NormalizeTracePlatform(ctx.AttachedHitraceSource); platform != "" && platform != tracequery.TracePlatformAuto {
			return platform, "attached_source"
		}
	}
	return tracequery.TracePlatformAuto, ""
}

func traceFlavorHintFromUserRequest(ctx *types.BusContext) tracequery.TraceFlavor {
	if ctx == nil || ctx.Mutable == nil {
		return tracequery.TraceFlavorAuto
	}
	text := strings.ToLower(ctx.Mutable.Objective())
	harmony := strings.Contains(text, "harmony") ||
		strings.Contains(text, "openharmony") ||
		strings.Contains(text, "ohos") ||
		strings.Contains(text, "hitrace") ||
		strings.Contains(text, "bytrace") ||
		strings.Contains(text, "鸿蒙") ||
		strings.Contains(text, "东湖")
	android := strings.Contains(text, "android") ||
		strings.Contains(text, "安卓") ||
		strings.Contains(text, "atrace")
	switch {
	case harmony && !android:
		return tracequery.TraceFlavorHarmonyHitrace
	case android && !harmony:
		return tracequery.TraceFlavorAndroidAtrace
	default:
		return tracequery.TraceFlavorAuto
	}
}

func tracePlatformHintFromUserRequest(ctx *types.BusContext) tracequery.TracePlatform {
	if ctx == nil || ctx.Mutable == nil {
		return tracequery.TracePlatformAuto
	}
	text := strings.ToLower(ctx.Mutable.Objective())
	switch {
	case strings.Contains(text, "东湖") || strings.Contains(text, "donghu"):
		return tracequery.TracePlatformDonghu
	case strings.Contains(text, "harmony") ||
		strings.Contains(text, "openharmony") ||
		strings.Contains(text, "ohos") ||
		strings.Contains(text, "hitrace") ||
		strings.Contains(text, "bytrace") ||
		strings.Contains(text, "鸿蒙"):
		return tracequery.TracePlatformHarmony
	case strings.Contains(text, "android") ||
		strings.Contains(text, "安卓") ||
		strings.Contains(text, "atrace"):
		return tracequery.TracePlatformAndroid
	default:
		return tracequery.TracePlatformAuto
	}
}

func traceQueryPathIsAttachedTraceBlob(ctx *types.BusContext, resolvedPath string) bool {
	if ctx == nil || strings.TrimSpace(resolvedPath) == "" || strings.TrimSpace(ctx.WorkDir) == "" {
		return false
	}
	want := filepath.Clean(filepath.Join(ctx.WorkDir, promptctx.AttachedTraceBlobName))
	got := filepath.Clean(resolvedPath)
	if !filepath.IsAbs(got) {
		got = filepath.Clean(resolveToolPath(ctx, got))
	}
	if absWant, err := filepath.Abs(want); err == nil {
		want = filepath.Clean(absWant)
	}
	if absGot, err := filepath.Abs(got); err == nil {
		got = filepath.Clean(absGot)
	}
	return got == want
}

type traceQueryRecipeDiscoveryMarker struct {
	Line    int     `json:"line"`
	Ts      float64 `json:"ts,omitempty"`
	Token   string  `json:"token,omitempty"`
	Primary bool    `json:"primary,omitempty"`
	Raw     string  `json:"raw,omitempty"`
}

type traceQueryRecipeDiscoveryToken struct {
	Text    string
	Primary bool
}

func (t *TraceQuery) maybeLargeRecipeDiscovery(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string) (types.ToolResult, bool) {
	info, err := os.Stat(path)
	if err != nil || !traceQueryShouldUseLargeRecipeDiscovery(p, info.Size()) {
		return types.ToolResult{}, false
	}
	markers, scannedLines, truncated, scanErr := scanTraceQueryRecipeMarkers(contextFromBus(ctx), path, traceQueryRecipeDiscoveryTokens(ctx, p), 48)
	if scanErr != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("trace_query recipe discovery failed for %s: %v", path, scanErr),
			Timestamp: time.Now(),
		}, true
	}
	payload := map[string]any{
		"mode":          "large_trace_recipe_discovery",
		"source_path":   path,
		"source":        sourceLabel,
		"recipe_name":   firstNonEmptyTraceString(p.RecipeName, "jank"),
		"size_bytes":    info.Size(),
		"scanned_lines": scannedLines,
		"truncated":     truncated,
		"markers":       markers,
	}
	payloadBytes, _ := json.MarshalIndent(payload, "", "  ")
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-recipe-discovery.json", string(payloadBytes))
	summary := traceQueryRecipeDiscoverySummary(path, sourceLabel, p, info.Size(), scannedLines, markers, truncated, payloadRef)
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
	}, true
}

func traceQueryShouldUseLargeRecipeDiscovery(p traceQueryParams, size int64) bool {
	if size < traceQueryLargeRecipeDiscoveryMinBytes {
		return false
	}
	if strings.TrimSpace(p.View) != "recipe" {
		return false
	}
	recipe := strings.ToLower(strings.TrimSpace(p.RecipeName))
	recipe = strings.ReplaceAll(recipe, "-", "_")
	if recipe != "" && recipe != "auto" && recipe != "jank" && recipe != "frame" && recipe != "frame_jank" && recipe != "render" && recipe != "render_pipeline" {
		return false
	}
	return !traceQueryHasExplicitNarrowing(p)
}

func traceQueryHasExplicitNarrowing(p traceQueryParams) bool {
	return p.TimeStart.Set() ||
		p.TimeEnd.Set() ||
		p.LineStart.Int() > 0 ||
		p.LineEnd.Int() > 0 ||
		p.PID.Int() > 0 ||
		strings.TrimSpace(p.Thread) != "" ||
		strings.TrimSpace(p.SpanName) != "" ||
		len(p.EventTypes.Strings()) > 0
}

func traceQueryRecipeDiscoveryTokens(ctx *types.BusContext, p traceQueryParams) []traceQueryRecipeDiscoveryToken {
	var tokens []traceQueryRecipeDiscoveryToken
	add := func(s string, primary bool) {
		s = strings.TrimSpace(s)
		if s != "" {
			tokens = append(tokens, traceQueryRecipeDiscoveryToken{Text: s, Primary: primary})
		}
	}
	for _, token := range traceQueryObjectiveKVTokenRE.FindAllString(traceQueryObjectiveText(ctx), 12) {
		add(token, true)
	}
	add(p.Pattern, true)
	add(p.SpanName, true)
	add("jank_frames", false)
	add("jank", false)
	add("FrameTimeline", false)
	add("ActualTimeline", false)
	add("ExpectedTimeline", false)
	add("Choreographer#doFrame", false)
	add("RenderFrame", false)
	seen := map[string]bool{}
	out := make([]traceQueryRecipeDiscoveryToken, 0, len(tokens))
	for _, token := range tokens {
		key := strings.ToLower(strings.TrimSpace(token.Text))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, token)
	}
	return out
}

func traceQueryObjectiveText(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	return ctx.Mutable.Objective()
}

func scanTraceQueryRecipeMarkers(ctx context.Context, path string, tokens []traceQueryRecipeDiscoveryToken, maxMarkers int) ([]traceQueryRecipeDiscoveryMarker, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxMarkers <= 0 {
		maxMarkers = 48
	}
	lowerTokens := make([]traceQueryRecipeDiscoveryToken, 0, len(tokens))
	for _, token := range tokens {
		token.Text = strings.ToLower(strings.TrimSpace(token.Text))
		if token.Text != "" {
			lowerTokens = append(lowerTokens, token)
		}
	}
	if len(lowerTokens) == 0 {
		lowerTokens = []traceQueryRecipeDiscoveryToken{{Text: "jank"}}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, false, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 256*1024)
	var markers []traceQueryRecipeDiscoveryMarker
	truncated := false
	lineNo := 0
	for {
		if err := ctx.Err(); err != nil {
			return markers, lineNo, truncated, err
		}
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			lineNo++
			trimmed := strings.TrimRight(line, "\r\n")
			if token, primary := firstTraceQueryMarkerToken(trimmed, lowerTokens); token != "" {
				marker := traceQueryRecipeDiscoveryMarker{
					Line:    lineNo,
					Ts:      traceQueryTimestampFromLine(trimmed),
					Token:   token,
					Primary: primary,
					Raw:     truncateForLog(trimmed, 500),
				}
				if len(markers) < maxMarkers {
					markers = append(markers, marker)
				} else if primary && replaceLastFallbackMarker(markers, marker) {
					truncated = true
				} else {
					truncated = true
					if primary {
						break
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return markers, lineNo, truncated, err
		}
	}
	return markers, lineNo, truncated, nil
}

func firstTraceQueryMarkerToken(line string, lowerTokens []traceQueryRecipeDiscoveryToken) (string, bool) {
	lower := strings.ToLower(line)
	for _, token := range lowerTokens {
		if strings.Contains(lower, token.Text) {
			return token.Text, token.Primary
		}
	}
	return "", false
}

func replaceLastFallbackMarker(markers []traceQueryRecipeDiscoveryMarker, marker traceQueryRecipeDiscoveryMarker) bool {
	for i := len(markers) - 1; i >= 0; i-- {
		if !markers[i].Primary {
			markers[i] = marker
			return true
		}
	}
	return false
}

func traceQueryTimestampFromLine(line string) float64 {
	m := traceQueryTimestampRE.FindStringSubmatch(line)
	if len(m) < 2 {
		return 0
	}
	var ts float64
	_, _ = fmt.Sscanf(m[1], "%f", &ts)
	return ts
}

func traceQueryRecipeDiscoverySummary(path, sourceLabel string, p traceQueryParams, size int64, scannedLines int, markers []traceQueryRecipeDiscoveryMarker, truncated bool, payloadRef string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[trace_query params: view=recipe source=%s path=%s origin=runtime_artifact artifact_id=%s artifact_kind=trace recipe_name=%s mode=large_trace_recipe_discovery platform=%s trace_flavor=%s payload_ref=%s]\n",
		sourceLabel,
		sanitizeForBanner(path),
		traceQueryArtifactID(sourceLabel),
		sanitizeForBanner(firstNonEmptyTraceString(p.RecipeName, "jank")),
		sanitizeForBanner(p.Platform),
		sanitizeForBanner(p.TraceFlavor),
		sanitizeForBanner(payloadRef),
	)
	b.WriteString("# Trace Query: recipe discovery\n\n")
	fmt.Fprintf(&b, "source=%s size_bytes=%d scanned_lines=%d matched_markers=%d\n", sanitizeForBanner(path), size, scannedLines, len(markers))
	if payloadRef != "" {
		fmt.Fprintf(&b, "payload_ref=%s\n", sanitizeForBanner(payloadRef))
	}
	b.WriteString("large_trace_recipe_guard=unbounded jank recipe on a large trace was not expanded into full-trace window_stats/root_cause_rank/scheduler scans. Select a marker below, then rerun trace_query with line_start/line_end or time_start/time_end.\n")
	if truncated {
		b.WriteString("discovery_compacted=true; more markers exist in the trace, see payload_ref or refine with span_name/time/line filters.\n")
	}
	if len(markers) > 0 {
		b.WriteString("\n## Candidate jank/frame markers\n")
		for _, marker := range markers {
			primary := ""
			if marker.Primary {
				primary = " primary=true"
			}
			fmt.Fprintf(&b, "- line=%d ts=%.6f token=%s%s raw=%s\n", marker.Line, marker.Ts, sanitizeForBanner(marker.Token), primary, sanitizeForBanner(marker.Raw))
		}
		first := firstPrimaryTraceQueryMarker(markers)
		start := first.Line - 200
		if start < 1 {
			start = 1
		}
		end := first.Line + 200
		fmt.Fprintf(&b, "\nnext_call_hint=rerun `trace_query` with view=\"recipe\", recipe_name=\"jank\", path=\"%s\", line_start=%d, line_end=%d; if this marker is a B/E span, alternatively use span_window around the marker then rerun with time_start/time_end.\n", sanitizeForBanner(path), start, end)
		if first.Ts > 0 {
			tsStart := first.Ts - 0.250
			if tsStart < 0 {
				tsStart = 0
			}
			fmt.Fprintf(&b, "time_window_hint=around first marker: time_start=%.6f time_end=%.6f seconds\n", tsStart, first.Ts+0.250)
		}
	} else {
		b.WriteString("\nno_marker_advisory=no jank/frame marker was found by the light discovery tokens. Provide pattern with one exact literal frame id/span label/marker token, span_name, time_start/time_end, line_start/line_end, pid/thread, or run event_search for a narrower deterministic query before requesting the full recipe.\n")
	}
	return b.String()
}

func firstPrimaryTraceQueryMarker(markers []traceQueryRecipeDiscoveryMarker) traceQueryRecipeDiscoveryMarker {
	for _, marker := range markers {
		if marker.Primary {
			return marker
		}
	}
	if len(markers) == 0 {
		return traceQueryRecipeDiscoveryMarker{}
	}
	return markers[0]
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
	fmt.Fprintf(&b, "[trace_query params: view=%s source=%s path=%s origin=runtime_artifact artifact_id=%s artifact_kind=trace thread=%s pid=%s line_start=%s line_end=%s time_start=%s time_end=%s pattern=%s span_name=%s interaction_direction=%s recipe_name=%s platform=%s platform_candidate=%s trace_flavor=%s trace_flavor_confidence=%.2f priority_rule=%s payload_ref=%s]\n",
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
		sanitizeForBanner(p.Pattern),
		sanitizeForBanner(p.SpanName),
		sanitizeForBanner(p.InteractionDirection),
		sanitizeForBanner(p.RecipeName),
		sanitizeForBanner(result.Platform),
		sanitizeForBanner(result.PlatformCandidate),
		sanitizeForBanner(result.TraceFlavor),
		result.FlavorConfidence,
		traceQueryPriorityRuleBanner(result.TraceFlavor),
		sanitizeForBanner(payloadRef),
	)
	fmt.Fprintf(&b, "# Trace Query: %s\n\n", result.View)
	fmt.Fprintf(&b, "source=%s lines=%d parsed_events=%d timestamp_unit=%s selected_window=%.6f..%.6f seconds\n", result.SourcePath, result.LineCount, result.EventCount, firstNonEmptyTraceString(result.TimeUnit, "seconds"), result.TimeStart, result.TimeEnd)
	if result.TraceFlavor != "" {
		fmt.Fprintf(&b, "trace_flavor=%s confidence=%.2f\n", result.TraceFlavor, result.FlavorConfidence)
	}
	if result.Platform != "" {
		fmt.Fprintf(&b, "platform=%s framework_mode=%s\n", result.Platform, result.FrameworkMode)
	}
	if result.PlatformCandidate != "" {
		fmt.Fprintf(&b, "platform_candidate=%s confidence=%.2f\n", result.PlatformCandidate, result.PlatformCandidateConfidence)
	}
	if len(result.FrameworkSurfaces) > 0 {
		var parts []string
		for _, surface := range result.FrameworkSurfaces {
			parts = append(parts, fmt.Sprintf("%s:%d", surface.Surface, surface.ProcessCount))
		}
		fmt.Fprintf(&b, "framework_surfaces=%s\n", sanitizeForBanner(strings.Join(parts, ",")))
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
			for _, caveat := range wait.Caveats {
				fmt.Fprintf(&b, "  binder_wait_detail=%s\n", caveat)
			}
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
		writeTraceBinderEvents(&b, result.IPCGraph.BinderEvents)
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
	if result.SchedulerLatency != nil {
		b.WriteString("## Scheduler latency stats\n")
		fmt.Fprintf(&b, "- count=%d mean=%.3fms p50=%.3fms p95=%.3fms p99=%.3fms max=%.3fms\n",
			result.SchedulerLatency.Count, result.SchedulerLatency.MeanMs, result.SchedulerLatency.P50Ms, result.SchedulerLatency.P95Ms, result.SchedulerLatency.P99Ms, result.SchedulerLatency.MaxMs)
		for _, item := range result.SchedulerLatency.Items {
			fmt.Fprintf(&b, "- runnable_wait %s %.6f..%.6f duration=%.3fms cpu=%d freq=%dkHz prio=%d/%s same_cpu_busy=%.3fms same_cpu_idle=%.3fms other_cpu_idle=%.3fms high_prio_running=%.3fms lines=%d-%d — %s\n",
				traceThreadLabel(item.Thread), item.StartTs, item.EndTs, item.DurationMs, item.CPU, item.Frequency, item.Priority, item.PriorityClass, item.SameCPUBusyMs, item.SameCPUIdleMs, item.OtherCPUIdleMs, item.HighPriorityRunningMs, item.StartLine, item.EndLine, item.Summary)
		}
		for _, caveat := range result.SchedulerLatency.Caveats {
			fmt.Fprintf(&b, "- scheduler_latency_caveat=%s\n", caveat)
		}
		b.WriteString("\n")
	}
	if result.WindowStats != nil {
		b.WriteString("## Window stats\n")
		for _, cpu := range result.WindowStats.CPU {
			fmt.Fprintf(&b, "- cpu=%d core_class=%s busy=%.3fms idle=%.3fms freq=%d%s\n", cpu.CPU, sanitizeForBanner(cpu.CoreClass), cpu.BusyMs, cpu.IdleMs, cpu.Frequency, traceFrequencyResidencySummary(cpu.FrequencyResidency))
		}
		for _, core := range result.WindowStats.CoreTopology {
			fmt.Fprintf(&b, "- core_class=%s cpus=%v busy=%.3fms idle=%.3fms runnable_wait=%.3fms high_prio_running=%.3fms max_freq=%dkHz source=%s signal=%s\n",
				sanitizeForBanner(core.Class), core.CPUs, core.BusyMs, core.IdleMs, core.RunnableWaitMs, core.HighPriorityRunMs, core.MaxFrequency, sanitizeForBanner(core.TopologySource), sanitizeForBanner(core.ComputeSupplySignal))
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
		for _, limit := range result.WindowStats.CPUFrequencyLimits {
			fmt.Fprintf(&b, "- cpu_frequency_limit cpu=%d min=%dkHz max=%dkHz count=%d line=%d\n",
				limit.CPU, limit.MinFrequency, limit.MaxFrequency, limit.Count, limit.Line)
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
		for _, resource := range result.WindowStats.BIOResources {
			writeTraceRuntimeResource(&b, "bio", resource)
		}
		for _, resource := range result.WindowStats.FilesystemResources {
			writeTraceRuntimeResource(&b, "filesystem", resource)
		}
		for _, resource := range result.WindowStats.PageFaultResources {
			writeTraceRuntimeResource(&b, "page_fault", resource)
		}
		for _, event := range result.WindowStats.AbilityEvents {
			writeTracePluginSummary(&b, event)
		}
		for _, event := range result.WindowStats.XPowerEvents {
			writeTracePluginSummary(&b, event)
		}
		for _, event := range result.WindowStats.HiSystemEvents {
			writeTracePluginSummary(&b, event)
		}
		for _, subsystem := range result.WindowStats.SubsystemEvents {
			fmt.Fprintf(&b, "- subsystem kind=%s event_type=%s count=%d line=%d example=%s\n",
				subsystem.Kind, subsystem.EventType, subsystem.Count, subsystem.Line, sanitizeForBanner(subsystem.Example))
		}
		for _, drift := range result.WindowStats.ThreadDrifts {
			fmt.Fprintf(&b, "- thread_identity_caveat pid=%d names=%s tgids=%v lines=%d-%d\n",
				drift.PID, strings.Join(drift.Names, ","), drift.TGIDs, drift.LineStart, drift.LineEnd)
		}
		for _, supply := range result.WindowStats.ComputeSupply {
			fmt.Fprintf(&b, "- compute_supply %s state=%s cpu=%d core_class=%s duration=%.3fms freq=%dkHz busy=%.3fms idle=%.3fms runnable_wait=%.3fms high_prio_running=%.3fms verdict=%s confidence=%.2f lines=%d-%d — %s\n",
				traceThreadLabel(supply.Thread), supply.State, supply.CPU, sanitizeForBanner(supply.CoreClass), supply.DurationMs, supply.Frequency, supply.CPUBusyMs, supply.CPUIdleMs, supply.RunnableWaitMs, supply.HighPriorityRunningMs, supply.Verdict, supply.Confidence, supply.LineStart, supply.LineEnd, supply.Summary)
		}
		fmt.Fprintf(&b, "- counts block_issue=%d block_remap=%d block_complete=%d binder=%d binder_received=%d binder_aux=%d irq=%d softirq=%d memory=%d storage=%d filesystem=%d power=%d ability=%d xpower=%d hi_sysevent=%d workqueue=%d dma_fence=%d blocked_reason=%d iowait_blocked=%d\n\n",
			result.WindowStats.BlockIssueCount, result.WindowStats.BlockRemapCount, result.WindowStats.BlockCompleteCount, result.WindowStats.BinderCount, result.WindowStats.BinderReceivedCount, result.WindowStats.BinderAuxCount, result.WindowStats.IRQCount, result.WindowStats.SoftIRQCount, result.WindowStats.MemoryEventCount, result.WindowStats.StorageEventCount, result.WindowStats.FilesystemEventCount, result.WindowStats.PowerEventCount, result.WindowStats.AbilityEventCount, result.WindowStats.XPowerEventCount, result.WindowStats.HiSystemEventCount, result.WindowStats.WorkqueueEventCount, result.WindowStats.DMAFenceEventCount, result.WindowStats.BlockedReasonCount, result.WindowStats.IOWaitBlockedCount)
	}
	if result.FramePipeline != nil {
		b.WriteString("## Frame/render pipeline\n")
		for _, item := range result.FramePipeline.Items {
			fmt.Fprintf(&b, "- frame_phase=%s %s %q %.6f..%.6f duration=%.3fms lines=%d-%d — %s\n",
				item.Phase, traceThreadLabel(item.Thread), item.Name, item.StartTs, item.EndTs, item.DurationMs, item.StartLine, item.EndLine, item.Summary)
		}
		for _, caveat := range result.FramePipeline.Caveats {
			fmt.Fprintf(&b, "- frame_pipeline_caveat=%s\n", caveat)
		}
		b.WriteString("\n")
	}
	if result.FrameTimeline != nil {
		b.WriteString("## Frame timeline\n")
		for _, item := range result.FrameTimeline.Items {
			fmt.Fprintf(&b, "- frame_item index=%d role=%s phase=%s thread=%s frame_id=%s %.6f..%.6f duration=%.3fms lines=%d-%d — %s\n",
				item.Index, item.Role, item.Phase, traceThreadLabel(item.Thread), sanitizeForBanner(item.FrameID), item.StartTs, item.EndTs, item.DurationMs, item.StartLine, item.EndLine, sanitizeForBanner(item.Summary))
		}
		for _, flow := range result.FrameTimeline.Flows {
			fmt.Fprintf(&b, "- frame_flow %d->%d %s/%s -> %s/%s latency=%.3fms lines=%d-%d — %s\n",
				flow.FromIndex, flow.ToIndex, traceThreadLabel(flow.From), flow.FromPhase, traceThreadLabel(flow.To), flow.ToPhase, flow.LatencyMs, flow.LineStart, flow.LineEnd, sanitizeForBanner(flow.Summary))
		}
		for _, caveat := range result.FrameTimeline.Caveats {
			fmt.Fprintf(&b, "- frame_timeline_caveat=%s\n", caveat)
		}
		b.WriteString("\n")
	}
	if result.CriticalBlocking != nil {
		b.WriteString("## Critical blocking calls\n")
		for _, item := range result.CriticalBlocking.Items {
			fmt.Fprintf(&b, "- blocking type=%s thread=%s peer=%s duration=%.3fms lines=%d-%d confidence=%.2f — %s\n",
				item.Type, traceThreadLabel(item.Thread), traceThreadLabel(item.Peer), item.DurationMs, item.LineStart, item.LineEnd, item.Confidence, item.Summary)
		}
		for _, caveat := range result.CriticalBlocking.Caveats {
			fmt.Fprintf(&b, "- critical_blocking_caveat=%s\n", caveat)
		}
		b.WriteString("\n")
	}
	if result.Recipe != nil {
		b.WriteString("## Recipe\n")
		fmt.Fprintf(&b, "- recipe=%s included_views=%s — %s\n", result.Recipe.Name, strings.Join(result.Recipe.IncludedViews, ","), result.Recipe.Summary)
		for _, caveat := range result.Recipe.Caveats {
			fmt.Fprintf(&b, "- recipe_caveat=%s\n", caveat)
		}
		b.WriteString("\n")
	}
	if len(result.Events) > 0 {
		b.WriteString("## Events\n")
		for _, ev := range result.Events {
			fmt.Fprintf(&b, "- line=%d ts=%.6f type=%s thread=%s%s%s%s raw=%s\n",
				ev.Line,
				ev.Ts,
				ev.Type,
				traceThreadLabel(tracequery.ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}),
				traceEventPriorityDetail(ev),
				traceEventSchedulerDetail(ev),
				traceEventResourceDetail(ev),
				strings.TrimSpace(ev.Raw),
			)
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

func traceQueryPriorityRuleBanner(flavor string) string {
	switch tracequery.TraceFlavor(strings.TrimSpace(flavor)) {
	case tracequery.TraceFlavorHarmonyHitrace:
		return "harmony_larger_numeric_higher_1_40_CFS_41_139_RT"
	case tracequery.TraceFlavorAndroidAtrace:
		return "android_raw_scheduler_priority_no_harmony_mapping"
	default:
		return "generic_raw_scheduler_priority"
	}
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

func writeTraceBinderEvents(b *strings.Builder, events []tracequery.BinderEventSummary) {
	for i, event := range events {
		if i >= 12 {
			fmt.Fprintf(b, "... omitted %d binder auxiliary event(s); see payload_ref\n", len(events)-i)
			break
		}
		fmt.Fprintf(b, "- binder_event type=%s thread=%s transaction=%d debug_id=%d data=%d offsets=%d extra=%d tag=%s line=%d — %s\n",
			event.Type,
			traceThreadLabel(event.Thread),
			event.TransactionID,
			event.DebugID,
			event.DataSize,
			event.OffsetsSize,
			event.ExtraBuffersSize,
			sanitizeForBanner(event.Tag),
			event.Line,
			sanitizeForBanner(event.Summary),
		)
	}
}

func writeTraceRuntimeResource(b *strings.Builder, label string, item tracequery.RuntimeResourceSummary) {
	fmt.Fprintf(b, "- %s_resource op=%s path=%s thread=%s count=%d total_latency=%.3fms max_latency=%.3fms bytes=%d line=%d example=%s\n",
		label,
		sanitizeForBanner(item.Operation),
		sanitizeForBanner(item.Path),
		traceThreadLabel(item.Thread),
		item.Count,
		item.TotalLatencyMs,
		item.MaxLatencyMs,
		item.Bytes,
		item.Line,
		sanitizeForBanner(item.Example),
	)
	if item.Callstack != "" {
		fmt.Fprintf(b, "  callstack=%s\n", sanitizeForBanner(item.Callstack))
	}
}

func writeTracePluginSummary(b *strings.Builder, item tracequery.TracePluginSummary) {
	fmt.Fprintf(b, "- plugin_event kind=%s domain=%s event=%s metric=%s value=%s category=%s thread=%s count=%d line=%d example=%s\n",
		sanitizeForBanner(item.Kind),
		sanitizeForBanner(item.Domain),
		sanitizeForBanner(item.EventName),
		sanitizeForBanner(item.Metric),
		sanitizeForBanner(item.Value),
		sanitizeForBanner(item.Category),
		traceThreadLabel(item.Thread),
		item.Count,
		item.Line,
		sanitizeForBanner(item.Example),
	)
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

func traceEventPriorityDetail(ev tracequery.EventView) string {
	var parts []string
	if ev.PrevPrio > 0 {
		parts = append(parts, traceEventPrioField("prev_prio", ev.PrevPrio, ev.PrevPrioClass))
	}
	if ev.NextPrio > 0 {
		parts = append(parts, traceEventPrioField("next_prio", ev.NextPrio, ev.NextPrioClass))
	}
	if ev.WakeePrio > 0 {
		parts = append(parts, traceEventPrioField("wakee_prio", ev.WakeePrio, ev.WakeePrioClass))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func traceEventSchedulerDetail(ev tracequery.EventView) string {
	var parts []string
	if ev.NextInfo != "" {
		parts = append(parts, "next_info="+ev.NextInfo)
	}
	if ev.CGroup != "" {
		parts = append(parts, "cgroup="+ev.CGroup)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func traceEventResourceDetail(ev tracequery.EventView) string {
	var parts []string
	if ev.FrequencyMin > 0 || ev.FrequencyMax > 0 {
		parts = append(parts, fmt.Sprintf("freq_limit=%d..%dkHz", ev.FrequencyMin, ev.FrequencyMax))
	}
	if ev.BlockError != "" {
		parts = append(parts, "block_error="+ev.BlockError)
	}
	if ev.SubsystemKind != "" {
		parts = append(parts, "subsystem="+ev.SubsystemKind)
	}
	if ev.PluginEventName != "" {
		parts = append(parts, "plugin_event="+sanitizeForBanner(ev.PluginEventName))
	}
	if ev.PluginMetric != "" || ev.PluginValue != "" {
		parts = append(parts, fmt.Sprintf("metric=%s value=%s", sanitizeForBanner(ev.PluginMetric), sanitizeForBanner(ev.PluginValue)))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func traceEventPrioField(name string, prio int, class string) string {
	if strings.TrimSpace(class) == "" {
		return fmt.Sprintf("%s=%d", name, prio)
	}
	return fmt.Sprintf("%s=%d/%s", name, prio, class)
}

func traceThreadDurationLocation(td tracequery.ThreadDuration) string {
	parts := []string{fmt.Sprintf("cpu=%d", td.CPU)}
	if td.CoreClass != "" {
		parts = append(parts, "core_class="+td.CoreClass)
	}
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
