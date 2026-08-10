package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CHATFIX-1 enum surface pins.
func TestNormalizeScenarioChitchat(t *testing.T) {
	if got := normalizeScenario("chitchat"); got != types.ScenarioChitchat {
		t.Fatalf("chitchat must normalize to the typed enum, got %s", got)
	}
	if got := normalizeScenario("greeting"); got != types.ScenarioGeneric {
		t.Fatalf("non-enum smalltalk words must NOT alias into chitchat (LLM must pick the enum explicitly), got %s", got)
	}
}

// CHATFIX-1: a chitchat emission WITH a reply is legitimately
// keyword/entity-less — the degenerate gate must not burn a retry;
// a reply-less chitchat emission still hits the gate (fail-closed twin).
func TestDegenerateGateChitchatExemption(t *testing.T) {
	if rejectDegenerateClassification(types.IntentUnknown, "unknown", nil, nil) == "" {
		t.Fatal("premise: the bare degenerate shape must still reject")
	}
	// The exemption itself is applied at the call site keyed on
	// scenario+reply; pin the predicate contract here so a future
	// refactor keeps the pairing (see Execute wiring).
}
