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
	// View windows are publication envelopes. The exact same absolute segment
	// inventory inside two enclosing views is one physical account.
	if got := stateAccountIdentity(
		ThreadRef{Comm: "app", PID: 20},
		string(StateRunnable),
		TimeWindow{StartTs: 10.5, EndTs: 11.5},
		intervals,
		5,
	); got != key {
		t.Fatalf("enclosing publication windows must not split one physical account: first=%q other=%q", key, got)
	}
	if got := stateAccountIdentity(
		ThreadRef{Comm: "app", PID: 20},
		string(StateRunnable),
		TimeWindow{StartTs: 11.001, EndTs: 11.008},
		intervals,
		5,
	); got != "" {
		t.Fatalf("a view that does not contain the complete inventory must fail open: %q", got)
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

func TestStampStateAccountPublicationKeysSupportsExactIOWaitAcrossViewWindows(t *testing.T) {
	intervals := []foldInterval{{start: 2.003, end: 2.014}}
	rank := RootCauseRankResult{
		Window: TimeWindow{StartTs: 2, EndTs: 2.020},
		Items: []RootCauseRankItem{{
			Rank: 1, Type: "io_wait", Thread: ThreadRef{Comm: "worker", PID: 400},
			DominantState: string(StateIOWait), IOWaitMs: 11, EffectiveImpactMs: 11,
			dioSegmentIntervals:   append([]foldInterval(nil), intervals...),
			dioSegmentIntervalsIO: append([]foldInterval(nil), intervals...),
		}},
	}
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{{
		Thread:        ThreadRef{Comm: "worker", PID: 400},
		Window:        TimeWindow{StartTs: 2.002, EndTs: 2.016},
		DominantState: string(StateIOWait), DominantImpactMs: 11,
		stateAccountIntervals: append([]foldInterval(nil), intervals...),
	}}}
	stampStateAccountPublicationKeys(&chain, &rank)
	if rank.Items[0].StateAccountKey == "" || rank.Items[0].StateAccountKey != chain.CausalImpacts[0].StateAccountKey {
		t.Fatalf("one exact IO account must survive differing enclosing view windows: rank=%q impact=%q",
			rank.Items[0].StateAccountKey, chain.CausalImpacts[0].StateAccountKey)
	}

	// Equal 11ms on a disjoint occurrence must never join.
	chain.CausalImpacts[0].stateAccountIntervals = []foldInterval{{start: 2.004, end: 2.015}}
	stampStateAccountPublicationKeys(&chain, &rank)
	if rank.Items[0].StateAccountKey == "" || chain.CausalImpacts[0].StateAccountKey == "" ||
		rank.Items[0].StateAccountKey == chain.CausalImpacts[0].StateAccountKey {
		t.Fatalf("equal IO scalar on disjoint physical segments must keep distinct identities: rank=%q impact=%q",
			rank.Items[0].StateAccountKey, chain.CausalImpacts[0].StateAccountKey)
	}
}

func TestStampStateAccountPublicationKeysAreIndependentOfViewCoPublication(t *testing.T) {
	intervals := stateAccountTestIntervals()
	rank := stateAccountTestRank(intervals)
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{stateAccountTestImpact(intervals)}}
	stampStateAccountPublicationKeys(&chain, &rank)
	if rank.Items[0].StateAccountKey == "" ||
		rank.Items[0].StateAccountKey != chain.CausalImpacts[0].StateAccountKey {
		t.Fatalf("one exact rank/impact pair must share one key: rank=%+v impacts=%+v", rank.Items, chain.CausalImpacts)
	}

	// Repeated impact publications of the same exact physical inventory retain
	// the same identity; the projection consumer owns keeper ambiguity.
	rank = stateAccountTestRank(intervals)
	impact := stateAccountTestImpact(intervals)
	chain = ChainResult{CausalImpacts: []WakeupCausalImpact{impact, impact}}
	stampStateAccountPublicationKeys(&chain, &rank)
	want := rank.Items[0].StateAccountKey
	if want == "" || chain.CausalImpacts[0].StateAccountKey != want || chain.CausalImpacts[1].StateAccountKey != want {
		t.Fatalf("repeated exact publications must carry the same physical identity: rank=%+v impacts=%+v", rank.Items, chain.CausalImpacts)
	}

	standaloneImpact := ChainResult{CausalImpacts: []WakeupCausalImpact{stateAccountTestImpact(intervals)}}
	stampStateAccountPublicationKeys(&standaloneImpact, nil)
	standaloneRank := stateAccountTestRank(intervals)
	stampStateAccountPublicationKeys(nil, &standaloneRank)
	if standaloneImpact.CausalImpacts[0].StateAccountKey == "" ||
		standaloneImpact.CausalImpacts[0].StateAccountKey != standaloneRank.Items[0].StateAccountKey {
		t.Fatalf("query order/view shape must not suppress an exact account identity: rank=%+v impact=%+v",
			standaloneRank.Items, standaloneImpact.CausalImpacts)
	}
}

func TestStampResultStateAccountPublicationKeysCarriesExactIdentityToStateChurn(t *testing.T) {
	intervals := stateAccountTestIntervals()
	res := Result{
		WindowStats: &WindowStats{
			Window: TimeWindow{StartTs: 11, EndTs: 11.008},
			StateChurn: []ThreadStateChurnSummary{{
				Thread: ThreadRef{Comm: "app", PID: 20}, DominantState: string(StateRunnable),
				DominantImpactMs: 5, RunningMs: 3, RunnableMs: 5,
			}},
		},
		RootCauseRank: func() *RootCauseRankResult { v := stateAccountTestRank(intervals); return &v }(),
		WakeupChain: &ChainResult{CausalImpacts: []WakeupCausalImpact{func() WakeupCausalImpact {
			v := stateAccountTestImpact(intervals)
			v.RunningMs = 3
			v.RunnableMs = 5
			return v
		}()}},
	}
	stampResultStateAccountPublicationKeys(&res)
	got := res.WindowStats.StateChurn[0].StateAccountKey
	if got == "" || got != res.RootCauseRank.Items[0].StateAccountKey || got != res.WakeupChain.CausalImpacts[0].StateAccountKey {
		t.Fatalf("one physical state account must share one key across churn/rank/wakeup: churn=%q rank=%q wakeup=%q",
			got, res.RootCauseRank.Items[0].StateAccountKey, res.WakeupChain.CausalImpacts[0].StateAccountKey)
	}

	res.WindowStats.StateChurn[0].RunningMs = 2.5
	stampResultStateAccountPublicationKeys(&res)
	if res.WindowStats.StateChurn[0].StateAccountKey != "" {
		t.Fatalf("different five-state partition must not inherit the key: %+v", res.WindowStats.StateChurn[0])
	}
}
