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
//	高∧显著∧反转 → (PTV8-RCR-A §24 ②) cause nodes: clause suppressed — the
//	               four-line grammar's 行3+拆解子行 carry the composition;
//	               residual non-cause shape: two-caliber WithDemand text
//	高∧显著      → 前两口径独立并列 (不可加和)
//	高∧不显著    → 供给折算缺口为主,running 含跑慢成分
//	无缺口       → basis 全 known: 肯定标注「已满频满核(或近满),running 属
//	               真实工作量」; unknown>0: 如实「CPU 频率数据不全,无法折算」
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

// CAP (§26 C3) capability-source wire tokens — byte-identical mirrors of
// tracequery's CoreCapabilitySource* constants (core_capability.go; equality
// pinned cross-package in the CAP display tests). The display layer keys the
// disclosure wording on these EXACT tokens, never on re-derived heuristics.
const (
	runtimeTraceCapabilitySourceDefault  = "default_table"
	runtimeTraceCapabilitySourceEvidence = "evidence_table"
	runtimeTraceCapabilitySourceFreqOnly = "freq_only"
)

// runtimeTraceProjCapabilityCaliberClause maps a typed capability-source token
// to its inline disclosure words (CAP §26 C3 三态披露):
//
//	default_table  → 按默认算力比粗算 (the §26 default ratio table priced the
//	                 fold — coarse, not a vendor-measured capability table);
//	freq_only      → 簇结构不可判,按纯频率比折算 (fail-loud fallback: the
//	                 class gap is NOT priced);
//	evidence_table → 按实测算力表折算 (reserved — no evidence channel yet);
//	""             → "" (pre-CAP record: no claim, legacy wording preserved).
//
// Callers place the clause inside their own parenthesis/punctuation; the words
// themselves are the closed extension set the §26 ruling added to the §24.1
// caliber vocabulary, each backed by a legend entry
// (runtimeTraceProjMarkCaliberDefaultCapability / …FreqOnlyCapability).
func runtimeTraceProjCapabilityCaliberClause(source string, zh bool) string {
	switch source {
	case runtimeTraceCapabilitySourceDefault:
		if zh {
			return "按默认算力比粗算"
		}
		return "default capability-ratio estimate"
	case runtimeTraceCapabilitySourceEvidence:
		if zh {
			return "按实测算力表折算"
		}
		return "measured capability table"
	case runtimeTraceCapabilitySourceFreqOnly:
		if zh {
			return "簇结构不可判,按纯频率比折算"
		}
		return "cluster structure unjudged, frequency-ratio fold only"
	default:
		return ""
	}
}

// runtimeTraceProjCapabilityCaliberSuffix is the joined form for insertion at
// the tail of an existing caliber parenthesis: ",<clause>" (zh) / ", <clause>"
// (EN); "" for no claim.
func runtimeTraceProjCapabilityCaliberSuffix(source string, zh bool) string {
	clause := runtimeTraceProjCapabilityCaliberClause(source, zh)
	if clause == "" {
		return ""
	}
	if zh {
		return "," + clause
	}
	return ", " + clause
}

// runtimeTraceProjCapabilityCaliberMark returns the legend mark backing the
// clause (ok=false for no claim). The evidence form teaches through the
// default-capability seat's neighbour — it names its own table source inline
// and needs no ratio-table legend, so it maps to no mark until the evidence
// channel lands with its own wording ruling.
func runtimeTraceProjCapabilityCaliberMark(source string) (runtimeTraceProjMark, bool) {
	switch source {
	case runtimeTraceCapabilitySourceDefault:
		return runtimeTraceProjMarkCaliberDefaultCapability, true
	case runtimeTraceCapabilitySourceFreqOnly:
		return runtimeTraceProjMarkCaliberFreqOnlyCapability, true
	default:
		return 0, false
	}
}

// runtimeTraceProjFoldReferenceClusterWord (CAP 复核 F1 判词随实际基准簇) maps
// the typed demoted-reference class token to the basis-cluster word. Empty /
// "big" / any unrecognized token (the producer mints exactly small/middle/
// prime on demotion) keep the legacy 大核 basis word — absence means the
// nominated big-class basis, never a guess.
func runtimeTraceProjFoldReferenceClusterWord(refClass string, zh bool) (string, bool) {
	switch refClass {
	case "small":
		if zh {
			return "小核", true
		}
		return "small-cluster", true
	case "middle":
		if zh {
			return "中核", true
		}
		return "middle-cluster", true
	case "prime":
		if zh {
			return "超大核", true
		}
		return "prime-cluster", true
	default:
		if zh {
			return "大核", false
		}
		return "big-cluster", false
	}
}

// runtimeTraceProjFoldReferenceMark picks the caliber legend seat for the
// fold-basis word: the legacy 按大核满频折算 entry for the big-class basis,
// the demoted-reference entry otherwise (its legend explains the same-cluster
// demotion the class word signals).
func runtimeTraceProjFoldReferenceMark(refClass string) runtimeTraceProjMark {
	if _, demoted := runtimeTraceProjFoldReferenceClusterWord(refClass, true); demoted {
		return runtimeTraceProjMarkCaliberReferenceClusterFmax
	}
	return runtimeTraceProjMarkCaliberBigCoreFmax
}

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

// PTV8-RCR-A (§24 ②, 2026-07-08). EVOLUTION RECORD: the F-4 helper
// runtimeTraceProjSupplyFoldEmbedsInversionComposition is RETIRED with the
// Triple clause's inversion embed — inversion cause nodes render the
// four-line grammar (行3+拆解子行) instead of any mechanism sentence, so no
// surface needs the embed-suppression check anymore.

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
// mislabel this clause exists to kill.
//
// PTV8-RCR-A (§24 ②, 2026-07-08). EVOLUTION RECORD: the former consumers —
// the Triple clause's 内含 parenthetical, the independent 影响构成 tag and
// the §21 RNB R1 ⧖ runnable sub-row — are ALL retired; this text survives as
// the FAIL-OPEN lossless mirror only (detail block 有效归因构成 when the 行3
// identity Σ计入==V cannot balance and therefore refuses to render).
func runtimeTraceProjInversionCompositionText(node types.TraceCausalProjectionNode, zh bool) string {
	// CAP (§26 C3): the discounted running component discloses its capability
	// caliber (the runnable component is counted in full — no fold, no claim).
	capSuffix := runtimeTraceProjCapabilityCaliberSuffix(node.GatedCapabilitySource, zh)
	if zh {
		return fmt.Sprintf("runnable %.3fms(全额)+ running 折算 %.3fms(按下游消费核折算%s)", node.GatedRunnableMS, node.GatedRunningDeficitMS, capSuffix)
	}
	return fmt.Sprintf("runnable %.3fms (in full) + discounted running %.3fms (folded at the downstream consumer core%s)", node.GatedRunnableMS, node.GatedRunningDeficitMS, capSuffix)
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
	// CAP (§26 C3): every branch that states folded numbers or the affirmative
	// no-deficit claim discloses the fold's capability caliber (typed token —
	// empty on pre-CAP records keeps the legacy wording byte-identical). The
	// no-claim "无法折算" branch stays bare: nothing was folded.
	capSuffix := runtimeTraceProjCapabilityCaliberSuffix(node.SupplyFoldCapabilitySource, zh)
	// CAP 复核 F1 (判词随实际基准簇): the basis-cluster word follows the typed
	// demoted-reference class; the big-class basis keeps every legacy string
	// byte-identically.
	refWord, refDemoted := runtimeTraceProjFoldReferenceClusterWord(node.SupplyFoldReferenceClass, zh)
	switch verdict {
	case runtimeTraceProjSupplyFoldTriple:
		// PTV8-RCR-A (§24 ②, 2026-07-08). EVOLUTION RECORD: the Triple
		// branch's inversion embed (…·优先级反转 X(gated 口径,内含 …)) is
		// RETIRED — inversion cause nodes render the four-line grammar and
		// suppress this clause entirely; the "gated" user-facing word leaves
		// the display layer with it (wire tokens untouched). The residual
		// Triple shape (an inversion row that is NOT a cause node) renders the
		// two-caliber WithDemand text below — same wording home, no inversion
		// member, no summing frame.
		fallthrough
	case runtimeTraceProjSupplyFoldWithDemand:
		if zh {
			return fmt.Sprintf("机制构成(各口径独立、不可加和): 供给折算缺口 %.3fms(按%s满频折算,下界%s)·%s runnable %.3fms(就绪排队积压口径)",
				deficit, refWord, capSuffix, runtimeTraceSupplyPressureDisplayLabel(true), node.RunnableMS), "机制构成", true
		}
		return fmt.Sprintf("mechanism (each caliber is independent and not additive): supply-fold deficit %.3fms (folded at %s fmax, lower bound%s) · %s runnable %.3fms (ready-queue backlog caliber)",
			deficit, refWord, capSuffix, runtimeTraceSupplyPressureDisplayLabel(false), node.RunnableMS), "mechanism", true
	case runtimeTraceProjSupplyFoldDominant:
		if zh {
			return fmt.Sprintf("供给折算缺口 %.3fms(按%s满频折算,下界%s)为主,running 时间含降频/小核导致的跑慢成分", deficit, refWord, capSuffix), "供给折算缺口", true
		}
		return fmt.Sprintf("supply-fold deficit %.3fms (folded at %s fmax, lower bound%s) leads; running time carries a slow share from down-clocking / little cores", deficit, refWord, capSuffix), "supply-fold deficit", true
	case runtimeTraceProjSupplyFoldNoDeficit:
		// Affirmative exclusion (§7.10 fourth branch, via_thread-NOT family
		// value): only a fully-known basis may make this claim.
		//
		// PTV8-RCR-C (§24.9 维度A F3 / G4 两形, 2026-07-08). EVOLUTION RECORD:
		// this branch spoke ONE sentence for the whole 0 ≤ deficit < 阈 band —
		// "无供给缺口" printed beside 有效归因0.186ms whose entire semantics IS
		// that deficit (§20.2 made the wording deny its own neighbour number).
		// The wording now forks on the PRECISE deficit value (嘈声阈值只选句形,
		// 数字单源): deficit == 0 keeps the affirmative sentence byte-identical;
		// 0 < deficit < 阈 names the deficit and its attribution relation (the
		// counted tail only on the §20.2 identity eff==deficit) — the sentence
		// never denies the number beside it.
		// CAP (§26 判词重判): under the capability fold a zero/near-zero
		// deficit now asserts big-CLASS full-frequency equivalence — a small
		// core at its own fmax mints a deficit and structurally leaves this
		// branch (engine witness pin). The affirmative wording therefore
		// stands, but it must carry its capability caliber: under freq_only
		// the class gap was NOT priced and the claim is frequency-only.
		if deficit > 0 {
			counted := runtimeTraceProjRound3Equal(node.EffectiveImpactMS, deficit)
			switch {
			case zh && counted:
				return fmt.Sprintf("接近%s满频,缺口仅 %.3fms(已计入有效归因%s)", refWord, deficit, capSuffix), "接近" + refWord + "满频", true
			case zh:
				return fmt.Sprintf("接近%s满频,缺口仅 %.3fms(独立口径,不计入有效归因%s)", refWord, deficit, capSuffix), "接近" + refWord + "满频", true
			case counted:
				return fmt.Sprintf("near %s fmax; the deficit is only %.3fms (counted into the attribution%s)", refWord, deficit, capSuffix), "near fmax", true
			default:
				return fmt.Sprintf("near %s fmax; the deficit is only %.3fms (independent caliber, not counted into attribution%s)", refWord, deficit, capSuffix), "near fmax", true
			}
		}
		capParen := ""
		if clause := runtimeTraceProjCapabilityCaliberClause(node.SupplyFoldCapabilitySource, zh); clause != "" {
			if zh {
				capParen = "(" + clause + ")"
			} else {
				capParen = " (" + clause + ")"
			}
		}
		if zh {
			return "已按" + refWord + "满频(或接近)运行,无供给缺口,running 为真实工作量" + capParen, "已按" + refWord + "满频", true
		}
		if refDemoted {
			return "ran at (near) full frequency on the " + refWord + " basis; running is true workload" + capParen, "full frequency", true
		}
		return "ran at (near) full frequency on the top cluster; running is true workload" + capParen, "full frequency", true
	default: // runtimeTraceProjSupplyFoldUnknownBasis
		// PTV8-RCR-C (§24.9 G4 co-repair, 2026-07-08): a partially-unknown
		// basis CAN still have minted a lower-bound deficit (known slices fold,
		// unknown slices fold at ratio 1 and mint none) — "无法折算" beside a
		// published deficit denied the neighbour number the same way the old
		// NoDeficit form did. The deficit shape states the lower bound; the
		// no-deficit shape keeps the honest incomplete-data sentence
		// byte-identically.
		if deficit > 0 {
			if zh {
				return fmt.Sprintf("CPU 频率数据部分缺失,已计部分按%s满频折算:缺口 %.3fms 为下界%s", refWord, deficit, capSuffix), "CPU 频率数据部分缺失", true
			}
			return fmt.Sprintf("frequency data partially missing; the known share folded at %s fmax: the %.3fms deficit is a lower bound%s", refWord, deficit, capSuffix), "frequency data partially missing", true
		}
		if zh {
			return "CPU 频率数据不全,无法折算", "CPU 频率数据不全", true
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
