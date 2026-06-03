package operation

import (
	"strings"
	"testing"
)

func TestBuildPlanPresentationNoProviderIsPlanOnly(t *testing.T) {
	plan := BuildPlan(Request{
		Operation:       "presentation_generation",
		NeedsRepoAccess: true,
		RiskLevel:       "low",
		SideEffects:     []string{"local_file_write", "local_file_write"},
		TargetSurface:   "slides",
	}, nil)

	if plan.Kind != "presentation_generation" {
		t.Fatalf("Kind=%q", plan.Kind)
	}
	if plan.CanExecute {
		t.Fatal("plan should not execute without an explicit provider")
	}
	if plan.MissingCapability == "" {
		t.Fatal("missing capability should be explained")
	}
	if len(plan.SideEffects) != 1 || plan.SideEffects[0] != "local_file_write" {
		t.Fatalf("SideEffects=%v", plan.SideEffects)
	}
	if len(plan.Steps) < 3 || !plan.Steps[0].Blocked {
		t.Fatalf("repo-dependent operation should include a blocked source-facts step: %+v", plan.Steps)
	}
}

func TestBuildPlanHighRiskRequiresConfirmation(t *testing.T) {
	plan := BuildPlan(Request{
		OperationKind:        "browser_operation",
		RiskLevel:            "high",
		TargetSurface:        "browser",
		SideEffects:          []string{"browser_ui", "network_submit", "destructive"},
		RequiresConfirmation: false,
	}, []ProviderInfo{{Name: "browser", Kind: "browser_operation", Surfaces: []string{"browser"}}})

	if !plan.RequiresConfirmation {
		t.Fatal("high-risk operation should require confirmation")
	}
	if plan.CanExecute {
		t.Fatal("high-risk operation must not execute before confirmation")
	}
	if plan.Provider != "browser" {
		t.Fatalf("Provider=%q", plan.Provider)
	}
}

func TestBuildPlanProviderGateBlocksExecution(t *testing.T) {
	plan := BuildPlan(Request{
		OperationKind: "presentation_generation",
		RiskLevel:     "low",
		TargetSurface: "slides",
	}, []ProviderInfo{{
		Name:         "mcp:slides",
		Kind:         "presentation_generation",
		Surfaces:     []string{"slides"},
		RequiresGate: true,
		ToolName:     "run_operation",
	}})

	if plan.Provider != "mcp:slides" {
		t.Fatalf("Provider=%q", plan.Provider)
	}
	if !plan.RequiresConfirmation {
		t.Fatal("provider gate should require confirmation")
	}
	if plan.CanExecute {
		t.Fatal("provider gate must pause execution")
	}
	if plan.ProviderTool != "run_operation" {
		t.Fatalf("ProviderTool=%q", plan.ProviderTool)
	}
	if plan.MissingCapability == "" {
		t.Fatal("provider gate should explain why execution paused")
	}
}

func TestBuildPlanLowRiskProviderCanExecuteWithoutGate(t *testing.T) {
	plan := BuildPlan(Request{
		OperationKind: "presentation_generation",
		RiskLevel:     "low",
		TargetSurface: "slides",
		SideEffects:   []string{"local_file_write"},
	}, []ProviderInfo{{
		Name:     "skill:slides",
		Kind:     "presentation_generation",
		Surfaces: []string{"slides"},
		ToolName: "run",
	}})

	if plan.Provider != "skill:slides" {
		t.Fatalf("Provider=%q", plan.Provider)
	}
	if plan.RequiresConfirmation {
		t.Fatalf("low-risk provider without explicit gate should not require confirmation: %+v", plan)
	}
	if !plan.CanExecute {
		t.Fatalf("low-risk provider with tool should execute: %+v", plan)
	}
}

func TestBuildPlanProviderWithoutToolIsPlanOnly(t *testing.T) {
	plan := BuildPlan(Request{
		OperationKind: "presentation_generation",
		RiskLevel:     "low",
		TargetSurface: "slides",
	}, []ProviderInfo{{
		Name:         "mcp:slides",
		Kind:         "presentation_generation",
		Surfaces:     []string{"slides"},
		RequiresGate: false,
	}})

	if plan.Provider != "mcp:slides" {
		t.Fatalf("Provider=%q", plan.Provider)
	}
	if plan.CanExecute {
		t.Fatal("provider with no tool must stay plan-only")
	}
	if !strings.Contains(plan.MissingCapability, "operation_tool") {
		t.Fatalf("MissingCapability=%q", plan.MissingCapability)
	}
}

func TestBuildPlanMatchesEntryWorkflowProvider(t *testing.T) {
	plan := BuildPlan(Request{
		OperationKind: "external_skill_workflow",
		RiskLevel:     "medium",
		TargetSurface: "slides",
	}, []ProviderInfo{{
		Name:         "skill:manual_reader",
		Kind:         "external_skill_workflow",
		Surfaces:     []string{"local_file", "slides"},
		RequiresGate: true,
		ToolName:     "run",
		Workflows: []WorkflowInfo{{
			Name:          "manual_to_deck",
			Entry:         true,
			OperationKind: "external_skill_workflow",
			TargetSurface: "slides",
		}},
	}})

	if plan.Provider != "skill:manual_reader" {
		t.Fatalf("Provider=%q", plan.Provider)
	}
	if plan.Kind != "external_skill_workflow" || plan.TargetSurface != "slides" {
		t.Fatalf("unexpected plan kind/target: %+v", plan)
	}
	if !plan.RequiresConfirmation || plan.CanExecute {
		t.Fatalf("workflow provider should pause for approval: %+v", plan)
	}
}
