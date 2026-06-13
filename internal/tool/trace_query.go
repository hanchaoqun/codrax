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
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/logging"
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
	traceQueryWindowedIndexMinBytes        int64 = 64 << 20
	traceQueryObjectiveKVTokenRE                 = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_./:-]*=[^\s,，。；;"'）)]+`)
	traceQueryObjectiveFrameIDRE                 = regexp.MustCompile(`(?i)Choreographer#doFrame\D{0,32}([0-9]{3,})`)
	traceQueryObjectiveHexTokenRE                = regexp.MustCompile(`(?i)\b0x[0-9a-f]{4,}\b`)
	traceQueryObjectiveQuotedTokenRE             = regexp.MustCompile(`"([^"\n]{3,160})"|“([^”\n]{3,160})”|'([^'\n]{3,160})'|‘([^’\n]{3,160})’`)
	traceQueryObjectiveLabeledTokenRE            = regexp.MustCompile(`(?i)(?:span(?:_name)?(?:\s*关键字)?|marker|label|keyword|关键字|标记|标签|span名|span名称)\s*(?:=|:|：|为|是|叫|名为)?\s*([A-Za-z0-9_#./:$@+\-]{3,160})`)
	traceQueryObjectivePreLabeledTokenRE         = regexp.MustCompile(`(?i)([A-Za-z0-9_#./:$@+\-]{3,160})\s*(?:这个|该|此)?\s*(?:span|marker|label|keyword|关键字|标记|标签)`)
	traceQueryTimestampRE                        = regexp.MustCompile(`\s([0-9]+(?:\.[0-9]+)?):\s+`)
)

func traceQueryMemoryForLog() (heapAlloc, heapSys uint64, gcCount uint32) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc, stats.HeapSys, stats.NumGC
}

func (t *TraceQuery) Name() string { return "trace_query" }

func (t *TraceQuery) Description() string {
	return "Deterministically queries large runtime trace/log artifacts for scheduler timelines, scheduler latency stats, trace span/frame windows, frame timelines/flows, render pipelines, ranked root causes, wakeup chains, frame root-cause bundles, binder IPC graphs, critical blocking calls, interaction Top-N, same-window resource stats, recipes, structured event search, and line-backed evidence packs. wakeup_chain/root_cause_rank/frame_root_cause_bundle publish structured wakeup_chain path, per-edge wakeup_chain_edge rows, causal_impact rows, and chain_relevance fields (on_chain, adjacent, background); consume those ordered path/edge/relevance fields before paraphrasing dependency chains so upstream waker -> intermediate dependency -> target causality is not lost in prose and off-chain background load is not promoted to primary cause. root_cause_rank rows carry dominant_state plus running/runnable/sleep/d_state/io_wait totals; when an on_chain runnable, running/compute-supply, low-frequency, affinity/cpuset, D-state, or IO dependency is tier=primary, report it as a co-primary cause instead of moving it to background. wakeup_chain also reports aggregated_impact rows when repeated fragmented branches share a common dependency path, so compare the aggregate against single long intervals. Treat critical_blocking_calls as direct blocking surfaces: for binder/futex/lock/sync waits, preserve peer, peer_state, chain_relevance, overlap, nearest_chain_thread, and then continue into peer thread state, wakeup_chain, root_cause_rank, and resource rows before naming the cause; if peer/on-chain evidence is missing, keep the wait as a bounded symptom/candidate with caveat. window_stats/event_search can filter or summarize scheduler, binder transaction/received/lock/alloc/reply rows, CPU idle/frequency/frequency-limit, CPU affinity/cpuset/migration constraint evidence, block IO, IRQ/softirq, storage, filesystem, power, Ability/XPower/HiSystemEvent resource observations, workqueue, DMA fence, memory-like events, and SmartPerf-style eBPF BIO/FileSystem/PageFault resource rows when converted to text key/value fields. For runnable root causes, window_stats/root_cause_rank report runnable_context, thread_cpu_load, cpu_constraints, and secondary process_cpu_load: consume the concrete thread load, same-CPU competitors, CPU/core class, other-core idle, Harmony/Donghu sched_switch next_info affinity/restricted fields, cpuset/allowed CPU evidence, and only then the process rollup. These are output sections/candidate signals, not separate views; use view=window_stats to inspect them directly, view=root_cause_rank to let them enrich and compete with scheduler candidates, or view=frame_root_cause_bundle for frame/jank windows that need wakeup_chain + rank + blocking + IO/IRQ/workqueue/supply/trace-mark evidence in one handoff-safe result. window_stats/root_cause_rank/frame_root_cause_bundle also report inode-level IO outputs: file_io_by_inode for Android FS/F2FS/EXT4-style file read/write/sync/direct-IO rows, page_cache_by_inode for mm_filemap add/delete churn, storage_latency_by_layer for block/MMC/SCSI/F2FS/Android-FS start-done latency pairs, block_io_by_inode to join inode activity with nearest block/storage latency, io_burst_episodes for D-state/iowait/storage bursts, and io_pressure_summary to relate inode IO, page-cache churn, block/storage latency, sched_blocked_reason iowait, and D-state totals. For IO completion questions, preserve file_io completions/ret/example and each storage_latency example together with bytes/len/offset and max_latency, so a single 4KB completion latency is not hidden by aggregate bytes or total latency. These are output sections/candidate signals, not separate views; use view=window_stats to inspect them directly or view=root_cause_rank/frame_root_cause_bundle to let them compete with scheduler and blocking causes. window_stats/frame_root_cause_bundle also report irq_activity, softirq_activity, workqueue_activity, supply_pressure_summary, trace_mark_categories, and async_file_work as supporting signals; use them to explain supply-side pressure and background interference without treating them as proof unless they overlap the target window or wakeup chain. For frame/drop/jank windows with no single long sleep/runnable/D/IO/running segment, window_stats/root_cause_rank also report state_churn: frequent state switching with per-state cumulative impact, fragment count, max/p95 segment, and next-step guidance so the dominant cumulative state can still rank as the primary cause. state_churn is an output section/candidate signal, not an independent view; use view=window_stats to inspect it directly or view=root_cause_rank/frame_root_cause_bundle to let it compete with other causes. For frame/span, runnable-context, or inode discovery, use view=event_search with pattern as a case-insensitive literal substring, not a regex; it is best for frame ids, jank ids, span labels, trace marker labels, thread labels, next_info tokens, cpuset labels, inode tokens such as 0x478e5, entry_name values, or one exact timestamp/event token before broad grep. Treat entry_name as a trace file-name label, not an absolute path; do not prefix it with /, /data/, or any directory unless that full path appears in the trace or an external mapping. If multiple span windows or zero rows come back, narrow with the returned line/time windows, a shorter literal pattern, event_types=[\"trace_mark\"], event_types=[\"cpu_constraint\"] for affinity/cpuset/next_info rows, event_types=[\"file_io\"] or event_types=[\"page_cache\"] for inode rows, pid/thread, or span_window before running recipe/root-cause views. Once a result reports selected_window, index_windowed, or a concrete line window, keep that same time_start/time_end or line_start/line_end on every follow-up heavy scheduler/resource/root-cause view; thread/pid alone is not enough for large traces. For big/middle/small core analysis, pass core_topology like \"small=0-3,middle=4-7,big=8-11\"; if omitted the tool only infers classes from observed CPU frequencies and reports that caveat. For very large traces, an unbounded jank recipe without time_start/time_end, line_start/line_end, span_name, pid, or thread first does light marker discovery; when timestamped top jank/frame markers are found it automatically runs bounded recipe analysis for the top candidate windows, and otherwise returns marker discovery plus next-call hints instead of expanding expensive full-trace root-cause/resource views. Trace timestamps are seconds end-to-end: 928.081774 means 928 seconds + 0.081774 seconds; with six fractional digits, the fractional part is microsecond-precision (81774 us), not a separate millisecond field. Only derived durations are rendered in ms. Trace flavor is auto-detected as harmony_hitrace, android_atrace, or generic_ftrace; pass trace_flavor/platform when the user names a producer. Explicit user intent such as Harmony/鸿蒙/东湖/OHOS or Android/安卓 wins for the current call and is not auto-corrected, though content signals remain in caveats for audit. Auto detection may report platform_candidate=mixed_harmony_base when Harmony-base trace signals coexist with Android-framework process surfaces; this uses Donghu/Harmony scheduler priority semantics, not Android priority semantics. Donghu/东湖 uses Harmony/OpenHarmony trace scheduler semantics with process-isolated Android-framework and Harmony-framework surfaces; priority and timestamp semantics still follow Harmony. For HarmonyOS/hitrace user-space priority, larger numeric priority means higher priority: 1-40=CFS, 41-139=RT. Android/generic ftrace keeps raw scheduler priority and does not apply Harmony ranges. Thread selectors accept pid plus common ftrace/hitrace labels such as com.tencent.mm-36379, com.tencent.mm 36379, com.tencent.mm [36379], [GT]ColdPool#5-36624, binder:486_1-10803, or pid=36379; pass pid directly when known. Use this before ad-hoc grep/awk for ftrace/systrace/hitrace time-window causality questions; keep grep/read_file as fallback for unsupported formats."
}

func (t *TraceQuery) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
	    "source": {"type":"string","enum":["path","attached_trace"],"x-codrax-enum-style-alias":true,"description":"Use attached_trace for the current --htrace/--atrace blob; use path for an explicit workspace/repo file."},
	    "path": {"type":"string","description":"Repo/workspace-relative or absolute trace/log path when source=path."},
	    "trace_flavor": {"type":"string","enum":["auto","harmony_hitrace","android_atrace","generic_ftrace"],"x-codrax-enum-style-alias":true,"description":"Optional producer/platform flavor. Defaults to auto detection. Use harmony_hitrace for HarmonyOS HiTrace priority semantics, android_atrace for Android/Linux atrace raw scheduler priorities, and generic_ftrace when uncertain."},
	    "platform": {"type":"string","enum":["auto","donghu","harmony","harmony_hitrace","android","android_atrace","generic","generic_ftrace"],"x-codrax-enum-style-alias":true,"description":"Optional platform hint. Use donghu when the user says 东湖: scheduler/time/priority semantics follow Harmony/OpenHarmony, while Android-framework and Harmony-framework processes may coexist at process boundaries. harmony/harmony_hitrace selects Harmony semantics; android/android_atrace selects Android raw scheduler priority semantics."},
		    "view": {"type":"string","enum":["event_search","span_window","frame_window","render_pipeline","frame_timeline","frame_flow","thread_timeline","window_stats","scheduler_latency_stats","ipc_graph","wakeup_chain","root_cause_rank","frame_root_cause_bundle","critical_blocking_calls","interaction_stats","recipe","evidence_pack"],"x-codrax-enum-style-alias":true,"x-codrax-enum-aliases":{"state_churn":"window_stats","causal_impact":"wakeup_chain","frame_bundle":"frame_root_cause_bundle","frame_rootcause_bundle":"frame_root_cause_bundle","frame_root_cause":"frame_root_cause_bundle"},"description":"The deterministic trace view to compute. Use span_window to turn a unique B/E trace span into a time window; frame_window/render_pipeline for Choreographer/RenderFrame/VSYNC/draw/present spans; frame_timeline/frame_flow for Expected/Actual/Jank/GPU/RS/UI phase summaries and cross-thread frame flows; scheduler_latency_stats for runnable wait p95/p99/max and CPU competition; wakeup_chain for wakeup edges and causal_impacts per chain node; critical_blocking_calls for futex/lock/sync/binder/IO/D-state candidates, with peer_state breakdown when the peer thread timeline is visible; root_cause_rank for primary/secondary/tertiary cause candidates, including dominant_state/running/runnable/sleep/d_state/io_wait totals, fragmented state_churn candidates when frequent short state switches cumulatively dominate, wakeup_chain causal_impacts and aggregated_impacts when repeated fragmented branches share a common dependency path, and co-primary on-chain runnable/running/compute-supply/D-state/IO dependencies when they are part of the same causal chain; frame_root_cause_bundle returns wakeup_chain + frame_timeline + root_cause_rank + critical_blocking_calls plus IO/IRQ/workqueue/supply/trace-mark bundle fields for frame/jank handoff; state_churn and causal_impacts are output sections, not standalone views; view=state_churn is accepted and treated as view=window_stats, view=causal_impact is accepted as wakeup_chain, and view=frame_bundle/frame_rootcause_bundle is accepted as frame_root_cause_bundle; interaction_stats for target-thread wakeup/binder interaction Top-N; recipe for standard evidence packs; and ipc_graph for binder transaction send/receive causality."},
	    "thread": {"type":"string","description":"Thread name, substring, or ftrace/hitrace task label to resolve when pid is unknown. Accepts forms like \"com.tencent.mm-36379\", \"com.tencent.mm 36379\", \"com.tencent.mm [36379]\", \"[GT]ColdPool#5-36624\", \"binder:486_1-10803\", or \"pid=36379\"; pid is preferred when known."},
    "pid": {"type":"integer","description":"Thread pid to analyze when known."},
    "time_start": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window start in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\" or \"928.081774 秒\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "time_end": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window end in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\" or \"928.081774 秒\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "line_start": {"type":"integer","description":"Optional artifact line window start for bounded search."},
    "line_end": {"type":"integer","description":"Optional artifact line window end for bounded search."},
	    "event_types": {"type":"array","items":{"type":"string"},"x-codrax-split-string-array":true,"description":"Optional event filters such as sched_switch, sched_wakeup, sched_blocked_reason, cpu_idle, cpu_frequency, cpu_frequency_limits, cpu_constraint, clock_set_rate, block_rq_issue, block_bio_remap, binder_transaction, binder_transaction_received, binder_transaction_alloc_buf, binder_lock, binder_locked, binder_unlock, binder_reply, irq, softirq, storage, filesystem, file_io, page_cache, android_fs, f2fs, scsi, mmc, storage_latency, io_pressure, power, ability_monitor, xpower, hi_sysevent, workqueue, dma_fence. Use cpu_constraint/affinity/cpuset to inspect sched_setaffinity, sched_migrate_task, cpuset/cgroup attach, and Harmony/Donghu sched_switch next_info affinity/restricted evidence. Use file_io/page_cache with pattern=<inode or entry_name> for inode-level IO rows. This field also accepts a comma/semicolon separated string, and friendly aliases such as inode_io, pageCache, mm_filemap, cpuAffinity, schedMigrate, storageLayerLatency, irq_activity, softirq_activity, and block_io_by_inode are accepted and mapped to the matching event types."},
    "pattern": {"type":"string","description":"For event_search, optional case-insensitive literal substring matched against parsed event text, span names, thread labels, scheduler roles, resource fields, and raw-like field text. Use this for frame ids such as \"1917295\", jank ids such as \"jank_frames=7\", exact timestamps, or trace labels such as \"Choreographer#doFrame\"; it is not a regex. Start with one exact token, then add event_types/time/line/thread filters after the first hit."},
    "span_name": {"type":"string","description":"Optional trace B/E span name substring. For span_window, returns matching span windows. For wakeup_chain/root_cause_rank/evidence_pack without explicit time_start/time_end, a unique matching span derives the selected window."},
    "interaction_direction": {"type":"string","enum":["both","incoming","outgoing"],"x-codrax-enum-style-alias":true,"description":"For interaction_stats: both is default; incoming counts peers waking/calling the target, outgoing counts target waking/calling peers."},
    "recipe_name": {"type":"string","enum":["auto","sleep_root_cause","jank","runnable_delay","binder_wait","io_wait","cpu_supply"],"x-codrax-enum-style-alias":true,"description":"For view=recipe: choose a standard deterministic evidence pack. auto picks from span_name/event_types/question-shape hints; recipes remain advisory and line-backed."},
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
		return failStrictDecodeWithError(t.Name(), time.Now(), err, nil, params)
	}
	path, sourceLabel, reject := resolveTraceQuerySource(ctx, p)
	if reject != nil {
		return *reject, nil
	}
	timeStart, timeEnd, timeCaveat := normalizedTraceQueryWindow(p)
	if auto, ok := t.maybeLargeRecipeAutoWindow(ctx, p, path, sourceLabel, timeCaveat); ok {
		return auto, nil
	}
	if narrowed, ok := t.maybeLargePatternWindowedView(ctx, p, path, sourceLabel, timeCaveat); ok {
		return narrowed, nil
	}
	if discovery, ok := t.maybeLargeRecipeDiscovery(ctx, p, path, sourceLabel); ok {
		return discovery, nil
	}
	if guard, ok := t.maybeLargeTraceHeavyViewGuard(ctx, p, path, sourceLabel); ok {
		return guard, nil
	}
	if streamed, ok := t.maybeLargeEventSearchStream(ctx, p, path, sourceLabel, timeStart, timeEnd, timeCaveat); ok {
		return streamed, nil
	}
	buildStart := time.Now()
	logging.Debug("[trace_query] phase=build_index view=%s source=%s path=%s start time_start=%.6f time_end=%.6f line_start=%d line_end=%d",
		p.View, sourceLabel, path, timeStart, timeEnd, p.LineStart.Int(), p.LineEnd.Int())
	idx, err := traceQueryBuildIndex(contextFromBus(ctx), path, p, timeStart, timeEnd)
	if err != nil {
		logging.Debug("[trace_query] phase=build_index view=%s path=%s failed elapsed=%s err=%v", p.View, path, time.Since(buildStart), err)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("trace_query failed to parse %s: %v", path, err),
			Timestamp: time.Now(),
		}, nil
	}
	heapAlloc, heapSys, gcCount := traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=build_index view=%s path=%s done elapsed=%s events=%d lines=%d windowed=%v index_window=%.6f..%.6f line_window=%d..%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
		p.View, path, time.Since(buildStart), len(idx.Events), idx.ScannedLineCount, idx.Windowed, idx.IndexTimeStart, idx.IndexTimeEnd, idx.IndexLineStart, idx.IndexLineEnd, heapAlloc, heapSys, gcCount)
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, timeStart, timeEnd)
	runStart := time.Now()
	logging.Debug("[trace_query] phase=run_view view=%s path=%s start events=%d windowed=%v", q.View, path, len(idx.Events), idx.Windowed)
	result := tracequery.Run(idx, q)
	heapAlloc, heapSys, gcCount = traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=run_view view=%s path=%s done elapsed=%s evidence=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d", q.View, path, time.Since(runStart), len(result.EvidencePack), len(result.Caveats), heapAlloc, heapSys, gcCount)
	if timeCaveat != "" {
		result.Caveats = append(result.Caveats, timeCaveat)
	}
	result.Caveats = append(result.Caveats, traceQueryObjectiveExactTokenCaveats(ctx, p, result)...)
	storeStart := time.Now()
	payload, _ := json.MarshalIndent(result, "", "  ")
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	summary := traceQuerySummary(result, p, sourceLabel, payloadRef)
	preview, rawRef := StoreBlob(ctx, t.Name(), summary)
	if rawRef == "" {
		rawRef = payloadRef
	}
	logging.Debug("[trace_query] phase=store_result view=%s path=%s done elapsed=%s payload_ref=%s raw_ref=%s", q.View, path, time.Since(storeStart), payloadRef, rawRef)
	now := time.Now()
	return types.ToolResult{
		ToolName:     t.Name(),
		Success:      true,
		Summary:      preview,
		RawRef:       rawRef,
		Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now),
		Timestamp:    now,
	}, nil
}

func traceQueryBuildQuery(ctx *types.BusContext, p traceQueryParams, sourceLabel, path string, timeStart, timeEnd float64) tracequery.Query {
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
	return q
}

func (t *TraceQuery) maybeLargePatternWindowedView(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, timeCaveat string) (types.ToolResult, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < traceQueryWindowedIndexMinBytes || !traceQueryShouldAutoWindowFromPattern(p) {
		return types.ToolResult{}, false
	}
	pattern := firstNonEmptyTraceString(p.Pattern, p.SpanName)
	searchP := p
	searchP.View = "event_search"
	searchP.Pattern = pattern
	searchQ := traceQueryBuildQuery(ctx, searchP, sourceLabel, path, 0, 0)
	if searchQ.Limit < 20 {
		searchQ.Limit = 20
	}
	streamStart := time.Now()
	logging.Debug("[trace_query] phase=auto_window_search view=%s source=%s path=%s start pattern=%s",
		p.View, sourceLabel, path, pattern)
	searchResult, err := tracequery.StreamEventSearch(contextFromBus(ctx), path, searchQ)
	if err != nil {
		logging.Debug("[trace_query] phase=auto_window_search view=%s path=%s failed elapsed=%s err=%v", p.View, path, time.Since(streamStart), err)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("trace_query failed to locate %s in %s: %v", pattern, path, err),
			Timestamp: time.Now(),
		}, true
	}
	heapAlloc, heapSys, gcCount := traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=auto_window_search view=%s path=%s done elapsed=%s matched=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
		p.View, path, time.Since(streamStart), len(searchResult.Events), len(searchResult.Caveats), heapAlloc, heapSys, gcCount)
	candidates := traceQueryAutoWindowCandidatesFromEvents(p, searchResult.Events, traceQueryAutoWindowMaxCandidates)
	if len(candidates) == 0 {
		searchResult.Caveats = append(searchResult.Caveats,
			fmt.Sprintf("auto_window_from_pattern=false; no timestamped event matched pattern %q for view=%s", pattern, firstNonEmptyTraceString(p.View, "frame_window")))
		payload, _ := json.MarshalIndent(searchResult, "", "  ")
		payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
		summary := traceQuerySummary(searchResult, searchP, sourceLabel, payloadRef)
		preview, rawRef := StoreBlob(ctx, t.Name(), summary)
		if rawRef == "" {
			rawRef = payloadRef
		}
		now := time.Now()
		return types.ToolResult{
			ToolName:     t.Name(),
			Success:      true,
			Summary:      preview,
			RawRef:       rawRef,
			Observations: traceQueryTypedObservations(searchResult, sourceLabel, payloadRef, rawRef, "", now),
			Timestamp:    now,
		}, true
	}
	if traceQueryShouldRunMultiplePatternWindows(p, len(candidates)) {
		return t.runAutoWindowCandidates(ctx, p, path, sourceLabel, "large_trace_pattern_auto_windows", candidates, timeCaveat), true
	}
	start, end := candidates[0].Start, candidates[0].End
	boundedP := p
	boundedP.TimeStart = traceSecondFromAutoWindow(start)
	boundedP.TimeEnd = traceSecondFromAutoWindow(end)
	buildStart := time.Now()
	logging.Debug("[trace_query] phase=auto_window_build view=%s path=%s start pattern=%s time_start=%.6f time_end=%.6f matches=%d",
		p.View, path, pattern, start, end, len(searchResult.Events))
	idx, err := traceQueryBuildIndex(contextFromBus(ctx), path, boundedP, start, end)
	if err != nil {
		logging.Debug("[trace_query] phase=auto_window_build view=%s path=%s failed elapsed=%s err=%v", p.View, path, time.Since(buildStart), err)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("trace_query failed to parse auto-window %.6f..%.6f in %s: %v", start, end, path, err),
			Timestamp: time.Now(),
		}, true
	}
	heapAlloc, heapSys, gcCount = traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=auto_window_build view=%s path=%s done elapsed=%s events=%d lines=%d windowed=%v heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
		p.View, path, time.Since(buildStart), len(idx.Events), idx.ScannedLineCount, idx.Windowed, heapAlloc, heapSys, gcCount)
	q := traceQueryBuildQuery(ctx, boundedP, sourceLabel, path, start, end)
	runStart := time.Now()
	result := tracequery.Run(idx, q)
	heapAlloc, heapSys, gcCount = traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=auto_window_run view=%s path=%s done elapsed=%s events=%d evidence=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
		q.View, path, time.Since(runStart), len(idx.Events), len(result.EvidencePack), len(result.Caveats), heapAlloc, heapSys, gcCount)
	result.Caveats = append(result.Caveats,
		fmt.Sprintf("auto_window_from_pattern=true; pattern %q matched %d event(s), then ran %s in %.6f..%.6f seconds without building a full trace index",
			pattern, len(searchResult.Events), firstNonEmptyTraceString(p.View, "frame_window"), start, end))
	if timeCaveat != "" {
		result.Caveats = append(result.Caveats, timeCaveat)
	}
	result.Caveats = append(result.Caveats, traceQueryObjectiveExactTokenCaveats(ctx, p, result)...)
	storeStart := time.Now()
	payload, _ := json.MarshalIndent(result, "", "  ")
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	summary := traceQuerySummary(result, boundedP, sourceLabel, payloadRef)
	preview, rawRef := StoreBlob(ctx, t.Name(), summary)
	if rawRef == "" {
		rawRef = payloadRef
	}
	logging.Debug("[trace_query] phase=store_result view=%s path=%s done elapsed=%s payload_ref=%s raw_ref=%s", q.View, path, time.Since(storeStart), payloadRef, rawRef)
	now := time.Now()
	return types.ToolResult{
		ToolName:     t.Name(),
		Success:      true,
		Summary:      preview,
		RawRef:       rawRef,
		Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now),
		Timestamp:    now,
	}, true
}

func traceQueryShouldAutoWindowFromPattern(p traceQueryParams) bool {
	if traceQueryHasExplicitIndexWindow(p) {
		return false
	}
	if strings.TrimSpace(firstNonEmptyTraceString(p.Pattern, p.SpanName)) == "" {
		return false
	}
	return traceQueryPatternWindowableHeavyView(p.View)
}

func traceQueryPatternWindowableHeavyView(view string) bool {
	switch strings.TrimSpace(view) {
	case "span_window", "frame_window", "render_pipeline", "frame_timeline", "frame_flow",
		"thread_timeline", "scheduler_latency_stats", "root_cause_rank", "window_stats", "critical_blocking_calls",
		"ipc_graph", "wakeup_chain", "frame_root_cause_bundle", "interaction_stats", "evidence_pack", "recipe":
		return true
	default:
		return false
	}
}

const traceQueryAutoWindowMaxCandidates = 3

type traceQueryAutoWindowCandidate struct {
	Rank    int     `json:"rank"`
	Source  string  `json:"source,omitempty"`
	Token   string  `json:"token,omitempty"`
	Ts      float64 `json:"ts,omitempty"`
	Line    int     `json:"line,omitempty"`
	Start   float64 `json:"time_start"`
	End     float64 `json:"time_end"`
	Primary bool    `json:"primary,omitempty"`
	Raw     string  `json:"raw,omitempty"`
}

type traceQueryAutoWindowChild struct {
	Candidate traceQueryAutoWindowCandidate `json:"candidate"`
	Result    tracequery.Result             `json:"result,omitempty"`
	Error     string                        `json:"error,omitempty"`
}

func traceQueryAutoWindowFromEvents(p traceQueryParams, events []tracequery.EventView) (float64, float64, bool) {
	candidates := traceQueryAutoWindowCandidatesFromEvents(p, events, 1)
	if len(candidates) == 0 {
		return 0, 0, false
	}
	return candidates[0].Start, candidates[0].End, true
}

func traceQueryAutoWindowCandidatesFromEvents(p traceQueryParams, events []tracequery.EventView, max int) []traceQueryAutoWindowCandidate {
	if max <= 0 {
		max = traceQueryAutoWindowMaxCandidates
	}
	token := firstNonEmptyTraceString(p.Pattern, p.SpanName)
	candidates := make([]traceQueryAutoWindowCandidate, 0, max)
	for _, ev := range events {
		candidate, ok := traceQueryAutoWindowCandidateForTimestamp(p, "event_search", token, ev.Ts, ev.Line, false, ev.Raw)
		if !ok || traceQueryAutoWindowCandidateIsDuplicate(candidates, candidate) {
			continue
		}
		candidates = append(candidates, candidate)
		if len(candidates) >= max {
			break
		}
	}
	traceQueryRankAutoWindowCandidates(candidates)
	return candidates
}

func traceQueryAutoWindowCandidatesFromMarkers(p traceQueryParams, markers []traceQueryRecipeDiscoveryMarker, max int) []traceQueryAutoWindowCandidate {
	if max <= 0 {
		max = traceQueryAutoWindowMaxCandidates
	}
	ordered := append([]traceQueryRecipeDiscoveryMarker(nil), markers...)
	hasSpecificMarker := false
	for _, marker := range ordered {
		if traceQueryRecipeMarkerPriority(marker) <= 3 {
			hasSpecificMarker = true
			break
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		pi := traceQueryRecipeMarkerPriority(ordered[i])
		pj := traceQueryRecipeMarkerPriority(ordered[j])
		if pi != pj {
			return pi < pj
		}
		if ordered[i].Line != ordered[j].Line {
			return ordered[i].Line < ordered[j].Line
		}
		return ordered[i].Ts < ordered[j].Ts
	})
	candidates := make([]traceQueryAutoWindowCandidate, 0, max)
	for _, marker := range ordered {
		if hasSpecificMarker && traceQueryRecipeMarkerPriority(marker) >= 4 {
			continue
		}
		candidate, ok := traceQueryAutoWindowCandidateForTimestamp(p, "recipe_marker", marker.Token, marker.Ts, marker.Line, marker.Primary, marker.Raw)
		if !ok || traceQueryAutoWindowCandidateIsDuplicate(candidates, candidate) {
			continue
		}
		candidates = append(candidates, candidate)
		if len(candidates) >= max {
			break
		}
	}
	traceQueryRankAutoWindowCandidates(candidates)
	return candidates
}

func traceQueryAutoWindowCandidateForTimestamp(p traceQueryParams, source, token string, ts float64, line int, primary bool, raw string) (traceQueryAutoWindowCandidate, bool) {
	if ts <= 0 {
		return traceQueryAutoWindowCandidate{}, false
	}
	before, after := traceQueryAutoWindowPadding(p.View)
	start := ts - before
	if start < 0 {
		start = 0
	}
	end := ts + after
	if end <= start {
		end = start + after
	}
	return traceQueryAutoWindowCandidate{
		Source:  source,
		Token:   token,
		Ts:      ts,
		Line:    line,
		Start:   start,
		End:     end,
		Primary: primary,
		Raw:     truncateForLog(raw, 500),
	}, true
}

func traceQueryAutoWindowCandidateIsDuplicate(existing []traceQueryAutoWindowCandidate, candidate traceQueryAutoWindowCandidate) bool {
	for _, prev := range existing {
		delta := candidate.Ts - prev.Ts
		if delta < 0 {
			delta = -delta
		}
		if delta <= 0.001 && strings.EqualFold(strings.TrimSpace(candidate.Token), strings.TrimSpace(prev.Token)) {
			return true
		}
		if candidate.Line > 0 && candidate.Line == prev.Line {
			return true
		}
	}
	return false
}

func traceQueryRankAutoWindowCandidates(candidates []traceQueryAutoWindowCandidate) {
	for i := range candidates {
		candidates[i].Rank = i + 1
	}
}

func traceQueryRecipeMarkerPriority(marker traceQueryRecipeDiscoveryMarker) int {
	token := strings.ToLower(strings.TrimSpace(marker.Token))
	raw := strings.ToLower(strings.TrimSpace(marker.Raw))
	switch {
	case marker.Primary:
		return 0
	case strings.Contains(token, "jank_frames") || strings.Contains(raw, "jank_frames"):
		return 1
	case strings.Contains(token, "actualtimeline") || strings.Contains(token, "expectedtimeline") ||
		strings.Contains(raw, "actualtimeline") || strings.Contains(raw, "expectedtimeline"):
		return 2
	case strings.Contains(token, "choreographer") || strings.Contains(token, "renderframe") ||
		strings.Contains(raw, "choreographer") || strings.Contains(raw, "renderframe"):
		return 3
	case strings.Contains(token, "jank") || strings.Contains(raw, "jank"):
		return 4
	default:
		return 5
	}
}

func traceQueryShouldRunMultiplePatternWindows(p traceQueryParams, count int) bool {
	if count <= 1 {
		return false
	}
	switch strings.TrimSpace(p.View) {
	case "span_window", "frame_window", "render_pipeline", "frame_timeline", "frame_flow",
		"thread_timeline", "ipc_graph", "wakeup_chain", "frame_root_cause_bundle", "interaction_stats", "recipe":
		return true
	default:
		return false
	}
}

func traceQueryAutoWindowPadding(view string) (float64, float64) {
	switch strings.TrimSpace(view) {
	case "span_window":
		return 0.250, 2.000
	case "frame_window", "render_pipeline", "frame_timeline", "frame_flow", "frame_root_cause_bundle":
		return 0.250, 2.000
	case "recipe":
		return 0.500, 2.000
	default:
		return 0.250, 1.000
	}
}

func (t *TraceQuery) runAutoWindowCandidates(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, mode string, candidates []traceQueryAutoWindowCandidate, timeCaveat string) types.ToolResult {
	children := make([]traceQueryAutoWindowChild, 0, len(candidates))
	for _, candidate := range candidates {
		boundedP := p
		boundedP.TimeStart = traceSecondFromAutoWindow(candidate.Start)
		boundedP.TimeEnd = traceSecondFromAutoWindow(candidate.End)
		logging.Debug("[trace_query] phase=auto_window_candidate_build mode=%s view=%s path=%s rank=%d token=%s time_start=%.6f time_end=%.6f",
			mode, p.View, path, candidate.Rank, candidate.Token, candidate.Start, candidate.End)
		idx, err := traceQueryBuildIndex(contextFromBus(ctx), path, boundedP, candidate.Start, candidate.End)
		if err != nil {
			logging.Debug("[trace_query] phase=auto_window_candidate_build mode=%s view=%s path=%s rank=%d failed err=%v", mode, p.View, path, candidate.Rank, err)
			children = append(children, traceQueryAutoWindowChild{Candidate: candidate, Error: err.Error()})
			continue
		}
		heapAlloc, heapSys, gcCount := traceQueryMemoryForLog()
		logging.Debug("[trace_query] phase=auto_window_candidate_build mode=%s view=%s path=%s rank=%d done events=%d lines=%d windowed=%v heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
			mode, p.View, path, candidate.Rank, len(idx.Events), idx.ScannedLineCount, idx.Windowed, heapAlloc, heapSys, gcCount)
		q := traceQueryBuildQuery(ctx, boundedP, sourceLabel, path, candidate.Start, candidate.End)
		runStart := time.Now()
		result := tracequery.Run(idx, q)
		heapAlloc, heapSys, gcCount = traceQueryMemoryForLog()
		logging.Debug("[trace_query] phase=auto_window_candidate_run mode=%s view=%s path=%s rank=%d done elapsed=%s events=%d evidence=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
			mode, q.View, path, candidate.Rank, time.Since(runStart), len(idx.Events), len(result.EvidencePack), len(result.Caveats), heapAlloc, heapSys, gcCount)
		result.Caveats = append(result.Caveats,
			fmt.Sprintf("auto_window_candidate=true; mode=%s rank=%d source=%s token=%q line=%d ts=%.6f window=%.6f..%.6f seconds",
				mode, candidate.Rank, candidate.Source, candidate.Token, candidate.Line, candidate.Ts, candidate.Start, candidate.End))
		if timeCaveat != "" {
			result.Caveats = append(result.Caveats, timeCaveat)
		}
		children = append(children, traceQueryAutoWindowChild{Candidate: candidate, Result: result})
	}
	payload := map[string]any{
		"mode":           mode,
		"source_path":    path,
		"source":         sourceLabel,
		"requested_view": firstNonEmptyTraceString(p.View, "recipe"),
		"recipe_name":    p.RecipeName,
		"candidates":     candidates,
		"results":        children,
	}
	payloadBytes, _ := json.MarshalIndent(payload, "", "  ")
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-auto-windows.json", string(payloadBytes))
	summary := traceQueryAutoWindowSummary(path, sourceLabel, p, mode, children, payloadRef)
	preview, rawRef := StoreBlob(ctx, t.Name(), summary)
	if rawRef == "" {
		rawRef = payloadRef
	}
	now := time.Now()
	var observations []types.ObservationRecord
	for _, child := range children {
		if child.Error != "" {
			continue
		}
		observations = append(observations, traceQueryTypedObservations(
			child.Result, sourceLabel, payloadRef, rawRef,
			fmt.Sprintf("w%d", child.Candidate.Rank), now)...)
	}
	return types.ToolResult{
		ToolName:     t.Name(),
		Success:      true,
		Summary:      preview,
		RawRef:       rawRef,
		Observations: observations,
		Timestamp:    now,
	}
}

func traceSecondFromAutoWindow(seconds float64) TraceSecond {
	return TraceSecond{
		seconds:        seconds,
		set:            true,
		raw:            fmt.Sprintf("%.6f", seconds),
		unit:           "s",
		fractionDigits: 6,
		scale:          1,
	}
}

func (t *TraceQuery) maybeLargeEventSearchStream(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string, timeStart, timeEnd float64, timeCaveat string) (types.ToolResult, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < traceQueryWindowedIndexMinBytes || !traceQueryShouldStreamEventSearch(p) {
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, timeStart, timeEnd)
	streamStart := time.Now()
	logging.Debug("[trace_query] phase=stream_event_search view=%s source=%s path=%s start pattern=%s event_types=%d",
		q.View, sourceLabel, path, p.Pattern, len(q.EventTypes))
	result, err := tracequery.StreamEventSearch(contextFromBus(ctx), path, q)
	if err != nil {
		logging.Debug("[trace_query] phase=stream_event_search view=%s path=%s failed elapsed=%s err=%v", q.View, path, time.Since(streamStart), err)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("trace_query failed to stream-search %s: %v", path, err),
			Timestamp: time.Now(),
		}, true
	}
	heapAlloc, heapSys, gcCount := traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=stream_event_search view=%s path=%s done elapsed=%s matched=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
		q.View, path, time.Since(streamStart), len(result.Events), len(result.Caveats), heapAlloc, heapSys, gcCount)
	if timeCaveat != "" {
		result.Caveats = append(result.Caveats, timeCaveat)
	}
	result.Caveats = append(result.Caveats, traceQueryObjectiveExactTokenCaveats(ctx, p, result)...)
	storeStart := time.Now()
	payload, _ := json.MarshalIndent(result, "", "  ")
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	summary := traceQuerySummary(result, p, sourceLabel, payloadRef)
	preview, rawRef := StoreBlob(ctx, t.Name(), summary)
	if rawRef == "" {
		rawRef = payloadRef
	}
	logging.Debug("[trace_query] phase=store_result view=%s path=%s done elapsed=%s payload_ref=%s raw_ref=%s", q.View, path, time.Since(storeStart), payloadRef, rawRef)
	now := time.Now()
	return types.ToolResult{
		ToolName:     t.Name(),
		Success:      true,
		Summary:      preview,
		RawRef:       rawRef,
		Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now),
		Timestamp:    now,
	}, true
}

func traceQueryShouldStreamEventSearch(p traceQueryParams) bool {
	view := strings.TrimSpace(p.View)
	if view != "" && view != "event_search" {
		return false
	}
	return !traceQueryHasExplicitIndexWindow(p)
}

func traceQueryBuildIndex(ctx context.Context, path string, p traceQueryParams, timeStart, timeEnd float64) (*tracequery.Index, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() < traceQueryWindowedIndexMinBytes || !traceQueryHasExplicitIndexWindow(p) {
		return tracequery.BuildIndex(ctx, path)
	}
	opts := tracequery.BuildOptions{
		TimeStart:          timeStart,
		TimeEnd:            timeEnd,
		TimeStartSet:       p.TimeStart.Set(),
		TimeEndSet:         p.TimeEnd.Set(),
		TimePaddingBefore:  traceQueryWindowedIndexTimePadding(p),
		TimePaddingAfter:   traceQueryWindowedIndexTimePadding(p),
		LineStart:          p.LineStart.Int(),
		LineEnd:            p.LineEnd.Int(),
		LinePaddingBefore:  200,
		LinePaddingAfter:   200,
		AllowWindowedParse: true,
	}
	return tracequery.BuildIndexWithOptions(ctx, path, opts)
}

func traceQueryHasExplicitIndexWindow(p traceQueryParams) bool {
	return p.TimeStart.Set() || p.TimeEnd.Set() || p.LineStart.Int() > 0 || p.LineEnd.Int() > 0
}

func traceQueryWindowedIndexTimePadding(p traceQueryParams) float64 {
	view := strings.TrimSpace(p.View)
	switch view {
	case "event_search":
		return 0.050
	case "thread_timeline", "scheduler_latency_stats":
		return 0.250
	default:
		return 0.500
	}
}

func (t *TraceQuery) maybeLargeTraceHeavyViewGuard(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string) (types.ToolResult, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < traceQueryWindowedIndexMinBytes || !traceQueryIsHeavyView(p.View) ||
		traceQueryHasBoundedTraceScope(p) || traceQueryHasPatternOrSpanScope(p) {
		return types.ToolResult{}, false
	}
	summary := traceQueryHeavyViewGuardSummary(path, sourceLabel, p, info.Size())
	preview, rawRef := StoreBlob(ctx, t.Name(), summary)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   preview,
		RawRef:    rawRef,
		Timestamp: time.Now(),
	}, true
}

func traceQueryIsHeavyView(view string) bool {
	switch strings.TrimSpace(view) {
	case "scheduler_latency_stats", "root_cause_rank", "window_stats", "critical_blocking_calls", "evidence_pack", "recipe",
		"span_window", "frame_window", "render_pipeline", "frame_timeline", "frame_flow", "frame_root_cause_bundle",
		"thread_timeline", "ipc_graph", "wakeup_chain", "interaction_stats":
		return true
	default:
		return false
	}
}

// traceQueryHasPatternOrSpanScope reports whether the call carries a
// pattern/span selector a narrowing path can use (the auto-window path for
// its views, or span-derived windows inside tracequery.Run). Such calls are
// not genuinely unbounded, so the heavy-view guard must let them run.
func traceQueryHasPatternOrSpanScope(p traceQueryParams) bool {
	return strings.TrimSpace(firstNonEmptyTraceString(p.Pattern, p.SpanName)) != ""
}

func traceQueryHasBoundedTraceScope(p traceQueryParams) bool {
	return p.TimeStart.Set() ||
		p.TimeEnd.Set() ||
		p.LineStart.Int() > 0 ||
		p.LineEnd.Int() > 0
}

func traceQueryHeavyViewGuardSummary(path, sourceLabel string, p traceQueryParams, size int64) string {
	view := firstNonEmptyTraceString(p.View, "window_stats")
	var b strings.Builder
	fmt.Fprintf(&b, "[trace_query params: view=%s source=%s path=%s origin=runtime_artifact artifact_id=%s artifact_kind=trace mode=large_trace_heavy_view_guard thread=%s pid=%s pattern=%s span_name=%s platform=%s trace_flavor=%s]\n",
		sanitizeForBanner(view),
		sourceLabel,
		sanitizeForBanner(path),
		traceQueryArtifactID(sourceLabel),
		sanitizeForBanner(p.Thread),
		positiveIntBannerValue(p.PID.Int()),
		sanitizeForBanner(p.Pattern),
		sanitizeForBanner(p.SpanName),
		sanitizeForBanner(p.Platform),
		sanitizeForBanner(p.TraceFlavor),
	)
	b.WriteString("# Trace Query: large trace guard\n\n")
	fmt.Fprintf(&b, "source=%s size_bytes=%d requested_view=%s\n", sanitizeForBanner(path), size, sanitizeForBanner(view))
	b.WriteString("guard=heavy trace view was not expanded without a bounded time/line/span/pattern scope. A thread or pid alone can still require scanning millions of scheduler events.\n")
	b.WriteString("next_call_hint=first narrow the trace with trace_query(view=\"event_search\", pattern=\"<frame id / jank id / exact timestamp / span label>\", event_types=[\"trace_mark\"], limit=40), or trace_query(view=\"span_window\", span_name=\"<span label>\"), then rerun this same view with time_start/time_end or line_start/line_end.\n")
	if p.PID.Int() > 0 {
		fmt.Fprintf(&b, "target_hint=after selecting a window, rerun trace_query(view=%q, pid=%d, time_start=<seconds>, time_end=<seconds>).\n", sanitizeForBanner(view), p.PID.Int())
	} else if strings.TrimSpace(p.Thread) != "" {
		fmt.Fprintf(&b, "target_hint=after selecting a window, rerun trace_query(view=%q, thread=%q, time_start=<seconds>, time_end=<seconds>).\n", sanitizeForBanner(view), sanitizeForBanner(p.Thread))
	}
	b.WriteString("window_carryover_hint=if a previous trace_query result already showed a selected_window, keep that same time_start/time_end on subsequent scheduler/root-cause/resource views.\n")
	return b.String()
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
		item = normalizeTraceQueryEventTypeToken(item)
		if item == "" {
			continue
		}
		out = append(out, tracequery.EventType(item))
	}
	return out
}

func normalizeTraceQueryEventTypeToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	prevLowerOrDigit := false
	for _, r := range raw {
		switch {
		case r == '-' || r == ' ' || r == '.':
			b.WriteByte('_')
			prevLowerOrDigit = false
		case r >= 'A' && r <= 'Z':
			if prevLowerOrDigit {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
			prevLowerOrDigit = false
		default:
			b.WriteRune(r)
			prevLowerOrDigit = (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		}
	}
	token := strings.ToLower(strings.Trim(b.String(), "_"))
	switch token {
	case "inode", "inode_io", "file_inode", "file_inode_io":
		return "file_io"
	case "pagecache", "filemap", "mm_filemap":
		return "page_cache"
	case "androidfs":
		return "android_fs"
	case "storage_layer", "storage_layer_latency":
		return "storage_latency"
	case "interrupt", "interrupts", "irq_activity":
		return "irq"
	case "soft_irq", "softirq_activity":
		return "softirq"
	case "block_inode", "block_io_inode", "block_io_by_inode":
		return "storage_latency"
	case "affinity", "cpu_affinity", "cpuaffinity", "cpuset", "sched_migrate", "sched_migration", "migration", "cpu_constraint", "cpu_constraints":
		return "cpu_constraint"
	default:
		return token
	}
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

func (t *TraceQuery) maybeLargeRecipeAutoWindow(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, timeCaveat string) (types.ToolResult, bool) {
	info, err := os.Stat(path)
	if err != nil || !traceQueryShouldUseLargeRecipeDiscovery(p, info.Size()) {
		return types.ToolResult{}, false
	}
	markers, _, _, scanErr := scanTraceQueryRecipeMarkers(contextFromBus(ctx), path, traceQueryRecipeDiscoveryTokens(ctx, p), 48)
	if scanErr != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("trace_query recipe auto-window discovery failed for %s: %v", path, scanErr),
			Timestamp: time.Now(),
		}, true
	}
	candidates := traceQueryAutoWindowCandidatesFromMarkers(p, markers, traceQueryAutoWindowMaxCandidates)
	if len(candidates) == 0 {
		return types.ToolResult{}, false
	}
	return t.runAutoWindowCandidates(ctx, p, path, sourceLabel, "large_trace_recipe_auto_windows", candidates, timeCaveat), true
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

type traceQueryObjectiveExactToken struct {
	Token string
	Kind  string
}

func traceQueryObjectiveExactTokenCaveats(ctx *types.BusContext, p traceQueryParams, result tracequery.Result) []string {
	view := strings.TrimSpace(result.View)
	switch view {
	case "event_search", "span_window", "frame_window", "render_pipeline", "frame_timeline", "frame_flow":
	default:
		return nil
	}
	tokens := traceQueryObjectiveExactTokens(ctx)
	if len(tokens) == 0 {
		return nil
	}
	selector := firstNonEmptyTraceString(p.Pattern, p.SpanName)
	var out []string
	for _, token := range tokens {
		if traceQuerySelectorContainsObjectiveToken(selector, token.Token) || traceQueryResultContainsObjectiveToken(result, token.Token) {
			continue
		}
		selectorName := traceQueryObjectiveSelectorName(p)
		switch token.Kind {
		case "frame":
			out = append(out, fmt.Sprintf("objective_exact_frame_hint=user request names Choreographer#doFrame %s, but this %s %s %q does not include requested token %q; returned rows are not evidence that frame %s is absent. Rerun trace_query(view=\"frame_window\", pattern=%q, event_types=[\"trace_mark\"]) or trace_query(view=\"event_search\", pattern=%q, event_types=[\"trace_mark\"]) before making an absence claim",
				token.Token, firstNonEmptyTraceString(view, "event_search"), selectorName, selector, token.Token, token.Token, token.Token, token.Token))
		case "span", "quoted":
			out = append(out, fmt.Sprintf("objective_exact_span_hint=user request names exact span/marker token %q, but this %s %s %q does not include it and the returned rows do not contain it; do not infer absence from this broad result. Rerun trace_query(view=\"span_window\", span_name=%q) or trace_query(view=\"event_search\", pattern=%q, event_types=[\"trace_mark\"]) before making an absence claim",
				token.Token, firstNonEmptyTraceString(view, "event_search"), selectorName, selector, token.Token, token.Token))
		default:
			out = append(out, fmt.Sprintf("objective_exact_token_hint=user request names exact token %q (kind=%s), but this %s %s %q does not include it and the returned rows do not contain it; do not infer absence from this broad result. Rerun trace_query(view=\"event_search\", pattern=%q) or an appropriate specialized view with that exact literal token before making an absence claim",
				token.Token, token.Kind, firstNonEmptyTraceString(view, "event_search"), selectorName, selector, token.Token))
		}
	}
	return out
}

func traceQueryObjectiveSelectorName(p traceQueryParams) string {
	if strings.TrimSpace(p.Pattern) != "" {
		return "pattern"
	}
	if strings.TrimSpace(p.SpanName) != "" {
		return "span_name"
	}
	return "selector"
}

func traceQueryObjectiveExactTokens(ctx *types.BusContext) []traceQueryObjectiveExactToken {
	text := traceQueryObjectiveText(ctx)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var out []traceQueryObjectiveExactToken
	seen := map[string]bool{}
	add := func(raw, kind string) {
		token := traceQueryNormalizeObjectiveToken(raw)
		if token == "" || !traceQueryObjectiveTokenAllowed(token) {
			return
		}
		key := strings.ToLower(token)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, traceQueryObjectiveExactToken{Token: token, Kind: kind})
	}
	for _, m := range traceQueryObjectiveFrameIDRE.FindAllStringSubmatch(text, 8) {
		if len(m) >= 2 {
			add(m[1], "frame")
		}
	}
	for _, m := range traceQueryObjectiveQuotedTokenRE.FindAllStringSubmatch(text, 12) {
		for i := 1; i < len(m); i++ {
			if strings.TrimSpace(m[i]) != "" {
				add(m[i], "quoted")
				break
			}
		}
	}
	for _, token := range traceQueryObjectiveKVTokenRE.FindAllString(text, 16) {
		add(token, "kv")
	}
	for _, token := range traceQueryObjectiveHexTokenRE.FindAllString(text, 12) {
		add(token, "hex")
	}
	for _, re := range []*regexp.Regexp{traceQueryObjectiveLabeledTokenRE, traceQueryObjectivePreLabeledTokenRE} {
		for _, m := range re.FindAllStringSubmatch(text, 12) {
			if len(m) >= 2 {
				add(m[1], "span")
			}
		}
	}
	return out
}

func traceQueryNormalizeObjectiveToken(raw string) string {
	token := strings.TrimSpace(raw)
	token = strings.Trim(token, " \t\r\n,，.。;；:：()（）[]【】{}<>《》\"'“”‘’")
	return token
}

func traceQueryObjectiveTokenAllowed(token string) bool {
	if len([]rune(token)) < 3 {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(token))
	switch lower {
	case "trace", "systrace", "hitrace", "atrace", "span", "marker", "label", "keyword", "event_search", "frame_window", "span_window":
		return false
	}
	if strings.HasSuffix(lower, ".systrace") || strings.HasSuffix(lower, ".htrace") || strings.HasSuffix(lower, ".trace") {
		return false
	}
	return true
}

func traceQuerySelectorContainsObjectiveToken(selector, token string) bool {
	selector = strings.ToLower(strings.TrimSpace(selector))
	token = strings.ToLower(strings.TrimSpace(token))
	return selector != "" && token != "" && strings.Contains(selector, token)
}

func traceQueryResultContainsObjectiveToken(result tracequery.Result, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	contains := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), token) {
				return true
			}
		}
		return false
	}
	for _, ev := range result.Events {
		if contains(ev.Raw, ev.SpanName, ev.FieldText, ev.Comm, ev.PrevComm, ev.NextComm, ev.WakeeComm) {
			return true
		}
	}
	for _, span := range result.SpanWindows {
		if contains(span.Name) {
			return true
		}
	}
	if result.FramePipeline != nil {
		for _, item := range result.FramePipeline.Items {
			if contains(item.Name, item.Summary) {
				return true
			}
		}
	}
	if result.FrameTimeline != nil {
		for _, item := range result.FrameTimeline.Items {
			if contains(item.Name, item.FrameID, item.Summary) {
				return true
			}
		}
	}
	return false
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

func traceQueryAutoWindowSummary(path, sourceLabel string, p traceQueryParams, mode string, children []traceQueryAutoWindowChild, payloadRef string) string {
	view := firstNonEmptyTraceString(p.View, "recipe")
	var b strings.Builder
	fmt.Fprintf(&b, "[trace_query params: view=%s source=%s path=%s origin=runtime_artifact artifact_id=%s artifact_kind=trace thread=%s pid=%s pattern=%s span_name=%s recipe_name=%s mode=%s platform=%s trace_flavor=%s payload_ref=%s]\n",
		sanitizeForBanner(view),
		sourceLabel,
		sanitizeForBanner(path),
		traceQueryArtifactID(sourceLabel),
		sanitizeForBanner(p.Thread),
		positiveIntBannerValue(p.PID.Int()),
		sanitizeForBanner(p.Pattern),
		sanitizeForBanner(p.SpanName),
		sanitizeForBanner(p.RecipeName),
		sanitizeForBanner(mode),
		sanitizeForBanner(p.Platform),
		sanitizeForBanner(p.TraceFlavor),
		sanitizeForBanner(payloadRef),
	)
	b.WriteString("# Trace Query: auto window candidates\n\n")
	fmt.Fprintf(&b, "source=%s requested_view=%s candidate_windows=%d mode=%s\n", sanitizeForBanner(path), sanitizeForBanner(view), len(children), sanitizeForBanner(mode))
	if payloadRef != "" {
		fmt.Fprintf(&b, "payload_ref=%s\n", sanitizeForBanner(payloadRef))
	}
	b.WriteString("auto_window_policy=lightweight discovery selected timestamped marker/span/frame matches, then each candidate was analyzed with a bounded time window and a windowed index.\n")
	if len(children) > 0 {
		b.WriteString("\n## Candidate windows\n")
		for _, child := range children {
			candidate := child.Candidate
			primary := ""
			if candidate.Primary {
				primary = " primary=true"
			}
			fmt.Fprintf(&b, "- rank=%d source=%s token=%s%s line=%d ts=%.6f time_start=%.6f time_end=%.6f raw=%s\n",
				candidate.Rank,
				sanitizeForBanner(candidate.Source),
				sanitizeForBanner(candidate.Token),
				primary,
				candidate.Line,
				candidate.Ts,
				candidate.Start,
				candidate.End,
				sanitizeForBanner(candidate.Raw),
			)
		}
		b.WriteString("\n")
	}
	for _, child := range children {
		candidate := child.Candidate
		fmt.Fprintf(&b, "## Candidate %d\n", candidate.Rank)
		fmt.Fprintf(&b, "candidate_window=%.6f..%.6f seconds token=%s line=%d ts=%.6f source=%s\n",
			candidate.Start,
			candidate.End,
			sanitizeForBanner(candidate.Token),
			candidate.Line,
			candidate.Ts,
			sanitizeForBanner(candidate.Source),
		)
		if child.Error != "" {
			fmt.Fprintf(&b, "candidate_error=%s\n\n", sanitizeForBanner(child.Error))
			continue
		}
		boundedP := p
		boundedP.TimeStart = traceSecondFromAutoWindow(candidate.Start)
		boundedP.TimeEnd = traceSecondFromAutoWindow(candidate.End)
		childSummary := traceQuerySummary(child.Result, boundedP, sourceLabel, "")
		b.WriteString(childSummary)
		if !strings.HasSuffix(childSummary, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
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
	if result.IndexWindowed {
		fmt.Fprintf(&b, "index_windowed=true scanned_lines=%d index_time=%.6f..%.6f index_lines=%d..%d\n", result.ScannedLineCount, result.IndexTimeStart, result.IndexTimeEnd, result.IndexLineStart, result.IndexLineEnd)
	}
	if result.UnparsedLineCount > 0 || result.ParseLinePanics > 0 || result.ClockRegressions > 0 {
		fmt.Fprintf(&b, "scanned_lines=%d parsed_events=%d unparsed_lines=%d parse_line_panics=%d clock_regressions=%d\n",
			result.ScannedLineCount, result.EventCount, result.UnparsedLineCount, result.ParseLinePanics, result.ClockRegressions)
	}
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
	if result.FrameRootCauseBundle != nil {
		writeTraceFrameRootCauseBundleSummary(&b, result.FrameRootCauseBundle)
	}
	if result.WakeupChain != nil {
		b.WriteString("## Wakeup chain\n")
		if path := traceQueryWakeupChainPath(*result.WakeupChain); path != "" {
			fmt.Fprintf(&b, "- wakeup_chain path=%s\n", sanitizeForBanner(path))
		}
		for _, edge := range result.WakeupChain.Edges {
			fmt.Fprintf(&b, "- %s -> %s at %.6f line %d (latency %.3fms) waker_prio=%d/%s wakee_prio=%d/%s relation=%s priority_inversion_candidate=%t\n",
				traceThreadLabel(edge.Waker), traceThreadLabel(edge.Wakee), edge.WakeupTs, edge.WakeupLine, edge.LatencyMs,
				edge.WakerPriority, sanitizeForBanner(edge.WakerPriorityClass), edge.WakeePriority, sanitizeForBanner(edge.WakeePriorityClass),
				sanitizeForBanner(edge.PriorityRelation), edge.PriorityInversionCandidate)
		}
		for _, impact := range result.WakeupChain.CausalImpacts {
			fmt.Fprintf(&b, "- causal_impact thread=%s depth=%d causality=%s dominant_state=%s impact=%.3fms total=%.3fms target_impact=%.3fms fragments=%d switches=%d max_segment=%.3fms p95_segment=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms prio=%d/%s target_prio=%d/%s priority_relation=%s priority_inversion_candidate=%t lines=%d-%d — %s\n",
				traceThreadLabel(impact.Thread), impact.ChainDepth, traceQueryCausalityLabel(impact.OnChain),
				sanitizeForBanner(impact.DominantState), impact.DominantImpactMs, impact.TotalMs, impact.TargetBlockedMs,
				impact.FragmentCount, impact.StateSwitches, impact.MaxSegmentMs, impact.P95SegmentMs,
				impact.RunningMs, impact.RunnableMs, impact.SleepMs, impact.DStateMs, impact.IOWaitMs,
				impact.Priority, sanitizeForBanner(impact.PriorityClass), impact.TargetPriority, sanitizeForBanner(impact.TargetPriorityClass),
				sanitizeForBanner(impact.PriorityRelation), impact.PriorityInversionCandidate,
				impact.LineStart, impact.LineEnd, sanitizeForBanner(impact.Summary))
		}
		for _, aggregate := range result.WakeupChain.AggregatedImpacts {
			fmt.Fprintf(&b, "- aggregated_impact thread=%s path=%s depth=%d occurrences=%d dominant_state=%s impact=%.3fms total=%.3fms target_impact=%.3fms fragments=%d switches=%d max_segment=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms priority_relation=%s priority_inversion_candidate=%t lines=%d-%d — %s\n",
				traceThreadLabel(aggregate.Thread), sanitizeForBanner(aggregate.Path), aggregate.ChainDepth, aggregate.OccurrenceCount,
				sanitizeForBanner(aggregate.DominantState), aggregate.DominantImpactMs, aggregate.TotalMs, aggregate.TargetBlockedMs,
				aggregate.FragmentCount, aggregate.StateSwitches, aggregate.MaxSegmentMs,
				aggregate.RunningMs, aggregate.RunnableMs, aggregate.SleepMs, aggregate.DStateMs, aggregate.IOWaitMs,
				sanitizeForBanner(aggregate.PriorityRelation), aggregate.PriorityInversion, aggregate.LineStart, aggregate.LineEnd, sanitizeForBanner(aggregate.Summary))
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
			fmt.Fprintf(&b, "- rank=%d tier=%s type=%s thread=%s window=%.6f..%.6f dominant_state=%s running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms impact=%.3fms target_impact=%.3fms score=%.3f confidence=%.2f lines=%d-%d source=%s causality=%s chain_relevance=%s chain_depth=%d overlap=%.3fms edge_count=%d nearest_chain=%s nearest_window=%.6f..%.6f — %s\n",
				item.Rank, item.Tier, item.Type, traceThreadLabel(item.Thread), item.StartTs, item.EndTs,
				sanitizeForBanner(item.DominantState), item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs,
				item.ImpactMs, item.TargetImpactMs, item.Score, item.Confidence,
				item.LineStart, item.LineEnd, item.Source, sanitizeForBanner(item.Causality), sanitizeForBanner(item.ChainRelevance), item.ChainDepth, item.OverlapMs, item.EdgeCount,
				traceThreadLabel(item.NearestChainThread), item.NearestChainWindow.StartTs, item.NearestChainWindow.EndTs, item.Summary)
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
			fmt.Fprintf(&b, "- runnable_wait %s %.6f..%.6f duration=%.3fms cpu=%d core_class=%s freq=%dkHz prio=%d/%s same_cpu_busy=%.3fms same_cpu_idle=%.3fms other_cpu_idle=%.3fms high_prio_running=%.3fms lines=%d-%d — %s\n",
				traceThreadLabel(item.Thread), item.StartTs, item.EndTs, item.DurationMs, item.CPU, sanitizeForBanner(item.CoreClass), item.Frequency, item.Priority, item.PriorityClass, item.SameCPUBusyMs, item.SameCPUIdleMs, item.OtherCPUIdleMs, item.HighPriorityRunningMs, item.StartLine, item.EndLine, item.Summary)
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
		for _, load := range result.WindowStats.ThreadCPULoad {
			writeTraceThreadCPULoad(&b, load)
		}
		for _, constraint := range result.WindowStats.CPUConstraints {
			writeTraceCPUConstraint(&b, constraint)
		}
		for _, ctx := range result.WindowStats.RunnableContext {
			writeTraceRunnableContext(&b, ctx)
		}
		for _, proc := range result.WindowStats.ProcessCPULoad {
			writeTraceProcessCPULoad(&b, proc)
		}
		for _, churn := range result.WindowStats.StateChurn {
			fmt.Fprintf(&b, "- state_churn %s dominant_state=%s impact=%.3fms total=%.3fms fragments=%d switches=%d max_segment=%.3fms p95_segment=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms confidence=%.2f lines=%d-%d — %s\n",
				traceThreadLabel(churn.Thread), sanitizeForBanner(churn.DominantState), churn.DominantImpactMs, churn.TotalMs, churn.FragmentCount, churn.StateSwitches, churn.MaxSegmentMs, churn.P95SegmentMs, churn.RunningMs, churn.RunnableMs, churn.SleepMs, churn.DStateMs, churn.IOWaitMs, churn.Confidence, churn.LineStart, churn.LineEnd, sanitizeForBanner(churn.Summary))
		}
		for _, span := range result.WindowStats.TraceSpans {
			fmt.Fprintf(&b, "- trace_span %s %q category=%s subcategory=%s duration=%.3fms lines=%d-%d\n",
				traceThreadLabel(span.Thread), span.Name, sanitizeForBanner(span.Category), sanitizeForBanner(span.Subcategory), span.DurationMs, span.StartLine, span.EndLine)
		}
		for _, category := range result.WindowStats.TraceMarkCategories {
			fmt.Fprintf(&b, "- trace_mark_category category=%s subcategory=%s count=%d total=%.3fms max=%.3fms top_span=%s top_thread=%s lines=%d-%d — %s\n",
				sanitizeForBanner(category.Category), sanitizeForBanner(category.Subcategory), category.Count, category.TotalMs, category.MaxDurationMs, sanitizeForBanner(category.TopSpan), traceThreadLabel(category.TopThread), category.LineStart, category.LineEnd, sanitizeForBanner(category.Summary))
		}
		for _, work := range result.WindowStats.AsyncFileWork {
			fmt.Fprintf(&b, "- async_file_work %s category=%s span=%s duration=%.3fms lines=%d-%d — %s\n",
				traceThreadLabel(work.Thread), sanitizeForBanner(work.Category), sanitizeForBanner(work.Name), work.DurationMs, work.LineStart, work.LineEnd, sanitizeForBanner(work.Summary))
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
		for _, file := range result.WindowStats.FileIOByInode {
			writeTraceFileIO(&b, file)
		}
		for _, cache := range result.WindowStats.PageCacheByInode {
			writeTracePageCache(&b, cache)
		}
		for _, storage := range result.WindowStats.StorageLatencyByLayer {
			writeTraceStorageLatency(&b, storage)
		}
		if result.WindowStats.IOPressureSummary != nil {
			writeTraceIOPressure(&b, *result.WindowStats.IOPressureSummary)
		}
		for _, episode := range result.WindowStats.IOBurstEpisodes {
			fmt.Fprintf(&b, "- io_burst_episode %s chain_relevance=%s signal=%s duration=%.3fms d_state=%.3fms io_wait=%.3fms block_max=%.3fms storage_max=%.3fms inode=%s dev=%s name=%s file_bytes=%d page_cache_churn=%d overlap=%.3fms nearest_chain=%s lines=%d-%d confidence=%.2f — %s\n",
				traceThreadLabel(episode.Thread), sanitizeForBanner(episode.ChainRelevance), sanitizeForBanner(episode.DominantSignal), episode.DurationMs, episode.DStateMs, episode.IOWaitMs, episode.BlockMaxLatencyMs, episode.StorageMaxLatencyMs,
				sanitizeForBanner(episode.TopInode), sanitizeForBanner(episode.TopDev), sanitizeForBanner(episode.TopEntryName), episode.FileIOBytes, episode.PageCacheChurn, episode.OverlapMs, traceThreadLabel(episode.NearestChainThread), episode.LineStart, episode.LineEnd, episode.Confidence, sanitizeForBanner(episode.Summary))
		}
		for _, inode := range result.WindowStats.BlockIOByInode {
			fmt.Fprintf(&b, "- block_io_by_inode inode=%s dev=%s name=%s thread=%s block_dev=%s op=%s file_bytes=%d page_cache_churn=%d block_max=%.3fms storage_max=%.3fms nearest_block_thread=%s line=%d-%d confidence=%.2f — %s\n",
				sanitizeForBanner(inode.Inode), sanitizeForBanner(inode.Dev), sanitizeForBanner(inode.EntryName), traceThreadLabel(inode.Thread), sanitizeForBanner(inode.BlockDev), sanitizeForBanner(inode.Operation), inode.FileIOBytes, inode.PageCacheChurn, inode.BlockMaxLatencyMs, inode.StorageMaxLatencyMs, traceThreadLabel(inode.NearestBlockThread), inode.LineStart, inode.LineEnd, inode.Confidence, sanitizeForBanner(inode.Summary))
		}
		for _, irq := range result.WindowStats.IRQActivity {
			fmt.Fprintf(&b, "- irq_activity kind=%s cpu=%d core_class=%s vector=%d name=%s count=%d paired=%d active=%.3fms max=%.3fms lines=%d-%d — %s\n",
				sanitizeForBanner(irq.Kind), irq.CPU, sanitizeForBanner(irq.CoreClass), irq.Vector, sanitizeForBanner(irq.Name), irq.Count, irq.PairedCount, irq.ActiveMs, irq.MaxActiveMs, irq.LineStart, irq.LineEnd, sanitizeForBanner(irq.Summary))
		}
		for _, soft := range result.WindowStats.SoftIRQActivity {
			fmt.Fprintf(&b, "- softirq_activity kind=%s cpu=%d core_class=%s vector=%d name=%s count=%d paired=%d active=%.3fms max=%.3fms lines=%d-%d — %s\n",
				sanitizeForBanner(soft.Kind), soft.CPU, sanitizeForBanner(soft.CoreClass), soft.Vector, sanitizeForBanner(soft.Name), soft.Count, soft.PairedCount, soft.ActiveMs, soft.MaxActiveMs, soft.LineStart, soft.LineEnd, sanitizeForBanner(soft.Summary))
		}
		for _, work := range result.WindowStats.WorkqueueActivity {
			fmt.Fprintf(&b, "- workqueue_activity %s work=%s function=%s count=%d paired=%d duration=%.3fms max=%.3fms lines=%d-%d — %s\n",
				traceThreadLabel(work.Thread), sanitizeForBanner(work.Work), sanitizeForBanner(work.Function), work.Count, work.PairedCount, work.DurationMs, work.MaxLatencyMs, work.LineStart, work.LineEnd, sanitizeForBanner(work.Summary))
		}
		if result.WindowStats.SupplyPressureSummary != nil {
			supply := result.WindowStats.SupplyPressureSummary
			fmt.Fprintf(&b, "- supply_pressure signal=%s cpu_pressure=%.3fms runnable=%.3fms high_prio=%.3fms low_freq_cpus=%v clock_set_rate=%d thermal=%d ddr=%d l3=%d throughput=%d lines=%d-%d — %s\n",
				sanitizeForBanner(supply.Signal), supply.CPUPressureMs, supply.RunnableWaitMs, supply.HighPriorityRunningMs, supply.LowFrequencyCPUs, supply.ClockSetRateCount, supply.ThermalEventCount, supply.DDREventCount, supply.L3EventCount, supply.ThroughputEventCount, supply.LineStart, supply.LineEnd, sanitizeForBanner(supply.Summary))
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
			fmt.Fprintf(&b, "- blocking type=%s thread=%s peer=%s chain_relevance=%s overlap=%.3fms edge_count=%d nearest_chain=%s duration=%.3fms lines=%d-%d confidence=%.2f — %s\n",
				item.Type, traceThreadLabel(item.Thread), traceThreadLabel(item.Peer), sanitizeForBanner(item.ChainRelevance), item.OverlapMs, item.EdgeCount, traceThreadLabel(item.NearestChainThread), item.DurationMs, item.LineStart, item.LineEnd, item.Confidence, item.Summary)
			if item.PeerState != nil {
				fmt.Fprintf(&b, "  peer_state thread=%s window=%.6f..%.6f dominant_state=%s total=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms fragments=%d max_segment=%.3fms lines=%d-%d — %s\n",
					traceThreadLabel(item.PeerState.Thread), item.PeerState.Window.StartTs, item.PeerState.Window.EndTs, sanitizeForBanner(item.PeerState.DominantState), item.PeerState.TotalMs,
					item.PeerState.RunningMs, item.PeerState.RunnableMs, item.PeerState.SleepMs, item.PeerState.DStateMs, item.PeerState.IOWaitMs,
					item.PeerState.FragmentCount, item.PeerState.MaxSegmentMs, item.PeerState.LineStart, item.PeerState.LineEnd, sanitizeForBanner(item.PeerState.Summary))
			}
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

func writeTraceFrameRootCauseBundleSummary(b *strings.Builder, bundle *tracequery.FrameRootCauseBundle) {
	if b == nil || bundle == nil {
		return
	}
	b.WriteString("## Frame root cause bundle\n")
	fmt.Fprintf(b, "- target=%s window=%.6f..%.6f root_causes=%d blocking=%d io_bursts=%d block_inode=%d irq=%d softirq=%d workqueue=%d trace_categories=%d async_file=%d\n",
		traceThreadLabel(bundle.Target), bundle.Window.StartTs, bundle.Window.EndTs,
		traceQueryBundleRootCauseCount(bundle), traceQueryBundleBlockingCount(bundle), len(bundle.IOBurstEpisodes), len(bundle.BlockIOByInode), len(bundle.IRQActivity), len(bundle.SoftIRQActivity), len(bundle.WorkqueueActivity), len(bundle.TraceMarkCategories), len(bundle.AsyncFileWork))
	if bundle.RootCauseRank != nil && len(bundle.RootCauseRank.Items) > 0 {
		top := bundle.RootCauseRank.Items[0]
		fmt.Fprintf(b, "- bundle_top_cause type=%s thread=%s chain_relevance=%s dominant_state=%s impact=%.3fms d_state=%.3fms io_wait=%.3fms score=%.3f source=%s — %s\n",
			top.Type, traceThreadLabel(top.Thread), sanitizeForBanner(top.ChainRelevance), sanitizeForBanner(top.DominantState), top.ImpactMs, top.DStateMs, top.IOWaitMs, top.Score, sanitizeForBanner(top.Source), sanitizeForBanner(top.Summary))
	}
	if bundle.WakeupChain != nil {
		if path := traceQueryWakeupChainPath(*bundle.WakeupChain); path != "" {
			fmt.Fprintf(b, "- bundle_wakeup_chain path=%s\n", sanitizeForBanner(path))
		}
	}
	for _, episode := range bundle.IOBurstEpisodes {
		fmt.Fprintf(b, "- bundle_io_burst %s chain_relevance=%s signal=%s duration=%.3fms inode=%s overlap=%.3fms nearest_chain=%s — %s\n",
			traceThreadLabel(episode.Thread), sanitizeForBanner(episode.ChainRelevance), sanitizeForBanner(episode.DominantSignal), episode.DurationMs, sanitizeForBanner(episode.TopInode), episode.OverlapMs, traceThreadLabel(episode.NearestChainThread), sanitizeForBanner(episode.Summary))
	}
	if bundle.SupplyPressureSummary != nil {
		fmt.Fprintf(b, "- bundle_supply signal=%s cpu_pressure=%.3fms low_freq_cpus=%v — %s\n",
			sanitizeForBanner(bundle.SupplyPressureSummary.Signal), bundle.SupplyPressureSummary.CPUPressureMs, bundle.SupplyPressureSummary.LowFrequencyCPUs, sanitizeForBanner(bundle.SupplyPressureSummary.Summary))
	}
	for _, caveat := range bundle.Caveats {
		fmt.Fprintf(b, "- bundle_caveat=%s\n", sanitizeForBanner(caveat))
	}
	b.WriteString("\n")
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

func traceQueryBundleRootCauseCount(bundle *tracequery.FrameRootCauseBundle) int {
	if bundle == nil || bundle.RootCauseRank == nil {
		return 0
	}
	return len(bundle.RootCauseRank.Items)
}

func traceQueryBundleBlockingCount(bundle *tracequery.FrameRootCauseBundle) int {
	if bundle == nil || bundle.CriticalBlocking == nil {
		return 0
	}
	return len(bundle.CriticalBlocking.Items)
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

func writeTraceFileIO(b *strings.Builder, item tracequery.FileIOSummary) {
	fmt.Fprintf(b, "- file_io inode=%s dev=%s name=%s op=%s thread=%s count=%d completions=%d bytes=%d total_latency=%.3fms max_latency=%.3fms ret=%d offsets=%s example=%s lines=%d-%d — %s\n",
		sanitizeForBanner(firstNonEmptyTraceString(item.Inode, "unknown")),
		sanitizeForBanner(firstNonEmptyTraceString(item.Dev, "unknown")),
		sanitizeForBanner(firstNonEmptyTraceString(item.EntryName, "unknown")),
		sanitizeForBanner(item.Operation),
		traceThreadLabel(item.Thread),
		item.Count,
		item.CompletionCount,
		item.Bytes,
		item.TotalLatencyMs,
		item.MaxLatencyMs,
		item.Ret,
		traceOffsetRange(item.MinOffset, item.MaxOffset),
		sanitizeForBanner(item.Example),
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func writeTracePageCache(b *strings.Builder, item tracequery.PageCacheSummary) {
	fmt.Fprintf(b, "- page_cache inode=%s dev=%s thread=%s adds=%d deletes=%d churn=%d bytes=%d offsets=%s lines=%d-%d — %s\n",
		sanitizeForBanner(firstNonEmptyTraceString(item.Inode, "unknown")),
		sanitizeForBanner(firstNonEmptyTraceString(item.Dev, "unknown")),
		traceThreadLabel(item.Thread),
		item.Adds,
		item.Deletes,
		item.Churn,
		item.Bytes,
		traceOffsetRange(item.MinOffset, item.MaxOffset),
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func writeTraceStorageLatency(b *strings.Builder, item tracequery.StorageLatencySummary) {
	fmt.Fprintf(b, "- storage_latency layer=%s event=%s dev=%s inode=%s name=%s op=%s thread=%s count=%d paired=%d unpaired_start=%d unpaired_done=%d max_latency=%.3fms avg_latency=%.3fms bytes=%d example=%s lines=%d-%d — %s\n",
		sanitizeForBanner(item.Layer),
		sanitizeForBanner(item.Event),
		sanitizeForBanner(firstNonEmptyTraceString(item.Dev, "unknown")),
		sanitizeForBanner(item.Inode),
		sanitizeForBanner(item.EntryName),
		sanitizeForBanner(item.Operation),
		traceThreadLabel(item.Thread),
		item.Count,
		item.PairedCount,
		item.UnpairedStartCount,
		item.UnpairedDoneCount,
		item.MaxLatencyMs,
		item.AvgLatencyMs,
		item.Bytes,
		sanitizeForBanner(item.Example),
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func writeTraceIOPressure(b *strings.Builder, item tracequery.IOPressureSummary) {
	fmt.Fprintf(b, "- io_pressure signal=%s score=%.3f block_max=%.3fms storage_max=%.3fms file_bytes=%d file_events=%d page_cache_churn=%d iowait_blocked=%d d_state=%.3fms top_inode=%s top_dev=%s top_name=%s lines=%d-%d — %s\n",
		sanitizeForBanner(item.Signal),
		item.Score,
		item.BlockMaxLatencyMs,
		item.StorageMaxLatencyMs,
		item.FileIOBytes,
		item.FileIOEvents,
		item.PageCacheChurn,
		item.IOWaitBlockedCount,
		item.DStateMs,
		sanitizeForBanner(firstNonEmptyTraceString(item.TopInode, "unknown")),
		sanitizeForBanner(firstNonEmptyTraceString(item.TopDev, "unknown")),
		sanitizeForBanner(firstNonEmptyTraceString(item.TopEntryName, "unknown")),
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func writeTraceThreadCPULoad(b *strings.Builder, item tracequery.ThreadCPULoadSummary) {
	fmt.Fprintf(b, "- thread_cpu_load thread=%s running=%.3fms runnable=%.3fms high_prio_running=%.3fms cpu=%d core_class=%s freq=%dkHz prio=%d/%s lines=%d-%d — %s\n",
		traceThreadLabel(item.Thread),
		item.RunningMs,
		item.RunnableWaitMs,
		item.HighPriorityRunningMs,
		item.CPU,
		sanitizeForBanner(item.CoreClass),
		item.Frequency,
		item.Priority,
		sanitizeForBanner(item.PriorityClass),
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func writeTraceCPUConstraint(b *strings.Builder, item tracequery.CPUConstraintSummary) {
	fmt.Fprintf(b, "- cpu_constraint thread=%s kind=%s allowed_cpus=%s allowed_core_classes=%s cpuset=%s policy=%s observed_cpu=%s observed_core_class=%s migrations=%d runnable=%.3fms other_cpu_idle=%.3fms lines=%d-%d — %s\n",
		traceThreadLabel(item.Thread),
		sanitizeForBanner(item.Kind),
		traceIntList(item.AllowedCPUs),
		sanitizeForBanner(strings.Join(item.AllowedCoreClasses, ",")),
		sanitizeForBanner(item.CPUSet),
		sanitizeForBanner(item.Policy),
		traceKnownCPU(item.ObservedCPUKnown, item.ObservedCPU),
		sanitizeForBanner(item.ObservedCoreClass),
		item.MigrationCount,
		item.RunnableWaitMs,
		item.OtherCPUIdleMs,
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func traceKnownCPU(known bool, cpu int) string {
	if !known {
		return ""
	}
	return strconv.Itoa(cpu)
}

func writeTraceRunnableContext(b *strings.Builder, item tracequery.RunnableContextSummary) {
	var bgThreads []string
	for i, load := range item.TopBackgroundThreads {
		if i >= 4 {
			break
		}
		bgThreads = append(bgThreads, fmt.Sprintf("%s/%.3fms", traceThreadLabel(load.Thread), load.RunningMs+load.RunnableWaitMs))
	}
	constraint := "none"
	if item.CPUConstraint != nil {
		constraint = fmt.Sprintf("allowed_cpus=%s;allowed_core_classes=%s;cpuset=%s;policy=%s",
			traceIntList(item.CPUConstraint.AllowedCPUs),
			strings.Join(item.CPUConstraint.AllowedCoreClasses, ","),
			sanitizeForBanner(item.CPUConstraint.CPUSet),
			sanitizeForBanner(item.CPUConstraint.Policy))
	}
	process := "none"
	if item.TopBackgroundProcess != nil {
		process = fmt.Sprintf("%s/%.3fms", traceThreadLabel(item.TopBackgroundProcess.Process), item.TopBackgroundProcess.RunningMs+item.TopBackgroundProcess.RunnableWaitMs)
	}
	fmt.Fprintf(b, "- runnable_context thread=%s runnable=%.3fms cpu=%d core_class=%s freq=%dkHz same_cpu_busy=%.3fms same_cpu_idle=%.3fms other_cpu_idle=%.3fms high_prio_running=%.3fms top_background_threads=%s top_background_process=%s constraint=%s verdict=%s confidence=%.2f lines=%d-%d — %s\n",
		traceThreadLabel(item.Thread),
		item.RunnableWaitMs,
		item.CPU,
		sanitizeForBanner(item.CoreClass),
		item.Frequency,
		item.SameCPUBusyMs,
		item.SameCPUIdleMs,
		item.OtherCPUIdleMs,
		item.HighPriorityRunningMs,
		sanitizeForBanner(strings.Join(bgThreads, ",")),
		sanitizeForBanner(process),
		sanitizeForBanner(constraint),
		sanitizeForBanner(item.Verdict),
		item.Confidence,
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func writeTraceProcessCPULoad(b *strings.Builder, item tracequery.ProcessCPULoadSummary) {
	fmt.Fprintf(b, "- process_cpu_load process=%s threads=%d running=%.3fms runnable=%.3fms high_prio_running=%.3fms top_thread=%s top_thread_ms=%.3fms cpus=%s core_classes=%s lines=%d-%d — %s\n",
		traceThreadLabel(item.Process),
		item.ThreadCount,
		item.RunningMs,
		item.RunnableWaitMs,
		item.HighPriorityRunningMs,
		traceThreadLabel(item.TopThread),
		item.TopThreadMs,
		traceIntList(item.CPUs),
		sanitizeForBanner(strings.Join(item.CoreClasses, ",")),
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func traceIntList(in []int) string {
	if len(in) == 0 {
		return ""
	}
	parts := make([]string, 0, len(in))
	for _, v := range in {
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, ",")
}

func traceOffsetRange(minOffset, maxOffset int64) string {
	if minOffset == 0 && maxOffset == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%d..%d", minOffset, maxOffset)
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
	if ev.NextInfoAffinity != "" {
		parts = append(parts, fmt.Sprintf("next_info_affinity=%s allowed_cpus=%s load=%d group=%d restricted=%t expel=%d",
			sanitizeForBanner(ev.NextInfoAffinity), traceIntList(ev.NextInfoAllowedCPUs), ev.NextInfoLoad, ev.NextInfoGroup, ev.NextInfoRestricted, ev.NextInfoExpel))
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
	if ev.BlockSrcDev != "" {
		parts = append(parts, fmt.Sprintf("block_src=%s/%d", sanitizeForBanner(ev.BlockSrcDev), ev.BlockSrcSector))
	}
	if ev.SubsystemKind != "" {
		parts = append(parts, "subsystem="+ev.SubsystemKind)
	}
	if ev.Inode != "" || ev.FSDev != "" || ev.EntryName != "" {
		parts = append(parts, fmt.Sprintf("file_io dev=%s inode=%s name=%s op=%s offset=%d len=%d ret=%d",
			sanitizeForBanner(ev.FSDev),
			sanitizeForBanner(ev.Inode),
			sanitizeForBanner(ev.EntryName),
			sanitizeForBanner(ev.FileRW),
			ev.FileOffset,
			ev.FileLen,
			ev.FileRet))
	}
	if ev.PluginEventName != "" {
		parts = append(parts, "plugin_event="+sanitizeForBanner(ev.PluginEventName))
	}
	if ev.PluginMetric != "" || ev.PluginValue != "" {
		parts = append(parts, fmt.Sprintf("metric=%s value=%s", sanitizeForBanner(ev.PluginMetric), sanitizeForBanner(ev.PluginValue)))
	}
	if ev.Type == tracequery.EventCPUConstraint || ev.ConstraintKind != "" || ev.CPUSet != "" || len(ev.AllowedCPUs) > 0 {
		parts = append(parts, fmt.Sprintf("cpu_constraint target=%s-%d kind=%s allowed_cpus=%s cpuset=%s policy=%s observed_cpu=%d orig_cpu=%d dest_cpu=%d",
			sanitizeForBanner(ev.ConstraintComm),
			ev.ConstraintPID,
			sanitizeForBanner(firstNonEmptyTraceString(ev.ConstraintKind, ev.Name)),
			traceIntList(ev.AllowedCPUs),
			sanitizeForBanner(ev.CPUSet),
			sanitizeForBanner(ev.ConstraintPolicy),
			ev.ConstraintCPU,
			ev.ConstraintOrigCPU,
			ev.ConstraintDestCPU))
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

func traceThreadLabelOptional(t tracequery.ThreadRef) string {
	if t.PID <= 0 && strings.TrimSpace(t.Comm) == "" {
		return ""
	}
	return traceThreadLabel(t)
}

// ---------------------------------------------------------------------------
// Typed observation publication.
//
// trace_query computes a fully typed tracequery.Result, but the ToolResult
// Summary is a capped prose preview (16 evidence-pack facts, blob-clipped
// previews) and the observation ledger historically re-parsed that preview
// line by line, silently losing every fact beyond the caps. The builders below
// project the typed result directly into ledger-ready ObservationRecord rows
// and attach them to the ToolResult (ToolResult.Observations), so the ledger
// compiles them without re-parsing while keeping the summary re-parse as the
// fallback for results without typed rows.
//
// Every row keeps runtime-artifact origin and the same role / grounding /
// provenance-lane classification the re-parse path assigned, so trace rows can
// never drift into the current-source citation lane. Row IDs are precise and
// payload-anchored (stored payload blob basename + family + ordinal) so the
// ledger's ID-level dedup keeps typed rows authoritative across duplicate
// copies of the same result.

const (
	// traceQueryTypedEvidenceFactCap bounds published evidence-pack rows.
	// Deliberately 4x the prose preview's 16-fact cap; the full payload
	// remains addressable via the stored payload reference.
	traceQueryTypedEvidenceFactCap = 64
	// traceQueryTypedFamilyRowCap bounds every other per-family row list.
	traceQueryTypedFamilyRowCap = 32
)

// traceQueryObservationScope derives the precise per-result ID namespace for
// typed rows. The stored payload reference is content-hashed and therefore
// unique per distinct result; the view + selected window is the fallback when
// no blob could be stored (e.g. no work directory).
func traceQueryObservationScope(result tracequery.Result, payloadRef, rawRef string) string {
	if ref := strings.TrimSpace(payloadRef); ref != "" {
		return filepath.Base(ref)
	}
	if ref := strings.TrimSpace(rawRef); ref != "" {
		return filepath.Base(ref)
	}
	return fmt.Sprintf("%s@%.6f-%.6f", firstNonEmptyTraceString(result.View, "trace_query"), result.TimeStart, result.TimeEnd)
}

func traceQueryObservationSourceRef(result tracequery.Result, sourceLabel, payloadRef, rawRef string) types.ObservationSourceRef {
	return types.ObservationSourceRef{
		Kind:         types.ObservationSourceRuntimeArtifact,
		Path:         strings.TrimSpace(result.SourcePath),
		ArtifactID:   traceQueryArtifactID(sourceLabel),
		ArtifactKind: "trace",
		PayloadRef:   strings.TrimSpace(payloadRef),
		RawRef:       firstNonEmptyTraceString(rawRef, payloadRef),
	}
}

func traceQueryObservationSupportRefs(ref types.ObservationSourceRef, lineStart, lineEnd int) []string {
	if lineStart <= 0 {
		return nil
	}
	path := firstNonEmptyTraceString(ref.Path, ref.ArtifactID, "runtime_artifact")
	if lineEnd <= 0 || lineEnd == lineStart {
		return []string{fmt.Sprintf("%s:%d", path, lineStart)}
	}
	return []string{fmt.Sprintf("%s:%d-%d", path, lineStart, lineEnd)}
}

func traceQueryRootCauseTierLabel(rank int) string {
	switch rank {
	case 1:
		return "primary"
	case 2:
		return "secondary"
	case 3:
		return "tertiary"
	default:
		if rank > 0 {
			return fmt.Sprintf("rank_%d", rank)
		}
		return "ranked"
	}
}

func traceQueryRootCauseRankHasForeground(items []tracequery.RootCauseRankItem) bool {
	for _, item := range items {
		switch traceQueryRootCauseItemRelevance(item) {
		case "on_chain", "adjacent":
			return true
		}
	}
	return false
}

func traceQueryRootCauseItemRelevance(item tracequery.RootCauseRankItem) string {
	if relevance := strings.TrimSpace(item.ChainRelevance); relevance != "" {
		return relevance
	}
	switch strings.TrimSpace(item.Causality) {
	case "on_wakeup_chain":
		return "on_chain"
	case "adjacent_to_wakeup_chain":
		return "adjacent"
	case "background":
		return "background"
	default:
		return ""
	}
}

func traceQueryObservationMSValue(ms float64) string {
	if ms <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3f", ms)
}

func traceQueryTimestampValue(ts float64) string {
	if ts <= 0 {
		return ""
	}
	return fmt.Sprintf("%.6f", ts)
}

func traceQueryWindowValue(start, end float64) string {
	if start <= 0 && end <= 0 {
		return ""
	}
	return fmt.Sprintf("%.6f..%.6f", start, end)
}

// traceQueryTypedObservations projects the typed product of the executed view
// into ledger-ready observation rows. idScope is appended to the per-result
// namespace so multi-window results (one ToolResult carrying several bounded
// child runs) keep distinct row IDs.
func traceQueryTypedObservations(result tracequery.Result, sourceLabel, payloadRef, rawRef, idScope string, observedAt time.Time) []types.ObservationRecord {
	ref := traceQueryObservationSourceRef(result, sourceLabel, payloadRef, rawRef)
	scope := traceQueryObservationScope(result, payloadRef, rawRef)
	if strings.TrimSpace(idScope) != "" {
		scope += ":" + strings.TrimSpace(idScope)
	}
	at := observedAt.Format("2006-01-02T15:04:05Z07:00")
	var out []types.ObservationRecord

	if result.RootCauseRank != nil {
		hasForegroundRootCause := traceQueryRootCauseRankHasForeground(result.RootCauseRank.Items)
		for i, item := range result.RootCauseRank.Items {
			if i >= traceQueryTypedFamilyRowCap {
				break
			}
			rank := item.Rank
			if rank <= 0 {
				rank = i + 1
			}
			tier := firstNonEmptyTraceString(item.Tier, traceQueryRootCauseTierLabel(rank))
			if strings.TrimSpace(item.Type) == "" && strings.TrimSpace(item.Summary) == "" {
				continue
			}
			notes := traceQueryTypedPriorityRichNotes(rank, tier, item.Type, item.Source, item.Causality, item.ChainDepth, item.Score, item.ImpactMs, item.TargetImpactMs)
			notes = append(notes, traceQueryTypedRootCauseStateRichNotes(item)...)
			notes = append(notes, traceQueryTypedKVNotes([][2]string{
				{"chain_relevance", item.ChainRelevance},
				{"overlap", traceQueryObservationMSValue(item.OverlapMs)},
				{"edge_count", traceQueryTypedCount(item.EdgeCount)},
				{"nearest_chain_thread", traceThreadLabelOptional(item.NearestChainThread)},
				{"nearest_chain_window", traceQueryTypedTimeWindow(item.NearestChainWindow)},
			})...)
			role := types.AnswerAggregateRolePrincipalAnswer
			grounding := types.ClaimGroundingHard
			provenance := types.ObservationProvenanceObservedDirectCause
			claimKey := "root_cause_" + tier
			predicate := claimKey
			if hasForegroundRootCause && traceQueryRootCauseItemRelevance(item) == "background" {
				role = types.AnswerAggregateRoleSupportingCoverage
				provenance = types.ObservationProvenanceArtifactSpan
				claimKey = "root_cause_background"
				predicate = "root_cause_background"
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#root_cause_rank:%d", scope, rank),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            role,
				GroundingPolicy: grounding,
				ProvenanceLane:  provenance,
				SourceRef:       ref,
				Span:            types.ObservationSpan{LineStart: item.LineStart, LineEnd: item.LineEnd},
				ClaimKey:        claimKey,
				Subject:         traceThreadLabel(item.Thread),
				Predicate:       predicate,
				Object:          item.Type,
				Value:           traceQueryObservationMSValue(item.ImpactMs),
				Unit:            "ms",
				Summary:         firstNonEmptyTraceString(item.Summary, fmt.Sprintf("%s cause #%d (%s)", tier, rank, item.Type)),
				RichNotes:       notes,
				SupportRefs:     traceQueryObservationSupportRefs(ref, item.LineStart, item.LineEnd),
				ObservedAt:      at,
				Confidence:      item.Confidence,
			})
		}
	}

	for i, fact := range result.EvidencePack {
		if i >= traceQueryTypedEvidenceFactCap {
			break
		}
		if strings.TrimSpace(fact.Subject) == "" && strings.TrimSpace(fact.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#evidence_fact:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span: types.ObservationSpan{
				LineStart: fact.LineStart,
				LineEnd:   fact.LineEnd,
				StartTs:   fact.StartTs,
				EndTs:     fact.EndTs,
			},
			ClaimKey:    "evidence_fact:" + firstNonEmptyTraceString(fact.Predicate, fact.Subject),
			Subject:     fact.Subject,
			Predicate:   fact.Predicate,
			Object:      fact.Object,
			Summary:     fact.Summary,
			SupportRefs: traceQueryObservationSupportRefs(ref, fact.LineStart, fact.LineEnd),
			ObservedAt:  at,
			Confidence:  fact.Confidence,
		})
	}

	if result.WakeupChain != nil {
		path := traceQueryWakeupChainPath(*result.WakeupChain)
		if path != "" {
			lineStart, lineEnd := traceQueryWakeupChainLineRange(*result.WakeupChain)
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#wakeup_chain:path", scope),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
				SourceRef:       ref,
				Span: types.ObservationSpan{
					LineStart: lineStart,
					LineEnd:   lineEnd,
					StartTs:   result.WakeupChain.Window.StartTs,
					EndTs:     result.WakeupChain.Window.EndTs,
				},
				ClaimKey:    "wakeup_chain:path",
				Subject:     traceThreadLabel(result.WakeupChain.Target),
				Predicate:   "wakeup_chain",
				Object:      path,
				Summary:     "wakeup_chain path=" + path,
				RichNotes:   traceQueryTypedWakeupPathRichNotes(*result.WakeupChain, path),
				SupportRefs: traceQueryObservationSupportRefs(ref, lineStart, lineEnd),
				ObservedAt:  at,
				Confidence:  0.82,
			})
		}
		for i, edge := range traceQuerySortedWakeupEdges(*result.WakeupChain) {
			if i >= traceQueryTypedFamilyRowCap {
				break
			}
			if strings.TrimSpace(traceThreadLabel(edge.Waker)) == "" && strings.TrimSpace(traceThreadLabel(edge.Wakee)) == "" {
				continue
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#wakeup_chain_edge:%d", scope, i+1),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
				SourceRef:       ref,
				Span: types.ObservationSpan{
					LineStart: edge.WakeupLine,
					LineEnd:   edge.WakeupLine,
					StartTs:   edge.WakeupTs,
					EndTs:     edge.WakeupTs,
				},
				ClaimKey:    fmt.Sprintf("wakeup_chain_edge:%s->%s", traceThreadLabel(edge.Waker), traceThreadLabel(edge.Wakee)),
				Subject:     traceThreadLabel(edge.Waker),
				Predicate:   "wakeup_chain_edge",
				Object:      traceThreadLabel(edge.Wakee),
				Value:       traceQueryObservationMSValue(edge.LatencyMs),
				Unit:        "ms",
				Summary:     traceQueryWakeupEdgeSummary(edge),
				RichNotes:   traceQueryTypedWakeupEdgeRichNotes(edge, path),
				SupportRefs: traceQueryObservationSupportRefs(ref, edge.WakeupLine, edge.WakeupLine),
				ObservedAt:  at,
				Confidence:  0.82,
			})
		}
		for i, impact := range result.WakeupChain.CausalImpacts {
			if i >= traceQueryTypedFamilyRowCap {
				break
			}
			if strings.TrimSpace(impact.DominantState) == "" && strings.TrimSpace(impact.Summary) == "" {
				continue
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#wakeup_causal_impact:%d", scope, i+1),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
				SourceRef:       ref,
				Span: types.ObservationSpan{
					LineStart: impact.LineStart,
					LineEnd:   impact.LineEnd,
					StartTs:   impact.Window.StartTs,
					EndTs:     impact.Window.EndTs,
				},
				ClaimKey:    "wakeup_causal_impact:" + firstNonEmptyTraceString(traceThreadLabel(impact.Thread), impact.DominantState),
				Subject:     traceThreadLabel(impact.Thread),
				Predicate:   "wakeup_causal_impact",
				Object:      impact.DominantState,
				Value:       traceQueryObservationMSValue(impact.DominantImpactMs),
				Unit:        "ms",
				Summary:     impact.Summary,
				RichNotes:   traceQueryTypedCausalImpactRichNotes(impact),
				SupportRefs: traceQueryObservationSupportRefs(ref, impact.LineStart, impact.LineEnd),
				ObservedAt:  at,
				Confidence:  0.78,
			})
		}
		for i, aggregate := range result.WakeupChain.AggregatedImpacts {
			if i >= traceQueryTypedFamilyRowCap {
				break
			}
			if strings.TrimSpace(aggregate.DominantState) == "" && strings.TrimSpace(aggregate.Summary) == "" {
				continue
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#wakeup_causal_aggregate:%d", scope, i+1),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
				SourceRef:       ref,
				Span: types.ObservationSpan{
					LineStart: aggregate.LineStart,
					LineEnd:   aggregate.LineEnd,
					StartTs:   aggregate.FirstTs,
					EndTs:     aggregate.LastTs,
				},
				ClaimKey:    "wakeup_causal_aggregate:" + firstNonEmptyTraceString(traceThreadLabel(aggregate.Thread), aggregate.DominantState),
				Subject:     traceThreadLabel(aggregate.Thread),
				Predicate:   "wakeup_causal_aggregate",
				Object:      aggregate.DominantState,
				Value:       traceQueryObservationMSValue(aggregate.DominantImpactMs),
				Unit:        "ms",
				Summary:     aggregate.Summary,
				RichNotes:   traceQueryTypedCausalAggregateRichNotes(aggregate),
				SupportRefs: traceQueryObservationSupportRefs(ref, aggregate.LineStart, aggregate.LineEnd),
				ObservedAt:  at,
				Confidence:  0.80,
			})
		}
		for i, root := range result.WakeupChain.RootEvidence {
			if i >= traceQueryTypedFamilyRowCap {
				break
			}
			if strings.TrimSpace(root.Type) == "" && strings.TrimSpace(root.Summary) == "" {
				continue
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#root_evidence:%d", scope, i+1),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
				SourceRef:       ref,
				Span:            types.ObservationSpan{LineStart: root.LineStart, LineEnd: root.LineEnd},
				ClaimKey:        "root_evidence:" + root.Type,
				Subject:         traceThreadLabel(root.Thread),
				Predicate:       root.Type,
				Value:           traceQueryObservationMSValue(root.DurationMs),
				Unit:            "ms",
				Summary:         root.Summary,
				SupportRefs:     traceQueryObservationSupportRefs(ref, root.LineStart, root.LineEnd),
				ObservedAt:      at,
				Confidence:      root.Confidence,
			})
		}
	}

	if result.CriticalBlocking != nil {
		for i, item := range result.CriticalBlocking.Items {
			if i >= traceQueryTypedFamilyRowCap {
				break
			}
			if strings.TrimSpace(item.Type) == "" && strings.TrimSpace(item.Summary) == "" {
				continue
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#critical_blocking:%d", scope, i+1),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
				SourceRef:       ref,
				Span:            types.ObservationSpan{LineStart: item.LineStart, LineEnd: item.LineEnd},
				ClaimKey:        "critical_blocking:" + item.Type,
				Subject:         traceThreadLabel(item.Thread),
				Predicate:       "critical_blocking",
				Object:          firstNonEmptyTraceString(traceThreadLabel(item.Peer), item.Type),
				Value:           traceQueryObservationMSValue(item.DurationMs),
				Unit:            "ms",
				Summary:         item.Summary,
				RichNotes:       traceQueryTypedCriticalBlockingRichNotes(item),
				SupportRefs:     traceQueryObservationSupportRefs(ref, item.LineStart, item.LineEnd),
				ObservedAt:      at,
				Confidence:      item.Confidence,
			})
		}
	}

	if result.WindowStats != nil {
		out = append(out, traceQueryTypedWindowStatsObservations(*result.WindowStats, ref, scope, at)...)
	}

	return out
}

func traceQueryTypedPriorityRichNotes(rank int, tier, typ, source, causality string, chainDepth int, score, impact, targetImpact float64) []string {
	var notes []string
	if rank > 0 {
		notes = append(notes, fmt.Sprintf("rank=%d", rank))
	}
	if tier != "" {
		notes = append(notes, "tier="+tier)
	}
	if typ != "" {
		notes = append(notes, "type="+typ)
	}
	if impact > 0 {
		notes = append(notes, fmt.Sprintf("impact_ms=%.3f", impact))
	}
	if targetImpact > 0 {
		notes = append(notes, fmt.Sprintf("target_impact_ms=%.3f", targetImpact))
	}
	if score > 0 {
		notes = append(notes, fmt.Sprintf("score=%.3f", score))
	}
	if source != "" {
		notes = append(notes, "source="+source)
	}
	if causality != "" {
		notes = append(notes, "causality="+causality)
	}
	if chainDepth > 0 {
		notes = append(notes, fmt.Sprintf("chain_depth=%d", chainDepth))
	}
	return notes
}

func traceQueryTypedRootCauseStateRichNotes(item tracequery.RootCauseRankItem) []string {
	return traceQueryTypedKVNotes([][2]string{
		{"dominant_state", item.DominantState},
		{"running", traceQueryObservationMSValue(item.RunningMs)},
		{"runnable", traceQueryObservationMSValue(item.RunnableMs)},
		{"sleep", traceQueryObservationMSValue(item.SleepMs)},
		{"d_state", traceQueryObservationMSValue(item.DStateMs)},
		{"io_wait", traceQueryObservationMSValue(item.IOWaitMs)},
	})
}

func traceQueryTypedCausalImpactRichNotes(impact tracequery.WakeupCausalImpact) []string {
	return traceQueryTypedKVNotes([][2]string{
		{"depth", traceQueryTypedCount(impact.ChainDepth)},
		{"causality", traceQueryCausalityLabel(impact.OnChain)},
		{"dominant_state", impact.DominantState},
		{"impact", traceQueryObservationMSValue(impact.DominantImpactMs)},
		{"total", traceQueryObservationMSValue(impact.TotalMs)},
		{"target_impact", traceQueryObservationMSValue(impact.TargetBlockedMs)},
		{"fragments", traceQueryTypedCount(impact.FragmentCount)},
		{"switches", traceQueryTypedCount(impact.StateSwitches)},
		{"max_segment", traceQueryObservationMSValue(impact.MaxSegmentMs)},
		{"p95_segment", traceQueryObservationMSValue(impact.P95SegmentMs)},
		{"running", traceQueryObservationMSValue(impact.RunningMs)},
		{"runnable", traceQueryObservationMSValue(impact.RunnableMs)},
		{"sleep", traceQueryObservationMSValue(impact.SleepMs)},
		{"d_state", traceQueryObservationMSValue(impact.DStateMs)},
		{"io_wait", traceQueryObservationMSValue(impact.IOWaitMs)},
		{"priority", traceQueryPriorityPair(impact.Priority, impact.PriorityClass)},
		{"target_priority", traceQueryPriorityPair(impact.TargetPriority, impact.TargetPriorityClass)},
		{"priority_relation", impact.PriorityRelation},
		{"priority_inversion_candidate", traceQueryTypedBool(impact.PriorityInversionCandidate)},
		{"next_step", impact.NextStep},
	})
}

func traceQueryTypedCausalAggregateRichNotes(aggregate tracequery.WakeupCausalAggregate) []string {
	return traceQueryTypedKVNotes([][2]string{
		{"depth", traceQueryTypedCount(aggregate.ChainDepth)},
		{"path", aggregate.Path},
		{"occurrences", traceQueryTypedCount(aggregate.OccurrenceCount)},
		{"dominant_state", aggregate.DominantState},
		{"impact", traceQueryObservationMSValue(aggregate.DominantImpactMs)},
		{"total", traceQueryObservationMSValue(aggregate.TotalMs)},
		{"target_impact", traceQueryObservationMSValue(aggregate.TargetBlockedMs)},
		{"fragments", traceQueryTypedCount(aggregate.FragmentCount)},
		{"switches", traceQueryTypedCount(aggregate.StateSwitches)},
		{"max_segment", traceQueryObservationMSValue(aggregate.MaxSegmentMs)},
		{"running", traceQueryObservationMSValue(aggregate.RunningMs)},
		{"runnable", traceQueryObservationMSValue(aggregate.RunnableMs)},
		{"sleep", traceQueryObservationMSValue(aggregate.SleepMs)},
		{"d_state", traceQueryObservationMSValue(aggregate.DStateMs)},
		{"io_wait", traceQueryObservationMSValue(aggregate.IOWaitMs)},
		{"priority_relation", aggregate.PriorityRelation},
		{"priority_inversion_candidate", traceQueryTypedBool(aggregate.PriorityInversion)},
	})
}

func traceQueryTypedCriticalBlockingRichNotes(item tracequery.CriticalBlockingCandidate) []string {
	notes := traceQueryTypedKVNotes([][2]string{
		{"type", item.Type},
		{"peer", traceThreadLabel(item.Peer)},
		{"chain_relevance", item.ChainRelevance},
		{"overlap", traceQueryObservationMSValue(item.OverlapMs)},
		{"edge_count", traceQueryTypedCount(item.EdgeCount)},
		{"nearest_chain_thread", traceThreadLabel(item.NearestChainThread)},
	})
	if item.PeerState != nil {
		notes = append(notes, traceQueryTypedKVNotes([][2]string{
			{"peer_state_dominant", item.PeerState.DominantState},
			{"peer_state_total", traceQueryObservationMSValue(item.PeerState.TotalMs)},
			{"peer_state_running", traceQueryObservationMSValue(item.PeerState.RunningMs)},
			{"peer_state_runnable", traceQueryObservationMSValue(item.PeerState.RunnableMs)},
			{"peer_state_sleep", traceQueryObservationMSValue(item.PeerState.SleepMs)},
			{"peer_state_d_state", traceQueryObservationMSValue(item.PeerState.DStateMs)},
			{"peer_state_io_wait", traceQueryObservationMSValue(item.PeerState.IOWaitMs)},
			{"peer_state_fragments", traceQueryTypedCount(item.PeerState.FragmentCount)},
		})...)
	}
	return notes
}

func traceQuerySortedWakeupEdges(chain tracequery.ChainResult) []tracequery.WakeupEdge {
	if len(chain.Edges) == 0 {
		return nil
	}
	edges := append([]tracequery.WakeupEdge(nil), chain.Edges...)
	nodeDepth := map[string]int{}
	for _, node := range chain.Nodes {
		if node.ID == "" {
			continue
		}
		depth := 0
		if node.Impact != nil {
			depth = node.Impact.ChainDepth
		}
		nodeDepth[node.ID] = depth
	}
	sort.SliceStable(edges, func(i, j int) bool {
		di := nodeDepth[edges[i].From]
		dj := nodeDepth[edges[j].From]
		if di != dj {
			return di > dj
		}
		if edges[i].WakeupTs != edges[j].WakeupTs {
			return edges[i].WakeupTs < edges[j].WakeupTs
		}
		return edges[i].WakeupLine < edges[j].WakeupLine
	})
	return edges
}

func traceQueryWakeupChainPath(chain tracequery.ChainResult) string {
	edges := traceQuerySortedWakeupEdges(chain)
	if len(edges) == 0 {
		return ""
	}
	var labels []string
	for _, edge := range edges {
		waker := traceThreadLabel(edge.Waker)
		wakee := traceThreadLabel(edge.Wakee)
		if len(labels) == 0 {
			labels = append(labels, waker)
		} else if labels[len(labels)-1] != waker {
			labels = append(labels, waker)
		}
		labels = append(labels, wakee)
	}
	return strings.Join(labels, " -> ")
}

func traceQueryWakeupChainLineRange(chain tracequery.ChainResult) (int, int) {
	minLine, maxLine := 0, 0
	add := func(line int) {
		if line <= 0 {
			return
		}
		if minLine == 0 || line < minLine {
			minLine = line
		}
		if line > maxLine {
			maxLine = line
		}
	}
	for _, edge := range chain.Edges {
		add(edge.WakeupLine)
		add(edge.EvidenceLine)
	}
	for _, impact := range chain.CausalImpacts {
		add(impact.LineStart)
		add(impact.LineEnd)
	}
	return minLine, maxLine
}

func traceQueryTypedWakeupPathRichNotes(chain tracequery.ChainResult, path string) []string {
	priorityInversions := 0
	for _, edge := range chain.Edges {
		if edge.PriorityInversionCandidate {
			priorityInversions++
		}
	}
	return traceQueryTypedKVNotes([][2]string{
		{"path", path},
		{"target", traceThreadLabel(chain.Target)},
		{"edges", traceQueryTypedCount(len(chain.Edges))},
		{"nodes", traceQueryTypedCount(len(chain.Nodes))},
		{"priority_inversion_edges", traceQueryTypedCount(priorityInversions)},
		{"window", traceQueryWindowValue(chain.Window.StartTs, chain.Window.EndTs)},
	})
}

func traceQueryTypedWakeupEdgeRichNotes(edge tracequery.WakeupEdge, path string) []string {
	return traceQueryTypedKVNotes([][2]string{
		{"path", path},
		{"wakeup_ts", traceQueryTimestampValue(edge.WakeupTs)},
		{"latency", traceQueryObservationMSValue(edge.LatencyMs)},
		{"waker_priority", traceQueryPriorityPair(edge.WakerPriority, edge.WakerPriorityClass)},
		{"wakee_priority", traceQueryPriorityPair(edge.WakeePriority, edge.WakeePriorityClass)},
		{"priority_relation", edge.PriorityRelation},
		{"priority_inversion_candidate", traceQueryTypedBool(edge.PriorityInversionCandidate)},
	})
}

func traceQueryWakeupEdgeSummary(edge tracequery.WakeupEdge) string {
	parts := []string{
		fmt.Sprintf("wakeup_chain_edge %s -> %s", traceThreadLabel(edge.Waker), traceThreadLabel(edge.Wakee)),
		fmt.Sprintf("at %.6f", edge.WakeupTs),
		fmt.Sprintf("line=%d", edge.WakeupLine),
	}
	if edge.LatencyMs > 0 {
		parts = append(parts, fmt.Sprintf("latency=%.3fms", edge.LatencyMs))
	}
	if priority := traceQueryPriorityPair(edge.WakerPriority, edge.WakerPriorityClass); priority != "" {
		parts = append(parts, "waker_prio="+priority)
	}
	if priority := traceQueryPriorityPair(edge.WakeePriority, edge.WakeePriorityClass); priority != "" {
		parts = append(parts, "wakee_prio="+priority)
	}
	if edge.PriorityRelation != "" {
		parts = append(parts, "relation="+edge.PriorityRelation)
	}
	if edge.PriorityInversionCandidate {
		parts = append(parts, "priority_inversion_candidate=true")
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedWindowStatsObservations(stats tracequery.WindowStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	var out []types.ObservationRecord

	for i, load := range stats.ThreadCPULoad {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(traceThreadLabel(load.Thread)) == "" {
			continue
		}
		total := load.RunningMs + load.RunnableWaitMs
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#thread_cpu_load:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: load.LineStart, LineEnd: load.LineEnd},
			ClaimKey:        "thread_cpu_load:" + traceThreadLabel(load.Thread),
			Subject:         traceThreadLabel(load.Thread),
			Predicate:       "thread_cpu_load",
			Object:          fmt.Sprintf("cpu=%d", load.CPU),
			Value:           traceQueryObservationMSValue(total),
			Unit:            "ms",
			Summary:         traceQueryTypedThreadCPULoadSummary(load),
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"thread", traceThreadLabel(load.Thread)},
				{"running", traceQueryObservationMSValue(load.RunningMs)},
				{"runnable", traceQueryObservationMSValue(load.RunnableWaitMs)},
				{"high_prio_running", traceQueryObservationMSValue(load.HighPriorityRunningMs)},
				{"cpu", strconv.Itoa(load.CPU)},
				{"core_class", load.CoreClass},
				{"freq", traceQueryTypedCount(load.Frequency)},
				{"priority", traceQueryPriorityPair(load.Priority, load.PriorityClass)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, load.LineStart, load.LineEnd),
			ObservedAt:  at,
			Confidence:  0.72,
		})
	}

	for i, constraint := range stats.CPUConstraints {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(constraint.Summary) == "" && len(constraint.AllowedCPUs) == 0 && strings.TrimSpace(constraint.CPUSet) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#cpu_constraint:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: constraint.LineStart, LineEnd: constraint.LineEnd, StartTs: constraint.StartTs, EndTs: constraint.EndTs},
			ClaimKey:        "cpu_constraint:" + traceThreadLabel(constraint.Thread),
			Subject:         traceThreadLabel(constraint.Thread),
			Predicate:       "cpu_constraint",
			Object:          firstNonEmptyTraceString(constraint.CPUSet, traceIntList(constraint.AllowedCPUs), constraint.Kind),
			Value:           traceQueryObservationMSValue(constraint.RunnableWaitMs),
			Unit:            "ms",
			Summary:         traceQueryTypedCPUConstraintSummary(constraint),
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"thread", traceThreadLabel(constraint.Thread)},
				{"kind", constraint.Kind},
				{"allowed_cpus", traceIntList(constraint.AllowedCPUs)},
				{"allowed_core_classes", strings.Join(constraint.AllowedCoreClasses, ",")},
				{"cpuset", constraint.CPUSet},
				{"policy", constraint.Policy},
				{"observed_cpu", traceKnownCPU(constraint.ObservedCPUKnown, constraint.ObservedCPU)},
				{"observed_core_class", constraint.ObservedCoreClass},
				{"migrations", traceQueryTypedCount(constraint.MigrationCount)},
				{"runnable", traceQueryObservationMSValue(constraint.RunnableWaitMs)},
				{"other_cpu_idle", traceQueryObservationMSValue(constraint.OtherCPUIdleMs)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, constraint.LineStart, constraint.LineEnd),
			ObservedAt:  at,
			Confidence:  0.72,
		})
	}

	for i, ctx := range stats.RunnableContext {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(ctx.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#runnable_context:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: ctx.LineStart, LineEnd: ctx.LineEnd},
			ClaimKey:        "runnable_context:" + traceThreadLabel(ctx.Thread),
			Subject:         traceThreadLabel(ctx.Thread),
			Predicate:       "runnable_context",
			Object:          ctx.Verdict,
			Value:           traceQueryObservationMSValue(ctx.RunnableWaitMs),
			Unit:            "ms",
			Summary:         traceQueryTypedRunnableContextSummary(ctx),
			RichNotes:       traceQueryTypedRunnableContextNotes(ctx),
			SupportRefs:     traceQueryObservationSupportRefs(ref, ctx.LineStart, ctx.LineEnd),
			ObservedAt:      at,
			Confidence:      ctx.Confidence,
		})
	}

	for i, proc := range stats.ProcessCPULoad {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(proc.Summary) == "" {
			continue
		}
		total := proc.RunningMs + proc.RunnableWaitMs
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#process_cpu_load:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: proc.LineStart, LineEnd: proc.LineEnd},
			ClaimKey:        "process_cpu_load:" + traceThreadLabel(proc.Process),
			Subject:         traceThreadLabel(proc.Process),
			Predicate:       "process_cpu_load",
			Object:          traceThreadLabel(proc.TopThread),
			Value:           traceQueryObservationMSValue(total),
			Unit:            "ms",
			Summary:         traceQueryTypedProcessCPULoadSummary(proc),
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"process", traceThreadLabel(proc.Process)},
				{"threads", traceQueryTypedCount(proc.ThreadCount)},
				{"running", traceQueryObservationMSValue(proc.RunningMs)},
				{"runnable", traceQueryObservationMSValue(proc.RunnableWaitMs)},
				{"high_prio_running", traceQueryObservationMSValue(proc.HighPriorityRunningMs)},
				{"top_thread", traceThreadLabel(proc.TopThread)},
				{"top_thread_ms", traceQueryObservationMSValue(proc.TopThreadMs)},
				{"cpus", traceIntList(proc.CPUs)},
				{"core_classes", strings.Join(proc.CoreClasses, ",")},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, proc.LineStart, proc.LineEnd),
			ObservedAt:  at,
			Confidence:  0.66,
		})
	}

	for i, churn := range stats.StateChurn {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(churn.DominantState) == "" && strings.TrimSpace(churn.Summary) == "" {
			continue
		}
		notes := []string{}
		appendNote := func(key, value string) {
			if strings.TrimSpace(value) != "" {
				notes = append(notes, key+"="+value)
			}
		}
		appendNote("dominant_state", churn.DominantState)
		if churn.FragmentCount > 0 {
			appendNote("fragments", strconv.Itoa(churn.FragmentCount))
		}
		if churn.StateSwitches > 0 {
			appendNote("switches", strconv.Itoa(churn.StateSwitches))
		}
		appendNote("max_segment", traceQueryObservationMSValue(churn.MaxSegmentMs))
		appendNote("p95_segment", traceQueryObservationMSValue(churn.P95SegmentMs))
		appendNote("running", traceQueryObservationMSValue(churn.RunningMs))
		appendNote("runnable", traceQueryObservationMSValue(churn.RunnableMs))
		appendNote("sleep", traceQueryObservationMSValue(churn.SleepMs))
		appendNote("d_state", traceQueryObservationMSValue(churn.DStateMs))
		appendNote("io_wait", traceQueryObservationMSValue(churn.IOWaitMs))
		if churn.RunnableCPUKnown {
			appendNote("runnable_cpu", strconv.Itoa(churn.RunnableCPU))
		}
		appendNote("top_competitor", churn.TopCompetitor)
		appendNote("top_competitor_running", traceQueryObservationMSValue(churn.TopCompetitorRunningMs))
		appendNote("next_step", churn.NextStep)
		if churn.TotalMs > 0 {
			notes = append(notes, fmt.Sprintf("total=%.3fms", churn.TotalMs))
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#state_churn:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: churn.LineStart, LineEnd: churn.LineEnd},
			ClaimKey:        "state_churn:" + churn.DominantState,
			Subject:         traceThreadLabel(churn.Thread),
			Predicate:       "state_churn",
			Object:          churn.DominantState,
			Value:           traceQueryObservationMSValue(churn.DominantImpactMs),
			Unit:            "ms",
			Summary:         churn.Summary,
			RichNotes:       notes,
			SupportRefs:     traceQueryObservationSupportRefs(ref, churn.LineStart, churn.LineEnd),
			ObservedAt:      at,
			Confidence:      churn.Confidence,
		})
	}

	for i, file := range stats.FileIOByInode {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(file.Inode) == "" && strings.TrimSpace(file.Summary) == "" {
			continue
		}
		value := ""
		if file.Bytes > 0 {
			value = strconv.FormatInt(file.Bytes, 10)
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#file_io:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: file.LineStart, LineEnd: file.LineEnd, StartTs: file.StartTs, EndTs: file.EndTs},
			ClaimKey:        "file_io:" + firstNonEmptyTraceString(file.Inode, file.EntryName, file.Operation),
			Subject:         firstNonEmptyTraceString(file.EntryName, "inode="+file.Inode),
			Predicate:       "file_io_by_inode",
			Object:          file.Operation,
			Value:           value,
			Unit:            "bytes",
			Summary:         traceQueryTypedFileIOSummary(file),
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"inode", file.Inode},
				{"dev", file.Dev},
				{"name", file.EntryName},
				{"op", file.Operation},
				{"thread", traceThreadLabel(file.Thread)},
				{"count", traceQueryTypedCount(file.Count)},
				{"completions", traceQueryTypedCount(file.CompletionCount)},
				{"bytes", value},
				{"total_latency", traceQueryObservationMSValue(file.TotalLatencyMs)},
				{"max_latency", traceQueryObservationMSValue(file.MaxLatencyMs)},
				{"ret", traceQueryTypedInt64(file.Ret)},
				{"offsets", traceQueryTypedOffsetRange(file.MinOffset, file.MaxOffset)},
				{"example", file.Example},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, file.LineStart, file.LineEnd),
			ObservedAt:  at,
			Confidence:  0.74,
		})
	}

	for i, cache := range stats.PageCacheByInode {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(cache.Inode) == "" && strings.TrimSpace(cache.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#page_cache:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: cache.LineStart, LineEnd: cache.LineEnd, StartTs: cache.StartTs, EndTs: cache.EndTs},
			ClaimKey:        "page_cache:" + firstNonEmptyTraceString(cache.Inode, cache.Dev),
			Subject:         "inode=" + cache.Inode,
			Predicate:       "page_cache_by_inode",
			Object:          cache.Dev,
			Value:           traceQueryTypedCount(cache.Churn),
			Unit:            "events",
			Summary:         cache.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"inode", cache.Inode},
				{"dev", cache.Dev},
				{"thread", traceThreadLabel(cache.Thread)},
				{"adds", traceQueryTypedCount(cache.Adds)},
				{"deletes", traceQueryTypedCount(cache.Deletes)},
				{"churn", traceQueryTypedCount(cache.Churn)},
				{"bytes", traceQueryTypedInt64(cache.Bytes)},
				{"offsets", traceQueryTypedOffsetRange(cache.MinOffset, cache.MaxOffset)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, cache.LineStart, cache.LineEnd),
			ObservedAt:  at,
			Confidence:  0.70,
		})
	}

	for i, storage := range stats.StorageLatencyByLayer {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(storage.Layer) == "" && strings.TrimSpace(storage.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#storage_latency:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: storage.LineStart, LineEnd: storage.LineEnd, StartTs: storage.StartTs, EndTs: storage.EndTs},
			ClaimKey:        "storage_latency:" + firstNonEmptyTraceString(storage.Layer, storage.Event),
			Subject:         storage.Layer,
			Predicate:       "storage_latency_by_layer",
			Object:          storage.Event,
			Value:           traceQueryObservationMSValue(storage.MaxLatencyMs),
			Unit:            "ms",
			Summary:         traceQueryTypedStorageLatencySummary(storage),
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"layer", storage.Layer},
				{"event", storage.Event},
				{"dev", storage.Dev},
				{"op", storage.Operation},
				{"thread", traceThreadLabel(storage.Thread)},
				{"count", traceQueryTypedCount(storage.Count)},
				{"paired", traceQueryTypedCount(storage.PairedCount)},
				{"unpaired_start", traceQueryTypedCount(storage.UnpairedStartCount)},
				{"unpaired_done", traceQueryTypedCount(storage.UnpairedDoneCount)},
				{"max_latency", traceQueryObservationMSValue(storage.MaxLatencyMs)},
				{"avg_latency", traceQueryObservationMSValue(storage.AvgLatencyMs)},
				{"bytes", traceQueryTypedInt64(storage.Bytes)},
				{"example", storage.Example},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, storage.LineStart, storage.LineEnd),
			ObservedAt:  at,
			Confidence:  0.72,
		})
	}

	if pressure := stats.IOPressureSummary; pressure != nil &&
		(strings.TrimSpace(pressure.Signal) != "" || strings.TrimSpace(pressure.Summary) != "") {
		value := ""
		if pressure.Score > 0 {
			value = fmt.Sprintf("%.3f", pressure.Score)
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#io_pressure:1", scope),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: pressure.LineStart, LineEnd: pressure.LineEnd},
			ClaimKey:        "io_pressure:" + pressure.Signal,
			Subject:         "io_pressure",
			Predicate:       pressure.Signal,
			Object:          pressure.TopInode,
			Value:           value,
			Unit:            "score",
			Summary:         traceQueryTypedIOPressureSummary(*pressure),
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"signal", pressure.Signal},
				{"score", value},
				{"block_max", traceQueryObservationMSValue(pressure.BlockMaxLatencyMs)},
				{"storage_max", traceQueryObservationMSValue(pressure.StorageMaxLatencyMs)},
				{"file_bytes", traceQueryTypedInt64(pressure.FileIOBytes)},
				{"file_events", traceQueryTypedCount(pressure.FileIOEvents)},
				{"page_cache_churn", traceQueryTypedCount(pressure.PageCacheChurn)},
				{"iowait_blocked", traceQueryTypedCount(pressure.IOWaitBlockedCount)},
				{"d_state", traceQueryObservationMSValue(pressure.DStateMs)},
				{"top_inode", pressure.TopInode},
				{"top_dev", pressure.TopDev},
				{"top_name", pressure.TopEntryName},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, pressure.LineStart, pressure.LineEnd),
			ObservedAt:  at,
			Confidence:  0.70,
		})
	}

	for i, episode := range stats.IOBurstEpisodes {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(episode.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#io_burst_episode:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: episode.LineStart, LineEnd: episode.LineEnd, StartTs: episode.StartTs, EndTs: episode.EndTs},
			ClaimKey:        "io_burst_episode:" + firstNonEmptyTraceString(episode.DominantSignal, episode.TopInode),
			Subject:         traceThreadLabel(episode.Thread),
			Predicate:       "io_burst_episode",
			Object:          episode.TopInode,
			Value:           traceQueryObservationMSValue(episode.DurationMs),
			Unit:            "ms",
			Summary:         episode.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"chain_relevance", episode.ChainRelevance},
				{"signal", episode.DominantSignal},
				{"d_state", traceQueryObservationMSValue(episode.DStateMs)},
				{"io_wait", traceQueryObservationMSValue(episode.IOWaitMs)},
				{"block_max", traceQueryObservationMSValue(episode.BlockMaxLatencyMs)},
				{"storage_max", traceQueryObservationMSValue(episode.StorageMaxLatencyMs)},
				{"inode", episode.TopInode},
				{"dev", episode.TopDev},
				{"name", episode.TopEntryName},
				{"file_bytes", traceQueryTypedInt64(episode.FileIOBytes)},
				{"page_cache_churn", traceQueryTypedCount(episode.PageCacheChurn)},
				{"overlap", traceQueryObservationMSValue(episode.OverlapMs)},
				{"nearest_chain_thread", traceThreadLabelOptional(episode.NearestChainThread)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, episode.LineStart, episode.LineEnd),
			ObservedAt:  at,
			Confidence:  episode.Confidence,
		})
	}

	for i, inode := range stats.BlockIOByInode {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(inode.Inode) == "" && strings.TrimSpace(inode.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#block_io_by_inode:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: inode.LineStart, LineEnd: inode.LineEnd, StartTs: inode.StartTs, EndTs: inode.EndTs},
			ClaimKey:        "block_io_by_inode:" + firstNonEmptyTraceString(inode.Inode, inode.EntryName),
			Subject:         firstNonEmptyTraceString(inode.EntryName, "inode="+inode.Inode),
			Predicate:       "block_io_by_inode",
			Object:          inode.BlockDev,
			Value:           traceQueryObservationMSValue(firstPositiveTraceFloat(inode.BlockMaxLatencyMs, inode.StorageMaxLatencyMs)),
			Unit:            "ms",
			Summary:         inode.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"inode", inode.Inode},
				{"dev", inode.Dev},
				{"name", inode.EntryName},
				{"thread", traceThreadLabel(inode.Thread)},
				{"block_dev", inode.BlockDev},
				{"op", inode.Operation},
				{"file_bytes", traceQueryTypedInt64(inode.FileIOBytes)},
				{"page_cache_churn", traceQueryTypedCount(inode.PageCacheChurn)},
				{"block_max", traceQueryObservationMSValue(inode.BlockMaxLatencyMs)},
				{"storage_max", traceQueryObservationMSValue(inode.StorageMaxLatencyMs)},
				{"nearest_block_thread", traceThreadLabelOptional(inode.NearestBlockThread)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, inode.LineStart, inode.LineEnd),
			ObservedAt:  at,
			Confidence:  inode.Confidence,
		})
	}

	out = append(out, traceQueryTypedInterruptObservations("irq_activity", stats.IRQActivity, ref, scope, at)...)
	out = append(out, traceQueryTypedInterruptObservations("softirq_activity", stats.SoftIRQActivity, ref, scope, at)...)
	for i, work := range stats.WorkqueueActivity {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(work.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#workqueue_activity:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: work.LineStart, LineEnd: work.LineEnd, StartTs: work.StartTs, EndTs: work.EndTs},
			ClaimKey:        "workqueue_activity:" + firstNonEmptyTraceString(work.Function, work.Work),
			Subject:         traceThreadLabel(work.Thread),
			Predicate:       "workqueue_activity",
			Object:          firstNonEmptyTraceString(work.Function, work.Work),
			Value:           traceQueryObservationMSValue(work.DurationMs),
			Unit:            "ms",
			Summary:         work.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"work", work.Work},
				{"function", work.Function},
				{"count", traceQueryTypedCount(work.Count)},
				{"paired", traceQueryTypedCount(work.PairedCount)},
				{"max", traceQueryObservationMSValue(work.MaxLatencyMs)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, work.LineStart, work.LineEnd),
			ObservedAt:  at,
			Confidence:  0.64,
		})
	}
	if supply := stats.SupplyPressureSummary; supply != nil && strings.TrimSpace(supply.Summary) != "" {
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#supply_pressure:1", scope),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: supply.LineStart, LineEnd: supply.LineEnd},
			ClaimKey:        "supply_pressure:" + supply.Signal,
			Subject:         "supply_pressure",
			Predicate:       supply.Signal,
			Value:           traceQueryObservationMSValue(supply.CPUPressureMs),
			Unit:            "ms",
			Summary:         supply.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"runnable", traceQueryObservationMSValue(supply.RunnableWaitMs)},
				{"high_prio", traceQueryObservationMSValue(supply.HighPriorityRunningMs)},
				{"low_freq_cpus", traceIntList(supply.LowFrequencyCPUs)},
				{"clock_set_rate", traceQueryTypedCount(supply.ClockSetRateCount)},
				{"thermal", traceQueryTypedCount(supply.ThermalEventCount)},
				{"ddr", traceQueryTypedCount(supply.DDREventCount)},
				{"l3", traceQueryTypedCount(supply.L3EventCount)},
				{"throughput", traceQueryTypedCount(supply.ThroughputEventCount)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, supply.LineStart, supply.LineEnd),
			ObservedAt:  at,
			Confidence:  0.62,
		})
	}
	for i, category := range stats.TraceMarkCategories {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#trace_mark_category:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: category.LineStart, LineEnd: category.LineEnd},
			ClaimKey:        "trace_mark_category:" + firstNonEmptyTraceString(category.Category, category.Subcategory),
			Subject:         category.Category,
			Predicate:       "trace_mark_category",
			Object:          category.TopSpan,
			Value:           traceQueryObservationMSValue(category.TotalMs),
			Unit:            "ms",
			Summary:         category.Summary,
			SupportRefs:     traceQueryObservationSupportRefs(ref, category.LineStart, category.LineEnd),
			ObservedAt:      at,
			Confidence:      0.62,
		})
	}
	for i, work := range stats.AsyncFileWork {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#async_file_work:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: work.LineStart, LineEnd: work.LineEnd, StartTs: work.StartTs, EndTs: work.EndTs},
			ClaimKey:        "async_file_work:" + work.Name,
			Subject:         traceThreadLabel(work.Thread),
			Predicate:       "async_file_work",
			Object:          work.Name,
			Value:           traceQueryObservationMSValue(work.DurationMs),
			Unit:            "ms",
			Summary:         work.Summary,
			SupportRefs:     traceQueryObservationSupportRefs(ref, work.LineStart, work.LineEnd),
			ObservedAt:      at,
			Confidence:      0.64,
		})
	}

	out = append(out, traceQueryTypedResourceObservations("bio", stats.BIOResources, ref, scope, at)...)
	out = append(out, traceQueryTypedResourceObservations("filesystem", stats.FilesystemResources, ref, scope, at)...)
	out = append(out, traceQueryTypedResourceObservations("page_fault", stats.PageFaultResources, ref, scope, at)...)
	out = append(out, traceQueryTypedPluginObservations(stats, ref, scope, at)...)
	return out
}

func traceQueryTypedResourceObservations(label string, items []tracequery.RuntimeResourceSummary, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	var out []types.ObservationRecord
	for i, item := range items {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(item.Path) == "" && strings.TrimSpace(item.Operation) == "" {
			continue
		}
		notes := traceQueryTypedKVNotes([][2]string{
			{"op", item.Operation},
			{"path", item.Path},
			{"thread", traceThreadLabel(item.Thread)},
			{"count", traceQueryTypedCount(item.Count)},
			{"total_latency", traceQueryObservationMSValue(item.TotalLatencyMs)},
			{"max_latency", traceQueryObservationMSValue(item.MaxLatencyMs)},
			{"bytes", traceQueryTypedInt64(item.Bytes)},
			{"line", traceQueryTypedCount(item.Line)},
		})
		if strings.TrimSpace(item.Callstack) != "" {
			notes = append(notes, "callstack="+sanitizeForBanner(item.Callstack))
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#%s_resource:%d", scope, label, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: item.Line, LineEnd: item.Line},
			ClaimKey:        label + "_resource:" + firstNonEmptyTraceString(item.Path, item.Operation),
			Subject:         firstNonEmptyTraceString(item.Path, label+"_resource"),
			Predicate:       label + "_resource",
			Object:          item.Operation,
			Value:           traceQueryObservationMSValue(item.TotalLatencyMs),
			Unit:            "ms",
			Summary:         traceQueryTypedResourceSummary(label, item),
			RichNotes:       notes,
			SupportRefs:     traceQueryObservationSupportRefs(ref, item.Line, item.Line),
			ObservedAt:      at,
			Confidence:      0.68,
		})
	}
	return out
}

func traceQueryTypedInterruptObservations(label string, items []tracequery.InterruptActivity, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	var out []types.ObservationRecord
	for i, item := range items {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(item.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#%s:%d", scope, label, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: item.LineStart, LineEnd: item.LineEnd, StartTs: item.StartTs, EndTs: item.EndTs},
			ClaimKey:        label + ":" + firstNonEmptyTraceString(item.Name, strconv.Itoa(item.Vector)),
			Subject:         fmt.Sprintf("cpu=%d", item.CPU),
			Predicate:       label,
			Object:          item.Name,
			Value:           traceQueryObservationMSValue(item.ActiveMs),
			Unit:            "ms",
			Summary:         item.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"kind", item.Kind},
				{"core_class", item.CoreClass},
				{"vector", traceQueryTypedCount(item.Vector)},
				{"count", traceQueryTypedCount(item.Count)},
				{"paired", traceQueryTypedCount(item.PairedCount)},
				{"max", traceQueryObservationMSValue(item.MaxActiveMs)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, item.LineStart, item.LineEnd),
			ObservedAt:  at,
			Confidence:  0.60,
		})
	}
	return out
}

func traceQueryTypedFileIOSummary(item tracequery.FileIOSummary) string {
	parts := []string{"file_io_by_inode"}
	for _, kv := range [][2]string{
		{"inode", item.Inode},
		{"dev", item.Dev},
		{"name", item.EntryName},
		{"op", item.Operation},
		{"bytes", traceQueryTypedInt64(item.Bytes)},
		{"count", traceQueryTypedCount(item.Count)},
		{"completions", traceQueryTypedCount(item.CompletionCount)},
		{"total_latency", traceQueryObservationMSValue(item.TotalLatencyMs)},
		{"max_latency", traceQueryObservationMSValue(item.MaxLatencyMs)},
		{"ret", traceQueryTypedInt64(item.Ret)},
		{"offsets", traceQueryTypedOffsetRange(item.MinOffset, item.MaxOffset)},
		{"example", item.Example},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+sanitizeForBanner(kv[1]))
		}
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		parts = append(parts, "detail="+sanitizeForBanner(summary))
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedStorageLatencySummary(item tracequery.StorageLatencySummary) string {
	parts := []string{"storage_latency_by_layer"}
	for _, kv := range [][2]string{
		{"layer", item.Layer},
		{"event", item.Event},
		{"dev", item.Dev},
		{"op", item.Operation},
		{"thread", traceThreadLabel(item.Thread)},
		{"count", traceQueryTypedCount(item.Count)},
		{"paired", traceQueryTypedCount(item.PairedCount)},
		{"unpaired_start", traceQueryTypedCount(item.UnpairedStartCount)},
		{"unpaired_done", traceQueryTypedCount(item.UnpairedDoneCount)},
		{"max_latency", traceQueryObservationMSValue(item.MaxLatencyMs)},
		{"avg_latency", traceQueryObservationMSValue(item.AvgLatencyMs)},
		{"bytes", traceQueryTypedInt64(item.Bytes)},
		{"example", item.Example},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+sanitizeForBanner(kv[1]))
		}
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		parts = append(parts, "detail="+sanitizeForBanner(summary))
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedIOPressureSummary(item tracequery.IOPressureSummary) string {
	parts := []string{"io_pressure_summary"}
	for _, kv := range [][2]string{
		{"signal", item.Signal},
		{"score", traceQueryTypedFloat(item.Score)},
		{"top_inode", item.TopInode},
		{"top_dev", item.TopDev},
		{"top_name", item.TopEntryName},
		{"file_bytes", traceQueryTypedInt64(item.FileIOBytes)},
		{"file_events", traceQueryTypedCount(item.FileIOEvents)},
		{"page_cache_churn", traceQueryTypedCount(item.PageCacheChurn)},
		{"storage_max", traceQueryObservationMSValue(item.StorageMaxLatencyMs)},
		{"block_max", traceQueryObservationMSValue(item.BlockMaxLatencyMs)},
		{"iowait_blocked", traceQueryTypedCount(item.IOWaitBlockedCount)},
		{"d_state", traceQueryObservationMSValue(item.DStateMs)},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+sanitizeForBanner(kv[1]))
		}
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		parts = append(parts, "detail="+sanitizeForBanner(summary))
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedThreadCPULoadSummary(item tracequery.ThreadCPULoadSummary) string {
	parts := []string{"thread_cpu_load"}
	for _, kv := range [][2]string{
		{"thread", traceThreadLabel(item.Thread)},
		{"running", traceQueryObservationMSValue(item.RunningMs)},
		{"runnable", traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"high_prio_running", traceQueryObservationMSValue(item.HighPriorityRunningMs)},
		{"cpu", strconv.Itoa(item.CPU)},
		{"core_class", item.CoreClass},
		{"freq", traceQueryTypedCount(item.Frequency)},
		{"priority", traceQueryPriorityPair(item.Priority, item.PriorityClass)},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+sanitizeForBanner(kv[1]))
		}
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedCPUConstraintSummary(item tracequery.CPUConstraintSummary) string {
	parts := []string{"cpu_constraint"}
	for _, kv := range [][2]string{
		{"thread", traceThreadLabel(item.Thread)},
		{"kind", item.Kind},
		{"allowed_cpus", traceIntList(item.AllowedCPUs)},
		{"allowed_core_classes", strings.Join(item.AllowedCoreClasses, ",")},
		{"cpuset", item.CPUSet},
		{"policy", item.Policy},
		{"observed_cpu", traceKnownCPU(item.ObservedCPUKnown, item.ObservedCPU)},
		{"observed_core_class", item.ObservedCoreClass},
		{"migrations", traceQueryTypedCount(item.MigrationCount)},
		{"runnable", traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"other_cpu_idle", traceQueryObservationMSValue(item.OtherCPUIdleMs)},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+sanitizeForBanner(kv[1]))
		}
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		parts = append(parts, "detail="+sanitizeForBanner(summary))
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedRunnableContextSummary(item tracequery.RunnableContextSummary) string {
	parts := []string{"runnable_context"}
	for _, kv := range [][2]string{
		{"thread", traceThreadLabel(item.Thread)},
		{"runnable", traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"cpu", strconv.Itoa(item.CPU)},
		{"core_class", item.CoreClass},
		{"freq", traceQueryTypedCount(item.Frequency)},
		{"same_cpu_busy", traceQueryObservationMSValue(item.SameCPUBusyMs)},
		{"same_cpu_idle", traceQueryObservationMSValue(item.SameCPUIdleMs)},
		{"other_cpu_idle", traceQueryObservationMSValue(item.OtherCPUIdleMs)},
		{"high_prio_running", traceQueryObservationMSValue(item.HighPriorityRunningMs)},
		{"top_background_threads", traceQueryRunnableContextBackgroundThreads(item.TopBackgroundThreads)},
		{"top_background_process", traceQueryRunnableContextBackgroundProcess(item.TopBackgroundProcess)},
		{"constraint", traceQueryRunnableContextConstraint(item.CPUConstraint)},
		{"verdict", item.Verdict},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+sanitizeForBanner(kv[1]))
		}
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		parts = append(parts, "detail="+sanitizeForBanner(summary))
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedRunnableContextNotes(item tracequery.RunnableContextSummary) []string {
	return traceQueryTypedKVNotes([][2]string{
		{"thread", traceThreadLabel(item.Thread)},
		{"runnable", traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"cpu", strconv.Itoa(item.CPU)},
		{"core_class", item.CoreClass},
		{"freq", traceQueryTypedCount(item.Frequency)},
		{"priority", traceQueryPriorityPair(item.Priority, item.PriorityClass)},
		{"same_cpu_busy", traceQueryObservationMSValue(item.SameCPUBusyMs)},
		{"same_cpu_idle", traceQueryObservationMSValue(item.SameCPUIdleMs)},
		{"other_cpu_idle", traceQueryObservationMSValue(item.OtherCPUIdleMs)},
		{"high_prio_running", traceQueryObservationMSValue(item.HighPriorityRunningMs)},
		{"top_background_threads", traceQueryRunnableContextBackgroundThreads(item.TopBackgroundThreads)},
		{"top_background_process", traceQueryRunnableContextBackgroundProcess(item.TopBackgroundProcess)},
		{"constraint", traceQueryRunnableContextConstraint(item.CPUConstraint)},
		{"verdict", item.Verdict},
	})
}

func traceQueryTypedProcessCPULoadSummary(item tracequery.ProcessCPULoadSummary) string {
	parts := []string{"process_cpu_load"}
	for _, kv := range [][2]string{
		{"process", traceThreadLabel(item.Process)},
		{"threads", traceQueryTypedCount(item.ThreadCount)},
		{"running", traceQueryObservationMSValue(item.RunningMs)},
		{"runnable", traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"high_prio_running", traceQueryObservationMSValue(item.HighPriorityRunningMs)},
		{"top_thread", traceThreadLabel(item.TopThread)},
		{"top_thread_ms", traceQueryObservationMSValue(item.TopThreadMs)},
		{"cpus", traceIntList(item.CPUs)},
		{"core_classes", strings.Join(item.CoreClasses, ",")},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+sanitizeForBanner(kv[1]))
		}
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		parts = append(parts, "detail="+sanitizeForBanner(summary))
	}
	return strings.Join(parts, " ")
}

func traceQueryRunnableContextBackgroundThreads(items []tracequery.ThreadCPULoadSummary) string {
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for i, item := range items {
		if i >= 4 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s/%.3fms", traceThreadLabel(item.Thread), item.RunningMs+item.RunnableWaitMs))
	}
	return strings.Join(parts, ",")
}

func traceQueryRunnableContextBackgroundProcess(item *tracequery.ProcessCPULoadSummary) string {
	if item == nil {
		return ""
	}
	return fmt.Sprintf("%s/%.3fms", traceThreadLabel(item.Process), item.RunningMs+item.RunnableWaitMs)
}

func traceQueryRunnableContextConstraint(item *tracequery.CPUConstraintSummary) string {
	if item == nil {
		return ""
	}
	parts := []string{}
	if cpus := traceIntList(item.AllowedCPUs); cpus != "" {
		parts = append(parts, "allowed_cpus="+cpus)
	}
	if len(item.AllowedCoreClasses) > 0 {
		parts = append(parts, "allowed_core_classes="+strings.Join(item.AllowedCoreClasses, ","))
	}
	if item.CPUSet != "" {
		parts = append(parts, "cpuset="+item.CPUSet)
	}
	if item.Policy != "" {
		parts = append(parts, "policy="+item.Policy)
	}
	return strings.Join(parts, ";")
}

func traceQueryTypedResourceSummary(label string, item tracequery.RuntimeResourceSummary) string {
	parts := []string{label + "_resource"}
	for _, kv := range [][2]string{
		{"op", item.Operation},
		{"path", item.Path},
		{"total_latency", traceQueryObservationMSValue(item.TotalLatencyMs)},
		{"max_latency", traceQueryObservationMSValue(item.MaxLatencyMs)},
		{"bytes", traceQueryTypedInt64(item.Bytes)},
		{"count", traceQueryTypedCount(item.Count)},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+kv[1])
		}
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedPluginObservations(stats tracequery.WindowStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	var out []types.ObservationRecord
	ordinal := 0
	for _, group := range [][]tracequery.TracePluginSummary{stats.AbilityEvents, stats.XPowerEvents, stats.HiSystemEvents} {
		for _, item := range group {
			if ordinal >= traceQueryTypedFamilyRowCap {
				return out
			}
			if strings.TrimSpace(item.Kind) == "" && strings.TrimSpace(item.EventName) == "" {
				continue
			}
			ordinal++
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#plugin_event:%d", scope, ordinal),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingSoft,
				ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
				SourceRef:       ref,
				Span:            types.ObservationSpan{LineStart: item.Line, LineEnd: item.Line},
				ClaimKey:        "plugin_event:" + firstNonEmptyTraceString(item.Kind, item.Domain, item.EventName, item.Metric),
				Subject:         firstNonEmptyTraceString(item.Domain, item.Kind, "plugin_event"),
				Predicate:       firstNonEmptyTraceString(item.Kind, "plugin_event"),
				Object:          firstNonEmptyTraceString(item.EventName, item.Metric),
				Value:           item.Value,
				Summary:         traceQueryTypedPluginSummary(item),
				RichNotes: traceQueryTypedKVNotes([][2]string{
					{"kind", item.Kind},
					{"domain", item.Domain},
					{"event", item.EventName},
					{"metric", item.Metric},
					{"value", item.Value},
					{"category", item.Category},
					{"thread", traceThreadLabel(item.Thread)},
					{"count", traceQueryTypedCount(item.Count)},
					{"line", traceQueryTypedCount(item.Line)},
				}),
				SupportRefs: traceQueryObservationSupportRefs(ref, item.Line, item.Line),
				ObservedAt:  at,
				Confidence:  0.68,
			})
		}
	}
	return out
}

func traceQueryTypedPluginSummary(item tracequery.TracePluginSummary) string {
	parts := []string{"plugin_event"}
	for _, kv := range [][2]string{
		{"kind", item.Kind},
		{"domain", item.Domain},
		{"event", item.EventName},
		{"metric", item.Metric},
		{"value", item.Value},
		{"category", item.Category},
		{"count", traceQueryTypedCount(item.Count)},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+kv[1])
		}
	}
	return strings.Join(parts, " ")
}

func traceQueryTypedKVNotes(pairs [][2]string) []string {
	var notes []string
	for _, kv := range pairs {
		if strings.TrimSpace(kv[1]) == "" {
			continue
		}
		notes = append(notes, kv[0]+"="+kv[1])
	}
	return notes
}

func traceQueryTypedOffsetRange(minOffset, maxOffset int64) string {
	if minOffset == 0 && maxOffset == 0 {
		return ""
	}
	return fmt.Sprintf("%d..%d", minOffset, maxOffset)
}

func traceQueryTypedCount(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func traceQueryTypedBool(v bool) string {
	if !v {
		return ""
	}
	return "true"
}

func traceQueryPriorityPair(priority int, class string) string {
	if priority <= 0 && strings.TrimSpace(class) == "" {
		return ""
	}
	if strings.TrimSpace(class) == "" {
		return strconv.Itoa(priority)
	}
	return fmt.Sprintf("%d/%s", priority, sanitizeForBanner(class))
}

func traceQueryCausalityLabel(onChain bool) string {
	if onChain {
		return "on_wakeup_chain"
	}
	return "background"
}

func traceQueryTypedInt64(n int64) string {
	if n <= 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

func traceQueryTypedFloat(v float64) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3f", v)
}

func firstPositiveTraceFloat(values ...float64) float64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func traceQueryTypedTimeWindow(w tracequery.TimeWindow) string {
	if w.StartTs == 0 && w.EndTs == 0 {
		return ""
	}
	return fmt.Sprintf("%.6f..%.6f", w.StartTs, w.EndTs)
}
