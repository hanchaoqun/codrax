package orchestrator

import (
	"os"
	"strings"
	"testing"
)

// A3 wiring tripwire (customer feedback 2026-07-28): the orchestrator's
// reviewer/critic/classifier lanes (审阅/校验/验收/规划反思/续判) build bare
// system prompts outside BuildContext and stream USER-VISIBLE thinking —
// each must append the shared promptctx.ReasoningLanguagePreference; a lane
// silently losing it goes red here.
func TestReasoningLanguagePreferenceWiredIntoReviewerLanes(t *testing.T) {
	expected := map[string]int{
		"answer_reviewer.go":           1,
		"self_consistency_reviewer.go": 1,
		"semantic_quality_reviewer.go": 1,
		"plan_critic.go":               1,
		"acceptance_checker.go":        1,
		"reflector.go":                 1,
		"continuation_classifier.go":   1,
	}
	for file, want := range expected {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if got := strings.Count(string(raw), "promptctx.ReasoningLanguagePreference("); got != want {
			t.Fatalf("%s: %d ReasoningLanguagePreference wirings, want %d", file, got, want)
		}
	}
}
