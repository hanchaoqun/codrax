package tool

import (
	"path/filepath"
	"testing"
)

func TestRunTestsRefinementPreservesScopedVerifierParams(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	plan := runnerPlan{
		Runner:    "python",
		Framework: "pytest",
		Root:      filepath.Join(repoRoot, "pkg"),
		Suite:     "tests/test_widget.py::test_handles_edge",
	}
	hint := runTestsRefinement(runTestsParams{TimeoutSeconds: 30}, &plan, repoRoot, "run_tests_timeout", MaxInlineBytes+1)
	if hint == nil {
		t.Fatal("expected refinement")
	}
	if hint.ReasonCode != "run_tests_timeout" || !hint.ResultTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	wantParams := map[string]string{
		"runner":          "python",
		"framework":       "pytest",
		"working_dir":     "pkg",
		"suite":           "tests/test_widget.py::test_handles_edge",
		"timeout_seconds": "30",
	}
	for key, want := range wantParams {
		if got := hint.PreferredParams[key]; got != want {
			t.Fatalf("preferred param %s = %q, want %q; hint=%+v", key, got, want, hint)
		}
	}
	if len(hint.RequiredFields) != 0 {
		t.Fatalf("scoped verifier params should not ask for extra fields: %+v", hint)
	}
}

func TestRunTestsRefinementAsksForSuiteOnlyWhenScopeIsBroad(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	plan := runnerPlan{
		Runner: "go",
		Root:   repoRoot,
	}
	hint := runTestsRefinement(runTestsParams{}, &plan, repoRoot, "run_tests_cpu_limit", 128)
	if hint == nil {
		t.Fatal("expected refinement")
	}
	if hint.ResultTruncated {
		t.Fatalf("resource exhaustion should not claim truncation for small output: %+v", hint)
	}
	if !sameStringSet(hint.RequiredFields, []string{"suite"}) {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
	if got := hint.PreferredParams["runner"]; got != "go" {
		t.Fatalf("runner param = %q; hint=%+v", got, hint)
	}
}

func TestRunTestsRefinementRunnerMissingRequiresTypedEscape(t *testing.T) {
	hint := runTestsRefinement(runTestsParams{}, nil, "", "run_tests_runner_missing", 0)
	if hint == nil {
		t.Fatal("expected refinement")
	}
	if hint.ReasonCode != "run_tests_runner_missing" {
		t.Fatalf("reason = %q; hint=%+v", hint.ReasonCode, hint)
	}
	if !sameStringSet(hint.RequiredFields, []string{"runner", "verification_probe"}) {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
	if len(hint.PreferredParams) != 0 {
		t.Fatalf("runner-missing without typed params must not invent preferred params: %+v", hint)
	}
}

func TestRunTestsRefinementLargeAggregateOutput(t *testing.T) {
	hint := runTestsRefinement(runTestsParams{Runner: "go", WorkingDir: "pkg"}, nil, "", "", MaxInlineBytes+1)
	if hint == nil {
		t.Fatal("expected refinement")
	}
	if hint.ReasonCode != "run_tests_output_truncated" || !hint.ResultTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if got := hint.PreferredParams["working_dir"]; got != "pkg" {
		t.Fatalf("working_dir = %q; hint=%+v", got, hint)
	}
	if !sameStringSet(hint.RequiredFields, []string{"suite"}) {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
}

func TestRunTestsRefinementSmallOutputStaysSilent(t *testing.T) {
	if hint := runTestsRefinement(runTestsParams{Runner: "go"}, nil, "", "", MaxInlineBytes); hint != nil {
		t.Fatalf("small output without typed reason should not emit refinement: %+v", hint)
	}
}
