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

func TestHasAttributeBearingEnumeration_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
		EnumerationBoundary: &RequestedEnumerationBoundary{
			DeclaredCount: 25,
			SourceQuote:   "all packages",
		},
	}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + relational lookup + boundary should be attribute-bearing")
	}

	rm.EnumerationBoundary = nil
	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all packages"}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + relational lookup + completeness obligation should be attribute-bearing")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.PredicateAxis = AxisDefine
	rm.AnalyzerHints.Entities = []string{"aggregator", "compiler"}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + predicate axis + exhaustive multi-member entity set should be attribute-bearing")
	}

	rm.PredicateAxis = AxisUnknown
	rm.SubTopics = []SubTopic{{Summary: "members"}, {Summary: "attributes"}}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + subtopic split + exhaustive multi-member entity set should be attribute-bearing")
	}

	rm.CompletenessObligation = nil
	rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{
		{Path: "a.go", Confidence: 0.9},
		{Path: "b.go", Confidence: 0.9},
	}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + subtopic split + required file hints for multiple members should be attribute-bearing")
	}

	rm.AnalyzerHints.RequiredFileHints = nil
	rm.SubTopics = nil
	if HasAttributeBearingEnumeration(rm) {
		t.Fatal("plain enumeration must not be treated as attribute-bearing")
	}
}

func TestHasBoundedCategoryEnumerationMembers_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"aggregator", "compiler"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all subpackages"},
	}
	if !HasBoundedCategoryEnumerationMembers(rm) {
		t.Fatal("category enumeration + multi-member completeness should expose a bounded member lane")
	}

	rm.Predicates.IsRelationalLookup = true
	if HasBoundedCategoryEnumerationMembers(rm) {
		t.Fatal("relational lookup entities are not a plain principal-member lane")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.Predicates.IsCategoryEnumeration = false
	if HasBoundedCategoryEnumerationMembers(rm) {
		t.Fatal("non-enumeration request must not expose bounded enumeration members")
	}
}

func TestIsCategoryEnumerationAnswerShape_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
	}
	if !IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("explicit category-enumeration predicate should mark set-valued answer shape")
	}

	rm = RequestModel{Intent: IntentEnumerate}
	if !IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("enumerate intent without scalar contradiction should mark set-valued answer shape")
	}

	rm.Predicates.IsScalarAnswer = true
	if IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("scalar answer predicate must keep enumerate intent out of set-valued answer shape")
	}

	rm.Predicates.IsScalarAnswer = false
	rm.Predicates.IsRoleLocateLookup = true
	if IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("role-locate predicate must keep enumerate intent out of set-valued answer shape")
	}

	rm.Predicates.IsRoleLocateLookup = false
	rm.Predicates.IsCountQuestion = true
	if IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("count predicate must keep enumerate intent out of set-valued answer shape")
	}
}

func TestCanUseAnalyzerEntitiesAsHardPrincipalMembers_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore"},
		},
	}
	if !CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("plain category enumeration entities should be eligible as hard principal members")
	}

	rm.Predicates.IsRelationalLookup = true
	rm.EnumerationBoundary = &RequestedEnumerationBoundary{
		DeclaredCount: 2,
		SourceQuote:   "2 agents",
	}
	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all agents"}
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("relation-shaped enumeration entities are mixed search hints, not a hard principal-member lane")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.Predicates.IsCategoryEnumeration = false
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("non-enumeration entities must not seed principal-member obligations")
	}

	rm.Predicates.IsCategoryEnumeration = true
	rm.AnalyzerHints.Entities = nil
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("empty entity list must not seed principal-member obligations")
	}
}

func TestIsSingleTopicMechanismExplanation_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		PredicateAxis: AxisCondition,
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqMechanism),
			Entities: []string{"emit_evidence", "anchor_kind", "EmitEvidence"},
		},
	}
	if !IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("single-topic mechanism explanation should be recognized from typed fields")
	}

	rm.Predicates.IsCrossComponent = true
	if IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("cross-component explanations are not the single-topic mechanism lane")
	}

	rm.Predicates.IsCrossComponent = false
	rm.EnumerationBoundary = &RequestedEnumerationBoundary{
		DeclaredCount: 3,
		SourceQuote:   "3 steps",
	}
	if IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("structural obligations should keep bounded mechanism questions out of the lightweight lane")
	}
}

func TestIsCodeIdentitySurface_CrossLanguage(t *testing.T) {
	accepted := []string{
		"aggregator",
		"findings_validator",
		"com.example.api",
		"react-dom",
		"@scope/pkg",
		"foo::bar",
		"packages/core",
		"ohos.ability",
	}
	for _, surface := range accepted {
		if !IsCodeIdentitySurface(surface) {
			t.Fatalf("expected %q to be accepted as a cross-language code identity", surface)
		}
	}

	rejected := []string{
		"",
		"two words",
		"entry point",
		"foo,bar",
		"needs?answer",
	}
	for _, surface := range rejected {
		if IsCodeIdentitySurface(surface) {
			t.Fatalf("expected %q to be rejected as prose / punctuation, not a code identity", surface)
		}
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

func TestIsScalarSourceLiteralLookup_DiagnosticPredicateBlocksScalarLaneWithoutBundle(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentRootCause,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup:   true,
			IsScalarAnswer:       true,
			IsDiagnosticQuestion: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
	}
	if IsScalarSourceLiteralLookup(rm) {
		t.Fatal("diagnostic questions must not enter the scalar source-literal lane, even without an attached artifact")
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
