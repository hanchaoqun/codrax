package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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

// traceQueryScopedIndexMaxBytes is the in-memory byte budget for a single,
// deliberate pid/thread-scoped heavy-view index. A pinned pid+window query
// (e.g. root_cause_rank on one frame's thread) is issued one view at a time —
// unlike 16-way-parallel grep — so it can afford more headroom than the shared
// default event cap without OOM risk. The effective event ceiling is
// budget / traceIndexEventSizeEstimateBytes (~524K events at 1 GiB), roughly
// 2x the default 250K, letting a dense GB-trace window's heavy views run
// instead of failing outright with IndexEventLimitError. Unscoped / non-heavy
// calls keep the default cap. Tunable in tests.
var traceQueryScopedIndexMaxBytes int64 = 1 << 30

// traceIndexEventSizeEstimateBytes conservatively approximates the retained
// cost of one parsed tracequery.Event (the flat struct is ~1.7 KiB; the extra
// budget covers string backing arrays). Used only to translate the scoped byte
// budget into an event count, never as an exact allocator figure.
const traceIndexEventSizeEstimateBytes = 2048

var (
	traceQueryLargeRecipeDiscoveryMinBytes      int64 = 128 << 20
	traceQueryWindowedIndexMinBytes             int64 = 64 << 20
	traceQueryMicroWindowProbeSeconds                 = 0.050
	traceQueryPreferredCoverageWindowMinSeconds       = 0.080
	traceQueryPreferredCoverageWindowMaxSeconds       = 0.150
	traceQueryParentWindowStrategySeconds             = 1.000
	traceQueryObjectiveKVTokenRE                      = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_./:-]*=[^\s,，。；;"'）)]+`)
	traceQueryObjectiveFrameIDRE                      = regexp.MustCompile(`(?i)Choreographer#doFrame\D{0,32}([0-9]{3,})`)
	traceQueryObjectiveHexTokenRE                     = regexp.MustCompile(`(?i)\b0x[0-9a-f]{4,}\b`)
	traceQueryObjectiveQuotedTokenRE                  = regexp.MustCompile(`"([^"\n]{3,160})"|“([^”\n]{3,160})”|'([^'\n]{3,160})'|‘([^’\n]{3,160})’`)
	traceQueryObjectiveLabeledTokenRE                 = regexp.MustCompile(`(?i)(?:span(?:_name)?(?:\s*关键字)?|marker|label|keyword|关键字|标记|标签|span名|span名称)\s*(?:=|:|：|为|是|叫|名为)?\s*([A-Za-z0-9_#./:$@+\-]{3,160})`)
	traceQueryObjectivePreLabeledTokenRE              = regexp.MustCompile(`(?i)([A-Za-z0-9_#./:$@+\-]{3,160})\s*(?:这个|该|此)?\s*(?:span|marker|label|keyword|关键字|标记|标签)`)
	traceQueryTimestampRE                             = regexp.MustCompile(`\s([0-9]+(?:\.[0-9]+)?):\s+`)
)

func traceQueryMemoryForLog() (heapAlloc, heapSys uint64, gcCount uint32) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc, stats.HeapSys, stats.NumGC
}

func (t *TraceQuery) Name() string { return "trace_query" }

func (t *TraceQuery) Description() string {
	description := strings.Replace("Deterministically queries large runtime trace/log artifacts for scheduler timelines, scheduler latency stats, trace span/frame windows, frame timelines/flows, render pipelines, ranked root causes, wakeup chains, frame root-cause bundles, binder IPC graphs with explicit oneway/sync_like/blocking_candidate fields, critical blocking calls, interaction Top-N, same-window resource stats, recipes, structured event search, and line-backed evidence packs. Path inputs may be .ftrace/.trace/.systrace/.htrace/.atrace/.perftrace or .tracebundle.json; trace_query automatically promotes sibling .tracebundle.json and merges sibling .systrace+.perftrace pairs, so one path can carry joint trace+perf evidence. wakeup_chain/root_cause_rank/frame_root_cause_bundle publish structured wakeup_chain path, per-edge wakeup_chain_edge rows, causal_impact rows, and chain_relevance fields (on_chain, adjacent, background); consume those ordered path/edge/relevance fields before paraphrasing dependency chains so upstream waker -> intermediate dependency -> target causality is not lost in prose and off-chain background load is not promoted to primary cause. root_cause_rank rows carry projected_impact_ms/projected_total_ms for the impact projected into the selected target/wakeup-chain window, actual_impact_ms/actual_total_ms/actual_window for the underlying scheduler state segment that may extend outside that projection, plus cumulative_impact_ms, effective_impact_ms, dominant_state, and running/runnable/sleep/d_state/io_wait totals; semantic span-work candidates add span_name/span_kind/span_category/span_subcategory/semantic_class/effective_impact_ms for system-classified on-chain runtime work such as JIT compilation, class verification, shader compilation, and runtime compilation, while generic trace_span rows remain supporting context. Use projected_* for current-window real-time projection, actual_* only to explain cross-window duration, and effective_impact_ms as a bounded ranking/hidden-cost signal rather than elapsed time. When an on_chain runnable, running/compute-supply, semantic span-work, low-frequency, affinity/cpuset, D-state, or IO dependency is tier=primary, report it as a co-primary cause instead of moving it to background, and compare same-chain primary rows by effective_impact_ms before score; for non-semantic rows effective_impact_ms defaults to cumulative_impact_ms. wakeup_chain also reports aggregated_impact rows when repeated fragmented branches share a common dependency path; these rows and the corresponding root_cause_rank candidates carry bounded occurrence_windows, so enumerate the representative repeated windows and compare the aggregate against single long intervals. Treat critical_blocking_calls as direct blocking surfaces: for binder/futex/lock/sync waits, consume oneway/sync_like/blocking_candidate instead of inferring blocking semantics from raw flags, preserve peer, peer_state, chain_relevance, overlap, nearest_chain_thread, and then continue into peer thread state, wakeup_chain, root_cause_rank, and resource rows before naming the cause; if peer/on-chain evidence is missing, keep the wait as a bounded symptom/candidate with caveat. window_stats/event_search can filter or summarize scheduler, sched_stat accounting, binder transaction/received/lock/alloc/reply rows, CPU idle/frequency/frequency-limit, CPU affinity/cpuset/migration constraint evidence, block IO, IRQ/softirq/IPI, storage, filesystem, power, Ability/XPower/HiSystemEvent resource observations, workqueue, DMA fence, memory-like events, SmartPerf-style eBPF BIO/FileSystem/PageFault resource rows, and perf_sample CPU sampling rows when converted to text key/value fields. For perf samples, consume window_stats.perf_samples top_symbols/top_dso/top_callchains/top_threads and perf_quality/quality summaries as supporting code-execution evidence for running threads, runnable competitors, wakeup-chain dependencies, binder peers, or semantic span-work candidates; if a SQL-primary row has comm_source=trace_thread plus perf_thread_comm, thread_comm/pid/tid are the canonical trace-aligned identity and perf_thread_comm is raw converter provenance, not a separate thread. root_cause_rank candidates may carry interval/thread-filtered perf_context plus role-aware perf_contexts rows such as candidate_thread, target_running, on_chain_dependency, same_cpu_competitor, cpu_pressure_top_running, and compute_supply_cpu, and frame_root_cause_bundle may carry target_running_perf, on_chain_perf, binder_peer_perf, and same_cpu_competitor_perf role contexts. perf_quality reports source mix, sample_kind, weight_unit, symbolization_status, cpu_known/cpu_unknown, sample_cpu_scope, clock, clock_confidence, callchain_status, and caveats; sample_cpu_scope=unknown or cpu_unknown means the official/sample source did not expose sample CPU id and must not be attributed to any concrete CPU/core or used as absence proof, sample_kind=off_cpu must not be narrated as running CPU execution, unsymbolized/ip_only means raw fallback or IP/DSO-only evidence, assumed/unknown clock_confidence means trace/perf overlap is supporting evidence unless calibrated, and perf period/sample_weight values are event/sample weights rather than elapsed duration or expected sample density unless explicit sampling configuration plus calibrated CPU frequency are available. For perf evidence-quality questions, answer from sample_cpu_scope/sample_kind/weight_unit first; adjacent sched_switch CPU fields describe scheduler event rows, not the perf sample's CPU location, and should stay out of the perf hotspot conclusion unless the user explicitly asks for scheduler CPU placement. For running/compute-supply/semantic span-work causes, report perf_contexts as the code-execution support for where CPU time was spent, while scheduler overlap, chain relevance, CPU/core/frequency/affinity, D-state/IO, and supply pressure remain the causal basis. Do not treat samples alone as proof of a scheduling root cause. For runnable root causes, window_stats/root_cause_rank report runnable_context, thread_cpu_load, cpu_constraints, and secondary process_cpu_load: consume the concrete thread load, same-CPU competitors, CPU/core class, other-core idle, Harmony/Donghu sched_switch next_info affinity/restricted fields, cpuset/allowed CPU evidence, and only then the process rollup. These are output sections/candidate signals, not separate views; use view=window_stats to inspect them directly, view=root_cause_rank to let them enrich and compete with scheduler candidates, or view=frame_root_cause_bundle for frame/jank windows that need wakeup_chain + rank + blocking + IO/IRQ/IPI/workqueue/sched_stat/supply/trace-mark evidence and role-specific perf contexts in one handoff-safe result. window_stats/root_cause_rank/frame_root_cause_bundle also report inode-level IO outputs: file_io_by_inode for Android FS/F2FS/EXT4-style file read/write/sync/direct-IO rows, page_cache_by_inode for mm_filemap add/delete churn, storage_latency_by_layer for block/MMC/SCSI/F2FS/Android-FS start-done latency pairs, block_io_by_inode to join inode activity with nearest block/storage latency, io_burst_episodes for D-state/iowait/storage bursts, and io_pressure_summary to relate inode IO, page-cache churn, block/storage latency, sched_blocked_reason iowait, and D-state totals. For IO completion questions, preserve file_io completions/ret/example and each storage_latency example together with bytes/len/offset and max_latency, so a single 4KB completion latency is not hidden by aggregate bytes or total latency. These are output sections/candidate signals, not separate views; use view=window_stats to inspect them directly or view=root_cause_rank/frame_root_cause_bundle to let them compete with scheduler and blocking causes. When a wakeup chain exists, treat window_stats IO/D-state/CPU-pressure rows as background context unless the corresponding root_cause_rank candidate says chain_relevance=on_chain/causality=on_wakeup_chain; aggregate rows such as cpu_pressure/io_pressure/supply_pressure remain supporting context and must not be promoted into the direct root-cause chain merely because their representative thread overlaps the chain; generic trace_span rows also stay supporting unless root_cause_rank emits a dedicated semantic span-work type. Off-chain pressure can explain system load but must not become the direct root-cause chain. window_stats/frame_root_cause_bundle also report irq_activity, softirq_activity, ipi_activity, workqueue_activity, sched_stat_accounting, supply_pressure_summary, trace_mark_categories, and async_file_work as supporting signals; use them to explain supply-side pressure and background interference without treating them as proof unless they overlap the target window or wakeup chain. sched_stat_accounting is kernel accounting corroboration and should not replace sched_switch interval timing when both exist; ipi_activity is interrupt/reschedule pressure context, with ipi_raise counted as an instant target_mask signal unless entry/exit pairs provide active_ms. For frame/drop/jank windows with no single long sleep/runnable/D/IO/running segment, window_stats/root_cause_rank also report state_churn: frequent state switching with per-state cumulative impact, fragment count, max/p95 segment, and next-step guidance so the dominant cumulative state can still rank as the primary cause. state_churn is an output section/candidate signal, not an independent view; use view=window_stats to inspect it directly or view=root_cause_rank/frame_root_cause_bundle to let it compete with other causes. For frame/span, runnable-context, inode discovery, or perf hotspot discovery, use view=event_search with pattern as a case-insensitive literal substring, not a regex; it is best for frame ids, jank ids, span labels, trace marker labels, thread labels, next_info tokens, cpuset labels, inode tokens such as 0x478e5, entry_name values, sched_stat thread/kind fields, IPI reason/target_mask fields, perf symbols/DSOs/callchains/source/sample_kind/symbolization_status/callchain_status/clock_confidence/cpu_known, or one exact timestamp/event token before broad grep. Trace markers include B/E/C/S/F rows: event_search rows expose span_action, span_pid, span_name, and span_value; span_window/window_stats trace_spans expose kind=sync|async plus category/subcategory/semantic_class. Synchronous B/E spans end with unnamed E|<pid> or bare E on the same ftrace thread stack, async S/F spans pair by marker pid + name + cookie, and searching E|<pid>|<span_name> is not a valid end-marker test. Treat entry_name as a trace file-name label, not an absolute path; do not prefix it with /, /data/, or any directory unless that full path appears in the trace or an external mapping. If multiple span windows or zero rows come back, narrow with the returned line/time windows, a shorter literal pattern, event_types=[\"trace_mark\"], event_types=[\"perf_sample\"] for CPU sample rows, event_types=[\"cpu_constraint\"] for affinity/cpuset/next_info rows, event_types=[\"sched_stat\"] for scheduler accounting rows, event_types=[\"ipi\"] for IPI rows, event_types=[\"file_io\"] or event_types=[\"page_cache\"] for inode rows, pid/thread, or span_window before running recipe/root-cause views. Once a result reports selected_window, index_windowed, or a concrete line window, keep that same time_start/time_end or line_start/line_end on every follow-up heavy scheduler/resource/root-cause view; thread/pid alone is not enough for large traces. For big/middle/small core analysis, pass core_topology like \"small=0-3,middle=4-7,big=8-11\"; if omitted the tool only infers classes from observed CPU frequencies and reports that caveat. For very large traces, an unbounded jank recipe without time_start/time_end, line_start/line_end, span_name, pid, or thread first does light marker discovery; when timestamped top jank/frame markers are found it automatically runs bounded recipe analysis for the top candidate windows, and otherwise returns marker discovery plus next-call hints instead of expanding expensive full-trace root-cause/resource views. Trace timestamps are seconds end-to-end: 928.081774 means 928 seconds + 0.081774 seconds; with six fractional digits, the fractional part is microsecond-precision (81774 us), not a separate millisecond field. Compound timestamps such as \"1s 501ms 565μs 915ns\" are accepted and normalized to seconds. Only derived durations are rendered in ms. Trace flavor is auto-detected as harmony_hitrace, android_atrace, or generic_ftrace; set trace_flavor/platform in the typed tool call when task context requires a platform override. Raw user wording is not re-parsed by this tool for platform selection. Auto detection may report platform_candidate=mixed_harmony_base when Harmony-base trace signals coexist with Android-framework process surfaces; this uses Donghu/Harmony scheduler priority semantics, not Android priority semantics. Donghu uses Harmony/OpenHarmony trace scheduler semantics with process-isolated Android-framework and Harmony-framework surfaces; priority and timestamp semantics still follow Harmony. For HarmonyOS/hitrace user-space priority, larger numeric priority means higher priority: 1-40=CFS, 41-139=RT. Android/generic ftrace keeps raw scheduler priority and does not apply Harmony ranges. Thread selectors accept pid plus common ftrace/hitrace labels such as com.tencent.mm-36379, com.tencent.mm 36379, com.tencent.mm [36379], [GT]ColdPool#5-36624, binder:486_1-10803, or pid=36379; pass pid directly when known. Use this before ad-hoc grep/awk for ftrace/systrace/hitrace time-window causality questions; a zero-event result in a bounded window is a window/filter diagnostic, not evidence that .ftrace is unsupported. Keep grep/read_file as fallback for truly unsupported formats.", "so one path can carry joint trace+perf evidence. ", "so one path can carry joint trace+perf evidence. A .ftrace/.trace/.systrace path by itself is sufficient for core event queries, including SQL-primary perf_sample rows embedded in systrace; tracebundle is recommended context, not required input. When present, tracebundle result caveats may include tracebundle_trace_provider, tracebundle_trace_db_coverage, tracebundle_trace_coverage, and tracebundle_trace_tool_gate; use them to qualify conversion engine, SQL table coverage, trace_query cross-validation completeness, clock/perf provenance, and commercial guardrail state, not as direct runtime root causes. In tracebundle_trace_db_coverage, role=resolver_index means the DB table was consumed for joins/indexes and rows_emitted=0 is expected; role=systrace_text_output, role=perftrace_text_output, and role=query_ready_export identify text rows produced for trace_query. ", 1)
	description = strings.Replace(description, "state_churn is an output section/candidate signal, not an independent view; use view=window_stats to inspect it directly or view=root_cause_rank/frame_root_cause_bundle to let it compete with other causes.", "state_churn is an output section/candidate signal, not an independent view; use view=window_stats to inspect it directly or view=root_cause_rank/frame_root_cause_bundle to let it compete with other causes. The state_drilldown rows are the state-first handoff: top_sleep is a ranked Top-N cumulative sleep surface, long top_sleep rows require wakeup_chain/root_cause_rank recursive drilldown, fragmented sleep churn stays visible but non-recursive with thread_timeline/interaction_stats/window_stats follow-up, and fragmented runnable or D/IO waits remain recursive root-cause candidates. Preserve state_drilldown source, recommended_views, chain_required, and recursive flags instead of guessing from prose. Each state_drilldown row also carries window_proportion (fraction 0..1 of the selected window that state consumed) and a significant flag: the top-ranked state is always significant, and lower-ranked states are significant only when they clear the proportion floor; rows with significant=false are kept for coverage completeness but are too small to be worth their own per-layer root-cause drilldown, so prioritize significant=true states for per-layer root-cause analysis.", 1)
	description = strings.Replace(description, "Once a result reports selected_window, index_windowed, or a concrete line window, keep that same time_start/time_end or line_start/line_end on every follow-up heavy scheduler/resource/root-cause view; thread/pid alone is not enough for large traces.", "Once a result reports selected_window, index_windowed, or a concrete line window, keep that same time_start/time_end or line_start/line_end on every follow-up heavy scheduler/resource/root-cause view; thread/pid alone is not enough for large traces. If a call supplies both a frame/span selector and explicit time_start/time_end, frame_root_cause_bundle preserves the explicit query window and unions it with the frame-derived previous-frame-end..current-frame-end window instead of shrinking to an interior vsync/frame marker; span_window/span_name does the same for a uniquely-matched named span, unioning the explicit window with the matched span's own start/end instead of narrowing to whichever is smaller. For jank/stall root-cause analysis over a broader typed period, prefer frame/span-derived windows or coverage windows around 80-150ms for recipe/root_cause_rank/frame_root_cause_bundle before shrinking further; sub-50ms windows are micro-probes and must not be treated as representative unless the selected frame/span itself is that short. If the task's typed target is a process id, thread id, or thread label, set pid/thread explicitly in the tool call and keep that typed filter on follow-up trace_query calls unless deliberately inspecting a named peer; if omitted and the structured request model exposes exactly one runtime_targets entry, trace_query inherits only that typed pid/thread and reports trace_query_target_inherited, but trace_query does not infer omitted pid/thread values from raw request prose, analyzer entity strings, objective text, or prior summaries. For long transaction/lifecycle windows, preserve the full typed time window as parent coverage; use event_search/span_window/frame_window to discover phase boundaries, then drill into the heaviest phase windows. If a result reports mode=index_event_limit or selected window too dense, do not retry the same parameters; for local jank/stall root-cause views split toward 80-150ms coverage windows first, add line_start/line_end, or use event_search/span_window/event_types to narrow before rerunning the heavy view; shrink below 50ms only as a local micro-probe with a caveat.", 1)
	return description
}

func (t *TraceQuery) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
	    "source": {"type":"string","enum":["path","attached_trace"],"x-codrax-enum-style-alias":true,"description":"Use attached_trace for the current --htrace/--atrace blob; use path for an explicit workspace/repo file."},
	    "path": {"type":"string","description":"Repo/workspace-relative or absolute trace/log path when source=path. Accepts ftrace-compatible text such as .ftrace/.trace/.systrace/.htrace/.atrace, text .perftrace, and .tracebundle.json. A converted .systrace or raw .ftrace text is sufficient for core event queries and may already contain SQL-primary perf_sample rows; .tracebundle.json adds provider/coverage/clock/caveat provenance. When a sibling .tracebundle.json exists, or a sibling .systrace/.perftrace pair exists, trace_query automatically builds a joint trace+perf index."},
	    "trace_flavor": {"type":"string","enum":["auto","harmony_hitrace","android_atrace","generic_ftrace"],"x-codrax-enum-style-alias":true,"description":"Optional producer/platform flavor. Defaults to auto detection. Use harmony_hitrace for HarmonyOS HiTrace priority semantics, android_atrace for Android/Linux atrace raw scheduler priorities, and generic_ftrace when uncertain."},
	    "platform": {"type":"string","enum":["auto","donghu","harmony","harmony_hitrace","android","android_atrace","generic","generic_ftrace"],"x-codrax-enum-style-alias":true,"description":"Optional typed platform hint. Use donghu when the typed task/tool call selects Donghu: scheduler/time/priority semantics follow Harmony/OpenHarmony, while Android-framework and Harmony-framework processes may coexist at process boundaries. harmony/harmony_hitrace selects Harmony semantics; android/android_atrace selects Android raw scheduler priority semantics."},
		    "view": {"type":"string","enum":["event_search","span_window","frame_window","render_pipeline","frame_timeline","frame_flow","thread_timeline","window_stats","perf_stats","perf_timeline","trace_perf_bundle","scheduler_latency_stats","ipc_graph","wakeup_chain","root_cause_rank","frame_root_cause_bundle","critical_blocking_calls","interaction_stats","recipe","evidence_pack"],"x-codrax-enum-style-alias":true,"x-codrax-enum-aliases":{"state_churn":"window_stats","cpu_samples":"perf_stats","cpu_sample_stats":"perf_stats","sample_timeline":"perf_timeline","perf_sample_timeline":"perf_timeline","perf_bundle":"trace_perf_bundle","trace_perf":"trace_perf_bundle","trace_plus_perf":"trace_perf_bundle","causal_impact":"wakeup_chain","frame_bundle":"frame_root_cause_bundle","frame_rootcause_bundle":"frame_root_cause_bundle","frame_root_cause":"frame_root_cause_bundle"},"description":"The deterministic trace view to compute. Use span_window to turn a unique trace span into a time window: synchronous B/E spans close with unnamed E|<pid> or bare E on the same ftrace thread stack, and async S/F spans close by marker pid + name + cookie. Do not search for E|<pid>|<span_name> as an end marker. Use frame_window/render_pipeline for Choreographer/RenderFrame/VSYNC/draw/present spans; frame_timeline/frame_flow for Expected/Actual/Jank/GPU/RS/UI phase summaries and cross-thread frame flows; perf_stats for same-window CPU sample top_symbols/top_dso/top_callchains/top_threads, perf_timeline for bucketed sample weight over time, and trace_perf_bundle for a handoff-safe bundle that combines window/root-cause/wakeup evidence with perf sample context; scheduler_latency_stats for runnable wait p95/p99/max and CPU competition; wakeup_chain for wakeup edges and causal_impacts per chain node plus aggregated_impacts with bounded occurrence_windows when repeated fragmented branches share a common dependency path; critical_blocking_calls for futex/lock/sync/binder/IO/D-state candidates, with peer_state breakdown when the peer thread timeline is visible; root_cause_rank for primary/secondary/tertiary cause candidates, including projected_impact_ms/projected_total_ms for selected-window projection, actual_impact_ms/actual_total_ms/actual_window for full scheduler-state duration, cumulative_impact_ms, effective_impact_ms, dominant_state/running/runnable/sleep/d_state/io_wait totals, occurrence_windows for aggregate common dependency paths, candidate-level perf_context plus role-aware perf_contexts such as candidate_thread, target_running, on_chain_dependency, same_cpu_competitor, cpu_pressure_top_running, and compute_supply_cpu, fragmented state_churn candidates when frequent short state switches cumulatively dominate, wakeup_chain causal_impacts and aggregated_impacts when repeated fragmented branches share a common dependency path, semantic span-work candidates for on-chain JIT/class verification/shader/runtime compilation hidden cost, and co-primary on-chain runnable/running/compute-supply/D-state/IO dependencies when they are part of the same causal chain; same-chain primary root_cause_rank rows are ordered by effective_impact_ms before score, and non-semantic rows default effective_impact_ms to cumulative_impact_ms; frame_root_cause_bundle returns wakeup_chain + frame_timeline + root_cause_rank + critical_blocking_calls plus IO/IRQ/workqueue/supply/trace-mark bundle fields and role-specific perf contexts target_running_perf/on_chain_perf/binder_peer_perf/same_cpu_competitor_perf for frame/jank handoff; state_churn and causal_impacts are output sections, not standalone views; view=state_churn is accepted and treated as view=window_stats, view=causal_impact is accepted as wakeup_chain, view=perf_bundle/trace_perf/trace_plus_perf is accepted as trace_perf_bundle, and view=frame_bundle/frame_rootcause_bundle is accepted as frame_root_cause_bundle; interaction_stats for target-thread wakeup/binder interaction Top-N; recipe for standard evidence packs; and ipc_graph for binder transaction send/receive causality with explicit oneway/sync_like/blocking_candidate fields."},
	    "thread": {"type":"string","description":"Thread name, substring, or ftrace/hitrace task label to resolve when pid is unknown. Accepts forms like \"com.tencent.mm-36379\", \"com.tencent.mm 36379\", \"com.tencent.mm [36379]\", \"[GT]ColdPool#5-36624\", \"binder:486_1-10803\", or \"pid=36379\"; pid is preferred when known."},
    "pid": {"type":"integer","description":"Thread pid to analyze when known."},
    "time_start": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window start in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\", \"928.081774 秒\", or compound forms like \"1s 501ms 565μs 915ns\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "time_end": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window end in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\", \"928.081774 秒\", or compound forms like \"3s 116ms\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "line_start": {"type":"integer","description":"Optional artifact line window start for bounded search."},
    "line_end": {"type":"integer","description":"Optional artifact line window end for bounded search."},
	    "event_types": {"type":"array","items":{"type":"string"},"x-codrax-split-string-array":true,"description":"Optional event filters such as trace_mark, sched_switch, sched_wakeup, sched_blocked_reason, sched_stat, cpu_idle, cpu_frequency, cpu_frequency_limits, cpu_constraint, clock_set_rate, block_rq_issue, block_rq_complete, block_bio_remap, binder_transaction, binder_transaction_received, binder_transaction_alloc_buf, binder_lock, binder_locked, binder_unlock, binder_reply, irq, softirq, ipi, storage, filesystem, file_io, page_cache, android_fs, f2fs, scsi, mmc, storage_latency, io_pressure, perf_sample, power, ability_monitor, xpower, hi_sysevent, workqueue, dma_fence. Official formatter aliases such as sched_wakeup_new, sched_stat_wait, sched_stat_sleep, sched_stat_iowait, sched_stat_blocked, sched_stat_runtime, ipi_raise, ipi_entry, ipi_exit, block_rq_insert, block_getrq, block_bio_queue, block_bio_complete, print, tracing_mark_write_xacct, and xacct_tracing_mark_write are accepted and mapped to the matching structured event type. Use trace_mark for B/E/C/S/F marker rows; B/E end rows are unnamed E|<pid> or E, so use span_window rather than E|<pid>|<span_name> searches to prove completion. Use sched_stat/sched_stat_accounting as kernel accounting corroboration for wait/iowait/blocked/runtime, not as a replacement for sched_switch interval timing when both exist. Use ipi/ipi_activity as interrupt/scheduler-reschedule pressure context; ipi_raise target_mask is an instant signal unless paired ipi_entry/exit gives active_ms. Use perf_sample with pattern=<symbol, dso, callchain, event, thread, source, symbolization_status, callchain_status, clock_confidence, or cpu_known> for CPU sampling rows; window_stats.perf_samples summarizes top_symbols/top_dso/top_callchains/top_threads plus perf_quality as supporting execution context, not standalone root-cause proof. Raw fallback rows may have source=raw_perfdata_fallback, symbolization_status=unsymbolized, and callchain_status=ip_only; OpenHarmony hiperf proto rows may have cpu_known=false because sample CPU is unavailable. Result caveats may also carry tracebundle perf/profiler/trace conversion quality provenance such as lost_records/lost_events, lost_sample_records/lost_samples, throttle_records/unthrottle_records, aux_records/aux_bytes, ftrace-plugin structured metadata, profiler plugin metadata, dropped_events, overrun, commit_overrun, overwrite, trace_clock, clock_details, symbol_examples, tracebundle_perf_capability, tracebundle_perf_clock_alignment, tracebundle_trace_provider, tracebundle_trace_db_coverage, tracebundle_trace_coverage, and tracebundle_trace_tool_gate; use them to qualify sample/capture/conversion reliability, coverage, and converter guardrail state, not as direct runtime root causes. Use cpu_constraint/affinity/cpuset to inspect sched_setaffinity, sched_migrate_task, cpuset/cgroup attach, and Harmony/Donghu sched_switch next_info affinity/restricted evidence. Use file_io/page_cache with pattern=<inode or entry_name> for inode-level IO rows. This field also accepts a comma/semicolon separated string, and friendly aliases such as inode_io, pageCache, mm_filemap, cpuSample, perfSamples, topSymbols, callchain, cpuAffinity, schedMigrate, storageLayerLatency, irq_activity, softirq_activity, ipi_activity, sched_stat_accounting, and block_io_by_inode are accepted and mapped to the matching event types."},
    "pattern": {"type":"string","description":"For event_search, optional case-insensitive literal substring matched against parsed event text, span names, thread labels, scheduler roles, resource fields, and raw-like field text. Use this for frame ids such as \"1917295\", jank ids such as \"jank_frames=7\", exact timestamps, or trace labels such as \"Choreographer#doFrame\"; it is not a regex. Start with one exact token, then add event_types/time/line/thread filters after the first hit."},
    "span_name": {"type":"string","description":"Optional trace span name substring. For span_window, returns matching sync B/E or async S/F span windows; sync B/E end rows do not repeat the span name and appear as E|<pid> or bare E on the same ftrace thread stack. For wakeup_chain/root_cause_rank/evidence_pack without explicit time_start/time_end, a unique matching span derives the selected window."},
    "interaction_direction": {"type":"string","enum":["both","incoming","outgoing"],"x-codrax-enum-style-alias":true,"description":"For interaction_stats: both is default; incoming counts peers waking/calling the target, outgoing counts target waking/calling peers."},
    "recipe_name": {"type":"string","enum":["auto","sleep_root_cause","jank","runnable_delay","binder_wait","io_wait","cpu_supply"],"x-codrax-enum-style-alias":true,"description":"For view=recipe: choose a standard deterministic evidence pack. auto picks from span_name/event_types/question-shape hints; recipes remain advisory and line-backed."},
    "max_depth": {"type":"integer","description":"wakeup_chain recursion limit; default 10."},
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
	traceQueryRecordExplicitRuntimeTarget(ctx, p)
	var targetCaveat string
	p, targetCaveat = traceQueryApplyRequestModelTarget(ctx, p)
	path, sourceLabel, reject := resolveTraceQuerySource(ctx, p)
	if reject != nil {
		return *reject, nil
	}
	timeStart, timeEnd, timeCaveat := normalizedTraceQueryWindow(p)
	timeCaveat = traceQueryJoinCallCaveats(timeCaveat, targetCaveat)
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
		if limit, ok := t.traceQueryIndexLimitResult(ctx, p, path, sourceLabel, err); ok {
			return limit, nil
		}
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
	traceQueryAppendCallCaveats(&result, timeCaveat)
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
		Refinement:   traceQueryRefinement(result, q, p, sourceLabel),
		Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now),
		Timestamp:    now,
	}, nil
}

func traceQueryAppendCallCaveats(result *tracequery.Result, timeCaveat string) {
	if result == nil {
		return
	}
	if timeCaveat != "" {
		result.Caveats = append(result.Caveats, timeCaveat)
	}
}

func (t *TraceQuery) traceQueryIndexLimitResult(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string, err error) (types.ToolResult, bool) {
	var limitErr *tracequery.IndexEventLimitError
	if !errors.As(err, &limitErr) {
		return types.ToolResult{}, false
	}
	summary := traceQueryIndexLimitSummary(path, sourceLabel, p, limitErr)
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, p.TimeStart.Seconds(), p.TimeEnd.Seconds())
	if cluster, clusterErr := tracequery.StreamStateCluster(contextFromBus(ctx), path, q, 8); clusterErr == nil && cluster.WindowStats != nil {
		cluster.Caveats = append([]string{
			fmt.Sprintf("index_event_limit_fallback=true; original_view=%s parsed_events=%d max_events=%d", sanitizeForBanner(firstNonEmptyTraceString(p.View, "window_stats")), limitErr.Events, limitErr.MaxEvents),
		}, cluster.Caveats...)
		payload, _ := json.MarshalIndent(cluster, "", "  ")
		payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-state-cluster.json", string(payload))
		summary += "\n" + traceQuerySummary(cluster, p, sourceLabel, payloadRef)
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
			Refinement:   traceQueryIndexLimitRefinement(ctx, p, sourceLabel, path),
			Observations: traceQueryTypedObservations(cluster, sourceLabel, payloadRef, rawRef, "stream_state_cluster", now),
			Timestamp:    now,
		}, true
	} else if clusterErr != nil {
		summary += fmt.Sprintf("stream_state_cluster_unavailable=%s\n", sanitizeForBanner(clusterErr.Error()))
	}
	preview, rawRef := StoreBlob(ctx, t.Name(), summary)
	now := time.Now()
	return types.ToolResult{
		ToolName:   t.Name(),
		Success:    true,
		Summary:    preview,
		RawRef:     rawRef,
		Refinement: traceQueryIndexLimitRefinement(ctx, p, sourceLabel, path),
		Timestamp:  now,
	}, true
}

func traceQueryIndexLimitSummary(path, sourceLabel string, p traceQueryParams, limitErr *tracequery.IndexEventLimitError) string {
	view := firstNonEmptyTraceString(p.View, "window_stats")
	var b strings.Builder
	fmt.Fprintf(&b, "[trace_query params: view=%s source=%s path=%s origin=runtime_artifact artifact_id=%s artifact_kind=trace mode=index_event_limit thread=%s pid=%s pattern=%s span_name=%s platform=%s trace_flavor=%s]\n",
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
	b.WriteString("# Trace Query: selected window too dense\n\n")
	fmt.Fprintf(&b, "guard=index_event_limit parsed_events=%d max_events=%d line=%d scanned_lines=%d windowed=%t\n",
		limitErr.Events, limitErr.MaxEvents, limitErr.Line, limitErr.ScannedLines, limitErr.Windowed)
	fmt.Fprintf(&b, "index_window=time %.6f..%.6f seconds lines %d..%d parsed_time=%.6f..%.6f\n",
		limitErr.IndexTimeStart, limitErr.IndexTimeEnd, limitErr.IndexLineStart, limitErr.IndexLineEnd, limitErr.FirstTs, limitErr.LastTs)
	b.WriteString("meaning=trace_query stopped before growing the in-memory Event index further; this is an OOM guard, not evidence that the trace/ftrace format is unsupported.\n")
	b.WriteString("state_first_hint=before shrinking into arbitrary micro-windows, use the stream_state_cluster/window_stats rows below to identify the target thread's dominant and secondary states, then drill down by state family: sleep->wakeup_chain, runnable->scheduler_latency/root_cause_rank with same CPU competitors, running->perf/compute-supply/semantic span work, D-state/IO->critical_blocking/window_stats IO resources.\n")
	fmt.Fprintf(&b, "next_call_hint=do not retry the same heavy view with the same dense scope. Split toward %.0f-%.0fms coverage windows for jank/stall root-cause views, add line_start/line_end from a prior event_search/span_window result, or first run event_search with exact timestamp/span/event_types filters to locate a tighter line window. Shrink below %.0fms only as a local micro-probe and do not extrapolate it to the broader requested period.\n",
		traceQueryPreferredCoverageWindowMinSeconds*1000,
		traceQueryPreferredCoverageWindowMaxSeconds*1000,
		traceQueryMicroWindowProbeSeconds*1000)
	if hint := traceQueryParentWindowStrategyHint(p); hint != "" {
		b.WriteString(hint)
	}
	if p.PID.Int() > 0 {
		fmt.Fprintf(&b, "target_hint=keep pid=%d and rerun view=%q with a narrower time_start/time_end or line_start/line_end.\n", p.PID.Int(), sanitizeForBanner(view))
	} else if strings.TrimSpace(p.Thread) != "" {
		fmt.Fprintf(&b, "target_hint=keep thread=%q and rerun view=%q with a narrower time_start/time_end or line_start/line_end.\n", sanitizeForBanner(p.Thread), sanitizeForBanner(view))
	}
	if len(p.EventTypes.Strings()) > 0 {
		fmt.Fprintf(&b, "event_type_hint=current event_types=%s; if still dense, split by one event type family per call.\n", sanitizeForBanner(strings.Join(p.EventTypes.Strings(), ",")))
	}
	return b.String()
}

func traceQueryParentWindowStrategyHint(p traceQueryParams) string {
	duration := traceQueryParamWindowDurationSeconds(p)
	if duration < traceQueryParentWindowStrategySeconds {
		return ""
	}
	return fmt.Sprintf("parent_window_strategy=selected window is %.3fms. For long transaction/lifecycle windows, preserve this full window as parent coverage, discover phase/span/marker boundaries with event_search/span_window/frame_window/interaction_stats, then drill into the heaviest phase windows; do not treat a few arbitrary micro-windows as exhaustive coverage of the parent window.\n", duration*1000)
}

func traceQueryParamWindowDurationSeconds(p traceQueryParams) float64 {
	if !p.TimeStart.Set() || !p.TimeEnd.Set() {
		return 0
	}
	start, end := p.TimeStart.Seconds(), p.TimeEnd.Seconds()
	if end <= start {
		return 0
	}
	return end - start
}

func traceQueryIndexLimitRefinement(ctx *types.BusContext, p traceQueryParams, sourceLabel, path string) *types.ToolRefinementHint {
	next := p
	required := []string{"time_start", "time_end", "state_cluster_first"}
	if next.LineStart.Int() <= 0 && next.LineEnd.Int() <= 0 {
		next.View = "event_search"
		if len(next.EventTypes.Strings()) == 0 {
			next.EventTypes = TraceEventTypes{"trace_mark"}
		}
		if next.Limit.Int() <= 0 {
			next.Limit = FlexInt(40)
		}
		required = append(required, "line_start", "line_end")
	}
	hint := traceQueryParamsRefinement(ctx, "trace_query_index_event_limit", next, sourceLabel, path, true, required)
	if hint != nil {
		if hint.PreferredParams == nil {
			hint.PreferredParams = map[string]string{}
		}
		hint.PreferredParams["parent_coverage"] = "stream_state_cluster"
		hint.PreferredParams["micro_window_policy"] = "sub_50ms_local_only"
		normalized := types.NormalizeToolRefinementHint(*hint)
		hint = &normalized
	}
	return hint
}

func traceQueryBuildQuery(ctx *types.BusContext, p traceQueryParams, sourceLabel, path string, timeStart, timeEnd float64) tracequery.Query {
	p, _ = traceQueryApplyRequestModelTarget(ctx, p)
	q := tracequery.Query{
		View:                 p.View,
		Thread:               p.Thread,
		ThreadInput:          p.Thread,
		PID:                  p.PID.Int(),
		TimeStart:            timeStart,
		TimeEnd:              timeEnd,
		TimeStartSet:         p.TimeStart.Set(),
		TimeEndSet:           p.TimeEnd.Set(),
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
		traceQueryAppendCallCaveats(&searchResult, timeCaveat)
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
			Refinement:   traceQueryRefinement(searchResult, searchQ, searchP, sourceLabel),
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
		if limit, ok := t.traceQueryIndexLimitResult(ctx, boundedP, path, sourceLabel, err); ok {
			return limit, true
		}
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
	q.FrameWindowAutoDerived = true
	runStart := time.Now()
	result := tracequery.Run(idx, q)
	heapAlloc, heapSys, gcCount = traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=auto_window_run view=%s path=%s done elapsed=%s events=%d evidence=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
		q.View, path, time.Since(runStart), len(idx.Events), len(result.EvidencePack), len(result.Caveats), heapAlloc, heapSys, gcCount)
	result.Caveats = append(result.Caveats,
		fmt.Sprintf("auto_window_from_pattern=true; pattern %q matched %d event(s), then ran %s in %.6f..%.6f seconds without building a full trace index",
			pattern, len(searchResult.Events), firstNonEmptyTraceString(p.View, "frame_window"), start, end))
	traceQueryAppendCallCaveats(&result, timeCaveat)
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
		Refinement:   traceQueryRefinement(result, q, boundedP, sourceLabel),
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
		"ipc_graph", "wakeup_chain", "frame_root_cause_bundle", "interaction_stats", "perf_stats", "perf_timeline", "trace_perf_bundle", "evidence_pack", "recipe":
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
		"thread_timeline", "ipc_graph", "wakeup_chain", "frame_root_cause_bundle", "interaction_stats", "perf_stats", "perf_timeline", "trace_perf_bundle", "recipe":
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
		q.FrameWindowAutoDerived = true
		runStart := time.Now()
		result := tracequery.Run(idx, q)
		heapAlloc, heapSys, gcCount = traceQueryMemoryForLog()
		logging.Debug("[trace_query] phase=auto_window_candidate_run mode=%s view=%s path=%s rank=%d done elapsed=%s events=%d evidence=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
			mode, q.View, path, candidate.Rank, time.Since(runStart), len(idx.Events), len(result.EvidencePack), len(result.Caveats), heapAlloc, heapSys, gcCount)
		result.Caveats = append(result.Caveats,
			fmt.Sprintf("auto_window_candidate=true; mode=%s rank=%d source=%s token=%q line=%d ts=%.6f window=%.6f..%.6f seconds",
				mode, candidate.Rank, candidate.Source, candidate.Token, candidate.Line, candidate.Ts, candidate.Start, candidate.End))
		traceQueryAppendCallCaveats(&result, timeCaveat)
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
		Refinement:   traceQueryAutoWindowCandidatesRefinement(ctx, p, sourceLabel, path, children),
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
	traceQueryAppendCallCaveats(&result, timeCaveat)
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
		Refinement:   traceQueryRefinement(result, q, p, sourceLabel),
		Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now),
		Timestamp:    now,
	}, true
}

func traceQueryRefinement(result tracequery.Result, q tracequery.Query, p traceQueryParams, sourceLabel string) *types.ToolRefinementHint {
	reasonCode := ""
	resultTruncated := false
	switch {
	case traceQueryEventSearchLimitReached(result, q):
		reasonCode = "trace_query_event_search_limit_reached"
		resultTruncated = true
	case traceQueryResultCompacted(result):
		reasonCode = "trace_query_result_compacted"
		resultTruncated = true
	case traceQueryEventSearchZeroMatch(result, q):
		reasonCode = "trace_query_event_search_zero_match"
	}
	if reasonCode == "" {
		return nil
	}
	hint := types.NormalizeToolRefinementHint(types.ToolRefinementHint{
		ReasonCode:        reasonCode,
		ResultTruncated:   resultTruncated,
		PreferredNextTool: tTraceQueryName,
		PreferredParams:   traceQueryRefinementPreferredParams(result, q, p, sourceLabel),
		RequiredFields:    traceQueryRefinementRequiredFields(result, q),
	})
	if hint.Empty() {
		return nil
	}
	return &hint
}

const tTraceQueryName = "trace_query"

func traceQueryHeavyViewGuardRefinement(ctx *types.BusContext, p traceQueryParams, sourceLabel, path string) *types.ToolRefinementHint {
	next := p
	next.View = "event_search"
	next.EventTypes = TraceEventTypes{"trace_mark"}
	if next.Limit.Int() <= 0 {
		next.Limit = FlexInt(40)
	}
	return traceQueryParamsRefinement(ctx, "trace_query_heavy_view_requires_scope", next, sourceLabel, path, true, []string{"pattern"})
}

func traceQueryRecipeDiscoveryRefinement(ctx *types.BusContext, p traceQueryParams, sourceLabel, path string, markers []traceQueryRecipeDiscoveryMarker, truncated bool) *types.ToolRefinementHint {
	next := p
	if len(markers) > 0 {
		first := firstPrimaryTraceQueryMarker(markers)
		start := first.Line - 200
		if start < 1 {
			start = 1
		}
		next.View = "recipe"
		next.RecipeName = firstNonEmptyTraceString(p.RecipeName, "jank")
		next.LineStart = FlexInt(start)
		next.LineEnd = FlexInt(first.Line + 200)
		return traceQueryParamsRefinement(ctx, "trace_query_recipe_discovery_marker_window", next, sourceLabel, path, truncated, nil)
	}
	next.View = "event_search"
	next.EventTypes = TraceEventTypes{"trace_mark"}
	if next.Limit.Int() <= 0 {
		next.Limit = FlexInt(40)
	}
	return traceQueryParamsRefinement(ctx, "trace_query_recipe_discovery_needs_scope", next, sourceLabel, path, truncated, []string{"pattern"})
}

func traceQueryAutoWindowCandidatesRefinement(ctx *types.BusContext, p traceQueryParams, sourceLabel, path string, children []traceQueryAutoWindowChild) *types.ToolRefinementHint {
	for _, child := range children {
		if child.Error != "" {
			continue
		}
		next := p
		next.TimeStart = traceSecondFromAutoWindow(child.Candidate.Start)
		next.TimeEnd = traceSecondFromAutoWindow(child.Candidate.End)
		return traceQueryParamsRefinement(ctx, "trace_query_auto_window_candidate", next, sourceLabel, path, len(children) > 1, nil)
	}
	return nil
}

func traceQueryParamsRefinement(ctx *types.BusContext, reasonCode string, p traceQueryParams, sourceLabel, path string, resultTruncated bool, requiredFields []string) *types.ToolRefinementHint {
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		return nil
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, p.TimeStart.Seconds(), p.TimeEnd.Seconds())
	hint := types.NormalizeToolRefinementHint(types.ToolRefinementHint{
		ReasonCode:        reasonCode,
		ResultTruncated:   resultTruncated,
		PreferredNextTool: tTraceQueryName,
		PreferredParams: traceQueryRefinementPreferredParams(tracequery.Result{
			View:       firstNonEmptyTraceString(p.View, q.View),
			SourcePath: path,
		}, q, p, sourceLabel),
		RequiredFields: requiredFields,
	})
	if hint.Empty() {
		return nil
	}
	return &hint
}

func traceQueryEventSearchLimitReached(result tracequery.Result, q tracequery.Query) bool {
	if traceQueryCanonicalView(result, q) != "event_search" {
		return false
	}
	if q.Limit > 0 && len(result.Events) >= q.Limit {
		return true
	}
	return traceQueryCaveatsContain(result, "event_search_limit_reached=true")
}

func traceQueryEventSearchZeroMatch(result tracequery.Result, q tracequery.Query) bool {
	if traceQueryCanonicalView(result, q) != "event_search" || len(result.Events) != 0 {
		return false
	}
	return traceQueryCaveatsContain(result, "matched_events=0")
}

func traceQueryResultCompacted(result tracequery.Result) bool {
	for _, caveat := range result.Caveats {
		lower := strings.ToLower(strings.TrimSpace(caveat))
		if strings.Contains(lower, " compacted from ") || strings.Contains(lower, " compacted after ") {
			return true
		}
	}
	return false
}

func traceQueryCaveatsContain(result tracequery.Result, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	for _, caveat := range result.Caveats {
		if strings.Contains(strings.ToLower(caveat), needle) {
			return true
		}
	}
	return false
}

func traceQueryCanonicalView(result tracequery.Result, q tracequery.Query) string {
	if view := strings.TrimSpace(result.View); view != "" {
		return view
	}
	return strings.TrimSpace(q.View)
}

func traceQueryRefinementPreferredParams(result tracequery.Result, q tracequery.Query, p traceQueryParams, sourceLabel string) map[string]string {
	params := map[string]string{}
	if source := firstNonEmptyTraceString(p.Source, sourceLabel); source != "" {
		params["source"] = source
	}
	if path := strings.TrimSpace(p.Path); path != "" && path != "." {
		params["path"] = path
	} else if source := firstNonEmptyTraceString(p.Source, sourceLabel); source == "path" {
		if path := strings.TrimSpace(result.SourcePath); path != "" {
			params["path"] = path
		}
	}
	if view := traceQueryCanonicalView(result, q); view != "" {
		params["view"] = view
	}
	if q.PID > 0 {
		params["pid"] = strconv.Itoa(q.PID)
	}
	if thread := firstNonEmptyTraceString(p.Thread, q.ThreadInput, q.Thread); thread != "" {
		params["thread"] = thread
	}
	if p.TimeStart.Set() {
		params["time_start"] = traceQuerySecondParamString(p.TimeStart)
	} else if q.TimeStart > 0 || q.TimeEnd > 0 {
		params["time_start"] = traceQueryFloatParamString(q.TimeStart)
	}
	if p.TimeEnd.Set() {
		params["time_end"] = traceQuerySecondParamString(p.TimeEnd)
	} else if q.TimeStart > 0 || q.TimeEnd > 0 {
		params["time_end"] = traceQueryFloatParamString(q.TimeEnd)
	}
	if q.LineStart > 0 {
		params["line_start"] = strconv.Itoa(q.LineStart)
	}
	if q.LineEnd > 0 {
		params["line_end"] = strconv.Itoa(q.LineEnd)
	}
	if eventTypes := traceQueryEventTypesParamString(q.EventTypes); eventTypes != "" {
		params["event_types"] = eventTypes
	}
	if pattern := strings.TrimSpace(q.Pattern); pattern != "" {
		params["pattern"] = pattern
	}
	if span := strings.TrimSpace(q.SpanName); span != "" {
		params["span_name"] = span
	}
	if q.Limit > 0 {
		params["limit"] = strconv.Itoa(q.Limit)
	}
	if flavor := strings.TrimSpace(p.TraceFlavor); flavor != "" {
		params["trace_flavor"] = flavor
	}
	if platform := strings.TrimSpace(p.Platform); platform != "" {
		params["platform"] = platform
	}
	return params
}

func traceQueryRefinementRequiredFields(result tracequery.Result, q tracequery.Query) []string {
	var fields []string
	view := traceQueryCanonicalView(result, q)
	if view == "event_search" {
		if strings.TrimSpace(q.Pattern) == "" {
			fields = append(fields, "pattern")
		}
		if len(q.EventTypes) == 0 {
			fields = append(fields, "event_types")
		}
	}
	if (traceQueryEventSearchLimitReached(result, q) || traceQueryResultCompacted(result)) &&
		q.TimeStart == 0 && q.TimeEnd == 0 && q.LineStart == 0 && q.LineEnd == 0 {
		fields = append(fields, "time_start", "time_end")
	}
	return fields
}

func traceQuerySecondParamString(ts TraceSecond) string {
	if raw := strings.TrimSpace(ts.Raw()); raw != "" {
		return raw
	}
	return traceQueryFloatParamString(ts.Seconds())
}

func traceQueryFloatParamString(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func traceQueryEventTypesParamString(eventTypes []tracequery.EventType) string {
	if len(eventTypes) == 0 {
		return ""
	}
	out := make([]string, 0, len(eventTypes))
	for _, eventType := range eventTypes {
		if s := strings.TrimSpace(string(eventType)); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, ",")
}

func traceQueryShouldStreamEventSearch(p traceQueryParams) bool {
	view := strings.TrimSpace(p.View)
	if view != "" && view != "event_search" {
		return false
	}
	return true
}

func traceQueryBuildIndex(ctx context.Context, path string, p traceQueryParams, timeStart, timeEnd float64) (*tracequery.Index, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() < traceQueryWindowedIndexMinBytes || !traceQueryHasExplicitIndexWindow(p) {
		return tracequery.BuildIndex(ctx, path)
	}
	return tracequery.BuildIndexWithOptions(ctx, path, traceQueryWindowedIndexOptions(p, timeStart, timeEnd))
}

// traceQueryWindowedIndexOptions builds the windowed BuildOptions for a
// large-trace heavy-view query, including the pid/thread-scoped MaxEvents raise.
// Pure and side-effect free so the scope/cap decision is unit-testable without
// materializing a multi-hundred-thousand-event fixture.
func traceQueryWindowedIndexOptions(p traceQueryParams, timeStart, timeEnd float64) tracequery.BuildOptions {
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
	// A single, deliberate pid/thread-scoped heavy view (the customer's pinned
	// pid+window root_cause_rank / wakeup_chain case) gets a larger, still
	// unpruned index from an explicit byte budget so a dense GB-trace window can
	// actually run instead of hitting the shared 250K event cap. The index stays
	// a full window (no relation pruning), so every view remains correct and the
	// higher cap cannot poison other queries — the cacheKey already encodes
	// MaxEvents. Unscoped / non-heavy calls are untouched (MaxEvents stays 0 →
	// the default cap), preserving byte-identical behavior for existing paths.
	if pid := p.PID.Int(); (pid > 0 || strings.TrimSpace(p.Thread) != "") && traceQueryIsHeavyView(p.View) {
		if scoped := traceQueryScopedIndexMaxEvents(); scoped > opts.MaxEvents {
			opts.MaxEvents = scoped
		}
		opts.ScopePID = pid
		opts.ScopeThread = strings.TrimSpace(p.Thread)
		// Gap 3 Step 2: for the two causal-chain views whose consumption is
		// provably a subset of {target pid + its scheduler wakers + binder},
		// additionally pid-relation-prune the index so even a very dense GB-trace
		// window fits. This is DELIBERATELY restricted to thread_timeline /
		// wakeup_chain — root_cause_rank / frame_root_cause_bundle / window_stats
		// consume whole-window × all-thread aggregates that pruning would
		// silently drop, so they keep the full (Step-1 byte-budgeted) index.
		// Requires an explicit pid (thread-only scoping is not relation-pruned).
		if pid > 0 && traceQueryRelationScopedView(p.View) {
			opts.RelationScoped = true
			opts.ScopeMaxDepth = traceQueryRelationScopeMaxDepth(p)
		}
	}
	return opts
}

// traceQueryRelationScopedView reports whether a view's event consumption is a
// provable subset of the target pid's threads, their transitive scheduler
// wakers, and binder rows — the only views for which relation-scope index
// pruning is complete (verified design w9ffnwv29).
func traceQueryRelationScopedView(view string) bool {
	switch strings.TrimSpace(view) {
	case "thread_timeline", "wakeup_chain":
		return true
	}
	return false
}

// traceQueryRelationScopeMaxDepth returns the waker-closure depth for pass-1
// discovery: at least the wakeup-chain query's default MaxDepth (10) plus one
// buffer hop so the pruned index covers one level deeper than expandChain walks.
func traceQueryRelationScopeMaxDepth(p traceQueryParams) int {
	d := p.MaxDepth.Int()
	if d < 10 {
		d = 10
	}
	return d + 1
}

// traceQueryScopedIndexMaxEvents translates the scoped in-memory byte budget
// into an event ceiling. Returns 0 (meaning "use the default cap") if the
// budget is not configured above the default.
func traceQueryScopedIndexMaxEvents() int {
	if traceQueryScopedIndexMaxBytes <= 0 {
		return 0
	}
	return int(traceQueryScopedIndexMaxBytes / traceIndexEventSizeEstimateBytes)
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
		ToolName:   t.Name(),
		Success:    true,
		Summary:    preview,
		RawRef:     rawRef,
		Refinement: traceQueryHeavyViewGuardRefinement(ctx, p, sourceLabel, path),
		Timestamp:  time.Now(),
	}, true
}

func traceQueryIsHeavyView(view string) bool {
	switch strings.TrimSpace(view) {
	case "scheduler_latency_stats", "root_cause_rank", "window_stats", "critical_blocking_calls", "evidence_pack", "recipe",
		"span_window", "frame_window", "render_pipeline", "frame_timeline", "frame_flow", "frame_root_cause_bundle",
		"thread_timeline", "ipc_graph", "wakeup_chain", "interaction_stats", "perf_stats", "perf_timeline", "trace_perf_bundle":
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
	fmt.Fprintf(&b, "next_call_hint=first narrow the trace with trace_query(view=\"event_search\", pattern=\"<frame id / jank id / exact timestamp / span label>\", event_types=[\"trace_mark\"], limit=40), or trace_query(view=\"span_window\", span_name=\"<span label>\"), then rerun this same view with time_start/time_end or line_start/line_end. For jank/stall analysis, use frame/span-derived windows or roughly %.0f-%.0fms coverage windows before trying sub-%.0fms micro-probes.\n",
		traceQueryPreferredCoverageWindowMinSeconds*1000,
		traceQueryPreferredCoverageWindowMaxSeconds*1000,
		traceQueryMicroWindowProbeSeconds*1000)
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
		if strings.TrimSpace(p.Path) == "" || traceQueryPathDefaultsToAttachedTrace(ctx, p.Path) {
			source = "attached_trace"
		} else {
			source = "path"
		}
	}
	if source == "path" && traceQueryPathDefaultsToAttachedTrace(ctx, p.Path) {
		source = "attached_trace"
	}
	if source == "attached_trace" {
		if path, ok := resolveAttachedTraceQueryPath(ctx); ok {
			return path, "attached_trace", nil
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
	resolved := resolveToolPath(ctx, p.Path)
	if info, err := os.Stat(resolved); err == nil && info.IsDir() {
		return "", source, &types.ToolResult{
			ToolName: "trace_query",
			Success:  false,
			Summary: fmt.Sprintf(
				"trace_query source=path requires a trace file, but path %q resolves to a directory. Use source=\"attached_trace\" for the current attached --htrace/--atrace blob, or pass an explicit .systrace/.htrace/.atrace/.perftrace/.tracebundle.json file.",
				strings.TrimSpace(p.Path),
			),
			Timestamp: time.Now(),
		}
	}
	return resolved, "path", nil
}

func resolveAttachedTraceQueryPath(ctx *types.BusContext) (string, bool) {
	if ctx != nil && strings.TrimSpace(ctx.WorkDir) != "" {
		blob := filepath.Join(ctx.WorkDir, promptctx.AttachedTraceBlobName)
		if _, err := os.Stat(blob); err == nil {
			return blob, true
		}
	}
	if ctx != nil && strings.TrimSpace(ctx.AttachedHitrace) != "" {
		ref := StoreBlobArtifact(ctx.WorkDir, "trace_query", promptctx.AttachedTraceBlobName, ctx.AttachedHitrace)
		if strings.TrimSpace(ref) != "" {
			return ref, true
		}
	}
	return "", false
}

func traceQueryPathDefaultsToAttachedTrace(ctx *types.BusContext, rawPath string) bool {
	if _, ok := resolveAttachedTraceQueryPath(ctx); !ok {
		return false
	}
	raw := strings.TrimSpace(rawPath)
	if raw == "" || raw == "." || raw == "./" {
		return true
	}
	resolved := filepath.Clean(resolveToolPath(ctx, raw))
	if !filepath.IsAbs(resolved) {
		if abs, err := filepath.Abs(resolved); err == nil {
			resolved = filepath.Clean(abs)
		}
	}
	for _, base := range []string{ctxRepoRoot(ctx), ctxWorkDir(ctx)} {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		base = filepath.Clean(base)
		if abs, err := filepath.Abs(base); err == nil {
			base = filepath.Clean(abs)
		}
		if resolved == base {
			return true
		}
	}
	return false
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
	case "sched_wakeup_new":
		return "sched_wakeup"
	case "sched_stat", "schedstat", "scheduler_accounting", "sched_stat_accounting", "sched_stat_wait", "sched_stat_sleep", "sched_stat_iowait", "sched_stat_blocked", "sched_stat_runtime":
		return "sched_stat"
	case "block_rq_insert", "block_getrq", "block_bio_queue":
		return "block_rq_issue"
	case "block_bio_complete":
		return "block_rq_complete"
	case "print", "tracing_mark_write_xacct", "xacct_tracing_mark_write":
		return "trace_mark"
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
	case "ipi_activity", "ipi_raise", "ipi_entry", "ipi_exit":
		return "ipi"
	case "block_inode", "block_io_inode", "block_io_by_inode":
		return "storage_latency"
	case "affinity", "cpu_affinity", "cpuaffinity", "cpuset", "sched_migrate", "sched_migration", "migration", "cpu_constraint", "cpu_constraints":
		return "cpu_constraint"
	case "perf", "sample", "samples", "perf_samples", "perfsample", "perfsamples", "perf_event", "cpu_sample", "cpu_samples", "cpusample", "cpusamples", "top_symbol", "top_symbols", "callchain", "callchains":
		return "perf_sample"
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
	if ctx != nil && (strings.TrimSpace(sourceLabel) == "attached_trace" || traceQueryPathIsAttachedTraceBlob(ctx, resolvedPath)) {
		if platform := tracequery.NormalizeTracePlatform(ctx.AttachedHitraceSource); platform != "" && platform != tracequery.TracePlatformAuto {
			return platform, "attached_source"
		}
	}
	return tracequery.TracePlatformAuto, ""
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
		ToolName:   t.Name(),
		Success:    true,
		Summary:    preview,
		RawRef:     rawRef,
		Refinement: traceQueryRecipeDiscoveryRefinement(ctx, p, sourceLabel, path, markers, truncated),
		Timestamp:  time.Now(),
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
	if diagnostic := traceQueryIndexDiagnostic(result); diagnostic != "" {
		fmt.Fprintf(&b, "parse_diagnostic=%s\n", diagnostic)
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
			fmt.Fprintf(&b, "- span %s %q %.6f..%.6f kind=%s duration=%.3fms lines=%d-%d\n",
				traceThreadLabel(span.Thread), span.Name, span.StartTs, span.EndTs, firstNonEmptyTraceString(span.Kind, "sync"), span.DurationMs, span.StartLine, span.EndLine)
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
			projection := traceQueryProjectedActualFields(impact.ProjectedImpactMs, impact.ProjectedTotalMs, impact.ActualImpactMs, impact.ActualTotalMs, impact.ActualWindow.StartTs, impact.ActualWindow.EndTs)
			fmt.Fprintf(&b, "- causal_impact thread=%s depth=%d causality=%s dominant_state=%s impact=%.3fms total=%.3fms target_impact=%.3fms%s fragments=%d switches=%d max_segment=%.3fms p95_segment=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms prio=%d/%s target_prio=%d/%s priority_relation=%s priority_inversion_candidate=%t lines=%d-%d — %s\n",
				traceThreadLabel(impact.Thread), impact.ChainDepth, traceQueryCausalityLabel(impact.OnChain),
				sanitizeForBanner(impact.DominantState), impact.DominantImpactMs, impact.TotalMs, impact.TargetBlockedMs,
				projection, impact.FragmentCount, impact.StateSwitches, impact.MaxSegmentMs, impact.P95SegmentMs,
				impact.RunningMs, impact.RunnableMs, impact.SleepMs, impact.DStateMs, impact.IOWaitMs,
				impact.Priority, sanitizeForBanner(impact.PriorityClass), impact.TargetPriority, sanitizeForBanner(impact.TargetPriorityClass),
				sanitizeForBanner(impact.PriorityRelation), impact.PriorityInversionCandidate,
				impact.LineStart, impact.LineEnd, sanitizeForBanner(impact.Summary))
		}
		for _, aggregate := range result.WakeupChain.AggregatedImpacts {
			occurrenceWindows := traceQueryOccurrenceWindowsCompact(aggregate.OccurrenceWindows, 4)
			projection := traceQueryProjectedActualFields(aggregate.ProjectedImpactMs, aggregate.ProjectedTotalMs, aggregate.ActualImpactMs, aggregate.ActualTotalMs, aggregate.ActualFirstTs, aggregate.ActualLastTs)
			fmt.Fprintf(&b, "- aggregated_impact thread=%s path=%s depth=%d occurrences=%d occurrence_windows=%s dominant_state=%s impact=%.3fms total=%.3fms target_impact=%.3fms%s fragments=%d switches=%d max_segment=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms priority_relation=%s priority_inversion_candidate=%t lines=%d-%d — %s\n",
				traceThreadLabel(aggregate.Thread), sanitizeForBanner(aggregate.Path), aggregate.ChainDepth, aggregate.OccurrenceCount,
				occurrenceWindows, sanitizeForBanner(aggregate.DominantState), aggregate.DominantImpactMs, aggregate.TotalMs, aggregate.TargetBlockedMs,
				projection, aggregate.FragmentCount, aggregate.StateSwitches, aggregate.MaxSegmentMs,
				aggregate.RunningMs, aggregate.RunnableMs, aggregate.SleepMs, aggregate.DStateMs, aggregate.IOWaitMs,
				sanitizeForBanner(aggregate.PriorityRelation), aggregate.PriorityInversion, aggregate.LineStart, aggregate.LineEnd, sanitizeForBanner(aggregate.Summary))
			traceQueryWriteOccurrenceRows(&b, "aggregate_occurrence", 0, aggregate.Thread, aggregate.OccurrenceWindows)
		}
		for _, root := range result.WakeupChain.RootEvidence {
			fmt.Fprintf(&b, "- root_evidence=%s thread=%s duration=%.3fms lines=%d-%d confidence=%.2f — %s\n",
				root.Type, traceThreadLabel(root.Thread), root.DurationMs, root.LineStart, root.LineEnd, root.Confidence, root.Summary)
		}
		for _, wait := range result.WakeupChain.BinderWaits {
			fmt.Fprintf(&b, "- binder_wait transaction=%d %s -> %s duration=%.3fms flags=%s oneway=%t sync_like=%t blocking_candidate=%t send_line=%d receive_line=%d sleep_line=%d wake_line=%d confidence=%.2f — %s\n",
				wait.TransactionID, traceThreadLabel(wait.Thread), traceThreadLabel(wait.Peer), wait.DurationMs, sanitizeForBanner(wait.Flags), wait.Oneway, wait.SyncLike, wait.BlockingCandidate, wait.SendLine, wait.ReceiveLine, wait.SleepLine, wait.WakeupLine, wait.Confidence, wait.Summary)
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
			occurrenceWindows := traceQueryOccurrenceWindowsCompact(item.OccurrenceWindows, 4)
			projection := traceQueryProjectedActualFields(item.ProjectedImpactMs, item.CumulativeImpactMs, item.ActualImpactMs, item.ActualTotalMs, item.ActualStartTs, item.ActualEndTs)
			fmt.Fprintf(&b, "- rank=%d tier=%s type=%s thread=%s window=%.6f..%.6f occurrence_windows=%s dominant_state=%s running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms impact=%.3fms cumulative_impact=%.3fms effective_impact=%.3fms target_impact=%.3fms%s score=%.3f confidence=%.2f lines=%d-%d source=%s causality=%s chain_relevance=%s chain_depth=%d overlap=%.3fms edge_count=%d nearest_chain=%s nearest_window=%.6f..%.6f span=%s perf_context=%s perf_contexts=%s — %s\n",
				item.Rank, item.Tier, item.Type, traceThreadLabel(item.Thread), item.StartTs, item.EndTs,
				occurrenceWindows, sanitizeForBanner(item.DominantState), item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs,
				item.ImpactMs, item.CumulativeImpactMs, traceQueryRootCauseEffectiveImpact(item), item.TargetImpactMs, projection, item.Score, item.Confidence,
				item.LineStart, item.LineEnd, item.Source, sanitizeForBanner(item.Causality), sanitizeForBanner(item.ChainRelevance), item.ChainDepth, item.OverlapMs, item.EdgeCount,
				traceThreadLabel(item.NearestChainThread), item.NearestChainWindow.StartTs, item.NearestChainWindow.EndTs, traceQueryRootCauseSpanCompact(item), traceQueryPerfContextCompact(item.PerfContext), traceQueryPerfRoleContextsCompact(item.PerfContexts, 4), item.Summary)
			writeTraceRootCausePerfRoles(&b, item.Rank, item.PerfContexts)
			traceQueryWriteOccurrenceRows(&b, "rank_occurrence", item.Rank, item.Thread, item.OccurrenceWindows)
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
		for _, td := range result.WindowStats.SleepTop {
			fmt.Fprintf(&b, "- top_sleep %s %.3fms %s%s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), traceThreadDurationLocation(td), td.LineStart, td.LineEnd)
		}
		for _, td := range result.WindowStats.DStateTop {
			fmt.Fprintf(&b, "- top_d_state %s %.3fms %s%s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), traceThreadDurationLocation(td), td.LineStart, td.LineEnd)
		}
		for _, td := range result.WindowStats.IOWaitTop {
			fmt.Fprintf(&b, "- top_io_wait %s %.3fms %s%s lines=%d-%d\n", traceThreadLabel(td.Thread), td.DurationMs, tracePriorityDetail(td), traceThreadDurationLocation(td), td.LineStart, td.LineEnd)
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
			fmt.Fprintf(&b, "- trace_span %s %q category=%s subcategory=%s semantic_class=%s kind=%s duration=%.3fms lines=%d-%d\n",
				traceThreadLabel(span.Thread), span.Name, sanitizeForBanner(span.Category), sanitizeForBanner(span.Subcategory), sanitizeForBanner(span.SemanticClass), firstNonEmptyTraceString(span.Kind, "sync"), span.DurationMs, span.StartLine, span.EndLine)
		}
		for _, category := range result.WindowStats.TraceMarkCategories {
			fmt.Fprintf(&b, "- trace_mark_category category=%s subcategory=%s count=%d total=%.3fms max=%.3fms top_span=%s top_thread=%s lines=%d-%d — %s\n",
				sanitizeForBanner(category.Category), sanitizeForBanner(category.Subcategory), category.Count, category.TotalMs, category.MaxDurationMs, sanitizeForBanner(category.TopSpan), traceThreadLabel(category.TopThread), category.LineStart, category.LineEnd, sanitizeForBanner(category.Summary))
		}
		for _, work := range result.WindowStats.AsyncFileWork {
			fmt.Fprintf(&b, "- async_file_work %s category=%s span=%s duration=%.3fms lines=%d-%d — %s\n",
				traceThreadLabel(work.Thread), sanitizeForBanner(work.Category), sanitizeForBanner(work.Name), work.DurationMs, work.LineStart, work.LineEnd, sanitizeForBanner(work.Summary))
		}
		writeTraceStateDrilldownSummary(&b, result.WindowStats.StateDrilldownPlan)
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
		if result.WindowStats.PerfSamples != nil {
			writeTracePerfContext(&b, *result.WindowStats.PerfSamples)
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
		for _, ipi := range result.WindowStats.IPIActivity {
			fmt.Fprintf(&b, "- ipi_activity kind=%s cpu=%d core_class=%s name=%s count=%d paired=%d active=%.3fms max=%.3fms target_mask=%s target_cpus=%s lines=%d-%d — %s\n",
				sanitizeForBanner(ipi.Kind), ipi.CPU, sanitizeForBanner(ipi.CoreClass), sanitizeForBanner(ipi.Name), ipi.Count, ipi.PairedCount, ipi.ActiveMs, ipi.MaxActiveMs, sanitizeForBanner(ipi.TargetMask), traceIntList(ipi.TargetCPUs), ipi.LineStart, ipi.LineEnd, sanitizeForBanner(ipi.Summary))
		}
		for _, work := range result.WindowStats.WorkqueueActivity {
			fmt.Fprintf(&b, "- workqueue_activity %s work=%s function=%s count=%d paired=%d duration=%.3fms max=%.3fms lines=%d-%d — %s\n",
				traceThreadLabel(work.Thread), sanitizeForBanner(work.Work), sanitizeForBanner(work.Function), work.Count, work.PairedCount, work.DurationMs, work.MaxLatencyMs, work.LineStart, work.LineEnd, sanitizeForBanner(work.Summary))
		}
		for _, accounting := range result.WindowStats.SchedStatAccounting {
			fmt.Fprintf(&b, "- sched_stat_accounting %s kind=%s count=%d delay=%.3fms max_delay=%.3fms runtime=%.3fms max_runtime=%.3fms lines=%d-%d — %s\n",
				traceThreadLabel(accounting.Thread), sanitizeForBanner(accounting.Kind), accounting.Count, accounting.TotalDelayMs, accounting.MaxDelayMs, accounting.TotalRuntimeMs, accounting.MaxRuntimeMs, accounting.LineStart, accounting.LineEnd, sanitizeForBanner(accounting.Summary))
		}
		if result.WindowStats.SupplyPressureSummary != nil {
			supply := result.WindowStats.SupplyPressureSummary
			fmt.Fprintf(&b, "- supply_pressure signal=%s cpu_pressure=%.3fms runnable=%.3fms high_prio=%.3fms sched_stat_wait=%.3fms sched_stat_iowait=%.3fms sched_stat_blocked=%.3fms ipi_events=%d ipi_active=%.3fms low_freq_cpus=%v clock_set_rate=%d thermal=%d ddr=%d l3=%d throughput=%d lines=%d-%d — %s\n",
				sanitizeForBanner(supply.Signal), supply.CPUPressureMs, supply.RunnableWaitMs, supply.HighPriorityRunningMs, supply.SchedStatWaitMs, supply.SchedStatIOWaitMs, supply.SchedStatBlockedMs, supply.IPIEventCount, supply.IPIActiveMs, supply.LowFrequencyCPUs, supply.ClockSetRateCount, supply.ThermalEventCount, supply.DDREventCount, supply.L3EventCount, supply.ThroughputEventCount, supply.LineStart, supply.LineEnd, sanitizeForBanner(supply.Summary))
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
	if result.PerfTimeline != nil {
		b.WriteString("## Perf timeline\n")
		fmt.Fprintf(&b, "- perf_timeline bucket_ms=%.3f buckets=%d window=%.6f..%.6f\n",
			result.PerfTimeline.BucketMs, len(result.PerfTimeline.Buckets), result.PerfTimeline.Window.StartTs, result.PerfTimeline.Window.EndTs)
		for _, bucket := range result.PerfTimeline.Buckets {
			fmt.Fprintf(&b, "- perf_bucket %.6f..%.6f sample_weight=%d samples=%d top_symbol=%s top_dso=%s event=%s cpus=%v threads=%s lines=%d-%d example=%s\n",
				bucket.StartTs, bucket.EndTs, bucket.Period, bucket.SampleCount, sanitizeForBanner(bucket.TopSymbol), sanitizeForBanner(bucket.TopDSO), sanitizeForBanner(bucket.TopEvent), bucket.CPUs, traceThreadLabels(bucket.Threads), bucket.LineStart, bucket.LineEnd, sanitizeForBanner(bucket.Example))
		}
		for _, caveat := range result.PerfTimeline.Caveats {
			fmt.Fprintf(&b, "- perf_timeline_caveat=%s\n", sanitizeForBanner(caveat))
		}
		b.WriteString("\n")
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
			blockingSemantics := ""
			if item.Flags != "" || item.Oneway != nil || item.SyncLike != nil || item.BlockingCandidate != nil {
				blockingSemantics = fmt.Sprintf(" flags=%s oneway=%s sync_like=%s blocking_candidate=%s",
					sanitizeForBanner(item.Flags),
					traceQueryBoolPtrBanner(item.Oneway),
					traceQueryBoolPtrBanner(item.SyncLike),
					traceQueryBoolPtrBanner(item.BlockingCandidate))
			}
			fmt.Fprintf(&b, "- blocking type=%s thread=%s peer=%s%s chain_relevance=%s overlap=%.3fms edge_count=%d nearest_chain=%s duration=%.3fms lines=%d-%d confidence=%.2f — %s\n",
				item.Type, traceThreadLabel(item.Thread), traceThreadLabel(item.Peer), blockingSemantics, sanitizeForBanner(item.ChainRelevance), item.OverlapMs, item.EdgeCount, traceThreadLabel(item.NearestChainThread), item.DurationMs, item.LineStart, item.LineEnd, item.Confidence, item.Summary)
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
			raw := strings.TrimSpace(ev.Raw)
			if ev.Type == tracequery.EventPerfSample {
				raw = traceQueryPerfSampleRawForModel(raw)
			}
			fmt.Fprintf(&b, "- line=%d ts=%.6f type=%s thread=%s%s%s%s raw=%s\n",
				ev.Line,
				ev.Ts,
				ev.Type,
				traceThreadLabel(tracequery.ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}),
				traceEventPriorityDetail(ev),
				traceEventSchedulerDetail(ev),
				traceEventResourceDetail(ev),
				raw,
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
	if bundle.TargetResolution != nil {
		resolution := bundle.TargetResolution
		fmt.Fprintf(b, "- target_resolution source=%s target=%s confidence=%.2f window_source=%s window=%.6f..%.6f candidates=%d\n",
			sanitizeForBanner(resolution.Source), traceThreadLabel(resolution.Target), resolution.Confidence,
			sanitizeForBanner(resolution.WindowSource), resolution.Window.StartTs, resolution.Window.EndTs, len(resolution.Candidates))
		if resolution.SelectedFrame != nil {
			selected := resolution.SelectedFrame
			fmt.Fprintf(b, "  selected_frame role=%s phase=%s thread=%s frame_id=%s %.6f..%.6f lines=%d-%d name=%s\n",
				sanitizeForBanner(selected.Role), sanitizeForBanner(selected.Phase), traceThreadLabel(selected.Thread),
				sanitizeForBanner(selected.FrameID), selected.Window.StartTs, selected.Window.EndTs,
				selected.StartLine, selected.EndLine, sanitizeForBanner(selected.Name))
		}
	}
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
	writeTracePerfContextRole(b, "bundle_perf_samples", bundle.PerfSamples)
	writeTracePerfContextRole(b, "bundle_target_running_perf", bundle.TargetRunningPerf)
	writeTracePerfContextRole(b, "bundle_on_chain_perf", bundle.OnChainPerf)
	writeTracePerfContextRole(b, "bundle_binder_peer_perf", bundle.BinderPeerPerf)
	writeTracePerfContextRole(b, "bundle_same_cpu_competitor_perf", bundle.SameCPUCompetitorPerf)
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

func writeTracePerfContextRole(b *strings.Builder, role string, ctx *tracequery.PerfContext) {
	if b == nil || ctx == nil {
		return
	}
	fmt.Fprintf(b, "- %s sample_count=%d total_sample_weight=%d summary=%s\n", role, ctx.SampleCount, ctx.TotalPeriod, traceQueryPerfContextCompact(ctx))
	writeTracePerfQuality(b, role+"_quality", ctx.Quality)
	if len(ctx.TopCallchains) > 0 {
		hot := ctx.TopCallchains[0]
		fmt.Fprintf(b, "  %s_top_callchain callchain=%s symbol=%s dso=%s weight_unit=%s sample_weight=%d samples=%d lines=%d-%d\n",
			role, sanitizeForBanner(hot.Callchain), sanitizeForBanner(hot.Symbol), sanitizeForBanner(hot.DSO), sanitizeForBanner(hot.WeightUnit), hot.Period, hot.SampleCount, hot.LineStart, hot.LineEnd)
	}
	if len(ctx.TopThreads) > 0 {
		thread := ctx.TopThreads[0]
		fmt.Fprintf(b, "  %s_top_thread thread=%s sample_weight=%d samples=%d cpus=%v lines=%d-%d\n",
			role, traceThreadLabel(thread.Thread), thread.Period, thread.SampleCount, thread.CPUs, thread.LineStart, thread.LineEnd)
	}
}

func writeTraceRootCausePerfRoles(b *strings.Builder, rank int, contexts []tracequery.RootCausePerfRoleContext) {
	if b == nil || len(contexts) == 0 {
		return
	}
	for i, role := range contexts {
		if i >= 4 {
			fmt.Fprintf(b, "  rank_perf_contexts_omitted=%d\n", len(contexts)-i)
			break
		}
		if role.PerfContext == nil || role.PerfContext.SampleCount == 0 {
			continue
		}
		fmt.Fprintf(b, "  rank_perf_context rank=%d role=%s thread=%s cpu=%d window=%.6f..%.6f samples=%d sample_weight=%d reason=%s quality=%s summary=%s\n",
			rank,
			sanitizeForBanner(role.Role),
			traceThreadLabel(role.Thread),
			role.CPU,
			role.Window.StartTs,
			role.Window.EndTs,
			role.PerfContext.SampleCount,
			role.PerfContext.TotalPeriod,
			sanitizeForBanner(role.Reason),
			traceQueryPerfQualityCompact(role.PerfContext.Quality),
			traceQueryPerfContextCompact(role.PerfContext),
		)
		if len(role.PerfContext.TopCallchains) > 0 {
			hot := role.PerfContext.TopCallchains[0]
			fmt.Fprintf(b, "    rank_perf_top_callchain role=%s callchain=%s symbol=%s dso=%s weight_unit=%s source=%s symbolization_status=%s sample_weight=%d samples=%d lines=%d-%d\n",
				sanitizeForBanner(role.Role),
				sanitizeForBanner(hot.Callchain),
				sanitizeForBanner(hot.Symbol),
				sanitizeForBanner(hot.DSO),
				sanitizeForBanner(hot.WeightUnit),
				sanitizeForBanner(hot.Source),
				sanitizeForBanner(hot.SymbolizationStatus),
				hot.Period,
				hot.SampleCount,
				hot.LineStart,
				hot.LineEnd,
			)
		}
	}
}

func writeTraceIPCEdges(b *strings.Builder, edges []tracequery.IPCEdge) {
	for i, edge := range edges {
		if i >= 12 {
			fmt.Fprintf(b, "... omitted %d IPC edge(s); see payload_ref\n", len(edges)-i)
			break
		}
		fmt.Fprintf(b, "- ipc transaction=%d %s -> %s send_line=%d receive_line=%d latency=%.3fms reply=%d flags=%s code=%s oneway=%t sync_like=%t blocking_candidate=%t confidence=%.2f\n",
			edge.TransactionID,
			traceThreadLabel(edge.Sender),
			traceThreadLabel(edge.Receiver),
			edge.SendLine,
			edge.ReceiveLine,
			edge.LatencyMs,
			edge.Reply,
			edge.Flags,
			edge.Code,
			edge.Oneway,
			edge.SyncLike,
			edge.BlockingCandidate,
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

func writeTracePerfContext(b *strings.Builder, item tracequery.PerfContext) {
	fmt.Fprintf(b, "- perf_samples sample_count=%d total_sample_weight=%d\n", item.SampleCount, item.TotalPeriod)
	writeTracePerfQuality(b, "perf_quality", item.Quality)
	for _, hot := range item.TopSymbols {
		fmt.Fprintf(b, "- perf_top_symbol symbol=%s dso=%s event=%s weight_unit=%s source=%s symbolization_status=%s sample_weight=%d samples=%d percent=%.2f cpus=%v threads=%s lines=%d-%d example=%s\n",
			sanitizeForBanner(hot.Symbol), sanitizeForBanner(hot.DSO), sanitizeForBanner(hot.Event), sanitizeForBanner(hot.WeightUnit), sanitizeForBanner(hot.Source), sanitizeForBanner(hot.SymbolizationStatus), hot.Period, hot.SampleCount, hot.Percent, hot.CPUs, traceThreadLabels(hot.Threads), hot.LineStart, hot.LineEnd, sanitizeForBanner(hot.Example))
	}
	for _, hot := range item.TopDSO {
		fmt.Fprintf(b, "- perf_top_dso dso=%s event=%s weight_unit=%s source=%s symbolization_status=%s sample_weight=%d samples=%d percent=%.2f cpus=%v threads=%s lines=%d-%d example=%s\n",
			sanitizeForBanner(hot.DSO), sanitizeForBanner(hot.Event), sanitizeForBanner(hot.WeightUnit), sanitizeForBanner(hot.Source), sanitizeForBanner(hot.SymbolizationStatus), hot.Period, hot.SampleCount, hot.Percent, hot.CPUs, traceThreadLabels(hot.Threads), hot.LineStart, hot.LineEnd, sanitizeForBanner(hot.Example))
	}
	for _, hot := range item.TopCallchains {
		fmt.Fprintf(b, "- perf_top_callchain callchain=%s symbol=%s dso=%s event=%s weight_unit=%s source=%s symbolization_status=%s sample_weight=%d samples=%d percent=%.2f cpus=%v threads=%s lines=%d-%d example=%s\n",
			sanitizeForBanner(hot.Callchain), sanitizeForBanner(hot.Symbol), sanitizeForBanner(hot.DSO), sanitizeForBanner(hot.Event), sanitizeForBanner(hot.WeightUnit), sanitizeForBanner(hot.Source), sanitizeForBanner(hot.SymbolizationStatus), hot.Period, hot.SampleCount, hot.Percent, hot.CPUs, traceThreadLabels(hot.Threads), hot.LineStart, hot.LineEnd, sanitizeForBanner(hot.Example))
	}
	for _, thread := range item.TopThreads {
		fmt.Fprintf(b, "- perf_top_thread thread=%s sample_weight=%d samples=%d percent=%.2f cpus=%v lines=%d-%d example=%s\n",
			traceThreadLabel(thread.Thread), thread.Period, thread.SampleCount, thread.Percent, thread.CPUs, thread.LineStart, thread.LineEnd, sanitizeForBanner(thread.Example))
	}
	for _, hot := range item.TopEvents {
		fmt.Fprintf(b, "- perf_top_event event=%s weight_unit=%s sample_weight=%d samples=%d percent=%.2f cpus=%v threads=%s lines=%d-%d example=%s\n",
			sanitizeForBanner(hot.Event), sanitizeForBanner(hot.WeightUnit), hot.Period, hot.SampleCount, hot.Percent, hot.CPUs, traceThreadLabels(hot.Threads), hot.LineStart, hot.LineEnd, sanitizeForBanner(hot.Example))
	}
}

func traceQueryPerfContextCompact(ctx *tracequery.PerfContext) string {
	if ctx == nil || ctx.SampleCount == 0 {
		return "none"
	}
	parts := []string{
		fmt.Sprintf("samples=%d", ctx.SampleCount),
		fmt.Sprintf("sample_weight=%d", ctx.TotalPeriod),
	}
	if len(ctx.TopSymbols) > 0 {
		hot := ctx.TopSymbols[0]
		parts = append(parts, fmt.Sprintf("top_symbol=%s", sanitizeForBanner(firstNonEmptyTraceString(hot.Symbol, "unknown"))))
		if hot.DSO != "" {
			parts = append(parts, fmt.Sprintf("dso=%s", sanitizeForBanner(hot.DSO)))
		}
		parts = append(parts, fmt.Sprintf("top_sample_weight=%d", hot.Period))
	}
	if len(ctx.TopThreads) > 0 {
		parts = append(parts, fmt.Sprintf("top_thread=%s", traceThreadLabel(ctx.TopThreads[0].Thread)))
	}
	if quality := traceQueryPerfQualityCompact(ctx.Quality); quality != "" && quality != "none" {
		parts = append(parts, "quality="+quality)
	}
	return strings.Join(parts, " ")
}

func traceQueryRootCauseSpanCompact(item tracequery.RootCauseRankItem) string {
	var parts []string
	if item.SpanName != "" {
		parts = append(parts, "name="+sanitizeForBanner(item.SpanName))
	}
	if item.EffectiveImpactMs > 0 {
		parts = append(parts, fmt.Sprintf("effective_impact=%.3fms", item.EffectiveImpactMs))
	}
	if item.SemanticClass != "" {
		parts = append(parts, "semantic_class="+sanitizeForBanner(item.SemanticClass))
	}
	if item.SpanCategory != "" {
		parts = append(parts, "category="+sanitizeForBanner(item.SpanCategory))
	}
	if item.SpanSubcategory != "" {
		parts = append(parts, "subcategory="+sanitizeForBanner(item.SpanSubcategory))
	}
	if item.SpanKind != "" {
		parts = append(parts, "kind="+sanitizeForBanner(item.SpanKind))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

func traceQueryRootCauseEffectiveImpact(item tracequery.RootCauseRankItem) float64 {
	if item.EffectiveImpactMs > 0 {
		return item.EffectiveImpactMs
	}
	if item.CumulativeImpactMs > 0 {
		return item.CumulativeImpactMs
	}
	if item.ImpactMs > 0 {
		return item.ImpactMs
	}
	return item.TargetImpactMs
}

func traceQueryPerfRoleContextsCompact(contexts []tracequery.RootCausePerfRoleContext, max int) string {
	if len(contexts) == 0 {
		return "none"
	}
	if max <= 0 || max > len(contexts) {
		max = len(contexts)
	}
	parts := make([]string, 0, max)
	for _, role := range contexts {
		if len(parts) >= max || role.PerfContext == nil || role.PerfContext.SampleCount == 0 {
			continue
		}
		fields := []string{
			sanitizeForBanner(role.Role),
			fmt.Sprintf("samples=%d", role.PerfContext.SampleCount),
			fmt.Sprintf("sample_weight=%d", role.PerfContext.TotalPeriod),
		}
		if label := traceThreadLabelOptional(role.Thread); label != "" {
			fields = append(fields, "thread="+label)
		}
		if role.CPU >= 0 {
			fields = append(fields, fmt.Sprintf("cpu=%d", role.CPU))
		}
		if len(role.PerfContext.TopSymbols) > 0 {
			hot := role.PerfContext.TopSymbols[0]
			fields = append(fields, "top_symbol="+sanitizeForBanner(firstNonEmptyTraceString(hot.Symbol, "unknown")))
			if hot.DSO != "" {
				fields = append(fields, "dso="+sanitizeForBanner(hot.DSO))
			}
			if hot.Source != "" {
				fields = append(fields, "source="+sanitizeForBanner(hot.Source))
			}
			if hot.SymbolizationStatus != "" {
				fields = append(fields, "symbolization_status="+sanitizeForBanner(hot.SymbolizationStatus))
			}
		}
		if quality := traceQueryPerfQualityCompact(role.PerfContext.Quality); quality != "" && quality != "none" {
			fields = append(fields, "quality="+quality)
		}
		parts = append(parts, strings.Join(fields, " "))
	}
	if len(parts) == 0 {
		return "none"
	}
	if len(contexts) > max {
		parts = append(parts, fmt.Sprintf("omitted=%d", len(contexts)-max))
	}
	return strings.Join(parts, " | ")
}

func writeTracePerfQuality(b *strings.Builder, label string, q *tracequery.PerfQualitySummary) {
	if b == nil || q == nil {
		return
	}
	fmt.Fprintf(b, "- %s cpu_known=%d cpu_unknown=%d sample_cpu_scope=%s callchain_known=%d callchain_unknown=%d sources=%s symbolization=%s sample_kind=%s weight_unit=%s clocks=%s clock_confidence=%s callchain_status=%s\n",
		label,
		q.CPUKnownCount,
		q.CPUUnknownCount,
		perfQualitySampleCPUScope(q),
		q.CallchainKnownCount,
		q.CallchainUnknownCount,
		traceQueryPerfValueCountsCompact(q.Sources),
		traceQueryPerfValueCountsCompact(q.SymbolizationStatuses),
		traceQueryPerfValueCountsCompact(q.SampleKinds),
		traceQueryPerfValueCountsCompact(q.WeightUnits),
		traceQueryPerfValueCountsCompact(q.Clocks),
		traceQueryPerfValueCountsCompact(q.ClockConfidences),
		traceQueryPerfValueCountsCompact(q.CallchainStatuses),
	)
	for _, caveat := range q.Caveats {
		fmt.Fprintf(b, "  %s_caveat=%s\n", label, sanitizeForBanner(caveat))
	}
}

func traceQueryPerfQualityCompact(q *tracequery.PerfQualitySummary) string {
	if q == nil {
		return "none"
	}
	parts := []string{
		fmt.Sprintf("cpu_known=%d", q.CPUKnownCount),
		fmt.Sprintf("cpu_unknown=%d", q.CPUUnknownCount),
		"sample_cpu_scope=" + perfQualitySampleCPUScope(q),
	}
	if len(q.Sources) > 0 {
		parts = append(parts, "source="+sanitizeForBanner(q.Sources[0].Value))
	}
	if len(q.SymbolizationStatuses) > 0 {
		parts = append(parts, "symbolization="+sanitizeForBanner(q.SymbolizationStatuses[0].Value))
	}
	if len(q.SampleKinds) > 0 {
		parts = append(parts, "sample_kind="+sanitizeForBanner(q.SampleKinds[0].Value))
	}
	if len(q.WeightUnits) > 0 {
		parts = append(parts, "weight_unit="+sanitizeForBanner(q.WeightUnits[0].Value))
	}
	if len(q.Clocks) > 0 {
		parts = append(parts, "clock="+sanitizeForBanner(q.Clocks[0].Value))
	}
	if len(q.ClockConfidences) > 0 {
		parts = append(parts, "clock_confidence="+sanitizeForBanner(q.ClockConfidences[0].Value))
	}
	if len(q.CallchainStatuses) > 0 {
		parts = append(parts, "callchain_status="+sanitizeForBanner(q.CallchainStatuses[0].Value))
	}
	return strings.Join(parts, ",")
}

func perfQualitySampleCPUScope(q *tracequery.PerfQualitySummary) string {
	if q == nil {
		return "none"
	}
	switch {
	case q.CPUKnownCount > 0 && q.CPUUnknownCount > 0:
		return "partial"
	case q.CPUKnownCount > 0:
		return "known"
	case q.CPUUnknownCount > 0:
		return "unknown"
	default:
		return "none"
	}
}

func traceQueryPerfValueCountsCompact(values []tracequery.PerfValueCount) string {
	if len(values) == 0 {
		return "none"
	}
	const maxValues = 3
	limit := len(values)
	if limit > maxValues {
		limit = maxValues
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		value := values[i]
		parts = append(parts, fmt.Sprintf("%s:%d/%d(%.1f%%)", sanitizeForBanner(value.Value), value.SampleCount, value.Period, value.Percent))
	}
	if len(values) > maxValues {
		parts = append(parts, fmt.Sprintf("+%d", len(values)-maxValues))
	}
	return strings.Join(parts, ",")
}

func perfQualityCaveatsForTraceQuery(q *tracequery.PerfQualitySummary) []string {
	if q == nil {
		return nil
	}
	return q.Caveats
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

func writeTraceStateDrilldownSummary(b *strings.Builder, steps []tracequery.StateDrilldownStep) {
	const summaryCap = 5
	for i, step := range steps {
		if i >= summaryCap {
			fmt.Fprintf(b, "- state_drilldown_omitted count=%d see payload_ref\n", len(steps)-summaryCap)
			return
		}
		fmt.Fprintf(b, "- state_drilldown rank=%d thread=%s state=%s impact=%.3fms total=%.3fms source=%s chain_required=%t recursive=%t window_proportion=%.4f significant=%t recommended_views=%s lines=%d-%d — %s\n",
			step.Rank, traceThreadLabel(step.Thread), sanitizeForBanner(step.State), step.ImpactMs, step.TotalMs, sanitizeForBanner(step.Source),
			step.ChainRequired, step.Recursive, step.WindowProportion, step.Significant, sanitizeForBanner(strings.Join(step.RecommendedViews, ",")), step.LineStart, step.LineEnd, sanitizeForBanner(step.Summary))
	}
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
	if ev.Type == tracequery.EventPerfSample {
		parts = append(parts, fmt.Sprintf("perf_sample pid=%d tid=%d sample_weight=%d event=%s symbol=%s dso=%s source=%s sample_kind=%s symbolization_status=%s cpu_known=%s clock=%s clock_confidence=%s callchain_status=%s callchain=%s",
			ev.PerfPID,
			ev.PerfTID,
			ev.PerfPeriod,
			sanitizeForBanner(ev.PerfEvent),
			sanitizeForBanner(ev.PerfSymbol),
			sanitizeForBanner(ev.PerfDSO),
			sanitizeForBanner(ev.PerfSource),
			sanitizeForBanner(ev.PerfSampleKind),
			sanitizeForBanner(ev.PerfSymbolizationStatus),
			traceQueryBoolPtrBanner(ev.PerfCPUKnown),
			sanitizeForBanner(ev.PerfClock),
			sanitizeForBanner(ev.PerfClockConfidence),
			sanitizeForBanner(ev.PerfCallchainStatus),
			sanitizeForBanner(ev.PerfCallchain)))
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

func traceQueryPerfSampleRawForModel(raw string) string {
	raw = strings.ReplaceAll(raw, " period=", " sample_weight=")
	raw = strings.ReplaceAll(raw, " sample_period=", " sample_weight=")
	raw = strings.ReplaceAll(raw, " event_count=", " sample_weight=")
	return raw
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

func traceQueryIndexDiagnostic(result tracequery.Result) string {
	if result.EventCount > 0 {
		return ""
	}
	if result.IndexWindowed {
		return "zero_events_in_selected_index_window; ftrace-compatible text is supported, so verify time_start/time_end/line_start/line_end and timestamp units before concluding parser incompatibility"
	}
	if result.ScannedLineCount > 0 && result.UnparsedLineCount >= result.ScannedLineCount {
		return "zero_events_all_scanned_lines_unparsed; input may be non-ftrace text, converted incorrectly, compressed/binary, or need a converter/parser adapter"
	}
	return "zero_events_in_trace_index; verify the file contains ftrace-compatible timestamped rows or pass a converted systrace/ftrace text artifact"
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

func traceThreadLabels(threads []tracequery.ThreadRef) string {
	if len(threads) == 0 {
		return "[]"
	}
	labels := make([]string, 0, len(threads))
	for _, thread := range threads {
		labels = append(labels, traceThreadLabel(thread))
	}
	return "[" + strings.Join(labels, ",") + "]"
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

func traceQueryProjectedActualFields(projectedImpact, projectedTotal, actualImpact, actualTotal, actualStart, actualEnd float64) string {
	var fields []string
	if projectedImpact > 0 {
		fields = append(fields, fmt.Sprintf("projected_impact=%.3fms", projectedImpact))
	}
	if projectedTotal > 0 {
		fields = append(fields, fmt.Sprintf("projected_total=%.3fms", projectedTotal))
	}
	if actualImpact > 0 {
		fields = append(fields, fmt.Sprintf("actual_impact=%.3fms", actualImpact))
	}
	if actualTotal > 0 {
		fields = append(fields, fmt.Sprintf("actual_total=%.3fms", actualTotal))
	}
	if actualWindow := traceQueryWindowValue(actualStart, actualEnd); actualWindow != "" {
		fields = append(fields, "actual_window="+actualWindow)
	}
	if len(fields) == 0 {
		return ""
	}
	return " " + strings.Join(fields, " ")
}

func traceQueryOccurrenceWindowsCompact(items []tracequery.WakeupCausalOccurrence, limit int) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		item := items[i]
		window := traceQueryWindowValue(item.Window.StartTs, item.Window.EndTs)
		if window == "" {
			continue
		}
		fields := []string{window}
		if item.DominantState != "" {
			fields = append(fields, "state="+sanitizeForBanner(item.DominantState))
		}
		if item.DominantImpactMs > 0 {
			fields = append(fields, fmt.Sprintf("impact=%.3fms", item.DominantImpactMs))
		}
		if item.ProjectedImpactMs > 0 {
			fields = append(fields, fmt.Sprintf("projected_impact=%.3fms", item.ProjectedImpactMs))
		}
		if item.TotalMs > 0 {
			fields = append(fields, fmt.Sprintf("total=%.3fms", item.TotalMs))
		}
		if item.ProjectedTotalMs > 0 {
			fields = append(fields, fmt.Sprintf("projected_total=%.3fms", item.ProjectedTotalMs))
		}
		if item.ActualImpactMs > 0 {
			fields = append(fields, fmt.Sprintf("actual_impact=%.3fms", item.ActualImpactMs))
		}
		if item.ActualTotalMs > 0 {
			fields = append(fields, fmt.Sprintf("actual_total=%.3fms", item.ActualTotalMs))
		}
		if actualWindow := traceQueryWindowValue(item.ActualWindow.StartTs, item.ActualWindow.EndTs); actualWindow != "" {
			fields = append(fields, "actual_window="+actualWindow)
		}
		if item.TargetBlockedMs > 0 {
			fields = append(fields, fmt.Sprintf("target=%.3fms", item.TargetBlockedMs))
		}
		if item.RunningMs > 0 {
			fields = append(fields, fmt.Sprintf("running=%.3fms", item.RunningMs))
		}
		if item.RunnableMs > 0 {
			fields = append(fields, fmt.Sprintf("runnable=%.3fms", item.RunnableMs))
		}
		if item.SleepMs > 0 {
			fields = append(fields, fmt.Sprintf("sleep=%.3fms", item.SleepMs))
		}
		if item.DStateMs > 0 {
			fields = append(fields, fmt.Sprintf("d_state=%.3fms", item.DStateMs))
		}
		if item.IOWaitMs > 0 {
			fields = append(fields, fmt.Sprintf("io_wait=%.3fms", item.IOWaitMs))
		}
		if item.LineStart > 0 {
			if item.LineEnd > item.LineStart {
				fields = append(fields, fmt.Sprintf("lines=%d-%d", item.LineStart, item.LineEnd))
			} else {
				fields = append(fields, fmt.Sprintf("lines=%d", item.LineStart))
			}
		}
		parts = append(parts, strings.Join(fields, ","))
	}
	return strings.Join(parts, ";")
}

func traceQueryWriteOccurrenceRows(b *strings.Builder, label string, rank int, thread tracequery.ThreadRef, items []tracequery.WakeupCausalOccurrence) {
	if b == nil || len(items) == 0 {
		return
	}
	for _, item := range items {
		rankField := ""
		if rank > 0 {
			rankField = fmt.Sprintf(" rank=%d", rank)
		}
		projection := traceQueryProjectedActualFields(item.ProjectedImpactMs, item.ProjectedTotalMs, item.ActualImpactMs, item.ActualTotalMs, item.ActualWindow.StartTs, item.ActualWindow.EndTs)
		fmt.Fprintf(b, "  %s%s thread=%s window=%.6f..%.6f dominant_state=%s impact=%.3fms total=%.3fms target_impact=%.3fms%s fragments=%d switches=%d max_segment=%.3fms p95_segment=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms lines=%d-%d — %s\n",
			label, rankField, traceThreadLabel(thread), item.Window.StartTs, item.Window.EndTs, sanitizeForBanner(item.DominantState),
			item.DominantImpactMs, item.TotalMs, item.TargetBlockedMs, projection, item.FragmentCount, item.StateSwitches, item.MaxSegmentMs, item.P95SegmentMs,
			item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs, item.LineStart, item.LineEnd, sanitizeForBanner(item.Summary))
	}
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

	if result.FrameRootCauseBundle != nil && result.FrameRootCauseBundle.TargetResolution != nil &&
		(result.FrameRootCauseBundle.TargetResolution.Target.PID > 0 || strings.TrimSpace(result.FrameRootCauseBundle.TargetResolution.Target.Comm) != "") {
		resolution := result.FrameRootCauseBundle.TargetResolution
		lineStart, lineEnd := 0, 0
		startTs, endTs := resolution.Window.StartTs, resolution.Window.EndTs
		if resolution.SelectedFrame != nil {
			lineStart = resolution.SelectedFrame.StartLine
			lineEnd = resolution.SelectedFrame.EndLine
			if resolution.SelectedFrame.Window.StartTs > 0 {
				startTs = resolution.SelectedFrame.Window.StartTs
			}
			if resolution.SelectedFrame.Window.EndTs > resolution.SelectedFrame.Window.StartTs {
				endTs = resolution.SelectedFrame.Window.EndTs
			}
		}
		notes := traceQueryTypedKVNotes([][2]string{
			{"source", resolution.Source},
			{"window_source", resolution.WindowSource},
			{"window", traceQueryTypedTimeWindow(resolution.Window)},
			{"candidate_count", traceQueryTypedCount(len(resolution.Candidates))},
		})
		if resolution.SelectedFrame != nil {
			notes = append(notes, traceQueryTypedKVNotes([][2]string{
				{"selected_role", resolution.SelectedFrame.Role},
				{"selected_phase", resolution.SelectedFrame.Phase},
				{"selected_frame_id", resolution.SelectedFrame.FrameID},
				{"selected_name", resolution.SelectedFrame.Name},
			})...)
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#frame_target_resolution", scope),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span: types.ObservationSpan{
				LineStart: lineStart,
				LineEnd:   lineEnd,
				StartTs:   startTs,
				EndTs:     endTs,
			},
			ClaimKey:    "frame_target_resolution:" + firstNonEmptyTraceString(traceThreadLabel(resolution.Target), resolution.Source),
			Subject:     traceThreadLabel(resolution.Target),
			Predicate:   "frame_target_resolution",
			Object:      resolution.Source,
			Summary:     fmt.Sprintf("frame target resolved as %s from %s", traceThreadLabel(resolution.Target), resolution.Source),
			RichNotes:   notes,
			SupportRefs: traceQueryObservationSupportRefs(ref, lineStart, lineEnd),
			ObservedAt:  at,
			Confidence:  resolution.Confidence,
		})
	}

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
			notes := traceQueryTypedOccurrenceWindowRichNotes(item.OccurrenceWindows)
			notes = append(notes, traceQueryTypedPriorityRichNotes(rank, tier, item.Type, item.Source, item.Causality, item.ChainDepth, item.Score, item.ImpactMs, item.CumulativeImpactMs, traceQueryRootCauseEffectiveImpact(item), item.TargetImpactMs, item.ProjectedImpactMs, item.ActualImpactMs, item.ActualTotalMs, item.ActualStartTs, item.ActualEndTs)...)
			notes = append(notes, traceQueryTypedRootCauseStateRichNotes(item)...)
			notes = append(notes, traceQueryTypedKVNotes([][2]string{
				{"chain_relevance", item.ChainRelevance},
				{"overlap", traceQueryObservationMSValue(item.OverlapMs)},
				{"edge_count", traceQueryTypedCount(item.EdgeCount)},
				{"nearest_chain_thread", traceThreadLabelOptional(item.NearestChainThread)},
				{"nearest_chain_window", traceQueryTypedTimeWindow(item.NearestChainWindow)},
				{"span_name", item.SpanName},
				{"span_kind", item.SpanKind},
				{"span_category", item.SpanCategory},
				{"span_subcategory", item.SpanSubcategory},
				{"semantic_class", item.SemanticClass},
				{"perf_context", traceQueryPerfContextCompact(item.PerfContext)},
				{"perf_contexts", traceQueryPerfRoleContextsCompact(item.PerfContexts, 4)},
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
		out = append(out, traceQueryTypedSemanticTraceSpanObservations(result, *result.WindowStats, ref, scope, at)...)
	}

	return out
}

func traceQueryTypedPriorityRichNotes(rank int, tier, typ, source, causality string, chainDepth int, score, impact, cumulativeImpact, effectiveImpact, targetImpact, projectedImpact, actualImpact, actualTotal, actualStart, actualEnd float64) []string {
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
	if projectedImpact > 0 {
		notes = append(notes, fmt.Sprintf("projected_impact_ms=%.3f", projectedImpact))
	}
	if cumulativeImpact > 0 {
		notes = append(notes, fmt.Sprintf("cumulative_impact_ms=%.3f", cumulativeImpact))
		notes = append(notes, fmt.Sprintf("projected_total_ms=%.3f", cumulativeImpact))
	}
	if effectiveImpact > 0 {
		notes = append(notes, fmt.Sprintf("effective_impact_ms=%.3f", effectiveImpact))
	}
	if targetImpact > 0 {
		notes = append(notes, fmt.Sprintf("target_impact_ms=%.3f", targetImpact))
	}
	if actualImpact > 0 {
		notes = append(notes, fmt.Sprintf("actual_impact_ms=%.3f", actualImpact))
	}
	if actualTotal > 0 {
		notes = append(notes, fmt.Sprintf("actual_total_ms=%.3f", actualTotal))
	}
	if actualWindow := traceQueryWindowValue(actualStart, actualEnd); actualWindow != "" {
		notes = append(notes, "actual_window="+actualWindow)
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
	views := traceQueryCausalImpactRecommendedViews(impact)
	return traceQueryTypedKVNotes([][2]string{
		{"depth", traceQueryTypedCount(impact.ChainDepth)},
		{"causality", traceQueryCausalityLabel(impact.OnChain)},
		{"dominant_state", impact.DominantState},
		{"impact", traceQueryObservationMSValue(impact.DominantImpactMs)},
		{"projected_impact", traceQueryObservationMSValue(impact.ProjectedImpactMs)},
		{"total", traceQueryObservationMSValue(impact.TotalMs)},
		{"projected_total", traceQueryObservationMSValue(impact.ProjectedTotalMs)},
		{"actual_impact", traceQueryObservationMSValue(impact.ActualImpactMs)},
		{"actual_total", traceQueryObservationMSValue(impact.ActualTotalMs)},
		{"actual_window", traceQueryWindowValue(impact.ActualWindow.StartTs, impact.ActualWindow.EndTs)},
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
		{"actual_running", traceQueryObservationMSValue(impact.ActualRunningMs)},
		{"actual_runnable", traceQueryObservationMSValue(impact.ActualRunnableMs)},
		{"actual_sleep", traceQueryObservationMSValue(impact.ActualSleepMs)},
		{"actual_d_state", traceQueryObservationMSValue(impact.ActualDStateMs)},
		{"actual_io_wait", traceQueryObservationMSValue(impact.ActualIOWaitMs)},
		{"priority", traceQueryPriorityPair(impact.Priority, impact.PriorityClass)},
		{"target_priority", traceQueryPriorityPair(impact.TargetPriority, impact.TargetPriorityClass)},
		{"priority_relation", impact.PriorityRelation},
		{"priority_inversion_candidate", traceQueryTypedBool(impact.PriorityInversionCandidate)},
		{"recommended_views", strings.Join(views, ",")},
		{"chain_required", traceQueryTypedBool(impact.OnChain && traceQueryCausalImpactNeedsChain(impact.DominantState))},
		{"recursive", traceQueryTypedBool(impact.OnChain && traceQueryCausalImpactNeedsChain(impact.DominantState))},
		{"next_step", impact.NextStep},
	})
}

func traceQueryCausalImpactRecommendedViews(impact tracequery.WakeupCausalImpact) []string {
	switch impact.DominantState {
	case string(tracequery.StateSSleep):
		return []string{"wakeup_chain", "root_cause_rank"}
	case string(tracequery.StateRunnable):
		return []string{"scheduler_latency_stats", "root_cause_rank"}
	case string(tracequery.StateRunning):
		return []string{"trace_perf_bundle", "perf_stats", "root_cause_rank"}
	case string(tracequery.StateDSleep), string(tracequery.StateIOWait):
		return []string{"critical_blocking_calls", "window_stats", "root_cause_rank"}
	default:
		return []string{"thread_timeline", "window_stats"}
	}
}

func traceQueryCausalImpactNeedsChain(state string) bool {
	switch state {
	case string(tracequery.StateSSleep), string(tracequery.StateRunnable), string(tracequery.StateDSleep), string(tracequery.StateIOWait):
		return true
	default:
		return false
	}
}

func traceQueryTypedCausalAggregateRichNotes(aggregate tracequery.WakeupCausalAggregate) []string {
	notes := traceQueryTypedOccurrenceWindowRichNotes(aggregate.OccurrenceWindows)
	notes = append(notes, traceQueryTypedKVNotes([][2]string{
		{"depth", traceQueryTypedCount(aggregate.ChainDepth)},
		{"path", aggregate.Path},
		{"occurrences", traceQueryTypedCount(aggregate.OccurrenceCount)},
		{"dominant_state", aggregate.DominantState},
		{"impact", traceQueryObservationMSValue(aggregate.DominantImpactMs)},
		{"projected_impact", traceQueryObservationMSValue(aggregate.ProjectedImpactMs)},
		{"total", traceQueryObservationMSValue(aggregate.TotalMs)},
		{"projected_total", traceQueryObservationMSValue(aggregate.ProjectedTotalMs)},
		{"actual_impact", traceQueryObservationMSValue(aggregate.ActualImpactMs)},
		{"actual_total", traceQueryObservationMSValue(aggregate.ActualTotalMs)},
		{"actual_window", traceQueryWindowValue(aggregate.ActualFirstTs, aggregate.ActualLastTs)},
		{"target_impact", traceQueryObservationMSValue(aggregate.TargetBlockedMs)},
		{"fragments", traceQueryTypedCount(aggregate.FragmentCount)},
		{"switches", traceQueryTypedCount(aggregate.StateSwitches)},
		{"max_segment", traceQueryObservationMSValue(aggregate.MaxSegmentMs)},
		{"running", traceQueryObservationMSValue(aggregate.RunningMs)},
		{"runnable", traceQueryObservationMSValue(aggregate.RunnableMs)},
		{"sleep", traceQueryObservationMSValue(aggregate.SleepMs)},
		{"d_state", traceQueryObservationMSValue(aggregate.DStateMs)},
		{"io_wait", traceQueryObservationMSValue(aggregate.IOWaitMs)},
		{"actual_running", traceQueryObservationMSValue(aggregate.ActualRunningMs)},
		{"actual_runnable", traceQueryObservationMSValue(aggregate.ActualRunnableMs)},
		{"actual_sleep", traceQueryObservationMSValue(aggregate.ActualSleepMs)},
		{"actual_d_state", traceQueryObservationMSValue(aggregate.ActualDStateMs)},
		{"actual_io_wait", traceQueryObservationMSValue(aggregate.ActualIOWaitMs)},
		{"priority_relation", aggregate.PriorityRelation},
		{"priority_inversion_candidate", traceQueryTypedBool(aggregate.PriorityInversion)},
	})...)
	return notes
}

func traceQueryTypedOccurrenceWindowRichNotes(items []tracequery.WakeupCausalOccurrence) []string {
	if value := traceQueryOccurrenceWindowsCompact(items, 4); value != "" {
		return []string{"occurrence_windows=" + value}
	}
	return nil
}

func traceQueryTypedCriticalBlockingRichNotes(item tracequery.CriticalBlockingCandidate) []string {
	notes := traceQueryTypedKVNotes([][2]string{
		{"type", item.Type},
		{"peer", traceThreadLabel(item.Peer)},
		{"flags", item.Flags},
		{"oneway", traceQueryTypedBoolPtr(item.Oneway)},
		{"sync_like", traceQueryTypedBoolPtr(item.SyncLike)},
		{"blocking_candidate", traceQueryTypedBoolPtr(item.BlockingCandidate)},
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

	out = append(out, traceQueryTypedThreadDurationObservations(stats.TopRunning, ref, scope, at, "top_running", "running_time", "running", "selected-window running time", 0.70)...)
	out = append(out, traceQueryTypedThreadDurationObservations(stats.RunnableTop, ref, scope, at, "top_runnable", "runnable_wait", "runnable", "selected-window runnable wait", 0.75)...)
	out = append(out, traceQueryTypedThreadDurationObservations(stats.SleepTop, ref, scope, at, "top_sleep", "sleep_wait", "sleep", "selected-window sleep before wakeup", 0.76)...)
	out = append(out, traceQueryTypedThreadDurationObservations(stats.DStateTop, ref, scope, at, "top_d_state", "d_state_or_io_wait", "d_state", "selected-window D-state or IO-like wait", 0.80)...)
	out = append(out, traceQueryTypedThreadDurationObservations(stats.IOWaitTop, ref, scope, at, "top_io_wait", "io_wait", "io_wait", "selected-window IO wait", 0.82)...)

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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
		if strings.HasPrefix(strings.TrimSpace(churn.Summary), "state_cluster ") {
			appendNote("coverage_mode", "state_cluster")
		}
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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

	for i, step := range stats.StateDrilldownPlan {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if step.Thread.PID <= 0 || strings.TrimSpace(step.State) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#state_drilldown:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: step.LineStart, LineEnd: step.LineEnd, StartTs: step.StartTs, EndTs: step.EndTs},
			ClaimKey:        "state_drilldown:" + traceThreadLabel(step.Thread) + ":" + step.State,
			Subject:         traceThreadLabel(step.Thread),
			Predicate:       "state_drilldown",
			Object:          step.State,
			Value:           traceQueryObservationMSValue(step.ImpactMs),
			Unit:            "ms",
			Summary:         step.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"rank", traceQueryTypedCount(step.Rank)},
				{"state", step.State},
				{"impact", traceQueryObservationMSValue(step.ImpactMs)},
				{"total", traceQueryObservationMSValue(step.TotalMs)},
				{"source", step.Source},
				{"recommended_views", strings.Join(step.RecommendedViews, ",")},
				{"chain_required", strconv.FormatBool(step.ChainRequired)},
				{"recursive", strconv.FormatBool(step.Recursive)},
				{"window_proportion", strconv.FormatFloat(step.WindowProportion, 'f', 4, 64)},
				{"significant", strconv.FormatBool(step.Significant)},
				{"window", traceQueryWindowValue(step.StartTs, step.EndTs)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, step.LineStart, step.LineEnd),
			ObservedAt:  at,
			Confidence:  0.74,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
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
	out = append(out, traceQueryTypedInterruptObservations("ipi_activity", stats.IPIActivity, ref, scope, at)...)
	for i, accounting := range stats.SchedStatAccounting {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		if strings.TrimSpace(accounting.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#sched_stat_accounting:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: accounting.LineStart, LineEnd: accounting.LineEnd, StartTs: accounting.StartTs, EndTs: accounting.EndTs},
			ClaimKey:        "sched_stat_accounting:" + traceThreadLabel(accounting.Thread) + ":" + accounting.Kind,
			Subject:         traceThreadLabel(accounting.Thread),
			Predicate:       "sched_stat_accounting",
			Object:          accounting.Kind,
			Value:           traceQueryObservationMSValue(firstPositiveTraceFloat(accounting.TotalDelayMs, accounting.TotalRuntimeMs)),
			Unit:            "ms",
			Summary:         accounting.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"thread", traceThreadLabel(accounting.Thread)},
				{"kind", accounting.Kind},
				{"count", traceQueryTypedCount(accounting.Count)},
				{"delay", traceQueryObservationMSValue(accounting.TotalDelayMs)},
				{"max_delay", traceQueryObservationMSValue(accounting.MaxDelayMs)},
				{"runtime", traceQueryObservationMSValue(accounting.TotalRuntimeMs)},
				{"max_runtime", traceQueryObservationMSValue(accounting.MaxRuntimeMs)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, accounting.LineStart, accounting.LineEnd),
			ObservedAt:  at,
			Confidence:  0.54,
		})
	}
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
	if stats.PerfSamples != nil {
		for i, hot := range stats.PerfSamples.TopSymbols {
			if i >= traceQueryTypedFamilyRowCap {
				break
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#perf_sample_top_symbol:%d", scope, i+1),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
				SourceRef:       ref,
				Span:            types.ObservationSpan{LineStart: hot.LineStart, LineEnd: hot.LineEnd},
				ClaimKey:        "perf_sample_top_symbol:" + firstNonEmptyTraceString(hot.Symbol, hot.DSO, hot.Event),
				Subject:         firstNonEmptyTraceString(hot.Symbol, "perf_sample"),
				Predicate:       "perf_sample_top_symbol",
				Object:          hot.DSO,
				Value:           strconv.FormatInt(hot.Period, 10),
				Unit:            "sample_weight",
				Summary:         fmt.Sprintf("perf samples symbol=%s dso=%s event=%s weight_unit=%s source=%s symbolization_status=%s quality=%s sample_weight=%d samples=%d percent=%.2f%%", firstNonEmptyTraceString(hot.Symbol, "unknown"), firstNonEmptyTraceString(hot.DSO, "unknown"), firstNonEmptyTraceString(hot.Event, "unknown"), firstNonEmptyTraceString(hot.WeightUnit, "unknown"), firstNonEmptyTraceString(hot.Source, "unknown"), firstNonEmptyTraceString(hot.SymbolizationStatus, "unknown"), traceQueryPerfQualityCompact(stats.PerfSamples.Quality), hot.Period, hot.SampleCount, hot.Percent),
				RichNotes: traceQueryTypedKVNotes([][2]string{
					{"symbol", hot.Symbol},
					{"dso", hot.DSO},
					{"event", hot.Event},
					{"weight_unit", hot.WeightUnit},
					{"source", hot.Source},
					{"symbolization_status", hot.SymbolizationStatus},
					{"perf_quality", traceQueryPerfQualityCompact(stats.PerfSamples.Quality)},
					{"perf_quality_caveats", strings.Join(perfQualityCaveatsForTraceQuery(stats.PerfSamples.Quality), "; ")},
					{"sample_weight", strconv.FormatInt(hot.Period, 10)},
					{"samples", traceQueryTypedCount(hot.SampleCount)},
					{"percent", fmt.Sprintf("%.2f", hot.Percent)},
					{"threads", traceThreadLabels(hot.Threads)},
				}),
				SupportRefs: traceQueryObservationSupportRefs(ref, hot.LineStart, hot.LineEnd),
				ObservedAt:  at,
				Confidence:  0.72,
			})
		}
	}
	out = append(out, traceQueryTypedPluginObservations(stats, ref, scope, at)...)
	return out
}

func traceQueryTypedSemanticTraceSpanObservations(result tracequery.Result, stats tracequery.WindowStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if len(stats.TraceSpans) == 0 {
		return nil
	}
	chain := traceQueryResultWakeupChain(result)
	out := make([]types.ObservationRecord, 0, minInt(len(stats.TraceSpans), traceQueryTypedFamilyRowCap))
	ordinal := 0
	for _, span := range stats.TraceSpans {
		if ordinal >= traceQueryTypedFamilyRowCap {
			break
		}
		semanticClass := strings.TrimSpace(span.SemanticClass)
		if semanticClass == "" || span.DurationMs <= 0 {
			continue
		}
		ordinal++
		ctx := traceQuerySemanticTraceSpanContext(span, chain)
		notes := traceQueryTypedKVNotes([][2]string{
			{"span_name", span.Name},
			{"span_kind", firstNonEmptyTraceString(span.Kind, "sync")},
			{"span_category", span.Category},
			{"span_subcategory", span.Subcategory},
			{"semantic_class", semanticClass},
			{"chain_relevance", ctx.chainRelevance},
			{"causality", ctx.causality},
			{"chain_depth", traceQueryTypedCount(ctx.chainDepth)},
			{"overlap", traceQueryObservationMSValue(ctx.overlapMs)},
			{"window", traceQueryWindowValue(span.StartTs, span.EndTs)},
		})
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#trace_semantic_span:%d", scope, ordinal),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span: types.ObservationSpan{
				LineStart: span.StartLine,
				LineEnd:   span.EndLine,
				StartTs:   span.StartTs,
				EndTs:     span.EndTs,
			},
			ClaimKey:    "trace_semantic_span:" + semanticClass,
			Subject:     traceThreadLabel(span.Thread),
			Predicate:   "trace_semantic_span",
			Object:      semanticClass,
			Value:       traceQueryObservationMSValue(span.DurationMs),
			Unit:        "ms",
			Summary:     traceQuerySemanticTraceSpanSummary(span, ctx),
			RichNotes:   notes,
			SupportRefs: traceQueryObservationSupportRefs(ref, span.StartLine, span.EndLine),
			ObservedAt:  at,
			Confidence:  traceQuerySemanticTraceSpanConfidence(ctx.chainRelevance),
		})
	}
	return out
}

type traceQuerySemanticSpanContext struct {
	chainRelevance string
	causality      string
	chainDepth     int
	overlapMs      float64
}

func traceQueryResultWakeupChain(result tracequery.Result) *tracequery.ChainResult {
	if result.WakeupChain != nil {
		return result.WakeupChain
	}
	if result.FrameRootCauseBundle != nil && result.FrameRootCauseBundle.WakeupChain != nil {
		return result.FrameRootCauseBundle.WakeupChain
	}
	return nil
}

func traceQuerySemanticTraceSpanContext(span tracequery.TraceSpanSummary, chain *tracequery.ChainResult) traceQuerySemanticSpanContext {
	if chain == nil || (len(chain.Nodes) == 0 && len(chain.CausalImpacts) == 0 && len(chain.Edges) == 0) {
		return traceQuerySemanticSpanContext{}
	}
	var out traceQuerySemanticSpanContext
	bestOverlap := 0.0
	setOnChain := func(overlap float64, depth int) {
		if overlap <= 0 {
			return
		}
		if out.chainRelevance != "on_chain" || overlap > bestOverlap {
			out.chainRelevance = "on_chain"
			out.causality = "on_wakeup_chain"
			out.overlapMs = overlap
			out.chainDepth = depth
			bestOverlap = overlap
		}
	}
	if traceQuerySameThreadRef(span.Thread, chain.Target) {
		setOnChain(traceQueryWindowOverlapMS(span.StartTs, span.EndTs, chain.Window.StartTs, chain.Window.EndTs), 0)
	}
	for _, node := range chain.Nodes {
		if !traceQuerySameThreadRef(span.Thread, node.Thread) {
			continue
		}
		depth := 0
		if node.Impact != nil {
			depth = node.Impact.ChainDepth
		}
		setOnChain(traceQueryWindowOverlapMS(span.StartTs, span.EndTs, node.Window.StartTs, node.Window.EndTs), depth)
	}
	for _, impact := range chain.CausalImpacts {
		if !traceQuerySameThreadRef(span.Thread, impact.Thread) {
			continue
		}
		setOnChain(traceQueryWindowOverlapMS(span.StartTs, span.EndTs, impact.Window.StartTs, impact.Window.EndTs), impact.ChainDepth)
	}
	if out.chainRelevance != "" {
		return out
	}
	if traceQueryWindowOverlapMS(span.StartTs, span.EndTs, chain.Window.StartTs, chain.Window.EndTs) > 0 {
		return traceQuerySemanticSpanContext{chainRelevance: "adjacent", causality: "adjacent_to_wakeup_chain"}
	}
	return traceQuerySemanticSpanContext{chainRelevance: "background", causality: "background"}
}

func traceQuerySameThreadRef(a, b tracequery.ThreadRef) bool {
	if a.PID > 0 && b.PID > 0 {
		return a.PID == b.PID
	}
	al, bl := traceThreadLabelOptional(a), traceThreadLabelOptional(b)
	return al != "" && bl != "" && al == bl
}

func traceQueryWindowOverlapMS(aStart, aEnd, bStart, bEnd float64) float64 {
	if aEnd <= aStart || bEnd <= bStart {
		return 0
	}
	start := traceQueryMaxFloat(aStart, bStart)
	end := traceQueryMinFloat(aEnd, bEnd)
	if end <= start {
		return 0
	}
	return (end - start) * 1000
}

func traceQueryMaxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func traceQueryMinFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func traceQuerySemanticTraceSpanSummary(span tracequery.TraceSpanSummary, ctx traceQuerySemanticSpanContext) string {
	parts := []string{
		fmt.Sprintf("semantic trace span %q class=%s lasted %.3fms", span.Name, span.SemanticClass, span.DurationMs),
	}
	if ctx.chainRelevance != "" {
		parts = append(parts, "chain_relevance="+ctx.chainRelevance)
	}
	if ctx.overlapMs > 0 {
		parts = append(parts, fmt.Sprintf("overlap=%.3fms", ctx.overlapMs))
	}
	if ctx.chainDepth > 0 {
		parts = append(parts, fmt.Sprintf("chain_depth=%d", ctx.chainDepth))
	}
	return strings.Join(parts, "; ")
}

func traceQuerySemanticTraceSpanConfidence(relevance string) float64 {
	switch strings.TrimSpace(relevance) {
	case "on_chain":
		return 0.82
	case "adjacent":
		return 0.70
	case "background":
		return 0.62
	default:
		return 0.66
	}
}

func traceQueryTypedThreadDurationObservations(items []tracequery.ThreadDuration, ref types.ObservationSourceRef, scope, at, family, predicate, state, label string, confidence float64) []types.ObservationRecord {
	var out []types.ObservationRecord
	for i, td := range items {
		if i >= traceQueryTypedFamilyRowCap {
			break
		}
		thread := traceThreadLabel(td.Thread)
		if strings.TrimSpace(thread) == "" || td.DurationMs <= 0 {
			continue
		}
		notes := traceQueryTypedKVNotes([][2]string{
			{"state", state},
			{"duration", traceQueryObservationMSValue(td.DurationMs)},
			{"window", traceQueryWindowValue(td.StartTs, td.EndTs)},
			{"cpu", traceKnownCPU(td.CPU >= 0, td.CPU)},
			{"core_class", td.CoreClass},
			{"freq", traceQueryTypedCount(td.Frequency)},
			{"priority", traceQueryPriorityPair(td.Priority, td.PriorityClass)},
		})
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#%s:%d", scope, family, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: td.LineStart, LineEnd: td.LineEnd, StartTs: td.StartTs, EndTs: td.EndTs},
			ClaimKey:        predicate + ":" + thread,
			Subject:         thread,
			Predicate:       predicate,
			Object:          state,
			Value:           traceQueryObservationMSValue(td.DurationMs),
			Unit:            "ms",
			Summary:         fmt.Sprintf("%s %s %.3fms%s", thread, label, td.DurationMs, traceThreadDurationLocation(td)),
			RichNotes:       notes,
			SupportRefs:     traceQueryObservationSupportRefs(ref, td.LineStart, td.LineEnd),
			ObservedAt:      at,
			Confidence:      confidence,
		})
	}
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
				{"target_mask", item.TargetMask},
				{"target_cpus", traceIntList(item.TargetCPUs)},
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

func traceQueryTypedBoolPtr(v *bool) string {
	if v == nil {
		return ""
	}
	return strconv.FormatBool(*v)
}

func traceQueryBoolPtrBanner(v *bool) string {
	if v == nil {
		return "unknown"
	}
	return strconv.FormatBool(*v)
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

type traceQueryRequestTarget struct {
	PID    int
	Thread string
	Source string
}

const traceQueryMaxInheritedPID = 4194304

func traceQueryApplyRequestModelTarget(ctx *types.BusContext, p traceQueryParams) (traceQueryParams, string) {
	if p.PID.Int() > 0 || strings.TrimSpace(p.Thread) != "" {
		return p, ""
	}
	target, ok := traceQuerySingleRuntimeTarget(ctx)
	if !ok {
		return p, traceQueryUntypedTargetHintCaveat(ctx)
	}
	if target.PID > 0 {
		p.PID = FlexInt(target.PID)
	}
	if target.Thread != "" {
		p.Thread = target.Thread
	}
	return p, traceQueryRequestTargetCaveat(target)
}

func traceQueryRecordExplicitRuntimeTarget(ctx *types.BusContext, p traceQueryParams) {
	if ctx == nil {
		return
	}
	pid := p.PID.Int()
	thread := strings.TrimSpace(p.Thread)
	if pid <= 0 && thread == "" {
		return
	}
	target := types.RuntimeTarget{
		Kind:       types.RuntimeTargetKindProcess,
		PID:        pid,
		Thread:     thread,
		Source:     "trace_query_explicit_tool_call",
		Confidence: 1,
	}
	if thread != "" {
		target.Kind = types.RuntimeTargetKindThread
	}
	if !target.Active() {
		return
	}
	if ctx.AnalysisIR != nil {
		ctx.AnalysisIR.RequestModel.RuntimeTargets = traceQueryAppendRuntimeTarget(ctx.AnalysisIR.RequestModel.RuntimeTargets, target)
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			before := len(rm.RuntimeTargets)
			rm.RuntimeTargets = traceQueryAppendRuntimeTarget(rm.RuntimeTargets, target)
			if len(rm.RuntimeTargets) != before {
				ctx.Mutable.SetRequestModel(*rm)
			}
		}
	}
}

func traceQueryAppendRuntimeTarget(existing []types.RuntimeTarget, target types.RuntimeTarget) []types.RuntimeTarget {
	if !target.Active() {
		return existing
	}
	key := traceQueryRuntimeTargetKey(target)
	for _, current := range existing {
		if traceQueryRuntimeTargetKey(current) == key {
			return existing
		}
	}
	return append(existing, target)
}

func traceQueryRuntimeTargetKey(target types.RuntimeTarget) string {
	return fmt.Sprintf("%s:%d:%s", types.NormalizeRuntimeTargetKind(target.Kind), target.PID, strings.ToLower(strings.TrimSpace(target.Thread)))
}

func traceQuerySingleRuntimeTarget(ctx *types.BusContext) (traceQueryRequestTarget, bool) {
	if ctx == nil {
		return traceQueryRequestTarget{}, false
	}
	if ctx.AnalysisIR != nil {
		if target, ok := traceQuerySingleRuntimeTargetFromRequestModel(&ctx.AnalysisIR.RequestModel); ok {
			return target, true
		}
	}
	if ctx.Mutable != nil {
		if target, ok := traceQuerySingleRuntimeTargetFromRequestModel(ctx.Mutable.RequestModel()); ok {
			return target, true
		}
	}
	return traceQueryRequestTarget{}, false
}

func traceQuerySingleRuntimeTargetFromRequestModel(rm *types.RequestModel) (traceQueryRequestTarget, bool) {
	if rm == nil {
		return traceQueryRequestTarget{}, false
	}
	targets := map[string]traceQueryRequestTarget{}
	for _, runtimeTarget := range rm.RuntimeTargets {
		target := traceQueryRequestTarget{
			PID:    runtimeTarget.PID,
			Thread: strings.TrimSpace(runtimeTarget.Thread),
			Source: strings.TrimSpace(runtimeTarget.Source),
		}
		if target.Source == "" {
			target.Source = "request_model_runtime_targets"
		}
		if !traceQueryTypedRuntimeTargetSafe(target) {
			continue
		}
		key := fmt.Sprintf("%d\x00%s", target.PID, strings.ToLower(target.Thread))
		targets[key] = target
	}
	if len(targets) != 1 {
		return traceQueryRequestTarget{}, false
	}
	for _, target := range targets {
		return target, true
	}
	return traceQueryRequestTarget{}, false
}

func traceQueryTypedRuntimeTargetSafe(target traceQueryRequestTarget) bool {
	if target.PID < 0 || target.PID > traceQueryMaxInheritedPID {
		return false
	}
	if strings.TrimSpace(target.Thread) == "" {
		return target.PID > 0
	}
	if len(target.Thread) > 120 || strings.ContainsAny(target.Thread, "\n\r\t/\\") {
		return false
	}
	return true
}

func traceQueryRequestTargetCaveat(target traceQueryRequestTarget) string {
	parts := []string{"trace_query_target_inherited=true"}
	if target.PID > 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", target.PID))
	}
	if target.Thread != "" {
		parts = append(parts, fmt.Sprintf("thread=%q", target.Thread))
	}
	if target.Source != "" {
		parts = append(parts, "source="+target.Source)
	}
	parts = append(parts, "reason=single typed request target was preserved because the tool call omitted pid/thread")
	return strings.Join(parts, "; ")
}

func traceQueryJoinCallCaveats(items ...string) string {
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return strings.Join(out, "\n")
}

func traceQueryUntypedTargetHintCaveat(ctx *types.BusContext) string {
	if !traceQueryHasUntypedTargetHints(ctx) {
		return ""
	}
	return "trace_query_target_not_inherited=true; reason=only untyped analyzer target strings were present; pass pid/thread explicitly or provide request_model.runtime_targets for typed inheritance"
}

func traceQueryHasUntypedTargetHints(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.AnalysisIR != nil && traceQueryRequestModelHasUntypedTargetHints(&ctx.AnalysisIR.RequestModel) {
		return true
	}
	if ctx.Mutable != nil && traceQueryRequestModelHasUntypedTargetHints(ctx.Mutable.RequestModel()) {
		return true
	}
	return false
}

func traceQueryRequestModelHasUntypedTargetHints(rm *types.RequestModel) bool {
	if rm == nil {
		return false
	}
	return len(rm.AnalyzerHints.ExactTargets) > 0 ||
		len(rm.AnalyzerHints.MentionedEntities) > 0 ||
		len(rm.AnalyzerHints.PrimaryEntities) > 0 ||
		len(rm.AnalyzerHints.Entities) > 0
}
