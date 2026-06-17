package tool

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func TestVerificationProbeSchemaEnumsUseRuntimeRegistry(t *testing.T) {
	want := supportedVerificationProbeLanguages()
	cases := []struct {
		name  string
		raw   json.RawMessage
		paths [][]string
	}{
		{
			name: "emit_change_plan",
			raw:  (&EmitChangePlan{}).Parameters(),
			paths: [][]string{
				{"properties", "verification_probes", "items", "properties", "language", "enum"},
				{"properties", "changes", "items", "properties", "verification_probes", "items", "properties", "language", "enum"},
			},
		},
		{
			name: "emit_plan_skeleton",
			raw:  (&EmitPlanSkeleton{}).Parameters(),
			paths: [][]string{
				{"properties", "verification_probes", "items", "properties", "language", "enum"},
			},
		},
		{
			name: "run_tests",
			raw:  (&RunTests{}).Parameters(),
			paths: [][]string{
				{"properties", "verification_probe", "properties", "language", "enum"},
			},
		},
	}

	for _, tc := range cases {
		var schema map[string]any
		if err := json.Unmarshal(tc.raw, &schema); err != nil {
			t.Fatalf("%s schema must be valid JSON: %v\n%s", tc.name, err, string(tc.raw))
		}
		if !strings.Contains(string(tc.raw), supportedVerificationProbeRuntimeDescription()) {
			t.Fatalf("%s schema should render registry description %q", tc.name, supportedVerificationProbeRuntimeDescription())
		}
		for _, path := range tc.paths {
			got := schemaStringArrayAt(t, schema, path)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s enum at %s = %v, want %v", tc.name, strings.Join(path, "."), got, want)
			}
		}
	}
}

func schemaStringArrayAt(t *testing.T, schema map[string]any, path []string) []string {
	t.Helper()
	var cur any = schema
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("schema path %s reached non-object %T", strings.Join(path, "."), cur)
		}
		cur, ok = obj[key]
		if !ok {
			t.Fatalf("schema path %s missing key %q", strings.Join(path, "."), key)
		}
	}
	values, ok := cur.([]any)
	if !ok {
		t.Fatalf("schema path %s reached non-array %T", strings.Join(path, "."), cur)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		s, ok := value.(string)
		if !ok {
			t.Fatalf("schema path %s has non-string enum value %T", strings.Join(path, "."), value)
		}
		out = append(out, s)
	}
	return out
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

func TestEmitChangePlanRejectsJavaScriptProbeThatCopiesImplementation(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a JavaScript behaviour",
		"summary": "Modify widget.js and attach a probe that incorrectly checks a copied local value instead of importing the changed module.",
		"changes": [
			{"path": "widget.js", "kind": "modify", "new_content": "exports.value = 42;\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "copied_value", "language": "javascript", "code": "const assert = require('assert'); const value = 42; assert.strictEqual(value, 42);"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected copied JavaScript probe to be rejected, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "changed JavaScript/TypeScript production module") {
		t.Fatalf("summary should explain JS/TS changed-module coupling, got: %s", res.Summary)
	}
}

func TestEmitChangePlanAcceptsJavaScriptProbeImportingPackageName(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "package.json"), []byte(`{"name":"@codrax/widgets"}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	params := json.RawMessage(`{
		"request": "fix a TypeScript public package behaviour",
		"summary": "Modify src/widget.ts and verify through the package public entrypoint declared in package.json.",
		"changes": [
			{"path": "src/widget.ts", "kind": "modify", "new_content": "export const value = 42;\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "package_value", "language": "javascript", "code": "const assert = require('assert'); const widget = require('@codrax/widgets'); assert.strictEqual(widget.value, 42);"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected package-entrypoint JavaScript probe to be accepted, got: %s", res.Summary)
	}
}

func TestEmitChangePlanRejectsRubyProbeThatCopiesImplementation(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a Ruby behaviour",
		"summary": "Modify lib/widget.rb and attach a probe that incorrectly checks a copied local constant.",
		"changes": [
			{"path": "lib/widget.rb", "kind": "modify", "new_content": "module Widget\n  VALUE = 42\nend\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "copied_value", "language": "ruby", "code": "value = 42\nraise 'wrong' unless value == 42\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected copied Ruby probe to be rejected, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "Ruby import/require declarations") {
		t.Fatalf("summary should explain Ruby changed-module coupling, got: %s", res.Summary)
	}
}

func TestEmitChangePlanAcceptsRubyProbeRequiringChangedModule(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a Ruby behaviour",
		"summary": "Modify lib/widget.rb and verify through require of the changed library module.",
		"changes": [
			{"path": "lib/widget.rb", "kind": "modify", "new_content": "module Widget\n  VALUE = 42\nend\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "required_value", "language": "ruby", "code": "require 'widget'\nraise 'wrong' unless Widget::VALUE == 42\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected changed-module Ruby probe to be accepted, got: %s", res.Summary)
	}
}

func TestValidateVerificationProbeCouplingForGoModuleImports(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/probe\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	ctx := &types.BusContext{RepoRoot: root}
	changes := []types.FileChange{{
		Path:       "internal/widget/widget.go",
		Kind:       "modify",
		NewContent: "package widget\n\nfunc Value() int { return 42 }\n",
	}}
	copiedProbe := []types.VerificationProbe{{
		ID:       "copied_value",
		Language: "go",
		Code:     "package main\n\nimport \"fmt\"\n\nfunc main() { if 42 != 42 { panic(fmt.Sprint(\"wrong\")) } }\n",
	}}
	if rej := validateVerificationProbeCoupling(ctx, changes, copiedProbe); !strings.Contains(rej, "changed Go production module") {
		t.Fatalf("expected copied Go probe rejection, got: %q", rej)
	}
	importingProbe := []types.VerificationProbe{{
		ID:       "module_value",
		Language: "go",
		Code:     "package main\n\nimport \"example.com/probe/internal/widget\"\n\nfunc main() { if widget.Value() != 42 { panic(\"wrong\") } }\n",
	}}
	if rej := validateVerificationProbeCoupling(ctx, changes, importingProbe); rej != "" {
		t.Fatalf("expected Go probe importing changed package to pass, got: %s", rej)
	}
}
