//go:build windows

package memory

import "syscall"

// pidAlive reports whether a process with the given PID currently
// exists. Used by LivePeerCount to ignore stale sidecar files left
// behind by crashed peer Stores.
const (
	processQueryLimitedInformation = 0x00001000
	stillActive                    = 259
)

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
