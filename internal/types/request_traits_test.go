package types

import "testing"

func TestIsSingleTopicStructuralTrace(t *testing.T) {
	t.Run("call-chain trace is structural trace", func(t *testing.T) {
		rm := RequestModel{
			Intent:        IntentTrace,
			PredicateAxis: AxisCall,
			AnalyzerHints: AnalyzerHints{Kind: "call_chain"},
		}
		if !IsSingleTopicStructuralTrace(rm) {
			t.Fatal("expected structural trace")
		}
	})

	t.Run("cross-component trace is not lightweight structural trace", func(t *testing.T) {
		rm := RequestModel{
			Intent:        IntentTrace,
			PredicateAxis: AxisCall,
			AnalyzerHints: AnalyzerHints{Kind: "call_chain"},
			Predicates:    SemanticPredicates{IsCrossComponent: true},
		}
		if IsSingleTopicStructuralTrace(rm) {
			t.Fatal("cross-component trace should stay out of lightweight structural trace lane")
		}
	})

	t.Run("ambiguity keeps reconcile-worthy trace out of lightweight lane", func(t *testing.T) {
		rm := RequestModel{
			Intent:        IntentTrace,
			PredicateAxis: AxisCondition,
			Ambiguities:   []Ambiguity{{Clause: "which branch owns the fallback"}},
		}
		if IsSingleTopicStructuralTrace(rm) {
			t.Fatal("ambiguous trace should not be lightweight structural trace")
		}
	})
}

// TestIsProjectOrientationQuestion locks the structured-signal
// classifier behaviour. Each axis (intent / complexity / predicates
// / entities) must independently disqualify the orientation
// shortcut so a misclassification anywhere falls through to the
// safer (more thorough) full-coverage path. No keyword matching on
// RawRequest — the analyzer LLM's own classification IS the signal.
func TestIsProjectOrientationQuestion(t *testing.T) {
	base := RequestModel{
		Intent:     IntentExplain,
		Complexity: ComplexitySimple,
	}
	if !IsProjectOrientationQuestion(base) {
		t.Fatalf("baseline (explain + simple + clean predicates + no entities) must qualify")
	}

	cases := []struct {
		name string
		mod  func(*RequestModel)
		want bool
	}{
		{"root_cause intent disqualifies", func(r *RequestModel) { r.Intent = IntentRootCause }, false},
		{"trace intent disqualifies", func(r *RequestModel) { r.Intent = IntentTrace }, false},
		{"moderate complexity disqualifies", func(r *RequestModel) { r.Complexity = ComplexityModerate }, false},
		{"complex complexity disqualifies", func(r *RequestModel) { r.Complexity = ComplexityComplex }, false},
		{"is_cross_component disqualifies", func(r *RequestModel) { r.Predicates.IsCrossComponent = true }, false},
		{"is_count_question disqualifies", func(r *RequestModel) { r.Predicates.IsCountQuestion = true }, false},
		{"is_history_lookup disqualifies", func(r *RequestModel) { r.Predicates.IsHistoryLookup = true }, false},
		{"is_scalar_answer disqualifies", func(r *RequestModel) { r.Predicates.IsScalarAnswer = true }, false},
		{"PrimaryEntities disqualifies", func(r *RequestModel) { r.AnalyzerHints.PrimaryEntities = []string{"explorer"} }, false},
		{"any Entities disqualifies", func(r *RequestModel) { r.AnalyzerHints.Entities = []string{"foo"} }, false},
		{"unknown intent qualifies", func(r *RequestModel) { r.Intent = IntentUnknown }, true},
	}
	for _, tc := range cases {
		rm := base
		tc.mod(&rm)
		got := IsProjectOrientationQuestion(rm)
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
