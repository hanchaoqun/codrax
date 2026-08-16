package skill

import (
	"strings"
	"testing"
)

// B873: reserve independence wording for an exact physical-relation verdict.
// Separate accounting lanes, ordinal domains, evidence channels, and
// supporting background must not reuse that word in trace-gated teaching.
func TestTraceGuidanceSeparatesPhysicalRelationFromAccountingVocabulary(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	var traceBodies strings.Builder
	for _, skillName := range []string{"explore-skill", "answer-document-skill"} {
		sk, err := r.Get(skillName)
		if err != nil {
			t.Fatalf("Get(%s): %v", skillName, err)
		}
		for _, item := range sk.WorkflowTierB {
			if item.AppliesTo.RequiresTrace {
				traceBodies.WriteString(item.Body)
				traceBodies.WriteByte('\n')
			}
		}
	}
	got := traceBodies.String()
	for _, want := range []string{
		"supporting background whose physical relation remains unresolved",
		"separate trace_semantic_span channel",
		"Regardless of root-cause TOP N",
		"never as two separate competing causes",
		"distinct ordinal domains",
		"two separate evidence lanes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("trace guidance missing separated vocabulary %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{
		"independent background",
		"independent trace_semantic_span channel",
		"Independently of root-cause TOP N",
		"independent competing causes",
		"independent ordinal domains",
		"two independent lanes",
	} {
		if strings.Contains(got, banned) {
			t.Fatalf("trace guidance still overloads physical independence through %q:\n%s", banned, got)
		}
	}

	if !strings.HasPrefix(independentMechanismContrastDirective, "MECHANISM-SPECIFIC EVIDENCE CONTRAST:") {
		t.Fatalf("generic mechanism guidance retained the overloaded title: %s", independentMechanismContrastDirective)
	}
	if strings.Contains(independentMechanismContrastDirective, "independently supported members") {
		t.Fatalf("generic mechanism guidance retained ambiguous accounting vocabulary: %s", independentMechanismContrastDirective)
	}
}
