package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildWorkflowStateDiagnosticsProjectsTypedIssues(t *testing.T) {
	got := BuildWorkflowStateDiagnostics(WorkflowStateDiagnosticsInput{
		State: WorkflowStateView{
			AllowedNextActions:         []string{string(dataquery.DataActionDeriveFields), string(dataquery.DataActionFilterRecords)},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
			EntityResolutionRequired:   true,
		},
		Records: []WorkflowRecord{{
			Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
				ID:         "compute",
				Kind:       dataquery.DataActionComputeContribs,
				InputPaths: []string{"joined"},
			}}},
			Err: `data planning incomplete: action 1 (compute) references field(s) [amount, status] that are not present on input joined fields [id, name]. Use an existing artifact.`,
		}},
		ArtifactAccess: []ArtifactAccessView{
			{ID: "joined", Aliases: []string{"joined"}, Fields: []string{"id", "name"}},
			{ID: "enriched", Aliases: []string{"enriched"}, Fields: []string{"id", "amount", "status"}},
		},
		DiagnosticBatches: []ArtifactDiagnosticBatch{{
			Round: 2,
			Artifacts: []dataquery.DataArtifact{
				{
					ID:   "filtered",
					Kind: string(dataquery.DataActionFilterRecords),
					Fields: map[string]string{
						"artifact_aliases":   "filtered,filtered.json",
						"input_path":         "records.json",
						"input_rows":         "10",
						"output_rows":        "0",
						"filter_fields":      "status",
						"filter_diagnostics": `{"total":10,"combined_match":0}`,
					},
				},
				{
					ID:       "resolved",
					Kind:     string(dataquery.DataActionApplyResolutions),
					RowCount: 2,
					Fields: map[string]string{
						"artifact_aliases":          "resolved,resolved.json",
						"base_path":                 "records.json",
						"base_rows":                 "2",
						"output_rows":               "2",
						"target_fields":             "canonical_id",
						"matched_canonical_id":      "0",
						"unmatched_canonical_id":    "2",
						"unmatched_canonical_label": "2",
					},
				},
				{
					ID:   "qualified",
					Kind: string(dataquery.DataActionQualifyRecords),
					Fields: map[string]string{
						"artifact_aliases":      "qualified,qualified.json",
						"input_path":            "records.json",
						"input_rows":            "12",
						"eligible_rows":         "0",
						"include_filters":       `{"status":"eligible"}`,
						"qualification_reasons": "all rows failed the declared eligibility contract",
					},
				},
			},
		}},
		Limit: 4,
	})
	if len(got.FieldContractViolations) != 1 {
		t.Fatalf("field issues=%+v, want one", got.FieldContractViolations)
	}
	if len(got.ZeroMatchFilterViolations) != 1 {
		t.Fatalf("zero-match issues=%+v, want one", got.ZeroMatchFilterViolations)
	}
	if len(got.UnmatchedResolutionViolations) != 1 {
		t.Fatalf("unmatched issues=%+v, want one", got.UnmatchedResolutionViolations)
	}
	if len(got.ZeroEligibleViolations) != 1 {
		t.Fatalf("zero-eligible issues=%+v, want one", got.ZeroEligibleViolations)
	}
}

func TestBuildWorkflowStateDiagnosticsSkipsDownstreamIssuesAfterContributions(t *testing.T) {
	got := BuildWorkflowStateDiagnostics(WorkflowStateDiagnosticsInput{
		State: WorkflowStateView{
			ContributionLedgerRequired: true,
			ContributionRecords:        1,
		},
		DiagnosticBatches: []ArtifactDiagnosticBatch{{
			Round: 1,
			Artifacts: []dataquery.DataArtifact{{
				ID:   "filtered",
				Kind: string(dataquery.DataActionFilterRecords),
				Fields: map[string]string{
					"input_rows":  "10",
					"output_rows": "0",
				},
			}},
		}},
		Limit: 4,
	})
	if len(got.ZeroMatchFilterViolations) != 0 || len(got.ZeroEligibleViolations) != 0 {
		t.Fatalf("diagnostics=%+v, downstream issues should be skipped after contributions", got)
	}
}
