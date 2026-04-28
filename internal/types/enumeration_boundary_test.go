package types

import (
	"strings"
	"testing"
)

func TestRecoverRequestedEnumerationBoundary_SingleOwnerMechanism(t *testing.T) {
	rm := RequestModel{
		RawRequest: "gate.Run 的 7 项检查是按什么顺序跑的？",
		Predicates: SemanticPredicates{},
		AnalyzerHints: AnalyzerHints{
			Kind:              "mechanism",
			Entities:          []string{"gate.Run"},
			PrimaryEntities:   []string{"gate.Run"},
			MentionedEntities: []string{"gate.Run"},
		},
	}
	got := RecoverRequestedEnumerationBoundary(rm)
	if got == nil {
		t.Fatal("expected recovered enumeration boundary")
	}
	if got.DeclaredCount != 7 {
		t.Fatalf("DeclaredCount = %d, want 7", got.DeclaredCount)
	}
	if !strings.Contains(got.SourceQuote, "7 项检查") {
		t.Fatalf("SourceQuote = %q, want it to contain %q", got.SourceQuote, "7 项检查")
	}
}

func TestRecoverRequestedEnumerationBoundary_SkipsCrossComponent(t *testing.T) {
	rm := RequestModel{
		RawRequest: "What are the first 3 differences between A and B?",
		Predicates: SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: AnalyzerHints{
			Kind:              "mechanism",
			Entities:          []string{"A", "B"},
			PrimaryEntities:   []string{"A", "B"},
			MentionedEntities: []string{"A", "B"},
		},
	}
	if got := RecoverRequestedEnumerationBoundary(rm); got != nil {
		t.Fatalf("expected nil boundary for cross-component request, got %+v", got)
	}
}
