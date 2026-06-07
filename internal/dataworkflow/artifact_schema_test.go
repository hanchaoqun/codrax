package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestProjectArtifactSchemasPreservesFullFields(t *testing.T) {
	headers := make([]string, 0, 48)
	for i := 0; i < 40; i++ {
		headers = append(headers, "field_"+string(rune('a'+(i%26))))
	}
	headers = append(headers, "required_field")
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:      "joined_rows",
		Kind:    "record_artifact",
		Headers: headers,
		Fields: map[string]string{
			"json_shape": "array(len=10)",
		},
	}})
	if len(projections) != 1 {
		t.Fatalf("projection count = %d, want 1", len(projections))
	}
	if !containsString(projections[0].Fields, "required_field") {
		t.Fatalf("full contract fields dropped required_field: %#v", projections[0].Fields)
	}
}

func TestProjectArtifactSchemasPreservesLineageRoles(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:                "mapping",
		Kind:              string(dataquery.DataActionNormalizeEntities),
		SourcePaths:       []string{"reference.csv", "items.csv"},
		SourceRecordPaths: []string{"items.csv"},
		ReferencePaths:    []string{"reference.csv"},
		EvidencePaths:     []string{"items.csv:2"},
		Fields: map[string]string{
			"json_shape": "array(len=2)",
		},
	}})
	if len(projections) != 1 {
		t.Fatalf("projection count = %d, want 1", len(projections))
	}
	if got := projections[0]; !containsString(got.SourceRecordPaths, "items.csv") || !containsString(got.ReferencePaths, "reference.csv") || !containsString(got.EvidencePaths, "items.csv:2") {
		t.Fatalf("projection=%+v, want source/reference/evidence lineage roles", got)
	}
}

func TestProjectArtifactSchemasNewestFirstDedupesByAlias(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{
		{
			ID:      "new_artifact",
			Kind:    "record_artifact",
			Headers: []string{"new_field"},
			Fields: map[string]string{
				"artifact_aliases": "shared_alias",
				"json_shape":       "array(len=1)",
			},
		},
		{
			ID:      "old_artifact",
			Kind:    "record_artifact",
			Headers: []string{"old_field"},
			Fields: map[string]string{
				"artifact_aliases": "shared_alias",
				"json_shape":       "array(len=1)",
			},
		},
	})
	if len(projections) != 1 {
		t.Fatalf("projection count = %d, want 1", len(projections))
	}
	if projections[0].ID != "new_artifact" {
		t.Fatalf("kept %q, want newest artifact", projections[0].ID)
	}
	if !containsString(projections[0].Fields, "new_field") {
		t.Fatalf("kept projection does not expose newest fields: %#v", projections[0].Fields)
	}
}

func TestProjectArtifactSchemasIncludesChildren(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:   "parent",
		Kind: "summary",
		Fields: map[string]string{
			"json_shape": "object",
		},
		Children: []dataquery.DataArtifact{{
			ID:      "child_records",
			Kind:    "record_artifact",
			Headers: []string{"child_field"},
			Fields: map[string]string{
				"json_shape": "array(len=2)",
			},
		}},
	}})
	if len(projections) != 2 {
		t.Fatalf("projection count = %d, want 2", len(projections))
	}
	if projections[1].ID != "child_records" {
		t.Fatalf("child projection missing or out of order: %#v", projections)
	}
	if !containsString(projections[1].Fields, "child_field") {
		t.Fatalf("child fields missing: %#v", projections[1].Fields)
	}
}

func TestProjectArtifactSchemasClassifiesWorkflowLedgerAndDiagnosticChildren(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:   "workflow_entity_resolutions",
		Kind: "workflow_ledger/entity_resolutions",
		Fields: map[string]string{
			"workflow_ledger": "true",
			"json_shape":      "array(len=2)",
		},
		Children: []dataquery.DataArtifact{{
			ID:      "records.json#base",
			Kind:    "apply_entity_resolutions/base",
			Headers: []string{"id"},
			Fields:  map[string]string{"json_shape": "array(len=2)"},
		}},
	}})
	if len(projections) != 2 {
		t.Fatalf("projection count=%d, want workflow ledger plus child", len(projections))
	}
	if projections[0].NodeClass != ArtifactNodeClassWorkflowLedger {
		t.Fatalf("workflow ledger node_class=%q", projections[0].NodeClass)
	}
	if projections[1].NodeClass != ArtifactNodeClassDiagnosticChild {
		t.Fatalf("diagnostic child node_class=%q", projections[1].NodeClass)
	}
}

func TestProjectArtifactSchemasClassifiesEntityResolutionSourceChildrenAsDiagnostic(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:      "entity_mappings.json",
		Kind:    string(dataquery.DataActionNormalizeEntities),
		Headers: []string{"item_id", "source_value", "canonical_id", "canonical_label"},
		Fields:  map[string]string{"json_shape": "array(len=2)"},
		Children: []dataquery.DataArtifact{{
			ID:      "records.json#entity_resolution_source",
			Kind:    "apply_entity_resolutions/resolution",
			Headers: []string{"item_id", "source_value", "canonical_id"},
			Fields:  map[string]string{"json_shape": "array(len=2)"},
		}, {
			ID:      "records.json#entity_resolutions",
			Kind:    "entity_resolution_source",
			Headers: []string{"source_field", "canonical_id_field"},
			Fields:  map[string]string{"json_shape": "array(len=2)"},
		}},
	}})
	if len(projections) != 3 {
		t.Fatalf("projection count=%d, want parent plus two children", len(projections))
	}
	for _, projection := range projections[1:] {
		if projection.NodeClass != ArtifactNodeClassDiagnosticChild {
			t.Fatalf("%s node_class=%q, want diagnostic_child", projection.ID, projection.NodeClass)
		}
		if ArtifactUsableForRecordAction(projection) {
			t.Fatalf("%s should not be usable as record action input", projection.ID)
		}
	}
}

func TestMissingFieldsOnArtifactSchemaUsesAliasContract(t *testing.T) {
	projections := []ArtifactSchemaProjection{{
		ID:      "records",
		Aliases: []string{"records.json", "/tmp/work/records.json"},
		Fields:  []string{"id", "amount", "status"},
	}}

	missing := MissingFieldsOnArtifactSchema(projections, "records.json", []string{"amount", "currency", "status", "currency"})
	if len(missing) != 1 || missing[0] != "currency" {
		t.Fatalf("missing=%v, want currency only", missing)
	}
	if _, ok := ArtifactSchemaByAlias(projections, "/tmp/work/records.json"); !ok {
		t.Fatalf("expected path alias to resolve")
	}
	if missing := MissingFieldsOnArtifactSchema(projections, "unknown.json", []string{"amount"}); len(missing) != 0 {
		t.Fatalf("unknown schema should not hard fail, missing=%v", missing)
	}
}

func TestArtifactUsableForRecordActionUsesNodeClassAndKind(t *testing.T) {
	if !ArtifactUsableForRecordAction(ArtifactSchemaProjection{
		ID:        "records",
		Kind:      string(dataquery.DataActionExtractRecords),
		NodeClass: ArtifactNodeClassRecord,
		Fields:    []string{"id"},
		JSONShape: "array(len=1)",
	}) {
		t.Fatalf("record artifact should be usable for record actions")
	}
	if ArtifactUsableForRecordAction(ArtifactSchemaProjection{
		ID:        "records#base",
		Kind:      string(dataquery.DataActionApplyResolutions) + "/base",
		NodeClass: ArtifactNodeClassDiagnosticChild,
		Fields:    []string{"id"},
		JSONShape: "array(len=1)",
	}) {
		t.Fatalf("diagnostic child should not be usable for record actions")
	}
	if ArtifactUsableForRecordAction(ArtifactSchemaProjection{
		ID:        "workflow_contributions",
		Kind:      "workflow_ledger/contributions",
		NodeClass: ArtifactNodeClassWorkflowLedger,
		Fields:    []string{"group_key"},
		JSONShape: "array(len=1)",
	}) {
		t.Fatalf("workflow ledger should not be used as an ordinary record action base")
	}
	if ArtifactUsableForRecordAction(ArtifactSchemaProjection{
		ID:        "rules",
		Kind:      string(dataquery.DataActionDeriveRules),
		NodeClass: ArtifactNodeClassArtifact,
		Fields:    []string{"rule_id"},
	}) {
		t.Fatalf("derive_rules artifact should not be treated as record-action input")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
