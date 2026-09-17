//go:build !windows

package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The leaf itself announces readiness after it is running, so cancellation
// does not depend on guessing whether a shell has forked its next command.
func TestSupervisedRunCommandContextTreeCancellationB1715(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"command_context_only", "same_context", "plain_command"} {
		t.Run(mode, func(t *testing.T) {
			caller, cancel := context.WithCancel(context.Background())
			defer cancel()
			supervisorContext := context.Background()
			if mode != "command_context_only" {
				supervisorContext = caller
			}
			script := shellQuoteWord(binary) + " -test.run='^TestSupervisedRunCancellationLeafB1715$' & wait"
			var cmd *exec.Cmd
			if mode == "plain_command" {
				cmd = exec.Command("/bin/sh", "-c", script)
			} else {
				cmd = exec.CommandContext(caller, "/bin/sh", "-c", script)
			}
			cmd.Env = append(os.Environ(), "CODRAX_B1715_SUPERVISOR_LEAF=1")
			output := &safeBuffer{}
			cmd.Stdout, cmd.Stderr = output, output
			done := make(chan SupervisedResult, 1)
			go func() { done <- SupervisedRun(supervisorContext, cmd, SupervisedRunOptions{}) }()
			var leafPID int
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if line := strings.TrimSpace(output.Snapshot()); strings.HasPrefix(line, "leaf-ready:") {
					leafPID, err = strconv.Atoi(strings.TrimPrefix(line, "leaf-ready:"))
					if err != nil || leafPID <= 0 {
						t.Fatalf("invalid leaf readiness: %q, %v", line, err)
					}
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if leafPID <= 0 {
				cancel()
				t.Fatalf("leaf process never became ready: %q", output.Snapshot())
			}
			// Cleanup is only a test safety net. The assertion below records a
			// failure before it intervenes in a broken production supervisor.
			completed := false
			defer func() {
				if !completed {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
			}()
			canceledAt := time.Now()
			cancel()
			var result SupervisedResult
			select {
			case result = <-done:
			case <-time.After(2 * time.Second):
				t.Errorf("%s cancellation left a ready leaf holding output pipes open", mode)
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				select {
				case result = <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("supervisor did not return even after test cleanup")
				}
			}
			completed = true
			if result.Err == nil {
				t.Errorf("interrupted command returned a successful result: %+v", result)
			}
			if result.ExitKind != SupervisedExitNormal {
				t.Errorf("caller cancellation became resource exhaustion: %+v", result)
			}
			if mode == "command_context_only" && supervisorContext.Err() != nil {
				t.Fatal("fixture accidentally canceled the supervisor context")
			}
			if mode == "plain_command" && cmd.Cancel != nil {
				t.Error("plain exec.Command acquired a forbidden CommandContext callback")
			}
			// Retained bytes are not termination evidence: independently check
			// the reported live leaf PID, allowing the OS to reap a killed child.
			deadline = time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				err = syscall.Kill(leafPID, 0)
				if errors.Is(err, syscall.ESRCH) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !errors.Is(err, syscall.ESRCH) {
				t.Errorf("ready leaf %d remains after cancellation (%v): %v", leafPID, time.Since(canceledAt), err)
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		})
	}
}

func TestSupervisedRunCancellationLeafB1715(t *testing.T) {
	if os.Getenv("CODRAX_B1715_SUPERVISOR_LEAF") != "1" {
		return
	}
	fmt.Printf("leaf-ready:%d\n", os.Getpid())
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func TestSupervisedRunPreCanceledPlainCommandB1715(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "command-started")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := exec.Command("/bin/sh", "-c", "printf started > "+shellQuoteWord(sentinel))
	result := SupervisedRun(ctx, cmd, SupervisedRunOptions{})
	if cmd.Process != nil || cmd.Cancel != nil {
		t.Errorf("pre-canceled plain command was started or given a context callback: process=%v callback=%t", cmd.Process, cmd.Cancel != nil)
	}
	if !errors.Is(result.Err, context.Canceled) || result.ExitKind != SupervisedExitNormal {
		t.Errorf("pre-canceled command result = %+v", result)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Errorf("pre-canceled command wrote its sentinel: %v", err)
	}
}

func TestSupervisedRunProcessGroupInvalidPIDB1715(t *testing.T) {
	for _, cmd := range []*exec.Cmd{nil, {}, {Process: &os.Process{}}, {Process: &os.Process{Pid: -1}}} {
		if err := killSupervisedUnixProcessGroup(cmd); !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("missing/non-positive process identity must never become a process-group signal: %v", err)
		}
	}
}

func TestB1715UnixInterruptedResultNeverPasses(t *testing.T) {
	for _, contextErr := range []error{context.Canceled, context.DeadlineExceeded} {
		for _, waitErr := range []error{nil, errors.New("actual interrupted command exit")} {
			result := supervisedUnixInterruptedResult(contextErr, waitErr)
			if result.Err == nil || !errors.Is(result.Err, contextErr) || (waitErr != nil && !errors.Is(result.Err, waitErr)) {
				t.Errorf("interruption borrowed a clean exit or lost the actual wait result: %+v", result)
			}
			wantKind := SupervisedExitNormal
			if errors.Is(contextErr, context.DeadlineExceeded) {
				wantKind = SupervisedExitTimeout
			}
			if result.ExitKind != wantKind {
				t.Errorf("interruption kind=%v, want %v", result.ExitKind, wantKind)
			}
		}
	}
}
