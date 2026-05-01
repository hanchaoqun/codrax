package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestShouldReviewConsistency_ScalarShapeSkipped pins the gate:
// scalar-shape answers (Value / Boolean / ConfigValue) lack the
// dual-paragraph structure that intra-answer contradictions live
// in, so the reviewer is skipped for them.
func TestShouldReviewConsistency_ScalarShapeSkipped(t *testing.T) {
	long := strings.Repeat("a", 200)
	for _, shape := range []types.AnswerShape{
		types.ShapeValue, types.ShapeBoolean, types.ShapeConfigValue,
	} {
		doc := &types.AnswerDocument{
			Shape:   shape,
			Summary: long,
			Steps:   []types.AnswerStep{{Description: "1"}, {Description: "2"}, {Description: "3"}},
		}
		if shouldReviewConsistency(doc) {
			t.Errorf("shape=%s should skip consistency review", shape)
		}
	}
}

// TestShouldReviewConsistency_ShortSummarySkipped pins the floor:
// summary < 100 chars is too brief to host a meaningful summary↔body
// contradiction.
func TestShouldReviewConsistency_ShortSummarySkipped(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeStepList,
		Summary: "too short",
		Steps:   []types.AnswerStep{{Description: "1"}, {Description: "2"}, {Description: "3"}},
	}
	if shouldReviewConsistency(doc) {
		t.Error("short summary should skip review")
	}
}

// TestShouldReviewConsistency_FewBulletsSkipped pins the body
// floor: < 3 bullets means no real "list to disagree with summary".
func TestShouldReviewConsistency_FewBulletsSkipped(t *testing.T) {
	long := strings.Repeat("a", 200)
	doc := &types.AnswerDocument{
		Shape:   types.ShapeStepList,
		Summary: long,
		Steps:   []types.AnswerStep{{Description: "1"}, {Description: "2"}},
	}
	if shouldReviewConsistency(doc) {
		t.Error("< 3 bullets should skip review")
	}
}

// TestShouldReviewConsistency_HappyPathPasses pins the trigger:
// long summary + step_list shape + 3+ steps → reviewer runs.
func TestShouldReviewConsistency_HappyPathPasses(t *testing.T) {
	long := strings.Repeat("a", 200)
	doc := &types.AnswerDocument{
		Shape:   types.ShapeStepList,
		Summary: long,
		Steps: []types.AnswerStep{
			{Description: "step 1"},
			{Description: "step 2"},
			{Description: "step 3"},
		},
	}
	if !shouldReviewConsistency(doc) {
		t.Error("happy path should run reviewer")
	}
}

// TestRenderConsistencyReviewBody_FormatsBulletShape pins the
// rendered body shape so the reviewer LLM sees the finalizer's
// numbered/bulleted structure verbatim.
func TestRenderConsistencyReviewBody_FormatsBulletShape(t *testing.T) {
	doc := &types.AnswerDocument{
		Steps: []types.AnswerStep{
			{Description: "first step"},
			{Description: "second step"},
		},
		Symbols: []types.AnswerSymbol{
			{Name: "Foo", File: "foo.go", Line: 42, Rationale: "the foo"},
		},
	}
	body := renderConsistencyReviewBody(doc)
	if !strings.Contains(body, "1. first step") {
		t.Errorf("expected numbered first step in: %s", body)
	}
	if !strings.Contains(body, "2. second step") {
		t.Errorf("expected numbered second step in: %s", body)
	}
	if !strings.Contains(body, "- Foo @ foo.go:42 — the foo") {
		t.Errorf("expected bulleted symbol in: %s", body)
	}
}
