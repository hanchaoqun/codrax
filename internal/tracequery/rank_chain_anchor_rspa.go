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
// no census basis (legacy/direct-literal WindowStats), or a MAX-fallback fold
// — all keep the legacy full-window on-chain publication byte-identically.
//
// EVOLUTION RECORD (RNB-1, §29.88 R2/R4 user rulings, 2026-07-14): two former
// fail-open lanes are RETIRED — they were the customer runnable.txt W1/W2
// escape (deep-unresolved on-chain seats riding FULL window values, E8 26.392
// full vs chain-proven 8.606):
//   1. the §6.3 µs-identity gate no longer blocks the migration — the census
//      bipartition (full = anchored + remainder) is self-sufficiently exact at
//      its single ledger close site, so identity failure only DEMOTES the
//      case-A ownership claim: the window seat still migrates to ◇ and the
//      relation downgrades to the double-account disclosure (both Σs + delta
//      ride typed fields — the armed-tick face, §29.84 件① 同构). Case B
//      (no chain seat) bisects from the census pair exempt from the gate.
//   2. the per-seat value == pid-census-full identity is replaced by the
//      census-GROUP ledger stamp (T1 root fix): each seat reconciles against
//      ITS OWN group account, so a mixed-lane split fold (dust group re-laned
//      adjacent by the enrich overlap arm) no longer fails the whole pid open.
// Additionally per R4 (边=凭证,边前=有效,边后=解除 — the exclusive rule over
// ALL state families): the priority-inversion-rewritten window seat and the
// cpu_affinity_or_cpuset satellite gained lane arms — indivisible accounts
// that cannot show a typed-edge anchored share ride ◇ with values untouched.
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
//
// RNB-1 (§29.88 R2, 2026-07-14): the µs identity between the census-anchored
// Σ and the chain lane's own per-state Σ is no longer a mint gate — it rides
// the decision as the case-A OWNERSHIP qualification (identityHolds) plus the
// two Σs for the typed double-account disclosure. chainLaneMs is 0 with
// chainLanePresent=false when the pid holds no depth>0 causal-impact account
// (transit-node pids) — the ownership question is void there (case B).
type rspaFamilyDecision struct {
	migrate          bool
	anchoredMs       float64
	fullMs           float64
	chainLaneMs      float64
	chainLanePresent bool
	identityHolds    bool
}

func rspaWithinTol(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= rspaAnchorIdentityTolMs
}

// buildRSPAFamilyDecisions computes the runnable and D-IO per-pid migration
// decisions. Mint gates (all precise typed signals):
//  1. the stats sweep ran WITH anchor data (stats.chainAnchorsByPID != nil)
//     and the pid has ≥1 typed jump window;
//  2. the census basis exists (nil census = legacy fixture → fail open);
//  3. anchored ≤ full (+tol) — structural sanity of the single-ledger split;
//  4. 件6 (修复轮, 2026-07-14): the off-CPU ordered-stream premise holds
//     (stats.offCPUProducerDisjoint) — a clock-regressed trace folds seats
//     to the MAX fallback and clips nothing, so NO decision may mint (the
//     mint-time chain D-IO seat suppression reads the same decisions; a
//     regressed trace suppressing the chain seat while window seats stayed
//     unclipped would silently drop the chain value from the board).
//
// EVOLUTION RECORD (RNB-1, §29.88 R2, 2026-07-14): the former gates 3'
// (chain-lane per-state account present) and 5' (§6.3 µs identity) are no
// longer mint gates — the census bipartition is self-sufficiently exact, so
// the decision now ALWAYS mints on gates 1-4 and carries the identity verdict
// (identityHolds) + both Σs as the case-A ownership qualification and the
// typed double-account disclosure inputs. The M-D mint-time chain D-IO seat
// suppression keys on identityHolds (suppressing the chain seat while the
// partition seats carry a DIVERGING anchored Σ would silently swap the chain
// lane's own account for the census account — case A' keeps the chain seat
// as its own ⛓ representative instead).
func buildRSPAFamilyDecisions(chain ChainResult, stats WindowStats) (runnable, dio map[int]rspaFamilyDecision) {
	if stats.chainAnchorsByPID == nil || !stats.offCPUProducerDisjoint {
		return nil, nil
	}
	chainSums := chainAnchoredStateMsByPID(chain)
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
			if sums.anchoredMs > sums.fullMs+rspaAnchorIdentityTolMs {
				continue
			}
			chainStates, chainLanePresent := chainSums[pid]
			laneMs := 0.0
			if chainLanePresent {
				laneMs = chainMs(chainStates)
			}
			if out == nil {
				out = map[int]rspaFamilyDecision{}
			}
			out[pid] = rspaFamilyDecision{
				migrate:          true,
				anchoredMs:       sums.anchoredMs,
				fullMs:           sums.fullMs,
				chainLaneMs:      laneMs,
				chainLanePresent: chainLanePresent,
				identityHolds:    chainLanePresent && rspaWithinTol(sums.anchoredMs, laneMs),
			}
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
//
// EVOLUTION RECORD (RSPA-HYG 修复轮, §29.77, 2026-07-14): the Sprintf arg
// order swapped anchored/full since the RSPA batch — the sentence read
// "full-window account 0.039ms = 0.307ms anchored" (arithmetic
// self-contradictory; live donghu specimen udk-irq-12-92, caught by the 件⑤
// typed sweep dump). The typed fields (ChainAnchoredMs/ChainAnchorFullMs)
// and every display face were always correct; only this engine-side prose
// slot was swapped. Pinned by TestRSPAHygRemainderSummaryArithmetic.
func rspaRemainderSummary(thread ThreadRef, family string, remainder, anchored, full float64) string {
	// RNB-2 件3 (§29.88 W3 病③, 2026-07-15): anchored==0 (typed zero — the
	// case-B zero-credential form) means NO seat holds any anchored share; the
	// ownership claim would name a holder that does not exist (customer E32:
	// 「= 0.000ms anchored ... (owned by the chain seat)」). The zero form also
	// keeps the rspaPatchSummariesForTwinVisibility rewrite a no-op by
	// construction (its verbatim anchor is absent) — the 「seat not published」
	// downgrade would be equally false here.
	owner := rspaSummaryOwnedByChainSeat
	if anchored <= 0 {
		owner = rspaSummaryNoAnchoredShare
	}
	return fmt.Sprintf("%s %s remainder %.3fms outside its wakeup-dependency windows (no chain credential for these segments); full-window account %.3fms = %.3fms anchored inside typed dependency windows %s + this remainder — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, remainder, full, anchored, owner)
}

// rspaRemainderSummaryDivergent — RNB-1 case A' (§29.88 R2, 2026-07-14): the
// honest double-account form. The census bipartition stays exact (full =
// anchored + remainder at the single ledger close site), but the chain seat's
// own published account diverges from the anchored ledger Σ, so this sentence
// never claims the chain seat OWNS the anchored portion and never invites
// adding the two seats. Both Σs and their delta are disclosed verbatim.
func rspaRemainderSummaryDivergent(thread ThreadRef, family string, remainder, anchored, full, chainLaneMs, censusMs float64) string {
	delta := chainLaneMs - censusMs
	if delta < 0 {
		delta = -delta
	}
	// RNB-2 件6 (§29.90 残留③ P4, 2026-07-15): the sentence names TWO anchored
	// quantities — this row's own anchored share (window-seat group stamp /
	// satellite interval account) and the pid-census anchored ledger Σ. Window
	// seats usually pass them µs-equal and the bare word is honest; when the
	// typed floats DIFFER (satellite arm :777 / multi-group stamped seats)
	// each mention gains its account qualifier — the same word must not pun
	// two values in one sentence. Equal pairs keep the pinned bytes.
	anchoredNoun, censusNoun := "anchored inside typed dependency windows", "the anchored ledger sum"
	if !rspaWithinTol(anchored, censusMs) {
		anchoredNoun = "anchored inside typed dependency windows (this seat's own account)"
		censusNoun = "the pid-wide anchored ledger sum"
	}
	return fmt.Sprintf("%s %s remainder %.3fms outside its wakeup-dependency windows (no chain credential for these segments); full-window account %.3fms = %.3fms %s + this remainder — same segment set, mutually disjoint. Anchored-ownership divergence: the chain seat publishes its own account %.3fms while %s is %.3fms (delta %.3fms) — two separate accounts, never additive with each other",
		threadLabel(thread), family, remainder, full, anchored, anchoredNoun, chainLaneMs, censusNoun, censusMs, delta)
}

// rspaLaneDemotionSummary — RNB-1 R4 lane arms (§29.88 R4, 2026-07-14): the
// value-untouched ◇ demotion disclosure for indivisible on-chain rows that
// cannot show a typed-edge anchored share for their whole account. detail
// names the row-specific reason (satellite without interval inventory /
// displacement-measured inversion overlap).
func rspaLaneDemotionSummary(anchored, full float64, detail string) string {
	return fmt.Sprintf("; no chain credential for the full account: the governing full-window ledger account holds %.3fms of %.3fms inside the thread's typed wakeup-dependency windows and %s — the whole row rides the adjacent lane with every published value unchanged",
		anchored, full, detail)
}

// rspaRepresentedDemotionSummary — XLANE-1 件1 (§29.104.1/§29.104.2,
// 2026-07-15): the honest engine-side sentence for the fully-anchored
// satellite whose anchored share is already represented by the chain-lane
// seat. Deliberately NOT the R4 无凭证 word family — this satellite's whole
// account IS credential-anchored; the demotion reason is seat representation
// (one physical account must not hold two full chain seats).
func rspaRepresentedDemotionSummary(anchored, full float64) string {
	return fmt.Sprintf("; anchored share represented by the chain seat: this satellite's whole account (%.3fms of %.3fms) lies inside the thread's typed wakeup-dependency windows AND a same-pool chain-lane runnable seat physically covers those segments on the chain tier — this diagnostic projection rides the adjacent lane whole with every published value unchanged (a second full chain seat would double-represent one physical account)",
		anchored, full)
}

// rspaRowIntervalAnchoredMs computes the anchored overlap of a row that
// carries its OWN typed interval inventory (scheduler_latency /
// low_frequency satellites). Family-folded rows contribute their member
// intervals; a singleton contributes its typed [StartTs, EndTs]. Returns
// ok=false (fail open) when the interval inventory does not reproduce the
// row's own runnable scalar (tolerance) — a hull would misstate the account.
// XLANE-1 件1: the validated interval inventory rides back to the caller so
// the fully-anchored satellite arm can test physical intersection against
// the chain-lane seats without re-deriving (one inventory, two consumers).
func rspaRowIntervalAnchoredMs(windows []TimeWindow, item RootCauseRankItem) (anchored float64, intervals []foldInterval, ok bool) {
	intervals = item.familyMemberIntervals
	if len(intervals) == 0 {
		if item.EndTs <= item.StartTs {
			return 0, nil, false
		}
		intervals = []foldInterval{{start: item.StartTs, end: item.EndTs}}
	}
	lengthMs := 0.0
	for _, iv := range intervals {
		if iv.end <= iv.start {
			return 0, nil, false
		}
		lengthMs += (iv.end - iv.start) * 1000
		anchored += anchorWindowsOverlapMs(windows, iv.start, iv.end)
	}
	if !rspaWithinTol(lengthMs, item.RunnableMs) {
		return 0, nil, false
	}
	return anchored, intervals, true
}

// rspaChainRunnableSeatWindowsByPID (XLANE-1 件1, §29.104.2, 2026-07-15)
// collects the chain-lane runnable-family seats' OWN published segment
// inventory per pid (wakeup_chain.causal_impacts / aggregated_impacts
// sources, runnable dominant state — the same discriminator
// rspaChainSeatPresenceByPID keys on). Inventory priority (precise typed
// segments only):
//  1. familyMemberIntervals — the fold pass's exact member segments;
//  2. OccurrenceWindows — the aggregate lane's typed per-occurrence windows;
//  3. the row's own [StartTs, EndTs] ONLY on a single-occurrence seat.
//
// XLANE-1 修复轮 P1-1 (对抗复核, 2026-07-16) — EVOLUTION RECORD: the first
// cut fell back to StartTs..EndTs even on aggregated seats, where that pair
// is the FirstTs..LastTs ENVELOPE spanning the gaps between occurrences — a
// satellite segment lying entirely inside such a gap "intersected" the hull
// and the sole-representative protection silently died (reachable on mixed
// dominant-state threads). Hull/envelope timestamps are NOISY signals and
// must never feed this hard gate (精确信号进硬门,包络=嘈声): a
// multi-member seat without member inventory and without occurrence windows
// contributes NOTHING (fail open — the satellite keeps its legacy chain
// lane, 唯一代表保护).
func rspaChainRunnableSeatWindowsByPID(items []RootCauseRankItem) map[int][]TimeWindow {
	var out map[int][]TimeWindow
	for i := range items {
		if items[i].Thread.PID <= 0 || !strings.HasPrefix(strings.TrimSpace(items[i].Source), "wakeup_chain") {
			continue
		}
		if items[i].DominantState != string(StateRunnable) {
			continue
		}
		var windows []TimeWindow
		switch {
		case len(items[i].familyMemberIntervals) > 0:
			for _, iv := range items[i].familyMemberIntervals {
				if iv.end > iv.start {
					windows = append(windows, TimeWindow{StartTs: iv.start, EndTs: iv.end})
				}
			}
		case len(items[i].OccurrenceWindows) > 0:
			for _, occ := range items[i].OccurrenceWindows {
				if occ.Window.EndTs > occ.Window.StartTs {
					windows = append(windows, occ.Window)
				}
			}
		case items[i].MemberCount <= 1 && items[i].EndTs > items[i].StartTs:
			// True singleton: the pair IS the one occurrence segment, never
			// an envelope.
			windows = append(windows, TimeWindow{StartTs: items[i].StartTs, EndTs: items[i].EndTs})
		}
		if len(windows) == 0 {
			continue
		}
		if out == nil {
			out = map[int][]TimeWindow{}
		}
		out[items[i].Thread.PID] = append(out[items[i].Thread.PID], windows...)
	}
	for pid, windows := range out {
		out[pid] = mergeAnchorTimeWindows(windows)
	}
	return out
}

// rspaIntervalsOverlapMs sums the overlap of an interval inventory with a
// merged window union — the XLANE-1 physical-intersection test between a
// satellite's own segments and the chain-lane seats' published segments.
func rspaIntervalsOverlapMs(windows []TimeWindow, intervals []foldInterval) float64 {
	total := 0.0
	for _, iv := range intervals {
		total += anchorWindowsOverlapMs(windows, iv.start, iv.end)
	}
	return total
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
//
// rspaChainSeatPresence — RNB-1 B-3 (§29.88 R2, 2026-07-14): presence alone
// is no longer the case-A verdict. The chain seats' published per-family
// value Σ rides beside the booleans so the migration can VERIFY that the
// chain seat actually holds the anchored account (|seat Σ − census-anchored
// Σ| ≤ µs tolerance ∧ decision.identityHolds); a present-but-diverging chain
// seat takes the case-A' double-account disposition instead of the additive
// 同源二分 claim. The Σ reads the published state channels (RunnableMs /
// DStateMs+IOWaitMs — the values the board shows), never Effective (which
// carries gated/folded algebra on inversion and supply rows).
type rspaChainSeatPresence struct {
	runnable   bool
	runnableMs float64
	dio        bool
	dioMs      float64
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
			presence.runnableMs = items[i].RunnableMs
		case string(StateDSleep), string(StateIOWait):
			presence.dio = true
			presence.dioMs = items[i].DStateMs + items[i].IOWaitMs
		default:
			continue
		}
		if out == nil {
			out = map[int]rspaChainSeatPresence{}
		}
		merged := out[items[i].Thread.PID]
		merged.runnable = merged.runnable || presence.runnable
		merged.runnableMs += presence.runnableMs
		merged.dio = merged.dio || presence.dio
		merged.dioMs += presence.dioMs
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
// rspaReanchorOwnedType is the closed set of rank types whose chain-lane
// credential vocabulary is OWNED by the re-anchoring machinery below (the
// reanchorOnChainStateSeats dispatch cases — keep the two lists adjacent).
// ONCHAIN-FIX-2 件1 (2026-07-18): the envelope-tier honest-word stamp in the
// chain-context enrich excludes these types — their credential words (锚定
// 二分 / 卫星降道 / R4 无凭证族) are minted here, and the documented RSPA
// fail-open boundary is deliberately NOT re-worded by the envelope pass
// (审计表已列覆盖行; one lane, one vocabulary owner).
func rspaReanchorOwnedType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "runnable_wait", "priority_inversion_runnable_wait", "cpu_affinity_or_cpuset",
		"scheduler_latency", "low_frequency", "fragmented_runnable_wait",
		"d_state_or_io_wait", "io_wait", "fragmented_d_state_or_io_wait":
		return true
	default:
		return false
	}
}

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
	// XLANE-1 件1: the chain-lane runnable seats' own published segment union
	// per pid — the physical-intersection witness for the fully-anchored
	// satellite demotion (computed once per pass; the demotion never mutates
	// the chain seats themselves, so the snapshot stays valid).
	chainRunnableSeatWindows := rspaChainRunnableSeatWindowsByPID(items)
	var appended []RootCauseRankItem
	for i := range items {
		item := &items[i]
		if item.ChainAnchorRemainderSeat || item.ChainAnchorFullMs > 0 || item.AbsorbedByRankFamily ||
			item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat {
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
			// RNB-1 T1 (§29.88 R2): the seat reconciles against ITS OWN census
			// group ledger account (mint stamp, Σ'd by the family fold) — the
			// former seat-value == pid-census-full identity failed open on any
			// mixed-lane split fold and kept the full multi-fragment value on
			// the chain tier (customer E8/E22; INV-D S5/S6). Stamp-less seats
			// (legacy fixtures) keep the pid-identity fallback.
			var anchored, full float64
			switch {
			case item.ledgerAnchorStamped:
				anchored, full = item.ledgerAnchoredRunnableMs, item.RunnableMs
			case rspaWithinTol(item.RunnableMs, decision.fullMs):
				anchored, full = decision.anchoredMs, decision.fullMs
			default:
				continue
			}
			if full <= 0 || anchored > full+rspaAnchorIdentityTolMs {
				continue
			}
			remainder := rspaClampNonNegative(full - anchored)
			presence := chainSeats[item.Thread.PID]
			// RNB-1 B-3: case A requires the chain seat to provably HOLD the
			// anchored account — presence ∧ µs identity (decision) ∧ published
			// seat Σ ≈ census-anchored Σ. A present-but-diverging chain seat is
			// case A' (double-account disclosure, no additive claim).
			caseAOwned := presence.runnable && decision.identityHolds && rspaWithinTol(presence.runnableMs, decision.anchoredMs)
			switch {
			case caseAOwned:
				// Case A: the chain seat owns the anchored portion; this seat
				// publishes the remainder (≈0 → absorbed by the recon arm).
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					rspaRemainderSummary(item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full))
			case presence.runnable:
				// Case A' (RNB-1, §29.88 R2): the window seat still rides ◇ as
				// the remainder (the census bipartition is exact regardless),
				// the chain seat stays published as its own ⛓ representative,
				// and the typed double-Σ disclosure replaces the additive claim.
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					rspaRemainderSummaryDivergent(item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full, presence.runnableMs, decision.anchoredMs))
				item.ChainAnchorOwnershipDivergent = true
				item.ChainAnchorChainLaneMs = presence.runnableMs
				item.ChainAnchorCensusMs = decision.anchoredMs
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
		case "priority_inversion_runnable_wait":
			// RNB-1 R4 lane arm (§29.88 R4/§29.61.10c 逐边核): the inversion-
			// rewritten window seat's gated eff is a same-CPU displacement
			// measurement (10a: pure time overlap = adjacency-level evidence)
			// that cannot be split along the anchor boundary without minting a
			// value equal to neither the measurement nor any partition term
			// (§29.83 件③ narrowing). A fully-anchored seat account keeps the
			// chain lane byte-identically (every runnable ms sits before a
			// typed edge, so the inversion claim is pre-edge too); any
			// unanchored share demotes the WHOLE seat to ◇ — values untouched.
			decision, ok := runnableDecisions[item.Thread.PID]
			if !ok || !decision.migrate || item.Source != "window_stats" {
				continue
			}
			var anchored, full float64
			switch {
			case item.ledgerAnchorStamped:
				anchored, full = item.ledgerAnchoredRunnableMs, item.RunnableMs
			case rspaWithinTol(item.RunnableMs, decision.fullMs):
				anchored, full = decision.anchoredMs, decision.fullMs
			default:
				continue
			}
			if full <= 0 || full-anchored <= rspaAnchorIdentityTolMs {
				continue
			}
			item.ChainCredentialLaneDemoted = true
			item.Causality = "adjacent_to_wakeup_chain"
			item.ChainRelevance = "adjacent"
			item.Summary += rspaLaneDemotionSummary(anchored, full,
				"the priority-inversion overlap is a same-CPU displacement measurement that cannot be split along that boundary")
		case "cpu_affinity_or_cpuset":
			// RNB-1 R4 lane arm (§29.88 R4; INV-C B-2): the affinity satellite
			// duplicates the thread's runnable wait on the capped top basis and
			// carries NO per-row interval inventory — it can never prove its
			// own anchored share, so a pid with ANY unanchored census share
			// demotes the whole row to ◇ (values untouched; the formal window/
			// chain seats own the bisected accounts). A fully-anchored pid
			// (census remainder ≤ tol) keeps the chain lane byte-identically.
			decision, ok := runnableDecisions[item.Thread.PID]
			if !ok || !decision.migrate || item.Source != "window_stats.cpu_constraints" {
				continue
			}
			if item.RunnableMs <= 0 || decision.fullMs-decision.anchoredMs <= rspaAnchorIdentityTolMs {
				continue
			}
			item.ChainCredentialLaneDemoted = true
			item.Causality = "adjacent_to_wakeup_chain"
			item.ChainRelevance = "adjacent"
			item.Summary += rspaLaneDemotionSummary(decision.anchoredMs, decision.fullMs,
				"this affinity/cpuset satellite carries no interval inventory of its own")
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
			anchored, rowIntervals, intervalOK := rspaRowIntervalAnchoredMs(stats.chainAnchorsByPID[item.Thread.PID], *item)
			if !intervalOK {
				// RNB-1 R4 fallback (§29.88.2/§29.88.8 case 4, 2026-07-14) —
				// EVOLUTION RECORD: the former arm failed OPEN here (hull
				// mismatch or no interval inventory kept the FULL value on the
				// chain tier — the donghu 2955 低频运行 22.408/12.386 and the
				// tieba 59953 low_frequency 10.776 live escapes: the
				// compute_supply-sourced verdict rows carry no typed interval
				// at all). A satellite that cannot prove its OWN anchored
				// share and whose pid census holds any unanchored share now
				// rides the ◇ lane whole (values untouched — clipping on the
				// pid pair would misstate this row's own account); a fully-
				// anchored pid keeps the chain lane byte-identically.
				if decision.fullMs-decision.anchoredMs <= rspaAnchorIdentityTolMs {
					continue
				}
				item.ChainCredentialLaneDemoted = true
				item.Causality = "adjacent_to_wakeup_chain"
				item.ChainRelevance = "adjacent"
				item.Summary += rspaLaneDemotionSummary(decision.anchoredMs, decision.fullMs,
					"this satellite's own interval inventory cannot prove its anchored share")
				continue
			}
			full := item.RunnableMs
			if anchored > full+rspaAnchorIdentityTolMs {
				continue
			}
			remainder := rspaClampNonNegative(full - anchored)
			if remainder <= rspaAnchorIdentityTolMs {
				// XLANE-1 件1 (§29.104.1/§29.104.2, 2026-07-15) — EVOLUTION
				// RECORD: this fully-anchored path used to keep the WHOLE
				// satellite on the chain tier unconditionally (the runnable2
				// customer escape: E11 调度延迟 23.471 full beside chain seats
				// E26/E28 of the SAME physical runnable — chain-lane runnable
				// eff Σ 53.5ms on a 26.725ms full-window account, 2.0×). Per
				// the B4 header semantics (the satellite "must not mint a
				// second seat"): when the same-pid chain-lane runnable seats'
				// typed segment inventory FULLY COVERS this row's own interval
				// inventory, the anchored share is already represented on the
				// chain tier in full, so the satellite rides ◇ whole — values
				// untouched, honest word face (represented-by-chain-seat,
				// never 无凭证: this account IS credential-anchored). The
				// exact interval-twin subset is subsequently absorbed into the
				// chain seat by the extended B4 recon pair (single seat + E#
				// merge); this arm is the non-twin-but-covered fallback.
				//
				// XLANE-1 修复轮 P1-2 (对抗复核, 2026-07-16) — EVOLUTION
				// RECORD: the first cut demoted on ANY intersection (>0),
				// letting a 1ms chain seat swallow a 5ms satellite whole while
				// the sentence claimed 「已由链上席全额代表」. The gate is now
				// a COVERAGE PROOF: Σ overlap of the chain seats' segment
				// union over the satellite's own intervals equals the
				// satellite's account within the µs tolerance (the demoted
				// row's invariant: coverage ≈ its published value). Partial
				// coverage keeps the chain lane byte-identically — the
				// sole-representative protection's natural extension (the
				// uncovered share has no other chain representative), same for
				// an absent chain seat or an unprovable inventory
				// (禁把锚定份丢出链, negative pins).
				if seatWindows := chainRunnableSeatWindows[item.Thread.PID]; len(seatWindows) > 0 &&
					rspaWithinTol(rspaIntervalsOverlapMs(seatWindows, rowIntervals), full) {
					item.ChainAnchorRepresentedByChainSeat = true
					item.Causality = "adjacent_to_wakeup_chain"
					item.ChainRelevance = "adjacent"
					item.Summary += rspaRepresentedDemotionSummary(anchored, full)
				}
				continue
			}
			// RNB-1: a divergent-ownership pid's satellite speaks the honest
			// double-account form too — its legacy sentence claims the chain
			// seat owns the anchored share, which case A' cannot claim.
			if presence := chainSeats[item.Thread.PID]; presence.runnable &&
				!(decision.identityHolds && rspaWithinTol(presence.runnableMs, decision.anchoredMs)) {
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					rspaRemainderSummaryDivergent(item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full, presence.runnableMs, decision.anchoredMs))
				item.ChainAnchorOwnershipDivergent = true
				item.ChainAnchorChainLaneMs = presence.runnableMs
				item.ChainAnchorCensusMs = decision.anchoredMs
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
			if presence := chainSeats[item.Thread.PID]; presence.runnable &&
				!(decision.identityHolds && rspaWithinTol(presence.runnableMs, decision.anchoredMs)) {
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					rspaRemainderSummaryDivergent(item.Thread, "fragmented runnable (scheduling-pressure candidate)", remainder, anchored, full, presence.runnableMs, decision.anchoredMs))
				item.ChainAnchorOwnershipDivergent = true
				item.ChainAnchorChainLaneMs = presence.runnableMs
				item.ChainAnchorCensusMs = decision.anchoredMs
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
			presence := chainSeats[item.Thread.PID]
			// RNB-1 B-3: same case-A ownership qualification as the runnable
			// arm (presence ∧ decision identity ∧ published seat Σ ≈ anchored).
			caseAOwned := presence.dio && decision.identityHolds && rspaWithinTol(presence.dioMs, decision.anchoredMs)
			switch {
			case caseAOwned:
				rspaRewriteSeatToRemainder(item, anchored, full, 0, remainderD, remainderIO,
					rspaRemainderSummary(item.Thread, "D/IO blocking", remainder, anchored, full))
			case presence.dio:
				// Case A' (RNB-1): remainder migration + typed double-Σ
				// disclosure; the chain seat keeps its own ⛓ account (the M-D
				// mint suppression never fires on divergent pids).
				rspaRewriteSeatToRemainder(item, anchored, full, 0, remainderD, remainderIO,
					rspaRemainderSummaryDivergent(item.Thread, "D/IO blocking", remainder, anchored, full, presence.dioMs, decision.anchoredMs))
				item.ChainAnchorOwnershipDivergent = true
				item.ChainAnchorChainLaneMs = presence.dioMs
				item.ChainAnchorCensusMs = decision.anchoredMs
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
			if presence := chainSeats[item.Thread.PID]; presence.dio &&
				!(decision.identityHolds && rspaWithinTol(presence.dioMs, decision.anchoredMs)) {
				rspaRewriteSeatToRemainder(item, anchored, full, 0, item.DStateMs*share, item.IOWaitMs*share,
					rspaRemainderSummaryDivergent(item.Thread, "fragmented D/IO blocking", remainder, anchored, full, presence.dioMs, decision.anchoredMs))
				item.ChainAnchorOwnershipDivergent = true
				item.ChainAnchorChainLaneMs = presence.dioMs
				item.ChainAnchorCensusMs = decision.anchoredMs
				continue
			}
			rspaRewriteSeatToRemainder(item, anchored, full, 0, item.DStateMs*share, item.IOWaitMs*share,
				rspaRemainderSummary(item.Thread, "fragmented D/IO blocking", remainder, anchored, full))
		}
	}
	return append(items, appended...)
}

// rspaSummaryOwnedByChainSeat / rspaSummaryRemainderTwinPublished are the two
// engine-side EN co-publication claims the account sentences mint
// (rspaRemainderSummary / rspaAnchoredSummary). rspaPatchSummariesForTwin-
// Visibility rewrites them when the claimed twin did not survive to the
// PUBLISHED board (D1 修复轮, §29.88 复核, 2026-07-14): a truncation-killed
// twin turned "(owned by the chain seat)" into a dangling pointer — the board
// showed a remainder account claiming an owner nobody could see. Verbatim
// substring anchors (precise signals); already-patched rows are no-ops, so
// the build→enrich double pass stays idempotent.
const rspaSummaryOwnedByChainSeat = "(owned by the chain seat)"
const rspaSummaryOwnedByChainSeatUnpublished = "(the owning seat is not on the published board; see the compaction disclosure)"

// rspaSummaryNoAnchoredShare — RNB-2 件3 (§29.88 W3 病③, 2026-07-15): the
// zero-anchored remainder form. No seat holds any anchored share, so neither
// ownership claim above may render (both would name a nonexistent holder).
const rspaSummaryNoAnchoredShare = "(no anchored share exists — no seat holds it)"
const rspaSummaryRemainderTwinPublished = "(published as a separate adjacent seat)"
const rspaSummaryRemainderTwinUnpublished = "(its adjacent remainder seat is not on the published board; see the compaction disclosure)"

// rspaPatchSummariesForTwinVisibility runs AFTER each candidate/side-lane
// truncation (build + enrich): for every published bipartition half it checks
// whether the co-published twin the sentence claims actually survived, and
// downgrades the claim to the honest unpublished form when it did not. The
// typed decomposition fields are untouched — only the engine prose pointer.
func rspaPatchSummariesForTwinVisibility(items []RootCauseRankItem) {
	type familyPresence struct {
		runnable bool
		dio      bool
	}
	clipped := map[int]familyPresence{}
	remainder := map[int]familyPresence{}
	mark := func(m map[int]familyPresence, pid int, dominant string) {
		p := m[pid]
		switch dominant {
		case string(StateRunnable):
			p.runnable = true
		case string(StateDSleep), string(StateIOWait):
			p.dio = true
		default:
			return
		}
		m[pid] = p
	}
	for i := range items {
		if items[i].Thread.PID <= 0 || items[i].ChainAnchorFullMs <= 0 {
			continue
		}
		if items[i].ChainAnchorRemainderSeat {
			mark(remainder, items[i].Thread.PID, items[i].DominantState)
		} else {
			mark(clipped, items[i].Thread.PID, items[i].DominantState)
		}
	}
	chainSeats := rspaChainSeatPresenceByPID(items)
	has := func(m map[int]familyPresence, pid int, dominant string) bool {
		switch dominant {
		case string(StateRunnable):
			return m[pid].runnable
		case string(StateDSleep), string(StateIOWait):
			return m[pid].dio
		default:
			// running/s_sleep rows never mint bipartition halves — no claim
			// to verify.
			return false
		}
	}
	for i := range items {
		item := &items[i]
		if item.Thread.PID <= 0 || item.ChainAnchorFullMs <= 0 {
			continue
		}
		if item.ChainAnchorRemainderSeat {
			chainTwin := false
			switch item.DominantState {
			case string(StateRunnable):
				chainTwin = chainSeats[item.Thread.PID].runnable
			case string(StateDSleep), string(StateIOWait):
				chainTwin = chainSeats[item.Thread.PID].dio
			default:
				// running/s_sleep dominants never mint remainder seats.
			}
			if !chainTwin && !has(clipped, item.Thread.PID, item.DominantState) {
				item.Summary = strings.Replace(item.Summary, rspaSummaryOwnedByChainSeat, rspaSummaryOwnedByChainSeatUnpublished, 1)
			}
		} else if !has(remainder, item.Thread.PID, item.DominantState) {
			item.Summary = strings.Replace(item.Summary, rspaSummaryRemainderTwinPublished, rspaSummaryRemainderTwinUnpublished, 1)
		}
	}
}

// stampResourceClosureEvaluation marks every resource-attribution row with
// the typed "closure credential was computable" bit when the stats sweep ran
// with the RSPA anchor basis. The enrich lane decision requires the
// completion-closure credential ONLY on evaluated io_latency rows — an
// anchor-less build (legacy fixtures, chainless queries) keeps the pre-RSPA
// overlap behavior byte-identically. Other resource projections keep their
// lane (host-wait/host-work credential form, see the enrich arm comment) but
// carry the bit so the display can distinguish "evaluated" from "legacy".
//
// RSPA-HYG 件③ (§29.77 立案③, 2026-07-14): the io_burst_episode /
// block_io_by_inode facets additionally carry the typed host-containment
// verdict — their host-form credential (§3.1: the row's interval IS the
// anchored host thread's own wait/work occupying its dependency window) is
// refined per the §29.61.10c per-edge criterion from "any overlap" to
// interval ⊆ anchor-window union (µs tolerance). io_latency owns the stronger
// per-IO completion-closure credential above.
//
// EVOLUTION RECORD (RSPA-HYG 残余批, §29.83 残余③, 2026-07-14): the four
// facets the 立案③ batch left on the legacy host form were audited per edge
// (§29.61.10d 逐边核非按类) and dispose as follows:
//   - file_io_hot_inode  应锚定核验 — per-(dev,inode,op,PID) aggregation
//     ENVELOPE carrying the host's own Σ file-IO latencies (or a bytes-derived
//     advisory value): the same composite-over-envelope caliber as
//     block_io_by_inode, so partial containment means part of the account sits
//     outside the dependency windows (pure time overlap → ◇, value untouched);
//   - workqueue_activity 应锚定核验 — per-(source,PID,work,fn) lane summing the
//     worker's own paired execution durations under a first/last-event
//     envelope: host-own-work form, same containment refinement;
//   - dma_fence_activity 应锚定核验 — per-(source,PID,driver,timeline,ctx,
//     seqno) lane summing the host's own paired fence waits under an event
//     envelope: host-own-wait form, same containment refinement;
//   - page_cache_churn   应豁免(已邻近)— structurally excluded from the
//     rootCauseTypeCanBeDirectOnChain closed list, so its rows can NEVER hold
//     the chain tier and a containment bit would be dead semantics (its value
//     is a synthetic churn-count score, not a wall-clock account). Production
//     witnesses in both flagship windows publish adjacent already (tieba
//     ThreadPoolForeg-60555 churn envelope 61.540ms/24.568ms inside; donghu
//     17267 target-self 169.355ms envelope).
//
// 如实注: neither flagship window mints an on-chain row of the three newly
// covered facets — their arms are covered by the synthetic unit fixtures
// (same disposition as io_burst_episode in the 立案③ batch).
func stampResourceClosureEvaluation(stats WindowStats, items []RootCauseRankItem) {
	if stats.chainAnchorsByPID == nil {
		return
	}
	for i := range items {
		if !rootCauseTypeIsResourceAttribution(items[i].Type) {
			continue
		}
		items[i].resourceClosureEvaluated = true
		switch strings.TrimSpace(items[i].Type) {
		case "io_burst_episode", "block_io_by_inode", "file_io_hot_inode", "workqueue_activity", "dma_fence_activity":
			if items[i].EndTs <= items[i].StartTs {
				// Interval-less rows are already demoted by the typed-interval
				// arm (enrich :15377) — nothing to evaluate here.
				continue
			}
			items[i].resourceHostContainmentEvaluated = true
			lengthMs := (items[i].EndTs - items[i].StartTs) * 1000
			anchored := anchorWindowsOverlapMs(stats.chainAnchorsByPID[items[i].Thread.PID], items[i].StartTs, items[i].EndTs)
			items[i].resourceHostWindowContained = anchored >= lengthMs-rspaAnchorIdentityTolMs
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
