package operation

import "testing"

func TestDecideCommandPlanApprovalMatrix(t *testing.T) {
	basePlan := CommandOperationPlan{
		Status:    StatusReady,
		RiskLevel: "medium",
		WorkDir:   "/repo",
		Steps: []CommandStep{{
			ID:           "probe",
			Program:      "go",
			Args:         []string{"version"},
			RiskLevel:    "low",
			AutoApproval: StepAutoEligible,
		}},
	}
	lowRiskPrevious := commandApprovalPlanWithRisk(basePlan, "low")
	tests := []struct {
		name   string
		policy CommandPolicy
		plan   CommandOperationPlan
		opts   CommandApprovalOptions
		action CommandApprovalAction
		mode   string
		code   string
	}{
		{
			name:   "initial auto approve overrides low-risk-only off",
			policy: commandApprovalPolicy(true, false),
			plan:   basePlan,
			opts:   CommandApprovalOptions{Phase: CommandApprovalInitial},
			action: CommandApprovalAutoExecute,
			mode:   ApprovalAutoLowRisk,
			code:   "all_steps_eligible",
		},
		{
			name:   "auto disabled falls back to manual",
			policy: commandApprovalPolicy(false, false),
			plan:   basePlan,
			opts:   CommandApprovalOptions{Phase: CommandApprovalInitial},
			action: CommandApprovalManual,
			mode:   ApprovalManual,
			code:   "auto_disabled",
		},
		{
			name:   "manual step blocks plan auto",
			policy: commandApprovalPolicy(true, true),
			plan:   commandApprovalPlanWithStepApproval(basePlan, StepAutoManual),
			opts:   CommandApprovalOptions{Phase: CommandApprovalInitial},
			action: CommandApprovalManual,
			mode:   ApprovalManual,
			code:   "step_manual",
		},
		{
			name:   "denied step denies plan",
			policy: commandApprovalPolicy(true, true),
			plan:   commandApprovalPlanWithStepApproval(basePlan, StepAutoDenied),
			opts:   CommandApprovalOptions{Phase: CommandApprovalInitial},
			action: CommandApprovalDeny,
			mode:   ApprovalDenied,
			code:   "step_denied",
		},
		{
			name:   "replan allows observation network read",
			policy: commandApprovalPolicy(true, false),
			plan:   commandApprovalPlanWithSideEffects(basePlan, []string{"network_read"}),
			opts:   CommandApprovalOptions{Phase: CommandApprovalReplan, PreviousPlan: &basePlan},
			action: CommandApprovalAutoExecute,
			mode:   ApprovalAutoLowRisk,
			code:   "all_steps_eligible",
		},
		{
			name:   "replan local write requires manual",
			policy: commandApprovalPolicy(true, true),
			plan:   commandApprovalPlanWithSideEffects(basePlan, []string{"local_file_write"}),
			opts:   CommandApprovalOptions{Phase: CommandApprovalReplan, PreviousPlan: &basePlan},
			action: CommandApprovalManual,
			mode:   ApprovalManual,
			code:   "replan_side_effect",
		},
		{
			name:   "replan changed workdir requires manual",
			policy: commandApprovalPolicy(true, true),
			plan:   commandApprovalPlanWithWorkDir(basePlan, "/tmp"),
			opts:   CommandApprovalOptions{Phase: CommandApprovalReplan, PreviousPlan: &basePlan},
			action: CommandApprovalManual,
			mode:   ApprovalManual,
			code:   "replan_workdir_changed",
		},
		{
			name:   "replan risk escalation requires manual",
			policy: commandApprovalPolicy(true, true),
			plan:   commandApprovalPlanWithRisk(basePlan, "high"),
			opts:   CommandApprovalOptions{Phase: CommandApprovalReplan, PreviousPlan: &lowRiskPrevious},
			action: CommandApprovalManual,
			mode:   ApprovalManual,
			code:   "replan_risk_escalated",
		},
		{
			name:   "replan medium observation remains auto when all steps eligible",
			policy: commandApprovalPolicy(true, false),
			plan: CommandOperationPlan{
				Status:    StatusReady,
				RiskLevel: "medium",
				WorkDir:   "/repo",
				Steps: []CommandStep{
					{
						ID:           "api",
						Program:      "/usr/bin/curl",
						Args:         []string{"-s", "http://localhost:8000/api/tags"},
						RiskLevel:    "low",
						SideEffects:  []string{"network_read"},
						AutoApproval: StepAutoEligible,
					},
					{
						ID:           "process",
						Shell:        "ps aux | grep -i omlx | grep -v grep",
						RiskLevel:    "medium",
						AutoApproval: StepAutoEligible,
					},
				},
			},
			opts:   CommandApprovalOptions{Phase: CommandApprovalReplan, PreviousPlan: &lowRiskPrevious},
			action: CommandApprovalAutoExecute,
			mode:   ApprovalAutoLowRisk,
			code:   "all_steps_eligible",
		},
		{
			name:   "continuation auto executes eligible plan",
			policy: commandApprovalPolicy(true, false),
			plan:   basePlan,
			opts:   CommandApprovalOptions{Phase: CommandApprovalContinuation},
			action: CommandApprovalAutoExecute,
			mode:   ApprovalAutoLowRisk,
			code:   "all_steps_eligible",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideCommandPlanApproval(tt.policy, tt.plan, tt.opts)
			if got.Action != tt.action || got.ApprovalMode != tt.mode || got.ReasonCode != tt.code {
				t.Fatalf("decision=(%s,%s,%s), want (%s,%s,%s): %+v", got.Action, got.ApprovalMode, got.ReasonCode, tt.action, tt.mode, tt.code, got)
			}
			applied := ApplyCommandPlanApprovalDecision(tt.plan, got)
			if applied.ApprovalMode != tt.mode {
				t.Fatalf("applied ApprovalMode=%q, want %q", applied.ApprovalMode, tt.mode)
			}
			if tt.action == CommandApprovalDeny && applied.Status != StatusBlocked {
				t.Fatalf("denied plan Status=%q, want blocked", applied.Status)
			}
		})
	}
}

func commandApprovalPolicy(autoApprove, autoLowRisk bool) CommandPolicy {
	policy := DefaultCommandPolicy()
	policy.AutoApprove = autoApprove
	policy.AutoLowRisk = autoLowRisk
	return policy
}

func commandApprovalPlanWithStepApproval(plan CommandOperationPlan, approval string) CommandOperationPlan {
	plan.Steps = append([]CommandStep(nil), plan.Steps...)
	plan.Steps[0].AutoApproval = approval
	return plan
}

func commandApprovalPlanWithSideEffects(plan CommandOperationPlan, effects []string) CommandOperationPlan {
	plan.Steps = append([]CommandStep(nil), plan.Steps...)
	plan.Steps[0].SideEffects = append([]string(nil), effects...)
	return plan
}

func commandApprovalPlanWithWorkDir(plan CommandOperationPlan, workDir string) CommandOperationPlan {
	plan.WorkDir = workDir
	return plan
}

func commandApprovalPlanWithRisk(plan CommandOperationPlan, risk string) CommandOperationPlan {
	plan.RiskLevel = risk
	return plan
}
