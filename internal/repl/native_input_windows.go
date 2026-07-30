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
//
// KNOWN LIMITATION (REGFIX-C #6, regression sweep 2026-07-30 — recorded
// rather than patched blind): a console input handle signals READY for
// ANY input record, including focus changes, mouse moves and
// screen-buffer resizes that yield no byte. The ReadByte below then
// blocks until a real key arrives, so a timed probe (the editor's 25ms
// ESC disambiguation) can overstay its budget on an idle-but-focused
// terminal. The clean fix is PeekConsoleInputW + ReadConsoleInputW to
// discard non-key records, which cannot be compile-verified from this
// (unix, CGO-bound) build host; per the postmortem's platform ruling we
// do NOT ship unverified Windows syscall code. Containment already in
// place: the TTY run-input window never arms on Windows
// (run_input_mode_other.go), so the wedge cannot reach the run phase
// where it froze the customer's REPL; the prompt lane retains the
// pre-existing behavior. The wait is sliced so the ready/timeout
// bookkeeping stays bounded and cancellable by the caller's budget.
func readByteFDWithTimeout(reader *bufio.Reader, fd int, d time.Duration) (byte, bool, error) {
	if reader.Buffered() > 0 {
		b, err := reader.ReadByte()
		return b, err == nil, err
	}
	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if d > 0 && remaining <= 0 {
			return 0, false, nil
		}
		slice := uint32(20)
		if d > 0 {
			ms := uint32((remaining + time.Millisecond - 1) / time.Millisecond)
			if ms < slice {
				slice = ms
			}
		}
		if slice == 0 {
			slice = 1
		}
		event, err := windows.WaitForSingleObject(windows.Handle(uintptr(fd)), slice)
		if err != nil {
			return 0, false, err
		}
		if event == uint32(windows.WAIT_TIMEOUT) {
			continue
		}
		if event != windows.WAIT_OBJECT_0 {
			return 0, false, nil
		}
		b, err := reader.ReadByte()
		return b, err == nil, err
	}
}
