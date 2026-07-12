package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CR-2 组③ P7 / F-3 — 覆盖句子集断言守真 (ledger §29.49 移交, 冷读 F-3,
// 2026-07-12; witness donghu 20260712-133933 树头: 「关注线程睡眠 35.351ms 中
// 15.758ms 已由链上解释」 — the 15.758 pacing segment was CARVED OUT of the
// ×7=35.351 sleep family into its own ∿ row (PACE-ROW), so the 「X 中 Y」
// subset claim was false: the only chain-explained mass sits OUTSIDE the
// quoted sleep total). 判据: the subset wording may not bridge two disjoint
// families; when the chain-explained amount provably equals the carved-out
// cadence-idle segment, the sentence speaks two separate facts.

func cr2F3Projection(pacingImpact float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"app-9511", "aweme-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// Depth-1 chain row whose typed target caliber is the pacing segment.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
				Subject: "app-9511", Object: "s_sleep", StateKind: "s_sleep",
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
				ImpactMS: 15.565, CumulativeImpactMS: 15.565, TargetImpactMS: 15.758,
				Confidence: 0.78},
			// The carved-out ∿ pacing row (target self, value on the cumulative
			// channel — the engine's independent idle row shape).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-pacing",
				Subject: "aweme-17267", Object: "pacing_idle", TypeToken: "pacing_idle",
				Tier: types.TraceCausalTierContextOnly, ChainRelevance: "on_chain",
				CumulativeImpactMS: pacingImpact, Confidence: 0.8},
		},
		SupportingHops: []types.TraceCausalProjectionNode{
			// The ×7 sleep family hop view (pacing already carved out).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-sleepview",
				Subject: "aweme-17267", Object: "s_sleep", StateKind: "s_sleep",
				Predicate: "wakeup_causal_impact",
				ImpactMS:  35.351, EffectiveImpactMS: 35.351,
				MergedCount: 7, MergedMinMS: 1.337, MergedMaxMS: 14.302,
				Confidence: 0.78},
		},
	}
}

// F-3 形 pin: when the chain-explained amount equals the carved-out cadence
// segment, the subset claim 「X 中 Y 已由链上解释」 must not render — the
// sentence states the two disjoint facts instead.
func TestCR2F3CoverageSentenceNoFalseSubsetOverCarvedPacing(t *testing.T) {
	projection := cr2F3Projection(15.758)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if strings.Contains(line, "关注线程睡眠 35.351ms 中 15.758ms 已由链上解释") {
		t.Fatalf("the subset claim bridges two disjoint families (pacing was carved out):\n%s", line)
	}
	if !strings.Contains(line, "关注线程睡眠 35.351ms(不含帧间空闲)") {
		t.Fatalf("the sleep total must disclose the carve-out:\n%s", line)
	}
	if !strings.Contains(line, "链上解释的 15.758ms 为独立成行的帧间空闲段,不在上句睡眠合计内") {
		t.Fatalf("the chain-explained mass must be named as the carved idle segment:\n%s", line)
	}
}

// Control: when the chain-explained amount does NOT equal the carved idle
// segment, the legacy subset sentence stays byte-identical (the fork is a
// precise Round3 equality on typed values, never a heuristic).
func TestCR2F3CoverageSentenceLegacyWhenNoCarveMatch(t *testing.T) {
	projection := cr2F3Projection(9.999) // pacing value ≠ attributed 15.758
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "关注线程睡眠 35.351ms 中 15.758ms 已由链上解释。") {
		t.Fatalf("non-matching shapes keep the legacy sentence:\n%s", line)
	}
}
