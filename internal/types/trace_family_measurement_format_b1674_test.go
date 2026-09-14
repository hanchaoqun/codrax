package types

import (
	"math"
	"strings"
	"testing"
)

func TestB1674FamilyMeasurementFormatterKeepsRulersAndMissingValues(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		for _, tc := range []struct {
			fold, en, zh string
			timed        bool
		}{
			{"sum_disjoint", "sum of disjoint intervals", "互斥区间求和", true},
			{"interval_union", "interval union", "区间去重并集", true},
			{"max_overlap_fallback", "maximum-record fallback", "取记录最大值回退", false},
			{"count_sum", "count-equivalent aggregation, not wall-clock time", "计数当量聚合，不是墙钟时长", false},
			{"private_future_fold", "aggregation basis not established", "聚合口径未明确", false},
			{"", "aggregation basis not established", "聚合口径未明确", false},
		} {
			t.Run(lang+"/"+tc.fold, func(t *testing.T) {
				got := FormatTraceFamilyMeasurement(47, .782, tc.fold, lang)
				want := tc.en
				if lang == "zh" {
					want = tc.zh
				}
				if !strings.Contains(got, want) || !strings.Contains(got, "47") || !strings.Contains(got, "0.782") {
					t.Fatalf("record count/max/ruler lost: %s", got)
				}
				if strings.Contains(got, "private_future_fold") {
					t.Fatal("unknown enum emitted")
				}
				if !tc.timed && (strings.Contains(got, "0.782 ms") || strings.Contains(got, "0.782 毫秒")) {
					t.Fatal("non-interval or unknown measure gained duration unit")
				}
				for _, v := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
					got = FormatTraceFamilyMeasurement(47, v, tc.fold, lang)
					if strings.Contains(got, "record maximum ") || strings.Contains(got, "记录最大值 ") {
						t.Fatalf("unpublished/invalid maximum fabricated: %s", got)
					}
				}
			})
		}
		for _, n := range []int{-1, 0, 1} {
			if got := FormatTraceFamilyMeasurement(n, .782, "sum_disjoint", lang); got != "" {
				t.Fatalf("no family published: %q", got)
			}
		}
	}
}
