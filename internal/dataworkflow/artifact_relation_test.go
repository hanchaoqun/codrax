package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestArtifactGraphStateIncludesStructuralRelations(t *testing.T) {
	state := ArtifactGraphStateFromProjections([]ArtifactSchemaProjection{
		{
			ID:        "orders",
			Kind:      string(dataquery.DataActionFilterRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"orders.json"},
			Fields:    []string{"order_id", "status"},
		},
		{
			ID:        "lookup",
			Kind:      string(dataquery.DataActionNormalizeEntities),
			NodeClass: ArtifactNodeClassArtifact,
			Aliases:   []string{"lookup.json"},
			Fields:    []string{"order_id", "status_code", "status_label"},
		},
	}, 10)
	if state.RelationCount == 0 || len(state.Relations) == 0 {
		t.Fatalf("relations=%+v, want structural relation", state.Relations)
	}
	got := state.Relations[0]
	if got.BaseAlias != "orders.json" || got.LookupAlias != "lookup.json" {
		t.Fatalf("relation=%+v, want orders -> lookup", got)
	}
	if len(got.BaseFields) != 1 || got.BaseFields[0] != "order_id" ||
		len(got.LookupFields) != 1 || got.LookupFields[0] != "order_id" {
		t.Fatalf("relation fields=%+v/%+v, want order_id pair", got.BaseFields, got.LookupFields)
	}
	if !containsString(got.LookupValueFields, "status_code") || !containsString(got.LookupValueFields, "status_label") {
		t.Fatalf("lookup value fields=%+v, want lookup non-key fields", got.LookupValueFields)
	}
}

func TestArtifactGraphStateIncludesTypedMetadataRelation(t *testing.T) {
	state := ArtifactGraphStateFromProjections([]ArtifactSchemaProjection{
		{
			ID:        "base",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"base_records"},
			Fields:    []string{"id", "raw_name", "amount"},
		},
		{
			ID:                "resolution",
			Kind:              string(dataquery.DataActionNormalizeEntities),
			NodeClass:         ArtifactNodeClassArtifact,
			Aliases:           []string{"entity_resolution"},
			SourceRecordPaths: []string{"base_records"},
			Fields:            []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
			Diagnostics: map[string]string{
				"source_fields": `["raw_name"]`,
			},
		},
	}, 10)
	if state.RelationCount == 0 || len(state.Relations) == 0 {
		t.Fatalf("relations=%+v, want typed metadata relation", state.Relations)
	}
	got := state.Relations[0]
	if got.BaseAlias != "base_records" || got.LookupAlias != "entity_resolution" {
		t.Fatalf("relation=%+v, want base_records -> entity_resolution", got)
	}
	if len(got.BaseFields) != 1 || got.BaseFields[0] != "raw_name" ||
		len(got.LookupFields) != 1 || got.LookupFields[0] != "source_value" {
		t.Fatalf("relation fields=%+v/%+v, want raw_name -> source_value", got.BaseFields, got.LookupFields)
	}
	if !containsString(got.LookupValueFields, "canonical_id") || !containsString(got.LookupValueFields, "canonical_label") {
		t.Fatalf("lookup value fields=%+v, want canonical fields", got.LookupValueFields)
	}
	if !containsString(got.Evidence, "typed relation metadata") {
		t.Fatalf("evidence=%+v, want typed relation metadata", got.Evidence)
	}
}

func TestArtifactGraphRelationsExcludeDiagnosticArtifacts(t *testing.T) {
	state := ArtifactGraphStateFromProjections([]ArtifactSchemaProjection{
		{
			ID:        "orders",
			Kind:      string(dataquery.DataActionFilterRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"orders.json"},
			Fields:    []string{"order_id", "status"},
		},
		{
			ID:        "diagnostic",
			Kind:      "diagnostic",
			NodeClass: ArtifactNodeClassDiagnosticChild,
			Aliases:   []string{"diagnostic.json"},
			Fields:    []string{"order_id", "status_code"},
		},
	}, 10)
	if len(state.Relations) != 0 {
		t.Fatalf("relations=%+v, diagnostic artifacts must not become lookup relations", state.Relations)
	}
}
