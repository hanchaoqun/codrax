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

// TestHasPythonTestInfrastructure_DetectsConfigFiles verifies the
// six configuration-file signals trigger detection at root level.
// Customer-reported scenario hinges on this returning false for a
// repo containing only a non-test .py source.
func TestHasPythonTestInfrastructure_DetectsConfigFiles(t *testing.T) {
	for _, name := range []string{
		"pytest.ini", "pyproject.toml", "setup.cfg",
		"tox.ini", "noxfile.py", "conftest.py",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, name), []byte("# stub"), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
			if !hasPythonTestInfrastructure(dir) {
				t.Errorf("%s should trigger test-infrastructure detection", name)
			}
		})
	}
}

// TestHasPythonTestInfrastructure_DetectsTestSourceFiles verifies
// the three test-source patterns trigger detection.
func TestHasPythonTestInfrastructure_DetectsTestSourceFiles(t *testing.T) {
	for _, name := range []string{
		"test_widget.py",
		"widget_test.py",
		"conftest.py",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			sub := filepath.Join(dir, "src")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(sub, name), []byte("# stub"), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
			if !hasPythonTestInfrastructure(dir) {
				t.Errorf("%s under src/ should trigger detection", name)
			}
		})
	}
}

// TestHasPythonTestInfrastructure_NegativeCases verifies that
// non-test .py files alone do NOT trigger detection — that's the
// customer scenario that must produce a false return so the
// py_compile fallback fires instead of forcing pytest.
func TestHasPythonTestInfrastructure_NegativeCases(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		if hasPythonTestInfrastructure(dir) {
			t.Error("empty dir should NOT have test infrastructure")
		}
	})
	t.Run("single non-test py file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "guess_number.py"), []byte("print(1)\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if hasPythonTestInfrastructure(dir) {
			t.Error("guess_number.py alone should NOT trigger test-infrastructure detection")
		}
	})
	t.Run("ignored dirs do not flag", func(t *testing.T) {
		dir := t.TempDir()
		// pytest.ini buried inside .venv (a recognised venv layout)
		// must NOT flag the parent — venv dirs are skipped.
		buried := filepath.Join(dir, ".venv", "lib", "python3.11", "site-packages", "somepkg")
		if err := os.MkdirAll(buried, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buried, "pytest.ini"), []byte("[pytest]\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if hasPythonTestInfrastructure(dir) {
			t.Error("pytest.ini buried under .venv/ should not flag (skipped)")
		}
	})
}

// TestRunnerHasNoTestWork_PythonVsOthers locks the dispatch:
// python uses hasPythonTestInfrastructure, other runners fall to
// hasNoTestSources. A python repo with only a pyproject.toml and
// no test files should still report tests_exist=true (per fix B
// design); a go repo with only main.go should report tests_exist=false.
func TestRunnerHasNoTestWork_PythonVsOthers(t *testing.T) {
	t.Run("python with pyproject.toml only", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644)
		if runnerHasNoTestWork("python", dir) {
			t.Error("pyproject.toml present should mean test work exists")
		}
	})
	t.Run("python with only non-test py", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "main.py"), []byte("print(1)\n"), 0o644)
		if !runnerHasNoTestWork("python", dir) {
			t.Error("non-test py only should mean no test work")
		}
	})
	t.Run("go with only main.go", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
		if !runnerHasNoTestWork("go", dir) {
			t.Error("go without _test.go should mean no test work")
		}
	})
	t.Run("go with _test.go", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o644)
		if runnerHasNoTestWork("go", dir) {
			t.Error("_test.go should mean test work exists")
		}
	})
}

// TestPlanFilesByExt_FiltersAndResolves verifies extension filtering
// + worktree-boundary enforcement. Files outside root are dropped
// (defense-in-depth even though plan validation also blocks them).
func TestPlanFilesByExt_FiltersAndResolves(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"a.py", "b.py", "c.txt", "sub/d.py"} {
		full := filepath.Join(root, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte("x"), 0o644)
	}
	plan := &types.ChangePlan{TargetPaths: []string{"a.py", "b.py", "c.txt", "sub/d.py", "../escape.py"}}
	mu := types.NewMutableState("test")
	mu.SetChangePlan(plan)
	ctx := &types.BusContext{Mutable: mu}
	got := planFilesByExt(ctx, root, []string{".py"})
	if len(got) != 3 {
		t.Errorf("expected 3 .py files (escape filtered), got %d: %v", len(got), got)
	}
	for _, p := range got {
		if !strings.HasSuffix(p, ".py") {
			t.Errorf("non-py file leaked: %q", p)
		}
		if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
			t.Errorf("escaped-root path leaked: %q", p)
		}
	}
}

// TestRunPyCompileFallback_PassWhenAllFilesParse drives the python
// fallback end-to-end with a file that py_compile accepts. Skipped
// when no usable Python dry-build runner is available on the test
// host (for example a PATH shim that resolves via LookPath but
// cannot actually execute `-m py_compile`).
func TestRunPyCompileFallback_PassWhenAllFilesParse(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	good := filepath.Join(root, "guess_number.py")
	if err := os.WriteFile(good, []byte("def main():\n    print('hello')\n\nmain()\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	plan := &types.ChangePlan{TargetPaths: []string{"guess_number.py"}}
	mu := types.NewMutableState("test")
	mu.SetChangePlan(plan)
	ctx := &types.BusContext{Mutable: mu, MainRepoRoot: root}
	report, output := runPyCompileFallback(ctx, "python@.", root, []string{good})
	if !report.Passed {
		t.Errorf("clean file should pass py_compile fallback; got: %+v", report)
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "python" {
		t.Errorf("Passed report should mark NoTestsRunners=[python]; got %v", report.NoTestsRunners)
	}
	if !strings.Contains(output, "ok    guess_number.py") {
		t.Errorf("output should record per-file ok: %q", output)
	}
}

func TestRunTestsEmptyParamsUsesPlanTouchedSyntaxFallback(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "guess_number.py"), []byte("def main():\n    print('hello')\n\nmain()\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mu := types.NewMutableState("empty params syntax fallback")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-empty-params-syntax",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"guess_number.py"},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("empty run_tests params should pass via plan-touched syntax fallback, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "python" {
		t.Fatalf("report should preserve no-tests runner evidence after syntax fallback: %+v", report)
	}
	if got := report.NormalizeVerificationStatus(); got != types.VerificationStatusUnavailable {
		t.Fatalf("syntax fallback without real tests should remain unavailable confidence, got %q report=%+v", got, report)
	}
	foundDefaultFallback := false
	foundCompileConfidence := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "python" && cmd.Outcome == "syntax_check_fallback" && cmd.Source == "test_surface_default" {
			foundDefaultFallback = true
		}
	}
	if !foundDefaultFallback {
		t.Fatalf("executed command evidence should record system default syntax fallback, got %+v", report.ExecutedCommands)
	}
	for _, confidence := range report.VerificationConfidence {
		if confidence.Category == "source_compile" && confidence.Status == "satisfied" && confidence.ReasonCode == "source_compile_ok" {
			foundCompileConfidence = true
		}
	}
	if !foundCompileConfidence {
		t.Fatalf("verification confidence should record syntax fallback success, got %+v", report.VerificationConfidence)
	}
}

// TestRunPyCompileFallback_FailOnSyntaxError verifies broken syntax
// produces a build_error TestResult with the file as AssertionID
// and a non-empty FailureDetail. Skipped when no usable Python
// dry-build runner is available.
func TestRunPyCompileFallback_FailOnSyntaxError(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	bad := filepath.Join(root, "bad.py")
	if err := os.WriteFile(bad, []byte("def broken(:\n    pass\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	plan := &types.ChangePlan{TargetPaths: []string{"bad.py"}}
	mu := types.NewMutableState("test")
	mu.SetChangePlan(plan)
	ctx := &types.BusContext{Mutable: mu, MainRepoRoot: root}
	report, _ := runPyCompileFallback(ctx, "python@.", root, []string{bad})
	if report.Passed {
		t.Error("syntax-broken file should NOT pass py_compile fallback")
	}
	if !report.BuildFailed {
		t.Error("syntax-broken file should set BuildFailed=true")
	}
	if report.FailureKind != types.FailureKindBuildFailure {
		t.Errorf("FailureKind = %q, want build_failure", report.FailureKind)
	}
	if len(report.TestResults) != 1 {
		t.Fatalf("expected 1 build_error TestResult; got %d", len(report.TestResults))
	}
	tr := report.TestResults[0]
	if tr.Suite != "py_compile" {
		t.Errorf("Suite = %q, want py_compile", tr.Suite)
	}
	if tr.AssertionID != "bad.py" {
		t.Errorf("AssertionID = %q, want bad.py", tr.AssertionID)
	}
	if tr.FailureDetail == "" {
		t.Error("FailureDetail should embed py_compile stderr")
	}
}

func TestRunPyCompileFallback_FailsOnUndefinedFunctionName(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	bad := filepath.Join(root, "bad.py")
	if err := os.WriteFile(bad, []byte("def add_url_rule(endpoint=None):\n    if endpoint:\n        pass\n    if name:\n        pass\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mu := types.NewMutableState("undefined name")
	mu.SetChangePlan(&types.ChangePlan{TargetPaths: []string{"bad.py"}})
	ctx := &types.BusContext{Mutable: mu, MainRepoRoot: root}
	report, output := runPyCompileFallback(ctx, "python@.", root, []string{bad})
	if report.Passed {
		t.Fatalf("undefined name should fail static preflight: report=%+v output=%s", report, output)
	}
	if report.FailureReasonCode != "python_static_undefined_name" {
		t.Fatalf("FailureReasonCode = %q, want python_static_undefined_name; report=%+v", report.FailureReasonCode, report)
	}
	if len(report.TestResults) != 1 || report.TestResults[0].Suite != "python_static_name_check" {
		t.Fatalf("static name check failure row missing: %+v", report.TestResults)
	}
	if !strings.Contains(report.TestResults[0].FailureDetail, "undefined global name 'name'") {
		t.Fatalf("failure detail should name undefined symbol, got %q", report.TestResults[0].FailureDetail)
	}
}

func TestRunPyCompileFallback_IgnoresPreexistingUndefinedOutsideChangedLines(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	file := filepath.Join(root, "bad.py")
	src := "def old():\n    return preexisting\n\ndef touched():\n    return 2\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mu := types.NewMutableState("preexisting undefined")
	mu.SetChangePlan(&types.ChangePlan{
		TargetPaths: []string{"bad.py"},
		Changes: []types.FileChange{{
			Path:  "bad.py",
			Kind:  "patch",
			Patch: "--- a/bad.py\n+++ b/bad.py\n@@ -3,3 +3,3 @@\n \n def touched():\n-    return 1\n+    return 2\n",
		}},
	})
	ctx := &types.BusContext{Mutable: mu, MainRepoRoot: root}
	report, output := runPyCompileFallback(ctx, "python@.", root, []string{file})
	if !report.Passed {
		t.Fatalf("pre-existing undefined outside changed lines should not hard-fail: report=%+v output=%s", report, output)
	}
}

func TestRunPyCompileFallback_FailsOnNewUndefinedInsideChangedLines(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	file := filepath.Join(root, "bad.py")
	src := "def ok():\n    return 1\n\ndef touched():\n    return missing_value\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mu := types.NewMutableState("new undefined")
	mu.SetChangePlan(&types.ChangePlan{
		TargetPaths: []string{"bad.py"},
		Changes: []types.FileChange{{
			Path:  "bad.py",
			Kind:  "patch",
			Patch: "--- a/bad.py\n+++ b/bad.py\n@@ -3,3 +3,3 @@\n \n def touched():\n-    return 1\n+    return missing_value\n",
		}},
	})
	ctx := &types.BusContext{Mutable: mu, MainRepoRoot: root}
	report, output := runPyCompileFallback(ctx, "python@.", root, []string{file})
	if report.Passed {
		t.Fatalf("new undefined inside changed lines should fail: report=%+v output=%s", report, output)
	}
	if report.FailureReasonCode != "python_static_undefined_name" {
		t.Fatalf("FailureReasonCode=%q", report.FailureReasonCode)
	}
	if len(report.TestResults) != 1 || len(report.TestResults[0].BuildErrors) != 1 {
		t.Fatalf("expected structured build error for changed undefined name: %+v", report.TestResults)
	}
	if report.TestResults[0].BuildErrors[0].Line != 5 {
		t.Fatalf("build error line=%d want 5", report.TestResults[0].BuildErrors[0].Line)
	}
}

func TestRunPyCompileFallback_AllowsDefinedGlobalsAndImports(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	good := filepath.Join(root, "good.py")
	if err := os.WriteFile(good, []byte("import os\nVALUE = 1\n\ndef ok(path):\n    return os.path.basename(path), VALUE, len(path)\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mu := types.NewMutableState("defined names")
	mu.SetChangePlan(&types.ChangePlan{TargetPaths: []string{"good.py"}})
	ctx := &types.BusContext{Mutable: mu, MainRepoRoot: root}
	report, output := runPyCompileFallback(ctx, "python@.", root, []string{good})
	if !report.Passed {
		t.Fatalf("defined names should pass static preflight: report=%+v output=%s", report, output)
	}
}

func TestRunTestsPythonSyntaxPreflightFailsBeforeProjectRunner(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[tool.pytest.ini_options]\n"), 0o644); err != nil {
		t.Fatalf("write pyproject: %v", err)
	}
	bad := filepath.Join(root, "pkg", "bad.py")
	if err := os.MkdirAll(filepath.Dir(bad), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(bad, []byte("def broken(:\n    pass\n"), 0o644); err != nil {
		t.Fatalf("write bad.py: %v", err)
	}
	mu := types.NewMutableState("syntax preflight")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-syntax-preflight",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"pkg/bad.py"},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner":    "python",
		"framework": "pytest",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("syntax-broken changed Python source must fail before pytest, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if got := report.NormalizeVerificationStatus(); got != types.VerificationStatusFailed {
		t.Fatalf("VerificationStatus = %q, want failed; report=%+v", got, report)
	}
	if report.FailureKind != types.FailureKindBuildFailure || !report.BuildFailed {
		t.Fatalf("syntax preflight should be build_failure, got kind=%q build_failed=%v report=%+v",
			report.FailureKind, report.BuildFailed, report)
	}
	if len(report.TestResults) != 1 || report.TestResults[0].Suite != "py_compile" || report.TestResults[0].AssertionID != "pkg/bad.py" {
		t.Fatalf("py_compile failure row missing: %+v", report.TestResults)
	}
	foundPreflight := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "python" && cmd.Framework == "pytest" && cmd.Outcome == "syntax_preflight" {
			foundPreflight = true
		}
		if cmd.Runner == "python" && cmd.Framework == "pytest" && cmd.Outcome == "executed" {
			t.Fatalf("project pytest should not execute after syntax preflight failure: %+v", report.ExecutedCommands)
		}
	}
	if !foundPreflight {
		t.Fatalf("executed command evidence should record syntax_preflight, got %+v", report.ExecutedCommands)
	}
}

func TestRunTestsDryRunSuiteProbeRejectedInWritePlan(t *testing.T) {
	root := t.TempDir()
	writeMakefile(t, root, "test:\n\t@echo failing probe\n\t@exit 1\n")
	mu := types.NewMutableState("probe channel")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StagePlan,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner":  "make",
		"dry_run": true,
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success || result.Repair == nil || result.Repair.Code != "write_planner_run_tests_requires_verification_probe" {
		t.Fatalf("suite dry-run should be rejected before test execution, got %+v", result)
	}
	if got := mu.ChangeReport(); got != nil {
		t.Fatalf("rejected plan-stage suite dry-run must not populate ChangeReport, got %+v", got)
	}
	if probes := mu.PlanStageProbeReports(); len(probes) != 0 {
		t.Fatalf("rejected plan-stage suite dry-run must not populate planner probes, got %+v", probes)
	}
}

func TestRunTestsDryRunVerificationProbe_ModeApplyStagePlan(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 41\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	mu := types.NewMutableState("planner verification probe")
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
			"id":       "value_contract",
			"language": "python",
			"code":     "import widget\nassert widget.VALUE == 42\n",
		},
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("failing verification probe should return Success=false, got %+v", result)
	}
	if !strings.Contains(result.Summary, "verification_probes verdict=FAILED") {
		t.Fatalf("summary should render verification probe verdict, got %q", result.Summary)
	}
	if got := mu.ChangeReport(); got != nil {
		t.Fatalf("plan-stage verification_probe dry_run must not populate ChangeReport, got %+v", got)
	}
	probes := mu.PlanStageProbeReports()
	if len(probes) != 1 {
		t.Fatalf("expected one planner probe report, got %d", len(probes))
	}
	if probes[0].Channel != types.ChangeReportChannelPlannerProbe {
		t.Fatalf("probe channel = %q, want planner_probe", probes[0].Channel)
	}
	if probes[0].FailureKind != types.FailureKindTestsFailed {
		t.Fatalf("probe failure kind = %q, want tests_failed; report=%+v", probes[0].FailureKind, probes[0])
	}
}

func TestRunTestsDryRunVerificationProbe_PassingOutputIsInline(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	mu := types.NewMutableState("planner verification probe output")
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
			"id":       "facts",
			"language": "python",
			"code":     "print('probe_fact: pandas_unique_accepts_list=false')\n",
		},
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("passing verification probe should return Success=true, got %+v", result)
	}
	if !strings.Contains(result.Summary, "Probe output:") || !strings.Contains(result.Summary, "probe_fact: pandas_unique_accepts_list=false") {
		t.Fatalf("passing planner probe output should be inline for handoff, got %q", result.Summary)
	}
	if got := mu.ChangeReport(); got != nil {
		t.Fatalf("plan-stage verification_probe dry_run must not populate ChangeReport, got %+v", got)
	}
	if probes := mu.PlanStageProbeReports(); len(probes) != 1 || !probes[0].Passed {
		t.Fatalf("expected one passing planner probe report, got %+v", probes)
	}
}

func TestRunTestsRejectsSuiteWithEmbeddedCLIFlags(t *testing.T) {
	mu := types.NewMutableState("suite flag rejection")
	ctx := &types.BusContext{Mutable: mu}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
		"suite":  "tests/test_sample.py::test_thing -v",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("suite with embedded flag must be rejected, got %+v", result)
	}
	if !strings.Contains(result.Summary, "contains CLI option token") || !strings.Contains(result.Summary, "-v") {
		t.Fatalf("rejection should name the typed suite flag problem, got %q", result.Summary)
	}
	if got := mu.ChangeReport(); got != nil {
		t.Fatalf("parameter rejection must not install verify ChangeReport, got %+v", got)
	}
}

func TestValidateRunTestsSuiteSelector_AllowsSpacedTestName(t *testing.T) {
	if got := validateRunTestsSuiteSelector("renders empty state"); got != "" {
		t.Fatalf("spaced test-name selectors should remain valid; got %q", got)
	}
}

func TestRunTestsDryRunFlagIgnoredOutsideStagePlan(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available on PATH")
	}
	root := t.TempDir()
	writeMakefile(t, root, "test:\n\t@echo verified\n")
	mu := types.NewMutableState("verify channel")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner":  "make",
		"dry_run": true,
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("verify run should pass, got %+v", result)
	}
	if got := mu.ChangeReport(); got == nil {
		t.Fatal("verify-stage run_tests must populate authoritative ChangeReport")
	} else if got.Channel != types.ChangeReportChannelPostApplyVerify {
		t.Fatalf("verify report channel = %q, want post_apply_verify", got.Channel)
	}
	if probes := mu.PlanStageProbeReports(); len(probes) != 0 {
		t.Fatalf("verify-stage dry_run flag must not append planner probes, got %+v", probes)
	}
}

func TestRunTestsNoTestWorkUsesVerificationProbeVerdict(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 41\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	mu := types.NewMutableState("probe verdict")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-probe",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py"},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "value_contract",
			Language: "python",
			Code:     "import widget\nassert widget.VALUE == 42\n",
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("failing verification_probe must fail run_tests, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.FailureKind != types.FailureKindTestsFailed {
		t.Fatalf("FailureKind = %q, want tests_failed; report=%+v", report.FailureKind, report)
	}
	if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "value_contract" || report.TestResults[0].Passed {
		t.Fatalf("verification probe result missing or wrong: %+v", report.TestResults)
	}
	foundProbeCommand := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "verification_probe" && cmd.Framework == "python" && cmd.Source == "pre_suite_verification_probe" {
			foundProbeCommand = true
		}
	}
	if !foundProbeCommand {
		t.Fatalf("executed command evidence should include verification_probe source, got %+v", report.ExecutedCommands)
	}
}

func TestRunTestsVerificationProbePassSkipsProjectSuiteWhenProbeComplete(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatalf("mkdir tests: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "__init__.py"), []byte(""), 0o644); err != nil {
		t.Fatalf("write tests package marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	testBody := "import unittest\n\nclass ProjectSuite(unittest.TestCase):\n    def test_project_suite_would_fail(self):\n        self.assertTrue(False)\n"
	if err := os.WriteFile(filepath.Join(root, "tests", "test_project_suite.py"), []byte(testBody), 0o644); err != nil {
		t.Fatalf("write test: %v", err)
	}
	mu := types.NewMutableState("probe primary")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-probe-pass",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py"},
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "outcome-1",
			Kind:     types.WriteBehaviorObservable,
			Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpEquals,
			Expected: "42",
			Required: true,
			Source:   "write_analyzer",
		}},
		VerificationProbes: []types.VerificationProbe{{
			ID:                "value_contract",
			Language:          "python",
			Code:              "import widget\nassert widget.VALUE == 42\n",
			ContractRefs:      []string{"outcome-1"},
			ChangedSymbolRefs: []string{"widget.VALUE"},
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner":    "python",
		"framework": "unittest",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("passing verification_probe should pass without project-suite hard gate, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		t.Fatalf("VerificationStatus = %q, want passed; report=%+v", report.NormalizeVerificationStatus(), report)
	}
	if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "value_contract" || !report.TestResults[0].Passed {
		t.Fatalf("verification probe result missing or wrong: %+v", report.TestResults)
	}
	foundProbeCommand := false
	foundSkippedSuite := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "verification_probe" && cmd.Source == "pre_suite_verification_probe" && cmd.Outcome == "executed" {
			foundProbeCommand = true
		}
		if cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Source == "probe_primary_suite_skipped" && cmd.Outcome == "suite_skipped" {
			foundSkippedSuite = true
		}
		if cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Source == "llm_choice" && cmd.Outcome == "executed" {
			t.Fatalf("project unittest suite should not execute after passing bounded probe: %+v", report.ExecutedCommands)
		}
	}
	if !foundProbeCommand {
		t.Fatalf("executed command evidence should include pre-suite verification_probe, got %+v", report.ExecutedCommands)
	}
	if !foundSkippedSuite {
		t.Fatalf("executed command evidence should record skipped project suite, got %+v", report.ExecutedCommands)
	}
	if report.TestSurface == nil || report.TestSurface.SelectedID == "" {
		t.Fatalf("probe-primary report must retain test surface, got %+v", report.TestSurface)
	}
	if changeReportHasVerificationConfidence(report, "probe_contract_refs", "missing", "verification_probe_missing_required_contract_ref") {
		t.Fatalf("complete probe should not carry missing contract-ref downgrade: %+v", report.VerificationConfidence)
	}
	if !changeReportHasVerificationConfidence(report, "probe_contract_refs", "satisfied", "verification_probe_contract_ref_covered") {
		t.Fatalf("complete probe should carry covered contract-ref evidence: %+v", report.VerificationConfidence)
	}
	if !strings.Contains(result.Summary, "verification_probes verdict=PASSED") {
		t.Fatalf("summary should explain probe-primary verdict, got %q", result.Summary)
	}
}

func TestRunTestsVerificationProbePassSkipsProjectSuiteWhenOnlyContractRefMissing(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatalf("mkdir tests: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "__init__.py"), []byte(""), 0o644); err != nil {
		t.Fatalf("write tests package marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	testBody := "import unittest\n\nclass ProjectSuite(unittest.TestCase):\n    def test_project_suite_would_fail(self):\n        self.assertTrue(False)\n"
	if err := os.WriteFile(filepath.Join(root, "tests", "test_project_suite.py"), []byte(testBody), 0o644); err != nil {
		t.Fatalf("write test: %v", err)
	}
	mu := types.NewMutableState("probe missing contract ref")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-probe-missing-contract-ref",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py"},
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "outcome-1",
			Kind:     types.WriteBehaviorObservable,
			Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpEquals,
			Expected: "42",
			Required: true,
			Source:   "write_analyzer",
		}},
		VerificationProbes: []types.VerificationProbe{{
			ID:                "value_contract",
			Language:          "python",
			Code:              "import widget\nassert widget.VALUE == 42\n",
			ChangedSymbolRefs: []string{"widget.VALUE"},
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner":    "python",
		"framework": "unittest",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("passing verification_probe should pass without project-suite hard gate, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	foundSkippedSuite := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Source == "probe_primary_suite_skipped" && cmd.Outcome == "suite_skipped" {
			foundSkippedSuite = true
		}
		if cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Source == "llm_choice" && cmd.Outcome == "executed" {
			t.Fatalf("missing contract refs alone must not execute project suite after passing bounded probe: %+v", report.ExecutedCommands)
		}
	}
	if !foundSkippedSuite {
		t.Fatalf("executed command evidence should record skipped project suite, got %+v", report.ExecutedCommands)
	}
	if !changeReportHasVerificationConfidence(report, "probe_contract_refs", "missing", "verification_probe_missing_required_contract_ref") {
		t.Fatalf("missing contract-ref confidence downgrade should be retained: %+v", report.VerificationConfidence)
	}
	if !strings.Contains(result.Summary, "verification_probes verdict=PASSED") {
		t.Fatalf("summary should explain probe-primary verdict, got %q", result.Summary)
	}
}

func TestRunTestsVerificationProbePassContinuesProjectSuiteWhenPlanTouchesTests(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatalf("mkdir tests: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "__init__.py"), []byte(""), 0o644); err != nil {
		t.Fatalf("write tests package marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	testBody := "import unittest\n\nclass ProjectSuite(unittest.TestCase):\n    def test_project_suite_would_fail(self):\n        self.assertTrue(False)\n"
	if err := os.WriteFile(filepath.Join(root, "tests", "test_project_suite.py"), []byte(testBody), 0o644); err != nil {
		t.Fatalf("write test: %v", err)
	}
	mu := types.NewMutableState("probe primary continues")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-probe-pass-continue",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py", "tests/test_project_suite.py"},
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "outcome-1",
			Kind:     types.WriteBehaviorObservable,
			Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpSatisfies,
			Expected: "widget value is 42",
			Required: true,
			Source:   "expected_outcome_fallback",
		}},
		VerificationProbes: []types.VerificationProbe{{
			ID:                "value_contract",
			Language:          "python",
			Code:              "import widget\nassert widget.VALUE == 42\n",
			ContractRefs:      []string{"outcome-1"},
			ChangedSymbolRefs: []string{"widget.VALUE"},
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner":    "python",
		"framework": "unittest",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("project-suite failure should fail after probe continuation, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.NormalizeVerificationStatus() != types.VerificationStatusFailed {
		t.Fatalf("VerificationStatus = %q, want failed; report=%+v", report.NormalizeVerificationStatus(), report)
	}
	foundProbeCommand := false
	foundContinuedSuite := false
	foundExecutedSuite := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "verification_probe" && cmd.Source == "pre_suite_verification_probe" && cmd.Outcome == "executed" {
			foundProbeCommand = true
		}
		if cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Source == "probe_primary_suite_continued" &&
			cmd.Outcome == "suite_continued" && cmd.ReasonCode == "plan_touches_test_path" {
			foundContinuedSuite = true
		}
		if cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Source == "llm_choice" && cmd.Outcome == "executed" {
			foundExecutedSuite = true
		}
		if cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Source == "probe_primary_suite_skipped" {
			t.Fatalf("suite must not be marked skipped when continuation is required: %+v", report.ExecutedCommands)
		}
	}
	if !foundProbeCommand || !foundContinuedSuite || !foundExecutedSuite {
		t.Fatalf("expected probe, continuation, and suite command evidence; got %+v", report.ExecutedCommands)
	}
}

func TestRunTestsVerificationProbeSubprocessInheritsWorktreeSrcRoot(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	pkgDir := filepath.Join(root, "src", "probe_pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write package: %v", err)
	}
	mu := types.NewMutableState("probe subprocess src layout")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-src-probe",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"src/probe_pkg/__init__.py"},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "subprocess_src_import",
			Language: "python",
			Code: strings.Join([]string{
				"import os, subprocess, sys",
				"snippet = \"import os, probe_pkg; print(probe_pkg.VALUE); print(os.path.realpath(probe_pkg.__file__))\"",
				"res = subprocess.run([sys.executable, '-c', snippet], capture_output=True, text=True)",
				"assert res.returncode == 0, res.stdout + res.stderr",
				"print(res.stdout)",
				"expected = os.path.realpath(os.path.join(os.getcwd(), 'src', 'probe_pkg', '__init__.py'))",
				"assert '42' in res.stdout, res.stdout",
				"assert expected in res.stdout, res.stdout",
			}, "\n"),
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("verification probe subprocess should inherit worktree src import root, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		t.Fatalf("VerificationStatus = %q, want passed; report=%+v", report.NormalizeVerificationStatus(), report)
	}
	if len(report.TestResults) != 1 || !report.TestResults[0].Passed {
		t.Fatalf("verification probe result missing or failed: %+v", report.TestResults)
	}
}

func TestRunTestsRejectsTestSurfaceCandidateIDAsSuiteSelector(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[tool.pytest.ini_options]\ntestpaths = [\"testing\"]\n"), 0o644); err != nil {
		t.Fatalf("write pyproject: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "testing"), 0o755); err != nil {
		t.Fatalf("mkdir testing: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "testing", "test_widget.py"), []byte("def test_ok():\n    assert True\n"), 0o644); err != nil {
		t.Fatalf("write test: %v", err)
	}
	mu := types.NewMutableState("surface id suite")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
		"suite":  "python/pytest@.",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("TestSurface candidate id in suite should be rejected before execution, got %+v", result)
	}
	if !strings.Contains(result.Summary, "typed TestSurface candidate id") ||
		!strings.Contains(result.Summary, `runner="python"`) ||
		!strings.Contains(result.Summary, `working_dir="."`) {
		t.Fatalf("rejection should explain how to select the candidate, got: %s", result.Summary)
	}
	if report := mu.ChangeReport(); report != nil {
		t.Fatalf("suite candidate-id rejection must not install authoritative report: %+v", report)
	}
	if probes := mu.PlanStageProbeReports(); len(probes) != 0 {
		t.Fatalf("suite candidate-id rejection must not install planner probe report: %+v", probes)
	}
}

func TestRunTestsInheritsScopedSuiteFromVerifyFailureHandoff(t *testing.T) {
	mu := types.NewMutableState("verify scope inheritance")
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		PlanID:  "plan-old",
		BatchID: "batch-1",
		Attempt: 1,
		Executed: []types.ExecutedCommand{{
			Runner:     "python",
			Framework:  "pytest",
			WorkingDir: "pkg",
			Outcome:    "executed",
		}},
		FailingTests: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "test_a", Suite: "tests/test_cli.py", Passed: false},
			{Kind: types.TestResultKindUnit, AssertionID: "test_b", Suite: "tests/test_cli.py", Passed: false},
		},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
	}
	params := runTestsParams{Runner: "python"}
	if !inheritRunTestsScopeFromVerifyFailureHandoff(ctx, &params) {
		t.Fatal("expected verify scope to be inherited from typed handoff")
	}
	if params.Suite != "tests/test_cli.py" {
		t.Fatalf("suite = %q, want scoped failing suite", params.Suite)
	}
	if params.WorkingDir != "pkg" {
		t.Fatalf("working_dir = %q, want inherited command cwd", params.WorkingDir)
	}
	if params.Framework != "pytest" {
		t.Fatalf("framework = %q, want inherited framework", params.Framework)
	}
}

func TestRunTestsDoesNotInventSuiteForAmbiguousFailureHandoff(t *testing.T) {
	mu := types.NewMutableState("ambiguous verify scope")
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		Executed: []types.ExecutedCommand{{
			Runner:  "python",
			Outcome: "executed",
		}},
		FailingTests: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "test_a", Suite: "tests/test_cli.py", Passed: false},
			{Kind: types.TestResultKindUnit, AssertionID: "test_b", Suite: "tests/test_json.py", Passed: false},
		},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
	}
	params := runTestsParams{}
	if !inheritRunTestsScopeFromVerifyFailureHandoff(ctx, &params) {
		t.Fatal("expected runner provenance to be inherited")
	}
	if params.Runner != "python" {
		t.Fatalf("runner = %q, want inherited python", params.Runner)
	}
	if params.Suite != "" {
		t.Fatalf("ambiguous suites must not be collapsed into one selector, got %q", params.Suite)
	}
}

func TestRunTestsDoesNotInheritScopeAcrossAmbiguousExecutedCommands(t *testing.T) {
	mu := types.NewMutableState("ambiguous executed commands")
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		Executed: []types.ExecutedCommand{
			{Runner: "python", Framework: "pytest", WorkingDir: ".", Outcome: "executed"},
			{Runner: "make", WorkingDir: ".", Outcome: "executed"},
		},
		FailingTests: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "test_a", Suite: "tests/test_cli.py", Passed: false},
		},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
	}
	params := runTestsParams{}
	if inheritRunTestsScopeFromVerifyFailureHandoff(ctx, &params) {
		t.Fatalf("ambiguous command lineage must not be inherited: %+v", params)
	}
	params = runTestsParams{Runner: "python"}
	if !inheritRunTestsScopeFromVerifyFailureHandoff(ctx, &params) {
		t.Fatal("explicit runner should disambiguate command lineage")
	}
	if params.Suite != "tests/test_cli.py" || params.Framework != "pytest" {
		t.Fatalf("disambiguated scope not inherited: %+v", params)
	}
}

func TestRunTestsVerificationProbeImportErrorIsParserError(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	mu := types.NewMutableState("probe import error")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-probe-import",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py"},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "missing_dependency",
			Language: "python",
			Code:     "import definitely_missing_codrax_probe_dependency\n",
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("import-error verification_probe must not pass run_tests, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.FailureKind != types.FailureKindParserError {
		t.Fatalf("FailureKind = %q, want parser_error; report=%+v", report.FailureKind, report)
	}
	if report.FailureReasonCode != "verification_probe_module_not_found" {
		t.Fatalf("FailureReasonCode = %q, want verification_probe_module_not_found; report=%+v", report.FailureReasonCode, report)
	}
	if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "missing_dependency" || report.TestResults[0].Passed {
		t.Fatalf("verification probe result missing or wrong: %+v", report.TestResults)
	}
	if !strings.Contains(report.TestResults[0].FailureDetail, "structured outcome: import_error") {
		t.Fatalf("failure detail should include structured import_error outcome, got: %s", report.TestResults[0].FailureDetail)
	}
	foundParserErrorCommand := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "verification_probe" && cmd.Outcome == "parser_error" && cmd.Source == "pre_suite_verification_probe" && cmd.ReasonCode == "verification_probe_module_not_found" {
			foundParserErrorCommand = true
		}
	}
	if !foundParserErrorCommand {
		t.Fatalf("executed command evidence should include parser_error probe reason_code, got %+v", report.ExecutedCommands)
	}
}

func TestRunTestsVerificationProbePythonPackageWorkingDirExecutesAtProjectRoot(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "setup.py"), []byte("from setuptools import setup\nsetup(name='shadow-demo')\n"), 0o644); err != nil {
		t.Fatalf("write setup.py: %v", err)
	}
	pkg := filepath.Join(root, "lib", "matplotlib")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatalf("mkdir package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "__init__.py"), []byte(""), 0o644); err != nil {
		t.Fatalf("write __init__.py: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "collections.py"), []byte("raise RuntimeError('package collections shadowed stdlib')\n"), 0o644); err != nil {
		t.Fatalf("write collections.py: %v", err)
	}
	mu := types.NewMutableState("probe package cwd")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-probe-cwd",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"lib/matplotlib/backend_ps.py"},
		VerificationProbes: []types.VerificationProbe{{
			ID:         "stdlib_collections_visible",
			Language:   "python",
			WorkingDir: "lib/matplotlib",
			Code:       "from collections import deque\nq = deque([1])\nassert q.pop() == 1\n",
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("verification_probe should execute at project root and pass, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		t.Fatalf("VerificationStatus = %q, want passed; report=%+v", report.NormalizeVerificationStatus(), report)
	}
	foundProjectRootCWD := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "verification_probe" && cmd.Framework == "python" {
			foundProjectRootCWD = cmd.WorkingDir == "."
		}
	}
	if !foundProjectRootCWD {
		t.Fatalf("verification_probe should record project-root cwd, got %+v", report.ExecutedCommands)
	}
}

func TestRunTestsVerificationProbeRuntimeExceptionIsTestFailure(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	mu := types.NewMutableState("probe exception")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-probe-exception",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py"},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "missing_xml_artifact",
			Language: "python",
			Code:     "raise FileNotFoundError('/tmp/missing.xml')\n",
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("exception verification_probe must not pass run_tests, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.FailureKind != types.FailureKindTestsFailed {
		t.Fatalf("FailureKind = %q, want tests_failed; report=%+v", report.FailureKind, report)
	}
	if report.FailureReasonCode != "verification_probe_exception" {
		t.Fatalf("FailureReasonCode = %q, want verification_probe_exception; report=%+v", report.FailureReasonCode, report)
	}
	if got := report.NormalizeVerificationStatus(); got != types.VerificationStatusFailed {
		t.Fatalf("VerificationStatus = %q, want failed", got)
	}
	if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "missing_xml_artifact" || report.TestResults[0].Passed {
		t.Fatalf("verification probe result missing or wrong: %+v", report.TestResults)
	}
	if !strings.Contains(report.TestResults[0].FailureDetail, "structured outcome: exception") {
		t.Fatalf("failure detail should include structured exception outcome, got: %s", report.TestResults[0].FailureDetail)
	}
	foundFailedCommand := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "verification_probe" && cmd.Outcome == "executed" && cmd.Source == "pre_suite_verification_probe" && cmd.ReasonCode == "verification_probe_exception" {
			foundFailedCommand = true
		}
	}
	if !foundFailedCommand {
		t.Fatalf("executed command evidence should include failing probe reason_code, got %+v", report.ExecutedCommands)
	}
}

func TestRunTestsVerificationProbeNameErrorIsParserError(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	mu := types.NewMutableState("probe name error")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-probe-name-error",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py"},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "bad_probe_symbol",
			Language: "python",
			Code:     "missing_probe_symbol\n",
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("NameError verification_probe must not pass run_tests, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.FailureKind != types.FailureKindParserError {
		t.Fatalf("FailureKind = %q, want parser_error; report=%+v", report.FailureKind, report)
	}
	if report.FailureReasonCode != "verification_probe_name_error" {
		t.Fatalf("FailureReasonCode = %q, want verification_probe_name_error; report=%+v", report.FailureReasonCode, report)
	}
	if got := report.NormalizeVerificationStatus(); got != types.VerificationStatusUnavailable {
		t.Fatalf("VerificationStatus = %q, want unavailable", got)
	}
	if !changeReportHasVerificationDiagnostic(report, "probe_authoring", "verification_probe_name_error") {
		t.Fatalf("verification probe authoring diagnostic missing: %+v", report.VerificationDiagnostics)
	}
}

func TestVerificationDiagnosticsPreserveProbeAndSuiteSignals(t *testing.T) {
	diags := verificationDiagnosticsFromExecutedCommands([]types.ExecutedCommand{
		{
			Runner:     "verification_probe",
			Framework:  "python",
			WorkingDir: ".",
			Command:    "python -c <verification_probe:bad>",
			ExitCode:   1,
			Source:     "pre_suite_verification_probe",
			Outcome:    "parser_error",
			ReasonCode: "verification_probe_name_error",
		},
		{
			Runner:     "python",
			Framework:  "unittest",
			WorkingDir: ".",
			Command:    "python -m unittest discover",
			ExitCode:   1,
			Source:     "llm_choice",
			Outcome:    "parser_error",
			ReasonCode: "unittest_loader_import_error",
		},
	})
	if len(diags) != 2 {
		t.Fatalf("expected two diagnostics, got %+v", diags)
	}
	if !verificationDiagnosticsContain(diags, "probe_authoring", "verification_probe_name_error") {
		t.Fatalf("probe authoring diagnostic missing: %+v", diags)
	}
	if !verificationDiagnosticsContain(diags, "parser_or_startup", "unittest_loader_import_error") {
		t.Fatalf("suite parser/startup diagnostic missing: %+v", diags)
	}
}

func TestVerificationConfidenceRecordsFromProbeReport(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-confidence",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "outcome-1",
			Kind:     types.WriteBehaviorObservable,
			Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpEquals,
			Expected: "repr shows units",
			Required: true,
			Source:   "write_analyzer",
		}},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "probe",
			Language: "python",
			Code:     "assert True",
		}},
	}
	report := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{{
			AssertionID: "probe",
			Suite:       "verification_probe/python",
			Passed:      true,
		}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:    "verification_probe",
			Framework: "python",
			Source:    "pre_suite_verification_probe",
			Outcome:   "executed",
		}, {
			Runner:  "python",
			Source:  "probe_primary_suite_skipped",
			Outcome: "suite_skipped",
		}},
	}
	records := verificationConfidenceRecordsFromReport(plan, report)
	if !verificationConfidenceContains(records, "probe_contract_refs", "missing", "verification_probe_missing_required_contract_ref") {
		t.Fatalf("missing contract-ref confidence record absent: %+v", records)
	}
	if !verificationConfidenceContains(records, "probe_changed_symbol", "missing", "verification_probe_missing_changed_symbol_ref") {
		t.Fatalf("missing changed-symbol confidence record absent: %+v", records)
	}

	plan.VerificationProbes[0].ContractRefs = []string{"outcome-1"}
	plan.VerificationProbes[0].ChangedSymbolRefs = []string{"widget.repr"}
	records = verificationConfidenceRecordsFromReport(plan, report)
	if verificationConfidenceContains(records, "probe_contract_refs", "missing", "verification_probe_missing_required_contract_ref") {
		t.Fatalf("covered contract refs should not emit missing record: %+v", records)
	}
	if !verificationConfidenceContains(records, "probe_contract_refs", "satisfied", "verification_probe_contract_ref_covered") {
		t.Fatalf("covered contract refs should emit satisfied record: %+v", records)
	}
	if !verificationConfidenceContains(records, "probe_changed_symbol", "satisfied", "verification_probe_changed_symbol_coupled") {
		t.Fatalf("changed symbol coupling should emit satisfied record: %+v", records)
	}
}

func TestVerificationConfidenceRecordsFromProbeReportIgnoresSoftSatisfiesContractRefs(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-confidence-soft",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "soft-outcome",
			Kind:     types.WriteBehaviorObservable,
			Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpSatisfies,
			Expected: "natural language behavior text",
			Required: true,
			Source:   "write_analyzer",
		}},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "probe",
			Language: "python",
			Code:     "assert True",
		}},
	}
	report := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{{
			AssertionID: "probe",
			Suite:       "verification_probe/python",
			Passed:      true,
		}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:    "verification_probe",
			Framework: "python",
			Source:    "pre_suite_verification_probe",
			Outcome:   "executed",
		}, {
			Runner:  "python",
			Source:  "probe_primary_suite_skipped",
			Outcome: "suite_skipped",
		}},
	}
	records := verificationConfidenceRecordsFromReport(plan, report)
	if verificationConfidenceContains(records, "probe_contract_refs", "missing", "verification_probe_missing_required_contract_ref") {
		t.Fatalf("soft satisfies contract should not emit missing hard contract-ref record: %+v", records)
	}
	if verificationConfidenceContains(records, "probe_contract_refs", "satisfied", "verification_probe_contract_ref_covered") {
		t.Fatalf("soft satisfies contract should not emit hard contract-ref covered record: %+v", records)
	}
}

func TestVerificationConfidenceRecordsFromProbeReportRecordsPartialMissingContracts(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-confidence-partial",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "c1",
			Kind:     types.WriteBehaviorObservable,
			Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpEquals,
			Expected: "first behavior",
			Required: true,
			Source:   "write_analyzer",
		}, {
			ID:       "c2",
			Kind:     types.WriteBehaviorObservable,
			Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpEquals,
			Expected: "second behavior",
			Required: true,
			Source:   "write_analyzer",
		}},
		VerificationProbes: []types.VerificationProbe{{
			ID:           "probe",
			Language:     "python",
			Code:         "assert True",
			ContractRefs: []string{"c1"},
		}},
	}
	report := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{{
			AssertionID: "probe",
			Suite:       "verification_probe/python",
			Passed:      true,
		}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:  "verification_probe",
			Source:  "pre_suite_verification_probe",
			Outcome: "executed",
		}},
	}
	records := verificationConfidenceRecordsFromReport(plan, report)
	if !verificationConfidenceContains(records, "probe_contract_refs", "satisfied", "verification_probe_contract_ref_covered") {
		t.Fatalf("partial coverage should still record covered contract refs: %+v", records)
	}
	var missing []string
	for _, record := range records {
		if record.Category == "probe_contract_refs" && record.Status == "missing" &&
			record.ReasonCode == "verification_probe_missing_required_contract_ref" {
			missing = record.ContractRefs
			break
		}
	}
	if len(missing) != 1 || missing[0] != "c2" {
		t.Fatalf("missing contract refs = %+v, want [c2]; records=%+v", missing, records)
	}
}

func TestVerificationConfidenceRecordsFromUnavailableAndSyntaxFallback(t *testing.T) {
	unavailable := &types.ChangeReport{
		Passed:            false,
		FailureKind:       types.FailureKindParserError,
		FailureReasonCode: "pytest_import_startup_error",
	}
	records := verificationConfidenceRecordsFromReport(nil, unavailable)
	if !verificationConfidenceContains(records, "project_runner", "unavailable", "pytest_import_startup_error") {
		t.Fatalf("project runner unavailable confidence missing: %+v", records)
	}

	syntax := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{{
			AssertionID: "py_compile",
			Suite:       "py_compile",
			Passed:      true,
		}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:  "python",
			Source:  "auto_detect",
			Outcome: "syntax_check_fallback",
		}},
	}
	records = verificationConfidenceRecordsFromReport(nil, syntax)
	if !verificationConfidenceContains(records, "source_compile", "satisfied", "source_compile_ok") {
		t.Fatalf("source compile confidence missing: %+v", records)
	}
}

func TestRunTestsVerificationProbeProductNameErrorIsTestFailure(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("def explode():\n    return missing_product_symbol\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	mu := types.NewMutableState("product name error")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-product-name-error",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"widget.py"},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "product_name_error",
			Language: "python",
			Code:     "import widget\nwidget.explode()\n",
		}},
	})
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("product NameError verification_probe must not pass run_tests, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.FailureKind != types.FailureKindTestsFailed {
		t.Fatalf("FailureKind = %q, want tests_failed; report=%+v", report.FailureKind, report)
	}
	if report.FailureReasonCode != "verification_probe_name_error" {
		t.Fatalf("FailureReasonCode = %q, want verification_probe_name_error; report=%+v", report.FailureReasonCode, report)
	}
	if got := report.NormalizeVerificationStatus(); got != types.VerificationStatusFailed {
		t.Fatalf("VerificationStatus = %q, want failed", got)
	}
}

func TestRenderVerificationProbeOutputCarriesProbeSource(t *testing.T) {
	out := renderVerificationProbeOutput([]types.VerificationProbe{{
		ID:                     "punctuation_note",
		Language:               "python",
		WorkingDir:             "src",
		TimeoutSeconds:         2,
		ExpectedStdout:         []string{"ok"},
		ContractRefs:           []string{"quiet-stdout"},
		ChangedSymbolRefs:      []string{"pkg.module.func"},
		ExpectsBaselineFailure: true,
		Code:                   "print('ok')\n",
	}}, []string{"ok\n"})
	for _, want := range []string{
		"#### punctuation_note",
		"language=python working_dir=src timeout_seconds=2",
		"expected_stdout=[\"ok\"]",
		"contract_refs=[\"quiet-stdout\"]",
		"changed_symbol_refs=[\"pkg.module.func\"]",
		"expects_baseline_failure=true",
		"source:\n```python\nprint('ok')\n```",
		"output:\nok",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("probe output missing %q:\n%s", want, out)
		}
	}
}

func TestDetectPythonTestFramework(t *testing.T) {
	t.Run("django runtests wins", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "tests"), 0o755); err != nil {
			t.Fatalf("mkdir tests: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "tests", "runtests.py"), []byte("print('django tests')\n"), 0o644); err != nil {
			t.Fatalf("write runtests.py: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "pytest.ini"), []byte("[pytest]\n"), 0o644); err != nil {
			t.Fatalf("write pytest.ini: %v", err)
		}
		if got := detectPythonTestFramework(root); got != pythonFrameworkDjango {
			t.Fatalf("framework = %q, want django", got)
		}
	})
	t.Run("pytest config wins", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "pytest.ini"), []byte("[pytest]\n"), 0o644); err != nil {
			t.Fatalf("write pytest.ini: %v", err)
		}
		if got := detectPythonTestFramework(root); got != pythonFrameworkPytest {
			t.Fatalf("framework = %q, want pytest", got)
		}
	})
	t.Run("unittest source signal", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "test_widget.py"), []byte("import unittest\n\nclass WidgetTests(unittest.TestCase):\n    pass\n"), 0o644); err != nil {
			t.Fatalf("write test: %v", err)
		}
		if got := detectPythonTestFramework(root); got != pythonFrameworkUnittest {
			t.Fatalf("framework = %q, want unittest", got)
		}
	})
	t.Run("pytest style default", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "test_widget.py"), []byte("def test_widget():\n    assert True\n"), 0o644); err != nil {
			t.Fatalf("write test: %v", err)
		}
		if got := detectPythonTestFramework(root); got != pythonFrameworkPytest {
			t.Fatalf("framework = %q, want pytest", got)
		}
	})
}

func TestShouldPreservePythonPytestZeroTestsForExplicitPytestSurface(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[tool.pytest.ini_options]\ntestpaths = [\"xarray/tests\"]\n"), 0o644); err != nil {
		t.Fatalf("write pyproject.toml: %v", err)
	}
	plan := runnerPlan{Runner: "python", Framework: pythonFrameworkPytest, Root: root}
	if !shouldPreservePythonPytestZeroTests(plan) {
		t.Fatal("pytest zero-tests should stay no_tests when an explicit pytest surface exists")
	}
	plan.Framework = pythonFrameworkUnittest
	if shouldPreservePythonPytestZeroTests(plan) {
		t.Fatal("unittest zero-tests must not use pytest preservation")
	}
	plainRoot := t.TempDir()
	plan = runnerPlan{Runner: "python", Framework: pythonFrameworkPytest, Root: plainRoot}
	if shouldPreservePythonPytestZeroTests(plan) {
		t.Fatal("bare python roots without pytest config may still escalate to another typed candidate")
	}
}

func TestRunTestsPythonUnittestFrameworkPasses(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	testFile := filepath.Join(root, "test_sample.py")
	if err := os.WriteFile(testFile, []byte(`import unittest

class SampleTests(unittest.TestCase):
    def test_ok(self):
        self.assertEqual(1 + 1, 2)
`), 0o644); err != nil {
		t.Fatalf("write unittest file: %v", err)
	}
	mu := types.NewMutableState("unittest verify")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner":    "python",
		"framework": "unittest",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("unittest run should pass, got %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if report.Channel != types.ChangeReportChannelPostApplyVerify {
		t.Fatalf("report channel = %q, want post_apply_verify", report.Channel)
	}
	if !report.Passed || len(report.TestResults) != 1 || !report.TestResults[0].Passed {
		t.Fatalf("unexpected unittest report: %+v", report)
	}
	if !strings.Contains(report.TestResults[0].Suite, "SampleTests") {
		t.Fatalf("unittest suite should identify the TestCase, got %+v", report.TestResults[0])
	}
}

func TestRunTestsFrameworkOnlyDjangoBypassesManifestAutoDetect(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatalf("mkdir tests: %v", err)
	}
	runtests := `import sys

print("test_scoped (migrations.test_commands.SqlMigrateTests) ... ok")
print("")
print("----------------------------------------------------------------------")
print("Ran 1 test in 0.001s")
print("")
print("OK")
print("ARGV:" + " ".join(sys.argv[1:]))
`
	if err := os.WriteFile(filepath.Join(root, "tests", "runtests.py"), []byte(runtests), 0o644); err != nil {
		t.Fatalf("write runtests.py: %v", err)
	}
	// These manifests would make legacy auto-detect try unrelated
	// lanes first. A typed framework enum is already a precise runner
	// decision, so it must execute only the Django runner.
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"test":"echo node should not run"}}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	writeMakefile(t, root, "check:\n\t@echo make should not run\n")

	mu := types.NewMutableState("django framework-only verify")
	ctx := &types.BusContext{
		Mutable:       mu,
		Mode:          types.ModeApply,
		PipelineStage: types.StageVerify,
		RepoRoot:      root,
		MainRepoRoot:  root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"framework": "django",
		"suite":     "migrations.test_commands.SqlMigrateTests.test_sqlmigrate_for_atomic_migration_without_ddl_rollback",
	}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("django framework-only run should pass, got %+v report=%+v", result, mu.ChangeReport())
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("run_tests should populate ChangeReport")
	}
	if len(report.ExecutedCommands) != 1 {
		t.Fatalf("framework-only selection must bypass manifest auto-detect; executed=%+v", report.ExecutedCommands)
	}
	cmd := report.ExecutedCommands[0]
	if cmd.Runner != "python" || cmd.Framework != pythonFrameworkDjango || cmd.Source != "llm_framework_choice" {
		t.Fatalf("unexpected command provenance: %+v", cmd)
	}
	if strings.Contains(cmd.Command, "npm") || strings.Contains(cmd.Command, "make ") {
		t.Fatalf("command should be Django runner only, got %q", cmd.Command)
	}
	if !strings.Contains(cmd.Command, "tests/runtests.py") ||
		!strings.Contains(cmd.Command, "migrations.test_commands.SqlMigrateTests.test_sqlmigrate_for_atomic_migration_without_ddl_rollback") {
		t.Fatalf("django command should preserve scoped suite, got %q", cmd.Command)
	}
}

// TestPythonInterpreter_InterleavedVenvOrdering locks fix A: when
// both worktree and main_repo carry venvs, the probe interleaves
// per dir-name (.venv across all roots before falling to venv across
// all roots). Concretely: main/.venv must beat worktree/venv even
// though worktree comes first in the roots list.
func TestPythonInterpreter_InterleavedVenvOrdering(t *testing.T) {
	worktree := t.TempDir()
	mainRepo := t.TempDir()
	// worktree has only `venv/` (older convention).
	wtVenv := filepath.Join(worktree, "venv", "bin")
	if err := os.MkdirAll(wtVenv, 0o755); err != nil {
		t.Fatalf("mkdir worktree venv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtVenv, "python"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	// mainRepo has the more conventional `.venv/` — should win.
	mrVenv := filepath.Join(mainRepo, ".venv", "bin")
	if err := os.MkdirAll(mrVenv, 0o755); err != nil {
		t.Fatalf("mkdir mainRepo .venv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mrVenv, "python"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	interp, asModule := pythonInterpreter(worktree, mainRepo)
	if !asModule {
		t.Error("asModule should be true for venv pythons")
	}
	if !strings.Contains(filepath.ToSlash(interp), filepath.ToSlash(mainRepo)) {
		t.Errorf("expected mainRepo .venv to win; got %q", interp)
	}
	if !strings.Contains(interp, ".venv") {
		t.Errorf("expected .venv to win over venv across roots; got %q", interp)
	}
}

// TestRunNodeCheckFallback_PassWhenAllFilesParse drives the node
// fallback end-to-end. Skipped without node on PATH.
func TestRunNodeCheckFallback_PassWhenAllFilesParse(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH; skip")
	}
	root := t.TempDir()
	good := filepath.Join(root, "app.js")
	if err := os.WriteFile(good, []byte("console.log('hi');\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	report, output := runNodeCheckFallback(nil, "node@.", root, []string{good})
	if !report.Passed {
		t.Errorf("clean js should pass node --check; got: %+v", report)
	}
	if !strings.Contains(output, "ok    app.js") {
		t.Errorf("output should record per-file ok: %q", output)
	}
}

func TestRunNodeCheckFallback_FailureCarriesBuildErrors(t *testing.T) {
	dir := t.TempDir()
	nodeBin := filepath.Join(dir, "node")
	script := `#!/bin/sh
printf "%s:3\nconst =\n      ^\n\nSyntaxError: Unexpected token '='\n" "$2"
exit 1
`
	if err := os.WriteFile(nodeBin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake node: %v", err)
	}
	t.Setenv("PATH", dir)
	root := t.TempDir()
	bad := filepath.Join(root, "app.js")
	if err := os.WriteFile(bad, []byte("const =\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	report, _ := runNodeCheckFallback(nil, "node@.", root, []string{bad})
	if report == nil || report.Passed {
		t.Fatalf("bad js should fail node --check, got %+v", report)
	}
	if report.FailureReasonCode != "node_syntax_check_failed" {
		t.Fatalf("FailureReasonCode=%q", report.FailureReasonCode)
	}
	if len(report.TestResults) != 1 || len(report.TestResults[0].BuildErrors) != 1 {
		t.Fatalf("node syntax failure should carry one build error, got %+v", report.TestResults)
	}
	if got := report.TestResults[0].BuildErrors[0]; got.File != bad || got.Line != 3 || got.Symbol != "SyntaxError" {
		t.Fatalf("node build error mis-parsed: %+v", got)
	}
}

// TestRunRubyCheckFallback_PassWhenAllFilesParse drives the ruby
// fallback end-to-end. Skipped without ruby on PATH.
func TestRunRubyCheckFallback_PassWhenAllFilesParse(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH; skip")
	}
	root := t.TempDir()
	good := filepath.Join(root, "app.rb")
	if err := os.WriteFile(good, []byte("puts 'hi'\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	report, output := runRubyCheckFallback(nil, "ruby@.", root, []string{good})
	if !report.Passed {
		t.Errorf("clean rb should pass ruby -c; got: %+v", report)
	}
	if !strings.Contains(output, "ok    app.rb") {
		t.Errorf("output should record per-file ok: %q", output)
	}
}

func TestRunRubyCheckFallback_FailureCarriesBuildErrors(t *testing.T) {
	dir := t.TempDir()
	rubyBin := filepath.Join(dir, "ruby")
	script := `#!/bin/sh
printf "%s:4: syntax error, unexpected end-of-input, expecting end\n" "$2"
exit 1
`
	if err := os.WriteFile(rubyBin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ruby: %v", err)
	}
	t.Setenv("PATH", dir)
	root := t.TempDir()
	bad := filepath.Join(root, "app.rb")
	if err := os.WriteFile(bad, []byte("def x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	report, _ := runRubyCheckFallback(nil, "ruby@.", root, []string{bad})
	if report == nil || report.Passed {
		t.Fatalf("bad rb should fail ruby -c, got %+v", report)
	}
	if report.FailureReasonCode != "ruby_syntax_check_failed" {
		t.Fatalf("FailureReasonCode=%q", report.FailureReasonCode)
	}
	if len(report.TestResults) != 1 || len(report.TestResults[0].BuildErrors) != 1 {
		t.Fatalf("ruby syntax failure should carry one build error, got %+v", report.TestResults)
	}
	if got := report.TestResults[0].BuildErrors[0]; got.File != bad || got.Line != 4 {
		t.Fatalf("ruby build error mis-parsed: %+v", got)
	}
}

// TestSyntaxCheckExtensions_OnlySupportedRunners locks which
// runners opt into the syntax-check fallback. Adding a new runner
// to syntaxCheckExtensions without a matching runSyntaxCheckFallback
// dispatch would silently drop pre-flight files; this drift-guard
// test catches it.
func TestSyntaxCheckExtensions_OnlySupportedRunners(t *testing.T) {
	for _, runner := range []string{"python", "node", "ruby"} {
		exts := syntaxCheckExtensions(runner)
		if len(exts) == 0 {
			t.Errorf("runner %q should declare extensions for syntax-check fallback", runner)
		}
		// Verify dispatch exists by attempting on empty file list —
		// the dispatcher must return ok=true even with zero files
		// (it'll produce a NoTestsRunners report).
		report, _, ok := runSyntaxCheckFallback(nil, "test", t.TempDir(), runner, nil)
		if !ok {
			t.Errorf("runner %q has extensions but no syntax-check dispatcher", runner)
		}
		if report == nil {
			t.Errorf("runner %q dispatcher returned nil report", runner)
		}
	}
	for _, runner := range []string{"go", "rust", "java", "cmake"} {
		_, _, ok := runSyntaxCheckFallback(nil, "test", t.TempDir(), runner, nil)
		if ok {
			t.Errorf("runner %q should NOT have syntax-check dispatcher (extensions unsupported)", runner)
		}
	}
}

// TestWarningPassedReport_StructureBilingual locks the structure
// of the fix-C downgrade report: Passed=true, NoTestsRunners
// populated, FailureSummary carries the advisory wording in the
// requested language.
func TestWarningPassedReport_StructureBilingual(t *testing.T) {
	zh := warningPassedReport("python", "pytest", "exit_code_127", 127, "stderr", "zh")
	if !zh.Passed {
		t.Error("warning report Passed should be true")
	}
	if len(zh.NoTestsRunners) != 1 || zh.NoTestsRunners[0] != "python" {
		t.Errorf("NoTestsRunners = %v; want [python]", zh.NoTestsRunners)
	}
	if !strings.Contains(zh.FailureSummary, "无测试任务") {
		t.Errorf("zh summary should contain 无测试任务; got %q", zh.FailureSummary)
	}
	en := warningPassedReport("python", "pytest", "exit_code_127", 127, "stderr", "en")
	if !strings.Contains(en.FailureSummary, "no test work to do") {
		t.Errorf("en summary should mention no test work; got %q", en.FailureSummary)
	}
}

func changeReportHasVerificationDiagnostic(report *types.ChangeReport, category, reasonCode string) bool {
	if report == nil {
		return false
	}
	return verificationDiagnosticsContain(report.VerificationDiagnostics, category, reasonCode)
}

func changeReportHasVerificationConfidence(report *types.ChangeReport, category, status, reasonCode string) bool {
	if report == nil {
		return false
	}
	return verificationConfidenceContains(report.VerificationConfidence, category, status, reasonCode)
}

func verificationConfidenceContains(records []types.VerificationConfidenceRecord, category, status, reasonCode string) bool {
	for _, record := range records {
		if record.Category == category && record.Status == status && record.ReasonCode == reasonCode {
			return true
		}
	}
	return false
}

func verificationDiagnosticsContain(diags []types.VerificationDiagnostic, category, reasonCode string) bool {
	for _, diag := range diags {
		if diag.Category == category && diag.ReasonCode == reasonCode {
			return true
		}
	}
	return false
}

func writeMakefile(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte(body), 0o644); err != nil {
		t.Fatalf("write Makefile: %v", err)
	}
}

func runTestsJSONParams(t *testing.T, params map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return raw
}
