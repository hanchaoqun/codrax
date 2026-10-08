package types

import "testing"

func TestRuntimeMeasurementMemberSetHandoffLifecycle(t *testing.T) {
	m := NewMutableState("measurement handoff")
	choice := AnswerRuntimeMeasurementReceipt{ObservationID: "native-a", View: RuntimeMeasurementMembers, BoundTable: &RuntimeMeasurementTable{Label: "not retained"}}
	m.SetInvestigationMeasurementMemberSets([]AnswerRuntimeMeasurementReceipt{choice})
	if len(m.InvestigationMeasurementMemberSets()) != 0 {
		t.Fatal("unaccepted choice retained")
	}
	m.SetInvestigationComplete("complete")
	m.SetInvestigationMeasurementMemberSets([]AnswerRuntimeMeasurementReceipt{choice})
	read := m.InvestigationMeasurementMemberSets()
	if len(read) != 1 || read[0].BoundTable != nil {
		t.Fatal("choice lost or values retained")
	}
	read[0].ObservationID = "mutated"
	inherited := m.ForkForExploreDispatch()
	accepted := m.ForkForExploreDispatch()
	accepted.SetInvestigationComplete("later")
	accepted.SetInvestigationMeasurementMemberSets([]AnswerRuntimeMeasurementReceipt{{ObservationID: "native-b", View: RuntimeMeasurementDistribution}})
	m.MergeExploreFork(accepted)
	m.MergeExploreFork(inherited)
	if got := m.InvestigationMeasurementMemberSets(); len(got) != 1 || got[0].ObservationID != "native-b" {
		t.Fatalf("inherited fork reverted new selection: %+v", got)
	}
	failed := m.ForkForExploreDispatch()
	failed.ResetInvestigationComplete()
	failed.SetInvestigationMeasurementMemberSets([]AnswerRuntimeMeasurementReceipt{choice})
	m.MergeExploreFork(failed)
	if got := m.InvestigationMeasurementMemberSets(); len(got) != 1 || got[0].ObservationID != "native-b" {
		t.Fatal("noncompleted fork changed selection")
	}
	clear := m.ForkForExploreDispatch()
	clear.SetInvestigationComplete("no selected population")
	clear.SetInvestigationMeasurementMemberSets(nil)
	m.MergeExploreFork(clear)
	if len(m.InvestigationMeasurementMemberSets()) != 0 {
		t.Fatal("accepted omitted choice didn't clear")
	}
	m.SetInvestigationComplete("complete")
	m.SetInvestigationMeasurementMemberSets([]AnswerRuntimeMeasurementReceipt{choice})
	m.ResetTurnAArtifacts()
	if len(m.InvestigationMeasurementMemberSets()) != 0 {
		t.Fatal("choice crossed turn boundary")
	}
}
