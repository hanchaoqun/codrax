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

func TestTraceAnswerDecisionDirectionSectionsKeepsExpandedBoardOutOfPrincipalSubtotal(t *testing.T) {
	inside := true
	exact := types.TraceCausalProjectionNode{
		EvidenceID: "exact", Subject: "worker-200", Object: "priority_inversion_candidate",
		TypeToken: "priority_inversion_candidate", Rank: 1,
		EffectiveImpactMS: 8.300, EffectiveImpactPublished: true,
		FixDirection: "lock_priority", ChainRelevance: "on_chain",
		WithinRequestedWindow: &inside, StartTs: 1.0012, EndTs: 1.0095,
		RankBoardTarget: "app-100", RankBoardParamsFingerprint: "board-a",
		RankQueryWindowStartTs: 1, RankQueryWindowEndTs: 1.010,
	}
	expanded := exact
	expanded.EvidenceID = "expanded"
	expanded.Subject = "app-100"
	expanded.Object = "priority_inversion_runnable_wait"
	expanded.TypeToken = expanded.Object
	expanded.Rank = 2
	expanded.EffectiveImpactMS = 0.020
	expanded.StartTs, expanded.EndTs = 1.010, 1.010020
	expanded.RankQueryWindowEndTs = 1.011
	projection := types.TraceCausalProjection{
		WindowStartTs: 1, WindowEndTs: 1.010,
		WakeupPath:  []string{"worker-200", "app-100"},
		RankedSeats: []types.TraceCausalProjectionNode{exact, expanded},
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject: "app-100", WindowStartTs: 1, WindowEndTs: 1.010, TotalMS: 10,
		},
		OnChainCauses: []types.TraceCausalProjectionNode{
			exact, expanded,
		},
	}

	sections := TraceAnswerDecisionDirectionSections(projection)
	if len(sections) != 1 || len(sections[0].Members) != 1 ||
		sections[0].Leader.EvidenceID != "exact" || sections[0].SubtotalMS != 0 {
		t.Fatalf("expanded-window seat entered selected-window direction arithmetic: %+v", sections)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for _, row := range rows {
			if row.Node.EvidenceID == "expanded" {
				t.Fatalf("expanded-window seat retained a principal tree lane: %+v", row.Node)
			}
		}
	}
	foundBackground := false
	for _, row := range model.Background {
		if row.Node.EvidenceID == "expanded" {
			foundBackground = true
			if row.Node.Rank != 0 || row.Node.ChainRelevance != "background" {
				t.Fatalf("expanded-window context copy retained principal authority: %+v", row.Node)
			}
		}
	}
	if !foundBackground {
		t.Fatal("expanded-window evidence disappeared instead of moving to context")
	}
}
