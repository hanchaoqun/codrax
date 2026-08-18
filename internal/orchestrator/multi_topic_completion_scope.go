package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// acceptedCompletionMustRemainWindowScoped keeps a successful
// emit_investigation_complete local to the dispatched probe/evidence window
// while the typed DAG still has a required evidence lane for another
// analyzer-authored sub-topic.
//
// The caller retains the accepted evidence and rationale, marks the current
// window done, and consumes the single completion-reset throat before the next
// scheduler environment is built. This ordering prevents stale completion
// state from firing stopcond or accepted-closure auto-completion before the
// remaining topic lane can be dispatched.
func (o *Orchestrator) acceptedCompletionMustRemainWindowScoped(state *graphState, ir *types.AnalysisIR) bool {
	if !state.hasPendingRequiredMultiTopicEvidence(ir) {
		return false
	}
	logging.Info("[orchestrator] accepted explore completion scoped to current window; required multi-topic evidence remains")
	return true
}
