package orchestrator

import "testing"

// TestAssemblePlanCritique_LowConfidenceSuppressed pins commit 10
// #7: when the LLM self-reports confidence=low, the critique is
// dropped (the model said "I'm not sure"; rendering risks the LLM
// itself doubts would crowd out high-confidence critiques on
// other plans).
func TestAssemblePlanCritique_LowConfidenceSuppressed(t *testing.T) {
	got := assemblePlanCritique([]string{"some risk"}, "low")
	if got != "" {
		t.Errorf("low-confidence critique should be suppressed; got %q", got)
	}
	// Mixed case still suppressed.
	if got := assemblePlanCritique([]string{"some risk"}, "LOW"); got != "" {
		t.Errorf("uppercase 'LOW' should also be suppressed; got %q", got)
	}
	if got := assemblePlanCritique([]string{"some risk"}, "  Low  "); got != "" {
		t.Errorf("whitespace-padded 'Low' should be suppressed; got %q", got)
	}
}

// TestAssemblePlanCritique_HighConfidenceRenders pins the
// happy path: high or medium confidence critiques pass through.
func TestAssemblePlanCritique_HighConfidenceRenders(t *testing.T) {
	out := assemblePlanCritique([]string{"plan changes auth.go but not auth_test.go"}, "high")
	if out == "" {
		t.Fatal("high-confidence critique should render")
	}
	for _, want := range []string{"auth.go", "(confidence: high)"} {
		if !contains(out, want) {
			t.Errorf("expected %q in critique; got %q", want, out)
		}
	}
	if got := assemblePlanCritique([]string{"some risk"}, "medium"); got == "" {
		t.Error("medium-confidence critique should render")
	}
}

// TestAssemblePlanCritique_EmptyRisks pins the existing path:
// zero risks → empty critique regardless of confidence.
func TestAssemblePlanCritique_EmptyRisks(t *testing.T) {
	if got := assemblePlanCritique(nil, "high"); got != "" {
		t.Errorf("empty risks should yield empty critique; got %q", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
