package tool

import (
	"strings"
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
		types.SubjectFunctionName,
		types.SubjectTypeName,
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
		ir.RequestModel.AnswerSubject.Kind = types.SubjectFunctionName
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

// TestSimulateCitationGrounding_AllMiss_Rejects is the G1 primary
// regression: when every citation file sits outside the readFiles
// whitelist, the dry-run returns a non-empty message so the caller
// can short-circuit the real grounder.
func TestSimulateCitationGrounding_AllMiss_Rejects(t *testing.T) {
	citations := []types.Citation{
		{File: "internal/agent/subagent.go", Line: 63},
		{File: "internal/agent/agent.go", Line: 919},
	}
	readFiles := map[string]bool{
		"cmd/root.go":               true,
		"internal/skill/defaults.go": true,
	}
	msg := simulateCitationGrounding(citations, readFiles, nil, nil)
	if msg == "" {
		t.Fatal("expected rejection message, got empty")
	}
	for _, want := range []string{
		"REJECTED",
		"subagent.go:63",
		"agent.go:919",
		"cmd/root.go",
		"internal/skill/defaults.go",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

// TestSimulateCitationGrounding_AtLeastOneMatch_Passes asserts that
// when AT LEAST one citation is in the whitelist, the dry-run
// returns empty string — deferring the decision to the real
// grounder, which will drop the bad citations per-entry but keep
// the good ones.
func TestSimulateCitationGrounding_AtLeastOneMatch_Passes(t *testing.T) {
	citations := []types.Citation{
		{File: "internal/agent/ghost.go", Line: 1},
		{File: "internal/skill/defaults.go", Line: 14},
	}
	readFiles := map[string]bool{
		"internal/skill/defaults.go": true,
	}
	if got := simulateCitationGrounding(citations, readFiles, nil, nil); got != "" {
		t.Errorf("1 match out of 2 must pass dry-run, got:\n%s", got)
	}
}

// TestSimulateCitationGrounding_EmptyInputs_Passes asserts the
// dry-run accepts empty citations / empty readFiles (sub-agent,
// test paths, absence answers) without emitting an error.
func TestSimulateCitationGrounding_EmptyInputs_Passes(t *testing.T) {
	if got := simulateCitationGrounding(nil, map[string]bool{"a.go": true}, nil, nil); got != "" {
		t.Errorf("empty citations must pass, got %q", got)
	}
	if got := simulateCitationGrounding([]types.Citation{{File: "a.go", Line: 1}}, nil, nil, nil); got != "" {
		t.Errorf("empty readFiles must pass (skip whitelist), got %q", got)
	}
}
