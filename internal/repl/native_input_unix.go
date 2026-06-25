//go:build !windows

package repl

import (
	"time"

	"golang.org/x/sys/unix"
)

func (e *nativeLineInput) readByteWithTimeout(d time.Duration) (byte, bool, error) {
	if e.reader.Buffered() > 0 {
		b, err := e.reader.ReadByte()
		return b, err == nil, err
	}
	var set unix.FdSet
	set.Zero()
	set.Set(e.fd)
	tv := unix.NsecToTimeval(d.Nanoseconds())
	n, err := unix.Select(e.fd+1, &set, nil, nil, &tv)
	if err != nil {
		return 0, false, err
	}
	if n <= 0 || !set.IsSet(e.fd) {
		return 0, false, nil
	}
	b, err := e.reader.ReadByte()
	return b, err == nil, err
}
