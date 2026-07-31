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
	// Off-chain semantic spans are bounded detail. Typed on-chain semantic
	// spans are causal candidates and are never truncated at projection compile.
	traceCausalProjectionSemanticOffChainLimit = 16
	traceCausalProjectionSupportingHopLimit    = 10
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
	// TargetStateAccount (§29.27② COV-4, 2026-07-11): the focused thread's
	// full-window state partition compiled from the bundle's typed
	// target_window_states record — running + runnable + sleep + D-state
	// (io_wait = the typed IO refinement inside the D-state wall clock) plus
	// the deterministic-running intersection (确定性工作, wall clock). The
	// attach admits ONLY the record whose typed selected_window matches the
	// resolved anchor window within the F-2 tolerance (禁猜 — a partition
	// that cannot prove its window makes no window claim). Display renders
	// the four-state coverage account ONLY when Σ(states) balances against
	// the analysis window (不平衡拒渲不造数); no gate reads this field.
	TargetStateAccount *TraceCausalProjectionTargetStateAccount `json:"target_state_account,omitempty"`
	// BusinessSpanMentions (SPANVIS-1, user ruling 2026-07-19 定形原则): the
	// pure-advisory business-lens span mention face — a projection-level SIDE
	// CHANNEL compiled from business_span_mention records (each parsed
	// all-or-nothing; a record failing any typed field is dropped whole). The
	// rows join NO node bucket, NO ordinal population, NO conservation or
	// census denominator (不参与根因排序); display consumers are the tree
	// fence 「◈ 业务span提示」 advisory block and the ◎ overview 旁栏
	// footnote. Order = producer order (engine reading order: total desc);
	// deduped by (subject, name, lines) so a re-published record set cannot
	// double the list.
	BusinessSpanMentions []TraceCausalProjectionBusinessSpanMention `json:"business_span_mentions,omitempty"`
	// BusinessSpanMentionOmitted is the engine's honest count of admitted
	// (≥significance floor) families beyond the mention cap (件3 截断诚实
	// 披露; micro families never count). First parsed value wins.
	BusinessSpanMentionOmitted int `json:"business_span_mention_omitted,omitempty"`
	// GatedCompositeEdgeShareDisclosures (PARTSPLIT-1, §29.150④ user ruling
	// 2026-07-19): the R4-mirror refusal NON-SEAT disclosure side channel —
	// compiled from gated_composite_edge_share records (each parsed
	// all-or-nothing; a record failing any typed field is dropped whole).
	// The rows join NO node bucket, NO ordinal population, NO conservation
	// or census denominator; the display consumer is the ◎ overview's
	// non-seat 边前份披露 mention row (no ordinal, never in a section
	// maximum, never additive to the seat's published value). Deduped by
	// (subject, boundary) so a re-published record set cannot double the
	// list.
	GatedCompositeEdgeShareDisclosures []TraceCausalProjectionGatedCompositeEdgeShareDisclosure `json:"gated_composite_edge_share_disclosures,omitempty"`
	// SelfRunnableTwoRulerAccountings (RULER2-1, §29.150② user ruling /
	// R-19-b, 2026-07-19): the self runnable two-ruler accounting side
	// channel — compiled from self_runnable_two_ruler records (each parsed
	// all-or-nothing; a record failing any typed field or either same-ruler
	// Σ identity is dropped whole). The records join NO node bucket, NO
	// ordinal population, NO conservation or census denominator; the display
	// consumer is the 行2 按两把尺记账 cross-row sentence stamped onto the
	// LEAD seat row (carriers absent → silent). Deduped by subject.
	SelfRunnableTwoRulerAccountings []TraceCausalProjectionSelfRunnableTwoRuler `json:"self_runnable_two_ruler_accountings,omitempty"`
	// SelfRunningFoldUnmeasured (SELFRUN-DISC, §29.192① (b) user ruling /
	// A2 件11(b) handoff §29.194, 2026-07-21): the self supply-fold
	// 「量不了」 absence disclosure side channel — compiled from
	// self_running_fold_unmeasured records (each parsed all-or-nothing; a
	// record failing any typed field or the running==unknown fold identity
	// is dropped whole). The records join NO node bucket, NO ordinal
	// population, NO conservation or census denominator; the display
	// consumer is the ◎ auxiliary 另账 row 「运行频点未采集,自身降频折算
	// 不可量」 (absence silent). Deduped by subject.
	SelfRunningFoldUnmeasured []TraceCausalProjectionSelfRunningFoldUnmeasured `json:"self_running_fold_unmeasured,omitempty"`
}

// TraceCausalProjectionGatedCompositeEdgeShareDisclosure is one R4-mirror-
// refused gated composite seat's pre-edge-share disclosure row (PARTSPLIT-1).
// All fields are typed verbatim transports of the engine's
// GatedCompositeEdgeShareDisclosure record — never re-derived, never
// re-scaled; the display re-validates PreMS + PostMS == AccountMS (µs)
// before rendering.
type TraceCausalProjectionGatedCompositeEdgeShareDisclosure struct {
	// Subject is the refused seat's thread label (record Subject, verbatim).
	Subject string `json:"subject"`
	// PreMS / PostMS / AccountMS: the X/Y bisection measures and the runnable
	// census account they partition (X+Y==Account, the pinned identity).
	PreMS     float64 `json:"pre_ms"`
	PostMS    float64 `json:"post_ms"`
	AccountMS float64 `json:"account_ms"`
	// AnchorTS / Via: WHICH credential edge bisected the account (closed-set
	// via vocabulary: direct / chain_hop / direct+chain_hop).
	AnchorTS float64 `json:"anchor_ts"`
	Via      string  `json:"via"`
	// SeatPublished: the refused seat itself survived the publication cap
	// (false = the seat lives only in the candidate pool — the ◎ mention's
	// off-board honesty clause input).
	SeatPublished bool `json:"seat_published"`
}

// TraceCausalProjectionSelfRunnableTwoRuler is the self runnable two-ruler
// accounting record (RULER2-1, §29.150② / R-19-b): the analysis target's own
// runnable seats split across the two closed rulers. All fields are typed
// verbatim transports of the engine's SelfRunnableTwoRulerAccounting record —
// never re-derived, never re-scaled; the display re-validates each
// same-ruler Σ identity (µs) before rendering, and NO cross-ruler total
// exists anywhere on the record or any face (M3 禁混尺).
type TraceCausalProjectionSelfRunnableTwoRuler struct {
	// Subject is the target thread label (record Subject, verbatim).
	Subject string `json:"subject"`
	// WallEffsMS/WallRanks — the self wall-clock ruler's seat values and
	// board ordinals (parallel, board order). EdgeEffsMS/EdgeRanks — the
	// wakeup-edge ruler's.
	WallEffsMS []float64 `json:"wall_effs_ms"`
	WallRanks  []int     `json:"wall_ranks"`
	EdgeEffsMS []float64 `json:"edge_effs_ms"`
	EdgeRanks  []int     `json:"edge_ranks"`
	// WallSubtotalMS / EdgeSubtotalMS — the same-ruler subtotals (Σ of that
	// ruler's values; the pinned µs identities).
	WallSubtotalMS float64 `json:"wall_subtotal_ms"`
	EdgeSubtotalMS float64 `json:"edge_subtotal_ms"`
}

// TraceCausalProjectionSelfRunningFoldUnmeasured is the self supply-fold
// 「量不了」 absence disclosure (SELFRUN-DISC, §29.192① (b)): the analysis
// target ran RunningMS inside the window while the fold basis was ENTIRELY
// unknown (no governed frequency coverage on any slice), so the self
// down-clock fold is unmeasurable — the zero deficit must never wear the
// affirmative "no loss" face. All fields are typed verbatim transports of
// the engine's SelfRunningFoldUnmeasuredDisclosure record — never
// re-derived, never re-scaled; the parser and the display both re-validate
// the fold identity RunningMS == UnknownMS (KnownMs==0 form) before any
// wording renders.
type TraceCausalProjectionSelfRunningFoldUnmeasured struct {
	// Subject is the analysis target's thread label (record Subject,
	// verbatim).
	Subject string `json:"subject"`
	// RunningMS / UnknownMS: the window-projected running wall clock and the
	// unknown-basis wall (equal by the KnownMs==0 fold identity).
	RunningMS float64 `json:"running_ms"`
	UnknownMS float64 `json:"unknown_ms"`
}

// TraceCausalProjectionBusinessSpanMention is one advisory business-span
// mention row (SPANVIS-1). All fields are typed verbatim transports of the
// engine's BusinessSpanMention family — never re-derived, never re-scaled.
type TraceCausalProjectionBusinessSpanMention struct {
	// Subject is the owning thread label (record Subject, verbatim).
	Subject string `json:"subject"`
	// Name is the verbatim span name (typed family key).
	Name string `json:"name"`
	// Count / TotalMS / MaxMS: admitted member count, Σ in-window member
	// durations, largest single member (双杠杆线索 typed fact trio).
	Count   int     `json:"count"`
	TotalMS float64 `json:"total_ms"`
	MaxMS   float64 `json:"max_ms"`
	// StartLine/EndLine: the member line envelope (行a..b pointer).
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
	// Basis ∈ {self, chain_member, host_wakeup_edge} (closed set, 凭证词如实).
	Basis string `json:"basis"`
	// Hidden: members below the bounded display view — informational 0..Count
	// (POOL2-1 件①, §29.160①: no longer an admission input; 0 = the family is
	// fully visible on the bounded seat view and still mentions).
	Hidden int `json:"hidden"`
}

// TraceCausalProjectionTargetStateAccount is the §29.27② typed carrier of the
// focused thread's full-window state partition (see the field doc above).
type TraceCausalProjectionTargetStateAccount struct {
	Subject    string  `json:"subject,omitempty"`
	RunningMS  float64 `json:"running_ms,omitempty"`
	RunnableMS float64 `json:"runnable_ms,omitempty"`
	SleepMS    float64 `json:"sleep_ms,omitempty"`
	DStateMS   float64 `json:"d_state_ms,omitempty"`
	IOWaitMS   float64 `json:"io_wait_ms,omitempty"`
	// SleepIOWaitMS (复核 A-1): the sleep-side IO refinement label value —
	// already inside SleepMS, never an addend (the Σ identity gate ignores
	// it); feeds the sleep term's 「其中 IO等待」 clause only.
	SleepIOWaitMS          float64 `json:"sleep_io_wait_ms,omitempty"`
	TotalMS                float64 `json:"total_ms,omitempty"`
	DeterministicRunningMS float64 `json:"deterministic_running_ms,omitempty"`
	// HeadCarry* / TailOpen* (§29.140 G6, ANSWERFACE-1 件2): the account's
	// window-boundary extrapolated components — head prefix carried from the
	// recovered pre-window scheduler state, tail suffix flushed from the
	// final open interval (no in-window closing event). Each ms value is
	// ALREADY inside the lane its *State names (running/runnable/sleep/
	// d_state/io_wait); the Σ identity gate ignores them (disclosure only,
	// never an addend); they feed the four-state row's
	// 「含未覆盖段…折入」 term annotation.
	HeadCarryMS    float64 `json:"head_carry_ms,omitempty"`
	HeadCarryState string  `json:"head_carry_state,omitempty"`
	TailOpenMS     float64 `json:"tail_open_ms,omitempty"`
	TailOpenState  string  `json:"tail_open_state,omitempty"`
	WindowStartTs  float64 `json:"window_start_ts,omitempty"`
	WindowEndTs    float64 `json:"window_end_ts,omitempty"`
	EvidenceID     string  `json:"evidence_id,omitempty"`
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
// §29.183 G8: existence via the shared predicate — a rebased [0,end] anchor
// window is a REAL window and keeps its duration (and thereby the 占窗% and
// bar full-scale denominators downstream).
func (p TraceCausalProjection) WindowDurationMS() float64 {
	if !TraceCausalProjectionWindowPresent(p.WindowStartTs, p.WindowEndTs) {
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
	// OnChainBasis mirrors the producer's typed on_chain_basis rich note
	// (SELF-SEM §29.61.1 + SELF-ALL §29.61.2, 2026-07-13): "" = legacy
	// chain-window overlap basis; "self_deterministic_span" = the analysis
	// target's own deterministic semantic work admitted to the on-chain
	// channel without overlap and without any wakeup-edge claim (causality
	// carries "self_deterministic"); "self_wall_clock_interval" = the
	// target's own wall-clock seat admitted the same way (causality
	// "self_wall_clock"); "host_wakeup_edge_pre_span" (R3-IMPL §29.88.1) = a
	// NON-target host's semantic span seated by the host's own in-window
	// typed wakeup edge toward the target (causality keeps the honest
	// "on_wakeup_chain" — a real edge exists; the 行2 唤醒锚定(宿主→目标)
	// sentence forks on this token); "host_wakeup_edge_pre_state"
	// (ONCHAIN-3c, 2026-07-19) = a NON-target, NON-chain-member host's
	// runnable / D-IO STATE seat anchored by the same credential (value = the
	// segment inventory's pre-edge share sum; same honest causality; the 行2
	// sentence forks its value clause on this sibling token). Display wording
	// input ONLY (the 「目标自身·确定性优化」/「目标自身·墙钟席」 Row2 qualifiers and
	// the R3 disclosure sentence); no gate, score or sort lane reads it.
	OnChainBasis string `json:"on_chain_basis,omitempty"`
	// ChainBranch is the owning branch ordinal of the node's chain measurement
	// (typed chain_branch note — P0-E CHAIN-PATH, ledger §22.1). The display
	// tree keys its depth attach to (branch, depth): a node from a DIFFERENT
	// branch than the elected trunk never fabricates a trunk position (the
	// fake-L26/L27 family); it keeps its honest 父节点未确认 seat instead. 0 =
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
	// its honest 父节点未确认 seat (缺窗身份≠可挂靠 — the lossy zero never passes
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
	// IOPressure* mirrors the aggregate io_pressure caliber notes. A
	// count-only blocked_reason roster remains an activity marker even when
	// its legacy mixed-unit index is numerically large. These fields drive
	// only system-authored wording and never alter causal seating or rank.
	IOPressureSignal             string  `json:"io_pressure_signal,omitempty"`
	IOPressureEvidenceQuality    string  `json:"io_pressure_evidence_quality,omitempty"`
	IOPressureScoreCaliber       string  `json:"io_pressure_score_caliber,omitempty"`
	IOPressureConclusion         string  `json:"io_pressure_conclusion,omitempty"`
	IOPressureIOWaitBlockedCount int     `json:"io_pressure_iowait_blocked_count,omitempty"`
	IOPressureBlockMaxMS         float64 `json:"io_pressure_block_max_ms,omitempty"`
	IOPressureStorageMaxMS       float64 `json:"io_pressure_storage_max_ms,omitempty"`
	IOPressureFileBytes          int64   `json:"io_pressure_file_bytes,omitempty"`
	IOPressureFileEvents         int     `json:"io_pressure_file_events,omitempty"`
	IOPressurePageCacheChurn     int     `json:"io_pressure_page_cache_churn,omitempty"`
	// DStateRefinedNonIO (DSTATE-REFINE arm a, CAL-1 件③, 2026-07-12): the
	// engine's typed refined-D proof (dstate_all_noniowait note — io_wait
	// share zero AND blocked_reason 全覆盖∧全0). Display word gate for the
	// refined 「D-state」 form on merged d_state_or_io_wait rows; false keeps
	// the honest 「D-state/iowait」 merged word.
	DStateRefinedNonIO bool `json:"d_state_refined_non_io,omitempty"`
	// BlockedReasonCaller (件③): the unanimous blocked_reason semantic caller
	// (dma_fence_default_wait family) for the 行2 等待对象 disclosure; ""
	// when absent/conflicting (absence never guesses).
	BlockedReasonCaller string `json:"blocked_reason_caller,omitempty"`
	// BlockedReasonWindowCount / BlockedReasonWindowCaller (CR-3 件② P10,
	// 2026-07-12): the UNCONSUMED sched_blocked_reason residual — set only
	// when the row consumed no caller yet the window holds markers for its
	// thread (冷读案7: root-cause row read 未解析 with the GPU-fence marker
	// in hand). Drives the 「窗内存在 N 条 blocked_reason 记录」 disclosure
	// on the unresolved word faces; wording input only.
	BlockedReasonWindowCount  int    `json:"blocked_reason_window_count,omitempty"`
	BlockedReasonWindowCaller string `json:"blocked_reason_window_caller,omitempty"`
	// DStateCauseUnprovenRemainder (§29.50.5 证明分区, v5 P1 批 件②,
	// 2026-07-13): the row is the honest-remainder D/IO seat — its fragments
	// proved NO wait object while sibling cause seat(s) of the same thread
	// carried theirs. Drives the 「D-state(原因未证)」 display qualifier;
	// wording input only (no gate, score or sort lane reads it).
	DStateCauseUnprovenRemainder bool `json:"d_state_cause_unproven_remainder,omitempty"`
	// ChainAnchoredMS / ChainAnchorFullMS / ChainAnchorRemainderSeat (RSPA
	// §29.61.10a/b/c, 2026-07-14): the on-chain seat-value re-anchoring
	// bipartition — a migrated chain thread's window state seat splits into
	// the ⛓ anchored portion (segments ∩ typed wakeup-dependency jump
	// windows) and the ◇ remainder (no chain credential). The two floats ride
	// BOTH halves; the boolean marks the ◇ remainder half. Drives the 行2
	// 「全窗X=锚定Y+其余Z」 decomposition, the ◇ 「调度压力候选」/⧗ lane
	// words and the WO-C1 同源二分 relation sentence; wording/relation input
	// only (values were re-derived engine-side, no display math).
	ChainAnchoredMS          float64 `json:"chain_anchored_ms,omitempty"`
	ChainAnchorFullMS        float64 `json:"chain_anchor_full_ms,omitempty"`
	ChainAnchorRemainderSeat bool    `json:"chain_anchor_remainder_seat,omitempty"`
	// ChainAnchorOwnershipDivergent + ChainAnchorChainLaneMS /
	// ChainAnchorCensusMS (RNB-1, §29.88 R2, 2026-07-14): the case-A'
	// ownership-divergence disclosure — the pid's chain seat is present but
	// does not provably hold the census-anchored account, so the 行2
	// relation sentence downgrades from the additive 同源二分 form to the
	// 账目关系(锚定权属失合) double-account form reading these two Σs (chain
	// seats' published Σ / pid census-anchored Σ; delta is display
	// arithmetic). Wording/relation input only.
	ChainAnchorOwnershipDivergent bool    `json:"chain_anchor_ownership_divergent,omitempty"`
	ChainAnchorChainLaneMS        float64 `json:"chain_anchor_chain_lane_ms,omitempty"`
	ChainAnchorCensusMS           float64 `json:"chain_anchor_census_ms,omitempty"`
	// ChainCredentialLaneDemoted (RNB-1 R4, §29.88.2, 2026-07-14): the whole
	// seat rides the ◇ adjacent channel because its account cannot show a
	// typed causal-edge anchored share (affinity satellite / inversion-
	// retyped seat / zero-credential D-IO view row); values untouched —
	// drives the 「无链上凭证(整席降道)」 disclosure line only.
	ChainCredentialLaneDemoted bool `json:"chain_credential_lane_demoted,omitempty"`
	// ChainCredentialSegments / ChainCredentialSegmentDisjoint /
	// ChainCredentialEnvelopeLevel (HULL-CRED, §29.104 终判③, 2026-07-17):
	// the keep-⛓ per-segment credential trio of the chain-lane D/IO VIEW
	// verdict, parsed strictly from the chain_credential_* notes.
	//
	//   - ChainCredentialSegments is the row's COMPLETE typed evidence
	//     segment inventory ([start,end] seconds pairs) — all-or-nothing
	//     decode (any malformed entry, or a set beyond the engine-mirrored
	//     TraceCausalProjectionChainCredentialSegmentCap, decodes to nil:
	//     a partial inventory could fake an adjudication; absence never
	//     judges). Present on the two segment-adjudicated verdict forms only.
	//   - ChainCredentialSegmentDisjoint marks the per-segment-proven demote
	//     form; the 逐段核验 word fork renders ONLY when the decoded
	//     inventory is present beside it (claim gated on proof — an
	//     inventory-less marker falls back to the generic R4 word bytes).
	//   - ChainCredentialEnvelopeLevel marks the honest conservative keep:
	//     the ⛓ lane was retained on the envelope/census fail-open tier and
	//     the row wears the 「交集证明(包络级)」 word. Wording/channel inputs
	//     only; never a gate, score or sort lane.
	//   - ChainCredentialSegmentsTruncated (ONCHAIN-FIX-2 件3, Q6 已追认,
	//     2026-07-18): the decoded inventory is the ledger's checked PREFIX
	//     of a beyond-cap D/IO group — a proven LOWER BOUND, not the
	//     complete account. Meaningful ONLY beside a non-empty decoded
	//     inventory on a keep-⛓ row (the ≥1-true-intersection arm); drives
	//     the 「凭证清单不完整,实际锚定不小于所证」 wording. A truncated prefix
	//     never rides the disjoint demote form (缺证≠证无 — the engine falls
	//     to the envelope tier there).
	ChainCredentialSegments          [][2]float64 `json:"chain_credential_segments,omitempty"`
	ChainCredentialSegmentDisjoint   bool         `json:"chain_credential_segment_disjoint,omitempty"`
	ChainCredentialEnvelopeLevel     bool         `json:"chain_credential_envelope_level,omitempty"`
	ChainCredentialSegmentsTruncated bool         `json:"chain_credential_segments_truncated,omitempty"`
	// ChainIdentityInheritance (ONCHAIN-FIX-1 件1, 2026-07-18): the
	// interval-less identity-inheritance admission marker — the row published
	// no typed interval and inherited the on-chain lane from bare thread
	// identity (fail-open keep; the fabricated whole-node-window overlap is
	// retired). Drives the 「成员继承(链窗级,无区间凭证)」 disclosure word
	// ONLY while the row rides the on-chain lane and carries no stronger
	// credential vocabulary (HULL-CRED per-segment / envelope words win).
	// Wording/channel input only; never a gate, score or sort lane.
	ChainIdentityInheritance bool `json:"chain_identity_inheritance,omitempty"`
	// ChainCredentialCensus (CHAINGUARD-1 件2, §29.204/§29.204.1, 2026-07-22):
	// the engine chain-credential census verdict (closed enum wakeup_anchored
	// / target_self / interval_proven / member_inherited / none; "" = absent —
	// pre-census artifact or chainless board, every consumer keeps the legacy
	// behavior). Strict-parsed from the chain_credential_census note. The ◎
	// credential chip maps this enum (件3 引擎同源) and the board/crown second
	// seat gate rejects "none" seats (件2 — a zero-credential seat merged in
	// from another query can never elect/crown/badge on a chained tree).
	// Wording/channel input only; never a value, score or sort lane.
	ChainCredentialCensus string `json:"chain_credential_census,omitempty"`
	// ChainAnchorRepresentedByChainSeat (XLANE-1 件1, §29.104.1/§29.104.2,
	// 2026-07-15): the fully-anchored runnable-family satellite whose anchored
	// share is already represented by a physically intersecting same-pid
	// chain-lane runnable seat — the whole seat rides the ◇ adjacent channel
	// with values untouched. Drives the 「锚定份由链席[E#]代表(整席降道)」
	// disclosure line ONLY (honest word face: the satellite HAS credential,
	// so the 无链上凭证 sentence is forbidden on it). Wording/channel input
	// only.
	ChainAnchorRepresentedByChainSeat bool `json:"chain_anchor_represented_by_chain_seat,omitempty"`
	// GatedShare* (LEVELMERGE-1 件2 方案 P 区间分账, user ruling 2026-07-18):
	// the (pid,runnable) chain aggregate seat's interval-accounting split
	// against the same thread's priority-inversion seat(s) — the A share
	// (claimed, already counted inside the inversion seat's gated composite)
	// rides the demoted constituent row (GatedShareConstituentSeat=true,
	// adjacent lane, value = claimed) while the surviving seat publishes the
	// residual B with claimed + residual == GatedShareFullMS (the pinned
	// identity). GatedShareClaimSeats carries the claiming inversion seats'
	// own line intervals ("start..end") — the display resolves each to its
	// [E#] all-or-nothing. GatedShareOverlapDisclosureMS is the fail-open
	// arm (裁定④ 「其中 X ms 与[E#](反转席)重叠」 clause; published values
	// untouched). Wording/relation inputs only — never a gate/score/sort
	// lane.
	GatedShareClaimedMS           float64  `json:"gated_share_claimed_ms,omitempty"`
	GatedShareFullMS              float64  `json:"gated_share_full_ms,omitempty"`
	GatedShareConstituentSeat     bool     `json:"gated_share_constituent_seat,omitempty"`
	GatedShareClaimSeats          []string `json:"gated_share_claim_seats,omitempty"`
	GatedShareOverlapDisclosureMS float64  `json:"gated_share_overlap_disclosure_ms,omitempty"`
	// GatedCompositeEdge* (PARTSPLIT-1, §29.150④ user ruling 2026-07-19): the
	// R4-mirror refusal record on a gated composite seat — the pre/post
	// bisection measures of the seat's runnable census account at the host's
	// own credential-edge boundary, DISCLOSURE ONLY (the conversion was
	// refused; every published value/lane/ordinal untouched). Drives the 行2
	// 分账披露 sub-line, which re-validates PreShare + PostShare == the row's
	// own RunnableMS (µs) before any wording renders (宁漏勿假指). The four
	// fields travel together or not at all (atomic engine stamp). Wording/
	// description inputs only — never a gate/score/sort lane.
	GatedCompositeEdgePreShareMS  float64 `json:"gated_composite_edge_pre_share_ms,omitempty"`
	GatedCompositeEdgePostShareMS float64 `json:"gated_composite_edge_post_share_ms,omitempty"`
	GatedCompositeEdgeAnchorTS    float64 `json:"gated_composite_edge_anchor_ts,omitempty"`
	GatedCompositeEdgeAnchorVia   string  `json:"gated_composite_edge_anchor_via,omitempty"`
	// AbsorbedWholeSeatDemotedView (XLANE-2 件3, E11 rider §29.109 记录①,
	// 2026-07-17): this explicit chain-face survivor R1-absorbed a WHOLE-SEAT
	// DEMOTED ◇ view of its fact (R4 no-credential / XLANE-1 represented).
	// The demotion markers themselves never cross onto the chain face (the
	// three-face contradiction: ➊ + ├─链上─ + 根因排序#N + 无链上凭证 on one
	// row), but the account-identity memory must survive: the absorbed view's
	// account can OVERLAP its same-(subject,object) siblings, and the ×N
	// same-kind fold would otherwise re-Σ overlapping accounts (the fused
	// donghu 低频运行 32.877 false-sum shape). anchorFormKey forks on it —
	// word faces never read it. INTERNAL typed carrier (minted only at the
	// R1 absorb single point, propagated OR-monotone by later merges; no
	// note key / no LLM surface — the EffectiveImpactPublished carrier class,
	// R2' exempt).
	AbsorbedWholeSeatDemotedView bool `json:"absorbed_whole_seat_demoted_view,omitempty"`
	// HostWakeupEdgeAnchorTS / HostWakeupEdgeAnchorVia (R3-IMPL, §29.88.1
	// user ruling 2026-07-14): the host-edge-anchored semantic seat's typed
	// credential pair — the latest in-window credential edge timestamp (the
	// bisection boundary, µs-verifiable) and the typed edge-inventory word
	// (direct / chain_hop / direct+chain_hop). Drives the 行2 边锚定(宿主→
	// 目标) disclosure sentence beside the OnChainBasis fork; wording/
	// description input only.
	HostWakeupEdgeAnchorTS  float64 `json:"host_wakeup_edge_anchor_ts,omitempty"`
	HostWakeupEdgeAnchorVia string  `json:"host_wakeup_edge_anchor_via,omitempty"`
	// CPUConstraint* (RNB-2 件5 AFF-EVID, §29.88.6, 2026-07-15): the affinity/
	// cpuset seat's typed judgment payload — drives the 行3/明细 constraint-
	// description line (允许核集 vs 全域观测核对照 + cpuset 组名 + 判定依据
	// kind); ExcludedCPUs doubles as the §29.88.4 R5a comparison-input reserve
	// (per-core-档 refinement lands with RNB-4 R6). Wording/description
	// inputs only; never rank/score inputs.
	CPUConstraintKind   string `json:"cpu_constraint_kind,omitempty"`
	CPUConstraintCPUSet string `json:"cpu_constraint_cpuset,omitempty"`
	// V1 dual-review P2 (2026-07-26): binding-provenance bit — true = the
	// group name came from a real binding EVENT (gate input); false = the
	// sched_switch cg= proxy (display context), and the word face must say
	// cgroup, not cpuset.
	CPUConstraintCPUSetIsBinding bool   `json:"cpu_constraint_cpuset_is_binding,omitempty"`
	CPUConstraintPolicy          string `json:"cpu_constraint_policy,omitempty"`
	CPUConstraintAllowedCPUs     []int  `json:"cpu_constraint_allowed_cpus,omitempty"`
	CPUConstraintExcludedCPUs    []int  `json:"cpu_constraint_excluded_cpus,omitempty"`
	// R5a (§29.88.4 场景② 按核档, 2026-07-15): the tier-exclusion proof pair
	// — non-zero together exactly when the binding provably excludes a bigger
	// core tier; drives the obligatory 「绑核排除更大核档」 mention on the
	// constraint-description line. Wording input only.
	CPUConstraintAllowedMaxTierKHz int `json:"cpu_constraint_allowed_max_tier_khz,omitempty"`
	CPUConstraintGlobalMaxTierKHz  int `json:"cpu_constraint_global_max_tier_khz,omitempty"`
	// ResourceCompletionClosure (RSPA M-IO §29.61.10c): the io_latency row's
	// typed per-IO completion-closure credential (completion thread woke an
	// anchored D/IO wait of a chain thread inside the IO's lifetime).
	// Wording/context input only.
	ResourceCompletionClosure bool `json:"resource_completion_closure,omitempty"`
	// SystemSupplement (SUPP-CORE 修复轮 件5 / 冷读 SC-F1, 2026-07-14):
	// the node compiled from a SUPP-CORE system-supplement record
	// (ObservationRecord.SystemSupplement — deterministic tool witness the
	// SYSTEM dispatched at the explore→extract boundary, not the model).
	// Drives the E# audit face's `origin=system_supplement` render token
	// (pure display token, no wire note key — R2' exempt); never a gate,
	// score or sort input.
	SystemSupplement bool `json:"system_supplement,omitempty"`
	// ProcessTGID / ProcessComm (CR-3 件③ P11, 2026-07-12; 冷读案8 关键角色
	// 裸线程名无 tgid): the row's process attribution — the trace-published
	// tgid and the resolved owning process comm. Detail identity 「进程
	// tgid=G comm=P」 line input only.
	ProcessTGID int    `json:"process_tgid,omitempty"`
	ProcessComm string `json:"process_comm,omitempty"`
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
	// EffectiveImpactPublished (EPUB, §29.31 立案 2026-07-11) is the typed
	// effective-published marker: true iff the source record PUBLISHED an
	// effective-attribution rich note (effective_impact_ms / legacy
	// effective_impact present — the engine prints authoritative zeros
	// explicitly on the context_only / periodic / causal-impact lanes, while
	// the positive-only note filters DROP unpublished slots), so a published
	// 0 is distinguishable from an unpublished (absent) effective. On the
	// float64 wire both are 0 — exactly the ambiguity this field kills.
	// INTERNAL typed carrier (no LLM-facing schema/note surface — same class
	// as the StateKind/Undrillable presentation fields, no R2' six-spot
	// obligation): minted ONLY at the record decode single point and
	// propagated by the R1 merge / ×N fold arms; consumers MUST NOT re-derive
	// it from EffectiveImpactMS==0. Consumed by the V1 rankless lead lane's
	// generalized published-eff≤0 refusal arm (PeriodicSource stays the #68
	// 用户裁定 typed exemption). Degraded text re-parse lanes drop zero notes
	// (positive-only filters), mapping published-0 → unpublished there: the
	// fail-open direction (a lost marker can only KEEP today's crown, never
	// refuse one).
	EffectiveImpactPublished bool    `json:"effective_impact_published,omitempty"`
	ActualImpactMS           float64 `json:"actual_impact_ms,omitempty"`
	// ActualWindowStartTs/EndTs is the physical extent of the underlying
	// scheduler-state segment behind ActualImpactMS — the producer's typed
	// actual_window note through the ONE strict window parser (CR-2 组③ P7,
	// ledger §29.42, 2026-07-12). The ⚠ 词面 gate consumes it: 「实际状态跨出
	// 分析窗」 may only render when this interval provably leaves the analysis
	// window (interval containment — a value comparison alone proved false on
	// 11 rows, 冷读案19: the actual exceeded the row's own OCCURRENCE
	// sub-window while sitting fully inside the analysis window). Zero when
	// the producer published no interval (absence never guesses — and never
	// mints a ⚠).
	ActualWindowStartTs float64 `json:"actual_window_start_ts,omitempty"`
	ActualWindowEndTs   float64 `json:"actual_window_end_ts,omitempty"`
	// SemanticChainProjectedMS (审计 #5/#62, §29.25 处置委托 + §29.26 待主会话
	// 落账, 2026-07-10) is the engine's exact member∩chain intersection union
	// of an ON-CHAIN trace_semantic_span record — the ONE participation value
	// the rank lane publishes as EffectiveImpactMs under the SEM-LEAD
	// intersection caliber, while the record's Value/ImpactMS stays the
	// complete selected-window member union (lossless observation, §24.10
	// 窗口投影 semantics). Sourced from the promoted projected_impact (family)
	// / overlap (single-span) rich notes, ONLY on on-chain semantic rows.
	// Consumers: the E9/E13 twin-seat fold's value mirror (rank participation
	// vs THIS value — same source on both arms) and the ✦ row's 有效归因
	// label (never the bare union). Zero = no typed intersection (off-chain
	// rows, legacy replays) — every consumer fails open to legacy behavior.
	SemanticChainProjectedMS float64 `json:"semantic_chain_projected_ms,omitempty"`
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
	// DrilldownWakeupPointKnown keeps the point-presence bit separate from
	// the timestamp because a trace may legitimately start at 0s. The point
	// and line come only from the typed wakeup_chain_edge ObservationSpan;
	// path-only fallback and conflicting repeated edges leave them unknown.
	DrilldownWakeupPointKnown bool    `json:"drilldown_wakeup_point_known,omitempty"`
	DrilldownWakeupTs         float64 `json:"drilldown_wakeup_ts,omitempty"`
	DrilldownWakeupLine       int     `json:"drilldown_wakeup_line,omitempty"`
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
	// MicroAnchorFold (RNB-5B 件⑦, §29.96.2 终判⑦, 2026-07-15) marks the
	// micro anchored-cut-seat fold row: chain-lane anchored bipartition CUT
	// seats below the display micro threshold fold into ONE counted row
	// (「其余 N 项微额锚定席」) whose value channel carries the members' account
	// Σ (per the user ruling's explicit 「(合计 X)见明细」 form — an account
	// sum over sub-0.1ms anchored shares, disclosed as 合计). The credential
	// semantics are preserved: the fold row stays on the ⛓ on-chain channel.
	// Display-built only (buildRuntimeTraceProjTreeModel); the engine never
	// mints it.
	MicroAnchorFold bool `json:"micro_anchor_fold,omitempty"`
	// MergedWireFold (RNB-5B 件⑥, §29.96.2 终判⑥, 2026-07-15) marks a Merged*
	// channel re-materialized from the ENGINE's wire folded_* note family
	// (TraceNoteKeyFoldedRows — the wire-cap impact fold / engine aggregate
	// fold, whose published value IS the member MAX by construction: 引擎
	// wire-fold 自发布). This is the typed SOURCE bit the §24.2 event-class
	// 「单次最大(a~b,共N次)」 equation face keys on — the former trigger
	// (eff==MergedMaxMS at print precision) was a numeric coincidence: a
	// display-merged Σ row whose sum happens to equal its largest member
	// (all other members zero-eff) wore a caliber word describing a fold that
	// never ran. Display-side merges (aggregate ×N, dedup folds, family
	// merges) never set this bit.
	MergedWireFold bool `json:"merged_wire_fold,omitempty"`
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
	// MergedSameSegmentMirror (ISPGAP-1 复核 F-B, §29.207 裁定, 2026-07-22)
	// marks the BACKGROUND-lane same-window cross-record mirror caliber on an
	// R2 ×N row: the members share ONE query window (or carry none) yet their
	// typed occurrence intervals OVERLAP — the multi-call union ledger shape
	// (a targeted board's ▒ twin beside a chainless board's demoted account
	// of the SAME physical whole-window state) re-measures the same wall
	// clock, so a SUM double-counts (§7.5 R2 同段墙钟不可加和 同构; the u2
	// specimen: 52.500 折算 + 150.000 全额 summed to 202.500ms = 135% of the
	// window). The published value is the members' interval-union deduction
	// (overlap counted once — the §11-N2 greedy, exact when every valued
	// member carries a contained interval) or, when the per-segment deduction
	// is unavailable, the member MAX as the honest lower bound (取大作下界,
	// never invents, never the SUM). MergedSumMS keeps the lossless raw Σ.
	// Ordinary single-board ▒ rosters (disjoint per-instance segments) never
	// engage and keep the SUM byte-identically. Mutually exclusive with
	// MergedIntervalUnion / MergedCrossWindowMax (the distinct-window shapes
	// own those calibers and are judged first).
	MergedSameSegmentMirror bool `json:"merged_same_segment_mirror,omitempty"`
	// MergedChainAnchorMemberAccounts (RNB-2 件2, §29.88 W3 病①, 2026-07-15):
	// the R2 ×N merge absorbed ≥1 member carrying a ChainAnchor bipartition
	// account (◇ remainder / ⛓ clipped seat). The per-seat triple is cleared
	// on the merged row (「本行」 grammar cannot speak for a member Σ — the
	// seed's account impersonating the merge is the pinned regression) and
	// this display-only marker makes the 行2 qualifier say the split accounts
	// stay on the members. Never wire-decoded; set exclusively by the merge
	// body (same lane as the Merged* caliber fields above).
	MergedChainAnchorMemberAccounts bool `json:"merged_chain_anchor_member_accounts,omitempty"`
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
	// RankBoardTarget / RankBoardParamsFingerprint (XLANE-3 件1, §29.104.2
	// 定谳③ + §29.104.9 形③, 2026-07-16): the rank BOARD identity triple's
	// target and params halves (the window half is QueryWindow*/
	// RankQueryWindow* above) — verbatim from the producer's typed
	// rank_board_target / rank_board_params_fingerprint notes (the
	// result-level rank target's canonical thread label + the engine's
	// normalized rank-knob fingerprint). A rank ordinal is a PER-BOARD
	// identity: two same-window steps with different targets are two ordinal
	// domains, and the pre-XLANE-3 window-endpoint-only board key rendered
	// their #N chips as bare collisions (donghu 形③ 根因排序#1..#3 各×2).
	// On a merged row both fields follow the rank-supplying member (same
	// donor discipline as RankQueryWindow* — the ordinal and its board
	// identity travel together). Display board-identity/wording inputs.
	// XLANE-3 point-authority follow-up (2026-07-17): the compile's exact-node
	// dedupe preserves distinct NON-EMPTY triples. That is an information-
	// preservation boundary, not a rank/score input: two boards may publish
	// byte-identical subject/predicate/support coordinates while owning
	// different per-board accounts, so collapsing them before presentation
	// silently deletes one board's evidence. An identity-less value-bearing
	// seat remains its own unnamed board; an identity-less zero-account mirror
	// never splits from a named seat. Absence therefore never inherits a board
	// claim and never fabricates an extra display row.
	RankBoardTarget            string `json:"rank_board_target,omitempty"`
	RankBoardParamsFingerprint string `json:"rank_board_params_fingerprint,omitempty"`
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
	// MergedMaxSubject / MergedMaxStateKind (RUN2FIX-A 件2, §29.174 处置②,
	// runnable_2:361-363 witness: the tree's largest single value 47.282ms
	// hid inside 「其余 6 项(折叠)」 with only a range disclosed) name the
	// fold's MAX member — the owner of the published member-MAX value — so the
	// fold row's line 2 can disclose 线程·状态·值 (CAPFIX §29.150① 件2 带值
	// 披露同构). Set by the ONE overflow-fold constructor; all-or-nothing:
	// unknown-subject maxima leave both empty and the display keeps the
	// legacy line (宁漏勿假). Disclosure-only carriers — the fold's published
	// value/min/max/count and every seat/ordinal channel are untouched.
	MergedMaxSubject   string `json:"merged_max_subject,omitempty"`
	MergedMaxStateKind string `json:"merged_max_state_kind,omitempty"`
	// SecondaryObjects carries the other typed views' Objects after an R1 merge
	// when they differ from this node's Object (e.g. the udk-irq peer thread a
	// same-interval critical_blocking row named) — rendered as an 影响点 note.
	SecondaryObjects []string `json:"secondary_objects,omitempty"`
	// IdleCadenceMS / IdleCadenceKind (ENG-2, 复核冷读 CP1-③, 2026-07-12):
	// the value-bearing idle-cadence annotation surviving the R1 same-fact
	// merge. When a P9 arm-c typed idle row (pacing_idle / periodic_idle)
	// folds into a co-located scheduler-state twin, the twin used to carry
	// the token only in SecondaryObjects — which no display face reads, so
	// the 帧间空闲 word vanished from the whole report while the audit face
	// still published it. The absorb records the idle view's ms (one-fact
	// MAX) and kind here; the ×N same-kind merge SUMS members' annotations;
	// the tree row renders 「其中 X.XXXms 帧间空闲(等待下一帧)」 with the
	// matching legend entry.
	IdleCadenceMS   float64 `json:"idle_cadence_ms,omitempty"`
	IdleCadenceKind string  `json:"idle_cadence_kind,omitempty"`
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
	// FamilyMemberLineRanges (XLANE-2 件1, §29.104.1/.2 定谳④, 2026-07-17):
	// the COMPLETE typed per-member trace line ranges of a semantic family
	// seat, parsed strictly from the member_line_ranges note — set ONLY when
	// every entry parses and the entry count equals FamilyMemberCount
	// (all-or-nothing: a partial set could fake a member-subset verdict).
	// Sole consumer = the display 成员子集 subset judgment (the
	// 「为[E#]成员子集」 demotion lane); no gate, score or sort lane reads it.
	FamilyMemberLineRanges [][2]int `json:"family_member_line_ranges,omitempty"`
	// FamilyMemberWallMS (SPANTOP-1 件1, §29.131, 2026-07-18): the COMPLETE
	// typed per-member in-window wall-clock durations of a semantic family
	// seat (same member order as FamilyMemberLineRanges), parsed strictly
	// from the member_wall_ms note — set ONLY when every entry decodes to a
	// positive float and the entry count equals FamilyMemberCount
	// (all-or-nothing: a partial list could fake a member decomposition).
	// Sole consumer = the display constituent top-3 sub-row lane, which
	// additionally requires the µs identity Σ(members) == the seat's 行1
	// value before rendering; no gate, score or sort lane reads it.
	FamilyMemberWallMS []float64 `json:"family_member_wall_ms,omitempty"`
	// SelfGapSemanticOverlaps (XLANE-2 件2, user ruling §29.104.17 ④
	// 披露式拆分, 2026-07-17): the self running supply-fold deficit seat's
	// per-partner semantic-overlap disclosure (typed interval-intersection
	// wall clock + the partner's line envelope, parsed per-entry from the
	// self_gap_semantic_overlaps note — each clause is its own truth,
	// invalid entries drop, never guess). Drives the row-level
	// 「其中 X ms 与语义席[E#]重叠」 clause ONLY (主值零动 — no value
	// channel, gate, score or sort lane reads it).
	SelfGapSemanticOverlaps []TraceCausalProjectionSelfGapSemanticOverlap `json:"self_gap_semantic_overlaps,omitempty"`
	// FixDirection (AXIOM-V2 件1, user rulings 2026-07-18): the registry
	// repair-direction token of the row's causal type, verbatim from the
	// fix_direction note (closed set owned by the tracequery registry; ""
	// = unresolved/legacy — absence never guesses). Attribute axis only: the
	// display 行2 direction word and the 互指句 direction qualifier fork on
	// it; no gate, ordinal or value lane reads it.
	FixDirection string `json:"fix_direction,omitempty"`
	// CrossDirectionOverlaps (AXIOM-V2 件2): the cross-direction overlap pair
	// roster, parsed per-entry from the cross_direction_overlaps note (typed
	// interval-intersection wall clock + the PARTNER's line envelope, fix
	// direction and support basis). SYMMETRIC across the pair on the wire;
	// the display resolves both [E#] endpoints verbatim and renders the
	// 「同段重叠…收益不叠加」 互指句 on both seats or neither (宁漏勿假指).
	// Pure disclosure — no value channel, gate, score or sort lane reads it.
	CrossDirectionOverlaps []TraceCausalProjectionCrossDirectionOverlap `json:"cross_direction_overlaps,omitempty"`
	// DirectionConservationExcess (ELIM-V2 守恒尾行, 2026-07-18; the AXIOM-V2
	// 件3 checker's violation finding, parsed verbatim from the
	// direction_conservation_excess note "direction@sumMs@windowMs@seatCount").
	// Identical across every member seat of the violating (thread, direction)
	// population — the ◎ overview dedupes per finding tuple and renders the
	// per-direction violation disclosure line (§29.104.13 非致命不硬拦: pure
	// disclosure; no gate, ordinal or value lane reads it). nil = the checker
	// published no violation for this seat.
	DirectionConservationExcess *TraceCausalProjectionDirectionConservation `json:"direction_conservation_excess,omitempty"`
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
	BlockingHolderSource string `json:"blocking_holder_source,omitempty"`
	BlockingOwnerTidRaw  int    `json:"blocking_owner_tid_raw,omitempty"`
	// BlockingOwnerTidPresence (LOCKNS-FIX 修补 件A, 冷读 P2-F1+P3-F7 同族,
	// 2026-07-16): mirrors the producer's typed owner_tid_presence note —
	// closed set "absent" / "present_collision" / "present_comm_mismatch"
	// (engine constants tracequery.OwnerTidPresence*). The detail 持有者来历
	// wakeup-edge origin line forks its presence clause on it: the two
	// present shapes stop claiming 「不在本 trace」/"absent from this trace"
	// (that claim contradicted the same board's engine collision Summary);
	// empty (absent shape / legacy records / unknown values) keeps the legacy
	// sentence byte-identically (fail-open). Wording input only — never a
	// behavior gate.
	BlockingOwnerTidPresence    string `json:"blocking_owner_tid_presence,omitempty"`
	BlockingHolderHandoff       string `json:"blocking_holder_handoff,omitempty"`
	BlockingHolderContradiction string `json:"blocking_holder_contradiction,omitempty"`
	// BlockingHolderNsUnification (LOCKNS-FIX 件6 / OM-10 关账, §29.104.12,
	// 2026-07-16): the typed ②×③ identity-unification declaration
	// ("owner_ns_tid=<N> host=<thread> lanes=…") from the holder_ns_unification
	// rich note — the ns-span derivation and the closing wakeup edge
	// INDEPENDENTLY named the same host thread. The detail 持有者来历 line
	// appends the 「发射对×收尾唤醒两道互证」 disclosure when present; empty
	// (single-lane derivations, legacy records) renders nothing.
	BlockingHolderNsUnification string `json:"blocking_holder_ns_unification,omitempty"`
	// BlockingOwnerKeyUnregistered (LOCKNS-FIX 件3, §29.104.12, 2026-07-16):
	// mirrors the producer's typed blocking_owner_key_unregistered note — a
	// payload-less blocking span whose name speaks lock-owner vocabulary but
	// matched no registered contention morphology (fail-open lane; no holder
	// minted). Drives the detail 持有者核查 「owner 未解析(形态未注册)」
	// disclosure line only — never a behavior gate.
	BlockingOwnerKeyUnregistered bool `json:"blocking_owner_key_unregistered,omitempty"`
	// BlockingHolderContradictionParts (G10-EN 根修, QH2-A 2026-07-14):
	// the typed components of the withdrawal witness above, assembled from
	// the holder_self_contradiction_* note quintet — the zh/EN detail lanes
	// each word their own sentence from them (WitnessText); the zh string
	// stays the byte-frozen legacy value. nil (legacy records without the
	// component notes) keeps the verbatim-string fallback on both lanes.
	BlockingHolderContradictionParts *TraceHolderSelfContradictionWitness `json:"blocking_holder_contradiction_parts,omitempty"`
	// BlockingSubjectIsHolder (BLK §15.C, 2026-07-06) mirrors the producer's
	// typed "subject_is_lock_holder=true" note: THIS node's Subject is the lock
	// HOLDER and BlockingPeer is the blocked WAITER (the resolved rank lock
	// row). The renderer then reads the row as a HOLD — "持锁 X ms 阻塞了
	// <BlockingPeer>" — instead of the reversed "锁竞争等待(持有者 <BlockingPeer>)"
	// the waiter-subject critical_blocking node already carries for the SAME
	// physical lock, and the next-step names the HOLDER (the subject), never the
	// waiter. Empty/false keeps the waiter-subject lock-wait wording.
	BlockingSubjectIsHolder bool `json:"blocking_subject_is_holder,omitempty"`
	// XERR1-FIX 件1/件2/件3 (§29.104.3/.4, 2026-07-15) + XERR1-EXT 裁定⑤
	// (§29.104.17, 2026-07-16) — the blocking_span value-convergence carriage
	// (blocking_value_basis / blocking_wait_* / blocking_span_envelope_ms
	// notes), BOTH payload lanes since XERR1-EXT.
	//
	// BlockingValueBasis forks the word face on PAYLOAD-LESS rows only:
	// "wait_segments" keeps the 阻塞等待 family (the value IS the waiter's
	// proven Σ(sleep+D+iowait) inside span∩window) with the peer demoted to
	// 「span 期间最后唤醒者(推断)」; "span_envelope" retreats the word to
	// 「span 包络(含运行)」 (convergence impossible). PAYLOAD-TYPED rows
	// (BlockingKind!="") carry the basis too — their VALUE converged the same
	// way (fold value-winner interval) — but keep the lock word family
	// (锁竞争·阻塞/持锁) and consume the basis only on the 值口径 detail
	// line. Empty (legacy artifacts) keeps every legacy word byte-identically.
	// BlockingWaitSleepMS>0 gates the 互指 disclosure against the thread's
	// own sleep seat (payload-less rows only — a payload row's convergence
	// interval is not on the wire, so the containment proof would prove the
	// wrong interval). The budget trio drives the 件3 ⚠ line 「span 包络 X >
	// 窗内非 running Y:含 running Z,非阻塞等待段」 (on holder-subject rank
	// records the budget describes the WAITER — the record's BlockingPeer).
	BlockingValueBasis             string  `json:"blocking_value_basis,omitempty"`
	BlockingWaitSegmentMS          float64 `json:"blocking_wait_segment_ms,omitempty"`
	BlockingWaitSleepMS            float64 `json:"blocking_wait_sleep_ms,omitempty"`
	BlockingSpanEnvelopeMS         float64 `json:"blocking_span_envelope_ms,omitempty"`
	BlockingWaitBudgetExceeded     bool    `json:"blocking_wait_budget_exceeded,omitempty"`
	BlockingWaitBudgetNonRunningMS float64 `json:"blocking_wait_budget_non_running_ms,omitempty"`
	BlockingWaitBudgetRunningMS    float64 `json:"blocking_wait_budget_running_ms,omitempty"`
	// BlockingWaitCoveragePartial + BlockingWaitAccountCoveredMS (XERR1-FIX
	// 修补 件F, 冷读 P3-3, 2026-07-16): the waiter's state account did not
	// tile the whole span∩window interval, so the converged wait_segments
	// value is a PROVEN LOWER BOUND — the detail face adds the 覆盖核查 line
	// 「等待段账目未满覆盖 span 窗(账目 X ms/span 窗 Y ms):收敛值为已证下界」.
	// wait_segments basis only (both payload lanes since XERR1-EXT;
	// payload-typed rows render the NUMBERLESS claim — their convergence
	// interval is not on the wire, 不造数); absence renders nothing.
	BlockingWaitCoveragePartial  bool    `json:"blocking_wait_coverage_partial,omitempty"`
	BlockingWaitAccountCoveredMS float64 `json:"blocking_wait_account_covered_ms,omitempty"`
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
	// PeriodicTimerCaller (GAP-B2 复核修, 2026-07-25): non-empty (bare caller
	// symbol, e.g. "timerfd_read") exactly when the periodic discount came
	// via the D∧timer arm — the wording-fork credential: such a row's
	// discounted quantity is a D-state timer wait, so the 期内睡眠 caption
	// must not render for it. Wording input only, never a gate.
	PeriodicTimerCaller string `json:"periodic_timer_caller,omitempty"`
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
	// folded at the R5 global max-core peak-frequency basis (lower bound
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
	// SupplyFoldCapabilityFreqOnlyReason (CLUSTER-FIX-2 件1, S1): the typed
	// freq_only CAUSE token (fold_capability_freq_only_reason rich note,
	// closed CoreCapabilityFreqOnlyReason* set). Wording input only: the
	// display forks the single-cluster wording (仅单簇有频点采样…) on it;
	// absence renders every legacy freq_only wording byte-identically. The
	// gated twin is GatedCapabilityFreqOnlyReason below (DISPHYG-3 件7,
	// 2026-07-20 — the formerly open "no reason twin" boundary has closed).
	SupplyFoldCapabilityFreqOnlyReason string `json:"supply_fold_capability_freq_only_reason,omitempty"`
	// SupplyFoldReferenceClass (CAP 复核 F1): the demoted fold-reference class
	// (small/middle/prime — fold_reference_class rich note). Empty = the
	// nominated big-class basis (the producer emits the note only on
	// demotion — R5 retired the demotion word fork; the field stays a
	// wire/audit record of the basis cluster's class).
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
	// GatedCapabilityFreqOnlyReason (DISPHYG-3 件7, 2026-07-20): the gated
	// twin of SupplyFoldCapabilityFreqOnlyReason — the typed freq_only cause
	// token of the gated running component's capability judgment
	// (gated_capability_freq_only_reason rich note; non-empty iff the gated
	// caliber is freq_only). Wording input only: the gated caliber suffix
	// forks its freq_only wording through the SAME clause single point as
	// the supply-fold face.
	GatedCapabilityFreqOnlyReason string `json:"gated_capability_freq_only_reason,omitempty"`
	// ThermalCapKHz (THERM §28.5-T7, disclosure-only): the fold's dominant
	// running cluster was pressed below its fmax inside the governance window
	// (thermal rail and/or governing limits Max) down to this kHz value —
	// thermal_cap_khz rich note. The display appends the 窗内该簇受热限压至 X
	// sentence; zero-weight (no number changes), absent when cluster
	// attribution was unavailable (absence never guesses).
	ThermalCapKHz int `json:"thermal_cap_khz,omitempty"`
	// ThermalCapWitnessed (CR-3 件⑥ F-10, 2026-07-12; 冷读 D5): whether the
	// cap has an IN-WINDOW limits/thermal event witness — the wording gate
	// between 受热限压至 X and 运行于 X(限压原因未见证).
	ThermalCapWitnessed bool `json:"thermal_cap_witnessed,omitempty"`
	// RunnableMS mirrors the node's typed "runnable=" rich note (the row's
	// own in-window runnable wall clock) — consumed by the §7.10 decision
	// table's shared RN-1 significance check and the mechanism clause's
	// "调度压力 runnable Y ms" magnitude. 0 when the source row did not
	// expose the per-state split.
	RunnableMS float64 `json:"runnable_ms,omitempty"`
	// DStateSplitMS / IOWaitSplitMS mirror the node's typed "d_state=" /
	// "io_wait=" rich notes (the rank row's own per-state split — the SAME
	// already-emitted notes the RunnableMS lane consumes, one more decoded
	// consumer, never a new signal). WO-A1 (SMR-1 批 SMR-S5, smr_audit_report
	// §④判定(b), 2026-07-12): the addition-identity arm's typed complement —
	// a chain d/io rank seat publishing X = d_state + io_wait beside the
	// same thread's d_state trunk aggregate Y where |X−(D+IO)|≤tie ∧ Y≈D
	// proves Y ⊂ X (31693 E4/E11: 17.819 = 17.442 + 0.377). Display wording
	// input only; 0 when the source row did not expose the split.
	DStateSplitMS float64 `json:"d_state_split_ms,omitempty"`
	IOWaitSplitMS float64 `json:"io_wait_split_ms,omitempty"`
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
	// OverflowMirrorEvidenceIDs (WO-D1③ 多引用 tag arm, SMR-1 批 SMR-S9,
	// 2026-07-12; 31552 E25 witness): set ONLY on an OnChainOverflowFold row
	// whose headline (取最大) member is an ×N aggregate whose derivable member
	// values each µs-match a distinct RENDERED same-(subject,state) row — the
	// matched kept rows' evidence ids, so the display stamps 「同段镜像·与
	// [E#]+[E#]同一物理时间,不可相加」 on the headline. Tag-only (the pool row
	// and its count stay honest); no gate/sort lane reads it.
	OverflowMirrorEvidenceIDs []string `json:"overflow_mirror_evidence_ids,omitempty"`
	// OverflowProjectionEvidenceID (P2-2 跨口径穿透, SMR-1 修复轮 2026-07-13;
	// 冷读 F-4/F-5: tieba E21 members {4.558,6.325,6.936} are E11's three
	// occurrence PROJECTIONS — Σ=17.819=E11 µs-exact; donghu E26 headline
	// 3.183 = E13's published EffectiveImpactMS µs-exact — cross-CALIBER
	// re-publications the display-value fingerprint cannot see): the rendered
	// row whose account the pool's contents project. Tag-only (「同一物理时间
	// 的口径投影·与[E#]不可相加」); the pool row and its count stay honest.
	OverflowProjectionEvidenceID string `json:"overflow_projection_evidence_id,omitempty"`
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
// their tree seats but never seat on the shared rank board (lead / ➊➋➌) and
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

// TraceCausalTierContextOnly mirrors tracequery.RootCauseTierContextOnly: a
// chain/adjacent/background state observation retained as causal-analysis
// context, but carrying no effective attribution and therefore no root-cause
// rank seat. The exact tier token is the authority; thread names, state words
// and prose never infer this status. ChainRelevance independently decides
// whether the display calls it chain, adjacent, background, or generic context.
const TraceCausalTierContextOnly = "context_only"

// IsContextOnlyRow reports whether this node is causal-analysis context rather
// than a root-cause contender. Context-only rows keep their evidence/display
// seat, but display and election layers must never promote them to a board
// seat, badge, lead, or root-cause candidate even if a stale persisted record
// carries rank fields.
func (n TraceCausalProjectionNode) IsContextOnlyRow() bool {
	return strings.TrimSpace(n.Tier) == TraceCausalTierContextOnly
}

// TraceCausalTierCaliberSide mirrors tracequery.RootCauseTierCaliberSide
// (V2-P0 行级尺守卫, rank_order_v2_design_20260712.md §6.1 新裁定 A,
// 2026-07-12): a count-additivity / composite-score row that left the
// chain/◇ ordinal space — the ⌗ 口径旁栏. The row keeps its channel seat and
// its evidence/display obligation (no silent-disappearance path), publishes
// its value under an explicit caliber word, and must never be promoted to a
// board seat, badge, lead or root-cause candidate even if a stale persisted
// record carries rank fields.
const TraceCausalTierCaliberSide = "caliber_side"

// IsCaliberSideRow reports whether this node rides the ⌗ 口径旁栏 (exact
// typed tier token — thread names, state words and prose never infer it).
func (n TraceCausalProjectionNode) IsCaliberSideRow() bool {
	return strings.TrimSpace(n.Tier) == TraceCausalTierCaliberSide
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
	var targetStateAccounts []traceCausalProjectionTargetStateCandidate
	var queryWindows []TraceCausalProjectionQueryWindow
	queryWindowsTruncated := false
	// SPANVIS-1: the advisory business-span mention side channel — collected
	// per record (all-or-nothing strict parse), deduped by identity so a
	// re-published record set cannot double the list.
	var businessSpanMentions []TraceCausalProjectionBusinessSpanMention
	businessSpanMentionOmitted := 0
	businessSpanMentionSeen := map[string]bool{}
	// PARTSPLIT-1 (§29.150④): the R4-mirror refusal disclosure side channel —
	// collected per record (all-or-nothing strict parse), deduped by
	// (subject, boundary) identity.
	var gatedCompositeEdgeShares []TraceCausalProjectionGatedCompositeEdgeShareDisclosure
	gatedCompositeEdgeShareSeen := map[string]bool{}
	// RULER2-1 (§29.150②): the self runnable two-ruler accounting side
	// channel — collected per record (all-or-nothing strict parse), deduped
	// by subject.
	var selfRunnableTwoRulers []TraceCausalProjectionSelfRunnableTwoRuler
	selfRunnableTwoRulerSeen := map[string]bool{}
	// SELFRUN-DISC (§29.192① (b)): the self supply-fold 「量不了」 absence
	// disclosure side channel — collected per record (all-or-nothing strict
	// parse with the running==unknown identity), deduped by subject.
	var selfRunningFoldUnmeasured []TraceCausalProjectionSelfRunningFoldUnmeasured
	selfRunningFoldUnmeasuredSeen := map[string]bool{}
	for _, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		// NEW-9: exact typed note match — the producer publishes it on every
		// record of a capacity-truncated result (single helper, precise bool).
		if strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCapacityTruncated)) == "true" {
			capacityTruncated = true
		}
		// SPANVIS-1 (2026-07-19): a business_span_mention observation is the
		// pure-advisory business-lens mention face — a projection-level side
		// channel, never a node of its own. Strict all-or-nothing parse; a
		// record failing any typed field drops whole (fail-open to absence).
		if strings.TrimSpace(record.Predicate) == "business_span_mention" {
			if mention, omitted, ok := traceCausalProjectionBusinessSpanMentionFromRecord(record); ok {
				key := mention.Subject + "\x00" + mention.Name + "\x00" +
					strconv.Itoa(mention.StartLine) + ".." + strconv.Itoa(mention.EndLine)
				if !businessSpanMentionSeen[key] {
					businessSpanMentionSeen[key] = true
					businessSpanMentions = append(businessSpanMentions, mention)
				}
				if businessSpanMentionOmitted == 0 && omitted > 0 {
					businessSpanMentionOmitted = omitted
				}
			}
			continue
		}
		// PARTSPLIT-1 (§29.150④): a gated_composite_edge_share observation is
		// the R4-mirror refusal's NON-SEAT pre-edge-share disclosure — a
		// projection-level side channel, never a node of its own. Strict
		// all-or-nothing parse; a record failing any typed field drops whole
		// (fail-open to absence).
		if strings.TrimSpace(record.Predicate) == "gated_composite_edge_share" {
			if disclosure, ok := traceCausalProjectionGatedCompositeEdgeShareFromRecord(record); ok {
				key := disclosure.Subject + "\x00" + strconv.FormatFloat(disclosure.AnchorTS, 'f', 6, 64)
				if !gatedCompositeEdgeShareSeen[key] {
					gatedCompositeEdgeShareSeen[key] = true
					gatedCompositeEdgeShares = append(gatedCompositeEdgeShares, disclosure)
				}
			}
			continue
		}
		// RULER2-1 (§29.150②): a self_runnable_two_ruler observation is the
		// cross-row two-ruler accounting record — a projection-level side
		// channel, never a node of its own. Strict all-or-nothing parse; a
		// record failing any typed field or either same-ruler Σ identity
		// drops whole (fail-open to absence).
		if strings.TrimSpace(record.Predicate) == "self_runnable_two_ruler" {
			if accounting, ok := traceCausalProjectionSelfRunnableTwoRulerFromRecord(record); ok {
				if !selfRunnableTwoRulerSeen[accounting.Subject] {
					selfRunnableTwoRulerSeen[accounting.Subject] = true
					selfRunnableTwoRulers = append(selfRunnableTwoRulers, accounting)
				}
			}
			continue
		}
		// SELFRUN-DISC (§29.192① (b)): a self_running_fold_unmeasured
		// observation is the self supply-fold 「量不了」 absence disclosure —
		// a projection-level side channel, never a node of its own. Strict
		// all-or-nothing parse; a record failing any typed field or the
		// running==unknown fold identity drops whole (fail-open to absence).
		if strings.TrimSpace(record.Predicate) == "self_running_fold_unmeasured" {
			if disclosure, ok := traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord(record); ok {
				if !selfRunningFoldUnmeasuredSeen[disclosure.Subject] {
					selfRunningFoldUnmeasuredSeen[disclosure.Subject] = true
					selfRunningFoldUnmeasured = append(selfRunningFoldUnmeasured, disclosure)
				}
			}
			continue
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
		// §29.27② (COV-4, 2026-07-11): a target_window_states record is the
		// focused thread's full-window state partition — a projection-level
		// side channel, never a node of its own. Only candidates with a
		// parseable typed selected_window collect (禁猜, F-2 discipline).
		if strings.TrimSpace(record.Predicate) == "target_window_states" {
			if candidate, ok := traceCausalProjectionTargetStateCandidateFromRecord(record); ok {
				targetStateAccounts = append(targetStateAccounts, candidate)
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
		case traceCausalProjectionContextOnly(record):
			// A context-only root_cause_* record is intentionally retained on
			// its declared relevance lane, but it must not enter the primary
			// bucket merely because its producer predicate is
			// root_cause_primary. Role normalization here makes every later
			// projection consumer see the same typed non-candidate identity.
			classified = append(classified, traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record))
		case traceCausalProjectionIsPrimaryRootCause(record):
			node := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record)
			// ISPGAP-1 复核 F-A (P1, 2026-07-22): the VALUELESS evidence_fact
			// mirror family (ClaimKey "evidence_fact:…", zero notes, zero
			// value — trace_query.go EvidencePack emission) is EXEMPT from
			// both new relevance gates and never enters the primary slice:
			// on a chained board the mirror was always the dedupe-dropped
			// zero-account twin (it registers no namedBoardBases identity),
			// so classified-with-empty-token is byte-identical to the
			// pre-ISPGAP face; on a chainless board keeping it out of the
			// primary slice preserves the crown vacuum (PrimaryRootCauses
			// stays empty). The first-round background backfill had bucketed
			// the mirrors and (a) surfaced 0.000ms ▒ noise twins, (b) made
			// them ambiguous AXIOM-V2 overlap partners, killing the 互指句
			// (both-or-neither arm) — the exemption restores both faces.
			if strings.HasPrefix(record.ClaimKey, "evidence_fact:") {
				classified = append(classified, node)
				continue
			}
			// ISPGAP-1 件2' (§29.202 / §29.204 CHAINGUARD-F1 定谳, 2026-07-21):
			// the primary/rank lane gains the #1a-style relevance admission
			// gate the hop lane has carried since [P1 修正轮 2026-07-06] — an
			// UNDECLARED-relevance rank-lane record (the chainless board form:
			// untargeted root_cause_rank / span-unresolved frame bundle mints
			// every row with empty Causality/ChainRelevance at full value) must
			// never enter the primary bucket, where the ⛓ crown lanes and the
			// depthless 链上·深度未解析 edge would claim chain identity the
			// engine never declared (customer witness cust_runnable2_cli.txt
			// E10: isplogcat-1225 整窗 D 144.504ms 三无席加冕 ➊). The
			// classified copy defaults into the ▒ background seat instead —
			// the honest, ordinal-less landing (PTS 永不静默丢); the engine's
			// chainless ordinal fail-open (rootCauseOrdinalChannel) stays
			// untouched. Declared rows are byte-identical.
			if node.ChainRelevance == "" {
				node.Role = TraceCausalRoleRootCauseContext
				node.ChainRelevance = "background"
				classified = append(classified, node)
				continue
			}
			primary = append(primary, node)
			classified = append(classified, node)
		case traceCausalProjectionIsRootCauseContext(record):
			node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
			// ISPGAP-1 件2' (same gate, context tier): an undeclared-relevance
			// root_cause_* row otherwise lands in NO bucket at all (the
			// [P1 修正轮 2026-07-06] zero-seat shape) — default the honest ▒
			// seat; declared rows keep their bucket byte-identically.
			// ISPGAP-1 复核 F-A (P1, 2026-07-22): the valueless evidence_fact
			// mirrors stay on the empty token (no bucket — the pre-ISPGAP
			// invisible-carrier face; see the primary-case exemption above).
			if node.ChainRelevance == "" && !strings.HasPrefix(record.ClaimKey, "evidence_fact:") {
				node.ChainRelevance = "background"
			}
			classified = append(classified, node)
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
		OnChainCauses: traceCausalProjectionSelectChainRelevance(classified, "on_chain"),
		// RNB-1 D1 修复轮 (§29.88 复核, 2026-07-14): the context buckets enter
		// aggregation UNCAPPED and value-seat-ordered — the former plain
		// nodes[:8] cut ran on the classified order, whose in-path-first arm
		// let context rows of chain-path threads unconditionally preempt
		// value-bearing seats (donghu 2955 witness: 8 in-path rows ate the
		// whole cap and every ◇ remainder seat, 47.660 down, silently
		// vanished). Value seats (余段/降道/邻近 rank 席) now compete by
		// value; context rows keep their legacy relative order behind them;
		// the fold-with-count cap applies AFTER aggregation (PTS 同型 —
		// post-merge truth, zero silent drops).
		// RNB-5B 件② (§29.96.2 终判②, 2026-07-15): the target-self ⌗ count
		// rows arrive with the NON-CHANNEL "self_caliber_side" token — they
		// ride the adjacent bucket as display CARRIAGE only (the tree model's
		// SELF-LANE relocation re-seats every target-subject row into the self
		// stanza), exempt from the context bucket-cap fold below (the 17267
		// production death: the 计数当量 81.616 self row folded into the ◇
		// overflow row and its count-equivalent value published as the fold's
		// bare-ms MAX with a window share).
		AdjacentCauses: traceCausalProjectionSortContextBucket(append(
			traceCausalProjectionSelectChainRelevance(classified, "adjacent"),
			traceCausalProjectionSelectChainRelevance(classified, "self_caliber_side")...), pathIndex),
		BackgroundCauses:                   traceCausalProjectionSortContextBucket(traceCausalProjectionSelectChainRelevance(classified, "background"), pathIndex),
		SemanticSpans:                      traceCausalProjectionSelectSemanticSpans(semantic, traceCausalProjectionSemanticOffChainLimit),
		WakeupPath:                         wakeupPath,
		WakeupPathUserElected:              wakeupPathUserElected,
		WakeupPathUserEntityHits:           wakeupPathUserEntityHits,
		WakeupPathBranch:                   wakeupPathElected.branch,
		WakeupPathRootDepth:                wakeupPathRootDepth,
		WakeupPathQueryWindowStartTs:       wakeupPathElected.windowStart,
		WakeupPathQueryWindowEndTs:         wakeupPathElected.windowEnd,
		SupportingHops:                     hops,
		WakeupChainRecommendedNotRun:       chainRequiredRecommended && !wakeupChainObserved,
		RootCauseFamilyObserved:            rootCauseFamilyObserved,
		CapacityTruncated:                  capacityTruncated,
		QueryWindows:                       traceCausalProjectionSortQueryWindows(queryWindows),
		QueryWindowsTruncated:              queryWindowsTruncated,
		BusinessSpanMentions:               businessSpanMentions,
		BusinessSpanMentionOmitted:         businessSpanMentionOmitted,
		GatedCompositeEdgeShareDisclosures: gatedCompositeEdgeShares,
		SelfRunnableTwoRulerAccountings:    selfRunnableTwoRulers,
		SelfRunningFoldUnmeasured:          selfRunningFoldUnmeasured,
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
	// RNB-1 D1 修复轮: the context buckets cap by folding AFTER aggregation
	// (count = post-merge truth); runs before the on-chain fold so its
	// seated-data-gap carve reads the FINAL kept stanza rows.
	out.AdjacentCauses = traceCausalProjectionLimitContextNodesFold(out.AdjacentCauses, traceCausalProjectionContextBucketLimit, "adjacent")
	out.BackgroundCauses = traceCausalProjectionLimitContextNodesFold(out.BackgroundCauses, traceCausalProjectionContextBucketLimit, "background")
	out.OnChainCauses = traceCausalProjectionLimitNodesOnChainFold(out.OnChainCauses, traceCausalProjectionOnChainLimit,
		traceCausalProjectionSeatedDataGapSubjects(out.AdjacentCauses, out.BackgroundCauses),
		out.PrimaryRootCauses, out.SupportingHops)
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
	// §29.27② (COV-4): attach the target four-state account AFTER the anchor
	// window resolution — only the candidate whose typed selected_window
	// matches the resolved anchor within the F-2 tolerance attaches (ties:
	// largest TotalMS, deterministic on record order independence).
	traceCausalProjectionAttachTargetStateAccount(&out, targetStateAccounts)
	traceCausalProjectionAttachSleepDrilldownTargets(&out, wakeupEdges, wakeupPath)
	if !out.Active() {
		return TraceCausalProjection{}
	}
	return out
}

// traceCausalProjectionContextOnly is the compile-side admission gate for a
// root_cause_* record on the typed non-ranking context lane. Causal-hop
// families may carry the same tier, but must keep their CausalHop role and
// SupportingHops edge semantics, so this shortcut deliberately excludes them.
func traceCausalProjectionContextOnly(record ObservationRecord) bool {
	return traceCausalProjectionIsRootCauseContext(record) &&
		strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyTier)) == TraceCausalTierContextOnly
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
	Waker            string
	Wakee            string
	EvidenceID       string
	Relation         string
	WakeupPointKnown bool
	WakeupTs         float64
	WakeupLine       int
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
	edge := traceCausalProjectionWakeupEdge{
		Waker:      waker,
		Wakee:      wakee,
		EvidenceID: strings.TrimSpace(record.ID),
		Relation:   "wakeup_chain_edge",
	}
	// Positive timestamps have an unambiguous presence signal in the typed
	// span. At exactly trace-zero, JSON zero values cannot distinguish
	// present from omitted, so require the producer's registered wakeup_ts=0
	// note as the presence witness; a line-only record must not fabricate 0s.
	zeroPointWitnessed := false
	if raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyWakeupTs)); raw != "" {
		if value, err := strconv.ParseFloat(raw, 64); err == nil && value == 0 {
			zeroPointWitnessed = true
		}
	}
	if (record.Span.StartTs != 0 || record.Span.EndTs != 0 || zeroPointWitnessed) &&
		!math.IsNaN(record.Span.StartTs) && !math.IsInf(record.Span.StartTs, 0) &&
		record.Span.StartTs >= 0 {
		edge.WakeupPointKnown = true
		edge.WakeupTs = record.Span.StartTs
		edge.WakeupLine = record.Span.LineStart
	}
	return edge, true
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
	capabilitySource string
	// capabilityFreqOnlyReason (CLUSTER-FIX-2 件1, S1) travels with its
	// caliber token: the twin's wording fork must match its donor's.
	capabilityFreqOnlyReason string
	referenceClass           string
	topologySource           string
	thermalCapKHz            int
	thermalCapWitnessed      bool
	windowStart, windowEnd   float64
	windowDeclared           bool
	conflict                 bool
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
				capabilitySource:         node.SupplyFoldCapabilitySource,
				capabilityFreqOnlyReason: node.SupplyFoldCapabilityFreqOnlyReason,
				referenceClass:           node.SupplyFoldReferenceClass,
				topologySource:           node.SupplyFoldTopologySource,
				thermalCapKHz:            node.ThermalCapKHz,
				thermalCapWitnessed:      node.ThermalCapWitnessed,
				windowStart:              node.QueryWindowStartTs,
				windowEnd:                node.QueryWindowEndTs,
				windowDeclared:           TraceCausalProjectionWindowPresent(node.QueryWindowStartTs, node.QueryWindowEndTs),
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
			if donor.windowDeclared && TraceCausalProjectionWindowPresent(node.QueryWindowStartTs, node.QueryWindowEndTs) &&
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
			node.SupplyFoldCapabilityFreqOnlyReason = donor.capabilityFreqOnlyReason
			node.SupplyFoldReferenceClass = donor.referenceClass
			node.SupplyFoldTopologySource = donor.topologySource
			node.ThermalCapKHz = donor.thermalCapKHz
			node.ThermalCapWitnessed = donor.thermalCapWitnessed
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
		if !TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
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

// traceCausalProjectionTargetStateCandidate is one collected
// target_window_states record (§29.27② COV-4) before the anchor-window
// admission in traceCausalProjectionAttachTargetStateAccount.
type traceCausalProjectionTargetStateCandidate struct {
	Account     TraceCausalProjectionTargetStateAccount
	WindowStart float64
	WindowEnd   float64
}

// traceCausalProjectionTargetStateCandidateFromRecord parses one typed
// target_window_states record. ok=false when the record has no subject, no
// positive per-state data, or no parseable selected_window (a partition that
// cannot state its own window makes no window claim — 禁猜).
func traceCausalProjectionTargetStateCandidateFromRecord(record ObservationRecord) (traceCausalProjectionTargetStateCandidate, bool) {
	subject := strings.TrimSpace(record.Subject)
	if subject == "" {
		return traceCausalProjectionTargetStateCandidate{}, false
	}
	ws, we, ok := traceCausalProjectionSelectedWindowNote(record.RichNotes)
	if !ok {
		return traceCausalProjectionTargetStateCandidate{}, false
	}
	account := TraceCausalProjectionTargetStateAccount{
		Subject:                subject,
		RunningMS:              traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyRunning),
		RunnableMS:             traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyRunnable),
		SleepMS:                traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeySleep),
		DStateMS:               traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyDState),
		IOWaitMS:               traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyIOWait),
		SleepIOWaitMS:          traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeySleepIOWait),
		TotalMS:                traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyTotal),
		DeterministicRunningMS: traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyDeterministicRunning),
		HeadCarryMS:            traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyHeadCarryMS),
		HeadCarryState:         strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHeadCarryState)),
		TailOpenMS:             traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyTailOpenMS),
		TailOpenState:          strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyTailOpenState)),
		WindowStartTs:          ws,
		WindowEndTs:            we,
		EvidenceID:             record.ID,
	}
	if account.TotalMS <= 0 {
		return traceCausalProjectionTargetStateCandidate{}, false
	}
	return traceCausalProjectionTargetStateCandidate{Account: account, WindowStart: ws, WindowEnd: we}, true
}

// traceCausalProjectionAttachTargetStateAccount admits the ONE candidate
// whose typed selected_window matches the resolved anchor window within the
// F-2 ±1ms tolerance (both endpoints). Ties (same window re-queried) resolve
// to the largest TotalMS — deterministic on record order independence (RN-12
// precedent). A projection without an anchor window attaches nothing (禁猜).
func traceCausalProjectionAttachTargetStateAccount(projection *TraceCausalProjection, candidates []traceCausalProjectionTargetStateCandidate) {
	if projection == nil || len(candidates) == 0 {
		return
	}
	if !TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
		return
	}
	var chosen *traceCausalProjectionTargetStateCandidate
	for i := range candidates {
		candidate := &candidates[i]
		if math.Abs(candidate.WindowStart-projection.WindowStartTs) > traceCausalProjectionFullWindowSameWindowToleranceS ||
			math.Abs(candidate.WindowEnd-projection.WindowEndTs) > traceCausalProjectionFullWindowSameWindowToleranceS {
			continue
		}
		if chosen == nil || candidate.Account.TotalMS > chosen.Account.TotalMS {
			chosen = candidate
		}
	}
	if chosen == nil {
		return
	}
	account := chosen.Account
	projection.TargetStateAccount = &account
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
	Target              string
	EvidenceID          string
	Relation            string
	Ambiguous           bool
	WakeupPointKnown    bool
	WakeupTs            float64
	WakeupLine          int
	WakeupPointConflict bool
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
		candidate := traceCausalProjectionDrilldownTarget{
			Target:     strings.TrimSpace(waker),
			EvidenceID: strings.TrimSpace(evidenceID),
			Relation:   strings.TrimSpace(relation),
		}
		raw[wakeeKey][wakerKey] = candidate
	}
	for _, edge := range edges {
		wakeeKey := traceCausalProjectionCanonicalNode(edge.Wakee)
		wakerKey := traceCausalProjectionCanonicalNode(edge.Waker)
		if wakeeKey == "" || wakerKey == "" {
			continue
		}
		if raw[wakeeKey] == nil {
			raw[wakeeKey] = map[string]traceCausalProjectionDrilldownTarget{}
		}
		candidate := traceCausalProjectionDrilldownTarget{
			Target:           strings.TrimSpace(edge.Waker),
			EvidenceID:       strings.TrimSpace(edge.EvidenceID),
			Relation:         strings.TrimSpace(edge.Relation),
			WakeupPointKnown: edge.WakeupPointKnown,
			WakeupTs:         edge.WakeupTs,
			WakeupLine:       edge.WakeupLine,
		}
		if prior, ok := raw[wakeeKey][wakerKey]; ok {
			switch {
			case prior.WakeupPointConflict:
				candidate.WakeupPointKnown = false
				candidate.WakeupPointConflict = true
				candidate.EvidenceID = ""
				candidate.WakeupTs = 0
				candidate.WakeupLine = 0
			case prior.WakeupPointKnown && candidate.WakeupPointKnown &&
				prior.WakeupTs != candidate.WakeupTs:
				candidate.WakeupPointKnown = false
				candidate.WakeupPointConflict = true
				candidate.EvidenceID = ""
				candidate.WakeupTs = 0
				candidate.WakeupLine = 0
			case prior.WakeupPointKnown && candidate.WakeupPointKnown &&
				prior.WakeupTs == candidate.WakeupTs && prior.WakeupLine != candidate.WakeupLine:
				// The exact time is still authoritative; only the physical
				// line locator is ambiguous across duplicate publications.
				candidate.EvidenceID = ""
				candidate.WakeupLine = 0
			case prior.WakeupPointKnown && !candidate.WakeupPointKnown:
				candidate = prior
			case !prior.WakeupPointKnown && !candidate.WakeupPointKnown:
				// Keep the first deterministic evidence identity instead of
				// making record order choose the last duplicate.
				candidate = prior
			}
		}
		raw[wakeeKey][wakerKey] = candidate
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
	node.DrilldownWakeupPointKnown = target.WakeupPointKnown && !target.WakeupPointConflict
	node.DrilldownWakeupTs = target.WakeupTs
	node.DrilldownWakeupLine = target.WakeupLine
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
		// §29.183 G8: shared existence predicate — a frame anchor Span of
		// [0,end] (rebased trace, explicit 0..X query window) IS the anchor.
		if !TraceCausalProjectionWindowPresent(s, e) {
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
	if !TraceCausalProjectionWindowPresent(start, end) {
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
// end > start >= 0 (§29.183 G8: a rebased trace's window legally starts at
// exactly 0). This is the only CMP-2 fallback-anchor carrier; a malformed or
// absent note yields ok=false and the legacy "起止未采集" behavior.
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
	if errStart != nil || errEnd != nil || !TraceCausalProjectionWindowPresent(start, end) {
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
	// §29.183 G8: shared existence predicate — a node span of [0,end] earns
	// its within-window verdict like any other real span.
	if node == nil || !TraceCausalProjectionWindowPresent(node.StartTs, node.EndTs) {
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
			// rank_family_key itself is the precise engine verdict. G1 stamps it
			// on a same-type family merge; B4 also stamps it on a singleton
			// d_state_or_io_wait row that absorbed one exactly coincident
			// io_burst_episode rank observation. Requiring MemberCount>1 here
			// would make the B4 absorber invisible. No display-side type/value
			// inference is allowed — the verbatim key remains the only join.
			if key := strings.TrimSpace(node.RankFamilyKey); key != "" {
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
		// 件5 (SC-F1): typed provenance carry — the audit face's
		// origin=system_supplement token reads this, never re-derives.
		SystemSupplement: record.SystemSupplement,
	}
	// RANKDIS-EXT A3 (§29.104.16.1 M15, 2026-07-16): the causal `rank` note
	// parses into Node.Rank ONLY for rank-board records — the root_cause_*
	// predicate family (root_cause_<tier> / _background / _absorbed /
	// _data_gap / _caliber_side), the exact family both rank-note producers
	// emit under. Node.Rank>0 is board-seat currency downstream (badges,
	// election, the ◎ §29.112 population arm), so a non-board record that
	// carries a rank-spelled note (the retired state_drilldown borrow; any
	// future leak) must never mint a seat here. The drilldown ordinal now
	// rides its dedicated state_rank display lane.
	if strings.HasPrefix(node.Predicate, "root_cause") {
		node.Rank = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyRank)
	}
	// G2 显示半场 (§27.2/§28.1, 2026-07-09): the typed blind-spot criterion —
	// wording input for the ◇ inline disclosure fork; absent = legacy wording.
	node.TraceGapKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyTraceGapKind))
	// SELF-SEM (§29.61.1, 2026-07-13): the typed on-chain proof basis — the
	// 「目标自身·确定性优化」 display qualifier forks on THIS single field (never a
	// subject∧class∧relevance recomposition); absent = legacy overlap basis.
	node.OnChainBasis = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyOnChainBasis))
	// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16): composite-score rows publish
	// the *_score twin instead of the ms key (one row emits exactly one
	// family) — the union read keeps the caliber-side row values flowing.
	node.CumulativeImpactMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyCumulativeImpactMS, TraceNoteKeyCumulativeImpactScore)
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
	node.IOPressureSignal = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyIOPressureSignal))
	node.IOPressureEvidenceQuality = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyIOPressureEvidenceQuality))
	node.IOPressureScoreCaliber = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyIOPressureScoreCaliber))
	node.IOPressureConclusion = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyIOPressureConclusion))
	node.IOPressureIOWaitBlockedCount = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyIOPressureIOWaitBlockedCount)
	node.IOPressureBlockMaxMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyIOPressureBlockMaxMS)
	node.IOPressureStorageMaxMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyIOPressureStorageMaxMS)
	node.IOPressureFileBytes = traceCausalProjectionRichNoteInt64(record.RichNotes, TraceNoteKeyIOPressureFileBytes)
	node.IOPressureFileEvents = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyIOPressureFileEvents)
	node.IOPressurePageCacheChurn = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyIOPressurePageCacheChurn)
	// DSTATE-REFINE arm a (件③): exact typed boolean + caller symbol.
	node.DStateRefinedNonIO = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyDStateRefinedNonIO)) == "true"
	node.BlockedReasonCaller = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockedReasonCaller))
	// CR-3 件② P10: the unconsumed-marker residual pair (int + symbols).
	if raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockedReasonWindowCount)); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			node.BlockedReasonWindowCount = n
		}
	}
	node.BlockedReasonWindowCaller = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockedReasonWindowCaller))
	// §29.50.5 (v5 P1 批 件②): the proof-partition honest-remainder marker.
	node.DStateCauseUnprovenRemainder = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyDStateCauseUnprovenRemainder)) == "true"
	// RSPA (§29.61.10): the re-anchoring bipartition trio + M-IO closure.
	node.ChainAnchoredMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyChainAnchored)
	node.ChainAnchorFullMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyChainAnchorFull)
	node.ChainAnchorRemainderSeat = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainAnchorRemainderSeat)) == "true"
	// RNB-1 (§29.88 R2/R4): case-A' ownership-divergence trio + the R4
	// whole-seat lane-demotion marker.
	node.ChainAnchorOwnershipDivergent = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainAnchorOwnershipDivergent)) == "true"
	node.ChainAnchorChainLaneMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyChainAnchorChainLane)
	node.ChainAnchorCensusMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyChainAnchorCensus)
	node.ChainCredentialLaneDemoted = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainCredentialLaneDemoted)) == "true"
	// HULL-CRED (§29.104 终判③): the keep-⛓ per-segment credential trio.
	node.ChainCredentialSegments = traceCausalProjectionParseCredentialSegments(
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainCredentialSegments))
	node.ChainCredentialSegmentDisjoint = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainCredentialSegmentDisjoint)) == "true"
	node.ChainCredentialEnvelopeLevel = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainCredentialEnvelopeLevel)) == "true"
	// ONCHAIN-FIX-2 件3 (Q6): the truncated lower-bound prefix marker —
	// meaningful only beside a successfully decoded inventory (claim gated
	// on proof, mirroring the disjoint word's gate).
	node.ChainCredentialSegmentsTruncated = len(node.ChainCredentialSegments) > 0 &&
		strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainCredentialSegmentsTruncated)) == "true"
	// ONCHAIN-FIX-1 件1: the interval-less identity-inheritance admission
	// marker (fail-open keep disclosure; fabricated overlap retired).
	node.ChainIdentityInheritance = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainIdentityInheritance)) == "true"
	// CHAINGUARD-1 件2: the engine census verdict (single strict parser; ""
	// = absent, every consumer keeps the legacy behavior byte-identically).
	node.ChainCredentialCensus = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainCredentialCensus))
	// XLANE-1 件1 (§29.104.2): the represented-by-chain-seat satellite marker.
	node.ChainAnchorRepresentedByChainSeat = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyChainAnchorRepresentedByChainSeat)) == "true"
	// LEVELMERGE-1 件2 (方案 P 区间分账): the gated-share split family.
	node.GatedShareClaimedMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyGatedShareClaimed)
	node.GatedShareFullMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyGatedShareFull)
	node.GatedShareConstituentSeat = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedShareConstituentSeat)) == "true"
	if raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedShareClaimSeats)); raw != "" {
		for _, span := range strings.Split(raw, ",") {
			if span = strings.TrimSpace(span); span != "" {
				node.GatedShareClaimSeats = append(node.GatedShareClaimSeats, span)
			}
		}
	}
	node.GatedShareOverlapDisclosureMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyGatedShareOverlap)
	// PARTSPLIT-1 (§29.150④): the R4-mirror refusal record — the four fields
	// travel together or not at all (atomic engine stamp; a partial set never
	// mints the disclosure — all-or-nothing, 宁漏勿假指).
	{
		pre := traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyGatedCompositeEdgePreShare)
		post := traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyGatedCompositeEdgePostShare)
		anchorTs := traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyGatedCompositeEdgeAnchorTs)
		via := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCompositeEdgeAnchorVia))
		if pre > 0 && post > 0 && anchorTs > 0 && via != "" {
			node.GatedCompositeEdgePreShareMS = pre
			node.GatedCompositeEdgePostShareMS = post
			node.GatedCompositeEdgeAnchorTS = anchorTs
			node.GatedCompositeEdgeAnchorVia = via
		}
	}
	// R3-IMPL (§29.88.1): the host-edge-anchored credential disclosure pair.
	node.HostWakeupEdgeAnchorTS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyHostWakeupEdgeAnchorTs)
	node.HostWakeupEdgeAnchorVia = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHostWakeupEdgeAnchorVia))
	node.ResourceCompletionClosure = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyResourceCompletionClosure)) == "true"
	// RNB-2 件5 AFF-EVID (§29.88.6): the affinity/cpuset judgment payload —
	// the constraint-description inputs (行3/明细).
	node.CPUConstraintKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCPUConstraintKind))
	node.CPUConstraintCPUSet = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCPUConstraintCPUSet))
	node.CPUConstraintCPUSetIsBinding = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCPUConstraintCPUSetIsBinding)) == "true"
	node.CPUConstraintPolicy = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCPUConstraintPolicy))
	node.CPUConstraintAllowedCPUs = traceCausalProjectionRichNoteCPUList(record.RichNotes, TraceNoteKeyCPUConstraintAllowedCPUs)
	node.CPUConstraintExcludedCPUs = traceCausalProjectionRichNoteCPUList(record.RichNotes, TraceNoteKeyCPUConstraintExcludedCPUs)
	node.CPUConstraintAllowedMaxTierKHz = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyCPUConstraintAllowedMaxTierKHz)
	node.CPUConstraintGlobalMaxTierKHz = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyCPUConstraintGlobalMaxTierKHz)
	// CR-3 件③ P11: the process attribution pair (tgid + owning comm).
	if raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyTGID)); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			node.ProcessTGID = n
		}
	}
	node.ProcessComm = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyProcessComm))
	// XLANE-3 件1: the rank board identity triple's target/params halves —
	// verbatim typed notes, absence stays empty (legacy window-only board key).
	node.RankBoardTarget = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyRankBoardTarget))
	node.RankBoardParamsFingerprint = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyRankBoardParams))
	if node.StateKind == "" {
		// Root-cause / hop rows encode the scheduler state as the Object
		// (sleep_wait / running / io_wait / …). Fall back to it ONLY when it is a
		// recognized state word, so the state column stays a real scheduler state
		// and non-state objects (compute_supply, class_verification) leave it empty.
		node.StateKind = traceCausalProjectionCanonicalStateWord(record.Object)
	}
	// RANKDIS-M18: effective_impact_score is the composite-score twin (one row
	// emits exactly one key family).
	node.EffectiveImpactMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyEffectiveImpactMS, TraceNoteKeyEffectiveImpact, TraceNoteKeyEffectiveImpactScore)
	// EPUB (§29.31): the effective-published marker mints HERE and only here —
	// note PRESENCE, never the parsed value (an explicit 0.000 is present).
	// 复核 M1 exemption: the root_evidence: audit family never mints published —
	// its effective_impact_ms=0.000 is a RANKING-EXCLUSION SENTINEL (see the
	// trace_query.go root_evidence emit: the reduced-shape wakeup witness
	// "does not carry CAP/gated/state-union provenance, so only the richer
	// root_cause_rank/causal-impact lanes may participate in ranking"), not an
	// authoritative published zero; minting it would let the R1 same-fact OR
	// arm transplant sentinel authority onto an eff-unpublished ranked
	// survivor and falsely refuse its crown. Precise typed claim-key prefix —
	// the same family signal the hop admission gates read
	// (traceCausalProjectionIsCausalHop / traceCausalProjectionHopOnChain);
	// the emit side keeps its note untouched (other consumers).
	node.EffectiveImpactPublished = !strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_evidence:") &&
		traceCausalProjectionRichNoteAnyPresent(record.RichNotes, TraceNoteKeyEffectiveImpactMS, TraceNoteKeyEffectiveImpact, TraceNoteKeyEffectiveImpactScore)
	node.ActualImpactMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyActualImpactMS, TraceNoteKeyActualImpact)
	// CR-2 组③ P7 (2026-07-12): the actual channel's physical interval — the
	// same strict window parser as every window-valued note (malformed notes
	// leave the pair zero, never a fabricated interval).
	if raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyActualWindow)); raw != "" {
		if start, end, ok := TraceCausalProjectionParseWindowValue(raw); ok {
			node.ActualWindowStartTs, node.ActualWindowEndTs = start, end
		}
	}
	// 审计 #5/#62 (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): the
	// on-chain semantic-span intersection participation — promoted from the
	// producer's projected_impact (family) / overlap (single-span) notes,
	// gated to trace_semantic_span records on the on_chain lane only (rank
	// rows and off-chain semantic rows keep the zero fail-open).
	if strings.TrimSpace(record.Predicate) == "trace_semantic_span" && node.ChainRelevance == "on_chain" {
		node.SemanticChainProjectedMS = traceCausalProjectionRichNoteFirstFloat(record.RichNotes, TraceNoteKeyProjectedImpact, TraceNoteKeyOverlap)
	}
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
		// LOCKNS-FIX 修补 件A (2026-07-16): the typed presence verdict rides
		// beside the raw tid (持有者来历 presence 分句 fork; absence keeps
		// the legacy sentence byte-identically).
		node.BlockingOwnerTidPresence = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyOwnerTidPresence))
		node.BlockingHolderHandoff = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHolderHandoff))
		node.BlockingHolderContradiction = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHolderSelfContradiction))
		// G10-EN 根修 (QH2-A, 2026-07-14): the typed witness components ride
		// beside the zh string (per-lane wording source; nil on legacy
		// records keeps the verbatim fallback).
		node.BlockingHolderContradictionParts = traceCausalProjectionParseHolderSelfContradiction(record.RichNotes)
		// LOCKNS-FIX 件6 / OM-10 关账 (§29.104.12, 2026-07-16): the ②×③
		// identity-unification declaration reaches the 持有者来历 detail line.
		node.BlockingHolderNsUnification = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyHolderNsUnification))
	}
	// XERR1-FIX 件1/件3 (§29.104.3/.4, 2026-07-15): the payload-less
	// blocking_span value-basis + budget carriage — parsed OUTSIDE the
	// BlockingKind gate above (the basis mints exactly on BlockingKind==""
	// rows; the budget trio also rides holder-subject rank records via the
	// twin-port lane). Absence keeps every legacy word face byte-identically.
	node.BlockingValueBasis = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockingValueBasis))
	node.BlockingWaitSegmentMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyBlockingWaitSegmentMS)
	node.BlockingWaitSleepMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyBlockingWaitSleepMS)
	node.BlockingSpanEnvelopeMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyBlockingSpanEnvelopeMS)
	node.BlockingWaitBudgetExceeded = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockingWaitBudgetExceeded)) == "true"
	node.BlockingWaitBudgetNonRunningMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyBlockingWaitBudgetNonRunningMS)
	node.BlockingWaitBudgetRunningMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyBlockingWaitBudgetRunningMS)
	// 件F (2026-07-16): the partial-coverage lower-bound disclosure pair.
	node.BlockingWaitCoveragePartial = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockingWaitCoveragePartial)) == "true"
	node.BlockingWaitAccountCoveredMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyBlockingWaitAccountCoveredMS)
	// LOCKNS-FIX 件3 (§29.104.12, 2026-07-16): the unknown-morphology fail-open
	// marker — parsed OUTSIDE the BlockingKind gate (it mints exactly on
	// payload-less rows). Disclosure input only.
	node.BlockingOwnerKeyUnregistered = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBlockingOwnerKeyUnregistered)) == "true"
	// §7.30.3 D3: gated-impact composition for priority-inversion rows.
	node.GatedRunnableMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyGatedRunnable)
	node.GatedRunningDeficitMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyGatedRunningDeficit)
	// CAP (§26 C3): typed capability caliber of the discounted running
	// component — exact typed note match, wording input only. CAP-2: the
	// cluster-topology source rides beside it.
	node.GatedCapabilitySource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCapability))
	node.GatedTopologySource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedClusterTopology))
	// DISPHYG-3 件7: the gated freq_only cause token rides beside them.
	node.GatedCapabilityFreqOnlyReason = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedFreqOnlyReason))
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
		// GAP-B2 复核修 (2026-07-25): the D∧timer wording-fork credential —
		// exact typed note match, consumed only when the periodic stamp is
		// present (a stray note on a non-periodic row carries no claim).
		node.PeriodicTimerCaller = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyTimerWaitCaller))
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
		// CLUSTER-FIX-2 件1 (S1): the typed freq_only cause token rides the
		// same presence gate (emitted only beside a freq_only caliber).
		node.SupplyFoldCapabilityFreqOnlyReason = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldCapabilityFreqOnlyReason))
		// CAP 复核 F1: the demoted basis class (absent = big-class basis).
		node.SupplyFoldReferenceClass = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldReferenceClass))
		// CAP-2 (§28.4/§28.5): cluster-structure source (absent = explicit/
		// legacy — the default-table wording stands byte-identically) and the
		// THERM in-window press disclosure.
		node.SupplyFoldTopologySource = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldClusterTopology))
		node.ThermalCapKHz = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyThermalCapKHz)
		// CR-3 件⑥ F-10: the cap's in-window witness bit (冷读 D5) — read
		// only beside its value.
		node.ThermalCapWitnessed = node.ThermalCapKHz > 0 &&
			strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyThermalCapWitnessed)) == "true"
	}
	node.RunnableMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyRunnable)
	// WO-A1 (SMR-1 批, 2026-07-12): the d/io per-state split — the same
	// already-emitted note family as RunnableMS above, one more consumer.
	node.DStateSplitMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyDState)
	node.IOWaitSplitMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyIOWait)
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
		// RNB-5B 件⑥: the typed wire-fold source bit — set ONLY on this
		// folded_* re-materialization (the engine self-published take-MAX
		// fold); display-side merges never mint it.
		node.MergedWireFold = true
		node.MergedMinMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyFoldedMinMS)
		node.MergedMaxMS = traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyFoldedMaxMS)
		for _, subject := range strings.Split(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldedSubjects), ",") {
			traceCausalProjectionAppendMergedSubject(&node, subject)
		}
		// A2 件5② (§29.179 委托, 2026-07-21): re-materialize the wire-fold
		// max-member identity into the SAME carriers the display-side folds
		// mint (RUN2FIX-A 件2), all-or-nothing on the subject (宁漏勿假 — a
		// state without a subject claims nothing).
		if maxSubject := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldedMaxSubject)); maxSubject != "" {
			node.MergedMaxSubject = maxSubject
			node.MergedMaxStateKind = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFoldedMaxStateKind))
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
		// XLANE-2 件1: the complete typed member line-range set — strict
		// all-or-nothing parse against the family member count.
		node.FamilyMemberLineRanges = traceCausalProjectionParseMemberLineRanges(
			traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyMemberLineRanges), familyCount)
		// SPANTOP-1 件1: the complete typed per-member wall-clock list — same
		// strict all-or-nothing parse against the family member count.
		node.FamilyMemberWallMS = traceCausalProjectionParseMemberWallMS(
			traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyMemberWallMS), familyCount)
	}
	// XLANE-2 件2: the self-gap seat's semantic-overlap disclosure roster
	// (per-entry independent parse; empty on every other row).
	node.SelfGapSemanticOverlaps = traceCausalProjectionParseSelfGapSemanticOverlaps(
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySelfGapSemanticOverlaps))
	// AXIOM-V2 (2026-07-18): the registry fix-direction attribute and the
	// cross-direction overlap pair roster (件1/件2; per-entry independent
	// parse — each clause is its own truth).
	node.FixDirection = strings.TrimSpace(
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyFixDirection))
	node.CrossDirectionOverlaps = traceCausalProjectionParseCrossDirectionOverlaps(
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCrossDirectionOverlaps))
	// ELIM-V2 守恒尾行 (2026-07-18): the 件3 conservation violation finding —
	// strict whole-tuple parse (a partial tuple could fake a violation claim;
	// absence never judges).
	node.DirectionConservationExcess = traceCausalProjectionParseDirectionConservation(
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyDirectionConservationExcess))
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

// TraceCausalProjectionSelfGapSemanticOverlap is one parsed entry of the
// XLANE-2 件2 self-gap semantic-overlap disclosure roster.
type TraceCausalProjectionSelfGapSemanticOverlap struct {
	OverlapMS float64 `json:"overlap_ms"`
	LineStart int     `json:"line_start"`
	LineEnd   int     `json:"line_end"`
}

// TraceCausalProjectionSelfGapSemanticOverlapCap mirrors the engine emission
// cap (selfGapSemanticOverlapPartnerCap — the equality is pinned tool-side
// where both packages are visible).
const TraceCausalProjectionSelfGapSemanticOverlapCap = 6

// traceCausalProjectionParseSelfGapSemanticOverlaps parses the typed
// self_gap_semantic_overlaps note ("overlapMs@lineStart..lineEnd" joined with
// "|" — single producer format, traceQuerySelfGapSemanticOverlapsNote).
// Entries parse INDEPENDENTLY (each clause is its own truth): an invalid
// entry drops, never guesses; nothing valid → nil.
func traceCausalProjectionParseSelfGapSemanticOverlaps(raw string) []TraceCausalProjectionSelfGapSemanticOverlap {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []TraceCausalProjectionSelfGapSemanticOverlap
	for _, entry := range strings.Split(raw, "|") {
		msRaw, rangeRaw, ok := strings.Cut(strings.TrimSpace(entry), "@")
		if !ok {
			continue
		}
		ms := traceCausalProjectionFloat(strings.TrimSpace(msRaw))
		startRaw, endRaw, ok := strings.Cut(strings.TrimSpace(rangeRaw), "..")
		if !ok || ms <= 0 {
			continue
		}
		start, errStart := strconv.Atoi(strings.TrimSpace(startRaw))
		end, errEnd := strconv.Atoi(strings.TrimSpace(endRaw))
		if errStart != nil || errEnd != nil || start <= 0 || end < start {
			continue
		}
		if len(out) < TraceCausalProjectionSelfGapSemanticOverlapCap {
			out = append(out, TraceCausalProjectionSelfGapSemanticOverlap{OverlapMS: ms, LineStart: start, LineEnd: end})
		}
	}
	return out
}

// TraceCausalProjectionCrossDirectionOverlap is one parsed entry of the
// AXIOM-V2 件2 cross-direction overlap disclosure (see the node field).
type TraceCausalProjectionCrossDirectionOverlap struct {
	OverlapMS float64
	LineStart int
	LineEnd   int
	Direction string
	Basis     string
}

// TraceCausalProjectionCrossDirectionOverlapCap mirrors the engine emission
// cap (tracequery.RootCauseCrossDirectionOverlapPartnerCap) — pinned equal by
// the tool-side mirror test.
const TraceCausalProjectionCrossDirectionOverlapCap = 6

// traceCausalProjectionParseCrossDirectionOverlaps parses the typed
// cross_direction_overlaps note ("overlapMs@lineStart..lineEnd@direction@basis"
// joined with "|" — single producer format,
// traceQueryCrossDirectionOverlapsNote). Entries parse INDEPENDENTLY (each
// clause is its own truth): an invalid entry drops, never guesses.
func traceCausalProjectionParseCrossDirectionOverlaps(raw string) []TraceCausalProjectionCrossDirectionOverlap {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []TraceCausalProjectionCrossDirectionOverlap
	for _, entry := range strings.Split(raw, "|") {
		parts := strings.Split(strings.TrimSpace(entry), "@")
		if len(parts) != 4 {
			continue
		}
		ms := traceCausalProjectionFloat(strings.TrimSpace(parts[0]))
		startRaw, endRaw, ok := strings.Cut(strings.TrimSpace(parts[1]), "..")
		if !ok || ms <= 0 {
			continue
		}
		start, errStart := strconv.Atoi(strings.TrimSpace(startRaw))
		end, errEnd := strconv.Atoi(strings.TrimSpace(endRaw))
		if errStart != nil || errEnd != nil || start <= 0 || end < start {
			continue
		}
		direction := strings.TrimSpace(parts[2])
		basis := strings.TrimSpace(parts[3])
		if direction == "" || basis == "" {
			continue
		}
		if len(out) < TraceCausalProjectionCrossDirectionOverlapCap {
			out = append(out, TraceCausalProjectionCrossDirectionOverlap{
				OverlapMS: ms, LineStart: start, LineEnd: end,
				Direction: direction, Basis: basis,
			})
		}
	}
	return out
}

// TraceCausalProjectionDirectionConservation is the parsed AXIOM-V2 件3
// conservation violation finding (ELIM-V2 守恒尾行 consumer): within one
// (thread, direction) strict on-chain full-seat population the Σ of per-seat
// support-interval union lengths exceeded the physical window. Disclosure
// transcription only.
// The json tags deliberately avoid the registered note-key spellings
// (window_ms / seat_count are live note keys — the notekeys census reads
// quoted-literal references, and this struct is a display parse artifact,
// not a note consumer of those keys).
type TraceCausalProjectionDirectionConservation struct {
	Direction string  `json:"direction,omitempty"`
	SumMS     float64 `json:"conservation_sum_ms,omitempty"`
	WindowMS  float64 `json:"conservation_window_ms,omitempty"`
	SeatCount int     `json:"conservation_seat_count,omitempty"`
}

// traceCausalProjectionParseDirectionConservation parses the typed
// direction_conservation_excess note ("direction@sumMs@windowMs@seatCount" —
// single producer format, traceQueryDirectionConservationNote). STRICT
// whole-tuple parse: every field must decode with sum > window > 0 and
// seatCount ≥ 2 (the engine only mints the finding on that shape) — anything
// else returns nil (a partial tuple could fake a violation; absence never
// judges).
func traceCausalProjectionParseDirectionConservation(raw string) *TraceCausalProjectionDirectionConservation {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "@")
	if len(parts) != 4 {
		return nil
	}
	direction := strings.TrimSpace(parts[0])
	sum := traceCausalProjectionFloat(strings.TrimSpace(parts[1]))
	window := traceCausalProjectionFloat(strings.TrimSpace(parts[2]))
	seats, err := strconv.Atoi(strings.TrimSpace(parts[3]))
	if direction == "" || err != nil || seats < 2 || window <= 0 || sum <= window {
		return nil
	}
	// 修补轮 件6②: ParseFloat accepts "NaN"/"Inf" spellings and NaN escapes
	// every ordering comparison above — a non-finite field can never mint a
	// violation finding (the engine only publishes finite ms).
	if math.IsNaN(sum) || math.IsInf(sum, 0) || math.IsNaN(window) || math.IsInf(window, 0) {
		return nil
	}
	return &TraceCausalProjectionDirectionConservation{
		Direction: direction, SumMS: sum, WindowMS: window, SeatCount: seats,
	}
}

// traceCausalProjectionParseMemberLineRanges parses the typed
// member_line_ranges note value ("start..end" entries joined with "|" —
// single producer format, SemanticSpanFamily.MemberLineRangeEntries). STRICT
// all-or-nothing (XLANE-2 件1): every entry must parse with start ≥ 1 and
// end ≥ start, and the entry count must equal the family member count —
// anything else returns nil (a partial set could fake a member-subset
// verdict; absence never judges).
func traceCausalProjectionParseMemberLineRanges(raw string, memberCount int) [][2]int {
	raw = strings.TrimSpace(raw)
	if raw == "" || memberCount <= 1 {
		return nil
	}
	parts := strings.Split(raw, "|")
	if len(parts) != memberCount {
		return nil
	}
	out := make([][2]int, 0, len(parts))
	for _, part := range parts {
		startRaw, endRaw, ok := strings.Cut(strings.TrimSpace(part), "..")
		if !ok {
			return nil
		}
		start, errStart := strconv.Atoi(strings.TrimSpace(startRaw))
		end, errEnd := strconv.Atoi(strings.TrimSpace(endRaw))
		if errStart != nil || errEnd != nil || start <= 0 || end < start {
			return nil
		}
		out = append(out, [2]int{start, end})
	}
	return out
}

// traceCausalProjectionParseMemberWallMS parses the typed member_wall_ms note
// value ("%.3f" per-member durations joined with "|" — single producer
// format, MemberWallMsEntries, same member order as member_line_ranges).
// STRICT all-or-nothing (SPANTOP-1 件1, same discipline as the line-range
// parser): the entry count must equal the family member count and every entry
// must decode to a positive float — anything else returns nil (a partial list
// could fake a member decomposition; absence never judges).
func traceCausalProjectionParseMemberWallMS(raw string, memberCount int) []float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || memberCount <= 1 {
		return nil
	}
	parts := strings.Split(raw, "|")
	if len(parts) != memberCount {
		return nil
	}
	out := make([]float64, 0, len(parts))
	for _, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || v <= 0 {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// TraceCausalProjectionChainCredentialSegmentCap mirrors the engine-side
// CriticalBlockingCredentialSegmentCap (HULL-CRED, §29.104 终判③; types
// cannot import tracequery, so the equality is pinned in internal/tool where
// both packages are visible). A decoded set beyond this cap is rejected whole
// — the engine never mints one, so an oversized set can only be a corrupt or
// foreign artifact and must not adjudicate anything.
const TraceCausalProjectionChainCredentialSegmentCap = 32

// traceCausalProjectionParseCredentialSegments parses the typed
// chain_credential_segments note value ("start..end" seconds entries joined
// with "|" — single producer format,
// criticalBlockingCredentialSegmentEntries). STRICT all-or-nothing
// (HULL-CRED): every entry must parse with start >= 0 and end > start, and the
// set must stay within the engine-mirrored cap — anything else returns nil (a
// partial or corrupt inventory could fake a per-segment adjudication; absence
// never judges). §29.183 G8 boundary ruling: the all-or-nothing INTEGRITY
// semantics live in the parse errors, the end>start arm and the whole-set
// nil-out — not in the start>0 boundary; a segment starting at exactly ts=0
// is a legal timestamp in a rebased trace, and rejecting the WHOLE inventory
// over it was the same silent-loss disease (a zero-filled corrupt entry
// "0..0" still nils the set via end>start).
func traceCausalProjectionParseCredentialSegments(raw string) [][2]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "|")
	if len(parts) > TraceCausalProjectionChainCredentialSegmentCap {
		return nil
	}
	out := make([][2]float64, 0, len(parts))
	for _, part := range parts {
		startRaw, endRaw, ok := strings.Cut(strings.TrimSpace(part), "..")
		if !ok {
			return nil
		}
		start, errStart := strconv.ParseFloat(strings.TrimSpace(startRaw), 64)
		end, errEnd := strconv.ParseFloat(strings.TrimSpace(endRaw), 64)
		if errStart != nil || errEnd != nil || !TraceCausalProjectionWindowPresent(start, end) {
			return nil
		}
		out = append(out, [2]float64{start, end})
	}
	return out
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

// traceCausalProjectionParseHolderSelfContradiction (G10-EN 根修, QH2-A
// 2026-07-14) assembles the typed witness components from the
// holder_self_contradiction_* note quintet. All five components must parse
// (positive tid/durations, a sane line range) or the whole set yields nil —
// the display lanes then fall back to the legacy verbatim string; absence
// never guesses a component.
func traceCausalProjectionParseHolderSelfContradiction(notes []string) *TraceHolderSelfContradictionWitness {
	holder := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, TraceNoteKeyHolderSelfContradictionHolder))
	if holder == "" {
		return nil
	}
	ownerTid := traceCausalProjectionRichNoteInt(notes, TraceNoteKeyHolderSelfContradictionOwnerTid)
	queuedMs := traceCausalProjectionRichNoteFloat(notes, TraceNoteKeyHolderSelfContradictionQueuedMs)
	spanMs := traceCausalProjectionRichNoteFloat(notes, TraceNoteKeyHolderSelfContradictionSpanMs)
	lines := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, TraceNoteKeyHolderSelfContradictionLines))
	startRaw, endRaw, ok := strings.Cut(lines, "-")
	if !ok || ownerTid <= 0 || queuedMs <= 0 || spanMs <= 0 {
		return nil
	}
	lineStart, errStart := strconv.Atoi(strings.TrimSpace(startRaw))
	lineEnd, errEnd := strconv.Atoi(strings.TrimSpace(endRaw))
	if errStart != nil || errEnd != nil || lineStart <= 0 || lineEnd < lineStart {
		return nil
	}
	return &TraceHolderSelfContradictionWitness{
		Holder:    holder,
		OwnerTid:  ownerTid,
		QueuedMs:  queuedMs,
		SpanMs:    spanMs,
		LineStart: lineStart,
		LineEnd:   lineEnd,
	}
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

// traceCausalProjectionRichNoteCPUList parses a comma-joined CPU-id note
// (RNB-2 件5 AFF-EVID: cpu_constraint_allowed_cpus / _excluded_cpus) into a
// sorted int list. Malformed members are dropped (absence never guesses);
// nil when the note is absent or empty.
func traceCausalProjectionRichNoteCPUList(notes []string, key string) []int {
	raw := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, key))
	if raw == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(raw, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n >= 0 {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// traceCausalProjectionRichNoteAnyPresent reports whether ANY of the keys is
// present with a non-empty value — the note-PRESENCE half FirstFloat drops
// (its positive-only scan cannot tell an explicit 0.000 from an absent note).
// EPUB (§29.31): the effective-published marker mint reads this.
func traceCausalProjectionRichNoteAnyPresent(notes []string, keys ...string) bool {
	for _, key := range keys {
		if traceCausalProjectionRichNoteValue(notes, key) != "" {
			return true
		}
	}
	return false
}

// traceCausalProjectionWindow parses a "window" RichNote of the form
// "%.6f..%.6f" (as emitted by trace_query.go traceQueryWindowValue /
// traceQueryTypedTimeWindow) into a start/end pair. ok is true only when the
// pair passes the shared existence predicate (end > start >= 0, §29.183 G8 —
// the wire already carries "0.000000..X" for rebased [0,end] windows; only
// this consumer used to reject it).
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
	if !TraceCausalProjectionWindowPresent(start, end) {
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
		strings.TrimSpace(node.Causality) == "on_dependency_chain" ||
		// SELF-SEM (§29.61.1) / SELF-ALL (§29.61.2): self-basis on-chain rows.
		strings.TrimSpace(node.Causality) == "self_deterministic" ||
		strings.TrimSpace(node.Causality) == "self_wall_clock"
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

// traceCausalProjectionContextValueSeat — RNB-1 D1 修复轮 (2026-07-14): the
// value-bearing seat class of the ◇/▒ context buckets — a bipartition ◇
// remainder seat, an R4 credential-demoted seat, or an ordinal-seated context
// row (邻近影响#N — engine Rank>0). These compete by VALUE for the bucket
// seats; plain context rows (in-path sleep/context faces) keep their legacy
// relative order BEHIND them and may no longer unconditionally preempt.
func traceCausalProjectionContextValueSeat(node TraceCausalProjectionNode) bool {
	return node.ChainAnchorRemainderSeat || node.ChainCredentialLaneDemoted ||
		node.ChainAnchorRepresentedByChainSeat || node.Rank > 0
}

// traceCausalProjectionSortContextBucket applies the two-class context-bucket
// order: value seats first (value order), context rows after (legacy
// classified order — the hop comparator).
func traceCausalProjectionSortContextBucket(nodes []TraceCausalProjectionNode, pathIndex map[string]int) []TraceCausalProjectionNode {
	sort.SliceStable(nodes, func(i, j int) bool {
		aSeat, bSeat := traceCausalProjectionContextValueSeat(nodes[i]), traceCausalProjectionContextValueSeat(nodes[j])
		if aSeat != bSeat {
			return aSeat
		}
		if aSeat {
			return traceCausalProjectionNodeLess(nodes[i], nodes[j])
		}
		// Non-seat context rows keep the legacy classified order (role tier
		// then the hop comparator) so their relative order is unchanged from
		// the pre-D1 bucket both at compile assembly and at the
		// post-aggregation resort (one comparator, two call sites).
		return traceCausalProjectionClassifiedLess(nodes[i], nodes[j], pathIndex)
	})
	return nodes
}

// traceCausalProjectionLimitContextNodesFold — RNB-1 D1 修复轮: the zero-
// silent-drop cap for the ◇/▒ context buckets. Rows beyond the cap fold into
// ONE counted subjectless row (the shared fold constructor: member MAX value,
// min–max range, roster, every member evidence id absorbed) seated in the
// SAME bucket — the display's existing stanza-fold form (subjectless ∧
// MergedCount>1 ∧ !OnChainOverflowFold) renders it with the 邻近─/背景─ lane
// word. ≤limit inputs return byte-identical to the plain limiter.
func traceCausalProjectionLimitContextNodesFold(nodes []TraceCausalProjectionNode, limit int, relevance string) []TraceCausalProjectionNode {
	// RNB-5B 件② (§29.96.2 终判②, 2026-07-15): the target-self ⌗ side-rail
	// rows (typed self_caliber_side token) never enter the cap/fold population
	// — folding one published its count-equivalent value as the fold's bare-ms
	// wall-clock MAX (the 17267 production witness), and the ⌗ row's display
	// obligation is unconditional (零静默消失). They re-append after the fold
	// unconditionally; the cap applies to the channel rows only.
	var sideRail, capped []TraceCausalProjectionNode
	for _, node := range nodes {
		if strings.TrimSpace(node.ChainRelevance) == "self_caliber_side" {
			sideRail = append(sideRail, node)
			continue
		}
		capped = append(capped, node)
	}
	if limit <= 0 || len(capped) == 0 || len(capped) <= limit {
		return append(traceCausalProjectionLimitNodes(capped, limit), sideRail...)
	}
	kept := append([]TraceCausalProjectionNode(nil), capped[:limit]...)
	fold := traceCausalProjectionOverflowFoldRow(capped[limit:])
	fold.ChainRelevance = relevance
	fold.OnChainOverflowFold = false
	return append(append(kept, fold), sideRail...)
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

// traceCausalProjectionSelectSemanticSpans preserves the compiler's stable
// semantic ordering while applying capacity only to OFF-CHAIN detail. Every
// typed on-chain semantic node survives: it is both a deterministic
// optimization point and a root-cause candidate, so a projection-level
// nodes[:16] cut would silently remove causal facts before report rendering.
// Off-chain rows remain bounded and can still appear in the background
// context bucket under its independent capacity policy.
func traceCausalProjectionSelectSemanticSpans(nodes []TraceCausalProjectionNode, offChainLimit int) []TraceCausalProjectionNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]TraceCausalProjectionNode, 0, len(nodes))
	offChain := 0
	for _, node := range nodes {
		if traceCausalProjectionNodeOnChain(node) {
			out = append(out, node)
			continue
		}
		if offChainLimit <= 0 || offChain >= offChainLimit {
			continue
		}
		out = append(out, node)
		offChain++
	}
	return out
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
// WO-D1③ host domain (96717/2609 复放追修, 2026-07-12): the flat
// re-publication's RENDERED host can live in a SIBLING chain-universe bucket
// (the 42.131 trunk sleep hop sits in SupportingHops while its flat copy
// overflows the on-chain cap) — extraHosts passes the other rendered buckets
// as additional absorption hosts (mutated in place: the E# joins that row's
// bracket).
func traceCausalProjectionLimitNodesOnChainFold(nodes []TraceCausalProjectionNode, limit int, seatedDataGapSubjects map[string]bool, extraHosts ...[]TraceCausalProjectionNode) []TraceCausalProjectionNode {
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
		// CR-2 组① P4 徽章-图例闭合 (§29.42 P4, witness 冷读 F-6 2026-07-12):
		// a published TOP-5 seat row NEVER folds — the fold roster wears no
		// badge and takes no ordinal, so folding seat #2 deleted ➋ from every
		// render surface while the legend kept promising ➊..➎ (donghu
		// JankManager 16.687ms swallowed by 「其余 7 项(链上折叠)」). 持席行
		// (typed engine Rank ∈ 1..TopN) is the v5 E.3 永不折叠白名单 realized
		// at the compile fold: it stays an individual row after the kept
		// block, and the fold count honestly shrinks (same accounting rule as
		// the G2 carve-out above).
		if traceCausalProjectionSeatFoldExempt(member) {
			kept = append(kept, member)
			continue
		}
		// WO-D1③ 归属检 (SMR-1 批 SMR-S9, smr_audit_report §②, 2026-07-12;
		// witnesses 42729 E18(+5) / 56643 E20(+5): the「其余N项(链上折叠)」
		// headline republished an already-RENDERED row's value to the µs
		// (42.131 ×3发) and drew an extra 37% ghost bar — the value-mirror
		// display arm is structurally unreachable for MergedCount>1 fold
		// rows, so the membership check runs HERE, before the pool seats).
		// A pool candidate whose value-mirror fingerprint (canonical subject
		// + state + µs display AND cumulative + query window — the SAME
		// fingerprint the display arm keys on, one more consumer, never a
		// second mechanism) matches exactly ONE kept row is that row's flat
		// re-publication: it absorbs into the kept row (E# joins the merged
		// ids — evidence stays reachable, 零静默消失) and the fold count
		// honestly shrinks. Ambiguity (≥2 kept matches) fails open into the
		// pool. 禁用裸成员盘存重叠判 — only the FULL µs fingerprint absorbs
		// (a loose overlap would swallow C-type different accounts, W-A).
		if host, ok := traceCausalProjectionOverflowMirrorHostRef(kept, extraHosts, member); ok {
			absorbed := map[string]bool{traceCausalProjectionCanonicalNode(host.EvidenceID): true}
			for _, id := range host.MergedEvidenceIDs {
				absorbed[traceCausalProjectionCanonicalNode(id)] = true
			}
			for _, id := range append([]string{member.EvidenceID}, member.MergedEvidenceIDs...) {
				if id = strings.TrimSpace(id); id != "" && !absorbed[traceCausalProjectionCanonicalNode(id)] {
					absorbed[traceCausalProjectionCanonicalNode(id)] = true
					host.MergedEvidenceIDs = append(host.MergedEvidenceIDs, id)
				}
			}
			continue
		}
		overflow = append(overflow, member)
	}
	if len(overflow) == 0 {
		return kept
	}
	fold := traceCausalProjectionOverflowFoldRow(overflow)
	// WO-D1③ 多引用 tag arm (31552 E25 shape): the pool's headline (取最大)
	// member can itself be an ×N aggregate whose DERIVABLE member values each
	// µs-match a rendered same-(subject,state) row (E25 20.816 = E5 15.565 +
	// E10 5.251 to the µs) — the headline then re-publishes their combined
	// physical time. The fold row carries the matched kept rows' evidence ids
	// so the display can stamp「同段镜像·与[E5]+[E10]同一物理时间,不可相加」
	// on the headline (tag-only; the pool row and its count stay honest).
	if ids := traceCausalProjectionOverflowHeadlineMirrorIDsAcross(kept, extraHosts, overflow); len(ids) > 0 {
		fold.OverflowMirrorEvidenceIDs = ids
	} else {
		rendered := append([]TraceCausalProjectionNode(nil), kept...)
		for _, group := range extraHosts {
			rendered = append(rendered, group...)
		}
		fold.OverflowProjectionEvidenceID = traceCausalProjectionOverflowProjectionMirrorID(rendered, overflow)
	}
	return append(kept, fold)
}

// traceCausalProjectionOverflowMirrorHostRef finds the UNIQUE rendered row
// (kept bucket first, then the sibling chain-universe buckets) whose
// value-mirror fingerprint matches the pool candidate — ambiguity across ALL
// groups fails open.
func traceCausalProjectionOverflowMirrorHostRef(kept []TraceCausalProjectionNode, extras [][]TraceCausalProjectionNode, member TraceCausalProjectionNode) (*TraceCausalProjectionNode, bool) {
	// Every host matching the member ALREADY shares the FULL value-mirror
	// fingerprint (subject + state + µs display AND cumulative + window) by
	// the match predicate — under the established fingerprint semantics
	// (ValueMirror arm: this fingerprint = 同一物理时间), multiple matched
	// hosts are the same physical time on several lanes, never an ambiguity.
	// The FIRST rendered host takes the absorption (8869 复放: the on-chain +
	// hops copies of one trunk row; 14047 复放: the trunk hop + its
	// value-mirror aggregate twin — both stalls were this over-conservative
	// fail-open re-seating the ghost headline).
	for _, group := range append([][]TraceCausalProjectionNode{kept}, extras...) {
		for i := range group {
			if traceCausalProjectionOverflowMirrorHostMatch(group[i], member) {
				return &group[i], true
			}
		}
	}
	return nil, false
}

// traceCausalProjectionOverflowProjectionMirrorID (P2-2 跨口径穿透, SMR-1
// 修复轮 2026-07-13) resolves the pool's cross-caliber projection host among
// the rendered rows. Two µs-precise lanes (occurrence_windows inventory is
// not display-reachable, so the identities ARE the typed proof):
//
//	(a) Σ(pool member displays, all one canonical subject) µs-equals a
//	    rendered same-subject row's display — the pool re-publishes that
//	    row's occurrence projections (tieba E21 → E11);
//	(b) the pool headline µs-equals a rendered same-subject row's PUBLISHED
//	    effective attribution — the eff caliber re-issued as a pool value
//	    (donghu E26 → E13).
//
// Ambiguity (≥2 hosts) or any miss returns "" (fail-open, no tag).
func traceCausalProjectionOverflowProjectionMirrorID(rendered, overflow []TraceCausalProjectionNode) string {
	if len(overflow) == 0 {
		return ""
	}
	subject := traceCausalProjectionCanonicalNode(overflow[0].Subject)
	sameSubject := subject != "" && traceCausalProjectionKnownSubject(overflow[0].Subject)
	sum, maxDisplay := 0.0, 0.0
	for _, member := range overflow {
		if traceCausalProjectionCanonicalNode(member.Subject) != subject {
			sameSubject = false
		}
		display := member.ImpactMS
		if display <= 0 {
			display = member.CumulativeImpactMS
		}
		sum += display
		if display > maxDisplay {
			maxDisplay = display
		}
	}
	host, count := "", 0
	consider := func(id string) {
		if id == "" {
			return
		}
		if host == traceCausalProjectionCanonicalNode(id) {
			return
		}
		host = traceCausalProjectionCanonicalNode(id)
		count++
	}
	for i := range rendered {
		node := rendered[i]
		if node.OnChainOverflowFold || traceCausalProjectionCanonicalNode(node.Subject) == "" {
			continue
		}
		display := node.ImpactMS
		if display <= 0 {
			display = node.CumulativeImpactMS
		}
		if sameSubject && traceCausalProjectionCanonicalNode(node.Subject) == subject &&
			sum > 0 && display > 0 && math.Abs(sum-display) < TraceCausalProjectionSameValueTieMS {
			consider(node.EvidenceID) // lane (a)
			continue
		}
		if maxDisplay > 0 && node.EffectiveImpactMS > 0 &&
			traceCausalProjectionCanonicalNode(node.Subject) == subject &&
			math.Abs(maxDisplay-node.EffectiveImpactMS) < TraceCausalProjectionSameValueTieMS {
			consider(node.EvidenceID) // lane (b)
		}
	}
	if count != 1 {
		return ""
	}
	return host
}

// traceCausalProjectionOverflowHeadlineMirrorIDsAcross is the multi-bucket
// form of the headline multi-ref arm (rendered rows = kept + siblings).
func traceCausalProjectionOverflowHeadlineMirrorIDsAcross(kept []TraceCausalProjectionNode, extras [][]TraceCausalProjectionNode, overflow []TraceCausalProjectionNode) []string {
	rendered := append([]TraceCausalProjectionNode(nil), kept...)
	for _, group := range extras {
		rendered = append(rendered, group...)
	}
	return traceCausalProjectionOverflowHeadlineMirrorIDs(rendered, overflow)
}

// traceCausalProjectionOverflowMirrorHostMatch is the WO-D1③ absorption
// fingerprint (value-mirror 同款). Precise signals only: canonical subject +
// trimmed state + µs-equal display AND cumulative + compatible typed query
// window.
func traceCausalProjectionOverflowMirrorHostMatch(candidate, member TraceCausalProjectionNode) bool {
	display := member.ImpactMS
	if display <= 0 {
		display = member.CumulativeImpactMS
	}
	if display <= 0 {
		return false
	}
	subject := traceCausalProjectionCanonicalNode(member.Subject)
	if subject == "" || !traceCausalProjectionKnownSubject(member.Subject) {
		return false
	}
	if candidate.OnChainOverflowFold {
		return false
	}
	if traceCausalProjectionCanonicalNode(candidate.Subject) != subject ||
		strings.TrimSpace(candidate.StateKind) != strings.TrimSpace(member.StateKind) {
		return false
	}
	hostDisplay := candidate.ImpactMS
	if hostDisplay <= 0 {
		hostDisplay = candidate.CumulativeImpactMS
	}
	if math.Abs(hostDisplay-display) >= TraceCausalProjectionSameValueTieMS ||
		math.Abs(candidate.CumulativeImpactMS-member.CumulativeImpactMS) >= TraceCausalProjectionSameValueTieMS {
		return false
	}
	if traceCausalProjectionIntervalValid(member.QueryWindowStartTs, member.QueryWindowEndTs) &&
		traceCausalProjectionIntervalValid(candidate.QueryWindowStartTs, candidate.QueryWindowEndTs) &&
		(math.Abs(member.QueryWindowStartTs-candidate.QueryWindowStartTs) > TraceCausalProjectionSameWindowToleranceS ||
			math.Abs(member.QueryWindowEndTs-candidate.QueryWindowEndTs) > TraceCausalProjectionSameWindowToleranceS) {
		return false
	}
	return true
}

// traceCausalProjectionOverflowHeadlineMirrorIDs resolves the WO-D1③
// multi-reference arm: when the pool's MAX (headline) member is an ×N
// aggregate whose losslessly derivable member values EACH µs-match a distinct
// kept same-(subject, state) row, the matched kept rows' evidence ids are
// returned in member-value order. Any unmatched member, an underivable
// multiset, or a sub-2 aggregate returns nil (fail-open, no tag).
func traceCausalProjectionOverflowHeadlineMirrorIDs(kept, overflow []TraceCausalProjectionNode) []string {
	maxIdx, maxDisplay := -1, 0.0
	for i := range overflow {
		display := overflow[i].ImpactMS
		if display <= 0 {
			display = overflow[i].CumulativeImpactMS
		}
		if display > maxDisplay {
			maxIdx, maxDisplay = i, display
		}
	}
	if maxIdx < 0 {
		return nil
	}
	head := overflow[maxIdx]
	if head.MergedCount < 2 || head.MergedMinMS <= 0 || head.MergedMaxMS < head.MergedMinMS ||
		head.MergedValuelessCount > 0 {
		return nil
	}
	var members []float64
	switch head.MergedCount {
	case 2:
		members = []float64{head.MergedMinMS, head.MergedMaxMS}
	case 3:
		sum := head.MergedSumMS
		if sum <= 0 && !head.MergedIntervalUnion && !head.MergedCrossWindowMax && !head.MergedSameSegmentMirror {
			sum = head.ImpactMS
		}
		middle := sum - head.MergedMinMS - head.MergedMaxMS
		if middle <= 0 {
			return nil
		}
		members = []float64{head.MergedMinMS, middle, head.MergedMaxMS}
	default:
		return nil // >3 members are not losslessly derivable — fail open
	}
	subject := traceCausalProjectionCanonicalNode(head.Subject)
	if subject == "" || !traceCausalProjectionKnownSubject(head.Subject) {
		return nil
	}
	var ids []string
	used := map[int]bool{}
	for _, value := range members {
		matched := -1
		for i := range kept {
			if used[i] || kept[i].OnChainOverflowFold {
				continue
			}
			if traceCausalProjectionCanonicalNode(kept[i].Subject) != subject ||
				strings.TrimSpace(kept[i].StateKind) != strings.TrimSpace(head.StateKind) {
				continue
			}
			display := kept[i].ImpactMS
			if display <= 0 {
				display = kept[i].CumulativeImpactMS
			}
			if math.Abs(display-value) < TraceCausalProjectionSameValueTieMS {
				matched = i
				break
			}
		}
		if matched < 0 {
			return nil // every derived member must be a rendered row (full proof)
		}
		used[matched] = true
		if id := strings.TrimSpace(kept[matched].EvidenceID); id != "" {
			ids = append(ids, id)
		} else {
			return nil
		}
	}
	return ids
}

// TraceCausalProjectionSeatFoldExemptTopN is the seat population whose rows are
// exempt from the counted overflow folds — exactly the ➊..➎ badge promise
// (display parity pinned against runtimeTraceProjBadgeTopN by
// TestCR2P4SeatExemptTopNMatchesBadgeTopN in internal/tool).
const TraceCausalProjectionSeatFoldExemptTopN = 5

// traceCausalProjectionSeatFoldExempt reports whether the node holds a
// published TOP-N root-cause seat (typed engine Rank, precise integer signal —
// never a score/heuristic): such rows are white-listed out of the overflow
// folds so the badge/ordinal promise survives the cap (CR-2 P4). Fold rows
// themselves never qualify (a roster carries no seat by construction).
//
// SELF-ALL rider (§29.61.2 连带, 2026-07-13): ⌗ caliber-side rows join the
// exemption — the V2-P0 legend promises 「行照常显示并经 [E#] 互链」, and the
// SELF-ALL promotion grew the on-chain bucket population enough that a
// caliber-side row (donghu block_io_by_inode, the ONLY carrier of its
// composite-caliber words) could overflow into the counted fold, whose roster
// speaks subjects — the caliber words silently vanished (h3 复放 flake
// witness, 2026-07-13). Typed tier token, bounded population (the V2-P0 side
// rail is small by construction).
func traceCausalProjectionSeatFoldExempt(node TraceCausalProjectionNode) bool {
	if node.OnChainOverflowFold {
		return false
	}
	if node.IsCaliberSideRow() {
		return true
	}
	// EVOLUTION RECORD (SELF-ALL 修复轮 件1 F1, 2026-07-13): Rank ≤ TopN →
	// EVERY seated row (Rank > 0). The SELF-ALL promotion grew the on-chain
	// population by the target's own seats, and the +1 pushed keva-3 seat #7
	// past the positional compile cap — the row vanished from every render
	// surface while the visible ordinal sequence read #6→#8 (the legend
	// promises contiguous seat ordinals; 133136 A/B witness). The widened
	// exemption is still a precise bounded signal: engine ordinals are capped
	// by the rank candidate limit, so the exempt population never exceeds the
	// published board (CR-2 P4 philosophy, 勿显示层打补丁). The TopN const
	// keeps its badge-parity meaning (➊..➎) unchanged.
	return node.Rank > 0
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
		// CR-2 组① P4: the hop fold shares the seat white-list — one promise,
		// one predicate (see traceCausalProjectionLimitNodesOnChainFold).
		if traceCausalProjectionSeatFoldExempt(member) {
			kept = append(kept, member)
			continue
		}
		// WO-D1③ (SMR-1 批 SMR-S9, 2026-07-12; 23245 复放实锤: the flat
		// 42.131 re-publication reached the HOPS overflow on this run's bucket
		// placement and re-seated the 37% ghost headline): the hop fold runs
		// the SAME absorption arm as the on-chain fold — a member whose full
		// value-mirror fingerprint matches a rendered host (kept hops or the
		// on-chain bucket) absorbs into it (E# joins the bracket) and the pool
		// honestly shrinks. One predicate, two call sites (never a fork).
		if host, ok := traceCausalProjectionOverflowMirrorHostRef(kept, [][]TraceCausalProjectionNode{onChain}, member); ok {
			absorbed := map[string]bool{traceCausalProjectionCanonicalNode(host.EvidenceID): true}
			for _, id := range host.MergedEvidenceIDs {
				absorbed[traceCausalProjectionCanonicalNode(id)] = true
			}
			for _, id := range append([]string{member.EvidenceID}, member.MergedEvidenceIDs...) {
				if id = strings.TrimSpace(id); id != "" && !absorbed[traceCausalProjectionCanonicalNode(id)] {
					absorbed[traceCausalProjectionCanonicalNode(id)] = true
					host.MergedEvidenceIDs = append(host.MergedEvidenceIDs, id)
				}
			}
			continue
		}
		overflow = append(overflow, member)
	}
	if len(overflow) == 0 {
		return kept
	}
	fold := traceCausalProjectionOverflowFoldRow(overflow)
	if ids := traceCausalProjectionOverflowHeadlineMirrorIDsAcross(kept, [][]TraceCausalProjectionNode{onChain}, overflow); len(ids) > 0 {
		fold.OverflowMirrorEvidenceIDs = ids
	} else {
		rendered := append(append([]TraceCausalProjectionNode(nil), kept...), onChain...)
		fold.OverflowProjectionEvidenceID = traceCausalProjectionOverflowProjectionMirrorID(rendered, overflow)
	}
	return append(kept, fold)
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
			// RUN2FIX-A 件2: record the MAX member's identity for the fold
			// row's 线程·状态·值 disclosure. An absorbed fold member passes
			// through its own recorded maximum (the true value owner); an
			// unknown/empty subject clears both fields (宁漏勿假 — the
			// display keeps the legacy line).
			if member.OnChainOverflowFold && member.MergedCount > 0 {
				fold.MergedMaxSubject = member.MergedMaxSubject
				fold.MergedMaxStateKind = member.MergedMaxStateKind
			} else if traceCausalProjectionKnownSubject(member.Subject) {
				fold.MergedMaxSubject = strings.TrimSpace(member.Subject)
				fold.MergedMaxStateKind = strings.TrimSpace(member.StateKind)
			} else {
				fold.MergedMaxSubject = ""
				fold.MergedMaxStateKind = ""
			}
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
	// A zero-account legacy/context publication can carry the exact same base
	// fact as a richer named-board rank publication. It must not create a new
	// unnamed seat merely because the richer peer learned typed board identity
	// later in the pipeline ("absence never splits"). Learn those base facts in
	// a first pass so the result is deterministic even when the mirror precedes
	// its richer peer. Value-bearing/ranked identity-less publications are NOT
	// mirrors: they are honest legacy accounts and remain on the unnamed board.
	namedBoardBases := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		if traceCausalProjectionRankBoardDedupeKey(node) == "" {
			continue
		}
		namedBoardBases[traceCausalProjectionDedupeBaseKey(node)] = true
	}
	seen := make(map[string]bool, len(nodes))
	out := make([]TraceCausalProjectionNode, 0, len(nodes))
	for _, node := range nodes {
		baseKey := traceCausalProjectionDedupeBaseKey(node)
		// XLANE-3 board-domain closure (2026-07-17): a ranked account belongs
		// to the typed triple (query window, board target, params fingerprint).
		// The former key stopped at role/subject/predicate/object/support refs;
		// after priority authority correctly reclassified one logd.writer
		// inversion seat into ordinary runnable, that row shared those bytes
		// with a different target board and silently swallowed the other
		// board's 0.018ms anchored seat. Carry the triple only when a producer
		// supplied either board half. Identity-less VALUE-BEARING accounts keep
		// an explicit unnamed-board domain; identity-less zero-account mirrors
		// of a named fact are discarded as the old byte-key dedupe did.
		rankBoard := traceCausalProjectionRankBoardDedupeKey(node)
		if rankBoard == "" && namedBoardBases[baseKey] && !traceCausalProjectionNodeCarriesDedupeAccount(node) {
			continue
		}
		if rankBoard == "" && namedBoardBases[baseKey] {
			rankBoard = "unnamed"
		}
		key := baseKey + "\x00" + rankBoard
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	return out
}

func traceCausalProjectionDedupeBaseKey(node TraceCausalProjectionNode) string {
	// RSPA (§29.61.10, 2026-07-14): the re-anchoring bipartition halves are
	// TWO ACCOUNTS of one segment set (⛓ anchored + ◇ remainder) — same
	// Role/Subject/Predicate/Object, deliberately co-published; the dedupe key
	// forks on the typed remainder marker so the ◇ half can never be
	// swallowed as a duplicate of its ⛓ sibling.
	remainderHalf := ""
	if node.ChainAnchorRemainderSeat {
		remainderHalf = "remainder"
	}
	return strings.Join([]string{
		traceCausalProjectionCanonicalNode(node.Role),
		traceCausalProjectionCanonicalNode(node.Subject),
		traceCausalProjectionCanonicalNode(node.Predicate),
		traceCausalProjectionCanonicalNode(node.Object),
		traceCausalProjectionCanonicalNode(strings.Join(node.SupportRefs, "|")),
		remainderHalf,
	}, "\x00")
}

func traceCausalProjectionRankBoardDedupeKey(node TraceCausalProjectionNode) string {
	if strings.TrimSpace(node.RankBoardTarget) == "" && strings.TrimSpace(node.RankBoardParamsFingerprint) == "" {
		return ""
	}
	windowStart, windowEnd := node.QueryWindowStartTs, node.QueryWindowEndTs
	if TraceCausalProjectionWindowPresent(node.RankQueryWindowStartTs, node.RankQueryWindowEndTs) {
		windowStart, windowEnd = node.RankQueryWindowStartTs, node.RankQueryWindowEndTs
	}
	return strings.Join([]string{
		traceCausalProjectionCanonicalNode(node.RankBoardTarget),
		strings.TrimSpace(node.RankBoardParamsFingerprint),
		strconv.FormatFloat(windowStart, 'g', -1, 64),
		strconv.FormatFloat(windowEnd, 'g', -1, 64),
	}, "\x01")
}

func traceCausalProjectionNodeCarriesDedupeAccount(node TraceCausalProjectionNode) bool {
	return node.Rank > 0 ||
		node.ImpactMS > 0 ||
		node.CumulativeImpactMS > 0 ||
		node.EffectiveImpactMS > 0 ||
		node.ActualImpactMS > 0 ||
		node.TargetImpactMS > 0
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
	// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16): the composite-score twin —
	// composite rank rows publish impact_score instead of impact_ms.
	if value := traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyImpactScore); value > 0 {
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
	// RNB-5B 件② (§29.96.2 终判②, 2026-07-15): "self_caliber_side" joined the
	// wire closed set — the target-self count row's NON-CHANNEL ⌗ side-rail
	// token. It must survive this strict parser verbatim: falling through to
	// the causality fallback re-minted the "adjacent" channel claim the token
	// retires (and fed the row back into the ◇ bucket-cap fold, where its
	// count-equivalent value published as the fold's bare-ms MAX — the 17267
	// production witness).
	case "on_chain", "adjacent", "background", "self_caliber_side":
		return strings.TrimSpace(relevance)
	}
	switch strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, TraceNoteKeyCausality)) {
	// SELF-SEM (§29.61.1) / SELF-ALL (§29.61.2): the self tokens denote
	// on-chain channel membership on the typed self basis (no wakeup-edge claim).
	case "on_wakeup_chain", "on_dependency_chain", "self_deterministic", "self_wall_clock":
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

// traceCausalProjectionGatedCompositeEdgeShareFromRecord (PARTSPLIT-1,
// §29.150④) is the all-or-nothing parser of one gated_composite_edge_share
// record. ok=false drops the record whole (fail-open to absence). The X+Y==
// Account identity is re-validated HERE at the print quantum: the engine
// gate is rspaWithinTol (1µs slack, not exact), and three "%.3f" roundings
// add ≤ 1.5µs, so the honest worst case is 2.5µs — the check allows 3µs
// (print-quantum + engine-gate headroom; never an identity tolerance
// borrowed across semantics). A record whose three values disagree beyond
// that proves nothing and never publishes. Via is the R3 closed set.
func traceCausalProjectionGatedCompositeEdgeShareFromRecord(record ObservationRecord) (TraceCausalProjectionGatedCompositeEdgeShareDisclosure, bool) {
	var out TraceCausalProjectionGatedCompositeEdgeShareDisclosure
	out.Subject = strings.TrimSpace(record.Subject)
	if out.Subject == "" {
		return out, false
	}
	pre, errPre := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCompositeEdgePreShare), 64)
	post, errPost := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCompositeEdgePostShare), 64)
	account, errAccount := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCompositeEdgeAccount), 64)
	if errPre != nil || errPost != nil || errAccount != nil || !(pre > 0) || !(post > 0) || !(account > 0) {
		return out, false
	}
	if diff := pre + post - account; diff > 0.003 || diff < -0.003 {
		return out, false
	}
	out.PreMS, out.PostMS, out.AccountMS = pre, post, account
	anchorTs, errTs := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCompositeEdgeAnchorTs), 64)
	if errTs != nil || !(anchorTs > 0) {
		return out, false
	}
	out.AnchorTS = anchorTs
	out.Via = strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCompositeEdgeAnchorVia))
	switch out.Via {
	case "direct", "chain_hop", "direct+chain_hop":
	default:
		return out, false
	}
	switch strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyGatedCompositeEdgeSeatPublished)) {
	case "true":
		out.SeatPublished = true
	case "false":
		out.SeatPublished = false
	default:
		return out, false
	}
	return out, true
}

// traceCausalProjectionSelfRunnableTwoRulerFromRecord (RULER2-1, §29.150②)
// is the all-or-nothing parser of one self_runnable_two_ruler record.
// ok=false drops the record whole (fail-open to absence). Each same-ruler Σ
// identity is re-validated HERE at the print quantum: every value prints at
// "%.3f" upstream, so n addend roundings plus the subtotal rounding bound the
// honest drift by (n+1)×0.5µs — the check allows 1µs per participating value
// (never an identity tolerance borrowed across semantics). A ruler whose
// values disagree with its subtotal beyond that proves nothing and never
// publishes. Both rulers MUST be occupied (a single-ruler record is not a
// two-ruler accounting) and the eff/rank lists must be parallel.
func traceCausalProjectionSelfRunnableTwoRulerFromRecord(record ObservationRecord) (TraceCausalProjectionSelfRunnableTwoRuler, bool) {
	var out TraceCausalProjectionSelfRunnableTwoRuler
	out.Subject = strings.TrimSpace(record.Subject)
	if out.Subject == "" {
		return out, false
	}
	parseRuler := func(effsKey, ranksKey, subtotalKey string) ([]float64, []int, float64, bool) {
		effsRaw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, effsKey))
		ranksRaw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, ranksKey))
		subtotal, errSub := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, subtotalKey), 64)
		if effsRaw == "" || ranksRaw == "" || errSub != nil || !(subtotal > 0) {
			return nil, nil, 0, false
		}
		effParts := strings.Split(effsRaw, ",")
		rankParts := strings.Split(ranksRaw, ",")
		if len(effParts) == 0 || len(effParts) != len(rankParts) {
			return nil, nil, 0, false
		}
		effs := make([]float64, 0, len(effParts))
		ranks := make([]int, 0, len(rankParts))
		sum := 0.0
		for i := range effParts {
			eff, errEff := strconv.ParseFloat(strings.TrimSpace(effParts[i]), 64)
			rank, errRank := strconv.Atoi(strings.TrimSpace(rankParts[i]))
			if errEff != nil || errRank != nil || !(eff > 0) || rank <= 0 {
				return nil, nil, 0, false
			}
			effs = append(effs, eff)
			ranks = append(ranks, rank)
			sum += eff
		}
		tol := float64(len(effs)+1) * 0.001
		if diff := sum - subtotal; diff > tol || diff < -tol {
			return nil, nil, 0, false
		}
		return effs, ranks, subtotal, true
	}
	wallEffs, wallRanks, wallSubtotal, okWall := parseRuler(
		TraceNoteKeySelfTwoRulerWallEffs, TraceNoteKeySelfTwoRulerWallRanks, TraceNoteKeySelfTwoRulerWallSubtotal)
	edgeEffs, edgeRanks, edgeSubtotal, okEdge := parseRuler(
		TraceNoteKeySelfTwoRulerEdgeEffs, TraceNoteKeySelfTwoRulerEdgeRanks, TraceNoteKeySelfTwoRulerEdgeSubtotal)
	if !okWall || !okEdge {
		return out, false
	}
	out.WallEffsMS, out.WallRanks, out.WallSubtotalMS = wallEffs, wallRanks, wallSubtotal
	out.EdgeEffsMS, out.EdgeRanks, out.EdgeSubtotalMS = edgeEffs, edgeRanks, edgeSubtotal
	return out, true
}

// traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord (SELFRUN-DISC,
// §29.192① (b)) is the all-or-nothing parser of one
// self_running_fold_unmeasured record. ok=false drops the record whole
// (fail-open to absence). The fold identity running == unknown (the engine's
// KnownMs==0 form: KnownMs+UnknownMs==RunningMs) is re-validated HERE at the
// print quantum: both values print at "%.3f" upstream, so two roundings
// bound the honest drift by 1µs — the check allows 2µs (print-quantum
// headroom; never an identity tolerance borrowed across semantics). A record
// whose two values disagree beyond that proves nothing — a PARTIALLY-known
// basis is exactly the shape this disclosure must never claim — and never
// publishes.
func traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord(record ObservationRecord) (TraceCausalProjectionSelfRunningFoldUnmeasured, bool) {
	var out TraceCausalProjectionSelfRunningFoldUnmeasured
	out.Subject = strings.TrimSpace(record.Subject)
	if out.Subject == "" {
		return out, false
	}
	running, errRunning := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySelfRunningFoldUnmeasuredRunningMS), 64)
	unknown, errUnknown := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeySelfRunningFoldUnmeasuredUnknownMS), 64)
	if errRunning != nil || errUnknown != nil || !(running > 0) || !(unknown > 0) {
		return out, false
	}
	if diff := running - unknown; diff > 0.002 || diff < -0.002 {
		return out, false
	}
	out.RunningMS, out.UnknownMS = running, unknown
	return out, true
}

// traceCausalProjectionBusinessSpanMentionFromRecord is the SPANVIS-1 strict
// all-or-nothing parser of one business_span_mention record. ok=false drops
// the record whole (fail-open to absence): a mention row may never publish a
// partially-typed value set. Basis is a CLOSED SET — the literals mirror the
// engine's BusinessSpanMentionBasis* constants (pinned equal by the engine
// tests; this package sits below the engine and cannot import them).
func traceCausalProjectionBusinessSpanMentionFromRecord(record ObservationRecord) (TraceCausalProjectionBusinessSpanMention, int, bool) {
	var out TraceCausalProjectionBusinessSpanMention
	out.Subject = strings.TrimSpace(record.Subject)
	if out.Subject == "" {
		return out, 0, false
	}
	out.Name = traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBusinessSpanName)
	if out.Name == "" {
		return out, 0, false
	}
	out.Count = traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyBusinessSpanCount)
	if out.Count < 1 {
		return out, 0, false
	}
	total, err := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBusinessSpanTotalMS), 64)
	if err != nil || !(total > 0) {
		return out, 0, false
	}
	max, err := strconv.ParseFloat(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBusinessSpanMaxMS), 64)
	if err != nil || !(max > 0) {
		return out, 0, false
	}
	// Both values print at the µs quantum ("%.3f"); Σ(true) ≥ max(true), so
	// the printed pair may diverge by at most one print quantum per side.
	// Own tolerance, own semantics (容差常量禁跨语义借用): print-quantum
	// noise only, never an identity tolerance.
	if max > total+0.001 {
		return out, 0, false
	}
	out.TotalMS, out.MaxMS = total, max
	lines := traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBusinessSpanLines)
	sep := strings.Index(lines, "..")
	if sep <= 0 {
		return out, 0, false
	}
	start, errStart := strconv.Atoi(strings.TrimSpace(lines[:sep]))
	end, errEnd := strconv.Atoi(strings.TrimSpace(lines[sep+2:]))
	if errStart != nil || errEnd != nil || start < 1 || end < start {
		return out, 0, false
	}
	out.StartLine, out.EndLine = start, end
	out.Basis = traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBusinessSpanBasis)
	switch out.Basis {
	case "self", "chain_member", "host_wakeup_edge":
	default:
		return out, 0, false
	}
	// POOL2-1 件① (§29.160/§29.160.1 ruling 2026-07-20): the engine's third
	// admission gate (≥1 cap-hidden member) is removed, so Hidden==0 (fully-
	// visible family) is now a VALID value — the strict arm evolves from
	// [1,Count] to [0,Count] but keeps requiring the key's PRESENCE (the
	// emitter publishes 0 explicitly; an absent/junk key still drops whole).
	hiddenRaw := strings.TrimSpace(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyBusinessSpanHidden))
	hidden, errHidden := strconv.Atoi(hiddenRaw)
	if errHidden != nil || hidden < 0 || hidden > out.Count {
		return out, 0, false
	}
	out.Hidden = hidden
	omitted := traceCausalProjectionRichNoteInt(record.RichNotes, TraceNoteKeyBusinessSpanOmitted)
	if omitted < 0 {
		omitted = 0
	}
	return out, omitted, true
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

func traceCausalProjectionRichNoteInt64(notes []string, key string) int64 {
	value := traceCausalProjectionRichNoteValue(notes, key)
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "ms"))
	n, err := strconv.ParseInt(value, 10, 64)
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
