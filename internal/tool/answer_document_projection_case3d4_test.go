package tool

// answer_document_projection_case3d4_test.go — CASE3-D4 display three-face pin
// (§29.84 件④ 裁定 B 根修, real_trace_campaign_20260705.md, 2026-07-14): the
// merged plain ×N row's effective face is Σ member eff on ALL THREE faces —
// 行1/tag (tree row), 行 merged per-occurrence disclosure, and the ◎ overview
// seat value — never the seed's single-member inherited value (the LT-HYG
// CASE-3 ➍ witness 「3次(2.000~4.000ms) · 有效归因 2.500ms」 with ◎ seating
// 2.500 as if a total).
//
// Mutation self-check M-D4 (verified RED during development, then restored):
// restoring the engine plain arm's inherited-seed copy turns every assertion
// below red (2.500 resurfaces on the tree tag and the ◎ seat; 8.000
// disappears).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func case3d4DisplayProjection() types.TraceCausalProjection {
	member := func(id string, line int, impactMS, effectiveMS float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: "worker-9", Object: "runnable_wait", Predicate: "root_cause_secondary",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: impactMS, CumulativeImpactMS: impactMS,
			EffectiveImpactMS: effectiveMS, EffectiveImpactPublished: effectiveMS > 0,
			Rank: 1, Confidence: 0.8, LineStart: line, LineEnd: line + 4,
			QueryWindowStartTs: 100.0, QueryWindowEndTs: 100.201,
		}
	}
	// The mission witness shape: displays 2.000/3.000/4.000 (Σ 9.000), member
	// effectives 2.500/2.500/3.000 (Σ 8.000, seed copy 2.500) — the three
	// magnitudes are pairwise distinct so seed inheritance (2.500), display
	// substitution (9.000) and the honest Σ (8.000) are distinguishable.
	merged := types.TraceCausalProjectionMergeOccurrenceRows([]types.TraceCausalProjectionNode{
		member("E1", 100, 2.000, 2.500),
		member("E2", 110, 3.000, 2.500),
		member("E3", 120, 4.000, 3.000),
	})
	return types.TraceCausalProjection{
		WakeupPath:              []string{"worker-9", "target-1"},
		WindowStartTs:           100.0,
		WindowEndTs:             100.201,
		RootCauseFamilyObserved: true,
		OnChainCauses:           []types.TraceCausalProjectionNode{merged},
	}
}

// case3d4MemberWindowSpanProjection is the representative-legend fixture of
// the CASE3-D4 伴生 chip qualifier (huadong_792 E22 geometry re-valued onto
// the canonical MergedSum probe values 10/20/30): a ◇ merged rank seat whose
// three members span TWO query windows — the seat chip must carry
// 「(供席成员窗,成员跨2窗)」 and the row tags keep the canonical
// 3次(10.000~30.000ms) per-occurrence form. Consumed by
// TestTraceProjectionLegendBidirectionalAcrossRepresentativeShapes.
func case3d4MemberWindowSpanProjection() types.TraceCausalProjection {
	member := func(id string, rank int, impact, ws, we float64, line int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: "oney.hmn.berlin-42591", Object: "trace_span",
			SpanName: "H:ReceiveVsync", ChainRelevance: "adjacent",
			ImpactMS: impact, CumulativeImpactMS: impact,
			EffectiveImpactMS: impact, EffectiveImpactPublished: true,
			Rank: rank, Confidence: 0.7, LineStart: line, LineEnd: line + 4,
			QueryWindowStartTs: ws, QueryWindowEndTs: we,
		}
	}
	merged := types.TraceCausalProjectionMergeOccurrenceRows([]types.TraceCausalProjectionNode{
		member("m1", 2, 30.000, 6793222.700, 6793222.901, 100),
		member("m2", 5, 10.000, 6793224.299, 6793224.501, 110),
		member("m3", 0, 20.000, 6793224.299, 6793224.501, 120),
	})
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "target-1"},
		WindowStartTs: 6793222.700,
		WindowEndTs:   6793222.901,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				ChainDepth: 1, ImpactMS: 12.444, CumulativeImpactMS: 12.444,
				EffectiveImpactMS: 5.071, Rank: 1, Confidence: 0.8,
				QueryWindowStartTs: 6793224.299, QueryWindowEndTs: 6793224.501},
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{merged},
	}
}

func TestCase3D4MergedRowThreeFaceSigma(t *testing.T) {
	projection := case3d4DisplayProjection()
	if got := projection.OnChainCauses[0].EffectiveImpactMS; got != 8.0 {
		t.Fatalf("fixture premise: merged eff must be Σ member eff 8.000, got %.3f", got)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// 面1 — 行1/tag: the tree row's effective face carries the Σ, never the
	// seed's single-member 2.500.
	if !strings.Contains(fence, "有效归因 8.000ms") {
		t.Fatalf("tree face must carry 有效归因 8.000ms (Σ member eff):\n%s", fence)
	}
	if strings.Contains(fence, "2.500ms") {
		t.Fatalf("the seed's single-member 2.500 must not survive anywhere on the tree face:\n%s", fence)
	}
	// 面2 — 逐发生 disclosure: the per-occurrence range stays lossless beside
	// the Σ (the reader can still see the 2.000~4.000 members).
	if !strings.Contains(fence, "3次(2.000~4.000ms)") {
		t.Fatalf("tree face must keep the per-occurrence disclosure:\n%s", fence)
	}
	// 面3 — ◎ overview: the seat value is the same Σ (one authority field).
	overview := runtimeTraceProjElimOverviewFence(projection, model, true)
	if !strings.Contains(overview, "8.000ms") {
		t.Fatalf("◎ overview must seat the merged row at Σ 8.000ms:\n%s", overview)
	}
	if strings.Contains(overview, "2.500ms") {
		t.Fatalf("◎ overview must not seat the seed's single-member value:\n%s", overview)
	}
	if !strings.Contains(overview, "worker-9") {
		t.Fatalf("◎ overview must name the merged row's subject:\n%s", overview)
	}
}
