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

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// llmToolSchemaT2 mirrors llm.ToolSchema for the T2 test helpers.
// Kept local so the file's other tests do not need to import the llm
// package, and to give the assertion helpers a stable comparison
// surface (Name + Description; the JSON Parameters field is not part
// of the schema-filter contract).
type llmToolSchemaT2 = llm.ToolSchema

func callFilterToolSchemasT2(e *answerDocumentEvaluator, ctx *types.AgentContext, in []llmToolSchemaT2) []llmToolSchemaT2 {
	return e.FilterToolSchemas(ctx, in)
}

func sameSchemaNamesT2(a, b []llmToolSchemaT2) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

func ctxWithAnswerPatchBase() *types.AgentContext {
	mut := types.NewMutableState("retry")
	mut.SetRetryState(&types.RetryState{
		Attempt:      1,
		PrevEmitJSON: []byte(`{"blocks":[{"id":"s1","kind":"summary","text":"x"}]}`),
	})
	return &types.AgentContext{Mutable: mut}
}

func TestEmitSwitchToPatchSignal_NoLastResult_NoOp(t *testing.T) {
	e := &answerDocumentEvaluator{}
	if got := e.emitSwitchToPatchSignal(nil, LoopObservation{}); got.HintRequested {
		t.Errorf("nil LastToolResult: expected no hint; got %+v", got)
	}
}

func TestEmitSwitchToPatchSignal_NonEmitTool_NoOp(t *testing.T) {
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "read_file", Success: false},
	}
	if got := e.emitSwitchToPatchSignal(nil, obs); got.HintRequested {
		t.Errorf("non-emit tool: expected no hint; got %+v", got)
	}
}

func TestEmitSwitchToPatchSignal_FirstFailure_NoNudge(t *testing.T) {
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	got := e.emitSwitchToPatchSignal(nil, obs)
	if got.HintRequested {
		t.Errorf("1st failure: expected no nudge (LLM gets one fair chance); got %+v", got)
	}
	if e.emitFullDocFailStreak != 1 {
		t.Errorf("streak should be 1 after 1st failure; got %d", e.emitFullDocFailStreak)
	}
}

func TestEmitSwitchToPatchSignal_SecondFailure_NudgeFires(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	// 1st failure: no nudge.
	e.emitSwitchToPatchSignal(ctx, obs)
	// 2nd failure: nudge.
	got := e.emitSwitchToPatchSignal(ctx, obs)
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

func TestEmitSwitchToPatchSignal_NoPatchBase_NoNudge(t *testing.T) {
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	e.emitSwitchToPatchSignal(nil, obs)
	got := e.emitSwitchToPatchSignal(nil, obs)
	if got.HintRequested {
		t.Fatalf("patch nudge must not fire before a previous successful answer document exists; got %+v", got)
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
	ctx := ctxWithAnswerPatchBase()
	failObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	e.emitSwitchToPatchSignal(ctx, failObs) // 1st failure → no nudge yet
	for i := 0; i < 5; i++ {
		got := e.emitSwitchToPatchSignal(ctx, failObs)
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
	ctx := ctxWithAnswerPatchBase()
	failObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	e.emitSwitchToPatchSignal(ctx, failObs)
	got := e.emitSwitchToPatchSignal(ctx, failObs)
	if !got.HintRequested {
		t.Fatal("2nd failure: expected nudge before latch")
	}
	// LLM acknowledges by calling patch (here: failure case — the
	// switch happened, the call shape may still be wrong).
	patchObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false},
	}
	if sig := e.emitSwitchToPatchSignal(ctx, patchObs); sig.HintRequested {
		t.Errorf("patch observation should latch the nudge silently; got %+v", sig)
	}
	if !e.emitPatchNudgeFired {
		t.Errorf("emitPatchNudgeFired must latch on patch observation; got false")
	}
	// Subsequent full-doc failure must NOT re-fire (latch is closed).
	if sig := e.emitSwitchToPatchSignal(ctx, failObs); sig.HintRequested {
		t.Errorf("after latch: full-doc failure must not re-fire nudge; got %+v", sig)
	}
}

func TestEmitSwitchToPatchSignal_RefireBypassesThrottle(t *testing.T) {
	// Regression: every fire of the nudge must carry
	// BypassThrottle=true. The 2026-05-10 forensic showed that
	// without this, MinInjectInterval (default 3) suppressed the
	// nudge after a prior tool-reject hint at iter-1.
	e := &answerDocumentEvaluator{emitFullDocFailStreak: 1}
	ctx := ctxWithAnswerPatchBase()
	failObs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	got := e.emitSwitchToPatchSignal(ctx, failObs)
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
	e.emitSwitchToPatchSignal(nil, failObs)    // streak = 1
	e.emitSwitchToPatchSignal(nil, successObs) // streak resets
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
	e.emitSwitchToPatchSignal(nil, successObs)
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
	got := e.emitSwitchToPatchSignal(nil, patchFailObs)
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

func TestEmitPatchRejectFullRewriteSignal_FailedPatchRequestsFreshFullEmit(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document_patch",
			Success:  false,
			Summary:  `patch: unchanged_block_ids["new_block"] not present in previous emit`,
		},
	}

	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested {
		t.Fatalf("failed patch should request a full rewrite hint; got %+v", got)
	}
	if got.HintKey != "answer_doc.patch_rewrite" {
		t.Fatalf("HintKey=%q, want answer_doc.patch_rewrite", got.HintKey)
	}
	for _, want := range []string{
		"emit_answer_document",
		"fresh `citations[]` array",
		"renumber every `blocks[].items[].citation_ref`",
		"Stop patching",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("patch rewrite hint missing %q:\n%s", want, got.Hint)
		}
	}
	if !got.BypassThrottle || !got.BypassBudget {
		t.Errorf("patch rewrite hint must bypass throttle/budget; got %+v", got)
	}
	if !e.forceFullEmitNext {
		t.Fatal("patch rewrite signal must force a full-emit schema pass next")
	}
}

func TestEmitSwitchToPatchSignal_HintIsLanguageNeutral(t *testing.T) {
	// R6 audit: no internal stage names ("explorer" / "extractor"
	// / "downstream stage"), no internal field names.
	e := &answerDocumentEvaluator{emitFullDocFailStreak: 1}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{ToolName: "emit_answer_document", Success: false},
	}
	got := e.emitSwitchToPatchSignal(ctx, obs)
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

// TestFilterToolSchemas_T2_PatchFirstEnforcement pins the T2 contract:
// once full-emit has failed twice AND a patch base exists, the
// evaluator's ToolSchemaFilter implementation drops
// emit_answer_document from the schema list so the next LLM turn can
// only call emit_answer_document_patch. The mid-loop nudge that
// already advises the switch is now backed by schema-level
// enforcement — the model cannot choose the wasteful full-emit path.
func TestFilterToolSchemas_T2_PatchFirstEnforcement(t *testing.T) {
	baseSchemas := []llmToolSchemaT2{
		{Name: "emit_answer_document"},
		{Name: "emit_answer_document_patch"},
		{Name: "propose_sub_agents"},
	}

	t.Run("no_filter_when_streak_below_threshold", func(t *testing.T) {
		e := &answerDocumentEvaluator{emitFullDocFailStreak: 1}
		ctx := ctxWithAnswerPatchBase()
		got := callFilterToolSchemasT2(e, ctx, baseSchemas)
		if !sameSchemaNamesT2(got, baseSchemas) {
			t.Errorf("streak=1: filter must be no-op; got %+v", got)
		}
	})

	t.Run("no_filter_when_no_patch_base", func(t *testing.T) {
		e := &answerDocumentEvaluator{emitFullDocFailStreak: 3}
		ctx := &types.AgentContext{Mutable: types.NewMutableState("q")}
		got := callFilterToolSchemasT2(e, ctx, baseSchemas)
		if !sameSchemaNamesT2(got, baseSchemas) {
			t.Errorf("no patch base: filter must be no-op; got %+v", got)
		}
	})

	t.Run("drops_full_emit_when_streak_meets_threshold_and_base_present", func(t *testing.T) {
		e := &answerDocumentEvaluator{emitFullDocFailStreak: 2}
		ctx := ctxWithAnswerPatchBase()
		got := callFilterToolSchemasT2(e, ctx, baseSchemas)
		for _, s := range got {
			if s.Name == "emit_answer_document" {
				t.Errorf("emit_answer_document must be filtered out; got remaining schemas %+v", got)
			}
		}
		foundPatch := false
		foundUnrelated := false
		for _, s := range got {
			if s.Name == "emit_answer_document_patch" {
				foundPatch = true
			}
			if s.Name == "propose_sub_agents" {
				foundUnrelated = true
			}
		}
		if !foundPatch {
			t.Errorf("patch tool must remain in schema list; got %+v", got)
		}
		if !foundUnrelated {
			t.Errorf("unrelated tools must be preserved; got %+v", got)
		}
	})

	t.Run("nil_evaluator_safe", func(t *testing.T) {
		var e *answerDocumentEvaluator
		got := callFilterToolSchemasT2(e, ctxWithAnswerPatchBase(), baseSchemas)
		if !sameSchemaNamesT2(got, baseSchemas) {
			t.Errorf("nil evaluator: pass-through; got %+v", got)
		}
	})

	t.Run("does_not_mutate_input_slice", func(t *testing.T) {
		e := &answerDocumentEvaluator{emitFullDocFailStreak: 3}
		ctx := ctxWithAnswerPatchBase()
		original := append([]llmToolSchemaT2(nil), baseSchemas...)
		_ = callFilterToolSchemasT2(e, ctx, baseSchemas)
		if !sameSchemaNamesT2(baseSchemas, original) {
			t.Errorf("filter mutated input slice: got %+v, want %+v", baseSchemas, original)
		}
	})

	// REGRESSION (2026-05-17 hotfix): if the incoming schema list does
	// NOT contain emit_answer_document_patch (because buildToolSchemas
	// auto-add gate did not flip ON yet at the moment this iter's
	// schemas were derived), the filter MUST be a no-op. Without this
	// safety the filter strips emit_answer_document and leaves zero
	// callable tools, which sends the LLM into a content-only soft-
	// stop loop (forensic: a 13-iter run with tools=0 producing 12k-
	// token prose responses, tool_calls=0 every iter). The hotfix
	// added a hasPatch precondition that returns the input unchanged
	// when the patch tool is absent from the candidate schema set.
	t.Run("safety_no_op_when_patch_absent_even_if_base_available", func(t *testing.T) {
		schemasWithoutPatch := []llmToolSchemaT2{
			{Name: "emit_answer_document"},
			{Name: "propose_sub_agents"},
		}
		e := &answerDocumentEvaluator{emitFullDocFailStreak: 5}
		ctx := ctxWithAnswerPatchBase() // patch base exists in mutable
		got := callFilterToolSchemasT2(e, ctx, schemasWithoutPatch)
		if !sameSchemaNamesT2(got, schemasWithoutPatch) {
			t.Errorf("safety check failed — filter stripped emit_answer_document despite patch tool absent; got %+v want %+v",
				got, schemasWithoutPatch)
		}
	})

	t.Run("patch_reject_full_rewrite_keeps_full_emit_and_drops_patch_once", func(t *testing.T) {
		e := &answerDocumentEvaluator{
			emitFullDocFailStreak: 5,
			forceFullEmitNext:     true,
		}
		ctx := ctxWithAnswerPatchBase()
		got := callFilterToolSchemasT2(e, ctx, baseSchemas)
		hasFull := false
		hasPatch := false
		for _, s := range got {
			if s.Name == "emit_answer_document" {
				hasFull = true
			}
			if s.Name == "emit_answer_document_patch" {
				hasPatch = true
			}
		}
		if !hasFull || hasPatch {
			t.Fatalf("full-rewrite pass should expose full emit only, got %+v", got)
		}
	})
}
