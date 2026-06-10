package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

const (
	defaultWriteWorkflowMaxBatches           = 5
	defaultWriteWorkflowMaxExplorationRounds = 2
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
	// importedPlanMirror is the --plan-file path this run was started
	// with (empty otherwise). The controller workflow may replace the
	// active plan across replan rounds; mirroring every accepted plan
	// (and its later status/worktree updates) back to this file keeps
	// the operator-visible artifact pointing at the LIVE plan instead
	// of the first imported snapshot.
	importedPlanMirror := strings.TrimSpace(o.busCtx.PlanPath)
	explorationRounds := map[string]int{}
	verifyFailures := map[string]int{}
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
			run.Status = types.WriteWorkflowRunBlocked
			appendControllerProgress(&run, run.ActiveBatchID, "budget_exhausted", "global step budget exhausted")
			o.persistWriteWorkflowRun(&run)
			return fmt.Errorf("write workflow blocked: global step budget exhausted")
		}
		controllerTurns++
		o.busCtx.Mutable.SetWriteWorkflowRun(&run)
		decision, err := o.dispatchWriteControllerDecision()
		*stepsUsed++
		if err != nil {
			return err
		}
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
		o.syncCurrentWriteContextPackToRun(&run)
		o.persistWriteWorkflowRun(&run)

		switch decision.Action {
		case writeflow.ActionExploreCode:
			batchID := run.ActiveBatchID
			if batchID == "" && decision.ExplorationRequest != nil {
				batchID = decision.ExplorationRequest.BatchID
			}
			if explorationRounds[batchID] >= defaultWriteWorkflowMaxExplorationRounds {
				run.Status = types.WriteWorkflowRunBlocked
				updateWorkflowRunBatchStatus(&run, batchID, types.WriteWorkflowBatchBlocked)
				appendControllerProgress(&run, batchID, "exploration_budget_exhausted", "batch exploration round budget exhausted")
				o.persistWriteWorkflowRun(&run)
				return fmt.Errorf("write workflow blocked: exploration budget exhausted for %s", batchID)
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
				appendControllerProgress(&run, batchID, "exploration_degraded", err.Error())
			} else {
				updateWorkflowRunBatchStatus(&run, batchID, types.WriteWorkflowBatchReadyToPlan)
				appendControllerProgress(&run, batchID, "exploration_complete", "")
			}
			o.syncCurrentWriteContextPackToRun(&run)
			o.persistWriteWorkflowRun(&run)
		case writeflow.ActionPlanBatch, writeflow.ActionAppendBatch, writeflow.ActionSplitBatch, writeflow.ActionReplanBatch:
			if len(before.Batches) < len(run.Batches) && run.Budget.MaxBatches > 0 && len(run.Batches) > run.Budget.MaxBatches {
				run.Status = types.WriteWorkflowRunBlocked
				appendControllerProgress(&run, run.ActiveBatchID, "max_batches_reached", "controller attempted to append beyond max_batches")
				o.persistWriteWorkflowRun(&run)
				return fmt.Errorf("write workflow blocked: max batch budget reached")
			}
			innerErr := o.runControllerPlanBatch(decision.Batch, stepsUsed)
			plan := o.busCtx.Mutable.ChangePlan()
			if plan != nil {
				updateWorkflowRunBatchPlan(&run, run.ActiveBatchID, plan.ID)
				run = attachPlanContextPackToWorkflowRun(run, plan)
			}
			if innerErr != nil {
				lastInnerErr = innerErr
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchReadyToPlan)
				appendControllerProgress(&run, run.ActiveBatchID, "plan_batch_failed", lastInnerErr.Error())
				o.persistWriteWorkflowRun(&run)
				return lastInnerErr
			}
			updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchPlanned)
			appendControllerProgress(&run, run.ActiveBatchID, "batch_planned", "")
			o.mirrorActivePlanToImportFile(importedPlanMirror)
			o.syncCurrentWriteContextPackToRun(&run)
			o.persistWriteWorkflowRun(&run)
			if writePlanDeniedByApproval(plan) {
				run.Status = types.WriteWorkflowRunBlocked
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
				appendControllerProgress(&run, run.ActiveBatchID, "approval_denied", "typed write approval denied the plan")
				o.persistWriteWorkflowRun(&run)
				return fmt.Errorf("write workflow blocked: approval denied for plan %s", plan.ID)
			}
			if o.busCtx.Mode == types.ModeApply && writePlanNeedsManualApproval(plan) {
				run.Status = types.WriteWorkflowRunInProgress
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchPendingApproval)
				appendControllerProgress(&run, run.ActiveBatchID, string(types.WriteWorkflowBatchPendingApproval), "typed write approval requires operator confirmation")
				o.persistWriteWorkflowRun(&run)
				return fmt.Errorf("write workflow pending approval for plan %s: %s", plan.ID, plan.Approval.ReasonCode)
			}
			if o.busCtx.Mode == types.ModePlan {
				run.Status = types.WriteWorkflowRunComplete
				appendControllerProgress(&run, run.ActiveBatchID, "plan_mode_complete", "plan mode stops after producing a reviewable ChangePlan")
				o.persistWriteWorkflowRun(&run)
				return nil
			}
		case writeflow.ActionApplyPlan:
			if o.busCtx.Mode == types.ModePlan {
				run.Status = types.WriteWorkflowRunBlocked
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
				appendControllerProgress(&run, run.ActiveBatchID, "apply_not_allowed_in_plan_mode", "")
				o.persistWriteWorkflowRun(&run)
				return fmt.Errorf("write workflow blocked: apply_plan is not valid in plan mode")
			}
			innerErr := o.runControllerApplyPlan(stepsUsed)
			plan := o.busCtx.Mutable.ChangePlan()
			if plan != nil {
				updateWorkflowRunBatchPlan(&run, run.ActiveBatchID, plan.ID)
				updateWorkflowRunBatchApply(&run, run.ActiveBatchID, plan, writeWorkflowApplyAttemptStatus(plan, innerErr), writeWorkflowApplyAttemptReason(plan, innerErr))
				run = attachPlanContextPackToWorkflowRun(run, plan)
			}
			if innerErr != nil {
				lastInnerErr = innerErr
				switch {
				case writePlanNeedsManualApproval(plan):
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchPendingApproval)
					appendControllerProgress(&run, run.ActiveBatchID, string(types.WriteWorkflowBatchPendingApproval), innerErr.Error())
					o.persistWriteWorkflowRun(&run)
					return innerErr
				case plan != nil && plan.Status == types.PlanStatusBlocked:
					run.Status = types.WriteWorkflowRunBlocked
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
					appendControllerProgress(&run, run.ActiveBatchID, string(plan.Status), innerErr.Error())
					o.persistWriteWorkflowRun(&run)
					return innerErr
				default:
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchPlanned)
					appendControllerProgress(&run, run.ActiveBatchID, "apply_failed", innerErr.Error())
					o.busCtx.TaskState.LastError = ""
					o.persistWriteWorkflowRun(&run)
					continue
				}
			}
			updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchVerifying)
			appendControllerProgress(&run, run.ActiveBatchID, "batch_applied", "")
			o.syncCurrentWriteContextPackToRun(&run)
			o.persistWriteWorkflowRun(&run)
		case writeflow.ActionVerifyBatch:
			if o.busCtx.Mode == types.ModePlan {
				run.Status = types.WriteWorkflowRunBlocked
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
				appendControllerProgress(&run, run.ActiveBatchID, "verify_not_allowed_in_plan_mode", "")
				o.persistWriteWorkflowRun(&run)
				return fmt.Errorf("write workflow blocked: verify_batch is not valid in plan mode")
			}
			innerErr := o.runControllerVerifyBatch(stepsUsed)
			report := o.busCtx.Mutable.ChangeReport()
			if report != nil {
				o.ensureChangeReportPlanID(report)
				o.syncMutablePlanStatusAfterVerify(report, innerErr)
				o.mirrorActivePlanToImportFile(importedPlanMirror)
				o.busCtx.Mutable.MergeWriteContextPack(types.WriteContextPackFromChangeReport(report))
				updateWorkflowRunBatchVerify(&run, run.ActiveBatchID, report, writeWorkflowVerifyAttemptStatus(report, innerErr), writeWorkflowVerifyAttemptReason(report, innerErr))
				o.syncCurrentWriteContextPackToRun(&run)
			} else if o.skipVerify {
				o.syncMutablePlanStatusAfterSkippedVerify()
				if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
					updateWorkflowRunBatchVerifySkipped(&run, run.ActiveBatchID, plan.ID)
				}
			}
			if innerErr != nil || (report != nil && !report.Passed) {
				if innerErr == nil && report != nil {
					innerErr = fmt.Errorf("verify failed: %s", strings.TrimSpace(report.FailureSummary))
				}
				lastInnerErr = innerErr
				verifyFailures[run.ActiveBatchID]++
				updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchReadyToPlan)
				appendControllerProgress(&run, run.ActiveBatchID, "verify_failed", innerErr.Error())
				diffRef := o.persistVerifyFailureEvidence(&run, report, verifyFailures[run.ActiveBatchID])
				if handoff := types.BuildVerifyFailureHandoff(report, run.ActiveBatchID, verifyFailures[run.ActiveBatchID], diffRef); handoff != nil {
					o.busCtx.Mutable.SetVerifyFailureHandoff(handoff)
				}
				o.persistWriteWorkflowRun(&run)
				if verifyFailures[run.ActiveBatchID] > o.writeRetryBudget {
					run.Status = types.WriteWorkflowRunBlocked
					updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchBlocked)
					appendControllerProgress(&run, run.ActiveBatchID, "verify_retry_budget_exhausted", innerErr.Error())
					o.persistWriteWorkflowRun(&run)
					return innerErr
				}
				o.busCtx.TaskState.LastError = ""
				continue
			}
			updateWorkflowRunBatchStatus(&run, run.ActiveBatchID, types.WriteWorkflowBatchComplete)
			appendControllerProgress(&run, run.ActiveBatchID, "batch_verified", "")
			o.busCtx.Mutable.ResetVerifyFailureHandoff()
			o.persistWriteWorkflowRun(&run)
		case writeflow.ActionFinish:
			run.Status = types.WriteWorkflowRunComplete
			o.busCtx.Mutable.ResetVerifyFailureHandoff()
			o.persistWriteWorkflowRun(&run)
			if strings.TrimSpace(o.busCtx.Mutable.Result()) == "" {
				result := "write workflow complete"
				if decision.FinishDisposition == writeflow.FinishDispositionAcceptUnverified {
					if unverified := writeflow.UnverifiedBatchSummaries(run); len(unverified) > 0 {
						result += " — accepted with failed verification: " + strings.Join(unverified, ", ")
					}
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
			o.busCtx.Mutable.SetResultPlain(msg)
			return fmt.Errorf("%s", msg)
		}
	}
	if o.runBudgetCompletionVerify(&run, stepsUsed, importedPlanMirror) {
		return nil
	}
	run.Status = types.WriteWorkflowRunBlocked
	appendControllerProgress(&run, run.ActiveBatchID, "controller_turn_budget_exhausted", "")
	o.persistWriteWorkflowRun(&run)
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
			wc := o.busCtx.Mutable.WriteClosure()
			for _, c := range plan.Changes {
				wc.EnqueuePendingApply(types.PendingApply{
					Path:      c.Path,
					Rationale: c.Rationale,
					Origin:    "workflow_seed_plan_file",
				})
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
		run.Batches = append(run.Batches, types.WriteWorkflowBatch{
			ID:        run.ActiveBatchID,
			Goal:      strings.TrimSpace(importedPlan.Summary),
			Status:    types.WriteWorkflowBatchPlanned,
			PlanID:    importedPlan.ID,
			PlanRef:   importedPlan.ID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
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
			ID:        run.ActiveBatchID,
			Goal:      seed.NextBatch.Goal,
			Status:    status,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
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
	if pack := o.busCtx.Mutable.WriteContextPack(); pack != nil {
		run.ContextPacks = append(run.ContextPacks, *pack)
	}
	run = types.NormalizeWriteWorkflowRun(run)
	o.busCtx.Mutable.SetWriteWorkflowRun(&run)
	return run
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
	decision = writeflow.NormalizeWriteWorkflowDecision(decision)
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
	noPlanRetried := false
	lastStallSignature := ""
	for {
		o.prepareControllerPlanningState()
		_, err := o.runControllerWriteStage(types.StagePlan, stepsUsed)
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
		if err == nil && o.busCtx.Mutable.ChangePlan() != nil {
			return nil
		}
		// A planning round that ends with no ChangePlan installed is not
		// terminal on its first occurrence while typed verify-failure
		// evidence is waiting: grant exactly one re-dispatch whose planning
		// hint cites the empty round. The condition reads typed state only
		// (no plan installed + handoff present), never the error text.
		if !noPlanRetried && o.busCtx.Mutable.ChangePlan() == nil && o.busCtx.Mutable.VerifyFailureHandoff() != nil {
			noPlanRetried = true
			o.busCtx.TaskState.LastError = ""
			hint := strings.TrimSpace(o.busCtx.Mutable.PlanningHint())
			if hint != "" {
				hint += "\n\n"
			}
			hint += "The previous planning round ended without a stored change plan. Emit exactly one bounded repair plan via emit_change_plan that addresses the verification failure section above; do not finish the round without the emit."
			if rejection := lastPlanEmitRejectionSummary(o.busCtx.Mutable.DispatchToolResults()); rejection != "" {
				hint += "\n\nThe previous round's plan emit was rejected by validation with this exact reason — correct it in the re-emit:\n" + rejection
			}
			o.busCtx.Mutable.SetPlanningHint(hint)
			logging.Warning("[orchestrator] controller replan round produced no plan; granting one re-dispatch with typed failure context")
			continue
		}
		if err != nil {
			return err
		}
		return errors.New("write controller plan_batch produced no ChangePlan")
	}
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
	o.busCtx.Mutable.SetChangePlan(plan)
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
	if batch.Goal != "" {
		fmt.Fprintf(&b, "Plan only this bounded batch: %s\n\n", batch.Goal)
	}
	if batch.Purpose != "" {
		fmt.Fprintf(&b, "Purpose: %s\n\n", batch.Purpose)
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
	req := types.WriteExplorationRequest{
		BatchID:              batchIDOrDefault(batch.ID, "batch-1"),
		Goal:                 batch.Goal,
		CandidatePaths:       append([]string(nil), batch.ExpectedPaths...),
		EvidenceRequirements: append([]string(nil), batch.SuccessCriteria...),
	}
	o.busCtx.Mutable.SetWriteExplorationRequest(&req)
}

func (o *Orchestrator) syncCurrentWriteContextPackToRun(run *types.WriteWorkflowRun) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return
	}
	pack := o.busCtx.Mutable.WriteContextPack()
	if pack == nil || len(pack.Items) == 0 {
		return
	}
	if strings.TrimSpace(run.ActiveBatchID) != "" {
		pack.BatchID = strings.TrimSpace(run.ActiveBatchID)
	}
	*run = upsertWorkflowRunContextPack(*run, *pack)
	o.busCtx.Mutable.SetWriteWorkflowRun(run)
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
	o.busCtx.Mutable.SetWriteWorkflowRun(&normalized)
	if o.writeWorkflowRunStore == nil {
		return
	}
	if _, err := o.writeWorkflowRunStore.Save(&normalized); err != nil {
		logging.Warning("[orchestrator] write workflow run persist failed: %v", err)
	}
}

func upsertWorkflowRunContextPack(run types.WriteWorkflowRun, pack types.WriteContextPack) types.WriteWorkflowRun {
	pack = types.NormalizeWriteContextPack(pack)
	if len(pack.Items) == 0 {
		return run
	}
	key := workflowContextPackKey(pack)
	for i := range run.ContextPacks {
		if workflowContextPackKey(run.ContextPacks[i]) == key {
			run.ContextPacks[i] = pack
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
	pack := types.WriteContextPackFromChangePlan(plan)
	if strings.TrimSpace(run.ActiveBatchID) != "" {
		pack.BatchID = strings.TrimSpace(run.ActiveBatchID)
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

func updateWorkflowRunBatchPlan(run *types.WriteWorkflowRun, batchID, planID string) {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID == batchID {
			run.Batches[i].PlanID = strings.TrimSpace(planID)
			run.Batches[i].PlanRef = strings.TrimSpace(planID)
			run.Batches[i].UpdatedAt = time.Now()
			appendWorkflowBatchAttempt(&run.Batches[i], "plan", "complete", "", strings.TrimSpace(planID), "", strings.TrimSpace(planID))
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        batchID,
		PlanID:    strings.TrimSpace(planID),
		PlanRef:   strings.TrimSpace(planID),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: []types.WriteWorkflowAttempt{{
			ID:          fmt.Sprintf("attempt-%d", 1),
			Kind:        "plan",
			Status:      "complete",
			PlanID:      strings.TrimSpace(planID),
			ArtifactRef: strings.TrimSpace(planID),
			FinishedAt:  time.Now(),
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
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
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
	})
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
			appendWorkflowBatchAttempt(&run.Batches[i], "verify", status, reasonCode, planID, reportID, reportID)
			return
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        batchID,
		VerifyRef: reportID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: []types.WriteWorkflowAttempt{{
			ID:          fmt.Sprintf("attempt-%d", 1),
			Kind:        "verify",
			Status:      strings.TrimSpace(status),
			ReasonCode:  strings.TrimSpace(reasonCode),
			PlanID:      planID,
			ReportID:    reportID,
			ArtifactRef: reportID,
			FinishedAt:  time.Now(),
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

func writeWorkflowVerifyAttemptStatus(report *types.ChangeReport, err error) string {
	if err != nil {
		return "failed"
	}
	if report == nil {
		return "missing_report"
	}
	if !report.Passed {
		return "failed"
	}
	if len(report.NoTestsRunners) > 0 {
		return "unverified"
	}
	return "passed"
}

func writeWorkflowVerifyAttemptReason(report *types.ChangeReport, err error) string {
	if report == nil {
		return "missing_report"
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
	if len(report.NoTestsRunners) > 0 {
		return "no_tests"
	}
	return "tests_passed"
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
	if batch == nil {
		return
	}
	batch.Attempts = append(batch.Attempts, types.WriteWorkflowAttempt{
		ID:          fmt.Sprintf("attempt-%d", len(batch.Attempts)+1),
		Kind:        strings.TrimSpace(kind),
		Status:      strings.TrimSpace(status),
		ReasonCode:  strings.TrimSpace(reasonCode),
		PlanID:      strings.TrimSpace(planID),
		ReportID:    strings.TrimSpace(reportID),
		ArtifactRef: strings.TrimSpace(artifactRef),
		StartedAt:   time.Now(),
		FinishedAt:  time.Now(),
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

func writePlanNeedsManualApproval(plan *types.ChangePlan) bool {
	if plan == nil || plan.Approval == nil {
		return false
	}
	if plan.Approval.Action != string(writeflow.ApprovalActionManual) {
		return false
	}
	return !writeApprovalRecordAllowsManualApply(plan, types.PlanFingerprint(plan))
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
// the standard report path, and the attempt's applied-commit patch is written
// next to it as <stem>.attempt-N.diff. The diff artifact ref is attached to
// the batch's latest verify attempt record. Capture is best-effort and never
// alters cleanup behaviour (red line L5); failures log at warning.
func (o *Orchestrator) persistVerifyFailureEvidence(run *types.WriteWorkflowRun, report *types.ChangeReport, attempt int) string {
	if o == nil || o.busCtx == nil || run == nil {
		return ""
	}
	if report != nil {
		o.saveChangeReport(report)
	}
	if o.busCtx.WorktreePath == "" || strings.TrimSpace(o.currentIterCommitSHA) == "" {
		return ""
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
		return ""
	}
	patch, err := worktree.CaptureCommitPatch(o.busCtx.WorktreePath, o.currentIterCommitSHA)
	if err != nil {
		logging.Warning("[orchestrator] verify-failure diff capture failed: %v", err)
		return ""
	}
	if attempt < 1 {
		attempt = 1
	}
	diffName := fmt.Sprintf("%s.attempt-%d.diff", stem, attempt)
	diffPath := filepath.Join(planDir, diffName)
	if err := os.WriteFile(diffPath, []byte(patch), 0o644); err != nil {
		logging.Warning("[orchestrator] verify-failure diff save failed: %v", err)
		return ""
	}
	logging.Info("[orchestrator] verify-failure attempt diff saved: %s", diffPath)
	attachDiffArtifactToLatestVerifyAttempt(run, run.ActiveBatchID, diffName)
	return diffName
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
	logging.Info("[orchestrator] budget exhausted with applied-but-unverified batch %s — running completion verify", run.ActiveBatchID)
	appendControllerProgress(run, run.ActiveBatchID, "budget_completion_verify", "running final verification for the applied batch before the budget verdict")
	innerErr := o.runControllerVerifyBatch(stepsUsed)
	report := o.busCtx.Mutable.ChangeReport()
	if report != nil {
		o.ensureChangeReportPlanID(report)
		o.syncMutablePlanStatusAfterVerify(report, innerErr)
		o.mirrorActivePlanToImportFile(importedPlanMirror)
		o.busCtx.Mutable.MergeWriteContextPack(types.WriteContextPackFromChangeReport(report))
		updateWorkflowRunBatchVerify(run, run.ActiveBatchID, report, writeWorkflowVerifyAttemptStatus(report, innerErr), writeWorkflowVerifyAttemptReason(report, innerErr))
		o.syncCurrentWriteContextPackToRun(run)
	}
	if innerErr == nil && report != nil && report.Passed {
		updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchComplete)
		appendControllerProgress(run, run.ActiveBatchID, "batch_verified", "")
		o.busCtx.Mutable.ResetVerifyFailureHandoff()
		run.Status = types.WriteWorkflowRunComplete
		o.persistWriteWorkflowRun(run)
		if strings.TrimSpace(o.busCtx.Mutable.Result()) == "" {
			o.busCtx.Mutable.SetResult("write workflow complete")
		}
		return true
	}
	if innerErr == nil && report != nil {
		innerErr = fmt.Errorf("verify failed: %s", strings.TrimSpace(report.FailureSummary))
	}
	if innerErr != nil {
		appendControllerProgress(run, run.ActiveBatchID, "verify_failed", innerErr.Error())
	}
	o.persistVerifyFailureEvidence(run, report, len(active.Attempts))
	o.persistWriteWorkflowRun(run)
	return false
}

// lastPlanEmitRejectionSummary returns the most recent failed plan-emit
// tool result summary (system-generated validation text, bounded), so a
// no-plan re-dispatch can cite the exact typed rejection instead of a
// generic "emit something" nudge. Reads tool-result records only — never
// model prose.
func lastPlanEmitRejectionSummary(results []types.ToolResult) string {
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
			return ""
		}
		summary := strings.TrimSpace(r.Summary)
		if summary == "" {
			return ""
		}
		const maxLen = 600
		if len(summary) > maxLen {
			summary = summary[:maxLen] + "…"
		}
		return summary
	}
	return ""
}
