package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RECEIPT-1 (§40.31.1 ○14 → §40.32, 2026-09-02): the receipt's
// unproven-mechanism clause names a dropped frame only when the request is a
// typed frame question; a non-frame question gets the generic mechanism
// words with identical credential facts.
func TestRenderV2_RuntimeWorkRelationReceiptFrameWordingFollowsTypedDecision(t *testing.T) {
	receiptFor := func(frame bool, conclusion types.RuntimeWorkRelationConclusion, credential string) *types.AnswerRuntimeWorkRelationReceipt {
		return &types.AnswerRuntimeWorkRelationReceipt{
			ObservationID: "trace_query:test#trace_semantic_span:1", Conclusion: conclusion,
			BoundRow: types.RuntimeWorkRelationRow{
				ObservationID: "trace_query:test#trace_semantic_span:1", WorkLabel: "VerifyClass Demo", Subject: "worker-7",
				MeasuredDurationMS: 0.285, AllowedConclusions: []types.RuntimeWorkRelationConclusion{conclusion},
				Credential: credential, FrameCausalityApplicable: frame,
			},
		}
	}
	render := func(lang string, receipt *types.AnswerRuntimeWorkRelationReceipt) string {
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "x", RuntimeWorkRelation: receipt}}}
		return RenderAnswerDocument(doc, lang)
	}
	arms := []struct {
		conclusion types.RuntimeWorkRelationConclusion
		credential string
	}{
		{types.RuntimeWorkRelationConclusionRelatedCausalityUnproven, "host_direct_wakeup_edge"},
		{types.RuntimeWorkRelationConclusionRelatedCausalityUnproven, "typed_chain_interval_overlap"},
		{types.RuntimeWorkRelationConclusionRelationUnproven, "host_direct_wakeup_edge"},
		{types.RuntimeWorkRelationConclusionRelationUnproven, "typed_chain_interval_overlap"},
		{types.RuntimeWorkRelationConclusionRelationUnproven, "target_self_execution"},
		{types.RuntimeWorkRelationConclusionTargetSelfWorkObserved, "target_self_execution"},
	}
	for _, arm := range arms {
		zhFrame := render("zh", receiptFor(true, arm.conclusion, arm.credential))
		zhPlain := render("zh", receiptFor(false, arm.conclusion, arm.credential))
		enFrame := render("en", receiptFor(true, arm.conclusion, arm.credential))
		enPlain := render("en", receiptFor(false, arm.conclusion, arm.credential))
		if !(strings.Contains(zhFrame, "丢帧") || strings.Contains(zhFrame, "该帧超时")) {
			t.Fatalf("%s/%s zh frame form must name the frame: %s", arm.conclusion, arm.credential, zhFrame)
		}
		if strings.Contains(zhPlain, "丢帧") || strings.Contains(zhPlain, "帧超时") || !strings.Contains(zhPlain, "因果贡献") {
			t.Fatalf("%s/%s zh non-frame form must speak the generic mechanism: %s", arm.conclusion, arm.credential, zhPlain)
		}
		if !(strings.Contains(enFrame, "dropped-frame") || strings.Contains(enFrame, "frame or deadline miss")) {
			t.Fatalf("%s/%s en frame form must name the frame: %s", arm.conclusion, arm.credential, enFrame)
		}
		if strings.Contains(enPlain, "frame") || !strings.Contains(enPlain, "causal contribution") && !strings.Contains(enPlain, "causal-contribution") {
			t.Fatalf("%s/%s en non-frame form must speak the generic mechanism: %s", arm.conclusion, arm.credential, enPlain)
		}
		// Credential facts are identical on both forks.
		for _, fact := range []string{"直接唤醒目标", "链上区间存在关系", "目标自身执行的工作"} {
			if strings.Contains(zhFrame, fact) != strings.Contains(zhPlain, fact) {
				t.Fatalf("%s/%s credential fact %q must not depend on the frame decision", arm.conclusion, arm.credential, fact)
			}
		}
	}
}
