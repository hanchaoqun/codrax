package tracequery

// rank_state_edge_anchor.go — ONCHAIN-3c: the R3 host-edge credential arm's
// reach extends from semantic SPANS to the runnable / D-IO STATE seats of
// bare-census-edge hosts (mint audit docs/design/onchain_mint_audit_20260718.md
// 反向缺口5; evaluation docs/design/edge3be_eval_20260719.md §3c; R3 precedent
// §29.88.1/§29.88.2, ledger docs/design/real_trace_campaign_20260705.md).
//
// Shape: a NON-chain-member host thread (or, since STATERES-1, a chain-member
// host's RESIDUAL segments — see the ownership boundary below) holds a REAL
// typed wakeup edge toward the analysis target (the raw census pair
// host→target), yet only its deterministic semantic spans could ride the
// chain tier (R3 span basis). Its runnable / D-IO state seats had no door:
// RSPA re-anchoring only runs for pids with depth>0 chain anchor windows, and
// the generic enrich lane sees no node-window overlap.
//
// Pricing (HMC377 E2, owner ruling 2026-11-18 — supersedes the ONCHAIN-3c
// [window start, latest edge] reading of §29.139 and the §40.30 residual
// lane's same reading): the host's segments INSIDE a wait of the analysis
// target that the host's own recorded wakeup ended are direct supply-side
// causality (ready there = the waker was CPU-starved and delayed the wakeup
// the target was waiting for; D/IO there = the waker was blocked before
// delivering it). Segments before that wait began, between waits or after the
// wakeup are not a statement about the target: they go to the adjacent
// remainder, totals conserved. A wakeup that found the target not waiting
// prices nothing.
//
// Mechanism (existing templates only, zero new credential machinery):
//   - credential  = hostSemanticSpanEdgeAnchor (R3 function — typed census/
//     chain-edge inventories, degenerate-window fail-closed, target carve);
//     its windows are the host's dependency windows (target_wait_dependency.go);
//   - partition   = splitFoldIntervalsByDependencyWindows per TRUE segment
//     (the seat's validated close-site inventory: runnableIntervals /
//     dioSegmentIntervals* — never the StartTs..EndTs hull; HULL-CRED
//     discipline), inside share ⛓ / outside share ◇;
//   - remainder   = the RSPA bipartition trio + ChainAnchorRemainderSeat
//     clone (side-lane plumbing, twin-visibility verbatim anchors, fold
//     anchor-form keys — all existing consumers);
//   - disclosure  = one wait clause (stateEdgeWaitClause) with the typed
//     HostWakeupEdgeAnchorTs/Via pair = the wakeup that closed the latest
//     priced wait, via the host's own direct wakeup; span/state co-billing
//     of one host is disclosed by the AXIOM-V2 件2 cross-direction mutual
//     pointers (runnable=scheduling_supply, D-IO=io_dependency, semantic
//     spans=self_workload — different directions, same thread, both support
//     inventories present ⇒ the existing pass fires with zero new code).
//
// Ownership boundary (one lane, one vocabulary owner): RSPA owns chain
// members' state-credential vocabulary (§29.61.10 处置矩阵, RNB-1 §29.88
// R2/R4) — an RSPA ⛓ clipped seat or ◇ remainder is NEVER re-judged here.
// STATERES-1 (user ruling §40.30 V-STATE-1 plan A, 2026-09-02, "RSPA 优先、R3
// 补残"): a chain-member host's runnable / D-IO seat whose TYPED segment
// inventory lies outside EVERY RSPA chain window of that host (zero overlap,
// belt-checked against the census-time ledger stamp) takes the SAME
// credential, restricted to the host's DIRECT census edge toward the target
// (never its chain-hop edge — that is what the chain windows price). Its
// walked waits are inside its chain windows, so only the waits the expansion
// did not walk can price a residual segment. The
// priority_inversion_runnable_wait R4-mirror arm stays bare-host only
// (ruling scope = runnable / D-IO state segments; knowledge-pinned). The
// analysis target is excluded (self-causality, R8).
//
// Fail-closed forms (宁漏勿假指): no credential / degenerate window (R3
// gate), absent dependency carrier, no segment inside a wait the host ended,
// missing or Σ-broken segment inventory, MAX-fallback folds (producer
// disjointness lost), inversion-rewritten or gated-algebra seats (indivisible
// composite — the RSPA R4 arm's reasoning), periodic-discounted rows. Every
// refused row keeps its lane and bytes untouched.

import (
	"fmt"
	"sort"
	"strings"
)

// stateEdgeSegmentSplit is one inventory's exact partition: pre* = the pieces
// inside the host's dependency windows (the chain seat), post* = the pieces
// outside them (the adjacent remainder); preMs + postMs reproduces the
// inventory Σ exactly (同源二分). Built by splitFoldIntervalsByDependencyWindows.
type stateEdgeSegmentSplit struct {
	preMs, postMs      float64
	pre, post          []foldInterval
	preStart, preEnd   float64
	postStart, postEnd float64
}

// foldIntervalsLengthMs sums a segment inventory's clipped lengths; ok=false
// on any degenerate segment (fail-closed — a broken inventory proves nothing).
func foldIntervalsLengthMs(intervals []foldInterval) (float64, bool) {
	total := 0.0
	for _, iv := range intervals {
		if iv.end <= iv.start {
			return 0, false
		}
		total += (iv.end - iv.start) * 1000
	}
	return total, true
}

// stateEdgeAnchorFamilyWord — the board family word (shared with the RSPA
// sentence family so one thread's accounts speak one vocabulary).
func stateEdgeAnchorFamilyWord(typ string) string {
	if typ == "runnable_wait" {
		return "runnable (scheduling-pressure candidate)"
	}
	return "D/IO blocking"
}

// stateEdgeWaitClause — the shared description of WHICH waits priced a host
// share (HMC377 E2, owner ruling 2026-11-18): the analysed thread's waits that
// this thread's own recorded wakeup ended, how many there were, how long they
// lasted together and when the last of them was ended. One clause, every
// host-edge sentence (state seats, their remainders, spans, disclosures).
//
// EVOLUTION RECORD (HMC377 E2-R5): the counts were those of the waits holding
// a priced piece, while the words name every wait this thread's wakeup ended
// (android.bg-7812 on Douyin reboot5: "1 wait(s), 0.245ms" for 2 waits /
// 0.252ms in raw). The clause now counts every such wait, and the timestamp
// is named as the wakeup that ended the latest wait holding part of this
// account.
func stateEdgeWaitClause(split dependencyWindowSplit) string {
	return fmt.Sprintf("the analysed thread's wait(s) that this thread's own wakeup ended (%d wait(s), %.3fms of waiting; the latest one holding part of this account was ended at %.6f)",
		split.allWaitCount, split.allWaitMs, split.lastWakeupTs)
}

// stateEdgeAnchoredSummary — the ⛓ seat's engine-side account for an
// inventory partly inside the host's dependency windows. Its pointer to the
// ◇ clone is a typed claim (stateEdgeAnchoredSentence, HMC377 fix round 5)
// resolved against the published board.
func stateEdgeAnchoredSummary(thread ThreadRef, family string, inside, full, outside float64, split dependencyWindowSplit) string {
	return stateEdgeAnchoredSentence(thread, family, inside, full, outside, split, RootCauseShareClaim{}).text
}

func stateEdgeAnchoredSentence(thread ThreadRef, family string, inside, full, outside float64, split dependencyWindowSplit, twin RootCauseShareClaim) rspaSentence {
	var s rspaSentence
	twinPhrase := s.span(rspaSummaryRemainderTwinPublished, rspaSummaryRemainderTwinUnpublished, twin)
	s.text = fmt.Sprintf("%s %s %.3fms inside %s; full-window account %.3fms = this share + %.3fms outside those waits %s — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, inside, stateEdgeWaitClause(split), full, outside, twinPhrase)
	return s
}

// stateEdgeFullyAnchoredSummary — the whole-account conversion form (every
// segment inside the host's dependency windows; no bipartition, no twin).
func stateEdgeFullyAnchoredSummary(thread ThreadRef, family string, full float64, split dependencyWindowSplit) string {
	return fmt.Sprintf("%s %s %.3fms lies entirely inside %s",
		threadLabel(thread), family, full, stateEdgeWaitClause(split))
}

// stateEdgeInversionDetailClause — HMC377 E2-R1 / R-377-4: the clause a seat
// priced from an indivisible inversion row's ready time carries. It names the
// overlap measurement with its own word (measured over the whole ready
// account, never split) and where that measurement rides.
func stateEdgeInversionDetailClause(overlap, account float64) string {
	return fmt.Sprintf("; the same-CPU lower-priority running overlap %.3fms was measured over the whole %.3fms ready-time account and is never split — that whole-account row rides as this seat's lossless detail",
		overlap, account)
}

// stateEdgePlainRunnableCopy returns a ready-time (runnable_wait) copy of an
// inversion-retyped window row: same thread, inventory and evidence, with
// every inversion-only claim cleared (the copy publishes ready time only).
func stateEdgePlainRunnableCopy(item RootCauseRankItem) RootCauseRankItem {
	plain := cloneRootCauseMeasurementSources(item)
	plain.Type = "runnable_wait"
	plain.PriorityOverlap = nil
	plain.GatedRunnableMs, plain.GatedRunningDeficitMs = 0, 0
	plain.PriorityRelationCaliber = ""
	plain.PriorityRelationProvenLowerMs = 0
	plain.PriorityRelationUnknownOrNonLowerMs = 0
	plain.PriorityRelationArtifactSources = nil
	plain.inversionAccountRepresentedBy = ""
	plain.inversionAccountRepresents = ""
	plain.subsetDetailRepresentedBy, plain.subsetDetailRepresents = "", nil
	rspaClearClaims(&plain)
	return plain
}

// stateEdgeRemainderSummary — the ◇ clone's account: the host's time outside
// every wait of the analysed thread that its wakeup ended. Its ownership
// claim names the ⛓ edge seat (stateEdgeRemainderSentence, a typed claim
// resolved against the published board).
func stateEdgeRemainderSummary(thread ThreadRef, family string, outside, full, inside float64, split dependencyWindowSplit) string {
	return stateEdgeRemainderSentence(thread, family, outside, full, inside, split, RootCauseShareClaim{}).text
}

func stateEdgeRemainderSentence(thread ThreadRef, family string, outside, full, inside float64, split dependencyWindowSplit, owner RootCauseShareClaim) rspaSentence {
	// HMC377 fix round 5: the holder is the thread's own edge seat (the
	// residual-lane role) — the sentence names that role, never "the chain
	// seat" (a walked chain-lane seat holds none of these instants).
	var s rspaSentence
	ownerPhrase := s.span(rspaSummaryUnwalkedOwnSeat, rspaSummaryUnwalkedOwnSeatUnpublished, owner)
	s.text = fmt.Sprintf("%s %s remainder %.3fms outside %s — the analysed thread was not waiting for this thread there (no chain credential for these segments); full-window account %.3fms = %.3fms inside those waits %s + this remainder — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, outside, stateEdgeWaitClause(split), full, inside, ownerPhrase)
	return s
}

// stateEdgeClaims are the two claims of one edge bisection: the ◇ clone's
// share inside the waits is held by the ⛓ edge seat, and the ⛓ seat's rest
// by the ◇ clone (HMC377 fix round 5).
func stateEdgeClaims(pid int, dio bool, preMs float64, pre []foldInterval, postMs float64, post []foldInterval) (owner, twin RootCauseShareClaim) {
	return rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderResidualSeat, pid, dio, preMs, pre),
		rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, pid, dio, postMs, post)
}

// bareCensusEdgeHostRunnableMintSet (ONCHAIN-3c mint-domain widening — 帽基
// 当全量 fifth instance) returns the pids of bare-census-edge hosts whose
// runnable census carries a POSITIVE share inside the host's dependency
// windows (HMC377 E2): exactly the rows the state-seat credential arm will
// examine, so the widening never strands a background row it will not
// examine (admission ≠ conversion: an examined row may still be refused and
// hold its home lane — the indivisible inversion form, minted then
// inversion-recast then R4-mirror refused). Gates (all typed): the
// ordered-stream premise (a regressed trace cannot prove the partition Σ —
// same premise as the RSPA decision gate 4), the R3 credential function
// verbatim (single edge-judgment authority, 禁自造第二套边判定 — covers the
// degenerate-window fail-close and the target carve), host ∉ chain thread
// set (RSPA vocabulary ownership), and Σ over the pid's census segment
// inventories inside the dependency windows > tolerance.
func bareCensusEdgeHostRunnableMintSet(chain ChainResult, chainThreads map[int]bool,
	census map[string]ThreadDuration, producerDisjoint bool) map[int]bool {
	if !producerDisjoint || len(census) == 0 {
		return nil
	}
	if len(chain.WakeupEdgeCensus) == 0 {
		return nil
	}
	var out map[int]bool
	seen := map[int]bool{}
	for _, pair := range chain.WakeupEdgeCensus {
		host := pair.Waker
		if pair.Count <= 0 || host.PID <= 0 || seen[host.PID] {
			continue
		}
		seen[host.PID] = true
		if threadInSet(chainThreads, host) || sameThreadRef(host, chain.Target) {
			continue
		}
		anchor, ok := hostSemanticSpanEdgeAnchor(&chain, host)
		if !ok {
			continue
		}
		if len(anchor.windows) == 0 {
			continue // every wakeup found the target not waiting: nothing to price
		}
		preMs := 0.0
		for _, td := range census {
			if td.Thread.PID != host.PID {
				continue
			}
			preMs += splitFoldIntervalsByDependencyWindows(td.runnableIntervals, anchor.windows).preMs
		}
		if preMs <= rspaAnchorIdentityTolMs {
			continue
		}
		if out == nil {
			out = map[int]bool{}
		}
		out[host.PID] = true
	}
	return out
}

// stateEdgeCredentialLane is the typed admission verdict of the state-seat
// credential pass: refused, the ONCHAIN-3c bare-census-host lane (full
// credential: direct census edge or the host's own chain-hop edge), or the
// STATERES-1 chain-member residual lane (direct census edge only).
type stateEdgeCredentialLane int

const (
	stateEdgeLaneRefused stateEdgeCredentialLane = iota
	stateEdgeLaneBareCensusHost
	stateEdgeLaneChainMemberResidual
)

// bareCensusEdgeStateSeatEligible is the typed admission gate (see the file
// header's fail-closed roster). PRECISE signals only. chainWindows is the
// per-pid merged depth>0 chain-node window set (chainAnchorWindowsByPID —
// the exact RSPA anchor set).
func bareCensusEdgeStateSeatEligible(item *RootCauseRankItem, chainThreads map[int]bool, target ThreadRef, chainWindows map[int][]TimeWindow) stateEdgeCredentialLane {
	if item.Thread.PID <= 0 || sameThreadRef(item.Thread, target) || item.SubjectIsAnalysisTarget {
		return stateEdgeLaneRefused
	}
	if rootCauseItemIsOnChain(*item) {
		return stateEdgeLaneRefused // already on the chain tier — nothing to extend
	}
	// STATERES-1 复核收编 (2026-09-02): RSPA's ZERO-anchored whole-account
	// rewrite (case B, anchored==0 — "no anchored share exists — no seat holds
	// it") is not a partition: the ◇ row IS the complete account RSPA found
	// nothing to price (a D/IO seat whose StartTs..EndTs hull crossed a chain
	// window while every true segment lay outside it). That form is exactly
	// the residual population and passes to the residual lane; a genuine
	// partition (anchored share > 0, a ⛓ twin exists) is never re-judged.
	rspaZeroWholeRemainder := item.ChainAnchorRemainderSeat && item.HostWakeupEdgeAnchorTs == 0 &&
		item.ChainAnchoredMs <= rspaAnchorIdentityTolMs && item.ChainAnchorFullMs > 0 &&
		item.ChainAnchorUnwalkedMs <= 0 && item.rspaUnwalkedHeldMs <= 0 &&
		threadInSet(chainThreads, item.Thread)
	if strings.TrimSpace(item.OnChainBasis) != "" || item.AbsorbedByRankFamily ||
		item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
		item.inversionAccountRepresentedBy != "" ||
		item.GatedShareConstituentSeat || item.ChainIdentityInheritance ||
		((item.ChainAnchorRemainderSeat || item.ChainAnchorFullMs > 0) && !rspaZeroWholeRemainder) {
		return stateEdgeLaneRefused // already adjudicated / processed forms (RSPA ⛓ and partition ◇ included)
	}
	if item.PeriodicSource {
		return stateEdgeLaneRefused // cadence-discounted account — indivisible here
	}
	if !threadInSet(chainThreads, item.Thread) {
		return stateEdgeLaneBareCensusHost
	}
	if item.rspaUnwalkedShareOwned {
		// HMC377 fix round 4: the unwalked-share ledger gave this thread's
		// time inside the waits its own wakeup ended (outside the walked
		// chain windows — the only time the residual lane prices for a chain
		// member) to a seat: the thread's own unwalked seat or its case-B
		// clipped window seat. A row that kept those instants is never priced
		// a second time here.
		return stateEdgeLaneRefused
	}
	if chainMemberResidualStateSeatEligible(item, chainWindows[item.Thread.PID]) {
		return stateEdgeLaneChainMemberResidual
	}
	return stateEdgeLaneRefused // RSPA owns chain members' chain-window state vocabulary
}

// chainMemberResidualStateSeatEligible (STATERES-1, §40.30 V-STATE-1 plan A)
// admits a chain-member host's state seat to the residual lane on PRECISE
// typed signals only:
//   - the ruling's literal scope: runnable / D-IO state seats (the
//     priority_inversion_runnable_wait R4-mirror arm is NOT opened);
//   - a non-empty typed segment inventory (the TRUE close-site segments,
//     never the StartTs..EndTs hull — HULL-CRED discipline);
//   - ZERO overlap between that inventory and every RSPA chain window of the
//     host (vacuously true for an edge-only / transit member with no depth>0
//     window: RSPA cannot price such a member either);
//   - the census-time ledger stamp as a belt: the stamp must be present and
//     record zero anchored ms for the family — a missing stamp (legacy
//     fixtures, MAX-fallback folds) or any disagreement between the overlay
//     and the ledger fails closed (宁漏勿猜).
func chainMemberResidualStateSeatEligible(item *RootCauseRankItem, windows []TimeWindow) bool {
	var inventory []foldInterval
	var ledgerAnchoredMs float64
	switch strings.TrimSpace(item.Type) {
	case "runnable_wait":
		inventory = item.runnableIntervals
		ledgerAnchoredMs = item.ledgerAnchoredRunnableMs
	case "d_state_or_io_wait", "io_wait":
		inventory = item.dioSegmentIntervals
		ledgerAnchoredMs = item.ledgerAnchoredDMs + item.ledgerAnchoredIOMs
	default:
		return false
	}
	if len(inventory) == 0 || !item.ledgerAnchorStamped {
		return false
	}
	if ledgerAnchoredMs > rspaAnchorIdentityTolMs {
		return false // RSPA priced (part of) this family — RSPA-owned
	}
	if rspaIntervalsOverlapMs(windows, inventory) > rspaAnchorIdentityTolMs {
		return false // overlay disagrees with the ledger stamp — fail closed
	}
	return true
}

// anchorBareCensusEdgeStateSeats is the ONCHAIN-3c pass. Runs right after
// reanchorOnChainStateSeats in BOTH rank passes (build + scheduler enrich) —
// idempotent: converted rows carry the state basis token / the remainder
// marker and are never re-processed (the enrich pass mints no new formal
// window state seats, so the second call is structurally a no-op). Returns
// the item slice (bisections append the ◇ remainder twin).
func anchorBareCensusEdgeStateSeats(chain ChainResult, items []RootCauseRankItem) []RootCauseRankItem {
	if len(items) == 0 {
		return items
	}
	if len(chain.Nodes) == 0 && len(chain.Edges) == 0 && len(chain.CausalImpacts) == 0 {
		return items
	}
	chainThreads := wakeupChainThreadSet(chain)
	chainWindows := chainAnchorWindowsByPID(chain)
	cachedAnchor := func(cache map[int]*semanticHostEdgeAnchor, resolve func(*ChainResult, ThreadRef) (semanticHostEdgeAnchor, bool)) func(ThreadRef) (semanticHostEdgeAnchor, bool) {
		return func(host ThreadRef) (semanticHostEdgeAnchor, bool) {
			if cached, ok := cache[host.PID]; ok {
				if cached == nil {
					return semanticHostEdgeAnchor{}, false
				}
				return *cached, true
			}
			anchor, ok := resolve(&chain, host)
			if !ok {
				cache[host.PID] = nil
				return semanticHostEdgeAnchor{}, false
			}
			cache[host.PID] = &anchor
			return anchor, true
		}
	}
	// Bare-census hosts take the full credential (direct or chain-hop);
	// chain-member residual seats take the DIRECT census edge only.
	fullAnchorFor := cachedAnchor(map[int]*semanticHostEdgeAnchor{}, hostSemanticSpanEdgeAnchor)
	directAnchorFor := cachedAnchor(map[int]*semanticHostEdgeAnchor{}, hostDirectCensusEdgeAnchor)
	var appended []RootCauseRankItem
	for i := range items {
		item := &items[i]
		lane := bareCensusEdgeStateSeatEligible(item, chainThreads, chain.Target, chainWindows)
		if lane == stateEdgeLaneRefused {
			continue
		}
		anchorFor := fullAnchorFor
		var saved RootCauseRankItem
		restore := false
		if lane == stateEdgeLaneChainMemberResidual {
			anchorFor = directAnchorFor
			if item.ChainAnchorRemainderSeat {
				// RSPA's zero-anchored whole-account ◇ form: the remainder IS the
				// full account — start the residual partition from a clean seat,
				// and give the row back untouched when nothing converts (a
				// refused row keeps its lane and bytes; HMC377 E2 makes the
				// refusal the common outcome for residual seats).
				saved, restore = *item, true
				item.ChainAnchorRemainderSeat = false
				item.ChainAnchoredMs, item.ChainAnchorFullMs = 0, 0
			}
		}
		clones := anchorStateSeatInsideDependencyWindows(item, lane, anchorFor)
		if restore && strings.TrimSpace(item.OnChainBasis) == "" {
			*item = saved
		}
		appended = append(appended, clones...)
	}
	return append(items, appended...)
}

// anchorStateSeatInsideDependencyWindows converts one admitted state seat
// (HMC377 E2): its true segment inventory is partitioned against the host's
// dependency windows; the inside share takes the chain seat, the outside share
// the adjacent remainder clone it returns. A refused seat returns nil.
func anchorStateSeatInsideDependencyWindows(item *RootCauseRankItem, lane stateEdgeCredentialLane, anchorFor func(ThreadRef) (semanticHostEdgeAnchor, bool)) []RootCauseRankItem {
	var out []RootCauseRankItem
	switch strings.TrimSpace(item.Type) {
	case "priority_inversion_runnable_wait":
		if lane != stateEdgeLaneBareCensusHost {
			return nil // STATERES-1 scope: the R4-mirror arm stays bare-host only
		}
		// RSPA R4 mirror arm (§29.88 R4; §29.83 件③ narrowing): the
		// inversion-rewritten seat's gated eff is an INDIVISIBLE composite
		// that cannot be split along the credential boundary without
		// minting a value equal to neither the measurement nor any
		// partition term. A FULLY pre-edge account changes lane whole with
		// every published value untouched (the inversion claim is pre-edge
		// too — the R4 「fully-anchored keeps the chain lane」 form in the
		// promotion direction); any post-edge share leaves the whole seat
		// on its home lane untouched (宁漏勿假指).
		if item.Source != "window_stats" || item.DominantState != string(StateRunnable) {
			return nil
		}
		if len(item.runnableIntervals) == 0 || !item.memberSegmentsProducerDisjoint {
			return nil
		}
		lengthMs, ok := foldIntervalsLengthMs(item.runnableIntervals)
		if !ok || !rspaWithinTol(lengthMs, item.RunnableMs) {
			return nil
		}
		anchor, ok := anchorFor(item.Thread)
		if !ok {
			return nil
		}
		split := splitFoldIntervalsByDependencyWindows(item.runnableIntervals, anchor.windows)
		if split.preMs <= rspaAnchorIdentityTolMs {
			return nil // no share inside a wait the host ended: grants nothing
		}
		priced := anchor.pricedBy(split)
		if split.postMs > rspaAnchorIdentityTolMs {
			// HMC377 E2-R1 (owner ruling 2026-11-18 "a thread that only
			// delivered a wakeup is priced for its own time inside that wait";
			// R-377-4 "one physical thread, one object") — EVOLUTION RECORD:
			// PARTSPLIT-1 (§29.150④) refused the conversion whole and put the
			// inside share on a disclosure-only record whose window-relative
			// floor hid it (Yuanbao bindApplication: DefaultDispatch-10729
			// ready 7.220ms inside an 8.163ms wait it ended, on no face). The
			// inside share is a Σ of TRUE ready segments, not the gated
			// composite: it is priced as its own on-chain scheduling-delay
			// seat (the runnable_wait arm's ⛓ half, same partition). The
			// inversion row itself is still never split: every value stays,
			// and it rides as that seat's lossless detail (paired key →
			// reconcileInversionAccountsRepresentedOnChain), so the thread is
			// one object and its overlap measurement is never re-labelled.
			// The inside and outside shares are the runnable_wait arm's exact
			// partition (⛓ inside + ◇ remainder twin, Σ == the ready account),
			// minted from a plain ready-time copy of the row.
			plain := stateEdgePlainRunnableCopy(*item)
			clone := plain
			full := split.preMs + split.postMs
			family := stateEdgeAnchorFamilyWord("runnable_wait")
			seat := plain
			seat.Rank, seat.BackgroundRank, seat.Tier = 0, 0, ""
			ownerClaim, twinClaim := stateEdgeClaims(item.Thread.PID, false, split.preMs, split.pre, split.postMs, split.post)
			anchoredSentence := stateEdgeAnchoredSentence(item.Thread, family, split.preMs, full, split.postMs, split, twinClaim)
			anchoredSentence.text += stateEdgeInversionDetailClause(item.EffectiveImpactMs, item.RunnableMs)
			convertStateSeatToEdgeAnchored(&seat, priced, split.preMs, split.preMs, 0, 0,
				split.pre, nil, nil, split.stateEdgeSegmentSplit, anchoredSentence)
			seat.ChainAnchoredMs = split.preMs
			seat.ChainAnchorFullMs = full
			remainder := mintStateEdgeRemainderClone(clone, priced, split.preMs, full,
				split.postMs, 0, 0, split.post, nil, nil, split.stateEdgeSegmentSplit,
				stateEdgeRemainderSentence(item.Thread, family, split.postMs, full, split.preMs, split, ownerClaim))
			key := inversionAccountKey(*item)
			seat.inversionAccountRepresents = key
			item.inversionAccountRepresentedBy = key
			// HMC377 fix round 5: the detail sentence names both seats — one
			// typed claim each, resolved against the published board.
			var detail rspaSentence
			seatPhrase := detail.span("is priced as its own on-chain scheduling-delay seat",
				"is priced as its own on-chain scheduling-delay seat, which is not on the published board (see the compaction disclosure),", ownerClaim)
			restPhrase := detail.span("the rest as its adjacent remainder",
				"the rest as its adjacent remainder, which is not on the published board (see the compaction disclosure)", twinClaim)
			detail.text = fmt.Sprintf("its ready time inside %s (%.3fms of this %.3fms account) %s and %s; this whole-account row keeps every value (the overlap measurement is never split) and rides as that seat's lossless detail",
				stateEdgeWaitClause(split), split.preMs, item.RunnableMs, seatPhrase, restPhrase)
			item.Summary = strings.TrimSpace(item.Summary)
			if item.Summary != "" {
				detail.text = "; " + detail.text
			}
			rspaAppendSentence(item, detail)
			return append(out, seat, remainder)
		}
		item.ChainRelevance = "on_chain"
		item.Causality = "on_wakeup_chain"
		item.OnChainBasis = RootCauseOnChainBasisHostWakeupEdgeState
		item.HostWakeupEdgeAnchorTs = priced.boundaryTs
		item.HostWakeupEdgeAnchorVia = priced.via()
		rspaHold(item, RootCauseShareHolderResidualSeat, false, item.runnableIntervals, item.RunnableMs)
		item.Summary = appendRootCauseSummaryDetail(item.Summary,
			fmt.Sprintf("the whole runnable account lies inside %s — every published value unchanged, the inversion seat is never split",
				stateEdgeWaitClause(split)))
	case "runnable_wait":
		if item.Source != "window_stats" || item.DominantState != string(StateRunnable) {
			return nil
		}
		if item.GatedRunnableMs > 0 || item.GatedRunningDeficitMs > 0 {
			return nil // gated composite algebra — indivisible (RSPA R4 reasoning)
		}
		if len(item.runnableIntervals) == 0 || !item.memberSegmentsProducerDisjoint {
			return nil
		}
		lengthMs, ok := foldIntervalsLengthMs(item.runnableIntervals)
		if !ok || !rspaWithinTol(lengthMs, item.RunnableMs) {
			return nil // inventory does not reproduce the account — fail closed
		}
		anchor, ok := anchorFor(item.Thread)
		if !ok {
			return nil
		}
		split := splitFoldIntervalsByDependencyWindows(item.runnableIntervals, anchor.windows)
		if split.preMs <= rspaAnchorIdentityTolMs {
			return nil // no share inside a wait the host ended: grants nothing
		}
		priced := anchor.pricedBy(split)
		full := split.preMs + split.postMs
		family := stateEdgeAnchorFamilyWord(item.Type)
		if split.postMs <= rspaAnchorIdentityTolMs {
			// Whole-account conversion: every segment inside the waits the
			// host ended — the seat changes lane with its published account
			// intact (值通道重铸为全账,无二分 trio,无余席).
			convertStateSeatToEdgeAnchored(item, priced, item.RunnableMs, item.RunnableMs, 0, 0,
				item.runnableIntervals, nil, nil, split.stateEdgeSegmentSplit,
				rspaPlainSentence(stateEdgeFullyAnchoredSummary(item.Thread, family, item.RunnableMs, split)))
			return nil
		}
		clone := *item
		ownerClaim, twinClaim := stateEdgeClaims(item.Thread.PID, false, split.preMs, split.pre, split.postMs, split.post)
		convertStateSeatToEdgeAnchored(item, priced, split.preMs, split.preMs, 0, 0,
			split.pre, nil, nil, split.stateEdgeSegmentSplit,
			stateEdgeAnchoredSentence(item.Thread, family, split.preMs, full, split.postMs, split, twinClaim))
		item.ChainAnchoredMs = split.preMs
		item.ChainAnchorFullMs = full
		return append(out, mintStateEdgeRemainderClone(clone, priced, split.preMs, full,
			split.postMs, 0, 0, split.post, nil, nil, split.stateEdgeSegmentSplit,
			stateEdgeRemainderSentence(item.Thread, family, split.postMs, full, split.preMs, split, ownerClaim)))
	case "d_state_or_io_wait", "io_wait":
		if item.Source != "window_stats" && item.Source != "window_stats.io_wait_top" {
			return nil
		}
		switch item.DominantState {
		case string(StateDSleep), string(StateIOWait):
		default:
			return nil
		}
		if len(item.dioSegmentIntervals) == 0 {
			return nil // no true-segment carrier (缺证≠证无 — fail closed)
		}
		dLen, okD := foldIntervalsLengthMs(item.dioSegmentIntervalsD)
		ioLen, okIO := foldIntervalsLengthMs(item.dioSegmentIntervalsIO)
		if !okD || !okIO || !rspaWithinTol(dLen, item.DStateMs) || !rspaWithinTol(ioLen, item.IOWaitMs) {
			return nil // per-state inventories must reproduce both channels
		}
		anchor, ok := anchorFor(item.Thread)
		if !ok {
			return nil
		}
		splitD := splitFoldIntervalsByDependencyWindows(item.dioSegmentIntervalsD, anchor.windows)
		splitIO := splitFoldIntervalsByDependencyWindows(item.dioSegmentIntervalsIO, anchor.windows)
		splitAll := splitFoldIntervalsByDependencyWindows(item.dioSegmentIntervals, anchor.windows)
		preMs := splitD.preMs + splitIO.preMs
		postMs := splitD.postMs + splitIO.postMs
		if preMs <= rspaAnchorIdentityTolMs {
			return nil
		}
		priced := anchor.pricedBy(splitAll)
		full := preMs + postMs
		family := stateEdgeAnchorFamilyWord(item.Type)
		if postMs <= rspaAnchorIdentityTolMs {
			convertStateSeatToEdgeAnchored(item, priced, item.DStateMs+item.IOWaitMs, 0, item.DStateMs, item.IOWaitMs,
				nil, item.dioSegmentIntervalsD, item.dioSegmentIntervalsIO, splitAll.stateEdgeSegmentSplit,
				rspaPlainSentence(stateEdgeFullyAnchoredSummary(item.Thread, family, item.DStateMs+item.IOWaitMs, splitAll)))
			return nil
		}
		clone := *item
		ownerClaim, twinClaim := stateEdgeClaims(item.Thread.PID, true, preMs, rspaFoldUnion(splitD.pre, splitIO.pre),
			postMs, rspaFoldUnion(splitD.post, splitIO.post))
		convertStateSeatToEdgeAnchored(item, priced, preMs, 0, splitD.preMs, splitIO.preMs,
			nil, splitD.pre, splitIO.pre, splitAll.stateEdgeSegmentSplit,
			stateEdgeAnchoredSentence(item.Thread, family, preMs, full, postMs, splitAll, twinClaim))
		item.ChainAnchoredMs = preMs
		item.ChainAnchorFullMs = full
		return append(out, mintStateEdgeRemainderClone(clone, priced, preMs, full,
			postMs, splitD.postMs, splitIO.postMs, nil, splitD.post, splitIO.post, splitAll.stateEdgeSegmentSplit,
			stateEdgeRemainderSentence(item.Thread, family, postMs, full, preMs, splitAll, ownerClaim)))
	}
	return out
}

// convertStateSeatToEdgeAnchored rewrites one bare-census state seat into the
// ⛓ pre-edge seat: lane/causality/basis/typed edge pair, value channels =
// the pre-edge account (per-state for D/IO — exact, never apportioned), the
// seat's segment inventories replaced by their pre-edge clips (so the
// AXIOM-V2 direction support union reproduces the published value — the
// support==claim identity holds on both halves by construction), Score
// re-derived from the published value (§7.30 S1 discipline). The per-(thread,
// cpu) group hulls (familyMemberIntervals) are CLEARED on both halves: they
// describe the pre-split full account and would let a cross-type exact recon
// match a half seat against a full-account interval identity (偏离备案:
// halves carry their own precise clipped inventories instead).
func convertStateSeatToEdgeAnchored(item *RootCauseRankItem, anchor semanticHostEdgeAnchor,
	totalMs, runnableMs, dMs, ioMs float64,
	runnableSegs, dSegs, ioSegs []foldInterval, split stateEdgeSegmentSplit, summary rspaSentence) {
	item.ChainRelevance = "on_chain"
	item.Causality = "on_wakeup_chain"
	item.OnChainBasis = RootCauseOnChainBasisHostWakeupEdgeState
	item.HostWakeupEdgeAnchorTs = anchor.boundaryTs
	item.HostWakeupEdgeAnchorVia = anchor.via()
	item.RunnableMs = runnableMs
	item.DStateMs = dMs
	item.IOWaitMs = ioMs
	item.ImpactMs = totalMs
	item.ProjectedImpactMs = totalMs
	item.CumulativeImpactMs = totalMs
	item.EffectiveImpactMs = totalMs
	item.Score = totalMs * item.Confidence * rootCauseItemScoreWeight(*item)
	item.runnableIntervals = runnableSegs
	item.dioSegmentIntervalsD = dSegs
	item.dioSegmentIntervalsIO = ioSegs
	item.dioSegmentIntervals = nil
	if len(dSegs) > 0 || len(ioSegs) > 0 {
		item.dioSegmentIntervals = append(append([]foldInterval(nil), dSegs...), ioSegs...)
	}
	item.familyMemberIntervals = nil
	// The state totals above are now the pre-edge account; an inventory
	// copied at construction no longer describes them.
	item.measuredStateIntervals = nil
	// TAILHYG-1 defensive clear (§29.156 备案): a successfully lane-switched
	// seat never carries the PARTSPLIT R4-mirror refusal quartet. Invariant:
	// 换道席永不携拒转记录 — the on-chain basis and a refusal-disclosure
	// record are mutually exclusive accounts of ONE seat (the harvest would
	// otherwise mint a disclosure row for a seat that DID convert). Today
	// this path is unreachable with a stamped quartet: the stamp mints only
	// inside the priority_inversion_runnable_wait case arm (which either
	// converts whole via its own inline form — no quartet on that path — or
	// continues without calling this converter), while this converter is
	// reached only from the runnable_wait / D-IO case arms; item.Type never
	// mutates between the two deterministic rank passes, so no seat can flow
	// from the stamp site to this site. Cleared defensively anyway (precise
	// zero-cost guard; pinned by TestConvertStateSeatClearsStaleRefusalQuartet).
	item.GatedCompositeEdgePreShareMs = 0
	item.GatedCompositeEdgePostShareMs = 0
	item.GatedCompositeEdgeAnchorTs = 0
	item.GatedCompositeEdgeAnchorVia = ""
	if split.preEnd > split.preStart {
		item.StartTs, item.EndTs = split.preStart, split.preEnd
	}
	rspaSetSentence(item, summary)
	// HMC377 fix round 5: the edge seat holds its pre-edge account (the
	// residual-lane role; the unwalked-seat mint re-records its own role).
	item.rspaHeld = nil
	rspaHold(item, RootCauseShareHolderResidualSeat, len(runnableSegs) == 0 && (len(dSegs) > 0 || len(ioSegs) > 0),
		rspaFoldUnion(runnableSegs, dSegs, ioSegs), totalMs)
}

// mintStateEdgeRemainderClone mints the ◇ post-edge twin from the pre-rewrite
// seat copy: the RSPA remainder trio + marker (side-lane, fold anchor-form
// and twin-visibility plumbing all existing), values = the post-edge account,
// segment inventories = the post-edge clips, typed edge pair kept as the
// disclosure of WHICH boundary split this account (the span remainder clone
// precedent keeps the pair too).
func mintStateEdgeRemainderClone(clone RootCauseRankItem, anchor semanticHostEdgeAnchor,
	preMs, fullMs, postMs, dMs, ioMs float64,
	runnableSegs, dSegs, ioSegs []foldInterval, split stateEdgeSegmentSplit, summary rspaSentence) RootCauseRankItem {
	rspaClearClaims(&clone)
	clone.Rank = 0
	clone.BackgroundRank = 0
	clone.Tier = ""
	clone.ChainRelevance = "adjacent"
	clone.Causality = "adjacent_to_wakeup_chain"
	clone.OnChainBasis = ""
	clone.ChainAnchorRemainderSeat = true
	clone.ChainAnchoredMs = preMs
	clone.ChainAnchorFullMs = fullMs
	clone.HostWakeupEdgeAnchorTs = anchor.boundaryTs
	clone.HostWakeupEdgeAnchorVia = anchor.via()
	var runnableMs float64
	if len(runnableSegs) > 0 {
		runnableMs = postMs
	}
	clone.RunnableMs = runnableMs
	clone.DStateMs = dMs
	clone.IOWaitMs = ioMs
	clone.ImpactMs = postMs
	clone.ProjectedImpactMs = postMs
	clone.CumulativeImpactMs = postMs
	clone.EffectiveImpactMs = postMs
	clone.Score = postMs * clone.Confidence * rootCauseItemScoreWeight(clone)
	clone.runnableIntervals = runnableSegs
	clone.dioSegmentIntervalsD = dSegs
	clone.dioSegmentIntervalsIO = ioSegs
	clone.dioSegmentIntervals = nil
	if len(dSegs) > 0 || len(ioSegs) > 0 {
		clone.dioSegmentIntervals = append(append([]foldInterval(nil), dSegs...), ioSegs...)
	}
	clone.familyMemberIntervals = nil
	// Same rule as the pre-edge half: the post-edge totals are a new account.
	clone.measuredStateIntervals = nil
	if split.postEnd > split.postStart {
		clone.StartTs, clone.EndTs = split.postStart, split.postEnd
	}
	rspaSetSentence(&clone, summary)
	rspaHold(&clone, RootCauseShareHolderRemainderSeat, len(runnableSegs) == 0 && (len(dSegs) > 0 || len(ioSegs) > 0),
		rspaFoldUnion(runnableSegs, dSegs, ioSegs), postMs)
	return clone
}

// GatedCompositeEdgeShareDisclosure (PARTSPLIT-1, §29.150④ user ruling
// 2026-07-19) is one R4-mirror-refused gated composite seat's NON-SEAT
// pre-edge-share disclosure record — the result-level side channel the ◎
// overview mentions it through (no ordinal, never in a section maximum,
// never additive to the seat's published value). PreMs + PostMs == AccountMs
// (the seat's runnable census account) to the µs by construction.
type GatedCompositeEdgeShareDisclosure struct {
	Thread ThreadRef `json:"thread"`
	// PreMs / PostMs — the X/Y bisection measures (disclosure only).
	PreMs  float64 `json:"pre_ms"`
	PostMs float64 `json:"post_ms"`
	// AccountMs — the seat's runnable census account the identity holds
	// against (NOT the gated eff: the composite eff is exactly what the R4
	// floor refuses to split).
	AccountMs float64 `json:"account_ms"`
	// BoundaryTs / Via — WHICH credential edge bisected the account (the R3
	// closed-set via vocabulary).
	BoundaryTs float64 `json:"boundary_ts"`
	Via        string  `json:"via"`
	// SeatPublished — the refused seat itself survived the publication cap
	// (typed honesty input for the ◎ mention's off-board clause; the tieba
	// 23088 live form is false: the seat lives only in the pool).
	SeatPublished bool `json:"seat_published"`
	// LineStart / LineEnd — the refused seat's own trace-line evidence range
	// (verbatim from the stamped item; the wire record's grounding span).
	LineStart int `json:"line_start,omitempty"`
	LineEnd   int `json:"line_end,omitempty"`
}

// harvestGatedCompositeEdgeShareDisclosures builds the result-level side
// channel from the pre-truncation pool ∪ the published board (the pool is the
// build+enrich union and may hold one seat twice — first stamped occurrence
// wins per (pid, boundary)). Admission is the typed stamp itself (all four
// fields minted atomically at the single R4-mirror refusal site); absence
// harvests nothing. Runs at BOTH rank-pass tails (idempotent overwrite: the
// enrich harvest re-reads the union, so publishedness reflects the final
// board).
//
// POOL2-1 件③ (§29.160③ user ruling 2026-07-20 「复用 SPANVIS 双分量地板…+
// 行序改值降序;微真值降入 typed 记号(审计保留)」): a row whose PRE-EDGE
// share sits below the SPANVIS two-component significance floor —
// max(BusinessSpanMentionDustFloorMs, BusinessSpanMentionWindowShareFloor ×
// window) — is NOT issued on the disclosure channel (noise discipline; the
// share components reuse the SPANVIS constants verbatim, zero second table).
// 宁降不删: the refused seat's typed four-field stamp (GatedCompositeEdge*)
// stays on the item (wire-visible when the seat publishes; pool-only seats retain it in process) — the audit record
// survives, only the reader-facing row is withheld; a windowless query keeps
// the dust component alone (demote, never delete). Rows are ordered by
// pre-edge share DESC (ties: line start asc, then pid asc — a deterministic
// READING order, not an ordinal).
func harvestGatedCompositeEdgeShareDisclosures(q Query, pool, published []RootCauseRankItem) []GatedCompositeEdgeShareDisclosure {
	floorMs := BusinessSpanMentionDustFloorMs
	if windowMs := (q.TimeEnd - q.TimeStart) * 1000; windowMs > 0 {
		if share := windowMs * BusinessSpanMentionWindowShareFloor; share > floorMs {
			floorMs = share
		}
	}
	type key struct {
		pid      int
		boundary float64
	}
	publishedSet := map[key]bool{}
	for i := range published {
		item := &published[i]
		if item.GatedCompositeEdgePreShareMs > 0 && item.GatedCompositeEdgePostShareMs > 0 {
			publishedSet[key{item.Thread.PID, item.GatedCompositeEdgeAnchorTs}] = true
		}
	}
	var out []GatedCompositeEdgeShareDisclosure
	seen := map[key]bool{}
	for _, items := range [][]RootCauseRankItem{pool, published} {
		for i := range items {
			item := &items[i]
			if item.GatedCompositeEdgePreShareMs <= 0 || item.GatedCompositeEdgePostShareMs <= 0 ||
				item.GatedCompositeEdgeAnchorTs <= 0 {
				continue
			}
			if item.GatedCompositeEdgePreShareMs < floorMs {
				// 件③ floor: micro pre-edge shares stay on the typed stamp
				// (audit) and never mint a disclosure row.
				continue
			}
			k := key{item.Thread.PID, item.GatedCompositeEdgeAnchorTs}
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, GatedCompositeEdgeShareDisclosure{
				Thread:        item.Thread,
				PreMs:         item.GatedCompositeEdgePreShareMs,
				PostMs:        item.GatedCompositeEdgePostShareMs,
				AccountMs:     item.RunnableMs,
				BoundaryTs:    item.GatedCompositeEdgeAnchorTs,
				Via:           item.GatedCompositeEdgeAnchorVia,
				SeatPublished: publishedSet[k],
				LineStart:     item.LineStart,
				LineEnd:       item.LineEnd,
			})
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].PreMs != out[b].PreMs {
			return out[a].PreMs > out[b].PreMs
		}
		if out[a].LineStart != out[b].LineStart {
			return out[a].LineStart < out[b].LineStart
		}
		return out[a].Thread.PID < out[b].Thread.PID
	})
	return out
}
