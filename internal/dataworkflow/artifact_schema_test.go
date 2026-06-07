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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
