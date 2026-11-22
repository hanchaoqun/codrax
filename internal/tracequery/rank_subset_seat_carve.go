package tracequery

// rank_subset_seat_carve.go — HMC377 round 11 (batch-owner ruling R-377-11 ②,
// 2026-11-21): one carrier per instant of a thread's ready time.
//
// A cpu_affinity_or_cpuset seat publishes the thread's ready time under a CPU
// restriction: a typed subset of the thread's runnable segments
// (CPUConstraintSummary.restrictedRunnableIntervals, the same segments the
// runnable census is built from). The thread's other ready-time seats were
// minted from the whole census account: the chain object (the chain seat, the
// case-B clipped window seat, the thread's own unwalked seat, the residual
// lane seat) holds the instants inside the dependency windows, and the ◇
// remainder seat holds every other instant. Before this pass nothing linked
// the restriction seat to them, so the same instants were priced twice: on the
// review-10 real windows the R-377-4 ready remainder (the whole remaining
// ready account) stood beside the restriction seat on ◎, the tree and the
// native board (105 网易云 后台销毁 #4 nputmethod.anco-38135: 7.121 + 98.304 +
// 36.968 > raw ready 105.717).
//
// The pass partitions the restriction seat's instants by the typed holds the
// other seats recorded where they were minted (rspaHeld: role, instants):
//   - instants the chain object holds stay with it — one physical thread, one
//     object (R-377-4). The restriction seat hands them over by carved claims
//     (part anchored / unwalked) and publishes the rest;
//   - every ◇ remainder seat of the thread leaves the restriction seat's own
//     instants to it: the cause-specific seat keeps its instants, the
//     remainder carries the rest and says which seat counts the share (a
//     carved claim, part subset, resolved to that seat's [E#] on the face);
//   - a restriction seat whose instants all lie inside one chain-object row
//     rides as that row's lossless detail (the B4 absorbed carrier, the same
//     pairing as the R-377-4 inversion detail).
//
// A carved claim is resolved by the one resolver (resolveRankShareClaims)
// against the published board: a share whose holder is on the board is
// counted there, and a share whose holder the cap or the publication gate cut
// returns to the row that handed it over (rspaWriteCarvedShares) — every share
// keeps exactly one carrier. Typed instants and thread identity only; values
// never decide. A thread whose holds carry no instants (a seat whose
// inventory did not reproduce its account) is left untouched (fail open).

import (
	"fmt"
	"math"
	"strings"
)

const (
	// RootCauseShareClaimSubset — the share of a ◇ remainder's account that a
	// cause-specific subset seat of the same thread (the CPU-restriction
	// seat) publishes; the remainder does not count it.
	RootCauseShareClaimSubset RootCauseShareClaimPart = "subset"
	// RootCauseShareHolderSubsetSeat — a seat whose instants are a
	// cause-specific subset of the thread's ready time.
	RootCauseShareHolderSubsetSeat RootCauseShareHolderRole = "subset_seat"
)

// cpuConstraintSeatInstants returns a restriction seat's own instants when
// they reproduce its published account (µs tolerance per interval): its
// runnable account and its published impact are exactly those instants. A
// background-lane seat publishes a capped impact on a lane ◎ does not count
// (backgroundImpactMs), so it is never a carrier another seat could leave
// instants to.
func cpuConstraintSeatInstants(item RootCauseRankItem) ([]foldInterval, bool) {
	if strings.TrimSpace(item.Type) != "cpu_affinity_or_cpuset" || item.Thread.PID <= 0 || item.AbsorbedByRankFamily ||
		len(item.cpuConstraintRestrictedIntervals) == 0 || item.RunnableMs <= rspaAnchorIdentityTolMs ||
		strings.TrimSpace(item.ChainRelevance) == "background" {
		return nil, false
	}
	merged := rspaFoldCopy(item.cpuConstraintRestrictedIntervals)
	length, ok := foldIntervalsLengthMs(merged)
	tol := rspaAnchorIdentityTolMs * float64(1+len(merged))
	if !ok || math.Abs(length-item.RunnableMs) > tol || math.Abs(length-item.ImpactMs) > tol {
		return nil, false
	}
	return merged, true
}

// rspaCarvedHolderPhrases are the forms of the clause a restriction seat
// writes for the share one chain-object role holds: held, and its holder not
// on the published board (compact, the cap's form). HMC377 round-11 merge
// (R-377-11 ① × ②, R-377-8): the holder published only as context by the
// gate (context), the share then counted by another row of this thread as
// that holder's successor (successor), or passed back to this seat, which
// counts it with its own value (returned) — a holder the gate published as
// context is never called absent.
func rspaCarvedHolderPhrases(role RootCauseShareHolderRole, ms float64) rspaSpanForms {
	seat := "this thread's on-chain seat"
	switch role {
	case RootCauseShareHolderChainSeat:
		seat = "this thread's chain seat"
	case RootCauseShareHolderClippedSeat:
		seat = "this thread's clipped on-chain window seat"
	case RootCauseShareHolderUnwalkedSeat:
		seat = "this thread's own on-chain seat for the waits the chain did not walk"
	}
	return rspaSpanForms{
		held:      fmt.Sprintf("; %.3fms of it lies inside instants %s already publishes (one thread, one object: counted there, not here)", ms, seat),
		compact:   fmt.Sprintf("; %.3fms of it lies inside instants of %s, which is not on the published board (counted here)", ms, seat),
		context:   fmt.Sprintf("; %.3fms of it lies inside instants of %s, which %s (counted here)", ms, seat, rspaHolderContextOnly),
		successor: fmt.Sprintf("; %.3fms of it lies inside instants of %s, which %s; another row of this thread on the published board counts them (not here)", ms, seat, rspaHolderContextOnly),
		returned:  fmt.Sprintf("; %.3fms of it lies inside instants of %s, which %s, so this row counts them with its own value", ms, seat, rspaHolderContextOnly),
	}
}

// carveCPUConstraintReadyInstants is the one-carrier pass over a board's
// restriction seats (idempotent: a seat is decided once).
func carveCPUConstraintReadyInstants(items []RootCauseRankItem) {
	for xi := range items {
		x := &items[xi]
		if x.cpuConstraintCarveDecided {
			continue
		}
		restricted, ok := cpuConstraintSeatInstants(*x)
		if !ok {
			continue
		}
		type hold struct {
			idx     int
			role    RootCauseShareHolderRole
			windows []foldInterval
		}
		var onChain, remainders []hold
		windowless := false
		for j := range items {
			if j == xi || items[j].Thread.PID != x.Thread.PID || items[j].AbsorbedByRankFamily {
				continue
			}
			for _, h := range items[j].rspaHeld {
				if h.dio || h.semantic || h.ms <= rspaAnchorIdentityTolMs || h.role == RootCauseShareHolderSubsetSeat {
					continue
				}
				if len(h.windows) == 0 {
					windowless = true
					continue
				}
				if h.role == RootCauseShareHolderRemainderSeat {
					remainders = append(remainders, hold{j, h.role, h.windows})
				} else {
					onChain = append(onChain, hold{j, h.role, h.windows})
				}
			}
		}
		if windowless || (len(onChain) == 0 && len(remainders) == 0) {
			continue
		}
		x.cpuConstraintCarveDecided = true
		// The chain object's share of the restricted instants, per holder
		// role (one claim per role) and per holding row.
		var inside []foldInterval
		var roles []RootCauseShareHolderRole
		insideByRole := map[RootCauseShareHolderRole][]foldInterval{}
		holderRows := map[int]bool{}
		for _, h := range onChain {
			part := rspaFoldIntersect(restricted, rspaFoldCopy(h.windows))
			if rspaFoldLengthMs(part) <= rspaAnchorIdentityTolMs {
				continue
			}
			if _, seen := insideByRole[h.role]; !seen {
				roles = append(roles, h.role)
			}
			insideByRole[h.role] = rspaFoldUnion(insideByRole[h.role], part)
			inside = rspaFoldUnion(inside, part)
			holderRows[h.idx] = true
		}
		own := foldIntervalSubtract(restricted, inside)
		fullMs, insideMs, ownMs := rspaFoldLengthMs(restricted), rspaFoldLengthMs(inside), rspaFoldLengthMs(own)
		base := x.Summary
		if x.cpuConstraintBaseSummarySet {
			base = x.cpuConstraintBaseSummary
		}
		switch {
		case insideMs > rspaAnchorIdentityTolMs && ownMs <= rspaAnchorIdentityTolMs && len(holderRows) == 1:
			// Every restricted instant lies inside one chain-object row: the
			// restriction seat rides as that row's lossless detail.
			for j := range holderRows {
				key := fmt.Sprintf("cpu_constraint_account:pid=%d|lines=%d-%d|window=%.6f..%.6f",
					x.Thread.PID, x.LineStart, x.LineEnd, x.StatsWindowStartTs, x.StatsWindowEndTs)
				x.subsetDetailRepresentedBy = key
				items[j].subsetDetailRepresents = append(append([]string(nil), items[j].subsetDetailRepresents...), key)
			}
			x.ChainCredentialLaneDemoted = false
			x.Causality = "adjacent_to_wakeup_chain"
			x.ChainRelevance = "adjacent"
			rspaSetSentence(x, rspaPlainSentence(base+fmt.Sprintf("; all %.3fms of this ready time under the CPU restriction lies inside instants this thread's on-chain seat already publishes — one thread, one object: this row rides as that seat's lossless detail", fullMs)))
			continue
		case insideMs > rspaAnchorIdentityTolMs:
			// The restriction seat keeps the instants the chain object does
			// not hold; it publishes them as its own ◇ account.
			var s rspaSentence
			text := base + fmt.Sprintf("; this row counts the ready time under this CPU restriction that no on-chain seat of this thread publishes (%.3fms under the restriction in all)", fullMs)
			var anchoredMs, unwalkedMs float64
			for _, role := range roles {
				part := insideByRole[role]
				ms := rspaFoldLengthMs(part)
				claimPart := RootCauseShareClaimAnchored
				if role == RootCauseShareHolderUnwalkedSeat {
					claimPart = RootCauseShareClaimUnwalked
					unwalkedMs += ms
				} else {
					anchoredMs += ms
				}
				claim := rspaClaim(claimPart, role, x.Thread.PID, false, ms, part)
				claim.carved = true
				text += s.spanForms(rspaCarvedHolderPhrases(role, ms), claim)
			}
			// Case A' (the thread's chain seat publishes its own account,
			// which diverges from the anchored ledger): the remainder of the
			// thread carries the typed divergence verdict; this row speaks
			// the same double-account form for its share inside the walked
			// windows (never "held by the chain seat" at that value).
			if _, walked := insideByRole[RootCauseShareHolderChainSeat]; walked {
				for _, r := range remainders {
					rr := items[r.idx]
					if !rr.ChainAnchorOwnershipDivergent {
						continue
					}
					x.ChainAnchorOwnershipDivergent = true
					x.ChainAnchorChainLaneMs = rr.ChainAnchorChainLaneMs
					x.ChainAnchorCensusMs = rr.ChainAnchorCensusMs
					account := rspaClaim(RootCauseShareClaimChainAccount, RootCauseShareHolderChainSeat, x.Thread.PID, false, rr.ChainAnchorChainLaneMs, nil)
					accountHeld, accountCompact, accountContext := rspaChainAccountPhrases(rr.ChainAnchorChainLaneMs)
					text += "; anchored-ownership divergence: " + s.spanContext(accountHeld, accountCompact, accountContext, account) +
						fmt.Sprintf(" while the anchored ledger sum is %.3fms — two separate accounts, never additive with each other", rr.ChainAnchorCensusMs)
					break
				}
			}
			s.text = text
			rspaApplyCarvedShares(x, insideMs)
			x.ChainAnchorRemainderSeat = true
			x.ChainAnchoredMs = anchoredMs
			x.ChainAnchorFullMs = fullMs
			if unwalkedMs > rspaAnchorIdentityTolMs {
				x.ChainAnchorUnwalkedMs = unwalkedMs
				x.ChainAnchorOutsideMs = ownMs
			}
			x.ChainCredentialLaneDemoted = false
			x.Causality = "adjacent_to_wakeup_chain"
			x.ChainRelevance = "adjacent"
			rspaSetSentence(x, s)
		case x.cpuConstraintBaseSummarySet:
			// Instants inside the dependency windows whose holding seat is
			// not on this board stay with this row.
			rspaSetSentence(x, rspaPlainSentence(base+fmt.Sprintf("; %.3fms of this ready time under the CPU restriction lies inside this thread's dependency windows and no on-chain seat holding it is on this board, so this row counts all %.3fms on the adjacent lane", x.cpuConstraintChainWindowMs, fullMs)))
		}
		// The restriction seat holds its own instants: as the
		// cause-specific subset of the thread's ready time, and — where the
		// thread's ◇ remainder held them — as part of the thread's remainder.
		rspaHold(x, RootCauseShareHolderSubsetSeat, false, own, ownMs)
		var ownRemainder []foldInterval
		for _, r := range remainders {
			carve := rspaFoldIntersect(own, rspaFoldCopy(r.windows))
			carveMs := rspaFoldLengthMs(carve)
			if carveMs <= rspaAnchorIdentityTolMs {
				continue
			}
			ownRemainder = rspaFoldUnion(ownRemainder, carve)
			if emptied := rspaCarveRemainderShare(&items[r.idx], carve, carveMs); emptied {
				// The restriction seat now publishes every instant this
				// remainder held: the remainder rides as its lossless detail.
				key := fmt.Sprintf("cpu_constraint_remainder:pid=%d|lines=%d-%d|window=%.6f..%.6f",
					items[r.idx].Thread.PID, items[r.idx].LineStart, items[r.idx].LineEnd,
					items[r.idx].StatsWindowStartTs, items[r.idx].StatsWindowEndTs)
				items[r.idx].subsetDetailRepresentedBy = key
				x.subsetDetailRepresents = append(append([]string(nil), x.subsetDetailRepresents...), key)
			}
		}
		if len(ownRemainder) > 0 {
			rspaHold(x, RootCauseShareHolderRemainderSeat, false, ownRemainder, rspaFoldLengthMs(ownRemainder))
		}
	}
}

// rspaCarveRemainderShare leaves a restriction seat's instants (carve) to it:
// the remainder's ◇ hold loses them, its value is its account less the carved
// shares, and its sentence names where the share is counted. It reports
// whether the remainder holds no instant of its own any more.
func rspaCarveRemainderShare(r *RootCauseRankItem, carve []foldInterval, carveMs float64) bool {
	held := make([]rspaHeldShare, 0, len(r.rspaHeld))
	left := 0.0
	for _, h := range r.rspaHeld {
		if h.role == RootCauseShareHolderRemainderSeat && !h.dio && !h.semantic {
			h.windows = foldIntervalSubtract(rspaFoldCopy(h.windows), carve)
			h.ms = rspaFoldLengthMs(h.windows)
			left += h.ms
		}
		held = append(held, h)
	}
	r.rspaHeld = held
	claim := rspaClaim(RootCauseShareClaimSubset, RootCauseShareHolderSubsetSeat, r.Thread.PID, false, carveMs, carve)
	claim.carved = true
	var s rspaSentence
	s.text = s.spanContext(
		fmt.Sprintf("; %.3fms of this remainder lies under this thread's CPU affinity/cpuset restriction and is counted on that restriction's own row, not here", carveMs),
		fmt.Sprintf("; %.3fms of this remainder lies under this thread's CPU affinity/cpuset restriction (counted here: that restriction's row is not on the published board)", carveMs),
		fmt.Sprintf("; %.3fms of this remainder lies under this thread's CPU affinity/cpuset restriction (counted here: that restriction's row is published only as context)", carveMs),
		claim)
	rspaAppendSentence(r, s)
	rspaApplyCarvedShares(r, rspaCarvedClaimsMs(*r, false))
	return left <= rspaAnchorIdentityTolMs
}

// rspaCarvedClaimsMs sums a row's carved claims (resolvedOnly: those whose
// holder is on the published board).
func rspaCarvedClaimsMs(item RootCauseRankItem, resolvedOnly bool) float64 {
	total := 0.0
	for _, c := range item.ShareClaims {
		if c.carved && (!resolvedOnly || c.Resolved) {
			total += c.ShareMs
		}
	}
	return total
}

// rspaWriteCarvedShares publishes a row's account less the carved shares the
// published board counts elsewhere (written by the one resolver).
func rspaWriteCarvedShares(item *RootCauseRankItem) {
	if item.rspaCarvedAppliedMs <= 0 && rspaCarvedClaimsMs(*item, false) <= 0 {
		return
	}
	rspaApplyCarvedShares(item, rspaCarvedClaimsMs(*item, true))
}

// rspaApplyCarvedShares leaves exactly targetMs of carved shares out of the
// row's value channels: the difference to the share left out now returns to
// (or leaves) every channel that carries the row's account. Incremental, so a
// channel normalized after the carve (the effective channel) keeps its
// normalization.
func rspaApplyCarvedShares(item *RootCauseRankItem, targetMs float64) {
	delta := item.rspaCarvedAppliedMs - targetMs
	if delta == 0 {
		return
	}
	if item.rspaCarvedAppliedMs == 0 {
		item.rspaCarvedEffective = item.EffectiveImpactMs > 0
	}
	adjust := func(v *float64) { *v = rspaClampNonNegative(*v + delta) }
	adjust(&item.RunnableMs)
	adjust(&item.ImpactMs)
	adjust(&item.ProjectedImpactMs)
	adjust(&item.CumulativeImpactMs)
	if item.EffectiveImpactMs > 0 || item.rspaCarvedEffective {
		before := item.EffectiveImpactMs
		adjust(&item.EffectiveImpactMs)
		item.rspaCarvedEffective = true
		if before > 0 && item.Score > 0 {
			item.Score *= item.EffectiveImpactMs / before
		}
	}
	item.rspaCarvedAppliedMs = targetMs
}

// reconcileSubsetDetails joins a restriction seat whose instants all lie
// inside one chain-object row to that row, and a remainder whose every instant
// the restriction seat publishes to that seat, as lossless detail (the same
// closed typed proof as reconcileInversionAccountsRepresentedOnChain: the
// paired key minted by carveCPUConstraintReadyInstants, the same numeric TID,
// exactly one representing row in the pool).
func reconcileSubsetDetails(pool []RootCauseRankItem, absorbed map[int]int) {
	owners := map[string][]int{}
	for i := range pool {
		for _, key := range pool[i].subsetDetailRepresents {
			owners[key] = append(owners[key], i)
		}
	}
	if len(owners) == 0 {
		return
	}
	for i := range pool {
		key := pool[i].subsetDetailRepresentedBy
		if key == "" {
			continue
		}
		if _, already := absorbed[i]; already {
			continue
		}
		candidates := owners[key]
		if len(candidates) != 1 || candidates[0] == i ||
			pool[candidates[0]].Thread.PID <= 0 || pool[candidates[0]].Thread.PID != pool[i].Thread.PID {
			continue
		}
		absorbed[i] = candidates[0]
	}
}
