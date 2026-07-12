package types

// AnswerDisplayAttachment carries user-visible content recovered from
// a malformed final-answer emit when the structured AnswerDocumentV2
// path could not preserve that content losslessly.
//
// It is intentionally NOT part of the LLM-facing emit_answer_document
// schema and validators do not treat it as grounded answer evidence.
// The field is a last-mile rendering safeguard: if a model already
// produced useful prose or a diagram but wrapped it in broken JSON, the
// user should still be able to inspect it instead of losing it during
// structural recovery.
type AnswerDisplayAttachment struct {
	Kind     string `json:"kind"`
	Title    string `json:"title,omitempty"`
	Language string `json:"language,omitempty"`
	Body     string `json:"body"`
	Source   string `json:"source,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

const (
	AnswerDisplayAttachmentDiagram  = "diagram"
	AnswerDisplayAttachmentMarkdown = "markdown"
	AnswerDisplayAttachmentText     = "text"
)

// System-authored attachment sources: bodies the SYSTEM wrote in its own
// voice (deterministic content), as opposed to preserved MODEL output. The
// renderer forks the panel lead-in on this typed signal (P2-1, §29.47.1
// follow-up 2026-07-12: the system cross-check appendix must never be
// introduced as "模型已生成" content).
const (
	AnswerDisplayAttachmentSourceSystemCrossCheck = "orchestrator.system_crosscheck_appendix"
	AnswerDisplayAttachmentSourceDraftReviewNote  = "orchestrator.strict_review_disabled"
)

// SystemAuthored reports whether the attachment body is the system's own
// deterministic content (system voice). Unknown sources default to the
// model-provenance panel — the incumbent wording for preserved model output.
func (a AnswerDisplayAttachment) SystemAuthored() bool {
	switch a.Source {
	case AnswerDisplayAttachmentSourceSystemCrossCheck, AnswerDisplayAttachmentSourceDraftReviewNote:
		return true
	}
	return false
}
