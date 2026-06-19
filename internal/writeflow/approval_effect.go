package writeflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
)

const ApprovalEffectSourceRuntimeUnit = "writeflow_runtime_unit_effect"

func ApprovalEffectBinding(plan *types.ChangePlan, run *types.WriteWorkflowRun) (safety.EffectDescriptor, string, []string, bool) {
	effect, ok := ApprovalEffectDescriptor(plan, run)
	if !ok {
		return safety.EffectDescriptor{}, "", nil, false
	}
	fingerprint := safety.EffectFingerprint(effect)
	scope := approvalEffectScope(effect, run, plan)
	return effect, fingerprint, scope, fingerprint != ""
}

func ApprovalEffectDescriptor(plan *types.ChangePlan, run *types.WriteWorkflowRun) (safety.EffectDescriptor, bool) {
	if plan == nil {
		return safety.EffectDescriptor{}, false
	}
	paths := approvalEffectPaths(plan, run)
	effect := safety.NormalizeEffectDescriptor(safety.EffectDescriptor{
		Role:            safety.EffectRoleCoder,
		Kind:            safety.EffectKindApplyPatch,
		Tool:            "apply_patch",
		Stage:           "write_runtime_unit",
		Mode:            "write",
		Paths:           paths,
		MutatesWorktree: true,
		Source:          ApprovalEffectSourceRuntimeUnit,
	})
	return effect, true
}

func BindApprovalRecordEffect(record *types.WriteApprovalRecord, plan *types.ChangePlan, run *types.WriteWorkflowRun) {
	if record == nil {
		return
	}
	_, fingerprint, scope, ok := ApprovalEffectBinding(plan, run)
	if !ok {
		return
	}
	types.StampApprovalRecordEffect(record, fingerprint, scope, ApprovalEffectSourceRuntimeUnit)
}

func ApprovalRecordEffectMatches(record *types.WriteApprovalRecord, plan *types.ChangePlan, run *types.WriteWorkflowRun) bool {
	if record == nil {
		return false
	}
	if strings.TrimSpace(record.EffectFingerprint) == "" && len(record.EffectScope) == 0 {
		return true
	}
	_, fingerprint, scope, ok := ApprovalEffectBinding(plan, run)
	if !ok || strings.TrimSpace(record.EffectFingerprint) == "" {
		return false
	}
	if strings.TrimSpace(record.EffectFingerprint) != fingerprint {
		return false
	}
	return strings.Join(types.NormalizeApprovalEffectScope(scope), "\x00") ==
		strings.Join(types.NormalizeApprovalEffectScope(record.EffectScope), "\x00")
}

func approvalEffectPaths(plan *types.ChangePlan, run *types.WriteWorkflowRun) []string {
	if plan == nil {
		return nil
	}
	if run != nil {
		if slice, ok := ActiveRuntimeUnitApplySlice(plan, run); ok && len(slice.Paths) > 0 {
			return append([]string(nil), slice.Paths...)
		}
	}
	if len(plan.TargetPaths) > 0 {
		return append([]string(nil), plan.TargetPaths...)
	}
	paths := make([]string, 0, len(plan.Changes))
	for _, change := range plan.Changes {
		paths = append(paths, change.Path)
	}
	return paths
}

func approvalEffectScope(effect safety.EffectDescriptor, run *types.WriteWorkflowRun, plan *types.ChangePlan) []string {
	var scope []string
	if run != nil {
		if batchID := strings.TrimSpace(run.ActiveBatchID); batchID != "" {
			scope = append(scope, "batch:"+batchID)
		}
		if unit, ok := ActiveRuntimeUnitView(types.ModeApply, *run, plan, nil); ok {
			if unitID := strings.TrimSpace(unit.UnitID); unitID != "" {
				scope = append(scope, "unit:"+unitID)
			}
		}
	}
	for _, path := range effect.Paths {
		if path = strings.TrimSpace(path); path != "" {
			scope = append(scope, "path:"+path)
		}
	}
	return types.NormalizeApprovalEffectScope(scope)
}
