package types

import (
	"sync"
	"testing"
)

func TestRepairAttemptHistory_IncrementAndGet(t *testing.T) {
	h := NewRepairAttemptHistory()
	k := FpKey{Kind: ViolDiagramEdgeUnsupported, Fingerprint: "block:d1"}
	if got := h.Get(k); got != 0 {
		t.Errorf("fresh history Get = %d, want 0", got)
	}
	if got := h.Increment(k); got != 1 {
		t.Errorf("first Increment = %d, want 1", got)
	}
	if got := h.Increment(k); got != 2 {
		t.Errorf("second Increment = %d, want 2", got)
	}
	if got := h.Get(k); got != 2 {
		t.Errorf("Get after two increments = %d, want 2", got)
	}
}

func TestRepairAttemptHistory_DistinctKeysIndependent(t *testing.T) {
	h := NewRepairAttemptHistory()
	k1 := FpKey{Kind: ViolDiagramEdgeUnsupported, Fingerprint: "block:d1"}
	k2 := FpKey{Kind: ViolFacetUncovered, Fingerprint: "facet:diagram_spine"}
	h.Increment(k1)
	h.Increment(k1)
	h.Increment(k2)
	if got := h.Get(k1); got != 2 {
		t.Errorf("k1 = %d, want 2", got)
	}
	if got := h.Get(k2); got != 1 {
		t.Errorf("k2 = %d, want 1", got)
	}
}

func TestRepairAttemptHistory_Exhausted(t *testing.T) {
	h := NewRepairAttemptHistory()
	k := FpKey{Kind: ViolDiagramEdgeUnsupported, Fingerprint: "block:d1"}
	for i := 0; i < MaxRepairAttemptsPerRoot-1; i++ {
		h.Increment(k)
		if h.Exhausted(k) {
			t.Errorf("after %d increments Exhausted should be false", i+1)
		}
	}
	h.Increment(k)
	if !h.Exhausted(k) {
		t.Errorf("after %d increments Exhausted should be true", MaxRepairAttemptsPerRoot)
	}
}

func TestRepairAttemptHistory_Reset(t *testing.T) {
	h := NewRepairAttemptHistory()
	k := FpKey{Kind: ViolDiagramEdgeUnsupported, Fingerprint: "block:d1"}
	h.Increment(k)
	h.Increment(k)
	h.Reset()
	if got := h.Get(k); got != 0 {
		t.Errorf("Reset did not clear; Get = %d", got)
	}
}

func TestRepairAttemptHistory_NilSafe(t *testing.T) {
	var h *RepairAttemptHistory
	if got := h.Get(FpKey{}); got != 0 {
		t.Errorf("nil Get = %d, want 0", got)
	}
	if got := h.Increment(FpKey{}); got != 0 {
		t.Errorf("nil Increment = %d, want 0", got)
	}
	if h.Exhausted(FpKey{}) {
		t.Error("nil Exhausted = true, want false")
	}
	h.Reset() // must not panic
	if got := h.Snapshot(); got != nil {
		t.Errorf("nil Snapshot = %v, want nil", got)
	}
}

func TestRepairAttemptHistory_Snapshot(t *testing.T) {
	h := NewRepairAttemptHistory()
	k1 := FpKey{Kind: ViolDiagramEdgeUnsupported, Fingerprint: "fp1"}
	k2 := FpKey{Kind: ViolFacetUncovered, Fingerprint: "fp2"}
	h.Increment(k1)
	h.Increment(k1)
	h.Increment(k2)
	snap := h.Snapshot()
	if len(snap) != 2 || snap[k1] != 2 || snap[k2] != 1 {
		t.Errorf("Snapshot wrong: %+v", snap)
	}
	// Snapshot must be a copy — mutating it must NOT change history.
	snap[k1] = 99
	if got := h.Get(k1); got != 2 {
		t.Errorf("Snapshot mutation leaked into history: %d", got)
	}
}

func TestRepairAttemptHistory_ConcurrentIncrement(t *testing.T) {
	h := NewRepairAttemptHistory()
	k := FpKey{Kind: ViolDiagramEdgeUnsupported, Fingerprint: "racy"}
	const goroutines = 50
	const incrementsPerGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				h.Increment(k)
			}
		}()
	}
	wg.Wait()
	want := goroutines * incrementsPerGoroutine
	if got := h.Get(k); got != want {
		t.Errorf("after race: Get = %d, want %d (data race likely)", got, want)
	}
}

// TestMutableState_RepairAttempts_LazyInit pins the lazy init
// pattern: nil state stays nil, fresh state lazy-allocates,
// repeated access returns the same instance.
func TestMutableState_RepairAttempts_LazyInit(t *testing.T) {
	m := NewMutableState("task")
	h1 := m.RepairAttempts()
	if h1 == nil {
		t.Fatal("RepairAttempts on fresh state returned nil")
	}
	k := FpKey{Kind: ViolDiagramEdgeUnsupported, Fingerprint: "x"}
	h1.Increment(k)
	h2 := m.RepairAttempts()
	if h1 != h2 {
		t.Error("repeated RepairAttempts call returned different instance")
	}
	if got := h2.Get(k); got != 1 {
		t.Errorf("history not shared: Get = %d", got)
	}
}

// TestMutableState_ResetRepairAttempts pins the per-task reset.
func TestMutableState_ResetRepairAttempts(t *testing.T) {
	m := NewMutableState("task")
	h := m.RepairAttempts()
	k := FpKey{Kind: ViolFacetUncovered, Fingerprint: "f"}
	h.Increment(k)
	h.Increment(k)
	m.ResetRepairAttempts()
	if got := h.Get(k); got != 0 {
		t.Errorf("Reset did not clear: Get = %d", got)
	}
}
