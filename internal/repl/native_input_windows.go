//go:build windows

package repl

import (
	"bufio"
	"time"

	"golang.org/x/sys/windows"
)

func (e *nativeLineInput) readByteWithTimeout(d time.Duration) (byte, bool, error) {
	return readByteFDWithTimeout(e.reader, e.fd, d)
}

// readByteFDWithTimeout is the shared buffered-first byte read with a
// handle-level wait (TTY-1/T-2); see the unix twin for the contract.
func readByteFDWithTimeout(reader *bufio.Reader, fd int, d time.Duration) (byte, bool, error) {
	if reader.Buffered() > 0 {
		b, err := reader.ReadByte()
		return b, err == nil, err
	}
	timeout := uint32(0)
	if d > 0 {
		timeout = uint32((d + time.Millisecond - 1) / time.Millisecond)
		if timeout == 0 {
			timeout = 1
		}
	}
	event, err := windows.WaitForSingleObject(windows.Handle(uintptr(fd)), timeout)
	if err != nil {
		return 0, false, err
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		return 0, false, nil
	}
	if event != windows.WAIT_OBJECT_0 {
		return 0, false, nil
	}
	b, err := reader.ReadByte()
	return b, err == nil, err
}
