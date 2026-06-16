package writeflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ApplyWorkflowDecisionToRun applies one validated controller action to the
// durable workflow run envelope. It is intentionally typed: routing switches on
// decision.Action only; model-authored reason text is recorded for audit but
// never interpreted.
func ApplyWorkflowDecisionToRun(run types.WriteWorkflowRun, decision WriteWorkflowDecision) (types.WriteWorkflowRun, error) {
	decision = NormalizeWriteWorkflowDecision(decision)
	if errs := ValidateWriteWorkflowDecision(decision); len(errs) > 0 {
		return run, fmt.Errorf("invalid write workflow decision: %s", strings.Join(errs, "; "))
	}
	run = types.NormalizeWriteWorkflowRun(run)
	if strings.TrimSpace(run.RunID) == "" {
		return run, fmt.Errorf("workflow run_id is required")
	}
	if run.Goal == "" && decision.Workflow.Goal != "" {
		run.Goal = decision.Workflow.Goal
	}
	if run.Status == "" {
		run.Status = types.WriteWorkflowRunInProgress
	}
	if run.Budget.MaxBatches == 0 && decision.Workflow.MaxBatches > 0 {
		run.Budget.MaxBatches = decision.Workflow.MaxBatches
	}

	switch decision.Action {
	case ActionExploreCode:
		batchID, goal := batchIDAndGoalFromExploration(decision, run)
		added := ensureWorkflowBatch(&run, batchID, goal, types.WriteWorkflowBatchNeedsExploration)
		if added {
			run.Budget.BatchesUsed++
		}
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		run.Budget.ExplorationRoundsUsed++
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeExplore, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionPlanBatch:
		batchID, goal := batchIDAndGoalFromBatch(decision, run)
		added := ensureWorkflowBatch(&run, batchID, goal, types.WriteWorkflowBatchReadyToPlan)
		if added {
			run.Budget.BatchesUsed++
		}
		updateWorkflowBatch(&run, batchID, goal, types.WriteWorkflowBatchReadyToPlan)
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		appendWorkflowEdge(&run, types.WriteWorkflowEdgePlan, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionApplyPlan:
		batchID := activeOrNextBatchID(run)
		updateWorkflowBatch(&run, batchID, "", types.WriteWorkflowBatchApplying)
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeApply, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionVerifyBatch:
		batchID := activeOrNextBatchID(run)
		updateWorkflowBatch(&run, batchID, "", types.WriteWorkflowBatchVerifying)
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeVerify, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionAppendBatch:
		batchID, goal := batchIDAndGoalFromBatch(decision, run)
		added := ensureWorkflowBatch(&run, batchID, goal, types.WriteWorkflowBatchReadyToPlan)
		if added {
			run.Budget.BatchesUsed++
		}
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeFollowup, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionSplitBatch:
		batchID, goal := batchIDAndGoalFromBatch(decision, run)
		added := ensureWorkflowBatch(&run, batchID, goal, types.WriteWorkflowBatchReadyToPlan)
		if added {
			run.Budget.BatchesUsed++
		}
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeSplit, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionReplanBatch:
		batchID, goal := batchIDAndGoalFromBatch(decision, run)
		updateWorkflowBatch(&run, batchID, goal, types.WriteWorkflowBatchReadyToPlan)
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		appendWorkflowEdge(&run, types.WriteWorkflowEdgePlan, batchID, batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionFinish:
		if blocked := FinishBlockedReason(run, decision); blocked != "" {
			return run, fmt.Errorf("finish rejected: %s", blocked)
		}
		if run.ActiveBatchID != "" {
			updateWorkflowBatch(&run, run.ActiveBatchID, "", types.WriteWorkflowBatchComplete)
			markWorkflowBatchCompletionFromLatestVerify(&run, run.ActiveBatchID, decision)
		}
		run.Status = types.WriteWorkflowRunComplete
		MarkWorkflowRunCompletionFromBatches(&run)
		appendWorkflowProgress(&run, run.ActiveBatchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionBlock:
		if run.ActiveBatchID != "" {
			updateWorkflowBatch(&run, run.ActiveBatchID, "", types.WriteWorkflowBatchBlocked)
		}
		run.Status = types.WriteWorkflowRunBlocked
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeBlocked, "", run.ActiveBatchID, decision.ReasonCode)
		appendWorkflowProgress(&run, run.ActiveBatchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionAskUser:
		batchID := activeOrNextBatchID(run)
		updateWorkflowBatch(&run, batchID, "", types.WriteWorkflowBatchBlocked)
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunBlocked
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeBlocked, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, strings.Join(decision.QuestionsForUser, " | "))
		if len(run.ProgressLedger) > 0 {
			run.ProgressLedger[len(run.ProgressLedger)-1].FactKeys = MissingUserFactKeys(decision.MissingFacts)
		}
	}
	return types.NormalizeWriteWorkflowRun(run), nil
}

func batchIDAndGoalFromExploration(decision WriteWorkflowDecision, run types.WriteWorkflowRun) (string, string) {
	if decision.ExplorationRequest != nil {
		id := strings.TrimSpace(decision.ExplorationRequest.BatchID)
		if id == "" {
			id = activeOrNextBatchID(run)
		}
		return id, strings.TrimSpace(decision.ExplorationRequest.Goal)
	}
	return activeOrNextBatchID(run), ""
}

func batchIDAndGoalFromBatch(decision WriteWorkflowDecision, run types.WriteWorkflowRun) (string, string) {
	if decision.Batch != nil {
		id := strings.TrimSpace(decision.Batch.ID)
		if id == "" {
			id = activeOrNextBatchID(run)
		}
		return id, strings.TrimSpace(decision.Batch.Goal)
	}
	return activeOrNextBatchID(run), ""
}

func activeOrNextBatchID(run types.WriteWorkflowRun) string {
	if strings.TrimSpace(run.ActiveBatchID) != "" {
		return strings.TrimSpace(run.ActiveBatchID)
	}
	return fmt.Sprintf("batch-%d", len(run.Batches)+1)
}

func ensureWorkflowBatch(run *types.WriteWorkflowRun, id, goal string, status types.WriteWorkflowBatchStatus) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		id = activeOrNextBatchID(*run)
	}
	for i := range run.Batches {
		if run.Batches[i].ID == id {
			updateWorkflowBatch(run, id, goal, status)
			return false
		}
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        id,
		Goal:      strings.TrimSpace(goal),
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	return true
}

func updateWorkflowBatch(run *types.WriteWorkflowRun, id, goal string, status types.WriteWorkflowBatchStatus) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID != id {
			continue
		}
		if strings.TrimSpace(goal) != "" {
			run.Batches[i].Goal = strings.TrimSpace(goal)
		}
		if status != "" {
			run.Batches[i].Status = status
			if status != types.WriteWorkflowBatchComplete {
				run.Batches[i].Completion = nil
			}
		}
		if run.Batches[i].CreatedAt.IsZero() {
			run.Batches[i].CreatedAt = time.Now()
		}
		run.Batches[i].UpdatedAt = time.Now()
		return
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:        id,
		Goal:      strings.TrimSpace(goal),
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
}

func markWorkflowBatchCompletionFromLatestVerify(run *types.WriteWorkflowRun, batchID string, decision WriteWorkflowDecision) {
	if run == nil || strings.TrimSpace(batchID) == "" {
		return
	}
	for i := range run.Batches {
		if strings.TrimSpace(run.Batches[i].ID) != strings.TrimSpace(batchID) {
			continue
		}
		completion := completionFromBatchAttempts(run.Batches[i], decision, "finish")
		if completion.Verdict == "" {
			run.Batches[i].Completion = nil
		} else {
			run.Batches[i].Completion = &completion
		}
		run.Batches[i].UpdatedAt = time.Now()
		return
	}
}

func completionFromBatchAttempts(batch types.WriteWorkflowBatch, decision WriteWorkflowDecision, source string) types.WriteWorkflowCompletion {
	latestVerify := latestAttemptOfKind(batch, "verify")
	now := time.Now()
	if latestVerify == nil {
		if strings.TrimSpace(decision.FinishDisposition) == FinishDispositionAcceptUnverified {
			return types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionUnverified,
				ReasonCode: "accepted_without_local_verify",
				Source:     source,
				At:         now,
			}
		}
		return types.WriteWorkflowCompletion{}
	}
	reason := strings.TrimSpace(latestVerify.ReasonCode)
	if reason == "" {
		reason = strings.TrimSpace(latestVerify.Status)
	}
	switch strings.TrimSpace(latestVerify.Status) {
	case "passed":
		return types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionVerified,
			ReasonCode: reason,
			Source:     source,
			At:         now,
		}
	case "unverified":
		return types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionUnverified,
			ReasonCode: reason,
			Source:     source,
			At:         now,
		}
	case "failed":
		if strings.TrimSpace(decision.FinishDisposition) == FinishDispositionAcceptUnverified {
			if verifyFailureAllowsAcceptUnverified(latestVerify.Status, reason) {
				return types.WriteWorkflowCompletion{
					Verdict:    types.WriteWorkflowCompletionUnverified,
					ReasonCode: reason,
					Source:     source,
					At:         now,
				}
			}
			return types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionAcceptedFailed,
				ReasonCode: reason,
				Source:     source,
				At:         now,
			}
		}
	}
	return types.WriteWorkflowCompletion{}
}

// MarkWorkflowRunCompletionFromBatches aggregates batch terminal completion
// verdicts into the run-level completion verdict. It reads typed batch
// completion records and verify attempts only.
func MarkWorkflowRunCompletionFromBatches(run *types.WriteWorkflowRun) {
	if run == nil || run.Status != types.WriteWorkflowRunComplete {
		if run != nil {
			run.Completion = nil
		}
		return
	}
	now := time.Now()
	out := types.WriteWorkflowCompletion{
		Verdict:    types.WriteWorkflowCompletionVerified,
		ReasonCode: "all_batches_verified",
		Source:     "batch_completion_aggregate",
		At:         now,
	}
	sawComplete := false
	for i := range run.Batches {
		if run.Batches[i].Status != types.WriteWorkflowBatchComplete {
			continue
		}
		sawComplete = true
		if run.Batches[i].Completion == nil {
			derived := completionFromBatchAttempts(run.Batches[i], WriteWorkflowDecision{}, "batch_attempts")
			if derived.Verdict != "" {
				run.Batches[i].Completion = &derived
			}
		}
		if run.Batches[i].Completion == nil {
			out = types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionUnverified,
				ReasonCode: "missing_terminal_verify_verdict",
				Source:     "batch_completion_aggregate",
				At:         now,
			}
			continue
		}
		switch run.Batches[i].Completion.Verdict {
		case types.WriteWorkflowCompletionAcceptedFailed:
			run.Completion = &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionAcceptedFailed,
				ReasonCode: run.Batches[i].Completion.ReasonCode,
				Source:     "batch_completion_aggregate",
				At:         now,
			}
			return
		case types.WriteWorkflowCompletionUnverified:
			if out.Verdict != types.WriteWorkflowCompletionAcceptedFailed {
				out = types.WriteWorkflowCompletion{
					Verdict:    types.WriteWorkflowCompletionUnverified,
					ReasonCode: run.Batches[i].Completion.ReasonCode,
					Source:     "batch_completion_aggregate",
					At:         now,
				}
			}
		}
	}
	if !sawComplete {
		run.Completion = nil
		return
	}
	run.Completion = &out
}

func appendWorkflowEdge(run *types.WriteWorkflowRun, kind types.WriteWorkflowEdgeKind, from, to, reasonCode string) {
	if kind == "" {
		return
	}
	run.Edges = append(run.Edges, types.WriteWorkflowEdge{
		FromBatchID: strings.TrimSpace(from),
		ToBatchID:   strings.TrimSpace(to),
		Kind:        kind,
		ReasonCode:  strings.TrimSpace(reasonCode),
	})
}

func appendWorkflowProgress(run *types.WriteWorkflowRun, batchID, stage, status, reasonCode, message string) {
	run.ProgressLedger = append(run.ProgressLedger, types.WriteWorkflowProgress{
		BatchID:    strings.TrimSpace(batchID),
		Stage:      strings.TrimSpace(stage),
		Status:     strings.TrimSpace(status),
		ReasonCode: strings.TrimSpace(reasonCode),
		Message:    strings.TrimSpace(message),
		At:         time.Now(),
	})
}
