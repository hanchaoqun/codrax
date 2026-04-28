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
