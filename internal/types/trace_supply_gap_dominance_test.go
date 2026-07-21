package types

import "testing"

// trace_supply_gap_dominance_test.go — INV-SUPPLY 件① criterion pins
// (§29.61.11; 收尾件1 P2-1 边界 pin, 2026-07-14).
//
// MUTATION self-checks (cp-copy recovery only): flipping the ≥ to > must go
// red on the exact-boundary probe below (measured 2026-07-14); dropping
// either zero-operand guard must go red on the fail-closed arms.
func TestTraceSupplyGapDominantBoundary(t *testing.T) {
	// Exact boundary: deficit == eff × share. Binary-exact operands (8.0 and
	// 4.0; the share 0.50 halves exactly), so the comparison is a true
	// equality at the double level — a ≥→> mutation flips exactly this case.
	if !TraceSupplyGapDominant(4.0, 8.0) {
		t.Fatalf("deficit == eff×%.2f must satisfy the criterion (≥, not >)", TraceSupplyGapDominanceShare)
	}
	// One ulp-free step below the boundary stays false (the criterion is a
	// threshold, not a tie-break).
	if TraceSupplyGapDominant(3.999, 8.0) {
		t.Fatalf("deficit below eff×share must not claim dominance")
	}
	// Witness magnitudes (donghu 090607): the ➊ seat ratio 103% passes, the
	// keva-1 runnable-only seat carries no fold and its absent deficit fails
	// closed below.
	if !TraceSupplyGapDominant(7.296, 7.081) {
		t.Fatalf("the 090607 ➊ witness ratio must pass")
	}
	// Fail-closed both arms: an unpublished operand never claims dominance.
	if TraceSupplyGapDominant(0, 8.0) {
		t.Fatalf("zero/absent deficit must fail closed")
	}
	if TraceSupplyGapDominant(4.0, 0) {
		t.Fatalf("zero/absent effective attribution must fail closed")
	}
	if TraceSupplyGapDominant(0, 0) {
		t.Fatalf("both operands absent must fail closed")
	}
	if TraceSupplyGapDominant(-1.0, 8.0) || TraceSupplyGapDominant(4.0, -1.0) {
		t.Fatalf("negative operands must fail closed")
	}
}
