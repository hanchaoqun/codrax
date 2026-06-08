package dataworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestWorkflowStateSnapshotDerivesStageAndAllowedActions(t *testing.T) {
	snapshot := WorkflowStateSnapshot{
		StageFacts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
			EntityStageMaterialized:    true,
		},
	}
	if snapshot.NextStage() != StagePrepareContributionInputs {
		t.Fatalf("NextStage=%q, want %q", snapshot.NextStage(), StagePrepareContributionInputs)
	}
	kinds := strings.Join(snapshot.AllowedNextActions(), ",")
	for _, want := range []string{
		string(dataquery.DataActionDeriveFields),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionComputeContribs),
	} {
		if !strings.Contains(kinds, want) {
			t.Fatalf("AllowedNextActions=%q, want %q", kinds, want)
		}
	}
}

func TestWorkflowStateViewOwnsWorkflowStateJSONSchema(t *testing.T) {
	view := WorkflowStateView{
		MaterialCoverageSufficient: true,
		WorkflowContract: CoverageContractView{
			Layer:                      CoverageLayerWorkflow,
			RequiredRunnerInputPaths:   []string{"input.csv"},
			ContributionLedgerRequired: true,
		},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
		AllowedNextActions: []string{
			string(dataquery.DataActionFilterRecords),
		},
		ActionScaffold: []ActionScaffold{{
			Kind:       string(dataquery.DataActionFilterRecords),
			Executable: true,
		}},
		WorkflowViolations: []WorkflowViolation{{
			Code: "field_contract_violation",
		}},
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal workflow state view: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"material_coverage_sufficient", "workflow_contract", "output_contract", "allowed_next_actions", "action_scaffold", "workflow_violations"} {
		if !strings.Contains(text, want) {
			t.Fatalf("workflow state view JSON missing %q: %s", want, text)
		}
	}
	if view.Facts().ContributionLedgerRequired != true || view.ComputedNextStage() != StagePrepareContributionInputs {
		t.Fatalf("workflow state view facts/stage=%+v/%s, want contribution stage", view.Facts(), view.ComputedNextStage())
	}
	if got := strings.Join(view.MissingValidationStages(), ","); !strings.Contains(got, "contribution_ledger") {
		t.Fatalf("MissingValidationStages=%q, want contribution_ledger", got)
	}
	if got := strings.Join(view.ComputedAllowedNextActions(), ","); !strings.Contains(got, string(dataquery.DataActionFilterRecords)) {
		t.Fatalf("ComputedAllowedNextActions=%q, want explicit allowed action", got)
	}
}

func TestBuildWorkflowStateSnapshotReducesGraphsAndDecision(t *testing.T) {
	ready := dataquery.DataAction{
		ID:             "compute",
		Kind:           dataquery.DataActionComputeContribs,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "contribs.json",
	}
	snapshot := BuildWorkflowStateSnapshot(WorkflowStateSnapshotInput{
		StageFacts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
		},
		ActionGraph: ActionGraphInput{
			Ready:      []dataquery.DataAction{ready},
			EventLimit: 8,
		},
		OutputGraph: OutputProjectionGraphInput{
			Output: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
			Coverage: dataquery.CoverageContract{
				ContributionLedgerRequired: true,
			},
		},
		Artifacts: []ArtifactProjectionSource{{
			ID:       "records",
			Kind:     "record_artifact",
			Headers:  []string{"group", "value"},
			Fields:   map[string]string{"json_shape": "array"},
			RowCount: 2,
		}},
		ProgressEvents: []ProgressEvent{{
			Round:                   1,
			ResultPresent:           true,
			ContributionRecords:     0,
			RuleCoverageRecords:     0,
			EntityResolutionRecords: 0,
		}},
	})
	if snapshot.LedgerGraph.NextStage != StagePrepareContributionInputs || snapshot.LedgerGraph.FirstMissing != string(LedgerContributions) {
		t.Fatalf("ledger graph=%+v, want missing contributions", snapshot.LedgerGraph)
	}
	if len(snapshot.ActionGraph.Ready) != 1 || snapshot.ActionGraph.Ready[0].ID != "compute" {
		t.Fatalf("action graph ready=%+v, want ready compute", snapshot.ActionGraph.Ready)
	}
	if snapshot.ArtifactGraph.NodeCount != 1 || len(snapshot.ArtifactGraph.Nodes) != 1 {
		t.Fatalf("artifact graph=%+v, want one node", snapshot.ArtifactGraph)
	}
	if snapshot.OutputGraph.Status == "" {
		t.Fatalf("output graph=%+v, want structured output status", snapshot.OutputGraph)
	}
	if snapshot.Decision.Status != "continue" || !strings.Contains(strings.Join(snapshot.Decision.NextActions, ","), string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("decision=%+v, want continue with contribution action", snapshot.Decision)
	}
}

func TestBuildWorkflowReducerSnapshotDerivesRecordInputs(t *testing.T) {
	current := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:             "next",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "filtered.json",
	}}}
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "extract",
			Kind: dataquery.DataActionExtractRecords,
		}}},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{{
				ID:       "records",
				Kind:     "record_artifact",
				Headers:  []string{"status", "value"},
				Fields:   map[string]string{"json_shape": "array"},
				RowCount: 3,
			}},
			Rows: []dataquery.RowDecision{{RowID: "row1"}},
		},
	}}
	snapshot := BuildWorkflowReducerSnapshot(WorkflowReducerInput{
		Records: records,
		Current: current,
		StageFacts: StageFacts{
			MaterialCoverageSufficient: true,
			DecisionRecordsRequired:    true,
			DecisionRecords:            1,
		},
		OutputGraph: OutputProjectionGraphInput{
			Output: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
		},
		ActionEventLimit: 8,
		ArtifactLimit:    8,
		ProgressLimit:    8,
	})
	if len(snapshot.ActionGraph.Executed) != 1 || snapshot.ActionGraph.Executed[0].ID != "extract" {
		t.Fatalf("executed=%+v, want record-derived extract", snapshot.ActionGraph.Executed)
	}
	if len(snapshot.ActionGraph.Ready) != 1 || snapshot.ActionGraph.Ready[0].ID != "next" {
		t.Fatalf("ready=%+v, want current action", snapshot.ActionGraph.Ready)
	}
	if snapshot.ArtifactGraph.NodeCount != 1 || snapshot.Progress.Latest.ArtifactRows != 3 || snapshot.Progress.Latest.DecisionRecords != 1 {
		t.Fatalf("snapshot artifacts/progress=%+v / %+v, want record-derived state", snapshot.ArtifactGraph, snapshot.Progress)
	}
}
