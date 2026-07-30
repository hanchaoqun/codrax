//go:build linux

package repl

import "golang.org/x/sys/unix"

const (
	runInputIoctlRead  = unix.TCGETS
	runInputIoctlWrite = unix.TCSETS
)

// See run_input_mode_bsd.go for the full contract: cbreak (ECHO|ICANON
// cleared), OPOST and ISIG deliberately kept so the dock's repaint
// arithmetic and the SIGINT handler behave exactly as before the run
// window existed.
func applyRunInputMode(t *unix.Termios) {
	t.Lflag &^= unix.ECHO | unix.ICANON
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
}

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
