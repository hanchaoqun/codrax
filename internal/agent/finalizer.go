package agent

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// NewFinalizerAgent creates the finalizer agent. After the
// 2026-04-14 simplification the finalizer has exactly one
// implementation: answerDocumentEvaluator, which emits a
// structured types.AnswerDocument via the emit_answer_document
// tool and renders it through internal/render/answerdoc.go.
//
// The pre-cleanup legacy prose finalizer (finalizerEvaluator with
// S3 symbol-set validation, shape validators, retry loop) was
// deleted in the same pass along with the answer_document_mode
// flag that used to gate the choice.
func NewFinalizerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentFinalizer, deps, &answerDocumentEvaluator{})
}
