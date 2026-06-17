package skill

import "strings"

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
			When:   "structured row lookup of exact frame/jank ids, span or marker labels, B/E/C/S/F trace_mark rows, inode tokens, entry_name values, perf sample symbols/DSOs/callchains, timestamps, or event labels",
		},
		{
			View:   "span_window",
			Params: "`span_name`",
			When:   "turning a named trace span into a selected time window when the user names a span instead of exact timestamps; B/E ends are unnamed E|pid or E on the same ftrace thread stack, and S/F async spans pair by marker pid+name+cookie",
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
			When: "cross-thread frame flow edges linking one frame's UI/RS/GPU segments with per-hop latency",
		},
		{
			View: "thread_timeline",
			When: "one thread's running/runnable/sleep intervals",
		},
		{
			View: "window_stats",
			When: "same-window CPU/IO/binder/IRQ/frequency, compute-supply, `state_churn` context, perf_samples top symbols/DSOs/callchains, file_io_by_inode, page_cache_by_inode, storage_latency_by_layer, and io_pressure_summary context",
		},
		{
			View:   "perf_stats",
			Params: "`event_types=[\"perf_sample\"]` only when filtering sample rows explicitly",
			When:   "same-window CPU sample aggregation by top_symbols, top_dso, top_callchains, top_threads, and top_events",
		},
		{
			View: "perf_timeline",
			When: "bucketed CPU sample period over time for a selected window/thread/process/symbol context",
		},
		{
			View: "trace_perf_bundle",
			When: "handoff-safe joint trace+perf context that keeps window stats, wakeup/root-cause evidence, root-cause perf_context, and perf sample hotspots together",
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
			When: "deterministic primary/secondary/tertiary cause candidates ordered by chain relevance and same-chain cumulative_impact_ms, including dominant_state state totals, candidate-level perf_context, occurrence_windows for aggregate common dependency paths, co-primary on-chain runnable/running/compute-supply/D-state/IO dependency causes, fragmented state-churn causes, aggregated wakeup-chain causes, and inode-level IO causes",
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
			Params: "`recipe_name=auto|sleep_root_cause|jank|runnable_delay|binder_wait|io_wait|cpu_supply`",
			When:   "a standard evidence pack that adapts the included views to the question shape",
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
