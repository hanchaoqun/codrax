package orchestrator

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Resolve the retained source from the live apply ledger, never from the new
// registration's self-described source. Restore must still find the original
// source artifact; an imported registration JSON alone is not authority.
func (o *Orchestrator) nativeTestRegistrationSource(prior *types.ChangePlan) (types.VerificationDeliverySnapshot, []types.WriteBehaviorContract, []string, error) {
	mu := o.busCtx.Mutable
	run := mu.WriteWorkflowRun()
	ids := o.writeFinalReportAppliedPlanIDs(run)
	if !activeBatchProofFollowupPurpose(run) || len(ids) != 1 {
		return types.VerificationDeliverySnapshot{}, nil, nil, fmt.Errorf("read-only registration requires exactly one live applied source")
	}
	source := o.loadDurablePlanArtifact(ids[0])
	if source == nil && prior != nil && prior.ID == ids[0] {
		source = prior
	}
	delivery, ok := verificationSourcePlanDelivery(source)
	if !ok {
		return delivery, nil, nil, fmt.Errorf("original applied source artifact is unavailable")
	}
	if prior != nil && prior.ID == ids[0] {
		other, valid := verificationSourcePlanDelivery(prior)
		if !valid || !verificationDeliveriesEqual(delivery, other) {
			return delivery, nil, nil, fmt.Errorf("source delivery identities conflict")
		}
	}
	retired := map[string]bool{}
	for _, row := range mu.BehaviorContractTombstoneLedger() {
		retired[row.ID] = true
	}
	var contracts []types.WriteBehaviorContract
	byID := map[string]string{}
	for _, contract := range types.ChangePlanVerificationBehaviorContracts(source) {
		if retired[contract.ID] {
			continue
		}
		body, _ := json.Marshal(contract)
		byID[contract.ID] = string(body)
		contracts = append(contracts, contract)
	}
	// A new/rebased requirement needs ordinary planning, not a read-only
	// registration against old source requirements that happen to share IDs.
	for _, contract := range mu.ProjectBehaviorContractGeneration(nil, nil).Contracts {
		body, _ := json.Marshal(contract)
		if byID[contract.ID] != string(body) {
			return delivery, nil, nil, fmt.Errorf("active contract definition differs from retained source")
		}
	}
	if len(contracts) == 0 {
		return delivery, nil, nil, fmt.Errorf("no active retained source contracts")
	}
	return delivery, contracts, types.ChangePlanVerificationTargetPaths(source, run), nil
}

func (o *Orchestrator) nativeTestRegistrationRoot() (string, error) {
	root, err := filepath.EvalSymlinks(o.busCtx.RepoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Abs(root)
}

func (o *Orchestrator) prepareNativeTestRegistration(prior *types.ChangePlan) {
	mu := o.busCtx.Mutable
	mu.RevokeNativeTestRegistrationAuthorization()
	delivery, contracts, targets, err := o.nativeTestRegistrationSource(prior)
	if err != nil {
		return
	}
	root, err := o.nativeTestRegistrationRoot()
	if err != nil {
		return
	}
	_ = mu.AuthorizeNativeTestRegistration(root, delivery, contracts, targets)
}

func (o *Orchestrator) authorizeNativeTestRegistrationVerification(plan *types.ChangePlan) error {
	mu := o.busCtx.Mutable
	mu.RevokeNativeTestRegistrationExecution()
	if plan == nil || (plan.NativeTestRegistration == nil && plan.PersistenceKind != types.PlanPersistenceNativeTestRegistration) {
		return nil
	}
	if o.skipVerify {
		return fmt.Errorf("read-only test registration cannot be completed with verification skipped")
	}
	delivery, contracts, _, err := o.nativeTestRegistrationSource(nil)
	if err != nil {
		return err
	}
	root, err := o.nativeTestRegistrationRoot()
	if err != nil {
		return err
	}
	return mu.AuthorizeNativeTestRegistrationExecution(plan, root, delivery, contracts)
}

func changePlanIsReadOnlyProof(plan *types.ChangePlan) bool {
	return changePlanIsProofProbeOnly(plan) || types.IsPersistedNativeTestRegistrationPlan(plan)
}
