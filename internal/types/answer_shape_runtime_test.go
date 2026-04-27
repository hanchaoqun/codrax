package types

import "testing"

func TestExactResolutionTargetIsConfigKey(t *testing.T) {
	if !ExactResolutionTargetIsConfigKey(&ExactResolutionContract{TargetKind: SubjectConfigKey}) {
		t.Fatal("target kind=config_key should be recognized")
	}
	if !ExactResolutionTargetIsConfigKey(&ExactResolutionContract{TargetLabel: "config key"}) {
		t.Fatal("target label=config key should be recognized")
	}
	if ExactResolutionTargetIsConfigKey(&ExactResolutionContract{TargetKind: SubjectFunctionName, TargetLabel: "symbol"}) {
		t.Fatal("non-config contract should not be treated as config key")
	}
}

func TestStableAbsentExactConfigRequiresExplanation(t *testing.T) {
	ir := &AnalysisIR{
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeConfigValue,
			ExactResolution: &ExactResolutionContract{
				TargetKind:   SubjectConfigKey,
				AllowAbsence: true,
			},
		},
	}
	m := NewMutableState("")
	m.SetInvestigationResultKind("absence")
	m.SetAbsenceJustification("repo-wide search found no exact key")
	if !StableAbsentExactConfigRequiresExplanation(ir, m) {
		t.Fatal("stable absent exact config target should require explanation")
	}
	if got := EffectiveRequiredAnswerShape(ir, m); got != ShapeExplanation {
		t.Fatalf("EffectiveRequiredAnswerShape = %s, want explanation", got)
	}
}

func TestExplanationAllowsAnchorSkeleton(t *testing.T) {
	ir := &AnalysisIR{
		AnswerContract: AnswerContract{RequiredAnswerShape: ShapeExplanation},
		RequestModel: RequestModel{
			SubTopics: []SubTopic{{Summary: "one"}},
		},
	}
	if !ExplanationAllowsAnchorSkeleton(ir) {
		t.Fatal("multi-topic explanation should allow anchor skeleton")
	}

	ir.RequestModel.SubTopics = nil
	if ExplanationAllowsAnchorSkeleton(ir) {
		t.Fatal("single-topic explanation should not allow anchor skeleton")
	}
}

func TestEffectiveRequiredAnswerShape_PreservesConfiguredShapeWithoutStableAbsence(t *testing.T) {
	ir := &AnalysisIR{
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeConfigValue,
			ExactResolution: &ExactResolutionContract{
				TargetKind:   SubjectConfigKey,
				AllowAbsence: true,
			},
		},
	}
	m := NewMutableState("")
	if got := EffectiveRequiredAnswerShape(ir, m); got != ShapeConfigValue {
		t.Fatalf("EffectiveRequiredAnswerShape = %s, want config_value", got)
	}
}
