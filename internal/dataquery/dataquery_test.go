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

func TestRunnerAllowsTextEvidenceRequiredMaterialConsumption(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "evidence.txt"), []byte("total=42\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"evidence.txt"},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{{
				Path:             "source.bin",
				Purpose:          "non-text source covered by extracted text",
				Required:         true,
				UsageMode:        MaterialUseTextEvidenceConsumed,
				TextEvidencePath: "evidence.txt",
			}},
		},
		Script: `
text = read_text("evidence.txt")
emit_result(text.strip().split("=")[1], output_contract={"format": "plain_single_line", "explanation_allowed": False}, audit_summary="used extracted text evidence")
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "42" {
		t.Fatalf("Answer=%q", res.Answer)
	}
	if strings.Join(res.ConsumedPaths, ",") != "evidence.txt" {
		t.Fatalf("ConsumedPaths=%v", res.ConsumedPaths)
	}
}

func TestRunnerAllowsPlannerDistilledRequiredMaterial(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "orders.csv", Purpose: "input table", Required: true},
				{Path: "rules.md", Purpose: "rules distilled into validation_rules", Required: true, UsageMode: MaterialUsePlannerDistilled, DistilledNotes: []string{"sum all rows"}},
			},
			ValidationRules: []string{"sum all rows"},
		},
		Script: `
rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit_result(str(total), output_contract={"format": "plain_single_line", "explanation_allowed": False}, audit_summary="used distilled rule: sum all rows")
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "10" {
		t.Fatalf("Answer=%q", res.Answer)
	}
	if strings.Join(res.ConsumedPaths, ",") != "orders.csv" {
		t.Fatalf("ConsumedPaths=%v", res.ConsumedPaths)
	}
}

func TestRunnerRejectsPlannerDistilledWithoutNotes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "orders.csv", Purpose: "input table", Required: true},
				{Path: "rules.md", Purpose: "rules", Required: true, UsageMode: MaterialUsePlannerDistilled},
			},
		},
		Script: `emit_result("10", output_contract={"format": "plain_single_line", "explanation_allowed": False})`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "planner_distilled") || !strings.Contains(err.Error(), "distilled_notes") {
		t.Fatalf("Run err=%v, want missing distilled notes failure", err)
	}
}

func TestClassifyExecutionError(t *testing.T) {
	cases := map[string]string{
		`execute data task: data task script redefines reserved helper "read_text"`:                                          "reserved_helper_redefined",
		`data coverage incomplete: required material "rules.md" was not consumed by the script`:                              "required_material_not_consumed",
		`data coverage incomplete: required material "rules.md" uses planner_distilled but distilled_notes is empty`:         "planner_distilled_notes_missing",
		`data coverage incomplete: text evidence "scan.txt" for required material "scan.pdf" was not consumed by the script`: "text_evidence_not_consumed",
		`data validation incomplete: result.contributions[0] has unsupported operation "merge"`:                              "unsupported_contribution_operation",
		`data reconcile failed: group "A/amount" has no matching contribution records`:                                       "reconcile_group_mismatch",
		`data reconcile failed: group "A/amount" expected=9 but contributions sum to 10`:                                     "reconcile_sum_mismatch",
	}
	for text, want := range cases {
		if got := ClassifyExecutionError(text).Code; got != want {
			t.Fatalf("ClassifyExecutionError(%q)=%q, want %q", text, got, want)
		}
	}
}

func TestRunnerAllowsUTF8ScriptAndRequiredTextConsumption(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("规则：汇总所有金额\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv", "rules.md"},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "orders.csv", Purpose: "input table", Required: true},
				{Path: "rules.md", Purpose: "task rules", Required: true},
			},
			RuleCoverageRequired: true,
		},
		Script: `
# 允许 UTF-8 注释和字符串常量
rows = csv_rows("orders.csv")
rules = read_text("rules.md")
total = sum(int(r["amount"]) for r in rows)
emit({
  "answer": str(total),
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "audit_summary": "已读取规则并汇总金额",
  "rule_coverage": [{"rule_id": "r1", "rule_text": rules.strip(), "status": "applied", "evidence_refs": ["rules.md"]}],
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "10" {
		t.Fatalf("Answer=%q", res.Answer)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "orders.csv,rules.md" {
		t.Fatalf("ConsumedPaths=%v", res.ConsumedPaths)
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
  "rule_coverage": [{"rule_id": "r1", "rule_text": "include paid rows", "status": "applied", "notes": "paid rows are included"}],
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

func TestRunnerNormalizesContributionLedgerShapes(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "17",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "entity_resolutions": [{"source_value": "A", "canonical_id": "A", "status": "resolved", "candidates": ["A", "Alpha"]}],
  "contributions": [
    {"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A/amount", "metric": "amount", "value": "10", "operation": "sum"},
    {"item_id": "row2", "source": "orders.csv", "source_locator": "row 3", "group_key": "A/amount", "metric": "amount", "value": "7", "operation": "sum"}
  ],
  "reconcile": {"status": "pass", "expected_answer": "17", "actual_answer": "17", "groups": [{"group_key": "A/amount", "metric": "amount", "expected": "17", "actual": "17"}]}
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := res.Contributions[0].Operation.String(); got != "add" {
		t.Fatalf("operation normalized to %q, want add", got)
	}
	if got := res.Contributions[0].GroupKey.String(); got != "A" {
		t.Fatalf("group_key normalized to %q, want A", got)
	}
	if len(res.EntityResolutions) != 1 || len(res.EntityResolutions[0].Candidates) != 2 || res.EntityResolutions[0].Candidates[0].ID.String() != "A" {
		t.Fatalf("entity candidate string repair failed: %+v", res.EntityResolutions)
	}
}

func TestRunnerLedgerHelpersProduceCanonicalResult(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
add_rule_coverage(rule_id="r1", rule_text="include all rows", status="applied", notes="all rows read", evidence_refs=["orders.csv"])
add_resolution(source_value="A", canonical_id="A", canonical_label="A", status="resolved", candidates=["A"], evidence_refs=["orders.csv"])
total = Decimal("0")
for i, row in enumerate(rows):
    total += Decimal(row["amount"])
    add_decision(item_id=str(i), source="orders.csv", source_locator=f"row_{i+2}", decision="included", reason="included by r1", rule_refs=["r1"])
    add_contribution(item_id=str(i), source="orders.csv", source_locator=f"row_{i+2}", group_key=row["vendor"], metric="amount", value=row["amount"], operation="sum", reason="included by r1", rule_refs=["r1"])
emit_result(str(int(total)), output_contract={"format": "plain_single_line", "explanation_allowed": False}, audit_summary="helper-backed", reconcile={"status": "pass", "expected_answer": "17", "actual_answer": "17", "groups": [{"group_key": "A", "metric": "amount", "expected": "17", "actual": "17"}]})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17" || len(res.Rows) != 2 || len(res.Contributions) != 2 || len(res.RuleCoverage) != 1 || len(res.EntityResolutions) != 1 {
		t.Fatalf("unexpected helper-backed result: %+v", res)
	}
}

func TestRunnerRejectsUnsupportedContributionOperation(t *testing.T) {
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
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			ContributionLedgerRequired: true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [{"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "multiply"}]
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "unsupported operation") {
		t.Fatalf("Run err=%v, want unsupported operation failure", err)
	}
}

func TestRunnerRejectsRuleCoverageWithoutSupport(t *testing.T) {
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
			RequiredMaterials:    []CoverageMaterial{{Path: "orders.csv", Required: true}},
			RuleCoverageRequired: true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "rule_coverage": [{"rule_id": "r1", "rule_text": "include all rows", "status": "applied"}]
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "has no evidence_refs, notes, or linked rule_refs") {
		t.Fatalf("Run err=%v, want unsupported rule coverage failure", err)
	}
}

func TestRunnerAllowsRuleCoverageLinkedByRuleRefs(t *testing.T) {
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
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "rule_coverage": [{"rule_id": "r1", "rule_text": "include all rows", "status": "applied"}],
  "contributions": [{"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "total", "metric": "amount", "value": "10", "operation": "add", "rule_refs": ["r1"]}],
  "reconcile": {"status": "pass", "expected_answer": "10", "actual_answer": "10", "groups": [{"group_key": "total", "metric": "amount", "expected": "10", "actual": "10"}]}
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "10" {
		t.Fatalf("Answer=%q", res.Answer)
	}
}

func TestRunnerRejectsUnknownRuleRefs(t *testing.T) {
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
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "rule_coverage": [{"rule_id": "r1", "rule_text": "include all rows", "status": "applied", "notes": "applied to one row"}],
  "contributions": [{"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "total", "metric": "amount", "value": "10", "operation": "add", "rule_refs": ["missing"]}]
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), `references unknown rule_id "missing"`) {
		t.Fatalf("Run err=%v, want unknown rule ref failure", err)
	}
}

func TestRunnerRejectsContributionWithoutCanonicalEffect(t *testing.T) {
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
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			ContributionLedgerRequired: true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [{"source": "orders.csv"}]
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "contains no canonical contribution records") {
		t.Fatalf("Run err=%v, want canonical contribution failure", err)
	}
}

func TestRunnerRejectsBlankReconcileGroups(t *testing.T) {
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
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [{"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "add"}],
  "reconcile": {"status": "pass", "expected_answer": "10", "actual_answer": "10", "groups": [{}]}
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "groups contains no canonical group records") {
		t.Fatalf("Run err=%v, want canonical reconcile group failure", err)
	}
}

func TestRunnerRejectsUnreportedContributionGroup(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nB,5\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "15",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [
    {"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "add"},
    {"item_id": "row2", "source": "orders.csv", "source_locator": "row 3", "group_key": "B", "metric": "amount", "value": "5", "operation": "add"}
  ],
  "reconcile": {"status": "pass", "expected_answer": "15", "actual_answer": "15", "groups": [{"group_key": "A", "metric": "amount", "expected": "10", "actual": "10"}]}
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "does not report it") {
		t.Fatalf("Run err=%v, want missing reconcile group failure", err)
	}
}

func TestRunnerAllowsZeroReconcileGroupWithoutContributions(t *testing.T) {
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
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "A=10,B=0",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [
    {"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "add"}
  ],
  "reconcile": {"status": "pass", "groups": [
    {"group_key": "A", "metric": "amount", "expected": "10", "actual": "10"},
    {"group_key": "B", "metric": "amount", "expected": "0", "actual": "0"}
  ]}
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "A=10,B=0" {
		t.Fatalf("Answer=%q", res.Answer)
	}
}

func TestRunnerReportsAvailableContributionGroupsForMissingNonzeroGroup(t *testing.T) {
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
			RequiredMaterials:          []CoverageMaterial{{Path: "orders.csv", Required: true}},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "15",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [
    {"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "add"}
  ],
  "reconcile": {"status": "pass", "groups": [
    {"group_key": "B", "metric": "amount", "expected": "5", "actual": "5"}
  ]}
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), `available contribution groups: A/amount`) {
		t.Fatalf("Run err=%v, want available group diagnostic", err)
	}
}

func TestRunnerRejectsEntityResolutionWithoutCanonicalOrReason(t *testing.T) {
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
			RequiredMaterials:        []CoverageMaterial{{Path: "orders.csv", Required: true}},
			EntityResolutionRequired: true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "entity_resolutions": [{"source_value": "A", "status": "resolved"}]
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "missing canonical_id or canonical_label") {
		t.Fatalf("Run err=%v, want missing canonical failure", err)
	}

	plan.Script = `
rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "entity_resolutions": [{"source_value": "A", "status": "unresolved"}]
})
`
	_, err = (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "has no reason, candidates, or evidence_refs") {
		t.Fatalf("Run err=%v, want unresolved reason failure", err)
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
