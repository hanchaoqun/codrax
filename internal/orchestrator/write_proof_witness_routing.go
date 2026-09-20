package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// proofPlanningAssertionWitnessUnavailable is a dispatch guard, not a proof
// verdict. It suppresses only an impossible assertion-only inline-probe batch,
// not native-test re-verification or useful changed-target execution work. An
// existing native assertion declaration keeps that path open; discovering and
// binding an undeclared native assertion needs its own controller authority.
// Unknown/mixed obligations stay on the existing routing path. This guard must
// never mint observations or change the existing plan/report.
func proofPlanningAssertionWitnessUnavailable(plan *types.ChangePlan, ledger types.VerificationProofLedger, paths []string, runtimeAvailable func(string) bool) bool {
	if plan == nil || runtimeAvailable == nil {
		return false
	}
	// A classified non-authoritative failed probe can still be corrected to
	// obtain execution evidence. Do not confuse that capability failure with
	// the assertion-only debt left after otherwise successful verification.
	if ledger.FailedCount > 0 || ledger.CapabilityFailedCount > 0 {
		return false
	}
	contracts := types.ChangePlanVerificationBehaviorContracts(plan)
	required := types.RequiredWriteBehaviorContractIDs(contracts, true)
	byID := map[string]types.WriteBehaviorContract{}
	for _, contract := range contracts {
		byID[strings.TrimSpace(contract.ID)] = contract
	}
	nativeRefs := map[string]bool{}
	for _, observation := range types.ChangePlanVerificationProjectTestObservations(plan) {
		if strings.TrimSpace(observation.TestPath) == "" || strings.TrimSpace(observation.AssertionSuite) == "" || strings.TrimSpace(observation.AssertionID) == "" {
			continue
		}
		for _, ref := range observation.ContractRefs {
			nativeRefs[strings.TrimSpace(ref)] = true
		}
	}
	assertionDebt := false
	for _, obligation := range ledger.Obligations {
		if !verificationProofLedgerObligationNeedsFollowup(obligation.Status) {
			continue
		}
		ref := strings.TrimSpace(obligation.ContractRef)
		contract, known := byID[ref]
		if _, active := required[ref]; ref == "" || !known || !active {
			return false
		}
		switch obligation.Kind {
		case "behavior_contract":
			if nativeRefs[ref] || types.WriteBehaviorWitnessSatisfies(contract.Kind, types.WriteBehaviorWitnessSourceText) {
				return false
			}
		case "rendered_text_placement_contract":
			// ProjectTestObservation binds behavior refs, not placement refs.
		default:
			return false
		}
		assertionDebt = true
	}
	if !assertionDebt {
		return false
	}
	seenRuntime := false
	for _, raw := range paths {
		path := normalizeControllerPath(raw)
		if path == "" || types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(path)) {
			continue
		}
		language := controllerDirectInlineProbeLanguage(path)
		if language == "" || !runtimeAvailable(language) || !types.VerificationProbeUsesExecutionOnlyWitness(language) {
			return false
		}
		seenRuntime = true
	}
	return seenRuntime
}
