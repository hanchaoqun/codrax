package types

import "testing"

// EVOLUTION RECORD (V1-5, colleague_merge_audit §40.16): this pin is the
// contract half of the former TestTraceMagnitudeComponentsDoNotAliasStoredFacts
// (final_answer_artifacts_test.go). The finding half exercised the retired
// Required trace_finding lane (SetFinalAnswerArtifactsWithMutation /
// TraceFinding) and went with it; the contract's defensive clone stays pinned.
func TestTraceFindingContractMagnitudeComponentsDoNotAliasStoredFacts(t *testing.T) {
	state := NewMutableState("trace")
	decision := TraceCauseDecision{Magnitude: &TypedMagnitude{Value: 3,
		Components: &TraceMagnitudeComponents{SupplyFoldComputed: true, SupplyFoldUnknownMS: 2,
			GatedComponentsPresent: true, GatedRunnableMS: 1, GatedRunningDeficitMS: 2}}}
	contract := &TraceFindingContract{Candidates: []TraceFindingCandidateV1{{Decision: decision}}}
	state.SetTraceFindingContract(contract)
	decision.Magnitude.Components.SupplyFoldUnknownMS = 77
	decision.Magnitude.Components.GatedRunningDeficitMS = 77
	read := state.TraceFindingContract()
	if read.Candidates[0].Decision.Magnitude.Components.SupplyFoldUnknownMS != 2 || read.Candidates[0].Decision.Magnitude.Components.GatedRunningDeficitMS != 2 {
		t.Fatal("contract write aliases caller")
	}
	read.Candidates[0].Decision.Magnitude.Components.SupplyFoldUnknownMS = 88
	read.Candidates[0].Decision.Magnitude.Components.GatedRunnableMS = 88
	if state.TraceFindingContract().Candidates[0].Decision.Magnitude.Components.SupplyFoldUnknownMS != 2 || state.TraceFindingContract().Candidates[0].Decision.Magnitude.Components.GatedRunnableMS != 1 {
		t.Fatal("contract read aliases stored facts")
	}
}
