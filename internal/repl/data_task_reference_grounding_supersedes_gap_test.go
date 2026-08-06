package repl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
)

func TestDataTaskOutputGraphGroundedReferenceSupersedesEarlierCandidateGap(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "targets.csv"), []byte("target_id,canonical_label\nT1,GroupA\nT2,GroupX\nT3,GroupC\n"), 0600); err != nil {
		t.Fatal(err)
	}
	current := dataquery.TaskPlan{
		InputPaths: []string{"contributions_by_canonical.json", "targets.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
			Delimiter:          ",",
		},
		Actions: []dataquery.DataAction{{
			ID:         "assemble_final_output",
			Kind:       dataquery.DataActionAssembleAnswer,
			InputPaths: []string{"contributions_by_canonical.json", "targets.csv"},
			Params: map[string]string{
				"projection":          "values",
				"order_by":            "reference",
				"reference_path":      "targets.csv",
				"reference_key_field": "canonical_label",
				"delimiter":           ",",
			},
		}},
	}
	result := dataquery.Result{
		Answer:         "17,0,5",
		OutputContract: current.OutputContract,
		Contributions: []dataquery.ContributionRecord{
			{ItemID: dataquery.LooseText("r1"), GroupKey: dataquery.LooseText("GroupA"), Metric: dataquery.LooseText("value"), Value: dataquery.LooseText("17"), Operation: dataquery.LooseText("add"), Role: dataquery.LooseText("target")},
			{ItemID: dataquery.LooseText("r2"), GroupKey: dataquery.LooseText("GroupB"), Metric: dataquery.LooseText("value"), Value: dataquery.LooseText("4"), Operation: dataquery.LooseText("add"), Role: dataquery.LooseText("target")},
			{ItemID: dataquery.LooseText("r3"), GroupKey: dataquery.LooseText("GroupC"), Metric: dataquery.LooseText("value"), Value: dataquery.LooseText("5"), Operation: dataquery.LooseText("add"), Role: dataquery.LooseText("target")},
		},
		Reconcile: &dataquery.ReconcileReport{Status: dataquery.LooseText("pass"), Groups: []dataquery.ReconcileGroup{
			{GroupKey: dataquery.LooseText("GroupA"), Metric: dataquery.LooseText("value"), Expected: dataquery.LooseText("17"), Actual: dataquery.LooseText("17")},
			{GroupKey: dataquery.LooseText("GroupB"), Metric: dataquery.LooseText("value"), Expected: dataquery.LooseText("4"), Actual: dataquery.LooseText("4")},
			{GroupKey: dataquery.LooseText("GroupC"), Metric: dataquery.LooseText("value"), Expected: dataquery.LooseText("5"), Actual: dataquery.LooseText("5")},
		}},
		Artifacts: []dataquery.DataArtifact{
			{
				ID:       "contributions_by_canonical.json",
				Kind:     string(dataquery.DataActionComputeContribs),
				Headers:  []string{"group_key", "value"},
				Sample:   []string{`{"group_key":"GroupA","value":"17"}`, `{"group_key":"GroupB","value":"4"}`, `{"group_key":"GroupC","value":"5"}`},
				RowCount: 3,
			},
			{
				ID:   "final_answer",
				Kind: string(dataquery.DataActionAssembleAnswer),
				Fields: map[string]string{
					"projection":          "values",
					"delimiter":           ",",
					"value_field":         "actual",
					"reference_projected": "true",
					"reference_path":      "targets.csv",
					"reference_key_field": "canonical_label",
					"reference_key_count": "3",
				},
			},
		},
	}

	input := dataTaskWorkflowCompletionOutputProjectionGraphInput(root, nil, current, result)
	if !input.ReferenceGroundingEvaluated || input.ReferenceGroundingMismatch {
		t.Fatalf("input=%+v, want successful typed reference grounding", input)
	}
	if input.ReferenceGapPresent {
		t.Fatalf("input=%+v, successful grounding must clear earlier candidate gap", input)
	}
	graph := dataworkflow.BuildOutputProjectionGraph(input)
	if graph.Status != dataworkflow.OutputProjectionStatusSatisfied || !graph.ReferenceComplete {
		t.Fatalf("graph=%+v, want grounded complete output", graph)
	}
	if guard := dataTaskWorkflowCompletionGateGuardResultWithRepo(root, nil, current, result); !guard.Empty() {
		t.Fatalf("guard=%+v, grounded correct reference answer must complete", guard)
	}
}
