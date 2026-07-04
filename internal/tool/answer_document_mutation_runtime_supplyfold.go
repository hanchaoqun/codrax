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
//	高∧显著∧反转 → 供给折算缺口 + 调度压力 + 优先级反转(构成) 共同作用
//	高∧显著      → 前两项共同作用
//	高∧不显著    → 供给折算缺口为主,running 含跑慢成分
//	无缺口       → basis 全 known: 肯定标注「已满频满核(或近满),running 属
//	               真实工作量」; unknown>0: 如实「频点数据不全,无法折算」
//
// Wording lanes (RN-16 lint): 「供给折算缺口」 is ComputeDelivery-lane wording
// and lives ONLY in runtimeTraceProjSupplyFoldClause below; the inversion
// lane's 「运行折算」 lives ONLY in runtimeTraceProjInversionCompositionText.
// The two folds are different mechanisms (own-running frequency fold vs
// consumer-relative inversion discount) and their words never mix. Magnitudes
// always carry their own units; the clause NEVER sums the mechanisms (S1 /
// 墙钟裁定: different calibers do not add).

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
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
// §7.30.3 D3 gated-composition wording (可运行等待 X + 运行折算 Y). The
// inversion lane's 「运行折算」 term lives here and ONLY here (RN-16 wording
// lint) — the VS-2 供给折算 lane never borrows it.
func runtimeTraceProjInversionCompositionText(node types.TraceCausalProjectionNode, zh bool) string {
	if zh {
		return fmt.Sprintf("可运行等待 %.3fms + 运行折算 %.3fms", node.GatedRunnableMS, node.GatedRunningDeficitMS)
	}
	return fmt.Sprintf("runnable %.3fms + discounted running %.3fms", node.GatedRunnableMS, node.GatedRunningDeficitMS)
}

// runtimeTraceProjSupplyFoldClause renders the §7.10 mechanism clause for one
// node: the clause text and its keep-marker (the phrase a width fit must
// never shave below). ok=false when the fold never ran. Single source for
// every display surface (conclusion line, tree tail tag, detail table) — the
// three can never disagree. Each magnitude carries its own unit; mechanisms
// are joined, NEVER summed.
func runtimeTraceProjSupplyFoldClause(node types.TraceCausalProjectionNode, windowMS float64, zh bool) (string, string, bool) {
	verdict := runtimeTraceProjSupplyFoldVerdictFor(node, windowMS)
	if verdict == runtimeTraceProjSupplyFoldNone {
		return "", "", false
	}
	deficit := node.SupplyFoldDeficitMS
	switch verdict {
	case runtimeTraceProjSupplyFoldTriple:
		if zh {
			return fmt.Sprintf("机制构成: 供给折算缺口 %.3fms(按大核满频折算,下界)+ %s(runnable %.3fms)+ 优先级反转(构成: %s)共同作用",
				deficit, runtimeTraceSupplyPressureDisplayLabel(true), node.RunnableMS,
				runtimeTraceProjInversionCompositionText(node, true)), "机制构成", true
		}
		return fmt.Sprintf("mechanism: supply-fold deficit %.3fms (folded at big-cluster fmax, lower bound) + %s (runnable %.3fms) + priority inversion (composition: %s) acting together",
			deficit, runtimeTraceSupplyPressureDisplayLabel(false), node.RunnableMS,
			runtimeTraceProjInversionCompositionText(node, false)), "mechanism", true
	case runtimeTraceProjSupplyFoldWithDemand:
		if zh {
			return fmt.Sprintf("机制构成: 供给折算缺口 %.3fms(按大核满频折算,下界)+ %s(runnable %.3fms)共同作用",
				deficit, runtimeTraceSupplyPressureDisplayLabel(true), node.RunnableMS), "机制构成", true
		}
		return fmt.Sprintf("mechanism: supply-fold deficit %.3fms (folded at big-cluster fmax, lower bound) + %s (runnable %.3fms) acting together",
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

// runtimeTraceProjSupplyFoldTag renders the clause as the node row's tail tag:
// Keep + NoTruncate + ContinuationLane — the mechanism magnitudes have no
// other fence carrier; on width pressure the clause moves intact onto its own
// ↳ continuation line(s) (same sanctioned overflow class as the D3
// composition split and the RN-1 occupier roster).
func runtimeTraceProjSupplyFoldTag(node types.TraceCausalProjectionNode, windowMS float64, zh bool) (runtimeTraceProjTag, bool) {
	text, marker, ok := runtimeTraceProjSupplyFoldClause(node, windowMS, zh)
	if !ok {
		return runtimeTraceProjTag{}, false
	}
	return runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep,
		NoTruncate: true, ContinuationLane: true,
		MinKeep: runewidth.StringWidth(marker) + 1}, true
}
