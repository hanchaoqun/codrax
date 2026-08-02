package tracequery

// state_account_identity.go — B7-T2 exact cross-publication identity.
//
// A scheduler state account can be published both as a root-rank seat and as
// a wakeup causal-impact view. Equal subject/state/value is not proof that
// the rows cover the same physical segments: disjoint waits may have the same
// duration, and their line envelopes may overlap. This file mints an opaque
// identity only from the complete, exact segment inventory.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func stateAccountIdentity(thread ThreadRef, state string, window TimeWindow, intervals []foldInterval, expectedMS float64) string {
	state = strings.TrimSpace(strings.ToLower(state))
	if thread.PID <= 0 || state == "" || window.EndTs <= window.StartTs ||
		len(intervals) == 0 || expectedMS <= 0 || math.IsNaN(expectedMS) || math.IsInf(expectedMS, 0) {
		return ""
	}
	ordered := append([]foldInterval(nil), intervals...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].start != ordered[j].start {
			return ordered[i].start < ordered[j].start
		}
		return ordered[i].end < ordered[j].end
	})
	total := 0.0
	var body strings.Builder
	fmt.Fprintf(&body, "v1|pid=%d|state=%s|window=%.9f..%.9f", thread.PID, state, window.StartTs, window.EndTs)
	lastEnd := 0.0
	for i, interval := range ordered {
		if interval.end <= interval.start || math.IsNaN(interval.start) || math.IsNaN(interval.end) ||
			math.IsInf(interval.start, 0) || math.IsInf(interval.end, 0) {
			return ""
		}
		// One thread cannot occupy the same scheduler state twice at once.
		// Overlap therefore means this is not a physical partition.
		if i > 0 && interval.start < lastEnd {
			return ""
		}
		total += (interval.end - interval.start) * 1000
		fmt.Fprintf(&body, "|%.9f..%.9f", interval.start, interval.end)
		lastEnd = interval.end
	}
	if math.Abs(total-expectedMS) >= types.TraceCausalProjectionSameValueTieMS {
		return ""
	}
	sum := sha256.Sum256([]byte(body.String()))
	return "state_account:v1:" + hex.EncodeToString(sum[:16])
}

func rootCauseRankStateIntervals(item RootCauseRankItem) []foldInterval {
	switch strings.TrimSpace(strings.ToLower(item.DominantState)) {
	case string(StateRunnable):
		return item.runnableIntervals
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
	}
	return 0
}

// stampStateAccountPublicationKeys joins exactly one active rank seat and one
// wakeup-impact publication. Ambiguity on either side publishes no key, so
// projection remains fail-open to the two visible rows.
func stampStateAccountPublicationKeys(chain *ChainResult, rank *RootCauseRankResult) {
	if chain == nil || rank == nil {
		return
	}
	for i := range chain.CausalImpacts {
		chain.CausalImpacts[i].StateAccountKey = ""
	}
	for i := range rank.Items {
		rank.Items[i].StateAccountKey = ""
	}
	impactByKey := map[string][]int{}
	for i := range chain.CausalImpacts {
		impact := &chain.CausalImpacts[i]
		key := stateAccountIdentity(impact.Thread, impact.DominantState, impact.Window,
			impact.stateAccountIntervals, impact.DominantImpactMs)
		if key != "" {
			impactByKey[key] = append(impactByKey[key], i)
		}
	}
	rankByKey := map[string][]int{}
	for i := range rank.Items {
		item := &rank.Items[i]
		value := rootCauseRankStateAccountValue(*item)
		if value <= 0 {
			continue
		}
		window := rank.Window
		if item.StatsWindowEndTs > item.StatsWindowStartTs {
			window = TimeWindow{StartTs: item.StatsWindowStartTs, EndTs: item.StatsWindowEndTs}
		}
		key := stateAccountIdentity(item.Thread, item.DominantState, window,
			rootCauseRankStateIntervals(*item), value)
		if key != "" {
			rankByKey[key] = append(rankByKey[key], i)
		}
	}
	for key, impactIndexes := range impactByKey {
		rankIndexes := rankByKey[key]
		if len(impactIndexes) != 1 || len(rankIndexes) != 1 {
			continue
		}
		chain.CausalImpacts[impactIndexes[0]].StateAccountKey = key
		rank.Items[rankIndexes[0]].StateAccountKey = key
	}
}

func stampResultStateAccountPublicationKeys(res *Result) {
	if res == nil {
		return
	}
	stampStateAccountPublicationKeys(res.WakeupChain, res.RootCauseRank)
	stampStateChurnAccountPublicationKeys(res.WindowStats, res.WakeupChain)
}

// stampStateChurnAccountPublicationKeys extends the exact rank<->wakeup join
// credential onto the whole-window state_churn face. The key has already been
// minted from the complete scheduler-segment inventories of one unambiguous
// rank/impact pair; this propagation only accepts the same thread, state,
// selected window, dominant impact, and five-state partition. It therefore
// lets display consumers collapse three publications of one physical account
// without treating an equal scalar from a different occurrence as identical.
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
