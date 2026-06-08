package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestDiscoverFieldContractIssuesFromTypedGuardError(t *testing.T) {
	issues := DiscoverFieldContractIssues(FieldContractIssueInput{
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
		AllowedNextActions: []string{string(dataquery.DataActionDeriveFields), string(dataquery.DataActionJoinRecords)},
		Limit:              4,
	})
	if len(issues) != 1 {
		t.Fatalf("issue count=%d, want 1: %+v", len(issues), issues)
	}
	got := issues[0]
	if got.ActionID != "compute" || got.InputAlias != "joined" || !containsString(got.MissingFields, "amount") || !containsString(got.MissingFields, "status") {
		t.Fatalf("issue=%+v, want action/input/missing fields", got)
	}
	if len(got.CandidateArtifacts) == 0 {
		t.Fatalf("CandidateArtifacts empty: %+v", got)
	}
	if len(got.RepairActionHints) == 0 {
		t.Fatalf("RepairActionHints empty: %+v", got)
	}
}

func TestDiscoverFieldContractIssuesFromZeroMatchExtractArtifact(t *testing.T) {
	issues := DiscoverFieldContractIssues(FieldContractIssueInput{
		Records: []WorkflowRecord{{
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "extracted",
				Kind:    string(dataquery.DataActionExtractFields),
				Headers: []string{"text"},
				Fields: map[string]string{
					"input_rows":             "10",
					"output_rows":            "0",
					"input_path":             "source_records",
					"source_fields":          "text",
					"required_fields":        "amount",
					"zero_match_diagnostics": "pattern produced no rows",
				},
			}}},
		}},
		ArtifactAccess: []ArtifactAccessView{
			{ID: "source_records", Aliases: []string{"source_records"}, Fields: []string{"text"}},
			{ID: "candidate_records", Aliases: []string{"candidate_records"}, Fields: []string{"amount"}},
		},
		AllowedNextActions: []string{string(dataquery.DataActionExtractFields)},
		Limit:              4,
	})
	if len(issues) != 1 {
		t.Fatalf("issue count=%d, want 1: %+v", len(issues), issues)
	}
	got := issues[0]
	if got.ArtifactID != "extracted" || got.ActionKind != string(dataquery.DataActionExtractFields) || !containsString(got.MissingFields, "amount") {
		t.Fatalf("issue=%+v, want extract_fields zero-match issue", got)
	}
	if got.Reason == "" || len(got.RepairActionHints) == 0 {
		t.Fatalf("issue=%+v, want reason and repair hints", got)
	}
}
