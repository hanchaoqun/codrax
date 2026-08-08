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

func TestAttachFirstDraftReference_RerendersWithAttachment(t *testing.T) {
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
	if !strings.Contains(out.FinalAnswer, "最终稿") ||
		!strings.Contains(out.FinalAnswer, "第一稿答案（校验前参考）") ||
		!strings.Contains(out.FinalAnswer, "第一稿仍有 1 项待补充") ||
		!strings.Contains(out.FinalAnswer, "第一稿") {
		t.Fatalf("first draft attachment missing from rendered answer:\n%s", out.FinalAnswer)
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

	// Any model-visible carrier change invalidates the identity guard and keeps
	// the original first draft as a distinct reference.
	changed := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "真正不同的修复稿",
	}}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, changed)
	out = &agent.StageOutput{FinalAnswer: "真正不同的修复稿"}
	o.attachFirstDraftReference(out, "首稿主体", []types.Violation{{Kind: types.ViolMustInclude}}, true, snapshot)
	if got := mut.AnswerDisplayAttachments(); len(got) != 1 || !strings.Contains(got[0].Body, "首稿主体") {
		t.Fatalf("changed structured carrier must preserve distinct first-draft content: %+v", got)
	}
}
