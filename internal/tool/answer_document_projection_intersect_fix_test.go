package tool

// answer_document_projection_intersect_fix_test.go — INTERSECT-FIX render
// live pin (§29.143 INTERSECT-REG 归因, 2026-07-19), re-based by INTERFLOOR-1
// (user ruling §29.150③, 2026-07-19: 极小交集判噪音,相对形地板) to carry
// BOTH arms on the FULL production chain (BuildIndex → typed observations →
// projection compile → zh render):
//
//   - donghu 17267 flagship board (this harness's query shape: wakeup_chain +
//     root_cause_rank, MinDurationMs 0.5, Limit 12): BIO-WAKE-1 recovers the
//     complete strict completion→issuer-wake request census. IO-CAL-1 prices
//     that family on issuer switch-out→wake response time, which is disjoint
//     from the same thread's running seat. The former request-residence
//     1.440ms overlap and every related sentence/chip/footnote must disappear;
//     the two published values remain untouched.
//   - tieba 61839 W-A board: the nested full-containment pair (0.705ms = 100%
//     of the smaller seat) sits far above the floor and must KEEP the full
//     mutual sentence face, and the ∩ legend entry must teach the floor
//     (significant-keep arm — an over-eager floor or a lost legend clause
//     turns this red).
//
// 突变职责: stripping the family_member_segment_intervals basis arm still
// reproduces INTERSECT-REG (the engine live pin loses the undisclosed record
// and the kept 0.116 pair); flipping the de-minimis comparison resurrects the
// 0.230 sentences here (negative arm red) and kills the 0.705 keep arm.

import (
	"os"
	"strings"
	"testing"
)

func TestIntersectFixMutualClauseDonghuFlagship(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticDonghuTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	md := elimSemanticRealMarkdown(t, elimSemanticDonghuTrace, 17267, 13762.791708, 13763.024898)

	// INTERFLOOR-1 negative arm: the old boundary-riding pairs remain silent.
	for _, banned := range []string{"同段重叠 0.230ms", "同段重叠 0.043ms"} {
		if strings.Contains(md, banned) {
			t.Fatalf("de-minimis 降道: %q must not render on the flagship board:\n%s", banned, md)
		}
	}
	// IO-CAL-1: a task cannot be response-blocked and running at once. The
	// old 1.440ms pair came from the device request-residence ruler.
	if got := strings.Count(md, "同段重叠 1.440ms"); got != 0 ||
		strings.Contains(md, "∩ 重叠对    [E1]∩[E5] 1.440ms") {
		t.Fatalf("request-residence overlap must not survive on the response-impact face, got=%d:\n%s", got, md)
	}
	// 值面零动: overlap disclosure does not subtract or rewrite either seat.
	if !strings.Contains(md, "58.320ms") || !strings.Contains(md, "11.141ms") {
		t.Fatalf("published values must stay untouched by the disclosure gate:\n%s", md)
	}
}

func TestINTERFLOOR1KeepMutualClauseTieba61839(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticTiebaTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	md := elimSemanticRealMarkdown(t, elimSemanticTiebaTrace, 61839, 34579.470, 34579.520)

	// Significant-keep arm: the nested full-containment pair keeps its full
	// mutual sentence on both rows (both-or-neither).
	if got := strings.Count(md, "同段重叠 0.705ms"); got < 2 {
		t.Fatalf("keep 正臂: the 0.705ms mutual clause must render on both rows, got %d occurrence(s):\n%s", got, md)
	}
	if !strings.Contains(md, "(修向 频率与热治理)同段重叠 0.705ms") ||
		!strings.Contains(md, "(修向 调度供给)同段重叠 0.705ms") {
		t.Fatalf("keep 正臂: both partner 修向 words must render at the clause sites:\n%s", md)
	}
	// 件4 图例随动: the ∩ sentence legend teaches the relative floor
	// (低于显著阈的重叠不发句,记号道可审计).
	if !strings.Contains(md, "低于显著性阈值的极小重叠不展开互指句，但仍保留在审计明细中") {
		t.Fatalf("the ∩ legend must teach the de-minimis floor:\n%s", md)
	}
}
