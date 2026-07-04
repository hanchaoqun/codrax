package types

// trace_note_keys.go — SINGLE SOURCE OF TRUTH for the trace_query rich-note
// key names that travel on ObservationRecord.RichNotes between the producer
// (internal/tool/trace_query.go typed-observation projection, plus the legacy
// text-parse lane in observation_ledger.go) and the consumers (the causal
// projection compiler in trace_causal_projection.go, the coverage view in
// trace_observation_coverage.go, and the display layer in
// internal/tool/answer_document_mutation_runtime.go /
// emit_investigation_complete.go). NKR (再审计 P0-3, 2026-07-05).
//
// ── Failure-class precedent (READ BEFORE TOUCHING ANY KEY) ──────────────────
//
// These keys are a WIRE FORMAT: the producer renders "key=value" strings and
// every consumer re-parses them by exact-prefix match. A key typo on either
// side does not fail any test by itself — the parse silently returns "" and
// the downstream gate silently skips. This is the same failure class as the
// §7.4 CMP-10 incident (a token published under the wrong lane rode wire
// silence into a wrong user-facing attribution) and the F-2 same-window gate
// (a full-window total whose selected_window note is missing or misspelled is
// silently never attached — 禁猜 means the gate cannot recover the window by
// guessing). The projection compile promotions (periodic_source →
// PeriodicSource, fold_basis → SupplyFold*, occupier_N → OccupierSummary)
// skip silently the same way.
//
// ── Change protocol (three steps, no shortcuts) ─────────────────────────────
//
//  1. Change the key HERE first (constant for contract-tier keys, table row
//     for display-tier keys) and update the golden snapshot
//     (trace_note_keys_golden_test.go) — the golden diff is the review
//     surface, exactly like the causal-token registry golden.
//  2. Update BOTH ends through the constant. Never re-introduce a bare
//     literal at a producer or consumer call site: the consumer AST pin
//     (trace_note_keys_consumer_pin_test.go) and the producer emit pin
//     (internal/tool/trace_note_keys_emit_pin_test.go) enforce membership.
//  3. Test files deliberately KEEP verbatim "key=value" literals — that is
//     an intentional double-write so the constants and the wire format can
//     never drift together unnoticed. Do not "clean them up" to constants.
//
// Tier semantics:
//   - Contract tier (exported TraceNoteKey* constants): every key parsed by
//     at least one consumer, plus the wire keys of the flagged families
//     (occupancy / density / supply-fold). Producers and consumers MUST
//     reference these through the constants.
//   - Display tier (table rows only): keys that are emitted but never
//     parsed anywhere. A typo there has no silent-gate failure mode; the
//     emit pin still forces registration so new keys pass review here.

// TraceNoteKeyCarrier classifies who, if anyone, parses a key back off the
// wire — i.e. what breaks when the key drifts.
type TraceNoteKeyCarrier string

const (
	// TraceNoteCarrierAnchorWindow — 锚窗白名单载体: the projection compiler
	// selects/validates its 关注窗口 through these keys (F1 selected-window
	// note, window_source lane switch, node window parse). Typo = anchor
	// fallback and the F-2 same-window verdict silently disappear.
	TraceNoteCarrierAnchorWindow TraceNoteKeyCarrier = "anchor_window"
	// TraceNoteCarrierHardConsumer — parsed by the causal projection COMPILE
	// (typed node fields, gate inputs, roster promotion). Typo = silent
	// compile-promotion loss.
	TraceNoteCarrierHardConsumer TraceNoteKeyCarrier = "hard_consumer"
	// TraceNoteCarrierSoftConsumer — parsed only by display/coverage
	// surfaces (metric snapshot, coverage rows, next-step wording). Typo =
	// display omission, no gate involved.
	TraceNoteCarrierSoftConsumer TraceNoteKeyCarrier = "soft_consumer"
	// TraceNoteCarrierDisplayOnly — emitted for human/LLM display, parsed by
	// nobody.
	TraceNoteCarrierDisplayOnly TraceNoteKeyCarrier = "display_only"
)

// Contract-tier constants, grouped by family.
//
// 锚窗族 (anchor-window family).
//
// Cross-ref (F3, 双"恰三"白名单): anchoring the 关注窗口 is TWO-gated. Gate 1
// is the KEY dimension — exactly the three anchor_window carriers below
// (selected_window / window / window_source, pinned by
// TestTraceNoteKeyRegistryStructure). Gate 2 is the PREDICATE-FAMILY
// whitelist in trace_causal_projection.go
// traceCausalProjectionSelectedWindowAnchorFamily (wakeup_causal_aggregate /
// wakeup_causal_impact / root_cause_* — also exactly three). A record anchors
// only when BOTH gates pass: a NEW producer family that starts emitting
// selected_window does NOT automatically gain anchoring — it must be
// adjudicated into the predicate whitelist too (see the 裁定沿革 there).
const (
	// TraceNoteKeySelectedWindow is THE selected-query-window carrier (F1):
	// root_cause_rank / wakeup aggregate / blocking / state_churn /
	// drilldown / thread-duration / occupancy rows all publish the engine's
	// own two-sided query window under this key, and the projection anchors
	// its 关注窗口 fallback and the F-2 RN-12 same-window verdict EXCLUSIVELY
	// on it (record Span is the member-impact envelope, never the window).
	TraceNoteKeySelectedWindow = "selected_window"
	// TraceNoteKeyWindow is the per-row segment/span window note; the
	// projection parses it for node windows and the window_source=span lane.
	TraceNoteKeyWindow = "window"
	// TraceNoteKeyWindowSource selects the compile-side anchor lane
	// (target_resolution / frame selection provenance).
	TraceNoteKeyWindowSource = "window_source"
	// TraceNoteKeyWindowMS is the CMP-9 wall-window length (ms) that
	// normalizes cross-trace densities; display/comparison wire token.
	TraceNoteKeyWindowMS = "window_ms"
	// TraceNoteKeyActualWindow / Nearest / Occurrence are window-valued
	// display carriers (coverage view window column, §7.30 dual-basis
	// labeling); deliberately OUTSIDE the anchor whitelist.
	TraceNoteKeyActualWindow       = "actual_window"
	TraceNoteKeyNearestChainWindow = "nearest_chain_window"
	TraceNoteKeyOccurrenceWindows  = "occurrence_windows"
)

// 因果排名族 (causal-rank family).
const (
	TraceNoteKeyRank           = "rank"
	TraceNoteKeyTier           = "tier"
	TraceNoteKeyType           = "type"
	TraceNoteKeySource         = "source"
	TraceNoteKeyCausality      = "causality"
	TraceNoteKeyChainRelevance = "chain_relevance"
	TraceNoteKeyChainDepth     = "chain_depth"
	// TraceNoteKeyDepth: RN-14c consumers key chain-root detection on the
	// ABSENCE of this note (depth 0 is zero-dropped by the producer) — do
	// not switch the producer to always-print without updating them.
	TraceNoteKeyDepth             = "depth"
	TraceNoteKeySubjectKind       = "subject_kind"
	TraceNoteKeyCapacityTruncated = "capacity_truncated"
	TraceNoteKeyChainRequired     = "chain_required"
	TraceNoteKeyRecursive         = "recursive"
	TraceNoteKeySignificant       = "significant"
	TraceNoteKeyRecommendedViews  = "recommended_views"
)

// 冲击度量族 (impact-metric family).
const (
	TraceNoteKeyImpact             = "impact"
	TraceNoteKeyImpactMS           = "impact_ms"
	TraceNoteKeyCumulativeImpactMS = "cumulative_impact_ms"
	TraceNoteKeyEffectiveImpactMS  = "effective_impact_ms"
	// TraceNoteKeyEffectiveImpact is a consumer-side legacy alias (FirstFloat
	// fallback); no current producer emits it.
	TraceNoteKeyEffectiveImpact = "effective_impact"
	TraceNoteKeyActualImpactMS  = "actual_impact_ms"
	TraceNoteKeyActualImpact    = "actual_impact"
	TraceNoteKeyTotal           = "total"
	TraceNoteKeyActualTotalMS   = "actual_total_ms"
	TraceNoteKeyActualTotal     = "actual_total"
)

// TraceNoteKeyActualPrefix marks the §7.30 S1 dual-basis family: the display
// layer treats ANY "actual_*" note as "this row carries aligned
// underlying-window values" (prefix match, runtimeTraceRecordHasActualWindowValues).
// Every key starting with this prefix MUST therefore mean
// underlying-actual-window accounting — never reuse the prefix for anything else.
const TraceNoteKeyActualPrefix = "actual_"

// 状态族 (state family).
//
// NKR×TSK intersection, deliberate (TSH review F7 ruling: feature retention,
// lanes orthogonal): five KEY NAMES below (running / runnable / sleep /
// d_state / io_wait) are spelled with the same words as the scheduler-state
// word registry (trace_state_kinds.go, TSK). They are NOT the same lane:
// here the word is a rich-note KEY whose VALUE is a per-state ms duration
// ("runnable=12.400"); in TSK the word is itself the VALUE domain of
// StateKind / dominant_state notes. Both registries guard their own lane and
// the goldens never merge. Rename protocol: changing any of these five words
// on EITHER side is a wire-format change that must visit BOTH registries (and
// both golden/pin sets) in the same change — trace_state_kinds.go's header
// carries the mirror pointer.
const (
	TraceNoteKeyDominantState  = "dominant_state"
	TraceNoteKeyRunning        = "running"
	TraceNoteKeyRunnable       = "runnable"
	TraceNoteKeySleep          = "sleep"
	TraceNoteKeyDState         = "d_state"
	TraceNoteKeyIOWait         = "io_wait"
	TraceNoteKeyFragments      = "fragments"
	TraceNoteKeySwitches       = "switches"
	TraceNoteKeyMaxSegment     = "max_segment"
	TraceNoteKeyP95Segment     = "p95_segment"
	TraceNoteKeyActualRunning  = "actual_running"
	TraceNoteKeyActualRunnable = "actual_runnable"
	TraceNoteKeyActualSleep    = "actual_sleep"
	TraceNoteKeyActualDState   = "actual_d_state"
	TraceNoteKeyActualIOWait   = "actual_io_wait"
)

// 周期族 (periodic-source family, VS-1 §7.8).
const (
	TraceNoteKeyPeriodicSource   = "periodic_source"
	TraceNoteKeyDetectedPeriodMS = "detected_period_ms"
	TraceNoteKeyLatenessMS       = "lateness_ms"
)

// 折算族 (supply-fold family, VS-2 §7.10).
const (
	TraceNoteKeyFoldBasis             = "fold_basis"
	TraceNoteKeySupplyFoldDeficitMS   = "supply_fold_deficit_ms"
	TraceNoteKeySupplyFoldIdealMS     = "supply_fold_ideal_ms"
	TraceNoteKeyFoldFmax              = "fold_fmax"
	TraceNoteKeyFoldFmaxFinding       = "fold_fmax_finding"
	TraceNoteKeyFoldClusterLaneCaveat = "fold_cluster_lane_caveat"
)

// 占用族 (runnable-occupancy family, RN-1 §7.9) + CMP-9 density.
const (
	TraceNoteKeyStarvedRunnableMS = "starved_runnable_ms"
	// TraceNoteKeyOccupierPrefix + ordinal builds the occupier roster keys;
	// the producer caps the roster at 3, and the consumer promotes exactly
	// occupier_1..occupier_3 — keep prefix, cap, and the three constants in
	// lockstep.
	TraceNoteKeyOccupierPrefix  = "occupier_"
	TraceNoteKeyOccupier1       = "occupier_1"
	TraceNoteKeyOccupier2       = "occupier_2"
	TraceNoteKeyOccupier3       = "occupier_3"
	TraceNoteKeyAlsoStarved     = "also_starved"
	TraceNoteKeyPressureDensity = "pressure_density"
)

// 算力供给族 (compute-supply family, CMP-10; the comparison overview parses
// these three back for the cross-artifact supply cells).
const (
	TraceNoteKeySupplyRatio    = "supply_ratio"
	TraceNoteKeyIdleMismatchMS = "idle_mismatch_ms"
)

// 阻塞族 (blocking family, §7.30.3 D1).
const (
	TraceNoteKeyBlockingKind = "blocking_kind"
	TraceNoteKeyPeer         = "peer"
	TraceNoteKeyHolderSite   = "holder_site"
	TraceNoteKeyWaiters      = "waiters"
)

// 门控族 (gated-composition family, §7.30.3 D3).
const (
	TraceNoteKeyGatedRunnable       = "gated_runnable"
	TraceNoteKeyGatedRunningDeficit = "gated_running_deficit"
)

// 语义跨度族 (semantic-span family).
const (
	TraceNoteKeySpanName        = "span_name"
	TraceNoteKeySpanKind        = "span_kind"
	TraceNoteKeySpanCategory    = "span_category"
	TraceNoteKeySpanSubcategory = "span_subcategory"
	TraceNoteKeySemanticClass   = "semantic_class"
)

// 引导族 (guidance family — display-layer parsed).
const (
	TraceNoteKeyNextStep      = "next_step"
	TraceNoteKeyNextStepKind  = "next_step_kind"
	TraceNoteKeyRunnableCPU   = "runnable_cpu"
	TraceNoteKeyTopCompetitor = "top_competitor"
)

// 采样族 (perf family — display-layer parsed).
const (
	TraceNoteKeyPerfQuality = "perf_quality"
	// TraceNoteKeyPerfQualityCaveats rides next to perf_quality on
	// perf_sample_top_symbol rows; the evaluator's metric-check supplement
	// (runtimeTracePerfQualityNote) parses BOTH keys back off the wire.
	TraceNoteKeyPerfQualityCaveats = "perf_quality_caveats"
	TraceNoteKeyDSO                = "dso"
)

// 路径族 (chain-path family — emit_investigation_complete parses the chain
// tail PID out of this note).
const TraceNoteKeyPath = "path"

// 账本标记族 (ledger-marker family — NOT trace_query wire notes): composite
// marker notes appended by the observation-ledger compile itself.
// reconcileRuntimeObservationProducerPrecedence demotes pre-triage records
// and stamps TraceNoteMarkerAdvisoryPretriage; observationRecordRank then
// re-ranks on an EXACT full-note match of the same composite (soft consumer —
// evidence ordering only, no gate). legacySummaryToolObservationRecord stamps
// TraceNoteMarkerLegacySummaryFallback; nobody parses it back at runtime
// (display tier: human/LLM prompt surfaces only).
const (
	TraceNoteKeyAdvisoryPretriage                = "advisory_pretriage"
	TraceNoteKeyDeterministicRuntimeQueryPresent = "deterministic_runtime_query_present"
	TraceNoteKeyLegacySummaryFallback            = "legacy_summary_fallback"
	TraceNoteKeyNotAnswerGrade                   = "not_answer_grade"
)

// Composite ledger-marker wire strings. The append site and the exact-match
// consumer MUST both reference these composites — never rebuild the string
// from parts at a call site (the exact-match consumer would silently miss).
const (
	TraceNoteMarkerAdvisoryPretriage     = TraceNoteKeyAdvisoryPretriage + "; " + TraceNoteKeyDeterministicRuntimeQueryPresent + "=true"
	TraceNoteMarkerLegacySummaryFallback = TraceNoteKeyLegacySummaryFallback + "=true; " + TraceNoteKeyNotAnswerGrade + "=true"
)

// TraceNoteKeyRow is one registry row: the wire key, its family, and who
// parses it back (carrier).
type TraceNoteKeyRow struct {
	Key     string
	Family  string
	Carrier TraceNoteKeyCarrier
}

// traceNoteKeyRows is the FULL key universe of (a) the trace_query rich-note
// wire (typed projection + legacy text-parse lane) and (b) the ledger-lane
// marker notes the observation-ledger compile appends itself (family
// ledger_marker; composite wire strings built from the constants above).
// RichNotes written by other producers (log/perf bundle compiles, MCP
// responses) are out of registry scope. Contract-tier rows reference the
// constants above; display-tier rows are registered here as literals (single
// source; the producer literal is the only other write).
var traceNoteKeyRows = []TraceNoteKeyRow{
	// 锚窗族.
	{TraceNoteKeySelectedWindow, "anchor_window", TraceNoteCarrierAnchorWindow},
	{TraceNoteKeyWindow, "anchor_window", TraceNoteCarrierAnchorWindow},
	{TraceNoteKeyWindowSource, "anchor_window", TraceNoteCarrierAnchorWindow},
	{TraceNoteKeyWindowMS, "anchor_window", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyActualWindow, "anchor_window", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyNearestChainWindow, "anchor_window", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyOccurrenceWindows, "anchor_window", TraceNoteCarrierSoftConsumer},
	{"window_proportion", "anchor_window", TraceNoteCarrierDisplayOnly},

	// 因果排名族.
	{TraceNoteKeyRank, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyTier, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyType, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySource, "causal_rank", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyCausality, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainRelevance, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainDepth, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyDepth, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySubjectKind, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCapacityTruncated, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainRequired, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyRecursive, "causal_rank", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeySignificant, "causal_rank", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyRecommendedViews, "causal_rank", TraceNoteCarrierSoftConsumer},
	{"recommended_sections", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"score", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"edge_count", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"nearest_chain_thread", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"overlap", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"candidate_count", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_role", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_phase", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_frame_id", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_name", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"occurrences", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"coverage_mode", "causal_rank", TraceNoteCarrierDisplayOnly},

	// 冲击度量族.
	{TraceNoteKeyImpact, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyImpactMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCumulativeImpactMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyEffectiveImpactMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyEffectiveImpact, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyActualImpactMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyActualImpact, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyTotal, "impact", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyActualTotalMS, "impact", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyActualTotal, "impact", TraceNoteCarrierSoftConsumer},
	{"projected_impact", "impact", TraceNoteCarrierDisplayOnly},
	{"projected_impact_ms", "impact", TraceNoteCarrierDisplayOnly},
	{"projected_total", "impact", TraceNoteCarrierDisplayOnly},
	{"projected_total_ms", "impact", TraceNoteCarrierDisplayOnly},
	{"target_impact", "impact", TraceNoteCarrierDisplayOnly},
	{"target_impact_ms", "impact", TraceNoteCarrierDisplayOnly},
	{"rank_impact", "impact", TraceNoteCarrierDisplayOnly},
	{"duration", "impact", TraceNoteCarrierDisplayOnly},

	// 状态族.
	{TraceNoteKeyDominantState, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyRunning, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyRunnable, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySleep, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyDState, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyIOWait, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyFragments, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeySwitches, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyMaxSegment, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyP95Segment, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyActualRunning, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyActualRunnable, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyActualSleep, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyActualDState, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyActualIOWait, "state", TraceNoteCarrierSoftConsumer},
	{"state", "state", TraceNoteCarrierDisplayOnly},

	// 周期族 (VS-1).
	{TraceNoteKeyPeriodicSource, "periodic", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyDetectedPeriodMS, "periodic", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyLatenessMS, "periodic", TraceNoteCarrierHardConsumer},

	// 折算族 (VS-2).
	{TraceNoteKeyFoldBasis, "supply_fold", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySupplyFoldDeficitMS, "supply_fold", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySupplyFoldIdealMS, "supply_fold", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyFoldFmax, "supply_fold", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyFoldFmaxFinding, "supply_fold", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyFoldClusterLaneCaveat, "supply_fold", TraceNoteCarrierDisplayOnly},

	// 占用族 (RN-1) + CMP-9 density.
	{TraceNoteKeyStarvedRunnableMS, "occupancy", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyOccupier1, "occupancy", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyOccupier2, "occupancy", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyOccupier3, "occupancy", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyAlsoStarved, "occupancy", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyPressureDensity, "occupancy", TraceNoteCarrierDisplayOnly},

	// 阻塞族.
	{TraceNoteKeyBlockingKind, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyPeer, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHolderSite, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyWaiters, "blocking", TraceNoteCarrierHardConsumer},
	{"flags", "blocking", TraceNoteCarrierDisplayOnly},
	{"oneway", "blocking", TraceNoteCarrierDisplayOnly},
	{"sync_like", "blocking", TraceNoteCarrierDisplayOnly},
	{"blocking_candidate", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_state_dominant", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_state_total", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_state_running", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_state_runnable", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_state_sleep", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_state_d_state", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_state_io_wait", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_state_fragments", "blocking", TraceNoteCarrierDisplayOnly},

	// 门控族 (D3).
	{TraceNoteKeyGatedRunnable, "gating", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedRunningDeficit, "gating", TraceNoteCarrierHardConsumer},
	{"priority_inversion_gated", "gating", TraceNoteCarrierDisplayOnly},
	{"priority_inversion_candidate", "gating", TraceNoteCarrierDisplayOnly},
	{"priority_inversion_edges", "gating", TraceNoteCarrierDisplayOnly},
	{"priority_relation", "gating", TraceNoteCarrierDisplayOnly},

	// 语义跨度族.
	{TraceNoteKeySpanName, "span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySpanKind, "span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySpanCategory, "span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySpanSubcategory, "span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySemanticClass, "span", TraceNoteCarrierHardConsumer},

	// 引导族.
	{TraceNoteKeyNextStep, "guidance", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyNextStepKind, "guidance", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyRunnableCPU, "guidance", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyTopCompetitor, "guidance", TraceNoteCarrierSoftConsumer},
	{"top_competitor_overlap", "guidance", TraceNoteCarrierDisplayOnly},
	{"top_competitor_running", "guidance", TraceNoteCarrierDisplayOnly},

	// 采样族 (perf).
	{TraceNoteKeyPerfQuality, "perf", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyPerfQualityCaveats, "perf", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyDSO, "perf", TraceNoteCarrierSoftConsumer},
	{"perf_context", "perf", TraceNoteCarrierDisplayOnly},
	{"perf_contexts", "perf", TraceNoteCarrierDisplayOnly},
	{"symbol", "perf", TraceNoteCarrierDisplayOnly},
	{"symbolization_status", "perf", TraceNoteCarrierDisplayOnly},
	{"weight_unit", "perf", TraceNoteCarrierDisplayOnly},
	{"sample_weight", "perf", TraceNoteCarrierDisplayOnly},
	{"samples", "perf", TraceNoteCarrierDisplayOnly},
	{"percent", "perf", TraceNoteCarrierDisplayOnly},

	// 路径/链族.
	{TraceNoteKeyPath, "chain_path", TraceNoteCarrierSoftConsumer},
	{"target", "chain_path", TraceNoteCarrierDisplayOnly},
	{"edges", "chain_path", TraceNoteCarrierDisplayOnly},
	{"nodes", "chain_path", TraceNoteCarrierDisplayOnly},
	{"wakeup_ts", "chain_path", TraceNoteCarrierDisplayOnly},
	{"latency", "chain_path", TraceNoteCarrierDisplayOnly},
	{"waker_priority", "chain_path", TraceNoteCarrierDisplayOnly},
	{"wakee_priority", "chain_path", TraceNoteCarrierDisplayOnly},

	// 负载/约束族 (cpu load & affinity).
	{"thread", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"threads", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"process", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"cpu", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"cpus", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"core_class", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"core_classes", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"freq", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"priority", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"target_priority", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"prio", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"target_prio", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"high_prio_running", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"high_prio_overlap", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"high_prio", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"top_thread", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"top_thread_ms", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"top_background_threads", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"top_background_process", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"same_cpu_busy", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"same_cpu_idle", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"other_cpu_idle", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"verdict", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"constraint", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"kind", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"allowed_cpus", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"allowed_core_classes", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"cpuset", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"policy", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"observed_cpu", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"observed_core_class", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"migrations", "cpu_load", TraceNoteCarrierDisplayOnly},

	// 供给压力/算力族 (CMP-9/CMP-10).
	{"low_freq_cpus", "supply_pressure", TraceNoteCarrierDisplayOnly},
	{"clock_set_rate", "supply_pressure", TraceNoteCarrierDisplayOnly},
	{"thermal", "supply_pressure", TraceNoteCarrierDisplayOnly},
	{"ddr", "supply_pressure", TraceNoteCarrierDisplayOnly},
	{"l3", "supply_pressure", TraceNoteCarrierDisplayOnly},
	{"throughput", "supply_pressure", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeySupplyRatio, "compute_supply", TraceNoteCarrierSoftConsumer},
	{"delivered_cpu_ms", "compute_supply", TraceNoteCarrierDisplayOnly},
	{"low_freq_loss_cpu_ms", "compute_supply", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyIdleMismatchMS, "compute_supply", TraceNoteCarrierSoftConsumer},
	{"core_limited_cpu_ms", "compute_supply", TraceNoteCarrierDisplayOnly},
	{"cpu_count", "compute_supply", TraceNoteCarrierDisplayOnly},

	// IO 族.
	{"inode", "io", TraceNoteCarrierDisplayOnly},
	{"dev", "io", TraceNoteCarrierDisplayOnly},
	{"name", "io", TraceNoteCarrierDisplayOnly},
	{"op", "io", TraceNoteCarrierDisplayOnly},
	{"count", "io", TraceNoteCarrierDisplayOnly},
	{"completions", "io", TraceNoteCarrierDisplayOnly},
	{"bytes", "io", TraceNoteCarrierDisplayOnly},
	{"total_latency", "io", TraceNoteCarrierDisplayOnly},
	{"max_latency", "io", TraceNoteCarrierDisplayOnly},
	{"avg_latency", "io", TraceNoteCarrierDisplayOnly},
	{"ret", "io", TraceNoteCarrierDisplayOnly},
	{"offsets", "io", TraceNoteCarrierDisplayOnly},
	{"example", "io", TraceNoteCarrierDisplayOnly},
	{"adds", "io", TraceNoteCarrierDisplayOnly},
	{"deletes", "io", TraceNoteCarrierDisplayOnly},
	{"churn", "io", TraceNoteCarrierDisplayOnly},
	{"layer", "io", TraceNoteCarrierDisplayOnly},
	{"event", "io", TraceNoteCarrierDisplayOnly},
	{"paired", "io", TraceNoteCarrierDisplayOnly},
	{"unpaired_start", "io", TraceNoteCarrierDisplayOnly},
	{"unpaired_done", "io", TraceNoteCarrierDisplayOnly},
	{"signal", "io", TraceNoteCarrierDisplayOnly},
	{"block_max", "io", TraceNoteCarrierDisplayOnly},
	{"storage_max", "io", TraceNoteCarrierDisplayOnly},
	{"file_bytes", "io", TraceNoteCarrierDisplayOnly},
	{"file_events", "io", TraceNoteCarrierDisplayOnly},
	{"page_cache_churn", "io", TraceNoteCarrierDisplayOnly},
	{"iowait_blocked", "io", TraceNoteCarrierDisplayOnly},
	{"top_inode", "io", TraceNoteCarrierDisplayOnly},
	{"top_dev", "io", TraceNoteCarrierDisplayOnly},
	{"top_name", "io", TraceNoteCarrierDisplayOnly},
	{"block_dev", "io", TraceNoteCarrierDisplayOnly},
	{"nearest_block_thread", "io", TraceNoteCarrierDisplayOnly},
	{"line", "io", TraceNoteCarrierDisplayOnly},
	{"callstack", "io", TraceNoteCarrierDisplayOnly},

	// 中断族.
	{"vector", "interrupt", TraceNoteCarrierDisplayOnly},
	{"max", "interrupt", TraceNoteCarrierDisplayOnly},
	{"target_mask", "interrupt", TraceNoteCarrierDisplayOnly},
	{"target_cpus", "interrupt", TraceNoteCarrierDisplayOnly},

	// 内核账目/工作队列/围栏族.
	{"delay", "sched_accounting", TraceNoteCarrierDisplayOnly},
	{"max_delay", "sched_accounting", TraceNoteCarrierDisplayOnly},
	{"runtime", "sched_accounting", TraceNoteCarrierDisplayOnly},
	{"max_runtime", "sched_accounting", TraceNoteCarrierDisplayOnly},
	{"work", "workqueue", TraceNoteCarrierDisplayOnly},
	{"function", "workqueue", TraceNoteCarrierDisplayOnly},
	{"driver", "dma_fence", TraceNoteCarrierDisplayOnly},
	{"timeline", "dma_fence", TraceNoteCarrierDisplayOnly},
	{"context", "dma_fence", TraceNoteCarrierDisplayOnly},
	{"seqno", "dma_fence", TraceNoteCarrierDisplayOnly},

	// 插件族 (Ability/XPower/HiSystemEvent).
	{"domain", "plugin", TraceNoteCarrierDisplayOnly},
	{"metric", "plugin", TraceNoteCarrierDisplayOnly},
	{"value", "plugin", TraceNoteCarrierDisplayOnly},
	{"category", "plugin", TraceNoteCarrierDisplayOnly},

	// 账本标记族 (ledger-lane markers; emitted by the observation-ledger
	// compile, not by trace_query — the tool-side emit pin skips this family
	// and the consumer pin covers the append/exact-match sites instead).
	{TraceNoteKeyAdvisoryPretriage, "ledger_marker", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyDeterministicRuntimeQueryPresent, "ledger_marker", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyLegacySummaryFallback, "ledger_marker", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyNotAnswerGrade, "ledger_marker", TraceNoteCarrierDisplayOnly},
}

// TraceNoteKeyRows returns a copy of the full registry table.
func TraceNoteKeyRows() []TraceNoteKeyRow {
	out := make([]TraceNoteKeyRow, len(traceNoteKeyRows))
	copy(out, traceNoteKeyRows)
	return out
}

var traceNoteKeyIndex = func() map[string]TraceNoteKeyRow {
	index := make(map[string]TraceNoteKeyRow, len(traceNoteKeyRows))
	for _, row := range traceNoteKeyRows {
		index[row.Key] = row
	}
	return index
}()

// TraceNoteKeyRegistered reports whether key is a registered rich-note key.
// Exact match only — the occupier roster keys are registered individually
// (occupier_1..occupier_3), matching the producer's hard cap of 3.
func TraceNoteKeyRegistered(key string) bool {
	_, ok := traceNoteKeyIndex[key]
	return ok
}

// TraceNoteKeyLookup returns the registry row for key.
func TraceNoteKeyLookup(key string) (TraceNoteKeyRow, bool) {
	row, ok := traceNoteKeyIndex[key]
	return row, ok
}
