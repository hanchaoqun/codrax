package dataworkflow

import "testing"

func TestRequiredMaterialSchedulingGuardRejectsTerminalMissingPaths(t *testing.T) {
	guard := RequiredMaterialSchedulingGuardResult(RequiredMaterialSchedulingGuardInput{
		RequiredPaths:  []string{"orders.csv", "rules.md"},
		ScheduledPaths: []string{"orders.csv"},
	})
	if guard.Code != "required_material_scheduling" {
		t.Fatalf("guard=%+v, want required_material_scheduling", guard)
	}
	if len(guard.Violations) != 1 {
		t.Fatalf("violations=%+v, want one typed violation", guard.Violations)
	}
	if got := guard.Violations[0].InputAliases; len(got) != 1 || got[0] != "rules.md" {
		t.Fatalf("missing inputs=%+v, want rules.md", got)
	}
}

func TestRequiredMaterialSchedulingGuardAllowsContinuation(t *testing.T) {
	guard := RequiredMaterialSchedulingGuardResult(RequiredMaterialSchedulingGuardInput{
		ContinueAfter: true,
		RequiredPaths: []string{"rules.md"},
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want empty while continuation remains", guard)
	}
}

func TestRequiredMaterialSchedulingGuardAllowsScheduledPaths(t *testing.T) {
	guard := RequiredMaterialSchedulingGuardResult(RequiredMaterialSchedulingGuardInput{
		RequiredPaths:  []string{"orders.csv", "rules.md"},
		ScheduledPaths: []string{"rules.md", "orders.csv"},
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want empty when required paths are scheduled", guard)
	}
}
