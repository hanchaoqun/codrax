package operation

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCommandExecutorRunsProgramArgs(t *testing.T) {
	executor := CommandExecutor{Policy: DefaultCommandPolicy(), OutputDir: t.TempDir()}
	plan := CommandOperationPlan{
		ID:           "op-test",
		Status:       StatusReady,
		ApprovalMode: ApprovalManual,
		WorkDir:      ".",
		Steps: []CommandStep{{
			ID:        "step-1",
			Program:   "go",
			Args:      []string{"version"},
			TimeoutMS: 30_000,
		}},
	}

	result := executor.Execute(context.Background(), plan)
	if result.Status != StatusExecuted {
		t.Fatalf("Status=%q result=%+v", result.Status, result)
	}
	if len(result.StepResults) != 1 || !strings.Contains(result.StepResults[0].OutputPreview, "go version") {
		t.Fatalf("unexpected output: %+v", result.StepResults)
	}
}

func TestCommandExecutorStopsOnFailure(t *testing.T) {
	executor := CommandExecutor{Policy: DefaultCommandPolicy(), OutputDir: t.TempDir()}
	plan := CommandOperationPlan{
		ID:           "op-fail",
		Status:       StatusReady,
		ApprovalMode: ApprovalManual,
		WorkDir:      ".",
		Steps: []CommandStep{{
			ID:        "step-1",
			Program:   "go",
			Args:      []string{"definitely-not-a-go-subcommand"},
			TimeoutMS: 30_000,
		}, {
			ID:      "step-2",
			Program: "go",
			Args:    []string{"version"},
		}},
	}

	result := executor.Execute(context.Background(), plan)
	if result.Status != StatusFailed {
		t.Fatalf("Status=%q result=%+v", result.Status, result)
	}
	if len(result.StepResults) != 1 {
		t.Fatalf("expected execution to stop after first failure: %+v", result.StepResults)
	}
	if result.StepResults[0].ExitCode == 0 {
		t.Fatalf("expected non-zero exit code: %+v", result.StepResults[0])
	}
}

func TestCommandExecutorWritesTruncatedOutputRef(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell quoting differs on Windows")
	}
	dir := t.TempDir()
	policy := DefaultCommandPolicy()
	policy.OutputPreviewBytes = 8
	executor := CommandExecutor{Policy: policy, OutputDir: dir}
	plan := CommandOperationPlan{
		ID:           "op-large",
		Status:       StatusReady,
		ApprovalMode: ApprovalManual,
		WorkDir:      ".",
		Steps: []CommandStep{{
			ID:        "step-1",
			Shell:     "printf 'abcdefghijklmnop'",
			TimeoutMS: 30_000,
		}},
	}

	result := executor.Execute(context.Background(), plan)
	if result.Status != StatusExecuted {
		t.Fatalf("Status=%q result=%+v", result.Status, result)
	}
	ref := result.StepResults[0].PayloadRef
	if ref == "" {
		t.Fatalf("expected payload ref for truncated output: %+v", result.StepResults[0])
	}
	if !strings.HasPrefix(ref, dir) {
		t.Fatalf("payload ref %q not under output dir %q", ref, dir)
	}
	data, err := os.ReadFile(ref)
	if err != nil {
		t.Fatalf("read payload ref: %v", err)
	}
	if string(data) != "abcdefghijklmnop" {
		t.Fatalf("payload content=%q", string(data))
	}
	if filepath.Dir(ref) != dir {
		t.Fatalf("payload dir=%q want %q", filepath.Dir(ref), dir)
	}
}

func TestCommandExecutorTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on Windows")
	}
	executor := CommandExecutor{Policy: DefaultCommandPolicy(), OutputDir: t.TempDir()}
	plan := CommandOperationPlan{
		ID:           "op-timeout",
		Status:       StatusReady,
		ApprovalMode: ApprovalManual,
		WorkDir:      ".",
		Steps: []CommandStep{{
			ID:        "step-1",
			Shell:     "sleep 2",
			TimeoutMS: 50,
		}},
	}

	start := time.Now()
	result := executor.Execute(context.Background(), plan)
	if time.Since(start) > time.Second {
		t.Fatalf("timeout did not stop promptly: %s", time.Since(start))
	}
	if result.Status != StatusFailed {
		t.Fatalf("Status=%q result=%+v", result.Status, result)
	}
	if !strings.Contains(result.StepResults[0].Error, "timed out") {
		t.Fatalf("timeout error missing: %+v", result.StepResults[0])
	}
	if !result.StepResults[0].TimedOut {
		t.Fatalf("timeout marker missing: %+v", result.StepResults[0])
	}
}
