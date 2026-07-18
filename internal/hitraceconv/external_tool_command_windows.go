//go:build windows

package hitraceconv

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	externalToolWindowsThreadDiscoveryTimeout = 250 * time.Millisecond
	externalToolWindowsJobExitTimeout         = 5 * time.Second
)

type externalToolWindowsJobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

type externalToolProcessSupervisor struct {
	job windows.Handle
}

func newExternalToolProcessSupervisor(cmd *exec.Cmd) (*externalToolProcessSupervisor, error) {
	if cmd == nil {
		return nil, errors.New("external tool command is nil")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(job))
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// The child must not execute before it belongs to the Job Object. Without
	// CREATE_SUSPENDED it could spawn an untracked descendant in the interval
	// between CreateProcess and AssignProcessToJobObject.
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	return &externalToolProcessSupervisor{job: job}, nil
}

func (supervisor *externalToolProcessSupervisor) afterStart(cmd *exec.Cmd) error {
	if supervisor == nil || supervisor.job == 0 || cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return errors.New("started Windows child has no Job Object authority")
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		return errors.Join(err, cmd.Process.Kill())
	}
	assignErr := windows.AssignProcessToJobObject(supervisor.job, process)
	closeErr := windows.CloseHandle(process)
	if assignErr != nil {
		return errors.Join(assignErr, closeErr, cmd.Process.Kill())
	}
	if closeErr != nil {
		return closeErr
	}
	return resumeExternalToolWindowsProcess(uint32(cmd.Process.Pid))
}

func (supervisor *externalToolProcessSupervisor) terminate() error {
	if supervisor == nil || supervisor.job == 0 {
		return nil
	}
	active, err := externalToolWindowsJobActiveProcesses(supervisor.job)
	if err != nil {
		return err
	}
	if active == 0 {
		return nil
	}
	if err := windows.TerminateJobObject(supervisor.job, 1); err != nil {
		return err
	}
	deadline := time.Now().Add(externalToolWindowsJobExitTimeout)
	for {
		active, err := externalToolWindowsJobActiveProcesses(supervisor.job)
		if err != nil {
			return err
		}
		if active == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("Windows Job Object retained %d active process(es) after termination", active)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (supervisor *externalToolProcessSupervisor) close() error {
	if supervisor == nil || supervisor.job == 0 {
		return nil
	}
	job := supervisor.job
	supervisor.job = 0
	return windows.CloseHandle(job)
}

func resumeExternalToolWindowsProcess(pid uint32) error {
	deadline := time.Now().Add(externalToolWindowsThreadDiscoveryTimeout)
	var lastErr error
	for {
		thread, found, err := findExternalToolWindowsProcessThread(pid)
		if err != nil {
			lastErr = err
		} else if found {
			previous, resumeErr := windows.ResumeThread(thread)
			closeErr := windows.CloseHandle(thread)
			if resumeErr != nil {
				return errors.Join(resumeErr, closeErr)
			}
			if previous != 1 {
				return errors.Join(
					fmt.Errorf("Windows child primary thread suspend count=%d, want 1", previous),
					closeErr,
				)
			}
			return closeErr
		}
		if !time.Now().Before(deadline) {
			if lastErr == nil {
				lastErr = errors.New("suspended Windows child thread was not found")
			}
			return lastErr
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func findExternalToolWindowsProcessThread(pid uint32) (windows.Handle, bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0, false, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return 0, false, nil
		}
		return 0, false, err
	}
	for {
		if entry.OwnerProcessID == pid {
			thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if err != nil {
				return 0, false, err
			}
			return thread, true, nil
		}
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return 0, false, nil
			}
			return 0, false, err
		}
	}
}

func externalToolWindowsJobActiveProcesses(job windows.Handle) (uint32, error) {
	var accounting externalToolWindowsJobAccounting
	var returned uint32
	if err := windows.QueryInformationJobObject(
		job,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&accounting)),
		uint32(unsafe.Sizeof(accounting)),
		&returned,
	); err != nil {
		return 0, err
	}
	if returned != 0 && returned < uint32(unsafe.Sizeof(accounting)) {
		return 0, fmt.Errorf("Windows Job Object accounting bytes=%d, want %d", returned, unsafe.Sizeof(accounting))
	}
	return accounting.ActiveProcesses, nil
}
