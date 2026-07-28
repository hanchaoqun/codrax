package repl

import (
	"os"
	"strings"
	"testing"
)

// reasoning_language_wiring_test.go — A2/A4 wiring tripwire (customer
// feedback + audits 2026-07-28): the REPL's hand-built system prompts sit
// outside BuildContext. EVERY system-prompt mint in the covered files must
// append promptctx.ReasoningLanguagePreference — the A4 audit caught two
// retry rounds and two operation twins that the count-only tripwire missed,
// so the invariant is now EQUALITY: system mints == directive wirings per
// file (a new bare mint OR a silently unwired lane both go red).
func TestReasoningLanguagePreferenceWiredIntoREPLSystemPrompts(t *testing.T) {
	for _, file := range []string{
		"turn_policy.go",
		"chitchat.go",
		"data_task_planner.go",
		"data_material_extractor.go",
		"command_operation_planner.go",
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
			t.Fatalf("%s: %d system-prompt mints but %d reasoning-language wirings — a lane is bare or unwired", file, mints, wired)
		}
	}
}
