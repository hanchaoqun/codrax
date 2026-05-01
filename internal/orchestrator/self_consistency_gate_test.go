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

// TestBuildSelfContradictionRepair_CoversFiveErrorClasses pins
// the comprehensive rewrite guidance per contradiction (commit
// 62 follow-up). The Repair must convey enough information for
// the LLM to fix every model-fix-able error class:
//
//   1. pure prose miswriting
//   2. post-revision summary stale (most likely cause)
//   3. genuine ambiguity needing hedge (FALLBACK)
//   4. partial-fix introducing new error (multi-contradiction case)
//   5. LLM missed some contradictions in this batch (batch hint)
//
// Each error class needs a specific signal in the Repair text;
// this test pins those signals.
func TestBuildSelfContradictionRepair_CoversFiveErrorClasses(t *testing.T) {
	c := SelfConsistencyContradiction{
		Topic:        "read vs write check count",
		SummaryClaim: "write runs 9 / read runs 5",
		BodyClaim:    "if !isWrite (read) adds 4-7",
	}
	reasoning := "summary inverts the read/write count assignment"

	// Single contradiction case — should NOT carry batch framing.
	repair := buildSelfContradictionRepair(c, reasoning, 1, 1)

	// 1) Topic surfaced (LLM scans Topic first).
	if !strings.Contains(repair, "TOPIC: read vs write check count") {
		t.Errorf("Repair missing TOPIC framing: %s", repair)
	}
	// 2) Both verbatim claims present.
	if !strings.Contains(repair, "write runs 9 / read runs 5") {
		t.Errorf("Repair missing summary verbatim: %s", repair)
	}
	if !strings.Contains(repair, "if !isWrite (read) adds 4-7") {
		t.Errorf("Repair missing body verbatim: %s", repair)
	}
	// 3) Reviewer's reasoning included (LLM sees prior judgment).
	if !strings.Contains(repair, "summary inverts the read/write") {
		t.Errorf("Repair missing reviewer reasoning: %s", repair)
	}
	// 4) Clear preference: rewrite SUMMARY to match BODY (because
	// body has citations).
	if !strings.Contains(repair, "rewrite the SUMMARY to align with the BODY") {
		t.Errorf("Repair missing rewrite-summary-to-match-body action: %s", repair)
	}
	if !strings.Contains(repair, "grounder") {
		t.Errorf("Repair should explain WHY body is authoritative (citations validated by grounder): %s", repair)
	}
	// 5) Fallback for genuine ambiguity (covers error class 3).
	if !strings.Contains(repair, "FALLBACK") || !strings.Contains(repair, "hedge") {
		t.Errorf("Repair missing FALLBACK/hedge guidance for ambiguous-evidence case: %s", repair)
	}
	// 6) "Do NOT introduce new claims" prevents the LLM from
	// hallucinating a third version (rare but observed).
	if !strings.Contains(repair, "Do NOT introduce new") {
		t.Errorf("Repair missing 'do not introduce new claims' guard: %s", repair)
	}
	// 7) Single-contradiction case does NOT carry batch framing.
	if strings.Contains(repair, "[1 of 1") {
		t.Errorf("Single-contradiction Repair should not show batch index: %s", repair)
	}
	if strings.Contains(repair, "Reconcile ALL") {
		t.Errorf("Single-contradiction Repair should not carry multi-fix preamble: %s", repair)
	}
}

// TestBuildSelfContradictionRepair_MultiContradictionBatchFraming
// pins error-class 4+5: when the batch carries N contradictions,
// each Repair shows its index + total + the "reconcile ALL before
// emitting" + "partial fix triggers another retry" framing.
func TestBuildSelfContradictionRepair_MultiContradictionBatchFraming(t *testing.T) {
	c := SelfConsistencyContradiction{
		Topic:        "X",
		SummaryClaim: "A",
		BodyClaim:    "B",
	}
	repair := buildSelfContradictionRepair(c, "", 2, 3)

	if !strings.Contains(repair, "[2 of 3 contradictions]") {
		t.Errorf("multi-batch Repair missing index/total prefix: %s", repair)
	}
	if !strings.Contains(repair, "Reconcile ALL listed contradictions") {
		t.Errorf("multi-batch Repair missing reconcile-all preamble: %s", repair)
	}
	if !strings.Contains(repair, "partial fix") || !strings.Contains(repair, "another retry") {
		t.Errorf("multi-batch Repair missing partial-fix-triggers-retry warning: %s", repair)
	}
}

// TestBuildSelfContradictionRepair_EmptyReasoningSkipped pins
// graceful handling: when reviewer didn't fill reasoning (200
// chars max but optional), the Repair omits the reasoning
// segment cleanly rather than emitting "Reviewer reasoning: . ".
func TestBuildSelfContradictionRepair_EmptyReasoningSkipped(t *testing.T) {
	c := SelfConsistencyContradiction{Topic: "T", SummaryClaim: "S", BodyClaim: "B"}
	repair := buildSelfContradictionRepair(c, "", 1, 1)
	if strings.Contains(repair, "Reviewer reasoning:") {
		t.Errorf("empty reasoning should not emit a 'Reviewer reasoning:' segment: %s", repair)
	}
}

// TestClampReasoningForRepair_NewlinesAndCap pins the helper.
func TestClampReasoningForRepair_NewlinesAndCap(t *testing.T) {
	long := "first line\nsecond line\rthird line " + strings.Repeat("x", 300)
	got := clampReasoningForRepair(long)
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("clamped reasoning should have no newlines: %q", got)
	}
	// 200 + ellipsis (1 char) = 201 runes max.
	if r := []rune(got); len(r) > 201 {
		t.Errorf("clamped reasoning exceeded 201 runes: %d", len(r))
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
