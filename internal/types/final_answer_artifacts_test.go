package types

import "testing"

func TestFinalAnswerArtifactsAtomicRoundTrip(t *testing.T) {
	state := &MutableState{}
	finding := &TraceFindingV1{
		SchemaVersion: TraceFindingSchemaVersion,
		FindingID:     "finding-1",
		AnalysisKey:   "analysis-1",
		PrimaryCause: &TraceCauseDecision{
			CandidateID:  "candidate-1",
			EvidenceRefs: []string{"evidence-1"},
		},
	}
	state.SetFinalAnswerArtifactsWithMutation(MutationReplaceAll, &FinalAnswerArtifactsV1{
		Document:     AnswerDocumentV2{DocumentModel: "v2"},
		TraceFinding: finding,
	})

	got := state.FinalAnswerArtifacts()
	if got == nil || got.TraceFinding == nil || got.TraceFinding.FindingID != "finding-1" {
		t.Fatalf("atomic artifacts missing: %#v", got)
	}
	got.TraceFinding.PrimaryCause.EvidenceRefs[0] = "mutated"
	if state.TraceFinding().PrimaryCause.EvidenceRefs[0] != "evidence-1" {
		t.Fatal("returned finding aliases mutable state")
	}

	state.ResetAnswerDocumentV2()
	if state.AnswerDocumentV2() != nil || state.TraceFinding() != nil {
		t.Fatal("reset left half of the atomic artifact pair behind")
	}
}

func TestTraceMagnitudeComponentsDoNotAliasStoredFacts(t *testing.T) {
	state := NewMutableState("trace")
	decision := TraceCauseDecision{Magnitude: &TypedMagnitude{Value: 3,
		Components: &TraceMagnitudeComponents{SupplyFoldComputed: true, SupplyFoldUnknownMS: 2}}}
	contract := &TraceFindingContract{Candidates: []TraceFindingCandidateV1{{Decision: decision}}}
	state.SetTraceFindingContract(contract)
	decision.Magnitude.Components.SupplyFoldUnknownMS = 77
	read := state.TraceFindingContract()
	if read.Candidates[0].Decision.Magnitude.Components.SupplyFoldUnknownMS != 2 {
		t.Fatal("contract write aliases caller")
	}
	read.Candidates[0].Decision.Magnitude.Components.SupplyFoldUnknownMS = 88
	if state.TraceFindingContract().Candidates[0].Decision.Magnitude.Components.SupplyFoldUnknownMS != 2 {
		t.Fatal("contract read aliases stored facts")
	}
	finding := &TraceFindingV1{PrimaryCause: &decision, Contributors: []TraceCauseDecision{decision}}
	state.SetFinalAnswerArtifactsWithMutation(MutationReplaceAll, &FinalAnswerArtifactsV1{TraceFinding: finding})
	decision.Magnitude.Components.SupplyFoldUnknownMS = 99
	for _, got := range []*TraceCauseDecision{state.TraceFinding().PrimaryCause, &state.TraceFinding().Contributors[0]} {
		if got.Magnitude.Components.SupplyFoldUnknownMS != 77 {
			t.Fatal("finding write aliases caller")
		}
		got.Magnitude.Components.SupplyFoldUnknownMS = 100
	}
	if state.TraceFinding().PrimaryCause.Magnitude.Components.SupplyFoldUnknownMS != 77 ||
		state.TraceFinding().Contributors[0].Magnitude.Components.SupplyFoldUnknownMS != 77 {
		t.Fatal("finding read aliases stored facts")
	}
}
