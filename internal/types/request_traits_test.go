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

func TestHasObservationOnlyRuntimeArtifact_RequiredFilesOpenCurrentSourceLane(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "timeout"}},
		},
		AnalyzerHints: AnalyzerHints{
			RequiredFileHints: []RequiredFileHint{{
				Path:       "internal/llm/openai.go",
				Confidence: 0.9,
				Rationale:  "pre-scan matched the current-source mechanism requested by the user",
			}},
		},
	}
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("external runtime artifact with typed required files must keep the current-source explanation lane open")
	}

	rm.AnalyzerHints.RequiredFileHints = nil
	if !rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("external runtime artifact without a typed current-source anchor should remain observation-only")
	}
}

func TestHasObservationOnlyRuntimeArtifact_CurrentKeyCodeDimensionOpensCurrentSourceLane(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "timeout"}},
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Confidence:          0.9,
			Dimensions: []RequestedAnswerDimension{{
				Label:    "当前关键代码",
				Role:     RequestedAnswerDimensionCurrentKeyCode,
				Required: true,
				Index:    1,
			}},
		},
	}
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("explicit current_key_code dimension should keep mixed runtime/current-source lane open")
	}

	rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionImpact
	if !rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("non-current-source presentation dimensions must not open repo reads for external-only runtime artifacts")
	}
}

func TestHasObservationOnlyRuntimeArtifact_CurrentSourceExplanationProfileOpensLane(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "timeout"}},
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"当前源码解释"},
			Confidence:                          0.9,
		},
	}
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("typed current-source explanation profile should keep mixed runtime/current-source lane open")
	}

	rm.CurrentSourceExplanationProfile.SourceQuotes = nil
	if !rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("inactive current-source explanation profile must not open current-source lane")
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

func TestRequiresExhaustiveEnumerationMemberSetHandoff_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"Intent", "QuestionFamily", "Scenario"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all public enum types"},
	}
	if !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("exhaustive category enumeration should require a structured member_set handoff")
	}

	rm.CompletenessObligation = nil
	rm.EnumerationBoundary = &RequestedEnumerationBoundary{DeclaredCount: 3, SourceQuote: "three public enum types"}
	if !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("declared-count enumeration should require a structured member_set handoff")
	}

	rm.EnumerationBoundary = nil
	rm.Predicates.IsRelationalLookup = true
	if !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("set-valued relational enumeration should require structured member_set handoff")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.EnumerationBoundary = nil
	rm.CompletenessObligation = nil
	rm.AnalyzerHints.Entities = []string{"aggregator", "compiler"}
	if !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("typed non-relation category member lane should require structured member_set handoff even without a count quote")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.Predicates.IsScalarAnswer = true
	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all"}
	if RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("scalar contradiction must keep the request out of exhaustive member-set handoff")
	}

	role := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
			IsRoleLocateLookup:    true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqReturnValue)},
	}
	if RequiresExhaustiveEnumerationMemberSetHandoff(role) {
		t.Fatal("scalar role-locate relation lookup must not require member_set handoff")
	}

	arch := RequestModel{
		Intent:      IntentExplain,
		Scenario:    ScenarioArchitectureExplain,
		Complexity:  ComplexityComplex,
		Predicates:  SemanticPredicates{IsCategoryEnumeration: true, IsCrossComponent: true},
		DiagramHint: &DiagramHint{Kind: DiagramSequence},
		SubTopics:   []SubTopic{{Summary: "A"}, {Summary: "B"}},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"Analyzer", "Explorer", "Finalizer"},
		},
	}
	if RequiresExhaustiveEnumerationMemberSetHandoff(arch) {
		t.Fatal("architecture narrative component names are context, not an exhaustive principal member_set")
	}
}

func TestRequiresRelationMemberSetHandoff_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsRelationalLookup:    true,
			IsCategoryEnumeration: true,
		},
	}
	if !RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("set-valued relation lookup should require a structured relation member_set handoff")
	}

	rm.Predicates.IsCategoryEnumeration = false
	rm.Predicates.IsCountQuestion = true
	if !RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("relation count lookup should require a structured member_set so the count has a verified member basis")
	}

	rm.Predicates.IsCountQuestion = false
	rm.Predicates.IsRoleLocateLookup = true
	if RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("scalar role-location relation must not require a member_set")
	}

	rm.Predicates.IsRoleLocateLookup = false
	rm.Intent = IntentExplain
	if RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("mechanism-only relation explanation must not require a qualifying-member set")
	}
}

func TestHistoryLookupPrefersVCSNarrativePrincipal_TypedBoundary(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioGeneric,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		SubTopics: []SubTopic{
			{Summary: "commit topic A", Entities: []string{"ScalarAnswer"}},
			{Summary: "commit topic B", Entities: []string{"AggregateScalar"}},
		},
	}
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("non-scalar history narrative should keep VCS metadata as the principal lane despite analyzer subtopics")
	}

	rm.Scenario = ScenarioArchitectureExplain
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqHistory)}
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("history evolution questions may be architecture_explain while still being VCS-principal")
	}
	rm.Scenario = ScenarioGeneric
	rm.AnalyzerHints = AnalyzerHints{}

	rm.Intent = IntentEnumerate
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("pure recent-N commit enumeration must remain VCS-principal and must not force current-source reads")
	}
	rm.Intent = IntentExplain
	rm.Predicates.IsCategoryEnumeration = true
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("pure commit category enumeration is still VCS metadata, not current source evidence")
	}
	rm.Predicates.IsCategoryEnumeration = false

	rm.Predicates.IsCrossComponent = true
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqHistory)}
	rm.Buckets = []QuestionBucket{{Label: "commit A", Index: 1}, {Label: "commit B", Index: 2}}
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("pure typed commit-history comparison should remain VCS-principal instead of forcing current-source reads")
	}
	rm.Buckets = nil
	rm.Predicates.IsCrossComponent = false
	rm.AnalyzerHints = AnalyzerHints{}

	currentSourceContract := &AnswerContract{Diagram: &DiagramContract{Required: true, RequiredKind: DiagramFlow}}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, currentSourceContract) {
		t.Fatal("answer contract current_source origin must keep mixed history/current-source lane")
	}

	rm.Predicates.IsScalarAnswer = true
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("true scalar history lookup must not enter narrative-only lane")
	}
	rm.Predicates.IsScalarAnswer = false

	rm.DiagramHint = &DiagramHint{Kind: DiagramFlow}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("history + diagram/code-flow request needs mixed history/current-source evidence")
	}
	rm.DiagramHint = nil

	contract := &AnswerContract{Diagram: &DiagramContract{Required: true, RequiredKind: DiagramFlow}}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, contract) {
		t.Fatal("required diagram contract must keep current-source evidence principal")
	}

	rm.ChangeImpactProfile = &ChangeImpactProfile{IsChangeImpact: true}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("history + change-impact request needs current-source principal evidence")
	}
	rm.ChangeImpactProfile = nil

	rm.DiagnosticProfile = DiagnosticIntentProfile{IsDiagnostic: true, CurrentVersionCheck: true}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("history + current diagnostic check needs mixed evidence")
	}
}

func TestIsHistoryBackedCurrentCodeExplanation_TypedBoundary(t *testing.T) {
	rm := RequestModel{
		RawRequest:    "从历史提交里找一次相关改动，再结合当前代码解释现在链路怎么工作",
		Intent:        IntentExplain,
		Scenario:      ScenarioArchitectureExplain,
		Complexity:    ComplexityComplex,
		PredicateAxis: AxisDefine,
		Predicates: SemanticPredicates{
			IsHistoryLookup:  true,
			IsCrossComponent: true,
		},
		AnalyzerHints:       AnalyzerHints{Kind: string(ReqMechanism)},
		EnumerationBoundary: &RequestedEnumerationBoundary{DeclaredCount: 1, SourceQuote: "一次"},
	}
	if !IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("history + current-code mechanism explanation should keep a mixed narrative lane")
	}

	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqHistory)}
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("pure history evolution request must not be treated as current-code mixed analysis just because scenario=architecture_explain")
	}
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqMechanism)}

	rm.Intent = IntentTrace
	if !IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("history + current-code mechanism trace with define axis should still use mixed narrative lane")
	}
	rm.PredicateAxis = AxisCall
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqCallChain)}
	if !IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("history + current-code explanation misclassified as call_chain should still use mixed narrative lane when no explicit endpoints exist")
	}
	rm.AnalyzerHints.ExactTargets = []string{"A", "B"}
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("true call-chain trace with explicit endpoints must keep call-chain semantics")
	}
	rm.PredicateAxis = AxisDefine
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqMechanism)}
	rm.Intent = IntentExplain

	rm.Intent = IntentEnumerate
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("explicit history enumeration/list request must not be treated as narrative-only")
	}
	rm.Intent = IntentExplain

	rm.Buckets = []QuestionBucket{{Label: "commit A", Index: 1}, {Label: "commit B", Index: 2}}
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("explicit bucketed history comparison should stay in comparison lane")
	}
	rm.Buckets = nil

	rm.Predicates.IsScalarAnswer = true
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("scalar history lookup must not become mixed current-code narrative")
	}
	rm.Predicates.IsScalarAnswer = false

	rm.Intent = IntentEnumerate
	rm.Predicates.IsCategoryEnumeration = true
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("explicit history enumeration/list request must not be treated as narrative-only")
	}
	rm.Intent = IntentExplain
	if !IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("category-enumeration drift must not override history + current-code mechanism explanation")
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

	rm.CompletenessObligation = &CompletenessObligation{
		Required:    true,
		SourceQuote: "all members",
	}
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("exhaustive enumeration entities are search hints until exploration emits the verified member set")
	}
	rm.CompletenessObligation = nil

	rm.EnumerationBoundary = &RequestedEnumerationBoundary{
		DeclaredCount: 2,
		SourceQuote:   "2 members",
	}
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("declared-count enumeration entities must not seed hard members before exploration")
	}
	rm.EnumerationBoundary = nil

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

func TestHasPrincipalCategoryEnumerationMemberLane_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"aggregator", "compiler"},
		},
	}
	if !HasPrincipalCategoryEnumerationMemberLane(rm) {
		t.Fatal("plain typed category enumeration with multiple emitted members should expose a principal member lane")
	}

	rm.Predicates.IsRelationalLookup = true
	if HasPrincipalCategoryEnumerationMemberLane(rm) {
		t.Fatal("relation lookup entities are mixed targets/helpers, not a principal member lane")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.Intent = IntentExplain
	rm.AnalyzerHints.Kind = string(ReqMechanism)
	if HasPrincipalCategoryEnumerationMemberLane(rm) {
		t.Fatal("non-enumerate/non-enumeration-kind category drift must not expose a hard member lane")
	}
}

func TestStructuralRelationScopeCandidates_UsesProvenanceLanes(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"UserFacingInterface"},
			ExactTargets:      []string{"ExactFallback"},
			PrimaryEntities:   []string{"PrimaryInterface"},
			Entities:          []string{"UserFacingInterface", "ContextHelper", "SymbolResolver"},
			DerivedEntities:   []string{"ContextHelper", "SymbolResolver"},
		},
	}
	got := StructuralRelationScopeCandidates(rm)
	want := []string{"UserFacingInterface", "ExactFallback", "PrimaryInterface"}
	if len(got) != len(want) {
		t.Fatalf("candidates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidates = %+v, want %+v", got, want)
		}
	}
}

func TestStructuralRelationScopeCandidates_LegacyEntitiesFallback(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"Looper", "Looper", "Other"},
		},
	}
	got := StructuralRelationScopeCandidates(rm)
	want := []string{"Looper", "Other"}
	if len(got) != len(want) {
		t.Fatalf("legacy candidates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("legacy candidates = %+v, want %+v", got, want)
		}
	}
}

func TestShouldSurfaceTypedRelationHints_ImplementAxisCoversDiagramMechanism(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Scenario:      ScenarioArchitectureExplain,
		PredicateAxis: AxisImplement,
		DiagramHint:   &DiagramHint{Kind: DiagramArchitecture},
	}
	if !ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("predicate_axis=implement should surface typed relation facts even outside enumeration")
	}
	rm.PredicateAxis = AxisUnknown
	if ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("plain architecture diagram without a typed relation axis must not surface relation hints")
	}
	rm.Predicates.IsRelationalLookup = true
	if !ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("relational lookup should continue surfacing typed relation facts")
	}
}

func TestHasTypedRelationMemberSetShape_TypedOnly(t *testing.T) {
	rm := RequestModel{PredicateAxis: AxisImplement}
	if !HasTypedRelationMemberSetShape(rm) {
		t.Fatal("predicate_axis=implement should mark principal member sets as relation membership")
	}
	rm.PredicateAxis = AxisUnknown
	if HasTypedRelationMemberSetShape(rm) {
		t.Fatal("unknown axis without relational predicate must not mark source inventory as relation membership")
	}
	rm.Predicates.IsRelationalLookup = true
	if !HasTypedRelationMemberSetShape(rm) {
		t.Fatal("relational lookup predicate should mark principal member sets as relation membership")
	}
}

func TestArchitectureNarrativeExplanation_TypedBoundary(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityComplex,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true,
		},
		DiagramHint: &DiagramHint{Kind: DiagramSequence},
		SubTopics: []SubTopic{
			{Summary: "调度机制", Entities: []string{"Orchestrator", "PipelineStage"}},
			{Summary: "Agent职责", Entities: []string{"AnalyzerAgent", "ExplorerAgent"}},
			{Summary: "上下文传播", Entities: []string{"BusContext", "AgentContext"}},
		},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"Orchestrator", "AnalyzerAgent", "ExplorerAgent", "AgentContext"},
		},
	}
	if !IsArchitectureNarrativeExplanation(rm) {
		t.Fatal("architecture view + diagram + subtopics should be recognized as narrative, not a member slate")
	}
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("architecture narrative entities must remain soft hints even when category drift is present")
	}
	if HasBoundedCategoryEnumerationMembers(rm) {
		t.Fatal("architecture narrative must not expose a bounded principal-member lane")
	}

	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all components"}
	if IsArchitectureNarrativeExplanation(rm) {
		t.Fatal("explicit completeness obligation should keep an architecture/member-list hybrid out of narrative-only lane")
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
	if !IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("bare cross-component breadth without subtopics should still stay in the single-topic mechanism lane")
	}

	rm.SubTopics = []SubTopic{
		{Summary: "first branch", Entities: []string{"First"}},
		{Summary: "second branch", Entities: []string{"Second"}},
	}
	if !IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("analyzer sub-topic decomposition alone must not turn one mechanism question into architecture/comparison")
	}

	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "cover every branch"}
	if !CompletenessObligationIsMechanismCoverageOnly(rm) {
		t.Fatal("typed mechanism completeness should be treated as coverage, not a principal member-set boundary")
	}
	if !IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("coverage-only completeness must not turn one mechanism explanation into an enumeration slate")
	}

	rm.Predicates.IsCategoryEnumeration = true
	if CompletenessObligationIsMechanismCoverageOnly(rm) {
		t.Fatal("category-enumeration completeness is a principal member-set boundary, not coverage-only")
	}
	if IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("category-enumeration shape must not stay in the lightweight mechanism lane")
	}
	rm.Predicates.IsCategoryEnumeration = false

	rm.Predicates.IsCrossComponent = false
	rm.SubTopics = nil
	rm.CompletenessObligation = nil
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
		// 2026-05-16 fix: CJK display labels were silently accepted
		// because Go's unicode.IsLetter is true for CJK ideographs. The
		// citation-alignment oracle then demanded these labels name a
		// symbol at the cited file:line, which no Chinese-only label
		// can satisfy. Pure-CJK and CJK-mixed labels are display prose
		// in this codebase.
		"引用锚定",   // citation grounding
		"自审查机制",  // self-review mechanism
		"质量门",    // quality gate
		"Foo 函数", // mixed: code symbol + CJK prose
		"Привет", // Cyrillic
		"αβγ",    // Greek
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
