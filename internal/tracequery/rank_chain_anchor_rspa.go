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
	sourcePath string
	wakerPID   int
	wakeePID   int
	ts         float64
	line       int
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
func rspaRewriteSeatToRemainder(item *RootCauseRankItem, anchoredMs, fullMs float64, remainderRunnable, remainderD, remainderIO float64, summary rspaSentence) {
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
	if summary.text != "" {
		rspaSetSentence(item, summary)
	}
	// HMC377 fix round 5: the ◇ seat holds its remainder (its instants are
	// recorded by the arm when its inventory reproduces it; the value
	// otherwise).
	rspaHold(item, RootCauseShareHolderRemainderSeat, remainderD+remainderIO > 0, nil, remainder)
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
	return rspaRemainderSentence(thread, family, remainder, anchored, full, RootCauseShareClaim{}).text
}

// rspaRemainderSentence is rspaRemainderSummary with the claim that names
// the anchored share's holder (HMC377 fix round 5: resolved against the
// published board by resolveRankShareClaims).
func rspaRemainderSentence(thread ThreadRef, family string, remainder, anchored, full float64, owner RootCauseShareClaim) rspaSentence {
	// RNB-2 件3 (§29.88 W3 病③, 2026-07-15): anchored==0 (typed zero — the
	// case-B zero-credential form) means NO seat holds any anchored share; the
	// ownership claim would name a holder that does not exist (customer E32:
	// 「= 0.000ms anchored ... (owned by the chain seat)」). The zero form makes
	// no holder claim at all — the 「seat not published」 disclosure would be
	// equally false here.
	var s rspaSentence
	ownerPhrase := rspaSummaryNoAnchoredShare
	if anchored > 0 {
		ownerPhrase = s.ownerSpan(owner)
	}
	s.text = fmt.Sprintf("%s %s remainder %.3fms outside its wakeup-dependency windows (no chain credential for these segments); full-window account %.3fms = %.3fms anchored inside typed dependency windows %s + this remainder — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, remainder, full, anchored, ownerPhrase)
	return s
}

// rspaChainAccountPhrases are the held / compaction-disclosure / context
// forms of a case-A' remainder's pointer to the chain seat's own (diverging)
// account.
func rspaChainAccountPhrases(chainLaneMs float64) (string, string, string) {
	return fmt.Sprintf("the chain seat publishes its own account %.3fms", chainLaneMs),
		fmt.Sprintf("the chain seat's own account %.3fms is not on the published board (see the compaction disclosure)", chainLaneMs),
		fmt.Sprintf("the chain seat's own account %.3fms %s", chainLaneMs, rspaHolderContextOnly)
}

// rspaRemainderSummaryDivergent — RNB-1 case A' (§29.88 R2, 2026-07-14): the
// honest double-account form. The census bipartition stays exact (full =
// anchored + remainder at the single ledger close site), but the chain seat's
// own published account diverges from the anchored ledger Σ, so this sentence
// never claims the chain seat OWNS the anchored portion and never invites
// adding the two seats. Both Σs and their delta are disclosed verbatim.
func rspaRemainderSummaryDivergent(thread ThreadRef, family string, remainder, anchored, full, chainLaneMs, censusMs float64) string {
	return rspaRemainderSentenceDivergent(thread, family, remainder, anchored, full, chainLaneMs, censusMs, RootCauseShareClaim{}).text
}

// rspaRemainderSentenceDivergent is rspaRemainderSummaryDivergent with the
// claim that the chain seat keeps its own account on the board.
func rspaRemainderSentenceDivergent(thread ThreadRef, family string, remainder, anchored, full, chainLaneMs, censusMs float64, chainAccount RootCauseShareClaim) rspaSentence {
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
	var s rspaSentence
	held, compact, context := rspaChainAccountPhrases(chainLaneMs)
	account := s.spanContext(held, compact, context, chainAccount)
	s.text = fmt.Sprintf("%s %s remainder %.3fms outside its wakeup-dependency windows (no chain credential for these segments); full-window account %.3fms = %.3fms %s + this remainder — same segment set, mutually disjoint. Anchored-ownership divergence: %s while %s is %.3fms (delta %.3fms) — two separate accounts, never additive with each other",
		threadLabel(thread), family, remainder, full, anchored, anchoredNoun, account, censusNoun, censusMs, delta)
	return s
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

// rspaInversionRepresentedSummary — HMC377 R-377-4: the engine-side sentence
// for an inversion window account whose ready time inside the dependency
// windows is published by the same thread's chain seat (the row becomes that
// seat's lossless detail). It states the split of the RAW READY account; the
// overlap measurement keeps its own name and is never re-labelled as the
// whole account.
func rspaInversionRepresentedSummary(anchored, full float64, anchoredClaim, remainderClaim RootCauseShareClaim) rspaSentence {
	var s rspaSentence
	head, published, at := s.inversionDetailHead(anchoredClaim)
	remainder := s.span(rspaInversionRemainderPublished, rspaInversionRemainderCompacted, remainderClaim)
	tail := s.inversionDetailTail(at)
	s.text = fmt.Sprintf("%s%.3fms of this %.3fms ready-time account lies inside the thread's typed wakeup-dependency windows, %s, and %.3fms lies outside them (%s) — this whole-account row (its same-CPU lower-priority overlap is measured over the whole ready account and is never split) %s",
		head, anchored, full, published, full-anchored, remainder, tail)
	return s
}

// rspaInversionDetailHead / rspaInversionDetailTail are the phrases of every
// R-377-4 detail sentence that name the chain seat the detail rides under
// (held form; the compaction form is the same words — the engine cap cut a
// holder, the seat is still the row's family). HMC377 round 11 (R-377-11 ①):
// when the publication gate withdrew that seat they say it is published only
// as context, and the detail no longer claims to be that seat's detail.
const (
	rspaInversionDetailHeadHeld    = "; represented by this thread's chain seat: "
	rspaInversionDetailHeadContext = "; this thread's chain seat " + rspaHolderContextOnly + ": "
	rspaInversionDetailTailHeld    = "rides as that seat's lossless detail, not as a second object for the same thread"
	rspaInversionDetailTailContext = "rides as lossless detail, not as a second object for the same thread"
)

// inversionDetailHead registers the head phrase and the chain-seat phrase of
// an R-377-4 detail sentence on the anchored claim (registered once); at is
// that claim's index for the tail.
func (s *rspaSentence) inversionDetailHead(anchored RootCauseShareClaim) (head, published string, at int) {
	before := len(s.claims)
	head = s.spanForms(rspaSpanForms{held: rspaInversionDetailHeadHeld, compact: rspaInversionDetailHeadHeld,
		context: rspaInversionDetailHeadContext, successor: rspaInversionDetailHeadContext, returned: rspaInversionDetailHeadContext}, anchored)
	at = s.lastClaim(before)
	published = s.spanOnClaim(rspaSpanForms{held: rspaInversionChainSeatPublishes, compact: rspaInversionChainSeatCompacted,
		context: rspaInversionChainSeatContext, successor: rspaInversionChainSeatSuccessor}, at)
	if at < 0 {
		published = rspaInversionChainSeatPublishes
	}
	return head, published, at
}

// inversionDetailTail registers the tail phrase on the same anchored claim.
func (s *rspaSentence) inversionDetailTail(at int) string {
	return s.spanOnClaim(rspaSpanForms{held: rspaInversionDetailTailHeld, compact: rspaInversionDetailTailHeld,
		context: rspaInversionDetailTailContext, successor: rspaInversionDetailTailContext, returned: rspaInversionDetailTailContext}, at)
}

// rspaInversionChainSeatPublishes / rspaInversionRemainderPublished are the
// held forms of the two seat claims of every R-377-4 detail sentence (the
// chain seat holding the ready time inside the dependency windows, the
// thread's ◇ ready-time remainder), and the *Compacted forms their
// compaction disclosures.
const (
	rspaInversionChainSeatPublishes = "which that chain seat already publishes"
	rspaInversionChainSeatCompacted = "which the chain seat holding it publishes, though that seat is not on the published board (see the compaction disclosure)"
	rspaInversionChainSeatContext   = "which the row holding it publishes only as context, tier context_only"
	rspaInversionRemainderPublished = "published as the thread's adjacent ready-time remainder"
	rspaInversionRemainderCompacted = "the thread's adjacent ready-time remainder seat is not on the published board; see the compaction disclosure"
	// rspaInversionChainSeatSuccessor (HMC377 round 11, R-377-11 ①): the
	// chain seat was withdrawn by the publication gate and its segments passed
	// to a published successor (the ◇ remainder or another row of this thread).
	rspaInversionChainSeatSuccessor = "which another row of this thread on the published board counts (the row that held it is published only as context, tier context_only)"
)

// inversionAccountKey is the paired identity minted on an indivisible
// inversion window row and on the one seat that represents it (HMC377
// R-377-4 / E2-R1). Exact typed fields only.
func inversionAccountKey(item RootCauseRankItem) string {
	return fmt.Sprintf("inversion_account:pid=%d|lines=%d-%d|window=%.6f..%.6f",
		item.Thread.PID, item.LineStart, item.LineEnd, item.StatsWindowStartTs, item.StatsWindowEndTs)
}

// rspaChainRunnableSeatIndexByPID lists, per pid, the chain-lane seats that
// own a runnable account under the rspaChainSeatPresenceByPID roles (the same
// typed predicate: a priority_inversion_candidate row only through its gated
// runnable component, every other chain row through DominantState runnable).
func rspaChainRunnableSeatIndexByPID(items []RootCauseRankItem) map[int][]int {
	var out map[int][]int
	for i := range items {
		if items[i].Thread.PID <= 0 || !strings.HasPrefix(strings.TrimSpace(items[i].Source), "wakeup_chain") ||
			items[i].AbsorbedByRankFamily {
			continue
		}
		runnable := false
		if strings.TrimSpace(items[i].Type) == "priority_inversion_candidate" {
			runnable = items[i].GatedRunnableMs > 0
		} else {
			runnable = items[i].DominantState == string(StateRunnable)
		}
		if !runnable {
			continue
		}
		if out == nil {
			out = map[int][]int{}
		}
		out[items[i].Thread.PID] = append(out[items[i].Thread.PID], i)
	}
	return out
}

// rspaRepresentedDemotionSummary — XLANE-1 件1 (§29.104.1/§29.104.2,
// 2026-07-15): the honest engine-side sentence for the fully-anchored
// satellite whose anchored share is already represented by the chain-lane
// seat. Deliberately NOT the R4 无凭证 word family — this satellite's whole
// account IS credential-anchored; the demotion reason is seat representation
// (one physical account must not hold two full chain seats).
func rspaRepresentedDemotionSummary(anchored, full float64, claims ...RootCauseShareClaim) rspaSentence {
	var s rspaSentence
	s.text = s.representedSpan(fmt.Sprintf("; anchored share represented by the chain seat: this satellite's whole account (%.3fms of %.3fms) lies inside the thread's typed wakeup-dependency windows AND a same-pool chain-lane runnable seat physically covers those segments on the chain tier — this diagnostic projection rides the adjacent lane whole with every published value unchanged (a second full chain seat would double-represent one physical account)",
		anchored, full), anchored, full, claims...)
	return s
}

// rspaRepresentedCompact is the compaction-disclosure form of every XLANE-1
// represented sentence: the satellite's account lies inside the thread's
// dependency windows, the seat(s) holding it are off the published board,
// and the row keeps its own value on the adjacent lane (never a second chain
// seat — the share keeps its one owner).
func rspaRepresentedCompact(inside, full float64) string {
	return fmt.Sprintf("; this satellite's whole account (%.3fms of %.3fms) lies inside the thread's dependency windows, but the seat holding that time is not on the published board (see the compaction disclosure) — this diagnostic projection keeps its own value on the adjacent lane",
		inside, full)
}

// rspaRepresentedContext is the form of a represented sentence whose holder
// the publication gate withdrew (HMC377 round 10, R-377-10 ④): the holding
// row is published only as context, and the satellite keeps its own value.
func rspaRepresentedContext(inside, full float64) string {
	return fmt.Sprintf("; this satellite's whole account (%.3fms of %.3fms) lies inside the thread's dependency windows, but the row holding that time %s — this diagnostic projection keeps its own value on the adjacent lane",
		inside, full, rspaHolderContextOnly)
}

// rspaRepresentedPartial is the form of a represented sentence that resolves
// only in part (HMC377 round 10, R-377-10 ③): heldMs is held and counted by
// seats of this thread on the published board, the other restMs by a seat the
// cap compacted off the board (the compaction disclosure, unchanged wording)
// or by a row the publication gate published only as context (restContext).
// The row's published values are its own account less heldMs.
func rspaRepresentedPartial(inside, full, heldMs, restMs float64, restContext bool) string {
	rest := "is not on the published board (see the compaction disclosure)"
	if restContext {
		rest = rspaHolderContextOnly
	}
	return fmt.Sprintf("; this satellite's whole account (%.3fms of %.3fms) lies inside the thread's dependency windows: %.3fms of it is held by a seat of this thread on the published board, which counts it, and the seat holding the other %.3fms %s — this diagnostic projection rides the adjacent lane and its published values count only those %.3fms",
		inside, full, heldMs, restMs, rest, restMs)
}

// rspaRepresentedSuccessor is the form of a represented sentence whose every
// claim resolves while some of its share is held by the published successor
// of a seat the publication gate withdrew (HMC377 round 11, R-377-11 ①): the
// row is represented whole by another row of this thread, which is not a
// chain seat.
func rspaRepresentedSuccessor(inside, full float64) string {
	return fmt.Sprintf("; anchored share counted by another row of this thread: this satellite's whole account (%.3fms of %.3fms) lies inside the thread's dependency windows; the row that held that time %s, and another row of this thread on the published board counts it — this diagnostic projection rides the adjacent lane whole with every published value unchanged",
		inside, full, rspaHolderContextOnly)
}

// rspaHolderContextOnly is the one clause every claim phrase takes when the
// holder the engine resolved it to was published by the trace_query
// publication gate as context only (HMC377 round 10, R-377-10 ④): it states
// the row's published tier, never that the row is absent and never that a
// seat owns the share.
const rspaHolderContextOnly = "is published only as context, tier context_only"

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
		isPriorityRunnableSubseat := RootCauseTypeIsPriorityInversion(items[i].Type) &&
			strings.TrimSpace(items[i].Type) == "priority_inversion_candidate" &&
			items[i].GatedRunnableMs > 0
		if items[i].DominantState != string(StateRunnable) && !isPriorityRunnableSubseat {
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
		// B1260: a priority-inversion row can now be a scheduling-supply
		// SUB-SEAT beside a sleep / D / IO dominant blocking seat. Its original
		// dominant state remains useful evidence context, but it does not change
		// the physical account owned by this rank seat: only the exact gated
		// runnable component can own a runnable census share, and it can never
		// own the D/IO account. Classifying this row by DominantState made a 1ms
		// scheduling sub-seat impersonate an 11ms IO chain seat; RSPA then
		// rewrote the real IO owner to a zero remainder and silently lost the
		// blocking value. The typed rank kind is the precise seat-role signal.
		if RootCauseTypeIsPriorityInversion(items[i].Type) &&
			strings.TrimSpace(items[i].Type) == "priority_inversion_candidate" {
			if items[i].GatedRunnableMs <= 0 {
				continue
			}
			presence.runnable = true
			presence.runnableMs = items[i].GatedRunnableMs
		} else {
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
func rspaCloneAsRemainderSeat(seat RootCauseRankItem, anchoredMs, fullMs, remainderRunnable, remainderD, remainderIO float64, summary rspaSentence) RootCauseRankItem {
	clone := cloneRootCauseMeasurementSources(seat)
	rspaClearClaims(&clone)
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
func rspaClipSeatToAnchored(item *RootCauseRankItem, anchoredMs, fullMs float64, anchoredRunnable, anchoredD, anchoredIO float64, summary rspaSentence) {
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
	if summary.text != "" {
		rspaSetSentence(item, summary)
	}
}

// rspaAnchoredSummary renders the ⛓ clipped seat's engine-side account.
func rspaAnchoredSummary(thread ThreadRef, family string, anchored, full float64) string {
	return rspaAnchoredSentence(thread, family, anchored, full, RootCauseShareClaim{}).text
}

// rspaAnchoredSentence is rspaAnchoredSummary with the claim that names its
// ◇ remainder twin.
func rspaAnchoredSentence(thread ThreadRef, family string, anchored, full float64, twin RootCauseShareClaim) rspaSentence {
	var s rspaSentence
	twinPhrase := s.span(rspaSummaryRemainderTwinPublished, rspaSummaryRemainderTwinUnpublished, twin)
	s.text = fmt.Sprintf("%s %s %.3fms anchored inside its typed wakeup-dependency windows (chain credential); full-window account %.3fms = this anchored portion + %.3fms remainder outside the dependency windows %s — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, anchored, full, full-anchored, twinPhrase)
	return s
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
	if RootCauseTypeIsPriorityInversion(typ) {
		return true
	}
	switch strings.TrimSpace(typ) {
	case "runnable_wait", "cpu_affinity_or_cpuset",
		"scheduler_latency", "low_frequency", "fragmented_runnable_wait",
		"d_state_or_io_wait", "io_wait", "fragmented_d_state_or_io_wait":
		return true
	default:
		return false
	}
}

// reanchorOnChainStateSeats is one stand-alone re-anchoring pass (its
// unwalked-share decisions are not carried anywhere). A rank build runs two
// passes over one board and carries the first pass's decisions into the
// second through reanchorOnChainStateSeatsCarried.
func reanchorOnChainStateSeats(chain ChainResult, stats WindowStats, items []RootCauseRankItem) []RootCauseRankItem {
	return reanchorOnChainStateSeatsCarried(chain, stats, items, &rspaUnwalkedDecisions{})
}

// reanchorOnChainStateSeatsCarried is the re-anchoring pass of a rank build:
// carried holds the unwalked-share decisions of the board's earlier pass
// (adopted) and receives this pass's (HMC377 fix round 4).
func reanchorOnChainStateSeatsCarried(chain ChainResult, stats WindowStats, items []RootCauseRankItem, carried *rspaUnwalkedDecisions) []RootCauseRankItem {
	if len(items) == 0 || stats.chainAnchorsByPID == nil {
		return items
	}
	runnableDecisions, dioDecisions := buildRSPAFamilyDecisions(chain, stats)
	if len(runnableDecisions) == 0 && len(dioDecisions) == 0 {
		return items
	}
	chainSeats := rspaChainSeatPresenceByPID(items)
	chainRunnableSeatIndex := rspaChainRunnableSeatIndexByPID(items)
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
	// AUD-03 (§14.4, 2026-07-25): the suppressed chain seat may be the pid's
	// VS-1/GAP-B2 PERIODIC account — its discount and timer credential must
	// survive onto the canonical single window owner, or the µs-identical
	// wall clock that was already proven normal timer cadence resurrects at
	// full value on the final board. Transfer ONLY on exact Σ identity with
	// the single owner (the frozen §14.4 direction: 多分区或不等值 fail-close
	// — never copy one aggregate discount onto several partition seats).
	periodicChainDIOAccount := func(pid int) (period, lateness, effective, blocking float64, timerWait bool, timerCaller string, ok bool) {
		for i := range chain.rankAggregateCensus {
			agg := &chain.rankAggregateCensus[i]
			if agg.Thread.PID != pid || !agg.PeriodicSource || !dominantStateIsDStateOrIOWait(agg.DominantState) {
				continue
			}
			return agg.DetectedPeriodMs, agg.LatenessMs, agg.EffectivePeriodicImpactMs,
				aggregateBlockingMs(*agg), agg.PeriodicTimerWait, agg.PeriodicTimerCaller, true
		}
		if impact, present := strongestImpactByPID[pid]; present && impact.PeriodicSource &&
			dominantStateIsDStateOrIOWait(impact.DominantState) {
			return impact.DetectedPeriodMs, impact.LatenessMs, impact.EffectivePeriodicImpactMs,
				causalImpactBlockingMs(impact), impact.PeriodicTimerWait,
				TimerWaitCallerSymbol(impact.DFamilyBlockedCaller), true
		}
		return 0, 0, 0, 0, false, "", false
	}
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
		// AUD-03: the periodic discount rides the SAME transfer — gated on
		// the Σ identity between the suppressed chain account's blocking and
		// this owner's own D/IO account (µs tolerance, the rspaWithinTol
		// authority). A mismatch keeps the owner untouched (fail-close).
		if !item.PeriodicSource {
			if period, lateness, effective, blocking, timerWait, timerCaller, present := periodicChainDIOAccount(item.Thread.PID); present &&
				rspaWithinTol(blocking, item.DStateMs+item.IOWaitMs) {
				item.PeriodicSource = true
				item.DetectedPeriodMs = period
				item.LatenessMs = lateness
				item.EffectiveImpactMs = effective
				item.PeriodicTimerWait = timerWait
				item.PeriodicTimerCaller = timerCaller
				// 复核修 (wf_8fe3fe39, 2026-07-25): §7.30 S1 — the published
				// sort key re-derives with the published value, exactly like
				// the sibling seat rewriters (rspaRewriteSeatToRemainder /
				// rspaClipSeatToAnchored); a stale full-value Score beside a
				// discounted effective is the q4 rank1-score dissonance class.
				item.Score = effective * item.Confidence * rootCauseItemScoreWeight(*item)
			}
		}
	}
	// XLANE-1 件1: the chain-lane runnable seats' own published segment union
	// per pid — the physical-intersection witness for the fully-anchored
	// satellite demotion (computed once per pass; the demotion never mutates
	// the chain seats themselves, so the snapshot stays valid).
	chainRunnableSeatWindows := rspaChainRunnableSeatWindowsByPID(items)
	decomposableRunnableWindowSeat := map[int]bool{}
	for i := range items {
		if items[i].Thread.PID > 0 && strings.TrimSpace(items[i].Type) == "runnable_wait" &&
			strings.TrimSpace(items[i].Source) == "window_stats" {
			decomposableRunnableWindowSeat[items[i].Thread.PID] = true
		}
	}
	// HMC377 fix round 3: the unwalked-share ledger — every migrating
	// thread's census partitioned once into walked / unwalked / outside — and
	// its one owner per (thread, family), decided before any arm runs. The
	// thread's own unwalked seats are minted here and only here; every arm
	// below reads the ledger for its representation check and its sentence,
	// and subtracts the unwalked share only when its owner exists.
	// HMC377 fix round 4: a formal window seat applies the planner's ruling
	// for its row (formalRowScope) — the arms never re-decide; a share the
	// board's earlier pass decided is adopted from carried.
	ledger := buildRSPAUnwalkedLedger(&chain, stats, runnableDecisions, dioDecisions)
	appended := rspaPlanUnwalkedOwners(chain, stats, ledger, items, chainSeats, runnableDecisions, dioDecisions, decomposableRunnableWindowSeat, carried)
	// HMC377 fix round 5: what each chain-lane seat holds (once per board),
	// for the claims the arms below make about the walked share.
	rspaStampChainSeatHolds(items, stats, runnableDecisions, dioDecisions)
	walkedFoldOf := func(pid int) []foldInterval { return rspaWindowsFold(stats.chainAnchorsByPID[pid]) }
	for i := range items {
		item := &items[i]
		if item.ChainAnchorRemainderSeat || item.ChainAnchorFullMs > 0 || item.AbsorbedByRankFamily ||
			item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
			item.inversionAccountRepresentedBy != "" || item.ChainAnchorUnwalkedSeat {
			continue
		}
		if item.Thread.PID <= 0 || !rootCauseItemIsOnChain(*item) {
			continue
		}
		if rspaRowIsSelfExempt(*item, chain.Target) {
			continue
		}
		// STATERES-1 (§40.30 V-STATE-1 plan A, 2026-09-02) lane-decided-once:
		// a seat that already wears a typed on-chain basis (the R3 host-edge
		// state/span bases, the self bases) was adjudicated by its own lane —
		// the ENRICH pass runs this re-anchoring BEFORE the state-edge pass,
		// and a whole-account residual seat (on_chain, ChainAnchorFullMs==0,
		// RSPA decision anchored=0) would otherwise be rewritten into a
		// zero-credential ◇ remainder, silently reverting the ruling on the
		// second pass. Mirrors the query.go keep arm for the same bases.
		if strings.TrimSpace(item.OnChainBasis) != "" {
			continue
		}
		switch strings.TrimSpace(item.Type) {
		case "priority_inversion_candidate":
			// B1260: a sleep/D/IO-dominant dependency may publish a separate
			// scheduling-supply sub-seat.  Its typed GatedRunnableMs (backed by
			// the exact interval inventory carried on familyMemberIntervals) is
			// the runnable account; DominantState and the composite display value
			// are deliberately not used here.  Stamp the legacy RSPA decomposition
			// only when this one row provably owns the COMPLETE anchored runnable
			// census.  A multi-row owner stays unstamped rather than making each
			// fragment claim the whole anchored share.
			decision, ok := runnableDecisions[item.Thread.PID]
			if !ok || !decision.migrate || !decision.identityHolds || item.GatedRunnableMs <= 0 ||
				!decomposableRunnableWindowSeat[item.Thread.PID] {
				continue
			}
			presence := chainSeats[item.Thread.PID]
			if !presence.runnable || !rspaWithinTol(presence.runnableMs, decision.anchoredMs) ||
				!rspaWithinTol(item.GatedRunnableMs, decision.anchoredMs) {
				continue
			}
			item.ChainAnchoredMs = item.GatedRunnableMs
			item.ChainAnchorFullMs = decision.fullMs
			// HMC377 fix round 3: the gated runnable share is the walked hop's
			// ready time only — the thread's time inside the waits the chain
			// did not walk is its own unwalked seat, never part of this
			// inversion estimate; the stamp carries the three typed parts.
			// HMC377 fix round 5: the stamp names the seats holding the other
			// parts (the thread's ◇ remainder, its own unwalked seat) — typed
			// claims resolved against the published board like any sentence's.
			part := ledger.part(item.Thread.PID, false)
			_, outside := rspaCensusInstants(stats, item.Thread.PID, false)
			remainderMs := decision.fullMs - item.GatedRunnableMs
			var claims []RootCauseShareClaim
			if part.ownSeat() {
				rspaStampThreeWay(item, item.GatedRunnableMs, part.unwalkedMs, part.outsideMs, decision.fullMs)
				remainderMs = part.outsideMs
				claims = append(claims, rspaClaim(RootCauseShareClaimUnwalked, RootCauseShareHolderUnwalkedSeat, item.Thread.PID, false, part.unwalkedMs, part.pieces))
				if outside != nil {
					outside = foldIntervalSubtract(outside, rspaFoldCopy(part.ownerSupport))
				}
			}
			if remainderMs > rspaAnchorIdentityTolMs {
				claims = append([]RootCauseShareClaim{rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, item.Thread.PID, false, remainderMs, outside)}, claims...)
			}
			rspaAppendSentence(item, rspaSentence{claims: claims})
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
			// HMC377 fix round 3: the ledger's owner holds the share of this
			// seat inside the waits the chain did not walk (one seat of its own
			// beside a chain seat, or this clipped seat itself without one).
			const family = "runnable (scheduling-pressure candidate)"
			part := ledger.part(item.Thread.PID, false)
			scope := part.formalRowScope(i, item.runnableIntervals, full, anchored)
			presence := chainSeats[item.Thread.PID]
			// HMC377 fix round 5: the instants of each part of this seat's
			// account, for the typed claims its sentences make.
			inst := rspaInstantsOf(item.runnableIntervals, full, walkedFoldOf(item.Thread.PID), part.ownedSupport(scope))
			// RNB-1 B-3: case A requires the chain seat to provably HOLD the
			// anchored account — presence ∧ µs identity (decision) ∧ published
			// seat Σ ≈ census-anchored Σ. A present-but-diverging chain seat is
			// case A' (double-account disclosure, no additive claim).
			caseAOwned := presence.runnable && decision.identityHolds && rspaWithinTol(presence.runnableMs, decision.anchoredMs)
			switch {
			case caseAOwned:
				// Case A: the chain seat owns the anchored portion; this seat
				// publishes the remainder (≈0 → absorbed by the recon arm).
				remainder := rspaClampNonNegative(full - anchored - scope.owned)
				claims := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderChainSeat, anchored, inst.walkedOrNil(), scope, inst.ownedOrNil(), 0)
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					part.remainderSummary(scope, item.Thread, family, remainder, anchored, full, claims))
				rspaHold(item, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
				scope.stamp(item, anchored, remainder, full)
			case presence.runnable:
				// Case A' (RNB-1, §29.88 R2): the window seat still rides ◇ as
				// the remainder (the census bipartition is exact regardless),
				// the chain seat stays published as its own ⛓ representative,
				// and the typed double-Σ disclosure replaces the additive claim.
				remainder := rspaClampNonNegative(full - anchored - scope.owned)
				claims := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderChainSeat, 0, nil, scope, inst.ownedOrNil(), presence.runnableMs)
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					part.divergentSummary(scope, item.Thread, family, remainder, anchored, full, presence.runnableMs, decision.anchoredMs, claims))
				rspaHold(item, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
				scope.stamp(item, anchored, remainder, full)
				item.ChainAnchorOwnershipDivergent = true
				item.ChainAnchorChainLaneMs = presence.runnableMs
				item.ChainAnchorCensusMs = decision.anchoredMs
			default:
				// Case B (no chain seat): this seat holds the walked share and,
				// when the ledger says so, the unwalked share too — one object.
				inside := anchored + scope.owned
				remainder := rspaClampNonNegative(full - inside)
				claims := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderClippedSeat, inside, inst.inside(), rspaRowScope{}, nil, 0)
				switch {
				case remainder <= rspaAnchorIdentityTolMs:
					// Case B, fully anchored: the seat is entirely credentialed —
					// keep the legacy on-chain publication byte-identically.
					item.rspaUnwalkedHeldMs = scope.owned
					rspaHold(item, RootCauseShareHolderClippedSeat, false, inst.inside(), inside)
				case inside <= rspaAnchorIdentityTolMs:
					// Case B, zero credential: the whole account is remainder.
					rspaRewriteSeatToRemainder(item, inside, full, remainder, 0, 0,
						part.remainderSummary(scope, item.Thread, family, remainder, inside, full, claims))
					rspaHold(item, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
				default:
					// Case B split: this seat becomes the ⛓ anchored seat; the
					// remainder twin mints beside it on the ◇ lane.
					twin := rspaCloneAsRemainderSeat(*item, inside, full, remainder, 0, 0,
						part.remainderSummary(scope, item.Thread, family, remainder, inside, full, claims))
					twin.rspaUnwalkedHeldMs = scope.owned
					rspaHold(&twin, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
					appended = append(appended, twin)
					rspaClipSeatToAnchored(item, inside, full, inside, 0, 0,
						part.anchoredSummary(scope, item.Thread, "runnable", inside, full, anchored,
							rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, item.Thread.PID, false, remainder, inst.restOrNil())))
					item.rspaUnwalkedHeldMs = scope.owned
					rspaHold(item, RootCauseShareHolderClippedSeat, false, inst.inside(), inside)
				}
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
			// HMC377 R-377-4 (owner ruling, §376 A audit failure 1): one
			// physical thread, one object. The ready time of this window
			// account inside the thread's dependency windows is the same time
			// a chain seat of the thread publishes (case A: the chain lane
			// already holds exactly one runnable seat of the pid) or must
			// publish (case B: no chain runnable seat — the anchored share is
			// minted as a plain ⛓ ready-time seat, the runnable_wait arm's
			// clip). Either way the window account must not ride beside it as
			// a separate time-adjacent "no chain credential" candidate: it
			// keeps every value (the overlap measurement is still never split)
			// and becomes that seat's lossless detail
			// (reconcileInversionAccountsRepresentedOnChain, the B4 absorbed
			// carrier). The ready time outside the windows is different time:
			// the plain ◇ remainder twin, exactly as for an un-retyped window
			// seat. Several chain runnable seats (no unique representative),
			// or an un-retyped window runnable seat of the same pid that owns
			// the same census account (case B would mint the anchored share
			// twice), keep the former whole-seat demotion below.
			// HMC377 fix round 3: the ready time inside the waits the chain did
			// not walk belongs to the ledger's owner — the thread's own plain
			// ready-time seat beside a chain seat (never the inversion
			// estimate of the walked hop), or the clipped plain seat without
			// one. A same-pid un-retyped window runnable seat owns the same
			// census account and its arm prices the share.
			const family = "runnable (scheduling-pressure candidate)"
			part := ledger.part(item.Thread.PID, false)
			scope := part.formalRowScope(i, item.runnableIntervals, full, anchored)
			inst := rspaInstantsOf(item.runnableIntervals, full, walkedFoldOf(item.Thread.PID), part.ownedSupport(scope))
			if owners := chainRunnableSeatIndex[item.Thread.PID]; anchored > rspaAnchorIdentityTolMs &&
				(len(owners) == 1 || (len(owners) == 0 && !decomposableRunnableWindowSeat[item.Thread.PID])) {
				twin := stateEdgePlainRunnableCopy(*item)
				twin.Rank, twin.BackgroundRank, twin.Tier = 0, 0, ""
				presence := chainSeats[item.Thread.PID]
				key := inversionAccountKey(*item)
				var represented rspaSentence
				switch {
				case len(owners) == 1 && presence.runnable && decision.identityHolds &&
					rspaWithinTol(presence.runnableMs, decision.anchoredMs):
					remainder := rspaClampNonNegative(full - anchored - scope.owned)
					claims := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderChainSeat, anchored, inst.walkedOrNil(), scope, inst.ownedOrNil(), 0)
					rspaRewriteSeatToRemainder(&twin, anchored, full, remainder, 0, 0,
						part.remainderSummary(scope, item.Thread, family, remainder, anchored, full, claims))
					scope.stamp(&twin, anchored, remainder, full)
					rspaHold(&twin, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
					items[owners[0]].inversionAccountRepresents = key
					claims.remainder = rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, item.Thread.PID, false, remainder, inst.restOrNil())
					represented = part.inversionRepresentedSummary(scope, anchored, remainder, full, claims)
				case len(owners) == 1:
					remainder := rspaClampNonNegative(full - anchored - scope.owned)
					claims := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderChainSeat, 0, nil, scope, inst.ownedOrNil(), presence.runnableMs)
					rspaRewriteSeatToRemainder(&twin, anchored, full, remainder, 0, 0,
						part.divergentSummary(scope, item.Thread, family, remainder, anchored, full, presence.runnableMs, decision.anchoredMs, claims))
					scope.stamp(&twin, anchored, remainder, full)
					rspaHold(&twin, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
					twin.ChainAnchorOwnershipDivergent = true
					twin.ChainAnchorChainLaneMs = presence.runnableMs
					twin.ChainAnchorCensusMs = decision.anchoredMs
					items[owners[0]].inversionAccountRepresents = key
					detail := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderChainSeat, anchored, inst.walkedOrNil(), scope, inst.ownedOrNil(), 0)
					detail.remainder = rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, item.Thread.PID, false, remainder, inst.restOrNil())
					represented = part.inversionRepresentedSummary(scope, anchored, remainder, full, detail)
				default:
					// Case B: the clipped plain ready-time seat holds the walked
					// share and the ledger's unwalked share as one object.
					inside := anchored + scope.owned
					remainder := rspaClampNonNegative(full - inside)
					remainderClaim := rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, item.Thread.PID, false, remainder, inst.restOrNil())
					seat := stateEdgePlainRunnableCopy(*item)
					seat.Rank, seat.BackgroundRank, seat.Tier = 0, 0, ""
					anchoredSentence := part.anchoredSummary(scope, item.Thread, "runnable", inside, full, anchored, remainderClaim)
					anchoredSentence.text += stateEdgeInversionDetailClause(item.EffectiveImpactMs, item.RunnableMs)
					rspaClipSeatToAnchored(&seat, inside, full, inside, 0, 0, anchoredSentence)
					seat.inversionAccountRepresents = key
					seat.rspaUnwalkedHeldMs = scope.owned
					rspaHold(&seat, RootCauseShareHolderClippedSeat, false, inst.inside(), inside)
					appended = append(appended, seat)
					claims := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderClippedSeat, inside, inst.inside(), rspaRowScope{}, nil, 0)
					rspaRewriteSeatToRemainder(&twin, inside, full, remainder, 0, 0,
						part.remainderSummary(scope, item.Thread, family, remainder, inside, full, claims))
					twin.rspaUnwalkedHeldMs = scope.owned
					rspaHold(&twin, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
					represented = rspaInversionRepresentedSummary(inside, full, claims.anchored, remainderClaim)
				}
				appended = append(appended, twin)
				item.inversionAccountRepresentedBy = key
				item.Causality = "adjacent_to_wakeup_chain"
				item.ChainRelevance = "adjacent"
				rspaAppendSentence(item, represented)
				continue
			}
			item.ChainCredentialLaneDemoted = true
			item.Causality = "adjacent_to_wakeup_chain"
			item.ChainRelevance = "adjacent"
			rspaAppendSentence(item, part.laneDemotionSummary(anchored, full,
				"the priority-inversion overlap is a same-CPU displacement measurement that cannot be split along that boundary"))
		case "cpu_affinity_or_cpuset":
			// RNB-1 R4 lane arm (§29.88 R4; INV-C B-2): the affinity satellite
			// duplicates the thread's runnable wait on the capped top basis and
			// carries NO per-row interval inventory — it can never prove its
			// own anchored share, so a pid with ANY unanchored census share
			// demotes the whole row to ◇ (values untouched; the formal window/
			// chain seats own the bisected accounts). A fully-anchored pid
			// (census remainder ≤ tol) keeps the chain lane byte-identically.
			// HMC377 fix round 3: a share inside the waits the chain did not
			// walk is never "anchored" here — its owner is another seat, so a
			// row that would stand beside both seats on the chain lane rides
			// ◇ whole like any other unanchored account.
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
			rspaAppendSentence(item, ledger.part(item.Thread.PID, false).laneDemotionSummary(decision.anchoredMs, decision.fullMs,
				"this affinity/cpuset satellite carries no interval inventory of its own"))
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
			part := ledger.part(item.Thread.PID, false)
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
				rspaAppendSentence(item, part.laneDemotionSummary(decision.anchoredMs, decision.fullMs,
					"this satellite's own interval inventory cannot prove its anchored share"))
				continue
			}
			full := item.RunnableMs
			if anchored > full+rspaAnchorIdentityTolMs {
				continue
			}
			// HMC377 fix round 3: the satellite's time inside the waits the
			// chain did not walk is held by the ledger's owner (a satellite
			// never mints a seat); it leaves this row only when that owner
			// exists, and the representation check reads the owner's support.
			scope := part.rowScope(rowIntervals, full, anchored, true)
			remainder := rspaClampNonNegative(full - anchored - scope.owned)
			// HMC377 fix round 5: the satellite's instants per part, and the
			// role that holds its walked share — the chain seat when one
			// holds the thread's walked account (case A), the clipped window
			// seat otherwise (case B).
			inst := rspaInstantsOf(rowIntervals, full, walkedFoldOf(item.Thread.PID), part.ownedSupport(scope))
			walkedRole := RootCauseShareHolderChainSeat
			if !chainSeats[item.Thread.PID].runnable {
				walkedRole = RootCauseShareHolderClippedSeat
			}
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
				seatWindows := chainRunnableSeatWindows[item.Thread.PID]
				if scope.owned > rspaAnchorIdentityTolMs {
					// HMC377 fix round 3: the share inside the unwalked waits
					// is represented by its owner (the ledger's support —
					// minted in this same pass or an earlier one).
					seatWindows = mergeAnchorTimeWindows(append(append([]TimeWindow(nil), seatWindows...), rspaFoldWindows(part.ownerSupport)...))
				}
				if len(seatWindows) > 0 && rspaWithinTol(rspaIntervalsOverlapMs(seatWindows, rowIntervals), full) {
					// HMC377 fix round 5: the ledger's planned holders prove the
					// demotion here (the lane decision); the published board
					// decides whether the row wears the represented form — the
					// sentence's claims resolve against it once, at the end.
					item.ChainAnchorRepresentedByChainSeat = true
					item.Causality = "adjacent_to_wakeup_chain"
					item.ChainRelevance = "adjacent"
					claims := rspaRowClaimsOf(item.Thread.PID, false, walkedRole, anchored, inst.walkedOrNil(), scope, inst.ownedOrNil(), 0)
					if part.oneObject(scope) {
						claims = rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderClippedSeat, anchored+scope.owned, inst.inside(), rspaRowScope{}, nil, 0)
					}
					rspaAppendSentence(item, part.representedSummary(scope, anchored, full, claims))
					continue
				}
				if scope.owned > rspaAnchorIdentityTolMs {
					// Part of this satellite's instants is held by the owner:
					// standing on the chain lane beside it would price them
					// twice, and the walked rest is not proven covered.
					item.ChainCredentialLaneDemoted = true
					item.Causality = "adjacent_to_wakeup_chain"
					item.ChainRelevance = "adjacent"
					rspaAppendSentence(item, rspaLaneDemotionSentenceThreeWay(anchored, scope.owned, full, part.clause(), scope,
						rspaClaim(RootCauseShareClaimUnwalked, scope.unwalkedRole(), item.Thread.PID, false, scope.owned, inst.ownedOrNil()),
						"this satellite's own interval inventory cannot prove that the chain seats hold its walked share"))
				}
				continue
			}
			// RNB-1: a divergent-ownership pid's satellite speaks the honest
			// double-account form too — its legacy sentence claims the chain
			// seat owns the anchored share, which case A' cannot claim.
			if presence := chainSeats[item.Thread.PID]; presence.runnable &&
				!(decision.identityHolds && rspaWithinTol(presence.runnableMs, decision.anchoredMs)) {
				claims := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderChainSeat, 0, nil, scope, inst.ownedOrNil(), presence.runnableMs)
				rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
					part.divergentSummary(scope, item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full, presence.runnableMs, decision.anchoredMs, claims))
				rspaHold(item, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
				scope.stamp(item, anchored, remainder, full)
				item.ChainAnchorOwnershipDivergent = true
				item.ChainAnchorChainLaneMs = presence.runnableMs
				item.ChainAnchorCensusMs = decision.anchoredMs
				continue
			}
			if part.oneObject(scope) {
				// Case B: the clipped window seat holds walked + unwalked.
				claims := rspaRowClaimsOf(item.Thread.PID, false, RootCauseShareHolderClippedSeat, anchored+scope.owned, inst.inside(), rspaRowScope{}, nil, 0)
				rspaRewriteSeatToRemainder(item, anchored+scope.owned, full, remainder, 0, 0,
					part.remainderSummary(scope, item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored+scope.owned, full, claims))
				rspaHold(item, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
				continue
			}
			claims := rspaRowClaimsOf(item.Thread.PID, false, walkedRole, anchored, inst.walkedOrNil(), scope, inst.ownedOrNil(), 0)
			rspaRewriteSeatToRemainder(item, anchored, full, remainder, 0, 0,
				part.remainderSummary(scope, item.Thread, "runnable (scheduling-pressure candidate)", remainder, anchored, full, claims))
			rspaHold(item, RootCauseShareHolderRemainderSeat, false, inst.restOrNil(), remainder)
			scope.stamp(item, anchored, remainder, full)
		case "fragmented_runnable_wait":
			decision, ok := runnableDecisions[item.Thread.PID]
			if !ok || !decision.migrate || item.Source != "window_stats.state_churn" {
				continue
			}
			full := item.RunnableMs
			if full <= 0 {
				continue
			}
			rspaRewritePIDLevelTwin(item, ledger.part(item.Thread.PID, false), decision, chainSeats[item.Thread.PID].runnable,
				chainSeats[item.Thread.PID].runnableMs, full, "fragmented runnable (scheduling-pressure candidate)", false, stats)
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
			// HMC377 fix round 3: the ledger's owner holds this seat's D / IO
			// time inside the waits the chain did not walk, per channel.
			const family = "D/IO blocking"
			part := ledger.part(item.Thread.PID, true)
			scope := part.formalRowScope(i, item.dioSegmentIntervals, full, anchored)
			ownedD, ownedIO, channelsOK := part.rowOwnedChannels(scope, item.dioSegmentIntervalsD, item.dioSegmentIntervalsIO)
			if !channelsOK {
				// The channel inventories do not reproduce the owned share:
				// the row keeps it (nothing is handed to an owner it cannot
				// split per channel).
				scope = rspaRowScope{walkedOnly: true, unpriced: -1}
			}
			presence := chainSeats[item.Thread.PID]
			// HMC377 fix round 5: the instants of each part of this seat's
			// account (both channels — one thread's D and IO segments never
			// overlap, so a partition's instants name that partition's share).
			// HMC377 fix round 6: a partition seat minted from proof slices
			// carries no dioSegmentIntervals; its claims take the partition's
			// own instants when the slices carried a complete segment list
			// (otherwise they stay windowless and the typed partition identity
			// alone keeps them inside their partition).
			instants := item.dioSegmentIntervals
			if len(instants) == 0 {
				instants = item.dioPartitionInstants
			}
			inst := rspaInstantsOf(instants, full, walkedFoldOf(item.Thread.PID), part.ownedSupport(scope))
			// RNB-1 B-3: same case-A ownership qualification as the runnable
			// arm (presence ∧ decision identity ∧ published seat Σ ≈ anchored).
			caseAOwned := presence.dio && decision.identityHolds && rspaWithinTol(presence.dioMs, decision.anchoredMs)
			switch {
			case caseAOwned:
				remainderD := rspaClampNonNegative(item.DStateMs - anchoredD - ownedD)
				remainderIO := rspaClampNonNegative(item.IOWaitMs - anchoredIO - ownedIO)
				remainder := remainderD + remainderIO
				claims := rspaRowClaimsOf(item.Thread.PID, true, RootCauseShareHolderChainSeat, anchored, inst.walkedOrNil(), scope, inst.ownedOrNil(), 0)
				rspaRewriteSeatToRemainder(item, anchored, full, 0, remainderD, remainderIO,
					part.remainderSummary(scope, item.Thread, family, remainder, anchored, full, claims))
				rspaHold(item, RootCauseShareHolderRemainderSeat, true, inst.restOrNil(), remainder)
				scope.stamp(item, anchored, remainder, full)
			case presence.dio:
				// Case A' (RNB-1): remainder migration + typed double-Σ
				// disclosure; the chain seat keeps its own ⛓ account (the M-D
				// mint suppression never fires on divergent pids).
				remainderD := rspaClampNonNegative(item.DStateMs - anchoredD - ownedD)
				remainderIO := rspaClampNonNegative(item.IOWaitMs - anchoredIO - ownedIO)
				remainder := remainderD + remainderIO
				claims := rspaRowClaimsOf(item.Thread.PID, true, RootCauseShareHolderChainSeat, 0, nil, scope, inst.ownedOrNil(), presence.dioMs)
				rspaRewriteSeatToRemainder(item, anchored, full, 0, remainderD, remainderIO,
					part.divergentSummary(scope, item.Thread, family, remainder, anchored, full, presence.dioMs, decision.anchoredMs, claims))
				rspaHold(item, RootCauseShareHolderRemainderSeat, true, inst.restOrNil(), remainder)
				scope.stamp(item, anchored, remainder, full)
				item.ChainAnchorOwnershipDivergent = true
				item.ChainAnchorChainLaneMs = presence.dioMs
				item.ChainAnchorCensusMs = decision.anchoredMs
			default:
				insideD, insideIO := anchoredD+ownedD, anchoredIO+ownedIO
				inside := insideD + insideIO
				remainderD := rspaClampNonNegative(item.DStateMs - insideD)
				remainderIO := rspaClampNonNegative(item.IOWaitMs - insideIO)
				remainder := remainderD + remainderIO
				claims := rspaRowClaimsOf(item.Thread.PID, true, RootCauseShareHolderClippedSeat, inside, inst.inside(), rspaRowScope{}, nil, 0)
				switch {
				case remainder <= rspaAnchorIdentityTolMs:
					// Case B, fully anchored: the seat keeps its full publication
					// and (single-seat pids) inherits the suppressed chain row's
					// typed context — it IS the pid's one D-IO seat now.
					item.rspaUnwalkedHeldMs = scope.owned
					rspaHold(item, RootCauseShareHolderClippedSeat, true, inst.inside(), inside)
					transferChainDIOContext(item)
				case inside <= rspaAnchorIdentityTolMs:
					rspaRewriteSeatToRemainder(item, inside, full, 0, remainderD, remainderIO,
						part.remainderSummary(scope, item.Thread, family, remainder, inside, full, claims))
					rspaHold(item, RootCauseShareHolderRemainderSeat, true, inst.restOrNil(), remainder)
				default:
					twin := rspaCloneAsRemainderSeat(*item, inside, full, 0, remainderD, remainderIO,
						part.remainderSummary(scope, item.Thread, family, remainder, inside, full, claims))
					twin.rspaUnwalkedHeldMs = scope.owned
					rspaHold(&twin, RootCauseShareHolderRemainderSeat, true, inst.restOrNil(), remainder)
					appended = append(appended, twin)
					rspaClipSeatToAnchored(item, inside, full, 0, insideD, insideIO,
						part.anchoredSummary(scope, item.Thread, family, inside, full, anchored,
							rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, item.Thread.PID, true, remainder, inst.restOrNil())))
					item.rspaUnwalkedHeldMs = scope.owned
					rspaHold(item, RootCauseShareHolderClippedSeat, true, inst.inside(), inside)
					transferChainDIOContext(item)
				}
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
			rspaRewritePIDLevelTwin(item, ledger.part(item.Thread.PID, true), decision, chainSeats[item.Thread.PID].dio,
				chainSeats[item.Thread.PID].dioMs, full, "fragmented D/IO blocking", true, stats)
		}
	}
	items = append(items, appended...)
	// S14-A2 (§14.4 fail-close residual, 2026-07-25): when a pid's suppressed
	// chain PERIODIC D∧timer account could NOT ride the single-owner transfer
	// (multi-partition window seats / Σ mismatch / caseAOwned remainder
	// rewrites), the periodic credential and its discounted value would die
	// with the mint-time suppression while the partition seats advertise the
	// µs-identical wall clock at FULL value. The frozen §14.4 direction:
	// fail-close = RESURRECT the chain periodic seat with a typed
	// dual-account disclosure — never copy one aggregate discount onto
	// several partition seats. Idempotent across the double reanchor pass:
	// a resurrected (or transferred) seat carries PeriodicSource, which the
	// carried-scan below treats as done.
	for pid, decision := range dioDecisions {
		if !decision.migrate || !decision.identityHolds {
			// Suppression never fired (divergent pids keep their chain seat
			// with the discount on it) — nothing to resurrect.
			continue
		}
		period, lateness, effective, _, timerWait, timerCaller, present := periodicChainDIOAccount(pid)
		if !present || !timerWait {
			continue
		}
		carried := false
		for i := range items {
			if items[i].Thread.PID != pid {
				continue
			}
			switch strings.TrimSpace(items[i].Type) {
			case "d_state_or_io_wait", "io_wait", "fragmented_d_state_or_io_wait":
				if items[i].PeriodicSource {
					carried = true
				}
			}
		}
		if carried {
			continue
		}
		resurrected, ok := rspaResurrectPeriodicChainSeat(chain, pid)
		if !ok {
			continue
		}
		resurrected.PeriodicSource = true
		resurrected.DetectedPeriodMs = period
		resurrected.LatenessMs = lateness
		resurrected.EffectiveImpactMs = effective
		resurrected.PeriodicTimerWait = true
		resurrected.PeriodicTimerCaller = timerCaller
		resurrected.Score = effective * resurrected.Confidence * rootCauseItemScoreWeight(resurrected)
		// Typed dual-account disclosure (the A' double-Σ vocabulary),
		// OUTCOME-NEUTRAL at reanchor time (§15.12 批戊): whether the raw
		// partition rows end up absorbed is decided LATER by the RSPA-MP1
		// reconciliation — on its fail-open arms the raw rows stay ACTIVE
		// competing seats, so the sole-owner/absorbed claim may only be
		// written by the committed absorption itself.
		resurrected.ChainAnchorOwnershipDivergent = true
		resurrected.ChainAnchorChainLaneMs = decision.chainLaneMs
		resurrected.ChainAnchorCensusMs = decision.anchoredMs
		resurrected.Summary += fmt.Sprintf("; dual account: the pid's raw window/partition D-IO rows publish this same physical wall clock (census-anchored %.3fms) — never add the accounts; this seat carries the periodic timer-wait credential (caller=%s) plus discounted attribution %.3fms", decision.anchoredMs, timerCaller, effective)
		items = append(items, resurrected)
	}
	return items
}

// rspaResurrectPeriodicChainSeat re-mints the suppressed chain periodic D∧
// timer seat at the owner-decision altitude (S14-A2): the census aggregate
// is the preferred carrier (it already holds the reconciled discount), the
// single periodic impact the fallback. The mint-time RootEvidence twin
// seeding stays valid — this is a deliberate owner-decision publication,
// not a lane resurrecting the same occurrence by accident.
func rspaResurrectPeriodicChainSeat(chain ChainResult, pid int) (RootCauseRankItem, bool) {
	for i := range chain.rankAggregateCensus {
		agg := &chain.rankAggregateCensus[i]
		if agg.Thread.PID == pid && agg.PeriodicSource && agg.PeriodicTimerWait &&
			dominantStateIsDStateOrIOWait(agg.DominantState) {
			return rootCauseItemFromCausalAggregate(*agg), true
		}
	}
	for i := range chain.CausalImpacts {
		impact := &chain.CausalImpacts[i]
		if impact.Thread.PID == pid && impact.PeriodicSource && impact.PeriodicTimerWait &&
			impact.ChainDepth > 0 && dominantStateIsDStateOrIOWait(impact.DominantState) {
			return rootCauseItemFromCausalImpact(*impact), true
		}
	}
	return RootCauseRankItem{}, false
}

// rspaSummaryOwnedByChainSeat / rspaSummaryRemainderTwinPublished are the
// held forms of the two co-publication claims the account sentences make
// (the remainder's anchored share is owned by its ⛓ holder; the ⛓ seat's
// remainder is published as a separate ◇ seat), and the *Unpublished forms
// their compaction disclosures. HMC377 fix round 5: each phrase is a typed
// claim span (rspaSentence.span) resolved against the published board by
// resolveRankShareClaims — the former D1 修复轮 pass that rewrote these
// verbatim anchors on family-level presence is retired.
const rspaSummaryOwnedByChainSeat = "(owned by the chain seat)"
const rspaSummaryOwnedByChainSeatUnpublished = "(the owning seat is not on the published board; see the compaction disclosure)"
const rspaSummaryOwnedByChainSeatContext = "(the owning row " + rspaHolderContextOnly + ")"

// rspaSummaryOwnedBySuccessor / rspaSummaryOwnedReturned (HMC377 round 11,
// R-377-11 ①): the owner phrase when the publication gate withdrew the
// owning seat and the anchored share passed to its published successor —
// another row of this thread, or this ◇ row itself, which then counts it with
// its own value. rspaOwnerMixedForm is the phrase when the share ends split.
const rspaSummaryOwnedBySuccessor = "(the owning row " + rspaHolderContextOnly + "; another row of this thread on the published board counts that share)"
const rspaSummaryOwnedReturned = "(the owning row " + rspaHolderContextOnly + ", so this row counts that share with its own value)"

// rspaOwnerMixedForm names each part of an anchored share that ended split
// (HMC377 round 11): the part a chain seat on the board owns (or that another
// row of this thread counts as a withdrawn seat's successor), the part that
// passed back to this row, and the part whose owner is not on the board.
func rspaOwnerMixedForm(st rspaSpanState) string {
	var parts []string
	if st.resolvedMs > rspaAnchorIdentityTolMs {
		if st.successor {
			parts = append(parts, fmt.Sprintf("%.3fms of it counted by another row of this thread on the published board", st.resolvedMs))
		} else {
			parts = append(parts, fmt.Sprintf("%.3fms of it owned by the chain seat", st.resolvedMs))
		}
	}
	parts = append(parts, fmt.Sprintf("%.3fms whose owning row %s, so this row counts them with its own value", st.returnedMs, rspaHolderContextOnly))
	if st.restMs > rspaAnchorIdentityTolMs {
		if st.restWithdrawn {
			parts = append(parts, fmt.Sprintf("%.3fms whose owning row %s", st.restMs, rspaHolderContextOnly))
		} else {
			parts = append(parts, fmt.Sprintf("%.3fms whose owning seat is not on the published board (see the compaction disclosure)", st.restMs))
		}
	}
	return "(" + strings.Join(parts, "; ") + ")"
}

// rspaSummaryNoAnchoredShare — RNB-2 件3 (§29.88 W3 病③, 2026-07-15): the
// zero-anchored remainder form. No seat holds any anchored share, so neither
// ownership form above may render (both would name a nonexistent holder).
const rspaSummaryNoAnchoredShare = "(no anchored share exists — no seat holds it)"
const rspaSummaryRemainderTwinPublished = "(published as a separate adjacent seat)"
const rspaSummaryRemainderTwinUnpublished = "(its adjacent remainder seat is not on the published board; see the compaction disclosure)"

// rspaRunnableLedgerFallbackCaveatPrefix is the RUNSPLIT-1 件3 disclosure
// sentinel (§29.209 ③, 2026-07-22): the runnable-lane ledger-fallback
// population count — the precise 席名集合并 replace/append key.
const rspaRunnableLedgerFallbackCaveatPrefix = "runnable_ledger_fallback:"

// hasRunnableLedgerFallbackCaveat reports whether a fallback disclosure
// sentence is already present, matched by its sentinel prefix.
//
// DISPFIX-1 件1 已实现席名集合并 (§29.213 排期件5, 2026-07-23): the former F-4
// 记档候办 — the enrich re-publication DROPPING its sentence on a prefix
// collision and swallowing a fallback seat the build lane never named — is
// CLOSED. The enrich lane now MERGES the fallback seat-name SETS (build 车道
// 席集 ∪ enrich 车道席集) and REPLACES the build sentence with the union-
// rendered one (mergeRunnableLedgerFallbackCaveat → replaceOrAppendCaveatByPrefix;
// the build set is carried truncation-robustly on
// RootCauseRankResult.runnableFallbackLabels). That migration removed this
// predicate's ONLY production caller (the former `&& !hasRunnableLedgerFallbackCaveat(...)`
// enrich-lane emit guard), so it now has NO production role — it is retained
// solely as a test-only presence helper (the RUNSPLIT disclosure pins assert
// present/absent). Contrast hasLockSeatRankFailLoudCaveat (query.go), the
// sibling lane that still runs a LIVE has-check dedup rather than migrating to
// replaceOrAppendCaveatByPrefix.
func hasRunnableLedgerFallbackCaveat(caveats []string) bool {
	for _, caveat := range caveats {
		if strings.HasPrefix(caveat, rspaRunnableLedgerFallbackCaveatPrefix) {
			return true
		}
	}
	return false
}

// mergeRunnableLedgerFallbackCaveat folds ONE publication lane's fallback set
// into the running disclosure (DISPFIX-1 件1, §29.213 排期件5): union the
// carried set (the prior lane's) with THIS lane's fallback labels, re-render,
// and replace any existing fallback sentence in place (or append). Returns the
// updated caveats and the merged set (the caller carries it to the next lane).
// The empty union leaves caveats untouched (single-lane forms stay byte-
// identical). The inversion sub-clause count is recomputed over the union by
// the renderer, so a build seat X and a DIFFERENT enrich seat Y both name their
// seats with an honest per-union inversion count.
func mergeRunnableLedgerFallbackCaveat(caveats, carried, laneLabels []string) ([]string, []string) {
	merged := mergeStableDedupLabels(carried, laneLabels)
	sentence := renderRunnableLedgerFallbackCaveat(merged)
	if sentence == "" {
		return caveats, merged
	}
	return replaceOrAppendCaveatByPrefix(caveats, rspaRunnableLedgerFallbackCaveatPrefix, sentence), merged
}

// rspaRunnableLedgerFallbackCaveat — RUNSPLIT-1 件3 (§29.209 user ruling ③,
// 2026-07-22): the no-ledger fallback keeps its FULL value with the existing
// credential word (维持全额, 禁估算 — inventing an estimated split is
// forbidden), and the fallback POPULATION is disclosed at the report level
// (回退面席数披露): how many on-chain runnable member seats kept full-window
// values because the RSPA runnable ledger holds no usable anchored-share
// record for them.
//
// Scope (all precise typed signals):
//   - armed boards only: stats.chainAnchorsByPID != nil — a board with no
//     anchor sweep at all (legacy/direct-literal WindowStats, chainless
//     queries) is the §29.61.10 documented fail-open boundary, not a per-seat
//     ledger miss; it stays silent byte-identically.
//   - the §29.209 件1 seat family only: basis-less membership-admitted window
//     runnable member seats (runnable_wait window_stats / fragmented_runnable_
//     wait state_churn), INCLUDING the window seat's in-place priority-
//     inversion recast (修复轮 FIX-1, §29.209 复核对抗席 F1, 2026-07-22: the
//     mint site stamps the ledger fields BEFORE the recast and the reanchor
//     dispatch consumes them through the same no-decision / stamp-less-non-
//     identity / unusable-split fallback arms — same physical account, so the
//     recast face counts under the identical predicate; because its ranking
//     weight is ALREADY reduced to the directly measured overlap, the
//     sentence face carries an honest sub-clause instead of the bare
//     full-window-keep claim). Seats carrying a typed OnChainBasis (host-edge
//     / self) ride their own credential lanes; satellites carry their own R4
//     lane arms (§29.88); wakeup_chain-source rows are constructive mints
//     (value already inside edge-closed windows).
//   - fallback ⇔ the reanchor dispatch left the seat at its full value
//     because no usable ledger account existed: no per-pid decision (no typed
//     jump window / census skip / MAX-fallback board), the stamp-less
//     non-identity legacy form (the dispatch default arm), or a structurally
//     unusable split (anchored > full).
//
// Pure disclosure: one advisory caveat, zero value/lane/ordinal movement
// (soft guidance on a precise signal; never a gate).
//
// DISPFIX-1 件1 (§29.213 排期件5, 2026-07-23): the classification loop moved to
// rspaRunnableLedgerFallbackLabels (returning the FULL uncapped fallback set)
// and the sentence to renderRunnableLedgerFallbackCaveat, so both publication
// lanes render from the UNION of their sets (席名集合并). This wrapper keeps
// the single-lane string API (call sites / pins) byte-identical.
func rspaRunnableLedgerFallbackCaveat(chain ChainResult, stats WindowStats, items []RootCauseRankItem) string {
	return renderRunnableLedgerFallbackCaveat(rspaRunnableLedgerFallbackLabels(chain, stats, items))
}

// rspaRunnableLedgerFallbackLabels returns the FULL ordered fallback seat-name
// set (uncapped, board order) for ONE board — see rspaRunnableLedgerFallbackCaveat
// for the scope narrative. DISPFIX-1 件1 席名集合并 input: the enrich lane
// unions this with the build lane's carried set.
func rspaRunnableLedgerFallbackLabels(chain ChainResult, stats WindowStats, items []RootCauseRankItem) []string {
	if stats.chainAnchorsByPID == nil {
		return nil
	}
	runnableDecisions, _ := buildRSPAFamilyDecisions(chain, stats)
	var labels []string
	for i := range items {
		item := &items[i]
		if item.Thread.PID <= 0 || item.AbsorbedByRankFamily ||
			item.ChainAnchorRemainderSeat || item.ChainAnchorFullMs > 0 ||
			item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat {
			continue
		}
		if item.RunnableMs <= 0 || strings.TrimSpace(item.OnChainBasis) != "" {
			continue
		}
		if !rootCauseItemIsOnChain(*item) || rspaRowIsSelfExempt(*item, chain.Target) {
			continue
		}
		fallback := false
		switch strings.TrimSpace(item.Type) {
		case "runnable_wait", "priority_inversion_runnable_wait":
			// 修复轮 FIX-1: ONE predicate for both type faces — the inversion
			// form is the same window census seat recast in place at the mint
			// site (same ledger stamp, same dispatch fallback arms); counting
			// only one face made the disclosure population depend on a rank
			// retype that never touches the ledger. A recast seat the dispatch
			// DID resolve either keeps a proven fully-anchored chain lane (not
			// a fallback) or lane-demotes whole (ChainCredentialLaneDemoted —
			// excluded by the top gate above).
			if item.Source != "window_stats" {
				continue
			}
			decision, ok := runnableDecisions[item.Thread.PID]
			switch {
			case !ok || !decision.migrate:
				fallback = true
			case !item.ledgerAnchorStamped && !rspaWithinTol(item.RunnableMs, decision.fullMs):
				fallback = true
			case item.ledgerAnchorStamped && item.ledgerAnchoredRunnableMs > item.RunnableMs+rspaAnchorIdentityTolMs:
				fallback = true
			}
		case "fragmented_runnable_wait":
			if item.Source != "window_stats.state_churn" {
				continue
			}
			decision, ok := runnableDecisions[item.Thread.PID]
			fallback = !ok || !decision.migrate
		default:
			continue
		}
		if !fallback {
			continue
		}
		labels = append(labels, fmt.Sprintf("%s %s", item.Type, threadLabel(item.Thread)))
	}
	return labels
}

// renderRunnableLedgerFallbackCaveat formats the fallback-population disclosure
// from a fallback seat-name set (DISPFIX-1 件1 single word-face shared by both
// lanes' render + 席名集合并 re-render). "" for the empty set. count = |set|;
// the priority-inversion sub-clause count is recomputed from the set's own Type
// tokens — labels are "<Type> <threadLabel>" and every Type is space-free, so a
// "priority_inversion_runnable_wait " prefix (with the delimiting space) is the
// precise inversion signal (no regex). A single-lane set yields the byte-
// identical legacy sentence (负臂).
func renderRunnableLedgerFallbackCaveat(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	count := len(labels)
	inversionKept := 0
	for _, label := range labels {
		if strings.HasPrefix(label, "priority_inversion_runnable_wait ") {
			inversionKept++
		}
	}
	named := labels
	suffix := ""
	if count > 4 {
		named = labels[:4]
		suffix = fmt.Sprintf(" and %d more", count-4)
	}
	// 修复轮 FIX-3: the parenthetical speaks the direct customer-visible form
	// (no internal mechanism vocabulary on the answer face).
	detail := "values kept unchanged, no split estimated"
	if inversionKept > 0 {
		// 修复轮 FIX-1 honest sub-clause: the recast seats' ranking weight is
		// already reduced to the directly measured overlap, so the full-window
		// keep is true of their raw wait account only — the family sentence
		// may not claim more for them.
		detail += fmt.Sprintf("; %d priority-inversion seat(s) among them already rank by their directly measured same-CPU overlap rather than the full wait, so the full-window keep applies to their raw runnable-wait account only", inversionKept)
	}
	return fmt.Sprintf("%s %d on-chain runnable seat(s) kept full-window values with no usable anchored-share ledger record (%s): %s%s",
		rspaRunnableLedgerFallbackCaveatPrefix, count, detail, strings.Join(named, ", "), suffix)
}

// stampResourceClosureEvaluation marks every resource-attribution row with
// the typed "closure credential was computable" bit when the stats sweep ran
// with the RSPA anchor basis. The enrich lane decision requires the
// completion-closure credential ONLY on evaluated io_latency rows. Native IO
// rows already set that bit independently of anchors; this backfill never
// clears it. Only legacy rows still lacking the bit keep the pre-RSPA overlap
// behavior. Other resource projections keep their
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
// this IO's completion thread, directed to this IO's issuing thread, after the
// physical completion row and within the bounded handler slack.  Source and
// physical line order are part of the credential; timestamp proximity alone
// never joins artifacts or unrelated wakees.
func resourceCompletionClosureProven(stats WindowStats, sourcePath string, issuePID, completePID int, issueTs, completeTs float64, completeLine int) bool {
	if strings.TrimSpace(sourcePath) == "" || issuePID <= 0 || completePID <= 0 || completeTs <= issueTs || completeLine <= 0 {
		return false
	}
	for _, record := range stats.anchoredDIOWakeups {
		if record.sourcePath != sourcePath || record.wakerPID != completePID || record.wakeePID != issuePID {
			continue
		}
		if record.line > completeLine && record.ts >= completeTs && record.ts <= completeTs+rspaIOCompletionClosureTolS {
			return true
		}
	}
	return false
}
