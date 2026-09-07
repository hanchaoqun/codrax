package types

import (
	"encoding/json"
	"testing"
)

func TestDirectionSectionArithmeticRequiresOneKnownQueryDomain(t *testing.T) {
	for _, scenario := range []string{"same", "params", "target", "window", "missing_params", "all_missing_params", "missing_target", "missing_window", "fallback_window", "zero_window_start", "mixed_window_precision", "overlap", "merged", "multi_board"} {
		t.Run(scenario, func(t *testing.T) {
			members := []TraceCausalProjectionNode{
				{Subject: "first", RankBoardTarget: "app-100", RankBoardParamsFingerprint: "depth-a", RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 11, StartTs: 10.01, EndTs: 10.02, EffectiveImpactMS: 7.405},
				{Subject: "second", RankBoardTarget: "app-100", RankBoardParamsFingerprint: "depth-a", RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 11, StartTs: 10.03, EndTs: 10.04, EffectiveImpactMS: 4.710},
			}
			want, value := TraceAnswerDirectionArithmeticNone, 0.0
			multi := false
			switch scenario {
			case "same":
				want, value = TraceAnswerDirectionArithmeticSubtotal, 12.115
			case "fallback_window":
				members[1].RankQueryWindowStartTs, members[1].RankQueryWindowEndTs = 0, 0
				members[1].QueryWindowStartTs, members[1].QueryWindowEndTs = 10, 11
				want, value = TraceAnswerDirectionArithmeticSubtotal, 12.115
			case "zero_window_start":
				members[0].RankQueryWindowStartTs, members[1].RankQueryWindowStartTs = 0, 0
				want, value = TraceAnswerDirectionArithmeticSubtotal, 12.115
			case "params":
				members[1].RankBoardParamsFingerprint = "depth-b"
			case "target":
				members[1].RankBoardTarget = "app-200"
			case "window":
				members[1].RankQueryWindowEndTs = 12
			case "mixed_window_precision":
				members[1].RankQueryWindowEndTs += 0.0000001
			case "missing_params":
				members[1].RankBoardParamsFingerprint = ""
			case "all_missing_params":
				members[0].RankBoardParamsFingerprint, members[1].RankBoardParamsFingerprint = "", ""
			case "missing_target":
				members[1].RankBoardTarget = ""
			case "missing_window":
				members[1].RankQueryWindowStartTs, members[1].RankQueryWindowEndTs = 0, 0
			case "overlap":
				members[1].StartTs, members[1].EndTs = 10.015, 10.025
				want = TraceAnswerDirectionArithmeticOverlap
			case "merged":
				members[1].MergedCount = 2
			case "multi_board":
				multi = true
			}
			before, _ := json.Marshal(members)
			got, subtotal := TraceAnswerDirectionSectionArithmetic("lock_priority", members, multi, true)
			if got != want || subtotal != value {
				t.Fatalf("scope=%s arithmetic=%s subtotal=%.3f; want %s %.3f", scenario, got, subtotal, want, value)
			}
			after, _ := json.Marshal(members)
			if string(before) != string(after) {
				t.Fatal("arithmetic scope check changed source rows")
			}
		})
	}
}
