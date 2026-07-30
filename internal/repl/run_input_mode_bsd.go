//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package repl

import "golang.org/x/sys/unix"

const (
	runInputIoctlRead  = unix.TIOCGETA
	runInputIoctlWrite = unix.TIOCSETA
)

// applyRunInputMode is the cbreak transform for the RUN-phase input
// window (regression fix for the customer-reported dock corruption,
// design ledger docs/design/tty_single_stdin_owner_20260729.md §5):
// clear ECHO|ICANON only — byte-at-a-time reads without echo — while
// DELIBERATELY KEEPING
//   - OPOST/ONLCR: the renderer keeps writing \n-terminated lines and
//     the dock keeps repainting with \x1b[4A while the run is live;
//     full raw mode (term.MakeRaw clears OPOST) staircases that output
//     and corrupts the dock's rewind arithmetic — dock rows leak into
//     permanent scrollback exactly as the customer report showed;
//   - ISIG: Ctrl+C during a run stays a REAL SIGINT for the one
//     installed handler, byte-identical with the pre-window behavior.
func applyRunInputMode(t *unix.Termios) {
	t.Lflag &^= unix.ECHO | unix.ICANON
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
}

// makeRunInputMode switches the terminal into the run-window cbreak
// mode and returns the restore func.
func makeRunInputMode(fd int) (func(), error) {
	old, err := unix.IoctlGetTermios(fd, runInputIoctlRead)
	if err != nil {
		return nil, err
	}
	next := *old
	applyRunInputMode(&next)
	if err := unix.IoctlSetTermios(fd, runInputIoctlWrite, &next); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(fd, runInputIoctlWrite, old) }, nil
}
