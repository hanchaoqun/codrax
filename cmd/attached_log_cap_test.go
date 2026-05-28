package cmd

import (
	"strings"
	"testing"
)

// TestTruncateAttachedLog_DefaultCap confirms the out-of-the-box
// ceiling still fires when the package-level var has not been tweaked.
// Uses a local save/restore so the test does not leak state across
// other tests that may swap the cap.
func TestTruncateAttachedLog_DefaultCap(t *testing.T) {
	prev := maxAttachedLogBytes
	defer func() { maxAttachedLogBytes = prev }()
	maxAttachedLogBytes = defaultAttachedLogMaxBytes

	// Feed default cap + 1 byte and expect exactly the default cap out.
	input := strings.Repeat("x", defaultAttachedLogMaxBytes+1)
	got := truncateAttachedLog(input)
	if len(got) != defaultAttachedLogMaxBytes {
		t.Fatalf("default cap: len=%d, want %d", len(got), defaultAttachedLogMaxBytes)
	}
}

// TestTruncateAttachedLog_HonorsConfiguredOverride is the core
// regression guard for the new log_attach_max_bytes yaml knob. When
// initApp writes a smaller value into maxAttachedLogBytes, the
// truncateAttachedLog path must cut to exactly that length.
func TestTruncateAttachedLog_HonorsConfiguredOverride(t *testing.T) {
	prev := maxAttachedLogBytes
	defer func() { maxAttachedLogBytes = prev }()

	const customCap = 2048
	maxAttachedLogBytes = customCap

	input := strings.Repeat("y", 3*1024)
	got := truncateAttachedLog(input)
	if len(got) != customCap {
		t.Fatalf("custom cap %d: len=%d, want %d", customCap, len(got), customCap)
	}
}

// TestTruncateAttachedLog_NoTruncationWhenUnderCap verifies the
// fast-path: payloads ≤ cap flow through unchanged. Guards against
// an accidental "always cut" regression if someone rewrites the
// guard condition.
func TestTruncateAttachedLog_NoTruncationWhenUnderCap(t *testing.T) {
	prev := maxAttachedLogBytes
	defer func() { maxAttachedLogBytes = prev }()
	maxAttachedLogBytes = 4096

	input := "short payload\n"
	got := truncateAttachedLog(input)
	if got != input {
		t.Fatalf("under-cap input mutated: got %q, want %q", got, input)
	}
}

// TestTruncateAttachedLog_HandlesRaisedCap documents the intended
// upgrade path: yaml override ABOVE the default still works, so an
// operator who attaches a 4 MB log with log_attach_max_bytes: 5242880
// gets the full payload through untouched.
func TestTruncateAttachedLog_HandlesRaisedCap(t *testing.T) {
	prev := maxAttachedLogBytes
	defer func() { maxAttachedLogBytes = prev }()
	maxAttachedLogBytes = 5 * 1024 * 1024 // 5 MB

	input := strings.Repeat("z", 4*1024*1024) // 4 MB, below the override
	got := truncateAttachedLog(input)
	if len(got) != len(input) {
		t.Fatalf("raised cap: len=%d, want %d (no truncation expected)", len(got), len(input))
	}
}
