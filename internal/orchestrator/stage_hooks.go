package orchestrator

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// stage_hooks.go implements per-stage pre/post hooks the write-mode
// scheduler runs around each dispatchStage call. The hooks own the
// "stage-edge" work that isn't part of the agent's ReAct loop:
// worktree provisioning, baseline capture, plan-status persistence,
// disk side-effects on the report.
//
// Read-mode stages have no hooks because the read scheduler keeps
// its existing inline logic (Tier1Floor, contract retry, etc.).
// Hooks are write-mode only — read mode looks them up in
// writeStageHooks and gets the empty Hooks zero value, which
// short-circuits to no-op.

// stageHookFunc is the per-hook signature. Pre hooks return an error
// that terminates the stage dispatch (e.g. worktree provision
// failure). Post hooks are best-effort: errors log a warning but do
// not fail the stage — the hook's job is to record state, not gate
// the dispatch.
type stageHookFunc func(o *Orchestrator) error

// stageHooks bundles a stage's pre + post hooks. Either may be nil.
type stageHooks struct {
	Pre  stageHookFunc
	Post stageHookPostFunc
}

// stageHookPostFunc gets the StageOutput so the hook can branch on
// success vs. error (e.g. apply post hook persists PlanStatusApplyFailed
// when StageOutput.Error is set; success leaves status untouched
// because verify will flip it).
type stageHookPostFunc func(o *Orchestrator, out *agent.StageOutput) error

// writeStageHooks is the static lookup table the write scheduler
// consults before/after each dispatchStage call. Entries without a
// hook (e.g. StagePlan post-hook is just a Result render) keep the
// nil pointer; the scheduler skips nil hooks.
var writeStageHooks = map[types.PipelineStage]stageHooks{
	types.StagePlan: {
		Post: planPostHook,
	},
	types.StageApply: {
		Pre:  applyPreHook,
		Post: applyPostHook,
	},
	types.StageVerify: {
		Pre:  verifyPreHook,
		Post: verifyPostHook,
	},
}

// runStageHooks fires the pre hook before the dispatch + the post
// hook after, with sensible nil-skipping. Returns the pre-hook
// error verbatim so the dispatch loop can surface it as a stage
// failure (the post hook never runs in that case).
func runStagePreHook(o *Orchestrator, stage types.PipelineStage) error {
	h := writeStageHooks[stage].Pre
	if h == nil {
		return nil
	}
	return h(o)
}

func runStagePostHook(o *Orchestrator, stage types.PipelineStage, out *agent.StageOutput) {
	h := writeStageHooks[stage].Post
	if h == nil {
		return
	}
	if err := h(o, out); err != nil {
		logging.Warning("[orchestrator] %s post-hook: %v", stage, err)
	}
}

// planPostHook fires after the planner returns. The plan is on
// Mutable.ChangePlan via emit_change_plan; this hook reads it back
// and renders the human-visible Result summary that ModePlan Runs
// terminate with.
//
// Idempotent PendingApplies refresh: emit_change_plan.Execute
// normally populates WriteClosure.PendingApplies as part of the tool
// call, but a test stub or alternative emission path may install the
// ChangePlan without touching the closure. To make the plan node's
// CritPlanReady SuccessCriteria robust to that, the hook seeds
// PendingApplies from plan.Changes when the queue is empty. Real
// LLM-driven runs see the queue already populated so the loop is a
// no-op.
func planPostHook(o *Orchestrator, out *agent.StageOutput) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		// Planner ran but no plan emitted — surface a fail-loud message.
		msg := "plan stage completed but no ChangePlan was installed on Mutable (planner did not call emit_change_plan)"
		o.busCtx.Mutable.SetResult(msg)
		return fmt.Errorf("%s", msg)
	}
	wc := o.busCtx.Mutable.WriteClosure()
	if len(wc.PendingApplies()) == 0 {
		for _, c := range plan.Changes {
			wc.EnqueuePendingApply(types.PendingApply{
				Path:      c.Path,
				Rationale: c.Rationale,
				Origin:    "plan_post_hook",
			})
		}
	}
	o.busCtx.Mutable.SetResult(renderChangePlanSummary(plan))
	logging.Info("[orchestrator] plan stage: id=%s changes=%d", plan.ID, len(plan.Changes))
	return nil
}

// applyPreHook prepares the worktree before the coder dispatches.
// Three sub-steps:
//
//  1. Load ChangePlan from disk when PlanPath is set and Mutable
//     hasn't already received one (the post-plan-stage path leaves
//     Mutable.ChangePlan populated; the --plan-file path doesn't).
//  2. Provision a fresh worktree and swap busCtx.RepoRoot to it.
//     Idempotent: if WorktreePath is already set (mid-retry second
//     apply round), reuses the existing worktree's path.
//  3. Optionally capture the pre-apply baseline test snapshot.
func applyPreHook(o *Orchestrator) error {
	if o == nil || o.busCtx == nil {
		return fmt.Errorf("apply pre-hook: nil orchestrator/bus")
	}
	// 1. Plan loaded?
	if o.busCtx.Mutable.ChangePlan() == nil {
		if o.busCtx.PlanPath == "" {
			msg := "apply stage: Mutable.ChangePlan is nil and no --plan-file was supplied; cannot apply without a plan"
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		plan, err := types.LoadChangePlanFromFile(o.busCtx.PlanPath)
		if err != nil {
			msg := fmt.Sprintf("apply stage: load plan file failed: %v", err)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		o.busCtx.Mutable.SetChangePlan(plan)
		// Seed PendingApplies so CritPlanReady passes on the first
		// EntryConditions evaluation. Same shape the legacy
		// the apply stage hook used.
		wc := o.busCtx.Mutable.WriteClosure()
		for _, c := range plan.Changes {
			wc.EnqueuePendingApply(types.PendingApply{
				Path:      c.Path,
				Rationale: c.Rationale,
				Origin:    "load_from_file",
			})
		}
		logging.Info("[orchestrator] apply pre-hook: loaded plan %s from %s (%d changes)",
			plan.ID, o.busCtx.PlanPath, len(plan.Changes))
	}
	// 2. Worktree provisioning.
	if o.worktreeBase == "" {
		msg := "apply stage: worktree base directory not configured (orchestrator.SetWorktreeBase was not called)"
		o.busCtx.Mutable.SetResult(msg)
		return fmt.Errorf("%s", msg)
	}
	if o.busCtx.WorktreePath == "" {
		sess, err := worktree.Create(o.worktreeBase, o.busCtx.MainRepoRoot, o.busCtx.TraceID)
		if err != nil {
			msg := fmt.Sprintf("apply stage: worktree provisioning failed: %v", err)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		o.busCtx.WorktreePath = sess.Path()
		o.busCtx.RepoRoot = sess.Path()
		o.busCtx.Mutable.SetRepoRoot(sess.Path())
		logging.Info("[orchestrator] apply pre-hook: worktree at %s", sess.Path())
	}
	// 3. Baseline capture (opt-in).
	if o.baselineCaptureEnabled && o.busCtx.Mutable.BaselineReport() == nil {
		o.captureBaseline()
	}
	return nil
}

// applyPostHook runs after the coder returns. On dispatch error,
// persists PlanStatusApplyFailed and surfaces the error so the
// scheduler stops the cycle. On success, renders an apply-summary
// Result. Status persistence on success is deferred to verify's
// post-hook (verify_failed vs applied) so the disk file isn't
// double-written on every Run.
func applyPostHook(o *Orchestrator, out *agent.StageOutput) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	if out != nil && out.Error != "" {
		o.busCtx.Mutable.SetResult(out.Error)
		o.persistPlanStatus(types.PlanStatusApplyFailed, nil)
		return nil
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		return nil
	}
	applied := o.busCtx.Mutable.WriteClosure().AppliedSet()
	o.busCtx.Mutable.SetResult(renderApplySummary(plan, applied, o.busCtx.WorktreePath))
	logging.Info("[orchestrator] apply stage: completed, %d/%d changes applied",
		len(applied), len(plan.TargetPaths))
	return nil
}

// verifyPreHook prepares the verify stage. Two responsibilities:
//
//  1. Load the ChangePlan from disk when ModeVerify runs standalone
//     (`--mode=verify --plan-file=X` or REPL `/verify <id>`). In
//     ModeApply the plan is already on Mutable from the apply pre-
//     hook or the planner stage; only standalone verify needs to
//     hydrate it. Skipped when ChangePlan is already set.
//
//  2. Swap busCtx.RepoRoot to a preserved worktree path when
//     orchestrator.SetReuseWorktreePath was called (REPL `/verify
//     <plan-id>` against a worktree preserved by
//     pipeline_keep_worktree_on_success). Without the swap, run_tests
//     would execute against the main repo HEAD instead of the
//     applied-bytes worktree — defeating the re-verify intent.
//
// The reused path is NOT written to busCtx.WorktreePath so the outer
// Run() cleanup defer leaves the preserved tree alone.
func verifyPreHook(o *Orchestrator) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	if o.busCtx.Mutable.ChangePlan() == nil && o.busCtx.PlanPath != "" {
		plan, err := types.LoadChangePlanFromFile(o.busCtx.PlanPath)
		if err != nil {
			msg := fmt.Sprintf("verify stage: load plan file failed: %v", err)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		o.busCtx.Mutable.SetChangePlan(plan)
		logging.Info("[orchestrator] verify pre-hook: loaded plan %s from %s", plan.ID, o.busCtx.PlanPath)
		// CLI auto-pickup: when the loaded plan has a WorktreePath
		// recorded (preserved via pipeline_keep_worktree_on_success
		// during the original apply), and the orchestrator wasn't
		// explicitly told to reuse a different path, use the plan's
		// own recorded worktree. This makes `--mode=verify
		// --plan-file=X` work the same way as REPL `/verify <id>`
		// without forcing the CLI user to discover the worktree
		// path manually.
		if o.reuseWorktreePath == "" && plan.WorktreePath != "" {
			o.reuseWorktreePath = plan.WorktreePath
			logging.Info("[orchestrator] verify pre-hook: auto-reusing plan's recorded worktree %s", plan.WorktreePath)
		}
	}
	// Reuse-worktree swap. Stat the path to fail-loud when the user
	// supplies a stale plan whose preserved worktree was already
	// cleaned up by the next-startup reap.
	if o.reuseWorktreePath != "" {
		info, err := os.Stat(o.reuseWorktreePath)
		if err != nil || !info.IsDir() {
			msg := fmt.Sprintf("verify stage: preserved worktree %q missing or not a directory (was it discarded?)", o.reuseWorktreePath)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		o.busCtx.RepoRoot = o.reuseWorktreePath
		o.busCtx.Mutable.SetRepoRoot(o.reuseWorktreePath)
		logging.Info("[orchestrator] verify pre-hook: swapped RepoRoot to preserved worktree %s", o.reuseWorktreePath)
	}
	return nil
}

// verifyPostHook persists the ChangeReport to disk + flips the plan
// status. On verify pass: PlanStatusApplied + AppliedAt timestamp.
// On verify fail: PlanStatusVerifyFailed (status field only). Result
// rendering uses renderVerifySuccess (appends to apply summary) for
// pass and renderVerifyFailure for fail.
func verifyPostHook(o *Orchestrator, out *agent.StageOutput) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	report := o.busCtx.Mutable.ChangeReport()
	if report != nil {
		o.saveChangeReport(report)
	}
	if out != nil && out.Error != "" {
		o.busCtx.Mutable.SetResult(renderVerifyFailure(report, out.Error))
		o.persistPlanStatus(types.PlanStatusVerifyFailed, nil)
		return nil
	}
	existing := o.busCtx.Mutable.Result()
	o.busCtx.Mutable.SetResult(existing + renderVerifySuccess(report))
	now := time.Now()
	o.persistPlanStatus(types.PlanStatusApplied, &now)
	return nil
}

// clearForReplan resets write-mode Mutable state between verify→plan
// retry iterations and seeds Mutable.PlanningHint with the previous
// failure narrative so the planner's next dispatch incorporates the
// retry rationale.
//
// Called by the write scheduler when verify SuccessCriteria fails
// AND retry budget remains. Six jobs:
//
//  1. Build a PlanningHint from the previous ChangeReport + plan.
//     Read BEFORE the resets below clear them.
//  2. Discard the worktree (if one was provisioned) and restore
//     RepoRoot to MainRepoRoot. The next applyPreHook will create
//     a fresh worktree.
//  3. Reset ChangePlan + ChangeReport + WriteClosure.
//  4. Clear o.planPath so a user-supplied --plan-file doesn't
//     override the regenerated plan.
//  5. Install the PlanningHint on Mutable so plannerEvaluator
//     picks it up via consume-once.
func clearForReplan(o *Orchestrator, attempt int) {
	if o == nil || o.busCtx == nil {
		return
	}
	prevReport := o.busCtx.Mutable.ChangeReport()
	prevPlan := o.busCtx.Mutable.ChangePlan()
	heuristicHint := buildRetryHint(prevReport, prevPlan, attempt)

	// Reflexion-pattern critic (optional). When configured, dispatch
	// one side LLM call to interpret the failure as a critique
	// paragraph; planner sees critique + heuristic facts together.
	// Any reflector failure path silently degrades to heuristic-only
	// — retry MUST NOT be blocked by reflection plumbing.
	hint := heuristicHint
	if o.reflector != nil {
		input := buildReflectorInput(o.busCtx, prevReport, prevPlan, attempt)
		critique, err := o.reflector.Reflect(input)
		if err != nil {
			logging.Warning("[orchestrator] reflector failed (degrading to heuristic hint): %v", err)
		} else if strings.TrimSpace(critique) != "" {
			hint = critique + "\n\n" + heuristicHint
		}
	}
	if o.busCtx.WorktreePath != "" {
		if err := worktree.DiscardByPath(o.busCtx.WorktreePath, o.busCtx.MainRepoRoot); err != nil {
			logging.Warning("[orchestrator] retry worktree discard failed: %v", err)
		}
		o.busCtx.WorktreePath = ""
	}
	if o.busCtx.MainRepoRoot != "" {
		o.busCtx.RepoRoot = o.busCtx.MainRepoRoot
		o.busCtx.Mutable.SetRepoRoot(o.busCtx.MainRepoRoot)
	}
	o.busCtx.Mutable.ResetChangePlan()
	o.busCtx.Mutable.ResetChangeReport()
	o.busCtx.Mutable.WriteClosure().Reset()
	o.planPath = ""
	o.busCtx.PlanPath = ""
	o.busCtx.Mutable.SetPlanningHint(hint)
	logging.Info("[orchestrator] verify→plan retry attempt %d: hint=%q", attempt, hint)
}

// buildReflectorInput translates the failed iteration's structured
// state into a ReflectorInput. Mirrors buildRetryHint's data
// gathering so the heuristic and the critic see the same facts;
// reflector formats them as natural-language prompt while
// buildRetryHint formats them as terse "Failing tests:" bullet list.
//
// Caps: top 3 failing tests; first non-empty line of each
// FailureDetail (cap 200 chars). Inputs to the critic LLM are
// intentionally small — see Self-Debug 2023 for why concise summary
// beats raw trace.
func buildReflectorInput(busCtx *types.BusContext, report *types.ChangeReport, plan *types.ChangePlan, attempt int) ReflectorInput {
	in := ReflectorInput{Attempt: attempt}
	if busCtx != nil && busCtx.Mutable != nil {
		in.OriginalRequest = strings.TrimSpace(busCtx.Mutable.Objective())
	}
	if plan != nil {
		in.PlanSummary = plan.Summary
		in.TargetPaths = append([]string(nil), plan.TargetPaths...)
		in.AcceptanceTests = append([]string(nil), plan.AcceptanceTests...)
	}
	if report != nil {
		in.FailureSummary = report.FailureSummary
		in.BuildFailed = report.BuildFailed
		const maxFailing = 3
		shown := 0
		for _, tr := range report.TestResults {
			if tr.Passed {
				continue
			}
			detail := ""
			if tr.FailureDetail != "" {
				detail = strings.TrimSpace(strings.SplitN(tr.FailureDetail, "\n", 2)[0])
			}
			in.FailingTests = append(in.FailingTests, ReflectorFailedTest{
				Suite:       tr.Suite,
				AssertionID: tr.AssertionID,
				Detail:      detail,
			})
			shown++
			if shown >= maxFailing {
				break
			}
		}
	}
	return in
}
