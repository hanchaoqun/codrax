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
