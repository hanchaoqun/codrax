package tracequery

import "testing"

// HMC377 E1E6-R7 (test gap): a span of the analysed thread whose every instant
// is a wait of its dependency chain has no own work; the own-work family is
// dropped and only the wait family of the same span publishes.
func TestHMC377OwnSpanThatIsAllWaitPublishesOnlyTheWaitFamily(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	span := rcmSpan(target, "VerifyClass com.example.Blocked", 5.100, 5.130, 10, 20)
	chain := hmc377Chain(target,
		ChainNode{Thread: target, Window: TimeWindow{StartTs: 5.095, EndTs: 5.135}, Dominant: StateSSleep})
	stats := WindowStats{Window: TimeWindow{StartTs: 5.0, EndTs: 6.0}, TraceSpans: []TraceSpanSummary{span}}
	families := SemanticSpanFamiliesForStats(chain, &stats)
	if len(families) != 1 || families[0].OnChainBasis != "" || !hmc377Near(families[0].ProjectedImpactMs, 30.0) {
		t.Fatalf("an all-wait span publishes only its 30 ms wait family: %+v", families)
	}
}
