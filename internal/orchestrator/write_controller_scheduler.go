package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/sourceowner"
	truthledger "github.com/hanchaoqun/codrax/internal/truth"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
	writeconvention "github.com/hanchaoqun/codrax/internal/writeflow/convention"
	writeimpact "github.com/hanchaoqun/codrax/internal/writeflow/impact"
)

const (
	defaultWriteWorkflowMaxBatches           = 5
	defaultWriteWorkflowMaxExplorationRounds = 2
	defaultWriteWorkflowMaxAskUserPerBatch   = 1
)

var errPlannerProbePassedExistingWorktree = errors.New("planner probe passed existing applied worktree")

type writeControllerApplyTransitionOutcome string

const (
	writeControllerApplyTransitionApplied  writeControllerApplyTransitionOutcome = "applied"
	writeControllerApplyTransitionRetry    writeControllerApplyTransitionOutcome = "retry"
	writeControllerApplyTransitionTerminal writeControllerApplyTransitionOutcome = "terminal"
)

// runWriteControllerWorkflow is the canonical outer DAG for write mode. The
// scheduler switches only on schema-validated controller actions; model prose is
// never interpreted as routing logic.
func (o *Orchestrator) runWriteControllerWorkflow(stepsUsed *int) error {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	if !o.busCtx.Mode.IsWrite() {
		return fmt.Errorf("write controller workflow requires write mode, got %q", o.busCtx.Mode)
	}
	run := o.loadOrSeedWriteWorkflowRun()
	o.persistWriteWorkflowRun(&run)
	explorationRounds := map[string]int{}
	verifyFailures := map[string]int{}
	verifyInfraFailures := map[string]int{}
	// Resume hydration: a run loaded from the store carries its history only
	// in typed attempt records. Rebuild the in-memory retry ledger, the
	// active plan, and the verify-failure carrier from those records and the
	// durable artifacts so a resumed run behaves like the run that paused —
	// the retry budget cannot silently reset and replan keeps its evidence.
	o.hydrateResumedWorkflowState(&run, verifyFailures)
	if msg := o.pendingApprovalResumeMessage(&run); msg != "" {
		appendControllerProgress(&run, run.ActiveBatchID, "pending_approval_resume_paused", msg)
		o.persistWriteWorkflowRun(&run)
		o.publishBlockedRunGuidance(&run, "pending_approval")
		return fmt.Errorf("%s", msg)
	}
	// importedPlanMirror is the --plan-file path this run was started
	// with (empty otherwise). The controller workflow may replace the
	// active plan across replan rounds; mirroring every accepted plan
	// (and its later status/worktree updates) back to this file keeps
	// the operator-visible artifact pointing at the LIVE plan instead
	// of the first imported snapshot.
	importedPlanMirror := strings.TrimSpace(o.busCtx.PlanPath)
	controllerTurns := 0
	maxTurns := defaultWriteWorkflowMaxBatches*4 + 4
	var lastInnerErr error

	for controllerTurns < maxTurns {
		if *stepsUsed >= o.maxSteps {
			// Completion lane: an applied-but-unverified batch gets its
			// verification BEFORE the budget verdict. Without this, a run
			// that spends its last steps applying a (possibly correct)
			// plan dies blocked, cleanup discards the worktree, and the
			// applied work never receives a verdict. The condition reads
			// typed attempt records only; one bounded verify dispatch is
			// allowed past the ceiling because completion outranks pacing.
			if o.runBudgetCompletionVerify(&run, stepsUsed, importedPlanMirror) {
				return nil
			}
			if o.completeBudgetExhaustedRunIfAllBatchesComplete(&run, "budget_all_batches_complete") {
				return nil
			}
			run.Status = types.WriteWorkflowRunBlocked
			appendControllerProgress(&run, run.ActiveBatchID, "budget_exhausted", "global step budget exhausted")
			o.persistWriteWorkflowRun(&run)
			o.publishBlockedRunGuidance(&run, "budget_exhausted")
			return fmt.Errorf("write workflow blocked: global step budget exhausted")
		}
		controllerTurns++
		o.busCtx.Mutable.SetWriteWorkflowRun(&run)
		decision, err := o.dispatchWriteControllerDecision()
		*stepsUsed++
		if err != nil {
			if errors.Is(err, ErrCanceled) || errors.Is(err, context.Canceled) {
				if o.completeDispatchInterruptedRunIfAllBatchesComplete(&run, "controller_dispatch_interrupted_after_complete") {
					return nil
				}
				if o.runDispatchInterruptedCompletionVerify(&run, stepsUsed, importedPlanMirror) {
					return nil
				}
				if o.completeDispatchInterruptedProofFollowupIfSourceComplete(&run, "controller_dispatch_write_deadline_proof_followup_unverified") {
					return nil
				}
				if o.completeInterruptedFollowupIfSourceComplete(&run, "controller_dispatch_followup_unverified", "controller_dispatch") {
					return nil
				}
				if o.publishAppliedPatchInterruptedGuidance(&run, o.busCtx.Mutable.ChangePlan(), err, "controller_dispatch_interrupted_after_applied_patch") {
					if o.appliedPatchInterruptedShouldBlock(&run, err) {
						run.Status = types.WriteWorkflowRunBlocked
						updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
						appendControllerProgress(&run, run.ActiveBatchID, "controller_dispatch_interrupted_after_applied_patch_blocked",
							"applied patch was preserved, but deterministic workflow interruption requires a terminal blocked state")
					} else if o.appliedPatchInterruptedCanResume(&run, err) {
						run.Status = types.WriteWorkflowRunInProgress
						updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchReadyToPlan)
						appendControllerProgress(&run, run.ActiveBatchID, "controller_dispatch_interrupted_after_applied_patch_resumable",
							"controller dispatch was interrupted after preserving an applied failed-verify patch; durable run remains resumable from typed failure evidence")
					}
					o.persistWriteWorkflowRun(&run)
					return err
				}
				if o.controllerDispatchInterruptedCanResumeRepairPlan(&run, o.busCtx.Mutable.ChangePlan(), err) {
					run.Status = types.WriteWorkflowRunInProgress
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchPlanned)
					appendControllerProgress(&run, run.ActiveBatchID, "controller_dispatch_write_deadline_resumable_repair_plan",
						"write-mode wall-clock deadline interrupted controller dispatch after a failed-verify repair plan was persisted; durable run remains resumable")
					o.persistWriteWorkflowRun(&run)
					return err
				}
				if o.cancellationSourceIsWriteDeadline(err) {
					run.Status = types.WriteWorkflowRunBlocked
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
					appendControllerProgress(&run, run.ActiveBatchID, "controller_dispatch_write_deadline_blocked",
						"write-mode wall-clock deadline interrupted controller dispatch before a terminal workflow decision")
					o.persistWriteWorkflowRun(&run)
					o.publishBlockedRunGuidance(&run, "controller_dispatch_write_deadline")
				}
				return err
			}
			var recovered bool
			decision, recovered = o.controllerDecisionFromTypedStateAfterDispatchError(&run)
			if !recovered {
				return err
			}
		}
		rawDecision := decision
		decision = o.normalizeControllerTypedStateDecision(decision, &run)
		if rawDecision.Action != decision.Action ||
			strings.TrimSpace(rawDecision.ReasonCode) != strings.TrimSpace(decision.ReasonCode) ||
			rawDecision.FinishDisposition != decision.FinishDisposition {
			logging.Info("[orchestrator] write controller decision normalized: action=%s reason=%s disposition=%s -> action=%s reason=%s disposition=%s",
				rawDecision.Action, strings.TrimSpace(rawDecision.ReasonCode), rawDecision.FinishDisposition,
				decision.Action, strings.TrimSpace(decision.ReasonCode), decision.FinishDisposition)
		}
		if activeBatchCompletedWithUnverifiedVerdict(&run) && decisionTargetsActiveBatch(decision, run.ActiveBatchID) {
			appendControllerProgress(&run, run.ActiveBatchID, "unverified_terminal_action_overridden",
				"active batch already completed with a typed unverified verifier outcome; finishing instead of replanning the same bytes")
			decision = writeflow.WriteWorkflowDecision{
				Action:            writeflow.ActionFinish,
				ReasonCode:        "active_batch_unverified_terminal",
				FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
			}
		}
		enforcedDecision := o.enforceControllerWorkflowTransition(decision, &run)
		if enforcedDecision.Action != decision.Action ||
			strings.TrimSpace(enforcedDecision.ReasonCode) != strings.TrimSpace(decision.ReasonCode) ||
			enforcedDecision.FinishDisposition != decision.FinishDisposition {
			logging.Info("[orchestrator] write controller transition enforced: action=%s reason=%s disposition=%s -> action=%s reason=%s disposition=%s",
				decision.Action, strings.TrimSpace(decision.ReasonCode), decision.FinishDisposition,
				enforcedDecision.Action, strings.TrimSpace(enforcedDecision.ReasonCode), enforcedDecision.FinishDisposition)
		}
		decision = enforcedDecision
		// Typed finish gate, evaluated BEFORE the decision mutates the run
		// (ApplyWorkflowDecisionToRun marks the active batch complete on
		// finish): a batch whose latest post-apply verify attempt failed
		// cannot be silently finished past. The model's typed escape lane
		// is finish_disposition=accept_unverified (schema-validated enum);
		// the rejection is surfaced through the progress ledger the next
		// controller turn renders.
		if decision.Action == writeflow.ActionFinish {
			if blocked := writeflow.FinishBlockedReason(run, decision); blocked != "" {
				lastInnerErr = fmt.Errorf("finish rejected: %s", blocked)
				appendControllerProgress(&run, run.ActiveBatchID, "finish_rejected_failed_verify", blocked)
				o.persistWriteWorkflowRun(&run)
				continue
			}
		}
		before := run
		run, err = writeflow.ApplyWorkflowDecisionToRun(run, decision)
		if err != nil {
			return err
		}
		o.busCtx.Mutable.SetWriteWorkflowRun(&run)
		// A batch switch invalidates the previous batch's verify-failure
		// carrier: the new batch's planner must not open with another
		// batch's failure section.
		if before.ActiveBatchID != run.ActiveBatchID {
			if h := o.busCtx.Mutable.VerifyFailureHandoff(); h != nil && h.BatchID != run.ActiveBatchID {
				o.busCtx.Mutable.ResetVerifyFailureHandoff()
			}
		}
		o.syncCurrentWriteContextPackToRun(&run)
		o.persistWriteWorkflowRun(&run)

		switch decision.Action {
		case writeflow.ActionExploreCode:
			batchID := run.ActiveBatchID
			if batchID == "" && decision.ExplorationRequest != nil {
				batchID = decision.ExplorationRequest.BatchID
			}
			exploreBudget := loopkernel.LoopBudget{
				MaxUnits:  controllerExplorationRoundBudget(run),
				UnitsUsed: explorationRounds[batchID],
			}
			if advance, ok := o.controllerLoopAdvanceAllowsAction(&run, loopkernel.LoopActionLocalize, exploreBudget); !ok {
				// Reject the ACTION, not the run (same pattern as the finish
				// gate): the controller sees the typed event next turn and can
				// still plan, replan, finish, or block on its own.
				lastInnerErr = fmt.Errorf("exploration budget exhausted for %s", batchID)
				appendControllerProgress(&run, batchID, "exploration_budget_exhausted",
					"loopkernel rejected explore_code with "+firstNonEmptyController(advance.ReasonCode, "loop_budget_units_exhausted")+"; choose plan_batch, replan_batch, finish, or block")
				o.persistWriteWorkflowRun(&run)
				continue
			}
			explorationRounds[batchID]++
			if decision.ExplorationRequest != nil {
				o.busCtx.Mutable.SetWriteExplorationRequest(decision.ExplorationRequest)
			}
			runner := o.readExplorationRunner
			if runner == nil {
				runner = defaultReadExplorationRunner{}
			}
			used, err := runner.Run(o)
			*stepsUsed += used
			if err != nil {
				logging.Warning("[orchestrator] write controller exploration degraded: %v", err)
				updateWorkflowRunBatchExplore(&run, batchID, "degraded", err.Error())
				appendControllerProgress(&run, batchID, "exploration_degraded", err.Error())
				if activeBatchHasUsableExplorationHandoffForRun(o.busCtx.Mutable, run) {
					updateWorkflowRunBatchStatus(&run, batchID, types.WriteWorkflowBatchReadyToPlan)
					appendControllerProgress(&run, batchID, "exploration_degraded_handoff_ready", "typed exploration handoff available after degraded read-only exploration")
				}
			} else {
				updateWorkflowRunBatchStatus(&run, batchID, types.WriteWorkflowBatchReadyToPlan)
				updateWorkflowRunBatchExplore(&run, batchID, "complete", "")
				appendControllerProgress(&run, batchID, "exploration_complete", "")
			}
			o.syncCurrentWriteContextPackToRun(&run)
			o.persistWriteWorkflowRun(&run)
		case writeflow.ActionPlanBatch, writeflow.ActionAppendBatch, writeflow.ActionSplitBatch, writeflow.ActionReplanBatch:
			if len(before.Batches) < len(run.Batches) && run.Budget.MaxBatches > 0 && len(run.Batches) > run.Budget.MaxBatches {
				run.Status = types.WriteWorkflowRunBlocked
				appendControllerProgress(&run, run.ActiveBatchID, "max_batches_reached", "controller attempted to append beyond max_batches")
				o.persistWriteWorkflowRun(&run)
				o.publishBlockedRunGuidance(&run, "max_batches_reached")
				return fmt.Errorf("write workflow blocked: max batch budget reached")
			}
			priorPlan := o.busCtx.Mutable.ChangePlan()
			priorPlanFingerprint := types.PlanFingerprint(priorPlan)
			replanFromFailedVerify := false
			if decision.Action == writeflow.ActionReplanBatch {
				_, replanFromFailedVerify = activeBatchNeedsReplan(&before)
			}
			if replanFromFailedVerify {
				if restored, restoreErr := o.restoreActiveSliceCheckpointBeforeReplan(&run, "verify_failed_replan"); restoreErr != nil {
					appendControllerProgress(&run, run.ActiveBatchID, "checkpoint_restore_failed", restoreErr.Error())
					o.persistWriteWorkflowRun(&run)
				} else if restored {
					appendControllerProgress(&run, run.ActiveBatchID, "checkpoint_restored_before_replan", "active slice worktree reset to checkpoint before planner replan")
					o.persistWriteWorkflowRun(&run)
				}
			}
			innerErr := o.runControllerPlanBatch(decision.Batch, stepsUsed)
			if current := o.busCtx.Mutable.WriteWorkflowRun(); current != nil && strings.TrimSpace(current.RunID) == strings.TrimSpace(run.RunID) {
				run = *current
			}
			plan := o.busCtx.Mutable.ChangePlan()
			if innerErr == nil && replanFromFailedVerify && replanReusedFailedPlan(priorPlanFingerprint, plan) {
				innerErr = fmt.Errorf("write controller replan reused the failed ChangePlan fingerprint without producing a replacement plan")
				if !changePlanHasAppliedWork(plan) && priorPlan != nil {
					o.busCtx.Mutable.SetChangePlan(priorPlan)
					plan = priorPlan
				}
			}
			if plan != nil {
				o.stampWorkflowPlanForActiveBatch(plan, &run)
				updateWorkflowRunBatchPlan(&run, run.ActiveBatchID, plan, priorPlan)
				run = attachPlanContextPackToWorkflowRun(run, plan)
			}
			if errors.Is(innerErr, errPlannerProbePassedExistingWorktree) {
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchVerifying)
				markWorkflowRunActiveSliceObservingForRestoredPlan(&run, run.ActiveBatchID, plan, "planner_probe_passed_existing_worktree")
				appendControllerProgress(&run, run.ActiveBatchID, "planner_probe_passed_existing_worktree", "planner dry-run probe passed after replan without a new ChangePlan; run post-apply verify on the current worktree")
				o.mirrorActivePlanToImportFile(importedPlanMirror)
				o.syncCurrentWriteContextPackToRun(&run)
				o.persistWriteWorkflowRun(&run)
				o.busCtx.TaskState.LastError = ""
				continue
			}
			if innerErr != nil {
				lastInnerErr = innerErr
				if o.completeInterruptedFollowupIfSourceComplete(&run, "plan_batch_followup_unverified", "plan_batch") {
					return nil
				}
				if o.publishAppliedPatchInterruptedGuidance(&run, plan, lastInnerErr, "plan_batch_interrupted_after_applied_patch") {
					if o.appliedPatchInterruptedShouldBlock(&run, lastInnerErr) {
						run.Status = types.WriteWorkflowRunBlocked
						updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
						appendControllerProgress(&run, run.ActiveBatchID, "plan_batch_interrupted_after_applied_patch_blocked",
							"failed verification needed a replacement plan, but planning stopped before producing one")
					} else if o.appliedPatchInterruptedCanResume(&run, lastInnerErr) {
						run.Status = types.WriteWorkflowRunInProgress
						updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchReadyToPlan)
						appendControllerProgress(&run, run.ActiveBatchID, "plan_batch_interrupted_after_applied_patch_resumable",
							"failed verification repair was interrupted after preserving an applied patch; durable run remains resumable from typed failure evidence")
					}
					o.persistWriteWorkflowRun(&run)
					return lastInnerErr
				}
				if errors.Is(lastInnerErr, ErrCanceled) || errors.Is(lastInnerErr, context.Canceled) {
					run.Status = types.WriteWorkflowRunBlocked
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
					appendControllerProgress(&run, run.ActiveBatchID, "plan_batch_canceled", lastInnerErr.Error())
					o.persistWriteWorkflowRun(&run)
					o.publishBlockedRunGuidance(&run, "plan_batch_canceled")
					return lastInnerErr
				}
				if !errors.Is(lastInnerErr, ErrCanceled) && !errors.Is(lastInnerErr, context.Canceled) {
					run.Status = types.WriteWorkflowRunBlocked
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
					appendControllerProgress(&run, run.ActiveBatchID, "plan_batch_failed_blocked",
						"planner did not produce a ChangePlan after bounded retries: "+lastInnerErr.Error())
					o.persistWriteWorkflowRun(&run)
					o.publishBlockedRunGuidance(&run, "plan_batch_failed")
					return lastInnerErr
				}
			}
			if innerErr == nil && o.busCtx.Mode == types.ModeApply {
				if explored, exploreErr := o.runOwnerLocalizationRequirementExplorationBeforeApply(&run, plan, stepsUsed); explored {
					if exploreErr != nil {
						lastInnerErr = exploreErr
					}
					continue
				}
			}
			updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchPlanned)
			appendControllerProgress(&run, run.ActiveBatchID, "batch_planned", "")
			o.persistCurrentChangePlanSnapshot()
			o.mirrorActivePlanToImportFile(importedPlanMirror)
			o.syncCurrentWriteContextPackToRun(&run)
			o.persistWriteWorkflowRun(&run)
			if o.busCtx.Mode == types.ModeApply && changePlanIsProofProbeOnly(plan) {
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchVerifying)
				appendControllerProgress(&run, run.ActiveBatchID, "proof_probe_only_plan_ready",
					"controller-owned proof follow-up produced no source edits and will verify typed probes against the current worktree")
				o.persistWriteWorkflowRun(&run)
				o.busCtx.TaskState.LastError = ""
				continue
			}
			if writePlanDeniedByApproval(plan) {
				run.Status = types.WriteWorkflowRunBlocked
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
				appendControllerProgress(&run, run.ActiveBatchID, "approval_denied", "typed write approval denied the plan")
				o.persistWriteWorkflowRun(&run)
				o.publishBlockedRunGuidance(&run, "approval_denied")
				return fmt.Errorf("write workflow blocked: approval denied for plan %s", plan.ID)
			}
			if o.busCtx.Mode == types.ModeApply && writePlanNeedsManualApproval(plan, &run) {
				run.Status = types.WriteWorkflowRunInProgress
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchPendingApproval)
				appendControllerProgress(&run, run.ActiveBatchID, string(types.WriteWorkflowBatchPendingApproval), "typed write approval requires operator confirmation")
				o.persistWriteWorkflowRun(&run)
				o.publishBlockedRunGuidance(&run, "pending_approval")
				return fmt.Errorf("write workflow pending approval for plan %s: %s", plan.ID, plan.Approval.ReasonCode)
			}
			if o.busCtx.Mode == types.ModePlan {
				run.Status = types.WriteWorkflowRunComplete
				appendControllerProgress(&run, run.ActiveBatchID, "plan_mode_complete", "plan mode stops after producing a reviewable ChangePlan")
				o.persistWriteWorkflowRun(&run)
				return nil
			}
			if o.busCtx.Mode == types.ModeApply && *stepsUsed >= o.maxSteps && o.writePlanCanProceedWithoutApprovalPause(plan) {
				appendControllerProgress(&run, run.ActiveBatchID, "budget_ready_plan_auto_apply",
					"deterministic write approval policy allows the freshly planned batch to apply without another controller turn")
				outcome, applyErr := o.runControllerApplyPlanTransition(&run, stepsUsed, "auto_executable_plan")
				switch outcome {
				case writeControllerApplyTransitionTerminal:
					return applyErr
				case writeControllerApplyTransitionRetry:
					lastInnerErr = applyErr
					continue
				}
			}
		case writeflow.ActionApplyPlan:
			if o.busCtx.Mode == types.ModePlan {
				run.Status = types.WriteWorkflowRunBlocked
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
				appendControllerProgress(&run, run.ActiveBatchID, "apply_not_allowed_in_plan_mode", "")
				o.persistWriteWorkflowRun(&run)
				o.publishBlockedRunGuidance(&run, "apply_not_allowed_in_plan_mode")
				return fmt.Errorf("write workflow blocked: apply_plan is not valid in plan mode")
			}
			outcome, innerErr := o.runControllerApplyPlanTransition(&run, stepsUsed, decision.ReasonCode)
			switch outcome {
			case writeControllerApplyTransitionTerminal:
				return innerErr
			case writeControllerApplyTransitionRetry:
				lastInnerErr = innerErr
				continue
			}
		case writeflow.ActionVerifyBatch:
			if o.busCtx.Mode == types.ModePlan {
				run.Status = types.WriteWorkflowRunBlocked
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
				appendControllerProgress(&run, run.ActiveBatchID, "verify_not_allowed_in_plan_mode", "")
				o.persistWriteWorkflowRun(&run)
				o.publishBlockedRunGuidance(&run, "verify_not_allowed_in_plan_mode")
				return fmt.Errorf("write workflow blocked: verify_batch is not valid in plan mode")
			}
			var innerErr error
			var report *types.ChangeReport
			var outcome writeflow.VerifyAttemptOutcome
			for {
				innerErr = o.runControllerVerifyBatch(stepsUsed)
				report = o.busCtx.Mutable.ChangeReport()
				if report != nil {
					o.ensureChangeReportPlanID(report)
					if reportErr := o.validateControllerPostApplyReport(report); reportErr != nil && innerErr == nil {
						innerErr = reportErr
					}
					o.syncMutablePlanStatusAfterVerify(report, innerErr)
					o.mirrorActivePlanToImportFile(importedPlanMirror)
					if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
						run = attachPlanContextPackToWorkflowRun(run, plan)
					}
					reportPack := types.WriteContextPackFromChangeReport(report).
						WithScope(run.ActiveBatchID, activeWorkflowRunSliceID(run, run.ActiveBatchID))
					o.busCtx.Mutable.MergeWriteContextPack(reportPack)
					status := writeWorkflowVerifyAttemptStatus(report, innerErr)
					reason := writeWorkflowVerifyAttemptReason(report, innerErr)
					updateWorkflowRunBatchVerify(&run, run.ActiveBatchID, report, status, reason)
					updateWorkflowRunActiveSliceObserve(&run, run.ActiveBatchID, report, status, reason)
					o.syncCurrentWriteContextPackToRun(&run)
				} else if o.skipVerify {
					o.syncMutablePlanStatusAfterSkippedVerify()
					if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
						updateWorkflowRunBatchVerifySkipped(&run, run.ActiveBatchID, plan.ID)
						updateWorkflowRunActiveSliceObserveSkipped(&run, run.ActiveBatchID, plan.ID)
					}
				}
				outcome = writeflow.ClassifyVerifyAttemptOutcome(report, innerErr)
				if o.skipVerify || report != nil || !outcome.Retryable {
					break
				}
				planID := ""
				if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
					planID = plan.ID
				}
				if verifyInfraFailures[run.ActiveBatchID] >= o.writeRetryBudget {
					lastInnerErr = innerErr
					if lastInnerErr == nil {
						lastInnerErr = fmt.Errorf("verify executor did not produce a ChangeReport")
					}
					run.Status = types.WriteWorkflowRunBlocked
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
					updateWorkflowRunBatchVerifyInfra(&run, run.ActiveBatchID, planID, "infra_error", outcome.ReasonCode)
					o.markActivePlanUnverifiedAfterVerifyInfra(outcome.ReasonCode)
					appendControllerProgress(&run, run.ActiveBatchID, "verify_infra_retry_budget_exhausted", lastInnerErr.Error())
					o.persistWriteWorkflowRun(&run)
					o.publishBlockedRunGuidance(&run, "verify_infra_retry_budget_exhausted")
					return lastInnerErr
				}
				verifyInfraFailures[run.ActiveBatchID]++
				updateWorkflowRunBatchVerifyInfra(&run, run.ActiveBatchID, planID, "infra_error", outcome.ReasonCode)
				appendControllerProgress(&run, run.ActiveBatchID, "verify_infra_retry", outcome.ReasonCode)
				o.persistWriteWorkflowRun(&run)
				o.busCtx.TaskState.LastError = ""
			}
			if outcome.Kind == writeflow.VerifyOutcomeRunnerMissing ||
				outcome.Kind == writeflow.VerifyOutcomeParserError ||
				outcome.Kind == writeflow.VerifyOutcomeNoTests {
				if o.advanceWorkflowAfterSuccessfulSliceObserve(&run, outcome) {
					o.mirrorActivePlanToImportFile(importedPlanMirror)
					o.syncCurrentWriteContextPackToRun(&run)
					o.persistWriteWorkflowRun(&run)
					continue
				}
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchComplete)
				updateWorkflowRunBatchCompletion(&run, run.ActiveBatchID, types.WriteWorkflowCompletionUnverified, outcome.ReasonCode, "verify_attempt")
				appendControllerProgress(&run, run.ActiveBatchID, "batch_unverified", outcome.ReasonCode)
				o.busCtx.TaskState.LastError = ""
				o.busCtx.Mutable.ResetVerifyFailureHandoff()
				o.persistWriteWorkflowRun(&run)
				continue
			}
			if outcome.Kind == writeflow.VerifyOutcomeReportFailed || innerErr != nil || (report != nil && !report.Passed) {
				if innerErr == nil && report != nil {
					innerErr = fmt.Errorf("verify failed: %s", strings.TrimSpace(report.FailureSummary))
				}
				lastInnerErr = innerErr
				verifyFailures[run.ActiveBatchID]++
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchReadyToPlan)
				appendControllerProgress(&run, run.ActiveBatchID, "verify_failed", innerErr.Error())
				diffRef, surfaceRef := o.persistVerifyFailureEvidence(&run, report, verifyFailures[run.ActiveBatchID])
				if handoff := o.resolveVerifyFailureHandoffArtifacts(types.BuildVerifyFailureHandoff(report, run.ActiveBatchID, verifyFailures[run.ActiveBatchID], diffRef, surfaceRef)); handoff != nil {
					o.busCtx.Mutable.SetVerifyFailureHandoff(handoff)
				}
				o.persistWriteWorkflowRun(&run)
				if verifyFailures[run.ActiveBatchID] > o.writeRetryBudget {
					run.Status = types.WriteWorkflowRunBlocked
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
					appendControllerProgress(&run, run.ActiveBatchID, "verify_retry_budget_exhausted", innerErr.Error())
					o.persistWriteWorkflowRun(&run)
					o.publishBlockedRunGuidance(&run, "verify_retry_budget_exhausted")
					return innerErr
				}
				o.busCtx.TaskState.LastError = ""
				continue
			}
			if o.advanceWorkflowAfterSuccessfulSliceObserve(&run, outcome) {
				if o.appendCumulativePatchReviewFollowupIfNeeded(&run, report, outcome) {
					o.mirrorActivePlanToImportFile(importedPlanMirror)
					o.syncCurrentWriteContextPackToRun(&run)
					o.persistWriteWorkflowRun(&run)
					continue
				}
				o.mirrorActivePlanToImportFile(importedPlanMirror)
				o.syncCurrentWriteContextPackToRun(&run)
				o.persistWriteWorkflowRun(&run)
				continue
			}
			updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchComplete)
			if outcome.Kind == writeflow.VerifyOutcomeNoTests {
				updateWorkflowRunBatchCompletion(&run, run.ActiveBatchID, types.WriteWorkflowCompletionUnverified, outcome.ReasonCode, "verify_attempt")
				appendControllerProgress(&run, run.ActiveBatchID, "batch_unverified", outcome.ReasonCode)
			} else {
				updateWorkflowRunBatchCompletion(&run, run.ActiveBatchID, types.WriteWorkflowCompletionVerified, outcome.ReasonCode, "verify_attempt")
				appendControllerProgress(&run, run.ActiveBatchID, "batch_verified", "")
			}
			if o.appendCumulativePatchReviewFollowupIfNeeded(&run, report, outcome) {
				o.mirrorActivePlanToImportFile(importedPlanMirror)
				o.syncCurrentWriteContextPackToRun(&run)
				o.persistWriteWorkflowRun(&run)
				continue
			}
			o.busCtx.Mutable.ResetVerifyFailureHandoff()
			o.persistWriteWorkflowRun(&run)
		case writeflow.ActionFinish:
			run.Status = types.WriteWorkflowRunComplete
			writeflow.MarkWorkflowRunCompletionFromBatches(&run)
			o.busCtx.Mutable.ResetVerifyFailureHandoff()
			o.persistWriteWorkflowRun(&run)
			if strings.TrimSpace(o.busCtx.Mutable.Result()) == "" {
				result := "write workflow complete"
				if decision.FinishDisposition == writeflow.FinishDispositionAcceptUnverified {
					if unverified := writeflow.UnverifiedBatchSummaries(run); len(unverified) > 0 {
						result += " — accepted with failed verification: " + strings.Join(unverified, ", ")
					}
				}
				if caveat := writeWorkflowUnverifiedCompletionCaveat(run); caveat != "" {
					result += " — " + caveat
				}
				o.busCtx.Mutable.SetResult(result)
			}
			return nil
		case writeflow.ActionBlock, writeflow.ActionAskUser:
			run.Status = types.WriteWorkflowRunBlocked
			o.persistWriteWorkflowRun(&run)
			msg := "write workflow blocked: " + decision.ReasonCode
			if decision.Reason != "" {
				msg += ": " + decision.Reason
			}
			// ask_user carries typed questions; the operator must see them
			// verbatim, not just the reason prose.
			if len(decision.QuestionsForUser) > 0 {
				msg += "\nQuestions for the user:"
				for i, q := range decision.QuestionsForUser {
					if i >= 5 {
						msg += fmt.Sprintf("\n- … (+%d more)", len(decision.QuestionsForUser)-i)
						break
					}
					msg += "\n- " + q
				}
			}
			if len(decision.MissingFacts) > 0 {
				msg += "\nMissing facts:"
				for i, fact := range decision.MissingFacts {
					if i >= 5 {
						msg += fmt.Sprintf("\n- … (+%d more)", len(decision.MissingFacts)-i)
						break
					}
					detail := fact.Kind + " for " + fact.Consumer + ": " + fact.Description
					if fact.EvidenceRef != "" {
						detail += " [" + fact.EvidenceRef + "]"
					}
					msg += "\n- " + detail
				}
			}
			o.busCtx.Mutable.SetResultPlain(msg)
			return fmt.Errorf("%s", msg)
		}
	}
	if o.runBudgetCompletionVerify(&run, stepsUsed, importedPlanMirror) {
		return nil
	}
	if o.completeBudgetExhaustedRunIfAllBatchesComplete(&run, "controller_turn_budget_all_batches_complete") {
		return nil
	}
	run.Status = types.WriteWorkflowRunBlocked
	appendControllerProgress(&run, run.ActiveBatchID, "controller_turn_budget_exhausted", "")
	o.persistWriteWorkflowRun(&run)
	o.publishBlockedRunGuidance(&run, "controller_turn_budget_exhausted")
	if lastInnerErr != nil {
		return fmt.Errorf("write workflow blocked after controller turn budget: %w", lastInnerErr)
	}
	return fmt.Errorf("write workflow blocked after controller turn budget")
}

func (o *Orchestrator) loadOrSeedWriteWorkflowRun() types.WriteWorkflowRun {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return types.WriteWorkflowRun{}
	}
	if strings.TrimSpace(o.busCtx.PlanPath) == "" {
		if loader, ok := o.writeWorkflowRunStore.(WriteWorkflowRunActiveLoader); ok && loader != nil {
			run, err := loader.FindActiveRun()
			if err != nil {
				logging.Warning("[orchestrator] write workflow resume lookup failed: %v", err)
			} else if isResumableWriteWorkflowRun(run) {
				normalized := types.NormalizeWriteWorkflowRun(*run)
				o.busCtx.Mutable.SetWriteWorkflowRun(&normalized)
				appendControllerProgress(&normalized, normalized.ActiveBatchID, "workflow_resumed", "resumed active workflow run from store")
				return normalized
			}
		}
	}
	return o.seedWriteWorkflowRun()
}

func isResumableWriteWorkflowRun(run *types.WriteWorkflowRun) bool {
	if run == nil || strings.TrimSpace(run.RunID) == "" {
		return false
	}
	switch run.Status {
	case types.WriteWorkflowRunComplete, types.WriteWorkflowRunBlocked:
		return false
	default:
		return true
	}
}

func (o *Orchestrator) pendingApprovalResumeMessage(run *types.WriteWorkflowRun) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || o.busCtx.Mode != types.ModeApply {
		return ""
	}
	batch, ok := activeWriteWorkflowControllerBatch(run)
	if !ok || batch.Status != types.WriteWorkflowBatchPendingApproval {
		return ""
	}
	planID := strings.TrimSpace(batch.PlanID)
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil && planID != "" {
		plan = o.loadDurablePlanArtifact(planID)
		if plan != nil {
			o.busCtx.Mutable.SetChangePlan(plan)
		}
	}
	if plan != nil && planID != "" && strings.TrimSpace(plan.ID) == planID &&
		writeApprovalRecordAllowsManualApply(plan, types.PlanFingerprint(plan), run) {
		return ""
	}
	if plan != nil && strings.TrimSpace(plan.ID) != "" {
		planID = strings.TrimSpace(plan.ID)
	}
	if planID == "" {
		return fmt.Sprintf("write workflow pending approval for batch %s", writeWorkflowActiveBatchIDOrDefault(run))
	}
	reason := ""
	if plan != nil && plan.Approval != nil {
		reason = strings.TrimSpace(plan.Approval.ReasonCode)
	}
	if reason == "" {
		return fmt.Sprintf("write workflow pending approval for plan %s", planID)
	}
	return fmt.Sprintf("write workflow pending approval for plan %s: %s", planID, reason)
}

func writeWorkflowActiveBatchIDOrDefault(run *types.WriteWorkflowRun) string {
	if run == nil {
		return "active"
	}
	if id := strings.TrimSpace(run.ActiveBatchID); id != "" {
		return id
	}
	return "active"
}

func activeWriteWorkflowControllerBatch(run *types.WriteWorkflowRun) (types.WriteWorkflowBatch, bool) {
	if run == nil {
		return types.WriteWorkflowBatch{}, false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" && len(run.Batches) > 0 {
		return run.Batches[0], true
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == activeID {
			return batch, true
		}
	}
	return types.WriteWorkflowBatch{}, false
}

func (o *Orchestrator) seedWriteWorkflowRun() types.WriteWorkflowRun {
	seed := writeflow.WorkflowSeedFromWriteAnalysis(o.busCtx.Mutable.WriteAnalysisIR())
	goal := strings.TrimSpace(seed.Goal)
	if goal == "" {
		goal = types.StripConversationPrefix(o.busCtx.Mutable.Objective())
	}
	var importedPlan *types.ChangePlan
	if path := strings.TrimSpace(o.busCtx.PlanPath); path != "" {
		plan, err := types.LoadChangePlanFromFile(path)
		if err != nil {
			logging.Warning("[orchestrator] write workflow seed: load imported plan failed: %v", err)
		} else if plan != nil {
			importedPlan = plan
			o.busCtx.Mutable.SetChangePlan(plan)
			if plan.WriteAnalysisIR != nil {
				o.busCtx.Mutable.SetWriteAnalysisIR(plan.WriteAnalysisIR)
			}
			if o.busCtx.Mode == types.ModeVerify && o.reuseWorktreePath == "" && plan.WorktreePath != "" {
				o.reuseWorktreePath = plan.WorktreePath
			}
			if strings.TrimSpace(plan.Request) != "" {
				goal = strings.TrimSpace(plan.Request)
			} else if strings.TrimSpace(plan.Summary) != "" {
				goal = strings.TrimSpace(plan.Summary)
			}
		}
	}
	run := types.WriteWorkflowRun{
		RunID:         fmt.Sprintf("wf-%d-%d", time.Now().UnixNano(), os.Getpid()),
		Goal:          goal,
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Budget: types.WriteWorkflowBudget{
			MaxBatches:           defaultWriteWorkflowMaxBatches,
			MaxExplorationRounds: defaultWriteWorkflowMaxExplorationRounds,
		},
	}
	if importedPlan != nil {
		batch := types.WriteWorkflowBatch{
			ID:        run.ActiveBatchID,
			Goal:      strings.TrimSpace(importedPlan.Summary),
			Status:    types.WriteWorkflowBatchPlanned,
			PlanID:    importedPlan.ID,
			PlanRef:   importedPlan.ID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		initializeWorkflowBatchSlicesFromPlan(&batch, importedPlan, "", nil)
		run.Batches = append(run.Batches, batch)
		run.Budget.BatchesUsed = 1
		run = attachPlanContextPackToWorkflowRun(run, importedPlan)
	} else if seed.NextBatch != nil {
		run.ActiveBatchID = batchIDOrDefault(seed.NextBatch.ID, "batch-1")
		// The seed's typed batch status carries the micro-scope short
		// path: analyzer-classified contained changes with known anchors
		// start ready_to_plan and skip the default exploration round.
		// Same DAG, same action schema — only the starting state differs.
		status := types.WriteWorkflowBatchNeedsExploration
		if seed.NextBatch.Status == writeflow.BatchReadyForChangePlan {
			status = types.WriteWorkflowBatchReadyToPlan
		}
		run.Batches = append(run.Batches, types.WriteWorkflowBatch{
			ID:              run.ActiveBatchID,
			Goal:            seed.NextBatch.Goal,
			Purpose:         seed.NextBatch.Purpose,
			ExpectedPaths:   append([]string(nil), seed.NextBatch.ExpectedPaths...),
			SuccessCriteria: append([]string(nil), seed.NextBatch.SuccessCriteria...),
			Status:          status,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		})
		run.Budget.BatchesUsed = 1
	} else {
		run.Batches = append(run.Batches, types.WriteWorkflowBatch{
			ID:        run.ActiveBatchID,
			Goal:      goal,
			Status:    types.WriteWorkflowBatchReadyToPlan,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		run.Budget.BatchesUsed = 1
	}
	run = o.withSeededWriteAnalysisContextPack(run, goal)
	if pack := o.busCtx.Mutable.WriteContextPack(); pack != nil {
		run = upsertWorkflowRunContextPack(run, *pack)
	}
	run = types.NormalizeWriteWorkflowRun(run)
	o.busCtx.Mutable.SetWriteWorkflowRun(&run)
	if importedPlan != nil {
		pending, _ := pendingAppliesForActivePlanScope(o.busCtx.Mutable, importedPlan, "workflow_seed_plan_file")
		o.busCtx.Mutable.WriteClosure().ReplacePendingApplies(pending)
	}
	return run
}

func (o *Orchestrator) withSeededWriteAnalysisContextPack(run types.WriteWorkflowRun, goal string) types.WriteWorkflowRun {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return run
	}
	ir := o.busCtx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return run
	}
	pack := types.WriteContextPackFromWriteAnalysisIR(ir)
	if len(pack.Items) == 0 {
		return run
	}
	if batchID := strings.TrimSpace(run.ActiveBatchID); batchID != "" {
		pack.BatchID = batchID
	}
	if strings.TrimSpace(pack.Goal) == "" {
		pack.Goal = strings.TrimSpace(goal)
	}
	return upsertWorkflowRunContextPack(run, pack)
}

func (o *Orchestrator) dispatchWriteControllerDecision() (writeflow.WriteWorkflowDecision, error) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return writeflow.WriteWorkflowDecision{}, fmt.Errorf("write controller: nil orchestrator/bus")
	}
	o.busCtx.PipelineStage = types.StageWriteController
	o.busCtx.TaskState.Stage = types.StageWriteController
	out, err := o.dispatchStage(types.StageWriteController)
	if err != nil {
		return writeflow.WriteWorkflowDecision{}, err
	}
	raw := o.busCtx.Mutable.WriteWorkflowDecisionJSON()
	if len(raw) == 0 && out != nil && len(out.Data) > 0 {
		raw = out.Data
	}
	if len(raw) == 0 {
		if out != nil && out.Error != "" {
			return writeflow.WriteWorkflowDecision{}, fmt.Errorf("%s", out.Error)
		}
		return writeflow.WriteWorkflowDecision{}, fmt.Errorf("write controller produced no decision")
	}
	var decision writeflow.WriteWorkflowDecision
	if err := json.Unmarshal(raw, &decision); err != nil {
		return writeflow.WriteWorkflowDecision{}, fmt.Errorf("write controller decision parse failed: %w", err)
	}
	if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
		decision = writeflow.HydrateWriteWorkflowDecisionFromRun(decision, *run)
	} else {
		decision = writeflow.NormalizeWriteWorkflowDecision(decision)
	}
	if errs := writeflow.ValidateWriteWorkflowDecision(decision); len(errs) > 0 {
		return writeflow.WriteWorkflowDecision{}, fmt.Errorf("write controller decision rejected: %s", strings.Join(errs, "; "))
	}
	return decision, nil
}

func (o *Orchestrator) runControllerPlanBatch(batch *writeflow.WriteBatchPlan, stepsUsed *int) error {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return fmt.Errorf("write controller plan batch missing context")
	}
	if batch != nil {
		o.seedControllerBatchPlanningHint(*batch)
		o.seedControllerBatchExplorationContext(*batch)
	}
	transientUsed := 0
	noPlanRetryCount := 0
	offScopeRiskReplanRetried := false
	localizationReplanRetried := false
	localizationOwnerReplanRetried := false
	testContractReplanRetried := false
	testContractSourceOnlyReplanRetried := false
	proofProbeReplanRetried := false
	proofSourceEditReplanRetried := false
	proofNonExecutableReplanRetried := false
	lastStallSignature := ""
	for {
		priorPlan := o.busCtx.Mutable.ChangePlan()
		o.prepareControllerPlanningState()
		_, err := o.runControllerWriteStage(types.StagePlan, stepsUsed)
		if err != nil && (errors.Is(err, ErrCanceled) || errors.Is(err, context.Canceled)) && changePlanHasAppliedWork(priorPlan) {
			o.busCtx.Mutable.SetChangePlan(priorPlan)
			return err
		}
		if err != nil && llm.IsStreamLevelRetryable(err) && transientUsed < o.transientRetryBudget {
			sig := computeStallSignature(o.busCtx.Mutable.DispatchToolResults(), nil, types.StagePlan)
			if transientHardPlateauEligible(err, sig) && sig == lastStallSignature {
				friendly := stallPlateauMessage(o.busCtx, types.StagePlan, friendlyDispatchErr(err), o.autoInitRepo, o.scaffoldEnabled)
				o.busCtx.TaskState.LastError = friendly
				return errors.New(friendly)
			}
			lastStallSignature = sig
			transientUsed++
			o.busCtx.TaskState.LastError = ""
			logging.Warning("[orchestrator] controller plan transient dispatch error; retry %d/%d: %v",
				transientUsed, o.transientRetryBudget, err)
			continue
		}
		o.syncPlannerObservationContextPack(batch)
		if err == nil && changePlanIsNoChangeRequired(o.busCtx.Mutable.ChangePlan()) {
			plan := o.busCtx.Mutable.ChangePlan()
			if changePlanIsProofProbeOnly(plan) && activeBatchProofFollowupPurpose(o.busCtx.Mutable.WriteWorkflowRun()) {
				if o.enrichProofFollowupPlanProbeRefs(batch, plan) {
					o.busCtx.Mutable.SetChangePlan(plan)
				}
				return nil
			}
			if plannerProbePassedExistingAppliedWorktree(o.busCtx.Mutable, priorPlan) {
				o.busCtx.Mutable.SetChangePlan(priorPlan)
				o.busCtx.TaskState.LastError = ""
				logging.Info("[orchestrator] controller replan emitted no-change sentinel and planner dry-run probe passed; restoring plan %s for post-apply verify", priorPlan.ID)
				return errPlannerProbePassedExistingWorktree
			}
			return errors.New("write controller replan emitted no-change sentinel without a typed passing planner probe on an applied prior plan")
		}
		if err == nil && o.busCtx.Mutable.ChangePlan() != nil {
			if hint, paths := o.testContractReplanHint(o.busCtx.Mutable.ChangePlan()); hint != "" {
				if !testContractReplanRetried {
					testContractReplanRetried = true
					existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
					if existing != "" {
						existing += "\n\n"
					}
					o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + hint))
					o.busCtx.TaskState.LastError = ""
					logging.Warning("[orchestrator] controller plan weakened protected regression test contract; retrying bounded planning once paths=%s", strings.Join(paths, ","))
					continue
				}
				if !testContractSourceOnlyReplanRetried {
					testContractSourceOnlyReplanRetried = true
					if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
						appendControllerProgress(run, run.ActiveBatchID, "protected_test_source_only_replan_requested", strings.Join(paths, ","))
						o.busCtx.Mutable.SetWriteWorkflowRun(run)
					}
					existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
					if existing != "" {
						existing += "\n\n"
					}
					o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + renderProtectedTestSourceOnlyRepairHint(paths)))
					o.busCtx.TaskState.LastError = ""
					logging.Warning("[orchestrator] controller plan still weakened protected regression test contract; forcing one source-only repair attempt paths=%s", strings.Join(paths, ","))
					continue
				}
				return fmt.Errorf("write controller plan weakened protected regression test contract after retry: %s", strings.Join(paths, ","))
			}
			if !offScopeRiskReplanRetried {
				if hint, paths := o.offScopeHighRiskReplanHint(o.busCtx.Mutable.ChangePlan()); hint != "" {
					offScopeRiskReplanRetried = true
					existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
					if existing != "" {
						existing += "\n\n"
					}
					o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + hint))
					o.busCtx.TaskState.LastError = ""
					logging.Warning("[orchestrator] controller plan touched off-scope high-risk path(s); retrying bounded planning once paths=%s", strings.Join(paths, ","))
					continue
				}
			}
			if !localizationReplanRetried {
				if hint, paths := o.localizationCoverageReplanHint(o.busCtx.Mutable.ChangePlan()); hint != "" {
					localizationReplanRetried = true
					existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
					if existing != "" {
						existing += "\n\n"
					}
					o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + hint))
					o.busCtx.TaskState.LastError = ""
					logging.Warning("[orchestrator] controller plan touched source path(s) outside prior localization context; retrying bounded planning once paths=%s", strings.Join(paths, ","))
					continue
				}
			}
			if !localizationOwnerReplanRetried && !localizationReplanRetried && !offScopeRiskReplanRetried && !testContractReplanRetried {
				if hint, paths := o.localizationOwnerDepthReplanHint(o.busCtx.Mutable.ChangePlan()); hint != "" {
					localizationOwnerReplanRetried = true
					if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
						appendControllerProgress(run, run.ActiveBatchID, "owner_localization_depth_replan_requested", strings.Join(paths, ","))
						o.busCtx.Mutable.SetWriteWorkflowRun(run)
					}
					existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
					if existing != "" {
						existing += "\n\n"
					}
					o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + hint))
					o.busCtx.TaskState.LastError = ""
					logging.Warning("[orchestrator] controller plan used path-only localization without owner/evidence anchors; retrying bounded planning once paths=%s", strings.Join(paths, ","))
					continue
				}
			}
			plan := o.busCtx.Mutable.ChangePlan()
			if hint := proofFollowupProbeRequiredHint(batch, plan); hint != "" {
				if !proofProbeReplanRetried {
					proofProbeReplanRetried = true
					existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
					if existing != "" {
						existing += "\n\n"
					}
					o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + hint))
					o.busCtx.TaskState.LastError = ""
					logging.Warning("[orchestrator] proof-follow-up plan omitted verification_probes; retrying bounded planning once")
					continue
				}
				return fmt.Errorf("proof follow-up ChangePlan missing verification_probes")
			}
			if o.enrichProofFollowupPlanProbeRefs(batch, plan) {
				o.busCtx.Mutable.SetChangePlan(plan)
			}
			if !proofSourceEditReplanRetried {
				if hint, paths := proofFollowupProductionEditWithoutFailureHint(o.busCtx.Mutable, batch, plan); hint != "" {
					proofSourceEditReplanRetried = true
					existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
					if existing != "" {
						existing += "\n\n"
					}
					o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + hint))
					o.busCtx.TaskState.LastError = ""
					logging.Warning("[orchestrator] pure proof follow-up tried to edit production source without typed failure evidence; retrying bounded planning once paths=%s", strings.Join(paths, ","))
					continue
				}
			} else if hint, paths := proofFollowupProductionEditWithoutFailureHint(o.busCtx.Mutable, batch, plan); hint != "" {
				return fmt.Errorf("pure proof follow-up ChangePlan edited production source without typed failure evidence in %s", strings.Join(paths, ","))
			}
			if !proofNonExecutableReplanRetried {
				if hint, paths := proofFollowupNonExecutableChangeHint(batch, plan); hint != "" {
					proofNonExecutableReplanRetried = true
					existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
					if existing != "" {
						existing += "\n\n"
					}
					o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + hint))
					o.busCtx.TaskState.LastError = ""
					logging.Warning("[orchestrator] proof-follow-up plan only added non-executable production comments; retrying bounded planning once paths=%s", strings.Join(paths, ","))
					continue
				}
			} else if hint, paths := proofFollowupNonExecutableChangeHint(batch, plan); hint != "" {
				return fmt.Errorf("proof follow-up ChangePlan only added non-executable production comments in %s", strings.Join(paths, ","))
			}
			return nil
		}
		if plannerProbePassedExistingAppliedWorktree(o.busCtx.Mutable, priorPlan) {
			o.busCtx.Mutable.SetChangePlan(priorPlan)
			o.busCtx.TaskState.LastError = ""
			logging.Info("[orchestrator] controller replan produced no plan but planner dry-run probe passed; restoring plan %s for post-apply verify", priorPlan.ID)
			return errPlannerProbePassedExistingWorktree
		}
		// A planning round that ends with no ChangePlan installed is not
		// immediately terminal while typed planning context is waiting:
		// grant bounded re-dispatches whose planning hint cites the empty
		// round. A second re-dispatch is reserved for structured plan-emit
		// validation failures (for example duplicate FileChange entries or
		// old_text mismatch). The condition reads typed state/tool results
		// only, never model prose.
		rejection := lastPlanEmitRejectionView(o.busCtx.Mutable.DispatchToolResults())
		rejectionHint := rejection.RenderHint()
		if o.busCtx.Mutable.ChangePlan() == nil &&
			noPlanRetryCount < controllerNoPlanRetryBudget(o.busCtx.Mutable, batch, rejectionHint) &&
			controllerNoPlanRetryEligible(o.busCtx.Mutable, batch) {
			if cerr := o.checkCanceled("write_controller_plan_retry", *stepsUsed); cerr != nil {
				o.busCtx.TaskState.LastError = cerr.Error()
				return cerr
			}
			noPlanRetryCount++
			o.busCtx.TaskState.LastError = ""
			hint := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
			if hint != "" {
				hint += "\n\n"
			}
			hint += "The previous planning round ended without a stored change plan. Emit exactly one bounded ChangePlan via emit_change_plan using the typed planning context above; do not finish the round without the emit."
			if o.busCtx.Mutable.VerifyFailureHandoff() != nil {
				hint += " Address the verification failure section above."
			}
			if o.busCtx.Mutable.WriteExplorationHandoff() != nil {
				hint += " Preserve the exploration findings and evidence refs already supplied to this planning batch."
			}
			if controllerHasBatchOrAnalysisAnchors(o.busCtx.Mutable, batch) {
				hint += " Use the batch expected paths and WriteAnalysisIR scope anchors as the source boundary; only call read_file for exact current bytes or line ranges needed to construct the patch."
			}
			if rejectionHint != "" {
				hint += "\n\nThe previous round's plan emit was rejected by validation. Correct it in the re-emit using this bounded typed repair input:\n" + rejectionHint
			}
			o.busCtx.Mutable.SetPlanningHint(hint)
			logging.Warning("[orchestrator] controller planning round produced no plan; granting re-dispatch %d/%d with typed planning context",
				noPlanRetryCount, controllerNoPlanRetryBudget(o.busCtx.Mutable, batch, rejectionHint))
			continue
		}
		if err != nil {
			return err
		}
		return errors.New("write controller plan_batch produced no ChangePlan")
	}
}

func replanReusedFailedPlan(priorFingerprint string, plan *types.ChangePlan) bool {
	priorFingerprint = strings.TrimSpace(priorFingerprint)
	if priorFingerprint == "" || plan == nil {
		return false
	}
	return types.PlanFingerprint(plan) == priorFingerprint
}

func changePlanIsNoChangeRequired(plan *types.ChangePlan) bool {
	return plan != nil &&
		strings.TrimSpace(plan.Status) == types.PlanStatusNoChangeRequired &&
		len(plan.Changes) == 0
}

func changePlanIsProofProbeOnly(plan *types.ChangePlan) bool {
	return changePlanIsNoChangeRequired(plan) && len(plan.VerificationProbes) > 0
}

func activeBatchProofFollowupPurpose(run *types.WriteWorkflowRun) bool {
	if run == nil {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeID {
			continue
		}
		switch strings.TrimSpace(batch.Purpose) {
		case "verification_proof_followup", "impact_and_verification_proof_followup":
			return true
		default:
			return false
		}
	}
	return false
}

func activeBatchOptionalFollowupPurpose(run *types.WriteWorkflowRun) bool {
	if run == nil {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeID {
			continue
		}
		switch strings.TrimSpace(batch.Purpose) {
		case "verification_proof_followup", "impact_and_verification_proof_followup", "impact_obligation_followup":
			return true
		default:
			return false
		}
	}
	return false
}

func plannerProbePassedExistingAppliedWorktree(mu *types.MutableState, priorPlan *types.ChangePlan) bool {
	if mu == nil {
		return false
	}
	return writeflow.QualifyNoChangeReplanSentinel(writeflow.NoChangeReplanQualificationInput{
		VerifyFailureHandoff: mu.VerifyFailureHandoff(),
		PriorPlan:            priorPlan,
		PlannerProbeReports:  mu.PlanStageProbeReports(),
		RequireAppliedWork:   true,
	}).Allowed
}

func controllerNoPlanRetryEligible(mu *types.MutableState, batch *writeflow.WriteBatchPlan) bool {
	if mu == nil {
		return false
	}
	if mu.VerifyFailureHandoff() != nil || mu.WriteExplorationHandoff() != nil {
		return true
	}
	return controllerHasBatchOrAnalysisAnchors(mu, batch)
}

func controllerNoPlanRetryBudget(mu *types.MutableState, batch *writeflow.WriteBatchPlan, latestEmitRejection string) int {
	if !controllerNoPlanRetryEligible(mu, batch) {
		return 0
	}
	// One empty planning round is often a harmless exploration spillover.
	// Give one additional bounded retry when the latest round produced
	// successful read-only tool observations: the planner is still turning
	// typed handoff into exact bytes, not free-running on prose. Structured
	// emit validation failures get the same second correction dispatch.
	// Inputs are tool-result records only.
	if strings.TrimSpace(latestEmitRejection) != "" {
		return 2
	}
	if planningRoundHasReadOnlyProgress(mu.DispatchToolResults()) {
		return 2
	}
	return 1
}

func planningRoundHasReadOnlyProgress(results []types.ToolResult) bool {
	for i := len(results) - 1; i >= 0; i-- {
		r := results[i]
		if !r.Success {
			continue
		}
		switch types.CanonicalToolName(r.ToolName) {
		case "read_file", "grep", "list_files", "repo_map", "git_diff", "git_log", "git_history_search":
			return true
		}
	}
	return false
}

func (o *Orchestrator) syncPlannerObservationContextPack(batch *writeflow.WriteBatchPlan) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	results := o.busCtx.Mutable.DispatchToolResults()
	if len(results) == 0 {
		return
	}
	batchID := "batch-1"
	goal := ""
	if batch != nil {
		normalized := writeflow.NormalizeBatchPlan(*batch)
		if strings.TrimSpace(normalized.ID) != "" {
			batchID = strings.TrimSpace(normalized.ID)
		}
		goal = strings.TrimSpace(normalized.Goal)
	}
	if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
		if strings.TrimSpace(run.ActiveBatchID) != "" {
			batchID = strings.TrimSpace(run.ActiveBatchID)
		}
		if goal == "" {
			goal = strings.TrimSpace(run.Goal)
		}
	}
	pack := types.WriteContextPackFromPlannerToolResults(batchID, goal, results)
	if o.busCtx.AnalysisIR != nil {
		policy := types.CompileRepoMapNavigationPolicy(
			o.busCtx.AnalysisIR.RequestModel,
			&o.busCtx.AnalysisIR.AnswerContract,
			o.busCtx.ExploreLanePlan,
		)
		review := types.SourceLocalizationReviewFromWriteContextPacks([]types.WriteContextPack{pack}, types.WriteConsumerPlanner, batchID, "")
		policy = types.RepoMapNavigationPolicyWithLocalizationReview(policy, &review)
		coverage := types.RepoMapNavigationCoverageFromToolResults(policy, results)
		coveragePack := types.WriteContextPackFromRepoMapNavigationCoverage(batchID, goal, coverage)
		pack = types.MergeWriteContextPacks(batchID, goal, pack, coveragePack)
	}
	if len(pack.Items) == 0 {
		return
	}
	if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
		updated := types.CloneWriteWorkflowRun(*run)
		activeBatchID := strings.TrimSpace(updated.ActiveBatchID)
		activeSliceID := activeWorkflowRunSliceID(updated, activeBatchID)
		if activeBatchID != "" {
			pack = pack.WithScope(activeBatchID, activeSliceID)
		}
		updated = upsertWorkflowRunContextPack(updated, pack)
		o.busCtx.Mutable.SetWriteWorkflowRun(&updated)
		return
	}
	o.busCtx.Mutable.SetWriteContextPack(&pack)
}

func controllerHasBatchOrAnalysisAnchors(mu *types.MutableState, batch *writeflow.WriteBatchPlan) bool {
	if batch != nil {
		normalized := writeflow.NormalizeBatchPlan(*batch)
		if len(normalized.ExpectedPaths) > 0 || len(normalized.ExploreTargets) > 0 {
			return true
		}
	}
	if mu == nil {
		return false
	}
	ir := mu.WriteAnalysisIR()
	if ir == nil {
		return false
	}
	return len(ir.Request.ScopeAnchors) > 0
}

func (o *Orchestrator) localizationCoverageReplanHint(plan *types.ChangePlan) (string, []string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return "", nil
	}
	ir := plan.WriteAnalysisIR
	if ir == nil {
		ir = o.busCtx.Mutable.WriteAnalysisIR()
	}
	if ir != nil && (ir.Request.Risk.ChangesBuildSystem || ir.Request.Task.Kind == types.WriteTaskConfig) {
		return "", nil
	}
	prior := o.localizationRequirementPriorContextPacks()
	requirements := types.LocalizationRequirementsFromWritePlanContext("", "", types.WriteConsumerPlanner, prior, plan, 0)
	missing := types.WritePlanSourcePathsOutsidePriorContext(prior, plan)
	if len(missing) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("## Typed localization coverage critique\n\n")
	b.WriteString("The previous ChangePlan edited source paths that were not present in prior P0/P1 localization context. Replan this same batch once using the evidence-backed owner paths where possible.\n\n")
	b.WriteString("Source paths missing prior localization evidence:\n")
	for _, p := range missing {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	if rows := types.LocalizationRequirementEvidenceRows(requirements, 8); len(rows) > 0 {
		b.WriteString("\nTyped localization requirements:\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "- %s\n", row)
		}
	}
	if ir != nil && len(ir.Request.ScopeAnchors) > 0 {
		b.WriteString("\nCurrent WriteAnalysisIR scope anchors:\n")
		for _, p := range normalizedWorkflowScopeAnchors(ir.Request.ScopeAnchors) {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	b.WriteString("\nKeep a new source path only if direct current-repo read/repomap evidence in this planning round proves it is the owner boundary; otherwise split the new owner investigation into a later explore/replan batch. Test-only paths are excluded from this critique.")
	return strings.TrimSpace(b.String()), missing
}

func (o *Orchestrator) localizationOwnerDepthReplanHint(plan *types.ChangePlan) (string, []string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return "", nil
	}
	ir := plan.WriteAnalysisIR
	if ir == nil {
		ir = o.busCtx.Mutable.WriteAnalysisIR()
	}
	if ir != nil && (ir.Request.Risk.ChangesBuildSystem || ir.Request.Task.Kind == types.WriteTaskConfig) {
		return "", nil
	}
	requirements := types.LocalizationRequirementsFromWritePlanContext("", "", types.WriteConsumerPlanner, o.localizationRequirementPriorContextPacks(), plan, 0)
	missing := requirements.MissingOwnerAnchorPaths
	if len(missing) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("## Typed owner localization depth critique\n\n")
	b.WriteString("The previous ChangePlan edited source paths that were covered only by broad/supporting context. Replan this same batch once with typed owner localization where possible; a broad scope path or supporting evidence is not owner proof.\n\n")
	b.WriteString("Source paths missing typed owner localization anchors:\n")
	for _, p := range missing {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	if rows := types.LocalizationRequirementEvidenceRows(requirements, 8); len(rows) > 0 {
		b.WriteString("\nTyped localization requirements:\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "- %s\n", row)
		}
	}
	if ir != nil && len(ir.Request.ScopeAnchors) > 0 {
		b.WriteString("\nCurrent WriteAnalysisIR scope anchors:\n")
		for _, p := range normalizedWorkflowScopeAnchors(ir.Request.ScopeAnchors) {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	b.WriteString("\nIf the path remains correct, preserve the patch and add exact current-repo read/repomap evidence for the enclosing owner symbol before re-emitting the bounded ChangePlan. If owner evidence points elsewhere, move or split the fix accordingly. If this remains unresolved after bounded owner exploration, apply pauses before mutating the worktree.")
	return strings.TrimSpace(b.String()), missing
}

func (o *Orchestrator) localizationRequirementPriorContextPacks() []types.WriteContextPack {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	var prior []types.WriteContextPack
	if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
		prior = append(prior, run.ContextPacks...)
	}
	if pack := o.busCtx.Mutable.WriteContextPack(); pack != nil {
		prior = append(prior, *pack)
	}
	return prior
}

func (o *Orchestrator) runOwnerLocalizationRequirementExplorationBeforeApply(run *types.WriteWorkflowRun, plan *types.ChangePlan, stepsUsed *int) (bool, error) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || plan == nil || stepsUsed == nil {
		return false, nil
	}
	if changePlanIsNoChangeRequired(plan) || changePlanIsProofProbeOnly(plan) {
		return false, nil
	}
	batchID := strings.TrimSpace(run.ActiveBatchID)
	if batchID == "" ||
		!writeWorkflowRunHasProgressReasonForBatch(*run, batchID, "owner_localization_depth_replan_requested") ||
		writeWorkflowRunHasProgressReasonForBatch(*run, batchID, "owner_localization_requirement_explored") ||
		writeWorkflowRunHasProgressReasonForBatch(*run, batchID, "owner_localization_requirement_degraded") {
		return false, nil
	}
	requirements := types.LocalizationRequirementsFromWritePlanContext(
		batchID,
		activeWorkflowRunSliceID(*run, batchID),
		types.WriteConsumerPlanner,
		o.localizationRequirementPriorContextPacks(),
		plan,
		0,
	)
	rows := types.LocalizationRequirementEvidenceRows(types.LocalizationRequirementSet{Items: localizationRequirementOwnerOpenItems(requirements)}, 8)
	if len(rows) == 0 {
		return false, nil
	}
	candidatePaths := requirements.CandidatePaths
	if len(candidatePaths) == 0 {
		candidatePaths = append([]string(nil), requirements.MissingOwnerAnchorPaths...)
	}
	goal := strings.TrimSpace(activeWorkflowBatchGoal(*run, batchID))
	if goal == "" {
		goal = strings.TrimSpace(plan.Summary)
	}
	req := types.WriteExplorationRequest{
		BatchID:              batchID,
		Goal:                 goal,
		CandidatePaths:       candidatePaths,
		EvidenceRequirements: rows,
	}
	o.busCtx.Mutable.SetWriteExplorationRequest(&req)
	updateWorkflowRunBatchStatus(run, batchID, types.WriteWorkflowBatchNeedsExploration)
	clearWorkflowRunActiveBatchPlanRefs(run, batchID)
	appendControllerProgress(run, batchID, "owner_localization_requirement_explore_requested",
		"open typed owner-localization requirement requires one bounded read-only exploration before apply")
	o.persistWriteWorkflowRun(run)
	runner := o.readExplorationRunner
	if runner == nil {
		runner = defaultReadExplorationRunner{}
	}
	used, err := runner.Run(o)
	*stepsUsed += used
	if err != nil {
		logging.Warning("[orchestrator] owner-localization requirement exploration degraded: %v", err)
		updateWorkflowRunBatchExplore(run, batchID, "degraded", err.Error())
		appendControllerProgress(run, batchID, "owner_localization_requirement_degraded", err.Error())
	} else {
		updateWorkflowRunBatchExplore(run, batchID, "complete", "")
		appendControllerProgress(run, batchID, "owner_localization_requirement_explored", strings.Join(requirements.MissingOwnerAnchorPaths, ","))
	}
	updateWorkflowRunBatchStatus(run, batchID, types.WriteWorkflowBatchReadyToPlan)
	o.syncCurrentWriteContextPackToRun(run)
	o.persistWriteWorkflowRun(run)
	o.busCtx.Mutable.ResetChangePlan()
	o.busCtx.TaskState.LastError = ""
	return true, err
}

func (o *Orchestrator) runOwnerLocalizationAuthorityGateBeforeApply(run *types.WriteWorkflowRun, plan *types.ChangePlan, stepsUsed *int) (writeControllerApplyTransitionOutcome, bool, error) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || plan == nil || stepsUsed == nil {
		return "", false, nil
	}
	if changePlanIsNoChangeRequired(plan) || changePlanIsProofProbeOnly(plan) {
		return "", false, nil
	}
	batchID := strings.TrimSpace(run.ActiveBatchID)
	if batchID == "" || !writeWorkflowRunHasProgressReasonForBatch(*run, batchID, "owner_localization_depth_replan_requested") {
		return "", false, nil
	}
	if !writeWorkflowRunHasProgressReasonForBatch(*run, batchID, "owner_localization_requirement_explored") &&
		!writeWorkflowRunHasProgressReasonForBatch(*run, batchID, "owner_localization_requirement_degraded") {
		explored, err := o.runOwnerLocalizationRequirementExplorationBeforeApply(run, plan, stepsUsed)
		if explored {
			return writeControllerApplyTransitionRetry, true, err
		}
	}
	o.syncPlanEditOwnerAnchorsToRun(run, plan)
	requirements := types.LocalizationRequirementsFromWritePlanContext(
		batchID,
		activeWorkflowRunSliceID(*run, batchID),
		types.WriteConsumerPlanner,
		o.localizationRequirementPriorContextPacks(),
		plan,
		0,
	)
	openOwnerItems := localizationRequirementOwnerOpenItems(requirements)
	if len(openOwnerItems) == 0 {
		return "", false, nil
	}
	rows := types.LocalizationRequirementEvidenceRows(types.LocalizationRequirementSet{Items: openOwnerItems}, 8)
	if len(rows) == 0 {
		return "", false, nil
	}
	if !writeWorkflowRunHasProgressReasonForBatch(*run, batchID, "owner_localization_authority_replan_requested") {
		existing := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
		if existing != "" {
			existing += "\n\n"
		}
		o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(existing + renderOwnerLocalizationAuthorityGateHint(rows, requirements.MissingOwnerAnchorPaths)))
		updateWorkflowRunBatchStatus(run, batchID, types.WriteWorkflowBatchReadyToPlan)
		clearWorkflowRunActiveBatchPlanRefs(run, batchID)
		appendControllerProgress(run, batchID, "owner_localization_authority_replan_requested",
			strings.Join(requirements.MissingOwnerAnchorPaths, ","))
		o.persistWriteWorkflowRun(run)
		o.busCtx.Mutable.ResetChangePlan()
		o.busCtx.TaskState.LastError = ""
		return writeControllerApplyTransitionRetry, true, nil
	}
	run.Status = types.WriteWorkflowRunBlocked
	updateWorkflowRunBatchStatus(run, batchID, types.WriteWorkflowBatchBlocked)
	appendControllerProgress(run, batchID, "owner_localization_authority_unresolved_blocked",
		strings.Join(requirements.MissingOwnerAnchorPaths, ","))
	o.persistWriteWorkflowRun(run)
	o.publishBlockedRunGuidance(run, "owner_localization_authority_unresolved")
	return writeControllerApplyTransitionTerminal, true, fmt.Errorf("write workflow blocked: source owner localization unresolved for %s", strings.Join(requirements.MissingOwnerAnchorPaths, ", "))
}

func (o *Orchestrator) syncPlanEditOwnerAnchorsToRun(run *types.WriteWorkflowRun, plan *types.ChangePlan) {
	if o == nil || o.busCtx == nil || run == nil || plan == nil {
		return
	}
	repoRoot := strings.TrimSpace(o.busCtx.RepoRoot)
	if wt := strings.TrimSpace(o.busCtx.WorktreePath); wt != "" {
		repoRoot = wt
	}
	if repoRoot == "" {
		repoRoot = strings.TrimSpace(o.busCtx.MainRepoRoot)
	}
	if repoRoot == "" {
		return
	}
	pack := buildPlanEditOwnerContextPack(repoRoot, strings.TrimSpace(run.ActiveBatchID), activeWorkflowRunSliceID(*run, run.ActiveBatchID), plan)
	if len(pack.Items) == 0 {
		return
	}
	*run = upsertWorkflowRunContextPack(*run, pack)
	if o.busCtx.Mutable != nil {
		o.busCtx.Mutable.SetWriteWorkflowRun(run)
	}
}

func buildPlanEditOwnerContextPack(repoRoot, batchID, sliceID string, plan *types.ChangePlan) types.WriteContextPack {
	if plan == nil || strings.TrimSpace(repoRoot) == "" {
		return types.WriteContextPack{}
	}
	pack := types.WriteContextPack{
		PackID:      "plan-edit-owner-" + strings.TrimSpace(plan.ID),
		BatchID:     strings.TrimSpace(batchID),
		SourceStage: "plan_edit_owner",
		Goal:        strings.TrimSpace(plan.Summary),
	}
	seen := map[string]struct{}{}
	contentCache := map[string][]byte{}
	contentForPath := func(rel string) ([]byte, bool) {
		if rel == "" || strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			return nil, false
		}
		if content, ok := contentCache[rel]; ok {
			return content, len(content) > 0
		}
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil || len(content) == 0 {
			return nil, false
		}
		contentCache[rel] = content
		return content, true
	}
	appendOwnerItem := func(rel string, line int, reasonCode, summaryPrefix string) {
		if rel == "" || strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			return
		}
		content, ok := contentForPath(rel)
		if !ok {
			return
		}
		anchor, ok := sourceowner.FindEnclosingOwner(rel, content, line)
		if !ok || anchor.OwnerSymbol == "" {
			return
		}
		key := rel + "|" + strconv.Itoa(anchor.Line) + "|" + anchor.OwnerSymbol + "|" + anchor.AnchorSymbol
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		ref := types.WriteExplorationEvidenceRef{
			ID:           "plan-edit-owner|" + strings.ReplaceAll(key, "|", ":"),
			Kind:         "plan_edit_owner",
			Source:       rel,
			LineStart:    anchor.Line,
			LineEnd:      anchor.Line,
			Subject:      rel + ":" + anchor.OwnerSymbol,
			OwnerSymbol:  anchor.OwnerSymbol,
			AnchorSymbol: anchor.AnchorSymbol,
			Summary:      summaryPrefix + " is enclosed by " + anchor.OwnerSymbol,
		}
		locAnchor := types.SourceLocalizationAnchor{
			Path:         rel,
			Role:         types.ClassifySourcePathRole(rel),
			SourceStage:  "plan_edit_owner",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			EvidenceRef:  &ref,
			Subject:      ref.Subject,
			OwnerSymbol:  anchor.OwnerSymbol,
			AnchorSymbol: anchor.AnchorSymbol,
			ReasonCode:   reasonCode,
		}
		item := types.WriteContextItem{
			ID:                 "plan_edit_owner|" + strings.ReplaceAll(key, "|", ":"),
			Priority:           types.WriteContextP1,
			Kind:               "localization_anchor",
			Text:               "path=" + rel + " owner=" + anchor.OwnerSymbol + " anchor=" + anchor.AnchorSymbol + " line=" + strconv.Itoa(anchor.Line) + " reason=" + reasonCode,
			SourceStage:        "plan_edit_owner",
			SourceID:           strings.TrimSpace(plan.ID),
			BatchID:            strings.TrimSpace(batchID),
			SliceID:            strings.TrimSpace(sliceID),
			Consumers:          []types.WriteContextConsumer{types.WriteConsumerController, types.WriteConsumerPlanner, types.WriteConsumerVerifier},
			EvidenceRef:        &ref,
			LocalizationAnchor: &locAnchor,
		}
		pack.Items = append(pack.Items, item)
	}
	for _, change := range plan.Changes {
		rel := filepath.ToSlash(strings.TrimSpace(change.Path))
		content, ok := contentForPath(rel)
		if !ok {
			continue
		}
		for _, line := range planEditOwnerLineCandidates(change, content) {
			appendOwnerItem(rel, line, "plan_edit_structural_owner", "plan edit line")
		}
	}
	for rel, lines := range planEditOwnerPatchEffectLineCandidates(plan.PatchEffect) {
		for _, line := range lines {
			appendOwnerItem(rel, line, "actual_diff_structural_owner", "actual patch line")
		}
	}
	return types.NormalizeWriteContextPack(pack)
}

func planEditOwnerLineCandidates(change types.FileChange, content []byte) []int {
	seen := map[int]struct{}{}
	var out []int
	add := func(line int) {
		if line <= 0 {
			return
		}
		if _, ok := seen[line]; ok {
			return
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	totalLines := len(strings.Split(string(content), "\n"))
	for _, edit := range change.Edits {
		switch strings.TrimSpace(edit.Kind) {
		case "insert_at_eof":
			add(totalLines)
		case "insert_before_final_brace":
			add(lastStandaloneBraceLine(content))
		default:
			add(edit.StartLine)
		}
	}
	for _, line := range unifiedDiffOldStartLines(change.Patch) {
		add(line)
	}
	return out
}

func planEditOwnerPatchEffectLineCandidates(effect *types.PatchEffectRecord) map[string][]int {
	out := map[string][]int{}
	if effect == nil {
		return out
	}
	seen := map[string]map[int]struct{}{}
	add := func(path string, line int) {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || line <= 0 {
			return
		}
		if seen[path] == nil {
			seen[path] = map[int]struct{}{}
		}
		if _, ok := seen[path][line]; ok {
			return
		}
		seen[path][line] = struct{}{}
		out[path] = append(out[path], line)
	}
	for _, file := range effect.Files {
		path := normalizeControllerPath(file.Path)
		if path == "" {
			path = normalizeControllerPath(file.OldPath)
		}
		if path == "" {
			continue
		}
		for _, hunk := range file.Hunks {
			for _, line := range hunk.AddedLineNumbers {
				add(path, line)
			}
		}
	}
	for path := range out {
		sort.Ints(out[path])
	}
	return out
}

func unifiedDiffOldStartLines(patch string) []int {
	var out []int
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "@@") {
			continue
		}
		start := strings.Index(line, "-")
		if start < 0 {
			continue
		}
		rest := line[start+1:]
		end := strings.IndexAny(rest, " ,")
		if end < 0 {
			continue
		}
		n, err := strconv.Atoi(rest[:end])
		if err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

func lastStandaloneBraceLine(content []byte) int {
	lines := strings.Split(string(content), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "}" {
			return i + 1
		}
	}
	return len(lines)
}

func renderOwnerLocalizationAuthorityGateHint(rows, paths []string) string {
	var b strings.Builder
	b.WriteString("## Typed owner localization authority gate\n\n")
	b.WriteString("The current ChangePlan still has open owner-localization requirements after bounded owner exploration. Replan this batch before apply. Supporting evidence can guide exploration, but only typed production anchors with strength=owner satisfy this gate.\n\n")
	if len(paths) > 0 {
		b.WriteString("Source paths still missing typed owner authority:\n")
		for _, p := range paths {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	b.WriteString("\nOpen typed requirements:\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "- %s\n", row)
	}
	b.WriteString("\nMove the patch to an owner-backed source path, split a smaller owner-discovery batch, or emit a no-change sentinel only if the typed evidence proves no source edit is required.")
	return strings.TrimSpace(b.String())
}

func localizationRequirementOwnerOpenItems(set types.LocalizationRequirementSet) []types.LocalizationRequirement {
	set = types.NormalizeLocalizationRequirementSet(set, 0)
	var out []types.LocalizationRequirement
	for _, item := range set.Items {
		if item.Status == types.LocalizationRequirementOpen && item.Kind == types.LocalizationRequirementOwnerEvidence {
			out = append(out, item)
		}
	}
	return out
}

func writeWorkflowRunHasProgressReasonForBatch(run types.WriteWorkflowRun, batchID, reasonCode string) bool {
	batchID = strings.TrimSpace(batchID)
	reasonCode = strings.TrimSpace(reasonCode)
	if batchID == "" || reasonCode == "" {
		return false
	}
	for _, event := range run.ProgressLedger {
		if strings.TrimSpace(event.BatchID) == batchID && strings.TrimSpace(event.ReasonCode) == reasonCode {
			return true
		}
	}
	return false
}

func activeWorkflowBatchGoal(run types.WriteWorkflowRun, batchID string) string {
	batchID = strings.TrimSpace(batchID)
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == batchID {
			return strings.TrimSpace(batch.Goal)
		}
	}
	return strings.TrimSpace(run.Goal)
}

func clearWorkflowRunActiveBatchPlanRefs(run *types.WriteWorkflowRun, batchID string) {
	if run == nil {
		return
	}
	batchID = strings.TrimSpace(batchID)
	for i := range run.Batches {
		if strings.TrimSpace(run.Batches[i].ID) != batchID {
			continue
		}
		run.Batches[i].PlanID = ""
		run.Batches[i].PlanRef = ""
		run.Batches[i].UpdatedAt = time.Now().UTC()
		for j := range run.Batches[i].Slices {
			if strings.TrimSpace(run.Batches[i].Slices[j].PlanID) != "" {
				run.Batches[i].Slices[j].PlanID = ""
			}
		}
		return
	}
}

func (o *Orchestrator) offScopeHighRiskReplanHint(plan *types.ChangePlan) (string, []string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return "", nil
	}
	ir := plan.WriteAnalysisIR
	if ir == nil {
		ir = o.busCtx.Mutable.WriteAnalysisIR()
	}
	if ir == nil {
		return "", nil
	}
	if ir.Request.Risk.ChangesBuildSystem || ir.Request.Task.Kind == types.WriteTaskConfig {
		return "", nil
	}
	anchors := normalizedWorkflowScopeAnchors(ir.Request.ScopeAnchors)
	if len(anchors) == 0 {
		return "", nil
	}
	policy := o.writeApprovalPolicy
	if policy == "" {
		policy = writeflow.ApprovalPolicyAutoSafe
	}
	assessment := writeflow.AssessWriteRisk(o.writeRiskAssessmentInput(plan))
	decision := writeflow.DecideWriteApproval(policy, assessment)
	if decision.Action != writeflow.ApprovalActionManual || assessment.Level != writeflow.RiskHigh {
		return "", nil
	}
	offScope := map[string]bool{}
	highReasons := 0
	for _, reason := range assessment.Reasons {
		switch reason.Level {
		case writeflow.RiskCritical:
			return "", nil
		case writeflow.RiskHigh:
		default:
			continue
		}
		clean, ok := normalizeWorkflowScopePath(reason.Path)
		if !ok {
			return "", nil
		}
		highReasons++
		if workflowPathWithinAnyScope(clean, anchors) {
			return "", nil
		}
		offScope[clean] = true
	}
	if highReasons == 0 || len(offScope) == 0 {
		return "", nil
	}
	paths := make([]string, 0, len(offScope))
	for p := range offScope {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	sort.Strings(anchors)
	var b strings.Builder
	b.WriteString("## Typed plan-risk critique\n\n")
	b.WriteString("The previous ChangePlan touched high-risk paths outside the WriteAnalysisIR scope anchors. Replan this same batch without those off-scope high-risk paths unless a later typed exploration result expands the scope.\n\n")
	b.WriteString("Off-scope high-risk paths to exclude:\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	b.WriteString("\nCurrent scope anchors:\n")
	for _, p := range anchors {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	b.WriteString("\nKeep the next ChangePlan bounded to the scope anchors and necessary same-scope tests. If a high-risk build/config change is truly required, produce the source/test plan first and let the approval boundary handle the separate high-risk batch.")
	return strings.TrimSpace(b.String()), paths
}

func normalizedWorkflowScopeAnchors(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, p := range paths {
		clean, ok := normalizeWorkflowScopePath(p)
		if !ok || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func normalizeWorkflowScopePath(p string) (string, bool) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return "", false
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean == "" {
		return "", false
	}
	return clean, true
}

func workflowPathWithinAnyScope(path string, anchors []string) bool {
	for _, anchor := range anchors {
		if anchor == "." {
			return true
		}
		trimmed := strings.TrimSuffix(anchor, "/")
		if path == trimmed || strings.HasPrefix(path, trimmed+"/") {
			return true
		}
	}
	return false
}

func changePlanHasAppliedWork(plan *types.ChangePlan) bool {
	if plan == nil {
		return false
	}
	if strings.TrimSpace(plan.AppliedCommitSHA) != "" || strings.TrimSpace(plan.WorktreePath) != "" {
		return true
	}
	for _, change := range plan.Changes {
		if change.Apply != nil && strings.TrimSpace(change.Apply.Status) == "applied" {
			return true
		}
	}
	return false
}

func (o *Orchestrator) appliedPatchInterruptedShouldBlock(run *types.WriteWorkflowRun, err error) bool {
	if err == nil {
		return false
	}
	if !errors.Is(err, ErrCanceled) && !errors.Is(err, context.Canceled) {
		return true
	}
	if !o.cancellationSourceIsWriteDeadline(err) {
		return false
	}
	return !o.appliedPatchInterruptedCanResume(run, err)
}

func (o *Orchestrator) cancellationSourceIsWriteDeadline(err error) bool {
	if err == nil {
		return false
	}
	if !errors.Is(err, ErrCanceled) && !errors.Is(err, context.Canceled) {
		return false
	}
	if o == nil || o.cancelToken == nil {
		return false
	}
	return o.cancelToken.Source() == CancelSourceWriteDeadline
}

func (o *Orchestrator) appliedPatchInterruptedCanResume(run *types.WriteWorkflowRun, err error) bool {
	if run == nil || err == nil {
		return false
	}
	if !errors.Is(err, ErrCanceled) && !errors.Is(err, context.Canceled) {
		return false
	}
	if !writeWorkflowActiveBatchHasFailedVerify(run) {
		return false
	}
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	return activeBatchHasVerifyFailureHandoff(o.busCtx.Mutable, run.ActiveBatchID)
}

func (o *Orchestrator) controllerDispatchInterruptedCanResumeRepairPlan(run *types.WriteWorkflowRun, plan *types.ChangePlan, err error) bool {
	if run == nil || plan == nil {
		return false
	}
	if !o.cancellationSourceIsWriteDeadline(err) {
		return false
	}
	if !changePlanReadyForApply(plan) || writePlanDeniedByApproval(plan) {
		return false
	}
	if !writeWorkflowActiveBatchHasFailedVerify(run) {
		return false
	}
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	if !activeBatchHasVerifyFailureHandoff(o.busCtx.Mutable, run.ActiveBatchID) {
		return false
	}
	active, ok := activeWriteWorkflowControllerBatch(run)
	if !ok {
		return false
	}
	if planID := strings.TrimSpace(active.PlanID); planID != "" && strings.TrimSpace(plan.ID) != "" && planID != strings.TrimSpace(plan.ID) {
		return false
	}
	switch active.Status {
	case types.WriteWorkflowBatchPlanned, types.WriteWorkflowBatchReadyToPlan, types.WriteWorkflowBatchPendingApproval:
		return true
	default:
		return false
	}
}

func writeWorkflowActiveBatchHasFailedVerify(run *types.WriteWorkflowRun) bool {
	if run == nil {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeID {
			continue
		}
		for i := len(batch.Attempts) - 1; i >= 0; i-- {
			attempt := batch.Attempts[i]
			if strings.TrimSpace(attempt.Kind) != "verify" {
				continue
			}
			return strings.TrimSpace(attempt.Status) == "failed"
		}
		return false
	}
	return false
}

func writeWorkflowActiveBatchHasAppliedAttempt(run *types.WriteWorkflowRun) bool {
	if run == nil {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeID {
			continue
		}
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) == "apply" && strings.TrimSpace(attempt.Status) == "applied" {
				return true
			}
		}
		return false
	}
	return false
}

func (o *Orchestrator) publishAppliedPatchInterruptedGuidance(run *types.WriteWorkflowRun, plan *types.ChangePlan, err error, reasonCode string) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || plan == nil {
		return false
	}
	if !changePlanHasAppliedWork(plan) {
		return false
	}
	msg := o.appliedPatchInterruptedMessage(run, plan, o.busCtx.Mutable.ChangeReport(), err)
	if strings.TrimSpace(msg) == "" {
		return false
	}
	o.busCtx.Mutable.SetResultPlain(msg)
	o.busCtx.TaskState.LastError = msg
	appendControllerProgress(run, run.ActiveBatchID, reasonCode, "applied patch preserved; workflow interrupted before a replacement plan or terminal finish")
	return true
}

func (o *Orchestrator) appliedPatchInterruptedMessage(run *types.WriteWorkflowRun, plan *types.ChangePlan, report *types.ChangeReport, err error) string {
	zh := false
	if o != nil && o.busCtx != nil {
		zh = isChineseLang(o.busCtx.Language)
	}
	planID := strings.TrimSpace(plan.ID)
	if planID == "" {
		planID = "active"
	}
	status := strings.TrimSpace(plan.Status)
	if status == "" {
		status = "applied"
	}
	ref := ""
	if plan.ID != "" {
		ref = "refs/codrax/applied/" + plan.ID
	}
	verifyLine := appliedPatchVerifyLine(report, plan, zh)
	interrupt := ""
	if err != nil {
		interrupt = strings.TrimSpace(err.Error())
	}
	var b strings.Builder
	if zh {
		b.WriteString("写模式已中断，但当前补丁已经应用并保留；不会把这次结果报告成“没有生成 ChangePlan”。\n")
		fmt.Fprintf(&b, "- plan: `%s`\n", planID)
		fmt.Fprintf(&b, "- plan status: `%s`\n", status)
		if ref != "" {
			fmt.Fprintf(&b, "- recovery ref: `%s`，可在主仓执行 `git cherry-pick %s` 取回补丁\n", ref, ref)
		}
		if wt := strings.TrimSpace(plan.WorktreePath); wt != "" {
			fmt.Fprintf(&b, "- worktree: `%s`\n", wt)
		}
		if verifyLine != "" {
			b.WriteString("- 最新验证: " + verifyLine + "\n")
		}
		if interrupt != "" {
			b.WriteString("- 中断原因: " + interrupt + "\n")
		}
		if run != nil && strings.TrimSpace(run.RunID) != "" {
			fmt.Fprintf(&b, "- 工作流详情: REPL 中 `/workflow show %s`\n", run.RunID)
		}
		b.WriteString("后续可以继续同一目标让 controller 重新规划失败点，或先取回上面的 recovery ref。")
		return strings.TrimSpace(b.String())
	}
	b.WriteString("Write mode was interrupted, but the current patch was already applied and preserved; this run is not being reported as an empty ChangePlan.\n")
	fmt.Fprintf(&b, "- plan: `%s`\n", planID)
	fmt.Fprintf(&b, "- plan status: `%s`\n", status)
	if ref != "" {
		fmt.Fprintf(&b, "- recovery ref: `%s`; recover with `git cherry-pick %s` from the main repo\n", ref, ref)
	}
	if wt := strings.TrimSpace(plan.WorktreePath); wt != "" {
		fmt.Fprintf(&b, "- worktree: `%s`\n", wt)
	}
	if verifyLine != "" {
		b.WriteString("- latest verification: " + verifyLine + "\n")
	}
	if interrupt != "" {
		b.WriteString("- interruption: " + interrupt + "\n")
	}
	if run != nil && strings.TrimSpace(run.RunID) != "" {
		fmt.Fprintf(&b, "- workflow details: `/workflow show %s` in the REPL\n", run.RunID)
	}
	b.WriteString("Continue the same goal to let the controller replan the failed point, or recover the patch from the ref above.")
	return strings.TrimSpace(b.String())
}

func appliedPatchVerifyLine(report *types.ChangeReport, plan *types.ChangePlan, zh bool) string {
	if report != nil {
		switch report.NormalizeVerificationStatus() {
		case types.VerificationStatusPassed:
			if zh {
				return "通过"
			}
			return "passed"
		case types.VerificationStatusUnavailable:
			if zh {
				return "环境/测试不可用，代码补丁已保留"
			}
			return "unverified because the local verify environment or test surface was unavailable"
		}
		parts := []string{}
		if report.FailureKind != "" {
			parts = append(parts, string(report.FailureKind))
		}
		if report.BuildFailed {
			parts = append(parts, "build_failed")
		}
		if len(report.NoTestsRunners) > 0 {
			parts = append(parts, "unverified")
		}
		summary := truncateInline(strings.TrimSpace(report.FailureSummary), 240)
		if len(parts) == 0 {
			parts = append(parts, "failed")
		}
		if summary != "" {
			return strings.Join(parts, "/") + ": " + summary
		}
		return strings.Join(parts, "/")
	}
	switch strings.TrimSpace(plan.Status) {
	case types.PlanStatusVerifyFailed:
		if zh {
			return "未通过"
		}
		return "failed"
	case types.PlanStatusUnverified:
		if zh {
			return "环境/测试不可用，代码补丁已保留"
		}
		return "unverified because the local verify environment or test surface was unavailable"
	case types.PlanStatusApplied:
		if zh {
			return "通过"
		}
		return "passed"
	}
	return ""
}

func truncateInline(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return strings.TrimSpace(s[:max-3]) + "..."
}

func changePlanAllDeclaredChangesApplied(plan *types.ChangePlan) bool {
	if plan == nil || len(plan.Changes) == 0 {
		return false
	}
	appliedPaths := map[string]bool{}
	for _, raw := range plan.AppliedPaths {
		if p := normalizeAppliedPlanPath(raw); p != "" {
			appliedPaths[p] = true
		}
	}
	for _, change := range plan.Changes {
		if change.Apply == nil || strings.TrimSpace(change.Apply.Status) != "applied" {
			return false
		}
		if p := normalizeAppliedPlanPath(change.Path); p != "" {
			appliedPaths[p] = true
		}
		if p := normalizeAppliedPlanPath(change.NewPath); p != "" {
			appliedPaths[p] = true
		}
	}
	for _, raw := range plan.TargetPaths {
		p := normalizeAppliedPlanPath(raw)
		if p == "" {
			return false
		}
		if !appliedPaths[p] {
			return false
		}
	}
	return true
}

func normalizeAppliedPlanPath(raw string) string {
	p := strings.TrimSpace(filepath.ToSlash(raw))
	if p == "" || p == "." {
		return ""
	}
	return strings.TrimPrefix(p, "./")
}

func (o *Orchestrator) runControllerApplyPlan(stepsUsed *int) error {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return fmt.Errorf("write controller apply plan missing context")
	}
	if o.busCtx.Mutable.ChangePlan() == nil && strings.TrimSpace(o.busCtx.PlanPath) == "" {
		return fmt.Errorf("write controller apply_plan requires a ChangePlan or imported plan file")
	}
	_, err := o.runControllerWriteStage(types.StageApply, stepsUsed)
	return err
}

func (o *Orchestrator) reviewActiveAppliedPatchScope(run *types.WriteWorkflowRun, plan *types.ChangePlan) types.PatchReviewRecord {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return types.PatchReviewRecord{}
	}
	activeSlice, _ := writeflow.ActiveRuntimeUnitApplySlice(plan, run)
	o.attachActivePatchEffectRecord(plan, activeSlice)
	if run != nil {
		o.syncPlanEditOwnerAnchorsToRun(run, plan)
	}
	conventionGraph := o.conventionGraphForPatchReview(run, plan)
	review := writeflow.ReviewAppliedPatchSemantic(writeflow.SemanticPatchReviewInput{
		Plan:              plan,
		ActiveSlice:       activeSlice,
		ImpactAnalysis:    plan.ImpactAnalysis,
		ImpactObligations: plan.ImpactObligations,
		ConventionGraph:   conventionGraph,
	})
	review = types.NormalizePatchReviewRecord(review)
	plan.PatchReview = &review
	o.busCtx.Mutable.SetChangePlan(plan)
	if run != nil {
		*run = attachPlanContextPackToWorkflowRun(*run, plan)
		o.busCtx.Mutable.SetWriteWorkflowRun(run)
	}
	o.persistCurrentChangePlanSnapshot()
	return review
}

func (o *Orchestrator) appendCumulativePatchReviewFollowupIfNeeded(run *types.WriteWorkflowRun, report *types.ChangeReport, outcome writeflow.VerifyAttemptOutcome) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || report == nil {
		return false
	}
	if outcome.Kind != writeflow.VerifyOutcomeReportPassed || !report.Passed {
		return false
	}
	if !activeBatchCompletedWithNonFailedVerifierVerdict(run) {
		return false
	}
	if workflowProgressReasonCount(run, "", "cumulative_actual_diff_review_followup_requested") > 0 {
		return false
	}
	plan := o.buildCumulativePatchReviewPlan(run, report, nil)
	if plan == nil || plan.PatchReview == nil || !changePlanTouchesNonTestCode(plan) {
		return false
	}
	review := types.NormalizePatchReviewRecord(*plan.PatchReview)
	if review.HardBlock {
		_, _ = o.handlePatchReviewHardBlock(run, plan, review, "cumulative_actual_diff:"+patchReviewHardBlockReason(review))
		return true
	}
	items := selectImpactRepairQueueItems(plan, report, 0)
	items = filterPassedVerifyGraphTelemetryItems(run, run.ActiveBatchID, items)
	if len(items) == 0 {
		return false
	}
	if len(items) > 4 {
		items = items[:4]
	}
	purpose := repairFollowupPurposeForItems(items)
	expectedPaths := impactRepairExpectedPaths(items)
	criteria := impactRepairSuccessCriteria(items)
	criteria = append(criteria, impactRepairNavigationSuccessCriteria(items)...)
	batch := writeflow.WriteBatchPlan{
		ID:                   nextRepairBatchID(run, run.ActiveBatchID, "cumulative-review"),
		Goal:                 impactRepairFollowupGoal(items),
		Purpose:              purpose,
		Status:               writeflow.BatchReadyForChangePlan,
		NeedsCodeExploration: len(expectedPaths) == 0 || impactRepairNeedsGraphNavigation(items),
		ExploreTargets:       expectedPaths,
		ExpectedPaths:        expectedPaths,
		SuccessCriteria:      criteria,
	}
	oldBatchID := strings.TrimSpace(run.ActiveBatchID)
	appendControllerProgress(run, oldBatchID, "cumulative_actual_diff_review_followup_requested",
		"typed cumulative actual-diff review found uncovered source proof after a non-failed verifier attempt")
	if proofFollowupPurpose(batch.Purpose) {
		appendControllerProgress(run, oldBatchID, "verification_proof_followup_requested",
			repairFollowupProgressMessage(batch.Purpose))
	} else {
		appendControllerProgress(run, oldBatchID, "impact_obligation_followup_requested",
			repairFollowupProgressMessage(batch.Purpose))
	}
	pack := types.WriteContextPackFromChangePlan(plan).WithScope(batch.ID, "")
	o.busCtx.Mutable.MergeWriteContextPack(pack)
	next, err := writeflow.ApplyWorkflowDecisionToRun(*run, writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionAppendBatch,
		ReasonCode: "cumulative_actual_diff_review_followup",
		Reason:     "typed cumulative actual-diff review found uncovered source proof",
		Batch:      &batch,
	})
	if err != nil {
		appendControllerProgress(run, oldBatchID, "cumulative_actual_diff_review_followup_failed", err.Error())
		return false
	}
	*run = next
	o.busCtx.Mutable.SetWriteWorkflowRun(run)
	o.syncCurrentWriteContextPackToRun(run)
	return true
}

func (o *Orchestrator) buildCumulativePatchReviewPlan(run *types.WriteWorkflowRun, report *types.ChangeReport, err error) *types.ChangePlan {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return nil
	}
	wt := strings.TrimSpace(o.busCtx.WorktreePath)
	if wt == "" {
		return nil
	}
	baseRef := firstAppliedWorkflowBaseRef(run)
	if baseRef == "" {
		return nil
	}
	ownedPaths := o.cumulativeReviewOwnedPaths(run)
	if len(ownedPaths) == 0 {
		return nil
	}
	patch, captureErr := worktree.CaptureRangePatchForPaths(wt, baseRef, "HEAD", ownedPaths)
	if captureErr != nil {
		logging.Warning("[orchestrator] cumulative patch review diff capture failed: %v", captureErr)
		return nil
	}
	if strings.TrimSpace(patch) == "" {
		return nil
	}
	planID := "plan-cumulative-" + sanitizeWorkflowArtifactID(run.RunID)
	effect := writeflow.PatchEffectRecordFromUnifiedDiff(planID, "cumulative", "workflow_cumulative_owned_diff", baseRef, "HEAD", patch)
	writeflow.AnnotatePatchEffectStructuredFileParses(&effect, wt)
	graphProvider := writeimpact.GraphProviderFromSearchGraph(o.busCtx.Mutable.SearchGraph())
	writeimpact.AnnotatePatchEffectLineFeatureEvents(&effect, graphProvider)
	paths := patchEffectRecordPaths(effect)
	if len(paths) == 0 {
		return nil
	}
	current := o.busCtx.Mutable.ChangePlan()
	plan := &types.ChangePlan{
		ID:                 planID,
		Status:             types.PlanStatusApplied,
		Summary:            "cumulative actual-diff review",
		TargetPaths:        append([]string(nil), paths...),
		AppliedPaths:       append([]string(nil), paths...),
		Changes:            patchEffectFileChanges(effect),
		PatchEffect:        &effect,
		BehaviorContracts:  nil,
		VerificationProbes: nil,
	}
	if current != nil {
		plan.BehaviorContracts = append([]types.WriteBehaviorContract(nil), current.BehaviorContracts...)
		plan.VerificationProbes = append([]types.VerificationProbe(nil), current.VerificationProbes...)
	}
	stampChangePlanImpactObligations(plan, graphProvider)
	applyVerifyCoverageToChangePlan(plan, report, err)
	conventionGraph := o.conventionGraphForPatchReview(run, plan)
	review := writeflow.ReviewAppliedPatchSemantic(writeflow.SemanticPatchReviewInput{
		Plan:              plan,
		ActiveSlice:       types.ChangePlanSlice{ID: "cumulative", Paths: paths},
		ImpactAnalysis:    plan.ImpactAnalysis,
		ImpactObligations: plan.ImpactObligations,
		ConventionGraph:   conventionGraph,
	})
	review = types.NormalizePatchReviewRecord(review)
	plan.PatchReview = &review
	applyVerifyCoverageToChangePlan(plan, report, err)
	return plan
}

func (o *Orchestrator) cumulativeReviewOwnedPaths(run *types.WriteWorkflowRun) []string {
	if o == nil || run == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		p := normalizeControllerPath(raw)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, batch := range run.Batches {
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) != "apply" || strings.TrimSpace(attempt.Status) != "applied" {
				continue
			}
			plan := o.loadDurablePlanArtifact(attempt.PlanID)
			if plan == nil {
				continue
			}
			for _, p := range plan.TargetPaths {
				add(p)
			}
			for _, p := range plan.AppliedPaths {
				add(p)
			}
			for _, change := range plan.Changes {
				add(change.Path)
				add(change.NewPath)
			}
		}
	}
	sort.Strings(out)
	return out
}

func firstAppliedWorkflowBaseRef(run *types.WriteWorkflowRun) string {
	if run == nil {
		return ""
	}
	for _, batch := range run.Batches {
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) != "apply" || strings.TrimSpace(attempt.Status) != "applied" {
				continue
			}
			ref := strings.TrimSpace(attempt.ArtifactRef)
			if ref == "" && strings.TrimSpace(attempt.PlanID) != "" {
				ref = worktree.AppliedRef(attempt.PlanID)
			}
			if ref == "" {
				continue
			}
			return ref + "^"
		}
	}
	return ""
}

func patchEffectRecordPaths(effect types.PatchEffectRecord) []string {
	seen := map[string]bool{}
	var out []string
	for _, file := range effect.Files {
		for _, raw := range []string{file.Path, file.OldPath} {
			path := normalizeControllerPath(raw)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func patchEffectFileChanges(effect types.PatchEffectRecord) []types.FileChange {
	paths := patchEffectRecordPaths(effect)
	changes := make([]types.FileChange, 0, len(paths))
	for _, path := range paths {
		changes = append(changes, types.FileChange{
			Path:  path,
			Kind:  "patch",
			Apply: &types.FileChangeApplyRecord{Status: "applied"},
		})
	}
	return changes
}

func changePlanTouchesNonTestCode(plan *types.ChangePlan) bool {
	if plan == nil {
		return false
	}
	paths := append([]string(nil), plan.TargetPaths...)
	paths = append(paths, plan.AppliedPaths...)
	for _, change := range plan.Changes {
		paths = append(paths, change.Path, change.NewPath)
	}
	if plan.PatchEffect != nil {
		for _, file := range plan.PatchEffect.Files {
			paths = append(paths, file.Path, file.OldPath)
		}
	}
	for _, raw := range paths {
		path := normalizeControllerPath(raw)
		if path == "" {
			continue
		}
		if !types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(path)) {
			return true
		}
	}
	return false
}

func sanitizeWorkflowArtifactID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "workflow"
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "workflow"
	}
	return out
}

func (o *Orchestrator) conventionGraphForPatchReview(run *types.WriteWorkflowRun, plan *types.ChangePlan) *types.ConventionGraph {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return nil
	}
	var graphs []types.ConventionGraph
	if handoff := o.busCtx.Mutable.WriteExplorationHandoff(); handoff != nil && handoff.ConventionGraph != nil {
		graphs = append(graphs, *handoff.ConventionGraph)
	}
	graphProvider := writeimpact.GraphProviderFromSearchGraph(o.busCtx.Mutable.SearchGraph())
	learned := writeconvention.LearnFromGraph(writeconvention.LearnInput{
		BatchID:       firstNonEmptyController(activeWorkflowRunBatchID(run), "batch-1"),
		Goal:          firstNonEmptyController(plan.Summary, plan.Request),
		ActivePaths:   patchReviewConventionActivePaths(plan),
		ActiveSymbols: patchReviewConventionActiveSymbols(plan),
		PatchEffect:   plan.PatchEffect,
		Graph:         graphProvider,
	})
	if len(learned.Nodes) > 0 {
		graphs = append(graphs, learned)
	}
	if len(graphs) == 0 {
		return nil
	}
	runID := ""
	if run != nil {
		runID = strings.TrimSpace(run.RunID)
	}
	merged := writeconvention.Merge(runID, graphs...)
	if strings.TrimSpace(o.reportDir) != "" && runID != "" {
		if saved, _, err := writeconvention.NewStore(o.reportDir).MergeAndSave(runID, graphs...); err == nil {
			merged = saved
		} else {
			logging.Warning("[orchestrator] convention learner persist failed: %v", err)
		}
	}
	selected := writeconvention.SelectTopN(merged, patchReviewConventionActivePaths(plan), patchReviewConventionActiveSymbols(plan), 16)
	if len(selected.Nodes) == 0 {
		return nil
	}
	return &selected
}

func activeWorkflowRunBatchID(run *types.WriteWorkflowRun) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.ActiveBatchID)
}

func patchReviewConventionActivePaths(plan *types.ChangePlan) []string {
	if plan == nil {
		return nil
	}
	var paths []string
	paths = append(paths, plan.AppliedPaths...)
	paths = append(paths, plan.TargetPaths...)
	if plan.PatchEffect != nil {
		for _, file := range plan.PatchEffect.Files {
			paths = append(paths, file.Path, file.OldPath)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range paths {
		path := normalizeControllerPath(raw)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func patchReviewConventionActiveSymbols(plan *types.ChangePlan) []string {
	if plan == nil || plan.ImpactAnalysis == nil {
		return nil
	}
	analysis := types.NormalizeImpactAnalysisResult(*plan.ImpactAnalysis)
	seen := map[string]bool{}
	var out []string
	for _, surface := range analysis.ChangedSurfaces {
		symbol := strings.TrimSpace(surface.Symbol)
		if symbol == "" || seen[symbol] {
			continue
		}
		seen[symbol] = true
		out = append(out, symbol)
	}
	return out
}

func normalizeControllerPath(raw string) string {
	path := filepath.ToSlash(strings.TrimSpace(raw))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func (o *Orchestrator) attachActivePatchEffectRecord(plan *types.ChangePlan, activeSlice types.ChangePlanSlice) *types.PatchEffectRecord {
	if o == nil || o.busCtx == nil || plan == nil {
		return nil
	}
	sha := strings.TrimSpace(o.currentIterCommitSHA)
	if sha == "" {
		sha = strings.TrimSpace(plan.AppliedCommitSHA)
	}
	if strings.TrimSpace(o.busCtx.WorktreePath) == "" || sha == "" {
		return nil
	}
	patch, err := worktree.CaptureCommitPatch(o.busCtx.WorktreePath, sha)
	if err != nil {
		logging.Warning("[orchestrator] patch effect diff capture failed: %v", err)
		return nil
	}
	effect := writeflow.PatchEffectRecordFromUnifiedDiff(
		plan.ID,
		activeSlice.ID,
		"applied_commit",
		sha+"^",
		sha,
		patch,
	)
	writeflow.AnnotatePatchEffectStructuredFileParses(&effect, o.busCtx.WorktreePath)
	plan.PatchEffect = &effect
	graphProvider := writeimpact.GraphProviderFromSearchGraph(o.busCtx.Mutable.SearchGraph())
	writeimpact.AnnotatePatchEffectLineFeatureEvents(&effect, graphProvider)
	stampChangePlanImpactObligations(plan, graphProvider)
	o.busCtx.Mutable.SetChangePlan(plan)
	o.persistCurrentChangePlanSnapshot()
	return &effect
}

func (o *Orchestrator) stampActivePlanCheckpointMetadata(plan *types.ChangePlan) *types.ChangePlan {
	if o == nil || o.busCtx == nil || plan == nil {
		return plan
	}
	changed := false
	sha := strings.TrimSpace(o.currentIterCommitSHA)
	if strings.TrimSpace(plan.AppliedCommitSHA) == "" && sha != "" {
		plan.AppliedCommitSHA = sha
		changed = true
	}
	wt := strings.TrimSpace(o.busCtx.WorktreePath)
	if strings.TrimSpace(plan.WorktreePath) == "" && wt != "" {
		plan.WorktreePath = wt
		changed = true
	}
	if changed && o.busCtx.Mutable != nil {
		o.busCtx.Mutable.SetChangePlan(plan)
		o.persistCurrentChangePlanSnapshot()
	}
	return plan
}

func patchReviewHardBlockReason(review types.PatchReviewRecord) string {
	review = types.NormalizePatchReviewRecord(review)
	for _, finding := range review.Findings {
		if finding.Severity != types.PatchReviewSeverityError {
			continue
		}
		if finding.Path != "" {
			return finding.Code + ":" + finding.Path
		}
		return finding.Code
	}
	if len(review.ReasonCodes) > 0 {
		return strings.Join(review.ReasonCodes, ",")
	}
	return "patch_review_failed"
}

func patchReviewHardBlockShouldReplan(review types.PatchReviewRecord) bool {
	review = types.NormalizePatchReviewRecord(review)
	for _, finding := range review.Findings {
		if finding.Severity == types.PatchReviewSeverityError &&
			finding.Category == types.PatchReviewCategoryStructural {
			return true
		}
	}
	return false
}

func patchReviewFailureReasonCode(review types.PatchReviewRecord) string {
	review = types.NormalizePatchReviewRecord(review)
	for _, finding := range review.Findings {
		if finding.Severity != types.PatchReviewSeverityError ||
			finding.Category != types.PatchReviewCategoryStructural {
			continue
		}
		if code := strings.TrimSpace(finding.Code); code != "" {
			return code
		}
	}
	return "patch_review_structural_failed"
}

func patchReviewFailureReport(plan *types.ChangePlan, review types.PatchReviewRecord, reason string) *types.ChangeReport {
	review = types.NormalizePatchReviewRecord(review)
	planID := ""
	if plan != nil {
		planID = strings.TrimSpace(plan.ID)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = patchReviewHardBlockReason(review)
	}
	reasonCode := patchReviewFailureReasonCode(review)
	if reasonCode == "" {
		reasonCode = "patch_review_structural_failed"
	}
	detail := "patch review structural finding requires replan"
	if reason != "" {
		detail += ": " + reason
	}
	report := &types.ChangeReport{
		PlanID:            planID,
		Channel:           types.ChangeReportChannelPostApplyVerify,
		Passed:            false,
		FailureKind:       types.FailureKindTestsFailed,
		FailureReasonCode: reasonCode,
		FailureSummary:    detail,
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindUnit,
			AssertionID:   "patch_review:" + reasonCode,
			Suite:         "patch_review",
			Passed:        false,
			FailureDetail: detail,
		}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:     "patch_review",
			Framework:  "semantic_patch_review",
			Command:    "post-apply patch review",
			Source:     "post_apply_patch_review",
			Outcome:    "failed",
			ReasonCode: reasonCode,
		}},
		VerificationDiagnostics: []types.VerificationDiagnostic{{
			Source:     "patch_review",
			Category:   string(types.PatchReviewCategoryStructural),
			Severity:   string(types.PatchReviewSeverityError),
			ReasonCode: reasonCode,
			Runner:     "patch_review",
			Framework:  "semantic_patch_review",
			Outcome:    "failed",
			Detail:     detail,
		}},
		GeneratedAt: time.Now(),
	}
	report.EnsureVerificationStatus()
	return report
}

func workflowBatchPatchReviewFailureCount(run *types.WriteWorkflowRun, batchID string) int {
	if run == nil {
		return 0
	}
	batchID = strings.TrimSpace(batchID)
	count := 0
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != batchID {
			continue
		}
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) == "patch_review" &&
				strings.TrimSpace(attempt.Status) == "failed" {
				count++
			}
		}
		return count
	}
	return 0
}

func (o *Orchestrator) handlePatchReviewHardBlock(run *types.WriteWorkflowRun, plan *types.ChangePlan, review types.PatchReviewRecord, reason string) (writeControllerApplyTransitionOutcome, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = patchReviewHardBlockReason(review)
	}
	if patchReviewHardBlockShouldReplan(review) && workflowBatchPatchReviewFailureCount(run, run.ActiveBatchID) < o.writeRetryBudget {
		report := patchReviewFailureReport(plan, review, reason)
		if plan != nil {
			plan.Status = types.PlanStatusVerifyFailed
			o.busCtx.Mutable.SetChangePlan(plan)
			o.persistCurrentChangePlanSnapshot()
		}
		o.busCtx.Mutable.SetChangeReport(report)
		updateWorkflowRunBatchVerify(run, run.ActiveBatchID, report, "failed", "patch_review_structural_failed")
		updateWorkflowRunActiveSliceObserve(run, run.ActiveBatchID, report, "failed", "patch_review_structural_failed")
		markWorkflowRunActiveSlicePatchReviewFailed(run, run.ActiveBatchID, plan, reason)
		updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchReadyToPlan)
		appendControllerProgress(run, run.ActiveBatchID, "patch_review_failed", reason)
		diffRef, surfaceRef := o.persistVerifyFailureEvidence(run, report, workflowBatchPatchReviewFailureCount(run, run.ActiveBatchID))
		if handoff := o.resolveVerifyFailureHandoffArtifacts(types.BuildVerifyFailureHandoff(report, run.ActiveBatchID, workflowBatchPatchReviewFailureCount(run, run.ActiveBatchID), diffRef, surfaceRef)); handoff != nil {
			o.busCtx.Mutable.SetVerifyFailureHandoff(handoff)
		}
		reportPack := types.WriteContextPackFromChangeReport(report).
			WithScope(run.ActiveBatchID, activeWorkflowRunSliceID(*run, run.ActiveBatchID))
		o.busCtx.Mutable.MergeWriteContextPack(reportPack)
		o.syncCurrentWriteContextPackToRun(run)
		o.persistWriteWorkflowRun(run)
		o.busCtx.TaskState.LastError = ""
		return writeControllerApplyTransitionRetry, fmt.Errorf("patch review failed: %s", reason)
	}
	if plan != nil {
		plan.Status = types.PlanStatusBlocked
		o.busCtx.Mutable.SetChangePlan(plan)
		o.persistCurrentChangePlanSnapshot()
	}
	run.Status = types.WriteWorkflowRunBlocked
	updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
	markWorkflowRunActiveSlicePatchReviewBlocked(run, run.ActiveBatchID, plan, reason)
	reasonCode := "patch_review_blocked"
	if patchReviewHardBlockShouldReplan(review) {
		reasonCode = "patch_review_retry_budget_exhausted"
	}
	appendControllerProgress(run, run.ActiveBatchID, reasonCode, reason)
	o.persistWriteWorkflowRun(run)
	o.publishBlockedRunGuidance(run, reasonCode)
	return writeControllerApplyTransitionTerminal, fmt.Errorf("patch review blocked: %s", reason)
}

func (o *Orchestrator) runControllerApplyPlanTransition(run *types.WriteWorkflowRun, stepsUsed *int, reasonCode string) (writeControllerApplyTransitionOutcome, error) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return writeControllerApplyTransitionTerminal, fmt.Errorf("write controller apply transition missing context")
	}
	if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
		if outcome, handled, gateErr := o.runOwnerLocalizationAuthorityGateBeforeApply(run, plan, stepsUsed); handled {
			return outcome, gateErr
		}
		markWorkflowRunActiveSliceApplying(run, run.ActiveBatchID, plan, reasonCode)
		o.persistWriteWorkflowRun(run)
	}
	innerErr := o.runControllerApplyPlan(stepsUsed)
	plan := o.busCtx.Mutable.ChangePlan()
	recoveredApplyError := false
	if innerErr != nil && changePlanAllDeclaredChangesApplied(plan) {
		recoveredApplyError = true
		appendControllerProgress(run, run.ActiveBatchID, "apply_transport_recovered_all_changes",
			"typed ChangePlan records every declared change as applied; continuing to post-apply verification")
		if plan.Status == types.PlanStatusApplyFailed || plan.Status == types.PlanStatusPartiallyApplied {
			plan.Status = types.PlanStatusPending
			o.busCtx.Mutable.SetChangePlan(plan)
		}
		innerErr = nil
	}
	if innerErr == nil {
		o.markActivePlanAppliedPendingVerify()
		plan = o.busCtx.Mutable.ChangePlan()
	}
	if innerErr == nil && plan != nil {
		plan = o.stampActivePlanCheckpointMetadata(plan)
	}
	if plan != nil {
		applyStatus := writeWorkflowApplyAttemptStatus(plan, innerErr)
		applyReason := writeWorkflowApplyAttemptReason(plan, innerErr)
		if recoveredApplyError {
			applyReason = "apply_transport_recovered_all_changes"
		}
		updateWorkflowRunBatchPlan(run, run.ActiveBatchID, plan)
		updateWorkflowRunBatchApply(run, run.ActiveBatchID, plan, applyStatus, applyReason)
		if innerErr == nil {
			review := o.reviewActiveAppliedPatchScope(run, plan)
			plan = o.busCtx.Mutable.ChangePlan()
			if review.HardBlock {
				reason := patchReviewHardBlockReason(review)
				return o.handlePatchReviewHardBlock(run, plan, review, reason)
			}
		}
		*run = attachPlanContextPackToWorkflowRun(*run, plan)
	}
	if innerErr != nil {
		switch {
		case writePlanNeedsManualApproval(plan, run):
			updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchPendingApproval)
			appendControllerProgress(run, run.ActiveBatchID, string(types.WriteWorkflowBatchPendingApproval), innerErr.Error())
			o.persistWriteWorkflowRun(run)
			o.publishBlockedRunGuidance(run, "pending_approval")
			return writeControllerApplyTransitionTerminal, innerErr
		case plan != nil && plan.Status == types.PlanStatusBlocked:
			run.Status = types.WriteWorkflowRunBlocked
			updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
			appendControllerProgress(run, run.ActiveBatchID, string(plan.Status), innerErr.Error())
			o.persistWriteWorkflowRun(run)
			o.publishBlockedRunGuidance(run, "approval_denied")
			return writeControllerApplyTransitionTerminal, innerErr
		default:
			updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchPlanned)
			appendControllerProgress(run, run.ActiveBatchID, "apply_failed", innerErr.Error())
			o.busCtx.TaskState.LastError = ""
			o.persistWriteWorkflowRun(run)
			return writeControllerApplyTransitionRetry, innerErr
		}
	}
	updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchVerifying)
	appendControllerProgress(run, run.ActiveBatchID, "batch_applied", "")
	o.syncCurrentWriteContextPackToRun(run)
	o.persistWriteWorkflowRun(run)
	return writeControllerApplyTransitionApplied, nil
}

func (o *Orchestrator) runControllerVerifyBatch(stepsUsed *int) error {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return fmt.Errorf("write controller verify batch missing context")
	}
	if o.skipVerify {
		now := time.Now()
		o.persistPlanStatus(types.PlanStatusApplied, &now)
		logging.Info("[orchestrator] controller verify skipped (--skip-verify); plan marked applied without test execution")
		return nil
	}
	_, err := o.runControllerWriteStage(types.StageVerify, stepsUsed)
	return err
}

func (o *Orchestrator) runControllerObserveSlice(stepsUsed *int) error {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return fmt.Errorf("write controller observe slice missing context")
	}
	run := o.busCtx.Mutable.WriteWorkflowRun()
	plan := o.busCtx.Mutable.ChangePlan()
	if !writeflow.HasActiveRuntimeUnit(plan, run) {
		return fmt.Errorf("write controller observe_slice requires an active plan slice")
	}
	return o.runControllerVerifyBatch(stepsUsed)
}

func (o *Orchestrator) syncMutablePlanStatusAfterVerify(report *types.ChangeReport, err error) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || report == nil {
		return
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		return
	}
	now := time.Now()
	switch {
	case reportIndicatesVerificationUnavailable(report):
		plan.Status = types.PlanStatusUnverified
		plan.AppliedAt = &now
	case err != nil || !report.Passed:
		plan.Status = types.PlanStatusVerifyFailed
		plan.AppliedAt = nil
	case len(report.NoTestsRunners) > 0 && planTouchesNonTestCode(o.busCtx):
		plan.Status = types.PlanStatusUnverified
		plan.AppliedAt = &now
	default:
		plan.Status = types.PlanStatusApplied
		plan.AppliedAt = &now
	}
	if o.busCtx.WorktreePath != "" {
		plan.WorktreePath = o.busCtx.WorktreePath
	}
	applyVerifyCoverageToChangePlan(plan, report, err)
	o.busCtx.Mutable.SetChangePlan(plan)
	o.persistCurrentChangePlanSnapshot()
	pack := types.WriteContextPackFromChangePlan(plan)
	if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
		pack = pack.WithScope(strings.TrimSpace(run.ActiveBatchID), activeWorkflowRunSliceID(*run, run.ActiveBatchID))
	}
	if len(pack.Items) > 0 {
		o.busCtx.Mutable.MergeWriteContextPack(pack)
	}
}

const (
	impactCoverageVerified    = "verified"
	impactCoverageUnverified  = "unverified"
	impactCoverageUnavailable = "unavailable"
)

type verifyCoverageProjection struct {
	ImpactStatus string
	ReviewStatus types.PatchReviewCoverageStatus
	Confidence   verifyCoverageConfidence
}

type verifyCoverageConfidence struct {
	MissingChangedSymbol bool
	MissingContracts     map[string]bool
	CoveredSymbols       map[string]bool
	CoveredContracts     map[string]bool
	CoveredPaths         map[string]bool
	ProbeUnavailable     bool
}

func applyVerifyCoverageToChangePlan(plan *types.ChangePlan, report *types.ChangeReport, err error) {
	if plan == nil || report == nil {
		return
	}
	projection, ok := verifyCoverageProjectionFromReport(report, err)
	if !ok {
		return
	}
	if plan.ImpactAnalysis != nil {
		analysis := types.NormalizeImpactAnalysisResult(*plan.ImpactAnalysis)
		for i := range analysis.VerificationTargets {
			analysis.VerificationTargets[i].CoverageStatus = impactCoverageForTarget(analysis.VerificationTargets[i], projection)
		}
		analysis = types.NormalizeImpactAnalysisResult(analysis)
		plan.ImpactAnalysis = &analysis
	}
	if plan.PatchReview != nil {
		review := types.NormalizePatchReviewRecord(*plan.PatchReview)
		for i := range review.Findings {
			if review.Findings[i].Category != types.PatchReviewCategorySemanticCoverage {
				continue
			}
			review.Findings[i].CoverageStatus = patchReviewCoverageForFinding(review.Findings[i], projection)
		}
		review = types.NormalizePatchReviewRecord(review)
		plan.PatchReview = &review
	}
}

func verifyCoverageProjectionFromReport(report *types.ChangeReport, err error) (verifyCoverageProjection, bool) {
	if report == nil {
		return verifyCoverageProjection{}, false
	}
	authority := writeflow.DeriveObservationAuthorityFromReport(report, err)
	projection := verifyCoverageProjection{Confidence: verifyCoverageConfidenceFromReport(report)}
	switch authority.State {
	case writeflow.ObservationAuthorityVerified:
		projection.ImpactStatus = impactCoverageVerified
		projection.ReviewStatus = types.PatchReviewCoverageVerified
	case writeflow.ObservationAuthorityUnverified:
		projection.ImpactStatus = impactCoverageUnavailable
		projection.ReviewStatus = types.PatchReviewCoverageUnavailable
	case writeflow.ObservationAuthorityFailed:
		projection.ImpactStatus = impactCoverageUnverified
		projection.ReviewStatus = types.PatchReviewCoverageUnverified
	default:
		return verifyCoverageProjection{}, false
	}
	return projection, true
}

func verifyCoverageConfidenceFromReport(report *types.ChangeReport) verifyCoverageConfidence {
	conf := verifyCoverageConfidence{
		MissingContracts: map[string]bool{},
		CoveredSymbols:   map[string]bool{},
		CoveredContracts: map[string]bool{},
		CoveredPaths:     map[string]bool{},
	}
	if report == nil {
		return conf
	}
	for _, result := range report.TestResults {
		if !result.Passed {
			continue
		}
		conf.addCoveredPath(result.Suite)
	}
	for _, cmd := range report.ExecutedCommands {
		if !verifyCoverageCommandCoversPath(cmd) {
			continue
		}
		conf.addCoveredPath(cmd.Suite)
	}
	for _, rec := range report.VerificationConfidence {
		status := strings.TrimSpace(rec.Status)
		category := strings.TrimSpace(rec.Category)
		switch {
		case status == "unavailable" && category == "probe_execution":
			conf.ProbeUnavailable = true
		case status == "missing" && category == "probe_changed_symbol":
			conf.MissingChangedSymbol = true
		case status == "missing" && category == "probe_contract_refs":
			for _, ref := range rec.ContractRefs {
				if ref = strings.TrimSpace(ref); ref != "" {
					conf.MissingContracts[ref] = true
				}
			}
		case status == "satisfied" && category == "probe_changed_symbol":
			for _, ref := range rec.ChangedSymbolRefs {
				if ref = strings.TrimSpace(ref); ref != "" {
					conf.CoveredSymbols[ref] = true
				}
			}
		case status == "satisfied" && category == "probe_contract_refs":
			for _, ref := range rec.ContractRefs {
				if ref = strings.TrimSpace(ref); ref != "" {
					conf.CoveredContracts[ref] = true
				}
			}
		}
	}
	return conf
}

func verifyCoverageCommandCoversPath(cmd types.ExecutedCommand) bool {
	if strings.TrimSpace(cmd.Suite) == "" || cmd.ExitCode != 0 {
		return false
	}
	switch strings.TrimSpace(cmd.Outcome) {
	case "", "executed", "syntax_check_fallback":
		return true
	default:
		return false
	}
}

func (conf *verifyCoverageConfidence) addCoveredPath(raw string) {
	if conf == nil {
		return
	}
	if conf.CoveredPaths == nil {
		conf.CoveredPaths = map[string]bool{}
	}
	for _, alias := range verifyCoveragePathAliases(raw) {
		conf.CoveredPaths[alias] = true
	}
}

func impactCoverageForTarget(target types.ImpactVerificationTarget, projection verifyCoverageProjection) string {
	switch projection.ImpactStatus {
	case impactCoverageVerified:
		if projection.Confidence.ProbeUnavailable {
			switch target.Kind {
			case "changed_symbol":
				if target.Symbol == "" || !projection.Confidence.CoveredSymbols[target.Symbol] {
					return impactCoverageUnverified
				}
			case "behavior_contract":
				ref := strings.TrimSpace(target.ContractRef)
				if ref == "" {
					ref = strings.TrimSpace(target.EvidenceRef)
				}
				if ref == "" || !projection.Confidence.CoveredContracts[ref] {
					return impactCoverageUnverified
				}
			}
		}
		if target.Kind == "changed_symbol" && projection.Confidence.MissingChangedSymbol {
			if target.Symbol == "" || !projection.Confidence.CoveredSymbols[target.Symbol] {
				return impactCoverageUnverified
			}
		}
		if target.Kind == "behavior_contract" {
			ref := strings.TrimSpace(target.ContractRef)
			if ref == "" {
				ref = strings.TrimSpace(target.EvidenceRef)
			}
			if ref != "" && projection.Confidence.MissingContracts[ref] && !projection.Confidence.CoveredContracts[ref] {
				return impactCoverageUnverified
			}
		}
		if impactTargetNeedsScopedCoverage(target) && !projection.Confidence.CoversPath(target.RelatedPath, target.Path) {
			if status := strings.TrimSpace(target.CoverageStatus); status != "" {
				return status
			}
			return impactCoverageUnverified
		}
		return impactCoverageVerified
	case impactCoverageUnavailable:
		return impactCoverageUnavailable
	case impactCoverageUnverified:
		return impactCoverageUnverified
	default:
		return strings.TrimSpace(target.CoverageStatus)
	}
}

func patchReviewCoverageForFinding(finding types.PatchReviewFinding, projection verifyCoverageProjection) types.PatchReviewCoverageStatus {
	switch projection.ReviewStatus {
	case types.PatchReviewCoverageVerified:
		switch strings.TrimSpace(finding.Code) {
		case "changed_symbol_without_probe_coverage":
			if projection.Confidence.ProbeUnavailable {
				symbol := strings.TrimSpace(finding.SubjectSymbol)
				if symbol == "" || !projection.Confidence.CoveredSymbols[symbol] {
					return types.PatchReviewCoverageUnverified
				}
			}
			if projection.Confidence.MissingChangedSymbol {
				symbol := strings.TrimSpace(finding.SubjectSymbol)
				if symbol == "" || !projection.Confidence.CoveredSymbols[symbol] {
					return types.PatchReviewCoverageUnverified
				}
			}
		case "behavior_contract_without_verify_coverage":
			ref := strings.TrimSpace(finding.EvidenceRef)
			if projection.Confidence.ProbeUnavailable && (ref == "" || !projection.Confidence.CoveredContracts[ref]) {
				return types.PatchReviewCoverageUnverified
			}
			if ref != "" && projection.Confidence.MissingContracts[ref] && !projection.Confidence.CoveredContracts[ref] {
				return types.PatchReviewCoverageUnverified
			}
		case "dependent_surface_without_verify_coverage", "related_test_surface_unverified":
			if !projection.Confidence.CoversPath(finding.RelatedPath, finding.Path) {
				return preservePatchReviewUncoveredStatus(finding.CoverageStatus)
			}
		default:
			if writeflow.PatchReviewEffectUnknownCoverage(finding.Code) {
				return preservePatchReviewUncoveredStatus(finding.CoverageStatus)
			}
			if patchReviewFindingNeedsExplicitCoverage(finding) {
				return preservePatchReviewUncoveredStatus(finding.CoverageStatus)
			}
		}
		return types.PatchReviewCoverageVerified
	case types.PatchReviewCoverageUnavailable:
		return types.PatchReviewCoverageUnavailable
	case types.PatchReviewCoverageUnverified:
		return types.PatchReviewCoverageUnverified
	default:
		return finding.CoverageStatus
	}
}

func (conf verifyCoverageConfidence) CoversPath(paths ...string) bool {
	for _, path := range paths {
		for _, alias := range verifyCoveragePathAliases(path) {
			if conf.CoveredPaths[alias] {
				return true
			}
		}
	}
	return false
}

func impactTargetNeedsScopedCoverage(target types.ImpactVerificationTarget) bool {
	switch strings.TrimSpace(target.Kind) {
	case "dependent", "test_surface", "dependency", "effect_followup":
		return true
	default:
		return false
	}
}

func patchReviewFindingNeedsExplicitCoverage(finding types.PatchReviewFinding) bool {
	if finding.Category != types.PatchReviewCategorySemanticCoverage {
		return false
	}
	switch strings.TrimSpace(finding.Code) {
	case "changed_symbol_without_probe_coverage", "behavior_contract_without_verify_coverage":
		return false
	default:
		return finding.CoverageStatus == types.PatchReviewCoverageUnknown ||
			finding.CoverageStatus == types.PatchReviewCoverageUnverified ||
			finding.CoverageStatus == types.PatchReviewCoverageUnavailable
	}
}

func preservePatchReviewUncoveredStatus(status types.PatchReviewCoverageStatus) types.PatchReviewCoverageStatus {
	switch status {
	case types.PatchReviewCoverageUnknown, types.PatchReviewCoverageUnverified, types.PatchReviewCoverageUnavailable:
		return status
	default:
		return types.PatchReviewCoverageUnverified
	}
}

func normalizeVerifyCoveragePath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." || path == "/" {
		return ""
	}
	return path
}

func verifyCoveragePathAliases(raw string) []string {
	raw = normalizeVerifyCoveragePath(raw)
	if raw == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = normalizeVerifyCoveragePath(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	add(raw)
	if idx := strings.Index(raw, "::"); idx > 0 {
		add(raw[:idx])
	}
	if verifyCoverageLooksLikeModuleSelector(raw) {
		for _, alias := range verifyCoverageModuleSelectorAliases(raw) {
			add(alias)
		}
	}
	return out
}

func verifyCoverageLooksLikeModuleSelector(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "/") || strings.Contains(raw, "\\") || strings.ContainsAny(raw, " \t\n\r") {
		return false
	}
	if !strings.Contains(raw, ".") {
		return false
	}
	for _, part := range strings.Split(raw, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
	}
	return verifyCoverageSelectorHasTestSignal(raw)
}

func verifyCoverageSelectorHasTestSignal(raw string) bool {
	for _, part := range strings.Split(raw, ".") {
		lower := strings.ToLower(strings.TrimSpace(part))
		if lower == "" {
			continue
		}
		if lower == "tests" || lower == "test" || strings.HasPrefix(lower, "test_") ||
			strings.HasSuffix(lower, "_test") ||
			strings.HasSuffix(lower, "_tests") || strings.HasSuffix(lower, "test") ||
			strings.HasSuffix(lower, "tests") || strings.Contains(lower, "_tests_") ||
			strings.Contains(lower, "_test_") {
			return true
		}
	}
	return false
}

func verifyCoverageModuleSelectorAliases(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	var out []string
	for i := 1; i <= len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		if prefix == "" {
			continue
		}
		lowerLast := strings.ToLower(parts[i-1])
		looksLikeClass := strings.HasSuffix(parts[i-1], "Test") || strings.HasSuffix(parts[i-1], "Tests")
		looksLikePythonModule := i >= 2 && verifyCoverageSelectorHasTestSignal(strings.Join(parts[:i], "."))
		if looksLikePythonModule {
			out = append(out, prefix+".py", "tests/"+prefix+".py")
			out = append(out, prefix+".rb", "spec/"+prefix+".rb")
		}
		if looksLikeClass || strings.HasSuffix(lowerLast, "test") || strings.HasSuffix(lowerLast, "tests") {
			out = append(out,
				prefix+".java",
				"src/test/java/"+prefix+".java",
				prefix+".kt",
				"src/test/kotlin/"+prefix+".kt",
			)
		}
	}
	return out
}

func (o *Orchestrator) markActivePlanUnverifiedAfterVerifyInfra(reason string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		return
	}
	if strings.TrimSpace(plan.Status) == types.PlanStatusApplied {
		return
	}
	now := time.Now()
	plan.Status = types.PlanStatusUnverified
	plan.AppliedAt = &now
	if o.busCtx.WorktreePath != "" {
		plan.WorktreePath = o.busCtx.WorktreePath
	}
	o.busCtx.Mutable.SetChangePlan(plan)
	o.persistPlanStatus(types.PlanStatusUnverified, &now)
	logging.Info("[orchestrator] verify infra unavailable for plan %s (%s); marked unverified", plan.ID, strings.TrimSpace(reason))
}

func (o *Orchestrator) syncMutablePlanStatusAfterSkippedVerify() {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		return
	}
	now := time.Now()
	plan.Status = types.PlanStatusApplied
	plan.AppliedAt = &now
	if o.busCtx.WorktreePath != "" {
		plan.WorktreePath = o.busCtx.WorktreePath
	}
	o.busCtx.Mutable.SetChangePlan(plan)
}

func (o *Orchestrator) validateControllerPostApplyReport(report *types.ChangeReport) error {
	if report == nil {
		return nil
	}
	currentPlanID := ""
	if o != nil && o.busCtx != nil && o.busCtx.Mutable != nil {
		if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
			currentPlanID = strings.TrimSpace(plan.ID)
		}
		if currentPlanID == "" {
			if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
				for _, batch := range run.Batches {
					if strings.TrimSpace(batch.ID) == strings.TrimSpace(run.ActiveBatchID) {
						currentPlanID = strings.TrimSpace(batch.PlanID)
						break
					}
				}
			}
		}
	}
	if report.Channel == "" {
		report.Channel = types.ChangeReportChannelPostApplyVerify
	}
	if currentPlanID != "" && strings.TrimSpace(report.PlanID) == "" {
		report.PlanID = currentPlanID
	}
	if report.Channel != types.ChangeReportChannelPostApplyVerify {
		msg := fmt.Sprintf("verify produced non-post-apply ChangeReport channel %q for plan %s", report.Channel, strings.TrimSpace(report.PlanID))
		markControllerInvalidVerifyReport(report, currentPlanID, msg)
		return fmt.Errorf("%s", msg)
	}
	if currentPlanID != "" && strings.TrimSpace(report.PlanID) != currentPlanID {
		msg := fmt.Sprintf("verify produced ChangeReport for stale plan %s; active plan is %s", strings.TrimSpace(report.PlanID), currentPlanID)
		markControllerInvalidVerifyReport(report, currentPlanID, msg)
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func markControllerInvalidVerifyReport(report *types.ChangeReport, planID, message string) {
	if report == nil {
		return
	}
	if strings.TrimSpace(planID) != "" {
		report.PlanID = strings.TrimSpace(planID)
	}
	report.Passed = false
	report.BuildFailed = false
	report.FailureSummary = strings.TrimSpace(message)
	if report.FailureKind == "" {
		report.FailureKind = types.FailureKindTestsFailed
	}
}

func (o *Orchestrator) runControllerWriteStage(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
	if o == nil || o.busCtx == nil {
		return nil, fmt.Errorf("write controller stage %s missing context", stage)
	}
	if o.controllerWriteStageFn != nil {
		o.busCtx.PipelineStage = stage
		o.busCtx.TaskState.Stage = stage
		return o.controllerWriteStageFn(stage, stepsUsed)
	}
	if cerr := o.checkCanceled("write_controller_"+string(stage), *stepsUsed); cerr != nil {
		o.busCtx.TaskState.LastError = cerr.Error()
		return nil, cerr
	}
	o.busCtx.PipelineStage = stage
	o.busCtx.TaskState.Stage = stage
	if err := runStagePreHook(o, stage); err != nil {
		o.busCtx.TaskState.LastError = err.Error()
		return nil, err
	}
	out, dispatchErr := o.dispatchStage(stage)
	*stepsUsed++
	runStagePostHook(o, stage, out)
	if dispatchErr != nil {
		return out, dispatchErr
	}
	if out != nil && strings.TrimSpace(out.Error) != "" {
		return out, errors.New(strings.TrimSpace(out.Error))
	}
	o.busCtx.TaskState.LastError = ""
	return out, nil
}

// prepareControllerPlanningState resets the per-round planning channels.
// Mutable.VerifyFailureHandoff deliberately survives this reset: it is the
// typed carrier that brings the latest failed verification into the replan
// prompt, and the scheduler clears it only on green verify or finish.
func (o *Orchestrator) prepareControllerPlanningState() {
	mu := o.busCtx.Mutable
	mu.ResetChangePlan()
	mu.ResetChangeReport()
	mu.ResetBaselineReport()
	mu.ResetBestPlanReport()
	mu.ResetWriteClosure()
	mu.ResetIterationLedger()
	o.planPath = ""
	o.busCtx.PlanPath = ""
	o.bestAppliedCommitSHA = ""
	o.currentIterCommitSHA = ""
	o.phaseContextPrefix = ""
}

func (o *Orchestrator) seedControllerBatchPlanningHint(batch writeflow.WriteBatchPlan) {
	batch = writeflow.NormalizeBatchPlan(batch)
	var b strings.Builder
	fmt.Fprintf(&b, "## Workflow batch %s\n\n", batchIDOrDefault(batch.ID, "batch-1"))
	if activeBatchHasUsableExplorationHandoff(o.busCtx.Mutable) {
		b.WriteString("## Exploration convergence\n\n")
		b.WriteString("A prior read-only exploration attempt for this active batch produced a typed handoff/context pack. This planning dispatch is the bounded ChangePlan synthesis step: use the handoff as the lead context, use read-only tools only to fetch exact current bytes or line ranges needed for old_text/patch construction, then emit one ChangePlan. Do not restart broad source exploration in this dispatch; if the typed handoff is insufficient, emit the smallest bounded plan that preserves the recorded unknowns or let the no-plan retry surface the gap.\n\n")
	}
	if batch.Goal != "" {
		fmt.Fprintf(&b, "Plan only this bounded batch: %s\n\n", batch.Goal)
	}
	if batch.Purpose != "" {
		fmt.Fprintf(&b, "Purpose: %s\n\n", batch.Purpose)
	}
	ownerView := o.controllerPlanningOwnerAnchorView(batch, types.WriteConsumerPlanner, 8)
	if ownerRows := types.OwnerAnchorEvidenceRequirements(ownerView, 6); len(ownerRows) > 0 {
		b.WriteString("Typed owner-anchor repair candidates:\n")
		for _, row := range ownerRows {
			fmt.Fprintf(&b, "- %s\n", row)
		}
		b.WriteString("\n")
	}
	requirements := types.LocalizationRequirementsFromCandidatePaths(batch.ExpectedPaths, ownerView, batchIDOrDefault(batch.ID, "batch-1"), "", "controller_preplan", 8)
	if requirementRows := types.LocalizationRequirementEvidenceRows(requirements, 6); len(requirementRows) > 0 {
		b.WriteString("Typed localization requirements:\n")
		for _, row := range requirementRows {
			fmt.Fprintf(&b, "- %s\n", row)
		}
		b.WriteString("\n")
	}
	if len(batch.ExpectedPaths) > 0 {
		b.WriteString("Expected paths:\n")
		for _, p := range batch.ExpectedPaths {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString("\n")
	}
	if len(batch.SuccessCriteria) > 0 {
		b.WriteString("Batch success criteria:\n")
		for _, c := range batch.SuccessCriteria {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	o.busCtx.Mutable.SetPlanningHint(strings.TrimSpace(b.String()))
}

func (o *Orchestrator) seedControllerBatchExplorationContext(batch writeflow.WriteBatchPlan) {
	batch = writeflow.NormalizeBatchPlan(batch)
	ownerView := o.controllerPlanningOwnerAnchorView(batch, types.WriteConsumerPlanner, 12)
	candidatePaths := types.OwnerAnchorCandidatePaths(ownerView, batch.ExpectedPaths, 16)
	prePlanRequirements := types.LocalizationRequirementsFromCandidatePaths(batch.ExpectedPaths, ownerView, batchIDOrDefault(batch.ID, "batch-1"), "", "controller_preplan", 8)
	evidenceRequirements := append(types.OwnerAnchorEvidenceRequirements(ownerView, 8), types.LocalizationRequirementEvidenceRows(prePlanRequirements, 8)...)
	evidenceRequirements = append(evidenceRequirements, batch.SuccessCriteria...)
	req := types.WriteExplorationRequest{
		BatchID:              batchIDOrDefault(batch.ID, "batch-1"),
		Goal:                 batch.Goal,
		CandidatePaths:       candidatePaths,
		EvidenceRequirements: evidenceRequirements,
	}
	o.busCtx.Mutable.SetWriteExplorationRequest(&req)
}

func (o *Orchestrator) controllerPlanningOwnerAnchorView(batch writeflow.WriteBatchPlan, consumer types.WriteContextConsumer, limit int) types.OwnerAnchorView {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return types.OwnerAnchorView{}
	}
	batch = writeflow.NormalizeBatchPlan(batch)
	var packs []types.WriteContextPack
	batchID := strings.TrimSpace(batch.ID)
	sliceID := ""
	if run := o.busCtx.Mutable.WriteWorkflowRun(); run != nil {
		packs = append(packs, run.ContextPacks...)
		if batchID == "" {
			batchID = strings.TrimSpace(run.ActiveBatchID)
		}
		if batchID != "" {
			sliceID = activeWorkflowRunSliceID(*run, batchID)
		}
	}
	if pack := o.busCtx.Mutable.WriteContextPack(); pack != nil {
		packs = append(packs, *pack)
	}
	if len(packs) == 0 {
		return types.OwnerAnchorView{}
	}
	return types.OwnerAnchorViewFromWriteContextPacks(packs, consumer, batchID, sliceID, limit)
}

func (o *Orchestrator) syncCurrentWriteContextPackToRun(run *types.WriteWorkflowRun) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return
	}
	pack := o.busCtx.Mutable.WriteContextPack()
	if pack == nil || len(pack.Items) == 0 {
		return
	}
	activeBatchID := strings.TrimSpace(run.ActiveBatchID)
	activeSliceID := activeWorkflowRunSliceID(*run, activeBatchID)
	scoped := pack.ForScope(activeBatchID, activeSliceID)
	if len(scoped.Items) == 0 {
		return
	}
	*run = upsertWorkflowRunContextPack(*run, scoped)
	o.busCtx.Mutable.SetWriteWorkflowRun(run)
}

func activeWorkflowRunSliceID(run types.WriteWorkflowRun, batchID string) string {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return ""
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == batchID {
			return strings.TrimSpace(batch.ActiveSliceID)
		}
	}
	return ""
}

func (o *Orchestrator) persistWriteWorkflowRun(run *types.WriteWorkflowRun) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	run.UpdatedAt = time.Now()
	normalized := types.NormalizeWriteWorkflowRun(*run)
	if violations := writeflow.ValidateWorkflowRunState(normalized, o.busCtx.Mutable.ChangePlan()); len(violations) > 0 {
		logging.Warning("[orchestrator] write workflow state invariant warning: %s", strings.Join(violations, "; "))
	}
	o.busCtx.Mutable.SetWriteWorkflowRun(&normalized)
	workflowPath := ""
	if o.writeWorkflowRunStore == nil {
		o.persistWriteFinalReportIfAuditable(&normalized, "")
		return
	}
	if path, err := o.writeWorkflowRunStore.Save(&normalized); err != nil {
		logging.Warning("[orchestrator] write workflow run persist failed: %v", err)
	} else {
		workflowPath = path
	}
	o.persistWriteFinalReportIfAuditable(&normalized, workflowPath)
}

func (o *Orchestrator) persistWriteFinalReportIfAuditable(run *types.WriteWorkflowRun, workflowPath string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return
	}
	plan := o.busCtx.Mutable.ChangePlan()
	report := o.busCtx.Mutable.ChangeReport()
	if !writeWorkflowRunHasAuditableFinalReportState(run, plan, report) {
		return
	}
	planID := writeFinalReportArtifactID(run, plan, report)
	if report == nil {
		report = o.loadWriteFinalReportChangeReport(planID)
	}
	if plan == nil && strings.TrimSpace(planID) != "" {
		if loaded, err := o.loadWriteFinalReportChangePlan(planID); err == nil && loaded != nil {
			plan = loaded
		}
	}
	planPath := ""
	if plan != nil {
		planPath = o.ensureChangePlanPath()
	}
	planDir := o.ensureChangeReportDir()
	if planDir == "" {
		return
	}
	stem := writeWorkflowArtifactFileStem(planID)
	if stem == "" {
		return
	}
	reportPath := ""
	if report != nil {
		reportPath = filepath.Join(planDir, stem+".report.json")
	}
	delivery := o.writeFinalReportDeliverySummary(run, plan, report)
	primarySourcePlan := plan
	if delivery.PrimarySourcePlanID != "" && (primarySourcePlan == nil || strings.TrimSpace(primarySourcePlan.ID) != delivery.PrimarySourcePlanID) {
		if loaded, err := o.loadWriteFinalReportChangePlan(delivery.PrimarySourcePlanID); err == nil && loaded != nil {
			primarySourcePlan = loaded
		} else {
			primarySourcePlan = nil
		}
	}
	final := types.BuildWriteFinalReport(types.WriteFinalReportInput{
		Run:               run,
		Plan:              plan,
		PrimarySourcePlan: primarySourcePlan,
		Report:            report,
		ProofArtifacts:    o.writeFinalReportProofArtifacts(run, plan, report),
		Delivery:          delivery,
		PlanPath:          planPath,
		ReportPath:        reportPath,
		WorkflowPath:      workflowPath,
	})
	final.Loop = writeFinalLoopSummaryFromRun(run)
	final.ReasoningGraph = reasoninggraph.WriteFinalReasoningGraphSummaryFromRun(run, reasoninggraph.DefaultWriteFinalReasoningGraphEventRefLimit)
	finalPath := filepath.Join(planDir, stem+".final.json")
	if err := types.WriteFinalReportToFile(&final, finalPath); err != nil {
		logging.Warning("[orchestrator] write final report persist failed: %v", err)
		return
	}
	logging.Info("[orchestrator] WriteFinalReport saved: %s", finalPath)
}

func writeWorkflowRunHasAuditableFinalReportState(run *types.WriteWorkflowRun, plan *types.ChangePlan, report *types.ChangeReport) bool {
	if run == nil {
		return false
	}
	if run.Status == types.WriteWorkflowRunComplete || run.Status == types.WriteWorkflowRunBlocked {
		return true
	}
	if report != nil {
		return true
	}
	if plan != nil {
		switch strings.TrimSpace(plan.Status) {
		case types.PlanStatusAppliedPendingVerify,
			types.PlanStatusApplied,
			types.PlanStatusApplyFailed,
			types.PlanStatusVerifyFailed,
			types.PlanStatusUnverified,
			types.PlanStatusPartiallyApplied,
			types.PlanStatusBlocked,
			types.PlanStatusNoChangeRequired:
			return true
		}
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.VerifyRef) != "" || strings.TrimSpace(batch.ApplyRef) != "" {
			return true
		}
		for _, attempt := range batch.Attempts {
			switch strings.TrimSpace(attempt.Kind) {
			case "apply", "verify":
				if strings.TrimSpace(attempt.Status) != "" || strings.TrimSpace(attempt.PlanID) != "" || strings.TrimSpace(attempt.ReportID) != "" {
					return true
				}
			}
		}
		for _, event := range batch.SliceEvents {
			switch event.Event {
			case types.WriteWorkflowSliceEventApplyCompleted,
				types.WriteWorkflowSliceEventVerified,
				types.WriteWorkflowSliceEventUnverified,
				types.WriteWorkflowSliceEventFailed:
				return true
			}
		}
	}
	return false
}

func (o *Orchestrator) writeFinalReportDeliverySummary(run *types.WriteWorkflowRun, finalPlan *types.ChangePlan, report *types.ChangeReport) types.WriteFinalDeliverySummary {
	out := types.WriteFinalDeliverySummary{}
	if run == nil && finalPlan == nil {
		return out
	}
	if finalPlan != nil {
		out.FinalPlanID = strings.TrimSpace(finalPlan.ID)
		finalSources, finalTests := writeFinalReportPlanPathRoles(finalPlan)
		out.FinalPlanTestOnly = len(writeFinalReportPlanChangePaths(finalPlan)) > 0 && len(finalSources) == 0
		out.FinalPlanValidationOnly = len(writeFinalReportPlanChangePaths(finalPlan)) == 0 && len(finalPlan.VerificationProbes) > 0
		if len(finalSources) > 0 {
			out.SourcePaths = append(out.SourcePaths, finalSources...)
		}
		if len(finalTests) > 0 {
			out.TestPaths = append(out.TestPaths, finalTests...)
		}
	}
	if report != nil {
		out.ReportPlanID = strings.TrimSpace(report.PlanID)
	}

	restoredByBatch := writeFinalReportRestoredPlanIDsByBatch(run)
	restoredSourceOwner := false
	for _, planID := range o.writeFinalReportAppliedPlanIDs(run) {
		plan := finalPlan
		if plan == nil || strings.TrimSpace(plan.ID) != planID {
			if loaded, err := o.loadWriteFinalReportChangePlan(planID); err == nil && loaded != nil {
				plan = loaded
			} else {
				plan = nil
			}
		}
		if plan == nil {
			continue
		}
		sources, tests := writeFinalReportPlanPathRoles(plan)
		if len(tests) > 0 {
			out.TestPaths = append(out.TestPaths, tests...)
		}
		if len(sources) == 0 {
			continue
		}
		out.SourceOwnerPlanIDs = append(out.SourceOwnerPlanIDs, planID)
		if out.PrimarySourcePlanID == "" {
			out.PrimarySourcePlanID = planID
		}
		out.SourcePaths = append(out.SourcePaths, sources...)
		if writeFinalReportPlanIDInRestoredBatches(planID, restoredByBatch) {
			restoredSourceOwner = true
		}
	}
	if len(out.SourceOwnerPlanIDs) == 0 && finalPlan != nil {
		finalSources, _ := writeFinalReportPlanPathRoles(finalPlan)
		if len(finalSources) > 0 {
			out.SourceOwnerPlanIDs = append(out.SourceOwnerPlanIDs, strings.TrimSpace(finalPlan.ID))
			out.PrimarySourcePlanID = strings.TrimSpace(finalPlan.ID)
			out.SourcePaths = append(out.SourcePaths, finalSources...)
		}
	}
	if len(out.SourceOwnerPlanIDs) > 0 {
		out.Status = "coherent"
		out.Relation = "source_plan_owns_final_delivery"
		if out.FinalPlanTestOnly && out.FinalPlanID != "" && !writeFinalReportStringSliceContains(out.SourceOwnerPlanIDs, out.FinalPlanID) {
			if restoredSourceOwner {
				out.Relation = "source_plan_with_later_same_batch_test_replan"
			} else {
				out.Relation = "source_plan_with_later_test_followup"
			}
		} else if out.FinalPlanValidationOnly && out.FinalPlanID != "" && !writeFinalReportStringSliceContains(out.SourceOwnerPlanIDs, out.FinalPlanID) {
			out.Relation = "source_plan_with_later_validation_followup"
		}
	} else if out.FinalPlanTestOnly || out.FinalPlanValidationOnly {
		out.Status = "empty"
		out.ReasonCode = "delivery_no_source_owner_plan"
	} else if out.FinalPlanID != "" {
		out.Status = "empty"
		out.ReasonCode = "delivery_no_source_patch"
	}
	return types.NormalizeWriteFinalDeliverySummary(out)
}

func (o *Orchestrator) writeFinalReportAppliedPlanIDs(run *types.WriteWorkflowRun) []string {
	if run == nil {
		return nil
	}
	restoreCutoffs := writeFinalReportRestoreCutoffsByBatch(run)
	restoredByBatch := writeFinalReportRestoredPlanIDsByBatch(run)
	seen := map[string]bool{}
	var out []string
	for _, batch := range run.Batches {
		batchID := strings.TrimSpace(batch.ID)
		cutoff := restoreCutoffs[batchID]
		restored := restoredByBatch[batchID]
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) != "apply" || strings.TrimSpace(attempt.Status) != "applied" {
				continue
			}
			planID := strings.TrimSpace(attempt.PlanID)
			if planID == "" {
				continue
			}
			if !cutoff.IsZero() && !restored[planID] {
				at := writeFinalReportAttemptTime(attempt)
				if at.IsZero() || at.Before(cutoff) {
					continue
				}
			}
			if seen[planID] {
				continue
			}
			seen[planID] = true
			out = append(out, planID)
		}
	}
	return out
}

func writeFinalReportRestoreCutoffsByBatch(run *types.WriteWorkflowRun) map[string]time.Time {
	out := map[string]time.Time{}
	if run == nil {
		return out
	}
	for _, item := range run.ProgressLedger {
		if strings.TrimSpace(item.ReasonCode) != "checkpoint_restored_before_replan" {
			continue
		}
		batchID := strings.TrimSpace(item.BatchID)
		if batchID == "" || item.At.IsZero() {
			continue
		}
		if out[batchID].IsZero() || item.At.After(out[batchID]) {
			out[batchID] = item.At
		}
	}
	return out
}

func writeFinalReportRestoredPlanIDsByBatch(run *types.WriteWorkflowRun) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	latest := map[string]time.Time{}
	if run == nil {
		return out
	}
	for _, batch := range run.Batches {
		batchID := strings.TrimSpace(batch.ID)
		if batchID == "" {
			continue
		}
		for _, event := range batch.SliceEvents {
			if event.Event != types.WriteWorkflowSliceEventRestored {
				continue
			}
			planID := strings.TrimSpace(event.PlanID)
			if planID == "" {
				planID = strings.TrimPrefix(strings.TrimSpace(event.ArtifactRef), "refs/codrax/applied/")
			}
			planID = strings.TrimSpace(planID)
			if planID == "" {
				continue
			}
			if latest[batchID].IsZero() || event.At.After(latest[batchID]) {
				latest[batchID] = event.At
				out[batchID] = map[string]bool{planID: true}
			} else if event.At.Equal(latest[batchID]) {
				if out[batchID] == nil {
					out[batchID] = map[string]bool{}
				}
				out[batchID][planID] = true
			}
		}
	}
	return out
}

func writeFinalReportPlanIDInRestoredBatches(planID string, restored map[string]map[string]bool) bool {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return false
	}
	for _, ids := range restored {
		if ids[planID] {
			return true
		}
	}
	return false
}

func writeFinalReportAttemptTime(attempt types.WriteWorkflowAttempt) time.Time {
	if !attempt.FinishedAt.IsZero() {
		return attempt.FinishedAt
	}
	return attempt.StartedAt
}

func writeFinalReportPlanPathRoles(plan *types.ChangePlan) ([]string, []string) {
	var sources, tests []string
	for _, path := range writeFinalReportPlanChangePaths(plan) {
		if types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(path)) {
			tests = append(tests, path)
		} else {
			sources = append(sources, path)
		}
	}
	return writeFinalReportDedupStrings(sources), writeFinalReportDedupStrings(tests)
}

func writeFinalReportPlanChangePaths(plan *types.ChangePlan) []string {
	if plan == nil {
		return nil
	}
	var out []string
	add := func(raw string) {
		p := normalizeControllerPath(raw)
		if p != "" {
			out = append(out, p)
		}
	}
	for _, p := range plan.TargetPaths {
		add(p)
	}
	for _, p := range plan.AppliedPaths {
		add(p)
	}
	for _, change := range plan.Changes {
		add(change.Path)
		add(change.NewPath)
	}
	return writeFinalReportDedupStrings(out)
}

func writeFinalReportStringSliceContains(items []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}

func writeFinalReportDedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		item := strings.TrimSpace(raw)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func (o *Orchestrator) writeFinalReportProofArtifacts(run *types.WriteWorkflowRun, primaryPlan *types.ChangePlan, primaryReport *types.ChangeReport) []types.VerificationProofArtifact {
	if o == nil || run == nil {
		return nil
	}
	var out []types.VerificationProofArtifact
	seen := map[string]bool{}
	add := func(planID, reportID string) {
		planID = strings.TrimSpace(planID)
		reportID = writeFinalReportArtifactBase(reportID)
		if planID == "" && reportID == "" {
			return
		}
		key := planID
		if key == "" {
			key = reportID
		}
		if key != "" {
			if seen[key] {
				return
			}
			seen[key] = true
		}
		var plan *types.ChangePlan
		if primaryPlan != nil && strings.TrimSpace(primaryPlan.ID) == planID {
			plan = primaryPlan
		} else if planID != "" {
			if loaded, err := o.loadWriteFinalReportChangePlan(planID); err == nil && loaded != nil {
				plan = loaded
			}
		}
		var report *types.ChangeReport
		if primaryReport != nil && planID != "" && strings.TrimSpace(primaryReport.PlanID) == planID {
			report = primaryReport
		}
		if report == nil && reportID != "" {
			report = o.loadDurableReportArtifact(reportID)
		}
		if report == nil && planID != "" {
			report = o.loadWriteFinalReportChangeReport(planID)
		}
		if plan == nil && report == nil {
			return
		}
		out = append(out, types.VerificationProofArtifact{Plan: plan, Report: report})
	}
	for _, batch := range run.Batches {
		if batch.Status != types.WriteWorkflowBatchComplete {
			continue
		}
		planID := strings.TrimSpace(batch.PlanID)
		reportID := writeFinalReportBatchReportID(batch, planID)
		add(planID, reportID)
	}
	return out
}

func writeFinalReportBatchReportID(batch types.WriteWorkflowBatch, planID string) string {
	if ref := strings.TrimSpace(batch.VerifyRef); ref != "" {
		return writeFinalReportArtifactBase(ref)
	}
	for i := len(batch.Attempts) - 1; i >= 0; i-- {
		attempt := batch.Attempts[i]
		if reportID := strings.TrimSpace(attempt.ReportID); reportID != "" {
			return writeFinalReportArtifactBase(reportID)
		}
	}
	if planID = strings.TrimSpace(planID); planID != "" {
		if stem := writeWorkflowArtifactFileStem(planID); stem != "" {
			return stem + ".report.json"
		}
	}
	return ""
}

func writeFinalReportArtifactBase(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	base := strings.TrimSpace(filepath.Base(ref))
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

func writeFinalReportArtifactID(run *types.WriteWorkflowRun, plan *types.ChangePlan, report *types.ChangeReport) string {
	if plan != nil && strings.TrimSpace(plan.ID) != "" {
		return strings.TrimSpace(plan.ID)
	}
	if report != nil && strings.TrimSpace(report.PlanID) != "" {
		return strings.TrimSpace(report.PlanID)
	}
	if run != nil {
		activeBatchID := strings.TrimSpace(run.ActiveBatchID)
		for i := len(run.Batches) - 1; i >= 0; i-- {
			batch := run.Batches[i]
			if activeBatchID != "" && strings.TrimSpace(batch.ID) != activeBatchID {
				continue
			}
			if planID := strings.TrimSpace(batch.PlanID); planID != "" {
				return planID
			}
			for j := len(batch.Attempts) - 1; j >= 0; j-- {
				if planID := strings.TrimSpace(batch.Attempts[j].PlanID); planID != "" {
					return planID
				}
			}
		}
		for i := len(run.Batches) - 1; i >= 0; i-- {
			if planID := strings.TrimSpace(run.Batches[i].PlanID); planID != "" {
				return planID
			}
		}
		if runID := strings.TrimSpace(run.RunID); runID != "" {
			return runID
		}
	}
	return ""
}

func (o *Orchestrator) loadWriteFinalReportChangeReport(planID string) *types.ChangeReport {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return nil
	}
	planDir := o.ensureChangeReportDir()
	if planDir == "" {
		return nil
	}
	stem := writeWorkflowArtifactFileStem(planID)
	if stem == "" {
		return nil
	}
	path := filepath.Join(planDir, stem+".report.json")
	report, err := types.LoadChangeReportFromFile(path)
	if err != nil {
		return nil
	}
	return report
}

func (o *Orchestrator) loadWriteFinalReportChangePlan(planID string) (*types.ChangePlan, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return nil, nil
	}
	planDir := o.ensureChangeReportDir()
	if planDir == "" {
		return nil, nil
	}
	stem := writeWorkflowArtifactFileStem(planID)
	if stem == "" {
		return nil, nil
	}
	return types.LoadChangePlanFromFile(filepath.Join(planDir, stem+".json"))
}

func upsertWorkflowRunContextPack(run types.WriteWorkflowRun, pack types.WriteContextPack) types.WriteWorkflowRun {
	pack = types.NormalizeWriteContextPack(pack)
	if len(pack.Items) == 0 {
		return run
	}
	key := workflowContextPackKey(pack)
	for i := range run.ContextPacks {
		if workflowContextPackKey(run.ContextPacks[i]) == key {
			existing := types.NormalizeWriteContextPack(run.ContextPacks[i])
			batchID := strings.TrimSpace(existing.BatchID)
			if batchID == "" {
				batchID = strings.TrimSpace(pack.BatchID)
			}
			goal := strings.TrimSpace(existing.Goal)
			if goal == "" {
				goal = strings.TrimSpace(pack.Goal)
			}
			merged := types.MergeWriteContextPacks(batchID, goal, existing, pack)
			if strings.TrimSpace(existing.PackID) != "" {
				merged.PackID = strings.TrimSpace(existing.PackID)
			} else if strings.TrimSpace(pack.PackID) != "" {
				merged.PackID = strings.TrimSpace(pack.PackID)
			}
			if strings.TrimSpace(existing.SourceStage) != "" {
				merged.SourceStage = strings.TrimSpace(existing.SourceStage)
			} else {
				merged.SourceStage = strings.TrimSpace(pack.SourceStage)
			}
			run.ContextPacks[i] = types.NormalizeWriteContextPack(merged)
			return types.NormalizeWriteWorkflowRun(run)
		}
	}
	run.ContextPacks = append(run.ContextPacks, pack)
	return types.NormalizeWriteWorkflowRun(run)
}

func attachPlanContextPackToWorkflowRun(run types.WriteWorkflowRun, plan *types.ChangePlan) types.WriteWorkflowRun {
	if plan == nil {
		return run
	}
	localizationReview := types.SourceLocalizationReviewFromWritePlanContext(
		strings.TrimSpace(run.ActiveBatchID),
		strings.TrimSpace(run.Goal),
		run.ContextPacks,
		plan,
	)
	plan.LocalizationReview = types.CloneSourceLocalizationReviewPtr(&localizationReview)
	types.StampChangePlanOwnerAnchors(plan, 12)
	pack := types.WriteContextPackFromChangePlan(plan)
	if strings.TrimSpace(run.ActiveBatchID) != "" {
		pack = pack.WithScope(strings.TrimSpace(run.ActiveBatchID), activeWorkflowRunSliceID(run, run.ActiveBatchID))
	}
	coverage := types.WriteContextPackFromPlanContextCoverage(pack.BatchID, pack.Goal, run.ContextPacks, plan)
	if len(coverage.Items) > 0 {
		pack.Items = append(pack.Items, coverage.Items...)
		pack = types.NormalizeWriteContextPack(pack)
	}
	return upsertWorkflowRunContextPack(run, pack)
}

func workflowContextPackKey(pack types.WriteContextPack) string {
	if pack.PackID != "" || pack.BatchID != "" {
		return pack.PackID + "|" + pack.BatchID
	}
	return pack.SourceStage + "|" + pack.Goal
}

func updateWorkflowRunBatchStatus(run *types.WriteWorkflowRun, batchID string, status types.WriteWorkflowBatchStatus) {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			run.Batches[i].Status = status
			if status != types.WriteWorkflowBatchComplete {
				run.Batches[i].Completion = nil
			}
			if run.Batches[i].CreatedAt.IsZero() {
				run.Batches[i].CreatedAt = time.Now()
			}
			run.Batches[i].UpdatedAt = time.Now()
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        batchID,
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
}

func updateWorkflowRunBatchCompletion(run *types.WriteWorkflowRun, batchID string, verdict types.WriteWorkflowCompletionVerdict, reasonCode, source string) {
	if run == nil || strings.TrimSpace(batchID) == "" || verdict == "" {
		return
	}
	completion := &types.WriteWorkflowCompletion{
		Verdict:    verdict,
		ReasonCode: strings.TrimSpace(reasonCode),
		Source:     strings.TrimSpace(source),
		At:         time.Now(),
	}
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			run.Batches[i].Completion = completion
			if run.Batches[i].CreatedAt.IsZero() {
				run.Batches[i].CreatedAt = time.Now()
			}
			run.Batches[i].UpdatedAt = time.Now()
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:         batchID,
		Completion: completion,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	})
}

func updateWorkflowRunBatchPlan(run *types.WriteWorkflowRun, batchID string, plan *types.ChangePlan, priorPlans ...*types.ChangePlan) {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	var priorPlan *types.ChangePlan
	if len(priorPlans) > 0 {
		priorPlan = priorPlans[0]
	}
	planID := ""
	if plan != nil {
		plan.Slices = types.NormalizeChangePlanSlices(plan, types.OnlineChangePlanSliceOptions(plan))
		stampChangePlanImpactObligations(plan, nil)
		planID = strings.TrimSpace(plan.ID)
	}
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			priorPlanID := strings.TrimSpace(run.Batches[i].PlanID)
			if workflowBatchHasFailedExecutionStateForPlan(run.Batches[i], priorPlanID) {
				priorPlanID = ""
			}
			run.Batches[i].PlanID = planID
			run.Batches[i].PlanRef = planID
			initializeWorkflowBatchSlicesFromPlan(&run.Batches[i], plan, priorPlanID, priorPlan)
			run.Batches[i].UpdatedAt = time.Now()
			appendWorkflowBatchAttempt(&run.Batches[i], "plan", "complete", "", planID, "", planID)
			return
		}
	}
	batch := types.WriteWorkflowBatch{
		ID:        batchID,
		PlanID:    planID,
		PlanRef:   planID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: []types.WriteWorkflowAttempt{{
			ID:          fmt.Sprintf("attempt-%d", 1),
			Kind:        "plan",
			Status:      "complete",
			PlanID:      planID,
			ArtifactRef: planID,
			FinishedAt:  time.Now(),
		}},
	}
	initializeWorkflowBatchSlicesFromPlan(&batch, plan, "", nil)
	run.Batches = append(run.Batches, batch)
}

func initializeWorkflowBatchSlicesFromPlan(batch *types.WriteWorkflowBatch, plan *types.ChangePlan, priorPlanID string, priorPlan *types.ChangePlan) {
	if batch == nil || plan == nil {
		return
	}
	planID := strings.TrimSpace(plan.ID)
	if len(plan.Changes) == 0 {
		if strings.TrimSpace(priorPlanID) != planID {
			batch.Slices = nil
			batch.ActiveSliceID = ""
			batch.SliceEvents = nil
		}
		return
	}
	plan.Slices = types.NormalizeChangePlanSlices(plan, types.OnlineChangePlanSliceOptions(plan))
	derived := types.WriteWorkflowSlicesFromChangePlan(plan)
	if len(derived) == 0 {
		if strings.TrimSpace(priorPlanID) != planID {
			batch.Slices = nil
			batch.ActiveSliceID = ""
			batch.SliceEvents = nil
		}
		return
	}
	now := time.Now()
	if strings.TrimSpace(priorPlanID) == planID && workflowSlicesAlign(batch.Slices, derived) {
		mergeWorkflowBatchSlices(batch, derived, planID, now)
	} else {
		if priorPlan != nil {
			derived = writeflow.PreserveVerifiedRuntimeUnits(batch.Slices, priorPlan, plan, derived)
		}
		batch.Slices = derived
		for i := range batch.Slices {
			batch.Slices[i].PlanID = planID
			if batch.Slices[i].CreatedAt.IsZero() {
				batch.Slices[i].CreatedAt = now
			}
			if batch.Slices[i].UpdatedAt.IsZero() {
				batch.Slices[i].UpdatedAt = now
			}
			batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
				SliceID: batch.Slices[i].ID,
				Event:   types.WriteWorkflowSliceEventPlanned,
				Status:  batch.Slices[i].Status,
				PlanID:  planID,
				At:      now,
			})
		}
	}
	batch.ActiveSliceID = workflowBatchActiveSliceID(batch)
}

func workflowSlicesAlign(existing, derived []types.WriteWorkflowSlice) bool {
	if len(existing) == 0 || len(existing) != len(derived) {
		return false
	}
	for i := range existing {
		if strings.TrimSpace(existing[i].ID) != strings.TrimSpace(derived[i].ID) {
			return false
		}
		if !sameIntSet(existing[i].ChangeIndexes, derived[i].ChangeIndexes) {
			return false
		}
	}
	return true
}

func mergeWorkflowBatchSlices(batch *types.WriteWorkflowBatch, derived []types.WriteWorkflowSlice, planID string, now time.Time) {
	if batch == nil {
		return
	}
	existing := make(map[string]types.WriteWorkflowSlice, len(batch.Slices))
	for _, slice := range batch.Slices {
		if id := strings.TrimSpace(slice.ID); id != "" {
			existing[id] = slice
		}
	}
	out := make([]types.WriteWorkflowSlice, 0, len(derived))
	for _, slice := range derived {
		id := strings.TrimSpace(slice.ID)
		if prior, ok := existing[id]; ok {
			slice.Status = prior.Status
			slice.ApplyRef = prior.ApplyRef
			slice.ObserveRef = prior.ObserveRef
			slice.VerifyRef = prior.VerifyRef
			slice.ApprovalRef = prior.ApprovalRef
			if prior.Checkpoint != nil {
				cp := *prior.Checkpoint
				slice.Checkpoint = &cp
			}
			if prior.Restore != nil {
				restore := *prior.Restore
				slice.Restore = &restore
			}
			slice.ContextPackIDs = append([]string(nil), prior.ContextPackIDs...)
			slice.Completion = prior.Completion
			slice.Attempts = append([]types.WriteWorkflowAttempt(nil), prior.Attempts...)
			slice.CreatedAt = prior.CreatedAt
			slice.UpdatedAt = prior.UpdatedAt
		}
		if slice.Status == "" {
			slice.Status = types.ChangePlanSlicePending
		}
		slice.PlanID = planID
		if slice.CreatedAt.IsZero() {
			slice.CreatedAt = now
		}
		if slice.UpdatedAt.IsZero() {
			slice.UpdatedAt = now
		}
		out = append(out, slice)
	}
	batch.Slices = out
}

func workflowBatchActiveSliceID(batch *types.WriteWorkflowBatch) string {
	if batch == nil || len(batch.Slices) == 0 {
		return ""
	}
	current := strings.TrimSpace(batch.ActiveSliceID)
	if current != "" {
		for _, slice := range batch.Slices {
			if strings.TrimSpace(slice.ID) == current && !workflowSliceStatusTerminal(slice.Status) {
				return current
			}
		}
	}
	for _, slice := range batch.Slices {
		if !workflowSliceStatusTerminal(slice.Status) {
			return strings.TrimSpace(slice.ID)
		}
	}
	return strings.TrimSpace(batch.Slices[len(batch.Slices)-1].ID)
}

func sameIntSet(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[int]int{}
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func updateWorkflowRunBatchExplore(run *types.WriteWorkflowRun, batchID, status, reasonCode string) {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			run.Batches[i].UpdatedAt = time.Now()
			appendWorkflowBatchAttempt(&run.Batches[i], "explore", strings.TrimSpace(status), strings.TrimSpace(reasonCode), "", "", "")
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        batchID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: []types.WriteWorkflowAttempt{{
			ID:         fmt.Sprintf("attempt-%d", 1),
			Kind:       "explore",
			Status:     strings.TrimSpace(status),
			ReasonCode: strings.TrimSpace(reasonCode),
			StartedAt:  time.Now(),
			FinishedAt: time.Now(),
		}},
	})
}

func updateWorkflowRunBatchApply(run *types.WriteWorkflowRun, batchID string, plan *types.ChangePlan, status, reasonCode string) {
	if run == nil || strings.TrimSpace(batchID) == "" || plan == nil {
		return
	}
	planID := strings.TrimSpace(plan.ID)
	applyRef := writeWorkflowApplyRef(plan, status)
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			if applyRef != "" {
				run.Batches[i].ApplyRef = applyRef
			}
			run.Batches[i].UpdatedAt = time.Now()
			appendWorkflowBatchAttempt(&run.Batches[i], "apply", status, reasonCode, planID, "", applyRef)
			updateWorkflowBatchActiveSliceApply(&run.Batches[i], plan, status, reasonCode, applyRef)
			return
		}
	}
	batch := types.WriteWorkflowBatch{
		ID:        batchID,
		ApplyRef:  applyRef,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: []types.WriteWorkflowAttempt{{
			ID:          fmt.Sprintf("attempt-%d", 1),
			Kind:        "apply",
			Status:      strings.TrimSpace(status),
			ReasonCode:  strings.TrimSpace(reasonCode),
			PlanID:      planID,
			ArtifactRef: applyRef,
			FinishedAt:  time.Now(),
		}},
	}
	initializeWorkflowBatchSlicesFromPlan(&batch, plan, "", nil)
	updateWorkflowBatchActiveSliceApply(&batch, plan, status, reasonCode, applyRef)
	run.Batches = append(run.Batches, batch)
}

func updateWorkflowBatchActiveSliceApply(batch *types.WriteWorkflowBatch, plan *types.ChangePlan, status, reasonCode, applyRef string) {
	if batch == nil || plan == nil || strings.TrimSpace(batch.ActiveSliceID) == "" {
		return
	}
	slice := activeWorkflowSliceForUpdate(batch)
	if slice == nil {
		return
	}
	now := time.Now()
	planID := strings.TrimSpace(plan.ID)
	status = strings.TrimSpace(status)
	reasonCode = strings.TrimSpace(reasonCode)
	applyRef = strings.TrimSpace(applyRef)
	slice.PlanID = firstNonEmptyController(planID, slice.PlanID)
	if applyRef != "" {
		slice.ApplyRef = applyRef
	}
	switch status {
	case "applied":
		slice.Status = types.ChangePlanSliceObserving
		slice.Completion = nil
		if checkpoint := writeWorkflowSliceCheckpointFromPlan(plan, reasonCode, "slice_apply", now); checkpoint != nil {
			slice.Checkpoint = checkpoint
		}
	case "partial", "failed":
		slice.Status = types.ChangePlanSliceFailed
		slice.Completion = nil
	case "blocked":
		slice.Status = types.ChangePlanSliceBlocked
		slice.Completion = &types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionAcceptedFailed,
			ReasonCode: reasonCode,
			Source:     "slice_apply",
			At:         now,
		}
	default:
		return
	}
	appendWorkflowSliceAttempt(slice, "apply", status, reasonCode, planID, "", applyRef)
	batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
		SliceID:     slice.ID,
		Event:       types.WriteWorkflowSliceEventApplyCompleted,
		Status:      slice.Status,
		ReasonCode:  reasonCode,
		PlanID:      planID,
		ArtifactRef: applyRef,
		At:          now,
	})
	if slice.Status == types.ChangePlanSliceFailed {
		batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
			SliceID:     slice.ID,
			Event:       types.WriteWorkflowSliceEventFailed,
			Status:      slice.Status,
			ReasonCode:  reasonCode,
			PlanID:      planID,
			ArtifactRef: applyRef,
			At:          now,
		})
	}
	if slice.Status == types.ChangePlanSliceBlocked {
		batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
			SliceID:     slice.ID,
			Event:       types.WriteWorkflowSliceEventBlocked,
			Status:      slice.Status,
			ReasonCode:  reasonCode,
			PlanID:      planID,
			ArtifactRef: applyRef,
			At:          now,
		})
	}
	slice.UpdatedAt = now
	batch.UpdatedAt = now
}

func markWorkflowRunActiveSlicePatchReviewBlocked(run *types.WriteWorkflowRun, batchID string, plan *types.ChangePlan, reasonCode string) {
	if run == nil || plan == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	batch := activeWorkflowBatchForUpdate(run, batchID)
	if batch == nil {
		return
	}
	slice := activeWorkflowSliceForUpdate(batch)
	if slice == nil {
		return
	}
	now := time.Now()
	planID := strings.TrimSpace(plan.ID)
	reasonCode = strings.TrimSpace(reasonCode)
	slice.PlanID = firstNonEmptyController(planID, slice.PlanID)
	slice.Status = types.ChangePlanSliceBlocked
	slice.Completion = &types.WriteWorkflowCompletion{
		Verdict:    types.WriteWorkflowCompletionAcceptedFailed,
		ReasonCode: reasonCode,
		Source:     "patch_review",
		At:         now,
	}
	appendWorkflowSliceAttempt(slice, "patch_review", "blocked", reasonCode, planID, "", "")
	batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
		SliceID:    slice.ID,
		Event:      types.WriteWorkflowSliceEventBlocked,
		Status:     slice.Status,
		ReasonCode: reasonCode,
		PlanID:     planID,
		At:         now,
	})
	slice.UpdatedAt = now
	batch.UpdatedAt = now
}

func markWorkflowRunActiveSlicePatchReviewFailed(run *types.WriteWorkflowRun, batchID string, plan *types.ChangePlan, reasonCode string) {
	if run == nil || plan == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	batch := activeWorkflowBatchForUpdate(run, batchID)
	if batch == nil {
		return
	}
	slice := activeWorkflowSliceForUpdate(batch)
	if slice == nil {
		return
	}
	now := time.Now()
	planID := strings.TrimSpace(plan.ID)
	reasonCode = strings.TrimSpace(reasonCode)
	slice.PlanID = firstNonEmptyController(planID, slice.PlanID)
	slice.Status = types.ChangePlanSliceFailed
	slice.Completion = nil
	appendWorkflowSliceAttempt(slice, "patch_review", "failed", reasonCode, planID, "", "")
	appendWorkflowBatchAttempt(batch, "patch_review", "failed", reasonCode, planID, "", "")
	batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
		SliceID:    slice.ID,
		Event:      types.WriteWorkflowSliceEventFailed,
		Status:     slice.Status,
		ReasonCode: reasonCode,
		PlanID:     planID,
		At:         now,
	})
	slice.UpdatedAt = now
	batch.UpdatedAt = now
}

func markWorkflowRunActiveSliceObservingForRestoredPlan(run *types.WriteWorkflowRun, batchID string, plan *types.ChangePlan, reasonCode string) {
	if run == nil || plan == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	batch := activeWorkflowBatchForUpdate(run, batchID)
	if batch == nil {
		return
	}
	if len(batch.Slices) == 0 {
		initializeWorkflowBatchSlicesFromPlan(batch, plan, strings.TrimSpace(batch.PlanID), nil)
	}
	if strings.TrimSpace(batch.ActiveSliceID) == "" {
		batch.ActiveSliceID = workflowBatchActiveSliceID(batch)
	}
	slice := activeWorkflowSliceForUpdate(batch)
	if slice == nil {
		return
	}
	now := time.Now()
	planID := strings.TrimSpace(plan.ID)
	slice.PlanID = firstNonEmptyController(planID, slice.PlanID)
	slice.Status = types.ChangePlanSliceObserving
	slice.Completion = nil
	if slice.ApplyRef == "" {
		slice.ApplyRef = writeWorkflowApplyRef(plan, "applied")
	}
	if checkpoint := writeWorkflowSliceCheckpointFromPlan(plan, reasonCode, "restored_plan", now); checkpoint != nil {
		if slice.Checkpoint == nil {
			cp := *checkpoint
			slice.Checkpoint = &cp
		}
		slice.Restore = &types.WriteWorkflowRestore{
			CheckpointRef: checkpoint.Ref,
			CommitSHA:     checkpoint.CommitSHA,
			WorktreePath:  checkpoint.WorktreePath,
			Source:        "planner_probe",
			ReasonCode:    strings.TrimSpace(reasonCode),
			At:            now,
		}
	}
	slice.UpdatedAt = now
	batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
		SliceID:    slice.ID,
		Event:      types.WriteWorkflowSliceEventObserveStarted,
		Status:     slice.Status,
		ReasonCode: strings.TrimSpace(reasonCode),
		PlanID:     planID,
		At:         now,
	})
	batch.UpdatedAt = now
}

func (o *Orchestrator) restoreActiveSliceCheckpointBeforeReplan(run *types.WriteWorkflowRun, reasonCode string) (bool, error) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || strings.TrimSpace(run.ActiveBatchID) == "" {
		return false, nil
	}
	batch := activeWorkflowBatchForUpdate(run, run.ActiveBatchID)
	if batch == nil || strings.TrimSpace(batch.ActiveSliceID) == "" {
		return false, nil
	}
	slice := activeWorkflowSliceForUpdate(batch)
	if slice == nil || slice.Checkpoint == nil {
		return false, nil
	}
	checkpoint := *slice.Checkpoint
	sha := strings.TrimSpace(checkpoint.CommitSHA)
	if sha == "" {
		return false, nil
	}
	wt, ok, reason := o.safeWorkflowCheckpointWorktreePath(checkpoint.WorktreePath)
	if !ok {
		if reason != "" {
			appendControllerProgress(run, run.ActiveBatchID, "checkpoint_restore_skipped_"+reason, "checkpoint restore skipped before replan")
		}
		return false, nil
	}
	if err := worktree.ResetHard(wt, sha); err != nil {
		return false, err
	}
	o.busCtx.WorktreePath = wt
	o.busCtx.RepoRoot = wt
	o.busCtx.Mutable.SetRepoRoot(wt)
	if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
		changed := false
		if strings.TrimSpace(plan.WorktreePath) == "" {
			plan.WorktreePath = wt
			changed = true
		}
		if strings.TrimSpace(plan.AppliedCommitSHA) == "" {
			plan.AppliedCommitSHA = sha
			changed = true
		}
		if changed {
			o.busCtx.Mutable.SetChangePlan(plan)
			o.persistCurrentChangePlanSnapshot()
		}
	}
	recordWorkflowActiveSliceRestore(batch, slice, checkpoint, wt, strings.TrimSpace(reasonCode), "checkpoint_rewind")
	return true, nil
}

func (o *Orchestrator) safeWorkflowCheckpointWorktreePath(raw string) (string, bool, string) {
	path := strings.TrimSpace(raw)
	if path == "" && o != nil && o.busCtx != nil {
		path = strings.TrimSpace(o.busCtx.WorktreePath)
	}
	if path == "" {
		return "", false, "missing_worktree"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false, "invalid_worktree"
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return "", false, "missing_worktree"
	}
	if o != nil && o.busCtx != nil && sameCleanPath(abs, o.busCtx.WorktreePath) {
		return abs, true, ""
	}
	if o != nil && pathWithinCleanDir(abs, o.worktreeBase) {
		return abs, true, ""
	}
	return "", false, "untrusted_worktree"
}

func recordWorkflowActiveSliceRestore(batch *types.WriteWorkflowBatch, slice *types.WriteWorkflowSlice, checkpoint types.WriteWorkflowCheckpoint, worktreePath, reasonCode, source string) {
	if batch == nil || slice == nil {
		return
	}
	now := time.Now()
	reasonCode = strings.TrimSpace(reasonCode)
	source = strings.TrimSpace(source)
	restore := &types.WriteWorkflowRestore{
		CheckpointRef: strings.TrimSpace(checkpoint.Ref),
		CommitSHA:     strings.TrimSpace(checkpoint.CommitSHA),
		WorktreePath:  strings.TrimSpace(worktreePath),
		Source:        source,
		ReasonCode:    reasonCode,
		At:            now,
	}
	slice.Restore = restore
	appendWorkflowSliceAttempt(slice, "restore", "restored", reasonCode, strings.TrimSpace(slice.PlanID), "", restore.CheckpointRef)
	batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
		SliceID:     slice.ID,
		Event:       types.WriteWorkflowSliceEventRestored,
		Status:      slice.Status,
		ReasonCode:  reasonCode,
		PlanID:      strings.TrimSpace(slice.PlanID),
		ArtifactRef: restore.CheckpointRef,
		At:          now,
	})
	slice.UpdatedAt = now
	batch.UpdatedAt = now
}

func sameCleanPath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

func pathWithinCleanDir(path, dir string) bool {
	path = strings.TrimSpace(path)
	dir = strings.TrimSpace(dir)
	if path == "" || dir == "" {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(dirAbs), filepath.Clean(pathAbs))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func markWorkflowRunActiveSliceApplying(run *types.WriteWorkflowRun, batchID string, plan *types.ChangePlan, reasonCode string) {
	if run == nil || plan == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	batch := activeWorkflowBatchForUpdate(run, batchID)
	if batch == nil {
		return
	}
	if len(batch.Slices) == 0 {
		initializeWorkflowBatchSlicesFromPlan(batch, plan, strings.TrimSpace(batch.PlanID), nil)
	}
	if strings.TrimSpace(batch.ActiveSliceID) == "" {
		batch.ActiveSliceID = workflowBatchActiveSliceID(batch)
	}
	slice := activeWorkflowSliceForUpdate(batch)
	if slice == nil || workflowSliceStatusTerminal(slice.Status) {
		return
	}
	now := time.Now()
	planID := strings.TrimSpace(plan.ID)
	reasonCode = strings.TrimSpace(reasonCode)
	slice.PlanID = firstNonEmptyController(planID, slice.PlanID)
	slice.Status = types.ChangePlanSliceApplying
	slice.Completion = nil
	appendWorkflowSliceAttempt(slice, "apply", "started", reasonCode, planID, "", "")
	batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
		SliceID:    slice.ID,
		Event:      types.WriteWorkflowSliceEventApplyStarted,
		Status:     slice.Status,
		ReasonCode: reasonCode,
		PlanID:     planID,
		At:         now,
	})
	slice.UpdatedAt = now
	batch.UpdatedAt = now
}

func updateWorkflowRunBatchVerify(run *types.WriteWorkflowRun, batchID string, report *types.ChangeReport, status, reasonCode string) {
	if run == nil || strings.TrimSpace(batchID) == "" || report == nil {
		return
	}
	reportID := writeWorkflowReportID(report)
	planID := strings.TrimSpace(report.PlanID)
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			run.Batches[i].VerifyRef = reportID
			run.Batches[i].UpdatedAt = time.Now()
			appendWorkflowBatchAttemptWithFailureReason(&run.Batches[i], "verify", status, reasonCode,
				writeWorkflowVerifyAttemptFailureReason(report), planID, reportID, reportID)
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        batchID,
		VerifyRef: reportID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: []types.WriteWorkflowAttempt{{
			ID:                fmt.Sprintf("attempt-%d", 1),
			Kind:              "verify",
			Status:            strings.TrimSpace(status),
			ReasonCode:        strings.TrimSpace(reasonCode),
			FailureReasonCode: strings.TrimSpace(writeWorkflowVerifyAttemptFailureReason(report)),
			PlanID:            planID,
			ReportID:          reportID,
			ArtifactRef:       reportID,
			FinishedAt:        time.Now(),
		}},
	})
}

func updateWorkflowRunBatchVerifySkipped(run *types.WriteWorkflowRun, batchID, planID string) {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			run.Batches[i].UpdatedAt = time.Now()
			appendWorkflowBatchAttempt(&run.Batches[i], "verify", "skipped", "skip_verify", strings.TrimSpace(planID), "", "")
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        batchID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: []types.WriteWorkflowAttempt{{
			ID:         fmt.Sprintf("attempt-%d", 1),
			Kind:       "verify",
			Status:     "skipped",
			ReasonCode: "skip_verify",
			PlanID:     strings.TrimSpace(planID),
			FinishedAt: time.Now(),
		}},
	})
}

func updateWorkflowRunActiveSliceObserve(run *types.WriteWorkflowRun, batchID string, report *types.ChangeReport, status, reasonCode string) {
	if run == nil || strings.TrimSpace(batchID) == "" || report == nil {
		return
	}
	batch := activeWorkflowBatchForUpdate(run, batchID)
	if batch == nil || strings.TrimSpace(batch.ActiveSliceID) == "" {
		return
	}
	slice := activeWorkflowSliceForUpdate(batch)
	if slice == nil {
		return
	}
	now := time.Now()
	reportID := writeWorkflowReportID(report)
	planID := strings.TrimSpace(report.PlanID)
	observeStatus, terminalEvent, completionVerdict := observeSliceStatusFromVerifyAttempt(status, report)
	slice.Status = observeStatus
	slice.PlanID = firstNonEmptyController(planID, slice.PlanID)
	slice.ObserveRef = reportID
	slice.VerifyRef = reportID
	if completionVerdict != "" {
		slice.Completion = &types.WriteWorkflowCompletion{
			Verdict:    completionVerdict,
			ReasonCode: strings.TrimSpace(reasonCode),
			Source:     "slice_observe",
			At:         now,
		}
	} else {
		slice.Completion = nil
	}
	appendWorkflowSliceAttemptWithFailureReason(slice, "observe", strings.TrimSpace(status), strings.TrimSpace(reasonCode),
		writeWorkflowVerifyAttemptFailureReason(report), planID, reportID, reportID)
	batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
		SliceID:     slice.ID,
		Event:       types.WriteWorkflowSliceEventObserveCompleted,
		Status:      observeStatus,
		ReasonCode:  strings.TrimSpace(reasonCode),
		PlanID:      planID,
		ReportID:    reportID,
		ArtifactRef: reportID,
		At:          now,
	})
	if terminalEvent != "" {
		batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
			SliceID:     slice.ID,
			Event:       terminalEvent,
			Status:      observeStatus,
			ReasonCode:  strings.TrimSpace(reasonCode),
			PlanID:      planID,
			ReportID:    reportID,
			ArtifactRef: reportID,
			At:          now,
		})
	}
	batch.UpdatedAt = now
}

func updateWorkflowRunActiveSliceObserveSkipped(run *types.WriteWorkflowRun, batchID, planID string) {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	batch := activeWorkflowBatchForUpdate(run, batchID)
	if batch == nil || strings.TrimSpace(batch.ActiveSliceID) == "" {
		return
	}
	slice := activeWorkflowSliceForUpdate(batch)
	if slice == nil {
		return
	}
	now := time.Now()
	slice.Status = types.ChangePlanSliceUnverified
	slice.PlanID = firstNonEmptyController(strings.TrimSpace(planID), slice.PlanID)
	slice.Completion = &types.WriteWorkflowCompletion{
		Verdict:    types.WriteWorkflowCompletionUnverified,
		ReasonCode: "skip_verify",
		Source:     "slice_observe",
		At:         now,
	}
	appendWorkflowSliceAttempt(slice, "observe", "skipped", "skip_verify", strings.TrimSpace(planID), "", "")
	batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
		SliceID:    slice.ID,
		Event:      types.WriteWorkflowSliceEventUnverified,
		Status:     types.ChangePlanSliceUnverified,
		ReasonCode: "skip_verify",
		PlanID:     strings.TrimSpace(planID),
		At:         now,
	})
	batch.UpdatedAt = now
}

func (o *Orchestrator) advanceWorkflowAfterSuccessfulSliceObserve(run *types.WriteWorkflowRun, outcome writeflow.VerifyAttemptOutcome) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return false
	}
	batch := activeWorkflowBatchForUpdate(run, run.ActiveBatchID)
	if batch == nil || len(batch.Slices) == 0 || strings.TrimSpace(batch.ActiveSliceID) == "" {
		return false
	}
	active := activeWorkflowSliceForUpdate(batch)
	if active == nil {
		return false
	}
	if active.Status != types.ChangePlanSliceVerified && active.Status != types.ChangePlanSliceUnverified {
		return false
	}
	plan := o.busCtx.Mutable.ChangePlan()
	units := writeflow.DeriveRuntimeUnitViews(o.busCtx.Mode, *run, plan, nil)
	if nextID := writeflow.NextRunnableRuntimeUnitID(units); nextID != "" {
		now := time.Now()
		batch.ActiveSliceID = nextID
		if next := activeWorkflowSliceForUpdate(batch); next != nil {
			if next.Status == "" {
				next.Status = types.ChangePlanSlicePending
			}
			next.UpdatedAt = now
			batch.SliceEvents = append(batch.SliceEvents, types.WriteWorkflowSliceEvent{
				SliceID:    next.ID,
				Event:      types.WriteWorkflowSliceEventPlanned,
				Status:     next.Status,
				ReasonCode: "next_slice_selected",
				PlanID:     next.PlanID,
				At:         now,
			})
		}
		batch.Status = types.WriteWorkflowBatchPlanned
		batch.Completion = nil
		batch.UpdatedAt = now
		run.Status = types.WriteWorkflowRunInProgress
		appendControllerProgress(run, batch.ID, "slice_observed_advance", nextID)
		o.prepareMutableStateForNextWorkflowSlice(run)
		return true
	}
	verdict, reasonCode := workflowBatchSliceCompletion(batch, outcome)
	if verdict == "" {
		return false
	}
	updateWorkflowRunBatchStatus(run, batch.ID, types.WriteWorkflowBatchComplete)
	updateWorkflowRunBatchCompletion(run, batch.ID, verdict, reasonCode, "slice_observe")
	if verdict == types.WriteWorkflowCompletionUnverified {
		appendControllerProgress(run, batch.ID, "batch_unverified", reasonCode)
	} else {
		appendControllerProgress(run, batch.ID, "batch_verified", reasonCode)
	}
	o.busCtx.TaskState.LastError = ""
	o.busCtx.Mutable.ResetVerifyFailureHandoff()
	return true
}

func (o *Orchestrator) prepareMutableStateForNextWorkflowSlice(run *types.WriteWorkflowRun) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return
	}
	o.busCtx.Mutable.SetWriteWorkflowRun(run)
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		return
	}
	plan.Slices = types.NormalizeChangePlanSlices(plan, types.OnlineChangePlanSliceOptions(plan))
	plan.Status = types.PlanStatusPending
	plan.AppliedAt = nil
	o.busCtx.Mutable.SetChangePlan(plan)
	o.busCtx.Mutable.ResetChangeReport()
	pending, _ := pendingAppliesForActivePlanScope(o.busCtx.Mutable, plan, "online_slice")
	o.busCtx.Mutable.WriteClosure().ReplacePendingApplies(pending)
	o.persistCurrentChangePlanSnapshot()
}

func workflowBatchSliceCompletion(batch *types.WriteWorkflowBatch, outcome writeflow.VerifyAttemptOutcome) (types.WriteWorkflowCompletionVerdict, string) {
	if batch == nil || len(batch.Slices) == 0 {
		return "", ""
	}
	reason := strings.TrimSpace(outcome.ReasonCode)
	verdict := types.WriteWorkflowCompletionVerified
	for _, slice := range batch.Slices {
		switch slice.Status {
		case types.ChangePlanSliceVerified:
			continue
		case types.ChangePlanSliceUnverified:
			verdict = types.WriteWorkflowCompletionUnverified
			if reason == "" && slice.Completion != nil {
				reason = strings.TrimSpace(slice.Completion.ReasonCode)
			}
		default:
			return "", ""
		}
	}
	if reason == "" {
		if verdict == types.WriteWorkflowCompletionVerified {
			reason = "all_slices_verified"
		} else {
			reason = "slice_unverified"
		}
	}
	return verdict, reason
}

func workflowSliceStatusTerminal(status types.ChangePlanSliceStatus) bool {
	switch status {
	case types.ChangePlanSliceVerified, types.ChangePlanSliceUnverified, types.ChangePlanSliceFailed, types.ChangePlanSliceBlocked:
		return true
	default:
		return false
	}
}

func stampChangePlanImpactObligations(plan *types.ChangePlan, graph writeimpact.GraphProvider) {
	if plan == nil {
		return
	}
	analysis := writeimpact.BuildAnalysisResult(writeimpact.Input{Plan: plan, PatchEffect: plan.PatchEffect, Graph: graph})
	plan.ImpactAnalysis = &analysis
	if analysis.ObligationSet != nil {
		set := *analysis.ObligationSet
		plan.ImpactObligations = &set
		return
	}
	impact := writeimpact.BuildObligationSet(writeimpact.Input{Plan: plan, PatchEffect: plan.PatchEffect, Graph: graph})
	plan.ImpactObligations = &impact
}

func activeWorkflowBatchForUpdate(run *types.WriteWorkflowRun, batchID string) *types.WriteWorkflowBatch {
	if run == nil {
		return nil
	}
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		batchID = strings.TrimSpace(run.ActiveBatchID)
	}
	if batchID == "" {
		return nil
	}
	for i := range run.Batches {
		if strings.TrimSpace(run.Batches[i].ID) == batchID {
			return &run.Batches[i]
		}
	}
	return nil
}

func activeWorkflowSliceForUpdate(batch *types.WriteWorkflowBatch) *types.WriteWorkflowSlice {
	if batch == nil {
		return nil
	}
	activeID := strings.TrimSpace(batch.ActiveSliceID)
	if activeID == "" && len(batch.Slices) > 0 {
		activeID = strings.TrimSpace(batch.Slices[0].ID)
		batch.ActiveSliceID = activeID
	}
	if activeID == "" {
		return nil
	}
	for i := range batch.Slices {
		if strings.TrimSpace(batch.Slices[i].ID) == activeID {
			return &batch.Slices[i]
		}
	}
	return nil
}

func observeSliceStatusFromVerifyAttempt(status string, report *types.ChangeReport) (types.ChangePlanSliceStatus, types.WriteWorkflowSliceEventKind, types.WriteWorkflowCompletionVerdict) {
	switch strings.TrimSpace(status) {
	case "passed":
		return types.ChangePlanSliceVerified, types.WriteWorkflowSliceEventVerified, types.WriteWorkflowCompletionVerified
	case "unverified", "skipped":
		return types.ChangePlanSliceUnverified, types.WriteWorkflowSliceEventUnverified, types.WriteWorkflowCompletionUnverified
	}
	if report != nil && report.NormalizeVerificationStatus() == types.VerificationStatusUnavailable {
		return types.ChangePlanSliceUnverified, types.WriteWorkflowSliceEventUnverified, types.WriteWorkflowCompletionUnverified
	}
	return types.ChangePlanSliceFailed, types.WriteWorkflowSliceEventFailed, ""
}

func appendWorkflowSliceAttempt(slice *types.WriteWorkflowSlice, kind, status, reasonCode, planID, reportID, artifactRef string) {
	appendWorkflowSliceAttemptWithFailureReason(slice, kind, status, reasonCode, "", planID, reportID, artifactRef)
}

func appendWorkflowSliceAttemptWithFailureReason(slice *types.WriteWorkflowSlice, kind, status, reasonCode, failureReasonCode, planID, reportID, artifactRef string) {
	if slice == nil {
		return
	}
	slice.Attempts = append(slice.Attempts, types.WriteWorkflowAttempt{
		ID:                fmt.Sprintf("attempt-%d", len(slice.Attempts)+1),
		Kind:              strings.TrimSpace(kind),
		Status:            strings.TrimSpace(status),
		ReasonCode:        strings.TrimSpace(reasonCode),
		FailureReasonCode: strings.TrimSpace(failureReasonCode),
		PlanID:            strings.TrimSpace(planID),
		ReportID:          strings.TrimSpace(reportID),
		ArtifactRef:       strings.TrimSpace(artifactRef),
		StartedAt:         time.Now(),
		FinishedAt:        time.Now(),
	})
}

func firstNonEmptyController(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func updateWorkflowRunBatchVerifyInfra(run *types.WriteWorkflowRun, batchID, planID, status, reasonCode string) {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			run.Batches[i].UpdatedAt = time.Now()
			appendWorkflowBatchAttemptWithFailureReason(&run.Batches[i], "verify", status, reasonCode, reasonCode, strings.TrimSpace(planID), "", "")
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        batchID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: []types.WriteWorkflowAttempt{{
			ID:                fmt.Sprintf("attempt-%d", 1),
			Kind:              "verify",
			Status:            strings.TrimSpace(status),
			ReasonCode:        strings.TrimSpace(reasonCode),
			FailureReasonCode: strings.TrimSpace(reasonCode),
			PlanID:            strings.TrimSpace(planID),
			FinishedAt:        time.Now(),
		}},
	})
}

func writeWorkflowApplyAttemptStatus(plan *types.ChangePlan, err error) string {
	if err == nil {
		return "applied"
	}
	if plan == nil {
		return "failed"
	}
	if writePlanNeedsManualApproval(plan) {
		return string(types.WriteWorkflowBatchPendingApproval)
	}
	switch plan.Status {
	case types.PlanStatusBlocked:
		return "blocked"
	case types.PlanStatusPartiallyApplied:
		return "partial"
	default:
		return "failed"
	}
}

func writeWorkflowApplyAttemptReason(plan *types.ChangePlan, err error) string {
	if err == nil {
		return "apply_succeeded"
	}
	if plan == nil {
		return "apply_error"
	}
	if writePlanNeedsManualApproval(plan) {
		return "approval_required"
	}
	switch plan.Status {
	case types.PlanStatusBlocked:
		return "approval_denied"
	case types.PlanStatusPartiallyApplied:
		return "partial_apply"
	default:
		return "apply_error"
	}
}

func writeWorkflowApplyRef(plan *types.ChangePlan, status string) string {
	if plan == nil || strings.TrimSpace(plan.ID) == "" || strings.TrimSpace(status) != "applied" {
		return ""
	}
	return worktree.AppliedRef(plan.ID)
}

func writeWorkflowSliceCheckpointFromPlan(plan *types.ChangePlan, reasonCode, source string, at time.Time) *types.WriteWorkflowCheckpoint {
	if plan == nil {
		return nil
	}
	sha := strings.TrimSpace(plan.AppliedCommitSHA)
	ref := ""
	if strings.TrimSpace(plan.ID) != "" {
		ref = worktree.AppliedRef(plan.ID)
	}
	wt := strings.TrimSpace(plan.WorktreePath)
	source = strings.TrimSpace(source)
	reasonCode = strings.TrimSpace(reasonCode)
	if ref == "" && sha == "" && wt == "" {
		return nil
	}
	return &types.WriteWorkflowCheckpoint{
		Ref:          ref,
		CommitSHA:    sha,
		WorktreePath: wt,
		Source:       source,
		ReasonCode:   reasonCode,
		At:           at,
	}
}

func (o *Orchestrator) controllerDecisionFromTypedStateAfterDispatchError(run *types.WriteWorkflowRun) (writeflow.WriteWorkflowDecision, bool) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.Mode != types.ModeApply {
		return writeflow.WriteWorkflowDecision{}, false
	}
	plan := o.busCtx.Mutable.ChangePlan()
	batchID := ""
	if run != nil {
		batchID = run.ActiveBatchID
	}
	if activeBatchAppliedPlanPendingVerify(run, plan) {
		appendControllerProgress(run, batchID, "controller_dispatch_recovered_post_apply_verify",
			"controller dispatch failed, but typed workflow state requires post-apply verification of the active plan")
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionVerifyBatch,
			ReasonCode: "post_apply_verify_required",
			Reason:     "typed workflow state requires post-apply verification of the active plan",
		}), true
	}
	if activeBatchProofProbeOnlyPendingVerify(run, plan) {
		appendControllerProgress(run, batchID, "controller_dispatch_recovered_proof_probe_verify",
			"controller dispatch failed, but typed workflow state requires verification of the proof-only probe plan")
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionVerifyBatch,
			ReasonCode: "proof_probe_only_verify_required",
			Reason:     "typed workflow state requires verification of the proof-only probe plan",
		}), true
	}
	if o.writePlanCanProceedWithoutApprovalPause(plan) {
		appendControllerProgress(run, batchID, "controller_dispatch_recovered_ready_plan",
			"controller dispatch failed, but deterministic write approval policy allows the current plan to apply")
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionApplyPlan,
			ReasonCode: "auto_executable_plan",
			Reason:     "deterministic write approval policy allows this pending plan to apply",
		}), true
	}
	return writeflow.WriteWorkflowDecision{}, false
}

func (o *Orchestrator) normalizeControllerTypedStateDecision(decision writeflow.WriteWorkflowDecision, run *types.WriteWorkflowRun) writeflow.WriteWorkflowDecision {
	decision = writeflow.NormalizeWriteWorkflowDecision(decision)
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.Mode != types.ModeApply {
		return decision
	}
	plan := o.busCtx.Mutable.ChangePlan()
	batchID := ""
	if run != nil {
		batchID = run.ActiveBatchID
	}
	if activeBatchAppliedPlanPendingVerify(run, plan) {
		if controllerActionDelaysPostApplyVerify(decision.Action) {
			appendControllerProgress(run, batchID, "post_apply_verify_action_overridden",
				fmt.Sprintf("controller action %s suppressed because the active applied plan has no post-apply verification verdict", decision.Action))
			return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
				Action:     writeflow.ActionVerifyBatch,
				ReasonCode: "post_apply_verify_required",
				Reason:     "active applied plan requires a post-apply verification verdict before more planning",
			})
		}
		return decision
	}
	if activeBatchProofProbeOnlyPendingVerify(run, plan) {
		if controllerActionDelaysPostApplyVerify(decision.Action) {
			appendControllerProgress(run, batchID, "proof_probe_only_verify_action_overridden",
				fmt.Sprintf("controller action %s suppressed because the active proof-only probe plan has no verification verdict", decision.Action))
			return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
				Action:     writeflow.ActionVerifyBatch,
				ReasonCode: "proof_probe_only_verify_required",
				Reason:     "active proof-only probe plan requires a verification verdict before more planning",
			})
		}
		return decision
	}
	if controllerActionCanAutoExploreNavigationGap(decision.Action) {
		if explore, ok := o.controllerNavigationCoverageExploreDecision(run); ok {
			appendControllerProgress(run, firstNonEmptyController(explore.ExplorationRequest.BatchID, batchID), "navigation_coverage_auto_explore_requested",
				"typed repo_map navigation coverage is missing required lens coverage; running bounded read-only exploration before planning")
			return explore
		}
	}
	if batch, ok := activeBatchNeedsReplan(run); ok && controllerActionReusesStalePlanAfterFailedVerify(decision.Action) {
		appendControllerProgress(run, batchID, "needs_replan_stale_action_overridden",
			fmt.Sprintf("controller action %s suppressed because the active batch has a failed post-apply verify attempt; a new bounded plan is required before applying or verifying again", decision.Action))
		goal := strings.TrimSpace(batch.Goal)
		if goal == "" && run != nil {
			goal = strings.TrimSpace(run.Goal)
		}
		if goal == "" {
			goal = "replan active batch"
		}
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionReplanBatch,
			ReasonCode: "needs_replan_after_failed_verify",
			Reason:     "active batch has a failed post-apply verify attempt; replan the failed point before re-applying or re-verifying",
			Batch: &writeflow.WriteBatchPlan{
				ID:     batch.ID,
				Goal:   goal,
				Status: writeflow.BatchReadyForChangePlan,
			},
		})
	}
	if activeBatchProofFollowupPurpose(run) &&
		activeBatchCompletedWithUnverifiedVerdict(run) &&
		!activeBatchHasVerifyFailureHandoff(o.busCtx.Mutable, batchID) &&
		(decision.Action == writeflow.ActionFinish ||
			decision.Action == writeflow.ActionReplanBatch ||
			controllerActionInterruptsUnverifiedCompletion(decision.Action)) {
		appendControllerProgress(run, batchID, "followup_unverified_terminal_overridden",
			"controller follow-up repair suppressed because the active follow-up batch completed with typed unavailable verification and no code-failure handoff")
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:            writeflow.ActionFinish,
			ReasonCode:        "accept_unverified_followup_without_failure_evidence",
			Reason:            "active follow-up batch is complete; local verification is unavailable, but there is no typed code-failure evidence for another follow-up",
			FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
		})
	}
	repairBatch := impactObligationRepairFollowupBatch(run, batchID, plan, o.busCtx.Mutable.ChangeReport())
	if activeBatchCompletedWithNonFailedVerifierVerdict(run) &&
		!activeBatchHasVerifyFailureHandoff(o.busCtx.Mutable, batchID) &&
		repairBatch != nil &&
		(decision.Action == writeflow.ActionFinish ||
			decision.Action == writeflow.ActionReplanBatch ||
			controllerActionInterruptsUnverifiedCompletion(decision.Action)) {
		for _, reasonCode := range repairFollowupRequestedReasonCodes(repairBatch.Purpose) {
			appendControllerProgress(run, batchID, reasonCode, repairFollowupProgressMessage(repairBatch.Purpose))
		}
		if repairBatch.NeedsCodeExploration {
			return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
				Action:     writeflow.ActionExploreCode,
				ReasonCode: repairFollowupActionReasonCode(repairBatch.Purpose, true),
				Reason:     repairFollowupDecisionReason(repairBatch.Purpose, true),
				ExplorationRequest: &types.WriteExplorationRequest{
					BatchID:              repairBatch.ID,
					Goal:                 repairBatch.Goal,
					CandidatePaths:       repairBatch.ExpectedPaths,
					EvidenceRequirements: repairBatch.SuccessCriteria,
				},
			})
		}
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionAppendBatch,
			ReasonCode: repairFollowupActionReasonCode(repairBatch.Purpose, false),
			Reason:     repairFollowupDecisionReason(repairBatch.Purpose, false),
			Batch:      repairBatch,
		})
	}
	if decision.Action == writeflow.ActionReplanBatch &&
		activeBatchCompletedWithUnverifiedVerdict(run) &&
		!activeBatchHasVerifyFailureHandoff(o.busCtx.Mutable, batchID) {
		appendControllerProgress(run, batchID, "replan_without_failure_evidence_overridden",
			"controller replan_batch suppressed because the active batch completed with a typed unverified verdict and no failed-verification handoff")
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:            writeflow.ActionFinish,
			ReasonCode:        "accept_unverified_without_failure_evidence",
			Reason:            "active batch has no typed failure evidence; finish with an unverified caveat instead of replanning from stale goal context",
			FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
		})
	}
	if activeBatchCompletedWithUnverifiedVerdict(run) &&
		!activeBatchHasVerifyFailureHandoff(o.busCtx.Mutable, batchID) &&
		controllerActionInterruptsUnverifiedCompletion(decision.Action) &&
		o.busCtx.Mode != types.ModeVerify {
		appendControllerProgress(run, batchID, "unverified_action_overridden",
			fmt.Sprintf("controller action %s suppressed because the active batch is complete with a typed unverified verdict and no code-failure evidence", decision.Action))
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:            writeflow.ActionFinish,
			ReasonCode:        "accept_unverified_without_failure_evidence",
			Reason:            "active batch is complete; local verification is unavailable, but there is no typed code-failure evidence",
			FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
		})
	}
	if truthDecision, ok := o.controllerTruthLedgerDecision(decision, run); ok {
		appendControllerProgress(run, batchID, truthDecision.ReasonCode, truthDecision.Reason)
		return truthDecision
	}
	if plan == nil && controllerPlanDecisionRequiresInitialExploration(decision, run) {
		explore := controllerExplorationDecisionFromPlanDecision(decision, run)
		appendControllerProgress(run, batchIDOrDefault(explore.ExplorationRequest.BatchID, batchID), "plan_batch_needs_exploration_overridden",
			"controller plan action suppressed because the typed batch requested code exploration and no exploration attempt exists")
		return explore
	}
	if o.writePlanCanProceedWithoutApprovalPause(plan) && controllerActionDelaysReadyPlan(decision.Action) {
		reasonCode := "ready_plan_action_overridden"
		message := fmt.Sprintf("controller action %s suppressed because deterministic write approval policy allows the current plan to apply", decision.Action)
		if decision.Action == writeflow.ActionAskUser {
			reasonCode = "ask_user_auto_executable_overridden"
			message = "controller ask_user suppressed because deterministic write approval policy allows this pending plan to apply"
		}
		appendControllerProgress(run, batchID, reasonCode, message)
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionApplyPlan,
			ReasonCode: "auto_executable_plan",
			Reason:     "deterministic write approval policy allows this pending plan to apply",
		})
	}
	if decision.Action == writeflow.ActionAskUser {
		if repeated := repeatedAskUserFactKeys(run, decision); len(repeated) > 0 {
			appendControllerProgress(run, batchID, "ask_user_repeated_suppressed",
				"controller repeated already surfaced typed missing fact keys: "+strings.Join(repeated, ", "))
			return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
				Action:     writeflow.ActionBlock,
				ReasonCode: "repeated_user_fact_request",
				Reason:     "controller repeated an already surfaced typed missing fact; stopping to avoid repeated user interruptions",
			})
		}
		askBudget := loopkernel.LoopBudget{
			MaxApprovals:  defaultWriteWorkflowMaxAskUserPerBatch,
			ApprovalsUsed: askUserInterruptionsForBatch(run, batchID),
		}
		if advance, ok := o.controllerLoopAdvanceAllowsAction(run, loopkernel.LoopActionAskApproval, askBudget); !ok {
			appendControllerProgress(run, batchID, "ask_user_budget_exhausted",
				"loopkernel rejected ask_user with "+firstNonEmptyController(advance.ReasonCode, "loop_budget_approvals_exhausted"))
			return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
				Action:     writeflow.ActionBlock,
				ReasonCode: "ask_user_budget_exhausted",
				Reason:     "controller exceeded ask_user interruption budget for this batch",
			})
		}
	}
	return decision
}

func (o *Orchestrator) controllerTruthLedgerDecision(decision writeflow.WriteWorkflowDecision, run *types.WriteWorkflowRun) (writeflow.WriteWorkflowDecision, bool) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return writeflow.WriteWorkflowDecision{}, false
	}
	plan := o.busCtx.Mutable.ChangePlan()
	report := o.busCtx.Mutable.ChangeReport()
	view := writeflow.DeriveWorkflowExecutionViewWithReport(o.busCtx.Mode, *run, plan, report)
	ledger := truthledger.Project(truthledger.ProjectInput{
		View:   view,
		Plan:   plan,
		Report: report,
	})
	return controllerTruthLedgerDecisionFromView(decision, view, ledger, run)
}

func controllerTruthLedgerDecisionFromView(decision writeflow.WriteWorkflowDecision, view writeflow.WorkflowExecutionView, ledger types.TruthLedger, run *types.WriteWorkflowRun) (writeflow.WriteWorkflowDecision, bool) {
	decision = writeflow.NormalizeWriteWorkflowDecision(decision)
	ledger = types.NormalizeTruthLedger(ledger)
	switch ledger.State {
	case types.TruthLedgerFailed:
		return controllerTruthLedgerFailedDecision(decision, view, ledger, run)
	case types.TruthLedgerWeak:
		return controllerTruthLedgerWeakDecision(decision, view, ledger, run)
	default:
		return writeflow.WriteWorkflowDecision{}, false
	}
}

func controllerTruthLedgerFailedDecision(decision writeflow.WriteWorkflowDecision, view writeflow.WorkflowExecutionView, ledger types.TruthLedger, run *types.WriteWorkflowRun) (writeflow.WriteWorkflowDecision, bool) {
	switch decision.Action {
	case writeflow.ActionExploreCode, writeflow.ActionReplanBatch, writeflow.ActionBlock:
		return writeflow.WriteWorkflowDecision{}, false
	}
	switch view.State {
	case writeflow.WorkflowExecutionNeedsReplan,
		writeflow.WorkflowExecutionObserveRequired,
		writeflow.WorkflowExecutionComplete:
	default:
		return writeflow.WriteWorkflowDecision{}, false
	}
	batchID := firstNonEmptyController(view.BatchID, controllerDecisionBatchID(decision, run), "batch-1")
	return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionReplanBatch,
		ReasonCode: "truth_ledger_failed_requires_repair",
		Reason:     "typed truth ledger is failed (" + firstNonEmptyController(ledger.ReasonCode, "truth_failed") + "); repair the active batch before continuing",
		Batch: &writeflow.WriteBatchPlan{
			ID:     batchID,
			Goal:   workflowTransitionBatchGoal(view, run, batchID),
			Status: writeflow.BatchReadyForChangePlan,
		},
	}), true
}

func controllerTruthLedgerWeakDecision(decision writeflow.WriteWorkflowDecision, view writeflow.WorkflowExecutionView, ledger types.TruthLedger, run *types.WriteWorkflowRun) (writeflow.WriteWorkflowDecision, bool) {
	if !controllerTruthLedgerWeakActionNeedsOverride(decision.Action) {
		return writeflow.WriteWorkflowDecision{}, false
	}
	switch ledger.RecommendedAction {
	case types.TruthActionLocalize:
		if !controllerTruthLedgerLocalizationActionable(view) {
			return writeflow.WriteWorkflowDecision{}, false
		}
		switch view.State {
		case writeflow.WorkflowExecutionReadyToPlan,
			writeflow.WorkflowExecutionNeedsReplan,
			writeflow.WorkflowExecutionComplete:
		default:
			return writeflow.WriteWorkflowDecision{}, false
		}
		batchID := firstNonEmptyController(view.BatchID, controllerDecisionBatchID(decision, run), "batch-1")
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionExploreCode,
			ReasonCode: "truth_ledger_weak_requires_localization",
			Reason:     "typed truth ledger requires localization before this workflow can finish",
			ExplorationRequest: &types.WriteExplorationRequest{
				BatchID:              batchID,
				Goal:                 workflowTransitionBatchGoal(view, run, batchID),
				CandidatePaths:       workflowTransitionLocalizationCandidatePaths(view),
				EvidenceRequirements: workflowTransitionEvidenceRequirements(view),
			},
		}), true
	case types.TruthActionVerify, types.TruthActionAddProof:
		if view.State != writeflow.WorkflowExecutionComplete {
			return writeflow.WriteWorkflowDecision{}, false
		}
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionVerifyBatch,
			ReasonCode: "truth_ledger_weak_requires_proof",
			Reason:     "typed truth ledger is weak (" + firstNonEmptyController(ledger.ReasonCode, "truth_weak") + "); collect proof before finishing",
		}), true
	case types.TruthActionRepair:
		if view.State != writeflow.WorkflowExecutionNeedsReplan && view.State != writeflow.WorkflowExecutionComplete {
			return writeflow.WriteWorkflowDecision{}, false
		}
		batchID := firstNonEmptyController(view.BatchID, controllerDecisionBatchID(decision, run), "batch-1")
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionReplanBatch,
			ReasonCode: "truth_ledger_weak_requires_repair",
			Reason:     "typed truth ledger requires a bounded repair before finishing",
			Batch: &writeflow.WriteBatchPlan{
				ID:     batchID,
				Goal:   workflowTransitionBatchGoal(view, run, batchID),
				Status: writeflow.BatchReadyForChangePlan,
			},
		}), true
	default:
		return writeflow.WriteWorkflowDecision{}, false
	}
}

func controllerTruthLedgerLocalizationActionable(view writeflow.WorkflowExecutionView) bool {
	if !view.Localization.RequiresMoreContext {
		return false
	}
	if view.LocalizationGateEligible {
		return true
	}
	return writeflow.WorkflowTransitionNeedsNavigationCoverage(view)
}

func controllerTruthLedgerWeakActionNeedsOverride(action writeflow.WorkflowAction) bool {
	switch action {
	case writeflow.ActionFinish, writeflow.ActionAskUser:
		return true
	default:
		return false
	}
}

func (o *Orchestrator) controllerNavigationCoverageExploreDecision(run *types.WriteWorkflowRun) (writeflow.WriteWorkflowDecision, bool) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return writeflow.WriteWorkflowDecision{}, false
	}
	view := writeflow.DeriveWorkflowExecutionViewWithReport(o.busCtx.Mode, *run, o.busCtx.Mutable.ChangePlan(), o.busCtx.Mutable.ChangeReport())
	if !writeflow.WorkflowTransitionNeedsNavigationCoverage(view) {
		return writeflow.WriteWorkflowDecision{}, false
	}
	batchID := firstNonEmptyController(view.BatchID, run.ActiveBatchID, "batch-1")
	return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionExploreCode,
		ReasonCode: "navigation_coverage_auto_explore",
		Reason:     "typed repo_map navigation coverage is missing required lens coverage while localization still needs source-owner context",
		ExplorationRequest: &types.WriteExplorationRequest{
			BatchID:              batchID,
			Goal:                 workflowTransitionBatchGoal(view, run, batchID),
			CandidatePaths:       workflowTransitionLocalizationCandidatePaths(view),
			EvidenceRequirements: workflowTransitionEvidenceRequirements(view),
		},
	}), true
}

func controllerActionCanAutoExploreNavigationGap(action writeflow.WorkflowAction) bool {
	switch action {
	case writeflow.ActionExploreCode, writeflow.ActionBlock:
		return false
	default:
		return true
	}
}

func (o *Orchestrator) enforceControllerWorkflowTransition(decision writeflow.WriteWorkflowDecision, run *types.WriteWorkflowRun) writeflow.WriteWorkflowDecision {
	decision = writeflow.NormalizeWriteWorkflowDecision(decision)
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return decision
	}
	view := writeflow.DeriveWorkflowExecutionViewWithReport(o.busCtx.Mode, *run, o.busCtx.Mutable.ChangePlan(), o.busCtx.Mutable.ChangeReport())
	graphAudit := reasoninggraph.AuditSummaryFromEvents(reasoninggraph.EventsFromWriteWorkflowRun(*run), "write_controller", 32)
	validation := writeflow.ValidateWorkflowTransitionWithGraph(view, decision, graphAudit)
	if validation.Allowed {
		return decision
	}
	batchID := firstNonEmptyController(view.BatchID, run.ActiveBatchID)
	appendControllerProgress(run, batchID, "workflow_transition_rejected",
		fmt.Sprintf("%s rejected in %s: %s", decision.Action, view.State, validation.ReasonCode))
	next := controllerDecisionFromWorkflowTransitionValidation(view, validation, decision, run)
	next = writeflow.NormalizeWriteWorkflowDecision(next)
	if errs := writeflow.ValidateWriteWorkflowDecision(next); len(errs) > 0 {
		appendControllerProgress(run, batchID, "workflow_transition_recovery_invalid",
			strings.Join(errs, "; "))
		return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionBlock,
			ReasonCode: firstNonEmptyController(validation.ReasonCode, "workflow_transition_rejected"),
			Reason:     firstNonEmptyController(validation.Reason, "controller decision rejected by typed workflow state"),
		})
	}
	if next.Action != decision.Action || strings.TrimSpace(next.ReasonCode) != strings.TrimSpace(decision.ReasonCode) {
		appendControllerProgress(run, batchID, "workflow_transition_overridden",
			fmt.Sprintf("%s -> %s via %s", decision.Action, next.Action, validation.ReasonCode))
	}
	return next
}

func controllerDecisionFromWorkflowTransitionValidation(view writeflow.WorkflowExecutionView, validation writeflow.WorkflowTransitionValidation, prior writeflow.WriteWorkflowDecision, run *types.WriteWorkflowRun) writeflow.WriteWorkflowDecision {
	reasonCode := firstNonEmptyController(validation.ReasonCode, "workflow_transition_rejected")
	reason := firstNonEmptyController(validation.Reason, "controller decision rejected by typed workflow state")
	switch validation.RecommendedAction {
	case writeflow.ActionExploreCode:
		batchID := firstNonEmptyController(view.BatchID, controllerDecisionBatchID(prior, run), "batch-1")
		return writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionExploreCode,
			ReasonCode: reasonCode,
			Reason:     reason,
			ExplorationRequest: &types.WriteExplorationRequest{
				BatchID:              batchID,
				Goal:                 workflowTransitionBatchGoal(view, run, batchID),
				CandidatePaths:       workflowTransitionLocalizationCandidatePaths(view),
				EvidenceRequirements: workflowTransitionEvidenceRequirements(view),
			},
		}
	case writeflow.ActionApplyPlan:
		return writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionApplyPlan,
			ReasonCode: reasonCode,
			Reason:     reason,
		}
	case writeflow.ActionVerifyBatch:
		return writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionVerifyBatch,
			ReasonCode: reasonCode,
			Reason:     reason,
		}
	case writeflow.ActionReplanBatch:
		batchID := firstNonEmptyController(view.BatchID, controllerDecisionBatchID(prior, run), "batch-1")
		return writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionReplanBatch,
			ReasonCode: reasonCode,
			Reason:     reason,
			Batch: &writeflow.WriteBatchPlan{
				ID:     batchID,
				Goal:   workflowTransitionBatchGoal(view, run, batchID),
				Status: writeflow.BatchReadyForChangePlan,
			},
		}
	case writeflow.ActionAskUser:
		return workflowTransitionAskUserDecision(view, reasonCode, reason)
	case writeflow.ActionFinish:
		return writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionFinish,
			ReasonCode: reasonCode,
			Reason:     reason,
		}
	case writeflow.ActionBlock:
		return writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionBlock,
			ReasonCode: reasonCode,
			Reason:     reason,
		}
	default:
		return writeflow.WriteWorkflowDecision{
			Action:     writeflow.ActionBlock,
			ReasonCode: reasonCode,
			Reason:     reason,
		}
	}
}

func workflowTransitionLocalizationCandidatePaths(view writeflow.WorkflowExecutionView) []string {
	var paths []string
	paths = append(paths, view.Localization.OwnerMissingPaths...)
	paths = append(paths, view.Localization.SourcePaths...)
	paths = append(paths, view.Localization.AuxiliaryPaths...)
	return dedupTrimControllerStrings(paths)
}

func workflowTransitionLocalizationEvidenceRequirements(view writeflow.WorkflowExecutionView) []string {
	var rows []string
	if view.Localization.ReasonCode != "" || view.Localization.State != "" {
		rows = append(rows, fmt.Sprintf("localization_authority state=%s reason=%s required=typed_owner_localization_anchor",
			view.Localization.State, view.Localization.ReasonCode))
	}
	for _, path := range workflowTransitionLocalizationCandidatePaths(view) {
		rows = append(rows, "localization_requirement path="+path+" kind=owner_evidence required=typed_owner_localization_anchor source=workflow_transition")
		if len(rows) >= 8 {
			break
		}
	}
	return dedupTrimControllerStrings(rows)
}

func workflowTransitionNavigationEvidenceRequirements(view writeflow.WorkflowExecutionView) []string {
	if len(view.Navigation.MissingRoutes) == 0 {
		return nil
	}
	var rows []string
	for _, route := range view.Navigation.MissingRoutes {
		if !route.Valid() {
			continue
		}
		rows = append(rows, fmt.Sprintf("repo_map_navigation_requirement route=%s state=%s reason=%s required=typed_navigation_coverage source=workflow_transition",
			route, view.Navigation.State, view.Navigation.ReasonCode))
		if len(rows) >= 8 {
			break
		}
	}
	return dedupTrimControllerStrings(rows)
}

func workflowTransitionEvidenceRequirements(view writeflow.WorkflowExecutionView) []string {
	rows := workflowTransitionLocalizationEvidenceRequirements(view)
	rows = append(rows, workflowTransitionNavigationEvidenceRequirements(view)...)
	return dedupTrimControllerStrings(rows)
}

func workflowTransitionAskUserDecision(view writeflow.WorkflowExecutionView, reasonCode, reason string) writeflow.WriteWorkflowDecision {
	evidence := firstNonEmptyController(view.PlanID, view.BatchID, view.RunID)
	description := firstNonEmptyController(reason, "operator approval is required before continuing the active write workflow")
	return writeflow.WriteWorkflowDecision{
		Action:           writeflow.ActionAskUser,
		ReasonCode:       firstNonEmptyController(reasonCode, "workflow_transition_requires_user"),
		Reason:           description,
		QuestionsForUser: []string{"Approve the active write workflow step before Codrax continues?"},
		MissingFacts: []writeflow.MissingUserFact{{
			Kind:        "approval_boundary",
			Description: description,
			EvidenceRef: evidence,
			Consumer:    "controller",
		}},
	}
}

func workflowTransitionBatchGoal(view writeflow.WorkflowExecutionView, run *types.WriteWorkflowRun, batchID string) string {
	if run != nil {
		for _, batch := range run.Batches {
			if strings.TrimSpace(batch.ID) == strings.TrimSpace(batchID) {
				if goal := strings.TrimSpace(batch.Goal); goal != "" {
					return goal
				}
			}
		}
		if goal := strings.TrimSpace(run.Goal); goal != "" {
			return goal
		}
	}
	if view.State == writeflow.WorkflowExecutionNeedsReplan {
		return "repair failed verification"
	}
	return "continue active write workflow"
}

func controllerPlanDecisionRequiresInitialExploration(decision writeflow.WriteWorkflowDecision, run *types.WriteWorkflowRun) bool {
	switch decision.Action {
	case writeflow.ActionPlanBatch, writeflow.ActionAppendBatch, writeflow.ActionSplitBatch:
	default:
		return false
	}
	batchID := controllerDecisionBatchID(decision, run)
	if batchID != "" && workflowBatchHasExploreAttempt(run, batchID) {
		return false
	}
	return controllerDecisionBatchNeedsExploration(decision, batchID)
}

func controllerDecisionBatchNeedsExploration(decision writeflow.WriteWorkflowDecision, batchID string) bool {
	if batchNeedsExploration(decision.Batch, batchID) {
		return true
	}
	if batchNeedsExploration(decision.Workflow.NextBatch, batchID) {
		return true
	}
	for i := range decision.Workflow.PlannedBatches {
		if batchNeedsExploration(&decision.Workflow.PlannedBatches[i], batchID) {
			return true
		}
	}
	return false
}

func batchNeedsExploration(batch *writeflow.WriteBatchPlan, batchID string) bool {
	if batch == nil || !batch.NeedsCodeExploration {
		return false
	}
	id := strings.TrimSpace(batch.ID)
	return batchID == "" || id == "" || id == batchID
}

func controllerDecisionBatchID(decision writeflow.WriteWorkflowDecision, run *types.WriteWorkflowRun) string {
	if decision.Batch != nil && strings.TrimSpace(decision.Batch.ID) != "" {
		return strings.TrimSpace(decision.Batch.ID)
	}
	if decision.Workflow.NextBatch != nil && strings.TrimSpace(decision.Workflow.NextBatch.ID) != "" {
		return strings.TrimSpace(decision.Workflow.NextBatch.ID)
	}
	for _, batch := range decision.Workflow.PlannedBatches {
		if strings.TrimSpace(batch.ID) != "" {
			return strings.TrimSpace(batch.ID)
		}
	}
	if run != nil && strings.TrimSpace(run.ActiveBatchID) != "" {
		return strings.TrimSpace(run.ActiveBatchID)
	}
	return ""
}

func controllerExplorationDecisionFromPlanDecision(decision writeflow.WriteWorkflowDecision, run *types.WriteWorkflowRun) writeflow.WriteWorkflowDecision {
	batchID := controllerDecisionBatchID(decision, run)
	goal := controllerDecisionBatchGoal(decision)
	paths := controllerDecisionBatchExplorePaths(decision, batchID)
	return writeflow.NormalizeWriteWorkflowDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionExploreCode,
		ReasonCode: "batch_requires_code_exploration",
		Reason:     "typed batch requested code exploration before bounded planning",
		ExplorationRequest: &types.WriteExplorationRequest{
			BatchID:        batchIDOrDefault(batchID, "batch-1"),
			Goal:           goal,
			CandidatePaths: append([]string(nil), paths...),
		},
	})
}

func controllerDecisionBatchGoal(decision writeflow.WriteWorkflowDecision) string {
	if decision.Batch != nil && strings.TrimSpace(decision.Batch.Goal) != "" {
		return strings.TrimSpace(decision.Batch.Goal)
	}
	if decision.Workflow.NextBatch != nil && strings.TrimSpace(decision.Workflow.NextBatch.Goal) != "" {
		return strings.TrimSpace(decision.Workflow.NextBatch.Goal)
	}
	for _, batch := range decision.Workflow.PlannedBatches {
		if strings.TrimSpace(batch.Goal) != "" {
			return strings.TrimSpace(batch.Goal)
		}
	}
	if strings.TrimSpace(decision.Workflow.Goal) != "" {
		return strings.TrimSpace(decision.Workflow.Goal)
	}
	return "Explore code needed for the active write batch"
}

func controllerDecisionBatchExplorePaths(decision writeflow.WriteWorkflowDecision, batchID string) []string {
	var paths []string
	addBatchPaths := func(batch *writeflow.WriteBatchPlan) {
		if batch == nil {
			return
		}
		id := strings.TrimSpace(batch.ID)
		if batchID != "" && id != "" && id != batchID {
			return
		}
		paths = append(paths, batch.ExploreTargets...)
		paths = append(paths, batch.ExpectedPaths...)
	}
	addBatchPaths(decision.Batch)
	addBatchPaths(decision.Workflow.NextBatch)
	for i := range decision.Workflow.PlannedBatches {
		addBatchPaths(&decision.Workflow.PlannedBatches[i])
	}
	return dedupTrimControllerStrings(paths)
}

func workflowBatchHasExploreAttempt(run *types.WriteWorkflowRun, batchID string) bool {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != strings.TrimSpace(batchID) {
			continue
		}
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) == "explore" {
				return true
			}
		}
		return false
	}
	return false
}

func dedupTrimControllerStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func activeBatchHasCompletedExplorationHandoff(mu *types.MutableState) bool {
	return activeBatchHasExplorationHandoffWithStatuses(mu, map[string]bool{"complete": true})
}

func activeBatchHasUsableExplorationHandoff(mu *types.MutableState) bool {
	return activeBatchHasExplorationHandoffWithStatuses(mu, map[string]bool{
		"complete": true,
		"degraded": true,
	})
}

func activeBatchHasUsableExplorationHandoffForRun(mu *types.MutableState, run types.WriteWorkflowRun) bool {
	if mu == nil {
		return false
	}
	handoff := mu.WriteExplorationHandoff()
	if !writeExplorationHandoffHasPlanningMaterial(handoff) {
		return false
	}
	if strings.TrimSpace(run.ActiveBatchID) == "" {
		return false
	}
	batchID := strings.TrimSpace(run.ActiveBatchID)
	if strings.TrimSpace(handoff.BatchID) != "" && strings.TrimSpace(handoff.BatchID) != batchID {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != batchID {
			continue
		}
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) == "explore" &&
				(strings.TrimSpace(attempt.Status) == "complete" || strings.TrimSpace(attempt.Status) == "degraded") {
				return true
			}
		}
		return false
	}
	return false
}

func activeBatchHasExplorationHandoffWithStatuses(mu *types.MutableState, statuses map[string]bool) bool {
	if mu == nil {
		return false
	}
	handoff := mu.WriteExplorationHandoff()
	if !writeExplorationHandoffHasPlanningMaterial(handoff) {
		return false
	}
	run := mu.WriteWorkflowRun()
	if run == nil || strings.TrimSpace(run.ActiveBatchID) == "" {
		return false
	}
	batchID := strings.TrimSpace(run.ActiveBatchID)
	if strings.TrimSpace(handoff.BatchID) != "" && strings.TrimSpace(handoff.BatchID) != batchID {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != batchID {
			continue
		}
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) == "explore" &&
				statuses[strings.TrimSpace(attempt.Status)] {
				return true
			}
		}
		return false
	}
	return false
}

func writeExplorationHandoffHasPlanningMaterial(handoff *types.WriteExplorationHandoff) bool {
	if handoff == nil {
		return false
	}
	normalized := types.NormalizeWriteExplorationHandoff(*handoff)
	return len(normalized.TargetFiles) > 0 ||
		len(normalized.RelevantSymbols) > 0 ||
		len(normalized.ExistingPatterns) > 0 ||
		len(normalized.Invariants) > 0 ||
		len(normalized.TestSurface) > 0 ||
		len(normalized.RiskNotes) > 0 ||
		len(normalized.Unknowns) > 0 ||
		len(normalized.EvidenceRefs) > 0 ||
		(normalized.ConventionGraph != nil && len(normalized.ConventionGraph.Nodes) > 0)
}

func plannerSoftCapForCompletedExploration(base, current, effectiveCurrent int) int {
	cap := effectiveCurrent
	if cap <= 0 {
		cap = current
	}
	if cap <= 0 {
		cap = base
	}
	// A completed/degraded typed exploration handoff means the planner should
	// synthesize a bounded ChangePlan, not restart broad investigation. It still
	// commonly needs several read-only diagnostic calls to fetch exact current
	// bytes for line edits and old_text. Reserve that synthesis room without
	// affecting simple write tasks that skip the exploration handoff path.
	floor := base + 6
	if cap < floor {
		return floor
	}
	return cap
}

func askUserInterruptionsForBatch(run *types.WriteWorkflowRun, batchID string) int {
	if run == nil {
		return 0
	}
	batchID = strings.TrimSpace(batchID)
	count := 0
	for _, item := range run.ProgressLedger {
		if item.Status != string(writeflow.ActionAskUser) {
			continue
		}
		if batchID != "" && strings.TrimSpace(item.BatchID) != batchID {
			continue
		}
		count++
	}
	return count
}

func controllerExplorationRoundBudget(run types.WriteWorkflowRun) int {
	run = types.NormalizeWriteWorkflowRun(run)
	if run.Budget.MaxExplorationRounds > 0 {
		return run.Budget.MaxExplorationRounds
	}
	return defaultWriteWorkflowMaxExplorationRounds
}

func (o *Orchestrator) controllerLoopAdvanceAllowsAction(run *types.WriteWorkflowRun, action loopkernel.LoopRecommendedAction, budget loopkernel.LoopBudget) (loopkernel.LoopAdvanceDecision, bool) {
	if run == nil {
		return loopkernel.LoopAdvanceDecision{Allowed: true}, true
	}
	ctx := context.Background()
	if o != nil {
		ctx = o.CancelContext()
	}
	loopRun := loopkernel.LoopRun{
		RunID:        strings.TrimSpace(run.RunID),
		Mode:         "write",
		ActiveUnitID: strings.TrimSpace(run.ActiveBatchID),
		Budget:       loopkernel.NormalizeLoopBudget(budget),
	}
	_, decision := loopRun.Advance(ctx, loopkernel.LoopAdvanceInput{
		Events:          loopkernel.EventsFromWriteWorkflowRun(*run),
		RequestedAction: action,
		Now:             time.Now(),
	})
	if decision.ReasonCode == "" && !decision.Allowed {
		decision.ReasonCode = "loop_budget_rejected"
	}
	return decision, decision.Allowed
}

func repeatedAskUserFactKeys(run *types.WriteWorkflowRun, decision writeflow.WriteWorkflowDecision) []string {
	if run == nil {
		return nil
	}
	keys := writeflow.MissingUserFactKeys(decision.MissingFacts)
	if len(keys) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, item := range run.ProgressLedger {
		if item.Status != string(writeflow.ActionAskUser) {
			continue
		}
		for _, key := range item.FactKeys {
			key = strings.TrimSpace(key)
			if key != "" {
				seen[key] = true
			}
		}
	}
	repeated := make([]string, 0, len(keys))
	for _, key := range keys {
		if seen[key] {
			repeated = append(repeated, key)
		}
	}
	return repeated
}

func (o *Orchestrator) stampWorkflowPlanForActiveBatch(plan *types.ChangePlan, run *types.WriteWorkflowRun) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return
	}
	var snap *types.WriteWorkflowRun
	if run != nil {
		cp := types.CloneWriteWorkflowRun(*run)
		snap = &cp
	} else {
		snap = o.busCtx.Mutable.WriteWorkflowRun()
	}
	if snap == nil {
		return
	}
	runID := strings.TrimSpace(snap.RunID)
	batchID := strings.TrimSpace(snap.ActiveBatchID)
	if runID == "" || batchID == "" {
		return
	}
	idx := 0
	found := false
	for i, batch := range snap.Batches {
		if strings.TrimSpace(batch.ID) == batchID {
			idx = i
			found = true
			break
		}
	}
	if !found {
		return
	}
	plan.PhaseGroupID = runID
	plan.PhaseIndex = idx
	o.busCtx.Mutable.SetChangePlan(plan)
}

func controllerActionDelaysReadyPlan(action writeflow.WorkflowAction) bool {
	switch action {
	case writeflow.ActionApplyPlan, writeflow.ActionBlock:
		return false
	case "":
		return false
	default:
		return true
	}
}

func controllerActionDelaysPostApplyVerify(action writeflow.WorkflowAction) bool {
	switch action {
	case writeflow.ActionVerifyBatch, writeflow.ActionBlock:
		return false
	case "":
		return false
	default:
		return true
	}
}

func controllerActionInterruptsUnverifiedCompletion(action writeflow.WorkflowAction) bool {
	switch action {
	case writeflow.ActionExploreCode, writeflow.ActionPlanBatch, writeflow.ActionApplyPlan,
		writeflow.ActionVerifyBatch, writeflow.ActionReplanBatch, writeflow.ActionAskUser,
		writeflow.ActionBlock:
		return true
	default:
		return false
	}
}

func activeBatchNeedsReplan(run *types.WriteWorkflowRun) (types.WriteWorkflowBatch, bool) {
	if run == nil {
		return types.WriteWorkflowBatch{}, false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return types.WriteWorkflowBatch{}, false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeID {
			continue
		}
		if workflowBatchActiveSliceFailed(batch) {
			return batch, true
		}
		state := writeflow.DeriveBatchAttemptState(batch)
		return batch, state.Phase == writeflow.BatchPhaseNeedsReplan
	}
	return types.WriteWorkflowBatch{}, false
}

func workflowBatchActiveSliceFailed(batch types.WriteWorkflowBatch) bool {
	activeID := strings.TrimSpace(batch.ActiveSliceID)
	if activeID == "" {
		return false
	}
	for _, slice := range batch.Slices {
		if strings.TrimSpace(slice.ID) != activeID {
			continue
		}
		return slice.Status == types.ChangePlanSliceFailed || slice.Status == types.ChangePlanSliceBlocked
	}
	return false
}

func workflowBatchHasFailedExecutionStateForPlan(batch types.WriteWorkflowBatch, planID string) bool {
	if workflowBatchActiveSliceFailed(batch) {
		return true
	}
	planID = strings.TrimSpace(planID)
	for i := len(batch.Attempts) - 1; i >= 0; i-- {
		attempt := batch.Attempts[i]
		if strings.TrimSpace(attempt.Kind) != "verify" {
			continue
		}
		if planID != "" && strings.TrimSpace(attempt.PlanID) != planID {
			continue
		}
		return strings.TrimSpace(attempt.Status) == "failed"
	}
	return false
}

func controllerActionReusesStalePlanAfterFailedVerify(action writeflow.WorkflowAction) bool {
	switch action {
	case writeflow.ActionApplyPlan, writeflow.ActionVerifyBatch, writeflow.ActionPlanBatch:
		return true
	default:
		return false
	}
}

func activeBatchAppliedPlanPendingVerify(run *types.WriteWorkflowRun, plan *types.ChangePlan) bool {
	if run == nil || plan == nil {
		return false
	}
	planID := strings.TrimSpace(plan.ID)
	if planID == "" {
		return false
	}
	if unit, ok := writeflow.ActiveRuntimeUnitView(types.ModeApply, *run, plan, nil); ok {
		return unit.Status == types.ChangePlanSliceObserving
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != strings.TrimSpace(run.ActiveBatchID) {
			continue
		}
		lastApply := -1
		for i, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) != "apply" || strings.TrimSpace(attempt.Status) != "applied" {
				continue
			}
			if strings.TrimSpace(attempt.PlanID) != "" && strings.TrimSpace(attempt.PlanID) != planID {
				continue
			}
			lastApply = i
		}
		if lastApply < 0 {
			return false
		}
		for i := lastApply + 1; i < len(batch.Attempts); i++ {
			attempt := batch.Attempts[i]
			if strings.TrimSpace(attempt.Kind) != "verify" {
				continue
			}
			if strings.TrimSpace(attempt.PlanID) == "" || strings.TrimSpace(attempt.PlanID) == planID {
				return false
			}
		}
		return true
	}
	return false
}

func activeBatchProofProbeOnlyPendingVerify(run *types.WriteWorkflowRun, plan *types.ChangePlan) bool {
	if run == nil || !changePlanIsProofProbeOnly(plan) || !activeBatchProofFollowupPurpose(run) {
		return false
	}
	planID := strings.TrimSpace(plan.ID)
	if planID == "" {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeID {
			continue
		}
		if batch.Status != types.WriteWorkflowBatchVerifying && batch.Status != types.WriteWorkflowBatchPlanned {
			return false
		}
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) != "verify" {
				continue
			}
			if strings.TrimSpace(attempt.PlanID) == "" || strings.TrimSpace(attempt.PlanID) == planID {
				return false
			}
		}
		return true
	}
	return false
}

func activeBatchCompletedWithUnverifiedVerdict(run *types.WriteWorkflowRun) bool {
	if run == nil {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeID {
			continue
		}
		if batch.Status != types.WriteWorkflowBatchComplete {
			return false
		}
		for i := len(batch.Attempts) - 1; i >= 0; i-- {
			attempt := batch.Attempts[i]
			if strings.TrimSpace(attempt.Kind) != "verify" {
				continue
			}
			if strings.TrimSpace(attempt.Status) != "unverified" {
				return false
			}
			switch strings.TrimSpace(attempt.ReasonCode) {
			case "no_tests", string(types.FailureKindRunnerMissing), string(types.FailureKindParserError), string(types.FailureKindPreexistingBuildFailure):
				return true
			default:
				return false
			}
		}
		return false
	}
	return false
}

func activeBatchCompletedWithNonFailedVerifierVerdict(run *types.WriteWorkflowRun) bool {
	if run == nil {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return false
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeID {
			continue
		}
		if batch.Status != types.WriteWorkflowBatchComplete {
			return false
		}
		for i := len(batch.Attempts) - 1; i >= 0; i-- {
			attempt := batch.Attempts[i]
			if strings.TrimSpace(attempt.Kind) != "verify" {
				continue
			}
			switch strings.TrimSpace(attempt.Status) {
			case "passed", "unverified":
				return true
			default:
				return false
			}
		}
		return false
	}
	return false
}

type impactRepairQueueItem struct {
	ID             string
	Code           string
	Kind           string
	Path           string
	RelatedPath    string
	Paths          []string
	Symbol         string
	ContractRef    string
	CoverageStatus string
	EvidenceRef    string
	Source         string
	Priority       int
}

func impactRepairFollowupAlreadyRequested(run *types.WriteWorkflowRun, batchID string) bool {
	if workflowProgressReasonCount(run, "", "impact_obligation_followup_requested") > 0 {
		return true
	}
	// Durable runs created before RC-14 used this narrower reason. Treat it as
	// the same one-shot repair request so resumed workflows do not recurse.
	return workflowProgressReasonCount(run, "", "semantic_patch_review_followup_requested") > 0
}

func verificationProofFollowupAlreadyRequested(run *types.WriteWorkflowRun) bool {
	return workflowProgressReasonCount(run, "", "verification_proof_followup_requested") > 0
}

func (o *Orchestrator) enrichProofFollowupPlanProbeRefs(batch *writeflow.WriteBatchPlan, plan *types.ChangePlan) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return false
	}
	if len(plan.VerificationProbes) != 1 {
		return false
	}
	run := o.busCtx.Mutable.WriteWorkflowRun()
	if !proofFollowupRunAuthorized(run) {
		return false
	}
	purpose, criteria := proofFollowupBatchMetadata(run, batch)
	if !proofFollowupPurpose(purpose) {
		return false
	}
	changed := false
	contractRefs := proofFollowupContractRefsFromCriteria(criteria, plan)
	if len(contractRefs) > 0 {
		before := strings.Join(dedupTrimControllerStrings(plan.VerificationProbes[0].ContractRefs), "\x00")
		plan.VerificationProbes[0].ContractRefs = dedupTrimControllerStrings(append(plan.VerificationProbes[0].ContractRefs, contractRefs...))
		after := strings.Join(plan.VerificationProbes[0].ContractRefs, "\x00")
		if after != before {
			changed = true
		}
	}
	if len(dedupTrimControllerStrings(plan.VerificationProbes[0].ChangedSymbolRefs)) == 0 {
		if refs := proofFollowupChangedSymbolRefsFromCriteria(criteria); len(refs) > 0 {
			plan.VerificationProbes[0].ChangedSymbolRefs = refs
			changed = true
		} else if paths := planSourceRepairPaths(plan, 2); len(paths) == 1 {
			plan.VerificationProbes[0].ChangedSymbolRefs = []string{"path:" + paths[0]}
			changed = true
		}
	}
	if changed {
		o.refreshAutoApprovalAfterDeterministicPlanMutation(plan, "proof_followup_probe_ref_enrichment")
		logging.Info("[orchestrator] enriched proof-follow-up verification probe refs plan=%s contracts=%s",
			strings.TrimSpace(plan.ID), strings.Join(contractRefs, ","))
	}
	return changed
}

func (o *Orchestrator) refreshAutoApprovalAfterDeterministicPlanMutation(plan *types.ChangePlan, source string) bool {
	if o == nil || plan == nil || plan.Approval == nil {
		return false
	}
	if strings.TrimSpace(plan.Approval.Action) != string(writeflow.ApprovalActionAutoExecute) ||
		strings.TrimSpace(plan.Approval.UserDecision) != "auto" {
		return false
	}
	if !plan.ApprovalRecordIntegrityOK() {
		return false
	}
	if writeflow.DeriveApprovalExecutionView(plan).State == writeflow.ApprovalExecutionAutoAllowed {
		return false
	}
	policy := o.writeApprovalPolicy
	if policy == "" {
		policy = writeflow.ApprovalPolicyAutoSafe
	}
	assessment := writeflow.AssessWriteRisk(o.writeRiskAssessmentInput(plan))
	decision := writeflow.DecideWriteApproval(policy, assessment)
	if decision.Action != writeflow.ApprovalActionAutoExecute {
		return false
	}
	stampWriteApprovalRecord(o, plan, assessment, decision, strings.TrimSpace(source), "auto", types.PlanFingerprint(plan))
	persistWriteApprovalRecord(o, plan)
	mergeWriteRiskContextPack(o, plan, assessment, decision)
	mergeWritePlanContextPack(o, plan)
	return true
}

func proofFollowupRunAuthorized(run *types.WriteWorkflowRun) bool {
	return run != nil && workflowProgressReasonCount(run, "", "verification_proof_followup_requested") > 0
}

func proofFollowupBatchMetadata(run *types.WriteWorkflowRun, batch *writeflow.WriteBatchPlan) (string, []string) {
	if run != nil {
		activeID := strings.TrimSpace(run.ActiveBatchID)
		if activeID != "" {
			for _, candidate := range run.Batches {
				if strings.TrimSpace(candidate.ID) != activeID {
					continue
				}
				if candidate.Purpose != "" || len(candidate.SuccessCriteria) > 0 {
					return strings.TrimSpace(candidate.Purpose), append([]string(nil), candidate.SuccessCriteria...)
				}
				break
			}
		}
	}
	if batch == nil {
		return "", nil
	}
	normalized := writeflow.NormalizeBatchPlan(*batch)
	return strings.TrimSpace(normalized.Purpose), append([]string(nil), normalized.SuccessCriteria...)
}

func proofFollowupPurpose(purpose string) bool {
	switch strings.TrimSpace(purpose) {
	case "verification_proof_followup", "impact_and_verification_proof_followup":
		return true
	default:
		return false
	}
}

func proofFollowupProbeRequiredHint(batch *writeflow.WriteBatchPlan, plan *types.ChangePlan) string {
	if batch == nil || plan == nil || !proofFollowupPurpose(batch.Purpose) || len(plan.VerificationProbes) > 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("This batch is a typed verification proof follow-up. The next ChangePlan must include at least one verification_probes[] entry that imports or executes the changed production code and asserts the uncovered typed symbol/contract behavior.")
	if len(batch.SuccessCriteria) > 0 {
		b.WriteString(" Preserve the typed proof criteria when choosing contract_refs or changed_symbol_refs: ")
		b.WriteString(strings.Join(batch.SuccessCriteria, " | "))
		b.WriteString(".")
	}
	b.WriteString(" Code changes may be included only if the probe or exact current bytes show another code repair is needed; do not emit a prose-only or code-only proof plan.")
	return b.String()
}

func proofFollowupProductionEditWithoutFailureHint(mu *types.MutableState, batch *writeflow.WriteBatchPlan, plan *types.ChangePlan) (string, []string) {
	if batch == nil || plan == nil || strings.TrimSpace(batch.Purpose) != "verification_proof_followup" || len(plan.Changes) == 0 {
		return "", nil
	}
	if activeBatchHasVerifyFailureHandoff(mu, batch.ID) {
		return "", nil
	}
	paths := proofFollowupProductionChangePaths(plan)
	if len(paths) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("## Typed proof-follow-up source-edit boundary\n\n")
	b.WriteString("This batch is a pure verification proof follow-up and there is no active typed verification-failure handoff for this batch. It must not modify production source just to close proof metadata. Emit changes: [] with verification_probes[] for the already-applied worktree, or ask the controller to split a separate impact repair batch when a typed probe failure proves source behavior still needs a repair.\n\n")
	b.WriteString("Production source paths in the rejected proof plan:\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	return strings.TrimSpace(b.String()), paths
}

func proofFollowupProductionChangePaths(plan *types.ChangePlan) []string {
	if plan == nil {
		return nil
	}
	var paths []string
	for _, change := range plan.Changes {
		path := normalizeControllerPath(change.Path)
		if path == "" || types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(path)) {
			continue
		}
		paths = append(paths, path)
	}
	return dedupTrimControllerStrings(paths)
}

func proofFollowupNonExecutableChangeHint(batch *writeflow.WriteBatchPlan, plan *types.ChangePlan) (string, []string) {
	if batch == nil || plan == nil || !proofFollowupPurpose(batch.Purpose) || len(plan.Changes) == 0 {
		return "", nil
	}
	paths := proofFollowupNonExecutableProductionChangePaths(plan)
	if len(paths) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("## Typed proof-follow-up patch critique\n\n")
	b.WriteString("The previous proof-follow-up ChangePlan only added comments or blank lines to production source paths. A controller-owned proof follow-up must either emit a real executable source/test repair or emit changes: [] with verification_probes[] to verify the already-applied worktree.\n\n")
	b.WriteString("Non-executable production source changes:\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	b.WriteString("\nDo not add explanatory comments, doc placeholders, or verification notes to production files just to satisfy changes[]. If the current source bytes are already correct, emit a probe-only proof plan with changes: [] and the typed verification_probes[] required by this batch.")
	return strings.TrimSpace(b.String()), paths
}

func proofFollowupNonExecutableProductionChangePaths(plan *types.ChangePlan) []string {
	if plan == nil {
		return nil
	}
	var paths []string
	for _, change := range plan.Changes {
		path := normalizeControllerPath(change.Path)
		if path == "" || types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(path)) {
			continue
		}
		if changeOnlyAddsCommentOrBlankLines(change) {
			paths = append(paths, path)
		}
	}
	return dedupTrimControllerStrings(paths)
}

func changeOnlyAddsCommentOrBlankLines(change types.FileChange) bool {
	switch strings.TrimSpace(change.Kind) {
	case "patch":
		return patchOnlyAddsCommentOrBlankLines(change.Patch)
	default:
		return false
	}
}

func patchOnlyAddsCommentOrBlankLines(patch string) bool {
	added := 0
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "diff --git ") ||
			strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "new file mode ") ||
			strings.HasPrefix(line, "deleted file mode ") {
			continue
		}
		if strings.HasPrefix(line, "-") {
			if strings.TrimSpace(strings.TrimPrefix(line, "-")) != "" {
				return false
			}
			continue
		}
		if !strings.HasPrefix(line, "+") {
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(line, "+"))
		if text == "" {
			continue
		}
		added++
		if !lineLooksLikeCommentOnly(text) {
			return false
		}
	}
	return added > 0
}

func lineLooksLikeCommentOnly(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	switch {
	case strings.HasPrefix(text, "#"),
		strings.HasPrefix(text, "//"),
		strings.HasPrefix(text, "/*"),
		strings.HasPrefix(text, "*"),
		strings.HasPrefix(text, "*/"),
		strings.HasPrefix(text, "<!--"),
		strings.HasPrefix(text, "-->"),
		strings.HasPrefix(text, `"""`),
		strings.HasPrefix(text, `'''`):
		return true
	default:
		return false
	}
}

func proofFollowupContractRefsFromCriteria(criteria []string, plan *types.ChangePlan) []string {
	if len(criteria) == 0 || plan == nil || len(plan.BehaviorContracts) == 0 {
		return nil
	}
	valid := map[string]bool{}
	for _, contract := range plan.BehaviorContracts {
		id := strings.TrimSpace(contract.ID)
		if id != "" {
			valid[id] = true
		}
	}
	var refs []string
	for _, row := range criteria {
		for _, field := range strings.Fields(row) {
			key, value, ok := strings.Cut(field, "=")
			if !ok || strings.TrimSpace(key) != "contract_ref" {
				continue
			}
			for _, raw := range strings.Split(value, ",") {
				ref := strings.TrimSpace(raw)
				if ref != "" && valid[ref] {
					refs = append(refs, ref)
				}
			}
		}
	}
	return dedupTrimControllerStrings(refs)
}

func proofFollowupChangedSymbolRefsFromCriteria(criteria []string) []string {
	var refs []string
	for _, row := range criteria {
		for _, field := range strings.Fields(row) {
			key, value, ok := strings.Cut(field, "=")
			if !ok || strings.TrimSpace(key) != "symbol" {
				continue
			}
			for _, raw := range strings.Split(value, ",") {
				if ref := strings.TrimSpace(raw); ref != "" {
					refs = append(refs, ref)
				}
			}
		}
	}
	return dedupTrimControllerStrings(refs)
}

func impactObligationRepairFollowupBatch(run *types.WriteWorkflowRun, activeBatchID string, plan *types.ChangePlan, report *types.ChangeReport) *writeflow.WriteBatchPlan {
	items := selectImpactRepairQueueItems(plan, report, 0)
	items = filterPassedVerifyGraphTelemetryItems(run, activeBatchID, items)
	items = filterPendingImpactRepairQueueItems(run, items)
	if len(items) == 0 {
		return nil
	}
	if len(items) > 4 {
		items = items[:4]
	}
	purpose := repairFollowupPurposeForItems(items)
	id := nextRepairBatchID(run, activeBatchID, repairFollowupBatchIDSuffix(purpose))
	expectedPaths := impactRepairExpectedPaths(items)
	criteria := impactRepairSuccessCriteria(items)
	criteria = append(criteria, impactRepairNavigationSuccessCriteria(items)...)
	batch := writeflow.WriteBatchPlan{
		ID:                   id,
		Goal:                 impactRepairFollowupGoal(items),
		Purpose:              purpose,
		Status:               writeflow.BatchReadyForChangePlan,
		NeedsCodeExploration: len(expectedPaths) == 0 || impactRepairNeedsGraphNavigation(items),
		ExploreTargets:       expectedPaths,
		ExpectedPaths:        expectedPaths,
		SuccessCriteria:      criteria,
	}
	return &batch
}

func selectImpactRepairQueueItems(plan *types.ChangePlan, report *types.ChangeReport, limit int) []impactRepairQueueItem {
	if plan == nil {
		return nil
	}
	seen := map[string]bool{}
	var items []impactRepairQueueItem
	add := func(item impactRepairQueueItem) {
		item = normalizeImpactRepairQueueItem(item)
		key := impactRepairQueueItemDedupeKey(item)
		if key == "" || !impactRepairCoverageNeedsWork(item.CoverageStatus) ||
			!impactRepairQueueItemRequiresFollowup(item) || seen[key] {
			return
		}
		seen[key] = true
		items = append(items, item)
	}
	if plan.PatchReview != nil {
		review := types.NormalizePatchReviewRecord(*plan.PatchReview)
		for _, finding := range review.Findings {
			if finding.Category != types.PatchReviewCategorySemanticCoverage {
				continue
			}
			item := impactRepairQueueItemFromFinding(finding)
			if item.Path == "" && item.RelatedPath == "" && len(item.Paths) == 0 {
				switch item.Kind {
				case "changed_symbol", "behavior_contract":
					item.Paths = planSourceRepairPaths(plan, 4)
				}
			}
			add(item)
		}
	}
	if plan.ImpactAnalysis != nil {
		analysis := types.NormalizeImpactAnalysisResult(*plan.ImpactAnalysis)
		for _, target := range analysis.VerificationTargets {
			add(impactRepairQueueItemFromTarget(target))
		}
	}
	for _, item := range verificationConfidenceRepairQueueItems(plan, report) {
		add(item)
	}
	for _, item := range verificationProofLedgerRepairQueueItems(plan, report) {
		add(item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		return items[i].ID < items[j].ID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func filterPassedVerifyGraphTelemetryItems(run *types.WriteWorkflowRun, activeBatchID string, items []impactRepairQueueItem) []impactRepairQueueItem {
	if len(items) == 0 || activeBatchLatestVerifyStatus(run, activeBatchID) != "passed" {
		return items
	}
	out := make([]impactRepairQueueItem, 0, len(items))
	for _, item := range items {
		if impactRepairQueueItemRequiresGraphNavigation(item) && !impactRepairQueueItemFromVerificationProof(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func activeBatchLatestVerifyStatus(run *types.WriteWorkflowRun, activeBatchID string) string {
	if run == nil {
		return ""
	}
	activeBatchID = strings.TrimSpace(activeBatchID)
	if activeBatchID == "" {
		activeBatchID = strings.TrimSpace(run.ActiveBatchID)
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) != activeBatchID {
			continue
		}
		for i := len(batch.Attempts) - 1; i >= 0; i-- {
			attempt := batch.Attempts[i]
			if strings.TrimSpace(attempt.Kind) == "verify" {
				return strings.TrimSpace(attempt.Status)
			}
		}
		return ""
	}
	return ""
}

func verificationProofLedgerRepairQueueItems(plan *types.ChangePlan, report *types.ChangeReport) []impactRepairQueueItem {
	if plan == nil || report == nil {
		return nil
	}
	ledger := types.BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != types.VerificationProofLedgerLowConfidence &&
		ledger.State != types.VerificationProofLedgerFailed {
		return nil
	}
	var items []impactRepairQueueItem
	for _, obligation := range ledger.Obligations {
		if !verificationProofLedgerObligationNeedsFollowup(obligation.Status) {
			continue
		}
		item := impactRepairQueueItem{
			ID:             "verification-proof-ledger:" + strings.TrimSpace(obligation.ID),
			Code:           firstNonEmptyController(obligation.ReasonCode, string(obligation.Status), "verification_proof_uncovered"),
			Kind:           obligation.Kind,
			Path:           obligation.Path,
			RelatedPath:    obligation.RelatedPath,
			Symbol:         obligation.Symbol,
			ContractRef:    obligation.ContractRef,
			CoverageStatus: verificationProofLedgerCoverageStatus(obligation.Status),
			EvidenceRef:    firstNonEmptyController(obligation.EvidenceRef, strings.Trim(strings.TrimSpace(obligation.Source)+":"+strings.TrimSpace(obligation.Category), ":")),
			Source:         "verification_proof_ledger",
			Priority:       impactRepairPriority(obligation.Kind),
		}
		if item.Path == "" && item.RelatedPath == "" && len(item.Paths) == 0 {
			switch item.Kind {
			case "changed_symbol", "behavior_contract", "rendered_text_placement_contract":
				if item.Symbol != "" || item.ContractRef != "" {
					item.Paths = planSourceRepairPaths(plan, 4)
				}
			}
		}
		if item.Path == "" && item.RelatedPath == "" && len(item.Paths) == 0 &&
			item.Symbol == "" && item.ContractRef == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func verificationProofLedgerObligationNeedsFollowup(status types.VerificationProofLedgerItemStatus) bool {
	switch status {
	case types.VerificationProofLedgerItemMissing,
		types.VerificationProofLedgerItemUnverified,
		types.VerificationProofLedgerItemUnavailable,
		types.VerificationProofLedgerItemUnknown:
		return true
	default:
		return false
	}
}

func verificationProofLedgerCoverageStatus(status types.VerificationProofLedgerItemStatus) string {
	switch status {
	case types.VerificationProofLedgerItemUnavailable:
		return impactCoverageUnavailable
	default:
		return impactCoverageUnverified
	}
}

func filterPendingImpactRepairQueueItems(run *types.WriteWorkflowRun, items []impactRepairQueueItem) []impactRepairQueueItem {
	if len(items) == 0 {
		return nil
	}
	impactAlreadyRequested := impactRepairFollowupAlreadyRequested(run, "")
	proofAlreadyRequested := verificationProofFollowupAlreadyRequested(run)
	out := make([]impactRepairQueueItem, 0, len(items))
	for _, item := range items {
		if impactRepairQueueItemFromVerificationProof(item) {
			if proofAlreadyRequested {
				continue
			}
		} else if impactAlreadyRequested {
			continue
		}
		out = append(out, item)
	}
	return out
}

func verificationConfidenceRepairQueueItems(plan *types.ChangePlan, report *types.ChangeReport) []impactRepairQueueItem {
	if plan == nil || report == nil || len(report.VerificationConfidence) == 0 {
		return nil
	}
	planID := strings.TrimSpace(plan.ID)
	reportPlanID := strings.TrimSpace(report.PlanID)
	if planID != "" && reportPlanID != "" && planID != reportPlanID {
		return nil
	}
	paths := planSourceRepairPaths(plan, 4)
	var items []impactRepairQueueItem
	addContractRefs := func(rec types.VerificationConfidenceRecord, refs []string) {
		for _, ref := range dedupTrimControllerStrings(refs) {
			items = append(items, impactRepairQueueItem{
				ID:             "verification-confidence:" + strings.TrimSpace(rec.Category) + ":" + strings.TrimSpace(rec.ReasonCode) + ":" + ref,
				Code:           strings.TrimSpace(rec.ReasonCode),
				Kind:           "behavior_contract",
				Paths:          paths,
				ContractRef:    ref,
				CoverageStatus: impactCoverageUnverified,
				EvidenceRef:    strings.Trim(strings.TrimSpace(rec.Source)+":"+strings.TrimSpace(rec.Category), ":"),
				Source:         "verification_confidence",
				Priority:       impactRepairPriority("behavior_contract"),
			})
		}
	}
	addChangedSymbolRefs := func(rec types.VerificationConfidenceRecord, refs []string) {
		for _, ref := range dedupTrimControllerStrings(refs) {
			items = append(items, impactRepairQueueItem{
				ID:             "verification-confidence:" + strings.TrimSpace(rec.Category) + ":" + strings.TrimSpace(rec.ReasonCode) + ":" + ref,
				Code:           strings.TrimSpace(rec.ReasonCode),
				Kind:           "changed_symbol",
				Paths:          paths,
				Symbol:         ref,
				CoverageStatus: impactCoverageUnverified,
				EvidenceRef:    strings.Trim(strings.TrimSpace(rec.Source)+":"+strings.TrimSpace(rec.Category), ":"),
				Source:         "verification_confidence",
				Priority:       impactRepairPriority("changed_symbol"),
			})
		}
	}
	for _, rec := range report.VerificationConfidence {
		if strings.TrimSpace(rec.Status) != "missing" || strings.TrimSpace(rec.ReasonCode) == "" {
			continue
		}
		switch strings.TrimSpace(rec.Category) {
		case "probe_soft_contract_refs", "probe_contract_refs":
			addContractRefs(rec, rec.ContractRefs)
		case "probe_changed_symbol":
			addChangedSymbolRefs(rec, rec.ChangedSymbolRefs)
		}
	}
	return items
}

func planSourceRepairPaths(plan *types.ChangePlan, limit int) []string {
	if plan == nil {
		return nil
	}
	var raw []string
	raw = append(raw, plan.AppliedPaths...)
	raw = append(raw, plan.TargetPaths...)
	for _, change := range plan.Changes {
		raw = append(raw, change.Path, change.NewPath)
	}
	seen := map[string]bool{}
	var out []string
	for _, candidate := range raw {
		path := normalizeControllerPath(candidate)
		if path == "" || seen[path] {
			continue
		}
		if types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(path)) {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func repairFollowupPurposeForItems(items []impactRepairQueueItem) string {
	hasImpact := false
	hasProof := false
	for _, item := range items {
		if impactRepairQueueItemFromVerificationProof(item) {
			hasProof = true
		} else {
			hasImpact = true
		}
	}
	switch {
	case hasImpact && hasProof:
		return "impact_and_verification_proof_followup"
	case hasProof:
		return "verification_proof_followup"
	default:
		return "impact_obligation_followup"
	}
}

func repairFollowupBatchIDSuffix(purpose string) string {
	switch strings.TrimSpace(purpose) {
	case "verification_proof_followup":
		return "proof-repair"
	case "impact_and_verification_proof_followup":
		return "obligation-repair"
	default:
		return "impact-repair"
	}
}

func repairFollowupRequestedReasonCodes(purpose string) []string {
	switch strings.TrimSpace(purpose) {
	case "verification_proof_followup":
		return []string{"verification_proof_followup_requested"}
	case "impact_and_verification_proof_followup":
		return []string{"impact_obligation_followup_requested", "verification_proof_followup_requested"}
	default:
		return []string{"impact_obligation_followup_requested"}
	}
}

func repairFollowupProgressMessage(purpose string) string {
	switch strings.TrimSpace(purpose) {
	case "verification_proof_followup":
		return "typed verification proof obligations remain uncovered after a non-failed verifier attempt; appending one bounded proof follow-up batch"
	case "impact_and_verification_proof_followup":
		return "typed impact and verification proof obligations remain uncovered after a non-failed verifier attempt; appending one bounded follow-up batch"
	default:
		return "typed impact obligations remain uncovered after a non-failed verifier attempt; appending one bounded follow-up batch"
	}
}

func repairFollowupActionReasonCode(purpose string, explore bool) string {
	suffix := ""
	if explore {
		suffix = "_explore"
	}
	switch strings.TrimSpace(purpose) {
	case "verification_proof_followup":
		return "verification_proof_followup" + suffix
	case "impact_and_verification_proof_followup":
		return "typed_obligation_followup" + suffix
	default:
		return "impact_obligation_followup" + suffix
	}
}

func repairFollowupDecisionReason(purpose string, explore bool) string {
	switch strings.TrimSpace(purpose) {
	case "verification_proof_followup":
		if explore {
			return "typed verification proof obligations need bounded localization before repair planning"
		}
		return "typed verification proof obligations remain uncovered after local observation"
	case "impact_and_verification_proof_followup":
		if explore {
			return "typed impact and verification proof obligations need bounded localization before repair planning"
		}
		return "typed impact and verification proof obligations remain uncovered after local observation"
	default:
		if explore {
			return "typed impact obligations need bounded localization before repair planning"
		}
		return "typed impact obligations remain uncovered after local observation"
	}
}

func impactRepairQueueItemFromTarget(target types.ImpactVerificationTarget) impactRepairQueueItem {
	normalized := types.NormalizeImpactAnalysisResult(types.ImpactAnalysisResult{VerificationTargets: []types.ImpactVerificationTarget{target}})
	if len(normalized.VerificationTargets) == 0 {
		return impactRepairQueueItem{}
	}
	target = normalized.VerificationTargets[0]
	return impactRepairQueueItem{
		ID:             firstNonEmptyController(target.ID, "impact-target:"+target.Kind+":"+target.Path+":"+target.RelatedPath+":"+target.Symbol+":"+target.ContractRef+":"+target.EvidenceRef),
		Code:           target.Kind + "_coverage_unverified",
		Kind:           target.Kind,
		Path:           target.Path,
		RelatedPath:    target.RelatedPath,
		Symbol:         target.Symbol,
		ContractRef:    target.ContractRef,
		CoverageStatus: target.CoverageStatus,
		EvidenceRef:    target.EvidenceRef,
		Source:         firstNonEmptyController(target.Source, "impact_analysis"),
		Priority:       target.Priority,
	}
}

func impactRepairQueueItemFromFinding(finding types.PatchReviewFinding) impactRepairQueueItem {
	kind := strings.TrimSpace(finding.ImpactKind)
	if kind == "" {
		kind = impactRepairKindFromPatchReviewCode(finding.Code)
	}
	return impactRepairQueueItem{
		ID:             firstNonEmptyController(finding.Code+":"+finding.Path+":"+finding.RelatedPath+":"+finding.SubjectSymbol+":"+finding.EvidenceRef, finding.Code),
		Code:           finding.Code,
		Kind:           kind,
		Path:           finding.Path,
		RelatedPath:    finding.RelatedPath,
		Symbol:         finding.SubjectSymbol,
		CoverageStatus: string(finding.CoverageStatus),
		EvidenceRef:    finding.EvidenceRef,
		Source:         "patch_review",
		Priority:       impactRepairPriority(kind),
	}
}

func normalizeImpactRepairQueueItem(item impactRepairQueueItem) impactRepairQueueItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Code = strings.TrimSpace(item.Code)
	item.Kind = strings.TrimSpace(item.Kind)
	item.Path = normalizeControllerPath(item.Path)
	item.RelatedPath = normalizeControllerPath(item.RelatedPath)
	for i, raw := range item.Paths {
		item.Paths[i] = normalizeControllerPath(raw)
	}
	item.Paths = dedupTrimControllerStrings(item.Paths)
	item.Symbol = strings.TrimSpace(item.Symbol)
	item.ContractRef = strings.TrimSpace(item.ContractRef)
	item.CoverageStatus = strings.TrimSpace(item.CoverageStatus)
	item.EvidenceRef = strings.TrimSpace(item.EvidenceRef)
	item.Source = strings.TrimSpace(item.Source)
	if item.Kind == "" {
		item.Kind = "semantic_coverage"
	}
	if item.Priority <= 0 {
		item.Priority = impactRepairPriority(item.Kind)
	}
	if item.ID == "" {
		item.ID = strings.Join([]string{item.Kind, item.Code, item.Path, item.RelatedPath, strings.Join(item.Paths, ","), item.Symbol, item.ContractRef, item.EvidenceRef}, "|")
	}
	return item
}

func impactRepairQueueItemDedupeKey(item impactRepairQueueItem) string {
	item = normalizeImpactRepairQueueItem(item)
	key := strings.Join([]string{
		item.Kind,
		item.Path,
		item.RelatedPath,
		strings.Join(item.Paths, ","),
		item.Symbol,
		item.ContractRef,
		item.EvidenceRef,
	}, "|")
	key = strings.Trim(key, "|")
	if key == "" {
		key = strings.TrimSpace(item.ID)
	}
	return key
}

func impactRepairCoverageNeedsWork(status string) bool {
	switch strings.TrimSpace(status) {
	case "", "unverified", "unavailable", "unknown":
		return true
	default:
		return false
	}
}

func impactRepairQueueItemRequiresFollowup(item impactRepairQueueItem) bool {
	item = normalizeImpactRepairQueueItem(item)
	switch item.Kind {
	case "changed_symbol", "behavior_contract", "rendered_text_placement_contract":
		return true
	case "dependent", "dependency", "test_surface":
		return true
	case "semantic_coverage", "effect_followup":
		return writeflow.PatchReviewEffectUnknownCoverage(item.Code)
	default:
		return false
	}
}

func impactRepairQueueItemFromVerificationConfidence(item impactRepairQueueItem) bool {
	return strings.TrimSpace(item.Source) == "verification_confidence"
}

func impactRepairQueueItemFromVerificationProof(item impactRepairQueueItem) bool {
	item = normalizeImpactRepairQueueItem(item)
	if impactRepairQueueItemFromVerificationConfidence(item) {
		return true
	}
	if item.Source == "verification_proof_ledger" {
		return true
	}
	switch item.Code {
	case "changed_symbol_without_probe_coverage", "behavior_contract_without_verify_coverage":
		return true
	default:
		return false
	}
}

func impactRepairKindFromPatchReviewCode(code string) string {
	switch strings.TrimSpace(code) {
	case "changed_symbol_without_probe_coverage":
		return "changed_symbol"
	case "behavior_contract_without_verify_coverage":
		return "behavior_contract"
	case "dependent_surface_without_verify_coverage":
		return "dependent"
	case "related_test_surface_unverified":
		return "test_surface"
	default:
		return "semantic_coverage"
	}
}

func impactRepairPriority(kind string) int {
	switch strings.TrimSpace(kind) {
	case "behavior_contract":
		return 10
	case "rendered_text_placement_contract":
		return 15
	case "changed_symbol":
		return 20
	case "dependent":
		return 30
	case "test_surface":
		return 40
	case "effect_followup", "semantic_coverage":
		return 50
	case "changed_file":
		return 60
	default:
		return 90
	}
}

func impactRepairExpectedPaths(items []impactRepairQueueItem) []string {
	seen := map[string]bool{}
	var paths []string
	for _, item := range items {
		rawPaths := append([]string{item.Path, item.RelatedPath}, item.Paths...)
		for _, raw := range rawPaths {
			path := normalizeControllerPath(raw)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	if len(paths) > 8 {
		paths = paths[:8]
	}
	return paths
}

func impactRepairSuccessCriteria(items []impactRepairQueueItem) []string {
	criteria := make([]string, 0, len(items))
	for _, item := range items {
		parts := []string{
			"impact_obligation=" + item.ID,
			"kind=" + item.Kind,
		}
		if item.Code != "" {
			parts = append(parts, "code="+item.Code)
		}
		if item.Path != "" {
			parts = append(parts, "path="+item.Path)
		}
		if item.RelatedPath != "" {
			parts = append(parts, "related_path="+item.RelatedPath)
		}
		if len(item.Paths) > 0 {
			parts = append(parts, "paths="+strings.Join(item.Paths, ","))
		}
		if item.Symbol != "" {
			parts = append(parts, "symbol="+item.Symbol)
		}
		if item.ContractRef != "" {
			parts = append(parts, "contract_ref="+item.ContractRef)
		}
		if impactRepairQueueItemFromVerificationProof(item) {
			parts = append(parts, "verification_probe_required=true")
		}
		if item.EvidenceRef != "" {
			parts = append(parts, "evidence_ref="+item.EvidenceRef)
		}
		if item.Source != "" {
			parts = append(parts, "source="+item.Source)
		}
		criteria = append(criteria, strings.Join(parts, " "))
	}
	return criteria
}

func impactRepairNeedsGraphNavigation(items []impactRepairQueueItem) bool {
	return len(impactRepairNavigationRoutes(items)) > 0
}

func impactRepairQueueItemRequiresGraphNavigation(item impactRepairQueueItem) bool {
	return len(impactRepairNavigationRoutes([]impactRepairQueueItem{item})) > 0
}

func impactRepairNavigationRoutes(items []impactRepairQueueItem) []types.RepoMapNavigationRoute {
	seen := map[types.RepoMapNavigationRoute]bool{}
	var routes []types.RepoMapNavigationRoute
	add := func(route types.RepoMapNavigationRoute) {
		route = types.NormalizeRepoMapNavigationRoute(string(route))
		if !route.Valid() || seen[route] {
			return
		}
		seen[route] = true
		routes = append(routes, route)
	}
	for _, item := range items {
		item = normalizeImpactRepairQueueItem(item)
		switch item.Kind {
		case "dependent", "dependency", "test_surface":
			add(types.RepoMapNavigationRouteEditImpact)
			add(types.RepoMapNavigationRouteRelationMap)
		}
	}
	return routes
}

func impactRepairNavigationSuccessCriteria(items []impactRepairQueueItem) []string {
	if len(items) == 0 {
		return nil
	}
	var rows []string
	seen := map[string]bool{}
	for _, item := range items {
		item = normalizeImpactRepairQueueItem(item)
		routes := impactRepairNavigationRoutes([]impactRepairQueueItem{item})
		for _, route := range routes {
			parts := []string{
				"repo_map_navigation_requirement",
				"route=" + string(route),
				"kind=" + item.Kind,
				"required=typed_impact_navigation_coverage",
			}
			if item.Path != "" {
				parts = append(parts, "path="+item.Path)
			}
			if item.RelatedPath != "" {
				parts = append(parts, "related_path="+item.RelatedPath)
			}
			if item.Source != "" {
				parts = append(parts, "source="+item.Source)
			}
			row := strings.Join(parts, " ")
			if seen[row] {
				continue
			}
			seen[row] = true
			rows = append(rows, row)
			if len(rows) >= 8 {
				return rows
			}
		}
	}
	return rows
}

func impactRepairFollowupGoal(items []impactRepairQueueItem) string {
	scope := strings.Join(impactRepairExpectedPaths(items), ", ")
	if scope == "" {
		scope = "the typed impact obligations from the active patch"
	}
	var codes []string
	seen := map[string]bool{}
	for _, item := range items {
		code := strings.TrimSpace(firstNonEmptyController(item.Code, item.Kind))
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		codes = append(codes, code)
		if len(codes) >= 4 {
			break
		}
	}
	suffix := ""
	if len(codes) > 0 {
		suffix = " Required coverage: " + strings.Join(codes, ", ") + "."
	}
	switch repairFollowupPurposeForItems(items) {
	case "verification_proof_followup":
		return "Close the typed verification proof obligations left uncovered by the latest local observation; keep the follow-up bounded to " + scope + "." + suffix
	case "impact_and_verification_proof_followup":
		return "Close the typed impact and verification proof obligations left uncovered by the active patch; keep the follow-up bounded to " + scope + "." + suffix
	default:
		return "Close the typed impact obligations left uncovered by the active patch; keep the follow-up bounded to " + scope + "." + suffix
	}
}

func nextImpactRepairBatchID(run *types.WriteWorkflowRun, activeBatchID string) string {
	return nextRepairBatchID(run, activeBatchID, "impact-repair")
}

func nextRepairBatchID(run *types.WriteWorkflowRun, activeBatchID string, suffix string) string {
	activeBatchID = strings.TrimSpace(activeBatchID)
	if activeBatchID == "" {
		activeBatchID = "batch-1"
	}
	suffix = strings.Trim(strings.TrimSpace(suffix), "-")
	if suffix == "" {
		suffix = "impact-repair"
	}
	base := activeBatchID + "-" + suffix
	if !workflowBatchIDExists(run, base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !workflowBatchIDExists(run, candidate) {
			return candidate
		}
	}
}

func workflowProgressReasonCount(run *types.WriteWorkflowRun, batchID, reasonCode string) int {
	if run == nil {
		return 0
	}
	batchID = strings.TrimSpace(batchID)
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		return 0
	}
	count := 0
	for _, item := range run.ProgressLedger {
		if strings.TrimSpace(item.ReasonCode) != reasonCode {
			continue
		}
		if batchID != "" && strings.TrimSpace(item.BatchID) != batchID {
			continue
		}
		count++
	}
	return count
}

func workflowBatchIDExists(run *types.WriteWorkflowRun, id string) bool {
	if run == nil {
		return false
	}
	id = strings.TrimSpace(id)
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == id {
			return true
		}
	}
	return false
}

func decisionTargetsActiveBatch(decision writeflow.WriteWorkflowDecision, activeID string) bool {
	activeID = strings.TrimSpace(activeID)
	if activeID == "" {
		return false
	}
	switch decision.Action {
	case writeflow.ActionExploreCode:
		if decision.ExplorationRequest == nil || strings.TrimSpace(decision.ExplorationRequest.BatchID) == "" {
			return true
		}
		return strings.TrimSpace(decision.ExplorationRequest.BatchID) == activeID
	case writeflow.ActionPlanBatch, writeflow.ActionReplanBatch, writeflow.ActionApplyPlan, writeflow.ActionVerifyBatch:
		if decision.Batch == nil || strings.TrimSpace(decision.Batch.ID) == "" {
			return true
		}
		return strings.TrimSpace(decision.Batch.ID) == activeID
	default:
		return false
	}
}

func activeBatchHasVerifyFailureHandoff(mu *types.MutableState, batchID string) bool {
	if mu == nil {
		return false
	}
	handoff := mu.VerifyFailureHandoff()
	if handoff == nil {
		return false
	}
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return strings.TrimSpace(handoff.BatchID) != ""
	}
	return strings.TrimSpace(handoff.BatchID) == batchID
}

func (o *Orchestrator) writePlanCanProceedWithoutApprovalPause(plan *types.ChangePlan) bool {
	if plan == nil || !changePlanReadyForApply(plan) || writePlanDeniedByApproval(plan) {
		return false
	}
	if plan.Approval != nil {
		approval := writeflow.DeriveApprovalExecutionView(plan)
		switch approval.State {
		case writeflow.ApprovalExecutionAutoAllowed:
			return true
		case writeflow.ApprovalExecutionManualApproved:
			return true
		default:
			return false
		}
	}
	policy := o.writeApprovalPolicy
	if policy == "" {
		policy = writeflow.ApprovalPolicyAutoSafe
	}
	assessment := writeflow.AssessWriteRisk(o.writeRiskAssessmentInput(plan))
	decision := writeflow.DecideWriteApproval(policy, assessment)
	return decision.Action == writeflow.ApprovalActionAutoExecute
}

func changePlanReadyForApply(plan *types.ChangePlan) bool {
	if plan == nil {
		return false
	}
	switch strings.TrimSpace(plan.Status) {
	case "", types.PlanStatusPending, types.PlanStatusApplyFailed:
		return true
	default:
		return false
	}
}

func writeWorkflowVerifyAttemptStatus(report *types.ChangeReport, err error) string {
	if report == nil {
		return "missing_report"
	}
	if report.NormalizeVerificationStatus() == types.VerificationStatusUnavailable {
		return "unverified"
	}
	if err != nil {
		return "failed"
	}
	if report.NormalizeVerificationStatus() == types.VerificationStatusFailed {
		return "failed"
	}
	return "passed"
}

func writeWorkflowVerifyAttemptReason(report *types.ChangeReport, err error) string {
	if report == nil {
		return "missing_report"
	}
	if report.NormalizeVerificationStatus() == types.VerificationStatusUnavailable {
		switch report.FailureKind {
		case types.FailureKindRunnerMissing, types.FailureKindParserError, types.FailureKindPreexistingBuildFailure:
			return string(report.FailureKind)
		}
		if len(report.NoTestsRunners) > 0 {
			return "no_tests"
		}
	}
	switch report.FailureKind {
	case types.FailureKindRunnerMissing, types.FailureKindParserError, types.FailureKindTimeout, types.FailureKindOOM,
		types.FailureKindCPULimit, types.FailureKindCrash, types.FailureKindPreexistingBuildFailure:
		return string(report.FailureKind)
	}
	if report.BuildFailed {
		return "build_failed"
	}
	if !report.Passed {
		return "tests_failed"
	}
	if err != nil {
		return "verify_error"
	}
	return "tests_passed"
}

func writeWorkflowVerifyAttemptFailureReason(report *types.ChangeReport) string {
	if report == nil {
		return ""
	}
	if reason := strings.TrimSpace(report.FailureReasonCode); reason != "" {
		return reason
	}
	if report.FailureKind != "" {
		return string(report.FailureKind)
	}
	if report.BuildFailed {
		return "build_failed"
	}
	if !report.Passed {
		return "tests_failed"
	}
	return ""
}

func writeWorkflowReportID(report *types.ChangeReport) string {
	if report == nil {
		return ""
	}
	stem := writeWorkflowArtifactFileStem(report.PlanID)
	if stem == "" {
		return ""
	}
	return stem + ".report.json"
}

func appendWorkflowBatchAttempt(batch *types.WriteWorkflowBatch, kind, status, reasonCode, planID, reportID, artifactRef string) {
	appendWorkflowBatchAttemptWithFailureReason(batch, kind, status, reasonCode, "", planID, reportID, artifactRef)
}

func appendWorkflowBatchAttemptWithFailureReason(batch *types.WriteWorkflowBatch, kind, status, reasonCode, failureReasonCode, planID, reportID, artifactRef string) {
	if batch == nil {
		return
	}
	batch.Attempts = append(batch.Attempts, types.WriteWorkflowAttempt{
		ID:                fmt.Sprintf("attempt-%d", len(batch.Attempts)+1),
		Kind:              strings.TrimSpace(kind),
		Status:            strings.TrimSpace(status),
		ReasonCode:        strings.TrimSpace(reasonCode),
		FailureReasonCode: strings.TrimSpace(failureReasonCode),
		PlanID:            strings.TrimSpace(planID),
		ReportID:          strings.TrimSpace(reportID),
		ArtifactRef:       strings.TrimSpace(artifactRef),
		StartedAt:         time.Now(),
		FinishedAt:        time.Now(),
	})
}

func appendControllerProgress(run *types.WriteWorkflowRun, batchID, reasonCode, message string) {
	if run == nil {
		return
	}
	run.ProgressLedger = append(run.ProgressLedger, types.WriteWorkflowProgress{
		BatchID:    strings.TrimSpace(batchID),
		Stage:      string(types.StageWriteController),
		Status:     "progress",
		ReasonCode: strings.TrimSpace(reasonCode),
		Message:    strings.TrimSpace(message),
		At:         time.Now(),
	})
}

func batchIDOrDefault(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		return raw
	}
	return fallback
}

func writePlanNeedsManualApproval(plan *types.ChangePlan, run ...*types.WriteWorkflowRun) bool {
	if plan == nil || plan.Approval == nil {
		return false
	}
	if plan.Approval.Action != string(writeflow.ApprovalActionManual) {
		return false
	}
	return !writeApprovalRecordAllowsManualApply(plan, types.PlanFingerprint(plan), run...)
}

func writePlanDeniedByApproval(plan *types.ChangePlan) bool {
	if plan == nil {
		return false
	}
	if plan.Status == types.PlanStatusBlocked {
		return true
	}
	return plan.Approval != nil && plan.Approval.Action == string(writeflow.ApprovalActionDeny)
}

// persistVerifyFailureEvidence makes a failed verify attempt durable before
// the next controller turn (and before the unconditional worktree cleanup at
// Run exit can destroy the bytes): the typed ChangeReport is saved through
// the standard report path, the typed TestSurface is saved as a standalone
// artifact, and the attempt's applied-commit patch is written next to it as
// <stem>.attempt-N.diff. Artifact refs are attached to the batch's latest
// verify attempt record. Capture is best-effort and never alters cleanup
// behaviour (red line L5); failures log at warning.
func (o *Orchestrator) persistVerifyFailureEvidence(run *types.WriteWorkflowRun, report *types.ChangeReport, attempt int) (string, string) {
	if o == nil || o.busCtx == nil || run == nil {
		return "", ""
	}
	if report != nil {
		o.saveChangeReport(report)
	}
	planID := ""
	if report != nil {
		planID = strings.TrimSpace(report.PlanID)
	}
	if planID == "" {
		if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
			planID = strings.TrimSpace(plan.ID)
		}
	}
	stem := writeWorkflowArtifactFileStem(planID)
	planDir := o.ensureChangeReportDir()
	if stem == "" || planDir == "" {
		return "", ""
	}
	surfaceRef := o.persistVerifyFailureSurface(run, report, stem, planDir, attempt)
	if o.busCtx.WorktreePath == "" || strings.TrimSpace(o.currentIterCommitSHA) == "" {
		return "", surfaceRef
	}
	patch, err := worktree.CaptureCommitPatch(o.busCtx.WorktreePath, o.currentIterCommitSHA)
	if err != nil {
		logging.Warning("[orchestrator] verify-failure diff capture failed: %v", err)
		return "", surfaceRef
	}
	if attempt < 1 {
		attempt = 1
	}
	diffName := fmt.Sprintf("%s.attempt-%d.diff", stem, attempt)
	diffPath := filepath.Join(planDir, diffName)
	if err := os.WriteFile(diffPath, []byte(patch), 0o644); err != nil {
		logging.Warning("[orchestrator] verify-failure diff save failed: %v", err)
		return "", surfaceRef
	}
	logging.Info("[orchestrator] verify-failure attempt diff saved: %s", diffPath)
	attachDiffArtifactToLatestVerifyAttempt(run, run.ActiveBatchID, diffName)
	return diffName, surfaceRef
}

func (o *Orchestrator) resolveVerifyFailureHandoffArtifacts(h *types.VerifyFailureHandoff) *types.VerifyFailureHandoff {
	if h == nil {
		return nil
	}
	types.ResolveVerifyFailureHandoffArtifactPaths(h, o.ensureChangeReportDir())
	o.attachVerifyFailureRepairSourceSnapshots(h)
	return h
}

const (
	maxVerifyFailureRepairSourceSnapshots = 6
	verifyFailureRepairSourceWindowRadius = 12
)

type verifyFailureRepairSourceCandidate struct {
	path       string
	line       int
	reasonCode string
}

func (o *Orchestrator) attachVerifyFailureRepairSourceSnapshots(h *types.VerifyFailureHandoff) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || h == nil {
		return
	}
	repoRoot := strings.TrimSpace(o.busCtx.WorktreePath)
	if repoRoot == "" {
		repoRoot = strings.TrimSpace(o.busCtx.RepoRoot)
	}
	if repoRoot == "" {
		repoRoot = strings.TrimSpace(o.busCtx.MainRepoRoot)
	}
	if repoRoot == "" {
		return
	}
	plan := o.busCtx.Mutable.ChangePlan()
	candidates := verifyFailureRepairSourceCandidates(repoRoot, h, plan)
	if len(candidates) == 0 {
		return
	}
	seen := map[string]struct{}{}
	for _, cand := range candidates {
		if len(h.RepairSourceSnapshots) >= maxVerifyFailureRepairSourceSnapshots {
			break
		}
		content, ok := readVerifyFailureRepairSource(repoRoot, cand.path)
		if !ok {
			continue
		}
		start, end, snippet := verifyFailureRepairSourceWindow(content, cand.line, verifyFailureRepairSourceWindowRadius)
		if snippet == "" {
			continue
		}
		key := cand.path + "|" + strconv.Itoa(start) + "|" + strconv.Itoa(end)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		snap := types.RepairSourceSnapshot{
			Path:       cand.path,
			LineStart:  start,
			LineEnd:    end,
			ReasonCode: cand.reasonCode,
			Snippet:    snippet,
		}
		if anchor, ok := sourceowner.FindEnclosingOwner(cand.path, content, cand.line); ok {
			snap.OwnerSymbol = anchor.OwnerSymbol
			snap.AnchorSymbol = anchor.AnchorSymbol
		}
		h.RepairSourceSnapshots = append(h.RepairSourceSnapshots, snap)
	}
}

func verifyFailureRepairSourceCandidates(repoRoot string, h *types.VerifyFailureHandoff, plan *types.ChangePlan) []verifyFailureRepairSourceCandidate {
	seen := map[string]struct{}{}
	var out []verifyFailureRepairSourceCandidate
	add := func(path string, line int, reason string) {
		path = verifyFailureRepairSourcePath(repoRoot, path)
		reason = strings.TrimSpace(reason)
		if path == "" || line <= 0 || reason == "" || !types.HasCodeOrConfigPathSuffix(path) {
			return
		}
		key := path + "|" + strconv.Itoa(line) + "|" + reason
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		out = append(out, verifyFailureRepairSourceCandidate{path: path, line: line, reasonCode: reason})
	}
	if plan != nil {
		for path, lines := range planEditOwnerPatchEffectLineCandidates(plan.PatchEffect) {
			for _, line := range lines {
				add(path, line, "patch_effect_added_line")
			}
		}
	}
	if h != nil {
		for _, be := range h.BuildErrors {
			add(be.File, be.Line, "build_error_line")
		}
		for _, signal := range h.FailureSignals {
			if path, line, ok := verifyFailureRepairSignalLocation(signal.Location); ok {
				add(path, line, "failure_signal_location")
			}
		}
	}
	if len(out) == 0 && plan != nil {
		for _, path := range plan.TargetPaths {
			add(path, 1, "target_path_current_source")
		}
	}
	return out
}

func verifyFailureRepairSourcePath(repoRoot, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "<") {
		return ""
	}
	if filepath.IsAbs(raw) {
		root := strings.TrimSpace(repoRoot)
		if root == "" {
			return ""
		}
		rel, err := filepath.Rel(root, raw)
		if err != nil {
			return ""
		}
		raw = rel
	}
	path := normalizeControllerPath(raw)
	if path == "" || path == "." || verifyFailureRepairSourcePathEscapesRepo(path) || filepath.IsAbs(path) {
		return ""
	}
	return path
}

func verifyFailureRepairSourcePathEscapesRepo(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	return clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../")
}

func verifyFailureRepairSignalLocation(location string) (string, int, bool) {
	location = strings.TrimSpace(location)
	if location == "" || strings.HasPrefix(location, "<") {
		return "", 0, false
	}
	idx := strings.LastIndex(location, ":")
	if idx <= 0 || idx >= len(location)-1 {
		return "", 0, false
	}
	line, err := strconv.Atoi(strings.TrimSpace(location[idx+1:]))
	if err != nil || line <= 0 {
		return "", 0, false
	}
	return strings.TrimSpace(location[:idx]), line, true
}

func readVerifyFailureRepairSource(repoRoot, rel string) ([]byte, bool) {
	if rel == "" || verifyFailureRepairSourcePathEscapesRepo(rel) || filepath.IsAbs(rel) {
		return nil, false
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil || len(content) == 0 {
		return nil, false
	}
	return content, true
}

func verifyFailureRepairSourceWindow(content []byte, line, radius int) (int, int, string) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return 0, 0, ""
	}
	if line <= 0 {
		line = 1
	}
	if line > len(lines) {
		line = len(lines)
	}
	if radius < 0 {
		radius = 0
	}
	start := line - radius
	if start < 1 {
		start = 1
	}
	end := line + radius
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%5d| %s\n", i, lines[i-1])
	}
	return start, end, strings.TrimRight(b.String(), "\n")
}

func (o *Orchestrator) persistVerifyFailureSurface(run *types.WriteWorkflowRun, report *types.ChangeReport, stem, planDir string, attempt int) string {
	if report == nil || report.TestSurface == nil || run == nil {
		return ""
	}
	if attempt < 1 {
		attempt = 1
	}
	surfaceName := fmt.Sprintf("%s.attempt-%d.surface.json", stem, attempt)
	surfacePath := filepath.Join(planDir, surfaceName)
	if err := types.WriteTestSurfaceToFile(report.TestSurface, surfacePath); err != nil {
		logging.Warning("[orchestrator] verify-failure test-surface save failed: %v", err)
		return ""
	}
	logging.Info("[orchestrator] verify-failure test surface saved: %s", surfacePath)
	attachSurfaceArtifactToLatestVerifyAttempt(run, run.ActiveBatchID, surfaceName)
	return surfaceName
}

// attachDiffArtifactToLatestVerifyAttempt points the most recent verify
// attempt's ArtifactRef at the captured attempt diff so audit and resume can
// find the bytes without the worktree.
func attachDiffArtifactToLatestVerifyAttempt(run *types.WriteWorkflowRun, batchID, artifactRef string) {
	if run == nil || strings.TrimSpace(batchID) == "" || strings.TrimSpace(artifactRef) == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID != batchID {
			continue
		}
		for j := len(run.Batches[i].Attempts) - 1; j >= 0; j-- {
			if run.Batches[i].Attempts[j].Kind != "verify" {
				continue
			}
			run.Batches[i].Attempts[j].ArtifactRef = strings.TrimSpace(artifactRef)
			return
		}
		return
	}
}

func attachSurfaceArtifactToLatestVerifyAttempt(run *types.WriteWorkflowRun, batchID, surfaceRef string) {
	if run == nil || strings.TrimSpace(batchID) == "" || strings.TrimSpace(surfaceRef) == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID != batchID {
			continue
		}
		for j := len(run.Batches[i].Attempts) - 1; j >= 0; j-- {
			if run.Batches[i].Attempts[j].Kind != "verify" {
				continue
			}
			run.Batches[i].Attempts[j].SurfaceRef = strings.TrimSpace(surfaceRef)
			return
		}
		return
	}
}

// mirrorActivePlanToImportFile writes the current active ChangePlan back to
// the --plan-file path the run was started with, so the operator-visible
// artifact always reflects the live plan across replan rounds. It also
// re-anchors busCtx.PlanPath at that file so downstream consumers (report
// directory resolution, approval persistence, the preserve-on-success
// worktree path write at Run exit) keep working after the per-round
// planning reset cleared the original path. No-op when the run was not
// started from a plan file.
func (o *Orchestrator) mirrorActivePlanToImportFile(mirrorPath string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || strings.TrimSpace(mirrorPath) == "" {
		return
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		return
	}
	if err := types.WritePlanToFile(plan, mirrorPath); err != nil {
		logging.Warning("[orchestrator] active plan mirror to %s failed: %v", mirrorPath, err)
		return
	}
	o.planPath = mirrorPath
	o.busCtx.PlanPath = mirrorPath
}

// runBudgetCompletionVerify is the budget-exhaustion completion lane: when
// the active batch's latest apply attempt succeeded and no verify attempt
// followed it, one bounded verify dispatch runs past the ceiling so the
// applied work receives a typed verdict. Returns true when that verdict
// completes the run (batch verified green); on failure the standard
// evidence persistence runs and the caller proceeds to the blocked verdict.
// All conditions read typed attempt records — never prose.
func (o *Orchestrator) runBudgetCompletionVerify(run *types.WriteWorkflowRun, stepsUsed *int, importedPlanMirror string) bool {
	return o.runAppliedPendingCompletionVerify(run, stepsUsed, importedPlanMirror,
		"budget_completion_verify",
		"running final verification for the applied batch before the budget verdict",
		"budget exhausted with applied-but-unverified batch")
}

// runDispatchInterruptedCompletionVerify gives the online convergence loop one
// final observation opportunity when controller dispatch is interrupted after a
// patch has already been applied. User/global cancellation remains authoritative:
// if the cancel token is set, the scheduler returns the interruption immediately.
func (o *Orchestrator) runDispatchInterruptedCompletionVerify(run *types.WriteWorkflowRun, stepsUsed *int, importedPlanMirror string) bool {
	if o != nil && o.cancelToken != nil && o.cancelToken.IsCanceled() {
		return false
	}
	return o.runAppliedPendingCompletionVerify(run, stepsUsed, importedPlanMirror,
		"controller_dispatch_completion_verify",
		"controller dispatch was interrupted after apply; running final verification before surfacing interruption",
		"controller dispatch interrupted with applied-but-unverified batch")
}

// completeDispatchInterruptedProofFollowupIfSourceComplete handles write
// deadlines that land after the source batch already has a typed completion
// verdict and the active follow-up is proof-only/no-change. In that state a
// blocked workflow would force needless user recovery for missing local proof,
// so the run completes with an explicit unverified caveat instead. The decision
// reads only workflow attempts, batch purpose, ChangePlan shape, and typed
// failure handoff state.
func (o *Orchestrator) completeDispatchInterruptedProofFollowupIfSourceComplete(run *types.WriteWorkflowRun, reasonCode string) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return false
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if !activeBatchProofProbeOnlyPendingVerify(run, plan) {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" || activeBatchHasVerifyFailureHandoff(o.busCtx.Mutable, activeID) {
		return false
	}
	if !workflowRunNonActiveBatchesComplete(run) {
		return false
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		reasonCode = "proof_followup_unverified"
	}
	updateWorkflowRunBatchStatus(run, activeID, types.WriteWorkflowBatchComplete)
	updateWorkflowRunBatchCompletion(run, activeID, types.WriteWorkflowCompletionUnverified, reasonCode, "write_deadline")
	appendControllerProgress(run, activeID, reasonCode,
		"write-mode wall-clock deadline interrupted a proof-only follow-up after source batches already had typed completion verdicts; finishing with unverified proof caveat instead of blocking")
	run.Status = types.WriteWorkflowRunComplete
	writeflow.MarkWorkflowRunCompletionFromBatches(run)
	o.persistWriteWorkflowRun(run)
	if strings.TrimSpace(o.busCtx.Mutable.Result()) == "" {
		result := "write workflow complete"
		if caveat := writeWorkflowUnverifiedCompletionCaveat(*run); caveat != "" {
			result += " — " + caveat
		}
		o.busCtx.Mutable.SetResult(result)
	}
	return true
}

// completeInterruptedFollowupIfSourceComplete handles optional proof/impact
// follow-up batches that are interrupted before they apply anything while all
// primary source batches already have typed completion verdicts. In that state
// blocking the whole workflow would poison a deliverable source patch with an
// unfinished audit lane, so the follow-up completes as unverified instead. The
// gate consumes only durable workflow batch purpose, attempts, completion, and
// verify-failure handoff state.
func (o *Orchestrator) completeInterruptedFollowupIfSourceComplete(run *types.WriteWorkflowRun, reasonCode, source string) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return false
	}
	if !activeBatchOptionalFollowupPurpose(run) {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" || activeBatchHasVerifyFailureHandoff(o.busCtx.Mutable, activeID) {
		return false
	}
	if writeWorkflowActiveBatchHasFailedVerify(run) || writeWorkflowActiveBatchHasAppliedAttempt(run) {
		return false
	}
	if !workflowRunNonActiveBatchesComplete(run) {
		return false
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		reasonCode = "followup_unverified"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "followup_interrupted"
	}
	updateWorkflowRunBatchStatus(run, activeID, types.WriteWorkflowBatchComplete)
	updateWorkflowRunBatchCompletion(run, activeID, types.WriteWorkflowCompletionUnverified, reasonCode, source)
	appendControllerProgress(run, activeID, reasonCode,
		"optional proof/impact follow-up stopped before applying work after primary source batches already completed; finishing with unverified follow-up caveat instead of blocking the delivered patch")
	run.Status = types.WriteWorkflowRunComplete
	writeflow.MarkWorkflowRunCompletionFromBatches(run)
	o.persistWriteWorkflowRun(run)
	if strings.TrimSpace(o.busCtx.Mutable.Result()) == "" {
		result := "write workflow complete"
		if caveat := writeWorkflowUnverifiedCompletionCaveat(*run); caveat != "" {
			result += " — " + caveat
		}
		o.busCtx.Mutable.SetResult(result)
	}
	return true
}

func workflowRunNonActiveBatchesComplete(run *types.WriteWorkflowRun) bool {
	if run == nil {
		return false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return false
	}
	completed := 0
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == activeID {
			continue
		}
		if batch.Status != types.WriteWorkflowBatchComplete {
			return false
		}
		completed++
	}
	return completed > 0
}

// runAppliedPendingCompletionVerify is the shared completion lane: when the
// active batch's latest apply attempt succeeded and no verify attempt followed
// it, run one bounded typed verify before a terminal pacing/interruption verdict.
// Returns true when that verdict completes the run. On failure it persists the
// standard verify evidence and returns false so the caller can keep its original
// terminal behavior. All conditions read typed workflow attempts, never prose.
func (o *Orchestrator) runAppliedPendingCompletionVerify(run *types.WriteWorkflowRun, stepsUsed *int, importedPlanMirror, reasonCode, message, logPrefix string) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || o.skipVerify {
		return false
	}
	if o.busCtx.Mutable.ChangePlan() == nil {
		return false
	}
	var active *types.WriteWorkflowBatch
	for i := range run.Batches {
		if run.Batches[i].ID == run.ActiveBatchID {
			active = &run.Batches[i]
			break
		}
	}
	if active == nil {
		return false
	}
	lastApplyIdx, lastVerifyIdx := -1, -1
	for i, attempt := range active.Attempts {
		switch attempt.Kind {
		case "apply":
			if attempt.Status == "applied" {
				lastApplyIdx = i
			}
		case "verify":
			lastVerifyIdx = i
		}
	}
	if lastApplyIdx < 0 || lastVerifyIdx > lastApplyIdx {
		return false
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		reasonCode = "completion_verify"
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "running final verification for the applied batch before terminalizing the workflow"
	}
	logPrefix = strings.TrimSpace(logPrefix)
	if logPrefix == "" {
		logPrefix = "terminalizing with applied-but-unverified batch"
	}
	logging.Info("[orchestrator] %s %s — running completion verify", logPrefix, run.ActiveBatchID)
	appendControllerProgress(run, run.ActiveBatchID, reasonCode, message)
	innerErr := o.runControllerVerifyBatch(stepsUsed)
	report := o.busCtx.Mutable.ChangeReport()
	if report != nil {
		o.ensureChangeReportPlanID(report)
		if reportErr := o.validateControllerPostApplyReport(report); reportErr != nil && innerErr == nil {
			innerErr = reportErr
		}
		o.syncMutablePlanStatusAfterVerify(report, innerErr)
		o.mirrorActivePlanToImportFile(importedPlanMirror)
		if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
			*run = attachPlanContextPackToWorkflowRun(*run, plan)
		}
		o.busCtx.Mutable.MergeWriteContextPack(types.WriteContextPackFromChangeReport(report))
		updateWorkflowRunBatchVerify(run, run.ActiveBatchID, report, writeWorkflowVerifyAttemptStatus(report, innerErr), writeWorkflowVerifyAttemptReason(report, innerErr))
		o.syncCurrentWriteContextPackToRun(run)
	}
	outcome := writeflow.ClassifyVerifyAttemptOutcome(report, innerErr)
	if report != nil && (outcome.Kind == writeflow.VerifyOutcomeReportPassed ||
		outcome.Kind == writeflow.VerifyOutcomeNoTests ||
		outcome.Kind == writeflow.VerifyOutcomeRunnerMissing ||
		outcome.Kind == writeflow.VerifyOutcomeParserError) {
		updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchComplete)
		switch outcome.Kind {
		case writeflow.VerifyOutcomeNoTests, writeflow.VerifyOutcomeRunnerMissing, writeflow.VerifyOutcomeParserError:
			updateWorkflowRunBatchCompletion(run, run.ActiveBatchID, types.WriteWorkflowCompletionUnverified, outcome.ReasonCode, "verify_attempt")
			appendControllerProgress(run, run.ActiveBatchID, "batch_unverified", outcome.ReasonCode)
		default:
			updateWorkflowRunBatchCompletion(run, run.ActiveBatchID, types.WriteWorkflowCompletionVerified, outcome.ReasonCode, "verify_attempt")
			appendControllerProgress(run, run.ActiveBatchID, "batch_verified", "")
		}
		o.busCtx.Mutable.ResetVerifyFailureHandoff()
		run.Status = types.WriteWorkflowRunComplete
		writeflow.MarkWorkflowRunCompletionFromBatches(run)
		o.persistWriteWorkflowRun(run)
		if strings.TrimSpace(o.busCtx.Mutable.Result()) == "" {
			result := "write workflow complete"
			if caveat := writeWorkflowUnverifiedCompletionCaveat(*run); caveat != "" {
				result += " — " + caveat
			}
			o.busCtx.Mutable.SetResult(result)
		}
		return true
	}
	if innerErr == nil && report != nil {
		innerErr = fmt.Errorf("verify failed: %s", strings.TrimSpace(report.FailureSummary))
	}
	if innerErr != nil {
		appendControllerProgress(run, run.ActiveBatchID, "verify_failed", innerErr.Error())
	}
	_, _ = o.persistVerifyFailureEvidence(run, report, writeflow.DeriveBatchAttemptState(*active).FailedVerifyAttempts+1)
	o.persistWriteWorkflowRun(run)
	return false
}

func (o *Orchestrator) completeBudgetExhaustedRunIfAllBatchesComplete(run *types.WriteWorkflowRun, reasonCode string) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || !workflowRunAllBatchesComplete(run) {
		return false
	}
	run.Status = types.WriteWorkflowRunComplete
	writeflow.MarkWorkflowRunCompletionFromBatches(run)
	appendControllerProgress(run, run.ActiveBatchID, reasonCode, "all workflow batches already have typed completion verdicts")
	o.persistWriteWorkflowRun(run)
	if strings.TrimSpace(o.busCtx.Mutable.Result()) == "" {
		result := "write workflow complete"
		if run.Completion != nil && run.Completion.Verdict == types.WriteWorkflowCompletionAcceptedFailed {
			result += " — accepted with failed verification: " + strings.TrimSpace(run.Completion.ReasonCode)
		}
		if caveat := writeWorkflowUnverifiedCompletionCaveat(*run); caveat != "" {
			result += " — " + caveat
		}
		o.busCtx.Mutable.SetResult(result)
	}
	return true
}

func (o *Orchestrator) completeDispatchInterruptedRunIfAllBatchesComplete(run *types.WriteWorkflowRun, reasonCode string) bool {
	if o == nil || run == nil || !workflowRunAllBatchesComplete(run) {
		return false
	}
	probe := types.CloneWriteWorkflowRun(*run)
	decision := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: reasonCode,
		Reason:     "controller dispatch was interrupted after all batches already had typed completion verdicts",
	}, &probe)
	if decision.Action != writeflow.ActionFinish {
		return false
	}
	if blocked := writeflow.FinishBlockedReason(*run, decision); blocked != "" {
		return false
	}
	return o.completeBudgetExhaustedRunIfAllBatchesComplete(run, reasonCode)
}

func workflowRunAllBatchesComplete(run *types.WriteWorkflowRun) bool {
	if run == nil || len(run.Batches) == 0 {
		return false
	}
	for _, batch := range run.Batches {
		if batch.Status != types.WriteWorkflowBatchComplete {
			return false
		}
	}
	return true
}

func writeWorkflowUnverifiedCompletionCaveat(run types.WriteWorkflowRun) string {
	var entries []string
	allNoTests := true
	for _, batch := range run.Batches {
		latestStatus := ""
		latestReason := ""
		for i := len(batch.Attempts) - 1; i >= 0; i-- {
			attempt := batch.Attempts[i]
			if strings.TrimSpace(attempt.Kind) != "verify" {
				continue
			}
			latestStatus = strings.TrimSpace(attempt.Status)
			latestReason = strings.TrimSpace(attempt.ReasonCode)
			break
		}
		if latestStatus != "unverified" {
			continue
		}
		entry := strings.TrimSpace(batch.ID)
		if entry == "" {
			entry = "batch"
		}
		if latestReason != "" && latestReason != "no_tests" {
			entry += " (" + latestReason + ")"
			allNoTests = false
		}
		if latestReason != "no_tests" {
			allNoTests = false
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return ""
	}
	if allNoTests {
		return "no tests executed for: " + strings.Join(entries, ", ")
	}
	return "local verification unavailable for: " + strings.Join(entries, ", ")
}

type planEmitRejectionView struct {
	Summary    string
	RepairPack *types.PlanRepairPack
}

func (v planEmitRejectionView) RenderHint() string {
	if v.RepairPack != nil {
		return renderPlanRepairPackRetryHint(*v.RepairPack)
	}
	return strings.TrimSpace(v.Summary)
}

func renderPlanRepairPackRetryHint(pack types.PlanRepairPack) string {
	pack = types.NormalizePlanRepairPack(pack)
	if pack.ReasonCode == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("plan_repair_pack:\n")
	if pack.ToolName != "" {
		fmt.Fprintf(&b, "- tool_name: %s\n", pack.ToolName)
	}
	fmt.Fprintf(&b, "- reason_code: %s\n", pack.ReasonCode)
	if pack.Message != "" {
		fmt.Fprintf(&b, "- message: %s\n", truncateInline(pack.Message, 320))
	}
	if len(pack.FailingFieldPaths) > 0 {
		fmt.Fprintf(&b, "- failing_fields: %s\n", strings.Join(pack.FailingFieldPaths, ", "))
	}
	if len(pack.FailingPaths) > 0 {
		fmt.Fprintf(&b, "- failing_paths: %s\n", strings.Join(pack.FailingPaths, ", "))
	}
	if len(pack.AcceptedEnums) > 0 {
		keys := make([]string, 0, len(pack.AcceptedEnums))
		for key := range pack.AcceptedEnums {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteString("- accepted_enums:\n")
		for _, key := range keys {
			fmt.Fprintf(&b, "  - %s: %s\n", key, strings.Join(pack.AcceptedEnums[key], ", "))
		}
	}
	if len(pack.CurrentBytes) > 0 {
		b.WriteString("- current_bytes:\n")
		for i, entry := range pack.CurrentBytes {
			if i >= 3 {
				fmt.Fprintf(&b, "  - ... %d more omitted\n", len(pack.CurrentBytes)-i)
				break
			}
			parts := []string{}
			if entry.Path != "" {
				parts = append(parts, "path="+entry.Path)
			}
			if entry.EditIndex > 0 {
				parts = append(parts, fmt.Sprintf("edit_index=%d", entry.EditIndex))
			}
			if entry.StartLine > 0 || entry.EndLine > 0 {
				parts = append(parts, fmt.Sprintf("lines=%d-%d", entry.StartLine, entry.EndLine))
			}
			if entry.AnchorLine > 0 {
				parts = append(parts, fmt.Sprintf("anchor_line=%d", entry.AnchorLine))
			}
			if entry.FileLineCount > 0 {
				parts = append(parts, fmt.Sprintf("file_line_count=%d", entry.FileLineCount))
			}
			if len(parts) == 0 {
				parts = append(parts, "entry")
			}
			fmt.Fprintf(&b, "  - %s\n", strings.Join(parts, " "))
			writePlanRepairBytesPreview(&b, "expected_old_text", entry.ExpectedOldText)
			writePlanRepairBytesPreview(&b, "supplied_old_text", entry.SuppliedOldText)
			writePlanRepairBytesPreview(&b, "current_bytes", entry.CurrentBytes)
			if len(entry.SafeEditKinds) > 0 {
				fmt.Fprintf(&b, "    safe_edit_kinds: %s\n", strings.Join(entry.SafeEditKinds, ", "))
			}
			if len(entry.RelocationCandidates) > 0 {
				b.WriteString("    relocation_candidates:\n")
				for _, candidate := range entry.RelocationCandidates {
					parts := []string{}
					if candidate.Path != "" {
						parts = append(parts, "path="+candidate.Path)
					}
					if candidate.StartLine > 0 || candidate.EndLine > 0 {
						parts = append(parts, fmt.Sprintf("lines=%d-%d", candidate.StartLine, candidate.EndLine))
					}
					if candidate.Source != "" {
						parts = append(parts, "source="+candidate.Source)
					}
					if len(parts) == 0 {
						continue
					}
					fmt.Fprintf(&b, "      - %s\n", strings.Join(parts, " "))
				}
			}
		}
	}
	if pack.PartialPlanRetained {
		b.WriteString("- partial_plan_retained: true\n")
	}
	if pack.RetryInstruction != "" {
		fmt.Fprintf(&b, "- retry_instruction: %s\n", truncateInline(pack.RetryInstruction, 320))
	}
	return truncatePlanRepairHint(b.String(), 2400)
}

func writePlanRepairBytesPreview(b *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(b, "    %s:\n", label)
	for _, line := range strings.Split(truncatePlanRepairHint(value, 600), "\n") {
		fmt.Fprintf(b, "      %s\n", line)
	}
}

func truncatePlanRepairHint(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return strings.TrimSpace(s[:max-3]) + "..."
}

// lastPlanEmitRejectionSummary returns the most recent failed plan-emit
// fallback summary (system-generated validation text, bounded). It is kept for
// compatibility with callers/tests that need the legacy scalar view; controller
// retry hints prefer lastPlanEmitRejectionView's typed repair metadata when
// available. Reads tool-result records only — never model prose.
func lastPlanEmitRejectionSummary(results []types.ToolResult) string {
	return lastPlanEmitRejectionView(results).Summary
}

func lastPlanEmitRejectionView(results []types.ToolResult) planEmitRejectionView {
	for i := len(results) - 1; i >= 0; i-- {
		r := results[i]
		switch r.ToolName {
		case "emit_change_plan", "emit_plan_skeleton", "emit_plan_change":
		default:
			continue
		}
		// The newest plan-emit outcome decides: a success means any older
		// rejection was already resolved and must not be re-cited.
		if r.Success {
			return planEmitRejectionView{}
		}
		summary := boundedPlanEmitRejectionSummary(r.Summary)
		if pack, ok := types.PlanRepairPackFromToolResult(r); ok {
			return planEmitRejectionView{Summary: summary, RepairPack: &pack}
		}
		return planEmitRejectionView{Summary: summary}
	}
	return planEmitRejectionView{}
}

func boundedPlanEmitRejectionSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	const maxLen = 600
	if len(summary) > maxLen {
		summary = summary[:maxLen] + "..."
	}
	return summary
}

// hydrateResumedWorkflowState rebuilds the in-memory controller state for a
// run loaded from the store. Inputs are typed only: failed-verify counts come
// from attempt records, the plan from the durable plan artifact next to the
// report directory (or the live mirror file), and the verify-failure carrier
// from the persisted report via the same projection the live path uses.
// Best-effort: a missing artifact degrades to the corresponding live-state
// default and logs at warning.
func (o *Orchestrator) hydrateResumedWorkflowState(run *types.WriteWorkflowRun, verifyFailures map[string]int) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil || len(run.Batches) == 0 {
		return
	}
	resumed := false
	for _, p := range run.ProgressLedger {
		if p.ReasonCode == "workflow_resumed" {
			resumed = true
			break
		}
	}
	if !resumed {
		return
	}
	var active *types.WriteWorkflowBatch
	for i := range run.Batches {
		st := writeflow.DeriveBatchAttemptState(run.Batches[i])
		if st.FailedVerifyAttempts > 0 {
			verifyFailures[run.Batches[i].ID] = st.FailedVerifyAttempts
		}
		if run.Batches[i].ID == run.ActiveBatchID {
			active = &run.Batches[i]
		}
	}
	if active == nil {
		return
	}
	st := writeflow.DeriveBatchAttemptState(*active)
	if o.busCtx.Mutable.ChangePlan() == nil && st.PlanID != "" {
		if plan := o.loadDurablePlanArtifact(st.PlanID); plan != nil {
			o.busCtx.Mutable.SetChangePlan(plan)
			logging.Info("[orchestrator] resume: hydrated active plan %s from durable artifact", st.PlanID)
		} else {
			logging.Warning("[orchestrator] resume: active batch references plan %s but no durable plan artifact was found", st.PlanID)
		}
	}
	if o.busCtx.Mutable.VerifyFailureHandoff() == nil && st.Phase == writeflow.BatchPhaseNeedsReplan && st.ReportID != "" {
		if report := o.loadDurableReportArtifact(st.ReportID); report != nil && !report.Passed {
			diffRef := ""
			if strings.HasSuffix(st.LatestVerifyArtifactRef, ".diff") {
				diffRef = st.LatestVerifyArtifactRef
			}
			if handoff := o.resolveVerifyFailureHandoffArtifacts(types.BuildVerifyFailureHandoff(report, active.ID, st.FailedVerifyAttempts, diffRef, st.LatestVerifySurfaceRef)); handoff != nil {
				o.busCtx.Mutable.SetVerifyFailureHandoff(handoff)
				logging.Info("[orchestrator] resume: rebuilt verify-failure context from %s", st.ReportID)
			}
		}
	}
}

// loadDurablePlanArtifact resolves a plan id to its persisted JSON: first the
// imported plan file when its id matches, then the report directory's
// <stem>.json sibling.
func (o *Orchestrator) loadDurablePlanArtifact(planID string) *types.ChangePlan {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return nil
	}
	if path := strings.TrimSpace(o.busCtx.PlanPath); path != "" {
		if plan, err := types.LoadChangePlanFromFile(path); err == nil && plan != nil && strings.TrimSpace(plan.ID) == planID {
			return plan
		}
	}
	stem := writeWorkflowArtifactFileStem(planID)
	dir := o.ensureChangeReportDir()
	if stem == "" || dir == "" {
		return nil
	}
	plan, err := types.LoadChangePlanFromFile(filepath.Join(dir, stem+".json"))
	if err != nil || plan == nil || strings.TrimSpace(plan.ID) != planID {
		return nil
	}
	return plan
}

// loadDurableReportArtifact reads a persisted ChangeReport by its artifact
// file name (the batch attempt's ReportID) from the report directory.
func (o *Orchestrator) loadDurableReportArtifact(reportID string) *types.ChangeReport {
	reportID = strings.TrimSpace(reportID)
	dir := o.ensureChangeReportDir()
	if reportID == "" || dir == "" {
		return nil
	}
	report, err := types.LoadChangeReportFromFile(filepath.Join(dir, reportID))
	if err != nil {
		return nil
	}
	return report
}
