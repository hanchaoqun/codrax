package orchestrator

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// runPhaseGroup is stage II's multi-phase outer loop. Replaces
// the single call to runTaskPhase when WriteAnalysisIR carries
// a sequential phase proposal AND the orchestrator has a
// PlanGroupStore wired. Single-phase / no-IR cases skip this
// and use the existing single-phase path (back-compat — that
// codepath is byte-identical to pre-stage-II).
//
// Per-phase iteration:
//
//  1. Pick the next non-terminal phase (group.ActiveIdx). Skip
//     phases already at terminal status (Accepted / RolledBack /
//     Skipped) — operator-driven /phase skip lets you step over
//     a phase without rerunning it.
//
//  2. Reset per-phase Mutable slots so prior-phase state doesn't
//     leak (ChangePlan, ChangeReport, BestPlanReport).
//     AppliedSet stays — phases ACCUMULATE on the same worktree.
//     WriteAnalysisIR stays — single task framing across all
//     phases.
//
//  3. Seed Mutable.PlanningHint with phase-specific context
//     (goal + rough_target_paths). The planner reads this via
//     consume-once.
//
//  4. Build a fresh single-phase TaskGraph (3 nodes:
//     plan→apply→verify) and run the existing scheduler. Each
//     phase gets the full retryBudget (per-phase, not shared
//     across phases — failing phase 1 doesn't drain phase 2's
//     retry slots).
//
//  5. After verify passes, dispatch acceptance_check (one LLM
//     call). On accept → mark Phase as Accepted, advance.
//     On reject → mark PhaseRolledBack + group failed.
//
//  6. Persist group at every state transition.
//
// Failure handling: any phase's verify→plan retry budget is
// independent. When a phase exhausts retries, group transitions
// to PlanGroupFailed and the loop exits without entering
// subsequent phases.
//
// Worktree handling: a single worktree carries through all
// phases (cumulative effect — phase 2's planner sees the files
// phase 1 created). The cleanup defer in Run() honours
// keep_worktree_on_success only after group reaches Completed
// status.
func (o *Orchestrator) runPhaseGroup(group *types.PlanGroup, stepsUsed *int) error {
	if group == nil {
		return fmt.Errorf("runPhaseGroup: nil group")
	}
	if len(group.Phases) == 0 {
		return fmt.Errorf("runPhaseGroup: group has zero phases")
	}

	// Persist initial state so a process crash mid-group can
	// be inspected via /phase show even before any phase has
	// dispatched.
	o.persistGroup(group)

	// UX (commit 43): emit a structured block once at group
	// entry so the dock renders the full phase enumeration
	// — visually parallel to the analyzer's sub-topic block.
	// Pre-commit-43 the operator only saw per-phase Reasoning
	// events with the LLM-thinking marker; now they see
	// the workflow shape upfront.
	if o.emit != nil {
		phaseList := make([]render.PhaseInfo, 0, len(group.Phases))
		for _, p := range group.Phases {
			phaseList = append(phaseList, render.PhaseInfo{
				Index: p.Index,
				Goal:  oneLineClampPhase(p.Goal, 100),
			})
		}
		o.emit(render.Event{
			Kind:      render.EventPhaseGroupStart,
			Timestamp: time.Now(),
			Agent:     "orchestrator",
			PhaseList: phaseList,
		})
	}

	for group.ActiveIdx < len(group.Phases) {
		// Cancel check at every iteration boundary (commit 32).
		// Before this, /cancel + Ctrl+C only landed when the
		// in-flight LLM Chat hit a cancelled context — phase
		// 1's verify might unwind cleanly, but the outer loop
		// would happily march into phase 2 because nothing
		// re-checked the cancel token. Mirror of the same
		// gate runWriteSchedulerLoop installs at line 74.
		if cerr := o.checkCanceled("phase_scheduler", *stepsUsed); cerr != nil {
			return cerr
		}

		phase := &group.Phases[group.ActiveIdx]

		// Skip phases already at terminal status (operator
		// /phase skip / earlier rolled-back state).
		if types.IsTerminalPhaseStatus(phase.Status) {
			group.ActiveIdx++
			o.persistGroup(group)
			continue
		}

		now := time.Now()
		phase.Status = types.PhaseInProgress
		phase.StartedAt = &now
		// Stamp the owning process so FindActiveGroup's orphan
		// reaper can detect a stale in_progress phase if this
		// process crashes mid-phase. Cleared at acceptance time
		// (FinishedAt is non-nil) so completed phases don't
		// trigger reaper noise.
		phase.OwnerPID = os.Getpid()
		o.persistGroup(group)

		// Per-phase progress event (commit 43). Replaces the
		// pre-commit-43 EventAgentReasoning shape (which used
		// the LLM-thinking marker — semantically wrong for
		// structural progression). Dock now renders this with
		// ▶ icon + statusObjective color, parallel to the
		// sub-topic block style.
		if o.emit != nil {
			o.emit(render.Event{
				Kind:              render.EventPhaseProgress,
				Timestamp:         time.Now(),
				Agent:             "orchestrator",
				PhaseIndex:        phase.Index,
				PhaseTotal:        len(group.Phases),
				PhaseProgressKind: render.PhaseProgressStart,
				PhaseDetail:       oneLineClampPhase(phase.Goal, 80),
			})
		}

		// Seed planning context for THIS phase. Each phase has
		// its own goal + rough target paths from the LLM's
		// PhaseProposal. The hint is consume-once.
		o.seedPlanningHintFromPhase(phase, group)

		// Reset per-phase Mutable so prior phase's plan/report
		// don't leak into this one's planner prompt.
		o.resetForNextPhase()

		// Build a single-phase 3-node TaskGraph for this phase.
		// retryBudget per-phase: each phase gets the full budget
		// independently (failing phase 1 doesn't drain phase 2's
		// retries).
		phaseGraph := BuildWriteTaskGraph(o.busCtx.Mode, "", o.writeRetryBudget)
		o.busCtx.AnalysisIR.TaskGraph = phaseGraph
		o.busCtx.Mutable.ResetBestPlanReport()

		// Drive the existing single-phase scheduler. Same code
		// path single-phase mode uses; the only difference is
		// this is invoked PER PHASE inside a loop. Tests can
		// override o.runTaskPhaseFn to substitute a stub that
		// pre-seeds Mutable.ChangePlan + ChangeReport without
		// dispatching real agents — the runPhaseGroup outer
		// loop is otherwise unreachable from a unit test
		// because runTaskPhase requires the full ReAct stack.
		runner := o.runTaskPhaseFn
		if runner == nil {
			runner = o.runTaskPhase
		}
		taskErr := runner(stepsUsed)

		// Inspect outcome. The plan emitted by THIS phase, the
		// commit SHA reached, the verify report.
		plan := o.busCtx.Mutable.ChangePlan()
		report := o.busCtx.Mutable.ChangeReport()
		// Record retry count from the per-phase iteration
		// ledger (commit 32). The ledger was reset at phase
		// entry by resetForNextPhase; each clearForReplan
		// invocation appends one row; len(ledger) is the
		// number of verify→plan retries this phase consumed
		// before reaching its terminal status.
		phase.RetryAttempts = len(o.busCtx.Mutable.IterationLedger())
		// Stamp phase coordinates onto the ChangeReport so
		// /history can cluster consecutive reports by group
		// instead of treating them as 5 unrelated tasks
		// (commit 32 schema gap). report may be nil when
		// runTaskPhase short-circuited before verify produced
		// one — leave nil reports alone.
		if report != nil {
			report.PhaseGroupID = group.ID
			report.PhaseIndex = phase.Index
		}
		if plan != nil {
			phase.PlanID = plan.ID
			plan.PhaseGroupID = group.ID
			plan.PhaseIndex = phase.Index
			// Persist the cross-link if the plan has a path on
			// disk. The PlanStore.Save in the REPL post-Run path
			// would otherwise overwrite without these fields.
			if o.busCtx.PlanPath != "" {
				if werr := types.WritePlanToFile(plan, o.busCtx.PlanPath); werr != nil {
					logging.Warning("[orchestrator] phase %d plan persist (with phase link) failed: %v",
						phase.Index, werr)
				}
			}
			// Persist into PlanStore so /plan show / /history /
			// /approve <id> can find this phase's plan. The
			// REPL's post-Run auto-save fires only on ModePlan;
			// stage II runs in ModeApply, so without this hop
			// phase plans never reach disk.
			if o.planSaver != nil {
				if _, werr := o.planSaver.Save(plan); werr != nil {
					logging.Warning("[orchestrator] phase %d PlanStore.Save failed: %v",
						phase.Index, werr)
				}
			}
		}
		if o.currentIterCommitSHA != "" {
			phase.AppliedSHA = o.currentIterCommitSHA
		}

		if taskErr != nil {
			// Phase failed (retries exhausted or apply rejected).
			finished := time.Now()
			phase.Status = types.PhaseRolledBack
			phase.FinishedAt = &finished
			group.Status = types.PlanGroupFailed
			o.persistGroup(group)
			return fmt.Errorf("phase %d failed: %w", phase.Index, taskErr)
		}

		phase.Status = types.PhaseVerified
		evaluation := o.evaluateWritePhaseWorkflow(group, phase, plan, report)
		phase.WorkflowEvaluation = snapshotWriteWorkflowEvaluation(evaluation)
		logging.Info("[orchestrator] write workflow evaluation: group=%s phase=%d status=%s reason=%s next_batch=%s risk=%s approval=%s",
			group.ID, phase.Index, evaluation.Status, evaluation.ReasonCode, evaluation.NextBatchID, evaluation.RiskLevel, evaluation.ApprovalAction)
		if evaluation.Status == writeflow.EvalDangerDenied {
			finished := time.Now()
			phase.Status = types.PhaseRolledBack
			phase.FinishedAt = &finished
			group.Status = types.PlanGroupFailed
			o.persistGroup(group)
			return fmt.Errorf("phase %d denied by write workflow evaluator: %s", phase.Index, evaluation.ReasonCode)
		}
		o.persistGroup(group)

		// Acceptance check (commit 19). Independent LLM verdict
		// on whether the phase satisfied its goal. Outcomes
		// classified by applyAcceptanceVerdict — see that helper
		// for the three-state translation (rejected / unverified /
		// accepted).
		if o.acceptanceChecker != nil {
			// UX (commit 44): emit start/end events around the
			// synchronous Check dispatch so dock row 1 flips to
			// "验收审查中" / "acceptance review" instead of
			// freezing at the prior verify's "请求模型中" for
			// the 5-30s the LLM call takes. Without these the
			// status row was indistinguishable from a stalled
			// tool call.
			if o.emit != nil {
				o.emit(render.Event{
					Kind:       render.EventAcceptanceReviewStart,
					Timestamp:  time.Now(),
					Agent:      "orchestrator",
					PhaseIndex: phase.Index,
					PhaseTotal: len(group.Phases),
				})
			}
			ac, err := o.acceptanceChecker.Check(o.CancelContext(), AcceptanceCheckInput{
				PhaseGoal:   phase.Goal,
				PlanSummary: planSummaryForAcceptance(plan),
				TestReport:  report,
				Outcomes:    expectedOutcomesForAcceptance(o.busCtx),
				Attempt:     1, // per-phase retry counter is internal to runTaskPhase
				PhaseIndex:  phase.Index + 1,
				PhaseTotal:  len(group.Phases),
			})
			if o.emit != nil {
				o.emit(render.Event{
					Kind:      render.EventAcceptanceReviewEnd,
					Timestamp: time.Now(),
					Agent:     "orchestrator",
				})
			}
			if rejected, rejErr := o.applyAcceptanceVerdict(phase, group, ac, err); rejected {
				return rejErr
			}
		} else {
			// No checker wired: simple advance.
			finished := time.Now()
			phase.Status = types.PhaseAccepted
			phase.FinishedAt = &finished
		}
		group.Status = types.PlanGroupInFlight
		group.ActiveIdx++
		o.persistGroup(group)
	}

	group.Status = types.PlanGroupCompleted
	o.persistGroup(group)
	return nil
}

func (o *Orchestrator) evaluateWritePhaseWorkflow(group *types.PlanGroup, phase *types.PhaseRecord, plan *types.ChangePlan, report *types.ChangeReport) writeflow.WriteEvaluation {
	var workflow writeflow.WriteWorkflowPlan
	if o != nil && o.busCtx != nil && o.busCtx.Mutable != nil {
		workflow = writeflow.WorkflowSeedFromWriteAnalysis(o.busCtx.Mutable.WriteAnalysisIR())
	}
	workflow = writeflow.NormalizeWorkflowPlan(workflow)
	if workflow.Goal == "" && group != nil {
		workflow.Goal = group.Goal
	}
	workflow.Status = writeflow.WorkflowInProgress
	if group != nil {
		workflow.MaxBatches = len(group.Phases)
		for _, p := range group.Phases {
			if p.Index == phase.Index {
				continue
			}
			if p.Status == types.PhaseAccepted || p.Status == types.PhaseAcceptanceUnverified {
				workflow.CompletedBatches = append(workflow.CompletedBatches, fmt.Sprintf("batch-%d", p.Index+1))
			}
		}
		nextIdx := phase.Index + 1
		if nextIdx < len(group.Phases) {
			nextPhase := group.Phases[nextIdx]
			workflow.NextBatch = &writeflow.WriteBatchPlan{
				ID:                   fmt.Sprintf("batch-%d", nextPhase.Index+1),
				Goal:                 nextPhase.Goal,
				Status:               writeflow.BatchNeedsExploration,
				NeedsCodeExploration: true,
				ExploreTargets:       append([]string(nil), nextPhase.RoughTargetPaths...),
				ExpectedPaths:        append([]string(nil), nextPhase.RoughTargetPaths...),
				SuccessCriteria:      append([]string(nil), workflow.SuccessCriteria...),
			}
		} else {
			workflow.NextBatch = nil
		}
	}
	batch := phaseToWriteBatchPlan(phase, workflow.SuccessCriteria)
	assessment := writeflow.AssessWriteRisk(writeflow.AssessmentInput{Plan: plan})
	policy := writeflow.ApprovalPolicyAutoSafe
	if o != nil && o.writeApprovalPolicy != "" {
		policy = writeflow.NormalizeApprovalPolicy(o.writeApprovalPolicy)
	}
	decision := writeflow.DecideWriteApproval(policy, assessment)
	batchCount := 0
	if phase != nil {
		batchCount = phase.Index
	}
	maxBatches := workflow.MaxBatches
	if maxBatches == 0 && group != nil {
		maxBatches = len(group.Phases)
	}
	return writeflow.EvaluateWriteWorkflow(writeflow.EvaluationInput{
		Workflow:         workflow,
		Batch:            &batch,
		Plan:             plan,
		Report:           report,
		RiskAssessment:   assessment,
		ApprovalDecision: decision,
		BatchCount:       batchCount,
		MaxBatches:       maxBatches,
	})
}

func phaseToWriteBatchPlan(phase *types.PhaseRecord, criteria []string) writeflow.WriteBatchPlan {
	if phase == nil {
		return writeflow.WriteBatchPlan{Status: writeflow.BatchPending}
	}
	status := writeflow.BatchPending
	switch phase.Status {
	case types.PhaseInProgress:
		status = writeflow.BatchReadyForChangePlan
	case types.PhaseApplied:
		status = writeflow.BatchApplied
	case types.PhaseVerified, types.PhaseAccepted, types.PhaseAcceptanceUnverified:
		status = writeflow.BatchComplete
	case types.PhaseRolledBack:
		status = writeflow.BatchNeedsRepair
	case types.PhaseSkipped:
		status = writeflow.BatchBlocked
	}
	return writeflow.NormalizeBatchPlan(writeflow.WriteBatchPlan{
		ID:                   fmt.Sprintf("batch-%d", phase.Index+1),
		Goal:                 phase.Goal,
		Status:               status,
		NeedsCodeExploration: status == writeflow.BatchPending || status == writeflow.BatchReadyForChangePlan,
		ExploreTargets:       append([]string(nil), phase.RoughTargetPaths...),
		ExpectedPaths:        append([]string(nil), phase.RoughTargetPaths...),
		SuccessCriteria:      append([]string(nil), criteria...),
	})
}

func snapshotWriteWorkflowEvaluation(e writeflow.WriteEvaluation) *types.WriteWorkflowEvaluationSnapshot {
	return &types.WriteWorkflowEvaluationSnapshot{
		Status:         string(e.Status),
		ReasonCode:     e.ReasonCode,
		Reason:         e.Reason,
		NextBatchID:    e.NextBatchID,
		RiskLevel:      string(e.RiskLevel),
		ApprovalAction: string(e.ApprovalAction),
	}
}

// applyAcceptanceVerdict translates an AcceptanceChecker.Check
// result into the per-phase status mutation. Three outcomes:
//
//  1. err != nil — phase marked PhaseAcceptanceUnverified
//     (commit 26: was fail-open auto-accept; now visibly
//     unverified so /phase show surfaces the gap). Returns
//     (false, nil) so the caller advances ActiveIdx — the
//     scheduler still advances so infra failures don't
//     deadlock the entire multi-phase Run.
//
//  2. ac != nil && !ac.Passed — phase RolledBack, group
//     Failed. Returns (true, error) so the caller exits
//     runPhaseGroup with the rejection error; the group
//     stops here.
//
//  3. ac != nil && ac.Passed (the happy path) — phase
//     Accepted, NextHint propagated to o.nextPhaseHint.
//     Returns (false, nil); caller advances.
//
// Extracted from runPhaseGroup for unit-testability: the
// scheduler-side outer loop calls runTaskPhase which depends
// on the full agent stack, so testing only the verdict-
// translation logic in isolation is the cleanest way to pin
// the new PhaseAcceptanceUnverified path without standing up a
// fake agent fleet.
func (o *Orchestrator) applyAcceptanceVerdict(phase *types.PhaseRecord, group *types.PlanGroup, ac *types.AcceptanceCheck, err error) (rejected bool, rejErr error) {
	if err != nil {
		logging.Warning("[orchestrator] phase %d acceptance_check errored: %v (marking acceptance_unverified)",
			phase.Index, err)
		finished := time.Now()
		phase.Status = types.PhaseAcceptanceUnverified
		phase.FinishedAt = &finished
		// Stash the LLM error on AcceptanceCheck.Reasoning so
		// /phase show renders something operator-actionable
		// instead of just the bare status string.
		phase.AcceptanceCheck = &types.AcceptanceCheck{
			Passed:    false,
			Reasoning: fmt.Sprintf("acceptance_check infra failure: %v", err),
		}
		return false, nil
	}
	if ac == nil {
		// Defensive: nil checker output with no error — treat
		// as accepted since there's nothing actionable to act
		// on. This branch is unreachable in production
		// (Check returns either a verdict or an error) but the
		// guard keeps the helper total.
		finished := time.Now()
		phase.Status = types.PhaseAccepted
		phase.FinishedAt = &finished
		return false, nil
	}
	phase.AcceptanceCheck = ac
	if !ac.Passed {
		finished := time.Now()
		phase.Status = types.PhaseRolledBack
		phase.FinishedAt = &finished
		group.Status = types.PlanGroupFailed
		o.persistGroup(group)
		// UX (commit 43): emit a typed PhaseProgress event so
		// the dock renders ✗ + statusObjective color (not the
		// thinking marker used by the legacy Reasoning path).
		// Detail carries the reviewer's reasoning verbatim.
		if o.emit != nil {
			o.emit(render.Event{
				Kind:              render.EventPhaseProgress,
				Timestamp:         time.Now(),
				Agent:             "orchestrator",
				PhaseIndex:        phase.Index,
				PhaseTotal:        len(group.Phases),
				PhaseProgressKind: render.PhaseProgressRejected,
				PhaseDetail:       oneLineClampPhase(ac.Reasoning, 200),
			})
		}
		return true, fmt.Errorf("phase %d acceptance rejected: %s",
			phase.Index, ac.Reasoning)
	}
	// Carry the next-phase hint forward. The next
	// iteration's seedPlanningHintFromPhase reads
	// o.nextPhaseHint and consumes it.
	if strings.TrimSpace(ac.NextHint) != "" {
		o.nextPhaseHint = ac.NextHint
	}
	finished := time.Now()
	phase.Status = types.PhaseAccepted
	phase.FinishedAt = &finished
	// UX (commit 43): typed accept event — dock renders ✓ icon
	// + statusObjective color, parallel to the sub-topic
	// block style.
	if o.emit != nil {
		o.emit(render.Event{
			Kind:              render.EventPhaseProgress,
			Timestamp:         time.Now(),
			Agent:             "orchestrator",
			PhaseIndex:        phase.Index,
			PhaseTotal:        len(group.Phases),
			PhaseProgressKind: render.PhaseProgressAccepted,
		})
	}
	return false, nil
}

// oneLineClampPhase trims newlines + caps the phase goal to N
// runes for the progress event. The dock renderer wraps long
// reasoning strings; this just keeps single-line shape so the
// phase header doesn't visually outweigh the actual stage
// progression beneath.
func oneLineClampPhase(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if n > 0 {
		runes := []rune(s)
		if len(runes) > n {
			return string(runes[:n]) + "…"
		}
	}
	return s
}

// extractPhaseGoalFromPrefix peels the "## Phase X of Y: " header
// off seedPlanningHintFromPhase's output and returns the phase
// goal text. Used by dispatchStage(StagePlan) to bias the
// stage-3 pitfall ranking toward this phase's specific work
// (commit 40 P2). Empty input or non-matching shape returns
// empty so the caller can degrade silently.
func extractPhaseGoalFromPrefix(prefix string) string {
	if prefix == "" {
		return ""
	}
	const marker = "## Phase "
	idx := strings.Index(prefix, marker)
	if idx < 0 {
		return ""
	}
	tail := prefix[idx+len(marker):]
	// Skip past the "X of Y: " run by finding the first colon.
	colon := strings.Index(tail, ":")
	if colon < 0 {
		return ""
	}
	goal := strings.TrimSpace(tail[colon+1:])
	// Drop trailing newlines/whitespace; the seeded prefix
	// includes "\n\n" but the goal itself is the first line.
	if nl := strings.IndexByte(goal, '\n'); nl >= 0 {
		goal = strings.TrimSpace(goal[:nl])
	}
	return goal
}

// resetForNextPhase clears per-phase Mutable slots so the next
// phase starts fresh. AppliedSet stays — phases accumulate
// changes in the same worktree. WriteAnalysisIR stays — same
// task framing through all phases.
func (o *Orchestrator) resetForNextPhase() {
	mu := o.busCtx.Mutable
	if mu == nil {
		return
	}
	mu.ResetChangePlan()
	mu.ResetChangeReport()
	mu.ResetWriteExplorationRequest()
	mu.ResetWriteExplorationHandoff()
	// IterationLedger reset (commit 32) so the per-phase
	// retry-attempt counter we read at end of phase reflects
	// THIS phase's retries only — not phase 1 + 2 cumulative.
	// The ledger is consumed by clearForReplan as the side-LLM
	// reflector input; resetting between phases gives each
	// phase a clean slate for its own diagnostic critique.
	mu.ResetIterationLedger()
	// AppliedSet preserved (cumulative across phases).
	// WriteAnalysisIR preserved (single task, multiple phases).
	// WriteExplorationRequest/Handoff reset (per-batch planning context).
	// PlanCritique preserved on the previous phase's plan
	// struct; the next phase will produce its own when its
	// planner emits.
}

// seedPlanningHintFromPhase renders the phase-specific context
// onto Mutable.PlanningHint so the planner's
// BuildInitialInstruction picks it up. Includes:
//   - Phase number + goal (so the planner knows which subtask
//     it's on)
//   - Rough target paths the LLM suggested upfront
//   - Optional NextHint from the previous phase's acceptance
//     check (e.g. "users.email column was added; ORM still
//     references old name")
//
// Also pins the phase header onto o.phaseContextPrefix so an
// intra-phase verify→plan retry (clearForReplan) can
// re-prepend "you are still in phase X of Y" — PlanningHint
// itself is drained by the first planner dispatch and would
// otherwise vanish before the retry hint replaces it.
func (o *Orchestrator) seedPlanningHintFromPhase(phase *types.PhaseRecord, group *types.PlanGroup) {
	header := fmt.Sprintf("## Phase %d of %d: %s\n\n", phase.Index+1, len(group.Phases), phase.Goal)
	o.phaseContextPrefix = header

	var b strings.Builder
	b.WriteString(header)
	if len(phase.RoughTargetPaths) > 0 {
		b.WriteString("Rough target paths the analysis suggested for this phase:\n")
		for _, p := range phase.RoughTargetPaths {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString("\n")
	}
	if o.nextPhaseHint != "" {
		fmt.Fprintf(&b, "Carry-over from previous phase: %s\n\n", o.nextPhaseHint)
		o.nextPhaseHint = "" // consume-once
	}
	if o.busCtx.Mutable != nil {
		o.busCtx.Mutable.SetPlanningHint(b.String())
	}
}

// persistGroup writes the group's current state to disk via
// the configured PlanGroupStore. Failure is non-fatal —
// in-memory group is authoritative for the rest of the Run;
// disk copy is for crash recovery + REPL inspection.
func (o *Orchestrator) persistGroup(g *types.PlanGroup) {
	if o.planGroupStore == nil || g == nil {
		return
	}
	if _, err := o.planGroupStore.Save(g); err != nil {
		logging.Warning("[orchestrator] persist group %s: %v", g.ID, err)
	}
}

// buildPlanGroupFromProposal constructs a fresh PlanGroup from
// the WriteAnalysisIR's sequential PhaseProposal. Called once
// at Run() entry when isMultiPhaseRun returns true.
func (o *Orchestrator) buildPlanGroupFromProposal(ir *types.WriteAnalysisIR) *types.PlanGroup {
	now := time.Now()
	id := fmt.Sprintf("group-%d-%d", now.UnixNano(), os.Getpid())
	g := &types.PlanGroup{
		ID:        id,
		Goal:      ir.Request.RawRequest,
		CreatedAt: now,
		Status:    types.PlanGroupPlanning,
		Decision:  "linear",
		Phases:    make([]types.PhaseRecord, len(ir.PhaseProposal.Phases)),
	}
	for i, seed := range ir.PhaseProposal.Phases {
		g.Phases[i] = types.PhaseRecord{
			Index:            i,
			Goal:             strings.TrimSpace(seed.Goal),
			RoughTargetPaths: append([]string(nil), seed.RoughTargetPaths...),
			Status:           types.PhasePending,
		}
	}
	return g
}

// isMultiPhaseRun decides whether the current Run should drive
// runPhaseGroup or fall through to the existing single-phase
// path. Three preconditions:
//  1. WriteAnalysisIR is on Mutable (write_analyzer ran)
//  2. PhaseProposal.Split == "sequential" with ≥2 phases
//  3. PlanGroupStore wired (cmd/root.go's stage II flag)
//
// All three must hold. If any is false, falls through to single-
// phase byte-identical path.
func (o *Orchestrator) isMultiPhaseRun() bool {
	if o.planGroupStore == nil {
		return false
	}
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	ir := o.busCtx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return false
	}
	if ir.PhaseProposal.Split != "sequential" {
		return false
	}
	if len(ir.PhaseProposal.Phases) < 2 {
		return false
	}
	// ModePlan only emits the plan; multi-phase requires the
	// full plan→apply→verify per phase, so only ModeApply
	// drives multi-phase execution. ModeVerify against an
	// existing multi-phase group is a future stage's concern.
	if o.busCtx.Mode != types.ModeApply {
		return false
	}
	return true
}

// planSummaryForAcceptance extracts the plan's summary for the
// acceptance checker's input. Empty when plan is nil (defensive).
func planSummaryForAcceptance(plan *types.ChangePlan) string {
	if plan == nil {
		return ""
	}
	return plan.Summary
}

// expectedOutcomesForAcceptance pulls WriteAnalysisIR's expected
// outcomes onto the acceptance checker's input. Empty slice
// when no IR or no outcomes set.
func expectedOutcomesForAcceptance(busCtx *types.BusContext) []string {
	if busCtx == nil || busCtx.Mutable == nil {
		return nil
	}
	ir := busCtx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return nil
	}
	return append([]string(nil), ir.Request.ExpectedOutcomes...)
}
