//go:build !windows

package repl

import (
	"os"
	"syscall"
)

// raiseSelfSIGINT converts the run-window's raw-mode Ctrl+C byte back
// into a process SIGINT so the ONE installed handler owns all cancel
// semantics (single tap = cancel, double tap = clean exit) — the byte
// path and the cooked-mode signal path stay a single implementation
// (TTY-2; design ledger docs/design/tty_single_stdin_owner_20260729.md).
func raiseSelfSIGINT() bool {
	return syscall.Kill(os.Getpid(), syscall.SIGINT) == nil
}
