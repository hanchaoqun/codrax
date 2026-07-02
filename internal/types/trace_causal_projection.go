package types

import (
	"sort"
	"strconv"
	"strings"
)

const (
	TraceCausalRolePrimaryRootCause = "primary_root_cause"
	TraceCausalRoleCausalHop        = "causal_hop"
	TraceCausalRoleRootCauseContext = "root_cause_context"
	TraceCausalRoleSemanticSpan     = "semantic_span"
)

const (
	traceCausalProjectionPrimaryLimit       = 10
	traceCausalProjectionOnChainLimit       = 24
	traceCausalProjectionContextBucketLimit = 8
	traceCausalProjectionSemanticSpanLimit  = 16
	traceCausalProjectionSupportingHopLimit = 10
)

type TraceCausalProjection struct {
	PrimaryRootCause  *TraceCausalProjectionNode  `json:"primary_root_cause,omitempty"`
	PrimaryRootCauses []TraceCausalProjectionNode `json:"primary_root_causes,omitempty"`
	OnChainCauses     []TraceCausalProjectionNode `json:"on_chain_causes,omitempty"`
	AdjacentCauses    []TraceCausalProjectionNode `json:"adjacent_causes,omitempty"`
	BackgroundCauses  []TraceCausalProjectionNode `json:"background_causes,omitempty"`
	SemanticSpans     []TraceCausalProjectionNode `json:"semantic_spans,omitempty"`
	WakeupPath        []string                    `json:"wakeup_path,omitempty"`
	SupportingHops    []TraceCausalProjectionNode `json:"supporting_hops,omitempty"`
	// WakeupChainRecommendedNotRun is true when this run's ledger contains a
	// state_drilldown observation whose typed chain_required=true rich note
	// recommended a wakeup-chain drilldown, but NO wakeup_chain-family
	// observation (wakeup_chain / wakeup_chain_edge / wakeup_causal_impact /
	// wakeup_causal_aggregate) was produced — i.e. the recommended drilldown was
	// never executed (§7.30 裁定3). Precise typed signals only: the flag never
	// derives from prose. Renderers use it to distinguish "the drilldown was not
	// run this round" from "the sleep interval had no sched_wakeup record"
	// (missing_wakeup) when the wakeup path is empty.
	WakeupChainRecommendedNotRun bool `json:"wakeup_chain_recommended_not_run,omitempty"`
	// WindowStartTs/WindowEndTs is the user's originally-requested analysis
	// window (seconds), sourced from the same precise frame_target_resolution
	// anchor that feeds WithinRequestedWindow (window_source=query_window or the
	// explicit-union variant). Zero when no such anchor exists — the renderer
	// must then fall back to a relative bar scale and MUST NOT fabricate a
	// window or percentages (presentation v3 §5 fallback rule).
	WindowStartTs float64 `json:"window_start_ts,omitempty"`
	WindowEndTs   float64 `json:"window_end_ts,omitempty"`
}

// WindowDurationMS returns the requested-window length in milliseconds, or 0
// when the window anchor was absent (renderer falls back per v3 §5).
func (p TraceCausalProjection) WindowDurationMS() float64 {
	if p.WindowStartTs <= 0 || p.WindowEndTs <= p.WindowStartTs {
		return 0
	}
	return (p.WindowEndTs - p.WindowStartTs) * 1000
}

func (p TraceCausalProjection) Active() bool {
	return p.PrimaryRootCause != nil ||
		len(p.PrimaryRootCauses) > 0 ||
		len(p.OnChainCauses) > 0 ||
		len(p.AdjacentCauses) > 0 ||
		len(p.BackgroundCauses) > 0 ||
		len(p.SemanticSpans) > 0 ||
		len(p.WakeupPath) > 0 ||
		len(p.SupportingHops) > 0
}

type TraceCausalProjectionNode struct {
	Role               string   `json:"role,omitempty"`
	EvidenceID         string   `json:"evidence_id,omitempty"`
	Subject            string   `json:"subject,omitempty"`
	Predicate          string   `json:"predicate,omitempty"`
	Object             string   `json:"object,omitempty"`
	Value              string   `json:"value,omitempty"`
	Unit               string   `json:"unit,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	SupportRefs        []string `json:"support_refs,omitempty"`
	LineStart          int      `json:"line_start,omitempty"`
	LineEnd            int      `json:"line_end,omitempty"`
	Rank               int      `json:"rank,omitempty"`
	Tier               string   `json:"tier,omitempty"`
	Causality          string   `json:"causality,omitempty"`
	ChainRelevance     string   `json:"chain_relevance,omitempty"`
	ChainDepth         int      `json:"chain_depth,omitempty"`
	ImpactMS           float64  `json:"impact_ms,omitempty"`
	CumulativeImpactMS float64  `json:"cumulative_impact_ms,omitempty"`
	SpanName           string   `json:"span_name,omitempty"`
	SpanKind           string   `json:"span_kind,omitempty"`
	SpanCategory       string   `json:"span_category,omitempty"`
	SpanSubcategory    string   `json:"span_subcategory,omitempty"`
	SemanticClass      string   `json:"semantic_class,omitempty"`
	// StartTs/EndTs is this node's own trace window (seconds), when the source
	// observation exposed one (semantic_span / state_drilldown rows do; plain
	// root_cause primary rows carry only line spans and leave these zero).
	StartTs float64 `json:"start_ts,omitempty"`
	EndTs   float64 `json:"end_ts,omitempty"`
	// WithinRequestedWindow is three-state: nil = unknown (no precise anchor
	// window, or this node has no window of its own), true = this node's
	// window intersects the user's originally-requested analysis window,
	// false = it was drilled into a window outside the user's request. Only
	// populated when a frame_target_resolution anchor with window_source=
	// query_window (or the explicit-union variant) is present, i.e. the R1
	// pinned-thread/window trigger path.
	WithinRequestedWindow *bool   `json:"within_requested_window,omitempty"`
	Confidence            float64 `json:"confidence,omitempty"`
	// StateKind is the node's dominant scheduler state (e.g. s_sleep / running /
	// runnable / d_state / io_wait), sourced verbatim from the trace_query
	// "dominant_state" rich note. It is a precise typed signal (not parsed from
	// prose): a sleep-dominant node is a SYMPTOM that must be drilled down to a
	// non-sleep root, never itself a root cause — the renderer uses this to mark
	// sleep rows and drive the sleep-drilldown surface (presentation gaps d/e).
	// Empty when the source row exposed no dominant_state.
	StateKind string `json:"state_kind,omitempty"`
	// UndrillableReason is a typed enum (currently only "missing_wakeup") set when
	// a sleep interval could NOT be resolved to an upstream waker — sourced from a
	// root_evidence:missing_wakeup observation ("sleep interval has no matching
	// sched_wakeup row in the selected trace window"). It promotes that fact from
	// a trace-level caveat into the projection so the renderer can explicitly show
	// "cannot drill further" instead of silently dropping the chain (gap e).
	UndrillableReason string `json:"undrillable_reason,omitempty"`
	// EffectiveImpactMS / ActualImpactMS are the remaining two members of the
	// duration triad the trace_query rows already carry (alongside ImpactMS =
	// projected and CumulativeImpactMS). Effective is a bounded ranking/hidden-cost
	// signal (defaults to cumulative for non-semantic rows upstream); Actual is the
	// underlying scheduler-state duration that may extend outside the projected
	// window. Sourced from the effective_impact_ms / actual_impact_ms rich notes.
	// Zero when the source row did not expose them (gap c three-column magnitude).
	EffectiveImpactMS float64 `json:"effective_impact_ms,omitempty"`
	ActualImpactMS    float64 `json:"actual_impact_ms,omitempty"`
	// DrilldownTarget is the direct upstream node a sleep symptom should drill
	// into. It is attached only from typed wakeup_chain_edge/path records, and
	// only when the immediate waker for this node is unique. Empty means the
	// renderer must avoid inventing a global target and should point to the
	// wakeup chain instead.
	DrilldownTarget     string `json:"drilldown_target,omitempty"`
	DrilldownEvidenceID string `json:"drilldown_evidence_id,omitempty"`
	DrilldownRelation   string `json:"drilldown_relation,omitempty"`
	// Pre-render deterministic aggregation results (presentation v3 §6; strict
	// tolerance only — pure comparisons, never model prose, never ±ε):
	//
	// MergedEvidenceIDs lists the observation ids of rows merged into this node
	// by R1 (same subject + same projected ms at 3 decimals + same evidence line
	// range across predicates — e.g. an io_latency row and its same-interval
	// critical_blocking twin) or R2 (same subject+object repeated ≥3 times).
	// EvidenceID stays the lead id; renderers union both for the E# roster.
	MergedEvidenceIDs []string `json:"merged_evidence_ids,omitempty"`
	// MergedCount > 1 marks an R2 ×N aggregate row: ImpactMS/CumulativeImpactMS
	// then carry the SUM over the merged instances and MergedMinMS/MergedMaxMS
	// the per-instance display range (lossless: every instance id is kept).
	MergedCount int     `json:"merged_count,omitempty"`
	MergedMinMS float64 `json:"merged_min_ms,omitempty"`
	MergedMaxMS float64 `json:"merged_max_ms,omitempty"`
	// SecondaryObjects carries the other typed views' Objects after an R1 merge
	// when they differ from this node's Object (e.g. the udk-irq peer thread a
	// same-interval critical_blocking row named) — rendered as an 影响点 note.
	SecondaryObjects []string `json:"secondary_objects,omitempty"`
	// SubjectKind is sourced verbatim from the typed subject_kind rich note.
	// Empty = the subject is a (possibly unresolved) thread. The only non-empty
	// value today is TraceCausalSubjectKindAggregateMetric: the row is a
	// window/CPU-scoped aggregate metric (cpu/io/irq/ipi/frequency/supply
	// pressure) with no thread subject at all — renderers must present the
	// metric semantics instead of an "unresolved thread" and must not seat the
	// row on the on-chain tree (§7.30 裁定1/2).
	SubjectKind string `json:"subject_kind,omitempty"`
	// BlockingKind / BlockingPeer / BlockingHolderSite / BlockingWaiters carry
	// the deterministic lock-contention payload parse (§7.30.3 D1) from the
	// critical_blocking rich notes (blocking_kind / peer / holder_site /
	// waiters). BlockingKind is a typed enum ("monitor_contention" /
	// "lock_contention"); BlockingPeer is the LOCK OWNER's thread label and is
	// empty when the payload named no resolvable owner — renderers then keep
	// the contention semantics but omit the holder, never a bare duration.
	BlockingKind       string `json:"blocking_kind,omitempty"`
	BlockingPeer       string `json:"blocking_peer,omitempty"`
	BlockingHolderSite string `json:"blocking_holder_site,omitempty"`
	BlockingWaiters    int    `json:"blocking_waiters,omitempty"`
}

// TraceCausalSubjectKindAggregateMetric mirrors the trace_query typed
// subject_kind for aggregate-metric rows (window-scoped pressure rows whose
// empty thread subject is structural, not a resolution gap).
const TraceCausalSubjectKindAggregateMetric = "aggregate_metric"

// IsAggregateMetric reports whether this node is a window/CPU-scoped aggregate
// metric row (typed subject_kind check — never a prose or sentinel heuristic).
func (n TraceCausalProjectionNode) IsAggregateMetric() bool {
	return strings.TrimSpace(strings.ToLower(n.SubjectKind)) == TraceCausalSubjectKindAggregateMetric
}

// IsSleepState reports whether this node's dominant state is an (interruptible)
// sleep state — a symptom that must be drilled down to its non-sleep waker via
// the wakeup chain, never itself a root cause. Precise typed check on StateKind
// (never a prose substring match). Deliberately narrow to the S-sleep family:
// io_wait / d_state have their OWN inode/resource drilldown path, not the
// wakeup-chain drilldown this surface models.
func (n TraceCausalProjectionNode) IsSleepState() bool {
	switch strings.TrimSpace(strings.ToLower(n.StateKind)) {
	case "s_sleep", "sleep", "sleep_wait":
		return true
	}
	return false
}

// Undrillable reports whether a sleep symptom could not be resolved to an
// upstream waker (typed UndrillableReason present).
func (n TraceCausalProjectionNode) Undrillable() bool {
	return strings.TrimSpace(n.UndrillableReason) != ""
}

func CompileTraceCausalProjection(ledger ObservationLedger) TraceCausalProjection {
	return TraceCausalProjectionFromObservationRecords(ledger.Records)
}

func TraceCausalProjectionFromObservationRecords(records []ObservationRecord) TraceCausalProjection {
	if len(records) == 0 {
		return TraceCausalProjection{}
	}
	var primary []TraceCausalProjectionNode
	var classified []TraceCausalProjectionNode
	var semantic []TraceCausalProjectionNode
	var hops []TraceCausalProjectionNode
	var wakeupPath []string
	var wakeupEdges []traceCausalProjectionWakeupEdge
	chainRequiredRecommended := false
	wakeupChainObserved := false
	for _, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		// 裁定3 typed inputs: a state_drilldown row recommending the wakeup-chain
		// drilldown (chain_required=true) vs. any wakeup_chain-family observation
		// proving the drilldown actually ran. Exact typed predicate / rich-note
		// matches only.
		switch strings.TrimSpace(record.Predicate) {
		case "wakeup_chain", "wakeup_chain_edge", "wakeup_causal_impact", "wakeup_causal_aggregate":
			wakeupChainObserved = true
		case "state_drilldown":
			if strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, "chain_required")) == "true" {
				chainRequiredRecommended = true
			}
		}
		if edge, ok := traceCausalProjectionWakeupEdgeFromRecord(record); ok {
			wakeupEdges = append(wakeupEdges, edge)
		}
		switch {
		case traceCausalProjectionIsPrimaryRootCause(record):
			node := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record)
			primary = append(primary, node)
			classified = append(classified, node)
		case traceCausalProjectionIsRootCauseContext(record):
			classified = append(classified, traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record))
		case traceCausalProjectionIsSemanticSpan(record):
			node := traceCausalProjectionNodeFromRecord(TraceCausalRoleSemanticSpan, record)
			semantic = append(semantic, node)
			classified = append(classified, node)
		case strings.TrimSpace(record.Predicate) == "wakeup_chain" && len(wakeupPath) == 0:
			wakeupPath = traceCausalProjectionPath(record.Object)
		case traceCausalProjectionIsCausalHop(record):
			node := traceCausalProjectionNodeFromRecord(TraceCausalRoleCausalHop, record)
			hops = append(hops, node)
			classified = append(classified, node)
		}
	}
	pathIndex := traceCausalProjectionPathIndex(wakeupPath)
	sort.SliceStable(primary, func(i, j int) bool {
		return traceCausalProjectionPrimaryLess(primary[i], primary[j], pathIndex)
	})
	primary = traceCausalProjectionDedupeNodes(primary)
	sort.SliceStable(hops, func(i, j int) bool {
		return traceCausalProjectionHopLess(hops[i], hops[j], pathIndex)
	})
	hops = traceCausalProjectionDedupeNodes(hops)
	if len(hops) > traceCausalProjectionSupportingHopLimit {
		hops = hops[:traceCausalProjectionSupportingHopLimit]
	}
	sort.SliceStable(classified, func(i, j int) bool {
		return traceCausalProjectionClassifiedLess(classified[i], classified[j], pathIndex)
	})
	classified = traceCausalProjectionDedupeNodes(classified)
	sort.SliceStable(semantic, func(i, j int) bool {
		return traceCausalProjectionClassifiedLess(semantic[i], semantic[j], pathIndex)
	})
	semantic = traceCausalProjectionDedupeNodes(semantic)
	out := TraceCausalProjection{
		PrimaryRootCauses:            traceCausalProjectionLimitNodes(primary, traceCausalProjectionPrimaryLimit),
		OnChainCauses:                traceCausalProjectionLimitNodes(traceCausalProjectionSelectChainRelevance(classified, "on_chain"), traceCausalProjectionOnChainLimit),
		AdjacentCauses:               traceCausalProjectionLimitNodes(traceCausalProjectionSelectChainRelevance(classified, "adjacent"), traceCausalProjectionContextBucketLimit),
		BackgroundCauses:             traceCausalProjectionLimitNodes(traceCausalProjectionSelectChainRelevance(classified, "background"), traceCausalProjectionContextBucketLimit),
		SemanticSpans:                traceCausalProjectionLimitNodes(semantic, traceCausalProjectionSemanticSpanLimit),
		WakeupPath:                   wakeupPath,
		SupportingHops:               hops,
		WakeupChainRecommendedNotRun: chainRequiredRecommended && !wakeupChainObserved,
	}
	// Presentation v3 §6: deterministic pre-render aggregation (strict tolerance).
	// Runs on the bucketed projection before window marking / drilldown attach so
	// those passes see the final node set. Bucket-overlap semantics (a primary
	// on-chain node also appearing in OnChainCauses as the same-EvidenceID copy)
	// are preserved — renderers keep deduping by node key.
	traceCausalProjectionAggregateForPresentation(&out)
	if len(out.PrimaryRootCauses) > 0 {
		node := out.PrimaryRootCauses[0]
		out.PrimaryRootCause = &node
	}
	if anchorStart, anchorEnd, ok := traceCausalProjectionAnchorWindow(records); ok {
		out.WindowStartTs, out.WindowEndTs = anchorStart, anchorEnd
		traceCausalProjectionMarkWithinWindow(out.PrimaryRootCauses, anchorStart, anchorEnd)
		traceCausalProjectionMarkWithinWindow(out.OnChainCauses, anchorStart, anchorEnd)
		traceCausalProjectionMarkWithinWindow(out.AdjacentCauses, anchorStart, anchorEnd)
		traceCausalProjectionMarkWithinWindow(out.BackgroundCauses, anchorStart, anchorEnd)
		traceCausalProjectionMarkWithinWindow(out.SemanticSpans, anchorStart, anchorEnd)
		traceCausalProjectionMarkWithinWindow(out.SupportingHops, anchorStart, anchorEnd)
		if out.PrimaryRootCause != nil {
			traceCausalProjectionMarkNodeWithinWindow(out.PrimaryRootCause, anchorStart, anchorEnd)
		}
	}
	traceCausalProjectionAttachSleepDrilldownTargets(&out, wakeupEdges, wakeupPath)
	if !out.Active() {
		return TraceCausalProjection{}
	}
	return out
}

type traceCausalProjectionWakeupEdge struct {
	Waker      string
	Wakee      string
	EvidenceID string
	Relation   string
}

func traceCausalProjectionWakeupEdgeFromRecord(record ObservationRecord) (traceCausalProjectionWakeupEdge, bool) {
	if strings.TrimSpace(record.Predicate) != "wakeup_chain_edge" {
		return traceCausalProjectionWakeupEdge{}, false
	}
	waker := strings.TrimSpace(record.Subject)
	wakee := strings.TrimSpace(record.Object)
	if waker == "" || wakee == "" {
		return traceCausalProjectionWakeupEdge{}, false
	}
	return traceCausalProjectionWakeupEdge{
		Waker:      waker,
		Wakee:      wakee,
		EvidenceID: strings.TrimSpace(record.ID),
		Relation:   "wakeup_chain_edge",
	}, true
}

func traceCausalProjectionAttachSleepDrilldownTargets(projection *TraceCausalProjection, edges []traceCausalProjectionWakeupEdge, path []string) {
	if projection == nil {
		return
	}
	targets := traceCausalProjectionUniqueDrilldownTargets(edges, path)
	apply := func(nodes []TraceCausalProjectionNode) {
		for i := range nodes {
			traceCausalProjectionAttachSleepDrilldownTarget(&nodes[i], targets)
		}
	}
	apply(projection.PrimaryRootCauses)
	apply(projection.OnChainCauses)
	apply(projection.AdjacentCauses)
	apply(projection.BackgroundCauses)
	apply(projection.SemanticSpans)
	apply(projection.SupportingHops)
	if projection.PrimaryRootCause != nil {
		traceCausalProjectionAttachSleepDrilldownTarget(projection.PrimaryRootCause, targets)
	}
}

type traceCausalProjectionDrilldownTarget struct {
	Target     string
	EvidenceID string
	Relation   string
	Ambiguous  bool
}

func traceCausalProjectionUniqueDrilldownTargets(edges []traceCausalProjectionWakeupEdge, path []string) map[string]traceCausalProjectionDrilldownTarget {
	raw := map[string]map[string]traceCausalProjectionDrilldownTarget{}
	add := func(wakee, waker, evidenceID, relation string) {
		wakeeKey := traceCausalProjectionCanonicalNode(wakee)
		wakerKey := traceCausalProjectionCanonicalNode(waker)
		if wakeeKey == "" || wakerKey == "" {
			return
		}
		if raw[wakeeKey] == nil {
			raw[wakeeKey] = map[string]traceCausalProjectionDrilldownTarget{}
		}
		raw[wakeeKey][wakerKey] = traceCausalProjectionDrilldownTarget{
			Target:     strings.TrimSpace(waker),
			EvidenceID: strings.TrimSpace(evidenceID),
			Relation:   strings.TrimSpace(relation),
		}
	}
	for _, edge := range edges {
		add(edge.Wakee, edge.Waker, edge.EvidenceID, edge.Relation)
	}
	// The path is deterministic trace_query output, not model prose. Use it only
	// as a fallback when explicit edge rows were absent for that wakee.
	for i := 1; i < len(path); i++ {
		wakeeKey := traceCausalProjectionCanonicalNode(path[i])
		if wakeeKey == "" || len(raw[wakeeKey]) > 0 {
			continue
		}
		add(path[i], path[i-1], "", "wakeup_chain_path")
	}
	out := make(map[string]traceCausalProjectionDrilldownTarget, len(raw))
	for wakee, byWaker := range raw {
		if len(byWaker) != 1 {
			out[wakee] = traceCausalProjectionDrilldownTarget{Ambiguous: true}
			continue
		}
		for _, target := range byWaker {
			out[wakee] = target
		}
	}
	return out
}

func traceCausalProjectionAttachSleepDrilldownTarget(node *TraceCausalProjectionNode, targets map[string]traceCausalProjectionDrilldownTarget) {
	if node == nil || !node.IsSleepState() || node.Undrillable() {
		return
	}
	target, ok := targets[traceCausalProjectionCanonicalNode(node.Subject)]
	if !ok || target.Ambiguous || strings.TrimSpace(target.Target) == "" {
		return
	}
	node.DrilldownTarget = target.Target
	node.DrilldownEvidenceID = target.EvidenceID
	node.DrilldownRelation = target.Relation
}

// traceCausalProjectionAnchorWindow returns the user's originally-requested
// analysis window when a precise, non-circular anchor is available. The only
// such anchor is a frame_target_resolution observation whose window_source is
// an explicit, user-driven value (query_window = user gave pid/thread +
// time_start/time_end; the explicit-union variant = R9). This whitelist is an
// exact typed-string match, never a substring/heuristic. When several exist
// (multiple frames resolved in one turn) the last one wins as the most recent
// pinned window. Returns ok=false when no such record exists, so callers leave
// WithinRequestedWindow nil rather than fabricating a window.
func traceCausalProjectionAnchorWindow(records []ObservationRecord) (float64, float64, bool) {
	var start, end float64
	var ok bool
	for _, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		if strings.TrimSpace(record.Predicate) != "frame_target_resolution" {
			continue
		}
		switch strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, "window_source")) {
		case "query_window", "explicit_query_union_previous_frame_end_to_current_frame_end":
		default:
			continue
		}
		s, e := record.Span.StartTs, record.Span.EndTs
		if s <= 0 || e <= s {
			if ws, we, wok := traceCausalProjectionWindow(record.RichNotes); wok {
				s, e = ws, we
			} else {
				continue
			}
		}
		start, end, ok = s, e, true
	}
	return start, end, ok
}

func traceCausalProjectionMarkWithinWindow(nodes []TraceCausalProjectionNode, anchorStart, anchorEnd float64) {
	for i := range nodes {
		traceCausalProjectionMarkNodeWithinWindow(&nodes[i], anchorStart, anchorEnd)
	}
}

func traceCausalProjectionMarkNodeWithinWindow(node *TraceCausalProjectionNode, anchorStart, anchorEnd float64) {
	if node == nil || node.StartTs <= 0 || node.EndTs <= node.StartTs {
		return
	}
	// "within" = the node's window has any overlap with the requested window;
	// a node fully outside (zero intersection) was drilled into an adjacent
	// window during recursion. Intersection (not strict containment) avoids
	// misclassifying a segment that straddles the window boundary.
	within := node.StartTs < anchorEnd && node.EndTs > anchorStart
	node.WithinRequestedWindow = &within
}

func traceCausalProjectionTraceQueryRecord(record ObservationRecord) bool {
	return record.Origin == AnswerEvidenceOriginRuntimeArtifact &&
		runtimeObservationProducerIsDeterministicQuery(record.Producer) &&
		record.GroundingPolicy == ClaimGroundingHard
}

func traceCausalProjectionIsPrimaryRootCause(record ObservationRecord) bool {
	return strings.TrimSpace(record.Predicate) == "root_cause_primary" ||
		strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_primary")
}

func traceCausalProjectionIsRootCauseContext(record ObservationRecord) bool {
	predicate := strings.TrimSpace(record.Predicate)
	claimKey := strings.TrimSpace(record.ClaimKey)
	if predicate == "" && claimKey == "" {
		return false
	}
	return strings.HasPrefix(predicate, "root_cause_") ||
		strings.HasPrefix(claimKey, "root_cause_")
}

func traceCausalProjectionIsSemanticSpan(record ObservationRecord) bool {
	return strings.TrimSpace(record.Predicate) == "trace_semantic_span" ||
		strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "trace_semantic_span:")
}

func traceCausalProjectionIsCausalHop(record ObservationRecord) bool {
	switch strings.TrimSpace(record.Predicate) {
	case "wakeup_causal_impact", "wakeup_causal_aggregate", "critical_blocking":
		return true
	default:
		return strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_evidence:")
	}
}

func traceCausalProjectionNodeFromRecord(role string, record ObservationRecord) TraceCausalProjectionNode {
	node := TraceCausalProjectionNode{
		Role:            role,
		EvidenceID:      strings.TrimSpace(record.ID),
		Subject:         strings.TrimSpace(record.Subject),
		Predicate:       strings.TrimSpace(record.Predicate),
		Object:          strings.TrimSpace(record.Object),
		Value:           strings.TrimSpace(record.Value),
		Unit:            strings.TrimSpace(record.Unit),
		Summary:         strings.TrimSpace(record.Summary),
		SupportRefs:     cloneStringSlice(record.SupportRefs),
		LineStart:       record.Span.LineStart,
		LineEnd:         record.Span.LineEnd,
		Rank:            traceCausalProjectionRichNoteInt(record.RichNotes, "rank"),
		Tier:            traceCausalProjectionRichNoteValue(record.RichNotes, "tier"),
		Causality:       traceCausalProjectionRichNoteValue(record.RichNotes, "causality"),
		ChainRelevance:  traceCausalProjectionChainRelevance(record.RichNotes),
		ChainDepth:      traceCausalProjectionRichNoteFirstInt(record.RichNotes, "chain_depth", "depth"),
		ImpactMS:        traceCausalProjectionImpact(record),
		SpanName:        traceCausalProjectionRichNoteValue(record.RichNotes, "span_name"),
		SpanKind:        traceCausalProjectionRichNoteValue(record.RichNotes, "span_kind"),
		SpanCategory:    traceCausalProjectionRichNoteValue(record.RichNotes, "span_category"),
		SpanSubcategory: traceCausalProjectionRichNoteValue(record.RichNotes, "span_subcategory"),
		SemanticClass:   traceCausalProjectionRichNoteValue(record.RichNotes, "semantic_class"),
		Confidence:      record.Confidence,
	}
	node.CumulativeImpactMS = traceCausalProjectionRichNoteFloat(record.RichNotes, "cumulative_impact_ms")
	if node.CumulativeImpactMS <= 0 {
		node.CumulativeImpactMS = node.ImpactMS
	}
	node.StartTs = record.Span.StartTs
	node.EndTs = record.Span.EndTs
	if node.StartTs <= 0 && node.EndTs <= 0 {
		if start, end, ok := traceCausalProjectionWindow(record.RichNotes); ok {
			node.StartTs, node.EndTs = start, end
		}
	}
	// Presentation gaps c/d/e: surface the already-emitted (but until now
	// unconsumed) typed rich notes so the renderer can show the duration triad
	// and mark sleep symptoms / undrillable sleeps precisely — no prose parsing.
	node.StateKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, "dominant_state"))
	if node.StateKind == "" {
		// Root-cause / hop rows encode the scheduler state as the Object
		// (sleep_wait / running / io_wait / …). Fall back to it ONLY when it is a
		// recognized state word, so the state column stays a real scheduler state
		// and non-state objects (compute_supply, class_verification) leave it empty.
		node.StateKind = traceCausalProjectionCanonicalStateWord(record.Object)
	}
	node.EffectiveImpactMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, "effective_impact_ms", "effective_impact")
	node.ActualImpactMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, "actual_impact_ms", "actual_impact")
	node.UndrillableReason = traceCausalProjectionUndrillableReason(record)
	// §7.30 裁定1/2: aggregate-metric rows carry a typed subject_kind so the
	// renderer can show metric semantics instead of an "unresolved thread".
	node.SubjectKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, "subject_kind"))
	// §7.30.3 D1: typed lock-contention semantics from the structured payload
	// parse. The peer sentinel ("unknown-thread") means the payload named no
	// resolvable owner — keep BlockingPeer empty rather than a sentinel label.
	node.BlockingKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, "blocking_kind"))
	if node.BlockingKind != "" {
		if peer := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, "peer")); traceCausalProjectionKnownSubject(peer) {
			node.BlockingPeer = peer
		}
		node.BlockingHolderSite = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, "holder_site"))
		node.BlockingWaiters = traceCausalProjectionRichNoteInt(record.RichNotes, "waiters")
	}
	return node
}

// traceCausalProjectionUndrillableReason returns a typed reason when a
// root_evidence observation marks a sleep interval that cannot be resolved to an
// upstream waker. Exact typed claim-key / predicate match — never a substring of
// prose. Currently the only such reason is "missing_wakeup" (query.go:10177,
// "sleep interval has no matching sched_wakeup row in the selected trace window").
func traceCausalProjectionUndrillableReason(record ObservationRecord) string {
	if strings.TrimSpace(record.Predicate) == "missing_wakeup" ||
		strings.TrimSpace(record.ClaimKey) == "root_evidence:missing_wakeup" {
		return "missing_wakeup"
	}
	return ""
}

// traceCausalProjectionCanonicalStateWord returns the raw string when it names a
// recognized scheduler state, else "". Used to derive StateKind from a node's
// Object without letting non-state cause categories leak into the state column.
func traceCausalProjectionCanonicalStateWord(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "running", "runnable", "sleep", "s_sleep", "sleep_wait",
		"d_sleep", "d_state", "io_wait", "uninterruptible_sleep":
		return strings.TrimSpace(raw)
	}
	return ""
}

func traceCausalProjectionRichNoteFirstFloat(notes []string, keys ...string) float64 {
	for _, key := range keys {
		if v := traceCausalProjectionRichNoteFloat(notes, key); v > 0 {
			return v
		}
	}
	return 0
}

// traceCausalProjectionWindow parses a "window" RichNote of the form
// "%.6f..%.6f" (as emitted by trace_query.go traceQueryWindowValue /
// traceQueryTypedTimeWindow) into a start/end pair. ok is true only when both
// ends are positive and end > start.
func traceCausalProjectionWindow(notes []string) (float64, float64, bool) {
	raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, "window"))
	if raw == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(raw, "..", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	start := traceCausalProjectionFloat(parts[0])
	end := traceCausalProjectionFloat(parts[1])
	if start <= 0 || end <= start {
		return 0, 0, false
	}
	return start, end, true
}

func traceCausalProjectionNodeLess(a, b TraceCausalProjectionNode) bool {
	if a.CumulativeImpactMS != b.CumulativeImpactMS {
		return a.CumulativeImpactMS > b.CumulativeImpactMS
	}
	if a.ImpactMS != b.ImpactMS {
		return a.ImpactMS > b.ImpactMS
	}
	if a.Confidence != b.Confidence {
		return a.Confidence > b.Confidence
	}
	if a.Rank > 0 && b.Rank > 0 && a.Rank != b.Rank {
		return a.Rank < b.Rank
	}
	return a.EvidenceID < b.EvidenceID
}

func traceCausalProjectionPrimaryLess(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	aClass := traceCausalProjectionPrimaryClass(a, pathIndex)
	bClass := traceCausalProjectionPrimaryClass(b, pathIndex)
	if aClass != bClass {
		return aClass < bClass
	}
	return traceCausalProjectionNodeLess(a, b)
}

func traceCausalProjectionPrimaryClass(node TraceCausalProjectionNode, pathIndex map[string]int) int {
	onChain := traceCausalProjectionNodeOnChain(node)
	inPath := traceCausalProjectionNodeInPath(node, pathIndex)
	known := traceCausalProjectionKnownSubject(node.Subject)
	switch {
	case onChain && inPath && known:
		return 0
	case inPath && known:
		return 1
	case onChain && known:
		return 2
	case known:
		return 3
	default:
		return 4
	}
}

func traceCausalProjectionHopLess(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	aOnChain := traceCausalProjectionNodeOnChain(a)
	bOnChain := traceCausalProjectionNodeOnChain(b)
	if aOnChain != bOnChain {
		return aOnChain
	}
	aInPath := traceCausalProjectionNodeInPath(a, pathIndex)
	bInPath := traceCausalProjectionNodeInPath(b, pathIndex)
	if aInPath != bInPath {
		return aInPath
	}
	if a.ChainDepth > 0 && b.ChainDepth > 0 && a.ChainDepth != b.ChainDepth {
		return a.ChainDepth < b.ChainDepth
	}
	return traceCausalProjectionNodeLess(a, b)
}

func traceCausalProjectionClassifiedLess(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	aRelevance := traceCausalProjectionChainRelevanceRank(a.ChainRelevance)
	bRelevance := traceCausalProjectionChainRelevanceRank(b.ChainRelevance)
	if aRelevance != bRelevance {
		return aRelevance < bRelevance
	}
	if a.Role != b.Role {
		return traceCausalProjectionRoleRank(a.Role) < traceCausalProjectionRoleRank(b.Role)
	}
	return traceCausalProjectionHopLess(a, b, pathIndex)
}

func traceCausalProjectionRoleRank(role string) int {
	switch strings.TrimSpace(role) {
	case TraceCausalRolePrimaryRootCause:
		return 0
	case TraceCausalRoleCausalHop:
		return 1
	default:
		return 2
	}
}

func traceCausalProjectionNodeOnChain(node TraceCausalProjectionNode) bool {
	return strings.TrimSpace(node.ChainRelevance) == "on_chain" ||
		strings.TrimSpace(node.Causality) == "on_wakeup_chain" ||
		strings.TrimSpace(node.Causality) == "on_dependency_chain"
}

func traceCausalProjectionChainRelevanceRank(relevance string) int {
	switch strings.TrimSpace(relevance) {
	case "on_chain":
		return 0
	case "adjacent":
		return 1
	case "background":
		return 2
	default:
		return 3
	}
}

func traceCausalProjectionLimitNodes(nodes []TraceCausalProjectionNode, limit int) []TraceCausalProjectionNode {
	if limit <= 0 || len(nodes) == 0 {
		return nil
	}
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}
	return append([]TraceCausalProjectionNode(nil), nodes...)
}

func traceCausalProjectionSelectChainRelevance(nodes []TraceCausalProjectionNode, relevance string) []TraceCausalProjectionNode {
	relevance = strings.TrimSpace(relevance)
	if relevance == "" {
		return nil
	}
	var out []TraceCausalProjectionNode
	for _, node := range nodes {
		if strings.TrimSpace(node.ChainRelevance) == relevance {
			out = append(out, node)
		}
	}
	return traceCausalProjectionDedupeNodes(out)
}

func traceCausalProjectionPathIndex(path []string) map[string]int {
	if len(path) == 0 {
		return nil
	}
	out := make(map[string]int, len(path))
	for i, item := range path {
		item = traceCausalProjectionCanonicalNode(item)
		if item != "" {
			out[item] = i + 1
		}
	}
	return out
}

func traceCausalProjectionNodeInPath(node TraceCausalProjectionNode, pathIndex map[string]int) bool {
	if len(pathIndex) == 0 {
		return false
	}
	_, ok := pathIndex[traceCausalProjectionCanonicalNode(node.Subject)]
	return ok
}

func traceCausalProjectionKnownSubject(subject string) bool {
	subject = traceCausalProjectionCanonicalNode(subject)
	return subject != "" && subject != "unknown-thread" && subject != "unknown"
}

func traceCausalProjectionCanonicalNode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func traceCausalProjectionDedupeNodes(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < 2 {
		return nodes
	}
	seen := make(map[string]bool, len(nodes))
	out := make([]TraceCausalProjectionNode, 0, len(nodes))
	for _, node := range nodes {
		key := strings.Join([]string{
			traceCausalProjectionCanonicalNode(node.Role),
			traceCausalProjectionCanonicalNode(node.Subject),
			traceCausalProjectionCanonicalNode(node.Predicate),
			traceCausalProjectionCanonicalNode(node.Object),
			traceCausalProjectionCanonicalNode(strings.Join(node.SupportRefs, "|")),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	return out
}

func traceCausalProjectionPath(raw string) []string {
	parts := strings.Split(raw, "->")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func traceCausalProjectionImpact(record ObservationRecord) float64 {
	if value := traceCausalProjectionRichNoteFloat(record.RichNotes, "impact_ms"); value > 0 {
		return value
	}
	if value := traceCausalProjectionRichNoteFloat(record.RichNotes, "impact"); value > 0 {
		return value
	}
	if strings.TrimSpace(record.Unit) == "ms" {
		return traceCausalProjectionFloat(record.Value)
	}
	return 0
}

func traceCausalProjectionRichNoteValue(notes []string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	if prefix == "=" {
		return ""
	}
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if strings.HasPrefix(note, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(note, prefix))
		}
	}
	return ""
}

func traceCausalProjectionChainRelevance(notes []string) string {
	relevance := traceCausalProjectionRichNoteValue(notes, "chain_relevance")
	switch strings.TrimSpace(relevance) {
	case "on_chain", "adjacent", "background":
		return strings.TrimSpace(relevance)
	}
	switch strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, "causality")) {
	case "on_wakeup_chain", "on_dependency_chain":
		return "on_chain"
	case "adjacent_to_wakeup_chain", "adjacent_to_dependency_chain":
		return "adjacent"
	case "background", "off_chain":
		return "background"
	default:
		return ""
	}
}

func traceCausalProjectionRichNoteFloat(notes []string, key string) float64 {
	return traceCausalProjectionFloat(traceCausalProjectionRichNoteValue(notes, key))
}

func traceCausalProjectionRichNoteInt(notes []string, key string) int {
	value := traceCausalProjectionRichNoteValue(notes, key)
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "ms"))
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func traceCausalProjectionRichNoteFirstInt(notes []string, keys ...string) int {
	for _, key := range keys {
		if value := traceCausalProjectionRichNoteInt(notes, key); value > 0 {
			return value
		}
	}
	return 0
}

func traceCausalProjectionFloat(raw string) float64 {
	value := strings.TrimSpace(raw)
	value = strings.TrimSuffix(strings.ToLower(value), "ms")
	value = strings.TrimSpace(strings.TrimSuffix(value, "毫秒"))
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return f
}
