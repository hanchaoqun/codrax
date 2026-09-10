package orchestrator

import (
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) persistPlanStatusWithApplied(status string, appliedAt *time.Time, appliedPaths []string) {
	if o.busCtx == nil {
		return
	}
	if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
		types.PreserveProofProbeOnlyPlanIdentity(plan)
		plan.Status = status
		if appliedAt != nil {
			plan.AppliedAt = appliedAt
		}
		if status == types.PlanStatusApplied &&
			(o.keepWorktreeOnSuccess || o.skipVerify) &&
			o.busCtx.WorktreePath != "" {
			o.stampPlanWorktree(plan)
		}
		if appliedPaths != nil {
			plan.AppliedPaths = appliedPaths
		}
		o.busCtx.Mutable.SetChangePlan(plan)
		o.persistCurrentChangePlanSnapshot()
		return
	}
	path := o.ensureChangePlanPath()
	if path == "" {
		return
	}
	wt := ""
	if status == types.PlanStatusApplied &&
		(o.keepWorktreeOnSuccess || o.skipVerify) &&
		o.busCtx.WorktreePath != "" {
		wt = o.busCtx.WorktreePath
	}
	if err := types.UpdatePlanStatusOnDiskWithApplied(path, status, appliedAt, wt, appliedPaths); err != nil {
		logging.Warning("[orchestrator] plan status update failed: %v", err)
		return
	}
	logging.Info("[orchestrator] plan status persisted: %s applied_paths=%d", status, len(appliedPaths))
}

// collectAppliedTargetPaths returns the subset of plan.TargetPaths
// that successfully landed in the worktree per WriteClosure.
// Used by applyPostHook to populate ChangePlan.AppliedPaths on
// partially_applied transitions. Returns nil when no plan or
// no applied set.
func (o *Orchestrator) collectAppliedTargetPaths() []string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil || len(plan.TargetPaths) == 0 {
		return nil
	}
	applied := o.busCtx.Mutable.WriteClosure().AppliedSet()
	if len(applied) == 0 {
		return nil
	}
	out := make([]string, 0, len(plan.TargetPaths))
	for _, p := range plan.TargetPaths {
		if applied[p] {
			out = append(out, p)
		}
	}
	return out
}

func (o *Orchestrator) persistPlanStatus(status string, appliedAt *time.Time) {
	if o.busCtx == nil {
		return
	}
	if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
		types.PreserveProofProbeOnlyPlanIdentity(plan)
		plan.Status = status
		if appliedAt != nil {
			plan.AppliedAt = appliedAt
		}
		if status == types.PlanStatusApplied &&
			(o.keepWorktreeOnSuccess || o.skipVerify) &&
			o.busCtx.WorktreePath != "" {
			o.stampPlanWorktree(plan)
		}
		o.busCtx.Mutable.SetChangePlan(plan)
		o.persistCurrentChangePlanSnapshot()
		return
	}
	path := o.ensureChangePlanPath()
	if path == "" {
		// In REPL plan-mode flow, the PlanStore writes the file AFTER
		// Run returns, so there's nothing on disk to update from here.
		// The REPL layer is responsible for that post-Run save.
		return
	}
	// Persist the worktree path alongside the status ONLY when
	// Fix 4's preserve-on-success fires (status=applied + yaml knob
	// on + real worktree). Any other status leaves the field
	// untouched so a later /reject or retry doesn't leak a stale
	// path onto the plan JSON.
	wt := ""
	// Persist worktree path on every successful apply that ends up
	// preserved — that's keep_on_success yaml knob OR --skip-verify
	// (which implies preserve, see the outer Run() defer).
	// Without persisting, /merge / /worktree list / /verify <id>
	// can't find the worktree even though it's still on disk.
	if status == types.PlanStatusApplied &&
		(o.keepWorktreeOnSuccess || o.skipVerify) &&
		o.busCtx.WorktreePath != "" {
		wt = o.busCtx.WorktreePath
	}
	if err := types.UpdatePlanStatusOnDisk(path, status, appliedAt, wt); err != nil {
		logging.Warning("[orchestrator] plan status update failed: %v", err)
	} else {
		logging.Info("[orchestrator] plan status updated: %s → %s", path, status)
	}
}
