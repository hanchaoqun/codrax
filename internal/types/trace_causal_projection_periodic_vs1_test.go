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
