package tracequery

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestB1630cFrequencyLimitSummarySeparatesInventoryFromRepresentative(t *testing.T) {
	trace := strings.Join([]string{
		`idle-0 (0) [000] .... 0.900000: cpu_frequency_limits: min=100000 max=900000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.000000: cpu_frequency_limits: min=0 max=0 cpu_id=0`,
		`idle-0 (0) [000] .... 1.100000: cpu_frequency_limits: min=400000 max=2000000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.200000: cpu_frequency_limits: min=300000 max=1500000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.300000: cpu_frequency_limits: min=800000 max=1500000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.400000: cpu_frequency_limits: min=0 max=0 cpu_id=0`,
		`idle-0 (0) [001] .... 1.500000: cpu_frequency_limits: min=300000 max=1700000 cpu_id=1`,
		`idle-0 (0) [000] .... 2.100000: cpu_frequency_limits: min=100000 max=500000 cpu_id=0`,
	}, "\n") + "\n"
	for _, tc := range []struct {
		name  string
		trace string
		query Query
		want  []CPUFrequencyLimit
	}{
		{"mixed_max_and_zero", trace, Query{TimeStart: 1, TimeEnd: 2}, []CPUFrequencyLimit{
			{CPU: 0, MinFrequency: 300000, MaxFrequency: 1500000, Count: 5, Line: 4, Ts: 1.2},
			{CPU: 1, MinFrequency: 300000, MaxFrequency: 1700000, Count: 1, Line: 7, Ts: 1.5},
		}},
		{"same_max_keeps_first_atomic_row", strings.ReplaceAll(strings.ReplaceAll(trace, "min=300000 max=1500000", "min=600000 max=1500000"), "min=800000 max=1500000", "min=200000 max=1500000"), Query{TimeStart: 1, TimeEnd: 2}, []CPUFrequencyLimit{
			{CPU: 0, MinFrequency: 600000, MaxFrequency: 1500000, Count: 5, Line: 4, Ts: 1.2},
			{CPU: 1, MinFrequency: 300000, MaxFrequency: 1700000, Count: 1, Line: 7, Ts: 1.5},
		}},
		{"time_window_excludes_previous_representative", trace, Query{TimeStart: 1.3, TimeEnd: 1.5}, []CPUFrequencyLimit{
			{CPU: 0, MinFrequency: 800000, MaxFrequency: 1500000, Count: 2, Line: 5, Ts: 1.3},
			{CPU: 1, MinFrequency: 300000, MaxFrequency: 1700000, Count: 1, Line: 7, Ts: 1.5},
		}},
		{"line_window_excludes_zero_and_other_cpu", trace, Query{LineStart: 3, LineEnd: 5}, []CPUFrequencyLimit{
			{CPU: 0, MinFrequency: 300000, MaxFrequency: 1500000, Count: 3, Line: 4, Ts: 1.2},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "limits.systrace", tc.trace)
			for _, view := range []string{"window_stats", "root_cause_rank"} {
				t.Run(view, func(t *testing.T) {
					q := tc.query
					q.View = view
					result := Run(idx, q)
					if result.WindowStats == nil || !reflect.DeepEqual(result.WindowStats.CPUFrequencyLimits, tc.want) {
						t.Fatalf("strict query inventory/atomic representative changed: want=%+v stats=%+v", tc.want, result.WindowStats)
					}
					for _, want := range tc.want {
						if view == "window_stats" {
							var matches []EvidenceFact
							for _, fact := range result.EvidencePack {
								if fact.Predicate == "cpu_frequency_limit" && fact.Subject == fmt.Sprintf("cpu=%d", want.CPU) {
									matches = append(matches, fact)
								}
							}
							if len(matches) != 1 {
								t.Fatalf("expected exact CPU evidence row: cpu=%d matches=%+v", want.CPU, matches)
							}
							fact := matches[0]
							if fact.LineStart != want.Line || fact.LineEnd != want.Line || fact.StartTs != want.Ts || fact.EndTs != want.Ts || fact.Confidence != 0.68 {
								t.Fatalf("representative evidence coordinates/numbers changed: %+v", fact)
							}
							assertB1630cFrequencyLimitSummary(t, fact.Summary, want)
						} else {
							if result.RootCauseRank == nil {
								t.Fatal("rank was not produced")
							}
							var matches []RootCauseRankItem
							for _, row := range result.RootCauseRank.Items {
								if row.Type == "cpu_frequency_limit" && row.LineStart == want.Line {
									matches = append(matches, row)
								}
							}
							if len(matches) != 1 {
								t.Fatalf("expected exact limit rank row: line=%d rows=%+v", want.Line, result.RootCauseRank.Items)
							}
							row := matches[0]
							if row.LineEnd != want.Line || row.Confidence != 0.58 || row.Source != "window_stats" || row.Thread != (ThreadRef{}) {
								t.Fatalf("rank identity/metadata changed: %+v", row)
							}
							windowMs := (result.RootCauseRank.Window.EndTs - result.RootCauseRank.Window.StartTs) * 1000
							if math.Abs(row.ImpactMs-windowMs) > 1e-8 || math.Abs(row.ProjectedImpactMs-windowMs) > 1e-8 || math.Abs(row.CumulativeImpactMs-windowMs) > 1e-8 || row.Score != 0 {
								t.Fatalf("existing rank value/weight changed: window_ms=%g impact=%g projected=%g cumulative=%g score=%g", windowMs, row.ImpactMs, row.ProjectedImpactMs, row.CumulativeImpactMs, row.Score)
							}
							assertB1630cFrequencyLimitSummary(t, row.Summary, want)
						}
					}
				})
			}
		})
	}
}

func assertB1630cFrequencyLimitSummary(t *testing.T, summary string, limit CPUFrequencyLimit) {
	t.Helper()
	for _, required := range []string{
		fmt.Sprintf("cpu=%d", limit.CPU),
		fmt.Sprintf("%d valid frequency-limit record(s)", limit.Count),
		"selected query scope", "strictest positive maximum representative",
		fmt.Sprintf("min=%dkHz max=%dkHz", limit.MinFrequency, limit.MaxFrequency),
		fmt.Sprintf("line=%d ts=%.6f", limit.Line, limit.Ts),
		"not the repetition count of this min/max pair",
	} {
		if !strings.Contains(summary, required) {
			t.Errorf("missing %q in frequency inventory disclosure: %s", required, summary)
		}
	}
	if strings.Contains(summary, "appeared") {
		t.Errorf("representative pair still claims event multiplicity: %s", summary)
	}
}

func TestB1630cFrequencyLimitZeroInventoryDoesNotClaimPositiveCeiling(t *testing.T) {
	idx := buildTraceIndex(t, "zero-limits.systrace", strings.Join([]string{
		`idle-0 (0) [002] .... 0.900000: cpu_frequency_limits: min=100000 max=1800000 cpu_id=2`,
		`idle-0 (0) [002] .... 1.100000: cpu_frequency_limits: min=0 max=0 cpu_id=2`,
		`idle-0 (0) [002] .... 1.200000: cpu_frequency_limits: min=0 max=0 cpu_id=2`,
		`idle-0 (0) [002] .... 2.100000: cpu_frequency_limits: min=100000 max=1800000 cpu_id=2`,
	}, "\n")+"\n")
	for _, tc := range []struct {
		name string
		q    Query
		want []CPUFrequencyLimit
	}{
		{"all_zero", Query{TimeStart: 1, TimeEnd: 2}, []CPUFrequencyLimit{{CPU: 2, Count: 2, Line: 2, Ts: 1.1}}},
		{"no_rows_is_not_zero_measurement", Query{TimeStart: 1.3, TimeEnd: 2}, []CPUFrequencyLimit{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, view := range []string{"window_stats", "root_cause_rank"} {
				q := tc.q
				q.View = view
				result := Run(idx, q)
				if result.WindowStats == nil || !reflect.DeepEqual(result.WindowStats.CPUFrequencyLimits, tc.want) {
					t.Fatalf("zero/empty inventory changed: want=%+v stats=%+v", tc.want, result.WindowStats)
				}
				if result.RootCauseRank != nil {
					for _, row := range result.RootCauseRank.Items {
						if row.Type == "cpu_frequency_limit" {
							t.Fatalf("zero inventory minted a positive limit rank: %+v", row)
						}
					}
				}
				if view != "window_stats" {
					continue
				}
				var facts []EvidenceFact
				for _, fact := range result.EvidencePack {
					if fact.Predicate == "cpu_frequency_limit" {
						facts = append(facts, fact)
					}
				}
				if len(facts) != len(tc.want) {
					t.Fatalf("zero inventory absent or fabricated: %+v", facts)
				}
				for _, fact := range facts {
					for _, required := range []string{"2 valid frequency-limit record(s)", "no positive maximum ceiling in these records", "min=0kHz max=0kHz", "line=2 ts=1.100000"} {
						if !strings.Contains(fact.Summary, required) {
							t.Errorf("zero inventory missing %q: %s", required, fact.Summary)
						}
					}
					if strings.Contains(fact.Summary, "strictest positive maximum representative") || strings.Contains(fact.Summary, "appeared") {
						t.Errorf("zero inventory promoted to a repeated/positive ceiling: %s", fact.Summary)
					}
				}
			}
		})
	}
}
