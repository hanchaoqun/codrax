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

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
	"strings"
)

// Soft-face thresholds of the §7.10 decision table. Noisy-signal guidance
// only: they select WORDING, never a hard gate, never rank.
const (
	runtimeTraceProjSupplyFoldDeficitShare   = 0.20
	runtimeTraceProjSupplyFoldDeficitFloorMs = 1.0
)

// RNB-5B 件③ (§29.96.2 终判③, 2026-07-15) — the 「接近<basis>」 word-form
// double gate: the NEAR claim may only render when the deficit is small BOTH
// absolutely AND relative to the seat's own raw running (缺口/原始). The E8
// negative witness (donghu 17267 E19 form: raw 1.409ms, deficit 0.933ms —
// relative 66%) wore 「接近全域最大核最高频」 on the strength of the absolute
// floor alone while two thirds of its running was below-peak. Seats failing
// the double gate state the deficit + the R5b mention fact WITHOUT the 接近
// claim. Dedicated constants — never borrowed from the §7.10 decision-table
// thresholds above (容差常量禁跨语义借用).
const (
	runtimeTraceProjNearPeakAbsFloorMs = 1.0
	runtimeTraceProjNearPeakRelShare   = 0.15
)

// runtimeTraceProjNearPeakDoubleGate reports whether the 接近 word may render:
// deficit < 1.0ms (absolute) AND deficit/raw-running < 15% (relative). Two
// typed values, pure comparison; rawRunning ≤ 0 fails the gate (a NEAR claim
// with no measurable base is unprovable).
func runtimeTraceProjNearPeakDoubleGate(deficit, rawRunning float64) bool {
	return deficit < runtimeTraceProjNearPeakAbsFloorMs &&
		rawRunning > 0 && deficit/rawRunning < runtimeTraceProjNearPeakRelShare
}

// CAP (§26 C3) capability-source wire tokens — byte-identical mirrors of
// tracequery's CoreCapabilitySource* constants (core_capability.go; equality
// pinned cross-package in the CAP display tests). The display layer keys the
// disclosure wording on these EXACT tokens, never on re-derived heuristics.
const (
	runtimeTraceCapabilitySourceDefault  = "default_table"
	runtimeTraceCapabilitySourceEvidence = "evidence_table"
	runtimeTraceCapabilitySourceFreqOnly = "freq_only"
)

// CAP-2 (§28.4/§28.5) cluster-topology-source wire tokens — byte-identical
// mirrors of tracequery's CoreCapabilityTopology* constants. Empty = explicit
// topology / legacy record: the §26 default-table wording stands
// byte-identically (absence preserves every pre-CAP-2 surface).
const (
	runtimeTraceCapabilityTopologyComovement = "freq_comovement"
	runtimeTraceCapabilityTopologyKeyedRail  = "keyed_rail"
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
//
// CAP-2 (§28.4/§28.5 三级披露词): the default-table word forks on the typed
// cluster-topology token — the "簇结构不可判" degrade wording's structural
// UPGRADE forms when the structure evidence exists:
//
//	topo ""              → 按默认算力比粗算            (explicit/legacy, byte-identical)
//	topo freq_comovement → 按实测频点共动分簇折算       (Tier-1 — membership measured
//	                       from co-moving cpu_frequency change points; the
//	                       default-ratio coefficient detail moves to its legend)
//	topo keyed_rail      → 按簇轨实测折算(成员按锚点连续推定) (Tier-2 — the rail claim
//	                       and the membership PRESUMPTION are worded separately
//	                       by ruling)
func runtimeTraceProjCapabilityCaliberClause(source string, zh bool) string {
	return runtimeTraceProjCapabilityCaliberClauseTopo(source, "", zh)
}

// runtimeTraceProjCapabilityCaliberClauseTopo is the reason-less compatibility
// wrapper (see runtimeTraceProjCapabilityCaliberClauseReason).
// EVOLUTION RECORD (DISPHYG-3 件7, 2026-07-20): the gated lane's deliberate
// batch boundary is CLOSED — its wire now carries the
// gated_capability_freq_only_reason twin and both gated emission sites feed
// the reason-aware single point; this wrapper remains for the reason-less
// legacy caller family only.
func runtimeTraceProjCapabilityCaliberClauseTopo(source, topo string, zh bool) string {
	return runtimeTraceProjCapabilityCaliberClauseReason(source, topo, "", zh)
}

// Typed freq_only cause-token mirrors — byte-identical to tracequery's
// CoreCapabilityFreqOnlyReason* closed set (the CAP token-mirror precedent
// above; equality pinned cross-package). CLUSTERSTREAM-1 件3 (§29.193.1,
// dossier §3.3): the freq_only wording forks PER ARM — six of the seven
// causes used to fold into one 「簇结构不可判」 sentence, the self-diagnosis
// gap the dossier's test_trace_02 three-way ambiguity exposed. An absent
// token (pre-batch records) keeps the ruled generic wording byte-identically.
const (
	runtimeTraceCapabilityFreqOnlyReasonNoDomains        = "no_domains"
	runtimeTraceCapabilityFreqOnlyReasonNoSampledCluster = "no_sampled_cluster"
	runtimeTraceCapabilityFreqOnlyReasonSingleCluster    = "single_cluster"
	runtimeTraceCapabilityFreqOnlyReasonClusterOverflow  = "cluster_overflow"
	runtimeTraceCapabilityFreqOnlyReasonFmaxTie          = "fmax_tie"
	runtimeTraceCapabilityFreqOnlyReasonComoveFloor      = "comove_floor"
	runtimeTraceCapabilityFreqOnlyReasonComoveFloorBurst = "comove_floor_single_burst"
)

// runtimeTraceCapabilityFreqOnlyReasonClosedSet is the FULL closed token set
// above plus "" (the reason-less generic arm) — the single roster every
// derived word-face consumer walks (复核 F2/F4, 2026-07-21): the legend
// cause-enumeration completeness pin iterates it (图例是承诺面 — a row cause
// word outside the legend enumeration is a promise-face mismatch), and the
// wrap-atom table derives the causes' unbreakable sub-clauses from it
// (DISPLAY-HYG 主张词不可断). A future arm added to the constants without
// joining this roster escapes both guards — extend them together.
var runtimeTraceCapabilityFreqOnlyReasonClosedSet = []string{
	"",
	runtimeTraceCapabilityFreqOnlyReasonNoDomains,
	runtimeTraceCapabilityFreqOnlyReasonNoSampledCluster,
	runtimeTraceCapabilityFreqOnlyReasonSingleCluster,
	runtimeTraceCapabilityFreqOnlyReasonClusterOverflow,
	runtimeTraceCapabilityFreqOnlyReasonFmaxTie,
	runtimeTraceCapabilityFreqOnlyReasonComoveFloor,
	runtimeTraceCapabilityFreqOnlyReasonComoveFloorBurst,
}

// runtimeTraceProjFreqOnlyCauseShort is the SHORT cause phrase per typed
// freq_only arm (word-face single point for the compressed no-deficit
// parenthetical and the joined caliber suffix). The empty token (pre-batch
// wire) and any unknown future token render the ruled generic cause. The two
// comove-floor tokens share one phrase (same cause, the burst token is a
// disclosure refinement).
func runtimeTraceProjFreqOnlyCauseShort(reason string, zh bool) string {
	switch reason {
	case runtimeTraceCapabilityFreqOnlyReasonSingleCluster:
		if zh {
			return "仅单簇有频点采样"
		}
		return "single-cluster samples only"
	case runtimeTraceCapabilityFreqOnlyReasonFmaxTie:
		if zh {
			return "簇最高频并列,核类排序不可判"
		}
		return "cluster peak frequencies tie — class order unjudgeable"
	case runtimeTraceCapabilityFreqOnlyReasonClusterOverflow:
		if zh {
			return "簇数超出核类表(>4)"
		}
		return "cluster count exceeds the class table (>4)"
	case runtimeTraceCapabilityFreqOnlyReasonComoveFloor, runtimeTraceCapabilityFreqOnlyReasonComoveFloorBurst:
		if zh {
			return "簇合并证据不足(共见证变迁<2)"
		}
		return "insufficient cluster-merge evidence (co-witnessed transitions <2)"
	case runtimeTraceCapabilityFreqOnlyReasonNoDomains:
		if zh {
			return "无频点采样"
		}
		return "no frequency samples"
	case runtimeTraceCapabilityFreqOnlyReasonNoSampledCluster:
		if zh {
			return "声明簇均无频点采样"
		}
		return "declared clusters carry no frequency samples"
	default:
		if zh {
			return "簇结构不可判"
		}
		return "cluster structure unjudged"
	}
}

// runtimeTraceProjCapabilityCaliberClauseReason is the topology- and
// reason-aware SINGLE SOURCE of the capability caliber clause (word-face
// single point). CLUSTER-FIX-2 件1 (S1 审计底稿 2026-07-18) opened the fork
// with the single-cluster arm; CLUSTERSTREAM-1 件3 (§29.193.1, dossier §3.3)
// completes it PER ARM — every typed freq_only cause now names itself
// (fmax_tie / cluster_overflow / comove_floor(±burst) / no_domains /
// no_sampled_cluster), because six causes folded into one 「簇结构不可判」
// sentence were indistinguishable on the answer face (the test_trace_02
// three-way ambiguity = the self-diagnosis gap). Absence (pre-batch records,
// reason-less wire) keeps the ruled generic wording byte-identically.
func runtimeTraceProjCapabilityCaliberClauseReason(source, topo, reason string, zh bool) string {
	switch source {
	case runtimeTraceCapabilitySourceDefault:
		switch topo {
		case runtimeTraceCapabilityTopologyComovement:
			if zh {
				return "按实测频点共动分簇折算"
			}
			return "measured co-moving frequency clusters (default capability ratios)"
		case runtimeTraceCapabilityTopologyKeyedRail:
			if zh {
				return "按簇轨实测折算(成员按锚点连续推定)"
			}
			return "measured cluster-rail fold (membership by anchor contiguity)"
		}
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
		if reason == runtimeTraceCapabilityFreqOnlyReasonSingleCluster {
			// S1 bespoke full form (pinned bytes): the structure IS judged,
			// the missing piece is cross-cluster capability information.
			if zh {
				return "仅单簇有频点采样,无跨簇算力信息,按纯频率比折算(单簇内等价)"
			}
			return "only one cluster carries frequency samples — no cross-cluster capability information, frequency-ratio fold only (equivalent within the single cluster)"
		}
		cause := runtimeTraceProjFreqOnlyCauseShort(reason, zh)
		if zh {
			return cause + ",按纯频率比折算"
		}
		return cause + ", frequency-ratio fold only"
	default:
		return ""
	}
}

// runtimeTraceProjCapabilityCaliberSuffix is the joined form for insertion at
// the tail of an existing caliber parenthesis: ",<clause>" (zh) / ", <clause>"
// (EN); "" for no claim.
func runtimeTraceProjCapabilityCaliberSuffix(source string, zh bool) string {
	return runtimeTraceProjCapabilityCaliberSuffixTopo(source, "", zh)
}

// runtimeTraceProjCapabilityCaliberSuffixTopo is the topology-aware suffix.
func runtimeTraceProjCapabilityCaliberSuffixTopo(source, topo string, zh bool) string {
	return runtimeTraceProjCapabilityCaliberSuffixReason(source, topo, "", zh)
}

// runtimeTraceProjCapabilityCaliberSuffixReason is the reason-aware suffix
// (CLUSTER-FIX-2 件1 — see the clause single source). CLUSTERSTREAM-1 件3
// 并注 (§29.193.1; dossier §4 witness runnable_2.txt:225): every suffix
// consumer appends inside a sentence that already carries a 折算 verb
// (「折算,按全域最高频,…」 / 「按X折算,下界…」), so the freq_only clause's own
// trailing 「按纯频率比折算」 duplicated the verb — two notes spliced into one
// parenthesis (「按全域最高频折算…按纯频率比折算」, rhetorical redundancy,
// not a semantic conflict). The suffix therefore renders the MERGED single
// note: the typed cause phrase + the legend-taught short term 「按频率比」
// ("frequency-ratio basis") — same facts, one 折算 verb. The standalone
// CLAUSE form (above) keeps the full 「按纯频率比折算」 wording for
// verb-less contexts; non-freq_only sources keep their unmerged suffixes.
func runtimeTraceProjCapabilityCaliberSuffixReason(source, topo, reason string, zh bool) string {
	if source == runtimeTraceCapabilitySourceFreqOnly {
		cause := runtimeTraceProjFreqOnlyCauseShort(reason, zh)
		if zh {
			return "," + cause + ",按频率比"
		}
		return ", " + cause + ", frequency-ratio basis"
	}
	clause := runtimeTraceProjCapabilityCaliberClauseReason(source, topo, reason, zh)
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
	return runtimeTraceProjCapabilityCaliberMarkTopo(source, "")
}

// runtimeTraceProjCapabilityCaliberMarkTopo is the topology-aware legend seat
// (CAP-2: each upgraded word carries its own legend entry — 括注扩展须配图例).
func runtimeTraceProjCapabilityCaliberMarkTopo(source, topo string) (runtimeTraceProjMark, bool) {
	switch source {
	case runtimeTraceCapabilitySourceDefault:
		switch topo {
		case runtimeTraceCapabilityTopologyComovement:
			return runtimeTraceProjMarkCaliberComovementTopology, true
		case runtimeTraceCapabilityTopologyKeyedRail:
			return runtimeTraceProjMarkCaliberKeyedRailTopology, true
		}
		return runtimeTraceProjMarkCaliberDefaultCapability, true
	case runtimeTraceCapabilitySourceFreqOnly:
		return runtimeTraceProjMarkCaliberFreqOnlyCapability, true
	default:
		return 0, false
	}
}

// runtimeTraceProjFoldBasisWord is the R5 (§29.88.3/§29.88.12 单基准单算法,
// 2026-07-15) SINGLE SOURCE of the conversion-basis word — every running-
// conversion surface (inversion running component, supply-fold deficit
// clause, ◎ caliber note, detail sub-rows) speaks THIS basis:
//
//	judged clusters → 全域最大核最高频 / "the global max-core peak frequency"
//	freq_only       → 全域最高频 / "the global peak frequency" (UXR-1
//	                  §29.36.4 ② 核类词诚实门: an unjudged cluster structure
//	                  never wears a core-class word — the basis is still the
//	                  factual global maximum frequency point)
//
// EVOLUTION RECORD: replaces the 按下游消费核 (gated lane) and 按大核满频 /
// 按小核·中核·超大核满频 (fold lane + demotion) word family — the two
// algorithms unified onto one trace-global basis, so the demoted-reference
// words (runtimeTraceProjFoldReferenceClusterWord) have no producer and are
// retired with their legend seat.
func runtimeTraceProjFoldBasisWord(capabilitySource string, zh bool) string {
	if capabilitySource == runtimeTraceCapabilitySourceFreqOnly {
		if zh {
			return "全域最高频"
		}
		return "the global peak frequency"
	}
	if zh {
		return "全域最大核最高频"
	}
	return "the global max-core peak frequency"
}

// R5b (§29.88.7 用户裁定 2026-07-14) mention-obligation word — the third
// mention scenario: any running conversion with a non-zero gap against the
// R5 basis MUST name the fact (合流: it rides the deficit word family as the
// gap's cause disclosure, obligation-grade like 优化点无条件入正文).
const (
	runtimeTraceProjBelowPeakMentionZH = "运行频点非最高"
	runtimeTraceProjBelowPeakMentionEN = "running below peak frequency"
)

// runtimeTraceProjBelowPeakMention returns the R5b mention word.
func runtimeTraceProjBelowPeakMention(zh bool) string {
	if zh {
		return runtimeTraceProjBelowPeakMentionZH
	}
	return runtimeTraceProjBelowPeakMentionEN
}

// --- INV-SUPPLY 件① compound type word (§29.61.11/.11a, 2026-07-14) -----------

// runtimeTraceProjSupplyGapDominantSeat is the typed dominance predicate for
// ONE seat: an inversion row whose supply fold ran and whose published
// deficit dominates its published effective attribution (the shared
// types.TraceSupplyGapDominant inequality — the SAME criterion the model-face
// seat-composition fact consumes, internal/context). Pure typed comparison;
// it selects wording only (soft face — ranking, values and gates untouched).
//
// 普查结论 (work-order census, 2026-07-14): the Dominant supply-fold FORM
// also appears on NON-inversion seats — exactly the PURE running family
// (tieba witness: com.baidu.tieba-59566 算力供给候选 #7, eff 2.089 ==
// deficit 2.089), where the §20.2 running arm publishes eff = deficit by
// identity, making the ratio 100% by construction. The compression DISEASE
// the ruling names (行2 类型词单形 → prose drops the frequency component)
// does NOT exist there: that family's type word 算力供给候选 ALREADY names
// the supply mechanism, and 行3 speaks 折算,按全域最大核最高频 — appending
// 「·供给缺口主导」 would be a same-family tautology (§29.36.4 冗余判据).
// The suffix arm therefore covers seats whose type word names a NON-supply
// mechanism while the gap dominates — today exactly the inversion family;
// the composer below is the generic word+suffix form the ruling pre-approved
// should a future family need the same arm.
func runtimeTraceProjSupplyGapDominantSeat(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceCausalProjectionInversionRow(node) && node.SupplyFoldComputed &&
		// RNB-1 C-2② (§29.88.10 R7-2, 2026-07-14): the constitutive
		// precondition — the seat's EFFECTIVE composition must CONTAIN a
		// running-折算 component (typed GatedRunningDeficitMS > 0, the same
		// field the 行3 composition builder keys its running component on).
		// The deficit is an independent caliber that never enters eff; a
		// composition with ZERO supply component wearing 「供给缺口主导」 was
		// a self-contradicting word face (witness 20260714-230952 E31:
		// eff 0.423 = runnable(全额) only). The ≥50% ratio stays on top.
		node.GatedRunningDeficitMS > 0 &&
		types.TraceSupplyGapDominant(node.SupplyFoldDeficitMS, node.EffectiveImpactMS)
}

// runtimeTraceProjInversionSupplyGapCompoundWord is THE single composer of
// the compound type word 「优先级反转候选·供给缺口主导」 (zh) /
// "priority inversion (candidate) · supply-gap dominant" (en) — consumed by
// the 行2 category-word arm AND the ◎ overview class-word slot (转录制同词,
// 零新词源: the overview transcribes these exact bytes, never a re-spelling).
// Word bytes single-sourced in tracefence (table ③b); face separators follow
// the within-tag conventions (zh no-space "·", en spaced " · " — supply-fold
// clause F3 precedent). ok=false below the threshold or without a fold — the
// callers keep their pre-INV-SUPPLY words byte-identically.
func runtimeTraceProjInversionSupplyGapCompoundWord(node types.TraceCausalProjectionNode, zh bool) (string, bool) {
	if !runtimeTraceProjSupplyGapDominantSeat(node) {
		return "", false
	}
	if zh {
		return runtimeTraceRootCauseTypeZHLabel("priority_inversion_candidate") + "·" + tracefence.SupplyGapDominantWordZH, true
	}
	// EVOLUTION RECORD (RULE3-1 双复核修复 件1, §29.182① 「EN 同批」 + ②
	// EN 词表, 2026-07-21): the PTV6-C ruling-B D2 discipline (「the EN
	// inversion category word is the raw wire token」) is RETIRED — the EN
	// base word now consumes the same 件8 verdict table every sibling EN face
	// speaks (runtimeTraceRootCauseTypeENLabel); the raw wire token keeps its
	// seats on the detail 类型 column and the evidence/JSON keys only. The
	// suffix rides with the spaced en separator, unchanged.
	return runtimeTraceRootCauseTypeENLabel("priority_inversion_candidate") + " · " + tracefence.SupplyGapDominantWordEN, true
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
	// CAP-2: the wording upgrades on the typed cluster-topology token.
	// R5 (§29.88.12 单基准, 2026-07-15): the running component folds at the
	// 全域最大核最高频 basis (the 按下游消费核 wording retired with its
	// algorithm) and names the R5b mention fact (the component exists only
	// when the gap is non-zero).
	// DISPHYG-3 件7: the gated reason twin feeds the same clause single point
	// (see the rcr.go primary emission site).
	capSuffix := runtimeTraceProjCapabilityCaliberSuffixReason(node.GatedCapabilitySource, node.GatedTopologySource, node.GatedCapabilityFreqOnlyReason, zh)
	basisWord := runtimeTraceProjFoldBasisWord(node.GatedCapabilitySource, zh)
	mention := runtimeTraceProjBelowPeakMention(zh)
	if zh {
		return fmt.Sprintf("runnable %.3fms(全额)+ running 折算 %.3fms(%s,按%s折算%s)", node.GatedRunnableMS, node.GatedRunningDeficitMS, mention, basisWord, capSuffix)
	}
	return fmt.Sprintf("runnable %.3fms (in full) + discounted running %.3fms (%s, folded at %s%s)", node.GatedRunnableMS, node.GatedRunningDeficitMS, mention, basisWord, capSuffix)
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

// --- GATED-CAL 件1 (§29.104.16.1 M3 四面一根, 2026-07-16) ----------------------
//
// A gated composite value (runnable counted IN FULL + running deficit counted
// DISCOUNTED — the R5d inversion caliber the engine publishes on
// EffectiveImpactMs AND, for inversion rank rows, on ImpactMs; overwrite root
// = tracequery/query.go rootCauseItemFromCausalImpact/-Aggregate) used to
// impersonate a single caliber on four display faces (「全额」 false cover /
// window-projection column vs its legend promise / bare 有效归因X tag / ◎ bare
// state word). The helpers below are the ONE typed gate + ONE word source all
// four repaired faces consume — precise signals only (the same Gated* fields
// the 行3 composition builder keys on), so a pure full-amount seat and a pure
// discounted seat stay byte-identical on every face (negative arms pinned).

// runtimeTraceProjGatedCompositeShortWord is THE single composer of the
// GATED-CAL composite caliber word — the value is a multi-component
// composition, not one caliber; the split lives on the row's 行3 / detail
// block. Consumed by the 行2 degenerate tail, the Q1 bare-tag belt, the ◎
// caliber-note arm and the key-metric window-projection cell (四面同词).
func runtimeTraceProjGatedCompositeShortWord(zh bool) string {
	if zh {
		return "构成,见明细"
	}
	return "composite, see the detail blocks"
}

// runtimeTraceProjGatedCompositeSeat is the precise typed composite gate: the
// published effective CONTAINS BOTH the full-amount runnable component and the
// discounted running component (the same two fields the 行3 builder keys on),
// PROVEN by the print-precision inclusion identity eff == runnable+deficit
// (修补轮 件B, 双复核 P2-1, 2026-07-17 — same ruler as the projection-cell
// predicate below: components merely PUBLISHED beside a value they were not
// counted into must not flip the word; the tieba E15 shape carries a 0.073
// deficit outside its 8.049 pure-runnable value). Identity fails → NOT a
// composite: the ◎ keeps its bare word, the bare-tag belt stays off (宁缺勿造)
// — one predicate, three consumer faces (◎ 注记臂/类词臂/裸 tag 保底), one fix.
// Single-component gated values (pure runnable / pure deficit) are NOT
// composites — their existing single-caliber words stay byte-identical.
func runtimeTraceProjGatedCompositeSeat(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceCausalProjectionInversionRow(node) &&
		node.GatedRunnableMS > 0 && node.GatedRunningDeficitMS > 0 &&
		runtimeTraceProjRound3Equal(node.EffectiveImpactMS, node.GatedRunnableMS+node.GatedRunningDeficitMS)
}

// runtimeTraceProjGatedCompositeProjectionCell (件1③, M3-c) reports whether a
// key-metric window-projection CELL value is in fact the engine's gated
// composite: the rank lane publishes ImpactMs = PriorityInversionGatedMs on
// inversion rows (query.go 覆写根), so the cell then violates the column's
// 「该节点的状态落在分析窗内的时长」 promise. Print-precision identity on the
// typed component sum — a genuine state projection that merely coexists with
// gated fields (E13-shape: projection 8.294 beside gated 7.405) never
// annotates. Deficit-bearing values only: a pure gated-runnable cell IS
// wall-clock state time inside the window.
func runtimeTraceProjGatedCompositeProjectionCell(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceCausalProjectionInversionRow(node) &&
		node.GatedRunningDeficitMS > 0 && node.ImpactMS > 0 &&
		runtimeTraceProjRound3Equal(node.ImpactMS, node.GatedRunnableMS+node.GatedRunningDeficitMS)
}

// runtimeTraceProjFamilyValueIsGatedComposite (件1②, M3-b) reports that a
// FAMILY row's published value is the inversion machinery's gated product —
// NOT the family fold's own same-thread total — so the 「合计(共N段,同线程)」
// "=" claim would mislabel it. Print-precision identity on the typed component
// sum — the identity, not any deficit-free assumption, is the safety (修补轮
// 件D勘正, 2026-07-17: the runnable-overlap retype's SINGLE-row mint publishes
// deficit=0, but folded/merged carriers CAN carry a published-yet-uncounted
// deficit — tieba E15: 8.049 == gatedRunnable beside deficit 0.073; that shape
// fails this identity and stays on its existing lanes byte-identically, the
// DISPLAY-WRAP A3 E6 witness shape likewise).
func runtimeTraceProjFamilyValueIsGatedComposite(node types.TraceCausalProjectionNode) bool {
	if !runtimeTraceCausalProjectionInversionRow(node) || node.GatedRunningDeficitMS <= 0 {
		return false
	}
	v := runtimeTraceProjFamilyPublishedMS(node)
	return v > 0 && runtimeTraceProjRound3Equal(v, node.GatedRunnableMS+node.GatedRunningDeficitMS)
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
// lane's running-deficit COMPONENT — R5 (§29.88.12 单基准, 2026-07-15):
// both now fold against the SAME 全域最大核最高频 basis and are ONE number
// per seat (同源可互推); the non-additivity discipline stays (a deficit and
// a runnable backlog remain different calibers). RCX² 复核 F1: the inversion TOTAL is a gated composite, not a
// folded value — it wears only the gated-caliber word; the consumer-core
// ruler sits on the running component inside the composition text (the
// runnable component is counted in full and wears 全额). The ONE additive
// breakdown is that candidate's OWN gated split and it stays inside its
// parenthetical. F3: the zh face joins the calibers with the no-space "·"
// (the within-tag convention, e.g. 周期性信号源…·有效归因X), keeping it
// visually distinct from the BETWEEN-tag " · " separator so a neighbouring
// tag never reads as a fourth caliber; the EN face keeps the spaced " · "
// (its within-tag convention, e.g. "periodic signal source · attribution").
// runtimeTraceProjProseClauseRegime (DISPHYG-3 收编 P2-1, 2026-07-20) applies
// the C8 prose punctuation regime to a fence-shared zh clause at its LEAD
// embedding only: depth-0 half-width ,/; become ，/；, while parenthetical
// interiors (fence-shared word-face tokens) keep half-width. One transform
// helper at one lead call site = the composer stays a single word-face point
// (punctuation regime is presentation, parameterized like zh/EN), the fence
// 行2 face keeps its legacy bytes by construction, and the last same-sentence
// mixed-mark flagship line (donghu_2955 supply conclusion) dies.
func runtimeTraceProjProseClauseRegime(text string) string {
	depth := 0
	var b strings.Builder
	for _, r := range text {
		switch r {
		case '(', '（':
			depth++
		case ')', '）':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				b.WriteRune('，')
				continue
			}
		case ';':
			if depth == 0 {
				b.WriteRune('；')
				continue
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func runtimeTraceProjSupplyFoldClause(node types.TraceCausalProjectionNode, windowMS float64, zh bool) (string, string, bool) {
	text, keep, ok := runtimeTraceProjSupplyFoldClauseCore(node, windowMS, zh)
	if !ok {
		return text, keep, ok
	}
	// THERM (§28.5-T7, disclosure-only zero-weight edit): the typed in-window
	// thermal/policy press on the fold's dominant running cluster appends its
	// own sentence — it never changes a number and never denies a neighbour
	// value (数值+单位; absent field = no sentence, absence never guesses).
	if node.ThermalCapKHz > 0 {
		// CR-3 件⑥ F-10 (2026-07-12; CR-2 冷读 D5 witness — a carry-in cap
		// wore the thermal word with zero in-window event): the 受热限压
		// wording requires an IN-WINDOW limits/thermal event witness; an
		// unwitnessed press states the governed frequency without asserting
		// its cause (数值真实,标签不越权).
		if node.ThermalCapWitnessed {
			if zh {
				text += fmt.Sprintf(";窗内该簇受热限压至 %.2fGHz", float64(node.ThermalCapKHz)/1e6)
			} else {
				text += fmt.Sprintf("; a thermal/policy cap pressed this cluster to %.2fGHz in-window", float64(node.ThermalCapKHz)/1e6)
			}
		} else {
			if zh {
				text += fmt.Sprintf(";窗内该簇运行于 %.2fGHz(限压原因未见证)", float64(node.ThermalCapKHz)/1e6)
			} else {
				text += fmt.Sprintf("; this cluster ran governed at %.2fGHz in-window (cap cause unwitnessed)", float64(node.ThermalCapKHz)/1e6)
			}
		}
	}
	return text, keep, ok
}

// runtimeTraceProjSupplyFoldClauseCore is the §7.10 four-branch body (see
// runtimeTraceProjSupplyFoldClause for the THERM appendix).
func runtimeTraceProjSupplyFoldClauseCore(node types.TraceCausalProjectionNode, windowMS float64, zh bool) (string, string, bool) {
	verdict := runtimeTraceProjSupplyFoldVerdictFor(node, windowMS)
	if verdict == runtimeTraceProjSupplyFoldNone {
		return "", "", false
	}
	deficit := node.SupplyFoldDeficitMS
	// CAP (§26 C3): every branch that states folded numbers or the affirmative
	// no-deficit claim discloses the fold's capability caliber (typed token —
	// empty on pre-CAP records keeps the legacy wording byte-identical). The
	// no-claim "无法折算" branch stays bare: nothing was folded.
	// CAP-2: the wording upgrades on the typed cluster-topology token.
	// CLUSTER-FIX-2 件1 (S1): the freq_only wording forks on the typed
	// single-cluster cause token (absence = legacy bytes).
	capSuffix := runtimeTraceProjCapabilityCaliberSuffixReason(node.SupplyFoldCapabilitySource, node.SupplyFoldTopologySource, node.SupplyFoldCapabilityFreqOnlyReason, zh)
	// R5 单基准词面 (§29.88.3/§29.88.12): ONE basis word for every branch —
	// 全域最大核最高频 (judged) / 全域最高频 (freq_only; UXR-1 §29.36.4 ②
	// 核类词诚实门 preserved: no core-class word under an unjudged structure).
	// R5b (§29.88.7): every branch stating a non-zero gap ALSO names the
	// cause fact 运行频点非最高 (mention obligation, 合流 into the deficit
	// word family).
	basisWord := runtimeTraceProjFoldBasisWord(node.SupplyFoldCapabilitySource, zh)
	mention := runtimeTraceProjBelowPeakMention(zh)
	undecidable := node.SupplyFoldCapabilitySource == runtimeTraceCapabilitySourceFreqOnly
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
			return fmt.Sprintf("机制构成(各口径独立、不可加和): 供给折算缺口 %.3fms(%s,按%s折算,下界%s)·%s runnable %.3fms(就绪排队积压口径)",
				deficit, mention, basisWord, capSuffix, runtimeTraceSupplyPressureDisplayLabel(true), node.RunnableMS), "机制构成", true
		}
		return fmt.Sprintf("mechanism (each caliber is independent and not additive): supply-fold deficit %.3fms (%s, folded at %s, lower bound%s) · %s runnable %.3fms (ready-queue backlog caliber)",
			deficit, mention, basisWord, capSuffix, runtimeTraceSupplyPressureDisplayLabel(false), node.RunnableMS), "mechanism", true
	case runtimeTraceProjSupplyFoldDominant:
		// UXR-1 §29.36.4 ②: the 小核 class word in the slow-share tail is
		// forbidden when the cluster structure is undecidable.
		slowShareZH := "running 时间含降频/小核导致的跑慢成分"
		slowShareEN := "running time carries a slow share from down-clocking / little cores"
		if undecidable {
			slowShareZH = "running 时间含降频等导致的跑慢成分"
			slowShareEN = "running time carries a slow share from down-clocking"
		}
		if zh {
			return fmt.Sprintf("供给折算缺口 %.3fms(%s,按%s折算,下界%s)为主,%s", deficit, mention, basisWord, capSuffix, slowShareZH), "供给折算缺口", true
		}
		return fmt.Sprintf("supply-fold deficit %.3fms (%s, folded at %s, lower bound%s) leads; %s", deficit, mention, basisWord, capSuffix, slowShareEN), "supply-fold deficit", true
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
			// RNB-5B 件③ (§29.96.2 终判③): the 接近 claim requires the double
			// gate (absolute <1.0ms ∧ relative <15% of the raw running). A seat
			// failing it states the deficit + the R5b mention fact only — the
			// same magnitudes, no NEAR claim (E8 negative witness: 1.409/0.933,
			// relative 66%).
			if !runtimeTraceProjNearPeakDoubleGate(deficit, runtimeTraceProjSupplyFoldRunningMS(node)) {
				switch {
				case zh && counted:
					return fmt.Sprintf("供给折算缺口 %.3fms(%s,已计入有效归因%s)", deficit, mention, capSuffix), "供给折算缺口", true
				case zh:
					return fmt.Sprintf("供给折算缺口 %.3fms(%s,独立口径,不计入有效归因%s)", deficit, mention, capSuffix), "供给折算缺口", true
				case counted:
					return fmt.Sprintf("supply-fold deficit %.3fms (%s, counted into the attribution%s)", deficit, mention, capSuffix), "supply-fold deficit", true
				default:
					return fmt.Sprintf("supply-fold deficit %.3fms (%s, independent caliber, not counted into attribution%s)", deficit, mention, capSuffix), "supply-fold deficit", true
				}
			}
			switch {
			case zh && counted:
				return fmt.Sprintf("接近%s,缺口仅 %.3fms(%s,已计入有效归因%s)", basisWord, deficit, mention, capSuffix), "接近" + basisWord, true
			case zh:
				return fmt.Sprintf("接近%s,缺口仅 %.3fms(%s,独立口径,不计入有效归因%s)", basisWord, deficit, mention, capSuffix), "接近" + basisWord, true
			case counted:
				return fmt.Sprintf("near %s; the deficit is only %.3fms (%s, counted into the attribution%s)", basisWord, deficit, mention, capSuffix), "near " + basisWord, true
			default:
				return fmt.Sprintf("near %s; the deficit is only %.3fms (%s, independent caliber, not counted into attribution%s)", basisWord, deficit, mention, capSuffix), "near " + basisWord, true
			}
		}
		// UXR-1 §29.36.4 ① (推论链冗余判据, a4/2549 witness): the affirmative
		// sentence stays 证据+末端结论+口径括注 (the legend carries the
		// expanded semantics). Zero gap = the R5b mention's NEGATIVE arm: no
		// 运行频点非最高 claim may appear (禁无中生有).
		if undecidable {
			// CLUSTER-FIX-2 件1 (S1) opened this fork with the single-cluster
			// arm; CLUSTERSTREAM-1 件3 (§29.193.1) completes it per arm via
			// the shared short-cause single point (absence keeps the legacy
			// 簇结构不可判 bytes). All variants keep the legend-taught
			// 按频率比 term (runtimeTraceProjMarkCaliberFreqOnlyCapability
			// seat unchanged).
			cause := runtimeTraceProjFreqOnlyCauseShort(node.SupplyFoldCapabilityFreqOnlyReason, zh)
			if zh {
				return "已按" + basisWord + "(或接近)运行·无供给折算(" + cause + ",按频率比)", "已按" + basisWord, true
			}
			return "ran at (near) " + basisWord + " · no supply fold (" + cause + ", frequency-ratio basis)", "ran at (near) " + basisWord, true
		}
		capParen := ""
		if clause := runtimeTraceProjCapabilityCaliberClauseReason(node.SupplyFoldCapabilitySource, node.SupplyFoldTopologySource, node.SupplyFoldCapabilityFreqOnlyReason, zh); clause != "" {
			if zh {
				capParen = "(" + clause + ")"
			} else {
				capParen = " (" + clause + ")"
			}
		}
		if zh {
			return "已按" + basisWord + "(或接近)运行·无供给折算" + capParen, "已按" + basisWord, true
		}
		return "ran at (near) " + basisWord + " · no supply fold" + capParen, "ran at (near) " + basisWord, true
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
				return fmt.Sprintf("CPU 频率数据部分缺失,已计部分按%s折算:缺口 %.3fms 为下界(%s%s)", basisWord, deficit, mention, capSuffix), "CPU 频率数据部分缺失", true
			}
			return fmt.Sprintf("frequency data partially missing; the known share folded at %s: the %.3fms deficit is a lower bound (%s%s)", basisWord, deficit, mention, capSuffix), "frequency data partially missing", true
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
