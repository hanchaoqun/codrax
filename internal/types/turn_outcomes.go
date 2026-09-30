package types

import (
	"encoding/json"
	"fmt"
)

// TurnOutcomeSet is a comparable, value-owned set. Its wire form is a short
// enum array, not a second free-form task plan. It records requested results,
// never evidence, successful completion, or permission to mutate anything.
type TurnOutcomeSet uint8

const (
	TurnOutcomeAnswer TurnOutcomeSet = 1 << iota
	TurnOutcomeMeasurement
	TurnOutcomeSourceExplanation
	TurnOutcomeExternalArtifact
	TurnOutcomeComputerAction
	TurnOutcomeSourceChange
)

var turnOutcomeNames = [...]string{"answer", "measurement", "source_explanation", "external_artifact", "computer_action", "source_change"}

func (s TurnOutcomeSet) Has(kind TurnOutcomeSet) bool { return s&kind != 0 }

func (s TurnOutcomeSet) Names() []string {
	out := make([]string, 0, len(turnOutcomeNames))
	for i, name := range turnOutcomeNames {
		if s.Has(1 << i) {
			out = append(out, name)
		}
	}
	return out
}

func (s TurnOutcomeSet) MarshalJSON() ([]byte, error) { return json.Marshal(s.Names()) }

func (s *TurnOutcomeSet) UnmarshalJSON(data []byte) error {
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	var parsed TurnOutcomeSet
	for _, name := range names {
		found := false
		for i, known := range turnOutcomeNames {
			if name == known {
				parsed |= 1 << i
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown required_outcomes kind %q", name)
		}
	}
	*s = parsed
	return nil
}

// Simple single-action/measurement turns retain their cheap completion path.
// Composite results and independently verifiable deliverables need a goal
// check even when stdout was too short to create a payload file.
func (s TurnOutcomeSet) NeedsGoalEvaluation() bool {
	return s.Has(TurnOutcomeSourceExplanation|TurnOutcomeExternalArtifact) || len(s.Names()) > 1
}
