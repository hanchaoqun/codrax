package types

import (
	"encoding/json"
	"testing"
)

func TestHMC247NativeFactHandoffLifecycle(t *testing.T) {
	m := NewMutableState("question")
	h := &InvestigationNativeFactHandoff{RuntimeMeasurements: []AnswerRuntimeMeasurementReceipt{{ObservationID: "a", View: RuntimeMeasurementSummary, BoundTable: &RuntimeMeasurementTable{Label: "not retained"}}}, Interpretation: "independent"}
	m.AcceptInvestigationNativeFactHandoff(h)
	if m.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("unaccepted handoff retained")
	}
	m.SetInvestigationComplete("first")
	m.AcceptInvestigationNativeFactHandoff(h)
	h.Interpretation = "mutated"
	got := m.AcceptedInvestigationNativeFactHandoff()
	if got.Interpretation != "independent" || got.RuntimeMeasurements[0].BoundTable != nil {
		t.Fatal("intent aliased or table authority retained")
	}
	got.RuntimeMeasurements[0].ObservationID = "mutated"
	m.SetTurnAArtifacts(TurnAArtifacts{AcceptedClosureReason: "first"})
	ta := m.TurnAArtifacts()
	if ta.AcceptedNativeFactHandoff.RuntimeMeasurements[0].ObservationID != "a" {
		t.Fatal("mutable getter leaked")
	}
	wire, err := json.Marshal(ta)
	if err != nil {
		t.Fatal(err)
	}
	var restored TurnAArtifacts
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	fresh := NewMutableState("question")
	fresh.SetTurnAArtifacts(restored)
	fresh.SetInvestigationComplete("first")
	if fresh.AcceptedInvestigationNativeFactHandoff() != nil || fresh.TurnAArtifacts().AcceptedNativeFactHandoff == nil {
		t.Fatal("JSON lost audit intent or minted acceptance")
	}
	inherited := m.ForkForExploreDispatch()
	deciding := m.ForkForExploreDispatch()
	deciding.SetInvestigationComplete("second")
	deciding.AcceptInvestigationNativeFactHandoff(&InvestigationNativeFactHandoff{RuntimeMeasurements: []AnswerRuntimeMeasurementReceipt{{ObservationID: "b", View: RuntimeMeasurementTimeline}}, Interpretation: "second analysis"})
	m.MergeExploreFork(deciding)
	m.MergeExploreFork(inherited)
	if m.AcceptedInvestigationNativeFactHandoff().RuntimeMeasurements[0].ObservationID != "b" {
		t.Fatal("inherited fork replaced current decision")
	}
	m.ResetInvestigationComplete()
	if m.AcceptedInvestigationNativeFactHandoff() == nil {
		t.Fatal("accepted interpretation lost across same investigation retry")
	}
	m.SetInvestigationComplete("second")
	if m.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("system completion reused old generation")
	}
	m.AcceptInvestigationNativeFactHandoff(h)
	m.ResetTurnAArtifacts()
	if m.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("handoff crossed task reset")
	}
}
