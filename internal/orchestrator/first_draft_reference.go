package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// attachFirstDraftReference preserves model-authored fallback content only
// when no accepted structured carrier supersedes it. The shared post-finalize
// attachment boundary drops rejected markdown/text once a later structured
// answer is accepted; soft or residual concerns use system-owned appendices.
func (o *Orchestrator) attachFirstDraftReference(out *agent.StageOutput, firstDraft string, concerns []types.Violation, rejectedForRewrite bool, firstDraftCarrierSnapshot []byte) {
	if o == nil || out == nil || !rejectedForRewrite {
		return
	}
	// FRCAP can restore the exact first structured carrier and then append a
	// residual system caveat to FinalAnswer. The rendered strings differ in
	// that case, but attaching the whole first draft would duplicate the model
	// answer under itself. Compare typed visible carriers instead; a changed
	// block or recovered model attachment still takes the normal preserve path.
	if o.busCtx != nil && finalizeRepairVisibleCarrierMatches(firstDraftCarrierSnapshot, o.busCtx.Mutable) {
		return
	}
	firstDraft = strings.TrimSpace(firstDraft)
	finalAnswer := strings.TrimSpace(out.FinalAnswer)
	if firstDraft == "" || finalAnswer == "" || firstDraft == finalAnswer {
		return
	}
	lang := ""
	if o.busCtx != nil {
		lang = o.busCtx.Language
	}
	body := firstDraft
	if note := draftConcernSummary(lang, concerns, true); note != "" {
		body = note + "\n\n" + body
	}
	o.appendAnswerDisplayAttachment(out, types.AnswerDisplayAttachment{
		Kind:   types.AnswerDisplayAttachmentMarkdown,
		Title:  draftReferenceTitle(lang),
		Body:   body,
		Source: "orchestrator.first_finalize_draft",
	})
}

// First-draft attachment titles (moved here from orchestrator.go under the
// IR delivery hot-file ratchet, §40.43 F-orch 三轮收编).
func draftReferenceTitle(lang string) string {
	if lang == "zh" {
		return "第一稿答案（校验前参考）"
	}
	return "First Draft Answer (Pre-review Reference)"
}

func draftReviewNoteTitle(lang string) string {
	if lang == "zh" {
		return "第一稿校验提示"
	}
	return "First Draft Review Notes"
}

func strictReviewDisabledTitle(lang string) string {
	if lang == "zh" {
		return "第一稿答案：强校验已关闭"
	}
	return "First Draft Answer: Strict Review Disabled"
}
