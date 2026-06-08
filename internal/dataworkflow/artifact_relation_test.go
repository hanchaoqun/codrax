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
