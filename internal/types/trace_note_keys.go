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
	TraceNoteKeyRank = "rank"
	TraceNoteKeyTier = "tier"
	// TraceNoteKeyBackgroundRank (DCS E1b/E6, ledger §23.1 rulings ②/③,
	// 2026-07-08): the row's 1-based typed 榜位 on the non-on-chain composite
	// board. Emitted on semantic compile span rank rows only; the prose
	// mention-obligation double gate reads background_rank<=3 for non-chain
	// optimization spans — never a prose position guess.
	TraceNoteKeyBackgroundRank = "background_rank"
	TraceNoteKeyType           = "type"
	TraceNoteKeySource         = "source"
	TraceNoteKeyCausality      = "causality"
	TraceNoteKeyChainRelevance = "chain_relevance"
	// TraceNoteKeyOnChainBasis (SELF-SEM §29.61.1 + SELF-ALL §29.61.2 user
	// rulings 2026-07-13): the typed proof basis behind
	// chain_relevance=on_chain. Closed set — absent = legacy chain-window
	// overlap basis; "self_deterministic_span" = the analysis target's own
	// deterministic semantic span(s) admitted to the on-chain channel WITHOUT
	// chain-window overlap and WITHOUT any wakeup-edge claim (the row's
	// causality then reads "self_deterministic", never "on_wakeup_chain");
	// "self_wall_clock_interval" = the target's own WALL-CLOCK seat
	// (blocked-state family / IO facet / runnable / running) admitted the same
	// way (causality "self_wall_clock"); "host_wakeup_edge_pre_span" (R3-IMPL
	// §29.88.1, 2026-07-15) = a NON-target host's deterministic semantic span
	// seated by the HOST's own in-window typed wakeup edge toward the target
	// (causality keeps the honest "on_wakeup_chain" — a real edge exists; the
	// host_wakeup_edge_anchor_ts/-via pair rides beside it);
	// "host_wakeup_edge_pre_state" (ONCHAIN-3c, 2026-07-19) = a NON-target,
	// NON-chain-member host's runnable / D-IO STATE seat anchored by the same
	// credential (value = the segment inventory's pre-edge share sum; same
	// honest causality; same ts/-via pair). Emitted on the
	// causal_rank family AND the critical_blocking / io_burst_episode
	// witness-feeder records (one 道别 predicate, three producers). The
	// projection compile parses it into TraceCausalProjectionNode.OnChainBasis;
	// the 「目标自身·确定性优化」/「目标自身·墙钟席」 display qualifiers and the R3
	// 唤醒锚定(宿主→目标) sentence fork on THIS single field.
	TraceNoteKeyOnChainBasis = "on_chain_basis"
	TraceNoteKeyChainDepth   = "chain_depth"
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
	// TraceNoteKeyTraceGapKind (G2 判据 typed 化, §27.2/§28.1, 2026-07-09;
	// carrier promoted display→hard by the DISP-2 display half, Wave-3.2):
	// the precise blind-spot criterion enum on a Type=trace_gap rank row —
	// no_sched_data (the thread timeline holds no interval at all in the
	// aligned window) / no_eligible_wait (intervals exist but ALL sit below
	// the MinDurationMs floor). The projection compile parses it into
	// TraceCausalProjectionNode.TraceGapKind and the ◇ row wording forks on
	// it — a typo now silently kills the blind-spot wording fork.
	TraceNoteKeyTraceGapKind = "trace_gap_kind"
	// PTS 折叠族 (#68 用户裁定 2026-07-05, 零静默丢弃): a producer-side fold
	// record represents the on-chain rows beyond the per-family wire cap —
	// folded_rows counts the folded ROWS, folded_min_ms/folded_max_ms carry
	// their display range (the record's own value is the member MAX; wall
	// clock never sums across threads) and folded_subjects lists up to 8
	// member thread labels comma-separated. Consumed by the projection compile
	// (MergedCount/MergedMinMS/MergedMaxMS/MergedSubjects re-materialization).
	TraceNoteKeyFoldedRows     = "folded_rows"
	TraceNoteKeyFoldedMinMS    = "folded_min_ms"
	TraceNoteKeyFoldedMaxMS    = "folded_max_ms"
	TraceNoteKeyFoldedSubjects = "folded_subjects"
	// TraceNoteKeyFoldedMaxSubject / TraceNoteKeyFoldedMaxStateKind (A2 件5②,
	// §29.179 A 批委托, 2026-07-21): the label + dominant state of the member
	// holding folded_max_ms — the wire carriers behind the RUN2FIX-A 件2
	// max-member disclosure (「成员最大 <线程> · <状态> <值>ms」) on wire-fold
	// re-materialized rows (the E19-class gap: display-side folds carried
	// MergedMaxSubject while the engine folds carried none, so the wire-fold
	// row could only show a range). Producers emit both keys only when the max
	// member has a non-empty label (宁漏勿假); the projection compile
	// re-materializes them into MergedMaxSubject/MergedMaxStateKind on the
	// SAME all-or-nothing discipline.
	TraceNoteKeyFoldedMaxSubject   = "folded_max_subject"
	TraceNoteKeyFoldedMaxStateKind = "folded_max_state_kind"
	// TraceNoteKeySameValueMembers (DIAG A1, §28.11-3(a) G12,
	// real_trace_campaign_20260705.md, 2026-07-09): rides beside the folded_*
	// family on a producer-side cross-thread take-MAX fold record when ≥2
	// members tie the published MAX to the µs (strict
	// TraceCausalProjectionSameValueTieMS band) — the suspected same-segment
	// double-attribution witness (huadong_79 E23: hmfs_discard + target thread
	// both 14.272ms). Value is a comma-joined roster of
	// "<subject>@<line_start>-<line_end>" entries (cap 4; subjects follow the
	// folded_subjects comma convention). Hard consumer: the projection compile
	// re-materializes it into TraceCausalProjectionNode.SameValueMembers for
	// the audit-token face. Disclosure only — never a fold-value input.
	TraceNoteKeySameValueMembers = "same_value_members"
	// RCM 家族合并族 (§24.7.1/§24.10 user rulings 2026-07-08,
	// real_trace_campaign_20260705.md §24.12): the ENGINE-side same-(thread,
	// type) / (thread, semantic class) family-merge carriers on a rank
	// observation — member_count counts the merged member INSTANCES, member_
	// max_ms/member_min_ms their raw value range, member_sum_ms the lossless
	// raw Σ (emitted ONLY when the published value sits below it — the
	// union/max-fallback disclosure), member_fold_caliber the closed-set typed
	// ruler that produced the published value (sum_disjoint / interval_union /
	// max_overlap_fallback / count_sum) and member_roster the bounded
	// "key value" member inventory joined with " | " (entries may contain
	// commas — consumers split on the pipe, never on ","). DELIBERATELY a
	// separate lane from the display-side folded_* family above: folded_* is a
	// CROSS-THREAD wire-cap fold whose value is the member MAX; member_* is a
	// SAME-THREAD engine merge whose value is legally additive — the
	// projection compile re-materializes them into the FamilyMember* node
	// fields and MUST NOT touch the MergedCount/MergedMaxMS lane (the display
	// lead selector folds Merged* rows to their member MAX, which would
	// collapse the family total back to its largest member).
	TraceNoteKeyMemberCount       = "member_count"
	TraceNoteKeyMemberMaxMS       = "member_max_ms"
	TraceNoteKeyMemberMinMS       = "member_min_ms"
	TraceNoteKeyMemberSumMS       = "member_sum_ms"
	TraceNoteKeyMemberFoldCaliber = "member_fold_caliber"
	TraceNoteKeyMemberRoster      = "member_roster"
	// TraceNoteKeySelfGapSemanticOverlaps (XLANE-2 件2, user ruling
	// §29.104.17 ④ 披露式拆分, 2026-07-17): the self running supply-fold
	// deficit seat's semantic-overlap disclosure — per-partner
	// "overlapMs@lineStart..lineEnd" entries joined with "|" (overlap DESC,
	// engine cap 6). The projection compile parses entries independently
	// (each clause is its own truth; invalid entries drop, never guess) into
	// SelfGapSemanticOverlaps; the display resolves each partner's [E#] from
	// the line envelope and renders the 「其中 X ms 与语义席[E#]重叠」 clause.
	// Pure disclosure — no value channel, gate, score or sort lane reads it.
	TraceNoteKeySelfGapSemanticOverlaps = "self_gap_semantic_overlaps"
	// AXIOM-V2 (user rulings 2026-07-18) — the fix-direction attribute axis
	// plus the cross-direction overlap disclosure family:
	//
	//   - fix_direction: the registry repair-direction token of the row's
	//     causal type (closed set: scheduling_supply / lock_priority /
	//     io_dependency / memory / frequency_thermal / self_workload;
	//     unresolved never emits — absence = fail-open). Attribute axis only
	//     (方向为属性轴,序数芯片本体零动): the display 行2 direction word
	//     forks on it; no gate/ordinal/value lane reads it.
	//   - cross_direction_overlaps: the 件2 typed pair table — per-partner
	//     "overlapMs@lineStart..lineEnd@direction@basis" entries joined with
	//     "|" (overlap DESC, engine cap 6, SYMMETRIC across the pair). The
	//     projection compile parses entries independently; the display
	//     resolves each partner's [E#] verbatim from the line envelope and
	//     renders the 互指句 (「同段重叠 X ms…收益不叠加」) on BOTH seats or
	//     neither (宁漏勿假指). 口径词=同段重叠; identity: overlap ≤ min of
	//     the two seats' support unions.
	//   - cross_direction_overlap_undisclosed: 件3 closure — partner TYPE
	//     tokens ("|"-joined) of detected cross-direction overlaps whose
	//     mutual-pointer carrier is absent (no line envelope / roster cap);
	//     audit disclosure only, never an [E#] claim.
	//   - direction_conservation_excess: the 件3 violation finding
	//     ("direction@sumMs@windowMs@seatCount") stamped on every member seat
	//     of a (thread, direction) population whose Σ of support-interval
	//     unions exceeds the physical window (公理 v2 违宪形). Pure
	//     disclosure / 立案素材 — emission always proceeds (永不硬拦).
	//     ELIM-V2 (2026-07-18): the projection compile now parses the tuple
	//     (Node.DirectionConservationExcess, strict whole-tuple) so the ◎
	//     守恒尾行 can transcribe the per-direction violation line; still a
	//     disclosure-only lane (no gate/ordinal/value consumer).
	TraceNoteKeyFixDirection                     = "fix_direction"
	TraceNoteKeyCrossDirectionOverlaps           = "cross_direction_overlaps"
	TraceNoteKeyCrossDirectionOverlapUndisclosed = "cross_direction_overlap_undisclosed"
	TraceNoteKeyDirectionConservationExcess      = "direction_conservation_excess"
	// TraceNoteKeyMemberLineRanges (XLANE-2 件1, §29.104.1/.2 定谳④,
	// 2026-07-17): the semantic family seat's COMPLETE per-member trace line
	// ranges — "start..end" entries joined with "|", member order, minted
	// all-or-nothing at the engine (any line-less member or an over-cap family
	// mints nothing, so a consumer can never see a truncated set). The
	// projection compile parses it into FamilyMemberLineRanges ONLY when the
	// entry count equals member_count (strict; anything else drops the whole
	// set). Display-side 成员子集 subset-judgment input only (the
	// 「为[E#]成员子集」 demotion lane) — no gate, score or sort lane reads it.
	TraceNoteKeyMemberLineRanges = "member_line_ranges"
	// TraceNoteKeyMemberWallMS (SPANTOP-1 件1, §29.131, 2026-07-18): the
	// semantic family seat's COMPLETE per-member in-window wall-clock
	// durations — "%.3f" entries joined with "|", member order (the same
	// order as member_line_ranges), minted all-or-nothing at the engine (any
	// non-positive member or an over-cap family mints nothing). The
	// projection compile parses it into FamilyMemberWallMS ONLY when the
	// entry count equals member_count and every entry decodes to a positive
	// float (strict; anything else drops the whole list). Display-side
	// constituent top-3 sub-row input only, and only after the display's own
	// µs identity gate (Σ members == the seat's 行1 value) passes — no gate,
	// score or sort lane reads it.
	TraceNoteKeyMemberWallMS = "member_wall_ms"
	// P3MEASURE-1 键族 (§29.169 user ruling chain, 2026-07-20) — the
	// ONCHAIN-P3 stage-one SILENT on-chain seat measurement wire:
	//
	//   - p3m_counterfactual_valid_ms / p3m_counterfactual_invalid_ms: the
	//     seat's anchor-window time split by the COUNTERFACTUAL caliber
	//     (缩短此段⇒边前移⇒目标提前醒). Identity: µs(valid)+µs(invalid) ==
	//     the seat's anchor-window time exactly; an absent key reads as 0.
	//     invalid = typed counterexample family ① only (periodic/absolute-
	//     deadline-pinned closing edges, judged by the EXISTING VS-1
	//     periodic-source classification — no new heuristic; ruled-legal
	//     兼服/边前继承 forms count VALID, §29.169 red line).
	//   - p3m_edge_witnessed_ms: the structural edge-witnessed share (P3
	//     原结构口径, onchain_segment_audit_20260718.md 同源 census join);
	//     always ≤ the seat's published value.
	//   - p3m_disposition: the closed measurement-form token
	//     (measured_segment_join / measured_edge_terminated_window /
	//     measured_counterfactual_only / self_ruled / no_typed_inventory /
	//     no_anchor_windows / occurrence_inventory_capped).
	//   - p3m_coverage: the measurement's own family-coverage disclosure —
	//     verbatim "families:[periodic_pinned]" (family ② late-relay has no
	//     typed criterion this round and is honestly OUT of coverage).
	//
	// ── ADVISORY-ONLY RED LINE (supply_pressure 分离先例) ──────────────
	// These keys are SILENT AUDIT MEASUREMENT, carrier display_only with NO
	// parsing consumer and NO rendered face — model/user double-invisible
	// (双不可见 is pinned by four-flagship-board A/B byte identity). They
	// MUST NEVER drive a hard gate, a lane/seat/ordinal/score decision, or
	// any user/model wording. Stage two (data-gated disclosure) requires a
	// NEW user ruling and must speak 见证下界 semantics, never a ratio form
	// (§29.169). Promoting any of these keys off display_only reddens the
	// carrier pin — that red IS the review surface for that future ruling.
	TraceNoteKeyP3MCounterfactualValidMS   = "p3m_counterfactual_valid_ms"
	TraceNoteKeyP3MCounterfactualInvalidMS = "p3m_counterfactual_invalid_ms"
	TraceNoteKeyP3MEdgeWitnessedMS         = "p3m_edge_witnessed_ms"
	TraceNoteKeyP3MDisposition             = "p3m_disposition"
	TraceNoteKeyP3MCoverage                = "p3m_coverage"
)

// 业务 span 提及族 (SPANVIS-1, user ruling 2026-07-19 定形原则): the
// pure-advisory business-lens span mention face. One business_span_mention
// observation record per admitted (thread, verbatim span name) family; the
// record is a projection-level SIDE CHANNEL (never a node, never a seat, no
// ordinal/conservation/census membership — 不参与根因排序). The projection
// compile parses a record ALL-OR-NOTHING: every key below except the omitted
// counter must be present and strictly valid or the whole record is dropped
// (fail-open to absence; the face never publishes a partially-typed row).
const (
	// TraceNoteKeyBusinessSpanName — the verbatim span name (typed family
	// key; value carried verbatim after '=', outer whitespace trimmed).
	TraceNoteKeyBusinessSpanName = "business_span_name"
	// TraceNoteKeyBusinessSpanCount — admitted member count (int ≥ 1).
	TraceNoteKeyBusinessSpanCount = "business_span_count"
	// TraceNoteKeyBusinessSpanTotalMS — Σ in-window member durations ("%.3f").
	TraceNoteKeyBusinessSpanTotalMS = "business_span_total_ms"
	// TraceNoteKeyBusinessSpanMaxMS — largest single member ("%.3f").
	TraceNoteKeyBusinessSpanMaxMS = "business_span_max_ms"
	// TraceNoteKeyBusinessSpanLines — member line envelope "start..end".
	TraceNoteKeyBusinessSpanLines = "business_span_lines"
	// TraceNoteKeyBusinessSpanBasis — closed-set on-chain credential token
	// (self / chain_member / host_wakeup_edge).
	TraceNoteKeyBusinessSpanBasis = "business_span_basis"
	// TraceNoteKeyBusinessSpanHidden — members below the bounded display view
	// (int 0..Count since POOL2-1 §29.160①: 0 = fully-visible admitted family,
	// published explicitly; the strict parser requires the key's presence).
	TraceNoteKeyBusinessSpanHidden = "business_span_hidden"
	// TraceNoteKeyBusinessSpanOmitted — admitted families beyond the mention
	// cap (件3 截断诚实披露; only ≥floor families count). Rides every mention
	// record of the result with the same value; parse takes the first.
	TraceNoteKeyBusinessSpanOmitted = "business_span_omitted"
)

// RCM 区分键族 (§24.7.1 ①/§24.9-B F3, 2026-07-08): the typed real
// distinguishing keys of the inode-keyed IO rank families. EVOLUTION RECORD:
// both key names existed as display-tier "io"-family literals (emitted on the
// io observation rows); the rank lane now also emits them from the typed
// RootCauseRankItem.Inode/Dev fields and the projection compile parses them
// into node fields — promoted literals → contract-tier constants,
// display_only → hard_consumer.
const (
	TraceNoteKeyInode = "inode"
	TraceNoteKeyDev   = "dev"
)

// IO pressure caliber family (CBZ-B2-IO-CALIBER, 2026-07-23). These notes
// carry the exact aggregate constituents and the evidence-quality ceiling
// into the system-authored projection. They are display/authority inputs:
// no rank, retry or hard admission gate reads them.
const (
	TraceNoteKeyIOPressureSignal             = "io_pressure_signal"
	TraceNoteKeyIOPressureEvidenceQuality    = "io_pressure_evidence_quality"
	TraceNoteKeyIOPressureScoreCaliber       = "io_pressure_score_caliber"
	TraceNoteKeyIOPressureConclusion         = "io_pressure_conclusion"
	TraceNoteKeyIOPressureIOWaitBlockedCount = "io_pressure_iowait_blocked_count"
	TraceNoteKeyIOPressureBlockMaxMS         = "io_pressure_block_max_ms"
	TraceNoteKeyIOPressureStorageMaxMS       = "io_pressure_storage_max_ms"
	TraceNoteKeyIOPressureFileBytes          = "io_pressure_file_bytes"
	TraceNoteKeyIOPressureFileEvents         = "io_pressure_file_events"
	TraceNoteKeyIOPressurePageCacheChurn     = "io_pressure_page_cache_churn"
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
	// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16): the composite-score twins of
	// the ms-semantic value keys above. A composite-score rank row
	// (tracequery.CausalTokenCompositeValueWire — io_pressure /
	// block_io_by_inode) publishes its magnitude EXCLUSIVELY under these keys
	// (one row emits exactly one family; parsers read the union), because the
	// value is a composite score over mixed units, never wall-clock ms.
	TraceNoteKeyImpactScore           = "impact_score"
	TraceNoteKeyCumulativeImpactScore = "cumulative_impact_score"
	TraceNoteKeyEffectiveImpactScore  = "effective_impact_score"
	TraceNoteKeyActualImpactMS        = "actual_impact_ms"
	TraceNoteKeyActualImpact          = "actual_impact"
	TraceNoteKeyTotal                 = "total"
	TraceNoteKeyActualTotalMS         = "actual_total_ms"
	TraceNoteKeyActualTotal           = "actual_total"
	// TraceNoteKeyProjectedImpact / TraceNoteKeyOverlap — EVOLUTION RECORD
	// (审计 #5/#62, §29.25 处置委托 + §29.26 待主会话落账, 2026-07-10):
	// display_only → hard_consumer. On an ON-CHAIN trace_semantic_span record
	// they carry the engine's exact member∩chain intersection union — the ONE
	// participation value the rank lane publishes as EffectiveImpactMs after
	// the SEM-LEAD intersection caliber (§24.10 参赛值=窗口投影合计 evolved to
	// the on-chain intersection; the complete member union stays on Value/
	// cumulative). The projection compile promotes them into
	// TraceCausalProjectionNode.SemanticChainProjectedMS so the E9/E13
	// twin-seat fold can mirror rank participation against the SAME value
	// (union≠intersection on partial-overlap families broke the old
	// display-impact mirror structurally) and so the ✦ row's 有效归因 label
	// never falls back to the bare union. Producers: the semantic family
	// observation emits both keys; the single-span observation emits overlap.
	TraceNoteKeyProjectedImpact = "projected_impact"
	TraceNoteKeyOverlap         = "overlap"
	// TraceNoteKeyActualCaliberNote (DIAG A2, §28.11-3(b) D-10,
	// real_trace_campaign_20260705.md, 2026-07-09): the producer's typed
	// two-caliber divergence disclosure — value is the closed enum
	// TraceActualCaliberStateSegmentVsThreadTotal, emitted ONLY when the same
	// row publishes BOTH the dominant-state segment actual (actual_impact
	// lane) and the thread-level actual total (actual_total lane) and they
	// diverge by more than 10% of the larger (opendir_79 E5: 表面 "实际状态
	// 59.050ms" beside "actual_total=112.234ms" read as a contradiction).
	// Hard consumer: the projection compile parses it into
	// TraceCausalProjectionNode.ActualCaliberNote and the detail stanza's
	// 实际口径 line keys on it. Neither value is judged or edited (不猜哪个对).
	TraceNoteKeyActualCaliberNote = "actual_caliber_note"
	// TraceNoteKeyTargetImpactMS / TraceNoteKeyTargetImpact carry the engine's
	// TargetBlockedMs caliber — how much of the 🎯 target's own blocked wall
	// clock THIS row's chain actually explains (rank lane emits
	// target_impact_ms=%.3f, the causal_impact/aggregated_impact lanes carry the
	// summary field target_impact=%.3fms verbatim). COV §24.9 D-1
	// (real_trace_campaign_20260705.md, opendir_78): promoted from display-only
	// to a typed consumer key so the coverage-sentence numerator can consume the
	// 已由链上解释 semantic instead of the §20.1 display-overwritten cumulative.
	TraceNoteKeyTargetImpactMS = "target_impact_ms"
	TraceNoteKeyTargetImpact   = "target_impact"
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
	TraceNoteKeyDominantState = "dominant_state"
	TraceNoteKeyRunning       = "running"
	TraceNoteKeyRunnable      = "runnable"
	TraceNoteKeySleep         = "sleep"
	TraceNoteKeyDState        = "d_state"
	TraceNoteKeyIOWait        = "io_wait"
	// TraceNoteKeySleepIOWait (§29.27② COV-4 复核 A-1, 2026-07-11): the
	// target_window_states record's sleep-side IO refinement — the wall
	// clock of S-opened sleep intervals whose wakeup paired an iowait>0
	// sched_blocked_reason marker (G12 §29.13 Harmony platform IO-wait
	// form). Already contained in the sleep lane; NEVER an addend. Consumed
	// by the projection compile into TargetStateAccount.SleepIOWaitMS for
	// the sleep term's 「其中 IO等待」 label.
	TraceNoteKeySleepIOWait = "sleep_io_wait"
	// TraceNoteKeyHeadCarry* / TraceNoteKeyTailOpen* (§29.140 G6,
	// ANSWERFACE-1 件2, 2026-07-19): the target_window_states boundary-fold
	// disclosure quartet — the window-head prefix carried from the recovered
	// pre-window scheduler state (head_carry_*) and the window-tail suffix
	// flushed from the final still-open interval with no in-window closing
	// event (tail_open_*). Each ms value is ALREADY inside the account lane
	// its paired *_state key names (running/runnable/sleep/d_state/io_wait)
	// — disclosure only, never an addend. Consumed by the projection compile
	// into TraceCausalProjectionTargetStateAccount.HeadCarry*/TailOpen* for
	// the four-state row's 「含未覆盖段…折入」 term annotation (禁静默折入).
	TraceNoteKeyHeadCarryMS    = "head_carry_ms"
	TraceNoteKeyHeadCarryState = "head_carry_state"
	TraceNoteKeyTailOpenMS     = "tail_open_ms"
	TraceNoteKeyTailOpenState  = "tail_open_state"
	// TraceNoteKeyDStateRefinedNonIO (DSTATE-REFINE arm a, CAL-1 件③
	// §29.39②/§29.47.2, 2026-07-12): the merged D/IO rank row's typed
	// refinement proof — "true" ONLY when the row's io_wait share is zero AND
	// every member segment on the D ledger carried a sched_blocked_reason
	// marker with iowait=0 (blocked_reason 全覆盖∧全0). Consumed by the
	// projection compile into TraceCausalProjectionNode.DStateRefinedNonIO —
	// the display's refined 「D-state」 word gate (coverage-less rows keep the
	// honest merged 「D-state/iowait」 form).
	TraceNoteKeyDStateRefinedNonIO = "dstate_all_noniowait"
	// TraceNoteKeyBlockedReasonCaller (件③ caller 等待对象族, 2026-07-12): the
	// UNANIMOUS semantic caller symbol of the row's D-ledger blocked_reason
	// markers (dma_fence_default_wait family; witness CompThread 12/12).
	// Emitted only when every marked member agrees on ONE caller; consumed by
	// the projection compile into TraceCausalProjectionNode.BlockedReasonCaller
	// for the 行2 等待对象 disclosure. Distinct from the lock-lane
	// wait_object key (different producer lane, different semantics).
	TraceNoteKeyBlockedReasonCaller = "blocked_reason_caller"
	// TraceNoteKeyBlockedReasonWindowCount / ...WindowCaller (CR-3 件② P10,
	// 2026-07-12): the UNCONSUMED sched_blocked_reason residual on a D-family
	// rank row whose unanimous-caller lane minted nothing — the thread's
	// in-window marker count and the distinct semantic symbols ("/"-joined,
	// cap 2). Consumed by the projection compile into
	// TraceCausalProjectionNode.BlockedReasonWindowCount/-Caller for the
	// 「该行标未解析,但窗内存在 N 条 blocked_reason 记录」 disclosure (冷读
	// 案7 GPU-fence witness). Absent whenever the row consumed its marker
	// (blocked_reason_caller) or the window holds none.
	TraceNoteKeyBlockedReasonWindowCount  = "blocked_reason_window_count"
	TraceNoteKeyBlockedReasonWindowCaller = "blocked_reason_window_caller"
	// TraceNoteKeyBlockedReasonCensus / ...CensusOverflow (件1 census 根修,
	// 修复轮 2026-07-13): ONE thread's pid-keyed full-window blocked_reason
	// census — per-caller 符号×count×Σms entries "sym×N(Σx.xxxms)" joined by
	// "/" (Σms only when every row of the caller carried a vendor delay
	// field), off the FULL pre-truncation accumulator (never the top-8
	// display view — 复核实锤: the old model-face census read the display
	// truncation and under-reported split-offset symbols). The overflow key
	// carries the count of DISTINCT caller symbols beyond the per-pid cap.
	// Consumed deterministically by the model evidence feed
	// (internal/context wait-object summary).
	TraceNoteKeyBlockedReasonCensus         = "blocked_reason_census"
	TraceNoteKeyBlockedReasonCensusOverflow = "blocked_reason_census_overflow"
	// TraceNoteKeyWakeupEdgeCensus* (WAKE-CENSUS §29.58, 2026-07-13): the
	// per-(waker → wakee) wakeup-edge census notes riding each typed
	// wakeup_edge_census record (Subject=waker, Object=wakee, Value=count).
	// The count folds over the engine's FULL pre-cap edge set — the per-edge
	// wakeup_chain_edge rows are row-capped, so counts re-derived from them
	// are silent lower bounds (PRC-F1 witness: the model invented
	//「OS_IPC_14_34911 ×4」for a pair whose only raw edge ran the opposite
	// direction). First/last carry the pair's observed wakeup timestamp
	// bounds; the overflow pair carries the census pair-cap trim (distinct
	// pairs + their deduplicated edges beyond the listed rows; absent ⇔ 0 ⇔
	// the pair enumeration is complete). Consumed deterministically by the
	// model evidence feed (internal/context wait-object summary).
	// EVOLUTION RECORD (WAKE-CENSUS-D 2A 换源, §29.58.4, RANK-U Stage 1
	// 2026-07-13): the count caliber strengthened from "FULL pre-cap edge
	// set" to WINDOW-TOTAL raw sched_wakeup rows for the chain-thread wakee
	// set (target ∪ chain nodes) — D-exit and off-expansion-path S-exit
	// wakeups now count (the donghu gpu-token ×12 structural absence closed).
	// The exit-split trio partitions each pair's count exactly by the
	// scheduler state the wakee left (sleep / D-family / other-or-
	// unclassified — 双加恒等式); measurement-face counts only, the D-state
	// CAUSAL lane stays with sched_blocked_reason.
	TraceNoteKeyWakeupEdgeCensusFirstTs       = "wakeup_edge_census_first_ts"
	TraceNoteKeyWakeupEdgeCensusLastTs        = "wakeup_edge_census_last_ts"
	TraceNoteKeyWakeupEdgeCensusOverflowPairs = "wakeup_edge_census_overflow_pairs"
	TraceNoteKeyWakeupEdgeCensusOverflowEdges = "wakeup_edge_census_overflow_edges"
	TraceNoteKeyWakeupEdgeCensusSleepExit     = "wakeup_edge_census_sleep_exit"
	TraceNoteKeyWakeupEdgeCensusDExit         = "wakeup_edge_census_d_exit"
	TraceNoteKeyWakeupEdgeCensusOtherExit     = "wakeup_edge_census_other_exit"
	// TraceNoteKeyWakeupEdgeCensusTargetWakee (修复轮 件2, 2026-07-13):
	// "true" iff THIS census pair's wakee is the publishing RESULT's own
	// analysis target — that wakee's pair set is pair-cap immune on the
	// engine and tool faces by construction (件5), so its enumeration is
	// complete even when the scope overflowed. The context TOTAL lead's
	// anchor arm reads exactly this per-result marker — never the
	// session-global anchor flag (a T1 anchor must not vouch for a T2
	// result's trimmed pair set).
	TraceNoteKeyWakeupEdgeCensusTargetWakee = "wakeup_edge_census_target_wakee"
	// TraceNoteKeyDStateCauseUnprovenRemainder (§29.50.5 证明分区, v5 P1 批
	// 件②, 2026-07-13): "true" ONLY on the honest-remainder D/IO seat — the
	// unproven fragments of a thread whose other fragments proved a concrete
	// wait object and were carved into sibling cause seat(s) (逐片段证明门;
	// 绝不灌根因席). Consumed by the projection compile into
	// TraceCausalProjectionNode.DStateCauseUnprovenRemainder — the display's
	// 「D-state(原因未证)」 remainder word gate. A thread with no cause seat
	// never emits it (a lone generic seat is not a remainder).
	TraceNoteKeyDStateCauseUnprovenRemainder = "dstate_cause_unproven_remainder"
	// TraceNoteKeyChainAnchored / TraceNoteKeyChainAnchorFull /
	// TraceNoteKeyChainAnchorRemainderSeat (RSPA §29.61.10a/b/c, 2026-07-14):
	// the on-chain seat-value re-anchoring decomposition. A migrated chain
	// thread's window state seat splits into the same-source bipartition —
	// the ⛓ anchored portion (segments ∩ typed wakeup-dependency jump
	// windows; owned by the chain-lane seat or the clipped window seat) and
	// the ◇ remainder (no chain credential). chain_anchored_ms + the row's
	// published value channels reconstruct the full account exactly:
	// full = anchored + remainder (同源二分,唯一可相加还原形 — wall clock
	// across DIFFERENT accounts stays non-additive). remainder_seat="true"
	// marks the ◇ half; the ⛓ half carries the same two floats with the
	// marker absent. Consumed by the projection compile into
	// TraceCausalProjectionNode.ChainAnchoredMS/ChainAnchorFullMS/
	// ChainAnchorRemainderSeat — the 行2 「全窗X=锚定Y+其余Z」 decomposition
	// and the WO-C1 同源二分 relation sentence read these; never a
	// rank/score input (values were re-derived engine-side).
	TraceNoteKeyChainAnchored            = "chain_anchored"
	TraceNoteKeyChainAnchorFull          = "chain_anchor_full"
	TraceNoteKeyChainAnchorRemainderSeat = "chain_anchor_remainder_seat"
	// TraceNoteKeyChainAnchorOwnershipDivergent + TraceNoteKeyChainAnchor-
	// ChainLane / TraceNoteKeyChainAnchorCensus (RNB-1, §29.88 R2, 2026-07-14):
	// the case-A' ownership-divergence disclosure. ownership_divergent="true"
	// ONLY on a migrated ◇ remainder seat whose pid's chain seat is present
	// but does not provably hold the census-anchored account (µs identity or
	// published-value check failed) — the 行2 relation sentence downgrades
	// from the additive 同源二分 form to the 账目关系(锚定权属失合) double-
	// account form, reading the two Σs from chain_anchor_chain_lane (the
	// chain seats' published per-state Σ) and chain_anchor_census (the
	// pid-level census-anchored Σ). Wording/relation input only; never a
	// rank/score input. Absent on case-A/case-B rows.
	TraceNoteKeyChainAnchorOwnershipDivergent = "chain_anchor_ownership_divergent"
	TraceNoteKeyChainAnchorChainLane          = "chain_anchor_chain_lane"
	TraceNoteKeyChainAnchorCensus             = "chain_anchor_census"
	// TraceNoteKeyChainCredentialLaneDemoted (RNB-1 R4 排他通则, §29.88.2,
	// 2026-07-14): "true" on a row whose WHOLE account rides the ◇ adjacent
	// channel because it cannot show a typed causal-edge anchored share —
	// the cpu_affinity/cpuset satellite (no per-row interval inventory), the
	// priority-inversion-retyped window seat (displacement-measured gated
	// eff, indivisible along the anchor boundary) and the interval-less
	// chain-lane D/IO VIEW rows of a zero-credential pid (customer E9/E10).
	// Every published value channel is untouched (值零动,通道位归位); the
	// display adds the 「无链上凭证(整席降道)」 disclosure line. Wording/
	// channel input only.
	TraceNoteKeyChainCredentialLaneDemoted = "chain_credential_lane_demoted"
	// TraceNoteKeyChainCredentialSegments /
	// TraceNoteKeyChainCredentialSegmentDisjoint /
	// TraceNoteKeyChainCredentialEnvelopeLevel (HULL-CRED, §29.104 终判③,
	// 2026-07-17): the keep-⛓ per-segment credential family of the chain-lane
	// D/IO VIEW verdict.
	//
	//   - chain_credential_segments carries the row's typed evidence segment
	//     inventory ("start..end" seconds joined with "|" — the
	//     member_line_ranges carriage pattern), published only on the
	//     segment-adjudicated verdicts (the ≥1-true-intersection keep and the
	//     all-disjoint demotion): the claim and its proof travel on one row.
	//     Strict decode; capped by the engine-mirrored
	//     TraceCausalProjectionChainCredentialSegmentCap.
	//   - chain_credential_segment_disjoint = "true" on the NEW demote form:
	//     the row's hull intersected the anchor windows but EVERY real
	//     segment lies in the hull's occurrence gaps — rides beside
	//     chain_credential_lane_demoted and the published inventory; the
	//     display speaks the 逐段核验 fork of the 无链上凭证 word ONLY when
	//     the decoded inventory is present (claim gated on proof). Minted
	//     from COMPLETE inventories only — a truncated prefix never proves
	//     absence (缺证≠证无, ONCHAIN-FIX-2 件3).
	//   - chain_credential_envelope_level = "true" on a keep-⛓ row whose
	//     credential was verified only at the conservative envelope/census
	//     tier (segment inventory absent, or an ONCHAIN-FIX-2 件3 truncated
	//     prefix that proved no intersection): the display adds the
	//     「交集证明(包络级)」 honest word. Never set on a demoted row.
	//   - chain_credential_segments_truncated (ONCHAIN-FIX-2 件3, Q6 已追认,
	//     2026-07-18) = "true" when the published inventory is the ledger's
	//     immutable checked PREFIX of a beyond-cap D/IO group — a proven
	//     LOWER BOUND, not the complete account. Rides ONLY beside a
	//     non-empty chain_credential_segments on the ≥1-true-intersection
	//     keep; the display adds the 「凭证清单不完整,实际锚定不小于所证」
	//     wording. Wording/channel input only; every published value channel
	//     is untouched on all four keys (值零动).
	TraceNoteKeyChainCredentialSegments          = "chain_credential_segments"
	TraceNoteKeyChainCredentialSegmentDisjoint   = "chain_credential_segment_disjoint"
	TraceNoteKeyChainCredentialEnvelopeLevel     = "chain_credential_envelope_level"
	TraceNoteKeyChainCredentialSegmentsTruncated = "chain_credential_segments_truncated"
	// TraceNoteKeyChainCredentialCensus (CHAINGUARD-1 件2, §29.204/§29.204.1,
	// 2026-07-22): the engine chain-credential census verdict of a chained
	// board's chain-channel ranked seat — closed enum wakeup_anchored /
	// target_self / interval_proven / member_inherited / none, minted at
	// exactly ONE engine point (censusChainSeatCredential at the ordinal
	// publication tail) and emitted through exactly ONE tool helper
	// (traceQueryChainCredentialCensusNote — CHAINGUARD-F5 anti-divergence).
	// "none" is the typed VIOLATION record: the seat carried zero credential
	// stamps and the engine demoted it to the ▒ background lane (values
	// untouched, result caveat raised). Consumers: the projection strict
	// parse (Node.ChainCredentialCensus), the ◎ credential chip word mapping
	// (件3 同源 — chip and engine can no longer drift), and the board/crown
	// second seat gate (census=none never elects/crowns/badges even on a
	// stale or cross-query merged artifact). Absent = pre-census artifact or
	// chainless board: every consumer keeps the legacy behavior
	// byte-identically (渐进兼容). Wording/channel input only.
	TraceNoteKeyChainCredentialCensus = "chain_credential_census"
	// TraceNoteKeyChainIdentityInheritance (ONCHAIN-FIX-1 件1, mint audit
	// 命题2 不一致①, 2026-07-18): "true" on an on-chain row that published NO
	// typed interval and inherited the chain lane from bare thread identity
	// (its pid is a chain member — the documented fail-open conservative
	// keep). The pre-fix shape fabricated overlap_ms from the whole
	// node-window wall clock on exactly these rows; post-fix the overlap note
	// is honestly absent and this marker drives the
	// 「成员继承(链窗级,无区间凭证)」 disclosure word instead. Emitted only
	// while the row still rides the on-chain lane (链上面与降道面不同行共存);
	// the HULL-CRED adjudicated rows and the analysis target's own rows never
	// carry it. Wording/channel input only; every published value channel is
	// untouched (值零动 — the retired fabricated value was never a seat value).
	TraceNoteKeyChainIdentityInheritance = "chain_identity_inheritance"
	// TraceNoteKeyChainAnchorRepresentedByChainSeat (XLANE-1 件1, §29.104.1/
	// §29.104.2, 2026-07-15): "true" on a fully-anchored runnable-family
	// SATELLITE (scheduler_latency / low_frequency) whose whole interval
	// inventory lies inside the thread's typed wakeup-dependency windows AND
	// whose same-pid chain-lane runnable seat physically intersects those
	// segments — the anchored share is already represented on the chain tier,
	// so the satellite rides the ◇ adjacent channel whole with every published
	// value untouched. Deliberately DISTINCT from chain_credential_lane_
	// demoted: this account HAS chain credential; the display speaks the
	// 「锚定份由链席代表(整席降道)」 word family, never 无链上凭证. Wording/
	// channel input only.
	TraceNoteKeyChainAnchorRepresentedByChainSeat = "chain_anchor_represented_by_chain_seat"
	// TraceNoteKeyGatedShareClaimed / TraceNoteKeyGatedShareFull /
	// TraceNoteKeyGatedShareConstituentSeat / TraceNoteKeyGatedShareClaimSeats
	// / TraceNoteKeyGatedShareOverlap (LEVELMERGE-1 件2 方案 P 区间分账, user
	// ruling 2026-07-18): the (pid,runnable) chain aggregate seat's
	// interval-accounting split against the same thread's priority-inversion
	// seat(s) whose gated composite already counts the overlapping runnable
	// share at full value (the runnable2 E26+E28 Σ>物理 mechanism).
	//   - gated_share_claimed = the A share (|∪claim windows ∩ ∪occurrence
	//     windows|, clamped to the account) — rides BOTH halves; the
	//     surviving seat's published value channels carry the residual B and
	//     claimed + residual == gated_share_full (the pinned identity);
	//   - gated_share_constituent_seat="true" marks the demoted A constituent
	//     row (adjacent lane, never competes, value = claimed share);
	//   - gated_share_claim_seats = the claiming inversion seat(s)' own line
	//     intervals "start..end" (comma-joined) — the display resolves each
	//     to its [E#] all-or-nothing (宁漏勿假指);
	//   - gated_share_overlap = the fail-open disclosure arm (裁定④ form
	//     「其中 X ms 与[E#](反转席)重叠」): a partial typed inventory
	//     witnesses the overlap (lower bound over available real segments)
	//     with every published value untouched; absent whenever the split ran.
	// Consumed by the projection compile into TraceCausalProjectionNode
	// GatedShare*; wording/relation inputs only — never a rank/score input.
	TraceNoteKeyGatedShareClaimed         = "gated_share_claimed"
	TraceNoteKeyGatedShareFull            = "gated_share_full"
	TraceNoteKeyGatedShareConstituentSeat = "gated_share_constituent_seat"
	TraceNoteKeyGatedShareClaimSeats      = "gated_share_claim_seats"
	TraceNoteKeyGatedShareOverlap         = "gated_share_overlap"
	// TraceNoteKeyGatedCompositeEdge* (PARTSPLIT-1, §29.150④ user ruling
	// 2026-07-19): the R4-mirror refusal record — a gated composite seat
	// (priority_inversion_runnable_wait family) whose runnable census account
	// bisected cleanly at the host's own credential-edge boundary but whose
	// lane conversion was REFUSED because a post-edge share exists (the gated
	// eff is an indivisible composite; RSPA R4/§29.83 既裁). Two carriers:
	//   - the refused SEAT's own root_cause_* record rides pre/post/anchor_ts/
	//     anchor_via (all four together or none — the atomic engine stamp),
	//     compiled into TraceCausalProjectionNode.GatedCompositeEdge* for the
	//     行2 分账披露 sub-line (identity re-validated against the row's own
	//     runnable note before any wording renders);
	//   - the NON-SEAT gated_composite_edge_share side-channel record
	//     additionally rides account (the runnable census account — the X+Y
	//     identity base, NOT the gated eff) and seat_published (the refused
	//     seat survived the publication cap), compiled into the projection's
	//     GatedCompositeEdgeShareDisclosures side list for the ◎ non-seat
	//     mention row (no ordinal, never in a section maximum).
	// Disclosure/wording inputs only — never a rank/score/value input; the
	// seat's every published channel stays untouched (R4 整席不拆 floor).
	TraceNoteKeyGatedCompositeEdgePreShare      = "gated_composite_edge_pre_share"
	TraceNoteKeyGatedCompositeEdgePostShare     = "gated_composite_edge_post_share"
	TraceNoteKeyGatedCompositeEdgeAccount       = "gated_composite_edge_account"
	TraceNoteKeyGatedCompositeEdgeAnchorTs      = "gated_composite_edge_anchor_ts"
	TraceNoteKeyGatedCompositeEdgeAnchorVia     = "gated_composite_edge_anchor_via"
	TraceNoteKeyGatedCompositeEdgeSeatPublished = "gated_composite_edge_seat_published"
	// TraceNoteKeySelfTwoRuler* (RULER2-1, §29.150② user ruling / R-19-b,
	// 2026-07-19): the self runnable two-ruler accounting record — ONE
	// NON-SEAT self_runnable_two_ruler side-channel record per rank result
	// whose published board carries the analysis target's own runnable seats
	// on BOTH closed rulers (self_wall_clock vs on_wakeup_chain). Per ruler:
	// the seat effective values (comma-joined "%.3f", board order), the seat
	// ordinals (comma-joined ints, same order) and the same-ruler subtotal
	// (Σ of that ruler's values — µs identity, re-validated by the strict
	// parser AND again at render). There is deliberately NO cross-ruler
	// total key (M3 禁混尺: a Σ across the two rulers is a mixed-ruler
	// number and never rides any face). Compiled into the projection's
	// SelfRunnableTwoRulerAccountings side list for the 行2 按两把尺记账
	// cross-row sentence under the lead seat row. Disclosure/wording inputs
	// only — never a rank/score/value input.
	TraceNoteKeySelfTwoRulerWallEffs     = "self_two_ruler_wall_effs"
	TraceNoteKeySelfTwoRulerWallRanks    = "self_two_ruler_wall_ranks"
	TraceNoteKeySelfTwoRulerWallSubtotal = "self_two_ruler_wall_subtotal"
	TraceNoteKeySelfTwoRulerEdgeEffs     = "self_two_ruler_edge_effs"
	TraceNoteKeySelfTwoRulerEdgeRanks    = "self_two_ruler_edge_ranks"
	TraceNoteKeySelfTwoRulerEdgeSubtotal = "self_two_ruler_edge_subtotal"
	// TraceNoteKeyRankBoardTarget / TraceNoteKeyRankBoardParams (XLANE-3 件1,
	// §29.104.2 定谳③ + §29.104.9 形③, 2026-07-16): the rank BOARD identity
	// triple's target and params halves, riding EVERY root_cause_* rank-lane
	// observation of one rank result (the window half already travels on
	// TraceNoteKeySelectedWindow). rank_board_target = the result-level typed
	// rank target's canonical thread label (comm-pid — the board the ordinal
	// domain belongs to, NOT the row's ranked subject);
	// rank_board_params_fingerprint = the engine's normalized rank-knob
	// fingerprint (tracequery rootCauseBoardParamsFingerprint, closed knob
	// set, 8 hex). Consumed by the projection compile into
	// TraceCausalProjectionNode.RankBoardTarget/-ParamsFingerprint so the
	// display multi-board detection can split same-window different-target
	// (or same-target different-params) boards instead of rendering colliding
	// bare #N ordinal domains (donghu 形③: 根因排序#1..#3 各×2). Board
	// identity/wording input only — never a rank/score/value input; absence
	// keeps the legacy window-only board identity byte-identical.
	TraceNoteKeyRankBoardTarget = "rank_board_target"
	TraceNoteKeyRankBoardParams = "rank_board_params_fingerprint"
	// TraceNoteKeyHostWakeupEdgeAnchorTs / TraceNoteKeyHostWakeupEdgeAnchorVia
	// (R3-IMPL, §29.88.1 user ruling 2026-07-14): the host-edge-anchored
	// semantic seat's typed credential disclosure pair — ts = the LATEST
	// in-window credential edge timestamp (the bisection boundary; µs-
	// verifiable against the raw sched_wakeup line), via = the typed edge
	// inventory word (closed set: direct / chain_hop / direct+chain_hop).
	// Ride ONLY rows whose on_chain_basis is "host_wakeup_edge_pre_span" or
	// "host_wakeup_edge_pre_state" (ONCHAIN-3c, 2026-07-19) and the ◇
	// remainder clones of their bisected accounts. Consumed by the projection
	// compile into TraceCausalProjectionNode.HostWakeupEdgeAnchorTS/-Via for
	// the 行2 唤醒锚定(宿主→目标) disclosure sentence. Wording/description
	// input only; never a rank/score input.
	TraceNoteKeyHostWakeupEdgeAnchorTs  = "host_wakeup_edge_anchor_ts"
	TraceNoteKeyHostWakeupEdgeAnchorVia = "host_wakeup_edge_anchor_via"
	// TraceNoteKeyCPUConstraint* (RNB-2 件5 AFF-EVID, §29.88.6, 2026-07-15):
	// the affinity/cpuset seat's typed judgment payload — kind = the
	// judgment-basis event kind (sched_switch_next_info / raw constraint
	// event name); cpuset = the group name; policy = the verbatim policy
	// string (ices_boost=true rides here); allowed_cpus / excluded_cpus =
	// comma-joined sorted CPU lists (allowed union vs the in-window observed
	// CPUs absent from it — the restriction gate's own comparison, and the
	// §29.88.4 R5a 「限制上更大核可能性」 comparison input). Consumed by the
	// projection compile into TraceCausalProjectionNode.CPUConstraint* for
	// the 行3/明细 constraint-description line. Wording/description input
	// only; never a rank/score input.
	TraceNoteKeyCPUConstraintKind   = "cpu_constraint_kind"
	TraceNoteKeyCPUConstraintCPUSet = "cpu_constraint_cpuset"
	// V1 dual-review P2 (2026-07-26): binding-provenance bit — "true" only
	// when the cpuset name came from a real binding EVENT (the gate input);
	// absent for the sched_switch cg= proxy fill (display context only).
	TraceNoteKeyCPUConstraintCPUSetIsBinding = "cpu_constraint_cpuset_is_binding"
	TraceNoteKeyCPUConstraintPolicy          = "cpu_constraint_policy"
	TraceNoteKeyCPUConstraintAllowedCPUs     = "cpu_constraint_allowed_cpus"
	TraceNoteKeyCPUConstraintExcludedCPUs    = "cpu_constraint_excluded_cpus"
	// R5a (§29.88.4 场景② 按核档, 2026-07-15): the tier-exclusion proof pair
	// — minted together exactly when the binding provably excludes a bigger
	// core tier; drives the obligatory 「绑核排除更大核档」 mention line.
	TraceNoteKeyCPUConstraintAllowedMaxTierKHz = "cpu_constraint_allowed_max_tier_khz"
	TraceNoteKeyCPUConstraintGlobalMaxTierKHz  = "cpu_constraint_global_max_tier_khz"
	// TraceNoteKeyResourceCompletionClosure (RSPA M-IO, §29.61.10c): "true"
	// on an io_latency rank row whose completion thread performed the wakeup
	// that ended an ANCHORED D/IO wait of a chain thread inside the IO's
	// lifetime — the typed per-IO completion-closure credential that keeps
	// the row on the chain lane (pure overlap demotes to ◇). Wording/context
	// input only.
	TraceNoteKeyResourceCompletionClosure = "resource_completion_closure"
	// TraceNoteKeyTGID / TraceNoteKeyProcessComm (CR-3 件③ P11, 2026-07-12;
	// 冷读案8 关键角色裸线程名无 tgid): the rank row's process attribution —
	// the TGID the trace's second column published for the thread, plus the
	// owning process comm resolved from the window thread catalog (tgid==tid
	// main-thread entry; comm absent when unresolvable — the thread's own
	// comm never substitutes). Consumed by the projection compile into
	// TraceCausalProjectionNode.ProcessTGID/ProcessComm (detail identity
	// 「进程 tgid=G comm=P」 line) and read by the board-summary feed (LLM
	// seat rows gain tgid=G).
	TraceNoteKeyTGID        = "tgid"
	TraceNoteKeyProcessComm = "process_comm"
	// TraceNoteKeyThermalCapWitnessed (CR-3 件⑥ F-10, 2026-07-12; CR-2 冷读
	// D5): whether the fold's thermal/policy cap has an IN-WINDOW
	// cpu_frequency_limits / thermal-rail event witness ("true"/"false",
	// emitted only beside thermal_cap_khz). Consumed by the projection
	// compile into TraceCausalProjectionNode.ThermalCapWitnessed — the
	// display words an unwitnessed press 运行于 X(限压原因未见证) instead of
	// 受热限压至 X (词面仅当窗内存在 typed 见证).
	TraceNoteKeyThermalCapWitnessed = "thermal_cap_witnessed"
	// TraceNoteKeyDeterministicRunning (§29.27② COV-4, 2026-07-11): the
	// target_window_states record's 确定性工作 lane — the wall-clock union of
	// the focused thread's own semantic-span intervals ∩ its running
	// intervals inside the analysis window (engine
	// TargetWindowStateAccount.DeterministicRunningMs; never a converted
	// value). Consumed by the projection compile into the typed
	// TargetStateAccount for the four-state coverage account.
	TraceNoteKeyDeterministicRunning = "deterministic_running"
	TraceNoteKeyFragments            = "fragments"
	TraceNoteKeySwitches             = "switches"
	TraceNoteKeyMaxSegment           = "max_segment"
	TraceNoteKeyP95Segment           = "p95_segment"
	TraceNoteKeyActualRunning        = "actual_running"
	TraceNoteKeyActualRunnable       = "actual_runnable"
	TraceNoteKeyActualSleep          = "actual_sleep"
	TraceNoteKeyActualDState         = "actual_d_state"
	TraceNoteKeyActualIOWait         = "actual_io_wait"
	// TraceNoteKeyRunnableBelowRTPreempted (SYM-2 §24.17 R2, 2026-07-08): the
	// typed 「优先级低于RT」 disclosure on a SELF runnable-family rank row —
	// the engine minted it only when the target's own priority class is below
	// RT (Harmony ohos_cfs) and an RT-class competitor's running overlapped
	// the wait on the same CPU (R5g displacement evidence). Value is the
	// literal "true"; absent otherwise (absence never guesses). The projection
	// compile promotes it onto the node for the 行2 tail wording.
	TraceNoteKeyRunnableBelowRTPreempted = "runnable_below_rt_preempted"
)

// 周期族 (periodic-source family, VS-1 §7.8).
const (
	TraceNoteKeyPeriodicSource   = "periodic_source"
	TraceNoteKeyDetectedPeriodMS = "detected_period_ms"
	TraceNoteKeyLatenessMS       = "lateness_ms"
	// GAP-B2 复核修 (2026-07-25): the D∧timer credential — non-empty (bare
	// caller symbol, e.g. "timerfd_read") exactly when the periodic stamp
	// came via the D∧timer arm, so display faces fork 期内定时等待 wording
	// instead of asserting a sleep the row never held.
	TraceNoteKeyTimerWaitCaller = "timer_wait_caller"
)

// 折算族 (supply-fold family, VS-2 §7.10).
const (
	TraceNoteKeyFoldBasis             = "fold_basis"
	TraceNoteKeySupplyFoldDeficitMS   = "supply_fold_deficit_ms"
	TraceNoteKeySupplyFoldIdealMS     = "supply_fold_ideal_ms"
	TraceNoteKeyFoldFmax              = "fold_fmax"
	TraceNoteKeyFoldFmaxFinding       = "fold_fmax_finding"
	TraceNoteKeyFoldClusterLaneCaveat = "fold_cluster_lane_caveat"
	// TraceNoteKeyFoldClusterFreqReuse (CFR #75 簇共频): discloses fold slices
	// whose frequency was reused from a same-cluster sampled core under
	// explicit topology (SupplyFoldBasis.ClusterFreqReuse roster).
	TraceNoteKeyFoldClusterFreqReuse = "fold_cluster_freq_reuse"
	// TraceNoteKeyFoldCapability (CAP §26 C3): typed three-state capability
	// caliber of the fold (default_table / evidence_table / freq_only) — the
	// display keys the "按默认算力比粗算" / "簇结构不可判,按纯频率比折算"
	// disclosures on this token, never on re-derived heuristics.
	TraceNoteKeyFoldCapability = "fold_capability"
	// TraceNoteKeyFoldCapabilityFreqOnlyReason (CLUSTER-FIX-2 件1, S1
	// 2026-07-20): the typed freq_only CAUSE token (closed set: no_domains /
	// no_sampled_cluster / single_cluster / cluster_overflow / fmax_tie /
	// comove_floor / comove_floor_single_burst) — emitted beside
	// fold_capability iff it is freq_only. The display forks the
	// single-cluster wording (仅单簇有频点采样…) on it; every other reason
	// keeps the legacy 簇结构不可判 wording byte-identically. Disclosure
	// input only, no gate reads it.
	TraceNoteKeyFoldCapabilityFreqOnlyReason = "fold_capability_freq_only_reason"
	// TraceNoteKeyFoldReferenceClass (CAP 复核 F1, 2026-07-08): the capability
	// class of the fold's SAME-CLUSTER (fmax, cap) reference. Emitted ONLY
	// when the reference demoted away from the §26-nominated big class
	// (small/middle/prime) — absence means the big-class basis, so the legacy
	// R5 (§29.88.12) retired the demotion word fork — the field stays a
	// wire/audit record of the basis cluster's class.
	TraceNoteKeyFoldReferenceClass = "fold_reference_class"
	// TraceNoteKeySelfRunningFoldUnmeasured* (SELFRUN-DISC, §29.192① (b)
	// user ruling / A2 件11(b) handoff §29.194, 2026-07-21): the self
	// supply-fold 「量不了」 absence disclosure — ONE NON-SEAT
	// self_running_fold_unmeasured side-channel record per rank result whose
	// analysis target RAN inside the window while the fold basis was
	// ENTIRELY unknown (KnownMs==0 ∧ UnknownMs>0: every slice folded at
	// ratio 1 因数据缺席), so the zero deficit means "unmeasurable", never
	// the affirmative "no loss". running_ms = the window-projected running
	// wall clock; unknown_ms = the unknown-basis wall (equal by the fold
	// identity KnownMs+UnknownMs==RunningMs with KnownMs==0 — the strict
	// parser re-validates the equality, all-or-nothing). Compiled into the
	// projection's SelfRunningFoldUnmeasured side list for the ◎ auxiliary
	// 另账 row 「运行频点未采集,自身降频折算不可量」. Disclosure/wording
	// inputs only — never a rank/score/value input; a minted deficit seat
	// (缺口>0) and the truly-full-frequency zero (KnownMs>0) both emit
	// NOTHING (negative arms pinned).
	TraceNoteKeySelfRunningFoldUnmeasuredRunningMS = "self_running_fold_unmeasured_running_ms"
	TraceNoteKeySelfRunningFoldUnmeasuredUnknownMS = "self_running_fold_unmeasured_unknown_ms"
	// TraceNoteKeyFoldClusterTopology (CAP-2 §28.4/§28.5, 2026-07-09): the
	// typed cluster-STRUCTURE source of the fold's capability map —
	// freq_comovement (Tier-1 实测频点共动) / keyed_rail (Tier-2 键控簇轨,
	// 成员按锚点连续推定). Emitted ONLY on the two evidence forms; absence =
	// explicit topology / legacy, keeping every pre-CAP-2 note stream
	// byte-identical. Hard consumer: the projection compile promotes it and
	// the display upgrades the capability caliber wording on it.
	TraceNoteKeyFoldClusterTopology = "fold_cluster_topology"
	// TraceNoteKeyFoldRailBasis (CAP-2 §28.5-T6 审计注): the adopted keyed-
	// rail family mask + the rail-governed slice-CPU roster — the traceback
	// that keeps the anchor-presumption fold auditable. Display tier.
	//
	// G10-EN 族内豁免 (QH2-A adjudication, 2026-07-14; §28.8 记债): the
	// zh-only value ("族=…;cpuN 频点=簇轨 …") is EXEMPT from the G10-EN
	// per-lane wording fix — display_only with ZERO display/compile
	// consumers (holder_ns_unification precedent, §29.40 OM-10 class), so no
	// zh/EN report lane exists to fork; its only surface is the
	// evidence-index audit lane, which is ruled 原文保留 (§22.2.1 审计车道
	// verbatim traceback tokens). If a display consumer is ever wired, walk
	// the G10-EN component pattern (typed fields + per-lane wording) first.
	TraceNoteKeyFoldRailBasis = "fold_rail_basis"
	// TraceNoteKeyThermalCapKHz (THERM §28.5-T7, 2026-07-09): the fold's
	// dominant running cluster was pressed below its fmax inside the
	// governance window — value = the pressed-to ceiling in kHz (governing
	// limits Max and/or thermal-named cluster rail, window minimum).
	// Disclosure-only zero-weight edit; hard consumer: the projection
	// promotes it into the 窗内该簇受热限压至 X sentence. Absent when no
	// press or when cluster attribution is unavailable (absence never
	// guesses).
	TraceNoteKeyThermalCapKHz = "thermal_cap_khz"
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
	// TraceNoteKeyBlockingFromSite (BLOCKFROM, §27.4 G13 配套 / §28.1 收口批准
	// 2026-07-09, real_trace_campaign_20260705.md): the blocked WAITER's own
	// call site — the payload's "blocking from <sig>(<file:line>)" tail —
	// verbatim, the 等待点 counterpart of holder_site (持有点). Hard consumer:
	// the DISP-2 display half parses it in the projection compile
	// (TraceCausalProjectionNode.BlockingFromSite → the "等待点: …" detail
	// line; landed the same wave, holder_source promotion precedent). The
	// opendir G13 witness: prose invented an "enqueueMessage 消息队列锁" wait
	// point while the span payload named
	// AssetManager.getResourceValue(AssetManager.java:761).
	TraceNoteKeyBlockingFromSite = "blocking_from_site"
	TraceNoteKeyWaiters          = "waiters"
	// P0-E2a counterpart-resolution family (§10 A2 / §11 N8 / §12 Q4-C): the
	// typed origin of a resolved blocking counterpart, the raw payload owner tid
	// preserved when a cross-namespace phantom was replaced by a wakeup-edge
	// fallback, and the wait object of a payload-less blocking span. Actual
	// consumer state (UXG-1 假注释勘正, §29.40, 2026-07-12): holder_source and
	// owner_tid_raw were promoted to hard_consumer node-field read-ins (§24.9-C
	// F5); peer_source and wait_object remain display_only with NO
	// deterministic display consumer — wait_object is known_gap OM-11 (明细锁块
	// 「等待对象」行, host batch IC-L).
	TraceNoteKeyHolderSource = "holder_source"
	TraceNoteKeyPeerSource   = "peer_source"
	TraceNoteKeyOwnerTidRaw  = "owner_tid_raw"
	// TraceNoteKeyOwnerTidPresence (LOCKNS-FIX 修补 件A, 冷读 P2-F1+P3-F7
	// 同族, 2026-07-16): the typed presence verdict of the payload owner tid
	// on a rung-①-diverged row — closed set "absent" / "present_collision"
	// (G1 ns-divergent numeric collision) / "present_comm_mismatch" (tid
	// present, payload owner comm never observed on it); engine constants
	// tracequery.OwnerTidPresence*. Hard consumer: the projection compile
	// reads it into TraceCausalProjectionNode.BlockingOwnerTidPresence and
	// the detail 持有者来历 presence clause forks on it (the legacy
	// 「不在本 trace」/"absent from this trace" claim was FALSE on the two
	// present shapes — same fact the engine collision Summary already words).
	// Absent note (legacy wire) fail-opens to the legacy sentence
	// byte-identically.
	TraceNoteKeyOwnerTidPresence = "owner_tid_presence"
	TraceNoteKeyWaitObject       = "wait_object"
	// LCK-2 ns-span derivation family (§18.E / §18.E.1, 2026-07-07).
	// TraceNoteKeyHolderNsUnification is the typed ②×③ identity-unification
	// declaration ("owner_ns_tid=<N> host=<thread> lanes=ns_span_derivation+
	// wakeup_edge"): the rung-② ns-span mapping and the rung-③ closing wakeup
	// independently named the SAME host thread, so "payload owner and
	// releasing holder are one physical thread" is a system fact, not model
	// prose. TraceNoteKeyHolderHostProcess is the PROCESS-LEVEL rung-②
	// identity ("tgid=<G> ns_pid=<P> level=process[ comm=<name>]") published
	// when the container tid could not be mapped to a host thread — the host
	// tgid is NEVER stuffed into a peer PID (§19 typed-pair pin), it rides
	// this display note.
	// EVOLUTION RECORD (LOCKNS-FIX 件6 / OM-10 关账, §29.104.12, 2026-07-16):
	// holder_ns_unification is now a HARD consumer — the projection compile
	// reads it into TraceCausalProjectionNode.BlockingHolderNsUnification and
	// the detail 持有者来历 line appends the ②×③ cross-corroboration
	// disclosure (「发射对×收尾唤醒两道互证」). holder_host_process remains
	// display_only with no deterministic consumer (known_gap OM-10 进程级半场;
	// unification 半场已关账 — host batch IC-L).
	TraceNoteKeyHolderNsUnification = "holder_ns_unification"
	TraceNoteKeyHolderHostProcess   = "holder_host_process"
	// TraceNoteKeyBlockingOwnerKeyUnregistered (LOCKNS-FIX 件3, §29.104.12,
	// 2026-07-16): "true" on a PAYLOAD-LESS blocking_span row whose span name
	// speaks lock-owner vocabulary (word-boundary `owner`) yet matched no
	// registered lock-contention morphology — the row fail-opened to the
	// payload-less lane (no holder minted from an unregistered shape; value
	// rides the XERR1-FIX basis discipline untouched). Hard consumer: the
	// projection compile reads it into
	// TraceCausalProjectionNode.BlockingOwnerKeyUnregistered and the detail
	// face words the 「owner 未解析(形态未注册)」 disclosure. The detection
	// signal is NOISY, so it drives disclosure only — never a gate.
	TraceNoteKeyBlockingOwnerKeyUnregistered = "blocking_owner_key_unregistered"
	// P0-E 锁车道修2 (ledger §24.9-C F2, 2026-07-09): the payload hand-off
	// chain witness (the lock changed hands during the wait — the resolved
	// holder is the FINAL holder, never the whole-span holder) and the
	// same-lock self-contradiction demotion witness (an inferred holder that
	// was itself queued on the same lock for most of the span had its
	// attribution withdrawn). Node-field read-ins for the three disclosure
	// faces (tree row / detail stanza / lead qualifier).
	TraceNoteKeyHolderHandoff           = "holder_handoff"
	TraceNoteKeyHolderSelfContradiction = "holder_self_contradiction"
	// G10-EN 根修 (QH2-A, 2026-07-14; §28.7 留账): the typed COMPONENTS of the
	// self-contradiction witness above — inferred-holder label / payload
	// owner tid / the holder's own queued overlap (ms) / the attributed span
	// (ms) / the contradicting span's line range ("start-end"). The
	// projection compile assembles them into
	// TraceCausalProjectionNode.BlockingHolderContradictionParts so the zh/EN
	// detail lanes each word their own sentence
	// (TraceHolderSelfContradictionWitness.WitnessText); the legacy zh string
	// key above keeps the byte-frozen audit-verbatim value. All five ride or
	// drop together (partial sets parse to nil — absence never guesses).
	TraceNoteKeyHolderSelfContradictionHolder   = "holder_self_contradiction_holder"
	TraceNoteKeyHolderSelfContradictionOwnerTid = "holder_self_contradiction_owner_tid"
	TraceNoteKeyHolderSelfContradictionQueuedMs = "holder_self_contradiction_queued_ms"
	TraceNoteKeyHolderSelfContradictionSpanMs   = "holder_self_contradiction_span_ms"
	TraceNoteKeyHolderSelfContradictionLines    = "holder_self_contradiction_lines"
	// XERR1-FIX 件1/件3 family (§29.104.3/.4, 2026-07-15) — the payload-less
	// blocking_span value-convergence and budget-sanity carriage. The customer
	// E1 lesion: a span's window-envelope projection (199.992ms = 100% of the
	// window) was published as 「阻塞等待」 while the same thread's four-state
	// account said running 54%.
	//
	// TraceNoteKeyBlockingValueBasis — the typed value basis of a
	// blocking_span row, BOTH payload lanes since XERR1-EXT (§29.104.17
	// 裁定⑤, 2026-07-16): "wait_segments" (the published value is the
	// waiter's Σ(sleep+D+iowait) inside span∩window — payload-typed rows
	// window it on the fold value-winner interval; the envelope moved to the
	// blocking_span_envelope_ms disclosure) or "span_envelope" (convergence
	// impossible — no waiter timeline in the value interval, or an
	// empty/unavailable value interval; the payload-less display word family
	// must say 「span 包络(含运行)」, never 「阻塞等待」, while payload-typed
	// rows keep the lock family words and disclose on the 值口径 line).
	// Absent only on legacy artifacts (legacy wording fail-open). Hard
	// consumer: projection compile → Node.BlockingValueBasis → the 件2
	// word-face fork (peer-relation arms + §24.3 form family, payload-less
	// gated) + the 值口径 detail line (both lanes).
	//
	// TraceNoteKeyBlockingWaitSegmentMS / TraceNoteKeyBlockingWaitSleepMS —
	// the converged Σ value and its sleep component (the sleep share > 0
	// additionally gates the 件1 互指 disclosure against the thread's own
	// sleep seat — same physical sleep time in two accounts, cross-referenced
	// instead of added).
	//
	// TraceNoteKeyBlockingSpanEnvelopeMS — the span's in-window envelope
	// projection (pre-convergence value), disclosure only.
	//
	// TraceNoteKeyBlockingWaitBudgetExceeded (+NonRunning/Running budgets) —
	// the 件3 sanity marker: the row's envelope claim exceeds the waiter's
	// own non-running total over the same interval (F-2 同基 gate at the
	// mint; 值+预算随行, 禁 clamp 禁硬拒). Display adds the ⚠ line
	// 「span 包络 X > 窗内非 running Y:含 running Z,非阻塞等待段」.
	//
	// TraceNoteKeyBlockingWaitCoveragePartial (+AccountCoveredMS) — XERR1-FIX
	// 修补 件F (冷读 P3-3, 2026-07-16): the waiter's state account did not
	// tile the whole span∩window interval (the same coverage gap that 禁判s
	// the 件3 budget), so the converged wait_segments value is a PROVEN LOWER
	// BOUND. Display adds the detail 覆盖核查 line 「等待段账目未满覆盖
	// span 窗(账目 X ms/span 窗 Y ms):收敛值为已证下界」. wait_segments
	// basis only; absence renders nothing.
	TraceNoteKeyBlockingValueBasis             = "blocking_value_basis"
	TraceNoteKeyBlockingWaitSegmentMS          = "blocking_wait_segment_ms"
	TraceNoteKeyBlockingWaitSleepMS            = "blocking_wait_sleep_ms"
	TraceNoteKeyBlockingSpanEnvelopeMS         = "blocking_span_envelope_ms"
	TraceNoteKeyBlockingWaitBudgetExceeded     = "blocking_wait_budget_exceeded"
	TraceNoteKeyBlockingWaitBudgetNonRunningMS = "blocking_wait_budget_non_running_ms"
	TraceNoteKeyBlockingWaitBudgetRunningMS    = "blocking_wait_budget_running_ms"
	TraceNoteKeyBlockingWaitCoveragePartial    = "blocking_wait_coverage_partial"
	TraceNoteKeyBlockingWaitAccountCoveredMS   = "blocking_wait_account_covered_ms"
	// TraceNoteKeySubjectIsLockHolder (BLK §15.C, 2026-07-06): "true" on a
	// resolved blocking_span rank row whose SUBJECT is the lock HOLDER (and
	// whose peer= is the blocked WAITER). The projection compile reads it into
	// TraceCausalProjectionNode.BlockingSubjectIsHolder so the renderer shows a
	// HOLD ("持锁 X ms 阻塞了 <waiter>") instead of the reversed lock-WAIT the
	// waiter-subject critical_blocking row already carries for the SAME physical
	// lock, and steers the next-step drilldown to the holder (the subject), not
	// the waiter. Display tier (renderer wording + next-step identity only).
	TraceNoteKeySubjectIsLockHolder = "subject_is_lock_holder"
	// TraceNoteKeyLockTwinFolded (BLK-2 P2, 2026-07-06): "true" on a
	// holder-subject rank record that is the SINGLE publication of a physical
	// lock-contention span whose waiter-subject critical_blocking twin row was
	// folded into it (BLK §15.C ① single-publication fold). Precise fold
	// witness, emitted ONLY by the twin-port lane: the coverage view parses it
	// back (traceObservationSoftMissingDimensions) and counts marked rank
	// records as critical_blocking coverage, so a window whose only blocking
	// row was the folded twin can never fake a "critical_blocking_calls
	// missing" soft gap that pushes the LLM to re-run a query which cannot add
	// rows.
	TraceNoteKeyLockTwinFolded = "lock_twin_folded"
	// G1/B4 跨车道对账族 (§27.2-G1 and 2026-07-10 external-audit B4): the
	// typed absorption markers the tracequery engine stamps when two lanes
	// deterministically prove they published the same physical source events.
	// G1 covers critical_blocking io_latency ↔ same-thread rank family;
	// B4 covers exact d_state_or_io_wait ↔ io_burst_episode rank seats under
	// numeric TID + adjudicated producer pair + query endpoints + exact
	// interval/line-span equality.
	//
	// TraceNoteKeyAbsorbedByRankFamily — "true" on the absorbed
	// absorbed observation. TraceNoteKeyAbsorbedInto — the
	// engine-rendered canonical family identity on that same observation.
	// TraceNoteKeyRankFamilyKey — the SAME identity string on the absorbing
	// absorbing rank observation (single engine-rendered key; the
	// projection compile joins the two sides by verbatim string equality,
	// never a label re-derivation). All three hard consumers: the compile
	// relocates absorbed nodes out of the render buckets into
	// AbsorbedChainRows ONLY when both sides are present (负向保护: family
	// absent → the absorbed row keeps its seat), and the family detail stanza
	// prints the 链上并入 disclosure with the absorbed rows' E# list —
	// observations themselves keep publishing (观测照发不删, evidence/audit
	// lossless). A typo on either side silently kills the fold and the
	// duplicate rows come back — exactly the wire-silence failure class this
	// registry exists for.
	TraceNoteKeyAbsorbedByRankFamily = "absorbed_by_rank_family"
	TraceNoteKeyAbsorbedInto         = "absorbed_into"
	TraceNoteKeyRankFamilyKey        = "rank_family_key"
)

// 门控族 (gated-composition family, §7.30.3 D3).
const (
	TraceNoteKeyGatedRunnable       = "gated_runnable"
	TraceNoteKeyGatedRunningDeficit = "gated_running_deficit"
	// TraceNoteKeyGatedCapability (CAP §26 C3): typed capability caliber of
	// the discounted running component (same token set as fold_capability).
	TraceNoteKeyGatedCapability = "gated_capability"
	// TraceNoteKeyGatedClusterTopology (CAP-2 §28.4/§28.5): the gated twin of
	// fold_cluster_topology — same token set, same absence semantics.
	TraceNoteKeyGatedClusterTopology = "gated_cluster_topology"
	// TraceNoteKeyGatedFreqOnlyReason (DISPHYG-3 件7, 2026-07-20): the gated
	// twin of fold_capability_freq_only_reason — same token set
	// (CoreCapabilityFreqOnlyReason* closed enum), same absence semantics
	// (emitted iff the gated capability caliber is freq_only). Wording input
	// only: the gated caliber suffix forks its freq_only wording on it so the
	// gated face and the supply-fold face can never contradict on one page.
	TraceNoteKeyGatedFreqOnlyReason = "gated_capability_freq_only_reason"
	// TraceNoteKeyPriorityInversionCandidate (PTV5 Q4, #68 用户裁定 2026-07-05):
	// promoted from a display-only literal to a consumer-parsed key — the
	// projection compile reads it into the typed
	// TraceCausalProjectionNode.PriorityInversionCandidate field.
	TraceNoteKeyPriorityInversionCandidate = "priority_inversion_candidate"
	// TQ-PRIORITY-POINT-AUTHORITY: point/range proof provenance and the
	// relation-scoped coverage account.  The *_source values carry the closed
	// priority evidence caliber (exact_at_point / closed_range_stable /
	// advisory_nearest / unknown); the coverage pair partitions the eligible
	// scheduler-state wall account into proven-lower and everything else. The
	// additive *_artifact_source(s) keys carry physical artifact:N identities;
	// compatibility caliber fields are deliberately not renamed.
	TraceNoteKeyWakeupTs                            = "wakeup_ts"
	TraceNoteKeyWakerPrioritySource                 = "waker_priority_source"
	TraceNoteKeyWakerPriorityArtifactSource         = "waker_priority_artifact_source"
	TraceNoteKeyWakeePrioritySource                 = "wakee_priority_source"
	TraceNoteKeyWakeePriorityArtifactSource         = "wakee_priority_artifact_source"
	TraceNoteKeyWakeePriorityAuthority              = "wakee_priority_authority"
	TraceNoteKeyPrioritySource                      = "priority_source"
	TraceNoteKeyPriorityArtifactSource              = "priority_artifact_source"
	TraceNoteKeyTargetPrioritySource                = "target_priority_source"
	TraceNoteKeyTargetPriorityArtifactSource        = "target_priority_artifact_source"
	TraceNoteKeyPriorityRelationCaliber             = "priority_relation_caliber"
	TraceNoteKeyPriorityRelationProvenLowerMS       = "priority_relation_proven_lower_ms"
	TraceNoteKeyPriorityRelationUnknownOrNonLowerMS = "priority_relation_unknown_or_nonlower_ms"
	TraceNoteKeyPriorityRelationArtifactSources     = "priority_relation_artifact_sources"
)

// 语义跨度族 (semantic-span family).
const (
	TraceNoteKeySpanName        = "span_name"
	TraceNoteKeySpanKind        = "span_kind"
	TraceNoteKeySpanCategory    = "span_category"
	TraceNoteKeySpanSubcategory = "span_subcategory"
	TraceNoteKeySemanticClass   = "semantic_class"
)

// 引导族 (guidance family — wire-compat emitted; the display lane that
// parsed them retired with A2 件1, §29.174 UX-13).
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

// P0-E CHAIN-PATH (ledger §22.1, 2026-07-09): per-branch wakeup path records
// replace the retired cross-branch flattened walk. TraceNoteKeyChainPathBranch
// is the record's 1-based branch ordinal (the projection election keys its
// branch-form candidate-pool switch on its PRESENCE); Branches is the total
// expanded branch count (a published-record count below it discloses a wire
// cap). TraceNoteKeyChainBranch rides rank/impact/aggregate rows: the owning
// branch of the row's chain measurement, so the display tree keys its depth
// attach to (branch, depth) instead of a cross-branch flat position.
const (
	TraceNoteKeyChainPathBranch   = "branch"
	TraceNoteKeyChainPathBranches = "branches"
	TraceNoteKeyChainBranch       = "chain_branch"
)

// 线程选择诊断族。数值 PID 仍是 exact identity；这些 soft-consumer keys
// 只把名称不匹配和候选名册送到答案覆盖边界，不参与路由、角色判定或因果 gate。
const (
	TraceNoteKeyRequestedPID   = "requested_pid"
	TraceNoteKeyRequestedName  = "requested_name"
	TraceNoteKeySelectedThread = "selected_thread"
	TraceNoteKeyRouting        = "routing"
	TraceNoteKeyNameCandidates = "name_candidates"
)

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
	// EVAL-B1-R12/R14: exact bounded target-wait occurrence rosters are
	// prompt-consumed typed values. Keep their wire names centralized so
	// producers and the repair-handoff projection cannot drift.
	TraceNoteKeyTargetWaitOccurrence          = "target_wait_occurrence"
	TraceNoteKeyTargetWaitOccurrencePrompt    = "target_wait_occurrence_prompt"
	TraceNoteKeyTargetWaitOccurrencePromptSum = "target_wait_occurrence_prompt_sum_ms"
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
	// EVOLUTION RECORD (CR-2 组③ P7 + 修复轮 R-P2-2, 2026-07-12): soft→hard —
	// the projection compile parses the actual channel's physical interval
	// into node.ActualWindowStartTs/EndTs (the ⚠ 词面 containment judge).
	{TraceNoteKeyActualWindow, "anchor_window", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyNearestChainWindow, "anchor_window", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyOccurrenceWindows, "anchor_window", TraceNoteCarrierSoftConsumer},
	{"window_proportion", "anchor_window", TraceNoteCarrierDisplayOnly},
	// EVAL-B1-R12 (2026-07-30): bounded target-wait rosters cannot live in
	// one prompt summary because the shared text clamp would expose a
	// misleading prefix. These notes carry prompt-level completeness, its
	// emitted-prefix sum, and one exact engine-paired occurrence per note.
	{TraceNoteKeyTargetWaitOccurrence, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyTargetWaitOccurrencePrompt, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyTargetWaitOccurrencePromptSum, "state", TraceNoteCarrierSoftConsumer},

	// 因果排名族.
	{TraceNoteKeyRank, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyTier, "causal_rank", TraceNoteCarrierHardConsumer},
	// CR-3 件③ P11 (2026-07-12): rank-row process attribution pair.
	{TraceNoteKeyTGID, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyProcessComm, "causal_rank", TraceNoteCarrierHardConsumer},
	// EVOLUTION RECORD (修复轮 R-P2-2 census 反向臂首跑, 2026-07-12): soft→hard
	// — the compile has parsed it into node.BackgroundRank since DCS §23.1;
	// the carrier column under-reported.
	{TraceNoteKeyBackgroundRank, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyType, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySource, "causal_rank", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyCausality, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainRelevance, "causal_rank", TraceNoteCarrierHardConsumer},
	// SELF-SEM (§29.61.1, 2026-07-13): the on-chain proof-basis marker — the
	// projection compile parses it (node.OnChainBasis) and the self qualifier
	// wording forks on it.
	{TraceNoteKeyOnChainBasis, "causal_rank", TraceNoteCarrierHardConsumer},
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
	// EVOLUTION RECORD (审计 #5/#62, 2026-07-10): overlap promoted
	// display_only → hard_consumer — the projection compile consumes it on
	// on-chain trace_semantic_span records only (SemanticChainProjectedMS,
	// the single-span intersection carrier; see TraceNoteKeyOverlap).
	{TraceNoteKeyOverlap, "causal_rank", TraceNoteCarrierHardConsumer},
	{"candidate_count", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_role", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_role_kind", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_role_source", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_phase", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_frame_id", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"selected_name", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"target_role_kind", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"target_role_source", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"occurrences", "causal_rank", TraceNoteCarrierDisplayOnly},
	{"coverage_mode", "causal_rank", TraceNoteCarrierDisplayOnly},
	// trace_gap_kind (G2 判据 typed 化, §27.2/§28.1 user ruling 2026-07-09,
	// real_trace_campaign_20260705.md): the precise blind-spot criterion on a
	// trace_gap rank observation — closed enum no_sched_data (the thread
	// timeline holds no interval at all in the aligned window) /
	// no_eligible_wait (intervals exist but ALL sit below the MinDurationMs
	// floor — 复核 P3-5 precise fact; the legacy "窗内无调度数据" wording
	// over-claimed on this form). EVOLUTION RECORD (Wave-3.2 收尾,
	// 2026-07-09): display→hard_consumer — the DISP-2 display half parses it
	// in the projection compile (TraceCausalProjectionNode.TraceGapKind, the
	// ◇ row wording fork), exactly the promotion this row's original comment
	// promised; constant exported alongside.
	{TraceNoteKeyTraceGapKind, "causal_rank", TraceNoteCarrierHardConsumer},
	// PTS 折叠族 (#68 用户裁定 2026-07-05): wire-cap overflow fold accounting.
	{TraceNoteKeyFoldedRows, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyFoldedMinMS, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyFoldedMaxMS, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyFoldedSubjects, "causal_rank", TraceNoteCarrierHardConsumer},
	// A2 件5② (2026-07-21): wire-fold max-member disclosure carriers.
	{TraceNoteKeyFoldedMaxSubject, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyFoldedMaxStateKind, "causal_rank", TraceNoteCarrierHardConsumer},
	// same_value_members (DIAG A1, §28.11-3(a) G12, 2026-07-09): µs-tie member
	// roster beside the folded_* family — projection compile re-materializes
	// it into node.SameValueMembers (audit-token disclosure face).
	{TraceNoteKeySameValueMembers, "causal_rank", TraceNoteCarrierHardConsumer},
	// RCM 家族合并族 (§24.7.1/§24.10, 2026-07-08): engine same-thread family
	// merge accounting — parsed by the projection compile into the isolated
	// FamilyMember* node lane (never MergedCount/MergedMaxMS).
	{TraceNoteKeyMemberCount, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyMemberMaxMS, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyMemberMinMS, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyMemberSumMS, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyMemberFoldCaliber, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyMemberRoster, "causal_rank", TraceNoteCarrierHardConsumer},
	// XLANE-2 件1 (2026-07-17): complete per-member line ranges of a semantic
	// family seat — projection compile parses it (strict count match) into
	// FamilyMemberLineRanges; the display subset-judgment lane consumes it.
	{TraceNoteKeyMemberLineRanges, "causal_rank", TraceNoteCarrierHardConsumer},
	// SPANTOP-1 件1 (§29.131, 2026-07-18): complete per-member wall-clock list
	// of a semantic family seat — projection compile parses it (strict count
	// match, positive floats) into FamilyMemberWallMS; the display constituent
	// top-3 sub-row lane consumes it under its µs identity gate.
	{TraceNoteKeyMemberWallMS, "causal_rank", TraceNoteCarrierHardConsumer},
	// SPANVIS-1 (2026-07-19): the pure-advisory business-span mention face —
	// one business_span_mention record per admitted on-chain family; the
	// projection compile parses each record ALL-OR-NOTHING into
	// BusinessSpanMentions (a projection-level side channel: no node, no
	// seat, no ordinal/conservation/census membership). Display consumers:
	// tree-fence 「◈ 业务span提示」 advisory block + ◎ overview 旁栏 footnote.
	{TraceNoteKeyBusinessSpanName, "business_span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBusinessSpanCount, "business_span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBusinessSpanTotalMS, "business_span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBusinessSpanMaxMS, "business_span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBusinessSpanLines, "business_span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBusinessSpanBasis, "business_span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBusinessSpanHidden, "business_span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBusinessSpanOmitted, "business_span", TraceNoteCarrierHardConsumer},
	// XLANE-2 件2 (2026-07-17): the self-gap seat's semantic-overlap
	// disclosure roster — projection compile parses it into
	// SelfGapSemanticOverlaps; the display renders the 行内 overlap clause.
	{TraceNoteKeySelfGapSemanticOverlaps, "causal_rank", TraceNoteCarrierHardConsumer},
	// AXIOM-V2 (2026-07-18): the fix-direction attribute (display 行2 word +
	// legend) and the cross-direction overlap pair roster (display 互指句) —
	// projection compile parses both; the undisclosed lane stays an audit-face
	// disclosure (generic detail rendering, 立案素材). ELIM-V2 (2026-07-18):
	// the conservation finding is now compile-parsed too (Node.
	// DirectionConservationExcess → the ◎ 守恒尾行 violation transcription).
	{TraceNoteKeyFixDirection, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCrossDirectionOverlaps, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCrossDirectionOverlapUndisclosed, "causal_rank", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyDirectionConservationExcess, "causal_rank", TraceNoteCarrierHardConsumer},
	// P3MEASURE-1 (§29.169, 2026-07-20): the silent on-chain measurement
	// family — display_only IS the red line (advisory-only, no parser, no
	// rendered face, never a future hard gate without a new user ruling);
	// see the constants block for the full caliber doc.
	{TraceNoteKeyP3MCounterfactualValidMS, "causal_rank", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyP3MCounterfactualInvalidMS, "causal_rank", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyP3MEdgeWitnessedMS, "causal_rank", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyP3MDisposition, "causal_rank", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyP3MCoverage, "causal_rank", TraceNoteCarrierDisplayOnly},
	// G1 跨车道对账 (§27.2-G1, 2026-07-09): family-side canonical identity on
	// the absorbing rank observation (absorbed-side markers ride the blocking
	// family below) — projection compile joins the two sides on it.
	{TraceNoteKeyRankFamilyKey, "causal_rank", TraceNoteCarrierHardConsumer},

	// 冲击度量族.
	{TraceNoteKeyImpact, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyImpactMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCumulativeImpactMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyEffectiveImpactMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyEffectiveImpact, "impact", TraceNoteCarrierHardConsumer},
	// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16): composite-score twins — a
	// composite rank row (io_pressure / block_io_by_inode, registry wire arm)
	// publishes its value slots under these instead of the ms-semantic keys.
	// impact/cumulative/effective are hard consumers exactly like their ms
	// twins (projection compile float read-ins + board summary); the projected
	// pair mirrors the display-only ms echoes below.
	{TraceNoteKeyImpactScore, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCumulativeImpactScore, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyEffectiveImpactScore, "impact", TraceNoteCarrierHardConsumer},
	{"projected_impact_score", "impact", TraceNoteCarrierDisplayOnly},
	{"projected_total_score", "impact", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyActualImpactMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyActualImpact, "impact", TraceNoteCarrierHardConsumer},
	// EVOLUTION RECORD (修复轮 R-P2-2 census 反向臂首跑, 2026-07-12): soft→hard
	// — the compile parses it into node.TotalMS; the carrier column
	// under-reported.
	{TraceNoteKeyTotal, "impact", TraceNoteCarrierHardConsumer},
	// EVOLUTION RECORD (DIAG A2, §28.11-3(b) D-10, 2026-07-09): actual_total /
	// actual_total_ms promoted soft_consumer → hard_consumer — the projection
	// compile now parses them into node.ActualTotalMS (the 实际口径 stanza
	// line's thread-total half); the coverage-view soft parse is unchanged.
	{TraceNoteKeyActualTotalMS, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyActualTotal, "impact", TraceNoteCarrierHardConsumer},
	// actual_caliber_note (DIAG A2): typed two-caliber divergence disclosure —
	// node-field read-in; the detail stanza's 实际口径 line keys on it.
	{TraceNoteKeyActualCaliberNote, "impact", TraceNoteCarrierHardConsumer},
	// EVOLUTION RECORD (审计 #5/#62, 2026-07-10): projected_impact promoted
	// display_only → hard_consumer — the on-chain semantic family record's
	// exact intersection participation (SemanticChainProjectedMS; see
	// TraceNoteKeyProjectedImpact). projected_impact_ms (rank-lane display
	// echo) stays display-only.
	{TraceNoteKeyProjectedImpact, "impact", TraceNoteCarrierHardConsumer},
	{"projected_impact_ms", "impact", TraceNoteCarrierDisplayOnly},
	{"projected_total", "impact", TraceNoteCarrierDisplayOnly},
	{"projected_total_ms", "impact", TraceNoteCarrierDisplayOnly},
	// EVOLUTION RECORD (COV 批, §24.9 D-1, 2026-07-08): target_impact family
	// display_only → hard_consumer — the coverage-sentence numerator now
	// consumes the typed TargetImpactMS projection field sourced from these
	// notes (traceCausalProjectionNodeFromRecord), because the cumulative
	// channel is display-overwritten by §20.1 on inversion∧running rank rows
	// (opendir_78: 58.919 raw vs target_impact 112.175 → "已归因45%/未归因55%"
	// fabricated against a ~97% explained wait).
	{TraceNoteKeyTargetImpact, "impact", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyTargetImpactMS, "impact", TraceNoteCarrierHardConsumer},
	// inherited_target_blocked_ms (Q4-B, §12.3 ruling 2, P0-E1): the
	// wakeup-dependency window value an on-chain resource row INHERITS as
	// annotation-only context — 承自只作注记,永不作硬排序键. Display tier
	// with NO consumer today (UXG-1 假注释勘正, §29.40, 2026-07-12: the former
	// "P0-A display batch consumes it" claim never landed) — the 承接目标阻塞
	// annotation line is known_gap OM-13, host batch IC-A.
	{"inherited_target_blocked_ms", "impact", TraceNoteCarrierDisplayOnly},
	// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16): renamed from `rank_impact` —
	// the state-drilldown ranking-only composite weight (StateDrilldownStep.
	// RankImpactMs, §7.30 S1 witness) leaves the bare-impact vocabulary; the
	// JSON tag renamed rank_impact_ms → rank_impact_score in the same batch.
	// Census: zero parsers (display-only carrier), zero-compat rename.
	{"rank_impact_score", "impact", TraceNoteCarrierDisplayOnly},
	{"duration", "impact", TraceNoteCarrierDisplayOnly},

	// 状态族.
	{TraceNoteKeyDominantState, "state", TraceNoteCarrierHardConsumer},
	// RANKDIS-EXT A3 (§29.104.16.1 M15, 2026-07-16): the state_drilldown
	// Top-N ordinal's DEDICATED lane — the drilldown observation previously
	// borrowed the causal-rank `rank` key, so the projection compile parsed
	// a state-board ordinal into Node.Rank exactly like a root-cause board
	// seat (empty chain_relevance = chain channel); the only防线 was
	// prompt-side dedup coincidence. Both drilldown producers (typed lane
	// internal/tool/trace_query.go and the legacy text re-parse lane
	// observation_ledger.go) mint THIS key as a display-tier literal;
	// `rank` stays exclusively causal-board and the projection's Rank parse
	// is additionally predicate-gated to root_cause_* records.
	{"state_rank", "state", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyRunnableBelowRTPreempted, "state", TraceNoteCarrierHardConsumer},
	// §29.27② (COV-4, 2026-07-11) EVOLUTION: the five per-state keys are now
	// ALSO hard-consumed by the projection compile on target_window_states
	// records (typed TargetStateAccount for the four-state coverage account);
	// their state_churn / wakeup-aggregate carriers stay display/soft.
	{TraceNoteKeyRunning, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyRunnable, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySleep, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyDState, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyDStateRefinedNonIO, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockedReasonCaller, "state", TraceNoteCarrierHardConsumer},
	// CR-3 件② P10 (2026-07-12): unconsumed blocked_reason residual pair.
	{TraceNoteKeyBlockedReasonWindowCount, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockedReasonWindowCaller, "state", TraceNoteCarrierHardConsumer},
	// 件1 census 根修 (2026-07-13): pid-keyed per-caller census pair — the
	// model evidence feed (internal/context wait-object summary) is the
	// consumer; a miss is a feed omission, never a gate (soft lane).
	{TraceNoteKeyBlockedReasonCensus, "state", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyBlockedReasonCensusOverflow, "state", TraceNoteCarrierSoftConsumer},
	// §29.50.5 (v5 P1 批 件②, 2026-07-13): proof-partition honest remainder.
	{TraceNoteKeyDStateCauseUnprovenRemainder, "state", TraceNoteCarrierHardConsumer},
	// RSPA (§29.61.10, 2026-07-14): the re-anchoring bipartition trio + the
	// M-IO completion-closure credential.
	{TraceNoteKeyChainAnchored, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainAnchorFull, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainAnchorRemainderSeat, "state", TraceNoteCarrierHardConsumer},
	// RNB-1 (§29.88 R2/R4, 2026-07-14): case-A' double-account disclosure
	// trio + the R4 whole-seat lane-demotion marker.
	{TraceNoteKeyChainAnchorOwnershipDivergent, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainAnchorChainLane, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainAnchorCensus, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainCredentialLaneDemoted, "state", TraceNoteCarrierHardConsumer},
	// HULL-CRED (§29.104 终判③, 2026-07-17): the keep-⛓ per-segment
	// credential family — the segment inventory (proof carriage), the
	// all-disjoint demote marker, the envelope-tier honest-word marker and
	// the ONCHAIN-FIX-2 件3 truncated lower-bound prefix marker.
	{TraceNoteKeyChainCredentialSegments, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainCredentialSegmentDisjoint, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainCredentialEnvelopeLevel, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainCredentialSegmentsTruncated, "state", TraceNoteCarrierHardConsumer},
	// ONCHAIN-FIX-1 件1 (2026-07-18): the interval-less identity-inheritance
	// admission marker (fail-open keep disclosure; fabricated overlap retired).
	{TraceNoteKeyChainIdentityInheritance, "state", TraceNoteCarrierHardConsumer},
	// CHAINGUARD-1 件2 (2026-07-22): the engine chain-credential census
	// verdict — projection strict parse + ◎ chip mapping + board second gate.
	{TraceNoteKeyChainCredentialCensus, "causal_rank", TraceNoteCarrierHardConsumer},
	// XLANE-1 件1 (§29.104.2, 2026-07-15): the fully-anchored satellite
	// represented-by-chain-seat whole-seat ◇ demotion marker.
	{TraceNoteKeyChainAnchorRepresentedByChainSeat, "state", TraceNoteCarrierHardConsumer},
	// LEVELMERGE-1 件2 (方案 P 区间分账, 2026-07-18): the gated-share split
	// family — A/B decomposition floats, the constituent-row marker, the
	// claim-seat [E#] pointer roster and the fail-open overlap disclosure.
	{TraceNoteKeyGatedShareClaimed, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedShareFull, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedShareConstituentSeat, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedShareClaimSeats, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedShareOverlap, "state", TraceNoteCarrierHardConsumer},
	// PARTSPLIT-1 (§29.150④, 2026-07-19): the R4-mirror refusal record —
	// seat-face pre/post/anchor pair + side-channel account/seat_published
	// (行2 分账披露 sub-line and ◎ non-seat mention inputs).
	{TraceNoteKeyGatedCompositeEdgePreShare, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedCompositeEdgePostShare, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedCompositeEdgeAccount, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedCompositeEdgeAnchorTs, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedCompositeEdgeAnchorVia, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedCompositeEdgeSeatPublished, "state", TraceNoteCarrierHardConsumer},
	// RULER2-1 (§29.150② / R-19-b, 2026-07-19): the self runnable two-ruler
	// accounting record — per-ruler seat values/ordinals + same-ruler
	// subtotals (行2 按两把尺记账 cross-row sentence inputs; NO cross-ruler
	// total key exists — M3 禁混尺).
	{TraceNoteKeySelfTwoRulerWallEffs, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySelfTwoRulerWallRanks, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySelfTwoRulerWallSubtotal, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySelfTwoRulerEdgeEffs, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySelfTwoRulerEdgeRanks, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySelfTwoRulerEdgeSubtotal, "causal_rank", TraceNoteCarrierHardConsumer},
	// XLANE-3 件1 (§29.104.2 定谳③, 2026-07-16): the rank board identity
	// triple's target/params halves (multi-board split + chip anchor inputs).
	{TraceNoteKeyRankBoardTarget, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyRankBoardParams, "causal_rank", TraceNoteCarrierHardConsumer},
	// R3-IMPL (§29.88.1, 2026-07-15): the host-edge-anchored semantic seat's
	// credential disclosure pair (行2 唤醒锚定(宿主→目标) sentence inputs).
	{TraceNoteKeyHostWakeupEdgeAnchorTs, "causal_rank", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHostWakeupEdgeAnchorVia, "causal_rank", TraceNoteCarrierHardConsumer},
	// RNB-2 件5 AFF-EVID (§29.88.6, 2026-07-15): affinity/cpuset judgment
	// payload — the 行3/明细 constraint-description inputs.
	{TraceNoteKeyCPUConstraintKind, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCPUConstraintCPUSet, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCPUConstraintCPUSetIsBinding, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCPUConstraintPolicy, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCPUConstraintAllowedCPUs, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCPUConstraintExcludedCPUs, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCPUConstraintAllowedMaxTierKHz, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyCPUConstraintGlobalMaxTierKHz, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyResourceCompletionClosure, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOWait, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySleepIOWait, "state", TraceNoteCarrierHardConsumer},
	// ANSWERFACE-1 件2 (§29.140 G6, 2026-07-19): the target_window_states
	// boundary-fold disclosure quartet — hard-consumed by the projection
	// compile into the four-state row's term annotation.
	{TraceNoteKeyHeadCarryMS, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHeadCarryState, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyTailOpenMS, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyTailOpenState, "state", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyDeterministicRunning, "state", TraceNoteCarrierHardConsumer},
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
	// GAP-B2 复核修 (2026-07-25): D∧timer 词面分叉凭证 — decoded onto
	// Node.PeriodicTimerCaller inside the PeriodicSource arm.
	{TraceNoteKeyTimerWaitCaller, "periodic", TraceNoteCarrierHardConsumer},

	// 折算族 (VS-2).
	{TraceNoteKeyFoldBasis, "supply_fold", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySupplyFoldDeficitMS, "supply_fold", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySupplyFoldIdealMS, "supply_fold", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyFoldFmax, "supply_fold", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyFoldFmaxFinding, "supply_fold", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyFoldClusterLaneCaveat, "supply_fold", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyFoldClusterFreqReuse, "supply_fold", TraceNoteCarrierDisplayOnly},
	// fold_capability (CAP §26 C3): typed node-field read-in — the projection
	// wording forks (按默认算力比粗算 / 簇结构不可判) key on it.
	{TraceNoteKeyFoldCapability, "supply_fold", TraceNoteCarrierHardConsumer},
	// fold_capability_freq_only_reason (CLUSTER-FIX-2 件1, S1): typed
	// node-field read-in — the single-cluster wording fork
	// (仅单簇有频点采样…) keys on it; absence keeps every legacy freq_only
	// wording byte-identical.
	{TraceNoteKeyFoldCapabilityFreqOnlyReason, "supply_fold", TraceNoteCarrierHardConsumer},
	// self_running_fold_unmeasured_* (SELFRUN-DISC §29.192① (b)): the
	// NON-SEAT 「量不了」 absence disclosure pair — projection side-channel
	// read-in for the ◎ auxiliary 另账 row (strict all-or-nothing parse with
	// the running==unknown identity re-validated).
	{TraceNoteKeySelfRunningFoldUnmeasuredRunningMS, "supply_fold", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySelfRunningFoldUnmeasuredUnknownMS, "supply_fold", TraceNoteCarrierHardConsumer},
	// fold_reference_class (CAP 复核 F1): typed node-field read-in — the
	// 按X核满频 basis wording keys on it (absence = big).
	{TraceNoteKeyFoldReferenceClass, "supply_fold", TraceNoteCarrierHardConsumer},
	// fold_cluster_topology (CAP-2 §28.4/§28.5): typed node-field read-in —
	// the capability caliber wording upgrades on it (按实测频点共动分簇折算 /
	// 按簇轨实测折算(成员按锚点连续推定); absence = explicit/legacy).
	{TraceNoteKeyFoldClusterTopology, "supply_fold", TraceNoteCarrierHardConsumer},
	// fold_rail_basis (CAP-2 §28.5-T6 审计注): rail family + rail-governed
	// slice roster traceback — display tier.
	{TraceNoteKeyFoldRailBasis, "supply_fold", TraceNoteCarrierDisplayOnly},
	// thermal_cap_khz (THERM §28.5-T7): typed node-field read-in — the
	// 窗内该簇受热限压至 X disclosure sentence keys on it.
	{TraceNoteKeyThermalCapKHz, "supply_fold", TraceNoteCarrierHardConsumer},
	// CR-3 件⑥ F-10 (2026-07-12): the cap's in-window witness bit — the
	// 受热限压 vs 运行于(限压原因未见证) wording gate.
	{TraceNoteKeyThermalCapWitnessed, "supply_fold", TraceNoteCarrierHardConsumer},

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
	// BLOCKFROM (§27.4 G13, 2026-07-09): waiter-side blocking call site.
	// EVOLUTION RECORD (Wave-3.2 收尾, 2026-07-09): display→hard_consumer in
	// the SAME wave — the DISP-2 display half landed its projection read-in
	// (TraceCausalProjectionNode.BlockingFromSite, the 等待点 detail line)
	// immediately, the holder_source promotion precedent compressed to one
	// wave. A typo now silently kills the 等待点 line.
	{TraceNoteKeyBlockingFromSite, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyWaiters, "blocking", TraceNoteCarrierHardConsumer},
	// P0-E2a counterpart-resolution keys. P0-E 锁车道修3 (§24.9-C F5,
	// 2026-07-09): holder_source / owner_tid_raw are now typed node-field
	// read-ins — the projection's 持有者来历 detail line, the tree-row and
	// lead 推断 qualifiers key on them (display decisions, hard read-in tier;
	// same promotion precedent as subject_is_lock_holder).
	{TraceNoteKeyHolderSource, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyPeerSource, "blocking", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyOwnerTidRaw, "blocking", TraceNoteCarrierHardConsumer},
	// LOCKNS-FIX 修补 件A (2026-07-16): typed payload-owner-tid presence
	// verdict — hard node-field read-in driving the detail 持有者来历
	// presence-clause fork (absent keeps the legacy sentence; the two
	// present shapes stop claiming "not present in this trace").
	{TraceNoteKeyOwnerTidPresence, "blocking", TraceNoteCarrierHardConsumer},
	// P0-E 锁车道修2 witnesses — node-field read-ins (disclosure faces).
	{TraceNoteKeyHolderHandoff, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHolderSelfContradiction, "blocking", TraceNoteCarrierHardConsumer},
	// G10-EN 根修 (QH2-A, 2026-07-14): the witness component quintet — the
	// compile assembles BlockingHolderContradictionParts from them (the zh/EN
	// detail lanes' per-lane wording source), hard read-ins.
	{TraceNoteKeyHolderSelfContradictionHolder, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHolderSelfContradictionOwnerTid, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHolderSelfContradictionQueuedMs, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHolderSelfContradictionSpanMs, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHolderSelfContradictionLines, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyWaitObject, "blocking", TraceNoteCarrierDisplayOnly},
	// XERR1-FIX 件1/件3 (§29.104.3/.4, 2026-07-15): payload-less blocking_span
	// value-basis + budget-sanity carriage. Hard consumers: projection compile
	// node-field read-ins driving the 件2 word-face fork and the 件3 ⚠
	// disclosure line.
	{TraceNoteKeyBlockingValueBasis, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockingWaitSegmentMS, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockingWaitSleepMS, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockingSpanEnvelopeMS, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockingWaitBudgetExceeded, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockingWaitBudgetNonRunningMS, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockingWaitBudgetRunningMS, "blocking", TraceNoteCarrierHardConsumer},
	// 件F (2026-07-16): partial-coverage lower-bound disclosure pair — hard
	// node-field read-ins driving the detail 覆盖核查 line.
	{TraceNoteKeyBlockingWaitCoveragePartial, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyBlockingWaitAccountCoveredMS, "blocking", TraceNoteCarrierHardConsumer},
	// LOCKNS-FIX 件3 (§29.104.12, 2026-07-16): unknown-morphology fail-open
	// marker — hard node-field read-in driving the detail 持有者核查
	// 「owner 未解析(形态未注册)」 line on payload-less rows.
	{TraceNoteKeyBlockingOwnerKeyUnregistered, "blocking", TraceNoteCarrierHardConsumer},
	// LCK-2 ns-span derivation keys (§18.E/§18.E.1).
	// EVOLUTION RECORD (LOCKNS-FIX 件6 / OM-10 关账, §29.104.12, 2026-07-16):
	// holder_ns_unification display→hard_consumer — the projection compile
	// now reads it into TraceCausalProjectionNode.BlockingHolderNsUnification
	// and the detail 持有者来历 line appends the ②×③ cross-corroboration
	// disclosure. holder_host_process stays display_only (known_gap OM-10
	// 进程级半场; unification 半场已关账 — host batch IC-L).
	{TraceNoteKeyHolderNsUnification, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyHolderHostProcess, "blocking", TraceNoteCarrierDisplayOnly},
	// BLK §15.C: subject-is-holder display flag (renderer HOLD wording +
	// next-step holder identity). Display tier, hard node-field read-in.
	{TraceNoteKeySubjectIsLockHolder, "blocking", TraceNoteCarrierHardConsumer},
	// BLK-2 P2: precise twin-fold witness on the surviving rank record; the
	// coverage soft-missing scan parses it back (critical_blocking coverage).
	{TraceNoteKeyLockTwinFolded, "blocking", TraceNoteCarrierSoftConsumer},
	// G1 跨车道对账族 (§27.2-G1, 2026-07-09): absorbed-side markers on the
	// critical_blocking observation, family-side identity on the rank
	// observation — the projection compile parses all three (bucket
	// relocation + 链上并入 stanza join), hard consumers.
	{TraceNoteKeyAbsorbedByRankFamily, "blocking", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyAbsorbedInto, "blocking", TraceNoteCarrierHardConsumer},
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
	// subject_state_* / subject_chain_* (BLK-2 P1 指代翻转修复, 2026-07-06):
	// the twin-port lane RE-KEYS the folded twin's peer_state_* / peer_chain_*
	// families when porting them onto the holder-subject rank record — there
	// the measured thread (the twin's peer, i.e. the lock HOLDER) is the rank
	// record's OWN SUBJECT, while the rank record's peer= names the blocked
	// WAITER. Porting under the original peer_* keys paired peer=<waiter> with
	// peer_state_dominant=<holder state> on one record — the "等待方 running
	// 主导" false fact. Display tier, emitted only by the twin-port lane
	// (traceQueryTypedLockTwinSubjectStateNotes / ...SubjectChainNotes);
	// critical_blocking rows keep the peer_* spellings (their peer IS the
	// described thread).
	{"subject_state_dominant", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_state_total", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_state_running", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_state_runnable", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_state_sleep", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_state_d_state", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_state_io_wait", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_state_fragments", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_chain_state", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_chain_blocker", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_chain_blocker_state", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_chain_blocker_source", "blocking", TraceNoteCarrierDisplayOnly},
	{"subject_chain_presumptive", "blocking", TraceNoteCarrierDisplayOnly},
	// peer_chain_* (A1 bounded continuation, §12.3-5 ruling 5): ONE sub-goal hop
	// off the resolved counterpart — the peer's OWN dominant state + its single
	// direct 1-hop blocker (depth hard-capped at 1). peer_chain_blocker_source
	// is ALWAYS wakeup_edge when a blocker is named (F2: the hop-2 name is
	// structurally an inference, never payload-direct); peer_chain_presumptive
	// is true when the counterpart itself was only wakeup-edge-resolved
	// (inference on inference). Display tier with NO projection/answer-face
	// consumer today (UXG-1 假注释勘正, §29.40, 2026-07-12) — the continuation
	// direction word face is the OM-11 companion filing, host batch IC-L.
	{"peer_chain_state", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_chain_blocker", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_chain_blocker_state", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_chain_blocker_source", "blocking", TraceNoteCarrierDisplayOnly},
	{"peer_chain_presumptive", "blocking", TraceNoteCarrierDisplayOnly},
	// drill_status (RCX① engine side, §12.3 ruling 1, P0-E1): typed drill-debt
	// verdict for a row's blocking counterpart (drilled /
	// undrilled_peer_known / peer_unknown), emitted on critical_blocking and
	// lock-lane root_cause_rank observations. Display tier with NO
	// projection/answer-face consumer today (UXG-1 假注释勘正, §29.40,
	// 2026-07-12): only the bundle-head disclosure half landed
	// (internal/tool/trace_query.go); the 投影头部强制披露 half is known_gap
	// OM-7, host batch IC-A (promotion to a constant follows the
	// priority_inversion_candidate precedent when that batch lands).
	{"drill_status", "blocking", TraceNoteCarrierDisplayOnly},

	// 门控族 (D3).
	{TraceNoteKeyGatedRunnable, "gating", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyGatedRunningDeficit, "gating", TraceNoteCarrierHardConsumer},
	// gated_cluster_topology (CAP-2): the gated twin of fold_cluster_topology
	// — typed node-field read-in, same wording fork.
	{TraceNoteKeyGatedClusterTopology, "gating", TraceNoteCarrierHardConsumer},
	// gated_capability_freq_only_reason (DISPHYG-3 件7): the gated twin of
	// fold_capability_freq_only_reason — typed node-field read-in, same
	// wording fork as the supply-fold clause single point.
	{TraceNoteKeyGatedFreqOnlyReason, "gating", TraceNoteCarrierHardConsumer},
	// gated_capability (CAP §26 C3): typed node-field read-in — the R5d
	// 折算,按全域最大核最高频 caliber's capability disclosure keys on it.
	{TraceNoteKeyGatedCapability, "gating", TraceNoteCarrierHardConsumer},
	{"priority_inversion_gated", "gating", TraceNoteCarrierDisplayOnly},
	// gated_aggregation_caliber (P0-E §20 E-Gap② / F3 absorption, 2026-07-07):
	// WHICH ruler produced an inversion-typed aggregate's gated total —
	// sum_disjoint_occurrences (member windows pairwise disjoint, wall
	// additive) or max_overlap_fallback (honest lower bound). Emitted on
	// wakeup_causal_aggregate observations only when the row is
	// inversion-TYPED (F2 gate). Display tier with NO parser today (UXG-1
	// 期票勘正, 2026-07-12: P0-A shipped without the promotion); a future
	// promotion would follow the priority_inversion_candidate precedent
	// (exported constant + compile read-in) — no host batch is scheduled.
	{"gated_aggregation_caliber", "gating", TraceNoteCarrierDisplayOnly},
	// PTV5 Q4 (#68 用户裁定 2026-07-05): promoted display_only → hard_consumer
	// (typed node field read-in).
	{TraceNoteKeyPriorityInversionCandidate, "gating", TraceNoteCarrierHardConsumer},
	{"priority_inversion_edges", "gating", TraceNoteCarrierDisplayOnly},
	{"priority_relation", "gating", TraceNoteCarrierDisplayOnly},
	// TQ-PRIORITY-POINT-AUTHORITY EVOLUTION RECORD (2026-07-17): the generic
	// dependency/target source, relation caliber, and two-sided coverage keys
	// are parsed back through the legacy observation/projection compile lane;
	// they therefore graduate display_only -> hard_consumer under the NKR
	// protocol. The edge-local waker/wakee provenance keys remain honest
	// display-only audit context: no projection gate reads them back.
	{TraceNoteKeyWakerPrioritySource, "gating", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyWakerPriorityArtifactSource, "gating", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyWakeePrioritySource, "gating", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyWakeePriorityArtifactSource, "gating", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyWakeePriorityAuthority, "gating", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyPrioritySource, "gating", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyPriorityArtifactSource, "gating", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyTargetPrioritySource, "gating", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyTargetPriorityArtifactSource, "gating", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyPriorityRelationCaliber, "gating", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyPriorityRelationProvenLowerMS, "gating", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyPriorityRelationUnknownOrNonLowerMS, "gating", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyPriorityRelationArtifactSources, "gating", TraceNoteCarrierDisplayOnly},

	// 语义跨度族.
	{TraceNoteKeySpanName, "span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySpanKind, "span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySpanCategory, "span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySpanSubcategory, "span", TraceNoteCarrierHardConsumer},
	{TraceNoteKeySemanticClass, "span", TraceNoteCarrierHardConsumer},

	// 引导族.
	// A2 件1 (§29.174 UX-13, 2026-07-21) EVOLUTION: soft_consumer →
	// display_only — the per-record next-step display lane retired (the ◎
	// direction-action lane replaces it); producers keep emitting these four
	// keys for wire compatibility, parsed by nobody.
	{TraceNoteKeyNextStep, "guidance", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyNextStepKind, "guidance", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyRunnableCPU, "guidance", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyTopCompetitor, "guidance", TraceNoteCarrierDisplayOnly},
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
	// P0-E CHAIN-PATH (ledger §22.1): branch identity of a per-branch path
	// record (hard: the projection election pools branch-form candidates on
	// it) and the rank/impact rows' owning-branch attach domain (hard: the
	// display tree's depth attach keys on it).
	{TraceNoteKeyChainPathBranch, "chain_path", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyChainPathBranches, "chain_path", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyChainBranch, "causal_rank", TraceNoteCarrierHardConsumer},
	{"edges", "chain_path", TraceNoteCarrierDisplayOnly},
	{"nodes", "chain_path", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyWakeupTs, "chain_path", TraceNoteCarrierHardConsumer},
	{"latency", "chain_path", TraceNoteCarrierDisplayOnly},
	{"waker_priority", "chain_path", TraceNoteCarrierDisplayOnly},
	{"wakee_priority", "chain_path", TraceNoteCarrierDisplayOnly},
	// WAKE-CENSUS (§29.58, 2026-07-13): per-pair whole-inventory wakeup-edge
	// census notes — the model evidence feed (internal/context wait-object
	// summary) is the consumer; a miss is a feed omission, never a gate
	// (soft lane, blocked_reason_census 同构).
	{TraceNoteKeyWakeupEdgeCensusFirstTs, "chain_path", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyWakeupEdgeCensusLastTs, "chain_path", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyWakeupEdgeCensusOverflowPairs, "chain_path", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyWakeupEdgeCensusOverflowEdges, "chain_path", TraceNoteCarrierSoftConsumer},
	// WAKE-CENSUS-D 2A (§29.58.4, 2026-07-13): the typed exit-state split
	// (context evidence feed consumer, same lane as first/last ts).
	{TraceNoteKeyWakeupEdgeCensusSleepExit, "chain_path", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyWakeupEdgeCensusDExit, "chain_path", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyWakeupEdgeCensusOtherExit, "chain_path", TraceNoteCarrierSoftConsumer},
	// 修复轮 件2 (2026-07-13): the per-result target-wakee completeness marker.
	{TraceNoteKeyWakeupEdgeCensusTargetWakee, "chain_path", TraceNoteCarrierSoftConsumer},

	// 线程选择诊断族（exact PID 路由不变；答案侧只做 soft disclosure）。
	{"selector_status", "thread_selection", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyRequestedPID, "thread_selection", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyRequestedName, "thread_selection", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeySelectedThread, "thread_selection", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyRouting, "thread_selection", TraceNoteCarrierSoftConsumer},
	{TraceNoteKeyNameCandidates, "thread_selection", TraceNoteCarrierSoftConsumer},
	{"name_candidate_role_authority", "thread_selection", TraceNoteCarrierDisplayOnly},

	// VSync/帧节拍发生器普查族 (SA-F2, DISPATCH-IND 批4, 2026-07-14): the
	// per-generator census notes (event/wakeup counts, the authoritative
	// period parsed from the generator's own period print, caliber marker).
	// Display tier today — no consumer parses them yet; promote to
	// contract-tier constants the moment a feed/projection consumer lands.
	{"vsync_generator_census_caliber", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_events", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_trace_marks", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_woken", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_period_prints", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_period_ns", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_refresh_rate", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_identified_by", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_first_ts", "vsync_census", TraceNoteCarrierDisplayOnly},
	{"vsync_generator_census_last_ts", "vsync_census", TraceNoteCarrierDisplayOnly},

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
	{"system_or_kernel_running", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"system_or_kernel_overlap", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"system_or_kernel_competitors", "cpu_load", TraceNoteCarrierDisplayOnly},
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
	// 批 B/E2 authority+epoch faces (§15.12 批丁 NOTES-R2', 2026-07-26):
	// display-tier disclosures on the cpu_constraint typed observation —
	// mask authority, restriction proof, trace-universe exclusion set with
	// its measured idle, epoch-scoped restricted runnable, and the epoch
	// roster/census counters.
	{"allowed_cpus_authority", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"restriction_proof", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"excluded_trace_cpus", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"excluded_cpu_idle", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"restricted_runnable", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"constraint_epoch_total", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"constraint_epoch_emitted", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"constraint_epoch_status", "cpu_load", TraceNoteCarrierDisplayOnly},
	{"constraint_epoch_roster", "cpu_load", TraceNoteCarrierDisplayOnly},

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
	// EVOLUTION RECORD (RCM §24.7.1 ①/§24.9-B F3, 2026-07-08): inode/dev
	// promoted display_only → hard_consumer — the rank lane now emits them
	// from typed fields and the projection compile parses them into
	// node.Inode/node.Dev (the keys previously lived only in free-text
	// Summary prose and every display face dropped them).
	{TraceNoteKeyInode, "io", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyDev, "io", TraceNoteCarrierHardConsumer},
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
	// INODE (§28.6, 2026-07-09): whole-window (dev,inode) fold observations
	// (claimKey prefix top_io_inode:). reads/writes are the closed-set op
	// decomposition; top_threads carries per-thread WITHIN-thread latency
	// sums (never a cross-thread latency total); groups_total is the
	// truncation-honesty disclosure riding every row.
	{"reads", "io", TraceNoteCarrierDisplayOnly},
	{"writes", "io", TraceNoteCarrierDisplayOnly},
	{"top_threads", "io", TraceNoteCarrierDisplayOnly},
	{"groups_total", "io", TraceNoteCarrierDisplayOnly},
	{"layer", "io", TraceNoteCarrierDisplayOnly},
	{"event", "io", TraceNoteCarrierDisplayOnly},
	{"paired", "io", TraceNoteCarrierDisplayOnly},
	{"unpaired_start", "io", TraceNoteCarrierDisplayOnly},
	{"unpaired_done", "io", TraceNoteCarrierDisplayOnly},
	{"ambiguous_cohorts", "io", TraceNoteCarrierDisplayOnly},
	{"pairing_suppressed", "io", TraceNoteCarrierDisplayOnly},
	{"signal", "io", TraceNoteCarrierDisplayOnly},
	{TraceNoteKeyIOPressureSignal, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressureEvidenceQuality, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressureScoreCaliber, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressureConclusion, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressureIOWaitBlockedCount, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressureBlockMaxMS, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressureStorageMaxMS, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressureFileBytes, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressureFileEvents, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
	{TraceNoteKeyIOPressurePageCacheChurn, "io_pressure_caliber", TraceNoteCarrierHardConsumer},
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
