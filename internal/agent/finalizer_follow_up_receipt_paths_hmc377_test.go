package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 D1 review findings: the follow-up receipt is written on every
// not-sent path, it records what the request actually published, and the
// follow-up teaching never contradicts or extends what the projected request
// offers.

func hmc377RunFollowUp(t *testing.T, maxIter, window int, settings types.AgentSettings) (*types.AgentContext, *hmc376Adapter, *answerDocumentEvaluator) {
	t.Helper()
	ctx := hmc377Context(false)(t)
	if settings.FinalizerRequestBytePolicy == "" {
		settings.FinalizerRequestBytePolicy = types.FinalizerRequestByteEnforce
	}
	sk := traceTeachingSkill(t, "answer-document-skill")
	adapter := &hmc376Adapter{window: window}
	adapter.script = func(n int) (llm.Response, error) {
		switch n {
		case 1:
			return hmc377Emit(""), nil
		case 2:
			return hmc377Select(), nil
		}
		return llm.Response{}, hmc377Stop
	}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: maxIter, AgentSettings: settings}).(*BaseAgent)
	_, _ = base.Execute(ctx, sk)
	return ctx, adapter, base.eval.(*answerDocumentEvaluator)
}

// D1-R1: the offer is latched on the last allowed iteration — the loop ends
// before any request decision. The receipt says the follow-up was not sent
// and why, so the side file never reads "the model never selected".
func TestHMC377FollowUpOfferedOnLastIterationRecordsNotSent(t *testing.T) {
	ctx, adapter, e := hmc377RunFollowUp(t, 1, 1_000_000, types.AgentSettings{})
	if !e.postEmitAdvisoryDelivered || len(adapter.messages) != 1 {
		t.Fatalf("fixture: delivered=%t calls=%d", e.postEmitAdvisoryDelivered, len(adapter.messages))
	}
	receipt := ctx.Mutable.AnswerFollowUpDelivery()
	if receipt == nil || receipt.Sent || receipt.NotSentCause != types.AnswerFollowUpNotSentLoopStopped ||
		!receipt.RootCauseSelectionOffered {
		t.Fatalf("an offered follow-up the loop never sent must leave a not-sent receipt: %+v", receipt)
	}
	if !receipt.RootCauseSelectionNotSent() {
		t.Fatal("the side-file arm must read the not-sent follow-up")
	}
}

// D1-R4: a force-stop under a configured hard pressure ratio is the same
// not-sent path.
func TestHMC377FollowUpAfterForceStopRecordsNotSent(t *testing.T) {
	_, wide, _ := hmc377RunFollowUp(t, 4, 1_000_000, types.AgentSettings{})
	first := wide.size(0)
	window := hmc376Window(first + 2048)
	hard := float64(first) / float64(window*4) * 0.99
	ctx, adapter, e := hmc377RunFollowUp(t, 4, window, types.AgentSettings{ContextPressureSoftRatio: hard * 0.9, ContextPressureHardRatio: hard})
	if !e.postEmitAdvisoryDelivered || len(adapter.messages) != 1 {
		t.Fatalf("fixture: delivered=%t calls=%d", e.postEmitAdvisoryDelivered, len(adapter.messages))
	}
	receipt := ctx.Mutable.AnswerFollowUpDelivery()
	if receipt == nil || receipt.Sent || receipt.NotSentCause != types.AnswerFollowUpNotSentLoopStopped {
		t.Fatalf("a force-stopped follow-up must leave a not-sent receipt: %+v", receipt)
	}
}

// D1-R5: when the projection refuses (the dispatch schema does not publish a
// declared operation) the whole surface is published, and the receipt says
// exactly that — never the declared union.
func TestHMC377FollowUpReceiptRecordsPublishedOperations(t *testing.T) {
	ctx := hmc377Context(false)(t)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "x"}}}
	e := &answerDocumentEvaluator{language: "en", mu: ctx.Mutable, requestBytePolicy: types.FinalizerRequestByteEnforce}
	_ = e.BuildInitialInstruction(ctx, nil)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	if sig := e.postEmitAdvisorySignal(ctx, ctx.Mutable.AnswerDocumentV2()); !sig.HintRequested {
		t.Fatal("fixture: no advisory")
	}
	whole := llm.ToolSchema{Name: answerDocumentPatchToolName, Description: "whole", Parameters: json.RawMessage(`{"type":"object","properties":{"unchanged_block_ids":{"type":"array"},"replace_blocks":{"type":"array"}}}`)}
	tools := e.FilterToolSchemas(ctx, []llm.ToolSchema{{Name: answerDocumentToolName, Parameters: json.RawMessage(`{}`)}, whole})
	if len(tools) != 1 || tools[0].Description != "whole" {
		t.Fatalf("fixture: the refused projection must keep the whole surface: %+v", tools)
	}
	e.recordFollowUpDelivery(ctx, true, "", 1)
	if r := ctx.Mutable.AnswerFollowUpDelivery(); r == nil || len(r.Operations) != 0 {
		t.Fatalf("the receipt must record the whole surface it published, not the declared union: %+v", r)
	}
	// A projection that applies records exactly what it applied.
	ctx2 := hmc377Context(false)(t)
	e2 := &answerDocumentEvaluator{language: "en", mu: ctx2.Mutable, requestBytePolicy: types.FinalizerRequestByteEnforce}
	_ = e2.BuildInitialInstruction(ctx2, nil)
	ctx2.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	if sig := e2.postEmitAdvisorySignal(ctx2, ctx2.Mutable.AnswerDocumentV2()); !sig.HintRequested {
		t.Fatal("fixture: no advisory")
	}
	full := llm.ToolSchema{Name: answerDocumentPatchToolName, Description: "whole", Parameters: (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx2)}
	_ = e2.FilterToolSchemas(ctx2, []llm.ToolSchema{{Name: answerDocumentToolName, Parameters: json.RawMessage(`{}`)}, full})
	e2.recordFollowUpDelivery(ctx2, true, "", 1)
	if r := ctx2.Mutable.AnswerFollowUpDelivery(); r == nil || strings.Join(r.Operations, ",") != "replace_trace_root_causes,unchanged_block_ids" {
		t.Fatalf("an applied projection records its operations: %+v", r)
	}
}

// D1-R2: on the narrow order + selection surface the hint names no patch
// operation the projected schema leaves out.
func TestHMC377OrderPlusSelectionHintNamesOnlyPublishedOperations(t *testing.T) {
	ctx := hmc377Context(false)(t)
	ctx.Language = "en"
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain,
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
			{Index: 1, Label: "Diagram", Role: types.RequestedAnswerDimensionDiagram, Required: true},
			{Index: 2, Label: "Members", Role: types.RequestedAnswerDimensionMemberSet, Required: true},
		}}}}
	diagram := types.AnswerBlock{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n A->>B: call"}}
	roster := types.AnswerBlock{ID: "roster", Kind: types.BlockBulletList, SurfaceRole: types.SurfacePrincipal, FacetIDs: []string{string(types.FacetMemberSet)}, Items: []types.AnswerBlockItem{{ID: "m1", Text: "member"}}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "summary"}, roster, diagram}}
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	e := &answerDocumentEvaluator{language: "en", mu: ctx.Mutable}
	_ = e.BuildInitialInstruction(ctx, nil)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	sig := e.postEmitAdvisorySignal(ctx, ctx.Mutable.AnswerDocumentV2())
	if !sig.HintRequested || len(e.postEmitAdvisoryKinds) < 2 {
		t.Fatalf("fixture: order + selection rows expected, got %v", e.postEmitAdvisoryKinds)
	}
	patch := llm.ToolSchema{Name: answerDocumentPatchToolName, Description: "whole", Parameters: (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)}
	tools := e.FilterToolSchemas(ctx, []llm.ToolSchema{{Name: answerDocumentToolName, Parameters: json.RawMessage(`{}`)}, patch})
	published := map[string]bool{}
	for _, p := range hmc377Properties(t, tools[0]) {
		published[p] = true
	}
	for _, op := range []string{"replace_blocks", "block_field_edits_v1", "add_blocks", "remove_block_ids", "model_block_order", "replace_trace_root_causes", "unchanged_block_ids"} {
		if strings.Contains(sig.Hint, "`"+op+"`") && !published[op] {
			t.Fatalf("the hint names `%s`, which the projected follow-up surface does not publish:\n%s", op, sig.Hint)
		}
	}
}

// D1-R3: the follow-up representation does not declare every item optional;
// each item in the latest message states its own optionality.
func TestHMC377FollowUpTeachingLeavesOptionalityToTheItems(t *testing.T) {
	follow := traceTeachingSkill(t, "answer-document-skill").AnswerDocumentFollowUpTeaching(nil)
	text := follow.Goal + "\n" + strings.Join(follow.Workflow, "\n") + "\n" + follow.OutputFormat
	for _, banned := range []string{"Every item is optional", "Optionally add"} {
		if strings.Contains(text, banned) {
			t.Fatalf("the follow-up representation decides optionality for every item (%q):\n%s", banned, text)
		}
	}
	if !strings.Contains(text, "each item says whether it is optional") {
		t.Fatalf("the representation must defer optionality to the items:\n%s", text)
	}
}
