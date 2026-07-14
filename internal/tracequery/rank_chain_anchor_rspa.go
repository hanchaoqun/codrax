package tracequery

// rank_chain_anchor_rspa.go — RSPA on-chain seat-value re-anchoring
// (§29.61.10 filing; user rulings §29.61.10a/b/c, 2026-07-13/14).
//
// Ruling chain: 10a — chain seats anchor on wakeup edges; pure scheduling-
// pressure semantics demote to ◇ adjacent. 10b — typed causal edges are the
// only chain credential across ALL state families. 10c — the criterion is the
// EVIDENCE FACE, not the category face: a kernel-recorded typed directed
// dependency edge carrying direct chain-dependency proof ranks on-chain; pure
// time overlap is always adjacent.
//
// Mechanism (权属重排, no new algorithm — the A-form value already exists as
// the wakeup_chain lane's per-jump-window clipped accounting, witness-proven
// µs-identical):
//   - The typed anchor window set of a chain thread is the UNION of the
//     chain's depth>0 node windows for that pid — each window
//     [sleepStart, wakeupTs] is CLOSED by a recorded sched_wakeup edge, so
//     every same-thread segment inside it is direct dependency evidence and
//     every segment outside it carries zero chain credential.
//   - computeOffCPUStats accumulates the per-segment anchored overlap at its
//     single ledger close site (same clamped endpoints as the census sums →
//     全窗账 = anchored + remainder is an EXACT same-segment-set bipartition).
//   - reanchorOnChainStateSeats then rewrites each migrable window_stats
//     state seat into the ◇ adjacent REMAINDER seat (value = full − anchored)
//     while the chain-lane seat keeps owning the anchored portion it already
//     publishes. A fully-anchored seat (remainder ≈ 0) is absorbed into its
//     chain seat by the upgraded cross-type recon arm (§6.3 锚定分解吸收).
//
// Fail-open boundary (无凭证形禁猜): no chain, no jump windows for the pid,
// no census basis (legacy/direct-literal WindowStats), a MAX-fallback fold,
// a priority-inversion-rewritten row, or a failed µs identity between the
// ledger-anchored sum and the chain lane's own per-state value — all keep the
// legacy full-window on-chain publication byte-identically.
//
// Exemptions (negative forms, §29.61.10 处置矩阵):
//   - self-causality: the analysis TARGET's own seats (SELF-SEM/SELF-ALL and
//     the plain target window seats) are fully anchored by definition — the
//     target's every runnable/D ms directly delays the target, no edge needed;
//   - lock blocking_span (typed resolved waiter→holder pair), binder_wait
//     (P9 typed transaction pair), semantic span families (intersection value
//     already implemented), running (supply-fold deficit computed inside the
//     jump window), sleep (eff=0 structural) — already credential-anchored,
//     zero migration.

import (
	"fmt"
	"sort"
	"strings"
)

// rspaAnchorIdentityTolMs is the µs-scale identity tolerance (§6.3 精确臂):
// the ledger-anchored sum and the chain lane's published per-state value are
// two deterministic computations over the same segments — they agree to float
// dust or not at all. 0.001ms (1µs) absorbs summation-order dust while any
// real segmentation divergence trips the fail-open lane.
const rspaAnchorIdentityTolMs = 0.001

// anchoredDIOWakeupCap bounds the wakeup-closure record inventory. Overflow
// direction is honest: a dropped record can only leave an io row on the ◇
// lane (宁漏勿猜), never promote one.
const anchoredDIOWakeupCap = 1024

// rspaIOCompletionClosureTolS is the completion→wakeup pairing slack in
// SECONDS (0.0005s = 0.5ms — a deliberately GENEROUS bound; the sched_wakeup
// that ends the blocked D segment is emitted by the completion handler within
// the same code path as block_rq_complete, typically µs after it, and the
// bound only ever DEMOTES on a miss, never promotes). Same boundary-tolerance
// nature as the chain's own findWakeup slack.
const rspaIOCompletionClosureTolS = 0.0005

// anchoredDIOWakeupRecord is one wakeup-closed anchored D/IO segment end:
// waker identity + wakeup timestamp (the typed directed edge the M-IO
// completion-closure credential reads).
type anchoredDIOWakeupRecord struct {
	wakerPID int
	ts       float64
}

// chainAnchorWindowsByPID collects the typed wakeup-dependency jump windows
// per chain thread: the UNION (merged, sorted) of the chain's depth>0 node
// windows for each pid. Depth-0 nodes are the target's own branch windows —
// the target is excluded here (self-causality: fully anchored by definition,
// 豁免重锚). Returns nil when the chain carries no such window.
func chainAnchorWindowsByPID(chain ChainResult) map[int][]TimeWindow {
	var out map[int][]TimeWindow
	for _, node := range chain.Nodes {
		if node.Depth <= 0 || node.Thread.PID <= 0 || node.Thread.PID == chain.Target.PID {
			continue
		}
		if node.Window.EndTs <= node.Window.StartTs {
			continue
		}
		if out == nil {
			out = map[int][]TimeWindow{}
		}
		out[node.Thread.PID] = append(out[node.Thread.PID], node.Window)
	}
	for pid, windows := range out {
		out[pid] = mergeAnchorTimeWindows(windows)
	}
	return out
}

// mergeAnchorTimeWindows sorts and unions overlapping/adjacent windows so the
// per-segment overlap sum can never double-count one physical instant even
// when two branches explored overlapping dependency windows of one thread.
func mergeAnchorTimeWindows(windows []TimeWindow) []TimeWindow {
	if len(windows) < 2 {
		return windows
	}
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].StartTs != windows[j].StartTs {
			return windows[i].StartTs < windows[j].StartTs
		}
		return windows[i].EndTs < windows[j].EndTs
	})
	merged := windows[:1]
	for _, w := range windows[1:] {
		last := &merged[len(merged)-1]
		if w.StartTs <= last.EndTs {
			if w.EndTs > last.EndTs {
				last.EndTs = w.EndTs
			}
			continue
		}
		merged = append(merged, w)
	}
	return merged
}

// anchorWindowsOverlapMs sums the overlap of [startTs, endTs] with a merged
// window union (windows pairwise disjoint ⇒ the sum is exact, never a
// double count).
func anchorWindowsOverlapMs(windows []TimeWindow, startTs, endTs float64) float64 {
	if endTs <= startTs || len(windows) == 0 {
		return 0
	}
	total := 0.0
	for _, w := range windows {
		if w.StartTs >= endTs {
			break
		}
		total += windowOverlapMs(startTs, endTs, w.StartTs, w.EndTs)
	}
	return total
}

// chainAnchoredStateSums is the chain lane's own per-thread per-state anchored
// account: Σ over the pid's depth>0 causal impacts of the per-state values the
// chain computed INSIDE its jump windows. The §6.3 identity gate compares the
// ledger-anchored sum against THIS (µs tolerance) — when the same thread's
// jump windows overlapped across occurrences the two sums diverge and the
// migration fails open (the chain Σ would double-count; the union does not).
type chainAnchoredStateSums struct {
	runnableMs float64
	dioMs      float64
}

func chainAnchoredStateMsByPID(chain ChainResult) map[int]chainAnchoredStateSums {
	var out map[int]chainAnchoredStateSums
	for _, impact := range chain.CausalImpacts {
		if impact.Thread.PID <= 0 || impact.ChainDepth <= 0 {
			continue
		}
		if out == nil {
			out = map[int]chainAnchoredStateSums{}
		}
		sums := out[impact.Thread.PID]
		sums.runnableMs += impact.RunnableMs
		sums.dioMs += impact.DStateMs + impact.IOWaitMs
		out[impact.Thread.PID] = sums
	}
	return out
}

// censusAnchorSums is the full-window census account of one pid on one state
// family: Σ durMs (full) and Σ anchoredMs (anchored) over the pid's census
// groups — the same segment set the seats minted from.
type censusAnchorSums struct {
	fullMs     float64
	anchoredMs float64
}

func censusAnchorSumsByPID(census map[string]ThreadDuration) map[int]censusAnchorSums {
	var out map[int]censusAnchorSums
	for _, td := range census {
		if td.Thread.PID <= 0 || td.DurationMs <= 0 {
			continue
		}
		if out == nil {
			out = map[int]censusAnchorSums{}
		}
		sums := out[td.Thread.PID]
		sums.fullMs += td.DurationMs
		sums.anchoredMs += td.anchoredMs
		out[td.Thread.PID] = sums
	}
	return out
}

func addCensusAnchorSums(dst map[int]censusAnchorSums, src map[int]censusAnchorSums) map[int]censusAnchorSums {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[int]censusAnchorSums{}
	}
	for pid, sums := range src {
		merged := dst[pid]
		merged.fullMs += sums.fullMs
		merged.anchoredMs += sums.anchoredMs
		dst[pid] = merged
	}
	return dst
}

// rspaFamilyDecision is the per-(pid, state family) migration verdict — ONE
// deterministic decision per thread and family, applied to every row of that
// family (the formal window seat and its scheduler_latency / low_frequency /
// fragmented satellites), so complementary accounts can never diverge.
type rspaFamilyDecision struct {
	migrate    bool
	anchoredMs float64
	fullMs     float64
}

func rspaWithinTol(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= rspaAnchorIdentityTolMs
}

// buildRSPAFamilyDecisions computes the runnable and D-IO per-pid migration
// decisions. Gates (all precise typed signals):
//  1. the stats sweep ran WITH anchor data (stats.chainAnchorsByPID != nil)
//     and the pid has ≥1 typed jump window;
//  2. the census basis exists (nil census = legacy fixture → fail open);
//  3. anchored ≤ full (+tol);
//  4. §6.3 µs identity: ledger-anchored Σ == the chain lane's own per-state
//     value for the pid (both deterministic; divergence = overlapping jump
//     windows or segmentation drift → fail open, 禁猜);
//  5. 件6 (修复轮, 2026-07-14): the off-CPU ordered-stream premise holds
//     (stats.offCPUProducerDisjoint) — a clock-regressed trace folds seats
//     to the MAX fallback and clips nothing, so NO decision may mint (the
//     mint-time chain D-IO seat suppression reads the same decisions; a
//     regressed trace suppressing the chain seat while window seats stayed
//     unclipped would silently drop the chain value from the board).
func buildRSPAFamilyDecisions(chain ChainResult, stats WindowStats) (runnable, dio map[int]rspaFamilyDecision) {
	if stats.chainAnchorsByPID == nil || !stats.offCPUProducerDisjoint {
		return nil, nil
	}
	chainSums := chainAnchoredStateMsByPID(chain)
	if len(chainSums) == 0 {
		return nil, nil
	}
	runnableCensus := censusAnchorSumsByPID(stats.runnableCensus)
	dioCensus := addCensusAnchorSums(censusAnchorSumsByPID(stats.dstateCensus), censusAnchorSumsByPID(stats.iowaitCensus))
	decide := func(census map[int]censusAnchorSums, chainMs func(chainAnchoredStateSums) float64) map[int]rspaFamilyDecision {
		var out map[int]rspaFamilyDecision
		for pid, sums := range census {
			if pid == chain.Target.PID {
				// Self-causality exemption: the target's own account is fully
				// anchored by definition (§29.61.1/.2) — never re-anchored.
				continue
			}
			windows := stats.chainAnchorsByPID[pid]
			if len(windows) == 0 {
				continue
			}
			chainStates, onChain := chainSums[pid]
			if !onChain {
				continue
			}
			if sums.anchoredMs > sums.fullMs+rspaAnchorIdentityTolMs {
				continue
			}
			if !rspaWithinTol(sums.anchoredMs, chainMs(chainStates)) {
				continue
			}
			if out == nil {
				out = map[int]rspaFamilyDecision{}
			}
			out[pid] = rspaFamilyDecision{migrate: true, anchoredMs: sums.anchoredMs, fullMs: sums.fullMs}
		}
		return out
	}
	runnable = decide(runnableCensus, func(s chainAnchoredStateSums) float64 { return s.runnableMs })
	dio = decide(dioCensus, func(s chainAnchoredStateSums) float64 { return s.dioMs })
	return runnable, dio
}

func rspaClampNonNegative(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

// rspaRowIsSelfExempt: the analysis target's own rows and every self-basis
// row keep their lane untouched (self-causality = full anchoring).
func rspaRowIsSelfExempt(item RootCauseRankItem, target ThreadRef) bool {
	if item.SubjectIsAnalysisTarget {
		return true
	}
	if rootCauseOnChainBasisIsSelf(item.OnChainBasis) {
		return true
	}
	return target.PID > 0 && item.Thread.PID == target.PID
}

// rspaRewriteSeatToRemainder converts one on-chain window seat into the ◇
// adjacent remainder seat: value channels = full − anchored, lane = adjacent,
// typed decomposition fields stamped, Score re-derived from the published
// value (the §7.30 S1 discipline: published faces never drift from sort keys).
func rspaRewriteSeatToRemainder(item *RootCauseRankItem, anchoredMs, fullMs float64, remainderRunnable, remainderD, remainderIO float64, summary string) {
	remainder := remainderRunnable + remainderD + remainderIO
	item.ChainAnchoredMs = anchoredMs
	item.ChainAnchorFullMs = fullMs
	item.ChainAnchorRemainderSeat = true
	item.RunnableMs = remainderRunnable
	item.DStateMs = remainderD
	item.IOWaitMs = remainderIO
	item.ImpactMs = remainder
	item.ProjectedImpactMs = remainder
	item.CumulativeImpactMs = remainder
	item.EffectiveImpactMs = remainder
	item.Score = remainder * item.Confidence * rootCauseItemScoreWeight(*item)
	item.Causality = "adjacent_to_wakeup_chain"
	item.ChainRelevance = "adjacent"
	if summary != "" {
		item.Summary = summary
	}
}

// rspaRemainderSummary renders the honest engine-side (English) remainder
// account. LLM-facing: states the decomposition facts only, no internal
// batch vocabulary.
func rspaRemainderSummary(thread ThreadRef, family string, remainder, anchored, full float64) string {
	return fmt.Sprintf("%s %s remainder %.3fms outside its wakeup-dependency windows (no chain credential for these segments); full-window account %.3fms = %.3fms anchored inside typed dependency windows (owned by the chain seat) + this remainder — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, remainder, anchored, full)
}

// rspaRowIntervalAnchoredMs computes the anchored overlap of a row that
// carries its OWN typed interval inventory (scheduler_latency /
// low_frequency satellites). Family-folded rows contribute their member
// intervals; a singleton contributes its typed [StartTs, EndTs]. Returns
// ok=false (fail open) when the interval inventory does not reproduce the
// row's own runnable scalar (tolerance) — a hull would misstate the account.
func rspaRowIntervalAnchoredMs(windows []TimeWindow, item RootCauseRankItem) (anchored float64, ok bool) {
	intervals := item.familyMemberIntervals
	if len(intervals) == 0 {
		if item.EndTs <= item.StartTs {
			return 0, false
		}
		intervals = []foldInterval{{start: item.StartTs, end: item.EndTs}}
	}
	lengthMs := 0.0
	for _, iv := range intervals {
		if iv.end <= iv.start {
			return 0, false
		}
		lengthMs += (iv.end - iv.start) * 1000
		anchored += anchorWindowsOverlapMs(windows, iv.start, iv.end)
	}
	if !rspaWithinTol(lengthMs, item.RunnableMs) {
		return 0, false
	}
	return anchored, true
}

// rspaChainSeatPresence records, per pid, whether the CHAIN LANE itself
// publishes a rank seat for the runnable / D-IO family — the case fork of the
// migration pass:
//   - chain seat present (case A): the anchored portion already lives on that
//     seat; the window seat becomes the ◇ remainder (remainder ≈ 0 → marked
//     and absorbed into the chain seat by the upgraded recon arm);
//   - chain seat absent (case B — e.g. a sleep-dominant intermediate waker
//     whose impact is rank-suppressed): the window seat ITSELF becomes the ⛓
//     anchored seat (value clipped to the anchored portion) and the remainder
//     mints a NEW ◇ seat — the anchored evidence must never vanish and the
//     remainder must never keep riding the chain tier (零静默消失 both ways).
type rspaChainSeatPresence struct {
	runnable bool
	dio      bool
}

func rspaChainSeatPresenceByPID(items []RootCauseRankItem) map[int]rspaChainSeatPresence {
	var out map[int]rspaChainSeatPresence
	for i := range items {
		if items[i].Thread.PID <= 0 || !strings.HasPrefix(strings.TrimSpace(items[i].Source), "wakeup_chain") {
			continue
		}
		var presence rspaChainSeatPresence
		switch items[i].DominantState {
		case string(StateRunnable):
			presence.runnable = true
		case string(StateDSleep), string(StateIOWait):
			presence.dio = true
		default:
			continue
		}
		if out == nil {
			out = map[int]rspaChainSeatPresence{}
		}
		merged := out[items[i].Thread.PID]
		merged.runnable = merged.runnable || presence.runnable
		merged.dio = merged.dio || presence.dio
		out[items[i].Thread.PID] = merged
	}
	return out
}

// rspaCloneAsRemainderSeat clones a window seat that stays ⛓ (case B split)
// into its ◇ remainder twin. The clone shares the physical evidence identity
// (lines / intervals / roster); its published value channels carry only the
// remainder and the typed decomposition fields disclose the split.
func rspaCloneAsRemainderSeat(seat RootCauseRankItem, anchoredMs, fullMs, remainderRunnable, remainderD, remainderIO float64, summary string) RootCauseRankItem {
	clone := seat
	clone.Rank = 0
	clone.BackgroundRank = 0
	clone.Tier = ""
	rspaRewriteSeatToRemainder(&clone, anchoredMs, fullMs, remainderRunnable, remainderD, remainderIO, summary)
	return clone
}

// rspaClipSeatToAnchored rewrites a case-B window seat into the ⛓ anchored
// seat: the published value channels carry ONLY the credential-anchored
// portion; the typed decomposition fields disclose the full/anchored split
// (ChainAnchorRemainderSeat stays false — this row publishes the ⛓ side).
func rspaClipSeatToAnchored(item *RootCauseRankItem, anchoredMs, fullMs float64, anchoredRunnable, anchoredD, anchoredIO float64, summary string) {
	item.ChainAnchoredMs = anchoredMs
	item.ChainAnchorFullMs = fullMs
	item.RunnableMs = anchoredRunnable
	item.DStateMs = anchoredD
	item.IOWaitMs = anchoredIO
	item.ImpactMs = anchoredMs
	item.ProjectedImpactMs = anchoredMs
	item.CumulativeImpactMs = anchoredMs
	item.EffectiveImpactMs = anchoredMs
	item.Score = anchoredMs * item.Confidence * rootCauseItemScoreWeight(*item)
	if summary != "" {
		item.Summary = summary
	}
}

// rspaAnchoredSummary renders the ⛓ clipped seat's engine-side account.
func rspaAnchoredSummary(thread ThreadRef, family string, anchored, full float64) string {
	return fmt.Sprintf("%s %s %.3fms anchored inside its typed wakeup-dependency windows (chain credential); full-window account %.3fms = this anchored portion + %.3fms remainder outside the dependency windows (published as a separate adjacent seat) — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, anchored, full, full-anchored)
}

// reanchorOnChainStateSeats is the RSPA migration pass. Runs in BOTH rank
// passes (build + scheduler enrich), after the family fold and before the
// cross-type recon / sort — idempotent: migrated rows carry the typed
// decomposition fields and are never re-processed. Returns the item slice
// (case-B splits append the ◇ remainder twin).
func reanchorOnChainStateSeats(chain ChainResult, stats WindowStats, items []RootCauseRankItem) []RootCauseRankItem {
	if len(items) == 0 || stats.chainAnchorsByPID == nil {
		return items
	}
	runnableDecisions, dioDecisions := buildRSPAFamilyDecisions(chain, stats)
	if len(runnableDecisions) == 0 && len(dioDecisions) == 0 {
		return items
	}
	chainSeats := rspaChainSeatPresenceByPID(items)
	// RSPA M-D one-seat closure context transfer: the suppressed chain-lane
	// D-IO rank row carried physical-extent (actual_*) and target-blocked
	// disclosure the partition seats lack. When a pid holds EXACTLY ONE
	// window D-IO seat, that seat inherits the suppressed row's typed chain
	// context (strongest dependency impact — never overwriting a present
	// value). Multi-partition pids transfer nothing (a per-seat copy would
	// multiply the target attribution; the lossless CausalImpacts view keeps
	// the full record — absence never guesses a split).
	windowDIOSeatCount := map[int]int{}
	for i := range items {
		if items[i].Thread.PID <= 0 || items[i].AbsorbedByRankFamily {
			continue
		}
		switch strings.TrimSpace(items[i].Type) {
		case "d_state_or_io_wait", "io_wait":
			if items[i].Source == "window_stats" || items[i].Source == "window_stats.io_wait_top" {
				windowDIOSeatCount[items[i].Thread.PID]++
			}
		}
	}
	strongestImpactByPID := strongestWakeupDependencyImpactByThread(chain)
	transferChainDIOContext := func(item *RootCauseRankItem) {
		if windowDIOSeatCount[item.Thread.PID] != 1 {
			return
		}
		impact, ok := strongestImpactByPID[item.Thread.PID]
		if !ok {
			return
		}
		if item.ActualEndTs <= item.ActualStartTs && impact.ActualWindow.EndTs > impact.ActualWindow.StartTs {
			item.ActualStartTs = impact.ActualWindow.StartTs
			item.ActualEndTs = impact.ActualWindow.EndTs
		}
		if item.ActualImpactMs == 0 && impact.ActualImpactMs > 0 {
			item.ActualImpactMs = impact.ActualImpactMs
		}
		if item.ActualTotalMs == 0 && impact.ActualTotalMs > 0 {
			item.ActualTotalMs = impact.ActualTotalMs
		}
		if item.TargetImpactMs == 0 && impact.TargetBlockedMs > 0 {
			item.TargetImpactMs = impact.TargetBlockedMs
		}
		if item.ChainDepth == 0 && impact.ChainDepth > 0 {
			item.ChainDepth = impact.ChainDepth
			item.ChainBranch = impact.ChainBranch
		}
	}
	var appended []RootCauseRankItem
	for i := range items {
		item := &items[i]
		if item.ChainAnchorRemainderSeat || item.ChainAnchorFullMs > 0 || item.AbsorbedByRankFamily {
			continue
		}
		if item.Thread.PID <= 0 || !rootCauseItemIsOnChain(*item) {
			continue
		}
		if rspaRowIsSelfExempt(*item, chain.Target) {
			continue
		}
		switch strings.TrimSpace(item.Type) {
		case "runnable_wait":
			decision, ok := runnableDecisions[item.Thread.PID]
			if !ok || !decision.migrate || item.Source != "window_stats" {
				continue
			}
			// The seat must BE the census full-window account (a MAX-fallback
			// fold or any rewritten value fails this precise equality → open).
			if !rspaWithinTol(item.RunnableMs, decision.fullMs) {
				continue
			}
			anchored := decision.anchoredMs
			full := decision.fullMs
			remainder := rspaClampNonNegative(full - anchored)
			hasChainSeat := chainSeats[item.Thread.PID].runnable
			switch {
			case hasChainSeat:
				// Case A: the chain seat owns the anchored portion; this seat
				// publishes the remainder (≈0 → absorbed by the recon arm).
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					rspaRemainderSummary(item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full))
			case remainder <= rspaAnchorIdentityTolMs:
				// Case B, fully anchored: the seat is entirely credentialed —
				// keep the legacy on-chain publication byte-identically.
			case anchored <= rspaAnchorIdentityTolMs:
				// Case B, zero credential: the whole account is remainder.
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					rspaRemainderSummary(item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full))
			default:
				// Case B split: this seat becomes the ⛓ anchored seat; the
				// remainder twin mints beside it on the ◇ lane.
				appended = append(appended, rspaCloneAsRemainderSeat(*item, anchored, full, remainder, 0, 0,
					rspaRemainderSummary(item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full)))
				rspaClipSeatToAnchored(item, anchored, full, anchored, 0, 0,
					rspaAnchoredSummary(item.Thread, "runnable", anchored, full))
			}
		case "scheduler_latency", "low_frequency":
			// Satellite diagnostic rows of the runnable family: the anchored
			// share is owned by the formal lane (window/chain seat) — a fully
			// anchored satellite keeps its lane untouched; any unanchored
			// share demotes the row to the ◇ remainder form (no ⛓ twin —
			// minting one would double-count the formal seat's account).
			decision, ok := runnableDecisions[item.Thread.PID]
			if !ok || !decision.migrate {
				continue
			}
			if item.DominantState != string(StateRunnable) || item.RunnableMs <= 0 {
				continue
			}
			anchored, intervalOK := rspaRowIntervalAnchoredMs(stats.chainAnchorsByPID[item.Thread.PID], *item)
			if !intervalOK {
				continue
			}
			full := item.RunnableMs
			if anchored > full+rspaAnchorIdentityTolMs {
				continue
			}
			remainder := rspaClampNonNegative(full - anchored)
			if remainder <= rspaAnchorIdentityTolMs {
				continue
			}
			rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
				rspaRemainderSummary(item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full))
		case "fragmented_runnable_wait":
			decision, ok := runnableDecisions[item.Thread.PID]
			if !ok || !decision.migrate || item.Source != "window_stats.state_churn" {
				continue
			}
			full := item.RunnableMs
			if full <= 0 {
				continue
			}
			anchored := decision.anchoredMs
			if anchored > full {
				anchored = full
			}
			remainder := rspaClampNonNegative(full - anchored)
			if remainder <= rspaAnchorIdentityTolMs && !chainSeats[item.Thread.PID].runnable {
				continue
			}
			rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
				rspaRemainderSummary(item.Thread, "fragmented runnable (scheduling-pressure candidate)", remainder, anchored, full))
		case "d_state_or_io_wait", "io_wait":
			decision, ok := dioDecisions[item.Thread.PID]
			if !ok || !decision.migrate || !item.ledgerAnchorStamped {
				continue
			}
			if item.Source != "window_stats" && item.Source != "window_stats.io_wait_top" {
				continue
			}
			anchoredD, anchoredIO := item.ledgerAnchoredDMs, item.ledgerAnchoredIOMs
			if anchoredD > item.DStateMs+rspaAnchorIdentityTolMs || anchoredIO > item.IOWaitMs+rspaAnchorIdentityTolMs {
				continue
			}
			full := item.DStateMs + item.IOWaitMs
			anchored := anchoredD + anchoredIO
			remainderD := rspaClampNonNegative(item.DStateMs - anchoredD)
			remainderIO := rspaClampNonNegative(item.IOWaitMs - anchoredIO)
			remainder := remainderD + remainderIO
			hasChainSeat := chainSeats[item.Thread.PID].dio
			switch {
			case hasChainSeat:
				rspaRewriteSeatToRemainder(item, anchored, full, 0, remainderD, remainderIO,
					rspaRemainderSummary(item.Thread, "D/IO blocking", remainder, anchored, full))
			case remainder <= rspaAnchorIdentityTolMs:
				// Case B, fully anchored: the seat keeps its full publication
				// and (single-seat pids) inherits the suppressed chain row's
				// typed context — it IS the pid's one D-IO seat now.
				transferChainDIOContext(item)
			case anchored <= rspaAnchorIdentityTolMs:
				rspaRewriteSeatToRemainder(item, anchored, full, 0, remainderD, remainderIO,
					rspaRemainderSummary(item.Thread, "D/IO blocking", remainder, anchored, full))
			default:
				appended = append(appended, rspaCloneAsRemainderSeat(*item, anchored, full, 0, remainderD, remainderIO,
					rspaRemainderSummary(item.Thread, "D/IO blocking", remainder, anchored, full)))
				rspaClipSeatToAnchored(item, anchored, full, 0, anchoredD, anchoredIO,
					rspaAnchoredSummary(item.Thread, "D/IO blocking", anchored, full))
				transferChainDIOContext(item)
			}
		case "fragmented_d_state_or_io_wait":
			decision, ok := dioDecisions[item.Thread.PID]
			if !ok || !decision.migrate || item.Source != "window_stats.state_churn" {
				continue
			}
			full := item.DStateMs + item.IOWaitMs
			if full <= 0 {
				continue
			}
			anchored := decision.anchoredMs
			if anchored > full {
				anchored = full
			}
			remainder := rspaClampNonNegative(full - anchored)
			if remainder <= rspaAnchorIdentityTolMs && !chainSeats[item.Thread.PID].dio {
				continue
			}
			// The churn twin has no per-lane anchored split; scale both lanes
			// by the same remainder share so the family scalar stays exact.
			share := 0.0
			if full > 0 {
				share = remainder / full
			}
			rspaRewriteSeatToRemainder(item, anchored, full, 0, item.DStateMs*share, item.IOWaitMs*share,
				rspaRemainderSummary(item.Thread, "fragmented D/IO blocking", remainder, anchored, full))
		}
	}
	return append(items, appended...)
}

// stampResourceClosureEvaluation marks every resource-attribution row with
// the typed "closure credential was computable" bit when the stats sweep ran
// with the RSPA anchor basis. The enrich lane decision requires the
// completion-closure credential ONLY on evaluated io_latency rows — an
// anchor-less build (legacy fixtures, chainless queries) keeps the pre-RSPA
// overlap behavior byte-identically. Other resource projections keep their
// lane (host-wait/host-work credential form, see the enrich arm comment) but
// carry the bit so the display can distinguish "evaluated" from "legacy".
func stampResourceClosureEvaluation(stats WindowStats, items []RootCauseRankItem) {
	if stats.chainAnchorsByPID == nil {
		return
	}
	for i := range items {
		if rootCauseTypeIsResourceAttribution(items[i].Type) {
			items[i].resourceClosureEvaluated = true
		}
	}
}

// resourceCompletionClosureProven is the M-IO per-IO credential check: some
// wakeup-closed ANCHORED D/IO segment end of a chain thread was performed by
// this IO's completion thread within the IO's lifetime (+µs handler slack).
func resourceCompletionClosureProven(stats WindowStats, completePID int, issueTs, completeTs float64) bool {
	if completePID <= 0 || completeTs <= issueTs {
		return false
	}
	for _, record := range stats.anchoredDIOWakeups {
		if record.wakerPID != completePID {
			continue
		}
		if record.ts >= issueTs && record.ts <= completeTs+rspaIOCompletionClosureTolS {
			return true
		}
	}
	return false
}
