package tool

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeVerificationProbesAcceptsMultiLanguageRuntimes(t *testing.T) {
	probes, rej := normalizeVerificationProbes([]types.VerificationProbe{
		{ID: "py", Language: "python", Code: "assert True\n"},
		{ID: "js", Language: "node", Code: "throw new Error('expected failure when false')"},
		{ID: "rb", Language: "ruby", Code: "raise 'bad' unless true\n"},
		{ID: "go", Language: "golang", Code: "package main\nfunc main() { panic(\"bad\") }\n"},
	})
	if rej != "" {
		t.Fatalf("normalizeVerificationProbes rejected supported runtimes: %s", rej)
	}
	if got, want := len(probes), 4; got != want {
		t.Fatalf("normalized probes = %d, want %d", got, want)
	}
	wantLanguages := []string{"python", "javascript", "ruby", "go"}
	for i, want := range wantLanguages {
		if probes[i].Language != want {
			t.Fatalf("probe[%d].Language = %q, want %q", i, probes[i].Language, want)
		}
	}
}

func TestNormalizeVerificationProbesRejectsPrintOnlyJavaScriptProbe(t *testing.T) {
	_, rej := normalizeVerificationProbes([]types.VerificationProbe{{
		ID:       "print_only",
		Language: "javascript",
		Code:     "if (false) console.log('FAIL');",
	}})
	if rej == "" {
		t.Fatal("expected print-only JavaScript probe to be rejected")
	}
	if !strings.Contains(rej, "javascript executable failure signal") {
		t.Fatalf("rejection should name JavaScript failure-signal requirement, got: %s", rej)
	}
}

func TestRunTestsDryRunJavaScriptVerificationProbe(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available on PATH")
	}
	root := t.TempDir()
	mu := types.NewMutableState("javascript probe")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StagePlan,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run": true,
		"verification_probe": map[string]any{
			"id":              "js_value",
			"language":        "javascript",
			"code":            "const assert = require('assert'); assert.strictEqual(21 * 2, 42); console.log('VALUE=42');",
			"expected_stdout": []string{"VALUE=42"},
		},
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected JavaScript probe to pass, got: %s", result.Summary)
	}
	reports := mu.PlanStageProbeReports()
	if len(reports) != 1 || !reports[0].Passed {
		t.Fatalf("expected one passing planner probe report, got %+v", reports)
	}
	if got := reports[0].ExecutedCommands[0].Framework; got != "javascript" {
		t.Fatalf("framework = %q, want javascript", got)
	}
}

func TestRunTestsDryRunRubyVerificationProbe(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not available on PATH")
	}
	root := t.TempDir()
	mu := types.NewMutableState("ruby probe")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StagePlan,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run": true,
		"verification_probe": map[string]any{
			"id":       "ruby_value",
			"language": "ruby",
			"code":     "raise 'wrong' unless 21 * 2 == 42\nputs 'VALUE=42'\n",
		},
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Ruby probe to pass, got: %s", result.Summary)
	}
}

func TestRunTestsDryRunGoVerificationProbe(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available on PATH")
	}
	root := t.TempDir()
	mu := types.NewMutableState("go probe")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StagePlan,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run": true,
		"verification_probe": map[string]any{
			"id":              "go_value",
			"language":        "go",
			"code":            "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"VALUE=42\") }\n",
			"expected_stdout": []string{"VALUE=42"},
		},
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Go probe to pass, got: %s", result.Summary)
	}
}

func TestRunTestsDryRunGoVerificationProbeCanImportModuleInternalPackage(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available on PATH")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/probe\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "widget"), 0o755); err != nil {
		t.Fatalf("mkdir internal package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "widget", "widget.go"), []byte("package widget\n\nfunc Value() int { return 42 }\n"), 0o644); err != nil {
		t.Fatalf("write widget.go: %v", err)
	}
	mu := types.NewMutableState("go internal probe")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StagePlan,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run": true,
		"verification_probe": map[string]any{
			"id":              "go_internal_value",
			"language":        "go",
			"code":            "package main\n\nimport (\n  \"fmt\"\n  \"example.com/probe/internal/widget\"\n)\n\nfunc main() { fmt.Printf(\"VALUE=%d\\n\", widget.Value()) }\n",
			"expected_stdout": []string{"VALUE=42"},
		},
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Go internal-package probe to pass, got: %s", result.Summary)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".codrax", "tmp", "verification-probes", "*.go")); len(matches) != 0 {
		t.Fatalf("Go probe temp files should be removed, found %v", matches)
	}
}

func TestEmitChangePlanAcceptsJavaScriptVerificationProbe(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a JavaScript behaviour",
		"summary": "Modify widget.js and attach a bounded Node.js behaviour probe.",
		"changes": [
			{"path": "widget.js", "kind": "modify", "new_content": "exports.value = 42;\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "node_value", "language": "javascript", "code": "const assert = require('assert'); const widget = require('./widget'); assert.strictEqual(widget.value, 42);"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected JavaScript probe to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) != 1 {
		t.Fatalf("expected accepted plan with one probe, got: %+v", plan)
	}
	if got := plan.VerificationProbes[0].Language; got != "javascript" {
		t.Fatalf("probe language = %q, want javascript", got)
	}
}
