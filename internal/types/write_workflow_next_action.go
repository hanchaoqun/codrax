package types

import "strings"

type WriteWorkflowNextActionState string

const (
	WriteWorkflowNextNoActive      WriteWorkflowNextActionState = "no_active_batch"
	WriteWorkflowNextRunning       WriteWorkflowNextActionState = "running"
	WriteWorkflowNextPlanReady     WriteWorkflowNextActionState = "plan_ready"
	WriteWorkflowNextNeedsApproval WriteWorkflowNextActionState = "needs_approval"
	WriteWorkflowNextComplete      WriteWorkflowNextActionState = "complete"
	WriteWorkflowNextBlocked       WriteWorkflowNextActionState = "blocked"
)

type WriteWorkflowNextActionID string

const (
	WriteWorkflowNextActionWait            WriteWorkflowNextActionID = "wait"
	WriteWorkflowNextActionReviewPlan      WriteWorkflowNextActionID = "review_plan"
	WriteWorkflowNextActionApproveBatch    WriteWorkflowNextActionID = "approve_batch"
	WriteWorkflowNextActionRejectBatch     WriteWorkflowNextActionID = "reject_batch"
	WriteWorkflowNextActionInspectWorkflow WriteWorkflowNextActionID = "inspect_workflow"
	WriteWorkflowNextActionInspectEvidence WriteWorkflowNextActionID = "inspect_evidence"
	WriteWorkflowNextActionMerge           WriteWorkflowNextActionID = "merge"
	WriteWorkflowNextActionRunVerify       WriteWorkflowNextActionID = "run_verify"
	WriteWorkflowNextActionStartWriteGoal  WriteWorkflowNextActionID = "start_write_goal"
)

// WriteWorkflowNextActionView is the typed, UI-agnostic state card for the
// write Auto Pilot. Renderers may turn these action IDs into text, buttons, or
// slash-command hints, but scheduler/safety logic must continue consuming the
// underlying workflow, plan, approval, and report artifacts directly.
type WriteWorkflowNextActionView struct {
	State                WriteWorkflowNextActionState   `json:"state,omitempty"`
	RequiresUser         bool                           `json:"requires_user,omitempty"`
	RunID                string                         `json:"run_id,omitempty"`
	BatchID              string                         `json:"batch_id,omitempty"`
	BatchStatus          WriteWorkflowBatchStatus       `json:"batch_status,omitempty"`
	CompletionVerdict    WriteWorkflowCompletionVerdict `json:"completion_verdict,omitempty"`
	CompletionReasonCode string                         `json:"completion_reason_code,omitempty"`
	PlanID               string                         `json:"plan_id,omitempty"`
	PrimaryAction        WriteWorkflowNextActionID      `json:"primary_action,omitempty"`
	SecondaryActions     []WriteWorkflowNextActionID    `json:"secondary_actions,omitempty"`
	AdvancedActions      []WriteWorkflowNextActionID    `json:"advanced_actions,omitempty"`
}

func DeriveWriteWorkflowNextActionView(run WriteWorkflowRun) WriteWorkflowNextActionView {
	run = NormalizeWriteWorkflowRun(run)
	view := WriteWorkflowNextActionView{
		RunID: strings.TrimSpace(run.RunID),
	}
	batch, ok := writeWorkflowActiveBatch(run)
	if !ok {
		view.State = WriteWorkflowNextNoActive
		view.PrimaryAction = WriteWorkflowNextActionStartWriteGoal
		view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
		return view
	}
	view.BatchID = strings.TrimSpace(batch.ID)
	view.BatchStatus = batch.Status
	if batch.Completion != nil {
		view.CompletionVerdict = batch.Completion.Verdict
		view.CompletionReasonCode = strings.TrimSpace(batch.Completion.ReasonCode)
	}
	view.PlanID = strings.TrimSpace(batch.PlanID)
	switch batch.Status {
	case WriteWorkflowBatchPendingApproval:
		view.State = WriteWorkflowNextNeedsApproval
		view.RequiresUser = true
		view.PrimaryAction = WriteWorkflowNextActionApproveBatch
		view.SecondaryActions = []WriteWorkflowNextActionID{
			WriteWorkflowNextActionRejectBatch,
			WriteWorkflowNextActionReviewPlan,
		}
		view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
	case WriteWorkflowBatchPlanned:
		if view.PlanID == "" {
			view.State = WriteWorkflowNextRunning
			view.PrimaryAction = WriteWorkflowNextActionWait
			view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
			return view
		}
		if run.Status == WriteWorkflowRunInProgress && !writeWorkflowRunHasProgressReason(run, "plan_mode_complete") {
			view.State = WriteWorkflowNextRunning
			view.PrimaryAction = WriteWorkflowNextActionWait
			view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
			return view
		}
		view.State = WriteWorkflowNextPlanReady
		view.RequiresUser = true
		view.PrimaryAction = WriteWorkflowNextActionReviewPlan
		view.SecondaryActions = []WriteWorkflowNextActionID{
			WriteWorkflowNextActionApproveBatch,
			WriteWorkflowNextActionRejectBatch,
		}
		view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
	case WriteWorkflowBatchNeedsExploration, WriteWorkflowBatchReadyToPlan,
		WriteWorkflowBatchApplying, WriteWorkflowBatchVerifying:
		view.State = WriteWorkflowNextRunning
		view.PrimaryAction = WriteWorkflowNextActionWait
		view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
	case WriteWorkflowBatchComplete:
		view.State = WriteWorkflowNextComplete
		view.PrimaryAction = WriteWorkflowNextActionMerge
		view.SecondaryActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionRunVerify}
		view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
	case WriteWorkflowBatchBlocked:
		view.State = WriteWorkflowNextBlocked
		view.RequiresUser = true
		view.PrimaryAction = WriteWorkflowNextActionInspectEvidence
		view.SecondaryActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionStartWriteGoal}
		view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
	default:
		view.State = WriteWorkflowNextRunning
		view.PrimaryAction = WriteWorkflowNextActionWait
		view.AdvancedActions = []WriteWorkflowNextActionID{WriteWorkflowNextActionInspectWorkflow}
	}
	return view
}

func writeWorkflowRunHasProgressReason(run WriteWorkflowRun, reasonCode string) bool {
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		return false
	}
	for _, item := range run.ProgressLedger {
		if strings.TrimSpace(item.ReasonCode) == reasonCode {
			return true
		}
	}
	return false
}

func writeWorkflowActiveBatch(run WriteWorkflowRun) (WriteWorkflowBatch, bool) {
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" && len(run.Batches) > 0 {
		return run.Batches[0], true
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == activeID {
			return batch, true
		}
	}
	return WriteWorkflowBatch{}, false
}
