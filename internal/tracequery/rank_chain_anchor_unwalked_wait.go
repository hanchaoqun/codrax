package tracequery

// rank_chain_anchor_unwalked_wait.go — HMC377 fix round 2, item F3.
//
// A thread on the analysed thread's chain is priced over the union of its
// chain windows and its direct dependency windows. RSPA prices the chain
// windows (the waits the chain expansion walked: [wait start, recorded
// wakeup]); the E2 carrier (ChainResult.directDependencyWindows) holds every
// wait of the analysed thread that the thread's own recorded wakeup ended.
// The expansion does not walk every such wait, and the thread's ready / D /
// IO time inside the ones it did not walk is still its own time inside a wait
// of the analysed thread that its wakeup ended — a cause under the one-source
// rule (skill.TraceWakeupDependencyCreditTeaching), not a remainder "outside
// the dependency windows".
//
// Mechanism: for a seat whose segment inventory reproduces its account and
// whose overlap with the chain windows equals the census-stamped anchored
// share, the inventory outside the chain windows is partitioned against the
// thread's unwalked dependency windows (its direct windows minus its chain
// windows). The inside part is priced on the chain with the E2 credential
// (the wakeup that ended the latest priced wait, via the thread's own direct
// wakeup); the remainder is the time outside every dependency window. Every
// instant is counted once: the chain windows and the unwalked windows are
// disjoint by construction. Fail closed (no direct credential, no unwalked
// wait, an inventory that does not reproduce the account or the stamp):
// nothing is priced here and the seat keeps the chain windows' partition.

import "fmt"

// unwalkedDependencyWindows returns the parts of a thread's dependency windows
// (the analysed thread's waits its own wakeup ended) that lie outside the
// thread's chain windows (merged, sorted). Each part keeps the closing wakeup
// of the wait it belongs to.
func unwalkedDependencyWindows(windows []targetWaitDependencyWindow, walked []TimeWindow) []targetWaitDependencyWindow {
	if len(windows) == 0 {
		return nil
	}
	walkedFold := make([]foldInterval, 0, len(walked))
	for _, w := range walked {
		if w.EndTs > w.StartTs {
			walkedFold = append(walkedFold, foldInterval{start: w.StartTs, end: w.EndTs})
		}
	}
	walkedFold, _ = foldIntervalUnionWithDisjoint(walkedFold)
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

// rspaUnwalkedWaitShare is one inventory's priced part inside the thread's
// unwalked dependency windows.
type rspaUnwalkedWaitShare struct {
	split  dependencyWindowSplit
	anchor semanticHostEdgeAnchor
	// waits / waitMs: the distinct waits of the analysed thread the thread's
	// wakeup ended that the chain did not walk, and their unwalked length.
	waits  int
	waitMs float64
}

func (s rspaUnwalkedWaitShare) ms() float64 { return s.split.preMs }

// clause names those waits for every sentence that prices or excludes them.
func (s rspaUnwalkedWaitShare) clause() string {
	return fmt.Sprintf("the analysed thread's wait(s) that this thread's own wakeup ended and that the dependency chain did not walk (%d wait(s), %.3fms of waiting; the latest one holding part of this account was ended at %.6f)",
		s.waits, s.waitMs, s.split.lastWakeupTs)
}

// rspaUnwalkedWaitShareOf prices one seat inventory. walkedWindows are the
// thread's chain windows (the RSPA anchor set), fullMs the inventory's
// account and walkedMs the census-stamped share inside walkedWindows.
func rspaUnwalkedWaitShareOf(chain *ChainResult, thread ThreadRef, walkedWindows []TimeWindow, inventory []foldInterval, fullMs, walkedMs float64) (rspaUnwalkedWaitShare, bool) {
	if chain == nil || len(inventory) == 0 || fullMs <= rspaAnchorIdentityTolMs {
		return rspaUnwalkedWaitShare{}, false
	}
	anchor, ok := hostDirectCensusEdgeAnchor(chain, thread)
	if !ok || len(anchor.windows) == 0 {
		return rspaUnwalkedWaitShare{}, false
	}
	unwalked := unwalkedDependencyWindows(anchor.windows, walkedWindows)
	if len(unwalked) == 0 {
		return rspaUnwalkedWaitShare{}, false
	}
	merged, reproduces := rspaInventoryReproducesAccount(inventory, walkedWindows, fullMs, walkedMs)
	if !reproduces {
		return rspaUnwalkedWaitShare{}, false
	}
	walkedFold := make([]foldInterval, 0, len(walkedWindows))
	for _, w := range walkedWindows {
		walkedFold = append(walkedFold, foldInterval{start: w.StartTs, end: w.EndTs})
	}
	walkedFold, _ = foldIntervalUnionWithDisjoint(walkedFold)
	split := splitFoldIntervalsByDependencyWindows(foldIntervalSubtract(merged, walkedFold), unwalked)
	if split.preMs <= rspaAnchorIdentityTolMs {
		return rspaUnwalkedWaitShare{}, false
	}
	out := rspaUnwalkedWaitShare{split: split, anchor: anchor.pricedBy(split)}
	seen := map[float64]bool{}
	for _, w := range unwalked {
		out.waitMs += (w.endTs - w.startTs) * 1000
		if !seen[w.wakeupTs] {
			seen[w.wakeupTs] = true
			out.waits++
		}
	}
	return out, true
}

// rspaInventoryReproducesAccount is the belt every unwalked-wait pricing reads:
// the merged inventory reproduces the account it claims to be, and its overlap
// with the chain windows reproduces the census-stamped anchored share.
func rspaInventoryReproducesAccount(inventory []foldInterval, walkedWindows []TimeWindow, fullMs, walkedMs float64) ([]foldInterval, bool) {
	merged, _ := foldIntervalUnionWithDisjoint(inventory)
	lengthMs, okLength := foldIntervalsLengthMs(merged)
	if !okLength || !rspaWithinTol(lengthMs, fullMs) {
		return nil, false // the inventory does not reproduce the account
	}
	if !rspaWithinTol(rspaIntervalsOverlapMs(walkedWindows, merged), walkedMs) {
		return nil, false // the inventory disagrees with the census stamp
	}
	return merged, true
}

// rspaUnwalkedWaitCensusMsByPID is the pid-level unwalked share of one census
// (Σ over the pid's groups), read by the rows that carry no inventory of their
// own (pid-level satellites and churn twins). A pid with any group whose
// inventory cannot prove its share maps to -1 (unknown: its rows keep the
// chain windows' partition); a pid with no unwalked wait is absent.
func rspaUnwalkedWaitCensusMsByPID(chain *ChainResult, census map[string]ThreadDuration, walkedByPID map[int][]TimeWindow, dio bool) map[int]float64 {
	if chain == nil || len(census) == 0 || len(walkedByPID) == 0 {
		return nil
	}
	sums := map[int]float64{}
	broken := map[int]bool{}
	for _, td := range census {
		pid := td.Thread.PID
		walked, member := walkedByPID[pid]
		if pid <= 0 || pid == chain.Target.PID || !member || td.DurationMs <= 0 || broken[pid] {
			continue
		}
		inventory := td.runnableIntervals
		if dio {
			if td.dioIntervalsOverflow {
				broken[pid] = true
				continue
			}
			inventory = td.dioIntervals
		}
		if share, ok := rspaUnwalkedWaitShareOf(chain, td.Thread, walked, inventory, td.DurationMs, td.anchoredMs); ok {
			sums[pid] += share.ms()
			continue
		}
		if _, reproduces := rspaInventoryReproducesAccount(inventory, walked, td.DurationMs, td.anchoredMs); !reproduces {
			broken[pid] = true
		}
	}
	out := map[int]float64{}
	for pid, ms := range sums {
		if ms > rspaAnchorIdentityTolMs {
			out[pid] = ms
		}
	}
	for pid := range broken {
		out[pid] = -1
	}
	return out
}

// rspaUnwalkedDIOWaitShareOf prices a D/IO seat per channel: the combined
// share (its anchor and wait clause) and the D and IO parts, which must add up
// to it. A channel with no time has no part; every channel with time must
// carry an inventory that reproduces it.
func rspaUnwalkedDIOWaitShareOf(chain *ChainResult, item RootCauseRankItem, walkedWindows []TimeWindow, walkedD, walkedIO float64) (all, d, io rspaUnwalkedWaitShare, ok bool) {
	all, ok = rspaUnwalkedWaitShareOf(chain, item.Thread, walkedWindows, item.dioSegmentIntervals, item.DStateMs+item.IOWaitMs, walkedD+walkedIO)
	if !ok {
		return rspaUnwalkedWaitShare{}, rspaUnwalkedWaitShare{}, rspaUnwalkedWaitShare{}, false
	}
	channel := func(inventory []foldInterval, value, walked float64) (rspaUnwalkedWaitShare, bool) {
		if value <= rspaAnchorIdentityTolMs && len(inventory) == 0 {
			return rspaUnwalkedWaitShare{}, true
		}
		share, priced := rspaUnwalkedWaitShareOf(chain, item.Thread, walkedWindows, inventory, value, walked)
		if !priced {
			// A reproduced inventory with nothing inside the unwalked waits
			// is a zero part, not a failure.
			merged, _ := foldIntervalUnionWithDisjoint(inventory)
			length, valid := foldIntervalsLengthMs(merged)
			if !valid || !rspaWithinTol(length, value) {
				return rspaUnwalkedWaitShare{}, false
			}
		}
		return share, true
	}
	d, okD := channel(item.dioSegmentIntervalsD, item.DStateMs, walkedD)
	io, okIO := channel(item.dioSegmentIntervalsIO, item.IOWaitMs, walkedIO)
	if !okD || !okIO || !rspaWithinTol(d.ms()+io.ms(), all.ms()) {
		return rspaUnwalkedWaitShare{}, rspaUnwalkedWaitShare{}, rspaUnwalkedWaitShare{}, false
	}
	return all, d, io, true
}

// rspaUnwalkedWaitSeat mints the on-chain seat of the priced part from a copy
// of the seat whose inventory it was cut from: the E2 host-edge state basis,
// the closing wakeup of the latest priced wait, value channels and support =
// the inside pieces only. The copy carries no partition trio (it is neither
// half of the chain windows' partition) and no census stamp (the stamp is the
// full account's).
func rspaUnwalkedWaitSeat(from RootCauseRankItem, share rspaUnwalkedWaitShare, runnableMs, dMs, ioMs float64,
	runnableSegs, dSegs, ioSegs []foldInterval, summary string) RootCauseRankItem {
	seat := cloneRootCauseMeasurementSources(from)
	seat.Rank, seat.BackgroundRank, seat.Tier = 0, 0, ""
	seat.ChainAnchorRemainderSeat = false
	seat.ChainAnchoredMs, seat.ChainAnchorFullMs = 0, 0
	seat.ChainAnchorOwnershipDivergent = false
	seat.ChainAnchorChainLaneMs, seat.ChainAnchorCensusMs = 0, 0
	seat.ledgerAnchorStamped = false
	seat.ledgerAnchoredRunnableMs, seat.ledgerAnchoredDMs, seat.ledgerAnchoredIOMs = 0, 0, 0
	seat.inversionAccountRepresentedBy, seat.inversionAccountRepresents = "", ""
	convertStateSeatToEdgeAnchored(&seat, share.anchor, runnableMs+dMs+ioMs, runnableMs, dMs, ioMs,
		runnableSegs, dSegs, ioSegs, share.split.stateEdgeSegmentSplit, summary)
	return seat
}

// rspaUnwalkedWaitSeatSummary — the priced part's engine-side account.
func rspaUnwalkedWaitSeatSummary(thread ThreadRef, family string, inside, walked, remainder, full float64, clause string) string {
	return fmt.Sprintf("%s %s %.3fms inside %s; full-window account %.3fms = this share + %.3fms inside the dependency windows the chain walked for this thread + %.3fms outside every dependency window of this thread — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, inside, clause, full, walked, remainder)
}

// rspaRemainderSummaryUnwalked — the remainder's account when part of the
// thread's time outside its chain windows lies inside unwalked waits its
// wakeup ended (that part is priced as its own seat).
func rspaRemainderSummaryUnwalked(thread ThreadRef, family string, remainder, walked, inside, full float64, clause string) string {
	return fmt.Sprintf("%s %s remainder %.3fms outside its wakeup-dependency windows — outside the dependency windows the chain walked for this thread and outside %s; the analysed thread was not waiting for this thread there (no chain credential for these segments); full-window account %.3fms = %.3fms anchored inside the walked dependency windows %s + %.3fms inside those unwalked waits (priced as this thread's own on-chain seat) + this remainder — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, remainder, clause, full, walked, rspaSummaryOwnedByChainSeat, inside)
}

// rspaUnwalkedWaitDivergenceClause — appended to the case-A' double-account
// remainder sentence: its anchored share includes the unwalked part, priced
// as its own seat, which the chain seat's own account never held.
func rspaUnwalkedWaitDivergenceClause(inside float64, clause string) string {
	return fmt.Sprintf("; %.3fms of this seat's anchored share lies inside %s and is priced as this thread's own on-chain seat", inside, clause)
}

// rspaInversionRepresentedSummaryUnwalked — the R-377-4 detail sentence when
// part of the ready account lies inside unwalked waits the thread's wakeup
// ended.
func rspaInversionRepresentedSummaryUnwalked(walked, inside, full float64, clause string) string {
	return fmt.Sprintf("; represented by this thread's chain seat: %.3fms of this %.3fms ready-time account lies inside the dependency windows the chain walked for this thread, which that chain seat already publishes, %.3fms lies inside %s (priced as this thread's own on-chain ready-time seat), and %.3fms lies outside every dependency window of this thread (published as the thread's adjacent ready-time remainder) — this whole-account row (its same-CPU lower-priority overlap is measured over the whole ready account and is never split) rides as that seat's lossless detail, not as a second object for the same thread",
		walked, full, inside, clause, rspaClampNonNegative(full-walked-inside))
}

// rspaUnwalkedWaitsWord names the unwalked waits on a pid-level row, which
// carries no per-wait inventory of its own.
const rspaUnwalkedWaitsWord = "the analysed thread's wait(s) that this thread's own wakeup ended and that the dependency chain did not walk"

// rspaInsideWaitRunnableSeatWindowsByPID collects, per pid, the support of the
// on-chain ready-time seats priced inside the analysed thread's waits the
// thread's own wakeup ended (the host-edge state basis): the representation
// witness a satellite's unwalked share is checked against.
func rspaInsideWaitRunnableSeatWindowsByPID(items []RootCauseRankItem) map[int][]TimeWindow {
	var out map[int][]TimeWindow
	for i := range items {
		item := &items[i]
		if item.Thread.PID <= 0 || item.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState ||
			item.DominantState != string(StateRunnable) || !rootCauseItemIsOnChain(*item) {
			continue
		}
		for _, iv := range item.runnableIntervals {
			if iv.end > iv.start {
				if out == nil {
					out = map[int][]TimeWindow{}
				}
				out[item.Thread.PID] = append(out[item.Thread.PID], TimeWindow{StartTs: iv.start, EndTs: iv.end})
			}
		}
	}
	for pid, windows := range out {
		out[pid] = mergeAnchorTimeWindows(windows)
	}
	return out
}
