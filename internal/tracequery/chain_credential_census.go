package tracequery

// chain_credential_census.go — CHAINGUARD-1 件1 (§29.204/§29.204.1 spec §2/§3,
// user ruling 「排查是否还有其它误上链场景,一定要看护好避免回归」, 2026-07-21).
//
// 【INV-CHAINGUARD 结构不变式】 For one rank publication (the tail of
// assignRootCauseRanksAndTiers) with hasCausalChain==true: every published
// item with rootCauseOrdinalChannel==chain ∧ Rank>0 ∧ effective>0 MUST carry
// at least ONE typed chain-credential stamp. The census verdict is a CLOSED
// enum (four credential tiers + none), minted at exactly one point
// (censusChainSeatCredential) inside the ordinal publication authority — a
// future third caller of the allocator structurally cannot skip it
// (CHAINGUARD-F3: ≥7 mutation/mint lanes run AFTER normalize, so 「chain 在场
// ⟹ normalize 全员盖章」 was only a timing assumption; 铸序即普查 closes it).
//
// 【Stamp closed set — all PRECISE typed signals, zero new admission signals】
//   S1  wakeup_anchored    — OnChainBasis host_wakeup_edge_pre_span/_state
//                            (R3/ONCHAIN-3c real typed edge credential; the
//                            S8 constructive wakeup_chain source is
//                            deliberately tiered under S8' interval_proven —
//                            see that row).
//   S2  target_self        — OnChainBasis self bases ∨ Causality self tokens
//                            ∨ SubjectIsAnalysisTarget (R8 自身恒链上,
//                            §29.61.1/.2 既裁 — the token IS the stamp; the
//                            typed-subject arm was added mid-implementation
//                            after real-trace tests caught the target's own
//                            basis-less satellite seats being judged
//                            zero-credential and demoted off their own chain).
//   S3  member_inherited   — ChainIdentityInheritance (§29.134 fail-open keep
//                            既裁合法: the typed admission record IS its
//                            credential; the invariant requires 「at least one
//                            stamp」, never 「an edge」 — 相容论证在此闭合,
//                            seat and values move zero).
//   S4/S5/S6/S7/S8' interval_proven —
//                            ChainCredentialEnvelopeLevel (§29.104 终判③
//                            envelope keep 既裁) ∨ ChainAnchoredMs>0 (RSPA
//                            anchored share) ∨ Source prefix "wakeup_chain"
//                            (constructive edge-recursion mint — value minted
//                            inside edge-closed windows, 构造即凭证; §29.134
//                            件2 同款前缀门; deliberately mapped to the
//                            interval tier so the §29.187① completeness-arm
//                            交集证明 word face stays byte-identical) ∨
//                            OverlapMs>0 with a VALID own interval
//                            (rankFoldStartUsable member-side read — a
//                            (0,end) pseudo-interval's prefix-envelope overlap
//                            is NOT a stamp, the ISPGAP 主嫌疑 independent
//                            second net).
//   none                   — zero stamps: the census violation verdict.
//
// 【Violation disposition — fail-loud disclosure, never an answer block
// (§29.104.13 非致命不硬拦红线)】 a none seat (a) withdraws its chain ordinal
// and lands on the honest ▒ background lane (value channels untouched; ◇
// adjacency is also a claim and is never fabricated), (b) keeps the typed
// census=none violation record on the wire, and (c) raises one result-level
// audit caveat. First beneficiaries: the LANE-G bare-membership satellite
// mints (scheduler_latency / low_frequency / cpu_affinity threadInSet 直译)
// and the LANE-C RSPA fail-open zero-word forms — exactly the audit's
// 「chip 穿透候选」 lanes.
//
// Spec §2(c)(d) disposition (§29.210 裁定, 2026-07-22): the none-seat ▒-row
// display honest word and the --tracediag strict fail-loud arm are WITHDRAWN
// — the four shipped layers (result caveat + sticky census=none wire record
// + the census second seat gate + the second-seat/P4 pins) already disclose
// and contain the violation, and the real fleet's none population is zero
// (eight boards, §29.204.1). Re-open only if a live census=none seat
// (none>0) ever appears in the fleet.
//
// 【禁重诉核对】 §29.134 identity inheritance / §29.104 终判③ envelope keep /
// §29.61.2 self-always-on-chain all carry stamps by construction — the
// invariant only ever fires on the ZERO-stamp form, so the three adjudicated
// fail-open keeps are outside the strike surface (§29.185① DRIFTGUARD).
// Chainless boards (hasCausalChain==false) are exempt whole (§29.36.2 既裁
// 单宇宙 fail-open; the ISPGAP-1 display gates own that face). Rank==0 / eff≤0
// seats and the ⌗ caliber side rail are outside the crown-competition
// population and are never stamped.

import (
	"fmt"
	"strings"
)

// RootCauseChainCredentialCensus* — the closed census verdict enum
// (CHAINGUARD-1; the four credential tiers are the §29.187① 四字族 engine
// authority, wire token = chain_credential_census).
const (
	RootCauseChainCredentialCensusWakeupAnchored  = "wakeup_anchored"
	RootCauseChainCredentialCensusTargetSelf      = "target_self"
	RootCauseChainCredentialCensusIntervalProven  = "interval_proven"
	RootCauseChainCredentialCensusMemberInherited = "member_inherited"
	RootCauseChainCredentialCensusNone            = "none"
)

// censusChainSeatCredential is THE single verdict mint point (CHAINGUARD-1
// spec §3①). Precedence mirrors the ◎ chip word arms exactly (件3 同源:
// self → edge → inheritance → interval family), so the display mapping and
// this engine authority can never disagree on a credentialed seat.
func censusChainSeatCredential(item RootCauseRankItem, zeroStartReal bool) string {
	switch strings.TrimSpace(item.OnChainBasis) {
	case RootCauseOnChainBasisSelfDeterministicSpan, RootCauseOnChainBasisSelfWallClockInterval:
		return RootCauseChainCredentialCensusTargetSelf
	case RootCauseOnChainBasisHostWakeupEdge, RootCauseOnChainBasisHostWakeupEdgeState:
		return RootCauseChainCredentialCensusWakeupAnchored
	}
	switch strings.TrimSpace(item.Causality) {
	case RootCauseCausalitySelfDeterministic, RootCauseCausalitySelfWallClock:
		return RootCauseChainCredentialCensusTargetSelf
	}
	if item.SubjectIsAnalysisTarget {
		// R8 自身恒链上 (§29.61.2 既裁; §29.96.2: self rows are never ◇/▒):
		// the typed subject identity IS the analysis target's chain
		// credential — a self seat that arrived without a basis/token stamp
		// (e.g. the target's own scheduler satellite) must never be judged
		// zero-credential and demoted off its own chain.
		return RootCauseChainCredentialCensusTargetSelf
	}
	if item.ChainIdentityInheritance && !item.ChainCredentialEnvelopeLevel {
		return RootCauseChainCredentialCensusMemberInherited
	}
	if item.ChainCredentialEnvelopeLevel {
		return RootCauseChainCredentialCensusIntervalProven
	}
	if item.ChainAnchoredMs > 0 {
		return RootCauseChainCredentialCensusIntervalProven
	}
	if strings.HasPrefix(strings.TrimSpace(item.Source), "wakeup_chain") {
		return RootCauseChainCredentialCensusIntervalProven
	}
	if item.OverlapMs > 0 && rankFoldStartUsable(item.StartTs, item.EndTs, zeroStartReal) {
		return RootCauseChainCredentialCensusIntervalProven
	}
	return RootCauseChainCredentialCensusNone
}

// stampChainSeatCredentialCensus runs the census over one finished ordinal
// publication and applies the violation disposition. Called ONLY from the
// tail of assignRootCauseRanksAndTiers (铸序即普查 — both publication sites
// share it automatically; a later mint lane cannot bypass it because it
// cannot obtain ordinals without re-entering the authority).
//
// Idempotence: verdicts are recomputed from scratch for the in-population
// seats (assign, never |=; the ONCHAIN-FIX-2 recompute precedent). The
// census=none VIOLATION RECORD is deliberately sticky on its demoted row —
// like ChainIdentityInheritance it records the admission history and is the
// wire disclosure the display/caveat faces read; it clears only if the row
// ever re-enters the population and earns a real stamp (structurally
// impossible without new credential).
func stampChainSeatCredentialCensus(items []RootCauseRankItem, hasCausalChain, zeroStartReal bool) string {
	if !hasCausalChain {
		// Chainless single-universe boards are exempt whole (§29.36.2 既裁);
		// clear any stale verdict so a re-publication never carries a census
		// the board's universe cannot mean.
		for i := range items {
			items[i].ChainCredentialCensus = ""
		}
		return ""
	}
	demoted := 0
	var demotedLabels []string
	for i := range items {
		if items[i].ChainCredentialCensus != RootCauseChainCredentialCensusNone {
			items[i].ChainCredentialCensus = ""
		}
		if rootCauseOrdinalChannel(items[i]) != rootCauseOrdinalChannelChain {
			continue
		}
		if items[i].Rank <= 0 || rootCauseEffectiveImpactMs(items[i]) <= 0 {
			continue
		}
		verdict := censusChainSeatCredential(items[i], zeroStartReal)
		items[i].ChainCredentialCensus = verdict
		if verdict != RootCauseChainCredentialCensusNone {
			continue
		}
		// Violation disposition (a): withdraw the chain claim wholesale — the
		// relevance token AND the causality word (a bare-membership mint may
		// carry "on_wakeup_chain" with zero edges; leaving it would keep the
		// row on the chain channel through the causality fallback). Value
		// channels move zero.
		items[i].ChainRelevance = "background"
		items[i].Causality = "background"
		demoted++
		if len(demotedLabels) < 4 {
			demotedLabels = append(demotedLabels, fmt.Sprintf("%s %s", items[i].Type, threadLabel(items[i].Thread)))
		}
	}
	if demoted == 0 {
		return ""
	}
	// The demoted rows changed channel: re-run the allocator so the chain
	// ordinal space repacks without them (the background channel publishes no
	// ordinal by construction). No second census pass follows, and none is
	// needed (dual-review F-1 构造论证): the allocator's seat verdicts are
	// item-local — every arm (trace_gap / self-symptom / ⌗ caliber / eff≤0)
	// reads only the row's own fields, never its position or its neighbours —
	// so the repack is contractive: it only removes chain seats (the demoted
	// rows) and can never promote a previously Rank=0 row into a newly-seated
	// unstamped chain seat. One pass IS the fixed point by construction.
	assignRootCauseRankOrdinalsAndTiers(items)
	suffix := ""
	if demoted > len(demotedLabels) {
		suffix = fmt.Sprintf(" and %d more", demoted-len(demotedLabels))
	}
	return fmt.Sprintf("chain_credential_census: %d on-chain ranked seat(s) carried zero typed chain credential and were demoted to the background lane (census=none fail-loud disclosure; values untouched): %s%s",
		demoted, strings.Join(demotedLabels, ", "), suffix)
}

// rootEvidenceSeatNodeWindow (CHAINGUARD-1 件5, LANE-B) resolves the ONE
// same-pid chain node window backing a RootEvidence rank seat. ok=false when
// the pid holds zero or several positive node windows — ambiguity never
// guesses an interval (宁漏勿假; the multi-node form keeps the §29.134
// identity-inheritance arm byte-identically). Duplicate windows with
// identical endpoints count as one.
func rootEvidenceSeatNodeWindow(chain ChainResult, thread ThreadRef) (TimeWindow, bool) {
	if thread.PID <= 0 {
		return TimeWindow{}, false
	}
	var window TimeWindow
	found := false
	for _, node := range chain.Nodes {
		if node.Thread.PID != thread.PID || node.Window.EndTs <= node.Window.StartTs {
			continue
		}
		if found {
			if node.Window.StartTs == window.StartTs && node.Window.EndTs == window.EndTs {
				continue
			}
			return TimeWindow{}, false
		}
		window = node.Window
		found = true
	}
	return window, found
}

// hasChainCredentialCensusCaveat dedupes the enrich re-publication's census
// caveat against the build lane's by the sentinel prefix (the XERR1-EXT
// lock-caveat precedent). Known limit, 记档候办 (dual-review F-4): the prefix
// key dedupes whole SENTENCES, so if the build lane already published a
// caveat naming demoted seat X and the enrich lane later demotes a DIFFERENT
// seat Y, Y's name is swallowed from the caveat face (Y's sticky census=none
// violation record still rides the wire untouched). Reachability needs the
// two lanes to demote different seats; the real fleet's none population is
// zero (eight boards, §29.204.1). Upgrade path when it ever matters: merge
// the demoted seat-name SETS into the existing sentence instead of deduping
// on the bare prefix.
func hasChainCredentialCensusCaveat(caveats []string) bool {
	for _, caveat := range caveats {
		if strings.HasPrefix(caveat, "chain_credential_census:") {
			return true
		}
	}
	return false
}
