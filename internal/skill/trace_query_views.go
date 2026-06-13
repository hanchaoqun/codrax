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
			When:   "structured row lookup of exact frame/jank ids, span or marker labels, inode tokens, entry_name values, timestamps, or event labels",
		},
		{
			View:   "span_window",
			Params: "`span_name`",
			When:   "turning a named B/E trace span into a selected time window when the user names a span instead of exact timestamps",
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
			When: "same-window CPU/IO/binder/IRQ/frequency, compute-supply, `state_churn` context, file_io_by_inode, page_cache_by_inode, storage_latency_by_layer, and io_pressure_summary context",
		},
		{
			View: "scheduler_latency_stats",
			When: "runnable wait p95/p99/max and same-CPU competition",
		},
		{
			View: "ipc_graph",
			When: "binder transaction send/receive causality",
		},
		{
			View: "wakeup_chain",
			When: "recursive sleep/wakeup source chains",
		},
		{
			View: "root_cause_rank",
			When: "deterministic primary/secondary/tertiary cause candidates, including fragmented state-churn causes and inode-level IO causes",
		},
		{
			View: "frame_root_cause_bundle",
			When: "handoff-safe frame/jank root-cause bundles that combine wakeup chain, frame timeline, ranked causes, critical blocking, IO, IRQ, workqueue, supply pressure, and trace-marker evidence",
		},
		{
			View: "critical_blocking_calls",
			When: "futex/lock/sync/binder/IO/D-state blocking candidates",
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
