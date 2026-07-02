package tracequery

import "strings"

// This file is the single source of truth for per-view trace_query result-row
// budgets (E4). Every numeric value here is a literal consolidation of caps
// that already shipped — changing any of them changes model-visible result
// width, which historically (3bde10fa) pushed the model to abandon trace_query
// for grep, so TestViewCapacityTablePinsCurrentBehavior pins them and any
// future change must be a deliberate test edit. The per-call index budgets
// (defaultTraceIndexMaxEvents 250K, tool-side 512MiB scoped byte budget) are
// deliberately NOT in this table: they belong to the C3/Gap3 index ladder and
// bound a different stage (parse-time index growth, not result rows).

// Canonical fallback tokens for the C3 recovery surfaces. Both the engine's
// IndexEventLimitError.RecoveryParams and the tool-side guard/index-limit
// refinements interpolate these, so the two surfaces cannot drift.
const (
	// FallbackViewEventSearch is the C3 streaming escape hatch: event_search
	// scans without materializing the in-memory event index, so every budget
	// or scope failure can steer to it. It must never itself be told to
	// switch views on over-capacity (133520c1) — its over-cap remedy is a
	// narrower window or a concrete limit, encoded by its empty FallbackView.
	FallbackViewEventSearch = "event_search"
	// FallbackParentCoverageStateCluster is the C3 dense-window parent
	// coverage mode published as parent_coverage by the index-limit
	// refinement (StreamStateCluster, not a query view).
	FallbackParentCoverageStateCluster = "stream_state_cluster"
)

// StreamStateClusterDefaultMax bounds the per-state Top-N rows of the C3
// StreamStateCluster fallback (tool call site and the function's own floor).
const StreamStateClusterDefaultMax = 8

// sharedDefaultResultLimit is normalizeQuery's shared Limit default. It is
// view-independent on purpose: per-view hard clamps (MaxLimit below) apply on
// top of it, so e.g. scheduler_latency_stats' effective default stays
// min(40,20)=20. Introducing per-view defaults here would silently change
// those effective values.
const sharedDefaultResultLimit = 40

// spanWindowFloorLimit is FindSpanWindows' floor when a caller passes max<=0
// directly (bypassing normalizeQuery's shared default).
const spanWindowFloorLimit = 8

// Wakeup-chain recursion caps. MaxBranches=8 truncation was ruled caveat-only
// (methodology audit O10): raising it is deferred pending real cases, so the
// table records it but nothing may widen it.
const (
	wakeupChainDefaultMaxDepth    = 10
	wakeupChainDefaultMaxBranches = 8
)

// Typed compaction dimensions. One label per truncated row family so the
// refinement layer can name what was cut without parsing caveat prose.
const (
	CompactionDimensionCandidates = "candidates"
	CompactionDimensionEdges      = "edges"
	CompactionDimensionSpans      = "spans"
	CompactionDimensionIntervals  = "intervals"
	CompactionDimensionEvents     = "events"
	CompactionDimensionPeers      = "peers"
	CompactionDimensionPhaseSpans = "phase_spans"
)

// ViewCapacity is one row of the per-view capacity table.
type ViewCapacity struct {
	View string
	// DefaultLimit is the effective row cap when the caller passes limit<=0
	// (normalizeQuery's shared default, then MaxLimit clamps on top).
	DefaultLimit int
	// MaxLimit is the hard upper clamp on the emitted row count; 0 means the
	// view honors the requested limit without an upper clamp.
	MaxLimit int
	// FloorLimit is the fallback cap for call sites that pass max<=0 without
	// going through normalizeQuery (span_window's FindSpanWindows floor).
	FloorLimit int
	// Dimension names the row family the limit truncates.
	Dimension string
	// HeavyView marks views that expand scheduler/resource aggregates and
	// need a bounded time/line/span/pattern scope on large traces.
	HeavyView bool
	// RelationScoped marks views whose event consumption is a provable
	// subset of the target pid's threads, their transitive scheduler wakers,
	// and binder rows — the only views for which relation-scope index
	// pruning is complete (verified design w9ffnwv29).
	RelationScoped bool
	// Composite marks bundle views that assemble multiple sub-views; their
	// row budgets live on the sub-view entries.
	Composite bool
	// FallbackView is the behaviorally-established recovery view on
	// over-capacity / unbounded-scope failures. Empty for event_search: it
	// IS the C3 escape hatch and must never be told to leave itself.
	FallbackView string
	// FallbackEventTypes are the discovery filters that accompany
	// FallbackView (the heavy-view guard's event_search+trace_mark shape).
	FallbackEventTypes []string
	// MaxDepth / MaxBranches are the wakeup_chain recursion caps. Recorded
	// only; MaxBranches truncation is caveat-only per the O10 ruling.
	MaxDepth    int
	MaxBranches int
	// Advisory window_stats sub-caps. These document literals that stay at
	// their call sites (they are not consumed by any clamp path here); the
	// generic/semantic trace-mark caps are DELIBERATELY two entries — v7 O1
	// reserved semantic-class spans their own slots so a short JIT/verify
	// span is not evicted by longer generic spans, and collapsing them into
	// one number would revert that.
	AdvisoryBackgroundThreads      int
	AdvisoryBackgroundProcesses    int
	AdvisoryGenericTraceMarkSpans  int
	AdvisorySemanticTraceMarkSpans int
}

// ViewCompaction is the typed record of one result truncation, published on
// Result.Compactions alongside the existing prose caveat (caveats stay
// verbatim — tests and §7.11 consumers pin them). Total==0 means the producer
// stopped scanning at the cap and the true total is unknown (indexed
// event_search). LastEmittedTs/LastEmittedLine come from the final emitted
// row and are what make concrete window-split suggestions possible.
type ViewCompaction struct {
	View            string  `json:"view"`
	Dimension       string  `json:"dimension,omitempty"`
	Total           int     `json:"total,omitempty"`
	Emitted         int     `json:"emitted"`
	LastEmittedTs   float64 `json:"last_emitted_ts,omitempty"`
	LastEmittedLine int     `json:"last_emitted_line,omitempty"`
}

var viewCapacityTable = map[string]ViewCapacity{
	"event_search": {
		View:         "event_search",
		DefaultLimit: sharedDefaultResultLimit,
		Dimension:    CompactionDimensionEvents,
	},
	"span_window": {
		View:               "span_window",
		DefaultLimit:       sharedDefaultResultLimit,
		FloorLimit:         spanWindowFloorLimit,
		Dimension:          CompactionDimensionSpans,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"thread_timeline": {
		View:               "thread_timeline",
		HeavyView:          true,
		RelationScoped:     true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"window_stats": {
		View:                           "window_stats",
		HeavyView:                      true,
		FallbackView:                   FallbackViewEventSearch,
		FallbackEventTypes:             []string{string(EventTraceMark)},
		AdvisoryBackgroundThreads:      4,
		AdvisoryBackgroundProcesses:    8,
		AdvisoryGenericTraceMarkSpans:  8,
		AdvisorySemanticTraceMarkSpans: traceMarkSemanticSpanCap,
	},
	"perf_stats": {
		View:               "perf_stats",
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"perf_timeline": {
		View:               "perf_timeline",
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"trace_perf_bundle": {
		View:               "trace_perf_bundle",
		HeavyView:          true,
		Composite:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"scheduler_latency_stats": {
		View:               "scheduler_latency_stats",
		DefaultLimit:       20,
		MaxLimit:           20,
		Dimension:          CompactionDimensionIntervals,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"ipc_graph": {
		View:               "ipc_graph",
		DefaultLimit:       sharedDefaultResultLimit,
		Dimension:          CompactionDimensionEdges,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"wakeup_chain": {
		View:               "wakeup_chain",
		HeavyView:          true,
		RelationScoped:     true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
		MaxDepth:           wakeupChainDefaultMaxDepth,
		MaxBranches:        wakeupChainDefaultMaxBranches,
	},
	"root_cause_rank": {
		View:               "root_cause_rank",
		DefaultLimit:       12,
		MaxLimit:           12,
		Dimension:          CompactionDimensionCandidates,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"frame_root_cause_bundle": {
		View:               "frame_root_cause_bundle",
		HeavyView:          true,
		Composite:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"critical_blocking_calls": {
		View:               "critical_blocking_calls",
		DefaultLimit:       20,
		MaxLimit:           20,
		Dimension:          CompactionDimensionCandidates,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"interaction_stats": {
		View:               "interaction_stats",
		DefaultLimit:       20,
		MaxLimit:           20,
		Dimension:          CompactionDimensionPeers,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"frame_window": {
		View:               "frame_window",
		DefaultLimit:       20,
		MaxLimit:           20,
		Dimension:          CompactionDimensionPhaseSpans,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"render_pipeline": {
		View:               "render_pipeline",
		DefaultLimit:       20,
		MaxLimit:           20,
		Dimension:          CompactionDimensionPhaseSpans,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"frame_timeline": {
		View:               "frame_timeline",
		DefaultLimit:       20,
		MaxLimit:           20,
		Dimension:          CompactionDimensionPhaseSpans,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"frame_flow": {
		View:               "frame_flow",
		DefaultLimit:       20,
		MaxLimit:           20,
		Dimension:          CompactionDimensionPhaseSpans,
		HeavyView:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"recipe": {
		View:               "recipe",
		HeavyView:          true,
		Composite:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
	"evidence_pack": {
		View:               "evidence_pack",
		HeavyView:          true,
		Composite:          true,
		FallbackView:       FallbackViewEventSearch,
		FallbackEventTypes: []string{string(EventTraceMark)},
	},
}

// CanonicalViewName resolves the empty-view default and the caller-facing
// view aliases normalizeQuery accepts, so capacity lookups, normalizeQuery,
// and the tool layer agree on one name.
func CanonicalViewName(view string) string {
	view = strings.TrimSpace(view)
	switch view {
	case "":
		return "event_search"
	case "frame_bundle", "frame_rootcause_bundle", "frame_root_cause":
		return "frame_root_cause_bundle"
	}
	return view
}

// ViewCapacityFor returns the capacity row for a (possibly aliased) view.
// Unknown views get a zero row with only the canonical name set, matching the
// old switch defaults (not heavy, not relation-scoped, no clamp).
func ViewCapacityFor(view string) ViewCapacity {
	capacity, ok := viewCapacityTable[CanonicalViewName(view)]
	if !ok {
		return ViewCapacity{View: CanonicalViewName(view)}
	}
	// Copy the slice so callers converting FallbackEventTypes into their own
	// parameter carriers cannot mutate the table entry.
	capacity.FallbackEventTypes = append([]string(nil), capacity.FallbackEventTypes...)
	return capacity
}

// IsHeavyView reports whether a view expands scheduler/resource aggregates
// and therefore needs a bounded scope on large traces (the former tool-side
// switch list, now table-driven).
func IsHeavyView(view string) bool {
	return ViewCapacityFor(view).HeavyView
}

// RelationScopedView reports whether relation-scope index pruning is complete
// for a view (verified design w9ffnwv29: only thread_timeline/wakeup_chain).
func RelationScopedView(view string) bool {
	return ViewCapacityFor(view).RelationScoped
}

// ClampLimit applies the view's clamp rule to a requested limit,
// byte-identical to the historical per-site literals: for clamped views,
// limit<=0 or limit>MaxLimit collapses to MaxLimit; unclamped views only
// substitute DefaultLimit for limit<=0.
func (c ViewCapacity) ClampLimit(limit int) int {
	if c.MaxLimit > 0 && (limit <= 0 || limit > c.MaxLimit) {
		return c.MaxLimit
	}
	if limit <= 0 {
		return c.DefaultLimit
	}
	return limit
}
