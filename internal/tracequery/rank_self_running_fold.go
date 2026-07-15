package tracequery

import (
	"fmt"
	"strings"
)

// rank_self_running_fold.go — ELIM-SELF-FIX 件1 (§29.93 user ruling R8 +
// §29.93.1 排查定谳, ledger docs/design/real_trace_campaign_20260705.md,
// 2026-07-15): the analysis TARGET's own RUNNING supply-fold deficit gets its
// own rank seat, minted from the target's window-projected running intervals.
//
// Form-1 disease (§29.93.1): the ONLY self-running rank mint channel used to
// be the depth-0 CausalImpacts exception arm, and interestingIntervals
// explicitly skips RUNNING segments — so an ordinary window minted ZERO
// self-running seats and the ◎ eliminable-overview board structurally missed
// the target's own conversion deficit (donghu 17267 flagship-window witness:
// a would-be deficit 8.2× the board's #1 seat published nothing). The
// second-order harm: the compute_supply running-dominant low_frequency
// suppression arm (enrichRootCauseRankWithScheduler) assumed "the causal
// running seat already represents this verdict" — false for the target in an
// ordinary window, so the whole self low-frequency running lane had no
// outlet. This seat completes that premise.
//
// Seat semantics (all typed, mirroring the depth-0 running lane):
//   - input   = the target's OWN running intervals projected into the query
//     window — the SAME ThreadTimeline decomposition every target-state face
//     reads (threadTimelineForTarget via the chain-query cache; incarnation /
//     scheduler-integrity fail-closes inherit: no provable intervals → no
//     seat);
//   - fold    = cache.supplyFoldRunningIntervals — THE single R5 conversion
//     (§29.88.3/§29.88.12 单基准单算法, 全域最大核最高频点 basis; RNB-4
//     §29.94). No second conversion algorithm is introduced here;
//   - eff     = the fold deficit ONLY (§20.2: running participates through
//     its ELIMINABLE deficit; raw wall clock stays display evidence). A
//     deficit of 0 — full-frequency running or missing frequency data (the
//     fold books unknown slices at ratio 1, 不伪造) — mints NO seat: zero
//     authority is never dressed up as an eliminable amount;
//   - channel = on_chain on the SELF basis (R8: 不特殊说明,目标线程自身恒为
//     「链上」— OnChainBasis=self_wall_clock_interval, the SELF-ALL §29.61.2
//     basis; Causality speaks the honest self token, no wakeup edge claimed);
//   - tier    = the ordinary election ladder (no special tier).
//
// ORD single-seat closure (§29.93.1 修向①): when a degenerate window's
// depth-0 CausalImpacts exception arm would ALSO mint a self running row
// (tieba head-window witness), the WINDOW-PROJECTION seat is primary — its
// interval set is a superset of the exception arm's chain-derived subset —
// and the depth-0 mint is suppressed at the CausalImpacts loop (its
// RootEvidence twin is still seeded so no other lane resurrects the same
// physical occurrence). One occurrence set, one seat.
func mintSelfRunningSupplyFoldDeficitSeat(idx *Index, q Query, chain ChainResult, cache *chainQueryCache) (RootCauseRankItem, bool) {
	if idx == nil || q.TimeEnd <= q.TimeStart {
		// The seat is a 窗内 account — an unbounded scan has no window to
		// project into (mirrors targetWindowTimeline's bounded-window gate).
		return RootCauseRankItem{}, false
	}
	if len(chain.Nodes) == 0 && len(chain.Edges) == 0 && len(chain.CausalImpacts) == 0 {
		// SELF-ALL admission half: no chain universe → no on-chain identity
		// to grant (rank_self_wall_clock_selfall.go discipline).
		return RootCauseRankItem{}, false
	}
	target := chain.Target
	if target.PID <= 0 && strings.TrimSpace(target.Comm) == "" {
		return RootCauseRankItem{}, false
	}
	if cache == nil {
		cache = newChainQueryCache(idx, q.runCancel)
	}
	tq := q
	tq.View = ""
	tq.PID = target.PID
	tq.Thread = target.Comm
	tq.ThreadInput = ""
	tl := cache.timeline(tq, target)
	var running []Interval
	total := 0.0
	envStart, envEnd := 0.0, 0.0
	lineStart, lineEnd := 0, 0
	for _, it := range tl.Intervals {
		if it.State != StateRunning || it.DurationMs <= 0 || it.EndTs <= it.StartTs {
			continue
		}
		running = append(running, it)
		total += it.DurationMs
		if envStart == 0 || it.StartTs < envStart {
			envStart = it.StartTs
		}
		if it.EndTs > envEnd {
			envEnd = it.EndTs
		}
		applyLineRange(&lineStart, &lineEnd, it.StartLine)
		applyLineRange(&lineStart, &lineEnd, it.EndLine)
	}
	if total <= 0 {
		return RootCauseRankItem{}, false
	}
	if !selfWallClockIntervalInWindow(&chain, envStart, envEnd) {
		return RootCauseRankItem{}, false
	}
	ideal, basis := cache.supplyFoldRunningIntervals(q, envStart, envEnd, running)
	deficit := total - ideal
	if deficit <= 0 {
		// Honest zero (0 权威不伪造): full-frequency/full-core running is
		// real workload, and unknown-frequency slices folded at ratio 1 mint
		// no deficit — either way there is nothing eliminable to seat.
		return RootCauseRankItem{}, false
	}
	item := rootCauseItem("running", target, total, 0.86, lineStart, lineEnd,
		"thread_timeline.self_running_fold",
		fmt.Sprintf("%s ran for %.3fms inside the window; supply-fold deficit %.3fms (ideal %.3fms at the global max-core max-frequency basis)",
			threadLabel(target), total, deficit, ideal))
	item.CumulativeImpactMs = total
	item.StartTs = envStart
	item.EndTs = envEnd
	item.DominantState = string(StateRunning)
	item.RunningMs = total
	// §20.2: the ELIMINABLE deficit is the attribution (effective/sort/
	// Score); raw running wall clock stays on the display channels
	// (ImpactMs / ProjectedImpactMs / CumulativeImpactMs above).
	item.EffectiveImpactMs = deficit
	item.SupplyFoldDeficitMs = deficit
	item.SupplyFoldIdealMs = ideal
	item.SupplyFoldBasis = &basis
	item.OnChainBasis = RootCauseOnChainBasisSelfWallClockInterval
	item.ChainRelevance = "on_chain"
	item.Causality = RootCauseCausalitySelfWallClock
	item.Score = deficit * 0.86 * rootCauseScoreWeightChainImpact
	// §21.1 CWD-2 window identity: the projection was computed over exactly
	// the query window (stamped directly — the source token is the thread
	// timeline, not a window_stats summary, so the prefix stamp pass never
	// touches this row).
	item.StatsWindowStartTs = q.TimeStart
	item.StatsWindowEndTs = q.TimeEnd
	return item, true
}

// enforceSelfSymptomRowsChainChannelWireFace — ELIM-SELF-FIX 件1③ (§29.93.1
// 修向③, R8 词面裁定级): the P-4 wire-face residual — the analysis target's
// own wait-SYMPTOM rows (registry wakeup_chain / lock_contention lanes, e.g.
// the pacing_idle context row) used to publish chain_relevance=adjacent on
// the rank wire while the display tree already seats them in the chain
// stanza. R8 (自身恒为链上): a self row never wears the ◇/▒ channel — the
// symptom-face EXCLUSION (不参与汇排) is an eliminability judgment, not a
// channel demotion. Wire wording only: tier (target_self_state /
// context_only), eff, sort and election lanes are untouched — these rows
// never compete (SYM §24.13) and their ordinal channel is skipped by the
// dedicated tier arms before the channel switch is consulted. RSPA remainder
// and credential-demoted seats keep their ◇ lane: their channel IS their
// credential story (无链上凭证), not a proximity verdict.
func enforceSelfSymptomRowsChainChannelWireFace(items []RootCauseRankItem) {
	for i := range items {
		if !items[i].SubjectIsAnalysisTarget {
			continue
		}
		if strings.TrimSpace(items[i].ChainRelevance) != "adjacent" {
			continue
		}
		if items[i].ChainAnchorRemainderSeat || items[i].ChainCredentialLaneDemoted {
			continue
		}
		if !rootCauseItemIsTargetWaitSymptomType(items[i]) {
			continue
		}
		items[i].ChainRelevance = "on_chain"
		// The honest self token — never "on_wakeup_chain" on a row without an
		// edge (SELF-ALL causality-minter discipline). No basis is stamped:
		// the symptom row claims no wall-clock-interval basis, only the R8
		// channel identity.
		items[i].Causality = RootCauseCausalitySelfWallClock
	}
}
