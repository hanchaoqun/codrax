//go:build !windows

package tool

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// safeBuffer is a strings.Builder wrapped in a mutex so the test
// can read snapshot bytes (Snapshot()) concurrently with os/exec's
// stdout-copy goroutine writing to it. Without this the test
// races on strings.Builder, which is not safe for concurrent use.
type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) Snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestSupervisedRun_KillsGrandchildrenOnCancel is the regression test
// for the 2026-04-26 OOM event: pytest grandchild outliving the
// `sh -c` wrapper for 9 hours after the parent codrax process exited,
// because exec.CommandContext only SIGKILLs the direct child.
//
// Setup mirrors the failure shape: the shell command is `sh -c
// "(sleep 100) & wait $!"` so the supervisor sees one direct child
// (sh) and one grandchild (sleep). On context cancel, the supervisor
// must kill BOTH within killWaitTimeout. We verify the grandchild
// is gone by trying to signal it with sig 0 (probe) — ESRCH means
// it's dead, no error means it's still around.
func TestSupervisedRun_KillsGrandchildrenOnCancel(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	// Use bash's $$ + sleep to expose the grandchild's PID via stdout
	// so the test can probe it. `bash -c 'sleep 100 & echo $!; wait'`
	// prints the sleep PID and then waits for it. We then SIGKILL the
	// supervisor via context cancel and verify sleep is gone.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("sh", "-c", "sleep 100 & echo $!; wait")
	stdout := &safeBuffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stdout

	resultCh := make(chan SupervisedResult, 1)
	go func() {
		resultCh <- SupervisedRun(ctx, cmd, SupervisedRunOptions{})
	}()

	// Wait for the sleep PID to appear on stdout — the grandchild has
	// to have started before we cancel, otherwise the test races.
	deadline := time.Now().Add(5 * time.Second)
	var sleepPid int
	for time.Now().Before(deadline) {
		out := strings.TrimSpace(stdout.Snapshot())
		if out != "" {
			pid, err := strconv.Atoi(strings.Fields(out)[0])
			if err == nil && pid > 0 {
				sleepPid = pid
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sleepPid == 0 {
		t.Fatalf("grandchild sleep never reported its PID; stdout=%q", stdout.Snapshot())
	}

	// Cancel: supervisor should fire syscall.Kill(-pgid, SIGKILL) and
	// the grandchild should die.
	cancel()

	// Wait for SupervisedRun to return.
	select {
	case res := <-resultCh:
		if res.ExitKind != SupervisedExitNormal {
			// Cancel from caller is classified as Normal in the Unix
			// supervisor (see classifyUnixExit comment). A Timeout
			// here would indicate the supervisor misclassified
			// caller-cancel as deadline-cancel.
			t.Logf("supervisor returned ExitKind=%d err=%v (expected Normal for caller-cancel)", res.ExitKind, res.Err)
		}
	case <-time.After(killWaitTimeout + 2*time.Second):
		t.Fatal("SupervisedRun did not return within killWaitTimeout after cancel")
	}

	// Probe the grandchild PID. ESRCH = process gone. EPERM is also
	// acceptable (the kernel reused the PID for someone else by now,
	// which means our sleep is dead).
	deadline = time.Now().Add(killWaitTimeout)
	for time.Now().Before(deadline) {
		err := syscall.Kill(sleepPid, 0)
		if err == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.EPERM) {
			return // grandchild gone — supervisor did its job
		}
		t.Fatalf("unexpected probe error on grandchild PID %d: %v", sleepPid, err)
	}
	t.Fatalf("grandchild sleep PID %d still alive %v after cancel — process-group teardown leaked", sleepPid, killWaitTimeout)
}

// TestSupervisedRun_TimeoutClassified verifies the wall-clock
// timeout path produces ExitKind=SupervisedExitTimeout, distinct from
// caller-cancel (Normal) and OOM-from-signal-9 (OOM).
func TestSupervisedRun_TimeoutClassified(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	cmd := exec.Command("sh", "-c", "sleep 5")
	res := SupervisedRun(ctx, cmd, SupervisedRunOptions{})
	if res.ExitKind != SupervisedExitTimeout {
		t.Errorf("expected ExitKind=SupervisedExitTimeout, got %d (err=%v)", res.ExitKind, res.Err)
	}
}

// TestSupervisedRun_NormalExitClassified verifies a clean exit (code
// 0, no signal) produces ExitKind=SupervisedExitNormal so a passing
// test suite isn't misclassified as OOM.
func TestSupervisedRun_NormalExitClassified(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.Command("sh", "-c", "true")
	res := SupervisedRun(ctx, cmd, SupervisedRunOptions{})
	if res.ExitKind != SupervisedExitNormal {
		t.Errorf("expected ExitKind=SupervisedExitNormal, got %d (err=%v)", res.ExitKind, res.Err)
	}
	if res.Err != nil {
		t.Errorf("expected no error from `true`, got %v", res.Err)
	}
}

// TestWrapShellCommandWithCaps_UnixPrefixesUlimit verifies the Unix
// shell-prefix path produces the documented `ulimit -v` and
// `ulimit -t` wrappers. Catches a regression where the prefix was
// accidentally dropped or computed wrong.
func TestWrapShellCommandWithCaps_UnixPrefixesUlimit(t *testing.T) {
	wrapped := wrapShellCommandWithCaps("pytest --json-report", SupervisedRunOptions{
		MemoryLimitBytes: 2 * 1024 * 1024 * 1024,
		CPULimitSeconds:  600,
	})
	if !strings.Contains(wrapped, "ulimit -v 2097152") {
		t.Errorf("expected `ulimit -v 2097152` (2 GiB in KB) prefix; got %q", wrapped)
	}
	if !strings.Contains(wrapped, "ulimit -t 600") {
		t.Errorf("expected `ulimit -t 600` prefix; got %q", wrapped)
	}
	if !strings.HasSuffix(wrapped, "pytest --json-report") {
		t.Errorf("expected original command to be the suffix; got %q", wrapped)
	}
}

// TestWrapShellCommandWithCaps_NoOpWhenZero verifies a zero-cap
// (operator opted out) leaves the command untouched. Without this the
// shell prefix would inject `ulimit -v 0` which sets a zero address-
// space cap and instantly kills the command.
func TestWrapShellCommandWithCaps_NoOpWhenZero(t *testing.T) {
	wrapped := wrapShellCommandWithCaps("pytest", SupervisedRunOptions{})
	if wrapped != "pytest" {
		t.Errorf("zero caps should leave command untouched; got %q", wrapped)
	}
}
