package types

// FinalAnswerArtifactsV1 is the atomic Finalizer commit envelope:
// AnswerDocumentV2 (display) plus optional TraceFindingV1 (typed truth).
// Cluster/batch fields must never be added to AnswerDocumentV2.
type FinalAnswerArtifactsV1 struct {
	Document     AnswerDocumentV2 `json:"document"`
	TraceFinding *TraceFindingV1  `json:"trace_finding,omitempty"`
}

// FinalAnswerArtifactsMutation pairs a document mutation with an optional
// finding replacement for the unified persist chokepoint.
type FinalAnswerArtifactsMutation struct {
	Document     AnswerDocumentMutation
	TraceFinding *TraceFindingV1
	// ClearTraceFinding forces clearing a previously committed finding when
	// the patch does not replace it and inheritance is not allowed.
	ClearTraceFinding bool
}
