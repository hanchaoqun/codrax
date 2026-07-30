//go:build !windows

package repl

import (
	"bufio"
	"time"

	"golang.org/x/sys/unix"
)

func (e *nativeLineInput) readByteWithTimeout(d time.Duration) (byte, bool, error) {
	return readByteFDWithTimeout(e.reader, e.fd, d)
}

// readByteFDWithTimeout is the shared buffered-first byte read with an
// fd-level wait (TTY-1/T-2): the bufio buffer wins before any kernel
// poll so shared-buffer residue can never be missed, and the run-phase
// input window reuses the same primitive for its tick loop.
func readByteFDWithTimeout(reader *bufio.Reader, fd int, d time.Duration) (byte, bool, error) {
	if reader.Buffered() > 0 {
		b, err := reader.ReadByte()
		return b, err == nil, err
	}
	// REGFIX-A #3 (sweep finding, high): select(2) is NOT restarted
	// under SA_RESTART on Linux or Darwin, so any signal the Go runtime
	// handles (a SIGWINCH burst while dragging a tmux pane is the
	// common case) surfaces as EINTR. Propagating it made the prompt
	// editor's ESC probe return an error, which the Loop treated as
	// end-of-session and printed "Goodbye!" — a resize storm could end
	// the user's session and discard the typed line. EINTR means
	// "nothing observed yet": retry within the remaining budget and
	// report a plain timeout when it runs out.
	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, false, nil
		}
		var set unix.FdSet
		set.Zero()
		set.Set(fd)
		tv := unix.NsecToTimeval(remaining.Nanoseconds())
		n, err := unix.Select(fd+1, &set, nil, nil, &tv)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, false, err
		}
		if n <= 0 || !set.IsSet(fd) {
			return 0, false, nil
		}
		break
	}
	b, err := reader.ReadByte()
	return b, err == nil, err
}
