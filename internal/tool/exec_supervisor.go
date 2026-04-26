package tool

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

// SupervisedRunOptions controls the resource caps applied to a
// supervised exec. Zero/empty values mean "no limit at this layer"
// — the supervisor still installs the process-group / JobObject
// boundary so context cancel reliably kills the entire descendant
// tree, even when no rlimit is asked for.
type SupervisedRunOptions struct {
	// MemoryLimitBytes caps total resident + virtual memory across
	// every process in the supervised tree. On Unix this maps to
	// `ulimit -v` + RLIMIT_AS via the shell prefix in the command
	// builder; on Windows it lands in JobObject's
	// JobMemoryLimit. Zero = no memory cap.
	MemoryLimitBytes uint64

	// CPULimitSeconds caps total CPU seconds across every process.
	// Unix: `ulimit -t` + RLIMIT_CPU. Windows: PerJobUserTimeLimit.
	// Zero = no CPU cap.
	CPULimitSeconds uint64
}

// SupervisedExitKind classifies the terminal state of a supervised
// run so callers can distinguish a normal failure (tests reported
// red) from a resource-exhaustion failure (OOM kill, CPU cap, wall
// timeout). Surfaces as types.FailureKind via the verify pipeline.
type SupervisedExitKind int

const (
	// SupervisedExitNormal — process exited on its own with whatever
	// exit code its framework chose. err is whatever exec.Cmd.Wait
	// returned (nil for code 0, *exec.ExitError otherwise).
	SupervisedExitNormal SupervisedExitKind = iota

	// SupervisedExitTimeout — wall-clock context deadline fired
	// before the process exited. Supervisor SIGKILLed the entire
	// process group / JobObject.
	SupervisedExitTimeout

	// SupervisedExitOOM — kernel OOM-killed a process in the tree
	// (Unix exit code 137 or signal SIGKILL with high RSS witnessed)
	// or the JobObject's memory limit fired. The verify retry-hint
	// prepends an explicit OOM advisory so the planner's next
	// dispatch avoids the failure mode.
	SupervisedExitOOM

	// SupervisedExitCPULimit — Unix RLIMIT_CPU (SIGXCPU) or Windows
	// PerJobUserTimeLimit fired. Distinct from Timeout: this means
	// the tree spent N CPU-seconds, not N wall-seconds.
	SupervisedExitCPULimit
)

// SupervisedResult packs the supervised exit information. ExitKind
// is authoritative for retry-hint classification; Output is the
// merged stdout+stderr captured by the caller's bytes.Buffer (we
// don't take a buffer here because every caller already manages its
// own — supervisor stays narrow on lifecycle + isolation).
type SupervisedResult struct {
	ExitKind SupervisedExitKind
	Err      error
}

// SupervisedRun runs cmd under a process-group / JobObject boundary
// so context cancel reliably kills every descendant. The platform
// implementation lives in exec_supervisor_unix.go (Setpgid + kill
// -pgid) and exec_supervisor_windows.go (CreateJobObject +
// AssignProcessToJobObject + TerminateJobObject).
//
// Caller contract:
//   - cmd.Start has NOT been called; supervisor calls it.
//   - cmd.Stdout / cmd.Stderr / cmd.Dir / cmd.Env are filled by the
//     caller before this entry point.
//   - opts.MemoryLimitBytes / CPULimitSeconds enforce resource caps
//     at the OS layer (JobObject on Windows; the shell-prefix
//     ulimit path on Unix is the caller's responsibility — see
//     applyUnixResourceCaps in run_tests.go's command builder).
//
// On context.DeadlineExceeded the supervisor SIGKILLs the entire
// tree and returns ExitKind=SupervisedExitTimeout. On a Linux OOM
// kill (exit code 137 for the root + the cgroup memory pressure
// witnessed via /proc/self/stat) the supervisor returns
// ExitKind=SupervisedExitOOM. Windows JobObject memory-limit
// triggers an explicit notification we map to OOM.
func SupervisedRun(ctx context.Context, cmd *exec.Cmd, opts SupervisedRunOptions) SupervisedResult {
	if cmd == nil {
		return SupervisedResult{ExitKind: SupervisedExitNormal, Err: errors.New("supervisor: nil cmd")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return supervisedRunPlatform(ctx, cmd, opts)
}

// killWaitTimeout caps how long the supervisor waits for cmd.Wait
// after issuing a tree-wide SIGKILL. SIGKILL is uninterruptible so
// in practice this never elapses; it's a defence against pathological
// kernel states (uninterruptible D-state on a hung NFS mount, etc.).
const killWaitTimeout = 10 * time.Second

// waitWithKillTimeout drains cmd.Wait but bounds the wait so a
// stuck D-state child doesn't deadlock the supervisor. Returns the
// Wait error if the process exited within killWaitTimeout, else
// nil (and the cmd's resources may leak — accepted because we've
// already SIGKILLed the tree and cgroup cleanup will handle it).
func waitWithKillTimeout(cmd *exec.Cmd) error {
	done := make(chan error, 1)
	var once sync.Once
	go func() {
		err := cmd.Wait()
		once.Do(func() { done <- err })
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(killWaitTimeout):
		return nil
	}
}
