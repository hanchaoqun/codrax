package dataworkflow

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestNormalizeRolePathActionInputsFillsNormalizeEntityInputs(t *testing.T) {
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{{
			Kind:       dataquery.DataActionNormalizeEntities,
			InputPaths: []string{"existing.json"},
			Params: map[string]string{
				"source_path":    "records.json",
				"reference_path": "reference.json",
			},
		}},
	}

	if !NormalizeRolePathActionInputs(&plan) {
		t.Fatalf("NormalizeRolePathActionInputs returned false, want true")
	}
	want := []string{"existing.json", "records.json", "reference.json"}
	if !reflect.DeepEqual(plan.Actions[0].InputPaths, want) {
		t.Fatalf("InputPaths=%v, want %v", plan.Actions[0].InputPaths, want)
	}
}

func TestNormalizeRolePathActionInputsKeepsJoinOrder(t *testing.T) {
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{{
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"seed.json"},
			Params: map[string]string{
				"right_path": "right.json",
				"left_path":  "left.json",
			},
		}},
	}

	NormalizeRolePathActionInputs(&plan)
	want := []string{"seed.json", "left.json", "right.json"}
	if !reflect.DeepEqual(plan.Actions[0].InputPaths, want) {
		t.Fatalf("InputPaths=%v, want %v", plan.Actions[0].InputPaths, want)
	}
}

func TestNormalizeRolePathActionInputsReadsResolutionSpecs(t *testing.T) {
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{{
			Kind: dataquery.DataActionApplyResolutions,
			Params: map[string]string{
				"resolution_specs_json": `[
					{"base_path":"base.json","resolution_path":"resolutions.json"},
					{"record_path":"records.json","reference_path":"reference.json"}
				]`,
			},
		}},
	}

	if !NormalizeRolePathActionInputs(&plan) {
		t.Fatalf("NormalizeRolePathActionInputs returned false, want true")
	}
	want := []string{"base.json", "resolutions.json", "records.json", "reference.json"}
	if !reflect.DeepEqual(plan.Actions[0].InputPaths, want) {
		t.Fatalf("InputPaths=%v, want %v", plan.Actions[0].InputPaths, want)
	}
}

func TestNormalizeRolePathActionInputsNoopsWhenAlreadyNormalized(t *testing.T) {
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{{
			Kind:       dataquery.DataActionEnrichRecords,
			InputPaths: []string{"base.json", "lookup.json"},
			Params: map[string]string{
				"base_path":   "base.json",
				"lookup_path": "lookup.json",
			},
		}},
	}

	if NormalizeRolePathActionInputs(&plan) {
		t.Fatalf("NormalizeRolePathActionInputs returned true for an already normalized action")
	}
	want := []string{"base.json", "lookup.json"}
	if !reflect.DeepEqual(plan.Actions[0].InputPaths, want) {
		t.Fatalf("InputPaths=%v, want %v", plan.Actions[0].InputPaths, want)
	}
}
