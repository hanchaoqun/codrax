package agent

import "github.com/hanchaoqun/codrax/internal/types"

const (
	answerDocumentToolName      = "emit_answer_document"
	answerDocumentPatchToolName = "emit_answer_document_patch"
)

func isAnswerDocumentTool(name string) bool {
	return name == answerDocumentToolName || name == answerDocumentPatchToolName
}

// observedAnswerAttempts counts the answer-tool results of one observed tool
// batch. A call to a tool the request did not offer never reached that tool:
// it submitted nothing a validator saw, so it is not an attempt. Direct
// callers that report only the last result are counted from it.
func observedAnswerAttempts(obs LoopObservation) int {
	results := obs.CurrentToolResults
	if len(results) == 0 && obs.LastToolResult != nil {
		results = []types.ToolResult{*obs.LastToolResult}
	}
	attempts := 0
	for _, result := range results {
		if isAnswerDocumentTool(result.ToolName) && obs.ToolAvailable(result.ToolName) {
			attempts++
		}
	}
	return attempts
}

// retryStateRendersPreviousEmit reports whether the dynamic instruction of this
// dispatch shows a previous structured payload (the retry lane that restates
// the earlier draft together with its required changes).
func retryStateRendersPreviousEmit(rs *types.RetryState) bool {
	return rs != nil && rs.HasContent() && retryStateHasPreviousEmit(rs)
}

// answerPatchOperandHidden names the one dispatch state in which a patch call
// would edit a draft the request does not show: the evaluator is bound to this
// dispatch, the dispatch has observed no answer attempt of its own, nothing is
// accepted, it is not the accepted-answer advisory round, and its instruction
// renders no previous emit. A retained draft of an earlier dispatch stays the
// patch base for later rounds; it only does not make the first request offer a
// second answer tool. Typed dispatch state only — no request, model or
// validator text is read.
//
// No in-dispatch counterpart is needed: once a dispatch has an attempt of its
// own and a patch base exists, every rejection of a single answer call already
// selects the patch tool alone for the next request, so two answer catalogs are
// never published beside a recorded attempt.
func (e *answerDocumentEvaluator) answerPatchOperandHidden(ctx *types.AgentContext) bool {
	if e == nil || ctx == nil || ctx.Mutable == nil || ctx.Stage != types.StageFinalize || e.mu != ctx.Mutable {
		return false
	}
	return e.dispatchAnswerAttempts == 0 && !e.postEmitAdvisoryDelivered &&
		ctx.Mutable.AnswerDocumentV2() == nil && !retryStateRendersPreviousEmit(ctx.Mutable.RetryState())
}
