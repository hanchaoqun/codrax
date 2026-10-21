package types

import (
	"encoding/json"
	"testing"
)

func TestHMC276InterpretationUnitCloneAndRestoration(t *testing.T) {
	m := NewMutableState("analysis")
	m.SetInvestigationComplete("done")
	h := &InvestigationNativeFactHandoff{Interpretation: "legacy audit", InterpretationUnits: []InvestigationInterpretationUnit{{Text: "business clue", Boundaries: []string{"not a proved cause"}, SupportRefs: []string{"artifact:7"}}}}
	m.AcceptInvestigationNativeFactHandoff(h)
	h.InterpretationUnits[0].Text = "mutated"
	h.InterpretationUnits[0].Boundaries[0] = "mutated"
	h.InterpretationUnits[0].SupportRefs[0] = "mutated"
	got := m.AcceptedInvestigationNativeFactHandoff()
	if got.InterpretationUnits[0].Text != "business clue" || got.InterpretationUnits[0].Boundaries[0] != "not a proved cause" || got.InterpretationUnits[0].SupportRefs[0] != "artifact:7" {
		t.Fatal("accepted unit aliases caller")
	}
	if got.NarrativeInterpretation() != "" || !got.IsAuditOnlyInterpretationCopy("legacy audit") {
		t.Fatal("legacy duplicated beside units")
	}
	m.SetTurnAArtifacts(TurnAArtifacts{AcceptedClosureReason: "done"})
	wire, _ := json.Marshal(m.TurnAArtifacts())
	var restored TurnAArtifacts
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	fresh := NewMutableState("restored")
	fresh.SetTurnAArtifacts(restored)
	fresh.SetInvestigationComplete("done")
	if fresh.AcceptedInvestigationNativeFactHandoff() != nil || len(fresh.TurnAArtifacts().AcceptedNativeFactHandoff.InterpretationUnits) != 1 {
		t.Fatal("JSON minted acceptance or lost audit unit")
	}
	fork := m.ForkForExploreDispatch()
	u := fork.AcceptedInvestigationNativeFactHandoff()
	u.InterpretationUnits[0].Boundaries[0] = "changed clone"
	if m.AcceptedInvestigationNativeFactHandoff().InterpretationUnits[0].Boundaries[0] != "not a proved cause" {
		t.Fatal("fork/getter aliases unit")
	}
}
