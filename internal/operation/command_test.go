package operation

import "testing"

func TestBuildCommandOperationPlanNeedsClarificationWhenDetailsMissing(t *testing.T) {
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "move that file",
		ClarifyingQuestions: []ClarifyingQuestion{{
			Question:    "Which file should be moved?",
			Suggestions: []string{"provide a source path", "provide a destination path"},
		}},
	}, DefaultCommandPolicy())

	if plan.Status != StatusNeedsClarification {
		t.Fatalf("Status=%q, want needs_clarification", plan.Status)
	}
	if plan.ApprovalMode != "" {
		t.Fatalf("ApprovalMode=%q, want empty while clarifying", plan.ApprovalMode)
	}
	if len(plan.ClarifyingQuestions) != 1 || plan.ClarifyingQuestions[0].ID == "" {
		t.Fatalf("ClarifyingQuestions=%+v", plan.ClarifyingQuestions)
	}
}

func TestLintCommandOperationPlanRejectsInvalidCommandShapes(t *testing.T) {
	tests := []struct {
		name string
		plan CommandOperationPlan
		code string
	}{
		{
			name: "empty ready plan",
			plan: CommandOperationPlan{Status: StatusReady},
			code: PlanLintEmptyPlan,
		},
		{
			name: "empty step",
			plan: CommandOperationPlan{Status: StatusReady, Steps: []CommandStep{{ID: "s1"}}},
			code: PlanLintEmptyCommandStep,
		},
		{
			name: "bare grep shell",
			plan: CommandOperationPlan{Status: StatusReady, Steps: []CommandStep{{ID: "s1", Shell: "grep vpn"}}},
			code: PlanLintStdinWithoutInput,
		},
		{
			name: "bare cat shell",
			plan: CommandOperationPlan{Status: StatusReady, Steps: []CommandStep{{ID: "s1", Shell: "cat"}}},
			code: PlanLintStdinWithoutInput,
		},
		{
			name: "leading pipe shell",
			plan: CommandOperationPlan{Status: StatusReady, Steps: []CommandStep{{ID: "s1", Shell: "| grep vpn"}}},
			code: PlanLintInvalidShellShape,
		},
		{
			name: "leading or shell",
			plan: CommandOperationPlan{Status: StatusReady, Steps: []CommandStep{{ID: "s1", Shell: "|| grep vpn"}}},
			code: PlanLintInvalidShellShape,
		},
		{
			name: "leading and shell",
			plan: CommandOperationPlan{Status: StatusReady, Steps: []CommandStep{{ID: "s1", Shell: "&& echo ok"}}},
			code: PlanLintInvalidShellShape,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lint := LintCommandOperationPlan(tt.plan)
			if lint.OK() {
				t.Fatalf("lint OK, want issue %s", tt.code)
			}
			if lint.Issues[0].Code != tt.code {
				t.Fatalf("issue code=%q, want %q; issues=%+v", lint.Issues[0].Code, tt.code, lint.Issues)
			}
			if lint.Summary() == "" {
				t.Fatalf("summary empty for issues=%+v", lint.Issues)
			}
		})
	}
}

func TestLintCommandOperationPlanAllowsExplicitInputs(t *testing.T) {
	plan := CommandOperationPlan{
		Status: StatusReady,
		Steps: []CommandStep{{
			ID:    "grep-file",
			Shell: "grep vpn /tmp/processes.txt",
		}, {
			ID:    "pipeline",
			Shell: "ps aux | grep vpn",
		}, {
			ID:    "cat-file",
			Shell: "cat /tmp/config.txt",
		}},
	}
	if lint := LintCommandOperationPlan(plan); !lint.OK() {
		t.Fatalf("lint rejected explicit inputs: %+v", lint.Issues)
	}
}

func TestBuildCommandOperationPlanNeedsClarificationWhenReadyHasNoSteps(t *testing.T) {
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text:      "show disk size",
		RiskLevel: "low",
	}, DefaultCommandPolicy())

	if plan.Status != StatusNeedsClarification {
		t.Fatalf("Status=%q, want needs_clarification", plan.Status)
	}
	if plan.ApprovalMode != "" {
		t.Fatalf("ApprovalMode=%q, want empty while clarifying", plan.ApprovalMode)
	}
	if len(plan.ClarifyingQuestions) == 0 {
		t.Fatalf("expected clarification for empty command plan: %+v", plan)
	}
}

func TestBuildCommandOperationPlanLowRiskAutoEnabledByDefault(t *testing.T) {
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "show current directory",
		Steps: []CommandStep{{
			Program: "pwd",
		}},
	}, DefaultCommandPolicy())

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("ApprovalMode=%q, want auto_low_risk", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoEligible {
		t.Fatalf("step AutoApproval=%q, want eligible", got)
	}
}

func TestBuildCommandOperationPlanLowRiskAutoCanBeDisabled(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoApprove = false
	policy.AutoLowRisk = false
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "show current directory",
		Steps: []CommandStep{{
			Program: "pwd",
		}},
	}, policy)

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalManual {
		t.Fatalf("ApprovalMode=%q, want manual", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoEligible {
		t.Fatalf("step AutoApproval=%q, want eligible", got)
	}
}

func TestBuildCommandOperationPlanAutoApproveWorksWhenLowRiskOnlyDisabled(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoApprove = true
	policy.AutoLowRisk = false
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "download a user guide page",
		Steps: []CommandStep{{
			Program:     "curl",
			Args:        []string{"-s", "-L", "--max-time", "30", "http://codrax.net/user_guide.html"},
			RiskLevel:   "low",
			SideEffects: []string{"network_read"},
		}},
	}, policy)

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("ApprovalMode=%q, want auto_low_risk", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoEligible {
		t.Fatalf("step AutoApproval=%q, want eligible", got)
	}
}

func TestBuildCommandOperationPlanModelConfirmationDoesNotBlockProvenLowRisk(t *testing.T) {
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text:                 "show go version",
		RequiresConfirmation: true,
		Steps: []CommandStep{{
			Program: "go",
			Args:    []string{"version"},
		}},
	}, DefaultCommandPolicy())

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("ApprovalMode=%q, want auto_low_risk", plan.ApprovalMode)
	}
}

func TestBuildCommandOperationPlanSystemInfoCommandsAutoEligible(t *testing.T) {
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "show OS memory CPU GPU info",
		Steps: []CommandStep{{
			ID:      "os",
			Program: "sw_vers",
			Args:    []string{"-productName", "-productVersion", "-buildVersion"},
		}, {
			ID:      "cpu",
			Program: "sysctl",
			Args:    []string{"-n", "hw.ncpu"},
		}, {
			ID:      "mem",
			Program: "sysctl",
			Args:    []string{"-n", "hw.memsize"},
		}, {
			ID:      "gpu",
			Program: "system_profiler",
			Args:    []string{"SPDisplaysDataType", "-detailLevel", "mini"},
		}},
	}, DefaultCommandPolicy())

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q, reason=%s", plan.Status, plan.BlockReason)
	}
	if plan.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("ApprovalMode=%q, want auto_low_risk", plan.ApprovalMode)
	}
	for _, step := range plan.Steps {
		if step.AutoApproval != StepAutoEligible {
			t.Fatalf("step %s AutoApproval=%q, want eligible", step.ID, step.AutoApproval)
		}
	}
}

func TestBuildCommandOperationPlanSysctlWriteRequiresManual(t *testing.T) {
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "change sysctl",
		Steps: []CommandStep{{
			Program: "sysctl",
			Args:    []string{"-w", "kern.maxfiles=1024"},
		}},
	}, DefaultCommandPolicy())

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalManual {
		t.Fatalf("ApprovalMode=%q, want manual", plan.ApprovalMode)
	}
	if plan.RiskLevel != "high" {
		t.Fatalf("RiskLevel=%q, want high", plan.RiskLevel)
	}
}

func TestBuildCommandOperationPlanHighRiskSideEffectRequiresManual(t *testing.T) {
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "submit to external system",
		Steps: []CommandStep{{
			Program:     "corp-submit",
			Args:        []string{"--prod"},
			SideEffects: []string{"external_system_write"},
		}},
	}, DefaultCommandPolicy())

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalManual {
		t.Fatalf("ApprovalMode=%q, want manual", plan.ApprovalMode)
	}
	if plan.RiskLevel != "high" {
		t.Fatalf("RiskLevel=%q, want high", plan.RiskLevel)
	}
}

func TestBuildCommandOperationPlanLowRiskAutoEnabled(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoLowRisk = true
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "show git status",
		Steps: []CommandStep{{
			Program: "git",
			Args:    []string{"status", "--short"},
		}},
	}, policy)

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("ApprovalMode=%q, want auto_low_risk", plan.ApprovalMode)
	}
	if plan.RiskLevel != "low" {
		t.Fatalf("RiskLevel=%q", plan.RiskLevel)
	}
}

func TestBuildCommandOperationPlanUnknownProgramAutoApprovesByDefault(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoLowRisk = true
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "run custom inventory checker",
		Steps: []CommandStep{{
			Program: "corp-inventory-check",
			Args:    []string{"--list"},
		}},
	}, policy)

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("ApprovalMode=%q, want auto_low_risk", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoEligible {
		t.Fatalf("step AutoApproval=%q, want eligible", got)
	}
}

func TestBuildCommandOperationPlanUnknownProgramManualWhenAutoApproveDisabled(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoApprove = false
	policy.AutoLowRisk = true
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "run custom inventory checker",
		Steps: []CommandStep{{
			Program: "corp-inventory-check",
			Args:    []string{"--list"},
		}},
	}, policy)

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalManual {
		t.Fatalf("ApprovalMode=%q, want manual", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoManual {
		t.Fatalf("step AutoApproval=%q, want manual", got)
	}
}

func TestBuildCommandOperationPlanShellFormAutoEligibleWhenAutoApproveEnabled(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoLowRisk = true
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "count go files",
		Steps: []CommandStep{{
			Shell: "find . -name '*.go' | wc -l",
		}},
	}, policy)

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("ApprovalMode=%q, want auto_low_risk", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoEligible {
		t.Fatalf("step AutoApproval=%q, want eligible", got)
	}
	if plan.RiskLevel != "medium" {
		t.Fatalf("RiskLevel=%q, want medium for shell form", plan.RiskLevel)
	}
}

func TestBuildCommandOperationPlanShellFormRequiresManualWhenAutoApproveDisabled(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoApprove = false
	policy.AutoLowRisk = true
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "count go files",
		Steps: []CommandStep{{
			Shell: "find . -name '*.go' | wc -l",
		}},
	}, policy)

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalManual {
		t.Fatalf("ApprovalMode=%q, want manual", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoManual {
		t.Fatalf("step AutoApproval=%q, want manual", got)
	}
}

func TestBuildCommandOperationPlanHardDeniesCatastrophicCommand(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoLowRisk = true
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "remove everything",
		Steps: []CommandStep{{
			Program: "rm",
			Args:    []string{"-rf", "/"},
		}},
	}, policy)

	if plan.Status != StatusBlocked {
		t.Fatalf("Status=%q, want blocked", plan.Status)
	}
	if plan.ApprovalMode != ApprovalDenied {
		t.Fatalf("ApprovalMode=%q, want denied", plan.ApprovalMode)
	}
	if plan.BlockReason == "" {
		t.Fatal("expected a block reason")
	}
}

func TestBuildCommandOperationPlanMkdirPAutoEligible(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoLowRisk = true
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "create logs directory",
		Steps: []CommandStep{{
			Program:     "mkdir",
			Args:        []string{"-p", "logs"},
			SideEffects: []string{"local_file_write"},
		}},
	}, policy)

	if plan.Status != StatusReady {
		t.Fatalf("Status=%q", plan.Status)
	}
	if plan.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("ApprovalMode=%q, want auto_low_risk", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoEligible {
		t.Fatalf("step AutoApproval=%q, want eligible", got)
	}
}

func TestBuildCommandOperationPlanNetworkDeny(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.NetworkPolicy = ApprovalDenied
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "download release notes",
		Steps: []CommandStep{{
			Program:     "curl",
			Args:        []string{"https://example.test/release.txt"},
			SideEffects: []string{"network_read"},
		}},
	}, policy)

	if plan.Status != StatusBlocked {
		t.Fatalf("Status=%q, want blocked", plan.Status)
	}
	if plan.ApprovalMode != ApprovalDenied {
		t.Fatalf("ApprovalMode=%q, want denied", plan.ApprovalMode)
	}
	if plan.BlockReason == "" {
		t.Fatal("expected a block reason")
	}
}

func TestBuildCommandOperationPlanInstallDeny(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.InstallPolicy = ApprovalDenied
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "install package",
		Steps: []CommandStep{{
			Program:     "npm",
			Args:        []string{"install", "left-pad"},
			SideEffects: []string{"package_install"},
		}},
	}, policy)

	if plan.Status != StatusBlocked {
		t.Fatalf("Status=%q, want blocked", plan.Status)
	}
	if plan.ApprovalMode != ApprovalDenied {
		t.Fatalf("ApprovalMode=%q, want denied", plan.ApprovalMode)
	}
}

func TestBuildCommandOperationPlanOverwriteDeny(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.OverwritePolicy = ApprovalDenied
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "overwrite report",
		Steps: []CommandStep{{
			Program:     "cp",
			Args:        []string{"-f", "draft.md", "report.md"},
			SideEffects: []string{"overwrite"},
		}},
	}, policy)

	if plan.Status != StatusBlocked {
		t.Fatalf("Status=%q, want blocked", plan.Status)
	}
	if plan.ApprovalMode != ApprovalDenied {
		t.Fatalf("ApprovalMode=%q, want denied", plan.ApprovalMode)
	}
}

func TestBuildCommandOperationPlanAllowedWriteRoots(t *testing.T) {
	policy := DefaultCommandPolicy()
	policy.AutoLowRisk = true
	policy.AllowedWriteRoots = []string{"/repo/out"}

	inside := BuildCommandOperationPlan(CommandOperationRequest{
		Text:    "create output directory",
		WorkDir: "/repo",
		Steps: []CommandStep{{
			Program:     "mkdir",
			Args:        []string{"-p", "out/reports"},
			SideEffects: []string{"local_file_write"},
		}},
	}, policy)
	if inside.Status != StatusReady {
		t.Fatalf("inside Status=%q, want ready, reason=%s", inside.Status, inside.BlockReason)
	}
	if inside.ApprovalMode != ApprovalAutoLowRisk {
		t.Fatalf("inside ApprovalMode=%q, want auto_low_risk", inside.ApprovalMode)
	}

	outside := BuildCommandOperationPlan(CommandOperationRequest{
		Text:    "create tmp directory",
		WorkDir: "/repo",
		Steps: []CommandStep{{
			Program:     "mkdir",
			Args:        []string{"-p", "../tmp"},
			SideEffects: []string{"local_file_write"},
		}},
	}, policy)
	if outside.Status != StatusBlocked {
		t.Fatalf("outside Status=%q, want blocked", outside.Status)
	}
	if outside.ApprovalMode != ApprovalDenied {
		t.Fatalf("outside ApprovalMode=%q, want denied", outside.ApprovalMode)
	}
}
