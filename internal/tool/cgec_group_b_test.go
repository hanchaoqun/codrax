package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDetectSubjectShapeMismatch_LiteralSubjects is the B2b pure-
// function regression: every source-code literal AnswerSubject kind
// paired with a non-literal AnswerShape MUST report mismatch, with
// the target shape always ShapeValue. Non-literal subjects and
// literal-friendly shapes return (false, "", "").
func TestDetectSubjectShapeMismatch_LiteralSubjects(t *testing.T) {
	literalKinds := []types.AnswerSubjectKind{
		types.SubjectSkillName,
		types.SubjectAgentName,
		types.SubjectFunctionName,
		types.SubjectTypeName,
		types.SubjectInterface,
		types.SubjectHandlerRoute,
		types.SubjectReturnValue,
	}
	badShapes := []types.AnswerShape{
		types.ShapeExplanation,
		types.ShapeStepList,
		types.ShapeBoolean,
		types.ShapeListOfSymbols,
	}
	for _, subj := range literalKinds {
		for _, shape := range badShapes {
			ir := &types.AnalysisIR{}
			ir.RequestModel.AnswerSubject.Kind = subj
			ir.AnswerContract.RequiredAnswerShape = shape
			m, from, to := detectSubjectShapeMismatch(ir)
			if !m {
				t.Errorf("subj=%s shape=%s: expected mismatch", subj, shape)
				continue
			}
			if from != shape {
				t.Errorf("subj=%s shape=%s: from=%s, want %s", subj, shape, from, shape)
			}
			if to != types.ShapeValue {
				t.Errorf("subj=%s shape=%s: to=%s, want ShapeValue", subj, shape, to)
			}
		}
	}
}

// TestDetectSubjectShapeMismatch_GoodShapes asserts that when the
// declared shape is already ShapeValue or ShapeConfigValue (the
// only two literal-carrying shapes), no mismatch is reported even
// for literal subjects.
func TestDetectSubjectShapeMismatch_GoodShapes(t *testing.T) {
	for _, shape := range []types.AnswerShape{types.ShapeValue, types.ShapeConfigValue} {
		ir := &types.AnalysisIR{}
		ir.RequestModel.AnswerSubject.Kind = types.SubjectSkillName
		ir.AnswerContract.RequiredAnswerShape = shape
		if m, _, _ := detectSubjectShapeMismatch(ir); m {
			t.Errorf("shape=%s must not report mismatch", shape)
		}
	}
}

// TestDetectSubjectShapeMismatch_NonLiteralSubjects asserts that
// non-literal subjects (free_text, unknown, ...) do not trigger
// RepairSwapShape even against non-value shapes.
func TestDetectSubjectShapeMismatch_NonLiteralSubjects(t *testing.T) {
	for _, subj := range []types.AnswerSubjectKind{types.SubjectUnknown, types.SubjectGeneric, types.SubjectConfigKey} {
		ir := &types.AnalysisIR{}
		ir.RequestModel.AnswerSubject.Kind = subj
		ir.AnswerContract.RequiredAnswerShape = types.ShapeExplanation
		if m, _, _ := detectSubjectShapeMismatch(ir); m {
			t.Errorf("subj=%s must not trigger mismatch", subj)
		}
	}
}

// TestDetectSubjectShapeMismatch_NilIR asserts no crash on nil IR
// (the caller path in preCompleteContractCheck has an IR guard too
// but belt-and-suspenders).
func TestDetectSubjectShapeMismatch_NilIR(t *testing.T) {
	if m, _, _ := detectSubjectShapeMismatch(nil); m {
		t.Error("nil IR must not trigger mismatch")
	}
}
