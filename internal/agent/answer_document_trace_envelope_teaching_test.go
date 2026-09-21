package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceEnvelopeTeachingAtBothPreFinalTokenSurfaces(t *testing.T) {
	inside := true
	for _, token := range []string{"priority_inversion_candidate", "runnable_wait", "d_state_or_io_wait"} {
		t.Run(token, func(t *testing.T) {
			seats := []types.TraceCausalProjectionNode{
				{EvidenceID: "a", Rank: 1, Subject: "alpha-20", EffectiveImpactMS: 7.405, StartTs: 10.01, EndTs: 10.09},
				{EvidenceID: "b", Rank: 2, Subject: "beta-30", EffectiveImpactMS: 4.710, StartTs: 10.03, EndTs: 10.11},
			}
			for i := range seats {
				n := &seats[i]
				n.Object, n.TypeToken, n.FixDirection = token, token, "lock_priority"
				n.EffectiveImpactPublished, n.WithinRequestedWindow = true, &inside
				n.ChainRelevance = "on_chain"
				n.RankBoardTarget, n.RankBoardParamsFingerprint = "target-100", "board-a"
				n.RankQueryWindowStartTs, n.RankQueryWindowEndTs = 10, 10.2
				if token == "priority_inversion_candidate" {
					n.GatedRunnableMS, n.GatedRunningDeficitMS = n.EffectiveImpactMS/2, n.EffectiveImpactMS/2
				}
			}
			set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
				ArtifactLabel: "capture.ftrace", WindowStartTs: 10, WindowEndTs: 10.2,
				WakeupPath: []string{"alpha-20", "target-100"}, RankedSeats: seats, OnChainCauses: seats,
			}}}
			before, _ := json.Marshal(set)
			for name, prompt := range map[string]string{
				"decision_handoff": renderAnswerDocTraceDecisionHandoffSet(set, runtimeTraceGuidanceView{}),
				"final_boundary":   renderTraceFinalCompactAuthorityLedger(set),
			} {
				if !strings.Contains(prompt, "forbidden_by_typed_overlap") {
					t.Fatalf("%s lost its conservative arithmetic token: %s", name, prompt)
				}
				for _, want := range []string{
					"published locator envelopes intersect",
					"do not establish physical overlap of the measured components",
				} {
					if !strings.Contains(prompt, want) {
						t.Errorf("%s misteaches the envelope-only token; missing %q: %s", name, want, prompt)
					}
				}
				if strings.Contains(prompt, "direction_subtotal=12.115") {
					t.Errorf("%s authorized a subtotal from intersecting locators", name)
				}
			}
			after, _ := json.Marshal(set)
			if string(before) != string(after) {
				t.Fatal("teaching changed the projection or ranked values")
			}
		})
	}
}
