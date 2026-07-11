package tool

// answer_document_projection_epub_test.go — EPUB 批 pins (ledger
// real_trace_campaign_20260705.md §29.31 立案, 2026-07-11): the GENERAL
// published-eff≤0 crown refusal on the V1 rankless lead lane, gated on the
// typed EffectiveImpactPublished marker with the #68 PeriodicSource typed
// exemption. The three preserved forms stay pinned elsewhere byte-identically:
//   - VS2 故意 pin (eff-UNPUBLISHED ranked rows crown + 行3 值拒造):
//     TestSupplyFoldClauseTripleBranchZH / TestSupplyFoldDeficitNeverRanks /
//     TestLockHolderInferredLeadQualifierP0E and the witness goldens;
//   - #68 用户裁定 (periodic published-0 crowns with the discounted-zero
//     wording): TestPTV5PeriodicZeroConclusionRendersDiscountedZero —
//     untouched, its survival IS the exemption proof;
//   - CLOSE-1 P6 (zero-CAP running, fold-telemetry arm): stays beside this
//     generalization for eff-UNPUBLISHED zero-CAP forms.
//
// 突变自查 (each applied locally and verified red before ship):
//   (M1) 记号摘除 — decode mint removed (marker always false) →
//        TestEPUBEffectivePublishedMintFromNotePresence + the end-to-end pin;
//   (M2) 豁免臂摘除 — !PeriodicSource dropped from the arm →
//        TestEPUBPublishedZeroPeriodicKeepsDiscountedZeroCrown (the original
//        #68 pin's fixture stays marker-false, so it survives M2 by the
//        marker gate — the EPUB pin is the published-form exemption witness);
//   (M3) 一般化臂摘除 — the new arm removed →
//        TestEPUBPublishedZeroRootCrownsNothing (both variants) +
//        TestEPUBPublishedZeroRootRefusalEndToEnd;
//   (M4) 未发布误判发布 (mint 侧) — mint flipped to always-true → the mint pin
//        + the record-driven VS2-class pins (TestSupplyFoldClauseTripleBranchZH
//        / TestSupplyFoldDeficitNeverRanks / TestTraceProjectionD2TypeLabels…).
//        NOTE (账目如实): this variant does NOT bite
//        TestEPUBUnpublishedZeroRankedRootKeepsCrown — that fixture is a node
//        LITERAL that never passes the mint; the lane pin is bitten by the
//        arm-side variant below. Both variants are covered — no hole;
//   (M5) 未发布误判发布 (臂侧, the naive literal form) — arm drops the marker
//        gate (fires on every eff≤0) →
//        TestEPUBUnpublishedZeroRankedRootKeepsCrown + the VS2 suite;
//   (M6) root_evidence 豁免臂摘除 (复核 M1) — sentinel exemption dropped from
//        the mint → TestEPUBRootEvidenceSentinelZeroNeverMintsPublished +
//        TestEPUBRootEvidenceSentinelNeverTransplantsPublished;
//   (P1) R1 OR 臂摘除 (复核加测) — merge marker propagation removed →
//        TestEPUBSameFactMergePublishedFollowsTheFact;
//   (P2) mixed-clear 去发布摘除 (复核加测) — the ×N mixed clear keeps the
//        inherited marker → TestEPUBSameKindFoldPublishedPropagation;
//   (L1-mut) plain-fold 去发布摘除 — the plain ×N fold keeps an inherited
//        published-0 seed marker → TestEPUBSameKindFoldPublishedPropagation.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func epubPublishedZeroRoot(effMS float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-pubzero",
		Subject: "worker-200", Object: "sleep_wait", TypeToken: "sleep_wait",
		StateKind: "s_sleep", Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 42.000, CumulativeImpactMS: 42.000,
		EffectiveImpactMS: effMS, EffectiveImpactPublished: true,
		ChainRelevance: "on_chain", Confidence: 0.8,
	}
}

// TestEPUBPublishedZeroRootCrownsNothing is the target-form pin (red before
// the EPUB arm): a root whose engine-PUBLISHED effective attribution is ≤0 —
// non-periodic, no fold telemetry — crowns nothing even as the ONLY root.
// The raw 42ms display projection must not resurrect through the value lane
// (the pre-EPUB behaviour) nor through the RN-3(a) fallback (its eff>0 hard
// gate). Lead absent + zero glyphs is the honest form, same terminal as the
// CLOSE-1 P6 shape.
func TestEPUBPublishedZeroRootCrownsNothing(t *testing.T) {
	for name, effMS := range map[string]float64{"published zero": 0, "published negative": -0.5} {
		root := epubPublishedZeroRoot(effMS)
		projection := types.TraceCausalProjection{
			WindowStartTs:     100.000,
			WindowEndTs:       100.600,
			PrimaryRootCauses: []types.TraceCausalProjectionNode{root},
			OnChainCauses:     []types.TraceCausalProjectionNode{root},
		}
		model := buildRuntimeTraceProjTreeModel(projection, nil, true)
		lead, lane := runtimeTraceProjLeadSelect(projection, model)
		if lead != nil {
			t.Fatalf("%s: 已发布 eff≤0 不得加冕 (lane=%d): %+v", name, lane, lead)
		}
		line := runtimeTraceProjConclusionLine(projection, model, true)
		if !strings.Contains(line, "未定位到链上主根因") {
			t.Fatalf("%s: the honest fallback must speak:\n%s", name, line)
		}
		if strings.Contains(line, "42.000") {
			t.Fatalf("%s: the raw display projection must not resurrect:\n%s", name, line)
		}
		for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
			for _, row := range rows {
				if row.Badge != 0 {
					t.Fatalf("%s: zero-seat shape must wear zero glyphs: %+v", name, row)
				}
			}
		}
	}
}

// TestEPUBPublishedZeroRootRefusalEndToEnd drives the same target form
// through the REAL record decode (mint → compile → lane), so the marker chain
// cannot be faked by a fixture literal: an explicit effective_impact_ms=0.000
// note on a tier=primary rank row refuses the crown end-to-end.
func TestEPUBPublishedZeroRootRefusalEndToEnd(t *testing.T) {
	record := types.ObservationRecord{
		ID:              "epub-e2e",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary:sleep_wait",
		Subject:         "worker-200",
		Object:          "sleep_wait",
		Value:           "42.000",
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: 120, LineEnd: 140},
		RichNotes: []string{
			"tier=primary", "rank=1", "impact_ms=42.000",
			"cumulative_impact_ms=42.000", "effective_impact_ms=0.000",
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
			"dominant_state=s_sleep",
		},
		Confidence: 0.9,
	}
	projection := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{record})
	if len(projection.PrimaryRootCauses) != 1 || !projection.PrimaryRootCauses[0].EffectiveImpactPublished {
		t.Fatalf("fixture must compile one PUBLISHED primary root, got %+v", projection.PrimaryRootCauses)
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if lead, lane := runtimeTraceProjLeadSelect(projection, model); lead != nil {
		t.Fatalf("end-to-end published-0 root must not crown (lane=%d): %+v", lane, lead)
	}
}

// TestEPUBPublishedZeroPeriodicKeepsDiscountedZeroCrown is the marker-true
// witness of the #68 typed exemption: the engine-true periodic wire form
// (VS-1 lane prints effective_impact_ms=0.000 explicitly → published) KEEPS
// the discounted-zero crown and its user-ruled wording. The original #68 pin
// (TestPTV5PeriodicZeroConclusionRendersDiscountedZero) stays untouched.
func TestEPUBPublishedZeroPeriodicKeepsDiscountedZeroCrown(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"vsync-1", "app-1"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.1,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-p",
			Subject: "vsync-1", Object: "sleep_wait", StateKind: "s_sleep",
			ChainRelevance: "on_chain", Rank: 1,
			ImpactMS: 36.0, CumulativeImpactMS: 36.0,
			EffectiveImpactMS: 0, EffectiveImpactPublished: true,
			PeriodicSource: true, DetectedPeriodMS: 8.3, Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.EvidenceID != "e-p" {
		t.Fatalf("#68 exemption: the published-0 periodic root must keep its crown, got lane=%d lead=%+v", lane, lead)
	}
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "有效归因 0.000ms(期内节拍已折算)") {
		t.Fatalf("#68 wording must survive the EPUB arm:\n%s", line)
	}
}

// TestEPUBRootEvidenceSentinelNeverTransplantsPublished is the 复核 M1 probe
// pin (red before the root_evidence mint exemption): the root_evidence lane's
// effective_impact_ms=0.000 is a RANKING-EXCLUSION SENTINEL (trace_query.go
// root_evidence emit comment: the reduced-shape witness "does not carry
// CAP/gated/state-union provenance, so only the richer lanes may participate
// in ranking"), never an authoritative published zero. When such a witness
// triple-collides (subject + %.3f + line span) with an eff-UNPUBLISHED ranked
// row, the R1 same-fact OR arm must NOT transplant the sentinel as published
// authority onto the survivor — pre-fix that transplant refused a crown the
// V1 raw-value lane legitimately held (假阳, breaking the fail-open claim).
func TestEPUBRootEvidenceSentinelNeverTransplantsPublished(t *testing.T) {
	rankRow := types.ObservationRecord{
		ID:              "epub-rank",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary:sleep_wait",
		Subject:         "worker-200",
		Object:          "sleep_wait",
		Value:           "42.000",
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: 120, LineEnd: 140},
		RichNotes: []string{
			"tier=primary", "rank=1", "impact_ms=42.000",
			"cumulative_impact_ms=42.000",
			"chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=s_sleep",
		},
		Confidence: 0.9,
	}
	rootEvidence := types.ObservationRecord{
		ID:              "epub-rootev",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "missing_wakeup",
		ClaimKey:        "root_evidence:missing_wakeup",
		Subject:         "worker-200",
		Value:           "42.000",
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: 120, LineEnd: 140},
		RichNotes: []string{
			"tier=context_only",
			"effective_impact_ms=0.000",
		},
		Confidence: 0.9,
	}
	projection := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{rankRow, rootEvidence})
	if len(projection.PrimaryRootCauses) != 1 {
		t.Fatalf("fixture must compile one primary root, got %+v", projection.PrimaryRootCauses)
	}
	merged := projection.PrimaryRootCauses[0]
	if merged.EffectiveImpactPublished {
		t.Fatalf("复核 M1: the root_evidence sentinel zero must not become published authority on the merged survivor: %+v", merged)
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.EvidenceID != "epub-rank" {
		t.Fatalf("the eff-unpublished ranked survivor must keep its V1 crown, got lane=%d lead=%+v", lane, lead)
	}
}

// TestEPUBUnpublishedZeroRankedRootKeepsCrown pins the VS2-form boundary at
// lane level: an eff-UNPUBLISHED (marker false) ranked root keeps the V1
// value-lane crown — the refusal arm reads the typed marker, never the
// float64 zero (M4 未发布误判发布 bites here).
func TestEPUBUnpublishedZeroRankedRootKeepsCrown(t *testing.T) {
	root := epubPublishedZeroRoot(0)
	root.EffectiveImpactPublished = false
	projection := types.TraceCausalProjection{
		WakeupPath:        []string{"waker-1", "app-1"},
		WindowStartTs:     100.000,
		WindowEndTs:       100.600,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{root},
		OnChainCauses:     []types.TraceCausalProjectionNode{root},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.EvidenceID != "e-pubzero" {
		t.Fatalf("unpublished ranked root must keep the V1 crown (VS2 form), got lane=%d lead=%+v", lane, lead)
	}
}
