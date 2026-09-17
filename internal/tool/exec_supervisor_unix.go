//go:build !windows

package tool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
)

// supervisedRunUnixOneShot establishes a private process group and sends a
// group-wide cancellation signal on Unix platforms other than Darwin.
// Descendants that leave the group or race the group snapshot are not proven
// terminated. Darwin uses a separately held group lease for repeated cleanup.
//
// Why this is the root-cause fix: exec.CommandContext's default
// cancel behaviour calls cmd.Process.Kill, which only SIGKILLs the
// direct child (the `sh -c` wrapper). Test harnesses (pytest, jest
// workers, cargo) fork further; those grandchildren become orphans
// when sh dies — reparented to PID 1, still alive, still allocating.
// The 9-hour 2.47-GiB pytest in the OOM event is the textbook
// instance of this leak.
func supervisedRunUnixOneShot(ctx context.Context, cmd *exec.Cmd, opts SupervisedRunOptions) SupervisedResult {
	if err := ctx.Err(); err != nil {
		return SupervisedResult{ExitKind: supervisedUnixContextExitKind(err), Err: err}
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	// Own a private group whose ID is the child PID, never a caller's group.
	cmd.SysProcAttr.Pgid = 0
	var commandContextInterrupted atomic.Bool
	if cmd.Cancel != nil {
		// CommandContext may carry a different context from the supervisor.
		// Keep its watcher, but make either cancellation path tear down the
		// same group instead of letting the default callback kill only the
		// launcher. Plain Command must keep Cancel nil (os/exec requires it).
		cmd.Cancel = func() error {
			commandContextInterrupted.Store(true)
			return killSupervisedUnixProcessGroup(cmd)
		}
	}

	if err := cmd.Start(); err != nil {
		return SupervisedResult{ExitKind: SupervisedExitNormal, Err: err}
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = killSupervisedUnixProcessGroup(cmd)
		// Wait on the SAME goroutine that already runs
		// cmd.Wait. Spawning a second cmd.Wait() raced (Go's
		// "Wait may not be called concurrently" rule); the fix
		// is to share the existing waitErr channel through
		// waitForExistingWait.
		err := waitForExistingWait(waitErr)
		return supervisedUnixInterruptedResult(ctx.Err(), err)
	case err := <-waitErr:
		// Both channels can become ready together. Preserve cancellation
		// classification, but do not signal a bare PGID after Wait reaped its
		// original leader: this implementation has no surviving group lease.
		if contextErr := ctx.Err(); contextErr != nil {
			return supervisedUnixInterruptedResult(contextErr, err)
		}
		if commandContextInterrupted.Load() {
			// Cmd's private context has no public error accessor. Its callback
			// proves interruption, not OOM, but cannot distinguish cancel from
			// deadline. Normal is a non-resource classification, not a pass.
			if err == nil {
				err = errors.New("supervisor: command context interrupted execution")
			}
			return SupervisedResult{ExitKind: SupervisedExitNormal, Err: err}
		}
		kind := classifyUnixExit(err)
		return SupervisedResult{ExitKind: kind, Err: err}
	}
}

func killSupervisedUnixProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

func supervisedUnixContextExitKind(err error) SupervisedExitKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return SupervisedExitTimeout
	}
	// Caller cancellation is not a resource-exhaustion/product diagnosis.
	return SupervisedExitNormal
}

func supervisedUnixInterruptedResult(contextErr, waitErr error) SupervisedResult {
	return SupervisedResult{ExitKind: supervisedUnixContextExitKind(contextErr), Err: errors.Join(contextErr, waitErr)}
}

// classifyUnixExit maps an exec.Cmd.Wait error onto a
// SupervisedExitKind. Two non-Normal cases the kernel surfaces:
//
//   - SIGKILL (signal 9, exit code 137 in shell convention) without
//     us cancelling — almost always the OOM killer. We classify as
//     SupervisedExitOOM so the verify retry-hint says "previous
//     plan triggered OOM". False positives possible (operator's
//     `kill -9`) but verify-mode is bounded to the worktree, so
//     external SIGKILL is rare and operator can re-read the report.
//
//   - SIGXCPU (signal 24) — RLIMIT_CPU exhausted. The shell-prefix
//     `ulimit -t` route in buildRunCommand triggers this. Classify
//     as SupervisedExitCPULimit so the planner sees "your tests
//     spun forever" rather than "tests failed".
func classifyUnixExit(err error) SupervisedExitKind {
	if err == nil {
		return SupervisedExitNormal
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return SupervisedExitNormal
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return SupervisedExitNormal
	}
	if ws.Signaled() {
		switch ws.Signal() {
		case syscall.SIGKILL:
			return SupervisedExitOOM
		case syscall.SIGXCPU:
			return SupervisedExitCPULimit
		}
	}
	// Shell-encoded "killed by signal N" exit codes (128 + N).
	if ws.ExitStatus() == 137 {
		return SupervisedExitOOM
	}
	if ws.ExitStatus() == 152 { // 128 + SIGXCPU
		return SupervisedExitCPULimit
	}
	return SupervisedExitNormal
}
