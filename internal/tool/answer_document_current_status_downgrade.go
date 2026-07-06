package tool

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_current_status_downgrade.go — SPR #72 (RTC ledger §8.3).
//
// Persist-time stamping of the current-status verdict evidence downgrade.
// Runs inside persistMergedAnswerDocument so BOTH emit paths (full +
// patch) share it, and every downstream renderer/gate reads one stamped
// carrier instead of re-deriving the lane state. See
// internal/types/current_status_verdict_downgrade.go for the semantics.

// stampCurrentStatusVerdictEvidenceDowngrade recomputes the downgrade
// stamp for the merged document. Idempotent per persist: any stale stamp
// from a previous emit is cleared first, so a later re-emit in a run that
// HAS gathered current_source evidence drops the downgrade again.
// Returns true when a downgrade is stamped.
func stampCurrentStatusVerdictEvidenceDowngrade(ctx *types.BusContext, doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	doc.CurrentStatusVerdictDowngrade = nil
	if ctx == nil {
		return false
	}
	var contract *types.CurrentStatusDiagnosticContract
	if view := types.BuildAnswerSemanticViewForBusContext(ctx); view != nil {
		contract = view.CurrentStatusDiagnostic
	}
	block := types.CurrentStatusDecisionBlock(doc)
	sidePicked := block != nil && block.CurrentStatusVerdict.PicksASide()
	required := contract != nil && contract.Required
	if !sidePicked && !required {
		// No current-status decision lane in play — skip the ledger
		// compile entirely so unrelated emits stay zero-cost.
		return false
	}
	doc.CurrentStatusVerdictDowngrade = types.ComputeCurrentStatusVerdictDowngrade(
		doc, contract, types.BusContextHasCurrentSourceObservationEvidence(ctx))
	return doc.CurrentStatusVerdictDowngrade != nil
}
