package types

import "testing"

// TestMutableState_RefundToolCall_ClampsAtZero pins the refund
// semantics: a RefundToolCall call undoes exactly one prior
// RecordToolCall, and spurious refunds below zero are clamped
// rather than underflowing into negative usage (which would make
// BudgetRemaining over-report by every phantom refund).
func TestMutableState_RefundToolCall_ClampsAtZero(t *testing.T) {
	m := NewMutableState("")
	m.SetExploreBudget(&ExploreBudget{
		PerToolCap:  map[string]int{"read_file": 3},
		PerToolUsed: map[string]int{},
		OverallCap:  10,
	})

	// Record 2 calls → used=2, rem=1.
	m.RecordToolCall("read_file")
	m.RecordToolCall("read_file")
	if got := m.BudgetRemaining("read_file"); got != 1 {
		t.Fatalf("after 2 records: BudgetRemaining=%d, want 1", got)
	}

	// Refund 1 → used=1, rem=2.
	m.RefundToolCall("read_file")
	if got := m.BudgetRemaining("read_file"); got != 2 {
		t.Errorf("after 1 refund: BudgetRemaining=%d, want 2", got)
	}

	// Refund 1 more → used=0, rem=3.
	m.RefundToolCall("read_file")
	if got := m.BudgetRemaining("read_file"); got != 3 {
		t.Errorf("after 2 refunds: BudgetRemaining=%d, want 3", got)
	}

	// Spurious extra refund → stays at used=0 (no underflow).
	m.RefundToolCall("read_file")
	if got := m.BudgetRemaining("read_file"); got != 3 {
		t.Errorf("after 3 refunds (1 spurious): BudgetRemaining=%d, want 3 (clamped)", got)
	}

	// Overall counter also clamped.
	eb := m.ExploreBudget()
	if eb.OverallUsed != 0 {
		t.Errorf("OverallUsed=%d, want 0 (clamped after over-refund)", eb.OverallUsed)
	}
}

// TestMutableState_RefundToolCall_NoBudgetInstalled — refund on a
// bare MutableState must be a no-op, not a panic. Matches the
// RecordToolCall contract for callers that run without an
// ExploreBudget installed (analyze/extract/finalize stages).
func TestMutableState_RefundToolCall_NoBudgetInstalled(t *testing.T) {
	m := NewMutableState("")
	m.RefundToolCall("read_file") // no budget installed → no-op
	// A nil receiver must also be safe (mirrors RecordToolCall).
	var nilM *MutableState
	nilM.RefundToolCall("read_file")
}
