package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPlannerProbeGranularityReachesActualInitialInstruction(t *testing.T) {
	for _, status := range []string{"satisfied", "failed", "unavailable"} {
		t.Run(status, func(t *testing.T) {
			report := &types.ChangeReport{
				PlanID: "plan-attempt", FailureKind: types.FailureKindTestsFailed,
				VerificationConfidence: []types.VerificationConfidenceRecord{{
					Source: "verification_probe", Category: "probe_contract_refs", Status: status,
					ContractRefs: []string{"search"}, Detail: "original probe observation",
					WitnessKind: types.WriteBehaviorWitnessVerificationProbe,
				}},
			}
			mu := types.NewMutableState("probe execution boundary")
			mu.SetVerifyFailureHandoff(types.BuildVerifyFailureHandoff(report, "batch", 1, "", ""))
			before, _ := json.Marshal(mu.VerifyFailureHandoff())
			text := (&plannerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mu}, nil)
			const phrase = "not per-contract/method execution receipts or runtime coverage"
			if strings.Contains(text, phrase) != (status == "satisfied") {
				t.Fatalf("actual planner prompt lost the exact proof boundary: %s", text)
			}
			if !strings.Contains(text, "original probe observation") {
				t.Fatal("planner discarded the existing confidence detail")
			}
			after, _ := json.Marshal(mu.VerifyFailureHandoff())
			if !bytes.Equal(before, after) {
				t.Fatal("prompt rendering mutated the failure carrier")
			}
		})
	}
}
