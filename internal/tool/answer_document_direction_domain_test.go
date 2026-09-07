package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDirectionDomainPublishedSectionAndAppendixShareQueryPremise(t *testing.T) {
	for _, scenario := range []string{"same", "different_params", "missing_params"} {
		t.Run(scenario, func(t *testing.T) {
			inside := true
			seats := []types.TraceCausalProjectionNode{
				{EvidenceID: "first", Subject: "worker-a", Object: "priority_inversion_candidate", TypeToken: "priority_inversion_candidate", Rank: 1, EffectiveImpactMS: 7.405, EffectiveImpactPublished: true, FixDirection: "lock_priority", ChainRelevance: "on_chain", WithinRequestedWindow: &inside, StartTs: 10.01, EndTs: 10.02, RankBoardTarget: "target-100", RankBoardParamsFingerprint: "depth-a", RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1},
				{EvidenceID: "second", Subject: "worker-b", Object: "priority_inversion_candidate", TypeToken: "priority_inversion_candidate", Rank: 2, EffectiveImpactMS: 4.710, EffectiveImpactPublished: true, FixDirection: "lock_priority", ChainRelevance: "on_chain", WithinRequestedWindow: &inside, StartTs: 10.03, EndTs: 10.04, RankBoardTarget: "target-100", RankBoardParamsFingerprint: "depth-a", RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1},
			}
			if scenario == "different_params" {
				seats[1].RankBoardParamsFingerprint = "depth-b"
			}
			if scenario == "missing_params" {
				seats[1].RankBoardParamsFingerprint = ""
			}
			projection := types.TraceCausalProjection{ArtifactPath: "/captures/test.trace", ArtifactLabel: "test.trace", WindowStartTs: 10, WindowEndTs: 10.1, WakeupPath: []string{"worker-a", "target-100"}, RankedSeats: seats, OnChainCauses: seats, RootCauseFamilyObserved: true}
			before, _ := json.Marshal(projection)
			sections := TraceAnswerDecisionDirectionSections(projection)
			if len(sections) != 1 || len(sections[0].Members) != 2 {
				t.Fatalf("scope checking erased individual members: %+v", sections)
			}
			want := types.TraceAnswerDirectionArithmeticNone
			if scenario == "same" {
				want = types.TraceAnswerDirectionArithmeticSubtotal
			}
			if sections[0].Arithmetic != want {
				t.Fatalf("different/unknown queries acquired exact addition: %+v", sections[0])
			}
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
			for _, zh := range []bool{false, true} {
				fence := runtimeTraceProjElimOverviewFence(projection, model, zh)
				if strings.Contains(fence, "12.115") != (scenario == "same") {
					t.Fatalf("appendix disagrees with published arithmetic, scope=%s zh=%t:\n%s", scenario, zh, fence)
				}
				for _, value := range []string{"7.405", "4.710"} {
					if !strings.Contains(fence, value) {
						t.Fatalf("scope check hid original amount %s:\n%s", value, fence)
					}
				}
			}
			after, _ := json.Marshal(projection)
			if string(before) != string(after) {
				t.Fatal("section/appendix calculation mutated projection")
			}
		})
	}
}
