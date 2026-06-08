package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestActionInputAvailabilityGuardUsesWorkflowOwnedAvailableSet(t *testing.T) {
	records := []WorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "joined_records",
				Kind:        string(dataquery.DataActionJoinRecords),
				SourcePaths: []string{"orders.csv", "lookup.csv"},
				Fields: map[string]string{
					"artifact_path": "generated/joined_records.json",
				},
			}},
		},
	}}
	action := dataquery.DataAction{
		ID:         "filter_joined",
		Kind:       dataquery.DataActionFilterRecords,
		InputPaths: []string{"joined_records"},
	}
	guard := ActionInputAvailabilityGuardResult(ActionInputAvailabilityGuardInput{
		Records:     records,
		Action:      action,
		ActionIndex: 0,
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want artifact alias accepted", guard)
	}

	action.InputPaths = []string{"source.csv"}
	guard = ActionInputAvailabilityGuardResult(ActionInputAvailabilityGuardInput{
		Records: records,
		Plan: dataquery.TaskPlan{
			InputPaths: []string{"source.csv"},
		},
		Action:      action,
		ActionIndex: 0,
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want top-level external material input accepted", guard)
	}

	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{
		{ID: "derive", Kind: dataquery.DataActionDeriveFields, OutputArtifact: "derived.json"},
		{ID: "filter", Kind: dataquery.DataActionFilterRecords, InputPaths: []string{"derived.json"}},
	}}
	guard = ActionInputAvailabilityGuardResult(ActionInputAvailabilityGuardInput{
		Records:     records,
		Plan:        plan,
		Action:      plan.Actions[1],
		ActionIndex: 1,
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want prior action output accepted", guard)
	}
}

func TestActionInputAvailabilityGuardBlocksUnavailableInputs(t *testing.T) {
	guard := ActionInputAvailabilityGuardResult(ActionInputAvailabilityGuardInput{
		Records: []WorkflowRecord{{Result: &dataquery.Result{ConsumedPaths: []string{"orders.csv"}}}},
		Action: dataquery.DataAction{
			ID:         "filter_missing",
			Kind:       dataquery.DataActionFilterRecords,
			InputPaths: []string{"future.json"},
		},
		ActionIndex: 0,
	})
	if guard.Code != "unavailable_action_input" {
		t.Fatalf("guard=%+v, want unavailable input guard", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].InputAliases[0] != "future.json" {
		t.Fatalf("violations=%+v, want missing future.json", guard.Violations)
	}
}

func TestActionSchemaShapeGuardRejectsNonRecordArtifacts(t *testing.T) {
	records := []WorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:     "schema_summary",
			Kind:   string(dataquery.DataActionInspectMaterial),
			Fields: map[string]string{"json_shape": "object", "count": "3"},
		}}},
	}}
	guard := ActionSchemaShapeGuardResult(ActionSchemaShapeGuardInput{
		Records: records,
		Action: dataquery.DataAction{
			ID:         "filter_schema_summary",
			Kind:       dataquery.DataActionFilterRecords,
			InputPaths: []string{"schema_summary"},
		},
		ActionIndex: 0,
	})
	if guard.Code != "artifact_schema_incompatible" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want artifact_schema_incompatible", guard)
	}
	violation := guard.Violations[0]
	if violation.InputAlias != "schema_summary" ||
		violation.ActionKind != string(dataquery.DataActionFilterRecords) ||
		violation.RepairActionHints[0] != string(dataquery.DataActionExtractRecords) {
		t.Fatalf("violation=%+v, want typed record-shape violation", violation)
	}
}
