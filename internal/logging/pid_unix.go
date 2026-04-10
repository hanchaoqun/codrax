//go:build !windows

package logging

import "syscall"

// isPidAlive reports whether a process with the given PID currently
// exists. It is used by the log retention sweeper to avoid deleting a
// log file that another running codrax instance is still appending to.
//
// On Unix the canonical idiom is sending signal 0: it performs the
// existence + permission check without delivering anything. ESRCH
// means "no such process"; EPERM means "exists but we lack
// permission", which still counts as alive for our purposes.
//
// Zombie processes (terminated but not yet reaped by their parent)
// will report as alive here. That is fine — codrax processes are
// reaped by their parent shell quickly, and the worst case of a
// false positive is one log file lingering one cycle longer.
func isPidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}
