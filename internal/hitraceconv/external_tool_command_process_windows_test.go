//go:build windows

package hitraceconv

import "golang.org/x/sys/windows"

func externalToolSupervisorTestIgnoreGracefulSignal() {}

func externalToolSupervisorTestProcessAlive(pid int) bool {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	status, err := windows.WaitForSingleObject(process, 0)
	return err == nil && status == uint32(windows.WAIT_TIMEOUT)
}
