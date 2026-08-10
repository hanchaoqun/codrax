package dataquery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActionRunnerStableRowOriginRejectsExcludedDerivedContribution(t *testing.T) {
	root := t.TempDir()
	writeDecisionLineageFixture(t, root)
	plan := decisionLineagePlan([]DataAction{
		activeFilterAction("active_records"),
		{
			ID:             "filter_resolved_from_original",
			Kind:           DataActionFilterRecords,
			InputPaths:     []string{"observations.csv"},
			OutputArtifact: "resolved_records",
			Params: map[string]string{
				"filters_json": `[{"field":"resolution_status","op":"eq","value":"resolved"}]`,
				"reason":       "resolution is complete",
			},
		},
		contributionAction("resolved_records"),
		{ID: "reconcile", Kind: DataActionReconcile},
	})

	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run err=nil, want stable source identity to expose excluded-row contribution")
	}
	for _, want := range []string{"observations.csv#3", "sums a row the decision ledger excludes", "recompute contributions over included rows only"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Run err=%v, want %q", err, want)
		}
	}
}

func TestActionRunnerStableRowOriginSurvivesFilteredContribution(t *testing.T) {
	root := t.TempDir()
	writeDecisionLineageFixture(t, root)
	plan := decisionLineagePlan([]DataAction{
		activeFilterAction("active_records"),
		contributionAction("active_records"),
		{ID: "reconcile", Kind: DataActionReconcile},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17" || len(res.Contributions) != 2 {
		t.Fatalf("Answer=%q Contributions=%+v, want two included rows totaling 17", res.Answer, res.Contributions)
	}
	for i, contribution := range res.Contributions {
		wantID := "observations.csv#" + string(rune('1'+i))
		if contribution.ItemID.String() != wantID || contribution.Source.String() != "observations.csv" || contribution.SourceLocator.String() != wantID {
			t.Fatalf("Contributions[%d]=%+v, want immutable source identity %q", i, contribution, wantID)
		}
	}
}

func writeDecisionLineageFixture(t *testing.T, root string) {
	t.Helper()
	raw := "record_id,value,active,resolution_status\nr1,10,true,resolved\nr2,7,true,resolved\nr3,3,false,resolved\n"
	if err := os.WriteFile(filepath.Join(root, "observations.csv"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func decisionLineagePlan(actions []DataAction) TaskPlan {
	return TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: actions,
	}
}

func activeFilterAction(output string) DataAction {
	return DataAction{
		ID:             "filter_active",
		Kind:           DataActionFilterRecords,
		InputPaths:     []string{"observations.csv"},
		OutputArtifact: output,
		Params: map[string]string{
			"filters_json": `[{"field":"active","op":"eq","value":"true"}]`,
			"reason":       "active rows only",
		},
	}
}

func contributionAction(input string) DataAction {
	return DataAction{
		ID:         "compute_total",
		Kind:       DataActionComputeContribs,
		InputPaths: []string{input},
		Params: map[string]string{
			"group_key_literal": "all",
			"metric":            "total",
			"value_field":       "value",
			"operation":         "add",
			"reason":            "included typed rows",
		},
	}
}
