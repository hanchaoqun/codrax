package types

import (
	"encoding/json"
	"testing"
)

func TestTurnOutcomesWireAndValueOwnership(t *testing.T) {
	want := TurnOutcomeMeasurement | TurnOutcomeSourceExplanation
	data, err := json.Marshal(want)
	if err != nil || string(data) != `["measurement","source_explanation"]` {
		t.Fatalf("wire: %s %v", data, err)
	}
	var got TurnOutcomeSet
	if err := json.Unmarshal(data, &got); err != nil || got != want {
		t.Fatalf("roundtrip: %v %v", got, err)
	}
	for _, bad := range []string{`["invented"]`, `"source_explanation"`, `[12]`} {
		if json.Unmarshal([]byte(bad), &got) == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	hint := TurnRouteHint{RequiredOutcomes: want}
	copy := hint
	copy.RequiredOutcomes |= TurnOutcomeExternalArtifact
	if hint == copy || hint.IsZero() || !hint.RequiresCurrentSourceEvidence() {
		t.Fatal("value-owned evidence obligation lost")
	}
	for _, set := range []TurnOutcomeSet{0, TurnOutcomeAnswer, TurnOutcomeMeasurement, TurnOutcomeComputerAction} {
		if set.NeedsGoalEvaluation() {
			t.Fatalf("simple turn gained evaluation: %v", set)
		}
	}
	for _, set := range []TurnOutcomeSet{want, TurnOutcomeSourceExplanation, TurnOutcomeExternalArtifact, TurnOutcomeAnswer | TurnOutcomeMeasurement} {
		if !set.NeedsGoalEvaluation() {
			t.Fatalf("composite/deliverable bypass: %v", set)
		}
	}
}
