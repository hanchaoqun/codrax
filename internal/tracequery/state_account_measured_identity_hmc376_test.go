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
		1, 1, 0, 0, pre, nil, nil, stateEdgeSegmentSplit{preMs: 1, postMs: 4, pre: pre, post: post}, rspaPlainSentence("pre-edge account"))
	if converted.measuredStateIntervals != nil {
		t.Error("the pre-edge seat kept the full-account inventory")
	}

	clone := mintStateEdgeRemainderClone(measuredIdentitySeat(StateRunnable, 5, 5, window, intervals),
		semanticHostEdgeAnchor{boundaryTs: 11.0015, direct: true}, 1, 5, 4, 0, 0, post, nil, nil,
		stateEdgeSegmentSplit{preMs: 1, postMs: 4, pre: pre, post: post}, rspaPlainSentence("post-edge account"))
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

// A seat priced differently from its measurement, without an inventory copied
// from a wakeup impact, answers from its own per-state segments. A stale copied
// inventory must not stop that answer.
func TestMeasuredStateAccountKeyFallsBackToTheSeatsOwnInventory(t *testing.T) {
	window := TimeWindow{StartTs: 11, EndTs: 11.008}
	intervals := stateAccountTestIntervals()
	want := stateAccountIdentity(ThreadRef{Comm: "app", PID: 20}, string(StateRunnable), window, intervals, 5)
	if want == "" {
		t.Fatal("fixture inventory must mint an identity")
	}
	for name, copied := range map[string][]foldInterval{
		"no copied inventory":    nil,
		"stale copied inventory": {{start: 11.0003, end: 11.0013}},
	} {
		seat := measuredIdentitySeat(StateRunnable, 5, 3, window, copied)
		seat.runnableIntervals = append([]foldInterval(nil), intervals...)
		rank := RootCauseRankResult{Window: window, Items: []RootCauseRankItem{seat}}
		stampStateAccountPublicationKeys(nil, &rank)
		if got := rank.Items[0]; got.StateAccountKey != "" || got.MeasuredStateAccountKey != want {
			t.Errorf("%s: pricing=%q measured=%q, want the seat's own account %q", name, got.StateAccountKey, got.MeasuredStateAccountKey, want)
		}
	}
}

func measuredAggregateOccurrence(start, end float64, intervals ...foldInterval) WakeupCausalImpact {
	total := 0.0
	for _, interval := range intervals {
		total += (interval.end - interval.start) * 1000
	}
	return WakeupCausalImpact{
		Thread: ThreadRef{Comm: "dep", PID: 21}, Window: TimeWindow{StartTs: start, EndTs: end},
		ChainDepth: 1, DominantState: string(StateRunnable), DominantImpactMs: total,
		TotalMs: total, RunnableMs: total, stateAccountIntervals: intervals,
	}
}

// A multi-occurrence account has one identity across every occurrence, shared
// by the aggregate, the rank seat built from it and each occurrence row. It is
// proved from the physical segments, so any inexact shape fails open.
func TestWakeupAggregateMeasuredAccountCoversEveryOccurrence(t *testing.T) {
	first := foldInterval{start: 11.000, end: 11.002}
	second := foldInterval{start: 11.004, end: 11.005}
	third := foldInterval{start: 11.007, end: 11.010}
	aggregateOf := func(impacts ...WakeupCausalImpact) ChainResult {
		chain := ChainResult{CausalImpacts: impacts}
		chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
		if len(chain.AggregatedImpacts) != 1 {
			t.Fatalf("fixture must aggregate into one account, got %d", len(chain.AggregatedImpacts))
		}
		stampStateAccountPublicationKeys(&chain, nil)
		return chain
	}
	thread := ThreadRef{Comm: "dep", PID: 21}
	want := stateAccountIdentity(thread, string(StateRunnable), TimeWindow{StartTs: 11, EndTs: 11.010},
		[]foldInterval{first, second, third}, 6)

	chain := aggregateOf(
		measuredAggregateOccurrence(11.000, 11.003, first),
		measuredAggregateOccurrence(11.003, 11.010, second, third),
	)
	aggregate := chain.AggregatedImpacts[0]
	if want == "" || aggregate.MeasuredStateAccountKey != want {
		t.Fatalf("aggregate identity=%q, want the hash of every occurrence's segments %q", aggregate.MeasuredStateAccountKey, want)
	}
	for i, impact := range chain.CausalImpacts {
		if impact.MeasuredOccurrenceGroupKey != want {
			t.Errorf("occurrence %d: group identity=%q, want %q", i, impact.MeasuredOccurrenceGroupKey, want)
		}
		if impact.StateAccountKey == "" || impact.StateAccountKey == want {
			t.Errorf("occurrence %d keeps its own account, got %q", i, impact.StateAccountKey)
		}
	}
	// The seat is priced differently from the account it shows.
	seat := rootCauseItemFromCausalAggregate(aggregate)
	seat.EffectiveImpactMs = 4
	rank := RootCauseRankResult{Window: TimeWindow{StartTs: 11, EndTs: 11.010}, Items: []RootCauseRankItem{seat}}
	stampStateAccountPublicationKeys(nil, &rank)
	if got := rank.Items[0]; got.StateAccountKey != "" || got.MeasuredStateAccountKey != want {
		t.Errorf("aggregate seat: pricing=%q measured=%q, want measured %q", got.StateAccountKey, got.MeasuredStateAccountKey, want)
	}
	for name, value := range map[string]any{"aggregate": aggregate, "occurrence": chain.CausalImpacts[0], "seat": rank.Items[0]} {
		wire, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(wire), want) {
			t.Errorf("%s: the account identity reached the stable JSON: %s", name, wire)
		}
	}

	// Branch windows that republish one physical segment are still one account.
	repeated := aggregateOf(
		measuredAggregateOccurrence(11.000, 11.003, first),
		measuredAggregateOccurrence(11.000, 11.003, first),
	)
	unionKey := stateAccountIdentity(thread, string(StateRunnable), TimeWindow{StartTs: 11, EndTs: 11.003}, []foldInterval{first}, 2)
	if got := repeated.AggregatedImpacts[0].MeasuredStateAccountKey; unionKey == "" || got != unionKey {
		t.Errorf("a republished segment: identity=%q, want the union's %q", got, unionKey)
	}

	// Anything less than the exact published account mints nothing.
	missing := measuredAggregateOccurrence(11.003, 11.010, second)
	missing.stateAccountIntervals = nil
	if got := aggregateOf(measuredAggregateOccurrence(11.000, 11.003, first), missing); got.AggregatedImpacts[0].MeasuredStateAccountKey != "" ||
		got.CausalImpacts[0].MeasuredOccurrenceGroupKey != "" {
		t.Errorf("an occurrence without an inventory leaves no account: %q", got.AggregatedImpacts[0].MeasuredStateAccountKey)
	}
	short := aggregateOf(measuredAggregateOccurrence(11.000, 11.003, first), measuredAggregateOccurrence(11.003, 11.010, second))
	short.AggregatedImpacts[0].DominantImpactMs = 9
	stampStateAccountPublicationKeys(&short, nil)
	if got := short.AggregatedImpacts[0].MeasuredStateAccountKey; got != "" {
		t.Errorf("segments that do not add up to the published time mint nothing: %q", got)
	}
	if got := wakeupAggregateStateAccountInventory(&chain, []int{0, 1}, string(StateSSleep)); got != nil {
		t.Errorf("occurrences dominated by another state have no inventory for this one: %v", got)
	}
	if got := wakeupAggregateStateAccountInventory(&chain, []int{0, 9}, string(StateRunnable)); got != nil {
		t.Errorf("a member outside the chain leaves no inventory: %v", got)
	}

	// A row outside the aggregation rule is not an occurrence of the account.
	outside := aggregateOf(
		measuredAggregateOccurrence(11.000, 11.003, first),
		measuredAggregateOccurrence(11.003, 11.010, second, third),
	)
	root := measuredAggregateOccurrence(11.000, 11.003, first)
	root.ChainDepth = 0
	outside.CausalImpacts = append(outside.CausalImpacts, root)
	stampStateAccountPublicationKeys(&outside, nil)
	if got := outside.CausalImpacts[2].MeasuredOccurrenceGroupKey; got != "" {
		t.Errorf("a row the aggregation excludes must not name the account: %q", got)
	}
}

// The same identity on a real capture: a wider window turns one wakeup impact
// into several occurrences of one account.
func TestWakeupAggregateMeasuredAccountPublic(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	run := func(view string) Result {
		return Run(idx, Query{View: view, PID: 59566, TimeStart: 34579.480, TimeEnd: 34579.510, Limit: 40,
			MaxBranches: 8, MaxChainNodes: 32, IncludeWindowStats: true, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	}
	chain, rank := run("wakeup_chain"), run("root_cause_rank")
	if chain.WakeupChain == nil || rank.RootCauseRank == nil {
		t.Fatal("public queries returned no wakeup chain or rank")
	}
	checked := 0
	for _, aggregate := range chain.WakeupChain.AggregatedImpacts {
		if aggregate.Thread.PID != 60595 || aggregate.DominantState != string(StateRunnable) {
			continue
		}
		checked++
		if aggregate.MeasuredStateAccountKey == "" {
			t.Fatal("the fixture's multi-occurrence runnable account minted no identity")
		}
		occurrences := 0
		for _, impact := range chain.WakeupChain.CausalImpacts {
			if impact.Thread.PID == 60595 && impact.DominantState == aggregate.DominantState {
				occurrences++
				if impact.MeasuredOccurrenceGroupKey != aggregate.MeasuredStateAccountKey {
					t.Errorf("occurrence names %q, aggregate is %q", impact.MeasuredOccurrenceGroupKey, aggregate.MeasuredStateAccountKey)
				}
			}
		}
		if occurrences != aggregate.OccurrenceCount {
			t.Errorf("occurrence rows=%d, aggregate counted %d", occurrences, aggregate.OccurrenceCount)
		}
		seats := 0
		for _, item := range rank.RootCauseRank.Items {
			if item.Thread.PID == 60595 && item.Source == "wakeup_chain.aggregated_impacts" {
				seats++
				if item.StateAccountKey != "" || item.MeasuredStateAccountKey != aggregate.MeasuredStateAccountKey {
					t.Errorf("seat: pricing=%q measured=%q, aggregate is %q", item.StateAccountKey, item.MeasuredStateAccountKey, aggregate.MeasuredStateAccountKey)
				}
			}
		}
		if seats == 0 {
			t.Fatal("the fixture no longer builds a rank seat from this aggregate")
		}
	}
	if checked != 1 {
		t.Fatalf("fixture changed: %d runnable aggregates for tid 60595", checked)
	}
}
