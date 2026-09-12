package writeflow

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1575ActualPatchReviewKeepsPlanningOnlyImpactAdvisory(t *testing.T) {
	plan := &types.ChangePlan{ID: "current", BehaviorContracts: []types.WriteBehaviorContract{{
		ID: "planning", Kind: types.WriteBehaviorObservable, Required: false,
		Source:   "write_analyzer;" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded,
		Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals, Expected: "42",
	}}, ImpactObligations: &types.ImpactObligationSet{PlanID: "current", Obligations: []types.ImpactObligation{
		{Kind: "behavior_contract", Relation: "contract_ref", Obligation: "verify_contract_ref", ContractRef: "planning", EvidenceRef: "planning", Strength: types.ImpactObligationStrengthDeclared},
		{Kind: "behavior_contract", Relation: "contract_ref", Obligation: "verify_contract_ref", ContractRef: "unknown", EvidenceRef: "unknown", Strength: types.ImpactObligationStrengthDeclared},
	}}}
	before, _ := json.Marshal(plan)
	review := ReviewAppliedPatchSemantic(SemanticPatchReviewInput{Plan: plan})
	if review.HardBlock {
		t.Fatal("planning observation changed hard scope gate")
	}
	seen := 0
	for _, finding := range review.Findings {
		if finding.ImpactKind != types.PatchReviewImpactKindBehaviorContract {
			continue
		}
		seen++
		want := types.PatchReviewCoverageUnverified
		if finding.EvidenceRef == "planning" {
			want = types.PatchReviewCoverageAdvisory
		}
		if finding.CoverageStatus != want {
			t.Errorf("review promoted planning support or erased unknown: %+v want=%s", finding, want)
		}
	}
	if seen != 2 {
		t.Fatalf("historical observations were erased: %+v", review)
	}
	after, _ := json.Marshal(plan)
	if !bytes.Equal(before, after) {
		t.Fatal("review mutated original plan")
	}
}
