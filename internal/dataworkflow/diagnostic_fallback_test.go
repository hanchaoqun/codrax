package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestGeneratedSchemaDiagnosticInputsPreferPlanReferencedRecordArtifacts(t *testing.T) {
	artifacts := []ArtifactSchemaProjection{
		{
			ID:        "older",
			Kind:      string(dataquery.DataActionInspectMaterial),
			Aliases:   []string{"older.json"},
			JSONShape: "array",
			Fields:    []string{"id"},
		},
		{
			ID:        "joined",
			Kind:      string(dataquery.DataActionJoinRecords),
			Aliases:   []string{"joined.json"},
			JSONShape: "array",
			Fields:    []string{"id", "amount", "status"},
		},
		{
			ID:        "derived",
			Kind:      string(dataquery.DataActionDeriveFields),
			Aliases:   []string{"derived.json"},
			JSONShape: "array",
			Fields:    []string{"id", "status"},
		},
	}
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		Kind:       dataquery.DataActionFilterRecords,
		InputPaths: []string{"derived.json"},
	}}}

	got := GeneratedSchemaDiagnosticInputs(GeneratedSchemaDiagnosticInput{
		Current:   plan,
		Artifacts: artifacts,
		Limit:     2,
	})
	if len(got) != 2 || got[0] != "derived.json" || got[1] != "joined.json" {
		t.Fatalf("GeneratedSchemaDiagnosticInputs() = %#v, want derived then joined", got)
	}
}

func TestBuildGeneratedSchemaDiagnosticFallbackPlanSuppressesSeenPlan(t *testing.T) {
	current := dataquery.TaskPlan{
		Goal: "continue typed workflow",
		Actions: []dataquery.DataAction{{
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"records.json"},
		}},
	}
	artifacts := []ArtifactSchemaProjection{{
		ID:        "records",
		Kind:      string(dataquery.DataActionExtractRecords),
		Aliases:   []string{"records.json"},
		JSONShape: "array",
		Fields:    []string{"id", "value"},
	}}
	plan, ok := BuildGeneratedSchemaDiagnosticFallbackPlan(GeneratedSchemaDiagnosticFallbackInput{
		Current:   current,
		Artifacts: artifacts,
		Limit:     3,
	})
	if !ok {
		t.Fatal("BuildGeneratedSchemaDiagnosticFallbackPlan ok=false")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionInspectMaterial {
		t.Fatalf("fallback action = %#v, want inspect_material", plan.Actions)
	}
	if got := plan.Actions[0].InputPaths; len(got) != 1 || got[0] != "records.json" {
		t.Fatalf("fallback inputs = %#v, want records.json", got)
	}

	_, ok = BuildGeneratedSchemaDiagnosticFallbackPlan(GeneratedSchemaDiagnosticFallbackInput{
		Current:   current,
		Artifacts: artifacts,
		Records:   []WorkflowRecord{{Plan: plan}},
		Limit:     3,
	})
	if ok {
		t.Fatal("seen diagnostic fallback should not be rebuilt")
	}
}
