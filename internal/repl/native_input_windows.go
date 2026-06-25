//go:build windows

package repl

import (
	"time"

	"golang.org/x/sys/windows"
)

func (e *nativeLineInput) readByteWithTimeout(d time.Duration) (byte, bool, error) {
	if e.reader.Buffered() > 0 {
		b, err := e.reader.ReadByte()
		return b, err == nil, err
	}
	timeout := uint32(0)
	if d > 0 {
		timeout = uint32((d + time.Millisecond - 1) / time.Millisecond)
		if timeout == 0 {
			timeout = 1
		}
	}
	event, err := windows.WaitForSingleObject(windows.Handle(uintptr(e.fd)), timeout)
	if err != nil {
		return 0, false, err
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		return 0, false, nil
	}
	if event != windows.WAIT_OBJECT_0 {
		return 0, false, nil
	}
	b, err := e.reader.ReadByte()
	return b, err == nil, err
}
