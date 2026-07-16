package agent

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalizerIdenticalErrorStreak raises the generic LoopPolicy
// IdenticalErrorStreak (3) to 4 for the finalize dispatch only (R10,
// §29.104.7, user ruling 2026-07-15). The coarse same-error-CLASS gate keys
// on tool+repair-code+fields, so a repair sequence that IS making member-set
// progress (different members each round) still reads as "identical" and was
// the gate that actually killed the XGAP witness loop after 5 rejects. One
// extra class round is modest headroom; genuine zero-progress storms are
// stopped EARLIER by the finer F8-T4 missing-obligation fingerprint breaker
// (memberSetCoverageRejectBreakerSignal). Other agents keep the default.
const finalizerIdenticalErrorStreak = 4

// NewFinalizerAgent creates the finalizer agent. The finalizer has
// exactly one evaluator: answerDocumentEvaluator, which emits a
// structured types.AnswerDocumentV2 via the emit_answer_document
// tool and renders it through internal/render/answerdoc.go.
func NewFinalizerAgent(deps *Dependencies) Agent {
	fdeps := *deps
	lp := fdeps.LoopPolicy
	if lp == (LoopPolicy{}) {
		lp = DefaultLoopPolicy()
	}
	lp.IdenticalErrorStreak = finalizerIdenticalErrorStreak
	fdeps.LoopPolicy = lp
	return NewBaseAgent(types.AgentFinalizer, &fdeps, &answerDocumentEvaluator{
		maxRetries:           deps.AgentSettings.FinalizerMaxCorrectionRetries,
		preservePriorProse:   deps.AgentSettings.FinalizerPreservePriorProse,
		shrinkageMinProseLen: deps.AgentSettings.FinalizerShrinkageMinProseLen,
		shrinkageRatio:       deps.AgentSettings.FinalizerShrinkageRatio,
	})
}
