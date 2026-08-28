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
	"reflect"
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

func ctxWithAnswerPatchBaseAndSequenceCallCapsule() *types.AgentContext {
	ctx := ctxWithAnswerPatchBase()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		},
		AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
			PreferredKinds: []types.DiagramKind{types.DiagramSequence},
		}},
	}
	ctx.EvidenceItems = []types.EvidenceItem{{
		ID: "call-edge", Kind: types.EvidenceRelationship,
		Source: "internal/orchestrator/orchestrator.go", LineStart: 2485,
		AnchorKind: types.AnchorCall, Subject: "Orchestrator.runAnalyzePhase", Object: "Orchestrator.dispatchStage",
		GroundingStatus: types.GroundingGrounded,
	}}
	return ctx
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

func TestEmitAnswerDocumentRejectSignal_FirstRejectPrefersPatchWhenDraftAvailable(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		Phase: PhaseMidLoop,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "ShouldNotLeak: Field: blocks[].kind=ordered_list",
			Repair: &types.ToolRepair{
				Code:   "answer_doc_pre_emit_contract",
				Fields: []string{"blocks[].kind=ordered_list"},
				Hint:   "Apply the typed answer-document schema correction and preserve existing answer facts.",
				Metadata: map[string]string{
					"expected_shapes": "emit at least 1 block(s) of kind=ordered_list",
				},
			},
		},
	}

	got := e.emitAnswerDocumentRejectSignal(ctx, obs)
	if !got.HintRequested {
		t.Fatalf("first full-doc reject should request a repair hint; got %+v", got)
	}
	for _, want := range []string{
		"Use `emit_answer_document_patch` now",
		"unchanged_block_ids",
		"replace_blocks",
		"add_blocks",
		"`blocks[].kind=ordered_list`",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Fatalf("patch-first hint missing %q:\n%s", want, got.Hint)
		}
	}
	if strings.Contains(got.Hint, "Re-emit `emit_answer_document` with the FULL previous payload") {
		t.Fatalf("optional diagram patch hint must not carry the contradictory full-emit directive:\n%s", got.Hint)
	}
	if strings.Contains(got.Hint, "paste the FULL previous payload") {
		t.Fatalf("patch-first hint must not ask for full-payload paste:\n%s", got.Hint)
	}
	if strings.Contains(got.Hint, "ShouldNotLeak") {
		t.Fatalf("patch-first hint must not include rendered tool summary detail:\n%s", got.Hint)
	}
	if !e.preferPatchNext {
		t.Fatal("patch-first full-doc reject should prefer patch on the next schema pass")
	}
}

func TestEmitAnswerDocumentRejectSignal_NoPatchBaseKeepsFullEmit(t *testing.T) {
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		Phase: PhaseMidLoop,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "ShouldNotLeak: Field: blocks[].kind=ordered_list",
		},
	}

	got := e.emitAnswerDocumentRejectSignal(&types.AgentContext{Mutable: types.NewMutableState("q")}, obs)
	if !got.HintRequested {
		t.Fatalf("full-doc reject without patch base should still request a repair hint; got %+v", got)
	}
	if !strings.Contains(got.Hint, "complete `emit_answer_document` payload") {
		t.Fatalf("without a patch base the hint should ask for a complete full emit:\n%s", got.Hint)
	}
	if strings.Contains(got.Hint, "paste the FULL previous payload") {
		t.Fatalf("without a patch base the hint must not ask for an unavailable previous payload:\n%s", got.Hint)
	}
	if strings.Contains(got.Hint, "ShouldNotLeak") || strings.Contains(got.Hint, "ordered_list") {
		t.Fatalf("summary-only full-doc reject must stay generic:\n%s", got.Hint)
	}
	if e.preferPatchNext {
		t.Fatal("no patch base must not prefer patch")
	}
}

func TestEmitAnswerDocumentRejectSignal_LossyBlocksStringUsesRecoveryMetadata(t *testing.T) {
	e := &answerDocumentEvaluator{}
	obs := LoopObservation{
		Phase: PhaseMidLoop,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "ShouldNotLeak: structured recovery could not preserve every visible block",
			Repair: &types.ToolRepair{
				Code:   "answer_doc_lossy_blocks_string_recovery",
				Fields: []string{"blocks"},
				Hint:   "Generic hint should be replaced by the typed lossy-blocks hint.",
				Metadata: map[string]string{
					"candidate_blocks":      "4",
					"recovered_blocks":      "3",
					"recovered_attachments": "0",
					"candidate_kinds":       "summary,ordered_list,caveat",
					"recovered_kinds":       "summary,ordered_list,caveat",
					"recovery_mode":         "brace_balanced_blocks",
				},
			},
		},
	}

	got := e.emitAnswerDocumentRejectSignal(&types.AgentContext{Mutable: types.NewMutableState("q")}, obs)
	if !got.HintRequested {
		t.Fatalf("lossy blocks-string reject should request a local repair hint; got %+v", got)
	}
	for _, want := range []string{
		"JSON-encoded string",
		"native JSON array",
		"candidate_blocks=4",
		"recovered_blocks=3",
		"Structured recovered block kinds: summary,ordered_list,caveat",
		"Keep the recovered structured blocks and `citations[]` intact",
		"Do not reopen files",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Fatalf("lossy blocks-string hint missing %q:\n%s", want, got.Hint)
		}
	}
	if strings.Contains(got.Hint, "ShouldNotLeak") || strings.Contains(got.Hint, "Generic hint") {
		t.Fatalf("hint must use typed repair metadata, not rendered summary/generic prose:\n%s", got.Hint)
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

func TestEmitPatchRejectFullRewriteSignal_FailedPatchRequestsPatchCorrection(t *testing.T) {
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
		t.Fatalf("failed patch should request a patch correction hint; got %+v", got)
	}
	if got.HintKey != "answer_doc.patch_correct" {
		t.Fatalf("HintKey=%q, want answer_doc.patch_correct", got.HintKey)
	}
	for _, want := range []string{
		"Keep using `emit_answer_document_patch`",
		"replace_blocks",
		"add_blocks",
		"unchanged_block_ids",
		"Preserve the inherited `citations[]` pool",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("patch correction hint missing %q:\n%s", want, got.Hint)
		}
	}
	if !got.BypassThrottle || !got.BypassBudget {
		t.Errorf("patch correction hint must bypass throttle/budget; got %+v", got)
	}
	if e.forceFullEmitNext {
		t.Fatal("patch correction must not force a full-emit schema pass next")
	}
	if !e.preferPatchNext {
		t.Fatal("patch correction must keep patch preferred for the next schema pass")
	}
}

func TestEmitPatchRejectFullRewriteSignal_TypedRepairDrivesPatchCorrection(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document_patch",
			Success:  false,
			Summary:  "summary prose should not become the correction detail",
			Repair: &types.ToolRepair{
				Code:   "answer_doc_patch_existing_block",
				Fields: []string{"add_blocks", "replace_blocks", "unchanged_block_ids"},
				Hint:   "Move the existing block into replace_blocks.",
			},
		},
	}

	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested {
		t.Fatalf("typed patch repair should request a patch correction hint; got %+v", got)
	}
	for _, want := range []string{
		"repair=answer_doc_patch_existing_block",
		"fields=add_blocks,replace_blocks,unchanged_block_ids",
		"Move the existing block into replace_blocks.",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("typed patch correction hint missing %q:\n%s", want, got.Hint)
		}
	}
	if strings.Contains(got.Hint, "summary prose should not become") {
		t.Fatalf("typed repair should suppress irrelevant summary detail:\n%s", got.Hint)
	}
}

func TestEmitPatchRejectFullRewriteSignal_OptionalDiagramCallEdgeOffersRemoval(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document_patch",
			Success:  false,
			Repair: &types.ToolRepair{
				Code:   "answer_doc_pre_emit_contract",
				Fields: []string{"blocks[].edge_anchors[] AND blocks[kind=diagram].diagram.body"},
				Metadata: map[string]string{
					"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
					types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
				},
			},
		},
	}

	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested || got.HintKey != "answer_doc.patch_optional_diagram_call_edge" {
		t.Fatalf("optional diagram-only reject should select bounded recovery lane, got %+v", got)
	}
	for _, want := range []string{
		"OPTIONAL diagram",
		"remove_block_ids",
		"typed call-edge evidence",
		"No typed topology template is available",
		"Visible node labels, edge/message labels, and Notes remain model-authored",
		"source locations are evidence metadata; do not copy them as the primary visible wording",
		"beyond the grounded textual answer",
		"This is authoring guidance, not a requirement to keep a diagram",
		"will not remove or rewrite the diagram for you",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("optional diagram recovery hint missing %q:\n%s", want, got.Hint)
		}
	}
	if e.forceFullEmitNext || !e.preferPatchNext {
		t.Fatalf("optional diagram recovery must remain model-owned and patch-local: forceFull=%t preferPatch=%t",
			e.forceFullEmitNext, e.preferPatchNext)
	}
}

func TestEmitAnswerDocumentRejectSignal_OptionalDiagramCallEdgeConvergesOnFirstReject(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	obs := LoopObservation{LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document",
		Success:  false,
		Repair: &types.ToolRepair{
			Code:   "answer_doc_pre_emit_contract",
			Fields: []string{"blocks[].edge_anchors[] AND blocks[kind=diagram].diagram.body"},
			Metadata: map[string]string{
				"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
			},
		},
	}}

	got := e.emitAnswerDocumentRejectSignal(ctx, obs)
	if !got.HintRequested || !strings.Contains(got.HintKey, "optional-diagram-call-edge") {
		t.Fatalf("first optional-diagram reject should select bounded patch recovery, got %+v", got)
	}
	for _, want := range []string{
		"Use `emit_answer_document_patch`",
		"typed topology authoring template",
		"prefer replacing only the rejected diagram",
		"This is authoring guidance, not a requirement to keep a diagram",
		"remove_block_ids",
		types.AnswerDocumentPatchOperationTeaching,
		"verified carrier's topology and anchor array",
		"Visible node labels, edge/message labels, and Notes remain model-authored",
		"participant n1 as Orchestrator.runAnalyzePhase",
		"n1->>n2: AUTHOR_BUSINESS_ACTION",
		`edge_anchors_json=` + "`" + `[{"from_node":"n1","to_node":"n2","from_identity":"Orchestrator.runAnalyzePhase","to_identity":"Orchestrator.dispatchStage","relation_kind":"call"}]` + "`",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("first-reject optional diagram hint missing %q:\n%s", want, got.Hint)
		}
	}
	if !e.preferPatchNext {
		t.Fatal("first optional-diagram reject should keep the next turn on patch surface")
	}
}

func TestGroundedDiagramMissingAnchorsPreserveModelGraphOnFullAndPatchReject(t *testing.T) {
	anchorPayload := `[{"block_id":"sequence","edge_anchors":[{"from_node":"A","to_node":"B","from_identity":"Alpha.Run","to_identity":"Beta.Run","relation_kind":"call"},{"from_node":"B","to_node":"C","from_identity":"Beta.Run","to_identity":"Gamma.Run","relation_kind":"call"}]}]`
	for _, tc := range []struct {
		name     string
		toolName string
		patch    bool
	}{
		{name: "full", toolName: "emit_answer_document"},
		{name: "patch", toolName: "emit_answer_document_patch", patch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &answerDocumentEvaluator{diagramRequired: true}
			ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
			result := &types.ToolResult{
				ToolName: tc.toolName,
				Success:  false,
				Repair: &types.ToolRepair{
					Code: "answer_doc_pre_emit_contract",
					Metadata: map[string]string{
						"violation_kinds":                                  string(types.ViolDiagramCallEdgeUnproven),
						types.ToolRepairMetaOffendingBlockKinds:            string(types.BlockDiagram),
						types.ToolRepairMetaDiagramRelationFailureIssues:   types.DiagramRelationFailureMissingGroundedCallAnchor,
						types.ToolRepairMetaDiagramGroundedAnchorPatchJSON: anchorPayload,
					},
				},
			}
			var got LoopSignal
			if tc.patch {
				got = e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
			} else {
				got = e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: result})
			}
			if !got.HintRequested || !strings.Contains(got.HintKey, "grounded_diagram_anchors") && !strings.Contains(got.HintKey, "grounded-diagram-anchors") {
				t.Fatalf("metadata-only grounded lane not selected: %+v", got)
			}
			for _, want := range []string{
				"Mermaid body byte-for-byte",
				"Preserve all model-authored prose and conclusions",
				"Do not remove, add, reverse, reconnect, relabel, or replace any visible relation",
				"do not substitute a reduced evidence skeleton",
				anchorPayload,
				"system supplies no Mermaid body, wording, ordering, relation, prose, or conclusion",
			} {
				if !strings.Contains(got.Hint, want) {
					t.Errorf("grounded metadata-only repair missing %q:\n%s", want, got.Hint)
				}
			}
			for _, forbidden := range []string{
				"Preserve the following evidence skeleton's exact node IDs",
				"typed topology authoring template",
				"remove the optional diagram",
			} {
				if strings.Contains(got.Hint, forbidden) {
					t.Errorf("metadata-only repair activated lossy replacement %q:\n%s", forbidden, got.Hint)
				}
			}
		})
	}
}

func TestGroundedDiagramMissingAnchorsPreserveModelGraphWithCompanionAnswerViolations(t *testing.T) {
	anchorPayload := `[{"block_id":"sequence","edge_anchors":[{"from_node":"A","to_node":"B","from_identity":"Alpha.Run","to_identity":"Beta.Run","relation_kind":"call"},{"from_node":"B","to_node":"C","from_identity":"Beta.Run","to_identity":"Gamma.Run","relation_kind":"call"}]}]`
	e := &answerDocumentEvaluator{diagramRequired: true}
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	result := &types.ToolResult{
		ToolName: "emit_answer_document",
		Success:  false,
		Repair: &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				"violation_kinds": strings.Join([]string{
					string(types.ViolCallChainEndpointOmitted),
					string(types.ViolDiagramCallEdgeUnproven),
				}, ","),
				types.ToolRepairMetaOffendingBlockKinds:            string(types.BlockOrderedList) + "," + string(types.BlockDiagram),
				types.ToolRepairMetaDiagramRelationFailureIssues:   types.DiagramRelationFailureMissingGroundedCallAnchor,
				types.ToolRepairMetaDiagramGroundedAnchorPatchJSON: anchorPayload,
			},
		},
	}

	if !answerDocumentRejectOnlyGroundedMissingCallAnchors(result) {
		t.Fatal("companion non-relation violations must not bypass exact diagram metadata repair")
	}
	got := e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: result})
	if !got.HintRequested || !strings.Contains(got.HintKey, "grounded-diagram-anchors") {
		t.Fatalf("mixed answer failure did not preserve the grounded model graph first: %+v", got)
	}
	for _, want := range []string{
		"Mermaid body byte-for-byte",
		"other reported fields remain for the next normal validation pass",
		anchorPayload,
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("mixed answer metadata repair missing %q:\n%s", want, got.Hint)
		}
	}
	for _, forbidden := range []string{"remove the optional diagram"} {
		if strings.Contains(got.Hint, forbidden) {
			t.Errorf("mixed answer metadata repair offered lossy graph removal %q:\n%s", forbidden, got.Hint)
		}
	}
}

func TestGroundedDiagramAnchorRepairFailsClosedForMixedIssues(t *testing.T) {
	result := &types.ToolResult{
		ToolName: "emit_answer_document",
		Success:  false,
		Repair: &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				"violation_kinds":                                  string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaOffendingBlockKinds:            string(types.BlockDiagram),
				types.ToolRepairMetaDiagramRelationFailureIssues:   types.DiagramRelationFailureMissingGroundedCallAnchor + ",unproven_call_edge",
				types.ToolRepairMetaDiagramGroundedAnchorPatchJSON: `[{"block_id":"sequence","edge_anchors":[{"from_node":"A","to_node":"B","from_identity":"Alpha.Run","to_identity":"Beta.Run","relation_kind":"call"}]}]`,
			},
		},
	}
	if answerDocumentRejectOnlyGroundedMissingCallAnchors(result) {
		t.Fatal("mixed grounded/unproved failures must not enter metadata-only repair")
	}
	if _, ok := answerDocGroundedDiagramAnchorPatchHint(result, false); ok {
		t.Fatal("mixed relation failures must stay on fail-closed evidence repair")
	}
}

func TestOptionalDiagramCallEdgePatchHintUsesExactBoundaryWhenWholeFlowSkeletonWithheld(t *testing.T) {
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow

	got := answerDocOptionalDiagramCallEdgePatchHint(ctx, true)
	for _, want := range []string{
		"whole-diagram skeleton is intentionally unavailable",
		"bounded exact relation boundary",
		"typed relation boundary, not a complete-flow claim",
		"node_alias[n1]=`Orchestrator.runAnalyzePhase`",
		"edge_recipe[1]=`n1 -> n2`",
		`edge_anchor_json=` + "`" + `{"from_node":"n1","to_node":"n2","from_identity":"Orchestrator.runAnalyzePhase","to_identity":"Orchestrator.dispatchStage","relation_kind":"call"}` + "`",
		"keep separate components disconnected",
		"Typed topology component templates follow",
		"Verified component fragment 1 (not a complete flow)",
		"participant n1 as Orchestrator.runAnalyzePhase",
		"n1->>n2: AUTHOR_BUSINESS_ACTION",
		`component_edge_anchors_json[1]=` + "`" + `[{"from_node":"n1","to_node":"n2","from_identity":"Orchestrator.runAnalyzePhase","to_identity":"Orchestrator.dispatchStage","relation_kind":"call"}]` + "`",
		"remove it with `remove_block_ids` only when you judge",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("optional flow boundary repair missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"typed topology authoring template is available",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("optional flow boundary repair made unsupported promise %q:\n%s", forbidden, got)
		}
	}
}

func TestRequiredFlowBoundaryCarriesCopyReadyDisconnectedComponentsWithoutBridges(t *testing.T) {
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
	ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{
		ID: "second-call", Kind: types.EvidenceRelationship,
		Source: "internal/agent/analyzer.go", LineStart: 397,
		AnchorKind: types.AnchorCall, Subject: "agent.prependEmitRetryDirective", Object: "Mutable.AnalyzerRetryHint",
		GroundingStatus: types.GroundingGrounded,
	})

	hint, ok := answerDocRequiredDiagramRelationBoundaryPatchHint(ctx, true)
	if !ok {
		t.Fatal("required flow boundary should be available")
	}
	for _, want := range []string{
		"Verified component fragment 1 (not a complete flow)",
		"Verified component fragment 2 (not a complete flow)",
		"Fragments are mutually unordered and disconnected",
		"participant n1 as Orchestrator.runAnalyzePhase",
		"participant n3 as agent.prependEmitRetryDirective",
		"n1->>n2: AUTHOR_BUSINESS_ACTION",
		"n3->>n4: AUTHOR_BUSINESS_ACTION",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("component-fragment boundary missing %q:\n%s", want, hint)
		}
	}
	for _, forbidden := range []string{"n2->>n3", "n3->>n2", "n1->>n4"} {
		if strings.Contains(hint, forbidden) {
			t.Fatalf("component fragments invented inter-component bridge %q:\n%s", forbidden, hint)
		}
	}
}

func TestEmitAnswerDocumentRejectSignal_RequiredDiagramDoesNotOfferRemoval(t *testing.T) {
	e := &answerDocumentEvaluator{diagramRequired: true}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document",
		Success:  false,
		Repair: &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
			},
		},
	}}

	got := e.emitAnswerDocumentRejectSignal(ctx, obs)
	if strings.Contains(got.Hint, "remove the optional diagram block") ||
		strings.Contains(got.Hint, "typed topology authoring template") {
		t.Fatalf("required diagram must stay on ordinary correction path:\n%s", got.Hint)
	}
}

func TestEmitPatchRejectFullRewriteSignal_RequiredDiagramCannotBeRemoved(t *testing.T) {
	e := &answerDocumentEvaluator{diagramRequired: true}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document_patch",
			Success:  false,
			Repair: &types.ToolRepair{
				Code: "answer_doc_pre_emit_contract",
				Metadata: map[string]string{
					"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
					types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
				},
			},
		},
	}

	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested || got.HintKey != "answer_doc.patch_correct" {
		t.Fatalf("required diagram must stay on ordinary repair lane, got %+v", got)
	}
	if strings.Contains(got.Hint, "remove the optional diagram block") {
		t.Fatalf("required diagram hint must not offer removal of the required diagram:\n%s", got.Hint)
	}
}

func TestEmitPatchRejectFullRewriteSignal_RequiredDiagramRepeatsExactTypedCapsule(t *testing.T) {
	e := &answerDocumentEvaluator{diagramRequired: true}
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	obs := LoopObservation{LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document_patch",
		Success:  false,
		Repair: &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
			},
		},
	}}

	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested || got.HintKey != "answer_doc.patch_required_diagram_call_edge" {
		t.Fatalf("required diagram-only call reject should select exact-capsule lane, got %+v", got)
	}
	for _, want := range []string{
		"REQUIRED diagram",
		"put the rejected diagram block in `replace_blocks`",
		"participant n1 as Orchestrator.runAnalyzePhase",
		"participant n2 as Orchestrator.dispatchStage",
		"n1->>n2: AUTHOR_BUSINESS_ACTION",
		`edge_anchors_json=` + "`" + `[{"from_node":"n1","to_node":"n2","from_identity":"Orchestrator.runAnalyzePhase","to_identity":"Orchestrator.dispatchStage","relation_kind":"call"}]` + "`",
		"same typed evidence consumed by the validator",
		"does not write a visible label or rewrite the model's prose, ordering, or conclusions",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("required exact-capsule hint missing %q:\n%s", want, got.Hint)
		}
	}
	if strings.Contains(got.Hint, "remove_block_ids") || strings.Contains(got.Hint, "remove the optional diagram") {
		t.Fatalf("required exact-capsule lane must never offer diagram removal:\n%s", got.Hint)
	}
	if e.forceFullEmitNext || !e.preferPatchNext {
		t.Fatalf("required exact-capsule recovery must remain patch-local: forceFull=%t preferPatch=%t",
			e.forceFullEmitNext, e.preferPatchNext)
	}
}

func TestEmitAnswerDocumentRejectSignal_RequiredDiagramRepeatsExactTypedCapsule(t *testing.T) {
	e := &answerDocumentEvaluator{diagramRequired: true}
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	obs := LoopObservation{LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document",
		Success:  false,
		Repair: &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
			},
		},
	}}

	got := e.emitAnswerDocumentRejectSignal(ctx, obs)
	if !got.HintRequested || !strings.Contains(got.HintKey, "required-diagram-call-edge") {
		t.Fatalf("first required diagram-only reject should select exact-capsule lane, got %+v", got)
	}
	for _, want := range []string{
		"Use `emit_answer_document_patch`",
		"REQUIRED diagram",
		"participant n1 as Orchestrator.runAnalyzePhase",
		"n1->>n2: AUTHOR_BUSINESS_ACTION",
		`edge_anchors_json=` + "`" + `[{"from_node":"n1","to_node":"n2","from_identity":"Orchestrator.runAnalyzePhase","to_identity":"Orchestrator.dispatchStage","relation_kind":"call"}]` + "`",
		"Visible node labels, edge/message labels, and Notes remain model-authored",
		"source locations are evidence metadata; do not copy them as the primary visible wording",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("first-reject required exact-capsule hint missing %q:\n%s", want, got.Hint)
		}
	}
	if strings.Contains(got.Hint, "remove_block_ids") || strings.Contains(got.Hint, "remove the optional diagram") {
		t.Fatalf("required first-reject recovery must not offer removal:\n%s", got.Hint)
	}
}

func TestEmitPatchRejectFullRewriteSignal_RequiredFlowUsesTypedRelationBoundaryWhenWholeDiagramWithheld(t *testing.T) {
	e := &answerDocumentEvaluator{diagramRequired: true}
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
	obs := LoopObservation{LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document_patch",
		Success:  false,
		Repair: &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
			},
		},
	}}

	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested || got.HintKey != "answer_doc.patch_required_diagram_relation_boundary" {
		t.Fatalf("required flow without a whole-diagram capsule should use the typed boundary lane, got %+v", got)
	}
	for _, want := range []string{
		"REQUIRED source diagram",
		"use each exact relation recipe below at most once",
		"node_alias[n1]=`Orchestrator.runAnalyzePhase`",
		"edge_recipe[1]=`n1 -> n2`",
		`edge_anchor_json=` + "`" + `{"from_node":"n1","to_node":"n2","from_identity":"Orchestrator.runAnalyzePhase","to_identity":"Orchestrator.dispatchStage","relation_kind":"call"}` + "`",
		"typed relation boundary, not a complete-flow claim",
		"exact node alias as the Mermaid node/participant ID",
		"Visible node labels, edge/message labels, and Notes remain model-authored",
		"source locations are evidence metadata; do not copy them as the primary visible wording",
		"does not rewrite the model's prose, ordering, or conclusion",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("typed relation-boundary hint missing %q:\n%s", want, got.Hint)
		}
	}
	if strings.Contains(got.Hint, "exact `node_alias` identity as the first visible label") {
		t.Fatalf("local repair must not force internal identity into primary display copy:\n%s", got.Hint)
	}
	if strings.Contains(got.Hint, "Boundary carrier placement") {
		t.Fatalf("a relation-only repair without uncovered participants must not teach an unnecessary carrier split:\n%s", got.Hint)
	}
	if strings.Contains(got.Hint, "remove_block_ids") || strings.Contains(got.Hint, "remove the optional diagram") {
		t.Fatalf("required boundary recovery must not offer diagram removal:\n%s", got.Hint)
	}
	if e.forceFullEmitNext || !e.preferPatchNext {
		t.Fatalf("required boundary recovery must remain patch-local: forceFull=%t preferPatch=%t",
			e.forceFullEmitNext, e.preferPatchNext)
	}
}

func TestRequiredDiagramParticipantRetryUsesProducerCompactDeltaOnFullAndPatchRejects(t *testing.T) {
	const delta = `{"version":1,"mismatches":[{"block_id":"diagram-flow-1","participant":"BusContext","issue":"available_typed_incident_edge_not_rendered"}],"actions":"repair_action[BusContext]=reuse_one_existing_typed_candidate","candidates":"typed_candidate[BusContext][1]={relation_kind:\"argument_flow\",from_identity:\"o.busCtx\",to_identity:\"BuildAgentContext\",participant_endpoint_side:\"from\",participant_node_id:\"BusContext\"}"}`
	result := func(toolName string) *types.ToolResult {
		return &types.ToolResult{
			ToolName: toolName, Success: false,
			Repair: &types.ToolRepair{
				Code: "answer_doc_pre_emit_contract",
				Metadata: map[string]string{
					"violation_kinds":                                     string(types.ViolDiagramParticipantCoverage),
					types.ToolRepairMetaOffendingBlockKinds:               string(types.BlockDiagram),
					types.ToolRepairMetaDiagramParticipantRepairDeltaJSON: delta,
				},
			},
		}
	}
	assertCompact := func(t *testing.T, hint string) {
		t.Helper()
		for _, want := range []string{
			"producer-owned delta", `"participant":"BusContext"`,
			`typed_candidate[BusContext][1]`, "select at most one candidate",
			"must not become visible diagram wording", "system has not selected",
			"diagram_boundary_replacements", "diagram_edge_edits",
		} {
			if !strings.Contains(hint, want) {
				t.Fatalf("compact retry missing %q:\n%s", want, hint)
			}
		}
		for _, forbidden := range []string{
			"Verified component fragment", "For every typed incident_required participant",
			"node_alias[n1]", "Typed topology component templates",
		} {
			if strings.Contains(hint, forbidden) {
				t.Fatalf("compact retry repeated full authority payload %q:\n%s", forbidden, hint)
			}
		}
		if len(hint) > 6000 {
			t.Fatalf("compact participant patch hint unexpectedly large: %d bytes", len(hint))
		}
	}

	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
	patchEvaluator := &answerDocumentEvaluator{diagramRequired: true}
	patchSignal := patchEvaluator.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result("emit_answer_document_patch")})
	if !patchSignal.HintRequested || patchSignal.HintKey != "answer_doc.patch_required_diagram_relation_boundary" {
		t.Fatalf("patch reject must route through the compact required-diagram lane: %+v", patchSignal)
	}
	assertCompact(t, patchSignal.Hint)

	fullEvaluator := &answerDocumentEvaluator{diagramRequired: true}
	fullSignal := fullEvaluator.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: result("emit_answer_document")})
	if !fullSignal.HintRequested || !fullEvaluator.preferPatchNext {
		t.Fatalf("full reject must switch to compact patch recovery: %+v", fullSignal)
	}
	assertCompact(t, fullSignal.Hint)
}

func TestRequiredDiagramMixedParticipantAndRelationDeltasStayInOneRetryGeneration(t *testing.T) {
	const participantDelta = `{"version":1,"mismatches":[{"block_id":"flow","participant":"analyze","issue":"required_participant_identity_not_visible"}],"actions":"repair_action[analyze]=add_only_the_missing_visible_identity","candidates":"typed_candidate[analyze][1]={relation_kind:\"precedence\",from_identity:\"analyzer\",to_identity:\"explorer\"}"}`
	const staleProducerRef = "rf1-000000000000000000000000"
	const relationDelta = `{"version":1,"failures":[{"failure_ref":"` + staleProducerRef + `","block_id":"flow","issue":"call_edge_unproven","relation_kind":"call","from_node":"A","to_node":"B","from_identity":"Owner.run","to_identity":"Worker.do","body_occurrence":1}],"preserve_unlisted_edges":true}`

	newContextAndResult := func(toolName string) (*types.AgentContext, *types.ToolResult) {
		mut := types.NewMutableState("mixed participant and relation repair")
		mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
			DocumentModel: "v2",
			Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
				{
					ID: "flow", Kind: types.BlockDiagram,
					Diagram: &types.AnswerDiagramBlock{
						Kind: types.DiagramSequence, Language: "mermaid",
						Body: "sequenceDiagram\n participant A as Owner\n participant B as Worker\n A->>B: run",
					},
					EdgeAnchors: []types.DiagramEdgeAnchor{{
						FromNode: "A", ToNode: "B", FromIdentity: "Owner.run", ToIdentity: "Worker.do",
						RelationKind: types.DiagramRelCall,
					}},
				},
			},
		})
		ctx := &types.AgentContext{Mutable: mut}
		return ctx, &types.ToolResult{
			ToolName: toolName, Success: false,
			Repair: &types.ToolRepair{
				Code: "answer_doc_pre_emit_contract",
				Metadata: map[string]string{
					"violation_kinds": strings.Join([]string{
						string(types.ViolDiagramParticipantCoverage),
						string(types.ViolDiagramCallEdgeUnproven),
					}, ","),
					types.ToolRepairMetaOffendingBlockKinds:               string(types.BlockDiagram),
					types.ToolRepairMetaDiagramParticipantRepairDeltaJSON: participantDelta,
					types.ToolRepairMetaDiagramRelationRepairDeltaJSON:    relationDelta,
				},
			},
		}
	}

	for _, toolName := range []string{"emit_answer_document", "emit_answer_document_patch"} {
		t.Run(toolName, func(t *testing.T) {
			ctx, result := newContextAndResult(toolName)
			e := &answerDocumentEvaluator{diagramRequired: true, mu: ctx.Mutable}
			var signal LoopSignal
			if toolName == "emit_answer_document" {
				signal = e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: result})
			} else {
				signal = e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
			}
			if !signal.HintRequested || !strings.Contains(signal.HintKey, "joint") || !e.preferPatchNext {
				t.Fatalf("mixed deltas must select one joint patch retry: %+v", signal)
			}
			lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
			if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].FailureRef == "" ||
				lease.Failures[0].FailureRef == staleProducerRef {
				t.Fatalf("joint retry must install a live same-generation relation lease: %+v", lease)
			}
			for _, want := range []string{
				`"participant_delta"`, `"participant":"analyze"`,
				`"relation_delta"`, `"from_identity":"Owner.run"`,
				`"failure_ref":"` + lease.Failures[0].FailureRef + `"`,
				"ONE atomic patch", "`diagram_boundary_replacements` and `diagram_edge_edits` may appear together",
				"candidate is permission, not a required edge", "preserve_unlisted_edges=true",
				"system has not selected, added, removed, relabelled, reversed, reconnected, or rewritten",
			} {
				if !strings.Contains(signal.Hint, want) {
					t.Fatalf("joint retry missing %q:\n%s", want, signal.Hint)
				}
			}
			if toolName == "emit_answer_document_patch" {
				for _, want := range []string{
					"failed call was not published",
					"newly issued refs/delta are the sole executable authority over the live patch base",
					"do not replay refs or operations from older attempts",
					"patch preservation for every unlisted carrier",
				} {
					if !strings.Contains(signal.Hint, want) {
						t.Fatalf("joint rejected transaction missing current-generation instruction %q:\n%s", want, signal.Hint)
					}
				}
			}
			if strings.Contains(signal.Hint, staleProducerRef) ||
				strings.Contains(signal.Hint, "use each exact relation recipe below at most once") {
				t.Fatalf("joint retry leaked stale/full-authority carrier:\n%s", signal.Hint)
			}
		})
	}
}

func TestRequiredDiagramParticipantOnlyCandidateGetsLiveSameGenerationAdditionRef(t *testing.T) {
	const participantDelta = `{"version":1,"mismatches":[{"block_id":"flow","participant":"BusContext","issue":"available_typed_incident_edge_not_rendered"}],"actions":"repair_action[BusContext]=reuse_one_existing_typed_candidate","candidates":"typed_candidate[BusContext][1]={relation_kind:\"argument_flow\",from_identity:\"o.busCtx\",to_identity:\"ctxbuilder.BuildAgentContext\"}"}`
	const relationDelta = `{"version":1,"failures":[],"preserve_unlisted_edges":true,"allowed_additions":[{"block_id":"flow","relation_kind":"argument_flow","from_identity":"o.busCtx","to_identity":"ctxbuilder.BuildAgentContext","source":"internal/orchestrator/extract_work.go:15"}]}`

	for _, toolName := range []string{"emit_answer_document", "emit_answer_document_patch"} {
		t.Run(toolName, func(t *testing.T) {
			mut := types.NewMutableState("participant-only current-generation addition")
			mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
				{ID: "flow", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n BusContext[BusContext]",
				}},
			}})
			ctx := &types.AgentContext{Mutable: mut}
			result := &types.ToolResult{ToolName: toolName, Success: false, Repair: &types.ToolRepair{
				Code: "answer_doc_pre_emit_contract",
				Metadata: map[string]string{
					types.ToolRepairMetaDiagramParticipantRepairDeltaJSON: participantDelta,
					types.ToolRepairMetaDiagramRelationRepairDeltaJSON:    relationDelta,
				},
			}}
			e := &answerDocumentEvaluator{diagramRequired: true, mu: mut}
			var signal LoopSignal
			if toolName == "emit_answer_document" {
				signal = e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: result})
			} else {
				signal = e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
			}
			lease := mut.AnswerDiagramRelationRepairLease()
			if !signal.HintRequested || !strings.Contains(signal.HintKey, "joint") || lease == nil ||
				len(lease.Failures) != 0 || len(lease.AllowedAdditions) != 1 {
				t.Fatalf("participant-only candidate must install one executable joint retry generation: signal=%+v lease=%+v", signal, lease)
			}
			liveRef := lease.AllowedAdditions[0].AdditionRef
			if liveRef == "" || !strings.Contains(signal.Hint, `"addition_ref":"`+liveRef+`"`) {
				t.Fatalf("hint must publish the exact ref owned by the executor's live lease: ref=%q hint=%s", liveRef, signal.Hint)
			}
			for _, want := range []string{"current-generation atomic addition capability", "you author both visible endpoints and the business label", "preserve_unlisted_edges=true"} {
				if !strings.Contains(signal.Hint, want) {
					t.Fatalf("addition-only retry missing %q: %s", want, signal.Hint)
				}
			}
			if strings.Contains(signal.Hint, "choose only a listed failure action") {
				t.Fatalf("addition-only retry must not teach a nonexistent failure_ref lane: %s", signal.Hint)
			}
		})
	}
}

func TestRequiredDiagramStaleBoundariesGetLiveLocalRefsOnFullAndPatchRejects(t *testing.T) {
	const delta = `{"version":1,"mismatches":[{"block_id":"flow","participant":"Analyzer","issue":"stale_boundary_for_connected_participant"},{"block_id":"flow","participant":"Explorer","issue":"stale_boundary_for_connected_participant"}],"actions":"remove_stale_boundary"}`
	for _, toolName := range []string{"emit_answer_document", "emit_answer_document_patch"} {
		t.Run(toolName, func(t *testing.T) {
			mut := types.NewMutableState("participant boundary refs")
			mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
				{ID: "flow", Kind: types.BlockDiagram,
					Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n Analyzer --> Explorer"},
					ParticipantBoundaries: []types.DiagramParticipantBoundary{
						{Participant: "Analyzer", Status: types.DiagramParticipantBoundaryUnproven},
						{Participant: "Explorer", Status: types.DiagramParticipantBoundaryUnproven},
						{Participant: "Keep", Status: types.DiagramParticipantBoundaryUnproven},
					},
				},
			}})
			ctx := &types.AgentContext{Mutable: mut}
			result := &types.ToolResult{ToolName: toolName, Success: false, Repair: &types.ToolRepair{
				Code: "answer_doc_pre_emit_contract",
				Metadata: map[string]string{
					types.ToolRepairMetaDiagramParticipantRepairDeltaJSON: delta,
					"violation_kinds":                       string(types.ViolDiagramParticipantCoverage),
					types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
				},
			}}
			e := &answerDocumentEvaluator{diagramRequired: true, mu: mut}
			var signal LoopSignal
			if toolName == "emit_answer_document" {
				signal = e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: result})
			} else {
				signal = e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
			}
			lease := mut.AnswerDiagramRelationRepairLease()
			if !signal.HintRequested || lease == nil || len(lease.ParticipantBoundaryFailures) != 2 {
				t.Fatalf("stale boundaries must install two live local capabilities: signal=%+v lease=%+v", signal, lease)
			}
			for _, failure := range lease.ParticipantBoundaryFailures {
				if failure.BoundaryRef == "" || !strings.Contains(signal.Hint, `"boundary_ref":"`+failure.BoundaryRef+`"`) ||
					!failure.AllowsBoundaryAction("remove_boundary") {
					t.Fatalf("hint and executor must share the exact ref/action: failure=%+v hint=%s", failure, signal.Hint)
				}
			}
			if !strings.Contains(signal.Hint, "diagram_boundary_edits") ||
				strings.Contains(signal.Hint, "prefer `diagram_boundary_replacements` when only participant_boundaries change") {
				t.Fatalf("local refs must supersede whole-array boundary teaching: %s", signal.Hint)
			}
		})
	}
}

func TestRequiredDiagramRelationRetryUsesProducerCompactDeltaBeforeFullAuthority(t *testing.T) {
	const staleProducerRef = "rf1-000000000000000000000000"
	const delta = `{"version":1,"failures":[{"failure_ref":"` + staleProducerRef + `","block_id":"pipeline-diagram","issue":"data_flow_edge_unproven","relation_kind":"data_flow","from_node":"BC","to_node":"E","from_identity":"BusContext","to_identity":"Explorer","body_occurrence":1}],"preserve_unlisted_edges":true,"allowed_additions":[{"block_id":"pipeline-diagram","relation_kind":"call","from_identity":"Orchestrator.applyStageOutput","to_identity":"o.busCtx.Mutable.SetTurnAArtifacts","source":"internal/orchestrator/orchestrator.go:8442"}],"candidate_alternatives":"typed_candidate[BusContext][1]={relation_kind:\"call\",from_identity:\"Orchestrator.applyStageOutput\",to_identity:\"o.busCtx.Mutable.SetTurnAArtifacts\",candidate_scope:\"local_operation_only\",requested_relation_closure:\"unproven\",retain_participant_boundary:true}"}`
	result := func(toolName string) *types.ToolResult {
		return &types.ToolResult{
			ToolName: toolName, Success: false,
			Repair: &types.ToolRepair{
				Code: "answer_doc_pre_emit_contract",
				Metadata: map[string]string{
					"violation_kinds":                                  string(types.ViolDiagramCallEdgeUnproven),
					types.ToolRepairMetaOffendingBlockKinds:            string(types.BlockDiagram),
					types.ToolRepairMetaDiagramRelationRepairDeltaJSON: delta,
				},
			},
		}
	}
	assertCompact := func(t *testing.T, signal LoopSignal) {
		t.Helper()
		if !signal.HintRequested ||
			(!strings.Contains(signal.HintKey, "required_diagram_relation_delta") &&
				!strings.Contains(signal.HintKey, "required-diagram-relation-delta")) {
			t.Fatalf("relation reject must select the compact delta lane: %+v", signal)
		}
		for _, want := range []string{
			`"from_node":"BC"`, `"to_node":"E"`, "preserve_unlisted_edges=true",
			`"body_occurrence":1`, "omit block_id, match, occurrence, and body_occurrence",
			`"allowed_additions"`, `"from_identity":"Orchestrator.applyStageOutput"`,
			"The allowed rows are permissions, not required edges",
			"diagram_edge_edits", "This generation publishes no action=attach capability",
			"system has not selected, added, removed, relabelled, reversed, or reconnected",
		} {
			if !strings.Contains(signal.Hint, want) {
				t.Fatalf("compact relation hint missing %q:\n%s", want, signal.Hint)
			}
		}
		for _, forbidden := range []string{
			"Typed topology authoring template", "Verified component fragment",
			"use each exact relation recipe below at most once", "node_alias[n1]",
			"typed_candidate[BusContext][1]", "local_operation_only",
		} {
			if strings.Contains(signal.Hint, forbidden) {
				t.Fatalf("compact relation hint repeated full authority %q:\n%s", forbidden, signal.Hint)
			}
		}
		if len(signal.Hint) > 6000 {
			t.Fatalf("compact relation retry unexpectedly large: %d bytes", len(signal.Hint))
		}
	}

	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
	ctx.Mutable.SetPendingAnswerDocumentPatchBase(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		{ID: "pipeline-diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n BC --> E"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "BC", ToNode: "E", FromIdentity: "BusContext", ToIdentity: "Explorer",
				RelationKind: types.DiagramRelDataFlow,
			}},
		},
	}})
	patchEvaluator := &answerDocumentEvaluator{diagramRequired: true}
	patchResult := result("emit_answer_document_patch")
	patchSignal := patchEvaluator.emitPatchRejectFullRewriteSignal(
		ctx, LoopObservation{LastToolResult: patchResult},
	)
	assertCompact(t, patchSignal)
	if lease := ctx.Mutable.AnswerDiagramRelationRepairLease(); lease == nil || len(lease.Failures) != 1 ||
		lease.Failures[0].FromNode != "BC" || len(lease.AllowedAdditions) != 1 ||
		lease.AllowedAdditions[0].FromIdentity != "Orchestrator.applyStageOutput" {
		t.Fatalf("compact relation lane must install the exact typed patch lease: %+v", lease)
	} else {
		if lease.Failures[0].FailureRef == "" || lease.Failures[0].FailureRef == staleProducerRef {
			t.Fatalf("live lease must replace the producer snapshot ref: %+v", lease.Failures[0])
		}
		if !strings.Contains(patchSignal.Hint, `"failure_ref":"`+lease.Failures[0].FailureRef+`"`) ||
			strings.Contains(patchSignal.Hint, staleProducerRef) {
			t.Fatalf("retry hint must publish only the executor's live lease ref %q:\n%s", lease.Failures[0].FailureRef, patchSignal.Hint)
		}
		if raw := patchResult.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]; !strings.Contains(raw, `"failure_ref":"`+lease.Failures[0].FailureRef+`"`) || strings.Contains(raw, staleProducerRef) {
			t.Fatalf("producer metadata must be canonicalized to the live lease ref: %s", raw)
		}
	}

	fullEvaluator := &answerDocumentEvaluator{diagramRequired: true}
	assertCompact(t, fullEvaluator.emitAnswerDocumentRejectSignal(
		ctx, LoopObservation{LastToolResult: result("emit_answer_document")},
	))
	if !fullEvaluator.preferPatchNext {
		t.Fatal("full reject compact relation recovery must switch to patch mode")
	}
}

func TestDiagramRelationRetryDoesNotAdvertiseAttachWithoutExactCapability(t *testing.T) {
	result := &types.ToolResult{ToolName: "emit_answer_document", Success: false, Repair: &types.ToolRepair{
		Code: "answer_doc_pre_emit_contract",
		Metadata: map[string]string{types.ToolRepairMetaDiagramRelationRepairDeltaJSON: `{
			"version":1,
			"failures":[{"failure_ref":"rf1-visible","block_id":"flow","issue":"missing_call_anchor","relation_kind":"call","from_node":"A","to_node":"B","from_identity":"pkg.A","to_identity":"pkg.B","target_carrier":"visible_body_edge","allowed_actions":["remove","replace"],"body_occurrence":1}],
			"preserve_unlisted_edges":true,
			"allowed_additions":[{"addition_ref":"ra1-unpaired","block_id":"flow","relation_kind":"callback","from_identity":"pkg.C","to_identity":"pkg.D","source":"source.go:10"}]
		}`},
	}}
	hint, ok := answerDocRequiredDiagramRelationDeltaPatchHint(result, false)
	if !ok || !strings.Contains(hint, "This generation publishes no action=attach capability") ||
		!strings.Contains(hint, "never combine a failure_ref and addition_ref") {
		t.Fatalf("unpaired generation must publish the exact negative capability: ok=%v\n%s", ok, hint)
	}
	for _, forbidden := range []string{
		"an action=attach branch exists", "attach requires both refs", "action=attach is valid only",
	} {
		if strings.Contains(hint, forbidden) {
			t.Fatalf("unpaired retry advertised an unavailable attach grammar %q:\n%s", forbidden, hint)
		}
	}
}

func TestDiagramRelationRetryMentionsAttachOnlyForExactLivePair(t *testing.T) {
	result := &types.ToolResult{ToolName: "emit_answer_document", Success: false, Repair: &types.ToolRepair{
		Code: "answer_doc_pre_emit_contract",
		Metadata: map[string]string{types.ToolRepairMetaDiagramRelationRepairDeltaJSON: `{
			"version":1,
			"failures":[{"failure_ref":"rf1-visible","block_id":"flow","issue":"missing_relation_anchor","from_node":"A","to_node":"B","from_identity":"pkg.A","to_identity":"pkg.B","target_carrier":"visible_body_edge","allowed_actions":["remove","attach"],"body_occurrence":1}],
			"preserve_unlisted_edges":true,
			"allowed_additions":[{"addition_ref":"ra1-paired","block_id":"flow","relation_kind":"argument_flow","from_identity":"pkg.A","to_identity":"pkg.B","source":"source.go:10"}]
		}`},
	}}
	hint, ok := answerDocRequiredDiagramRelationDeltaPatchHint(result, false)
	if !ok || !strings.Contains(hint, "action=attach is valid only from a schema branch that fixes both exact opaque ref values") ||
		strings.Contains(hint, "publishes no action=attach capability") {
		t.Fatalf("exact live pair must be taught only through the paired schema branch: ok=%v\n%s", ok, hint)
	}
}

func TestRequiredDiagramRelationRetryDoesNotInstallIdentityOnlyDeadLease(t *testing.T) {
	const delta = `{"version":1,"failures":[{"block_id":"pipeline-diagram","issue":"typed_anchor_without_visible_edge","relation_kind":"precedence","from_identity":"analyzer","to_identity":"explorer"}],"preserve_unlisted_edges":true}`
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch", Success: false,
		Repair: &types.ToolRepair{Code: "answer_doc_pre_emit_contract", Metadata: map[string]string{
			types.ToolRepairMetaDiagramRelationRepairDeltaJSON: delta,
		}},
	}
	hint, ok := answerDocRequiredDiagramRelationDeltaPatchHint(result, true)
	if !ok || !strings.Contains(hint, `"from_identity":"analyzer"`) ||
		!strings.Contains(hint, `"to_identity":"explorer"`) {
		t.Fatalf("identity-only typed locator must reach the model-owned patch lane: ok=%v hint=%s", ok, hint)
	}
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	if installAnswerDocDiagramRelationRepairLease(ctx, nil, result, true) {
		t.Fatal("identity-only typed locator without an executable carrier must not install a local lease")
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease != nil {
		t.Fatalf("dead identity-only lease leaked into the dynamic patch surface: %+v", lease)
	}
}

func TestRequiredDiagramRelationRetryKeepsExecutableRowsWhenOneTypedRowCannotBind(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n A -->|unsupported| B",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Alpha.Run", ToIdentity: "Beta.Handle",
			RelationKind: types.DiagramRelCall, VisibleLabel: "unsupported",
		}},
	}}}
	mut := types.NewMutableState("mixed executable relation rows")
	mut.SetPendingAnswerDocumentPatchBase(base)
	ctx := &types.AgentContext{Mutable: mut}
	const delta = `{"version":1,"failures":[` +
		`{"block_id":"flow","issue":"call_edge_unproven","relation_kind":"call","from_node":"A","to_node":"B","from_identity":"Alpha.Run","to_identity":"Beta.Handle","body_occurrence":1},` +
		`{"block_id":"flow","issue":"typed_anchor_without_visible_edge","relation_kind":"call","from_identity":"Ghost.Run","to_identity":"Missing.Handle"}` +
		`],"preserve_unlisted_edges":true,"allowed_additions":[` +
		`{"block_id":"flow","relation_kind":"argument_flow","from_identity":"Input.Value","to_identity":"Worker.Accept","source":"worker.go:17"},` +
		`{"block_id":"flow","relation_kind":"data_flow","from_identity":"Bad.Value","to_identity":"Bad.Accept","from_node_ids":[""],"source":"worker.go:18"}` +
		`]}`
	result := &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false, Repair: &types.ToolRepair{
		Code: "answer_doc_pre_emit_contract", Metadata: map[string]string{
			types.ToolRepairMetaDiagramRelationRepairDeltaJSON: delta,
		},
	}}
	if !installAnswerDocDiagramRelationRepairLease(ctx, mut, result, true) {
		t.Fatal("one non-bindable typed row must not suppress unrelated executable repair capabilities")
	}
	lease := mut.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 1 ||
		lease.Failures[0].FromNode != "A" || lease.AllowedAdditions[0].FromIdentity != "Input.Value" {
		t.Fatalf("unexpected executable subset: %+v", lease)
	}
	raw := result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]
	for _, want := range []string{`"failure_ref":"` + lease.Failures[0].FailureRef + `"`, `"addition_ref":"` + lease.AllowedAdditions[0].AdditionRef + `"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("canonical live subset missing %q: %s", want, raw)
		}
	}
	if strings.Contains(raw, "Ghost.Run") || strings.Contains(raw, "Missing.Handle") {
		t.Fatalf("non-executable typed row leaked as a live capability: %s", raw)
	}
}

func TestRelationRepairPatchBasePrefersPendingGenerationAcrossMutableCarriers(t *testing.T) {
	stale := types.NewMutableState("stale dispatch carrier")
	stale.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "stale-summary", Kind: types.BlockSummary, Text: "old",
	}, {
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n X"},
	}}})
	fresh := types.NewMutableState("fresh evaluator carrier")
	fresh.SetPendingAnswerDocumentPatchBase(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "fresh-summary", Kind: types.BlockSummary, Text: "new",
	}, {
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A -->|unsupported| B"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Alpha.Run", ToIdentity: "Beta.Handle",
			RelationKind: types.DiagramRelCall, VisibleLabel: "unsupported",
		}},
	}}})
	ctx := &types.AgentContext{Mutable: stale}
	const delta = `{"version":1,"failures":[{"block_id":"flow","issue":"call_edge_unproven","relation_kind":"call","from_node":"A","to_node":"B","from_identity":"Alpha.Run","to_identity":"Beta.Handle","body_occurrence":1}],"preserve_unlisted_edges":true}`
	result := &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false, Repair: &types.ToolRepair{
		Code: "answer_doc_pre_emit_contract", Metadata: map[string]string{
			types.ToolRepairMetaDiagramRelationRepairDeltaJSON: delta,
		},
	}}
	if !installAnswerDocDiagramRelationRepairLease(ctx, fresh, result, true) {
		t.Fatal("new pending generation must win over an older rejected document on another mutable carrier")
	}
	for name, mut := range map[string]*types.MutableState{"dispatch": stale, "evaluator": fresh} {
		lease := mut.AnswerDiagramRelationRepairLease()
		if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].FromNode != "A" {
			t.Fatalf("%s mutable did not receive the fresh generation lease: %+v", name, lease)
		}
	}
	roster := answerDocPatchBaseBlockRosterHint(ctx, fresh, "")
	if !strings.Contains(roster, `"id":"fresh-summary"`) || strings.Contains(roster, `"id":"stale-summary"`) {
		t.Fatalf("retry roster did not follow the fresh pending generation: %s", roster)
	}
}

func TestRequiredDiagramRelationRetryInstallsLabelPairRelabelRef(t *testing.T) {
	mut := types.NewMutableState("label pair retry")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A -->|model wording| B",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Alpha.Run", ToIdentity: "Beta.Accept",
			RelationKind: types.DiagramRelCall,
		}},
	}}})
	ctx := &types.AgentContext{Mutable: mut}
	const delta = `{"version":1,"failures":[{"block_id":"flow","issue":"diagram_typed_recipe_missing_visible_label","relation_kind":"call","from_node":"A","to_node":"B","from_identity":"Alpha.Run","to_identity":"Beta.Accept","body_occurrence":1}],"preserve_unlisted_edges":true}`
	result := &types.ToolResult{
		ToolName: "emit_answer_document", Success: false,
		Repair: &types.ToolRepair{Code: "answer_doc_pre_emit_contract", Metadata: map[string]string{
			types.ToolRepairMetaDiagramRelationRepairDeltaJSON: delta,
		}},
	}
	if !installAnswerDocDiagramRelationRepairLease(ctx, nil, result, true) {
		t.Fatal("label-pair producer delta must install a retry-local lease")
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.Failures) != 1 ||
		lease.Failures[0].TargetCarrier != types.AnswerDiagramRelationRepairCarrierLabelPair ||
		!lease.Failures[0].AllowsAction("relabel") || lease.Failures[0].FailureRef == "" {
		t.Fatalf("label-pair lease is not executable: %+v", lease)
	}
	raw := result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]
	if !strings.Contains(raw, `"failure_ref":"`+lease.Failures[0].FailureRef+`"`) ||
		!strings.Contains(raw, `"allowed_actions":["relabel"]`) {
		t.Fatalf("retry metadata did not publish the live relabel ref: %s", raw)
	}
	hint, ok := answerDocRequiredDiagramRelationDeltaPatchHint(result, false)
	if !ok || !strings.Contains(hint, `"failure_ref":"`+lease.Failures[0].FailureRef+`"`) ||
		!strings.Contains(hint, `"allowed_actions":["relabel"]`) {
		t.Fatalf("model retry hint did not receive the exact label-pair capability: ok=%v\n%s", ok, hint)
	}
}

func TestRequiredDiagramRelationRetryPublishesOptionalModelOwnedOrphanCleanup(t *testing.T) {
	mut := types.NewMutableState("optional orphan cleanup")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			" participant X as InternalController",
			" participant A as Analyze",
			" participant C as ContextBoundary",
			" X->>A: unsupported",
			" A->>C: keep",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "X", ToNode: "A", FromIdentity: "Internal.Run", ToIdentity: "Analyze", RelationKind: types.DiagramRelCall},
			{FromNode: "A", ToNode: "C", FromIdentity: "Analyze", ToIdentity: "ContextBoundary", RelationKind: types.DiagramRelDataFlow},
		},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{Participant: "ContextBoundary", Status: types.DiagramParticipantBoundaryUnproven}},
	}}})
	ctx := &types.AgentContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{
			PredicateAxis: types.AxisFlow,
			DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: []types.DiagramParticipantHint{
				{Identity: "Analyze", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Analyze"},
				{Identity: "ContextBoundary", Role: types.DiagramParticipantContextOnly, SourceQuote: "ContextBoundary"},
			}},
		},
		AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
			Required: true, PreferredKinds: []types.DiagramKind{types.DiagramSequence},
			Participants: []types.DiagramParticipantHint{
				{Identity: "Analyze", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Analyze"},
				{Identity: "ContextBoundary", Role: types.DiagramParticipantContextOnly, SourceQuote: "ContextBoundary"},
			},
		}},
	}}
	const delta = `{"version":1,"failures":[{"block_id":"flow","issue":"call_edge_unproven","relation_kind":"call","from_node":"X","to_node":"A","from_identity":"Internal.Run","to_identity":"Analyze","body_occurrence":1}],"preserve_unlisted_edges":true}`
	result := &types.ToolResult{ToolName: "emit_answer_document", Success: false, Repair: &types.ToolRepair{
		Code: "answer_doc_pre_emit_contract", Metadata: map[string]string{
			types.ToolRepairMetaDiagramRelationRepairDeltaJSON: delta,
		},
	}}
	if !installAnswerDocDiagramRelationRepairLease(ctx, nil, result, true) {
		t.Fatal("relation lease installation failed")
	}
	if lease := ctx.Mutable.AnswerDiagramRelationRepairLease(); lease == nil ||
		len(lease.OptionalOrphanCleanups) != 1 || lease.OptionalOrphanCleanups[0].ParticipantID != "X" ||
		!lease.OptionalOrphanCleanups[0].AllowsAction("retain_as_context") {
		t.Fatalf("executor lease must retain the same typed orphan decision: %+v", lease)
	}
	raw := result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]
	for _, want := range []string{
		`"optional_orphan_cleanups"`, `"participant_id":"X"`, `"allowed_actions":["remove_if_isolated","retain_as_context"]`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("typed optional cleanup missing %q: %s", want, raw)
		}
	}
	for _, forbidden := range []string{`"participant_id":"A"`, `"participant_id":"C"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("requested/boundary participant leaked as cleanup candidate %q: %s", forbidden, raw)
		}
	}
	hint, ok := answerDocRequiredDiagramRelationDeltaPatchHint(result, false)
	if !ok {
		t.Fatal("relation delta hint missing")
	}
	for _, want := range []string{"diagram_participant_edits", "remove_if_isolated", "retain_as_context", "selected edits isolate", `"participant_id":"X"`} {
		if !strings.Contains(hint, want) {
			t.Fatalf("model-owned cleanup teaching missing %q:\n%s", want, hint)
		}
	}
}

func TestRequiredDiagramRelationRetryNeverOffersDecoratedRequestedParticipantAsOrphan(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			` BC["BusContext<br/>internal/types/context.go:7593"]`,
			` X["InternalController<br/>internal/controller.go:7"]`,
			` A["Analyze"]`,
			" BC --> A",
			" X --> A",
		}, "\n")},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{
			Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven,
		}},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "BC", ToNode: "A", FromIdentity: "BusContext.Read", ToIdentity: "Analyze.Run", RelationKind: types.DiagramRelCall},
			{FromNode: "X", ToNode: "A", FromIdentity: "InternalController.Run", ToIdentity: "Analyze.Run", RelationKind: types.DiagramRelCall},
		},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "flow", Issue: "call_edge_unproven", FromNode: "BC", ToNode: "A", FromIdentity: "BusContext.Read", ToIdentity: "Analyze.Run", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
		{BlockID: "flow", Issue: "call_edge_unproven", FromNode: "X", ToNode: "A", FromIdentity: "InternalController.Run", ToIdentity: "Analyze.Run", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
	}, nil)
	if lease == nil {
		t.Fatal("test setup did not create a relation lease")
	}
	view := &types.AnswerSemanticView{DiagramParticipantObligations: []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}}
	got := answerDocDiagramOptionalOrphanCleanupCandidates(base, lease, view)
	foundX := false
	for _, candidate := range got {
		if candidate.ParticipantID == "BC" {
			t.Fatalf("decorated requested participant leaked into the orphan cleanup roster: %+v", got)
		}
		foundX = foundX || candidate.ParticipantID == "X"
	}
	if !foundX {
		t.Fatalf("unrelated removable participant should remain model-owned: %+v", got)
	}
}

func TestRequiredDiagramRelationRetryDoesNotPublishUnfulfillableRepeatedPairOrphanCleanup(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			" participant X as InternalController",
			" participant A as Analyze",
			" X->>A: first",
			" X->>A: second",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "X", ToNode: "A", FromIdentity: "Internal.Run", ToIdentity: "Analyze",
			RelationKind: types.DiagramRelCall, VisibleLabel: "first",
		}},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "X", ToNode: "A",
		FromIdentity: "Internal.Run", ToIdentity: "Analyze", RelationKind: types.DiagramRelCall,
		// Zero is intentionally ambiguous across the two same-pair body rows.
		BodyOccurrence: 0,
	}}, nil)
	if lease == nil || len(lease.Failures) != 1 ||
		!lease.Failures[0].AllowsAction(string(types.AnswerDiagramRelationRepairActionRemove)) {
		t.Fatalf("test setup did not mint one remove-capable prior-anchor ref: %+v", lease)
	}
	if got := answerDocDiagramOptionalOrphanCleanupCandidates(base, lease, nil); len(got) != 0 {
		t.Fatalf("one zero-occurrence ref cannot promise removal of two visible occurrences: %+v", got)
	}

	// Two distinct visible-body refs, each bound to one occurrence, can fulfill
	// the orphan contract and therefore remain an advertised model choice.
	base.Blocks[0].EdgeAnchors = nil
	lease = types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "X", ToNode: "A", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
		{BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "X", ToNode: "A", RelationKind: types.DiagramRelCall, BodyOccurrence: 2},
	}, nil)
	got := answerDocDiagramOptionalOrphanCleanupCandidates(base, lease, nil)
	foundX := false
	for _, candidate := range got {
		foundX = foundX || candidate.ParticipantID == "X"
	}
	if !foundX {
		t.Fatalf("two exact refs should authorize the repeated-pair cleanup: %+v", got)
	}
}

func TestRequiredDiagramRelationRetryDoesNotPublishSequenceReferencedOrphanCleanup(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			" participant X as JsonPlugin",
			" participant A as resolve",
			" X->>A: unsupported",
			" Note over X,A: model-authored source context",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "X", ToNode: "A", FromIdentity: "JsonPlugin", ToIdentity: "resolve",
			RelationKind: types.DiagramRelCall,
		}},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "X", ToNode: "A",
		FromIdentity: "JsonPlugin", ToIdentity: "resolve", RelationKind: types.DiagramRelCall, BodyOccurrence: 1,
	}}, nil)
	if lease == nil {
		t.Fatal("test setup did not create relation lease")
	}
	if got := answerDocDiagramOptionalOrphanCleanupCandidates(base, lease, nil); len(got) != 0 {
		t.Fatalf("participant retained by Note must not be advertised as removable orphan: %+v", got)
	}

	base.Blocks[0].Diagram.Body = strings.ReplaceAll(base.Blocks[0].Diagram.Body,
		"\n Note over X,A: model-authored source context", "")
	lease = types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "X", ToNode: "A",
		FromIdentity: "JsonPlugin", ToIdentity: "resolve", RelationKind: types.DiagramRelCall, BodyOccurrence: 1,
	}}, nil)
	if got := answerDocDiagramOptionalOrphanCleanupCandidates(base, lease, nil); len(got) != 2 {
		t.Fatalf("without a non-edge reference both uniquely declared endpoints may remain model-owned cleanup choices: %+v", got)
	}
}

func TestOptionalDiagramRelationRetryUsesProducerCompactDeltaWithSiblingViolation(t *testing.T) {
	const staleProducerRef = "rf1-stale-optional-generation"
	const delta = `{"version":1,"failures":[{"failure_ref":"` + staleProducerRef + `","block_id":"pipeline-diagram","issue":"data_flow_edge_unproven","relation_kind":"data_flow","from_node":"BC","to_node":"E","from_identity":"BusContext","to_identity":"Explorer","body_occurrence":1}],"preserve_unlisted_edges":true}`
	for _, toolName := range []string{"emit_answer_document", "emit_answer_document_patch"} {
		t.Run(toolName, func(t *testing.T) {
			ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
			ctx.Mutable.SetPendingAnswerDocumentPatchBase(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "s1", Kind: types.BlockSummary, Text: "grounded answer"},
				{ID: "pipeline-diagram", Kind: types.BlockDiagram,
					Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n BC --> E"},
					EdgeAnchors: []types.DiagramEdgeAnchor{{
						FromNode: "BC", ToNode: "E", FromIdentity: "BusContext", ToIdentity: "Explorer",
						RelationKind: types.DiagramRelDataFlow,
					}},
				},
				{ID: "ol1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1", Text: "grounded relation"}}},
			}})
			result := &types.ToolResult{ToolName: toolName, Success: false, Repair: &types.ToolRepair{
				Code:   "answer_doc_pre_emit_contract",
				Fields: []string{"blocks[].edge_anchors[] AND blocks[kind=diagram].diagram.body", "blocks[id=ol1].edge_anchors"},
				Hint:   "Repair the diagram relation and the sibling ordered-list relation carrier.",
				Metadata: map[string]string{
					"violation_kinds": strings.Join([]string{
						string(types.ViolDiagramCallEdgeUnproven), string(types.ViolCitation),
					}, ","),
					types.ToolRepairMetaOffendingBlockKinds:                strings.Join([]string{string(types.BlockDiagram), string(types.BlockOrderedList)}, ","),
					types.ToolRepairMetaDiagramRelationRepairDeltaJSON:     delta,
					types.ToolRepairMetaRelationRepairOrdinaryBlockIDsJSON: `["ol1"]`,
				},
			}}
			e := &answerDocumentEvaluator{diagramRequired: false, mu: ctx.Mutable}
			var got LoopSignal
			if toolName == "emit_answer_document" {
				got = e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: result})
			} else {
				got = e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
			}
			lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
			if !got.HintRequested || lease == nil || !lease.AllowTargetDiagramRemoval || len(lease.Failures) != 1 ||
				!reflect.DeepEqual(lease.OrdinaryValidationBlockIDs, []string{"ol1"}) {
				t.Fatalf("optional mixed reject must install one live local relation lease: signal=%+v lease=%+v", got, lease)
			}
			liveRef := lease.Failures[0].FailureRef
			for _, want := range []string{
				"diagram_edge_edits", `"failure_ref":"` + liveRef + `"`,
				"The diagram remains optional", "remove_block_ids",
				"does not discharge or replace the sibling non-diagram corrections",
				"system has not selected, added, removed, relabelled, reversed, or reconnected",
			} {
				if !strings.Contains(got.Hint, want) {
					t.Fatalf("optional compact relation hint missing %q:\n%s", want, got.Hint)
				}
			}
			if strings.Contains(got.Hint, staleProducerRef) || strings.Contains(got.Hint, "No typed topology template is available") {
				t.Fatalf("optional compact lane leaked stale/generic repair authority:\n%s", got.Hint)
			}
			if !e.preferPatchNext {
				t.Fatal("optional compact relation retry must remain on patch surface")
			}
		})
	}
}

func TestRelationRepairScopeRejectStaysOnCompactDeltaLane(t *testing.T) {
	const delta = `{"version":1,"failures":[{"failure_ref":"rf1-current-executor-generation","block_id":"pipeline-diagram","issue":"data_flow_edge_unproven","relation_kind":"data_flow","from_node":"BC","to_node":"E","from_identity":"BusContext","to_identity":"Explorer"}],"preserve_unlisted_edges":true}`
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(&types.AnswerDiagramRelationRepairLease{
		Version: 1,
		Failures: []types.AnswerDiagramRelationRepairFailure{{
			FailureRef: "rf1-current-executor-generation", BlockID: "pipeline-diagram",
			Issue: "data_flow_edge_unproven", TargetCarrier: types.AnswerDiagramRelationRepairCarrierPriorAnchor,
			AllowedActions: []types.AnswerDiagramRelationRepairAction{types.AnswerDiagramRelationRepairActionRemove},
		}},
		Blocks: []types.AnswerDiagramRelationRepairLeaseBlock{{BlockID: "pipeline-diagram", Kind: types.BlockDiagram}},
	})
	e := &answerDocumentEvaluator{diagramRequired: true}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch", Success: false,
		Repair: &types.ToolRepair{
			Code: types.ToolRepairCodeAnswerDocRelationRepairScope,
			Metadata: map[string]string{
				types.ToolRepairMetaDiagramRelationRepairDeltaJSON: delta,
			},
		},
	}
	signal := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !signal.HintRequested || signal.HintKey != "answer_doc.patch_relation_repair_scope" ||
		!strings.Contains(signal.Hint, "do not add any other relation") ||
		!strings.Contains(signal.Hint, "diagram_edge_edits") ||
		!strings.Contains(signal.Hint, "failed call was not published") ||
		!strings.Contains(signal.Hint, "newly issued refs/delta are the sole executable authority over the live patch base") ||
		!strings.Contains(signal.Hint, "do not replay refs or operations from older attempts") {
		t.Fatalf("lease rejection must remain on compact local repair lane: %+v", signal)
	}
	if strings.Contains(signal.Hint, "Typed topology authoring template") || strings.Contains(signal.Hint, "Verified component fragment") {
		t.Fatalf("lease rejection must not reopen the full relation handbook: %s", signal.Hint)
	}
}

func TestRelationRepairScopeForwardsExecutorLiveAdditionsWhenConsumerRebuildIsEmpty(t *testing.T) {
	const liveRef = "ra1-current-executor-generation"
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{
			ID: "pipeline-diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart LR\n BusContext -->|作为参数传递| BuildAgentContext",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "BusContext", ToNode: "BuildAgentContext",
				FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext",
				RelationKind: types.DiagramRelArgumentFlow,
			}},
		},
	}}
	mut := types.NewMutableState("live relation delta forwarding")
	mut.SetLastRejectedAnswerDocumentV2(base)
	mut.SetAnswerDiagramRelationRepairLease(&types.AnswerDiagramRelationRepairLease{
		Version: 1,
		AllowedAdditions: []types.AnswerDiagramRelationRepairCandidate{{
			AdditionRef: liveRef, BlockID: "pipeline-diagram", RelationKind: types.DiagramRelArgumentFlow,
			FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext", Source: "internal/orchestrator/extract_work.go:15",
		}},
		Blocks: []types.AnswerDiagramRelationRepairLeaseBlock{{BlockID: "pipeline-diagram", Kind: types.BlockDiagram}},
	})
	ctx := &types.AgentContext{Mutable: mut}
	e := &answerDocumentEvaluator{diagramRequired: true, mu: mut}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch", Success: false,
		Repair: &types.ToolRepair{
			Code: types.ToolRepairCodeAnswerDocRelationRepairScope,
			Metadata: map[string]string{
				types.ToolRepairMetaDiagramRelationRepairDeltaJSON: `{"version":1,"failures":[],"preserve_unlisted_edges":true,"allowed_additions":[{"addition_ref":"` + liveRef + `","block_id":"pipeline-diagram","relation_kind":"argument_flow","from_identity":"o.busCtx","to_identity":"ctxbuilder.BuildAgentContext","source":"internal/orchestrator/extract_work.go:15"}]}`,
			},
		},
	}

	// The evaluator's patch base is a later generation in which the candidate
	// is already anchored. Re-signing against it correctly produces no lease;
	// this must not erase the executor's still-live generation returned above.
	if installAnswerDocDiagramRelationRepairLease(ctx, mut, result, true) {
		t.Fatal("consumer-side rebuild should be empty for a candidate already anchored in its later patch base")
	}
	signal := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !signal.HintRequested || signal.HintKey != "answer_doc.patch_relation_repair_scope" ||
		!signal.BypassBudget || !signal.BypassThrottle || !strings.Contains(signal.Hint, `"addition_ref":"`+liveRef+`"`) ||
		!strings.Contains(signal.Hint, "complete current additions-only typed capability roster") {
		t.Fatalf("scope retry must forward the executor-owned current generation unchanged: %+v", signal)
	}
	if strings.Contains(signal.Hint, "answer_doc.patch_correct") ||
		strings.Contains(signal.Hint, "ra1-historical-generation") {
		t.Fatalf("scope retry must not fall back to a generic or historical capability lane: %s", signal.Hint)
	}
}

func TestRelationRepairScopeFindsExecutorLeaseOnEvaluatorMutable(t *testing.T) {
	const liveRef = "ra1-executor-evaluator-mutable"
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "pipeline-diagram", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A --> B",
		},
	}}}
	executorMutable := types.NewMutableState("executor relation lease")
	executorMutable.SetLastRejectedAnswerDocumentV2(base)
	executorMutable.SetAnswerDiagramRelationRepairLease(&types.AnswerDiagramRelationRepairLease{
		Version: 1,
		AllowedAdditions: []types.AnswerDiagramRelationRepairCandidate{{
			AdditionRef: liveRef, BlockID: "pipeline-diagram", RelationKind: types.DiagramRelDataFlow,
			FromIdentity: "producer.Value", ToIdentity: "consumer.Value", Source: "pipeline.go:10",
		}},
		Blocks: []types.AnswerDiagramRelationRepairLeaseBlock{{BlockID: "pipeline-diagram", Kind: types.BlockDiagram}},
	})
	// Recovery can temporarily give the evaluator and AgentContext different
	// MutableState carriers. The scope ToolResult was produced from the former;
	// the latter may still carry an older executable generation and must not make
	// the exact current generation look absent.
	ctx := &types.AgentContext{Mutable: types.NewMutableState("agent context without lease")}
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(&types.AnswerDiagramRelationRepairLease{
		Version: 1,
		AllowedAdditions: []types.AnswerDiagramRelationRepairCandidate{{
			AdditionRef: "ra1-stale-agent-context", BlockID: "pipeline-diagram", RelationKind: types.DiagramRelDataFlow,
			FromIdentity: "old.Producer", ToIdentity: "old.Consumer", Source: "old.go:1",
		}},
		Blocks: []types.AnswerDiagramRelationRepairLeaseBlock{{BlockID: "pipeline-diagram", Kind: types.BlockDiagram}},
	})
	e := &answerDocumentEvaluator{diagramRequired: true, mu: executorMutable}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch", Success: false,
		Repair: &types.ToolRepair{
			Code: types.ToolRepairCodeAnswerDocRelationRepairScope,
			Metadata: map[string]string{
				types.ToolRepairMetaDiagramRelationRepairDeltaJSON: `{"version":1,"failures":[],"preserve_unlisted_edges":true,"allowed_additions":[{"addition_ref":"` + liveRef + `","block_id":"pipeline-diagram","relation_kind":"data_flow","from_identity":"producer.Value","to_identity":"consumer.Value","source":"pipeline.go:10"}]}`,
			},
		},
	}

	signal := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !signal.HintRequested || signal.HintKey != "answer_doc.patch_relation_repair_scope" ||
		!strings.Contains(signal.Hint, `"addition_ref":"`+liveRef+`"`) || strings.Contains(signal.Hint, "answer_doc.patch_correct") {
		t.Fatalf("split mutable scope retry must keep the executor generation: %+v", signal)
	}
	mirrored := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if mirrored == nil || len(mirrored.AllowedAdditions) != 1 || mirrored.AllowedAdditions[0].AdditionRef != liveRef {
		t.Fatalf("next tool schema carrier did not receive the exact executor lease: %+v", mirrored)
	}
}

func TestRelationRepairScopeMalformedDeltaStillFallsBackWithoutMintingCapabilities(t *testing.T) {
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	e := &answerDocumentEvaluator{diagramRequired: true}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch", Success: false,
		Repair: &types.ToolRepair{
			Code: types.ToolRepairCodeAnswerDocRelationRepairScope,
			Metadata: map[string]string{
				types.ToolRepairMetaDiagramRelationRepairDeltaJSON: `{"version":1,"failures":[],"preserve_unlisted_edges":true,"allowed_additions":[{"addition_ref":"ra1-malformed","block_id":"pipeline-diagram","relation_kind":"argument_flow","from_identity":"o.busCtx","to_identity":"","source":"internal/orchestrator/extract_work.go:15"}]}`,
			},
		},
	}

	signal := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !signal.HintRequested || signal.HintKey != "answer_doc.patch_correct" {
		t.Fatalf("malformed producer delta must retain the existing fail-closed fallback: %+v", signal)
	}
	if strings.Contains(signal.Hint, `"addition_ref":"ra1-malformed"`) ||
		strings.Contains(signal.Hint, "complete current additions-only typed capability roster") {
		t.Fatalf("malformed delta must not mint or publish a relation capability: %s", signal.Hint)
	}
}

func TestRelationRepairScopeDeltaWithoutLiveRefStillFailsClosed(t *testing.T) {
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	e := &answerDocumentEvaluator{diagramRequired: true}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch", Success: false,
		Repair: &types.ToolRepair{
			Code: types.ToolRepairCodeAnswerDocRelationRepairScope,
			Metadata: map[string]string{
				types.ToolRepairMetaDiagramRelationRepairDeltaJSON: `{"version":1,"failures":[],"preserve_unlisted_edges":true,"allowed_additions":[{"block_id":"pipeline-diagram","relation_kind":"argument_flow","from_identity":"o.busCtx","to_identity":"ctxbuilder.BuildAgentContext","source":"internal/orchestrator/extract_work.go:15"}]}`,
			},
		},
	}

	signal := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !signal.HintRequested || signal.HintKey != "answer_doc.patch_correct" ||
		strings.Contains(signal.Hint, "complete current additions-only typed capability roster") {
		t.Fatalf("scope delta without an executor-owned live ref must fail closed: %+v", signal)
	}
}

func TestAbsentRelationRepairLeaseGetsTypedRevalidationLane(t *testing.T) {
	ctx := ctxWithAnswerPatchBase()
	e := &answerDocumentEvaluator{}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch", Success: false,
		Repair: &types.ToolRepair{
			Code:   types.ToolRepairCodeAnswerDocRelationRepairLeaseAbsent,
			Hint:   "No live relation lease exists; historical refs are invalid.",
			Fields: []string{"diagram_edge_edits[0].addition_ref"},
			Metadata: map[string]string{
				types.ToolRepairMetaDiagramRelationRepairLeaseStatus: "absent",
			},
		},
	}
	signal := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !signal.HintRequested || signal.HintKey != "answer_doc.patch_relation_lease_absent" ||
		!signal.BypassThrottle || !signal.BypassBudget ||
		!strings.Contains(signal.Hint, "unchanged_block_ids") ||
		!strings.Contains(signal.Hint, "contains no `diagram_edge_edits`") ||
		!strings.Contains(signal.Hint, "Current patch-base block roster") {
		t.Fatalf("absent lease must take the precise no-ref revalidation lane: %+v", signal)
	}
	for _, forbidden := range []string{"allowed_additions", "failure_ref, action", "switch to a full `emit_answer_document`"} {
		if strings.Contains(signal.Hint, forbidden) {
			t.Fatalf("absent lease guidance must not imply a live ref roster or whole-answer rewrite (%q): %s", forbidden, signal.Hint)
		}
	}
}

func TestMixedFullRejectStillArmsRelationRepairLease(t *testing.T) {
	const delta = `{"version":1,"failures":[{"block_id":"flow","issue":"call_edge_unproven","relation_kind":"call","from_node":"A","to_node":"B"}],"preserve_unlisted_edges":true}`
	mut := types.NewMutableState("mixed relation and table repair")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
		{
			ID: "flow", Kind: types.BlockDiagram,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n A->>B: call"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}},
		},
	}})
	ctx := &types.AgentContext{Mutable: mut}
	e := &answerDocumentEvaluator{diagramRequired: true, mu: mut}
	result := &types.ToolResult{
		ToolName: "emit_answer_document", Success: false,
		Repair: &types.ToolRepair{
			Code:   "answer_doc_pre_emit_contract",
			Fields: []string{"blocks[0].items[0].evidence_ids", "blocks[1].edge_anchors"},
			Metadata: map[string]string{
				"violation_kinds": "item_evidence_identity_required," + string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaDiagramRelationRepairDeltaJSON: delta,
			},
		},
	}
	_ = e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: result})
	lease := mut.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.Blocks) != 1 || lease.Blocks[0].BlockID != "flow" || lease.Blocks[0].Kind != types.BlockDiagram {
		t.Fatalf("mixed independent failure must still arm the exact diagram carrier lease: %+v", lease)
	}
}

func TestEmitAnswerDocumentRejectSignal_RequiredFlowUsesTypedRelationBoundaryWhenWholeDiagramWithheld(t *testing.T) {
	e := &answerDocumentEvaluator{diagramRequired: true}
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
	obs := LoopObservation{LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document",
		Success:  false,
		Repair: &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
			},
		},
	}}

	got := e.emitAnswerDocumentRejectSignal(ctx, obs)
	if !got.HintRequested || !strings.Contains(got.HintKey, "required-diagram-relation-boundary") {
		t.Fatalf("first required flow reject should use the typed relation boundary lane, got %+v", got)
	}
	if !strings.Contains(got.Hint, "edge_recipe[1]=`n1 -> n2`") {
		t.Fatalf("first-reject boundary lane must repeat the typed recipe:\n%s", got.Hint)
	}
}

func TestRequiredFlowMixedRelationAndParticipantRejectRepeatsTypedBoundary(t *testing.T) {
	for _, toolName := range []string{"emit_answer_document", "emit_answer_document_patch"} {
		t.Run(toolName, func(t *testing.T) {
			e := &answerDocumentEvaluator{diagramRequired: true}
			ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
			ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramSequence, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "AnalysisIR", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "AnalysisIR"},
					{Identity: "AnswerDocument", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "AnswerDocument"},
				},
			}
			ctx.AnalysisIR.AnswerContract.Diagram.Required = true
			ctx.AnalysisIR.AnswerContract.Diagram.Participants = append([]types.DiagramParticipantHint(nil),
				ctx.AnalysisIR.RequestModel.DiagramHint.Participants...)
			obs := LoopObservation{LastToolResult: &types.ToolResult{
				ToolName: toolName,
				Success:  false,
				Repair: &types.ToolRepair{
					Code: "answer_doc_pre_emit_contract",
					Metadata: map[string]string{
						"violation_kinds": strings.Join([]string{
							string(types.ViolDiagramCallEdgeUnproven),
							string(types.ViolDiagramParticipantCoverage),
						}, ","),
						types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
					},
				},
			}}

			var got LoopSignal
			if toolName == "emit_answer_document" {
				got = e.emitAnswerDocumentRejectSignal(ctx, obs)
			} else {
				got = e.emitPatchRejectFullRewriteSignal(ctx, obs)
			}
			if !got.HintRequested || !strings.Contains(got.HintKey, "required_diagram_relation_boundary") &&
				!strings.Contains(got.HintKey, "required-diagram-relation-boundary") {
				t.Fatalf("mixed typed diagram reject should repeat the positive boundary, got %+v", got)
			}
			for _, want := range []string{
				"edge_recipe[1]=`n1 -> n2`",
				"Requested participants without a proven directed incident relation must retain an unproven boundary",
				"boundary_recipe[1]",
				`boundary_row={"participant":"AnalysisIR","status":"unproven"}`,
				"edge_action=`none`",
				"copy that recipe's `edge_anchor_json` unchanged",
				"Visible node labels, edge/message labels, and Notes remain model-authored",
				"`participant_boundaries` is block-level and valid only on a block whose `kind` is `diagram`",
				"add one separate `kind=diagram` block carrying the Mermaid `diagram` object, `edge_anchors`, and `participant_boundaries`",
				"If the rejected block is already `kind=diagram`, replace it in place",
				"does not rewrite the model's prose, ordering, or conclusion",
			} {
				if !strings.Contains(got.Hint, want) {
					t.Errorf("mixed typed diagram repair missing %q:\n%s", want, got.Hint)
				}
			}
			if strings.Contains(got.Hint, "remove the optional diagram") {
				t.Fatalf("required diagram mixed repair must not offer removal:\n%s", got.Hint)
			}
		})
	}
}

func TestRequiredDiagramTypedRelationRepairRejectsUnrelatedOrNonDiagramViolations(t *testing.T) {
	base := &types.ToolResult{Repair: &types.ToolRepair{
		Code: "answer_doc_pre_emit_contract",
		Metadata: map[string]string{
			"violation_kinds": strings.Join([]string{
				string(types.ViolDiagramCallEdgeUnproven),
				string(types.ViolCitation),
			}, ","),
			types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
		},
	}}
	if answerDocumentRejectIsRequiredDiagramTypedRelationRepair(base) {
		t.Fatal("unrelated citation failure must stay on the ordinary repair lane")
	}
	base.Repair.Metadata["violation_kinds"] = strings.Join([]string{
		string(types.ViolDiagramCallEdgeUnproven),
		string(types.ViolDiagramParticipantCoverage),
	}, ",")
	base.Repair.Metadata[types.ToolRepairMetaOffendingBlockKinds] = strings.Join([]string{
		string(types.BlockDiagram), string(types.BlockTable),
	}, ",")
	if answerDocumentRejectIsRequiredDiagramTypedRelationRepair(base) {
		t.Fatal("mixed diagram/non-diagram location must stay on the ordinary repair lane")
	}
}

func TestOptionalDiagramCallEdgeRecoveryRequiresSingleTypedViolationKind(t *testing.T) {
	result := &types.ToolResult{Repair: &types.ToolRepair{
		Code: "answer_doc_pre_emit_contract",
		Metadata: map[string]string{
			"violation_kinds": strings.Join([]string{
				string(types.ViolDiagramCallEdgeUnproven),
				string(types.ViolCitation),
			}, ","),
			types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
		},
	}}
	if answerDocumentPatchRejectIsOptionalDiagramCallEdge(result, false) {
		t.Fatal("mixed violations must not select optional-diagram removal lane")
	}
	if answerDocumentPatchRejectIsOptionalDiagramCallEdge(result, true) {
		t.Fatal("required diagram must never select optional-diagram removal lane")
	}
}

func TestOptionalDiagramCallEdgeRecoveryRequiresTypedDiagramOnlyLocation(t *testing.T) {
	base := &types.ToolResult{Repair: &types.ToolRepair{
		Code: "answer_doc_pre_emit_contract",
		Metadata: map[string]string{
			"violation_kinds": string(types.ViolDiagramCallEdgeUnproven),
		},
	}}
	if answerDocumentPatchRejectIsOptionalDiagramCallEdge(base, false) {
		t.Fatal("a historical violation-family name without producer-owned location must not imply diagram-only")
	}
	base.Repair.Metadata[types.ToolRepairMetaOffendingBlockKinds] = string(types.BlockOrderedList)
	if answerDocumentPatchRejectIsOptionalDiagramCallEdge(base, false) {
		t.Fatal("ordered-list edge-anchor violation must not select optional-diagram removal")
	}
	base.Repair.Metadata[types.ToolRepairMetaOffendingBlockKinds] = strings.Join([]string{
		string(types.BlockDiagram), string(types.BlockOrderedList),
	}, ",")
	if answerDocumentPatchRejectIsOptionalDiagramCallEdge(base, false) {
		t.Fatal("mixed diagram/non-diagram locations must stay on the ordinary correction path")
	}
	base.Repair.Metadata[types.ToolRepairMetaOffendingBlockKinds] = string(types.BlockDiagram)
	if !answerDocumentPatchRejectIsOptionalDiagramCallEdge(base, false) {
		t.Fatal("one exact optional diagram location should select the bounded recovery lane")
	}
}

func TestEmitPatchRejectFullRewriteSignal_OrderedListCallEdgeStaysOnOrdinaryRepair(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document_patch",
		Success:  false,
		Repair: &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
				types.ToolRepairMetaOffendingBlockKinds: string(types.BlockOrderedList),
			},
		},
	}}
	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested || got.HintKey != "answer_doc.patch_correct" {
		t.Fatalf("ordered-list call-edge reject must use ordinary patch-local repair, got %+v", got)
	}
	if strings.Contains(got.Hint, "OPTIONAL diagram contains") ||
		strings.Contains(got.Hint, "remove the optional diagram block") {
		t.Fatalf("ordered-list repair must not be described as diagram-only:\n%s", got.Hint)
	}
}

func TestPatchCorrectionHintCarriesCanonicalRemoveAndNativeJSONTeaching(t *testing.T) {
	got := answerDocPatchRejectCorrectionHint(&types.ToolResult{Repair: &types.ToolRepair{
		Code:   "answer_doc_pre_emit_contract",
		Fields: []string{"blocks[].edge_anchors[]"},
	}})
	for _, want := range []string{
		types.AnswerDocumentPatchOperationTeaching,
		"`remove_block_ids`",
		"omitting a previous block id from all four operations does not delete it",
		"replaces the whole existing block rather than merging fields",
		"an omitted field is deleted",
		"never wrap an array or object payload in a JSON string",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("patch correction hint missing canonical operation teaching %q:\n%s", want, got)
		}
	}
}

func TestEmitPatchRejectFullRewriteSignal_SectionCountKeepsPatchPath(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document_patch",
			Success:  false,
			Summary:  "Field: blocks[].kind=section\nAction: reduce kind=section blocks to at most 2 (currently emitted: 3)",
			Repair: &types.ToolRepair{
				Code:   "answer_doc_pre_emit_contract",
				Fields: []string{"blocks[].kind=section"},
				Metadata: map[string]string{
					"violation_kinds": string(types.ViolBlockCoverageMissing),
					types.ToolRepairMetaBlockCardinalityRelation: "over_max",
					types.ToolRepairMetaOffendingBlockKinds:      string(types.BlockSection),
				},
			},
		},
	}

	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested {
		t.Fatalf("section-cardinality patch reject should request a targeted patch hint; got %+v", got)
	}
	if got.HintKey != "answer_doc.patch_cardinality" {
		t.Fatalf("HintKey=%q, want answer_doc.patch_cardinality", got.HintKey)
	}
	for _, want := range []string{
		"Keep using `emit_answer_document_patch`",
		"`kind=section`",
		"replace_blocks",
		"remove_block_ids",
		"append_citations",
	} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("section-cardinality hint missing %q:\n%s", want, got.Hint)
		}
	}
	if strings.Contains(got.Hint, "Stop patching") {
		t.Fatalf("section-cardinality reject must not force full rewrite:\n%s", got.Hint)
	}
	if e.forceFullEmitNext {
		t.Fatal("section-cardinality reject should keep the patch path available")
	}
	if !e.preferPatchNext {
		t.Fatal("section-cardinality reject should keep patch preferred")
	}
}

func TestEmitPatchRejectFullRewriteSignal_TypedSectionCountKeepsPatchPath(t *testing.T) {
	e := &answerDocumentEvaluator{}
	ctx := ctxWithAnswerPatchBase()
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document_patch",
			Success:  false,
			Repair: &types.ToolRepair{
				Code:   "answer_doc_pre_emit_contract",
				Fields: []string{"blocks[].kind=section"},
				Hint:   "Apply these typed answer-document schema corrections.",
				Metadata: map[string]string{
					"violation_kinds": string(types.ViolBlockCoverageMissing),
					types.ToolRepairMetaBlockCardinalityRelation: "over_max",
					types.ToolRepairMetaOffendingBlockKinds:      string(types.BlockSection),
				},
			},
		},
	}

	got := e.emitPatchRejectFullRewriteSignal(ctx, obs)
	if !got.HintRequested {
		t.Fatalf("typed section-cardinality patch reject should request a targeted patch hint; got %+v", got)
	}
	if got.HintKey != "answer_doc.patch_cardinality" {
		t.Fatalf("HintKey=%q, want answer_doc.patch_cardinality", got.HintKey)
	}
	if !strings.Contains(got.Hint, "`kind=section`") {
		t.Fatalf("typed section-cardinality hint did not keep patch cardinality path:\n%s", got.Hint)
	}
	if e.forceFullEmitNext || !e.preferPatchNext {
		t.Fatalf("typed section-cardinality should keep patch path: forceFull=%t preferPatch=%t",
			e.forceFullEmitNext, e.preferPatchNext)
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

	t.Run("drops_full_emit_when_first_reject_prefers_patch", func(t *testing.T) {
		e := &answerDocumentEvaluator{preferPatchNext: true}
		ctx := ctxWithAnswerPatchBase()
		got := callFilterToolSchemasT2(e, ctx, baseSchemas)
		for _, s := range got {
			if s.Name == "emit_answer_document" {
				t.Fatalf("preferPatchNext should filter full emit; got %+v", got)
			}
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

func TestAnswerDocumentEvaluator_DiagramRelationRelabelRepeatGuidance(t *testing.T) {
	const pair = "0123456789abcdef"
	result := func(toolName string, success bool) *types.ToolResult {
		return &types.ToolResult{
			ToolName: toolName,
			Success:  success,
			Repair: &types.ToolRepair{
				Code: "answer_doc_pre_emit_contract",
				Metadata: map[string]string{
					types.ToolRepairMetaDiagramRelationFailurePairs: pair,
				},
			},
		}
	}

	e := &answerDocumentEvaluator{}
	e.observeDiagramRelationFailure(result("emit_answer_document", false))
	if e.diagramRelationFailureRepeated {
		t.Fatal("first typed pair failure must not be called repeat")
	}
	e.observeDiagramRelationFailure(result("emit_answer_document_patch", false))
	if !e.diagramRelationFailureRepeated {
		t.Fatal("same typed pair across full/patch attempts must be recognised")
	}
	hint := e.appendDiagramRelationRepeatGuidance("repair")
	for _, want := range []string{"same typed diagram endpoint pair", "cannot create evidence", "remove that pair", "Preserve every unrelated grounded edge"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("repeat guidance missing %q: %s", want, hint)
		}
	}

	e.observeDiagramRelationFailure(result("emit_answer_document", true))
	if e.diagramRelationFailureRepeated || len(e.diagramRelationFailurePairStrikes) != 0 {
		t.Fatalf("successful structured emit must reset repeat state: repeated=%v strikes=%v", e.diagramRelationFailureRepeated, e.diagramRelationFailurePairStrikes)
	}
}

func TestAnswerDocumentEvaluator_RepeatedTypedUnprovenFlowUsesCompactNodeOnlyExit(t *testing.T) {
	mut := types.NewMutableState("required source-flow diagram")
	mut.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{
		Lane: types.DowngradeLaneFlowOperationCarrier,
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				PredicateAxis: types.AxisFlow,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				DiagramHint:   &types.DiagramHint{Kind: types.DiagramFlow, Required: true},
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramFlow},
			},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID: "definition", Kind: types.EvidenceDirect,
			Source: "src/tools.go", LineStart: 10, LineEnd: 10,
			Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition,
			AnchorSymbol: "FullTool", Subject: "FullTool",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	e := &answerDocumentEvaluator{
		diagramRequired:                true,
		diagramRelationFailureRepeated: true,
	}

	hint, ok := e.repeatedTypedUnprovenFlowRepairHint(ctx, true)
	if !ok {
		view := types.BuildAnswerSemanticViewForAgentContext(ctx)
		_, edges, _, _ := answerDocCurrentSourceMechanismRelations(ctx)
		t.Fatalf("repeated precise zero-relation flow must expose the compact typed exit: view=%+v edges=%+v caveat=%v",
			view, edges, ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneFlowOperationCarrier))
	}
	for _, want := range []string{
		"Keep using `emit_answer_document_patch`",
		"nonempty node/group inventory",
		"remove every structural arrow and its `edge_anchors`",
		"state the unproven relation boundary in your own answer prose",
		"not a system-authored diagram or conclusion",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("compact typed-unproven repair missing %q:\n%s", want, hint)
		}
	}
	for _, forbidden := range []string{
		"Typed relation authoring capsule",
		"Current-Source Mechanism Relation Authority",
		"Preserve the following evidence skeleton's exact node IDs",
	} {
		if strings.Contains(hint, forbidden) {
			t.Fatalf("repeat escalation re-injected a broad contract %q:\n%s", forbidden, hint)
		}
	}

	ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{
		ID: "edge", Kind: types.EvidenceRelationship,
		Subject: "FullTool", Predicate: "calls", Object: "PatchTool",
		Source: "src/tools.go", LineStart: 20, LineEnd: 20,
		Scope: types.ScopeLine, AnchorKind: types.AnchorCall,
		AnchorSymbol: "PatchTool", GroundingStatus: types.GroundingGrounded,
	})
	if hint, ok := e.repeatedTypedUnprovenFlowRepairHint(ctx, true); ok || hint != "" {
		t.Fatalf("a real typed relation must retain exact-recipe repair, got ok=%v hint=%q", ok, hint)
	}
}
