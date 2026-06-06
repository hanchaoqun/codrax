package dataquery

import (
	"context"
	"errors"
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

func TestRunnerEmitResultAcceptsStructuredObject(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount,status\nA,10,paid\nA,7,paid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: CoverageContract{
			DecisionRecordsRequired: true,
		},
		Script: `
rows = csv_rows("orders.csv")
for idx, row in enumerate(rows, start=1):
    add_decision(row_id=str(idx), decision="include", source="orders.csv", source_locator="row " + str(idx + 1), value=row["amount"])
emit_result({
  "answer": str(sum(int(r["amount"]) for r in rows)),
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "audit_summary": "object-style emit_result should be treated as a result object",
  "rows": []
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17" {
		t.Fatalf("Answer=%q, want object answer field only", res.Answer)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("Rows=%d, want helper rows merged", len(res.Rows))
	}
}

func TestRunnerPreservesExtraEmitPayloadAsArtifact(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rules.md"), []byte("R1: keep paid rows\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := Runner{RepoRoot: dir}
	result, err := runner.Run(context.Background(), TaskPlan{
		InputPaths: []string{"rules.md"},
		OutputContract: OutputContract{
			Format:             OutputFreeform,
			ExplanationAllowed: true,
		},
		Script: `content = read_text("rules.md")
emit({"content": content, "line_count": len(content.splitlines())})`,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Artifacts) == 0 || result.Artifacts[0].ID != "emitted_payload" {
		t.Fatalf("Artifacts=%+v, want emitted payload artifact", result.Artifacts)
	}
	if !strings.Contains(strings.Join(result.Artifacts[0].Sample, "\n"), "keep paid rows") {
		t.Fatalf("payload artifact sample=%v", result.Artifacts[0].Sample)
	}
	if !strings.Contains(result.Artifacts[0].Fields["payload_keys"], "content") {
		t.Fatalf("payload artifact fields=%v", result.Artifacts[0].Fields)
	}
}

func TestRunnerNormalizesScalarRowDecisions(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount,status\nA,10,paid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"orders.csv"},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: CoverageContract{
			DecisionRecordsRequired: true,
		},
		Script: `
rows = csv_rows("orders.csv")
emit({
  "answer": rows[0]["amount"],
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "rows": ["included paid row", 3, True]
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("Rows=%d, want 3", len(res.Rows))
	}
	if res.Rows[0].Decision != "observed" || res.Rows[0].Reason != "included paid row" {
		t.Fatalf("Rows[0]=%+v, want observed scalar row", res.Rows[0])
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

func TestActionRunnerMaterialInventoryAndInspectArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("sum all rows\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		Actions: []DataAction{
			{ID: "inventory", Kind: DataActionMaterialInventory, Purpose: "discover materials"},
			{ID: "inspect_orders", Kind: DataActionInspectMaterial, InputPaths: []string{"orders.csv"}, OutputArtifact: "orders_profile"},
		},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if len(res.Artifacts) != 2 {
		t.Fatalf("Artifacts=%d, want 2: %+v", len(res.Artifacts), res.Artifacts)
	}
	if !strings.Contains(res.Answer, "discovered") || !strings.Contains(res.Answer, "inspected") {
		t.Fatalf("Answer=%q, want artifact summary", res.Answer)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "orders.csv" {
		t.Fatalf("ConsumedPaths=%q, want orders.csv", got)
	}
}

func TestActionRunnerReturnsPartialArtifactsOnLaterActionFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		Actions: []DataAction{
			{ID: "inspect_orders", Kind: DataActionInspectMaterial, InputPaths: []string{"orders.csv"}, OutputArtifact: "orders_profile"},
			{ID: "bad_next_node", Kind: DataActionKind("unsupported_action"), Purpose: "force a later action failure"},
		},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatalf("Run unexpectedly succeeded")
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("partial Artifacts=%d, want 1: %+v", len(res.Artifacts), res.Artifacts)
	}
	if res.Artifacts[0].ID != "orders_profile" {
		t.Fatalf("partial artifact=%+v, want orders_profile", res.Artifacts[0])
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "orders.csv" {
		t.Fatalf("partial ConsumedPaths=%q, want orders.csv", got)
	}
	if !strings.Contains(res.AuditSummary, "inspected") {
		t.Fatalf("partial AuditSummary=%q, want inspected summary", res.AuditSummary)
	}
}

func TestActionRunnerExtractRecordsArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,amount\n1,10\n2,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "events.jsonl"), []byte("{\"id\":\"a\",\"ok\":true}\n{\"id\":\"b\",\"ok\":false}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := ActionRunner{RepoRoot: root, TempRoot: filepath.Join(root, ".tmp")}
	res, err := runner.Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputJSONOnly, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:              "extract",
			Kind:            DataActionExtractRecords,
			InputPaths:      []string{"orders.csv", "events.jsonl"},
			OutputArtifact:  "records",
			Params:          map[string]string{"limit": "1"},
			SuccessCriteria: []string{"bounded samples are available"},
		}},
	})
	if err != nil {
		t.Fatalf("ActionRunner.Run: %v", err)
	}
	if len(res.Artifacts) != 1 || len(res.Artifacts[0].Children) != 2 {
		t.Fatalf("artifacts=%+v", res.Artifacts)
	}
	var csvArtifact DataArtifact
	for _, child := range res.Artifacts[0].Children {
		if len(child.SourcePaths) > 0 && child.SourcePaths[0] == "orders.csv" {
			csvArtifact = child
			break
		}
	}
	if csvArtifact.Kind != "extract_records/csv" || strings.Join(csvArtifact.Headers, ",") != "id,amount" || csvArtifact.RowCount != 2 || len(csvArtifact.Sample) != 1 {
		t.Fatalf("csv artifact=%+v", csvArtifact)
	}
	if shape := res.Artifacts[0].Fields["json_shape"]; !strings.Contains(shape, "array") {
		t.Fatalf("artifact json_shape=%q, want array shape", shape)
	}
	if got := strings.Join(res.ConsumedPaths, ","); !strings.Contains(got, "orders.csv") || !strings.Contains(got, "events.jsonl") {
		t.Fatalf("ConsumedPaths=%v", res.ConsumedPaths)
	}
}

func TestActionRunnerCustomTransformConsumesPriorActionArtifact(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,amount\n1,10\n2,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{
			{
				ID:             "extract_orders",
				Kind:           DataActionExtractRecords,
				InputPaths:     []string{"orders.csv"},
				OutputArtifact: "orders_records",
				Params:         map[string]string{"limit": "10"},
			},
			{
				ID:         "sum_orders",
				Kind:       DataActionCustomTransform,
				InputPaths: []string{"artifacts/orders_records.json"},
				Script: `
rows = json_load("artifacts/orders_records.json")
total = sum(int(row["amount"]) for row in rows)
emit_result(str(total), output_contract={"format":"plain_single_line","explanation_allowed":False})
`,
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if res.Answer != "30" {
		t.Fatalf("Answer=%q, want 30", res.Answer)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "artifacts/orders_records.json,orders.csv" {
		t.Fatalf("ConsumedPaths=%q, want source plus materialized artifact", got)
	}
}

func TestActionRunnerCustomTransformConsumesActionNamespacedArtifact(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,amount\n1,10\n2,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{
			{
				ID:             "extract_orders",
				Kind:           DataActionExtractRecords,
				InputPaths:     []string{"orders.csv"},
				OutputArtifact: "orders_records",
				Params:         map[string]string{"limit": "10"},
			},
			{
				ID:         "sum_orders",
				Kind:       DataActionCustomTransform,
				InputPaths: []string{"artifacts/extract_orders/orders_records.json"},
				Script: `
rows = json_load("artifacts/extract_orders/orders_records.json")
total = sum(int(row["amount"]) for row in rows)
emit_result(str(total), output_contract={"format":"plain_single_line","explanation_allowed":False})
`,
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions with namespaced artifact alias: %v", err)
	}
	if res.Answer != "30" {
		t.Fatalf("Answer=%q, want 30", res.Answer)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "artifacts/extract_orders/orders_records.json,orders.csv" {
		t.Fatalf("ConsumedPaths=%q, want namespaced alias plus source", got)
	}
}

func TestActionRunnerCustomTransformConsumesSeedArtifactAcrossBatches(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	tempRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,amount\n1,10\n2,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		ContinueAfter:  true,
		Actions: []DataAction{{
			ID:             "extract_orders",
			Kind:           DataActionExtractRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "orders_records",
			Params:         map[string]string{"limit": "10"},
		}},
	}
	seed, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot}).Run(context.Background(), first)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if len(seed.Artifacts) == 0 || seed.Artifacts[0].Fields["artifact_path"] == "" {
		t.Fatalf("seed artifacts missing materialized path: %+v", seed.Artifacts)
	}
	if _, err := os.Stat(seed.Artifacts[0].Fields["artifact_path"]); err != nil {
		t.Fatalf("materialized artifact was not persisted across batches: %v", err)
	}
	second := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:         "sum_orders",
			Kind:       DataActionCustomTransform,
			InputPaths: []string{"artifacts/orders_records.json"},
			Script: `
rows = json_load("artifacts/orders_records.json")
total = sum(int(row["amount"]) for row in rows)
emit_result(str(total), output_contract={"format":"plain_single_line","explanation_allowed":False})
`,
		}},
	}
	res, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot, Seed: seed}).Run(context.Background(), second)
	if err != nil {
		t.Fatalf("second Run consumed seed artifact: %v", err)
	}
	if res.Answer != "30" {
		t.Fatalf("Answer=%q, want 30", res.Answer)
	}
}

func TestActionRunnerJSONRecordsReadsArrayAndWrapperArtifacts(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	tempRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,amount\n1,10\n2,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seed, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		ContinueAfter:  true,
		Actions: []DataAction{{
			ID:             "extract_orders",
			Kind:           DataActionExtractRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "orders_records",
			Params:         map[string]string{"limit": "10"},
		}},
	})
	if err != nil {
		t.Fatalf("seed Run: %v", err)
	}
	res, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot, Seed: seed}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:         "sum_records",
			Kind:       DataActionCustomTransform,
			InputPaths: []string{"orders_records"},
			Script: `
rows = json_records("orders_records")
total = sum(int(row["amount"]) for row in rows)
emit_result(str(total), output_contract={"format":"plain_single_line","explanation_allowed":False})
`,
		}},
	})
	if err != nil {
		t.Fatalf("json_records array artifact: %v", err)
	}
	if res.Answer != "30" {
		t.Fatalf("Answer=%q, want 30", res.Answer)
	}

	wrapper := filepath.Join(root, "wrapped.json")
	if err := os.WriteFile(wrapper, []byte(`{"rules":[{"id":"R1"},{"id":"R2"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = (Runner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		InputPaths:       []string{"wrapped.json"},
		OutputContract:   OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{},
		Script: `
rows = json_records("wrapped.json")
emit_result(str(len(rows)), output_contract={"format":"plain_single_line","explanation_allowed":False})
`,
	})
	if err != nil {
		t.Fatalf("json_records wrapper object: %v", err)
	}
	if res.Answer != "2" {
		t.Fatalf("Answer=%q, want 2", res.Answer)
	}
}

func TestActionRunnerTypedActionConsumesPriorActionArtifact(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount,status\nA,10,paid\nB,5,pending\nA,7,paid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:             "extract_orders",
				Kind:           DataActionExtractRecords,
				InputPaths:     []string{"orders.csv"},
				OutputArtifact: "orders_records",
				Params:         map[string]string{"limit": "10"},
			},
			{
				ID:         "contrib",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"orders_records"},
				Params: map[string]string{
					"group_key_field": "vendor",
					"metric":          "amount",
					"value_field":     "amount",
					"operation":       "add",
					"filters_json":    `[{"field":"status","op":"eq","value":"paid"}]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if res.Answer != "17" {
		t.Fatalf("Answer=%q, want 17", res.Answer)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want 2", res.Contributions)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "orders.csv,orders_records" {
		t.Fatalf("ConsumedPaths=%q, want source plus materialized artifact", got)
	}
}

func TestActionRunnerRuleContributionReconcileActions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount,status\nA,10,paid\nB,5,pending\nA,7,paid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:   "rules",
				Kind: DataActionDeriveRules,
				Params: map[string]string{
					"rules_json": `[{"id":"r1","text":"include paid records only","status":"applied","notes":"filter status=paid"}]`,
				},
			},
			{
				ID:         "contrib",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"orders.csv"},
				Params: map[string]string{
					"group_key_field": "vendor",
					"metric":          "amount",
					"value_field":     "amount",
					"operation":       "add",
					"filters_json":    `[{"field":"status","op":"eq","value":"paid"}]`,
					"rule_refs":       `["r1"]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if res.Answer != "17" {
		t.Fatalf("Answer=%q, want 17", res.Answer)
	}
	if len(res.RuleCoverage) != 1 || res.RuleCoverage[0].RuleID.String() != "r1" {
		t.Fatalf("RuleCoverage=%+v", res.RuleCoverage)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want 2 paid rows", res.Contributions)
	}
	if res.Reconcile == nil || len(res.Reconcile.Groups) != 1 || res.Reconcile.Groups[0].Actual.String() != "17" {
		t.Fatalf("Reconcile=%+v", res.Reconcile)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "orders.csv" {
		t.Fatalf("ConsumedPaths=%q, want orders.csv", got)
	}
}

func TestActionRunnerJoinRecordsContributionReconcileActions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("order_id,vendor_id,amount,status\nO1,V1,10,paid\nO2,V2,5,pending\nO3,V1,7,accepted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vendors.csv"), []byte("vendor_id,category\nV1,compute\nV2,office\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:             "join_orders_vendors",
				Kind:           DataActionJoinRecords,
				InputPaths:     []string{"orders.csv", "vendors.csv"},
				OutputArtifact: "orders_with_vendor",
				Params: map[string]string{
					"left_fields":  `["vendor_id"]`,
					"right_fields": `["vendor_id"]`,
				},
			},
			{
				ID:         "contrib",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"artifacts/join_orders_vendors/orders_with_vendor.json"},
				Params: map[string]string{
					"group_key_field": "category",
					"metric":          "amount",
					"value_field":     "amount",
					"operation":       "add",
					"item_id_field":   "order_id",
					"filters_json":    `[{"field":"status","op":"in","value":["paid","accepted"]}]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run join/contribution actions: %v", err)
	}
	if res.Answer != "17" {
		t.Fatalf("Answer=%q, want 17", res.Answer)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want 2 joined paid/accepted rows", res.Contributions)
	}
	if res.Artifacts[0].Kind != string(DataActionJoinRecords) || res.Artifacts[0].RowCount != 3 {
		t.Fatalf("join artifact=%+v", res.Artifacts[0])
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "artifacts/join_orders_vendors/orders_with_vendor.json,orders.csv,vendors.csv" {
		t.Fatalf("ConsumedPaths=%q, want joined artifact plus sources", got)
	}
}

func TestActionRunnerEnrichJoinContributionReconcileActions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("order_id,vendor_id,category_raw,amount,status\nO1,V1,云资源,10,paid\nO2,V1,办公,5,paid\nO3,V1,算力服务,7,accepted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "categories.csv"), []byte("category_code,category_name,aliases\nCOMPUTE,算力服务,云资源;算力\nOFFICE,办公用品,办公;文具\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "queries.csv"), []byte("query_id,vendor_id,category_code\nQ1,V1,COMPUTE\nQ2,V1,OFFICE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:             "enrich_orders",
				Kind:           DataActionEnrichRecords,
				InputPaths:     []string{"orders.csv", "categories.csv"},
				OutputArtifact: "orders_enriched",
				Params: map[string]string{
					"base_path":             "orders.csv",
					"mapping_path":          "categories.csv",
					"source_field":          "category_raw",
					"mapping_source_fields": `["category_name","aliases"]`,
					"mapping_value_field":   "category_code",
					"target_field":          "category_code",
					"match_mode":            "mapping_contains_source",
				},
			},
			{
				ID:             "join_queries",
				Kind:           DataActionJoinRecords,
				InputPaths:     []string{"orders_enriched", "queries.csv"},
				OutputArtifact: "orders_query_joined",
				Params: map[string]string{
					"left_fields":  `["vendor_id","category_code"]`,
					"right_fields": `["vendor_id","category_code"]`,
				},
			},
			{
				ID:         "contrib",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"orders_query_joined"},
				Params: map[string]string{
					"group_key_field": "query_id",
					"metric":          "amount",
					"value_field":     "amount",
					"operation":       "add",
					"item_id_field":   "order_id",
					"filters_json":    `[{"field":"status","op":"in","value":["paid","accepted"]}]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run enrich/join/contribution actions: %v", err)
	}
	if res.Answer != "Q1/amount=17; Q2/amount=5" {
		t.Fatalf("Answer=%q", res.Answer)
	}
	if len(res.Contributions) != 3 {
		t.Fatalf("Contributions=%+v, want 3 matched rows", res.Contributions)
	}
	if len(res.Artifacts) < 4 || res.Artifacts[0].Kind != string(DataActionEnrichRecords) {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	if got := res.Artifacts[0].Fields["matches_category_code"]; got != "3" {
		t.Fatalf("enrich matches=%q, want 3", got)
	}
}

func TestActionRunnerDeriveFieldsContributionReconcileActions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("item_id,token,value_raw,flag\nA,group:alpha,1.234s,ok\nB,group:beta,9ms,skip\nC,group:alpha,0.007s,ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:             "derive",
				Kind:           DataActionDeriveFields,
				InputPaths:     []string{"events.csv"},
				OutputArtifact: "events_derived",
				Params: map[string]string{
					"field_specs_json": `[
						{"source_field":"token","target_field":"group","operation":"regex_extract","pattern":"group:([a-z]+)"},
						{"source_field":"value_raw","target_field":"value_ms","operation":"parse_number","multiplier":"1000"},
						{"source_field":"flag","target_field":"flag_bucket","operation":"map","mapping":{"ok":"include","skip":"exclude"}}
					]`,
				},
			},
			{
				ID:         "contrib",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"events_derived"},
				Params: map[string]string{
					"group_key_field": "flag_bucket",
					"metric":          "value_ms",
					"value_field":     "value_ms",
					"operation":       "add",
					"item_id_field":   "item_id",
					"filters_json":    `[{"field":"group","op":"eq","value":"alpha"},{"field":"flag_bucket","op":"eq","value":"include"}]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run derive/contribution actions: %v", err)
	}
	if res.Answer != "1241" {
		t.Fatalf("Answer=%q, want 1241", res.Answer)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want 2 matched rows", res.Contributions)
	}
	if len(res.Artifacts) < 3 || res.Artifacts[0].Kind != string(DataActionDeriveFields) {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	if got := res.Artifacts[0].Fields["derived_fields"]; got != "group,value_ms,flag_bucket" {
		t.Fatalf("derived fields=%q", got)
	}
}

func TestActionRunnerComputeContributionsInheritsRuleRefsAndMaterializesPayload(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount,status\nA,10,paid\nB,5,pending\nA,7,paid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:   "rules",
				Kind: DataActionDeriveRules,
				Params: map[string]string{
					"rules_json": `[{"id":"r1","text":"include paid records only","status":"applied","notes":"filter status=paid"}]`,
				},
			},
			{
				ID:             "contrib",
				Kind:           DataActionComputeContribs,
				InputPaths:     []string{"orders.csv"},
				OutputArtifact: "totals",
				Params: map[string]string{
					"group_key_field": "vendor",
					"metric":          "amount",
					"value_field":     "amount",
					"operation":       "add",
					"filters_json":    `[{"field":"status","op":"eq","value":"paid"}]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
			{
				ID:         "final",
				Kind:       DataActionCustomTransform,
				InputPaths: []string{"totals.json"},
				Script: `data = json_load("totals.json")
total = sum(parse_money(c.get("value", "0")) for c in data.get("contributions", []))
emit_result(str(int(total)), output_contract={"format":"plain_single_line","explanation_allowed":False})`,
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if res.Answer != "17" {
		t.Fatalf("Answer=%q, want 17", res.Answer)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want 2 paid rows", res.Contributions)
	}
	for i, rec := range res.Contributions {
		if got := strings.Join(rec.RuleRefs, ","); got != "r1" {
			t.Fatalf("Contributions[%d].RuleRefs=%v, want inherited r1", i, rec.RuleRefs)
		}
		if rec.Role.String() != "target" {
			t.Fatalf("Contributions[%d].Role=%q, want target", i, rec.Role.String())
		}
	}
}

func TestActionRunnerImplicitCountContributionIsAuditRole(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.csv"), []byte("id,path\nA,one.txt\nB,two.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:         "count_material",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"manifest.csv"},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run implicit count action: %v", err)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want 2 count records", res.Contributions)
	}
	for i, rec := range res.Contributions {
		if rec.Role.String() != "audit" {
			t.Fatalf("Contributions[%d].Role=%q, want audit", i, rec.Role.String())
		}
	}
}

func TestActionRunnerComputeContributionsAllowsNonContributingInput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.csv"), []byte("id,path,amount\nA,one.txt,\nB,two.txt,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{{Path: "manifest.csv", Required: true}},
		},
		Actions: []DataAction{{
			ID:         "contrib",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"manifest.csv"},
			Params: map[string]string{
				"group_key_field": "id",
				"metric":          "amount",
				"value_field":     "amount",
				"operation":       "add",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run empty contribution action: %v", err)
	}
	if len(res.Contributions) != 0 {
		t.Fatalf("Contributions=%+v, want empty intermediate artifact", res.Contributions)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].Fields["count"] != "0" {
		t.Fatalf("Artifacts=%+v, want count=0 artifact", res.Artifacts)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "manifest.csv" {
		t.Fatalf("ConsumedPaths=%q, want manifest.csv", got)
	}
}

func TestActionRunnerComputeContributionsRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.csv"), []byte("id,path\nA,one.txt\nB,two.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:         "contrib",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"manifest.csv"},
			Params: map[string]string{
				"group_key_field": "id",
				"metric":          "amount",
				"value_field":     "amount",
				"operation":       "add",
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "value_field") || !strings.Contains(err.Error(), "amount") {
		t.Fatalf("Run err=%v, want missing value_field diagnostic", err)
	}
}

func TestActionRunnerComputeContributionsAcceptsArrayFilterValue(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status,amount\nA,paid,10\nB,draft,5\nC,accepted,7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:         "contrib",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"orders.csv"},
			Params: map[string]string{
				"group_key":    "all",
				"metric":       "amount",
				"value_field":  "amount",
				"operation":    "sum",
				"filters_json": `[{"field":"status","op":"in","value":["paid","accepted"]}]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run array filter action: %v", err)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want paid+accepted rows", res.Contributions)
	}
}

func TestCustomTransformAllowsUniqueLooseRawFieldAlias(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,amount_raw\nA,10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths:     []string{"orders.csv"},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit_result(str(rows[0]["amount"]), output_contract={"format": "plain_single_line", "explanation_allowed": False})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run loose alias script: %v", err)
	}
	if res.Answer != "10" {
		t.Fatalf("Answer=%q, want 10", res.Answer)
	}
}

func TestCustomTransformDoesNotAliasCodeToRawField(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,category_raw\nA,compute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths:     []string{"orders.csv"},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit_result(str(rows[0]["category_code"]), output_contract={"format": "plain_single_line", "explanation_allowed": False})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), `KeyError: 'category_code'`) {
		t.Fatalf("Run err=%v, want category_code runtime rejection", err)
	}
}

func TestActionRunnerReconcileUsesSeedContributions(t *testing.T) {
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{{
			ID:   "reconcile",
			Kind: DataActionReconcile,
		}},
	}
	seed := Result{
		Contributions: []ContributionRecord{{
			ItemID:        LooseText("item1"),
			Source:        LooseText("orders.csv"),
			SourceLocator: LooseText("row 2"),
			GroupKey:      LooseText("Q001"),
			Metric:        LooseText("amount"),
			Value:         LooseText("10"),
			Operation:     LooseText("add"),
			Reason:        LooseText("seeded contribution"),
		}},
	}
	res, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run seeded reconcile: %v", err)
	}
	if res.Reconcile == nil || len(res.Reconcile.Groups) != 1 || res.Reconcile.Groups[0].Actual.String() != "10" {
		t.Fatalf("Reconcile=%+v, want seeded contribution group", res.Reconcile)
	}
	if len(res.Contributions) != 1 {
		t.Fatalf("Contributions=%+v, want seeded contribution preserved", res.Contributions)
	}
}

func TestActionRunnerDeriveRulesConsumesInputMaterial(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("# Rules\n- include completed rows\n- exclude pending rows\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RequiredMaterials:    []CoverageMaterial{{Path: "rules.md", Purpose: "task rules", Required: true}},
			RuleCoverageRequired: true,
		},
		Actions: []DataAction{{
			ID:         "rules",
			Kind:       DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
			Params:     map[string]string{"limit": "10"},
		}},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "rules.md" {
		t.Fatalf("ConsumedPaths=%q, want rules.md", got)
	}
	if len(res.RuleCoverage) < 2 {
		t.Fatalf("RuleCoverage=%+v, want rules derived from input text", res.RuleCoverage)
	}
	if !strings.Contains(res.RuleCoverage[0].EvidenceRefs[0], "rules.md:") {
		t.Fatalf("RuleCoverage[0]=%+v, want line-backed evidence ref", res.RuleCoverage[0])
	}
}

func TestActionRunnerDeriveRulesPreservesExplicitRuleIDs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("RULE_KEEP: include completed rows\nRULE_SKIP: exclude pending rows\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RequiredMaterials:    []CoverageMaterial{{Path: "rules.md", Purpose: "task rules", Required: true}},
			RuleCoverageRequired: true,
		},
		Actions: []DataAction{{
			ID:         "rules",
			Kind:       DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
			Params: map[string]string{
				"rules": "RULE_KEEP: include completed rows\nRULE_SKIP: exclude pending rows",
			},
		}},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	ids := map[string]RuleCoverageRecord{}
	for _, rec := range res.RuleCoverage {
		ids[rec.RuleID.String()] = rec
	}
	for _, id := range []string{"RULE_KEEP", "RULE_SKIP"} {
		rec, ok := ids[id]
		if !ok {
			t.Fatalf("RuleCoverage IDs=%v, want %s", ids, id)
		}
		if got := strings.Join(rec.EvidenceRefs, ","); !strings.Contains(got, "rules.md") {
			t.Fatalf("RuleCoverage[%s]=%+v, want source-backed evidence ref", id, rec)
		}
	}
}

func TestActionRunnerCustomTransformCanReferenceDerivedExplicitRuleID(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("RULE_KEEP: include completed rows\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status,amount\n1,completed,10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "rules.md", Purpose: "task rules", Required: true},
				{Path: "orders.csv", Purpose: "source rows", Required: true},
			},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:         "rules",
				Kind:       DataActionDeriveRules,
				InputPaths: []string{"rules.md"},
				Params:     map[string]string{"rules": "RULE_KEEP: include completed rows"},
			},
			{
				ID:         "compute",
				Kind:       DataActionCustomTransform,
				InputPaths: []string{"orders.csv"},
				Script: `
rows = csv_rows("orders.csv")
total = 0
for row in rows:
    if row["status"] == "completed":
        total += int(row["amount"])
        add_decision(item_id=row["id"], source="orders.csv", source_locator="line 2", decision="include", rule_refs=["RULE_KEEP"], evidence_refs=["orders.csv:2"])
        add_contribution(item_id=row["id"], source="orders.csv", source_locator="line 2", group_key="all", metric="amount", value=row["amount"], operation="add", role="target", rule_refs=["RULE_KEEP"], evidence_refs=["orders.csv:2"])
emit_result(str(total), output_contract={"format":"plain_single_line","explanation_allowed":False})
`,
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if res.Answer != "10" {
		t.Fatalf("Answer=%q, want 10", res.Answer)
	}
}

func TestActionRunnerIntermediateActionUsesLocalCoverageContract(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("- include completed rows\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status\n1,completed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "rules.md", Purpose: "task rules", Required: true},
				{Path: "orders.csv", Purpose: "source rows", Required: true},
			},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{{
			ID:         "derive_rules",
			Kind:       DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
		}},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("intermediate ActionRunner.Run should use action-local coverage: %v", err)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "rules.md" {
		t.Fatalf("ConsumedPaths=%q, want rules.md", got)
	}
	if len(res.RuleCoverage) == 0 {
		t.Fatalf("RuleCoverage empty")
	}
}

func TestActionRunnerTerminalActionStillRequiresWorkflowCoverage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("- include completed rows\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status\n1,completed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "rules.md", Purpose: "task rules", Required: true},
				{Path: "orders.csv", Purpose: "source rows", Required: true},
			},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{{
			ID:         "derive_rules",
			Kind:       DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("terminal ActionRunner.Run unexpectedly accepted missing workflow coverage")
	}
	if !strings.Contains(err.Error(), "orders.csv") {
		t.Fatalf("err=%v, want missing orders.csv", err)
	}
}

func TestActionRunnerDeriveRulesPrefersExplicitInputMaterial(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("- include paid rows\n- exclude cancelled rows\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			ValidationRules:      []string{"format output as a single line"},
			RuleCoverageRequired: true,
		},
		Actions: []DataAction{{
			ID:         "rules",
			Kind:       DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
		}},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if len(res.RuleCoverage) < 2 {
		t.Fatalf("RuleCoverage=%+v, want input-derived records", res.RuleCoverage)
	}
	if !strings.Contains(res.RuleCoverage[0].EvidenceRefs[0], "rules.md:") {
		t.Fatalf("RuleCoverage[0]=%+v, want evidence from explicit input material", res.RuleCoverage[0])
	}
	for _, rec := range res.RuleCoverage {
		if strings.Contains(rec.RuleText.String(), "format output") {
			t.Fatalf("validation-only rule was used before explicit input material: %+v", res.RuleCoverage)
		}
	}
}

func TestActionRunnerNormalizeEntitiesAction(t *testing.T) {
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:   "normalize",
			Kind: DataActionNormalizeEntities,
			Params: map[string]string{
				"resolutions_json": `[{"item_id":"row1","source_value":"A Inc","canonical_label":"A Incorporated","status":"resolved","reason":"exact reference mapping"}]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: t.TempDir()}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if len(res.EntityResolutions) != 1 || res.EntityResolutions[0].CanonicalLabel.String() != "A Incorporated" {
		t.Fatalf("EntityResolutions=%+v", res.EntityResolutions)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].Kind != string(DataActionNormalizeEntities) {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
}

func TestActionRunnerNormalizeEntitiesFromStructuredInput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "vendors.csv"), []byte("status,vendor_id,short_name,brand_name,legal_name\nactive,V001,Acme,Acme Cloud,Acme Cloud Ltd\ninactive,V002,Old,Old Brand,Old Brand Ltd\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RequiredMaterials:        []CoverageMaterial{{Path: "vendors.csv", Required: true}},
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:         "normalize_vendors",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"vendors.csv"},
			Params: map[string]string{
				"source_fields":         "short_name,brand_name,legal_name",
				"canonical_id_field":    "vendor_id",
				"canonical_label_field": "legal_name",
				"filter_field":          "status",
				"filter_value":          "active",
				"reason":                "derive source-to-canonical mappings from reference rows",
				"max_records":           "50",
				"max_resolutions":       "20",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run structured normalize action: %v", err)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "vendors.csv" {
		t.Fatalf("ConsumedPaths=%q, want vendors.csv", got)
	}
	if len(res.EntityResolutions) != 3 {
		t.Fatalf("EntityResolutions=%+v, want 3 active source fields", res.EntityResolutions)
	}
	for _, rec := range res.EntityResolutions {
		if rec.CanonicalID.String() != "V001" || rec.CanonicalLabel.String() != "Acme Cloud Ltd" || rec.Status.String() != "resolved" {
			t.Fatalf("bad normalized record: %+v", rec)
		}
		if len(rec.EvidenceRefs) != 1 || rec.EvidenceRefs[0] != "vendors.csv:2" {
			t.Fatalf("bad evidence refs: %+v", rec)
		}
	}
}

func TestActionRunnerNormalizeEntitiesAllowsEmptyIntermediateArtifact(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "status.csv"), []byte("status,code\npaid,1\ncompleted,2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{{Path: "status.csv", Required: true}},
		},
		Actions: []DataAction{{
			ID:         "normalize_status",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"status.csv"},
			Params: map[string]string{
				"source_field":       "missing_source",
				"canonical_id_field": "missing_canonical",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run empty normalize action: %v", err)
	}
	if len(res.EntityResolutions) != 0 {
		t.Fatalf("EntityResolutions=%+v, want empty intermediate artifact", res.EntityResolutions)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].Fields["count"] != "0" {
		t.Fatalf("Artifacts=%+v, want count=0 artifact", res.Artifacts)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "status.csv" {
		t.Fatalf("ConsumedPaths=%q, want status.csv", got)
	}
}

func TestActionRunnerNormalizeEntitiesInfersStructuredFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "entities.csv"), []byte("entity_id,display_name,alias,amount\nE001,Alpha Ltd,Alpha,100\nE002,Beta LLC,Beta,200\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:         "normalize_entities",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"entities.csv"},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run inferred normalize action: %v", err)
	}
	if len(res.EntityResolutions) == 0 {
		t.Fatalf("EntityResolutions empty")
	}
	for _, rec := range res.EntityResolutions {
		if rec.CanonicalID.String() == "" {
			t.Fatalf("CanonicalID empty in inferred record: %+v", rec)
		}
		if rec.SourceValue.String() == "100" || rec.SourceValue.String() == "200" {
			t.Fatalf("numeric field should not be inferred as entity source: %+v", rec)
		}
	}
	if len(res.Artifacts) == 0 || len(res.Artifacts[0].Children) == 0 {
		t.Fatalf("expected inferred artifact children: %+v", res.Artifacts)
	}
	foundInference := false
	for _, child := range res.Artifacts[0].Children {
		if child.Fields["inferred_schema"] == "true" {
			foundInference = true
		}
	}
	if !foundInference {
		t.Fatalf("expected inferred_schema=true in child artifacts: %+v", res.Artifacts[0].Children)
	}
}

func TestActionRunnerCustomTransformUsesExistingRunner(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{{Path: "orders.csv", Required: true}},
		},
		Actions: []DataAction{{
			ID:         "sum_orders",
			Kind:       DataActionCustomTransform,
			InputPaths: []string{"orders.csv"},
			Script: `rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit_result(str(total), output_contract={"format": "plain_single_line", "explanation_allowed": False})`,
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run custom transform action: %v", err)
	}
	if res.Answer != "17" {
		t.Fatalf("Answer=%q, want 17", res.Answer)
	}
}

func TestActionRunnerCustomTransformRejectsUnsafeCSVFieldAliasBeforeExecution(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,category_raw\nA,compute\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:         "bad_fields",
			Kind:       DataActionCustomTransform,
			InputPaths: []string{"orders.csv"},
			Script: `rows = csv_rows("orders.csv")
for row in rows:
    total = row["category_code"]
emit_result("0", output_contract={"format": "plain_single_line", "explanation_allowed": False})`,
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run unexpectedly accepted unsafe CSV field alias")
	}
	if !strings.Contains(err.Error(), "custom_transform field contract failed") ||
		!strings.Contains(err.Error(), `line 3`) ||
		!strings.Contains(err.Error(), `missing field "category_code"`) ||
		!strings.Contains(err.Error(), "category_raw") {
		t.Fatalf("err=%v, want field contract with line and known headers", err)
	}
	violation := ClassifyExecutionError(err.Error())
	if violation.Code != "custom_transform_field_contract" || violation.ScriptLine != 3 {
		t.Fatalf("top violation=%+v, want custom_transform_field_contract line 3", violation)
	}
	if nested := ClassifyExecutionError(errors.Unwrap(err).Error()); nested.Code != "custom_transform_field_contract" || nested.ScriptLine != 3 {
		t.Fatalf("nested violation=%+v, want custom_transform_field_contract line 3", nested)
	}
}

func TestActionRunnerCustomTransformRejectsRequiredDirectoryMaterial(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "evidence"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "evidence", "a.txt"), []byte("alpha"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{{Path: "evidence", Required: true, UsageMode: MaterialUseScriptConsumed}},
		},
		Actions: []DataAction{{
			ID:         "bad_directory",
			Kind:       DataActionCustomTransform,
			InputPaths: []string{"evidence"},
			Script:     `emit_result("ok", output_contract={"format": "plain_single_line", "explanation_allowed": False})`,
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run unexpectedly accepted required directory material")
	}
	if !strings.Contains(err.Error(), "custom_transform material contract failed") ||
		!strings.Contains(err.Error(), "evidence") {
		t.Fatalf("err=%v, want directory material contract rejection", err)
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
		`execute data task: data task script redefines reserved helper "read_text"`:                                                                              "reserved_helper_redefined",
		`data coverage incomplete: required material "rules.md" was not consumed by the script`:                                                                  "required_material_not_consumed",
		`data coverage incomplete: required material "rules.md" uses planner_distilled but distilled_notes is empty`:                                             "planner_distilled_notes_missing",
		`data coverage incomplete: text evidence "scan.txt" for required material "scan.pdf" was not consumed by the script`:                                     "text_evidence_not_consumed",
		`data validation incomplete: result.contributions[0] has unsupported operation "merge"`:                                                                  "unsupported_contribution_operation",
		`NameError: name 'false' is not defined`:                                                                                                                 "python_json_literal_name",
		`ValueError: invalid literal for int() with base 10: 'Q001'`:                                                                                             "numeric_parse_failure",
		`data validation incomplete: result.contributions[0] references unknown rule_id "R03"`:                                                                   "unknown_rule_ref",
		`data validation incomplete: result.entity_resolutions[0] is missing status`:                                                                             "missing_entity_resolution_status",
		`data planning incomplete: plan is too large for one bounded data batch (script_lines=400 required_materials=12)`:                                        "oversized_data_plan",
		`data planning incomplete: actions[] plans must not carry a top-level script (script_lines=505)`:                                                         "action_top_level_script",
		`data reconcile failed: group "A/amount" has no matching contribution records`:                                                                           "reconcile_group_mismatch",
		`data reconcile failed: group "A/amount" expected=9 but contributions sum to 10`:                                                                         "reconcile_sum_mismatch",
		`NameError: name 'add_entity_resolution' is not defined`:                                                                                                 "unknown_runner_helper",
		`ValueError: path was not declared as an input: text_evidence/invoices/ATT-00006.txt`:                                                                    "undeclared_input_path",
		`execute data task: data action failed action_id="extract_1" action_kind="extract_records": open missing.csv: no such file or directory`:                 "data_action_failed",
		`execute data task: data action failed action_id="transform_1" action_kind="custom_transform": ValueError: path was not declared as an input: extra.csv`: "undeclared_input_path",
		`execute data task: data action failed action_id="transform_1" action_kind="custom_transform": AttributeError: 'list' object has no attribute 'get'`:     "json_shape_mismatch",
	}
	for text, want := range cases {
		if got := ClassifyExecutionError(text).Code; got != want {
			t.Fatalf("ClassifyExecutionError(%q)=%q, want %q", text, got, want)
		}
	}
	v := ClassifyExecutionError(`ValueError: path was not declared as an input: text_evidence/invoices/ATT-00006.txt`)
	if v.ActualSnippet != "text_evidence/invoices/ATT-00006.txt" || !strings.Contains(v.RepairHint, "atomic action") {
		t.Fatalf("undeclared path violation=%+v", v)
	}
	v = ClassifyExecutionError(`execute data task: data action failed action_id="extract_1" action_kind="extract_records": open missing.csv: no such file or directory`)
	if v.ActionID != "extract_1" || v.ActionKind != "extract_records" || !strings.Contains(v.RepairHint, "action/node") {
		t.Fatalf("data action violation=%+v", v)
	}
	v = ClassifyExecutionError(`execute data task: data action failed action_id="transform_1" action_kind="custom_transform": ValueError: path was not declared as an input: extra.csv`)
	if v.Code != "undeclared_input_path" || v.ActionID != "transform_1" || v.ActionKind != "custom_transform" || v.ActualSnippet != "extra.csv" {
		t.Fatalf("wrapped undeclared path violation=%+v", v)
	}
}

func TestRunnerAcceptsEntityResolutionHelperAlias(t *testing.T) {
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
add_entity_resolution(item_id="1", source_value=rows[0]["vendor"], canonical_id="A", canonical_label="Vendor A", status="resolved", evidence_refs=["orders.csv"])
emit_result("ok", output_contract={"format": "plain_single_line", "explanation_allowed": False})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.EntityResolutions) != 1 || res.EntityResolutions[0].CanonicalID.String() != "A" {
		t.Fatalf("EntityResolutions=%+v", res.EntityResolutions)
	}
}

func TestClassifyExecutionErrorCarriesScriptLine(t *testing.T) {
	text := `execute data task: data task script failed: exit status 1
Traceback (most recent call last):
  File "/tmp/codrax-data/_runner.py", line 130, in <module>
    exec(code, env, env)
  File "<string>", line 22, in <module>
NameError: name 'status' is not defined`
	v := ClassifyExecutionError(text)
	if v.ScriptLine != 22 {
		t.Fatalf("ScriptLine=%d, want 22", v.ScriptLine)
	}
	if v.RunnerLine != 130 {
		t.Fatalf("RunnerLine=%d, want 130", v.RunnerLine)
	}
	if v.Code != "runtime_failure" {
		t.Fatalf("Code=%q", v.Code)
	}
}

func TestRunnerLedgerHelpersNormalizeStructuralAliases(t *testing.T) {
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
			},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
		},
		Script: `rows = csv_rows("orders.csv")
add_rule_coverage(id="r1", text="include first row", notes="rule applied to parsed rows")
add_decision(row_id="1", source="orders.csv", locator="row_2", status="included", rule="r1")
add_resolution(record_id="1", source="A", canonical_id="A", candidates=["A"], rule_id="r1")
add_contribution(record_id="1", source="orders.csv", locator="row_2", group="A", metric="amount", value=rows[0]["amount"], op="sum", rule="r1")
emit_result("10", output_contract={"format": "plain_single_line", "explanation_allowed": false}, reconcile={"status": "pass", "actual_answer": "10", "groups": [{"group_key": "A", "metric": "amount", "actual": "10"}]})`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := res.Rows[0].RowID; got != "1" {
		t.Fatalf("row id alias not normalized: %q", got)
	}
	if got := res.Rows[0].Decision; got != "included" {
		t.Fatalf("decision status alias not normalized: %q", got)
	}
	if got := res.Contributions[0].Operation.String(); got != "add" {
		t.Fatalf("contribution op alias not normalized: %q", got)
	}
	if got := res.EntityResolutions[0].Status.String(); got != "resolved" {
		t.Fatalf("entity resolution default status=%q, want resolved", got)
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

func TestRunnerAcceptsRelativeTempRoot(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	plan := TaskPlan{
		InputPaths:     []string{"orders.csv"},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{{Path: "orders.csv", Required: true}},
		},
		Script: `rows = csv_rows("orders.csv")
emit_result(rows[0]["amount"], output_contract={"format": "plain_single_line", "explanation_allowed": False})`,
	}
	res, err := (Runner{RepoRoot: root, TempRoot: ".codrax/data"}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run with relative TempRoot: %v", err)
	}
	if res.Answer != "10" {
		t.Fatalf("answer=%q, want 10", res.Answer)
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
    {"item_id": "1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "add", "rule_refs": ["r1"]},
    {"item_id": "2", "source": "orders.csv", "source_locator": "row 3", "group_key": "A", "metric": "amount", "value": "7", "operation": "add", "rule_refs": ["r1"]}
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

func TestValidateReconcileAllowsAuditGroupsOutsideTargetContributions(t *testing.T) {
	res := Result{
		Answer:         "10",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Contributions: []ContributionRecord{{
			ItemID: "row-1", Source: "orders.csv", SourceLocator: "line:2", GroupKey: "Q001", Metric: "amount", Value: "10", Operation: "add", Role: "target",
		}},
		Reconcile: &ReconcileReport{
			Status: "pass",
			Groups: []ReconcileGroup{
				{GroupKey: "Q001", Metric: "amount", Expected: "10", Actual: "10"},
				{GroupKey: "overall", Metric: "query_count", Role: "audit", Expected: "12", Actual: "12"},
				{Role: "answer", Expected: "10", Actual: "10"},
			},
		},
	}
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	if _, err := validateRunnerResult(plan, res); err != nil {
		t.Fatalf("validateRunnerResult: %v", err)
	}
	res.Reconcile.Groups[1].Role = ""
	_, err := validateRunnerResult(plan, res)
	if err == nil || !strings.Contains(err.Error(), "overall/query_count") {
		t.Fatalf("validateRunnerResult err=%v, want non-audit reconcile group mismatch", err)
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

func TestRunnerNormalizesRawLedgerStructuralAliases(t *testing.T) {
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
			DecisionRecordsRequired:    true,
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
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "rows": [{"item_id": "row1", "source": "orders.csv", "locator": "row 2", "status": "included", "rule": "r1"}],
  "rule_coverage": [{"id": "r1", "text": "include selected rows", "outcome": "applied", "summary": "row 2 selected"}],
  "entity_resolutions": [{"record_id": "row1", "raw_value": "A", "canonical": "A", "candidates": ["A"], "rule": "r1"}],
  "contributions": [{"record_id": "row1", "source": "orders.csv", "locator": "row 2", "group": "A/amount", "measure": "amount", "value": "10", "op": "sum", "rule": "r1"}],
  "reconcile": {"status": "pass", "actual_answer": "10", "groups": [{"group": "A/amount", "measure": "amount", "actual_value": "10"}]}
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := res.Rows[0].RowID; got != "row1" {
		t.Fatalf("row alias not normalized: %q", got)
	}
	if got := res.RuleCoverage[0].RuleID.String(); got != "r1" {
		t.Fatalf("rule alias not normalized: %q", got)
	}
	if got := res.EntityResolutions[0].Status.String(); got != "resolved" {
		t.Fatalf("resolution status=%q, want resolved", got)
	}
	if got := res.Contributions[0].GroupKey.String(); got != "A" {
		t.Fatalf("group alias not normalized: %q", got)
	}
	if got := res.Reconcile.Groups[0].Actual.String(); got != "10" {
		t.Fatalf("reconcile actual alias not normalized: %q", got)
	}
	if len(res.ResultPatches) < 3 {
		t.Fatalf("ResultPatches=%+v, want structural patch audit records", res.ResultPatches)
	}
	patchPaths := map[string]bool{}
	for _, patch := range res.ResultPatches {
		patchPaths[patch.Path] = true
		if patch.Target != "result" || patch.Op != "replace" {
			t.Fatalf("unexpected patch=%+v", patch)
		}
	}
	for _, want := range []string{"/entity_resolutions/0/status", "/contributions/0/group_key", "/contributions/0/operation", "/reconcile/groups/0/group_key"} {
		if !patchPaths[want] {
			t.Fatalf("patch paths=%v, missing %s", patchPaths, want)
		}
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

func TestRunnerRowsExposeGenericSourceMetadata(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths:     []string{"orders.csv"},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
emit_result(rows[0]["_source"] + "|" + rows[0]["_source_index"] + "|" + rows[0]["_source_locator"], output_contract={"format": "plain_single_line", "explanation_allowed": False})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := res.Answer, "orders.csv|1|line:2"; got != want {
		t.Fatalf("Answer=%q, want %q", got, want)
	}
}

func TestRunnerDerivesDecisionRowsFromContributionLedger(t *testing.T) {
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
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
add_rule_coverage(rule_id="r1", rule_text="include all rows", status="applied")
add_contribution(item_id="row1", source=rows[0]["_source"], source_locator=rows[0]["_source_locator"], group_key="A", metric="amount", value=rows[0]["amount"], operation="sum", reason="included by r1", rule_refs=["r1"])
emit_result("10", output_contract={"format": "plain_single_line", "explanation_allowed": False})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Decision != "include" || res.Rows[0].SourceLocator != "line:2" {
		t.Fatalf("derived rows=%+v", res.Rows)
	}
	if len(res.RuleCoverage) != 1 || strings.TrimSpace(res.RuleCoverage[0].Notes.String()) == "" {
		t.Fatalf("rule coverage should carry helper support note: %+v", res.RuleCoverage)
	}
	if res.Reconcile == nil || len(res.Reconcile.Groups) != 1 {
		t.Fatalf("reconcile not derived from contribution: %+v", res.Reconcile)
	}
}

func TestRunnerEmitMergesHelperLedgers(t *testing.T) {
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
add_rule_coverage(rule_id="r1", rule_text="include all rows", status="applied", notes="all rows read")
add_resolution(source_value="A", canonical_id="A", canonical_label="A", status="resolved")
total = Decimal("0")
for i, row in enumerate(rows):
    total += Decimal(row["amount"])
    add_decision(item_id=str(i), source="orders.csv", source_locator=f"row_{i+2}", decision="included", reason="included by r1", rule_refs=["r1"])
    add_contribution(item_id=str(i), source="orders.csv", source_locator=f"row_{i+2}", group_key=row["vendor"], metric="amount", value=row["amount"], operation="add", reason="included by r1", rule_refs=["r1"])
emit({"answer": str(int(total)), "output_contract": {"format": "plain_single_line", "explanation_allowed": False}, "audit_summary": "direct emit keeps helper ledgers", "reconcile": {"status": "pass", "groups": [{"group_key": "A", "metric": "amount", "actual": "17"}]}})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17" || len(res.Rows) != 2 || len(res.Contributions) != 2 || len(res.RuleCoverage) != 1 || len(res.EntityResolutions) != 1 {
		t.Fatalf("direct emit did not merge helper ledgers: %+v", res)
	}
}

func TestRunnerEmptyExplicitLedgersDoNotOverrideHelperLedgers(t *testing.T) {
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
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
total = Decimal("0")
for i, row in enumerate(rows):
    total += Decimal(row["amount"])
    add_decision(item_id=str(i), source="orders.csv", source_locator=f"row_{i+2}", decision="included", reason="included")
    add_contribution(item_id=str(i), source="orders.csv", source_locator=f"row_{i+2}", group_key=row["vendor"], metric="amount", value=row["amount"], operation="add", reason="included")
emit_result(str(int(total)), output_contract={"format": "plain_single_line", "explanation_allowed": False}, rows=[], contributions=[], reconcile={"status": "pass", "groups": []})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17" || len(res.Rows) != 2 || len(res.Contributions) != 2 {
		t.Fatalf("helper ledgers were overwritten by explicit empty lists: %+v", res)
	}
	if res.Reconcile == nil || len(res.Reconcile.Groups) != 1 || res.Reconcile.Groups[0].Actual.String() != "17" {
		t.Fatalf("reconcile not filled from contribution ledger: %+v", res.Reconcile)
	}
}

func TestRunnerLedgerHelpersSupportOptionalLocalListSink(t *testing.T) {
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
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Script: `
rows = csv_rows("orders.csv")
local_rows = []
local_rules = []
local_contribs = []
add_rule_coverage(local_rules, rule_id="r1", rule_text="include rows", status="applied")
total = Decimal("0")
for i, row in enumerate(rows):
    total += Decimal(row["amount"])
    add_decision(local_rows, item_id=str(i), source="orders.csv", source_locator=f"row_{i+2}", decision="include", reason="included", rule_refs=["r1"])
    add_contribution(local_contribs, item_id=str(i), source="orders.csv", source_locator=f"row_{i+2}", group_key=row["vendor"], metric="amount", value=row["amount"], operation="sum", reason="included", rule_refs=["r1"])
emit_result(str(int(total)), output_contract={"format": "plain_single_line", "explanation_allowed": False}, rows=local_rows, rule_coverage=local_rules, contributions=local_contribs, reconcile={"status": "pass", "groups": [{"group_key": "A", "metric": "amount", "expected": "17", "actual": "17"}]})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17" || len(res.Rows) != 2 || len(res.Contributions) != 2 || len(res.RuleCoverage) != 1 {
		t.Fatalf("optional local sink ledgers not preserved: %+v", res)
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

func TestRunnerRejectsUnlinkedRuleCoverageWhenLedgersAreRequired(t *testing.T) {
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
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Script: `
rows = csv_rows("orders.csv")
amount = rows[0]["amount"]
add_rule_coverage(rule_id="r1", rule_text="include selected rows", status="applied", evidence_refs=["orders.csv:1"])
add_contribution(item_id="row1", source="orders.csv", source_locator="row 2", group_key="A", metric="amount", value=amount, operation="add", reason="selected")
emit_result(amount, output_contract={"format": "plain_single_line", "explanation_allowed": False}, reconcile={"status": "pass", "groups": [{"group_key": "A", "metric": "amount", "expected": amount, "actual": amount}]})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "no decision/contribution/entity record links") {
		t.Fatalf("Run err=%v, want unlinked rule coverage", err)
	}
	plan.Script = `
rows = csv_rows("orders.csv")
amount = rows[0]["amount"]
add_rule_coverage(rule_id="r1", rule_text="include selected rows", status="applied", evidence_refs=["orders.csv:1"])
add_contribution(item_id="row1", source="orders.csv", source_locator="row 2", group_key="A", metric="amount", value=amount, operation="add", reason="selected", rule_refs=["r1"])
emit_result(amount, output_contract={"format": "plain_single_line", "explanation_allowed": False}, reconcile={"status": "pass", "groups": [{"group_key": "A", "metric": "amount", "expected": amount, "actual": amount}]})
`
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run linked rule coverage: %v", err)
	}
	if res.Answer != "10" {
		t.Fatalf("Answer=%q, want 10", res.Answer)
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

func TestRunnerAllowsAnswerScopedReconcileGroup(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("query,amount\nQ001,10\nQ002,5\n"), 0600); err != nil {
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
  "answer": "10,5",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [
    {"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "Q001", "metric": "amount", "value": "10", "operation": "add"},
    {"item_id": "row2", "source": "orders.csv", "source_locator": "row 3", "group_key": "Q002", "metric": "amount", "value": "5", "operation": "add"}
  ],
  "reconcile": {"status": "pass", "groups": [{"scope": "answer", "group_key": "final", "metric": "payload", "expected": "10,5", "actual": "10,5"}]}
})
`,
	}
	res, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "10,5" {
		t.Fatalf("Answer=%q", res.Answer)
	}
}

func TestRunnerInfersAnswerScopedReconcileGroupFromFinalAnswerValue(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("query,amount\nQ001,10\nQ002,5\n"), 0600); err != nil {
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
  "answer": "10,5",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [
    {"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "Q001", "metric": "amount", "value": "10", "operation": "add"},
    {"item_id": "row2", "source": "orders.csv", "source_locator": "row 3", "group_key": "Q002", "metric": "amount", "value": "5", "operation": "add"}
  ],
  "reconcile": {"status": "pass", "groups": [{"group_key": "total", "metric": "payload", "expected": "10,5", "actual": "10,5"}]}
})
`,
	}
	if _, err := (Runner{RepoRoot: root}).Run(context.Background(), plan); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunnerValidatesFinalAnswerAgainstOrdinaryReconcileGroups(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("query,amount\nQ001,10\nQ002,5\n"), 0600); err != nil {
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
  "answer": "10,5",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [
    {"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "Q001", "metric": "amount", "value": "10", "operation": "add"},
    {"item_id": "row2", "source": "orders.csv", "source_locator": "row 3", "group_key": "Q002", "metric": "amount", "value": "5", "operation": "add"}
  ],
  "reconcile": {"status": "pass", "groups": [
    {"group_key": "Q001", "metric": "amount", "expected": "10", "actual": "10"},
    {"group_key": "Q002", "metric": "amount", "expected": "5", "actual": "5"}
  ]}
})
`,
	}
	if _, err := (Runner{RepoRoot: root}).Run(context.Background(), plan); err != nil {
		t.Fatalf("Run matching answer: %v", err)
	}
	plan.Script = strings.Replace(plan.Script, `"answer": "10,5"`, `"answer": "0,0"`, 1)
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "result.answer values") {
		t.Fatalf("Run err=%v, want answer/reconcile mismatch", err)
	}
}

func TestActionRunnerAssembleAnswerProjectsReconcileGroups(t *testing.T) {
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{ID: "reconcile", Kind: DataActionReconcile},
			{
				ID:   "answer",
				Kind: DataActionAssembleAnswer,
				Params: map[string]string{
					"projection": "values",
					"order_by":   "group_key",
					"delimiter":  ",",
				},
			},
		},
	}
	seed := Result{Contributions: []ContributionRecord{
		{
			ItemID:        LooseText("item10"),
			Source:        LooseText("records.csv"),
			SourceLocator: LooseText("line:10"),
			GroupKey:      LooseText("Q10"),
			Metric:        LooseText("amount"),
			Value:         LooseText("20"),
			Operation:     LooseText("add"),
			Role:          LooseText("target"),
		},
		{
			ItemID:        LooseText("item2"),
			Source:        LooseText("records.csv"),
			SourceLocator: LooseText("line:2"),
			GroupKey:      LooseText("Q2"),
			Metric:        LooseText("amount"),
			Value:         LooseText("10"),
			Operation:     LooseText("add"),
			Role:          LooseText("target"),
		},
	}}
	res, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "10,20" {
		t.Fatalf("Answer=%q, want natural group-key value projection", res.Answer)
	}
	if res.Reconcile == nil || res.Reconcile.ActualAnswer.String() != "10,20" {
		t.Fatalf("Reconcile=%+v, want projected actual answer", res.Reconcile)
	}
	if len(res.Artifacts) == 0 || res.Artifacts[len(res.Artifacts)-1].Kind != string(DataActionAssembleAnswer) {
		t.Fatalf("Artifacts=%+v, want assemble_answer artifact", res.Artifacts)
	}
}

func TestRunnerRejectsMismatchedAnswerScopedReconcileGroup(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("query,amount\nQ001,10\n"), 0600); err != nil {
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
  "contributions": [{"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "Q001", "metric": "amount", "value": "10", "operation": "add"}],
  "reconcile": {"status": "pass", "groups": [{"scope": "answer", "group_key": "final", "metric": "payload", "expected": "11", "actual": "11"}]}
})
`,
	}
	_, err := (Runner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "answer-scope group") {
		t.Fatalf("Run err=%v, want answer-scope mismatch", err)
	}
}

func TestRunnerIgnoresAuditContributionForMissingReconcileGroup(t *testing.T) {
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
  "contributions": [
    {"item_id": "row1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "add", "role": "target"},
    {"item_id": "orders.csv#sample", "source": "orders.csv", "source_locator": "sample", "group_key": "all", "metric": "count", "value": "1", "operation": "count", "role": "audit"}
  ],
  "reconcile": {"status": "pass", "expected_answer": "10", "actual_answer": "10", "groups": [{"group_key": "A", "metric": "amount", "expected": "10", "actual": "10"}]}
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
	var validationErr DataValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Run err=%T %v, want DataValidationError", err, err)
	}
	if len(validationErr.Violations) != 1 || validationErr.Violations[0].Code != "unsupported_open_entity_resolution" {
		t.Fatalf("violations=%+v, want unsupported_open_entity_resolution", validationErr.Violations)
	}
	if validationErr.Violations[0].JSONPath != "/entity_resolutions/0" {
		t.Fatalf("json_path=%q", validationErr.Violations[0].JSONPath)
	}
	if validationErr.Violations[0].Repairability != RepairabilityNeedsRecompute {
		t.Fatalf("repairability=%q", validationErr.Violations[0].Repairability)
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

func TestActionRunnerCustomTransformUsesNodeCoverageAndWorkflowCoverage(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("beta"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "a.txt", Required: true},
				{Path: "b.txt", Required: true},
			},
		},
		Actions: []DataAction{
			{
				ID:         "inspect_b",
				Kind:       DataActionInspectMaterial,
				InputPaths: []string{"b.txt"},
			},
			{
				ID:         "transform_a",
				Kind:       DataActionCustomTransform,
				InputPaths: []string{"a.txt"},
				Script: `
text = read_text("a.txt")
emit_result("ok", output_contract={"format": "plain_single_line", "explanation_allowed": False}, audit_summary=text)
`,
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("ActionRunner.Run: %v", err)
	}
	if res.Answer != "ok" {
		t.Fatalf("answer=%q, want ok", res.Answer)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "a.txt,b.txt" {
		t.Fatalf("ConsumedPaths=%q, want a.txt,b.txt", got)
	}
}

func TestApplyDataResultPatchPlanRepairsStructuralOperationDrift(t *testing.T) {
	plan := TaskPlan{
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
	}
	base := Result{
		Answer:         "10",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Contributions: []ContributionRecord{{
			ItemID:        "row-1",
			Source:        "orders.csv",
			SourceLocator: "row 2",
			GroupKey:      "A/amount",
			Metric:        "amount",
			Value:         "10",
			Operation:     "totalize",
			Reason:        "contributes to total",
		}},
		Reconcile: &ReconcileReport{
			Status:       "pass",
			ActualAnswer: "10",
			Groups: []ReconcileGroup{{
				GroupKey: "A/amount",
				Metric:   "amount",
				Expected: "10",
				Actual:   "10",
			}},
		},
	}
	_, err := validateRunnerResult(plan, base)
	if err == nil {
		t.Fatal("validateRunnerResult unexpectedly accepted unsupported operation")
	}
	var validationErr *DataResultValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("err=%T %v, want DataResultValidationError", err, err)
	}
	patched, patches, err := ApplyDataResultPatchPlan(plan, validationErr.Result, DataResultPatchPlan{
		Patches: []DataResultPatch{
			newDataResultPatch("replace", "/contributions/0/operation", "add", "normalize structural aggregation operation"),
		},
	})
	if err != nil {
		t.Fatalf("ApplyDataResultPatchPlan: %v", err)
	}
	if patched.Contributions[0].Operation.String() != "add" {
		t.Fatalf("operation=%q, want add", patched.Contributions[0].Operation.String())
	}
	if patched.Contributions[0].GroupKey.String() != "A" {
		t.Fatalf("group_key=%q, want metric suffix normalized", patched.Contributions[0].GroupKey.String())
	}
	if len(patches) < 1 || len(patched.ResultPatches) < 1 {
		t.Fatalf("patches=%v result_patches=%v, want audit records", patches, patched.ResultPatches)
	}
}

func TestApplyDataResultPatchPlanRejectsAnswerPatch(t *testing.T) {
	plan := TaskPlan{OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false}}
	base := Result{Answer: "10", OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false}}
	_, _, err := ApplyDataResultPatchPlan(plan, base, DataResultPatchPlan{
		Patches: []DataResultPatch{
			newDataResultPatch("replace", "/answer", "11", "answer changes are not structural"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "outside the safe structural patch set") {
		t.Fatalf("ApplyDataResultPatchPlan err=%v, want answer patch rejection", err)
	}
}

func TestApplyDataResultPatchPlanRejectsSemanticReconcileStatusPatch(t *testing.T) {
	plan := TaskPlan{CoverageContract: CoverageContract{ReconcileRequired: true}}
	base := Result{
		Answer: "10",
		Reconcile: &ReconcileReport{
			Status:      "fail",
			Differences: []string{"mismatch"},
		},
	}
	_, _, err := ApplyDataResultPatchPlan(plan, base, DataResultPatchPlan{
		Patches: []DataResultPatch{
			newDataResultPatch("replace", "/reconcile/status", "pass", "hiding reconcile failure is not structural"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "only fills a missing structural status") {
		t.Fatalf("ApplyDataResultPatchPlan err=%v, want reconcile status patch rejection", err)
	}
}

func TestApplyDataResultPatchPlanRejectsRemoveMove(t *testing.T) {
	plan := TaskPlan{OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false}}
	base := Result{
		Answer:         "10",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Contributions: []ContributionRecord{{
			ItemID: "row-1", GroupKey: "A", Metric: "amount", Value: "10", Operation: "add",
		}},
	}
	for _, op := range []string{"remove", "move"} {
		_, _, err := ApplyDataResultPatchPlan(plan, base, DataResultPatchPlan{
			Patches: []DataResultPatch{{
				Target: "result",
				Op:     op,
				Path:   "/contributions/0/metric",
				Reason: "duplicate wrapper cleanup",
			}},
		})
		if err == nil || !strings.Contains(err.Error(), "only replace existing structural scalar fields") {
			t.Fatalf("ApplyDataResultPatchPlan op=%q err=%v, want remove/move rejection", op, err)
		}
	}
}

func TestActionRunnerLedgerOnlyBatchPreservesSeedAnswer(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "entities.csv"), []byte("id,name\nA,Alpha\n"), 0600); err != nil {
		t.Fatal(err)
	}
	seed := Result{
		Answer:         "42",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Contributions: []ContributionRecord{{
			ItemID: "row-1", Source: "orders.csv", SourceLocator: "line:2", GroupKey: "all", Metric: "amount", Value: "42", Operation: "add", Reason: "seed",
		}},
	}
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			EntityResolutionRequired:   true,
			ContributionLedgerRequired: true,
		},
		Actions: []DataAction{{
			ID:         "complete_entities",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"entities.csv"},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("ActionRunner.Run: %v", err)
	}
	if res.Answer != "42" {
		t.Fatalf("Answer=%q, want preserved seed answer", res.Answer)
	}
	if len(res.EntityResolutions) == 0 {
		t.Fatal("entity resolutions not completed")
	}
}

func TestValidateRuleCoverageRequiresSourceBackedLinksWhenAvailable(t *testing.T) {
	plan := TaskPlan{
		CoverageContract: CoverageContract{
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
		},
	}
	res := Result{
		RuleCoverage: []RuleCoverageRecord{
			{RuleID: "source_rule", RuleText: "from source", Status: "derived", EvidenceRefs: []string{"rules.md:1"}},
			{RuleID: "local_rule", RuleText: "local", Status: "applied", Notes: "model generated"},
		},
		Contributions: []ContributionRecord{{
			ItemID: "row-1", Source: "orders.csv", SourceLocator: "line:2", GroupKey: "all", Metric: "amount", Value: "7", Operation: "add", Reason: "linked only to local", RuleRefs: []string{"local_rule"},
		}},
	}
	_, err := validateRunnerResult(plan, res)
	if err == nil || !strings.Contains(err.Error(), "source-backed rule coverage") {
		t.Fatalf("validateRunnerResult err=%v, want source-backed rule link failure", err)
	}
	res.Contributions[0].RuleRefs = []string{"source_rule"}
	if _, err := validateRunnerResult(plan, res); err != nil {
		t.Fatalf("validateRunnerResult with source-backed rule ref: %v", err)
	}
}
