package skill

import (
	"strings"
	"testing"
)

func TestB1645PublishedSkillDoesNotAssignOneRulerToEveryIOType(t *testing.T) {
	item := finBindTierBItem(t, "IO-LATENCY ROLE WORDS")
	if !item.AppliesTo.RequiresTrace {
		t.Fatal("existing trace-only scope changed")
	}
	if strings.Contains(item.Body, "a published IO-latency value is the REQUEST's own latency") ||
		strings.Contains(item.Body, "the request latency from the IO row") {
		t.Errorf("published shared skill assigns request residence to every IO row, contradicting native completion-closed issuer-blocked root rows: %s", item.Body)
	}
	for _, want := range []string{"request residence", "completion-closed", "S/D", "typed"} {
		if !strings.Contains(item.Body, want) {
			t.Errorf("published shared skill needs the row's explicit ruler rather than type-only role assignment: missing %q", want)
		}
	}
}
