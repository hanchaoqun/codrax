package operation

import (
	"path/filepath"
	"strings"
)

type CommandApprovalPhase string

const (
	CommandApprovalInitial      CommandApprovalPhase = "initial"
	CommandApprovalReplan       CommandApprovalPhase = "replan"
	CommandApprovalContinuation CommandApprovalPhase = "continuation"
)

type CommandApprovalAction string

const (
	CommandApprovalNone        CommandApprovalAction = "none"
	CommandApprovalAutoExecute CommandApprovalAction = "auto_execute"
	CommandApprovalManual      CommandApprovalAction = "manual"
	CommandApprovalDeny        CommandApprovalAction = "deny"
)

type CommandApprovalOptions struct {
	Phase        CommandApprovalPhase
	PreviousPlan *CommandOperationPlan
}

type CommandApprovalDecision struct {
	Action       CommandApprovalAction
	ApprovalMode string
	ReasonCode   string
	Reason       string
}

func (d CommandApprovalDecision) AutoExecute() bool {
	return d.Action == CommandApprovalAutoExecute
}

func DecideCommandPlanApproval(policy CommandPolicy, plan CommandOperationPlan, opts CommandApprovalOptions) CommandApprovalDecision {
	policy = normalizeCommandPolicy(policy)
	phase := opts.Phase
	if phase == "" {
		phase = CommandApprovalInitial
	}
	if plan.Status == StatusBlocked || plan.ApprovalMode == ApprovalDenied {
		return CommandApprovalDecision{
			Action:       CommandApprovalDeny,
			ApprovalMode: ApprovalDenied,
			ReasonCode:   "plan_blocked",
			Reason:       "operation plan is blocked or denied",
		}
	}
	if plan.Status != StatusReady {
		return CommandApprovalDecision{
			Action:       CommandApprovalNone,
			ApprovalMode: "",
			ReasonCode:   "plan_not_ready",
			Reason:       "operation plan is not ready for approval",
		}
	}
	if len(plan.Steps) == 0 {
		return CommandApprovalDecision{
			Action:       CommandApprovalManual,
			ApprovalMode: ApprovalManual,
			ReasonCode:   "empty_plan",
			Reason:       "operation plan has no executable steps",
		}
	}
	for _, step := range plan.Steps {
		switch step.AutoApproval {
		case StepAutoDenied:
			return CommandApprovalDecision{
				Action:       CommandApprovalDeny,
				ApprovalMode: ApprovalDenied,
				ReasonCode:   "step_denied",
				Reason:       strings.TrimSpace(step.Reason),
			}
		case StepAutoEligible:
			continue
		default:
			return CommandApprovalDecision{
				Action:       CommandApprovalManual,
				ApprovalMode: ApprovalManual,
				ReasonCode:   "step_manual",
				Reason:       strings.TrimSpace(step.Reason),
			}
		}
	}
	if phase == CommandApprovalReplan && opts.PreviousPlan != nil {
		if !sameApprovalWorkDir(opts.PreviousPlan.WorkDir, plan.WorkDir) {
			return CommandApprovalDecision{
				Action:       CommandApprovalManual,
				ApprovalMode: ApprovalManual,
				ReasonCode:   "replan_workdir_changed",
				Reason:       "revised operation plan changes working directory",
			}
		}
		if commandApprovalRiskRank(plan.RiskLevel) > commandApprovalRiskRank(opts.PreviousPlan.RiskLevel) &&
			commandApprovalRiskRank(plan.RiskLevel) >= commandApprovalRiskRank("high") {
			return CommandApprovalDecision{
				Action:       CommandApprovalManual,
				ApprovalMode: ApprovalManual,
				ReasonCode:   "replan_risk_escalated",
				Reason:       "revised operation plan raises risk level to high",
			}
		}
		if step := firstManualReplanSideEffectStep(plan); step != "" {
			return CommandApprovalDecision{
				Action:       CommandApprovalManual,
				ApprovalMode: ApprovalManual,
				ReasonCode:   "replan_side_effect",
				Reason:       "revised operation plan introduces side effects that require approval: " + step,
			}
		}
	}
	if policy.AutoApprove || policy.AutoLowRisk {
		return CommandApprovalDecision{
			Action:       CommandApprovalAutoExecute,
			ApprovalMode: ApprovalAutoLowRisk,
			ReasonCode:   "all_steps_eligible",
			Reason:       "all operation steps are eligible for automatic execution under policy",
		}
	}
	return CommandApprovalDecision{
		Action:       CommandApprovalManual,
		ApprovalMode: ApprovalManual,
		ReasonCode:   "auto_disabled",
		Reason:       "operation auto approval is disabled by policy",
	}
}

func ApplyCommandPlanApprovalDecision(plan CommandOperationPlan, decision CommandApprovalDecision) CommandOperationPlan {
	plan.ApprovalReasonCode = strings.TrimSpace(decision.ReasonCode)
	plan.ApprovalReason = strings.TrimSpace(decision.Reason)
	switch decision.Action {
	case CommandApprovalAutoExecute:
		plan.ApprovalMode = ApprovalAutoLowRisk
	case CommandApprovalManual:
		if plan.Status == StatusReady {
			plan.ApprovalMode = ApprovalManual
		}
	case CommandApprovalDeny:
		plan.Status = StatusBlocked
		plan.ApprovalMode = ApprovalDenied
		if strings.TrimSpace(plan.BlockReason) == "" {
			plan.BlockReason = decision.Reason
		}
	case CommandApprovalNone:
		if plan.Status != StatusReady {
			plan.ApprovalMode = ""
		}
	}
	return plan
}

func firstManualReplanSideEffectStep(plan CommandOperationPlan) string {
	for _, step := range plan.Steps {
		for _, effect := range cleanList(step.SideEffects) {
			if commandReplanSideEffectRequiresManual(effect) {
				if strings.TrimSpace(step.ID) != "" {
					return step.ID + ":" + effect
				}
				return effect
			}
		}
	}
	return ""
}

func commandReplanSideEffectRequiresManual(effect string) bool {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "", "network_read", "local_file_read", "file_read", "directory_read", "environment_read", "process_read", "metadata_read":
		return false
	default:
		return true
	}
}

func sameApprovalWorkDir(a, b string) bool {
	return filepath.Clean(strings.TrimSpace(a)) == filepath.Clean(strings.TrimSpace(b))
}

func commandApprovalRiskRank(risk string) int {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "none":
		return 0
	case "low", "":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 2
	}
}
