package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceAnswerDecisionDirectionSectionsSharesRenderedSubtotalPopulation(t *testing.T) {
	inside := true
	seat := func(rank int, subject string, value, start, end float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: "seat-" + subject, Subject: subject,
			Object: "priority_inversion_candidate", TypeToken: "priority_inversion_candidate",
			Rank: rank, EffectiveImpactMS: value, EffectiveImpactPublished: true,
			FixDirection: "lock_priority", ChainRelevance: "on_chain",
			WithinRequestedWindow: &inside, StartTs: start, EndTs: end,
			RankBoardTarget: "target-100", RankBoardParamsFingerprint: "board-a",
			RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
		}
	}
	seats := []types.TraceCausalProjectionNode{
		seat(1, "worker-a", 7.405, 10.010, 10.020),
		seat(2, "worker-b", 4.710, 10.030, 10.040),
	}
	projection := types.TraceCausalProjection{
		WindowStartTs: 10, WindowEndTs: 10.1, WakeupPath: []string{"worker-a", "target-100"},
		RankedSeats: seats, OnChainCauses: seats,
	}
	sections := TraceAnswerDecisionDirectionSections(projection)
	if len(sections) != 1 {
		t.Fatalf("expected one exact rendered direction section, got %+v", sections)
	}
	section := sections[0]
	if section.Direction != "lock_priority" || len(section.Members) != 2 ||
		section.Arithmetic != types.TraceAnswerDirectionArithmeticSubtotal || section.SubtotalMS != 12.115 {
		t.Fatalf("rendered direction subtotal authority drifted: %+v", section)
	}

	// The same population with measured overlap must step down rather than
	// inheriting the previous exact subtotal from its shared direction label.
	seats[1].StartTs, seats[1].EndTs = 10.015, 10.025
	projection.RankedSeats, projection.OnChainCauses = seats, seats
	sections = TraceAnswerDecisionDirectionSections(projection)
	if len(sections) != 1 || sections[0].Arithmetic != types.TraceAnswerDirectionArithmeticOverlap || sections[0].SubtotalMS != 0 {
		t.Fatalf("overlapping section must forbid the subtotal: %+v", sections)
	}
}
