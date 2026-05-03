package types

import "testing"

// TestIsScalarSourceLiteralLookup_RoleLocateBlockedByLogFrames pins
// the 2026-05-02 fix: a panic / crash question whose RequestModel has
// IsRoleLocateLookup=true must NOT short-circuit to scalar lane when
// the user attached a multi-frame LogBundle. The bundle is objective
// evidence that the answer surface is a multi-step mechanism, not a
// single source literal. Without this guard, the scenario reconciler
// rewrites root_cause → generic, the system telegraphs "scalar
// source-literal" via WARN, and the LLM self-corrupts to a scalar
// answer across multi-emit retries.
func TestIsScalarSourceLiteralLookup_RoleLocateBlockedByLogFrames(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentRootCause,
		Complexity:    ComplexityModerate,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
		LogTriage: &LogBundle{
			Errors: []LogError{{
				Type: "panic",
				Frames: []LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
					{File: "internal/agent/analyzer.go", Line: 320, Func: "ParseOutput"},
				},
			}},
		},
	}
	if IsScalarSourceLiteralLookup(rm) {
		t.Fatal("role-locate short-circuit must NOT fire when 2+ log frames attached — bundle is objective evidence of multi-step mechanism")
	}
}

// TestIsScalarSourceLiteralLookup_RoleLocateAllowedWithoutBundle is
// the negative-control: same RM minus the bundle → role-locate
// short-circuit is restored. Confirms the fix is gated by the
// artifact, not by a new universal blocker.
func TestIsScalarSourceLiteralLookup_RoleLocateAllowedWithoutBundle(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
		// no LogTriage / PerfTrace
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("role-locate short-circuit must still fire when no artifact bundle is attached (preserves single-literal locate behaviour)")
	}
}

// TestIsScalarSourceLiteralLookup_RoleLocateBlockedByPerfTrace pins
// the perf channel: HiTrace / atrace / systrace bundle with 2+ stalls
// or janks is also "multi-step mechanism" evidence and must block the
// scalar short-circuit, same as multi-frame log.
func TestIsScalarSourceLiteralLookup_RoleLocateBlockedByPerfTrace(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
		PerfTrace: &PerfBundle{
			Janks: []PerfJank{
				{StartTsMs: 100, DurationMs: 25, Reason: "main-thread-blocked"},
				{StartTsMs: 200, DurationMs: 30, Reason: "gc-pause"},
			},
		},
	}
	if IsScalarSourceLiteralLookup(rm) {
		t.Fatal("role-locate short-circuit must NOT fire when 2+ perf janks attached")
	}
}

// TestIsScalarSourceLiteralLookup_SingleFrameDoesNotBlock pins the
// threshold: a 1-frame stack alone is NOT multi-step evidence (could
// be a single-line assertion failure). The role-locate short-circuit
// is preserved in this case so a "find the file that crashed" lookup
// still resolves to a literal.
func TestIsScalarSourceLiteralLookup_SingleFrameDoesNotBlock(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
		LogTriage: &LogBundle{
			Errors: []LogError{{
				Type: "panic",
				Frames: []LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				},
			}},
		},
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("single-frame log should NOT temper role-locate short-circuit (threshold is 2+)")
	}
}

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
