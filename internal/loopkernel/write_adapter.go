package loopkernel

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
)

const WriteWorkflowAdapterSource = "write_workflow_run"

func EventsFromWriteWorkflowRun(run types.WriteWorkflowRun) []LoopEvent {
	run = types.NormalizeWriteWorkflowRun(run)
	if strings.TrimSpace(run.RunID) == "" {
		return nil
	}
	var events []LoopEvent
	seq := int64(0)
	add := func(kind LoopEventKind, unitID, reason string, payload any) {
		seq++
		events = append(events, LoopEvent{
			ID:         fmt.Sprintf("%s:%06d:%s", run.RunID, seq, kind),
			RunID:      run.RunID,
			UnitID:     strings.TrimSpace(unitID),
			Sequence:   seq,
			Kind:       kind,
			Source:     WriteWorkflowAdapterSource,
			ReasonCode: strings.TrimSpace(reason),
			Payload:    EventPayload(payload),
			At:         writeWorkflowRunEventTime(run),
		})
	}
	add(LoopEventRunSeeded, "", "write_workflow_run_seeded", nil)
	if batch, ok := activeWriteWorkflowBatch(run); ok {
		unitID := writeWorkflowRuntimeUnitID(batch)
		add(LoopEventUnitCreated, unitID, "write_workflow_active_batch", nil)
		addWriteWorkflowBatchEvents(add, batch, unitID)
	}
	switch run.Status {
	case types.WriteWorkflowRunComplete:
		add(LoopEventRunCompleted, "", writeWorkflowCompletionReason(run.Completion, "write_workflow_complete"), nil)
	case types.WriteWorkflowRunBlocked:
		add(LoopEventRunBlocked, "", "write_workflow_blocked", nil)
	}
	return NormalizeLoopEvents(events)
}

func addWriteWorkflowBatchEvents(add func(LoopEventKind, string, string, any), batch types.WriteWorkflowBatch, unitID string) {
	if strings.TrimSpace(batch.PlanID) != "" {
		add(LoopEventPlanEmitted, unitID, "write_workflow_plan_available", nil)
	}
	switch batch.Status {
	case types.WriteWorkflowBatchNeedsExploration:
		add(LoopEventContextRequested, unitID, "batch_needs_exploration", nil)
	case types.WriteWorkflowBatchReadyToPlan:
		add(LoopEventContextObserved, unitID, "batch_ready_to_plan", nil)
		add(LoopEventPlanRequested, unitID, "batch_ready_to_plan", nil)
	case types.WriteWorkflowBatchPlanned:
		add(LoopEventEffectDescribed, unitID, "batch_planned", nil)
	case types.WriteWorkflowBatchPendingApproval:
		permission := DerivePermissionAuthority(WriteWorkflowAdapterSource,
			safety.AskPermission(WriteWorkflowAdapterSource, "manual_approval_required", "active write batch requires approval"),
		)
		add(LoopEventPermissionDecided, unitID, permission.ReasonCode, permission)
		add(LoopEventApprovalRequested, unitID, "manual_approval_required", nil)
	case types.WriteWorkflowBatchApplying:
		add(LoopEventUnitApplyStarted, unitID, "batch_applying", nil)
	case types.WriteWorkflowBatchVerifying:
		add(LoopEventUnitApplyCompleted, unitID, "batch_applied_observation_required", nil)
		add(LoopEventObserveStarted, unitID, "batch_verifying", nil)
	case types.WriteWorkflowBatchComplete:
		add(LoopEventObserveCompleted, unitID, writeWorkflowCompletionReason(batch.Completion, "batch_complete"), nil)
		add(LoopEventProofProjected, unitID, writeWorkflowCompletionReason(batch.Completion, "batch_complete"), proofAuthorityFromWriteCompletion(batch.Completion))
		add(LoopEventUnitCompleted, unitID, writeWorkflowCompletionReason(batch.Completion, "batch_complete"), nil)
	case types.WriteWorkflowBatchBlocked:
		add(LoopEventUnitBlocked, unitID, "batch_blocked", nil)
	}
}

func activeWriteWorkflowBatch(run types.WriteWorkflowRun) (types.WriteWorkflowBatch, bool) {
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

func writeWorkflowRuntimeUnitID(batch types.WriteWorkflowBatch) string {
	if id := strings.TrimSpace(batch.ActiveSliceID); id != "" {
		return id
	}
	for _, slice := range batch.Slices {
		if !writeWorkflowSliceTerminal(slice.Status) {
			if id := strings.TrimSpace(slice.ID); id != "" {
				return id
			}
		}
	}
	for _, slice := range batch.Slices {
		if id := strings.TrimSpace(slice.ID); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(batch.ID); id != "" {
		return "batch:" + id
	}
	return "batch"
}

func writeWorkflowSliceTerminal(status types.ChangePlanSliceStatus) bool {
	switch status {
	case types.ChangePlanSliceVerified, types.ChangePlanSliceUnverified,
		types.ChangePlanSliceFailed, types.ChangePlanSliceBlocked:
		return true
	default:
		return false
	}
}

func proofAuthorityFromWriteCompletion(completion *types.WriteWorkflowCompletion) ProofCoverageAuthorityView {
	if completion == nil {
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageMissing, "proof_missing", LoopActionVerify)
	}
	switch completion.Verdict {
	case types.WriteWorkflowCompletionVerified:
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageCovered, writeWorkflowCompletionReason(completion, "proof_covered"), LoopActionContinue)
	case types.WriteWorkflowCompletionUnverified:
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageUnavailable, writeWorkflowCompletionReason(completion, "proof_unavailable"), LoopActionContinue)
	case types.WriteWorkflowCompletionAcceptedFailed:
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageFailed, writeWorkflowCompletionReason(completion, "proof_failed"), LoopActionRepair)
	default:
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageMissing, "proof_missing", LoopActionVerify)
	}
}

func writeWorkflowCompletionReason(completion *types.WriteWorkflowCompletion, fallback string) string {
	if completion != nil && strings.TrimSpace(completion.ReasonCode) != "" {
		return strings.TrimSpace(completion.ReasonCode)
	}
	return fallback
}

func writeWorkflowRunEventTime(run types.WriteWorkflowRun) time.Time {
	if !run.UpdatedAt.IsZero() {
		return run.UpdatedAt
	}
	if !run.CreatedAt.IsZero() {
		return run.CreatedAt
	}
	return time.Time{}
}
