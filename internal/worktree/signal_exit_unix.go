//go:build !windows

package worktree

import (
	"os"
	"syscall"
)

// terminateProcessAfterCleanup restores the default signal termination
// path after active sessions have been discarded, preserving the
// canonical shell status on Unix-like hosts.
func terminateProcessAfterCleanup(sig os.Signal) {
	if ssig, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(os.Getpid(), ssig)
	}
	os.Exit(signalExitCode(sig))
}
