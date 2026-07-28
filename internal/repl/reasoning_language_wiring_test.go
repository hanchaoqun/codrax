package repl

import (
	"os"
	"strings"
	"testing"
)

// reasoning_language_wiring_test.go — A2 wiring tripwire (customer feedback
// 2026-07-28): the REPL's hand-built system prompts sit outside BuildContext
// and used to carry ZERO language guidance — the turn-policy router's
// round-1 thinking (the first thing every session shows) was always English.
// Every covered system-prompt mint must append the shared
// promptctx.ReasoningLanguagePreference; silently unwiring any site goes red.
func TestReasoningLanguagePreferenceWiredIntoREPLSystemPrompts(t *testing.T) {
	expected := map[string]int{
		"turn_policy.go":               2, // router + local responder
		"chitchat.go":                  4, // two chitchat lanes + recall lane + classifier
		"data_task_planner.go":         4, // planner/evaluator/result-patch/tool-repair
		"data_material_extractor.go":   1,
		"command_operation_planner.go": 4, // planner/answerer/evaluation/tool-repair
	}
	for file, want := range expected {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		src := string(raw)
		got := strings.Count(src, "promptctx.ReasoningLanguagePreference(")
		if got != want {
			t.Fatalf("%s: %d ReasoningLanguagePreference wirings, want %d — a system-prompt lane lost its reasoning-language guidance", file, got, want)
		}
	}
}
