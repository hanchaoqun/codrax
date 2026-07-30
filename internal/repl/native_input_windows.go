//go:build windows

package repl

import (
	"bufio"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func (e *nativeLineInput) readByteWithTimeout(d time.Duration) (byte, bool, error) {
	return readByteFDWithTimeout(e.reader, e.fd, d)
}

// PeekConsoleInputW / ReadConsoleInputW are not exported by
// golang.org/x/sys/windows (v0.44.0 audit: absent), so they are loaded
// lazily from kernel32 the same way erikgeiser/coninput (Bubble Tea's
// Windows console dependency) does. NewLazySystemDLL restricts the
// search to the Windows system directory.
var (
	winConsoleKernel32    = windows.NewLazySystemDLL("kernel32.dll")
	procPeekConsoleInputW = winConsoleKernel32.NewProc("PeekConsoleInputW")
	procReadConsoleInputW = winConsoleKernel32.NewProc("ReadConsoleInputW")
)

// readByteFDWithTimeout is the shared buffered-first byte read with a
// handle-level wait (TTY-1/T-2); see the unix twin for the contract.
// d <= 0 is an instant poll (buffered bytes only), matching the twin.
//
// REGFIX-C #6 root fix (proof-based; dossier
// docs/design/tty_single_stdin_owner_20260729.md §8): a console input
// handle is signaled whenever the buffer holds ANY unread record —
// focus, menu, mouse and resize records included — but ReadFile only
// ever returns bytes for (some) key events and silently discards the
// rest, so readiness-then-blocking-read wedged on an idle-but-focused
// terminal. The loop now peeks the front record and discards the ones
// classification proves byte-free (consoleRecordYieldsNoByte, host-
// tested in console_input_record_test.go); it commits to the blocking
// read only when a byte may follow. Every failure or uncertainty lane
// commits — reproducing the pre-fix behavior, never worse than it.
//
// Verification status (platform ruling, postmortem §6/§7): the
// classification core is platform-neutral and host-tested; this shell
// is compile-verified (build + vet, amd64/386/arm64) in an isolated
// GOOS=windows module recorded in the dossier — but it has NOT been
// executed on a live Windows console from this unix build host. The
// TTY run-input window therefore stays disarmed on Windows
// (run_input_mode_other.go) until real-console verification; the only
// consumer of this function on Windows remains the prompt editor's
// ESC/CSI probes.
func readByteFDWithTimeout(reader *bufio.Reader, fd int, d time.Duration) (byte, bool, error) {
	if reader.Buffered() > 0 {
		b, err := reader.ReadByte()
		return b, err == nil, err
	}
	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, false, nil
		}
		slice := uint32(20)
		if ms := uint32((remaining + time.Millisecond - 1) / time.Millisecond); ms < slice {
			slice = ms
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
		// Signaled: at least one unread record. Peek it (Peek never
		// blocks and never removes records) and classify. If the peek
		// surface is unavailable for any reason, commit to the
		// blocking read — the pre-fix behavior.
		if procPeekConsoleInputW.Find() != nil || procReadConsoleInputW.Find() != nil {
			b, err := reader.ReadByte()
			return b, err == nil, err
		}
		var rec consoleInputRecord
		var n uint32
		r0, _, _ := syscall.SyscallN(procPeekConsoleInputW.Addr(),
			uintptr(fd), uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&n)))
		if r0 == 0 {
			b, err := reader.ReadByte()
			return b, err == nil, err
		}
		if n == 0 {
			// Spurious readiness (buffer drained between wait and
			// peek): re-wait within the remaining budget.
			continue
		}
		if consoleRecordYieldsNoByte(rec.eventType, rec.payload[:]) {
			// Provably byte-free: consume and drop the single record
			// — exactly what ReadFile would do with it, minus the
			// blocking wait for a key that may never come.
			r0, _, _ = syscall.SyscallN(procReadConsoleInputW.Addr(),
				uintptr(fd), uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&n)))
			if r0 == 0 {
				b, err := reader.ReadByte()
				return b, err == nil, err
			}
			continue
		}
		b, err := reader.ReadByte()
		return b, err == nil, err
	}
}
