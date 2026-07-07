package tool

// answer_document_mutation_runtime_supplyfold.go — VS-2 (§7.10, docs/design/
// customer_dead_session_audit_20260703.md): the supply-fold decision table and
// its single-source wording for on-chain RUNNING-dominant rows.
//
// Decision table (ALL typed inputs, SOFT face only — wording and labels; the
// verdict never touches ranking, effective attribution or any gate):
//
//	缺口占比高  deficit ≥ running×20% ∧ deficit ≥ 1ms   (running = ideal+deficit)
//	runnable显著 shared RN-1 gate traceQueryRunnableSignificant (§7.10 同源)
//	反转存在    the row IS a priority_inversion_candidate row
//
//	高∧显著∧反转 → 供给折算缺口 · 调度压力 · 优先级反转(gated) (三口径独立、不可加和)
//	高∧显著      → 前两口径独立并列 (不可加和)
//	高∧不显著    → 供给折算缺口为主,running 含跑慢成分
//	无缺口       → basis 全 known: 肯定标注「已满频满核(或近满),running 属
//	               真实工作量」; unknown>0: 如实「频点数据不全,无法折算」
//
// Wording lanes (RN-16 lint): 「供给折算缺口」 is ComputeDelivery-lane wording
// and lives ONLY in runtimeTraceProjSupplyFoldClause below; the inversion
// lane's 「running 折算」 (PTV7 word face of the former 运行折算) lives ONLY
// in runtimeTraceProjInversionCompositionText.
// The two folds are different mechanisms with DIFFERENT divisors (own-running
// big-cluster-fmax frequency fold vs consumer-core-relative inversion discount,
// §15.A) and their words never mix. Magnitudes always carry their own units and
// their own caliber basis; the clause NEVER sums the mechanisms (S1 / 墙钟裁定:
// different calibers do not add) — it joins them with "·" under an explicit
// 「各口径独立、不可加和」 leader, never the summing "共同作用" tail (Q4-G,
// §12.3/§15.D landing-surface fix for §7.10 red line 2).

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Soft-face thresholds of the §7.10 decision table. Noisy-signal guidance
// only: they select WORDING, never a hard gate, never rank.
const (
	runtimeTraceProjSupplyFoldDeficitShare   = 0.20
	runtimeTraceProjSupplyFoldDeficitFloorMs = 1.0
)

// runtimeTraceProjSupplyFoldVerdict is the typed four-branch outcome.
type runtimeTraceProjSupplyFoldVerdict int

const (
	runtimeTraceProjSupplyFoldNone runtimeTraceProjSupplyFoldVerdict = iota
	// 高∧显著∧反转 — deficit + scheduling pressure + inversion composition.
	runtimeTraceProjSupplyFoldTriple
	// 高∧显著 — deficit + scheduling pressure.
	runtimeTraceProjSupplyFoldWithDemand
	// 高∧不显著 — deficit-led, running contains a running-slow share.
	runtimeTraceProjSupplyFoldDominant
	// 无缺口 ∧ basis 全 known — affirmative: ran at (near) full frequency.
	runtimeTraceProjSupplyFoldNoDeficit
	// basis unknown>0 且非高缺口 — frequency data incomplete, no verdict.
	runtimeTraceProjSupplyFoldUnknownBasis
)

// runtimeTraceProjSupplyFoldRunningMS reconstructs the folded running total
// from the identity ideal + deficit == RunningMs (producer contract, exact).
func runtimeTraceProjSupplyFoldRunningMS(node types.TraceCausalProjectionNode) float64 {
	return node.SupplyFoldIdealMS + node.SupplyFoldDeficitMS
}

// runtimeTraceProjSupplyFoldVerdictFor evaluates the §7.10 decision table on
// one node. windowMS ≤ 0 (no anchor window) leaves the runnable-significance
// arm false — conservative: the clause then claims the deficit only, never
// scheduling pressure it cannot ground. A high deficit with a partially
// unknown basis STAYS a high verdict: unknown slices fold at ratio 1 and
// never mint deficit, so the published value remains a lower bound. The
// affirmative no-deficit branch alone requires a fully-known basis.
func runtimeTraceProjSupplyFoldVerdictFor(node types.TraceCausalProjectionNode, windowMS float64) runtimeTraceProjSupplyFoldVerdict {
	if !node.SupplyFoldComputed {
		return runtimeTraceProjSupplyFoldNone
	}
	running := runtimeTraceProjSupplyFoldRunningMS(node)
	if running > 0 &&
		node.SupplyFoldDeficitMS >= running*runtimeTraceProjSupplyFoldDeficitShare &&
		node.SupplyFoldDeficitMS >= runtimeTraceProjSupplyFoldDeficitFloorMs {
		significant := traceQueryRunnableSignificant(node.RunnableMS, windowMS)
		switch {
		case significant && runtimeTraceCausalProjectionInversionRow(node):
			return runtimeTraceProjSupplyFoldTriple
		case significant:
			return runtimeTraceProjSupplyFoldWithDemand
		default:
			return runtimeTraceProjSupplyFoldDominant
		}
	}
	if node.SupplyFoldUnknownMS > 0 || node.SupplyFoldKnownMS <= 0 {
		return runtimeTraceProjSupplyFoldUnknownBasis
	}
	return runtimeTraceProjSupplyFoldNoDeficit
}

// runtimeTraceProjSupplyFoldEmbedsInversionComposition reports whether this
// row's supply-fold clause is the Triple branch — the ONE branch whose text
// already embeds the D3 inversion composition ("优先级反转(构成: …)"). F-4
// (统一复核 2026-07-04): the row-level renderers consult this typed verdict
// (the SAME decision-table evaluation the clause renders from, so the two
// surfaces can never disagree) to suppress their independent "影响构成" tag —
// otherwise a triple-mechanism row carried the composition text twice on two
// ↳ continuation lines (H5-class inflation). The composition stays
// single-sourced inside the clause.
func runtimeTraceProjSupplyFoldEmbedsInversionComposition(node types.TraceCausalProjectionNode, windowMS float64) bool {
	return runtimeTraceProjSupplyFoldVerdictFor(node, windowMS) == runtimeTraceProjSupplyFoldTriple
}

// runtimeTraceProjInversionCompositionText is the SINGLE source of the
// §7.30.3 D3 gated-composition wording (PTV7 #74: runnable X + running 折算
// Y — the state words are canonical tokens, the 折算 caliber word stays
// localized). The inversion lane's 「running 折算」 term lives here and ONLY
// here (RN-16 wording lint) — the VS-2 供给折算 lane never borrows it.
//
// This is the ONE additive breakdown in the whole clause: runnable +
// running 折算 == the inversion candidate's own gated ms (PriorityInversionGatedMs,
// producer identity — both terms are that ONE node's own component ms, so
// they genuinely add). The OUTER three-mechanism join is a different regime
// (three different-caliber views, never summed) — the clause below states
// that non-additivity explicitly so a reader never carries the inner "+"
// across the outer separator.
//
// RCX² 复核 F1: each component wears ITS OWN ruler — the runnable component
// is counted IN FULL (producer contract, tracequery/types.go GatedRunnableMs:
// "runnable time counted in full"), only the running deficit rides the
// downstream-consumer-core fold. The divisor disclosure therefore sits on
// the running component, never on the runnable component or the total — a
// ruler stretched over a full-amount component would be exactly the caliber
// mislabel this clause exists to kill. Both the Triple clause's 内含
// parenthetical and the independent 影响构成 tag (non-Triple inversion rows)
// share this text, so both surfaces carry the per-component calibers.
//
// §21 RNB R3 (2026-07-07): the R1 display-only runnable sub-row (⧖ runnable
// X(全额)·就绪排队积压·gated 分量,不重复计入排序 — tree.go) reuses THIS
// component wording verbatim (runnable X(全额), 同词). The parenthetical here
// is the caliber DISCLOSURE of the gated total's decomposition and stays —
// it is not a duplicate of the R1 component row (裁定: 口径披露非重复行).
func runtimeTraceProjInversionCompositionText(node types.TraceCausalProjectionNode, zh bool) string {
	if zh {
		return fmt.Sprintf("runnable %.3fms(全额)+ running 折算 %.3fms(按下游消费核折算)", node.GatedRunnableMS, node.GatedRunningDeficitMS)
	}
	return fmt.Sprintf("runnable %.3fms (in full) + discounted running %.3fms (folded at the downstream consumer core)", node.GatedRunnableMS, node.GatedRunningDeficitMS)
}

// runtimeTraceProjInversionGatedTotalMS is the Triple clause's gated-composite
// total — SAME-SOURCE as the row's 有效归因 tag (RCX² 复核 F2): the engine
// publishes the gated composite ONCE through the rank-lane mirror
// (EffectiveImpactMS: gated>0 non-periodic inversion → R5d gated, PTV5 Q1
// single authority), while re-summing the two %.3f-rounded component notes
// can diverge from it by 0.001 (round3(a)+round3(b) != round3(a+b) — the
// S1/clamp dual-caliber-leak class). The component sum remains the fallback
// only for the corners where EffectiveImpactMS is NOT the gated composite:
// PeriodicSource rows (the VS-1 discount lane owns Effective there, and it
// is authoritative even at 0) and rows whose effective note never published
// (0). The engine's own priority_inversion_gated note would be the ideal
// source but has no projection consumer today — wiring one needs a typed
// node field + parse in internal/types (outside this display batch's file
// boundary; P0-E lifts this to that note when it opens the engine side).
func runtimeTraceProjInversionGatedTotalMS(node types.TraceCausalProjectionNode) float64 {
	gatedSum := node.GatedRunnableMS + node.GatedRunningDeficitMS
	if gatedSum > 0 && !node.PeriodicSource && node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	return gatedSum
}

// runtimeTraceProjSupplyFoldClause renders the §7.10 mechanism clause for one
// node: the clause text and its keep-marker (the phrase a width fit must
// never shave below). ok=false when the fold never ran. Single source for
// every display surface (conclusion line, tree tail tag, detail table) — the
// three can never disagree. Each magnitude carries its own unit and its own
// caliber basis; the mechanisms are DIFFERENT-caliber perspectives on the
// same running-dominant node and are NEVER summed (§7.10 red line 2, S1/墙钟
// 裁定: different calibers do not add).
//
// Q4-G (real_trace_campaign_20260705.md §12.3/§15.D): the former
// "A + B + C … 共同作用" phrasing INVITED the summing misread (q6 E7: a reader
// added 供给折算缺口 17.702 + runnable 20.713 + 优先级反转 37.410, three
// different-divisor calibers). The clause now (a) leads with an explicit
// 「各口径独立、不可加和」 disclaimer, (b) joins the perspectives with a neutral
// middot instead of "+", never the summing "共同作用" tail, and (c) names each
// number's OWN caliber inline — the §15.A two-divisor disclosure: the
// supply-fold deficit folds at the big cluster's fmax while the inversion
// lane's running-deficit COMPONENT folds at the downstream consumer core (a
// different divisor — the two 折算 numbers are NOT comparable and NOT
// additive). RCX² 复核 F1: the inversion TOTAL is a gated composite, not a
// folded value — it wears only the gated-caliber word; the consumer-core
// ruler sits on the running component inside the composition text (the
// runnable component is counted in full and wears 全额). The ONE additive
// breakdown is that candidate's OWN gated split and it stays inside its
// parenthetical. F3: the zh face joins the calibers with the no-space "·"
// (the within-tag convention, e.g. 周期性信号源…·有效归因X), keeping it
// visually distinct from the BETWEEN-tag " · " separator so a neighbouring
// tag never reads as a fourth caliber; the EN face keeps the spaced " · "
// (its within-tag convention, e.g. "periodic signal source · attribution").
func runtimeTraceProjSupplyFoldClause(node types.TraceCausalProjectionNode, windowMS float64, zh bool) (string, string, bool) {
	verdict := runtimeTraceProjSupplyFoldVerdictFor(node, windowMS)
	if verdict == runtimeTraceProjSupplyFoldNone {
		return "", "", false
	}
	deficit := node.SupplyFoldDeficitMS
	switch verdict {
	case runtimeTraceProjSupplyFoldTriple:
		// PTV7 (#74, supersedes the PTV5 C18 zh state word): the parenthetical
		// speaks the canonical runnable token on BOTH faces. §7.5 的
		// 调度压力(需求积压) label stays single-sourced and untouched. The
		// inversion candidate's own gated total (same-source as the row's
		// 有效归因 tag, F2) precedes its internal split so the "+" is
		// unambiguously scoped to that one node's own decomposition.
		inversionTotal := runtimeTraceProjInversionGatedTotalMS(node)
		if zh {
			return fmt.Sprintf("机制构成(各口径独立、不可加和): 供给折算缺口 %.3fms(按大核满频折算,下界)·%s runnable %.3fms(就绪排队积压口径)·优先级反转 %.3fms(gated 口径,内含 %s)",
				deficit, runtimeTraceSupplyPressureDisplayLabel(true), node.RunnableMS,
				inversionTotal, runtimeTraceProjInversionCompositionText(node, true)), "机制构成", true
		}
		return fmt.Sprintf("mechanism (each caliber is independent and not additive): supply-fold deficit %.3fms (folded at big-cluster fmax, lower bound) · %s runnable %.3fms (ready-queue backlog caliber) · priority inversion %.3fms (gated caliber, made of %s)",
			deficit, runtimeTraceSupplyPressureDisplayLabel(false), node.RunnableMS,
			inversionTotal, runtimeTraceProjInversionCompositionText(node, false)), "mechanism", true
	case runtimeTraceProjSupplyFoldWithDemand:
		if zh {
			return fmt.Sprintf("机制构成(各口径独立、不可加和): 供给折算缺口 %.3fms(按大核满频折算,下界)·%s runnable %.3fms(就绪排队积压口径)",
				deficit, runtimeTraceSupplyPressureDisplayLabel(true), node.RunnableMS), "机制构成", true
		}
		return fmt.Sprintf("mechanism (each caliber is independent and not additive): supply-fold deficit %.3fms (folded at big-cluster fmax, lower bound) · %s runnable %.3fms (ready-queue backlog caliber)",
			deficit, runtimeTraceSupplyPressureDisplayLabel(false), node.RunnableMS), "mechanism", true
	case runtimeTraceProjSupplyFoldDominant:
		if zh {
			return fmt.Sprintf("供给折算缺口 %.3fms(按大核满频折算,下界)为主,running 含跑慢成分", deficit), "供给折算缺口", true
		}
		return fmt.Sprintf("supply-fold deficit %.3fms (folded at big-cluster fmax, lower bound) leads; running carries a running-slow share", deficit), "supply-fold deficit", true
	case runtimeTraceProjSupplyFoldNoDeficit:
		// Affirmative exclusion (§7.10 fourth branch, via_thread-NOT family
		// value): only a fully-known basis may make this claim.
		if zh {
			return "已满频满核(或近满),running 属真实工作量", "已满频满核", true
		}
		return "ran at (near) full frequency on the top cluster; running is true workload", "full frequency", true
	default: // runtimeTraceProjSupplyFoldUnknownBasis
		if zh {
			return "频点数据不全,无法折算", "频点数据不全", true
		}
		return "frequency data incomplete; supply fold not computable", "frequency data incomplete", true
	}
}

// runtimeTraceProjSupplyFoldTag renders the clause as the node row's tail tag.
// PTV4 T1: the clause is never elided or shaved — on width pressure it
// demotes intact to a "· " subordinate detail line (the mechanism magnitudes
// have no other fence carrier).
func runtimeTraceProjSupplyFoldTag(node types.TraceCausalProjectionNode, windowMS float64, zh bool) (runtimeTraceProjTag, bool) {
	text, _, ok := runtimeTraceProjSupplyFoldClause(node, windowMS, zh)
	if !ok {
		return runtimeTraceProjTag{}, false
	}
	return runtimeTraceProjTag{Text: text}, true
}
