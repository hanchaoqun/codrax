//go:build unix

package cpulimit

import "syscall"

// setThreadNice raises the calling thread's scheduling nice value.
//
// On Linux the nice value is a per-thread attribute and
// setpriority(PRIO_PROCESS, 0, ...) adjusts the calling thread.
// Raising nice (lowering priority) never requires privilege, so this
// always succeeds for a target in [0, 19]; the error return is kept
// for the rare platform that rejects it.
func setThreadNice(nice int) error {
	return syscall.Setpriority(syscall.PRIO_PROCESS, 0, nice)
}
