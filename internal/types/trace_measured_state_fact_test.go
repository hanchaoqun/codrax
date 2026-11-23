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

// A ×N fold shows its occurrences together. It answers with the account every
// member names and never with the account of the member it was seeded from.
func TestMeasuredStateFactOfAFoldIsTheAccountAllItsOccurrencesName(t *testing.T) {
	const account = "state_account:v2:group"
	seat := measuredFactTestNode("", account, 13.699)
	seat.Predicate, seat.ImpactMS, seat.EffectiveImpactMS, seat.Rank = "root_cause_primary", 13.737, 13.737, 1

	fold := measuredFactTestNode("state_account:v2:seed-member", "", 7.843)
	fold.MergedCount, fold.ImpactMS, fold.MeasuredOccurrenceGroupKey = 3, 13.699, account
	if got := fold.MeasuredStateFactKey(); got != account {
		t.Fatalf("fold identity=%q, want the shared account %q", got, account)
	}
	if !TraceMeasuredStateFactSame(seat, fold) || !TraceMeasuredStateFactSame(fold, seat) {
		t.Fatal("a fold of every occurrence and the seat of that account are one measured fact")
	}

	bare := fold
	bare.MeasuredOccurrenceGroupKey = ""
	if got := bare.MeasuredStateFactKey(); got != "" {
		t.Fatalf("a fold must not answer with its seed member's account: %q", got)
	}
	seedOnly := measuredFactTestNode("state_account:v2:seed-member", "", 13.699)
	if TraceMeasuredStateFactSame(seedOnly, bare) {
		t.Fatal("a fold without a shared account must not join its seed member's account")
	}

	partial := fold
	partial.ImpactMS = 11.635 // two of the three occurrences
	if TraceMeasuredStateFactSame(seat, partial) {
		t.Fatal("a fold of only some occurrences shows another number and stays separate")
	}
	overflow := fold
	overflow.OnChainOverflowFold = true
	if TraceMeasuredStateFactSame(seat, overflow) {
		t.Fatal("an overflow fold is a member maximum, not this account's occupancy")
	}
	other := fold
	other.MeasuredOccurrenceGroupKey = "state_account:v2:other"
	if TraceMeasuredStateFactSame(seat, other) {
		t.Fatal("a fold of another account must not join on an equal number")
	}

	// A single occurrence row keeps its own account: the group it belongs to
	// is not what that one row shows.
	occurrence := measuredFactTestNode("state_account:v2:own", "", 7.843)
	occurrence.MeasuredOccurrenceGroupKey = account
	if got := occurrence.MeasuredStateFactKey(); got != "state_account:v2:own" {
		t.Fatalf("occurrence identity=%q", got)
	}
	if TraceMeasuredStateFactSame(seat, occurrence) {
		t.Fatal("one occurrence is not the whole account")
	}
}

func TestUnanimousOccurrenceGroupKey(t *testing.T) {
	node := func(key string) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{MeasuredOccurrenceGroupKey: key}
	}
	for _, tc := range []struct {
		name string
		keys []string
		want string
	}{
		{"every member names one account", []string{"a", " a ", "a"}, "a"},
		{"members name different accounts", []string{"a", "b"}, ""},
		{"one member names none", []string{"a", ""}, ""},
		{"the seed names none", []string{"", "a"}, ""},
		{"no member names one", []string{"", ""}, ""},
	} {
		nodes := make([]TraceCausalProjectionNode, len(tc.keys))
		members := make([]int, len(tc.keys))
		for i, key := range tc.keys {
			nodes[i], members[i] = node(key), i
		}
		if got := traceCausalProjectionUnanimousOccurrenceGroupKey(nodes, members); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// HMC377 R-377-13 ①: one occurrence row names the multi-occurrence account it
// is part of; a ×N fold of the occurrences shows that account itself, so it
// names no group (the tree's constituent pointer is for a part, never for the
// row that already shows the whole).
func TestMeasuredOccurrenceGroupIsNamedByOneOccurrenceOnly(t *testing.T) {
	node := measuredFactTestNode("state_account:v2:own", "", 5)
	node.MeasuredOccurrenceGroupKey = " state_account:v2:group "
	if got := node.MeasuredOccurrenceGroup(); got != "state_account:v2:group" {
		t.Fatalf("one occurrence names its group: %q", got)
	}
	if got := node.MeasuredStateFactKey(); got != "state_account:v2:own" {
		t.Fatalf("one occurrence shows its own account: %q", got)
	}
	node.MergedCount = 3
	if got := node.MeasuredOccurrenceGroup(); got != "" {
		t.Fatalf("a fold names no group: %q", got)
	}
	if got := node.MeasuredStateFactKey(); got != "state_account:v2:group" {
		t.Fatalf("a fold shows the account its occurrences name: %q", got)
	}
}
