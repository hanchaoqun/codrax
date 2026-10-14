package tracequery

import (
	"slices"
	"strings"
	"testing"
)

func TestPerfQualityAggregateCompactDoesNotSelectFirstHMC240(t *testing.T) {
	for _, tc := range []struct {
		values []PerfValueCount
		want   string
	}{
		{nil, ""}, {[]PerfValueCount{{Value: "unknown"}}, "unknown"},
		{[]PerfValueCount{{Value: "symbolized"}}, "symbolized"},
		{[]PerfValueCount{{Value: "unsymbolized"}, {Value: "symbolized"}}, "mixed"},
		{[]PerfValueCount{{Value: "unknown"}, {Value: "symbolized"}}, "mixed"},
	} {
		for i := 0; i < 2; i++ {
			if got := perfQualityTopValue(tc.values); got != tc.want {
				t.Fatalf("got=%s want=%s", got, tc.want)
			}
			q := &PerfQualitySummary{SymbolizationStatuses: tc.values, CallchainStatuses: tc.values, Sources: tc.values, SampleKinds: tc.values, Clocks: tc.values}
			if tc.want != "" && !strings.Contains(perfQualitySummaryCompact(q), "symbolization="+tc.want) {
				t.Fatal("native compact evidence still picks first status")
			}
			slices.Reverse(tc.values)
		}
	}
}
