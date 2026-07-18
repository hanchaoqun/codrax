//go:build !unix && !windows

package hitraceconv

import (
	"errors"
	"os/exec"
)

type externalToolProcessSupervisor struct{}

func newExternalToolProcessSupervisor(*exec.Cmd) (*externalToolProcessSupervisor, error) {
	return nil, errors.New("external tool process-tree supervision is unsupported on this platform")
}

func (*externalToolProcessSupervisor) afterStart(*exec.Cmd) error { return nil }
func (*externalToolProcessSupervisor) terminate() error           { return nil }
func (*externalToolProcessSupervisor) close() error               { return nil }
