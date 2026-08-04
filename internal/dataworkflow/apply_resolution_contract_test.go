package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestApplyResolutionContractGuardRejectsDiagnosticResolutionInput(t *testing.T) {
	projections := []ArtifactSchemaProjection{{
		ID:        "records",
		NodeClass: ArtifactNodeClassRecord,
		Aliases:   []string{"records.json"},
		JSONShape: "array(len=2)",
		Fields:    []string{"item_id", "name"},
	}, {
		ID:        "records.json#entity_resolution_source",
		Kind:      "apply_entity_resolutions/resolution",
		NodeClass: ArtifactNodeClassDiagnosticChild,
		Aliases:   []string{"records.json#entity_resolution_source"},
		JSONShape: "array(len=2)",
		Fields:    []string{"item_id", "source_value", "canonical_id", "canonical_label"},
	}}
	action := dataquery.DataAction{
		ID:         "apply_diag",
		Kind:       dataquery.DataActionApplyResolutions,
		InputPaths: []string{"records.json", "records.json#entity_resolution_source"},
		Params: map[string]string{
			"base_path": "records.json",
			"resolution_specs": `[{
				"resolution_path":"records.json#entity_resolution_source",
				"resolution_key_fields":["item_id"],
				"target_id_field":"name_canonical_id"
			}]`,
		},
	}
	guard := ApplyResolutionContractGuardResult(ApplyResolutionContractGuardInput{
		Action:            action,
		SchemaProjections: projections,
	})
	if guard.Code != "apply_resolution_diagnostic_input" {
		t.Fatalf("guard=%#v, want diagnostic input rejection", guard)
	}
}

func TestApplyResolutionContractGuardUsesSourceRecordLineage(t *testing.T) {
	projections := []ArtifactSchemaProjection{{
		ID:          "resolved_records.json",
		NodeClass:   ArtifactNodeClassRecord,
		Aliases:     []string{"resolved_records.json"},
		SourcePaths: []string{"records.csv", "lookup_a.csv"},
		JSONShape:   "array(len=20)",
		Fields:      []string{"item_id", "category_raw", "lookup_id"},
	}, {
		ID:                "entity_mappings_category_raw.json",
		NodeClass:         ArtifactNodeClassArtifact,
		Aliases:           []string{"entity_mappings_category_raw.json"},
		SourcePaths:       []string{"lookup_b.csv", "unrelated_reference.csv", "resolved_records.json"},
		SourceRecordPaths: []string{"resolved_records.json"},
		ReferencePaths:    []string{"lookup_b.csv"},
		JSONShape:         "array(len=8)",
		Fields:            []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
	}}
	action := dataquery.DataAction{
		ID:         "apply_category",
		Kind:       dataquery.DataActionApplyResolutions,
		InputPaths: []string{"resolved_records.json", "entity_mappings_category_raw.json"},
		Params: map[string]string{
			"base_path": "resolved_records.json",
			"resolution_specs": `[{
				"resolution_path":"entity_mappings_category_raw.json",
				"source_field":"category_raw",
				"resolution_key_fields":["item_id"],
				"target_id_field":"category_code"
			}]`,
		},
	}
	if guard := ApplyResolutionContractGuardResult(ApplyResolutionContractGuardInput{Action: action, SchemaProjections: projections}); !guard.Empty() {
		t.Fatalf("guard=%#v, want source_record_paths lineage to pass", guard)
	}
	projections[1].SourceRecordPaths = []string{"other_records.json"}
	guard := ApplyResolutionContractGuardResult(ApplyResolutionContractGuardInput{Action: action, SchemaProjections: projections})
	if guard.Code != "apply_resolution_lineage_contract" {
		t.Fatalf("guard=%#v, want source_record_paths lineage mismatch", guard)
	}
}

func TestApplyResolutionContractGuardAcceptsDerivedBaseFromResolutionSource(t *testing.T) {
	projections := ArtifactSchemaProjectionsWithTransitiveLineage([]ArtifactSchemaProjection{
		{
			ID:          "items_records",
			NodeClass:   ArtifactNodeClassRecord,
			Aliases:     []string{"items_records"},
			SourcePaths: []string{"items.csv"},
			JSONShape:   "array(len=3)",
			Fields:      []string{"raw_name", "qty", "unit_price"},
		},
		{
			ID:          "items_with_amount",
			NodeClass:   ArtifactNodeClassRecord,
			Aliases:     []string{"items_with_amount"},
			SourcePaths: []string{"items_records"},
			JSONShape:   "array(len=3)",
			Fields:      []string{"raw_name", "line_amount"},
		},
		{
			ID:                "name_resolutions",
			NodeClass:         ArtifactNodeClassArtifact,
			Aliases:           []string{"name_resolutions"},
			SourcePaths:       []string{"items_records", "canonical_records"},
			SourceRecordPaths: []string{"items_records"},
			ReferencePaths:    []string{"canonical_records"},
			JSONShape:         "array(len=3)",
			Fields:            []string{"item_id", "source_value", "canonical_id", "status"},
		},
	})
	action := dataquery.DataAction{
		ID:         "apply_names",
		Kind:       dataquery.DataActionApplyResolutions,
		InputPaths: []string{"items_with_amount", "name_resolutions"},
		Params: map[string]string{
			"base_path":       "items_with_amount",
			"resolution_path": "name_resolutions",
			"target_id_field": "canonical_name",
		},
	}
	if guard := ApplyResolutionContractGuardResult(ApplyResolutionContractGuardInput{Action: action, SchemaProjections: projections}); !guard.Empty() {
		t.Fatalf("guard=%#v, derived base shares the resolution source lineage and must pass", guard)
	}

	projections[1].SourcePaths = []string{"other_records"}
	projections = ArtifactSchemaProjectionsWithTransitiveLineage(projections)
	guard := ApplyResolutionContractGuardResult(ApplyResolutionContractGuardInput{Action: action, SchemaProjections: projections})
	if guard.Code != "apply_resolution_lineage_contract" {
		t.Fatalf("guard=%#v, unrelated derived base must remain rejected", guard)
	}
}
