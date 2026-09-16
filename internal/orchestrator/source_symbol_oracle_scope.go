package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// answerBlockUsesCurrentSourceSymbolOracle scopes repository-name diagnostics,
// not evidence admission. An external observation can contain identifier-shaped
// metrics, thread names or event labels without asserting a checkout declaration.
// Missing/mixed claim metadata keeps the legacy source checks unless the shared
// typed request decision explicitly excludes that source lane. This does not
// infer provenance from prose, spelling, graph style or the existence of buckets.
func answerBlockUsesCurrentSourceSymbolOracle(block types.AnswerBlock, mut *types.MutableState) bool {
	if rm := mut.RequestModel(); rm != nil && rm.CurrentSourceLaneDecision() == types.CurrentSourceLaneExcluded {
		return false
	}
	return !answerBlockHasOnlyExternalObservationClaimUses(block)
}
