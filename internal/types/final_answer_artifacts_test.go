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
