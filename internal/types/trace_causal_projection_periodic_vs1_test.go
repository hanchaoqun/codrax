package types

import (
	"math"
	"testing"
)

// VS-1 (§7.8): the projection compile lifts the typed periodic_source /
// detected_period_ms / lateness_ms / effective_impact_ms rich notes into the
// node's typed fields (berlin live numbers: VSyncGenerator ≈8.302ms cadence,
// lateness 0.071ms, discounted attribution 0.176ms vs raw 36.256ms sleep).
func TestTraceCausalProjectionCompilesPeriodicSourceNotes(t *testing.T) {
	periodic := ObservationRecord{
		ID: "E10", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "root_cause_primary", ClaimKey: "root_cause_primary:vsync",
		Subject: "VSyncGenerator-610", Object: "sleep_wait", Value: "36.256", Unit: "ms",
		Span:       ObservationSpan{LineStart: 100, LineEnd: 200},
		Confidence: 0.82,
		RichNotes: []string{
			"rank=2", "tier=primary", "impact_ms=36.256", "cumulative_impact_ms=36.256",
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
			"dominant_state=s_sleep",
			"periodic_source=true", "detected_period_ms=8.302", "lateness_ms=0.071",
			"effective_impact_ms=0.176",
		},
	}
	plain := ObservationRecord{
		ID: "E4", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "root_cause_primary", ClaimKey: "root_cause_primary:running",
		Subject: "RSUniRenderThre-1963", Object: "running", Value: "4.115", Unit: "ms",
		Span:       ObservationSpan{LineStart: 300, LineEnd: 320},
		Confidence: 0.86,
		RichNotes: []string{
			"rank=1", "tier=primary", "impact_ms=4.115", "cumulative_impact_ms=4.115",
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
			"dominant_state=running",
		},
	}
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{periodic, plain})
	if len(projection.PrimaryRootCauses) != 2 {
		t.Fatalf("expected two primary nodes, got %+v", projection.PrimaryRootCauses)
	}
	var vsync, running *TraceCausalProjectionNode
	for i := range projection.PrimaryRootCauses {
		switch projection.PrimaryRootCauses[i].EvidenceID {
		case "E10":
			vsync = &projection.PrimaryRootCauses[i]
		case "E4":
			running = &projection.PrimaryRootCauses[i]
		}
	}
	if vsync == nil || running == nil {
		t.Fatalf("missing compiled nodes: %+v", projection.PrimaryRootCauses)
	}
	if !vsync.PeriodicSource {
		t.Fatalf("periodic_source=true note must compile into the typed flag: %+v", vsync)
	}
	if vsync.DetectedPeriodMS != 8.302 || vsync.PeriodicLatenessMS != 0.071 {
		t.Fatalf("cadence notes must compile verbatim: period=%.3f lateness=%.3f", vsync.DetectedPeriodMS, vsync.PeriodicLatenessMS)
	}
	if vsync.EffectiveImpactMS != 0.176 {
		t.Fatalf("the discounted attribution must ride EffectiveImpactMS: %+v", vsync)
	}
	if vsync.ImpactMS != 36.256 {
		t.Fatalf("the raw window projection must stay lossless on ImpactMS: %+v", vsync)
	}
	if running.PeriodicSource || running.DetectedPeriodMS != 0 || running.PeriodicLatenessMS != 0 {
		t.Fatalf("non-periodic rows must not take the typed fields: %+v", running)
	}
}

// --- VS-1 F6 (adversarial review 2026-07-04): aggregation propagation --------

// periodicVS1FoldMember builds one berlin ×6 fold member: a periodic
// VSyncGenerator occurrence row with its own raw projection and per-occurrence
// discount.
func periodicVS1FoldMember(id string, line int, impactMS, effectiveMS, latenessMS float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: "VSyncGenerator-610", Object: "sleep_wait", Predicate: "wakeup_causal_impact",
		StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: impactMS, CumulativeImpactMS: impactMS,
		EffectiveImpactMS: effectiveMS,
		PeriodicSource:    true, DetectedPeriodMS: 8.302, PeriodicLatenessMS: latenessMS,
		LineStart: line, LineEnd: line + 4, Confidence: 0.82,
	}
}

func periodicVS1FoldMembers() []TraceCausalProjectionNode {
	// Σ impact = 36.256 (E10 raw), Σ effective = 0.176, Σ lateness = 0.071 —
	// the berlin live aggregate numbers, distributed across the 6 occurrences.
	return []TraceCausalProjectionNode{
		periodicVS1FoldMember("E10", 100, 6.000, 0.030, 0.000),
		periodicVS1FoldMember("E11", 110, 6.400, 0.030, 0.000),
		periodicVS1FoldMember("E12", 120, 6.200, 0.030, 0.000),
		periodicVS1FoldMember("E13", 130, 5.900, 0.030, 0.000),
		periodicVS1FoldMember("E14", 140, 6.100, 0.030, 0.035),
		periodicVS1FoldMember("E15", 150, 5.656, 0.026, 0.036),
	}
}

func periodicVS1NearlyEqual(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

// TestTraceCausalProjectionSameKindFoldRecomputesPeriodicFields pins F6(a),
// berlin ×6 shape: an R2 fold whose members are ALL periodic keeps the flag
// and re-derives the discount from the members — EffectiveImpactMS = Σ member
// effective (0.176, matching the rank row's aggregate face), PeriodicLatenessMS
// = Σ member lateness (a late tick is never hidden by the fold), and
// DetectedPeriodMS stays the group head's cadence. The raw SUM semantics of
// ImpactMS are untouched.
func TestTraceCausalProjectionSameKindFoldRecomputesPeriodicFields(t *testing.T) {
	folded := traceCausalProjectionAggregateSameKind(periodicVS1FoldMembers())
	if len(folded) != 1 {
		t.Fatalf("expected one ×6 fold row, got %+v", folded)
	}
	row := folded[0]
	if row.MergedCount != 6 {
		t.Fatalf("fold must carry ×6, got %+v", row)
	}
	if !row.PeriodicSource || row.DetectedPeriodMS != 8.302 {
		t.Fatalf("all-periodic fold must keep the flag + group-head period: %+v", row)
	}
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.176) {
		t.Fatalf("fold effective must be Σ member effective = 0.176 (rank-row consistent), got %.6f", row.EffectiveImpactMS)
	}
	if !periodicVS1NearlyEqual(row.PeriodicLatenessMS, 0.071) {
		t.Fatalf("fold must not hide member lateness: want Σ=0.071, got %.6f", row.PeriodicLatenessMS)
	}
	if !periodicVS1NearlyEqual(row.ImpactMS, 36.256) {
		t.Fatalf("fold raw SUM must stay 36.256, got %.6f", row.ImpactMS)
	}
}

// TestTraceCausalProjectionSameKindFoldClearsMixedPeriodic pins the F6(a)
// mixed shape: ANY non-periodic member reverts the ×N SUM row to raw
// semantics — flag, cadence fields and the inherited group-first discount are
// all cleared (a part-cadence sum labelled periodic would discount real waits
// it never measured).
func TestTraceCausalProjectionSameKindFoldClearsMixedPeriodic(t *testing.T) {
	members := periodicVS1FoldMembers()[:2]
	plain := periodicVS1FoldMember("E20", 200, 7.000, 0, 0)
	plain.PeriodicSource = false
	plain.DetectedPeriodMS = 0
	plain.PeriodicLatenessMS = 0
	members = append(members, plain)
	folded := traceCausalProjectionAggregateSameKind(members)
	if len(folded) != 1 || folded[0].MergedCount != 3 {
		t.Fatalf("expected one ×3 fold row, got %+v", folded)
	}
	row := folded[0]
	if row.PeriodicSource || row.DetectedPeriodMS != 0 || row.PeriodicLatenessMS != 0 || row.EffectiveImpactMS != 0 {
		t.Fatalf("mixed fold must clear the periodic labeling and the inherited discount: %+v", row)
	}
	if !periodicVS1NearlyEqual(row.ImpactMS, 19.4) {
		t.Fatalf("mixed fold keeps plain raw SUM semantics, got %.6f", row.ImpactMS)
	}
}

// periodicVS1CrossWindowMember builds one cross-window trap member for the
// PERIODIC-DEDUP pins below: a periodic occurrence row carrying its own typed
// occurrence interval (StartTs/EndTs) and query-window identity.
func periodicVS1CrossWindowMember(id string, line int, impactMS, effectiveMS, occStart, occEnd, qwStart, qwEnd float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: "VSyncGenerator-610", Object: "sleep_wait", Predicate: "wakeup_causal_impact",
		StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: impactMS, CumulativeImpactMS: impactMS,
		EffectiveImpactMS: effectiveMS, EffectiveImpactPublished: true,
		PeriodicSource: true, DetectedPeriodMS: 8.302, PeriodicLatenessMS: 0,
		StartTs: occStart, EndTs: occEnd,
		QueryWindowStartTs: qwStart, QueryWindowEndTs: qwEnd,
		LineStart: line, LineEnd: line + 4, Confidence: 0.82,
	}
}

// periodicVS1CrossWindowMerge runs the trap members through the full
// presentation aggregation and returns the single merged ×N row.
func periodicVS1CrossWindowMerge(t *testing.T, members []TraceCausalProjectionNode) TraceCausalProjectionNode {
	t.Helper()
	projection := TraceCausalProjection{OnChainCauses: members}
	traceCausalProjectionAggregateForPresentation(&projection)
	if len(projection.OnChainCauses) != 1 {
		t.Fatalf("trap members must reach ONE merged row (R1/V4 must not fold the value-divergent re-measurement), got %+v", projection.OnChainCauses)
	}
	row := projection.OnChainCauses[0]
	if row.MergedCount != len(members) || !row.PeriodicSource {
		t.Fatalf("trap fixture drifted: want an all-periodic ×%d fold, got %+v", len(members), row)
	}
	return row
}

// TestTraceCausalProjectionPeriodicCrossWindowDoubleCountSentinel is the
// §29.98 件2 合成双计诱错 pin on the shape §29.85 残留① flagged: two
// OVERLAPPING query windows re-measure the SAME physical periodic occurrence
// (E10 full 36ms / E11 window-clipped 16ms, identical line range 100–104),
// plus one distinct occurrence (E12).
//
// EVOLUTION RECORD (PERIODIC-DEDUP, §29.104 ① 终判, user 2026-07-15): this
// sentinel originally nailed the DOUBLE COUNT — the value channel's union
// caliber deduped the re-measurement (66.000 = 36 + 0(contained) + 30, the
// engine PROVING the two members re-measure one occurrence) while the
// periodic Σ-effective lane (VS-1 F6(a)) still summed ALL member discounts
// (0.090 = 0.030×3, the shared occurrence's discount counted TWICE). The
// §29.96.2 终判⑤ ruling had MAINTAINED that Σ pending adjudication; the
// §29.104 ① 终判 ruled the dedup IN: the Σ-effective lane now consumes the
// SAME same-segment proof as the value channel (window slots + occurrence
// interval overlap, traceCausalProjectionPeriodicDiscountCounted) and the
// shared occurrence's discount counts ONCE — 0.060, the unique Σ. A red here
// means the engine drifted off the §29.104 ① ruling; consult that record
// (docs/design/real_trace_campaign_20260705.md) before repinning.
func TestTraceCausalProjectionPeriodicCrossWindowDoubleCountSentinel(t *testing.T) {
	// E10/E11: ONE physical occurrence (lines 100–104) measured from two
	// overlapping query windows (E11's projection clipped by window 2). The
	// projected values differ (36 vs 16), so neither the R1 same-fact key
	// (value-keyed) nor the V4 ≤3% near-duplicate lane folds them — both
	// legitimately reach the R2 ×N merge. E12: a distinct occurrence.
	row := periodicVS1CrossWindowMerge(t, []TraceCausalProjectionNode{
		periodicVS1CrossWindowMember("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100),
		periodicVS1CrossWindowMember("E11", 100, 16.0, 0.030, 10.020, 10.036, 10.020, 10.120),
		periodicVS1CrossWindowMember("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100),
	})
	// Value channel: union caliber engaged and deduped the re-measurement.
	if !row.MergedIntervalUnion || !periodicVS1NearlyEqual(row.ImpactMS, 66.0) {
		t.Fatalf("union caliber must dedup the value channel (66.000), got union=%v impact=%.3f", row.MergedIntervalUnion, row.ImpactMS)
	}
	// 哨兵 (§29.104 ① behavior): the shared occurrence's discount counts ONCE
	// — 0.060 (E10 seat-window copy + E12), never the double-counted 0.090.
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.060) {
		t.Fatalf("哨兵: periodic cross-window Σ must dedup the re-measured occurrence's discount (§29.104 ①): want 0.060, got %.6f", row.EffectiveImpactMS)
	}
	if !row.EffectiveImpactPublished {
		t.Fatalf("deduped Σ over published member discounts stays published: %+v", row)
	}
}

// TestTraceCausalProjectionPeriodicCrossWindowDedupSeatWindowCopy pins the
// §29.104 ① pick rule: when the re-measured copies carry DIFFERENT discount
// values, the seat-owning window's copy is the one counted — typed window
// membership only (种子/席位窗优先), never a value heuristic.
func TestTraceCausalProjectionPeriodicCrossWindowDedupSeatWindowCopy(t *testing.T) {
	// Rankless group: the SEAT window is the SEED member's own window
	// (window 1), so E10's 0.030 — not E11's diverging 0.050 — enters the Σ:
	// 0.030 (E10) + 0.030 (E12) = 0.060.
	row := periodicVS1CrossWindowMerge(t, []TraceCausalProjectionNode{
		periodicVS1CrossWindowMember("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100),
		periodicVS1CrossWindowMember("E11", 100, 16.0, 0.050, 10.020, 10.036, 10.020, 10.120),
		periodicVS1CrossWindowMember("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100),
	})
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.060) {
		t.Fatalf("seed-window copy must win the divergent re-measurement (0.030+0.030): want 0.060, got %.6f", row.EffectiveImpactMS)
	}

	// Ranked group: the rank-supplying member (E11, window 2) carries the
	// ordinal, so the SEAT window is window 2 (typed RankQueryWindow, DISP-3
	// identity) and E11's 0.050 is the counted copy: 0.050 + 0.030 = 0.080.
	ranked := periodicVS1CrossWindowMember("E11", 100, 16.0, 0.050, 10.020, 10.036, 10.020, 10.120)
	ranked.Rank = 1
	row = periodicVS1CrossWindowMerge(t, []TraceCausalProjectionNode{
		periodicVS1CrossWindowMember("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100),
		ranked,
		periodicVS1CrossWindowMember("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100),
	})
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.080) {
		t.Fatalf("rank-seat window copy must win the divergent re-measurement (0.050+0.030): want 0.080, got %.6f", row.EffectiveImpactMS)
	}
}

// TestTraceCausalProjectionPeriodicCrossWindowDedupPublishedOverCounted pins
// the EPUB direction of the dedup (复核 P2-1, 2026-07-15): the fold's
// EffectiveImpactPublished is the OR over COUNTED members only — a SKIPPED
// re-measurement's marker speaks for a copy that is not in the Σ. Trap: the
// skipped window-2 copy is the ONLY published member; the counted copies are
// unpublished → the merged row must NOT publish (an OR-over-all regression
// would wrongly publish a Σ built purely from unpublished copies).
func TestTraceCausalProjectionPeriodicCrossWindowDedupPublishedOverCounted(t *testing.T) {
	unpublished := func(id string, line int, impactMS, effectiveMS, occStart, occEnd, qwStart, qwEnd float64) TraceCausalProjectionNode {
		node := periodicVS1CrossWindowMember(id, line, impactMS, effectiveMS, occStart, occEnd, qwStart, qwEnd)
		node.EffectiveImpactPublished = false
		return node
	}
	row := periodicVS1CrossWindowMerge(t, []TraceCausalProjectionNode{
		unpublished("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100),
		periodicVS1CrossWindowMember("E11", 100, 16.0, 0.030, 10.020, 10.036, 10.020, 10.120), // published, SKIPPED
		unpublished("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100),
	})
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.060) {
		t.Fatalf("trap fixture drifted (want deduped Σ 0.060), got %.6f", row.EffectiveImpactMS)
	}
	if row.EffectiveImpactPublished {
		t.Fatalf("published marker must OR over COUNTED members only — the skipped copy's marker must not publish the fold: %+v", row)
	}
}

// TestTraceCausalProjectionPeriodicCrossWindowDedupLateness pins the lateness
// half of the per-occurrence dedup (复核 P2-2/P3, 2026-07-15; 主会话追认
// lateness rides the same occurrence identity): one physical late tick
// carries ONE lateness amount, and the counted copy's lateness is the one
// that enters — both pick directions.
func TestTraceCausalProjectionPeriodicCrossWindowDedupLateness(t *testing.T) {
	withLateness := func(node TraceCausalProjectionNode, latenessMS float64) TraceCausalProjectionNode {
		node.PeriodicLatenessMS = latenessMS
		return node
	}
	members := func(rankOnE11 bool) []TraceCausalProjectionNode {
		e11 := withLateness(periodicVS1CrossWindowMember("E11", 100, 16.0, 0.050, 10.020, 10.036, 10.020, 10.120), 0.009)
		if rankOnE11 {
			e11.Rank = 1
		}
		return []TraceCausalProjectionNode{
			withLateness(periodicVS1CrossWindowMember("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100), 0.005),
			e11,
			withLateness(periodicVS1CrossWindowMember("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100), 0.008),
		}
	}
	// Rankless: the seed-window copies count — lateness 0.005 (E10) + 0.008
	// (E12) = 0.013, never the triple 0.022.
	row := periodicVS1CrossWindowMerge(t, members(false))
	if !periodicVS1NearlyEqual(row.PeriodicLatenessMS, 0.013) {
		t.Fatalf("rankless lateness Σ must dedup to the seed-window copies (0.005+0.008): want 0.013, got %.6f", row.PeriodicLatenessMS)
	}
	// Ranked on E11: the seat-window copy's lateness 0.009 replaces E10's
	// 0.005 — 0.009 + 0.008 = 0.017.
	row = periodicVS1CrossWindowMerge(t, members(true))
	if !periodicVS1NearlyEqual(row.PeriodicLatenessMS, 0.017) {
		t.Fatalf("ranked lateness Σ must take the seat-window copy (0.009+0.008): want 0.017, got %.6f", row.PeriodicLatenessMS)
	}
}

// TestTraceCausalProjectionPeriodicCrossWindowDedupNestedAndChain promotes
// the 复核 P2-3 (2026-07-15) ad-hoc probes into resident pins — the two
// deeper multi-window geometries the trap pair does not exercise:
//
//   - THREE nested windows (A⊃B⊃C) re-measuring ONE occurrence: exactly one
//     copy of the shared occurrence counts, in both pick directions;
//   - the CHAIN shape: a SKIPPED re-measurement must not chain-knock a third
//     member that overlaps only the skipped copy (its interval is never
//     recorded), so the third member's discount survives.
func TestTraceCausalProjectionPeriodicCrossWindowDedupNestedAndChain(t *testing.T) {
	// Nested A⊃B⊃C: windows 10.000–10.100 ⊃ 10.010–10.090 ⊃ 10.015–10.080,
	// the shared occurrence re-measured three times (E10 win-A 0.030 / E11
	// win-B 0.050 / E13 win-C 0.040), plus the distinct E12 (win-A, 0.030).
	nested := func(rankOnE11 bool) []TraceCausalProjectionNode {
		e11 := periodicVS1CrossWindowMember("E11", 100, 16.0, 0.050, 10.020, 10.036, 10.010, 10.090)
		if rankOnE11 {
			e11.Rank = 1
		}
		e13 := periodicVS1CrossWindowMember("E13", 100, 14.0, 0.040, 10.022, 10.036, 10.015, 10.080)
		return []TraceCausalProjectionNode{
			periodicVS1CrossWindowMember("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100),
			e11,
			e13,
			periodicVS1CrossWindowMember("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100),
		}
	}
	// Rankless: seat = seed window A → E10's 0.030 + E12's 0.030 = 0.060
	// (both nested re-measurements skipped).
	row := periodicVS1CrossWindowMerge(t, nested(false))
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.060) {
		t.Fatalf("nested rankless: exactly one copy of the shared occurrence counts (0.030+0.030): want 0.060, got %.6f", row.EffectiveImpactMS)
	}
	// Ranked on E11 (window B): its 0.050 is the counted copy — 0.050 + 0.030
	// = 0.080 (E10 and E13 skipped as the other two re-measurements).
	row = periodicVS1CrossWindowMerge(t, nested(true))
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.080) {
		t.Fatalf("nested ranked: the seat-window copy counts (0.050+0.030): want 0.080, got %.6f", row.EffectiveImpactMS)
	}

	// Chain no-knockout: E11 (win2) overlaps counted E10 (win1) → skipped;
	// E13 (win3) overlaps ONLY the skipped E11, never E10/E12 → E13 must
	// stay counted (0.030 E10 + 0.030 E12 + 0.020 E13 = 0.080). A regression
	// that records skipped intervals would knock E13 out (0.060).
	row = periodicVS1CrossWindowMerge(t, []TraceCausalProjectionNode{
		periodicVS1CrossWindowMember("E10", 100, 20.0, 0.030, 10.000, 10.030, 10.000, 10.100),
		periodicVS1CrossWindowMember("E11", 100, 16.0, 0.030, 10.020, 10.050, 10.010, 10.110),
		periodicVS1CrossWindowMember("E13", 100, 10.0, 0.020, 10.040, 10.060, 10.020, 10.120),
		periodicVS1CrossWindowMember("E12", 200, 15.0, 0.030, 10.070, 10.090, 10.000, 10.100),
	})
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.080) {
		t.Fatalf("chain shape: a skipped copy must not chain-knock a third member (0.030+0.030+0.020): want 0.080, got %.6f", row.EffectiveImpactMS)
	}
}

// TestTraceCausalProjectionPeriodicCrossWindowDedupNegativeShapes pins the
// §29.104 ① 负向臂 (F6(a) legal Σ must not be hurt — 禁一刀切清零): the
// dedup engages ONLY on a proven cross-window re-measurement, so every other
// shape keeps the plain member-order Σ byte-identically.
func TestTraceCausalProjectionPeriodicCrossWindowDedupNegativeShapes(t *testing.T) {
	// (1) Disjoint multi-window: two windows, three DISJOINT occurrences —
	// distinct facts across windows legitimately Σ (value channel keeps the
	// SUM too: 36+16+30).
	row := periodicVS1CrossWindowMerge(t, []TraceCausalProjectionNode{
		periodicVS1CrossWindowMember("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100),
		periodicVS1CrossWindowMember("E11", 300, 16.0, 0.030, 10.160, 10.176, 10.150, 10.250),
		periodicVS1CrossWindowMember("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100),
	})
	if row.MergedIntervalUnion || row.MergedCrossWindowMax || !periodicVS1NearlyEqual(row.ImpactMS, 82.0) {
		t.Fatalf("disjoint-window fixture drifted (want plain SUM 82.000): %+v", row)
	}
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.090) {
		t.Fatalf("disjoint multi-window Σ must stay the full member Σ: want 0.090, got %.6f", row.EffectiveImpactMS)
	}

	// (2) Single window: overlapping same-window occurrences are DISTINCT
	// facts (the E9/E10 strict pin direction) — no window fork, no dedup.
	row = periodicVS1CrossWindowMerge(t, []TraceCausalProjectionNode{
		periodicVS1CrossWindowMember("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100),
		periodicVS1CrossWindowMember("E11", 100, 16.0, 0.030, 10.020, 10.036, 10.000, 10.100),
		periodicVS1CrossWindowMember("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100),
	})
	if row.MergedIntervalUnion || row.MergedCrossWindowMax || !periodicVS1NearlyEqual(row.ImpactMS, 82.0) {
		t.Fatalf("single-window fixture drifted (want plain SUM 82.000): %+v", row)
	}
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.090) {
		t.Fatalf("single-window Σ must stay the full member Σ: want 0.090, got %.6f", row.EffectiveImpactMS)
	}

	// (3) §21 CWD windowed-no-interval shape: overlapping windows but E11
	// exposes NO occurrence interval — the double count is UNPROVABLE, the
	// value channel fails open to the member MAX, and the Σ-effective lane
	// fails open to the full Σ (an unprovable same-segment never deducts).
	noInterval := periodicVS1CrossWindowMember("E11", 100, 16.0, 0.030, 0, 0, 10.020, 10.120)
	row = periodicVS1CrossWindowMerge(t, []TraceCausalProjectionNode{
		periodicVS1CrossWindowMember("E10", 100, 36.0, 0.030, 10.000, 10.036, 10.000, 10.100),
		noInterval,
		periodicVS1CrossWindowMember("E12", 200, 30.0, 0.030, 10.050, 10.080, 10.000, 10.100),
	})
	if !row.MergedCrossWindowMax || !periodicVS1NearlyEqual(row.ImpactMS, 36.0) {
		t.Fatalf("windowed-no-interval fixture drifted (want §21 CWD member MAX 36.000): %+v", row)
	}
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 0.090) {
		t.Fatalf("unprovable same-segment must fail open to the full Σ: want 0.090, got %.6f", row.EffectiveImpactMS)
	}
}

// TestTraceCausalProjectionSameFactMergeKeepsAuthoritativeZero pins F6(b): an
// R1 same-fact merge must never backfill a periodic survivor's EffectiveImpactMS
// — the discounted 0 is the authoritative value (pure in-period cadence), and
// the raw-lane twin's positive effective would resurrect the discounted sleep.
// The survivor's periodic triple stays as published. A NON-periodic survivor
// keeps the pre-VS-1 backfill behaviour.
func TestTraceCausalProjectionSameFactMergeKeepsAuthoritativeZero(t *testing.T) {
	survivor := periodicVS1FoldMember("E10", 100, 36.256, 0, 0)
	twin := periodicVS1FoldMember("E99", 100, 36.256, 36.256, 0)
	twin.PeriodicSource = false
	twin.DetectedPeriodMS = 0
	projection := TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{survivor, twin}}
	traceCausalProjectionMergeSameFacts(&projection)
	if len(projection.OnChainCauses) != 1 {
		t.Fatalf("same-fact twins must merge, got %+v", projection.OnChainCauses)
	}
	merged := projection.OnChainCauses[0]
	if merged.EffectiveImpactMS != 0 {
		t.Fatalf("the authoritative periodic 0 must not be resurrected by the raw-lane twin: %+v", merged)
	}
	if !merged.PeriodicSource || merged.DetectedPeriodMS != 8.302 {
		t.Fatalf("the survivor's periodic triple must stay intact: %+v", merged)
	}

	// Control: a non-periodic survivor still takes the loser's effective.
	plainSurvivor := periodicVS1FoldMember("E10", 100, 36.256, 0, 0)
	plainSurvivor.PeriodicSource = false
	plainSurvivor.DetectedPeriodMS = 0
	plainProjection := TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{plainSurvivor, twin}}
	traceCausalProjectionMergeSameFacts(&plainProjection)
	if got := plainProjection.OnChainCauses[0].EffectiveImpactMS; got != 36.256 {
		t.Fatalf("non-periodic survivors keep the pre-VS-1 backfill, got %.3f", got)
	}
}
