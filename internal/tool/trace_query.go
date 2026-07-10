package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	ViaThread            string          `json:"via_thread,omitempty"`
	MinDurationMs        FlexFloat       `json:"min_duration_ms,omitempty"`
	IncludeWindowStats   *FlexBool       `json:"include_window_stats,omitempty"`
	Limit                FlexInt         `json:"limit,omitempty"`
	BucketMs             FlexFloat       `json:"bucket_ms,omitempty"`
	CoreTopology         string          `json:"core_topology,omitempty"`
	TraceFlavor          string          `json:"trace_flavor,omitempty"`
	Platform             string          `json:"platform,omitempty"`
}

// traceQueryScopedIndexMaxBytes is the in-memory byte budget for a single,
// deliberate pid/thread-scoped heavy-view index. The effective event ceiling is
// budget / traceIndexEventSizeEstimateBytes. Kept DELIBERATELY conservative
// (512 MiB → ~262K events, barely above the shared 250K default ≈ 434 MB that is
// known-safe on constrained/large-trace customer machines): the Event struct is
// ~1.7 KiB, so a bigger budget can momentarily hold >1 GiB during append growth
// and OOM/crash trace_query — which then makes the model abandon trace_query and
// fall back to grep. Real headroom for genuinely too-dense windows comes from
// the (lazy) relation-scope pruning fallback in traceQueryBuildIndex, not from
// an ever-larger unpruned index. Unscoped / non-heavy calls keep the default
// cap. Tunable in tests.
var traceQueryScopedIndexMaxBytes int64 = 512 << 20

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
	traceQueryRuntimeArtifactSelectionIDRE            = regexp.MustCompile(`(?i)^runtime_artifact:[0-9a-f]{16}$`)
)

func traceQueryMemoryForLog() (heapAlloc, heapSys uint64, gcCount uint32) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc, stats.HeapSys, stats.NumGC
}

func (t *TraceQuery) Name() string { return "trace_query" }

func (t *TraceQuery) Description() string {
	description := strings.Replace("Deterministically queries large runtime trace/log artifacts for scheduler timelines, scheduler latency stats, trace span/frame windows, frame timelines/flows, render pipelines, ranked root causes, wakeup chains, frame root-cause bundles, binder IPC graphs with explicit oneway/sync_like/blocking_candidate fields, critical blocking calls, interaction Top-N, same-window resource stats, recipes, structured event search, and line-backed evidence packs. Path inputs may be .ftrace/.trace/.systrace/.htrace/.atrace/.perftrace or .tracebundle.json; trace_query automatically promotes sibling .tracebundle.json and merges sibling .systrace+.perftrace pairs, so one path can carry joint trace+perf evidence. wakeup_chain/root_cause_rank/frame_root_cause_bundle publish structured wakeup_chain path records (one per expanded target segment; each path is a real waker chain that ends at the analyzed thread, and its branch/branches fields identify the segment), per-edge wakeup_chain_edge rows, causal_impact rows with depth/chain_branch identity, and chain_relevance fields (on_chain, adjacent, background); treat each path record as its own dependency chain - do not stitch different path records into one linear chain - and consume those ordered path/edge/relevance fields before paraphrasing dependency chains so upstream waker -> intermediate dependency -> target causality is not lost in prose and off-chain background load is not promoted to primary cause. root_cause_rank rows carry projected_impact_ms/projected_total_ms for the impact projected into the selected target/wakeup-chain window, actual_impact_ms/actual_total_ms/actual_window for the underlying scheduler state segment that may extend outside that projection, plus cumulative_impact_ms, effective_impact_ms, dominant_state, and running/runnable/sleep/d_state/io_wait totals; semantic span-work candidates add span_name/span_kind/span_category/span_subcategory/semantic_class/effective_impact_ms for system-classified runtime work such as JIT compilation, class verification, shader compilation, and runtime compilation: rows with chain_relevance=on_chain carry tier=deterministic_optimization and compete for the root cause on equal footing with other ranked rows: when such a row ranks highest, report it as the root cause named by its semantic class (for a merged row, the class word with its span count, never one member's span name), and ranked top or not, always also report it as a deterministic optimization point; rows without on-chain overlap stay background candidates and carry background_rank (their position among the non-on-chain rows), while generic trace_span rows remain supporting context. Same-thread rows of one cause family may arrive merged as a single ranked contender whose value is the family's combined magnitude: member_count carries the merged instance count, member_roster the per-member identities and values (inode/dev/span names), member_max_ms/member_min_ms the member range, member_fold_caliber the combining ruler (sum_disjoint, interval_union, max_overlap_fallback, count_sum), and member_sum_ms the raw member sum when the published value is a deduplicated lower bound; inode-keyed IO rows also expose typed inode/dev fields — report the merged row once with its combined value and name the member keys instead of re-listing members as separate causes. Use projected_* for current-window real-time projection, actual_* only to explain cross-window duration, and effective_impact_ms as a bounded ranking/hidden-cost signal rather than elapsed time. When an on_chain runnable, running/compute-supply, low-frequency, affinity/cpuset, D-state, or IO dependency is tier=primary, report it as a co-primary cause instead of moving it to background, and compare same-chain primary rows by effective_impact_ms before score; on-chain semantic span-work rows join that comparison on equal footing — a tier=deterministic_optimization row that ranks highest may be reported as the primary root cause, and every on-chain one stays a deterministic optimization point to mention with its projected share of the window; for non-semantic rows effective_impact_ms defaults to cumulative_impact_ms. Rank rows whose subject thread is the analysis target itself AND whose type is a wait-on-counterpart symptom (sleep_wait/fragmented_sleep_wait/missing_wakeup, binder_wait, blocking_span) carry tier=target_self_state: that wait/lock-hold/sleep is the symptom under analysis, so such rows carry rank=0 (no rank-board seat — rank ordinals are contiguous over the competing rows; trace_gap rows carry rank=0 the same way) and are never the primary or co-primary root cause — report them as the target's own state and take the root cause from the other ranked rows. Rows with type=trace_gap carry tier=data_gap: a data blind spot, never a cause — their trace_gap_kind field says whether the thread timeline had no intervals at all in the window (no_sched_data) or intervals that all sit below the min-duration floor (no_eligible_wait); do not report a blind spot as a ranked cause. The target's own runnable/running/IO/D-state rows are decomposable self causes, not symptoms: they compete normally (scheduling-pressure / compute-supply / IO-blocking / D-state candidates), may carry primary or co-primary tiers, and may be reported as the root cause on the target's own thread. wakeup_chain also reports aggregated_impact rows when repeated fragmented branches share a common dependency path; these rows and the corresponding root_cause_rank candidates carry bounded occurrence_windows, so enumerate the representative repeated windows and compare the aggregate against single long intervals. Treat critical_blocking_calls as direct blocking surfaces: for binder/futex/lock/sync waits, consume oneway/sync_like/blocking_candidate instead of inferring blocking semantics from raw flags, preserve peer, peer_state, chain_relevance, overlap, nearest_chain_thread, and then continue into peer thread state, wakeup_chain, root_cause_rank, and resource rows before naming the cause; if peer/on-chain evidence is missing, keep the wait as a bounded symptom/candidate with caveat. A critical_blocking row carrying absorbed_by_rank_family=true duplicates a same-thread merged rank family row of the same events (absorbed_into names that family, matching the rank row's rank_family_key): count it inside that family's combined value and cite the family row — never list absorbed rows as additional separate causes beside the family row. window_stats/event_search can filter or summarize scheduler, sched_stat accounting, binder transaction/received/lock/alloc/reply rows, CPU idle/frequency/frequency-limit, CPU affinity/cpuset/migration constraint evidence, block IO, IRQ/softirq/IPI, storage, filesystem, power, Ability/XPower/HiSystemEvent resource observations, workqueue, DMA fence, memory-like events, SmartPerf-style eBPF BIO/FileSystem/PageFault resource rows, and perf_sample CPU sampling rows when converted to text key/value fields. For perf samples, consume window_stats.perf_samples top_symbols/top_dso/top_callchains/top_threads and perf_quality/quality summaries as supporting code-execution evidence for running threads, runnable competitors, wakeup-chain dependencies, binder peers, or semantic span-work candidates; if a SQL-primary row has comm_source=trace_thread plus perf_thread_comm, thread_comm/pid/tid are the canonical trace-aligned identity and perf_thread_comm is raw converter provenance, not a separate thread. root_cause_rank candidates may carry interval/thread-filtered perf_context plus role-aware perf_contexts rows such as candidate_thread, target_running, on_chain_dependency, same_cpu_competitor, cpu_pressure_top_running, and compute_supply_cpu, and frame_root_cause_bundle may carry target_running_perf, on_chain_perf, binder_peer_perf, and same_cpu_competitor_perf role contexts. perf_quality reports source mix, sample_kind, weight_unit, symbolization_status, cpu_known/cpu_unknown, sample_cpu_scope, clock, clock_confidence, callchain_status, and caveats; sample_cpu_scope=unknown or cpu_unknown means the official/sample source did not expose sample CPU id and must not be attributed to any concrete CPU/core or used as absence proof, sample_kind=off_cpu must not be narrated as running CPU execution, unsymbolized/ip_only means raw fallback or IP/DSO-only evidence, assumed/unknown clock_confidence means trace/perf overlap is supporting evidence unless calibrated, and perf period/sample_weight values are event/sample weights rather than elapsed duration or expected sample density unless explicit sampling configuration plus calibrated CPU frequency are available. For perf evidence-quality questions, answer from sample_cpu_scope/sample_kind/weight_unit first; adjacent sched_switch CPU fields describe scheduler event rows, not the perf sample's CPU location, and should stay out of the perf hotspot conclusion unless the user explicitly asks for scheduler CPU placement. For running/compute-supply/semantic span-work causes, report perf_contexts as the code-execution support for where CPU time was spent, while scheduler overlap, chain relevance, CPU/core/frequency/affinity, D-state/IO, and supply pressure remain the causal basis. Do not treat samples alone as proof of a scheduling root cause. For runnable root causes, window_stats/root_cause_rank report runnable_context, thread_cpu_load, cpu_constraints, and secondary process_cpu_load: consume the concrete thread load, same-CPU competitors, CPU/core class, other-core idle, Harmony/Donghu sched_switch next_info affinity/restricted fields, cpuset/allowed CPU evidence, and only then the process rollup. These are output sections/candidate signals, not separate views; use view=window_stats to inspect them directly, view=root_cause_rank to let them enrich and compete with scheduler candidates, or view=frame_root_cause_bundle for frame/jank windows that need wakeup_chain + rank + blocking + IO/IRQ/IPI/workqueue/sched_stat/supply/trace-mark evidence and role-specific perf contexts in one handoff-safe result. window_stats/root_cause_rank/frame_root_cause_bundle also report inode-level IO outputs: file_io_by_inode for Android FS/F2FS/EXT4-style file read/write/sync/direct-IO rows, page_cache_by_inode for mm_filemap add/delete churn, storage_latency_by_layer for block/MMC/SCSI/F2FS/Android-FS start-done latency pairs, block_io_by_inode to join inode activity with nearest block/storage latency, io_burst_episodes for D-state/iowait/storage bursts, and io_pressure_summary to relate inode IO, page-cache churn, block/storage latency, sched_blocked_reason iowait, and D-state totals. For which-inodes-have-the-most-IO ranking or enumeration questions, read the window_stats top_io_inodes section first: it folds the whole selected window per (dev,inode) across all threads and operations before any per-section row truncation, orders groups by total event count (then bytes, then largest single-event latency), decomposes reads/writes/completions and page-cache adds/deletes, reports max_latency as the largest single event plus top_threads per-thread latency totals (latency is never summed across threads), and its trailing total-groups line discloses how many (dev,inode) groups exist beyond the listed rows. For IO completion questions, preserve file_io completions/ret/example and each storage_latency example together with bytes/len/offset and max_latency, so a single 4KB completion latency is not hidden by aggregate bytes or total latency. These are output sections/candidate signals, not separate views; use view=window_stats to inspect them directly or view=root_cause_rank/frame_root_cause_bundle to let them compete with scheduler and blocking causes. When a wakeup chain exists, treat window_stats IO/D-state/CPU-pressure rows as background context unless the corresponding root_cause_rank candidate says chain_relevance=on_chain/causality=on_wakeup_chain; aggregate rows such as cpu_pressure/io_pressure/supply_pressure remain supporting context and must not be promoted into the direct root-cause chain merely because their representative thread overlaps the chain; generic trace_span rows also stay supporting unless root_cause_rank emits a dedicated semantic span-work type. Off-chain pressure can explain system load but must not become the direct root-cause chain. window_stats/frame_root_cause_bundle also report irq_activity, softirq_activity, ipi_activity, workqueue_activity, dma_fence_activity, sched_stat_accounting, supply_pressure_summary, trace_mark_categories, and async_file_work as supporting signals; use them to explain supply-side pressure and background interference without treating them as proof unless they overlap the target window or wakeup chain. sched_stat_accounting is kernel accounting corroboration and should not replace sched_switch interval timing when both exist; ipi_activity is interrupt/reschedule pressure context, with ipi_raise counted as an instant target_mask signal unless entry/exit pairs provide active_ms. For frame/drop/jank windows with no single long sleep/runnable/D/IO/running segment, window_stats/root_cause_rank also report state_churn: frequent state switching with per-state cumulative impact, fragment count, max/p95 segment, and next-step guidance so the dominant cumulative state can still rank as the primary cause. state_churn is an output section/candidate signal, not an independent view; use view=window_stats to inspect it directly or view=root_cause_rank/frame_root_cause_bundle to let it compete with other causes. For frame/span, runnable-context, inode discovery, or perf hotspot discovery, use view=event_search with pattern as a case-insensitive literal substring, not a regex; it is best for frame ids, jank ids, span labels, trace marker labels, thread labels, next_info tokens, cpuset labels, inode tokens such as 0x478e5, entry_name values, sched_stat thread/kind fields, IPI reason/target_mask fields, perf symbols/DSOs/callchains/source/sample_kind/symbolization_status/callchain_status/clock_confidence/cpu_known, or one exact timestamp/event token before broad grep. Trace markers include B/E/C/S/F rows: event_search rows expose span_action, span_pid, span_name, and span_value; span_window/window_stats trace_spans expose kind=sync|async plus category/subcategory/semantic_class. Synchronous B/E spans end with unnamed E|<pid> or bare E on the same ftrace thread stack, async S/F spans pair by marker pid + name + cookie, and searching E|<pid>|<span_name> is not a valid end-marker test. Treat entry_name as a trace file-name label, not an absolute path; do not prefix it with /, /data/, or any directory unless that full path appears in the trace or an external mapping. If multiple span windows or zero rows come back, narrow with the returned line/time windows, a shorter literal pattern, event_types=[\"trace_mark\"], event_types=[\"perf_sample\"] for CPU sample rows, event_types=[\"cpu_constraint\"] for affinity/cpuset/next_info rows, event_types=[\"sched_stat\"] for scheduler accounting rows, event_types=[\"ipi\"] for IPI rows, event_types=[\"file_io\"] or event_types=[\"page_cache\"] for inode rows, pid/thread, or span_window before running recipe/root-cause views. Once a result reports selected_window, index_windowed, or a concrete line window, keep that same time_start/time_end or line_start/line_end on every follow-up heavy scheduler/resource/root-cause view; thread/pid alone is not enough for large traces. For big/middle/small core analysis, pass core_topology like \"small=0-3,middle=4-7,big=8-11\"; if omitted the tool only infers classes from observed CPU frequencies and reports that caveat. For very large traces, an unbounded jank recipe without time_start/time_end, line_start/line_end, span_name, pid, or thread first does light marker discovery; when timestamped top jank/frame markers are found it automatically runs bounded recipe analysis for the top candidate windows, and otherwise returns marker discovery plus next-call hints instead of expanding expensive full-trace root-cause/resource views. Trace timestamps are seconds end-to-end: 928.081774 means 928 seconds + 0.081774 seconds; with six fractional digits, the fractional part is microsecond-precision (81774 us), not a separate millisecond field. Compound timestamps such as \"1s 501ms 565μs 915ns\" are accepted and normalized to seconds. Only derived durations are rendered in ms. Trace flavor is auto-detected as harmony_hitrace, android_atrace, or generic_ftrace; set trace_flavor/platform in the typed tool call when task context requires a platform override. Raw user wording is not re-parsed by this tool for platform selection. Auto detection may report platform_candidate=mixed_harmony_base when Harmony-base trace signals coexist with Android-framework process surfaces; this uses Donghu/Harmony scheduler priority semantics, not Android priority semantics. Donghu uses Harmony/OpenHarmony trace scheduler semantics with process-isolated Android-framework and Harmony-framework surfaces; priority and timestamp semantics still follow Harmony. For HarmonyOS/hitrace user-space priority, larger numeric priority means higher priority: 1-40=CFS, 41-159=RT, >159=system_or_kernel/raw. Only ohos_rt enters high-priority pressure; raw system/kernel running and displacement overlap are reported in separate typed buckets and never compared numerically for priority inversion. Android/generic ftrace keeps raw scheduler priority and does not apply Harmony ranges. Thread selectors accept pid plus common ftrace/hitrace labels such as com.tencent.mm-36379, com.tencent.mm 36379, com.tencent.mm [36379], [GT]ColdPool#5-36624, binder:486_1-10803, or pid=36379; pass pid directly when known. Use this before ad-hoc grep/awk for ftrace/systrace/hitrace time-window causality questions; a zero-event result in a bounded window is a window/filter diagnostic, not evidence that .ftrace is unsupported. Keep grep/read_file as fallback for truly unsupported formats.", "so one path can carry joint trace+perf evidence. ", "so one path can carry joint trace+perf evidence. A .ftrace/.trace/.systrace path by itself is sufficient for core event queries, including SQL-primary perf_sample rows embedded in systrace; tracebundle is recommended context, not required input. When present, tracebundle result caveats may include tracebundle_trace_provider, tracebundle_trace_db_coverage, tracebundle_trace_coverage, and tracebundle_trace_tool_gate; use them to qualify conversion engine, SQL table coverage, trace_query cross-validation completeness, clock/perf provenance, and commercial guardrail state, not as direct runtime root causes. In tracebundle_trace_db_coverage, role=resolver_index means the DB table was consumed for joins/indexes and rows_emitted=0 is expected; role=systrace_text_output, role=perftrace_text_output, and role=query_ready_export identify text rows produced for trace_query. ", 1)
	description = strings.Replace(description, "semantic span-work candidates add span_name/span_kind/span_category/span_subcategory/semantic_class/effective_impact_ms for system-classified runtime work such as JIT compilation, class verification, shader compilation, and runtime compilation: rows with chain_relevance=on_chain carry tier=deterministic_optimization and compete for the root cause on equal footing with other ranked rows", "semantic span-work candidates add span_name/span_kind/span_category/span_subcategory/semantic_class/effective_impact_ms for system-classified runtime work such as JIT compilation, class verification, shader compilation, runtime compilation, texture upload, and explicit GC pauses: rows with chain_relevance=on_chain participate in the ordinary primary/secondary/tertiary root-cause election and must never enter the background board", 1)
	description = strings.Replace(description, "rows without on-chain overlap stay background candidates and carry background_rank", "only rows without on-chain overlap stay background candidates and carry background_rank", 1)
	description = strings.Replace(description, "on-chain semantic span-work rows join that comparison on equal footing — a tier=deterministic_optimization row that ranks highest may be reported as the primary root cause", "on-chain semantic span-work rows join that comparison through their primary/secondary/tertiary tier — a primary semantic row may be reported as the root cause", 1)
	description = strings.Replace(description, "trace_query automatically promotes sibling .tracebundle.json and merges sibling .systrace+.perftrace pairs", "trace_query automatically promotes sibling .tracebundle.json and builds a provenance-aware .systrace+.perftrace composite: every event and derived evidence range resolves to a physical source artifact, local line, and time domain; only identical time domains or an explicit calibrated finite affine clock map enter the shared causal timeline, while incompatible artifacts are isolated with a typed caveat and remain directly queryable by their .perftrace path", 1)
	description = strings.Replace(description, "state_churn is an output section/candidate signal, not an independent view; use view=window_stats to inspect it directly or view=root_cause_rank/frame_root_cause_bundle to let it compete with other causes.", "state_churn is an output section/candidate signal, not an independent view; use view=window_stats to inspect it directly or view=root_cause_rank/frame_root_cause_bundle to let it compete with other causes. The state_drilldown rows are the state-first handoff: top_sleep is a ranked Top-N cumulative sleep surface, long top_sleep rows require wakeup_chain/root_cause_rank recursive drilldown, fragmented sleep churn stays visible but non-recursive with thread_timeline/interaction_stats/window_stats follow-up, and fragmented runnable or D/IO waits remain recursive root-cause candidates. Preserve state_drilldown source, recommended_views, chain_required, and recursive flags instead of guessing from prose. Each state_drilldown row also carries window_proportion (fraction 0..1 of the selected window that state consumed) and a significant flag: the top-ranked state is always significant, and lower-ranked states are significant only when they clear the proportion floor; rows with significant=false are kept for coverage completeness but are too small to be worth their own per-layer root-cause drilldown, so prioritize significant=true states for per-layer root-cause analysis.", 1)
	description = strings.Replace(description, "Once a result reports selected_window, index_windowed, or a concrete line window, keep that same time_start/time_end or line_start/line_end on every follow-up heavy scheduler/resource/root-cause view; thread/pid alone is not enough for large traces.", "Once a result reports selected_window, index_windowed, or a concrete line window, keep that same time_start/time_end or line_start/line_end on every follow-up heavy scheduler/resource/root-cause view; thread/pid alone is not enough for large traces. If a call supplies both a frame/span selector and explicit time_start/time_end, frame_root_cause_bundle preserves the explicit query window and unions it with the frame-derived previous-frame-end..current-frame-end window instead of shrinking to an interior vsync/frame marker; span_window/span_name does the same for a uniquely-matched named span, unioning the explicit window with the matched span's own start/end instead of narrowing to whichever is smaller. For jank/stall root-cause analysis over a broader typed period, prefer frame/span-derived windows or coverage windows around 80-150ms for recipe/root_cause_rank/frame_root_cause_bundle before shrinking further; sub-50ms windows are micro-probes and must not be treated as representative unless the selected frame/span itself is that short. If the task's typed target is a process id, thread id, or thread label, set pid/thread explicitly in the tool call and keep that typed filter on follow-up trace_query calls unless deliberately inspecting a named peer; if omitted and the structured request model exposes exactly one runtime_targets entry, trace_query inherits only that typed pid/thread and reports trace_query_target_inherited, but trace_query does not infer omitted pid/thread values from raw request prose, analyzer entity strings, objective text, or prior summaries. For long transaction/lifecycle windows, preserve the full typed time window as parent coverage; use event_search/span_window/frame_window to discover phase boundaries, then drill into the heaviest phase windows. If a result reports mode=index_event_limit or selected window too dense, do not retry the same parameters; for local jank/stall root-cause views split toward 80-150ms coverage windows first, add line_start/line_end, or use event_search/span_window/event_types to narrow before rerunning the heavy view; shrink below 50ms only as a local micro-probe with a caveat.", 1)
	return description
}

func (t *TraceQuery) Parameters() json.RawMessage {
	schema := `{
  "type": "object",
  "properties": {
	    "source": {"type":"string","enum":["path","attached_trace"],"x-codrax-enum-style-alias":true,"description":"Use attached_trace for the current --htrace/--atrace blob; use path for an explicit workspace/repo file."},
	    "path": {"type":"string","description":"Repo/workspace-relative or absolute trace/log path when source=path. Use the typed artifact item's source value, not its runtime_artifact:<id>. For compatibility, a copied logical id is auto-resolved only when it names a current typed trace item that maps to exactly one physical artifact; the result reports the repair and canonical next-call form. Accepts ftrace-compatible text such as .ftrace/.trace/.systrace/.htrace/.atrace, text .perftrace, and .tracebundle.json. A converted .systrace or raw .ftrace text is sufficient for core event queries and may already contain SQL-primary perf_sample rows; .tracebundle.json adds provider/coverage/clock/caveat provenance. When a sibling .tracebundle.json exists, or a sibling .systrace/.perftrace pair exists, trace_query builds a provenance-aware composite index. Same-domain artifacts merge directly; different domains merge only through an explicit calibrated finite affine map, otherwise the incompatible artifact is isolated and disclosed. Pass the .perftrace path explicitly to query an isolated perf clock on its own."},
	    "trace_flavor": {"type":"string","enum":["auto","harmony_hitrace","android_atrace","generic_ftrace"],"x-codrax-enum-style-alias":true,"description":"Optional producer/platform flavor. Defaults to auto detection. Use harmony_hitrace for HarmonyOS HiTrace priority semantics: 1-40=CFS, 41-159=RT, >159=system_or_kernel/raw; only ohos_rt enters high-priority pressure and raw system/kernel tokens remain a separate typed bucket. Use android_atrace for Android/Linux atrace raw scheduler priorities, and generic_ftrace when uncertain."},
	    "platform": {"type":"string","enum":["auto","donghu","harmony","harmony_hitrace","android","android_atrace","generic","generic_ftrace"],"x-codrax-enum-style-alias":true,"description":"Optional typed platform hint. Use donghu when the typed task/tool call selects Donghu: scheduler/time/priority semantics follow Harmony/OpenHarmony, while Android-framework and Harmony-framework processes may coexist at process boundaries. harmony/harmony_hitrace selects Harmony semantics; android/android_atrace selects Android raw scheduler priority semantics."},
		    "view": {"type":"string","enum":["event_search","window_sweep","span_window","frame_window","render_pipeline","frame_timeline","frame_flow","thread_timeline","window_stats","perf_stats","perf_timeline","trace_perf_bundle","scheduler_latency_stats","ipc_graph","wakeup_chain","root_cause_rank","frame_root_cause_bundle","critical_blocking_calls","interaction_stats","recipe","evidence_pack"],"x-codrax-enum-style-alias":true,"x-codrax-enum-aliases":{"state_churn":"window_stats","cpu_samples":"perf_stats","cpu_sample_stats":"perf_stats","sample_timeline":"perf_timeline","perf_sample_timeline":"perf_timeline","perf_bundle":"trace_perf_bundle","trace_perf":"trace_perf_bundle","trace_plus_perf":"trace_perf_bundle","causal_impact":"wakeup_chain","frame_bundle":"frame_root_cause_bundle","frame_rootcause_bundle":"frame_root_cause_bundle","frame_root_cause":"frame_root_cause_bundle"},"description":"The deterministic trace view to compute. Use window_sweep for a second-scale or longer dense window before heavy views: it is a streaming per-bucket coverage scan (default bucket_ms=100, clamped 50..500) that is NOT subject to the index event budget, counts sched_switch/sched_wakeup/D-state-entry/irq-entry/trace_mark rows per bucket plus target-pid sched_switch participation when pid is set, and returns advisory top-K dense sub-windows with suggested follow-up views plus a compact coverage table (folded to at most 40 rows), so drill-down windows are picked from measured density instead of blind bisection. Use span_window to turn a unique trace span into a time window: synchronous B/E spans close with unnamed E|<pid> or bare E on the same ftrace thread stack, and async S/F spans close by marker pid + name + cookie. Do not search for E|<pid>|<span_name> as an end marker. Use frame_window/render_pipeline for Choreographer/RenderFrame/VSYNC/draw/present spans; frame_timeline/frame_flow for Expected/Actual/Jank/GPU/RS/UI phase summaries and cross-thread frame flows; perf_stats for same-window CPU sample top_symbols/top_dso/top_callchains/top_threads, perf_timeline for bucketed sample weight over time, and trace_perf_bundle for a handoff-safe bundle that combines window/root-cause/wakeup evidence with perf sample context; scheduler_latency_stats for runnable wait p95/p99/max and CPU competition; wakeup_chain for wakeup edges and causal_impacts per chain node plus aggregated_impacts with bounded occurrence_windows when repeated fragmented branches share a common dependency path; critical_blocking_calls for futex/lock/sync/binder/IO/D-state candidates, with peer_state breakdown when the peer thread timeline is visible; root_cause_rank for primary/secondary/tertiary cause candidates (rows whose subject is the analysis target itself with a wait-on-counterpart type (sleep/binder wait/lock hold) instead carry tier=target_self_state — the target's own symptom, never the root cause; the target's own runnable/running/IO/D-state rows compete normally as decomposable self causes), including projected_impact_ms/projected_total_ms for selected-window projection, actual_impact_ms/actual_total_ms/actual_window for full scheduler-state duration, cumulative_impact_ms, effective_impact_ms, dominant_state/running/runnable/sleep/d_state/io_wait totals, occurrence_windows for aggregate common dependency paths, candidate-level perf_context plus role-aware perf_contexts such as candidate_thread, target_running, on_chain_dependency, same_cpu_competitor, cpu_pressure_top_running, and compute_supply_cpu, fragmented state_churn candidates when frequent short state switches cumulatively dominate, wakeup_chain causal_impacts and aggregated_impacts when repeated fragmented branches share a common dependency path, semantic span-work candidates for JIT/class verification/shader/runtime compilation hidden cost (tier=deterministic_optimization when on-chain, background_rank position when not), and co-primary on-chain runnable/running/compute-supply/D-state/IO dependencies when they are part of the same causal chain; same-chain primary root_cause_rank rows are ordered by effective_impact_ms before score, and non-semantic rows default effective_impact_ms to cumulative_impact_ms; frame_root_cause_bundle returns wakeup_chain + frame_timeline + root_cause_rank + critical_blocking_calls plus IO/IRQ/workqueue/supply/trace-mark bundle fields and role-specific perf contexts target_running_perf/on_chain_perf/binder_peer_perf/same_cpu_competitor_perf for frame/jank handoff; state_churn and causal_impacts are output sections, not standalone views; view=state_churn is accepted and treated as view=window_stats, view=causal_impact is accepted as wakeup_chain, view=perf_bundle/trace_perf/trace_plus_perf is accepted as trace_perf_bundle, and view=frame_bundle/frame_rootcause_bundle is accepted as frame_root_cause_bundle; interaction_stats for target-thread wakeup/binder interaction Top-N; recipe for standard evidence packs; and ipc_graph for binder transaction send/receive causality with explicit oneway/sync_like/blocking_candidate fields."},
	    "thread": {"type":"string","description":"Thread name, substring, or ftrace/hitrace task label to resolve when pid is unknown. Accepts forms like \"com.tencent.mm-36379\", \"com.tencent.mm 36379\", \"com.tencent.mm [36379]\", \"[GT]ColdPool#5-36624\", \"binder:486_1-10803\", or \"pid=36379\"; pid is preferred when known."},
    "pid": {"type":"integer","description":"Thread pid to analyze when known. Do not combine pid/thread with event_search CPU-global state families (cpu_frequency, cpu_frequency_limits, cpu_idle, clock_set_rate): those rows are owned by CPUs, while the row-header task is only the incidental emitter. Remove the selector and keep the time/line window for those searches."},
    "time_start": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window start in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\", \"928.081774 秒\", or compound forms like \"1s 501ms 565μs 915ns\" and normalizes them to seconds; six fractional digits are microsecond precision."},
    "time_end": {"oneOf":[{"type":"number"},{"type":"string"}],"description":"Trace timestamp window end in seconds. Prefer a JSON number. Also accepts strings such as \"928.081774s\", \"928.081774 秒\", or compound forms like \"3s 116ms\" and normalizes them to seconds; six fractional digits are microsecond precision."},
	    "line_start": {"type":"integer","description":"Optional result line window start for bounded search. On a composite trace this is the index-global virtual line returned by trace_query; trace_artifacts/source_spans provide the physical artifact and local line."},
	    "line_end": {"type":"integer","description":"Optional result line window end for bounded search. On a composite trace this is the index-global virtual line returned by trace_query; trace_artifacts/source_spans provide the physical artifact and local line."},
	    "event_types": {"type":"array","items":{"type":"string"},"x-codrax-split-string-array":true,"description":"Optional event filters such as trace_mark, sched_switch, sched_wakeup, sched_blocked_reason, sched_stat, cpu_idle, cpu_frequency, cpu_frequency_limits, cpu_constraint, clock_set_rate, block_rq_issue, block_rq_complete, block_bio_remap, binder_transaction, binder_transaction_received, binder_transaction_alloc_buf, binder_lock, binder_locked, binder_unlock, binder_reply, irq, softirq, ipi, storage, filesystem, file_io, page_cache, android_fs, f2fs, scsi, mmc, storage_latency, io_pressure, perf_sample, power, ability_monitor, xpower, hi_sysevent, workqueue, dma_fence. Official formatter aliases such as sched_wakeup_new, sched_stat_wait, sched_stat_sleep, sched_stat_iowait, sched_stat_blocked, sched_stat_runtime, ipi_raise, ipi_entry, ipi_exit, block_rq_insert, block_getrq, block_bio_queue, block_bio_complete, print, tracing_mark_write_xacct, and xacct_tracing_mark_write are accepted and mapped to the matching structured event type. Use trace_mark for B/E/C/S/F marker rows; B/E end rows are unnamed E|<pid> or E, so use span_window rather than E|<pid>|<span_name> searches to prove completion. Use sched_stat/sched_stat_accounting as kernel accounting corroboration for wait/iowait/blocked/runtime, not as a replacement for sched_switch interval timing when both exist. Use ipi/ipi_activity as interrupt/scheduler-reschedule pressure context; ipi_raise target_mask is an instant signal unless paired ipi_entry/exit gives active_ms. Use perf_sample with pattern=<symbol, dso, callchain, event, thread, source, symbolization_status, callchain_status, clock_confidence, or cpu_known> for CPU sampling rows; window_stats.perf_samples summarizes top_symbols/top_dso/top_callchains/top_threads plus perf_quality as supporting execution context, not standalone root-cause proof. Raw fallback rows may have source=raw_perfdata_fallback, symbolization_status=unsymbolized, and callchain_status=ip_only; OpenHarmony hiperf proto rows may have cpu_known=false because sample CPU is unavailable. Result caveats may also carry tracebundle perf/profiler/trace conversion quality provenance such as lost_records/lost_events, lost_sample_records/lost_samples, throttle_records/unthrottle_records, aux_records/aux_bytes, ftrace-plugin structured metadata, profiler plugin metadata, dropped_events, overrun, commit_overrun, overwrite, trace_clock, clock_details, symbol_examples, tracebundle_perf_capability, tracebundle_perf_clock_alignment, tracebundle_trace_provider, tracebundle_trace_db_coverage, tracebundle_trace_coverage, and tracebundle_trace_tool_gate; use them to qualify sample/capture/conversion reliability, coverage, and converter guardrail state, not as direct runtime root causes. Use cpu_constraint/affinity/cpuset to inspect sched_setaffinity, sched_migrate_task, cpuset/cgroup attach, and Harmony/Donghu sched_switch next_info affinity/restricted evidence. Use file_io/page_cache with pattern=<inode or entry_name> for inode-level IO rows. This field also accepts a comma/semicolon separated string, and friendly aliases such as inode_io, pageCache, mm_filemap, cpuSample, perfSamples, topSymbols, callchain, cpuAffinity, schedMigrate, storageLayerLatency, irq_activity, softirq_activity, ipi_activity, sched_stat_accounting, and block_io_by_inode are accepted and mapped to the matching event types."},
    "pattern": {"type":"string","description":"For event_search, optional case-insensitive literal substring matched against parsed event text, span names, thread labels, scheduler roles, resource fields, and raw-like field text. Use this for frame ids such as \"1917295\", jank ids such as \"jank_frames=7\", exact timestamps, or trace labels such as \"Choreographer#doFrame\"; it is not a regex. Start with one exact token, then add event_types/time/line/thread filters after the first hit."},
    "span_name": {"type":"string","description":"Optional trace span name substring. For span_window, returns matching sync B/E or async S/F span windows; sync B/E end rows do not repeat the span name and appear as E|<pid> or bare E on the same ftrace thread stack. For wakeup_chain/root_cause_rank/evidence_pack without explicit time_start/time_end, a unique matching span derives the selected window."},
    "interaction_direction": {"type":"string","enum":["both","incoming","outgoing"],"x-codrax-enum-style-alias":true,"description":"For interaction_stats: both is default; incoming counts peers waking/calling the target, outgoing counts target waking/calling peers."},
    "recipe_name": {"type":"string","enum":["auto","sleep_root_cause","jank","runnable_delay","binder_wait","io_wait","cpu_supply","span_locate"],"x-codrax-enum-style-alias":true,"description":"For view=recipe: choose a standard deterministic evidence pack. auto picks from span_name/event_types/question-shape hints; recipes remain advisory and line-backed. span_locate turns a span label (span_name or bare pattern, no event_types needed) into its start/end time and line window in one call: a bare-pattern locate step followed by span_window resolution - use it before heavy views when the span's window is unknown."},
    "max_depth": {"type":"integer","description":"wakeup_chain recursion limit; default 10."},
    "max_branches": {"type":"integer","description":"Maximum branches to report; default 8."},
    "via_thread": {"type":"string","description":"For view=wakeup_chain: optional thread selector (same forms as thread: pid, \"comm-pid\", or a full thread name; matched exactly, never by substring). Target-thread segments whose wakeup subtree contains this thread are expanded even when max_branches would drop them, and the result reports a via_thread verdict: ON a wakeup path to the target with depth and per-hop wakeup latency, or NOT connected by any wakeup edge in this window, meaning its influence is scheduling contention (runnable queuing) rather than a wakeup dependency. Use it to test whether a runnable anchor thread sits on the user-focus thread's wakeup chain."},
    "min_duration_ms": {"type":"number","description":"Ignore intervals shorter than this; default 1ms."},
    "include_window_stats": {"type":"boolean","description":"For wakeup_chain, include same-window CPU/IO/binder/irq stats; default true."},
    "core_topology": {"type":"string","description":"Optional CPU core class map for compute-supply evaluation, e.g. \"small=0-3,middle=4-7,big=8-11\" or \"little=0-3,big=4-7\". If omitted, classes are inferred from observed CPU frequency tiers when possible. On devices where each CPU cluster shares one frequency point, frequency-weighted results reuse a same-cluster sampled core's cpu_frequency timeline for cluster members without their own samples (each reuse is disclosed per row/caveat together with its membership source): an explicit map is the authoritative membership, and in its absence clusters are derived from identical cpu_frequency change-point timelines with downward core-number inheritance only (cores above the highest sampled core are never extrapolated), so pass the real topology whenever known — it always overrides the derivation."},
    "limit": {"type":"integer","description":"event_search inline row cap; default 40. For view=window_sweep this is the hotspot top-K; default 8."},
    "bucket_ms": {"type":"number","description":"For view=window_sweep only: coverage bucket width in milliseconds. Default 100; values are clamped to 50..500. Accepts integers, floats, or duration strings such as \"100ms\"."}
  }
}`
	schema = strings.Replace(schema,
		"semantic span-work candidates for JIT/class verification/shader/runtime compilation hidden cost (tier=deterministic_optimization when on-chain, background_rank position when not)",
		"semantic span-work candidates for JIT/class verification/shader/runtime compilation, texture upload, and explicit GC pauses (ordinary primary/secondary/tertiary election when on-chain; background_rank only when off-chain)", 1)
	return json.RawMessage(schema)
}

func (t *TraceQuery) Execute(ctx *types.BusContext, params json.RawMessage) (out types.ToolResult, executeErr error) {
	var sourceAdaptation *traceQuerySourceAdaptation
	defer func() {
		traceQueryAnnotateSourceAdaptation(&out, sourceAdaptation)
	}()

	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())
	var p traceQueryParams
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return failStrictDecodeWithError(t.Name(), time.Now(), err, nil, params)
	}
	if globalTypes := traceQueryCPUGlobalEventSearchTypes(p); len(globalTypes) > 0 && (p.PID.Int() > 0 || strings.TrimSpace(p.Thread) != "") {
		return types.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Summary: fmt.Sprintf(
				"trace_query rejected pid/thread with CPU-global event_search types [%s]: a thread selector filters the task that emitted the row, not ownership of the CPU state, and can manufacture matched_events=0. Remove pid/thread and keep the time/line window plus event_types; split mixed thread-scoped and CPU-global event families into separate calls.",
				strings.Join(globalTypes, ","),
			),
			Timestamp: time.Now(),
		}, nil
	}
	traceQueryRecordExplicitRuntimeTarget(ctx, p)
	var targetCaveat string
	p, targetCaveat = traceQueryApplyRequestModelTarget(ctx, p)
	var sourceReject *types.ToolResult
	p, sourceAdaptation, sourceReject = traceQueryAdaptLogicalArtifactPath(ctx, p)
	if sourceReject != nil {
		return *sourceReject, nil
	}
	path, sourceLabel, reject := resolveTraceQuerySource(ctx, p)
	if reject != nil {
		return *reject, nil
	}
	window := normalizedTraceQueryWindow(p)
	callCaveat := traceQueryJoinCallCaveats(window.NormalizationCaveat, targetCaveat)
	if auto, ok := t.maybeLargeRecipeAutoWindow(ctx, p, path, sourceLabel, callCaveat); ok {
		return auto, nil
	}
	if narrowed, ok := t.maybeLargePatternWindowedView(ctx, p, path, sourceLabel, callCaveat); ok {
		return narrowed, nil
	}
	if discovery, ok := t.maybeLargeRecipeDiscovery(ctx, p, path, sourceLabel); ok {
		return discovery, nil
	}
	if guard, ok := t.maybeLargeTraceHeavyViewGuard(ctx, p, path, sourceLabel); ok {
		return guard, nil
	}
	lookupCaveat := traceQueryJoinCallCaveats(callCaveat, window.LookupCaveat)
	if streamed, ok := t.maybeStreamEventSearch(ctx, p, path, sourceLabel, window, lookupCaveat); ok {
		return streamed, nil
	}
	if sweep, ok := t.maybeStreamWindowSweep(ctx, p, path, sourceLabel, window.RequestedStart, window.RequestedEnd, callCaveat); ok {
		return sweep, nil
	}
	buildStart := time.Now()
	logging.Debug("[trace_query] phase=build_index view=%s source=%s path=%s start time_start=%.6f time_end=%.6f line_start=%d line_end=%d",
		p.View, sourceLabel, path, window.RequestedStart, window.RequestedEnd, p.LineStart.Int(), p.LineEnd.Int())
	idx, err := traceQueryBuildIndex(contextFromBus(ctx), path, p, window.RequestedStart, window.RequestedEnd)
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
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, window.RequestedStart, window.RequestedEnd)
	runStart := time.Now()
	logging.Debug("[trace_query] phase=run_view view=%s path=%s start events=%d windowed=%v", q.View, path, len(idx.Events), idx.Windowed)
	result := tracequery.Run(idx, q)
	heapAlloc, heapSys, gcCount = traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=run_view view=%s path=%s done elapsed=%s evidence=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d", q.View, path, time.Since(runStart), len(result.EvidencePack), len(result.Caveats), heapAlloc, heapSys, gcCount)
	traceQueryAppendCallCaveats(&result, callCaveat)
	result.Caveats = append(result.Caveats, traceQueryObjectiveExactTokenCaveats(ctx, p, result)...)
	// RN-14b (§7.9): runnable-dominant anchor result + pinned user-focus
	// thread → soft next-step hint that reconnects the anchor to the focus
	// thread's wakeup chain (via_thread, RN-14a).
	if hint := traceQueryRunnableAnchorRecoveryHint(ctx, result); hint != "" {
		result.Caveats = append(result.Caveats, hint)
	}
	storeStart := time.Now()
	payload, marshalFailure := traceQueryMarshalPayload(t.Name(), result)
	if marshalFailure != nil {
		return *marshalFailure, nil
	}
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

// traceQueryChainTargetRunnableDominant reports whether the chain target's
// own (depth-0) causal-impact state totals are runnable-dominant — the typed
// dominant_state==runnable condition of RN-14b (§7.9). Sums are the same
// per-state totals dominantCausalImpactState reads; runnable must strictly
// exceed every other state.
func traceQueryChainTargetRunnableDominant(chain *tracequery.ChainResult) bool {
	if chain == nil || chain.Target.PID <= 0 {
		return false
	}
	var running, runnable, sleep, dState, ioWait float64
	found := false
	for _, impact := range chain.CausalImpacts {
		if impact.ChainDepth != 0 || impact.Thread.PID != chain.Target.PID {
			continue
		}
		found = true
		running += impact.RunningMs
		runnable += impact.RunnableMs
		sleep += impact.SleepMs
		dState += impact.DStateMs
		ioWait += impact.IOWaitMs
	}
	if !found || runnable <= 0 {
		return false
	}
	return runnable > running && runnable > sleep && runnable > dState && runnable > ioWait
}

// traceQueryRunnableAnchorRecoveryHint is the RN-14b (§7.9) chain-recovery
// hint: when a wakeup_chain / root_cause_rank result's target is
// runnable-dominant (typed depth-0 state totals), an analyzer-pinned
// user-focus thread exists (typed RuntimeTargets lane, H4 channel), and the
// focus differs from the queried target (pid integer comparison), the result
// tail suggests connecting the runnable anchor to the user-focus thread's
// wakeup chain via the RN-14a via_thread parameter. Soft guidance only —
// nothing is gated on it, and any missing/ambiguous input keeps it silent.
func traceQueryRunnableAnchorRecoveryHint(ctx *types.BusContext, result tracequery.Result) string {
	switch tracequery.CanonicalViewName(result.View) {
	case "wakeup_chain", "root_cause_rank":
	default:
		return ""
	}
	if result.WakeupChain == nil || !traceQueryChainTargetRunnableDominant(result.WakeupChain) {
		return ""
	}
	focusPID, ok := analyzerPinnedFocusThreadPID(ctx)
	if !ok {
		return ""
	}
	targetPID := result.WakeupChain.Target.PID
	if targetPID <= 0 || focusPID == targetPID {
		return ""
	}
	return fmt.Sprintf("next: view=wakeup_chain pid=%d via_thread=%d — connect the runnable anchor to the user-focus thread's wakeup chain", focusPID, targetPID)
}

func traceQueryAppendCallCaveats(result *tracequery.Result, timeCaveat string) {
	if result == nil {
		return
	}
	if timeCaveat != "" {
		result.Caveats = append(result.Caveats, timeCaveat)
	}
}

// traceQueryMarshalPayload fail-closes every trace_query publication before a
// blob is created. encoding/json rejects NaN and infinities; propagating that
// failure as a typed tool result prevents an empty payload reference from
// masquerading as a successful deterministic query.
func traceQueryMarshalPayload(toolName string, value any) ([]byte, *types.ToolResult) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		return payload, nil
	}
	return nil, &types.ToolResult{
		ToolName:  toolName,
		Success:   false,
		Summary:   fmt.Sprintf("trace_query failed to serialize result: %v", err),
		Timestamp: time.Now(),
	}
}

func (t *TraceQuery) traceQueryIndexLimitResult(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string, err error) (types.ToolResult, bool) {
	var limitErr *tracequery.IndexEventLimitError
	if !errors.As(err, &limitErr) {
		return types.ToolResult{}, false
	}
	summary := traceQueryIndexLimitSummary(path, sourceLabel, p, limitErr)
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, p.TimeStart.Seconds(), p.TimeEnd.Seconds())
	if cluster, clusterErr := tracequery.StreamStateCluster(contextFromBus(ctx), path, q, tracequery.StreamStateClusterDefaultMax); clusterErr == nil && cluster.WindowStats != nil {
		cluster.Caveats = append([]string{
			fmt.Sprintf("index_event_limit_fallback=true; original_view=%s parsed_events=%d max_events=%d%s",
				sanitizeForBanner(firstNonEmptyTraceString(p.View, "window_stats")), limitErr.Events, limitErr.MaxEvents,
				sanitizeForBanner(limitErr.RecoveryParams())),
		}, cluster.Caveats...)
		payload, marshalFailure := traceQueryMarshalPayload(t.Name(), cluster)
		if marshalFailure != nil {
			return *marshalFailure, true
		}
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
	// CMP-8/CMP-10 (§7.1/§7.4): the runnable branch leads with the occupancy
	// side — top occupiers + compute-supply balance answer "who ate the CPU /
	// was capacity actually short" before the wait-side views.
	b.WriteString("state_first_hint=before shrinking into arbitrary micro-windows, use the stream_state_cluster/window_stats rows below to identify the target thread's dominant and secondary states, then drill down by state family: sleep->wakeup_chain, runnable->window_stats occupancy first (cpu_occupancy top occupiers + compute_supply_balance), then scheduler_latency/root_cause_rank with same CPU competitors, running->perf/compute-supply/semantic span work, D-state/IO->critical_blocking/window_stats IO resources.\n")
	// audit #55: when the failed request window ALREADY sits at or below the
	// preferred coverage band, "split toward 80-150ms" is self-referential —
	// the window that just hit the budget IS that size, so time splitting is
	// not the lever. Precise signal: the typed request-window duration only.
	// Non-time-splitting levers (pid/thread, event_types, line windows) take
	// the sentence instead; unset/longer windows keep the historical text.
	if duration := traceQueryParamWindowDurationSeconds(p); duration > 0 && duration <= traceQueryPreferredCoverageWindowMaxSeconds {
		fmt.Fprintf(&b, "next_call_hint=do not retry the same heavy view with the same dense scope. The requested window already spans %.0fms — at or below the preferred %.0f-%.0fms coverage band — so splitting the time window further is not the lever here: narrow by pid=<target pid> or thread, split by one event type family per call via event_types, or add line_start/line_end from a prior event_search/span_window result before rerunning the heavy view. Shrink below %.0fms only as a local micro-probe and do not extrapolate it to the broader requested period.\n",
			duration*1000,
			traceQueryPreferredCoverageWindowMinSeconds*1000,
			traceQueryPreferredCoverageWindowMaxSeconds*1000,
			traceQueryMicroWindowProbeSeconds*1000)
	} else {
		fmt.Fprintf(&b, "next_call_hint=do not retry the same heavy view with the same dense scope. Split toward %.0f-%.0fms coverage windows for jank/stall root-cause views, add line_start/line_end from a prior event_search/span_window result, or first run event_search with exact timestamp/span/event_types filters to locate a tighter line window. Shrink below %.0fms only as a local micro-probe and do not extrapolate it to the broader requested period.\n",
			traceQueryPreferredCoverageWindowMinSeconds*1000,
			traceQueryPreferredCoverageWindowMaxSeconds*1000,
			traceQueryMicroWindowProbeSeconds*1000)
	}
	// §4.7 W3: when the caller's explicit request window spans strictly more
	// than WindowSweepRecoveryMinWindowSeconds, the FIRST recovery
	// recommendation is window_sweep — one streaming coverage pass over the
	// same window replaces blind bisection into repeated denials. Precise
	// trigger signal: the typed request window duration only (never the
	// padded index window). The view name is the shared capacity-table token
	// so this surface cannot drift from the engine's recovery sentence.
	if duration := traceQueryParamWindowDurationSeconds(p); duration > tracequery.WindowSweepRecoveryMinWindowSeconds {
		fmt.Fprintf(&b, "window_sweep_first=requested window spans %.3fs; run trace_query(view=%q) FIRST with the SAME time_start/time_end (a streaming per-bucket coverage scan NOT subject to this index event budget) to rank dense sub-windows, then drill into its suggested sub-windows with heavy views.\n",
			duration, tracequery.ViewWindowSweep)
	}
	// C3 (§7.30.2): concrete, copy-pastable recovery parameters — the streaming
	// event_search escape hatch plus the exact window segment this index already
	// covered before hitting the budget, so the model can rerun immediately
	// instead of guessing a narrower scope. The escape-hatch view name is
	// interpolated from the capacity table's shared token (rendered text stays
	// byte-identical) so this surface cannot drift from the engine's.
	fmt.Fprintf(&b, "recovery_params=view=%s runs as a streaming scan and is NOT subject to this index event budget; use it (with pattern/event_types filters) to locate exact tokens and line windows first.", tracequery.FallbackViewEventSearch)
	if limitErr.LastTs > limitErr.FirstTs && limitErr.FirstTs > 0 {
		fmt.Fprintf(&b, " Or rerun view=%q with time_start=%.6f time_end=%.6f — the first window segment this index already covered before hitting the budget.",
			sanitizeForBanner(view), limitErr.FirstTs, limitErr.LastTs)
	}
	if p.PID.Int() <= 0 && strings.TrimSpace(p.Thread) == "" {
		b.WriteString(" Or add pid=<target pid> to scope the index.")
	}
	b.WriteString("\n")
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
	// §4.7 W3: the typed refinement channel must give the same first move as
	// the prose denial banner. Identical precise signal as the banner's
	// window_sweep_first sentence — an explicit request window STRICTLY longer
	// than WindowSweepRecoveryMinWindowSeconds — steers PreferredParams to
	// view=window_sweep over the SAME window. window_sweep consumes neither
	// pattern/event_types/span_name filters nor the state-cluster
	// parent-coverage step, and its whole point is "sweep first, do not narrow
	// yet", so the event_search fallback rewrite, the
	// state_cluster_first/parent_coverage extras, and the narrowing
	// suggestions are not attached on this branch. Shorter or unset windows
	// keep the historical event_search-shaped hint unchanged.
	if traceQueryParamWindowDurationSeconds(p) > tracequery.WindowSweepRecoveryMinWindowSeconds {
		sweep := p
		sweep.View = tracequery.ViewWindowSweep
		hint := traceQueryParamsRefinement(ctx, "trace_query_index_event_limit", sweep, sourceLabel, path, true, []string{"time_start", "time_end"})
		if hint != nil {
			if hint.PreferredParams == nil {
				hint.PreferredParams = map[string]string{}
			}
			for _, param := range []string{"pattern", "event_types", "span_name"} {
				delete(hint.PreferredParams, param)
			}
			hint.PreferredParams["micro_window_policy"] = "sub_50ms_local_only"
			normalized := types.NormalizeToolRefinementHint(*hint)
			hint = &normalized
		}
		return hint
	}
	next := p
	required := []string{"time_start", "time_end", "state_cluster_first"}
	if next.LineStart.Int() <= 0 && next.LineEnd.Int() <= 0 {
		traceQueryApplyEventSearchFallback(&next, false)
		required = append(required, "line_start", "line_end")
	}
	hint := traceQueryParamsRefinement(ctx, "trace_query_index_event_limit", next, sourceLabel, path, true, required)
	if hint != nil {
		if hint.PreferredParams == nil {
			hint.PreferredParams = map[string]string{}
		}
		hint.PreferredParams["parent_coverage"] = tracequery.FallbackParentCoverageStateCluster
		hint.PreferredParams["micro_window_policy"] = "sub_50ms_local_only"
		q := traceQueryBuildQuery(ctx, next, sourceLabel, path, next.TimeStart.Seconds(), next.TimeEnd.Seconds())
		hint.ParamNarrowingSuggestions = traceQueryNarrowingSuggestions(q, types.ToolParamNarrowReasonIndexEventLimit)
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
		ViaThread:            p.ViaThread,
		MinDurationMs:        p.MinDurationMs.Float64(),
		Limit:                p.Limit.Int(),
		BucketMs:             p.BucketMs.Float64(),
		CoreTopology:         p.CoreTopology,
		IncludeWindowStats:   p.IncludeWindowStats != nil && p.IncludeWindowStats.Bool(),
	}
	q.TracePlatformHint, q.TracePlatformSource = tracePlatformHintForQuery(ctx, p, sourceLabel, path)
	q.TraceFlavorHint, q.TraceFlavorHintSource = traceFlavorHintForQuery(ctx, p, sourceLabel, path, q.TracePlatformHint, q.TracePlatformSource)
	if p.IncludeWindowStats == nil && strings.TrimSpace(p.View) == "wakeup_chain" {
		q.IncludeWindowStats = true
	}
	// tool_width_trace_query_event_search_limit: the operator override for the
	// event_search default limit is applied here on the outgoing query when the
	// caller passed no explicit limit, so the engine capacity table (E4 single
	// source) stays untouched. Unset override leaves Limit<=0 and the engine
	// default applies — byte-identical default behavior.
	if q.Limit <= 0 && tracequery.CanonicalViewName(q.View) == tracequery.FallbackViewEventSearch {
		if override := traceQueryWidthEventSearchLimitOverride(); override > 0 {
			q.Limit = override
		}
	}
	return q
}

func (t *TraceQuery) maybeLargePatternWindowedView(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, timeCaveat string) (types.ToolResult, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < traceQueryWindowedIndexMinBytes || !traceQueryShouldAutoWindowFromPattern(p) {
		return types.ToolResult{}, false
	}
	// Auto-window discovery is a single-physical-file streaming optimization.
	// A bundle (including sibling-promoted artifacts) must fall through to the
	// normal indexed path so physical source coordinates and clock-domain
	// admission remain part of the query result.
	if tracequery.TracePathRequiresCompositeIndex(path) {
		return types.ToolResult{}, false
	}
	pattern := firstNonEmptyTraceString(p.Pattern, p.SpanName)
	searchP := p
	searchP.View = "event_search"
	searchP.Pattern = pattern
	searchQ := traceQueryBuildQuery(ctx, searchP, sourceLabel, path, 0, 0)
	if minLimit := traceQueryWidthStreamSearchMinLimit(); searchQ.Limit < minLimit {
		searchQ.Limit = minLimit
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
		payload, marshalFailure := traceQueryMarshalPayload(t.Name(), searchResult)
		if marshalFailure != nil {
			return *marshalFailure, true
		}
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
	payload, marshalFailure := traceQueryMarshalPayload(t.Name(), result)
	if marshalFailure != nil {
		return *marshalFailure, true
	}
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
	payloadBytes, marshalFailure := traceQueryMarshalPayload(t.Name(), payload)
	if marshalFailure != nil {
		return *marshalFailure
	}
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

func (t *TraceQuery) maybeStreamEventSearch(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string, window traceQueryNormalizedWindow, timeCaveat string) (types.ToolResult, bool) {
	// C3 (§7.30.2): view=event_search ALWAYS prefers the streaming scan,
	// regardless of trace size. The streaming path never materializes the
	// in-memory Event index, so it cannot hit the index event budget — the
	// budget-capped indexed path silently searched a truncated event set and
	// produced zero matches (trace_query_event_search_zero_match) on dense
	// traces. Any stat/stream failure falls back to the indexed path below.
	if !traceQueryShouldStreamEventSearch(p) {
		return types.ToolResult{}, false
	}
	if _, err := os.Stat(path); err != nil {
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, window.LookupStart, window.LookupEnd)
	streamStart := time.Now()
	logging.Debug("[trace_query] phase=stream_event_search view=%s source=%s path=%s start pattern=%s event_types=%d",
		q.View, sourceLabel, path, p.Pattern, len(q.EventTypes))
	result, err := tracequery.StreamEventSearch(contextFromBus(ctx), path, q)
	if err != nil {
		logging.Debug("[trace_query] phase=stream_event_search view=%s path=%s failed elapsed=%s err=%v; falling back to the indexed event_search path", q.View, path, time.Since(streamStart), err)
		return types.ToolResult{}, false
	}
	if len(result.Events) == 0 {
		// The streaming prefilter matches the RAW line text; the indexed
		// path additionally matches typed/normalized fields (e.g. the
		// canonical event type name). A zero-match stream therefore falls
		// back to the indexed search so type-only patterns keep working —
		// on budget-capped traces that path still returns the typed
		// recovery caveat instead of a silently truncated zero match.
		logging.Debug("[trace_query] phase=stream_event_search view=%s path=%s zero raw-text matches elapsed=%s; falling back to the indexed event_search path for typed-field matching", q.View, path, time.Since(streamStart))
		return types.ToolResult{}, false
	}
	heapAlloc, heapSys, gcCount := traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=stream_event_search view=%s path=%s done elapsed=%s matched=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
		q.View, path, time.Since(streamStart), len(result.Events), len(result.Caveats), heapAlloc, heapSys, gcCount)
	traceQueryAppendCallCaveats(&result, timeCaveat)
	result.Caveats = append(result.Caveats, traceQueryObjectiveExactTokenCaveats(ctx, p, result)...)
	storeStart := time.Now()
	payload, marshalFailure := traceQueryMarshalPayload(t.Name(), result)
	if marshalFailure != nil {
		return *marshalFailure, true
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	summary := traceQuerySummary(result, p, sourceLabel, payloadRef)
	preview, rawRef := StoreBlob(ctx, t.Name(), summary)
	if rawRef == "" {
		rawRef = payloadRef
	}
	logging.Debug("[trace_query] phase=store_result view=%s path=%s done elapsed=%s payload_ref=%s raw_ref=%s", q.View, path, time.Since(storeStart), payloadRef, rawRef)
	now := time.Now()
	observations := traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now)
	traceQueryAnnotateLookupWindowContract(observations, window)
	return types.ToolResult{
		ToolName:     t.Name(),
		Success:      true,
		Summary:      preview,
		RawRef:       rawRef,
		Refinement:   traceQueryRefinement(result, q, p, sourceLabel),
		Observations: observations,
		Timestamp:    now,
	}, true
}

// maybeStreamWindowSweep dispatches view=window_sweep (§4.7 W3) onto the
// streaming coverage engine. Unlike maybeStreamEventSearch there is no
// indexed fallback — window_sweep IS the streaming channel, so any failure is
// reported directly instead of silently falling through to an index build the
// view was designed to avoid.
func (t *TraceQuery) maybeStreamWindowSweep(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string, timeStart, timeEnd float64, timeCaveat string) (types.ToolResult, bool) {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewWindowSweep {
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, timeStart, timeEnd)
	sweepStart := time.Now()
	logging.Debug("[trace_query] phase=stream_window_sweep source=%s path=%s start time_start=%.6f time_end=%.6f bucket_ms=%.1f pid=%d",
		sourceLabel, path, timeStart, timeEnd, q.BucketMs, q.PID)
	result, err := tracequery.StreamWindowSweep(contextFromBus(ctx), path, q)
	if err != nil {
		logging.Debug("[trace_query] phase=stream_window_sweep path=%s failed elapsed=%s err=%v", path, time.Since(sweepStart), err)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("trace_query failed to sweep %s: %v", path, err),
			Timestamp: time.Now(),
		}, true
	}
	heapAlloc, heapSys, gcCount := traceQueryMemoryForLog()
	logging.Debug("[trace_query] phase=stream_window_sweep path=%s done elapsed=%s buckets=%d hotspots=%d caveats=%d heap_alloc_bytes=%d heap_sys_bytes=%d gc_count=%d",
		path, time.Since(sweepStart), traceQueryWindowSweepBucketCount(result), traceQueryWindowSweepHotspotCount(result), len(result.Caveats), heapAlloc, heapSys, gcCount)
	traceQueryAppendCallCaveats(&result, timeCaveat)
	payload, marshalFailure := traceQueryMarshalPayload(t.Name(), result)
	if marshalFailure != nil {
		return *marshalFailure, true
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	summary := traceQuerySummary(result, p, sourceLabel, payloadRef)
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
		Refinement:   traceQueryRefinement(result, q, p, sourceLabel),
		Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now),
		Timestamp:    now,
	}, true
}

func traceQueryWindowSweepBucketCount(result tracequery.Result) int {
	if result.WindowSweep == nil {
		return 0
	}
	return result.WindowSweep.BucketCount
}

func traceQueryWindowSweepHotspotCount(result tracequery.Result) int {
	if result.WindowSweep == nil {
		return 0
	}
	return len(result.WindowSweep.Hotspots)
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
	if resultTruncated {
		hint.ParamNarrowingSuggestions = traceQueryNarrowingSuggestions(q, types.ToolParamNarrowReasonEntriesOverThreshold)
		hint = types.NormalizeToolRefinementHint(hint)
	}
	if hint.Empty() {
		return nil
	}
	return &hint
}

// traceQueryNarrowingSuggestions builds the typed per-parameter narrowing
// rows for an over-wide trace_query result, from the typed query only. The
// time-window row carries the C3 coverage-window amplitude (80-150ms) rather
// than a concrete window: concrete recovery windows stay on PreferredParams
// (the E4 over-cap suggestions), which are anti-echo guarded.
func traceQueryNarrowingSuggestions(q tracequery.Query, reasonCode string) []types.ToolParamNarrowingSuggestion {
	out := []types.ToolParamNarrowingSuggestion{{
		Param:    "time_start,time_end",
		Priority: 1,
		Suggested: fmt.Sprintf("%.0f-%.0fms coverage window",
			traceQueryPreferredCoverageWindowMinSeconds*1000,
			traceQueryPreferredCoverageWindowMaxSeconds*1000),
		ReasonCode: reasonCode,
	}}
	lineWindow := ""
	if q.LineStart > 0 && q.LineEnd >= q.LineStart {
		lineWindow = fmt.Sprintf("%d-%d", q.LineStart, q.LineEnd)
	}
	out = append(out, types.ToolParamNarrowingSuggestion{
		Param:      "line_start,line_end",
		Priority:   2,
		Suggested:  lineWindow,
		ReasonCode: reasonCode,
	})
	eventTypes := traceQueryEventTypesParamString(q.EventTypes)
	if eventTypes == "" {
		eventTypes = string(tracequery.EventTraceMark)
	}
	out = append(out, types.ToolParamNarrowingSuggestion{
		Param:      "event_types",
		Priority:   3,
		Suggested:  eventTypes,
		ReasonCode: reasonCode,
	})
	out = append(out, types.ToolParamNarrowingSuggestion{
		Param:      "limit",
		Priority:   4,
		Suggested:  strconv.Itoa(traceQueryWidthEventSearchDefaultLimit()),
		ReasonCode: reasonCode,
	})
	return out
}

const tTraceQueryName = "trace_query"

func traceQueryHeavyViewGuardRefinement(ctx *types.BusContext, p traceQueryParams, sourceLabel, path string) *types.ToolRefinementHint {
	next := p
	traceQueryApplyEventSearchFallback(&next, true)
	return traceQueryParamsRefinement(ctx, "trace_query_heavy_view_requires_scope", next, sourceLabel, path, true, []string{"pattern"})
}

// traceQueryApplyEventSearchFallback rewrites next to the C3 event_search
// escape-hatch call shape. View name, discovery event types, and the default
// limit are all sourced from the tracequery capacity table's shared tokens
// (E4 literal-source consolidation) so the guard/index-limit/recipe recovery
// surfaces cannot drift from the engine. forceEventTypes preserves each call
// site's historical behavior: the heavy-view guard and recipe discovery
// always pin trace_mark discovery filters, while the index-limit path keeps
// caller-provided event_types.
func traceQueryApplyEventSearchFallback(next *traceQueryParams, forceEventTypes bool) {
	fallback := tracequery.ViewCapacityFor(tracequery.FallbackViewEventSearch)
	next.View = fallback.View
	if forceEventTypes || len(next.EventTypes.Strings()) == 0 {
		next.EventTypes = TraceEventTypes{string(tracequery.EventTraceMark)}
	}
	if next.Limit.Int() <= 0 {
		// Width-governor effective default: the tracequery table value unless
		// the tool_width_trace_query_event_search_limit override is set.
		next.Limit = FlexInt(traceQueryWidthEventSearchDefaultLimit())
	}
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
	traceQueryApplyEventSearchFallback(&next, true)
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
	// Typed-first (E4): engine truncation sites publish Result.Compactions.
	// The verbatim caveat-substring checks below remain only as a fallback
	// for paths not yet publishing typed records; "_compacted total=" is the
	// tracebundle list-compaction marker (parse.go), which the older two
	// substrings never matched.
	if len(result.Compactions) > 0 {
		return true
	}
	for _, caveat := range result.Caveats {
		lower := strings.ToLower(strings.TrimSpace(caveat))
		if strings.Contains(lower, " compacted from ") ||
			strings.Contains(lower, " compacted after ") ||
			strings.Contains(lower, "_compacted total=") {
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
	traceQueryApplyOverCapSuggestions(params, result, q)
	return params
}

// traceQueryApplyOverCapSuggestions upgrades the echoed over-capacity params
// to concrete recovery values (E4): a suggested limit bounded by the view's
// MaxLimit while raising the limit can still widen the result, otherwise a
// concrete first-segment window split derived from the last emitted row —
// the same copy-pastable style C3 established for index budget hits. All of
// this stays soft guidance; suggestions are strictly different from the
// failing call's params so repeated over-cap results present changing
// fingerprints to the same-cause no-progress breaker instead of feeding a
// suggestion→same-call→same-suggestion loop.
func traceQueryApplyOverCapSuggestions(params map[string]string, result tracequery.Result, q tracequery.Query) {
	compaction, ok := traceQueryPrimaryCompaction(result, q)
	if !ok {
		return
	}
	capacity := tracequery.ViewCapacityFor(traceQueryCanonicalView(result, q))
	// Heavy views advertise their behaviorally-established fallback view.
	// event_search never gets one: it IS the C3 streaming escape hatch
	// (133520c1) and must not be told to leave itself.
	if capacity.HeavyView && capacity.FallbackView != "" {
		params["fallback_view"] = capacity.FallbackView
	}
	// The widen-vs-split decision reads the TRUNCATING view's row, not the
	// result's: composite views (recipe, frame_root_cause_bundle, ...) have
	// MaxLimit=0 while their mirrored sub-view compactions carry hard clamps
	// a bigger limit can never satisfy — judging by the composite row would
	// suggest limit=Total forever, the exact identical-echo loop the
	// anti-echo rule forbids (adversarial-review finding on the first E4
	// cut). A clamped sub-view falls through to the window split instead.
	limitRow := capacity
	if compaction.View != "" && compaction.View != capacity.View {
		limitRow = tracequery.ViewCapacityFor(compaction.View)
	}
	effective := limitRow.ClampLimit(q.Limit)
	if compaction.Total > compaction.Emitted && (limitRow.MaxLimit == 0 || effective < limitRow.MaxLimit) {
		// The requested limit is below the truncating view's ceiling (or it
		// has none) and the true total is known: suggest the limit that
		// widens the result, never the echoed capped value.
		suggested := compaction.Total
		if limitRow.MaxLimit > 0 && suggested > limitRow.MaxLimit {
			suggested = limitRow.MaxLimit
		}
		params["limit"] = strconv.Itoa(suggested)
		return
	}
	traceQueryApplyWindowSplitSuggestion(params, result, q, compaction)
}

// traceQueryApplyWindowSplitSuggestion emits the concrete sub-window split
// for views already at their hard cap: the first segment ends at the last
// emitted row's timestamp, and next_segment names the remainder so the model
// can walk the window without guessing. Skipped unless the cut point falls
// strictly inside the current window (an out-of-window cut would echo the
// failing call or produce an empty segment).
func traceQueryApplyWindowSplitSuggestion(params map[string]string, result tracequery.Result, q tracequery.Query, compaction tracequery.ViewCompaction) {
	start := q.TimeStart
	if start == 0 {
		start = result.TimeStart
	}
	end := q.TimeEnd
	if end == 0 {
		end = result.TimeEnd
	}
	cut := compaction.LastEmittedTs
	if cut <= start || (end > 0 && cut >= end) {
		return
	}
	params["time_start"] = traceQueryFloatParamString(start)
	params["time_end"] = traceQueryFloatParamString(cut)
	if end > 0 {
		params["next_segment"] = fmt.Sprintf("time_start=%s time_end=%s", traceQueryFloatParamString(cut), traceQueryFloatParamString(end))
	} else {
		params["next_segment"] = "time_start=" + traceQueryFloatParamString(cut)
	}
}

// traceQueryPrimaryCompaction picks the typed truncation record that matches
// the result's canonical view, falling back to the first record when a
// composite view only carries sub-view compactions.
func traceQueryPrimaryCompaction(result tracequery.Result, q tracequery.Query) (tracequery.ViewCompaction, bool) {
	if len(result.Compactions) == 0 {
		return tracequery.ViewCompaction{}, false
	}
	view := traceQueryCanonicalView(result, q)
	for _, compaction := range result.Compactions {
		if compaction.View == view {
			return compaction, true
		}
	}
	return result.Compactions[0], true
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
	opts := traceQueryWindowedIndexOptions(p, timeStart, timeEnd)
	// Relation-scope pruning is a FALLBACK, not the default (verified design:
	// "first raise the cap; only prune if that is not enough"). Always try the
	// full, unpruned byte-budgeted window first — it is correct for every view
	// and carries zero pruning risk. Only when even the raised cap overflows
	// (IndexEventLimitError) on a relation-scopeable causal-chain view do we
	// retry with pid-relation pruning. This keeps the common large-trace case on
	// the safe path and reserves pruning for genuinely too-dense windows.
	wantRelationScoped := opts.RelationScoped
	opts.RelationScoped = false
	idx, buildErr := tracequery.BuildIndexWithOptions(ctx, path, opts)
	if buildErr != nil && wantRelationScoped {
		var limitErr *tracequery.IndexEventLimitError
		if errors.As(buildErr, &limitErr) {
			opts.RelationScoped = true
			idx, buildErr = tracequery.BuildIndexWithOptions(ctx, path, opts)
		}
	}
	// QF4 level 2: when the budget was exhausted BEFORE the request window
	// was covered (a denial the parse-side padding-tail degrade cannot save),
	// retry exactly once with window-proportional padding — the first build
	// keeps the historical fixed padding so healthy builds lose zero context.
	// The retry inherits whatever configuration just failed (including a
	// failed relation-scope retry above). If the retry fails too, the
	// pre-existing denial surfaces on the original error, as if no retry ran.
	if buildErr != nil {
		if retryOpts, ok := traceQueryReducedPaddingRetryOptions(p, opts, buildErr, timeStart, timeEnd); ok {
			logging.Warning("[trace_query] windowed index build exhausted its event budget before covering the request window (view=%s path=%s); retrying once with window-proportional padding ±%.6fs (was ±%.6fs)",
				firstNonEmptyTraceString(p.View, "window_stats"), path, retryOpts.TimePaddingBefore, opts.TimePaddingBefore)
			if retryIdx, retryErr := tracequery.BuildIndexWithOptions(ctx, path, retryOpts); retryErr == nil {
				retryIdx.Caveats = append(retryIdx.Caveats, traceQueryReducedPaddingCaveat(retryOpts.TimePaddingBefore))
				return retryIdx, nil
			}
		}
	}
	return idx, buildErr
}

// traceQueryWindowedIndexOptions builds the windowed BuildOptions for a
// large-trace heavy-view query, including the pid/thread-scoped MaxEvents raise.
// Pure and side-effect free so the scope/cap decision is unit-testable without
// materializing a multi-hundred-thousand-event fixture.
func traceQueryWindowedIndexOptions(p traceQueryParams, timeStart, timeEnd float64) tracequery.BuildOptions {
	timePadding := traceQueryWindowedIndexTimePadding(p.View)
	opts := tracequery.BuildOptions{
		TimeStart:          timeStart,
		TimeEnd:            timeEnd,
		TimeStartSet:       p.TimeStart.Set(),
		TimeEndSet:         p.TimeEnd.Set(),
		TimePaddingBefore:  timePadding,
		TimePaddingAfter:   timePadding,
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
		// Thread-only scopes are accepted here because tracequery's discovery
		// pass prunes only after resolving the typed thread selector to a single
		// pid/tgid universe; ambiguous selectors degrade to unpruned + caveat.
		if traceQueryRelationScopedView(p.View) {
			opts.RelationScoped = true
			opts.ScopeMaxDepth = traceQueryRelationScopeMaxDepth(p)
		}
	}
	return opts
}

// traceQueryRelationScopedView reports whether a view's event consumption is a
// provable subset of the target pid's threads, their transitive scheduler
// wakers, and binder rows — the only views for which relation-scope index
// pruning is complete (verified design w9ffnwv29). Table-driven (E4): the
// flag lives on the tracequery view capacity table.
func traceQueryRelationScopedView(view string) bool {
	return tracequery.RelationScopedView(view)
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

// traceQueryIndexTimePaddingFloorSeconds is the minimum context padding on
// each side of a windowed index build — enough to catch the wakeup edge /
// span-open marker immediately preceding a tight window without re-scanning
// a disproportionate tail.
const traceQueryIndexTimePaddingFloorSeconds = 0.050

// traceQueryIndexTimePaddingWindowRatio scales the padding with the request
// window so context stays proportionate: half the window on each side doubles
// the scanned range at most, regardless of how small the window is.
const traceQueryIndexTimePaddingWindowRatio = 0.5

// traceQueryWindowedIndexTimePadding returns the per-side index time padding
// for the FIRST windowed build attempt: the historical fixed per-view values,
// byte-identical to pre-2026-07-03 behavior. QF4 two-level policy: applying
// the window-proportional padding unconditionally silently shrank the
// pre-window visibility (open scheduler states, wakeup edges, span-open
// markers) for every healthy build that never came close to the event
// budget. The proportional value now lives EXCLUSIVELY on the single
// budget-exhaustion retry (traceQueryReducedIndexTimePadding), gated on a
// precise failure signal (IndexEventLimitError whose LastTs never reached
// the requested TimeEnd).
func traceQueryWindowedIndexTimePadding(view string) float64 {
	switch strings.TrimSpace(view) {
	case "event_search":
		return 0.050
	case "thread_timeline", "scheduler_latency_stats":
		return 0.250
	default:
		return 0.500
	}
}

// traceQueryReducedIndexTimePadding returns the window-proportional per-side
// padding for the single reduced-padding retry after a windowed build
// exhausted its event budget before covering the request window:
// min(viewCap, max(0.050, window*0.5)) — half the window per side, floored
// at the 50ms wakeup-context minimum, never above the per-view first-build
// value. ok=false when the call carries no complete [time_start,time_end]
// window (nothing to scale against), the window is degenerate, or the
// proportional value is not STRICTLY smaller than the current padding — a
// retry at unchanged padding would deterministically fail again.
func traceQueryReducedIndexTimePadding(p traceQueryParams, timeStart, timeEnd, current float64) (float64, bool) {
	if !p.TimeStart.Set() || !p.TimeEnd.Set() {
		return 0, false
	}
	window := timeEnd - timeStart
	if window <= 0 {
		return 0, false
	}
	padding := window * traceQueryIndexTimePaddingWindowRatio
	if padding < traceQueryIndexTimePaddingFloorSeconds {
		padding = traceQueryIndexTimePaddingFloorSeconds
	}
	if viewCap := traceQueryWindowedIndexTimePadding(p.View); padding > viewCap {
		padding = viewCap
	}
	if padding >= current {
		return 0, false
	}
	return padding, true
}

// traceQueryReducedPaddingRetryOptions decides whether a failed windowed
// build qualifies for the single reduced-padding retry (QF4) and returns the
// retry options. Precise trigger signals only: (1) the failure is a typed
// IndexEventLimitError; (2) the request window has a bounded TimeEnd the
// parse never reached (limitErr.LastTs < timeEnd — when the window WAS
// covered, the parse-side PaddingTruncated degrade already yields a usable
// index and shrinking padding adds nothing); (3) the proportional padding is
// strictly smaller than the padding that just failed. Everything except the
// paddings is preserved from the failed attempt (MaxEvents, scope, relation
// pruning), so this stays exactly one extra lever.
func traceQueryReducedPaddingRetryOptions(p traceQueryParams, opts tracequery.BuildOptions, buildErr error, timeStart, timeEnd float64) (tracequery.BuildOptions, bool) {
	var limitErr *tracequery.IndexEventLimitError
	if !errors.As(buildErr, &limitErr) {
		return tracequery.BuildOptions{}, false
	}
	if !p.TimeEnd.Set() || limitErr.LastTs >= timeEnd {
		return tracequery.BuildOptions{}, false
	}
	padding, ok := traceQueryReducedIndexTimePadding(p, timeStart, timeEnd, opts.TimePaddingBefore)
	if !ok {
		return tracequery.BuildOptions{}, false
	}
	retry := opts
	retry.TimePaddingBefore = padding
	retry.TimePaddingAfter = padding
	return retry, true
}

// traceQueryReducedPaddingCaveat is the Result-caveat note attached (via
// Index.Caveats) to an index that was rebuilt on the reduced-padding retry,
// so the model knows the context margin around the request window is thinner
// than the per-view default.
func traceQueryReducedPaddingCaveat(padding float64) string {
	return fmt.Sprintf("index rebuilt with reduced padding ±%.4fs (window-proportional) after budget exhaustion", padding)
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

// traceQueryIsHeavyView is table-driven (E4): every view except the two
// streaming channels (event_search, window_sweep) is heavy, and the flag
// lives on the tracequery view capacity table so the tool and engine cannot
// drift.
func traceQueryIsHeavyView(view string) bool {
	return tracequery.IsHeavyView(view)
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

// traceQuerySourceAdaptation records a deterministic compatibility repair
// applied before trace_query resolves its source. It is deliberately kept out
// of the engine query: the canonical Source/Path values replace the malformed
// model parameters, while this record exists only to teach the caller the
// cheaper form for subsequent calls.
type traceQuerySourceAdaptation struct {
	LogicalID       string
	CanonicalSource string
	CanonicalPath   string
}

type traceQueryLogicalArtifactCandidate struct {
	resolved string
	source   string
	attached bool
	info     os.FileInfo
}

// traceQueryAdaptLogicalArtifactPath accepts one narrow compatibility shape:
// a model copied a runtime-artifact selection id into source=path/path. A
// logical id is never sent through filesystem resolution. Instead, it must
// match exactly one item in the current typed selection view and that item must
// resolve, through typed carriers plus stat verification, to exactly one
// physical trace artifact. Zero/many candidates fail closed.
func traceQueryAdaptLogicalArtifactPath(ctx *types.BusContext, p traceQueryParams) (traceQueryParams, *traceQuerySourceAdaptation, *types.ToolResult) {
	logicalID := strings.TrimSpace(p.Path)
	if !traceQueryRuntimeArtifactSelectionIDRE.MatchString(logicalID) {
		return p, nil, nil
	}
	source := strings.TrimSpace(p.Source)
	if source != "" && source != "path" && source != "attached_trace" {
		return p, nil, traceQueryLogicalArtifactIDReject(
			"trace_query_runtime_artifact_id_invalid_source",
			fmt.Sprintf("trace_query did not treat logical path %q as a filesystem path: source=%q is not a supported trace source. Retry with source=\"attached_trace\" and no path for the current attachment, or source=\"path\" with the typed trace item's source value.", logicalID, source),
			logicalID,
		)
	}

	view := traceQueryRuntimeArtifactSelectionView(ctx)
	var matches []types.RuntimeArtifactSelectionItem
	for _, item := range view.Items {
		if strings.EqualFold(strings.TrimSpace(item.ID), logicalID) {
			matches = append(matches, item)
		}
	}
	if len(matches) != 1 {
		return p, nil, traceQueryLogicalArtifactIDReject(
			"trace_query_runtime_artifact_id_unknown",
			fmt.Sprintf("trace_query did not treat logical path %q as a filesystem path: it does not name exactly one item in the current typed runtime-artifact selection set.%s", logicalID, traceQueryRuntimeArtifactSelectionHint(view)),
			logicalID,
		)
	}
	item := matches[0]
	if item.Kind != "trace" {
		return p, nil, traceQueryLogicalArtifactIDReject(
			"trace_query_runtime_artifact_id_wrong_kind",
			fmt.Sprintf("trace_query did not treat logical path %q as a filesystem path: the current typed item has kind=%q, not trace. Use the trace item's source with trace_query; keep log artifacts on the log evidence lane.%s", logicalID, item.Kind, traceQueryRuntimeArtifactSelectionHint(view)),
			logicalID,
		)
	}

	candidates := traceQueryLogicalArtifactCandidates(ctx, item)
	if len(candidates) != 1 {
		if len(candidates) == 0 {
			return p, nil, traceQueryLogicalArtifactIDReject(
				"trace_query_runtime_artifact_id_unresolved",
				fmt.Sprintf("trace_query recognized logical id %q as the current typed trace item (source=%q, carriers=%s), but it has no stat-verified attached blob or trace path. Reattach the trace and call source=\"attached_trace\" without path, or call source=\"path\" with an existing trace file.", logicalID, item.Source, strings.Join(item.Carriers, "+")),
				logicalID,
			)
		}
		names := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			names = append(names, fmt.Sprintf("%q", candidate.resolved))
		}
		return p, nil, traceQueryLogicalArtifactIDReject(
			"trace_query_runtime_artifact_id_ambiguous",
			fmt.Sprintf("trace_query recognized logical id %q, but its typed carriers map to multiple physical trace files: %s. Auto-compatibility refuses to guess. Retry with source=\"attached_trace\" and no path for the current attachment, or source=\"path\" and one exact file path.", logicalID, strings.Join(names, ", ")),
			logicalID,
		)
	}
	// A perf_trace:<producer> item identifies a structured view of an
	// attachment, not a capture path. The mutable PerfBundle proves that it
	// came from the attached channel, but its generic producer token cannot
	// distinguish that capture from additional request-referenced trace files.
	// Keep the convenience repair only while the whole typed view resolves to
	// one physical trace. A direct attached_trace/attachment carrier remains an
	// exact channel identity and does not need this extra inventory gate.
	if candidates[0].attached && traceQuerySelectionItemIsPerfAlias(item) {
		physical := traceQueryPhysicalTraceCandidates(ctx, view)
		if len(physical) > 1 {
			names := make([]string, 0, len(physical))
			for _, candidate := range physical {
				names = append(names, fmt.Sprintf("%q", candidate.resolved))
			}
			return p, nil, traceQueryLogicalArtifactIDReject(
				"trace_query_runtime_artifact_id_ambiguous",
				fmt.Sprintf("trace_query recognized logical perf alias %q, but the current typed selection resolves to multiple physical trace files: %s. A producer alias does not identify one capture, so auto-compatibility refuses to guess. Retry with source=\"attached_trace\" and no path for the current attachment, or source=\"path\" and one exact file path.", logicalID, strings.Join(names, ", ")),
				logicalID,
			)
		}
	}

	candidate := candidates[0]
	adaptation := &traceQuerySourceAdaptation{LogicalID: logicalID}
	if candidate.attached {
		p.Source = "attached_trace"
		p.Path = ""
		adaptation.CanonicalSource = "attached_trace"
		logging.Warning("[trace_query] source=path logical typed artifact id %q auto-resolved to the single physical attached trace; canonical next call is source=attached_trace with no path", logicalID)
		return p, adaptation, nil
	}
	p.Source = "path"
	p.Path = candidate.source
	adaptation.CanonicalSource = "path"
	adaptation.CanonicalPath = candidate.source
	logging.Warning("[trace_query] source=path logical typed artifact id %q auto-resolved to the single stat-verified typed source %q", logicalID, candidate.source)
	return p, adaptation, nil
}

func traceQueryRuntimeArtifactSelectionView(ctx *types.BusContext) types.RuntimeArtifactSelectionView {
	if ctx == nil {
		return types.RuntimeArtifactSelectionViewFromAgentContext(nil)
	}
	var perf *types.PerfBundle
	if ctx.Mutable != nil {
		perf = ctx.Mutable.PerfTrace()
	}
	return types.RuntimeArtifactSelectionViewFromAgentContext(&types.AgentContext{
		Mutable:                  ctx.Mutable,
		RuntimeArtifactPreflight: ctx.RuntimeArtifactPreflight,
		AttachedLog:              ctx.AttachedLog,
		AttachedHitrace:          ctx.AttachedHitrace,
		AttachedHitraceSource:    ctx.AttachedHitraceSource,
		AnalysisIR:               ctx.AnalysisIR,
		PerfTrace:                perf,
	})
}

func traceQueryLogicalArtifactCandidates(ctx *types.BusContext, item types.RuntimeArtifactSelectionItem) []traceQueryLogicalArtifactCandidate {
	var out []traceQueryLogicalArtifactCandidate
	appendCandidate := func(candidate traceQueryLogicalArtifactCandidate) {
		info, err := os.Stat(candidate.resolved)
		if err != nil || info.IsDir() {
			return
		}
		candidate.info = info
		for _, prior := range out {
			if filepath.Clean(prior.resolved) == filepath.Clean(candidate.resolved) || os.SameFile(prior.info, info) {
				return
			}
		}
		out = append(out, candidate)
	}

	if traceQuerySelectionItemMapsToAttachedTrace(ctx, item) {
		if path, ok := resolveAttachedTraceQueryPath(ctx); ok {
			appendCandidate(traceQueryLogicalArtifactCandidate{resolved: path, source: "attached_trace", attached: true})
		}
	}
	if types.RuntimeArtifactPathKind(item.Source) == "trace" {
		appendCandidate(traceQueryLogicalArtifactCandidate{
			resolved: resolveToolPath(ctx, item.Source),
			source:   item.Source,
		})
	}
	return out
}

func traceQueryPhysicalTraceCandidates(ctx *types.BusContext, view types.RuntimeArtifactSelectionView) []traceQueryLogicalArtifactCandidate {
	var out []traceQueryLogicalArtifactCandidate
	for _, item := range view.Items {
		if item.Kind != "trace" {
			continue
		}
		for _, candidate := range traceQueryLogicalArtifactCandidates(ctx, item) {
			duplicate := false
			for _, prior := range out {
				if filepath.Clean(prior.resolved) == filepath.Clean(candidate.resolved) || os.SameFile(prior.info, candidate.info) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				out = append(out, candidate)
			}
		}
	}
	return out
}

func traceQuerySelectionItemIsPerfAlias(item types.RuntimeArtifactSelectionItem) bool {
	directAttachment := false
	perfAlias := false
	for _, carrier := range item.Carriers {
		switch strings.TrimSpace(carrier) {
		case "attachment", "attached_trace":
			directAttachment = true
		case "perf_trace", "mutable_perf_trace":
			perfAlias = true
		}
	}
	return perfAlias && !directAttachment
}

func traceQuerySelectionItemMapsToAttachedTrace(ctx *types.BusContext, item types.RuntimeArtifactSelectionItem) bool {
	if ctx == nil {
		return false
	}
	for _, carrier := range item.Carriers {
		switch strings.TrimSpace(carrier) {
		case "attachment", "attached_trace":
			return true
		}
	}
	var perf *types.PerfBundle
	if ctx.Mutable != nil {
		perf = ctx.Mutable.PerfTrace()
	}
	if perf == nil || traceQueryPerfArtifactSelectionSource(perf) != strings.TrimSpace(item.Source) {
		return false
	}
	for _, carrier := range item.Carriers {
		switch strings.TrimSpace(carrier) {
		case "perf_trace", "mutable_perf_trace":
			return true
		}
	}
	return false
}

func traceQueryPerfArtifactSelectionSource(perf *types.PerfBundle) string {
	if perf == nil || strings.TrimSpace(perf.Meta.Source) == "" {
		return "perf_trace"
	}
	return "perf_trace:" + strings.TrimSpace(perf.Meta.Source)
}

func traceQueryRuntimeArtifactSelectionHint(view types.RuntimeArtifactSelectionView) string {
	if len(view.Items) == 0 {
		return " No current typed runtime artifacts are available."
	}
	parts := make([]string, 0, len(view.Items))
	for i, item := range view.Items {
		if i >= 8 {
			parts = append(parts, fmt.Sprintf("... %d more", len(view.Items)-i))
			break
		}
		parts = append(parts, fmt.Sprintf("id=%s kind=%s source=%q", item.ID, item.Kind, item.Source))
	}
	return " Current typed items: " + strings.Join(parts, "; ") + "."
}

func traceQueryLogicalArtifactIDReject(code, summary, logicalID string) *types.ToolResult {
	return &types.ToolResult{
		ToolName: "trace_query",
		Success:  false,
		Summary:  summary,
		Repair: &types.ToolRepair{
			Code:   code,
			Hint:   "Use source=\"attached_trace\" with no path for the current attached trace. For a filesystem trace, use source=\"path\" with the typed item's source value, never its runtime_artifact:<id>.",
			Fields: []string{"source", "path"},
			Metadata: map[string]string{
				"logical_artifact_id": logicalID,
				"next_tool":           "trace_query",
			},
		},
		Timestamp: time.Now(),
	}
}

func traceQueryAnnotateSourceAdaptation(result *types.ToolResult, adaptation *traceQuerySourceAdaptation) {
	if result == nil || adaptation == nil || strings.TrimSpace(adaptation.LogicalID) == "" {
		return
	}
	next := `source="attached_trace" path=<omit>`
	if adaptation.CanonicalSource == "path" {
		next = fmt.Sprintf(`source="path" path=%q`, adaptation.CanonicalPath)
	}
	note := fmt.Sprintf("[trace_query source compatibility: logical_id=%s auto_resolved=true resolved_source=%s mapping=current_typed_selection+single_physical_trace canonical_next_call=%s]", adaptation.LogicalID, adaptation.CanonicalSource, next)
	if strings.Contains(result.Summary, note) {
		return
	}
	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = note
		return
	}
	result.Summary = note + "\n" + result.Summary
}

func resolveTraceQuerySource(ctx *types.BusContext, p traceQueryParams) (string, string, *types.ToolResult) {
	adapted, _, reject := traceQueryAdaptLogicalArtifactPath(ctx, p)
	if reject != nil {
		return "", strings.TrimSpace(p.Source), reject
	}
	p = adapted
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
		// Mechanical repair (customer friction 2026-07-03): when no
		// attached blob exists but the call already names a real trace
		// FILE, the model meant the referenced-by-path artifact — fall
		// back to source=path with the model's own params instead of
		// burning a retry round teaching it to flip one enum. Purely
		// deterministic: nothing is guessed, the path is model-supplied
		// and stat-verified.
		if candidate := strings.TrimSpace(p.Path); candidate != "" {
			resolved := resolveToolPath(ctx, candidate)
			if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
				logging.Warning("[trace_query] source=attached_trace has no attached blob; auto-resolved to source=path for %q", candidate)
				source = "path"
			}
		}
		if source == "attached_trace" {
			// Adversarial-review QF1' (2026-07-03): the auto-resolve lane
			// below exists ONLY for calls that named no path at all. When the
			// model DID pass an explicit path and it failed the stat check in
			// the lane above, silently substituting a different file would
			// answer about the wrong trace (e.g. the user names a fresh
			// capture that does not exist yet and quietly gets results from
			// an old one). Reject instead; known referenced artifacts are
			// surfaced as a hint only, never auto-adopted. The gate is a
			// precise signal: a single empty-string check on the param.
			if explicit := strings.TrimSpace(p.Path); explicit != "" {
				summary := fmt.Sprintf(
					"trace_query source=attached_trace has no attached trace blob, and the explicitly provided path %q does not resolve to an existing trace file. An explicit path is never silently replaced with a different artifact.",
					explicit,
				)
				if hints := attachedTraceQueryReferencedArtifactCandidates(ctx); len(hints) > 0 {
					names := make([]string, 0, len(hints))
					for _, hint := range hints {
						names = append(names, fmt.Sprintf("%q", hint.display))
					}
					summary += " Known referenced trace artifacts (hint only): " + strings.Join(names, ", ") + "."
				}
				summary += " Re-issue trace_query with source=\"path\" and an existing trace file, or attach one via --htrace/--atrace."
				return "", source, &types.ToolResult{
					ToolName:  "trace_query",
					Success:   false,
					Summary:   summary,
					Timestamp: time.Now(),
				}
			}
			// Mechanical repair, second lane (customer friction 2026-07-03,
			// donghu session): the model called source=attached_trace WITHOUT
			// a path because the CLI's main-attachment banner presented a
			// request-referenced trace file as "attached" — but that artifact
			// is a plain file on disk, not an --htrace/--atrace blob, so the
			// branch above found nothing and the call burned a retry round.
			// Recover from typed carriers only (RuntimeArtifactPreflight and
			// AnalysisIR structured fields; RawRequest is never read) and
			// accept ONLY when exactly one stat-verified trace file remains —
			// a precise integer-count gate, nothing fuzzy decides the path.
			candidates := attachedTraceQueryReferencedArtifactCandidates(ctx)
			if len(candidates) == 1 {
				logging.Warning("[trace_query] source=attached_trace has no attached blob; auto-resolved to the single request-referenced trace artifact %q from typed analysis carriers", candidates[0].display)
				return candidates[0].resolved, "path", nil
			}
			summary := "trace_query requires an attached trace blob, but none is available. Use source=\"path\" with an explicit trace file, or attach one via --htrace/--atrace."
			if len(candidates) > 1 {
				names := make([]string, 0, len(candidates))
				for _, candidate := range candidates {
					names = append(names, fmt.Sprintf("%q", candidate.display))
				}
				summary += " Multiple referenced trace artifacts were detected: " + strings.Join(names, ", ") + ". Re-issue trace_query with source=\"path\" and path set to exactly one of them."
			}
			return "", source, &types.ToolResult{
				ToolName:  "trace_query",
				Success:   false,
				Summary:   summary,
				Timestamp: time.Now(),
			}
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

// attachedTraceReferencedCandidate pairs the typed-carrier spelling of a
// request-referenced trace artifact (kept for the error/hint surface so the
// model can copy it into `path` verbatim) with its stat-verified resolved
// filesystem path.
type attachedTraceReferencedCandidate struct {
	display  string
	resolved string
}

// attachedTraceQueryReferencedArtifactCandidates collects trace artifacts the
// current turn REFERENCES by path rather than attaches as an --htrace/--atrace
// blob. Inputs are typed carriers only:
//
//   - BusContext.RuntimeArtifactPreflight (deterministic run-entry detector;
//     Carrier=="attachment" entries are skipped because the attached-blob lane
//     in resolveAttachedTraceQueryPath already owns them), and
//   - AnalysisIR structured fields (RequiredFileHints, ExactTargets, the
//     entity lanes, EvidencePlan.RequiredFiles).
//
// RawRequest prose is never consulted. Every candidate must be path-shaped
// with artifact family "trace" per types.RuntimeArtifactPathKind AND resolve
// (via resolveToolPath) to an existing regular file; results are deduplicated
// first by cleaned resolved-path string (fast layer) and then by os.SameFile
// physical identity (adversarial-review QF2', 2026-07-03: symlinked or
// case-variant spellings of ONE physical file must not read as two candidates
// and falsely fail the caller's exact-one gate). The caller applies the
// exact-one count gate — this function only gathers, it never picks.
func attachedTraceQueryReferencedArtifactCandidates(ctx *types.BusContext) []attachedTraceReferencedCandidate {
	if ctx == nil {
		return nil
	}
	var raw []string
	profile := types.NormalizeRuntimeArtifactPreflightProfile(ctx.RuntimeArtifactPreflight)
	for _, artifact := range profile.Artifacts {
		if artifact.Carrier == "attachment" {
			continue
		}
		if artifact.RuntimeArtifactKind() == "trace" {
			raw = append(raw, artifact.Source)
		}
	}
	if ir := ctx.AnalysisIR; ir != nil {
		hints := ir.RequestModel.AnalyzerHints
		for _, hint := range hints.RequiredFileHints {
			raw = append(raw, hint.Path)
		}
		raw = append(raw, hints.ExactTargets...)
		raw = append(raw, hints.MentionedEntities...)
		raw = append(raw, hints.PrimaryEntities...)
		raw = append(raw, hints.Entities...)
		raw = append(raw, ir.EvidencePlan.RequiredFiles...)
	}
	seen := map[string]bool{}
	var out []attachedTraceReferencedCandidate
	var outInfos []os.FileInfo
	for _, value := range raw {
		for _, token := range types.RuntimeArtifactPathTokensInText(value) {
			if types.RuntimeArtifactPathKind(token) != "trace" {
				continue
			}
			resolved := resolveToolPath(ctx, token)
			info, err := os.Stat(resolved)
			if err != nil || info.IsDir() {
				continue
			}
			key := filepath.Clean(resolved)
			if abs, absErr := filepath.Abs(key); absErr == nil {
				key = filepath.Clean(abs)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			// Second dedupe layer: os.SameFile on the stat we already hold.
			// Distinct spellings (symlink, case-insensitive filesystem) of one
			// physical file carry distinct string keys but identical identity.
			// Candidate counts are single digits, so the pairwise scan is free.
			samePhysicalFile := false
			for _, prior := range outInfos {
				if os.SameFile(prior, info) {
					samePhysicalFile = true
					break
				}
			}
			if samePhysicalFile {
				continue
			}
			outInfos = append(outInfos, info)
			out = append(out, attachedTraceReferencedCandidate{display: token, resolved: resolved})
		}
	}
	return out
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
	// Marker discovery is a single-physical-file streaming optimization. A
	// bundle (including a sibling-promoted artifact universe) must keep the
	// indexed path so marker timestamps, physical line coordinates, and clock
	// admission are interpreted with the same provenance as the recipe itself.
	if tracequery.TracePathRequiresCompositeIndex(path) {
		return types.ToolResult{}, false
	}
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
	// See maybeLargeRecipeAutoWindow: raw marker scanning cannot preserve a
	// composite artifact universe's virtual lines or calibrated clock mapping.
	if tracequery.TracePathRequiresCompositeIndex(path) {
		return types.ToolResult{}, false
	}
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
	payloadBytes, marshalFailure := traceQueryMarshalPayload(t.Name(), payload)
	if marshalFailure != nil {
		return *marshalFailure, true
	}
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

// traceQueryPayloadRefAdvisory is display-only soft guidance (H17, customer
// audit 2026-07-03) appended to every body-level payload_ref line so the model
// treats the ref as an audit/verification artifact instead of a next hop.
// Verified 2026-07-03: in read mode read_file CAN open the payload blob (it is
// an absolute path under WorkDir; resolveReadFilePath applies no repo-scope or
// .codrax exclusion outside the triage stages, only the bounded whole-read
// byte wall) — so the hint deliberately says "prefer views" rather than
// falsely claiming the file is unreadable.
const traceQueryPayloadRefAdvisory = "(audit artifact for verification; drill down with further trace_query views instead of reading this payload directly)"

// writeTraceQueryPayloadRefLine is the single formatting chokepoint for the
// body-level payload_ref line of trace_query summaries. Keeping one emitter
// means the H17 advisory cannot drift across the recipe-discovery, auto-window
// and main summary surfaces.
func writeTraceQueryPayloadRefLine(b *strings.Builder, payloadRef string, trailingBlankLine bool) {
	if strings.TrimSpace(payloadRef) == "" {
		return
	}
	fmt.Fprintf(b, "payload_ref=%s %s\n", sanitizeForBanner(payloadRef), traceQueryPayloadRefAdvisory)
	if trailingBlankLine {
		b.WriteString("\n")
	}
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
	writeTraceQueryPayloadRefLine(&b, payloadRef, false)
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
	writeTraceQueryPayloadRefLine(&b, payloadRef, false)
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

// traceQueryNormalizedWindow keeps the caller's requested computation window
// physically separate from the tiny boundary-tolerance window used only by
// event_search lookup.  Causal/statistical views, their selected_window wire
// fields, durations, percentages, and final reports always consume the exact
// Requested* pair.  This prevents a lookup convenience from silently becoming
// a metric denominator.
type traceQueryNormalizedWindow struct {
	RequestedStart      float64
	RequestedEnd        float64
	LookupStart         float64
	LookupEnd           float64
	NormalizationCaveat string
	LookupCaveat        string
}

func normalizedTraceQueryWindow(p traceQueryParams) traceQueryNormalizedWindow {
	window := traceQueryNormalizedWindow{
		RequestedStart: p.TimeStart.Seconds(),
		RequestedEnd:   p.TimeEnd.Seconds(),
		LookupStart:    p.TimeStart.Seconds(),
		LookupEnd:      p.TimeEnd.Seconds(),
	}
	startTol := p.TimeStart.QueryToleranceSeconds()
	endTol := p.TimeEnd.QueryToleranceSeconds()
	if p.TimeStart.Set() && startTol > 0 {
		window.LookupStart -= startTol
		if window.LookupStart < 0 {
			window.LookupStart = 0
		}
	}
	if p.TimeEnd.Set() && endTol > 0 {
		window.LookupEnd += endTol
	}
	var parts []string
	if p.TimeStart.Set() {
		parts = append(parts, fmt.Sprintf("time_start=%s normalized=%.6f", sanitizeForBanner(p.TimeStart.Raw()), p.TimeStart.Seconds()))
	}
	if p.TimeEnd.Set() {
		parts = append(parts, fmt.Sprintf("time_end=%s normalized=%.6f", sanitizeForBanner(p.TimeEnd.Raw()), p.TimeEnd.Seconds()))
	}
	if traceSecondNeedsNormalizationNote(p.TimeStart) || traceSecondNeedsNormalizationNote(p.TimeEnd) {
		window.NormalizationCaveat = "trace timestamp strings were normalized to seconds: " + strings.Join(parts, ", ")
	}
	if startTol > 0 || endTol > 0 {
		window.LookupCaveat = fmt.Sprintf(
			"trace_timestamp_lookup_window requested_window=%.6f..%.6f matching_window=%.6f..%.6f matching_window_policy=lookup_only_boundary_tolerance query_tolerance_seconds=start±%.9f/end±%.9f; causal/statistical selected_window metrics, durations, percentages, and reports always use requested_window",
			window.RequestedStart, window.RequestedEnd, window.LookupStart, window.LookupEnd, startTol, endTol)
	}
	return window
}

// traceQueryAnnotateLookupWindowContract mirrors the event_search lookup
// window contract onto typed observations.  Exact key/value notes let report
// and audit consumers distinguish the user's requested bounds from the wider
// matching bounds without parsing caveat prose.
func traceQueryAnnotateLookupWindowContract(records []types.ObservationRecord, window traceQueryNormalizedWindow) {
	if len(records) == 0 || window.LookupCaveat == "" {
		return
	}
	requested := fmt.Sprintf("requested_window=%.6f..%.6f", window.RequestedStart, window.RequestedEnd)
	matching := fmt.Sprintf("matching_window=%.6f..%.6f", window.LookupStart, window.LookupEnd)
	for i := range records {
		records[i].RichNotes = append(records[i].RichNotes,
			requested,
			matching,
			"matching_window_policy=lookup_only_boundary_tolerance",
			"metric_denominator_window=requested_window",
		)
	}
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

// writeTraceWindowSweepSummary renders the window_sweep advisory sections:
// ranked hotspot sub-windows first, then the compact coverage table. All
// rows are drill-down guidance only (§4.7 red line: nothing here is a hard
// classification).
func writeTraceWindowSweepSummary(b *strings.Builder, sweep *tracequery.WindowSweepResult) {
	b.WriteString("## Window sweep\n")
	fmt.Fprintf(b, "window=%.6f..%.6f bucket_ms=%g bucket_count=%d rank_basis=%s hotspots=%d coverage_rows=%d",
		sweep.Window.StartTs, sweep.Window.EndTs, sweep.BucketMs, sweep.BucketCount, sanitizeForBanner(sweep.RankBasis), len(sweep.Hotspots), len(sweep.Coverage))
	if sweep.TargetPID > 0 {
		fmt.Fprintf(b, " target_pid=%d", sweep.TargetPID)
	}
	if sweep.CoverageFolded {
		fmt.Fprintf(b, " coverage_folded=true fold_span=%d", sweep.CoverageFoldSpan)
	}
	b.WriteString("\n")
	for _, hotspot := range sweep.Hotspots {
		fmt.Fprintf(b, "- hotspot rank=%d window=%.6f..%.6f sched_switches=%d wakeups=%d d_state_entries=%d irq_entries=%d trace_marks=%d target_pid_switches=%d suggested_views=%s\n",
			hotspot.Rank, hotspot.StartTs, hotspot.EndTs, hotspot.SchedSwitches, hotspot.SchedWakeups, hotspot.DStateEntries, hotspot.IRQEntries, hotspot.TraceMarks, hotspot.TargetPIDSwitches,
			sanitizeForBanner(strings.Join(hotspot.SuggestedViews, ",")))
	}
	for _, row := range sweep.Coverage {
		fmt.Fprintf(b, "- coverage window=%.6f..%.6f buckets=%d sched_switches=%d wakeups=%d d_state_entries=%d irq_entries=%d trace_marks=%d target_pid_switches=%d\n",
			row.StartTs, row.EndTs, row.Buckets, row.SchedSwitches, row.SchedWakeups, row.DStateEntries, row.IRQEntries, row.TraceMarks, row.TargetPIDSwitches)
	}
	for _, caveat := range sweep.Caveats {
		fmt.Fprintf(b, "- window_sweep_caveat=%s\n", sanitizeForBanner(caveat))
	}
	b.WriteString("\n")
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
	for i, source := range result.TraceArtifacts {
		if i >= 8 {
			fmt.Fprintf(&b, "trace_artifacts_omitted=%d see=payload_ref\n", len(result.TraceArtifacts)-i)
			break
		}
		fmt.Fprintf(&b, "trace_artifact kind=%s source=%s virtual_line_base=%d local_lines=%d events=%d time_domain=%s canonical_domain=%s alignment=%s calibrated=%t causal_compatible=%t bytes=%d isolation_reason=%s\n",
			sanitizeForBanner(source.Kind), sanitizeForBanner(filepath.Base(source.SourcePath)), source.VirtualLineBase, source.LocalLineCount, source.EventCount,
			sanitizeForBanner(source.TimeDomain), sanitizeForBanner(source.CanonicalTimeDomain), sanitizeForBanner(source.ClockAlignment), source.ClockCalibrated, source.CausalCompatible, source.SourceBytes, sanitizeForBanner(source.IsolationReason))
	}
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
	writeTraceQueryPayloadRefLine(&b, payloadRef, true)
	if result.WindowStats != nil && (len(result.WindowStats.StateDrilldownPlan) > 0 || result.WindowStats.IdleWholeWindowSleepers != nil) {
		// The state-first handoff is an action-bearing root-cause surface, not
		// inventory detail. Render it before the verbose wakeup/rank sections so
		// StoreBlob's bounded head preview cannot hide it in the omitted middle.
		b.WriteString("## State drilldown\n")
		writeTraceStateDrilldownSummary(&b, result.WindowStats.StateDrilldownPlan, result.WindowStats.IdleWholeWindowSleepers)
		b.WriteString("\n")
	}
	if len(result.SpanWindows) > 0 {
		b.WriteString("## Span windows\n")
		for _, span := range result.SpanWindows {
			fmt.Fprintf(&b, "- span %s %q %.6f..%.6f kind=%s duration=%.3fms source=%s lines=%d-%d\n",
				traceThreadLabel(span.Thread), span.Name, span.StartTs, span.EndTs, firstNonEmptyTraceString(span.Kind, "sync"), span.DurationMs, traceQuerySourceBasename(span.SourcePath), span.StartLine, span.EndLine)
		}
		b.WriteString("\n")
	}
	if result.WindowSweep != nil {
		writeTraceWindowSweepSummary(&b, result.WindowSweep)
	}
	if result.FrameRootCauseBundle != nil {
		writeTraceFrameRootCauseBundleSummary(&b, result.FrameRootCauseBundle)
	}
	if result.WakeupChain != nil {
		b.WriteString("## Wakeup chain\n")
		// P0-E CHAIN-PATH (ledger §22.1): one line per REAL branch (each a
		// true waker chain ending at the target); the flattened cross-branch
		// walk survives only for identity-less legacy results. The line
		// prefix stays byte-identical so the legacy text-parse lane keeps
		// reconstructing one candidate per line.
		if branches := traceQueryWakeupChainBranches(*result.WakeupChain); len(branches) > 0 {
			for _, br := range branches {
				fmt.Fprintf(&b, "- wakeup_chain path=%s\n", sanitizeForBanner(br.Path))
			}
		} else if path := traceQueryWakeupChainPath(*result.WakeupChain); path != "" {
			fmt.Fprintf(&b, "- wakeup_chain path=%s\n", sanitizeForBanner(path))
		}
		// RN-14a (§7.9): the via_thread verdict is a dedicated stanza so the
		// on-chain-root-cause vs scheduling-contention ruling survives
		// paraphrase.
		if via := result.WakeupChain.ViaThread; via != nil {
			fmt.Fprintf(&b, "- %s\n", sanitizeForBanner(via.Summary))
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
		rankRows := make([]tracequery.RootCauseRankItem, 0, len(result.RootCauseRank.Items)+len(result.RootCauseRank.AbsorbedItems))
		rankRows = append(rankRows, result.RootCauseRank.Items...)
		rankRows = append(rankRows, result.RootCauseRank.AbsorbedItems...)
		for _, item := range rankRows {
			occurrenceWindows := traceQueryOccurrenceWindowsCompact(item.OccurrenceWindows, 4)
			projection := traceQueryProjectedActualFields(item.ProjectedImpactMs, item.CumulativeImpactMs, item.ActualImpactMs, item.ActualTotalMs, item.ActualStartTs, item.ActualEndTs)
			backgroundRank := ""
			if item.BackgroundRank > 0 && traceQueryRootCauseItemIsSemanticSpanWork(item.Type) {
				// DCS E6 (ledger §23.1 ruling ③): the typed non-chain board
				// position rides the same text face the model consumes rank
				// rows from — the mention gate reads background_rank<=3.
				backgroundRank = fmt.Sprintf(" background_rank=%d", item.BackgroundRank)
			}
			reconciliation := ""
			if item.AbsorbedRankRows > 0 && strings.TrimSpace(item.RankFamilyKey) != "" {
				reconciliation = fmt.Sprintf(" absorbed_rank_rows=%d rank_family_key=%s", item.AbsorbedRankRows, sanitizeForBanner(item.RankFamilyKey))
			} else if item.AbsorbedByRankFamily {
				reconciliation = " absorbed_by_rank_family=true absorbed_into=" + sanitizeForBanner(item.AbsorbedIntoFamily)
			}
			physicalSource := ""
			if strings.TrimSpace(item.PhysicalSourcePath) != "" {
				physicalSource = " physical_source=" + traceQuerySourceBasename(item.PhysicalSourcePath)
			}
			fmt.Fprintf(&b, "- rank=%d tier=%s%s type=%s thread=%s window=%.6f..%.6f occurrence_windows=%s dominant_state=%s running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms impact=%.3fms cumulative_impact=%.3fms effective_impact=%.3fms target_impact=%.3fms%s score=%.3f confidence=%.2f lines=%d-%d source=%s%s causality=%s chain_relevance=%s chain_depth=%d overlap=%.3fms edge_count=%d nearest_chain=%s nearest_window=%.6f..%.6f span=%s perf_context=%s perf_contexts=%s%s — %s\n",
				item.Rank, item.Tier, backgroundRank, item.Type, traceThreadLabel(item.Thread), item.StartTs, item.EndTs,
				occurrenceWindows, sanitizeForBanner(item.DominantState), item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs,
				item.ImpactMs, item.CumulativeImpactMs, traceQueryRootCauseEffectiveImpact(item), item.TargetImpactMs, projection, item.Score, item.Confidence,
				item.LineStart, item.LineEnd, item.Source, physicalSource, sanitizeForBanner(item.Causality), sanitizeForBanner(item.ChainRelevance), item.ChainDepth, item.OverlapMs, item.EdgeCount,
				traceThreadLabel(item.NearestChainThread), item.NearestChainWindow.StartTs, item.NearestChainWindow.EndTs, traceQueryRootCauseSpanCompact(item), traceQueryPerfContextCompact(item.PerfContext), traceQueryPerfRoleContextsCompact(item.PerfContexts, 4), reconciliation, item.Summary)
			writeTraceRootCausePerfRoles(&b, item.Rank, item.PerfContexts)
			writeTraceRootCauseBlockingDetail(&b, item)
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
		if head := result.Timeline.HeadState; head != nil {
			fmt.Fprintf(&b, "- head_state status=%s boundary=%.6f state=%s actual_start=%.6f source_line=%d reason=%s\n",
				sanitizeForBanner(head.Status), head.BoundaryTs, sanitizeForBanner(string(head.State)), head.ActualStartTs, head.SourceLine, sanitizeForBanner(head.Reason))
		}
		writeTraceTimelineStateTotals(&b, result.Timeline.Intervals)
		for i, it := range result.Timeline.Intervals {
			if i >= 12 {
				fmt.Fprintf(&b, "... omitted %d interval(s); see payload_ref (state_total rows above already sum ALL intervals)\n", len(result.Timeline.Intervals)-i)
				break
			}
			fmt.Fprintf(&b, "- %s %.6f..%.6f %.3fms lines=%d-%d wake_line=%d%s\n",
				it.State, it.StartTs, it.EndTs, it.DurationMs, it.StartLine, it.EndLine, it.WakeupLine, traceQueryIntervalActualFields(it))
		}
		b.WriteString("\n")
	}
	if result.SchedulerLatency != nil {
		b.WriteString("## Scheduler latency stats\n")
		fmt.Fprintf(&b, "- count=%d mean=%.3fms p50=%.3fms p95=%.3fms p99=%.3fms max=%.3fms\n",
			result.SchedulerLatency.Count, result.SchedulerLatency.MeanMs, result.SchedulerLatency.P50Ms, result.SchedulerLatency.P95Ms, result.SchedulerLatency.P99Ms, result.SchedulerLatency.MaxMs)
		for _, item := range result.SchedulerLatency.Items {
			fmt.Fprintf(&b, "- runnable_wait %s %.6f..%.6f duration=%.3fms cpu=%d core_class=%s freq=%dkHz weighted_freq=%dkHz observed_max_freq=%dkHz%s prio=%d/%s same_cpu_busy=%.3fms same_cpu_idle=%.3fms other_cpu_idle=%.3fms high_prio_running=%.3fms high_prio_overlap=%.3fms system_or_kernel_running=%.3fms system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d lines=%d-%d — %s\n",
				traceThreadLabel(item.Thread), item.StartTs, item.EndTs, item.DurationMs, item.CPU, sanitizeForBanner(item.CoreClass), item.Frequency, item.WeightedFrequency, item.ObservedMaxFrequency, traceFrequencySampleDetail(item.FrequencySample), item.Priority, item.PriorityClass, item.SameCPUBusyMs, item.SameCPUIdleMs, item.OtherCPUIdleMs, item.HighPriorityRunningMs, item.HighPriorityRunningOverlapMs, item.SystemOrKernelRunningMs, item.SystemOrKernelRunningOverlapMs, item.SystemOrKernelCompetitorCount, item.StartLine, item.EndLine, item.Summary)
		}
		for _, caveat := range result.SchedulerLatency.Caveats {
			fmt.Fprintf(&b, "- scheduler_latency_caveat=%s\n", caveat)
		}
		b.WriteString("\n")
	}
	if result.WindowStats != nil {
		b.WriteString("## Window stats\n")
		if coverage := result.WindowStats.SchedulerHeadCoverage; coverage != nil {
			fmt.Fprintf(&b, "- scheduler_head_coverage status=%s boundary=%.6f reason=%s missing_cpus=%d:%v missing_threads=%d:%v\n",
				sanitizeForBanner(coverage.Status), coverage.BoundaryTs, sanitizeForBanner(coverage.Reason), coverage.MissingCPUCount, coverage.MissingCPUs, coverage.MissingThreadCount, coverage.MissingThreadPIDs)
		}
		for _, cpu := range result.WindowStats.CPU {
			fmt.Fprintf(&b, "- cpu=%d core_class=%s busy=%.3fms idle=%.3fms freq=%d%s\n", cpu.CPU, sanitizeForBanner(cpu.CoreClass), cpu.BusyMs, cpu.IdleMs, cpu.Frequency, traceFrequencyResidencySummary(cpu.FrequencyResidency))
		}
		for _, core := range result.WindowStats.CoreTopology {
			fmt.Fprintf(&b, "- core_class=%s cpus=%v busy=%.3fms idle=%.3fms runnable_wait=%.3fms high_prio_running=%.3fms system_or_kernel_running=%.3fms max_freq=%dkHz source=%s signal=%s\n",
				sanitizeForBanner(core.Class), core.CPUs, core.BusyMs, core.IdleMs, core.RunnableWaitMs, core.HighPriorityRunMs, core.SystemOrKernelRunningMs, core.MaxFrequency, sanitizeForBanner(core.TopologySource), sanitizeForBanner(core.ComputeSupplySignal))
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
			fmt.Fprintf(&b, "- io_latency dev=%s op=%s sector=%d len=%d duration=%.3fms issue=%s complete=%s source=%s lines=%d-%d\n",
				io.Dev, io.Op, io.Sector, io.Len, io.DurationMs, traceThreadLabel(io.IssueThread), traceThreadLabel(io.CompleteThread), traceQuerySourceBasename(io.SourcePath), io.IssueLine, io.CompleteLine)
		}
		for _, limit := range result.WindowStats.CPUFrequencyLimits {
			fmt.Fprintf(&b, "- cpu_frequency_limit cpu=%d min=%dkHz max=%dkHz count=%d line=%d\n",
				limit.CPU, limit.MinFrequency, limit.MaxFrequency, limit.Count, limit.Line)
		}
		for _, pressure := range result.WindowStats.CPUPressure {
			// CMP-9: the per-CPU runnable-wait sum is cross-thread cpu·ms; the
			// density (value/wall window) is the cross-window-comparable form.
			density := ""
			if pressure.RunnableWaitDensity > 0 {
				density = fmt.Sprintf(" runnable_density=%.2f", pressure.RunnableWaitDensity)
			}
			fmt.Fprintf(&b, "- cpu_pressure cpu=%d runnable_wait=%.3fms%s running=%.3fms high_prio_running=%.3fms high_prio_overlap=%.3fms system_or_kernel_running=%.3fms system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d runnable_events=%d%s\n",
				pressure.CPU, pressure.RunnableWaitMs, density, pressure.RunningMs, pressure.HighPriorityRunningMs, pressure.HighPriorityRunningOverlapMs,
				pressure.SystemOrKernelRunningMs, pressure.SystemOrKernelRunningOverlapMs, pressure.SystemOrKernelCompetitorCount,
				pressure.RunnableEvents, traceOverlapCompetitorsDetail(pressure.OverlapCompetitors))
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
			fmt.Fprintf(&b, "- trace_span %s %q category=%s subcategory=%s semantic_class=%s kind=%s duration=%.3fms source=%s lines=%d-%d\n",
				traceThreadLabel(span.Thread), span.Name, sanitizeForBanner(span.Category), sanitizeForBanner(span.Subcategory), sanitizeForBanner(span.SemanticClass), firstNonEmptyTraceString(span.Kind, "sync"), span.DurationMs, traceQuerySourceBasename(span.SourcePath), span.StartLine, span.EndLine)
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
		for _, delta := range result.WindowStats.CounterDeltas {
			source := filepath.Base(strings.TrimSpace(delta.SourcePath))
			if source == "." || source == "" {
				source = "unknown"
			}
			tag := ""
			if delta.TrailingTag != "" {
				tag = " trailing_tag=" + sanitizeForBanner(delta.TrailingTag)
			}
			fmt.Fprintf(&b, "- counter_delta owner_scope=%s owner_pid=%d name=%q%s source=%s baseline=%s unit=%s first=%g last=%g min=%g max=%g delta=%+g samples=%d lines=%d-%d local_lines=%d-%d emitter=%s\n",
				sanitizeForBanner(delta.OwnerScope), delta.OwnerPID, delta.Name, tag, sanitizeForBanner(source),
				sanitizeForBanner(delta.Baseline), sanitizeForBanner(delta.UnitStatus), delta.First, delta.Last, delta.Min, delta.Max, delta.Delta,
				delta.Samples, delta.FirstLine, delta.LastLine, delta.FirstLocalLine, delta.LastLocalLine, traceThreadLabel(delta.Thread))
		}
		if quality := result.WindowStats.CounterQuality; quality != nil {
			fmt.Fprintf(&b, "- counter_quality rows=%d valid_identity=%d numeric=%d invalid=%d non_numeric=%d derived_invalid_series=%d series=%d series_status=%s published=%d suppressed=%d truncated=%d series_budget=%d budget_exceeded=%t overflow_rows=%d baseline_policy=%s unit_policy=%s\n",
				quality.Rows, quality.ValidIdentityRows, quality.NumericRows, quality.InvalidRows, quality.NonNumericRows, quality.DerivedInvalidSeries,
				quality.TotalSeries, sanitizeForBanner(quality.TotalSeriesStatus), quality.PublishedSeries, quality.SuppressedSeries, quality.TruncatedSeries,
				quality.SeriesBudget, quality.SeriesBudgetExceeded, quality.OverflowRows,
				sanitizeForBanner(quality.BaselinePolicy), sanitizeForBanner(quality.UnitPolicy))
			for _, issue := range quality.Issues {
				var samples []string
				for _, sample := range issue.Samples {
					source := filepath.Base(strings.TrimSpace(sample.SourcePath))
					if source == "." || source == "" {
						source = "unknown"
					}
					samples = append(samples, fmt.Sprintf("%s:%d(owner=%s,name=%q,value=%q,tag=%s)",
						sanitizeForBanner(source), sample.LocalLine, sanitizeForBanner(sample.OwnerRaw), sample.Name, sample.Value, sanitizeForBanner(sample.TrailingTag)))
				}
				fmt.Fprintf(&b, "  counter_issue reason=%s count=%d samples=%s\n",
					sanitizeForBanner(issue.Reason), issue.Count, strings.Join(samples, ","))
			}
		}
		for _, burst := range result.WindowStats.IRQBursts {
			fmt.Fprintf(&b, "- irq_burst cpu=%d irq=%d name=%s count=%d span=%.3fms duration_basis=inventory_not_active lines=%d-%d\n",
				burst.CPU, burst.IRQ, burst.Name, burst.Count, burst.SpanMs, burst.LineStart, burst.LineEnd)
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
		writeTraceTopIOInodes(&b, result.WindowStats.TopIOInodes)
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
			fmt.Fprintf(&b, "- irq_activity kind=%s cpu=%d core_class=%s vector=%d name=%s count=%d paired=%d active=%.3fms max=%.3fms source=%s lines=%d-%d — %s\n",
				sanitizeForBanner(irq.Kind), irq.CPU, sanitizeForBanner(irq.CoreClass), irq.Vector, sanitizeForBanner(irq.Name), irq.Count, irq.PairedCount, irq.ActiveMs, irq.MaxActiveMs, traceQuerySourceBasename(irq.SourcePath), irq.LineStart, irq.LineEnd, sanitizeForBanner(irq.Summary))
		}
		for _, soft := range result.WindowStats.SoftIRQActivity {
			fmt.Fprintf(&b, "- softirq_activity kind=%s cpu=%d core_class=%s vector=%d name=%s count=%d paired=%d active=%.3fms max=%.3fms source=%s lines=%d-%d — %s\n",
				sanitizeForBanner(soft.Kind), soft.CPU, sanitizeForBanner(soft.CoreClass), soft.Vector, sanitizeForBanner(soft.Name), soft.Count, soft.PairedCount, soft.ActiveMs, soft.MaxActiveMs, traceQuerySourceBasename(soft.SourcePath), soft.LineStart, soft.LineEnd, sanitizeForBanner(soft.Summary))
		}
		for _, ipi := range result.WindowStats.IPIActivity {
			fmt.Fprintf(&b, "- ipi_activity kind=%s cpu=%d core_class=%s name=%s count=%d paired=%d active=%.3fms max=%.3fms target_mask=%s target_cpus=%s source=%s lines=%d-%d — %s\n",
				sanitizeForBanner(ipi.Kind), ipi.CPU, sanitizeForBanner(ipi.CoreClass), sanitizeForBanner(ipi.Name), ipi.Count, ipi.PairedCount, ipi.ActiveMs, ipi.MaxActiveMs, sanitizeForBanner(ipi.TargetMask), traceIntList(ipi.TargetCPUs), traceQuerySourceBasename(ipi.SourcePath), ipi.LineStart, ipi.LineEnd, sanitizeForBanner(ipi.Summary))
		}
		for _, work := range result.WindowStats.WorkqueueActivity {
			fmt.Fprintf(&b, "- workqueue_activity %s work=%s function=%s count=%d paired=%d unpaired_start=%d unpaired_done=%d ambiguous_cohorts=%d pairing_suppressed=%d duration=%.3fms max=%.3fms source=%s lines=%d-%d — %s\n",
				traceThreadLabel(work.Thread), sanitizeForBanner(work.Work), sanitizeForBanner(work.Function), work.Count, work.PairedCount,
				work.UnpairedStartCount, work.UnpairedDoneCount, work.AmbiguousCohortCount, work.PairingSuppressedCount,
				work.DurationMs, work.MaxLatencyMs, traceQuerySourceBasename(work.SourcePath), work.LineStart, work.LineEnd, sanitizeForBanner(work.Summary))
		}
		for _, fence := range result.WindowStats.DMAFenceActivity {
			fmt.Fprintf(&b, "- dma_fence_activity %s driver=%s timeline=%s context=%s seqno=%s count=%d paired=%d unpaired_start=%d unpaired_done=%d ambiguous_cohorts=%d pairing_suppressed=%d wait=%.3fms max=%.3fms source=%s lines=%d-%d — %s\n",
				traceThreadLabel(fence.Thread), sanitizeForBanner(fence.Driver), sanitizeForBanner(fence.Timeline), sanitizeForBanner(fence.Context), sanitizeForBanner(fence.Seqno), fence.Count, fence.PairedCount,
				fence.UnpairedStartCount, fence.UnpairedDoneCount, fence.AmbiguousCohortCount, fence.PairingSuppressedCount,
				fence.WaitMs, fence.MaxWaitMs, traceQuerySourceBasename(fence.SourcePath), fence.LineStart, fence.LineEnd, sanitizeForBanner(fence.Summary))
		}
		for _, accounting := range result.WindowStats.SchedStatAccounting {
			fmt.Fprintf(&b, "- sched_stat_accounting %s kind=%s count=%d delay=%.3fms max_delay=%.3fms runtime=%.3fms max_runtime=%.3fms lines=%d-%d — %s\n",
				traceThreadLabel(accounting.Thread), sanitizeForBanner(accounting.Kind), accounting.Count, accounting.TotalDelayMs, accounting.MaxDelayMs, accounting.TotalRuntimeMs, accounting.MaxRuntimeMs, accounting.LineStart, accounting.LineEnd, sanitizeForBanner(accounting.Summary))
		}
		if result.WindowStats.SupplyPressureSummary != nil {
			supply := result.WindowStats.SupplyPressureSummary
			// CMP-9: always carry the wall window + normalized density next to
			// the cross-thread sum (density = value/window ≈ avg queue depth).
			density := ""
			if supply.WindowMs > 0 {
				density = fmt.Sprintf(" window_ms=%.3f pressure_density=%.2f", supply.WindowMs, supply.PressureDensity)
			}
			fmt.Fprintf(&b, "- supply_pressure signal=%s cpu_pressure=%.3fms%s runnable=%.3fms high_prio=%.3fms system_or_kernel_running=%.3fms system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d sched_stat_wait=%.3fms sched_stat_iowait=%.3fms sched_stat_blocked=%.3fms ipi_events=%d ipi_active=%.3fms low_freq_cpus=%v clock_set_rate=%d thermal=%d ddr=%d l3=%d throughput=%d lines=%d-%d — %s\n",
				sanitizeForBanner(supply.Signal), supply.CPUPressureMs, density, supply.RunnableWaitMs, supply.HighPriorityRunningMs,
				supply.SystemOrKernelRunningMs, supply.SystemOrKernelRunningOverlapMs, supply.SystemOrKernelCompetitorCount,
				supply.SchedStatWaitMs, supply.SchedStatIOWaitMs, supply.SchedStatBlockedMs, supply.IPIEventCount, supply.IPIActiveMs, supply.LowFrequencyCPUs, supply.ClockSetRateCount, supply.ThermalEventCount, supply.DDREventCount, supply.L3EventCount, supply.ThroughputEventCount, supply.LineStart, supply.LineEnd, sanitizeForBanner(supply.Summary))
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
			fmt.Fprintf(&b, "- compute_supply %s state=%s cpu=%d core_class=%s duration=%.3fms freq=%dkHz weighted_freq=%dkHz observed_max_freq=%dkHz%s busy=%.3fms idle=%.3fms runnable_wait=%.3fms high_prio_running=%.3fms high_prio_overlap=%.3fms system_or_kernel_running=%.3fms system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d verdict=%s confidence=%.2f lines=%d-%d — %s\n",
				traceThreadLabel(supply.Thread), supply.State, supply.CPU, sanitizeForBanner(supply.CoreClass), supply.DurationMs, supply.Frequency, supply.WeightedFrequency, supply.ObservedMaxFrequency, traceFrequencySampleDetail(supply.FrequencySample), supply.CPUBusyMs, supply.CPUIdleMs, supply.RunnableWaitMs, supply.HighPriorityRunningMs, supply.HighPriorityRunningOverlapMs,
				supply.SystemOrKernelRunningMs, supply.SystemOrKernelRunningOverlapMs, supply.SystemOrKernelCompetitorCount,
				supply.Verdict, supply.Confidence, supply.LineStart, supply.LineEnd, supply.Summary)
		}
		// CMP-8/CMP-10 advisory sections render on the canonical window_stats
		// view ONLY, and LAST in the stanza: composite views
		// (root_cause_rank / frame_root_cause_bundle / trace_perf_bundle)
		// already sit at the blob-preview width cliff, and fattening them
		// pushed the pinned state-first handoff sections (state_drilldown /
		// trace_mark_category) into the truncated middle. The typed fields
		// still ride Result.WindowStats in the JSON payload for every view;
		// the guidance surfaces (state_first_hint, recommended_sections)
		// steer follow-ups to view=window_stats where these sections render.
		if strings.EqualFold(strings.TrimSpace(result.View), "window_stats") {
			writeTraceProcessDomainCensus(&b, result.WindowStats.ProcessDomainCensus)
			writeTraceCPUOccupancy(&b, result.WindowStats.CPUOccupancy)
			writeTraceComputeSupplyBalance(&b, result.WindowStats.ComputeSupplyBalance)
			writeTraceClusterFrequencyCeilings(&b, result.WindowStats.ClusterFrequencyCeilings)
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
			// RCX① (§12.3 ruling 1): the typed drill-debt verdict rides the
			// blocking row text for counterpart-lane rows.
			if item.DrillStatus != "" {
				blockingSemantics += " drill_status=" + sanitizeForBanner(item.DrillStatus)
			}
			// G1 跨车道对账 (§27.2, 2026-07-09): an absorbed row tells the
			// model on the text face too — the same events already count
			// inside the named rank family row; do not re-list them as
			// separate causes. Row publication itself is unchanged (观测照发
			// 不删).
			if item.AbsorbedByRankFamily {
				blockingSemantics += " absorbed_by_rank_family=true absorbed_into=" + sanitizeForBanner(item.AbsorbedIntoFamily)
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
			fmt.Fprintf(&b, "- line=%d ts=%.6f type=%s thread=%s%s%s%s%s raw=%s\n",
				ev.Line,
				ev.Ts,
				ev.Type,
				traceThreadLabel(tracequery.ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}),
				traceEventProvenanceDetail(ev),
				traceEventPriorityDetail(ev),
				traceEventSchedulerDetail(ev),
				traceEventResourceDetail(ev),
				raw,
			)
		}
		writeTraceCPUFrequencyCensus(&b, result.CPUFrequencyCensus)
		b.WriteString("\n")
	}
	if len(result.EvidencePack) > 0 {
		b.WriteString("## Evidence pack\n")
		for i, fact := range result.EvidencePack {
			if i >= 16 {
				fmt.Fprintf(&b, "... omitted %d fact(s); see payload_ref\n", len(result.EvidencePack)-i)
				break
			}
			fmt.Fprintf(&b, "- %s %s %s lines=%d-%d%s confidence=%.2f — %s\n",
				fact.Subject, fact.Predicate, fact.Object, fact.LineStart, fact.LineEnd, traceEvidenceFactProvenanceDetail(fact), fact.Confidence, fact.Summary)
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
	fmt.Fprintf(b, "- target=%s window=%.6f..%.6f root_causes=%d blocking=%d io_bursts=%d block_inode=%d irq=%d softirq=%d workqueue=%d dma_fence=%d trace_categories=%d async_file=%d\n",
		traceThreadLabel(bundle.Target), bundle.Window.StartTs, bundle.Window.EndTs,
		traceQueryBundleRootCauseCount(bundle), traceQueryBundleBlockingCount(bundle), len(bundle.IOBurstEpisodes), len(bundle.BlockIOByInode), len(bundle.IRQActivity), len(bundle.SoftIRQActivity), len(bundle.WorkqueueActivity), len(bundle.DMAFenceActivity), len(bundle.TraceMarkCategories), len(bundle.AsyncFileWork))
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
	writeTraceFrameBundleTopBlocking(b, bundle)
	// RCX③ (§12.3-1 ③): the typed causal skeleton sits in the head region,
	// adjacent to top_blocking, so the model-facing spine stays inside the
	// head-24KB anchor zone. Structure only — the model configures the prose.
	writeTraceFrameBundleSkeleton(b, bundle)
	if bundle.WakeupChain != nil {
		// P0-E CHAIN-PATH (ledger §22.1): per-branch true paths; flattened
		// walk only for identity-less legacy results.
		if branches := traceQueryWakeupChainBranches(*bundle.WakeupChain); len(branches) > 0 {
			for _, br := range branches {
				fmt.Fprintf(b, "- bundle_wakeup_chain path=%s\n", sanitizeForBanner(br.Path))
			}
		} else if path := traceQueryWakeupChainPath(*bundle.WakeupChain); path != "" {
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
	// Keep the semantic trace-mark handoff in the bundle head.  Composite
	// summaries can exceed the blob preview and lose the middle Window Stats
	// rows; counts alone are not enough for the model to recover the category
	// or async-file work identity.
	for i, category := range bundle.TraceMarkCategories {
		if i >= 2 {
			break
		}
		fmt.Fprintf(b, "- bundle_trace_mark_category category=%s subcategory=%s count=%d total=%.3fms top_span=%s lines=%d-%d\n",
			sanitizeForBanner(category.Category), sanitizeForBanner(category.Subcategory), category.Count, category.TotalMs, sanitizeForBanner(category.TopSpan), category.LineStart, category.LineEnd)
	}
	for i, work := range bundle.AsyncFileWork {
		if i >= 2 {
			break
		}
		fmt.Fprintf(b, "- bundle_async_file_work thread=%s category=%s span=%s duration=%.3fms lines=%d-%d — %s\n",
			traceThreadLabel(work.Thread), sanitizeForBanner(work.Category), sanitizeForBanner(work.Name), work.DurationMs, work.LineStart, work.LineEnd, sanitizeForBanner(work.Summary))
	}
	for _, caveat := range bundle.Caveats {
		fmt.Fprintf(b, "- bundle_caveat=%s\n", sanitizeForBanner(caveat))
	}
	b.WriteString("\n")
}

// writeTraceFrameBundleTopBlocking (Q4-K 修1 + RCX①, ledger §12.1/§12.3-1):
// the bundle HEAD region names the top blocking candidates with their typed
// contention semantics (kind / owner / holder_site / duration) instead of a
// bare blocking=%d count, so the lock evidence stays inside the head-24KB
// anchor zone even when the full blocking section falls into the blob's
// middle blind spot. Selection prefers on_chain rows whose BlockingKind is
// resolved (stable partition, top-K ≤ 3). When the LARGEST measured blocking
// impact is an undrilled-but-known counterpart, a dedicated disclosure line
// surfaces the drill debt (§12.3 ruling 1 exploration face).
func writeTraceFrameBundleTopBlocking(b *strings.Builder, bundle *tracequery.FrameRootCauseBundle) {
	if b == nil || bundle == nil || bundle.CriticalBlocking == nil || len(bundle.CriticalBlocking.Items) == 0 {
		return
	}
	items := bundle.CriticalBlocking.Items
	preferred := make([]tracequery.CriticalBlockingCandidate, 0, len(items))
	rest := make([]tracequery.CriticalBlockingCandidate, 0, len(items))
	for _, item := range items {
		if item.ChainRelevance == "on_chain" && item.BlockingKind != "" && (item.Peer.PID > 0 || strings.TrimSpace(item.Peer.Comm) != "") {
			preferred = append(preferred, item)
		} else {
			rest = append(rest, item)
		}
	}
	ordered := append(preferred, rest...)
	for i, item := range ordered {
		if i >= 3 {
			break
		}
		parts := []string{
			"type=" + sanitizeForBanner(item.Type),
			"thread=" + traceThreadLabel(item.Thread),
		}
		if item.BlockingKind != "" {
			parts = append(parts, "kind="+sanitizeForBanner(item.BlockingKind))
		}
		if item.Peer.PID > 0 || strings.TrimSpace(item.Peer.Comm) != "" {
			parts = append(parts, "owner="+traceThreadLabel(item.Peer))
		}
		if item.HolderSite != "" {
			parts = append(parts, "holder_site="+sanitizeForBanner(item.HolderSite))
		}
		// BLOCKFROM (§27.4 G13): the waiter-side call site, same shape.
		if item.BlockingFromSite != "" {
			parts = append(parts, "blocking_from_site="+sanitizeForBanner(item.BlockingFromSite))
		}
		// P0-E2a: surface the counterpart-resolution origin so the head can say
		// whether the named holder/peer came straight from the payload or from
		// the waiter's wakeup edge (and preserve the phantom payload tid).
		if src := firstNonEmpty(item.HolderSource, item.PeerSource); src != "" {
			parts = append(parts, "counterpart_source="+sanitizeForBanner(src))
		}
		if item.OwnerTidRaw > 0 {
			parts = append(parts, fmt.Sprintf("owner_tid_raw=%d(payload; not in trace)", item.OwnerTidRaw))
		}
		// LCK-2 (§18.E/§18.E.1): identity unification / process-level identity.
		if item.HolderNsUnification != "" {
			parts = append(parts, "holder_ns_unification="+sanitizeForBanner(item.HolderNsUnification))
		}
		if item.HolderHostProcess != "" {
			parts = append(parts, "holder_host_process="+sanitizeForBanner(item.HolderHostProcess))
		}
		if item.WaitObject != "" {
			parts = append(parts, "wait_object="+sanitizeForBanner(item.WaitObject))
		}
		parts = append(parts, fmt.Sprintf("duration=%.3fms", item.DurationMs))
		parts = append(parts, "chain_relevance="+sanitizeForBanner(item.ChainRelevance))
		if item.DrillStatus != "" {
			parts = append(parts, "drill_status="+sanitizeForBanner(item.DrillStatus))
		}
		fmt.Fprintf(b, "- bundle_top_blocking %s\n", strings.Join(parts, " "))
	}
	// RCX① head disclosure: largest MEASURED blocking impact left undrilled.
	var largest *tracequery.CriticalBlockingCandidate
	for i := range items {
		if items[i].DrillStatus == "" {
			continue
		}
		if largest == nil || items[i].DurationMs > largest.DurationMs {
			largest = &items[i]
		}
	}
	if largest != nil && largest.DrillStatus == tracequery.DrillStatusUndrilledPeerKnown {
		fmt.Fprintf(b, "- bundle_largest_impact_undrilled: %s %.3fms peer=%s — the biggest measured blocking impact names a counterpart this report never examined; drill the peer before concluding\n",
			traceThreadLabel(largest.Thread), largest.DurationMs, traceThreadLabel(largest.Peer))
	}
}

// writeTraceFrameBundleSkeleton (RCX③, ledger §12.1/§12.3-1 item ③): renders the
// typed model-facing causal skeleton in the bundle HEAD region — an ordered,
// layer-tagged spine (target dominant wait → direct explainer → upstream chain
// head → supply background) the model narrates. Each node carries its measured
// ms, causal layer, drill status, and counterpart source. It is a STRUCTURE the
// model configures prose over, never a system-authored verdict
// (feedback_no_system_backfill). Nodes are rendered in causal-layer order and
// MUST NOT be re-sorted by the incommensurable per-layer ms values (§12.3-1 ②).
func writeTraceFrameBundleSkeleton(b *strings.Builder, bundle *tracequery.FrameRootCauseBundle) {
	if b == nil || bundle == nil || bundle.Skeleton == nil || len(bundle.Skeleton.Nodes) == 0 {
		return
	}
	skel := bundle.Skeleton
	// Header is a single compact teaching line; per-node lines carry ONLY the
	// typed fields (no prose notes — the typed struct keeps the Note for the P0-A
	// projection consumer, but the head text stays lean so the card never evicts
	// tail window-stats content from the 24KB preview budget).
	fmt.Fprintf(b, "- bundle_causal_skeleton target=%s nodes=%d — causal spine (layer order = causal role, NOT a ranking); narrate in prose, never re-sort layers by ms\n",
		traceThreadLabel(skel.Target), len(skel.Nodes))
	for _, node := range skel.Nodes {
		parts := []string{
			"layer=" + sanitizeForBanner(string(node.Layer)),
		}
		if node.Thread.PID > 0 || strings.TrimSpace(node.Thread.Comm) != "" {
			parts = append(parts, "thread="+traceThreadLabel(node.Thread))
		}
		if node.State != "" {
			parts = append(parts, "state="+sanitizeForBanner(node.State))
		}
		parts = append(parts, fmt.Sprintf("measured=%.3fms", node.MeasuredMs))
		if node.HolderSite != "" {
			parts = append(parts, "holder_site="+sanitizeForBanner(node.HolderSite))
		}
		// BLOCKFROM (§27.4 G13): the waiter-side call site, same shape.
		if node.BlockingFromSite != "" {
			parts = append(parts, "blocking_from_site="+sanitizeForBanner(node.BlockingFromSite))
		}
		if node.DrillStatus != "" {
			parts = append(parts, "drill_status="+sanitizeForBanner(node.DrillStatus))
		}
		if node.CounterpartSource != "" {
			parts = append(parts, "counterpart_source="+sanitizeForBanner(node.CounterpartSource))
		}
		b.WriteString("  bundle_skeleton_node " + strings.Join(parts, " ") + "\n")
	}
}

func traceQueryPriorityRuleBanner(flavor string) string {
	switch tracequery.TraceFlavor(strings.TrimSpace(flavor)) {
	case tracequery.TraceFlavorHarmonyHitrace:
		return "harmony_larger_numeric_higher_1_40_CFS_41_159_RT_gt159_raw"
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

// writeTraceRootCauseBlockingDetail (Q4-A 修1 + RCX① + Q4-B annotation face):
// one indented detail line under a rank row whenever the typed lock-lane /
// drill-debt / inherited-annotation fields are populated — the LLM-facing rank
// text names the contention kind, the counterpart, the holder site, the drill
// verdict, and the annotation-only inherited value without touching the
// pinned main-row format.
func writeTraceRootCauseBlockingDetail(b *strings.Builder, item tracequery.RootCauseRankItem) {
	if b == nil {
		return
	}
	if item.BlockingKind == "" && item.DrillStatus == "" && item.InheritedTargetBlockedMs <= 0 && !item.PriorityInversionLockDominated {
		return
	}
	parts := []string{fmt.Sprintf("rank_blocking_detail rank=%d", item.Rank)}
	if item.BlockingKind != "" {
		parts = append(parts, "kind="+sanitizeForBanner(item.BlockingKind))
	}
	// BLK-2 P3b: name the row's lock orientation explicitly on the text face —
	// this rank row's SUBJECT is the holder and the peer= token that follows is
	// the blocked WAITER (reading peer= as "the holder" here was the BLK 双向锁
	// misdirection entry point).
	if item.SubjectIsLockHolder {
		parts = append(parts, "subject_role=holder")
	}
	if item.BlockingPeer.PID > 0 || strings.TrimSpace(item.BlockingPeer.Comm) != "" {
		parts = append(parts, "peer="+traceThreadLabel(item.BlockingPeer))
	}
	if item.HolderSite != "" {
		parts = append(parts, "holder_site="+sanitizeForBanner(item.HolderSite))
	}
	// BLOCKFROM (§27.4 G13): the waiter-side call site, same shape.
	if item.BlockingFromSite != "" {
		parts = append(parts, "blocking_from_site="+sanitizeForBanner(item.BlockingFromSite))
	}
	if item.HolderSource != "" {
		parts = append(parts, "holder_source="+sanitizeForBanner(item.HolderSource))
	}
	if item.OwnerTidRaw > 0 {
		parts = append(parts, fmt.Sprintf("owner_tid_raw=%d(payload; not in trace)", item.OwnerTidRaw))
	}
	// LCK-2 (§18.E/§18.E.1): identity unification / process-level identity.
	if item.HolderNsUnification != "" {
		parts = append(parts, "holder_ns_unification="+sanitizeForBanner(item.HolderNsUnification))
	}
	if item.HolderHostProcess != "" {
		parts = append(parts, "holder_host_process="+sanitizeForBanner(item.HolderHostProcess))
	}
	// P0-E 锁车道修2 (§24.9-C F2): the text face names the hand-over and the
	// withdrawn-attribution witnesses next to the holder identity.
	if len(item.HolderHandoff) >= 2 {
		parts = append(parts, "holder_handoff="+sanitizeForBanner(strings.Join(item.HolderHandoff, " --> "))+"(final holder only; not whole-span)")
	}
	if item.HolderSelfContradiction != "" {
		parts = append(parts, "holder_self_contradiction="+sanitizeForBanner(item.HolderSelfContradiction))
	}
	if item.DrillStatus != "" {
		parts = append(parts, "drill_status="+sanitizeForBanner(item.DrillStatus))
	}
	if item.InheritedTargetBlockedMs > 0 {
		parts = append(parts, fmt.Sprintf("inherited_target_blocked=%.3fms(annotation_only)", item.InheritedTargetBlockedMs))
	}
	if item.PriorityInversionLockDominated {
		parts = append(parts, "priority_inversion_lock_dominated=true")
	}
	fmt.Fprintf(b, "  %s\n", strings.Join(parts, " "))
}

func writeTraceIPCEdges(b *strings.Builder, edges []tracequery.IPCEdge) {
	for i, edge := range edges {
		if i >= 12 {
			fmt.Fprintf(b, "... omitted %d IPC edge(s); see payload_ref\n", len(edges)-i)
			break
		}
		iface := ""
		if strings.TrimSpace(edge.Interface) != "" {
			iface = " interface=" + sanitizeForBanner(edge.Interface)
		}
		fmt.Fprintf(b, "- ipc transaction=%d %s -> %s send_line=%d receive_line=%d latency=%.3fms reply=%d flags=%s code=%s oneway=%t sync_like=%t blocking_candidate=%t confidence=%.2f%s\n",
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
			iface,
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

// traceQueryRootCauseItemIsSemanticSpanWork mirrors the engine's precise
// typed identity of a semantic span-work rank row (DCS/SEM-LEAD): these type
// tokens are minted only by the engine's semantic span lane.
func traceQueryRootCauseItemIsSemanticSpanWork(typ string) bool {
	switch typ {
	case "jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause":
		return true
	default:
		return false
	}
}

func traceQueryRootCauseEffectiveImpact(item tracequery.RootCauseRankItem) float64 {
	if item.PeriodicSource {
		// VS-1 (§7.8): a periodic-source row's discounted attribution is
		// authoritative even at 0 — the cumulative/impact fallbacks would
		// republish the raw in-period sleep as effective impact.
		return item.EffectiveImpactMs
	}
	if item.Type == "running" {
		// §20.2 (2026-07-07) mirror of the engine's rootCauseEffectiveImpactMs:
		// a causal-lane running row's attribution is its ELIMINABLE supply-fold
		// deficit, authoritative even at 0 — the cumulative fallback below
		// must never republish the raw wall clock as effective attribution
		// (the raw stays on the cumulative/impact display notes).
		return item.EffectiveImpactMs
	}
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

// writeTraceTopIOInodes renders the INODE (§28.6) whole-window (dev,inode)
// IO frequency block: one line per group, count-first order, plus the
// mandatory trailing group-total disclosure line (truncation is never
// silent). Latency fields follow the wall-clock red line: max_latency is the
// largest single event, top_threads lists per-thread WITHIN-thread sums.
func writeTraceTopIOInodes(b *strings.Builder, top *tracequery.TopIOInodeStats) {
	if top == nil {
		return
	}
	for _, g := range top.Groups {
		// P3-1 (复核收尾): the banner word is events= — the same-screen legacy
		// "- file_io ... count=" line counts ACTIVITY only, while this group
		// total spans the whole IO family (activity + completions + page-cache
		// adds/deletes); same word + different caliber is the G7
		// word/value-same-source hazard family, so the banner reuses the
		// engine Summary's events= word. The typed note key stays "count"
		// (Unit=events already disambiguates the typed lane).
		fmt.Fprintf(b, "- top_io_inode dev=%s inode=%s events=%d reads=%d writes=%d completions=%d bytes=%d page_cache_adds=%d page_cache_deletes=%d max_latency=%.3fms threads=%d%s entry=%s lines=%d-%d\n",
			sanitizeForBanner(firstNonEmptyTraceString(g.Dev, "unknown")),
			sanitizeForBanner(g.Inode),
			g.Count,
			g.ReadCount,
			g.WriteCount,
			g.CompletionCount,
			g.Bytes,
			g.PageCacheAdds,
			g.PageCacheDeletes,
			g.MaxLatencyMs,
			g.ThreadCount,
			traceTopIOInodeThreadDetail(g.TopThreadLatencies),
			sanitizeForBanner(firstNonEmptyTraceString(g.EntryName, "unknown")),
			g.LineStart,
			g.LineEnd,
		)
	}
	fmt.Fprintf(b, "- top_io_inode_groups total=%d shown=%d", top.TotalGroups, len(top.Groups))
	if top.UnidentifiedEvents > 0 {
		fmt.Fprintf(b, " unidentified_io_events=%d", top.UnidentifiedEvents)
	}
	b.WriteString("\n")
}

// traceTopIOInodeThreadDetail renders the banner form of the per-thread
// latency contributor roster (" top_threads=comm-pid:1.234ms|…"); empty when
// no member carried latency.
func traceTopIOInodeThreadDetail(items []tracequery.TopIOInodeThreadLatency) string {
	roster := traceTopIOInodeThreadRoster(items)
	if roster == "" {
		return ""
	}
	return " top_threads=" + roster
}

// traceTopIOInodeThreadRoster renders the bare per-thread latency contributor
// roster ("comm-pid:1.234ms|comm-pid:0.500ms") shared by the banner line and
// the typed observation note. Each value is that ONE thread's within-thread
// latency sum — never a cross-thread aggregate.
func traceTopIOInodeThreadRoster(items []tracequery.TopIOInodeThreadLatency) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s:%.3fms", traceThreadLabel(item.Thread), item.TotalLatencyMs))
	}
	return strings.Join(parts, "|")
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
	fmt.Fprintf(b, "- storage_latency layer=%s event=%s dev=%s inode=%s name=%s op=%s thread=%s count=%d paired=%d unpaired_start=%d unpaired_done=%d ambiguous_cohorts=%d pairing_suppressed=%d max_latency=%.3fms avg_latency=%.3fms bytes=%d example=%s source=%s lines=%d-%d — %s\n",
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
		item.AmbiguousCohortCount,
		item.PairingSuppressedCount,
		item.MaxLatencyMs,
		item.AvgLatencyMs,
		item.Bytes,
		sanitizeForBanner(item.Example),
		traceQuerySourceBasename(item.SourcePath),
		item.LineStart,
		item.LineEnd,
		sanitizeForBanner(item.Summary),
	)
}

func traceQuerySourceBasename(path string) string {
	source := filepath.Base(strings.TrimSpace(path))
	if source == "" || source == "." {
		source = "unknown"
	}
	return sanitizeForBanner(source)
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
	fmt.Fprintf(b, "- thread_cpu_load thread=%s running=%.3fms runnable=%.3fms high_prio_running=%.3fms system_or_kernel_running=%.3fms cpu=%d core_class=%s freq=%dkHz prio=%d/%s lines=%d-%d — %s\n",
		traceThreadLabel(item.Thread),
		item.RunningMs,
		item.RunnableWaitMs,
		item.HighPriorityRunningMs,
		item.SystemOrKernelRunningMs,
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

func writeTraceStateDrilldownSummary(b *strings.Builder, steps []tracequery.StateDrilldownStep, idleFold *tracequery.IdleWholeWindowSleeperFold) {
	summaryCap := traceQueryWidthStateDrilldownSummaryCap()
	for i, step := range steps {
		if i >= summaryCap {
			fmt.Fprintf(b, "- state_drilldown_omitted count=%d see payload_ref\n", len(steps)-summaryCap)
			break
		}
		fmt.Fprintf(b, "- state_drilldown rank=%d thread=%s state=%s impact=%.3fms total=%.3fms source=%s chain_required=%t recursive=%t window_proportion=%.4f significant=%t recommended_views=%s lines=%d-%d — %s\n",
			step.Rank, traceThreadLabel(step.Thread), sanitizeForBanner(step.State), step.ImpactMs, step.TotalMs, sanitizeForBanner(step.Source),
			step.ChainRequired, step.Recursive, step.WindowProportion, step.Significant, sanitizeForBanner(strings.Join(step.RecommendedViews, ",")), step.LineStart, step.LineEnd, sanitizeForBanner(step.Summary))
	}
	writeTraceIdleWholeWindowSleeperFold(b, idleFold)
}

// writeTraceIdleWholeWindowSleeperFold renders the one-line fold for top_sleep
// candidates the drilldown plan dropped because they slept through the whole
// selected window (typically idle service threads — audio sinks, DNS
// watchers, FFRT workers — 15+ impact=window rows drowned a customer's 101ms
// state_drilldown surface, berlin.systrace 2026-07-03). One folded line keeps
// the absence auditable. The wording is a NEUTRAL fact statement (QF2): the
// signal (sleep spanning the window) cannot distinguish a parked service
// thread from a victim blocked whole-window, so the line must not assert
// "not root-cause evidence" — the query-pinned target thread is exempted
// from the fold upstream, but unpinned victims can still land here.
func writeTraceIdleWholeWindowSleeperFold(b *strings.Builder, fold *tracequery.IdleWholeWindowSleeperFold) {
	if fold == nil || fold.Count <= 0 {
		return
	}
	fmt.Fprintf(b, "- state_drilldown_idle_folded count=%d threads=%s — whole-window sleepers (no in-window scheduling activity)\n",
		fold.Count, sanitizeForBanner(strings.Join(fold.Threads, ",")))
}

func traceKnownCPU(known bool, cpu int) string {
	if !known {
		return ""
	}
	return strconv.Itoa(cpu)
}

// traceFrequencySampleDetail renders the R5e typed frequency-provenance marker
// (" frequency_sample=nearest_fallback") or nothing when in-segment samples
// backed the weighted frequency.
func traceFrequencySampleDetail(sample string) string {
	if strings.TrimSpace(sample) == "" {
		return ""
	}
	return " frequency_sample=" + sanitizeForBanner(sample)
}

// traceOverlapCompetitorsDetail renders the displacement-evidenced competitor
// list (§7.30.2 R5g): threads whose running overlapped another thread's
// runnable wait on the CPU, with the overlapped ms only.
func traceOverlapCompetitorsDetail(items []tracequery.ThreadDuration) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for i, td := range items {
		if i >= 4 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s/%.3fms", traceThreadLabel(td.Thread), td.DurationMs))
	}
	return " overlap_competitors=" + sanitizeForBanner(strings.Join(parts, ","))
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
	fmt.Fprintf(b, "- runnable_context thread=%s runnable=%.3fms cpu=%d core_class=%s freq=%dkHz same_cpu_busy=%.3fms same_cpu_idle=%.3fms other_cpu_idle=%.3fms high_prio_running=%.3fms high_prio_overlap=%.3fms system_or_kernel_running=%.3fms system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d top_background_threads=%s top_background_process=%s constraint=%s verdict=%s confidence=%.2f lines=%d-%d — %s\n",
		traceThreadLabel(item.Thread),
		item.RunnableWaitMs,
		item.CPU,
		sanitizeForBanner(item.CoreClass),
		item.Frequency,
		item.SameCPUBusyMs,
		item.SameCPUIdleMs,
		item.OtherCPUIdleMs,
		item.HighPriorityRunningMs,
		item.HighPriorityRunningOverlapMs,
		item.SystemOrKernelRunningMs,
		item.SystemOrKernelRunningOverlapMs,
		item.SystemOrKernelCompetitorCount,
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

// writeTraceProcessDomainCensus renders the WSR §8 b3 pid-scoped
// process-domain census lane: the query target's process aggregated from the
// full pre-truncation running buckets, so threads= here is the honest census
// caliber (every thread of the process observed in the window scope), unlike
// the process_cpu_load rows whose threads= counts only the surviving display
// roster. Per-thread running is plain ms (one thread's cross-CPU segments
// never overlap in wall time); running_total is cross-thread cpu·ms (CMP-3).
// Roster overflow follows the PTS fold discipline: count + aggregate.
func writeTraceProcessDomainCensus(b *strings.Builder, census *tracequery.ProcessDomainCensus) {
	if census == nil {
		return
	}
	summary := ""
	if strings.TrimSpace(census.Summary) != "" {
		summary = " — " + sanitizeForBanner(census.Summary)
	}
	fmt.Fprintf(b, "- process_domain_census(进程域普查) process=%s threads=%d running_threads=%d running_total=%.3fcpu·ms cpus=%s core_classes=%s lines=%d-%d%s\n",
		traceThreadLabel(census.Process), census.ThreadCount, census.RunningThreadCount, census.TotalRunningMs,
		traceIntList(census.CPUs), sanitizeForBanner(strings.Join(census.CoreClasses, ",")), census.LineStart, census.LineEnd, summary)
	for _, td := range census.TopThreads {
		fmt.Fprintf(b, "- process_domain_census_thread %s running=%.3fms cpus=%s core_classes=%s prio=%d/%s lines=%d-%d\n",
			traceThreadLabel(td.Thread), td.RunningMs, traceIntList(td.CPUs), sanitizeForBanner(strings.Join(td.CoreClasses, ",")), td.Priority, sanitizeForBanner(td.PriorityClass), td.LineStart, td.LineEnd)
	}
	if census.FoldedThreadCount > 0 {
		fmt.Fprintf(b, "- process_domain_census_fold remaining_threads=%d running_total=%.3fcpu·ms — top list capped at %d threads; the other %d running threads are folded here, not dropped (其余 %d 线程合计)\n",
			census.FoldedThreadCount, census.FoldedRunningMs, len(census.TopThreads), census.FoldedThreadCount, census.FoldedThreadCount)
	}
	for _, caveat := range census.Caveats {
		fmt.Fprintf(b, "- process_domain_census_caveat=%s\n", sanitizeForBanner(caveat))
	}
}

// writeTraceCPUOccupancy renders the CMP-8 (§7.1) occupancy-side
// decomposition: who consumed the CPUs in the selected window. Every duration
// on this surface is cpu-time (cpu·ms) clipped to the wall window — the
// header line carries the unit contract so cross-CPU sums are never read as
// wall-clock elapsed time.
func writeTraceCPUOccupancy(b *strings.Builder, occ *tracequery.CPUOccupancyStats) {
	if occ == nil {
		return
	}
	fmt.Fprintf(b, "- cpu_occupancy window_ms=%.3f unit=cpu·ms (running cpu-time clipped to the wall window; cross-CPU sums may exceed window_ms)\n", occ.WindowMs)
	for _, item := range occ.TopThreads {
		fmt.Fprintf(b, "- cpu_occupancy_thread %s running=%.3fcpu·ms cpus=%s core_classes=%s prio=%d/%s lines=%d-%d\n",
			traceThreadLabel(item.Thread), item.RunningMs, traceIntList(item.CPUs), sanitizeForBanner(strings.Join(item.CoreClasses, ",")), item.Priority, sanitizeForBanner(item.PriorityClass), item.LineStart, item.LineEnd)
	}
	for _, item := range occ.TopProcesses {
		fmt.Fprintf(b, "- cpu_occupancy_process %s threads=%d running=%.3fcpu·ms top_thread=%s %.3fcpu·ms cpus=%s core_classes=%s lines=%d-%d\n",
			traceThreadLabel(item.Process), item.ThreadCount, item.RunningMs, traceThreadLabel(item.TopThread), item.TopThreadMs, traceIntList(item.CPUs), sanitizeForBanner(strings.Join(item.CoreClasses, ",")), item.LineStart, item.LineEnd)
	}
	for _, item := range occ.PerCPUTop {
		var tops []string
		for _, td := range item.Top {
			tops = append(tops, fmt.Sprintf("%s %.3fcpu·ms", traceThreadLabel(td.Thread), td.DurationMs))
		}
		fmt.Fprintf(b, "- cpu_occupancy_cpu cpu=%d core_class=%s busy=%.3fms idle=%.3fms top=[%s]\n",
			item.CPU, sanitizeForBanner(item.CoreClass), item.BusyMs, item.IdleMs, sanitizeForBanner(strings.Join(tops, "; ")))
	}
	for _, band := range occ.PriorityBands {
		fmt.Fprintf(b, "- cpu_occupancy_priority_band band=%s high_priority=%t running=%.3fcpu·ms threads=%d\n",
			sanitizeForBanner(band.Band), band.HighPriority, band.RunningMs, band.ThreadCount)
	}
	for _, caveat := range occ.Caveats {
		fmt.Fprintf(b, "- cpu_occupancy_caveat=%s\n", sanitizeForBanner(caveat))
	}
}

// writeTraceComputeSupplyBalance renders the CMP-10 (§7.4) supply-side
// ledger as its own 算力供给 (compute supply) stanza: frequency-weighted
// delivered compute vs nominal capacity plus the typed gap decomposition.
func writeTraceComputeSupplyBalance(b *strings.Builder, bal *tracequery.ComputeSupplyBalance) {
	if bal == nil {
		return
	}
	fmt.Fprintf(b, "- compute_supply_balance(算力供给) window_ms=%.3f cpus=%d nominal=%.3fcpu·ms delivered=%.3fcpu·ms supply_ratio=%.3f low_freq_loss=%.3fcpu·ms idle_mismatch=%.3fms(wall) core_limited≈%.3fcpu·ms — %s\n",
		bal.WindowMs, bal.CPUCount, bal.NominalCapacityMs, bal.DeliveredComputeMs, bal.SupplyRatio, bal.LowFrequencyLossMs, bal.IdleMismatchMs, bal.CoreLimitedMs, sanitizeForBanner(bal.Summary))
	for _, per := range bal.PerCPU {
		fmt.Fprintf(b, "- compute_supply_balance_cpu cpu=%d core_class=%s running=%.3fcpu·ms delivered=%.3fcpu·ms low_freq_loss=%.3fcpu·ms max_freq=%dkHz freq_known=%t\n",
			per.CPU, sanitizeForBanner(per.CoreClass), per.RunningMs, per.DeliveredComputeMs, per.LowFrequencyLossMs, per.MaxFrequencyKHz, per.FrequencyKnown)
	}
	for _, caveat := range bal.Caveats {
		fmt.Fprintf(b, "- compute_supply_balance_caveat=%s\n", sanitizeForBanner(caveat))
	}
}

// writeTraceClusterFrequencyCeilings renders the CFC (§7.10 VS-2c) window
// ceilings snapshot as ONE soft display line beside the compute-supply
// stanza (F2(a) — the snapshot's only display consumer): the deduped
// per-cluster "fastest this cluster could go in this window" with its VS-2b
// ladder provenance (limit = cpufreq policy ceiling, observed = highest
// governing cpu_frequency sample). Soft display only — no gate reads it —
// and the line is omitted entirely when the window minted no ceiling. Gated
// to view=window_stats with the other CMP-8/CMP-10 stanzas (width cliff).
func writeTraceClusterFrequencyCeilings(b *strings.Builder, ceilings []tracequery.ClusterFrequencyCeiling) {
	if len(ceilings) == 0 {
		return
	}
	// Engine order is small → middle → big → unclassified; the display leads
	// with the reference (big) cluster and keeps the unclassified pool last.
	parts := make([]string, 0, len(ceilings))
	var unclassified []string
	for i := len(ceilings) - 1; i >= 0; i-- {
		c := ceilings[i]
		if c.CoreClass == "" {
			unclassified = append(unclassified, fmt.Sprintf("unclassified=%.1fGHz(%s)", float64(c.FmaxKHz)/1e6, c.Source))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%.1fGHz(%s)", c.CoreClass, float64(c.FmaxKHz)/1e6, c.Source))
	}
	parts = append(parts, unclassified...)
	fmt.Fprintf(b, "- cluster_frequency_ceilings %s\n", sanitizeForBanner(strings.Join(parts, " ")))
}

func writeTraceProcessCPULoad(b *strings.Builder, item tracequery.ProcessCPULoadSummary) {
	fmt.Fprintf(b, "- process_cpu_load process=%s threads=%d running=%.3fms runnable=%.3fms high_prio_running=%.3fms system_or_kernel_running=%.3fms top_thread=%s top_thread_ms=%.3fms cpus=%s core_classes=%s lines=%d-%d — %s\n",
		traceThreadLabel(item.Process),
		item.ThreadCount,
		item.RunningMs,
		item.RunnableWaitMs,
		item.HighPriorityRunningMs,
		item.SystemOrKernelRunningMs,
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

// writeTraceCPUFrequencyCensus renders the RFC #71 (§8.2 c4) pre-truncation
// frequency tier ladder next to the truncated Events face. Two short rows:
// the census header (counts, cpu set, distinct tiers, range boundary) and the
// exhaustive ascending khz×rows listing (shared formatter with the evidence
// fact — single home). Only present when the display cap actually hid
// matched cpu_frequency rows.
func writeTraceCPUFrequencyCensus(b *strings.Builder, census *tracequery.CPUFrequencyCensus) {
	if census == nil {
		return
	}
	limitRows := ""
	if census.FrequencyLimitRows > 0 {
		limitRows = fmt.Sprintf(" limit_rows=%d", census.FrequencyLimitRows)
	}
	fmt.Fprintf(b, "- cpu_frequency_census(频点普查) matched_rows=%d displayed_rows=%d%s cpus=%s distinct_khz=%d range=%d..%dkHz lines=%d-%d — census aggregated over ALL matched cpu_frequency rows in the queried window BEFORE the chronological display truncation; the tier ladder below is exhaustive for this window even though the row list above is not\n",
		census.MatchedFrequencyRows,
		census.DisplayedFrequencyRows,
		limitRows,
		traceIntList(census.CPUs),
		len(census.Tiers),
		census.MinKHz,
		census.MaxKHz,
		census.LineStart,
		census.LineEnd,
	)
	fmt.Fprintf(b, "- cpu_frequency_census_tiers khz×rows=%s\n",
		tracequery.FormatCPUFrequencyCensusTiers(census.Tiers, 24))
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
	out := " freq_residency=" + strings.Join(parts, ",")
	if len(items) > 4 {
		// RFC #71 (§8.2 c4): the "+N" fold hides N chronological residency
		// SEGMENTS — on the c4 window the 807000kHz tier lived entirely
		// inside "+26", so the folded line silently misstated the per-cpu
		// ladder boundary. Append the distinct-tier census (count + min..max)
		// computed over the FULL segment list; unfolded lines (≤4 segments)
		// stay byte-identical.
		distinct := map[int]bool{}
		minKHz, maxKHz := 0, 0
		for _, item := range items {
			if item.Frequency <= 0 {
				continue
			}
			distinct[item.Frequency] = true
			if minKHz == 0 || item.Frequency < minKHz {
				minKHz = item.Frequency
			}
			if item.Frequency > maxKHz {
				maxKHz = item.Frequency
			}
		}
		if len(distinct) > 0 {
			out += fmt.Sprintf(" distinct_khz=%d range=%d..%dkHz", len(distinct), minKHz, maxKHz)
		}
	}
	return out
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

func traceEventProvenanceDetail(ev tracequery.EventView) string {
	if strings.TrimSpace(ev.SourcePath) == "" || ev.LocalLine <= 0 {
		return ""
	}
	parts := []string{
		"source=" + filepath.Base(ev.SourcePath),
		fmt.Sprintf("local_line=%d", ev.LocalLine),
	}
	if strings.TrimSpace(ev.TimeDomain) != "" {
		parts = append(parts, "time_domain="+sanitizeForBanner(ev.TimeDomain))
	}
	if ev.SourceTs != 0 && (ev.ClockAligned || ev.SourceTs != ev.Ts) {
		parts = append(parts, fmt.Sprintf("source_ts=%.6f", ev.SourceTs))
	}
	if ev.ClockAligned {
		parts = append(parts, "clock_aligned=true")
	}
	if ev.RawUnavailableReason != "" {
		parts = append(parts, "raw_unavailable="+sanitizeForBanner(ev.RawUnavailableReason))
	}
	return " " + strings.Join(parts, " ")
}

func traceEvidenceFactProvenanceDetail(fact tracequery.EvidenceFact) string {
	if len(fact.SourceSpans) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fact.SourceSpans))
	for _, span := range fact.SourceSpans {
		end := span.LocalLineEnd
		if end <= 0 {
			end = span.LocalLineStart
		}
		location := fmt.Sprintf("%s:%d", filepath.Base(span.SourcePath), span.LocalLineStart)
		if end != span.LocalLineStart {
			location += fmt.Sprintf("-%d", end)
		}
		if span.TimeDomain != "" {
			location += "@" + sanitizeForBanner(span.TimeDomain)
		}
		parts = append(parts, location)
	}
	return " sources=" + strings.Join(parts, ",")
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
	if blk := ev.BlockIOFields; blk != nil {
		if blk.Error != "" {
			parts = append(parts, "block_error="+blk.Error)
		}
		if blk.SrcDev != "" {
			parts = append(parts, fmt.Sprintf("block_src=%s/%d", sanitizeForBanner(blk.SrcDev), blk.SrcSector))
		}
	}
	if ev.SubsystemKind != "" {
		parts = append(parts, "subsystem="+ev.SubsystemKind)
	}
	if ff := ev.FileFields; ff != nil && (ff.Ino != "" || ff.Dev != "" || ff.Entry != "") {
		parts = append(parts, fmt.Sprintf("file_io dev=%s inode=%s name=%s op=%s offset=%d len=%d ret=%d",
			sanitizeForBanner(ff.Dev),
			sanitizeForBanner(ff.Ino),
			sanitizeForBanner(ff.Entry),
			sanitizeForBanner(ff.RW),
			ff.Offset,
			ff.Len,
			ff.Ret))
	}
	if pl := ev.PluginFields; pl != nil {
		if pl.EventName != "" {
			parts = append(parts, "plugin_event="+sanitizeForBanner(pl.EventName))
		}
		if pl.Metric != "" || pl.Value != "" {
			parts = append(parts, fmt.Sprintf("metric=%s value=%s", sanitizeForBanner(pl.Metric), sanitizeForBanner(pl.Value)))
		}
	}
	if ev.Type == tracequery.EventPerfSample {
		pf := ev.PerfFields
		if pf == nil {
			pf = &tracequery.PerfFields{}
		}
		parts = append(parts, fmt.Sprintf("perf_sample pid=%d tid=%d sample_weight=%d event=%s symbol=%s dso=%s source=%s sample_kind=%s symbolization_status=%s cpu_known=%s clock=%s clock_confidence=%s callchain_status=%s callchain=%s",
			pf.PID,
			pf.TID,
			pf.Period,
			sanitizeForBanner(pf.EventName),
			sanitizeForBanner(pf.Symbol),
			sanitizeForBanner(pf.DSO),
			sanitizeForBanner(pf.Source),
			sanitizeForBanner(pf.SampleKind),
			sanitizeForBanner(pf.SymbolizationStatus),
			traceQueryBoolPtrBanner(pf.CPUKnown),
			sanitizeForBanner(pf.Clock),
			sanitizeForBanner(pf.ClockConfidence),
			sanitizeForBanner(pf.CallchainStatus),
			sanitizeForBanner(pf.Callchain)))
	}
	cf := ev.ConstraintFields
	if ev.Type == tracequery.EventCPUConstraint || (cf != nil && (cf.Kind != "" || cf.CPUSetName != "" || len(cf.Allowed) > 0)) {
		if cf == nil {
			cf = &tracequery.ConstraintFields{}
		}
		parts = append(parts, fmt.Sprintf("cpu_constraint target=%s-%d kind=%s allowed_cpus=%s cpuset=%s policy=%s observed_cpu=%d orig_cpu=%d dest_cpu=%d",
			sanitizeForBanner(cf.Comm),
			cf.PID,
			sanitizeForBanner(firstNonEmptyTraceString(cf.Kind, ev.Name)),
			traceIntList(cf.Allowed),
			sanitizeForBanner(cf.CPUSetName),
			sanitizeForBanner(cf.Policy),
			cf.CPU,
			cf.OrigCPU,
			cf.DestCPU))
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

// traceQueryWidthTypedEvidenceFactCap (width governor) bounds published
// evidence-pack rows — deliberately 4x the prose preview's 16-fact cap; the
// full payload remains addressable via the stored payload reference.
// traceQueryWidthTypedFamilyRowCap bounds every other per-family row list.
// Both live accessors are defined in width_governor.go over the
// internal/tool/width single-source table.

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

// traceQueryRootCausePositionWord renders the summary-fallback board position
// (复核 P3-1, 2026-07-09, mirroring the engine's evidenceFromRootCauseRank
// face): a G9-demoted Rank=0 row states it holds no rank seat instead of
// fabricating a "#0" ordinal.
func traceQueryRootCausePositionWord(tier string, rank int) string {
	if rank <= 0 {
		return fmt.Sprintf("%s row (no rank seat)", tier)
	}
	return fmt.Sprintf("%s cause #%d", tier, rank)
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

// traceQueryObservationWindowMsValue renders the CMP-9 typed window_ms rich
// note ("" when the backing window was unbounded — never an estimate).
func traceQueryObservationWindowMsValue(ms float64) string {
	return traceQueryObservationMSValue(ms)
}

// traceQueryObservationDensityValue renders the CMP-9 typed pressure_density
// rich note (value / wall window ≈ average runnable queue depth).
func traceQueryObservationDensityValue(density float64) string {
	if density <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", density)
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

// traceQuerySelectedWindowNoteValue renders the typed selected-window basis
// note value (F1, adversarial review 2026-07-04): the engine's OWN query
// window (q.TimeStart/TimeEnd as carried on the view result), emitted only
// when it is a real two-sided window. The projection compiler anchors its
// 关注窗口 fallback exclusively on this note — never on a record's Span,
// because e.g. a wakeup_causal_aggregate Span is the member-impact envelope
// (FirstTs/LastTs), not the selected window.
func traceQuerySelectedWindowNoteValue(window tracequery.TimeWindow) string {
	if window.StartTs <= 0 || window.EndTs <= window.StartTs {
		return ""
	}
	return traceQueryWindowValue(window.StartTs, window.EndTs)
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

// traceQueryIntervalActualFields appends the dual-ledger actual_* tokens to a
// thread_timeline interval row when the query window cut the segment (E1-a,
// RTC-R1 e1 2026-07-05). Same token family as the root_cause_rank
// projected/actual discipline (traceQueryProjectedActualFields): the primary
// duration on the row stays the clamped in-window figure — the one the
// per-state totals sum — while actual_duration/actual_window disclose the
// full scheduler segment so a window-edge wait cannot be misread as 0ms.
// Clamp detection is the shared tracequery.Interval.WindowClamped predicate
// and the duration is the shared ActualDurationMsResolved fallback accessor —
// the same pair that drives the Summary/evidence-pack regeneration — so all
// three faces disclose (or not) in lockstep. Never read ActualDurationMs bare
// here: a bounds-only interval (actual bounds set, ActualDurationMs zero)
// would print actual_duration=0.000ms while the Summary face publishes the
// bounds-derived value (PTV4 review finding, RTC-R1 2026-07-05).
func traceQueryIntervalActualFields(it tracequery.Interval) string {
	if !it.WindowClamped() {
		return ""
	}
	return fmt.Sprintf(" actual_duration=%.3fms actual_window=%.6f..%.6f",
		it.ActualDurationMsResolved(), it.ActualStartTs, it.ActualEndTs)
}

// traceQueryTimelineActualGuardNote is display-only soft guidance appended to
// the state_totals block whenever any interval in the FULL slice is window-cut
// (RTC-R1 e1 rerun, 2026-07-05): the model summed the actual_duration= values
// of two window-cut running rows and published the sum as in-window CPU while
// the state_total row said running=0.000ms — an arithmetic self-contradiction
// in the final answer. Soft guidance only (red line: precise signals for hard
// gates, no gate on model prose); wording is a single pinned constant so the
// sentence cannot drift.
const traceQueryTimelineActualGuardNote = "- state_totals_note=actual_duration/actual_window values on interval rows measure the full segment, which extends at least partly outside this window; do not add actual_* values into in-window sums — the state_total rows above are the in-window authority\n"

// writeTraceTimelineStateTotals publishes the deterministic per-state totals
// block for the thread_timeline text face (E1-b, RTC-R1 e1 2026-07-05). The
// interval listing below it truncates at 12 rows, so the totals the model
// actually needs (per-state Σms + segment count) must be computed from the
// FULL interval slice — the same slice the JSON payload serialises —
// otherwise the model falls back to hand-pairing an equally truncated
// event_search and publishes wrong sums.
//
// Wall-clock additivity basis: a thread_timeline carries exactly one thread,
// and one thread occupies exactly one scheduler state at a time, so the
// intervals of this single timeline are non-overlapping and their per-state
// wall-clock sums are legal. Summing across threads stays forbidden (wall-
// clock red line; cross-thread layer-Σ was explicitly rejected in the
// projection v3 ruling). DurationMs is the clamped in-window ledger, matching
// both the per-row primary figures and the payload.
func writeTraceTimelineStateTotals(b *strings.Builder, intervals []tracequery.Interval) {
	if len(intervals) == 0 {
		return
	}
	totalMs := map[tracequery.ThreadState]float64{}
	segments := map[tracequery.ThreadState]int{}
	anyWindowCut := false
	for _, it := range intervals {
		totalMs[it.State] += it.DurationMs
		segments[it.State]++
		if it.WindowClamped() {
			anyWindowCut = true
		}
	}
	fmt.Fprintf(b, "- state_totals intervals=%d basis=single_thread_non_overlapping — per-state sums over ALL intervals in this window (interval rows below may be truncated; these totals are not); additive within this one thread only, never sum across threads\n", len(intervals))
	for _, state := range tracequery.ThreadStateUniverse() {
		if segments[state] == 0 {
			continue
		}
		fmt.Fprintf(b, "- state_total state=%s total=%.3fms segments=%d\n", state, totalMs[state], segments[state])
	}
	if anyWindowCut {
		// Guard decision is over the FULL slice (payload basis), not the 12
		// rendered rows: a window-cut interval past the truncation point still
		// carries actual_* tokens in the payload the model may consult. When no
		// interval is window-cut the note is omitted — noise dies at the source.
		b.WriteString(traceQueryTimelineActualGuardNote)
	}
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
			{types.TraceNoteKeySource, resolution.Source},
			{types.TraceNoteKeyWindowSource, resolution.WindowSource},
			{types.TraceNoteKeyWindow, traceQueryTypedTimeWindow(resolution.Window)},
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

	// BLK §15.C ①: ONE physical lock span publishes exactly ONE observation.
	// The resolved lock rank row (subject=holder) and its critical_blocking
	// row (subject=waiter) are two lane views of the SAME folded span
	// (collectBlockingSpanRows feeds both) — publishing both minted two
	// crossed-direction 112.223ms rows out of one monitor_contention span (the
	// q6 E1/E3 "双向锁" shape). The rank record is the single publication (it
	// carries rank/tier/effective/drill and the subject_is_lock_holder lane)
	// and PORTS the twin's display-exclusive note families; the twin is then
	// skipped in the critical_blocking loop below. Typed span identity only
	// (kind + exact line range + unordered thread pair); a rank lock row that
	// never emits (row cap / empty guard) marks nothing, so its twin keeps
	// publishing — the fold never loses the span entirely. The
	// critical_blocking VIEW face (result JSON / banner / peer-chain card) is
	// untouched.
	lockTwins := traceQueryLockContentionTwinIndex(result.CriticalBlocking)
	lockSpanPublishedByRank := map[string]bool{}

	if result.RootCauseRank != nil {
		hasForegroundRootCause := traceQueryRootCauseRankHasForeground(result.RootCauseRank.Items)
		rankRows := make([]tracequery.RootCauseRankItem, 0, len(result.RootCauseRank.Items)+len(result.RootCauseRank.AbsorbedItems))
		rankRows = append(rankRows, result.RootCauseRank.Items...)
		rankRows = append(rankRows, result.RootCauseRank.AbsorbedItems...)
		for i, item := range rankRows {
			if i >= traceQueryWidthTypedFamilyRowCap() {
				break
			}
			rank := item.Rank
			if rank <= 0 && strings.TrimSpace(item.Tier) == "" {
				// Positional backfill for legacy identity-less rows only
				// (hand-built fixtures / lanes that never ran
				// assignRootCauseRanksAndTiers — those carry no Tier). An
				// engine row always wears a Tier, and its Rank=0 is the
				// deliberate G9 no-board-seat signal (target_self_state /
				// data_gap rows — 复核 P1-2 restored periodic-row ordinals,
				// §28.1 user ruling 2026-07-09): resurrecting an ordinal here
				// would re-badge the
				// row on the projection face — 三面同源, the engine ordinal is
				// the only ordinal.
				rank = i + 1
			}
			tier := firstNonEmptyTraceString(item.Tier, traceQueryRootCauseTierLabel(rank))
			if strings.TrimSpace(item.Type) == "" && strings.TrimSpace(item.Summary) == "" {
				continue
			}
			notes := traceQueryTypedOccurrenceWindowRichNotes(item.OccurrenceWindows)
			notes = append(notes, traceQueryTypedPriorityRichNotes(rank, tier, item.Type, item.Source, item.Causality, item.ChainDepth, item.Score, item.ImpactMs, item.CumulativeImpactMs, traceQueryRootCauseEffectiveImpact(item), item.TargetImpactMs, item.ProjectedImpactMs, item.ActualImpactMs, item.ActualTotalMs, item.ActualStartTs, item.ActualEndTs)...)
			if item.BackgroundRank > 0 && traceQueryRootCauseItemIsSemanticSpanWork(item.Type) {
				// DCS E6 double gate (ledger §23.1 ruling ③, 2026-07-08): a
				// NON-CHAIN semantic compile span row publishes its typed
				// background-board position — the skill-tier mention
				// obligation reads background_rank<=3, never a prose count.
				notes = append(notes, fmt.Sprintf("%s=%d", types.TraceNoteKeyBackgroundRank, item.BackgroundRank))
			}
			if item.Type == "running" && traceQueryRootCauseEffectiveImpact(item) <= 0 {
				// §20.2 (2026-07-07): a running row's ZERO attribution is
				// authoritative (eliminable deficit; raw is display-only) and
				// must not vanish off the positive-only note filter — an
				// absent effective note would invite the cumulative fallback
				// (raw) exactly the way §20.2 forbids. Same explicit-zero
				// discipline as the VS-1 periodic lane.
				notes = append(notes, fmt.Sprintf("%s=%.3f", types.TraceNoteKeyEffectiveImpactMS, 0.0))
			}
			notes = append(notes, traceQueryTypedRootCauseStateRichNotes(item)...)
			if item.RunnableBelowRTPreempted {
				// SYM-2 (§24.17 R2, 2026-07-08): the typed below-RT preemption
				// disclosure travels to the projection compile for the 行2
				// 「(优先级低于RT)」 tail — emitted only when the engine minted
				// it (absence never guesses).
				notes = append(notes, types.TraceNoteKeyRunnableBelowRTPreempted+"=true")
			}
			// VS-1 (§7.8): the rank row that carries the discounted effective
			// impact publishes the same typed periodic notes as its backing
			// wakeup_causal_aggregate, so the projection labels the row and the
			// discounted attribution never shows up unexplained.
			notes = append(notes, traceQueryTypedPeriodicSourceRichNotes(item.PeriodicSource, item.DetectedPeriodMs, item.LatenessMs, item.EffectiveImpactMs, false)...)
			// VS-2 (§7.10): the rank row that fronts a running-dominant
			// on-chain node carries the same typed supply-fold notes as its
			// backing causal impact/aggregate, so the projection's decision
			// table works on the lead row too.
			notes = append(notes, traceQueryTypedSupplyFoldRichNotes(item.SupplyFoldBasis, item.SupplyFoldDeficitMs, item.SupplyFoldIdealMs)...)
			notes = append(notes, traceQueryTypedKVNotes([][2]string{
				// F1: root_cause rows have no Span ts at all — the selected
				// query window (RootCauseRankResult.Window = q.TimeStart/TimeEnd)
				// travels via the same typed note as wakeup_causal_aggregate.
				// §21.1 CWD-2 ② (cmp_01 C7 产端半场): a window_stats-derived
				// rank row additionally carries its own typed stats-window
				// identity — when the result envelope has no window, the
				// row-level identity keeps the note alive on the SAME key and
				// format, so the projection's window-base lanes (density /
				// coverage / mixed-window gates) can engage instead of
				// silently projecting a window-1 stats row into a window-2
				// anchored tree. The result window wins byte-identically
				// whenever both exist; no identity anywhere → no note (the
				// display side never guesses a window base).
				{types.TraceNoteKeySelectedWindow, firstNonEmptyTraceString(
					traceQuerySelectedWindowNoteValue(result.RootCauseRank.Window),
					traceQuerySelectedWindowNoteValue(tracequery.TimeWindow{StartTs: item.StatsWindowStartTs, EndTs: item.StatsWindowEndTs}))},
				{types.TraceNoteKeySubjectKind, item.SubjectKind},
				// G2 判据 typed 化 (§27.2/§28.1, 2026-07-09): the trace_gap
				// blind-spot criterion enum (no_sched_data / no_eligible_wait)
				// — set on Type=trace_gap rows only, zero-dropped elsewhere.
				// Hard-consumer tier since Wave-3.2 收尾: the projection
				// compile keys the ◇ row wording fork on it (constant, per the
				// contract-tier change protocol).
				{types.TraceNoteKeyTraceGapKind, item.TraceGapKind},
				// §7.30.3 D3: inversion rows publish the gated composition so
				// the projection can split the composite impact.
				{types.TraceNoteKeyGatedRunnable, traceQueryObservationMSValue(item.GatedRunnableMs)},
				{types.TraceNoteKeyGatedRunningDeficit, traceQueryObservationMSValue(item.GatedRunningDeficitMs)},
				// CAP (§26 C3): the discounted component's capability caliber.
				// CAP-2: the cluster-topology source rides beside it.
				{types.TraceNoteKeyGatedCapability, item.GatedCapabilitySource},
				{types.TraceNoteKeyGatedClusterTopology, item.GatedClusterTopology},
				// §20 E-Gap⑤ (P0-E engine half, 2026-07-07): the gated TOTAL
				// rides the rank row's own note face under the SAME registered
				// key the wakeup_causal_impact face already publishes — single
				// source: the full-precision sum of the two typed components
				// IS the engine's PriorityInversionGatedMs (their sum by
				// construction), formatted once (no round3(a)+round3(b)
				// re-add). Projection-side parsing lands in the P0-A batch.
				{"priority_inversion_gated", traceQueryObservationMSValue(item.GatedRunnableMs + item.GatedRunningDeficitMs)},
				// Q4-A 修1 (§12.3-5): lock-lane rank rows publish the typed
				// contention semantics under the SAME registered blocking-family
				// keys the critical_blocking face uses (zero new hard-consumer
				// keys), plus the RCX① drill verdict and the Q4-B inherited
				// annotation (both NKR display tier, P0-A consumes).
				{types.TraceNoteKeyBlockingKind, item.BlockingKind},
				{types.TraceNoteKeyPeer, traceThreadLabelOptional(item.BlockingPeer)},
				{types.TraceNoteKeyHolderSite, item.HolderSite},
				// BLOCKFROM (§27.4 G13): the waiter-side blocking call site
				// rides next to the holder site, same registered family.
				{types.TraceNoteKeyBlockingFromSite, item.BlockingFromSite},
				// BLK §15.C: the resolved lock rank row's subject IS the holder,
				// so the projection must render a HOLD ("持锁阻塞") and steer the
				// next-step to the holder, never the reversed lock-WAIT the
				// waiter-subject critical_blocking row already publishes for the
				// SAME physical span.
				{types.TraceNoteKeySubjectIsLockHolder, traceQueryTypedBool(item.SubjectIsLockHolder)},
				// P0-E2a: holder-resolution origin + phantom payload tid audit.
				{types.TraceNoteKeyHolderSource, item.HolderSource},
				{types.TraceNoteKeyOwnerTidRaw, traceQueryTypedCount(item.OwnerTidRaw)},
				// LCK-2 (§18.E/§18.E.1): the typed ②×③ identity-unification
				// declaration and the process-level ns-span identity (display
				// tier; the host tgid never rides a peer PID).
				{types.TraceNoteKeyHolderNsUnification, item.HolderNsUnification},
				{types.TraceNoteKeyHolderHostProcess, item.HolderHostProcess},
				// P0-E 锁车道修2 (§24.9-C F2): hand-off chain witness (the
				// resolved holder is the FINAL holder, never whole-span) and
				// the self-contradiction demotion witness.
				{types.TraceNoteKeyHolderHandoff, strings.Join(item.HolderHandoff, " --> ")},
				{types.TraceNoteKeyHolderSelfContradiction, item.HolderSelfContradiction},
				{"drill_status", item.DrillStatus},
				{"inherited_target_blocked_ms", traceQueryObservationMSValue(item.InheritedTargetBlockedMs)},
				{types.TraceNoteKeyChainRelevance, item.ChainRelevance},
				// P0-E CHAIN-PATH (ledger §22.1): the owning branch ordinal of
				// the chain impact this rank row was minted from — the display
				// tree keys its depth attach to (branch, depth); zero-dropped
				// when the row has no single branch identity.
				{types.TraceNoteKeyChainBranch, traceQueryTypedCount(item.ChainBranch)},
				{types.TraceNoteKeyOverlap, traceQueryObservationMSValue(item.OverlapMs)},
				{"edge_count", traceQueryTypedCount(item.EdgeCount)},
				{"nearest_chain_thread", traceThreadLabelOptional(item.NearestChainThread)},
				{types.TraceNoteKeyNearestChainWindow, traceQueryTypedTimeWindow(item.NearestChainWindow)},
				{types.TraceNoteKeySpanName, item.SpanName},
				{types.TraceNoteKeySpanKind, item.SpanKind},
				{types.TraceNoteKeySpanCategory, item.SpanCategory},
				{types.TraceNoteKeySpanSubcategory, item.SpanSubcategory},
				{types.TraceNoteKeySemanticClass, item.SemanticClass},
				// RCM 家族合并族 (§24.7.1/§24.10, 2026-07-08): the engine
				// same-thread family-merge accounting travels on its OWN typed
				// keys — never the folded_* cross-thread wire-cap family (the
				// projection re-materializes these into the isolated
				// FamilyMember* lane, not MergedCount/MergedMaxMS).
				// member_roster joins with " | " (member keys may contain
				// commas); member_sum_ms appears only when the engine set it
				// (published value below the raw Σ — union/max disclosure).
				{types.TraceNoteKeyMemberCount, traceQueryTypedCount(item.MemberCount)},
				{types.TraceNoteKeyMemberMaxMS, traceQueryObservationMSValue(item.MemberMaxMs)},
				{types.TraceNoteKeyMemberMinMS, traceQueryObservationMSValue(item.MemberMinMs)},
				{types.TraceNoteKeyMemberSumMS, traceQueryObservationMSValue(item.MemberSumMs)},
				{types.TraceNoteKeyMemberFoldCaliber, item.MemberFoldCaliber},
				{types.TraceNoteKeyMemberRoster, strings.Join(item.MemberRoster, " | ")},
				// G1 跨车道对账 (§27.2-G1, 2026-07-09): the family-side canonical
				// identity — stamped by the engine only on a family row that
				// absorbed same-(thread,type family,window) critical_blocking
				// rows; the projection joins absorbed rows against it verbatim.
				{types.TraceNoteKeyRankFamilyKey, item.RankFamilyKey},
				// B4 exact cross-type rank-seat reconciliation: the absorbed
				// io_burst_episode keeps publishing as a supporting observation
				// with the same generic marker pair the projection's single G1/B4
				// fold choke point consumes. No display-side re-derivation.
				{types.TraceNoteKeyAbsorbedByRankFamily, traceQueryTypedBool(item.AbsorbedByRankFamily)},
				{types.TraceNoteKeyAbsorbedInto, item.AbsorbedIntoFamily},
				// RCM 区分键族 (§24.9-B F3): typed inode/dev identity from the
				// rank item's own fields — never a Summary re-parse.
				{types.TraceNoteKeyInode, item.Inode},
				{types.TraceNoteKeyDev, item.Dev},
				{"perf_context", traceQueryPerfContextCompact(item.PerfContext)},
				{"perf_contexts", traceQueryPerfRoleContextsCompact(item.PerfContexts, 4)},
			})...)
			// BLK §15.C ①: this rank lock row IS the single publication of its
			// physical span — mark the span so the critical_blocking twin below
			// is skipped, and port the twin's display-exclusive note families
			// (richer form survives, same discipline as the
			// collectBlockingSpanRows fold). The port re-keys the twin's
			// peer-oriented families to subject_* (BLK-2 P1 — the twin's peer
			// is THIS record's subject) and stamps the typed lock_twin_folded
			// coverage witness (BLK-2 P2).
			if item.Type == "blocking_span" && item.BlockingKind != "" {
				if key := traceQueryLockContentionSpanKey(item.BlockingKind, item.LineStart, item.LineEnd, item.Thread, item.BlockingPeer); key != "" {
					lockSpanPublishedByRank[key] = true
					if twin, ok := lockTwins[key]; ok {
						notes = append(notes, traceQueryTypedLockTwinPortNotes(twin)...)
					}
				}
			}
			role := types.AnswerAggregateRolePrincipalAnswer
			grounding := types.ClaimGroundingHard
			provenance := types.ObservationProvenanceObservedDirectCause
			claimKey := "root_cause_" + tier
			predicate := claimKey
			if item.AbsorbedByRankFamily || tier == tracequery.RootCauseTierAbsorbed {
				// One physical segment already owns the principal rank seat. The
				// duplicate observation stays hard-grounded but supporting-only;
				// projection relocation plus the family stanza keep it lossless.
				role = types.AnswerAggregateRoleSupportingCoverage
				provenance = types.ObservationProvenanceArtifactSpan
				claimKey = "root_cause_absorbed"
				predicate = "root_cause_absorbed"
			} else if tier == tracequery.RootCauseTierDataGap {
				// 复核 P3-2 (2026-07-09): a data blind spot is NEVER a
				// principal answer — the role/provenance demote UNCONDITIONALLY
				// (the background arm below requires a foreground root cause,
				// so a foregroundless result used to keep the blind spot at
				// PrincipalAnswer/ObservedDirectCause against 数据盲区非成因).
				// The typed root_cause_data_gap claim/predicate identity is
				// preserved — never blurred into root_cause_background.
				role = types.AnswerAggregateRoleSupportingCoverage
				provenance = types.ObservationProvenanceArtifactSpan
			} else if hasForegroundRootCause && traceQueryRootCauseItemRelevance(item) == "background" {
				role = types.AnswerAggregateRoleSupportingCoverage
				provenance = types.ObservationProvenanceArtifactSpan
				claimKey = "root_cause_background"
				predicate = "root_cause_background"
			}
			out = append(out, types.ObservationRecord{
				// Record identity keys on the POSITION (i+1), not the rank
				// ordinal: G9 rows without a board seat carry Rank=0, and a
				// rank-keyed ID would collide across them. Byte-identical to
				// the pre-G9 shape whenever every row carries an ordinal
				// (there rank == i+1 held by construction).
				ID:              fmt.Sprintf("trace_query:%s#root_cause_rank:%d", scope, i+1),
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
				Summary:         firstNonEmptyTraceString(item.Summary, traceQueryRootCausePositionWord(tier, rank)+" ("+item.Type+")"),
				RichNotes:       notes,
				SupportRefs:     traceQueryObservationSupportRefs(ref, item.LineStart, item.LineEnd),
				ObservedAt:      at,
				Confidence:      item.Confidence,
			})
		}
	}

	for i, fact := range result.EvidencePack {
		if i >= traceQueryWidthTypedEvidenceFactCap() {
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
		// P0-E CHAIN-PATH 根修 (ledger §22.1, 2026-07-09): the wakeup_chain
		// path face publishes ONE record per REAL branch (typed
		// ChainNode.Branch identity; each Object is a true linear waker chain
		// terminating at the target by construction). The retired flattened
		// walk stitched every branch into one pseudo-linear string — huadong_78
		// serialized 29 elements with an oney⇄VSync ×7 ping-pong ladder and
		// fake L26/L27 trunk depths (§24.11 A witness). The legacy record shape
		// survives ONLY for identity-less results (no branch-stamped node —
		// hand-built fixtures / degraded lanes).
		branches := traceQueryWakeupChainBranches(*result.WakeupChain)
		branchPathByID := traceQueryWakeupChainBranchPathByID(branches)
		path := ""
		if len(branches) == 0 {
			path = traceQueryWakeupChainPath(*result.WakeupChain)
		}
		for i, br := range branches {
			if i >= traceQueryWidthTypedFamilyRowCap() {
				break
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#wakeup_chain:path:%d", scope, br.Branch),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
				SourceRef:       ref,
				Span: types.ObservationSpan{
					LineStart: br.LineStart,
					LineEnd:   br.LineEnd,
					StartTs:   result.WakeupChain.Window.StartTs,
					EndTs:     result.WakeupChain.Window.EndTs,
				},
				// Distinct per-branch ClaimKeys: same-key records are the
				// duplicate-publication fold's prey — two branches are two
				// FACTS, never one measurement printed twice.
				ClaimKey:    fmt.Sprintf("wakeup_chain:path#%d", br.Branch),
				Subject:     traceThreadLabel(result.WakeupChain.Target),
				Predicate:   "wakeup_chain",
				Object:      br.Path,
				Summary:     "wakeup_chain path=" + br.Path,
				RichNotes:   traceQueryTypedWakeupBranchPathRichNotes(*result.WakeupChain, br, len(branches)),
				SupportRefs: traceQueryObservationSupportRefs(ref, br.LineStart, br.LineEnd),
				ObservedAt:  at,
				Confidence:  0.82,
			})
		}
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
			if i >= traceQueryWidthTypedFamilyRowCap() {
				break
			}
			if strings.TrimSpace(traceThreadLabel(edge.Waker)) == "" && strings.TrimSpace(traceThreadLabel(edge.Wakee)) == "" {
				continue
			}
			// P0-E CHAIN-PATH: the per-edge path note names the OWNING
			// branch's true path; legacy identity-less edges keep the
			// flattened-walk string.
			edgePath := path
			if p, ok := branchPathByID[edge.Branch]; ok {
				edgePath = p
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
				RichNotes:   traceQueryTypedWakeupEdgeRichNotes(edge, edgePath),
				SupportRefs: traceQueryObservationSupportRefs(ref, edge.WakeupLine, edge.WakeupLine),
				ObservedAt:  at,
				Confidence:  0.82,
			})
		}
		for i, impact := range result.WakeupChain.CausalImpacts {
			if i >= traceQueryWidthTypedFamilyRowCap() {
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
				ClaimKey:  "wakeup_causal_impact:" + firstNonEmptyTraceString(traceThreadLabel(impact.Thread), impact.DominantState),
				Subject:   traceThreadLabel(impact.Thread),
				Predicate: "wakeup_causal_impact",
				Object:    impact.DominantState,
				Value:     traceQueryObservationMSValue(impact.DominantImpactMs),
				Unit:      "ms",
				Summary:   impact.Summary,
				// NEW-8 (账本 §7.6): per-node causal-impact rows publish
				// selected-window figures next to actual_* — the same typed
				// selected_window note as the aggregate rows below lets display
				// surfaces render the window endpoints inline instead of
				// pointing at the raw trace_query record.
				RichNotes: append(traceQueryTypedCausalImpactRichNotes(impact),
					traceQueryTypedKVNotes([][2]string{
						{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(result.WakeupChain.Window)},
					})...),
				SupportRefs: traceQueryObservationSupportRefs(ref, impact.LineStart, impact.LineEnd),
				ObservedAt:  at,
				Confidence:  0.78,
			})
		}
		// PTS (#68 用户裁定 2026-07-05, 零静默丢弃): the per-family wire cap above
		// bounds the individual rows; the ON-CHAIN overflow folds into ONE
		// counted record (typed folded_* notes) instead of vanishing.
		if rowCap := traceQueryWidthTypedFamilyRowCap(); len(result.WakeupChain.CausalImpacts) > rowCap {
			if fold, ok := traceQueryWakeupCausalImpactFoldRecord(scope, ref, at,
				result.WakeupChain.CausalImpacts[rowCap:], result.WakeupChain.Window); ok {
				out = append(out, fold)
			}
		}
		for i, aggregate := range result.WakeupChain.AggregatedImpacts {
			if i >= traceQueryWidthTypedFamilyRowCap() {
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
				ClaimKey:  "wakeup_causal_aggregate:" + firstNonEmptyTraceString(traceThreadLabel(aggregate.Thread), aggregate.DominantState),
				Subject:   traceThreadLabel(aggregate.Thread),
				Predicate: "wakeup_causal_aggregate",
				Object:    aggregate.DominantState,
				Value:     traceQueryObservationMSValue(aggregate.DominantImpactMs),
				Unit:      "ms",
				Summary:   aggregate.Summary,
				// F1: the record's Span above is the member-impact ENVELOPE
				// (FirstTs/LastTs) — the selected query window travels only via
				// this typed note, which is the projection's sole fallback anchor.
				RichNotes: append(traceQueryTypedCausalAggregateRichNotes(aggregate),
					traceQueryTypedKVNotes([][2]string{
						{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(result.WakeupChain.Window)},
					})...),
				SupportRefs: traceQueryObservationSupportRefs(ref, aggregate.LineStart, aggregate.LineEnd),
				ObservedAt:  at,
				Confidence:  0.80,
			})
		}
		// PTS-2 (#69 用户条件裁定 2026-07-06, 账本 §7.1): the ENGINE's aggregate
		// top-8 trim ships its overflow as one bounded synthetic fold member —
		// re-emit it here as ONE fold record on the same typed folded_* lane as
		// the wire-cap fold above, so the projection tree renders the
		// engine-level fold row (count + range + roster). nil on ≤8 groups →
		// zero emission (anti-noise).
		if fold := result.WakeupChain.AggregatedImpactsFold; fold != nil {
			out = append(out, traceQueryWakeupCausalAggregateFoldRecord(scope, ref, at, *fold, result.WakeupChain.Window))
		}
		for i, root := range result.WakeupChain.RootEvidence {
			if i >= traceQueryWidthTypedFamilyRowCap() {
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
			if i >= traceQueryWidthTypedFamilyRowCap() {
				break
			}
			if strings.TrimSpace(item.Type) == "" && strings.TrimSpace(item.Summary) == "" {
				continue
			}
			// BLK §15.C ①: the same physical lock span already published
			// through its rank row (subject=holder) above — skipping the
			// waiter-subject twin here is what keeps ONE monitor_contention
			// span from minting two crossed-direction observations.
			if item.BlockingKind != "" {
				if key := traceQueryLockContentionSpanKey(item.BlockingKind, item.LineStart, item.LineEnd, item.Thread, item.Peer); key != "" && lockSpanPublishedByRank[key] {
					continue
				}
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
				// NEW-8 (账本 §7.6): blocking rows are selected-window surfaces
				// too — carry the typed selected_window note (view window =
				// q.TimeStart/TimeEnd) so window-basis displays can name the
				// endpoints.
				RichNotes: append(traceQueryTypedCriticalBlockingRichNotes(item),
					traceQueryTypedKVNotes([][2]string{
						{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(result.CriticalBlocking.Window)},
					})...),
				SupportRefs: traceQueryObservationSupportRefs(ref, item.LineStart, item.LineEnd),
				ObservedAt:  at,
				Confidence:  item.Confidence,
			})
		}
	}

	if result.WindowStats != nil {
		out = append(out, traceQueryTypedWindowStatsObservations(*result.WindowStats, ref, scope, at)...)
		out = append(out, traceQueryTypedSemanticTraceSpanObservations(result, *result.WindowStats, ref, scope, at)...)
	}

	// NEW-9 (adversarial re-review 2026-07-04): a capacity-truncated result
	// (typed per-view compaction channel non-empty — row budgets cut the TAIL;
	// rank heads are fully kept) stamps every typed observation it publishes
	// with the precise capacity_truncated note, in ONE place. The projection
	// compile lifts it into TraceCausalProjection.CapacityTruncated and the
	// evidence-index header discloses it; nothing gates on it.
	if traceQueryResultCapacityTruncated(result) {
		for i := range out {
			out[i].RichNotes = append(out[i].RichNotes, types.TraceNoteKeyCapacityTruncated+"=true")
		}
	}

	return traceQueryApplyArtifactProvenance(result, out)
}

// traceQueryApplyArtifactProvenance keeps the tracebundle path as the capture
// identity (CMP-1 partitioning must not split one calibrated bundle into a
// systrace tree and a perftrace tree), while replacing ambiguous bundle-line
// support refs with physical artifact + local-line coordinates. The record's
// Span remains the index-global virtual coordinate and Selector carries the
// reversible mapping for consumers that do not inspect SupportRefs.
func traceQueryApplyArtifactProvenance(result tracequery.Result, records []types.ObservationRecord) []types.ObservationRecord {
	if len(records) == 0 || len(result.TraceArtifacts) == 0 {
		return records
	}
	for i := range records {
		spans, passthroughRefs := traceQueryRecordArtifactSpans(result, records[i])
		if len(spans) == 0 {
			if source, ok := traceQuerySingleCompatibleArtifact(result.TraceArtifacts); ok {
				records[i].SourceRef.TimeDomain = source.TimeDomain
				records[i].SourceRef.CanonicalTimeDomain = source.CanonicalTimeDomain
				traceQueryApplyObservationClockSources(&records[i].SourceRef, []tracequery.TraceArtifactSource{source})
			}
			continue
		}
		refs := append([]string(nil), passthroughRefs...)
		selectors := make([]string, 0, len(spans))
		for _, span := range spans {
			lineEnd := span.LocalLineEnd
			if lineEnd <= 0 {
				lineEnd = span.LocalLineStart
			}
			if lineEnd == span.LocalLineStart {
				refs = append(refs, fmt.Sprintf("%s:%d", span.SourcePath, span.LocalLineStart))
			} else {
				refs = append(refs, fmt.Sprintf("%s:%d-%d", span.SourcePath, span.LocalLineStart, lineEnd))
			}
			selectors = append(selectors, fmt.Sprintf("%s:%d-%d[%s]", span.SourcePath, span.LocalLineStart, lineEnd, span.TimeDomain))
		}
		records[i].SupportRefs = dedupTraceQueryStrings(refs)
		selector := "artifact_spans=" + strings.Join(selectors, ",")
		if strings.TrimSpace(records[i].Span.Selector) == "" {
			records[i].Span.Selector = selector
		} else {
			records[i].Span.Selector += ";" + selector
		}
		records[i].SourceRef.TimeDomain = spans[0].TimeDomain
		records[i].SourceRef.CanonicalTimeDomain = spans[0].CanonicalTimeDomain
		for _, span := range spans[1:] {
			if !strings.EqualFold(span.TimeDomain, records[i].SourceRef.TimeDomain) {
				records[i].SourceRef.TimeDomain = "multiple_aligned_domains"
			}
			if !strings.EqualFold(span.CanonicalTimeDomain, records[i].SourceRef.CanonicalTimeDomain) {
				records[i].SourceRef.CanonicalTimeDomain = ""
			}
		}
		traceQueryApplyObservationClockSources(&records[i].SourceRef, traceQueryArtifactSourcesForSpans(result.TraceArtifacts, spans))
	}
	return records
}

func traceQueryArtifactSourcesForSpans(artifacts []tracequery.TraceArtifactSource, spans []tracequery.TraceArtifactSpan) []tracequery.TraceArtifactSource {
	byPath := make(map[string]tracequery.TraceArtifactSource, len(artifacts))
	for _, source := range artifacts {
		byPath[source.SourcePath] = source
	}
	seen := map[string]bool{}
	out := make([]tracequery.TraceArtifactSource, 0, len(spans))
	for _, span := range spans {
		if seen[span.SourcePath] {
			continue
		}
		source, ok := byPath[span.SourcePath]
		if !ok {
			continue
		}
		seen[span.SourcePath] = true
		out = append(out, source)
	}
	return out
}

func traceQueryApplyObservationClockSources(ref *types.ObservationSourceRef, sources []tracequery.TraceArtifactSource) {
	if ref == nil || len(sources) == 0 {
		return
	}
	first := sources[0]
	sameMapping := true
	for _, source := range sources[1:] {
		if source.ClockAlignment != first.ClockAlignment || source.ClockCalibrated != first.ClockCalibrated ||
			!traceQuerySameOptionalFloat(source.ClockOffsetSec, first.ClockOffsetSec) ||
			!traceQuerySameOptionalFloat(source.ClockSlope, first.ClockSlope) {
			sameMapping = false
			break
		}
	}
	if !sameMapping {
		ref.ClockAlignment = "multiple"
		ref.ClockCalibrated = false
		ref.ClockOffsetSec = nil
		ref.ClockSlope = nil
		return
	}
	ref.ClockAlignment = first.ClockAlignment
	ref.ClockCalibrated = first.ClockCalibrated
	ref.ClockOffsetSec = traceQueryCloneOptionalFloat(first.ClockOffsetSec)
	ref.ClockSlope = traceQueryCloneOptionalFloat(first.ClockSlope)
}

func traceQuerySameOptionalFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return math.Float64bits(*a) == math.Float64bits(*b)
}

func traceQueryCloneOptionalFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func traceQueryRecordArtifactSpans(result tracequery.Result, record types.ObservationRecord) ([]tracequery.TraceArtifactSpan, []string) {
	var spans []tracequery.TraceArtifactSpan
	var passthrough []string
	for _, supportRef := range record.SupportRefs {
		lineStart, lineEnd, ok := traceQueryVirtualSupportRefRange(result.SourcePath, supportRef)
		if !ok {
			passthrough = append(passthrough, supportRef)
			continue
		}
		spans = append(spans, result.ResolveArtifactSpans(lineStart, lineEnd)...)
	}
	if len(spans) == 0 {
		spans = result.ResolveArtifactSpans(record.Span.LineStart, record.Span.LineEnd)
	}
	seen := map[string]bool{}
	compacted := make([]tracequery.TraceArtifactSpan, 0, len(spans))
	for _, span := range spans {
		key := fmt.Sprintf("%s\x00%d\x00%d\x00%s", span.SourcePath, span.LocalLineStart, span.LocalLineEnd, span.TimeDomain)
		if seen[key] {
			continue
		}
		seen[key] = true
		compacted = append(compacted, span)
	}
	return compacted, passthrough
}

func traceQueryVirtualSupportRefRange(sourcePath, supportRef string) (int, int, bool) {
	sourcePath = strings.TrimSpace(sourcePath)
	supportRef = strings.TrimSpace(supportRef)
	if sourcePath == "" || !strings.HasPrefix(supportRef, sourcePath+":") {
		return 0, 0, false
	}
	raw := strings.TrimPrefix(supportRef, sourcePath+":")
	startRaw, endRaw, hasRange := strings.Cut(raw, "-")
	start, err := strconv.Atoi(startRaw)
	if err != nil || start <= 0 {
		return 0, 0, false
	}
	end := start
	if hasRange {
		end, err = strconv.Atoi(endRaw)
		if err != nil || end < start {
			return 0, 0, false
		}
	}
	return start, end, true
}

func traceQuerySingleCompatibleArtifact(sources []tracequery.TraceArtifactSource) (tracequery.TraceArtifactSource, bool) {
	var selected tracequery.TraceArtifactSource
	count := 0
	for _, source := range sources {
		if !source.CausalCompatible {
			continue
		}
		selected = source
		count++
	}
	return selected, count == 1
}

func dedupTraceQueryStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// traceQueryResultCapacityTruncated is the single NEW-9 truncation predicate:
// true iff the engine published at least one typed ViewCompaction record for
// this result (tracequery Result.Compactions — the typed channel that marks
// truncated result rows). Precise boolean on the typed channel only; caveat
// prose never participates.
func traceQueryResultCapacityTruncated(result tracequery.Result) bool {
	return len(result.Compactions) > 0
}

func traceQueryTypedPriorityRichNotes(rank int, tier, typ, source, causality string, chainDepth int, score, impact, cumulativeImpact, effectiveImpact, targetImpact, projectedImpact, actualImpact, actualTotal, actualStart, actualEnd float64) []string {
	var notes []string
	if rank > 0 {
		notes = append(notes, fmt.Sprintf("%s=%d", types.TraceNoteKeyRank, rank))
	}
	if tier != "" {
		notes = append(notes, types.TraceNoteKeyTier+"="+tier)
	}
	if typ != "" {
		notes = append(notes, types.TraceNoteKeyType+"="+typ)
	}
	if impact > 0 {
		notes = append(notes, fmt.Sprintf("%s=%.3f", types.TraceNoteKeyImpactMS, impact))
	}
	if projectedImpact > 0 {
		notes = append(notes, fmt.Sprintf("projected_impact_ms=%.3f", projectedImpact))
	}
	if cumulativeImpact > 0 {
		notes = append(notes, fmt.Sprintf("%s=%.3f", types.TraceNoteKeyCumulativeImpactMS, cumulativeImpact))
		notes = append(notes, fmt.Sprintf("projected_total_ms=%.3f", cumulativeImpact))
	}
	if effectiveImpact > 0 {
		notes = append(notes, fmt.Sprintf("%s=%.3f", types.TraceNoteKeyEffectiveImpactMS, effectiveImpact))
	}
	if targetImpact > 0 {
		notes = append(notes, fmt.Sprintf("target_impact_ms=%.3f", targetImpact))
	}
	if actualImpact > 0 {
		notes = append(notes, fmt.Sprintf("%s=%.3f", types.TraceNoteKeyActualImpactMS, actualImpact))
	}
	if actualTotal > 0 {
		notes = append(notes, fmt.Sprintf("%s=%.3f", types.TraceNoteKeyActualTotalMS, actualTotal))
	}
	// DIAG A2 (§28.11-3(b) D-10, 2026-07-09): both actual calibers on one row
	// and >10% apart → typed disclosure (no value judged, no value edited).
	if caliber := traceQueryActualCaliberNote(actualImpact, actualTotal); caliber != "" {
		notes = append(notes, types.TraceNoteKeyActualCaliberNote+"="+caliber)
	}
	if actualWindow := traceQueryWindowValue(actualStart, actualEnd); actualWindow != "" {
		notes = append(notes, types.TraceNoteKeyActualWindow+"="+actualWindow)
	}
	if score > 0 {
		notes = append(notes, fmt.Sprintf("score=%.3f", score))
	}
	if source != "" {
		notes = append(notes, types.TraceNoteKeySource+"="+source)
	}
	if causality != "" {
		notes = append(notes, types.TraceNoteKeyCausality+"="+causality)
	}
	if chainDepth > 0 {
		notes = append(notes, fmt.Sprintf("%s=%d", types.TraceNoteKeyChainDepth, chainDepth))
	}
	return notes
}

// traceQueryActualCaliberNote is the single DIAG A2 divergence judgment
// (§28.11-3(b), D-10): both actual calibers present on ONE row — the
// dominant-state segment actual (actual_impact lane) and the thread-level
// actual total (actual_total lane) — and diverging by MORE than 10% of the
// larger. Returns the closed enum value; "" otherwise (the note zero-drops).
// Precise arithmetic on two typed floats; nobody guesses which caliber is
// "right" and neither value is edited (disclosure only, zero weight).
func traceQueryActualCaliberNote(actualImpact, actualTotal float64) string {
	if actualImpact <= 0 || actualTotal <= 0 {
		return ""
	}
	larger := actualImpact
	if actualTotal > larger {
		larger = actualTotal
	}
	if math.Abs(actualTotal-actualImpact) > 0.10*larger {
		return types.TraceActualCaliberStateSegmentVsThreadTotal
	}
	return ""
}

func traceQueryTypedRootCauseStateRichNotes(item tracequery.RootCauseRankItem) []string {
	return traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeyDominantState, item.DominantState},
		{types.TraceNoteKeyRunning, traceQueryObservationMSValue(item.RunningMs)},
		{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(item.RunnableMs)},
		{types.TraceNoteKeySleep, traceQueryObservationMSValue(item.SleepMs)},
		{types.TraceNoteKeyDState, traceQueryObservationMSValue(item.DStateMs)},
		{types.TraceNoteKeyIOWait, traceQueryObservationMSValue(item.IOWaitMs)},
	})
}

// traceQueryWakeupCausalImpactFoldRecord builds the PTS zero-silent-drop fold
// record (#68 用户裁定 2026-07-05: 凡 on-chain 项必须提及+进树,多则折叠+计数):
// the ON-CHAIN causal-impact rows beyond the per-family wire cap fold into ONE
// counted record — count + min–max range + an up-to-8 subject roster ride the
// typed folded_* notes (projection compile re-materializes the fold row); the
// full per-row detail remains in the stored raw payload. ok=false when the
// overflow contains no on-chain row (off-chain overflow must not borrow the
// on-chain lane).
func traceQueryWakeupCausalImpactFoldRecord(scope string, ref types.ObservationSourceRef, at string,
	overflow []tracequery.WakeupCausalImpact, window tracequery.TimeWindow) (types.ObservationRecord, bool) {
	var members []tracequery.WakeupCausalImpact
	for _, impact := range overflow {
		if impact.OnChain {
			members = append(members, impact)
		}
	}
	if len(members) == 0 {
		return types.ObservationRecord{}, false
	}
	var minMS, maxMS float64
	span := types.ObservationSpan{}
	var subjects []string
	seen := map[string]bool{}
	for _, member := range members {
		v := member.DominantImpactMs
		if minMS == 0 || (v > 0 && v < minMS) {
			minMS = v
		}
		if v > maxMS {
			maxMS = v
		}
		if member.LineStart > 0 && (span.LineStart <= 0 || member.LineStart < span.LineStart) {
			span.LineStart = member.LineStart
		}
		if member.LineEnd > span.LineEnd {
			span.LineEnd = member.LineEnd
		}
		if member.Window.StartTs > 0 && (span.StartTs <= 0 || member.Window.StartTs < span.StartTs) {
			span.StartTs = member.Window.StartTs
		}
		if member.Window.EndTs > span.EndTs {
			span.EndTs = member.Window.EndTs
		}
		if label := strings.TrimSpace(traceThreadLabel(member.Thread)); label != "" && !seen[label] && len(subjects) < 8 {
			seen[label] = true
			subjects = append(subjects, label)
		}
	}
	return types.ObservationRecord{
		ID:              fmt.Sprintf("trace_query:%s#wakeup_causal_impact_fold", scope),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            span,
		ClaimKey:        "wakeup_causal_impact:folded_overflow",
		Predicate:       "wakeup_causal_impact",
		Value:           traceQueryObservationMSValue(maxMS),
		Unit:            "ms",
		Summary: fmt.Sprintf("%d on-chain causal-impact rows beyond the typed row cap folded (max %.3fms); full rows remain in the stored trace_query payload",
			len(members), maxMS),
		RichNotes: traceQueryTypedKVNotes([][2]string{
			{types.TraceNoteKeyCausality, traceQueryCausalityLabel(true)},
			{types.TraceNoteKeyChainRelevance, "on_chain"},
			{types.TraceNoteKeyImpact, traceQueryObservationMSValue(maxMS)},
			{types.TraceNoteKeyFoldedRows, strconv.Itoa(len(members))},
			{types.TraceNoteKeyFoldedMinMS, traceQueryObservationMSValue(minMS)},
			{types.TraceNoteKeyFoldedMaxMS, traceQueryObservationMSValue(maxMS)},
			{types.TraceNoteKeyFoldedSubjects, strings.Join(subjects, ",")},
			// DIAG A1 (§28.11-3(a) G12, 2026-07-09): µs-tie member roster at
			// THIS wire-side take-MAX merge point (zero-dropped when <2 tie).
			{types.TraceNoteKeySameValueMembers, traceQuerySameValueMemberNote(members, maxMS)},
			{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(window)},
		}),
		SupportRefs: traceQueryObservationSupportRefs(ref, span.LineStart, span.LineEnd),
		ObservedAt:  at,
		Confidence:  0.78,
	}, true
}

// traceQuerySameValueMemberNote is the wire-side DIAG A1 tie collector
// (§28.11-3(a), same strict criterion as the projection folds —
// types.TraceCausalProjectionSameValueTieMS): members whose dominant impact
// ties the fold's published MAX to the µs are disclosed as
// "<subject>@<line_start>-<line_end>" comma-joined entries (cap 4). "" when
// fewer than two members with a NON-EMPTY thread label tie (复核 P3-1
// symmetry: a label-less member cannot be a nameable witness — the consumer
// parser discards subject-less entries, so the producer never mints them).
// The note zero-drops. Disclosure only: callers pass the already-final maxMS
// and never read anything back.
func traceQuerySameValueMemberNote(members []tracequery.WakeupCausalImpact, maxMS float64) string {
	if maxMS <= 0 {
		return ""
	}
	var entries []string
	for _, member := range members {
		label := strings.TrimSpace(traceThreadLabel(member.Thread))
		if label == "" {
			continue
		}
		v := member.DominantImpactMs
		if v <= 0 || math.Abs(v-maxMS) >= types.TraceCausalProjectionSameValueTieMS {
			continue
		}
		if len(entries) < 4 {
			entries = append(entries, fmt.Sprintf("%s@%d-%d", label, member.LineStart, member.LineEnd))
		}
	}
	if len(entries) < 2 {
		return ""
	}
	return strings.Join(entries, ",")
}

// traceQueryAggregateFoldTieNote renders the engine-computed P2-1 tie roster
// into the same "<subject>@<line_start>-<line_end>" comma-joined note form as
// traceQuerySameValueMemberNote (single parser downstream —
// traceCausalProjectionParseSameValueMembers). The engine helper already
// enforces the ≥2-labeled-ties / cap-4 / strict-band discipline, so this is a
// pure formatter; "" zero-drops the note.
func traceQueryAggregateFoldTieNote(ties []tracequery.WakeupCausalAggregateFoldTieMember) string {
	if len(ties) < 2 {
		return ""
	}
	entries := make([]string, 0, len(ties))
	for _, tie := range ties {
		label := strings.TrimSpace(tie.Label)
		if label == "" {
			continue
		}
		entries = append(entries, fmt.Sprintf("%s@%d-%d", label, tie.LineStart, tie.LineEnd))
	}
	if len(entries) < 2 {
		return ""
	}
	return strings.Join(entries, ",")
}

// traceQueryWakeupCausalAggregateFoldRecord builds the PTS-2 engine-level
// aggregate fold record (#69 用户条件裁定 2026-07-06): the engine's aggregate
// top-8 trim folded its rank>8 overflow into ONE bounded synthetic member
// (ChainResult.AggregatedImpactsFold) — this record carries that fold onto the
// SAME typed folded_* note lane as the wire-cap fold above (NKR 折叠族 reuse,
// zero new key families), so the projection compile re-materializes it through
// the existing MergedCount pipeline (caliber source / never leads / never
// badges / fold-row rendering all现成). The published value is the member MAX
// — wall clock never sums across threads. Aggregate members are on-chain by
// construction (ChainDepth>0 filter at aggregation).
func traceQueryWakeupCausalAggregateFoldRecord(scope string, ref types.ObservationSourceRef, at string,
	fold tracequery.WakeupCausalAggregateFold, window tracequery.TimeWindow) types.ObservationRecord {
	span := types.ObservationSpan{
		LineStart: fold.LineStart,
		LineEnd:   fold.LineEnd,
		StartTs:   fold.FirstTs,
		EndTs:     fold.LastTs,
	}
	return types.ObservationRecord{
		ID:              fmt.Sprintf("trace_query:%s#wakeup_causal_aggregate_fold", scope),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            span,
		ClaimKey:        "wakeup_causal_aggregate:folded_overflow",
		Predicate:       "wakeup_causal_aggregate",
		Value:           traceQueryObservationMSValue(fold.MaxImpactMs),
		Unit:            "ms",
		Summary: fmt.Sprintf("%d aggregated wakeup-causal pairs beyond the engine top-8 folded (max %.3fms); full per-hop causal impact rows remain in the stored trace_query payload",
			fold.Groups, fold.MaxImpactMs),
		RichNotes: traceQueryTypedKVNotes([][2]string{
			{types.TraceNoteKeyCausality, traceQueryCausalityLabel(true)},
			{types.TraceNoteKeyChainRelevance, "on_chain"},
			{types.TraceNoteKeyImpact, traceQueryObservationMSValue(fold.MaxImpactMs)},
			{types.TraceNoteKeyFoldedRows, strconv.Itoa(fold.Groups)},
			{types.TraceNoteKeyFoldedMinMS, traceQueryObservationMSValue(fold.MinImpactMs)},
			{types.TraceNoteKeyFoldedMaxMS, traceQueryObservationMSValue(fold.MaxImpactMs)},
			{types.TraceNoteKeyFoldedSubjects, strings.Join(fold.Subjects, ",")},
			// P2-1 (DIAG A1 第四取最大点, G12-ENG batch, 2026-07-09): the
			// engine's own tie roster rides the EXISTING same_value_members
			// note (zero new keys; the projection compile re-materializes it
			// into node.SameValueMembers — consumer chain built by DIAG A1).
			{types.TraceNoteKeySameValueMembers, traceQueryAggregateFoldTieNote(fold.SameValueMembers)},
			{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(window)},
		}),
		SupportRefs: traceQueryObservationSupportRefs(ref, span.LineStart, span.LineEnd),
		ObservedAt:  at,
		Confidence:  0.80,
	}
}

func traceQueryTypedCausalImpactRichNotes(impact tracequery.WakeupCausalImpact) []string {
	views := traceQueryCausalImpactRecommendedViews(impact)
	notes := traceQueryTypedKVNotes([][2]string{
		// F-1 mutual pin: traceQueryTypedCount zero-drops, so ChainDepth==0
		// publishes NO depth= note. RN-14c consumers in
		// emit_investigation_complete.go (traceChainObservationTargetPID /
		// traceRunnableAnchorObservationPID) key depth 0 on that ABSENCE —
		// do not switch this note to always-print without updating them.
		{types.TraceNoteKeyDepth, traceQueryTypedCount(impact.ChainDepth)},
		// P0-E CHAIN-PATH (ledger §22.1): the impact row's owning branch
		// ordinal (zero-dropped on legacy rows) — display attach domain only.
		{types.TraceNoteKeyChainBranch, traceQueryTypedCount(impact.ChainBranch)},
		{types.TraceNoteKeyCausality, traceQueryCausalityLabel(impact.OnChain)},
		{types.TraceNoteKeyDominantState, impact.DominantState},
		{types.TraceNoteKeyImpact, traceQueryObservationMSValue(impact.DominantImpactMs)},
		// PTV5 Q1 (#68 用户裁定 2026-07-05, 上游根治): every wakeup_causal_impact
		// row publishes its effective attribution with the SAME semantics as
		// the root_cause_rank lane (rootCauseEffectiveImpactMs): periodic rows
		// keep the VS-1 discounted lane (appended below, 0 included), inversion
		// candidates publish the R5d gated composite, plain rows publish the
		// raw attribution (no discount applies → effective == raw).
		{types.TraceNoteKeyEffectiveImpactMS, traceQueryCausalImpactEffectiveNoteValue(impact)},
		{types.TraceNoteKeyProjectedImpact, traceQueryObservationMSValue(impact.ProjectedImpactMs)},
		{types.TraceNoteKeyTotal, traceQueryObservationMSValue(impact.TotalMs)},
		{"projected_total", traceQueryObservationMSValue(impact.ProjectedTotalMs)},
		{types.TraceNoteKeyActualImpact, traceQueryObservationMSValue(impact.ActualImpactMs)},
		{types.TraceNoteKeyActualTotal, traceQueryObservationMSValue(impact.ActualTotalMs)},
		// DIAG A2 (§28.11-3(b) D-10): two actual calibers >10% apart on one
		// row → typed disclosure (zero-dropped otherwise).
		{types.TraceNoteKeyActualCaliberNote, traceQueryActualCaliberNote(impact.ActualImpactMs, impact.ActualTotalMs)},
		{types.TraceNoteKeyActualWindow, traceQueryWindowValue(impact.ActualWindow.StartTs, impact.ActualWindow.EndTs)},
		{"target_impact", traceQueryObservationMSValue(impact.TargetBlockedMs)},
		{types.TraceNoteKeyFragments, traceQueryTypedCount(impact.FragmentCount)},
		{types.TraceNoteKeySwitches, traceQueryTypedCount(impact.StateSwitches)},
		{types.TraceNoteKeyMaxSegment, traceQueryObservationMSValue(impact.MaxSegmentMs)},
		{types.TraceNoteKeyP95Segment, traceQueryObservationMSValue(impact.P95SegmentMs)},
		{types.TraceNoteKeyRunning, traceQueryObservationMSValue(impact.RunningMs)},
		{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(impact.RunnableMs)},
		{types.TraceNoteKeySleep, traceQueryObservationMSValue(impact.SleepMs)},
		{types.TraceNoteKeyDState, traceQueryObservationMSValue(impact.DStateMs)},
		{types.TraceNoteKeyIOWait, traceQueryObservationMSValue(impact.IOWaitMs)},
		{types.TraceNoteKeyActualRunning, traceQueryObservationMSValue(impact.ActualRunningMs)},
		{types.TraceNoteKeyActualRunnable, traceQueryObservationMSValue(impact.ActualRunnableMs)},
		{types.TraceNoteKeyActualSleep, traceQueryObservationMSValue(impact.ActualSleepMs)},
		{types.TraceNoteKeyActualDState, traceQueryObservationMSValue(impact.ActualDStateMs)},
		{types.TraceNoteKeyActualIOWait, traceQueryObservationMSValue(impact.ActualIOWaitMs)},
		{"priority", traceQueryPriorityPair(impact.Priority, impact.PriorityClass)},
		{"target_priority", traceQueryPriorityPair(impact.TargetPriority, impact.TargetPriorityClass)},
		{"priority_relation", impact.PriorityRelation},
		{types.TraceNoteKeyPriorityInversionCandidate, traceQueryTypedBool(impact.PriorityInversionCandidate)},
		{"priority_inversion_gated", traceQueryObservationMSValue(impact.PriorityInversionGatedMs)},
		// §7.30.3 D3: the gated composite's typed composition (runnable full
		// amount + capacity-discounted weak-core running deficit).
		{types.TraceNoteKeyGatedRunnable, traceQueryObservationMSValue(impact.GatedRunnableMs)},
		{types.TraceNoteKeyGatedRunningDeficit, traceQueryObservationMSValue(impact.GatedRunningDeficitMs)},
		// CAP (§26 C3): the discounted component's capability caliber.
		// CAP-2: the cluster-topology source rides beside it.
		{types.TraceNoteKeyGatedCapability, impact.GatedCapabilitySource},
		{types.TraceNoteKeyGatedClusterTopology, impact.GatedClusterTopology},
		{types.TraceNoteKeyRecommendedViews, strings.Join(views, ",")},
		{types.TraceNoteKeyChainRequired, traceQueryTypedBool(impact.OnChain && traceQueryCausalImpactNeedsChain(impact.DominantState))},
		{types.TraceNoteKeyRecursive, traceQueryTypedBool(impact.OnChain && traceQueryCausalImpactRecursive(impact.DominantState))},
		{types.TraceNoteKeyNextStep, impact.NextStep},
		{types.TraceNoteKeyNextStepKind, impact.NextStepKind},
	})
	notes = append(notes, traceQueryTypedPeriodicSourceRichNotes(impact.PeriodicSource, impact.DetectedPeriodMs, impact.LatenessMs, impact.EffectivePeriodicImpactMs, true)...)
	// VS-2 (§7.10): supply-fold accounting of a running-dominant on-chain node.
	return append(notes, traceQueryTypedSupplyFoldRichNotes(impact.SupplyFoldBasis, impact.SupplyFoldDeficitMs, impact.SupplyFoldIdealMs)...)
}

// traceQueryTypedPeriodicSourceRichNotes publishes the VS-1 (§7.8) periodic-
// signal-source accounting as typed notes, emitted ONLY on periodic rows.
// lateness_ms/effective_impact_ms print explicitly (0.000 included): a
// periodic row's discounted attribution being ~0 is exactly the load-bearing
// fact (the sleep was pure cadence), so the zero must survive to the display
// surfaces instead of being dropped by the positive-value note filter.
// includeEffective=false is for the root_cause_rank records whose
// positive-value priority notes already carried effective_impact_ms — the
// zero-effective note is still appended there so the discount never vanishes.
// traceQueryCausalImpactEffectiveNoteValue is the PTV5 Q1 effective-attribution
// value for one wakeup_causal_impact row (#68 用户裁定 2026-07-05): the number
// comes from the engine's exported single source
// (tracequery.WakeupCausalImpactEffectiveImpactMs — the rank lane's own
// branch chain, 复核 Med 真镜像 2026-07-06, no second implementation to
// drift); a periodic row defers to the VS-1 discounted note lane (returns ""
// so no duplicate key is emitted).
func traceQueryCausalImpactEffectiveNoteValue(impact tracequery.WakeupCausalImpact) string {
	if impact.PeriodicSource {
		return ""
	}
	value := tracequery.WakeupCausalImpactEffectiveImpactMs(impact)
	if value <= 0 && impact.DominantState == string(tracequery.StateRunning) {
		// §20.2 (2026-07-07): running rows' zero attribution prints
		// explicitly (VS-1 explicit-zero discipline) — the eliminable
		// deficit being 0 IS the load-bearing fact; dropping the note would
		// leave the raw wall clock looking like the attribution.
		return "0.000"
	}
	return traceQueryObservationMSValue(value)
}

func traceQueryTypedPeriodicSourceRichNotes(periodic bool, periodMs, latenessMs, effectiveMs float64, includeEffective bool) []string {
	if !periodic {
		return nil
	}
	notes := []string{
		types.TraceNoteKeyPeriodicSource + "=true",
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeyDetectedPeriodMS, periodMs),
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeyLatenessMS, latenessMs),
	}
	if includeEffective || effectiveMs <= 0 {
		notes = append(notes, fmt.Sprintf("%s=%.3f", types.TraceNoteKeyEffectiveImpactMS, effectiveMs))
	}
	return notes
}

// traceQueryTypedSupplyFoldRichNotes publishes the VS-2 (§7.10) supply-fold
// accounting as typed notes, emitted ONLY when the fold ran (basis non-nil —
// the producer computes it exclusively for on-chain running-dominant nodes).
// deficit/ideal print explicitly with zeros included: "deficit 0.000 on a
// fully-known basis" IS the affirmative ran-at-full-frequency fact the §7.10
// fourth decision branch consumes, so the zero must survive to the display
// surfaces. fold_basis carries the known/unknown wall split (token
// supply_fold_deficit in the causal-token registry, ComputeDelivery lane).
func traceQueryTypedSupplyFoldRichNotes(basis *tracequery.SupplyFoldBasis, deficitMs, idealMs float64) []string {
	if basis == nil {
		return nil
	}
	notes := []string{
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeySupplyFoldDeficitMS, deficitMs),
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeySupplyFoldIdealMS, idealMs),
		fmt.Sprintf("%s=known=%.3fms,unknown=%.3fms", types.TraceNoteKeyFoldBasis, basis.KnownMs, basis.UnknownMs),
	}
	// CAP (§26 C3): the fold's typed capability caliber — default_table /
	// evidence_table / freq_only. Empty only on pre-CAP aggregates re-serialized
	// from stored fixtures; the engine always stamps it when the fold runs.
	if basis.CapabilitySource != "" {
		notes = append(notes, fmt.Sprintf("%s=%s", types.TraceNoteKeyFoldCapability, basis.CapabilitySource))
	}
	// CAP 复核 F1: the demoted basis class — emitted ONLY when the reference
	// moved off the nominated big class, so every undemoted record's notes
	// stay byte-identical and absence precisely means the big-class basis.
	if basis.ReferenceClass != "" && basis.ReferenceClass != "big" {
		notes = append(notes, fmt.Sprintf("%s=%s", types.TraceNoteKeyFoldReferenceClass, basis.ReferenceClass))
	}
	// CAP-2 (§28.4/§28.5): the cluster-STRUCTURE source — emitted only on the
	// two evidence forms (freq_comovement / keyed_rail); absence keeps every
	// explicit-topology/legacy note stream byte-identical.
	if basis.ClusterTopologySource != "" {
		notes = append(notes, fmt.Sprintf("%s=%s", types.TraceNoteKeyFoldClusterTopology, basis.ClusterTopologySource))
	}
	// CAP-2 audit note (§28.5-T6 残洞兜底): the adopted rail family and the
	// rail-governed slice roster keep the anchor-presumption fold traceable.
	if basis.RailFamily != "" || len(basis.RailGoverned) > 0 {
		parts := make([]string, 0, len(basis.RailGoverned)+1)
		if basis.RailFamily != "" {
			parts = append(parts, "族="+basis.RailFamily)
		}
		for _, entry := range basis.RailGoverned {
			parts = append(parts, fmt.Sprintf("cpu%d 频点=簇轨 %s", entry.CPU, entry.Rail))
		}
		notes = append(notes, fmt.Sprintf("%s=%s", types.TraceNoteKeyFoldRailBasis, strings.Join(parts, ";")))
	}
	// THERM (§28.5-T7): the in-window thermal/policy press on the dominant
	// running cluster — disclosure-only, the display renders the
	// 窗内该簇受热限压至 X sentence off this typed value.
	if basis.ThermalCapKHz > 0 {
		notes = append(notes, fmt.Sprintf("%s=%d", types.TraceNoteKeyThermalCapKHz, basis.ThermalCapKHz))
	}
	// VS-2b (§7.10): fmax ladder provenance — limits (policy authority) vs
	// observed governance fallback. Zero on aggregates (mixed member windows
	// have no single fmax) and on all-unknown folds.
	if basis.FmaxKHz > 0 && basis.FmaxSource != "" {
		notes = append(notes, fmt.Sprintf("%s=%.3fGHz,source=%s", types.TraceNoteKeyFoldFmax, float64(basis.FmaxKHz)/1e6, basis.FmaxSource))
	}
	// CFR (#75 簇共频, 客户硬件域裁定): slices folded with a same-cluster
	// sampled core's frequency disclose the donor — short typed provenance so
	// the KNOWN-basis claim stays auditable (SupplyFoldBasis.ClusterFreqReuse,
	// cluster_freq_share.go authority). CFR-2 (#80) 披露区分: the suffix
	// names the membership source; the explicit wording stays byte-identical
	// to the CFR #75 original (pinned).
	if len(basis.ClusterFreqReuse) > 0 {
		parts := make([]string, 0, len(basis.ClusterFreqReuse))
		for _, pair := range basis.ClusterFreqReuse {
			parts = append(parts, fmt.Sprintf("cpu%d 频点=同簇 cpu%d", pair.CPU, pair.DonorCPU))
		}
		suffix := "(簇共频复用,显式拓扑)"
		if basis.ClusterFreqReuseSource == tracequery.ClusterFreqSourceDerived {
			suffix = "(簇共频复用,频点变化点推导)"
		}
		notes = append(notes, fmt.Sprintf("%s=%s%s", types.TraceNoteKeyFoldClusterFreqReuse, strings.Join(parts, ";"), suffix))
	}
	// VS-2b companion finding (typed engine comparison, soft display
	// wording): the governing policy ceiling sat below frequencies the same
	// cluster demonstrably reached elsewhere in the trace.
	if basis.LimitThrottled && basis.FmaxKHz > 0 {
		notes = append(notes, fmt.Sprintf("%s=大核受策略/温控限频至 %.2f GHz,缺口部分源于压频(全程观测最高 %.2f GHz)",
			types.TraceNoteKeyFoldFmaxFinding,
			float64(basis.FmaxKHz)/1e6, float64(basis.TraceObservedMaxKHz)/1e6))
	}
	// VS-2c(a): cluster-lane corroboration caveat, rendered ONLY on the
	// precise divergence flag (一致时不加注). Lane names AND units are
	// vendor free vocabulary — the flag means the raw sample matched the
	// fmax under NO unit hypothesis (2026-07-04 review), so the caveat
	// reports the RAW value without a unit and says the unit is unresolved
	// instead of asserting a false direction. Corroboration only, never the
	// fold basis.
	if basis.ClusterLaneDivergent && basis.ClusterLaneMaxKHz > 0 {
		notes = append(notes, fmt.Sprintf("%s=簇泳道 %s 最高原始值 %d(单位不明)在原值/千分/百万分单位假设下均与折算 fmax %.2f GHz 相差 >10%%,泳道名与单位均为厂商自由词汇仅旁证",
			types.TraceNoteKeyFoldClusterLaneCaveat,
			basis.ClusterLaneName, basis.ClusterLaneMaxKHz, float64(basis.FmaxKHz)/1e6))
	}
	return notes
}

// traceQueryCausalImpactRecommendedViews / traceQueryCausalImpactNeedsChain /
// traceQueryCausalImpactRecursive are the causal-impact twins of the
// tracequery state_drilldown drilldown-plan trio (stateDrilldownRecommendedViews /
// stateDrilldownNeedsWakeupChain / stateDrilldownNeedsRecursiveChainForSource)
// — born byte-identical in 9d4e6958, then left behind when RN-11 (§7.9,
// 7c5c236d) repointed the runnable branch at the CPU-competition surfaces.
// TSH review F2 re-synced them and put both switches (and the recursive
// comparison) under the dominant-state consumer pin
// (trace_query_dominant_state_pin_test.go) so the next member change cannot
// silently skip this copy again.
func traceQueryCausalImpactRecommendedViews(impact tracequery.WakeupCausalImpact) []string {
	switch impact.DominantState {
	case string(tracequery.StateSSleep):
		return []string{"wakeup_chain", "root_cause_rank"}
	case string(tracequery.StateRunnable):
		// RN-11 (§7.9): runnable drilldown surfaces are CPU competition ones —
		// scheduler latency, ranked competitors, and window_stats
		// (cpu_occupancy top occupiers + compute_supply_balance).
		return []string{"scheduler_latency_stats", "root_cause_rank", "window_stats"}
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
	// RN-11 (§7.9): StateRunnable is deliberately absent — a runnable-dominant
	// node is CPU competition, not a wakeup dependency, so chain_required must
	// not push the model into a wakeup_chain drilldown of it. This is the
	// causal-impact twin of stateDrilldownNeedsWakeupChain (tracequery); the
	// engine copy shipped with RN-11 while this one was missed (TSH review F2).
	// StateRunning never required a chain (occupancy: perf/compute-supply
	// surfaces own it). Both absences are ledgered in the pin test.
	case string(tracequery.StateSSleep), string(tracequery.StateDSleep), string(tracequery.StateIOWait):
		return true
	default:
		return false
	}
}

// traceQueryCausalImpactRecursive mirrors stateDrilldownNeedsRecursiveChainForSource:
// RN-11 drops the wakeup-chain REQUIREMENT for runnable-dominant nodes but
// keeps them recursive root-cause candidates (occupancy / scheduler-latency
// drilldown), so the recursive note must not collapse into needs-chain.
func traceQueryCausalImpactRecursive(state string) bool {
	if state == string(tracequery.StateRunnable) {
		return true
	}
	return traceQueryCausalImpactNeedsChain(state)
}

func traceQueryTypedCausalAggregateRichNotes(aggregate tracequery.WakeupCausalAggregate) []string {
	// F2 (§20.2 absorption): the single typed rank-face determination —
	// see the gating comment at the candidate note below.
	inversionTyped := tracequery.WakeupCausalAggregateInversionTyped(aggregate)
	notes := traceQueryTypedOccurrenceWindowRichNotes(aggregate.OccurrenceWindows)
	notes = append(notes, traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeyDepth, traceQueryTypedCount(aggregate.ChainDepth)},
		// P0-E CHAIN-PATH: single-branch aggregates carry their branch; a
		// cross-branch aggregate has no single identity (engine keeps 0).
		{types.TraceNoteKeyChainBranch, traceQueryTypedCount(aggregate.ChainBranch)},
		{types.TraceNoteKeyPath, aggregate.Path},
		{"occurrences", traceQueryTypedCount(aggregate.OccurrenceCount)},
		{"aggregation_caliber", aggregate.AggregationCaliber},
		{types.TraceNoteKeyDominantState, aggregate.DominantState},
		{types.TraceNoteKeyImpact, traceQueryObservationMSValue(aggregate.DominantImpactMs)},
		{types.TraceNoteKeyProjectedImpact, traceQueryObservationMSValue(aggregate.ProjectedImpactMs)},
		{types.TraceNoteKeyTotal, traceQueryObservationMSValue(aggregate.TotalMs)},
		{"projected_total", traceQueryObservationMSValue(aggregate.ProjectedTotalMs)},
		{types.TraceNoteKeyActualImpact, traceQueryObservationMSValue(aggregate.ActualImpactMs)},
		{types.TraceNoteKeyActualTotal, traceQueryObservationMSValue(aggregate.ActualTotalMs)},
		// DIAG A2 (§28.11-3(b) D-10): same two-caliber disclosure as the
		// per-occurrence impact lane (zero-dropped otherwise).
		{types.TraceNoteKeyActualCaliberNote, traceQueryActualCaliberNote(aggregate.ActualImpactMs, aggregate.ActualTotalMs)},
		{types.TraceNoteKeyActualWindow, traceQueryWindowValue(aggregate.ActualFirstTs, aggregate.ActualLastTs)},
		{"target_impact", traceQueryObservationMSValue(aggregate.TargetBlockedMs)},
		{types.TraceNoteKeyFragments, traceQueryTypedCount(aggregate.FragmentCount)},
		{types.TraceNoteKeySwitches, traceQueryTypedCount(aggregate.StateSwitches)},
		{types.TraceNoteKeyMaxSegment, traceQueryObservationMSValue(aggregate.MaxSegmentMs)},
		{types.TraceNoteKeyRunning, traceQueryObservationMSValue(aggregate.RunningMs)},
		{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(aggregate.RunnableMs)},
		{types.TraceNoteKeySleep, traceQueryObservationMSValue(aggregate.SleepMs)},
		{types.TraceNoteKeyDState, traceQueryObservationMSValue(aggregate.DStateMs)},
		{types.TraceNoteKeyIOWait, traceQueryObservationMSValue(aggregate.IOWaitMs)},
		{types.TraceNoteKeyActualRunning, traceQueryObservationMSValue(aggregate.ActualRunningMs)},
		{types.TraceNoteKeyActualRunnable, traceQueryObservationMSValue(aggregate.ActualRunnableMs)},
		{types.TraceNoteKeyActualSleep, traceQueryObservationMSValue(aggregate.ActualSleepMs)},
		{types.TraceNoteKeyActualDState, traceQueryObservationMSValue(aggregate.ActualDStateMs)},
		{types.TraceNoteKeyActualIOWait, traceQueryObservationMSValue(aggregate.ActualIOWaitMs)},
		{"priority_relation", aggregate.PriorityRelation},
		// F2 (§20.2 absorption, 2026-07-07): the candidate note AND the gated
		// notes below gate on the SAME typed determination the rank face uses
		// (WakeupCausalAggregateInversionTyped — F1's priority-sensitive
		// typing), never on the raw invCount>0 flag: a sleep/D/IO-dominant
		// aggregate with one inversion member must not light the inversion
		// label + gated composition prose while its bar shows the raw
		// dominant value (the contradiction row).
		{types.TraceNoteKeyPriorityInversionCandidate, traceQueryTypedBool(inversionTyped)},
	})...)
	if inversionTyped {
		notes = append(notes, traceQueryTypedKVNotes([][2]string{
			// §20 E-Gap② (2026-07-07): the aggregate face publishes its R5d
			// gated caliber under the SAME registered keys as the
			// per-occurrence face — components + total (total == components
			// sum by construction, F4; cross-occurrence value = sum over
			// disjoint occurrence windows, honest MAX fallback on overlap;
			// applyAggregateGatedInversion holds the argument).
			{types.TraceNoteKeyGatedRunnable, traceQueryObservationMSValue(aggregate.GatedRunnableMs)},
			{types.TraceNoteKeyGatedRunningDeficit, traceQueryObservationMSValue(aggregate.GatedRunningDeficitMs)},
			// CAP (§26 C3): the discounted component's capability caliber.
			// CAP-2: the cluster-topology source rides beside it.
			{types.TraceNoteKeyGatedCapability, aggregate.GatedCapabilitySource},
			{types.TraceNoteKeyGatedClusterTopology, aggregate.GatedClusterTopology},
			{"priority_inversion_gated", traceQueryObservationMSValue(aggregate.PriorityInversionGatedMs)},
			// F3 (§20.2 absorption): the aggregation caliber is disclosed as
			// a typed note so P0-A can parse WHICH ruler produced the gated
			// total (sum_disjoint_occurrences / max_overlap_fallback).
			{"gated_aggregation_caliber", aggregate.GatedAggregationCaliber},
		})...)
	}
	// VS-1 (§7.8): periodic-source cadence + discounted attribution, typed.
	notes = append(notes, traceQueryTypedPeriodicSourceRichNotes(aggregate.PeriodicSource, aggregate.DetectedPeriodMs, aggregate.LatenessMs, aggregate.EffectivePeriodicImpactMs, true)...)
	// VS-2 (§7.10): folded-member supply-fold sums.
	notes = append(notes, traceQueryTypedSupplyFoldRichNotes(aggregate.SupplyFoldBasis, aggregate.SupplyFoldDeficitMs, aggregate.SupplyFoldIdealMs)...)
	return notes
}

func traceQueryTypedOccurrenceWindowRichNotes(items []tracequery.WakeupCausalOccurrence) []string {
	if value := traceQueryOccurrenceWindowsCompact(items, 4); value != "" {
		return []string{types.TraceNoteKeyOccurrenceWindows + "=" + value}
	}
	return nil
}

// traceQueryLockThreadIdentity is one side of the typed lock-span pair
// identity: PID when present, exact comm otherwise, "" for a fully-unresolved
// ref. Both publication lanes read the SAME folded candidate
// (collectBlockingSpanRows), so the two sides always carry identical values —
// only their subject/peer ORIENTATION differs.
func traceQueryLockThreadIdentity(t tracequery.ThreadRef) string {
	if t.PID > 0 {
		return "pid:" + strconv.Itoa(t.PID)
	}
	if comm := strings.TrimSpace(t.Comm); comm != "" {
		return "comm:" + comm
	}
	return ""
}

// traceQueryLockContentionSpanKey (BLK §15.C ①) is the typed physical-span
// identity of one structured lock-contention observation: BlockingKind +
// exact evidence line range + the UNORDERED thread pair — orientation-free on
// purpose, so the holder-subject rank view and the waiter-subject
// critical_blocking view of one span collide on the same key. Empty when the
// kind is missing or the line range is invalid — such rows never fold.
func traceQueryLockContentionSpanKey(kind string, lineStart, lineEnd int, a, b tracequery.ThreadRef) string {
	kind = strings.TrimSpace(kind)
	if kind == "" || lineStart <= 0 || lineEnd < lineStart {
		return ""
	}
	ids := []string{traceQueryLockThreadIdentity(a), traceQueryLockThreadIdentity(b)}
	sort.Strings(ids)
	return strings.Join([]string{kind, strconv.Itoa(lineStart), strconv.Itoa(lineEnd), ids[0], ids[1]}, "\x00")
}

// traceQueryLockContentionTwinIndex (BLK §15.C ①) indexes the structured
// lock-contention critical_blocking rows by physical-span key so the rank
// loop can port a suppressed twin's display-exclusive notes. First row wins
// on a key collision (never observed — the engine folds one row per span).
func traceQueryLockContentionTwinIndex(blocking *tracequery.CriticalBlockingResult) map[string]tracequery.CriticalBlockingCandidate {
	if blocking == nil {
		return nil
	}
	var out map[string]tracequery.CriticalBlockingCandidate
	for _, item := range blocking.Items {
		if item.BlockingKind == "" {
			continue
		}
		key := traceQueryLockContentionSpanKey(item.BlockingKind, item.LineStart, item.LineEnd, item.Thread, item.Peer)
		if key == "" {
			continue
		}
		if out == nil {
			out = map[string]tracequery.CriticalBlockingCandidate{}
		}
		if _, exists := out[key]; !exists {
			out[key] = item
		}
	}
	return out
}

// traceQueryTypedLockTwinPortNotes (BLK §15.C ① + BLK-2 P1/P2) ports the
// display-exclusive note families of the suppressed critical_blocking twin
// onto the surviving rank record, RE-KEYED to the rank record's orientation
// (BLK-2 P1 指代翻转修复): on the twin those families describe the twin's
// PEER — the lock HOLDER — which on the holder-subject rank record is the
// record's OWN SUBJECT, while the rank record's peer= names the blocked
// WAITER. Porting them under the twin's original peer_* keys paired
// peer=<waiter> with peer_state_dominant=<holder state> on ONE record — the
// "等待方 running 主导" false fact the evaluator's peer↔peer_state_* pairing
// teaching then endorses. So: the state breakdown ports as subject_state_*
// and the A1 continuation as subject_chain_*; direction-neutral families
// (waiters / wait_object) keep their keys; peer_source is NOT ported at all
// (it is the twin's HOLDER-resolution origin — the same value the rank row
// already publishes as holder_source; under the original key it would
// mislabel the WAITER's resolution origin, re-keyed it would be a redundant
// duplicate). Keys the rank record already publishes (type / peer /
// blocking_kind / holder_site / holder_source / owner_tid_raw / drill_status
// / chain_relevance / …) are deliberately NOT ported — first-wins consumers
// must keep reading the rank row's own values. The typed lock_twin_folded
// marker (BLK-2 P2) is the precise fold witness: the coverage soft-missing
// scan counts marked rank records as critical_blocking coverage, so the fold
// can never fake a missing-blocking-dimension gap.
func traceQueryTypedLockTwinPortNotes(item tracequery.CriticalBlockingCandidate) []string {
	notes := traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeyLockTwinFolded, "true"},
		{types.TraceNoteKeyWaiters, traceQueryTypedCount(item.Waiters)},
		{types.TraceNoteKeyWaitObject, item.WaitObject},
	})
	notes = append(notes, traceQueryTypedLockTwinSubjectStateNotes(item.PeerState)...)
	if item.PeerChain != nil {
		notes = append(notes, traceQueryTypedLockTwinSubjectChainNotes(item.PeerChain)...)
	}
	return notes
}

// traceQueryTypedLockTwinSubjectStateNotes (BLK-2 P1) renders the folded
// twin's peer-state breakdown under subject_state_* keys: the measured
// thread — the twin's peer, i.e. the lock holder — IS the holder-subject
// rank record's own subject. Key-for-key value mirror of
// traceQueryTypedCriticalBlockingPeerStateNotes; only the referent spelling
// differs. Display tier (NKR).
func traceQueryTypedLockTwinSubjectStateNotes(state *tracequery.ThreadStateBreakdown) []string {
	if state == nil {
		return nil
	}
	return traceQueryTypedKVNotes([][2]string{
		{"subject_state_dominant", state.DominantState},
		{"subject_state_total", traceQueryObservationMSValue(state.TotalMs)},
		{"subject_state_running", traceQueryObservationMSValue(state.RunningMs)},
		{"subject_state_runnable", traceQueryObservationMSValue(state.RunnableMs)},
		{"subject_state_sleep", traceQueryObservationMSValue(state.SleepMs)},
		{"subject_state_d_state", traceQueryObservationMSValue(state.DStateMs)},
		{"subject_state_io_wait", traceQueryObservationMSValue(state.IOWaitMs)},
		{"subject_state_fragments", traceQueryTypedCount(state.FragmentCount)},
	})
}

// traceQueryTypedLockTwinSubjectChainNotes (BLK-2 P1) renders the folded
// twin's A1 bounded continuation under subject_chain_* keys: the
// continuation hangs off the twin's peer (the holder) — the rank record's
// OWN subject — so it is the SUBJECT's dominant state plus the subject's
// single direct 1-hop blocker, never the waiter's. Value mirror of
// traceQueryTypedCriticalBlockingPeerChainNotes. Display tier (NKR).
func traceQueryTypedLockTwinSubjectChainNotes(chain *tracequery.PeerChainStep) []string {
	if chain == nil || chain.State == nil {
		return nil
	}
	kv := [][2]string{
		{"subject_chain_state", chain.State.DominantState},
	}
	if chain.DirectBlocker.PID > 0 || strings.TrimSpace(chain.DirectBlocker.Comm) != "" {
		kv = append(kv,
			[2]string{"subject_chain_blocker", traceThreadLabel(chain.DirectBlocker)},
			[2]string{"subject_chain_blocker_state", chain.DirectBlockerState},
			[2]string{"subject_chain_blocker_source", chain.DirectBlockerSource},
		)
	}
	if chain.Presumptive {
		kv = append(kv, [2]string{"subject_chain_presumptive", "true"})
	}
	return traceQueryTypedKVNotes(kv)
}

func traceQueryTypedCriticalBlockingRichNotes(item tracequery.CriticalBlockingCandidate) []string {
	notes := traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeyType, item.Type},
		{types.TraceNoteKeyPeer, traceThreadLabel(item.Peer)},
		// §7.30.3 D1: typed contention semantics parsed from the structured
		// blocking print payload; renderers key on these, never on prose.
		{types.TraceNoteKeyBlockingKind, item.BlockingKind},
		{types.TraceNoteKeyHolderSite, item.HolderSite},
		// BLOCKFROM (§27.4 G13): waiter-side blocking call site, verbatim.
		{types.TraceNoteKeyBlockingFromSite, item.BlockingFromSite},
		{types.TraceNoteKeyWaiters, traceQueryTypedCount(item.Waiters)},
		// P0-E2a (§10 A2 / §11 N8 / §12 Q4-C): the typed counterpart-resolution
		// origin, the phantom payload owner tid preserved when the wakeup-edge
		// fallback fired, and a payload-less blocking span's wait object.
		{types.TraceNoteKeyHolderSource, item.HolderSource},
		{types.TraceNoteKeyPeerSource, item.PeerSource},
		{types.TraceNoteKeyOwnerTidRaw, traceQueryTypedCount(item.OwnerTidRaw)},
		{types.TraceNoteKeyWaitObject, item.WaitObject},
		// LCK-2 (§18.E/§18.E.1): typed ②×③ identity-unification declaration +
		// process-level ns-span identity (display tier; tgid never a peer PID).
		{types.TraceNoteKeyHolderNsUnification, item.HolderNsUnification},
		{types.TraceNoteKeyHolderHostProcess, item.HolderHostProcess},
		// P0-E 锁车道修2 (§24.9-C F2): hand-off / self-contradiction witnesses.
		{types.TraceNoteKeyHolderHandoff, strings.Join(item.HolderHandoff, " --> ")},
		{types.TraceNoteKeyHolderSelfContradiction, item.HolderSelfContradiction},
		// DCS E4 复核 F-1 (ledger §23.2): a window-clipped blocking span
		// publishes its physical B/E extent on the registered dual-basis
		// actual_* keys (zero-dropped when the span lay fully inside the
		// window) — same disclosure lane as the semantic span observations.
		{types.TraceNoteKeyActualImpactMS, traceQueryObservationMSValue(item.ActualDurationMs)},
		{types.TraceNoteKeyActualWindow, traceQueryWindowValue(item.ActualStartTs, item.ActualEndTs)},
		// RCX① (§12.3 ruling 1): typed drill-debt verdict for the row's
		// counterpart lane (NKR display tier; projection/answer-face
		// consumption is the P0-A batch).
		{"drill_status", item.DrillStatus},
		// G1 跨车道对账 (§27.2-G1, 2026-07-09): the engine's absorption
		// verdict + the canonical family identity (verbatim engine string —
		// the projection compile joins it against the family row's
		// rank_family_key and relocates this node out of the render buckets;
		// the observation itself keeps publishing, 观测照发不删).
		{types.TraceNoteKeyAbsorbedByRankFamily, traceQueryTypedBool(item.AbsorbedByRankFamily)},
		{types.TraceNoteKeyAbsorbedInto, item.AbsorbedIntoFamily},
		{"flags", item.Flags},
		{"oneway", traceQueryTypedBoolPtr(item.Oneway)},
		{"sync_like", traceQueryTypedBoolPtr(item.SyncLike)},
		{"blocking_candidate", traceQueryTypedBoolPtr(item.BlockingCandidate)},
		{types.TraceNoteKeyChainRelevance, item.ChainRelevance},
		{types.TraceNoteKeyOverlap, traceQueryObservationMSValue(item.OverlapMs)},
		{"edge_count", traceQueryTypedCount(item.EdgeCount)},
		{"nearest_chain_thread", traceThreadLabel(item.NearestChainThread)},
	})
	notes = append(notes, traceQueryTypedCriticalBlockingPeerStateNotes(item.PeerState)...)
	// A1 bounded continuation (§12.3-5 ruling 5): ONE sub-goal hop off the
	// resolved counterpart — the peer's own dominant state + its single direct
	// 1-hop blocker (depth hard-capped at 1). peer_chain_presumptive is true when
	// the counterpart itself was only wakeup-edge-resolved. Display tier (NKR);
	// projection/answer-face consumption is the P0-A batch.
	if item.PeerChain != nil {
		notes = append(notes, traceQueryTypedCriticalBlockingPeerChainNotes(item.PeerChain)...)
	}
	return notes
}

// traceQueryTypedCriticalBlockingPeerStateNotes renders the P0-E2a peer-state
// breakdown (display tier) on critical_blocking records, where the described
// thread IS the record's peer. The BLK §15.C ① twin-port lane deliberately
// does NOT share these keys (BLK-2 P1): on the holder-subject rank record the
// same breakdown describes the record's SUBJECT, so it ports re-keyed via
// traceQueryTypedLockTwinSubjectStateNotes.
func traceQueryTypedCriticalBlockingPeerStateNotes(state *tracequery.ThreadStateBreakdown) []string {
	if state == nil {
		return nil
	}
	return traceQueryTypedKVNotes([][2]string{
		{"peer_state_dominant", state.DominantState},
		{"peer_state_total", traceQueryObservationMSValue(state.TotalMs)},
		{"peer_state_running", traceQueryObservationMSValue(state.RunningMs)},
		{"peer_state_runnable", traceQueryObservationMSValue(state.RunnableMs)},
		{"peer_state_sleep", traceQueryObservationMSValue(state.SleepMs)},
		{"peer_state_d_state", traceQueryObservationMSValue(state.DStateMs)},
		{"peer_state_io_wait", traceQueryObservationMSValue(state.IOWaitMs)},
		{"peer_state_fragments", traceQueryTypedCount(state.FragmentCount)},
	})
}

// traceQueryTypedCriticalBlockingPeerChainNotes renders the A1 bounded
// continuation as display-tier notes: the peer's own dominant state, the single
// direct 1-hop blocker (never expanded further) with its ALWAYS-inferred typed
// origin (F2: peer_chain_blocker_source=wakeup_edge whenever a blocker is
// named), and the presumptive flag when the whole continuation hangs off a
// wakeup-edge-inferred counterpart.
func traceQueryTypedCriticalBlockingPeerChainNotes(chain *tracequery.PeerChainStep) []string {
	if chain == nil || chain.State == nil {
		return nil
	}
	kv := [][2]string{
		{"peer_chain_state", chain.State.DominantState},
	}
	if chain.DirectBlocker.PID > 0 || strings.TrimSpace(chain.DirectBlocker.Comm) != "" {
		kv = append(kv,
			[2]string{"peer_chain_blocker", traceThreadLabel(chain.DirectBlocker)},
			[2]string{"peer_chain_blocker_state", chain.DirectBlockerState},
			[2]string{"peer_chain_blocker_source", chain.DirectBlockerSource},
		)
	}
	if chain.Presumptive {
		kv = append(kv, [2]string{"peer_chain_presumptive", "true"})
	}
	return traceQueryTypedKVNotes(kv)
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
		// P0-E CHAIN-PATH (ledger §22.1): branch-stamped nodes carry their
		// TRUE recursion depth (nil-impact transits included) — the pre-P0-E
		// "nil impact defaults to depth 0" artifact only survives on legacy
		// identity-less fixtures (Branch==0), where the old fallback stays.
		depth := node.Depth
		if node.Branch <= 0 && node.Impact != nil {
			depth = node.Impact.ChainDepth
		}
		nodeDepth[node.ID] = depth
	}
	sort.SliceStable(edges, func(i, j int) bool {
		// P0-E CHAIN-PATH: edges group by their REAL branch first (legacy
		// Branch==0 edges all compare equal here — order unchanged), root
		// -deepest first inside one branch.
		if edges[i].Branch != edges[j].Branch {
			return edges[i].Branch < edges[j].Branch
		}
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

// traceQueryWakeupChainBranch is ONE top-level target-segment expansion's
// serialized TRUE parent chain (P0-E CHAIN-PATH 根修, ledger §22.1): the
// engine expands each interesting target segment into a LINEAR waker chain
// (ChainNode.Branch/Depth are typed identities), and the publication layer
// emits one path record per branch. The retired cross-branch flattened walk
// stitched every branch into one pseudo-linear string — the huadong_78
// witness serialized 29 elements with an oney⇄VSync ×7 ping-pong ladder and
// minted fake L26/L27 trunk depths downstream (§24.11 A).
type traceQueryWakeupChainBranch struct {
	Branch                 int
	Path                   string
	Nodes                  int
	Edges                  int
	PriorityInversionEdges int
	LineStart              int
	LineEnd                int
}

// traceQueryWakeupChainBranches derives the per-branch path serializations
// from the typed node identities. Nodes inside one branch sort by Depth
// DESCENDING (deepest root waker first … target last), so every published
// path terminates at the chain target BY CONSTRUCTION — the B1-b overshoot
// truncation is structurally unnecessary on this lane. A branch with no edges
// (single node, nothing resolved upstream) publishes no path, mirroring the
// legacy zero-edge "" behavior. Returns nil on legacy identity-less results
// (no node carries a Branch) — callers keep the legacy flattened fallback for
// that degraded shape only.
func traceQueryWakeupChainBranches(chain tracequery.ChainResult) []traceQueryWakeupChainBranch {
	byBranch := map[int][]tracequery.ChainNode{}
	var order []int
	for _, node := range chain.Nodes {
		if node.Branch <= 0 {
			continue
		}
		if _, ok := byBranch[node.Branch]; !ok {
			order = append(order, node.Branch)
		}
		byBranch[node.Branch] = append(byBranch[node.Branch], node)
	}
	if len(order) == 0 {
		return nil
	}
	sort.Ints(order)
	edgesByBranch := map[int][]tracequery.WakeupEdge{}
	for _, edge := range chain.Edges {
		if edge.Branch > 0 {
			edgesByBranch[edge.Branch] = append(edgesByBranch[edge.Branch], edge)
		}
	}
	var out []traceQueryWakeupChainBranch
	for _, branch := range order {
		nodes := byBranch[branch]
		edges := edgesByBranch[branch]
		if len(edges) == 0 {
			continue // single-node branch: no chain to publish
		}
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].Depth > nodes[j].Depth
		})
		labels := make([]string, 0, len(nodes))
		for _, node := range nodes {
			labels = append(labels, traceThreadLabel(node.Thread))
		}
		br := traceQueryWakeupChainBranch{
			Branch: branch,
			Path:   strings.Join(labels, " -> "),
			Nodes:  len(nodes),
			Edges:  len(edges),
		}
		addLine := func(line int) {
			if line <= 0 {
				return
			}
			if br.LineStart == 0 || line < br.LineStart {
				br.LineStart = line
			}
			if line > br.LineEnd {
				br.LineEnd = line
			}
		}
		for _, edge := range edges {
			if edge.PriorityInversionCandidate {
				br.PriorityInversionEdges++
			}
			addLine(edge.WakeupLine)
			addLine(edge.EvidenceLine)
		}
		for _, node := range nodes {
			if node.Impact != nil {
				addLine(node.Impact.LineStart)
				addLine(node.Impact.LineEnd)
			}
		}
		out = append(out, br)
	}
	return out
}

// traceQueryWakeupChainBranchPathByID maps each branch ordinal to its
// serialized path so per-edge records can name their OWNING branch's path
// instead of the retired cross-branch flattened string.
func traceQueryWakeupChainBranchPathByID(branches []traceQueryWakeupChainBranch) map[int]string {
	if len(branches) == 0 {
		return nil
	}
	out := make(map[int]string, len(branches))
	for _, br := range branches {
		out[br.Branch] = br.Path
	}
	return out
}

// traceQueryWakeupChainPath — LEGACY FALLBACK LANE ONLY (P0-E CHAIN-PATH
// EVOLUTION, ledger §22.1, 2026-07-09). The cross-branch flattened walk is
// RETIRED from every publication face for branch-stamped engine results:
// traceQueryWakeupChainBranches serializes one TRUE path per branch instead.
// This function survives solely for identity-less ChainResults (no
// ChainNode.Branch — hand-built fixtures / degraded shapes), where the B1-b
// overshoot truncation below remains the honest trim. Do NOT route a
// branch-stamped result back through this walk: the huadong_78 witness shape
// re-serializes an oney⇄VSync ×7 ping-pong ladder with fake trunk depths
// (pinned red by TestTraceQueryWakeupChainBranchPathsHuadongShape).
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
	// §22 B1-b F1(b) (huadong_01 CHAIN-PATH audit, 2026-07-07): the flattened
	// walk above serializes the multi-branch edge set in From-depth order, and
	// nil-impact transit nodes default to depth 0 — their edges sort LAST, so
	// the walk can overshoot chain.Target and end on an artifact transit node
	// while the true target sits mid-path. A path record must terminate at its
	// typed target: truncate at the target label's LAST occurrence (earlier
	// occurrences are the legitimate ↺ cycle shape; everything after the last
	// one is overshoot). Target absent from the walk (or unset) keeps the
	// legacy full walk — fail-open, no hard gate on a noisy shape. Display/LLM
	// face only: the individual wakeup_chain_edge records still publish every
	// edge, and the tracequery rank/attribution lanes never read this string.
	if target := traceThreadLabelOptional(chain.Target); target != "" {
		for i := len(labels) - 1; i >= 0; i-- {
			if labels[i] == target {
				labels = labels[:i+1]
				break
			}
		}
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
		{types.TraceNoteKeyPath, path},
		{"target", traceThreadLabel(chain.Target)},
		{"edges", traceQueryTypedCount(len(chain.Edges))},
		{"nodes", traceQueryTypedCount(len(chain.Nodes))},
		{"priority_inversion_edges", traceQueryTypedCount(priorityInversions)},
		{types.TraceNoteKeyWindow, traceQueryWindowValue(chain.Window.StartTs, chain.Window.EndTs)},
	})
}

// traceQueryTypedWakeupBranchPathRichNotes is the per-branch path record's
// typed note face (P0-E CHAIN-PATH, ledger §22.1): the same chain_path-family
// keys the retired flattened record carried, with edge/node/inversion counts
// scoped to THIS branch, plus the typed branch ordinal and the total branch
// count. The branch= note is the precise form marker the projection election
// keys its candidate-pool switch on — never a string-shape heuristic.
func traceQueryTypedWakeupBranchPathRichNotes(chain tracequery.ChainResult, br traceQueryWakeupChainBranch, branchTotal int) []string {
	return traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeyPath, br.Path},
		{"target", traceThreadLabel(chain.Target)},
		{types.TraceNoteKeyChainPathBranch, traceQueryTypedCount(br.Branch)},
		{types.TraceNoteKeyChainPathBranches, traceQueryTypedCount(branchTotal)},
		{"edges", traceQueryTypedCount(br.Edges)},
		{"nodes", traceQueryTypedCount(br.Nodes)},
		{"priority_inversion_edges", traceQueryTypedCount(br.PriorityInversionEdges)},
		{types.TraceNoteKeyWindow, traceQueryWindowValue(chain.Window.StartTs, chain.Window.EndTs)},
	})
}

func traceQueryTypedWakeupEdgeRichNotes(edge tracequery.WakeupEdge, path string) []string {
	return traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeyPath, path},
		{"wakeup_ts", traceQueryTimestampValue(edge.WakeupTs)},
		{"latency", traceQueryObservationMSValue(edge.LatencyMs)},
		{"waker_priority", traceQueryPriorityPair(edge.WakerPriority, edge.WakerPriorityClass)},
		{"wakee_priority", traceQueryPriorityPair(edge.WakeePriority, edge.WakeePriorityClass)},
		{"priority_relation", edge.PriorityRelation},
		{types.TraceNoteKeyPriorityInversionCandidate, traceQueryTypedBool(edge.PriorityInversionCandidate)},
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

// traceQueryRunnableOccupancyWindowShare is the relative arm of the RN-1
// (§7.9) significance gate: a thread's in-window runnable total is significant
// when it reaches 10% of the wall window. Precise arithmetic comparison
// against the engine's own WindowMs — never a heuristic score.
const traceQueryRunnableOccupancyWindowShare = 0.10

// traceQueryRunnableOccupancyAbsoluteFloorMs is the absolute arm of the same
// gate (§7.10 显著门二审裁定, user 2026-07-04): a pure relative threshold is
// DILUTED by wide windows — 10% of a 3.3s window is 330ms, so a 200ms
// absolute backlog (24 whole 120fps frame budgets) read as "not significant".
// Same defect family as CMP-9 cross-window dilution. The combined formula
// min(window×10%, 100ms) keeps ONE gate that only ever gets LOOSER as the
// window widens: narrow windows are governed by the relative arm, wide
// windows by the 100ms floor. The threshold drives SOFT surfaces only
// (observation publishing, wording) — never a hard structural gate.
const traceQueryRunnableOccupancyAbsoluteFloorMs = 100.0

// traceQueryRunnableSignificanceThresholdMs is THE shared RN-1/VS-2
// significance threshold (§7.10: the supply-fold decision table must stay
// 同源 with the RN-1 occupier-observation gate — one formula, no fork):
//
//	runnable_total ≥ min(windowMs × 10%, 100ms)
//
// windowMs ≤ 0 (unbounded window) returns 0 and the companion predicate
// refuses — no denominator, no significance verdict.
func traceQueryRunnableSignificanceThresholdMs(windowMs float64) float64 {
	if windowMs <= 0 {
		return 0
	}
	relative := windowMs * traceQueryRunnableOccupancyWindowShare
	if relative > traceQueryRunnableOccupancyAbsoluteFloorMs {
		return traceQueryRunnableOccupancyAbsoluteFloorMs
	}
	return relative
}

// traceQueryRunnableSignificant is the shared predicate over the threshold
// above, consumed by BOTH the RN-1 publisher and the VS-2 (§7.10) supply-fold
// decision table. Soft-face significance only.
func traceQueryRunnableSignificant(runnableMs, windowMs float64) bool {
	return windowMs > 0 && runnableMs > 0 &&
		runnableMs >= traceQueryRunnableSignificanceThresholdMs(windowMs)
}

// traceQueryRunnableOccupancyObservations publishes AT MOST ONE typed
// "runnable_occupancy" observation per result (RN-1, §7.9 cust_runnable
// 2026-07-04): when the query carried a complete two-sided time window
// (CPUOccupancy.WindowMs > 0 — queryWindowWallMs refuses to estimate) and some
// thread waited runnable for ≥ min(window×10%, 100ms) (§7.10 dual-basis
// significance gate), the CMP-8 occupancy decomposition
// already knows WHO occupied the CPUs — publish the starved thread (largest
// runnable) with its top-3 same-window occupiers (full-window cpu·ms order,
// the starved subject itself excluded) so the projection can attach the
// mechanism to the runnable row instead of a bare percentage. Multiple
// significant starved threads fold into a typed also_starved count.
func traceQueryTypedRunnableOccupancyObservations(stats tracequery.WindowStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	occ := stats.CPUOccupancy
	if occ == nil || occ.WindowMs <= 0 {
		return nil
	}
	var starved *tracequery.ThreadDuration
	significant := 0
	for i := range stats.RunnableTop {
		td := &stats.RunnableTop[i]
		if strings.TrimSpace(traceThreadLabel(td.Thread)) == "" || td.DurationMs <= 0 {
			continue
		}
		if traceQueryRunnableSignificant(td.DurationMs, occ.WindowMs) {
			significant++
			if starved == nil || td.DurationMs > starved.DurationMs {
				starved = td
			}
		}
	}
	if starved == nil {
		return nil
	}
	subject := traceThreadLabel(starved.Thread)
	notes := [][2]string{
		{types.TraceNoteKeyStarvedRunnableMS, fmt.Sprintf("%.3f", starved.DurationMs)},
	}
	var occupierParts []string
	occupiers := 0
	for _, top := range occ.TopThreads {
		if top.RunningMs <= 0 {
			continue
		}
		// The starved subject may also appear in the occupancy ranking; its
		// own running time is not "who kept it off the CPU" — exclude by
		// exact PID+Comm identity.
		if top.Thread.PID == starved.Thread.PID && top.Thread.Comm == starved.Thread.Comm {
			continue
		}
		occupiers++
		value := fmt.Sprintf("%s:%.3fms", traceThreadLabel(top.Thread), top.RunningMs)
		notes = append(notes, [2]string{fmt.Sprintf("%s%d", types.TraceNoteKeyOccupierPrefix, occupiers), value})
		occupierParts = append(occupierParts, value)
		if occupiers >= 3 {
			break
		}
	}
	if occupiers == 0 {
		// No same-window occupier besides the subject itself: the attribution
		// claim would be empty — occupancy-side silence, not a zero-value row.
		return nil
	}
	notes = append(notes, [2]string{types.TraceNoteKeyWindowMS, fmt.Sprintf("%.3f", occ.WindowMs)})
	if significant > 1 {
		notes = append(notes, [2]string{types.TraceNoteKeyAlsoStarved, strconv.Itoa(significant - 1)})
	}
	notes = append(notes, [2]string{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(stats.Window)})
	return []types.ObservationRecord{{
		ID:              fmt.Sprintf("trace_query:%s#runnable_occupancy:1", scope),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
		SourceRef:       ref,
		Span:            types.ObservationSpan{LineStart: starved.LineStart, LineEnd: starved.LineEnd, StartTs: starved.StartTs, EndTs: starved.EndTs},
		ClaimKey:        "runnable_occupancy",
		Subject:         subject,
		Predicate:       "runnable_occupancy",
		Object:          "runnable",
		Value:           traceQueryObservationMSValue(starved.DurationMs),
		Unit:            "ms",
		Summary: fmt.Sprintf("runnable_occupancy %s runnable=%.3fms window=%.3fms top_occupiers=%s (cpu·ms)",
			subject, starved.DurationMs, occ.WindowMs, strings.Join(occupierParts, ", ")),
		RichNotes:   traceQueryTypedKVNotes(notes),
		SupportRefs: traceQueryObservationSupportRefs(ref, starved.LineStart, starved.LineEnd),
		ObservedAt:  at,
		Confidence:  0.75,
	}}
}

func traceQueryTypedWindowStatsObservations(stats tracequery.WindowStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	var out []types.ObservationRecord

	out = append(out, traceQueryTypedThreadDurationObservations(stats.TopRunning, stats.Window, ref, scope, at, "top_running", "running_time", "running", "selected-window running time", 0.70)...)
	out = append(out, traceQueryTypedThreadDurationObservations(stats.RunnableTop, stats.Window, ref, scope, at, "top_runnable", "runnable_wait", "runnable", "selected-window runnable wait", 0.75)...)
	out = append(out, traceQueryTypedThreadDurationObservations(stats.SleepTop, stats.Window, ref, scope, at, "top_sleep", "sleep_wait", "sleep", "selected-window sleep before wakeup", 0.76)...)
	out = append(out, traceQueryTypedThreadDurationObservations(stats.DStateTop, stats.Window, ref, scope, at, "top_d_state", "d_state_or_io_wait", "d_state", "selected-window D-state or IO-like wait", 0.80)...)
	out = append(out, traceQueryTypedThreadDurationObservations(stats.IOWaitTop, stats.Window, ref, scope, at, "top_io_wait", "io_wait", "io_wait", "selected-window IO wait", 0.82)...)
	// RN-1 (§7.9): significant runnable starvation gets its same-window
	// occupier attribution published as a ledger observation — without it the
	// customer's runnable-dominant report (FFRT runnable 2528ms of a 3000ms
	// window) had no mechanism explanation at all.
	out = append(out, traceQueryTypedRunnableOccupancyObservations(stats, ref, scope, at)...)

	for i, load := range stats.ThreadCPULoad {
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
				{types.TraceNoteKeyRunning, traceQueryObservationMSValue(load.RunningMs)},
				{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(load.RunnableWaitMs)},
				{"high_prio_running", traceQueryObservationMSValue(load.HighPriorityRunningMs)},
				{"system_or_kernel_running", traceQueryObservationMSValue(load.SystemOrKernelRunningMs)},
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
				{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(constraint.RunnableWaitMs)},
				{"other_cpu_idle", traceQueryObservationMSValue(constraint.OtherCPUIdleMs)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, constraint.LineStart, constraint.LineEnd),
			ObservedAt:  at,
			Confidence:  0.72,
		})
	}

	for i, ctx := range stats.RunnableContext {
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
				{types.TraceNoteKeyRunning, traceQueryObservationMSValue(proc.RunningMs)},
				{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(proc.RunnableWaitMs)},
				{"high_prio_running", traceQueryObservationMSValue(proc.HighPriorityRunningMs)},
				{"system_or_kernel_running", traceQueryObservationMSValue(proc.SystemOrKernelRunningMs)},
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
		appendNote(types.TraceNoteKeyDominantState, churn.DominantState)
		if strings.HasPrefix(strings.TrimSpace(churn.Summary), "state_cluster ") {
			appendNote("coverage_mode", "state_cluster")
		}
		if churn.FragmentCount > 0 {
			appendNote(types.TraceNoteKeyFragments, strconv.Itoa(churn.FragmentCount))
		}
		if churn.StateSwitches > 0 {
			appendNote(types.TraceNoteKeySwitches, strconv.Itoa(churn.StateSwitches))
		}
		appendNote(types.TraceNoteKeyMaxSegment, traceQueryObservationMSValue(churn.MaxSegmentMs))
		appendNote(types.TraceNoteKeyP95Segment, traceQueryObservationMSValue(churn.P95SegmentMs))
		appendNote(types.TraceNoteKeyRunning, traceQueryObservationMSValue(churn.RunningMs))
		appendNote(types.TraceNoteKeyRunnable, traceQueryObservationMSValue(churn.RunnableMs))
		appendNote(types.TraceNoteKeySleep, traceQueryObservationMSValue(churn.SleepMs))
		appendNote(types.TraceNoteKeyDState, traceQueryObservationMSValue(churn.DStateMs))
		appendNote(types.TraceNoteKeyIOWait, traceQueryObservationMSValue(churn.IOWaitMs))
		if churn.RunnableCPUKnown {
			appendNote(types.TraceNoteKeyRunnableCPU, strconv.Itoa(churn.RunnableCPU))
		}
		appendNote(types.TraceNoteKeyTopCompetitor, churn.TopCompetitor)
		appendNote("top_competitor_overlap", traceQueryObservationMSValue(churn.TopCompetitorOverlapMs))
		appendNote("top_competitor_running", traceQueryObservationMSValue(churn.TopCompetitorRunningMs))
		appendNote(types.TraceNoteKeyNextStep, churn.NextStep)
		appendNote(types.TraceNoteKeyNextStepKind, churn.NextStepKind)
		if churn.TotalMs > 0 {
			notes = append(notes, fmt.Sprintf("%s=%.3fms", types.TraceNoteKeyTotal, churn.TotalMs))
		}
		// NEW-8 (账本 §7.6): state_churn rows are the metric-snapshot source —
		// publish the typed selected-window note (stats.Window = the resolved
		// q.TimeStart/TimeEnd) so the snapshot's window-basis line can render
		// the endpoints. Emitted only for a real two-sided window, same
		// semantics as every other family.
		appendNote(types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(stats.Window))
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
				{types.TraceNoteKeyRank, traceQueryTypedCount(step.Rank)},
				{"state", step.State},
				{types.TraceNoteKeyImpact, traceQueryObservationMSValue(step.ImpactMs)},
				{"rank_impact", traceQueryObservationMSValue(step.RankImpactMs)},
				{types.TraceNoteKeyTotal, traceQueryObservationMSValue(step.TotalMs)},
				{types.TraceNoteKeySource, step.Source},
				{types.TraceNoteKeyRecommendedViews, strings.Join(step.RecommendedViews, ",")},
				{types.TraceNoteKeyChainRequired, strconv.FormatBool(step.ChainRequired)},
				{types.TraceNoteKeyRecursive, strconv.FormatBool(step.Recursive)},
				{"window_proportion", strconv.FormatFloat(step.WindowProportion, 'f', 4, 64)},
				{types.TraceNoteKeySignificant, strconv.FormatBool(step.Significant)},
				{types.TraceNoteKeyWindow, traceQueryWindowValue(step.StartTs, step.EndTs)},
				// NEW-8 (账本 §7.6): the step's own `window` above is the state
				// segment; the selected QUERY window travels via the same typed
				// note as every other selected-window family.
				{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(stats.Window)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, step.LineStart, step.LineEnd),
			ObservedAt:  at,
			Confidence:  0.74,
		})
	}

	// INODE (§28.6): the whole-window (dev,inode) frequency groups ride their
	// own typed observation family (claimKey prefix "top_io_inode:"). Each
	// row carries the groups_total disclosure note so the typed lane stays
	// truncation-honest like the banner tail; latency notes follow the
	// wall-clock red line (max single event + per-thread within-thread sums).
	if stats.TopIOInodes != nil {
		groupsTotal := strconv.Itoa(stats.TopIOInodes.TotalGroups)
		for i, g := range stats.TopIOInodes.Groups {
			if i >= traceQueryWidthTypedFamilyRowCap() {
				break
			}
			if strings.TrimSpace(g.Inode) == "" {
				continue
			}
			out = append(out, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:%s#top_io_inode:%d", scope, i+1),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
				SourceRef:       ref,
				Span:            types.ObservationSpan{LineStart: g.LineStart, LineEnd: g.LineEnd, StartTs: g.StartTs, EndTs: g.EndTs},
				ClaimKey:        "top_io_inode:" + firstNonEmptyTraceString(g.Inode, g.EntryName),
				Subject:         firstNonEmptyTraceString(g.EntryName, "inode="+g.Inode),
				Predicate:       "top_io_inode",
				Object:          g.Dev,
				Value:           traceQueryTypedCount(g.Count),
				Unit:            "events",
				Summary:         traceQueryTypedTopIOInodeSummary(g),
				RichNotes: traceQueryTypedKVNotes([][2]string{
					{"inode", g.Inode},
					{"dev", g.Dev},
					{"name", g.EntryName},
					{"count", traceQueryTypedCount(g.Count)},
					{"reads", traceQueryTypedCount(g.ReadCount)},
					{"writes", traceQueryTypedCount(g.WriteCount)},
					{"completions", traceQueryTypedCount(g.CompletionCount)},
					{"bytes", traceQueryTypedInt64(g.Bytes)},
					{"adds", traceQueryTypedCount(g.PageCacheAdds)},
					{"deletes", traceQueryTypedCount(g.PageCacheDeletes)},
					{"max_latency", traceQueryObservationMSValue(g.MaxLatencyMs)},
					{"threads", traceQueryTypedCount(g.ThreadCount)},
					{"top_threads", traceTopIOInodeThreadRoster(g.TopThreadLatencies)},
					{"groups_total", groupsTotal},
				}),
				SupportRefs: traceQueryObservationSupportRefs(ref, g.LineStart, g.LineEnd),
				ObservedAt:  at,
				Confidence:  0.74,
			})
		}
	}

	for i, file := range stats.FileIOByInode {
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
				{"ambiguous_cohorts", traceQueryTypedCount(storage.AmbiguousCohortCount)},
				{"pairing_suppressed", traceQueryTypedCount(storage.PairingSuppressedCount)},
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
				{types.TraceNoteKeyDState, traceQueryObservationMSValue(pressure.DStateMs)},
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
				{types.TraceNoteKeyChainRelevance, episode.ChainRelevance},
				{"signal", episode.DominantSignal},
				{types.TraceNoteKeyDState, traceQueryObservationMSValue(episode.DStateMs)},
				{types.TraceNoteKeyIOWait, traceQueryObservationMSValue(episode.IOWaitMs)},
				{"block_max", traceQueryObservationMSValue(episode.BlockMaxLatencyMs)},
				{"storage_max", traceQueryObservationMSValue(episode.StorageMaxLatencyMs)},
				{"inode", episode.TopInode},
				{"dev", episode.TopDev},
				{"name", episode.TopEntryName},
				{"file_bytes", traceQueryTypedInt64(episode.FileIOBytes)},
				{"page_cache_churn", traceQueryTypedCount(episode.PageCacheChurn)},
				{types.TraceNoteKeyOverlap, traceQueryObservationMSValue(episode.OverlapMs)},
				{"nearest_chain_thread", traceThreadLabelOptional(episode.NearestChainThread)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, episode.LineStart, episode.LineEnd),
			ObservedAt:  at,
			Confidence:  episode.Confidence,
		})
	}

	for i, inode := range stats.BlockIOByInode {
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
				{"unpaired_start", traceQueryTypedCount(work.UnpairedStartCount)},
				{"unpaired_done", traceQueryTypedCount(work.UnpairedDoneCount)},
				{"ambiguous_cohorts", traceQueryTypedCount(work.AmbiguousCohortCount)},
				{"pairing_suppressed", traceQueryTypedCount(work.PairingSuppressedCount)},
				{"max", traceQueryObservationMSValue(work.MaxLatencyMs)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, work.LineStart, work.LineEnd),
			ObservedAt:  at,
			Confidence:  0.64,
		})
	}
	for i, fence := range stats.DMAFenceActivity {
		if i >= traceQueryWidthTypedFamilyRowCap() {
			break
		}
		if strings.TrimSpace(fence.Summary) == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#dma_fence_activity:%d", scope, i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{LineStart: fence.LineStart, LineEnd: fence.LineEnd, StartTs: fence.StartTs, EndTs: fence.EndTs},
			ClaimKey:        "dma_fence_activity:" + firstNonEmptyTraceString(fence.Timeline, fence.Driver, fence.Seqno),
			Subject:         traceThreadLabel(fence.Thread),
			Predicate:       "dma_fence_activity",
			Object:          firstNonEmptyTraceString(fence.Timeline, fence.Driver, fence.Seqno),
			Value:           traceQueryObservationMSValue(fence.WaitMs),
			Unit:            "ms",
			Summary:         fence.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{"driver", fence.Driver},
				{"timeline", fence.Timeline},
				{"context", fence.Context},
				{"seqno", fence.Seqno},
				{"count", traceQueryTypedCount(fence.Count)},
				{"paired", traceQueryTypedCount(fence.PairedCount)},
				{"unpaired_start", traceQueryTypedCount(fence.UnpairedStartCount)},
				{"unpaired_done", traceQueryTypedCount(fence.UnpairedDoneCount)},
				{"ambiguous_cohorts", traceQueryTypedCount(fence.AmbiguousCohortCount)},
				{"pairing_suppressed", traceQueryTypedCount(fence.PairingSuppressedCount)},
				{"max", traceQueryObservationMSValue(fence.MaxWaitMs)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, fence.LineStart, fence.LineEnd),
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
				{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(supply.RunnableWaitMs)},
				{"high_prio", traceQueryObservationMSValue(supply.HighPriorityRunningMs)},
				{"system_or_kernel_running", traceQueryObservationMSValue(supply.SystemOrKernelRunningMs)},
				{"system_or_kernel_overlap", traceQueryObservationMSValue(supply.SystemOrKernelRunningOverlapMs)},
				{"system_or_kernel_competitors", traceQueryTypedCount(supply.SystemOrKernelCompetitorCount)},
				{"low_freq_cpus", traceIntList(supply.LowFrequencyCPUs)},
				{"clock_set_rate", traceQueryTypedCount(supply.ClockSetRateCount)},
				{"thermal", traceQueryTypedCount(supply.ThermalEventCount)},
				{"ddr", traceQueryTypedCount(supply.DDREventCount)},
				{"l3", traceQueryTypedCount(supply.L3EventCount)},
				{"throughput", traceQueryTypedCount(supply.ThroughputEventCount)},
				// CMP-9 (§7.3): typed window + normalized density for
				// downstream cross-trace comparison ("" when the window is
				// unbounded — the KV helper drops empty values, no estimate).
				{types.TraceNoteKeyWindowMS, traceQueryObservationWindowMsValue(supply.WindowMs)},
				{types.TraceNoteKeyPressureDensity, traceQueryObservationDensityValue(supply.PressureDensity)},
				// CMP-10 (§7.4) guidance: the demand-backlog aggregate is a
				// dead end on its own — point at the occupancy side.
				{types.TraceNoteKeyRecommendedViews, "window_stats"},
				{"recommended_sections", "cpu_occupancy,compute_supply_balance,process_cpu_load"},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, supply.LineStart, supply.LineEnd),
			ObservedAt:  at,
			Confidence:  0.62,
		})
	}
	// CMP-10 / adversarial review 2026-07-04 F5: the compute-supply ledger is
	// published as exactly ONE typed observation per window_stats result (the
	// scope-keyed ID rides the existing dedup channel), so the downstream
	// comparison lane can consume the supply ratio and its decomposition
	// without re-parsing the rendered stanza. Rich notes always carry the
	// full contract set (%.3f floats + %d cpu_count, never the ""-dropping
	// ms helper — a 0.000 low_freq_loss is a fact, not an absence). No
	// balance (unbounded window / no sched-observed CPU) publishes nothing.
	if bal := stats.ComputeSupplyBalance; bal != nil {
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#compute_supply_balance:1", scope),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span:            types.ObservationSpan{StartTs: stats.Window.StartTs, EndTs: stats.Window.EndTs},
			ClaimKey:        "compute_supply_balance",
			Subject:         "compute_supply_balance",
			Predicate:       "compute_supply_balance",
			Value:           fmt.Sprintf("%.3f", bal.SupplyRatio),
			Summary:         bal.Summary,
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{types.TraceNoteKeySupplyRatio, fmt.Sprintf("%.3f", bal.SupplyRatio)},
				{"delivered_cpu_ms", fmt.Sprintf("%.3f", bal.DeliveredComputeMs)},
				{"low_freq_loss_cpu_ms", fmt.Sprintf("%.3f", bal.LowFrequencyLossMs)},
				{types.TraceNoteKeyIdleMismatchMS, fmt.Sprintf("%.3f", bal.IdleMismatchMs)},
				{"core_limited_cpu_ms", fmt.Sprintf("%.3f", bal.CoreLimitedMs)},
				{types.TraceNoteKeyWindowMS, fmt.Sprintf("%.3f", bal.WindowMs)},
				{"cpu_count", fmt.Sprintf("%d", bal.CPUCount)},
			}),
			ObservedAt: at,
			Confidence: 0.62,
		})
	}
	for i, category := range stats.TraceMarkCategories {
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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
			if i >= traceQueryWidthTypedFamilyRowCap() {
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
					{types.TraceNoteKeyDSO, hot.DSO},
					{"event", hot.Event},
					{"weight_unit", hot.WeightUnit},
					{types.TraceNoteKeySource, hot.Source},
					{"symbolization_status", hot.SymbolizationStatus},
					{types.TraceNoteKeyPerfQuality, traceQueryPerfQualityCompact(stats.PerfSamples.Quality)},
					{types.TraceNoteKeyPerfQualityCaveats, strings.Join(perfQualityCaveatsForTraceQuery(stats.PerfSamples.Quality), "; ")},
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
	// RCM §24.10/§24.12 (2026-07-08): the observation channel consumes the
	// SAME family fold as the rank minting loop (tracequery.
	// FoldSemanticSpanFamilies — one function, two consumers), so a
	// multi-span same-thread family publishes ONE record whose Value is the
	// family's window-projection total. A family of one keeps the pre-RCM
	// per-span record byte-identically (退化不变体).
	families := tracequery.FoldSemanticSpanFamilies(chain, stats.TraceSpans)
	out := make([]types.ObservationRecord, 0, minInt(len(families), traceQueryWidthTypedFamilyRowCap()))
	ordinal := 0
	// RCM-2 复核 F-4 (2026-07-08): the channel's background-comprehensive-
	// board position. Counting mirrors the rank lane's discipline (DCS §23.1:
	// the POSITION counts every published non-on-chain row; the FIELD is
	// stamped narrowly) — here every emitted non-on-chain record counts, and
	// the field lands on multi-member FAMILY records only (single-span
	// records stay byte-identical, 单员族逐字退化 pin). Without this note the
	// ✦ observation row's BackgroundRank was unmintable in production and the
	// 行2 背景榜位#N seat could never render.
	backgroundPos := 0
	for _, fam := range families {
		if ordinal >= traceQueryWidthTypedFamilyRowCap() {
			break
		}
		if len(fam.Members) > 1 {
			ordinal++
			backgroundRank := 0
			if !fam.OnChain {
				backgroundPos++
				backgroundRank = backgroundPos
			}
			out = append(out, traceQuerySemanticSpanFamilyObservation(fam, chain, stats, ref, scope, at, ordinal, backgroundRank))
			continue
		}
		span := fam.Members[0]
		semanticClass := strings.TrimSpace(span.SemanticClass)
		if semanticClass == "" || span.DurationMs <= 0 {
			continue
		}
		ordinal++
		// 审计复核 R1 (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10):
		// the single-member record derives lane/depth/overlap from the SAME
		// family fold the rank mint prices (fam.OnChain/fam.ProjectedImpactMs
		// — semanticTraceSpanChainIntersectionProjection, computed for
		// single-member families too), so the observation's overlap note and
		// the rank row's published EffectiveImpactMs are ONE value source.
		// The retired traceQuerySemanticTraceSpanContext lane took the best
		// SINGLE chain-window overlap (max): a span crossing several disjoint
		// same-thread chain windows published overlap=3.000 beside the
		// engine's cross-window intersection union 5.500 — the typed twin
		// mirror then failed structurally, the ✦/❶ twin seats returned, and
		// the detail cell surfaced a value the engine never published.
		ctx := traceQuerySemanticSpanFamilyFoldContext(fam, span, chain)
		if ctx.chainRelevance != "on_chain" {
			// F-4 counting: single-span records hold a board position (they
			// compete on the same board) but carry no stamped field.
			backgroundPos++
		}
		notes := traceQueryTypedKVNotes([][2]string{
			{types.TraceNoteKeySpanName, span.Name},
			{types.TraceNoteKeySpanKind, firstNonEmptyTraceString(span.Kind, "sync")},
			{types.TraceNoteKeySpanCategory, span.Category},
			{types.TraceNoteKeySpanSubcategory, span.Subcategory},
			{types.TraceNoteKeySemanticClass, semanticClass},
			{types.TraceNoteKeyChainRelevance, ctx.chainRelevance},
			{types.TraceNoteKeyCausality, ctx.causality},
			{types.TraceNoteKeyChainDepth, traceQueryTypedCount(ctx.chainDepth)},
			{types.TraceNoteKeyOverlap, traceQueryObservationMSValue(ctx.overlapMs)},
			{types.TraceNoteKeyWindow, traceQueryWindowValue(span.StartTs, span.EndTs)},
			// DCS E5 producer half (ledger §23 H2, cmp_01 E2 witness "83% 对
			// 锚窗", 2026-07-08): the row's typed SOURCE-window identity — the
			// window_stats query window these spans were computed over. The
			// display's window-share % must divide by THIS window, never by a
			// different anchor window the row happens to be rendered under.
			// Same key+format as the CWD-2 rank-row identity; the anchor
			// family whitelist does not admit trace_semantic_span rows, so
			// this can never re-anchor a projection.
			{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(stats.Window)},
			// DCS E4: a boundary-straddling span publishes its window-clipped
			// duration as the Value; the physical B/E extent rides the
			// registered dual-basis actual_* keys (absent when not clipped).
			{types.TraceNoteKeyActualImpactMS, traceQueryObservationMSValue(span.ActualDurationMs)},
			{types.TraceNoteKeyActualWindow, traceQueryWindowValue(span.ActualStartTs, span.ActualEndTs)},
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

// traceQuerySemanticSpanFamilyObservation renders the ONE typed record of a
// multi-member semantic span family (RCM §24.10/§24.12): Value = the family's
// complete selected-window member union (lossless observation), while an
// on-chain family's projected_impact/overlap notes carry the narrower exact
// member∩chain union that alone participates in causal ranking. Span = the
// member envelope (CMP-A precedent:
// Span is the member-impact envelope, never the window — the row's window
// identity stays on the selected_window note, DCS E5 lane unchanged), and the
// member accounting rides the member_* typed keys. The chain lane is the FOLD
// lane verbatim (two consumers, one 道别 predicate) — the record can never
// publish a lane the rank row disagrees with; a non-chain family degrades to
// adjacent/background by envelope-vs-chain-window overlap exactly like the
// single-span context tail.
func traceQuerySemanticSpanFamilyObservation(fam tracequery.SemanticSpanFamily, chain *tracequery.ChainResult, stats tracequery.WindowStats, ref types.ObservationSourceRef, scope, at string, ordinal, backgroundRank int) types.ObservationRecord {
	rep := fam.Members[0]
	relevance, causality := "", ""
	chainDepth := 0
	if fam.OnChain {
		relevance, causality, chainDepth = "on_chain", "on_wakeup_chain", fam.ChainDepth
	} else if chain != nil && (len(chain.Nodes) > 0 || len(chain.CausalImpacts) > 0 || len(chain.Edges) > 0) {
		if traceQueryWindowOverlapMS(fam.StartTs, fam.EndTs, chain.Window.StartTs, chain.Window.EndTs) > 0 {
			relevance, causality = "adjacent", "adjacent_to_wakeup_chain"
		} else {
			relevance, causality = "background", "background"
		}
	}
	memberSum := ""
	if fam.TotalMs < fam.SumMs {
		memberSum = traceQueryObservationMSValue(fam.SumMs)
	}
	projectedImpact, overlap := "", ""
	if fam.OnChain {
		projectedImpact = traceQueryObservationMSValue(fam.ProjectedImpactMs)
		overlap = projectedImpact
	}
	notes := traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeySpanName, rep.Name},
		{types.TraceNoteKeySpanKind, firstNonEmptyTraceString(rep.Kind, "sync")},
		{types.TraceNoteKeySpanCategory, rep.Category},
		{types.TraceNoteKeySpanSubcategory, rep.Subcategory},
		{types.TraceNoteKeySemanticClass, fam.SemanticClass},
		{types.TraceNoteKeyChainRelevance, relevance},
		{types.TraceNoteKeyCausality, causality},
		{types.TraceNoteKeyChainDepth, traceQueryTypedCount(chainDepth)},
		{types.TraceNoteKeyProjectedImpact, projectedImpact},
		{types.TraceNoteKeyOverlap, overlap},
		// RCM-2 复核 F-4 (2026-07-08): the family's seat on the channel's
		// background comprehensive board (registered key, DCS §23.1 lane) —
		// the display 行2 背景榜位#N consumer. 0 (on-chain families) drops the
		// note (typed-count discipline; absence never guesses a seat).
		{types.TraceNoteKeyBackgroundRank, traceQueryTypedCount(backgroundRank)},
		{types.TraceNoteKeyWindow, traceQueryWindowValue(fam.StartTs, fam.EndTs)},
		// DCS E5 producer half — unchanged at family grain: the source-window
		// identity of the stats run every member came from.
		{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(stats.Window)},
		// RCM family accounting (isolated member_* lane; see the rank-notes
		// emission for the caliber/roster wire rules).
		{types.TraceNoteKeyMemberCount, traceQueryTypedCount(len(fam.Members))},
		{types.TraceNoteKeyMemberMaxMS, traceQueryObservationMSValue(fam.MaxMs)},
		{types.TraceNoteKeyMemberMinMS, traceQueryObservationMSValue(fam.MinMs)},
		{types.TraceNoteKeyMemberSumMS, memberSum},
		{types.TraceNoteKeyMemberFoldCaliber, fam.FoldCaliber},
		{types.TraceNoteKeyMemberRoster, strings.Join(fam.MemberRosterEntries(), " | ")},
		// DCS E4 dual basis at family grain (absent when nothing clipped).
		{types.TraceNoteKeyActualImpactMS, traceQueryObservationMSValue(fam.ActualTotalMs)},
		{types.TraceNoteKeyActualWindow, traceQueryWindowValue(fam.ActualStartTs, fam.ActualEndTs)},
	})
	var summary string
	if fam.OnChain {
		summary = fmt.Sprintf("semantic trace span family class=%s x%d complete selected-window union=%.3fms (largest %q %.3fms); exact on-chain intersection participation=%.3fms",
			fam.SemanticClass, len(fam.Members), fam.TotalMs, rep.Name, fam.MaxMs, fam.ProjectedImpactMs)
	} else {
		summary = fmt.Sprintf("semantic trace span family class=%s x%d same-thread span(s) totalled %.3fms (largest %q %.3fms)", fam.SemanticClass, len(fam.Members), fam.TotalMs, rep.Name, fam.MaxMs)
	}
	if relevance != "" {
		summary += "; chain_relevance=" + relevance
	}
	summary += "; fold_caliber=" + fam.FoldCaliber
	if fam.TotalMs < fam.SumMs {
		summary += fmt.Sprintf("; interval_union=%.3fms < member_sum=%.3fms (overlapping same-thread segments deduplicated)", fam.TotalMs, fam.SumMs)
	}
	var supportRefs []string
	for i, member := range fam.Members {
		if i >= 8 {
			break
		}
		supportRefs = append(supportRefs, traceQueryObservationSupportRefs(ref, member.StartLine, member.EndLine)...)
	}
	return types.ObservationRecord{
		ID:              fmt.Sprintf("trace_query:%s#trace_semantic_span:%d", scope, ordinal),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
		SourceRef:       ref,
		Span: types.ObservationSpan{
			LineStart: fam.StartLine,
			LineEnd:   fam.EndLine,
			StartTs:   fam.StartTs,
			EndTs:     fam.EndTs,
		},
		ClaimKey:    "trace_semantic_span:" + fam.SemanticClass,
		Subject:     traceThreadLabel(fam.Thread),
		Predicate:   "trace_semantic_span",
		Object:      fam.SemanticClass,
		Value:       traceQueryObservationMSValue(fam.TotalMs),
		Unit:        "ms",
		Summary:     summary,
		RichNotes:   notes,
		SupportRefs: supportRefs,
		ObservedAt:  at,
		Confidence:  traceQuerySemanticTraceSpanConfidence(relevance),
	}
}

type traceQuerySemanticSpanContext struct {
	chainRelevance string
	causality      string
	chainDepth     int
	overlapMs      float64
}

// traceQuerySemanticSpanFamilyFoldContext derives the single-member semantic
// observation context from the SAME family fold the rank lane consumes (审计
// 复核 R1, §29.25 处置委托 + §29.26 待主会话落账, 2026-07-10):
// fam.OnChain / fam.ProjectedImpactMs / fam.ChainDepth come from
// FoldSemanticSpanFamilies → semanticTraceSpanChainIntersectionProjection —
// the exact primitive the single-span rank mint prices participation with —
// so the overlap note mirrors the rank row's published EffectiveImpactMs by
// construction (one 道别 predicate, one value source, two consumers; the
// divergent best-single-window traceQuerySemanticTraceSpanContext lane is
// retired from this producer). The off-chain tail mirrors the family record's
// derivation verbatim: envelope-vs-chain-window overlap decides
// adjacent/background; no chain → no lane claim.
func traceQuerySemanticSpanFamilyFoldContext(fam tracequery.SemanticSpanFamily, span tracequery.TraceSpanSummary, chain *tracequery.ChainResult) traceQuerySemanticSpanContext {
	if fam.OnChain {
		return traceQuerySemanticSpanContext{
			chainRelevance: "on_chain",
			causality:      "on_wakeup_chain",
			chainDepth:     fam.ChainDepth,
			overlapMs:      fam.ProjectedImpactMs,
		}
	}
	if chain == nil || (len(chain.Nodes) == 0 && len(chain.CausalImpacts) == 0 && len(chain.Edges) == 0) {
		return traceQuerySemanticSpanContext{}
	}
	if traceQueryWindowOverlapMS(span.StartTs, span.EndTs, chain.Window.StartTs, chain.Window.EndTs) > 0 {
		return traceQuerySemanticSpanContext{chainRelevance: "adjacent", causality: "adjacent_to_wakeup_chain"}
	}
	return traceQuerySemanticSpanContext{chainRelevance: "background", causality: "background"}
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

func traceQueryTypedThreadDurationObservations(items []tracequery.ThreadDuration, window tracequery.TimeWindow, ref types.ObservationSourceRef, scope, at, family, predicate, state, label string, confidence float64) []types.ObservationRecord {
	var out []types.ObservationRecord
	for i, td := range items {
		if i >= traceQueryWidthTypedFamilyRowCap() {
			break
		}
		thread := traceThreadLabel(td.Thread)
		if strings.TrimSpace(thread) == "" || td.DurationMs <= 0 {
			continue
		}
		notes := traceQueryTypedKVNotes([][2]string{
			{"state", state},
			{"duration", traceQueryObservationMSValue(td.DurationMs)},
			{types.TraceNoteKeyWindow, traceQueryWindowValue(td.StartTs, td.EndTs)},
			{"cpu", traceKnownCPU(td.CPU >= 0, td.CPU)},
			{"core_class", td.CoreClass},
			{"freq", traceQueryTypedCount(td.Frequency)},
			{"priority", traceQueryPriorityPair(td.Priority, td.PriorityClass)},
			// F-2 (统一复核 2026-07-04, NEW-8 pattern): the row's own `window`
			// above is the thread-state segment; the selected QUERY window
			// travels via the same typed note as every other selected-window
			// family. RN-12 collection refuses totals without it (禁猜), and
			// these predicates stay outside the CMP-2 anchor whitelist —
			// display/cross-reference carrier only, never an anchor.
			{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(window)},
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
			break
		}
		if strings.TrimSpace(item.Path) == "" && strings.TrimSpace(item.Operation) == "" {
			continue
		}
		notes := traceQueryTypedKVNotes([][2]string{
			{"op", item.Operation},
			{types.TraceNoteKeyPath, item.Path},
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
		if i >= traceQueryWidthTypedFamilyRowCap() {
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

func traceQueryTypedTopIOInodeSummary(item tracequery.TopIOInodeSummary) string {
	parts := []string{"top_io_inode"}
	for _, kv := range [][2]string{
		{"inode", item.Inode},
		{"dev", item.Dev},
		{"name", item.EntryName},
		{"count", traceQueryTypedCount(item.Count)},
		{"reads", traceQueryTypedCount(item.ReadCount)},
		{"writes", traceQueryTypedCount(item.WriteCount)},
		{"completions", traceQueryTypedCount(item.CompletionCount)},
		{"bytes", traceQueryTypedInt64(item.Bytes)},
		{"adds", traceQueryTypedCount(item.PageCacheAdds)},
		{"deletes", traceQueryTypedCount(item.PageCacheDeletes)},
		{"max_latency", traceQueryObservationMSValue(item.MaxLatencyMs)},
		{"threads", traceQueryTypedCount(item.ThreadCount)},
		{"top_threads", traceTopIOInodeThreadRoster(item.TopThreadLatencies)},
	} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+"="+sanitizeForBanner(kv[1]))
		}
	}
	return strings.Join(parts, " ")
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
		{"ambiguous_cohorts", traceQueryTypedCount(item.AmbiguousCohortCount)},
		{"pairing_suppressed", traceQueryTypedCount(item.PairingSuppressedCount)},
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
		{types.TraceNoteKeyDState, traceQueryObservationMSValue(item.DStateMs)},
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
		{types.TraceNoteKeyRunning, traceQueryObservationMSValue(item.RunningMs)},
		{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"high_prio_running", traceQueryObservationMSValue(item.HighPriorityRunningMs)},
		{"system_or_kernel_running", traceQueryObservationMSValue(item.SystemOrKernelRunningMs)},
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
		{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(item.RunnableWaitMs)},
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
		{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"cpu", strconv.Itoa(item.CPU)},
		{"core_class", item.CoreClass},
		{"freq", traceQueryTypedCount(item.Frequency)},
		{"same_cpu_busy", traceQueryObservationMSValue(item.SameCPUBusyMs)},
		{"same_cpu_idle", traceQueryObservationMSValue(item.SameCPUIdleMs)},
		{"other_cpu_idle", traceQueryObservationMSValue(item.OtherCPUIdleMs)},
		{"high_prio_running", traceQueryObservationMSValue(item.HighPriorityRunningMs)},
		{"high_prio_overlap", traceQueryObservationMSValue(item.HighPriorityRunningOverlapMs)},
		{"system_or_kernel_running", traceQueryObservationMSValue(item.SystemOrKernelRunningMs)},
		{"system_or_kernel_overlap", traceQueryObservationMSValue(item.SystemOrKernelRunningOverlapMs)},
		{"system_or_kernel_competitors", traceQueryTypedCount(item.SystemOrKernelCompetitorCount)},
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
		{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"cpu", strconv.Itoa(item.CPU)},
		{"core_class", item.CoreClass},
		{"freq", traceQueryTypedCount(item.Frequency)},
		{"priority", traceQueryPriorityPair(item.Priority, item.PriorityClass)},
		{"same_cpu_busy", traceQueryObservationMSValue(item.SameCPUBusyMs)},
		{"same_cpu_idle", traceQueryObservationMSValue(item.SameCPUIdleMs)},
		{"other_cpu_idle", traceQueryObservationMSValue(item.OtherCPUIdleMs)},
		{"high_prio_running", traceQueryObservationMSValue(item.HighPriorityRunningMs)},
		{"high_prio_overlap", traceQueryObservationMSValue(item.HighPriorityRunningOverlapMs)},
		{"system_or_kernel_running", traceQueryObservationMSValue(item.SystemOrKernelRunningMs)},
		{"system_or_kernel_overlap", traceQueryObservationMSValue(item.SystemOrKernelRunningOverlapMs)},
		{"system_or_kernel_competitors", traceQueryTypedCount(item.SystemOrKernelCompetitorCount)},
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
		{types.TraceNoteKeyRunning, traceQueryObservationMSValue(item.RunningMs)},
		{types.TraceNoteKeyRunnable, traceQueryObservationMSValue(item.RunnableWaitMs)},
		{"high_prio_running", traceQueryObservationMSValue(item.HighPriorityRunningMs)},
		{"system_or_kernel_running", traceQueryObservationMSValue(item.SystemOrKernelRunningMs)},
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
		{types.TraceNoteKeyPath, item.Path},
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
			if ordinal >= traceQueryWidthTypedFamilyRowCap() {
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

// traceQueryMaxInheritedPID is the shared Linux PID_MAX_LIMIT sanity cap
// (types.RuntimeTargetMaxPID) — one home for the emit-analysis normalizer, the
// inheritance lane here, and the B1 anchor-election ledger carrier (F4
// 教义统一).
const traceQueryMaxInheritedPID = types.RuntimeTargetMaxPID

func traceQueryApplyRequestModelTarget(ctx *types.BusContext, p traceQueryParams) (traceQueryParams, string) {
	if p.PID.Int() > 0 || strings.TrimSpace(p.Thread) != "" {
		return p, ""
	}
	if globalTypes := traceQueryCPUGlobalEventSearchTypes(p); len(globalTypes) > 0 {
		// CPU frequency/idle/control rows describe a CPU-global state lane. The
		// ftrace row header names whichever task happened to emit the change;
		// inheriting the analysis target would silently turn that incidental
		// emitter into an ownership filter and can manufacture a zero result.
		// Keep the query unscoped and disclose the skipped inheritance. Explicit
		// selectors are rejected earlier in Execute so the two authorities never
		// disagree.
		return p, fmt.Sprintf("trace_query_target_inheritance_skipped=cpu_global_event_search event_types=[%s]; pid/thread would filter emitter identity rather than CPU-state ownership", strings.Join(globalTypes, ","))
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

// traceQueryCPUGlobalEventSearchTypes returns the exact CPU-state/control
// families whose event rows have no thread ownership semantics. It deliberately
// applies only to event_search: target-scoped window_stats/root-cause views use
// PID to select the analyzed thread while consuming these same events as global
// context, which is correct and must remain allowed.
func traceQueryCPUGlobalEventSearchTypes(p traceQueryParams) []string {
	if tracequery.CanonicalViewName(p.View) != tracequery.FallbackViewEventSearch {
		return nil
	}
	seen := map[string]bool{}
	for _, eventType := range parseTraceQueryEventTypes(p.EventTypes.Strings()) {
		switch eventType {
		case tracequery.EventCPUFrequency, tracequery.EventCPUFrequencyLimit, tracequery.EventCPUIdle, tracequery.EventClockSetRate:
			seen[string(eventType)] = true
		}
	}
	out := make([]string, 0, len(seen))
	for eventType := range seen {
		out = append(out, eventType)
	}
	sort.Strings(out)
	return out
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
		Source:     traceQueryExplicitToolCallTargetSource,
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

// traceQueryExplicitToolCallTargetSource marks RuntimeTargets recorded from
// explicit trace_query tool-call pid/thread parameters — the model's own
// exploration cursor, as opposed to analyzer-pinned user-focus targets. Alias
// of the promoted types-layer constant (types.RuntimeTargetSourceExplicitToolCall)
// so the ledger B1 anchor election and this recovery lane share ONE exclusion
// key.
const traceQueryExplicitToolCallTargetSource = types.RuntimeTargetSourceExplicitToolCall

// analyzerPinnedFocusThreadPID returns the analyzer-pinned user-focus thread
// pid from the typed RuntimeTargets lane (the H4 BusContext channel; RN-14b
// §7.9). Explicit trace_query tool-call targets are excluded — they track
// where the model is currently looking, not what the user asked about. The
// focus must be unambiguous: exactly one distinct pinned pid, or, when
// several pinned targets exist, exactly one with the typed source
// "user_explicit". Anything else returns false — this feeds soft recovery
// hints only, and a guessed focus would be worse than none.
func analyzerPinnedFocusThreadPID(ctx *types.BusContext) (int, bool) {
	if ctx == nil {
		return 0, false
	}
	var pinned []types.RuntimeTarget
	collect := func(rm *types.RequestModel) {
		if rm == nil {
			return
		}
		for _, target := range rm.RuntimeTargets {
			if target.PID <= 0 || target.PID > traceQueryMaxInheritedPID {
				continue
			}
			if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
				continue
			}
			pinned = append(pinned, target)
		}
	}
	if ctx.AnalysisIR != nil {
		collect(&ctx.AnalysisIR.RequestModel)
	}
	if ctx.Mutable != nil {
		collect(ctx.Mutable.RequestModel())
	}
	distinct := map[int]bool{}
	userExplicit := map[int]bool{}
	for _, target := range pinned {
		distinct[target.PID] = true
		if strings.TrimSpace(target.Source) == "user_explicit" {
			userExplicit[target.PID] = true
		}
	}
	if len(distinct) == 1 {
		for pid := range distinct {
			return pid, true
		}
	}
	if len(userExplicit) == 1 {
		for pid := range userExplicit {
			return pid, true
		}
	}
	return 0, false
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
