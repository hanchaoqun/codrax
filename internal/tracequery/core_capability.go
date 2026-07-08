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
//     is a noisy signal and never feeds capability classes.
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

import "sort"

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

// coreCapabilityReferenceClass is the §26 fold-reference NOMINATION (复核 F1,
// 2026-07-08): the fold's (fmax, cap) basis pair is taken from the 大核-class
// cluster — §26 letter: ideal = running×(f_actual×cap(实际核类)) /
// (f_bigmax×cap(大核)) — so a four-cluster shape does NOT fold to prime. A
// user ruling on whether the TOP cluster should be the reference instead is
// in flight; switching the nomination is this ONE constant. The two basis
// values MUST stay same-cluster same-source (supplyFoldCapabilityReference):
// mixing one cluster's fmax with another cluster's cap fabricated deficits
// (复核 Probe A: 1.650ms on a full-frequency big core; Probe B: 5.987ms on a
// small-only governance window).
const coreCapabilityReferenceClass = coreCapabilityClassBig

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
	// capByCluster / classByCluster key on the domain label; refCap is the
	// NOMINATED fold-reference coefficient (coreCapabilityReferenceClass —
	// the 大核-class cap, 复核 F1). The fold itself may DEMOTE the reference
	// when the nominated cluster has no window-governed fmax
	// (supplyFoldCapabilityReference); the chosen cluster's (fmax, cap) pair
	// then stays same-cluster same-source — refCap here is nomination info,
	// never mixed into a demoted fold.
	capByCluster   map[string]float64
	classByCluster map[string]string
	refCap         float64
}

// usable reports whether a class table is in force (default or, once wired,
// evidence). freq_only / zero-value maps price at 1 everywhere.
func (m coreCapabilityMap) usable() bool {
	return m.source == CoreCapabilitySourceDefault || m.source == CoreCapabilitySourceEvidence
}

// clusterLabelFor resolves cpu's domain label, mirroring the CFR membership
// rules: direct map hit; derived-source unsampled cores inherit the nearest
// higher-or-equal sampled core's domain (向下继承); cores above the highest
// sampled core get nothing (向上不外推 — the derived prime pseudo-domain has
// no fmax and no coefficient by construction).
func (m coreCapabilityMap) clusterLabelFor(cpu int) string {
	if label, ok := m.domains.byCPU[cpu]; ok {
		return label
	}
	if m.domains.source != ClusterFreqSourceDerived {
		return ""
	}
	idx := sort.SearchInts(m.domains.sampledAsc, cpu)
	if idx >= len(m.domains.sampledAsc) {
		return ""
	}
	return m.domains.byCPU[m.domains.sampledAsc[idx]]
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
// cluster's cap (复核 F1: the caller passes the same-cluster pair's cap —
// never m.refCap directly, which is nomination info only). Unknown membership
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
// frequency domains and the resolving face's OWN full-trace per-CPU sample
// timelines (fold face: chainQueryCache.freqByCPU). See the file header for
// the discipline; every fallback is typed freq_only, never a guess.
func resolveCoreCapability(domains clusterFreqDomains, timelines map[int][]freqSample) coreCapabilityMap {
	out := coreCapabilityMap{source: CoreCapabilitySourceFreqOnly, domains: domains}
	if !domains.known() {
		return out
	}
	// Sampled clusters only: full-trace fmax per domain label.
	type clusterFmax struct {
		label string
		fmax  int
	}
	var clusters []clusterFmax
	for label, members := range domains.members {
		fmax := 0
		for _, cpu := range members {
			for _, sample := range timelines[cpu] {
				if sample.khz > fmax {
					fmax = sample.khz
				}
			}
		}
		if fmax > 0 {
			clusters = append(clusters, clusterFmax{label: label, fmax: fmax})
		}
	}
	classes, ok := coreCapabilityClassesByClusterCount[len(clusters)]
	if !ok {
		// <2 (no cross-class information) or >4 (outside the §26 table):
		// unjudgeable — fail-loud freq_only.
		return out
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].fmax != clusters[j].fmax {
			return clusters[i].fmax < clusters[j].fmax
		}
		return clusters[i].label < clusters[j].label
	})
	for i := 1; i < len(clusters); i++ {
		if clusters[i].fmax == clusters[i-1].fmax {
			// An fmax tie leaves no defensible order between the two clusters
			// — 簇结构不可判, fail-loud freq_only (precise signal, never a
			// coin flip on the label sort).
			return out
		}
	}
	out.source = CoreCapabilitySourceDefault
	out.capByCluster = make(map[string]float64, len(clusters))
	out.classByCluster = make(map[string]string, len(clusters))
	for i, cluster := range clusters {
		class := classes[i]
		out.classByCluster[cluster.label] = class
		out.capByCluster[cluster.label] = coreCapabilityDefaultByClass[class]
	}
	// 复核 F1: refCap is the NOMINATED reference coefficient (§26 letter —
	// cap(大核), present in every mapping row), not the ladder top: a
	// four-cluster shape does not fold to prime.
	out.refCap = coreCapabilityDefaultByClass[coreCapabilityReferenceClass]
	return out
}

// classClusterMembers returns the member CPUs of the cluster classified as
// class ("" roster when the class is absent). One cluster per class by
// construction (the mapping assigns each class at most once).
func (m coreCapabilityMap) classClusterMembers(class string) []int {
	for label, c := range m.classByCluster {
		if c == class {
			return m.domains.members[label]
		}
	}
	return nil
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
	domains := resolveClusterFreqDomains(rawTopology, func() map[int][]freqSample {
		c.buildFreqIndex()
		return c.freqByCPU
	})
	c.buildFreqIndex()
	capability := resolveCoreCapability(domains, c.freqByCPU)
	c.capabilityByTopo[rawTopology] = capability
	return capability
}
