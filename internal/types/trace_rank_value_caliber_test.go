package types

import "testing"

func TestTraceRankValueCaliberIsDisplayOnlyClosedMarker(t *testing.T) {
	for _, tc := range []struct{ name, predicate, caliber, want string }{
		{"native", "root_cause_background", TraceRankValueCaliberNativeDuration, TraceRankValueCaliberNativeDuration},
		{"legacy", "root_cause_background", "", ""},
		{"unknown", "root_cause_background", "measured_by_description", ""},
		{"not_rank", "io_latency", TraceRankValueCaliberNativeDuration, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := ObservationRecord{Producer: "trace_query", Predicate: tc.predicate, Subject: "backup-900", Object: "io_latency", Value: "47.000", Unit: "ms",
				RichNotes: []string{"chain_relevance=background", "effective_impact_ms=0.000", "impact_ms=47.000", "rank_value_caliber=" + tc.caliber}}
			n := traceCausalProjectionNodeFromRecord("root_cause_context", r)
			if n.RankValueCaliber != tc.want {
				t.Fatalf("caliber=%q, want %q", n.RankValueCaliber, tc.want)
			}
			if n.ImpactMS != 47 || n.EffectiveImpactMS != 0 || n.ChainRelevance != "background" || n.Rank != 0 {
				t.Fatalf("display marker changed value/causal authority: %+v", n)
			}
		})
	}
}
