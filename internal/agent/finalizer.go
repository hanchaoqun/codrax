package agent

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// NewFinalizerAgent creates the finalizer agent. The finalizer has
// exactly one evaluator: answerDocumentEvaluator, which emits a
// structured types.AnswerDocument via the emit_answer_document
// tool and renders it through internal/render/answerdoc.go.
func NewFinalizerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentFinalizer, deps, &answerDocumentEvaluator{
		maxRetries:           deps.AgentSettings.FinalizerMaxCorrectionRetries,
		preservePriorProse:   deps.AgentSettings.FinalizerPreservePriorProse,
		shrinkageMinProseLen: deps.AgentSettings.FinalizerShrinkageMinProseLen,
		shrinkageRatio:       deps.AgentSettings.FinalizerShrinkageRatio,
	})
}
