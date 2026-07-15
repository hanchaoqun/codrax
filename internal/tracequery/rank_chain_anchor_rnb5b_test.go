package tracequery

// rank_chain_anchor_rnb5b_test.go — RNB-5B 件④ (§29.96.2 终判④ D2,
// 2026-07-15): the critical_blocking D/IO VIEW row lane verdict refines from
// pid level to ROW level for interval-bearing rows —
//
//	行自身区间∩锚窗=∅ → ◇ (even on a credentialed pid — the D2 dust-anchored
//	                     refinement);
//	有交            → ⛓ (legacy lane);
//	无区间          → pid-level conservative rule byte-identical (zero-
//	                  credential census demotes, credentialed census keeps).
//
// The integration form (zero-credential demotion + tieba-60555-shape
// credentialed negative control) stays pinned by
// TestRSPARNBCriticalBlockingZeroCredentialDemotion — under the row-level
// arm the worker row's hull touches its window only at a point (overlap 0 →
// demote) and the helper row's hull overlaps (keep), so the pin doubles as
// the row-level regression witness.
//
// MUTATION self-check: reverting criticalBlockingDioRowDemotes to the pure
// pid-level rule (decision.anchoredMs ≤ tol regardless of the row interval)
// reds TestRNB5BDioRowLevelDemotionArms (the credentialed-pid/disjoint-row
// arm stops demoting).

import "testing"

func TestRNB5BDioRowLevelDemotionArms(t *testing.T) {
	windows := []TimeWindow{{StartTs: 1.000, EndTs: 1.010}}
	credentialed := rspaFamilyDecision{migrate: true, anchoredMs: 5.0, fullMs: 8.0}
	zeroCredential := rspaFamilyDecision{migrate: true, anchoredMs: 0.0, fullMs: 8.0}
	row := func(start, end float64) CriticalBlockingCandidate {
		return CriticalBlockingCandidate{Type: "d_state_or_io_wait",
			reconStartTs: start, reconEndTs: end}
	}
	// ① interval ∩ window = ∅ on a CREDENTIALED pid → demote (the D2
	// refinement: the pid's dust/real credential no longer shields a row
	// whose own interval never touches a jump window).
	if !criticalBlockingDioRowDemotes(row(1.020, 1.050), credentialed, windows) {
		t.Fatalf("① disjoint row interval must demote even on a credentialed pid")
	}
	// ② interval ∩ window > 0 → keep, regardless of the pid census.
	if criticalBlockingDioRowDemotes(row(1.005, 1.050), credentialed, windows) {
		t.Fatalf("② overlapping row interval must keep the ⛓ lane")
	}
	if criticalBlockingDioRowDemotes(row(1.005, 1.050), zeroCredential, windows) {
		t.Fatalf("② overlapping row interval keeps ⛓ on a zero-credential pid too (有交→⛓)")
	}
	// ③ interval-less rows keep the pid-level conservative rule.
	if !criticalBlockingDioRowDemotes(row(0, 0), zeroCredential, windows) {
		t.Fatalf("③ interval-less + zero credential must demote (B-4 legacy)")
	}
	if criticalBlockingDioRowDemotes(row(0, 0), credentialed, windows) {
		t.Fatalf("③ interval-less + credentialed pid must keep the lane (保守)")
	}
	// ④ point contact (end == window end boundary handled by overlap ≤ 0).
	if !criticalBlockingDioRowDemotes(row(1.010, 1.050), credentialed, windows) {
		t.Fatalf("④ point contact carries zero overlap → demote")
	}
}
