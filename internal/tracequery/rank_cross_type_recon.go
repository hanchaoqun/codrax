package tracequery

// rank_cross_type_recon.go — B4 exact cross-type rank-seat reconciliation
// (external audit ruling accepted 2026-07-10).
//
// computeIOBurstEpisodes deliberately projects every DStateTop segment into
// an io_burst_episode resource view. buildRootCauseRankFrom then used to mint
// both that resource view and the source d_state_or_io_wait state view as two
// competing rank rows. On the production witness they had the same numeric
// thread, query window, physical interval and evidence lines (1.062ms), so
// one physical wait occupied two board seats.
//
// This pass is intentionally an adjudicated exact-match table, not a generic
// similarity fold. d_state_or_io_wait owns the seat because it is the formal
// scheduler-state cause lane; io_burst_episode remains a lossless supporting
// observation in RootCauseRankResult.AbsorbedItems. Any missing or unequal
// precise dimension fails open to the former two-row publication:
//
//   1. numeric TID identity (both PID fields positive and exactly equal);
//   2. exact adjudicated type+producer pair;
//   3. exact typed query-window endpoints and chain lane;
//   4. exact physical interval AND exact source line span.
//
// Values, labels, confidence and Summary prose never participate.

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type crossTypeRankSeatReconSpec struct {
	absorberType    string
	absorberTypes   []string
	absorbedType    string
	absorberSource  string
	absorberSources []string
	absorbedSource  string
	// stateScalarMatch is reserved for the StateChurn projection, which has
	// no typed interval endpoints. It requires exact line-envelope and exact
	// same-source state-scalar equality instead of guessing an interval.
	stateScalarMatch   bool
	familyMatchAllowed bool
	// anchorAbsorb (RSPA §29.61.10 / matrix §6.3 锚定分解吸收, 2026-07-14):
	// upgrades this causal pair from exact interval-twin matching to the
	// anchored-decomposition arm — a re-anchored window seat whose ◇
	// remainder is ≈0 (µs tolerance) is FULLY the chain seat's account and
	// folds into it losslessly even though the interval sets are no longer
	// byte-equal; a remainder >tol is the COMPLEMENT account and must never
	// be absorbed by the chain seat. Only the causal_* pairs carry it.
	anchorAbsorb bool
	// sameAccountScalarMatch (CHAINGUARD-1 件4, probes F3/F4, §29.204.1
	// 处置: 成员行×churn 家族行同线程同 dominant_state 同值即同账, 2026-07-22):
	// relaxes the exact line-envelope dimension for the two state_churn pairs
	// ONLY. The churn projection is BY CONSTRUCTION the fragmentation view of
	// the same window off-CPU ledger the member seat published, but its
	// summary line span differs from the member's, so the exact-line arm
	// failed and the same physical account seated TWICE (F3: [E7] runnable
	// member ➊ 36.000ms beside [E9] fragmented churn ➋ 36.000ms — a TOP-board
	// double count; F4: the ◇ remainder twins double-published the same 余段).
	// The relaxed arm still requires every other precise dimension: numeric
	// TID, exact typed window, same fold lane, EXACT dominant-state equality
	// and the µs-exact state-scalar identity (同值即同账 — the adjudicated
	// criterion). Non-churn pairs keep the exact-line arm byte-identically.
	sameAccountScalarMatch bool
}

// Closed adjudication set: adding a pair changes rank semantics and requires
// a production witness plus an explicit ruling. The explicit order makes
// matching deterministic even when several source pairs share one type.
var crossTypeRankSeatReconPairs = map[string]crossTypeRankSeatReconSpec{
	"io_burst_episode": {
		absorberTypes:   []string{"d_state_or_io_wait", "io_wait"},
		absorbedType:    "io_burst_episode",
		absorberSources: []string{"window_stats", "window_stats.io_wait_top"},
		absorbedSource:  "window_stats.io_burst_episodes",
	},
	"scheduler_latency": {
		// RunnableTop is the formal scheduler-state lane. The same exact
		// wakeup-to-switch interval is also projected by
		// scheduler_latency_stats; that diagnostic projection must not mint a
		// second seat or resurrect the raw wait after the formal lane has been
		// gated to a smaller priority-inversion impact.
		absorberTypes:      []string{"runnable_wait", "priority_inversion_runnable_wait"},
		absorbedType:       "scheduler_latency",
		absorberSource:     "window_stats",
		absorbedSource:     "scheduler_latency_stats",
		familyMatchAllowed: true,
	},
	"causal_scheduler_latency": {
		// XLANE-1 件2 (§29.104.1/§29.104.2 witness E11↔E15, 2026-07-15): the
		// CHAIN-LANE runnable seats absorb an exact interval-twin
		// scheduler_latency satellite too — the runnable2 witness form (the
		// satellite's three member segments 6.997/8.226/8.248 逐值同 the chain
		// seat's segments) is the same physical account wearing the diagnostic
		// type word. Every precise dimension of the exact arm stays: window /
		// numeric TID / line span / interval-set equality (区间集全等臂照旧精
		// 确); the lane dimension is bridged ONLY for a 件1
		// represented-demoted candidate (see crossTypeRankSeatExactMatch).
		// Absorption is the strongest proof layer (single seat + E# merge);
		// the 件1 ◇ demotion stays as the non-twin fallback.
		absorberTypes:      []string{"runnable_wait", "priority_inversion_candidate"},
		absorbedType:       "scheduler_latency",
		absorberSources:    []string{"wakeup_chain.causal_impacts", "wakeup_chain.aggregated_impacts"},
		absorbedSource:     "scheduler_latency_stats",
		familyMatchAllowed: true,
	},
	"fragmented_runnable_wait": {
		absorberTypes:          []string{"runnable_wait", "priority_inversion_runnable_wait"},
		absorbedType:           "fragmented_runnable_wait",
		absorberSource:         "window_stats",
		absorbedSource:         "window_stats.state_churn",
		stateScalarMatch:       true,
		familyMatchAllowed:     true,
		sameAccountScalarMatch: true,
	},
	"fragmented_d_state_or_io_wait": {
		absorberTypes:          []string{"io_wait", "d_state_or_io_wait"},
		absorbedType:           "fragmented_d_state_or_io_wait",
		absorberSources:        []string{"window_stats.io_wait_top", "window_stats"},
		absorbedSource:         "window_stats.state_churn",
		stateScalarMatch:       true,
		familyMatchAllowed:     true,
		sameAccountScalarMatch: true,
	},
	"causal_runnable_window": {
		absorberType:       "runnable_wait",
		absorbedType:       "runnable_wait",
		absorberSources:    []string{"wakeup_chain.causal_impacts", "wakeup_chain.aggregated_impacts"},
		absorbedSource:     "window_stats",
		familyMatchAllowed: true,
		anchorAbsorb:       true,
	},
	"causal_inversion_window": {
		absorberType:       "priority_inversion_candidate",
		absorberTypes:      []string{"priority_inversion_candidate"},
		absorbedType:       "priority_inversion_runnable_wait",
		absorberSources:    []string{"wakeup_chain.causal_impacts", "wakeup_chain.aggregated_impacts"},
		absorbedSource:     "window_stats",
		familyMatchAllowed: true,
	},
	"causal_inversion_plain_window": {
		absorberType:       "priority_inversion_candidate",
		absorbedType:       "runnable_wait",
		absorberSources:    []string{"wakeup_chain.causal_impacts", "wakeup_chain.aggregated_impacts"},
		absorbedSource:     "window_stats",
		familyMatchAllowed: true,
		// RSPA: a chain thread whose chain-lane seat is the inversion row is
		// still the anchored owner of a fully-anchored window runnable twin.
		anchorAbsorb: true,
	},
	"causal_io_window": {
		absorberType:       "io_wait",
		absorbedType:       "io_wait",
		absorberSources:    []string{"wakeup_chain.causal_impacts", "wakeup_chain.aggregated_impacts"},
		absorbedSource:     "window_stats.io_wait_top",
		familyMatchAllowed: true,
		anchorAbsorb:       true,
	},
	"causal_dio_window": {
		absorberTypes:      []string{"d_state_or_io_wait", "io_wait"},
		absorbedType:       "d_state_or_io_wait",
		absorberSources:    []string{"wakeup_chain.causal_impacts", "wakeup_chain.aggregated_impacts"},
		absorbedSource:     "window_stats",
		familyMatchAllowed: true,
		anchorAbsorb:       true,
	},
}

var crossTypeRankSeatReconOrder = []string{
	"io_burst_episode",
	"scheduler_latency",
	"causal_scheduler_latency",
	"fragmented_runnable_wait",
	"fragmented_d_state_or_io_wait",
	"causal_runnable_window",
	"causal_inversion_window",
	"causal_inversion_plain_window",
	"causal_io_window",
	"causal_dio_window",
}

// reconcileExactCrossTypeRankSeats reclaims duplicate rank/capacity seats
// while retaining every absorbed observation on the dedicated lossless
// carrier. Reset-first recomputation makes the pass idempotent.
func reconcileExactCrossTypeRankSeats(rank *RootCauseRankResult) {
	if rank == nil {
		return
	}
	pool := make([]RootCauseRankItem, 0, len(rank.Items)+len(rank.AbsorbedItems))
	pool = append(pool, rank.Items...)
	pool = append(pool, rank.AbsorbedItems...)
	rank.AbsorbedItems = nil
	if len(pool) < 2 {
		rank.Items = pool
		return
	}

	// Clear B4-owned markers only. G1 may already own RankFamilyKey on an
	// io_latency family; AbsorbedRankRows is the precise ownership signal that
	// prevents this pass from erasing another reconciliation lane.
	for i := range pool {
		if pool[i].AbsorbedRankRows > 0 {
			pool[i].RankFamilyKey = ""
		}
		pool[i].AbsorbedRankRows = 0
		if pool[i].AbsorbedByRankFamily {
			pool[i].AbsorbedByRankFamily = false
			pool[i].AbsorbedIntoFamily = ""
			if pool[i].Tier == RootCauseTierAbsorbed {
				pool[i].Tier = ""
			}
			pool[i].Rank = 0
			pool[i].BackgroundRank = 0
		}
	}

	absorbed := make(map[int]int) // absorbed index -> unique absorber index
	for i := range pool {
		matched := -1
		for _, specKey := range crossTypeRankSeatReconOrder {
			spec := crossTypeRankSeatReconPairs[specKey]
			if strings.TrimSpace(pool[i].Type) != spec.absorbedType {
				continue
			}
			for j := range pool {
				if i == j || !crossTypeRankSeatExactMatch(pool[j], pool[i], spec, rank.Window) {
					continue
				}
				if matched >= 0 {
					// More than one possible owner is not a precise identity. Keep the
					// original publication rather than electing arbitrarily.
					matched = -2
					break
				}
				matched = j
			}
			if matched == -2 {
				break
			}
		}
		if matched >= 0 {
			absorbed[i] = matched
		}
	}
	// RSPA-MP1 (§15.2/§15.4 batch D, 2026-07-26): the multi-partition
	// periodic fail-close deliberately resurrects ONE chain seat instead of
	// copying its aggregate timer discount onto several window partitions.
	// At that point the raw partition rows and the resurrected seat are two
	// accounts of the SAME census wall clock. Leaving both on the active
	// board lets a raw, undiscounted partition outrank the periodic owner.
	//
	// Reconcile the group only on a closed typed proof:
	//   * exactly one resurrected periodic D/IO owner for the pid;
	//   * at least two raw window D/IO partitions (the one-seat path has its
	//     own direct credential transfer);
	//   * every raw row carries its ledger-anchor stamp and is fully anchored;
	//   * the raw Σ equals the owner's typed census Σ at the RSPA µs ruler.
	//
	// Any missing row proof, prior competing owner, aggregate ambiguity or Σ
	// mismatch leaves the former dual publication untouched. Successful rows
	// use the existing B4 lossless AbsorbedItems carrier, so projection and
	// evidence retain each partition while the discounted periodic row is
	// the sole rank-value owner.
	reconcileRSPAMultiPartitionPeriodicAccounts(pool, absorbed)
	// Resolve transitive ownership (for example scheduler_latency ->
	// window_stats runnable -> causal runnable) onto the terminal active row.
	// Cycles are impossible in the directed adjudication table; defend anyway.
	for candidate, owner := range absorbed {
		seen := map[int]bool{candidate: true}
		for {
			if seen[owner] {
				delete(absorbed, candidate)
				break
			}
			seen[owner] = true
			next, ok := absorbed[owner]
			if !ok {
				absorbed[candidate] = owner
				break
			}
			owner = next
		}
	}
	if len(absorbed) == 0 {
		rank.Items = pool
		return
	}

	active := make([]RootCauseRankItem, 0, len(pool)-len(absorbed))
	lossless := make([]RootCauseRankItem, 0, len(absorbed))
	keys := make(map[int]string)
	counts := make(map[int]int)
	// Preserve the original pool order. Iterating the absorbed map directly
	// made the lossless observation sequence process-random once more than one
	// adjudicated pair was present, leaking nondeterminism into JSON/evidence
	// order even though active rank seats were stable.
	for absorbedIndex := range pool {
		absorberIndex, ok := absorbed[absorbedIndex]
		if !ok {
			continue
		}
		key := keys[absorberIndex]
		if key == "" {
			key = crossTypeRankSeatReconKey(pool[absorberIndex], pool[absorbedIndex], rank.Window)
			keys[absorberIndex] = key
		}
		item := pool[absorbedIndex]
		item.Rank = 0
		item.BackgroundRank = 0
		item.Tier = RootCauseTierAbsorbed
		item.AbsorbedByRankFamily = true
		item.AbsorbedIntoFamily = key
		lossless = append(lossless, item)
		counts[absorberIndex]++
	}
	for i := range pool {
		if _, drop := absorbed[i]; drop {
			continue
		}
		item := pool[i]
		if count := counts[i]; count > 0 {
			item.RankFamilyKey = keys[i]
			item.AbsorbedRankRows = count
		}
		active = append(active, item)
	}
	rank.Items = active
	rank.AbsorbedItems = lossless
}

func reconcileRSPAMultiPartitionPeriodicAccounts(pool []RootCauseRankItem, absorbed map[int]int) {
	ownersByPID := make(map[int][]int)
	rawByPID := make(map[int][]int)
	rawProofComplete := make(map[int]bool)
	for i := range pool {
		item := pool[i]
		pid := item.Thread.PID
		if pid <= 0 {
			continue
		}
		typ := strings.TrimSpace(item.Type)
		source := strings.TrimSpace(item.Source)
		if item.PeriodicSource && item.PeriodicTimerWait &&
			item.ChainAnchorOwnershipDivergent && item.ChainAnchorCensusMs > 0 &&
			(typ == "d_state_or_io_wait" || typ == "io_wait") &&
			strings.HasPrefix(source, "wakeup_chain.") {
			ownersByPID[pid] = append(ownersByPID[pid], i)
			continue
		}
		if (typ != "d_state_or_io_wait" && typ != "io_wait") ||
			(source != "window_stats" && source != "window_stats.io_wait_top") {
			continue
		}
		rawByPID[pid] = append(rawByPID[pid], i)
		if _, seen := rawProofComplete[pid]; !seen {
			rawProofComplete[pid] = true
		}
		scalar := item.DStateMs + item.IOWaitMs
		anchored := item.ledgerAnchoredDMs + item.ledgerAnchoredIOMs
		if item.PeriodicSource || item.ChainAnchorRemainderSeat ||
			!item.ledgerAnchorStamped || scalar <= 0 ||
			!rspaWithinTol(scalar, anchored) {
			rawProofComplete[pid] = false
		}
	}
	for pid, owners := range ownersByPID {
		raw := rawByPID[pid]
		if len(owners) != 1 || len(raw) < 2 || !rawProofComplete[pid] {
			continue
		}
		owner := owners[0]
		if _, ownerAlreadyAbsorbed := absorbed[owner]; ownerAlreadyAbsorbed {
			continue
		}
		rawSum := 0.0
		precise := true
		for _, candidate := range raw {
			if _, alreadyAbsorbed := absorbed[candidate]; alreadyAbsorbed {
				precise = false
				break
			}
			rawSum += pool[candidate].DStateMs + pool[candidate].IOWaitMs
		}
		if !precise || !rspaWithinTol(rawSum, pool[owner].ChainAnchorCensusMs) {
			continue
		}
		for _, candidate := range raw {
			absorbed[candidate] = owner
		}
	}
}

func crossTypeRankSeatExactMatch(absorber, candidate RootCauseRankItem, spec crossTypeRankSeatReconSpec, queryWindow TimeWindow) bool {
	if !crossTypeRankSeatAbsorberTypeMatches(absorber.Type, spec) || candidate.Type != spec.absorbedType {
		return false
	}
	if !crossTypeRankSeatAbsorberSourceMatches(absorber.Source, spec) || candidate.Source != spec.absorbedSource {
		return false
	}
	// RSPA anchored-decomposition arm (§29.61.10 / §6.3): a re-anchored ◇
	// remainder candidate meets a chain-lane absorber on the VALUE-IDENTITY
	// criterion, not interval-twin equality — remainder ≈ 0 (µs tolerance)
	// proves the window seat's whole account is the chain seat's anchored
	// value; a positive remainder is the complement account and never folds
	// into the chain seat (the two rows together restore the full account).
	if candidate.ChainAnchorRemainderSeat && spec.anchorAbsorb {
		scalar, ok := crossTypeRankSeatStateScalar(candidate)
		if !ok || scalar > rspaAnchorIdentityTolMs {
			return false
		}
		if absorber.Thread.PID <= 0 || candidate.Thread.PID <= 0 || absorber.Thread.PID != candidate.Thread.PID {
			return false
		}
		absorberWindow, absorberWindowOK := crossTypeRankSeatWindow(absorber, queryWindow)
		candidateWindow, candidateWindowOK := crossTypeRankSeatWindow(candidate, queryWindow)
		return absorberWindowOK && candidateWindowOK && candidateWindow == absorberWindow
	}
	if candidate.ChainAnchorRemainderSeat && strings.HasPrefix(strings.TrimSpace(absorber.Source), "wakeup_chain.") {
		// A positive-remainder seat under any causal pair keeps its own ◇
		// seat (complement account) — never re-absorbed by exact matching.
		return false
	}
	// io_burst_episode families can mix D-state-derived episodes with
	// independently sourced block/storage episodes. Without a typed subtype
	// roster, a multi-member family is not proof that every member belongs to
	// the formal D-state account, even when its hull happens to match.
	if !spec.familyMatchAllowed && (absorber.MemberCount > 1 || candidate.MemberCount > 1) {
		return false
	}
	// Numeric TID only. Comm-only equality is deliberately insufficient.
	if absorber.Thread.PID <= 0 || candidate.Thread.PID <= 0 || absorber.Thread.PID != candidate.Thread.PID {
		return false
	}
	// A trace clock may legitimately start at t=0. End>start is the typed
	// presence/validity signal; zero/zero remains the absent representation.
	absorberWindow, absorberWindowOK := crossTypeRankSeatWindow(absorber, queryWindow)
	candidateWindow, candidateWindowOK := crossTypeRankSeatWindow(candidate, queryWindow)
	if !absorberWindowOK || !candidateWindowOK || candidateWindow != absorberWindow {
		return false
	}
	if rootCauseFamilyFoldLaneKey(absorber) != rootCauseFamilyFoldLaneKey(candidate) {
		// XLANE-1 件1↔件2 bridge (§29.104.2, 2026-07-15): a satellite whose
		// lane moved ONLY because the chain seat already represents its
		// anchored share (typed ChainAnchorRepresentedByChainSeat — the 件1
		// demotion is the sole minter) compares on its pre-demotion chain lane
		// against wakeup_chain absorbers; every other lane mismatch still
		// fails open (the two-row honest publication).
		if !(candidate.ChainAnchorRepresentedByChainSeat &&
			strings.HasPrefix(strings.TrimSpace(absorber.Source), "wakeup_chain.")) {
			return false
		}
	}
	linesExact := absorber.LineStart > 0 && absorber.LineEnd >= absorber.LineStart &&
		candidate.LineStart == absorber.LineStart && candidate.LineEnd == absorber.LineEnd
	if !linesExact {
		// CHAINGUARD-1 件4 (probes F3/F4 同账双席, §29.204.1 处置, 2026-07-22):
		// the state_churn pairs may pass on the SAME-ACCOUNT identity instead
		// of the exact line envelope — the churn projection summarizes the
		// same (thread, window, state) off-CPU ledger under its own line span,
		// so the exact-line arm structurally never matched and the one
		// physical account seated twice on the board (F3) and twice on the ◇
		// remainder stanza (F4). The relaxed arm demands BOTH line envelopes
		// present plus EXACT dominant-state equality on top of every
		// remaining precise dimension (TID / typed window / fold lane / the
		// µs-exact state-scalar identity below — 同线程同 dominant_state
		// 同值即同账). Every non-churn pair keeps the exact-line arm
		// byte-identically.
		if !spec.sameAccountScalarMatch {
			return false
		}
		if absorber.LineStart <= 0 || absorber.LineEnd < absorber.LineStart ||
			candidate.LineStart <= 0 || candidate.LineEnd < candidate.LineStart {
			return false
		}
		if strings.TrimSpace(absorber.DominantState) == "" ||
			strings.TrimSpace(absorber.DominantState) != strings.TrimSpace(candidate.DominantState) {
			return false
		}
	}
	if spec.stateScalarMatch {
		for _, item := range []RootCauseRankItem{absorber, candidate} {
			if item.MemberCount > 1 && item.MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint && len(item.familyMemberIntervals) == 0 {
				// A MAX lower bound is not a complete occurrence set and cannot
				// prove that a StateChurn aggregate is merely a duplicate.
				return false
			}
		}
		absorberScalar, absorberOK := crossTypeRankSeatStateScalar(absorber)
		candidateScalar, candidateOK := crossTypeRankSeatStateScalar(candidate)
		if absorber.ChainAnchorRemainderSeat && candidate.ChainAnchorRemainderSeat {
			// RSPA: a re-anchored pair compares the PRE-SPLIT full accounts —
			// both sides were rewritten with the same per-pid anchored value,
			// so full-account equality (the pre-RSPA match condition) still
			// certifies one physical account even when both remainders are 0.
			return absorberOK && candidateOK && absorber.ChainAnchorFullMs > 0 &&
				absorber.ChainAnchorFullMs == candidate.ChainAnchorFullMs &&
				absorberScalar == candidateScalar
		}
		if absorber.ChainAnchorRemainderSeat != candidate.ChainAnchorRemainderSeat {
			// One migrated, one fail-open — different calibers now; honest
			// dual beats wrong absorption.
			return false
		}
		return absorberOK && candidateOK && absorberScalar > 0 && absorberScalar == candidateScalar
	}
	if !crossTypeRankSeatIntervalSetsEqual(absorber, candidate) {
		return false
	}
	return true
}

func crossTypeRankSeatWindow(item RootCauseRankItem, queryWindow TimeWindow) (TimeWindow, bool) {
	if item.StatsWindowEndTs > item.StatsWindowStartTs {
		return TimeWindow{StartTs: item.StatsWindowStartTs, EndTs: item.StatsWindowEndTs}, true
	}
	if strings.HasPrefix(item.Source, "wakeup_chain.") && queryWindow.EndTs > queryWindow.StartTs {
		// WINFLAG-1: the recon compares windows by STRUCT equality below —
		// window identity here is a pure endpoint-value identity, so the
		// result-side StartSet carrier is stripped (a chain-lane window that
		// merely rides the flagged query window must still equal its
		// stats-scalar twin reconstructed without the flag).
		return TimeWindow{StartTs: queryWindow.StartTs, EndTs: queryWindow.EndTs}, true
	}
	return TimeWindow{}, false
}

func crossTypeRankSeatAbsorberTypeMatches(typ string, spec crossTypeRankSeatReconSpec) bool {
	if typ == spec.absorberType && spec.absorberType != "" {
		return true
	}
	for _, allowed := range spec.absorberTypes {
		if typ == allowed {
			return true
		}
	}
	return false
}

func crossTypeRankSeatAbsorberSourceMatches(source string, spec crossTypeRankSeatReconSpec) bool {
	if source == spec.absorberSource && spec.absorberSource != "" {
		return true
	}
	for _, allowed := range spec.absorberSources {
		if source == allowed {
			return true
		}
	}
	return false
}

func crossTypeRankSeatStateScalar(item RootCauseRankItem) (float64, bool) {
	switch strings.TrimSpace(item.Type) {
	case "runnable_wait", "priority_inversion_runnable_wait", "fragmented_runnable_wait":
		return item.RunnableMs, true
	case "io_wait", "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		return item.DStateMs + item.IOWaitMs, true
	default:
		return 0, false
	}
}

func crossTypeRankSeatIntervalSetsEqual(a, b RootCauseRankItem) bool {
	aIntervals := crossTypeRankSeatNormalizedIntervals(a)
	bIntervals := crossTypeRankSeatNormalizedIntervals(b)
	if len(aIntervals) == 0 || len(aIntervals) != len(bIntervals) {
		return false
	}
	for i := range aIntervals {
		if aIntervals[i].start != bIntervals[i].start || aIntervals[i].end != bIntervals[i].end {
			return false
		}
	}
	return true
}

func crossTypeRankSeatNormalizedIntervals(item RootCauseRankItem) []foldInterval {
	intervals := append([]foldInterval(nil), item.familyMemberIntervals...)
	if len(intervals) == 0 && item.MemberCount > 1 {
		// A merged family's hull is not its physical member set. Missing the
		// internal interval inventory therefore fails closed instead of letting
		// two different gap patterns with the same hull reconcile.
		return nil
	}
	if len(intervals) == 0 && item.EndTs > item.StartTs {
		physicalMs := rootCauseCumulativeImpactMs(item)
		if stateMs, ok := crossTypeRankSeatStateScalar(item); ok && stateMs > 0 {
			physicalMs = stateMs
		}
		if item.ChainAnchorRemainderSeat && item.ChainAnchorFullMs > 0 {
			// RSPA: the row's typed interval still spans the PRE-SPLIT
			// physical segment; the published scalar is the remainder. The
			// continuity proof must therefore read the full account.
			physicalMs = item.ChainAnchorFullMs
		}
		lengthMs := (item.EndTs - item.StartTs) * 1000
		tolerance := math.Max(0.000001, math.Abs(physicalMs)*0.000001)
		if physicalMs <= 0 || math.Abs(lengthMs-physicalMs) > tolerance {
			// A ThreadDuration bucket may aggregate disjoint segments while only
			// exposing their hull. Equality proves the singleton is continuous;
			// otherwise the missing internal inventory must fail open.
			return nil
		}
		intervals = append(intervals, foldInterval{start: item.StartTs, end: item.EndTs})
	}
	valid := intervals[:0]
	for _, interval := range intervals {
		if interval.end > interval.start {
			valid = append(valid, interval)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	sort.Slice(valid, func(i, j int) bool {
		if valid[i].start != valid[j].start {
			return valid[i].start < valid[j].start
		}
		return valid[i].end < valid[j].end
	})
	merged := make([]foldInterval, 0, len(valid))
	current := valid[0]
	for _, interval := range valid[1:] {
		if interval.start <= current.end {
			if interval.end > current.end {
				current.end = interval.end
			}
			continue
		}
		merged = append(merged, current)
		current = interval
	}
	return append(merged, current)
}

func crossTypeRankSeatReconKey(absorber, absorbed RootCauseRankItem, queryWindow TimeWindow) string {
	window, _ := crossTypeRankSeatWindow(absorber, queryWindow)
	return fmt.Sprintf("rank_pair:%s>%s|pid:%d|%s|window:%.6f..%.6f|interval:%.6f..%.6f|lines:%d-%d",
		absorber.Type, absorbed.Type, absorber.Thread.PID,
		rootCauseFamilyFoldLaneKey(absorber),
		window.StartTs, window.EndTs,
		absorber.StartTs, absorber.EndTs,
		absorber.LineStart, absorber.LineEnd)
}
