// Package agent — finalizer_switch_to_patch_test.go (2026-05-10).
//
// P3 of the post-sweep optimization: after ≥ 2 consecutive
// emit_answer_document failures within a dispatch, the finalizer
// evaluator emits a strategic nudge suggesting the LLM switch to
// emit_answer_document_patch. Reduces u10a-class
// (8-finalizer-iter) cases where the LLM kept retrying full-doc
// emits with minor structural fixes.
package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitSwitchToPatchSignal_NoLastResult_NoOp(t *testing.T) {
	e := &answerDocumentEvaluator{}
	if got := e.emitSwitchToPatchSignal(LoopObservation{}); got.HintRequested {
		t.Errorf("nil LastToolResult: expected no hint; got %+v", got)
	}
}

func TestEmitSwitchToPatchSignal_NonEmitTool_NoOp(t *testing.T) {
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "read_file", Success: false},
	}
	if got := e.emitSwitchToPatchSignal(obs); got.HintRequested {
		t.Errorf("non-emit tool: expected no hint; got %+v", got)
	}
}

func TestEmitSwitchToPatchSignal_FirstFailure_NoNudge(t *testing.T) {
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	got := e.emitSwitchToPatchSignal(obs)
	if got.HintRequested {
		t.Errorf("1st failure: expected no nudge (LLM gets one fair chance); got %+v", got)
	}
	if e.emitFullDocFailStreak != 1 {
		t.Errorf("streak should be 1 after 1st failure; got %d", e.emitFullDocFailStreak)
	}
}

func TestEmitSwitchToPatchSignal_SecondFailure_NudgeFires(t *testing.T) {
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	// 1st failure: no nudge.
	e.emitSwitchToPatchSignal(obs)
	// 2nd failure: nudge.
	got := e.emitSwitchToPatchSignal(obs)
	if !got.HintRequested {
		t.Fatalf("2nd failure: expected nudge; got %+v", got)
	}
	if got.HintKey != "answer_doc.switch_to_patch" {
		t.Errorf("expected HintKey=answer_doc.switch_to_patch; got %q", got.HintKey)
	}
	if !strings.Contains(got.Hint, "emit_answer_document_patch") {
		t.Errorf("expected patch tool name in hint; got %q", got.Hint)
	}
	if !strings.Contains(got.Hint, "unchanged_block_ids") {
		t.Errorf("expected unchanged_block_ids guidance; got %q", got.Hint)
	}
}

func TestEmitSwitchToPatchSignal_OneShotPerDispatch(t *testing.T) {
	// After the nudge fires once, subsequent failures should NOT
	// re-fire it (one-shot guard).
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	e.emitSwitchToPatchSignal(obs) // 1st
	got := e.emitSwitchToPatchSignal(obs) // 2nd — fires nudge
	if !got.HintRequested {
		t.Fatal("2nd should have fired nudge")
	}
	got = e.emitSwitchToPatchSignal(obs) // 3rd
	if got.HintRequested {
		t.Errorf("3rd failure: nudge should be one-shot; got %+v", got)
	}
}

func TestEmitSwitchToPatchSignal_SuccessfulEmitResetsStreak(t *testing.T) {
	e := &answerDocumentEvaluator{}
	failObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	successObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: true},
	}
	e.emitSwitchToPatchSignal(failObs)    // streak = 1
	e.emitSwitchToPatchSignal(successObs) // streak resets
	if e.emitFullDocFailStreak != 0 {
		t.Errorf("successful emit should reset streak; got %d", e.emitFullDocFailStreak)
	}
}

func TestEmitSwitchToPatchSignal_SuccessfulPatchResetsStreak(t *testing.T) {
	// LLM accepted the switch, used patch successfully → reset.
	e := &answerDocumentEvaluator{emitFullDocFailStreak: 2}
	successObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document_patch", Success: true},
	}
	e.emitSwitchToPatchSignal(successObs)
	if e.emitFullDocFailStreak != 0 {
		t.Errorf("successful patch should reset streak; got %d", e.emitFullDocFailStreak)
	}
}

func TestEmitSwitchToPatchSignal_FailedPatch_DoesNotIncrementFullStreak(t *testing.T) {
	// A patch failure is a different repair surface; we don't
	// want to escalate "switch to patch" further when patch
	// itself failed.
	e := &answerDocumentEvaluator{emitFullDocFailStreak: 1}
	patchFailObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false},
	}
	got := e.emitSwitchToPatchSignal(patchFailObs)
	if got.HintRequested {
		t.Errorf("patch failure: expected no nudge from full-doc evaluator; got %+v", got)
	}
	if e.emitFullDocFailStreak != 1 {
		t.Errorf("patch failure should not bump full-doc streak; got %d", e.emitFullDocFailStreak)
	}
}

func TestEmitSwitchToPatchSignal_HintIsLanguageNeutral(t *testing.T) {
	// R6 audit: no internal stage names ("explorer" / "extractor"
	// / "downstream stage"), no internal field names.
	e := &answerDocumentEvaluator{emitFullDocFailStreak: 1}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	got := e.emitSwitchToPatchSignal(obs)
	if !got.HintRequested {
		t.Fatal("expected nudge")
	}
	for _, internal := range []string{
		"explorer", "extractor", "finalizer", "analyzer",
		"downstream stage", "AnswerDocumentV2", "AnswerBlock",
	} {
		if strings.Contains(got.Hint, internal) {
			t.Errorf("internal term %q leaked into LLM-facing hint: %q", internal, got.Hint)
		}
	}
}
