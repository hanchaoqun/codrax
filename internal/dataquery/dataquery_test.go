package dataquery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerCSVAggregationSingleLine(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount,status\nA,10,paid\nB,5,pending\nA,7,paid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		OutputContract: OutputContract{
			Format:             OutputCSVLine,
			ExplanationAllowed: false,
		},
		Script: `
rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows if r["status"] == "paid")
emit({
  "answer": "paid_total," + str(total),
  "output_contract": {"format": "csv_line", "explanation_allowed": False},
  "audit_summary": "summed paid rows only",
  "rows": [
    {"row_id": "1", "decision": "include", "value": "10"},
    {"row_id": "2", "decision": "exclude", "reason": "pending"},
    {"row_id": "3", "decision": "include", "value": "7"}
  ]
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "paid_total,17" {
		t.Fatalf("Answer=%q", res.Answer)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("Rows=%d, want 3", len(res.Rows))
	}
	if strings.Join(res.ConsumedPaths, ",") != "orders.csv" {
		t.Fatalf("ConsumedPaths=%v, want orders.csv", res.ConsumedPaths)
	}
}

func TestRunnerJSONOnlyValidation(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.json"), []byte(`{"x": 3}`), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"data.json"},
		OutputContract: OutputContract{
			Format:             OutputJSONOnly,
			ExplanationAllowed: false,
		},
		Script: `emit({"answer": "{\"x\":3}", "output_contract": {"format": "json_only", "explanation_allowed": False}})`,
	}
	if _, err := (Runner{RepoRoot: root}).Run(context.Background(), plan); err != nil {
		t.Fatalf("Run valid JSON-only: %v", err)
	}
	plan.Script = `emit({"answer": "result: {\"x\":3}", "output_contract": {"format": "json_only", "explanation_allowed": False}})`
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run JSON-only with extractable payload: %v", err)
	}
	if res.Answer != `{"x":3}` || len(res.ContractWarnings) == 0 {
		t.Fatalf("res=%+v, want extracted JSON with warning", res)
	}
	plan.Script = `emit({"answer": "x=3", "output_contract": {"format": "json_only", "explanation_allowed": False}})`
	res, err = (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run invalid JSON-only should not hard-gate: %v", err)
	}
	if len(res.ContractWarnings) == 0 || !strings.Contains(res.ContractWarnings[len(res.ContractWarnings)-1], "valid JSON") {
		t.Fatalf("warnings=%v, want valid JSON soft warning", res.ContractWarnings)
	}
}

func TestRunnerRejectsUnsafeScript(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("x\n1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		Script:     `import os; emit({"answer":"bad"})`,
	}
	if _, err := (Runner{RepoRoot: root}).Run(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "import is blocked") {
		t.Fatalf("Run unsafe err=%v, want blocked import rejection", err)
	}
}

func TestRunnerAllowsCommonDataImportsAndSafeOpen(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\nB,3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		Script: `
import csv
from collections import defaultdict
totals = defaultdict(int)
with open("orders.csv", "r") as f:
    for row in csv.DictReader(f):
        totals[row["vendor"]] += int(row["amount"])
emit({"answer": "A=" + str(totals["A"]), "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "A=17" {
		t.Fatalf("Answer=%q", res.Answer)
	}
}

func TestRunnerAllowsBoundedDebugPrint(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		OutputContract: OutputContract{
			Format:             OutputCSVLine,
			ExplanationAllowed: false,
		},
		Script: `
print("debug: loading rows")
rows = csv_rows("orders.csv")
print("x" * 50000)
total = sum(int(r["amount"]) for r in rows)
emit({"answer": "A," + str(total), "output_contract": {"format": "csv_line", "explanation_allowed": False}})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run with debug print: %v", err)
	}
	if res.Answer != "A,17" {
		t.Fatalf("Answer=%q", res.Answer)
	}
}

func TestRunnerDeduplicatesOverlappingFileAndDirectoryInputs(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("alpha\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "b.txt"), []byte("beta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"docs/a.txt", "docs"},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{{Path: "docs", Required: true}},
		},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		Script: `
answer = read_text("docs/a.txt").strip() + "+" + read_text("docs/b.txt").strip()
emit({"answer": answer, "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "alpha+beta" {
		t.Fatalf("Answer=%q", res.Answer)
	}
	if strings.Join(res.ConsumedPaths, ",") != "docs/a.txt,docs/b.txt" {
		t.Fatalf("ConsumedPaths=%v", res.ConsumedPaths)
	}
}

func TestRunnerRejectsUnsafeOpen(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("x\n1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"absolute": `open("/etc/passwd").read(); emit({"answer":"bad"})`,
		"write":    `open("orders.csv", "w").write("bad"); emit({"answer":"bad"})`,
	} {
		t.Run(name, func(t *testing.T) {
			plan := TaskPlan{InputPaths: []string{"orders.csv"}, Script: script}
			if _, err := (Runner{RepoRoot: root}).Run(context.Background(), plan); err == nil {
				t.Fatal("Run unexpectedly succeeded")
			}
		})
	}
}

func TestDiscoverCandidateFilesSkipsSource(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"a.csv":         "x\n1\n",
		"nested/b.json": `{"x":1}`,
		"main.go":       "package main\n",
	}
	for path, body := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := DiscoverCandidateFiles(root, 10)
	if err != nil {
		t.Fatalf("DiscoverCandidateFiles: %v", err)
	}
	paths := make([]string, 0, len(got))
	for _, f := range got {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, ",") != "a.csv,nested/b.json" {
		t.Fatalf("paths=%v", paths)
	}
	for _, f := range got {
		if f.Path == "a.csv" {
			if strings.Join(f.Headers, ",") != "x" || f.Lines != 2 || f.Delimiter != "," || len(f.SampleRows) != 1 || strings.Join(f.SampleRows[0], ",") != "1" {
				t.Fatalf("csv metadata=%+v, want header and line count", f)
			}
		}
	}
}

func TestDiscoverCandidateFilesIncludesNonTextMaterials(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "images"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "images", "ATT-1.png"), []byte{0x89, 'P', 'N', 'G'}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ATT-1.txt"), []byte("extracted text\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverCandidateFiles(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	var image CandidateFile
	for _, f := range got {
		if f.Path == "images/ATT-1.png" {
			image = f
			break
		}
	}
	if image.Kind != "image" {
		t.Fatalf("image candidate not discovered: %+v", got)
	}
	if image.ExtractionStatus != "related_text_available" {
		t.Fatalf("ExtractionStatus=%q", image.ExtractionStatus)
	}
	if strings.Join(image.TextEvidencePaths, ",") != "ATT-1.txt" {
		t.Fatalf("TextEvidencePaths=%v", image.TextEvidencePaths)
	}
}

func TestRunnerRejectsNonTextInput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "scan.png"), []byte{0x89, 'P', 'N', 'G'}, 0644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths:     []string{"scan.png"},
		OutputContract: OutputContract{Format: OutputPlainSingleLine},
		Script:         `emit({"answer":"x","output_contract":{"format":"plain_single_line","explanation_allowed":false}})`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "extract text evidence first") {
		t.Fatalf("expected non-text rejection, got %v", err)
	}
}

func TestRunnerRejectsMissingRequiredMaterialConsumption(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules.txt"), []byte("sum paid rows\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv", "rules.txt"},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "orders.csv", Purpose: "input table", Required: true},
				{Path: "rules.txt", Purpose: "task rules", Required: true},
			},
		},
		Script: `
rows = csv_rows("orders.csv")
emit({"answer": str(len(rows)), "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), `required material "rules.txt" was not consumed`) {
		t.Fatalf("Run err=%v, want required material consumption failure", err)
	}
}

func TestRunnerRejectsMissingRequiredDecisionRecords(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:       []CoverageMaterial{{Path: "orders.csv", Required: true}},
			DecisionRecordsRequired: true,
		},
		Script: `
rows = csv_rows("orders.csv")
emit({"answer": str(len(rows)), "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "decision_records_required=true but result.rows is empty") {
		t.Fatalf("Run err=%v, want decision records failure", err)
	}
}

func TestRunnerRejectsReservedHelperRedefinition(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		Script: `
def csv_rows(path):
    return []
emit({"answer": "0", "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), `redefines reserved helper "csv_rows"`) {
		t.Fatalf("Run err=%v, want reserved helper failure", err)
	}
}

func TestRunnerRejectsEmptyDecisionRecordObjects(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:       []CoverageMaterial{{Path: "orders.csv", Required: true}},
			DecisionRecordsRequired: true,
		},
		Script: `
rows = csv_rows("orders.csv")
emit({"answer": "0", "output_contract": {"format": "plain_single_line", "explanation_allowed": False}, "rows": [{}]})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "result.rows contains no meaningful decision records") {
		t.Fatalf("Run err=%v, want meaningful decision record failure", err)
	}
}

func TestRunnerPreservesCustomDecisionRecordFields(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:       []CoverageMaterial{{Path: "orders.csv", Required: true}},
			DecisionRecordsRequired: true,
		},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "rows": [{"po_id": "PO-1", "included": True, "amount": "10"}],
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0].NormalizedFields["po_id"] != "PO-1" || res.Rows[0].NormalizedFields["included"] != "true" {
		t.Fatalf("Rows=%+v, want custom fields preserved in normalized_fields", res.Rows)
	}
}

func TestRunnerRejectsMissingRequiredValidationLedgers(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount,status\nA,10,paid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	base := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False}
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), base)
	if err == nil || !strings.Contains(err.Error(), "rule_coverage_required=true") {
		t.Fatalf("Run err=%v, want missing rule coverage failure", err)
	}
	base.CoverageContract.RuleCoverageRequired = false
	_, err = (Runner{RepoRoot: root}).Run(context.Background(), base)
	if err == nil || !strings.Contains(err.Error(), "contribution_ledger_required=true") {
		t.Fatalf("Run err=%v, want missing contribution failure", err)
	}
	base.CoverageContract.ContributionLedgerRequired = false
	_, err = (Runner{RepoRoot: root}).Run(context.Background(), base)
	if err == nil || !strings.Contains(err.Error(), "entity_resolution_required=true") {
		t.Fatalf("Run err=%v, want missing entity resolution failure", err)
	}
	base.CoverageContract.EntityResolutionRequired = false
	_, err = (Runner{RepoRoot: root}).Run(context.Background(), base)
	if err == nil || !strings.Contains(err.Error(), "reconcile_required=true") {
		t.Fatalf("Run err=%v, want missing reconcile failure", err)
	}
}

func TestRunnerValidatesContributionReconcile(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount,status\nA,10,paid\nA,7,paid\nB,5,pending\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputCSVLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "A,17",
  "output_contract": {"format": "csv_line", "explanation_allowed": False},
  "rule_coverage": [{"rule_id": "r1", "rule_text": "include paid rows", "status": "applied"}],
  "entity_resolutions": [{"item_id": "A", "source_value": "A", "canonical_id": "A", "status": "resolved"}],
  "contributions": [
    {"item_id": "1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "add"},
    {"item_id": "2", "source": "orders.csv", "source_locator": "row 3", "group_key": "A", "metric": "amount", "value": "7", "operation": "add"}
  ],
  "reconcile": {
    "status": "pass",
    "expected_answer": "A,17",
    "actual_answer": "A,17",
    "groups": [{"group_key": "A", "metric": "amount", "expected": "17", "actual": "17"}]
  }
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "A,17" || len(res.Contributions) != 2 || res.Reconcile == nil {
		t.Fatalf("res=%+v, want contribution-backed reconcile", res)
	}

	plan.Script = strings.Replace(plan.Script, `"expected": "17", "actual": "17"`, `"expected": "18", "actual": "18"`, 1)
	_, err = (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "contributions sum to 17") {
		t.Fatalf("Run err=%v, want reconcile mismatch", err)
	}
}

func TestRunnerRejectsFailedReconcileStatus(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{{Path: "orders.csv", Required: true}},
			ReconcileRequired: true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "reconcile": {"status": "fail", "differences": ["total mismatch"]}
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "data reconcile failed") {
		t.Fatalf("Run err=%v, want failed reconcile rejection", err)
	}
}
