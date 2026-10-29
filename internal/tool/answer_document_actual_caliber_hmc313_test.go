package tool

import (
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC313ActualCaliberExtentIsNotDuration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, end float64
		known      bool
	}{
		{"zero_start", 0, 2, true},
		{"gapped_original_segments", 3, 8, true},
		{"absent", 0, 0, false},
		{"reversed", 8, 3, false},
		{"negative", -1, 3, false},
		{"nan", math.NaN(), 3, false},
		{"infinite", 0, math.Inf(1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, zh := range []bool{true, false} {
				node := types.TraceCausalProjectionNode{ActualImpactMS: 1, ActualTotalMS: 6,
					ActualWindowStartTs: tc.start, ActualWindowEndTs: tc.end,
					StartTs: 20, EndTs: 30, QueryWindowStartTs: 15, QueryWindowEndTs: 40,
					RunningMS: 999, RunnableMS: 888, SleepMS: 777}
				got := runtimeTraceProjActualCaliberText(node, zh)
				if !strings.Contains(got, "1.000ms") || !strings.Contains(got, "6.000ms") {
					t.Fatalf("original values lost: %s", got)
				}
				for _, foreign := range []string{"999", "888", "777", "20.000000", "30.000000", "15.000000", "40.000000", "2000.000ms", "5000.000ms"} {
					if strings.Contains(got, foreign) {
						t.Fatalf("projected components, foreign window or envelope duration leaked: %s", got)
					}
				}
				if tc.known {
					if !strings.Contains(got, "..") || !(strings.Contains(got, "不证明连续执行") || strings.Contains(got, "does not establish continuous execution")) {
						t.Fatalf("known extent lost boundary: %s", got)
					}
				} else if strings.Contains(got, "..") || strings.Contains(got, "NaN") || strings.Contains(got, "Inf") {
					t.Fatalf("unknown original extent invented: %s", got)
				}
			}
		})
	}
}
