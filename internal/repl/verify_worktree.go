package repl

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// handleVerifyCmd re-runs the verify phase against an existing
// ChangePlan without re-applying. Two forms:
//
//	/verify            — use pendingPlanPath (the most recent plan)
//	/verify <plan-id>  — use the named plan (must live in PlanStore)
//
// Uses ModeVerify which the orchestrator already implements
// (cmd/root.go --mode=verify wires the same path). Requires a plan
// with a WorktreePath set (preserved via Fix 4's keep-on-success)
// since verify has to run tests against applied bytes.
//
// Common motivation: a user /approve'd, verify flaked (or the user
// wants to re-check after fixing a flaky test manually in the
// worktree), and they want a verdict refresh without re-dispatching
// plan + apply.
func (r *REPL) handleVerifyCmd(line string) {
	if r.planStore == nil {
		r.info("/verify disabled (no PlanStore configured)")
		return
	}
	arg := strings.TrimSpace(strings.TrimPrefix(line, "/verify"))

	var planPath string
	if arg == "" {
		if r.pendingPlanPath == "" {
			r.info("no pending plan — supply a plan id (see /plan list) or /mode plan first")
			return
		}
		planPath = r.pendingPlanPath
	} else {
		planPath = filepath.Join(r.planStore.PlanDir(), arg+".json")
	}
	id := strings.TrimSuffix(filepath.Base(planPath), ".json")
	plan, err := r.planStore.Load(id)
	if err != nil {
		r.errorf("/verify: %v\n", err)
		return
	}

	// ModeVerify re-runs tests against an applied worktree. We
	// require the plan to already be at status=applied (or
	// verify_failed — the user may be re-checking after a flake);
	// verifying a pending plan makes no sense because no bytes were
	// written yet.
	switch plan.Status {
	case types.PlanStatusApplied, types.PlanStatusVerifyFailed:
		// ok — proceed
	default:
		r.warn("/verify requires plan status applied or verify_failed; plan %s is %q\n",
			plan.ID, plan.Status)
		return
	}

	// The orchestrator will swap RepoRoot to a worktree — but
	// ModeVerify reuses the plan's recorded WorktreePath rather
	// than creating a fresh one. Without a recorded path the
	// orchestrator falls back to a fresh checkout which defeats the
	// purpose of re-verifying applied bytes.
	if plan.WorktreePath == "" {
		r.warn("/verify: plan %s has no preserved worktree (enable pipeline_keep_worktree_on_success "+
			"to preserve worktrees on successful apply)\n", plan.ID)
		return
	}

	mSetter, mOK := r.runner.(modeSetter)
	pSetter, pOK := r.runner.(planPathSetter)
	if !mOK || !pOK {
		r.warn("/verify requires a runner with SetMode + SetPlanPath (stub runner detected)\n")
		return
	}
	originalMode := r.currentMode
	mSetter.SetMode(types.ModeVerify)
	pSetter.SetPlanPath(planPath)
	// Hand the preserved worktree path to the orchestrator so the
	// verify pre-hook swaps RepoRoot to it. Without this the verify
	// agent's run_tests would execute against the unmodified main
	// repo, producing a misleading verdict.
	wSetter, wOK := r.runner.(reuseWorktreeSetter)
	if wOK {
		wSetter.SetReuseWorktreePath(plan.WorktreePath)
	} else {
		r.warn("/verify: runner doesn't support SetReuseWorktreePath; tests will run against the main repo (stub runner)\n")
	}
	defer func() {
		mSetter.SetMode(originalMode)
		pSetter.SetPlanPath("")
		if wOK {
			wSetter.SetReuseWorktreePath("")
		}
	}()

	// See approveDispatchRequest for why a literal "/verify <id>"
	// can't be the synthetic request: it would trip the analyzer's
	// REPL-control-input rejection and burn iterations.
	request := verifyDispatchRequest(plan)
	logging.Info("[repl] dispatching verify: plan=%s path=%s", plan.ID, planPath)
	r.info(verifyDispatching(r.language, plan.ID))
	if r.renderer != nil {
		r.renderer.StartSpinner()
	}
	busCtx, runErr := r.runner.Run(request, r.repoRoot, r.branch)
	if r.renderer != nil {
		r.renderer.StopSpinner()
	}
	if runErr != nil {
		logging.Error("[repl] verify failed: %v", runErr)
		r.errorf("verify: %v\n", runErr)
		return
	}

	response := strings.TrimSpace(r.render(busCtx))
	if response == "" || response == "(no result)" {
		fmt.Fprintln(r.out, "  (no result)")
	} else {
		r.renderBordered(response)
	}
	r.recordTurn(request, request, response, memory.KindPlan)
}

// handleWorktreeCmd dispatches /worktree subcommands. Recognised
// forms:
//
//	/worktree              — alias for /worktree list
//	/worktree list         — show every plan with a preserved worktree
//	/worktree show         — alias for /worktree list (operator
//	                         convenience: many users type `show` first)
//	/worktree discard <id> — remove the worktree for plan <id>
//	/worktree gc           — apply TTL + max-count caps now (manual
//	                         trigger of T3.1's startup reaper)
//
// "Preserved" means Fix 4's keep-on-success fired: the plan's
// Status=applied and its WorktreePath field was persisted by
// UpdatePlanStatusOnDisk after a successful apply+verify.
func (r *REPL) handleWorktreeCmd(line string) {
	if r.planStore == nil {
		r.info("/worktree disabled (no PlanStore configured)")
		return
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/worktree"))
	if rest == "" {
		rest = "list"
	}
	fields := strings.Fields(rest)
	switch fields[0] {
	case "list", "show":
		// `show` is the natural verb operators reach for after a
		// failed merge ("let me see what's there"); accept it as
		// an alias for list rather than printing usage and forcing
		// them to retype.
		r.worktreeList()
	case "discard":
		if len(fields) < 2 {
			r.info("/worktree discard <plan-id> — specify which preserved worktree to remove")
			return
		}
		r.worktreeDiscard(fields[1])
	case "gc":
		r.worktreeGC()
	default:
		r.info("/worktree subcommands: list (or `show`), discard <plan-id>, gc")
	}
}

// worktreeGC manually triggers the TTL + quota reaper that runs
// at startup. Useful when an operator wants to clean stale kept
// worktrees mid-session (e.g. just finished a heavy review batch
// and the disk is low). Reads the same yaml knobs the startup
// path uses; reports the count reaped.
func (r *REPL) worktreeGC() {
	if r.runtimeAnchor == "" {
		r.warn("/worktree gc: runtime anchor unset — cannot locate worktree base\n")
		return
	}
	base := filepath.Join(r.runtimeAnchor, "worktrees")
	reaped := worktree.PruneByAgeAndQuota(base, r.repoRoot,
		r.worktreeKeepTTL, r.worktreeKeepMaxCount)
	if reaped == 0 {
		r.info(fmt.Sprintf("/worktree gc: no preserved worktrees exceeded ttl=%v / max=%d", r.worktreeKeepTTL, r.worktreeKeepMaxCount))
		return
	}
	r.success(fmt.Sprintf("/worktree gc: reaped %d worktree(s) by ttl=%v / max=%d", reaped, r.worktreeKeepTTL, r.worktreeKeepMaxCount))
}

// worktreeList walks PlanStore and surfaces every plan whose
// WorktreePath field is non-empty. Output sorted by CreatedAt
// descending (newest first) so the row the user just /approve'd
// appears at the top.
func (r *REPL) worktreeList() {
	infos, err := r.planStore.List()
	if err != nil {
		r.errorf("/worktree list: %v\n", err)
		return
	}
	type row struct {
		info PlanInfo
		plan *types.ChangePlan
	}
	var rows []row
	for _, info := range infos {
		// Status filter: only applied plans can have preserved
		// worktrees; rejected / pending / apply_failed never had
		// bytes written. verify_failed theoretically could have a
		// worktree (apply landed) but we only set WorktreePath on
		// status=applied, so the field gate is sufficient.
		if info.Status != types.PlanStatusApplied {
			continue
		}
		plan, err := r.planStore.Load(info.ID)
		if err != nil {
			logging.Warning("[repl] /worktree list: load %s: %v", info.ID, err)
			continue
		}
		if plan.WorktreePath == "" {
			continue
		}
		rows = append(rows, row{info, plan})
	}
	if len(rows) == 0 {
		r.info("no preserved worktrees found (enable pipeline_keep_worktree_on_success to preserve on successful apply)")
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].info.ModTime > rows[j].info.ModTime
	})
	fmt.Fprintln(r.out, "  preserved worktrees:")
	for _, row := range rows {
		fmt.Fprintf(r.out, "    - %s  worktree=%s  summary=%s\n",
			row.plan.ID, row.plan.WorktreePath, oneLine(row.plan.Summary))
	}
}

// worktreeDiscard removes the git worktree + files for the named
// plan and clears the WorktreePath field on disk so /worktree list
// no longer surfaces a stale path. Leaves the plan's Status=applied
// intact — the apply outcome is historical and shouldn't be rewritten
// just because the user cleaned up the sandbox dir.
func (r *REPL) worktreeDiscard(planID string) {
	plan, err := r.planStore.Load(planID)
	if err != nil {
		r.errorf("/worktree discard: %v\n", err)
		return
	}
	if plan.WorktreePath == "" {
		r.info(fmt.Sprintf("/worktree discard: plan %s has no preserved worktree", plan.ID))
		return
	}
	// Determine mainRoot from the REPL context; DiscardByPath tolerates
	// empty mainRoot but misses the git-side cleanup step, so pass the
	// best value we have.
	mainRoot := r.repoRoot
	if err := worktree.DiscardByPath(plan.WorktreePath, mainRoot); err != nil {
		// Non-fatal: filesystem may already be gone. Warn + continue
		// so we still clear the disk field.
		logging.Warning("[repl] /worktree discard: %v", err)
	}
	// Clear the persisted WorktreePath. We do this by reading the
	// plan, zeroing the field, and re-saving — PlanStore.Save
	// preserves the ID-named filename.
	// Save is idempotent-overwrite, keyed on plan.ID, so re-saving
	// the mutated struct replaces the on-disk bytes cleanly.
	plan.WorktreePath = ""
	if _, err := r.planStore.Save(plan); err != nil {
		r.errorf("/worktree discard: plan-file update failed: %v\n", err)
		return
	}
	r.success(fmt.Sprintf("worktree discarded for plan %s", plan.ID))
}
