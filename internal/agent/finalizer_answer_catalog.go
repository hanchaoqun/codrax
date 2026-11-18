package agent

import "github.com/hanchaoqun/codrax/internal/types"

const (
	answerDocumentToolName      = "emit_answer_document"
	answerDocumentPatchToolName = "emit_answer_document_patch"
)

func isAnswerDocumentTool(name string) bool {
	return name == answerDocumentToolName || name == answerDocumentPatchToolName
}

// answerAttemptResult is the one definition of an answer attempt: the result of
// an answer-tool call the request offered. A call to a tool the request did
// not offer never reached that tool: it submitted nothing a validator saw. The
// dispatch counter, the repair history and the patch-lane signals all read
// this same typed surface test. A nil test is an unknown surface (a direct
// caller), which is not evidence that a tool was unavailable.
func answerAttemptResult(toolName string, offered func(string) bool) bool {
	return isAnswerDocumentTool(toolName) && (offered == nil || offered(toolName))
}

// observedAnswerAttempts counts the answer attempts of one observed tool
// batch. Direct callers that report only the last result are counted from it.
func observedAnswerAttempts(obs LoopObservation) int {
	results := obs.CurrentToolResults
	if len(results) == 0 && obs.LastToolResult != nil {
		results = []types.ToolResult{*obs.LastToolResult}
	}
	attempts := 0
	for _, result := range results {
		if answerAttemptResult(result.ToolName, obs.ToolAvailable) {
			attempts++
		}
	}
	return attempts
}

// answerPatchCallNotOffered reports that the last observed result belongs to a
// patch call the request did not offer. There is no patch transaction to
// correct and no patch lane to continue: the result of that call already names
// the tools the request offers, and it is the only correction.
func answerPatchCallNotOffered(obs LoopObservation) bool {
	return obs.LastToolResult != nil && obs.LastToolResult.ToolName == answerDocumentPatchToolName &&
		!answerAttemptResult(answerDocumentPatchToolName, obs.ToolAvailable)
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
// No in-dispatch catalog choice is implemented. After a response that carried
// a single answer call, a patch base and a rejection already select the patch
// tool alone for the next request. The known exception is a response that
// carried an answer call and also called a tool the stage does not offer: its
// correction asks for the complete answer tool by name, so publishing the patch
// catalog alone would contradict it. The next request keeps both answer tools,
// that response is not recorded for the free-text rungs, and a request over the
// allocation is refused as before.
func (e *answerDocumentEvaluator) answerPatchOperandHidden(ctx *types.AgentContext) bool {
	if e == nil || ctx == nil || ctx.Mutable == nil || ctx.Stage != types.StageFinalize || e.mu != ctx.Mutable {
		return false
	}
	return e.dispatchAnswerAttempts == 0 && !e.postEmitAdvisoryDelivered &&
		ctx.Mutable.AnswerDocumentV2() == nil && !retryStateRendersPreviousEmit(ctx.Mutable.RetryState())
}
