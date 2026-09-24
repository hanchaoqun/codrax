package orchestrator

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// writeFinalReportMaterialization projects only typed apply history and its
// persisted checkpoints. Git ancestry, object availability, the independent
// pre-run seed and actual commit deltas belong to the materializing consumer.
// Existing delivery/source/verification/merge judgments remain independent.
func (o *Orchestrator) writeFinalReportMaterialization(run *types.WriteWorkflowRun, finalPlan *types.ChangePlan) *types.WriteFinalMaterializationReceipt {
	out := &types.WriteFinalMaterializationReceipt{
		SchemaVersion: types.WriteFinalMaterializationSchemaVersion, Status: "unavailable",
		RetainedPlanIDs: []string{}, Owners: []types.WriteFinalMaterializationOwner{},
	}
	if run != nil {
		out.RunID = strings.TrimSpace(run.RunID)
	}
	if finalPlan != nil {
		out.FinalPlanID = strings.TrimSpace(finalPlan.ID)
	}
	unavailable := func(reason string) *types.WriteFinalMaterializationReceipt {
		out.ReasonCode = reason
		return out
	}
	if run == nil || out.RunID == "" {
		return unavailable("materialization_run_missing")
	}
	if finalPlan == nil || out.FinalPlanID == "" {
		return unavailable("materialization_final_plan_missing")
	}

	// The existing restore-aware collection names retained roots, not their
	// transitive apply ancestry. Retain every historical candidate separately
	// so the consumer can include A when restoring B whose commit contains A,
	// without resurrecting a later rolled-back C.
	var history []string
	seen := map[string]bool{}
	for _, batch := range run.Batches {
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) != "apply" || strings.TrimSpace(attempt.Status) != "applied" {
				continue
			}
			id := strings.TrimSpace(attempt.PlanID)
			if id == "" {
				return unavailable("applied_plan_id_missing")
			}
			if !seen[id] {
				seen[id] = true
				history = append(history, id)
			}
		}
	}
	if !seen[out.FinalPlanID] && writeFinalMaterializationHasAppliedMutation(finalPlan) {
		return unavailable("final_apply_attempt_missing")
	}

	owners := make([]types.WriteFinalMaterializationOwner, 0, len(history))
	ownerIDs := map[string]bool{}
	proofOnly := false
	for _, id := range history {
		plan := finalPlan
		if id != out.FinalPlanID {
			if o == nil || o.busCtx == nil {
				return unavailable("applied_plan_missing")
			}
			loaded, err := o.loadWriteFinalReportChangePlan(id)
			if err != nil || loaded == nil {
				return unavailable("applied_plan_missing")
			}
			plan = loaded
		}
		if strings.TrimSpace(plan.ID) != id {
			return unavailable("applied_plan_identity_mismatch")
		}
		if writeFinalMaterializationStrictProofOnly(plan) {
			proofOnly = true
			continue
		}
		checkpoint := plan.ApplyCheckpoint
		if checkpoint == nil {
			return unavailable("apply_checkpoint_missing")
		}
		if checkpoint.Partial {
			return unavailable("apply_checkpoint_partial")
		}
		if strings.TrimSpace(checkpoint.CommitError) != "" {
			return unavailable("apply_checkpoint_commit_failed")
		}
		sha := strings.TrimSpace(checkpoint.CommitSHA)
		if sha == "" {
			return unavailable("checkpoint_commit_sha_missing")
		}
		if applied := strings.TrimSpace(plan.AppliedCommitSHA); applied == "" || applied != sha {
			return unavailable("checkpoint_applied_sha_mismatch")
		}
		// A failed recovery-ref pin does not erase a successful checkpoint.
		// Preserve its existing TagError disclosure and let the consumer prove
		// SHA resolution. A present ref must still name this exact plan.
		if ref := strings.TrimSpace(checkpoint.RecoveryRef); ref != "" && ref != worktree.AppliedRef(id) {
			return unavailable("checkpoint_recovery_ref_mismatch")
		}
		paths, ok := writeFinalMaterializationCheckpointPaths(checkpoint.CommittedPaths)
		if !ok {
			return unavailable("checkpoint_paths_missing_or_invalid")
		}
		owners = append(owners, types.WriteFinalMaterializationOwner{PlanID: id, CommitSHA: sha, Paths: paths})
		ownerIDs[id] = true
	}
	retained := make([]string, 0, len(owners))
	for _, id := range o.writeFinalReportAppliedPlanIDs(run) {
		// Only strictly proven no-mutation proof plans can be omitted here;
		// every other historical row was validated above or failed the receipt.
		if ownerIDs[id] {
			retained = append(retained, id)
		}
	}
	out.Status, out.ReasonCode = "available", "applied_checkpoint_candidates"
	out.Owners, out.RetainedPlanIDs = owners, retained
	if len(owners) == 0 {
		out.ReasonCode = "no_applied_mutations"
		if proofOnly {
			out.ReasonCode = "proof_only_no_applied_mutation"
		}
	}
	return out
}

func writeFinalMaterializationStrictProofOnly(plan *types.ChangePlan) bool {
	if plan == nil || writeFinalMaterializationHasAppliedMutation(plan) {
		return false
	}
	copyPlan := *plan
	types.PreserveProofProbeOnlyPlanIdentity(&copyPlan)
	return types.IsPersistedProofProbeOnlyPlan(&copyPlan) || types.IsPersistedNativeTestRegistrationPlan(&copyPlan)
}

func writeFinalMaterializationHasAppliedMutation(plan *types.ChangePlan) bool {
	if plan == nil {
		return false
	}
	if plan.ApplyCheckpoint != nil || strings.TrimSpace(plan.AppliedCommitSHA) != "" || len(plan.AppliedPaths) > 0 ||
		(plan.PatchEffect != nil && (plan.PatchEffect.DiffBytes != 0 || len(plan.PatchEffect.Files) > 0)) {
		return true
	}
	if len(plan.Changes) > 0 {
		switch plan.Status {
		case types.PlanStatusApplied, types.PlanStatusAppliedPendingVerify, types.PlanStatusVerifyFailed,
			types.PlanStatusUnverified, types.PlanStatusPartiallyApplied, types.PlanStatusMerged:
			return true
		}
	}
	return false
}

func writeFinalMaterializationCheckpointPaths(in []string) ([]string, bool) {
	if len(in) == 0 {
		// CommittedPaths uses omitempty in the durable checkpoint. A complete
		// SHA can therefore legitimately carry no staged paths; the consumer
		// must prove that this commit's actual delta is empty.
		return []string{}, true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		// Do not repair an unsafe/empty member into a narrower apparent set.
		clean := path.Clean(raw)
		if strings.TrimSpace(raw) == "" || raw != clean || filepath.IsAbs(raw) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, false
		}
		if !seen[raw] {
			seen[raw] = true
			out = append(out, raw)
		}
	}
	sort.Strings(out)
	return out, true
}
