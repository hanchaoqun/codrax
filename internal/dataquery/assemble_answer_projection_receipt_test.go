package dataquery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func scalarReconcileWithStaleProjection() *ReconcileReport {
	return &ReconcileReport{
		Status: LooseText("pass"),
		Groups: []ReconcileGroup{
			{GroupKey: LooseText("count"), Metric: LooseText("filtered_count"), Expected: LooseText("2"), Actual: LooseText("2")},
			{GroupKey: LooseText("final_answer"), Metric: LooseText("projection"), Scope: LooseText("final_answer"), Role: LooseText("output"), Expected: LooseText("0"), Actual: LooseText("0")},
		},
	}
}

func TestAssembleAnswerReprojectsBusinessGroupsWithoutStaleAnswerReceipt(t *testing.T) {
	res, err := (ActionRunner{Seed: Result{Reconcile: scalarReconcileWithStaleProjection()}}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:   "repair_answer",
			Kind: DataActionAssembleAnswer,
			Params: map[string]string{
				"projection":  "values",
				"value_field": "actual",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "2" {
		t.Fatalf("Answer=%q, want fresh business scalar without stale receipt", res.Answer)
	}
	projectionGroups := 0
	for _, group := range res.Reconcile.Groups {
		if reconcileGroupIsFinalAnswerProjection(group) {
			projectionGroups++
			if group.Actual.String() != "2" || group.Expected.String() != "2" {
				t.Fatalf("projection group=%+v, want fresh answer receipt", group)
			}
		}
	}
	if projectionGroups != 1 {
		t.Fatalf("reconcile groups=%+v, want exactly one fresh answer receipt", res.Reconcile.Groups)
	}
}

func TestAssembleAnswerDoesNotZeroFillAcrossDisjointReferenceDomain(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "reconcile_rows.csv"), []byte("metric,actual\nfiltered_count,2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root, Seed: Result{Reconcile: scalarReconcileWithStaleProjection()}}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:         "repair_answer",
			Kind:       DataActionAssembleAnswer,
			InputPaths: []string{"reconcile_rows.csv"},
			Params: map[string]string{
				"projection":          "values",
				"value_field":         "actual",
				"reference_path":      "reconcile_rows.csv",
				"reference_key_field": "metric",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "2" {
		t.Fatalf("Answer=%q, want measured business scalar instead of fabricated zero", res.Answer)
	}
	artifact := res.Artifacts[len(res.Artifacts)-1]
	if artifact.Fields["reference_projected"] != "" || artifact.Fields["zero_filled_count"] != "" {
		t.Fatalf("artifact fields=%+v, disjoint reference domain must not gain projection authority", artifact.Fields)
	}
}
