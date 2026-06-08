package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildArtifactAccessViewsIncludesSchemaSamplesAndLineage(t *testing.T) {
	views := BuildArtifactAccessViews([]dataquery.DataArtifact{{
		ID:                "joined_rows",
		Kind:              string(dataquery.DataActionJoinRecords),
		SourcePaths:       []string{"left.csv", "right.csv"},
		SourceRecordPaths: []string{"left.csv"},
		ReferencePaths:    []string{"right.csv"},
		EvidencePaths:     []string{"left.csv:2"},
		Headers:           []string{"id", "amount", "status"},
		Fields: map[string]string{
			"artifact_aliases": "joined_rows.json,latest_join.json",
			"json_shape":       "array(len=2)",
		},
		Sample: []string{
			`{"id":"A1","amount":12.5,"status":"ok"}`,
			`{"id":"A2","amount":8,"status":"pending"}`,
		},
	}}, 4)
	if len(views) != 1 {
		t.Fatalf("view count=%d, want 1", len(views))
	}
	got := views[0]
	if got.NodeClass != ArtifactNodeClassRecord {
		t.Fatalf("NodeClass=%q, want record", got.NodeClass)
	}
	for _, alias := range []string{"joined_rows", "joined_rows.json", "latest_join.json"} {
		if !containsString(got.Aliases, alias) {
			t.Fatalf("aliases=%v, missing %q", got.Aliases, alias)
		}
	}
	if !containsString(got.Fields, "amount") || len(got.FieldSamples["amount"]) != 2 {
		t.Fatalf("fields=%v samples=%v, want amount samples", got.Fields, got.FieldSamples)
	}
	if !containsString(got.SourceRecordPaths, "left.csv") || !containsString(got.ReferencePaths, "right.csv") || !containsString(got.EvidencePaths, "left.csv:2") {
		t.Fatalf("view=%+v, want lineage roles", got)
	}
	if got.AccessHint == "" {
		t.Fatalf("AccessHint empty")
	}
}

func TestBuildArtifactAccessViewsDedupesButKeepsChildren(t *testing.T) {
	views := BuildArtifactAccessViews([]dataquery.DataArtifact{
		{
			ID:      "records",
			Headers: []string{"a"},
			Fields: map[string]string{
				"json_shape": "array(len=1)",
			},
		},
		{
			ID:      "records",
			Headers: []string{"a"},
			Fields: map[string]string{
				"json_shape": "array(len=1)",
			},
			Children: []dataquery.DataArtifact{{
				ID:      "records_diagnostic",
				Headers: []string{"field", "sample"},
				Fields: map[string]string{
					"json_shape": "array(len=1)",
				},
			}},
		},
	}, 4)
	if len(views) != 2 {
		t.Fatalf("view count=%d, want deduped parent plus child: %#v", len(views), views)
	}
	if views[0].ID != "records" || views[1].ID != "records_diagnostic" {
		t.Fatalf("views=%+v, want parent then child", views)
	}
}

func TestArtifactAccessViewsFromProjectionsBoundsPromptView(t *testing.T) {
	views := ArtifactAccessViewsFromProjections([]ArtifactSchemaProjection{{
		ID:                "artifact",
		Kind:              "derived",
		NodeClass:         ArtifactNodeClassRecord,
		Aliases:           []string{"artifact.json", "artifact-latest.json"},
		JSONShape:         "array(len=10)",
		Fields:            []string{"a", "b"},
		AccessHint:        "use json_records(alias)",
		SourcePaths:       []string{"source.csv"},
		SourceRecordPaths: []string{"source.csv"},
		ReferencePaths:    []string{"reference.csv"},
		EvidencePaths:     []string{"source.csv:2"},
	}}, 1)
	if len(views) != 1 {
		t.Fatalf("view count=%d, want 1", len(views))
	}
	got := views[0]
	if got.ID != "artifact" || got.AccessHint == "" || !containsString(got.Fields, "b") {
		t.Fatalf("view=%+v, want projection fields copied", got)
	}
	if len(got.FieldSamples) != 0 {
		t.Fatalf("projection view should not invent field samples: %+v", got.FieldSamples)
	}
}
