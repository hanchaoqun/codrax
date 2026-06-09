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

func TestTextConstraintCoverageGuardResultUsesTypedViolation(t *testing.T) {
	guard := TextConstraintCoverageGuardResult(TextConstraintCoverageGuardInput{
		ValidationLedgerCount:       2,
		CustomInputPaths:            []string{"rules.md", "data.csv"},
		ScriptConsumedMaterialPaths: []string{"rules.md", "data.csv"},
	})
	if guard.Code != "text_constraint_coverage_required" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed text-constraint coverage violation", guard)
	}
	v := guard.Violations[0]
	if len(v.InputAliases) != 1 || v.InputAliases[0] != "rules.md" || len(v.RepairActionHints) != 1 || v.RepairActionHints[0] != "derive_rules" {
		t.Fatalf("violation=%+v, want text material input and derive_rules hint", v)
	}
}

func TestTextConstraintCoverageGuardResultAllowsRuleCoverageRequired(t *testing.T) {
	guard := TextConstraintCoverageGuardResult(TextConstraintCoverageGuardInput{
		RuleCoverageRequired:        true,
		ValidationLedgerCount:       2,
		CustomInputPaths:            []string{"rules.md"},
		ScriptConsumedMaterialPaths: []string{"rules.md"},
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want existing rule coverage contract allowed", guard)
	}
}

func TestRuleCoveragePrerequisiteGuardRejectsDownstreamActions(t *testing.T) {
	guard := RuleCoveragePrerequisiteGuardResult(RuleCoveragePrerequisiteGuardInput{
		RuleCoverageRequired: true,
		Actions: []dataquery.DataAction{
			{ID: "read_rules", Kind: dataquery.DataActionExtractRecords, InputPaths: []string{"rules.md"}},
			{ID: "join_records", Kind: dataquery.DataActionJoinRecords, InputPaths: []string{"left.csv", "right.csv"}},
		},
		RuleMaterialPaths: []string{"rules.md"},
	})
	if guard.Code != "rule_coverage_prerequisite_missing" {
		t.Fatalf("guard=%+v, want rule_coverage_prerequisite_missing", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].ActionID != "join_records" || guard.Violations[0].RepairActionHints[0] != string(dataquery.DataActionDeriveRules) {
		t.Fatalf("violations=%+v, want blocked join with derive_rules hint", guard.Violations)
	}
}

func TestRuleCoveragePrerequisiteGuardAllowsCoverageAndDerivedRules(t *testing.T) {
	for name, input := range map[string]RuleCoveragePrerequisiteGuardInput{
		"already_has_rules": {
			RuleCoverageRequired: true,
			RuleCoverageRecords:  1,
			Actions:              []dataquery.DataAction{{ID: "compute", Kind: dataquery.DataActionComputeContribs}},
		},
		"derive_only": {
			RuleCoverageRequired: true,
			Actions:              []dataquery.DataAction{{ID: "rules", Kind: dataquery.DataActionDeriveRules}},
		},
		"coverage_only": {
			RuleCoverageRequired: true,
			Actions:              []dataquery.DataAction{{ID: "extract", Kind: dataquery.DataActionExtractRecords}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if guard := RuleCoveragePrerequisiteGuardResult(input); !guard.Empty() {
				t.Fatalf("guard=%+v, want empty", guard)
			}
		})
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
