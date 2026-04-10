//go:build !windows

package memory

import "syscall"

// pidAlive reports whether a process with the given PID currently
// exists. Used by LivePeerCount to ignore stale sidecar files left
// behind by crashed peer Stores. Same idiom as
// internal/logging.isPidAlive — duplicated rather than shared
// because the two functions are four lines apiece and a third
// package would be more ceremony than the duplication saves.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}
