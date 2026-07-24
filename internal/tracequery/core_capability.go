package tracequery

// core_capability.go — CAP batch (§26, docs/design/real_trace_campaign_
// 20260705.md, user ruling 2026-07-08): per-core-class CAPABILITY coefficients
// for the running supply folds. Frequency ratio alone under-prices weak cores:
// a small core at ITS full frequency still delivers a fraction of a big core's
// work per unit time, so both running folds now price
//
//	equivalent capacity = frequency × class capability coefficient
//
// This file is the SINGLE SOURCE for (a) the §26 default coefficient table,
// (b) the cluster-count → core-class mapping, and (c) the typed three-state
// capability-source disclosure tokens. No second capability judgment may grow
// beside resolveCoreCapability.
//
// Class identity discipline (precise signals only, 禁猜):
//
//   - Cluster membership comes from THE frequency-domain authority
//     (cluster_freq_share.go resolveClusterFreqDomains: explicit core_topology
//     verbatim labels, else the CFR-2 change-point derivation). The 3-class
//     frequency-tier INFERENCE (inferCoreTopologyFromFrequency, thirds-based)
//     is a noisy signal and never feeds capability classes. CAP-3 (§29.11):
//     the derivation basis is the Index-global sample stream under the
//     boundary-robust co-movement criterion (freqTimelinesCoMove) — cluster
//     topology is a trace attribute, judged once per Index and shared by
//     every governance window (同 trace 全窗折算词一致 acceptance, pinned by
//     TestSupplyFoldCapabilityTokensConsistentAcrossWindows +
//     TestCoreCapabilityCap2FourWindowParity).
//   - Classes map from the SAMPLED cluster structure ordered by full-trace
//     fmax (§26: 2 簇=小+大;3 簇=小+中+大;4 簇=小+中+大+超大). Full-trace —
//     not window-governed — because class identity is a hardware attribute;
//     the deficit VALUES keep the CMP-10 F1 governance caliber untouched.
//   - Fail-loud fallback (§26): a cluster structure that cannot be judged —
//     no resolvable domains, fewer than 2 or more than 4 sampled clusters, or
//     an fmax tie between clusters (no defensible order) — falls back to the
//     PURE FREQUENCY RATIO legacy formula with the typed freq_only disclosure
//     ("簇结构不可判,按纯频率比折算"). Never a guessed class.
//   - Evidence channel (§26 首选 cpu_capacity 类打点/厂商表工件): the parse
//     surfaces carry NO such channel today (scanned 2026-07-08) — the
//     evidence_table token is reserved wiring for when one lands; this batch
//     ships the default table only.
//
// Declared-but-never-sampled clusters (explicit topology) are excluded from
// the ladder AND the count: they cannot be placed on an fmax order, and their
// member slices can never carry a governed frequency anyway (donor reuse is
// same-cluster only), so they fold as UNKNOWN basis exactly as before.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// §26 default capability coefficients (同频点; single source — every consumer
// reads coreCapabilityDefaultByClass). Derivation pinned by the ruling:
// 中核 = 小核×2.3; 大核 = 中核×1.1 (= 小核×2.53); 超大核 = 大核×1.2
// (= 小核×3.036); 恒序 超大核>大核>中核>小核.
const (
	coreCapabilityDefaultSmall  = 1.0
	coreCapabilityDefaultMiddle = 2.3
	coreCapabilityDefaultBig    = 2.53
	coreCapabilityDefaultPrime  = 3.036
)

// Capability class labels (internal 4-class taxonomy; deliberately NOT the
// normalizeCoreClass display taxonomy, which folds prime into big).
const (
	coreCapabilityClassSmall  = "small"
	coreCapabilityClassMiddle = "middle"
	coreCapabilityClassBig    = "big"
	coreCapabilityClassPrime  = "prime"
)

// Typed cluster-topology-source tokens (CAP-2 §28.4/§28.5 三级披露词):
// WHERE the cluster STRUCTURE came from, orthogonal to the coefficient-table
// source above. Only the two evidence forms are minted — explicit-topology and
// pre-CAP-2 records stay token-less so their wire bytes and display wording
// remain byte-identical (legacy preserved by absence).
const (
	// CoreCapabilityTopologyComovement: Tier-1 — membership from the measured
	// cpu_frequency co-movement derivation (display: 按实测频点共动分簇折算).
	CoreCapabilityTopologyComovement = "freq_comovement"
	// CoreCapabilityTopologyKeyedRail: Tier-2 structure was USED — pure
	// keyed-rail membership, or a rail-anchor subdivision of Tier-1 clusters
	// (display: 按簇轨实测折算(成员按锚点连续推定)).
	CoreCapabilityTopologyKeyedRail = "keyed_rail"
)

// Typed capability-source tokens (§26 C3 三态披露: the display layer keys the
// "(按默认算力比粗算)" / "簇结构不可判,按纯频率比折算" wording on these, and
// ONLY on these — never on a re-derived heuristic).
const (
	// CoreCapabilitySourceDefault: the §26 default ratio table priced the fold
	// (no vendor-measured capacity evidence — coarse by declaration).
	CoreCapabilitySourceDefault = "default_table"
	// CoreCapabilitySourceEvidence: reserved for an in-trace capacity evidence
	// channel (cpu_capacity-class rows / vendor table artifacts). No parse
	// surface produces it yet (§26 scan 2026-07-08) — wiring only.
	CoreCapabilitySourceEvidence = "evidence_table"
	// CoreCapabilitySourceFreqOnly: cluster structure unjudgeable — the fold
	// ran the legacy pure-frequency-ratio formula (fail-loud disclosure).
	CoreCapabilitySourceFreqOnly = "freq_only"
)

// Typed freq_only CAUSE tokens (CLUSTER-FIX-2 件1, S1 审计底稿 2026-07-18
// docs/design/cluster_audit_code_20260718.md: 「freq_only 五个成因下游不可
// 分辨…加 typed freqOnlyReason 枚举(精确信号:groupCount、sampledAsc 长度,
// 已有)」). CLOSED SET — every resolveCoreCapabilityEvidence arm that leaves
// the verdict at freq_only mints exactly one of these; a judged verdict mints
// none. Pure disclosure lane (wire + note + wording fork input): no gate may
// ever read a reason token (precise-signals red line — the freq_only verdict
// itself is the gate; the reason only says WHICH arm fired).
//
// Deviation from the audit's five-token sketch (no_samples/single_cluster/
// >4/tie/floor), recorded: the code has SEVEN distinguishable arms — the
// audit's no_samples splits into no_domains (no resolvable membership at all)
// and no_sampled_cluster (explicit-topology membership exists but no cluster
// carries any fmax evidence), and the comove floor arm gains the
// single-burst refinement below (C1). Still a closed typed set.
const (
	// CoreCapabilityFreqOnlyReasonNoDomains: no resolvable cluster membership
	// at all — zero cpu_frequency samples (or all poisoned), no explicit
	// topology, no adoptable rail family.
	CoreCapabilityFreqOnlyReasonNoDomains = "no_domains"
	// CoreCapabilityFreqOnlyReasonNoSampledCluster: membership resolved
	// (explicit topology) but ZERO clusters carry any fmax evidence — nothing
	// to order.
	CoreCapabilityFreqOnlyReasonNoSampledCluster = "no_sampled_cluster"
	// CoreCapabilityFreqOnlyReasonSingleCluster: exactly ONE sampled cluster —
	// the container-side single-policy capture norm (xxx_all.systrace /
	// tieba witness: only one policy's notifier is traced). S1: the structure
	// IS judged (one cluster, leading cores closed in); what is missing is
	// CROSS-cluster capability information — the display words this form
	// honestly instead of「簇结构不可判」.
	CoreCapabilityFreqOnlyReasonSingleCluster = "single_cluster"
	// CoreCapabilityFreqOnlyReasonClusterOverflow: more than 4 sampled
	// clusters — outside the §26 class table (fragmentation shape or a true
	// 5-domain SoC; the split-audit localizes the first split).
	CoreCapabilityFreqOnlyReasonClusterOverflow = "cluster_overflow"
	// CoreCapabilityFreqOnlyReasonFmaxTie: two adjacent-order clusters share
	// one fmax — no defensible class order (禁掷币).
	CoreCapabilityFreqOnlyReasonFmaxTie = "fmax_tie"
	// CoreCapabilityFreqOnlyReasonComoveFloor: the Tier-1 sample floor
	// (clusterFreqComoveMinSamples) tripped — a multi-CPU merge whose
	// representative timeline carries fewer samples than the ruled floor.
	CoreCapabilityFreqOnlyReasonComoveFloor = "comove_floor"
	// CoreCapabilityFreqOnlyReasonComoveFloorSingleBurst (CLUSTER-FIX-2 件3,
	// C1 底稿 cluster_audit_refs_20260718.md): the floor form above WHERE the
	// offending cluster's merge evidence is exactly ONE co-emission burst —
	// every sampled member carries exactly one positive sample, all values
	// equal, timestamps within the FIXED clusterFreqDeriveMaxSkewSec bound
	// (§29.129 既裁③: burst 展宽自适应禁入硬门 — the constant is never
	// widened), and the sampled member ids form one contiguous ascending run.
	// The burst structure C1 proposes as a membership witness IS detected and
	// disclosed here; ADMITTING it as a merge hard gate stays a delegated
	// ruling point — it would reverse the §28.5 复核 P1 floor ruling (an
	// all-policy announcement sweep of two clusters parked at one value
	// satisfies every literal burst condition, the exact false-merge shape
	// the floor was ruled against; the refs' fourth condition — emitter
	// comm/pid identity — is not materializable on the freqSample basis and
	// would not discriminate anyway: one tppmgr thread emits every policy).
	CoreCapabilityFreqOnlyReasonComoveFloorSingleBurst = "comove_floor_single_burst"
)

// EVOLUTION RECORD (R5 §29.88.3 landed 2026-07-14; dead-code retirement
// 2026-07-15): the §26 big-class fold-reference NOMINATION
// (coreCapabilityReferenceClass) and its write-only carrier field
// (coreCapabilityMap.refCap) are RETIRED — the live fold reads its
// (fmax, cap, class) triple from supplyFoldGlobalMaxBasis's top judged
// cluster, and the retired demotion walk was the nomination's only
// consumer. The load-bearing rule survives where it is enforced: the two
// basis values MUST stay same-cluster same-source (inside
// supplyFoldGlobalMaxBasis) — mixing one cluster's fmax with another
// cluster's cap fabricated deficits (复核 Probe A: 1.650ms on a
// full-frequency big core; Probe B: 5.987ms on a small-only governance
// window).

// coreCapabilityClassRank orders the 4-class taxonomy for the reference
// demotion walk (highest sampled class wins when the nominated cluster has no
// window-governed fmax). Deliberately NOT coreClassRank — that display helper
// folds every non-{small,middle,big} label (prime included) into one bucket.
var coreCapabilityClassRank = map[string]int{
	coreCapabilityClassSmall:  0,
	coreCapabilityClassMiddle: 1,
	coreCapabilityClassBig:    2,
	coreCapabilityClassPrime:  3,
}

// coreCapabilityClassesByClusterCount is the §26 structural mapping: sampled
// clusters ordered by ascending full-trace fmax take these classes in order.
var coreCapabilityClassesByClusterCount = map[int][]string{
	2: {coreCapabilityClassSmall, coreCapabilityClassBig},
	3: {coreCapabilityClassSmall, coreCapabilityClassMiddle, coreCapabilityClassBig},
	4: {coreCapabilityClassSmall, coreCapabilityClassMiddle, coreCapabilityClassBig, coreCapabilityClassPrime},
}

// coreCapabilityDefaultByClass is the §26 default coefficient table (single
// source; see the constants above for the pinned derivation).
var coreCapabilityDefaultByClass = map[string]float64{
	coreCapabilityClassSmall:  coreCapabilityDefaultSmall,
	coreCapabilityClassMiddle: coreCapabilityDefaultMiddle,
	coreCapabilityClassBig:    coreCapabilityDefaultBig,
	coreCapabilityClassPrime:  coreCapabilityDefaultPrime,
}

// coreCapabilityMap is one resolved capability judgment: cluster → class →
// coefficient, plus the typed source disclosure. Zero value / freq_only maps
// price every CPU at coefficient 1 (pure frequency ratio — legacy behavior).
type coreCapabilityMap struct {
	// source is one of the CoreCapabilitySource* tokens ("" only for the zero
	// value, which behaves as freq_only pricing without a disclosure claim).
	source string
	// domains is the underlying frequency-domain resolution (shared with the
	// CFR donor-reuse lane so one query resolves clusters exactly once).
	domains clusterFreqDomains
	// capByCluster / classByCluster key on the domain label. (The former
	// refCap nomination field is retired — see the EVOLUTION RECORD above
	// coreCapabilityClassRank; it was written and never read once the
	// demotion walk went away.)
	capByCluster   map[string]float64
	classByCluster map[string]string

	// CAP-2 (§28.4/§28.5) evidence companions:
	//
	//	topologySource   — CoreCapabilityTopology* token ("" for explicit/
	//	                   legacy/freq_only records: absence keeps every
	//	                   pre-CAP-2 wire byte identical);
	//	fmaxByCluster    — the class-ORDERING fmax value per cluster
	//	                   (max() over limits/rail/observed, full trace —
	//	                   R5c §29.96.1) — also the THERM comparison base;
	//	railByCluster /  — the validated Tier-2 rail governance timeline and
	//	railNameByCluster  verbatim rail name per cluster label (fold slice
	//	                   lane + audit disclosure);
	//	railFamily       — the adopted family mask (审计注可回溯, §28.5-T6);
	//	railRejectReason — typed pin surface: why a gate-passing family was
	//	                   discarded at combination time (cross-validation /
	//	                   structure conflict), "" otherwise;
	//	comoveFloorTripped — Tier-1 样本数下限门 fired (pin surface).
	topologySource     string
	fmaxByCluster      map[string]int64
	railByCluster      map[string][]freqSample
	railNameByCluster  map[string]string
	railFamily         string
	railRejectReason   string
	comoveFloorTripped bool

	// freqOnlySplitAudit (CAP-3 复核 P2, 2026-07-10): when a DERIVED-lane
	// judgment fails loud to freq_only on a fragmentation arm (>4 clusters /
	// fmax tie), this localizes the FIRST co-movement split behind the
	// offending pair — "cpuA↔cpuB @ts 判定臂=token(zh label)".
	// DISCLOSURE/AUDIT ONLY: it rides SupplyFoldBasis.CapabilitySplitAudit
	// and the result-caveat lane so a customer replay can tell a carve-
	// boundary form (healed by CAP-3) from real mid-stream divergence (the
	// honest residual); NO gate may consume it (precise-signals-for-hard-
	// gates red line — this is a disclosure, not a door). Empty on explicit/
	// keyed-rail/legacy lanes and on the non-fragmentation freq_only arms
	// (<2 clusters, comove floor).
	freqOnlySplitAudit string

	// freqOnlyReason (CLUSTER-FIX-2 件1, S1): the typed freq_only cause token
	// (CoreCapabilityFreqOnlyReason* closed set). Set on EVERY freq_only
	// verdict this resolver mints; "" on judged verdicts and on the zero
	// value (which claims nothing). Disclosure only, never a gate.
	freqOnlyReason string

	// fmaxTieBreakAudit (CLUSTERTIE-1, §29.197①, 2026-07-21): when a JUDGED
	// verdict's class order required the fmax tie-break chain (two adjacent
	// clusters shared one three-lane fmax and a precise chain signal ordered
	// them), this discloses the pair, the tied value, the deciding chain and
	// both sides' deciding key values —
	// "labelLow↔labelHigh fmax=NkHz 破局链=chain(zh:XkHz vs YkHz)".
	// DISCLOSURE/AUDIT ONLY: it rides SupplyFoldBasis.CapabilityTieBreakAudit
	// and the engine caveat lane so a customer replay can audit WHY the tie
	// no longer degrades; no gate may ever read it (the chain verdict itself
	// already ordered the classes). Empty on untied verdicts, on every
	// freq_only verdict, and on pre-batch records.
	fmaxTieBreakAudit string

	// limitsAnchorMismatch (CLUSTER-FIX-2 件4, C2 底稿 cluster_audit_refs_
	// 20260718.md + S9 评估 cluster_audit_code_20260718.md): sorted
	// cpu_frequency_limits anchor CPUs (limits lanes are per-policy, keyed
	// cpu_id = policy leader — donghu witness anchors {0,4}) that sit
	// STRICTLY INSIDE a derived cluster's membership instead of at its first
	// member. A limits anchor is policy-boundary evidence, so an interior
	// anchor says the derived partition may have merged two policies
	// (constant-equal-value parked clusters). DISCLOSURE ONLY: S9 adjudicates
	// the membership arm (anchor-based subdivision / contiguity presumption)
	// as a ruling candidate — the per-policy-leader emission convention is a
	// fleet-level shape ASSUMPTION (a platform emitting limits on every
	// member would hard-split correct clusters), so no gate reads this roster
	// (precise-signals red line: hard arms need signals, not conventions).
	// Empty on non-derived lanes and when every anchor is a cluster start.
	limitsAnchorMismatch []int
}

// usable reports whether a class table is in force (default or, once wired,
// evidence). freq_only / zero-value maps price at 1 everywhere.
func (m coreCapabilityMap) usable() bool {
	return m.source == CoreCapabilitySourceDefault || m.source == CoreCapabilitySourceEvidence
}

// clusterLabelFor resolves cpu's domain label — direct map membership only.
// R6 (规则1/规则3): leading and enclosed sample-less cores are members of the
// derived map itself; cross-cluster gap cores and cores above the highest
// sampled core resolve to "" and price conservatively (向上不外推 unchanged;
// the former 向下继承 inheritance arm is retired — see
// deriveClusterFreqDomains).
func (m coreCapabilityMap) clusterLabelFor(cpu int) string {
	return m.domains.byCPU[cpu]
}

// capabilityFor returns cpu's class coefficient; 1 for unknown membership or a
// non-usable map. Direction caveat (复核 F2, 2026-07-08): the bare 1 fallback
// is conservative ONLY on the VS-2 slice side (it reproduces the pre-CAP pure
// frequency ratio there — never a guessed class advantage). On the R5d WAKER
// side a silent 1 UNDERSTATES an undeclared fast core and fabricates deficit,
// so R5d callers MUST use capabilityForKnown and degrade the whole slice to
// the pure frequency comparison when EITHER side's membership is unknown
// ("missing contributes zero, never a guess").
func (m coreCapabilityMap) capabilityFor(cpu int) float64 {
	cap, _ := m.capabilityForKnown(cpu)
	return cap
}

// capabilityForKnown is capabilityFor plus the PRECISE membership signal:
// known=false when the map is not usable or cpu's cluster membership (and
// therefore its class) is unknown. Callers that must not treat "unknown" as
// "coefficient 1" (R5d, 复核 F2) branch on the boolean.
func (m coreCapabilityMap) capabilityForKnown(cpu int) (float64, bool) {
	if !m.usable() {
		return 1, false
	}
	if cap, ok := m.capByCluster[m.clusterLabelFor(cpu)]; ok && cap > 0 {
		return cap, true
	}
	return 1, false
}

// sliceCapRatio is the VS-2 per-slice multiplier cap(实际核类)/cap(基准簇)
// applied on top of the frequency ratio against the CHOSEN reference
// cluster's cap (复核 F1: the caller passes the same-cluster pair's cap,
// sourced from supplyFoldGlobalMaxBasis). Unknown membership
// under a usable map returns 1: the slice then folds at the pure frequency
// ratio against the reference fmax — exactly the pre-CAP value, a bounded
// legacy-equivalent fallback (see capabilityFor for the direction analysis).
func (m coreCapabilityMap) sliceCapRatio(cpu int, refCap float64) float64 {
	if !m.usable() || refCap <= 0 {
		return 1
	}
	if cap, known := m.capabilityForKnown(cpu); known {
		return cap / refCap
	}
	return 1
}

// resolveCoreCapability builds the capability judgment over the resolved
// frequency domains and the resolving face's OWN full-trace positive per-CPU
// sample timelines. Zero carry barriers are deliberately absent. See the file header for
// the discipline; every fallback is typed freq_only, never a guess.
// Behavior-identical thin wrapper over the CAP-2 evidence resolver with no
// limits ladder and no rail evidence (the pre-CAP-2 shape, pinned).
func resolveCoreCapability(domains clusterFreqDomains, timelines map[int][]freqSample) coreCapabilityMap {
	return resolveCoreCapabilityEvidence(domains, timelines, nil, nil)
}

// resolveCoreCapabilityEvidence is the CAP-2 (§28.4/§28.5) two-tier evidence
// resolver. Priority ladder: 显式拓扑 > Tier-1 (co-movement derivation, behind
// the sample floor) > Tier-2 (six-gate keyed rail) > freq_only fallback.
// limits is the full-trace per-CPU cpu_frequency_limits Max timeline (nil =
// no ladder rung); rail is the six-gate-passed adoption (nil = none). Every
// fallback stays typed freq_only, never a guess.
func resolveCoreCapabilityEvidence(domains clusterFreqDomains, timelines, limits map[int][]freqSample, rail *clusterRailAdoption) coreCapabilityMap {
	out := coreCapabilityMap{source: CoreCapabilitySourceFreqOnly, domains: domains}
	var railTL map[string][]freqSample
	var railName map[string]string
	railFamily := ""
	if rail != nil {
		railFamily = rail.family
	}
	switch {
	case domains.source == ClusterFreqSourceExplicit:
		// Explicit topology outranks both tiers: rail evidence is not
		// consulted at all (§28.5 priority ladder).
	case domains.source == ClusterFreqSourceDerived:
		// Tier-1 样本数下限门 (see clusterFreqComoveMinSamples): an
		// unsupported multi-CPU merge corrupts the cluster count — the whole
		// judgment fails loud (no Tier-2 rescue against equally-thin
		// cross-validation data; conservative both ways: the fold degrades to
		// the pre-CAP pure frequency ratio). R6 (规则1/规则3): the floor
		// judges the CO-MOVEMENT claim, which only SAMPLED members make —
		// closure members (leading/enclosed sample-less cores) are absorbed by
		// interval rules, not by curve identity, and neither arm the floor nor
		// serve as its representative.
		for _, members := range domains.members {
			sampledMembers, rep := 0, -1
			for _, cpu := range members {
				if len(timelines[cpu]) > 0 {
					if rep < 0 {
						rep = cpu
					}
					sampledMembers++
				}
			}
			if sampledMembers >= 2 && len(timelines[rep]) < clusterFreqComoveMinSamples {
				out.comoveFloorTripped = true
				// CLUSTER-FIX-2 件1+件3 (S1/C1): the floor verdict itself is
				// unchanged (§28.5 复核 P1 ruling); the reason token refines to
				// the single-burst form when the merge evidence was exactly one
				// co-emission burst (see the token doc — disclosure only).
				out.freqOnlyReason = CoreCapabilityFreqOnlyReasonComoveFloor
				if comoveFloorSingleBurstWitness(members, timelines) {
					out.freqOnlyReason = CoreCapabilityFreqOnlyReasonComoveFloorSingleBurst
				}
				return out
			}
		}
		if rail != nil {
			// 两级并存 (§28.5): rail values must be compatible with the
			// anchor clusters' own measured samples; any positive proximate
			// contradiction discards Tier-2 (Tier-1 kept).
			if !clusterRailCrossValidate(rail, timelines) {
				out.railRejectReason = clusterRailRejectCrossValidation
				rail, railFamily = nil, ""
			} else if refined := refineDomainsWithRails(domains, rail); !refined.ok {
				out.railRejectReason = refined.reason
				rail, railFamily = nil, ""
			} else {
				domains = refined.domains
				out.domains = domains
				railTL = refined.railTLByLabel
				railName = refined.railNameByLabel
				if refined.structureUsed {
					out.topologySource = CoreCapabilityTopologyKeyedRail
				} else {
					// Rail aligned 1:1 with the measured clusters: membership
					// stays the Tier-1 claim, the rail rides only as an fmax
					// rung + THERM/audit timeline.
					out.topologySource = CoreCapabilityTopologyComovement
				}
			}
		}
		if out.topologySource == "" {
			out.topologySource = CoreCapabilityTopologyComovement
		}
	default:
		// No explicit topology, no Tier-1 samples: pure Tier-2 (§28.5 —
		// cpu_frequency 缺位时). Cross-validation is vacuous by construction
		// (no member has samples); the membership presumption is disclosed.
		if rail == nil {
			out.freqOnlyReason = CoreCapabilityFreqOnlyReasonNoDomains
			return out
		}
		domains, railTL, railName = railOnlyDomains(rail)
		out.domains = domains
		out.topologySource = CoreCapabilityTopologyKeyedRail
	}
	if !domains.known() {
		out.topologySource = ""
		out.freqOnlyReason = CoreCapabilityFreqOnlyReasonNoDomains
		return out
	}
	// CLUSTER-FIX-2 件4 (C2, disclosure only — see the field doc): the limits
	// anchor consistency check runs over the FINAL derived membership (post
	// rail refinement). The comove-floor arm above returns before membership
	// is final and is deliberately exempt (its structure is never published).
	out.limitsAnchorMismatch = limitsAnchorMismatchCPUs(domains, limits)
	// Sampled clusters: class-ordering fmax per cluster label via the CAP-2
	// ladder (see coreCapabilityClusterFmax). A cluster with no rung at all
	// never participates (unchanged: it folds as UNKNOWN basis).
	type clusterFmax struct {
		label string
		fmax  int64
	}
	var clusters []clusterFmax
	for label, members := range domains.members {
		fmax := coreCapabilityClusterFmax(members, railTL[label], timelines, limits)
		if fmax > 0 {
			clusters = append(clusters, clusterFmax{label: label, fmax: fmax})
		}
	}
	classes, ok := coreCapabilityClassesByClusterCount[len(clusters)]
	if !ok {
		// <2 (no cross-class information) or >4 (outside the §26 table):
		// unjudgeable — fail-loud freq_only.
		out.topologySource = ""
		switch {
		case len(clusters) == 0:
			out.freqOnlyReason = CoreCapabilityFreqOnlyReasonNoSampledCluster
		case len(clusters) == 1:
			out.freqOnlyReason = CoreCapabilityFreqOnlyReasonSingleCluster
		default:
			out.freqOnlyReason = CoreCapabilityFreqOnlyReasonClusterOverflow
		}
		if len(clusters) > 4 {
			// 复核 P2: >4 derived groups is the fragmentation shape — locate
			// a same-fmax twin pair (else the two lowest-fmax groups) and
			// disclose where the criterion split them (audit only, no gate).
			sorted := append([]clusterFmax(nil), clusters...)
			sort.SliceStable(sorted, func(i, j int) bool {
				if sorted[i].fmax != sorted[j].fmax {
					return sorted[i].fmax < sorted[j].fmax
				}
				return sorted[i].label < sorted[j].label
			})
			pairA, pairB := sorted[0].label, sorted[1].label
			for i := 1; i < len(sorted); i++ {
				if sorted[i].fmax == sorted[i-1].fmax {
					pairA, pairB = sorted[i-1].label, sorted[i].label
					break
				}
			}
			out.freqOnlySplitAudit = joinCapabilitySplitAuditClauses(
				capabilityFreqOnlySplitAudit(domains, timelines, pairA, pairB),
				capabilityPartitionRefusalClause(domains),
			)
		}
		return out
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].fmax != clusters[j].fmax {
			return clusters[i].fmax < clusters[j].fmax
		}
		return clusters[i].label < clusters[j].label
	})
	for i := 1; i < len(clusters); i++ {
		if clusters[i].fmax != clusters[i-1].fmax {
			continue
		}
		// CLUSTERTIE-1 (§29.197①, 2026-07-21): before the tie fails loud, the
		// precise tie-break chain gets one shot at ordering the tied PAIR —
		// see resolveCoreCapabilityFmaxTie for the three ruled chains. A run
		// of ≥3 equal-fmax clusters stays the honest tie: per-pair chain
		// verdicts need not be mutually transitive across a longer run, and a
		// composite key would order B↔C by a chain that never compared them
		// (禁掷币 extends to fabricated transitivity; the §26 table tops out
		// at 4 clusters, so a 3-way tie is already the deep-fragmentation
		// shape the honest arm exists for).
		pairTie := i+1 >= len(clusters) || clusters[i+1].fmax != clusters[i].fmax
		var tb coreCapabilityFmaxTieBreak
		if pairTie {
			tb = resolveCoreCapabilityFmaxTie(
				domains.members[clusters[i-1].label], domains.members[clusters[i].label],
				timelines, limits)
		}
		if !tb.decided {
			// An fmax tie the chain cannot order leaves no defensible order
			// between the two clusters — 簇结构不可判, fail-loud freq_only
			// (precise signal, never a coin flip on the label sort).
			out.topologySource = ""
			out.freqOnlyReason = CoreCapabilityFreqOnlyReasonFmaxTie
			// 复核 F1 (CLUSTERTIE-1 dual review 2026-07-21, P1): an EARLIER
			// tied pair in this same loop may already have minted a tie-break
			// audit. The whole verdict is now freq_only, so that audit is
			// stale — leaking it would put 「已由精确信号链定序…核类照常折算」
			// on the wire/caveat face of a degraded Result (答案面自相矛盾)
			// and break the field promise "Empty on every freq_only verdict".
			out.fmaxTieBreakAudit = ""
			// 复核 P2: disclose where the tied pair split (audit only).
			out.freqOnlySplitAudit = joinCapabilitySplitAuditClauses(
				capabilityFreqOnlySplitAudit(domains, timelines, clusters[i-1].label, clusters[i].label),
				capabilityPartitionRefusalClause(domains),
			)
			return out
		}
		// The chain ordered the pair: arrange ascending by the deciding key
		// (the tied fmax VALUES stay untouched — only the class ORDER between
		// the two clusters is judged) and disclose the verdict.
		lowKhz, highKhz := tb.aKhz, tb.bKhz
		if tb.aKhz > tb.bKhz {
			clusters[i-1], clusters[i] = clusters[i], clusters[i-1]
			lowKhz, highKhz = tb.bKhz, tb.aKhz
		}
		audit := fmt.Sprintf("%s↔%s fmax=%dkHz 破局链=%s(%s:%dkHz vs %dkHz)",
			clusters[i-1].label, clusters[i].label, clusters[i].fmax,
			tb.chain, coreCapabilityTieBreakChainZH(tb.chain), lowKhz, highKhz)
		if out.fmaxTieBreakAudit != "" {
			out.fmaxTieBreakAudit += "; " + audit
		} else {
			out.fmaxTieBreakAudit = audit
		}
	}
	out.source = CoreCapabilitySourceDefault
	out.capByCluster = make(map[string]float64, len(clusters))
	out.classByCluster = make(map[string]string, len(clusters))
	out.fmaxByCluster = make(map[string]int64, len(clusters))
	for i, cluster := range clusters {
		class := classes[i]
		out.classByCluster[cluster.label] = class
		out.capByCluster[cluster.label] = coreCapabilityDefaultByClass[class]
		out.fmaxByCluster[cluster.label] = cluster.fmax
	}
	out.railByCluster = railTL
	out.railNameByCluster = railName
	if len(railTL) > 0 {
		out.railFamily = railFamily
	}
	return out
}

// capabilityFreqOnlySplitAudit renders the disclosure-only split localization
// for a fragmentation freq_only verdict on the DERIVED lane (CAP-3 复核 P2;
// CLUSTERSTREAM-1 件3 败因因子 extension, §29.193.1). It re-diagnoses the
// offending pair's representatives with the SAME criterion implementation
// (freqWitnessCoMoveDiag — no second judgment copy may grow) and formats the
// localization elements plus the failure factors:
//
//	transition_conflict → "cpuA↔cpuB @ts 判定臂=transition_conflict(同窗异值
//	                       变迁:XkHz vs YkHz,偏斜Z.Zµs)" — the kHz 不等
//	                       factor with both sides' transition targets and the
//	                       conflicting pair's skew;
//	co_witness_floor    → "cpuA↔cpuB @ts 判定臂=co_witness_floor(共见证变迁
//	                       不足:共见证=N(<2)[;最近同值变迁偏斜Z.Zµs超界15µs])"
//	                       — the accumulated pro count, plus the 偏斜超界
//	                       factor when a same-value near-miss beyond the
//	                       fixed bound was observed.
//
// Empty when either roster is empty, on non-derived lanes (explicit
// membership cannot fragment; keyed-rail members carry no samples to
// diagnose), or when the pair co-moves AND no recorded veto explains the
// split (post-rail-refinement label shapes). Disclosure only — no gate may
// ever read it.
//
// 复核 F1 (2026-07-21): a co-moving REPRESENTATIVE pair does not mean the
// fragments split without contradiction — the union veto may have fired on a
// NON-representative cross pair ({0,1}|{2,3} split by con(1,2) while the
// representative pair (0,2) itself holds pro≥floor ∧ con==0). The derivation
// records every union-refusing con edge (clusterFreqDomains.conVetoes); when
// the representative re-diagnosis would come back empty-handed, the audit
// discloses the recorded edge between THESE two fragments directly — the
// transition_conflict factors verbatim, on the edge's own cpus. Zero
// matching record keeps the honest empty return.
//
// EVOLUTION RECORD (PARTDISC-1, 2026-07-24): the witness-lane localization
// above remains byte-identical, while the two fragmentation freq_only mint
// sites append capabilityPartitionRefusalClause. The appended clause reports
// exact partition_below_floor / partition_drift / partition_limits_veto facts
// carried from derivation; it never feeds a decision.
func capabilityFreqOnlySplitAudit(domains clusterFreqDomains, timelines map[int][]freqSample, labelA, labelB string) string {
	if domains.source != ClusterFreqSourceDerived {
		return ""
	}
	// R6 (规则1/规则3): representatives must be SAMPLED members — a closure
	// member (leading/enclosed sample-less core) carries no curve to diagnose.
	sampledRep := func(members []int) int {
		for _, cpu := range members {
			if len(timelines[cpu]) > 0 {
				return cpu
			}
		}
		return -1
	}
	repA, repB := sampledRep(domains.members[labelA]), sampledRep(domains.members[labelB])
	if repA < 0 || repB < 0 {
		return ""
	}
	cpuA, cpuB := repA, repB
	comoves := freqTimelinesSameEmission(timelines[repA], timelines[repB])
	var split freqCoMoveSplit
	if !comoves {
		var ok bool
		ok, split = freqWitnessCoMoveDiag(timelines[repA], timelines[repB])
		comoves = ok || split.arm == ""
	}
	if comoves {
		v, found := domains.conVetoBetween(labelA, labelB)
		if !found {
			return ""
		}
		cpuA, cpuB = v.cpuA, v.cpuB
		split = freqCoMoveSplit{arm: freqCoMoveSplitArmConflict, ts: v.ts,
			conKhzA: v.khzA, conKhzB: v.khzB, conSkewSec: v.skewSec}
	}
	factor := ""
	switch split.arm {
	case freqCoMoveSplitArmConflict:
		factor = fmt.Sprintf(":%dkHz vs %dkHz,偏斜%.1fµs", split.conKhzA, split.conKhzB, split.conSkewSec*1e6)
	case freqCoMoveSplitArmFloor:
		factor = fmt.Sprintf(":共见证=%d(<%d)", split.pro, clusterFreqCoWitnessFloor)
		if split.nearSkewSet {
			factor += fmt.Sprintf(";最近同值变迁偏斜%.1fµs超界%.0fµs", split.nearSkewSec*1e6, clusterFreqDeriveMaxSkewSec*1e6)
		}
	}
	return fmt.Sprintf("cpu%d↔cpu%d @%.6f 判定臂=%s(%s%s)", cpuA, cpuB, split.ts, split.arm, freqCoMoveSplitArmZH(split.arm), factor)
}

// joinCapabilitySplitAuditClauses combines independent disclosure clauses
// without requiring either lane to fabricate a placeholder.
func joinCapabilitySplitAuditClauses(clauses ...string) string {
	kept := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		if clause = strings.TrimSpace(clause); clause != "" {
			kept = append(kept, clause)
		}
	}
	return strings.Join(kept, "; ")
}

// capabilityPartitionRefusalClause renders the announcement-partition facts
// already captured during derivation. It is deliberately independent of the
// witness-pair localization: a real partition refusal remains visible even
// when capabilityFreqOnlySplitAudit has no representative pair to diagnose.
func capabilityPartitionRefusalClause(domains clusterFreqDomains) string {
	if domains.source != ClusterFreqSourceDerived {
		return ""
	}
	audit := domains.partitionAudit
	var clauses []string
	switch audit.refusal {
	case announcePartitionRefusalDrift:
		clauses = append(clauses, fmt.Sprintf(
			"分区车道=%s(公告快照分区已运行:此前完整公告快照%d次,@%.6f 快照内值分组发生变化,分区证据整体弃用)",
			audit.refusal, audit.snapshots, audit.driftTs))
	case announcePartitionRefusalBelowFloor:
		clauses = append(clauses, fmt.Sprintf(
			"分区车道=%s(公告快照分区已运行:完整公告快照仅%d次(<%d),证据不足,未参与判簇)",
			audit.refusal, audit.snapshots, clusterFreqCoWitnessFloor))
	}
	for _, group := range audit.limitsVetoGroups {
		members := make([]string, 0, len(group.members))
		for _, cpu := range group.members {
			members = append(members, strconv.Itoa(cpu))
		}
		ceilings := make([]string, 0, len(group.ceilings))
		for _, khz := range group.ceilings {
			ceilings = append(ceilings, strconv.FormatInt(khz, 10))
		}
		clauses = append(clauses, fmt.Sprintf(
			"分区车道=%s(公告快照分区已运行:值组[cpu%s]带%d档不同限频上界(%skHz),按政策边界矛盾该组未合并)",
			announcePartitionRefusalLimitsVeto, strings.Join(members, ","),
			len(group.ceilings), strings.Join(ceilings, "/")))
	}
	return strings.Join(clauses, "; ")
}

// comoveFloorSingleBurstWitness (CLUSTER-FIX-2 件3, C1) reports whether one
// floor-tripping cluster's ENTIRE merge evidence is a single co-emission
// burst. All four conditions are literal per-field judgments over the typed
// sample timelines (precise; the skew bound is the FIXED
// clusterFreqDeriveMaxSkewSec constant — §29.129 既裁③ forbids adaptive burst
// widening in any hard arm, and this is not even an arm, only a disclosure
// refinement):
//
//	(1) every sampled member carries exactly ONE positive sample;
//	(2) all sample values are equal;
//	(3) the sample timestamps spread within the fixed skew bound;
//	(4) the sampled member ids form one contiguous ascending run
//	    (核号连续升序 — the refs' burst shape).
//
// The refs' emitter-identity condition (same comm/pid) is NOT materializable
// on the freqSample basis (and would not discriminate on the witness
// platform: one tppmgr thread emits every policy's rows) — recorded as the
// delegated remainder of C1's four-condition sketch.
func comoveFloorSingleBurstWitness(members []int, timelines map[int][]freqSample) bool {
	var sampled []int
	for _, cpu := range members {
		if len(timelines[cpu]) > 0 {
			sampled = append(sampled, cpu)
		}
	}
	if len(sampled) < 2 {
		return false
	}
	sort.Ints(sampled)
	first := timelines[sampled[0]][0]
	minTs, maxTs := first.ts, first.ts
	for i, cpu := range sampled {
		tl := timelines[cpu]
		if len(tl) != 1 {
			return false // (1)
		}
		if tl[0].khz != first.khz {
			return false // (2)
		}
		if tl[0].ts < minTs {
			minTs = tl[0].ts
		}
		if tl[0].ts > maxTs {
			maxTs = tl[0].ts
		}
		if i > 0 && sampled[i] != sampled[i-1]+1 {
			return false // (4)
		}
	}
	return maxTs-minTs <= clusterFreqDeriveMaxSkewSec // (3)
}

// limitsAnchorMismatchCPUs (CLUSTER-FIX-2 件4, C2) returns the sorted
// cpu_frequency_limits anchor CPUs that sit strictly INSIDE a derived
// cluster's membership (i.e. the anchor is a member but not the cluster's
// first member). A limits anchor is per-policy-leader evidence (donghu
// witness {0,4} = the first cores of the first two policies), so an interior
// anchor discloses that the derived partition may have merged two policies.
// Derived membership only — an explicit topology is user-authoritative and a
// keyed-rail membership is itself anchor-built; anchors with no membership
// (cross-cluster gap / above the highest sampled core) make no partition
// claim and are skipped. Disclosure roster only, never a gate (see the
// limitsAnchorMismatch field doc for the S9 ruling-candidate boundary).
func limitsAnchorMismatchCPUs(domains clusterFreqDomains, limits map[int][]freqSample) []int {
	if domains.source != ClusterFreqSourceDerived || len(limits) == 0 {
		return nil
	}
	var out []int
	for cpu, tl := range limits {
		if len(tl) == 0 {
			continue
		}
		members := domains.domainMembersFor(cpu)
		if len(members) == 0 || members[0] == cpu {
			continue
		}
		out = append(out, cpu)
	}
	sort.Ints(out)
	return out
}

// coreCapabilityClusterFmax is the CAP-2 class-ORDERING fmax for one
// cluster, full-trace caliber (class identity is a hardware attribute).
// EVOLUTION RECORD (R5c 终判 §29.96.1, 2026-07-15) — 各簇 fmax 同法: the
// former lane PRIORITY walk (limits > rail > observed) is retired; the value
// is now the max() over the three evidence lanes (最大可能频点):
//
//	(1) cpu_frequency_limits Max over the members — the cpufreq POLICY
//	    ceiling; its full-trace maximum is the least-throttled policy value,
//	    the closest offline lower bound on the rated ceiling (witness:
//	    observed-only ordering read {2..9}=1744000 above {10..13}=1200000
//	    and misclassed the big cluster — the limits rows 2200000 vs 2295000
//	    order it correctly; under max() the limits lane still supplies these
//	    values because the observed peaks sit below them);
//	(2) the cluster's validated Tier-2 rail maximum — the governance
//	    timeline's own ceiling;
//	(3) the highest observed cpu_frequency sample — under max() an observed
//	    peak ABOVE a pressed/stale limits ceiling now counts (整文件被压
//	    shape: the core demonstrably reached it, so the class-ordering
//	    ceiling must not read below it).
//
// Ties across clusters still fail loud upstream (禁掷币). With nil limits and
// nil rail this IS the pre-CAP-2 observed-max computation, byte for byte.
func coreCapabilityClusterFmax(members []int, railTL []freqSample, timelines, limits map[int][]freqSample) int64 {
	var fmax int64
	for _, cpu := range members {
		for _, sample := range limits[cpu] {
			if sample.khz > fmax {
				fmax = sample.khz
			}
		}
	}
	for _, sample := range railTL {
		if sample.khz > fmax {
			fmax = sample.khz
		}
	}
	for _, cpu := range members {
		for _, sample := range timelines[cpu] {
			if sample.khz > fmax {
				fmax = sample.khz
			}
		}
	}
	return fmax
}

// --- CLUSTERTIE-1 (§29.197①, 2026-07-21): the fmax tie-break chain ----------
//
// The customer verdict form: streaming co-movement JUDGES the clusters, then
// the very next link — the three-lane fmax ladder — reads two clusters at ONE
// value and the whole judgment dies on the 禁掷币 tie arm (capRatio=1 + the
// per-row 「核类排序不可判」 words the customer read as total failure). The
// tie value is real, but a tie of the max() aggregate does not mean the
// underlying evidence is orderless: three PRECISE signals, tried in the ruled
// order (前链断才试后链), can order the tied pair without a coin flip —
//
//	(a) limits_rank        — per-cluster cpu_frequency_limits lane maxima:
//	                         the least-throttled policy ceilings. Fires when
//	                         BOTH clusters carry a positive limits maximum and
//	                         they differ — the tie then necessarily came from
//	                         an observed/rail value at-or-above both ceilings
//	                         (had both fmax come FROM the limits lane, equal
//	                         fmax would force equal limits), i.e. the 整文件
//	                         被压/stale-limits shape; the rated-ceiling order
//	                         is the class order.
//	(b) depressed_observed_rank — the 去热限窗观测法: per-cluster observed
//	                         maxima EXCLUDING samples taken while the
//	                         cluster's own limits state sat strictly below its
//	                         full-trace limits-lane maximum (a thermal/policy
//	                         press window — the pathology that flattens two
//	                         clusters' observed peaks onto one value). Fires
//	                         when both de-pressed maxima are positive and
//	                         differ.
//	(c) value_set_rank     — the §29.193① 值集分层 second signal (用户⑦
//	                         触发条件 = 判簇歧义再启, hit by this very tie):
//	                         one cluster's observed VALUE SET carries a value
//	                         strictly above the other cluster's entire set —
//	                         for finite non-empty sets exactly max(A)≠max(B).
//	                         Fires when both observed maxima are positive and
//	                         differ (the tie then came from the limits/rail
//	                         lanes while the measured sets stratify).
//	                         值集丰富度(计数)≠算力序 — the chain reads set
//	                         ORDER only; sample/value COUNTS are noisy signals
//	                         and never enter (negative-armed in the pins).
//
// All three chains are constant-free pure comparisons over the typed sample
// timelines (§29.129 既裁③: zero adaptive thresholds; chain (b)'s press
// predicate is "strictly below the cluster's own lane maximum" — a value
// comparison, not a tolerance). All chains broken ⇒ the honest tie stands
// (真同 fmax 异算力 SoC and same-signal fragment twins keep the fail-loud
// freq_only arm — 宁漏勿假).

// Typed tie-break chain tokens (audit vocabulary; greppable beside their zh
// labels exactly like the freqCoMoveSplit arm tokens).
const (
	coreCapabilityTieBreakChainLimits    = "limits_rank"
	coreCapabilityTieBreakChainDepressed = "depressed_observed_rank"
	coreCapabilityTieBreakChainValueSet  = "value_set_rank"
)

// coreCapabilityTieBreakChainZH maps a chain token to its zh audit label.
func coreCapabilityTieBreakChainZH(chain string) string {
	switch chain {
	case coreCapabilityTieBreakChainLimits:
		return "限频上界分簇序"
	case coreCapabilityTieBreakChainDepressed:
		return "去热限窗实测序"
	case coreCapabilityTieBreakChainValueSet:
		return "频点值集分层序"
	default:
		return ""
	}
}

// coreCapabilityFmaxTieBreak is one tied pair's chain verdict: decided=false
// means every chain broke and the honest tie stands. aKhz/bKhz are the
// deciding chain's per-side key values in the CALLER's pair order (A = the
// caller's first cluster) — the caller derives the class order from them.
type coreCapabilityFmaxTieBreak struct {
	decided    bool
	chain      string
	aKhz, bKhz int64
}

// resolveCoreCapabilityFmaxTie runs the ruled chain sequence over one tied
// cluster pair (see the chain doc above; 前链断才试后链 — a chain whose
// preconditions fail OR whose keys compare equal is broken, and the next
// chain runs). Disclosure of the verdict rides fmaxTieBreakAudit; no other
// consumer may read the chain internals.
func resolveCoreCapabilityFmaxTie(membersA, membersB []int, timelines, limits map[int][]freqSample) coreCapabilityFmaxTieBreak {
	// chain (a): limits lane maxima.
	if la, lb := clusterLimitsLaneMax(membersA, limits), clusterLimitsLaneMax(membersB, limits); la > 0 && lb > 0 && la != lb {
		return coreCapabilityFmaxTieBreak{decided: true, chain: coreCapabilityTieBreakChainLimits, aKhz: la, bKhz: lb}
	}
	// chain (b): de-pressed observed maxima.
	if da, db := clusterDepressedObservedMax(membersA, timelines, limits), clusterDepressedObservedMax(membersB, timelines, limits); da > 0 && db > 0 && da != db {
		return coreCapabilityFmaxTieBreak{decided: true, chain: coreCapabilityTieBreakChainDepressed, aKhz: da, bKhz: db}
	}
	// chain (c): observed value-set stratification.
	if oa, ob := clusterObservedMax(membersA, timelines), clusterObservedMax(membersB, timelines); oa > 0 && ob > 0 && oa != ob {
		return coreCapabilityFmaxTieBreak{decided: true, chain: coreCapabilityTieBreakChainValueSet, aKhz: oa, bKhz: ob}
	}
	return coreCapabilityFmaxTieBreak{}
}

// clusterLimitsLaneMax is the cluster's cpu_frequency_limits lane maximum
// over its members (0 = no positive limits sample — chain (a) inapplicable).
func clusterLimitsLaneMax(members []int, limits map[int][]freqSample) int64 {
	var max int64
	for _, cpu := range members {
		for _, sample := range limits[cpu] {
			if sample.khz > max {
				max = sample.khz
			}
		}
	}
	return max
}

// clusterObservedMax is the cluster's observed cpu_frequency maximum over its
// members (0 = no positive observed sample — chain (c) inapplicable).
func clusterObservedMax(members []int, timelines map[int][]freqSample) int64 {
	var max int64
	for _, cpu := range members {
		for _, sample := range timelines[cpu] {
			if sample.khz > max {
				max = sample.khz
			}
		}
	}
	return max
}

// clusterDepressedObservedMax (chain (b), 去热限窗观测法) is the cluster's
// observed maximum EXCLUDING samples taken inside a press window. Press-state
// resolution (constant-free, documented): the cluster's positive limits
// samples merge into one ascending state timeline; an observed sample at t is
// PRESSED iff the latest limits sample at-or-before t carries a value
// strictly below the cluster's own limits-lane maximum. Samples before the
// first limits row are NOT pressed (unknown press state is not a press claim
// — absence never claims), and a cluster with no limits rows at all excludes
// nothing (vacuous exclusion: the de-pressed maximum IS the observed
// maximum). 0 = no positive observed sample survived — chain inapplicable.
func clusterDepressedObservedMax(members []int, timelines, limits map[int][]freqSample) int64 {
	var limitTL []freqSample
	var laneMax int64
	for _, cpu := range members {
		for _, sample := range limits[cpu] {
			if sample.khz <= 0 {
				continue
			}
			limitTL = append(limitTL, sample)
			if sample.khz > laneMax {
				laneMax = sample.khz
			}
		}
	}
	sort.SliceStable(limitTL, func(i, j int) bool { return limitTL[i].ts < limitTL[j].ts })
	pressed := func(ts float64) bool {
		// Latest limits sample at-or-before ts (linear from the tail is fine:
		// limits lanes are sparse per-policy rows; the derivation itself runs
		// once per Index).
		for i := len(limitTL) - 1; i >= 0; i-- {
			if limitTL[i].ts <= ts {
				return limitTL[i].khz < laneMax
			}
		}
		return false
	}
	var max int64
	for _, cpu := range members {
		for _, sample := range timelines[cpu] {
			if sample.khz <= 0 || sample.khz <= max {
				continue
			}
			if len(limitTL) > 0 && pressed(sample.ts) {
				continue
			}
			max = sample.khz
		}
	}
	return max
}

// classClusterMembers returns the member CPUs of the cluster classified as
// class ("" roster when the class is absent). One cluster per class by
// construction (the mapping assigns each class at most once).
func (m coreCapabilityMap) classClusterMembers(class string) []int {
	label, ok := m.classClusterLabel(class)
	if !ok {
		return nil
	}
	return m.domains.members[label]
}

// classClusterLabel resolves the domain label carrying class (CAP-2: the fold
// reference walk needs the label to reach the cluster's rail timeline).
func (m coreCapabilityMap) classClusterLabel(class string) (string, bool) {
	for label, c := range m.classByCluster {
		if c == class {
			return label, true
		}
	}
	return "", false
}

// presentClassesByRankDesc lists the judged classes present in this map,
// highest capability class first — the reference demotion walk order.
func (m coreCapabilityMap) presentClassesByRankDesc() []string {
	classes := make([]string, 0, len(m.classByCluster))
	for _, class := range m.classByCluster {
		classes = append(classes, class)
	}
	sort.SliceStable(classes, func(i, j int) bool {
		return coreCapabilityClassRank[classes[i]] > coreCapabilityClassRank[classes[j]]
	})
	return classes
}

// indexDerivedCoreClassByCPU is the R6 (§29.88.9) trace-global core-class
// authority for the topology-INFERENCE consumers (window faces + the fold's
// legacy basis picker when no explicit core_topology exists): cluster
// membership from the four-rule derivation (deriveClusterFreqDomains over the
// full-file curves), classes from the §26 fmax-ordered mapping — the SAME
// judgment the capability fold prices with, memoized once per Index.
//
// EVOLUTION RECORD (R6, CLUSTER-DERIVE): this REPLACES the positional-thirds
// inference (inferCoreTopologyFromFrequency — sorted CPUs split at len/3 and
// 2·len/3 by POSITION), whose noisy split misclassified donghu cpu9/10/11
// into big beside cpu12/13 (§29.88.8 scan; ground truth [0-3]小/[4-11]中/
// [12,13]大) and fabricated class words from EQUAL-fmax cores. An
// unjudgeable cluster structure now returns nil and the faces degrade to the
// honest unclassified form — noisy signals never mint class words (precise-
// signals red line). The 4-class prime label folds to "big" for the display
// taxonomy (normalizeCoreClass precedent); the R5a 核档 comparison reads
// cluster fmax integers, never these display words.
func indexDerivedCoreClassByCPU(idx *Index) map[int]string {
	capability := indexDerivedCoreCapability(idx)
	if !capability.usable() {
		return nil
	}
	out := map[int]string{}
	for cpu, label := range capability.domains.byCPU {
		class := capability.classByCluster[label]
		if class == "" {
			continue
		}
		if class == coreCapabilityClassPrime {
			class = coreCapabilityClassBig
		}
		out[cpu] = class
	}
	return out
}

// indexDerivedCoreCapability is the memoized trace-global R6 capability
// judgment over the derived (no-explicit-topology) domains — the shared
// measurement authority behind indexDerivedCoreClassByCPU and the R5a
// per-core-档 comparison (tiers are measured hardware facts; an explicit
// query topology re-labels classes but never mints frequency tiers).
func indexDerivedCoreCapability(idx *Index) coreCapabilityMap {
	if idx == nil {
		return coreCapabilityMap{}
	}
	idx.derivedClassOnce.Do(func() {
		cache := &chainQueryCache{idx: idx}
		idx.derivedCapability = cache.coreCapability("")
	})
	return idx.derivedCapability
}

// cpuConstraintTierExclusion is the R5a (§29.88.4 + §29.88.7 场景②) 按核档
// judgment (§29.88.8 B锚点: the donghu mask=ffb exclusion is invisible to the
// core-CLASS taxonomy — cpu9-11 are middle-tier, cpu12/13 the 2750000 tier —
// so the comparison reads the R6 clusters' fmax TIERS): the binding provably
// excludes a bigger core tier ⇔ every allowed CPU's cluster fmax is known
// AND their maximum sits STRICTLY below the trace-global maximum tier.
// ok=false on any unresolvable tier, on a binding that includes the top tier,
// or when no bigger tier exists — the mention obligation only fires on proof
// (禁无中生有; the negative arms are the tieba double-negative acceptance).
func cpuConstraintTierExclusion(capability coreCapabilityMap, allowedCPUs []int) (allowedMaxKHz, globalMaxKHz int64, ok bool) {
	if !capability.usable() || len(allowedCPUs) == 0 {
		return 0, 0, false
	}
	for _, fmax := range capability.fmaxByCluster {
		if fmax > globalMaxKHz {
			globalMaxKHz = fmax
		}
	}
	if globalMaxKHz <= 0 {
		return 0, 0, false
	}
	for _, cpu := range allowedCPUs {
		label := capability.clusterLabelFor(cpu)
		fmax := capability.fmaxByCluster[label]
		if label == "" || fmax <= 0 {
			return 0, 0, false // unknown tier — exclusion unprovable, fail open
		}
		if fmax >= globalMaxKHz {
			return 0, 0, false // the binding includes the top tier — no claim
		}
		if fmax > allowedMaxKHz {
			allowedMaxKHz = fmax
		}
	}
	if allowedMaxKHz <= 0 {
		return 0, 0, false
	}
	return allowedMaxKHz, globalMaxKHz, true
}

// coreCapability memoizes one capability resolution per core_topology input on
// the chain cache (the derivation walks every sampled timeline — resolving it
// per fold call was the pre-CAP shape for domains alone; the capability map
// now shares that single resolution with the CFR donor-reuse lane).
func (c *chainQueryCache) coreCapability(rawTopology string) coreCapabilityMap {
	if c == nil {
		// Nil cache (no trace index): no fold and no gated deficit can be
		// computed anyway — the zero value prices at 1 and claims nothing.
		return coreCapabilityMap{}
	}
	if c.capabilityByTopo == nil {
		c.capabilityByTopo = map[string]coreCapabilityMap{}
	}
	if capability, ok := c.capabilityByTopo[rawTopology]; ok {
		return capability
	}
	hardFrequencySamples := indexFreqSampleTimelines(c.idx)
	// CLUSTERSTREAM-1 件1: the derived arm reads the per-Index memo — one
	// witness derivation per trace, shared by every query (复用纪律).
	domains := resolveClusterFreqDomainsIndexed(rawTopology, c.idx)
	// CAP-2 (§28.4/§28.5): the full-trace limits ladder rung and the six-gate
	// keyed-rail evidence join the resolution (both memoized on the cache).
	c.buildFreqLimitIndex()
	capability := resolveCoreCapabilityEvidence(domains, hardFrequencySamples, c.freqLimitByCPU, c.clusterRailScanResult().adoption)
	c.capabilityByTopo[rawTopology] = capability
	return capability
}
