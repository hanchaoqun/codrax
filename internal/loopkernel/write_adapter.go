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
		if review := LocalizationReviewFromWriteWorkflowRun(run, batch.ID); review != nil {
			authority := DeriveLocalizationAuthority(review)
			add(LoopEventLocalizationProjected, unitID, authority.ReasonCode, authority)
		}
		if proof, ok := proofAuthorityFromWriteWorkflowBatch(batch); ok {
			add(LoopEventProofProjected, unitID, proof.ReasonCode, proof)
		}
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
	if len(batch.Slices) > 0 {
		for _, slice := range writeWorkflowSlicesForEventOrder(batch, unitID) {
			addWriteWorkflowSliceEvents(add, batch, slice)
		}
		return
	}
	add(LoopEventUnitCreated, unitID, "write_workflow_active_batch", nil)
	if effect, ok := writeWorkflowRuntimeUnitEffect(batch, unitID); ok {
		payload := EffectEventPayload{
			Effect:      effect,
			Fingerprint: safety.EffectFingerprint(effect),
		}
		add(LoopEventEffectDescribed, unitID, "runtime_unit_effect_described", payload)
		permission := DerivePermissionAuthorityFromEffects(WriteWorkflowAdapterSource, effect)
		add(LoopEventPermissionDecided, unitID, permission.ReasonCode, permission)
	}
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
		if _, ok := writeWorkflowRuntimeUnitEffect(batch, unitID); !ok {
			add(LoopEventEffectDescribed, unitID, "batch_planned", nil)
		}
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
		authority := DeriveProofCoverageAuthorityFromCompletion(batch.Completion)
		add(LoopEventProofProjected, unitID, authority.ReasonCode, authority)
		add(LoopEventUnitCompleted, unitID, writeWorkflowCompletionReason(batch.Completion, "batch_complete"), nil)
	case types.WriteWorkflowBatchBlocked:
		add(LoopEventUnitBlocked, unitID, "batch_blocked", nil)
	}
}

func addWriteWorkflowSliceEvents(add func(LoopEventKind, string, string, any), batch types.WriteWorkflowBatch, slice types.WriteWorkflowSlice) {
	unitID := strings.TrimSpace(slice.ID)
	if unitID == "" {
		return
	}
	add(LoopEventUnitCreated, unitID, "write_workflow_slice", nil)
	if effect, ok := writeWorkflowRuntimeUnitEffect(batch, unitID); ok {
		payload := EffectEventPayload{
			Effect:      effect,
			Fingerprint: safety.EffectFingerprint(effect),
		}
		add(LoopEventEffectDescribed, unitID, "runtime_unit_effect_described", payload)
		permission := DerivePermissionAuthorityFromEffects(WriteWorkflowAdapterSource, effect)
		add(LoopEventPermissionDecided, unitID, permission.ReasonCode, permission)
	}
	if strings.TrimSpace(firstNonEmptyString(slice.PlanID, batch.PlanID)) != "" {
		add(LoopEventPlanEmitted, unitID, "write_workflow_plan_available", nil)
	}
	if slice.Checkpoint != nil && strings.TrimSpace(slice.Checkpoint.Ref) != "" {
		add(LoopEventCheckpointCreated, unitID, firstNonEmptyString(slice.Checkpoint.ReasonCode, "runtime_unit_checkpoint_created"), slice.Checkpoint)
	}
	if strings.TrimSpace(slice.ApplyRef) != "" {
		add(LoopEventPatchEffectRecorded, unitID, "runtime_unit_patch_effect_recorded", map[string]string{"apply_ref": strings.TrimSpace(slice.ApplyRef)})
	}
	switch slice.Status {
	case types.ChangePlanSliceApplying:
		add(LoopEventUnitApplyStarted, unitID, "slice_applying", nil)
	case types.ChangePlanSliceObserving:
		add(LoopEventUnitApplyCompleted, unitID, "slice_applied_observation_required", nil)
		add(LoopEventObserveStarted, unitID, "slice_observing", nil)
	case types.ChangePlanSliceVerified, types.ChangePlanSliceUnverified, types.ChangePlanSliceFailed:
		if strings.TrimSpace(slice.ApplyRef) != "" || slice.Checkpoint != nil {
			add(LoopEventUnitApplyCompleted, unitID, "slice_apply_completed", nil)
		}
		add(LoopEventObserveCompleted, unitID, writeWorkflowSliceCompletionReason(slice), nil)
		if proof, ok := proofAuthorityFromWriteWorkflowSlice(slice); ok {
			add(LoopEventProofProjected, unitID, proof.ReasonCode, proof)
		}
		if truth, ok := truthLedgerFromWriteWorkflowSlice(slice); ok {
			add(LoopEventTruthProjected, unitID, truth.ReasonCode, truth)
		}
		if slice.Status == types.ChangePlanSliceFailed {
			add(LoopEventRepairRequested, unitID, writeWorkflowSliceCompletionReason(slice), nil)
		} else {
			add(LoopEventUnitCompleted, unitID, writeWorkflowSliceCompletionReason(slice), nil)
		}
	case types.ChangePlanSliceBlocked:
		if truth, ok := truthLedgerFromWriteWorkflowSlice(slice); ok {
			add(LoopEventTruthProjected, unitID, truth.ReasonCode, truth)
		}
		add(LoopEventUnitBlocked, unitID, writeWorkflowSliceCompletionReason(slice), nil)
	case types.ChangePlanSlicePending, "":
	default:
	}
}

func writeWorkflowSlicesForEventOrder(batch types.WriteWorkflowBatch, activeUnitID string) []types.WriteWorkflowSlice {
	activeUnitID = strings.TrimSpace(activeUnitID)
	out := make([]types.WriteWorkflowSlice, 0, len(batch.Slices))
	var active *types.WriteWorkflowSlice
	for i := range batch.Slices {
		slice := batch.Slices[i]
		if activeUnitID != "" && strings.TrimSpace(slice.ID) == activeUnitID {
			cp := slice
			active = &cp
			continue
		}
		out = append(out, slice)
	}
	if active != nil {
		out = append(out, *active)
	}
	return out
}

func writeWorkflowRuntimeUnitEffect(batch types.WriteWorkflowBatch, unitID string) (safety.EffectDescriptor, bool) {
	if !writeWorkflowBatchHasRuntimeEffect(batch) {
		return safety.EffectDescriptor{}, false
	}
	effect := safety.EffectDescriptor{
		Role:            safety.EffectRoleCoder,
		Kind:            safety.EffectKindApplyPatch,
		Tool:            "apply_patch",
		Stage:           "write_runtime_unit",
		Mode:            "write",
		Paths:           writeWorkflowRuntimeUnitEffectPaths(batch, unitID),
		MutatesWorktree: true,
		Source:          WriteWorkflowAdapterSource,
	}
	return safety.NormalizeEffectDescriptor(effect), true
}

func proofAuthorityFromWriteWorkflowSlice(slice types.WriteWorkflowSlice) (ProofCoverageAuthorityView, bool) {
	if latest := latestWriteWorkflowAttemptOfKind(slice.Attempts, "verify"); latest != nil {
		return DeriveProofCoverageAuthorityFromAttempt(latest), true
	}
	if slice.Completion != nil {
		return DeriveProofCoverageAuthorityFromCompletion(slice.Completion), true
	}
	switch slice.Status {
	case types.ChangePlanSliceVerified:
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageCovered, "slice_verified", LoopActionContinue), true
	case types.ChangePlanSliceUnverified:
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageUnavailable, "slice_unverified", LoopActionContinue), true
	case types.ChangePlanSliceFailed:
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageFailed, "slice_failed", LoopActionRepair), true
	default:
		return ProofCoverageAuthorityView{}, false
	}
}

func truthLedgerFromWriteWorkflowSlice(slice types.WriteWorkflowSlice) (types.TruthLedger, bool) {
	proof, ok := proofAuthorityFromWriteWorkflowSlice(slice)
	if !ok {
		return types.TruthLedger{}, false
	}
	reason := firstNonEmptyString(proof.ReasonCode, writeWorkflowSliceCompletionReason(slice))
	proof.ReasonCode = reason
	return TruthLedgerFromProofCoverageAuthority("write_workflow_slice", proof, slice.Paths, compactWriteWorkflowStrings(slice.VerifyRef, slice.ObserveRef, slice.ApplyRef))
}

func writeWorkflowSliceCompletionReason(slice types.WriteWorkflowSlice) string {
	if slice.Completion != nil && strings.TrimSpace(slice.Completion.ReasonCode) != "" {
		return strings.TrimSpace(slice.Completion.ReasonCode)
	}
	switch slice.Status {
	case types.ChangePlanSliceVerified:
		return "slice_verified"
	case types.ChangePlanSliceUnverified:
		return "slice_unverified"
	case types.ChangePlanSliceFailed:
		return "slice_failed"
	case types.ChangePlanSliceBlocked:
		return "slice_blocked"
	default:
		return "slice_status"
	}
}

func compactWriteWorkflowStrings(values ...string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func writeWorkflowBatchHasRuntimeEffect(batch types.WriteWorkflowBatch) bool {
	if strings.TrimSpace(batch.PlanID) == "" && len(batch.Slices) == 0 {
		return false
	}
	switch batch.Status {
	case types.WriteWorkflowBatchPlanned,
		types.WriteWorkflowBatchPendingApproval,
		types.WriteWorkflowBatchApplying,
		types.WriteWorkflowBatchVerifying,
		types.WriteWorkflowBatchComplete,
		types.WriteWorkflowBatchBlocked:
		return true
	default:
		return false
	}
}

func writeWorkflowRuntimeUnitEffectPaths(batch types.WriteWorkflowBatch, unitID string) []string {
	unitID = strings.TrimSpace(unitID)
	for _, slice := range batch.Slices {
		if strings.TrimSpace(slice.ID) == unitID {
			if len(slice.Paths) > 0 {
				return append([]string(nil), slice.Paths...)
			}
			break
		}
	}
	if len(batch.ExpectedPaths) > 0 {
		return append([]string(nil), batch.ExpectedPaths...)
	}
	for _, slice := range batch.Slices {
		if !writeWorkflowSliceTerminal(slice.Status) && len(slice.Paths) > 0 {
			return append([]string(nil), slice.Paths...)
		}
	}
	for _, slice := range batch.Slices {
		if len(slice.Paths) > 0 {
			return append([]string(nil), slice.Paths...)
		}
	}
	return nil
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

func proofAuthorityFromWriteWorkflowBatch(batch types.WriteWorkflowBatch) (ProofCoverageAuthorityView, bool) {
	if latest := latestWriteWorkflowAttemptOfKind(batch.Attempts, "verify"); latest != nil {
		return DeriveProofCoverageAuthorityFromAttempt(latest), true
	}
	switch batch.Status {
	case types.WriteWorkflowBatchVerifying:
		return proofAuthority(ProofCoverageAuthorityView{}, ProofCoverageMissing, "proof_missing", LoopActionVerify), true
	default:
		return ProofCoverageAuthorityView{}, false
	}
}

func latestWriteWorkflowAttemptOfKind(attempts []types.WriteWorkflowAttempt, kind string) *types.WriteWorkflowAttempt {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if strings.TrimSpace(attempts[i].Kind) == kind {
			return &attempts[i]
		}
	}
	return nil
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
