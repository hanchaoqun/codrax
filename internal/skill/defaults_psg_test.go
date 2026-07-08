package skill

// PSG batch P-s pins (§25 ruling b assertion half,
// docs/design/real_trace_campaign_20260705.md, 2026-07-08; huadong_01 §22
// C-P1): the answer-side skill carries the two trace-gated discipline
// sentences — prose numeral grounding (locate-or-cite-or-remove) and object
// identity assertions (thread names and object names are never
// interchangeable; holder claims need typed evidence rows). Both bodies are
// also swept by TestNoInternalTermsInPrompts, so a jargon regression fails
// there; these pins hold the SUBSTANCE.

import (
	"strings"
	"testing"
)

func psgAnswerSkillTierBBody(t *testing.T, marker string) TierBItem {
	t.Helper()
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	for _, item := range sk.WorkflowTierB {
		if strings.Contains(item.Body, marker) {
			return item
		}
	}
	t.Fatalf("answer-document-skill WorkflowTierB missing the %q item", marker)
	return TierBItem{}
}

func TestPSGFinalizerSkillProseNumberGrounding(t *testing.T) {
	item := psgAnswerSkillTierBBody(t, "PROSE NUMBER GROUNDING")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("the prose-number rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
	}
	for _, want := range []string{
		"must be locatable",
		"exact source view and time window",
		"or be removed from the prose",
		"name the published values it was derived from",
		// PSG-2 binding clause (§24.14 B-3/D-2): the window/thread named
		// next to a number must be its publishing row's, and cross-window
		// comparisons normalize per side first.
		"must be the ones the number's evidence row was published under",
		"normalize each side by its own window length",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("prose-number rule missing %q:\n%s", want, item.Body)
		}
	}
}

func TestPSGFinalizerSkillObjectIdentityAssertions(t *testing.T) {
	item := psgAnswerSkillTierBBody(t, "OBJECT IDENTITY ASSERTIONS")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("the identity-assertion rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
	}
	for _, want := range []string{
		"must be backed by an evidence row",
		"thread names and object names are never interchangeable",
		"does not hold that object by virtue of its name",
		"without asserting a holder",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("identity-assertion rule missing %q:\n%s", want, item.Body)
		}
	}
}
