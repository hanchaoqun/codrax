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

func TestBuildCommandOperationPlanLowRiskAutoDisabledDefaultsManual(t *testing.T) {
	plan := BuildCommandOperationPlan(CommandOperationRequest{
		Text: "show current directory",
		Steps: []CommandStep{{
			Program: "pwd",
		}},
	}, DefaultCommandPolicy())

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

func TestBuildCommandOperationPlanUnknownProgramRequiresManual(t *testing.T) {
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
	if plan.ApprovalMode != ApprovalManual {
		t.Fatalf("ApprovalMode=%q, want manual", plan.ApprovalMode)
	}
	if got := plan.Steps[0].AutoApproval; got != StepAutoManual {
		t.Fatalf("step AutoApproval=%q, want manual", got)
	}
	if plan.RiskLevel != "medium" {
		t.Fatalf("RiskLevel=%q, want medium", plan.RiskLevel)
	}
}

func TestBuildCommandOperationPlanShellFormRequiresManual(t *testing.T) {
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
