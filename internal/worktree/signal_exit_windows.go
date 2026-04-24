//go:build windows

package worktree

import "os"

// terminateProcessAfterCleanup cannot re-raise POSIX signals on
// Windows, so preserve the user-visible semantics with the matching
// shell exit code after cleanup completes.
func terminateProcessAfterCleanup(sig os.Signal) {
	os.Exit(signalExitCode(sig))
}
