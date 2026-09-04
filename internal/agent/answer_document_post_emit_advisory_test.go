package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_post_emit_advisory_test.go — V3-3 (§40.51) behavioural
// pins. Red on the 3080934fd tree via `go test -overlay` of the pre-V3-3
// evaluator (the first Observe returned only the requested-dimensions lane,
// the second returned the G13 lane; the merged key did not exist); green
// after the lane table landed.

// postEmitAdvisoryTwoLaneFixture: a zh dispatch whose trace projection elects
// `hmfs_discard-5876` (G13 lane) and whose analyzer requested a member-set
// dimension (requested-dimensions lane); the accepted document satisfies
// neither.
func postEmitAdvisoryTwoLaneFixture(t *testing.T) (*answerDocumentEvaluator, *types.AgentContext, *types.MutableState) {
	t.Helper()
	// Same shape as gapcG13EvaluatorContext, with the requested-dimension
	// profile present BEFORE the first semantic-view compile (the view is
	// cached per AgentContext).
	mu := types.NewMutableState("卡顿主因是什么，涉及哪些线程")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{
			gapcG13RankRecord("hmfs_discard-5876", 1, 11.506),
		},
	}}})
	ctx := &types.AgentContext{
		Language: "zh",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{
					{Label: "涉及线程清单", Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 1},
				},
			},
		}},
		Mutable: mu,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll,
		gapcG13SummaryDoc("VSyncGenerator 延迟链是主要原因。"))
	return e, ctx, mu
}

func TestPostEmitAdvisory_MergesAllLanesIntoOneRound(t *testing.T) {
	e, ctx, _ := postEmitAdvisoryTwoLaneFixture(t)
	first := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !first.HintRequested || first.StopRequested {
		t.Fatalf("two open lanes must produce ONE hint without stopping, got %+v", first)
	}
	if first.HintKey != postEmitAdvisoryHintKey {
		t.Fatalf("merged pass must ride the single advisory key, got %q", first.HintKey)
	}
	for _, want := range []string{
		"涉及线程清单", `facet_ids:["member_set"]`, // requested-dimensions item
		"hmfs_discard-5876", "显式说明与该排序的差异", // G13 item
		"### 修订项 1", "### 修订项 2", // numbered sections
		"只调用一次 `emit_answer_document_patch`", "在同一个 patch 里覆盖全部修订项", // one-patch rule
	} {
		if !strings.Contains(first.Hint, want) {
			t.Fatalf("merged disclosure missing %q:\n%s", want, first.Hint)
		}
	}
	if strings.Count(first.Hint, "已经落地") != 1 {
		t.Fatalf("merged disclosure must carry exactly one preamble, got %d:\n%s", strings.Count(first.Hint, "已经落地"), first.Hint)
	}
	second := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if second.HintRequested || !second.StopRequested {
		t.Fatalf("the merged pass is one-shot: the next observation must accept, got %+v", second)
	}
}

func TestPostEmitAdvisory_NeverChargesHardRejectBudgets(t *testing.T) {
	e, ctx, _ := postEmitAdvisoryTwoLaneFixture(t)
	budget := e.rejectHintBudget()
	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.HintRequested || !sig.BypassBudget || !sig.BypassThrottle {
		t.Fatalf("advisory must fire with BypassBudget/BypassThrottle, got %+v", sig)
	}
	if e.retriesUsed != 0 || e.rejectHintsUsed != 0 || e.emitFullDocFailStreak != 0 ||
		e.emptyBlocksRejectStreak != 0 || e.preferPatchNext || e.forceFullEmitNext || e.emitPatchNudgeFired {
		t.Fatalf("advisory pass must not touch hard-reject accounting: retries=%d rejectHints=%d fullDocStreak=%d emptyStreak=%d preferPatch=%t forceFull=%t nudge=%t",
			e.retriesUsed, e.rejectHintsUsed, e.emitFullDocFailStreak, e.emptyBlocksRejectStreak, e.preferPatchNext, e.forceFullEmitNext, e.emitPatchNudgeFired)
	}
	if e.rejectHintBudget() != budget {
		t.Fatalf("hard-reject budget must be unaffected: %d → %d", budget, e.rejectHintBudget())
	}
}

func TestPostEmitAdvisory_RejectedPatchShipsAcceptedDoc(t *testing.T) {
	e, ctx, _ := postEmitAdvisoryTwoLaneFixture(t)
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); !sig.HintRequested {
		t.Fatalf("fixture must fire the advisory first, got %+v", sig)
	}
	// The model's advisory patch was rejected; the accepted document is still
	// the live one → ship it, no reject hint, no retry (typed escape lane).
	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document_patch", Success: false, Summary: "patch: unchanged_block_ids[\"x\"] not present in previous emit",
	}})
	if sig.HintRequested || !sig.StopRequested {
		t.Fatalf("a rejected advisory patch must ship the accepted document, got %+v", sig)
	}
	if e.retriesUsed != 0 || e.rejectHintsUsed != 0 {
		t.Fatalf("shipping after a rejected advisory patch must not spend a retry: retries=%d rejectHints=%d", e.retriesUsed, e.rejectHintsUsed)
	}
}

func TestPostEmitAdvisory_OneShotPerDispatchAndRearmed(t *testing.T) {
	e, ctx, _ := postEmitAdvisoryTwoLaneFixture(t)
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); !sig.HintRequested || sig.HintKey != postEmitAdvisoryHintKey {
		t.Fatalf("dispatch 1 must fire the merged advisory, got %+v", sig)
	}
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); !sig.StopRequested {
		t.Fatalf("dispatch 1 second observation must accept, got %+v", sig)
	}
	_ = e.BuildInitialInstruction(ctx, nil)
	if e.postEmitAdvisoryDelivered {
		t.Fatal("BuildInitialInstruction must re-arm the single latch")
	}
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); !sig.HintRequested || sig.StopRequested {
		t.Fatalf("a fresh dispatch must re-arm the merged advisory, got %+v", sig)
	}
}

// Ordering + coverage in one disclosure: the ordering constraint is scoped to
// its own item and the cross-item model_block_order rule is stated once.
func TestPostEmitAdvisory_InstructionConflictScoped(t *testing.T) {
	orderCtx := func(dims ...types.RequestedAnswerDimension) *types.AgentContext {
		return &types.AgentContext{Language: "zh", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions:          dims,
			},
		}}}
	}
	diagramDim := types.RequestedAnswerDimension{Index: 1, Label: "关系图", Role: types.RequestedAnswerDimensionDiagram, Required: true}
	rosterDim := types.RequestedAnswerDimension{Index: 2, Label: "成员清单", Role: types.RequestedAnswerDimensionMemberSet, Required: true}
	boundaryDim := types.RequestedAnswerDimension{Index: 3, Label: "适用边界", Role: types.RequestedAnswerDimensionBoundary, Required: true}
	ctx := orderCtx(diagramDim, rosterDim, boundaryDim)
	diagram := types.AnswerBlock{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
		Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n A->>B: call",
	}}
	roster := types.AnswerBlock{ID: "roster", Kind: types.BlockBulletList, SurfaceRole: types.SurfacePrincipal,
		FacetIDs: []string{string(types.FacetMemberSet)}, Items: []types.AnswerBlockItem{{ID: "m1", Text: "member"}}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "summary"}, roster, diagram,
	}}
	sig := (&answerDocumentEvaluator{language: "zh"}).postEmitAdvisorySignal(ctx, doc)
	if !sig.HintRequested {
		t.Fatalf("order violation plus a missing dimension must fire one merged advisory, got %+v", sig)
	}
	for _, want := range []string{
		"### 修订项 1", "### 修订项 2",
		"适用边界",                                       // coverage item names the missing dimension
		"model_block_order", "`diagram`", "`roster`", // ordering item
		"本项本身不增删块",                                  // ordering constraint scoped to its item
		"它不能与 `add_blocks` / `remove_block_ids` 同用", // cross-item rule stated once
		"优先这样做，让排序能随同一个 patch 提交",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("merged disclosure missing %q:\n%s", want, sig.Hint)
		}
	}
	if strings.Contains(sig.Hint, "不要增删块") {
		t.Fatalf("the ordering constraint must not forbid add/remove globally:\n%s", sig.Hint)
	}
	// Ordering alone: no cross-item sentence (fresh context — the semantic
	// view is cached per AgentContext).
	only := (&answerDocumentEvaluator{language: "zh"}).postEmitAdvisorySignal(orderCtx(diagramDim, rosterDim), doc)
	if !only.HintRequested || !strings.Contains(only.Hint, "### 修订项 1") || strings.Contains(only.Hint, "### 修订项 2") {
		t.Fatalf("ordering alone must render exactly one item, got %+v", only)
	}
	if strings.Contains(only.Hint, "它不能与 `add_blocks`") {
		t.Fatalf("the cross-item rule is stated only when several items share the disclosure:\n%s", only.Hint)
	}
}

func TestPostEmitAdvisoryHint_UserFacingOnly(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		e, ctx, _ := postEmitAdvisoryTwoLaneFixture(t)
		e.language = lang
		sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
		if !sig.HintRequested {
			t.Fatalf("lang=%s fixture must fire, got %+v", lang, sig)
		}
		for _, internal := range []string{
			"answer_doc.", "post_emit", "advisory lane",
			string(postEmitAdvisoryRequestedDimensions), string(postEmitAdvisoryRequestedDimensionOrder),
			string(postEmitAdvisoryExternalObservationSelectors), string(postEmitAdvisoryTracePrimaryCauseEntity),
			string(postEmitAdvisoryPreciseRepair),
			"source_location", "function_or_purpose",
		} {
			if strings.Contains(sig.Hint, internal) {
				t.Fatalf("lang=%s merged disclosure exposed internal token %q:\n%s", lang, internal, sig.Hint)
			}
		}
		if lang == "en" && !strings.Contains(sig.Hint, "### Item 1") {
			t.Fatalf("en disclosure must number its items:\n%s", sig.Hint)
		}
	}
}
