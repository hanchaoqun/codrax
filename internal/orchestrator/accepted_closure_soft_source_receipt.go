package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// acceptedSoftCurrentSourceCompletionReceipt honors a decision already made
// by emit_investigation_complete; soft authority alone never closes a lane.
// The same predicate feeds the pre-mint filter and its post-filter backstop,
// removing only current_source debt. It grants no source evidence and leaves
// the callers' policy, backtrack, pending-read and other-origin checks intact.
func (o *Orchestrator) acceptedSoftCurrentSourceCompletionReceipt() bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.AnalysisIR == nil {
		return false
	}
	mut := o.busCtx.Mutable
	if !mut.IsInvestigationComplete() || mut.InvestigationCompleteGeneration() == 0 {
		return false
	}
	closure := mut.EvidenceClosure()
	if closure == nil || !closure.HasCompletionCaveat(types.DowngradeLaneCurrentSourceLane) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(o.busCtx, types.ObservationExtractLedgerEvidenceLimit))
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(o.busCtx, ledger)
	return authority.Active && authority.HasRuntimeCarrier() &&
		authority.CurrentSourceRequirement == types.RuntimeSourceRequirementSoft &&
		!authority.CanHardBlockCompletion && authority.CanDowngradeToCaveat &&
		authority.CanUseRuntimeOnlyWithCaveat
}
