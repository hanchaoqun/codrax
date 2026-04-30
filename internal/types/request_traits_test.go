package types

import "testing"

func TestIsScalarSourceLiteralLookup_FallbackForUnnamedRoleLocate(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		AnalyzerHints: AnalyzerHints{
			Kind: "mechanism",
		},
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("expected unnamed simple role-locate lookup to stay in scalar source-literal lane")
	}
	if !IsScalarRoleLocateLookup(rm) {
		t.Fatal("expected unnamed scalar fallback to be treated as role-locate lookup")
	}
}

func TestIsScalarSourceLiteralLookup_FallbackDoesNotFireWhenEntityAlreadyNamed(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		AnalyzerHints: AnalyzerHints{
			Kind:            "mechanism",
			PrimaryEntities: []string{"buildAnalysisIR"},
			Entities:        []string{"buildAnalysisIR"},
		},
	}
	if IsScalarSourceLiteralLookup(rm) {
		t.Fatal("named-entity mechanism question should not be forced into scalar source-literal fallback")
	}
	if IsScalarRoleLocateLookup(rm) {
		t.Fatal("named-entity mechanism question should not be treated as role-locate fallback")
	}
}

func TestIsScalarSourceLiteralLookup_ExplicitRoleLocateKeepsScalarLaneWithClueEntity(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexityModerate,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsScalarAnswer:     true,
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind:            "mechanism",
			PrimaryEntities: []string{"AnalysisIR"},
			Entities:        []string{"AnalysisIR"},
		},
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("explicit role-locate predicate should keep the request in scalar source-literal lane even when the user named a clue entity")
	}
	if !IsScalarRoleLocateLookup(rm) {
		t.Fatal("explicit role-locate predicate should classify as scalar role-locate lookup")
	}
}

func TestIsScalarSourceLiteralLookup_ExplicitRoleLocateAllowsConfigKey(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
		Predicates: SemanticPredicates{
			IsScalarAnswer:     true,
			IsRoleLocateLookup: true,
		},
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("config-key role-locate should stay in scalar lookup lane")
	}
}
