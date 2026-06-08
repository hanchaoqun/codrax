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
	snapshot := view.Snapshot()
	if snapshot.DecisionFallbackReasonCode != StagePrepareContributionInputs || !snapshot.StageFacts.ContributionLedgerRequired {
		t.Fatalf("Snapshot=%+v, want contribution fallback and facts", snapshot)
	}
	view.WorkflowViolations[0].Code = "mutated"
	if snapshot.WorkflowViolations[0].Code != "field_contract_violation" {
		t.Fatalf("snapshot leaked view violation mutation: %+v", snapshot.WorkflowViolations)
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

func TestBuildWorkflowStateViewCentralizesCoverageStageAndDecision(t *testing.T) {
	current := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:             "filter",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "filtered.json",
	}}}
	contract := dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{{
			Path:      "input.csv",
			Required:  true,
			UsageMode: dataquery.MaterialUseScriptConsumed,
		}},
		ContributionLedgerRequired: true,
	}
	view := BuildWorkflowStateView(WorkflowStateViewBuildInput{
		Records: []WorkflowRecord{{
			Plan: current,
			Err:  `data planning incomplete: action 1 (filter) references field(s) [status] that are not present on input records.json fields [id]. Use an existing artifact.`,
		}},
		Current:              current,
		WorkflowContract:     contract,
		CurrentBatchContract: contract,
		CurrentBatchLayer:    CoverageLayerCurrentBatch,
		OutputContract:       dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
		CoveredMaterialPaths: map[string]bool{"./input.csv": true},
		LedgerCounts: WorkflowStateLedgerCounts{
			EntityStageMaterialized: true,
		},
		CustomTransformDisabledFunc: func(state WorkflowStateView) bool {
			if state.NextStage != StagePrepareContributionInputs {
				t.Fatalf("adapter saw NextStage=%q, want %q", state.NextStage, StagePrepareContributionInputs)
			}
			return true
		},
		DiagnosticArtifactAccess: []ArtifactAccessView{
			{ID: "records", Aliases: []string{"records.json"}, Fields: []string{"id"}},
			{ID: "enriched", Aliases: []string{"enriched.json"}, Fields: []string{"id", "status"}},
		},
		ArtifactLimit:    8,
		ProgressLimit:    4,
		ActionEventLimit: 8,
	})
	if !view.MaterialCoverageSufficient || view.MissingRequiredMaterialCount != 0 || len(view.CoveredRequiredMaterials) != 1 {
		t.Fatalf("coverage view=%+v, want covered material floor", view)
	}
	if view.NextStage != StagePrepareContributionInputs || view.Decision.Status != "blocked" {
		t.Fatalf("stage/decision=%q/%+v, want blocked contribution stage", view.NextStage, view.Decision)
	}
	if stringSliceContains(view.AllowedNextActions, string(dataquery.DataActionCustomTransform)) {
		t.Fatalf("AllowedNextActions=%v, custom_transform should be filtered by state callback", view.AllowedNextActions)
	}
	if len(view.ActionGraph.Blocked) == 0 || view.ActionGraph.Blocked[0].ID != "filter" {
		t.Fatalf("action graph blocked=%+v, want blocked filter action", view.ActionGraph.Blocked)
	}
	if len(view.WorkflowViolations) == 0 || view.WorkflowViolations[0].Code != "field_contract_violation" {
		t.Fatalf("workflow violations=%+v, want field contract violation", view.WorkflowViolations)
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

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
