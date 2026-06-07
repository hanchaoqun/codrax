package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

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

func TestBroadCustomPrerequisiteGuardRejectsMissingPrerequisites(t *testing.T) {
	action := dataquery.DataAction{
		ID:         "wide_transform",
		Kind:       dataquery.DataActionCustomTransform,
		Script:     "emit({})",
		InputPaths: []string{"a.csv", "b.csv"},
	}
	guard := BroadCustomPrerequisiteGuardResult(BroadCustomPrerequisiteGuardInput{
		Action:      action,
		ActionIndex: 2,
		IsBroad:     true,
		Missing:     []string{"b.csv"},
	})
	if guard.Code != "broad_custom_prerequisite_missing" {
		t.Fatalf("guard=%+v, want broad_custom_prerequisite_missing", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].ActionID != "wide_transform" {
		t.Fatalf("violations=%+v, want action-shaped violation", guard.Violations)
	}
	if got := guard.Violations[0].InputAliases; len(got) != 1 || got[0] != "b.csv" {
		t.Fatalf("input_aliases=%+v, want missing b.csv", got)
	}
}

func TestBroadCustomPrerequisiteGuardPassesNonBroadOrCovered(t *testing.T) {
	action := dataquery.DataAction{
		ID:     "wide_transform",
		Kind:   dataquery.DataActionCustomTransform,
		Script: "emit({})",
	}
	for name, input := range map[string]BroadCustomPrerequisiteGuardInput{
		"not_broad": {
			Action:  action,
			IsBroad: false,
			Missing: []string{"a.csv"},
		},
		"covered": {
			Action:  action,
			IsBroad: true,
		},
		"typed_action": {
			Action: dataquery.DataAction{
				ID:     "derive",
				Kind:   dataquery.DataActionDeriveFields,
				Script: "emit({})",
			},
			IsBroad: true,
			Missing: []string{"a.csv"},
		},
		"no_script": {
			Action: dataquery.DataAction{
				ID:   "wide_transform",
				Kind: dataquery.DataActionCustomTransform,
			},
			IsBroad: true,
			Missing: []string{"a.csv"},
		},
	} {
		if guard := BroadCustomPrerequisiteGuardResult(input); !guard.Empty() {
			t.Fatalf("%s: guard=%+v, want empty", name, guard)
		}
	}
}
