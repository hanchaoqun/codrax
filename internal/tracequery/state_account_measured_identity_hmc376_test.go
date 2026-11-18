package tracequery

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// A rank seat built from a wakeup impact carries the identity of the measured
// account it shows whether or not its price equals that measurement, while the
// pricing identity keeps its rule: empty unless priced exactly at the account.
func TestMeasuredStateAccountKeyIsIndependentOfPricePublic(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	run := func(view string) Result {
		return Run(idx, Query{View: view, PID: 59566, TimeStart: 34579.490, TimeEnd: 34579.500, Limit: 40,
			MaxBranches: 8, MaxChainNodes: 32, IncludeWindowStats: true, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	}
	chain, rank := run("wakeup_chain"), run("root_cause_rank")
	if chain.WakeupChain == nil || rank.RootCauseRank == nil {
		t.Fatal("public queries returned no wakeup chain or rank")
	}
	impactKey := map[int]string{}
	for _, impact := range chain.WakeupChain.CausalImpacts {
		if impact.StateAccountKey != "" {
			impactKey[impact.Thread.PID] = impact.StateAccountKey
		}
	}
	// Two shapes of one class: a seat priced differently from its measured
	// runnable account, and a separately priced sub-seat beside a sleep account.
	for _, tc := range []struct {
		tid   int
		state ThreadState
	}{{60595, StateRunnable}, {59843, StateSSleep}} {
		var seat *RootCauseRankItem
		for i := range rank.RootCauseRank.Items {
			item := &rank.RootCauseRank.Items[i]
			if item.Thread.PID == tc.tid && item.Type == "priority_inversion_candidate" {
				seat = item
			}
		}
		if seat == nil {
			t.Fatalf("tid %d: expected a priority seat built from the wakeup impact", tc.tid)
		}
		if seat.DominantState != string(tc.state) {
			t.Fatalf("tid %d: fixture changed, dominant state=%q", tc.tid, seat.DominantState)
		}
		if seat.StateAccountKey != "" {
			t.Errorf("tid %d: a seat priced differently from its measurement must not claim the pricing identity: %q", tc.tid, seat.StateAccountKey)
		}
		if seat.MeasuredStateAccountKey == "" || seat.MeasuredStateAccountKey != impactKey[tc.tid] {
			t.Errorf("tid %d: measured identity %q must equal the wakeup view's native identity %q", tc.tid, seat.MeasuredStateAccountKey, impactKey[tc.tid])
		}
		wire, err := json.Marshal(seat)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(wire), "measured_state_account") || strings.Contains(string(wire), seat.MeasuredStateAccountKey) {
			t.Errorf("tid %d: the measured identity reached the stable rank wire: %s", tc.tid, wire)
		}
	}
	// Every seat that already published the pricing identity shows that same
	// account.
	for _, item := range rank.RootCauseRank.Items {
		if item.StateAccountKey != "" && item.MeasuredStateAccountKey != item.StateAccountKey {
			t.Errorf("%s-%d: pricing identity %q and measured identity %q disagree", item.Thread.Comm, item.Thread.PID, item.StateAccountKey, item.MeasuredStateAccountKey)
		}
	}
}

func measuredIdentitySeat(state ThreadState, measuredMs, pricedMs float64, window TimeWindow, intervals []foldInterval) RootCauseRankItem {
	item := RootCauseRankItem{
		Rank: 1, Type: "priority_inversion_candidate", Thread: ThreadRef{Comm: "app", PID: 20},
		DominantState: string(state), EffectiveImpactMs: pricedMs, ImpactMs: pricedMs,
		measuredStateWindow:    window,
		measuredStateIntervals: append([]foldInterval(nil), intervals...),
	}
	switch state {
	case StateRunning:
		item.RunningMs = measuredMs
	case StateRunnable:
		item.RunnableMs = measuredMs
	case StateSSleep:
		item.SleepMs = measuredMs
	case StateDSleep:
		item.DStateMs = measuredMs
	case StateIOWait:
		item.IOWaitMs = measuredMs
	}
	return item
}

func TestStampMeasuredStateAccountKey(t *testing.T) {
	window := TimeWindow{StartTs: 11, EndTs: 11.008}
	intervals := stateAccountTestIntervals() // 1 + 2 + 2 = 5 ms
	stamp := func(item RootCauseRankItem, impact WakeupCausalImpact) (RootCauseRankItem, WakeupCausalImpact) {
		rank := RootCauseRankResult{Window: window, Items: []RootCauseRankItem{item}}
		chain := ChainResult{CausalImpacts: []WakeupCausalImpact{impact}}
		stampStateAccountPublicationKeys(&chain, &rank)
		return rank.Items[0], chain.CausalImpacts[0]
	}

	// Priced differently from the measurement: the measured identity is the
	// wakeup view's identity, the pricing identity stays withheld.
	seat, impact := stamp(measuredIdentitySeat(StateRunnable, 5, 3, window, intervals), stateAccountTestImpact(intervals))
	if seat.StateAccountKey != "" || seat.MeasuredStateAccountKey == "" || seat.MeasuredStateAccountKey != impact.StateAccountKey {
		t.Fatalf("priced != measured: pricing=%q measured=%q impact=%q", seat.StateAccountKey, seat.MeasuredStateAccountKey, impact.StateAccountKey)
	}

	// A seat priced exactly at its account keeps the pricing identity and
	// shows the same account.
	priced := stateAccountTestRank(intervals)
	stampStateAccountPublicationKeys(nil, &priced)
	if priced.Items[0].StateAccountKey == "" || priced.Items[0].MeasuredStateAccountKey != priced.Items[0].StateAccountKey ||
		priced.Items[0].StateAccountKey != impact.StateAccountKey {
		t.Fatalf("priced == measured: pricing=%q measured=%q", priced.Items[0].StateAccountKey, priced.Items[0].MeasuredStateAccountKey)
	}

	// The same duration on different physical segments is a different account.
	other := []foldInterval{{start: 11.0000, end: 11.0020}, {start: 11.0040, end: 11.0070}}
	otherSeat, _ := stamp(measuredIdentitySeat(StateRunnable, 5, 3, window, other), stateAccountTestImpact(intervals))
	if otherSeat.MeasuredStateAccountKey == "" || otherSeat.MeasuredStateAccountKey == seat.MeasuredStateAccountKey {
		t.Fatalf("equal duration on disjoint segments must not share an identity: %q vs %q", otherSeat.MeasuredStateAccountKey, seat.MeasuredStateAccountKey)
	}

	// An inventory that is not exactly the published account mints nothing.
	for name, bad := range map[string]RootCauseRankItem{
		"sum differs from the published state time": measuredIdentitySeat(StateRunnable, 4, 3, window, intervals),
		"overlapping segments":                      measuredIdentitySeat(StateRunnable, 5, 3, window, []foldInterval{{start: 11.0000, end: 11.0030}, {start: 11.0020, end: 11.0040}}),
		"segment outside the statistics window":     measuredIdentitySeat(StateRunnable, 5, 3, TimeWindow{StartTs: 11.001, EndTs: 11.008}, intervals),
		"no inventory at all":                       measuredIdentitySeat(StateRunnable, 5, 3, window, nil),
		"no measured state time":                    measuredIdentitySeat(StateRunnable, 0, 3, window, intervals),
	} {
		if got, _ := stamp(bad, stateAccountTestImpact(intervals)); got.MeasuredStateAccountKey != "" || got.StateAccountKey != "" {
			t.Errorf("%s: an inexact inventory must fail open, got measured=%q pricing=%q", name, got.MeasuredStateAccountKey, got.StateAccountKey)
		}
	}

	// All five accounting states mint; each equals the wakeup view of the same
	// state and differs from the other states over the same segments.
	seen := map[string]ThreadState{}
	for _, state := range []ThreadState{StateRunning, StateRunnable, StateSSleep, StateDSleep, StateIOWait} {
		view := stateAccountTestImpact(intervals)
		view.DominantState = string(state)
		got, gotView := stamp(measuredIdentitySeat(state, 5, 3, window, intervals), view)
		if got.MeasuredStateAccountKey == "" || got.MeasuredStateAccountKey != gotView.StateAccountKey {
			t.Errorf("%s: measured=%q view=%q", state, got.MeasuredStateAccountKey, gotView.StateAccountKey)
		}
		if prior, dup := seen[got.MeasuredStateAccountKey]; dup {
			t.Errorf("%s and %s share one identity", state, prior)
		}
		seen[got.MeasuredStateAccountKey] = state
	}
	// A state without a scheduler-state account mints nothing.
	if got, _ := stamp(measuredIdentitySeat(ThreadState("stopped"), 5, 3, window, intervals), stateAccountTestImpact(intervals)); got.MeasuredStateAccountKey != "" {
		t.Errorf("stopped: no state account, got %q", got.MeasuredStateAccountKey)
	}

	// A seat whose state totals were rewritten after construction does not
	// keep the pre-rewrite identity.
	rewritten := measuredIdentitySeat(StateRunnable, 5, 3, window, intervals)
	rewritten.RunnableMs = 3.5
	if got, _ := stamp(rewritten, stateAccountTestImpact(intervals)); got.MeasuredStateAccountKey != "" {
		t.Errorf("rewritten state total kept the pre-rewrite identity: %q", got.MeasuredStateAccountKey)
	}
	// ... and when the rewrite installs its own exact inventory, the seat
	// shows that new account, not the copied one.
	clipped := []foldInterval{{start: 11.0003, end: 11.0013}, {start: 11.0020, end: 11.0040}} // 3 ms
	rewritten = measuredIdentitySeat(StateRunnable, 5, 3, window, intervals)
	rewritten.RunnableMs = 3
	rewritten.runnableIntervals = clipped
	got, _ := stamp(rewritten, stateAccountTestImpact(intervals))
	if got.MeasuredStateAccountKey == "" || got.MeasuredStateAccountKey == seat.MeasuredStateAccountKey ||
		got.MeasuredStateAccountKey != stateAccountIdentity(rewritten.Thread, string(StateRunnable), window, clipped, 3) {
		t.Errorf("rewritten seat must show its own exact account: %q", got.MeasuredStateAccountKey)
	}
}

// The three places that rewrite a seat's state totals after construction drop
// the inventory copied from the wakeup impact.
func TestMeasuredStateInventoryIsClearedWhereStateTotalsAreRewritten(t *testing.T) {
	window := TimeWindow{StartTs: 11, EndTs: 11.008}
	intervals := stateAccountTestIntervals()
	pre := []foldInterval{intervals[0]}
	post := []foldInterval{intervals[1], intervals[2]}

	converted := measuredIdentitySeat(StateRunnable, 5, 5, window, intervals)
	convertStateSeatToEdgeAnchored(&converted, semanticHostEdgeAnchor{boundaryTs: 11.0015, direct: true},
		1, 1, 0, 0, pre, nil, nil, stateEdgeSegmentSplit{preMs: 1, postMs: 4, pre: pre, post: post}, "pre-edge account")
	if converted.measuredStateIntervals != nil {
		t.Error("the pre-edge seat kept the full-account inventory")
	}

	clone := mintStateEdgeRemainderClone(measuredIdentitySeat(StateRunnable, 5, 5, window, intervals),
		semanticHostEdgeAnchor{boundaryTs: 11.0015, direct: true}, 1, 5, 4, 0, 0, post, nil, nil,
		stateEdgeSegmentSplit{preMs: 1, postMs: 4, pre: pre, post: post}, "post-edge account")
	if clone.measuredStateIntervals != nil {
		t.Error("the post-edge twin kept the full-account inventory")
	}

	a := rcmRankItem("runnable_wait", ThreadRef{Comm: "app", PID: 20}, 3, 11.000, 11.003, 2, 6)
	a.measuredStateIntervals, a.measuredStateWindow = []foldInterval{{start: 11.000, end: 11.003}}, window
	b := rcmRankItem("runnable_wait", ThreadRef{Comm: "app", PID: 20}, 2, 11.004, 11.006, 7, 9)
	b.measuredStateIntervals, b.measuredStateWindow = []foldInterval{{start: 11.004, end: 11.006}}, window
	merged := mergeSameThreadTypeRankFamily(Query{TimeStart: 11, TimeEnd: 11.008}, false, []RootCauseRankItem{a, b}, []int{0, 1})
	if merged.MemberCount != 2 || merged.measuredStateIntervals != nil {
		t.Errorf("a family value kept its representative's inventory: members=%d inventory=%v", merged.MemberCount, merged.measuredStateIntervals)
	}
	single := mergeSameThreadTypeRankFamily(Query{TimeStart: 11, TimeEnd: 11.008}, false, []RootCauseRankItem{a}, []int{0})
	if len(single.measuredStateIntervals) != 1 {
		t.Errorf("a one-member family is that member's own account and keeps its inventory: %v", single.measuredStateIntervals)
	}
}
