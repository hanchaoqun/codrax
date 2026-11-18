package tracequery

// state_account_identity.go — B7-T2 exact cross-publication identity.
//
// A scheduler state account can be published both as a root-rank seat and as
// a wakeup causal-impact view. Equal subject/state/value is not proof that
// the rows cover the same physical segments: disjoint waits may have the same
// duration, and their line envelopes may overlap. This file mints an opaque
// identity only from the complete, exact segment inventory.
//
// One inventory hash answers two different questions, so a rank seat carries
// it on two fields:
//   - StateAccountKey: "this seat is PRICED exactly at this account". Minted
//     only when the seat's effective impact equals the measured state time;
//     the projection's one-seat convergence folds on it.
//   - MeasuredStateAccountKey: "this seat SHOWS this measured account".
//     Minted regardless of price, so display rows of one measurement can be
//     recognised without ever merging an estimate with a measurement.

import (
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func stateAccountIdentity(thread ThreadRef, state string, window TimeWindow, intervals []foldInterval, expectedMS float64) string {
	physical := make([][2]float64, len(intervals))
	for i, interval := range intervals {
		physical[i] = [2]float64{interval.start, interval.end}
	}
	return types.TraceSchedulerStateAccountIdentity(thread.PID, state, window.StartTs, window.EndTs, physical, expectedMS)
}

func rootCauseRankStateIntervals(item RootCauseRankItem) []foldInterval {
	switch strings.TrimSpace(strings.ToLower(item.DominantState)) {
	case string(StateRunnable):
		return item.runnableIntervals
	case string(StateDSleep):
		return item.dioSegmentIntervalsD
	case string(StateIOWait):
		return item.dioSegmentIntervalsIO
	}
	return nil
}

func rootCauseRankStateAccountValue(item RootCauseRankItem) float64 {
	switch strings.TrimSpace(strings.ToLower(item.DominantState)) {
	case string(StateRunnable):
		if item.RunnableMs <= 0 ||
			math.Abs(rootCauseEffectiveImpactMs(item)-item.RunnableMs) >= types.TraceCausalProjectionSameValueTieMS {
			return 0
		}
		return item.RunnableMs
	case string(StateDSleep):
		if item.DStateMs <= 0 ||
			math.Abs(rootCauseEffectiveImpactMs(item)-item.DStateMs) >= types.TraceCausalProjectionSameValueTieMS {
			return 0
		}
		return item.DStateMs
	case string(StateIOWait):
		if item.IOWaitMs <= 0 ||
			math.Abs(rootCauseEffectiveImpactMs(item)-item.IOWaitMs) >= types.TraceCausalProjectionSameValueTieMS {
			return 0
		}
		return item.IOWaitMs
	}
	return 0
}

// stampStateAccountPublicationKeys stamps every independently provable
// scheduler-state account. A wakeup-only query must carry the same physical
// identity as a later root-rank query; requiring both views to coexist in one
// Result would make identity depend on exploration order. Consumers decide
// convergence from the typed key and retain their own keeper-ambiguity gate.
func stampStateAccountPublicationKeys(chain *ChainResult, rank *RootCauseRankResult) {
	if chain != nil {
		for i := range chain.CausalImpacts {
			impact := &chain.CausalImpacts[i]
			impact.StateAccountKey = stateAccountIdentity(
				impact.Thread, impact.DominantState, impact.Window,
				impact.stateAccountIntervals, impact.DominantImpactMs,
			)
		}
	}
	if rank != nil {
		for i := range rank.Items {
			item := &rank.Items[i]
			item.StateAccountKey = ""
			item.MeasuredStateAccountKey = rootCauseRankMeasuredStateAccountIdentity(*item, rank.Window)
			value := rootCauseRankStateAccountValue(*item)
			if value <= 0 {
				continue
			}
			window := rank.Window
			if item.StatsWindowEndTs > item.StatsWindowStartTs {
				window = TimeWindow{StartTs: item.StatsWindowStartTs, EndTs: item.StatsWindowEndTs}
			}
			item.StateAccountKey = stateAccountIdentity(
				item.Thread, item.DominantState, window,
				rootCauseRankStateIntervals(*item), value,
			)
			if item.StateAccountKey != "" {
				// A seat priced exactly at an account shows that account.
				item.MeasuredStateAccountKey = item.StateAccountKey
			}
		}
	}
}

// rootCauseRankMeasuredStateAccountIdentity mints the identity of the seat's
// measured dominant-state account. It never reads the seat's price. The
// inventory copied from the seat's own wakeup impact answers first; a seat
// without one (or whose state totals were rewritten since) falls back to its
// own per-state segment inventory. The hash refuses any inventory whose sum
// differs from the published state time, that overlaps, or that leaves the
// window, so a stale or partial inventory yields no identity.
func rootCauseRankMeasuredStateAccountIdentity(item RootCauseRankItem, rankWindow TimeWindow) string {
	var measured float64
	switch strings.TrimSpace(strings.ToLower(item.DominantState)) {
	case string(StateRunning):
		measured = item.RunningMs
	case string(StateRunnable):
		measured = item.RunnableMs
	case string(StateSSleep):
		measured = item.SleepMs
	case string(StateDSleep):
		measured = item.DStateMs
	case string(StateIOWait):
		measured = item.IOWaitMs
	}
	if measured <= 0 {
		return ""
	}
	if key := stateAccountIdentity(item.Thread, item.DominantState, item.measuredStateWindow, item.measuredStateIntervals, measured); key != "" {
		return key
	}
	window := rankWindow
	if item.StatsWindowEndTs > item.StatsWindowStartTs {
		window = TimeWindow{StartTs: item.StatsWindowStartTs, EndTs: item.StatsWindowEndTs}
	}
	return stateAccountIdentity(item.Thread, item.DominantState, window, rootCauseRankStateIntervals(item), measured)
}

func stampResultStateAccountPublicationKeys(res *Result) {
	if res == nil {
		return
	}
	stampStateAccountPublicationKeys(res.WakeupChain, res.RootCauseRank)
	stampStateChurnAccountPublicationKeys(res.WindowStats, res.WakeupChain)
}

// stampStateChurnAccountPublicationKeys extends an exact wakeup account
// identity onto the whole-window state_churn face. The key was minted from a
// complete scheduler-segment inventory; this propagation only accepts the
// same thread, state, selected window, dominant impact, and five-state
// partition. It therefore lets display consumers collapse three publications
// of one physical account without treating an equal scalar from a different
// occurrence as identical.
func stampStateChurnAccountPublicationKeys(stats *WindowStats, chain *ChainResult) {
	if stats == nil || chain == nil {
		return
	}
	for i := range stats.StateChurn {
		stats.StateChurn[i].StateAccountKey = ""
		churn := &stats.StateChurn[i]
		keys := map[string]bool{}
		for j := range chain.CausalImpacts {
			impact := &chain.CausalImpacts[j]
			key := strings.TrimSpace(impact.StateAccountKey)
			if key == "" || !sameThreadRef(churn.Thread, impact.Thread) ||
				!strings.EqualFold(strings.TrimSpace(churn.DominantState), strings.TrimSpace(impact.DominantState)) ||
				math.Abs(stats.Window.StartTs-impact.Window.StartTs) > 1e-9 ||
				math.Abs(stats.Window.EndTs-impact.Window.EndTs) > 1e-9 ||
				!sameStateAccountMetric(churn.DominantImpactMs, impact.DominantImpactMs) ||
				!sameStateAccountMetric(churn.RunningMs, impact.RunningMs) ||
				!sameStateAccountMetric(churn.RunnableMs, impact.RunnableMs) ||
				!sameStateAccountMetric(churn.SleepMs, impact.SleepMs) ||
				!sameStateAccountMetric(churn.DStateMs, impact.DStateMs) ||
				!sameStateAccountMetric(churn.IOWaitMs, impact.IOWaitMs) {
				continue
			}
			keys[key] = true
		}
		if len(keys) == 1 {
			for key := range keys {
				churn.StateAccountKey = key
			}
		}
	}
}

func sameStateAccountMetric(a, b float64) bool {
	return !math.IsNaN(a) && !math.IsNaN(b) && !math.IsInf(a, 0) && !math.IsInf(b, 0) &&
		math.Abs(a-b) < types.TraceCausalProjectionSameValueTieMS
}
