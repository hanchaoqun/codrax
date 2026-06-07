package dataworkflow

import (
	"slices"
	"strings"
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

func TestJoinRecordScaffoldsIgnoreInternalLineageFields(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:        "left",
			Kind:      string(dataquery.DataActionJoinRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"left.json"},
			JSONShape: "array(len=2,item=object(keys=_source,_left_index,value))",
			Fields:    []string{"_source", "_left_index", "value"},
		},
		{
			ID:        "right",
			Kind:      string(dataquery.DataActionJoinRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"right.json"},
			JSONShape: "array(len=2,item=object(keys=_source,_left_index,other))",
			Fields:    []string{"_source", "_left_index", "other"},
		},
	}

	if got := JoinRecordScaffolds(projections, 4); len(got) != 0 {
		t.Fatalf("JoinRecordScaffolds=%+v, want no join on internal lineage fields", got)
	}

	projections[1].Fields = []string{"_source", "_left_index", "value", "other"}
	got := JoinRecordScaffolds(projections, 4)
	if len(got) != 1 {
		t.Fatalf("JoinRecordScaffolds=%+v, want one join on ordinary field", got)
	}
	if strings.Join(got[0].CommonFields, ",") != "value" {
		t.Fatalf("CommonFields=%v, want only ordinary field value", got[0].CommonFields)
	}
}

func TestApplyResolutionScaffoldsIgnoreDiagnosticResolutionChildren(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:        "records",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"records.json"},
			JSONShape: "array(len=2,item=object(keys=item_id,name))",
			Fields:    []string{"item_id", "name"},
		},
		{
			ID:          "entity_mappings.json",
			Kind:        string(dataquery.DataActionNormalizeEntities),
			NodeClass:   ArtifactNodeClassArtifact,
			Aliases:     []string{"entity_mappings.json"},
			JSONShape:   "array(len=2,item=object(keys=item_id,source_value,canonical_id,canonical_label))",
			Fields:      []string{"item_id", "source_value", "canonical_id", "canonical_label"},
			SourcePaths: []string{"records.json", "reference.json"},
		},
		{
			ID:        "entity_mappings.json#entity_resolution_source",
			Kind:      "apply_entity_resolutions/resolution",
			NodeClass: ArtifactNodeClassDiagnosticChild,
			Aliases:   []string{"entity_mappings.json#entity_resolution_source"},
			JSONShape: "array(len=2,item=object(keys=item_id,source_value,canonical_id))",
			Fields:    []string{"item_id", "source_value", "canonical_id"},
		},
	}

	scaffolds := ApplyResolutionScaffolds(projections, 4)
	if len(scaffolds) != 1 {
		t.Fatalf("ApplyResolutionScaffolds=%+v, want exactly one scaffold from real ledger", scaffolds)
	}
	if strings.Join(scaffolds[0].InputPaths, ",") != "records.json,entity_mappings.json" {
		t.Fatalf("InputPaths=%v, want diagnostic child excluded", scaffolds[0].InputPaths)
	}
}

func TestPrioritizeConcreteScaffoldsUsesWorkflowStageFacts(t *testing.T) {
	scaffolds := []ActionScaffold{
		{Kind: string(dataquery.DataActionComputeContribs)},
		{Kind: string(dataquery.DataActionJoinRecords)},
		{Kind: string(dataquery.DataActionFilterRecords)},
		{Kind: string(dataquery.DataActionApplyResolutions)},
	}
	facts := StageFacts{
		MaterialCoverageSufficient: true,
		EntityStageMaterialized:    true,
		ContributionLedgerRequired: true,
	}
	got := PrioritizeConcreteScaffolds(scaffolds, facts)
	kinds := []string{got[0].Kind, got[1].Kind, got[2].Kind, got[3].Kind}
	want := []string{
		string(dataquery.DataActionApplyResolutions),
		string(dataquery.DataActionJoinRecords),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionComputeContribs),
	}
	if !slices.Equal(kinds, want) {
		t.Fatalf("kinds=%v, want %v", kinds, want)
	}

	facts.EntityStageMaterialized = false
	got = PrioritizeConcreteScaffolds(scaffolds, facts)
	kinds = []string{got[0].Kind, got[1].Kind, got[2].Kind, got[3].Kind}
	if !slices.Equal(kinds, []string{
		string(dataquery.DataActionComputeContribs),
		string(dataquery.DataActionJoinRecords),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionApplyResolutions),
	}) {
		t.Fatalf("kinds=%v, want original order before entity stage materializes", kinds)
	}
}
