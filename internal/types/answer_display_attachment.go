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
