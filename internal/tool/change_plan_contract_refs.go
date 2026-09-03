package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// change_plan_contract_refs.go — V5-4 (colleague_merge_audit §40.24): every
// contract_refs / placement_refs gate resolves ids through ONE tombstone-aware
// helper over the plan's post-rebase generation. A retired id is rejected as
// "retired by <evidence>" (never "unknown"), an unknown id keeps the "unknown
// … use one of <active ids>" wording, and the sibling gates (verification
// probes vs project-test observations) can no longer disagree because they
// never read different snapshots. The pre-rebase analyzer snapshot is not an
// input here; a census test pins that no refs gate reads it.

const (
	verificationProbeContractRefsFailedReason         = "verification_probe_contract_refs_failed"
	verificationProbeContractRefRetiredReason         = "verification_probe_contract_ref_retired"
	projectTestObservationContractRefsFailedReason    = "project_test_observation_contract_refs_failed"
	projectTestObservationContractRefRetiredReason    = "project_test_observation_contract_ref_retired"
	plannerSupersededContractRefsInvalidReason        = "superseded_contract_refs_invalid"
	plannerSupersededContractRefsWithoutFailureReason = "superseded_contract_refs_without_verification_failure"
)

// resolveBehaviorContractIDs is the ruling-named resolver: the plan's active
// contract generation with its tombstones.
func resolveBehaviorContractIDs(plan *types.ChangePlan) types.WriteBehaviorContractResolution {
	return types.ResolveChangePlanBehaviorContractIDs(plan)
}

// validatePlanBehaviorContractRefs is the combined refs gate over the emitted
// plan: verification_probes[].contract_refs, verification_probes[].placement_refs
// and project_test_observations[].contract_refs. It returns the rejection, the
// typed repair reason code, and the failing field paths.
func validatePlanBehaviorContractRefs(plan *types.ChangePlan) (rej string, reasonCode string, fields []string) {
	if plan == nil {
		return "", "", nil
	}
	res := resolveBehaviorContractIDs(plan)
	active := res.ActiveIDs()
	placementIDs := types.PlacementRequiredWriteBehaviorContractIDs(res.Contracts)
	hasContractDomain := len(res.Contracts) > 0 || len(res.Tombstones) > 0
	// A plan without any typed contract domain cannot resolve probe refs at
	// all; the refs stay advisory there (unchanged behaviour of the former
	// probe gate, which returned early without contracts).
	if hasContractDomain {
		for i, probe := range plan.VerificationProbes {
			for _, ref := range probe.ContractRefs {
				if msg, retired := behaviorContractRefRejection(res, active, ref, fmt.Sprintf("verification_probes[%d].contract_refs", i)); msg != "" {
					return msg, probeContractRefReason(retired), []string{"$.verification_probes[].contract_refs", "$.changes[].verification_probes[].contract_refs"}
				}
			}
			for _, ref := range probe.PlacementRefs {
				field := fmt.Sprintf("verification_probes[%d].placement_refs", i)
				if msg, retired := behaviorContractRefRejection(res, active, ref, field); msg != "" {
					return msg, probeContractRefReason(retired), []string{"$.verification_probes[].placement_refs", "$.changes[].verification_probes[].placement_refs"}
				}
				if _, ok := placementIDs[strings.TrimSpace(ref)]; !ok {
					return fmt.Sprintf("%s contains behavior_contract id %q without placement{}; use one of %s", field, ref, formatStringSet(placementIDs)),
						verificationProbeContractRefsFailedReason, []string{"$.verification_probes[].placement_refs", "$.changes[].verification_probes[].placement_refs"}
				}
			}
		}
	}
	if len(plan.ProjectTestObservations) == 0 {
		return "", "", nil
	}
	if !hasContractDomain {
		return "project_test_observations require at least one typed behavior_contract", projectTestObservationContractRefsFailedReason, []string{"$.project_test_observations[].contract_refs"}
	}
	for i, observation := range plan.ProjectTestObservations {
		for _, ref := range observation.ContractRefs {
			if msg, retired := behaviorContractRefRejection(res, active, ref, fmt.Sprintf("project_test_observations[%d].contract_refs", i)); msg != "" {
				reason := projectTestObservationContractRefsFailedReason
				if retired {
					reason = projectTestObservationContractRefRetiredReason
				}
				return msg, reason, []string{"$.project_test_observations[].contract_refs"}
			}
		}
	}
	return "", "", nil
}

func probeContractRefReason(retired bool) string {
	if retired {
		return verificationProbeContractRefRetiredReason
	}
	return verificationProbeContractRefsFailedReason
}

// behaviorContractRefRejection words the rejection by resolved status. The
// retired wording names the failed attempt and its evidence so the planner
// can see why the id is closed instead of guessing at a typo.
func behaviorContractRefRejection(res types.WriteBehaviorContractResolution, active map[string]struct{}, ref, field string) (string, bool) {
	status, tombstone := res.Lookup(ref)
	switch status {
	case types.WriteBehaviorContractIDActive:
		return "", false
	case types.WriteBehaviorContractIDRetired:
		return fmt.Sprintf("%s contains behavior_contract id %q which %s and can no longer be verified; use one of %s",
			field, ref, describeBehaviorContractTombstone(tombstone), formatStringSet(active)), true
	default:
		return fmt.Sprintf("%s contains unknown behavior_contract id %q; use one of %s", field, ref, formatStringSet(active)), false
	}
}

// describeBehaviorContractTombstone renders one tombstone in model-facing
// words (no internal carrier names): "was retired after verification attempt
// 2 of plan P failed (tests_failed): failed verification probe probe:p1".
func describeBehaviorContractTombstone(t *types.WriteBehaviorContractTombstone) string {
	if t == nil {
		return "was retired by an earlier verification failure"
	}
	var b strings.Builder
	switch t.Reason {
	case types.WriteBehaviorContractRetiredPlannerSupersession:
		b.WriteString("was superseded by the repair plan")
	case types.WriteBehaviorContractRetiredFallbackGenerationRebase:
		b.WriteString("was replaced by the current plan's acceptance_tests")
	default:
		b.WriteString("was retired")
	}
	if t.Attempt > 0 || t.PlanID != "" {
		b.WriteString(" after verification attempt")
		if t.Attempt > 0 {
			fmt.Fprintf(&b, " %d", t.Attempt)
		}
		if t.PlanID != "" {
			fmt.Fprintf(&b, " of plan %s", t.PlanID)
		}
		b.WriteString(" failed")
		if t.FailureKind != "" {
			fmt.Fprintf(&b, " (%s)", t.FailureKind)
		}
	}
	if evidence := describeBehaviorContractRetirementEvidence(t.Reason, t.EvidenceRefs); evidence != "" {
		b.WriteString(": " + evidence)
	}
	return b.String()
}

func describeBehaviorContractRetirementEvidence(reason types.WriteBehaviorContractRetirementReason, refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	switch reason {
	case types.WriteBehaviorContractRetiredFailedVerificationProbe:
		return "failed verification probe " + strings.Join(refs, ", ")
	case types.WriteBehaviorContractRetiredFailedProjectTestAssertion:
		return "failed project test assertion " + strings.Join(refs, ", ")
	default:
		return "evidence " + strings.Join(refs, ", ")
	}
}

// validatePlannerSupersededContractRefs is the typed escape lane of the
// retention rule (§1.6): the planner may declare that a soft contract is
// superseded, but only on a verify-failure replan and only for ids that
// resolve, in the generation being projected (analyzer snapshot under the
// run's ledger and the current handoff, before this plan's own declaration),
// to a supersedable row — types.PlannerSupersedableWriteBehaviorContract,
// the SAME predicate the rebase tombstones on, so accept-set == retire-set
// (§40.46 C2/C5). An id the ledger already retired is accepted as a no-op
// re-declaration; hard, grounded, observed, or unknown ids are rejected with
// the row class named, so the lane cannot retire a proof obligation.
func validatePlannerSupersededContractRefs(ctx *types.BusContext, refs []string) (string, string) {
	trimmed := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref = strings.TrimSpace(ref); ref != "" {
			trimmed = append(trimmed, ref)
		}
	}
	if len(trimmed) == 0 || ctx == nil || ctx.Mutable == nil {
		return "", ""
	}
	if ctx.Mutable.VerifyFailureHandoff() == nil {
		return "superseded_contract_refs may only be declared on a repair plan after a failed verification attempt; this plan has no failed verification to supersede", plannerSupersededContractRefsWithoutFailureReason
	}
	generation := ctx.Mutable.ProjectBehaviorContractGeneration(nil, nil)
	byID := map[string]types.WriteBehaviorContract{}
	for _, contract := range generation.Contracts {
		if id := strings.TrimSpace(contract.ID); id != "" {
			byID[id] = contract
		}
	}
	for _, ref := range trimmed {
		status, _ := generation.Lookup(ref)
		if status == types.WriteBehaviorContractIDRetired {
			continue
		}
		contract, ok := byID[ref]
		if !ok {
			return fmt.Sprintf("superseded_contract_refs contains unknown behavior_contract id %q; only a soft (operator=satisfies, no evidence) or planning-only contract may be superseded", ref), plannerSupersededContractRefsInvalidReason
		}
		if supersedable, class := types.PlannerSupersedableWriteBehaviorContract(contract); !supersedable {
			return fmt.Sprintf("superseded_contract_refs contains behavior_contract id %q which is a %s contract and cannot be superseded by the planner; only a soft (operator=satisfies, no evidence) or planning-only contract may be declared superseded", ref, class), plannerSupersededContractRefsInvalidReason
		}
	}
	return "", ""
}
