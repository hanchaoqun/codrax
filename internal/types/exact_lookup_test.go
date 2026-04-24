package types

import (
	"reflect"
	"testing"
)

func TestBuildExactResolutionContract_ConfigKey(t *testing.T) {
	rm := RequestModel{
		Scenario: ScenarioConfigTrace,
		AnalyzerHints: AnalyzerHints{
			Kind:            "config_mapping",
			PrimaryEntities: []string{"explore_mid_loop_hint_budget"},
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

func TestExactResolutionScopeTerms(t *testing.T) {
	contract := &ExactResolutionContract{
		TargetKind:           SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		RelatedContextPolicy: ExactContextSameFamilyGrounded,
	}
	if got := ExactResolutionScopeTerms(contract); !reflect.DeepEqual(got, []string{"explore"}) {
		t.Fatalf("scope terms = %v, want [explore]", got)
	}
}
