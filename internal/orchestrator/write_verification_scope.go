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
	for _, planID := range o.writeFinalReportAppliedPlanIDs(run) {
		planID = strings.TrimSpace(planID)
		if planID == "" || planID == currentID {
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
		scope.TargetPaths = append(scope.TargetPaths, writeFinalReportPlanChangePaths(retained)...)
		for _, contract := range types.ChangePlanVerificationBehaviorContracts(retained) {
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
	}
	scope.SourcePlanIDs = writeFinalReportDedupStrings(scope.SourcePlanIDs)
	scope.TargetPaths = writeFinalReportDedupStrings(scope.TargetPaths)
	if len(scope.SourcePlanIDs) == 0 {
		return
	}
	plan.CumulativeVerificationScope = &scope
}
