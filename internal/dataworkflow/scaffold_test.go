package dataworkflow

import (
	"slices"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestRelationActionScaffoldsUseArtifactSchemaProjection(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:        "base",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"base.json"},
			JSONShape: "array(len=2,item=object(keys=id,name))",
			Fields:    []string{"id", "name"},
		},
		{
			ID:        "lookup",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"lookup.json"},
			JSONShape: "array(len=2,item=object(keys=id,label))",
			Fields:    []string{"id", "label"},
		},
		{
			ID:        "diag",
			NodeClass: ArtifactNodeClassDiagnosticChild,
			Aliases:   []string{"diag.json"},
			Fields:    []string{"id", "debug"},
		},
	}

	scaffolds := RelationActionScaffolds(projections, []string{
		string(dataquery.DataActionEnrichRecords),
		string(dataquery.DataActionJoinRecords),
	}, 8)
	var kinds []string
	for _, scaffold := range scaffolds {
		kinds = append(kinds, scaffold.Kind)
		for _, input := range scaffold.InputPaths {
			if input == "diag.json" {
				t.Fatalf("diagnostic artifact used as relation input: %+v", scaffold)
			}
		}
	}
	if !slices.Contains(kinds, string(dataquery.DataActionEnrichRecords)) {
		t.Fatalf("scaffolds=%+v, want enrich_records scaffold", scaffolds)
	}
	if !slices.Contains(kinds, string(dataquery.DataActionJoinRecords)) {
		t.Fatalf("scaffolds=%+v, want join_records scaffold", scaffolds)
	}
}
