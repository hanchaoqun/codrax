// Package agent — finalizer_switch_to_patch_test.go (2026-05-10).
//
// P3 of the post-sweep optimization: after ≥ 2 consecutive
// emit_answer_document failures within a dispatch, the finalizer
// evaluator emits a strategic nudge suggesting the LLM switch to
// emit_answer_document_patch. Reduces u10a-class
// (8-finalizer-iter) cases where the LLM kept retrying full-doc
// emits with minor structural fixes.
//
// 2026-05-10 PM follow-up after sweep digest forensic: the original
// one-shot semantics were broken — the nudge fires once and the
// MinInjectInterval throttle could swallow it. Updated to re-fire
// on every emit_answer_document failure beyond streak>=2 and to
// carry BypassThrottle=true; the latch (emitPatchNudgeFired) only
// closes when the LLM is observed calling
// emit_answer_document_patch (success or failure).
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

func TestEmitSwitchToPatchSignal_RefiresUntilLLMSwitches(t *testing.T) {
	// 2026-05-10 PM: forensic on s7b finalizer iter=1 showed the
	// one-shot guard fired even when the policy throttled away the
	// hint, leaving the LLM unaware of the recommendation. The new
	// semantics re-fire on every emit_answer_document failure
	// beyond streak>=2 — the per-HintKey cap (5) bounds total
	// fires.
	e := &answerDocumentEvaluator{}
	failObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	e.emitSwitchToPatchSignal(failObs) // 1st failure → no nudge yet
	for i := 0; i < 5; i++ {
		got := e.emitSwitchToPatchSignal(failObs)
		if !got.HintRequested {
			t.Fatalf("failure #%d (streak=%d): expected re-fire; got %+v",
				i+2, e.emitFullDocFailStreak, got)
		}
		if got.HintKey != "answer_doc.switch_to_patch" {
			t.Errorf("failure #%d: expected HintKey=answer_doc.switch_to_patch; got %q", i+2, got.HintKey)
		}
		if !got.BypassThrottle {
			t.Errorf("failure #%d: re-fire must carry BypassThrottle=true so MinInjectInterval can't suppress it", i+2)
		}
	}
}

func TestEmitSwitchToPatchSignal_LatchesAfterPatchObserved(t *testing.T) {
	// Once the LLM switches to emit_answer_document_patch (success
	// or failure), the latch closes — further full-doc failures
	// don't re-fire the nudge.
	e := &answerDocumentEvaluator{}
	failObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	e.emitSwitchToPatchSignal(failObs)
	got := e.emitSwitchToPatchSignal(failObs)
	if !got.HintRequested {
		t.Fatal("2nd failure: expected nudge before latch")
	}
	// LLM acknowledges by calling patch (here: failure case — the
	// switch happened, the call shape may still be wrong).
	patchObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false},
	}
	if sig := e.emitSwitchToPatchSignal(patchObs); sig.HintRequested {
		t.Errorf("patch observation should latch the nudge silently; got %+v", sig)
	}
	if !e.emitPatchNudgeFired {
		t.Errorf("emitPatchNudgeFired must latch on patch observation; got false")
	}
	// Subsequent full-doc failure must NOT re-fire (latch is closed).
	if sig := e.emitSwitchToPatchSignal(failObs); sig.HintRequested {
		t.Errorf("after latch: full-doc failure must not re-fire nudge; got %+v", sig)
	}
}

func TestEmitSwitchToPatchSignal_RefireBypassesThrottle(t *testing.T) {
	// Regression: every fire of the nudge must carry
	// BypassThrottle=true. The 2026-05-10 forensic showed that
	// without this, MinInjectInterval (default 3) suppressed the
	// nudge after a prior tool-reject hint at iter-1.
	e := &answerDocumentEvaluator{emitFullDocFailStreak: 1}
	failObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	got := e.emitSwitchToPatchSignal(failObs)
	if !got.HintRequested {
		t.Fatal("expected nudge")
	}
	if !got.BypassThrottle {
		t.Errorf("nudge must set BypassThrottle=true so MinInjectInterval cannot drop it; got %+v", got)
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
	// LLM accepted the switch, used patch successfully → reset
	// streak AND latch the nudge.
	e := &answerDocumentEvaluator{emitFullDocFailStreak: 2}
	successObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document_patch", Success: true},
	}
	e.emitSwitchToPatchSignal(successObs)
	if e.emitFullDocFailStreak != 0 {
		t.Errorf("successful patch should reset streak; got %d", e.emitFullDocFailStreak)
	}
	if !e.emitPatchNudgeFired {
		t.Errorf("successful patch should also latch emitPatchNudgeFired")
	}
}

func TestEmitSwitchToPatchSignal_FailedPatch_LatchesNudge(t *testing.T) {
	// A patch failure still means the LLM has SEEN the recommendation
	// and is iterating on the patch path. Latch the nudge so we
	// don't second-guess the LLM's choice. The streak is NOT reset
	// (patch is a different tool surface — its retries are
	// orthogonal to full-doc retry semantics).
	e := &answerDocumentEvaluator{emitFullDocFailStreak: 1}
	patchFailObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false},
	}
	got := e.emitSwitchToPatchSignal(patchFailObs)
	if got.HintRequested {
		t.Errorf("patch failure: expected silent latch (no hint); got %+v", got)
	}
	if !e.emitPatchNudgeFired {
		t.Errorf("patch observation (success or failure) must latch the nudge")
	}
	if e.emitFullDocFailStreak != 1 {
		t.Errorf("patch failure should not bump or reset full-doc streak; got %d", e.emitFullDocFailStreak)
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
