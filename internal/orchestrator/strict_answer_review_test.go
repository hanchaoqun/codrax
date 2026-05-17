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
	o.attachFirstDraftReference(out, "第一稿", []types.Violation{{Kind: types.ViolMustInclude}})
	if !strings.Contains(out.FinalAnswer, "最终稿") ||
		!strings.Contains(out.FinalAnswer, "第一稿答案（校验前参考）") ||
		!strings.Contains(out.FinalAnswer, "第一稿仍有 1 项待补充") ||
		!strings.Contains(out.FinalAnswer, "第一稿") {
		t.Fatalf("first draft attachment missing from rendered answer:\n%s", out.FinalAnswer)
	}
}
