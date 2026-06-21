package types

import "testing"

func TestHasBoundedSourceEnumerationScope_TypedRequiredFilesOnly(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			Confidence:     0.9,
		},
	}
	files := []string{
		"src/alpha/a.py",
		"src/alpha/b.java",
		"src/alpha/c.ts",
		"src/alpha/d.kt",
		"src/alpha/e.proto",
		"src/alpha/f.cj",
	}

	if !HasBoundedSourceEnumerationScope(rm, files, "") {
		t.Fatal("typed category enumeration with many same-scope source files should be bounded")
	}
	if got := BoundedSourceEnumerationCommonScope(BoundedSourceEnumerationScopeFiles(rm, files, "")); got != "src/alpha" {
		t.Fatalf("common scope = %q, want src/alpha", got)
	}
}

func TestHasBoundedSourceEnumerationScope_RejectsScalarOrBroadRoot(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsScalarAnswer:        true,
		},
	}
	files := []string{
		"a.py",
		"b.java",
		"c.ts",
		"d.kt",
		"e.proto",
		"f.cj",
	}
	if HasBoundedSourceEnumerationScope(rm, files, "") {
		t.Fatal("scalar answer shape must not activate bounded source inventory scope")
	}
	rm.Predicates.IsScalarAnswer = false
	if HasBoundedSourceEnumerationScope(rm, files, "") {
		t.Fatal("root-level file spread must not activate bounded source inventory scope")
	}
}

func TestIsTypedSourceEnumerationShape_ExcludesLookupScalars(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	if !IsTypedSourceEnumerationShape(rm) {
		t.Fatal("plain typed category enumeration should be a source enumeration shape")
	}
	rm.Predicates.IsCountQuestion = true
	if IsTypedSourceEnumerationShape(rm) {
		t.Fatal("count questions must not activate source enumeration shape")
	}
}

func TestIsTypedSourceEnumerationShape_SourceScopeBacksInventoryLane(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentEnumerate,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeAll,
			Confidence:     0.9,
		},
	}
	if !IsTypedSourceEnumerationShape(rm) {
		t.Fatal("source-scope-backed enumeration should preserve the inventory lane even when category predicate is missing")
	}

	rm.Predicates.IsRoleLocateLookup = true
	if IsTypedSourceEnumerationShape(rm) {
		t.Fatal("role lookup should not become a source-inventory lane")
	}
}
