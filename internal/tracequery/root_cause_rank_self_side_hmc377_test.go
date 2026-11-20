package tracequery

import "testing"

// HMC377 fix round 3, item 5: every cap-displaced valued self seat keeps a
// published form. The self side lane was sized for two seats per family, but
// the D/IO family mints one seat per proven wait-object cause plus the
// unproven remainder; seven displaced self seats (the Douyin reboot5 shape:
// 0.628 / 0.281 / 0.273 / 0.149 / 0.116 / 0.078 / 0.073 ms) lost the last one
// silently, and the merged family line then counted one occurrence fewer.
func TestHMC377EveryDisplacedSelfSeatIsPublished(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	var items []RootCauseRankItem
	for i := 0; i < 3; i++ {
		items = append(items, RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{Comm: "dep", PID: 300 + i},
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ImpactMs: 20 - float64(i), RunnableMs: 20 - float64(i)})
	}
	var self []RootCauseRankItem
	for i, ms := range []float64{0.628, 0.281, 0.273, 0.149, 0.116, 0.078, 0.073} {
		self = append(self, RootCauseRankItem{Type: "d_state_or_io_wait", Thread: target, SubjectIsAnalysisTarget: true,
			ChainRelevance: "on_chain", DStateMs: ms, ImpactMs: ms, LineStart: 10 + i, LineEnd: 10 + i})
	}
	items = append(items, self...)
	got, capDead, selfPublished, _, _, _, _ := truncateRootCauseRankCandidatesAndSideRows(items, 3)
	if selfPublished != len(self) || len(capDead) != 0 {
		t.Fatalf("every displaced self seat is published: selfPublished=%d capDead=%d", selfPublished, len(capDead))
	}
	published := map[int]bool{}
	for _, it := range got {
		if it.SubjectIsAnalysisTarget {
			published[it.LineStart] = true
		}
	}
	for _, s := range self {
		if !published[s.LineStart] {
			t.Fatalf("self D seat %.3f ms vanished from the published board", s.DStateMs)
		}
	}
}
