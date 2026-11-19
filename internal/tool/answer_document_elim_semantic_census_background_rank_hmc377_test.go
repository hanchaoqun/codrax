package tool

import (
	"strings"
	"testing"
)

// HMC377 E1E6-R3: a seatless adjacent semantic family that carries only a
// background-board position (Douyin bindApplication: PPS-DC-Cal-pool-13914
// class verification, 8 members, 571.120 ms) is shown nowhere on the ◎
// board, so the semantic census footnote must count it — the former
// rank-item arm treated background_rank>0 as already seated and the
// footnote said 「◇ 1 行(…最大 2.435ms)」.
func TestHMC377SemanticCensusCountsABackgroundRankedFamilyTheBoardLeavesOut(t *testing.T) {
	projection := elimBoardProjection()
	family := elimSemanticSeatlessNode("E-pps", "PPS-DC-Cal-pool-13914", "class_verification", "adjacent", 571.120, 400)
	family.BackgroundRank = 1
	projection.SemanticSpans = append(projection.SemanticSpans, family,
		elimSemanticSeatlessNode("E-tex", "RenderThread-1", "texture_upload", "adjacent", 2.435, 410))
	model, fence := elimRenderOverview(t, projection, true)
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "571.120ms") {
			t.Fatalf("fixture: the background-ranked family is not a board member here:\n%s", line)
		}
	}
	if !strings.Contains(fence, "◇ 2 行(类校验1、纹理上传1,最大 571.120ms [E") {
		t.Fatalf("the census must count the family the board leaves out:\n%s", fence)
	}
	_ = model
}
