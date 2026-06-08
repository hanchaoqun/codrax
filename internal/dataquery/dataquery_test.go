package dataquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestActionRunnerValueDistribution(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("status,group\npaid,A\npaid,B\naccepted,A\n,A\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status: "ready",
		Actions: []DataAction{{
			ID:         "dist",
			Kind:       DataActionValueDistribution,
			InputPaths: []string{"items.csv"},
			Params: map[string]string{
				"fields": `["status","group"]`,
				"top_n":  "2",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run value_distribution: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%d, want 1", len(res.Artifacts))
	}
	artifact := res.Artifacts[0]
	if artifact.Kind != string(DataActionValueDistribution) || strings.Join(artifact.SourceRecordPaths, ",") != "items.csv" {
		t.Fatalf("artifact=%+v, want value distribution with source record lineage", artifact)
	}
	var statusChild *DataArtifact
	for i := range artifact.Children {
		if artifact.Children[i].Fields["field"] == "status" {
			statusChild = &artifact.Children[i]
			break
		}
	}
	if statusChild == nil {
		t.Fatalf("children=%+v, want status distribution child", artifact.Children)
	}
	if statusChild.Fields["non_empty"] != "3" || statusChild.Fields["empty"] != "1" || statusChild.Fields["distinct"] != "2" || !strings.Contains(statusChild.Fields["top_values"], `"paid"`) {
		t.Fatalf("status distribution=%+v, want counts/top values", statusChild.Fields)
	}
}

func TestActionRunnerMappingCandidateProducesCandidateArtifactOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_label\n1,Alpha\n2,Beta\n3,Gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.csv"), []byte("code,label\nA,Alpha\nB,Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status: "ready",
		Actions: []DataAction{{
			ID:             "candidates",
			Kind:           DataActionMappingCandidate,
			InputPaths:     []string{"items.csv", "reference.csv"},
			OutputArtifact: "candidate_rows",
			Params: map[string]string{
				"source_field":          "raw_label",
				"reference_name_fields": `["label"]`,
				"canonical_id_field":    "code",
				"canonical_label_field": "label",
				"match_mode":            "exact",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run mapping_candidate: %v", err)
	}
	if len(res.EntityResolutions) != 0 {
		t.Fatalf("EntityResolutions=%+v, want candidate action not to satisfy entity ledger", res.EntityResolutions)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%d, want 1", len(res.Artifacts))
	}
	artifact := res.Artifacts[0]
	if artifact.Kind != string(DataActionMappingCandidate) || artifact.RowCount != 3 {
		t.Fatalf("artifact=%+v, want mapping_candidate artifact with 3 rows", artifact)
	}
	if artifact.Fields["matched_sources"] != "2" || artifact.Fields["unresolved_sources"] != "1" {
		t.Fatalf("fields=%+v, want matched/unresolved source counts", artifact.Fields)
	}
	if strings.Join(res.ConsumedPaths, ",") != "items.csv,reference.csv" {
		t.Fatalf("ConsumedPaths=%v, want source and reference paths", res.ConsumedPaths)
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
	artifactRoot := filepath.Join(root, ".actions")
	res, err := (ActionRunner{RepoRoot: root, TempRoot: artifactRoot}).Run(context.Background(), plan)
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
	artifactRoot := filepath.Join(root, ".actions")
	res, err := (ActionRunner{RepoRoot: root, TempRoot: artifactRoot}).Run(context.Background(), plan)
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

func TestActionRunnerExtractRecordsAcceptsMaxRecordsAlias(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	b.WriteString("id,amount\n")
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "%d,%d\n", i, i*10)
	}
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputJSONOnly, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "extract",
			Kind:           DataActionExtractRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "records",
			Params:         map[string]string{"max_records": "30"},
		}},
	})
	if err != nil {
		t.Fatalf("ActionRunner.Run: %v", err)
	}
	if len(res.Artifacts) != 1 || len(res.Artifacts[0].Children) != 1 {
		t.Fatalf("artifacts=%+v", res.Artifacts)
	}
	child := res.Artifacts[0].Children[0]
	if child.RowCount != 30 || len(child.Sample) != 30 {
		t.Fatalf("child row_count=%d sample=%d, want 30/30", child.RowCount, len(child.Sample))
	}
	if got := res.Artifacts[0].Fields["limit"]; got != "30" {
		t.Fatalf("parent limit=%q, want 30", got)
	}
	if got := child.Fields["record_completeness"]; got != "complete" {
		t.Fatalf("child record_completeness=%q, want complete", got)
	}
	if got := res.Artifacts[0].Fields["record_completeness"]; got != "complete" {
		t.Fatalf("parent record_completeness=%q, want complete", got)
	}
}

func TestActionRunnerExtractRecordsMarksSampledArtifacts(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	b.WriteString("id,amount\n")
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "%d,%d\n", i, i*10)
	}
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputJSONOnly, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "extract",
			Kind:           DataActionExtractRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "records",
			Params:         map[string]string{"limit": "10"},
		}},
	})
	if err != nil {
		t.Fatalf("ActionRunner.Run: %v", err)
	}
	child := res.Artifacts[0].Children[0]
	if got := child.Fields["record_completeness"]; got != "sampled" {
		t.Fatalf("child record_completeness=%q, want sampled", got)
	}
	if got := child.Fields["total_rows"]; got != "30" {
		t.Fatalf("child total_rows=%q, want 30", got)
	}
	if got := child.Fields["sample_count"]; got != "10" {
		t.Fatalf("child sample_count=%q, want 10", got)
	}
	if got := res.Artifacts[0].Fields["record_completeness"]; got != "sampled" {
		t.Fatalf("parent record_completeness=%q, want sampled", got)
	}
	if got := res.Artifacts[0].Fields["incomplete_child_count"]; got != "1" {
		t.Fatalf("incomplete_child_count=%q, want 1", got)
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
	artifactRoot := filepath.Join(root, ".actions")
	res, err := (ActionRunner{RepoRoot: root, TempRoot: artifactRoot}).Run(context.Background(), plan)
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

func TestActionRunnerTypedActionConsumesSourceRecordAliasAcrossBatches(t *testing.T) {
	root := t.TempDir()
	tempRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,amount\n1,10\n2,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "queries.csv"), []byte("id,label\n1,A\n2,B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seed, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		ContinueAfter:  true,
		Actions: []DataAction{{
			ID:             "extract_all",
			Kind:           DataActionExtractRecords,
			InputPaths:     []string{"orders.csv", "queries.csv"},
			OutputArtifact: "records",
			Params:         map[string]string{"limit": "10"},
		}},
	})
	if err != nil {
		t.Fatalf("seed Run: %v", err)
	}
	res, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot, Seed: seed}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "join_alias_records",
			Kind:           DataActionJoinRecords,
			InputPaths:     []string{"orders.csv#records", "queries.csv#records"},
			OutputArtifact: "joined",
			Params: map[string]string{
				"left_fields":  "id",
				"right_fields": "id",
			},
		}},
	})
	if err != nil {
		t.Fatalf("join_records consumed source #records aliases across batches: %v", err)
	}
	if len(res.Artifacts) == 0 || res.Artifacts[len(res.Artifacts)-1].RowCount != 2 {
		t.Fatalf("Artifacts=%+v, want joined two rows", res.Artifacts)
	}
	got := strings.Join(res.ConsumedPaths, ",")
	if !strings.Contains(got, "orders.csv#records") || !strings.Contains(got, "queries.csv#records") {
		t.Fatalf("ConsumedPaths=%q, want logical artifact aliases", got)
	}
}

func TestActionRunnerJoinRecordsAcceptsKeyFieldAliases(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "left.csv"), []byte("id,value\nA,3\nB,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "right.csv"), []byte("id,label\nA,alpha\nB,beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "join_key_aliases",
			Kind:           DataActionJoinRecords,
			InputPaths:     []string{"left.csv", "right.csv"},
			OutputArtifact: "joined",
			Params: map[string]string{
				"left_key_fields":  `["id"]`,
				"right_key_fields": `["id"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Artifacts) == 0 || res.Artifacts[len(res.Artifacts)-1].RowCount != 2 {
		t.Fatalf("Artifacts=%+v, want joined two rows", res.Artifacts)
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

func TestActionRunnerFilterRecordsFeedsContributionAction(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,vendor,amount,status\n1,A,10,paid\n2,B,5,pending\n3,A,7,accepted\n4,C,3,cancelled\n"), 0o644); err != nil {
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
				ID:             "filter_complete",
				Kind:           DataActionFilterRecords,
				InputPaths:     []string{"orders.csv"},
				OutputArtifact: "complete_orders",
				Params: map[string]string{
					"filters_json": `[{"field":"status","op":"in","value":["paid","accepted"]}]`,
				},
			},
			{
				ID:         "sum_complete",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"complete_orders"},
				Params: map[string]string{
					"group_key_field": "vendor",
					"metric":          "amount",
					"value_field":     "amount",
					"operation":       "add",
					"item_id_field":   "id",
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run filter_records actions: %v", err)
	}
	var filterArtifact DataArtifact
	for _, artifact := range res.Artifacts {
		if artifact.ID == "complete_orders" {
			filterArtifact = artifact
			break
		}
	}
	if filterArtifact.RowCount != 2 || filterArtifact.Kind != string(DataActionFilterRecords) {
		t.Fatalf("filter artifact=%+v, want 2 filtered rows", filterArtifact)
	}
	if len(res.Contributions) != 2 || res.Answer != "17" {
		t.Fatalf("Answer=%q Contributions=%+v, want sum 17 from two rows", res.Answer, res.Contributions)
	}
	gotConsumed := strings.Join(res.ConsumedPaths, ",")
	if !strings.Contains(gotConsumed, "orders.csv") || !strings.Contains(gotConsumed, "complete_orders") {
		t.Fatalf("ConsumedPaths=%q, want source plus filtered artifact", gotConsumed)
	}
}

func TestActionRunnerQualifyRecordsFeedsContributionAction(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,label,label_status,label_evidence,amount\n1,A,matched,ref.csv:1,10\n2,B,matched_ambiguous,ref.csv:2,5\n3,C,matched,ref.csv:3,7\n4,D,unresolved,,3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:             "qualify_items",
				Kind:           DataActionQualifyRecords,
				InputPaths:     []string{"items.csv"},
				OutputArtifact: "eligible_items",
				Params: map[string]string{
					"item_id_field":   "id",
					"status_fields":   `["label_status"]`,
					"evidence_fields": `["label_evidence"]`,
					"output_mode":     "filter",
				},
			},
			{
				ID:         "sum_eligible",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"eligible_items"},
				Params: map[string]string{
					"group_key":     "all",
					"metric":        "amount",
					"value_field":   "amount",
					"operation":     "add",
					"item_id_field": "id",
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run qualify_records actions: %v", err)
	}
	var qualifyArtifact DataArtifact
	for _, artifact := range res.Artifacts {
		if artifact.ID == "eligible_items" {
			qualifyArtifact = artifact
			break
		}
	}
	if qualifyArtifact.RowCount != 2 || qualifyArtifact.Kind != string(DataActionQualifyRecords) {
		t.Fatalf("qualify artifact=%+v, want 2 eligible rows", qualifyArtifact)
	}
	if len(res.Rows) != 6 {
		t.Fatalf("Rows=%+v, want 4 qualification decisions plus 2 contribution decisions", res.Rows)
	}
	if len(res.Contributions) != 2 || res.Answer != "17" {
		t.Fatalf("Answer=%q Contributions=%+v, want sum 17 from eligible rows", res.Answer, res.Contributions)
	}
}

func TestActionRunnerQualifyRecordsAutoBlocksResolutionStatus(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,canonical_id,resolution_status,amount\n1,A,resolved,10\n2,,unmatched,5\n3,,not_applicable,7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "qualify",
			Kind:           DataActionQualifyRecords,
			InputPaths:     []string{"items.csv"},
			OutputArtifact: "eligible_items",
			Params: map[string]string{
				"output_mode":   "filter",
				"item_id_field": "id",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run qualify_records auto status fields: %v", err)
	}
	var artifact DataArtifact
	for _, candidate := range res.Artifacts {
		if candidate.ID == "eligible_items" {
			artifact = candidate
			break
		}
	}
	if artifact.RowCount != 1 || artifact.Fields["status_fields"] != "resolution_status" {
		t.Fatalf("artifact=%+v, want only resolved row and auto status field", artifact)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("Rows=%+v, want one decision per input row", res.Rows)
	}
	if res.Rows[1].Decision != "exclude" || !strings.Contains(res.Rows[1].Reason, "status_blocked:resolution_status") {
		t.Fatalf("Rows=%+v, want unmatched row excluded by resolution status", res.Rows)
	}
}

func TestActionRunnerGroupRecordsThenExtractsCrossLineFields(t *testing.T) {
	root := t.TempDir()
	input := strings.Join([]string{
		"doc_id,text_clean",
		"D1,Record No: R-001",
		"D1,Related Item: ITEM-42",
		"D1,Total: USD 123",
		"D2,Record No: R-002",
		"D2,Related Item: ITEM-99",
		"D2,Total: CNY 456",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "lines.csv"), []byte(input+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{
			{
				ID:             "group_lines",
				Kind:           DataActionGroupRecords,
				InputPaths:     []string{"lines.csv"},
				OutputArtifact: "documents",
				Params: map[string]string{
					"group_field":  "doc_id",
					"text_fields":  `["text_clean"]`,
					"target_field": "document_text",
				},
			},
			{
				ID:             "extract_document_fields",
				Kind:           DataActionExtractFields,
				InputPaths:     []string{"documents"},
				OutputArtifact: "document_fields",
				Params: map[string]string{
					"field_specs": `[
						{"source_field":"document_text","target_field":"item_id","operation":"regex_extract","pattern":"Related Item: ([A-Z0-9-]+)","group":1},
						{"source_field":"document_text","target_field":"currency","operation":"regex_extract","pattern":"Total: ([A-Z]{3}) ([0-9]+)","group":1},
						{"source_field":"document_text","target_field":"amount","operation":"regex_extract","pattern":"Total: ([A-Z]{3}) ([0-9]+)","group":2}
					]`,
					"required_fields": `["doc_id","item_id","currency","amount"]`,
				},
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run group_records/extract_fields: %v", err)
	}
	var grouped, extracted DataArtifact
	for _, artifact := range res.Artifacts {
		switch artifact.ID {
		case "documents":
			grouped = artifact
		case "document_fields":
			extracted = artifact
		}
	}
	if grouped.RowCount != 2 || extracted.RowCount != 2 {
		t.Fatalf("grouped=%+v extracted=%+v, want two grouped and extracted records", grouped, extracted)
	}
	if extracted.Fields["non_empty_amount"] != "2" || extracted.Fields["non_empty_item_id"] != "2" {
		t.Fatalf("extracted fields=%+v, want amount and item_id extracted from grouped text", extracted.Fields)
	}
}

func TestActionRunnerExtractFieldsRepairsInvalidRegexEscapesInJSONString(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("id,text\n1,amount: 123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "extract_amount",
			Kind:           DataActionExtractFields,
			InputPaths:     []string{"records.csv"},
			OutputArtifact: "amounts",
			Params: map[string]string{
				"field_specs_json": `[{"source_field":"text","target_field":"amount","operation":"regex_extract","pattern":"amount:\s*(\d+)","group":1}]`,
				"required_fields":  `["amount"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run extract_fields with invalid JSON regex escapes: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].RowCount != 1 {
		t.Fatalf("Artifacts=%+v, want one extracted row", res.Artifacts)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if row["amount"] != "123" {
		t.Fatalf("row=%+v, want repaired regex extraction", row)
	}
}

func TestActionRunnerExtractFieldsRejectsAmbiguousParseNumberText(t *testing.T) {
	root := t.TempDir()
	text := "id: DOC-0006\ncreated: 2026-02-12\nsubtotal: 1391.15\ntotal: USD 24577\n"
	if err := os.WriteFile(filepath.Join(root, "doc.txt"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:         "extract_amount",
			Kind:       DataActionExtractFields,
			InputPaths: []string{"doc.txt"},
			Params: map[string]string{
				"record_scope":     "document",
				"field_specs_json": `[{"source_field":"text","target_field":"amount","operation":"parse_number"}]`,
				"required_fields":  `["amount"]`,
			},
		}},
	})
	if err == nil {
		t.Fatal("Run extract_fields ambiguous parse_number succeeded, want repairable error")
	}
	for _, want := range []string{"extract_fields parse_number", "numeric tokens", "regex_extract"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err=%q, want %q", err, want)
		}
	}
}

func TestActionRunnerExtractFieldsAllowsAnchoredNumericRegex(t *testing.T) {
	root := t.TempDir()
	text := "id: DOC-0006\ncreated: 2026-02-12\nsubtotal: 1391.15\ntotal: USD 24577\n"
	if err := os.WriteFile(filepath.Join(root, "doc.txt"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "extract_amount",
			Kind:           DataActionExtractFields,
			InputPaths:     []string{"doc.txt"},
			OutputArtifact: "amounts",
			Params: map[string]string{
				"record_scope":     "document",
				"field_specs_json": `[{"source_field":"text","target_field":"amount","operation":"regex_extract","pattern":"total:\\s*[A-Z]{3}\\s+([0-9]+)","group":1}]`,
				"required_fields":  `["amount"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run extract_fields anchored regex: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if row["amount"] != "24577" {
		t.Fatalf("row=%+v, want anchored amount", row)
	}
}

func TestActionRunnerExtractFieldsCanUseVirtualFileNameForTextInput(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "text"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "text", "ATT-00006.txt"), []byte("invoice total: CNY 24577\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "extract_invoice",
			Kind:           DataActionExtractFields,
			InputPaths:     []string{"text/ATT-00006.txt"},
			OutputArtifact: "invoice_fields",
			Params: map[string]string{
				"record_scope": "document",
				"field_specs_json": `[
					{"source_field":"file_name","target_field":"attachment_id","operation":"regex_extract","pattern":"ATT-[0-9]+","group":0},
					{"source_field":"text","target_field":"amount","operation":"regex_extract","pattern":"CNY\\s+([0-9]+)","group":1}
				]`,
				"required_fields": `["attachment_id","amount"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run extract_fields with virtual file_name: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].RowCount != 1 {
		t.Fatalf("Artifacts=%+v, want one extracted row", res.Artifacts)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if row["attachment_id"] != "ATT-00006" || row["amount"] != "24577" {
		t.Fatalf("row=%+v, want file-name provenance and amount extraction", row)
	}
}

func TestActionRunnerExtractFieldsZeroMatchDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("id,text\n1,total: 123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		ContinueAfter:  true,
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "extract_missing",
			Kind:           DataActionExtractFields,
			InputPaths:     []string{"records.csv"},
			OutputArtifact: "missing_extract",
			Params: map[string]string{
				"field_specs":     `[{"source_field":"text","target_field":"amount","operation":"regex_extract","pattern":"amount: ([0-9]+)","group":1}]`,
				"required_fields": `["amount"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run zero-match extract_fields: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one artifact", res.Artifacts)
	}
	fields := res.Artifacts[0].Fields
	for _, key := range []string{"zero_match_diagnostics", "source_field_samples", "required_missing_counts", "source_fields"} {
		if strings.TrimSpace(fields[key]) == "" {
			t.Fatalf("fields=%+v, want %s", fields, key)
		}
	}
	if !strings.Contains(fields["source_field_samples"], "total: 123") {
		t.Fatalf("source_field_samples=%q, want source value sample", fields["source_field_samples"])
	}
}

func TestActionRunnerComputeContributionsRejectsUnqualifiedGeneratedStatus(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,label,label_status,label_evidence,amount\n1,A,matched,ref.csv:1,10\n2,B,matched_ambiguous,ref.csv:2,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
		},
		Actions: []DataAction{{
			ID:         "sum_items",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"items.csv"},
			Params: map[string]string{
				"group_key":     "all",
				"metric":        "amount",
				"value_field":   "amount",
				"operation":     "add",
				"item_id_field": "id",
			},
		}},
	}
	_, err := (ActionRunner{
		RepoRoot: root,
		Seed: Result{RuleCoverage: []RuleCoverageRecord{{
			RuleID:   "rule-1",
			RuleText: "generated fields must be resolved before target contribution",
			Status:   "covered",
		}}},
	}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "qualify_records") || !strings.Contains(err.Error(), "label_status=matched_ambiguous") {
		t.Fatalf("Run err=%v, want unresolved generated status repair hint", err)
	}
}

func TestActionRunnerComputeContributionsRejectsOpenStatusAfterIncompleteFilter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,resolution_status,amount\n1,resolved,10\n2,unmatched,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
		},
		Actions: []DataAction{{
			ID:         "sum_items",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"items.csv"},
			Params: map[string]string{
				"group_key":     "all",
				"metric":        "amount",
				"value_field":   "amount",
				"operation":     "add",
				"item_id_field": "id",
				"filters_json":  `[{"field":"resolution_status","op":"not_in","value":["matched_ambiguous","unresolved"]}]`,
			},
		}},
	}
	_, err := (ActionRunner{
		RepoRoot: root,
		Seed: Result{RuleCoverage: []RuleCoverageRecord{{
			RuleID:   "rule-1",
			RuleText: "open generated statuses cannot contribute",
			Status:   "covered",
		}}},
	}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "resolution_status=unmatched") || !strings.Contains(err.Error(), "after_filter") {
		t.Fatalf("Run err=%v, want unmatched status blocked even after incomplete filter", err)
	}
}

func TestActionRunnerFilterRecordsAcceptsBoolLikeAliases(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,eligible,amount\n1,yes,10\n2,no,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "filter",
			Kind:           DataActionFilterRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "eligible_orders",
			Params: map[string]string{
				"filters_json": `[{"field":"eligible","op":"eq","value":true}]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run bool-like filter: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].RowCount != 1 {
		t.Fatalf("Artifacts=%+v, want one yes/true matched row", res.Artifacts)
	}
	diag := res.Artifacts[0].Fields["filter_diagnostics"]
	if !strings.Contains(diag, `"combined_match":1`) || !strings.Contains(diag, "yes") {
		t.Fatalf("filter_diagnostics=%q, want match count and sample value", diag)
	}
}

func TestActionRunnerFilterRecordsAcceptsJSONListFilterValue(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status,amount\n1,paid,10\n2,draft,5\n3,已验收,7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "filter",
			Kind:           DataActionFilterRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "eligible_orders",
			Params: map[string]string{
				"filter_field": "status",
				"filter_op":    "in",
				"filter_value": `["paid","accepted","已验收"]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run JSON-list filter_value: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].RowCount != 2 {
		t.Fatalf("Artifacts=%+v, want paid and accepted rows", res.Artifacts)
	}
	diag := res.Artifacts[0].Fields["filter_diagnostics"]
	if !strings.Contains(diag, `"combined_match":2`) || strings.Contains(diag, `\"paid\"`) {
		t.Fatalf("filter_diagnostics=%q, want typed list value normalized before matching", diag)
	}
}

func TestActionRunnerFilterRecordsRejectsNonNumericFieldForNumericOp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("id,eligible,value\n1,true,10\n2,false,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:         "filter",
			Kind:       DataActionFilterRecords,
			InputPaths: []string{"records.csv"},
			Params: map[string]string{
				"filters_json": `[{"field":"eligible","op":"gt","value":0}]`,
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run err=nil, want numeric field contract failure")
	}
	text := err.Error()
	for _, want := range []string{"numeric field contract", "eligible", "samples=true"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Run err=%q, want substring %q", text, want)
		}
	}
}

func TestActionRunnerFilterRecordsUsesActualJSONRecordFields(t *testing.T) {
	root := t.TempDir()
	payload := `{"headers":["id","predicted_only"],"records":[{"id":"1","status":"paid"}]}`
	if err := os.WriteFile(filepath.Join(root, "wrapped.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "filter",
			Kind:           DataActionFilterRecords,
			InputPaths:     []string{"wrapped.json"},
			OutputArtifact: "filtered",
			Params: map[string]string{
				"filters_json": `[{"field":"predicted_only","op":"eq","value":"x"}]`,
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run err=nil, want predicted-only field rejected against actual records")
	}
	text := err.Error()
	for _, want := range []string{"predicted_only", "fields [id, status]", "filter_diagnostics"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Run err=%q, want substring %q", text, want)
		}
	}
	var fieldErr DataFieldContractError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("err=%T %v, want DataFieldContractError", err, err)
	}
	if fieldErr.ActionKind != DataActionFilterRecords || fieldErr.InputAlias != "wrapped.json" || len(fieldErr.missingFields()) != 1 || fieldErr.missingFields()[0] != "predicted_only" {
		t.Fatalf("fieldErr=%+v, want typed filter field contract", fieldErr)
	}
}

func TestActionRunnerSeedAliasPrefersRecordArtifactOverMetadata(t *testing.T) {
	root := t.TempDir()
	recordPath := filepath.Join(root, "records.json")
	metaPath := filepath.Join(root, "metadata.json")
	if err := os.WriteFile(recordPath, []byte(`[{"id":"1","eligible":"yes","amount":"10"},{"id":"2","eligible":"no","amount":"5"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, []byte(`{"id":"clean","kind":"inspect_material","summary":"metadata only","children":[],"fields":{"count":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	seed := Result{Artifacts: []DataArtifact{
		{
			ID:     "clean",
			Kind:   "derive_fields",
			Fields: map[string]string{"artifact_path": recordPath, "artifact_aliases": "clean"},
		},
		{
			ID:     "clean",
			Kind:   "inspect_material",
			Fields: map[string]string{"artifact_path": metaPath, "artifact_aliases": "clean"},
		},
	}}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "filter",
			Kind:           DataActionFilterRecords,
			InputPaths:     []string{"clean"},
			OutputArtifact: "eligible",
			Params: map[string]string{
				"filters_json": `[{"field":"eligible","op":"eq","value":true}]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run seed alias preference: %v", err)
	}
	var got DataArtifact
	for _, artifact := range res.Artifacts {
		if artifact.ID == "eligible" {
			got = artifact
			break
		}
	}
	if got.RowCount != 1 || got.Fields["input_rows"] != "2" {
		t.Fatalf("Artifacts=%+v, want record artifact selected over metadata alias", res.Artifacts)
	}
}

func TestActionRunnerNormalizeEntitiesRejectsExplicitCanonicalOutsideReferenceUniverse(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "taxonomy.csv"), []byte("code,label\nA,Alpha\nB,Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "normalize",
			Kind:           DataActionNormalizeEntities,
			OutputArtifact: "normalized",
			Params: map[string]string{
				"reference_path":   "taxonomy.csv",
				"reference_field":  "code",
				"resolutions_json": `[{"source_value":"alpha","canonical_id":"ALPHA_ALIAS","status":"resolved"}]`,
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run err=nil, want explicit canonical universe rejection")
	}
	text := err.Error()
	for _, want := range []string{"ALPHA_ALIAS", "outside reference universe", "taxonomy.csv", "allowed_sample=[A, B]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Run err=%q, want substring %q", text, want)
		}
	}
}

func TestActionRunnerNormalizeEntitiesExpandsExplicitSourceValues(t *testing.T) {
	root := t.TempDir()
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "normalize_status",
			Kind:           DataActionNormalizeEntities,
			OutputArtifact: "status_mapping",
			Params: map[string]string{
				"resolutions_json": `[{"source_values":["done","paid","accepted"],"canonical_id":"complete","canonical_label":"Complete","status":"resolved"}]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run normalize_entities source_values expansion: %v", err)
	}
	if len(res.EntityResolutions) != 3 {
		t.Fatalf("EntityResolutions=%+v, want 3 expanded records", res.EntityResolutions)
	}
	got := map[string]string{}
	for _, rec := range res.EntityResolutions {
		got[rec.SourceValue.String()] = rec.CanonicalID.String()
	}
	for _, source := range []string{"done", "paid", "accepted"} {
		if got[source] != "complete" {
			t.Fatalf("expanded source %q maps to %q, want complete; all=%+v", source, got[source], got)
		}
	}
}

func TestActionRunnerNormalizeEntitiesReferenceModeUsesSourceCanonicalID(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_name,amount\nA1,not a reference label,10\nB2,also different,7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.csv"), []byte("id,label\nA1,Alpha\nB2,Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "normalize",
			Kind:           DataActionNormalizeEntities,
			InputPaths:     []string{"items.csv", "reference.csv"},
			OutputArtifact: "mapping",
			Params: map[string]string{
				"source_field":          "raw_name",
				"reference_name_fields": `["label"]`,
				"canonical_id_field":    "id",
				"canonical_label_field": "label",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run normalize_entities source canonical id: %v", err)
	}
	got := map[string]string{}
	for _, rec := range res.EntityResolutions {
		if strings.HasSuffix(rec.ItemID.String(), ":id") {
			got[rec.SourceValue.String()] = rec.CanonicalID.String()
		}
	}
	if got["A1"] != "A1" || got["B2"] != "B2" {
		t.Fatalf("source canonical id resolutions=%+v, want A1/B2 exact matches", res.EntityResolutions)
	}
	if len(res.Artifacts) == 0 {
		t.Fatalf("Artifacts empty, want normalize artifact")
	}
	artifact := res.Artifacts[0]
	if strings.Join(artifact.SourceRecordPaths, ",") != "items.csv" {
		t.Fatalf("SourceRecordPaths=%v, want items.csv", artifact.SourceRecordPaths)
	}
	if strings.Join(artifact.ReferencePaths, ",") != "reference.csv" {
		t.Fatalf("ReferencePaths=%v, want reference.csv", artifact.ReferencePaths)
	}
}

func TestActionRunnerNormalizeEntitiesReferenceModeFeedsEnrich(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_name,amount\n1,Alpha Team,10\n2,beta,7\n3,Gamma Unit,3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.csv"), []byte("code,label,aliases\nA1,Alpha,Alpha Team;Alpha Squad\nB2,Beta,beta;Beta Group\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			EntityResolutionRequired:   true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:             "name_mapping",
				Kind:           DataActionNormalizeEntities,
				InputPaths:     []string{"reference.csv", "items.csv"},
				OutputArtifact: "name_mapping",
				Params: map[string]string{
					"source_field":          "raw_name",
					"canonical_id_field":    "code",
					"canonical_label_field": "label",
					"reference_name_fields": `["aliases","label"]`,
					"match_mode":            "mapping_contains_source",
				},
			},
			{
				ID:             "items_enriched",
				Kind:           DataActionEnrichRecords,
				InputPaths:     []string{"items.csv", "name_mapping"},
				OutputArtifact: "items_enriched",
				Params: map[string]string{
					"base_path": "items.csv",
					"lookup_specs_json": `[{
						"source_fields":["raw_name"],
						"lookup_fields":["source_value"],
						"lookup_value_field":"canonical_id",
						"target_field":"canonical_code",
						"match_mode":"exact"
					}]`,
				},
			},
			{
				ID:         "contrib",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"items_enriched"},
				Params: map[string]string{
					"group_key_field": "canonical_code",
					"metric":          "amount",
					"value_field":     "amount",
					"operation":       "add",
					"item_id_field":   "id",
					"filters_json":    `[{"field":"canonical_code_status","op":"eq","value":"matched"}]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run normalize reference mode actions: %v", err)
	}
	if len(res.EntityResolutions) < 2 {
		t.Fatalf("EntityResolutions=%+v, want mapped source values", res.EntityResolutions)
	}
	mapped := map[string]string{}
	for _, rec := range res.EntityResolutions {
		if rec.SourceValue.String() != "" && rec.CanonicalID.String() != "" {
			mapped[rec.SourceValue.String()] = rec.CanonicalID.String()
		}
	}
	if mapped["Alpha Team"] != "A1" || mapped["beta"] != "B2" {
		t.Fatalf("mapped=%+v, want Alpha Team=>A1 and beta=>B2", mapped)
	}
	if res.Reconcile == nil || len(res.Reconcile.Groups) != 2 {
		t.Fatalf("Reconcile=%+v, want two canonical groups", res.Reconcile)
	}
}

func TestActionRunnerNormalizeEntitiesReferenceModeAppliesRepeatedSourceRows(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_name,amount\n1,Alpha Team,10\n2,Alpha Team,7\n3,beta,3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.csv"), []byte("code,label,aliases\nA1,Alpha,Alpha Team;Alpha Squad\nB2,Beta,beta;Beta Group\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{
			{
				ID:             "name_mapping",
				Kind:           DataActionNormalizeEntities,
				InputPaths:     []string{"items.csv", "reference.csv"},
				OutputArtifact: "name_mapping",
				Params: map[string]string{
					"source_field":          "raw_name",
					"canonical_id_field":    "code",
					"canonical_label_field": "label",
					"reference_name_fields": `["aliases","label"]`,
					"match_mode":            "mapping_contains_source",
				},
			},
			{
				ID:             "apply_names",
				Kind:           DataActionApplyResolutions,
				InputPaths:     []string{"items.csv", "name_mapping"},
				OutputArtifact: "items_with_code",
				Params: map[string]string{
					"base_path":             "items.csv",
					"resolution_path":       "name_mapping",
					"base_key_fields":       `["_source_index"]`,
					"resolution_key_fields": `["item_id"]`,
					"target_id_field":       "canonical_code",
					"target_label_field":    "canonical_label",
					"target_status_field":   "canonical_status",
					"canonical_id_field":    "canonical_id",
					"canonical_label_field": "canonical_label",
					"accepted_statuses":     `["resolved"]`,
					"preserve_base_rows":    "true",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run normalize/apply repeated rows: %v", err)
	}
	var artifact DataArtifact
	for _, candidate := range res.Artifacts {
		if candidate.ID == "items_with_code" {
			artifact = candidate
			break
		}
	}
	if artifact.ID == "" {
		t.Fatalf("Artifacts=%+v, want items_with_code", res.Artifacts)
	}
	if got := artifact.Fields["matched_canonical_code"]; got != "3" {
		t.Fatalf("matched_canonical_code=%q, want 3 row-level matches", got)
	}
	var rows []map[string]any
	for _, sample := range artifact.Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample row: %v", err)
		}
		rows = append(rows, row)
	}
	if len(rows) != 3 {
		t.Fatalf("sample rows=%d, want 3", len(rows))
	}
	for i, row := range rows {
		if row["canonical_status"] != "resolved" {
			t.Fatalf("row %d=%+v, want resolved status", i, row)
		}
	}
	if rows[0]["canonical_code"] != "A1" || rows[1]["canonical_code"] != "A1" || rows[2]["canonical_code"] != "B2" {
		t.Fatalf("rows=%+v, want repeated Alpha rows both mapped to A1 and beta to B2", rows)
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

func TestActionRunnerJoinRecordsAcceptsJSONFieldAliases(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "left.csv"), []byte("id,amount\nA,10\nB,7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "right.csv"), []byte("id,label\nA,alpha\nB,beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "join_aliases",
			Kind:           DataActionJoinRecords,
			InputPaths:     []string{"left.csv", "right.csv"},
			OutputArtifact: "joined",
			Params: map[string]string{
				"left_fields_json":  `["id"]`,
				"right_fields_json": `["id"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run join_records with *_fields_json aliases: %v", err)
	}
	if len(res.Artifacts) == 0 || res.Artifacts[0].RowCount != 2 {
		t.Fatalf("Artifacts=%+v, want two joined rows", res.Artifacts)
	}
}

func TestActionRunnerJoinRecordsValidatesLeftAndRightFieldsSeparately(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "left.csv"), []byte("id,amount\n1,10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "right.csv"), []byte("id,category_code\n1,A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:         "join",
			Kind:       DataActionJoinRecords,
			InputPaths: []string{"left.csv", "right.csv"},
			Params: map[string]string{
				"left_path":    "left.csv",
				"right_path":   "right.csv",
				"left_fields":  `["category_code"]`,
				"right_fields": `["category_code"]`,
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), `join_records left field "category_code" was not found in left input left.csv`) {
		t.Fatalf("err=%v, want side-specific left-field validation", err)
	}
	var fieldErr DataFieldContractError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("err=%T %v, want DataFieldContractError", err, err)
	}
	if fieldErr.Role != "left" || fieldErr.Field != "category_code" || fieldErr.InputAlias != "left.csv" {
		t.Fatalf("fieldErr=%+v, want left/category_code/left.csv", fieldErr)
	}
	violation := ClassifyExecutionFailure(err)
	if violation.Code != "field_contract_violation" ||
		violation.ActionID != "join" ||
		violation.ActionKind != string(DataActionJoinRecords) ||
		violation.InputAlias != "left.csv" ||
		len(violation.MissingFields) != 1 ||
		violation.MissingFields[0] != "category_code" {
		t.Fatalf("violation=%+v, want typed join field contract", violation)
	}
}

func TestActionRunnerJoinRecordsInfersSidesFromFieldContracts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "left.csv"), []byte("vendor_id,category_hint,amount\nV1,C1,10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "right.csv"), []byte("vendor_id,category_code,query_id\nV1,C1,Q1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "noise.csv"), []byte("id,label\n1,x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "join",
			Kind:           DataActionJoinRecords,
			InputPaths:     []string{"noise.csv", "right.csv", "left.csv"},
			OutputArtifact: "joined",
			Params: map[string]string{
				"left_fields":  `["vendor_id","category_hint"]`,
				"right_fields": `["vendor_id","category_code"]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run join with inferred sides: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].RowCount != 1 {
		t.Fatalf("Artifacts=%+v, want one joined row", res.Artifacts)
	}
	if got := res.Artifacts[0].Fields["left_path"]; got != "left.csv" {
		t.Fatalf("left_path=%q, want left.csv", got)
	}
	if got := res.Artifacts[0].Fields["right_path"]; got != "right.csv" {
		t.Fatalf("right_path=%q, want right.csv", got)
	}
}

func TestActionRunnerFilterRecordsRepairsConcatenatedFilterArrayShape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,year,status,amount\n1,2026,paid,10\n2,2025,paid,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "filter",
			Kind:           DataActionFilterRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "eligible",
			Params: map[string]string{
				"filters_json": `[{"field":"year","op":"eq","value":2026}],{"field":"status","op":"eq","value":"paid"}]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run filter with repaired filters_json: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].RowCount != 1 {
		t.Fatalf("Artifacts=%+v, want one filtered row", res.Artifacts)
	}
}

func TestActionRunnerJoinRecordsKeepsPredictedHeadersForZeroMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "left.csv"), []byte("id,amount,status\n1,10,paid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "right.csv"), []byte("id,status,label\n2,active,x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "join",
			Kind:           DataActionJoinRecords,
			InputPaths:     []string{"left.csv", "right.csv"},
			OutputArtifact: "joined",
			Params: map[string]string{
				"left_fields":  `["id"]`,
				"right_fields": `["id"]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run join: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].RowCount != 0 {
		t.Fatalf("Artifacts=%+v, want zero-row join artifact", res.Artifacts)
	}
	headers := strings.Join(res.Artifacts[0].Headers, ",")
	for _, want := range []string{"amount", "id", "right_status", "label", "_left_source", "_right_source"} {
		if !strings.Contains(headers, want) {
			t.Fatalf("headers=%q, missing %q", headers, want)
		}
	}
	if !strings.Contains(res.Artifacts[0].Fields["output_headers"], "right_status") {
		t.Fatalf("Fields=%+v, want output_headers with collision field contract", res.Artifacts[0].Fields)
	}
}

func TestActionRunnerJoinRecordsZeroMatchRemainsRecordSetForNextAction(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "left.csv"), []byte("id,amount,status\n1,10,paid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "right.csv"), []byte("id,status,label\n2,active,x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{
			{
				ID:             "join",
				Kind:           DataActionJoinRecords,
				InputPaths:     []string{"left.csv", "right.csv"},
				OutputArtifact: "joined",
				Params: map[string]string{
					"left_fields":  `["id"]`,
					"right_fields": `["id"]`,
				},
			},
			{
				ID:             "filter",
				Kind:           DataActionFilterRecords,
				InputPaths:     []string{"joined"},
				OutputArtifact: "filtered",
				Params: map[string]string{
					"filters_json": `[{"field":"amount","op":"gt","value":"0"}]`,
				},
			},
		},
	}
	artifactRoot := filepath.Join(root, ".actions")
	res, err := (ActionRunner{RepoRoot: root, TempRoot: artifactRoot}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run join->filter: %v", err)
	}
	if len(res.Artifacts) != 2 {
		t.Fatalf("Artifacts=%+v, want join and filter artifacts", res.Artifacts)
	}
	if res.Artifacts[0].RowCount != 0 || res.Artifacts[1].RowCount != 0 {
		t.Fatalf("Artifacts=%+v, want zero-row join and zero-row filter", res.Artifacts)
	}
	raw, err := os.ReadFile(res.Artifacts[0].Fields["artifact_path"])
	if err != nil {
		t.Fatalf("read materialized join artifact: %v", err)
	}
	if strings.TrimSpace(string(raw)) == "null" {
		t.Fatalf("join artifact payload is null; want schema-bearing empty record set")
	}
	if !strings.Contains(string(raw), `"headers"`) || !strings.Contains(string(raw), `"records"`) {
		t.Fatalf("join artifact payload=%s, want records wrapper with headers", raw)
	}
}

func TestActionRunnerLeftJoinPreservesRightSchemaForUnmatchedRows(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "left.csv"), []byte("id,amount,status\n1,10,paid\n2,20,paid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "right.csv"), []byte("id,status,label\n3,active,x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "join",
			Kind:           DataActionJoinRecords,
			InputPaths:     []string{"left.csv", "right.csv"},
			OutputArtifact: "joined",
			Params: map[string]string{
				"left_fields":  `["id"]`,
				"right_fields": `["id"]`,
				"join_type":    "left",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run left join: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].RowCount != 2 {
		t.Fatalf("Artifacts=%+v, want two left rows preserved", res.Artifacts)
	}
	headers := strings.Join(res.Artifacts[0].Headers, ",")
	for _, want := range []string{"amount", "right_status", "label", "_right_source"} {
		if !strings.Contains(headers, want) {
			t.Fatalf("headers=%q, missing %q", headers, want)
		}
	}
	if got := res.Artifacts[0].Fields["zero_matches"]; got != "true" {
		t.Fatalf("Fields=%+v, want zero_matches=true", res.Artifacts[0].Fields)
	}
	if got := res.Artifacts[0].Fields["unmatched_left_rows"]; got != "2" {
		t.Fatalf("Fields=%+v, want unmatched_left_rows=2", res.Artifacts[0].Fields)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if _, ok := row["right_status"]; !ok {
		t.Fatalf("row=%+v, want blank right_status field present", row)
	}
	if _, ok := row["label"]; !ok {
		t.Fatalf("row=%+v, want blank label field present", row)
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

func TestActionRunnerApplyEntityResolutionsMaterializesCanonicalFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_name,amount\n1,Alpha Team,10\n2,beta,7\n3,Gamma Unit,3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.csv"), []byte("code,label,aliases\nA1,Alpha,Alpha Team;Alpha Squad\nB2,Beta,beta;Beta Group\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			EntityResolutionRequired:   true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{
				ID:             "name_mapping",
				Kind:           DataActionNormalizeEntities,
				InputPaths:     []string{"items.csv", "reference.csv"},
				OutputArtifact: "name_mapping",
				Params: map[string]string{
					"source_path":           "items.csv",
					"reference_path":        "reference.csv",
					"source_field":          "raw_name",
					"canonical_id_field":    "code",
					"canonical_label_field": "label",
					"reference_name_fields": `["aliases","label"]`,
					"match_mode":            "mapping_contains_source",
				},
			},
			{
				ID:             "items_resolved",
				Kind:           DataActionApplyResolutions,
				InputPaths:     []string{"items.csv", "name_mapping"},
				OutputArtifact: "items_resolved",
				Params: map[string]string{
					"base_path": "items.csv",
					"resolution_specs_json": `[{
						"resolution_path":"name_mapping",
						"target_id_field":"canonical_code",
						"target_label_field":"canonical_label",
						"target_status_field":"canonical_status"
					}]`,
				},
			},
			{
				ID:         "contrib",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"items_resolved"},
				Params: map[string]string{
					"group_key_field": "canonical_code",
					"metric":          "amount",
					"value_field":     "amount",
					"operation":       "add",
					"item_id_field":   "id",
					"filters_json":    `[{"field":"canonical_status","op":"eq","value":"resolved"}]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions actions: %v", err)
	}
	if res.Answer != "A1/amount=10; B2/amount=7" {
		t.Fatalf("Answer=%q", res.Answer)
	}
	if len(res.Artifacts) < 3 || res.Artifacts[1].Kind != string(DataActionApplyResolutions) {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	if got := res.Artifacts[1].Fields["matched_canonical_code"]; got != "2" {
		t.Fatalf("matched_canonical_code=%q, want 2", got)
	}
}

func TestActionRunnerApplyEntityResolutionsIgnoresOpenChoicesByDefault(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("orders.json", []map[string]any{{"id": "A1", "raw_name": "ambiguous", "amount": "10"}})
	writeJSON("mapping.json", []map[string]any{
		{"item_id": "orders.json#1:raw_name", "source_value": "ambiguous", "canonical_id": "B2", "status": "matched_ambiguous"},
		{"item_id": "orders.json#1:id", "source_value": "A1", "canonical_id": "A1", "status": "resolved"},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "apply",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"orders.json", "mapping.json"},
			OutputArtifact: "orders_resolved",
			Params: map[string]string{
				"resolution_specs_json": `[{
					"resolution_path":"mapping.json",
					"resolution_key_fields":["item_id"],
					"target_id_field":"canonical_id",
					"target_status_field":"resolution_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions open choice filtering: %v", err)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].Fields["matched_canonical_id"] != "1" || res.Artifacts[0].Fields["ambiguous_canonical_id"] != "0" {
		t.Fatalf("artifact=%+v, want one resolved match and no ambiguity", res.Artifacts)
	}
	var sample map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &sample); err != nil {
		t.Fatal(err)
	}
	if sample["canonical_id"] != "A1" || sample["resolution_status"] != "resolved" {
		t.Fatalf("sample=%+v, want resolved A1", sample)
	}
}

func TestActionRunnerApplyEntityResolutionsHonorsSpecBasePathAndLocatorKey(t *testing.T) {
	root := t.TempDir()
	orders := []map[string]any{
		{"id": "o1", "raw_category": "compute", "amount": "10"},
		{"id": "o2", "raw_category": "storage", "amount": "7"},
	}
	resolutions := []map[string]any{
		{"item_id": "orders.json#1:raw_category", "canonical_id": "C1", "canonical_label": "Compute", "status": "resolved"},
		{"item_id": "orders.json#2:raw_category", "canonical_id": "S1", "canonical_label": "Storage", "status": "resolved"},
	}
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("orders.json", orders)
	writeJSON("category_resolution.json", resolutions)

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "orders_with_category",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"category_resolution.json", "orders.json"},
			OutputArtifact: "orders_with_category",
			Params: map[string]string{
				"resolution_specs_json": `[{
					"base_path":"orders.json",
					"resolution_path":"category_resolution.json",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"category_id",
					"target_label_field":"category_label",
					"target_status_field":"category_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with spec base path: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if got := artifact.Fields["matched_category_id"]; got != "2" {
		t.Fatalf("matched_category_id=%q, want 2; artifact=%+v", got, artifact)
	}
	var found bool
	for _, sample := range artifact.Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("sample %q: %v", sample, err)
		}
		if row["id"] == "o1" && row["category_id"] == "C1" && row["category_status"] == "resolved" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Sample=%v, want resolved order row", artifact.Sample)
	}
}

func TestActionRunnerApplyEntityResolutionsScopesLocatorKeyBySourceFields(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("orders.json", []map[string]any{
		{"id": "o1", "raw_vendor": "Acme", "raw_category": "compute"},
		{"id": "o2", "raw_vendor": "Beta", "raw_category": "storage"},
	})
	writeJSON("resolutions.json", []map[string]any{
		{"item_id": "orders.json#1:raw_vendor", "source_value": "Acme", "canonical_id": "V1", "canonical_label": "Acme Ltd", "status": "resolved"},
		{"item_id": "orders.json#1:raw_category", "source_value": "compute", "canonical_id": "C1", "canonical_label": "Compute", "status": "resolved"},
		{"item_id": "orders.json#2:raw_vendor", "source_value": "Beta", "canonical_id": "V2", "canonical_label": "Beta Ltd", "status": "resolved"},
		{"item_id": "orders.json#2:raw_category", "source_value": "storage", "canonical_id": "C2", "canonical_label": "Storage", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "apply",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"orders.json", "resolutions.json"},
			OutputArtifact: "orders_resolved",
			Params: map[string]string{
				"base_path": "orders.json",
				"resolution_specs_json": `[{
					"resolution_path":"resolutions.json",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"source_fields":["raw_vendor"],
					"target_id_field":"canonical_vendor_id",
					"target_status_field":"vendor_status"
				},{
					"resolution_path":"resolutions.json",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"source_fields":["raw_category"],
					"target_id_field":"canonical_category_code",
					"target_status_field":"category_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with source field scoping: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one output artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	for field, want := range map[string]string{
		"matched_canonical_vendor_id":       "2",
		"ambiguous_canonical_vendor_id":     "0",
		"matched_canonical_category_code":   "2",
		"ambiguous_canonical_category_code": "0",
	} {
		if got := artifact.Fields[field]; got != want {
			t.Fatalf("%s=%q, want %q; artifact=%+v", field, got, want, artifact)
		}
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(artifact.Sample[0]), &first); err != nil {
		t.Fatalf("sample %q: %v", artifact.Sample[0], err)
	}
	if first["canonical_vendor_id"] != "V1" || first["canonical_category_code"] != "C1" {
		t.Fatalf("first sample=%+v, want scoped vendor/category canonical fields", first)
	}
}

func TestActionRunnerApplyEntityResolutionsNormalizesBaseLocatorKey(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("records.json", []map[string]any{
		{"id": "r1", "_source_locator": "source.csv#1", "raw_name": "alpha"},
		{"id": "r2", "_source_locator": "source.csv#2", "raw_name": "beta"},
	})
	writeJSON("resolution.json", []map[string]any{
		{"item_id": "records.json#1:raw_name", "canonical_id": "A", "canonical_label": "Alpha", "status": "resolved"},
		{"item_id": "records.json#2:raw_name", "canonical_id": "B", "canonical_label": "Beta", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "records_with_resolution",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"records.json", "resolution.json"},
			OutputArtifact: "records_with_resolution",
			Params: map[string]string{
				"resolution_specs_json": `[{
					"base_path":"records.json",
					"resolution_path":"resolution.json",
					"base_key_fields":["_source_locator"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"resolved_id",
					"target_label_field":"resolved_label",
					"target_status_field":"resolution_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with base locator key: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if got := artifact.Fields["matched_resolved_id"]; got != "2" {
		t.Fatalf("matched_resolved_id=%q, want 2; artifact=%+v", got, artifact)
	}
}

func TestActionRunnerApplyEntityResolutionsKeepsDeclaredTargetFieldsWhenUnmatched(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("records.json", []map[string]any{
		{"id": "r1", "raw_value": "alpha"},
		{"id": "r2", "raw_value": "beta"},
	})
	writeJSON("resolution.json", []map[string]any{
		{"item_id": "records.json#99:raw_value", "canonical_id": "Z9", "canonical_label": "Zed", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "records_with_resolution",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"records.json", "resolution.json"},
			OutputArtifact: "records_with_resolution",
			Params: map[string]string{
				"resolution_specs_json": `[{
					"base_path":"records.json",
					"resolution_path":"resolution.json",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"resolved_id",
					"target_label_field":"resolved_label",
					"target_status_field":"resolution_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with unmatched records: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one output artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	for _, want := range []string{"resolved_id", "resolved_label", "resolution_status"} {
		if !stringSliceContainsFold(artifact.Headers, want) {
			t.Fatalf("Headers=%v, missing declared target field %q", artifact.Headers, want)
		}
	}
	if got := artifact.Fields["unmatched_resolved_id"]; got != "2" {
		t.Fatalf("unmatched_resolved_id=%q, want 2; artifact=%+v", got, artifact)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(artifact.Sample[0]), &row); err != nil {
		t.Fatalf("sample %q: %v", artifact.Sample[0], err)
	}
	if _, ok := row["resolved_id"]; !ok {
		t.Fatalf("row=%+v, missing empty resolved_id key", row)
	}
	if _, ok := row["resolved_label"]; !ok {
		t.Fatalf("row=%+v, missing empty resolved_label key", row)
	}
	if row["resolved_id"] != "" || row["resolved_label"] != "" || row["resolution_status"] != "unmatched" {
		t.Fatalf("row=%+v, want empty declared target fields with unmatched status", row)
	}
}

func TestActionRunnerApplyEntityResolutionsVerifiesExistingCanonicalID(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("records.json", []map[string]any{
		{"id": "r1", "raw_value": "not enough text", "existing_code": "A1"},
		{"id": "r2", "raw_value": "missing reference", "existing_code": "Z9"},
		{"id": "r3", "raw_value": "inactive reference", "existing_code": "D4"},
	})
	writeJSON("reference.json", []map[string]any{
		{"code": "A1", "label": "Alpha One", "state": "active"},
		{"code": "D4", "label": "Delta Four", "state": "inactive"},
	})
	writeJSON("resolution.json", []map[string]any{
		{"item_id": "records.json#1:raw_value", "source_value": "not enough text", "canonical_id": "", "canonical_label": "", "status": "unmatched"},
		{"item_id": "records.json#2:raw_value", "source_value": "missing reference", "canonical_id": "", "canonical_label": "", "status": "unmatched"},
		{"item_id": "records.json#3:raw_value", "source_value": "inactive reference", "canonical_id": "", "canonical_label": "", "status": "unmatched"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "records_resolved",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"records.json", "resolution.json", "reference.json"},
			OutputArtifact: "records_resolved",
			Params: map[string]string{
				"base_path": "records.json",
				"resolution_specs_json": `[{
					"resolution_path":"resolution.json",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"source_fields":["raw_value"],
					"target_id_field":"canonical_code",
					"target_label_field":"canonical_label",
					"target_status_field":"canonical_status",
					"existing_id_field":"existing_code",
					"reference_path":"reference.json",
					"reference_id_field":"code",
					"reference_label_field":"label",
					"reference_status_field":"state",
					"reference_accepted_statuses":["active"]
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with existing canonical ID verification: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one output artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.Fields["matched_canonical_code"] != "1" ||
		artifact.Fields["verified_existing_canonical_code"] != "1" ||
		artifact.Fields["unmatched_canonical_code"] != "2" {
		t.Fatalf("artifact fields=%+v, want one verified existing match and two unmatched rows", artifact.Fields)
	}
	var rows []map[string]any
	for _, sample := range artifact.Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["canonical_code"] != "A1" || rows[0]["canonical_label"] != "Alpha One" || rows[0]["canonical_status"] != "resolved" {
		t.Fatalf("row0=%+v, want verified existing canonical value", rows[0])
	}
	if rows[1]["canonical_code"] != "" || rows[1]["canonical_status"] != "unmatched" {
		t.Fatalf("row1=%+v, want missing reference to remain unmatched", rows[1])
	}
	if rows[2]["canonical_code"] != "" || rows[2]["canonical_status"] != "unmatched" {
		t.Fatalf("row2=%+v, want inactive reference to remain unmatched", rows[2])
	}
}

func TestActionRunnerApplyEntityResolutionsInfersBaseFieldForSourceValueKey(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("records.json", []map[string]any{
		{"id": "r1", "raw_label": "alpha", "amount": "10"},
		{"id": "r2", "raw_label": "beta", "amount": "7"},
	})
	writeJSON("resolution.json", []map[string]any{
		{"item_id": "records.json#1:raw_label", "source_value": "alpha", "canonical_id": "A", "canonical_label": "Alpha", "status": "resolved"},
		{"item_id": "records.json#2:raw_label", "source_value": "beta", "canonical_id": "B", "canonical_label": "Beta", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "records_resolved",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"records.json", "resolution.json"},
			OutputArtifact: "records_resolved",
			Params: map[string]string{
				"base_path":             "records.json",
				"resolution_path":       "resolution.json",
				"resolution_key_fields": `["source_value"]`,
				"target_id_field":       "canonical_code",
				"target_label_field":    "canonical_label",
				"target_status_field":   "canonical_status",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with source_value key inference: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one output artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if got := artifact.Fields["matched_canonical_code"]; got != "2" {
		t.Fatalf("matched_canonical_code=%q, want 2; artifact=%+v", got, artifact)
	}
	if got := artifact.Fields["source_value_base_fields_canonical_code"]; got != "raw_label" {
		t.Fatalf("source_value_base_fields_canonical_code=%q, want raw_label", got)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(artifact.Sample[0]), &first); err != nil {
		t.Fatalf("sample %q: %v", artifact.Sample[0], err)
	}
	if first["canonical_code"] != "A" || first["canonical_status"] != "resolved" {
		t.Fatalf("first sample=%+v, want resolved canonical A", first)
	}
}

func TestActionRunnerApplyEntityResolutionsFallsBackToSourceValueWhenExplicitKeyMisses(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("records.json", []map[string]any{
		{"row_id": "base-1", "raw_label": "alpha"},
	})
	writeJSON("resolution.json", []map[string]any{
		{"item_id": "resolution-row-1", "source_value": "alpha", "canonical_id": "A", "canonical_label": "Alpha", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "records_resolved",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"records.json", "resolution.json"},
			OutputArtifact: "records_resolved",
			Params: map[string]string{
				"base_path":             "records.json",
				"resolution_path":       "resolution.json",
				"base_key_fields":       `["row_id"]`,
				"resolution_key_fields": `["item_id"]`,
				"source_fields":         `["raw_label"]`,
				"target_id_field":       "canonical_code",
				"target_label_field":    "canonical_label",
				"target_status_field":   "canonical_status",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with explicit key fallback: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one output artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if got := artifact.Fields["matched_canonical_code"]; got != "1" {
		t.Fatalf("matched_canonical_code=%q, want source_value fallback match; artifact=%+v", got, artifact)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(artifact.Sample[0]), &first); err != nil {
		t.Fatalf("sample %q: %v", artifact.Sample[0], err)
	}
	if first["canonical_code"] != "A" || first["canonical_status"] != "resolved" {
		t.Fatalf("first sample=%+v, want resolved canonical A via source_value fallback", first)
	}
}

func TestActionRunnerApplyEntityResolutionsPreservesBaseRowsWhenBaseFiltered(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("records.json", []map[string]any{
		{"id": "r1", "raw_name": "alpha", "clean_id": ""},
		{"id": "r2", "raw_name": "beta", "clean_id": "B"},
		{"id": "r3", "raw_name": "gamma", "clean_id": ""},
	})
	writeJSON("resolution.json", []map[string]any{
		{"item_id": "records.json#1:raw_name", "canonical_id": "A", "canonical_label": "Alpha", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "records_with_resolution",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"records.json", "resolution.json"},
			OutputArtifact: "records_with_resolution",
			Params: map[string]string{
				"base_path":             "records.json",
				"resolution_path":       "resolution.json",
				"base_key_fields":       `["_source_index"]`,
				"resolution_key_fields": `["item_id"]`,
				"source_filters":        `[{"field":"clean_id","op":"empty"}]`,
				"target_id_field":       "canonical_id",
				"target_label_field":    "canonical_label",
				"target_status_field":   "canonical_status",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions preserving filtered base rows: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one output artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.RowCount != 3 || artifact.Fields["output_rows"] != "3" {
		t.Fatalf("artifact=%+v, want all 3 base rows preserved", artifact)
	}
	if artifact.Fields["base_filter_mode"] != "preserve_base_rows" {
		t.Fatalf("base_filter_mode=%q, want preserve_base_rows", artifact.Fields["base_filter_mode"])
	}
	if artifact.Fields["matched_canonical_id"] != "1" ||
		artifact.Fields["unmatched_canonical_id"] != "1" ||
		artifact.Fields["not_applicable_canonical_id"] != "1" {
		t.Fatalf("fields=%+v, want matched/unmatched/not_applicable counts 1/1/1", artifact.Fields)
	}
	var rows []map[string]any
	for _, sample := range artifact.Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if len(rows) != 3 {
		t.Fatalf("sample rows=%v, want 3", rows)
	}
	if rows[0]["canonical_id"] != "A" || rows[0]["canonical_status"] != "resolved" {
		t.Fatalf("row0=%+v, want resolved mapping", rows[0])
	}
	if rows[1]["canonical_status"] != "not_applicable" {
		t.Fatalf("row1=%+v, want base-filter skipped row to remain visible", rows[1])
	}
	if rows[2]["canonical_status"] != "unmatched" {
		t.Fatalf("row2=%+v, want eligible but unmatched row", rows[2])
	}
}

func TestActionRunnerApplyEntityResolutionsCanFilterOutputRowsExplicitly(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("records.json", []map[string]any{
		{"id": "r1", "raw_name": "alpha", "clean_id": ""},
		{"id": "r2", "raw_name": "beta", "clean_id": "B"},
		{"id": "r3", "raw_name": "gamma", "clean_id": ""},
	})
	writeJSON("resolution.json", []map[string]any{
		{"item_id": "records.json#1:raw_name", "canonical_id": "A", "canonical_label": "Alpha", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "records_with_resolution",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"records.json", "resolution.json"},
			OutputArtifact: "records_with_resolution",
			Params: map[string]string{
				"base_path":             "records.json",
				"resolution_path":       "resolution.json",
				"base_key_fields":       `["_source_index"]`,
				"resolution_key_fields": `["item_id"]`,
				"source_filters":        `[{"field":"clean_id","op":"empty"}]`,
				"base_filter_mode":      "filter_output",
				"target_id_field":       "canonical_id",
				"target_label_field":    "canonical_label",
				"target_status_field":   "canonical_status",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions filtering output rows: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one output artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.RowCount != 2 || artifact.Fields["output_rows"] != "2" {
		t.Fatalf("artifact=%+v, want filtered output rows", artifact)
	}
	if artifact.Fields["base_filter_mode"] != "filter_output_rows" {
		t.Fatalf("base_filter_mode=%q, want filter_output_rows", artifact.Fields["base_filter_mode"])
	}
}

func TestActionRunnerMaterializesSeedEntityResolutionsAsWorkflowHandle(t *testing.T) {
	root := t.TempDir()
	orders := []map[string]any{
		{"id": "o1", "raw_category": "compute", "amount": "10"},
		{"id": "o2", "raw_category": "storage", "amount": "7"},
	}
	raw, err := json.Marshal(orders)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "orders.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{
		RepoRoot: root,
		Seed: Result{
			EntityResolutions: []EntityResolutionRecord{
				{ItemID: "orders.json#1:raw_category", CanonicalID: "C1", CanonicalLabel: "Compute", Status: "resolved"},
				{ItemID: "orders.json#2:raw_category", CanonicalID: "S1", CanonicalLabel: "Storage", Status: "resolved"},
			},
		},
	}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "orders_with_category",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"orders.json", "workflow_entity_resolutions"},
			OutputArtifact: "orders_with_category",
			Params: map[string]string{
				"base_path": "orders.json",
				"resolution_specs_json": `[{
						"resolution_path":"workflow_entity_resolutions",
						"base_key_fields":["_source_index"],
						"resolution_key_fields":["source_locator"],
						"target_id_field":"category_id",
						"target_label_field":"category_label",
						"target_status_field":"category_status"
					}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with workflow handle: %v", err)
	}
	var hasHandle bool
	for _, artifact := range res.Artifacts {
		if artifact.ID == "workflow_entity_resolutions" && artifact.Fields["workflow_ledger"] == "true" {
			hasHandle = true
		}
	}
	if !hasHandle {
		t.Fatalf("Artifacts=%+v, want workflow_entity_resolutions handle", res.Artifacts)
	}
	got := res.Artifacts[len(res.Artifacts)-1]
	if got.Fields["matched_category_id"] != "2" {
		t.Fatalf("matched_category_id=%q, want 2; artifact=%+v", got.Fields["matched_category_id"], got)
	}
}

func TestActionRunnerApplyEntityResolutionsPreservesInputPathOrderForBase(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("z_orders.json", []map[string]any{
		{"id": "o1", "raw_category": "compute", "amount": "10"},
		{"id": "o2", "raw_category": "storage", "amount": "7"},
	})
	writeJSON("a_resolution.json", []map[string]any{
		{"item_id": "z_orders.json#1:raw_category", "canonical_id": "C1", "canonical_label": "Compute", "status": "resolved"},
		{"item_id": "z_orders.json#2:raw_category", "canonical_id": "S1", "canonical_label": "Storage", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "orders_with_category",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"z_orders.json", "a_resolution.json"},
			OutputArtifact: "orders_with_category",
			Params: map[string]string{
				"resolution_specs_json": `[{
					"resolution_path":"a_resolution.json",
					"target_id_field":"category_id",
					"target_label_field":"category_label",
					"target_status_field":"category_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with ordered inputs: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.RowCount != 2 || artifact.Fields["base_path"] != "z_orders.json" {
		t.Fatalf("artifact=%+v, want z_orders.json base with 2 rows", artifact)
	}
	if artifact.Fields["matched_category_id"] != "2" {
		t.Fatalf("matched_category_id=%q, want 2; artifact=%+v", artifact.Fields["matched_category_id"], artifact)
	}
	var found bool
	for _, sample := range artifact.Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("sample %q: %v", sample, err)
		}
		if row["id"] == "o1" && row["amount"] == "10" && row["category_id"] == "C1" {
			found = true
		}
		if _, hasResolutionOnlyField := row["source_value"]; hasResolutionOnlyField {
			t.Fatalf("sample row=%v, base was resolution ledger instead of source records", row)
		}
	}
	if !found {
		t.Fatalf("Sample=%v, want source order row with applied category", artifact.Sample)
	}
}

func TestActionRunnerApplyEntityResolutionsTreatsCanonicalRecordAsBase(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("orders_with_vendor.json", []map[string]any{
		{"_source_index": 1, "_source_locator": "orders.csv#1", "po_id": "o1", "raw_category": "compute", "vendor_id_canonical": "V1"},
		{"_source_index": 2, "_source_locator": "orders.csv#2", "po_id": "o2", "raw_category": "storage", "vendor_id_canonical": "V2"},
	})
	writeJSON("category_resolution.json", []map[string]any{
		{"item_id": "orders_with_vendor.json#1:raw_category", "source_value": "compute", "canonical_id": "C1", "canonical_label": "Compute", "status": "resolved"},
		{"item_id": "orders_with_vendor.json#2:raw_category", "source_value": "storage", "canonical_id": "S1", "canonical_label": "Storage", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "orders_with_category",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"orders_with_vendor.json", "category_resolution.json"},
			OutputArtifact: "orders_with_category",
			Params: map[string]string{
				"base_path":       "orders_with_vendor.json",
				"resolution_path": "category_resolution.json",
				"resolution_specs_json": `[{
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"category_id_canonical",
					"target_label_field":"category_label_canonical",
					"target_status_field":"category_resolution_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with canonical base record: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.Fields["base_path"] != "orders_with_vendor.json" || artifact.Fields["matched_category_id_canonical"] != "2" {
		t.Fatalf("artifact fields=%+v, want canonical record treated as base and category applied", artifact.Fields)
	}
	var found bool
	for _, sample := range artifact.Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("sample %q: %v", sample, err)
		}
		if row["po_id"] == "o1" && row["vendor_id_canonical"] == "V1" && row["category_id_canonical"] == "C1" {
			found = true
		}
		if _, ok := row["source_value"]; ok {
			t.Fatalf("sample row=%v, resolution fields leaked as base row", row)
		}
	}
	if !found {
		t.Fatalf("Sample=%v, want canonical base row with applied second resolution", artifact.Sample)
	}
}

func TestActionRunnerApplyEntityResolutionsInfersBaseWhenResolutionListedFirst(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("orders.json", []map[string]any{
		{"po_id": "P1", "raw_category": "compute", "amount": "10"},
		{"po_id": "P2", "raw_category": "storage", "amount": "7"},
	})
	writeJSON("category_resolution.json", []map[string]any{
		{"item_id": "orders.json#1:raw_category", "source_value": "compute", "canonical_id": "C1", "canonical_label": "Compute", "status": "resolved"},
		{"item_id": "orders.json#2:raw_category", "source_value": "storage", "canonical_id": "S1", "canonical_label": "Storage", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "orders_with_category",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"category_resolution.json", "orders.json"},
			OutputArtifact: "orders_with_category",
			Params: map[string]string{
				"resolution_specs_json": `[{
					"resolution_path":"category_resolution.json",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"category_id",
					"target_label_field":"category_label",
					"target_status_field":"category_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with inferred base role: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.Fields["base_path"] != "orders.json" {
		t.Fatalf("base_path=%q, want orders.json; fields=%+v", artifact.Fields["base_path"], artifact.Fields)
	}
	if !strings.Contains(artifact.Fields["role_inference"], "base_path inferred") {
		t.Fatalf("role_inference=%q, want audit note", artifact.Fields["role_inference"])
	}
	if artifact.Fields["matched_category_id"] != "2" {
		t.Fatalf("matched_category_id=%q, want 2; artifact=%+v", artifact.Fields["matched_category_id"], artifact)
	}
	var found bool
	for _, sample := range artifact.Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("sample %q: %v", sample, err)
		}
		if row["po_id"] == "P1" && row["category_id"] == "C1" && row["category_status"] == "resolved" {
			found = true
		}
		if _, ok := row["source_value"]; ok {
			t.Fatalf("sample row=%v, resolution fields leaked as base row", row)
		}
	}
	if !found {
		t.Fatalf("Sample=%v, want base rows with applied resolution fields", artifact.Sample)
	}
}

func TestActionRunnerApplyEntityResolutionsInheritsTopLevelResolutionPathForSpecs(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("orders.json", []map[string]any{
		{"id": "o1", "raw_name": "alpha", "amount": "10"},
		{"id": "o2", "raw_name": "beta", "amount": "7"},
	})
	writeJSON("workflow_entity_resolutions.json", []map[string]any{
		{"item_id": "orders.json#1:raw_name", "canonical_id": "A", "canonical_label": "Alpha", "status": "resolved"},
		{"item_id": "orders.json#2:raw_name", "canonical_id": "B", "canonical_label": "Beta", "status": "resolved"},
	})

	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "orders_with_canonical_fields",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"orders.json", "workflow_entity_resolutions.json"},
			OutputArtifact: "orders_with_canonical_fields",
			Params: map[string]string{
				"base_path":       "orders.json",
				"resolution_path": "workflow_entity_resolutions.json",
				"resolution_specs_json": `[{
					"target_id_field":"canonical_vendor_id",
					"target_label_field":"canonical_vendor_label",
					"target_status_field":"canonical_vendor_status"
				},{
					"target_id_field":"canonical_category_id",
					"target_label_field":"canonical_category_label",
					"target_status_field":"canonical_category_status"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with inherited top-level resolution path: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.Fields["matched_canonical_vendor_id"] != "2" || artifact.Fields["matched_canonical_category_id"] != "2" {
		t.Fatalf("artifact fields=%+v, want both specs matched through inherited resolution_path", artifact.Fields)
	}
}

func TestActionRunnerApplyEntityResolutionsRoutesGenericFiltersToResolutionSide(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("orders.json", []map[string]any{
		{"id": "o1", "raw_vendor": "Vendor A", "raw_category": "Compute", "amount": "10"},
		{"id": "o2", "raw_vendor": "Vendor B", "raw_category": "Storage", "amount": "7"},
	})

	res, err := (ActionRunner{
		RepoRoot: root,
		Seed: Result{
			EntityResolutions: []EntityResolutionRecord{
				{ItemID: "orders.json#1:raw_vendor", SourceValue: "Vendor A", CanonicalID: "V1", CanonicalLabel: "Vendor A", Status: "resolved"},
				{ItemID: "orders.json#2:raw_vendor", SourceValue: "Vendor B", CanonicalID: "V2", CanonicalLabel: "Vendor B", Status: "resolved"},
				{ItemID: "orders.json#1:raw_category", SourceValue: "Compute", CanonicalID: "C1", CanonicalLabel: "Compute", Status: "resolved"},
				{ItemID: "orders.json#2:raw_category", SourceValue: "Storage", CanonicalID: "C2", CanonicalLabel: "Storage", Status: "resolved"},
			},
		},
	}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{
			{
				ID:             "apply_vendor",
				Kind:           DataActionApplyResolutions,
				InputPaths:     []string{"orders.json", "workflow_entity_resolutions"},
				OutputArtifact: "orders_with_vendor",
				Params: map[string]string{
					"base_path":             "orders.json",
					"resolution_path":       "workflow_entity_resolutions",
					"base_key_fields":       `["_source_index"]`,
					"resolution_key_fields": `["item_id"]`,
					"filters_json":          `[{"field":"item_id","op":"contains","value":"raw_vendor"}]`,
					"target_id_field":       "canonical_vendor_id",
					"target_label_field":    "canonical_vendor_name",
					"target_status_field":   "vendor_status",
				},
			},
			{
				ID:             "apply_category",
				Kind:           DataActionApplyResolutions,
				InputPaths:     []string{"orders_with_vendor", "workflow_entity_resolutions"},
				OutputArtifact: "orders_with_category",
				Params: map[string]string{
					"base_path":             "orders_with_vendor",
					"resolution_path":       "workflow_entity_resolutions",
					"base_key_fields":       `["_source_index"]`,
					"resolution_key_fields": `["item_id"]`,
					"filters_json":          `[{"field":"item_id","op":"contains","value":"raw_category"}]`,
					"target_id_field":       "canonical_category_id",
					"target_label_field":    "canonical_category_name",
					"target_status_field":   "category_status",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run apply_entity_resolutions with filtered workflow ledger: %v", err)
	}
	var final DataArtifact
	for _, artifact := range res.Artifacts {
		if artifact.ID == "orders_with_category" {
			final = artifact
			break
		}
	}
	if final.ID == "" {
		t.Fatalf("Artifacts=%+v, want orders_with_category", res.Artifacts)
	}
	if final.Fields["matched_canonical_category_id"] != "2" || final.Fields["ambiguous_canonical_category_id"] != "0" {
		t.Fatalf("final fields=%+v, want category matches without ambiguity", final.Fields)
	}
	var first map[string]any
	if len(final.Sample) == 0 {
		t.Fatalf("final sample empty: %+v", final)
	}
	if err := json.Unmarshal([]byte(final.Sample[0]), &first); err != nil {
		t.Fatalf("sample %q: %v", final.Sample[0], err)
	}
	if first["canonical_vendor_id"] != "V1" || first["canonical_category_id"] != "C1" {
		t.Fatalf("sample row=%v, want vendor/category dimensions applied independently", first)
	}
	if first["canonical_vendor_id"] == first["canonical_category_id"] {
		t.Fatalf("sample row=%v, dimensions were cross-applied", first)
	}
}

func TestDedupeEntityResolutionRecordsCollapsesRepeatedWorkflowState(t *testing.T) {
	records := []EntityResolutionRecord{
		{
			ItemID:         "orders#1:name",
			SourceValue:    "alpha",
			CanonicalID:    "A",
			CanonicalLabel: "Alpha",
			Status:         "resolved",
			EvidenceRefs:   []string{"b", "a"},
			RuleRefs:       []string{"r2", "r1"},
		},
		{
			ItemID:         "orders#1:name",
			SourceValue:    "alpha",
			CanonicalID:    "A",
			CanonicalLabel: "Alpha",
			Status:         "resolved",
			EvidenceRefs:   []string{"a", "b"},
			RuleRefs:       []string{"r1", "r2"},
		},
		{
			ItemID:         "orders#2:name",
			SourceValue:    "beta",
			CanonicalID:    "B",
			CanonicalLabel: "Beta",
			Status:         "resolved",
		},
	}
	got := DedupeEntityResolutionRecords(records)
	if len(got) != 2 {
		t.Fatalf("len=%d records=%+v, want duplicate collapsed", len(got), got)
	}
	if got[0].ItemID.String() != "orders#1:name" || got[1].ItemID.String() != "orders#2:name" {
		t.Fatalf("records=%+v, want stable first occurrence order", got)
	}
}

func TestDedupeRuleCoverageRecordsCollapsesRepeatedWorkflowState(t *testing.T) {
	records := []RuleCoverageRecord{
		{
			RuleID:       "r1",
			RuleText:     "Use records that satisfy the current task rule",
			Status:       "derived",
			EvidenceRefs: []string{"b.md", "a.md"},
			Notes:        "source backed",
		},
		{
			RuleID:       "r1",
			RuleText:     "Use records that satisfy the current task rule",
			Status:       "derived",
			EvidenceRefs: []string{"a.md", "b.md"},
			Notes:        "source backed",
		},
		{
			RuleID:   "r2",
			RuleText: "Keep a different rule",
			Status:   "derived",
		},
	}
	got := DedupeRuleCoverageRecords(records)
	if len(got) != 2 {
		t.Fatalf("len=%d records=%+v, want duplicate collapsed", len(got), got)
	}
	if got[0].RuleID.String() != "r1" || got[1].RuleID.String() != "r2" {
		t.Fatalf("records=%+v, want stable first occurrence order", got)
	}
}

func TestActionRunnerDedupesSeedRuleCoverage(t *testing.T) {
	res, err := (ActionRunner{
		Seed: Result{
			RuleCoverage: []RuleCoverageRecord{{
				RuleID:       "r1",
				RuleText:     "Use records that satisfy the current task rule",
				Status:       "derived",
				EvidenceRefs: []string{"rules.md"},
				Notes:        "source backed",
			}},
		},
	}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RuleCoverageRequired: true,
		},
		Actions: []DataAction{{
			ID:   "derive_rules",
			Kind: DataActionDeriveRules,
			Params: map[string]string{
				"rules_json": `[{"id":"r1","text":"Use records that satisfy the current task rule","status":"derived","evidence_refs":["rules.md"],"notes":"source backed"}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_rules with duplicate seed: %v", err)
	}
	if len(res.RuleCoverage) != 1 {
		t.Fatalf("RuleCoverage=%+v, want duplicate seed/action rule collapsed", res.RuleCoverage)
	}
}

func TestActionRunnerEnrichRecordsSupportsMultipleBaseSourceFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_label,description\n1,alpha,first item\n2,,contains beta token\n3,gamma,unmatched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lookup.csv"), []byte("code,terms,label\nA,alpha;aaa,Alpha\nB,beta;bbb,Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "enrich",
			Kind:           DataActionEnrichRecords,
			InputPaths:     []string{"items.csv", "lookup.csv"},
			OutputArtifact: "items_enriched",
			Params: map[string]string{
				"base_path": "items.csv",
				"mapping_specs_json": `[
					{"mapping_path":"lookup.csv","source_fields":["raw_label","description"],"mapping_source_fields":["terms","label"],"mapping_value_field":"code","target_field":"canonical_code","match_mode":"contains"}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run enrich_records with multiple source fields: %v", err)
	}
	if got := res.Artifacts[0].Fields["matches_canonical_code"]; got != "2" {
		t.Fatalf("matches_canonical_code=%q, want 2; artifact=%+v", got, res.Artifacts[0])
	}
	var rows []map[string]any
	for _, sample := range res.Artifacts[0].Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["canonical_code"] != "A" || rows[1]["canonical_code"] != "B" {
		t.Fatalf("rows=%+v, want source field fallback matches", rows)
	}
}

func TestActionRunnerEnrichRecordsInfersReferenceSourceFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_label\n1,cloud\n2,security\n3,unknown\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lookup.csv"), []byte("canonical_code,display_name,aliases,description\nCLOUD,Cloud,cloud;compute,compute services\nSEC,Security,security;audit,security services\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "enrich",
			Kind:           DataActionEnrichRecords,
			InputPaths:     []string{"items.csv", "lookup.csv"},
			OutputArtifact: "items_enriched",
			Params: map[string]string{
				"base_path":           "items.csv",
				"mapping_path":        "lookup.csv",
				"source_field":        "raw_label",
				"mapping_value_field": "canonical_code",
				"target_field":        "canonical_code",
				"match_mode":          "contains",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run enrich_records with inferred reference fields: %v", err)
	}
	if got := res.Artifacts[0].Fields["matches_canonical_code"]; got != "2" {
		t.Fatalf("matches_canonical_code=%q, want 2; artifact=%+v", got, res.Artifacts[0])
	}
	var rows []map[string]any
	for _, sample := range res.Artifacts[0].Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["canonical_code"] != "CLOUD" || rows[1]["canonical_code"] != "SEC" {
		t.Fatalf("rows=%+v, want inferred reference source field matches", rows)
	}
}

func TestActionRunnerEnrichRecordsIndexesDelimitedReferenceTerms(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_label\n1,compute\n2,audit\n3,unknown\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lookup.csv"), []byte("canonical_code,display_name,aliases,description\nCLOUD,Cloud,cloud;compute,compute services\nSEC,Security,security;audit,security services\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "enrich",
			Kind:           DataActionEnrichRecords,
			InputPaths:     []string{"items.csv", "lookup.csv"},
			OutputArtifact: "items_enriched",
			Params: map[string]string{
				"base_path":           "items.csv",
				"mapping_path":        "lookup.csv",
				"source_field":        "raw_label",
				"mapping_value_field": "canonical_code",
				"target_field":        "canonical_code",
				"match_mode":          "exact",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run enrich_records with delimited inferred reference terms: %v", err)
	}
	if got := res.Artifacts[0].Fields["matches_canonical_code"]; got != "2" {
		t.Fatalf("matches_canonical_code=%q, want 2; artifact=%+v", got, res.Artifacts[0])
	}
	var rows []map[string]any
	for _, sample := range res.Artifacts[0].Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["canonical_code"] != "CLOUD" || rows[1]["canonical_code"] != "SEC" {
		t.Fatalf("rows=%+v, want exact matches from delimited reference terms", rows)
	}
}

func TestActionRunnerEnrichRecordsAppliesEntityResolutionArtifact(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,raw_label\n1,alpha\n2,beta\n3,gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{
			{
				ID:             "normalize",
				Kind:           DataActionNormalizeEntities,
				OutputArtifact: "label_map",
				Params: map[string]string{
					"resolutions_json": `[
						{"source_value":"alpha","canonical_id":"A","canonical_label":"Alpha"},
						{"source_value":"beta","canonical_id":"B","canonical_label":"Beta"}
					]`,
				},
			},
			{
				ID:             "apply",
				Kind:           DataActionEnrichRecords,
				InputPaths:     []string{"items.csv", "label_map"},
				OutputArtifact: "items_with_canonical",
				Params: map[string]string{
					"base_path":           "items.csv",
					"mapping_path":        "label_map",
					"source_field":        "raw_label",
					"mapping_value_field": "canonical_id",
					"target_field":        "canonical_label_id",
					"match_mode":          "exact",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run normalize_entities -> enrich_records: %v", err)
	}
	if got := res.Artifacts[len(res.Artifacts)-1].Fields["matches_canonical_label_id"]; got != "2" {
		t.Fatalf("matches_canonical_label_id=%q, want 2; artifact=%+v", got, res.Artifacts[len(res.Artifacts)-1])
	}
	var rows []map[string]any
	for _, sample := range res.Artifacts[len(res.Artifacts)-1].Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["canonical_label_id"] != "A" || rows[1]["canonical_label_id"] != "B" {
		t.Fatalf("rows=%+v, want canonical ids from entity resolution artifact", rows)
	}
	if len(res.EntityResolutions) == 0 {
		t.Fatalf("EntityResolutions empty, want enrich_records to emit mapping ledger")
	}
	if got := res.EntityResolutions[0].CanonicalID.String(); got != "A" {
		t.Fatalf("first enriched resolution canonical_id=%q, want A; resolutions=%+v", got, res.EntityResolutions)
	}
}

func TestActionRunnerDeriveFieldsNormalizesSlashOperationExamples(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("id,created_at,a,b,c\n1,2026-05-01,foo,bar,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"events.csv"},
			OutputArtifact: "events_derived",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_field":"created_at","target_field":"year","operation":"year/extract_year"},
					{"source_fields":["a","b"],"target_field":"joined","operation":"concat/join_fields","separator":"-"},
					{"source_fields":["c","b"],"target_field":"first","operation":"coalesce/first_non_empty"}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_fields slash operation aliases: %v", err)
	}
	if len(res.Artifacts) == 0 || len(res.Artifacts[0].Sample) == 0 {
		t.Fatalf("Artifacts=%+v, want derived sample", res.Artifacts)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if row["year"] != "2026" || row["joined"] != "foo-bar" || row["first"] != "bar" {
		t.Fatalf("row=%+v, want normalized slash operation outputs", row)
	}
}

func TestActionRunnerDeriveFieldsAcceptsStructuredParamAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("id,created_at,status\n1,2026-05-01, paid \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"events.csv"},
			OutputArtifact: "events_derived",
			Params: map[string]string{
				"field_specs": `[
					{"source_field":"created_at","target_field":"year","operation":"regex_extract","pattern":"^(\\d{4})"},
					{"source_field":"status","target_field":"status_clean","operation":"trim"}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_fields structured alias: %v", err)
	}
	var row map[string]any
	if len(res.Artifacts) == 0 || len(res.Artifacts[0].Sample) == 0 {
		t.Fatalf("Artifacts=%+v, want sample", res.Artifacts)
	}
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if row["year"] != "2026" || row["status_clean"] != "paid" {
		t.Fatalf("row=%+v, want derived values from field_specs alias", row)
	}
}

func TestActionRunnerExtractFieldsMaterializesMatchedTextRows(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.csv"), []byte("id,text\n1,alpha score=12 unit=ms\n2,no metric here\n3,beta score=7 unit=s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "extract",
			Kind:           DataActionExtractFields,
			InputPaths:     []string{"notes.csv"},
			OutputArtifact: "metrics",
			Params: map[string]string{
				"field_specs": `[
					{"source_field":"text","target_field":"metric_name","operation":"regex_extract","pattern":"^([a-z]+)\\s+score=", "group":1},
					{"source_field":"text","target_field":"metric_value","operation":"regex_extract","pattern":"score=(\\d+)", "group":1},
					{"source_field":"text","target_field":"metric_unit","operation":"regex_extract","pattern":"unit=([a-z]+)", "group":1}
				]`,
				"required_fields": `["metric_value","metric_unit"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run extract_fields: %v", err)
	}
	if len(res.Artifacts) == 0 || res.Artifacts[0].Kind != string(DataActionExtractFields) {
		t.Fatalf("Artifacts=%+v, want extract_fields artifact", res.Artifacts)
	}
	if res.Artifacts[0].RowCount != 2 || res.Artifacts[0].Fields["non_empty_metric_value"] != "2" {
		t.Fatalf("artifact=%+v, want two matched metric rows", res.Artifacts[0])
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if row["metric_name"] != "alpha" || row["metric_value"] != "12" || row["_extract_status"] != "matched" {
		t.Fatalf("row=%+v, want extracted text fields with locator-preserving status", row)
	}
}

func TestActionRunnerDeriveFieldsCanCopyVirtualSourceLocatorFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("id,amount\nA,10\nB,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"events.csv"},
			OutputArtifact: "events_derived",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_field":"_source_index","target_field":"row_index","operation":"copy"},
					{"source_field":"_source_locator","target_field":"row_locator","operation":"copy"}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_fields virtual locators: %v", err)
	}
	if len(res.Artifacts) == 0 || len(res.Artifacts[0].Sample) < 2 {
		t.Fatalf("Artifacts=%+v, want two derived samples", res.Artifacts)
	}
	var first, second map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &first); err != nil {
		t.Fatalf("unmarshal first sample: %v", err)
	}
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[1]), &second); err != nil {
		t.Fatalf("unmarshal second sample: %v", err)
	}
	if first["row_index"] != "1" || second["row_index"] != "2" {
		t.Fatalf("first=%+v second=%+v, want stable per-record row_index", first, second)
	}
	if !strings.Contains(fmt.Sprint(first["row_locator"]), "events.csv#1") {
		t.Fatalf("first=%+v, want source locator", first)
	}
}

func TestActionRunnerExtractFieldsCombinesMultipleSameSchemaInputs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("item=A\namount: 10\ncurrency: CNY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("item=B\namount: 25\ncurrency: USD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "extract",
			Kind:           DataActionExtractFields,
			InputPaths:     []string{"a.txt", "b.txt"},
			OutputArtifact: "parsed_text",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_field":"content","target_field":"item","operation":"regex_extract","pattern":"item=([A-Z])"},
					{"source_field":"content","target_field":"amount","operation":"regex_extract","pattern":"amount:\\s*(\\d+)"},
					{"source_field":"content","target_field":"currency","operation":"regex_extract","pattern":"currency:\\s*([A-Z]{3})"}
				]`,
				"required_fields": `["item","amount","currency"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run extract_fields multi-input: %v", err)
	}
	if len(res.Artifacts) == 0 {
		t.Fatalf("Artifacts=%+v, want extract_fields artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.RowCount != 2 || artifact.Fields["input_count"] != "2" {
		t.Fatalf("artifact=%+v, want two matched rows from two inputs", artifact)
	}
	if strings.Join(artifact.SourcePaths, ",") != "a.txt,b.txt" {
		t.Fatalf("SourcePaths=%v", artifact.SourcePaths)
	}
	if len(artifact.Children) != 2 {
		t.Fatalf("Children=%+v, want one child per input", artifact.Children)
	}
	var first, second map[string]any
	if err := json.Unmarshal([]byte(artifact.Sample[0]), &first); err != nil {
		t.Fatalf("unmarshal first sample: %v", err)
	}
	if err := json.Unmarshal([]byte(artifact.Sample[1]), &second); err != nil {
		t.Fatalf("unmarshal second sample: %v", err)
	}
	if first["item"] != "A" || second["item"] != "B" {
		t.Fatalf("first=%+v second=%+v, want extracted rows from both inputs", first, second)
	}
}

func TestActionRunnerExtractFieldsReadsDirectoryTextFiles(t *testing.T) {
	root := t.TempDir()
	evidenceDir := filepath.Join(root, "evidence")
	if err := os.Mkdir(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "A.txt"), []byte("record=A\namount: 12\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "B.txt"), []byte("record=B\namount: 34\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "extract_directory_text",
			Kind:           DataActionExtractFields,
			InputPaths:     []string{"evidence"},
			OutputArtifact: "parsed_evidence",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_field":"file_name","target_field":"record_id","operation":"regex_extract","pattern":"^([A-Z])\\.txt$","group":1},
					{"source_field":"text","target_field":"amount","operation":"regex_extract","pattern":"amount:\\s*([0-9]+)","group":1}
				]`,
				"required_fields": `["record_id","amount"]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run extract_fields directory text: %v", err)
	}
	if len(res.Artifacts) == 0 {
		t.Fatalf("Artifacts=%+v, want extract_fields artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.RowCount != 2 || artifact.Fields["input_count"] != "1" || artifact.Fields["input_rows"] != "2" {
		t.Fatalf("artifact=%+v, want two directory child records from one input", artifact)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(artifact.Sample[0]), &first); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if first["record_id"] != "A" || first["amount"] != "12" || first["_source_path"] != "evidence/A.txt" {
		t.Fatalf("first=%+v, want extracted fields and child file source path", first)
	}
}

func TestActionRunnerDeriveFieldsSkipsNoopReservedLocatorCopy(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("id,amount\nA,10\nB,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"events.csv"},
			OutputArtifact: "events_derived",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_field":"amount","target_field":"amount_number","operation":"parse_number"},
					{"source_field":"_source_index","target_field":"_source_index","operation":"copy"},
					{"source_field":"_source_locator","target_field":"_source_locator","operation":"copy"}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_fields noop reserved locator copy: %v", err)
	}
	if len(res.Artifacts) == 0 {
		t.Fatalf("Artifacts=%+v, want derive_fields artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.Fields["derived_fields"] != "amount_number" {
		t.Fatalf("derived_fields=%q, want only non-noop derived field", artifact.Fields["derived_fields"])
	}
	if got := artifact.Fields["noop_reserved_copy_fields"]; got != "_source_index,_source_locator" {
		t.Fatalf("noop_reserved_copy_fields=%q", got)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(artifact.Sample[0]), &first); err != nil {
		t.Fatalf("unmarshal first sample: %v", err)
	}
	if first["_source_index"] != "1" || !strings.Contains(fmt.Sprint(first["_source_locator"]), "events.csv#1") {
		t.Fatalf("first=%+v, want preserved source locator fields", first)
	}
	if first["amount_number"] != "10" {
		t.Fatalf("first=%+v, want parsed amount", first)
	}
}

func TestActionRunnerDeriveFieldsRejectsConstantIndexLikeFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("id,amount\nA,10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"events.csv"},
			OutputArtifact: "events_derived",
			Params: map[string]string{
				"field_specs_json": `[{"target_field":"row_index","operation":"constant","value":0}]`,
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "constant spec cannot create locator-like target_field") {
		t.Fatalf("err=%v, want locator-like constant rejection", err)
	}
}

func TestActionRunnerExpandRecordsSplitsMultiValueField(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("id,terms\n1,alpha; beta；alpha\n2,gamma|delta\n3,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "expand",
			Kind:           DataActionExpandRecords,
			InputPaths:     []string{"records.csv"},
			OutputArtifact: "expanded_terms",
			Params: map[string]string{
				"source_field":   "terms",
				"target_field":   "term",
				"original_field": "terms_raw",
				"delimiter":      "auto",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run expand_records: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one expand_records artifact", res.Artifacts)
	}
	artifact := res.Artifacts[0]
	if artifact.Kind != string(DataActionExpandRecords) || artifact.RowCount != 4 {
		t.Fatalf("artifact=%+v, want expand_records row_count=4", artifact)
	}
	if artifact.Fields["source_field"] != "terms" || artifact.Fields["target_field"] != "term" {
		t.Fatalf("artifact fields=%+v, want source/target fields", artifact.Fields)
	}
	var values []string
	var rows []map[string]any
	for _, sample := range artifact.Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
		values = append(values, row["term"].(string))
	}
	if strings.Join(values, ",") != "alpha,beta,gamma" {
		t.Fatalf("sample values=%v rows=%+v, want first three expanded terms", values, rows)
	}
	if rows[0]["terms_raw"] != "alpha; beta；alpha" || rows[0]["_source"] != "records.csv" {
		t.Fatalf("row0=%+v, want original field and source metadata", rows[0])
	}
}

func TestActionRunnerEnrichRecordsInfersMissingValueField(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("order_id,category_raw\nO1,compute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "category_mapping.csv"), []byte("source_value,label\ncompute,COMPUTE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "enrich",
			Kind:           DataActionEnrichRecords,
			InputPaths:     []string{"orders.csv", "category_mapping.csv"},
			OutputArtifact: "orders_enriched",
			Params: map[string]string{
				"base_path":             "orders.csv",
				"mapping_path":          "category_mapping.csv",
				"source_field":          "category_raw",
				"mapping_source_fields": `["source_value"]`,
				"mapping_value_field":   "canonical_id",
				"target_field":          "category_code",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run enrich_records inferred value field: %v", err)
	}
	if got := res.Artifacts[0].Fields["matches_category_code"]; got != "1" {
		t.Fatalf("matches_category_code=%q, want 1", got)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	if row["category_code"] != "COMPUTE" {
		t.Fatalf("row=%+v, want category_code inferred from label", row)
	}
}

func TestActionRunnerEnrichRecordsAcceptsLookupRoleAliases(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("order_id,category_raw\nO1,compute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "category_mapping.csv"), []byte("source_value,canonical_id\ncompute,COMPUTE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "enrich",
			Kind:           DataActionEnrichRecords,
			InputPaths:     []string{"orders.csv", "category_mapping.csv"},
			OutputArtifact: "orders_enriched",
			Params: map[string]string{
				"base_path": "orders.csv",
				"lookup_specs_json": `[{
					"lookup_path":"category_mapping.csv",
					"base_fields":["category_raw"],
					"lookup_fields":["source_value"],
					"lookup_value_field":"canonical_id",
					"target_field":"category_code"
				}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run enrich_records with lookup aliases: %v", err)
	}
	if got := res.Artifacts[0].Fields["matches_category_code"]; got != "1" {
		t.Fatalf("matches_category_code=%q, want 1", got)
	}
}

func TestActionRunnerEnrichRecordsSwapsInvertedBaseAndMapping(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("order_id,category_raw\nO1,compute\nO2,security\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "taxonomy.csv"), []byte("category_code,common_raw_terms\nCOMPUTE,compute;cloud\nSECURITY,security;audit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "enrich",
			Kind:           DataActionEnrichRecords,
			InputPaths:     []string{"taxonomy.csv", "orders.csv"},
			OutputArtifact: "orders_enriched",
			Params: map[string]string{
				"base_path":             "taxonomy.csv",
				"mapping_path":          "orders.csv",
				"source_field":          "category_raw",
				"mapping_source_fields": `["common_raw_terms"]`,
				"mapping_value_field":   "category_code",
				"target_field":          "category_code",
				"match_mode":            "mapping_contains_source",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run enrich_records inverted base/mapping repair: %v", err)
	}
	if got := res.Artifacts[0].Fields["base_path"]; got != "orders.csv" {
		t.Fatalf("base_path=%q, want deterministic swapped base orders.csv", got)
	}
	if got := res.Artifacts[0].Fields["matches_category_code"]; got != "2" {
		t.Fatalf("matches_category_code=%q, want 2", got)
	}
}

func TestActionRunnerEnrichRecordsRejectsMissingBaseSourceField(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("order_id,category_raw\nO1,compute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "category_mapping.csv"), []byte("source_value,canonical_id\ncompute,COMPUTE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "bad_enrich",
			Kind:           DataActionEnrichRecords,
			InputPaths:     []string{"category_mapping.csv", "orders.csv"},
			OutputArtifact: "orders_enriched",
			Params: map[string]string{
				"base_path":             "category_mapping.csv",
				"mapping_path":          "orders.csv",
				"source_field":          "category_raw",
				"mapping_source_fields": `["lookup"]`,
				"mapping_value_field":   "canonical_id",
				"target_field":          "category_code",
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), `enrich_records source_fields [category_raw] were not found in base input category_mapping.csv fields`) {
		t.Fatalf("err=%v, want missing base source-field validation", err)
	}
	var fieldErr DataFieldContractError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("err=%T %v, want DataFieldContractError", err, err)
	}
	if fieldErr.ActionKind != DataActionEnrichRecords || fieldErr.Role != "base" || fieldErr.InputAlias != "category_mapping.csv" || len(fieldErr.missingFields()) != 1 || fieldErr.missingFields()[0] != "category_raw" {
		t.Fatalf("fieldErr=%+v, want typed enrich base field contract", fieldErr)
	}
}

func TestActionRunnerEnrichRecordsRejectsMissingMappingFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("order_id,category_raw\nO1,compute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "category_mapping.csv"), []byte("source_value,notes\ncompute,Compute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "bad_mapping",
			Kind:           DataActionEnrichRecords,
			InputPaths:     []string{"orders.csv", "category_mapping.csv"},
			OutputArtifact: "orders_enriched",
			Params: map[string]string{
				"base_path":             "orders.csv",
				"mapping_path":          "category_mapping.csv",
				"source_field":          "category_raw",
				"mapping_source_fields": `["source_value"]`,
				"mapping_value_field":   "canonical_id",
				"target_field":          "category_code",
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), `enrich_records mapping_value_field "canonical_id" was not found in mapping input category_mapping.csv fields`) {
		t.Fatalf("err=%v, want missing mapping value-field validation", err)
	}
	var fieldErr DataFieldContractError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("err=%T %v, want DataFieldContractError", err, err)
	}
	if fieldErr.ActionKind != DataActionEnrichRecords || fieldErr.Role != "mapping_value" || fieldErr.InputAlias != "category_mapping.csv" || len(fieldErr.missingFields()) != 1 || fieldErr.missingFields()[0] != "canonical_id" {
		t.Fatalf("fieldErr=%+v, want typed enrich mapping field contract", fieldErr)
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

func TestActionRunnerDeriveFieldsParsesMappingJSONString(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("id,status\nA,ok\nB,skip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"events.csv"},
			OutputArtifact: "events_derived",
			Params: map[string]string{
				"field_specs_json": `[{"source_field":"status","target_field":"bucket","operation":"map","mapping":"{\"ok\":\"include\",\"skip\":\"exclude\"}"}]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run derive_fields with mapping JSON string: %v", err)
	}
	if len(res.Artifacts) == 0 || res.Artifacts[0].Kind != string(DataActionDeriveFields) {
		t.Fatalf("Artifacts=%+v, want derive_fields artifact", res.Artifacts)
	}
	if len(res.Artifacts[0].Sample) != 2 {
		t.Fatalf("sample=%+v, want 2 rows", res.Artifacts[0].Sample)
	}
	var rows []map[string]any
	for _, sample := range res.Artifacts[0].Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["bucket"] != "include" || rows[1]["bucket"] != "exclude" {
		t.Fatalf("rows=%+v, want mapped buckets", rows)
	}
}

func TestActionRunnerDeriveFieldsCanUsePriorSpecOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("id,status\nA,ok\nB,skip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"events.csv"},
			OutputArtifact: "events_derived",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_field":"status","target_field":"status_norm","operation":"map","mapping":{"ok":"valid","skip":"drop"}},
					{"source_field":"status_norm","target_field":"eligible","operation":"map","mapping":{"valid":"true","drop":"false"}}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_fields with prior spec output: %v", err)
	}
	var rows []map[string]any
	for _, sample := range res.Artifacts[0].Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["eligible"] != "true" || rows[1]["eligible"] != "false" {
		t.Fatalf("rows=%+v, want later spec to read earlier derived field", rows)
	}
}

func TestActionRunnerDeriveFieldsSupportsMultiSourceConcatAndCoalesce(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("id,a,b,c\n1,foo,bar,\n2,,bee,fallback\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"records.csv"},
			OutputArtifact: "derived",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_fields":["a","b"],"target_field":"joined","operation":"concat","separator":"|"},
					{"source_fields":["a","c","b"],"target_field":"first_value","operation":"coalesce"}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_fields concat/coalesce: %v", err)
	}
	if len(res.Artifacts) != 1 || len(res.Artifacts[0].Sample) != 2 {
		t.Fatalf("Artifacts=%+v, want two sample rows", res.Artifacts)
	}
	var rows []map[string]any
	for _, sample := range res.Artifacts[0].Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["joined"] != "foo|bar" || rows[0]["first_value"] != "foo" {
		t.Fatalf("row0=%+v, want concat and first non-empty", rows[0])
	}
	if rows[1]["joined"] != "bee" || rows[1]["first_value"] != "fallback" {
		t.Fatalf("row1=%+v, want concat and coalesce", rows[1])
	}
}

func TestActionRunnerDeriveFieldsAllowsPartialMultiSourceFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("id,b\n1,bee\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"records.csv"},
			OutputArtifact: "derived",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_fields":["missing","b"],"target_field":"first_value","operation":"coalesce"},
					{"source_fields":["missing","b"],"target_field":"joined","operation":"concat","separator":"|"}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_fields with partial multi-source fields: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatalf("unmarshal sample %q: %v", res.Artifacts[0].Sample[0], err)
	}
	if row["first_value"] != "bee" || row["joined"] != "bee" {
		t.Fatalf("row=%+v, want partial-source coalesce/concat to use available field", row)
	}
}

func TestActionRunnerDeriveFieldsSupportsCaseWhen(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("id,kind,a,b\n1,use_a,10,20\n2,use_b,30,40\n3,skip,50,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "derive",
			Kind:           DataActionDeriveFields,
			InputPaths:     []string{"records.csv"},
			OutputArtifact: "derived",
			Params: map[string]string{
				"field_specs_json": `[
					{"target_field":"chosen","operation":"case_when","cases":[
						{"filters":[{"field":"kind","op":"eq","value":"use_a"}],"value_field":"a"},
						{"filters":[{"field":"kind","op":"eq","value":"use_b"}],"value_field":"b"}
					],"default":"0"}
				]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run derive_fields case_when: %v", err)
	}
	var rows []map[string]any
	for _, sample := range res.Artifacts[0].Sample {
		var row map[string]any
		if err := json.Unmarshal([]byte(sample), &row); err != nil {
			t.Fatalf("unmarshal sample %q: %v", sample, err)
		}
		rows = append(rows, row)
	}
	if rows[0]["chosen"] != "10" || rows[1]["chosen"] != "40" || rows[2]["chosen"] != "0" {
		t.Fatalf("rows=%+v, want conditional field values", rows)
	}
}

func TestActionRunnerDeriveFieldsUnsupportedOperationReportsOperationFirst(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("id,a,b\n1,10,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:         "derive",
			Kind:       DataActionDeriveFields,
			InputPaths: []string{"records.csv"},
			Params: map[string]string{
				"field_specs_json": `[{"source_fields":["a","b"],"target_field":"x","operation":"not_a_supported_op"}]`,
			},
		}},
	})
	if err == nil {
		t.Fatal("Run derive_fields unsupported operation succeeded, want error")
	}
	if !strings.Contains(err.Error(), "unsupported operation") || strings.Contains(err.Error(), "requires source_field") {
		t.Fatalf("err=%q, want unsupported-operation contract error before source_field guidance", err)
	}
}

func TestActionRunnerMaterializedArtifactCarriesPayloadHeaders(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "vendors.csv"), []byte("vendor_id,vendor_name\nV1,Acme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "vendor_map",
			Kind:           DataActionNormalizeEntities,
			InputPaths:     []string{"vendors.csv"},
			OutputArtifact: "vendor_map",
			Params: map[string]string{
				"source_field":          "vendor_name",
				"canonical_id_field":    "vendor_id",
				"canonical_label_field": "vendor_name",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run normalize_entities: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("Artifacts=%+v, want one artifact", res.Artifacts)
	}
	headers := strings.Join(res.Artifacts[0].Headers, ",")
	for _, want := range []string{"source_value", "canonical_id", "canonical_label", "status"} {
		if !strings.Contains(headers, want) {
			t.Fatalf("headers=%q, missing %q", headers, want)
		}
	}
	if !strings.Contains(res.Artifacts[0].Fields["output_headers"], "source_value") {
		t.Fatalf("Fields=%+v, want output_headers from materialized payload", res.Artifacts[0].Fields)
	}
}

func TestDataActionArtifactPayloadHeadersUnionArrayRecords(t *testing.T) {
	raw := []byte(`[
		{"item_id":"row-1","source_value":"a","status":"unresolved"},
		{"item_id":"row-2","source_value":"b","canonical_id":"B","canonical_label":"Bee","status":"resolved"}
	]`)
	headers := strings.Join(dataActionArtifactPayloadHeaders(raw), ",")
	for _, want := range []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"} {
		if !strings.Contains(headers, want) {
			t.Fatalf("headers=%q, missing %q", headers, want)
		}
	}
}

func TestDataActionArtifactPayloadHeadersPreferWrapperRecords(t *testing.T) {
	raw := []byte(`{
		"records": [],
		"headers": ["id", "value"],
		"diagnostics": {"count":"0"},
		"fields": {"metadata_only":"true"}
	}`)
	headers := dataActionArtifactPayloadHeaders(raw)
	if got := strings.Join(headers, ","); got != "id,value" {
		t.Fatalf("headers=%q, want wrapper record headers only", got)
	}
}

func TestActionRunnerNormalizeEntitiesAppliesDefaultStatusToExplicitRecords(t *testing.T) {
	root := t.TempDir()
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "normalize_statuses",
			Kind:           DataActionNormalizeEntities,
			OutputArtifact: "status_map",
			Params: map[string]string{
				"resolutions_json": `[{"item_id":"s1","source_value":"已支付","canonical_id":"paid","canonical_label":"paid"}]`,
				"default_status":   "resolved",
				"reason":           "map status values",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run normalize_entities explicit records without status: %v", err)
	}
	if len(res.EntityResolutions) != 1 || res.EntityResolutions[0].Status.String() != "resolved" {
		t.Fatalf("EntityResolutions=%+v, want default status", res.EntityResolutions)
	}
}

func TestActionRunnerNormalizeEntitiesEmptyExplicitMappingsFallbackToStructuredInputs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "vendors.csv"), []byte("vendor_id,vendor_name\nV1,Acme\nV2,Globex\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "vendor_map",
			Kind:           DataActionNormalizeEntities,
			InputPaths:     []string{"vendors.csv"},
			OutputArtifact: "vendor_map",
			Params: map[string]string{
				"resolutions_json":      `[]`,
				"source_field":          "vendor_name",
				"canonical_id_field":    "vendor_id",
				"canonical_label_field": "vendor_name",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run normalize_entities empty explicit mappings: %v", err)
	}
	if len(res.EntityResolutions) != 2 {
		t.Fatalf("EntityResolutions=%+v, want structured fallback records", res.EntityResolutions)
	}
	if got := res.Artifacts[0].Fields["count"]; got != "2" {
		t.Fatalf("artifact count=%q, want 2", got)
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
	var fieldErr DataFieldContractError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("err=%T %v, want DataFieldContractError", err, err)
	}
	if fieldErr.ActionKind != DataActionComputeContribs || fieldErr.Role != "value" || len(fieldErr.missingFields()) != 1 || fieldErr.missingFields()[0] != "amount" {
		t.Fatalf("fieldErr=%+v, want typed contribution value field contract", fieldErr)
	}
}

func TestActionRunnerComputeContributionsRejectsUnknownFilterField(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status,amount\nA,paid,10\nB,draft,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
		},
		Actions: []DataAction{{
			ID:         "contrib",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"orders.csv"},
			Params: map[string]string{
				"group_key":    "all",
				"metric":       "amount",
				"value_field":  "amount",
				"operation":    "add",
				"filters_json": `[{"field":"status_flag","op":"eq","value":"paid"}]`,
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run err=nil, want missing filter field diagnostic")
	}
	text := err.Error()
	for _, want := range []string{"filter field", "status_flag", "orders.csv", "status", "field_samples"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Run err=%q, want substring %q", text, want)
		}
	}
	var fieldErr DataFieldContractError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("err=%T %v, want DataFieldContractError", err, err)
	}
	if fieldErr.ActionKind != DataActionComputeContribs || fieldErr.Role != "filter" || fieldErr.InputAlias != "orders.csv" || len(fieldErr.missingFields()) != 1 || fieldErr.missingFields()[0] != "status_flag" {
		t.Fatalf("fieldErr=%+v, want typed contribution filter field contract", fieldErr)
	}
}

func TestActionRunnerComputeContributionsRejectsNonNumericValueField(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status,eligible,amount\nA,paid,true,10\nB,paid,true,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
		},
		Actions: []DataAction{{
			ID:         "contrib",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"orders.csv"},
			Params: map[string]string{
				"group_key":    "all",
				"metric":       "eligible",
				"value_field":  "eligible",
				"operation":    "add",
				"filters_json": `[{"field":"status","op":"eq","value":"paid"}]`,
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run err=nil, want numeric value contract failure")
	}
	text := err.Error()
	for _, want := range []string{"numeric field contract", "value_field", "eligible", "parse_number"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Run err=%q, want substring %q", text, want)
		}
	}
}

func TestActionRunnerComputeContributionsAcceptsStructuredFilterAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status,amount\nA,paid,10\nB,draft,5\nC,accepted,7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
		},
		Actions: []DataAction{{
			ID:         "contrib",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"orders.csv"},
			Params: map[string]string{
				"group_key":   "all",
				"metric":      "amount",
				"value_field": "amount",
				"operation":   "add",
				"filters":     `[{"field":"status","op":"in","value":["paid","accepted"]}]`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run compute_contributions structured filter alias: %v", err)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want paid and accepted rows", res.Contributions)
	}
}

func TestActionRunnerComputeContributionsZeroRequiredLedgerHasDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status,amount\nA,paid,10\nB,draft,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
		},
		Actions: []DataAction{{
			ID:         "contrib",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"orders.csv"},
			Params: map[string]string{
				"group_key":    "all",
				"metric":       "amount",
				"value_field":  "amount",
				"operation":    "add",
				"filters_json": `[{"field":"status","op":"eq","value":"closed"}]`,
			},
		}},
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run err=nil, want zero contribution diagnostic")
	}
	text := err.Error()
	for _, want := range []string{"zero contribution", "filter_matched=0", "field_samples", "paid"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Run err=%q, want substring %q", text, want)
		}
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

func TestActionRunnerMaterialInventoryDoesNotConsumeCoverageMaterials(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("- include completed rows\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status\n1,completed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RequiredMaterials: []CoverageMaterial{
				{Path: "rules.md", Purpose: "task rules", Required: true, UsageMode: MaterialUseScriptConsumed},
				{Path: "orders.csv", Purpose: "source rows", Required: true, UsageMode: MaterialUseScriptConsumed},
			},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{{
			ID:         "inventory",
			Kind:       DataActionMaterialInventory,
			InputPaths: []string{"rules.md", "orders.csv"},
		}},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("material inventory should be an intermediate discovery action, got %v", err)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "" {
		t.Fatalf("ConsumedPaths=%q, want no consumption evidence from material_inventory", got)
	}
	if len(res.Artifacts) == 0 {
		t.Fatalf("Artifacts empty")
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

func TestActionRunnerDeriveRulesDoesNotTurnDataRowsIntoRules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("id,status,amount\n1,paid,10\n2,draft,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			ValidationRules:      []string{"include paid rows only"},
			RuleCoverageRequired: true,
		},
		Actions: []DataAction{{
			ID:         "rules",
			Kind:       DataActionDeriveRules,
			InputPaths: []string{"orders.csv"},
		}},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run actions: %v", err)
	}
	if got := strings.Join(res.ConsumedPaths, ","); got != "" {
		t.Fatalf("ConsumedPaths=%q, want ordinary data rows not consumed as rule material", got)
	}
	if len(res.RuleCoverage) != 1 {
		t.Fatalf("RuleCoverage=%+v, want only validation rule fallback", res.RuleCoverage)
	}
	if got := res.RuleCoverage[0].RuleText.String(); got != "include paid rows only" {
		t.Fatalf("RuleText=%q, want validation rule text", got)
	}
	if got := strings.Join(res.RuleCoverage[0].EvidenceRefs, ","); strings.Contains(got, "orders.csv") {
		t.Fatalf("EvidenceRefs=%v, ordinary data input should not back rule coverage", res.RuleCoverage[0].EvidenceRefs)
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

func TestActionRunnerNormalizeEntitiesSourceFieldOverridesContextFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("row_id,raw_name,context_a,context_b,canonical_id,label\n1,Acme,ignore-a,ignore-b,V001,Acme Ltd\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			RequiredMaterials:        []CoverageMaterial{{Path: "items.csv", Required: true}},
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:         "normalize_items",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"items.csv"},
			Params: map[string]string{
				"source_field":          "raw_name",
				"source_fields":         `["context_a","context_b","raw_name"]`,
				"canonical_id_field":    "canonical_id",
				"canonical_label_field": "label",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run structured normalize action: %v", err)
	}
	if len(res.EntityResolutions) != 1 {
		t.Fatalf("EntityResolutions=%+v, want only the explicit source_field", res.EntityResolutions)
	}
	if got := res.EntityResolutions[0].ItemID.String(); !strings.HasSuffix(got, ":raw_name") {
		t.Fatalf("ItemID=%q, want raw_name resolution only", got)
	}
	if res.EntityResolutions[0].SourceValue.String() != "Acme" {
		t.Fatalf("SourceValue=%q, want Acme", res.EntityResolutions[0].SourceValue.String())
	}
}

func TestActionRunnerNormalizeEntitiesAllowsEmptyIntermediateArtifact(t *testing.T) {
	root := t.TempDir()
	tempRoot := t.TempDir()
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
	res, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot}).Run(context.Background(), plan)
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
	artifactPath := res.Artifacts[0].Fields["artifact_path"]
	raw, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read materialized artifact: %v", err)
	}
	if strings.TrimSpace(string(raw)) == "null" {
		t.Fatalf("empty entity artifact materialized as null")
	}
	if !strings.Contains(string(raw), `"entity_resolutions":[]`) {
		t.Fatalf("empty entity artifact=%s, want entity_resolutions wrapper", raw)
	}
}

func TestActionRunnerNormalizeEntitiesRoutesGenericFilterToReferenceByMatchCount(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("po_id,vendor_name_raw,status\nPO1,Acme Cloud Ltd,completed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vendors.csv"), []byte("vendor_id,legal_name,short_name,status\nV001,Acme Cloud Ltd,Acme,active\nV002,Old Vendor,Old,inactive\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:         "normalize_vendors",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"orders.csv", "vendors.csv"},
			Params: map[string]string{
				"source_field":          "vendor_name_raw",
				"reference_name_fields": "legal_name,short_name",
				"canonical_id_field":    "vendor_id",
				"canonical_label_field": "legal_name",
				"match_mode":            "exact",
				"filters_json":          `[{"field":"status","op":"eq","value":"active"}]`,
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run reference-filter normalize action: %v", err)
	}
	if len(res.EntityResolutions) != 1 {
		t.Fatalf("EntityResolutions=%+v, want one resolution", res.EntityResolutions)
	}
	if got := res.EntityResolutions[0].CanonicalID.String(); got != "V001" {
		t.Fatalf("CanonicalID=%q, want V001; records=%+v", got, res.EntityResolutions)
	}
	if len(res.Artifacts) == 0 || len(res.Artifacts[0].Children) < 2 {
		t.Fatalf("Artifacts=%+v, want source/reference diagnostics", res.Artifacts)
	}
	if notes := res.Artifacts[0].Children[0].Fields["filter_role_notes"]; !strings.Contains(notes, "reference_by_match_count") {
		t.Fatalf("filter_role_notes=%q, want reference_by_match_count", notes)
	}
}

func TestActionRunnerNormalizeEntitiesUsesReferenceCanonicalID(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.csv"), []byte("item_id,label\n1,Alpha Alias\n2,Beta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.csv"), []byte("code,name,aliases\nA,Alpha,Alpha Alias;Alpha A\nB,Beta,Beta Alias\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:         "normalize_labels",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"source.csv", "reference.csv"},
			Params: map[string]string{
				"source_field":          "label",
				"reference_name_fields": `["name","aliases"]`,
				"canonical_id_field":    "code",
				"canonical_label_field": "name",
				"match_mode":            "mapping_contains_source",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run reference canonical normalize action: %v", err)
	}
	if len(res.EntityResolutions) != 2 {
		t.Fatalf("EntityResolutions=%+v, want 2", res.EntityResolutions)
	}
	for _, rec := range res.EntityResolutions {
		if rec.CanonicalID.String() == "" {
			t.Fatalf("CanonicalID empty for reference normalization: %+v", rec)
		}
	}
}

func TestActionRunnerNormalizeEntitiesInfersReferenceFirstInputOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("item_id,label\n1,Alpha Alias\n2,Beta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "canonical.csv"), []byte("code,name,aliases\nA,Alpha,Alpha Alias;Alpha A\nB,Beta,Beta Alias\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:         "normalize_labels",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"canonical.csv", "records.csv"},
			Params: map[string]string{
				"source_field":          "label",
				"reference_name_fields": `["name","aliases"]`,
				"canonical_id_field":    "code",
				"canonical_label_field": "name",
				"match_mode":            "mapping_contains_source",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run reference-first normalize action: %v", err)
	}
	got := map[string]string{}
	for _, rec := range res.EntityResolutions {
		got[rec.SourceValue.String()] = rec.CanonicalID.String()
	}
	if got["Alpha Alias"] != "A" || got["Beta"] != "B" {
		t.Fatalf("EntityResolutions=%+v, want inferred source/reference roles", res.EntityResolutions)
	}
	if len(res.Artifacts) == 0 || len(res.Artifacts[0].Children) < 2 {
		t.Fatalf("Artifacts=%+v, want source/reference diagnostics", res.Artifacts)
	}
	if got := res.Artifacts[0].Children[0].Fields["role_inference"]; !strings.Contains(got, "swapped_source_reference_by_field_contract") {
		t.Fatalf("role_inference=%q, want structural swap note", got)
	}
}

func TestActionRunnerNormalizeEntitiesExplicitSourcePathUsesNonSourceReferenceInput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("item_id,label\n1,Alpha Alias\n2,Beta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "canonical.csv"), []byte("code,name,aliases\nA,Alpha,Alpha Alias;Alpha A\nB,Beta,Beta Alias\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:         "normalize_labels",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"canonical.csv", "records.csv"},
			Params: map[string]string{
				"source_path":           "records.csv",
				"source_field":          "label",
				"reference_name_fields": `["name","aliases"]`,
				"canonical_id_field":    "code",
				"canonical_label_field": "name",
				"match_mode":            "mapping_contains_source",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run explicit-source normalize action: %v", err)
	}
	got := map[string]string{}
	for _, rec := range res.EntityResolutions {
		got[rec.SourceValue.String()] = rec.CanonicalID.String()
	}
	if got["Alpha Alias"] != "A" || got["Beta"] != "B" {
		t.Fatalf("EntityResolutions=%+v, want non-source input used as reference", res.EntityResolutions)
	}
}

func TestActionRunnerNormalizeEntitiesUsesUnicodeReferenceCanonicalID(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.csv"), []byte("item_id,label\n1,算力与云资源服务\n2,制度\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.csv"), []byte("code,name,aliases,keywords\nCOMPUTE_SERVICE,算力与云资源服务,云资源;算力,GPU;压测资源\nAI_CONSULTING,AI 咨询与治理,咨询;制度,AI治理;政策工作坊\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputMarkdown, ExplanationAllowed: true},
		CoverageContract: CoverageContract{
			EntityResolutionRequired: true,
		},
		Actions: []DataAction{{
			ID:         "normalize_labels",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"source.csv", "reference.csv"},
			Params: map[string]string{
				"source_field":          "label",
				"reference_name_fields": `["name","aliases","keywords"]`,
				"canonical_id_field":    "code",
				"canonical_label_field": "name",
				"match_mode":            "mapping_contains_source",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run unicode reference canonical normalize action: %v", err)
	}
	got := map[string]string{}
	for _, rec := range res.EntityResolutions {
		got[rec.SourceValue.String()] = rec.CanonicalID.String()
	}
	if got["算力与云资源服务"] != "COMPUTE_SERVICE" || got["制度"] != "AI_CONSULTING" {
		t.Fatalf("EntityResolutions=%+v, want unicode canonical ids", res.EntityResolutions)
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

func TestActionRunnerReconcileAnswerBeatsSeedArtifactSummary(t *testing.T) {
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{{ID: "reconcile", Kind: DataActionReconcile}},
	}
	seed := Result{
		Answer:         "- emitted_payload: previous material summary",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Contributions: []ContributionRecord{{
			ItemID:        LooseText("item-1"),
			Source:        LooseText("records.csv"),
			SourceLocator: LooseText("line:2"),
			GroupKey:      LooseText("Q001"),
			Metric:        LooseText("amount"),
			Value:         LooseText("42"),
			Operation:     LooseText("add"),
			Role:          LooseText("target"),
		}},
	}
	res, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "42" {
		t.Fatalf("Answer=%q, want reconcile-rendered value instead of seed artifact summary", res.Answer)
	}
}

func TestActionRunnerComputeContributionsProducesDecisionRows(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("query,amount,status\nQ002,7,done\nQ001,5,draft\nQ001,10,done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"records.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:          []CoverageMaterial{{Path: "records.csv", Required: true}},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		ContinueAfter:  true,
		Actions: []DataAction{
			{
				ID:             "contribs",
				Kind:           DataActionComputeContribs,
				InputPaths:     []string{"records.csv"},
				OutputArtifact: "contribs.json",
				Params: map[string]string{
					"group_key_field": "query",
					"value_field":     "amount",
					"item_id_field":   "_source_index",
					"metric":          "amount",
					"operation":       "add",
					"filter_field":    "status",
					"filter_equals":   "done",
				},
			},
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
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want 2 matching records", res.Contributions)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("Rows=%+v, want decision rows derived from contributions", res.Rows)
	}
	if res.Rows[0].Decision != "include" || strings.TrimSpace(res.Rows[0].SourceLocator) == "" {
		t.Fatalf("Rows=%+v, want meaningful include decisions", res.Rows)
	}
	if res.Reconcile == nil || len(res.Reconcile.Groups) != 2 {
		t.Fatalf("Reconcile=%+v, want groups derived from contributions", res.Reconcile)
	}
}

func TestActionRunnerComputeContributionsTreatsGroupKeyParamAsFieldWhenItMatchesInput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "records.csv"), []byte("query,amount\nQ002,7\nQ001,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"records.csv"},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		Actions: []DataAction{
			{
				ID:             "contribs",
				Kind:           DataActionComputeContribs,
				InputPaths:     []string{"records.csv"},
				OutputArtifact: "contribs.json",
				Params: map[string]string{
					"group_key":   "query",
					"value_field": "amount",
					"metric":      "amount",
					"operation":   "add",
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
			{ID: "answer", Kind: DataActionAssembleAnswer, Params: map[string]string{"projection": "values", "delimiter": ","}},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "10,7" {
		t.Fatalf("Answer=%q, want grouped values ordered by query field", res.Answer)
	}
	if got := len(res.Reconcile.Groups); got != 2 {
		t.Fatalf("Reconcile groups=%d, want 2: %+v", got, res.Reconcile.Groups)
	}
}

func TestActionRunnerComputeContributionsSkipsReferenceInputsMissingContributionFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "facts.csv"), []byte("query_id,amount,status\nQ002,7,done\nQ001,10,done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.csv"), []byte("query_id\nQ001\nQ002\nQ003\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"facts.csv", "reference.csv"},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
			Delimiter:          ",",
			CompleteReference:  true,
			ReferencePath:      "reference.csv",
			ReferenceKeyField:  "query_id",
		},
		Actions: []DataAction{
			{
				ID:         "contribs",
				Kind:       DataActionComputeContribs,
				InputPaths: []string{"facts.csv", "reference.csv"},
				Params: map[string]string{
					"group_key_field": "query_id",
					"value_field":     "amount",
					"operation":       "add",
					"filters":         `[{"field":"status","op":"eq","value":"done"}]`,
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
			{ID: "answer", Kind: DataActionAssembleAnswer, Params: map[string]string{"projection": "values", "delimiter": ","}},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "10,7,0" {
		t.Fatalf("Answer=%q, want contribution facts plus zero-filled reference", res.Answer)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("Contributions=%+v, want only fact rows", res.Contributions)
	}
}

func TestActionRunnerComputeContributionsInfersValueFieldFromMetric(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "events.csv"), []byte("id,group,duration_ms\n1,A,7\n2,A,5\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		CoverageContract: CoverageContract{ContributionLedgerRequired: true, ReconcileRequired: true},
		OutputContract:   OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{
			{
				ID:             "compute",
				Kind:           DataActionComputeContribs,
				InputPaths:     []string{"events.csv"},
				OutputArtifact: "contribs.json",
				Params: map[string]string{
					"group_key_field": "group",
					"item_id_field":   "id",
					"metric":          "duration_ms",
					"operation":       "sum",
				},
			},
			{ID: "reconcile", Kind: DataActionReconcile},
			{ID: "answer", Kind: DataActionAssembleAnswer, Params: map[string]string{"projection": "values"}},
		},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Reconcile == nil || len(res.Reconcile.Groups) != 1 {
		t.Fatalf("Reconcile=%+v, want one group", res.Reconcile)
	}
	if got := res.Reconcile.Groups[0].Actual.String(); got != "12" {
		t.Fatalf("Actual=%q, want 12", got)
	}
}

func TestActionRunnerAssembleAnswerCompletesReferenceKeysWithZero(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "queries.csv"), []byte("query_id\nQ001\nQ002\nQ003\n"), 0600); err != nil {
		t.Fatal(err)
	}
	seed := Result{Contributions: []ContributionRecord{{
		ItemID:        LooseText("row-1"),
		Source:        LooseText("records.csv"),
		SourceLocator: LooseText("row 2"),
		GroupKey:      LooseText("Q002"),
		Metric:        LooseText("amount"),
		Value:         LooseText("7"),
		Operation:     LooseText("add"),
		Role:          LooseText("target"),
	}}}
	plan := TaskPlan{
		InputPaths: []string{"queries.csv"},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		Actions: []DataAction{
			{ID: "reconcile", Kind: DataActionReconcile},
			{
				ID:         "answer",
				Kind:       DataActionAssembleAnswer,
				InputPaths: []string{"queries.csv"},
				Params: map[string]string{
					"projection":          "values",
					"complete_reference":  "true",
					"reference_key_field": "query_id",
					"metric":              "amount",
					"delimiter":           ",",
				},
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "0,7,0" {
		t.Fatalf("Answer=%q, want reference-complete value list", res.Answer)
	}
}

func TestActionRunnerAssembleAnswerCompletesReferenceKeysFromOutputContract(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "queries.csv"), []byte("query_id\nQ001\nQ002\nQ003\n"), 0600); err != nil {
		t.Fatal(err)
	}
	seed := Result{Contributions: []ContributionRecord{{
		ItemID:        LooseText("row-1"),
		Source:        LooseText("records.csv"),
		SourceLocator: LooseText("row 2"),
		GroupKey:      LooseText("Q002"),
		Metric:        LooseText("amount"),
		Value:         LooseText("7"),
		Operation:     LooseText("add"),
		Role:          LooseText("target"),
	}}}
	plan := TaskPlan{
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
			Delimiter:          ",",
			CompleteReference:  true,
			ReferencePath:      "queries.csv",
			ReferenceKeyField:  "query_id",
		},
		Actions: []DataAction{
			{ID: "reconcile", Kind: DataActionReconcile},
			{
				ID:     "answer",
				Kind:   DataActionAssembleAnswer,
				Params: map[string]string{"projection": "values", "delimiter": ","},
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "0,7,0" {
		t.Fatalf("Answer=%q, want reference-complete value list from output contract", res.Answer)
	}
}

func TestActionRunnerInfersReferenceKeyCandidateStructurally(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "targets.csv"), []byte("target\nA\nB\nC\n"), 0600); err != nil {
		t.Fatal(err)
	}
	candidate, ok := (ActionRunner{RepoRoot: root}).InferReferenceKeyCandidate(nil, []string{"A", "C"}, []string{"targets.csv"}, nil, 100)
	if !ok {
		t.Fatal("InferReferenceKeyCandidate returned no candidate")
	}
	if candidate.Path != "targets.csv" || candidate.Field != "target" || candidate.KeyCount != 3 || candidate.MissingCount != 1 {
		t.Fatalf("candidate=%+v, want structural reference candidate over targets.csv.target", candidate)
	}
	if got := strings.Join(candidate.MissingKeys, ","); got != "B" {
		t.Fatalf("MissingKeys=%q, want B", got)
	}
}

func TestActionRunnerAssembleAnswerDoesNotCompleteReferenceWithoutContract(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "queries.csv"), []byte("query_id\nQ001\nQ002\nQ003\n"), 0600); err != nil {
		t.Fatal(err)
	}
	seed := Result{Contributions: []ContributionRecord{{
		ItemID:        LooseText("row-1"),
		Source:        LooseText("records.csv"),
		SourceLocator: LooseText("row 2"),
		GroupKey:      LooseText("Q002"),
		Metric:        LooseText("amount"),
		Value:         LooseText("7"),
		Operation:     LooseText("add"),
		Role:          LooseText("target"),
	}}}
	plan := TaskPlan{
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{
			Format:             OutputPlainSingleLine,
			ExplanationAllowed: false,
			Delimiter:          ",",
			ReferencePath:      "queries.csv",
			ReferenceKeyField:  "query_id",
		},
		Actions: []DataAction{
			{ID: "reconcile", Kind: DataActionReconcile},
			{ID: "answer", Kind: DataActionAssembleAnswer, Params: map[string]string{"projection": "values", "delimiter": ","}},
		},
	}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "7" {
		t.Fatalf("Answer=%q, want existing groups only without complete_reference", res.Answer)
	}
}

func TestActionRunnerAssembleAnswerOverridesIntermediateSeedAnswer(t *testing.T) {
	root := t.TempDir()
	seed := Result{
		Answer: "1 artifact(s)",
		Contributions: []ContributionRecord{{
			ItemID:        LooseText("row-1"),
			Source:        LooseText("records.csv"),
			SourceLocator: LooseText("row 2"),
			GroupKey:      LooseText("Q001"),
			Metric:        LooseText("amount"),
			Value:         LooseText("12"),
			Operation:     LooseText("add"),
			Role:          LooseText("target"),
		}},
	}
	plan := TaskPlan{
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{
			{ID: "reconcile", Kind: DataActionReconcile},
			{ID: "answer", Kind: DataActionAssembleAnswer, Params: map[string]string{"projection": "values"}},
		},
	}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "12" {
		t.Fatalf("Answer=%q, want assembled answer to override intermediate seed summary", res.Answer)
	}
}

func TestActionRunnerTreatsUnsatisfiedTypedActionBatchAsIntermediate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("name\nalpha\nbeta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		InputPaths: []string{"items.csv"},
		CoverageContract: CoverageContract{
			RequiredMaterials:          []CoverageMaterial{{Path: "items.csv", Required: true}},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:             "normalize",
			Kind:           DataActionNormalizeEntities,
			InputPaths:     []string{"items.csv"},
			OutputArtifact: "normalized.json",
			Params:         map[string]string{"source_field": "name"},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.EntityResolutions) == 0 {
		t.Fatalf("EntityResolutions empty, want intermediate typed progress")
	}
	if len(res.Rows) != 0 {
		t.Fatalf("Rows=%+v, intermediate normalize batch should not synthesize decision rows", res.Rows)
	}
}

func TestActionRunnerReconcileSeedContributionsSatisfyDecisionRows(t *testing.T) {
	seed := Result{Contributions: []ContributionRecord{{
		ItemID:        LooseText("item-1"),
		Source:        LooseText("records.json"),
		SourceLocator: LooseText("json[0]"),
		GroupKey:      LooseText("Q001"),
		Metric:        LooseText("amount"),
		Value:         LooseText("42"),
		Operation:     LooseText("add"),
		Role:          LooseText("target"),
		Reason:        LooseText("included by previous typed action"),
	}}}
	plan := TaskPlan{
		CoverageContract: CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		ContinueAfter:  true,
		Actions: []DataAction{
			{ID: "reconcile", Kind: DataActionReconcile},
			{
				ID:   "answer",
				Kind: DataActionAssembleAnswer,
				Params: map[string]string{
					"projection": "values",
					"order_by":   "group_key",
				},
			},
		},
	}
	res, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0].RowID != "item-1" || res.Rows[0].Decision != "include" {
		t.Fatalf("Rows=%+v, want seed contribution-derived decision row", res.Rows)
	}
	if res.Reconcile == nil || len(res.Reconcile.Groups) != 1 {
		t.Fatalf("Reconcile=%+v, want seed contribution-backed reconcile", res.Reconcile)
	}
}

func TestValidateReconcileAllowsIntermediateArtifactSummaryAnswer(t *testing.T) {
	contract := CoverageContract{
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}
	res := Result{
		Answer: "1 artifact(s)",
		Contributions: []ContributionRecord{{
			ItemID:        LooseText("item-1"),
			Source:        LooseText("records.csv"),
			SourceLocator: LooseText("row 2"),
			GroupKey:      LooseText("Q001"),
			Metric:        LooseText("amount"),
			Value:         LooseText("42"),
			Operation:     LooseText("add"),
			Role:          LooseText("target"),
		}},
		Reconcile: &ReconcileReport{
			Status:         LooseText("pass"),
			ExpectedAnswer: LooseText("Q001/amount=42"),
			ActualAnswer:   LooseText("Q001/amount=42"),
			Groups: []ReconcileGroup{{
				GroupKey:   LooseText("Q001"),
				Metric:     LooseText("amount"),
				Expected:   LooseText("42"),
				Actual:     LooseText("42"),
				Difference: LooseText("0"),
			}},
		},
	}
	if err := ValidateResultAgainstContract(contract, res); err != nil {
		t.Fatalf("ValidateResultAgainstContract: %v", err)
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

func TestActionRunnerNormalizeEntitiesIgnoresMissingFilterField(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "taxonomy.csv"), []byte("code,label,aliases\nA,Alpha,alpha;first\nB,Beta,beta;second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		CoverageContract: CoverageContract{EntityResolutionRequired: true},
		ContinueAfter:    true,
		Actions: []DataAction{{
			ID:         "normalize_ref",
			Kind:       DataActionNormalizeEntities,
			InputPaths: []string{"taxonomy.csv"},
			Params: map[string]string{
				"canonical_id_field":    "code",
				"canonical_label_field": "label",
				"name_fields":           `["label","aliases"]`,
				"filter_field":          "status",
				"filter_value":          "active",
			},
		}},
	}
	res, err := (ActionRunner{RepoRoot: root, TempRoot: t.TempDir()}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("ActionRunner.Run: %v", err)
	}
	if len(res.EntityResolutions) == 0 {
		t.Fatal("expected entity resolutions despite non-existent filter field")
	}
	foundIgnored := false
	for _, artifact := range res.Artifacts {
		for _, child := range artifact.Children {
			if child.Fields["ignored_filter_fields"] == "status" {
				foundIgnored = true
			}
		}
	}
	if !foundIgnored {
		t.Fatalf("ignored_filter_fields=status not recorded in artifacts: %+v", res.Artifacts)
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
