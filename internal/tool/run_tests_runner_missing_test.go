package tool

import (
	"errors"
	"os/exec"
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
	missing, bin := detectRunnerMissing("python", runErr, "")
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
		missing, bin := detectRunnerMissing(tc.runner, nil, tc.output)
		if !missing {
			t.Errorf("%s: pattern %q should trigger detection", tc.runner, tc.output)
		}
		if bin != tc.wantBinary {
			t.Errorf("%s: bin = %q, want %q", tc.runner, bin, tc.wantBinary)
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
	missing, _ := detectRunnerMissing("python", nil, output)
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
	missing, _ := detectRunnerMissing("ruby", wrapped, "")
	if !missing {
		t.Errorf("exec.ErrNotFound should trigger detection")
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
	missing, _ := detectRunnerMissing("does-not-exist", nil, "anything: command not found\n")
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
	report := makeRunnerMissingReport("python", "pytest", "/bin/sh: 1: pytest: not found")
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
// drift-guard test.
func TestRunnerInstallHint_AllRunnersCovered(t *testing.T) {
	for r := range allowedRunners {
		hint := runnerInstallHint(r)
		if strings.TrimSpace(hint) == "" {
			t.Errorf("runner %q has no install hint — add one in runnerInstallHint", r)
		}
	}
}
