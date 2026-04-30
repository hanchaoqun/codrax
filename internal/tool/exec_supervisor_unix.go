//go:build !windows

package tool

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
)

// supervisedRunPlatform is the Unix implementation: Setpgid=true so
// every child + grandchild lives in the supervisor-owned process
// group, then on cancel we deliver SIGKILL to the negative pgid
// which the kernel translates into "every member of the group, no
// matter how nested".
//
// Why this is the root-cause fix: exec.CommandContext's default
// cancel behaviour calls cmd.Process.Kill, which only SIGKILLs the
// direct child (the `sh -c` wrapper). Test harnesses (pytest, jest
// workers, cargo) fork further; those grandchildren become orphans
// when sh dies — reparented to PID 1, still alive, still allocating.
// The 9-hour 2.47-GiB pytest in the OOM event is the textbook
// instance of this leak.
func supervisedRunPlatform(ctx context.Context, cmd *exec.Cmd, opts SupervisedRunOptions) SupervisedResult {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	if err := cmd.Start(); err != nil {
		return SupervisedResult{ExitKind: SupervisedExitNormal, Err: err}
	}
	pgid := cmd.Process.Pid

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		// Wait on the SAME goroutine that already runs
		// cmd.Wait. Spawning a second cmd.Wait() raced (Go's
		// "Wait may not be called concurrently" rule); the fix
		// is to share the existing waitErr channel through
		// waitForExistingWait.
		err := waitForExistingWait(waitErr)
		kind := SupervisedExitTimeout
		if errors.Is(ctx.Err(), context.Canceled) {
			// Caller cancelled (not a deadline) — preserve as Normal
			// so verify doesn't misclassify a graceful shutdown.
			kind = SupervisedExitNormal
		}
		return SupervisedResult{ExitKind: kind, Err: err}
	case err := <-waitErr:
		kind := classifyUnixExit(err)
		return SupervisedResult{ExitKind: kind, Err: err}
	}
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
