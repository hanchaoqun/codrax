package types

import (
	"strings"
	"testing"
)

// presentation_authority_test.go — V7-4 (§40.54) unit pins for the single
// typed current-turn presentation carrier.

func TestPresentationAuthority_HardVisualIsClosedTwoValueSet(t *testing.T) {
	if got := (PresentationAuthority{}).HardVisual(); got != PresentationHardVisualNotRequired {
		t.Fatalf("empty authority HardVisual = %q, want %q", got, PresentationHardVisualNotRequired)
	}
	if got := (PresentationAuthority{Directive: "markdown table"}).HardVisual(); got != PresentationHardVisualNotRequired {
		t.Fatalf("directive-only authority must not infer a hard visual from words: %q", got)
	}
	if got := (PresentationAuthority{DiagramRequired: true}).HardVisual(); got != PresentationHardVisualRequired {
		t.Fatalf("typed bool must map to required: %q", got)
	}
	states := PresentationHardVisualStates()
	if len(states) != 2 {
		t.Fatalf("closed set must have exactly two states, got %v", states)
	}
	for _, s := range states {
		if !s.IsValid() {
			t.Fatalf("declared state %q must be valid", s)
		}
	}
	if PresentationHardVisualState("maybe").IsValid() {
		t.Fatal("unknown state must be invalid")
	}
}

func TestPresentationAuthority_PresentNeedsEitherCarrier(t *testing.T) {
	if (PresentationAuthority{}).Present() {
		t.Fatal("empty authority must be absent so non-diagram prompts stay byte-stable")
	}
	if !(PresentationAuthority{Directive: "sequence"}).Present() {
		t.Fatal("directive alone must render")
	}
	if !(PresentationAuthority{DiagramRequired: true}).Present() {
		t.Fatal("typed bool alone must render")
	}
}

func TestPresentationAuthority_AccessorsTrimAndAreNilSafe(t *testing.T) {
	var nilBus *BusContext
	if got := nilBus.PresentationAuthority(); got.Present() {
		t.Fatalf("nil BusContext accessor must be empty: %+v", got)
	}
	var nilAgent *AgentContext
	if got := nilAgent.PresentationAuthority(); got.Present() {
		t.Fatalf("nil AgentContext accessor must be empty: %+v", got)
	}
	bus := &BusContext{PresentationDirective: "  逻辑视图  ", PresentationDiagramRequired: true}
	got := bus.PresentationAuthority()
	if got.Directive != "逻辑视图" || !got.DiagramRequired {
		t.Fatalf("BusContext accessor = %+v", got)
	}
	ac := &AgentContext{PresentationDirective: " table ", PresentationDiagramRequired: false}
	if got := ac.PresentationAuthority(); got.Directive != "table" || got.DiagramRequired {
		t.Fatalf("AgentContext accessor = %+v", got)
	}
}

func TestPresentationHardVisualStatement_BilingualFromOneTable(t *testing.T) {
	cases := []struct {
		state PresentationHardVisualState
		lang  string
		want  string
	}{
		{PresentationHardVisualRequired, "en", "Hard visual requirement: required"},
		{PresentationHardVisualNotRequired, "EN", "Hard visual requirement: not required"},
		{PresentationHardVisualRequired, "zh", "硬性图示要求：需要"},
		{PresentationHardVisualNotRequired, "", "硬性图示要求：不需要"},
		// An invalid state can never mint a requirement.
		{PresentationHardVisualState("corrupt"), "en", "Hard visual requirement: not required"},
	}
	for _, c := range cases {
		if got := PresentationHardVisualStatement(c.state, c.lang); got != c.want {
			t.Errorf("Statement(%q, %q) = %q, want %q", c.state, c.lang, got, c.want)
		}
	}
	if !PromptLanguageIsEnglish(" en ") || PromptLanguageIsEnglish("zh") || PromptLanguageIsEnglish("") {
		t.Fatal("PromptLanguageIsEnglish must accept only an explicit en")
	}
}

func TestPresentationHardVisualTeaching_NamesBothCarriersAndNoWireIdentifiers(t *testing.T) {
	teaching := PresentationHardVisualTeaching()
	for _, want := range []string{
		"\"" + PresentationDirectiveSectionTitle + "\" section",
		"`" + PresentationHardVisualStatement(PresentationHardVisualRequired, "en") + "`",
		"`" + PresentationHardVisualStatement(PresentationHardVisualRequired, "zh") + "`",
		"requested_answer_dimensions row with role=diagram",
		"source_quote is verbatim from the CURRENT request",
		"normalized to false",
	} {
		if !strings.Contains(teaching, want) {
			t.Errorf("teaching must contain %q:\n%s", want, teaching)
		}
	}
	for _, banned := range []string{"out-of-band", "requires_diagram", "PresentationDiagramRequired", "the system"} {
		if strings.Contains(teaching, banned) {
			t.Errorf("teaching must not name %q", banned)
		}
	}
}
