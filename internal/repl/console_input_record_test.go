package repl

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// Host-runnable pins for the REGFIX-C #6 decision core (proof dossier:
// docs/design/tty_single_stdin_owner_20260729.md §8). These run on every
// development platform — Go struct layout for the field shapes involved
// is GOOS-independent, so the wire-layout assumptions the Windows shell
// relies on are pinned HERE, where tests actually execute.

// TestConsoleInputRecord_WireLayout pins the 20-byte INPUT_RECORD
// mirror: EventType at offset 0, event union at offset 4 (2 padding
// bytes), total 20 — the layout all four surveyed production
// implementations encode and tcell hand-decodes.
func TestConsoleInputRecord_WireLayout(t *testing.T) {
	var rec consoleInputRecord
	if s := unsafe.Sizeof(rec); s != 20 {
		t.Fatalf("sizeof(consoleInputRecord) = %d, want 20", s)
	}
	if o := unsafe.Offsetof(rec.eventType); o != 0 {
		t.Fatalf("EventType offset = %d, want 0", o)
	}
	if o := unsafe.Offsetof(rec.payload); o != 4 {
		t.Fatalf("event union offset = %d, want 4", o)
	}
	if a := unsafe.Alignof(rec); a != 4 {
		t.Fatalf("alignment = %d, want 4 (C _Alignof(INPUT_RECORD))", a)
	}
}

// TestConsoleInputRecord_EventTypeValues pins the documented event-type
// constants (input-record-str; mirrored by x/sys/windows).
func TestConsoleInputRecord_EventTypeValues(t *testing.T) {
	want := map[string]uint16{
		"KEY_EVENT":                consoleKeyEvent,
		"MOUSE_EVENT":              consoleMouseEvent,
		"WINDOW_BUFFER_SIZE_EVENT": consoleWindowBufferSizeEvent,
		"MENU_EVENT":               consoleMenuEvent,
		"FOCUS_EVENT":              consoleFocusEvent,
	}
	values := map[string]uint16{
		"KEY_EVENT":                0x0001,
		"MOUSE_EVENT":              0x0002,
		"WINDOW_BUFFER_SIZE_EVENT": 0x0004,
		"MENU_EVENT":               0x0008,
		"FOCUS_EVENT":              0x0010,
	}
	for name, got := range want {
		if got != values[name] {
			t.Errorf("%s = %#x, want %#x", name, got, values[name])
		}
	}
}

// keyPayload builds a KEY_EVENT_RECORD union payload at the documented
// offsets: bKeyDown @0 (BOOL/int32), wRepeatCount @4, wVirtualKeyCode
// @6, wVirtualScanCode @8, uChar @10, dwControlKeyState @12.
func keyPayload(keyDown bool, vk, uChar uint16) []byte {
	p := make([]byte, 16)
	if keyDown {
		binary.LittleEndian.PutUint32(p[0:], 1)
	}
	binary.LittleEndian.PutUint16(p[4:], 1) // repeat count
	binary.LittleEndian.PutUint16(p[6:], vk)
	binary.LittleEndian.PutUint16(p[10:], uChar)
	return p
}

// TestConsoleRecordYieldsNoByte pins the discard/commit policy. The
// asymmetry is the contract: a wrong COMMIT merely reproduces the
// pre-fix blocking read; a wrong DISCARD would lose typed input — so
// every discard case must be provably byte-free.
func TestConsoleRecordYieldsNoByte(t *testing.T) {
	cases := []struct {
		name      string
		eventType uint16
		payload   []byte
		discard   bool
	}{
		// Non-key records: ReadFile discards these itself; peeking
		// them away is behavior-identical and unblocks the wait.
		{"focus", consoleFocusEvent, make([]byte, 16), true},
		{"menu", consoleMenuEvent, make([]byte, 16), true},
		{"mouse", consoleMouseEvent, make([]byte, 16), true},
		{"resize", consoleWindowBufferSizeEvent, make([]byte, 16), true},
		{"unknown-type", 0x0040, make([]byte, 16), true},

		// Byte-carrying key-downs must commit.
		{"keydown-letter", consoleKeyEvent, keyPayload(true, 0x41, 'a'), false},
		{"keydown-esc", consoleKeyEvent, keyPayload(true, 0x1B, 0x1b), false},
		{"keydown-ctrl-c", consoleKeyEvent, keyPayload(true, 0x43, 0x03), false},
		{"keydown-ime-vkpacket", consoleKeyEvent, keyPayload(true, 0xE7, 0x4F60), false},
		{"keydown-vt-arrow-seq-byte", consoleKeyEvent, keyPayload(true, 0, '['), false},

		// Char-free key-downs: modifiers/locks discard, everything
		// else commits (dead keys, non-VT arrows, Ctrl+Space NUL).
		{"keydown-shift", consoleKeyEvent, keyPayload(true, 0x10, 0), true},
		{"keydown-ctrl", consoleKeyEvent, keyPayload(true, 0x11, 0), true},
		{"keydown-alt", consoleKeyEvent, keyPayload(true, 0x12, 0), true},
		{"keydown-capslock", consoleKeyEvent, keyPayload(true, 0x14, 0), true},
		{"keydown-lwin", consoleKeyEvent, keyPayload(true, 0x5B, 0), true},
		{"keydown-numlock", consoleKeyEvent, keyPayload(true, 0x90, 0), true},
		{"keydown-rshift", consoleKeyEvent, keyPayload(true, 0xA1, 0), true},
		{"keydown-deadkey", consoleKeyEvent, keyPayload(true, 0xBA, 0), false},
		{"keydown-arrow-nonvt", consoleKeyEvent, keyPayload(true, 0x25, 0), false},
		{"keydown-ctrl-space-nul", consoleKeyEvent, keyPayload(true, 0x20, 0), false},

		// Key-ups: always byte-free — INCLUDING when they echo the
		// translated char (the echo is why uChar alone must not
		// decide) — except Alt+numpad composition on VK_MENU's own
		// release.
		{"keyup-letter-echoes-uchar", consoleKeyEvent, keyPayload(false, 0x41, 'a'), true},
		{"keyup-esc-echoes-uchar", consoleKeyEvent, keyPayload(false, 0x1B, 0x1b), true},
		{"keyup-arrow", consoleKeyEvent, keyPayload(false, 0x25, 0), true},
		{"keyup-shift", consoleKeyEvent, keyPayload(false, 0x10, 0), true},
		{"keyup-alt-plain", consoleKeyEvent, keyPayload(false, 0x12, 0), true},
		{"keyup-alt-numpad-composed", consoleKeyEvent, keyPayload(false, 0x12, 0x00E9), false},

		// Malformed peek payload: never-worse fallthrough.
		{"short-payload", consoleKeyEvent, make([]byte, 3), false},
	}
	for _, tc := range cases {
		if got := consoleRecordYieldsNoByte(tc.eventType, tc.payload); got != tc.discard {
			t.Errorf("%s: discard=%v, want %v", tc.name, got, tc.discard)
		}
	}
}
