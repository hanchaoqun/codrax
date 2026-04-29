package types

import (
	"reflect"
	"testing"
)

func TestBuildExactResolutionContract_ConfigKey(t *testing.T) {
	rm := RequestModel{
		RawRequest: "where is explore_mid_loop_hint_budget defined",
		Scenario:   ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind:            "config_mapping",
			PrimaryEntities: []string{"explore_mid_loop_hint_budget"},
			ExactTargets:    []string{"explore_mid_loop_hint_budget"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil")
	}
	if got.TargetLabel != "config key" {
		t.Fatalf("TargetLabel = %q, want config key", got.TargetLabel)
	}
	if !reflect.DeepEqual(got.Targets, []string{"explore_mid_loop_hint_budget"}) {
		t.Fatalf("Targets = %v, want exact config key", got.Targets)
	}
	if got.RelatedContextPolicy != ExactContextSameFamilyGrounded {
		t.Fatalf("RelatedContextPolicy = %q, want %q", got.RelatedContextPolicy, ExactContextSameFamilyGrounded)
	}
}

func TestBuildExactResolutionContract_PrefersRawRequestMentionedTargets(t *testing.T) {
	rm := RequestModel{
		RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
		Scenario:   ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind: "config_mapping",
			PrimaryEntities: []string{
				"explore_mid_loop_hint_budget",
				"ExploreMidLoopMinIteration",
				"DefaultExploreHeuristics",
			},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil")
	}
	if !reflect.DeepEqual(got.Targets, []string{"explore_mid_loop_hint_budget"}) {
		t.Fatalf("Targets = %v, want only raw-request-mentioned target", got.Targets)
	}
}

func TestBuildExactResolutionContract_ConfigKeyFiltersOutFileContextWithoutExplicitExactTargets(t *testing.T) {
	rm := RequestModel{
		RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？给我 code default / codrax.yaml / CLI 三层的覆盖优先级。",
		Scenario:   ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind: "config_mapping",
			PrimaryEntities: []string{
				"explore_mid_loop_hint_budget",
				"codrax.yaml",
			},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil after filtering file-context mentions")
	}
	if !reflect.DeepEqual(got.Targets, []string{"explore_mid_loop_hint_budget"}) {
		t.Fatalf("Targets = %v, want only the exact config key target", got.Targets)
	}
}

func TestBuildExactResolutionContract_ConfigKeyUsesExplicitExactTargets(t *testing.T) {
	rm := RequestModel{
		RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？给我 code default / codrax.yaml / CLI 三层的覆盖优先级。",
		Scenario:   ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind: "config_mapping",
			PrimaryEntities: []string{
				"explore_mid_loop_hint_budget",
				"codrax.yaml",
			},
			ExactTargets: []string{"explore_mid_loop_hint_budget"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil")
	}
	if !reflect.DeepEqual(got.Targets, []string{"explore_mid_loop_hint_budget"}) {
		t.Fatalf("Targets = %v, want exact target only", got.Targets)
	}
}

func TestBuildExactResolutionContract_ConfigTraceFiltersPathLikeExactTargetsEvenWhenAnswerSubjectDrifts(t *testing.T) {
	rm := RequestModel{
		RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？给我 code default / codrax.yaml / CLI 三层的覆盖优先级。",
		Scenario:   ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind: "config_mapping",
			PrimaryEntities: []string{
				"explore_mid_loop_hint_budget",
				"codrax.yaml",
			},
			ExactTargets: []string{"explore_mid_loop_hint_budget", "codrax.yaml"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectNumeric},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil")
	}
	if !reflect.DeepEqual(got.Targets, []string{"explore_mid_loop_hint_budget"}) {
		t.Fatalf("Targets = %v, want only the exact config key target after semantic subject filtering", got.Targets)
	}
}

func TestBuildExactResolutionContract_PersistsValidatedContextTerms(t *testing.T) {
	rm := RequestModel{
		RawRequest: "explore_mid_loop_hint_budget 鐨勬渶缁堟湁鏁堝€兼槸鎬庝箞璁＄畻鍑烘潵鐨勶紵",
		Scenario:   ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind:              "config_mapping",
			PrimaryEntities:   []string{"explore_mid_loop_hint_budget"},
			ExactTargets:      []string{"explore_mid_loop_hint_budget"},
			ExactContextTerms: []string{"explore"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil")
	}
	if !reflect.DeepEqual(got.RelatedContextTerms, []string{"explore"}) {
		t.Fatalf("RelatedContextTerms = %v, want [explore]", got.RelatedContextTerms)
	}
}

func TestBuildExactResolutionContract_FallsBackToValidatedKeywordContextTerms(t *testing.T) {
	rm := RequestModel{
		RawRequest: "explore_mid_loop_hint_budget 鐨勬渶缁堟湁鏁堝€兼槸鎬庝箞璁＄畻鍑烘潵鐨勶紵",
		Scenario:   ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind:            "config_mapping",
			Keywords:        []string{"explore", "override", "budget"},
			PrimaryEntities: []string{"explore_mid_loop_hint_budget"},
			ExactTargets:    []string{"explore_mid_loop_hint_budget"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil")
	}
	if !reflect.DeepEqual(got.RelatedContextTerms, []string{"explore"}) {
		t.Fatalf("RelatedContextTerms = %v, want family-root fallback terms", got.RelatedContextTerms)
	}
}

func TestBuildExactResolutionContract_FallsBackToTargetIdentifierTerms(t *testing.T) {
	rm := RequestModel{
		RawRequest: "FooHandler 在哪里定义？",
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities: []string{"FooHandler"},
			ExactTargets:    []string{"FooHandler"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil")
	}
	if !reflect.DeepEqual(got.RelatedContextTerms, []string{"foo", "handler"}) {
		t.Fatalf("RelatedContextTerms = %v, want lexical fallback terms", got.RelatedContextTerms)
	}
}

func TestBuildExactResolutionContract_DoesNotPromoteUnmentionedPrimaryEntity(t *testing.T) {
	rm := RequestModel{
		RawRequest: "why is this setting ignored?",
		Scenario:   ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind:            "config_mapping",
			PrimaryEntities: []string{"explore_mid_loop_hint_budget"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
	}

	if got := BuildExactResolutionContract(rm); got != nil {
		t.Fatalf("contract = %+v, want nil when target is not explicitly mentioned", got)
	}
}

func TestBuildExactResolutionContract_RemainsNilWhenMultipleSubjectCompatibleMentionsRemain(t *testing.T) {
	rm := RequestModel{
		RawRequest: "Foo 和 Bar 哪个才是最终调用点？",
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities: []string{"Foo", "Bar"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
	}

	if got := BuildExactResolutionContract(rm); got != nil {
		t.Fatalf("contract = %+v, want nil when multiple subject-compatible mentions remain", got)
	}
}

func TestExactResolutionProofAnchorMatchesAnyTarget_IgnoresObjectOnlyMentions(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:  SubjectConfigKey,
		TargetLabel: "config key",
		Targets:     []string{"explore_mid_loop_hint_budget"},
	}
	if ExactResolutionDirectAnchorMatchesAnyTarget(contract, "ResolvedExploreHeuristics", "ResolvedExploreHeuristics", "defaults merge excludes explore_mid_loop_hint_budget") {
		t.Fatal("direct anchor match should ignore object-only target mentions")
	}
	if ExactResolutionProofAnchorMatchesAnyTarget(contract, "ResolvedExploreHeuristics", "ResolvedExploreHeuristics", "defaults merge excludes explore_mid_loop_hint_budget") {
		t.Fatal("proof anchor match should ignore object-only target mentions")
	}
	if !ExactResolutionProofAnchorMatchesAnyTarget(contract, "explore_mid_loop_hint_budget", "", "") {
		t.Fatal("proof anchor match should still allow subject-only target mentions when no anchor symbol is present")
	}
}

func TestBuildExactResolutionContract_FunctionNameFiltersOutPathContextMention(t *testing.T) {
	rm := RequestModel{
		RawRequest: "buildAnalysisIR 在 internal/agent/analyzer.go 里定义在哪里？",
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities: []string{"buildAnalysisIR", "internal/agent/analyzer.go"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsScalarAnswer: true,
		},
	}

	got := BuildExactResolutionContract(rm)
	if got == nil {
		t.Fatal("contract = nil, want non-nil after filtering file-path context")
	}
	if !reflect.DeepEqual(got.Targets, []string{"buildAnalysisIR"}) {
		t.Fatalf("Targets = %v, want only the function target", got.Targets)
	}
}

func TestMentionedAndDerivedEntitiesFromRawRequest(t *testing.T) {
	candidates := []string{
		"explore_mid_loop_hint_budget",
		"DefaultExploreHeuristics",
		"ExploreMidLoopMinIteration",
	}
	mentioned := MentionedEntitiesFromRawRequest(
		"explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？", candidates)
	if !reflect.DeepEqual(mentioned, []string{"explore_mid_loop_hint_budget"}) {
		t.Fatalf("mentioned = %v, want only raw-request surface", mentioned)
	}
	derived := DerivedEntitiesFromMentioned(candidates, mentioned)
	if !reflect.DeepEqual(derived, []string{"DefaultExploreHeuristics", "ExploreMidLoopMinIteration"}) {
		t.Fatalf("derived = %v, want analyzer-derived context only", derived)
	}
}

func TestExactResolutionPendingTargets_ConfigKey(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:   SubjectConfigKey,
		TargetLabel:  "config key",
		Targets:      []string{"explore_mid_loop_hint_budget"},
		AllowAbsence: true,
	}
	finds := []UnverifiedFinding{{
		Token: "explore_mid_loop_hint_budget",
		Kind:  "symbol",
	}}
	got := ExactResolutionPendingTargets(contract, finds)
	if len(got) != 1 || got[0] != "explore_mid_loop_hint_budget" {
		t.Fatalf("pending targets = %v, want [explore_mid_loop_hint_budget]", got)
	}
	if !ExactResolutionTextMentionsTarget(contract, "仓库中不存在 explore_mid_loop_hint_budget 这个键", got[0]) {
		t.Fatalf("text mention check failed for %q", got[0])
	}
}

func TestExactResolutionPendingTargets_FilePath(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:   SubjectFilePath,
		TargetLabel:  "file path",
		Targets:      []string{`internal\agent\answer_document_evaluator.go`},
		AllowAbsence: true,
	}
	finds := []UnverifiedFinding{{
		Token: "internal/agent/answer_document_evaluator.go",
		Kind:  "path",
	}}
	got := ExactResolutionPendingTargets(contract, finds)
	if len(got) != 1 || got[0] != `internal\agent\answer_document_evaluator.go` {
		t.Fatalf("pending targets = %v, want exact file path", got)
	}
	if !ExactResolutionTextMentionsTarget(contract, "missing file internal/agent/answer_document_evaluator.go in this branch", got[0]) {
		t.Fatalf("path mention check failed for %q", got[0])
	}
}

func TestBuildExactResolutionContract_DoesNotTriggerOnEnumeration(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities: []string{"Explorer", "Finalizer"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsScalarAnswer: false,
		},
	}
	if got := BuildExactResolutionContract(rm); got != nil {
		t.Fatalf("contract = %+v, want nil for broad enumeration", got)
	}
}

func TestBuildExactResolutionContract_DoesNotPromoteSecondaryMentionOverPrimaryRoleEntity(t *testing.T) {
	rm := RequestModel{
		RawRequest: "仓库里负责解析用户请求并产出结构化 AnalysisIR 的那个入口函数叫什么？",
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities:   []string{"buildAnalysisIR"},
			MentionedEntities: []string{"AnalysisIR"},
			ExactTargets:      []string{"AnalysisIR"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsScalarAnswer: true,
		},
	}
	if got := BuildExactResolutionContract(rm); got != nil {
		t.Fatalf("contract = %+v, want nil when the mentioned output type is not the primary role entity", got)
	}
}

func TestBuildExactResolutionContract_DisablesImplicitTargetForLatentReturnLookup(t *testing.T) {
	rm := RequestModel{
		RawRequest: "仓库里负责解析用户请求并产出结构化 AnalysisIR 的那个入口函数具体叫什么名字？在哪个文件？",
		AnalyzerHints: AnalyzerHints{
			Kind:            "return_value",
			PrimaryEntities: []string{"AnalysisIR"},
			Entities:        []string{"AnalysisIR"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		PredicateAxis: AxisReturn,
		Predicates: SemanticPredicates{
			IsScalarAnswer: true,
		},
	}
	if got := BuildExactResolutionContract(rm); got != nil {
		t.Fatalf("contract = %+v, want nil for latent role lookup without explicit exact_targets", got)
	}
}

func TestBuildExactResolutionContract_RequiresExplicitDisambiguationForMultiPrimaryScalarLookup(t *testing.T) {
	rm := RequestModel{
		RawRequest: "仓库里负责解析用户请求并产出结构化 AnalysisIR 的那个入口函数具体叫什么名字？在哪个文件？",
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities: []string{"AnalysisIR", "buildAnalysisIR"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsScalarAnswer: true,
		},
	}
	if got := BuildExactResolutionContract(rm); got != nil {
		t.Fatalf("contract = %+v, want nil when multiple primary entities remain and analyzer did not set exact_targets", got)
	}
}

func TestExactResolutionContextTerms(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	if got := ExactResolutionContextTerms(contract); !reflect.DeepEqual(got, []string{"explore"}) {
		t.Fatalf("context terms = %v, want [explore]", got)
	}
}

func TestExactResolutionSameFamilyMatchScore_ConfigKey(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	if got := ExactResolutionSameFamilyMatchScore(contract, "DefaultExploreHeuristics internal/types/config.go"); got <= 0 {
		t.Fatalf("match score for DefaultExploreHeuristics = %d, want > 0", got)
	}
	if got := ExactResolutionSameFamilyMatchScore(contract, "DefaultLoopPolicy internal/agent/loop_policy.go"); got != 0 {
		t.Fatalf("match score for DefaultLoopPolicy = %d, want 0", got)
	}
	if got := ExactResolutionSameFamilyMatchScore(contract, "postPrimaryReadMidLoopSignal internal/agent/explorer.go"); got != 0 {
		t.Fatalf("match score for explorer.go helper = %d, want 0", got)
	}
	if got := ExactResolutionSameFamilyMatchScore(contract, "AgentLoopMaxMidLoopInjects internal/config/runtime.go"); got != 0 {
		t.Fatalf("match score for AgentLoopMaxMidLoopInjects = %d, want 0 for a different config family", got)
	}
}

func TestConfigTraceGroundedContextAnchorAllowed_RejectsAuxiliaryAbsenceSupport(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:   SubjectConfigKey,
		TargetLabel:  "config key",
		Targets:      []string{"explore_mid_loop_hint_budget"},
		AllowAbsence: true,
	}
	item := EvidenceItem{
		Source:          "internal/skill/analysis_contract.go",
		LineStart:       367,
		ContextRole:     EvidenceContextRoleAbsenceSupport,
		GroundingStatus: GroundingGrounded,
	}
	if ConfigTraceGroundedContextAnchorAllowed(contract, item) {
		t.Fatal("auxiliary/doc absence-support evidence should not be answer-grade config-trace context")
	}
}

func TestExactResolutionHasDefiningTargetProof_RequiresProofGradeAnchor(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:   SubjectConfigKey,
		TargetLabel:  "config key",
		Targets:      []string{"explore_mid_loop_hint_budget"},
		AllowAbsence: true,
	}
	rawMention := EvidenceItem{
		Source:          "internal/config/runtime.go",
		LineStart:       271,
		Subject:         "RuntimeSettings",
		Predicate:       "documents",
		Object:          "explore_mid_loop_hint_budget",
		ContextRole:     EvidenceContextRoleUnknown,
		GroundingStatus: GroundingGrounded,
	}
	if ExactResolutionHasDefiningTargetProof(contract, []EvidenceItem{rawMention}) {
		t.Fatal("raw surface mention without a structured anchor must not satisfy defining target proof")
	}
	proof := rawMention
	proof.AnchorKind = AnchorAssignment
	proof.AnchorSymbol = "explore_mid_loop_hint_budget"
	proof.ContextRole = EvidenceContextRoleDefining
	if !ExactResolutionHasDefiningTargetProof(contract, []EvidenceItem{proof}) {
		t.Fatal("structured anchored proof should satisfy defining target proof")
	}
}

func TestExactResolutionEvidenceBlocksAbsence_RequiresProofGradeAnchor(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:   SubjectConfigKey,
		TargetLabel:  "config key",
		Targets:      []string{"explore_mid_loop_hint_budget"},
		AllowAbsence: true,
	}
	rawMention := EvidenceItem{
		Source:          "internal/config/runtime.go",
		LineStart:       271,
		Subject:         "RuntimeSettings",
		Predicate:       "documents",
		Object:          "explore_mid_loop_hint_budget",
		ContextRole:     EvidenceContextRoleUnknown,
		GroundingStatus: GroundingGrounded,
	}
	if ExactResolutionEvidenceBlocksAbsence(contract, rawMention) {
		t.Fatal("raw surface mention without a structured anchor must not block absence")
	}
	proof := rawMention
	proof.AnchorKind = AnchorAssignment
	proof.AnchorSymbol = "explore_mid_loop_hint_budget"
	proof.ContextRole = EvidenceContextRoleDefining
	proof.Predicate = "defines"
	if !ExactResolutionEvidenceBlocksAbsence(contract, proof) {
		t.Fatal("structured anchored exact-target proof should block absence")
	}
}

func TestExactResolutionEvidenceCanSatisfyRelatedContext_IgnoresSummaryOnlyTermMatches(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	item := EvidenceItem{
		Source:          "internal/types/config.go",
		Subject:         "LoopMaxMidLoopInjects",
		AnchorSymbol:    "LoopMaxMidLoopInjects",
		Summary:         "related context only for explore_* controls",
		ContextRole:     EvidenceContextRoleRelatedContext,
		GroundingStatus: GroundingGrounded,
	}
	if ExactResolutionEvidenceCanSatisfyRelatedContext(contract, item, []string{"internal/types/config.go"}) {
		t.Fatal("summary-only same-family wording should not satisfy related-context proof")
	}
}

func TestExactResolutionAnswerContextAnchorAllowedInFiles_RestrictsUngradedSameScopeContextToRequiredFiles(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	item := EvidenceItem{
		Source:          "internal/types/explore_budget.go",
		LineStart:       40,
		AnchorSymbol:    "ExploreBudget",
		ContextRole:     EvidenceContextRoleRelatedContext,
		GroundingStatus: GroundingGrounded,
	}
	if ExactResolutionAnswerContextAnchorAllowedInFiles(contract, ScenarioConfigTrace, true, item, []string{"internal/types/config.go"}) {
		t.Fatal("same-family related-context outside the current same-scope file set should not be answer-grade context")
	}
}

func TestConfigTraceGroundedContextAnchorAllowedInFiles_AllowsValidatedPrecedenceAnchorOutsideRequiredFiles(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	item := EvidenceItem{
		Source:          "codrax.yaml.example",
		LineStart:       25,
		Subject:         "ExploreHeuristics",
		AnchorSymbol:    "ExploreHeuristics",
		AnchorKind:      AnchorDefinition,
		ContextRole:     EvidenceContextRoleRelatedContext,
		DiagramRole:     EvidenceDiagramRoleYAML,
		GroundingStatus: GroundingRecovered,
	}
	if !ConfigTraceGroundedContextAnchorAllowedInFiles(contract, item, []string{"internal/types/config.go"}) {
		t.Fatal("validated precedence anchors should stay diagram-grade even when same-scope context is narrowed to other files")
	}
}

func TestLooksLikeConfigFilePath_AcceptsCommonConfigExtensionsAndChains(t *testing.T) {
	for _, path := range []string{
		"codrax.yaml",
		"codrax.yaml.example",
		"settings.json",
		"settings.json5",
		"settings.json.sample",
		"pyproject.toml",
		"service.ini",
		"local.properties",
	} {
		if !LooksLikeConfigFilePath(path) {
			t.Fatalf("LooksLikeConfigFilePath(%q)=false, want true", path)
		}
	}
	for _, path := range []string{
		"internal/types/config.go",
		"docs/configuration.md",
		"README",
	} {
		if LooksLikeConfigFilePath(path) {
			t.Fatalf("LooksLikeConfigFilePath(%q)=true, want false", path)
		}
	}
}

func TestLooksLikeWrappedConfigFilePath_DetectsSecondaryConfigSurfaces(t *testing.T) {
	for _, path := range []string{
		"codrax.yaml.example",
		"settings.json.sample",
		"service.toml.template",
	} {
		if !LooksLikeWrappedConfigFilePath(path) {
			t.Fatalf("LooksLikeWrappedConfigFilePath(%q)=false, want true", path)
		}
	}
	for _, path := range []string{
		"codrax.yaml",
		"settings.json",
		"service.config.json",
		"internal/types/config.go",
	} {
		if LooksLikeWrappedConfigFilePath(path) {
			t.Fatalf("LooksLikeWrappedConfigFilePath(%q)=true, want false", path)
		}
	}
}

func TestExactResolutionRelatedContextProofAllowedInFiles_ConfigTraceRequiresValidatedPrecedenceAnchor(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	plainRelated := EvidenceItem{
		Source:          "internal/types/config.go",
		LineStart:       707,
		Subject:         "DefaultExploreHeuristics",
		AnchorSymbol:    "DefaultExploreHeuristics",
		AnchorKind:      AnchorDefinition,
		ContextRole:     EvidenceContextRoleRelatedContext,
		GroundingStatus: GroundingGrounded,
	}
	if ExactResolutionRelatedContextProofAllowedInFiles(contract, ScenarioConfigTrace, true, plainRelated, []string{"internal/types/config.go"}) {
		t.Fatal("config-trace closure should not accept same-scope related context without a validated precedence role")
	}
	precedenceRelated := plainRelated
	precedenceRelated.DiagramRole = EvidenceDiagramRoleDefault
	if !ExactResolutionRelatedContextProofAllowedInFiles(contract, ScenarioConfigTrace, true, precedenceRelated, []string{"internal/types/config.go"}) {
		t.Fatal("config-trace closure should accept a validated precedence anchor")
	}
}

func TestExactResolutionAbsenceClosureReady_AllowsRelatedContextOnlyWhenNoDefiningProof(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       815,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			GroundingStatus: GroundingGrounded,
			GroundingTier:   TierLineText,
		},
	}
	if !ExactResolutionAbsenceClosureReady(contract, ScenarioConfigTrace, contract.Targets, evidence, []string{"internal/types/config.go"}) {
		t.Fatal("validated same-scope related-context anchor with no exact defining proof should allow exact-absence closure")
	}
}

func TestExactResolutionAbsenceClosureReady_ConfigTraceNeedsTwoRolesWhenMultipleScopeFilesRemain(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       815,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			GroundingStatus: GroundingGrounded,
			GroundingTier:   TierLineText,
		},
	}
	if ExactResolutionAbsenceClosureReady(contract, ScenarioConfigTrace, contract.Targets, evidence, []string{"internal/types/config.go", "internal/config/runtime.go"}) {
		t.Fatal("single-role config-trace context should not close exact absence while multi-file precedence scope remains")
	}
	evidence = append(evidence, EvidenceItem{
		Kind:            EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       231,
		Subject:         "ExploreMidLoopMinIteration",
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "ExploreMidLoopMinIteration",
		ContextRole:     EvidenceContextRoleRelatedContext,
		DiagramRole:     EvidenceDiagramRoleRuntime,
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	})
	if !ExactResolutionAbsenceClosureReady(contract, ScenarioConfigTrace, contract.Targets, evidence, []string{"internal/types/config.go", "internal/config/runtime.go"}) {
		t.Fatal("two validated precedence roles should allow config-trace exact-absence closure")
	}
}

func TestBuildExactResolutionContract_PersistsRequestedContextRoles(t *testing.T) {
	rm := RequestModel{
		Scenario:      ScenarioConfigTrace,
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
		AnalyzerHints: AnalyzerHints{
			Kind:              "config_mapping",
			ExactTargets:      []string{"explore_mid_loop_hint_budget"},
			ExactContextRoles: []EvidenceDiagramRole{EvidenceDiagramRoleDefault, EvidenceDiagramRoleConfig, EvidenceDiagramRoleOverride},
		},
		RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？给我 code default / codrax.yaml / CLI 三层的覆盖优先级。",
	}
	contract := BuildExactResolutionContract(rm)
	if contract == nil {
		t.Fatal("contract = nil, want exact-resolution contract")
	}
	want := []EvidenceDiagramRole{EvidenceDiagramRoleDefault, EvidenceDiagramRoleConfig, EvidenceDiagramRoleOverride}
	if !reflect.DeepEqual(contract.RequestedContextRoles, want) {
		t.Fatalf("RequestedContextRoles = %v, want %v", contract.RequestedContextRoles, want)
	}
}

func TestExactResolutionAbsenceClosureReady_ConfigTraceRequiresRequestedRoles(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:            SubjectConfigKey,
		TargetLabel:           "config key",
		Targets:               []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:          true,
		RelatedContextPolicy:  ExactContextSameFamilyGrounded,
		RelatedContextTerms:   []string{"explore"},
		RequestedContextRoles: []EvidenceDiagramRole{EvidenceDiagramRoleDefault, EvidenceDiagramRoleConfig, EvidenceDiagramRoleOverride},
	}
	evidence := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       815,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			GroundingStatus: GroundingGrounded,
			GroundingTier:   TierLineText,
		},
		{
			Kind:            EvidenceDirect,
			Source:          "codrax.yaml.example",
			LineStart:       309,
			Subject:         "explore_midloop_min_iteration",
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "explore_midloop_min_iteration",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleConfig,
			GroundingStatus: GroundingGrounded,
			GroundingTier:   TierLineText,
		},
	}
	required := []string{"internal/types/config.go", "codrax.yaml.example", "cmd/root.go"}
	if ExactResolutionAbsenceClosureReady(contract, ScenarioConfigTrace, contract.Targets, evidence, required) {
		t.Fatal("missing requested override role should block exact-absence closure")
	}
	evidence = append(evidence, EvidenceItem{
		Kind:            EvidenceDirect,
		Source:          "cmd/root.go",
		LineStart:       120,
		Subject:         "ExploreMidLoopMinIteration",
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "ExploreMidLoopMinIteration",
		ContextRole:     EvidenceContextRoleRelatedContext,
		DiagramRole:     EvidenceDiagramRoleOverride,
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	})
	if !ExactResolutionAbsenceClosureReady(contract, ScenarioConfigTrace, contract.Targets, evidence, required) {
		t.Fatal("requested default/config/override coverage should allow exact-absence closure")
	}
}

func TestConfigTraceRelatedContextCitationCandidates_DedupesAndOrdersByPrecedenceRole(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	items := []EvidenceItem{
		{
			Source:          "internal/types/config.go",
			LineStart:       707,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			GroundingStatus: GroundingGrounded,
		},
		{
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			AnchorKind:      AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleRuntime,
			GroundingStatus: GroundingGrounded,
		},
		{
			Source:          "codrax.yaml.example",
			LineStart:       284,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "explore_midloop_min_iteration",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleConfig,
			GroundingStatus: GroundingRecovered,
		},
		{
			Source:          "codrax.yaml.example",
			LineStart:       284,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "explore_midloop_min_iteration",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleConfig,
			GroundingStatus: GroundingRecovered,
		},
		{
			Source:          "internal/types/explore_budget.go",
			LineStart:       40,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "ExploreBudget",
			ContextRole:     EvidenceContextRoleRelatedContext,
			GroundingStatus: GroundingGrounded,
		},
	}
	got := ConfigTraceRelatedContextCitationCandidates(contract, []string{"internal/types/config.go"}, items)
	if len(got) != 3 {
		t.Fatalf("candidate count = %d, want 3: %+v", len(got), got)
	}
	if got[0].Role != EvidenceDiagramRoleConfig || got[0].Source != "codrax.yaml.example" || got[0].Line != 284 {
		t.Fatalf("first candidate = %+v, want config precedence candidate first", got[0])
	}
	if got[1].Role != EvidenceDiagramRoleRuntime || got[1].Source != "internal/config/runtime.go" || got[1].Line != 231 {
		t.Fatalf("second candidate = %+v, want runtime precedence candidate second", got[1])
	}
	if got[2].Role != EvidenceDiagramRoleDefault || got[2].Source != "internal/types/config.go" || got[2].Line != 707 {
		t.Fatalf("third candidate = %+v, want default precedence candidate third", got[2])
	}
}

func TestConfigTraceValidatedDiagramRoleInFiles_RejectsRuntimeOnStaticDefinition(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"feature_timeout_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"feature"},
	}
	item := EvidenceItem{
		Source:          "internal/types/feature_budget.go",
		LineStart:       40,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "FeatureBudget",
		ContextRole:     EvidenceContextRoleRelatedContext,
		DiagramRole:     EvidenceDiagramRoleRuntime,
		GroundingStatus: GroundingGrounded,
	}
	if got := ConfigTraceValidatedDiagramRoleInFiles(contract, item, []string{"internal/types/config.go"}); got != EvidenceDiagramRoleUnknown {
		t.Fatalf("runtime role on static definition = %s, want unknown", got)
	}
}
