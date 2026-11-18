package types

import "testing"

func measuredFactTestNode(pricing, measured string, runnableMS float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		EvidenceID: "E", Subject: "app-20", Predicate: "wakeup_causal_impact",
		StateKind: "runnable", RunnableMS: runnableMS, ImpactMS: runnableMS,
		StartTs: 11, EndTs: 11.008,
		StateAccountKey: pricing, MeasuredStateAccountKey: measured,
	}
}

func TestMeasuredStateFactKeyPrefersThePricingIdentity(t *testing.T) {
	for _, tc := range []struct{ pricing, measured, want string }{
		{"", "", ""},
		{"state_account:v2:a", "", "state_account:v2:a"},
		{"", "state_account:v2:a", "state_account:v2:a"},
		{" state_account:v2:a ", "state_account:v2:b", "state_account:v2:a"},
	} {
		if got := measuredFactTestNode(tc.pricing, tc.measured, 5).MeasuredStateFactKey(); got != tc.want {
			t.Errorf("pricing=%q measured=%q: got %q, want %q", tc.pricing, tc.measured, got, tc.want)
		}
	}
}

// Same fact = the same producer-minted native identity AND the same published
// state occupancy. Neither half is enough alone.
func TestTraceMeasuredStateFactSame(t *testing.T) {
	const key = "state_account:v2:a"
	seat := measuredFactTestNode("", key, 5) // priced differently: measured-only identity
	seat.Predicate, seat.ImpactMS, seat.EffectiveImpactMS, seat.Rank = "root_cause_primary", 3, 3, 1
	view := measuredFactTestNode(key, "", 5)

	if !TraceMeasuredStateFactSame(seat, view) || !TraceMeasuredStateFactSame(view, seat) {
		t.Fatal("same native identity and same published occupancy is one measured fact")
	}
	if !TraceMeasuredStateFactSame(view, view) {
		t.Fatal("two publications priced at the same account are one measured fact")
	}

	// Same identity, different displayed number: a producer contradiction
	// stays two facts.
	other := view
	other.RunnableMS = 5.01
	if TraceMeasuredStateFactSame(seat, other) {
		t.Fatal("identity alone must not join two different displayed numbers")
	}
	// Inside the display tie the number is the same.
	other.RunnableMS = 5.0004
	if !TraceMeasuredStateFactSame(seat, other) {
		t.Fatal("a difference below the display tie is the same number")
	}

	// No identity: equal subject, state, window and value never join.
	if TraceMeasuredStateFactSame(measuredFactTestNode("", "", 5), measuredFactTestNode("", "", 5)) {
		t.Fatal("equal value and name without a native identity must not join")
	}
	if TraceMeasuredStateFactSame(seat, measuredFactTestNode("", "", 5)) {
		t.Fatal("one side without identity must not join")
	}
	// Different identities with everything else equal never join.
	if TraceMeasuredStateFactSame(seat, measuredFactTestNode("state_account:v2:b", "", 5)) {
		t.Fatal("different native identities must not join on equal values")
	}

	// A display fold does not publish one state account.
	merged := view
	merged.MergedCount = 2
	overflow := view
	overflow.OnChainOverflowFold = true
	if TraceMeasuredStateFactSame(seat, merged) || TraceMeasuredStateFactSame(seat, overflow) {
		t.Fatal("a merged or overflow-fold row has no single published state occupancy")
	}
	// A row whose kind is not a scheduler state publishes no state occupancy.
	span := view
	span.StateKind = ""
	if TraceMeasuredStateFactSame(seat, span) {
		t.Fatal("a row without a scheduler-state kind must not join")
	}
	// A row whose state lane was not published has no occupancy to compare.
	unpublished := seat
	unpublished.RunnableMS = 0
	if TraceMeasuredStateFactSame(unpublished, view) {
		t.Fatal("an unpublished occupancy must not join")
	}
}
