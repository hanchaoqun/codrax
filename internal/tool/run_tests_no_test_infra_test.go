package tool

import (
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
// when python3 isn't on the test host.
func TestRunPyCompileFallback_PassWhenAllFilesParse(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH; skip")
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

// TestRunPyCompileFallback_FailOnSyntaxError verifies broken syntax
// produces a build_error TestResult with the file as AssertionID
// and a non-empty FailureDetail. Skipped without python3.
func TestRunPyCompileFallback_FailOnSyntaxError(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH; skip")
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
	if !strings.Contains(interp, mainRepo) {
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
