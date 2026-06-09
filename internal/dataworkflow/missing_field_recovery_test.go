package dataworkflow

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestMissingJoinFieldFallbackPlanBuildsEnrichAction(t *testing.T) {
	violation := dataquery.DataTaskViolation{
		Code:          "field_contract_violation",
		ActionKind:    string(dataquery.DataActionJoinRecords),
		InputAlias:    "base.json",
		MissingFields: []string{"category_id"},
	}
	plan, ok := MissingJoinFieldFallbackPlan(MissingJoinFieldFallbackInput{
		Current:   dataquery.TaskPlan{Goal: "finish typed graph"},
		Violation: violation,
		SchemaProjections: []ArtifactSchemaProjection{
			{
				ID:      "base",
				Aliases: []string{"base.json"},
				Kind:    string(dataquery.DataActionFilterRecords),
				Fields:  []string{"item_id", "category_name"},
			},
			{
				ID:      "mapping",
				Aliases: []string{"mapping.json"},
				Kind:    string(dataquery.DataActionNormalizeEntities),
				Fields:  []string{"source_value", "category_name", "category_id"},
			},
		},
	})
	if !ok {
		t.Fatal("MissingJoinFieldFallbackPlan ok=false")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionEnrichRecords {
		t.Fatalf("actions=%+v, want enrich_records", plan.Actions)
	}
	action := plan.Actions[0]
	if action.OutputArtifact != "base_with_category_id.json" {
		t.Fatalf("OutputArtifact=%q, want base_with_category_id.json", action.OutputArtifact)
	}
	var specs []map[string]any
	if err := json.Unmarshal([]byte(action.Params["lookup_specs_json"]), &specs); err != nil || len(specs) != 1 {
		t.Fatalf("lookup_specs_json=%q err=%v", action.Params["lookup_specs_json"], err)
	}
	if specs[0]["target_field"] != "category_id" || specs[0]["lookup_path"] != "mapping.json" {
		t.Fatalf("spec=%+v, want target category_id via mapping.json", specs[0])
	}
	baseFields, ok := specs[0]["base_fields"].([]any)
	if !ok || len(baseFields) != 1 || baseFields[0] != "category_name" {
		t.Fatalf("base_fields=%+v, want exact structural key category_name", specs[0]["base_fields"])
	}
	lookupFields, ok := specs[0]["lookup_fields"].([]any)
	if !ok || len(lookupFields) != 1 || lookupFields[0] != "category_name" {
		t.Fatalf("lookup_fields=%+v, want exact structural key category_name", specs[0]["lookup_fields"])
	}
	if specs[0]["lookup_value_field"] != "category_id" || specs[0]["match_mode"] != "exact" {
		t.Fatalf("spec=%+v, want exact lookup value category_id", specs[0])
	}
}

func TestMissingJoinFieldFallbackPlanRequiresExactTargetField(t *testing.T) {
	violation := dataquery.DataTaskViolation{
		Code:          "field_contract_violation",
		ActionKind:    string(dataquery.DataActionJoinRecords),
		InputAlias:    "base.json",
		MissingFields: []string{"category_id"},
	}
	_, ok := MissingJoinFieldFallbackPlan(MissingJoinFieldFallbackInput{
		Current:   dataquery.TaskPlan{Goal: "finish typed graph"},
		Violation: violation,
		SchemaProjections: []ArtifactSchemaProjection{
			{
				ID:      "base",
				Aliases: []string{"base.json"},
				Kind:    string(dataquery.DataActionFilterRecords),
				Fields:  []string{"category_name"},
			},
			{
				ID:      "mapping",
				Aliases: []string{"mapping.json"},
				Kind:    string(dataquery.DataActionNormalizeEntities),
				Fields:  []string{"category_name", "canonical_id"},
			},
		},
	})
	if ok {
		t.Fatal("missing-field fallback should not infer a non-exact target value field")
	}
}

func TestMissingJoinFieldFallbackPlanRequiresStructuralKeyRelation(t *testing.T) {
	violation := dataquery.DataTaskViolation{
		Code:          "field_contract_violation",
		ActionKind:    string(dataquery.DataActionJoinRecords),
		InputAlias:    "base.json",
		MissingFields: []string{"category_id"},
	}
	_, ok := MissingJoinFieldFallbackPlan(MissingJoinFieldFallbackInput{
		Current:   dataquery.TaskPlan{Goal: "finish typed graph"},
		Violation: violation,
		SchemaProjections: []ArtifactSchemaProjection{
			{
				ID:      "base",
				Aliases: []string{"base.json"},
				Kind:    string(dataquery.DataActionFilterRecords),
				Fields:  []string{"category_name"},
			},
			{
				ID:      "mapping",
				Aliases: []string{"mapping.json"},
				Kind:    string(dataquery.DataActionNormalizeEntities),
				Fields:  []string{"source_value", "category_id"},
			},
		},
	})
	if ok {
		t.Fatal("missing-field fallback should not guess lookup keys without a structural common field")
	}
}

func TestHistoricalMissingJoinFieldFallbackPlanSuppressesSeenPlan(t *testing.T) {
	violation := dataquery.DataTaskViolation{
		Code:          "field_contract_violation",
		ActionKind:    string(dataquery.DataActionJoinRecords),
		InputAlias:    "base.json",
		MissingFields: []string{"category_id"},
	}
	projections := []ArtifactSchemaProjection{
		{ID: "base", Kind: string(dataquery.DataActionFilterRecords), NodeClass: ArtifactNodeClassRecord, Aliases: []string{"base.json"}, Fields: []string{"category_name"}},
		{ID: "mapping", Aliases: []string{"mapping.json"}, Kind: string(dataquery.DataActionNormalizeEntities), Fields: []string{"category_name", "category_id"}},
	}
	first, ok := HistoricalMissingJoinFieldFallbackPlan(MissingJoinFieldFallbackInput{
		Records:           []WorkflowRecord{{Violations: []dataquery.DataTaskViolation{violation}}},
		SchemaProjections: projections,
	})
	if !ok {
		t.Fatal("HistoricalMissingJoinFieldFallbackPlan ok=false")
	}
	_, ok = HistoricalMissingJoinFieldFallbackPlan(MissingJoinFieldFallbackInput{
		Records: []WorkflowRecord{
			{Violations: []dataquery.DataTaskViolation{violation}},
			{Plan: first},
		},
		SchemaProjections: projections,
	})
	if ok {
		t.Fatal("seen missing-field fallback should not be emitted again")
	}
}

func TestMissingComputeGroupFieldFallbackPlanUsesFocusedContributionBase(t *testing.T) {
	violation := dataquery.DataTaskViolation{
		Code:          "field_contract_violation",
		ActionID:      "compute_target_contributions",
		ActionKind:    string(dataquery.DataActionComputeContribs),
		Role:          "contribution_group_key",
		InputAlias:    "coverage_records.json",
		MissingFields: []string{"canonical_label"},
	}
	plan, ok := MissingComputeGroupFieldFallbackPlan(MissingComputeGroupFieldFallbackInput{
		Current: dataquery.TaskPlan{
			Goal: "finish grouped contribution projection",
			Actions: []dataquery.DataAction{{
				ID:         "compute_target_contributions",
				Kind:       dataquery.DataActionComputeContribs,
				InputPaths: []string{"coverage_records.json"},
				Params: map[string]string{
					"group_key_field": "canonical_label",
					"value_field":     "value",
					"item_id_field":   "record_id",
					"filters_json":    `[{"field":"active","op":"eq","value":true}]`,
				},
			}},
		},
		Violation: violation,
		SchemaProjections: []ArtifactSchemaProjection{
			{
				ID:      "coverage",
				Aliases: []string{"coverage_records.json"},
				Kind:    string(dataquery.DataActionExtractRecords),
				Fields:  []string{"active", "raw_label", "record_id", "value", "canonical_label"},
			},
			{
				ID:        "qualified",
				Aliases:   []string{"qualified_observations.json"},
				Kind:      string(dataquery.DataActionQualifyRecords),
				NodeClass: ArtifactNodeClassRecord,
				Fields:    []string{"active", "raw_label", "record_id", "value"},
				RowCount:  5,
			},
			{
				ID:        "labels",
				Aliases:   []string{"labels.csv#records"},
				Kind:      string(dataquery.DataActionExtractRecords),
				NodeClass: ArtifactNodeClassRecord,
				Fields:    []string{"raw_label", "canonical_label"},
				RowCount:  4,
			},
		},
	})
	if !ok {
		t.Fatal("MissingComputeGroupFieldFallbackPlan ok=false")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionEnrichRecords {
		t.Fatalf("actions=%+v, want enrich_records", plan.Actions)
	}
	action := plan.Actions[0]
	if action.Params["base_path"] != "qualified_observations.json" {
		t.Fatalf("base_path=%q, want focused qualified observations artifact", action.Params["base_path"])
	}
	if action.OutputArtifact != "qualified_observations_with_canonical_label.json" {
		t.Fatalf("OutputArtifact=%q, want qualified_observations_with_canonical_label.json", action.OutputArtifact)
	}
	var specs []map[string]any
	if err := json.Unmarshal([]byte(action.Params["lookup_specs_json"]), &specs); err != nil || len(specs) != 1 {
		t.Fatalf("lookup_specs_json=%q err=%v", action.Params["lookup_specs_json"], err)
	}
	if specs[0]["lookup_path"] != "labels.csv#records" ||
		specs[0]["target_field"] != "canonical_label" ||
		specs[0]["lookup_value_field"] != "canonical_label" {
		t.Fatalf("spec=%+v, want labels canonical lookup", specs[0])
	}
	baseFields, ok := specs[0]["base_fields"].([]any)
	if !ok || len(baseFields) != 1 || baseFields[0] != "raw_label" {
		t.Fatalf("base_fields=%+v, want raw_label", specs[0]["base_fields"])
	}
}

func TestInvalidRecordEnrichFallbackPlanBuildsLookupFromMultiInputDerive(t *testing.T) {
	plan, ok := InvalidRecordEnrichFallbackPlan(InvalidRecordEnrichFallbackInput{
		Current: dataquery.TaskPlan{Goal: "continue"},
		Action: dataquery.DataAction{
			Kind:       dataquery.DataActionDeriveFields,
			InputPaths: []string{"base.json", "mapping.json"},
			Params: map[string]string{
				"target_field": "category_id",
			},
		},
		InputPaths: []string{"base.json", "mapping.json"},
		SchemaProjections: []ArtifactSchemaProjection{
			{ID: "base", Aliases: []string{"base.json"}, Fields: []string{"category_name"}},
			{ID: "mapping", Aliases: []string{"mapping.json"}, Fields: []string{"category_name", "category_id"}},
		},
	})
	if !ok {
		t.Fatal("InvalidRecordEnrichFallbackPlan ok=false")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionEnrichRecords {
		t.Fatalf("actions=%+v, want enrich_records", plan.Actions)
	}
	if got := plan.Actions[0].InputPaths; len(got) != 2 || got[0] != "base.json" || got[1] != "mapping.json" {
		t.Fatalf("inputs=%+v, want base and mapping", got)
	}
}
