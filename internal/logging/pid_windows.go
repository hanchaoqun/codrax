//go:build windows

package logging

import "syscall"

// Windows constants. We define them locally rather than relying on
// syscall package exports because the set of exported constants
// varies between Go releases. PROCESS_QUERY_LIMITED_INFORMATION needs
// only minimal rights, which means it succeeds even for processes
// owned by other users — important for the multi-user case where two
// people share an install directory.
const (
	processQueryLimitedInformation = 0x00001000
	stillActive                    = 259 // STILL_ACTIVE: GetExitCodeProcess result for live processes
)

// isPidAlive reports whether a process with the given PID currently
// exists. Used by the log retention sweeper to avoid deleting a log
// file that another running codrax instance is still appending to.
//
// The Windows path opens a query handle and asks for the exit code:
// STILL_ACTIVE (259) means the process has not yet exited. There is
// a documented edge case where a process that legitimately exited
// with code 259 looks identical to a live one, but for codrax that
// would require os.Exit(259) which we never call.
func isPidAlive(pid int) bool {
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
