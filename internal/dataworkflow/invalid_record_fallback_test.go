package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildInvalidRecordMaterializationFallbackPlan(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:             "derive_status",
		Kind:           dataquery.DataActionDeriveFields,
		InputPaths:     []string{"rules.md", "orders.csv"},
		OutputArtifact: "derived_orders.json",
	}}}

	got, ok := BuildInvalidRecordMaterializationFallbackPlan(InvalidRecordMaterializationFallbackInput{
		Current:      plan,
		ExtractLimit: 42,
	})
	if !ok {
		t.Fatal("BuildInvalidRecordMaterializationFallbackPlan ok=false")
	}
	if len(got.Actions) != 1 || got.Actions[0].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("fallback actions = %#v, want extract_records", got.Actions)
	}
	if got.Actions[0].Params["limit"] != "42" || got.Actions[0].Params["max_records"] != "42" {
		t.Fatalf("fallback params = %#v, want bounded extract limits", got.Actions[0].Params)
	}
	if !got.ContinueAfter || got.Script != "" {
		t.Fatalf("fallback ContinueAfter=%v Script=%q, want typed continuation without script", got.ContinueAfter, got.Script)
	}
}

func TestBuildInvalidRecordMaterializationFallbackPlanSkipsExecutableSingleInputSpec(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		Kind:       dataquery.DataActionExtractFields,
		InputPaths: []string{"records.json"},
		Params: map[string]string{
			"field_specs": `[{"source_field":"raw","target_field":"value","operation":"trim"}]`,
		},
	}}}

	if ActionNeedsRecordMaterialization(plan.Actions[0]) {
		t.Fatal("single-input field action with specs should not require materialization fallback")
	}
	if _, ok := BuildInvalidRecordMaterializationFallbackPlan(InvalidRecordMaterializationFallbackInput{Current: plan}); ok {
		t.Fatal("materialization fallback should not be emitted")
	}
}

func TestBuildInvalidRecordMaterializationFallbackPlanDefersWhenPostCoverageGraphStage(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		Kind:       dataquery.DataActionDeriveFields,
		InputPaths: []string{"base.json", "mapping.json"},
	}}}

	_, ok := BuildInvalidRecordMaterializationFallbackPlan(InvalidRecordMaterializationFallbackInput{
		Current: plan,
		State: WorkflowStateView{
			MaterialCoverageSufficient: true,
			NextStage:                  StageComputeContributions,
		},
	})
	if ok {
		t.Fatal("post-coverage graph stage should leave recovery to enrichment/field-contract fallbacks")
	}
}
