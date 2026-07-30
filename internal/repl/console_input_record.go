package repl

import "encoding/binary"

// Windows console INPUT_RECORD classification — the decision core of the
// REGFIX-C #6 root fix (proof dossier:
// docs/design/tty_single_stdin_owner_20260729.md §8).
//
// This file is deliberately platform-NEUTRAL: the classification logic
// and the wire-layout mirror live here so their tests compile and RUN on
// every development host, leaving only a thin syscall shell
// (native_input_windows.go) that this unix/CGO-bound repo cannot
// compile-verify in place. The shell is compile-verified in an isolated
// GOOS=windows module instead; commands recorded in the dossier.
//
// Problem being solved: a console input handle is SIGNALED whenever the
// input buffer holds ANY unread record — focus changes, mouse moves,
// menu events, buffer resizes — none of which ReadFile will ever return
// a byte for (the docs state ReadFile/ReadConsole discard non-key
// records themselves). A readiness-then-blocking-read loop therefore
// wedges on an idle-but-focused terminal. The fix peeks the front
// record, discards the ones PROVEN byte-free via ReadConsoleInput, and
// only commits to the blocking read when a byte may actually follow.
// Classification errs toward commit: committing on a byte-free record
// reproduces the pre-fix blocking behavior (never worse), while
// discarding a byte-carrying record would lose input (never allowed).

// Win32 INPUT_RECORD event types
// (https://learn.microsoft.com/en-us/windows/console/input-record-str;
// values also mirrored in golang.org/x/sys/windows types_windows.go —
// pinned by TestConsoleInputRecord_EventTypeValues).
const (
	consoleKeyEvent              = 0x0001
	consoleMouseEvent            = 0x0002
	consoleWindowBufferSizeEvent = 0x0004
	consoleMenuEvent             = 0x0008
	consoleFocusEvent            = 0x0010
)

// consoleInputRecord mirrors the 20-byte INPUT_RECORD wire layout:
// WORD EventType at offset 0, two natural-alignment padding bytes, and
// the 16-byte event union at offset 4 (KEY_EVENT_RECORD and
// MOUSE_EVENT_RECORD are the co-largest members at 16 bytes; the
// BOOL/DWORD members of the union force its 4-byte alignment). The
// layout is identical on 386/amd64/arm64 — no member exceeds 4-byte
// alignment — and is pinned by TestConsoleInputRecord_WireLayout. Four
// independent production implementations (mattn/go-tty, gdamore/tcell,
// Azure/go-ansiterm, erikgeiser/coninput) encode this exact layout;
// tcell additionally hand-decodes the payload at these byte offsets.
// The zero-width leading field raises the Go struct's alignment from 2
// to 4 so a stack-allocated record always satisfies C's
// _Alignof(INPUT_RECORD)==4 when its address crosses the syscall
// boundary; it adds no bytes and moves no offsets.
type consoleInputRecord struct {
	_         [0]uint32
	eventType uint16
	_         uint16
	payload   [16]byte
}

// KEY_EVENT_RECORD field offsets within the 16-byte union payload
// (https://learn.microsoft.com/en-us/windows/console/key-event-record-str):
// BOOL bKeyDown @0, WORD wRepeatCount @4, WORD wVirtualKeyCode @6,
// WORD wVirtualScanCode @8, WCHAR uChar @10, DWORD dwControlKeyState @12.
// Windows is little-endian on every supported architecture.
const (
	keyEventKeyDownOffset = 0
	keyEventVirtualKeyOff = 6
	keyEventUCharOffset   = 10
)

// vkMenu is the ALT key's virtual-key code. Alt+numpad composition
// delivers the composed character in the ALT key's own key-UP record —
// the one key-up ReadFile can produce a byte from.
const vkMenu = 0x12

// consoleRecordYieldsNoByte reports whether the peeked record can be
// discarded because ReadFile can NEVER return a byte for it. false
// means "commit to the blocking read" — which for a record that turns
// out byte-free is exactly the pre-fix behavior, so every uncertain
// case falls through to false.
func consoleRecordYieldsNoByte(eventType uint16, payload []byte) bool {
	if eventType != consoleKeyEvent {
		// Focus and menu events are documented "used internally and
		// should be ignored"; mouse and buffer-size records are
		// documented as discarded by ReadFile/ReadConsole. None can
		// yield a byte. Unknown event types are equally non-key.
		return true
	}
	if len(payload) < 16 {
		return false // malformed peek: never-worse fallthrough
	}
	keyDown := binary.LittleEndian.Uint32(payload[keyEventKeyDownOffset:]) != 0
	vk := binary.LittleEndian.Uint16(payload[keyEventVirtualKeyOff:])
	uChar := binary.LittleEndian.Uint16(payload[keyEventUCharOffset:])
	if !keyDown {
		// Key releases yield no bytes — ReadFile extracts characters
		// from key-DOWN records only, even though releases often echo
		// the translated character in uChar (which is why this branch
		// must not test uChar alone: the echo would defeat the fix for
		// every keystroke's trailing release). The single key-up shape
		// ReadFile ever delivers is conhost GetChar's verbatim
		// condition `uChar != 0 && vk == VK_MENU` (the Alt+numpad
		// composition lane) — the commit condition below equals it
		// exactly, so key-up classification is provably equivalent to
		// what the blocking read would have produced.
		return !(vk == vkMenu && uChar != 0)
	}
	if uChar != 0 {
		// A translated character is present (printable keys, control
		// chars like Ctrl+C 0x03, ESC 0x1b, IME output via VK_PACKET,
		// VT-translated arrow-key sequence bytes): a byte follows.
		return false
	}
	// Char-free key-down: pure modifier and lock keys produce no byte
	// ever — the one conhost lane that synthesizes a byte from a
	// char-free key-down (the Ctrl+Space / Ctrl+@ NUL via
	// VkKeyScanW(0)) structurally cannot land on a modifier VK, which
	// closes the whitelist's proof. Anything else (dead keys, IME
	// pre-composition VK_PROCESSKEY, arrows on a non-VT console,
	// unrecognized keys) commits — never-worse fallthrough.
	return isModifierOnlyVirtualKey(vk)
}

// isModifierOnlyVirtualKey reports whether vk is a modifier or lock key
// whose press, alone, never produces input bytes: VK_SHIFT/VK_CONTROL/
// VK_MENU (+ their L/R variants VK_LSHIFT..VK_RMENU), VK_CAPITAL,
// VK_LWIN/VK_RWIN, VK_NUMLOCK, VK_SCROLL.
func isModifierOnlyVirtualKey(vk uint16) bool {
	switch vk {
	case 0x10, 0x11, 0x12, // VK_SHIFT, VK_CONTROL, VK_MENU
		0x14,       // VK_CAPITAL
		0x5B, 0x5C, // VK_LWIN, VK_RWIN
		0x90, 0x91, // VK_NUMLOCK, VK_SCROLL
		0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5: // VK_LSHIFT..VK_RMENU
		return true
	}
	return false
}
