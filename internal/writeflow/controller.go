package writeflow

import (
	"fmt"
	"strings"

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
	case ActionPlanChangeBatch:
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
	case ActionApplyReadyPlan:
		batchID := activeOrNextBatchID(run)
		updateWorkflowBatch(&run, batchID, "", types.WriteWorkflowBatchPendingApproval)
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeApply, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionVerify:
		batchID := activeOrNextBatchID(run)
		updateWorkflowBatch(&run, batchID, "", types.WriteWorkflowBatchVerifying)
		run.ActiveBatchID = batchID
		run.Status = types.WriteWorkflowRunInProgress
		appendWorkflowEdge(&run, types.WriteWorkflowEdgeVerify, "", batchID, decision.ReasonCode)
		appendWorkflowProgress(&run, batchID, "controller", string(decision.Action), decision.ReasonCode, decision.Reason)
	case ActionFinish:
		if run.ActiveBatchID != "" {
			updateWorkflowBatch(&run, run.ActiveBatchID, "", types.WriteWorkflowBatchComplete)
		}
		run.Status = types.WriteWorkflowRunComplete
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
		ID:     id,
		Goal:   strings.TrimSpace(goal),
		Status: status,
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
		}
		return
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{
		ID:     id,
		Goal:   strings.TrimSpace(goal),
		Status: status,
	})
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
	})
}
