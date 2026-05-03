package patcher

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSubmit_RejectsNonWhitelistedField(t *testing.T) {
	e := New(Budget{})
	rec := e.Submit(PatchRequest{Field: "random_field", Operation: OpReplace, NewValue: "x"})
	if rec.Applied {
		t.Errorf("non-whitelist field should NOT apply, got %+v", rec)
	}
	if rec.SkipReason != "field_not_whitelisted" {
		t.Errorf("SkipReason = %q, want field_not_whitelisted", rec.SkipReason)
	}
}

func TestSubmit_AppliesWhitelistedReplace(t *testing.T) {
	e := New(Budget{})
	rec := e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "value"})
	if !rec.Applied {
		t.Errorf("whitelisted replace should apply, got %+v", rec)
	}
	if e.AppliedCount() != 1 {
		t.Errorf("AppliedCount = %d, want 1", e.AppliedCount())
	}
}

func TestSubmit_IdempotentOnRepeatValue(t *testing.T) {
	e := New(Budget{})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "value"})
	rec2 := e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "value"})
	if rec2.Applied {
		t.Errorf("repeat of same value should be idempotent, got Applied=true")
	}
	if rec2.SkipReason != "idempotent" {
		t.Errorf("SkipReason = %q, want idempotent", rec2.SkipReason)
	}
}

func TestSubmit_MonotonicRejectsRevert(t *testing.T) {
	e := New(Budget{MaxPatchesPerRun: 10, MaxPatchesPerField: 10})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "value"})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "explanation"})
	// Revert back to the original — should hit the monotonic gate
	// BEFORE the per-field budget (we set a large budget above).
	rec := e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "value"})
	if rec.Applied {
		t.Errorf("revert to earlier value should be rejected")
	}
	if rec.SkipReason != "monotonic_violation" {
		t.Errorf("SkipReason = %q, want monotonic_violation", rec.SkipReason)
	}
}

func TestSubmit_PerFieldBudgetExhausted(t *testing.T) {
	e := New(Budget{MaxPatchesPerRun: 10, MaxPatchesPerField: 2})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "value"})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "explanation"})
	rec := e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "step_list"})
	if rec.Applied {
		t.Errorf("third patch on same field should hit per-field cap")
	}
	if rec.SkipReason != "budget_exhausted_field" {
		t.Errorf("SkipReason = %q, want budget_exhausted_field", rec.SkipReason)
	}
}

func TestSubmit_RunBudgetExhausted(t *testing.T) {
	e := New(Budget{MaxPatchesPerRun: 2, MaxPatchesPerField: 10})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "value"})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "return_value"})
	rec := e.Submit(PatchRequest{Field: "answer_subject.kind", Operation: OpReplace, NewValue: "skill_name"})
	if rec.Applied {
		t.Errorf("third patch should hit MaxPatchesPerRun")
	}
	if rec.SkipReason != "budget_exhausted_run" {
		t.Errorf("SkipReason = %q, want budget_exhausted_run", rec.SkipReason)
	}
}

func TestApply_MutatesRequestModelFields(t *testing.T) {
	e := New(Budget{})
	e.Submit(PatchRequest{Field: "answer_subject.kind", Operation: OpReplace, NewValue: types.SubjectFunctionName})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "return_value"})

	rm := &types.RequestModel{}
	e.Apply(rm)

	if rm.AnswerSubject.Kind != types.SubjectFunctionName {
		t.Errorf("AnswerSubject.Kind = %q, want function_name", rm.AnswerSubject.Kind)
	}
	if rm.AnalyzerHints.Kind != "return_value" {
		t.Errorf("AnalyzerHints.Kind = %q, want return_value", rm.AnalyzerHints.Kind)
	}
}

func TestApply_NilRequestModelSafe(t *testing.T) {
	e := New(Budget{})
	e.Submit(PatchRequest{Field: "question_kind", Operation: OpReplace, NewValue: "value"})
	// Must not panic.
	e.Apply(nil)
}

func TestIsFieldWhitelisted_Sanity(t *testing.T) {
	if !IsFieldWhitelisted("question_kind") {
		t.Errorf("question_kind must be whitelisted")
	}
	if IsFieldWhitelisted("arbitrary_field") {
		t.Errorf("unknown field must NOT be whitelisted")
	}
}
