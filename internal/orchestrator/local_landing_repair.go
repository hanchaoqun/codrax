package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) consumeExploreRepairsAndLocalLandingPolicy(
	blocked []nodeBlock,
	validationTargets []string,
	pendingViolation string,
	pendingStageRetry string,
) ([]types.RepairDirective, types.ReadDispatchPolicy, string, bool) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil, types.ReadDispatchPolicy{}, "", false
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	if o.acceptedClosureAllowsAdvisoryDebt() {
		if cleared := closure.ClearPendingReadsByDebtClass(types.RepairDebtAdvisory); cleared > 0 {
			logging.Info("[orchestrator] accepted closure dropped %d advisory pending read(s) before retry-hint render", cleared)
			o.emitAcceptedClosureAdvisoryDebtSkippedNotice()
		}
	}
	pendingReads := closure.PendingReads()
	completionCaveats := closure.CompletionCaveats()
	repairs := closure.ConsumeRepairs()
	var dropped int
	repairs, dropped = filterConvergedDowngradeLaneRepairs(repairs, completionCaveats)
	if dropped > 0 {
		logging.Info("[orchestrator] dropped %d repair directive(s) after typed downgrade convergence", dropped)
	}
	policy, hint, active := localLandingRepairDispatchPolicy(repairs, pendingReads, blocked, validationTargets, pendingViolation, pendingStageRetry, completionCaveats)
	return repairs, policy, hint, active
}

func localLandingRepairDispatchPolicy(
	repairs []types.RepairDirective,
	pendingReads []types.PendingRead,
	blocked []nodeBlock,
	validationTargets []string,
	pendingViolation string,
	pendingStageRetry string,
	completionCaveats []types.CompletionCaveat,
) (types.ReadDispatchPolicy, string, bool) {
	if len(validationTargets) > 0 ||
		strings.TrimSpace(pendingViolation) != "" ||
		strings.TrimSpace(pendingStageRetry) != "" ||
		len(blocked) > 0 {
		return types.ReadDispatchPolicy{}, "", false
	}
	for _, pending := range pendingReads {
		if types.PendingReadBlocksAcceptedClosure(pending) {
			return types.ReadDispatchPolicy{}, "", false
		}
	}
	if hasDowngradeLaneCaveat(completionCaveats, types.DowngradeLaneCompletionForm) {
		return types.ReadDispatchPolicy{}, "", false
	}
	var landingRepairs []types.RepairDirective
	for _, repair := range repairs {
		if types.RepairDirectiveIsCompletionFormDebt(repair) {
			landingRepairs = append(landingRepairs, repair)
			continue
		}
		if types.ClassifyRepairDirective(repair) == types.RepairDebtAdvisory {
			continue
		}
		return types.ReadDispatchPolicy{}, "", false
	}
	if len(landingRepairs) == 0 {
		return types.ReadDispatchPolicy{}, "", false
	}
	reasons := make([]string, 0, len(landingRepairs))
	seen := map[string]bool{}
	for _, repair := range landingRepairs {
		reason := strings.TrimPrefix(strings.TrimSpace(repair.Origin), types.RepairOriginCompletionFormPrefix)
		if reason == "" {
			reason = strings.TrimSpace(repair.Subject)
		}
		if reason == "" {
			reason = "completion_form"
		}
		if seen[reason] {
			continue
		}
		seen[reason] = true
		reasons = append(reasons, reason)
	}
	reasonCode := "completion_form"
	if len(reasons) > 0 {
		reasonCode = strings.Join(reasons, "+")
	}
	policy := types.NormalizeReadDispatchPolicy(types.ReadDispatchPolicy{
		Active:         true,
		Action:         types.ReadDispatchPolicyActionLandingRepair,
		ReasonCode:     reasonCode,
		RouteSurface:   types.ReadDispatchPolicySurfaceHandoff,
		AllowedTools:   []string{"emit_investigation_complete"},
		DeniedTools:    []string{"repo_map", "grep", "read_file", "list_files", "exec_command", "trace_query", "run_tests", "emit_evidence"},
		PreferredTools: []string{"emit_investigation_complete"},
		MaxToolCalls:   2,
		OneShot:        true,
	})
	return policy, renderLocalLandingRepairDirective(policy), true
}

func filterConvergedDowngradeLaneRepairs(repairs []types.RepairDirective, caveats []types.CompletionCaveat) ([]types.RepairDirective, int) {
	if len(repairs) == 0 || len(caveats) == 0 {
		return repairs, 0
	}
	out := make([]types.RepairDirective, 0, len(repairs))
	dropped := 0
	for _, repair := range repairs {
		lane := repairDowngradeLane(repair)
		if lane != "" && hasDowngradeLaneCaveat(caveats, lane) {
			dropped++
			continue
		}
		out = append(out, repair)
	}
	return out, dropped
}

func repairDowngradeLane(repair types.RepairDirective) types.DowngradeLane {
	if repair.DowngradeLane != "" {
		return repair.DowngradeLane
	}
	if types.RepairDirectiveIsCompletionFormDebt(repair) {
		return types.DowngradeLaneCompletionForm
	}
	return ""
}

func hasDowngradeLaneCaveat(caveats []types.CompletionCaveat, lane types.DowngradeLane) bool {
	if lane == "" {
		return false
	}
	for _, caveat := range caveats {
		if caveat.Lane == lane {
			return true
		}
	}
	return false
}

func renderLocalLandingRepairDirective(policy types.ReadDispatchPolicy) string {
	policy = types.NormalizeReadDispatchPolicy(policy)
	if !policy.Active || policy.Action != types.ReadDispatchPolicyActionLandingRepair {
		return ""
	}
	var b strings.Builder
	b.WriteString("Local landing repair:\n")
	b.WriteString("source=structured_repair_lane reason=")
	b.WriteString(firstNonEmptyRetryString(policy.ReasonCode, "completion_form"))
	b.WriteString(".\n")
	b.WriteString("The model already reached the answer surface; this retry is only for structured handoff landing. Do not reopen evidence collection or repository discovery, do not call emit_evidence/repo_map/grep/read_file/list_files/trace_query/run_tests/exec_command, and do not reinterpret the user question. Re-emit `emit_investigation_complete` using the existing evidence/context pack. If the form debt remains after this bounded attempt, preserve the caveat instead of widening exploration.")
	return b.String()
}
