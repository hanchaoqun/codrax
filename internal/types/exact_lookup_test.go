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

func TestBuildExactResolutionContract_ConfigKeyStaysNilWithoutExplicitExactTargets(t *testing.T) {
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

	if got := BuildExactResolutionContract(rm); got != nil {
		t.Fatalf("contract = %+v, want nil when multiple mentioned entities remain and analyzer did not disambiguate exact_targets", got)
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
	if !reflect.DeepEqual(got.RelatedContextTerms, []string{"budget", "explore"}) {
		t.Fatalf("RelatedContextTerms = %v, want keyword-grounded fallback terms", got.RelatedContextTerms)
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
