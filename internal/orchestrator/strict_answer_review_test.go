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
	o.attachFirstDraftReference(out, "第一稿", []types.Violation{{Kind: types.ViolMustInclude}}, true)
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
	o.attachFirstDraftReference(out, "第一稿内容", []types.Violation{{Kind: types.ViolCitation}}, false)
	if strings.Contains(out.FinalAnswer, "第一稿答案") || strings.Contains(out.FinalAnswer, "第一稿内容") {
		t.Fatalf("accepted first draft must not be attached without a real rewrite rejection:\n%s", out.FinalAnswer)
	}
	if got := len(mut.AnswerDisplayAttachments()); got != 0 {
		t.Fatalf("unexpected first-draft attachment stored: %d", got)
	}
}
