package dataworkflow

import (
	"slices"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestCoverageContractViewSeparatesRequiredAndRunnerPaths(t *testing.T) {
	contract := dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{
			{ID: "source", Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			{ID: "image", Path: "scan.pdf", TextEvidencePath: "evidence/scan.txt", Required: true, UsageMode: dataquery.MaterialUseTextEvidenceConsumed},
			{ID: "notes", Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUsePlannerDistilled, DistilledNotes: []string{"rule"}},
		},
		OptionalMaterials: []dataquery.CoverageMaterial{
			{ID: "sample", Path: "sample.json", UsageMode: dataquery.MaterialUseReferenceOnly},
		},
		ValidationRules:            []string{"rule"},
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}
	view := CoverageContractViewFor(CoverageLayerWorkflow, contract)
	if view.Layer != CoverageLayerWorkflow {
		t.Fatalf("Layer=%q, want workflow", view.Layer)
	}
	for _, want := range []string{"records.csv", "scan.pdf", "rules.md"} {
		if !slices.Contains(view.RequiredPaths, want) {
			t.Fatalf("RequiredPaths=%v, missing %s", view.RequiredPaths, want)
		}
	}
	for _, want := range []string{"records.csv", "evidence/scan.txt"} {
		if !slices.Contains(view.RequiredRunnerInputPaths, want) {
			t.Fatalf("RequiredRunnerInputPaths=%v, missing %s", view.RequiredRunnerInputPaths, want)
		}
	}
	if slices.Contains(view.RequiredRunnerInputPaths, "rules.md") {
		t.Fatalf("planner-distilled material should not require runner input: %v", view.RequiredRunnerInputPaths)
	}
	if view.ValidationRuleCount != 1 || !view.ContributionLedgerRequired || !view.ReconcileRequired {
		t.Fatalf("view=%+v, want ledger flags and validation rule count", view)
	}
	if len(view.OptionalMaterials) != 1 || view.OptionalMaterials[0].Path != "sample.json" {
		t.Fatalf("OptionalMaterials=%+v", view.OptionalMaterials)
	}
}
