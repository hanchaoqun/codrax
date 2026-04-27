package worktree

import (
	"testing"
)

// TestSetSignalHandlerSuppressed_RoundTrip locks the basic on/off
// contract — the REPL toggles this around its lifetime, and any
// caller that observes the flag sees the value the most recent
// setter wrote. atomic.Bool guarantees the visibility, this test
// pins the SetSignalHandlerSuppressed signature so refactors can't
// silently drop the surface the REPL depends on.
func TestSetSignalHandlerSuppressed_RoundTrip(t *testing.T) {
	// Save + restore so concurrent tests aren't perturbed.
	prev := signalHandlerSuppressed.Load()
	t.Cleanup(func() { signalHandlerSuppressed.Store(prev) })

	if signalHandlerSuppressed.Load() != prev {
		t.Fatalf("baseline drift: got %v want %v", signalHandlerSuppressed.Load(), prev)
	}

	SetSignalHandlerSuppressed(true)
	if !signalHandlerSuppressed.Load() {
		t.Errorf("SetSignalHandlerSuppressed(true) did not take effect")
	}
	SetSignalHandlerSuppressed(false)
	if signalHandlerSuppressed.Load() {
		t.Errorf("SetSignalHandlerSuppressed(false) did not take effect")
	}
}

// TestCleanActiveSessions_ExportedSurface verifies the public
// CleanActiveSessions wrapper exists for the REPL's exit-path
// cleanup. Empty active-session map → no-op (no panic). The actual
// cleanup machinery is exercised by TestDiscard / TestCreate elsewhere.
func TestCleanActiveSessions_ExportedSurface(t *testing.T) {
	clearActiveSessions(t)
	CleanActiveSessions() // no panic on empty map
}
