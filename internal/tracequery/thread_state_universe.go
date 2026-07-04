package tracequery

// thread_state_universe.go — TSH batch (typed-signal hygiene, 2026-07-05): the
// single authoritative home for (a) the ThreadState universe list, (b) the
// dominant-lane sub-universe (which members own a per-state ms accumulator
// lane), and (c) the shared state-classification predicates that used to live
// as duplicated branches (same failure family as the §7.4 selected_window
// token incident: scattered twin switches, one missed = silent
// misclassification — the §7.11 B-1 StateStopped/StateDead batch had to hand
// patch every copy).
//
// Bidirectional pin (thread_state_universe_pin_test.go, NEW-7 model):
//   - production direction: every universe member has a witnessed production
//     point (stateFromPrevState code table, timeline running intervals, the
//     D→io_wait blocked-reason reclassification);
//   - consumption direction, two lanes: (1) every switch in this package's
//     non-test files whose case list names ThreadState members either covers
//     the universe, carries a default, or declares its fall-through members
//     in the pin ledger with a rationale; (2) every ==/!= comparison against
//     a ThreadState member (bare State* or string(State*)) is registered in
//     a per-function literal golden with a matched-count sentinel (TSH review
//     F3 — the if-comparison consumer surface is as real as the switches and
//     was previously outside the pin). Adding a member without teaching every
//     consumer (or ledgering the skip) is a test failure, not a code review
//     hope.
//
// Behavior contract: the shared predicates below are BYTE-BEHAVIOR extractions
// of the historical inline branches — truth tables compared site by site
// before replacement; no site semantics changed. Divergent look-alikes (e.g.
// rootCauseItemHasDStateOrIO, which TrimSpaces its input) were deliberately
// NOT unified and stay at their call sites.

// threadStateUniverse is the closed ThreadState member list. Must stay in
// lockstep with the const block in types.go (pinned by AST parity +
// hardcoded golden in the pin test).
var threadStateUniverse = []ThreadState{
	StateRunning,
	StateRunnable,
	StateSSleep,
	StateDSleep,
	StateIOWait,
	StateStopped,
	StateDead,
	StateUnknown,
}

// threadStateDominantLanes is the dominant-state lane sub-universe IN
// PRIORITY ORDER: these five members own per-state ms accumulator lanes
// (running/runnable/sleep/d_state/io_wait), and every DominantState string
// published anywhere in this package is drawn from this list. The order is
// the tie-break priority of the dominant pick (first entry wins unless a
// later lane is STRICTLY larger) — io_wait > d_sleep > runnable > s_sleep >
// running, exactly the historical candidates-array order shared by the churn,
// peer-breakdown and causal-impact faces. stopped/dead/unknown have NO lane
// by the §7.11 B-1 ruling (their time is neither scheduling demand nor
// compute delivery; booking it into any lane would be a semantic lie).
var threadStateDominantLanes = []ThreadState{
	StateIOWait,
	StateDSleep,
	StateRunnable,
	StateSSleep,
	StateRunning,
}

// ThreadStateDominantLaneUniverse returns a copy of the dominant-lane
// sub-universe. Exported ONLY for the cross-package bridge pin: every string
// this package can publish as a dominant_state note value must be a
// registered projection state-kind word (internal/types
// TraceStateKindRegistered) — see the tool-side pin test.
func ThreadStateDominantLaneUniverse() []ThreadState {
	out := make([]ThreadState, len(threadStateDominantLanes))
	copy(out, threadStateDominantLanes)
	return out
}

// stateChurnWakeupReopenIneligible is the wakeup-side twin of the open gate:
// a sched_wakeup/sched_waking event closes the open churn segment and reopens
// it as runnable ONLY when the open state is an off-CPU wait — an already
// running/runnable open segment is left untouched (the wakeup adds no state
// edge). Shared authority for the previously hand-synced byte-identical
// guard halves in computeStateChurnSummaries (query.go) and StreamStateCluster
// (stream_search.go) — the same twin pair that shares stateChurnOpenIneligible
// (TSH review F3a; the two source lines were diffed byte-identical before the
// extraction, behavior unchanged).
func stateChurnWakeupReopenIneligible(state ThreadState) bool {
	return state == StateRunning || state == StateRunnable
}

// stateChurnOpenIneligible is THE churn-open eligibility gate shared by the
// indexed face (computeStateChurnSummaries) and the streaming face
// (streamStateClusterSearch) — the two copies were previously identical
// if-expressions coupled only by a comment. §7.11 B-1 sequel semantics,
// unchanged: churn tracks ACTIVE scheduling states only; stopped/dead are
// rejected exactly like Unknown and never open a churn segment, because the
// five per-state lanes skip them and an open stopped/dead segment would
// inflate ONLY fragments/switches/maxSegment (a dead exit tail then trips the
// maxSegment>=70% suppression gate and kills a REAL churn row).
func stateChurnOpenIneligible(thread ThreadRef, state ThreadState) bool {
	if thread.PID <= 0 {
		return true
	}
	switch state {
	case StateUnknown, StateStopped, StateDead:
		return true
	}
	return false
}

// dominantStateFromLanes is the shared 5-lane dominant-state pick previously
// hand-rolled four times (dominantChurnState, dominantThreadStateBreakdown,
// dominantCausalImpactState, dominantAggregateState) with an identical
// candidates array. The winner
// is the largest lane; ties keep the earlier threadStateDominantLanes entry
// (strict > comparison), byte-identical to the historical arrays.
func dominantStateFromLanes(runningMs, runnableMs, sleepMs, dStateMs, ioWaitMs float64) (string, float64) {
	laneMs := func(state ThreadState) float64 {
		switch state {
		case StateRunning:
			return runningMs
		case StateRunnable:
			return runnableMs
		case StateSSleep:
			return sleepMs
		case StateDSleep:
			return dStateMs
		case StateIOWait:
			return ioWaitMs
		}
		return 0
	}
	best := threadStateDominantLanes[0]
	bestMs := laneMs(best)
	for _, lane := range threadStateDominantLanes[1:] {
		if ms := laneMs(lane); ms > bestMs {
			best, bestMs = lane, ms
		}
	}
	return string(best), bestMs
}

// dominantStateIsDStateOrIOWait reports whether a published DominantState
// string names the uninterruptible/IO blocking family — the shared form of
// the four identical `== string(StateDSleep) || == string(StateIOWait)`
// blocking-magnitude branches (aggregate/causal, projected/actual). Exact
// match on the verbatim published token; NOT the same caliber as
// rootCauseItemHasDStateOrIO (which TrimSpaces its input and stays separate).
func dominantStateIsDStateOrIOWait(dominant string) bool {
	switch dominant {
	case string(StateDSleep), string(StateIOWait):
		return true
	}
	return false
}

// rootTypeForDominantState maps a dominant scheduler state to its root-cause
// wire token — the single authority for the previously duplicated
// aggregateRootCauseType / causalImpactRootType switches (identical bodies on
// two structs). priorityInversion is the caller's typed inversion flag
// (aggregate: PriorityInversion; impact: PriorityInversionCandidate); ioWaitMs
// disambiguates the D-sleep family exactly as both copies always did.
func rootTypeForDominantState(dominant string, priorityInversion bool, ioWaitMs float64) string {
	switch dominant {
	case string(StateRunnable):
		if priorityInversion {
			return "priority_inversion_runnable_wait"
		}
		return "runnable_wait"
	case string(StateDSleep):
		if ioWaitMs > 0 {
			return "io_wait"
		}
		return "d_state_or_io_wait"
	case string(StateIOWait):
		return "io_wait"
	case string(StateSSleep):
		return "sleep_wait"
	case string(StateRunning):
		return "running"
	default:
		return "unknown_state"
	}
}
