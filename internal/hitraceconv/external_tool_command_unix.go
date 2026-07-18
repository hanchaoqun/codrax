//go:build unix

package hitraceconv

import (
	"errors"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const externalToolProcessGroupTERMGrace = 250 * time.Millisecond

type externalToolProcessSupervisor struct {
	pgid int
}

func newExternalToolProcessSupervisor(cmd *exec.Cmd) (*externalToolProcessSupervisor, error) {
	if cmd == nil {
		return nil, errors.New("external tool command is nil")
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return &externalToolProcessSupervisor{}, nil
}

func (supervisor *externalToolProcessSupervisor) afterStart(cmd *exec.Cmd) error {
	if supervisor == nil || cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return errors.New("started Unix child has no process identity")
	}
	supervisor.pgid = cmd.Process.Pid
	return nil
}

func (supervisor *externalToolProcessSupervisor) terminate() error {
	if supervisor == nil || supervisor.pgid <= 0 {
		return nil
	}
	pgid := supervisor.pgid
	if err := unix.Kill(-pgid, unix.SIGTERM); err != nil {
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		return err
	}
	deadline := time.Now().Add(externalToolProcessGroupTERMGrace)
	for time.Now().Before(deadline) {
		if err := unix.Kill(-pgid, 0); err != nil {
			if errors.Is(err, unix.ESRCH) {
				return nil
			}
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := unix.Kill(-pgid, unix.SIGKILL); err != nil && !errors.Is(err, unix.ESRCH) {
		return err
	}
	return nil
}

func (*externalToolProcessSupervisor) close() error {
	return nil
}
