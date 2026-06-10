package dataworkflow

import (
	"strings"
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

func TestCoveredMaterialPathsUsesWorkflowCoverageSemantics(t *testing.T) {
	records := []WorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"consumed.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "records",
				SourcePaths: []string{"source.csv"},
				Fields:      map[string]string{"artifact_path": "generated/records.json"},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		InputPaths: []string{"logical_seed"},
		Actions: []dataquery.DataAction{{
			ID:             "derive",
			Kind:           dataquery.DataActionDeriveFields,
			InputPaths:     []string{"records"},
			OutputArtifact: "derived.json",
		}},
	}
	covered := CoveredMaterialPaths(records, plan, 1)
	for _, want := range []string{"consumed.csv", "source.csv", "records", "generated/records.json", "logical_seed", "derived.json"} {
		if !CoveragePathCovered(covered, want) {
			t.Fatalf("covered=%v, want %s covered", covered, want)
		}
	}
}

func TestAvailableActionInputPathsKeepsTopLevelLogicalSeedsOut(t *testing.T) {
	records := []WorkflowRecord{{Result: &dataquery.Result{ConsumedPaths: []string{"consumed.csv"}}}}
	plan := dataquery.TaskPlan{InputPaths: []string{"logical_seed"}}
	available := AvailableActionInputPaths(records, plan, 0)
	if CoveragePathCovered(available, "logical_seed") {
		t.Fatalf("available=%v, logical seed should not become executable input", available)
	}
	if !CoveragePathCovered(available, "consumed.csv") {
		t.Fatalf("available=%v, consumed.csv should remain executable", available)
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

func TestActionSchemaShapeGuardReportsExecutableRecordCandidates(t *testing.T) {
	records := []WorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:          "observations_raw",
			Kind:        "unknown",
			SourcePaths: []string{"observations.csv"},
			Fields:      map[string]string{"json_shape": "object"},
		}, {
			ID:                "READ_LABELS",
			Kind:              string(dataquery.DataActionExtractRecords),
			Headers:           []string{"raw_label", "canonical_label"},
			SourcePaths:       []string{"labels.csv"},
			SourceRecordPaths: []string{"labels.csv#records"},
			RowCount:          4,
			Fields: map[string]string{
				"json_shape":       "array(len=4,item=object(keys=raw_label,canonical_label))",
				"artifact_aliases": "labels.csv#records",
			},
		}, {
			ID:                "READ_OBSERVATIONS",
			Kind:              string(dataquery.DataActionExtractRecords),
			Headers:           []string{"raw_label", "active", "value"},
			SourcePaths:       []string{"observations.csv"},
			SourceRecordPaths: []string{"observations_raw", "observations.csv#records"},
			RowCount:          6,
			Fields: map[string]string{
				"json_shape":       "array(len=6,item=object(keys=raw_label,active,value))",
				"artifact_aliases": "observations.csv#records",
			},
		}}},
	}}
	guard := ActionSchemaShapeGuardResult(ActionSchemaShapeGuardInput{
		Records: records,
		Action: dataquery.DataAction{
			ID:         "join_observations_with_labels",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"observations_raw", "labels.csv#records"},
			Params: map[string]string{
				"left_fields":  `["raw_label"]`,
				"right_fields": `["raw_label"]`,
			},
		},
		ActionIndex: 0,
	})
	if guard.Code != "artifact_schema_incompatible" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want artifact_schema_incompatible", guard)
	}
	violation := guard.Violations[0]
	if len(violation.CandidateArtifacts) == 0 {
		t.Fatalf("violation=%+v, want executable record candidate handoff", violation)
	}
	if !strings.Contains(violation.CandidateArtifacts[0], "READ_OBSERVATIONS") ||
		!strings.Contains(violation.CandidateArtifacts[0], "raw_label") {
		t.Fatalf("candidate_artifacts=%v, want same-lineage READ_OBSERVATIONS with raw_label", violation.CandidateArtifacts)
	}
	if !strings.Contains(guard.Message, "Candidate record-shaped input") {
		t.Fatalf("message=%q, want candidate hint", guard.Message)
	}
}
