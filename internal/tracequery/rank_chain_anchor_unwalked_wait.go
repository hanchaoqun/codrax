package tracequery

// rank_chain_anchor_unwalked_wait.go — HMC377 fix rounds 2 and 3: one owner
// for a chain thread's time inside waits its own wakeup ended that the chain
// did not walk.
//
// Rule (approved E2 rule): a thread other than the analysed one is a cause
// only for its own ready / blocked / deterministic-work time inside a wait of
// the analysed thread that its own recorded wakeup ended — walked by the chain
// expansion or not. RSPA prices the walked part (the chain windows, held by
// the chain seat); the E2 carrier (ChainResult.directDependencyWindows) holds
// every wait the thread's wakeup ended. The part inside the waits the chain
// did not walk is the "unwalked share".
//
// Round 2 priced that share arm by arm, each arm cutting its own inventory and
// minting or assuming a seat: a satellite checked representation against a
// snapshot taken before the pass (it never saw the seat minted in the same
// pass — on-chain double count), several arms subtracted the share and said
// it was priced by a seat nobody minted, and the ◇ remainder carried
// walked + unwalked as "anchored" while the ⛓ chain seat said walked only.
//
// Round 3 mechanism — compute once, own once, read everywhere:
//   - ledger: per (thread, state family) the census inventory is partitioned
//     ONCE, before any arm runs, into walked (inside the chain windows),
//     unwalked (inside the thread's own-ended waits, outside the chain
//     windows) and outside; the windows are disjoint by construction and the
//     partition is proven against every census group's account and walked
//     stamp (fail closed otherwise);
//   - ownership: exactly one owner per (thread, family) is decided by
//     rspaPlanUnwalkedOwners — the thread's own unwalked seat when a chain-
//     lane seat holds the walked share (case A/A'), the clipped window seat
//     itself when none does (case B, walked + unwalked one object), the
//     residual-lane seat when nothing was walked (STATERES-1), or none;
//   - the own seat is minted by rspaMintUnwalkedSeat only, before the arms;
//   - every arm reads the ledger for both its representation check and its
//     sentence, and subtracts the unwalked share only when its owner exists;
//   - the three parts ride typed on the records (ChainAnchoredMs /
//     ChainAnchorUnwalkedMs / ChainAnchorOutsideMs, full = the three) so every
//     face renders them from engine values, never display arithmetic.

import (
	"fmt"
	"sort"
	"strings"
)

// unwalkedDependencyWindows returns the parts of a thread's dependency windows
// (the analysed thread's waits its own wakeup ended) that lie outside the
// thread's chain windows (merged, sorted). Each part keeps the closing wakeup
// of the wait it belongs to.
func unwalkedDependencyWindows(windows []targetWaitDependencyWindow, walked []TimeWindow) []targetWaitDependencyWindow {
	if len(windows) == 0 {
		return nil
	}
	walkedFold := rspaWindowsFold(walked)
	var out []targetWaitDependencyWindow
	for _, w := range windows {
		if w.endTs <= w.startTs {
			continue
		}
		for _, piece := range foldIntervalSubtract([]foldInterval{{start: w.startTs, end: w.endTs}}, walkedFold) {
			if piece.end > piece.start {
				out = append(out, targetWaitDependencyWindow{startTs: piece.start, endTs: piece.end, wakeupTs: w.wakeupTs})
			}
		}
	}
	return out
}

func rspaWindowsFold(windows []TimeWindow) []foldInterval {
	fold := make([]foldInterval, 0, len(windows))
	for _, w := range windows {
		if w.EndTs > w.StartTs {
			fold = append(fold, foldInterval{start: w.StartTs, end: w.EndTs})
		}
	}
	fold, _ = foldIntervalUnionWithDisjoint(fold)
	return fold
}

func rspaFoldWindows(fold []foldInterval) []TimeWindow {
	out := make([]TimeWindow, 0, len(fold))
	for _, iv := range fold {
		if iv.end > iv.start {
			out = append(out, TimeWindow{StartTs: iv.start, EndTs: iv.end})
		}
	}
	return mergeAnchorTimeWindows(out)
}

// rspaUnwalkedOwner is the one owner of a (thread, family) unwalked share.
type rspaUnwalkedOwner int

const (
	// rspaUnwalkedOwnerNone — nothing prices the share here: no arm subtracts
	// it and every sentence says the row's time is outside the WALKED chain
	// windows only.
	rspaUnwalkedOwnerNone rspaUnwalkedOwner = iota
	// rspaUnwalkedOwnerOwnSeat — the thread's own unwalked seat (a chain-lane
	// seat holds the walked share and cannot absorb other time).
	rspaUnwalkedOwnerOwnSeat
	// rspaUnwalkedOwnerWalkedSeat — case B: no chain-lane seat; the clipped
	// window seat holds walked + unwalked as one object.
	rspaUnwalkedOwnerWalkedSeat
	// rspaUnwalkedOwnerResidualSeat — nothing of the thread was walked; the
	// STATERES-1 residual lane priced its own-ended waits on its window seat.
	rspaUnwalkedOwnerResidualSeat
)

// rspaUnwalkedPart is one (thread, state family) row of the ledger.
type rspaUnwalkedPart struct {
	thread     ThreadRef
	dio        bool
	walked     []TimeWindow
	walkedFold []foldInterval
	unwalked   []targetWaitDependencyWindow
	// proven: every census group of the thread and family reproduces its
	// account and its walked stamp, so the partition below is exact.
	proven                                  bool
	fullMs, walkedMs, unwalkedMs, outsideMs float64
	unwalkedDMs, unwalkedIOMs               float64
	pieces, piecesD, piecesIO               []foldInterval
	split                                   dependencyWindowSplit
	splitD, splitIO                         dependencyWindowSplit
	anchor                                  semanticHostEdgeAnchor

	owner rspaUnwalkedOwner
	// ownerSupport: the instants the owner holds (the unwalked pieces; for a
	// case-B owner the walked windows too; for a residual owner its own
	// published support).
	ownerSupport []foldInterval
	// minted: the own seat was minted (now or by an earlier pass).
	minted bool
}

// rspaUnwalkedLedger holds the parts of one pass, keyed by pid.
type rspaUnwalkedLedger struct {
	runnable map[int]*rspaUnwalkedPart
	dio      map[int]*rspaUnwalkedPart
}

func (l rspaUnwalkedLedger) part(pid int, dio bool) *rspaUnwalkedPart {
	if dio {
		return l.dio[pid]
	}
	return l.runnable[pid]
}

// owns reports whether the share is held by a seat (the subtraction gate).
func (p *rspaUnwalkedPart) owns() bool {
	return p != nil && p.owner != rspaUnwalkedOwnerNone && p.unwalkedMs > rspaAnchorIdentityTolMs
}

// ownSeat reports the case-A/A' three-way form (walked on the chain seat,
// unwalked on its own seat).
func (p *rspaUnwalkedPart) ownSeat() bool {
	return p.owns() && p.owner == rspaUnwalkedOwnerOwnSeat
}

// rowOwnedMs is the part of one row inventory held by the owner: its
// instants outside the walked windows that the owner holds. 0 without an
// owner (nothing is subtracted).
func (p *rspaUnwalkedPart) rowOwnedMs(inventory []foldInterval) float64 {
	if !p.owns() || len(inventory) == 0 {
		return 0
	}
	merged, _ := foldIntervalUnionWithDisjoint(inventory)
	outsideWalked := foldIntervalSubtract(merged, p.walkedFold)
	return rspaIntervalsOverlapMs(rspaFoldWindows(p.ownerSupport), outsideWalked)
}

// rowUnwalkedMs is the part of one row inventory inside the thread's unwalked
// windows, owned or not (the disclosure of an unpriced share).
func (p *rspaUnwalkedPart) rowUnwalkedMs(inventory []foldInterval) float64 {
	if p == nil || len(inventory) == 0 {
		return 0
	}
	merged, _ := foldIntervalUnionWithDisjoint(inventory)
	return splitFoldIntervalsByDependencyWindows(foldIntervalSubtract(merged, p.walkedFold), p.unwalked).preMs
}

// clause names the unwalked waits for every sentence that prices or excludes
// them.
func (p *rspaUnwalkedPart) clause() string {
	waits, waitMs := 0, 0.0
	seen := map[float64]bool{}
	for _, w := range p.unwalked {
		waitMs += (w.endTs - w.startTs) * 1000
		if !seen[w.wakeupTs] {
			seen[w.wakeupTs] = true
			waits++
		}
	}
	if p.split.lastWakeupTs > 0 {
		return fmt.Sprintf("the analysed thread's wait(s) that this thread's own wakeup ended and that the dependency chain did not walk (%d wait(s), %.3fms of waiting; the latest one holding part of this account was ended at %.6f)",
			waits, waitMs, p.split.lastWakeupTs)
	}
	return fmt.Sprintf("the analysed thread's wait(s) that this thread's own wakeup ended and that the dependency chain did not walk (%d wait(s), %.3fms of waiting)", waits, waitMs)
}

// buildRSPAUnwalkedLedger partitions every migrating thread's census once.
// A thread whose own-ended waits all lie inside its chain windows (or that
// holds no direct credential) has no part: every arm keeps the chain
// windows' partition byte-identically.
func buildRSPAUnwalkedLedger(chain *ChainResult, stats WindowStats, runnable, dio map[int]rspaFamilyDecision) rspaUnwalkedLedger {
	var out rspaUnwalkedLedger
	if chain == nil {
		return out
	}
	for pid, decision := range runnable {
		if !decision.migrate {
			continue
		}
		if part := rspaBuildUnwalkedPart(chain, stats, pid, false); part != nil {
			if out.runnable == nil {
				out.runnable = map[int]*rspaUnwalkedPart{}
			}
			out.runnable[pid] = part
		}
	}
	for pid, decision := range dio {
		if !decision.migrate {
			continue
		}
		if part := rspaBuildUnwalkedPart(chain, stats, pid, true); part != nil {
			if out.dio == nil {
				out.dio = map[int]*rspaUnwalkedPart{}
			}
			out.dio[pid] = part
		}
	}
	return out
}

func rspaBuildUnwalkedPart(chain *ChainResult, stats WindowStats, pid int, dio bool) *rspaUnwalkedPart {
	if pid <= 0 || pid == chain.Target.PID {
		return nil
	}
	walked := stats.chainAnchorsByPID[pid]
	type group struct {
		td ThreadDuration
		io bool
	}
	var groups []group
	collect := func(census map[string]ThreadDuration, io bool) {
		keys := make([]string, 0, len(census))
		for key, td := range census {
			if td.Thread.PID == pid && td.DurationMs > 0 {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys) // deterministic group order (sums, thread identity)
		for _, key := range keys {
			groups = append(groups, group{td: census[key], io: io})
		}
	}
	if dio {
		collect(stats.dstateCensus, false)
		collect(stats.iowaitCensus, true)
	} else {
		collect(stats.runnableCensus, false)
	}
	if len(groups) == 0 {
		return nil
	}
	thread := groups[0].td.Thread
	anchor, ok := hostDirectCensusEdgeAnchor(chain, thread)
	if !ok || len(anchor.windows) == 0 {
		return nil
	}
	unwalked := unwalkedDependencyWindows(anchor.windows, walked)
	if len(unwalked) == 0 {
		return nil
	}
	part := &rspaUnwalkedPart{thread: thread, dio: dio, walked: walked, walkedFold: rspaWindowsFold(walked), unwalked: unwalked, proven: true}
	var outsideWalkedAll, outsideWalkedD, outsideWalkedIO []foldInterval
	for _, g := range groups {
		td := g.td
		part.fullMs += td.DurationMs
		part.walkedMs += td.anchoredMs
		inventory := td.runnableIntervals
		if dio {
			if td.dioIntervalsOverflow {
				part.proven = false
				continue
			}
			inventory = td.dioIntervals
		}
		merged, reproduces := rspaInventoryReproducesAccount(inventory, walked, td.DurationMs, td.anchoredMs)
		if !reproduces {
			part.proven = false
			continue
		}
		rest := foldIntervalSubtract(merged, part.walkedFold)
		outsideWalkedAll = append(outsideWalkedAll, rest...)
		if g.io {
			outsideWalkedIO = append(outsideWalkedIO, rest...)
		} else {
			outsideWalkedD = append(outsideWalkedD, rest...)
		}
	}
	if !part.proven {
		return part
	}
	outsideWalkedAll, _ = foldIntervalUnionWithDisjoint(outsideWalkedAll)
	part.split = splitFoldIntervalsByDependencyWindows(outsideWalkedAll, unwalked)
	part.unwalkedMs = part.split.preMs
	part.outsideMs = part.split.postMs
	part.pieces = part.split.pre
	if dio {
		outsideWalkedD, _ = foldIntervalUnionWithDisjoint(outsideWalkedD)
		outsideWalkedIO, _ = foldIntervalUnionWithDisjoint(outsideWalkedIO)
		part.splitD = splitFoldIntervalsByDependencyWindows(outsideWalkedD, unwalked)
		part.splitIO = splitFoldIntervalsByDependencyWindows(outsideWalkedIO, unwalked)
		part.unwalkedDMs, part.unwalkedIOMs = part.splitD.preMs, part.splitIO.preMs
		part.piecesD, part.piecesIO = part.splitD.pre, part.splitIO.pre
		if !rspaWithinTol(part.unwalkedDMs+part.unwalkedIOMs, part.unwalkedMs) {
			part.proven = false
			return part
		}
	}
	if !rspaWithinTol(part.walkedMs+part.unwalkedMs+part.outsideMs, part.fullMs) {
		part.proven = false
		return part
	}
	part.anchor = anchor.pricedBy(part.split)
	return part
}

// rspaInventoryReproducesAccount is the belt every unwalked-share reading
// takes: the merged inventory reproduces the account it claims to be, and its
// overlap with the chain windows reproduces the census-stamped walked share.
func rspaInventoryReproducesAccount(inventory []foldInterval, walkedWindows []TimeWindow, fullMs, walkedMs float64) ([]foldInterval, bool) {
	if len(inventory) == 0 {
		return nil, false
	}
	merged, _ := foldIntervalUnionWithDisjoint(inventory)
	lengthMs, okLength := foldIntervalsLengthMs(merged)
	if !okLength || !rspaWithinTol(lengthMs, fullMs) {
		return nil, false
	}
	if !rspaWithinTol(rspaIntervalsOverlapMs(walkedWindows, merged), walkedMs) {
		return nil, false
	}
	return merged, true
}

// rspaUnwalkedFamilyRow reports whether a row belongs to the runnable or the
// D/IO family (the state family its value channels publish).
func rspaUnwalkedFamilyRow(item RootCauseRankItem, dio bool) bool {
	if dio {
		return item.DominantState == string(StateDSleep) || item.DominantState == string(StateIOWait)
	}
	return item.DominantState == string(StateRunnable)
}

// rspaFormalWindowSeat reports the formal window seats whose census account
// the ledger partitions (the arms that split their own inventory).
func rspaFormalWindowSeat(item RootCauseRankItem, dio bool) bool {
	typ := strings.TrimSpace(item.Type)
	if dio {
		return (typ == "d_state_or_io_wait" || typ == "io_wait") &&
			(item.Source == "window_stats" || item.Source == "window_stats.io_wait_top")
	}
	return (typ == "runnable_wait" || typ == "priority_inversion_runnable_wait") && item.Source == "window_stats"
}

// rspaUnwalkedTemplateRow reports the other rows of a family an own seat may
// borrow its presentation identity from when no formal seat is in the pass
// (the value always comes from the ledger).
func rspaUnwalkedTemplateRow(item RootCauseRankItem, dio bool) bool {
	switch strings.TrimSpace(item.Type) {
	case "scheduler_latency", "low_frequency", "fragmented_runnable_wait", "cpu_affinity_or_cpuset":
		return !dio
	case "fragmented_d_state_or_io_wait":
		return dio
	}
	return false
}

// rspaArmWillProcess mirrors the entry gate of the re-anchoring loop: the row
// is an unprocessed on-chain seat of a non-target thread without a basis.
func rspaArmWillProcess(item RootCauseRankItem, target ThreadRef) bool {
	if item.ChainAnchorRemainderSeat || item.ChainAnchorFullMs > 0 || item.AbsorbedByRankFamily ||
		item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
		item.inversionAccountRepresentedBy != "" || item.ChainAnchorUnwalkedSeat {
		return false
	}
	if item.Thread.PID <= 0 || !rootCauseItemIsOnChain(item) || rspaRowIsSelfExempt(item, target) {
		return false
	}
	return strings.TrimSpace(item.OnChainBasis) == ""
}

// rspaFormalSeatShare reads one formal window seat's walked share and account
// the way its arm will (ledger stamp, or the pid identity on stamp-less
// seats). ok=false when the arm would leave the seat untouched on the chain
// lane (an unmigrable seat keeps its whole account there, so no other seat
// may own any of its instants).
func rspaFormalSeatShare(item RootCauseRankItem, decision rspaFamilyDecision, dio bool) (walked, full float64, inventory []foldInterval, ok bool) {
	if dio {
		if !item.ledgerAnchorStamped {
			return 0, 0, nil, false
		}
		walked, full = item.ledgerAnchoredDMs+item.ledgerAnchoredIOMs, item.DStateMs+item.IOWaitMs
		if item.ledgerAnchoredDMs > item.DStateMs+rspaAnchorIdentityTolMs || item.ledgerAnchoredIOMs > item.IOWaitMs+rspaAnchorIdentityTolMs {
			return 0, 0, nil, false
		}
		return walked, full, item.dioSegmentIntervals, true
	}
	switch {
	case item.ledgerAnchorStamped:
		walked, full = item.ledgerAnchoredRunnableMs, item.RunnableMs
	case rspaWithinTol(item.RunnableMs, decision.fullMs):
		walked, full = decision.anchoredMs, decision.fullMs
	default:
		return 0, 0, nil, false
	}
	if full <= 0 || walked > full+rspaAnchorIdentityTolMs {
		return 0, 0, nil, false
	}
	return walked, full, item.runnableIntervals, true
}

// rspaPlanUnwalkedOwners decides the one owner of every proven unwalked share
// and returns the own seats to mint in this pass (minted once per board: an
// earlier pass's own seat — on the board or compacted off it — is never minted
// again). Runs before every arm.
func rspaPlanUnwalkedOwners(chain ChainResult, ledger rspaUnwalkedLedger, items []RootCauseRankItem,
	chainSeats map[int]rspaChainSeatPresence, runnable, dio map[int]rspaFamilyDecision,
	decomposable map[int]bool) []RootCauseRankItem {
	var minted []RootCauseRankItem
	plan := func(part *rspaUnwalkedPart, pid int, decision rspaFamilyDecision, dioFamily bool) {
		if part == nil || !part.proven || part.unwalkedMs <= rspaAnchorIdentityTolMs {
			return
		}
		if part.walkedMs <= rspaAnchorIdentityTolMs {
			// Nothing walked: the STATERES-1 residual lane owns the thread's
			// own-ended waits on its window seat. Owner only while that seat
			// is on the board of this pass.
			for i := range items {
				it := &items[i]
				if it.Thread.PID == pid && it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState &&
					!it.ChainAnchorUnwalkedSeat && rootCauseItemIsOnChain(*it) && rspaUnwalkedFamilyRow(*it, dioFamily) {
					part.owner = rspaUnwalkedOwnerResidualSeat
					support := it.runnableIntervals
					if dioFamily {
						support = it.dioSegmentIntervals
					}
					part.ownerSupport = append(part.ownerSupport, support...)
				}
			}
			if part.owner == rspaUnwalkedOwnerResidualSeat {
				part.ownerSupport, _ = foldIntervalUnionWithDisjoint(part.ownerSupport)
			}
			return
		}
		// An earlier pass already decided: its own seat (on this board or
		// compacted off it), its case-B clipped seat, or — when its formal
		// seats were split without either — no owner.
		decidedEarlier := false
		for i := range items {
			it := &items[i]
			if it.Thread.PID != pid || !rspaUnwalkedFamilyRow(*it, dioFamily) {
				continue
			}
			switch {
			case it.ChainAnchorUnwalkedSeat, it.ChainAnchorUnwalkedMs > 0:
				part.owner, part.minted, part.ownerSupport = rspaUnwalkedOwnerOwnSeat, true, part.pieces
				return
			case it.rspaUnwalkedHeldMs > 0:
				part.owner = rspaUnwalkedOwnerWalkedSeat
				part.ownerSupport, _ = foldIntervalUnionWithDisjoint(append(append([]foldInterval(nil), part.walkedFold...), part.pieces...))
				return
			case rspaFormalWindowSeat(*it, dioFamily) && (it.ChainAnchorFullMs > 0 || it.ChainAnchorRemainderSeat ||
				it.ChainCredentialLaneDemoted || it.inversionAccountRepresentedBy != ""):
				decidedEarlier = true
			}
		}
		if decidedEarlier {
			return
		}
		// This pass decides. Every formal seat that holds part of the share
		// must give it up (an unmigrable seat keeps its whole account on the
		// chain lane — then nothing else may own any of its instants), and
		// together they must hold exactly the ledger's share.
		var formal []int
		heldMs := 0.0
		for i := range items {
			it := items[i]
			if it.Thread.PID != pid || !rspaFormalWindowSeat(it, dioFamily) || !rspaArmWillProcess(it, chain.Target) {
				continue
			}
			if !dioFamily && strings.TrimSpace(it.Type) == "priority_inversion_runnable_wait" && decomposable[pid] {
				continue // demoted whole beside the un-retyped seat that owns the same account
			}
			raw := it.runnableIntervals
			if dioFamily {
				raw = it.dioSegmentIntervals
			}
			walked, full, inventory, ok := rspaFormalSeatShare(it, decision, dioFamily)
			if !ok {
				if part.rowUnwalkedMs(raw) > rspaAnchorIdentityTolMs || len(raw) == 0 {
					return // it stays whole on the chain lane with (possibly) unwalked instants
				}
				continue
			}
			if _, reproduces := rspaInventoryReproducesAccount(inventory, part.walked, full, walked); !reproduces {
				return
			}
			held := part.rowUnwalkedMs(inventory)
			if held <= rspaAnchorIdentityTolMs {
				continue
			}
			formal = append(formal, i)
			heldMs += held
		}
		presence := chainSeats[pid]
		chainSeat := presence.runnable
		if dioFamily {
			chainSeat = presence.dio
		}
		if !chainSeat {
			// Case B: the clipped formal seat(s) hold walked + unwalked as
			// one object each; nothing to mint.
			if len(formal) == 0 || !rspaWithinTol(heldMs, part.unwalkedMs) {
				return
			}
			part.owner = rspaUnwalkedOwnerWalkedSeat
			part.ownerSupport, _ = foldIntervalUnionWithDisjoint(append(append([]foldInterval(nil), part.walkedFold...), part.pieces...))
			return
		}
		// Case A/A': one own seat holds the whole share.
		if len(formal) > 1 && dioFamily {
			return // several D/IO cause partitions — one seat cannot carry their causes
		}
		if len(formal) > 0 && !rspaWithinTol(heldMs, part.unwalkedMs) {
			return
		}
		template := -1
		if len(formal) > 0 {
			template = formal[0]
		} else {
			for i := range items {
				if items[i].Thread.PID == pid && rspaUnwalkedTemplateRow(items[i], dioFamily) && rspaArmWillProcess(items[i], chain.Target) {
					template = i
					break
				}
			}
		}
		if template < 0 {
			return
		}
		part.owner, part.minted, part.ownerSupport = rspaUnwalkedOwnerOwnSeat, true, part.pieces
		minted = append(minted, rspaMintUnwalkedSeat(items[template], part))
	}
	// Sorted pid order: the minted seats join the board in a deterministic
	// order (map iteration order must never reach the published rows).
	for _, pid := range rspaSortedPartPIDs(ledger.runnable) {
		plan(ledger.runnable[pid], pid, runnable[pid], false)
	}
	for _, pid := range rspaSortedPartPIDs(ledger.dio) {
		plan(ledger.dio[pid], pid, dio[pid], true)
	}
	return minted
}

func rspaSortedPartPIDs(parts map[int]*rspaUnwalkedPart) []int {
	pids := make([]int, 0, len(parts))
	for pid := range parts {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids
}

// rspaMintUnwalkedSeat is the ONE mint site of an unwalked seat: a copy of the
// family's template row carrying the ledger's share — the E2 host-edge state
// basis, the closing wakeup of the latest priced wait, value channels and
// support = the unwalked pieces only. The copy carries no partition trio and
// no census stamp (both describe the template's full account) and no member
// roster (its members are the pieces).
func rspaMintUnwalkedSeat(from RootCauseRankItem, part *rspaUnwalkedPart) RootCauseRankItem {
	seat := cloneRootCauseMeasurementSources(from)
	if !part.dio {
		seat = stateEdgePlainRunnableCopy(seat)
		seat.Type = "runnable_wait"
		seat.Source = "window_stats"
		seat.DominantState = string(StateRunnable)
	} else {
		switch {
		case part.unwalkedDMs <= rspaAnchorIdentityTolMs && part.unwalkedIOMs > 0:
			seat.Type, seat.Source, seat.DominantState = "io_wait", "window_stats.io_wait_top", string(StateIOWait)
		case strings.TrimSpace(seat.Type) != "d_state_or_io_wait" && strings.TrimSpace(seat.Type) != "io_wait":
			seat.Type, seat.Source, seat.DominantState = "d_state_or_io_wait", "window_stats", string(StateDSleep)
		}
	}
	seat.Rank, seat.BackgroundRank, seat.Tier = 0, 0, ""
	seat.ChainAnchorRemainderSeat = false
	seat.ChainAnchoredMs, seat.ChainAnchorFullMs = 0, 0
	seat.ChainAnchorUnwalkedMs, seat.ChainAnchorOutsideMs = 0, 0
	seat.ChainAnchorOwnershipDivergent = false
	seat.ChainAnchorChainLaneMs, seat.ChainAnchorCensusMs = 0, 0
	seat.ChainCredentialLaneDemoted, seat.ChainAnchorRepresentedByChainSeat = false, false
	seat.AbsorbedByRankFamily = false
	seat.ledgerAnchorStamped = false
	seat.ledgerAnchoredRunnableMs, seat.ledgerAnchoredDMs, seat.ledgerAnchoredIOMs = 0, 0, 0
	seat.inversionAccountRepresentedBy, seat.inversionAccountRepresents = "", ""
	seat.rspaUnwalkedHeldMs = 0
	seat.MemberCount, seat.MemberRoster, seat.MemberMaxMs, seat.MemberMinMs = 0, nil, 0, 0
	seat.MemberFoldCaliber, seat.MemberSumMs = "", 0
	seat.RunningMs, seat.SleepMs = 0, 0
	family := "runnable (scheduling-pressure candidate)"
	if part.dio {
		family = "D/IO blocking"
	}
	summary := rspaUnwalkedWaitSeatSummary(part.thread, family, part)
	if !part.dio {
		convertStateSeatToEdgeAnchored(&seat, part.anchor, part.unwalkedMs, part.unwalkedMs, 0, 0, part.pieces, nil, nil,
			part.split.stateEdgeSegmentSplit, summary)
	} else {
		convertStateSeatToEdgeAnchored(&seat, part.anchor, part.unwalkedMs, 0, part.unwalkedDMs, part.unwalkedIOMs, nil, part.piecesD, part.piecesIO,
			part.split.stateEdgeSegmentSplit, summary)
	}
	seat.ChainAnchorUnwalkedSeat = true
	return seat
}

// rspaSummaryUnwalkedOwnSeat is the verbatim claim every sentence makes when
// a share is held by the thread's own unwalked seat; the twin-visibility pass
// downgrades it when that seat is not on the published board.
const rspaSummaryUnwalkedOwnSeat = "(priced as this thread's own on-chain seat)"
const rspaSummaryUnwalkedOwnSeatUnpublished = "(priced as this thread's own on-chain seat, which is not on the published board; see the compaction disclosure)"

// rspaUnwalkedWaitSeatSummary — the unwalked seat's engine-side account.
func rspaUnwalkedWaitSeatSummary(thread ThreadRef, family string, part *rspaUnwalkedPart) string {
	return fmt.Sprintf("%s %s %.3fms inside %s; full-window account %.3fms = this share + %.3fms inside the dependency windows the chain walked for this thread + %.3fms outside every dependency window of this thread — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, part.unwalkedMs, part.clause(), part.fullMs, part.walkedMs, part.outsideMs)
}

// rspaAccountThreeWay — the "full = walked + unwalked + remainder" clause of
// every three-way sentence (walkedOwner names who holds the walked share).
func rspaAccountThreeWay(full, walked float64, walkedOwner string, unwalked float64, clause string) string {
	return fmt.Sprintf("full-window account %.3fms = %.3fms anchored inside the walked dependency windows %s + %.3fms inside %s %s + this remainder — same segment set, mutually disjoint, additive back to the full account",
		full, walked, walkedOwner, unwalked, clause, rspaSummaryUnwalkedOwnSeat)
}

// rspaRemainderSummaryThreeWay — a remainder whose account lies partly inside
// the walked chain windows (held by the chain seat) and partly inside the
// unwalked waits (held by the thread's own seat).
func rspaRemainderSummaryThreeWay(thread ThreadRef, family string, remainder, walked, unwalked, full float64, clause string) string {
	owner := rspaSummaryOwnedByChainSeat
	if walked <= 0 {
		owner = rspaSummaryNoAnchoredShare
	}
	return fmt.Sprintf("%s %s remainder %.3fms outside its wakeup-dependency windows — outside the dependency windows the chain walked for this thread and outside %s; the analysed thread was not waiting for this thread there (no chain credential for these segments); %s",
		threadLabel(thread), family, remainder, clause, rspaAccountThreeWay(full, walked, owner, unwalked, "those unwalked waits"))
}

// rspaRemainderSummaryDivergentThreeWay — the case-A' double-account form of
// a three-way remainder: the census split stays exact, the chain seat keeps
// its own (diverging) account and is never added.
func rspaRemainderSummaryDivergentThreeWay(thread ThreadRef, family string, remainder, walked, unwalked, full, chainLaneMs, censusMs float64, clause string) string {
	delta := chainLaneMs - censusMs
	if delta < 0 {
		delta = -delta
	}
	return fmt.Sprintf("%s %s remainder %.3fms outside its wakeup-dependency windows — outside the dependency windows the chain walked for this thread and outside %s (no chain credential for these segments); full-window account %.3fms = %.3fms anchored inside the walked dependency windows + %.3fms inside those unwalked waits %s + this remainder — same segment set, mutually disjoint. Anchored-ownership divergence: the chain seat publishes its own account %.3fms while the anchored ledger sum is %.3fms (delta %.3fms) — two separate accounts, never additive with each other",
		threadLabel(thread), family, remainder, clause, full, walked, unwalked, rspaSummaryUnwalkedOwnSeat, chainLaneMs, censusMs, delta)
}

// rspaRemainderSummaryWalkedOnly — a remainder of a thread with unwalked
// waits whose share no seat owns here: the sentence says what is true — the
// row's time lies outside the WALKED chain windows — and names the unpriced
// share inside the other waits when it is known (-1: not separable).
func rspaRemainderSummaryWalkedOnly(thread ThreadRef, family string, remainder, anchored, full, inside float64, clause string) string {
	owner := rspaSummaryOwnedByChainSeat
	if anchored <= 0 {
		owner = rspaSummaryNoAnchoredShare
	}
	return fmt.Sprintf("%s %s remainder %.3fms outside the dependency windows the chain walked for this thread (no chain credential on this row for these segments); %s; full-window account %.3fms = %.3fms anchored inside the walked dependency windows %s + this remainder — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, remainder, rspaUnpricedDetail(inside, clause), full, anchored, owner)
}

// rspaLaneDemotionSummaryThreeWay — the R4 whole-row demotion sentence for a
// thread whose account holds an owned unwalked share.
func rspaLaneDemotionSummaryThreeWay(walked, unwalked, full float64, clause, detail string) string {
	return fmt.Sprintf("; no chain credential for the full account: the governing full-window ledger account holds %.3fms of %.3fms inside the dependency windows the chain walked for this thread and %.3fms inside %s %s, and %s — the whole row rides the adjacent lane with every published value unchanged",
		walked, full, unwalked, clause, rspaSummaryUnwalkedOwnSeat, detail)
}

// rspaRepresentedDemotionSummaryThreeWay — the XLANE-1 represented form when
// the satellite's account is held by the chain seat (walked) and the thread's
// own unwalked seat.
func rspaRepresentedDemotionSummaryThreeWay(walked, unwalked, full float64, clause string) string {
	return fmt.Sprintf("; anchored share represented by the chain seats: this satellite's whole account (%.3fms of %.3fms) lies inside the thread's dependency windows — %.3fms inside the walked chain windows, which a same-pool chain-lane runnable seat physically covers, and %.3fms inside %s %s — this diagnostic projection rides the adjacent lane whole with every published value unchanged (a second full chain seat would double-represent one physical account)",
		walked+unwalked, full, walked, unwalked, clause, rspaSummaryUnwalkedOwnSeat)
}

// rspaInversionRepresentedSummaryUnwalked — the R-377-4 detail sentence when
// part of the ready account lies inside unwalked waits the thread's wakeup
// ended and its own seat holds it.
func rspaInversionRepresentedSummaryUnwalked(walked, inside, outside, full float64, clause string) string {
	return fmt.Sprintf("; represented by this thread's chain seat: %.3fms of this %.3fms ready-time account lies inside the dependency windows the chain walked for this thread, which that chain seat already publishes, %.3fms lies inside %s %s, and %.3fms lies outside every dependency window of this thread (published as the thread's adjacent ready-time remainder) — this whole-account row (its same-CPU lower-priority overlap is measured over the whole ready account and is never split) rides as that seat's lossless detail, not as a second object for the same thread",
		walked, full, inside, clause, rspaSummaryUnwalkedOwnSeat, outside)
}

// rspaStampThreeWay writes the typed three-way split on a record.
func rspaStampThreeWay(item *RootCauseRankItem, walked, unwalked, outside, full float64) {
	item.ChainAnchoredMs = walked
	item.ChainAnchorUnwalkedMs = unwalked
	item.ChainAnchorOutsideMs = outside
	item.ChainAnchorFullMs = full
}

// rspaRowScope is one row's reading of the ledger: the share its owner holds
// (subtracted from the row), or the share inside the unwalked waits nobody
// holds here (the row then speaks of the walked chain windows only).
type rspaRowScope struct {
	owned      float64
	ownedOwner rspaUnwalkedOwner
	walkedOnly bool
	// unpriced: the row's time inside the unwalked waits that this row keeps
	// (-1: not separable from its inventory).
	unpriced float64
}

// rowScope reads one row inventory (its account fullMs, its walked share
// walkedMs) against the ledger. allowOwner=false keeps the share on the row
// (a seat that may not give it up).
func (p *rspaUnwalkedPart) rowScope(inventory []foldInterval, fullMs, walkedMs float64, allowOwner bool) rspaRowScope {
	if p == nil {
		return rspaRowScope{}
	}
	merged, ok := rspaInventoryReproducesAccount(inventory, p.walked, fullMs, walkedMs)
	if !ok {
		return rspaRowScope{walkedOnly: true, unpriced: -1}
	}
	outsideWalked := foldIntervalSubtract(merged, p.walkedFold)
	rowU := splitFoldIntervalsByDependencyWindows(outsideWalked, p.unwalked).preMs
	if rowU <= rspaAnchorIdentityTolMs {
		return rspaRowScope{}
	}
	if allowOwner && p.owns() {
		owned := rspaIntervalsOverlapMs(rspaFoldWindows(p.ownerSupport), outsideWalked)
		if rspaWithinTol(owned, rowU) {
			return rspaRowScope{owned: owned, ownedOwner: p.owner}
		}
	}
	return rspaRowScope{walkedOnly: true, unpriced: rowU}
}

// threeWay: the row's walked share is held by a chain seat and its unwalked
// share by a separate seat (the own seat, or the residual-lane seat).
func (s rspaRowScope) threeWay() bool {
	return s.owned > rspaAnchorIdentityTolMs && s.ownedOwner != rspaUnwalkedOwnerWalkedSeat
}

// stamp writes the typed three-way split on a record of a three-way row.
func (s rspaRowScope) stamp(item *RootCauseRankItem, walked, outside, full float64) {
	if s.threeWay() {
		rspaStampThreeWay(item, walked, s.owned, outside, full)
	}
}

// oneObject: case B — the clipped window seat holds walked + unwalked.
func (p *rspaUnwalkedPart) oneObject(s rspaRowScope) bool {
	return p != nil && s.owned > rspaAnchorIdentityTolMs && s.ownedOwner == rspaUnwalkedOwnerWalkedSeat
}

// rowOwnedChannels splits an owned D/IO share per channel; ok=false when the
// channel inventories do not reproduce the combined share.
func (p *rspaUnwalkedPart) rowOwnedChannels(s rspaRowScope, d, io []foldInterval) (float64, float64, bool) {
	if s.owned <= rspaAnchorIdentityTolMs {
		return 0, 0, true
	}
	ownedOf := func(inventory []foldInterval) float64 {
		if len(inventory) == 0 {
			return 0
		}
		merged, _ := foldIntervalUnionWithDisjoint(inventory)
		return rspaIntervalsOverlapMs(rspaFoldWindows(p.ownerSupport), foldIntervalSubtract(merged, p.walkedFold))
	}
	ownedD, ownedIO := ownedOf(d), ownedOf(io)
	if !rspaWithinTol(ownedD+ownedIO, s.owned) {
		return 0, 0, false
	}
	return ownedD, ownedIO, true
}

// remainderSummary — the remainder sentence of one row (anchored is the
// share the row hands to its walked owner: walked, or walked + unwalked on a
// case-B one object).
func (p *rspaUnwalkedPart) remainderSummary(s rspaRowScope, thread ThreadRef, family string, remainder, anchored, full float64) string {
	switch {
	case p == nil:
		return rspaRemainderSummary(thread, family, remainder, anchored, full)
	case s.threeWay():
		return rspaRemainderSummaryThreeWay(thread, family, remainder, anchored, s.owned, full, p.clause())
	case s.walkedOnly:
		return rspaRemainderSummaryWalkedOnly(thread, family, remainder, anchored, full, s.unpriced, p.clause())
	default:
		return rspaRemainderSummary(thread, family, remainder, anchored, full)
	}
}

// divergentSummary — the case-A' double-account form of remainderSummary.
func (p *rspaUnwalkedPart) divergentSummary(s rspaRowScope, thread ThreadRef, family string, remainder, anchored, full, chainLaneMs, censusMs float64) string {
	switch {
	case p == nil:
		return rspaRemainderSummaryDivergent(thread, family, remainder, anchored, full, chainLaneMs, censusMs)
	case s.threeWay():
		return rspaRemainderSummaryDivergentThreeWay(thread, family, remainder, anchored, s.owned, full, chainLaneMs, censusMs, p.clause())
	case s.walkedOnly:
		return rspaRemainderSummaryDivergentWalkedOnly(thread, family, remainder, anchored, full, chainLaneMs, censusMs, s.unpriced, p.clause())
	default:
		return rspaRemainderSummaryDivergent(thread, family, remainder, anchored, full, chainLaneMs, censusMs)
	}
}

// anchoredSummary — the case-B clipped seat's sentence; it names what its
// anchored share covers when it holds unwalked waits too.
func (p *rspaUnwalkedPart) anchoredSummary(s rspaRowScope, thread ThreadRef, family string, inside, full, walked float64) string {
	if !p.oneObject(s) {
		return rspaAnchoredSummary(thread, family, inside, full)
	}
	return fmt.Sprintf("%s %s %.3fms anchored inside its typed wakeup-dependency windows (%.3fms inside the dependency windows the chain walked for this thread + %.3fms inside %s; one seat holds both) (chain credential); full-window account %.3fms = this anchored portion + %.3fms remainder outside the dependency windows %s — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, inside, walked, s.owned, p.clause(), full, full-inside, rspaSummaryRemainderTwinPublished)
}

// inversionRepresentedSummary — the R-377-4 detail sentence of the
// indivisible inversion row (case A / A').
func (p *rspaUnwalkedPart) inversionRepresentedSummary(s rspaRowScope, walked, outside, full float64) string {
	switch {
	case p != nil && s.threeWay():
		return rspaInversionRepresentedSummaryUnwalked(walked, s.owned, outside, full, p.clause())
	case p != nil && s.walkedOnly:
		return rspaInversionRepresentedSummaryWalkedOnly(walked, full, s.unpriced, p.clause())
	default:
		return rspaInversionRepresentedSummary(walked, full)
	}
}

// laneDemotionSummary — the R4 whole-row demotion sentence on the pid-level
// ledger account (walked = the census walked share).
func (p *rspaUnwalkedPart) laneDemotionSummary(walked, full float64, detail string) string {
	switch {
	case p == nil:
		return rspaLaneDemotionSummary(walked, full, detail)
	case p.owns() && p.owner == rspaUnwalkedOwnerWalkedSeat:
		return rspaLaneDemotionSummary(walked+p.unwalkedMs, full, detail)
	case p.owns():
		return rspaLaneDemotionSummaryThreeWay(walked, p.unwalkedMs, full, p.clause(), detail)
	case p.proven && p.unwalkedMs <= rspaAnchorIdentityTolMs:
		return rspaLaneDemotionSummary(walked, full, detail)
	case p.proven:
		return rspaLaneDemotionSummaryWalkedOnly(walked, full, p.unwalkedMs, p.clause(), detail)
	default:
		return rspaLaneDemotionSummaryWalkedOnly(walked, full, -1, p.clause(), detail)
	}
}

// representedSummary — the XLANE-1 represented sentence of a satellite.
func (p *rspaUnwalkedPart) representedSummary(s rspaRowScope, walked, full float64) string {
	switch {
	case p != nil && s.threeWay():
		return rspaRepresentedDemotionSummaryThreeWay(walked, s.owned, full, p.clause())
	case p != nil && p.oneObject(s):
		return rspaRepresentedDemotionSummaryOneObject(walked, s.owned, full, p.clause())
	default:
		return rspaRepresentedDemotionSummary(walked, full)
	}
}

// rspaRewritePIDLevelTwin re-anchors a pid-level row that carries no
// inventory of its own (the state-churn twins) on the ledger's pid account.
func rspaRewritePIDLevelTwin(item *RootCauseRankItem, part *rspaUnwalkedPart, decision rspaFamilyDecision,
	chainSeat bool, chainSeatMs, full float64, family string, dio bool) {
	anchored := decision.anchoredMs
	if anchored > full {
		anchored = full
	}
	scope := rspaRowScope{}
	switch {
	case part == nil:
	case part.owns() && anchored+part.unwalkedMs <= full+rspaAnchorIdentityTolMs:
		scope = rspaRowScope{owned: part.unwalkedMs, ownedOwner: part.owner}
	case part.proven && part.unwalkedMs <= rspaAnchorIdentityTolMs:
	default:
		// The pid holds time inside the unwalked waits that no seat owns here
		// (or the share does not fit this row's account): the row keeps it.
		scope = rspaRowScope{walkedOnly: true, unpriced: -1}
	}
	inside := anchored + scope.owned
	remainder := rspaClampNonNegative(full - inside)
	if remainder <= rspaAnchorIdentityTolMs && !chainSeat && scope.owned <= rspaAnchorIdentityTolMs {
		return
	}
	// The churn twin has no per-lane anchored split; scale both lanes by the
	// same remainder share so the family scalar stays exact.
	remainderRunnable, remainderD, remainderIO := remainder, 0.0, 0.0
	if dio {
		share := 0.0
		if full > 0 {
			share = remainder / full
		}
		remainderRunnable, remainderD, remainderIO = 0, item.DStateMs*share, item.IOWaitMs*share
	}
	handed := anchored // the share handed to the walked owner(s)
	if part.oneObject(scope) {
		handed = inside
	}
	if chainSeat && !(decision.identityHolds && rspaWithinTol(chainSeatMs, decision.anchoredMs)) {
		rspaRewriteSeatToRemainder(item, handed, full, remainderRunnable, remainderD, remainderIO,
			part.divergentSummary(scope, item.Thread, family, remainder, handed, full, chainSeatMs, decision.anchoredMs))
		scope.stamp(item, anchored, remainder, full)
		item.ChainAnchorOwnershipDivergent = true
		item.ChainAnchorChainLaneMs = chainSeatMs
		item.ChainAnchorCensusMs = decision.anchoredMs
		return
	}
	rspaRewriteSeatToRemainder(item, handed, full, remainderRunnable, remainderD, remainderIO,
		part.remainderSummary(scope, item.Thread, family, remainder, handed, full))
	scope.stamp(item, anchored, remainder, full)
}

// rspaRemainderSummaryDivergentWalkedOnly — case A' for a remainder that keeps
// time inside the unwalked waits.
func rspaRemainderSummaryDivergentWalkedOnly(thread ThreadRef, family string, remainder, anchored, full, chainLaneMs, censusMs, inside float64, clause string) string {
	delta := chainLaneMs - censusMs
	if delta < 0 {
		delta = -delta
	}
	return fmt.Sprintf("%s %s remainder %.3fms outside the dependency windows the chain walked for this thread (no chain credential on this row for these segments); %s; full-window account %.3fms = %.3fms anchored inside the walked dependency windows + this remainder — same segment set, mutually disjoint. Anchored-ownership divergence: the chain seat publishes its own account %.3fms while the anchored ledger sum is %.3fms (delta %.3fms) — two separate accounts, never additive with each other",
		threadLabel(thread), family, remainder, rspaUnpricedDetail(inside, clause), full, anchored, chainLaneMs, censusMs, delta)
}

// rspaUnpricedDetail names the time a row keeps inside the unwalked waits.
func rspaUnpricedDetail(inside float64, clause string) string {
	if inside >= 0 {
		return fmt.Sprintf("%.3fms of it lies inside %s, which this row does not price", inside, clause)
	}
	return "part of it may lie inside " + clause + " (not separated on this row)"
}

// rspaInversionRepresentedSummaryWalkedOnly — the R-377-4 detail sentence
// when the ready account keeps time inside the unwalked waits.
func rspaInversionRepresentedSummaryWalkedOnly(walked, full, inside float64, clause string) string {
	return fmt.Sprintf("; represented by this thread's chain seat: %.3fms of this %.3fms ready-time account lies inside the dependency windows the chain walked for this thread, which that chain seat already publishes, and %.3fms lies outside them (published as the thread's adjacent ready-time remainder; %s) — this whole-account row (its same-CPU lower-priority overlap is measured over the whole ready account and is never split) rides as that seat's lossless detail, not as a second object for the same thread",
		walked, full, full-walked, rspaUnpricedDetail(inside, clause))
}

// rspaLaneDemotionSummaryWalkedOnly — the R4 demotion sentence of a thread
// with time inside the unwalked waits that no seat owns here.
func rspaLaneDemotionSummaryWalkedOnly(walked, full, inside float64, clause, detail string) string {
	return fmt.Sprintf("; no chain credential for the full account: the governing full-window ledger account holds %.3fms of %.3fms inside the dependency windows the chain walked for this thread (%s), and %s — the whole row rides the adjacent lane with every published value unchanged",
		walked, full, rspaUnpricedDetail(inside, clause), detail)
}

// rspaRepresentedDemotionSummaryOneObject — the XLANE-1 represented form when
// the thread's clipped window seat holds walked + unwalked as one object.
func rspaRepresentedDemotionSummaryOneObject(walked, unwalked, full float64, clause string) string {
	return fmt.Sprintf("; anchored share represented by the thread's clipped on-chain window seat: this satellite's whole account (%.3fms of %.3fms) lies inside the thread's dependency windows — %.3fms inside the walked chain windows and %.3fms inside %s, all held by that one seat — this diagnostic projection rides the adjacent lane whole with every published value unchanged (a second full chain seat would double-represent one physical account)",
		walked+unwalked, full, walked, unwalked, clause)
}
