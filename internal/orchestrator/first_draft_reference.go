package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// firstFinalizeDraftRecord is the scheduler's typed snapshot of the FIRST
// finalize output of a task: the exact StageOutput pointer, its answer text
// at capture time and the visible-carrier snapshot. The pointer is the
// identity attachFirstDraftReference reads first (§40.43 F-orch 四轮复核
// finding V): every terminal exit that ships that same output — the
// rejected-draft backstop on blocked-DAG / stall / step-drain exits, the
// transient-failure delivery of the retained draft — appends caveats to it,
// so its text no longer equals the captured draft and the live carrier was
// cleared by ResetForFallback; the two text/carrier guards below cannot see
// that the shipping output IS the first draft, and the customer received
// the draft, the caveats, then the identical draft again under the
// "First Draft Answer" title. Identity by pointer short-circuits before
// any text comparison; a genuinely rewritten later output (a different
// StageOutput) still takes the normal reference path.
type firstFinalizeDraftRecord struct {
	Out     *agent.StageOutput
	Text    string
	Carrier []byte
}

// record captures out as the first finalize draft unless a draft with
// answer text is already recorded (the scheduler's "first non-empty draft"
// rule: an empty-text output is superseded by the next output).
func (r *firstFinalizeDraftRecord) record(out *agent.StageOutput, mut *types.MutableState) {
	if r == nil || out == nil || strings.TrimSpace(r.Text) != "" {
		return
	}
	r.Out = out
	r.Text = strings.TrimSpace(out.FinalAnswer)
	r.Carrier = finalizeRepairVisibleCarrierSnapshot(mut)
}

// attachFirstDraftReference preserves model-authored fallback content only
// when no accepted structured carrier supersedes it. The shared post-finalize
// attachment boundary drops rejected markdown/text once a later structured
// answer is accepted; soft or residual concerns use system-owned appendices.
func (o *Orchestrator) attachFirstDraftReference(out *agent.StageOutput, first firstFinalizeDraftRecord, concerns []types.Violation, rejectedForRewrite bool) {
	if o == nil || out == nil || !rejectedForRewrite {
		return
	}
	// Finding V: the shipping output IS the first draft (same StageOutput)
	// — a reference would repeat the body under itself, whatever caveats
	// were appended to it since. Typed identity, checked before any text
	// or carrier comparison.
	if first.Out != nil && out == first.Out {
		return
	}
	// FRCAP can restore the exact first structured carrier and then append a
	// residual system caveat to FinalAnswer. The rendered strings differ in
	// that case, but attaching the whole first draft would duplicate the model
	// answer under itself. Compare typed visible carriers instead; a changed
	// block or recovered model attachment still takes the normal preserve path.
	if o.busCtx != nil && finalizeRepairVisibleCarrierMatches(first.Carrier, o.busCtx.Mutable) {
		return
	}
	firstDraft := strings.TrimSpace(first.Text)
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
