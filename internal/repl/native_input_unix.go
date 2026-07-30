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
	var set unix.FdSet
	set.Zero()
	set.Set(fd)
	tv := unix.NsecToTimeval(d.Nanoseconds())
	n, err := unix.Select(fd+1, &set, nil, nil, &tv)
	if err != nil {
		return 0, false, err
	}
	if n <= 0 || !set.IsSet(fd) {
		return 0, false, nil
	}
	b, err := reader.ReadByte()
	return b, err == nil, err
}
