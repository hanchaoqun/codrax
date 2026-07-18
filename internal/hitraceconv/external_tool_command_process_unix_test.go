//go:build unix

package hitraceconv

import (
	"errors"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

func externalToolSupervisorTestIgnoreGracefulSignal() {
	signal.Ignore(syscall.SIGTERM)
}

func externalToolSupervisorTestProcessAlive(pid int) bool {
	err := unix.Kill(pid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}
