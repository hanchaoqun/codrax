package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerificationProbeTargetLanguageCompatibilityRejectsUnsupportedChangedFamilies(t *testing.T) {
	for _, path := range []string{
		"src/main.c",
		"src/main.cpp",
		"src/lib.rs",
		"entry/src/main/ets/Main.ets",
		"src/service.cj",
	} {
		t.Run(path, func(t *testing.T) {
			changes := []types.FileChange{{Path: path, Kind: "modify", NewContent: "changed"}}
			probes := []types.VerificationProbe{{ID: "native-wrapper", Language: "python", Code: "raise SystemExit(1)"}}
			got := validateVerificationProbeTargetLanguageCompatibility(changes, probes)
			if !strings.Contains(got, `language="python" cannot directly execute any changed source target`) ||
				!strings.Contains(got, path) || !strings.Contains(got, "not command wrappers") {
				t.Fatalf("unsupported changed family should reject without inspecting probe prose: %q", got)
			}
		})
	}
}

func TestVerificationProbeTargetLanguageCompatibilityAcceptsDirectAndMixedTargets(t *testing.T) {
	tests := []struct {
		name     string
		changes  []types.FileChange
		language string
	}{
		{name: "python", changes: []types.FileChange{{Path: "pkg/widget.py"}}, language: "python"},
		{name: "javascript", changes: []types.FileChange{{Path: "src/widget.js"}}, language: "javascript"},
		{name: "typescript-via-existing-js-provider", changes: []types.FileChange{{Path: "src/widget.ts"}}, language: "javascript"},
		{name: "ruby", changes: []types.FileChange{{Path: "lib/widget.rb"}}, language: "ruby"},
		{name: "java", changes: []types.FileChange{{Path: "src/Widget.java"}}, language: "java"},
		{name: "go", changes: []types.FileChange{{Path: "pkg/widget.go"}}, language: "go"},
		{name: "mixed-python-and-cpp", changes: []types.FileChange{{Path: "pkg/widget.py"}, {Path: "native/widget.cpp"}}, language: "python"},
		{name: "config-only-fails-open", changes: []types.FileChange{{Path: "config/settings.yaml"}}, language: "python"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probes := []types.VerificationProbe{{ID: "direct", Language: tc.language, Code: "assert true"}}
			if got := validateVerificationProbeTargetLanguageCompatibility(tc.changes, probes); got != "" {
				t.Fatalf("compatible or non-source plan rejected: %s", got)
			}
		})
	}
}

func TestEmitChangePlanRejectsPythonCompilerWrapperForCppOnlyChange(t *testing.T) {
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request":"fix the C++ source",
		"summary":"Modify main.cpp and leave native compilation to project verification.",
		"changes":[{
			"path":"main.cpp",
			"kind":"modify",
			"new_content":"int main() { return 0; }\n",
			"rationale":"repair the C++ source"
		}],
		"acceptance_tests":["g++ main.cpp succeeds"],
		"verification_probes":[{
			"id":"compile-wrapper",
			"language":"python",
			"code":"import subprocess\nresult = subprocess.run(['g++', 'main.cpp'])\nraise SystemExit(result.returncode)"
		}]
	}`)

	res, err := (&EmitChangePlan{}).Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("runtime-enum command wrapper must be rejected: %+v", ctx.Mutable.ChangePlan())
	}
	for _, want := range []string{
		"verification_probes[0].language=\"python\" cannot directly execute any changed source target main.cpp",
		"not command wrappers",
		"acceptance_tests",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("typed repair guidance missing %q: %s", want, res.Summary)
		}
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected wrapper must not install a plan: %+v", plan)
	}
}
