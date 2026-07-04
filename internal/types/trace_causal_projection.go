package types

import (
	"math"
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
	// ArtifactPath/ArtifactLabel is the typed artifact identity of the trace
	// this projection was compiled from (CMP-1, customer compare audit
	// 2026-07-03 §7.2): the canonicalised SourceRef.Path (shared canonicaliser,
	// never a string heuristic) and its display basename. Populated by the
	// partitioned compile entry (CompileTraceCausalProjections); the legacy
	// single-artifact entry leaves both empty, and single-projection renderers
	// ignore them so single-artifact output stays byte-identical.
	ArtifactPath  string `json:"artifact_path,omitempty"`
	ArtifactLabel string `json:"artifact_label,omitempty"`
	// CapacityTruncated is true when ANY trace_query record compiled into this
	// projection carries the producer's typed "capacity_truncated=true" rich
	// note (NEW-9, adversarial re-review 2026-07-04): the source result hit a
	// per-view row budget (tracequery Result.Compactions non-empty), so the
	// published rows are the rank HEAD of a longer list. Precise boolean from
	// the single producer helper (traceQueryResultCapacityTruncated) — never
	// caveat prose. Display-only: the evidence-index header discloses the
	// truncation; no gate reads it.
	CapacityTruncated bool `json:"capacity_truncated,omitempty"`
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
	// Exception: the R3 subjectless background fold spans DIFFERENT threads, so
	// its ImpactMS/CumulativeImpactMS carry the member MAX, never a sum (V3,
	// customer revisit 2026-07-03 — wall clock does not add across threads).
	MergedCount int     `json:"merged_count,omitempty"`
	MergedMinMS float64 `json:"merged_min_ms,omitempty"`
	MergedMaxMS float64 `json:"merged_max_ms,omitempty"`
	// DuplicatePublications ≥ 2 marks a duplicate-publication fold (V4, customer
	// revisit 2026-07-03): the SAME measurement was published N times as separate
	// observations — exactly equal projected ms on the same (subject, object,
	// type token) over overlapping line/time spans. The row's value is that ONE
	// measurement, never a sum, and MergedCount stays untouched (its ×N carries
	// SUM semantics). Set by the aggregation layer's pre-R2 dedup pass; the
	// renderer's former H6 display-layer fold writes the same field.
	DuplicatePublications int `json:"duplicate_publications,omitempty"`
	// MergedSubjects preserves the DISTINCT member thread subjects of a merged
	// (MergedCount>1) aggregate row, capped at 4 entries (overflow is expressed
	// by MergedCount, never by truncated names). Its load-bearing consumer is
	// the R3 subjectless fold row: without it the renderer's "其余 N 项合并"
	// line loses every folded thread name (customer complaint 2026-07-03).
	MergedSubjects []string `json:"merged_subjects,omitempty"`
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
	// TypeToken mirrors the producer's typed "type=" rich note verbatim (the
	// candidate/root-cause kind token, e.g. blocking_span / d_state_or_io_wait /
	// binder_wait on critical_blocking rows). Precise typed enum from the data
	// layer — renderers use it only to specialize DISPLAY wording (e.g. the
	// unresolved-peer sentinel), never as a behavior gate.
	TypeToken string `json:"type_token,omitempty"`
	// GatedRunnableMS / GatedRunningDeficitMS carry the R5d gated-impact
	// composition of a priority-inversion row (§7.30.3 D3), sourced from the
	// typed gated_runnable / gated_running_deficit rich notes: runnable time
	// counted in full plus the capacity-discounted weak-core running deficit.
	// The renderer shows "影响构成: 可运行等待 X + 运行折算 Y" instead of
	// claiming one scheduler state for the composite amount.
	GatedRunnableMS       float64 `json:"gated_runnable_ms,omitempty"`
	GatedRunningDeficitMS float64 `json:"gated_running_deficit_ms,omitempty"`
	// PeriodicSource / DetectedPeriodMS / PeriodicLatenessMS carry the VS-1
	// (§7.8) periodic-signal-source semantics from the typed periodic_source /
	// detected_period_ms / lateness_ms rich notes: the row's subject is a
	// periodic waker (e.g. a VSync generator) whose in-period sleep is normal
	// cadence — only runnable time and signal lateness count as attribution
	// (EffectiveImpactMS carries that discounted value; on a periodic row it is
	// authoritative even at 0, and ImpactMS keeps the lossless raw projection).
	// The renderer labels such rows and consumes the discount for selection —
	// the flag itself never gates anything structurally.
	PeriodicSource     bool    `json:"periodic_source,omitempty"`
	DetectedPeriodMS   float64 `json:"detected_period_ms,omitempty"`
	PeriodicLatenessMS float64 `json:"periodic_lateness_ms,omitempty"`
	// OccupierSummary carries the RN-1 (§7.9) same-window occupier attribution
	// for a runnable-dominant row: the joined top-occupier roster (thread
	// label + full-window cpu·ms each, e.g. "A-1:120.500ms、B-2:88.100ms")
	// compiled from a "runnable_occupancy" observation whose Subject exactly
	// matches this node's Subject. It answers the mechanism question a bare
	// runnable percentage leaves open — WHO occupied the CPU while this thread
	// sat ready. Display-only (rendered as the runnable row's tail tag and
	// never a gate); empty when no occupancy observation was published for
	// this subject.
	OccupierSummary string `json:"occupier_summary,omitempty"`
	// SupplyFold* carry the VS-2 (§7.10) supply-fold accounting of an
	// on-chain running-dominant row, sourced from the typed
	// supply_fold_deficit_ms / supply_fold_ideal_ms / fold_basis rich notes.
	// SupplyFoldComputed is the presence signal (the fold_basis note was
	// published) — a deficit of exactly 0 with a fully-known basis IS the
	// affirmative "ran at full frequency, running is true workload" fact, so
	// zeros are load-bearing and never collapse into "absent".
	// Deficit = running-SLOW share of the node's OWN running wall clock
	// folded at the big-cluster governed fmax (lower bound, frequency ratio
	// only); Ideal + Deficit reconstructs the folded running total; Known/
	// Unknown split the same total by frequency coverage. Display decision
	// table only (§7.10 four branches) — ranking and gates never read these.
	SupplyFoldComputed  bool    `json:"supply_fold_computed,omitempty"`
	SupplyFoldDeficitMS float64 `json:"supply_fold_deficit_ms,omitempty"`
	SupplyFoldIdealMS   float64 `json:"supply_fold_ideal_ms,omitempty"`
	SupplyFoldKnownMS   float64 `json:"supply_fold_known_ms,omitempty"`
	SupplyFoldUnknownMS float64 `json:"supply_fold_unknown_ms,omitempty"`
	// RunnableMS mirrors the node's typed "runnable=" rich note (the row's
	// own in-window runnable wall clock) — consumed by the §7.10 decision
	// table's shared RN-1 significance check and the mechanism clause's
	// "调度压力 runnable Y ms" magnitude. 0 when the source row did not
	// expose the per-state split.
	RunnableMS float64 `json:"runnable_ms,omitempty"`
	// FullWindowStateMS / FullWindowStateSource carry the RN-12 (§7.9,
	// cust_runnable 2026-07-04) full-window coverage cross-reference: the SAME
	// ledger published a full-window per-state total for this node's exact
	// canonical Subject and state CLASS (state_drilldown / top_runnable /
	// top_sleep families — typed Value, Unit=ms) that exceeds this row's own
	// window projection (×N merged SUM included) by more than the exact ×1.2
	// threshold. The customer read the chain's 635.981ms top fragment as "the
	// tree is truncated" while a state_drilldown row of the same thread held
	// the full-window runnable total 2528.721ms with no cross-reference. The
	// threshold is a precise float comparison (never ±ε) so a row whose value
	// IS the full total (or close to it) never grows a noise annotation.
	// Source is the producer family token of the total's carrier
	// ("state_drilldown" / "top_runnable" / "top_sleep"). Display-only: the
	// renderer emits the coverage tail note; no gate reads these fields.
	FullWindowStateMS     float64 `json:"full_window_state_ms,omitempty"`
	FullWindowStateSource string  `json:"full_window_state_source,omitempty"`
	// F-2 (统一复核 2026-07-04, RN-12 cross-window guard): the carrier
	// observation's own typed selected_window endpoints (seconds) and the
	// precise same-window verdict against the projection anchor window
	// (±1ms per endpoint, float tolerance). SameWindow=false means the total
	// was measured in a DIFFERENT query window than the projection anchor —
	// the display layer must label that window explicitly
	// ("另一查询窗(X–Y)内合计") instead of claiming "窗内": in the recovery
	// dual-window shape the old unconditional "窗内" wording rendered a
	// 2528.721ms runnable total inside a 300ms 关注窗 (an arithmetically
	// impossible claim, total = 8.4× the window length). Totals whose
	// carrier published NO selected_window note are never collected at all
	// (禁猜 — see traceCausalProjectionFullWindowStateTotal).
	FullWindowStateWindowStart float64 `json:"full_window_state_window_start,omitempty"`
	FullWindowStateWindowEnd   float64 `json:"full_window_state_window_end,omitempty"`
	FullWindowStateSameWindow  bool    `json:"full_window_state_same_window,omitempty"`
}

// TraceCausalProjectionStateClass maps a typed scheduler-state token to its
// RN-12 coverage class — the per-class lane the full-window cross-reference
// mechanism operates on ("runnable" and the S-sleep family "sleep"; no
// runnable special case). Exact typed-token switch, never a prose substring:
// io_wait / d_state stay out because they have their own inode/resource
// drilldown lanes (same narrowness rationale as IsSleepState). Exported so
// the display layer resolves the class word from the SAME table the compile
// side used — one source, no drift.
func TraceCausalProjectionStateClass(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "runnable":
		return "runnable"
	case "s_sleep", "sleep", "sleep_wait":
		return "sleep"
	}
	return ""
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
	capacityTruncated := false
	occupiersBySubject := map[string]string{}
	fullWindowStates := map[string]traceCausalProjectionFullWindowState{}
	for _, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		// NEW-9: exact typed note match — the producer publishes it on every
		// record of a capacity-truncated result (single helper, precise bool).
		if strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCapacityTruncated)) == "true" {
			capacityTruncated = true
		}
		// RN-1 (§7.9): a runnable_occupancy observation is a subject-keyed
		// attribution side-channel, never a node of its own — collect the
		// typed occupier roster for the attach pass below and keep it out of
		// the node classification.
		if strings.TrimSpace(record.Predicate) == "runnable_occupancy" {
			if subject := strings.TrimSpace(record.Subject); subject != "" {
				if summary := traceCausalProjectionOccupierRoster(record.RichNotes); summary != "" {
					occupiersBySubject[subject] = summary
				}
			}
			continue
		}
		// RN-12 (§7.9): collect full-window per-state totals as a subject+class
		// keyed side channel (largest total wins — deterministic on record order
		// independence). The carrier families (state_drilldown and the
		// window-stats top_runnable/top_sleep rows) never classify into nodes,
		// so collection never needs a `continue`; the state_drilldown record
		// still falls through to the 裁定3 chain_required check below.
		// F-2: only carriers with a parseable typed selected_window note
		// collect — a total that cannot state its own source window may not
		// make any window claim (禁猜).
		if class, total, ok := traceCausalProjectionFullWindowStateTotal(record); ok {
			key := traceCausalProjectionCanonicalNode(record.Subject) + "\x00" + class
			if prev, exists := fullWindowStates[key]; !exists || total.MS > prev.MS {
				fullWindowStates[key] = total
			}
		}
		// 裁定3 typed inputs: a state_drilldown row recommending the wakeup-chain
		// drilldown (chain_required=true) vs. any wakeup_chain-family observation
		// proving the drilldown actually ran. Exact typed predicate / rich-note
		// matches only.
		switch strings.TrimSpace(record.Predicate) {
		case "wakeup_chain", "wakeup_chain_edge", "wakeup_causal_impact", "wakeup_causal_aggregate":
			wakeupChainObserved = true
		case "state_drilldown":
			if strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainRequired)) == "true" {
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
		CapacityTruncated:            capacityTruncated,
	}
	// Presentation v3 §6: deterministic pre-render aggregation (strict tolerance).
	// Runs on the bucketed projection before window marking / drilldown attach so
	// those passes see the final node set. Bucket-overlap semantics (a primary
	// on-chain node also appearing in OnChainCauses as the same-EvidenceID copy)
	// are preserved — renderers keep deduping by node key.
	traceCausalProjectionAggregateForPresentation(&out)
	// RN-1 (§7.9): attach the same-window occupier roster to runnable nodes
	// (exact Subject match + typed runnable StateKind) after aggregation so
	// merged nodes carry it too, and before the PrimaryRootCause pointer copy.
	traceCausalProjectionAttachRunnableOccupiers(&out, occupiersBySubject)
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
	// RN-12 (§7.9): attach the full-window coverage cross-reference AFTER
	// aggregation (the ×1.2 comparison must run against the ×N merged SUM)
	// and — F-2 — AFTER the anchor window resolution above, because the
	// same-window verdict compares each carrier's selected_window against
	// the resolved anchor. Runs after the PrimaryRootCause pointer copy and
	// therefore attaches to the pointer explicitly.
	traceCausalProjectionAttachFullWindowStateTotals(&out, fullWindowStates)
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

// traceCausalProjectionOccupierRoster joins the typed occupier_1..occupier_3
// rich notes of a runnable_occupancy observation into the display roster
// ("A-1:120.500ms、B-2:88.100ms"). Exact typed note reads only; empty when the
// observation carried no occupier note (nothing to attach).
func traceCausalProjectionOccupierRoster(notes []string) string {
	var parts []string
	for _, key := range []string{TraceNoteKeyOccupier1, TraceNoteKeyOccupier2, TraceNoteKeyOccupier3} {
		if v := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, key)); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, "、")
}

// traceCausalProjectionAttachRunnableOccupiers copies the RN-1 occupier
// roster onto every node whose Subject exactly matches the observation's
// starved subject AND whose typed StateKind is runnable — the attribution is
// about time spent ready-but-off-CPU, so sleep/running/D rows of the same
// thread never inherit it. Pure typed-field plumbing; no gate reads it.
func traceCausalProjectionAttachRunnableOccupiers(projection *TraceCausalProjection, occupiers map[string]string) {
	if projection == nil || len(occupiers) == 0 {
		return
	}
	attach := func(nodes []TraceCausalProjectionNode) {
		for i := range nodes {
			if strings.TrimSpace(strings.ToLower(nodes[i].StateKind)) != "runnable" {
				continue
			}
			if summary := occupiers[strings.TrimSpace(nodes[i].Subject)]; summary != "" {
				nodes[i].OccupierSummary = summary
			}
		}
	}
	attach(projection.PrimaryRootCauses)
	attach(projection.OnChainCauses)
	attach(projection.AdjacentCauses)
	attach(projection.BackgroundCauses)
	attach(projection.SupportingHops)
}

// traceCausalProjectionFullWindowState is one collected RN-12 full-window
// per-state total: the typed ms value, the producer family token of its
// carrier observation, and — F-2 — the carrier's own typed selected_window
// endpoints (seconds), mandatory at collection time.
type traceCausalProjectionFullWindowState struct {
	MS          float64
	Source      string
	WindowStart float64
	WindowEnd   float64
}

// traceCausalProjectionFullWindowStateTotal recognizes an RN-12 full-window
// per-state total carrier and returns its (state class, collected total).
// Precise typed matches only:
//   - Predicate "state_drilldown": the per-thread dominant-state cumulative
//     (typed Value, the customer's 2528.721ms row), state = Object;
//   - Predicate "runnable_wait"/"sleep_wait": the window-stats top_runnable /
//     top_sleep family rows (these predicates belong EXCLUSIVELY to
//     traceQueryTypedThreadDurationObservations — root_cause/hop rows carry
//     those words only as Objects, never as Predicates).
//
// ok=false for any other record, an unclassifiable state token, a missing
// subject, a non-ms/non-positive value, or — F-2 (禁猜) — a missing or
// malformed typed selected_window note: a full-window total whose source
// window is unknown must never be attached, because the display layer could
// only guess which window the "合计" belongs to.
func traceCausalProjectionFullWindowStateTotal(record ObservationRecord) (string, traceCausalProjectionFullWindowState, bool) {
	if strings.TrimSpace(record.Subject) == "" {
		return "", traceCausalProjectionFullWindowState{}, false
	}
	var source string
	switch strings.TrimSpace(record.Predicate) {
	case "state_drilldown":
		source = "state_drilldown"
	case "runnable_wait":
		source = "top_runnable"
	case "sleep_wait":
		source = "top_sleep"
	default:
		return "", traceCausalProjectionFullWindowState{}, false
	}
	class := TraceCausalProjectionStateClass(record.Object)
	if class == "" {
		return "", traceCausalProjectionFullWindowState{}, false
	}
	if strings.TrimSpace(record.Unit) != "ms" {
		return "", traceCausalProjectionFullWindowState{}, false
	}
	ms := traceCausalProjectionFloat(record.Value)
	if ms <= 0 {
		return "", traceCausalProjectionFullWindowState{}, false
	}
	windowStart, windowEnd, ok := traceCausalProjectionSelectedWindowNote(record.RichNotes)
	if !ok {
		return "", traceCausalProjectionFullWindowState{}, false
	}
	return class, traceCausalProjectionFullWindowState{
		MS:          ms,
		Source:      source,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}, true
}

// traceCausalProjectionNodeWindowProjectionMS is the node's window-projection
// display value the RN-12 ×1.2 comparison runs against (after aggregation,
// ImpactMS carries the ×N merged SUM). Fallback chain mirrors the renderer's
// runtimeTraceProjNodeDisplayImpact (internal/tool) — keep the two aligned so
// the compile-side gate and the rendered "仅覆盖 top 片段" value can never
// disagree.
func traceCausalProjectionNodeWindowProjectionMS(node TraceCausalProjectionNode) float64 {
	if node.ImpactMS > 0 {
		return node.ImpactMS
	}
	if node.CumulativeImpactMS > 0 {
		return node.CumulativeImpactMS
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	return node.ActualImpactMS
}

// traceCausalProjectionFullWindowSameWindowToleranceS is the F-2 same-window
// tolerance (±1ms per endpoint, seconds domain): selected_window endpoints
// are float re-renders of the same engine values, so exact equality is not
// guaranteed, but anything beyond 1ms is a genuinely different query window.
const traceCausalProjectionFullWindowSameWindowToleranceS = 0.001

// traceCausalProjectionAttachFullWindowStateTotals copies the RN-12 typed
// cross-reference onto CHAIN-UNIVERSE nodes only (primary / on-chain /
// supporting-hop buckets — the rows the tree renders as chain or flat rows;
// adjacent/background stanza rows are window statistics themselves and never
// carry the note). Attach conditions, all precise: same canonical subject,
// same state class (typed StateKind through the shared class table), and
// full-window total STRICTLY greater than the node's window projection ×1.2 —
// a near-equal total is not a coverage gap and must not grow annotation
// noise. No gate reads the result.
//
// F-2 same-window verdict: the carrier's selected_window endpoints compare
// against the projection anchor window with the ±1ms tolerance — BOTH
// endpoints must match for SameWindow=true ("窗内" wording). A different
// window (recovery dual-window shape) or a projection without an anchor
// window (nothing to verify "窗内" against — 禁猜) sets SameWindow=false and
// the display layer labels the carrier window explicitly. Runs after the
// PrimaryRootCause pointer copy, so it attaches to the pointer explicitly.
func traceCausalProjectionAttachFullWindowStateTotals(projection *TraceCausalProjection, totals map[string]traceCausalProjectionFullWindowState) {
	if projection == nil || len(totals) == 0 {
		return
	}
	sameWindow := func(total traceCausalProjectionFullWindowState) bool {
		if projection.WindowStartTs <= 0 || projection.WindowEndTs <= projection.WindowStartTs {
			return false
		}
		return math.Abs(total.WindowStart-projection.WindowStartTs) <= traceCausalProjectionFullWindowSameWindowToleranceS &&
			math.Abs(total.WindowEnd-projection.WindowEndTs) <= traceCausalProjectionFullWindowSameWindowToleranceS
	}
	attachNode := func(node *TraceCausalProjectionNode) {
		class := TraceCausalProjectionStateClass(node.StateKind)
		if class == "" {
			return
		}
		total, ok := totals[traceCausalProjectionCanonicalNode(node.Subject)+"\x00"+class]
		if !ok {
			return
		}
		projected := traceCausalProjectionNodeWindowProjectionMS(*node)
		if projected <= 0 || total.MS <= projected*1.2 {
			return
		}
		node.FullWindowStateMS = total.MS
		node.FullWindowStateSource = total.Source
		node.FullWindowStateWindowStart = total.WindowStart
		node.FullWindowStateWindowEnd = total.WindowEnd
		node.FullWindowStateSameWindow = sameWindow(total)
	}
	attach := func(nodes []TraceCausalProjectionNode) {
		for i := range nodes {
			attachNode(&nodes[i])
		}
	}
	attach(projection.PrimaryRootCauses)
	attach(projection.OnChainCauses)
	attach(projection.SupportingHops)
	if projection.PrimaryRootCause != nil {
		attachNode(projection.PrimaryRootCause)
	}
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
// analysis window when a precise, non-circular anchor is available. The
// PRIMARY anchor is a frame_target_resolution observation whose window_source
// is an explicit, user-driven value (query_window = user gave pid/thread +
// time_start/time_end; the explicit-union variant = R9). This whitelist is an
// exact typed-string match, never a substring/heuristic. When several exist
// (multiple frames resolved in one turn) the last one wins as the most recent
// pinned window.
//
// CMP-2 (customer compare audit 2026-07-03, §7.2) as corrected by F1
// (adversarial review 2026-07-04): non-frame flows (e.g. bindApplication
// comparisons) never produce a frame_target_resolution row, so the anchor fell
// back to "关注窗口起止未采集" even though trace_query knew the precise
// selected query window. When NO frame anchor exists, the anchor falls back to
// the LAST anchor-family trace_query record carrying the producer's typed
// "selected_window=<start>..<end>" rich note — the engine's own query
// TimeStart/TimeEnd, published verbatim at observation build time. The note is
// the ONLY fallback carrier (the former Span/`window`-note lane is deleted,
// because a wakeup_causal_aggregate record's Span is the MEMBER-IMPACT
// ENVELOPE (engine FirstTs/LastTs), not the selected window — anchoring on it
// fabricated a "关注窗口": 300ms pseudo window → 269% shares + bogus ⚠ tags),
// and — F1, adversarial re-review 2026-07-04 — only the two anchor families
// gated by traceCausalProjectionSelectedWindowAnchorFamily may supply it.
// The frame anchor keeps absolute priority, so every existing frame-anchored
// render is byte-identical. Returns ok=false when no anchor of either lane
// exists, so callers leave WithinRequestedWindow nil and the renderer falls
// back to the relative bar scale rather than fabricating a window.
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
		switch strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyWindowSource)) {
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
	if ok {
		return start, end, true
	}
	for _, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		// F1 (re-review): only the two ANCHOR families may anchor; a record's
		// Span envelope or `window` note never anchors here either.
		if !traceCausalProjectionSelectedWindowAnchorFamily(record) {
			continue
		}
		if s, e, sok := traceCausalProjectionSelectedWindowNote(record.RichNotes); sok {
			start, end, ok = s, e, true
		}
	}
	return start, end, ok
}

// traceCausalProjectionSelectedWindowAnchorFamily reports whether a record
// belongs to one of the producer families whose typed selected_window note
// may act as the CMP-2 fallback ANCHOR: wakeup_causal_aggregate and
// wakeup_causal_impact rows (exact predicate matches — both belong to the
// wakeup CHAIN family, whose selected_window note carries the engine's own
// query window) and the root_cause_rank family (ClaimKey prefix
// "root_cause_" — root_cause_primary/<tier>/root_cause_background all share
// it; root_evidence:* does not). All checks are precise typed matches.
//
// 裁定沿革 (anti-ping-pong): the original CMP-2/F1 ruling deliberately kept NO
// consumer-side family whitelist — "the note itself IS the whitelist" — which
// was sound while only two families emitted it. NEW-8 (账本 §7.6) then
// extended the same note to four DISPLAY-ONLY families (wakeup_causal_impact /
// critical_blocking / state_churn / state_drilldown) so window-basis display
// lines can name endpoints inline; that silently turned the last-wins anchor
// loop into "whichever family published last wins", and a mixed-window session
// (main root_cause_rank window + a later window_stats micro-probe) re-anchored
// the 关注窗口 onto the micro-probe window — wrong 占窗%, bogus ⚠跨窗 tags,
// wrong coverage denominator (F1, adversarial re-review 2026-07-04). F1 then
// pinned a two-family whitelist. RN-5 (§7.9 runnable 主导场景审计 2026-07-04)
// re-admits exactly ONE of the four: wakeup_causal_impact — it is the wakeup
// CHAIN family (same producer pass, same query TimeStart/TimeEnd semantics as
// wakeup_causal_aggregate), and a 6.0-shape session whose ONLY selected_window
// carriers were impact rows fell back to "起止未采集" while every tree row
// wore a bogus ⚠跨窗. The three WINDOW-STATS micro-probe families
// (critical_blocking / state_churn / state_drilldown) stay pure display
// carriers and never anchor — sub-window probes were the actual F1 failure.
//
// Cross-ref (F3, 双"恰三"白名单): this predicate-family whitelist is one of
// TWO anchor gates — the other is the KEY-dimension anchor_window whitelist
// in trace_note_keys.go (exactly selected_window / window / window_source,
// pinned by TestTraceNoteKeyRegistryStructure). A record anchors only when
// BOTH gates pass; a new family emitting selected_window does not auto-anchor.
func traceCausalProjectionSelectedWindowAnchorFamily(record ObservationRecord) bool {
	switch strings.TrimSpace(record.Predicate) {
	case "wakeup_causal_aggregate", "wakeup_causal_impact":
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_")
}

// traceCausalProjectionSelectedWindowNote parses the producer's typed
// "selected_window=<start>..<end>" rich note (F1): exact key-prefix match,
// both ends strict floats (strconv.ParseFloat — no unit-suffix tolerance),
// end > start > 0. This is the only CMP-2 fallback-anchor carrier; a
// malformed or absent note yields ok=false and the legacy "起止未采集"
// behavior.
func traceCausalProjectionSelectedWindowNote(notes []string) (float64, float64, bool) {
	raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, TraceNoteKeySelectedWindow))
	return traceCausalProjectionParseSelectedWindow(raw)
}

// TraceCausalProjectionSelectedWindowNote is the exported parse surface for
// the same typed note (NEW-8, 账本 §7.6): display renderers (metric snapshot
// basis line, trace_query supplement basis token) reuse this ONE strict parser
// so producer format and every consumer stay pinned to a single
// "selected_window=<start>..<end>" shape — no second format, no second parser.
func TraceCausalProjectionSelectedWindowNote(notes []string) (float64, float64, bool) {
	return traceCausalProjectionSelectedWindowNote(notes)
}

func traceCausalProjectionParseSelectedWindow(raw string) (float64, float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(raw, "..", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, errStart := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	end, errEnd := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errStart != nil || errEnd != nil || start <= 0 || end <= start {
		return 0, 0, false
	}
	return start, end, true
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
		Rank:            traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyRank),
		Tier:            traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyTier),
		Causality:       traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCausality),
		ChainRelevance:  traceCausalProjectionChainRelevance(record.RichNotes),
		ChainDepth:      traceCausalProjectionRichNoteFirstInt(record.RichNotes, TraceNoteKeyChainDepth, TraceNoteKeyDepth),
		ImpactMS:        traceCausalProjectionImpact(record),
		SpanName:        traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySpanName),
		SpanKind:        traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySpanKind),
		SpanCategory:    traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySpanCategory),
		SpanSubcategory: traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySpanSubcategory),
		SemanticClass:   traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySemanticClass),
		Confidence:      record.Confidence,
	}
	node.CumulativeImpactMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyCumulativeImpactMS)
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
	node.StateKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyDominantState))
	if node.StateKind == "" {
		// Root-cause / hop rows encode the scheduler state as the Object
		// (sleep_wait / running / io_wait / …). Fall back to it ONLY when it is a
		// recognized state word, so the state column stays a real scheduler state
		// and non-state objects (compute_supply, class_verification) leave it empty.
		node.StateKind = traceCausalProjectionCanonicalStateWord(record.Object)
	}
	node.EffectiveImpactMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyEffectiveImpactMS, TraceNoteKeyEffectiveImpact)
	node.ActualImpactMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyActualImpactMS, TraceNoteKeyActualImpact)
	node.UndrillableReason = traceCausalProjectionUndrillableReason(record)
	// §7.30 裁定1/2: aggregate-metric rows carry a typed subject_kind so the
	// renderer can show metric semantics instead of an "unresolved thread".
	node.SubjectKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySubjectKind))
	// §7.30.3 D1: typed lock-contention semantics from the structured payload
	// parse. The peer sentinel ("unknown-thread") means the payload named no
	// resolvable owner — keep BlockingPeer empty rather than a sentinel label.
	node.BlockingKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockingKind))
	if node.BlockingKind != "" {
		if peer := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyPeer)); traceCausalProjectionKnownSubject(peer) {
			node.BlockingPeer = peer
		}
		node.BlockingHolderSite = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHolderSite))
		node.BlockingWaiters = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyWaiters)
	}
	// §7.30.3 D3: gated-impact composition for priority-inversion rows.
	node.GatedRunnableMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyGatedRunnable)
	node.GatedRunningDeficitMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyGatedRunningDeficit)
	// VS-1 (§7.8): periodic-signal-source semantics — exact typed note match.
	node.PeriodicSource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyPeriodicSource)) == "true"
	if node.PeriodicSource {
		node.DetectedPeriodMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyDetectedPeriodMS)
		node.PeriodicLatenessMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyLatenessMS)
	}
	// VS-2 (§7.10): supply-fold accounting — the fold_basis note is the typed
	// presence signal; deficit/ideal zeros are load-bearing (affirmative
	// fourth branch), so presence is keyed on the note, never on a positive
	// value.
	if basisRaw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldBasis)); basisRaw != "" {
		node.SupplyFoldComputed = true
		node.SupplyFoldKnownMS, node.SupplyFoldUnknownMS = traceCausalProjectionParseFoldBasis(basisRaw)
		node.SupplyFoldDeficitMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeySupplyFoldDeficitMS)
		node.SupplyFoldIdealMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeySupplyFoldIdealMS)
	}
	node.RunnableMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyRunnable)
	// Verbatim typed kind token (see TypeToken doc): lets renderers specialize
	// the unresolved-peer wording for blocking_span / d_state_or_io_wait rows.
	node.TypeToken = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyType))
	return node
}

// traceCausalProjectionParseFoldBasis parses the typed fold_basis note value
// ("known=15.000ms,unknown=0.000ms" — single producer format, see
// traceQueryTypedSupplyFoldRichNotes in internal/tool). Unparseable parts
// yield 0 — the consumer then sees unknown coverage and refuses the
// affirmative claim, never fabricates one.
func traceCausalProjectionParseFoldBasis(raw string) (knownMs, unknownMs float64) {
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		ms := traceCausalProjectionFloat(strings.TrimSuffix(strings.TrimSpace(value), "ms"))
		switch strings.TrimSpace(key) {
		case "known":
			knownMs = ms
		case "unknown":
			unknownMs = ms
		}
	}
	return knownMs, unknownMs
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
// The recognized set IS the registered state-kind universe
// (trace_state_kinds.go) — this production gate reads the single authority, so
// a universe member is producible by construction and a non-member can never
// enter StateKind through the Object lane.
func traceCausalProjectionCanonicalStateWord(raw string) string {
	if TraceStateKindRegistered(strings.TrimSpace(strings.ToLower(raw))) {
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
	raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, TraceNoteKeyWindow))
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
	if value := traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyImpactMS); value > 0 {
		return value
	}
	if value := traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyImpact); value > 0 {
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
	relevance := traceCausalProjectionRichNoteValue(notes, TraceNoteKeyChainRelevance)
	switch strings.TrimSpace(relevance) {
	case "on_chain", "adjacent", "background":
		return strings.TrimSpace(relevance)
	}
	switch strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, TraceNoteKeyCausality)) {
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
