package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) downgradeRuntimeAcceptedClosureExploreFallback(fallback FallbackTarget, violations []types.Violation) FallbackTarget {
	if fallback != FallbackBackToExplore || !o.shouldDowngradeRuntimeAcceptedClosureExploreFallback(violations) {
		return fallback
	}
	logging.Info("[orchestrator] runtime accepted-closure fallback guard: downgrading %s to %s because runtime/source authority has no precise current-source hard blocker",
		fallback, FallbackFinalizerOnly)
	return FallbackFinalizerOnly
}

func (o *Orchestrator) shouldDowngradeRuntimeAcceptedClosureExploreFallback(violations []types.Violation) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || len(violations) == 0 {
		return false
	}
	mut := o.busCtx.Mutable
	if !mut.IsInvestigationComplete() && strings.TrimSpace(mut.StableInvestigationCompleteReason()) == "" {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(o.busCtx, types.ObservationExtractLedgerEvidenceLimit))
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(o.busCtx, ledger)
	if !authority.Active || authority.CanHardBlockCompletion ||
		authority.CurrentSourceRequirement == types.RuntimeSourceRequirementPrecise {
		return false
	}
	if !authority.HasRuntimeCarrier() {
		return false
	}
	for _, violation := range violations {
		if !runtimeAcceptedClosureSoftSourceDebtViolation(violation) {
			return false
		}
	}
	return true
}

func runtimeAcceptedClosureSoftSourceDebtViolation(violation types.Violation) bool {
	if violation.RepairLocusOverride == types.LocusExplore {
		return false
	}
	kind := violation.Kind
	if violation.RootKind != "" {
		kind = violation.RootKind
	}
	switch kind {
	case types.ViolCitation,
		types.ViolGhostAnchor,
		types.ViolAcceptance,
		types.ViolSuccessCriterion,
		types.ViolChainDemoted:
		return true
	case types.ViolFacetUncovered:
		return runtimeAcceptedClosureSoftFacetDebtViolation(violation)
	default:
		return false
	}
}

func runtimeAcceptedClosureSoftFacetDebtViolation(violation types.Violation) bool {
	root := residualClusterValue(violation.ClusterKey, "root")
	facet := types.AnswerFacetKind(residualClusterValue(violation.ClusterKey, "facet"))
	if facet == "" {
		return false
	}
	switch root {
	case "answer_richness_facet_coverage":
		return true
	case "answer_facet_coverage":
	default:
		return false
	}
	switch facet {
	case types.FacetObservedArtifactFact,
		types.FacetCurrentCodePath,
		types.FacetUncertaintyBoundary,
		types.FacetDiagramSpine,
		types.FacetComponentRelation:
		return true
	default:
		return false
	}
}
