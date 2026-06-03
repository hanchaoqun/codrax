package operation

import "testing"

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
