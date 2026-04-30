//go:build windows

package tool

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"unsafe"
)

// Windows kernel32.dll bindings for JobObject. Same LazyDLL pattern
// as internal/memory/lock_windows.go to keep the dependency surface
// at "stdlib only" (the project's no-x/sys rule).
//
// Why JobObject is the right primitive:
//   - JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE: when our last handle to
//     the job closes (process exit, or explicit CloseHandle), every
//     process inside the job dies. This survives codrax crashes —
//     the OS itself does the cleanup, so a pytest grandchild never
//     outlives the supervisor.
//   - JOB_OBJECT_LIMIT_PROCESS_MEMORY / JOB_MEMORY: per-process or
//     per-job memory cap. We use JOB to cap the entire descendant
//     tree, which is the right semantics (one runaway test fixture
//     forking 8 workers shouldn't be able to allocate 8× the cap).
//   - JOB_OBJECT_LIMIT_JOB_TIME: per-job CPU time cap. Same scope
//     reasoning.
var (
	supExecKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW             = supExecKernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject      = supExecKernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject     = supExecKernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject           = supExecKernel32.NewProc("TerminateJobObject")
	procOpenProcessSup               = supExecKernel32.NewProc("OpenProcess")
	procCloseHandleSup               = supExecKernel32.NewProc("CloseHandle")
	procGetExitCodeProcessSup        = supExecKernel32.NewProc("GetExitCodeProcess")
)

// Windows constants. Lifted from MSDN; stdlib syscall package does
// not export them in Go 1.22.
const (
	jobObjectExtendedLimitInformation       = 9
	jobObjectLimitProcessMemory             = 0x00000100
	jobObjectLimitJobMemory                 = 0x00000200
	jobObjectLimitJobTime                   = 0x00000004
	jobObjectLimitKillOnJobClose            = 0x00002000
	jobObjectLimitDieOnUnhandledException   = 0x00000400
	processAllAccess                        = 0x1F0FFF
	stillActive                             = 259
	winSig9SuggestsKilledExitCode           = 1
)

// IO_COUNTERS — unused by us but required to round out the struct.
type winIOCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

// JOBOBJECT_BASIC_LIMIT_INFORMATION
type winJobBasicLimit struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

// JOBOBJECT_EXTENDED_LIMIT_INFORMATION
type winJobExtendedLimit struct {
	BasicLimitInformation winJobBasicLimit
	IoInfo                winIOCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// supervisedRunPlatform is the Windows implementation. Same contract
// as the Unix sibling: install a kill-on-close JobObject, assign the
// child immediately after Start, and on context cancel call
// TerminateJobObject so every descendant dies.
//
// Race window: between cmd.Start() and AssignProcessToJobObject() the
// child is already running and could fork. In practice the root child
// is `sh -c` / `cmd /C`, and the test framework underneath only forks
// AFTER it executes — by which point Assign has already run. We don't
// use CREATE_SUSPENDED because Go's os/exec doesn't expose the main
// thread handle needed to ResumeThread, and reaching it would require
// toolhelp32 enumeration which is fragile.
func supervisedRunPlatform(ctx context.Context, cmd *exec.Cmd, opts SupervisedRunOptions) SupervisedResult {
	jobHandle, err := createSupervisorJob(opts)
	if err != nil {
		// JobObject creation failed; fall back to unsupervised exec
		// rather than refusing to run. Log channel is upstream.
		return runUnsupervisedFallback(ctx, cmd)
	}
	defer closeWinHandle(jobHandle)

	if err := cmd.Start(); err != nil {
		return SupervisedResult{ExitKind: SupervisedExitNormal, Err: err}
	}

	if err := assignProcessToSupervisorJob(jobHandle, uint32(cmd.Process.Pid)); err != nil {
		// Failed to assign — the child has already started and is
		// not in our job. Best effort: cancel ctx via Kill, return
		// whatever the unsupervised path yields. The child may
		// outlive us in this case but we explicitly logged the
		// failure upstream.
		_ = cmd.Process.Kill()
		// Inline single-goroutine wait for the assignment-failure
		// branch (no shared waitErr exists yet here).
		failedWait := make(chan error, 1)
		go func() { failedWait <- cmd.Wait() }()
		_ = waitForExistingWait(failedWait)
		return SupervisedResult{ExitKind: SupervisedExitNormal, Err: err}
	}

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_, _, _ = procTerminateJobObject.Call(uintptr(jobHandle), uintptr(1))
		err := waitForExistingWait(waitErr)
		kind := SupervisedExitTimeout
		if errors.Is(ctx.Err(), context.Canceled) {
			kind = SupervisedExitNormal
		}
		// JobObject memory / CPU triggers raise STATUS_QUOTA_EXCEEDED
		// (0xC0000044) — distinguishable but we keep the simpler
		// timeout/normal split here. Memory-classified Windows OOM
		// would need GetExitCodeProcess + STATUS_QUOTA_EXCEEDED match.
		return SupervisedResult{ExitKind: kind, Err: err}
	case err := <-waitErr:
		return SupervisedResult{ExitKind: classifyWindowsExit(cmd, err), Err: err}
	}
}

// createSupervisorJob assembles a JobObject with the supplied limits
// and returns its handle. Caller MUST CloseHandle on the returned
// value (KILL_ON_JOB_CLOSE in the limits then triggers tree
// teardown). Returns syscall.Handle(0) and the underlying error on
// failure.
func createSupervisorJob(opts SupervisedRunOptions) (syscall.Handle, error) {
	r1, _, e1 := procCreateJobObjectW.Call(0, 0)
	if r1 == 0 {
		if e1 != nil {
			return 0, e1
		}
		return 0, errors.New("CreateJobObjectW failed")
	}
	job := syscall.Handle(r1)

	limits := winJobExtendedLimit{}
	limits.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose | jobObjectLimitDieOnUnhandledException

	if opts.MemoryLimitBytes > 0 {
		limits.BasicLimitInformation.LimitFlags |= jobObjectLimitJobMemory
		limits.JobMemoryLimit = uintptr(opts.MemoryLimitBytes)
	}
	if opts.CPULimitSeconds > 0 {
		limits.BasicLimitInformation.LimitFlags |= jobObjectLimitJobTime
		// PerJobUserTimeLimit is in 100-ns units (FILETIME convention).
		limits.BasicLimitInformation.PerJobUserTimeLimit = int64(opts.CPULimitSeconds) * 10_000_000
	}

	r1, _, e1 = procSetInformationJobObject.Call(
		uintptr(job),
		uintptr(jobObjectExtendedLimitInformation),
		uintptr(unsafe.Pointer(&limits)),
		unsafe.Sizeof(limits),
	)
	if r1 == 0 {
		closeWinHandle(job)
		if e1 != nil {
			return 0, e1
		}
		return 0, errors.New("SetInformationJobObject failed")
	}
	return job, nil
}

// assignProcessToSupervisorJob opens the target process by PID and
// adds it to the job. We re-open here because Go's os.Process keeps
// its HANDLE private — OpenProcess gives us a fresh one which we
// close immediately after the assign call (the job retains its own
// reference to the process).
func assignProcessToSupervisorJob(job syscall.Handle, pid uint32) error {
	r1, _, e1 := procOpenProcessSup.Call(uintptr(processAllAccess), 0, uintptr(pid))
	if r1 == 0 {
		if e1 != nil {
			return e1
		}
		return errors.New("OpenProcess failed")
	}
	procHandle := syscall.Handle(r1)
	defer closeWinHandle(procHandle)

	r1, _, e1 = procAssignProcessToJobObject.Call(uintptr(job), uintptr(procHandle))
	if r1 == 0 {
		if e1 != nil {
			return e1
		}
		return errors.New("AssignProcessToJobObject failed")
	}
	return nil
}

// classifyWindowsExit maps a Wait error onto SupervisedExitKind.
// On Windows there's no SIGKILL / SIGXCPU split — the JobObject
// surfaces resource exhaustion as STATUS_QUOTA_EXCEEDED (0xC0000044)
// in the process exit code. We probe via GetExitCodeProcess after
// Wait returns.
func classifyWindowsExit(cmd *exec.Cmd, err error) SupervisedExitKind {
	if err == nil {
		return SupervisedExitNormal
	}
	if cmd.Process == nil {
		return SupervisedExitNormal
	}
	r1, _, _ := procOpenProcessSup.Call(uintptr(processAllAccess), 0, uintptr(cmd.Process.Pid))
	if r1 == 0 {
		// Process already cleaned up — can't probe. Default to Normal.
		return SupervisedExitNormal
	}
	procHandle := syscall.Handle(r1)
	defer closeWinHandle(procHandle)

	var exitCode uint32
	r1, _, _ = procGetExitCodeProcessSup.Call(uintptr(procHandle), uintptr(unsafe.Pointer(&exitCode)))
	if r1 == 0 {
		return SupervisedExitNormal
	}
	if exitCode == stillActive {
		return SupervisedExitNormal
	}
	// STATUS_QUOTA_EXCEEDED — JobObject memory/CPU limit fired.
	if exitCode == 0xC0000044 {
		return SupervisedExitOOM
	}
	return SupervisedExitNormal
}

// runUnsupervisedFallback runs cmd without a JobObject when
// CreateJobObjectW failed (extremely rare — usually only on
// kernel32.dll being missing, e.g. in some sandboxed CI containers).
// The caller's context.WithTimeout still applies via
// exec.CommandContext default Kill, so timeout works even if
// imperfectly (descendants may leak).
func runUnsupervisedFallback(ctx context.Context, cmd *exec.Cmd) SupervisedResult {
	if err := cmd.Start(); err != nil {
		return SupervisedResult{ExitKind: SupervisedExitNormal, Err: err}
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		err := waitForExistingWait(waitErr)
		kind := SupervisedExitTimeout
		if errors.Is(ctx.Err(), context.Canceled) {
			kind = SupervisedExitNormal
		}
		return SupervisedResult{ExitKind: kind, Err: err}
	case err := <-waitErr:
		return SupervisedResult{ExitKind: SupervisedExitNormal, Err: err}
	}
}

// closeWinHandle wraps CloseHandle for the deferred cleanup paths.
// Errors here are unrecoverable (we're already exiting) so we drop
// them silently.
func closeWinHandle(h syscall.Handle) {
	if h == 0 {
		return
	}
	_, _, _ = procCloseHandleSup.Call(uintptr(h))
}
