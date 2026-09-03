package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// stampCumulativeVerificationScope rebuilds the system-owned verification
// view from plans whose applied bytes remain in the workflow. It never widens
// apply scope: only verification consumers read this snapshot.
func (o *Orchestrator) stampCumulativeVerificationScope(plan *types.ChangePlan, run *types.WriteWorkflowRun, candidates ...*types.ChangePlan) {
	if plan == nil {
		return
	}
	// A probe-pass replan restores priorPlan by pointer identity. Its existing
	// cumulative scope was already stamped by this controller and is the only
	// route to plans applied before the restore cutoff. Preserve that exact
	// object snapshot before clearing the field. ID equality is deliberately
	// insufficient: a newly emitted planner plan may reuse an ID and its
	// cumulative field remains untrusted.
	var restoredScope *types.CumulativeVerificationScope
	var priorControllerScopes []*types.CumulativeVerificationScope
	liveAppliedPlanIDs := map[string]bool{}
	for _, id := range o.writeFinalReportAppliedPlanIDs(run) {
		if id = strings.TrimSpace(id); id != "" {
			liveAppliedPlanIDs[id] = true
		}
	}
	for _, candidate := range candidates {
		if candidate == plan && plan.CumulativeVerificationScope != nil {
			prior := plan.CumulativeVerificationScope
			restoredScope = &types.CumulativeVerificationScope{
				SourcePlanIDs:           append([]string(nil), prior.SourcePlanIDs...),
				TargetPaths:             append([]string(nil), prior.TargetPaths...),
				BehaviorContracts:       append([]types.WriteBehaviorContract(nil), prior.BehaviorContracts...),
				VerificationProbes:      append([]types.VerificationProbe(nil), prior.VerificationProbes...),
				ProjectTestObservations: append([]types.ProjectTestObservation(nil), prior.ProjectTestObservations...),
			}
			break
		}
		// A newly emitted proof-only plan is a different object from its
		// predecessor. The predecessor's cumulative scope was nevertheless
		// controller-authored and is the only in-memory carrier for transitive
		// verification rows when older durable artifacts live outside the
		// active blob root. Carry it only when every named source plan remains
		// live according to the restore-aware workflow ledger. This cannot
		// resurrect rolled-back work and never changes the active apply scope.
		if candidate == nil || candidate.CumulativeVerificationScope == nil {
			continue
		}
		prior := candidate.CumulativeVerificationScope
		if len(prior.SourcePlanIDs) == 0 {
			continue
		}
		allSourcesLive := true
		for _, rawID := range prior.SourcePlanIDs {
			id := strings.TrimSpace(rawID)
			if id == "" || !liveAppliedPlanIDs[id] {
				allSourcesLive = false
				break
			}
		}
		if allSourcesLive {
			priorControllerScopes = append(priorControllerScopes, prior)
		}
	}
	// Discard any planner-provided value before rebuilding controller
	// authority from exact workflow attempts and durable plan artifacts.
	plan.CumulativeVerificationScope = nil
	currentID := strings.TrimSpace(plan.ID)
	byID := map[string]*types.ChangePlan{}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if id := strings.TrimSpace(candidate.ID); id != "" {
			byID[id] = candidate
		}
	}
	var scope types.CumulativeVerificationScope
	contractIDs := map[string]bool{}
	probeIDs := map[string]bool{}
	projectObservationIDs := map[string]bool{}
	activeRebasedFallbackGeneration := plan.BehaviorContractGeneration == types.WriteBehaviorContractGenerationPlanAcceptanceRebase
	// The active plan is the newest typed generation. Seed its IDs before
	// collecting retained plans so an older contract/probe with the same ID
	// cannot re-enter the raw cumulative scope and contradict the active row.
	for _, contract := range plan.BehaviorContracts {
		if id := strings.TrimSpace(contract.ID); id != "" {
			contractIDs[id] = true
		}
	}
	// Typed verify-failure rebase tombstones have the same precedence as an
	// active row. Seed them before collecting restored/retained plans so the
	// raw cumulative snapshot cannot resurrect a superseded soft contract.
	// Membership comes from the one tombstone authority (typed carrier ∪
	// legacy id list).
	for id := range types.SupersededWriteBehaviorContractTombstones(plan) {
		contractIDs[id] = true
	}
	// §40.46: the run's ledger has the same precedence — a retained older
	// plan can never resurrect an id any generation of this run retired.
	if o != nil && o.busCtx != nil && o.busCtx.Mutable != nil {
		for _, tombstone := range o.busCtx.Mutable.BehaviorContractTombstoneLedger() {
			contractIDs[tombstone.ID] = true
		}
	}
	for _, probe := range plan.VerificationProbes {
		if id := strings.TrimSpace(probe.ID); id != "" {
			probeIDs[id] = true
		}
	}
	for _, observation := range plan.ProjectTestObservations {
		if id := strings.TrimSpace(observation.ID); id != "" {
			projectObservationIDs[id] = true
		}
	}
	trustedScopes := priorControllerScopes
	if restoredScope != nil {
		trustedScopes = append(trustedScopes, restoredScope)
	}
	for _, trustedScope := range trustedScopes {
		scope.SourcePlanIDs = append(scope.SourcePlanIDs, trustedScope.SourcePlanIDs...)
		scope.TargetPaths = append(scope.TargetPaths, trustedScope.TargetPaths...)
		for _, contract := range trustedScope.BehaviorContracts {
			if activeRebasedFallbackGeneration && types.IsExpectedOutcomeFallbackWriteBehaviorContract(contract) {
				continue
			}
			id := strings.TrimSpace(contract.ID)
			if id == "" || contractIDs[id] {
				continue
			}
			contractIDs[id] = true
			scope.BehaviorContracts = append(scope.BehaviorContracts, contract)
		}
		for _, probe := range trustedScope.VerificationProbes {
			id := strings.TrimSpace(probe.ID)
			if id == "" || probeIDs[id] {
				continue
			}
			probeIDs[id] = true
			scope.VerificationProbes = append(scope.VerificationProbes, probe)
		}
		for _, observation := range trustedScope.ProjectTestObservations {
			id := strings.TrimSpace(observation.ID)
			if id == "" || projectObservationIDs[id] {
				continue
			}
			projectObservationIDs[id] = true
			scope.ProjectTestObservations = append(scope.ProjectTestObservations, observation)
		}
	}
	for _, planID := range o.writeFinalReportAppliedPlanIDs(run) {
		planID = strings.TrimSpace(planID)
		if planID == "" || planID == currentID {
			continue
		}
		// A trusted transitive scope already contains the complete verify-only
		// projection for this live source plan. Do not require the older plan
		// artifact to be reachable from the active blob root as well.
		if cumulativeScopeContainsSourcePlanID(scope.SourcePlanIDs, planID) {
			continue
		}
		retained := byID[planID]
		if retained == nil {
			retained = o.loadDurablePlanArtifact(planID)
		}
		if retained == nil {
			continue
		}
		scope.SourcePlanIDs = append(scope.SourcePlanIDs, planID)
		// A restored plan can itself be a controller-stamped replan over older
		// still-applied bytes. Preserve that typed provenance and path closure;
		// otherwise a later restore makes plans before the restore generation
		// disappear from verification even though their bytes remain. This is
		// verify-only metadata and never widens the active plan's apply scope.
		if retained.CumulativeVerificationScope != nil {
			scope.SourcePlanIDs = append(scope.SourcePlanIDs, retained.CumulativeVerificationScope.SourcePlanIDs...)
			scope.TargetPaths = append(scope.TargetPaths, retained.CumulativeVerificationScope.TargetPaths...)
		}
		scope.TargetPaths = append(scope.TargetPaths, writeFinalReportPlanChangePaths(retained)...)
		for _, contract := range types.ChangePlanVerificationBehaviorContracts(retained) {
			if activeRebasedFallbackGeneration && types.IsExpectedOutcomeFallbackWriteBehaviorContract(contract) {
				continue
			}
			id := strings.TrimSpace(contract.ID)
			if id == "" || contractIDs[id] {
				continue
			}
			contractIDs[id] = true
			scope.BehaviorContracts = append(scope.BehaviorContracts, contract)
		}
		for _, probe := range types.ChangePlanVerificationProbes(retained) {
			id := strings.TrimSpace(probe.ID)
			if id == "" || probeIDs[id] {
				continue
			}
			probeIDs[id] = true
			scope.VerificationProbes = append(scope.VerificationProbes, probe)
		}
		for _, observation := range types.ChangePlanVerificationProjectTestObservations(retained) {
			id := strings.TrimSpace(observation.ID)
			if id == "" || projectObservationIDs[id] {
				continue
			}
			projectObservationIDs[id] = true
			scope.ProjectTestObservations = append(scope.ProjectTestObservations, observation)
		}
	}
	scope.SourcePlanIDs = writeFinalReportDedupStrings(scope.SourcePlanIDs)
	scope.TargetPaths = writeFinalReportDedupStrings(scope.TargetPaths)
	if len(scope.SourcePlanIDs) == 0 {
		return
	}
	plan.CumulativeVerificationScope = &scope
}

func cumulativeScopeContainsSourcePlanID(values []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
