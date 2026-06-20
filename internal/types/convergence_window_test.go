package types

import "testing"

func TestConvergenceWindow_TailEquality(t *testing.T) {
	w := NewConvergenceWindow[int](4)
	if w.ConsecutiveEqualTail() != 0 || w.StalledAt(2) {
		t.Fatal("empty window must report no tail / not stalled")
	}
	w.Push(1)
	if w.ConsecutiveEqualTail() != 1 || w.StalledAt(2) {
		t.Fatalf("single push: tail=%d stalled=%v", w.ConsecutiveEqualTail(), w.StalledAt(2))
	}
	w.Push(2)
	w.Push(2)
	if w.ConsecutiveEqualTail() != 2 || !w.StalledAt(2) || w.StalledAt(3) {
		t.Fatalf("two equal tail: tail=%d stalled2=%v stalled3=%v", w.ConsecutiveEqualTail(), w.StalledAt(2), w.StalledAt(3))
	}
	w.Push(2)
	if !w.StalledAt(3) {
		t.Fatal("three equal tail must stall at 3")
	}
	w.Push(9) // a new typed delta breaks the run
	if w.ConsecutiveEqualTail() != 1 || w.StalledAt(2) {
		t.Fatalf("new delta resets tail: tail=%d", w.ConsecutiveEqualTail())
	}
}

func TestConvergenceWindow_BoundedCapacity(t *testing.T) {
	w := NewConvergenceWindow[int](3)
	for i := 0; i < 100; i++ {
		w.Push(7)
	}
	if w.Len() != 3 {
		t.Fatalf("window must stay bounded at 3, got %d", w.Len())
	}
	if !w.StalledAt(3) || w.StalledAt(4) {
		t.Fatal("bounded window of equal entries stalls at cap but not beyond")
	}
}

func TestStalledAtTail_SliceForm(t *testing.T) {
	type fp struct{ a, b uint32 }
	hist := []fp{{1, 1}, {2, 2}, {2, 2}, {2, 2}}
	if ConsecutiveEqualTail(hist) != 3 {
		t.Fatalf("tail run = %d, want 3", ConsecutiveEqualTail(hist))
	}
	if !StalledAtTail(hist, 2) || !StalledAtTail(hist, 3) || StalledAtTail(hist, 4) {
		t.Fatal("slice-form stall thresholds wrong")
	}
	if StalledAtTail(hist, 1) {
		t.Fatal("n<2 must never report stalled")
	}
}
