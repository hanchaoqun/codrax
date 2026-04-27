package tool

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDetectRunnerMissing_ExitCode127 covers the most common signal:
// shell-emitted exit 127 when the runner binary isn't on PATH. We
// stuff a synthetic *exec.ExitError into the chain to simulate what
// the supervisor would surface.
func TestDetectRunnerMissing_ExitCode127(t *testing.T) {
	// Drive a real command-not-found through exec so we get a
	// genuine *exec.ExitError with ProcessState — ee.ProcessState
	// can't be constructed by hand in test code.
	cmd := exec.Command("sh", "-c", "exit 127")
	runErr := cmd.Run()
	if runErr == nil {
		t.Fatal("test setup: sh -c 'exit 127' should return error")
	}
	missing, bin, _ := detectRunnerMissing("python", runErr, "")
	if !missing {
		t.Errorf("exit 127 should trigger runner-missing for python; got missing=false")
	}
	if bin != "pytest" {
		t.Errorf("expected binary=pytest, got %q", bin)
	}
}

// TestDetectRunnerMissing_StderrPattern covers the case where the
// supervisor swallowed the exit code but stderr captured the
// shell's "command not found" line.
func TestDetectRunnerMissing_StderrPattern(t *testing.T) {
	for _, tc := range []struct {
		runner     string
		output     string
		wantBinary string
	}{
		{"python", "/bin/sh: 1: pytest: not found\n", "pytest"},
		{"python", "/bin/sh: pytest: command not found\n", "pytest"},
		{"go", "bash: go: command not found\n", "go"},
		{"rust", "zsh: command not found: cargo\ncargo: not found\n", "cargo"},
		{"node", "/usr/bin/env: 'node': No such file or directory\nnpm: not found\n", "npm"},
		{"meson", "executable file not found in $PATH\n", "meson"},
	} {
		missing, bin, reason := detectRunnerMissing(tc.runner, nil, tc.output)
		if !missing {
			t.Errorf("%s: pattern %q should trigger detection", tc.runner, tc.output)
		}
		if bin != tc.wantBinary {
			t.Errorf("%s: bin = %q, want %q", tc.runner, bin, tc.wantBinary)
		}
		if !strings.HasPrefix(reason, "pattern: ") {
			t.Errorf("%s: reason should start with 'pattern: '; got %q", tc.runner, reason)
		}
	}
}

// TestDetectRunnerMissing_FalsePositiveResistance verifies the
// detector does NOT trigger on a test failure whose assertion
// message merely contains "not found" — that's a legitimate test
// red-output, not a missing-runner signal.
func TestDetectRunnerMissing_FalsePositiveResistance(t *testing.T) {
	output := `
=========================== short test summary ============================
FAILED tests/test_widget.py::test_lookup - AssertionError: 'foo' not found
1 failed, 7 passed
`
	missing, _, _ := detectRunnerMissing("python", nil, output)
	if missing {
		t.Errorf("test assertion 'not found' should NOT trigger detection; output=%q", output)
	}
}

// TestDetectRunnerMissing_ExecErrNotFound covers direct exec.Command
// failures where Go's os/exec wraps `errors.Is(err, exec.ErrNotFound)`
// — the signal a future direct-Cmd refactor would produce.
func TestDetectRunnerMissing_ExecErrNotFound(t *testing.T) {
	wrapped := errors.New("Cmd.Start: " + exec.ErrNotFound.Error())
	wrapped = errWrapNotFound{inner: exec.ErrNotFound}
	missing, _, reason := detectRunnerMissing("ruby", wrapped, "")
	if !missing {
		t.Errorf("exec.ErrNotFound should trigger detection")
	}
	if reason != "exec.ErrNotFound" {
		t.Errorf("reason = %q, want %q", reason, "exec.ErrNotFound")
	}
}

type errWrapNotFound struct{ inner error }

func (e errWrapNotFound) Error() string { return e.inner.Error() }
func (e errWrapNotFound) Unwrap() error { return e.inner }

// TestDetectRunnerMissing_UnknownRunnerNoBinary verifies the
// detector politely no-ops on an unknown runner identifier (defensive
// — should never happen with the allowedRunners gate, but the
// helper must not panic on garbage input).
func TestDetectRunnerMissing_UnknownRunnerNoBinary(t *testing.T) {
	missing, _, _ := detectRunnerMissing("does-not-exist", nil, "anything: command not found\n")
	if missing {
		t.Errorf("unknown runner should never trigger detection")
	}
}

// TestMakeRunnerMissingReport_ShapeAndFailureKind locks the wire
// contract: the synthesized report MUST set FailureKind to
// FailureKindRunnerMissing and BuildFailed=true so downstream consumers
// (the verify→plan retry suppressor + persistPlanStatus) can route
// on a single field.
func TestMakeRunnerMissingReport_ShapeAndFailureKind(t *testing.T) {
	report := makeRunnerMissingReport("python", "pytest", "/bin/sh: 1: pytest: not found", "en", "exit_code_127", 127)
	if report.FailureKind != types.FailureKindRunnerMissing {
		t.Errorf("FailureKind = %q, want %q", report.FailureKind, types.FailureKindRunnerMissing)
	}
	if report.Passed {
		t.Error("Passed should be false")
	}
	if !report.BuildFailed {
		t.Error("BuildFailed should be true (lifecycle slot mirrors compile failure)")
	}
	if !strings.Contains(report.FailureSummary, "pytest") {
		t.Errorf("FailureSummary should name the missing binary; got: %q", report.FailureSummary)
	}
	if !strings.Contains(report.FailureSummary, "install") {
		t.Errorf("FailureSummary should include install hint; got: %q", report.FailureSummary)
	}
	if len(report.TestResults) != 1 {
		t.Fatalf("TestResults count = %d, want 1 (synthetic build_error row)", len(report.TestResults))
	}
	tr := report.TestResults[0]
	if tr.Suite != "runner_missing" {
		t.Errorf("Suite = %q, want runner_missing", tr.Suite)
	}
	if tr.Kind != types.TestResultKindBuildError {
		t.Errorf("Kind = %q, want build_error", tr.Kind)
	}
}

// TestRunnerInstallHint_AllRunnersCovered locks the install-hint
// surface so adding a new runner forces a hint update via this
// drift-guard test. Both languages must be populated.
func TestRunnerInstallHint_AllRunnersCovered(t *testing.T) {
	for r := range allowedRunners {
		for _, lang := range []string{"zh", "en"} {
			hint := runnerInstallHint(r, lang)
			if strings.TrimSpace(hint) == "" {
				t.Errorf("runner %q lang=%q has no install hint — add one in runnerInstallHint",
					r, lang)
			}
		}
	}
}

// TestPythonInterpreter_PrefersDotVenv verifies the venv probe
// catches the canonical `.venv/bin/python` Unix layout and reports
// asModule=true so the caller invokes `<path> -m pytest`.
func TestPythonInterpreter_PrefersDotVenv(t *testing.T) {
	repo := t.TempDir()
	venvBin := filepath.Join(repo, ".venv", "bin")
	if err := os.MkdirAll(venvBin, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pyPath := filepath.Join(venvBin, "python")
	if err := os.WriteFile(pyPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	interp, asModule := pythonInterpreter(repo)
	if !asModule {
		t.Errorf("asModule should be true when venv exists")
	}
	if !strings.Contains(interp, ".venv") {
		t.Errorf("interpreter should point inside .venv; got %q", interp)
	}
	if !strings.Contains(interp, "/python") {
		t.Errorf("interpreter should be the venv python; got %q", interp)
	}
}

// TestPythonInterpreter_VenvAlternateLayouts verifies all four
// recognised venv directory names resolve. Each test creates a
// fresh tmp dir so the priority order doesn't interfere.
func TestPythonInterpreter_VenvAlternateLayouts(t *testing.T) {
	for _, dir := range []string{"venv", "env", ".virtualenv"} {
		t.Run(dir, func(t *testing.T) {
			repo := t.TempDir()
			binDir := filepath.Join(repo, dir, "bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			py := filepath.Join(binDir, "python3")
			if err := os.WriteFile(py, []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatalf("write: %v", err)
			}
			interp, asModule := pythonInterpreter(repo)
			if !asModule {
				t.Errorf("asModule should be true for %s/bin/python3 layout", dir)
			}
			if !strings.Contains(interp, dir) {
				t.Errorf("interpreter should reference %s; got %q", dir, interp)
			}
		})
	}
}

// TestPythonInterpreter_FallsBackToSystemPython verifies that with
// no venv present, the helper returns one of pytest / python3 /
// python from PATH (whichever exists on the host). We can't
// guarantee what's installed on the test host, so the assertion
// is loose: result is one of the expected fallbacks AND asModule
// matches the resolved interpreter (false only for bare pytest).
func TestPythonInterpreter_FallsBackToSystemPython(t *testing.T) {
	repo := t.TempDir()
	interp, asModule := pythonInterpreter(repo)
	switch interp {
	case "python3", "python":
		if !asModule {
			t.Errorf("asModule should be true for system %s", interp)
		}
	case "pytest":
		if asModule {
			t.Errorf("asModule should be false for bare pytest fallback")
		}
	default:
		t.Errorf("unexpected fallback interpreter %q", interp)
	}
}

// TestPythonInterpreter_PrefersStandalonePytestOverPython3 locks the
// load-bearing priority swap that fixed the customer scenario where
// `python3 -m pytest` failed because the resolved python3 was a
// different build (`/opt/python/bin/python3`) from the one pip
// installed pytest under (the system `/usr/bin/python3`). When
// standalone `pytest` is on PATH, its shebang knows the right python,
// so we MUST prefer it over `<other-python> -m pytest`.
//
// The test sandboxes PATH so only fake `pytest` and `python3`
// scripts are visible — the assertion is then deterministic
// regardless of what's installed on the host.
func TestPythonInterpreter_PrefersStandalonePytestOverPython3(t *testing.T) {
	dir := t.TempDir()
	pytestBin := filepath.Join(dir, "pytest")
	if err := os.WriteFile(pytestBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write pytest: %v", err)
	}
	python3Bin := filepath.Join(dir, "python3")
	if err := os.WriteFile(python3Bin, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write python3: %v", err)
	}
	t.Setenv("PATH", dir)

	repo := t.TempDir() // no venv inside, so probe falls through
	interp, asModule := pythonInterpreter(repo)
	if interp != "pytest" {
		t.Errorf("expected standalone pytest to win when both are on PATH; got %q", interp)
	}
	if asModule {
		t.Errorf("standalone pytest must be invoked bare (asModule=false); got asModule=true")
	}
}

// TestDetectRunnerMissing_PythonModuleMissing covers the
// `python -m pytest` failure mode where the interpreter resolves
// but pytest isn't installed. Should classify as runner_missing
// so the verify→plan retry suppressor short-circuits.
func TestDetectRunnerMissing_PythonModuleMissing(t *testing.T) {
	for _, output := range []string{
		"/usr/bin/python3: No module named pytest\n",
		"/path/to/python: No module named 'pytest'\n",
	} {
		missing, bin, _ := detectRunnerMissing("python", nil, output)
		if !missing {
			t.Errorf("python -m pytest output %q should trigger runner_missing", output)
		}
		if bin != "pytest" {
			t.Errorf("missing binary = %q, want pytest", bin)
		}
	}
}

// TestDetectRunnerMissing_PythonInterpreterMissing covers the
// inverse: the python interpreter itself isn't on PATH.
func TestDetectRunnerMissing_PythonInterpreterMissing(t *testing.T) {
	for _, output := range []string{
		"/bin/sh: 1: python: not found\n",
		"bash: python3: command not found\n",
	} {
		missing, _, _ := detectRunnerMissing("python", nil, output)
		if !missing {
			t.Errorf("python interpreter missing output %q should trigger runner_missing", output)
		}
	}
}

// TestRunnerMissingReport_Bilingual locks the language gating: zh
// (default) emits Chinese FailureSummary; en flips to English.
func TestRunnerMissingReport_Bilingual(t *testing.T) {
	zh := makeRunnerMissingReport("python", "pytest", "stderr", "zh", "pattern: No module named pytest", 1)
	if !strings.Contains(zh.FailureSummary, "未在本环境安装") {
		t.Errorf("zh report should contain Chinese phrasing; got %q", zh.FailureSummary)
	}
	if !strings.Contains(zh.FailureSummary, "exit=1") {
		t.Errorf("zh report should embed actual exit code; got %q", zh.FailureSummary)
	}
	en := makeRunnerMissingReport("python", "pytest", "stderr", "en", "exit_code_127", 127)
	if !strings.Contains(en.FailureSummary, "is not installed in this environment") {
		t.Errorf("en report should contain English phrasing; got %q", en.FailureSummary)
	}
	if !strings.Contains(en.FailureSummary, "exit=127") {
		t.Errorf("en report should embed actual exit code; got %q", en.FailureSummary)
	}
	if !strings.Contains(en.FailureSummary, "trigger=exit_code_127") {
		t.Errorf("en report should embed trigger reason; got %q", en.FailureSummary)
	}
}

// TestRunnerMissingToolResultSummary_EmbedsDiagnostic locks the
// model-facing summary contract: when verify reports runner_missing,
// the tool-result Summary the LLM sees MUST carry the actual exit
// code, the trigger signal, AND a stderr excerpt — not just an
// install hint. The customer-reported "verify says X is missing
// but I just installed it" case is only diagnosable when these
// three pieces are in front of the operator.
func TestRunnerMissingToolResultSummary_EmbedsDiagnostic(t *testing.T) {
	output := "/usr/bin/python3: No module named pytest\n"
	zh := runnerMissingToolResultSummary("zh", "python@.", "pytest",
		runnerInstallHint("python", "zh"), "pattern: No module named pytest", 1, output)
	for _, want := range []string{
		"子进程退出码=1", "触发信号=pattern: No module named pytest",
		"pytest", "No module named pytest",
	} {
		if !strings.Contains(zh, want) {
			t.Errorf("zh summary should contain %q; got %q", want, zh)
		}
	}
	en := runnerMissingToolResultSummary("en", "python@.", "pytest",
		runnerInstallHint("python", "en"), "exit_code_127", 127, "/bin/sh: pytest: not found\n")
	for _, want := range []string{
		"exit=127", "trigger=exit_code_127", "pytest: not found",
	} {
		if !strings.Contains(en, want) {
			t.Errorf("en summary should contain %q; got %q", want, en)
		}
	}
}

// TestTruncateForLog_Boundaries locks behaviour that the diagnostic
// log lines depend on: empty in → empty out, multi-line collapsed
// to single-line with pilcrow, oversize truncated with ellipsis,
// UTF-8 not split mid-rune.
func TestTruncateForLog_Boundaries(t *testing.T) {
	if got := truncateForLog("", 100); got != "" {
		t.Errorf("empty in: got %q", got)
	}
	if got := truncateForLog("  hello\nworld  ", 100); got != "hello ¶ world" {
		t.Errorf("multiline trim: got %q", got)
	}
	if got := truncateForLog(strings.Repeat("x", 50), 10); !strings.HasSuffix(got, "…") || len(got) > 14 {
		t.Errorf("oversize: got %q (len=%d)", got, len(got))
	}
	// 中 is 3 UTF-8 bytes. Cap of 4 bytes must back off rather than
	// slice mid-rune (which would produce invalid UTF-8 in the log).
	got := truncateForLog("中文测试", 4)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("UTF-8 truncation should end with ellipsis; got %q", got)
	}
	for _, r := range got {
		if r == 0xFFFD {
			t.Errorf("truncation produced invalid UTF-8: %q", got)
		}
	}
}
