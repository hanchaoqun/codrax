package orchestrator

import (
	"os"
	"strings"
	"testing"
)

// A3/A4 wiring tripwire: reviewer/critic/classifier lanes build bare system
// prompts outside BuildContext and stream USER-VISIBLE thinking. Invariant =
// EQUALITY per file: every system-prompt mint appends the shared
// promptctx.ReasoningLanguagePreference (new bare mints and unwired lanes
// both go red).
func TestReasoningLanguagePreferenceWiredIntoReviewerLanes(t *testing.T) {
	for _, file := range []string{
		"answer_reviewer.go",
		"self_consistency_reviewer.go",
		"semantic_quality_reviewer.go",
		"plan_critic.go",
		"acceptance_checker.go",
		"reflector.go",
		"continuation_classifier.go",
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		src := string(raw)
		mints := strings.Count(src, `{Role: "system"`)
		wired := strings.Count(src, "promptctx.ReasoningLanguagePreference(")
		if mints == 0 {
			t.Fatalf("%s: no system-prompt mints found — tripwire anchor drifted", file)
		}
		if mints != wired {
			t.Fatalf("%s: %d system-prompt mints but %d reasoning-language wirings", file, mints, wired)
		}
	}
}
