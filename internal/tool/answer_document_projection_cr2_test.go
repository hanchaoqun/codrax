package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CR-2 组① P4 徽章-图例闭合门 display pins (ledger §29.42 P4, witness 冷读 F-6
// 2026-07-12): the compile-layer seat white-list
// (traceCausalProjectionSeatFoldExempt, internal/types) keeps every published
// TOP-5 seat row out of the counted overflow folds; these pins hold the display
// half of the promise — the exemption bound equals the badge population, and an
// exempted seat row actually wears its glyph on the rendered fence.

// 两包常量恒等 pin: the fold exemption population IS the ❶..❺ badge population.
// A drift here re-opens F-6 (a seat row folding away while the legend promises
// its glyph) or over-exempts unseated rows.
func TestCR2P4SeatExemptTopNMatchesBadgeTopN(t *testing.T) {
	if runtimeTraceProjBadgeTopN != types.TraceCausalProjectionSeatFoldExemptTopN {
		t.Fatalf("badge TopN (%d) and seat fold-exempt TopN (%d) must stay equal",
			runtimeTraceProjBadgeTopN, types.TraceCausalProjectionSeatFoldExemptTopN)
	}
}

// F-6 显示半场 pin: a rank-2 chain row that survives the fold (the exempted
// shape) wears ❷ on the fence, and the fold roster beside it stays badge-free
// with its honest count.
func TestCR2P4FoldExemptSeatRowWearsBadgeTwo(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "target-9"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-seat1",
				Subject: "holder-1", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
				StateKind: "d_sleep", Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
				ImpactMS: 36.757, CumulativeImpactMS: 36.757, EffectiveImpactMS: 36.757,
				ChainRelevance: "on_chain", Confidence: 0.82, LineStart: 10, LineEnd: 20},
			// The F-6 witness shape post-exemption: seat #2 as an individual row.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-seat2",
				Subject: "seatholder-2", Object: "runnable_wait", TypeToken: "runnable_wait",
				StateKind: "runnable", Predicate: "root_cause_secondary", Rank: 2, Tier: "secondary",
				ImpactMS: 16.687, CumulativeImpactMS: 16.687, EffectiveImpactMS: 16.687,
				ChainRelevance: "on_chain", Confidence: 0.76, LineStart: 30, LineEnd: 40},
			// The fold roster the seat row was carved out of (count already
			// honest at the compile layer).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-fold",
				Subject: "", Object: "runnable_wait", TypeToken: "runnable_wait",
				Predicate: "root_cause_context", OnChainOverflowFold: true,
				MergedCount: 2, MergedMinMS: 1.1, MergedMaxMS: 2.0, ImpactMS: 2.0,
				EffectiveImpactMS: 2.0, ChainRelevance: "on_chain", Confidence: 0.5},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, runtimeTraceProjBadgeGlyph(2)) {
		t.Fatalf("the exempted seat #2 row must wear ❷ on the fence:\n%s", fence)
	}
	for _, line := range strings.Split(fence, "\n") {
		if !strings.Contains(line, "项(折叠)") {
			continue
		}
		for r := 1; r <= runtimeTraceProjBadgeTopN; r++ {
			if strings.Contains(line, runtimeTraceProjBadgeGlyph(r)) {
				t.Fatalf("the fold roster must stay badge-free: %q", line)
			}
		}
	}
}
