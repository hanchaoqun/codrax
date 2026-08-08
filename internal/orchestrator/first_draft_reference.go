package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// attachFirstDraftReference is the only final-answer path that may show a full
// first-draft reference. It is reserved for a rejected draft that caused a
// later finalizer rewrite; soft concerns on an accepted draft use caveats.
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
