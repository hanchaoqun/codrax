package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestStrictAnswerReviewEnabled_DefaultTrue(t *testing.T) {
	if !((&Orchestrator{}).strictAnswerReviewEnabledValue()) {
		t.Fatal("zero-value orchestrator must keep strict answer review enabled")
	}
	o := &Orchestrator{}
	o.SetStrictAnswerReviewEnabled(false)
	if o.strictAnswerReviewEnabledValue() {
		t.Fatal("explicit disable should turn strict answer review off")
	}
}

func TestAttachFirstDraftReference_AcceptedStructuredAnswerDropsRejectedDraftTelemetry(t *testing.T) {
	mut := types.NewMutableState("x")
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "最终稿",
		}},
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Language: "zh",
			Mutable:  mut,
		},
	}
	out := &agent.StageOutput{FinalAnswer: "最终稿"}
	o.attachFirstDraftReference(out, "第一稿", []types.Violation{{Kind: types.ViolMustInclude}}, true, nil)
	if strings.TrimSpace(out.FinalAnswer) != "最终稿" {
		t.Fatalf("accepted structured answer must remain the sole model answer carrier:\n%s", out.FinalAnswer)
	}
	if got := mut.AnswerDisplayAttachments(); len(got) != 0 {
		t.Fatalf("rejected first-draft telemetry escaped behind accepted document: %+v", got)
	}
}

func TestAttachFirstDraftReference_UnstructuredFallbackStillPreservesModelDraft(t *testing.T) {
	mut := types.NewMutableState("x")
	o := &Orchestrator{busCtx: &types.BusContext{Language: "zh", Mutable: mut}}
	out := &agent.StageOutput{FinalAnswer: "当前降级答案"}

	o.attachFirstDraftReference(out, "第一稿唯一内容", []types.Violation{{Kind: types.ViolMustInclude}}, true, nil)

	if !strings.Contains(out.FinalAnswer, "第一稿答案（校验前参考）") ||
		!strings.Contains(out.FinalAnswer, "第一稿唯一内容") {
		t.Fatalf("without an accepted structured carrier, model-authored fallback content must remain recoverable:\n%s", out.FinalAnswer)
	}
	if got := mut.AnswerDisplayAttachments(); len(got) != 1 {
		t.Fatalf("unstructured fallback attachment count=%d, want 1", len(got))
	}
}

func TestAttachFirstDraftReference_AcceptedAnswerRendersRequestedDimensionSupplementOnce(t *testing.T) {
	mut := types.NewMutableState("x")
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "已接受结构化答案。",
	}}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{busCtx: &types.BusContext{
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Index: 1, Label: "关系表格", SourceQuote: "表格说明它们在 finalizer 里的关系",
					Required: true, Role: types.RequestedAnswerDimensionOther,
				}},
				Confidence: 0.9,
			},
		}},
	}}
	out := &agent.StageOutput{FinalAnswer: "已接受结构化答案。"}
	firstDraft := "被拒第一稿\n\n---\n\n> **系统补充：输出维度核对**\n>\n> 旧稿补充"

	o.attachFirstDraftReference(out, firstDraft, []types.Violation{{Kind: types.ViolMustInclude}}, true, nil)

	if got := strings.Count(out.FinalAnswer, "系统补充：输出维度核对"); got != 1 {
		t.Fatalf("accepted answer must render the current typed supplement exactly once, got %d:\n%s", got, out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "被拒第一稿") || strings.Contains(out.FinalAnswer, "旧稿补充") {
		t.Fatalf("rejected draft and its stale supplement escaped into the accepted answer:\n%s", out.FinalAnswer)
	}
}

func TestAppendAnswerDisplayAttachment_AcceptedAnswerDedupesSystemAttachmentByHash(t *testing.T) {
	mut := types.NewMutableState("x")
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "模型主体",
	}}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{busCtx: &types.BusContext{Language: "zh", Mutable: mut}}
	out := &agent.StageOutput{FinalAnswer: "模型主体"}
	att := types.AnswerDisplayAttachment{
		Kind:   types.AnswerDisplayAttachmentMarkdown,
		Title:  "系统核对",
		Body:   "同一条 typed 核对结果",
		Source: types.AnswerDisplayAttachmentSourceSystemCrossCheck,
	}

	o.appendAnswerDisplayAttachment(out, att)
	o.appendAnswerDisplayAttachment(out, att)

	if got := mut.AnswerDisplayAttachments(); len(got) != 1 || !got[0].SystemAuthored() || got[0].Hash == "" {
		t.Fatalf("system attachment must survive once with a typed hash: %+v", got)
	}
	if got := strings.Count(out.FinalAnswer, att.Body); got != 1 {
		t.Fatalf("deduplicated system attachment body count=%d, want 1:\n%s", got, out.FinalAnswer)
	}
}

func TestAttachFirstDraftReference_NoopsWithoutRewriteRejection(t *testing.T) {
	mut := types.NewMutableState("x")
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "一次通过的最终稿",
		}},
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Language: "zh",
			Mutable:  mut,
		},
	}
	out := &agent.StageOutput{FinalAnswer: "一次通过的最终稿"}
	o.attachFirstDraftReference(out, "第一稿内容", []types.Violation{{Kind: types.ViolCitation}}, false, nil)
	if strings.Contains(out.FinalAnswer, "第一稿答案") || strings.Contains(out.FinalAnswer, "第一稿内容") {
		t.Fatalf("accepted first draft must not be attached without a real rewrite rejection:\n%s", out.FinalAnswer)
	}
	if got := len(mut.AnswerDisplayAttachments()); got != 0 {
		t.Fatalf("unexpected first-draft attachment stored: %d", got)
	}
}

func TestAttachFirstDraftReference_SameStructuredCarrierDoesNotDuplicateAfterSystemCaveat(t *testing.T) {
	mut := types.NewMutableState("x")
	firstDoc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "首稿主体",
	}}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, firstDoc)
	o := &Orchestrator{busCtx: &types.BusContext{Language: "zh", Mutable: mut}}
	snapshot := finalizeRepairVisibleCarrierSnapshot(mut)
	out := &agent.StageOutput{FinalAnswer: "首稿主体\n\n**补充说明：** 仍有一项 typed 校验边界。"}

	o.attachFirstDraftReference(out, "首稿主体", []types.Violation{{Kind: types.ViolMustInclude}}, true, snapshot)
	if got := mut.AnswerDisplayAttachments(); len(got) != 0 {
		t.Fatalf("same restored carrier must not be attached below itself: %+v", got)
	}
	if strings.Contains(out.FinalAnswer, "第一稿答案") || !strings.Contains(out.FinalAnswer, "typed 校验边界") {
		t.Fatalf("dedup must preserve the shipping answer and residual caveat without a duplicate panel:\n%s", out.FinalAnswer)
	}

	// Any accepted structured replacement remains the sole model answer
	// carrier. A changed carrier does not turn the rejected first draft back
	// into user-visible content; its exact body stays retry telemetry.
	changed := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "真正不同的修复稿",
	}}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, changed)
	out = &agent.StageOutput{FinalAnswer: "真正不同的修复稿"}
	o.attachFirstDraftReference(out, "首稿主体", []types.Violation{{Kind: types.ViolMustInclude}}, true, snapshot)
	if got := mut.AnswerDisplayAttachments(); len(got) != 0 {
		t.Fatalf("changed accepted carrier must still suppress rejected first-draft telemetry: %+v", got)
	}
}
