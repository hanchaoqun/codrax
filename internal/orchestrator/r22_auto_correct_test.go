// Package orchestrator — r22_auto_correct_test.go (2026-05-10).
//
// Fix-D of the analyzer-failure remediation: when the gate rejects
// an AnalysisIR purely because of the R2.2 longform_scalar_subject
// shape contradiction AND the analyzer LLM has had a chance to
// revise on retry (attempt ≥ 1) AND keeps emitting the same scalar
// AnswerSubject.Kind, the orchestrator auto-clears the scalar
// declaration and re-runs the gate.
package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === r22AutoCorrectShapeSubject ===

func TestR22AutoCorrect_NilIR_NoOp(t *testing.T) {
	if r22AutoCorrectShapeSubject(nil) {
		t.Error("nil IR: expected false; got true")
	}
}

func TestR22AutoCorrect_NotRejected_NoOp(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectStringLiteral
	ir.RequestModel.AnswerSubject.Confidence = 0.9
	ir.QualityGate.Rejected = false
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("not-rejected: expected false; got true")
	}
}

func TestR22AutoCorrect_RejectedButNotR22_NoOp(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectStringLiteral
	ir.RequestModel.AnswerSubject.Confidence = 0.9
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("non-R2.2 failure: expected false (don't broaden auto-correct surface)")
	}
}

func TestR22AutoCorrect_R22Fires_StringLiteralCleared(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectStringLiteral
	ir.RequestModel.AnswerSubject.Confidence = 0.9
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=root_cause_trace expects non-scalar payload but AnswerSubject.Kind=string_literal at confidence 0.90"},
	}
	if !r22AutoCorrectShapeSubject(ir) {
		t.Fatal("R2.2 + scalar string_literal: expected true (correction applied)")
	}
	if ir.RequestModel.AnswerSubject.Kind != types.SubjectUnknown {
		t.Errorf("kind should be cleared to Unknown; got %q", ir.RequestModel.AnswerSubject.Kind)
	}
	if ir.RequestModel.AnswerSubject.Confidence != 0 {
		t.Errorf("confidence should be cleared to 0; got %v", ir.RequestModel.AnswerSubject.Confidence)
	}
}

func TestR22AutoCorrect_R22Fires_NumericCleared(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectNumeric
	ir.RequestModel.AnswerSubject.Confidence = 0.7
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=architecture expects non-scalar payload but AnswerSubject.Kind=numeric at confidence 0.70"},
	}
	if !r22AutoCorrectShapeSubject(ir) {
		t.Fatal("R2.2 + scalar numeric: expected correction")
	}
}

func TestR22AutoCorrect_R22Fires_ReturnValueCleared(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectReturnValue
	ir.RequestModel.AnswerSubject.Confidence = 0.85
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=call_chain expects non-scalar payload but AnswerSubject.Kind=return_value at confidence 0.85"},
	}
	if !r22AutoCorrectShapeSubject(ir) {
		t.Fatal("R2.2 + scalar return_value: expected correction")
	}
}

func TestR22AutoCorrect_R22Fires_ButNonScalarKind_NoCorrection(t *testing.T) {
	// R2.2 wouldn't fire on a non-scalar kind in normal flow,
	// but defensively: if the IR somehow carries function_name
	// AND a R2.2 detail string (corrupted state), don't mutate
	// the kind — the caller's gate re-run will handle it.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectFunctionName
	ir.RequestModel.AnswerSubject.Confidence = 0.9
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("R2.2 + non-scalar kind: expected no correction (defensive)")
	}
}

func TestR22AutoCorrect_AlreadyUnknown_NoOp(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectUnknown
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("already Unknown: expected no correction (nothing to clear)")
	}
}

func TestR22AutoCorrect_R22ButPassedCheck_NoOp(t *testing.T) {
	// shape_subject_coherence appears as PASSED check — should
	// not trigger correction even though the Detail string
	// might mention R2.2 historically.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectStringLiteral
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: true, Detail: "family=generic subject_kind=string_literal scalar_pred=true sub_topics=0"},
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("shape_subject_coherence passed: expected no correction (different failure mode)")
	}
}

// === Resolver-omission safety guards (2026-05-10 audit follow-up) ===

func TestR22AutoCorrect_R22PlusSubtopicFail_NoOp(t *testing.T) {
	// CRITICAL safety: when R2.2 fires alongside a resolver-backed
	// coherence failure (R1.4 / R1.5 in subtopic_coherence), the
	// auto-correct must REFUSE. The post-correction re-run uses a
	// nil resolver, which makes R1.4/R1.5 no-ops — without this
	// guard, the coherence failure would be silently bypassed.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject = types.AnswerSubject{Kind: types.SubjectStringLiteral, Confidence: 0.9}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=root_cause_trace expects non-scalar payload but AnswerSubject.Kind=string_literal at confidence 0.90"},
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("R2.2 + R1.4: expected NO correction (would bypass resolver-backed R1.4 on re-run)")
	}
	// The IR's AnswerSubject must NOT have been mutated.
	if ir.RequestModel.AnswerSubject.Kind != types.SubjectStringLiteral {
		t.Errorf("expected scalar Kind preserved when correction refused; got %v", ir.RequestModel.AnswerSubject.Kind)
	}
}

func TestR22AutoCorrect_R22PlusContractFail_NoOp(t *testing.T) {
	// Same guard for non-coherence checks: contract_complete,
	// dag_closure, etc. Any other failed check blocks auto-correct
	// to keep the LLM-side retry path responsible for resolving
	// them. The fast-path re-run cannot validate non-R2.2 rules
	// without the original dispatch context.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject = types.AnswerSubject{Kind: types.SubjectNumeric, Confidence: 0.7}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: ..."},
		{Name: "contract_complete", Passed: false, Detail: "missing exact-resolution contract"},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("R2.2 + contract_complete fail: expected NO correction")
	}
}

func TestR22AutoCorrect_R22Solo_StillCorrects(t *testing.T) {
	// Sanity: when R2.2 is the SOLE failed check (other checks
	// either passed or aren't present), correction proceeds
	// normally — preserves the happy-path Fix-D contract.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject = types.AnswerSubject{Kind: types.SubjectStringLiteral, Confidence: 0.9}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: ..."},
		{Name: "coverage", Passed: true, Detail: "ok"},
		{Name: "dag_closure", Passed: true, Detail: "ok"},
	}
	if !r22AutoCorrectShapeSubject(ir) {
		t.Fatal("R2.2 sole-fail: expected correction to proceed")
	}
	if ir.RequestModel.AnswerSubject.Kind != types.SubjectUnknown {
		t.Errorf("kind should be cleared; got %v", ir.RequestModel.AnswerSubject.Kind)
	}
}
