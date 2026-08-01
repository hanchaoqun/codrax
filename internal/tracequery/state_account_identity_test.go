package tracequery

import "testing"

func stateAccountTestIntervals() []foldInterval {
	return []foldInterval{
		{start: 11.0003, end: 11.0013},
		{start: 11.0020, end: 11.0040},
		{start: 11.0050, end: 11.0070},
	}
}

func stateAccountTestRank(intervals []foldInterval) RootCauseRankResult {
	return RootCauseRankResult{
		Window: TimeWindow{StartTs: 11, EndTs: 11.008},
		Items: []RootCauseRankItem{{
			Rank:              1,
			Type:              "runnable_wait",
			Thread:            ThreadRef{Comm: "app", PID: 20},
			DominantState:     string(StateRunnable),
			RunnableMs:        5,
			EffectiveImpactMs: 5,
			runnableIntervals: append([]foldInterval(nil), intervals...),
		}},
	}
}

func stateAccountTestImpact(intervals []foldInterval) WakeupCausalImpact {
	return WakeupCausalImpact{
		Thread:                ThreadRef{Comm: "app", PID: 20},
		Window:                TimeWindow{StartTs: 11, EndTs: 11.008},
		DominantState:         string(StateRunnable),
		DominantImpactMs:      5,
		stateAccountIntervals: append([]foldInterval(nil), intervals...),
	}
}

func TestStateAccountIdentityRequiresExactSegmentInventory(t *testing.T) {
	intervals := stateAccountTestIntervals()
	key := stateAccountIdentity(
		ThreadRef{Comm: "app", PID: 20},
		string(StateRunnable),
		TimeWindow{StartTs: 11, EndTs: 11.008},
		intervals,
		5,
	)
	if key == "" {
		t.Fatal("complete exact interval inventory must mint an identity")
	}

	// Same subject/state/window and the same 5ms scalar are not enough:
	// changing the physical partition must change the identity.
	different := []foldInterval{
		{start: 11.0000, end: 11.0020},
		{start: 11.0040, end: 11.0070},
	}
	other := stateAccountIdentity(
		ThreadRef{Comm: "app", PID: 20},
		string(StateRunnable),
		TimeWindow{StartTs: 11, EndTs: 11.008},
		different,
		5,
	)
	if other == "" || other == key {
		t.Fatalf("equal scalar on different physical segments must not join: first=%q other=%q", key, other)
	}

	overlap := []foldInterval{
		{start: 11.0000, end: 11.0030},
		{start: 11.0020, end: 11.0040},
	}
	if got := stateAccountIdentity(
		ThreadRef{Comm: "app", PID: 20},
		string(StateRunnable),
		TimeWindow{StartTs: 11, EndTs: 11.008},
		overlap,
		5,
	); got != "" {
		t.Fatalf("overlapping inventory is not an exact physical partition: %q", got)
	}
}

func TestStampStateAccountPublicationKeysExactPairAndAmbiguityFailOpen(t *testing.T) {
	intervals := stateAccountTestIntervals()
	rank := stateAccountTestRank(intervals)
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{stateAccountTestImpact(intervals)}}
	stampStateAccountPublicationKeys(&chain, &rank)
	if rank.Items[0].StateAccountKey == "" ||
		rank.Items[0].StateAccountKey != chain.CausalImpacts[0].StateAccountKey {
		t.Fatalf("one exact rank/impact pair must share one key: rank=%+v impacts=%+v", rank.Items, chain.CausalImpacts)
	}

	// Two impact publications under the same physical identity are ambiguous.
	// The producer must publish no join credential rather than guessing.
	rank = stateAccountTestRank(intervals)
	impact := stateAccountTestImpact(intervals)
	chain = ChainResult{CausalImpacts: []WakeupCausalImpact{impact, impact}}
	stampStateAccountPublicationKeys(&chain, &rank)
	if rank.Items[0].StateAccountKey != "" ||
		chain.CausalImpacts[0].StateAccountKey != "" ||
		chain.CausalImpacts[1].StateAccountKey != "" {
		t.Fatalf("ambiguous publications must fail open without a join key: rank=%+v impacts=%+v", rank.Items, chain.CausalImpacts)
	}
}
