package types

import "testing"

// trace_causal_projection_case3d4_test.go — CASE3-D4 engine pins (§29.84 件④
// 裁定 B 根修, LT-HYG CASE-3 ❹ witness, real_trace_campaign_20260705.md,
// 2026-07-14): the plain (non-periodic) ×N merge re-mints the merged row's
// EffectiveImpactMS from its MEMBERS instead of inheriting the group-first
// seed's single-member value ("inherited VALUE stays untouched" retired —
// 「3次(2.000~4.000ms) · 有效归因2.500ms」 read as a total while being one
// member's value with zero qualifying words).
//
//   - Σ arm: EVERY member minted a positive effective → eff = Σ member eff
//     (§29.50.4 合计参赛 direction; per-member attributions of DISTINCT
//     facts), published marker OR-monotone (EPUB, same as the periodic arm);
//   - zero-fallback arm: ANY member without one (0 / never minted) → the
//     whole row clears to 0 unpublished (宁缺勿假 — a member cumulative must
//     NEVER substitute, per the ruling's explicit caliber ban);
//   - cross-window carve: the §11-N2 union / §21 CWD MAX value calibers zero
//     the effective too (墙钟跨窗不可加和 — member effectives on plain rows
//     are the same wall-clock magnitudes the value channel refused to SUM);
//   - both merge entries (the R2 ≥3 pass and the display trunk's threshold-2
//     TraceCausalProjectionMergeOccurrenceRows) share the ONE merge body, so
//     both are pinned; a pre-merged seed Σ is idempotent (grand Σ, no double
//     count — MergedEvidenceIDs stay a distinct set).
//
// Mutation self-check (verified RED during development, then restored):
//   M-D4: restoring the plain arm's inherited-seed copy (aggregate.
//         EffectiveImpactMS left untouched) → TestCase3D4PlainFoldSumsMember-
//         Effective, TestCase3D4OccurrenceMergeSumsMemberEffective and
//         TestCase3D4PreMergedSeedIdempotentSum red (seed 2.500 vs Σ 8.000);
//         TestCase3D4AnyMemberWithoutEffectiveClearsRow red (seed 2.500
//         survives); the display three-face pin
//         TestCase3D4MergedRowThreeFaceSigma (internal/tool) red.

// case3d4Member builds one plain (non-periodic) runnable occurrence row of
// the mission witness shape: display 2.000~4.000ms members with their own
// engine-published effectives.
func case3d4Member(id string, line int, impactMS, effectiveMS float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: "worker-9", Object: "runnable_wait", Predicate: "wakeup_causal_impact",
		StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: impactMS, CumulativeImpactMS: impactMS,
		EffectiveImpactMS:        effectiveMS,
		EffectiveImpactPublished: effectiveMS > 0,
		LineStart:                line, LineEnd: line + 4, Confidence: 0.8,
	}
}

func case3d4Members() []TraceCausalProjectionNode {
	// Seed eff 2.500 (the mission witness's single-member value), Σ member
	// eff = 2.500 + 2.500 + 3.000 = 8.000 ≠ seed 2.500 ≠ display Σ 9.000 —
	// all three magnitudes distinct so an inheritance regression and a
	// display-sum substitution both turn red.
	return []TraceCausalProjectionNode{
		case3d4Member("E1", 100, 2.000, 2.500),
		case3d4Member("E2", 110, 3.000, 2.500),
		case3d4Member("E3", 120, 4.000, 3.000),
	}
}

// TestCase3D4PlainFoldSumsMemberEffective pins the R2 (≥3) entry's Σ arm.
func TestCase3D4PlainFoldSumsMemberEffective(t *testing.T) {
	folded := traceCausalProjectionAggregateSameKind(case3d4Members())
	if len(folded) != 1 || folded[0].MergedCount != 3 {
		t.Fatalf("expected one plain ×3 fold row, got %+v", folded)
	}
	row := folded[0]
	// Σ member eff = 2.500 + 2.500 + 3.000 = 8.000 (hand-checked).
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 8.000) {
		t.Fatalf("plain fold effective must be Σ member eff = 8.000, got %.6f", row.EffectiveImpactMS)
	}
	if !row.EffectiveImpactPublished {
		t.Fatalf("Σ over published member effectives is itself published (EPUB OR-monotone): %+v", row)
	}
	// The value channels keep their own SUM caliber untouched: 2+3+4 = 9.000.
	if !periodicVS1NearlyEqual(row.ImpactMS, 9.000) || !periodicVS1NearlyEqual(row.CumulativeImpactMS, 9.000) {
		t.Fatalf("plain fold raw SUM must stay 9.000, got impact=%.6f cum=%.6f", row.ImpactMS, row.CumulativeImpactMS)
	}
}

// TestCase3D4AnyMemberWithoutEffectiveClearsRow pins the zero-fallback arm:
// one member without a minted effective clears the WHOLE row's effective —
// never the seed's inherited copy, never a cumulative substitution.
func TestCase3D4AnyMemberWithoutEffectiveClearsRow(t *testing.T) {
	members := case3d4Members()
	members[2].EffectiveImpactMS = 0
	members[2].EffectiveImpactPublished = false
	folded := traceCausalProjectionAggregateSameKind(members)
	if len(folded) != 1 || folded[0].MergedCount != 3 {
		t.Fatalf("expected one plain ×3 fold row, got %+v", folded)
	}
	row := folded[0]
	if row.EffectiveImpactMS != 0 {
		t.Fatalf("宁缺勿假: an unminted member must clear the fold effective (no seed inheritance, no cum substitute), got %.6f", row.EffectiveImpactMS)
	}
	if row.EffectiveImpactPublished {
		t.Fatalf("the cleared 0 is an ABSENT effective, never a published engine zero: %+v", row)
	}
	if !periodicVS1NearlyEqual(row.ImpactMS, 9.000) {
		t.Fatalf("the value channel SUM stays lossless beside the cleared effective, got %.6f", row.ImpactMS)
	}
}

// TestCase3D4OccurrenceMergeSumsMemberEffective pins the second merge entry —
// the display trunk's threshold-2 occurrence fold — through the SAME body.
func TestCase3D4OccurrenceMergeSumsMemberEffective(t *testing.T) {
	members := case3d4Members()[:2]
	row := TraceCausalProjectionMergeOccurrenceRows(members)
	if row.MergedCount != 2 {
		t.Fatalf("expected a ×2 occurrence merge, got %+v", row)
	}
	// Σ member eff = 2.500 + 2.500 = 5.000 (hand-checked); seed copy = 2.500.
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 5.000) {
		t.Fatalf("occurrence merge effective must be Σ member eff = 5.000, got %.6f", row.EffectiveImpactMS)
	}
	withoutEff := case3d4Members()[:2]
	withoutEff[1].EffectiveImpactMS = 0
	withoutEff[1].EffectiveImpactPublished = false
	row = TraceCausalProjectionMergeOccurrenceRows(withoutEff)
	if row.EffectiveImpactMS != 0 || row.EffectiveImpactPublished {
		t.Fatalf("occurrence merge zero-fallback must clear the seed copy too: %+v", row)
	}
}

// TestCase3D4PreMergedSeedIdempotentSum pins the Σ idempotence bookkeeping: a
// seed that is ITSELF a merged row (eff already its own member Σ) enters a
// later merge as ONE member — the grand Σ counts every underlying occurrence
// exactly once and MergedEvidenceIDs stays a distinct set.
func TestCase3D4PreMergedSeedIdempotentSum(t *testing.T) {
	preMerged := TraceCausalProjectionMergeOccurrenceRows(case3d4Members()[:2])
	// Pre-merged seed: eff = 2.500 + 2.500 = 5.000 over members E1+E2.
	if !periodicVS1NearlyEqual(preMerged.EffectiveImpactMS, 5.000) {
		t.Fatalf("pre-merged seed premise broke: %.6f", preMerged.EffectiveImpactMS)
	}
	tail := case3d4Member("E3", 120, 4.000, 3.000)
	row := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{preMerged, tail})
	// Grand Σ = 5.000 + 3.000 = 8.000 — E1/E2 counted once through the seed's
	// own Σ, never re-added (no double count).
	if !periodicVS1NearlyEqual(row.EffectiveImpactMS, 8.000) {
		t.Fatalf("grand Σ over a pre-merged seed must be 8.000 (idempotent, no double count), got %.6f", row.EffectiveImpactMS)
	}
	seen := map[string]bool{row.EvidenceID: true}
	for _, id := range row.MergedEvidenceIDs {
		if seen[id] {
			t.Fatalf("MergedEvidenceIDs double-counted %q: %+v", id, row.MergedEvidenceIDs)
		}
		seen[id] = true
	}
	for _, id := range []string{"E1", "E2", "E3"} {
		if !seen[id] {
			t.Fatalf("MergedEvidenceIDs lost %q: %+v", id, row.MergedEvidenceIDs)
		}
	}
}

// TestCase3D4CrossWindowCalibersClearEffective pins the carve: when the value
// channel abandoned the SUM for the §11-N2 union / §21 CWD cross-window MAX
// calibers, the effective channel publishes NOTHING — a Σ member eff there
// would re-mint the very overlapping-window double count the value channel
// retired (and exceed the published union/MAX value).
func TestCase3D4CrossWindowCalibersClearEffective(t *testing.T) {
	build := func(windows [3][2]float64, spans [3][2]float64) []TraceCausalProjectionNode {
		members := case3d4Members()
		for i := range members {
			members[i].QueryWindowStartTs = windows[i][0]
			members[i].QueryWindowEndTs = windows[i][1]
			members[i].StartTs = spans[i][0]
			members[i].EndTs = spans[i][1]
		}
		return members
	}
	// Union shape: two distinct windows, the small-window occurrence contained
	// in a big-window occurrence (the q2-E10 form).
	union := traceCausalProjectionAggregateSameKind(build(
		[3][2]float64{{100.0, 102.0}, {100.0, 102.0}, {100.5, 100.9}},
		[3][2]float64{{100.10, 100.80}, {101.00, 101.40}, {100.60, 100.75}},
	))
	if len(union) != 1 || !union[0].MergedIntervalUnion {
		t.Fatalf("union fixture premise broke: %+v", union)
	}
	if union[0].EffectiveImpactMS != 0 || union[0].EffectiveImpactPublished {
		t.Fatalf("union-caliber row must publish no effective (跨窗不可加和), got %+v", union[0])
	}
	// Cross-window MAX shape: overlapping query windows, no member occurrence
	// interval (the rank-lane no-Span form) — union unavailable.
	max := traceCausalProjectionAggregateSameKind(build(
		[3][2]float64{{100.0, 102.0}, {100.0, 102.0}, {100.5, 103.0}},
		[3][2]float64{{0, 0}, {0, 0}, {0, 0}},
	))
	if len(max) != 1 || !max[0].MergedCrossWindowMax {
		t.Fatalf("cross-window MAX fixture premise broke: %+v", max)
	}
	if max[0].EffectiveImpactMS != 0 || max[0].EffectiveImpactPublished {
		t.Fatalf("cross-window-MAX row must publish no effective (跨窗不可加和), got %+v", max[0])
	}
}
