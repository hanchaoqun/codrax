package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CR-2 组③ P7 口径标签守真门 — ⚠ 词面 typed-interval gate (ledger §29.42 P7;
// witnesses: 冷读案19 「⚠ 词面 11 行全假」 — donghu E5 app-9511 sleep projection
// 15.565 / actual 16.433 with actual_window 13762.991547..13763.008274 fully
// INSIDE the analysis window 13762.791708..13763.024898, yet the row claimed
// 实际状态跨出分析窗; CAL-1 冷读 F-5 — tieba E4 「17.442ms 15% ⚠实际6.936ms」:
// a ×N merged row's single-member actual reading as an 实际<投影 paradox).
// 判据: ⚠ 仅当 actual_interval ⊄ analysis_window (typed containment), 禁自由
// 文本;合并行单成员值必须标注来源口径.

func cr2P7Node(actual, projection float64, winStart, winEnd float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#wakeup_causal_impact:5",
		Subject: "app-1", Predicate: "wakeup_causal_impact", Object: "s_sleep",
		StateKind: "s_sleep", ChainRelevance: "on_chain",
		ImpactMS: projection, CumulativeImpactMS: projection, ActualImpactMS: actual,
		ActualWindowStartTs: winStart, ActualWindowEndTs: winEnd,
		Confidence: 0.78, LineStart: 22936, LineEnd: 24452,
	}
}

func cr2P7Projection(node types.TraceCausalProjectionNode) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"app-1", "target-9"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
}

// 冷读案19 形 pin: an actual interval fully INSIDE the analysis window must
// not wear ⚠ — the overshoot beyond the row's own episode speaks the episode
// word instead.
func TestCR2P7ContainedActualNeverWearsCrossWindowWarning(t *testing.T) {
	node := cr2P7Node(16.433, 15.565, 13762.991547, 13763.008274)
	model := buildRuntimeTraceProjTreeModel(cr2P7Projection(node), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "⚠实际16.433") {
		t.Fatalf("an in-window actual interval must never claim 跨出分析窗:\n%s", fence)
	}
	if !strings.Contains(fence, "实际16.433ms(超出发生段,窗内)") {
		t.Fatalf("the in-window overshoot must speak the episode-scope word:\n%s", fence)
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	var cells []string
	for _, item := range rows {
		cells = append(cells, strings.Join(item.Cells, " | "))
	}
	table := strings.Join(cells, "\n")
	if strings.Contains(table, "16.433ms ⚠") {
		t.Fatalf("the table cell must mirror the typed verdict (no ⚠):\n%s", table)
	}
	if !strings.Contains(table, "16.433ms(超出发生段,窗内)") {
		t.Fatalf("the table cell must speak the episode word:\n%s", table)
	}
}

// The proven-crossing shape keeps its ⚠ (the legend promise survives the gate).
func TestCR2P7CrossingActualKeepsWarning(t *testing.T) {
	node := cr2P7Node(16.433, 15.565, 13762.991547, 13763.030000) // ends past the window
	model := buildRuntimeTraceProjTreeModel(cr2P7Projection(node), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⚠实际16.433ms") {
		t.Fatalf("a proven analysis-window crossing must keep ⚠实际:\n%s", fence)
	}
}

// 宁漏勿假: an interval-less actual overshoot states the dual-basis fact
// without any window-scope claim.
func TestCR2P7IntervalLessActualClaimsNoScope(t *testing.T) {
	node := cr2P7Node(16.433, 15.565, 0, 0)
	model := buildRuntimeTraceProjTreeModel(cr2P7Projection(node), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "⚠实际") || strings.Contains(fence, "超出发生段") {
		t.Fatalf("no interval → no scope claim:\n%s", fence)
	}
	if !strings.Contains(fence, "实际16.433ms(区间未发布)") {
		t.Fatalf("the interval-less overshoot must still disclose the dual basis:\n%s", fence)
	}
}

// F-5 形 pin: a ×N merged row's actual is the merge seed's SINGLE member —
// both faces disclose the caliber, so 「实际 < 行值」 stops reading as a
// paradox. (Merged shape: donor caliber present, member max above the actual.)
func TestCR2P7MergedRowActualDisclosesSingleMember(t *testing.T) {
	node := cr2P7Node(6.936, 17.442, 34579.520, 34579.600) // crosses the window end below
	node.MergedCount = 3
	node.MergedMinMS = 4.426
	node.MergedMaxMS = 6.768
	node.MergedActualDonorCumulativeMS = 6.248
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"app-1", "target-9"},
		WindowStartTs: 34579.473,
		WindowEndTs:   34579.588,
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⚠实际6.936ms(单次成员)") {
		t.Fatalf("the merged row's actual must disclose the single-member caliber:\n%s", fence)
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	var cells []string
	for _, item := range rows {
		cells = append(cells, strings.Join(item.Cells, " | "))
	}
	table := strings.Join(cells, "\n")
	if !strings.Contains(table, "6.936ms ⚠(单次成员)") {
		t.Fatalf("the table cell must carry the same caliber word:\n%s", table)
	}
}
