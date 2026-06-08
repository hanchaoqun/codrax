package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestPlanActionSignatureEmptyPlan(t *testing.T) {
	if got := PlanActionSignature(dataquery.TaskPlan{}); got != "" {
		t.Fatalf("PlanActionSignature() = %q, want empty", got)
	}
	if PlanActionSignatureAlreadySeenInRecords([]WorkflowRecord{{Plan: dataquery.TaskPlan{}}}, dataquery.TaskPlan{}) {
		t.Fatal("empty candidate should not be treated as seen")
	}
}

func TestPlanActionSignatureNormalizesTypedActionFields(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		Kind:           "",
		InputPaths:     []string{" ./Rows.json ", "./Rows.json", "lookup.json"},
		OutputArtifact: "./Out.json",
		Params: map[string]string{
			"field": "amount",
		},
	}}}

	got := PlanActionSignature(plan)
	want := `{"actions":[{"kind":"custom_transform","input_paths":["./Rows.json","lookup.json"],"output_artifact":"out.json","params":{"field":"amount"}}]}`
	if got != want {
		t.Fatalf("PlanActionSignature() = %s, want %s", got, want)
	}
}

func TestPlanActionSignatureAlreadySeenInRecords(t *testing.T) {
	seen := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			Kind:           dataquery.DataActionFilterRecords,
			InputPaths:     []string{"records.json"},
			OutputArtifact: "filtered.json",
			Params: map[string]string{
				"filters_json": `[{"field":"status","op":"eq","value":"ready"}]`,
			},
		}}},
	}}
	candidate := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{" records.json "},
		OutputArtifact: "./filtered.json",
		Params: map[string]string{
			"filters_json": `[{"field":"status","op":"eq","value":"ready"}]`,
		},
	}}}

	if !PlanActionSignatureAlreadySeenInRecords(seen, candidate) {
		t.Fatal("candidate should be recognized as an already-seen typed action plan")
	}
	candidate.Actions[0].Params["filters_json"] = `[{"field":"status","op":"eq","value":"done"}]`
	if PlanActionSignatureAlreadySeenInRecords(seen, candidate) {
		t.Fatal("candidate with different typed params should not be recognized as seen")
	}
}
