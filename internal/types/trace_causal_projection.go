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
	// WakeupPathUserElected (§10-B1 锚归属, §12.3 裁定3 2026-07-06): true when
	// WakeupPath was ELECTED by a precise typed user-entity match — some
	// wakeup_chain path record matched a typed user entity (compile-side
	// frame_target_resolution explicit_query_target subjects first, then the
	// caller-supplied ledger AnchorUserEntities: runtime_targets pid/thread,
	// then the R2 AnalyzerHints entity face). False = the legacy
	// publication-order lane (first non-empty path record wins). Display
	// consumers use it to keep the 🎯 root label lane (‹用户关注线程›) from ever
	// disagreeing with an entity-elected anchor; no hard gate reads it.
	//
	// EVOLUTION RECORD (§22 B1-b, 2026-07-07): the original B1 predicate read
	// the path record's END element only. The huadong_01 audit (§22 CHAIN-PATH)
	// proved the producer's flattened walk can overshoot chain.Target — nil
	// -impact transit nodes default to depth 0 and sort last, so the END slot is
	// hijacked by an artifact suffix and the end-only predicate was structurally
	// unsatisfiable on that shape. The election now matches the user entity at
	// ANY path position (typed comparator unchanged) and anchors by truncating
	// the elected path at the matched position, so an elected WakeupPath still
	// ends at the user entity. Bar not lowered: same typed entity sources, same
	// cursor exclusion, same no-match legacy fallback.
	WakeupPathUserElected bool `json:"wakeup_path_user_elected,omitempty"`
	// WakeupPathUserEntityHits (§22 B1-b F2, 2026-07-07): the indexes of
	// WakeupPath elements that match a typed anchor user entity (same comparator
	// and same AnchorUserEntities sources as the election; sorted ascending,
	// deduped). Computed on the FINAL selected path — elected or legacy — so
	// display layers (the long-trunk fold's user-entity force-expand) never
	// re-derive entity matches with a diverging comparator. nil when no entity
	// matches (or no entities exist): the no-signal lane stays byte-stable.
	// Display-only guidance; no hard gate reads it.
	WakeupPathUserEntityHits []int `json:"wakeup_path_user_entity_hits,omitempty"`
	// WakeupPathBranch / WakeupPathRootDepth (P0-E CHAIN-PATH, ledger §22.1):
	// the elected path's typed branch ordinal (0 = legacy identity-less
	// candidate) and the engine depth of the elected path's END element (>0
	// only when the B1-b election truncated the path at a mid-chain user
	// entity — the displayed root then sits rootDepth hops up the REAL chain,
	// and the tree's (branch, depth) attach subtracts it so trunk positions
	// stay the engine's true depths).
	//
	// EVOLUTION RECORD (GAP-B G4, §27.2 real_trace_campaign_20260705.md,
	// 2026-07-09): the original "no gate reads either field" sentence is
	// RETIRED — the display tree's depth-attach admission gate READS both
	// fields (P0-E branch domain) and now also the WakeupPathQueryWindow*
	// pair below (window domain). These are hard admission gates on PRECISE
	// typed signals (integer ordinal equality / float endpoints under the ONE
	// shared tolerance), per the 精确信号红线; the anchor-election lanes stay
	// untouched.
	WakeupPathBranch    int `json:"wakeup_path_branch,omitempty"`
	WakeupPathRootDepth int `json:"wakeup_path_root_depth,omitempty"`
	// WakeupPathQueryWindowStartTs/EndTs (GAP-B G4, §27.2, 2026-07-09) is the
	// elected trunk's OWN query-window identity: the typed selected_window
	// note of the wakeup_chain record whose path won the anchor election,
	// through the ONE strict parser (traceCausalProjectionSelectedWindowNote).
	// Branch ordinals are numbered per query window by the engine (each query
	// starts at 1), so (branch, depth) alone collides ACROSS windows — the
	// huadong_79 witness attached a W2 hmfs L2 node under the W1 touch chain
	// and fabricated a "唤醒 OS_mmi_EventHdr" edge. The display tree's depth
	// attach therefore additionally requires the node's query-window identity
	// to match this pair. Zero when the elected record carried no
	// selected_window note — absence never manufactures a rejection domain
	// (the window gate then stays inert; 有损零值禁作硬门反向依据).
	WakeupPathQueryWindowStartTs float64                     `json:"wakeup_path_query_window_start_ts,omitempty"`
	WakeupPathQueryWindowEndTs   float64                     `json:"wakeup_path_query_window_end_ts,omitempty"`
	SupportingHops               []TraceCausalProjectionNode `json:"supporting_hops,omitempty"`
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
	// RootCauseFamilyObserved is true when this run's ledger contains at least
	// one root_cause_-family observation (exact "root_cause_" prefix on the
	// typed Predicate/ClaimKey — the SAME membership check the classification
	// switch uses via traceCausalProjectionIsRootCauseContext). 两态拆分
	// (2026-07-05, specimen real_trace_e1_dual_window_normalized-20260705-212408):
	// renderers key the empty-background-layer explanation on this flag to
	// separate "the background-statistics view never ran this round" (false —
	// the layer simply has no data yet) from "the view ran but its background
	// bucket came back empty" (true — absence of background rows is itself an
	// auditable outcome). Precise typed signal only; never derived from prose.
	RootCauseFamilyObserved bool `json:"root_cause_family_observed,omitempty"`
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
	// QueryWindows (PTV5 Q3, #68 用户裁定 2026-07-05): the DISTINCT typed
	// selected_window endpoint pairs observed across this compile's trace_query
	// records (single strict parser TraceCausalProjectionSelectedWindowNote;
	// ±1ms endpoint dedupe via the F-2 same-window tolerance; ascending start
	// order; capped at 8). DISPLAY-ONLY — the tree header declares "本报告含 N
	// 个查询窗" and the metric snapshot groups by per-record window (NEW-8
	// display 用途); no gate and no anchor derivation reads this list (the
	// anchor lanes above stay untouched).
	QueryWindows []TraceCausalProjectionQueryWindow `json:"query_windows,omitempty"`
	// QueryWindowsTruncated (复核 Low, 2026-07-06): a DISTINCT window arrived
	// after the 8-entry display cap — consumers must render the count as a
	// lower bound ("≥8 个查询窗"), never a fake exact number.
	QueryWindowsTruncated bool `json:"query_windows_truncated,omitempty"`
	// AbsorbedChainRows (G1 跨车道对账 display half, §27.2-G1 user ruling
	// 收口批准 §28.1, 2026-07-09, real_trace_campaign_20260705.md): the
	// critical_blocking nodes whose engine absorption marker
	// (absorbed_by_rank_family=true + absorbed_into=<key>) matched a rank
	// FAMILY node (rank_family_key == that key) present in THIS projection's
	// buckets. traceCausalProjectionFoldAbsorbedChainLaneRows relocates them
	// here BEFORE aggregation, so every render face (tree, ◇/▒ stanzas,
	// metric table, comparison cells) stops seating the duplicate rows
	// without per-face suppression code — while the nodes themselves stay
	// LOSSLESS for the evidence index (E# registration) and the family
	// stanza's 链上并入 disclosure. 负向保护: an absorbed marker whose family
	// key matches NO bucket node leaves the node in its bucket verbatim (the
	// honest duplicate render beats a silent drop). Deduped by EvidenceID
	// (one record can bucket twice: hop copy + classified copy).
	AbsorbedChainRows []TraceCausalProjectionNode `json:"absorbed_chain_rows,omitempty"`
}

// TraceCausalProjectionQueryWindow is one distinct query-window endpoint pair
// (seconds) for the PTV5 Q3 display surfaces.
type TraceCausalProjectionQueryWindow struct {
	StartTs float64 `json:"start_ts,omitempty"`
	EndTs   float64 `json:"end_ts,omitempty"`
}

// TraceCausalProjectionSameValueMember is one member of a cross-thread
// take-MAX fold whose display value ties the fold's published MAX to the µs
// (DIAG A1, §28.11-3(a), real_trace_campaign_20260705.md, 2026-07-09): the
// member's subject plus its OWN evidence line range, so a suspected
// same-segment double attribution (huadong_79 E23: hmfs_discard and the
// target thread both at 14.272ms) is answerable from the report output
// instead of needing the raw trace file.
type TraceCausalProjectionSameValueMember struct {
	Subject   string `json:"subject"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
}

const (
	// TraceCausalProjectionSameValueTieMS is the STRICT same-value criterion
	// of the DIAG A1 disclosure (user ruling §28.11-3: 值差 < 0.0005ms 视同值
	// — the µs-tie shape; NOT the display %.3f rounding band). Exported so the
	// wire-side fold producer (internal/tool) and the projection folds judge
	// ties with the one constant.
	TraceCausalProjectionSameValueTieMS = 0.0005
	// traceCausalProjectionSameValueMemberCap bounds the disclosed member
	// roster (帽 4, mirroring traceCausalProjectionMergedSubjectCap): the tie
	// FACT plus up to four (subject, line-range) witnesses; further tied
	// members stay countable through MergedCount.
	traceCausalProjectionSameValueMemberCap = 4
)

// TraceActualCaliberStateSegmentVsThreadTotal is the single closed-enum value
// of the DIAG A2 two-caliber divergence disclosure (§28.11-3(b), D-10): the
// row carries BOTH a dominant-state segment actual (actual_impact lane) and a
// thread-level actual total (actual_total lane) and they diverge >10% of the
// larger — two calibers from two sources, neither corrected, both stated.
const TraceActualCaliberStateSegmentVsThreadTotal = "state_segment_vs_thread_total"

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
	Role           string   `json:"role,omitempty"`
	EvidenceID     string   `json:"evidence_id,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Predicate      string   `json:"predicate,omitempty"`
	Object         string   `json:"object,omitempty"`
	Value          string   `json:"value,omitempty"`
	Unit           string   `json:"unit,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	SupportRefs    []string `json:"support_refs,omitempty"`
	LineStart      int      `json:"line_start,omitempty"`
	LineEnd        int      `json:"line_end,omitempty"`
	Rank           int      `json:"rank,omitempty"`
	Tier           string   `json:"tier,omitempty"`
	Causality      string   `json:"causality,omitempty"`
	ChainRelevance string   `json:"chain_relevance,omitempty"`
	ChainDepth     int      `json:"chain_depth,omitempty"`
	// TraceGapKind mirrors the producer's typed trace_gap_kind rich note (G2
	// 显示半场, §27.2/§28.1 user ruling 2026-07-09,
	// real_trace_campaign_20260705.md): the PRECISE blind-spot criterion of a
	// trace_gap rank row — closed enum "no_sched_data" (the thread timeline
	// holds no interval at all inside the aligned window) / "no_eligible_wait"
	// (intervals exist but ALL sit below the MinDurationMs floor). Display
	// wording input ONLY (the ◇ inline disclosure forks its wording on it);
	// empty (legacy replays, pre-G2 traces) keeps the legacy wording fail-open.
	// No gate, score or sort lane reads it.
	TraceGapKind string `json:"trace_gap_kind,omitempty"`
	// ChainBranch is the owning branch ordinal of the node's chain measurement
	// (typed chain_branch note — P0-E CHAIN-PATH, ledger §22.1). The display
	// tree keys its depth attach to (branch, depth): a node from a DIFFERENT
	// branch than the elected trunk never fabricates a trunk position (the
	// fake-L26/L27 family); it keeps its honest 未接入树 seat instead. 0 =
	// no branch identity (legacy rows keep the pre-P0-E depth attach).
	ChainBranch        int     `json:"chain_branch,omitempty"`
	ImpactMS           float64 `json:"impact_ms,omitempty"`
	CumulativeImpactMS float64 `json:"cumulative_impact_ms,omitempty"`
	SpanName           string  `json:"span_name,omitempty"`
	SpanKind           string  `json:"span_kind,omitempty"`
	SpanCategory       string  `json:"span_category,omitempty"`
	SpanSubcategory    string  `json:"span_subcategory,omitempty"`
	SemanticClass      string  `json:"semantic_class,omitempty"`
	// StartTs/EndTs is this node's own trace window (seconds), when the source
	// observation exposed one (semantic_span / state_drilldown rows do; plain
	// root_cause primary rows carry only line spans and leave these zero).
	StartTs float64 `json:"start_ts,omitempty"`
	EndTs   float64 `json:"end_ts,omitempty"`
	// QueryWindowStartTs/QueryWindowEndTs is the QUERY window this observation
	// was measured in — the record's own typed selected_window note through the
	// ONE strict parser (traceCausalProjectionSelectedWindowNote; §11-N2,
	// real_trace_campaign_20260705.md). Distinct from StartTs/EndTs (the
	// occurrence segment): two queries over overlapping windows can each carve
	// the SAME physical segment into a per-occurrence row, and only the window
	// identity tells the R2 ×N merge that the members are re-measurements
	// rather than distinct facts. Zero when the record carried no
	// selected_window note — absence never guesses a window. The anchor lanes
	// (two-gate whitelist) are untouched.
	//
	// EVOLUTION RECORD (GAP-B G4, §27.2, 2026-07-09): the original "no gate
	// reads these fields" sentence is RETIRED. The display tree's depth-attach
	// admission gate now compares this pair against the elected trunk's
	// WakeupPathQueryWindow* identity (branch ordinals are per-window and
	// collide across windows — the huadong_79 W2→W1 fake-edge witness). The
	// gate reads PRECISE typed endpoints under the ONE shared tolerance
	// (TraceCausalProjectionSameWindowToleranceS); a node with a ZERO pair on
	// a windowed trunk cannot PROVE domain membership and conservatively keeps
	// its honest 未接入树 seat (缺窗身份≠可挂靠 — the lossy zero never passes
	// the gate in EITHER direction: it neither attaches nor manufactures a
	// rejection when the trunk itself carries no window identity).
	QueryWindowStartTs float64 `json:"query_window_start_ts,omitempty"`
	QueryWindowEndTs   float64 `json:"query_window_end_ts,omitempty"`
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
	// ActualTotalMS is the THREAD-LEVEL actual total (Σ over all scheduler
	// states of the underlying segment) — a DIFFERENT caliber from
	// ActualImpactMS (the dominant-STATE segment actual). DIAG A2 (§28.11-3(b),
	// D-10, real_trace_campaign_20260705.md, 2026-07-09): opendir_79 E5 showed
	// "实际状态 59.050ms" (state-segment) beside "actual_total=112.234ms"
	// (thread total) with no caliber label — the two faces read as a
	// contradiction. Sourced from the actual_total_ms / actual_total rich
	// notes; zero when the source row did not expose it.
	ActualTotalMS float64 `json:"actual_total_ms,omitempty"`
	// ActualCaliberNote is the producer's typed two-caliber divergence
	// disclosure (closed enum, currently only
	// TraceActualCaliberStateSegmentVsThreadTotal): stamped by the engine-side
	// note builders when BOTH actual calibers are present on one row and
	// diverge by more than 10% of the larger. Disclosure only — no gate, no
	// value edit; the detail stanza renders the 实际口径 line from it. Never
	// re-derived display-side (single divergence judgment, one producer).
	ActualCaliberNote string `json:"actual_caliber_note,omitempty"`
	// TargetImpactMS is the engine's TargetBlockedMs caliber: how much of the
	// 🎯 target's own blocked wall clock THIS row's chain actually explains
	// (typed promotion, COV §24.9 D-1, real_trace_campaign_20260705.md,
	// 2026-07-08). Sourced from the target_impact_ms / target_impact rich
	// notes. It is the 已由链上解释 semantic the coverage-sentence numerator
	// consumes FIRST — the CumulativeImpactMS channel is display-overwritten by
	// §20.1 on inversion∧running rank rows (opendir_78: cumulative 58.919 vs
	// target_impact 112.175 fabricated "未归因55%" against a ~97% explained
	// wait). Merge rule everywhere (R1 absorb / R2 fold / R3 fold): member MAX,
	// never Σ — the members explain overlapping stretches of ONE target's
	// blocked clock — and never group-first inheritance (D-3 order-dependence
	// family). Zero when the source row did not expose it (consumers fall back
	// to the legacy cumulative channel byte-identically).
	TargetImpactMS float64 `json:"target_impact_ms,omitempty"`
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
	// Three exceptions: the R3 subjectless background fold spans DIFFERENT
	// threads, so its ImpactMS/CumulativeImpactMS carry the member MAX, never
	// a sum (V3, customer revisit 2026-07-03 — wall clock does not add across
	// threads); a cross-query-window ×N row whose members' occurrence
	// intervals overlap publishes the interval-union caliber instead of the
	// sum (§11-N2 — see MergedIntervalUnion below); and a cross-query-window
	// ×N row whose member QUERY WINDOWS overlap while the union deduction is
	// structurally unavailable publishes the member MAX (§21 CWD — see
	// MergedCrossWindowMax below).
	MergedCount int     `json:"merged_count,omitempty"`
	MergedMinMS float64 `json:"merged_min_ms,omitempty"`
	MergedMaxMS float64 `json:"merged_max_ms,omitempty"`
	// MergedValuelessCount (G12-ENG 修根, §29.1,
	// real_trace_campaign_20260705.md, 2026-07-09) counts the merged members
	// whose display value (ImpactMS → CumulativeImpactMS fallback) is NOT
	// positive: zero-duration marker rows (e.g. sched_blocked_reason
	// critical_blocking observations) that legitimately fold in for roster/
	// evidence losslessness but carry no measurable wall clock. MergedMinMS/
	// MergedMaxMS have always ranged over the POSITIVE displays only, while
	// MergedCount counts every member — so a mixed fold rendered
	// "×2(14.272–14.272ms)取最大", fabricating a second 14.272ms observation
	// under the valueless member's subject (huadong_79 E23: the target's real
	// ×5 binder-wait sum beside hmfs_discard's ×4 zero-duration blocked_reason
	// aggregate read as SAME-SEGMENT DOUBLE ATTRIBUTION and triggered a
	// customer-site raw-trace audit, g12_report.txt). Typed display input for
	// the honest mixed-fold wording; the published value/min/max/count and
	// every rank/score lane stay untouched. Mutation self-check: zeroing this
	// field re-fabricates the E23 form —
	// TestG12OverflowFoldValuelessDisclosure reds.
	MergedValuelessCount int `json:"merged_valueless_count,omitempty"`
	// MergedIntervalUnion marks the §11-N2 cross-query-window union caliber on
	// an R2 ×N row (real_trace_campaign_20260705.md; q2 specimen E10:
	// 104.127+50.057+15.206+14.550 SUMMED to 183.940ms while the 15.206ms
	// occurrence [3680.7995–3680.8192] lay entirely inside the 104.127ms
	// occurrence [3680.6909–3680.8192] — the same physical runnable segment
	// carved once per query window and double-counted ~15.2ms). Set ONLY when
	// members from DISTINCT query windows (typed QueryWindow identity, F-2
	// ±1ms endpoint tolerance) have overlapping occurrence intervals — bare
	// time overlap WITHOUT distinct window identity never engages the lane
	// (PTV6 adjudication family: same-window overlapping same-(subject,object)
	// rows are DISTINCT facts — the E9/E10 9µs strict pin — and keep the SUM).
	// On a union row ImpactMS/CumulativeImpactMS carry the union-caliber value
	// (per-member deduction = min(member value, wall clock already counted by
	// OTHER windows inside the member's interval) — interval algebra on typed
	// StartTs/EndTs, a lower bound that never invents and never deducts more
	// than the physical overlap), MergedSumMS keeps the lossless raw Σ, and
	// the renderer's ×N detail line must say the caliber explicitly:
	// "union 口径(N 窗重叠段不重复计)" — never the SUM wording.
	MergedIntervalUnion bool `json:"merged_interval_union,omitempty"`
	// MergedSumMS is the lossless raw member Σ of a MergedIntervalUnion or
	// MergedCrossWindowMax row (audit trail: published value + MergedSumMS
	// discloses exactly how much cross-window double counting was removed).
	// Zero on plain SUM rows — the published value IS the sum there.
	MergedSumMS float64 `json:"merged_sum_ms,omitempty"`
	// MergedQueryWindows lists the DISTINCT query windows the R2 ×N members
	// were measured in (typed member QueryWindow identity; F-2 ±1ms endpoint
	// dedupe; ascending start order) — the §11-N2 窗身份 disclosure for the ×N
	// detail line (联动 q1-B6). Present whenever at least one member carried a
	// window identity; members WITHOUT identity are not represented here, so
	// renderers must treat the list as the KNOWN sources, never as exhaustive.
	MergedQueryWindows []TraceCausalProjectionQueryWindow `json:"merged_query_windows,omitempty"`
	// MergedCrossWindowMax marks the §21-CWD cross-window MAX caliber on an R2
	// ×N row (cmp_01 revisit audit 2026-07-07, D-新P0 排队深度方向反转 engine
	// half, real_trace_campaign_20260705.md §21): members from DISTINCT query
	// windows whose QUERY WINDOWS overlap in time re-measured overlapping wall
	// clock (or overlapping cpu·ms capacity), so a SUM double-counts — yet the
	// §11-N2 per-segment interval deduction is structurally unavailable
	// (rank-lane members carry no occurrence Span ts, or a member breaks the
	// F-2 containment premise: value > own interval length, the density>1
	// cpu·ms shape). Such a row publishes the member MAX — a lower bound that
	// never invents (R3 cross-thread fold precedent: 墙钟跨窗不可加和) — in
	// ImpactMS/CumulativeImpactMS; MergedSumMS keeps the lossless raw Σ. The
	// specimen bug this roots out: 4 supply_pressure observations from 4
	// overlapping query windows SUMMED to 34008.569ms and then displayed ÷ the
	// 101ms anchor window, inverting the flagship comparison's direction
	// (displayed 6.0 > 7.0 while the tool truth was 7.0 > 6.0). Mutually
	// exclusive with MergedIntervalUnion — the union caliber is more precise
	// and wins whenever it can engage.
	MergedCrossWindowMax bool `json:"merged_cross_window_max,omitempty"`
	// MergedMaxWindowStartTs/EndTs is the typed query window of the member
	// whose display value became the published MAX on a MergedCrossWindowMax
	// row (verbatim member QueryWindowStartTs/EndTs — never a slot
	// representative). The display density normalizes the MAX numerator over
	// THIS window so numerator and denominator share one window base (§21 CWD
	// display half). Zero when that member carried no window identity: the
	// display layer then renders NO density rather than dividing across bases.
	MergedMaxWindowStartTs float64 `json:"merged_max_window_start_ts,omitempty"`
	MergedMaxWindowEndTs   float64 `json:"merged_max_window_end_ts,omitempty"`
	// RankQueryWindowStartTs/EndTs is the typed query window of the member
	// that SUPPLIED this merged row's Rank ordinal (verbatim member
	// QueryWindowStartTs/EndTs at the moment the min-rank member won — never a
	// slot representative). DISP-3 (§29.8 P2-⑧ E22 ◇席窗标缺失回归形,
	// real_trace_campaign_20260705.md, 2026-07-09): a rank ordinal is a
	// PER-WINDOW board identity (§24.13 裁定二), but the §11-N2 merge zeroes
	// the row-level QueryWindow whenever members span windows — the ◇ seat's
	// 根因排序#N chip then lost its 窗X–Ys half on every multi-window merge
	// (huadong_792 E22 vs the pre-merge huadong_79 chips). These fields keep
	// the ordinal's own window identity across the merge so the chip stamper
	// can fall back to it. Zero when the rank-supplying member carried no
	// window identity (absence never guesses). Display chip input only — no
	// gate, score or sort lane reads them.
	RankQueryWindowStartTs float64 `json:"rank_query_window_start_ts,omitempty"`
	RankQueryWindowEndTs   float64 `json:"rank_query_window_end_ts,omitempty"`
	// MergedActualDonorCumulativeMS is the pre-merge CumulativeImpactMS of the
	// member that SUPPLIED this merged row's ActualImpactMS (the merge seed —
	// the actual channel travels verbatim from it and is never re-derived).
	// DISP-3 复核 P2-1 (2026-07-10): the ⚠ predicate's dual-scope carve-out
	// compares actual against the row's cumulative, but the merge overwrites
	// the cumulative with the member SUM — a dual-scope SEED (its own actual
	// == its own pre-merge chain total, the no-⚠ shape) re-emerged from the
	// merge wearing a fabricated ⚠ (berlin E2 REPRO: seed 21.300/27.900/actual
	// 27.900 + a 25.000 member → merged max 25.000 < actual 27.900). This
	// field preserves the donor's own chain total so the display can apply the
	// member-level carve-out. Zero when the seed carried no actual or no
	// cumulative — the display then SUPPRESSES ⚠ on the merged row entirely
	// (conservative arm: a fake ⚠ fabricates, a missing ⚠ merely
	// under-discloses — 宁漏勿假). Internal display carrier only (same lane as
	// RankQueryWindow*/MergedMaxWindow* above — not an LLM-facing schema
	// field, R2' six-spot sync not applicable). No gate, score or sort lane
	// reads it.
	MergedActualDonorCumulativeMS float64 `json:"merged_actual_donor_cumulative_ms,omitempty"`
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
	// PriorityInversionCandidate mirrors the producer's typed
	// "priority_inversion_candidate=true" rich note (PTV5 Q4, #68 用户裁定
	// 2026-07-05 — promoted from a display-only note to a typed node field so
	// hop rows whose Object carries the dominant state still render their
	// inversion candidacy). Display wording only; the R5d gated composition
	// stays on GatedRunnableMS/GatedRunningDeficitMS.
	PriorityInversionCandidate bool `json:"priority_inversion_candidate,omitempty"`
	// RunnableBelowRTPreempted mirrors the producer's typed
	// "runnable_below_rt_preempted=true" rich note (SYM-2 §24.17 R2,
	// 2026-07-08): a SELF runnable-family rank row whose subject — the
	// analysis target — has a priority class below RT (Harmony ohos_cfs) and
	// was displaced by an RT-class competitor overlapping the wait on the same
	// CPU. Display wording only (the 行2 「(优先级低于RT)」 tail); no gate,
	// score or sort lane reads it.
	RunnableBelowRTPreempted bool `json:"runnable_below_rt_preempted,omitempty"`
	// OnChainOverflowFold marks the PTS zero-silent-drop fold row (#68 用户裁定
	// 2026-07-05): on-chain rows beyond a cap (the compile bucket limit, or the
	// producer's per-family wire cap re-materialized from the folded_rows note
	// family) fold into ONE counted row instead of being silently discarded.
	// MergedCount counts the folded ROWS, MergedMinMS/MergedMaxMS their display
	// range, and the published value is the member MAX (wall clock never sums
	// across threads). Renderers show 其余 N 项(链上折叠) and MUST NOT let this
	// row lead a conclusion or win a badge.
	OnChainOverflowFold bool `json:"on_chain_overflow_fold,omitempty"`
	// MergedAllDataGap (G19 显示半场, §27.5, 2026-07-09) marks an overflow fold
	// row EVERY member of which is a typed data-gap row (trace_gap token /
	// tier=data_gap — traceCausalProjectionDataGapRow). Display wording input
	// only: the all-zero fold note may then honestly say the members are data
	// blind spots instead of the generic no-value wording. Never a gate.
	MergedAllDataGap bool `json:"merged_all_data_gap,omitempty"`
	// SameValueMembers (DIAG A1, §28.11-3(a), G12,
	// real_trace_campaign_20260705.md, 2026-07-09) discloses the members of a
	// CROSS-THREAD take-MAX fold whose display values tie the published MAX to
	// the µs (strict |v−max| < traceCausalProjectionSameValueTieMS): the
	// huadong_79 E23 shape — hmfs_discard and the target thread folded ×2 with
	// both members at 14.272ms, the suspected same-segment double-attribution
	// that previously needed the raw trace to check. Each entry keeps the
	// member subject + its OWN evidence line range so the customer can verify
	// segment identity from the report. Cap 4
	// (traceCausalProjectionSameValueMemberCap); set only when ≥2 members tie.
	// Disclosure ONLY — the fold's published value/min/max/count are untouched
	// (zero weight; pinned).
	SameValueMembers []TraceCausalProjectionSameValueMember `json:"same_value_members,omitempty"`
	// --- RCM family-merge typed lane (§24.7.1/§24.10 user rulings 2026-07-08,
	// real_trace_campaign_20260705.md §24.12 dimension-A mandate ①) -----------
	//
	// FamilyMemberCount > 1 marks an ENGINE-side same-(thread,type) /
	// (thread,semantic-class) family merge: the node's value channels carry
	// the family's combined participation value (same-thread — legally
	// additive under the FamilyFoldCaliber ruler), FamilyMemberMaxMS/MinMS the
	// raw member range, FamilyMemberSumMS the lossless raw Σ when the
	// published value sits below it (union/max-fallback disclosure; zero =
	// published == Σ) and FamilyMemberRoster the bounded member inventory
	// carrying the real distinguishing keys (inode/dev/span names — §24.7.1 ①
	// they must never be dropped; overflow disclosed by the count).
	//
	// ISOLATION MANDATE (§24.12 dim-A ①, structural): this lane is a NEW typed
	// carrier, deliberately DISJOINT from MergedCount/MergedMinMS/MergedMaxMS —
	// those belong to the display-side R2/R3/PTS folds whose lead selector
	// collapses ×N rows to their member MAX (墙钟跨线程不可加和). A family
	// total riding the Merged* carriers would be folded back to its largest
	// member and the §24.10 合计参赛 ruling would be dead on arrival. The
	// compile parse below never writes Merged* from member_* notes (and never
	// writes FamilyMember* from folded_* notes) — pinned by negative test.
	FamilyMemberCount  int      `json:"family_member_count,omitempty"`
	FamilyMemberMaxMS  float64  `json:"family_member_max_ms,omitempty"`
	FamilyMemberMinMS  float64  `json:"family_member_min_ms,omitempty"`
	FamilyMemberSumMS  float64  `json:"family_member_sum_ms,omitempty"`
	FamilyFoldCaliber  string   `json:"family_fold_caliber,omitempty"`
	FamilyMemberRoster []string `json:"family_member_roster,omitempty"`
	// --- G1 跨车道对账 typed lane (§27.2-G1, 2026-07-09) ---------------------
	//
	// RankFamilyKey (family side, rank rows): the engine's canonical
	// reconciliation identity from the rank_family_key note — stamped only on
	// a family row that absorbed ≥1 critical_blocking row of the same
	// (thread, adjudicated type family, query window). AbsorbedByRankFamily /
	// AbsorbedInto (absorbed side, critical_blocking rows): the engine's
	// absorption verdict + the SAME identity string. The compile joins the two
	// sides by VERBATIM key equality (one engine renderer, never a display
	// label re-derivation) and relocates matched absorbed nodes into
	// TraceCausalProjection.AbsorbedChainRows; unmatched markers change
	// nothing (负向保护).
	RankFamilyKey        string `json:"rank_family_key,omitempty"`
	AbsorbedByRankFamily bool   `json:"absorbed_by_rank_family,omitempty"`
	AbsorbedInto         string `json:"absorbed_into,omitempty"`
	// BackgroundRank mirrors the producer's typed background_rank note (DCS
	// §23.1: a non-on-chain semantic span-work contender's position among the
	// non-chain rows — the 背景综合排序 board). Promoted for the RCM-2 display
	// half (§24.10 链上 tier 道与非链背景综合排序道同规, 2026-07-08): a family
	// row seated on the background board wears 「背景榜位#N」 on its 行2 the
	// way an on-chain seat wears 根因排序#N. Display wording only — no gate,
	// score or sort lane reads it. Zero when the source row carried none.
	BackgroundRank int `json:"background_rank,omitempty"`
	// Inode / Dev are the typed real distinguishing keys of the inode-keyed IO
	// rank families (§24.9-B F3: previously alive only inside free-text
	// Summary prose — every display face dropped them). Set only when the
	// producer's typed fields agreed across the family; per-member keys live
	// in FamilyMemberRoster.
	Inode string `json:"inode,omitempty"`
	Dev   string `json:"dev,omitempty"`
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
	// "lock_contention"). BlockingPeer's role follows
	// BlockingSubjectIsHolder below (BLK-2 P3a): when
	// BlockingSubjectIsHolder=false (waiter-subject critical_blocking rows)
	// BlockingPeer is the LOCK OWNER's thread label; when true (holder-subject
	// rank rows, BLK §15.C) the subject IS the holder and BlockingPeer is the
	// blocked WAITER. It is empty when the payload named no resolvable
	// counterpart — renderers then keep the contention semantics but omit the
	// counterpart, never a bare duration.
	BlockingKind       string `json:"blocking_kind,omitempty"`
	BlockingPeer       string `json:"blocking_peer,omitempty"`
	BlockingHolderSite string `json:"blocking_holder_site,omitempty"`
	// BlockingFromSite carries the WAITER-side blocking call site of a monitor
	// contention verbatim (the span's "blocking from ..." segment) — the typed
	// blocking_from_site rich note (BLOCKFROM, Wave-3.2 2026-07-09; the note
	// key name is the pinned wire contract with the TEX engine batch). It is
	// the 等待点 counterpart of the 持有点 BlockingHolderSite above: WHERE the
	// waiter blocked, vs WHERE the holder held. Display-only; empty renders
	// nothing (absence never fabricates a site).
	BlockingFromSite string `json:"blocking_from_site,omitempty"`
	BlockingWaiters  int    `json:"blocking_waiters,omitempty"`
	// BlockingHolderSource / BlockingOwnerTidRaw (P0-E 锁车道修3, ledger
	// §24.9-C F5, 2026-07-09): the typed holder-resolution origin
	// (contention_payload / ns_span_derivation / wakeup_edge) and the phantom
	// payload owner tid — the three disclosure faces (tree-row 推断 qualifier
	// / detail 持有者来历 line / lead 括注) key on them; pre-P0-E the engine
	// caveat existed but never reached any user face (置信"中" was the only
	// residue). BlockingHolderHandoff carries the verbatim payload hand-off
	// chain (修2: the named holder is the FINAL holder, never whole-span);
	// BlockingHolderContradiction carries the same-lock self-contradiction
	// withdrawal witness (the row's holder was demoted to unresolved).
	BlockingHolderSource        string `json:"blocking_holder_source,omitempty"`
	BlockingOwnerTidRaw         int    `json:"blocking_owner_tid_raw,omitempty"`
	BlockingHolderHandoff       string `json:"blocking_holder_handoff,omitempty"`
	BlockingHolderContradiction string `json:"blocking_holder_contradiction,omitempty"`
	// BlockingSubjectIsHolder (BLK §15.C, 2026-07-06) mirrors the producer's
	// typed "subject_is_lock_holder=true" note: THIS node's Subject is the lock
	// HOLDER and BlockingPeer is the blocked WAITER (the resolved rank lock
	// row). The renderer then reads the row as a HOLD — "持锁 X ms 阻塞了
	// <BlockingPeer>" — instead of the reversed "锁竞争等待(持有者 <BlockingPeer>)"
	// the waiter-subject critical_blocking node already carries for the SAME
	// physical lock, and the next-step names the HOLDER (the subject), never the
	// waiter. Empty/false keeps the waiter-subject lock-wait wording.
	BlockingSubjectIsHolder bool `json:"blocking_subject_is_holder,omitempty"`
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
	// PTV8-RCR-A (§24.1): the renderer shows the cause node's 行3 breakdown
	// 「有效归因 V = runnable(全额) x + running(折算) y」 plus the 拆解子行
	// (the former 影响构成 tag is retired) instead of claiming one scheduler
	// state for the composite amount.
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
	// SupplyFoldCapabilitySource / GatedCapabilitySource (CAP §26 C3): the
	// typed three-state capability caliber of the two running folds
	// (default_table / evidence_table / freq_only — fold_capability /
	// gated_capability rich notes). Wording inputs only: the display forks
	// the "按默认算力比粗算" / "簇结构不可判,按纯频率比折算" disclosures on
	// these tokens; empty (pre-CAP records) renders the undisclosed legacy
	// wording byte-identically.
	SupplyFoldCapabilitySource string `json:"supply_fold_capability_source,omitempty"`
	GatedCapabilitySource      string `json:"gated_capability_source,omitempty"`
	// SupplyFoldReferenceClass (CAP 复核 F1): the demoted fold-reference class
	// (small/middle/prime — fold_reference_class rich note). Empty = the
	// nominated big-class basis (the producer emits the note only on
	// demotion), so 按大核满频 renders byte-identically on undemoted records.
	SupplyFoldReferenceClass string `json:"supply_fold_reference_class,omitempty"`
	// SupplyFoldTopologySource / GatedTopologySource (CAP-2 §28.4/§28.5 三级
	// 披露词): the typed cluster-STRUCTURE source of the two running folds
	// (freq_comovement / keyed_rail — fold_cluster_topology /
	// gated_cluster_topology rich notes). Wording inputs only: the display
	// upgrades the default-table clause to 按实测频点共动分簇折算 /
	// 按簇轨实测折算(成员按锚点连续推定) on these tokens; empty (explicit
	// topology / legacy records) keeps the 按默认算力比粗算 wording
	// byte-identically.
	SupplyFoldTopologySource string `json:"supply_fold_topology_source,omitempty"`
	GatedTopologySource      string `json:"gated_topology_source,omitempty"`
	// ThermalCapKHz (THERM §28.5-T7, disclosure-only): the fold's dominant
	// running cluster was pressed below its fmax inside the governance window
	// (thermal rail and/or governing limits Max) down to this kHz value —
	// thermal_cap_khz rich note. The display appends the 窗内该簇受热限压至 X
	// sentence; zero-weight (no number changes), absent when cluster
	// attribution was unavailable (absence never guesses).
	ThermalCapKHz int `json:"thermal_cap_khz,omitempty"`
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

// TraceCausalTierTargetSelfState mirrors tracequery.RootCauseTierTargetSelfState
// (SYM §24.13 裁定一, real_trace_campaign_20260705.md, 2026-07-08): the wire
// tier token of a rank row whose subject is the analysis target itself. The
// token is minted ENGINE-side by the typed tid-first subject==target identity
// match and travels here verbatim through the tier rich note — display layers
// consume the token, never a label comparison of their own.
const TraceCausalTierTargetSelfState = "target_self_state"

// IsTargetSelfStateRow reports whether this node is the analysis target's own
// rank row (SYM §24.13 裁定一): typed tier-token equality only. Such rows keep
// their tree seats but never seat on the shared rank board (lead / ❶❷❸) and
// never speak root-cause layer words — the target's own wait/lock-hold is the
// symptom under analysis, not its cause.
//
// EVOLUTION RECORD (跨批 X1, GAP-B 收尾 2026-07-09): the former "keep their
// rank ordinals (榜位照发)" clause is RETIRED — the G9 ordinal renumbering
// assigns rank ordinals only to rows with a seated display identity, so
// symptom rows arrive with Rank=0. Display gates must key on THIS tier token
// alone, never on a `Rank > 0 && IsTargetSelfStateRow()` conjunction (the
// mutually-exclusive pair silently killed the §24.16 disclosure sentence).
func (n TraceCausalProjectionNode) IsTargetSelfStateRow() bool {
	return strings.TrimSpace(n.Tier) == TraceCausalTierTargetSelfState
}

// TraceCausalTierDataGap mirrors tracequery.RootCauseTierDataGap (G2 引擎半场,
// §27.2/§28.1 user ruling 2026-07-09): the wire tier token of a trace_gap
// data-blind-spot rank row — a data gap, never a cause; such rows arrive with
// Rank=0 and take no board seat.
const TraceCausalTierDataGap = "data_gap"

// traceCausalProjectionDataGapRow reports whether the node is a typed
// data-blind-spot row (G2 显示半场 双发布去重, §27.2, 2026-07-09): the engine
// tier token OR the trace_gap type token on the same TypeToken→Object→
// Predicate lane precedence the display predicates use. Exact typed-token
// membership only — never a prose/substring heuristic.
func traceCausalProjectionDataGapRow(node TraceCausalProjectionNode) bool {
	if strings.TrimSpace(node.Tier) == TraceCausalTierDataGap {
		return true
	}
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		if traceCausalProjectionCanonicalNode(token) == "trace_gap" {
			return true
		}
	}
	return false
}

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
	return traceCausalProjectionFromObservationRecords(ledger.Records,
		traceCausalProjectionAnchorEntitiesFromLedger(ledger.AnchorUserEntities))
}

func TraceCausalProjectionFromObservationRecords(records []ObservationRecord) TraceCausalProjection {
	return traceCausalProjectionFromObservationRecords(records, nil)
}

// TraceCausalProjectionFromObservationRecordsForUserEntities is the 裁定3
// (§12.3-3, 2026-07-06) anchor-election compile entry: userEntities is the
// user-entity list in PRIORITY order (runtime_targets user-source pid/thread
// before the R2 AnalyzerHints entity face — see
// ObservationLedger.AnchorUserEntities, the production carrier). Because this
// records-only entry cannot know each entity's carrier provenance, every entry
// is treated as a TYPED-lane entity (the noisy tid-tail / bare-int arms stay
// open) — production callers use the AnchorUserEntities carrier
// (ObservationLedgerAnchorEntities) which marks prose entities so F2/F3 apply.
// nil/empty keeps the legacy publication-order anchor byte-stable.
func TraceCausalProjectionFromObservationRecordsForUserEntities(records []ObservationRecord, userEntities []string) TraceCausalProjection {
	entities := make([]traceCausalProjectionAnchorEntity, 0, len(userEntities))
	for _, value := range userEntities {
		entities = append(entities, traceCausalProjectionAnchorEntity{value: value, typedLane: true})
	}
	return traceCausalProjectionFromObservationRecords(records, entities)
}

func traceCausalProjectionFromObservationRecords(records []ObservationRecord, userEntities []traceCausalProjectionAnchorEntity) TraceCausalProjection {
	if len(records) == 0 {
		return TraceCausalProjection{}
	}
	var primary []TraceCausalProjectionNode
	var classified []TraceCausalProjectionNode
	var semantic []TraceCausalProjectionNode
	var hops []TraceCausalProjectionNode
	// B1 anchor election (§10-B1, §12.3 裁定3): EVERY wakeup_chain path record
	// is an anchor CANDIDATE (publication order preserved); the selection runs
	// after the loop. Each candidate carries the record's typed Subject — the
	// producer publishes threadLabel(chain.Target) there (§22 B1-b: the typed
	// chain-target signal was already on the record; the election consumes it
	// instead of trusting the flattened Object's end slot). wakeupPathEmptyLegacy
	// preserves the exact legacy corner where wakeup_chain records existed but
	// none parsed a non-empty path (the old first-wins assignment left an empty
	// NON-NIL slice behind).
	var wakeupPathCandidates []traceCausalProjectionWakeupPathCandidate
	var wakeupPathEmptyLegacy []string
	var frameTargetEntities []string
	var wakeupEdges []traceCausalProjectionWakeupEdge
	chainRequiredRecommended := false
	wakeupChainObserved := false
	rootCauseFamilyObserved := false
	capacityTruncated := false
	occupiersBySubject := map[string]string{}
	fullWindowStates := map[string]traceCausalProjectionFullWindowState{}
	var queryWindows []TraceCausalProjectionQueryWindow
	queryWindowsTruncated := false
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
		// B1 anchor-entity lane 1 (§12.3 裁定3, as corrected by 二轮复核 F1b):
		// an explicitly-targeted frame_target_resolution's structural Object
		// carries the engine source enum "explicit_query_target". BUT that enum
		// fires for ANY tool call that passed pid/thread — INCLUDING the model's
		// exploration cursor (an evidence_pack/frame bundle over a waker mints
		// one). So the frame subject is only a CORROBORATING signal: it is
		// admitted solely when it AGREES with a caller user entity (which is
		// already cursor-filtered upstream). A frame subject that matches no
		// user entity is the cursor's own frame and must not elect. This keeps
		// the frame lane's value (it supplies the canonical thread LABEL to
		// match against path ends when the user gave only a bare pid) without
		// letting the cursor bundle dominate.
		if strings.TrimSpace(record.Predicate) == "frame_target_resolution" &&
			strings.TrimSpace(record.Object) == traceCausalProjectionFrameTargetSourceExplicit {
			if subject := strings.TrimSpace(record.Subject); subject != "" {
				frameTargetEntities = append(frameTargetEntities, subject)
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
		// 两态拆分 typed input (2026-07-05, 复用裁定3 模式): any root_cause_-family
		// observation (exact "root_cause_" prefix — the SAME membership check the
		// classification switch below uses) proves the background-statistics view
		// actually ran this round. Renderers split the empty-background-layer
		// explanation on the resulting RootCauseFamilyObserved flag.
		if traceCausalProjectionIsRootCauseContext(record) {
			rootCauseFamilyObserved = true
		}
		// PTV5 Q3 (#68 用户裁定 2026-07-05): collect the DISTINCT typed
		// selected_window pairs for the display-only QueryWindows list (single
		// strict parser; ±1ms endpoint dedupe; anchor lanes untouched).
		if s, e, wok := traceCausalProjectionSelectedWindowNote(record.RichNotes); wok {
			var dropped bool
			queryWindows, dropped = traceCausalProjectionAppendQueryWindow(queryWindows, s, e)
			queryWindowsTruncated = queryWindowsTruncated || dropped
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
		case strings.TrimSpace(record.Predicate) == "wakeup_chain":
			// B1 (§10-B1, §12.3 裁定3): collect EVERY parsed path as an anchor
			// candidate instead of the former first-wins assignment. A
			// wakeup_chain record matches no other classification case, so
			// collecting all of them changes NO node classification — records
			// #2..N previously fell through the switch into nothing.
			if path := traceCausalProjectionPath(record.Object); len(path) > 0 {
				branch := traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyChainPathBranch)
				// GAP-B G4: carry the record's own typed selected_window so the
				// elected candidate can publish the trunk's window identity
				// (absence stays zero — never guessed from other records).
				ws, we, wok := traceCausalProjectionSelectedWindowNote(record.RichNotes)
				if !wok {
					ws, we = 0, 0
				}
				wakeupPathCandidates = append(wakeupPathCandidates, traceCausalProjectionWakeupPathCandidate{
					path:        path,
					subject:     strings.TrimSpace(record.Subject),
					branch:      branch,
					branchForm:  branch > 0,
					windowStart: ws,
					windowEnd:   we,
				})
			} else if wakeupPathEmptyLegacy == nil {
				wakeupPathEmptyLegacy = path
			}
		case traceCausalProjectionIsCausalHop(record):
			node := traceCausalProjectionNodeFromRecord(TraceCausalRoleCausalHop, record)
			// PTV6 #1a (real-trace campaign 2026-07-06, specimen donghu_short
			// path_question_absolute): SupportingHops admission carries an
			// ON-CHAIN gate — typed signals only (chain_relevance=on_chain /
			// causality=on_wakeup_chain via the single strict parser, or the
			// root_evidence: audit family). A hop-family record classified
			// background (the specimen: critical_blocking
			// chain_relevance=background) must NOT double-cast into the hops
			// lane — its classified copy below is its ONLY seat (background
			// stanza), never a second on-chain tree seat.
			if traceCausalProjectionHopOnChain(record) {
				hops = append(hops, node)
			} else if node.ChainRelevance == "" {
				// [P1 修正轮 2026-07-06] UNDECLARED-relevance micro-probe rows
				// (reachable healthy shape: untargeted query / empty chain →
				// engine ChainRelevance "") failed the on-chain gate AND had
				// no relevance bucket to land in — zero seats anywhere, and a
				// lone such record flipped Active() false. Default the
				// classified copy into the background seat: only a
				// self-declared on-chain signal may reach the chain (#1a
				// 主树纯净), and the background stanza keeps the honest seat
				// (PTS 永不静默丢). Declared adjacent/background rows are
				// untouched (their bucket already seats them).
				node.ChainRelevance = "background"
			}
			classified = append(classified, node)
		}
	}
	// B1 anchor election (§10-B1, §12.3 裁定3): the selected path feeds
	// EVERYTHING downstream — pathIndex sorting, the projection WakeupPath (the
	// tree trunk / 🎯 root), and the drilldown-target path fallback — so the
	// election happens HERE at the compile root, never as a display-side
	// re-root (a post-compile re-root would leave the sort/cap decisions keyed
	// to the wrong trunk). Entity priority: frame subjects that CORROBORATE a
	// user entity (F1b) rank first (they carry the canonical thread label),
	// then the caller user entities in their own priority order.
	anchorEntities := traceCausalProjectionOrderedAnchorEntities(frameTargetEntities, userEntities)
	wakeupPath, wakeupPathUserElected, wakeupPathElected, wakeupPathRootDepth := traceCausalProjectionSelectWakeupPath(
		wakeupPathCandidates, anchorEntities)
	if wakeupPath == nil {
		wakeupPath = wakeupPathEmptyLegacy
	}
	// §22 B1-b F2: record which FINAL-path elements name a typed user entity
	// (single comparator, computed once at the compile root) so the display
	// fold layer can force-expand them without re-deriving entity matches.
	wakeupPathUserEntityHits := traceCausalProjectionPathUserEntityHits(wakeupPath, anchorEntities)
	pathIndex := traceCausalProjectionPathIndex(wakeupPath)
	sort.SliceStable(primary, func(i, j int) bool {
		return traceCausalProjectionPrimaryLess(primary[i], primary[j], pathIndex)
	})
	primary = traceCausalProjectionDedupeNodes(primary)
	sort.SliceStable(hops, func(i, j int) bool {
		return traceCausalProjectionHopLess(hops[i], hops[j], pathIndex)
	})
	hops = traceCausalProjectionDedupeNodes(hops)
	// PTV6 (PTS 连带, 永不静默丢): the former silent hops[:limit] discard is
	// gone — the hop surface caps by folding with a count AFTER aggregation
	// (below), where the count is the post-merge truth (复核 P1 同型).
	sort.SliceStable(classified, func(i, j int) bool {
		return traceCausalProjectionClassifiedLess(classified[i], classified[j], pathIndex)
	})
	classified = traceCausalProjectionDedupeNodes(classified)
	sort.SliceStable(semantic, func(i, j int) bool {
		return traceCausalProjectionClassifiedLess(semantic[i], semantic[j], pathIndex)
	})
	semantic = traceCausalProjectionDedupeNodes(semantic)
	out := TraceCausalProjection{
		PrimaryRootCauses: traceCausalProjectionLimitNodes(primary, traceCausalProjectionPrimaryLimit),
		// PTS (#68 用户裁定 2026-07-05): the on-chain bucket enters aggregation
		// UNCAPPED — the fold-with-count cap applies AFTER R1/R4/V4/R2 (复核
		// P1, 2026-07-06: a pre-aggregation fold counted rows the alias/
		// same-fact merges would have folded away — the donghu replay showed a
		// fake "其余 16 项" made of duplicates). Primary-bucket overflow of an
		// on_chain node is NOT a tree drop: the same record's classified copy
		// still enters this bucket; adjacent/background buckets are off-chain
		// by definition and keep the plain pre-aggregation limiter.
		OnChainCauses:                traceCausalProjectionSelectChainRelevance(classified, "on_chain"),
		AdjacentCauses:               traceCausalProjectionLimitNodes(traceCausalProjectionSelectChainRelevance(classified, "adjacent"), traceCausalProjectionContextBucketLimit),
		BackgroundCauses:             traceCausalProjectionLimitNodes(traceCausalProjectionSelectChainRelevance(classified, "background"), traceCausalProjectionContextBucketLimit),
		SemanticSpans:                traceCausalProjectionLimitNodes(semantic, traceCausalProjectionSemanticSpanLimit),
		WakeupPath:                   wakeupPath,
		WakeupPathUserElected:        wakeupPathUserElected,
		WakeupPathUserEntityHits:     wakeupPathUserEntityHits,
		WakeupPathBranch:             wakeupPathElected.branch,
		WakeupPathRootDepth:          wakeupPathRootDepth,
		WakeupPathQueryWindowStartTs: wakeupPathElected.windowStart,
		WakeupPathQueryWindowEndTs:   wakeupPathElected.windowEnd,
		SupportingHops:               hops,
		WakeupChainRecommendedNotRun: chainRequiredRecommended && !wakeupChainObserved,
		RootCauseFamilyObserved:      rootCauseFamilyObserved,
		CapacityTruncated:            capacityTruncated,
		QueryWindows:                 traceCausalProjectionSortQueryWindows(queryWindows),
		QueryWindowsTruncated:        queryWindowsTruncated,
	}
	// G1 跨车道对账 display half (§27.2-G1, 2026-07-09): relocate absorbed
	// critical_blocking nodes out of the render buckets BEFORE aggregation —
	// R1/R2/V4 then never see the duplicate rows (no ×N chimera of absorbed +
	// non-absorbed members), every render face inherits the fold from the
	// buckets, and the nodes stay lossless on AbsorbedChainRows for the
	// evidence index and the family stanza's 链上并入 disclosure.
	traceCausalProjectionFoldAbsorbedChainLaneRows(&out)
	// Presentation v3 §6: deterministic pre-render aggregation (strict tolerance).
	// Runs on the bucketed projection before window marking / drilldown attach so
	// those passes see the final node set. Bucket-overlap semantics (a primary
	// on-chain node also appearing in OnChainCauses as the same-EvidenceID copy)
	// are preserved — renderers keep deduping by node key.
	traceCausalProjectionAggregateForPresentation(&out)
	// PTS fold-cap runs on the AGGREGATED bucket (复核 P1, 2026-07-06): the
	// count is the post-merge truth, and every attach pass below sees the
	// final node set. The fold row appends after the impact-major resort —
	// its member-MAX value is by construction ≤ every kept row's display, so
	// order stays coherent without a second sort.
	//
	// PTV6 (PTS 连带): the SupportingHops surface folds the same way — and it
	// runs BEFORE the on-chain bucket fold so the cross-bucket overlap check
	// sees the UNCAPPED post-aggregation on-chain bucket: an overflow hop whose
	// evidence id is already represented there is the deliberate bucket overlap
	// (renderers dedupe by node key), not a silent drop; only hops represented
	// NOWHERE else fold with a count (F1 真计数同型 — no fake "其余 N 项" made
	// of rows that render anyway).
	out.SupportingHops = traceCausalProjectionLimitHopsFold(out.SupportingHops, out.OnChainCauses, traceCausalProjectionSupportingHopLimit)
	// G2 显示半场 双发布去重 (§27.2, 2026-07-09): a data-blind-spot thread that
	// already holds its own individual ◇/▒ stanza seat (adjacent/background
	// bucket copy — the higher-information surface: kind wording, evidence,
	// disclosure) is EXCLUDED from the on-chain overflow fold membership, so
	// one thread's blind spot publishes once instead of "×N(0.000…) fold
	// member + ◇ row" twice. Seat-conditioned (never unconditional): a
	// blind-spot row with NO individual seat still folds — zero silent drops.
	out.OnChainCauses = traceCausalProjectionLimitNodesOnChainFold(out.OnChainCauses, traceCausalProjectionOnChainLimit,
		traceCausalProjectionSeatedDataGapSubjects(out.AdjacentCauses, out.BackgroundCauses))
	// RN-1 (§7.9): attach the same-window occupier roster to runnable nodes
	// (exact Subject match + typed runnable StateKind) after aggregation so
	// merged nodes carry it too, and before the PrimaryRootCause pointer copy.
	traceCausalProjectionAttachRunnableOccupiers(&out, occupiersBySubject)
	// SFD (§15.A display half, user q6 issue 1): join the engine-published
	// supply-fold accounting onto the same segment's running twin projection.
	// Runs AFTER aggregation (final node set, ×N fold rows excluded inside)
	// and BEFORE the PrimaryRootCause pointer copy so the pointer inherits.
	traceCausalProjectionJoinSupplyFoldTwins(&out)
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

// traceCausalProjectionFrameTargetSourceExplicit is the ONE
// FrameTargetResolution.Source enum value that denotes an explicitly-passed
// query pid/thread (tracequery ResolveFrameTarget; the record publishes it
// verbatim as the structural Object). It is the only frame-resolution lane
// admitted into the B1 anchor election — every other Source value is a
// frame-timeline HEURISTIC candidate pick and stays out (精确信号硬门,
// 嘈声信号不入闸).
const traceCausalProjectionFrameTargetSourceExplicit = "explicit_query_target"

// traceCausalProjectionOrderedAnchorEntities builds the priority-ordered
// anchor-election entity list (§12.3 裁定3, 二轮复核 F1b). frameSubjects are
// the compile-side explicit_query_target frame_target_resolution subjects; a
// frame subject is admitted ONLY when it CORROBORATES a caller user entity —
// i.e. matches one via the same precise comparator the election uses. This
// neutralizes the cursor: the model's exploration bundle mints a frame subject
// too, but that subject matches no (cursor-filtered) user entity, so it never
// enters the list. Admitted frame subjects rank FIRST because they carry the
// canonical thread LABEL (the user side is often a bare pid), giving the label
// arm of the match a name to compare; the caller user entities follow in their
// own priority order.
func traceCausalProjectionOrderedAnchorEntities(frameSubjects []string, userEntities []traceCausalProjectionAnchorEntity) []traceCausalProjectionAnchorEntity {
	out := make([]traceCausalProjectionAnchorEntity, 0, len(frameSubjects)+len(userEntities))
	for _, subject := range frameSubjects {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			continue
		}
		frameEntity := traceCausalProjectionAnchorEntity{value: subject, typedLane: true}
		for _, user := range userEntities {
			if strings.TrimSpace(user.value) == "" {
				continue
			}
			// Corroboration is symmetric: the frame subject (a full thread
			// label) matched by the user entity's provenance-honoring arms.
			if traceCausalProjectionAnchorLabelMatchesEntity(subject, user) {
				out = append(out, frameEntity)
				break
			}
		}
	}
	return append(out, userEntities...)
}

// traceCausalProjectionAnchorEntity is one anchor-election user-entity
// candidate with its typed provenance. typedLane=true marks entities from
// TYPED carriers (runtime_targets user-source pid/thread, ExactTargets, the
// compile-side explicit_query_target frame subject) — only these may use the
// noisy "-<digits>" tid-tail and bare-integer parse arms. typedLane=false
// marks PROSE carriers (AnalyzerHints.Entities): a free-text string like
// "2026-07-06" or "issue-42" must NOT be mined for a tid handle (F2/F3), so a
// prose entity only matches via pure "pid=N" / whole-canonical equality —
// exactly the R2 comparator discipline.
type traceCausalProjectionAnchorEntity struct {
	value     string
	typedLane bool
}

// traceCausalProjectionWakeupPathCandidate is one wakeup_chain path record's
// election candidate: the parsed Object path plus the record's typed Subject
// (the producer publishes threadLabel(chain.Target) there — §22 B1-b: the
// precise chain-target signal that survives the flattened Object's end-slot
// artifact).
type traceCausalProjectionWakeupPathCandidate struct {
	path    []string
	subject string
	// branch / branchForm (P0-E CHAIN-PATH, ledger §22.1): the record's typed
	// branch= note — a per-branch TRUE path record. When ANY branch-form
	// candidate exists, the election pools branch-form candidates ONLY: the
	// retired flattened walk (and the legacy text-parse lane's identity-less
	// reconstructions) never compete against real branch paths.
	branch     int
	branchForm bool
	// windowStart/windowEnd (GAP-B G4, §27.2, 2026-07-09): the record's own
	// typed selected_window note (ONE strict parser) — the elected candidate
	// publishes it as the projection's WakeupPathQueryWindow* trunk identity.
	// Zero when the record carried no note (identity carriage only; the
	// election ladder never reads these).
	windowStart float64
	windowEnd   float64
}

// traceCausalProjectionSelectWakeupPath elects the projection anchor path
// (§10-B1 归因, §12.3 裁定3 用户裁定 2026-07-06: "存在 target 匹配用户实体的
// path 记录时,锚优先给用户实体线程;第一条 path 先到先得废止").
//
// EVOLUTION RECORD (§22 B1-b, 2026-07-07 — huadong_01 CHAIN-PATH audit): the
// original predicate matched the path's END element only. The producer's
// flattened multi-branch walk can overshoot chain.Target (nil-impact transit
// nodes default to depth 0 and sort last), so on that shape the end slot holds
// an artifact transit node and the end-only predicate was STRUCTURALLY
// unsatisfiable — the user thread sat mid-path while the root fell back to
// candidates[0] (VSyncGenerator + 免责横幅). The election now:
//
//   - candidates: every wakeup_chain path record's parsed path (publication
//     order preserved) plus its typed Subject (= threadLabel(chain.Target)).
//   - entities are already in priority order (the caller front-loads the
//     compile-side explicit_query_target frame subjects, then the ledger
//     AnchorUserEntities). Every entity carries its typed/prose provenance so
//     the match arms honor F2/F3 (prose entities never mine a tid handle).
//   - match predicate: the entity against ANY path element, via
//     traceCausalProjectionAnchorLabelMatchesEntity (tid-first, §11-N7,
//     comparator unchanged). Within one path the LAST matching position wins —
//     everything after it is walk overshoot (a well-formed target-terminated
//     path cannot continue past its target), and earlier duplicates are the
//     legitimate ↺ cycle shape that stays on the trunk. A match at position 0
//     is NOT electable: the elected path is truncated at the matched position,
//     and a position-0 root has no upstream chain to root a tree on (the ≥2
//     -element rule of the original election, preserved).
//   - anchoring: the elected path is the candidate TRUNCATED at the matched
//     position (compile-root anchoring, not a display re-root — pathIndex
//     sorting, trunk, caps and drilldown targets all key off the returned
//     path). The dropped suffix nodes keep their own observation records and
//     bucket seats; only the artifact trunk membership goes.
//   - tie-break across multiple matching candidates FOR ONE ENTITY (裁定:
//     §22 B1-b "位置最深/影响最大" — reasons pinned in the B1-b tests):
//     1. a candidate whose typed Subject ALSO matches the entity outranks a
//     position-only hit — the engine declared that chain's target to BE the
//     user entity, whereas a mid-path hit may be the user as transit in
//     another thread's chain (weaker claim on the same precise lane);
//     2. then the DEEPEST matched position (largest index) — the truncated
//     trunk retains the most resolved upstream causation for the user
//     entity (path records carry no typed per-path impact scalar, so
//     "影响最大" has no precise carrier; deriving one from neighboring
//     records would be a noisy join — 精确信号红线);
//     3. then publication order (FIRST wins — the original pin ④ rule,
//     preserved as the terminal tie-break).
//     Entities still scan in priority order (a lower-priority entity never
//     preempts a higher-priority entity's match).
//   - no entities / no match: candidates[0] — the legacy publication-order
//     anchor, byte-stable (pin ②/③).
//
// The second return reports a user election (feeds
// TraceCausalProjection.WakeupPathUserElected → the 🎯 root label lane).
//
// P0-E CHAIN-PATH EVOLUTION (ledger §22.1, 2026-07-09): candidates are now
// per-branch TRUE path records (one per top-level target-segment expansion;
// typed branch= note), so the election picks a BRANCH — the flattened
// cross-branch walk is retired from the producer. Semantics re-reviewed on
// real branches, bar only strengthened:
//   - pool switch: when ANY branch-form candidate exists, ONLY branch-form
//     candidates compete (a stray identity-less candidate — e.g. the legacy
//     text-parse reconstruction lane — never outranks a real branch);
//   - the any-position match + last-occurrence truncation are UNCHANGED (a
//     user entity can still sit mid-branch as a transit of a longer chain);
//     位置0不可当选 preserved;
//   - tie ladder unchanged: typed Subject hit > deepest matched position >
//     publication order. On true branches "deepest position" now reads
//     "longest resolved upstream causation for the entity" — exactly the
//     §22.1 rationale, minus the cross-branch stitching artifacts.
//
// The third return is the ELECTED CANDIDATE itself (GAP-B G4, 2026-07-09 —
// evolves the former bare branch-ordinal return): the caller publishes its
// typed branch ordinal AND its typed selected_window identity (the display
// tree's (branch, window, depth) attach domain inputs). The fourth return is
// the engine depth of the returned path's END element (0 unless the
// truncation dropped a suffix).
func traceCausalProjectionSelectWakeupPath(candidates []traceCausalProjectionWakeupPathCandidate, entities []traceCausalProjectionAnchorEntity) ([]string, bool, traceCausalProjectionWakeupPathCandidate, int) {
	if len(candidates) == 0 {
		return nil, false, traceCausalProjectionWakeupPathCandidate{}, 0
	}
	branchForm := false
	for _, candidate := range candidates {
		if candidate.branchForm {
			branchForm = true
			break
		}
	}
	if branchForm {
		pooled := make([]traceCausalProjectionWakeupPathCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.branchForm {
				pooled = append(pooled, candidate)
			}
		}
		candidates = pooled
	}
	for _, entity := range entities {
		if strings.TrimSpace(entity.value) == "" {
			continue
		}
		bestIdx, bestPos, bestSubject := -1, -1, false
		for i, candidate := range candidates {
			pos := traceCausalProjectionPathLastEntityMatch(candidate.path, entity)
			if pos < 1 {
				continue // no hit, or a position-0 hit with no upstream chain
			}
			subjectMatch := candidate.subject != "" &&
				traceCausalProjectionAnchorLabelMatchesEntity(candidate.subject, entity)
			better := false
			switch {
			case bestIdx < 0:
				better = true
			case subjectMatch != bestSubject:
				better = subjectMatch
			case pos != bestPos:
				better = pos > bestPos
			}
			if better {
				bestIdx, bestPos, bestSubject = i, pos, subjectMatch
			}
		}
		if bestIdx >= 0 {
			elected := candidates[bestIdx]
			return elected.path[:bestPos+1], true, elected, len(elected.path) - 1 - bestPos
		}
	}
	return candidates[0].path, false, candidates[0], 0
}

// traceCausalProjectionPathLastEntityMatch returns the LAST path position whose
// label matches the entity (§22 B1-b any-position predicate), -1 when none.
// Last occurrence: everything after it is producer walk overshoot, earlier
// occurrences are the ↺ cycle shape that belongs on the trunk.
func traceCausalProjectionPathLastEntityMatch(path []string, entity traceCausalProjectionAnchorEntity) int {
	for i := len(path) - 1; i >= 0; i-- {
		if traceCausalProjectionAnchorLabelMatchesEntity(path[i], entity) {
			return i
		}
	}
	return -1
}

// traceCausalProjectionPathUserEntityHits lists the positions of the FINAL
// selected wakeup path that match ANY typed anchor user entity (§22 B1-b F2 —
// the fold layer's force-expand input; ascending, deduped, nil when nothing
// matches so the no-signal lane stays byte-stable for the DeepEqual pins).
func traceCausalProjectionPathUserEntityHits(path []string, entities []traceCausalProjectionAnchorEntity) []int {
	if len(path) == 0 || len(entities) == 0 {
		return nil
	}
	var hits []int
	for i, label := range path {
		for _, entity := range entities {
			if strings.TrimSpace(entity.value) == "" {
				continue
			}
			if traceCausalProjectionAnchorLabelMatchesEntity(label, entity) {
				hits = append(hits, i)
				break
			}
		}
	}
	return hits
}

// traceCausalProjectionAnchorLabelMatchesEntity decides whether a wakeup-path
// END label names a user entity. PRECISE signals only (架构红线:硬门只读精确
// 信号), mirroring the tool-side R2 comparator
// (runtimeTraceProjTargetMatchesUserEntities) plus the §11-N7 tid-first rule,
// with the F2/F3 provenance guard on the noisy parse arms:
//
//  1. tid equality DECIDES whenever BOTH sides expose a tid — a thread label's
//     pure-digit "-pid" tail or the "pid=N" handle on either side, plus (TYPED
//     entities only) a bare pure-digit string. 同 tid 双名 (com.xs.fm.lite-6565
//     vs main-6565) matches; equal comm with a DIFFERENT tid never matches.
//     The entity's "-<digits>" tail and bare-int arms are gated on typedLane:
//     a PROSE entity ("2026-07-06", "issue-42") must never be mined for a tid
//     (F2/F3) — it exposes a tid only through the explicit "pid=N" handle.
//  2. otherwise canonical whole-label equality;
//  3. otherwise the R2 name arm: a "comm-pid" label whose comm part equals a
//     bare-comm entity.
func traceCausalProjectionAnchorLabelMatchesEntity(label string, entity traceCausalProjectionAnchorEntity) bool {
	label, value := strings.TrimSpace(label), strings.TrimSpace(entity.value)
	if label == "" || value == "" {
		return false
	}
	labelPid, labelHasPid := traceCausalProjectionNamePidTail(label)
	if !labelHasPid {
		labelPid, labelHasPid = traceCausalProjectionPidPeerForm(label)
	}
	// F2/F3: the "-<digits>" tail and bare-integer arms are TYPED-lane only.
	// The explicit "pid=N" handle is unambiguous and stays open to prose.
	entityPid, entityHasPid := traceCausalProjectionPidPeerForm(value)
	if !entityHasPid && entity.typedLane {
		if entityPid, entityHasPid = traceCausalProjectionNamePidTail(value); !entityHasPid {
			entityPid, entityHasPid = traceCausalProjectionPureInt(value)
		}
	}
	if labelHasPid && entityHasPid {
		return labelPid == entityPid
	}
	if traceCausalProjectionCanonicalNode(label) == traceCausalProjectionCanonicalNode(value) {
		return true
	}
	if labelHasPid {
		if idx := strings.LastIndex(label, "-"); idx > 0 &&
			traceCausalProjectionCanonicalNode(label[:idx]) == traceCausalProjectionCanonicalNode(value) {
			return true
		}
	}
	return false
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

// traceCausalProjectionSupplyFoldDonor is one collected SFD fold-basis donor:
// the VS-2 typed accounting of ONE causal-impact projection record, keyed by
// the segment identity below. Conflict marks a key whose donors disagree on
// any accounting value — such a key never joins (fail-open, 裸值保留).
type traceCausalProjectionSupplyFoldDonor struct {
	deficitMS, idealMS, knownMS, unknownMS float64
	// capabilitySource / referenceClass (CAP §26 C3 + 复核 F1) ride the copied
	// accounting group — the twin must disclose the SAME caliber and basis
	// cluster its donor's numbers were priced at. topologySource +
	// thermalCapKHz (CAP-2 §28.4 / THERM §28.5-T7) travel the same way: the
	// twin's wording must name the same cluster-structure source and the same
	// in-window press its donor's numbers were computed under.
	capabilitySource       string
	referenceClass         string
	topologySource         string
	thermalCapKHz          int
	windowStart, windowEnd float64
	windowDeclared         bool
	conflict               bool
}

// traceCausalProjectionSupplyFoldTwinKey is the SFD (§15.A display half, user
// q6 issue 1, 2026-07-07) same-segment identity: canonical subject + the exact
// evidence line range. The engine publishes ONE WakeupCausalImpact as two
// projection records (the causal-impact row carrying the typed fold_basis
// accounting, and the root-cause running twin via the rootCauseItem
// source="wakeup_chain" funnel that never sets SupplyFoldBasis — basis=nil by
// construction, query.go:9640); both carry the engine's OWN LineStart/LineEnd
// verbatim, so subject+range equality is a PRECISE engine-published signal of
// the shared underlying segment (q6 实证: both rows :45689-79142), never a
// similarity heuristic. Empty when the node lacks a valid line span or a
// resolvable real subject — such rows never join (the unknown-thread sentinel
// is not an identity; mirrors the R1 same-fact key's validity arm).
func traceCausalProjectionSupplyFoldTwinKey(node TraceCausalProjectionNode) string {
	if node.LineStart <= 0 || node.LineEnd < node.LineStart {
		return ""
	}
	if !traceCausalProjectionKnownSubject(node.Subject) {
		return ""
	}
	return traceCausalProjectionCanonicalNode(node.Subject) +
		"\x00" + strconv.Itoa(node.LineStart) + "\x00" + strconv.Itoa(node.LineEnd)
}

// traceCausalProjectionJoinSupplyFoldTwins re-uses the ALREADY-PUBLISHED
// supply-fold accounting across same-segment twin projections (SFD, §15.A
// display half): a running-state node whose funnel never published a
// fold_basis note (SupplyFoldComputed=false → the tree row fell to the bare
// 有效归因 branch while the SAME segment's causal-impact sibling carried the
// full 供给折算 accounting) takes its twin's typed SupplyFold* group, so the
// §7.10 decision table renders on the running row too and the two rows'
// deficit numbers are same-source by construction. No new data is minted —
// the copied values are the engine's own fold_basis / supply_fold_* notes.
//
// Precision rules (硬边界, fail-open to the bare value on every miss):
//   - join key = canonical subject + exact line range (see the key above);
//     different range or different subject never joins;
//   - recipients are running-STATE rows only (typed StateKind check — the
//     fold's deficit is defined over the segment's OWN running wall clock,
//     §7.10) that are NOT ×N aggregates (MergedCount>1 sums/envelopes many
//     segments; a single segment's fold does not describe the sum);
//   - donors are non-aggregate rows whose fold_basis actually published;
//     donors that DISAGREE on any accounting value under one key mark the
//     key conflicted and it never joins;
//   - a donor/recipient pair that BOTH declare their own typed
//     selected_window and disagree beyond the F-2 per-endpoint tolerance is
//     a cross-window re-measurement (§11-N2) — the fold describes the
//     donor's window's clamping, so the pair never joins.
//
// Chain-side buckets only (PrimaryRootCauses / OnChainCauses /
// SupportingHops): §7.10 is an on-chain lane, and every cross-bucket copy of
// one record patches from the SAME donor map so surfaces cannot disagree.
// Display-only field plumbing: ranking and gates never read SupplyFold*.
func traceCausalProjectionJoinSupplyFoldTwins(projection *TraceCausalProjection) {
	if projection == nil {
		return
	}
	buckets := [][]TraceCausalProjectionNode{
		projection.PrimaryRootCauses,
		projection.OnChainCauses,
		projection.SupportingHops,
	}
	donors := map[string]*traceCausalProjectionSupplyFoldDonor{}
	for _, bucket := range buckets {
		for i := range bucket {
			node := bucket[i]
			if !node.SupplyFoldComputed || node.MergedCount > 1 {
				continue
			}
			key := traceCausalProjectionSupplyFoldTwinKey(node)
			if key == "" {
				continue
			}
			donor := traceCausalProjectionSupplyFoldDonor{
				deficitMS: node.SupplyFoldDeficitMS, idealMS: node.SupplyFoldIdealMS,
				knownMS: node.SupplyFoldKnownMS, unknownMS: node.SupplyFoldUnknownMS,
				capabilitySource: node.SupplyFoldCapabilitySource,
				referenceClass:   node.SupplyFoldReferenceClass,
				topologySource:   node.SupplyFoldTopologySource,
				thermalCapKHz:    node.ThermalCapKHz,
				windowStart:      node.QueryWindowStartTs,
				windowEnd:        node.QueryWindowEndTs,
				windowDeclared:   node.QueryWindowStartTs > 0 && node.QueryWindowEndTs > node.QueryWindowStartTs,
			}
			if existing, seen := donors[key]; seen {
				// The same record's cross-bucket copy carries identical values
				// and stays a non-conflict; a DIFFERENT accounting under the
				// same key is ambiguity — never joined.
				//
				// SFD 复核 F5: same-ACCOUNT donors from different windows keep
				// the FIRST donor's window for the veto below — deterministic
				// (the bucket scan order primary → on-chain → hops is fixed
				// and stable-sorted upstream), and semantically safe: equal
				// accounting means the re-measurements agree on the fold, so
				// whichever window anchors the ±1ms comparison, a recipient
				// matching ANY of them is joining that same published
				// accounting; a recipient matching NONE is vetoed either way.
				if existing.deficitMS != donor.deficitMS || existing.idealMS != donor.idealMS ||
					existing.knownMS != donor.knownMS || existing.unknownMS != donor.unknownMS {
					existing.conflict = true
				}
				continue
			}
			donors[key] = &donor
		}
	}
	if len(donors) == 0 {
		return
	}
	for _, bucket := range buckets {
		for i := range bucket {
			node := &bucket[i]
			if node.SupplyFoldComputed || node.MergedCount > 1 {
				continue
			}
			if strings.TrimSpace(strings.ToLower(node.StateKind)) != "running" {
				continue
			}
			key := traceCausalProjectionSupplyFoldTwinKey(*node)
			if key == "" {
				continue
			}
			donor := donors[key]
			if donor == nil || donor.conflict {
				continue
			}
			if donor.windowDeclared && node.QueryWindowStartTs > 0 && node.QueryWindowEndTs > node.QueryWindowStartTs &&
				(math.Abs(node.QueryWindowStartTs-donor.windowStart) > traceCausalProjectionFullWindowSameWindowToleranceS ||
					math.Abs(node.QueryWindowEndTs-donor.windowEnd) > traceCausalProjectionFullWindowSameWindowToleranceS) {
				continue
			}
			node.SupplyFoldComputed = true
			node.SupplyFoldDeficitMS = donor.deficitMS
			node.SupplyFoldIdealMS = donor.idealMS
			node.SupplyFoldKnownMS = donor.knownMS
			node.SupplyFoldUnknownMS = donor.unknownMS
			node.SupplyFoldCapabilitySource = donor.capabilitySource
			node.SupplyFoldReferenceClass = donor.referenceClass
			node.SupplyFoldTopologySource = donor.topologySource
			node.ThermalCapKHz = donor.thermalCapKHz
		}
	}
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

// TraceCausalProjectionSameWindowToleranceS is the exported single authority
// for the ±1ms same-window endpoint tolerance (复核 Low, 2026-07-06): display
// consumers in internal/tool (metric-snapshot window grouping) MUST use this
// constant instead of re-minting the literal.
const TraceCausalProjectionSameWindowToleranceS = traceCausalProjectionFullWindowSameWindowToleranceS

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

// traceCausalProjectionAppendQueryWindow appends one selected_window pair to
// the PTV5 Q3 display list unless an existing entry matches both endpoints
// within the F-2 ±1ms tolerance (same constant as the RN-12 same-window
// verdict — one tolerance, two consumers). The list caps at 8 distinct
// windows: display sanity only, the count is the load-bearing fact and stays
// exact up to the cap. The second return is true when a DISTINCT window was
// DROPPED by the cap (复核 Low, 2026-07-06) — the caller latches it into
// QueryWindowsTruncated so display counts become lower bounds, never fake
// exact numbers.
func traceCausalProjectionAppendQueryWindow(windows []TraceCausalProjectionQueryWindow, start, end float64) ([]TraceCausalProjectionQueryWindow, bool) {
	if start <= 0 || end <= start {
		return windows, false
	}
	for _, w := range windows {
		if math.Abs(w.StartTs-start) <= traceCausalProjectionFullWindowSameWindowToleranceS &&
			math.Abs(w.EndTs-end) <= traceCausalProjectionFullWindowSameWindowToleranceS {
			return windows, false
		}
	}
	if len(windows) >= 8 {
		return windows, true
	}
	return append(windows, TraceCausalProjectionQueryWindow{StartTs: start, EndTs: end}), false
}

// traceCausalProjectionSortQueryWindows orders the display list by ascending
// start (then end) — deterministic on record order.
func traceCausalProjectionSortQueryWindows(windows []TraceCausalProjectionQueryWindow) []TraceCausalProjectionQueryWindow {
	sort.SliceStable(windows, func(i, j int) bool {
		if windows[i].StartTs != windows[j].StartTs {
			return windows[i].StartTs < windows[j].StartTs
		}
		return windows[i].EndTs < windows[j].EndTs
	})
	return windows
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

// TraceCausalProjectionParseWindowValue exposes the strict "start..end"
// window-value parser (the SAME ParseFloat/end>start validation lane as the
// selected_window note) for display consumers of OTHER window-valued notes
// (PTV6-C 修正轮: the actual_window inline endpoints).
func TraceCausalProjectionParseWindowValue(raw string) (float64, float64, bool) {
	return traceCausalProjectionParseSelectedWindow(raw)
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

// traceCausalProjectionFoldAbsorbedChainLaneRows is the G1 跨车道对账 display
// half (§27.2-G1, user ruling 收口批准 §28.1, 2026-07-09,
// real_trace_campaign_20260705.md): critical_blocking nodes the ENGINE marked
// absorbed (absorbed_by_rank_family=true + absorbed_into=<key>) relocate out
// of the render buckets into AbsorbedChainRows when — and only when — a rank
// FAMILY node carrying the VERBATIM same rank_family_key sits in this
// projection's buckets. One choke point before aggregation, so:
//   - the tree / ◇▒ stanzas / metric table / comparison cells all stop
//     seating the duplicate rows without per-face suppression code;
//   - R1/R2/V4 aggregation never builds a mixed absorbed+non-absorbed ×N
//     chimera (the absorbed rows are gone before grouping);
//   - the nodes stay LOSSLESS: the display layer registers each on the
//     evidence index (E# preserved) and the family stanza prints the
//     链上并入 disclosure — 观测照发不删, the underlying ObservationRecords
//     are untouched.
//
// 负向保护 (fail-open): an absorbed marker whose key matches NO present
// family node leaves the node in its bucket verbatim — an honest duplicate
// render always beats a silent drop. Join signals are PRECISE only: the
// engine-rendered key string compared verbatim on both sides (one renderer,
// rankFamilyReconKey) — never label/value similarity.
func traceCausalProjectionFoldAbsorbedChainLaneRows(out *TraceCausalProjection) {
	if out == nil {
		return
	}
	familyKeys := map[string]bool{}
	collect := func(nodes []TraceCausalProjectionNode) {
		for _, node := range nodes {
			// FamilyMemberCount>1 is defense in depth: the engine stamps
			// rank_family_key exclusively on family-merged rows.
			if key := strings.TrimSpace(node.RankFamilyKey); key != "" && node.FamilyMemberCount > 1 {
				familyKeys[key] = true
			}
		}
	}
	collect(out.PrimaryRootCauses)
	collect(out.OnChainCauses)
	collect(out.AdjacentCauses)
	collect(out.BackgroundCauses)
	collect(out.SupportingHops)
	if len(familyKeys) == 0 {
		return
	}
	absorbedKey := func(node TraceCausalProjectionNode) string {
		if id := strings.TrimSpace(node.EvidenceID); id != "" {
			return id
		}
		return strings.Join([]string{
			traceCausalProjectionCanonicalNode(node.Subject),
			traceCausalProjectionCanonicalNode(node.Predicate),
			traceCausalProjectionCanonicalNode(node.Object),
		}, "\x00")
	}
	seen := map[string]bool{}
	filter := func(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
		kept := make([]TraceCausalProjectionNode, 0, len(nodes))
		for _, node := range nodes {
			if node.AbsorbedByRankFamily && familyKeys[strings.TrimSpace(node.AbsorbedInto)] {
				// One record can bucket twice (hop copy + classified copy) —
				// AbsorbedChainRows carries it once, deduped by identity.
				if key := absorbedKey(node); !seen[key] {
					seen[key] = true
					out.AbsorbedChainRows = append(out.AbsorbedChainRows, node)
				}
				continue
			}
			kept = append(kept, node)
		}
		return kept
	}
	out.PrimaryRootCauses = filter(out.PrimaryRootCauses)
	out.OnChainCauses = filter(out.OnChainCauses)
	out.AdjacentCauses = filter(out.AdjacentCauses)
	out.BackgroundCauses = filter(out.BackgroundCauses)
	out.SupportingHops = filter(out.SupportingHops)
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

// traceCausalProjectionIsCausalHop is the causal-hop FAMILY membership check
// (three predicate tokens + the root_evidence: claim-key family). Family
// membership alone routes a record into the classified lane; the SupportingHops
// lane additionally requires the PTV6 #1a on-chain gate below.
func traceCausalProjectionIsCausalHop(record ObservationRecord) bool {
	switch strings.TrimSpace(record.Predicate) {
	case "wakeup_causal_impact", "wakeup_causal_aggregate", "critical_blocking":
		return true
	default:
		return strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_evidence:")
	}
}

// traceCausalProjectionHopOnChain is the PTV6 #1a on-chain admission gate for
// the SupportingHops lane (real-trace campaign 2026-07-06): a hop-family record
// enters the hops surface only when it is on-chain by a typed signal —
//   - chain_relevance=on_chain, or causality=on_wakeup_chain /
//     on_dependency_chain, both resolved through the ONE strict parser
//     traceCausalProjectionChainRelevance (same lane the classified copy's
//     bucket selection reads — the two seats can never disagree);
//   - the root_evidence: audit family (no relevance note by design;
//     SupportingHops is its only surface);
//   - relevance UNSTATED: only the wakeup-CHAIN-view families
//     (wakeup_causal_impact / wakeup_causal_aggregate) pass — the producer
//     emits them exclusively under result.WakeupChain, so chain membership is
//     by construction (the real aggregate records carry no relevance note; a
//     strict note-only gate would vanish them from a healthy tree). The
//     window-stats micro-probe family (critical_blocking) must STATE on-chain
//     membership or stay off the hops surface.
//
// A typed OFF-chain lane (adjacent/background) always rejects — that is the
// specimen bug: critical_blocking chain_relevance=background double-cast into
// SupportingHops and rendered as └─唤醒─ phantom children of the 🎯 target.
// Mutation pin (TestTraceCausalProjectionHopAdmissionRequiresOnChainSignal):
// reverting to predicate-only admission must red.
func traceCausalProjectionHopOnChain(record ObservationRecord) bool {
	if strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_evidence:") {
		return true
	}
	switch traceCausalProjectionChainRelevance(record.RichNotes) {
	case "on_chain":
		return true
	case "adjacent", "background":
		return false
	}
	switch strings.TrimSpace(record.Predicate) {
	case "wakeup_causal_impact", "wakeup_causal_aggregate":
		return true
	}
	return false
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
		ChainBranch:     traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyChainBranch),
		ImpactMS:        traceCausalProjectionImpact(record),
		SpanName:        traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySpanName),
		SpanKind:        traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySpanKind),
		SpanCategory:    traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySpanCategory),
		SpanSubcategory: traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySpanSubcategory),
		SemanticClass:   traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySemanticClass),
		Confidence:      record.Confidence,
	}
	// G2 显示半场 (§27.2/§28.1, 2026-07-09): the typed blind-spot criterion —
	// wording input for the ◇ inline disclosure fork; absent = legacy wording.
	node.TraceGapKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyTraceGapKind))
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
	// §11-N2: carry the record's own typed selected_window as the node's QUERY
	// window identity (single strict parser; anchor gates untouched) — the R2
	// ×N merge needs it to tell cross-window re-measurements of one physical
	// segment apart from genuinely distinct same-window facts.
	if ws, we, ok := traceCausalProjectionSelectedWindowNote(record.RichNotes); ok {
		node.QueryWindowStartTs, node.QueryWindowEndTs = ws, we
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
	// DIAG A2 (§28.11-3(b) D-10, 2026-07-09): the thread-level actual total
	// (SECOND actual caliber) plus the producer's typed divergence disclosure —
	// wording input for the 实际口径 detail line only, never a value edit.
	node.ActualTotalMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyActualTotalMS, TraceNoteKeyActualTotal)
	node.ActualCaliberNote = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyActualCaliberNote))
	// COV §24.9 D-1: the TargetBlockedMs typed promotion (rank lane emits
	// target_impact_ms=%.3f; the causal_impact lanes carry target_impact=%.3fms
	// verbatim — the shared float parser strips the unit).
	node.TargetImpactMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyTargetImpactMS, TraceNoteKeyTargetImpact)
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
		// BLOCKFROM (Wave-3.2, 2026-07-09): the waiter-side blocking call site
		// (等待点) rides the same lock-row gate as the holder site above — a
		// blocking-from without contention semantics is meaningless.
		node.BlockingFromSite = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockingFromSite))
		node.BlockingWaiters = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyWaiters)
		// BLK §15.C: the resolved rank lock row's subject is the holder — the
		// renderer reads a HOLD (not the reversed lock-wait) from this exact
		// typed note.
		node.BlockingSubjectIsHolder = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySubjectIsLockHolder)) == "true"
		// P0-E 锁车道修2/修3: holder-resolution origin + phantom tid + the
		// hand-off / self-contradiction witnesses (disclosure faces).
		node.BlockingHolderSource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHolderSource))
		node.BlockingOwnerTidRaw = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyOwnerTidRaw)
		node.BlockingHolderHandoff = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHolderHandoff))
		node.BlockingHolderContradiction = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHolderSelfContradiction))
	}
	// §7.30.3 D3: gated-impact composition for priority-inversion rows.
	node.GatedRunnableMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyGatedRunnable)
	node.GatedRunningDeficitMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyGatedRunningDeficit)
	// CAP (§26 C3): typed capability caliber of the discounted running
	// component — exact typed note match, wording input only. CAP-2: the
	// cluster-topology source rides beside it.
	node.GatedCapabilitySource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCapability))
	node.GatedTopologySource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedClusterTopology))
	// PTV5 Q4 (#68 用户裁定 2026-07-05): inversion candidacy is a typed field —
	// exact "true" match on the producer's note; the legacy Object-token lane
	// stays alive in the display predicate for root_cause rows whose Object
	// carries the type token.
	node.PriorityInversionCandidate = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyPriorityInversionCandidate)) == "true"
	// SYM-2 (§24.17 R2, 2026-07-08): the below-RT preemption disclosure on a
	// self runnable rank row — exact typed note match, wording input only.
	node.RunnableBelowRTPreempted = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyRunnableBelowRTPreempted)) == "true"
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
		// CAP (§26 C3): the fold's typed capability caliber rides the same
		// presence gate (a capability claim without a fold is meaningless).
		node.SupplyFoldCapabilitySource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldCapability))
		// CAP 复核 F1: the demoted basis class (absent = big-class basis).
		node.SupplyFoldReferenceClass = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldReferenceClass))
		// CAP-2 (§28.4/§28.5): cluster-structure source (absent = explicit/
		// legacy — the default-table wording stands byte-identically) and the
		// THERM in-window press disclosure.
		node.SupplyFoldTopologySource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldClusterTopology))
		node.ThermalCapKHz = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyThermalCapKHz)
	}
	node.RunnableMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyRunnable)
	// Verbatim typed kind token (see TypeToken doc): lets renderers specialize
	// the unresolved-peer wording for blocking_span / d_state_or_io_wait rows.
	node.TypeToken = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyType))
	// PTS (#68 用户裁定 2026-07-05): a producer-side fold record (rows beyond
	// the per-family wire cap folded with a count — zero silent drops) carries
	// its fold accounting in the typed folded_* note family; the node
	// re-materializes the fold so the tree renders 其余 N 项(链上折叠) with
	// the member range and roster.
	if folded := traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyFoldedRows); folded > 0 {
		node.MergedCount = folded
		node.MergedMinMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyFoldedMinMS)
		node.MergedMaxMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyFoldedMaxMS)
		for _, subject := range strings.Split(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldedSubjects), ",") {
			traceCausalProjectionAppendMergedSubject(&node, subject)
		}
		node.OnChainOverflowFold = node.ChainRelevance == "on_chain"
		// DIAG A1 (§28.11-3(a)): re-materialize the producer's µs-tie member
		// roster ("<subject>@<start>-<end>" comma-joined). Malformed entries
		// are dropped (absence never guesses); the fold values above are
		// untouched.
		node.SameValueMembers = traceCausalProjectionParseSameValueMembers(
			traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySameValueMembers))
		// G12-ENG 复核 P3-4 (2026-07-09): this wire re-materialization carries
		// NO MergedValuelessCount channel — deliberately. Both producers of
		// folded_* notes fold ONLY positive-display members by construction:
		// the wire-cap impact fold folds WakeupCausalImpact rows admitted via
		// the expandChain `impact.TotalMs > 0` entry guard (a positive lane
		// total forces a positive dominant), and the engine aggregate-trim
		// fold (foldWakeupCausalAggregateOverflow) folds aggregates built from
		// those same admitted impacts. A zero-display member is therefore
		// structurally unreachable here; if a future producer folds marker
		// rows onto this lane, port the valueless accounting (a folded_
		// valueless note) alongside — the display arms already fork on the
		// node field.
	}
	// RCM 家族合并族 (§24.7.1/§24.10, 2026-07-08): the engine same-thread
	// family-merge accounting re-materializes on the ISOLATED FamilyMember*
	// lane — NEVER on MergedCount/MergedMaxMS (§24.12 dim-A mandate ①: the
	// display lead selector folds Merged* rows to their member MAX, which
	// would collapse the same-thread family total back to its largest member).
	// member_roster entries are joined with " | " on the wire because member
	// keys/span names may legally contain commas.
	// RCM-2 (§24.10 display half, 2026-07-08): the background comprehensive
	// board seat — an already-emitted typed note (DCS §23.1) the display 行2
	// consumes as 背景榜位#N. Wording input only.
	node.BackgroundRank = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyBackgroundRank)
	if familyCount := traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyMemberCount); familyCount > 1 {
		node.FamilyMemberCount = familyCount
		node.FamilyMemberMaxMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyMemberMaxMS)
		node.FamilyMemberMinMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyMemberMinMS)
		node.FamilyMemberSumMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyMemberSumMS)
		node.FamilyFoldCaliber = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyMemberFoldCaliber))
		for _, entry := range strings.Split(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyMemberRoster), " | ") {
			if entry = strings.TrimSpace(entry); entry != "" {
				node.FamilyMemberRoster = append(node.FamilyMemberRoster, entry)
			}
		}
	}
	// RCM 区分键族 (§24.9-B F3): typed inode/dev identity — never a Summary
	// re-parse.
	node.Inode = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyInode))
	node.Dev = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyDev))
	// G1 跨车道对账 (§27.2-G1, 2026-07-09): the engine's cross-lane absorption
	// markers — family-side identity on rank rows, absorbed-side verdict +
	// identity on critical_blocking rows. Verbatim strings; the compile fold
	// (traceCausalProjectionFoldAbsorbedChainLaneRows) joins the two sides by
	// exact key equality.
	node.RankFamilyKey = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyRankFamilyKey))
	node.AbsorbedByRankFamily = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyAbsorbedByRankFamily)) == "true"
	node.AbsorbedInto = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyAbsorbedInto))
	return node
}

// traceCausalProjectionParseSameValueMembers parses the typed
// same_value_members note value ("<subject>@<line_start>-<line_end>" entries,
// comma-joined — single producer format, see the fold-record builders in
// internal/tool/trace_query.go). The subject is everything before the LAST
// '@' (labels never carry commas on this lane — folded_subjects convention);
// entries whose range half does not parse are dropped, never guessed. Cap 4,
// mirroring the fold-side traceCausalProjectionSameValueMemberCap.
func traceCausalProjectionParseSameValueMembers(raw string) []TraceCausalProjectionSameValueMember {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []TraceCausalProjectionSameValueMember
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		at := strings.LastIndex(entry, "@")
		if at <= 0 || at == len(entry)-1 {
			continue
		}
		subject := strings.TrimSpace(entry[:at])
		rangePart := entry[at+1:]
		dash := strings.IndexByte(rangePart, '-')
		if dash < 0 {
			continue
		}
		start, errStart := strconv.Atoi(strings.TrimSpace(rangePart[:dash]))
		end, errEnd := strconv.Atoi(strings.TrimSpace(rangePart[dash+1:]))
		if errStart != nil || errEnd != nil || subject == "" {
			continue
		}
		if len(out) < traceCausalProjectionSameValueMemberCap {
			out = append(out, TraceCausalProjectionSameValueMember{Subject: subject, LineStart: start, LineEnd: end})
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
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

// traceCausalProjectionLimitNodesOnChainFold is the PTS zero-silent-drop
// bucket limiter (#68 用户裁定 2026-07-05: 凡 on-chain 项必须提及+进树,多则
// 折叠+计数): rows beyond the on-chain bucket cap fold into ONE counted
// subjectless row — member MAX value (wall clock never sums across threads),
// min–max range, up-to-4 subject roster and every member evidence ID kept —
// instead of the former silent nodes[:limit] discard. ≤limit inputs return
// byte-identical to the plain limiter. Runs AFTER the presentation
// aggregation (复核 P1, 2026-07-06), so the count is the post-merge truth.
//
// 口径源 (复核 P1 ②): the fold's published value carries the CALIBER of the
// member that supplied the MAX — a window-projection max lands on ImpactMS
// (consumers may publish a window share), while a cumulative-only max lands
// on CumulativeImpactMS ONLY (ImpactMS stays 0 → the C00 display pipeline
// prints the 链上累计 caliber word and suppresses the %, and the (a) table's
// window-projection column honestly shows "—").
//
// G2 显示半场 双发布去重 (§27.2, 2026-07-09): overflow members that are typed
// data-blind-spot rows AND whose subject already holds an individual ◇/▒
// stanza seat (seatedDataGapSubjects — computed by the compile from the
// post-aggregation adjacent/background buckets) leave the fold membership, and
// the fold count honestly shrinks with them (计数如实减除). Information
// argument: a blind-spot fold member contributes only "exists, value 0.000" to
// the ×N counter, while its individual stanza row carries the full identity
// (thread, kind wording, evidence, disclosure) — dropping the fold copy loses
// nothing the seat does not state better. Members WITHOUT a seat keep folding
// (PTS 永不静默丢).
func traceCausalProjectionLimitNodesOnChainFold(nodes []TraceCausalProjectionNode, limit int, seatedDataGapSubjects map[string]bool) []TraceCausalProjectionNode {
	if limit <= 0 || len(nodes) == 0 || len(nodes) <= limit {
		return traceCausalProjectionLimitNodes(nodes, limit)
	}
	kept := append([]TraceCausalProjectionNode(nil), nodes[:limit]...)
	var overflow []TraceCausalProjectionNode
	for _, member := range nodes[limit:] {
		if traceCausalProjectionDataGapRow(member) &&
			seatedDataGapSubjects[traceCausalProjectionCanonicalNode(member.Subject)] {
			continue // G2: the individual stanza seat is the single publication
		}
		overflow = append(overflow, member)
	}
	if len(overflow) == 0 {
		return kept
	}
	return append(kept, traceCausalProjectionOverflowFoldRow(overflow))
}

// traceCausalProjectionSeatedDataGapSubjects collects the canonical subjects
// of typed data-blind-spot rows holding an individual adjacent/background
// stanza seat (G2 双发布去重 exclusivity key). nil when none — the fold then
// behaves byte-identically to the pre-G2 shape.
func traceCausalProjectionSeatedDataGapSubjects(buckets ...[]TraceCausalProjectionNode) map[string]bool {
	var out map[string]bool
	for _, bucket := range buckets {
		for _, node := range bucket {
			if !traceCausalProjectionDataGapRow(node) {
				continue
			}
			subject := traceCausalProjectionCanonicalNode(node.Subject)
			if subject == "" || !traceCausalProjectionKnownSubject(subject) {
				continue
			}
			if out == nil {
				out = map[string]bool{}
			}
			out[subject] = true
		}
	}
	return out
}

// traceCausalProjectionLimitHopsFold is the PTV6 (PTS 连带) SupportingHops
// limiter: same zero-silent-drop fold pipeline as the on-chain bucket, plus a
// cross-bucket overlap carve-out. Post-#1a every hop is on-chain, so an
// overflow hop whose evidence id already lives on the (still uncapped)
// on-chain bucket is the DELIBERATE bucket overlap — the renderer's node-key
// dedupe keeps exactly one copy either way — and folding it here would mint a
// fake "其余 N 项" made of rows that render anyway (复核 P1 的 donghu 假 16
// 项教训). Only overflow hops represented nowhere else (root_evidence: family
// rows carry no relevance note and never enter a relevance bucket) fold into
// the counted row. ≤limit inputs return byte-identical to the plain limiter.
func traceCausalProjectionLimitHopsFold(hops, onChain []TraceCausalProjectionNode, limit int) []TraceCausalProjectionNode {
	if limit <= 0 || len(hops) == 0 || len(hops) <= limit {
		return traceCausalProjectionLimitNodes(hops, limit)
	}
	represented := map[string]bool{}
	record := func(raw string) {
		if id := traceCausalProjectionCanonicalNode(raw); id != "" {
			represented[id] = true
		}
	}
	for _, node := range onChain {
		record(node.EvidenceID)
		for _, id := range node.MergedEvidenceIDs {
			record(id)
		}
	}
	kept := append([]TraceCausalProjectionNode(nil), hops[:limit]...)
	var overflow []TraceCausalProjectionNode
	for _, member := range hops[limit:] {
		if id := traceCausalProjectionCanonicalNode(member.EvidenceID); id != "" && represented[id] {
			continue // cross-bucket overlap: already represented on the on-chain surface
		}
		overflow = append(overflow, member)
	}
	if len(overflow) == 0 {
		return kept
	}
	return append(kept, traceCausalProjectionOverflowFoldRow(overflow))
}

// traceCausalProjectionOverflowFoldRow builds the counted subjectless fold row
// from an overflow member list — the ONE fold-row constructor both the
// on-chain bucket cap and the PTV6 hop cap consume (member MAX value, min–max
// range, roster, every member evidence id absorbed).
//
// TargetImpactMS is deliberately DROPPED on this fold (COV 复核建议,
// 2026-07-08): the overflow row is a counted cross-thread roster, not a
// coverage-numerator carrier — a zero caliber makes consumers fall back to the
// cumulative/display channel (conservative lower bound, never fabricates), and
// no surface consumes a fold-row target caliber today.
func traceCausalProjectionOverflowFoldRow(overflow []TraceCausalProjectionNode) TraceCausalProjectionNode {
	fold := TraceCausalProjectionNode{
		Role:                overflow[0].Role,
		Predicate:           overflow[0].Predicate,
		ChainRelevance:      "on_chain",
		Causality:           overflow[0].Causality,
		OnChainOverflowFold: true,
	}
	var minMS, maxMS float64
	maxFromWindowProjection := false
	// G19 (§27.5, 2026-07-09): typed all-members-are-data-gaps accounting for
	// the all-zero fold note's honest "(数据盲区)" qualifier.
	allDataGap := true
	absorbed := map[string]bool{}
	appendID := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
			return
		}
		absorbed[traceCausalProjectionCanonicalNode(raw)] = true
		if fold.EvidenceID == "" {
			fold.EvidenceID = raw
			return
		}
		fold.MergedEvidenceIDs = append(fold.MergedEvidenceIDs, raw)
	}
	memberRows := 0
	valuelessRows := 0
	for _, member := range overflow {
		if !traceCausalProjectionDataGapRow(member) {
			allDataGap = false
		}
		traceCausalProjectionAppendMergedSubject(&fold, member.Subject)
		// PTS-2 F1 (复核裁定 2026-07-06, 计数吸收 — 不做桶位豁免): an absorbed
		// member that is ITSELF an overflow fold row (engine aggregate fold /
		// wire-cap fold — the typed OnChainOverflowFold marker) represents
		// MergedCount already-folded rows, not one. The bucket fold absorbs
		// that count into its own N and merges the member's roster (global
		// roster cap still applies), so the rendered 其余 N 项 stays the true
		// row count. Ordinary ×N presentation-aggregate rows still count 1 —
		// their ×N stays inside their absorbed evidence IDs, unchanged.
		if member.OnChainOverflowFold && member.MergedCount > 0 {
			memberRows += member.MergedCount
			for _, subject := range member.MergedSubjects {
				traceCausalProjectionAppendMergedSubject(&fold, subject)
			}
		} else {
			memberRows++
		}
		display := member.ImpactMS
		fromWindowProjection := member.ImpactMS > 0
		if display <= 0 {
			display = member.CumulativeImpactMS
		}
		// G12-ENG (§29.1): a non-positive display member has ALWAYS been
		// invisible to the min–max range below — count it, so the render can
		// stop claiming "各 min–max ms" over members that never carried the
		// value (the E23 fabricated ×2 same-value form). An absorbed fold
		// member contributes its own valueless-row count (F1 计数吸收 twin).
		if member.OnChainOverflowFold && member.MergedCount > 0 {
			valuelessRows += member.MergedValuelessCount
		} else if display <= 0 {
			valuelessRows++
		}
		if minMS == 0 || (display > 0 && display < minMS) {
			minMS = display
		}
		if display > maxMS {
			maxMS = display
			maxFromWindowProjection = fromWindowProjection
		}
		appendID(member.EvidenceID)
		for _, id := range member.MergedEvidenceIDs {
			appendID(id)
		}
		if member.LineStart > 0 && (fold.LineStart <= 0 || member.LineStart < fold.LineStart) {
			fold.LineStart = member.LineStart
		}
		if member.LineEnd > fold.LineEnd {
			fold.LineEnd = member.LineEnd
		}
		if member.Confidence > 0 && (fold.Confidence <= 0 || member.Confidence < fold.Confidence) {
			fold.Confidence = member.Confidence
		}
	}
	// MergedCount counts folded ROWS (a member's own ×N stays inside its
	// absorbed evidence IDs; an absorbed FOLD member contributes its own
	// MergedCount — F1 计数吸收); the value is the member MAX, never a sum.
	fold.MergedCount = memberRows
	fold.MergedMinMS = minMS
	fold.MergedMaxMS = maxMS
	fold.MergedValuelessCount = valuelessRows
	fold.MergedAllDataGap = allDataGap
	// DISP-3 (§29.8 P3 "E19 跨窗折叠漏拒%", real_trace_campaign_20260705.md,
	// 2026-07-09): the fold's member query-window roster rides the row through
	// the SAME F-2 slot builder the R2 merge uses (one tolerance, no second
	// implementation). huadong_792 E19: an 11-member on-chain overflow fold
	// whose members straddle two query windows published its member MAX over
	// the single anchor window as "24%" — the §21.1 CWD-2 ① %-suppression gate
	// (runtimeTraceProjMultiWindowMergedRow) keys on MergedCount>1 ∧ >1 roster
	// windows and never saw this constructor's rows because the roster stayed
	// empty. Disclosure + display-gate input only; the fold's published
	// value/min/max/count are untouched, and single-window (or windowless)
	// folds keep their legacy % byte-identically (roster ≤1).
	memberIdx := make([]int, len(overflow))
	for i := range overflow {
		memberIdx[i] = i
	}
	roster := traceCausalProjectionCrossWindowUnion(overflow, memberIdx).roster
	for _, member := range overflow {
		// An absorbed member that is ITSELF a merged row carries its window
		// identities on its own roster (row-level pair already zeroed by the
		// §11-N2 merge) — fold them in so the fold's roster stays the KNOWN
		// member sources (never claimed exhaustive; same F-2 append helper,
		// same 8-slot display cap — the >1 threshold the %-gate reads is
		// unaffected by the cap).
		for _, w := range member.MergedQueryWindows {
			roster, _ = traceCausalProjectionAppendQueryWindow(roster, w.StartTs, w.EndTs)
		}
	}
	fold.MergedQueryWindows = traceCausalProjectionSortQueryWindows(roster)
	if maxFromWindowProjection {
		fold.ImpactMS = maxMS
		fold.CumulativeImpactMS = maxMS
	} else {
		fold.CumulativeImpactMS = maxMS
	}
	// DIAG A1 (§28.11-3(a), G12): the huadong_79 E23 shape — two cross-thread
	// members tying the published MAX to the µs — is disclosed as a typed
	// (subject, line-range) roster at THIS value-merge point. Zero weight: all
	// published values above are final before the roster is computed.
	fold.SameValueMembers = traceCausalProjectionSameValueFoldMembers(overflow, maxMS)
	return fold
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
