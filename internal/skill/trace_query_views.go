package skill

import "strings"

// Occupancy is a different ruler from full request latency or issue counts.
const TraceIOInFlightTeaching = "For simultaneous IO requests use view=\"window_stats\", section io_inflight (not a new view). peak_requests/mean_requests measure half-open complete-pair occupancy, grouped by physical source/layer/endpoint family/device/operation across all issuers; query_pid is not a filter. busy_ms is interval-union time; request_ms is request·ms area; mean divides by the full selected window including idle time. issue_count counts admitted starts, not concurrency. Line bounds retain their existing priority over time bounds: a line-selected cohort has no inferred time denominator, so occupancy values are unavailable; query a time window without line bounds for those values. Read pairing coverage and omitted groups/segments; absent values mean unavailable, not zero. Never extend unpaired requests to the window end, add different IO layers, or equate occupancy with device queue depth, target waiting or a root cause. Use reader language such as 同时在途请求数、平均在途数、忙碌时长 with the measured group/window."

// One explanation at every view-selection site; no duplicate request fields
// or output-prose validation contract is introduced for this observation.
const TraceIORequestLatencyDistributionTeaching = "For IO percentiles call trace_query with view=\"window_stats\" and read window_stats.storage_latency_by_layer[].request_latency_distribution; storage_latency_by_layer is an output section, not a view. Its sample_count, min_ms, mean_ms, max_ms, p50_ms, p90_ms, p95_ms, p99_ms cover all admitted pairs in that row's group, not Top-N requests. Block rows group all issuers by source/family/device/operation; their representative thread is not a population filter. Generic storage rows also retain inode/PID identity. Respect capture/index coverage; admitted pairs do not prove complete capture. Linear interpolation uses complete request durations intersecting the query (line bounds take precedence), including carry-in/out, without clipping. Absent means no qualified samples; measured zero is valid. storage_latency_overflow_groups/storage_latency_overflow_paired_count disclose omitted groups/pairs. Never average group percentiles into a whole-layer percentile or add RQ/BIO/filesystem measurements of the same work. Request duration is not target blocking time or causal proof; response impact still requires wakeup-chain and blocked-interval evidence. Use business wording (IO请求耗时、样本数、中位数、P99耗时) with group/window scope in answers, not internal policy enums."

// TraceQueryViewTeaching is one row of the shared trace_query view-teaching
// table: which deterministic view to pick, when it is the right lens, and the
// key parameters that make the call bounded. This table is the single source
// of truth for every prompt site that enumerates trace_query views; the
// render helpers below keep the wording identical across sites so per-site
// hand-edits cannot drift the teaching apart again.
type TraceQueryViewTeaching struct {
	// View is the trace_query view value exactly as the tool schema enum
	// spells it.
	View string
	// Params is an optional short key-parameter clause rendered as
	// "with <Params>". Empty when the defaults are usually right.
	Params string
	// When is the one-clause when-to-use guidance rendered as "for <When>".
	When string
}

// TraceQueryViewTeachings returns the full view table in tool-schema enum
// order. Every view the tool accepts has exactly one row here.
func TraceQueryViewTeachings() []TraceQueryViewTeaching {
	return []TraceQueryViewTeaching{
		{
			View:   "event_search",
			Params: "`pattern` as a literal substring (not a regex)",
			When:   "structured row lookup of exact frame/jank ids, span or marker labels, B/E/C/S/F trace_mark rows, inode tokens, entry_name values, perf sample symbols/DSOs/callchains, timestamps, or event labels. " + TraceResourceObservationContract,
		},
		{
			View:   "window_sweep",
			Params: "the same `time_start`/`time_end` as the dense request window, optional `bucket_ms` (default 100ms, clamped 50..500) and `pid`",
			When:   "a streaming per-bucket coverage scan of a second-scale or longer dense window (not subject to the index event budget) that ranks advisory top-K dense sub-windows by sched_switch density or target-pid participation and returns a compact coverage table, so heavy-view drill-down windows are picked from measured density instead of blind window bisection",
		},
		{
			View:   "span_window",
			Params: "`span_name`",
			When:   "turning a named trace span into a selected time window when the user names a span instead of exact timestamps; when the span's exact label or window is unknown, first locate it with a bare `event_search` pattern (no event_types), and fall back to that bare pattern search when span markers use a nonstandard form; B/E ends are unnamed E|pid or E on the same ftrace thread stack, and S/F async spans pair by marker pid+name+cookie",
		},
		{
			View: "frame_window",
			When: "locating one Choreographer/RenderFrame/VSYNC/draw/present frame envelope",
		},
		{
			View: "render_pipeline",
			When: "the UI/render-service/GPU draw/present pipeline spans inside a selected frame window",
		},
		{
			View: "frame_timeline",
			When: "per-frame Expected/Actual timeline summaries with jank and GPU/RS/UI phase attribution",
		},
		{
			View: "frame_flow",
			When: TraceFrameFlowEvidenceTeaching,
		},
		{
			View: "thread_timeline",
			When: "one thread's running/runnable/sleep intervals",
		},
		{
			View: "window_stats",
			When: "same-window CPU/IO/binder/IRQ/frequency, compute-supply, `state_churn` context, perf_samples top symbols/DSOs/callchains, file_io_by_inode, page_cache_by_inode, storage_latency_by_layer, io_pressure_summary context, and ranked/enumeration IO questions (which inodes/files see the most frequent IO) via the `top_io_inodes` section — whole-window per-(dev,inode) totals ordered by event count with the total group count disclosed; its latency figures are the largest single event plus per-thread totals, never cross-thread latency sums. " + TraceIORequestLatencyDistributionTeaching + " " + TraceIOInFlightTeaching,
		},
		{
			View:   "perf_stats",
			Params: "`event_types=[\"perf_sample\"]` only when filtering sample rows explicitly",
			When:   "same-window CPU sample aggregation by top_symbols, top_dso, top_callchains, top_threads, top_events, source, and symbolization_status",
		},
		{
			View: "perf_timeline",
			When: "bucketed CPU sample weight over time for a selected window/thread/process/symbol context",
		},
		{
			View: "trace_perf_bundle",
			When: "handoff-safe joint trace+perf context that keeps window stats, wakeup/root-cause evidence, root-cause perf_context/perf_contexts role contexts, and perf sample hotspots together",
		},
		{
			View: "scheduler_latency_stats",
			When: "runnable wait p95/p99/max and same-CPU competition",
		},
		{
			View: "ipc_graph",
			When: "binder transaction send/receive causality with explicit oneway/sync_like/blocking_candidate fields",
		},
		{
			View: "wakeup_chain",
			When: "recursive sleep/wakeup source chains with causal impacts and aggregated common fragmented dependency paths plus bounded occurrence_windows",
		},
		{
			View: "root_cause_rank",
			When: "deterministic primary/secondary/tertiary cause candidates with dominant_state state totals, candidate-level perf_context plus role-aware perf_contexts for running/CPU-pressure/compute-supply code-execution support, occurrence_windows for aggregate common dependency paths, closed-matrix state_churn candidates, positive-effective on-chain semantic span-work candidates, aggregated wakeup-chain causes, and inode-level IO causes. " + TraceRootCauseRankOrderTeaching + " Preserve cumulative_impact_ms, running/runnable/sleep/d_state/io_wait totals, and significant unpriced business/semantic work as a separate raw occupancy dimension and follow-up leads",
		},
		{
			View: "frame_root_cause_bundle",
			When: "handoff-safe frame/jank root-cause bundles that combine wakeup chain, frame timeline, ranked causes, critical blocking, IO, IRQ, workqueue, supply pressure, trace-marker evidence, and role-specific perf contexts target_running_perf/on_chain_perf/binder_peer_perf/same_cpu_competitor_perf",
		},
		{
			View: "critical_blocking_calls",
			When: "futex/lock/sync/binder/IO/D-state blocking candidates with oneway/sync_like/blocking_candidate semantics and peer_state breakdown when the peer thread timeline is visible",
		},
		{
			View: "interaction_stats",
			When: "target-thread wakeup/binder interaction Top-N",
		},
		{
			View:   "recipe",
			Params: "`recipe_name=auto|sleep_root_cause|jank|runnable_delay|binder_wait|io_wait|cpu_supply|span_locate`",
			When:   "a standard evidence pack that adapts the included views to the question shape; recipe_name=span_locate resolves a span label to its start/end window first (bare-pattern locate, no event_types)",
		},
		{
			View: "evidence_pack",
			When: "one fixed line-backed bundle (wakeup chain, window stats, IPC graph, scheduler latency, blocking calls) over an already-selected window — unlike `recipe`, the included views do not adapt",
		},
	}
}

// RenderTraceQueryViewMatrix renders the full table as a single prose run
// ("`view=\"x\"` for ..., ..., and `view=\"z\"` for ...") suitable for
// inlining after a site-local lead-in such as "use".
func RenderTraceQueryViewMatrix() string {
	rows := TraceQueryViewTeachings()
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			if i == len(rows)-1 {
				b.WriteString(", and ")
			} else {
				b.WriteString(", ")
			}
		}
		b.WriteString("`view=\"")
		b.WriteString(row.View)
		b.WriteString("\"`")
		if row.Params != "" {
			b.WriteString(" with ")
			b.WriteString(row.Params)
		}
		b.WriteString(" for ")
		b.WriteString(row.When)
	}
	return b.String()
}

// RenderTraceQueryViewNameList renders the compact backticked view-name list
// for space-constrained hints.
func RenderTraceQueryViewNameList() string {
	rows := TraceQueryViewTeachings()
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, "`"+row.View+"`")
	}
	return strings.Join(names, ", ")
}
