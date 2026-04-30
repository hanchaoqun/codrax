package types

import (
	"testing"
)

// TestMutableState_UnvalidatedReasons covers the record + drain pair.
func TestMutableState_UnvalidatedReasons(t *testing.T) {
	mu := NewMutableState("test")
	if got := mu.DrainUnvalidatedReasons(); len(got) != 0 {
		t.Errorf("zero-value drain should be empty; got %v", got)
	}
	mu.RecordUnvalidatedReason("rust: cargo not in PATH")
	mu.RecordUnvalidatedReason("kotlin: kotlinc not in PATH")
	got := mu.DrainUnvalidatedReasons()
	if len(got) != 2 {
		t.Fatalf("expected 2 reasons; got %d (%v)", len(got), got)
	}
	if got[0] != "rust: cargo not in PATH" || got[1] != "kotlin: kotlinc not in PATH" {
		t.Errorf("reasons drift: got %v", got)
	}
	// After drain the collector must be empty.
	if again := mu.DrainUnvalidatedReasons(); len(again) != 0 {
		t.Errorf("second drain should be empty; got %v", again)
	}
}

// TestMutableState_UnvalidatedReasons_NilSafe pins the defensive path.
func TestMutableState_UnvalidatedReasons_NilSafe(t *testing.T) {
	var mu *MutableState
	mu.RecordUnvalidatedReason("rust: cargo missing") // must not panic
	if got := mu.DrainUnvalidatedReasons(); got != nil {
		t.Errorf("nil-receiver drain should be nil; got %v", got)
	}
}
