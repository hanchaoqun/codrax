package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestZeroMatchFilterIssuesFromBatchesSuppressesOlderClearedAlias(t *testing.T) {
	batches := []ArtifactDiagnosticBatch{{
		Round: 2,
		Artifacts: []dataquery.DataArtifact{{
			ID:       "eligible",
			Kind:     string(dataquery.DataActionFilterRecords),
			RowCount: 3,
			Fields: map[string]string{
				"artifact_aliases": "eligible,eligible.json",
				"input_rows":       "5",
				"output_rows":      "3",
			},
		}},
	}, {
		Round: 1,
		Artifacts: []dataquery.DataArtifact{{
			ID:       "eligible",
			Kind:     string(dataquery.DataActionFilterRecords),
			RowCount: 0,
			Fields: map[string]string{
				"artifact_aliases":   "eligible,eligible.json",
				"input_path":         "records.json",
				"input_rows":         "5",
				"output_rows":        "0",
				"filter_fields":      "status",
				"filter_diagnostics": `{"total":5,"combined_match":0}`,
			},
		}},
	}}
	if got := ZeroMatchFilterIssuesFromBatches(batches, 4); len(got) != 0 {
		t.Fatalf("issues=%+v, want older zero-match alias suppressed by newer positive artifact", got)
	}
}

func TestZeroMatchFilterIssuesFromArtifactProjectsDiagnostics(t *testing.T) {
	got := ZeroMatchFilterIssuesFromArtifact(3, dataquery.DataArtifact{
		ID:   "filtered",
		Kind: string(dataquery.DataActionFilterRecords),
		Fields: map[string]string{
			"artifact_aliases":   "filtered,filtered.json",
			"input_path":         "records.json",
			"input_rows":         "10",
			"output_rows":        "0",
			"filter_fields":      "status,currency",
			"filter_diagnostics": strings.Repeat("x", 1300),
		},
	})
	if len(got) != 1 {
		t.Fatalf("issues=%+v, want one zero-match issue", got)
	}
	issue := got[0]
	if issue.Round != 3 || issue.InputRows != 10 || issue.OutputRows != 0 || !containsString(issue.Aliases, "filtered.json") {
		t.Fatalf("issue=%+v, want projected round/counts/aliases", issue)
	}
	if len(issue.FilterDiagnostics) > 1200 || !containsString(issue.FilterFields, "currency") {
		t.Fatalf("issue diagnostics/filter_fields=%q/%v", issue.FilterDiagnostics, issue.FilterFields)
	}
}

func TestUnmatchedResolutionIssuesFromArtifactProjectsTargetCounts(t *testing.T) {
	got := UnmatchedResolutionIssuesFromArtifact(4, dataquery.DataArtifact{
		ID:       "resolved",
		Kind:     string(dataquery.DataActionApplyResolutions),
		RowCount: 2,
		Fields: map[string]string{
			"artifact_aliases":          "resolved,resolved.json",
			"base_path":                 "records.json",
			"base_rows":                 "2",
			"output_rows":               "2",
			"target_fields":             "canonical_id,canonical_label",
			"matched_canonical_id":      "0",
			"unmatched_canonical_id":    "2",
			"matched_canonical_label":   "0",
			"unmatched_canonical_label": "2",
		},
	})
	if len(got) != 1 {
		t.Fatalf("issues=%+v, want one unmatched resolution issue", got)
	}
	issue := got[0]
	if issue.BaseRows != 2 || !containsString(issue.TargetFields, "canonical_id") || !containsString(issue.UnmatchedCounts, "canonical_label=2") {
		t.Fatalf("issue=%+v, want target count diagnostics", issue)
	}
}

func TestZeroEligibleIssuesFromArtifactProjectsQualificationDiagnostics(t *testing.T) {
	got := ZeroEligibleIssuesFromArtifact(5, dataquery.DataArtifact{
		ID:   "qualified",
		Kind: string(dataquery.DataActionQualifyRecords),
		Fields: map[string]string{
			"artifact_aliases":      "qualified,qualified.json",
			"input_path":            "records.json",
			"input_rows":            "12",
			"eligible_rows":         "0",
			"include_filters":       strings.Repeat("f", 1300),
			"qualification_reasons": "all rows failed the declared eligibility contract",
		},
	})
	if len(got) != 1 {
		t.Fatalf("issues=%+v, want one zero-eligible issue", got)
	}
	issue := got[0]
	if issue.Round != 5 || issue.InputRows != 12 || issue.EligibleRows != 0 || len(issue.IncludeFilterDiagnostics) > 1200 {
		t.Fatalf("issue=%+v, want projected qualification diagnostics", issue)
	}
}
